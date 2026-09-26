import { defineConfig, devices } from "@playwright/test";

/**
 * The local acceptance stack. Every service is deterministic and needs no
 * credentials; the ports below are the whole contract.
 *
 *   55432  PostgreSQL (the existing local `db` container, postgis + vector).
 *          Already migrated and seeded by `make db-migrate db-seed`.
 *   8182   ai-research, a deterministic FastAPI app. The source-processing
 *          worker needs it as AI_RESEARCH_URL to resolve entities and embed
 *          passages, so the E2E stack starts it rather than leaving the one
 *          AI-backed test skipped. Needs services/ai-research/.venv.
 *   8181   core-api. Needs DATABASE_URL, AI_RESEARCH_URL and SOURCE_STORAGE_DIR.
 *          WEB_ORIGIN is the web origin so the session cookie is not refused.
 *   -      the source-processing and analysis workers. They serve no port, so they
 *          are supervised by the API process (`node e2e/stack.mjs api` starts all
 *          three and exits if any dies): an upload, an identity scan and a research
 *          investigation only reach a terminal state because something claims
 *          their job, so leaving a worker out would make the journey that needs it
 *          wait forever.
 *   4173   the built web app, served by `vite preview`. Preview rather than dev
 *          because it is the artefact CI would ship, and it needs no dev server
 *          restart between runs.
 *
 * The API and AI ports are deliberately NOT the development ports. A developer
 * with `npm run dev` running has a different server, with a different
 * WEB_ORIGIN and a different DATABASE_URL, already on 8080 and 8000. Reusing it
 * would let the suite report a pass against somebody else's process, so
 * reuseExistingServer is off everywhere and a busy port is a loud failure.
 *
 * Environment the runner itself needs:
 *   DATABASE_URL  required. Without it the API starts without a pool and every
 *                 journey fails at the first request, so the runner refuses to
 *                 start instead.
 *
 * The web app must be BUILT with VITE_API_URL pointing at the API port, because
 * the browser reads that at build time. `make e2e` does it. The browser then
 * talks to the API directly and the session cookie is a plain cross-origin
 * credentialed request, the same shape `npm run dev` produces.
 */
const apiPort = Number(process.env.CORE_API_PORT ?? 8181);
const aiPort = Number(process.env.AI_RESEARCH_PORT ?? 8182);
const webPort = Number(process.env.WEB_E2E_PORT ?? 4173);
const webOrigin = `http://localhost:${webPort}`;
const apiOrigin = `http://localhost:${apiPort}`;

export const E2E_PORTS = { web: webPort, api: apiPort, ai: aiPort } as const;

if (!process.env.DATABASE_URL) {
  throw new Error(
    "playwright.config.ts needs DATABASE_URL. Run `make db-migrate db-seed` first, or use the documented E2E_DATABASE_URL override.",
  );
}

export default defineConfig({
  testDir: "./e2e",
  // Every journey writes real rows: an account, a tree, a source, a question, an
  // audit trail. The teardown removes exactly those, scoped to the synthetic
  // e2e-*@example.invalid actors, so a run leaves the database it was pointed at
  // as it found it - the development database included, which is where
  // `make e2e` runs by default. It is one file (e2e/cleanup.sql) shared with
  // `make e2e-clean`, and it is a single transaction, so a failure rolls back
  // instead of leaving a half-cleaned database behind.
  globalTeardown: "./e2e/global-teardown.ts",
  // One worker: every journey shares one PostgreSQL database and one API, and the
  // journeys deliberately create public trees and public suggestions that a
  // second worker would read as its own. Serial execution is what makes the
  // suite deterministic; the tests themselves still use unique accounts.
  workers: 1,
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: process.env.CI ? [["github"], ["list"]] : [["list"]],
  use: {
    baseURL: webOrigin,
    actionTimeout: 15_000,
    navigationTimeout: 20_000,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: "node e2e/stack.mjs ai",
      url: `http://localhost:${aiPort}/healthz`,
      reuseExistingServer: false,
      timeout: 60_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      command: "node e2e/stack.mjs api",
      url: `${apiOrigin}/readyz`,
      reuseExistingServer: false,
      timeout: 120_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      command: "node e2e/stack.mjs web",
      url: webOrigin,
      reuseExistingServer: false,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
    },
  ],
});
