# Hybrid retrieval: the measurement behind Phase 20's first criterion

**Status:** measured once, on one machine, on a synthetic corpus that one author
also wrote the labels for. The criterion is **still open**, and the reason is now
specific rather than vague: the only embedding provider this repository is allowed
to use returns a hash, so the "vector-only" arm is not an embedding baseline.

**Decision it informs:** `IMPLEMENTATION_PLAN.md:1458`, "multi-hop research
questions improve over vector-only RAG", and the Blocker 1 text in
`docs/phase-status.md`, which this plan replaces.

**Recorded run:** `docs/benchmarks/retrieval-arms.json`, produced by commit
`1c119c1` on 2026-09-27, corpus version `plan-014-retrieval-v1`, question-set
sha256 `90f647338d24b4d2e1ac352bc4d8675ed0530e7b792eeeba7b517e92eb01d0e1`.

## What was wrong in the record, and what is true now

`docs/phase-status.md` said the repository had "no embedding retrieval in the
query path" and that closing the blocker needed "a labelled question set *and a
second retrieval path*". The first half was false and the second was wrong about
the cause.

- **The second retrieval path already existed.** `retrieveVectorPassages`
  (`internal/research/retrieval.go:70-122`) scores passages with
  `1 - (sp.embedding <=> $1::vector)`, and `execute`
  (`internal/research/rag_service.go:77-85`) has always called both legs and fused
  them, persisting `Score.Lexical`, `Score.Vector`, `Score.Rerank` and
  `Score.Combined` separately (`rag_service.go:387-392`).
- **What was missing was data.** `source_passages` held three rows and none had an
  embedding, because `db/seeds/` is pure SQL and the only writer of that column is
  the source-processing worker. Every test, demo and benchmark in the repository
  had therefore been running the lexical leg alone, silently.

Both halves are fixed here. The corpus now carries embeddings produced through the
same `/embed` contract the worker calls, and there is a labelled question set and
a harness that measures the three arms.

## Reproducing it

The measurement needs a **migrated database and the AI service running**, and
nothing else. It provisions its own corpus, embeds it, measures it and drops it.

```sh
# any database with the schema applied; no seed required
COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=dawha_retrieval make db-migrate
cd services/ai-research && .venv/bin/python -m uvicorn app.main:app --host 127.0.0.1 --port 8000 &
COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=dawha_retrieval AI_RESEARCH_URL=http://127.0.0.1:8000 make retrieval-report
```

`make retrieval-report` refuses to start without `AI_RESEARCH_URL`, and the run
refuses to start unless the corpus it just provisioned is fully embedded, so a
report over a half-embedded corpus cannot be produced by accident. Once the corpus
is provisioned the measurement itself is read-only: every leg it calls is a
`SELECT` and the graph traversal runs in a read-only transaction.

### Where the corpus lives, and why it is not a seed

**The corpus is not in `db/seeds/`, and that is a correction rather than a
preference.** It was there first. `make db-seed` applies everything in
`db/seeds/`, so a corpus there lands in the developer's demo database, in the E2E
stack and in `make verify-full` - and it did, until it broke
`apps/web/e2e/journeys/09-research-query.spec.ts`. That journey asserts that a
query about nothing in the corpus is **refused**; its query was a seven-character
nonsense word, and against three short demo passages it matched nothing while
against forty-two long ones it shared a pg_trgm character trigram with three of
them, so the product answered instead of refusing. The journey was right and the
corpus was in the wrong place. Measurement scaffolding had become the product's
demo data, and the demo data's job is to be small.

The corpus therefore lives next to the labels that judge it, at
`services/core-api/internal/research/testdata/retrieval_corpus.sql`, and
`make retrieval-report` provisions it into an isolated schema
(`dawha_retrieval_measurement`) that it drops on the way out. Three properties
follow, and they are the reason for the shape rather than tidiness:

- **the command adds nothing to the database it is pointed at.** The only writes
  are the corpus and its embeddings, and both disappear with the schema, so it is
  safe against a developer's own database and cannot leak into the E2E stack.
- **a killed run cannot leave half a corpus behind**, because the schema is dropped
  before it is created.
