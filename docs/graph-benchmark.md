# Graph retrieval limits: the measurement behind the Neo4j decision

**Status:** re-recorded after plan 017, on one machine, with synthetic data. It
does not reopen the Neo4j decision, and the last section says plainly why the
data is still not enough to reopen it. Two numbers in the previous run were
findings about this code rather than about the system - a fingerprint that
changed when nothing changed, and 153 MiB of garbage per community-detection
call - and both are now measured after the fixes, with the before and the after
labelled as such everywhere they appear.

**Decision it informs:** the gate in `IMPLEMENTATION_PLAN.md:1619-1646` ("Add it
only if measurements show PostgreSQL graph retrieval is becoming limiting") and
the derived-projection sketch in `ARCHITECTURE.md:605-632`. PostgreSQL stays
authoritative; nothing in this document proposes otherwise.

**Recorded run:** `docs/benchmarks/graph-source-dependency.json`, produced by
commit `bc902e8` on 2026-09-27T19:05:29Z. The previous run, at commit
`4e54553` on 2026-09-26, is quoted only where a number changed, and says which
commit it came from. Runs 2 and 3 of the same command on the same machine are in
the repeatability section and were written to `/tmp`.

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
| Memory, and where it goes | Go-side `TotalAlloc` and `Mallocs` deltas around the timed loop, per call, for the call as a whole and for each of the three phases the detection function is written as. This is the API process. It is not PostgreSQL's memory. |
| Truncation | the returned node and edge counts, the truncation verdict, and which limit fired |
| Query plan | `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` of the exact query text in `graph_source_dependency.go`, same five parameters, same read-only repeatable-read transaction, same `statement_timeout = 2000ms` |
| Headroom | the same traversal with no bounds at all, run by the benchmark, to show what the bound is hiding |
| Stability | every graph re-requested 22 times, then deleted and rebuilt from scratch and re-requested, and - through the acceptance suite - the same corpus built twice in two separately created schemas and compared |

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
bounds reports truncation, or if a repeat request disagrees. The property this
run is now also asserting is `edge_set_fingerprint_survives_a_rebuild`: a
scenario that was not truncated must return the same edge-set fingerprint after
its rows are deleted and rebuilt. The two-schema version of that property,
`TestGraphSourceDependencyFingerprintIsTheSameGraphInTwoSchemas`, is in the
acceptance suite rather than here, because it needs two fixtures and belongs
where a failure blocks a merge.

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

The node selection is deterministic and the edge selection is not, and the
difference decides what the "above" row can be used for. `source_dependencies.id`
is a random uuid and the query ranks candidate edges by it
(`graph_source_dependency.go:475-569`), so above the edge bound the 200 retained
statements are 200 of 1,599 candidates in random order. Two builds of the same
scenario retain different statements. Everything about that scenario's *counts*
reproduces; its partition does not, and the stability section says so instead of
letting the reader assume otherwise.

## Latency and memory

Recorded run, 20 samples per operation, PostgreSQL 16.15 in the project's
container, Go 1.25.13, 8 CPUs, load average about 5 on the same machine during
the three runs. All figures in milliseconds; memory in KiB per call.

| Scenario | Operation | p50 | p95 | first call | KiB/call | allocs/call |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| below (50n/49e) | retrieval | 2.790 | 4.476 | 36.690 | 139 | 655 |
| below | neighborhood | 22.109 | 43.473 | 43.473 | 533 | 10,404 |
| below | community detection | 0.344 | 0.479 | 0.429 | 213 | 500 |
| below | communities | 23.852 | 44.613 | 26.941 | 807 | 10,978 |
| at (200n/200e) | retrieval | 8.286 | 14.207 | 17.422 | 560 | 2,320 |
| at | neighborhood | 83.147 | 122.552 | 85.743 | 2,215 | 41,821 |
| at | **community detection** | **5.293** | **8.173** | **5.869** | **2,601** | **1,874** |
| at | communities | 86.270 | 97.548 | 97.548 | 5,071 | 43,928 |
| above (1201n/1599e) | retrieval | 10.222 | 12.518 | 13.105 | 575 | 2,418 |
| above | neighborhood | 85.551 | 96.891 | 87.144 | 2,249 | 41,935 |
| above | community detection | 3.615 | 5.328 | 3.739 | 1,224 | 2,481 |
| above | communities | 89.320 | 94.694 | 92.833 | 3,721 | 44,628 |

The first call is reported separately rather than dropped. On the smallest graph
it is 36.7ms against a 2.8ms p50: a cold plan and a cold buffer cache. That is
real, it is what the first user after a deploy waits for, and averaging it into a
p95 would make the steady state look worse than it is while hiding the only part
a reader could act on.

### What the community-detection memory is spent on

The recorded run attributes a community-detection call to the three phases the
function is written as, measured on its own over the same path. This is new in
this run: the previous run reported one number for the call and a guess about the
mechanism, and the guess was right, which is the only reason it was harmless.

| Scenario | input | merges | report | total | merges share |
| --- | ---: | ---: | ---: | ---: | ---: |
| below (50n/49e) | 19.0 KiB / 68 allocs / 0.017ms | 185.4 KiB / 418 / 0.364ms | 9.0 KiB / 14 / 0.016ms | 213.4 KiB / 500 / 0.397ms | 87% |
| at (200n/200e) | 74.4 KiB / 223 / 0.049ms | 2,484.4 KiB / 1,623 / 5.063ms | 42.9 KiB / 28 / 0.059ms | 2,601.7 KiB / 1,874 / 5.171ms | 95% |
| above (1201n/1599e) | 74.4 KiB / 223 / 0.052ms | 1,085.4 KiB / 1,800 / 2.454ms | 64.0 KiB / 468 / 0.411ms | 1,223.8 KiB / 2,491 / 2.917ms | 89% |

So it is not the edge set being materialised (0.05% of the at-the-bound call) and
not the per-community accounting (0.03%). It is the greedy loop, and inside the
loop - from a `pprof` line profile taken at the bound with `-memprofilerate 1` -
97.7% of the whole call was three string concatenations
(`graph_source_dependency_communities.go:298-391` before the change): 899 of the
920 MiB the five profiled calls allocated, in the between-community map key built
once per candidate pair per merge, the same key rebuilt for the tie-break, and
the incumbent's key rebuilt again inside the comparison. A community was *named*
by its own member list, so a candidate pair's key was as long as the two
communities it joined, up to 7 KiB on a star that merges 198 times over 199
candidate pairs. The cost was `O(merges x pairs x members)`, and 153 MiB is what
that is at the bound.

Communities are now addressed by an integer, so the map key is two ints and no
copy, and the member-list name is built once per merge instead of once per
candidate per merge. The name is still built, because the tie-break between
equally-good merges is defined over member lists and changing that would change
which merge wins on a tied graph. The measured result, from a run of the same
command on the same machine immediately before the change (commit `c8bb88a`):

| At the 200-node bound | before (`c8bb88a`) | after (`bc902e8`) | ratio |
| --- | ---: | ---: | ---: |
| KiB per call | 156,983 | 2,601 | 0.017x |
| allocations per call | 82,328 | 1,874 | 0.023x |
| p50 | 45.972ms | 5.293ms | 0.12x |
| partitions / largest | 2 / 198 | 2 / 198 | unchanged |

The previous run of this document recorded 156,982 KiB and 82,307 allocations at
43.95-89.94ms p50 for the same call, from commit `4e54553`; this run's
immediately-before measurement is within 0.01% and 0.03% of it on bytes and
allocations, so the before column is a re-measurement of the same thing rather
than a different number.

What is left is 1.5 MiB of the 2.6, and it is the rebuild of the
between-community map once per merge - the loop's shape, not an accident of it.
Merges are bounded by `GraphMaxNodes` and edges by `GraphMaxEdges`, so the
operation's memory is now bounded by the same numbers the product already chose
to bound the answer by. Making that rebuild incremental is a larger rewrite of
the same loop for about 1.3 MiB at a bound this product set deliberately, so the
residual is recorded here rather than chased.

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
| below | 1.148 | 1.483 | 164 | `Recursive Union` over a materialized `active_edges` CTE; `Seq Scan` on `sources` (54 rows) and `source_dependencies` (50 rows) |
| at | 1.110 | 3.802 | 458 | same shape; `Seq Scan` on `sources` (254) and `source_dependencies` (250) |
| above | 1.132 | 6.061 | 2,381 | same shape; `Seq Scan` on `sources` (1,455) and `source_dependencies` (1,849) |

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
| below | 50 | 49 | 3.713 | 0.25x |
| at | 200 | 200 | 1.945 | 1.00x |
| above | 1,201 | 1,599 | 6.284 | 6.01x |

The unbounded traversal of a graph six times the bound finished in 6.3ms, against
6.1ms for the bounded query at one quarter the size. The bound is not what makes
this fast; the bound is what makes it *predictable*. Nothing in this measurement
shows PostgreSQL straining at 1,200 nodes — it shows the query is comfortable
there and the product has decided not to show more than 200 anyway.

## Stability

Within one database, everything is deterministic. Across 22 requests per scenario
the path id, the input fingerprint, the edge-set fingerprint and the community
partition fingerprint were identical, in all three scenarios, in all three runs.

Across a rebuild, the fingerprints are now the same too, and the previous run's
finding is gone. It used to read:

> the rebuilt graph returned a DIFFERENT edge-set fingerprint while the same
> nodes and edges: `source_dependencies.id` is a random uuid and the query orders
> candidate edges by it

The edge-set fingerprint hashed `source_dependencies.id`, and the two path ids
hashed the raw edge JSON, which carries that same id and the order the query
returned the edges in. A dependency statement is now identified by `(from, to,
predicate, status)` - the tuple `source_dependencies` already declares `UNIQUE`
(`db/migrations/0003_research_and_jobs.sql:223`) - and the statements are sorted,
so the fingerprint is a property of the set and not of the row ids or the
returned order. `graph_source_dependency.go:326-356` states what makes two
neighborhoods the same, because that is the contract and it used to be implied by
an implementation detail. Which neighbours are returned is unchanged: the query,
its `ORDER BY edge_id`, the 200/200 bounds, the edge ids and positions in the
persisted JSON, all of it.

The acceptance suite proves it where a single database cannot:
`TestGraphSourceDependencyFingerprintIsTheSameGraphInTwoSchemas` builds the same
corpus in two separately created schemas on the same server, in opposite
insertion order and with independent random edge row ids, and requires the path
id, the edge-set fingerprint, the partition fingerprint and the communities path
id to be equal across them. It also asserts the two schemas really did get
different row ids, that repeated reads of one corpus agree, and - the control
that gives the other two meaning - that adding one dependency statement changes
the fingerprint.

**What is still not stable above the bound, and is not the fingerprint.** The
rebuild check now distinguishes two things that used to be one sentence. Below and
at the bound a rebuild reproduces the fingerprint and the partition exactly, and
that is asserted (`edge_set_fingerprint_survives_a_rebuild`). Above the bound it
does not, and the reason is in the query rather than in the hash: the edge bound
keeps 200 candidates in random id order, so the rebuilt graph hands the algorithm
a different set of statements. That is why the above-the-bound partition count
moves between runs of identical code - 65, 67, 64, 68 in the four runs recorded
across the two documents, and 68, 66, 73 in the three runs of this one, with the
merge count moving 132, 134, 127 alongside it. The previous document explained
this as the greedy loop's tie-breaking depending on the random edge order. That
was wrong: the loop is order-independent, and what changed was which statements
reached it.

Fixing that would mean ranking candidate edges by `(source_id,
depends_on_source_id, dependency_type)` instead of by `id` - the same stable key
used for the fingerprint, no schema change required. It is not done here because
it changes which 200 of 1,599 statements a truncated graph returns, which is
outside what this measurement is allowed to decide. It is a decision, not an
oversight: a truncated neighborhood should be a predictable subset, and the
cheapest way to get that is one `ORDER BY` in the query.

## Repeatability

Three runs of the benchmark, same machine, same command, scratch database, 20
iterations each, load average about 5. Run 1 is the recorded artifact; runs 2 and
3 were written to `/tmp`. The corpus is synthetic and deterministic, so a
run-to-run difference is a finding about the code or the machine, never about the
data.

**Every structural result is identical in all three runs** except the two the
previous section explains, and every run's own assertions hold:

| Result | Runs 1, 2, 3 |
| --- | --- |
| returned nodes / edges per scenario | 50/49, 200/200, 200/200 - identical |
| truncation verdict and reasons | none, none, `node_limit`+`edge_limit` - identical |
| node and edge retention above the bound | 16.65% and 12.51% - identical |
| unbounded reachable nodes / edges | 50/49, 200/200, 1201/1599 - identical |
| plan shared buffers | 164, 458, 2381 - identical |
| partitions below and at the bound | 1 of 50 and 2 with the largest 198 - identical |
| partitions above the bound | 68, 66, 73 - **not** identical, and not expected to be |
| fingerprint survives a rebuild, untruncated scenarios | yes, both, all three runs |
| path id, input fingerprint, stable within the run | yes, all scenarios, all runs |

**Allocations are reproducible to the byte on the paths that do no I/O**, which
is the number worth having. Spread is `(max - min)` as a percentage of the mean
across the three runs:

| Metric | Run 1 | Run 2 | Run 3 | spread |
| --- | ---: | ---: | ---: | ---: |
| community detection, at the bound, KiB/call | 2,601 | 2,601 | 2,601 | 0.0% |
| community detection, at the bound, allocs/call | 1,874 | 1,874 | 1,874 | 0.0% |
| community detection, below the bound, KiB/call | 213 | 213 | 213 | 0.0% |
| neighborhood, at the bound, KiB/call | 2,215 | 2,269 | 2,232 | 2.4% |
| neighborhood, at the bound, allocs/call | 41,821 | 41,818 | 41,806 | 0.0% |
| retrieval, above the bound, KiB/call | 575 | 555 | 561 | 3.6% |
| retrieval, above the bound, allocs/call | 2,418 | 2,409 | 2,411 | 0.4% |
| community detection, above the bound, KiB/call | 1,224 | 1,264 | 1,190 | 6.0% |

The community-detection rows move at the bound by nothing at all and above it by
6%, and the reason is the same as the partition: above the bound the operation is
given a different 200 statements each run, and the algorithm's cost depends on
the statements it was given.

**Wall-clock latency is not reproducible on this machine, and the way it fails is
worth stating precisely.** It is not per-metric noise: each run is uniformly
faster or uniformly slower than the others, and the whole set sits 40-56% below
the previous document's, which is a property of the machine on the day rather
than of the code.

| p50 (ms) | Run 1 | Run 2 | Run 3 | spread |
| --- | ---: | ---: | ---: | ---: |
| retrieval, below the bound | 2.790 | 3.702 | 3.073 | 28.6% |
| retrieval, at the bound | 8.286 | 7.490 | 7.190 | 14.3% |
| retrieval, above the bound | 10.222 | 10.475 | 9.285 | 11.9% |
| neighborhood, at the bound | 83.147 | 81.114 | 87.179 | 7.2% |
| community detection, at the bound | 5.293 | 5.453 | 5.218 | 4.4% |
| communities, above the bound | 89.320 | 87.514 | 88.197 | 2.0% |

The p95 column is worse and should not be quoted from: its spread across the
three runs runs from 2.4% to 81.6%, and one at-the-bound neighborhood p95 came
out at 200.8ms against 122.6ms and 88.7ms. That is one slow sample on a shared
machine, which is what a p95 of 20 samples is: the 19th slowest observation, not
a tail.

So: **quote the sizes, the truncation verdicts, the allocations, the partition
counts below and at the bound, and the community-detection ratio from this
document. Do not quote a latency number from it as a property of the system.**

**The relationship this document exists for still holds, and it is now a
flat one.** The previous run's finding was that community detection on the star
at the bound cost 3.6x to 8.7x what it cost on the denser graph above it - a
star being *more* expensive than a six-times-larger graph, which is what sent a
reader looking. It is now 2.0x (2,601 against 1,224 KiB) and 1.5x by p50 (5.293
against 3.615ms), in line with 198 merges against 132 over 200 edges. The
inversion is gone with its cause, and what remains is the ordinary fact that the
greedy loop's cost follows the number of merges it performs.

## Findings

**1. The retrieval is not the cost. The bookkeeping and the persistence are.**
At the bound, retrieval is 8.3ms and the full operation is 83.1ms — 90% of the
request is everything that happens after the graph has been read: starting the
run, writing the path, the summary and the neighborhood row, completing the run.
A graph database would replace the 8.3ms and leave the 75ms. If this operation
ever needed to be faster, the profile points at the run bookkeeping, not at
PostgreSQL.

**2. Community detection's memory problem was an implementation artifact, and it
is fixed.** The at-the-bound scenario allocated **153 MiB per call** in 82,328
allocations, measured again immediately before the change. A line profile put
98.6% of it in three string concatenations in the merge loop, all of them paying
for the size of a community's member list once per candidate pair per merge. A
community is now addressed by an integer and the member list is built once per
merge: **2,601 KiB and 1,874 allocations**, a 60x and a 44x reduction, with the
partition identical. The cost also stopped being a function of how many
communities the greedy loop merges into one large one and became a function of
how many merges there are. This is the finding this document is for, and it was
never an argument for a graph database: it was an argument for one Go loop, and
the loop answered.

**3. Above the bound, both limits fire and the verdict is honest.** The
above-the-bound scenario retains 16.65% of the reachable nodes, reports
`node_limit` and `edge_limit`, and downgrades its status to `partial`. This is
the behavior the truncation reporting was built for and it is the strongest
evidence in this document that the bounds are a product decision rather than a
performance trick. What is *not* honest yet is which 200 edges survive the
retention: that is decided by a random uuid, and the stability section names the
one-word change that would make it predictable.

**4. The unbounded traversal is affordable at six times the bound.** 6.3ms for
1,201 nodes. The 200-node bound is not currently protecting the database from
anything. It is protecting the reader of a research result from a wall of
citations, which is a different reason and a defensible one.

**5. A fingerprint that cannot survive a rebuild cannot answer the question it
exists for.** The previous run recorded, correctly, that a fingerprint of the
neighborhood was a property of the row ids rather than of the graph. It is now a
property of the graph, asserted in the acceptance suite across two schemas and in
this run across a rebuild, and the property is stated in the code rather than
implied by a hash over a `gen_random_uuid()` column. This is plan 014's lesson
one layer up, and it is worth more than the memory number: a measurement that
changes when nothing changed is not a measurement.

## The Neo4j conclusion

**PostgreSQL is not the limiting factor at the sizes this product works at, and
this data does not reopen the decision.** Stated against each trigger condition
in `IMPLEMENTATION_PLAN.md:1619-1636`:

| Trigger condition | What this measurement says |
| --- | --- |
| frequent deep multi-hop traversals | Not measured. Every sample is depth 2; the bound is depth 3. Multi-hop at depth 3 over a 1,200-node graph was not run. |
| graph algorithms are core to product use | No. Two of the twelve graph operations in `graphrag.go:47-60` are source-dependency, and one of those is community detection. |
| recursive SQL becomes difficult to maintain | Not measured, and not a performance question. `graph_source_dependency.go:475-569` is 95 lines of one recursive query with three bounds in it. |
| relationship counts become very large | Not measured. The largest graph here is 1,599 edges. |
| graph-native exploration becomes latency-sensitive | No evidence. p50 is single-digit to tens of milliseconds for retrieval and roughly 85ms for the full operation, on one machine under load. |
| community detection becomes important | It is the opposite, and less so than last time: it is 5.3ms and 2.6 MiB at the bound, was 46-90ms and 153 MiB. A graph database would not have fixed the 153 MiB, and it is not what fixes the 5.3ms either. |
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

**Which number would have to move.** Nothing in this table can reopen the gate on
its own, and the one number that moved since the previous run moved *away* from
it: community detection's cost per call went from 156,982 KiB to 2,601, and at
153 MiB a call there was a real argument for bounding the operation before twenty
concurrent researchers found it - 20 x 153 MiB is 3 GiB of allocation churn, and
that argument is now 60x weaker. Of the four gaps above, three are inputs that
have to be collected rather than numbers that have to move, and the fourth would
be a p95 measured under concurrency rather than on this machine. The number to
watch is the query's shared buffers at a production row count: 2,381 at 1,849
rows, all resident, zero disk reads. If a real table made that the dominant term
and the plan went index-hunting, that would be the first evidence in this
document that could be read as a database limit rather than as a fixture.

## Caveats

- One machine, one container, one afternoon, under a load average of about 5. The
  latency column is evidence of order of magnitude and nothing finer.
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
- Above the edge bound the retained statements are a random subset, so that
  scenario's partition, merge count and cost are properties of one run and not of
  the corpus. The two scenarios that are not truncated are the ones to compare.
- No graph engine was run. This document contains no claim about what one would
  cost, and the numbers above are not a case for or against adding one.

## Maintenance

Re-run `make graph-benchmark` when any of these change: the bounds in
`graphrag.go:37-45`, the neighborhood query, the community detection loop, or the
`source_dependencies` indexes. Replace the table above and this file's commit
reference in the same commit, and keep the variance band honest - if a future run
is reproducible to within 5%, say so and say what changed, because that would be a
result about the machine as much as about the code. A number measured before a
change is not the number after it: the before and after columns above name their
commits for exactly that reason.
