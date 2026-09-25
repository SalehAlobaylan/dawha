# Plan 007: Batch list hydration and bound map queries

> **Executor instructions**: Optimize without weakening visibility, provenance, or deterministic ordering. Measure before and after.

## Status

- **Priority**: P2
- **Effort**: L
- **Risk**: MED
- **Depends on**: plans/001-enforce-resource-visibility.md, plans/002-atomic-provenance.md
- **Category**: perf
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

Several high-volume endpoints hydrate related records one row at a time, and the map endpoint loads complete feature tables before applying filters. The current caps keep some operations bounded, but the database round-trip count and payload size still grow with the dataset. This undermines the plan's bounded/performant research surfaces and can exhaust the small connection pool under concurrent use.

## Current state

- `services/core-api/internal/sourceprocessing/review.go:262-289` loads up to `MaxCandidates` (`types.go:21` = 5000) and calls `candidateReviews` once per candidate.
- `services/core-api/internal/research/history.go:173-317` performs repeated tree/source/graph authorization queries for each of up to 100 run summaries.
- `services/core-api/internal/geography/service.go:76-109` loads places, associations, migrations, and regions completely, then filters in Go.
- `services/core-api/internal/research/retrieval.go:14-51` scores lexical candidates before applying the final limit.
- `services/core-api/internal/search/service.go:170-199` performs similarity ranking across full entity scans.
- Existing pagination/limit conventions are in source-processing and research list services; do not remove them.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Focused | `go test ./internal/sourceprocessing ./internal/research ./internal/geography ./internal/search -count=1` | all pass |
| Full | `make verify` | exit 0 |
| DB | `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./... -count=1` | all pass |

## Scope

**In scope**:
- The four services above and their focused tests
- Bounded SQL queries/CTEs and pagination
- Index migrations where query plans justify them

**Out of scope**:
- Replacing pgvector or PostGIS.
- OpenSearch/Redis introduction.
- Changing ranking semantics without relevance fixtures.

## Git workflow

- Branch: `advisor/007-batch-hydration`.
- Commit each service optimization separately with before/after query-count notes.
- Do not push unless instructed.

## Steps

### Step 1: Batch source-candidate review hydration

Replace the per-candidate review query with one `WHERE candidate_id = ANY($1)` query grouped by candidate. Return paginated candidate summaries by default; fetch full passage text only when a candidate is opened/expanded.

**Verify**: add a test that counts/validates review grouping for multiple candidates and preserves ordering. Run focused source-processing tests → all pass.

### Step 2: Batch research-run authorization

Collect all run IDs and source/tree identifiers first, then use set-based queries with `ANY`/joins. Preserve the all-or-nothing visibility rule and per-run redaction from plans 001–002. Do not expose IDs for unauthorized runs in list responses.

**Verify**: add a 100-run authorization test with mixed public/private cases and assert no extra per-run queries in a query-counting test or benchmark. Run `go test ./internal/research -count=1` → all pass.

### Step 3: Push map filters into SQL

Apply status, place, time, and viewport/bounds predicates to each feature query. Add a bounded response limit and deterministic ordering. Keep inferred/documented/disputed layers distinct as required by `IMPLEMENTATION_PLAN.md:1539-1573`.

**Verify**: add tests that a time/status/place filter excludes records before response and that limits are reported/truncated. Run `go test ./internal/geography ./internal/httpapi -count=1` → all pass.

### Step 4: Add indexable lexical candidate stages

Use trigram/full-text/ILIKE candidate predicates to bound the set before expensive similarity/score computation. Validate with representative `EXPLAIN (ANALYZE, BUFFERS)` data before finalizing indexes; do not rely on a production-sized dataset that does not exist yet.

**Verify**: add relevance regression tests for exact, alias, fuzzy, and semantic results. Run `go test ./internal/search ./internal/research -count=1` → all pass.

## Test plan

- Query-count/payload regression tests or repeatable benchmarks.
- Ordering/pagination stability across pages.
- Visibility/redaction regression for mixed public/private records.
- Map status/time/viewport filtering and truncation.
- Lexical relevance fixtures for Arabic names, aliases, and passages.

## Done criteria

- [ ] No per-candidate/per-run N+1 hydration remains in the targeted paths.
- [ ] Map filters execute in SQL and responses are bounded.
- [ ] Ranking semantics remain covered by tests.
- [ ] `make verify` and DB tests pass.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if a proposed index cannot be justified with an `EXPLAIN` plan on a representative local dataset.
- Stop if batching would require exposing private records or changing deterministic ordering.
- Stop if verification fails twice.

## Maintenance notes

Record query counts and representative payload sizes in the PR. Any new list endpoint should use one bounded main query plus set-based hydration, not per-row lookups.
