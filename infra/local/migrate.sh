#!/bin/sh
# The migration runner every developer, the E2E stack and CI already use.
#
# It is deliberately the ONLY way this repository applies a schema, because
# every one of those callers - `make db-migrate`, `make verify-full`, the CI
# database job, the E2E stack - reaches the database through this file, and a
# safety property that only one of four paths has is not a safety property.
#
# What it guarantees, and why each one is here:
#
#   * Each migration and its version/checksum row commit in ONE transaction.
#     The old runner applied the file in one `psql` call and recorded the version
#     in a second, so a failure between them left a schema with no version row:
#     the next run replayed a file whose objects already existed, or a file
#     whose objects did not. The per-file transaction here makes "the schema
#     moved" and "the database knows the schema moved" the same commit.
#
#   * A concurrent runner is serialized by a transaction-scoped advisory lock
#     taken as the FIRST statement of that same transaction. Two runners can
#     start at once - CI and a laptop, two CI jobs, a developer who ran it
#     twice - and the second one waits for the first to commit before deciding
#     whether the file still needs applying. The lock is transaction-scoped
#     rather than session-scoped so it is released by PostgreSQL itself if this
#     process is killed mid-migration, instead of being stranded until the
#     backend ends.
#
#   * Every applied migration is checksum-protected. A recorded checksum that
#     disagrees with the file on disk is a hard failure: an already-applied
#     migration is history, and editing it silently means two databases that
#     both claim to be current have different schemas.
#
#   * A database that was migrated BEFORE checksums existed still works. Every
#     such row has a NULL checksum, and NULL is not treated as a mismatch: the
#     checksum is backfilled from the file on disk and the backfill is itself
#     recorded (checksum_origin = 'backfilled_from_file'). A naive checksum
#     check would have failed on every database in existence on the day it
#     landed, which is the fastest way to make a migration tool get uninstalled.
#
# Usage:
#
#   infra/local/migrate.sh            apply pending migrations (default)
#   infra/local/migrate.sh check      report drift and change nothing
#
# Environment it reads:
#   POSTGRES_DB            which database to migrate. Default dawha.
#   POSTGRES_USER          default dawha
#   MIGRATIONS_DIR         the *.sql files. Default db/migrations. The
#                          migration test points this at a fixture directory.
#   MIGRATE_LOCK_TIMEOUT   how long to wait for the advisory lock, as a
#                          PostgreSQL interval. Default 60s. A lock held by
#                          something wedged is a loud failure, not a hang.
#
# `check` needs the same database `apply` would write to, and it is what CI
# uses to prove the tree and the database agree.
set -eu

user="${POSTGRES_USER:-dawha}"
database="${POSTGRES_DB:-dawha}"
migrations_dir="${MIGRATIONS_DIR:-db/migrations}"
lock_timeout="${MIGRATE_LOCK_TIMEOUT:-60s}"
# 'DAWH' as a big-endian int32, and 1 as the class. A named constant rather than
# a bare number because this value is written down here and nowhere else: two
# runners only exclude each other if they use the same pair.
lock_key1=1145131592
lock_key2=1

mode="${1:-apply}"
case "$mode" in
  apply | check) ;;
  *)
    printf 'usage: %s [apply|check]\n' "$0" >&2
    exit 2
    ;;
esac

psql_exec() {
  docker compose exec -T db psql -U "$user" -d "$database" "$@"
}

# The checksum is computed on the host because the migrations are on the host,
# and the three tools that can hash a file cover macOS, Linux and a machine
# with neither.
checksum_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    openssl dgst -sha256 "$1" | sed 's/^.*= *//'
  fi
}

# A version and a checksum are interpolated into SQL below, so both are
# constrained to the shape they are supposed to have before that happens. A
# migration file named `'; DROP TABLE users; --.sql` is not a scenario anybody
# needs to support, and refusing to quote it is how the runner stays safe
# without pretending to be a parser.
require_safe_version() {
  if ! printf '%s' "$1" | grep -Eq '^[0-9]{4}_[A-Za-z0-9][A-Za-z0-9_]*$'; then
    printf 'refusing to run: %s is not a NNNN_name.sql migration version\n' "$1" >&2
    exit 2
  fi
}

require_safe_checksum() {
  if ! printf '%s' "$1" | grep -Eq '^[0-9a-f]{64}$'; then
    printf 'refusing to run: %s is not a sha256 hex digest\n' "$1" >&2
    exit 2
  fi
}

