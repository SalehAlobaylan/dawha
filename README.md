# Dawha

An Arabic lineage research platform: family trees, historical sources, claims with
provenance, disputes, and research that can be traced back to the passage it rests
on. A claim is never a fact here; it is a claim, with its evidence, its status and
its history attached.

- **[PRODUCT.md](PRODUCT.md)** - what the product is, who it is for, and what it
  refuses to do.
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - how the system is put together.
- **[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)** - the intended system, phase
  by phase. It is a plan, not a status report.
- **[docs/phase-status.md](docs/phase-status.md)** - what is actually built and
  proved today, phase by phase, with the remaining blockers. **Read this one to
  find out what the repository does.**
- **[plans/README.md](plans/README.md)** - the remediation plans and what each of
  them changed.

## Prerequisites

| Tool | Why | Notes |
| --- | --- | --- |
| Go 1.25+ | the API, the workers and the migrations runner | `go.mod` declares `go 1.25.0` and `toolchain go1.25.13`; an earlier Go works because toolchain switching is on by default. This checkout was verified on 1.24.5, which switched itself. |
| Node.js 22 or newer | the web app, the E2E suite, the root scripts | CI pins 22; this checkout was verified on 25.6.1 |
| Python 3.12 or newer, with `venv` and a working `pip` | the AI research service | `pyproject.toml` requires >=3.12; CI runs 3.12; this checkout was verified on 3.14.7 |
| Docker with Compose | PostgreSQL 16 + PostGIS + pgvector | `docker compose version` |
| `sqlc` (optional) | only for `make generated-check` and `make sqlc` | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1` |
| Playwright browsers (optional) | only for `make e2e`, installed once per machine | `npx playwright install chromium` |

The first four are required. The last two are prerequisites for one target each and
are not installed by `make install`, because a prerequisite a command installs for
you is a prerequisite nobody checks.

## Install

```sh
make install
```

That is `npm install`, then a Python virtualenv at `services/ai-research/.venv` with
the service installed into it in editable mode.

It needs an interpreter whose `venv` module can bootstrap `pip`, because the second
half is a `pip install -e`. On a machine where it cannot, the failure looks like a
packaging error in this repository rather than a missing `pip` on the interpreter,
and the fix belongs to the interpreter, not to the Makefile.

## Database

```sh
COMPOSE_PROJECT_NAME=dawha make db-migrate
COMPOSE_PROJECT_NAME=dawha make db-seed
```

`COMPOSE_PROJECT_NAME=dawha` reuses the container that is already running. Without
it, `docker compose` derives the project name from the **working directory**, so a
second checkout of this repository starts a second PostgreSQL on port 55432 and one
of them fails to bind. This is the single most confusing thing about a monorepo
with a fixed published port, and it is why the variable appears in every database
command below.

The runner (`infra/local/migrate.sh`) is the only thing in this repository that
applies a schema. It takes an advisory lock, applies each file with its checksum in
one transaction, and refuses to continue if an already-applied file has been edited
since. Point it somewhere else with `POSTGRES_DB`:

```sh
docker exec dawha-db-1 createdb -U dawha -T template_postgis <name>
COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<name> make db-migrate db-seed
```

`make verify-full` migrates and seeds the database it is given and never drops one,
which is why a scratch database is the honest way to verify from clean.

## Run it locally

```sh
make dev
```

Five supervised processes, killed together if any one of them fails:

| Service | URL | Notes |
| --- | --- | --- |
| web app | http://localhost:5173 | Vite dev server, `WEB_ORIGIN` matches |
| core API | http://localhost:8080 | `CORE_API_PORT` |
| AI research | http://localhost:8000 | deterministic, no credentials |
| source-processing worker | none | supervised by the API process |
| analysis worker | none | drains the identity-scan and research-investigation jobs |
| contradiction detector | none | started by `npm run dev` as well |
| PostgreSQL | `localhost:55432` | started by the `predev` hook |

`WEB_ORIGIN` has to match the web origin exactly, or the browser refuses the
credentialed request that carries the session cookie. Both are `localhost` in
development and only the port differs.

### Demo mode

`DEMO_MODE` decides whether the API may serve a static dashboard and a static tree
list at all. It is **off by default in the API** and **on in `npm run dev`** and in
the E2E stack, which set it explicitly. With it off, those two endpoints answer
`503` rather than a workspace nobody created, so a deployment whose API is up but
whose data is not cannot show a reader something that does not exist.

Every demo response is labelled three ways - an `X-Data-Source: demo` header, a
`mode: "demo"` field and a `demo: true` field - and the web app renders a banner
above the numbers. No browser journey asserts on the dashboard; the coverage for
demo mode is in `services/core-api/internal/httpapi/demo_mode_test.go` and
`apps/web/src/lib/api.test.ts`.

## Verifying a change

Two gates, and the difference between them is deliberate.

### `make verify` - the fast gate

No database, no browser, no service startup. Lint, typecheck, unit tests, a build,
and the documentation link check.

```sh
make verify
```

`DATABASE_URL` is deliberately not exported here, so the database-backed tests skip
instead of reaching for a PostgreSQL that may not be running.

### `make verify-full` - the acceptance gate

The whole release signal in one command, against a real database.

```sh
COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<a scratch database> make verify-full
```

In order: `verify`, then the generated-code drift check, `db-migrate`,
`migration-check`, `db-seed`, **the complete Go suite** through
`services/core-api/tools/dbtestguard`, the migration runner's own five cases
against another scratch database, and `make ai-eval`.

`dbtestguard` is what makes that a gate rather than a green tick. It fails on a
non-zero exit, on a `DATABASE_URL` that points at an unmigrated database, on any
test that skipped on the database gate, and on a package in its required manifest
that ran no test at all.

### Every target, and what it proves

| Target | Proves | Needs a database |
| --- | --- | --- |
| `make install` | dependencies are installed; nothing about correctness | no |
| `make dev` | the five processes start together | yes, running |
| `make build` | the web app and the API compile | no |
| `make lint` / `make typecheck` | eslint, `go vet`, ruff, `tsc --noEmit` | no |
| `make test` | web unit tests, the Go suite without a database, pytest | no |
| `make verify` | all of the above, plus `make docs-check` | no |
| `make db-up` / `make db-down` | the container is running or stopped | n/a |
| `make db-migrate` | the schema matches the tree, atomically and checksummed | yes |
| `make db-seed` | `db/seeds/001_demo.sql` applies cleanly | yes |
| `make migration-check` | drift: nothing pending, every recorded checksum equals the file on disk | yes, read-only |
| `make migration-test` | the runner's five cases: success, failure, rerun, checksum, concurrent run | yes, creates and drops its own |
| `make db-verify` | the complete Go suite with the skip audit, without touching migrations | yes |
| `make generated-check` | the committed sqlc output still matches `db/migrations` | no (needs `sqlc`) |
| `make ai-eval` | every AI quality group holds its thresholds; writes a JSON report | no |
| `make graph-benchmark` | what the bounded graph costs in PostgreSQL; writes a JSON report | yes, creates and drops a fixture schema |
| `make docs-check` | every path and line range cited by a document still exists | no |
| `make verify-full` | the acceptance signal, end to end | yes |
| `make security-scan` | dependency advisories and committed secrets; CI gate, needs the network | no |
| `make e2e` | 28 browser journeys against a three-process stack | yes |
| `make e2e-clean` | removes the rows a browser run created, and nothing else | yes |

`make security-scan` and `make e2e` are deliberately not part of `verify-full`:
the scanners need the network and install their own tools, and the browser suite
starts its own stack and has its own CI job. Folding either in would turn "is this
tree correct" into "is this laptop online".

### The browser suite

```sh
COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed
COMPOSE_PROJECT_NAME=dawha make e2e
```

28 journeys across nine files, against the deterministic local stack:

| Service | Port in the E2E stack |
| --- | --- |
| PostgreSQL | 55432 |
| core API | 8181 |
| AI research | 8182 |
| web (`vite preview`, built) | 4173 |

The API and AI ports are deliberately not the development ports, so a busy port is
a loud failure rather than a pass against somebody else's process. The stack runs
with `RATE_LIMIT_ENABLED=false` and `DEMO_MODE=true`, both set explicitly rather than
inherited, because twenty-eight journeys share one client address and the sign-in
budget is thirty a minute. A run removes its own rows on the way out.

`apps/web/e2e/README.md` documents the stack, the ports, what each journey covers
and exactly what the cleanup does and does not delete. Read it before pointing the
suite at a database you care about.

Two prerequisites, because they are the two things that bite:

- **`@playwright/test` must be installed in the working tree.** A checkout that has
  never run `npm install` has no `node_modules`, and `make e2e` fails at
  `npx playwright test` rather than at anything to do with the product.
- **The Playwright browsers must be installed on the machine**, once, with
  `npx playwright install chromium`. `make install` does not do this.

## Environment variables

Every variable this repository reads, by name. **No value belongs in a document**,
and none appears here: the values live in [`.env.example`](.env.example), which is
the file to copy to `.env`.

| Variable | Read by | What it decides |
| --- | --- | --- |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | compose, the migration runner, `make` | which database |
| `DATABASE_URL` | the API, every worker, the Go suite | overrides the URL derived from the three above |
| `COMPOSE_PROJECT_NAME` | every `make` target that touches Docker | which container to reuse |
| `VITE_API_URL` | the web build | the API the browser talks to |
| `WEB_ORIGIN` | the API | the exact origin allowed to send the session cookie |
| `CORE_API_PORT`, `AI_RESEARCH_PORT` | the API, the E2E stack | listen ports |
| `AI_RESEARCH_URL` | the API, the workers | where the AI service is |
| `SOURCE_STORAGE_DIR` | the API, the workers | the local storage driver's directory |
| `SOURCE_WORKER_POLL_INTERVAL`, `ANALYSIS_WORKER_POLL_INTERVAL` | the workers | how often a worker looks for a job |
| `STORAGE_DRIVER` | the API, the workers | `local` or `s3`; there is no fallback in either direction |
| `S3_BUCKET`, `S3_REGION`, `S3_ENDPOINT`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_USE_PATH_STYLE` | the S3 driver | the object store. Names only; a credential value belongs in the deployment, not in this repository |
| `PUBLIC_BASE_URL` | the API | the external address used to build signed object URLs; unset means signed download is unavailable, which is different from storage being unavailable |
| `STORAGE_SIGNING_SECRET` | the local driver | the signing secret. Unset generates one per process, which is right for development and wrong for a deployment |
| `RATE_LIMIT_ENABLED`, `RATE_LIMIT_WINDOW`, `RATE_LIMIT_AUTH_PER_MINUTE`, `RATE_LIMIT_UPLOAD_PER_MINUTE`, `RATE_LIMIT_SUGGESTION_PER_MINUTE`, `RATE_LIMIT_RESEARCH_PER_MINUTE`, `RATE_LIMIT_DEFAULT_PER_MINUTE` | the API | the abuse budgets. Per client address per minute, in-process, not shared between replicas |
| `TELEMETRY_METRICS_ENABLED`, `TELEMETRY_METRICS_ADDR`, `TELEMETRY_SERVICE_NAME` | the API, the workers | the `/metrics` listener. Off by default, and the label set is a fixed enumeration that cannot carry a query, a name or any content |
| `DEMO_MODE` | the API | whether the static dashboard and tree list may be served at all |
| `APP_ENV` | the API | the environment name; `production` is what turns on secure cookies |
| `AI_EVAL_REPORT` | `make ai-eval` | where the quality report is written |
| `DAWHA_GRAPH_BENCH`, `DAWHA_GRAPH_BENCH_ITERATIONS`, `DAWHA_GRAPH_BENCH_REPORT` | `make graph-benchmark` | the graph measurement |
| `DAWHA_MIGRATIONS_DIR` | the Go test fixtures | migrations outside the repository |

