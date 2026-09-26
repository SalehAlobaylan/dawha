// Starts one service of the deterministic local E2E stack and waits until it is
// serving. Playwright's webServer runs this once per service, so the three
// processes are independent and a failure names the service that failed.
//
//   node e2e/stack.mjs ai    -> ai-research on AI_RESEARCH_PORT (8182)
//   node e2e/stack.mjs api    -> core-api on CORE_API_PORT (8181), supervising
//                               the source-processing and analysis workers
//   node e2e/stack.mjs web    -> vite preview on WEB_E2E_PORT (4173)
//
// The environment each service needs is asserted before it starts, so a missing
// variable is a clear message rather than a service that boots and then fails
// every request.
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = join(here, "..");
const repositoryRoot = join(webRoot, "..", "..");
const service = process.argv[2];

function required(name, hint) {
  const value = process.env[name];
  if (!value) {
    process.stderr.write(`stack.mjs: ${name} is not set. ${hint}\n`);
    process.exit(2);
  }
  return value;
}

function repositoryFile(relativePath, hint) {
  const path = join(repositoryRoot, relativePath);
  if (!existsSync(path)) {
    process.stderr.write(`stack.mjs: ${relativePath} is missing. ${hint}\n`);
    process.exit(2);
  }
  return path;
}

const services = {
  ai() {
    const python = join(repositoryRoot, "services", "ai-research", ".venv", "bin", "python");
    if (!existsSync(python)) {
      process.stderr.write("stack.mjs: services/ai-research/.venv is missing. Run `make install` first.\n");
      process.exit(2);
    }
    const port = process.env.AI_RESEARCH_PORT ?? "8182";
    return {
      command: python,
      args: ["-m", "uvicorn", "app.main:app", "--host", "localhost", "--port", port],
      cwd: join(repositoryRoot, "services", "ai-research"),
      // The deterministic provider needs no credentials, which is what makes
      // this stack runnable in CI without a secret.
      env: {},
    };
  },
  api() {
    const databaseURL = required(
      "DATABASE_URL",
      "Run `COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed`, or set E2E_DATABASE_URL.",
    );
    const storage = required(
      "SOURCE_STORAGE_DIR",
      "Point it at a writable directory; the E2E stack defaults it to apps/web/.e2e-storage.",
    );
    return {
      command: "go",
      args: ["run", "./cmd/api"],
      cwd: join(repositoryRoot, "services", "core-api"),
      env: {
        DATABASE_URL: databaseURL,
        // The processing worker resolves entities and embeds passages through
        // the AI service, so the API needs the URL even though a couple of its
        // own handlers only forward it.
        AI_RESEARCH_URL: process.env.AI_RESEARCH_URL ?? `http://localhost:${process.env.AI_RESEARCH_PORT ?? 8182}`,
        SOURCE_STORAGE_DIR: storage,
        CORE_API_PORT: process.env.CORE_API_PORT ?? "8181",
        // The session cookie is refused unless the request's Origin matches, so
        // the API has to be told which origin the browser will use.
        WEB_ORIGIN: process.env.WEB_ORIGIN ?? `http://localhost:${process.env.WEB_E2E_PORT ?? 4173}`,
        APP_ENV: "development",
        // Abuse controls are OFF for the browser suite, and off EXPLICITLY. The
        // journeys share one API and one client address, so the auth budget of 30
        // a minute is spent by the register journey alone. Setting it here rather
        // than relying on a default means a change that made a journey depend on
        // being unthrottled shows up in this diff, and it means the E2E stack is
        // never the reason a rate limit was quietly widened. The unit tests in
        // platform/ratelimit are what prove the limits work; the CI database job
        // runs the same API with the limits ON.
        RATE_LIMIT_ENABLED: process.env.RATE_LIMIT_ENABLED ?? "false",
      },
    };
  },
  web() {
    const built = join(webRoot, "dist", "index.html");
    if (!existsSync(built)) {
      process.stderr.write("stack.mjs: apps/web/dist is missing. Run `npm run build` first.\n");
      process.exit(2);
    }
    repositoryFile("db/migrations", "The web service does not need migrations; this is a wiring check.");
    return {
      command: "npx",
      args: ["vite", "preview", "--host", "localhost", "--port", process.env.WEB_E2E_PORT ?? "4173", "--strictPort"],
      cwd: webRoot,
      env: {},
    };
  },
};

if (!service || !services[service]) {
  process.stderr.write("stack.mjs: pass one of ai, api, worker, web.\n");
  process.exit(2);
}

const plans = [services[service]()];
// Every worker a job type needs, started next to the API that queues the work. A
// stack that starts the API without its workers accepts runs and finishes none of
// them, and the symptom is a journey that waits for a status that never changes - so
// the workers are part of what "the API is up" means here, not an optional extra.
if (service === "api" && process.env.E2E_START_WORKER !== "0") {
  plans.push(sourceWorker(), analysisWorker());
}

const children = plans.map((plan) =>
  spawn(plan.command, plan.args, {
    cwd: plan.cwd,
    env: { ...process.env, ...plan.env },
    stdio: ["ignore", "inherit", "inherit"],
  }),
);

function sourceWorker() {
  const databaseURL = required(
    "DATABASE_URL",
    "Run `COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed`, or point DATABASE_URL at a migrated database.",
  );
  return {
    command: "go",
    args: ["run", "./cmd/source-processing"],
    cwd: join(repositoryRoot, "services", "core-api"),
    env: {
      DATABASE_URL: databaseURL,
      AI_RESEARCH_URL: process.env.AI_RESEARCH_URL ?? `http://localhost:${process.env.AI_RESEARCH_PORT ?? 8182}`,
      SOURCE_STORAGE_DIR: required(
        "SOURCE_STORAGE_DIR",
        "Point it at a writable directory; `make e2e` sets it for you.",
      ),
      // A one-second poll keeps the browser journey quick without busy-looping.
      SOURCE_WORKER_POLL_INTERVAL: "1s",
      SOURCE_WORKER_ID: "e2e-source-worker",
    },
  };
}

function analysisWorker() {
  const databaseURL = required(
    "DATABASE_URL",
    "Run `COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed`, or point DATABASE_URL at a migrated database.",
  );
  return {
    command: "go",
    args: ["run", "./cmd/analysis-worker"],
    cwd: join(repositoryRoot, "services", "core-api"),
    env: {
      DATABASE_URL: databaseURL,
      // The scorer embeds entity names through the AI service. It has a local
      // fallback, so this URL is about fidelity rather than about the worker
      // running at all.
      AI_RESEARCH_URL: process.env.AI_RESEARCH_URL ?? `http://localhost:${process.env.AI_RESEARCH_PORT ?? 8182}`,
      ANALYSIS_WORKER_POLL_INTERVAL: "200ms",
      ANALYSIS_WORKER_ID: "e2e-analysis-worker",
    },
  };
}

children.forEach((child, index) => {
  const label = index === 0 ? service : index === 1 ? "source-processing worker" : "analysis worker";
  child.on("exit", (code, signal) => {
    process.stderr.write(`stack.mjs: ${label} exited (code ${code}, signal ${signal})\n`);
    process.exit(code ?? 1);
  });
});
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => children.forEach((child) => child.kill(signal)));
}
