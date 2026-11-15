# Plan 027: The Atlas — make the geo map one honest, provenance-first surface

> **Executor instructions**: Do not weaken visibility, provenance, or the bounded
> response guarantees from plans 001/002/007. Every number, route and region the
> map draws must be derivable from fetched data, not constructed in the component.
> The product rule "a claim is never a fact here" applies to the map more strictly
> than anywhere else: a map that draws confidently is a map that lies fluently.

## Status

- **Priority**: P1 (workstream A is a correctness defect) / P2 (workstreams C–D)
- **Effort**: L
- **Risk**: MED (SQL touch in the intelligence service; frontend layout rebuild)
- **Depends on**: plans/007-batch-hydration.md (map filters already in SQL), plans/001, plans/002 (visibility)
- **Category**: data-representation + ux/correctness
- **Planned at**: commit `4957d0e`, 2026-10-29
- **Audit basis**: deep review of the geo stack (frontend atlas + API map service + geospatial intelligence) against `docs/phase-status.md` Phase 10 and Phase 23

## Why this matters

The geo map is three stacked systems with very different maturity. The backend
(`internal/geography`, `internal/geospatialintelligence`, migrations 0006/0025) is
production-grade: bounded responses, SQL-pushed filters, real role scoping with
disclosure avoidance, pinned algorithm versions, idempotent finding persistence,
and graceful 503 without a database — all pinned by tests, and confirmed as
`implemented` in `docs/phase-status.md`. The frontend, however, still runs the
flagship `/places` atlas on four hardcoded demo places, keeps two parallel marker
systems in two coordinate spaces, has layer pills that filter nothing, inspector
copy that is constants, and a geospatial radius filter whose units don't match its
columns. The demo panel and the live panel sit 40 pixels apart and can contradict
each other. The result is that the strongest data surface in the platform is
shielded by the weakest presentation in it — and the presentation is the only part
the reader ever sees.

Three root causes produce every listed problem; the plan fixes these, not symptoms:

1. **RC1 — The page pre-dates its API.** `/places` was built demo-first
   (`apps/web/src/routes/places.tsx:4`) and never migrated to the `MapFeature`
   contract.
2. **RC2 — No cartographic grammar.** Status, certainty, time and kind are
   rendered ad-hoc in three components with two conflicting vocabularies
   (`موثق/مقترح/متنازع عليه` in the atlas vs the five backend statuses in the
   live panel).
3. **RC3 — Provenance is an afterthought on maps.** The product's one inflexible
   rule never got a cartographic expression, so decorative chrome (fictional
   routes, a fictional scale bar, hardcoded insight copy) draws with more
   confidence than the record underneath.

## Current state (audit findings, with evidence)

Correctness / data:

- **A1 · `ST_DWithin` unit mismatch (real bug).** `internal/geospatialintelligence/repository.go:524-537`
  passes meters (`input.RadiusKM*1000`, `service.go:122`) to
  `ST_DWithin(first_place.geometry, second_place.geometry, $1)` where `geometry`
  is SRID 4326 and the predicate therefore reads **degrees**. Any meters value is
  "within" for every pair of points on Earth. Consequences: (a) the `RadiusKM`
  control has no effect on clustering — `buildClusters`
  (`internal/geospatialintelligence/analysis.go:257-334`) consumes these
  pre-filtered edges and never re-applies the radius, while buildContradictions
  (`analysis.go:418-419`) does re-filter with a Go haversine, which is why the
  small local fixture in `service_test.go:115` still passes; (b) the predicate no
  longer prunes, so the pair query degenerates toward an association cross-join
  capped only by `LIMIT MaximumRecords+1`.
- **A2 · Two vocabularies.** Atlas legend and markers speak demo statuses
  (`apps/web/src/routes/places.tsx:29,46,52`); the API and the live panel speak
  `documented|interpreted|platform_inferred|disputed|unresolved`
  (`internal/geography/service.go:561-563`, `HistoricalMapPanel.tsx:54-74`).
- **A3 · Constructed data.** Hardcoded inspector stats `03 / 02` and fixed
  "ماذا نعرف؟" copy (`places.tsx:47-49`); a fictional SVG route overlay and
  "200 كم تقريباً" scale bar (`places.tsx:52`); region labels for نجد/الأحساء/حجاز.