- **the measurement is hermetic.** The measurement schema gets the migrations and
  the corpus and *not* `db/seeds/001_demo.sql`, so the numbers are the same on a
  freshly migrated database as on a seeded one, and no demo passage that no label
  judges sits in the candidate pool. The corpus is also self-contained: it creates
  its own owner user and its own six places, because when it was a seed it referred
  to the demo seed's `users` and `places` rows by id, and a fixture that needs
  another file's rows is not a fixture.

`DAWHA_RETRIEVAL_KEEP_CORPUS=1` leaves the schema in place for reading in psql.

### The backfill, and why it is still a separate command

`make db-embed` still exists and still writes embeddings onto the *seeded*
passages, for the case where somebody wants an embedded demo corpus to look at in a
browser. `make retrieval-report` does not need it: it embeds its own corpus
through the same `/embed` contract.

Point `make db-embed` at a scratch database, not at one the Go suite runs against.
That is not caution, it is a fact about an existing test:
`internal/search`'s `TestSemanticSearchUsesPublicProvenance` asserts an exact count
of public passages, it passes on an unembedded seeded corpus, and it fails once the
seeded corpus has vectors. That is also the clearest evidence in this repository
that the seeded corpus's missing embeddings were load-bearing for a test that did
not know it.

## The corpus

`services/core-api/internal/research/testdata/retrieval_corpus.sql` is the corpus
the demo seed's three passages cannot support a comparison over: **42 passages over
13 sources, all 42 retrievable** (an accepted statement exists, which both legs
require), 9 people, 3 families, 6 places, 21 claims, 4 recorded migrations and one
published tree. It is synthetic and marked `{"synthetic":true}`. The people are
composites; the place names are real regions and the people attached to them are
not. No living person and no personal data appears.

The question set is
`services/core-api/internal/research/testdata/retrieval_measurement_cases.jsonl`:
**29 Arabic questions**, each with the passages a reviewer judged relevant, each
carrying `reviewed_by` and a one-line reason naming the hop chain.

| Class | Cases | What it is for |
| --- | ---: | --- |
| `multi_hop` | 17 | The criterion. A person reached through a family (8, `branch_claims`), a source reached through a claim (5, `evidence_connection`), a place reached through a migration (4, `geographic_path`). |
| `single_hop` | 7 | The control against a multi-hop subset that simply behaves like the corpus average. |
| `lexical_control` | 5 | Short, distinctive queries — a rare phrase that is a hapax in the corpus — so a leg that is merely fuzzy cannot win them. |

**The same author wrote the corpus and the labels.** There is no second reviewer.
These are internal fixtures judged by the person who built the thing they judge,
and every number below inherits that. It is said here, and it is in the JSON
report's `corpus.author_caveat` field, so it cannot be quoted without it.

Two further limitations belong next to the numbers rather than in a footnote:

- **The labels were written before the run and not revised after seeing it.** That
  is the strongest claim available about a self-authored fixture set; it is not the
  same as an independent judgement. One label did change during the work, before
  any measurement: `mh-geo-03` asked for حجاز as a destination and that place moved
  into the corpus's own id space when the corpus stopped referencing the demo
  seed's rows. The question and its judged passage are unchanged.
- **Every judged passage is a one-sentence statement about a named entity, and an
  answerable question has to name that entity.** So partial lexical overlap between
  a question and its answer is intrinsic to this corpus rather than a property of
  the retrieval paths. A null result on the multi-hop subset here is therefore not
  evidence that graph retrieval does not help on real corpora, and the report says
  so where it reports the null.

## The arms

Three arms over one corpus and one question set, differing only in the retrieval
path. All of them call the shipped functions; the ranking, the fusion weights
(0.55 lexical / 0.45 vector), the rerank call and every threshold are the ones the
service uses.

1. **vector-only** — `retrieveVectorPassages` alone, ordered by vector score.
2. **hybrid** — lexical + vector + `fuseAndRerank`, which is the shipped path in
   `execute`.
3. **graph-augmented** — the hybrid, plus the graph citations `execute` appends.

