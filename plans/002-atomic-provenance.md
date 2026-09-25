# Plan 002: Make provenance mutations and suggestion application atomic

> **Executor instructions**: Follow each step and verification gate. Do not edit files outside Scope. Stop on any STOP condition and report.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: plans/001-enforce-resource-visibility.md
- **Category**: bug
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

The evidence model promises that claims, statements, evidence links, versions, and audit events are a coherent history. Several mutations currently write the primary record and the audit/version record through separate pool calls. A failure after the first write can return an error while leaving a partially recorded mutation. The Phase 8 suggestion workflow has a related gap: an accepted suggestion only records a review and does not apply a reviewed domain change.

## Current state

- `services/core-api/internal/evidence/service.go:395-406` inserts a source statement with `Pool.Exec`, then calls `writeAudit` separately.
- `services/core-api/internal/evidence/service.go:492-509` inserts a claim, claim version, and audit event through independent writes.
- `services/core-api/internal/evidence/service.go:562-576` has the same pattern for evidence links.
- `services/core-api/internal/questions/service.go:350-357` persists a question update before its audit write.
- `services/core-api/internal/suggestions/service.go:224-250` creates a question only for the `converted` decision; `accepted` records a review/status only.
- `IMPLEMENTATION_PLAN.md:833-840` requires an accepted suggestion to be able to create auditable domain changes.
- Existing transactional exemplars are `services/core-api/internal/collaboration/service.go` and the transaction used by `services/core-api/internal/trees/service.go:141-196`.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast verification | `make verify` | exit 0 |
| DB tests | `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./... -count=1` | all pass |
| Focused | `go test ./internal/evidence ./internal/questions ./internal/suggestions -count=1` | all pass |
| Lint | `cd services/core-api && go vet ./...` | exit 0 |

## Scope

**In scope**:
- `services/core-api/internal/evidence/service.go`
- `services/core-api/internal/questions/service.go`
- `services/core-api/internal/suggestions/service.go`
- Focused tests in those packages
- A migration only if a typed suggestion change-set table is required

**Out of scope**:
- AI-generated automatic mutations.
- Changing the meaning of existing claim/source statuses.
- Broad UI redesign.
- Plan 001's public visibility policy implementation.

## Git workflow

- Branch: `advisor/002-atomic-provenance`.
- Commit per logical mutation family; do not push unless instructed.

## Steps

### Step 1: Wrap provenance mutations in transactions

Refactor statement, claim, evidence-link, and question-update mutations to use one `pgx.Tx` for the primary record, version/history rows, and audit event. Preserve the current validation and authorization order. Use the existing audit helper but pass the transaction executor rather than the pool where its signature allows it.

Handle `ErrNoRows`/permission failures before writing, and ensure deferred rollback remains harmless after commit.

**Verify**: add fault-injection tests or a test transaction that forces the second write to fail, then assert no primary row/version/audit partial state. Run `go test ./internal/evidence ./internal/questions -count=1` → all pass.

### Step 2: Define typed accepted-suggestion changes

Do not apply free-form Arabic text directly. Add a reviewable change-set shape containing an explicit target (`person`, `relationship`, `claim`, or `source_link`) and typed IDs/fields, or explicitly make acceptance create a claim/open question with an audit link. Reject malformed change sets with validation errors.

**Verify**: add tests for accepted-without-change-set, accepted-with-valid-change-set, rejected, and converted decisions. Run `go test ./internal/suggestions -count=1` → all pass.

### Step 3: Apply accepted changes transactionally

Within the same transaction as the suggestion review, apply the approved domain mutation using existing tree/claim services or repository helpers, then write the audit event linking original text, reviewer, change, and resulting record. Do not publish a tree or accept a claim automatically; the plan requires explicit authorized action.

**Verify**: add an integration test proving the accepted suggestion updates only the intended draft/claim and leaves an audit link. Run the focused suite → all pass.

### Step 4: Add rollback regression tests

Cover a failing audit insert, failing version insert, and a duplicate/invalid change-set. Assert the original suggestion remains pending and no partial domain object exists.

**Verify**: `DATABASE_URL=... go test ./internal/evidence ./internal/questions ./internal/suggestions -count=1` → all pass.

## Test plan

- Statement creation rollback when audit fails.
- Claim + version + audit atomicity.
- Evidence link + audit atomicity.
- Question update + audit atomicity.
- Suggestion accept/reject/convert semantics and audit linkage.
- Authorization tests remain intact.

## Done criteria

- [ ] Every provenance-bearing mutation uses one transaction.
- [ ] Accepted suggestions apply a typed, auditable change or a documented claim/question artifact.
- [ ] No automatic publish/claim-acceptance path is introduced.
- [ ] Focused and full DB tests pass.
- [ ] `make verify` passes.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if existing services cannot accept a transaction executor without a broad public interface refactor; report the smallest safe seam.
- Stop if the required change-set cannot be made explicit without inventing historical truth.
- Stop if a verification gate fails twice.

## Maintenance notes

New mutation methods must document which rows are part of the atomic history boundary. Reviewers should inspect audit failure tests and the suggestion change-set schema before approving.