- **A4 · Demo/live split-brain.** The atlas renders the four demo places from
  `data/demo.ts`; the live `HistoricalMapPanel` on the same page fetches
  `GET /api/v1/map`. Both visible at once.
- **A5 · Dead backend capability.** The client never sends the already-supported
  `min/max_longitude/latitude` viewport or `limit` (`fetchMapFeatures` —
  `apps/web/src/lib/api.ts:973-988`); no frontend client for
  `GET /api/v1/map/places/{placeID}` (`internal/httpapi/router.go:310`) or places
  CRUD (`router.go:362-366`).

Data representation:

- Regions render only as `ST_Centroid` points although
  `historical_regions.geometry` and `places.approximate_area` exist with GiST
  indexes (`db/migrations/0006_geography.sql:16`,
  `db/migrations/0002_identity_and_trees.sql:133`).
- Migration lines are clickable but carry no arrowhead, no direction reading, and
  no hover cursor (`HistoricalMapCanvas.tsx:66-72` registers cursor on points only).
- `MapResponse.Truncated` (`internal/geography/service.go:94-106`) is dropped at
  the client; the atlas cannot say "this is what fitted".
- Overdraw: features sharing one place render one marker per row with no
  aggregation or counts; period control offers three hardcoded decades
  (`HistoricalMapPanel.tsx:9-14`) while the API takes arbitrary `from_year/to_year`.

UX/UI:

- Two overlapping marker systems on one canvas: percentage-positioned DOM buttons
  (`place.x/y`) above geo-projected MapLibre circles; they only align at the
  default view (`places.tsx:52`).
- Layer pills toggle local styling state only; `MapLibreCanvas` takes no layers
  prop (`MapLibreCanvas.tsx:7-11`), `toggleLayer` filters nothing
  (`places.tsx:37`).
- Homepage `MapPreview` is decorative with dead toggles (`components/MapPreview.tsx:9,17-22`).
- No deep link of map state; inspector cannot hand off to research or to the claim.

Testing:

- Zero unit tests mount either MapLibre component, the `places` page, `MapFeature`
  encoders, or `fetchMapFeatures` (no jsdom canvas setup in `vite.config.ts`).
- E2E covers the demo atlas only (`e2e/journeys/08-browse-map.spec.ts`, 4 tests);
  nothing exercises `HistoricalMapPanel`, the map's error/loading states, or the
  geospatial intelligence panel.

## Target: what one feature is

One visible marker = one claim-with-evidence unit. The `MapFeature` contract
(`apps/web/src/types.ts:2156-2188`) already carries everything needed:
`kind`, `status`, `certainty`, `period` (through the one date contract in
`internal/dates`), `evidenceText`, `evidenceStatus`, `sourceTitle/sourceId`,
entity and geometry. The backend needs one targeted addition: expose `claim_id`
on association/migration features (columns already joined in
`internal/geography/service.go:367-448`) so the inspector can hand off to the
claim it serves. The bounded-response guarantee of plan 007 must not regress;
re-measure query counts and payload size after the change.

### The encoding contract (data representation)

| Attribute | Encoded by | Grammar |
|---|---|---|
| Status (epistemic layer) | fill + contour pattern, never color alone | table below |
| Kind (place / association / migration / region) | geometry itself: disc / point+line / line / polygon | no extra encoding |
| Certainty (`precise / approximate / uncertain`) | line style and ring weight | solid / dashed / dotted |
| Evidence count | monospace count badge (IBM Plex Mono) on aggregated markers | never marker size |
| Time | the scrubber; shape stays constant, membership changes | —

Status register, drawn within the existing palette
(`--ink #2d2520, --red #994438, --clay #b66f4d, --olive #747153, --sand #c79d62`):

| Status | Glyph | Arabic |
|---|---|---|
| `documented` | solid ink disc | موثق |
| `interpreted` | clay disc, dashed contour | مقروء |
| `platform_inferred` | olive **stamp ring** (rotated square seal, dotted) — the archive-stamp metaphor: the platform's reading is a stamp, not an ink-writing | استنتاجي |
| `disputed` | red disc with a split/ledger line through it | متنازع عليه |
| `unresolved` | hollow disc, muted contour | غير محسوم |

