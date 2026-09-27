# Plan 014: Make hybrid retrieval measurable, and run the comparison the plan requires

> **Executor instructions**: This is a measurement plan. The retrieval code already has two legs; what is missing is data that can tell them apart. Build the corpus, the harness, and the report. Do not tune retrieval to win a number.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: MED
- **Depends on**: plans 004, 007, 009
- **Category**: measurement
- **Planned at**: commit `4c40e77`, 2026-09-27

## Why this matters

`IMPLEMENTATION_PLAN.md:1458` sets the acceptance criterion for Phase 20: "multi-hop research questions improve over vector-only RAG". Nothing in this repository can currently test it, and the reason recorded in `docs/phase-status.md:459` is wrong: it says there is "no embedding retrieval in the query path" and that closing the blocker needs "a labelled question set *and a second retrieval path*".

The second retrieval path already exists. `internal/research/retrieval.go:69-100` scores passages with pgvector (`1 - (sp.embedding <=> $1::vector)`), and `internal/research/rag_service.go:77,81` calls the lexical and vector legs and combines them, persisting `Score.Lexical`, `Score.Vector`, `Score.Rerank` and `Score.Combined` separately at `rag_service.go:389`. What is missing is data: `source_passages` holds 3 rows and **none of them has an embedding**, because the seed is pure SQL and only the source-processing worker writes embeddings (`internal/sourceprocessing/processor.go:258`). So every test, demo and benchmark has silently run lexical-only, and the vector leg has never been exercised on real content.

The consequence is worse than a missing feature: the product's name rests on graph retrieval, and no claim about whether graph or vector retrieval helps is currently falsifiable here.

## Current state

- `internal/research/retrieval.go` — `retrieveLexicalPassages` and `retrieveVectorPassages`; the vector leg requires a 1536-dimension query vector and `sp.embedding IS NOT NULL`.
- `internal/research/rag_service.go:77-81` — both legs run; the combined score is what a caller sees.
- `evaluation/` — the harness plans 004 and 009 built: `evaluate.py`, `baselines.py`, `thresholds.py`, and `retrieval_cases.jsonl` measuring the lexical reranker. `vector_baseline.available = false` with a reason is the current, honest output.
- `docs/graph-benchmark.md` — the precedent for a measurement this repository can defend: synthetic corpora, recorded p50/p95, a stated variance, and a decision that refuses to be made because the data is insufficient.
- `db/seeds/001_demo.sql` — three passages, no embeddings, and no source is taken through processing by the seed.
- `apps/web/e2e/journeys/05-attach-source.spec.ts` — the only path today that produces embedded passages, and its teardown now deletes them.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Eval | `make ai-eval` | exit 0 |
| Retrieval report | the command this plan adds | writes a report |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |

## Scope

**In scope**:
- `services/core-api/internal/research/` — retrieval/rag paths, a corpus builder and a comparison harness
- `services/ai-research/evaluation/` — the retrieval and vector groups, their fixtures and thresholds
- `db/seeds/` — only if the seed needs a corpus or an embedding backfill step
- `docs/` — the comparison report and the corrected `docs/phase-status.md` blocker
- `Makefile` — the documented command

**Out of scope**:
- Changing retrieval ranking, weights or fusion to make a number look better. If a leg is broken, say so; do not tune it into the report.
- Building a second vector store, an embedding model, or any external AI dependency. The existing deterministic provider supplies embeddings.
- Neo4j, or any graph database. The plan's decision gate stands and is out of scope by instruction.
- Phase 18's cost measurement; that is plan 015.

## Steps

### Step 1: Give the vector leg something to score

The corpus must have embeddings, and it must get them the way production does. Choose one and implement it:

- **(a) Backfill through the pipeline** — a documented command that takes the seeded sources through the real processing path (`cmd/source-processing` against the seeded database) so the embeddings are produced by the same code that produces them in production. Slower, and it means what it says.
- **(b) Compute them with the provider directly** — a small, explicit backfill that calls the same `/embed` contract the worker calls, for the seeded passages only.

