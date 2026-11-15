# Plan 027 — Handoff for the next agent

> **สถานะ: งาน 027 ถูก implement เสร็จแล้ว** (true ณ 2026-11-05): §3 blocker ถูกแก้ —
> root cause จริงคือ query คู่ความสัมพันธ์เชื่อม place คู่ด้วยลำดับ place-id แต่กรองด้วย
> association-id ทำให้ edge หายแบบสุ่ม (แก้ด้วย LEAST/GREATEST + DISTINCT) และ test
> ถูกทำให้ deterministic; Web/Go gates เขียว; e2e journey 08 (8 tests) ผ่านบน seeded stack;
> phases/README.md 027 = DONE พร้อมหลักฐาน; dead components ถูกลบ; workstream D
> (intelligence overlay stamp-ring grammar + print snapshot + perf budget log) เสร็จแล้ว
> — ดู `docs/phase-status.md` Phase 10 และรายละเอียดด้านล่าง
>
> (ไฟล์นี้เก็บไว้เป็นหลักฐานการแก้ blocker และลำดับงานเดิม; เอกสารสถานะคือ `plans/README.md`)

> **Give this file (+ `plans/027-geo-map-atlas.md`) to the next agent.**
> Repo root: `/Users/salehalobaylan/Desktop/MyRepos/dawha`
> DB: container `dawha-db-1`, `postgres://dawha:dawha_local@localhost:55432/dawha`
> **No commits have been made.** All changes are uncommitted. **Never add AI `Co-Authored-By` / `Co-Generated-By` / `Authored-By` trailers to commits** (absolute repo rule, `AGENTS.md`). If you commit, use the human's own git identity only.

---

## 1. What the plan is

`plans/027-geo-map-atlas.md` — "The Atlas": `/places` becomes one honest, provenance-first surface.

- One MapLibre canvas, real `GET /api/v1/map` features only (`MapFeature[]`).
- One vocabulary module (`map-vocabulary.ts`): `documented | interpreted | platform_inferred | disputed | unresolved`.
- No demo data on the page, no invented geography, no modern borders, no tile layer.
- Regions render as polygons (not centroid dots), migrations as lines + arrowheads, markers aggregated per (place, status) with monospace counts.
- Provenance-first inspector: claim → source → evidence text → review state → claim handoff (`/search?claimId=...`) + place handoff (`/search?q=<placeName>&kind=places`).
- Functional filters (status/kind pills over in-memory data, dual-handle year scrubber, text search), deep-linkable URL (`place, feature, from, to, status, kind, q`), truncation banner.
- Workstreams: **A** truth (radius bug + vocabulary + single-source), **B** grammar (encoders, rings, `claim_id`), **C** chamber (inspector, filters, deep links, a11y, mobile), **D** instruments (intelligence overlay, place CRUD, print/share, perf budget).

## 2. What's DONE (verified today)

Web gates green:

```
cd apps/web
npm run typecheck   # clean
npm run lint        # clean (3 unused-var errors + 1 degenerate-ring assertion fixed)
npm test -- --run   # 21 files / 168 tests pass
npm run build       # clean
```

Go:

```
DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' \
  go test ./internal/geography -count=1 -timeout 180s   # PASS
```

Targeted geo tests that PASS:

```
TestGeospatialRadiusFilterRespectsMetricDistance
TestMapCarriesClaimAndRegionGeometry        (new: claim_id + ring serialization)
TestDecodeRegionRingKeepsAClosedCappedOutline (new: cap, closure, Polygon/MultiPolygon)
```

Implemented:

- **A1 radius fix** — `services/core-api/internal/geospatialintelligence/repository.go::loadSpatialEdges`: `ST_DWithin(...::geography, ...::geography, $1)`, distinct-place pair enumeration (`second_place.id >= first_place.id` — `>=` so same-place pairs cluster), latitude-aware `ST_Expand` prefilter, `ST_DistanceSphere` refinement. DB regression fixture added in `service_test.go` (100 km → 0 edges, 340 km → 1 edge, metric distance 200–400 km).
- **A2 vocabulary** — `apps/web/src/lib/map-vocabulary.ts` (+ tests): status glyph/stroke/tone/seal/hollow, certainty labels, relation/kind labels, `mapEvidenceStatusLabel`, `formatMapCount`.
- **A4 single-source** — `routes/places.tsx` renders only from `fetchMapFeatures` (viewport debounced 420 ms, limit 5000, abort). Demo import, % markers, SVG route, hardcoded `03/02` stats, dead layer pills removed. `routes/index.tsx` + `MapPreview.tsx` read live features, labelled as schematic, no dead toggles.
- **B grammar** — `lib/atlas.ts` pure encoders (aggregation, ringPolygon/regionCollection, migration line + bearing arrowheads, `periodPresets`, `viewportForCoordinates`, deep-link `parseAtlasSearch`, `atlasFilterValues/encodeAtlasFilter`, `markerProvenance`), `atlas.test.ts` + `map-vocabulary.test.ts`.
- **B backend** — `internal/geography/service.go`: `Feature.claimId`, `Feature.ring` (`Coordinate`, `MaxRegionRingVertices=160`, `decodeRegionRing` uniform-sample cap, `geometryViewportPredicate` + `ST_AsGeoJSON`), `claim_id` in association/migration SQL, `Truncated` already returned; `types.ts` MapFeature updated; `lib/api.ts` `fetchMapFeatures` sends viewport/limit/signal, `credentials: "include"`.
- **C chamber** — `components/AtlasMap.tsx` (MapLibre region/migration/graticule layers, dashed certainty, hatch for disputed, arrowheads, DOM marker buttons projected via `map.project()` per frame, auto-fit once, `resetSignal`, focus animation, WebGL fallback), `AtlasFilters.tsx` (status/kind pills with counts, dual range sliders, data-derived presets, text search, reset), `AtlasInspector.tsx` (claim/source/evidence/period/certainty, aggregated member list, region/migration readouts, claim + place handoffs), `AddPlaceDialog.tsx` (private place creation + validation, accessible modal), `routes/places.tsx` (deep links, list/rows, states, truncation banner, mobile order, aria-live), `route-tree.tsx` (`/places` `validateSearch: parseAtlasSearch`), `styles.css` (appended atlas/inspector/filter/dialog/marker/responsive block).
- **D (partial)** — place CRUD client (`fetchPlaceIndex`, `createPlace` in `lib/api.ts`) + dialog. **Intelligence overlay, print/share NOT done** (see §4).
- **E2E spec already rewritten** (see §4): `apps/web/e2e/journeys/08-browse-map.spec.ts` — 7 tests, real-API assertions via accessible names (legend, rows, inspector evidence/coords, filters persist on reload, `?status=none` empty state, `?feature=a100…` deep link, `?q=الأحساء` search, no-session add-place notice).

## 3. THE ONE BLOCKER — fix this first

**Failing:** `services/core-api/internal/geospatialintelligence`:

```
TestGeospatialIntelligenceRunSeparatesSourceAndInferredLayers
  service_test.go:119: unexpected geospatial run: { ... Report:{ ... Clusters:[] ... FindingCount:1 ... } }
```