Each status differs in fill + contour + hue, so the encoding survives grayscale
and colorblindness. The stamp is the one signature element; everything else quiet.

### The three data shapes

1. **Aggregation with a truthful count.** *n* features sharing place+status
   render one marker with an `n` badge and expand in the inspector into the full
   evidence list. No average-of-statuses marker; a faded "both" disc is a lie.
2. **Regions as polygons, not dots.** Serialize ring GeoJSON (vertex-capped
   server-side, inside the 5,000-feature bound). A disputed region renders
   dashed + hatched: a border whose very existence is contested must look it.
3. **Migration lines that read as motion.** Arrowheads (symbol layer), dash
   by certainty, color by status, period text beside the selected line.

### The temporal dimension

The hard part is already implemented — the overlap rule including "a feature
with no dates is kept under any year filter" (`internal/geography/service.go:179-186`.
The frontend gets a dual-handle year scrubber across the union of fetched bounds,
with presets derived from the data (the "a control that lies about what it does"
rule, `places.tsx:31-34`). The toolbar shows `n إشارة من أصل m` so a filter's
effect is always legible. Optional presence-at-year playback honors
`prefers-reduced-motion`.

### Truncation honesty

When `Truncated` is true the atlas states it:
«أُعيد أول ٥٬٠٠٠ إشارة ضمن الحد» plus the active filters. The backend already
returns the flag; the surface must carry it.

## Target UX/UI (the atlas as a folio)

Keep the parchment/archival identity (`--canvas #e9dfc9`, IBM Plex Sans Arabic
body, Noto Kufi display) — it is distinctive and correct for lineage research.
Change the layout grammar: the map stops being an illustration inside the page
and becomes the page, with panels arranged like a manuscript's margins (RTL).

```
┌──────────────────────────────────────────────────────────────────────┐
│ TopBar — الخريطة الجغرافية · "الموضع جزء من السؤال"                      │
├───────────────┬──────────────────────────────────────────────────────┤
│ (RTL: اللوحة     │              MAPLIBRE CANVAS (fills)                 │
│ تبدأ يميناً)    │   points · migration arrows · region polygons        │
│               │   legend on-map with the real encoding               │
│ التصفية        │   DOM overlay ONLY for selection halo +               │
│  ▸ الحالات ×5  │     keyboard-focusable ring                           │
│  ▸ الأنواع ×4  │                                                      │
│  ▸ الفترة     │                                                      │
│  ▸ بحث بالموضع │                                                      │
│               ├──────────────────────────────────────────────────────┤
│ الفاحصة        │ ── التوقيت ──────────●━━━━●────────────────────────── │
│ (inspector)    │   «١٬٢٣٦ إشارة من أصل ٤٬٥٠٠ · التصفية: موثق، هجرة»    │
│  • what is claimed │                                                  │
│  • by whom (source → link)                                          │
│  • evidence text + its review state                                 │
│  • the claim it serves → فتح في البحث                                │
└──────────────────────────────────────────────────────────────────────┘
```

Decisions:

- **One canvas.** Delete the percentage-positioned DOM markers; DOM is used only
  for the selection halo and the keyboard-focus ring, positioned via
  `map.project()` per frame.
- **The inspector is a provenance chamber.** In order: the claim → the source
  (linkable) → the evidence text → the evidence's review state → the claim the
  feature serves. `MapFeature` already feeds 90% of this; add `claim_id`.
- **Functional layer pills.** The five status pills filter canvas data already in
  memory (no refetch); the API `status` param is only used when a single layer is
  asked for outright.
- **Every state has a sentence.** Loading skeleton; error = the existing Arabic
  503 message + retry; empty-with-filters = «لا توجد إشارات تطابق التصفية» +
  reset; empty truly = «لم تُسجَّل إشارات بهذه المنطقة بعد».
