import { randomBytes } from "node:crypto";
import { expect, type APIRequestContext, type Page } from "@playwright/test";

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
