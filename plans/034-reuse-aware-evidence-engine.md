# Plan 034: Reuse-aware evidence engine and reference check (محرّك الأدلة الواعي بالاستنساخ)

> **Executor instructions**: the engine's one honest contribution is *independence
> correction*: classical sources copy one another, so corroboration must be counted over
> textual lineages, not passages. Nothing here re-grades or deletes existing rows; it
> recalculates how tallies are counted and exposes a bounded read-only verification
> endpoint. Claims are never merged; tallies are never merged; the checksum/dossier rules
> from `docs/external-data-sources.md` remain the import law.

Sources of truth: `docs/tribal-territory-solutions.md` §S5/v2 (KITAB/passim text-reuse,
Scite supporting/contrasting + reference check, bounded read-only MCP precedent) and the
fetched S5/S2 corpora in `docs/tribal-territory-sources.md`.

## Status

**DONE — all four phases shipped and gated (2026-11-19).**

- **Phase A (clustering)**: migration 0073 (`text_reuse_clusters` + `text_reuse_members`,
  the membership invariants at the door: FK to passages, no cluster <2 members, the
  cluster/passage unique pair); TWO honest scripts, split by the infra tree's own
  discipline (jobs touch data, gates touch checks): `build-reuse-corpus.py` (THE JOB —
  the only script in the lane that reads the live database, through the documented
  container path) and `build-reuse-clusters.py` (the ENGINE — pure/offline/stdlib
  only, reading the committed artifacts, normalizing with the identity engine's own
  three keys mirrored token-for-token, shingling word 5-grams, Jaccard at 0.55 with
  transitive linking bounded per source-pair (≤10/pair), same-source passages NEVER
  linking (a source quoting itself is its own redaction lineage)), writing the
  clusters/members artifacts + sha256 sidecars + the spot-check report that states
  its own sample limits verbatim.