The run succeeds, finds 1 contradiction and ≥1 migration hypothesis, but **`len(run.Report.Clusters) != 1`** (it's `[]`).

### Evidence gathered

- Radius test passes in isolation and the `loadSpatialEdges` query returns 5 place-pairs at 500 km on the seeded DB.
- Integration fixture: 3 places at Riyadh `(46.6753, 24.7136)`, Al-Hasa `(49.5658, 25.3647)`, Al-Ula `(38.1333, 26.6084)` → pairwise 300 km / 882 km / 1150 km. At `RadiusKM: 500` only Riyadh↔Al-Hasa should edge, giving exactly 1 cluster (`buildClusters` in `analysis.go:257` unions `SpatialEdges` ∩ loaded associations, requires `len(component) >= 2 && len(placeIDs) >= 2`).
- Association time windows were already shifted so Al-Hasa (1206–1209) overlaps Al-Ula (1207–1210) → the contradiction (distance > 500 km) fires correctly. So associations ARE loading.
- Cleanup works (0 `موضع%` rows after run). Run the single test isolated — it still fails.

### Most likely causes (check in this order)

1. **Fixture time overlap broke the cluster side:** `buildContradictions` and cluster building both consume associations, but the test's expected contradiction pair may now be the Al-Hasa↔Al-Ula pair while the cluster edge (Riyadh↔Al-Hasa) is fine — verify with a debug print of `clusters`, `findings`, and loaded `associations` IDs, or run with `-v` and dump `SpatialEdges` length.
2. **`loadSpatialEdges` inside a transaction:** `StartRun` calls it with `tx` (not `pool`). The isolated radius test calls it with `pool` directly and passes. If the transaction's snapshot/visibility differs (e.g. the test's own inserts aren't visible under the same isolation, or `tx` rolls back), edges would be empty in-run only. Check `service.go:122` context.
3. **Association `Latitude/Longitude` nil:** `loadAssociations` selects `ST_Y(p.geometry), ST_X(p.geometry)` into `latitude, longitude, pgtype.Float8`. If the scan order is swapped or either is NULL, `buildClusters` skips the association (`Latitude != nil && Longitude != nil`). Verify `repository.go:441-442` scan order vs SELECT order.
4. **Source join empties associations:** `loadAssociations` has `LEFT JOIN sources s ... WHERE s.visibility = 'public' AND s.dependency_status = 'independent'`. Integer/rune source titles (`"مصدر جغرافي "+string(rune('A'+index))`) — if a source fails the dependency check, its associations vanish and `byID` loses nodes. Check which of the 3 associations load.

**Do not** "fix" by changing the expected count to 0 — the plan requires the overlay/cluster behavior. The correct fix is in the query or fixture so a real edge produces a real cluster.

Suggested debug command (prints the run's clusters/findings):

```
cd services/core-api
DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' \
  go test ./internal/geospatialintelligence -run TestGeospatialIntelligenceRunSeparatesSourceAndInferredLayers -count=1 -timeout 120s -v 2>&1 | tail -n 100
```

Once green, also run `go test ./internal/geography ./internal/geospatialintelligence ./internal/httpapi` (and `gofmt -l` — note `gofmt` currently flags `geography/service.go`; run `gofmt -w` on changed Go files).

## 4. Remaining work (in order)

1. **Fix §3.** Then `make verify` (fast gate: lint/typecheck/unit/build/docs-check/ocr-benchmark-test/ocr-corpus-check/name-pipeline-guard-test).
2. **Delete dead components** (grep-confirmed unreferenced except by themselves):
   ```
   apps/web/src/components/MapLibreCanvas.tsx
   apps/web/src/components/HistoricalMapCanvas.tsx
   apps/web/src/components/HistoricalMapPanel.tsx
   ```
3. **Workstream D — Intelligence overlay + print/share (P2/P3, plan Step 7):**
   - Overlay clusters + conflict findings on the Atlas **in stamp-ring grammar only** (`▣` / rotated-square seal, olive), never as documented data. Each finding opens the existing review UI (`components/GeospatialIntelligencePanel.tsx` — currently used only from `routes/research-workspace.tsx:145`; `lib/api.ts` already has `startGeospatialIntelligence`, `fetchLatestGeospatialIntelligenceRun`, `fetchGeospatialFindings`, `reviewGeospatialFinding`; types `GeospatialIntelligenceRun/GeospatialFinding/GeospatialCluster` in `types.ts:2305+`). The overlay reads from a run selected by the current filters — reuse `placeIds`/`entityIds`/`questionId` already in `AtlasSearch`.
   - Compass/print: add a `window.print()` snapshot button + `@media print` rules in `styles.css` (print the rendered legend/rows/inspector, hide canvas controls).
   - Perf budget: record payload/query counts next to `TestMapResponseIsBoundedAndSaysSo` step-3 logs (plan 007 style).
   - E2E for overlay + review handoff (accessible-name assertions, no WebGL pixels — established rule).
4. **Docs in the same commit (repo rule):** update `docs/phase-status.md` Phase 10 (`:216`) + Phase 23 (`:629`) — they still cite `HistoricalMapPanel.tsx` (deleted in step 2) — and flip `plans/README.md:35` from `TODO` to `DONE` with the real evidence. Run `make docs-check` (the checker was already updated to include `plans/027-geo-map-atlas.md`).
5. **Final gates:** `make verify`, then `make e2e` against the seeded stack:
   ```
   COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed   # DB container dawha-db-1 already up
   COMPOSE_PROJECT_NAME=dawha make e2e
   ```
   Expect `journeys/08-browse-map.spec.ts` (7 tests) to pass on the seeded map (4 places, 2 associations at Riyadh, 1 inferred migration الأحساء→الرياض with evidence "إشارة موضع تقريبية إلى انتقال محتمل.", `needs_review`, coords 49.57/46.68).

## 5. Key files

| Area | Files |
|---|---|
| Plan (authoritative) | `plans/027-geo-map-atlas.md` |
| Route / orchestration | `apps/web/src/routes/places.tsx` |
| Canvas | `apps/web/src/components/AtlasMap.tsx` |
| Filters | `apps/web/src/components/AtlasFilters.tsx` |
| Inspector | `apps/web/src/components/AtlasInspector.tsx` |
| Add place | `apps/web/src/components/AddPlaceDialog.tsx` |
| Encoders/vocab | `apps/web/src/lib/atlas.ts`, `apps/web/src/lib/map-vocabulary.ts` (+ `.test.ts`) |
| API client | `apps/web/src/lib/api.ts` (`fetchMapFeatures`, place CRUD, geospatial) |
| Search validation | `apps/web/src/route-tree.tsx` |
| Backend map | `services/core-api/internal/geography/service.go` |
| Backend radius/clusters | `services/core-api/internal/geospatialintelligence/repository.go`, `analysis.go`, `service_test.go` |
| E2E | `apps/web/e2e/journeys/08-browse-map.spec.ts`, fixtures in `apps/web/e2e/fixtures.ts` |

## 6. Guardrails (do not regress)

- Bounded responses (`MaxMapFeatures=5000`, `Truncated`) from plans 001/002/007.
- Visibility/role scoping (`/places` stays a public read path; private places only for their author via `credentials: "include"`).
- No Hijri↔Gregorian conversion; no tile basemap; no modern borders; regions stay hatched approximate claims.
- Every number/route/region derivable from fetched data — never constructed in a component. The one
  enrichment, region `placeCount`, is derived server-side in the same visibility policy the place layer
  answers to, so a component never counts markers into a region and never gets it wrong silently.
- E2E asserts via accessible names, never WebGL pixels.
- Seed data has `claim_id NULL` on geographic rows — the "no claim" inspector sentence is the honest one unless a seed row is given a real claim.
