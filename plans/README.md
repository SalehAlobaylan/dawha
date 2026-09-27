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

Status values: `TODO | IN PROGRESS | DONE | BLOCKED | REJECTED`.

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
