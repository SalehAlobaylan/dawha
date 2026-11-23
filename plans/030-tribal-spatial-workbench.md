# Plan 030: Tribal spatial extraction & reconciliation workbench (ورشة الاستخلاص المكاني القبلي)

> **Executor instructions**: Do not weaken visibility, provenance, or the bounded-response
> guarantees from plans 001/002/007/027. Every extracted tribal claim lands as
> `platform_inferred` and reaches `documented` only through a named reviewer's acceptance
> recorded in the decision history. Every imported page carries a `sources` row with license,
> endpoint, fetch date and `checksum_sha256`. Geometry stipulation (binding): tribal dira
> polygons are **new** `historical_regions` rows with `certainty='approximate'`, clipped to the
> committed NE 50m landmass, `MaxRegionRingVertices = 160` untouched; accepted polygons never
> edit the existing seeded classical fivefold regions. A map that draws confidently is a map
> that lies fluently — the certainty grammar travels with every row this plan creates.

Sources of truth: `docs/tribal-territory-sources.md` (verified corpus, licenses, the live
extraction demo) and `docs/tribal-territory-solutions.md` §S2/v2 (mechanism, external
platform precedents: Allmaps toponym bootstrap, Recogito authority-tied annotation,
Tasking-Manager task grids, Label-Studio accept/reject queues).

## Status

- **Priority**: P1 (the factory every other tribal plan consumes)
- **Effort**: L total, staged; phases ship independently
- **Risk**: MED (new review surfaces; vision-crop reliability on dotted boundaries)
- **Depends on**: 001, 002, 007, 027, 028 (all DONE); Phase B needs the license dashboard (028-A2) green first
- **Category**: data-integration + geospatial + review-UX
- **Planned at**: post-029 working tree, 2026-11-15

## First principle: what already exists

| Reuse | Where | Feeds |
| --- | --- | --- |
| Pairing adjudication contract (candidates + decision history, reviewer answer beside proposal) | `db/migrations/0063_external_place_pairings.sql` (`external_place_candidates`, `place_external_ids`), `db/migrations/0065_pairing_decision_history.sql` | the tribe-lane tables copy this contract verbatim |
| Place-evidence endpoint pattern (accepted-only default, moderator include lane, bounded) | `services/core-api/internal/geography/place_evidence.go` | evidence rows this plan creates |
| Map status/certainty grammar + arrowheads | `apps/web/src/lib/map-vocabulary.ts` | dira polygon register |
| Offline builder pattern (idempotent `infra/local/build-*.py`, provenance header, committed filtered subsets) | `infra/local/build-name-variants.py`, `infra/local/build-pairings.py` | every phase's scripts |
| OCR stack (Poppler + Tesseract `ara`/`eng`), page evidence, durable jobs | `services/core-api/internal/sourceprocessing/` (local_crop_reader, name_reading), job system from plan 006 | gazetteer text lane |
| Hosted vision locate→read-crops adapter, off by default, per-source approval | `services/ai-research/app/chart_vision.py`, plan 023 capabilities | map-label lane |
| Review workbench UX (queue, side-by-side crop, accept/correct/reject/defer) | `apps/web/src/components/SourceSuggestionReview.tsx` (journey-05/06 pattern) | the workbench UI |

## Features this plan delivers (the application enablers)

| Feature | Surface | Phase |
| --- | --- | --- |
| قاموس قبائل أولي (tribe name harvest) — flagship tribes + aliases + external Q-id crosswalk as reviewed relationships | `tribes`, `tribe_aliases`, `entity_relationships`, seed scripts | A |
| مكتبة مصادر معلَّمة — Lorimer / Gazetteer of Arabia / Handbook of Arabia pages as checksummed sources with OCR passages | `sources`, `source_files`, `source_passages` | B |
| توصيف أماكن نصي — toponym spans in OCR text → pairing candidates bound to our authority (places/tribes) | `external_place_candidates` reuse + tribal candidates table | C |
| معايرة اللوحات — gazetteer coordinates become automatic control points; plate transforms stored with RMS report | `sources.metadata` + validation tests | D |
| استخلاص الملصقات — vision-extracted tribal labels → pixel→coordinate projection → dira candidates | new candidates table + jobs | E |
| ورشة المراجعة — tile tasks, accept/correct/reject/defer, retirement at 2 independent confirmations | workbench page + moderator endpoints | F |
| القياس — throughput, acceptance rate, dira coverage rows | `make` target + `docs/benchmarks/` artifact | G |

