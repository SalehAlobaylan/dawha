# خمس خطط للاستفادة من مصادر الجغرافيا القبلية — Five Utilization Solutions

**Status:** research (2026-11-15) · **Depends on:** `docs/tribal-territory-sources.md`, `docs/external-data-sources.md`, `plans/028-external-data-leverage.md`, `plans/029-geo-atlas-corrections.md`

The map corpus (Admiralty 1916, Dickson, Ottoman, Jordan/Yemen/Saudi plates) and the text corpus (Lorimer, Handbook of Arabia, Gazetteer of Arabia, OpenITI genealogies) are *raw material*. Displaying them is one solution among five. Each solution below is developed to the same depth: inputs → mechanism → outputs → utilization → effort/risk. All external data lands as `platform_inferred` until review, per platform rules.

---

## S1 — الأطلس القبلي التفاعلي (Interactive Tribal Atlas) — *the display solution, developed further*

**Problem it solves:** static plates answer "where was tribe X?" for one era only.

**Mechanism (from display → spatio-temporal atlas):**
- Era slider (مِطبقات): جاهلي / صدر الإسلام / عثماني / حدود ١٩١٦ / معاصر — reusing the classical fivefold division already seeded via `build-region-polygons.py`.
- Layer toggles by genealogical altitude (اتحاد → قبيلة → فخذ) driven by `branches` parents, not manual grouping.
- Certainty shading: `precise | approximate | disputed` polygons draw at different opacity + hatched borders; disputed plates open the two-source side-by-side on click.
- Every polygon click opens a provenance popover: source scan crop, passage link, status, reviewer — receipts, not claims.
- Shareable per-era permalinks (e.g. `/atlas?era=1916&tribe=shammar&layer=dira`) so the atlas becomes citable inside forum discussions and school material.

**Utilization (why more used):** the atlas is the landing surface every other solution feeds (S2 extracts into it, S3 pages link into it, S4 animates it, S5 certifies it). Permalinks and embeddable states turn one visitor into a returning researcher; tribal communities themselves become the review pool for their own tribal polygons.

**Effort/risk:** Medium. Frontend is MapLibre + existing `AtlasMap.tsx` stack; risk is polygon quality redrawing as "fact" — mitigated by certainty shading + popover receipts.

---

## S2 — استخلاص المصادر المكانية (Spatial Extraction & Reconciliation Workbench) — *the production engine*

**Problem it solves:** nothing tribal is ingested yet; doing it file-by-file by hand doesn't scale to 7 gazetteer volumes × 5+,00 pages.

**Mechanism:**
1. Live probes (already built in the research): Wikidata SPARQL → tribe QIDs + aliases; ar-wiki category API → 86-subcategory harvest; archive.org/Lorimer-derived OCR via Poppler/Tesseract (repo-standard).
2. Map tiles → the `chart_vision.py` locate→crops pattern generalized from family charts to territory maps; vision output = `{tribe, subtribe, anchor_toponyms, era, boundary_segments}`;
3. Toponym reconciliation: extracted anchors matched against `places` / `historical_place_names` (GeoNames + extracted gazetteer entries), creating candidate pairings in the same adjudication lane as `external_place_pairings` — reviewers accept/reject polygon-to-region matches, not raw pixels.
4. Every output row: `source_id` + checksum + `status = platform_inferred`, feeding `spatial_evidence`.

**Utilization:** a standing data factory — each newly ingested scan automatically enriches atlas (S1), dictionary (S3) and events (S4). The WORKBENCH is the more-utilized form: extraction not as a fire-and-forget import but as a queue reviewers can keep adjudicating over months.

**Effort/risk:** High. Vision reliability on dotted boundaries at 7163px is the main risk; mitigation = tile-level crops + anchor-town registration + reviewer queue (no auto-accept).

---

## S3 — معجم القبائل الجغرافي (Geographic Tribe Dictionary Dossiers)

**Problem it solves:** the dictionary (`entity_type='tribe'`) has data about a tribe; the atlas has geography; classical passages have text. Nobody joins them.

**Mechanism:** per-tribe dossier tabs, each grounded:
- الأسماء (names): Wikidata aliases + ar-wiki titles via `build-name-variants.py` → `tribe_aliases`;
- النسب (lineage): `branches` skeleton seeded from جمهرة أنساب العرب / جمهرة النسب scans;
- الديار (territory): atlas polygons + classical passage quotes (من ياقوت/الحمَداني) side-by-side;
- الهجرة (movement): rolled-up `migration_events`;
- الأثر القانوني (receipts): every statement shows its source row.

