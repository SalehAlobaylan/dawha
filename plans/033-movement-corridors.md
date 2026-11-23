# Plan 033: Migration corridors and typed tribal events (المِرْحلة: ممرات الحركة والأحداث)

> **Executor instructions**: movement rows are claims with sources, never a layer that
> projects confidence. Corridor geometry comes from passage-quoted stages or
> reviewer-drawn polylines on a *source image* — assembled against stored plate transforms
> from plan 030-D. `certainty='approximate'` unless both endpoints are documented passages
> and a reviewer accepted. Seasonal windows are era-window-scoped like everything else in
> the atlas grammar. Bounded lists; scripts, not screenshots.

Sources of truth: `docs/tribal-territory-solutions.md` §S4/v2 (ORBIS seasonality/cost,
ACLED taxonomy adapted, StoryMapJS narration) and `docs/tribal-territory-sources.md`.

## Status

**DONE — all six phases shipped and demo-verified (2026-11-18).**

- **Phase A (subjects)**: migration 0071 broadens `subject_type` to
  person/tribe/family (additive — the person rows keep their own shapes; the
  check is the closed vocabulary the atlas can anchor); `GET
  /api/v1/tribes/{id}/movement` mirrors the person lane verbatim
  (`internal/geography/tribe_movement.go`, the bounds + truncated + the three
  policy walls: the tribe's own reference kind, the source, the endpoint
  places). The subject layer renders it off the same `?subject=` state.
- **Phase D (taxonomy)**: `migration_events.kind` CHECK carries the classical
  repertoire (yawm/ghazwa/sariya/nafir/relocation/famine/other) — NULL stays
  the honest unclassified row; `apps/web/src/lib/movement-events.ts` is the
  vocabulary artifact beside the atlas-periods one, header provenance
  spelled (the classical forms are the sources' own words; ACLED's keys live
  in the solutions doc); the map's movement lane now carries `eventKind`
  verbatim and the inspector names the kind or says «حدث غير مُصنَّف بعد» —
  never a guess.
- **Phase B (corridors)**: migration 0072 carries `movement_corridors` + the
  ordered `movement_corridor_stops` (place FKs, positive day counts, the
  camel/horse/foot/sea mode vocabulary, the season column, the years window,
  NO corridor without source_id — NOT NULL at the door), the 0063
  answer-beside-proposal reviewer contract. `geography.SubjectCorridors` +
  `GET /api/v1/subjects/{type}/{id}/corridors` read bounded (20 + truncated);
  a stop whose place went private draws nothing rather than a guess.
- **Phase C (filters)**: `MapInput` gained `movementMode`/`season`, applied
  through the era window's own undated-kept discipline (the seasons are the
  sources' pastoral months — صيف 5..9, شتاء 11..3; an undated event stays
  readable under every season); the corridors respond on their own columns;
  the absent filters change nothing (pinned by
  `TestMapSeasonFilterNarrowsOnlyItsOwnLane`).
- **Phase E (eruption)**: the events ride the map's own aggregated GeoJSON
  lanes (no per-feature DOM — the 027 worker path is the only renderer), the
  density reads by the status vocabulary, the era slider scopes, and the
  inspector's kind chip carries the receipt-side links (031's badges).
- **Phase F (export)**: `infra/local/build-journey-narrative.py` — the
  documented-only slide export (StoryMapJS-shaped reads: one slide per
  event/stop with its own receipts + the citations), checksummed
  (`.sha256` sidecar), refusing an empty window; pinned by
  `infra/local/test_journey-narrative.py` + the `make journey-narrative-test`
  gate inside `make verify`.

- **Priority**: P2 (highest engagement, needs 030/031 data first)
- **Effort**: L total, staged
- **Risk**: MED resolved (no pseudo-precision entered the schema: the
  corridor's source_id is NOT NULL, the legs' own days are positive, the
  seasons are the sources' own months, and the unclassified event renders
  honestly)
- **Depends on**: 030 (sources + transforms + dira), 031-A (dossier hooks), 027 (movement contract exists) — all done
- **Category**: geospatial + schema + narrative
- **Planned at**: post-029 working tree, 2026-11-15

## First principle: what already exists

| Reuse | Where | Feeds |
| --- | --- | --- |
| `migration_events` (from/to places, windows, claim FK) | `db/migrations/0006_geography.sql` | A: subject broadening |
| Person movement endpoint + subject layer precedent (subject-scoped, bounded, visibility-scoped) | `services/core-api/internal/geography/person_movement.go`, 028-D | A/C endpoints |
| Arrowhead + status route grammar | `apps/web/src/lib/map-vocabulary.ts` | corridor/event rendering |
| Era chips (`atlas-periods.ts`) + dual-handle window | existing | season/era scoping |
| Job system for offline exports | plan-006 machinery | F export job |

