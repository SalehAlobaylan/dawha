# Plan 017: Make the source-dependency measurements stable and bounded

> **Executor instructions**: Two recorded findings from the graph benchmark, both about the same code path. Fix the instability first — a measurement that changes between identical runs is not a measurement — then decide the allocation question on evidence.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED
- **Depends on**: plans 007, 009, 014
- **Category**: perf/correctness
- **Planned at**: commit `0ae4171`, 2026-09-27

## Why this matters

`docs/graph-benchmark.md` is the evidence base for the Neo4j decision gate, and it recorded two
things about the source-dependency graph that weaken it. A neighbourhood fingerprint is stable
within one database but not across two equivalent ones, because the query orders candidate edges
by a random uuid — so the same corpus measured twice on different machines gives different
fingerprints. And community detection allocates 153 MiB per call on a 200-node star, which is a
latency and memory profile that will not survive a concurrent user.

Plan 014 already established the principle this plan applies: a result whose order is emergent
is not a result. A fingerprint whose identity depends on a random uuid is the same defect one
layer up.

## Current state

- `services/core-api/internal/research/graph_source_dependency_neighborhood.go` — the neighbourhood
  operation; its fingerprint is derived from a result order that includes `source_dependencies.id`,
  a `gen_random_uuid()` default.
- `services/core-api/internal/research/graph_source_dependency_communities.go:216-271` — community
  detection over the bounded edge set; measured at 156,982 KiB per call and 82,307 allocations at
  the 200-node/200-edge bound, against 11,271 KiB above it.
- `docs/graph-benchmark.md` — the recorded p50/p95, allocations, and plan buffers, with the
  variance stated and both findings recorded.
- `docs/benchmarks/graph-source-dependency.json` — the committed run.
- Plan 014 added the tie-break lesson to the retrieval path: an explicit final key makes an order
  total, and a test now pins it.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Bench | `DAWHA_GRAPH_BENCH=1 go test ./internal/research -run Graph -count=1 -v` | records a run |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |

## Scope

**In scope**:
- `services/core-api/internal/research/` — the two source-dependency operations and their tests
- `db/migrations/` — only if an index is justified by an `EXPLAIN` on a representative corpus
- `docs/graph-benchmark.md` and `docs/benchmarks/graph-source-dependency.json` — the re-recorded run

**Out of scope**:
- Adding a graph database, a cache, or any new dependency. The Neo4j decision gate stays closed
  and is not reopened by this plan.
- Changing which neighbours or communities are returned, the 200-node/200-edge bounds, or any
  other operation's semantics.
- Tuning for speed at the cost of an answer. A cheaper wrong answer is not an improvement.

## Steps

### Step 1: Make the fingerprint a property of the graph, not of insertion order

Establish what the fingerprint is for: it exists so two callers can tell whether a neighbourhood
is the same neighbourhood. A fingerprint that changes when nothing changed cannot do that job.
So:

- Determine what the current fingerprint is computed over, and replace every random or
  insertion-ordered component with a stable key. If the fingerprint is a hash of an ordered list,
  the list must be ordered by a stable key — the same fix plan 014 applied to retrieval, and for
  the same reason.
- State in the code comment what makes two neighbourhoods "the same", because that is the
  property being protected and it is currently implied by an implementation detail.
- Prove it: build the same corpus twice, in two separately created schemas on the same server,
  and assert the fingerprints are equal. A test that only compares two calls in one database is
  the test that let this through.

### Step 2: Decide the allocation question on evidence

153 MiB per call at the bound is a number to understand before optimising. Do the analysis, then
act only if the act is justified:

- Explain where the allocation comes from — the edge set materialised per iteration, the label
  propagation, the candidate pairs — with a profile, not a guess. The benchmark already records
  allocations per call; extend it to attribute them.
- Then choose, and justify:
  - **(a) Reduce the allocation** without changing the answer, if the profile shows an obvious
    redundancy: repeated materialisation of the same set, a per-iteration copy that a single
    pass would avoid, or a structure with a cheaper equivalent. Correctness first: any change
    that could alter the partition is out.
  - **(b) Bound it instead** — an explicit, documented budget: the operation refuses, or returns a
    truncated-but-labelled result, above a size where the answer stops being trustworthy. The
    plan's own uncertainty discipline applies: a bounded result must say it is bounded, the way
    the map and the candidate list do.
  - **(c) Record it as a known limit** with the measurement, the concurrency implication, and the
    condition under which it must be revisited. This is a legitimate answer, not a cop-out, if
    the profile says the cost is inherent to the algorithm at this size.
- Whatever you choose, re-run the benchmark and re-record the table. A number that was measured
  before the change is not the number after it, and the document must not mix the two.

### Step 3: Re-record and re-decide

`docs/graph-benchmark.md` exists to keep the Neo4j gate tied to measurements. After both fixes:

- Re-run the benchmark at, below and above the bound, three times, and report the variance
  explicitly. The corpus is synthetic and deterministic, so a run-to-run difference in a
  measurement is a finding about the code, not about the data.
- State whether anything in the document's decision changes. If the answer is still "the data is
  insufficient to reopen the Neo4j gate", say so with the numbers, and say which number would
  have to move for the answer to change.
- Update `docs/phase-status.md`: the fingerprint-stability residual goes when Step 1 lands; the
  allocation residual stays or goes with whatever Step 2 concluded, and the reason travels with it.

## Test plan

- Two equivalent corpora in two schemas produce equal fingerprints.
- A single corpus measured repeatedly produces equal results, and any difference is a failure.
- The allocation profile is recorded, and the chosen remedy — if any — does not change which
  communities are returned.
- The benchmark table is regenerated from the run, and its variance is stated.

## Done criteria

- [ ] A neighbourhood fingerprint is a property of the graph, proven equal across two equivalent corpora.
- [ ] The community-detection allocation is explained by a profile, and either reduced, bounded with a stated limit, or recorded as inherent with its condition.
- [ ] `docs/graph-benchmark.md` and its JSON are re-recorded from the post-change run, with the variance stated.
- [ ] The Neo4j decision is restated against the new numbers, or restated as still open with the numbers.
- [ ] `make verify`, `make verify-full` and `make e2e` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if a stable fingerprint would require storing an ordering the schema does not have; report the smallest schema change that would, as a decision rather than making it.
- Stop if reducing the allocation would change the partition, the community count, or any label.
- Stop if verification fails twice.

## Maintenance notes

A benchmark that is re-recorded after every change to the path it measures is the only reason the Neo4j gate stays honest. Keep the corpus, the bounds and the three-arm structure; change nothing else without re-running.