Plus a **nisba hook** into the name-reading pipeline: a candidate reading "الحربي" suggests حرب's dossier + `tribe_aliases` match (the `name_candidate_common.json` vocabulary already encodes exactly these nisba forms).

**Utilization:** makes the dictionary the *primary navigation* for genealogists — searches by surname surface geography and sources; dossier URLs become the citation target for external discussions.

**Effort/risk:** Medium. Mostly aggregation over tables that exist (0002/0006 + dictionary service); risk = dossier claims looking stronger than their evidence if tabs mix statuses — every tab must carry its own status chip.

---

## S4 — المرحلة و الرحل (Migration & Events Timeline)

**Problem it solves:** gazetteers record *drift* (Shammar north; Ḥarb east-of-Medina expansions; Ateibab secession of dira over centuries), and the classical أيام العرب sources record conflicts — events the atlas's era slider alone can't express.

**Mechanism:**
- Extract event passages (Handbook of Arabia movement notes; Lorimer tribal sections; أيام العرب from OpenITI literature) → `migration_events{from_place, to_place, era_window, claim_id}`;
- Visualize as animated routes + phase bands on the S1 atlas; each event page = claim with its source passage + confidence chip synced with the map's era slider;
- Support "tribe trajקtory" comparison views ("أين كانت حرب في كل مرحلة؟").

**Utilization:** highest-engagement narrative product; also the causal layer that makes the era slider *mean* something (polygons change because events happened). Time-boxed demos make great external presentations.

**Effort/risk:** Medium. `migration_events` schema exists; risk = dragging classical genealogy into pseudo-precise trajectories — keep every event window as `approximate` unless the passage names both endpoints.

---

## S5 — محرّك الأدلة المكانية (Evidence & Adjudication Engine over RAG/GraphRAG)

**Problem it solves:** researchers ask "موطن قبيلة شمر عام ١٩١٦؟" and today the answer is at best a wiki paragraph. The sources disagree with each other (Dickson vs Admiralty vs عثماني plates) — disagreements must become *features* (post استخدامها as conflicts), not noise.

**Mechanism:**
1. Ingest OCR'd Lorimer / Gazetteer of Arabia / Handbook of Arabia + OpenITI نسب corpus into the hybrid RAG/GraphRAG layer the product spec already plans;
2. Passage-level retrieval returns citable receipts (`source_id`, quote, checksum, license) — never merged statements;
3. When two passages/place-claims conflict on the same tribe-era footprint → a `disputed` candidate with both passages side-by-side enters the reviewer queue (same pattern as the extraction workbench queue); agreement across ≥2 independent sources can auto-promote to `documented` for review confirmation;
4. Feeds صحيفة الأثر (license dashboard): every claim's source chain visible.

**Utilization:** the trust layer behind S1–S4; also the fastest path to answering ad-hoc scholarly/community questions with citations. Utilization metric: research questions answered with receipts per month.

**Effort/risk:** High. Retrieval quality over classical OCR (إعراب/تشكيل variants) is the risk; mitigation = the name-variant matching pipeline first, then passage retrieval; ground answers only in the corpus.

---

## Sequencing

Build order: **S2 → S3 → S1 (+S4 hooks) → S5 full**. S2 produces the data the other four consume; S3 aggregates it into the dictionary genealogists actually browse; S1 makes it visible and citable; S4 deepens engagement; S5 certifies and accelerates. Rough effort: S2 3-4 weeks · S3 2 weeks · S1 3 weeks · S4 2 weeks · S5 3-4 weeks (overlapping; S2's reviewer queue and S1's certainty shading must exist before any portion is public).

---

# v2 — التطوير العملي (patterns adapted from external platforms)

Reviewed live: **ORBIS** (Stanford geospatial network model of the Roman world — orbis.stanford.edu), **Allmaps** (IIIF georeferencing ecosystem — allmaps.org), **Recogito2** (Pelagios semantic annotation, Apache-2.0 — github.com/pelagios/recogito2), **KITAB / OpenITI** (classical-Arabic text-reuse research programme — kitab-project.org), **Scite** (Smart Citations: supporting vs contrasting evidence — scite.ai). Patterns cited from knowledge: OSM Tasking Manager, Label Studio / Prodigy, StoryMapJS, FamilySearch record hints, WikiTree sourcing badges, Geni merge UX.

