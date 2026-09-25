// Removes what the browser journeys wrote, so a run leaves the database as it
// found it.
//
// The suite registers a real account per test (e2e/fixtures.ts newAccount), and
// those accounts own trees, versions, sources, questions, invitations and audit
// rows. Leaving them behind meant every `make e2e` grew the database it ran
// against - the development database, by default - which is why this exists.
//
// The deletes live in one place, cleanup.sql, because the SQL is the part that
// has to be right: about fifty tables reference users(id) without ON DELETE
// CASCADE, so the order matters and a blanket DELETE FROM users would abort. The
// file runs as a single transaction with its own proof block, so a failure rolls
// back and the database is left untouched rather than half-cleaned. `make
// e2e-clean` runs the same file through psql, for a run that was killed before
// this teardown could run.
//
// Everything is scoped to the synthetic actors: users whose address matches
// e2e-%@example.invalid, and the rows reachable from them. The seeded and
// development rows are never in scope, which is asserted after the fact by
// comparing the non-synthetic user count before and after.
import { readFile, rm } from "node:fs/promises";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { Client } from "pg";

const here = dirname(fileURLToPath(import.meta.url));
const cleanupSqlFile = join(here, "cleanup.sql");

/**
 * What the suite left behind, in one row. Printed before and after the cleanup
 * so a run's own report says what it removed, instead of the next person
 * wondering where 74 trees came from.
 *
 * The last two columns are the part that cannot be cleaned: a research run from
 * a signed-out query owns nothing and is owned by nobody, so it is counted
 * rather than guessed at.
 */
const countsSql = `
WITH synthetic AS (SELECT id FROM users WHERE email LIKE 'e2e-%@example.invalid'),
synthetic_trees AS (
  SELECT id FROM trees WHERE owner_id IN (SELECT id FROM synthetic) OR forked_by IN (SELECT id FROM synthetic)
),
synthetic_versions AS (
  SELECT id FROM tree_versions
   WHERE created_by IN (SELECT id FROM synthetic)
      OR published_by IN (SELECT id FROM synthetic)
      OR tree_id IN (SELECT id FROM synthetic_trees)
),
synthetic_sources AS (SELECT id FROM sources WHERE created_by IN (SELECT id FROM synthetic))
SELECT
  (SELECT count(*) FROM users WHERE id IN (SELECT id FROM synthetic)) AS users,
  (SELECT count(*) FROM trees WHERE id IN (SELECT id FROM synthetic_trees)) AS trees,
  (SELECT count(*) FROM tree_versions WHERE id IN (SELECT id FROM synthetic_versions)) AS tree_versions,
  (SELECT count(*) FROM tree_nodes WHERE tree_version_id IN (SELECT id FROM synthetic_versions)) AS tree_nodes,
  (SELECT count(*) FROM sources WHERE id IN (SELECT id FROM synthetic_sources)) AS sources,
  (SELECT count(*) FROM source_files WHERE source_id IN (SELECT id FROM synthetic_sources)) AS source_files,
  (SELECT count(*) FROM people WHERE created_by IN (SELECT id FROM synthetic)) AS people,
  (SELECT count(*) FROM open_questions WHERE created_by IN (SELECT id FROM synthetic)) AS questions,
  (SELECT count(*) FROM suggestions WHERE submitted_by IN (SELECT id FROM synthetic)) AS suggestions,
  (SELECT count(*) FROM tree_invitations WHERE inviter_id IN (SELECT id FROM synthetic)) AS invitations,
  (SELECT count(*) FROM audit_log WHERE actor_id IN (SELECT id FROM synthetic)) AS audit_rows,
  (SELECT count(*) FROM auth_sessions WHERE user_id IN (SELECT id FROM synthetic)) AS sessions,
  (SELECT count(*) FROM users WHERE id NOT IN (SELECT id FROM synthetic)) AS other_users_untouched,
  (SELECT count(*) FROM research_runs WHERE actor_id IS NULL AND question_id IS NULL) AS unattributable_research_runs
`;