## Binding stipulations

1. **Landing shape**: extraction = candidate rows only. Accept → `historical_regions`
   (`certainty='approximate'`) + `geographic_associations` (`status='documented'`, `created_by`
   = reviewer) + `spatial_evidence` rows tied to the exact passages. Reject/defer are first-class.
2. **Independence note on candidates**: two candidates from the *same* source are one
   confirmation; retirement requires confirmations from *independent* sources (the plan-034
   reuse graph later refines this; until then "independent" = distinct `sources` rows).
3. **Bounded lists**: every list endpoint of this plan carries `LIMIT` + `truncated`
   (the 028 stipulation, extended).
4. **Scripts, not screenshots**: all acquisition in idempotent `infra/local/build-*.py`;
   commit only filtered subsets; bulk scans stay external with fetch provenance recorded.
5. **No auto publish**: nothing here publishes to anonymous readers until an explicit
   visibility flip is reviewed.

## Phase A — قاموس قبائل أولي (2–3 days, P1)

Harvest the flagship tribes the sources doc verified (حرب، شمّر، عنزة، عتيبة، قحطان، مطير،
بني خالد، بني تميم، غامد، يام، هوازن، مضر): `infra/local/build-tribe-seeds.py` pulls Wikidata
Q-ids + Arabic labels/aliases (CC0, SPARQL pattern already exercised in 028-A), ar-wiki
category membership as *provenance context only* (not a claim source); emits a small
committed seed SQL. Provenance: each tribe gets an `entity_relationships` row
(`predicate='external_id'`, object = Q-id, `source_id` = the Wikidata sources row, status
`platform_inferred`) — no schema change to `tribes`.

**Acceptance**: seeds idempotent (verified live: applied twice, second pass a no-op);
every Q-id carries source + fetch date in the artifact and the `sources` row metadata (the
per-tribe *database* crosswalk lands with 030-C's lane tables — `entity_relationships`
cannot hold a Q-id: its `object_id` is `uuid NOT NULL`); `make verify` green; pilot
dossiers (plan 031) can already resolve aliases.

## Phase B — مكتبة مصادر معلَّمة (3–4 days, P1)

Upload the Lorimer Vol. II Geographical & Statistical tribal sections and the Gazetteer of
Arabia Vol. I tribal/well entries through the existing journey-05 lane (honest files,
checksums), OCR them as book sources (`book_assertions.go` path, Arabic+English pages), and
land the passages as `source_passages` rows (`extraction_method='ocr'`, page locators).
License posture recorded per source (Crown pre-1957 → public domain; Lorimer 1908/1915 PD;
recorded on the 028-A2 dashboard).

**Acceptance**: spot-check OCR quality per volume (reported, not asserted); every passage
row cites pages; dashboard shows licenses green.

## Phase C — توصيف أماكن نصي (4–5 days, P1, needs B)

Span-annotation over OCR passages: candidate toponyms + tribe names recognized
(lexicon from A + trigram fuzzy match) → `external_place_candidates` rows (existing table)
with `external_key` = gazetteer entry id, plus the *printed coordinates* of gazetteer
entries kept in `normalized_candidates`/`coordinates`. Reviewer accepts → the existing
effects (alias only / link / reject) exactly as 028-F defined.

**Acceptance**: one Gazetteer of Arabia chapter annotated end-to-end (≈50 toponyms) — the
v2 MVP calibration; throughput + acceptance rate recorded in a `docs/benchmarks/` artifact;
bounded queue lists.

## Phase D — معايرة اللوحات، التمهيد الطوبونيمي (3–4 days, P1, needs C)

