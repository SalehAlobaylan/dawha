# Implementation Plans

Generated from the read-only review of `IMPLEMENTATION_PLAN.md` at commit `a6397ae` on 2026-09-25. Execute in dependency order. These plans are handoff documents; an executor must run the stated verification gates and update the status row when complete.

## Execution order & status

| Plan | Title | Priority | Effort | Depends on | Status |
|------|-------|----------|--------|------------|--------|
| 001 | Enforce resource visibility and research authorization | P1 | L | — | DONE (merged `4086ab4`) |
| 002 | Make provenance mutations and suggestion application atomic | P1 | M | 001 | DONE (merged `ff20c2c`) |
| 003 | Align the V1 source upload and extraction contract | P1 | M | — | DONE (merged `fac52ed`) |
| 004 | Add real PostgreSQL/V1 acceptance coverage | P1 | L | 001, 002, 003 | DONE (merged `e8fc542`) |
| 005 | Complete the Phase 2 identity CRUD surface | P1 | L | 001 | DONE (merged `04b79aa`) |
| 006 | Add job lease fencing and asynchronous long analyses | P1 | L | 003, 004 | DONE (merged `a53026a`) |
| 007 | Batch list hydration and bound map queries | P2 | L | 001, 002 | DONE (merged `28fe4d9`) |
| 008 | Harden migrations, verification, storage, security, and telemetry | P1 | L | 004 | DONE (merged `0c55995`) |
| 009 | Document current state and benchmark graph limits | P2 | M | 004, 006 | DONE (merged `1777b9b`) |
| 010 | Fix the V1 frontend defects the browser suite exposed | P1 | S | 004 | DONE (merged `3302f1e`) |
| 011 | Prove the S3 adapter against MinIO, and record R2 as the production target | P2 | S | 008 | DONE (merged `2ddaa59`) |
| 012 | Stop the test-data leak, and stop citing sources that do not answer | P1 | M | 004, 009 | DONE (merged `2314de1`) |
| 013 | Close the V1 product gaps the status review found | P1 | L | 001, 002, 005, 010 | DONE (merged `e063bc3`) |
| 014 | Make hybrid retrieval measurable, and run the comparison the plan requires | P1 | L | 004, 007, 009 | DONE (merged `83eb658`) |
| 015 | Make Phase 18's cost criterion measurable, or say plainly that it cannot be | P2 | M | 008, 009, 014 | DONE (merged `6621961`) |
| 016 | Fix the remaining correctness, privacy and consistency defects | P1 | M | 001, 002, 006, 009, 012, 015 | DONE (merged `c6ddbf8`) |
| 017 | Make the source-dependency measurements stable and bounded | P2 | M | 007, 009, 014 | DONE (merged `2be2996`) |
| 018 | Build the corpus, produce the gold, and measure the state that actually exists | P1 | L | 003, 008, 014 | IN PROGRESS (corpus: 12 documents — 11 charts incl. the web GIF, the آل الصباح vector chart and three chat PNGs, plus the book; name-level paired check implemented; agreement blocked by fragment-sized proposals at the current render — §1.9; source-tile reading added for oversized charts in the product, but the corpus arm still reads captured page images and the Jasser pair is a 202/236-ppi painted source that still fragments) |
| 019 | Localise the name carriers instead of the ink runs, and read each one in isolation | P1 | L | 018 | IN PROGRESS (uncommitted: colour carriers + ink name groups on every page, hosted reader, raster-image path, source-resolution tiled reading on boxed charts; open: name grouping on dense/painted charts, separating names from prose, and a reader that can transcribe the calligraphic font — §1.10) |
| 020 | Make an import resolve identity, and make it reach the research graph | P1 | L | 018 | IN PROGRESS (uncommitted: linking, candidates, graph rows, aliases, dry run, retraction — 9 acceptances) |
| 021 | Put the machine's suggestions in the normal review screen | P1 | L | 019, 020 | IN PROGRESS (uncommitted: record, queue, source-crop workbench, stored alternatives, on-demand vision candidates, decision measurement; remaining: identity choices, relationship review, normal-draft fold-in, single-file gate) |
| 022 | Make name candidates measurable, and use the local judge only where it earns its place | P2 | L | 019, 021 | IMPLEMENTED, measurement pending (uncommitted: multi-key normalizer, evidence-aware bounded ranking, candidate snapshot, transactional validation, one-retry evidence fallback, source/class/date/judge `make name-eval`, local Liquid `d1:3b` adapter/endpoint/flags, shadow/active paths and tests; local privacy approval recorded and separate active row enabled; remaining human measurement: shadow sample ≥300 across three classes, go/no-go, any model ordering or accuracy claim) |
| 023 | Make reader availability explicit and keep evaluation versions separate | P1 | M | 019, 021, 022 implementation | DONE — explicit reader policy/capabilities, disabled action states, bounded retries, doctor, version cohorts and guarded isolated verification; 111 original actions preserved |
| 024 | Prepare local name readings once and resume them as durable jobs | P1 | L | 023 | DONE — fenced durable runs, private crop reuse, bounded local OCR variants, browser/operator controls; six-crop development benchmark and isolated acceptance passed; 111 human actions preserved |
| 025 | Build complete file context and prepare shadow evidence before human input | P1 | L | 023, 024 | DONE — counted lexicon, frozen initial/visible evidence, durable local shadow judging, versioned cohorts and development/held-out registration; 111 original actions preserved |
| 026 | Give reviewers history, deferred work, and region-repair tools | P1/P2 | L | 023, 024; evaluation joins from 025 | DONE — append-only reopen/revisions, deferred/history queue views, immutable region repair with local reread, split/merge lineage and superseded exclusion, first/latest truth policy, guarded browser verification |

