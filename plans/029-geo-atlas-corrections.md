# Plan 029: Make every geo answer on the atlas agree with the question it draws

> **Status: implemented 2026-11-12** — every phase below is in the working tree:
> A (loop by construction, selection survival, `count(DISTINCT)`), B (the
> LATERAL best-evidence join, the line viewport, edge eligibility; the D3
> extension was **dismissed on evidence** — see stipulation 1's finding), C
> (`datedInWindow`, the weakest voice, never hidden), D/E (subject dedupe,
> shoelace family, dead code, single-flight deep links, one setState per frame,
> one palette source), and F (gates: `make verify` green, 184 vitest,
> geography/geospatialintelligence/httpapi green, journey 08 11/11 on the
> seeded stack). The e2e investigation also uncovered and fixed a REAL
> 028-D defect the plan had not named: a documented مَسالِك event double-drew
> as both a main-layer migration and a subject-layer segment *under one
> feature id*, i.e. duplicate React keys in `.atlas-row-grid` — undefined
> reconciliation that stranded stale rows in the DOM after every data change;
> the subject layer now contributes only what the main layer does not already
> draw. Journeys 07/09 fail on this tree for reasons predating 029 (their
> locators were never updated after `4957d0e`'s research-workspace
> consolidation) — see `plans/README.md`'s 029 row for the note and ownership.

> **Executor instructions**: Do not weaken visibility, provenance, or the bounded
> response guarantees from plans 001/002/007/027/028. Every count the atlas draws
> stays server-derived through the visibility predicates (`ReferencePredicate`,
> `PersonPredicate`) — never computed in a component. Nothing here re-generates
> region geometry, touches `MaxRegionRingVertices`, or changes the no-conversion
> date contract (`docs/phase-status.md` Phase 10). The map rule still holds: a map
> that draws confidently is a map that lies fluently.

## Status

- **Priority**: P1
- **Effort**: M total; phases A/C/D/E are S each, B is M; independently shippable
- **Risk**: LOW for A/D/E; MED for B (it changes numbers tests pin)
- **Depends on**: 027 (DONE), 028 (in the working tree, gates green)
- **Category**: geo-correctness + provenance-UX
- **Planned at**: 2026-11-12, from a full read of the atlas front end and
  `internal/geography` + `internal/geospatialintelligence`

## What the analysis verified as sound — do not "fix" these

These were checked line-by-line and are correct as built. An executor that
"improves" one of them should expect the review to come back worse.

| What | Where | Why it is right |
| --- | --- | --- |
| One predicate set for four feature queries | `internal/geography/service.go:165-253` (`mapFilters`) | status/place/year/viewport fragments share one param numbering with the visibility predicates; queries cannot drift |
| SQL year rule ≡ Go year rule | `service.go:214-221` vs `apps/web/src/lib/atlas.ts:221-228` | both keep a row with no dates under every window; both skip a null-sided check. Verified equivalent on the one-sided cases |
| Bounded response with a leftover probe | `service.go:282-310` | truncation is reported as "bound spent, rows remain" with one extra 1-row query — not a count of the table |
| Drawn-status filter for regions | `service.go:193-196, 522, 553` | the filter compares the *drawn* status (`CASE WHEN certainty='disputed'`), not the column, so `status=interpreted` finds regions |
| `loadSpatialEdges` geography cast + latitude-aware expand | `geospatialintelligence/repository.go:524-568` | the longitude-safe `ST_Expand` term is always ≥ the latitude requirement (`1/cos ≥ 1`), so the `0.5°` additive cap is a margin, not a clip |
| Cluster/edge decoupling | `analysis.go:257-313` | clusters consume only edges whose **both** association ids are in the loaded (visible) set, so the un-statused edge query below cannot create a phantom cluster |
| Region polygons stored whole | `infra/local/build-region-polygons.py:181-197` | `buffer(0)` guarantees simple rings; overlap is advisory by design so the borders a user checks against Google are not silently carved |

## The defects the analysis found — each becomes a phase below

| # | Defect | Evidence |
| --- | --- | --- |
| D1 | **300 ms render loop whenever an era window is in the URL.** `fetchEraWindow` is a fresh object identity every render; `useDebouncedValue` keys its effect on `[value, delay]` and does `setDebounced(value)`, so with an object `Object.is` always fails → re-render every ~300 ms for as long as `?from`/`?to` is set | `apps/web/src/routes/places.tsx:98-100, 624-631` |
| D2 | **`personCount` counts associations, not persons.** `count(*)` over `ga JOIN people` with no `DISTINCT ep.id`, while the inspector labels it «أعيان موثَّقون» (notables). One person with two associations in the ring reports 2 notables | `service.go:536-543`, label at `AtlasInspector.tsx:177-181` |
| D3 | **`associationCount` ignores family/tribe/branch visibility.** It counts associations of entities the *layer itself* would drop (only persons get `PersonPredicate`). A hidden family's association leaks its existence into a region's era readout | `service.go:531-535` vs the four entity predicates at `:438-441` |
| D4 | **A `spatial_evidence` 1:N join duplicates map features.** `spatial_evidence` has no unique constraint on `geographic_association_id` (migration `0006:74-84`), and `associationFeatures`/`migrationFeatures` LEFT JOIN it raw. Two evidence rows on one association = two features = an inflated marker count and duplicated documentaiton. The geospatialintelligence loader already deduplicates with best-evidence ranking | `service.go:436, 481` vs `repository.go:392, 416` (`DISTINCT ON (ga.id) ... ORDER BY ga.id, accepted-first`) |
| D5 | **Migration viewport predicate tests one endpoint point, not the line.** `COALESCE(pt.geometry, pf.geometry)` — a Riyadh→Kuwait route dropped from a viewport that covers the middle of the route but neither endpoint. Regions already solve exactly this with `geometryViewportPredicate` | `service.go:464, 493` vs `:243-253` |
| D6 | **An undated region impersonates an era.** Regions with NULL `valid_from`/`valid_to` pass every window (`timePredicate`'s no-dates branch), so all 13 demo polygons survive every era chip and read as "این region existed in this period", which the record never says | `service.go:555`, schema `db/migrations/0006_geography.sql:3-16` |
| D7 | **`loadSpatialEdges` applies no status/visibility predicate**, unlike every sibling loader. Clusters intersect by loaded ids so nothing leaks, but the query ships edges no cluster may ever consume, and its cost grows with the whole association table | `repository.go:544-568` vs `:407-415` |
| D8 | **Changing the year window clears the selection.** `changeYears` navigates `feature: undefined, place: undefined`, so a reader checking "was this in the era?" loses the thing being checked mid-drag | `routes/places.tsx:313-327` |
| D9 | **Subject layer sits above every filter** — status pills, era window and `q` never touch `subjectFeatures` | `routes/places.tsx:213` |
| D10 | **Overlay projection does four `setState`s per animation frame**, replacing one `Record` each — the first jank suspect at high marker counts | `AtlasMap.tsx:227-282` |
| — | Plus: dead `filters.placeId` branch (`atlas.ts:274`), `?? 0` latitude fallbacks on a guarded path (`AtlasMap.tsx:264-267`), two statements on one line (`AtlasMap.tsx:69`), the inspector's empty-state encoding key duplicating the always-on legend (`AtlasInspector.tsx:112-116` vs `places.tsx:468-477`), status hex values in three files (`AtlasMap.tsx:47-48`, `map-vocabulary.ts`, `places.tsx:471`) | |

## Binding design stipulations

1. **Counters answer through the same visibility predicates as the layers.** When a
   predicate appears in a layer query and the matching counter omits it, that
   asymmetry is a defect (D3), not a nuance. **Implementation finding (recorded
   2026-11-12):** the person case is pinned by
   `TestRegionEraCountsMatchAnIndependentDerivation` as *deliberate* — a hidden
   person's association raises the era count without raising the person count,
   because the counter is an **existence reading** over documented/disputed rows
   and never names an entity. The family/tribe/branch "gap" (D3) is therefore that
   same deliberate reading, not an omission: the extension is dismissed, the
   counter's basis stays, and the inspector's caption (already stating the
   documented/disputed basis) is the disclosure. **Do not** add entity predicates
   to `associationCount` — it would contradict the pinned semantics.
2. **Dedup keeps the response contract's order.** The layer queries are ordered
   `time_from NULLS LAST, created_at DESC, id` and the LIMIT is applied across the
   concatenation. A naive `DISTINCT ON (ga.id)` would force `ORDER BY ga.id` and
   reorder the bound response. The dedup must be a LATERAL first-evidence join
   (subselect: `ORDER BY review-ranking, created_at LIMIT 1`), which keeps the outer
   order untouched.
3. **Undated is drawn, not hidden.** D6's fix never removes a region from a window
   — hiding it would claim the region did not exist then, a claim nothing in the
   archive supports. It gets a *weaker visual voice* and a sentence instead.
4. **Journey assertions stay relative** (accessible names, presence/ordering), per
   the 027/028 precedent. No WebGL pixels.
5. **`useDebouncedValue` loses the ability to loop.** Its parameter type narrows to
   a primitive (`string | number | null`), so passing a per-render object stops
   typechecking. D1 is then fixed by construction, not by memory.

---

## Phase A — The loop, the lost selection, the mislabeled count (S)

**Goal**: the three defects a reader meets in one session, with no data change.

1. **A1 (D1)** — `apps/web/src/routes/places.tsx`: change
   `useDebouncedValue` to accept `string | number | null` only; replace
   `useDebouncedValue(fetchEraWindow, 300)` with two primitive debounces of
   `search.from ?? null` and `search.to ?? null`, recomposed into the fetch window
   (both-or-neither, matching `parseAtlasSearch`'s window pairing at `atlas.ts:373-374`).
2. **A2 (D8)** — `changeYears` keeps `feature`/`place` in the URL. The inspector
   already reads its selection from the *unfiltered* feature set
   (`selectedFeature` over `[...features, ...subjectFeatures]`), so the chamber
   and its era readouts update in place instead of vanishing. Selection clearing
   stays on status/kind toggles, where the result set changes wholesale.
3. **A3 (D2)** — `service.go` `personCount`: `count(DISTINCT ep.id)`. Update the
   windowed-counter test (`service_test.go`, independent hand-SQL derivation) to a
   fixture where one person has two associations in the ring/window and assert
   `associationCount = 2`, `personCount = 1`.

**Acceptance**: `npm run typecheck` clean with the narrowed hook (A1 cannot
regress silently); geography test with the two-association/one-person fixture
green; journey 08 still fully green.

---

## Phase B — Same question, same predicates (M)

**Goal**: every count and layer answers through the predicates its sibling answers.

1. **B1 (D3)** — `associationCount` gains the family/tribe/branch visibility
   predicates, mirroring `associationFeatures`: person rows already gated by
   `PersonPredicate`; family/tribe/branch through three `LEFT JOIN`s +
   `ReferencePredicate`s inside the correlated subquery (or, equivalently, an
   `OR EXISTS` chain if that keeps the subquery readable). Extend the counter
   test with a hidden-family association inside the ring and assert it is
   absent from the count while the same fixture drops it from the layer too.
2. **B2 (D4)** — `associationFeatures` and `migrationFeatures` replace the raw
   `LEFT JOIN spatial_evidence` with a LATERAL best-evidence join:
   `LEFT JOIN LATERAL (SELECT se.id, se.evidence_text_ar, se.review_status FROM spatial_evidence se WHERE se.geographic_association_id = ga.id ORDER BY CASE review_status WHEN 'accepted' THEN 0 WHEN 'needs_review' THEN 1 ELSE 2 END, se.created_at LIMIT 1) se ON TRUE`
   (and the `migration_event_id` twin) — the same ranking `repository.go:416`
   already defined, so a feature's shown evidence is *the best* evidence rather
   than *one arbitrary* row, and the count is the association's. Test: one
   association with an accepted and a needs_review evidence row → exactly one
   feature, evidence = the accepted one; `truncated` counting unaffected.
3. **B3 (D5)** — `migrationFeatures` viewport predciate moves from the endpoint
   point to the geometry: `COALESCE(ST_MakeLine(pf.geometry, pt.geometry), pf.geometry, pt.geometry)` 
   fed to `geometryViewportPredicate`, so a mid-route viewport returns a route
   whose ends are elsewhere (the case regions already honour). But this must also work when only one endpoint has a position: `COALESCE(ST_MakeLine(pf.geometry, pt.geometry), pf.geometry, pt.geometry)` — the `COALESCE` chain handles the one-place migration case.
4. **B4 (D7)** — `loadSpatialEdges` carries the same status list as
   `loadAssociations` (`ga.status IN ('documented', 'interpreted', 'platform_inferred')`),
   so the edge set is a subset of the set clusters can consume and the query stops
   shipping edges nothing can claim. Clusters' behaviour is unchanged by
   construction (`analysis.go:283-286` already intersects); the test pins the new
   invariant: a place pair whose only associations are `unresolved` produces zero
   edge rows.

**Acceptance**: `go test ./internal/geography ./internal/geospatialintelligence ./internal/httpapi -count=1` green; the four new/extended fixtures above assert the parity, not the absence of rows; `gofmt -w` on changed files. The existing
`TestGeospatialRadiusFilterRespectsMetricDistance` and cluster separation tests
must remain untouched and green.

---

## Phase C — An undated region must not impersonate an era (S)

**Goal**: D6, without hiding anything.

1. `service.go` `regionFeatures`: when a year window is active
   (`input.FromYear > 0 || input.ToYear > 0`), each region gains
   `datedInWindow *bool` (`json:"datedInWindow,omitempty"`): `r.valid_from`/`valid_to`
   overlapping the window → `true`; either bound NULL → `false` and the feature is
   still returned (URI unchanged; `omitempty` keeps windowless responses byte-compatible).
2. Canvas: a region with `datedInWindow === false` renders in the *weakest voice*
   the grammar has — the vocabulary's hollow treatment (dashed outline
   opacity-down, fill near-invisible) — visually saying "record exists, era
   unproven". A fifth vocabulary entry in `map-vocabulary.ts`, like every other
   state: glyph/handle in the legend, not a color-only change.
3. Inspector: under an era window, an undated region's readout says
   «المنطقة بلا تواريخ مؤرَّخة؛ ظهورها في نافذة العهد لا يعني تواجدها فيها» —
   the same stated-absence form the era counters already use.
4. Tests: service — NULL-validity region + window → feature present,
   `datedInWindow=false`; dated region overlapping → `true`; dated region
   disjoint → `true` (it still has *its own* dated validity; the era counters
   carry the emptiness separately). Journey 08: era chip + undated region →
   inspector sentence accessible.

**STOP**: if the demo's 13 seeded regions turn out to all have NULL validity and
the fixture grammar change this forces onto `001_demo.sql` would need real dates
per region, stop — the generator's control points carry no dates, and inventing
them would be a fabricated attribution. Ship A–B and re-plan C against a seed
that records its own uncertainty honestly.

---

## Phase D — Canvas consistency (S)

1. **D9 (subject above filters)** — `subjectFeatures` passes through
   `filterFeatures` with the same `filters` the main layer takes, and the subject
   banner gains "الطبقة تابعة للتصفية" so the exception either disappears or is
   named. Journey 08 subject test updated to assert a filtered-out subject
   segment is absent.
2. **label fallback** — `atlas.ts` `featurePoint`'s ring-centroid fallback becomes
   the shoelace (area-weighted) centroid; nothing else reads `ringCentroid`
   (the server already sends `ST_Centroid` for regions at `service.go:519`), so
   the vertex-average half-truth loses its one unstable edge case. Test: a
   concave ring's computed point differs from the vertex mean and falls inside
   the ring's bbox.
3. **(nits in one commit)** — remove the `filters.placeId` branch (`atlas.ts:274`);
   drop the dead `?? 0` fallbacks at `AtlasMap.tsx:264-267`; split the
   two-statements-on-a-line at `AtlasMap.tsx:69`; delete the inspector's
   empty-state encoding key (`AtlasInspector.tsx:112-116`) — the legend is always
   on screen.
4. **focus double-flight** — when a `focus` exists at mount, `fitted` starts
   `true` so the peninsula `fitAll` is skipped and the map arrives once, on the
   feature. (Deep-link UX, no assertion change; journey 08's deep-link test must
   stay green.)

## Phase E — Canvas cost (S)

1. **E1 (D10)** — `updatePositions` builds one composite state
   (`{markers, inferences, regionLabels, captions, migration}`) and calls
   `setState` once per frame instead of four times; the render-side maps read
   from the bundle. Identical semantics, one reconciliation per frame.
2. **E2 (single palette)** — `map-vocabulary.ts` exports `mapStatusColors` (the
   hexes already there for the legend's stroke); `AtlasMap.tsx` builds
   `statusFills`/`statusStrokes` from it. Hexes then live in exactly one file.

---

## Phase F — Gates and docs (S)

1. `npm run typecheck && npm run lint && npm test && npm run build` (in `apps/web`).
2. `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./internal/geography ./internal/geospatialintelligence ./internal/httpapi -count=1` (in `services/core-api`).
3. `make verify`, then `make e2e` on the seeded stack (`COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed` first if needed) — journey 08 all tests.
4. Docs in the same commit (repo rule):
   - `plans/README.md`: flip **028** to **DONE** (all six workstreams and gates are
     recorded green; correct its counter sentence — the era counters answer the
     same *time and visibility* predicates, not the status pills, and the
     inspector caption already says so); add the **029** row with real evidence.
   - `docs/phase-status.md` Phase 10 rows that D3/D4/D5/D6 changed: add the new
     fixture names and the undated-region rule.

## STOP conditions

- Any change that would re-generate region geometry, touch `MaxRegionRingVertices`,
  or edit the region polygons' generated block. (Phase C's fixture STOP above.)
- Any visibility predicate weakening or any reader-scoped count computed on the client.
- Phase B changing the response *contract* (order, bound, field names) rather than
  the predicates beneath it.
- A status-filter param added to `fetchMapFeatures` to "fix" the counter basis —
  design stipulation 1 explicitly rejects it; reopen the design instead.