| Pattern (source platform) | What it actually does | Adopted into |
|---|---|---|
| Georeference Annotations, IIIF/W3C standard (Allmaps) | georeference stored as open annotation data; map warped **client-side** in the browser; `@allmaps/maplibre` plugin renders over MapLibre | S1, S2 |
| **Georeference by transcribed toponyms** (Allmaps) | geocode the toponyms from a map's gazetteer to auto-place it — no manual control points | S2 (the bootstrapping trick, §S2.3) |
| Gazetteer authority files (Recogito2) | annotation UI binds text/image spans to a curated gazetteer upload | S2 |
| Seasons + route/network/flow modes (ORBIS) | diachronic movement computed per-season with mode & priority | S4 |
| Text-reuse graph (KITAB/passim) | detect when sources copy one another → independence weighted, single-origin detected | S5 |
| Smart Citations: supporting / contrasting tallies (Scite) | per-claim evidence classification with one-click verification to the exact sentence | S3, S5 |
| Task grids + claim/validate (OSM Tasking Manager) | volunteers pull small bounded tasks from a projected grid; validation lane | S2 |
| Pre-annotation + accept/reject queue (Label Studio / Prodigy) | model proposes; human confirms keystroke-first | S2, S5 |
| Payment-free URL state (Allmaps: "it's all about the URL") | every map state / annotation / dossier is a shareable, embeddable citation | S1, S3 |
| Record hints (FamilySearch) | new ingested source auto-generates candidate statements on relevant dossiers | S3 |
| Challenge/merge with conflict resolution (WikiTree, Geni) | disputes as first-class workflows, not comments | S3 |

## S1 v2 — Evidence Atlas on Allmaps mechanics

- Store the Admiralty 1916, Dickson, and Ottoman plates as IIIF images; georeference **once** in Allmaps Editor; keep the Georeference Annotations (open standard) as project data. Render via `@allmaps/maplibre` (the web app is already MapLibre) — client-side warping, no GeoTIFF infra.
- Opacity slider + split-view "old vs modern"; an Allmaps-Here-style panel shows the **same** map point across every plate (1916 ↔ Dickson ↔ Ottoman) — the comparative view that makes contradictions legible.
- Era slider (مِطبقات) drives which polygons draw; every polygon click = provenance popover (scan crop, passage, status, reviewer).
- ORBIS-style "flow" toggle: beside dira polygons, draw alliance/raid/relation arcs from `entity_relationships` for the selected era.
- All state in the URL (era, tribe, layer, opacity, view-mode) → permalinks are citations; embeddable iframe for researchers' blogs and curriculum use.
- **MVP slice:** georeference the 1916 plate + Dickson (hours of work, zero infra, data lives in Allmaps/our repo), overlay the 10 largest tribal polygons manually traced as GeoJSON → first `historical_regions` rows with `certainty='approximate'`.
- **Metric:** atlas sessions with ≥1 permalink shared.

## S2 v2 — Toponym-bootstrapped annotation factory

1. **Grid projects** per source: each gazetteer volume / map plate decomposed into tile tasks (Tasking Manager pattern); contributors claim one tile; per-tile retirement = 2 independent confirmations.
2. **Text lane (Recogito-style):** OCR'd Lorimer / Gazetteer of Arabia / Handbook text opened in an annotation UI; toponym & tribe-name spans bound to dawha's own authority files (`places`, `tribes`) instead of an external gazetteer; Lorimer entries carry their printed lat/long, so every confirmed span is also a `places` seed.
3. **Map lane with the Allmaps bootstrap:** gazetteer coords of recognized toponyms become **automatic control points** to georeference the plates (Allmaps' transcribed-toponym method); then vision-extracted tribal labels (S2's `chart_vision` adapter) are projected through the same GcpTransformer from image pixels to coordinates. Text discovers the map; the map confirms the text — **bidirectional validation** with no manual control-point work.
4. Queue UX follows Label Studio / Prodigy: model pre-annotates, reviewer accepts/rejects (keyboard-first), everything lands as `platform_inferred` `spatial_evidence` rows.
- **MVP slice:** one chapter of the Gazetteer of Arabia Vol. I annotated end-to-end (≈50 toponyms + 5 tribal areas) to calibrate throughput and error rates.
- **Metric:** accepted rows/week; km² of dira reached `documented`.