Status values: `TODO | IN PROGRESS | DONE | BLOCKED | REJECTED`.

## Local name-reading pipeline: stages after the 111-action pilot

The operator selected a staged plan for the full local pipeline on 2026-10-27.
The read-only audit was performed at `f0fcb6e` with the existing uncommitted
Plan 018–022 implementation. The pilot has 111 review actions: 80 accepted or
corrected names, 14 deferrals, and 17 rejections. Of those 80 truth labels, 79
have complete shadow evidence, all in `calligraphic_dense`. The saved top-eight
pool contains the answer in 18 of the 80 cases. These counts identify engineering
failure examples; they are below the rate floor and do not establish model quality.

Execute the stages in this order:

1. [023 — capabilities and cohorts](023-local-reading-capabilities-and-cohorts.md):
   make the unavailable hosted-reader button honest, keep privacy policy explicit,
   provide operator diagnostics, and separate evaluation versions before changing them.
2. [024 — durable local reading](024-durable-local-name-reading.md):
   preserve detection before transcription, reuse crops, run local preprocessing/OCR
   through resumable jobs, and expose preparation/status/resume/cancel tools.
3. [025 — candidates and shadow preparation](025-name-context-and-shadow-preparation.md):
   use complete counted file context and local alternatives, freeze all judge input
   before human input, and let model completion happen without slowing a decision.
4. [026 — reviewer tools](026-name-review-repair-and-verification.md):
   return to deferred work, correct history through new revisions, repair crop
   geometry, reread locally, and verify the real bench on an isolated test stack.

Plan 026's history/geometry implementation may proceed after 024; its evaluation
counting must use 025's final versioned evidence contracts. Start with deferred
reopening and rectangle adjustment before split/merge. Existing Plan 019 owns the
reader intent and Plan 021 owns human review; these stages finish their missing
operational paths rather than replacing their provenance rules.

Plans 023–026 are complete. This audit covered the name-reading, candidate,
shadow-evidence, review-tool, and repair paths. It did not audit unrelated application
features, database query plans, dependency advisories, or external model quality.

### Vetted pipeline findings

Effort is for implementation including tests: S is hours, M about a day, and L
multiple days. Risk describes the proposed change, not the severity of the
current issue. These findings were confirmed from the live source; confidence
is HIGH for each.

