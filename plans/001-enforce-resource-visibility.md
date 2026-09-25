# Plan 001: Enforce resource visibility and research authorization

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before continuing. If a STOP condition occurs, stop and report; do not improvise. Update the status row in `plans/README.md` when complete.
>
> **Drift check (run first)**: `git diff --stat a6397ae..HEAD -- services/core-api/internal/dictionary services/core-api/internal/search services/core-api/internal/evidence services/core-api/internal/research services/core-api/internal/httpapi/router.go db/migrations` and compare the excerpts below before editing.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: HIGH
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

The public dictionary, search, claim, and research-history routes do not consistently apply the same resource visibility rules used by graph retrieval. A person inserted while building a private draft is stored in the global `people` table, and public dictionary/search queries can return that row without checking tree or research authorization. Separately, `GetRun` and `ListRuns` return run metadata to callers who are not authorized to view research history. This violates the Phase 1 authorization criterion and the privacy requirements in `PRODUCT.md:1383-1415`.

## Current state

- `services/core-api/internal/trees/service.go:169-180` inserts every new tree person into global `people`; the schema at `db/migrations/0002_identity_and_trees.sql:24-38` has no person visibility column.
- `services/core-api/internal/dictionary/service.go:239-276` reads person notes, aliases, claims, sources, and questions without a tree/source visibility predicate.
- `services/core-api/internal/search/service.go:170-199` ranks `people`, families, tribes, branches, and places without a public-membership predicate.
- `services/core-api/internal/evidence/service.go:409-466` lists and fetches claims with no actor/resource visibility filter.
- `services/core-api/internal/research/history.go:83-108` only calls `canAccessPersistedRun` when `includeAnswer` is true; the summary query still returns query text, status, model, route, counts, and timestamps.
- `services/core-api/internal/httpapi/research_handler.go:202-237` treats auth as optional for run listing/retrieval.

The existing graph/history code already demonstrates the desired all-or-nothing source filtering in `services/core-api/internal/research/history.go:250-430`: collect identifiers, then deny the run if any source is no longer publicly visible. Reuse that policy style rather than adding endpoint-specific ad-hoc checks.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Drift | `git diff --stat a6397ae..HEAD -- <in-scope paths>` | No unexplained in-scope drift |
| Fast checks | `make verify` | exit 0 |
| DB tests | `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./... -count=1` | all packages pass |
| Lint | `cd services/core-api && go vet ./...` | exit 0 |
| Frontend | `npm run lint && npm run typecheck` | exit 0 |

## Scope

**In scope**:
- `services/core-api/internal/dictionary/`
- `services/core-api/internal/search/`
- `services/core-api/internal/evidence/`
- `services/core-api/internal/research/history.go`
- `services/core-api/internal/httpapi/research_handler.go`
- `services/core-api/internal/httpapi/router.go`
- A new migration under `db/migrations/` if a visibility/index column is required
- Focused tests for these packages

**Out of scope**:
- Changing the public API response shapes without a compatibility plan.
- Removing the deliberately public `source visibility` policy from source/dependency graph features.
- Neo4j, Redis, OpenSearch, or Temporal changes.
- Broad identity CRUD (plan 005).

## Git workflow

- Branch: `advisor/001-resource-visibility` unless the operator specifies another branch.
- Commit logical units with messages matching existing style, e.g. `Add resource visibility policy`.
- Do not push or open a PR unless instructed.

## Steps

### Step 1: Define one central visibility policy

Create an internal authorization helper (prefer `services/core-api/internal/auth` or a focused `visibility` package) that answers whether an actor may read a person, claim, source, question, and published-tree context. It must distinguish:

- public published data;
- data owned by or collaborated on by the actor;
- research-only data visible to authorized roles;
- missing/deleted records.

Reuse existing permission helpers in `services/core-api/internal/auth/permissions.go` and source checks in `services/core-api/internal/evidence/dependencies.go:329-372`. Do not grant a role blanket access to every global person row.

**Verify**: add table-driven unit tests for anonymous, owner, collaborator, researcher, unrelated researcher, and admin cases; run `go test ./internal/auth ./internal/evidence -count=1` → all pass.

### Step 2: Scope dictionary and search queries

Make public dictionary and search queries join through authorized/published tree or source context. A person with only a private-draft `tree_nodes` row must not appear in anonymous dictionary/search results. A person referenced by a published public version may appear, subject to the privacy policy for living status.

Avoid adding a blanket `people.visibility` column without a migration/backfill policy; if a column is required, write a forward migration with a safe default and document the backfill query.

**Verify**: add DB tests that create a private draft person and a published person, then assert anonymous dictionary/search results include only the published record. Run `DATABASE_URL=... go test ./internal/dictionary ./internal/search -count=1` → all pass.

### Step 3: Filter claim reads and writes consistently

Apply the same policy to `ListClaims`, `GetClaim`, claim evidence/counter-evidence, and mutation handlers. Preserve the existing distinction between source statements, research claims, tree interpretations, and platform findings.

**Verify**: add anonymous/owner/unauthorized tests for claim list and direct claim ID. Run `go test ./internal/evidence ./internal/httpapi -count=1` → all pass.

### Step 4: Protect research run metadata

Decide and document the public contract for run summaries. Recommended default: anonymous callers receive only an existence-neutral 401/403/404; authorized researchers receive the full summary. Apply the check before loading the summary, not only when `includeAnswer` is true. Keep error responses free of query text and internal errors.

**Verify**: add tests for anonymous `ListRuns`/`GetRun`, unrelated researcher, authorized researcher, and missing run. Run `go test ./internal/research ./internal/httpapi -count=1` → all pass.

### Step 5: Run full verification

Run `make verify`, DB-backed `go test ./...`, and `git diff --check`. Confirm no response contract or private data appears in test logs.

**Verify**: all commands exit 0; `git status --short` shows only plan 001 files.

## Test plan

- Authorization matrix for public/private tree, source, claim, and research-run reads.
- Anonymous direct-ID tests for dictionary, search, claims, and runs.
- Regression tests proving public published records remain visible.
- Tests proving denied records do not leak titles, notes, IDs, counts, or existence.

## Done criteria

- [ ] Central visibility policy exists and is used by dictionary, search, claims, and run history.
- [ ] Anonymous callers cannot retrieve private research-run metadata.
- [ ] Private-draft people/claims/sources are absent from anonymous public queries.
- [ ] `make verify` passes.
- [ ] Full DB `go test ./...` passes.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` status updated.

## STOP conditions

- Stop if existing public routes intentionally depend on unscoped global `people` rows; report the compatibility decision instead of silently changing it.
- Stop if a new visibility column would require rewriting existing published-tree semantics.
- Stop if a verification command fails twice.

## Maintenance notes

Any new entity added to search or dictionary pages must declare its public-membership rule. Reviewers should scrutinize anonymous direct-ID paths, not only list endpoints, and verify that error bodies do not become an existence oracle.
