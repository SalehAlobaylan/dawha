import { randomBytes } from "node:crypto";
import { deflateSync } from "node:zlib";
import { expect, type APIRequestContext, type Page } from "@playwright/test";
import { Client } from "pg";

/**
 * Fixtures for the browser acceptance suite.
 *
 * Every test gets its own account with a unique address, so the journeys never
 * read each other's trees, sources or suggestions even though they share one
 * database. Nothing here deletes anything: a synthetic row left behind by a
 * failed run is harmless because the next run cannot collide with it, and the
 * rows are the only record that the journey happened. The one thing that is
 * cleaned up is the browser's own state, which Playwright does per test.
 */

export const API_URL = process.env.VITE_API_URL ?? `http://localhost:${process.env.CORE_API_PORT ?? 8181}`;
export const WEB_ORIGIN = `http://localhost:${process.env.WEB_E2E_PORT ?? 4173}`;

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let index = 0; index < 256; index += 1) {
    let value = index;
    for (let bit = 0; bit < 8; bit += 1) value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
    table[index] = value >>> 0;
  }
  return table;
})();

function crc32(data: Buffer): number {
  let value = 0xffffffff;
  for (const byte of data) value = CRC_TABLE[(value ^ byte) & 0xff] ^ (value >>> 8);
  return (value ^ 0xffffffff) >>> 0;
}

function pngChunk(type: string, data: Buffer): Buffer {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  const name = Buffer.from(type, "ascii");
  const checksum = Buffer.alloc(4);
  checksum.writeUInt32BE(crc32(Buffer.concat([name, data])));
  return Buffer.concat([length, name, data, checksum]);
}

/**
 * A large enough synthetic chart for the real source-space crop/repair path.
 * It contains one dark rectangle so the renderer and local quality checks have
 * pixels to measure; it is not a claim about any name and never becomes truth.
 */
export function syntheticSourcePNG(width = 600, height = 400): Buffer {
  const stride = width * 3 + 1;
  const raw = Buffer.alloc(stride * height, 245);
  for (let y = 0; y < height; y += 1) raw[y * stride] = 0;
  for (let y = 150; y < 190; y += 1) {
    for (let x = 200; x < 300; x += 1) {
      const offset = y * stride + 1 + x * 3;
      raw[offset] = 30;
      raw[offset + 1] = 25;
      raw[offset + 2] = 20;
    }
  }
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header[8] = 8;
  header[9] = 2;
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk("IHDR", header),
    pngChunk("IDAT", deflateSync(raw)),
    pngChunk("IEND", Buffer.alloc(0)),
  ]);
}

export interface Account {
  email: string;
  password: string;
  displayName: string;
}

/**
 * A fresh account. The domain is under example.invalid so it can never collide
 * with a real address, and the local part is random so two runs - or two tests
 * in one run - never do either.
 */
export function newAccount(label: string): Account {
  const token = randomBytes(6).toString("hex");
  return {
    email: `e2e-${label}-${token}@example.invalid`,
    password: `e2e-${token}-correct-horse`,
    displayName: `باحث ${label} ${token}`,
  };
}

/**
 * Registers through the UI, the way a reader does, and lands back on the app
 * with a session cookie. This is the register journey and the shared entry
 * point for the rest, so it is asserted rather than assumed.
 */
export async function registerThroughUi(page: Page, account: Account): Promise<void> {
  await page.goto("/login");
  await page.getByRole("button", { name: "إنشاء حساب" }).click();
  await page.getByLabel("الاسم بالعربية").fill(account.displayName);
  await page.getByLabel("البريد الإلكتروني").fill(account.email);
  // The password field's label also names the show/hide button beside it, so the
  // exact match is what selects the input rather than the button.
  await page.getByLabel("كلمة المرور", { exact: true }).fill(account.password);
  await page.getByRole("button", { name: "إنشاء الحساب" }).click();
  // Landing anywhere but the form is the redirect the form performs; the
  // session itself is asserted by the caller through /api/v1/auth/me, because
  // the shell has no signed-in banner to wait on.
  await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 20_000 });
}

/**
 * Signs an existing account in through the UI. Used for accounts that were
 * created through the API, because registering the same address twice is a
 * refusal, not a way in.
 */
export async function loginThroughUi(page: Page, account: Account): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("البريد الإلكتروني").fill(account.email);
  await page.getByLabel("كلمة المرور", { exact: true }).fill(account.password);
  await page.getByRole("button", { name: /دخول إلى دَوْحة/ }).click();
  await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 20_000 });
}