| Finding | Impact | Effort | Change risk | Evidence | Stage |
|---|---|---|---|---|---|
| Image-reader availability is absent from the UI contract | A disabled service still offers a working-looking action | M | LOW | `apps/web/src/components/SourceSuggestionReview.tsx:1128`; `services/ai-research/app/chart_vision.py:387` | 023 |
| Service failures fan out as individual crop requests | Setup/transport errors waste up to a batch's worth of repeated calls | S | LOW | `services/core-api/internal/sourceprocessing/carrier_reader.go:372`; `services/core-api/internal/sourceprocessing/chart_vision_client.go:243` | 023 |
| Full-file name reading runs inline | Work can exceed the API response timeout without durable progress/resume | L | MED | `services/core-api/internal/httpapi/source_processing_handler.go:443`; `services/core-api/cmd/api/main.go:120` | 024 |
| Detection/local transcription are not independent durable stages | Optional-reader failure prevents the normal path from saving detected geometry | L | MED | `services/core-api/internal/sourceprocessing/name_reading.go:203`; `services/core-api/internal/sourceprocessing/tree_suggestions.go:864` | 024 |
| Text-only judging reopens the original source | Each reviewed name repeats source I/O and PDF preparation already done for its crop | M | MED | `services/core-api/internal/sourceprocessing/name_judge.go:376`; `services/core-api/internal/sourceprocessing/pdf_extractor.go:153` | 024 |
| The bench discards file-name frequency and truncates context | The ranker's file-count signal is effectively one and larger-file context is incomplete | M | MED | `apps/web/src/components/SourceSuggestionReview.tsx:230`; `apps/web/src/lib/arabicNames.ts:244` | 025 |
| Shadow evidence exists only if the browser receives it before save | Fast valid decisions can lose judge evidence despite a frozen candidate snapshot | L | HIGH | `apps/web/src/components/SourceSuggestionReview.tsx:508`; `apps/web/src/components/SourceSuggestionReview.tsx:614` | 025 |
| Deferral has no return path and region repair is absent | Difficult/mistaken work cannot be resumed or repaired in the bench | L | HIGH | `services/core-api/internal/sourceprocessing/tree_suggestions.go:398`; `apps/web/src/components/SourceSuggestionReview.tsx:1326` | 026 |

The grounded product directions are an inspectable local reading workbench
(variants, crop repair, explicit reread) and a durable preparation/operator tool
(capabilities, stage progress, resume/cancel, aggregate evidence diagnostics).
Both reuse existing storage and PostgreSQL jobs. Their tradeoffs are artifact
retention/coordinate management and additional revision/cohort semantics; the
plans make those constraints explicit before UI polish.

### Pipeline options considered and rejected

- Automatically enabling hosted vision to make the button work: the collection
  session deliberately keeps source images local; availability must reflect policy.
- Using Liquid `d1:3b` as a visual reader: it remains a bounded text-only judge.
- Treating 111 actions as 111 truth labels or relaxing the 300-decision gate:
  rejected/deferred actions are useful operational data but not name truth.
- Rebuilding an old missing judge result after its human answer was known:
  that could leak the answer through live file context and is not valid recovery.
- Adding new queue infrastructure or a model-shopping project first: the existing
  PostgreSQL job/lease and storage systems can support these concrete gaps.
- Replacing the work with cosmetic polish: the current high-value problems are
  unavailable tooling, incomplete candidate evidence, repeated work, and missing
  review return paths.

## Scope coverage