The graph arm reproduces `execute`'s ordering rather than a better one.
`graphPassageCitations` appends graph citations **after** the fused list with
`Rerank: 0.5, Combined: 0.5` and does not re-sort, so in the shipped product a
graph passage can only reach the top five when the passage legs returned fewer
than five candidates. An arm that merged graph evidence into the ranking would be
measuring an improvement nobody shipped.

A fourth arm, **reversed** — the hybrid with its ranking inverted — is not a
proposal. It exists so the harness can be shown to notice a bad ranking. See
"The harness is not vacuous" below.

Metrics: `recall@5` is the fraction of each case's judged passages inside the
arm's first five results; `MRR` is the mean reciprocal rank of the first judged
passage. A question the arm cannot answer scores zero for both rather than being
dropped, so an arm cannot raise its average by answering fewer questions. Both
are over the case's whole judged set, because three of these cases have two
defensible answers and a metric that accepted only the first would be measuring
the corpus's tidiness.

## The numbers

Recorded run, 29 questions, k=5, PostgreSQL 16.15 in the project's container, Go
1.25.

| Arm | recall@5 (all) | MRR (all) | empty | **multi-hop recall@5** | **multi-hop MRR** | single-hop | lexical control |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| vector-only | 0.103 | 0.063 | 0 | **0.118** | **0.078** | 0.143 | 0.000 |
| hybrid | 1.000 | 0.848 | 0 | **1.000** | **0.784** | 1.000 | 1.000 |
| graph-augmented | 1.000 | 0.848 | 0 | **1.000** | **0.784** | 1.000 | 1.000 |
| reversed (deliberately broken) | 0.000 | 0.000 | 0 | 0.000 | 0.000 | 0.000 | 0.000 |

No arm returned nothing for any question.

## The stated variance

**0.248 on the multi-hop subset of 17 cases.** It is the sum of the two arms'
Wilson 95% half-widths for recall, and it is deliberately conservative twice over:

- it is an **interval half-width**, not a point estimate, so it does not collapse
  when an arm scores 1.0 — the plain binomial standard error is exactly zero at a
  perfect score, which is precisely backwards, since a perfect score on seventeen
  questions is the measurement most in need of an uncertainty interval;
- it treats the two arms as **independent samples** even though they are scored
  over the same questions. A paired statistic would be tighter. A criterion about
  multi-hop questions should not be met on a difference a tighter statistic would
  have called significant and this one would not.

One case in seventeen is worth 0.059 of recall on its own. At recalls near a half
the bar is about 1.96/sqrt(n): 0.43 at 17 cases, 0.20 at 100, 0.10 at 381.

## Findings

**1. The vector-only arm is not measuring embeddings, and this is the finding
everything else rests on.** `DeterministicProvider.embed` is a SHA-512 digest of
the normalized text spread across the requested dimensions. The run measures this
rather than assuming it, and the numbers are the signature of a hash:

> cosine(query, itself) = 1.0000; cosine(query, 42 other corpus texts) mean
> 0.0678, min −0.2573, max 0.3927, and **28 of 42** of them came out positive.

A semantic embedding places related texts well above unrelated ones and keeps the
corpus on one side of zero. This one is orthogonal to everything and signs at
random, so `WHERE vector_score > 0` admits a coin-flip half of the corpus ordered
by noise. The 0.103 recall@5 is a permutation, not a baseline. **A difference
against it is a difference against noise, and calling that "multi-hop questions
improve over vector-only RAG" would be exactly the unfalsifiable claim this plan
exists to stop.** The report's decision code refuses to mark the criterion met
unless the provider is semantic as well as the delta being large, and
`TestRetrievalDecisionCannotBeMetWithoutEvidence` asserts that half of the
condition on its own.

**2. The graph leg contributed four passages in total, across three cases, and
never changed a top-five.** All seventeen multi-hop cases name a traversal. Four of
them added a citable passage, and in all four the passage was already inside the
hybrid's top five, so the addition moved nothing. The other thirteen failed for
three distinct and nameable reasons:

| Outcome | Cases | Mechanism |
| --- | ---: | --- |
| Cited a passage the hybrid already had | 9 | `retrieveLexicalPassages` returns every passage sharing a trigram with the query, `fuseAndRerank` keeps thirty, and `graphPassageCitations` appends only what the hybrid did not already have. A multi-hop case here already returns 30 candidates, so the appended passages land at rank 31 and cannot reach k=5. |
| Evidence is not citable | 4 | Every `geographic_path` ref is a `geographic_event` row with a layer and no passage or statement id, and `graphPassageCitations` skips a ref with neither. A migration can be *shown* to a reader and nothing in it can be *cited*. |
| No path at all | 1 | `mh-chain-05` asks for the father of ميمون بن مهند. That relation is claim `…0117`, whose status is `unresolved`, and `evidence_connection`'s `eligible_claims` drops `unresolved`, `rejected`, `superseded` and `unknown`. The relation is documented in an accepted statement that retrieval reaches and traversal cannot. |

The second and third are defects worth naming, and neither was fixed here: this
plan changes no retrieval behaviour. The first is a structural property of the
shipped ordering and is the single most actionable thing in this document —
merging graph evidence into the fused ranking, rather than appending to it, is the
change that would let a graph passage compete for a top-five slot.

**3. On this corpus the lexical leg answers multi-hop questions perfectly, and
that is a statement about the corpus.** Hybrid recall@5 is 1.000 on the multi-hop
subset, the single-hop subset and the lexical controls alike, and the multi-hop
MRR (0.784) is *lower* than the single-hop MRR (0.893) only because the multi-hop
questions are longer, so their first relevant passage sits slightly deeper. There
is no subset on which the graph or vector legs win. A corpus of one-sentence
passages about named entities is a corpus the trigram leg is built for: the
question shares the entity's name with the passage that states the fact about it,
so the hop the question is named for is not needed to *find* the passage — only to
know *why* it is the answer. Recall@k does not measure that, and a graph
traversal's real contribution is provenance rather than retrieval.

**4. The "not enough evidence" branch is unreachable on a real corpus, and the
broken E2E journey is how this was found.** `retrieveLexicalPassages` filters on
`GREATEST(similarity(...), ILIKE hits) > 0` and `similarity()` is a pg_trgm
**character**-trigram score. On a corpus of ordinary Arabic sentences, *any* query
of three characters or more shares a trigram with something, so it becomes a
candidate and the insufficient-evidence branch is never reached. Measured on the
42-passage corpus: a seven-character nonsense word scores 0.0159 against three
passages and matches them; a five-character word matches five; only a string too
short to yield a trigram matches nothing. `apps/web/e2e/journeys/09-research-query.spec.ts`
had asserted the refusal with a seven-character word and passed only because the
demo corpus was three short passages.

And the branch is unreachable *always* once embeddings exist, for the reason in
finding 1: a hashed embedding is positive for roughly half of any corpus, so
`WHERE vector_score > 0` admits passages for every query. **A reader is told "not
enough evidence" only on a corpus with no vectors at all.**

This is not fixed here. It is a threshold question about `> 0` on a trigram score
and about what an embedding of unknown quality should be allowed to contribute, and
both are retrieval changes this plan does not make. It is recorded because the
criterion in `IMPLEMENTATION_PLAN.md` is about a research product refusing to
answer without material, and that refusal is currently reachable only in
configuration this repository does not ship.

**5. The reversed arm measures worse, so the harness is not vacuous.** MRR 1.000
for the good arm against 0.500 for the deliberately inverted one on a fixture
corpus, and recall@5 1.000 against 0.000 on the real one. The proof is on MRR
rather than recall@5 for a reason worth stating: with fewer than six results,
reversing a list moves every passage but keeps them all inside the top five, so
recall@5 cannot see the difference and a reader would be told the harness works
when it is blind to order. Both the fixture test and the report run assert
`good > broken`, so a change that made the ranking arbitrary fails a gate.

## The decision

**Phase 20's first criterion is still open.** Recorded in
`docs/phase-status.md` with this measurement beside it, and computed rather than
asserted: `applyRetrievalDecision` has no branch that marks the criterion met
unless the measured difference exceeds the stated variance **and** the provider's
embedding is semantic.

