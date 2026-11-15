# Plan 028: Turn verified external sources into flagship atlas features

> **Executor instructions**: Do not weaken visibility, provenance, or the bounded
> response guarantees from plans 001/002/007/027. Every number the atlas draws stays
> server-derived through the visibility predicates (`ReferencePredicate`,
> `PersonPredicate`, `SourcePredicate`) — never computed in a component. External
> data is never `documented` without review, never redraws region geometry, and
> never publishes automatically. Every import carries a `sources` row with license
> and fetch provenance. The map rule still holds: a map that draws confidently is a
> map that lies fluently.

`docs/external-data-sources.md` is the source-of-truth for the facts below: which
sources were verified (Wikidata CC0 via SPARQL — exercised; GeoNames CC-BY-4.0 dumps;
PeriodO public domain JSON; WHG per-dataset licences + ORCiD-gated downloads; Pleiades
CC-BY-3.0 per-record `rights`; OpenITI per-text licenses + Zenodo DOI), and which are
unverified leads (Fihrist, MASADER) that must stay out of execution until re-verified.

## Status

- **Priority**: P1 (phases A–D: visible value, low risk) / P2 (phases E–F: pipelines)
- **Effort**: L total, staged; each phase S–M and independently shippable
- **Risk**: MED (new review surfaces in phases C/F), LOW elsewhere
- **Depends on**: 001, 002, 007, 027 (implemented); `docs/external-data-sources.md`
- **Category**: data-integration + provenance-UX
- **Planned at**: post-027 working tree (uncommitted), 2026-11-07

## Why this ends up cheap: what already exists

The leverage plan is cheap because 027 built nearly all the machinery the flagships
need. Recognizing the reuses is the plan's first principle — no surface is rebuilt
that a query, predicate, or grammar already answers:

| Reuse | Where | Feeds flagship |
| --- | --- | --- |
| Year window end-to-end (`fromYear`/`toYear` on `MapInput`+`MapResponse`, `mapFilters.timePredicate` on all four queries, dual-handle scrubber) | `services/core-api/internal/geography/service.go:120-212` | طبقات (era chips only) |
| Server-derived per-region count inside a single correlated subquery whose status/certainty are handled by the drawn-status grammar | `service.go:503-527`, scan at `:549-569` | طبقات counters |
| Role-scoping predicates for every entity kind | `internal/visibility/visibility.go:433-471` | all phases |
| Per-place map endpoint precedent | `router.go:309-310` (`GET /api/v1/map/places/{placeID}`) | المَتْن evidence endpoint |
| Suggestion-vs-decision contract (machine reading stored beside the reviewer's answer, measured confidence, occurrence uniqueness) | `db/migrations/0051_source_tree_suggestions.sql` | المُقارِن table shape |
| Passage + statement review vocabulary (`extraction_method`, `review_status` cycles) | `db/migrations/0003_research_and_jobs.sql:38-65` | المَتْن |
| Schema-ready movement table | `db/migrations/0006_geography.sql:38-57` | رِحْل |
| Crop viewer + review-lane UX patterns | `apps/web/src/components/SourceSuggestionReview.*`, journey 05/06 | المَتْن folio, المُقارِن lane |
| Atlas map + filters + inspector trio, arrowhead/status grammar, `map-vocabulary.ts` | `apps/web/src/components/Atlas*, map-vocabulary.ts` (027-C/D) | every frontend phase |

## Design stipulations (binding on every phase)

1. **Counter semantics.** `placeCount` stays *timeless* (visible places intersecting the
   ring — a city does not vanish because the window moved). Era-scoped numbers are
   **new, distinct fields**: `associationCount` and `personCount`, derived from
   `geographic_associations` whose `time_from/time_to` intersect the window. An empty
   era is a *truthful* reading ("no timestamped evidence we hold"), and the inspector
   must caption it that way.
2. **Import landing shape.** Every external import = `sources` row
   (`source_type='website'` for gazetteers; `'book'`/`'manuscript'` for texts; license +
   endpoint + fetch date + checksum in `metadata` jsonb) → review-gated rows
   (`platform_inferred` / `needs_review`). Nothing auto-publishes.
3. **Scripts, not screenshots.** All data acquisition lives in idempotent
   `infra/local/build-*.py` scripts that patch clearly marked seed blocks (the region
   polygon generator pattern), and pin integrity via `source_files.checksum_sha256`.
   Commit only small *filtered subsets*; the bulk originals stay external.
4. **No geometry from external sources.** Regions keep their hand-drawn rings clipped
   to the committed NE 50m landmass; `MaxRegionRingVertices = 160` untouched. External
   pins sanity-check rings — they never edit them.
5. **Journey discipline.** New e2e assertions stay *relative* (presence/ordering,
   never hard-coded counts), per the 027 precedent.
6. **Bounded lists.** Every NEW list-style endpoint (Phase C evidence, Phase D
   movement, Phase F review queue) carries the map's own contract: an explicit
   `LIMIT` bound plus a `truncated` flag — "everything" vs "what fitted" must never
   be ambiguous (the plans/007 discipline, extended beyond maps).

---

## Phase A — F0: reference layer from Wikidata (~1 day, P1)

**Goal**: replace hand-coded reference captions with a deterministic artifact, so the
atlas stops looking bare without any DB/API change.

**Optimization**: generate a committed JSON artifact, not rows. Zero runtime network,
zero migrations, minimal component churn — `map-reference.ts` keeps its shape and
consumes `apps/web/src/assets/reference-places.json` (Q-ids, names ar/en, `P625`
point, short caption), header comment lists fetch date + SPARQL source + CC0.

**Implementation**:
- `infra/local/build-reference-layer.py`: curated Q-id list (seas: البحر الأحمر،
  الخليج العربي; mountains: جبل أحد، طهيق/طويق، سرو/شار mountains; canonical cities
  مكة، المدينة، الطائف، البصرة، الكوفة…) → one batched SPARQL GET (verified pattern,
  `format=json`, Arabic labels, `LIMIT` bounded, meaningful User-Agent).
- Regenerate → diff artifact → typecheck. Entry shape stays what
  `map-reference.ts` renders today so no atlas component changes.

**Acceptance**: artifact committed with header provenance (JSON import is safe:
`apps/web/tsconfig.app.json` already enables `resolveJsonModule`); `npm run build` +
typecheck green; atlas renders reference captions in their own visual register — a
reference pin must never read as a documented place pin under the 027 status grammar;
journey 08 stays green with no hard-coded caption assertions (its canvas assertion
measures *paint presence by compressed size*, spec lines 37–38, so added captions
cannot flip it).

## Phase A2 — صحيفة الأثر: license-trust dashboard (~2 days, P1)

**Goal**: make license cleanliness visible before any volume import starts.

**Implementation**:
- `GET /api/v1/admin/data-hygiene`: per `sources` row — license presence
  (`metadata->>'license'`), fetch date, checksum age, dependent row counts
  (`source_files`, passages, associations by status), missing-license flags;
  per-text `dependency_status` rollups. **Guard reality (verified)**: no admin-only
  routes exist (`httpapi/router.go` returns zero matches), so the gate reuses the
  authorization pattern of the nearest privileged handler
  (`identityHandler.updatePlace`, router.go:365) — a role check inside the handler,
  consistent with the codebase, not a new middleware invention.
- Seed one fully-documented demo source row (license, edition, metadata) beside a
  license-missing one, so the demo shows both states.
- Small admin UI card; nothing else.

**Acceptance**: non-admin gets the standard forbidden envelope; seeded demo flags at
least one license-missing source; unit test pins the query shape.

## Phase B — طبقات: era chips + era counters (3–4 days, P1)

**Goal**: the atlas reads as a time machine: era chips that drive the existing window,
and per-region readouts that show how knowledge thickens per era.

**Almost-free base**: the year window already flows `scrubber → fromYear/toYear →
timePredicate → all four queries → response echo`. What is missing is *era-scoped
evidence counters* and the era vocabulary.

**Implementation**:
- **API (counters)**: `Feature` gains `associationCount`/`personCount` (int, omitempty
  — same contract as `placeCount`). The region subquery (`service.go:503-527`) is one
  correlated subquery returning three counts in one logical scan:
  - `placeCount` — unchanged (timeless geometry intersection),
  - `associationCount` — associations at visible places intersecting the ring whose
    `time_window` intersects `fromYear/toYear` (reuse `timePredicate("ga.time_from",
    "ga.time_to")` inside the subquery — the year window the query already carries),
  - `personCount` — same, with `Policy.PersonPredicate(params, "…")` applied so the
    count never exceeds what the caller may read (`visibility.go:446`).
  All four queries gain the two `NULL::bigint` columns to keep column counts aligned —
  same surgery the `placeCount` change performed.
- **Vocabulary**: `apps/web/src/lib/atlas-periods.ts` — a curated artifact mapping era
  labels (الراشدون، الأمويون، العباسيون…) → `[fromYear, toYear]`; PeriodO-derived
  windows reviewed once and committed (public-domain source, editorial mapping ours;
  header notes the PeriodO snapshot date).
- **Frontend**: era chips in `AtlasFilters.tsx` that set the existing scrubber values
  (one control, not two); `AtlasInspector.tsx` region readout becomes three captioned
  rows: مواقع (timeless), شواهد مؤرَّخة في العصر, أعيان موثَّقون — each with the
  derivation caption ("مشتق من سجلات تقرؤها"، و"لا وثيقة مؤرَّخة في هذا العصر" is
  stated, never silently zeroed).
- **Calendar stipulation**: era windows are CE year integers — matching the existing
  predicates exactly (`EXTRACT(YEAR FROM …)` against CE `date` columns); the artifact
  carries only the Arabic era label + window. Hijri display/conversion is explicitly
  out of this phase (a later display concern that needs no SQL change).
- **Regions under a window**: seeded regions carry NULL `valid_from/to`, and the
  existing `timePredicate` deliberately keeps undated rows visible (its own comment,
  `service.go:197-204`). So regions do not fade under an era — the era manifests in
  the counters. That is the intended truthfulness, and the inspector copy must say so.
- **Tests**: extend `TestRegionPlaceCountCountsOnlyWhatTheReaderMaySee` with a windowed
  sibling whose comparator is an *independent SQL derivation in the test harness* (the
  `evaluatePredicate`/fixture pattern): the count must equal what a hand-written query
  over the same reader-visible places/associations yields for the same window; journey
  08 adds era-chip interaction with relative assertions only.

**Acceptance**: disputed/`disputed` layered queries stay correct under a window;
counters vanish (`omitempty`) when zero; inspector copy names its derivation; journey
green twice in a row.

## Phase C — المَتْن: the dossier pilot (4–6 days, P1)

**Goal**: hand-reviewed classical evidence readable on place pages, proving the
readout shape before any extraction pipeline exists.

**Implementation**:
- **API**: `GET /api/v1/places/{placeID}/evidence` in the geography/httpapi packages —
  reader rule: `spatial_evidence.review_status='accepted'` AND the owning source is
  readable by the caller (`SourcePredicate(params, "s.id")`) AND the place is readable;
  returns passage text (`source_passages.text_ar`, `page_number`, `locator_ar`),
  source title/edition, and the association it evidences. Moderators additionally
  request `?include=needs_review` (status check, path-guarded).
- **Pilot seeds**: real upload first — a reviewer uploads معجم البلدان page excerpts
  through the existing upload flow (journey-05 lane), so `sources` + `source_files`
  (checksum, storage key, real bytes) are honest. Then
  `infra/local/seed-dossier-pilot.sql` adds, per pilot place: `geographic_associations`
  rows **first** — `spatial_evidence` cannot exist without one (the 0006 CHECK requires
  an association or migration event) — created as `documented`, hand-reviewed rows
  (certainty `approximate`, `source_id` set, `created_by` = reviewer); then
  `source_passages` (sequence/page/locator, `extraction_method='manual'`), accepted
  `source_statements`, and `spatial_evidence` rows tying those associations → passages
  for five public demo places (مكة، المدينة، الطائف، اليمامة/الهفوف، العلا — actual
  ids read from `001_demo.sql`). Nothing bypasses review.
- **Frontend**: `AtlasInspector.tsx` gains an «الأدلة» tab — folio rows (passage text,
  locator, source citation, and a link that deep-links into the **existing page
  evidence workbench route** for that source page — the journey-05/06 surface; reuse
  the route, not a bespoke crop module); `needs_review` rows render behind a
  moderation band, embedded in the same component, not a second system.
- **e2e**: extend journey 08: reader sees only accepted rows; moderator sees the
  pending row; relative assertions.

**Acceptance**: a private source's evidence is invisible to a plain reader even though
the place is public — pinned by a unit test; folio link opens the real stored page in
the existing workbench; the evidence list is bounded (`LIMIT` + `truncated`), per
stipulation 6.

## Phase D — رِحْل: biographical movement pilot (3–4 days, P1, parallelizable with E)

**Goal**: a person's documented مسالك on the atlas, review-gated.

**Implementation**:
- **API**: `GET /api/v1/persons/{personID}/movement` — `migration_events` where
  `subject_type='person'`, `status='documented'`, scoped by `PersonPredicate` +
  `SourcePredicate` (source must be readable) + from/to place readability; ordered by
  `time_from`; bounded (`LIMIT` + `truncated`, stipulation 6); returns both endpoints'
  coordinates plus the event's source citation. Implemented in the geography package
  (it already imports visibility and the map grammar); routed under `/persons/` in
  `router.go`.
- **Pilot seeds**: `infra/local/seed-movement-pilot.sql` — ~10 documented events for
  one demo person between existing public demo places, sourced from the Phase-C
  upload, `created_by` = reviewer.
- **Frontend target (corrected by review)**: there is **no person detail page**
  (`apps/web/src/routes/` holds tree/tree-route/entity-resolution and the atlas — no
  `person.tsx`), so the movement renders where it is honest and cheapest: a **subject
  layer on the atlas** — `/places?subject=<personID>` fetches the movement endpoint and
  `AtlasMap` draws the route overlay (arrowhead grammar from `map-vocabulary.ts`
  exists), segments colored by event **status** in the shared vocabulary rather than
  one uniform "route gold". Empty state is honest: «لا مسار موثّق بعد». When a person
  page exists someday, the endpoint is already waiting to be embedded.
- **e2e**: extend journey 08 (the atlas journey): the subject layer renders only
  readable events, named-surface assertions only — relative.

**Acceptance**: a documenting source becoming private hides its events (unit test);
routes render only between places the reader may see.

## Phase E — مِفْتاح الأسماء: variants + variant benchmark (2–3 days + 1–2 days, P2)

**Goal**: search speaks يَثْرِب → المدينة، حَجَر → الهفوف in both directions — as data,
with zero search-code changes where the trigram union already reads
`historical_place_names` (migration 0042's matched-name branch; verify with the
benchmark, not faith).

**Implementation**:
- A small migration **first** (verified gap): add the dedupe guard
  `CREATE UNIQUE INDEX historical_place_names_unique ON historical_place_names
  (place_id, name_type, normalized_name_ar)` — the table (0002:135-145) has **no
  uniqueness at all**, and two pipelines (this one and Phase F's alias accept) would
  race without it.
- `infra/local/build-name-variants.py`: fetch GeoNames `SA.zip` + `alternateNamesv2`
  archive (CC-BY-4.0 attribution recorded), filter to Arabic alternate names of a
  *curated shortlist* of our public places, derive `name_type` from the GeoNames flags
  themselves (`isHistoric` → `historical`, otherwise `alternative`), dedupe against
  `normalized_name_ar`, and emit a small committed subset → seed inserts
  `historical_place_names` + one `sources` row (CC-BY note, URL, fetched-at, sha256
  of the fetched subset). Fallback if the archive is unwieldy: the per-id
  alternatenames API within the 10k-credit/day quota.
- **Benchmark**: add 10–12 variant→canonical retrieval fixtures to the retrieval
  measurement harness (per `docs/retrieval-measurement.md`), wired into `make verify`
  alongside the existing guards (exact target name pinned at execution from the
  Makefile's retrieval/name-eval targets — `name-pipeline-guard-test` is the current
  map-adjacent guard).

**Acceptance**: benchmark row exists and is green pre/post import (no regression,
measured recall gain recorded in the run row); every imported name row carries a
`source_id`; duplicates impossible via the new partial unique index. Search pickup is
data-only by design and now *cited*: the place branch already scores
`historical_place_names.normalized_name_ar ILIKE` at 85 and trigram similarity at 70
(`search/service.go:242`) — the benchmark certifies ingestion, it changes no search
code.

## Phase F — المُقارِن: pairing-adjudication lane (5–7 days, P2)

**Goal**: no "import and pray". Machine proposes pairings; reviewers adjudicate;
provenance stamps the decision.

**Implementation**:
- **Tables** (new migration, contract copied from the suggestion table):
  - `external_place_candidates` — the machine reading: `source_id` (external gazetteer
    source), `external_key` (GeoNames id / Wikidata Q-id), `occurrence_id` unique,
    `our_place_id` nullable, `normalized_candidates text` + `coordinates geometry(Point,4326)`,
    *measured* score (name trigram + distance components kept apart, never one vibe
    number), `status IN ('suggested','accepted','alias_only','rejected','deferred')`,
    and the reviewer's answer stored *beside* the proposal (`decided_by/at/note_ar`) —
    the 0051 contract verbatim.
  - `place_external_ids` — the accepted crosswalk: `place_id`, `source_id`,
    `external_key`, reviewer columns. One table serving المُقارِن and the future
    «معرّفات خارجية» place-page block.
- **Offline builder**: `infra/local/build-pairings.py` — candidates for public demo
  places from the GeoNames/Wikidata subsets already imported, ranked
  (trigram ∩ 25 km PostGIS nearest); writes `suggested` rows via seed. Nothing calls
  the network at review time.
- **API**: moderator-only review endpoints — list by status, accept (effect chosen
  explicitly per row: *alias only* → a small new write path for `historical_place_names`
  (**review note: no Go service writes that table today** — `search/service.go:242`,
  `dictionary/service.go:343`, `sourceprocessing/processor.go:745` only read it, and
  dictionary/search surfaces start displaying aliases the moment rows exist), *link* →
  `place_external_ids`, or *reject*; coordinate deltas are stored as a *proposal*
  applied explicitly with a provenance note through the existing
  `PATCH /api/v1/places/{placeID}` (`identityHandler.updatePlace`, router.go:365) —
  never silently overwritten by an accept action).
- **Frontend**: lane styled on the existing suggestion-review screen (journey-06
  pattern); place page gains «معرّفات خارجية» (read-only) once Phase F lands.

**Acceptance**: no candidate row is ever rendered as fact; accept→effect mapping
unit-tested; crosswalk visible on place pages only for reviewer-visible sources.

## Phase G — deliberately deferred (separate plan, after Phase F)

The OpenITI volume pipeline (roadmap Stage 4: معجم البلدان as a full check'd corpus
upload → OCR/name extraction → platform_inferred association farming) and WHG
crosswalk *downloads* (ORCiD-gated) land in a follow-on plan once the dashboard
(A2) shows license state and the lane (F) exists to receive the volume. Per-text
license verification is a hard precondition logged in that plan.

## Execution order

| # | Phase | Effort | Depends on | Value now |
| --- | --- | --- | --- | --- |
| 1 | A reference layer | 1d | — | atlas stops bare |
| 2 | A2 license dashboard | 2d | — | governance before imports |
| 3 | B طبقات counters + chips | 3–4d | — | era narrative visible |
| 4 | C المَتْن dossier pilot | 4–6d | A2 governance | provenance readable |
| 5 | D رِحْل movement pilot | 3–4d | C upload | movement layer on the atlas |
| 6 | E variants + benchmark | 3–5d | — (parallel w/ D) | search jumps |
| 7 | F pairing lane | 5–7d | E datasets, A2 dashboard | disciplined scale |

Total ≈ 3–4 weeks of focused work; every phase ships and demo-verifies alone.

## Verification gates (unchanged posture)

- `make verify` extended exactly where phases touch it (Phase B: geography queries;
  Phase E: retrieval fixtures; Phase F: review endpoints).
- Journey 08 extended for era chips (B), dossier tab (C), and any count-bearing change —
  relative assertions only; green twice consecutively before a phase is called done.
- `docs/phase-status.md` rows and the plans/README status row updated per phase; no
  phase closed on a red gate.
