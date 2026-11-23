# Plan 032: The evidence atlas — plates, comparison, flows, permalinks (الأطلس بالصورة الكاملة)

> **Executor instructions**: 027's grammar still governs: a reference/plate overlay must
> never read as a documented pin; status and certainty chips travel with every drawn layer;
> every number server-derived through the visibility predicates; every list bounded
> (`LIMIT` + `truncated`). All URLs stay deep-linkable state (027-D), and the embeddable
> surface exposes the same bounded contracts — never a second API.

Sources of truth: `docs/tribal-territory-solutions.md` §S1/v2 (Allmaps mechanics, ORBIS
flows, URL-first discipline) and `docs/tribal-territory-sources.md` (the plates and their
licenses).

## Status

**DONE — all six phases shipped and demo-verified (2026-11-18).**

- **Phase A (transforms)**: `internal/geography/plates.go` — the reader's own
  `ValidatePlateTransformation` over the artifact's true shape (equidistant
  conic, negative latitude ratio, the 8-strip outer/inner frame whose inner
  rectangle is the neatline, the held-out `fit` floor, `method: "none"` = the
  refusals lane). The register endpoint `GET /api/v1/plates` (bounded,
  policy-scoped) lists every plate: its validated transform or the honest
  disabled caption (the Dickson plate's own words). Tests:
  `internal/geography/plates_test.go`.
- **Phase B (overlay)**: `internal/regionreview/tiles.go` —
  `WarpPlateTile`: per-OUTPUT-pixel web-mercator → the stored conic's
  published inverse → bilinear read of the stored scan; transparent outside
  the neatline; XYZ 256, zoom band 2–7; `GET
  /api/v1/plates/{plateKey}/tiles/{z}/{x}/{y}.png` (the subtree route —
  ServeMux refuses `{tileY}.png`). The gate is the plate SOURCE's own
  visibility (the overlay is a map layer, not a review lane). Live-verified:
  the ink-round-trip test renders the plate's own colour at the exact pixel
  the transform names; the ocean tile is fully transparent.
- **Phase C (comparison)**: `internal/regionreview/pointcrop.go` —
  `GET /api/v1/plates/{plateKey}/crop?lon=&lat=` returns the plate's own
  pixels around one point (`insideNeatline` honest), base64 crop + honesty
  block. Front-end `PlateComparePanel.tsx` + URL state
  `compare=arm|lon,lat` (027-D). Front-end types:
  `src/lib/plate-atlas.ts`, `fetchAtlasPlateRegister`,
  `fetchAtlasPlatePointCrop`.
- **Phase D (disputes)**: `questions.ClaimDispute` + `GET
  /api/v1/claims/{claimID}/challenge`; `AtlasInspector`'s disputed block
  renders the challenge's side-by-side and links to فاحصة الحسم. A disputed
  polygon's register is the map's own vocabulary (no new grammar); the
  status/certainty chips travel with the layer.
- **Phase E (flows)**: `geography.AtlasRelations` + `GET
  /api/v1/atlas/relations` — the era's RECORDED `entity_relationships`, each
  endpoint resolved through the claims lane's own anchor (a claim's place the
  actor can read); unanchored ends are reported honestly, and the bound is
  the plan's 200-edge discipline. Front-end `relationArcCollection` draws
  dashed arcs + the reused arrow sprite; unanchored relations draw nothing.
- **Phase F (embed)**: `/atlas/embed` — the read-only surface with the SAME
  bounded endpoints, the SAME URL state (`from,to,plates,plateOpacity,
  compare, feature`), no editing chrome; the iframe contract sits in the
  page's own header comment.
- **Deferred on purpose**: the split-view (the opacity + toggle shipped; the
  second-canvas split adds a WebGL context per view — the phase's acceptance
  ran on toggles/opacity/URL, which is what the journey pins).

- **Priority**: P2 (sells the corpus; consumers arrive from 030/031 first)
- **Effort**: L total, staged
- **Risk**: MED resolved — the deliberate dependency gate answered WITHOUT
  `@allmaps/maplibre`: the per-pixel Go warp IS the fallback of record (a
  four-corner image source is the affine-class lie 030-D rejected); no new
  frontend dependency was added.
- **Depends on**: 030-D (plate transforms), 030-E (dira polygons), 031 (challenges feed the disputed view); 027 atlas trio — all done
- **Category**: frontend + visualization
- **Planned at**: post-029 working tree, 2026-11-15

## First principle: what already exists

| Reuse | Where | Feeds |
| --- | --- | --- |
| Provenance-first atlas (map + filters + inspector trio, print snapshot, deep-linkable state) | `apps/web/src/components/AtlasMap.tsx`, `AtlasFilters.tsx`, `AtlasInspector.tsx` (027-C/D) | everything |
| Status/certainty visual grammar + arrowheads | `apps/web/src/lib/map-vocabulary.ts` | plate register, dispute register, flow arcs |
| Era chips + windows | `apps/web/src/lib/atlas-periods.ts` | era-scoped renders |
| Sources carry `metadata` jsonb (028 import shape) with checksums | `sources` + plan-030-D stored transforms | plate overlays |
| Inference overlay pattern (stamp-ring, handoff to review UI) | `AtlasInferenceOverlay.tsx` | candidate polygons display |