The measured difference is large — hybrid beats vector-only by 0.882 recall@5 on
the multi-hop subset (0.897 over all 29), far outside the 0.248 bar — and it is still not evidence for
the criterion, because the arm it beats is a hash. A negative result here is
recorded as a finding, not reframed.

## What the smallest corpus that would conclude is

At 17 multi-hop cases the bar is 0.248. It shrinks roughly as 1/sqrt(n), so
clearing a ten-point difference needs about **381 judged multi-hop cases per arm**
and clearing a five-point difference needs about **1533**.

No synthetic corpus this repository can generate supplies those. They have to come
from a **production query log with judged results**: real questions, relevance
judgements by somebody who did not write the questions, and enough of them to
matter. Three things have to be true at once for that corpus to answer the
criterion, and each is a precondition rather than a task:

1. **A semantic embedding provider.** Without one there is no "vector-only" arm
   to improve on, whatever the corpus size. This is the binding constraint today
   and it is a decision, not a measurement.
2. **Graph evidence merged into the ranking.** With graph citations appended after
   a 30-item fused list, a corpus of any size measures the graph leg as
   contributing nothing.
3. **A few hundred judged multi-hop questions**, which needs real traffic.

Until those exist, the honest statement is that a corpus this size can rule an arm
out and cannot rule one in.

## Caveats

- **One machine, one container, one database, one run.** Nothing here is a
  latency claim and nothing here is reproducible as a timing.
- **The corpus and the labels share an author.** Stated in the report's own output
  and repeated at the head of this document.
- **The corpus is synthetic and small.** 42 passages and 29 questions. A perfect
  hybrid score is a statement about a corpus the trigram leg is built for, and the
  report says so rather than quoting 1.000 as an achievement.
- **The vector arm is a hash.** Its 0.103 is what noise scores on this corpus, and
  it is reported rather than hidden because the alternative is a report that reads
  as though a semantic comparison had happened.
- **`internal/search` fails on an embedded corpus.** A pre-existing assertion
  about the exact number of public passages, not a regression, and the reason the
  two workflows use different databases.
- **A pre-existing cross-package test race, found and fixed on the way.** Plan 014
  added a database-backed package, which changed the schedule of `go test ./...`
  enough to expose a defect that had been latent: fourteen packages write to the
  PUBLIC schema of one `DATABASE_URL`, and `internal/search`'s
  `TestSemanticSearchUsesPublicProvenance` asserted a *global* count of public
  passages, so it failed whenever `internal/sourceprocessing`'s integration-test
  fixture (`مصدر ثنائي قديم`, created with no cleanup) happened to be in flight.
  Reproduced deterministically on an **unmodified** checkout by inserting one
  public source and two passages from outside the package, which is how it is
  known to be pre-existing rather than caused here. The fix scopes every count and
  index in that test to its own rows; the private-leak assertions stay global,
  because those are the security ones. Worth knowing separately from this plan:
  any package that asserts a global count over a shared schema is a coin flip.
- **The seed is insert-only.** `db/seeds/001_demo.sql` carried one
  `normalized_text_ar` value that `identity.NormalizeArabicName` would not produce
  ("اشاره الى" where the normalizer gives "اشارات الي"). Re-seeding an existing
  database does not fix it, because every seed is `ON CONFLICT DO NOTHING`; a seed
  change is tested against a fresh database or not at all.

## Maintenance

Re-run `make retrieval-report` when any of these change: the fusion weights in
`fuseAndRerank`, the scoring in either leg, the ordering in `graphPassageCitations`,
the append-vs-merge decision, the provider's `embed`, the corpus, or a label. The
run refuses to produce a report over a corpus that has drifted from the labels
(a judged passage that is not in the database, or one with no accepted statement)
and it refuses to produce one over an unembedded corpus.

The corpus and its labels are the durable asset and they are versioned together by
`RetrievalMeasurementVersion`, for the reason `evaluation/thresholds.py` versions
its thresholds: two reports are only comparable when the thing they measured is the
same thing. **Bump it when a passage, a label or a judged set changes.** The
harness is disposable.