`apps/web/e2e/README.md` has the per-service list for the browser stack, and
[`.env.example`](.env.example) has the defaults with the reasoning beside each one.

## Repository layout

```text
apps/web/                 the web app and the Playwright suite
services/core-api/        the API, the workers, the graph and research code
services/ai-research/     the AI service and its evaluation harness
db/migrations/            the schema, one file per version, checksummed
db/seeds/                 the development seed
infra/local/              the migration runner, the link checker
docs/                     the phase status matrix and the graph benchmark record
plans/                    the remediation plans and their status
```

## When something is wrong

| Symptom | Cause | What to do |
| --- | --- | --- |
| `docker compose` starts a second database, or one fails to bind 55432 | the compose project name is derived from the working directory | pass `COMPOSE_PROJECT_NAME=dawha` |
| `make install` fails inside the venv | the interpreter's `venv` could not bootstrap `pip` | give that interpreter a working `pip`, or use one that has it, then re-run. The failure is the interpreter's, not the Makefile's. |
| `make e2e` fails at `npx playwright test` | no `node_modules` in this working tree | `npm install` here, not in another checkout |
| `make e2e` fails to launch a browser | the Playwright browsers are not installed on this machine | `npx playwright install chromium` |
| `make generated-check` exits 2 | `sqlc` is not installed | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1` |
| `make security-scan` exits 2 | `gitleaks` is not installed | `brew install gitleaks` |
| `make verify` says a database-backed test skipped | `DATABASE_URL` is unset, which is correct for the fast gate | use `make verify-full` when you meant acceptance |
| `make db-verify` exits 2 | `DATABASE_URL` is unset | it refuses to run, because a suite that silently skips is what it exists to catch |
| the browser is refused the session cookie | `WEB_ORIGIN` does not match the web origin exactly | make them the same origin, port included |
| a Go test fails on a "fixture schema survived the run" audit | a test did not clean up | the audit is right; the test is the bug |

## What this repository does not claim

- It is not a historical authority. `PRODUCT.md` says what the product will not
  decide, and the code enforces it: every response that carries a judgement -
  classification, routing, extraction, resolution, contradiction, research answer -
  carries `review_required: true` as a literal type, and graph results are
  structural rather than interpretive.
- The quality numbers in `make ai-eval` are agreement with small reviewed fixture
  sets, not accuracy on real questions, and there is no vector-only baseline here,
  so no retrieval improvement is claimed.
- The graph benchmark in [docs/graph-benchmark.md](docs/graph-benchmark.md) is one
  machine, one container, synthetic graphs and single-threaded requests. It closes
  no decision; it records what was measured.