- Public identity/search/claim visibility and research-run authorization: plan 001 (done). Two residual items were deferred out of plan 001: `internal/research/workspace.go:437` reads person aliases without source scoping, and `identity.NormalizeArabicName` rewrites ة→ه while the disputed-claims index and claim-name search compare the normalized term against the raw name column, so ة-containing names never match.
- Non-atomic provenance/audit writes and accepted-suggestion domain application: plan 002 (done). Applying a typed change set requires a global write role (`collaborator`/`researcher`/`moderator`/`admin`), the same set `evidence.AddEvidence` uses; tree-review rights alone still allow reject/convert/accept-without-change-set. The shipped review panel sends no change set, so the UI cannot compose one yet and an acceptance still produces the open-question artifact.
- Upload formats versus text-only extraction: plan 003 (done). V1 accepts `text/*`, `application/json`, `application/xml` from one shared matrix; anything else is refused with `415` before storage or enqueue, content is sniffed rather than trusted, and a legacy queued binary file fails deterministically. `ARCHITECTURE.md:1127` still lists PDFs among stored object types — plan 009 should reconcile that line.
- Database integration, browser E2E, and AI quality baselines: plan 004 (done). `make verify` stays the fast gate; `make verify-full` is the acceptance gate and runs the complete Go suite through `tools/dbtestguard`, which refuses to run without `DATABASE_URL` and fails on a database-gated skip. `make e2e` runs 9 of the 10 declared V1 journeys in a browser; the invitation-acceptance journey is a recorded `test.fixme` until plan 010. The Python packaging fix in this plan also un-breaks `make install` and the `ai-research` CI job, which were failing on `main`.
- Phase 2 identity CRUD gap: plan 005 (done). People, person aliases, families, tribes, branches, places and generic entity relationships are writable through permission-checked routes; every mutation authorizes in-transaction and commits with its audit event. A created row is research-only, and `db/migrations/0039_reference_visibility.sql` gives the four reference families the visibility column they lacked (existing rows backfilled public). Merging/resolution, `family_aliases`/`tribe_aliases` writes and any living-person policy remain out of scope.
- Job leases, stale recovery, synchronous entity resolution/research agent: plan 006 (done). Claims carry a lease token and expiry, `Complete`/`Fail`/`Renew` and every persisting transaction are owner-checked, a 30s heartbeat cancels work that loses its lease, and entity resolution plus the research agent run as queued jobs consumed by `cmd/analysis-worker`. Known follow-ups: `researchagent/stages.go` `search_sources` cannot match a NULL source id, and a step can report one more evidence item than it wrote.
- N+1 source-review/history/map queries and payload sizes: plan 007 (done). Candidate review hydration, research-run authorization and the map filters are set-based or pushed into SQL (100 runs: 502 → 9 queries; 100 candidates: 104 → 6), the batched visibility verdicts are pinned to equal the single-id verdicts, and `db/migrations/0042_passage_lexical_trigram_index.sql` is the only index an `EXPLAIN` justified. Still open: candidate passage text is still inline because the panel renders every candidate, and `sourceprocessing.files`/`runs` still order by `created_at` with no tiebreak.
- Migration atomicity, `make verify`, S3/signed access, rate/abuse controls, dependency scanning, observability: plan 008 (done). Migrations take an advisory lock, apply each file with its checksum in one transaction, and refuse an edited applied file; `verify-full` also runs generated-code and migration drift. Scanners run via `make security-scan` and are clean, with one reviewed gitleaks exception for a non-credential literal in a test. `S3Store` was contract-tested only against a double; plan 011 has now run the same suite against a local MinIO, including a presigned URL fetched over plain HTTP, so the "a deployment must run one live presign before trusting it" residual is narrowed to a Cloudflare R2 presign that nothing has run yet. Rate limits are in-process and IP-keyed, so a multi-replica deployment needs a shared limiter. The upload quarantine hook was deliberately not built: no requirement in this repository specifies the policy.
- README/status matrix, environment truthfulness, GraphRAG/Neo4j measurement: plan 009 (done). The root `README.md` documents every gate, both stacks' ports and the environment by name; `docs/phase-status.md` marks Phases 0–24 with a code citation and the remaining acceptance gap; `docs/graph-benchmark.md` records the PostgreSQL measurement that keeps the Neo4j decision tied to evidence; `make ai-eval` now reports six groups. The three blockers the matrix names: GraphRAG has no labelled corpus to beat a vector-only baseline, a research query will cite a context that shares only a preposition, and the disputed-claims index cannot be searched by a ة name.
- OCR and what a reliable family-tree import needs: `docs/ocr-and-import-decision.md`, with the
  evidence and the reading-strategy recommendation in `docs/import-reading-strategy.md`. The
  Arabic PDF and import pilot is implemented as a **supervised** workflow: local Poppler and
  Tesseract page evidence, append-only human transcription revisions, occurrence-level chart
  drafts with a second-reviewer gate and a retraction ledger, and manual lineage-book
  assertions. No accuracy claim is supported. Two facts are now measured rather than projected:
  the raster line detector truncates positionally at 1,000 segments per page
  (`internal/sourceprocessing/line_detector.go:52`), which a 500-name chart exceeds in its name
  boxes alone; and the hosted-vision run reached 39 detections against the chart's 500-plus
  visible names with no branch proposals. Plan 018 builds the corpus and the adjudicated gold
  and runs the comparison the harness has never been given; plan 019 recovers connector geometry
  from the PDF's vector content for digital charts and removes the positional truncation. Both
  are gated on the annotation labour, which is the real critical path.

  **Progress:** the CORS `PUT` defect that made three source routes unreachable from the
  browser is fixed and tested; the `tools/ocr-eval` harness is wired into `make verify` and
  into CI, with a guard that fails when a private family document is tracked by git; the name
  carrier localiser and the per-crop reader are implemented, and the reading path has been
  proven end to end on the Jasser chart — **474 names on the drawn rendering and 493 on the
  photographic one**, four per cent apart and right at the decision document's "500-plus
  visible names" — and the hosted reader transcribes the isolated crops correctly. The
  reading loop is now closed: the suggestions are a first-class record with a review queue,
  four decisions (accept, correct, reject, defer), an append-only decision ledger, and an
  accuracy figure computed from those decisions, so the feature measures itself. A suggestion
  is never a person, and nothing is accepted without a named reviewer. What is still missing
  is the corpus and the gold, without which no accuracy number is available.