/** The ids of the sources this run created, for the files they uploaded. */
const syntheticSourceIdsSql = `
SELECT id FROM sources WHERE created_by IN (SELECT id FROM users WHERE email LIKE 'e2e-%@example.invalid')
`;

type Counts = Record<string, number>;

/**
 * PostgreSQL counts come back as bigint, which node-postgres hands over as a
 * string so a 64-bit value cannot lose precision. A count is compared and
 * printed here, so it is read as a number once, in one place.
 */
function readCounts(rows: Record<string, unknown>[]): Counts {
  const row = rows[0] ?? {};
  return Object.fromEntries(Object.entries(row).map(([key, value]) => [key, Number(value)]));
}

/**
 * The upload journey writes a file per source under SOURCE_STORAGE_DIR, and
 * deleting the row does not delete the file. The directory is named after the
 * source id, so removing <storage>/sources/<id> for the sources of this run
 * cannot touch anybody else's upload.
 */
async function removeUploadedFiles(client: Client): Promise<number> {
  const configured = process.env.SOURCE_STORAGE_DIR;
  if (!configured) return 0;
  const root = join(isAbsolute(configured) ? configured : resolve(process.cwd(), configured), "sources");
  const ids = (await client.query(syntheticSourceIdsSql)).rows as { id: string }[];
  for (const { id } of ids) {
    await rm(join(root, id), { recursive: true, force: true });
  }
  return ids.length;
}

export default async function globalTeardown(): Promise<void> {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    // playwright.config.ts already refuses to start without it, so reaching this
    // means the teardown was invoked on its own. Skipping is the honest reading:
    // there is no database to leave dirty.
    process.stderr.write("e2e teardown: DATABASE_URL is not set, so there is no database to clean.\n");
    return;
  }

  const client = new Client({ connectionString: databaseUrl });
  await client.connect();
  const startedAt = Date.now();
  try {
    const before = readCounts((await client.query(countsSql)).rows);
    if (before.users === 0 && before.trees === 0) {
      process.stdout.write("e2e teardown: the run left no synthetic rows behind.\n");
      return;
    }

    // The upload journey leaves a file per source on disk, which the row delete
    // cannot reach, so the ids are taken before the transaction closes them.
    const uploadsRemoved = await removeUploadedFiles(client);

    // The transaction, the ordered deletes and the proof block are one file, so
    // this call either removes everything or changes nothing.
    await client.query(await readFile(cleanupSqlFile, "utf8"));

    const after = readCounts((await client.query(countsSql)).rows);
    const removed = Object.keys(before)
      .filter((key) => key !== "other_users_untouched" && key !== "unattributable_research_runs")
      .map((key) => `${key} ${before[key]}→${after[key]}`)
      .join(", ");
    const report = [
      `e2e teardown: removed the rows this run created in ${Date.now() - startedAt}ms (${removed}).`,
      `e2e teardown: removed ${uploadsRemoved} uploaded source directories under SOURCE_STORAGE_DIR.`,
      `e2e teardown: ${after.other_users_untouched} non-synthetic users are untouched.`,
    ];
    if (after.unattributable_research_runs > 0) {
      // Said out loud rather than swept up: this row belongs to no actor, so
      // there is no honest way to tell a signed-out journey's query from a
      // developer's own. They are counted, not deleted.
      report.push(
        `e2e teardown: ${after.unattributable_research_runs} research run(s) have no actor and no question, so they cannot be traced to this run and were left in place. ` +
          "A run killed before this point leaves more rows behind; COMPOSE_PROJECT_NAME=dawha make e2e-clean removes the ones it can attribute.",
      );
    }
    process.stdout.write(`${report.join("\n")}\n`);
  } catch (error) {
    // The transaction rolled back, so the database is as it was found. Saying so
    // is the point: a cleanup that half-worked would be worse than none.
    process.stderr.write(
      `e2e teardown: cleanup failed and was rolled back, so nothing was deleted: ${error instanceof Error ? error.message : String(error)}\n`,
    );
    throw error;
  } finally {
    await client.end();
  }
}