The v2 bootstrap: confirmed toponym coordinates (from C's accepted pairings) become
automatic control points per plate; `infra/local/build-plate-georef.py` fits the transform
(affine + quadratic fallback), validates with a held-out decile of control points, stores
`control_points` + transform + RMS error in `sources.metadata` (validated at read; max
bounds like 028's metadata discipline). Report surfaces on the source page: points used,
held-out RMS, plate date. Quality gate: RMS over a stated pixel-equivalent bound → plate
marked `needs_review`, never silently warped.

**Acceptance**: 1916 Admiralty + Dickson plates carry transforms with recorded RMS;
held-out validation test pins the quality gate.

**Delivered (2026-11-16) — the controller and the model both changed, for measured reasons.**

- The control points are the plates' own **printed degree labels** in the margin strips, not
  the accepted toponym coordinates this phase planned to bootstrap from. C's accepted
  pairings are not reviewed yet, and they were not needed: each label is centred on the line
  it names, so the labelled meridian lines are printed data. Interior line detection was
  measured and rejected — at full resolution the strongest interior graticule row covers
  0.32 of its span and the strongest column 0.35, because the dira stipple, the town names
  and the projection's own slant break every line.
- The transform is an **equidistant conic**, not affine with a quadratic fallback. The affine
  model matched the labels to 7 km and was wrong in the interior by more than 100 km: it
  cannot represent an arc, so it interpolates the parallels between the neatlines and misses
  their bow by the sagitta (~154 px ≈ 0.66° at this plate's centre). The plate's own town
  symbols exposed it by landing 0.5–0.8° south. The published conic fits the same labels to
  **8.56 km held-out RMS** (gate 15 km) and lands the seven printed towns at **17.4 km rms**,
  max 33.8 km (Ha'il) — errors of the 1916 compilation, reported as such.
- The **Dickson plate is registered without a transform** (`georef: none`). It prints no
  graticule and states outright that "Tribal Boundaries are only approximate"; fitting a
  transform to visually estimated town pixels would manufacture precision the plate
  disclaims. The documented route is anchor-town control points read and entered by a
  reviewer in phases E/F.
- Both plates publish `certainty: approximate`; the Admiralty plate is `needsReview` with the
  single reason `interior_parallels_unverified_by_ink` — the parallels are not recoverable as
  ink anywhere in the interior, so latitude is validated at the neatlines and at the town
  symbols instead. That is weaker than a graticule-wide check, and any geometry derived
  through the transform must stay `approximate` until a named reviewer accepts it.
- Gate: `infra/local/test_plate_georef.py` (10 stdlib tests, `make plate-georef-test`, part
  of `make verify`) pins model identifiability, held-out ≤ 15 km, km-versus-residual
  reconciliation, town-check reconciliation, round-trip through the published formulas,
  review-flag honesty, and Dickson's absent transform.

## Phase E — استخلاص الملصقات (4–5 days, P2, needs D)

Vision lane: the hosted adapter (off by default, per-source approval — plan 023 posture)
reads plate tiles → tribal label + boundary-segment candidates with pixel geometry →
projected through D's transform → new `external_region_candidates` table (contract copied
from 0063: source-id, occurrence, era label, polygon geometry, score components kept
separate, reviewer answer beside the proposal), with its own decision-history table per
the 0065 pattern. Reviewers see
image crop + warped position side-by-side; accept per stipulation 1.

**Acceptance**: one plate's labels → candidates end-to-end; no candidate renders as fact
anywhere; polygon vertex cap respected.

**Delivered (2026-11-16) — the proposal lane, without a hosted vision model.**

The hosted adapter is not enabled, so the lane does not pretend to read text. It does the
part a deterministic detector can do honestly:

- Migration `0066_external_region_candidates.sql` creates the candidate table and an
  append-only decision-history table on the 0063/0065 pattern. A candidate carries the crop
  box in the plate's own full-resolution pixels, the projected centre, the score components
  kept apart (`ink_density`, `component_count`, `label_confidence`, `transform_rms_km`), and
  an **empty** `label_text_ar` until a reviewer writes one. `region_geometry` is optional and
  capped at 160 vertices in the schema, the same bound the atlas enforces.
- `infra/local/build-plate-region-candidates.py` finds letter-sized connected components,
  groups them into single-baseline text lines inside the neatline, and projects each box
  centre through 030-D's published conic. It refuses a plate whose stored checksum does not
  match the georeference artifact, and it contributes nothing for the Dickson plate because
  that plate has no transform to project through.
- The first run proposes **74 crops** on the 1916 Admiralty plate: tribal names (Aneizah,
  Ghamid, Quhrah, Juheina), town names (Riyadh, Baghdad, Makalla, Shughrah, Yanbu) and the
  title cartouche, all proposed alike because the detector cannot know which is which.
  `label_text_ar` is empty on every row, `status` is `suggested`, and the seed is
  `ON CONFLICT DO NOTHING`, so re-applying it can never overwrite a reviewer's answer
  (verified by rejecting a row and re-seeding it).
- Two limits are measured and recorded in the artifact rather than hidden: a town name
  printed immediately above a tribal name can share one box, and a printed label is
  typeset *beside* its feature, so its box centre can sit up to half the label's width from
  that feature (Baghdad's centre reads about 1° east of the city). The coordinates locate
  the proposal, not a place.
- Gate: `infra/local/test_plate_region_candidates.py` (10 stdlib tests,
  `make plate-region-candidates-test`, in `make verify`) pins the crop-inside-neatline
  invariant, coordinate reproducibility from the published transform, the projection floor
  equalling the transform's held-out RMS, unique occurrence ids derived from the box, the
  absence of any reading/era/status field, rank totality, and the honest truncation flag.

**Still open in E**: the reviewer-facing surface (crop beside warped position, accept/
correct), the reading of a crop into `label_text_ar`, and acceptance's promotion path — the
latter two overlap phase F's workbench. Until then the lane is data only: nothing in it
renders, and nothing in it is a fact.

**Delivered (2026-11-17) — the reviewer surface for the label lane (API complete).**

The reviewer surface mirrors the pairing lane's transport (`internal/pairing`,
`internal/httpapi/pairing_handler.go`) over the 0066 tables, in a new
`services/core-api/internal/regionreview` package with the same error vocabulary and the same review-role gate
(`user_roles` in `('moderator','admin')` — the workspace's two moderator roles, no third
"reviewer" role invented):

- `GET /api/v1/admin/region-candidates` — one status lane (`suggested/accepted/corrected/
  rejected/deferred`), optionally filtered by exact plate key and searched by occurrence
  id / reading / plate key / source title, **LIMIT 100 + truncated**; every score component
  ships as its own field, `crop_px` as the detector's exact box, and the projected centre
  as a nullable point.
- `GET /api/v1/admin/region-candidates/{id}` — one proposal with the plate's honesty block
  (`certainty`, `needsReview`, `reviewReasons`) read from `sources.metadata.georef`, so the
  certainty grammar is on screen while a reviewer works.
- `GET /api/v1/admin/region-candidates/{id}/crop` — the plate's **own pixels** rendered
  from the stored scan (`storage.Store` via the checksummed `source_files` row; a missing
  or corrupt scan is a 503, never a silent empty image): the tight crop the detector
  ranked, plus context around it (growth 5×/4×, minima 560/300 px, response bounds
  1600 px / 2.5 Mpx — the suggestion-crop bounds discipline), both with geometry in plate
  pixels so the workbench can overlay the projected centre. The decoded plate is cached
  process-locally (one plate, checksum-keyed); the real 7163×7170 Admiralty scan crops in
  ~0.7 s first, ~0.1 s cached. Verified live through the local store.
- `POST /api/v1/admin/region-candidates/{id}/decide` — one endpoint over the closed
  action vocabulary (`accept/correct/reject/defer/reopen`); the decision-history row is
  written in the same transaction as the status change, guarded by `WHERE status =
  from-status` so two moderators race to one decision and one 409. **The reading rides
  the verdict**: accept/correct require a non-empty `label_text_ar` (the reviewer's
  words, recorded under their id — an accept without a reading would later promote into
  a named region nobody read), reject may record what the crop actually is, defer/reopen
  refuse one. A `correct` carries the previous reading into the history row's `diff`.
- `reopen` clears the decision attribution (the candidate is `suggested` again,
  undecided) while leaving an existing reading in place — a named human wrote it, and
  it stays attributable on the row and in the history. Reopen is legal from **every**
  decided status, accepted and corrected included: the pairing lane's rule (accepted
  is final) exists for decisions that published rows, and no decision here publishes
  one — in a two-pass workbench, a lane where a reading can never lose is a lane
  where reject is not first-class, and stipulation 1 makes it exactly that. When the
  promotion surface arrives, it will own the freeze of its own promoted rows. Two
  affinities found by the browser journey and pinned by tests: a further `correct`
  of an already-corrected answer is one action, and an **agreeing second pass is one
  action** (a second rejection of an already-rejected crop, a shared deferral) — the
  new decision row is appended under the second reviewer's name with no
  reopen-then-lie in between.
- **No decision in this surface promotes anything**: accept lands `status='accepted'`
  and stops. Promotion into `historical_regions` (`certainty='approximate'`,
  `needsReview` + review reasons carried) is the next surface in E/F; until it exists
  nothing renders to readers, and the unit tests pin that accept raises no
  `historical_regions`/`geographic_associations` rows.
- Gates: `internal/regionreview` service tests + `internal/httpapi` handler tests (role
  gates for list/decide/history/crop, bounded lane, transitions incl. reopen limits,
  diff/reading/history semantics, real-scan crop), all green with the dev database;
  `go vet` + `gofmt` clean.

Still open in E after this: the promotion path (accept → `historical_regions`
`certainty='approximate'` + `geographic_associations` `documented` + `spatial_evidence`,
per stipulation 1, with `needsReview`/`reviewReasons` carried) — which shares its surface
with F's workbench below.

## Phase F — ورشة المراجعة: task grid + retirement (4–5 days, P2, needs C/D)

Tile-task queue over sources (Tasking-Manager pattern): tasks bounded per page/plate
square, claim + work + validate lane; retirement = 2 *independent* confirmations per
stipulation 2; keyboard-first accept/correct/reject/defer modeled on the existing review
screen. Deferred work returns via the plan-026 history/reopen pattern.

**Acceptance**: a reviewer can close a full tile without bespoke tools; two-person
retirement unit-tested; e2e extends journey 06 (review lane) with relative assertions only.

**Delivered (2026-11-17) — the tile-task lane and its two-person retirement.**

- Migration `0067_region_tile_tasks.sql` creates the tile table and an append-only
  events table on the 0063/0065 pattern: a tile carries its square in plate pixels
  (`box_px`), its source and plate key, a closed status vocabulary (`open /
  in_progress / submitted / retired`), the current seat (`claimed_by/at`, cleared by
  release and by retirement), and the trail — claims, deferrals (release), submits with
  a bounded verdict snapshot, the confirmations, and the invalidations.
- `infra/local/build-plate-tile-tasks.py` divides each plate's neatline frame into
  1024-px squares and emits one task per square holding a proposal — the first run
  gives the 1916 Admiralty plate **24 tiles covering all 74 proposals**; the Dickson
  plate (whose proposal lane contributes nothing, no transform) contributes none. The
  seed is `ON CONFLICT DO NOTHING`: re-applying it can never disturb a claim, a pass
  or a retirement. Artifact: `plate-tile-tasks.json` + `seed-plate-tile-tasks.sql`,
  committed like every generator's filtered subsets.
- `internal/regionworkbench` owns the lane: bounded list (per status, per plate,
  LIMIT 100 + truncated), tile detail whose **membership is derived at read time**
  from the crop boxes against the tile's own square (centre-in-square), claim with
  exclusivity (`FOR UPDATE`; a seated tile admits no second reviewer), release (the
  deferral: seat cleared, note recorded, history keeps the round trip), submit — which
  **derives the pass's verdicts from the decision history, never re-typed**: an
  incomplete pass is refused naming the untouched candidates; a tile past the response
  bound is refused rather than silently truncated; the second submit compares the
  passes at the `Compare` arithmetic (accept/correct are one read class requiring an
  exact reading match, reject/reject confirms, a shared deferral never retires) and
  retires on agreement or returns the tile open naming the divergence. The same
  reviewer twice is refused at the **claim gate** (`ErrSelfReview`): one person
  counted twice is exactly what two-person retirement refuses — the arithmetic refuses
  it too.
- `0068_decision_sequence_numbers.sql` gives both decision trails a monotonic
  `sequence_number`: a reopen followed by a re-verdict lands in one clock tick, and
  "latest" ordered by `created_at` alone is ambiguous exactly then (the primary keys
  are random UUIDs, so a tie broken by id is a coin toss).
- Handlers + routes: `GET /api/v1/admin/region-workbench/tiles` (list),
  `GET …/tiles/{taskID}` (detail), `GET …/tiles/{taskID}/history`,
  `POST …/claim`, `POST …/release`, `POST …/submit` — moderator-gated like every
  review lane; `go test` green with the dev database, `make verify` including the new
  `plate-tile-tasks-test`.
- The workbench page (`/region-workbench`): the four lane tabs, the tile bench with
  the two crops per candidate (tight + context, the projected centre beside them),
  the plate's honesty block beside the crop, keyboard-first accept/correct/reject/
  defer, and the disagreement banner naming what the comparison found.
- e2e journey 13 (`apps/web/e2e/journeys/13-region-workbench.spec.ts`): two
  independent passes retire a tile end to end against the real stack — the first in
  the browser (claim, read, reject, submit, the submit button unlocked only when
  nothing is unanswered), the second through the API with the same readings; one
  pass alone never retires (the submitter's own re-claim is a 409); the retired lane
  and the history carry the arc; and the journey's last read asserts the arc
  promoted nothing (no region geometry on any answered row). The journey found two
  real gaps on its way to green, both fixed and pinned: `correct:corrected` (the
  rework flow revises an already-corrected answer) and the **agreeing second pass
  being one action** — a second rejection of an already-rejected crop (and a shared
  deferral) appends the second reviewer's own decision row without the
  reopen-then-lie in between, which is what lets the two confirmations carry two
  names.

**Still open in F**: nothing in the lane itself. Two documented adjacencies: the
workbench page's map underlay (the tile square drawn over the plate itself) belongs
to 032's plate layer — the bench's crops already answer without it — and the
candidate-decides remain available to any moderator through the API even while a
colleague holds a tile's seat: the tile is a courtesy queue, the decision history is
the law, and the two-person arithmetic at `Compare` is what actually gates
retirement. Journeys 07/09 fail on this working tree and failed identically before
this plan began (the pre-existing locator drift the 029 note records); journey 13
's greenness was verified through both the full-suite run and a standalone run.

## Phase G — القياس (1 day)

**Delivered (2026-11-17)**: `make extraction-eval` (`infra/local/extraction_eval.py`),
read-only through the documented container path: rows/week by lane, the decision mix
and acceptance rate per source over decided rows only (suggested rows sit in no
rate), the plates' recorded RMS floors, and dira coverage km² over actually traced
geometry — **zero today, stated as zero**; no claim without data, no number
manufactured. Writes `docs/benchmarks/extraction-eval.json` (the queries ride the
artifact beside their numbers). First run on the seeded stack: 24 open tiles, 74
suggested proposals, nothing decided — the honest state of an untouched factory.

## Execution order

| # | Phase | Effort | Depends on | Value now |
| --- | --- | --- | --- | --- |
| 1 | A tribe seeds | 2–3d | — | dictionary + 031 unblocked |
| 2 | B sources | 3–4d | A2 dashboard green | honest text corpus |
| 3 | C text lane | 4–5d | B | first candidates |
| 4 | D plate georef | 3–4d | C | plates placeable |
| 5 | E vision lane | 4–5d | D | dira polygons |
| 6 | F workbench | 4–5d | C,D | scale + crowdsafe |
| 7 | G measurement | 1d | all | the factory measures itself |

Total ≈ 4–5 weeks of focused work; every phase ships and demo-verifies alone.

## Verification gates

`make verify` extended where phases touch (C/F endpoints, D metadata validation);
journey 06/08 extensions relative-only; `make verify-full` before any phase is closed;
`docs/phase-status.md` + `plans/README.md` row updated per phase.