- Name-candidate adjudication and the local judge: plan 022. The workbench now derives canonical,
  compact, particle, and skeleton keys, ranks a bounded candidate pool with source and file
  evidence, and can abstain when no candidate is plausible. The append-only decision ledger
  stores what the reviewer saw: the shown candidates and their order, the selected candidate,
  the input method, normalizer and pool versions, and the decision duration. `make name-eval`
  replays those snapshots into recall@8, all-shown recall, no-candidate and all-wrong rates,
  top-1/top-3/MRR, typed rate, median time, source coverage,
  file-class/resolution/page/confidence/date splits, judge abstention, calibration, and a paired
  lift interval, while counting missing and stale snapshots separately. The bounded local Liquid
  `d1:3b` adapter, core client, config/judge endpoints, deterministic shadow sampling, separate
  active row, and optional tight-crop image path are implemented; `.env.example` enables the
  active row for local inference while `NAME_JUDGE_IMAGES=false` stays the default, and tests use
  a fake transport. The operator approved local inference, but the ≥300-decision shadow
  evaluation has not run, so no model ordering, percentage, quality, or adoption claim exists.
  The judge is a bounded opinion, not a reader or a normalizer; it cannot add a missing name, and
  its image mode remains an experiment. See
  [`docs/name-judge-privacy-and-evaluation.md`](../docs/name-judge-privacy-and-evaluation.md).
- Frontend defects the browser suite found (invitation token never reaching the page, wrong published-version number, inert place search): plan 010 (done). `make e2e` now covers all ten V1 journeys with zero fixme and zero skip, and its teardown removes the synthetic rows it created, so a run leaves the development database as it found it.
- The S3 adapter against a real endpoint, and the R2 cutover record: plan 011 (implemented, unmerged). The same contract suite now runs against an opt-in local MinIO (`make storage-up`, compose `storage` profile, nothing in the default stack starts it) and against a test double with no MinIO. It found two defects the double could not: every `Put` failed against a plaintext endpoint because the adapter's byte counter hid `Seek` from the AWS SDK, and the contract demanded a byte-identical presign that no real signer produces. `docs/storage-backends.md` records MinIO locally / Cloudflare R2 in production with the configuration difference cited to Cloudflare's own documentation, a seven-item cutover checklist that has never been run, and what a green MinIO run does not prove about R2.

## Dependency notes

- 002 depends on 001 because claim/question mutation authorization and redaction semantics must be settled before audit writes are made transactional.
- 004 depends on 001–003 because its acceptance fixtures must exercise the final visibility, provenance, and upload contracts.
- 006 depends on 003 and 004 so worker behavior is tested against the final format contract and real database harness.
- 007 depends on 001/002 so batching cannot accidentally bypass visibility or redaction rules.
- 009 depends on 006 so graph benchmarks measure the production job/worker shape.

## Findings considered and rejected

- Neo4j, Redis, OpenSearch, and Temporal introduction: not an immediate defect; `IMPLEMENTATION_PLAN.md:1619-1690` explicitly defers them behind measurement gates.
- Further broad graph algorithm breadth: not prioritized before the privacy, provenance, and V1 acceptance gaps above; `PRODUCT.md:1138-1151` calls graph results structural, not authoritative.
- Go toolchain upgrade: not included without verifying the repository's supported runtime policy and CI matrix first.

## Verification standing

