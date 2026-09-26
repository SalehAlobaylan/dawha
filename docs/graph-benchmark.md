# Graph retrieval limits: the measurement behind the Neo4j decision

**Status:** measured once, on one machine, with synthetic data. It does not
reopen the Neo4j decision, and the last section says plainly why the data is not
enough to reopen it.

**Decision it informs:** the gate in `IMPLEMENTATION_PLAN.md:1619-1646` ("Add it
only if measurements show PostgreSQL graph retrieval is becoming limiting") and
the derived-projection sketch in `ARCHITECTURE.md:605-632`. PostgreSQL stays
authoritative; nothing in this document proposes otherwise.

**Recorded run:** `docs/benchmarks/graph-source-dependency.json`, produced by
commit `4e54553` on 2026-09-26T17:41:05Z.

## What was measured, and what was not

The workload is the source-dependency graph: a public source, the public sources
it depends on, and the public sources those depend on, bounded at depth 2 and at
200 nodes / 200 edges. That is the operation
`IMPLEMENTATION_PLAN.md:1464-1501` (Phase 21) added, and it is the only graph
operation in the repository whose cost is driven by a graph someone else built
rather than by a tree the user drew.

Measured, for three synthetic graphs:

| Measure | How |
| --- | --- |
| Retrieval latency | `retrieveGraphSourceDependencyNeighborhood`, the recursive read-only transaction the service runs, p50 and p95 by nearest rank over 20 samples |
| Operation latency | the same traversal plus run bookkeeping and persistence: what a request actually waits for |
| Community detection | `detectGraphSourceDependencyCommunities`, the Go greedy modularity over the bounded path, measured on one retrieved path so the number is the algorithm and not a query |
| Memory | Go-side `TotalAlloc` and `Mallocs` deltas around the timed loop, per call. This is the API process. It is not PostgreSQL's memory. |
| Truncation | the returned node and edge counts, the truncation verdict, and which limit fired |
| Query plan | `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` of the exact query text in `graph_source_dependency.go`, same five parameters, same read-only repeatable-read transaction, same `statement_timeout = 2000ms` |
| Headroom | the same traversal with no bounds at all, run by the benchmark, to show what the bound is hiding |
| Stability | every graph re-requested 22 times, then deleted and rebuilt from scratch and re-requested |

Not measured, and not claimed:

- No production traffic. There is none. Every number here is a laptop and a
  container.
- No concurrency. Every sample is a single request on an otherwise idle
  database. `graphrag.go:69` sets a 2s query timeout, and nothing here says what
  happens to twenty concurrent ones.
- No real source graph. `source_dependencies` rows are written by the analysis
  worker from real processing; this repository has no production-shaped example,
  and inventing one would be guessing at the shape that matters.
- No comparison against any other engine. Nothing here ran Neo4j or anything
  else, so this document contains no claim about what another database would do.

## Reproducing it

```sh
COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<a scratch database> make graph-benchmark
```

`COMPOSE_PROJECT_NAME=dawha` reuses the running `db` container instead of
starting a second PostgreSQL on port 55432. The benchmark builds its graphs in an
isolated fixture schema (`p004_fixture_*`) and drops it on the way out, so the
database it is pointed at is left as it was found. The report path defaults to
`docs/benchmarks/graph-source-dependency.json`; `DAWHA_GRAPH_BENCH_ITERATIONS`
and `DAWHA_GRAPH_BENCH_REPORT` override the sample count and the destination.

The same command runs
`TestGraphSourceDependencyNeighborhoodStaysBoundedAcrossGraphSizes`, which is
also part of `make verify-full`. That test is the assertion rather than the
measurement: it fails if output leaves the bounds, if a graph exactly at the
bounds reports truncation, or if a repeat request disagrees.

The synthetic rows are marker titles (`benchmark-<scenario>`) in a schema that
stops existing when the command finishes. No secret and no personal data is
involved, which is why this measurement could be run at all.

## The graphs

| Scenario | Shape | Nodes | Edges |
| --- | --- | --- | --- |
| `below-50-nodes-49-edges` | root, 49 direct dependencies | 50 | 49 |
| `at-200-nodes-200-edges` | root, 199 direct dependencies, one extra statement between two of them | 200 | 200 |
| `above-1201-nodes-1599-edges` | root, 400 direct dependencies, each with 2 of its own and a second statement on its predecessor | 1201 | 1599 |

The "at" scenario exists to pin a bound that must **not** fire. A limit that
always truncates carries no information, so the assertion suite requires a graph
of exactly 200 nodes and 200 edges to come back whole.

The "above" scenario is layered *and* edge-dense on purpose. A wide layered
graph alone trips the node bound first and returns 199 edges, so the edge bound
is never exercised; the second dependency statement between each pair is what
puts more than 200 edges inside the retained node set and makes both limits fire.
That was found by writing it the obvious way first and watching `edge_limit` not
appear.

Synthetic ids sort in insertion order. That matters: the query retains the first
200 nodes ordered by `(depth, source_id)`, so with unordered ids the retained set
is a hash-ordered subset nobody can predict and the arithmetic above could not be
checked against the traversal. Each run also verifies that the traversal reached
exactly the node and edge counts in the table, and the recorded run's first draft
failed that check with an off-by-one edge count, which is the check earning its
keep.

## Latency and memory

Recorded run, 20 samples per operation, PostgreSQL 16.15 in the project's
container, Go 1.25.13, 8 CPUs. All figures in milliseconds; memory in KiB per
call.

| Scenario | Operation | p50 | p95 | first call | KiB/call | allocs/call |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| below (50n/49e) | retrieval | 4.631 | 8.007 | 61.341 | 180 | 602 |
| below | neighborhood | 36.023 | 37.623 | 63.078 | 577 | 10,351 |
| below | community detection | 1.842 | 2.323 | 2.443 | 2,722 | 5,428 |
| below | communities | 42.338 | 50.229 | 40.702 | 3,467 | 15,822 |
| at (200n/200e) | retrieval | 18.001 | 24.649 | 19.591 | 733 | 2,117 |
| at | neighborhood | 158.721 | 186.516 | 172.690 | 2,425 | 41,611 |
| at | **community detection** | **89.944** | **91.574** | 87.527 | **156,982** | **82,307** |
| at | communities | 277.306 | 373.163 | 268.215 | 160,081 | 123,979 |
| above (1201n/1599e) | retrieval | 23.347 | 44.292 | 21.444 | 725 | 2,212 |
| above | neighborhood | 183.158 | 213.284 | 367.297 | 2,415 | 41,719 |
| above | community detection | 19.765 | 26.964 | 17.148 | 11,271 | 54,912 |
| above | communities | 209.202 | 220.507 | 279.967 | 14,379 | 96,675 |

The first call is reported separately rather than dropped. On the smallest graph
it is 61ms against a 4.6ms p50: a cold plan and a cold buffer cache. That is
real, it is what the first user after a deploy waits for, and averaging it into a
p95 would make the steady state look worse than it is while hiding the only part
a reader could act on.

## Truncation and retention

| Scenario | Returned | Truncated | Reasons | Node retention | Edge retention | Truncation rate | Status |
| --- | --- | --- | --- | ---: | ---: | ---: | --- |
| below | 50 nodes / 49 edges | no | — | 100% | 100% | 0 of 22 | `structural` |
| at | 200 nodes / 200 edges | no | — | 100% | 100% | 0 of 22 | `structural` |
| above | 200 nodes / 200 edges | **yes** | `node_limit`, `edge_limit` | 16.65% | 12.51% | 22 of 22 | `partial` |

The run-level truncation rate is 0.3333 across 66 requests. That number is a
property of the three chosen sizes, not a measurement of how often real traffic
truncates: a graph below or at the bounds cannot truncate and a graph above them
must. The per-scenario reason list is the part worth reading.

Above the bound the operation returns the cap exactly, says so, and downgrades
its own status from `structural` to `partial`. A reader is told the neighborhood
is incomplete instead of being handed 200 of 1,201 dependencies as though it were
all of them.

## Query plan

| Scenario | Planning | Execution | Shared buffers | Plan shape |
| --- | ---: | ---: | ---: | --- |
| below | 2.172 | 2.624 | 164 | `Recursive Union` over a materialized `active_edges` CTE; `Seq Scan` on `sources` (54 rows) and `source_dependencies` (50 rows) |
| at | 3.102 | 10.679 | 458 | same shape; `Seq Scan` on `sources` (254) and `source_dependencies` (250) |
| above | 2.604 | 14.649 | 2,381 | same shape; `Seq Scan` on `sources` (1,455) and `source_dependencies` (1,849) |

Zero shared blocks were read from disk in any scenario: every buffer was already
resident. The plan is index-light and scan-heavy at all three sizes, which is what
a 1,849-row table in a fixture should produce. Whether that holds on a table with
a million `source_dependencies` rows is a question this document does not answer,
and it is the first thing a future run should measure.

**One development observation worth keeping.** The first working version of this
benchmark did not `ANALYZE` after seeding. The above-the-bound scenario measured
a retrieval p50 of 127ms while `EXPLAIN ANALYZE` of the same query on the same
rows measured 7ms, because the planner had no statistics for the freshly inserted
rows and inlined the `active_edges` CTE instead of materializing it — 250
re-reads of the `sources` index and 36,141 buffer hits where the analyzed plan
spends 458. Two orders of magnitude apart, both correct answers to different
questions. The harness now analyzes after seeding, and this paragraph is here so
that anyone reproducing the benchmark without it knows the number they get will
be wrong in a specific, explainable way. This is an observation about a fixture
built and measured in one second, not a claim about autovacuum in production.

## Headroom: what the bound is hiding

The same traversal with no node, edge or saturation bound, run by the benchmark:

| Scenario | Reachable nodes | Reachable edges | Elapsed | Overshoot vs `GraphMaxNodes` |
| --- | ---: | ---: | ---: | ---: |
| below | 50 | 49 | 3.570 | 0.25x |
| at | 200 | 200 | 4.322 | 1.00x |
| above | 1,201 | 1,599 | 14.453 | 6.01x |

The unbounded traversal of a graph six times the bound finished in 14.5ms, against
10.7ms for the bounded query at one quarter the size. The bound is not what makes
this fast; the bound is what makes it *predictable*. Nothing in this measurement
shows PostgreSQL straining at 1,200 nodes — it shows the query is comfortable
there and the product has decided not to show more than 200 anyway.

## Stability

Within one database, everything is deterministic. Across 22 requests per scenario
the path id, the input fingerprint, the edge-set fingerprint and the community
partition fingerprint were identical, in all three scenarios, in both recorded
runs.

Across a rebuilt graph they are not, and the run measures why rather than
asserting a verdict:

> the rebuilt graph returned a DIFFERENT edge-set fingerprint while the same
> nodes and edges: `source_dependencies.id` is a random uuid and the query orders
> candidate edges by it

`source_dependencies.id` is `gen_random_uuid()` (`db/migrations/0003_research_and_jobs.sql:223`),
and `graph_source_dependency.go:498-513` selects candidate edges `ORDER BY
edge_id`. So the edge-set fingerprint is a hash of a set whose element ids are
different in every database, and for the densest graph the greedy community loop
sees its edges in a different order, which changes which of two equally-good
merges wins. In the two recorded runs the above-the-bound partition came out as
65 communities (largest 45) and 67 communities (largest 38) — same graph, same
bounds, different answer.

The consequence for a reader: these fingerprints identify a neighborhood *within a
database*. They are a cache key and an audit trail, which is what they are for.
They are not a content-addressed hash two databases can be compared through, and
this document does not use them as one.

## Repeatability

Three runs of `make graph-benchmark`, same machine, same command, scratch
database, 20 iterations each. Run 1 is the recorded artifact; runs 2 and 3 were
written to `/tmp`.

**Every structural result is identical in all three runs**, and every run's own
assertions hold:

| Result | Runs 1, 2, 3 |
| --- | --- |
| returned nodes / edges per scenario | 50/49, 200/200, 200/200 - identical |
| truncation verdict and reasons | none, none, `node_limit`+`edge_limit` - identical |
| unbounded reachable nodes / edges | 50/49, 200/200, 1201/1599 - identical |
| plan shared buffers | 164, 458, 2381 - identical |
| merges / partitions, below and at the bound | 49/1 and 198/2 - identical |
| path id, input fingerprint, stable within the run | yes, all scenarios, all runs |

**Allocations are reproducible to five significant figures on the paths that do no
I/O**, which is the number worth having:

| Metric | Run 1 | Run 2 | Run 3 |
| --- | ---: | ---: | ---: |
| community detection, at the bound, bytes/call | 160,749,864 | 160,751,968 | 160,752,146 |
| neighborhood, at the bound, bytes/call | 2,482,939 | 2,488,592 | 2,434,136 |
| retrieval, above the bound, bytes/call | 742,468 | 736,065 | 742,832 |

**Wall-clock latency is not reproducible on this machine, and the way it fails is
worth stating precisely.** It is not per-metric noise: each run is uniformly faster
or uniformly slower than the others.

| Retrieval p50 (ms) | Run 1 | Run 2 | Run 3 |
| --- | ---: | ---: | ---: |
| below the bound | 4.6 | 7.8 | 2.8 |
| at the bound | 18.0 | 17.6 | 7.0 |
| above the bound | 23.3 | 16.9 | 12.8 |

Run 2 is 1.7x run 1 on the smallest graph and 0.7x on the largest; run 3 is 0.6x
run 1 on the smallest and 0.55x on the largest. The size ordering held in runs 1
and 3 and reversed in run 2, where the at-the-bound and above-the-bound retrievals
came out within 4% of each other. One `p95` moved by 10x between runs: the
below-the-bound neighborhood `p95` was 37.6ms, then 410.7ms, then 28.7ms, which is
one slow sample on a shared machine rather than a tail.

So: **quote the sizes, the truncation verdicts, the allocations and the
community-detection ratio from this document. Do not quote a latency number from it
as a property of the system.** The p50 column is evidence that the operation is
tens to hundreds of milliseconds, and it is not a number to hold anybody to.

**One relationship does reproduce, and it is the finding this document is for.**
Community detection on the star at the bound costs 3.6x to 8.7x what it costs on
the denser graph above it - 89.9/19.8, 123.8/14.2, 46.0/12.7 across the three runs -
with the allocations behind it agreeing to five significant figures. The cost is a
function of how many communities the greedy loop merges, not of how many edges it
was given, and that is a fact about the algorithm rather than about the machine.

## Findings

**1. The retrieval is not the cost. The bookkeeping and the persistence are.**
At the bound, retrieval is 18ms and the full operation is 159ms — 88% of the
request is everything that happens after the graph has been read: starting the
run, writing the path, the summary and the neighborhood row, completing the run.
A graph database would replace the 18ms and leave the 141ms. If this operation
ever needed to be faster, the profile points at the run bookkeeping, not at
PostgreSQL.

**2. Community detection is the one place with a real scaling problem, and it is
not a database problem.** The at-the-bound scenario, a root that cites 199
sources, allocates **153 MiB per call** (156,982 KiB) in 82,307 allocations to find two
communities. The below-the-bound scenario, the same shape at a quarter the size,
allocates 2.7 MiB. Four times the nodes costs fifty-eight times the memory. The
mechanism is in `graph_source_dependency_communities.go:216-271`: the greedy loop
rebuilds its between-community edge map on every merge, and each merge renames
the merged community by hashing its member list
(`graphSourceDependencyCommunityKey`), so a star-shaped graph pays
`O(members²)` in string building. `InferredMergeIterations` in the report is the
count that drives it: 49, 198 and 135 for the three scenarios, derived as nodes
minus surviving partitions.

This is the finding most worth acting on, and it is not an argument for a graph
database. It is an argument for bounding the greedy loop's working set, which is a
change to one Go function, and it is listed in the status matrix as an open item
rather than fixed here because this plan changes no production algorithm.

**3. Above the bound, both limits fire and the verdict is honest.** The
above-the-bound scenario retains 16.65% of the reachable nodes, reports
`node_limit` and `edge_limit`, and downgrades its status to `partial`. This is
the behavior the truncation reporting was built for and it is the strongest
evidence in this document that the bounds are a product decision rather than a
performance trick.

**4. The unbounded traversal is affordable at six times the bound.** 14.5ms for
1,201 nodes. The 200-node bound is not currently protecting the database from
anything. It is protecting the reader of a research result from a wall of
citations, which is a different reason and a defensible one.

## The Neo4j conclusion

**PostgreSQL is not the limiting factor at the sizes this product works at, and
this data does not reopen the decision.** Stated against each trigger condition
in `IMPLEMENTATION_PLAN.md:1619-1636`:

| Trigger condition | What this measurement says |
| --- | --- |
| frequent deep multi-hop traversals | Not measured. Every sample is depth 2; the bound is depth 3. Multi-hop at depth 3 over a 1,200-node graph was not run. |
| graph algorithms are core to product use | No. Two of the twelve graph operations in `graphrag.go:47-60` are source-dependency, and one of those is community detection. |
| recursive SQL becomes difficult to maintain | Not measured, and not a performance question. `graph_source_dependency.go:442-537` is 95 lines of one recursive query with three bounds in it. |
| relationship counts become very large | Not measured. The largest graph here is 1,599 edges. |
| graph-native exploration becomes latency-sensitive | No evidence. p50 is tens of milliseconds for retrieval, hundreds for the full operation, on one idle machine. |
| community detection becomes important | It is the opposite: community detection is the *slow* part (89ms, 153 MiB) and it is slow in Go, not in SQL. A graph database would not fix it. |
| source dependency networks become large | Not measured. 1,201 nodes is a synthetic maximum, not an observed one. |

**The data is insufficient to reopen this decision, and the honest reason is
that the interesting inputs do not exist yet.** Six of the seven trigger
conditions are about production scale, concurrency, or maintenance cost, and this
repository has no measurement of any of them. What would be needed, in the order
that would change the answer:

1. **A production-shaped dependency graph.** How many `source_dependencies` rows
   does a real workspace have, what is their degree distribution, and how deep
   does the reachable set actually go? The analysis worker writes these rows
   (`internal/analysisworker`); nothing counts them. Until that number exists, a
   synthetic 1,200-node graph is a guess with a p50 attached.
2. **Concurrency.** One idle request is not a service. The 2s query timeout in
   `graphrag.go:69` is a statement about what happens when a request is slow, and
   nothing in this document says how many of these a single database serves
   before the p95 stops being tens of milliseconds.
3. **A table big enough for the plan to change.** Every buffer in this run was
   already in cache, and every scan was a scan of a fixture-sized table. The
   interesting question is where the query plan flips from scan-driven to
   index-driven, and this run is far too small to show it.
4. **A p95 from real traffic** rather than from a loop on a laptop.

None of those four is a reason to add Neo4j. They are the reasons the gate stays
closed for now instead of being reopened on a date.

## Caveats

- One machine, one container, one afternoon. The latency column is evidence of
  order of magnitude and nothing finer.
- Single-threaded. Every sample is one request against an otherwise idle
  database.
- Synthetic shapes. Three graphs chosen to sit below, at and above a bound say
  nothing about the distribution of real ones; the truncation rate in particular
  is an artifact of that choice.
- The p95 of 20 samples is the 19th slowest observation. It is not a tail. A real
  tail needs real traffic, which is caveat 4 above.
- `community_detection` and `communities` are measured with the fixture's other
  two scenarios already seeded, so each scenario sees a slightly different table
  size. That is why the `at` and `above` rows are not two points on one curve,
  and it is why the document does not extrapolate from them.
- No graph engine was run. This document contains no claim about what one would
  cost, and the numbers above are not a case for or against adding one.

## Maintenance

Re-run `make graph-benchmark` when any of these change: the bounds in
`graphrag.go:37-45`, the neighborhood query, the community detection loop, or the
`source_dependencies` indexes. Replace the table above and this file's commit
reference in the same commit, and keep the variance band honest — if a future run
is reproducible to within 5%, say so and say what changed, because that would be
a result about the machine as much as about the code.
