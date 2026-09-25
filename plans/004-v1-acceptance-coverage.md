# Plan 004: Add real PostgreSQL and V1 acceptance coverage

> **Executor instructions**: Run the drift check first. Add tests and CI gates only after the production contracts from plans 001–003 are stable.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: MED
- **Depends on**: plans/001-enforce-resource-visibility.md, plans/002-atomic-provenance.md, plans/003-source-format-contract.md
- **Category**: tests
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

`make verify` currently runs ordinary Go tests without a database URL, so many integration tests self-skip. The CI database job runs only selected packages, and the web project has no browser E2E runner. This means the plan's declared V1 journeys can be broken while the main verification gate remains green.

## Current state

- `Makefile:23-26` runs `go test ./...` without `DATABASE_URL`; DB tests commonly skip when the environment variable is absent.
- `.github/workflows/ci.yml:62-71` runs migrations/seeds and a subset of Go packages, not the full DB suite or browser journeys.
- `apps/web/package.json:11` declares only `vitest run`.
- `IMPLEMENTATION_PLAN.md:1746-1760` requires E2E coverage for tree creation, publication, forking, collaboration, suggestions, sources, disputes/questions, maps, and research queries.
- Existing integration patterns: `services/core-api/internal/jobs/service_integration_test.go`, `services/core-api/internal/sourceprocessing/processor_integration_test.go`, and `services/core-api/internal/httpapi/router_test.go`.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full DB | `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./... -count=1` | all pass |
| AI eval | `make ai-eval` | exit 0 |
| Browser (after adding runner) | `npx playwright test` | exit 0 |

## Scope

**In scope**:
- `Makefile`
- `.github/workflows/ci.yml`
- `apps/web/package.json` and Playwright config/tests
- New integration/E2E test directories
- Test-only fixtures/seed helpers

**Out of scope**:
- Production feature changes unrelated to making tests deterministic.
- Browser tests against external services or real user data.
- Replacing the existing unit tests wholesale.

## Git workflow

- Branch: `advisor/004-v1-acceptance`.
- Keep the current fast checks fast; add a documented `verify-full`/CI job for DB/E2E.
- Do not push unless instructed.

## Steps

### Step 1: Make database verification explicit

Add a `verify-full` target that starts/reuses the local DB, applies migrations and seed data, exports `DATABASE_URL`, runs the complete Go suite, and runs AI evaluation. Keep `verify` as the fast local gate, but make the README/CI call the full target for acceptance.

**Verify**: `make verify-full` on a clean local DB → migrations, seed, all Go packages, and AI eval exit 0.

### Step 2: Add isolated PostgreSQL integration fixtures

Create a test helper that generates unique users/trees/sources per test and cleans up in `defer`. Cover register/login/session, tree create/publish/fork, invitation accept/revoke, source upload/process/review, claim/evidence/version/audit, question/dispute, and job claiming.

**Verify**: run each new package with `DATABASE_URL=... go test ./internal/auth ./internal/trees ./internal/collaboration ./internal/questions ./internal/evidence ./internal/jobs ./internal/sourceprocessing -count=1` → all pass repeatedly.

### Step 3: Add a minimal browser E2E suite

Add Playwright with a deterministic local stack and seeded synthetic data. Start with: register, create/publish tree, invite collaborator, attach source, create dispute/question, browse map, and run a research query. Use unique accounts per test and explicit cleanup/reset.

**Verify**: `npx playwright test` → all journeys pass against the local API/web/AI services.

### Step 4: Add CI services and caching

Run the database job with the complete Go DB suite, then the browser job with the built web/API/AI services. Cache npm/Go/Python dependencies using lockfiles; do not hide DB test skips.

**Verify**: inspect `.github/workflows/ci.yml`; a deliberate failing DB/E2E test fails CI.

### Step 5: Add quality baselines

Keep `routing_cases.jsonl`, and add small reviewed fixtures for retrieval, extraction, resolution, and contradiction precision. Record metrics in a machine-readable report; do not claim GraphRAG improvement without a vector-only baseline.

**Verify**: `make ai-eval` prints all configured metric groups and exits 0.

## Test plan

- Full DB suite must not silently skip when `DATABASE_URL` is set.
- CI must fail on skipped required packages.
- E2E must cover all ten critical journeys from the plan, even if smoke assertions are shallow.
- Tests must be isolated and rerunnable in any order.

## Done criteria

- [ ] `make verify-full` exists and passes on a clean DB.
- [ ] CI runs the complete DB suite.
- [ ] Playwright covers the declared V1 journeys.
- [ ] AI evaluation includes retrieval/extraction/resolution/contradiction baselines.
- [ ] `make verify` remains a working fast gate.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if browser automation would require a public deployment or external AI credentials.
- Stop if test isolation cannot be achieved without changing production data semantics.
- Stop if a required journey cannot run deterministically with the synthetic seed.

## Maintenance notes

Keep E2E fixtures small and explicit. Reviewers should check that CI does not treat skipped DB tests as success and that new tests reset state rather than depending on test order.