/** Registers an account through the API, for the journeys that need a second person. */
export async function registerThroughApi(request: APIRequestContext, account: Account): Promise<void> {
  const response = await request.post(`${API_URL}/api/v1/auth/register`, {
    data: { email: account.email, password: account.password, display_name_ar: account.displayName },
  });
  expect(response.status(), `register ${account.email}: ${await response.text()}`).toBe(201);
}

/**
 * The session cookie header for the API origin, assembled from the browser's own
 * cookie jar. A test that asks the API directly has to carry the session itself,
 * or it would be asking about an anonymous reader.
 */
export async function sessionCookieHeader(page: Page): Promise<string> {
  // The jar is read whole and filtered here: this Playwright version takes no
  // URL filter, and cookies() is asynchronous.
  const cookies = (await page.context().cookies()).filter(
    (cookie) => cookie.domain === "localhost" || cookie.domain === ".localhost",
  );
  return cookies.map((cookie) => `${cookie.name}=${cookie.value}`).join("; ");
}

/** Waits for the API to answer, so a journey never races the stack coming up. */
export async function waitForApi(request: APIRequestContext): Promise<void> {
  const response = await request.get(`${API_URL}/healthz`);
  expect(response.ok(), "core-api is not answering on /healthz").toBeTruthy();
  const ai = await request.get(`${process.env.AI_RESEARCH_URL ?? "http://localhost:8000"}/healthz`);
  expect(ai.ok(), "ai-research is not answering on /healthz").toBeTruthy();
}

/**
 * Grants a platform role to one of the suite's own synthetic accounts.
 *
 * Roles are not part of a signup journey - the API assigns `registered` and nothing
 * else - so a journey that needs a reviewer with a global write right has to grant
 * it, exactly as the Go fixtures do through testsupport.GrantRole. It writes through
 * `pg` rather than through an API route because there is no route, and inventing one
 * for a test would be a production surface added for a test.
 *
 * The row is removed by the teardown: user_roles cascades from users, and the
 * account is one of the synthetic actors cleanup.sql deletes.
 */
export async function grantPlatformRole(email: string, role: string): Promise<void> {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error("grantPlatformRole needs DATABASE_URL, which playwright.config.ts already requires");
  }
  const client = new Client({ connectionString: databaseUrl });
  await client.connect();
  try {
    const result = await client.query(
      `INSERT INTO user_roles (user_id, role)
       SELECT id, $2 FROM users WHERE email = $1
       ON CONFLICT (user_id, role) DO NOTHING
       RETURNING user_id`,
      [email, role],
    );
    expect(result.rowCount, `grant ${role} to ${email}: the account does not exist`).toBe(1);
  } finally {
    await client.end();
  }
}

/**
 * Reads rows for the journeys that assert on what the service recorded rather than on
 * what it serves. The audit trail and the stored change set are not readable through
 * any route, so "the review is audited" can only be proved against the table - and a
 * journey that asserted it through a route would be asserting a weaker thing.
 */
export async function readRows<T = Record<string, unknown>>(sql: string, values: unknown[] = []): Promise<T[]> {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error("readRows needs DATABASE_URL, which playwright.config.ts already requires");
  }
  const client = new Client({ connectionString: databaseUrl });
  await client.connect();
  try {
    const result = await client.query(sql, values);
    return result.rows as T[];
  } finally {
    await client.end();
  }
}

/**
 * Writes fixture rows that the browser suite owns. It is the same narrow seam as
 * readRows: the name bench has no public "create a machine reading" route, and
 * inventing one for a test would add a production surface. Callers must scope
 * every statement to the synthetic account/source they just created.
 */
export async function executeSql(sql: string, values: unknown[] = []): Promise<number> {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error("executeSql needs DATABASE_URL, which playwright.config.ts already requires");
  }
  const client = new Client({ connectionString: databaseUrl });
  await client.connect();
  try {
    const result = await client.query(sql, values);
    return result.rowCount ?? 0;
  } finally {
    await client.end();
  }
}

/** Waits for the source-processing worker to reach a terminal state on a run. */
export async function waitForProcessing(
  request: APIRequestContext,
  page: Page,
  sourceId: string,
  timeoutMs = 30_000,
): Promise<"succeeded" | "failed" | "still running"> {
  const deadline = Date.now() + timeoutMs;
  let last = "still running";
  while (Date.now() < deadline) {
    const response = await request.get(`${API_URL}/api/v1/sources/${sourceId}/processing`, {
      headers: { cookie: await sessionCookieHeader(page) },
    });
    if (response.ok()) {
      const body = (await response.json()) as { runs?: { status?: string }[] };
      const run = body.runs?.[0];
      if (run?.status === "succeeded") return "succeeded";
      if (run?.status === "failed") return "failed";
      last = run?.status ?? "queued";
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  return last === "failed" ? "failed" : "still running";
}