- **Phase B (tallies)**: `claimBadgeSelect` now counts distinct INDEPENDENT lineages
  (`COALESCE(cluster_id, 'src:'||source_id)) — a reuse cluster's copies count ONCE),
  `TallySemanticsIndependence = "independence_v1"` rides every response, and the
  caption says what the numbers now count. The pinned comparator:
  `TestBadgeTalliesCollapseReuseLineages` — three supporting passages of three
  sources with أ+ب in one cluster → supporting = **2, not 3**.
- **Phase C (reference check)**: `POST /api/v1/research/reference-check` — the
  bounded 2,000-char input (longer = a validation error, never a truncated read),
  LEXICON-ONLY extraction (the dictionary's five entity kinds, longest-first, the
  identity engine's canonical forms), per-span entity + kind + the explicit
  «غير معروف لنا» honesty class (an unmatched span is a REPORTED class, never a
  silent zero), the caller's own visibility predicates composed inside (anonymous =
  the published rows only). The dictionary's own optional-actor gate.
- **Phase D (bench)**: `evaluation/reuse_cases.jsonl` (10 hand-reviewed Arabic
  pairs: 3 identical / 2 paraphrased / 5 distinct, each with its reviewer + reason)
  + `evaluation/reuse.py` (the DETERMINISTIC verdict machine the engine itself runs
  — no LLM, per the plan's own stipulation) wired into the harness's GROUPS +
  thresholds (version-bumped `2026-11-19.1`, the honest rule kept: recall sits at
  the measured 0.6667 with its own KNOWN_DEFECTS row — the short-passage shingle
  floor, named, with its fix path). `pnpm`-green: ruff, pytest 17/17.
- **Deferred by design (the plan's own row)**: the MCP exposure of the certified
  read path — a separate plan if requested.
- **State**: `make verify` green, `make verify-full` green (twice); the
  per-tribe independence report lands at `docs/benchmarks/reuse-independence.json`
  (its sample limits stated verbatim); everything left UNCOMMITTED.

- **Priority**: P2 (trust layer; consumers are 031-B badges and 032-D dispute views)
- **Effort**: L total, staged
- **Risk**: MED resolved (the multi-key normalizer is the clustering's own canonical
  form; every cluster preview is human-checkable; the tallies' semantics is a
  versioned field, never silent)
- **Depends on**: 030-B/C (passage corpus), 031-A (badge consumers), 014 (measured retrieval posture) — all done
- **Category**: analysis + retrieval + governance
- **Planned at**: post-029 working tree, 2026-11-15

## First principle: what already exists

| Reuse | Where | Feeds |
| --- | --- | --- |
| Multi-key Arabic normalizer (canonical/skeleton/particle keys) | `services/core-api/internal/sourceprocessing/name_matching_keys.go` | reuse clustering keys |
| Passage statements + evidence rows with checksums | `docs/external-data-sources.md` rules, `db/migrations/0003_research_and_jobs.sql` | the corpus the engine reads |
| Evidence endpoints (place/tribe) that tallies flow through | `services/core-api/internal/geography/place_evidence.go`, plan 031-A | B: recomputed tallies |
| Evaluation harness discipline (fixtures, thresholds, thresholds-file pattern) | `services/ai-research/evaluation/` (`evaluation_cases.jsonl`, `thresholds.py`), `tools/ocr-eval/` | D: reuse-detection bench |
| Bounded read-only handler pattern + rate limits (in-process) | existing handlers + plan-008 limits | C: reference check |

## Features this plan delivers

| Feature | Surface | Phase |
| --- | --- | --- |
| سُلَّم الاستنساخ (text-reuse clusters): near-verbatim passage lineages across sources | new table + offline script | A |
| عدّ الأدلة بالاستقلال (independence-weighted tallies) recalculating 031-B/032-D | evidence endpoints | B |
| فحص مقتبس (reference check): paste text → per-claim status with spans | bounded read-only endpoint + page | C |
| المقاييس (bench): reuse precision/recall fixtures; independence report per tribe | evaluation harness | D |
| (Deferred decision, not omission) MCP exposure of the certified read path — Scite precedent | separate plan if requested | — |

## Binding stipulations

1. **Clusters are evidence-neutral**: a cluster links passages; it asserts «هذه النصوص
   متقاربة نصياً» and nothing semantic. Adjudicating *why* they agree stays human.
2. **Tallies change shape, not meaning**: badge counters switch from `sources`-distinct
   (031-B interim) to `clusters`-distinct + singletons; caption updates from «قبل تحليل
   الاستقلال» to the computed semantics; the semantic change is versioned in the response
   (a `tallySemantics` field), never silent.
3. **Bounded everything**: reuse clustering runs offline; the reference check accepts a
   bounded character budget and returns bounded results with in-process rate limits (008).
4. **No visibility laundering**: the reference check reads only what the
   caller's predicates allow (visibility predicate composition, verbatim).

## Phase A — reuse clustering (4–5 days, first)

`infra/local/build-reuse-clusters.py`: over `source_passages` (+ OpenITI-era classical
uploads), normalize (existing keys), shingle (word 5-grams), cluster via Jaccard/threshold
with transitive linking bounded per-source-pair; write `text_reuse_clusters` +
`text_reuse_members` (passage FKs + similarity + key excerpts). Cluster previews are
human-checkable (excerpts side by side) before tallies consume them.

**Acceptance**: seeded corpus produces clusters with a precision spot-check report (>0.9
agreement on a 30-cluster human sample recorded); script idempotent; membership invariants
pinned by tests (every member carries its passage checksum; no member without a passage,
no cluster without ≥2 members).

## Phase B — independence-weighted tallies (3 days, P2, needs A + 031-B)

Evidence endpoints recompute: supporting = distinct *independent* lineages (singleton
passage counts as its own lineage); contradicting likewise; the `tallySemantics` field
declares the version. 031 badges and 032 dispute views read the new field without UI
changes beyond the caption.

**Acceptance**: unit test — three passages, two in one lineage → supporting=2 (not 3);
independent SQL derivation comparator (the pinned pattern); response versioning field
present.

## Phase C — reference check (3–4 days, P2, needs A/B)

`POST /api/v1/research/reference-check` (read-only, role-gated like the dictionary,
bounded input ~2k chars, in-process rate limit): extracts geographic/tribal claims
(lexicon + normalization keys; no LLM in this phase), matches against claims/tallies,
returns per-claim: matched span, matched claim id, status, tallies, and the «غير معروف
لنا» honesty class for unmatched spans (never a silent zero).

**Acceptance**: unmatched ≠ matched-by-nothing silently (explicit class in response);
private-source statuses never leak into results for lower roles (unit test); load bounded
(500ms budget on the seeded stack, measured).

## Phase D — bench + report (2 days, P2, needs A)

Reuse-detection fixtures into the evaluation harness (Arabic hand-labeled pairs:
identical / paraphrased / distinct), thresholds file discipline; per-tribe independence
report generated into `docs/benchmarks/` after each cluster rebuild.

**Acceptance**: `make verify` runs the fixture gate; the report states its own sample
limits (the 014/015 honesty pattern).

## Execution order

| # | Phase | Effort | Depends on |
| --- | --- | --- | --- |
| 1 | A clustering | 4–5d | 030-B corpus |
| 2 | D bench | 2d | A |
| 3 | B tallies | 3d | A, 031-B |
| 4 | C reference check | 3–4d | B |

Total ≈ 2–3 weeks of focused work; each phase ships and measures independently.

## Verification gates

`make verify` per phase (new table migrations + endpoints + fixtures); `make verify-full`
before close; journeys untouched except C's page (new route, new journey section,
relative-only); phase rows in `docs/phase-status.md` + `plans/README.md`.
