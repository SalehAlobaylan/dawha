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
| 007 | Batch list hydration and bound map queries | P2 | L | 001, 002 | TODO |
| 008 | Harden migrations, verification, storage, security, and telemetry | P1 | L | 004 | TODO |
| 009 | Document current state and benchmark graph limits | P2 | M | 004, 006 | TODO |
| 010 | Fix the V1 frontend defects the browser suite exposed | P1 | S | 004 | DONE (merged `3302f1e`) |

Status values: `TODO | IN PROGRESS | DONE | BLOCKED | REJECTED`.

## Scope coverage

- Public identity/search/claim visibility and research-run authorization: plan 001 (done). Two residual items were deferred out of plan 001: `internal/research/workspace.go:437` reads person aliases without source scoping, and `identity.NormalizeArabicName` rewrites ة→ه while the disputed-claims index and claim-name search compare the normalized term against the raw name column, so ة-containing names never match.
- Non-atomic provenance/audit writes and accepted-suggestion domain application: plan 002 (done). Applying a typed change set requires a global write role (`collaborator`/`researcher`/`moderator`/`admin`), the same set `evidence.AddEvidence` uses; tree-review rights alone still allow reject/convert/accept-without-change-set. The shipped review panel sends no change set, so the UI cannot compose one yet and an acceptance still produces the open-question artifact.
- Upload formats versus text-only extraction: plan 003 (done). V1 accepts `text/*`, `application/json`, `application/xml` from one shared matrix; anything else is refused with `415` before storage or enqueue, content is sniffed rather than trusted, and a legacy queued binary file fails deterministically. `ARCHITECTURE.md:1127` still lists PDFs among stored object types — plan 009 should reconcile that line.
- Database integration, browser E2E, and AI quality baselines: plan 004 (done). `make verify` stays the fast gate; `make verify-full` is the acceptance gate and runs the complete Go suite through `tools/dbtestguard`, which refuses to run without `DATABASE_URL` and fails on a database-gated skip. `make e2e` runs 9 of the 10 declared V1 journeys in a browser; the invitation-acceptance journey is a recorded `test.fixme` until plan 010. The Python packaging fix in this plan also un-breaks `make install` and the `ai-research` CI job, which were failing on `main`.
- Phase 2 identity CRUD gap: plan 005 (done). People, person aliases, families, tribes, branches, places and generic entity relationships are writable through permission-checked routes; every mutation authorizes in-transaction and commits with its audit event. A created row is research-only, and `db/migrations/0039_reference_visibility.sql` gives the four reference families the visibility column they lacked (existing rows backfilled public). Merging/resolution, `family_aliases`/`tribe_aliases` writes and any living-person policy remain out of scope.
- Job leases, stale recovery, synchronous entity resolution/research agent: plan 006 (done). Claims carry a lease token and expiry, `Complete`/`Fail`/`Renew` and every persisting transaction are owner-checked, a 30s heartbeat cancels work that loses its lease, and entity resolution plus the research agent run as queued jobs consumed by `cmd/analysis-worker`. Known follow-ups: `researchagent/stages.go` `search_sources` cannot match a NULL source id, and a step can report one more evidence item than it wrote.
- N+1 source-review/history/map queries and payload sizes: plan 007.
- Migration atomicity, `make verify`, S3/signed access, rate/abuse controls, dependency scanning, observability: plan 008.
- README/status matrix, environment truthfulness, GraphRAG/Neo4j measurement: plan 009.
- Frontend defects the browser suite found (invitation token never reaching the page, wrong published-version number, inert place search): plan 010 (done). `make e2e` now covers all ten V1 journeys with zero fixme and zero skip, and its teardown removes the synthetic rows it created, so a run leaves the development database as it found it.

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
