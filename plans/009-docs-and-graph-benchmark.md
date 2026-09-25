# Plan 009: Document current state and benchmark graph limits

> **Executor instructions**: Produce evidence and documentation only; do not introduce Neo4j or another graph database as part of this plan.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: plans/004-v1-acceptance-coverage.md, plans/006-async-jobs.md
- **Category**: docs/direction
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

The repository has no root README or phase status matrix, so the aspirational plan and the shipped implementation are easy to confuse. The plan also requires measurement before reopening the Neo4j decision gate. Recent GraphRAG/source-dependency work now provides a concrete workload for measuring PostgreSQL limits instead of guessing.

## Current state

- `IMPLEMENTATION_PLAN.md:80-115` expects a root `README.md`; no root README is present.
- `IMPLEMENTATION_PLAN.md:119-1617` defines Phases 0–24 but has no implemented/verified status markers.
- `IMPLEMENTATION_PLAN.md:1619-1646` says Neo4j is measurement-driven and PostgreSQL remains authoritative.
- `ARCHITECTURE.md:605-632` describes Neo4j as a future derived projection.
- `graphrag.go:37-42` bounds graph operations at depth 3, 200 nodes, and 200 edges; the community operation at `graph_source_dependency_communities.go:215-270` repeatedly processes the bounded edge set.
- `services/ai-research/evaluation/routing_cases.jsonl` currently covers routing labels only.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Docs links | `python3 -c 'from pathlib import Path; ...'` or repository link checker | no broken local links |
| Fast | `make verify` | exit 0 |
| Full | `make verify-full` | exit 0 |
| Benchmark | documented benchmark command | report generated |

## Scope

**In scope**:
- New root `README.md`
- `IMPLEMENTATION_PLAN.md` status appendix or linked status matrix
- `docs/` benchmark/decision record if the repository convention permits
- `services/ai-research/evaluation/` fixtures/metrics
- `services/core-api/internal/research/` benchmark helpers or tests

**Out of scope**:
- Neo4j/Redis/OpenSearch/Temporal introduction.
- Changing production graph algorithms solely for benchmark results.
- Publishing external URLs or claims about production performance without evidence.

## Git workflow

- Branch: `advisor/009-docs-and-graph-benchmark`.
- Commit documentation and benchmark harness separately.
- Do not push unless instructed.

## Steps

### Step 1: Write the root README

Document prerequisites, environment variables by name (never secret values), `make install`, `make db-migrate`, `make db-seed`, `make verify`, `make verify-full`, local service URLs, demo mode, and troubleshooting. Link the product, architecture, and implementation plan documents.

**Verify**: all commands in the README are runnable or explicitly labeled as prerequisites; no broken relative links.

### Step 2: Add a phase status matrix

For each Phase 0–24, record `implemented`, `partial`, or `not started`, link representative code/tests, and state the remaining acceptance gap. Distinguish “code slice exists” from “V1 acceptance verified.” Include the V1 18-step definition from `IMPLEMENTATION_PLAN.md:1964-1992`.

**Verify**: a reviewer can identify the first three remaining blockers without reading commit history.

### Step 3: Build a graph benchmark spike

Create synthetic public-source dependency graphs at, below, and above the 200-node/200-edge bounds. Measure p50/p95 latency, truncation rate, query plan, memory/allocation, and result stability. Keep PostgreSQL authoritative and record results in a decision document.

**Verify**: benchmark runs repeatably and records a table; no Neo4j dependency is added.

### Step 4: Expand AI evaluation fixtures

Add small expert-reviewed Arabic fixtures for extraction, entity resolution, contradiction classification, retrieval hit rate, citation grounding, and a vector-only versus graph-augmented baseline. Keep routing fixtures and add versioned thresholds/report output.

**Verify**: `make ai-eval` reports every metric group and fails when a threshold is exceeded; existing routing metrics remain.

## Test plan

- README command smoke checks.
- Link/format validation.
- Benchmark reproducibility and bounded-output assertions.
- AI fixture schema/threshold tests.

## Done criteria

- [ ] Root README explains setup, environment, verification, and demo mode.
- [ ] Phase status matrix links code/tests and identifies remaining gaps.
- [ ] Graph benchmark report exists with p50/p95 and truncation data.
- [ ] AI evaluation covers retrieval/extraction/resolution/contradiction beyond routing.
- [ ] `make verify` and `make verify-full` pass.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if benchmark data would require production-like secrets or personal data.
- Stop if the status matrix would claim completion without code/test evidence.
- Stop if the AI evaluation would present a routing label as historical confidence.

## Maintenance notes

Update the status matrix whenever a phase acceptance criterion changes. Keep the Neo4j decision tied to measurements, not dates or intuition.