## S3 v2 — Dossiers with Smart-Citation badges and hints

- Every dossier claim shows a Scite-style badge stack: ✓ يدعم ×٣ (three supporting sources) / ✗ يناقض ×١ / ○ مذكور بلا دليل — tallies computed over `spatial_evidence`/claims with their passages; one click opens the exact sentence (OpenITI permalink for classical text; scan page-anchor for colonial plates).
- **Record hints** (FamilySearch pattern): when S2/S5 ingest a new source, matching dossiers receive hint cards ("١٧ إشارة لحرب في Lorimer 1908") — accepting a hint is the review action that moves rows `platform_inferred → documented`.
- **Challenge & merge flows:** disputing a claim opens a structured challenge thread (WikiTree-style) that flips status to `disputed` until adjudicated; duplicate tribe pages enter a Geni-style merge lane reusing the `external_place_pairings` adjudication machinery.
- Dossier URL = shareable citation; the nisba hook from `entity_relationships`/name-reading suggests dossiers from surname research.
- **MVP slice:** 10 flagship dossiers (حرب، شمّر، عتيبة، قحطان، مطير، بني خالد، عنزة، بني تميم، غامد، يام) with badge stacks, hints, and one challenge each to exercise the flow.
- **Metric:** hint-acceptance rate; challenges resolved/week.

## S4 v2 — Seasonal movement engine à la ORBIS + event taxonomy

- Corridors, not just polygons: encode known migration corridors (e.g. ʿAnazah's northward drift, Ḥarb's Medina-facing expansion, śayf circuits into Najd/Levant pastures) as **polylines with travel-day stages** sourced from Handbook of Arabia / Lorimer (their camel-pace day-stage measurements are the ORBIS "cost function"), selectable by mode (camel/horse/foot/sea) and **season (صيف/شتاء)** — ORBIS' seasonal departures, adopted.
- Event layer from ADER? No — **ACLED's event-type taxonomy adapted to the classical repertoire**: يوم / غزوة / سرية / نفير / منا? mapped to typed events with actors (tribe entities) and geometry, stored in `migration_events` + `entity_relationships`; markers erupt over the era slider.
- StoryMapJS-style narrated journeys ("رحلة عنزة شمالاً") = shareable slides of era snapshots: map + quote + receipt per slide — the public-facing narratives built from S1+S4 data.
- **MVP slice:** 3 corridors (ʿAnazah N, Ḥarb E, Shammar N) + 10 أيام العرب events, all passage-cited.
- **Metric:** events with independent receipts; corridors encoded with stage counts.

## S5 v2 — Evidence engine with reuse-aware independence (Scite + KITAB patterns)

- Ingest OpenITI نسب corpus + OCR'd gazetteers into passage-level claims (checksums mandatory, as in `docs/external-data-sources.md`).
- **Text-reuse graph (KITAB/passim pattern):** when the same dira description propagates across sources (Yāqūt → taghlib-lorimer-later compilations), cluster as one textual lineage so **independent corroboration is weighted, not double-counted** — "٣ مصادر تدعم" only when the 3 are independent by the reuse graph.
- Supporting/contrasting classification (Scite pattern) at passage level drives: S1's disputed side-by-side views, S3's badges.
- **Reference Check (outward-facing):** paste a paragraph from a forum/book; the engine marks every geographic claim with its status in the graph (documented / disputed / platform_inferred / unresolved) — turning the corpus into a verification service for the community, not just an importer.
- Expose the engine over **MCP** (Scite's own MCP precedent) so external AI assistants answer from the certified corpus with receipts — utilization multiplier beyond the web app.
- **MVP slice:** ingest one Lorimer tribal chapter; run reuse detection against the OpenITI نسب literature for those passages; produce the first "independent vs circular" support report for five tribes.
- **Metric:** % of atlas claims with ≥2 independent non-circular receipts; reference-checks run/month.

## Sequencing (v2 refinement)

The **toponym bootstrapping (S2.3)** is the fastest measurable win: georeferencing the plates is fed by the gazetteer coords we were going to extract anyway — no separate control-point project. Order stands: **S2 (with the bootstrap) → S3 dossiers → S1 atlas → S4 → S5**, with S3's hint pipeline and S1's permalinks coming online as soon as the first adjudicated batch exists.