## Features this plan delivers

| Feature | Surface | Phase |
| --- | --- | --- |
| طبقة اللوحات (plate overlays): warped 1916/Dickson/Ottoman plates over the basemap | atlas layer control | A,B |
| مقارنة النقطة (same-point view): crosshair-synced read of the same place across every plate | comparison panel | C |
| درجة الطبقات: opacity + split-view | layer control | B |
| سجل الأكيدية والطعون (certainty + dispute register): certainty shading + disputed side-by-side (from 031-D challenges) | polygon click | D |
| الأروابط (relation flows): alliance/raid/relationship arcs per era | flow layer | E |
| Permalinks + embeddable view (URL-state completion) | atlas routes | F |

## Binding stipulations

1. **Plate register**: warped plates and their labels render in a distinct visual register
   (like the reference pins of 028-A) — a plate caption must never be confusable with a
   documented region label; the vocabulary file carries the register.
2. **Dependency gate (Phase B)**: adding `@allmaps/maplibre` is a *recorded decision* —
   pinned version, license noted (open source), `npm audit` run, and a fallback documented
   (offline retiling via GDAL in `infra/local/`, producing XYZ tiles). If the plugin cannot
   be justified, the fallback is the plan of record.
3. **Geometry truthfulness**: plate overlays are *images* transformed by 030-D's stored
   transforms; they are never treated as data geometry. Only accepted dira polygons from
   030-E are `historical_regions`.
4. **Bounded query contracts** unchanged for every new layer request.

## Phase A — transform consumption + validation (2 days, P2, needs 030-D)

Read plate transforms from `sources.metadata` with strict validation (same bounds/shape
tests as the writer's); a source page badge shows RMS + control-point count + plate date;
missing/invalid transform → overlay control disabled with an honest caption, not a broken
render.

**Acceptance**: invalid metadata → disabled + caption; unit tests pin the read contract.

## Phase B — warped overlay layer (3–4 days, P2, needs A)

Add the warped-plate layer per the Phase-B dependency gate: opacity slider + split-view
(same extent, plate vs basemap) in `AtlasFilters`/`AtlasMap`; layer-on state in the URL.

**Acceptance**: journey 08 extension (relative assertions): layer toggles, opacity, URL
round-trip; canvas-paint presence check only where the existing 08 pattern already does it.

## Phase C — same-point comparison panel (3 days, P2, needs B)

Crosshair-synced panel: pick a coordinate → read the point everywhere (each plate crop +
accepted dira summary + receipts links). Allmaps-Here adapted; no new data invented —
composition of stored transforms + crops.

**Acceptance**: private/none states honest; panel bounded; no crop fetched for plates
without transforms.

## Phase D — certainty + dispute register (2–3 days, P2, needs 031-D)

Polygon click shows the certainty chip; `disputed` opens the challenge's two-source
side-by-side (031-D data) with links into the adjudication surface. Rendering follows
`map-vocabulary.ts` registers; no new status grammar invented.

**Acceptance**: a disputed polygon renders disputed everywhere simultaneously (unit test);
challenge links resolve.

## Phase E — relation flow arcs (3–4 days, P2, needs 030-A)

`entity_relationships` arcs for the selected era (alliance/marriage/raid predicates are
data-driven, no new schema); arrowhead grammar reused; era chips scope the layer; bounded
edge fetch (the 200-edge bound discipline from `docs/graph-benchmark.md` note applies).

**Acceptance**: era window filters arcs; visibility predicates applied to both endpoints
(the 027-A lesson is binding); e2e verification relative-only.

## Phase F — embeddable view (2 days, P2, needs A–C)

`/atlas/embed` route: same bounded contracts, no editing chrome, URL-driven state, and a
documented iframe contract. Same-origin API rules already fixed in 027 — reuse, don't
re-touch.

**Acceptance**: embed renders from deep-link state only; no new unbounded endpoints.

## Execution order

| # | Phase | Effort | Depends on |
| --- | --- | --- | --- |
| 1 | A transforms | 2d | 030-D |
| 2 | B overlay | 3–4d | A |
| 3 | C comparison | 3d | B |
| 4 | D disputes | 2–3d | 031-D |
| 5 | E flows | 3–4d | 030-A |
| 6 | F embed | 2d | A–C |

Total ≈ 3 weeks of focused work; every phase ships and demo-verifies alone.

## Verification gates

Journey 08 extensions (layers, chips, dispute view) relative-only, green twice; `make
verify` per phase; `make verify-full` before close; phase rows updated in
`docs/phase-status.md` + `plans/README.md`.
