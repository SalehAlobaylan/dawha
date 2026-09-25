# Plan 006: Add job lease fencing and asynchronous long analyses

> **Executor instructions**: Preserve PostgreSQL-backed jobs; do not introduce Temporal, Redis, or a second queue. Complete each verification gate before moving on.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: MED
- **Depends on**: plans/003-source-format-contract.md, plans/004-v1-acceptance-coverage.md
- **Category**: bug/perf
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

Source processing can exceed the worker's 15-minute stale-recovery window, allowing a second worker to reclaim and reprocess the same job. Entity resolution and research-agent investigations currently execute all stages inside HTTP-triggered calls, so slow AI/database work can outlive the request deadline and hold request resources. The plan already provides the right queue substrate; this plan hardens it instead of adding infrastructure.

## Current state

- `services/core-api/internal/jobs/service.go:285-307` requeues stale running jobs after 15 minutes without a lease token or heartbeat.
- `services/core-api/cmd/source-processing/main.go:40-70` processes one job to completion and never renews `locked_at`.
- `services/core-api/internal/sourceprocessing/processor.go:161-228` performs sequential page embedding/extraction work.
- `services/core-api/internal/entityresolution/service.go:37-103` loads, scores, and persists synchronously from the HTTP-triggered `Run` method.
- `services/core-api/internal/researchagent/service.go:60-127` executes all planned stages inside one transaction and request.
- Existing `jobs` schema and `FOR UPDATE SKIP LOCKED` claim pattern are in `db/migrations/0003_research_and_jobs.sql:233-249` and `jobs/service.go:184-213`.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Focused | `go test ./internal/jobs ./internal/sourceprocessing ./internal/entityresolution ./internal/researchagent -count=1` | all pass |
| Full | `make verify` | exit 0 |
| DB | `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./... -count=1` | all pass |

## Scope

**In scope**:
- `services/core-api/internal/jobs/`
- `services/core-api/cmd/source-processing/`
- `services/core-api/internal/sourceprocessing/`
- `services/core-api/internal/entityresolution/`
- `services/core-api/internal/researchagent/`
- A migration for lease tokens/heartbeat fields if required
- Worker/integration tests and CI job wiring

**Out of scope**:
- Temporal, Redis, Kafka, or a new queue system.
- Changing AI provider selection.
- Broad GraphRAG algorithm work.

## Git workflow

- Branch: `advisor/006-async-jobs`.
- Commit lease fencing, then heartbeat, then async run migration separately.
- Do not push unless instructed.

## Steps

### Step 1: Add owner-checked lease renewal and fencing

Add a claim token or versioned lease owner to the job record. Provide `Renew(ctx, jobID, workerID)` that only succeeds for the current running owner. Make `Complete`, `Fail`, and any processing persistence reject stale owners, not merely compare the worker string.

**Verify**: add tests for renewal by owner, rejection after stale recovery, and stale-worker completion. Run `go test ./internal/jobs -count=1` → all pass.

### Step 2: Heartbeat long source-processing stages

Start a renewal loop or renew between pages/stages. Ensure the processor observes lease loss and stops before committing results. Keep idempotency keys and candidate uniqueness intact.

**Verify**: add a test with a controlled slow stage/lease expiry; assert no duplicate accepted records and a clear failed/queued status. Run `go test ./internal/sourceprocessing ./internal/jobs -count=1` → all pass.

### Step 3: Move entity resolution to a queued run

Split `Run` into a short transaction that creates a run and enqueues work, then a worker stage that performs snapshot/candidate generation with checkpointed progress. Return a run ID/accepted status to HTTP. Preserve role checks, failed-run semantics, and current candidate/review APIs.

**Verify**: API test returns quickly with a queued run; worker integration test completes it; existing review/merge tests pass. Run `go test ./internal/entityresolution ./internal/httpapi -count=1` → all pass.

### Step 4: Move research-agent stages to checkpointed jobs

Persist each stage result and evidence reference so a worker can resume after failure. Keep the existing permission restrictions and unresolved-state semantics. Do not publish trees, accept claims, merge people, or modify source evidence automatically.

**Verify**: integration test runs all stages, resumes after an injected stage failure, and preserves traceable steps/citations. Run `go test ./internal/researchagent -count=1` → all pass.

### Step 5: Run full queue/CI verification

Run the full DB suite and add a CI job that exercises a real claim/renew/complete cycle.

**Verify**: `make verify` and DB `go test ./... -count=1` pass; no job test skips when `DATABASE_URL` is set.

## Test plan

- Lease owner/heartbeat/fencing matrix.
- Stale recovery while a worker is still active.
- Duplicate delivery and idempotent processing.
- Entity-resolution queued lifecycle and failure/resume.
- Research-agent stage checkpoint/resume and permission restrictions.
- Worker shutdown/context cancellation.

## Done criteria

- [ ] Long jobs renew leases and stale workers cannot commit.
- [ ] Entity resolution and research agent no longer execute all work in the request thread.
- [ ] Existing job API remains backward compatible.
- [ ] Full verification and DB tests pass.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if lease fencing requires a database-wide job semantic change not covered by the plan.
- Stop if asynchronous execution would expose partial candidate/agent results as succeeded.
- Stop if a verification gate fails twice.

## Maintenance notes

Any new worker must renew its lease during every external AI call and use owner-checked completion. Reviewers should test duplicate delivery, cancellation, and stale-worker behavior before approving throughput changes.