- `make verify` — fast gate: lint, typecheck, unit tests, 34 pytest, doc-link check, build. No database, no browser.
- `make verify-full` — acceptance gate: migrations, generated-code drift, the complete Go suite through `tools/dbtestguard`, and `make ai-eval`. Needs a database: `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<db> make verify-full`.
- `make e2e` — 10 V1 journeys in a browser (28 specs), self-cleaning.
- `make security-scan`, `make graph-benchmark`, `make migration-check`, `make storage-up` for the rest.
- The database suite fails the build on a database-gated skip, on a leaked synthetic test user, and on a leaked fixture schema. `make db-sweep` removes residue a crashed run left behind.

## Known residuals (all deliberate, all recorded)

- `S3Store` is proven against MinIO, never against R2; see `docs/storage-backends.md` for the cutover checklist.
- Rate limits are in-process and IP-keyed: no cross-replica enforcement, and one budget per address behind NAT.
- No upload quarantine policy exists; no requirement in this repository specifies one.
- GraphRAG has no labelled corpus, so "multi-hop beats vector-only RAG" has no evidence path.
- The disputed-claims dictionary index cannot be searched by a name containing ة.
- Candidate passage text is inline, and `sourceprocessing.files`/`runs` order by `created_at` with no tiebreak.

## Plans 013-015: closing the plan against itself

A review of `IMPLEMENTATION_PLAN.md` against the code (not against `docs/phase-status.md`)
found the plan substantially complete — 21 of 25 phases implemented, 4 partial, 0 not
started, and 14 of the 18 V1 steps proved outright — with three plans left to write:

- **013** (done) closed the product gaps: four sites where a normalized term was compared
  against a raw column, the change-set composer V1 step 12 promised, and the two criteria the
  status document never mentioned. `docs/phase-status.md` now records all eighteen V1 steps as
  proved, and the only remaining limit is plan 014's.
- **014** (done) made hybrid retrieval measurable, and the measurement refused to conclude. The
  vector leg is real code (`retrieval.go:69-100`, combined at `rag_service.go:77-81`); a backfill
  now embeds the corpus, and 29 labelled Arabic questions measure three arms. Multi-hop recall@5:
  vector-only 0.118, hybrid 1.000, graph-augmented 1.000 — and **Phase 20 stays open**, because the
  only embedding provider this repository may use returns a SHA-512 digest, so the vector arm is a
  permutation and the difference is against a hash. `docs/retrieval-measurement.md` names the corpus
  it would take (~381 judged cases per arm) to answer the question for real.
- **015** (done) addressed Phase 18's cost criterion. `dawha_ai_cost_units` was a counter that could
  only ever be passed `0`; it now carries `{operation, route, model}` in **dawha work units** —
  declared weights, not money, because no provider is configured and the one permitted embedder
  hashes its input. The routing set grew 15 → 33 cases with per-case provenance now a gate, and
  `route_accuracy` fell 1.0 → 0.9697 with all four disagreeing cases kept. `make cost-report` runs a
  synthetic workload and says in its own output that it is not a measured reduction in spend:
  **the criterion stays OPEN**, with a 28-day window, a ≥1,000-query floor, and a 20% deep-call-rate
  threshold stated before the data exists.

## Deferred by decision, not by omission

- **Object storage in production is Cloudflare R2; MinIO is the local and verification backend.**
  `S3Store` is contract-tested against MinIO, including a presigned URL fetched over HTTP with
  a plain client. R2 has never been contacted; `docs/storage-backends.md` holds the cutover
  checklist.
- **Above the graph bound, which 200 candidate edges are selected is still uuid-ordered.** The
  fingerprint is now stable and the partition is order-independent, but `ORDER BY edge_id LIMIT 200`
  hands two builds of the same corpus different subgraphs. One `ORDER BY (source_id,
  depends_on_source_id, dependency_type)` would fix it with no schema change, and it is recorded as
  a decision rather than made, because it changes which edges a truncated graph returns.
- **Redis is not being introduced.** The rate limiter is in-process and IP-keyed, so a
  multi-replica deployment has one budget per replica and one per address behind NAT. That is a
  recorded limit, not an oversight, and it stays until it is explicitly asked for.
- **OCR** is discussed in `docs/ocr-and-import-decision.md` and is not on the critical path for
  a family-tree import.