(a) is preferable because it also exercises the pipeline; (b) is acceptable if (a) proves impractical, and the report must say which was used. Requirements: the command is idempotent, it fails loudly on a missing or wrong-dimension vector, and a test asserts that after it, every passage the corpus needs has an embedding and that `retrieveVectorPassages` returns a non-empty result for a real query. Report how many passages carry an embedding before and after.

### Step 2: Author a labelled Arabic question set

This is the part that cannot be automated, and it is the part that makes the comparison mean something. Requirements:

- Small and reviewed: the existing fixtures are 6–15 cases per group, and that is the right order of magnitude. Aim for at least 20 questions, each with the passages a reviewer judged relevant, drawn from the seeded corpus.
- Include the cases the criterion is about: **multi-hop questions** whose answer needs two or more hops through the graph (a person reached through a family, a source reached through a claim, a place reached through a migration), plus single-hop and lexical-control questions so a vector leg cannot win by being fuzzy.
- Every case records `reviewed_by` and a one-line reason, following the convention in `citation_cases.jsonl`. Say plainly in the report that the same author wrote the corpus and the labels — that is a real limitation of an internal fixture set, and hiding it would make the number worthless.
- No personal data, no living people, and no content the seed does not contain.

### Step 3: Run three arms and record three numbers

The comparison the plan asks for is vector-only versus the hybrid. Run at minimum:

1. **vector-only** — `retrieveVectorPassages` alone.
2. **hybrid** — the current `rag_service` path (lexical + vector + rerank).
3. **graph-augmented** — the hybrid plus the graph traversal already implemented in `internal/research` for the multi-hop cases. If the graph leg cannot be scored on this corpus, say so rather than dropping the arm silently.

For each arm report, at minimum: recall@5, MRR, and the number of questions where the arm returns nothing. For the multi-hop subset, report the same three separately from the single-hop subset, because a criterion about multi-hop questions is not settled by an average.

**The comparison is run on a fixed corpus and a fixed question set, with the arms differing only in the retrieval path.** If the numbers are within noise, report that; the report must be able to come out "no measurable difference", and the fixture set must be small enough that a difference would be visible if it existed.

### Step 4: Decide, and correct the record

- If hybrid (or graph-augmented) beats vector-only on the multi-hop subset by more than the stated variance, Phase 20's first criterion can be marked met — with the corpus size, the reviewer, and the variance all stated next to the claim.
- If it does not, say so. A negative result on a 20-question internal corpus is a real finding and must be recorded as one, not reframed.
- Either way, correct `docs/phase-status.md`: the blocker text claiming there is no embedding retrieval in the query path must go, replaced by what is actually true — the vector leg exists, it had no data to score, and here is the measurement.
- If the corpus turns out too small or too synthetic to support the criterion, that is a legitimate outcome: state what would be needed for a real answer (a production query log with judged results) and leave the criterion open with that named.

## Test plan

- The backfill command is idempotent and fails loudly; after it, the vector leg returns results for a real query.
- Every fixture case parses, has a non-empty judged set, and a `reviewed_by`.
- The harness runs all arms over the same inputs, and a deliberately broken arm is visibly worse (so the harness is not vacuous).
- The report is regenerated from the run, not hand-written, and its numbers are traceable to the fixtures.

## Done criteria

- [ ] The seeded corpus carries embeddings produced by the real path, and the vector leg returns results.
- [ ] A labelled Arabic question set of at least 20 cases, including multi-hop, with provenance recorded.
- [ ] Three arms measured on one corpus, with multi-hop reported separately, and a stated variance.
- [ ] Phase 20's first criterion is either met with evidence or explicitly still open with the reason — never asserted.
- [ ] `docs/phase-status.md` no longer claims there is no embedding retrieval in the query path.
- [ ] `make verify`, `make verify-full`, `make ai-eval` and `make e2e` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if producing embeddings requires an external model provider or credential — the deterministic provider is the only permitted source.
- Stop if the corpus would need personal or sensitive data to be meaningful.
- Stop if the only honest outcome is a comparison too small to conclude anything: report that, with the smallest corpus that would conclude something.
- Stop if a verification gate fails twice.

## Maintenance notes

The corpus and its labels are the durable asset here; the harness is disposable. When the retrieval path changes, the corpus must not change silently — version it the way `thresholds.py` versions its thresholds, and re-run.