files=''
for file in "$migrations_dir"/*.sql; do
  [ -e "$file" ] || continue
  files="$files $file"
done
if [ -z "$files" ]; then
  printf 'no migrations found in %s\n' "$migrations_dir" >&2
  exit 2
fi

container_id="$(docker compose ps -q db)"
attempt=0
until [ "$(docker inspect --format '{{.State.Health.Status}}' "$container_id" 2>/dev/null || true)" = "healthy" ]; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 45 ]; then
    printf '%s\n' "database did not become healthy" >&2
    exit 1
  fi
  sleep 1
done

attempt=0
until psql_exec -c "SELECT 1" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    printf '%s\n' "database did not become ready" >&2
    exit 1
  fi
  sleep 1
done

if [ "$mode" = "check" ]; then
  # One read-only statement reports every disagreement at once. A gate that
  # printed one problem per run would take several runs to describe one
  # divergence, and the second run would be against a database someone had
  # already started fixing.
  values=''
  for file in $files; do
    version="$(basename "$file" .sql)"
    require_safe_version "$version"
    sum="$(checksum_of "$file")"
    require_safe_checksum "$sum"
    values="$values('$version', '$sum'),"
  done
  values="${values%,}"

  applied="$(psql_exec -Atc "SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'schema_migrations'")"
  if [ "$applied" != "1" ]; then
    printf 'schema_migrations does not exist in %s; run make db-migrate first\n' "$database" >&2
    exit 1
  fi

  # The lock is session-scoped and taken in its own statement before the report
  # is read, so the report cannot be taken between another runner's schema change
  # and its version row. A lock and a report inside ONE statement would not: the
  # statement's snapshot is taken before the function in it is evaluated.
  # \gset keeps the lock's void result out of the report.
  problems="$(psql_exec -At -v ON_ERROR_STOP=1 <<SQL
SELECT pg_advisory_lock($lock_key1, $lock_key2) AS dawha_check_lock \gset
WITH on_disk(version, checksum) AS (VALUES $values),
applied AS (SELECT version, checksum FROM schema_migrations),
problems(problem) AS (
  SELECT 'applied but no file on disk: ' || version FROM applied WHERE version NOT IN (SELECT version FROM on_disk)
  UNION ALL
  SELECT 'no checksum recorded: ' || version FROM applied WHERE checksum IS NULL
  UNION ALL
  SELECT 'checksum mismatch: ' || applied.version FROM applied JOIN on_disk USING (version) WHERE applied.checksum IS DISTINCT FROM on_disk.checksum
  UNION ALL
  SELECT 'pending migration: ' || version FROM on_disk WHERE version NOT IN (SELECT version FROM applied)
)
SELECT problem FROM problems ORDER BY problem;
SELECT pg_advisory_unlock($lock_key1, $lock_key2) AS dawha_check_unlock \gset
SQL
  )" || {
    printf 'the migration drift check could not be evaluated\n' >&2
    exit 1
  }

  if [ -n "$problems" ]; then
    printf 'migration drift in %s against %s:\n' "$database" "$migrations_dir" >&2
    printf '%s\n' "$problems" | while IFS= read -r problem; do
      printf '  %s\n' "$problem" >&2
    done
    printf '%s\n' "  'no checksum recorded' is fixed by running make db-migrate, which backfills it from the file on disk" >&2
    exit 1
  fi
  printf 'migrations match: %s files, none pending, every applied checksum recorded and unchanged\n' "$(printf '%s' "$files" | wc -w | tr -d ' ')"
  exit 0
fi

# The bookkeeping table. The three added columns are nullable and defaulted, so
# this is the same statement on a database created by the previous runner and on
# a fresh one. `checksum_origin` exists because "the checksum was recorded when
# the file was applied" and "the checksum was read off the file afterwards" are
# different claims, and a row that cannot tell you which one it is making is not
# evidence of anything.
psql_exec -v ON_ERROR_STOP=1 -c "
  BEGIN;
  SELECT pg_advisory_xact_lock($lock_key1, $lock_key2);
  CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
  );
  ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum text;
  ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum_origin text;
  ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum_recorded_at timestamptz;
  COMMIT;
" >/dev/null

# One read of the applied state, so the common case - a rerun where everything
# already matches - costs one query and no transactions at all. Correctness does
# not rest on this read: every file that needs a transaction gets one, and that
# transaction re-decides under the lock.
#
# NULL is spelled '<none>' rather than left empty, because "this database was
# migrated before checksums existed" and "this database has never been migrated"
# are different states and only one of them is a backfill.
recorded_state="$(psql_exec -At -F '|' -v ON_ERROR_STOP=1 -c "SELECT version, coalesce(checksum, '<none>') FROM schema_migrations")"

applied_count=0
backfilled_count=0
concurrent_count=0
noop_count=0

for file in $files; do
  version="$(basename "$file" .sql)"
  require_safe_version "$version"
  sum="$(checksum_of "$file")"
  require_safe_checksum "$sum"

  recorded="$(printf '%s\n' "$recorded_state" | grep "^$version|" | head -1 | cut -d'|' -f2)"
  if [ -n "${recorded:-}" ]; then
    if [ "$recorded" = "$sum" ]; then
      # Already applied, and the file is the one that was applied. Nothing to
      # write, so no transaction and no lock.
      noop_count=$((noop_count + 1))
      continue
    fi
  fi

  # One transaction per file: lock, decide, apply, record, commit. The whole
  # file is streamed into the same psql session between \if and \else, which is
  # why it is not passed as a path - the migrations live on the host and psql
  # runs inside the container.
  #
  # The \if is re-evaluated INSIDE the lock, so the decision the transaction
  # acts on is the one it made after the other runner committed, not the one the
  # read above suggested. psql skips the discarded branch without parsing it,
  # which is what lets a migration containing DO $$ ... $$ blocks be skipped
  # without being mis-read as an unbalanced conditional.
  #
  # The branch that ran announces itself, because the shell's pre-lock read is
  # not the transaction's decision: a runner that lost the race to another
  # runner has done nothing, and a runner that says "applied" for a file the
  # other one applied is a runner lying to whoever reads the log.
  if outcome="$(psql_exec -v ON_ERROR_STOP=1 -q <<SQL
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '$lock_timeout';
SELECT pg_advisory_xact_lock($lock_key1, $lock_key2);
SELECT NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '$version') AS dawha_needs_apply \gset
\if :dawha_needs_apply
$(cat "$file")
INSERT INTO schema_migrations (version, checksum, checksum_origin, checksum_recorded_at)
  VALUES ('$version', '$sum', 'applied_with_file', now());
\echo dawha_outcome_applied
\else
DO \$dawha_migrate\$
DECLARE
  recorded text;
BEGIN
  SELECT checksum INTO recorded FROM schema_migrations WHERE version = '$version';
  IF recorded IS NULL THEN
    UPDATE schema_migrations
       SET checksum = '$sum',
           checksum_origin = 'backfilled_from_file',
           checksum_recorded_at = now()
     WHERE version = '$version';
  ELSIF recorded <> '$sum' THEN
    RAISE EXCEPTION 'migration % was applied as checksum % but %s now hashes to %. An applied migration is history: add a new migration instead of editing this one.', '$version', recorded, '$file', '$sum';
  END IF;
END
\$dawha_migrate\$;
\echo dawha_outcome_already_applied
\endif
COMMIT;
SQL
  )"; then
    case "$outcome" in
      *dawha_outcome_applied*)
        applied_count=$((applied_count + 1))
        printf 'applied %s\n' "$version"
        ;;
      *dawha_outcome_already_applied*)
        if [ "${recorded:-}" = "<none>" ]; then
          backfilled_count=$((backfilled_count + 1))
          printf 'backfilled the checksum for %s from the file on disk\n' "$version"
        else
          # The row was absent when this runner read the state and present by
          # the time it held the lock: another runner applied it in between.
          # Doing nothing here is the whole point of the lock.
          concurrent_count=$((concurrent_count + 1))
          printf 'skipped %s; another runner applied it while this one waited for the lock\n' "$version"
        fi
        ;;
      *)
        printf 'migration %s produced no outcome, so the runner does not know what it did\n' "$version" >&2
        exit 1
        ;;
    esac
  else
    status=$?
    printf 'migration %s (%s) failed and was rolled back; the database is unchanged by it\n' "$version" "$file" >&2
    exit "$status"
  fi
done

printf 'migrations: %s applied, %s checksums backfilled, %s skipped behind another runner, %s already current (%s in %s)\n' \
  "$applied_count" "$backfilled_count" "$concurrent_count" "$noop_count" \
  "$(printf '%s' "$files" | wc -w | tr -d ' ')" "$database"
