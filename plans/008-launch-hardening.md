# Plan 008: Harden migrations, verification, storage, security, and telemetry

> **Executor instructions**: Treat this as a launch-readiness plan. Do not weaken local development defaults to make production gates pass.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: MED
- **Depends on**: plans/004-v1-acceptance-coverage.md
- **Category**: tech-debt/security/dx
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

The plan's public-launch milestones require migration safety, dependency/secret scanning, signed object access, rate limits, abuse controls, and observability. The repository currently has a permissive migration runner, local-only storage with no signed URLs, a telemetry package that is only a logger, and no CI dependency/secret scan. `make verify` also does not include the full DB or AI acceptance gates. These gaps make a green local run an incomplete release signal.

## Current state

- `infra/local/migrate.sh:32-41` applies each file and records its version in separate `psql` calls, with no advisory lock, transaction wrapper, or checksum.
- `Makefile:23-46` has `verify` but no full DB/AI/migration-drift gate.
- `services/core-api/platform/storage/local.go:96-98` returns `ErrReadOnly` for `SignedURL`; API and worker construct local stores independently.
- `services/core-api/platform/telemetry/telemetry.go:1-20` is a JSON `slog` logger, and `router.go:276-284` only returns a request ID header.
- `router.go:228` applies only CORS and request IDs; no rate-limit or abuse middleware.
- `.github/workflows/ci.yml:20-71` has no dependency audit, secret scan, or `govulncheck` step.
- `PRODUCT.md:1370-1415` and `IMPLEMENTATION_PLAN.md:1813-1827` require these controls before broad public launch.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Full | `make verify-full` | exit 0 |
| Migrations | `make db-migrate` on a clean DB | exit 0 |
| Go | `cd services/core-api && go test ./... && go vet ./...` | exit 0 |
| Python | `cd services/ai-research && .venv/bin/pytest && .venv/bin/ruff check .` | exit 0 |
| Web | `npm run lint && npm run typecheck && npm run test && npm run build` | exit 0 |

## Scope

**In scope**:
- `infra/local/migrate.sh` and migration tooling
- `Makefile` and `.github/workflows/ci.yml`
- `services/core-api/platform/storage/`
- `services/core-api/platform/telemetry/`
- `services/core-api/internal/httpapi/router.go` and rate-limit middleware
- Dependency/secret scanning configuration
- Environment documentation for storage/telemetry credentials

**Out of scope**:
- Introducing Redis, OpenSearch, Kafka, Kubernetes, or Temporal.
- Storing or logging secret values; only reference credential types/locations.
- Product-specific AI model changes.

## Git workflow

- Branch: `advisor/008-launch-hardening`.
- Commit migration safety, verification, storage, then security/telemetry separately.
- Do not push unless instructed.

## Steps

### Step 1: Make migrations atomic and verifiable

Acquire a PostgreSQL advisory lock. Execute each migration and its version/checksum record in one transaction. Reject edits to already-applied migrations via checksum mismatch. Add a CI check for a deliberately failing migration and concurrent runner behavior.

**Verify**: `make db-migrate` twice on the same DB → second run is a no-op; a failing migration leaves no partial schema/version marker. Run `git diff --check`.

### Step 2: Define a complete verification gate

Add `verify-fast`/`verify-full` (or equivalent) targets. `verify-full` must provision/reset an isolated DB, migrate/seed, run all Go DB tests, AI evaluation, web tests/build, migration drift, and generated-code drift. Keep a documented fast target for local iteration.

**Verify**: run the full target from a clean checkout → all steps exit 0; a deliberately failing DB test makes the target fail.

### Step 3: Add shared object storage with signed access

Define a storage interface, keep `LocalStore` as an explicit development adapter, and add an S3-compatible implementation. Inject the same configured store into API and workers. Add authorization-checked signed download routes/URLs; never expose raw storage keys.

**Verify**: contract tests run against both local and S3-compatible test doubles; unauthorized users cannot obtain a signed URL. Run `go test ./platform/storage ./internal/httpapi -count=1` → all pass.

### Step 4: Add abuse and dependency controls

Add configurable rate limits for auth, suggestions, uploads, and research mutations. Add content validation/quarantine hooks for uploads. Add `npm audit`, `govulncheck`, Python dependency auditing, and repository secret scanning to CI, with reviewed high/critical failure policy. Do not paste secret values into logs or plans.

**Verify**: tests exercise allowed and throttled requests; CI fails on a fixture advisory/secret in a test branch; no secret values appear in output.

### Step 5: Add minimal observability

Bind request IDs to context/log fields and add metrics for request latency/errors, DB latency, queue depth/failures, AI latency/cost, and research outcome counts. Use an OpenTelemetry-compatible local exporter or Prometheus-compatible endpoint. Redact query/source/person content from labels.

**Verify**: a local request/job/AI run emits correlated IDs and the documented metrics; `go test ./platform/telemetry ./internal/httpapi -count=1` passes.

### Step 6: Make demo fallback explicit

Gate static dashboard/demo responses behind an explicit `DEMO_MODE` setting and return dependency errors otherwise. Update the frontend so demo mode is visibly labeled and never silently substituted for a real outage.

**Verify**: with DB unavailable and `DEMO_MODE=false`, API returns an error; with `DEMO_MODE=true`, demo response is explicit. Existing demo tests are updated accordingly.

## Test plan

- Migration success, failure, rerun, checksum, and concurrent-run cases.
- Storage local/S3 contract and signed-URL authorization.
- Rate-limit and upload validation cases.
- Secret/dependency scanner fixtures.
- Request/job/AI telemetry correlation and redaction.
- Explicit demo-mode behavior.

## Done criteria

- [ ] Migrations are atomic, locked, and checksum-protected.
- [ ] Full verification is a documented/CI-enforced gate.
- [ ] Production storage/signed access has a tested implementation.
- [ ] Rate/abuse, dependency, and secret controls run in CI.
- [ ] Request/job/AI metrics and request IDs are observable without content leakage.
- [ ] Demo mode cannot mask production outages.
- [ ] `make verify-full` passes and `git diff --check` is clean.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if signed storage requires an unapproved cloud provider or credential-handling policy.
- Stop if rate limits would break the documented local demo workflow; add an explicit development override instead.
- Stop if telemetry would persist sensitive source/person content.
- Stop if a verification gate fails twice.

## Maintenance notes

Keep migration checksums and generated-code drift in CI. Treat demo data as an explicit adapter, never as an error fallback. Reviewers should inspect secret handling and telemetry labels before approving.