- **A11y floor.** Markers keyboard-reachable through a DOM overlay of visible
  features in the current viewport; `aria-live` announcement ("موضع: الرياض،
  موثق، ١٨ إشارة"); visible focus rings (pattern already in `styles.css`);
  reduced motion honored for zoom-to-selection and playback. E2E keeps the
  established rule: assert via accessible names, never WebGL pixels
  (`e2e/journeys/08-browse-map.spec.ts:5-13`).
- **Mobile.** Canvas first, inspector as bottom sheet, filters as a collapsible
  tray.

## Features (in priority order)

| Feature | Why it earns its place | Guardrail |
|---|---|---|
| Functional filters (status ×5, kind ×4, period, text) | baseline honesty of controls | data already in memory; API filters only for single-layer/refetch cases |
| Viewport fetch (debounced `moveend`, react-query keyed by it, abort) | the backend capability exists; big-data readability | 400 ms debounce; bounded by existing `MaxMapFeatures=5000` |
| Provenance chamber + `claim_id` handoff | the map's exit is the research flow | plan 007 bounds re-measured, not regressed |
| Deep-linkable state `?place=&years=&status=&kind=` | a map state is a research position | TanStack Router search params only |
| Aggregated markers + counts, regions as polygons, migration arrowheads | data representation contract above | server vertex cap |
| Dual-handle scrubber + data-derived presets | time is the map's missing axis | no invented calendar conversion (date contract stands) |
| Geospatial intelligence overlay (P2) | clusters + conflict findings on the map, each opening its reviewable finding | stamp-ring styling only; never drawn like documented data; review stays in the existing findings UI |
| Place CRUD client (P2) | unknown place names currently dead-end as `unresolved` | researcher-role gating as in the backend |
| Print/share snapshot (P3) | historians print | static export of rendered layers |
| Perf budget (P1) | debounced viewport, cancellation, capped payload | re-record payload/query count in plan 007's style |

## Workstream sequencing

| Phase | Contents | Gate |
|---|---|---|
| **027-A Truth (P0)** | A1 SQL fix + DB regression fixture; A2 vocabulary module; A4 single-source atlas (demo only through sanctioned `DEMO_MODE` channel); A3 remove constructed data and dead toggles | `make verify-full` + new DB tests; journey 08 extended with real-API assertions |
| **027-B Grammar (P1)** | Encoding table, aggregation, region polygons, arrowheads, scrubber, truncation banner, data-derived presets | unit tests for pure encoders (no WebGL assertions); payload/query count re-measured |
| **027-C Chamber (P1)** | Inspector as provenance chamber incl. `claim_id`; functional pills; deep links; empty/error states; a11y floor; mobile | e2e: inspector shows evidence text; keyboard walk; RTL mobile viewport smoke |
| **027-D Instruments (P2)** | Intelligence overlay; place CRUD client; intelligence panel coverage; print/share; perf budget | geospatial API tests extended; `docs/phase-status.md` Phase 10/23 rows updated in the same commit as the work (the repo's own rule, `phase-status.md:699-701`) |

## What this plan deliberately refuses

- A modern tile basemap rendering today's borders. Tileless/offline is a
  deliberate identity, and a base layer of current-day boundaries would import
  exactly the authority the product refuses to hold.
- Historical borders drawn as facts. Only hatched approximate areas labeled as
  claims.
- Any Hijri↔Gregorian conversion. The documented date-display contract stands
  (`docs/phase-status.md:269-279`); the `هـ` in demo strings remains a character
  in a string.
- Animations that imply certainty. The arrowhead says recorded route; the dash
  says hypothesis; never the other way.

## Steps

### Step 1 (027-A): Fix the radius predicate and bound the pair query

In `internal/geospatialintelligence/repository.go` use
`ST_DWithin(first_place.geometry::geography, second_place.geometry::geography, $1)`,
enumerate pairs over distinct places (prefilter with the geometry GiST via
`ST_Expand` in SRID degrees plus the geography refinement), then map place pairs
back onto associations in Go for clustering. Do not rescale the meters input by
hand (degrees ≈ meters/111320 is latitude-dependent and wrong at the margins).

**Verify**: a DB fixture with two far-apart places asserts a small `RadiusKM`
yields no spatial edge between them and that raising the radius past their
distance yields exactly the metric distance; a bounded-load test at the scale of
`map_filters_test.go` asserts the query no longer expands past the radius
(reasonable `EXPLAIN` bound). `go test ./internal/geospatialintelligence -count=1`
with DATABASE_URL all pass; `make verify` stays green.

### Step 2 (027-A): One vocabulary module

A new module beside the existing period vocabulary in `apps/web/src/lib/` (alongside
`periods.ts`) exports the single map of
`status → { label, glyph, tone, canvasStyle }`, `certainty → label`,
`relation → label`. Replace the three inline label functions in
`HistoricalMapPanel.tsx:54-74` and the demo-status ternaries in `places.tsx`
and the e2e selectors.

**Verify**: unit tests over the vocabulary map (every backend status value has
an entry; no demo-status literals remain in `src`). `make verify` passes.

### Step 3 (027-A): One source of features

`/places` renders one canvas fed by `fetchMapFeatures` (with `limit`, `status`,
years). Demo mode survives only through the sanctioned `DEMO_MODE` API channel.
Delete the demo import, the DOM percent markers, the SVG route overlay, the
fictional scale/region chrome, and the hardcoded inspector constants.

**Verify**: e2e `08-browse-map` rewritten to assert the API-backed journey
(including the error state when the API is absent). `make e2e` passes.

### Step 4 (027-B): The encoding contract in the canvas

Implement the status glyph table, aggregation with count badges (expandable in
inspector), polygon serialization for regions (`historical_regions.geometry`,
vertex-capped), arrowheads + dash-by-certainty on migrations, the dual-handle
scrubber with data-derived presets, and the truncation banner.

**Verify**: unit tests for pure encoders (feature→GeoJSON, aggregation, period
presets); backend test for region ring serialization and its cap; e2e legend
assertions via accessible names.

### Step 5 (027-B): `claim_id` on features

Add `claim_id` to the association/migration layer SQL and `MapFeature` TS; keep
the bounded-response guarantee (re-measure the two payload/query counters
recorded for plan 007 and annotate the result beside the test).

**Verify**: `go test ./internal/geography ./internal/httpapi -count=1`;
`make verify-full` for the bounded suite.

### Step 6 (027-C): Provenance chamber, pills, deep links, states, a11y

Inspector as the provenance chamber with the handoff link; functional status
pills over in-memory data; router search-param sync; the four states (pending,
error, filtered-empty, empty); keyboard overlay navigation with `aria-live`;
mobile bottom-sheet layout.

**Verify**: e2e journeys for inspector evidence text, keyboard walk, deep-link
restore, and mobile smoke; component tests for state sentences (assert text,
not pixels).

### Step 7 (027-D): Instruments

Intelligence overlay drawn exclusively in stamp-ring grammar with findings that
open the existing review UI; place CRUD client behind researcher gating;
intelligence panel e2e/unit coverage; print snapshot; final perf budget row.

**Verify**: focused `go test ./internal/geospatialintelligence ./internal/httpapi -count=1`;
e2e for the overlay and review handoff; `docs/phase-status.md` updated in the
same commit.

## Test plan

| Layer | Case | Expected |
|---|---|---|
| Go, DB | radius predicate: far-apart places, fixtures at two radii | no edge below distance, correct metric at/above |
| Go, DB | bounded pair query at plan-007 fixture scale | stable round-trips; no uncapped expansion |
| Go | region ring serialization + vertex cap | valid GeoJSON within bound |
| Go | `claim_id` joins without regress on bounds | plan-007 query counts hold |
| Web unit | vocabulary map completeness | every status has an entry; no demo-status literals |
| Web unit | feature→GeoJSON encoders, aggregation counts, scrubber presets | pure, deterministic, total order |
| Web unit | fetchMapFeatures with viewport/limit/abort + error states | contract matches API |
| E2E | journey 08 rewritten: API-backed atlas, filters, inspector, deep link, error state | passes on seeded stack |
| E2E | geospatial overlay opens its finding review | payload integrity |

## Out of scope

- Any tile-server/basemap procurement or integration.
- Calendar conversion or per-record epoch columns.
- Renaming the five backend statuses.
- Changing visibility semantics; the map stays a public read path with the
  role-scoped feature drops exactly as built (`internal/geography/service.go`,
  `internal/visibility/visibility.go`).