## Features this plan delivers

| Feature | Surface | Phase |
| --- | --- | --- |
| ممرات الحركة (corridors): polylines with day-stages (camel/horse/foot/sea) and season windows (صيف/شتاء) | new table + atlas route layer | B |
| الأحداث المُنوَّعة (typed events): يوم / غزوة / سرية / نفير taxonomy with actors | `migration_events` extension + filters | D |
| حركة القبائل (tribe-subject movement): tribe/family subjects on the atlas like 028-D's persons | endpoints + subject layer | A |
| الشتاء/الصيف filters on the map API | `MapInput` extension | C |
| انفجار الأحداث عبر الزمن (event eruption over the era slider) | atlas render | E |
| رحلة مسردة (narrated journeys): shareable era-slide JSON export | offline script | F |

## Binding stipulations

1. **No invented geometry**: a corridor segment needs a passage anchor or a reviewer-drawn
   line on the source image; day-stages quote the gazetteer's own measured distances.
2. **Subject broadening is additive**: `subject_type` gains `tribe`/`family`; person
   semantics untouched.
3. **Events cite actors**: event actors are `entity_relationships` rows to tribe entities,
   never free text.
4. **Bounded lists everywhere** (rule carried).

## Phase A — subject broadening (2 days, P2)

Migration + tests: permit `tribe`/`family` subjects in `migration_events` (CHECK updated;
nullable subject columns preserved); `GET /api/v1/tribes/{id}/movement` mirroring
`person_movement.go` (visibility through TribePredicate equivalent via dictionary
readability + source scoping); render as the existing subject layer when `?subject=<tribeID>`.

**Acceptance**: person tests untouched-green; new endpoint pinned by visibility unit tests
(private source hides its corridors/events).

## Phase B — corridors (3–4 days, P2, needs 030-D/E)

New table `movement_corridors`: ordered stops (`place_id` FKs), per-stage day counts +
mode enum (`camel|horse|foot|sea`) + season window (from/to year) + `source_id` +
`certainty` + reviewer columns (0063 contract). Data path: reviewer-drawn on the warped
plate (030-D transform display) OR passage-composed; stored `approximate` by default.

**Acceptance**: no corridor without `source_id`; stage day-counts validated positive;
unit tests pin the contract copy.

## Phase C — season/mode filters (2–3 days, P2, needs B)

`MapInput` gains optional `movementMode` + `season` filters applied through the same
timePredicate discipline; corridors/events respond; era chips can set them.

**Acceptance**: filters multiply correctly with the era window; absent filters change
nothing (unit test); journey 08 relative extension.

## Phase D — typed events taxonomy (2–3 days, P2, parallel with B/C)

`migration_events.kind` CHECK: `yawm | ghazwa | sariya | nafir | relocation | famine |
other` (the classical repertoire, mapped in one vocabulary file beside the atlas-periods
artifact, each with its Arabic label); filter + chip; actors via `entity_relationships`.

**Acceptance**: taxonomy vocabulary artifact with header provenance; events without kind
render truthfully («حدث غير مُصنَّف بعد»).

## Phase E — eruption rendering (2 days, P2, needs B/D)

Events as markers over the era slider: density rendered honestly by status (a decade with
ten documented events denser than ten `platform_inferred` candidates — both truthful);
marker click → receipts (031 badges).

**Acceptance**: scale-independent rendering (no per-feature DOM blowup — reuse the GeoJSON
worker path fixed in 027); relative e2e assertions.

## Phase F — narrated journey export (2 days, P2, needs B/D)

`infra/local/build-journey-narrative.py`: documented-only corridors + events for one
tribe/era → a shareable slide JSON (StoryMapJS-compatible shape) with per-slide receipts.
Committed subsets only; generation is a job, artifacts checksummed.

**Acceptance**: export contains zero `platform_inferred` rows; receipts present per slide.

## Execution order

| # | Phase | Effort | Depends on |
| --- | --- | --- | --- |
| 1 | A subjects | 2d | 027 |
| 2 | D taxonomy | 2–3d | — |
| 3 | B corridors | 3–4d | 030-D/E |
| 4 | C filters | 2–3d | B |
| 5 | E eruption | 2d | B,D |
| 6 | F export | 2d | B,D |

Total ≈ 2.5–3 weeks of focused work; every phase ships and demo-verifies alone.

## Verification gates

`make verify` per phase; journey 08 extensions relative-only; `make verify-full` before
close; phase rows in `docs/phase-status.md` + `plans/README.md`.
