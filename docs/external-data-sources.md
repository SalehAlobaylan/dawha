# External Data Sources for the Atlas

Research record: which internet/GitHub resources can actually advance the geo map atlas,
with licenses verified against canonical pages (not assumed), access patterns that were
exercised where possible, and how each source maps onto dawha's existing tables and
review flow. Nothing described here is implemented yet; this document is the input for a
future plan.

## Ground rules (unchanged by any external source)

- External data never enters as `documented`. Verified facts about *our* places come only
  from user sources through the provenance chain. External reference data lands as
  `platform_inferred` (status on `geographic_associations` / `migration_events`) or as
  review-gated seeds, always carrying a `sources` row and `source_id`.
- No modern borders, no tile basemap. Region geometry stays hand-drawn control points
  clipped to the committed Natural Earth 50m coastline. External sources may *guide*
  captions, counts, name variants, and literature — never redraw our outlines.
- Every import is reproducible: a committed script, a recorded fetch, the raw file stored
  as `source_files` (with `checksum_sha256`), and license/provenance in
  `sources.metadata` (jsonb).
- Arabic first: imports must fill `canonical_name_ar` / `name_ar` from the best Arabic
  label available; Latin names are secondary. Our `normalized_name_ar` rules run on
  import; unmatched rows fail closed.

## Verified sources

Verified 2026-11-06 from this environment (webfetch + unauthenticated GitHub REST;
`websearch` was unavailable). Sources marked "exercised" had their access pattern run
end-to-end during research.

### Wikidata — structured data, CC0

- What it gives: coordinates (`P625` as WKT), Arabic labels, instance-of (`P31`),
  country (`P17`), and daughter hierarchies for virtually every modern/historical entity;
  stable Q-ids usable as crosswalk keys.
- License: all structured data CC0 (project policy, verified via the project's licensing
  page).
- Access: SPARQL at `https://query.wikidata.org/sparql` (GET with `format=json` works
  without auth). Exercised: cities in Saudi Arabia returned with Arabic labels and
  `Point(lon lat)` literals, e.g. city الباحة. Etiquette: meaningful `User-Agent`,
  tight `LIMIT`s, and no concurrency spam; the endpoint is shared infrastructure.
- Fit: primary source for the reference layer (seas, mountains, historical cities)
  replacing hand-coded `map-reference.ts`, and for crosswalk ids.
- Caveats: modern entities dominate; "historical place" typing requires curated queries
  against `P31 → historical settlement / city` paths. Coordinates of modern cities are
  modern locations — fine for the reference layer, not silently treated as the historical
  settlement location.

### GeoNames — gazetteer, CC-BY 4.0

- What it gives: bulk toponyms with coordinates, feature classes (`H.` hydrographic,
  `T.` terrain, `ST.` populated-first-order), parent hierarchy (mappable to
  `places.parent_place_id`), and `alternateNames` including colloquial/Arabic variants.
- License: CC-BY 4.0 with attribution (verified on geonames.org/export terms).
- Access: daily dumps at `https://download.geonames.org/export/dump/` (`allCountries.zip`,
  feature files, `alternateNames`); documented delimiters/columns in the adjacent
  `readme.txt`. Free API needs a username with a 10,000-credit/day soft quota.
- Fit: name-variant enrichment (`historical_place_names`, `name_type='alternative'`) and
  candidate lists for gazetteer imports. The dump pipeline suits `source_files`
  checksumming better than the API.
- Caveats: massive and modern-tilted — filter by country code + feature class and curate
  before anything becomes reviewable; never bulk-import raw.

### PeriodO — period vocabulary, public domain

- What it gives: scholarly definitions of historical periods with start/end and spatial
  coverage, with stable URIs (e.g. `p0…`) for linking.
- License: public domain (stated on perio.do).
- Access: direct downloads `https://n2t.net/ark:/99152/p0dataset.{json,ttl,csv}`;
  browsable client at `client.perio.do`.
- Fit: a SQLite-free, JSON-artifact period vocabulary for a timeline filter on the atlas
  and for checking that region/name `valid_from`/`valid_to` windows are coherent with the
  period labels shown in captions.
- Caveats: coverage is strongest near Mediterranean/classics scholarship; Islamicate
  period labels useful here are a subset found inside collections, worth an audit step.

### World Historical Gazetteer (WHG) — reconciliation layer, per-dataset licences

- What it gives: reconciled crosswalks between historical place datasets (attestations,
  links to Jamestown/WorldCat etc. ids), plus downloadable peer datasets.
- License: every dataset/collection/reference gazetteer carries an explicit licence
  (verified on whgazetteer.org/licenses/); the reference gazetteer data is CC-BY-4.0-tier
  but read the per-dataset licence before import.
- Access: Swagger-documented API (`/api/schema/swagger-ui/`); interactive downloads and
  the workbench require registration with an ORCiD login.
- Fit: crosswalk enrichment — attach comparable external ids to our places and regions and
  surface them as "معرّفات خارجية"; also a discovery surface for Islamicate datasets
  (e.g. HatayMax). Best used read-only through the API.
- Caveats: ORCiD-gated downloads make bulk mirror impractical unattended; per-dataset
  licences differ, so one license sentence in `sources.metadata` per imported dataset is
  mandatory.

### Pleiades — ancient-world gazetteer, CC-BY 3.0 per record

- What it gives: Roman/Byzantine(-relevant) places with coordinates, temporal ranges,
  and per-record contributor metadata; DARMC point locations included per place.
- License: CC-BY 3.0, with each JSON record carrying its own `rights` evidence
  (`/places/<id>/json`). Its `/about` pages 404'd from this environment, so the per-record
  field is the verified ground truth.
- Access: per-place JSON documents (`/places/<id>/json`), exercised during research.
  Full-data dumps exist but were not verified from here.
- Fit: secondary — useful only for classical epigraphy-adjacent deductions on the
  Levant/Egypt/Hejaz Roman margins; not a primary Arabia layer.
- Caveats: era mismatch with the core corpus; import only where a user-visible claim
  about an ancient locality genuinely helps.

### OpenITI — machine-readable Islamicate texts (corpus)

- What it gives: premodern Arabic texts as versioned releases in mARkdown. The
  geographical and biographical literature this domain needs — معجم البلدان (Yāqūt),
  صفة جزيرة العرب (al-Hamdānī), المسالك والممالك (Ibn Ḥawqal, al-Iṣṭakhrī), the
  رجال/طبقات biographical dictionaries — are target titles, not confirmed holdings:
  per-title availability and license must be checked through the metadata service and
  release tags before any plan depends on them.
  Each release carries a Zenodo DOI (`doi:10.5281/zenodo.3082463`); a metadata service
  (`kitab-corpus-metadata.azurewebsites.net`) records per-text author/title/date/license.
- License: the corpus is downloadable, but licenses vary **per text** (collected group
  markers like `AR####AH` denote families); verify per title before ingest, and record it
  in `sources.metadata`.
- Access: GitHub releases/tags of `OpenITI/RELEASE` (verified live: data + metadata +
  release_notes layout), Zenodo for a citable snapshot.
- Fit: the biggest value source. Texts are *uploaded* — not copy-pasted — into the
  existing manual source pipeline (`sources` → `source_files` → pages/OCR → name review
  → `source_passages`), then the analysis layers propose
  `geographic_associations` (`status='platform_inferred'`) with `spatial_evidence`
  (`evidence_text_ar`, `review_status='needs_review'`). A reviewer accepting evidence
  turns associations `documented` — the provenance chamber shows passage + page.
- Caveats: mARkdown ≠ raw PDF; conversion to an ingest format must preserve the
  mARkdown source as the uploaded artifact. Never mass-publish extracted passages beyond
  what the per-text license permits — passages quoted inside review UIs must stay
  minimal.

### Shamela ecosystem (GitHub) — reading tooling for classical texts

- What it is: live ecosystem of *code*, not a data dump: `ragaeeb/shamela` (MIT,
  NodeJS library for Maktabah al-Shamela), `alhoqbani/shamela-mcp` (MIT, MCP server to
  query Shamela at review time), `OpenShamela/shamela_crawler` (GPL-3, Scrapy crawler).
  All verified via GitHub search API.
- License: the code licenses are fine to use. The **underlying book texts are not
  covered by those licenses** — per-title rights must be checked before any text lands
  in our pipeline.
- Fit: targeted retrieval of one public-domain classical title (معجم البلدان) as an
  author-tagged upload for the Stage-4 evidence pipeline. The MCP server is attractive as
  a *review-time* lookup tool (a reviewer pulls a citation from a classical source) but
  bulk-crawling is out — respect rate limits and robots.
- Caveats: any ingest must be a deliberate upload with a chosen title + license note,
  never an automated mirror.

## Examined, not verified (leads only)

- **Fihrist** (fihrist.org.uk) — union catalogue of Arabic manuscript holdings; useful
  later for manuscript-metadata enrichment. The site did not respond from this network,
  so its API/licence stance is unverified.
- **MASADER** (masader.org) — Arabic NLP dataset index; a plausible source of Arabic
  toponym NER datasets for the Stage-4 name pipeline. Fetch returned empty from this
  environment; treat as a lead, verify before citing in any plan.

## Mapping sources → dawha's model

| dawha surface | columns/tables concerned | external origin |
| --- | --- | --- |
| Reference layer (seas, mountains, landmark cities) | generated reference captions; later real `places` rows (`place_type` includes `city`,`other`) | Wikidata SPARQL (CC0), GeoNames cross-check |
| Name variants | `historical_place_names` (`name_type='alternative'|'historical'`, `source_id`, `valid_from/to`) | GeoNames alternateNames, Wikidata labels, classical texts |
| Gazetteer imports | `places` + `geographic_associations` (`status='platform_inferred'`) + `sources.source_type='website'|'historical_registry'` | Wikidata + GeoNames curated subsets |
| Classical-text evidence | `sources` (`source_type='book'|'manuscript'`) → `source_files` → OCR/pages → `source_passages` → `spatial_evidence` (`evidence_text_ar`) → `geographic_associations` | OpenITI (mARkdown), selective Shamela titles |
| Period vocabulary | JSON artifact + timeline filter; validates `valid_from/valid_to` | PeriodO |
| Crosswalk ids | `sources.metadata` jsonb (wikidata, geonames, pleiades, whg keys), shown as external-identifier blocks | WHG, Pleiades, id-bearing sources above |
| Region enrichment | `historical_regions` (`valid_from/valid_to`, captions, `source_id` refs) — **geometry stays ours** | PeriodO (coverage), WHG (identity), texts (evidence) |

Provenance invariants per row class:

- Every fetched artifact lands as a `source_files` row with `checksum_sha256`.
- Every imported row references a `sources` row whose `metadata` jsonb locks license,
  endpoint, and fetch date.
- `geographic_associations` from imports start `platform_inferred`; the flip to
  `documented` happens only via reviewer action backed by accepted `spatial_evidence`.
- Places that are reference-only stay at the least-visible visibility until promoted;
  external coordinates never widen anyone's reading scope (the existing
  `ReferencePredicate` place-scoping keeps holding).

## Staged roadmap

1. **Reference layer from Wikidata (≈1 day, high value).** A committed
   `infra/local/` script runs the verified SPARQL pattern (curated Q-id list + country
   filter), writes a vetted JSON artifact into the repo, and a generator fills the same
   data the hand-coded `map-reference.ts` consumes today. No schema change; provenance
   note in the script header records Q-ids + fetch date. Then Goldilocks promotion:
   reference entries that deserve it become real `places`.
2. **Name variants from GeoNames (1–2 days).** Country+feature-class filtered
   alternateNames for already-public places → `historical_place_names` rows, deduped
   against `normalized_name_ar`, each row carrying a `sources` row + dump checksum.
3. **Curated gazetteer import (3–5 days).** A working list (100–300 entries: cities,
   historical settlements, mountains, hydrography) of Wikidata×GeoNames candidates,
   human-reviewed list once, then seeded; associations start `platform_inferred`; review
   UI gets a batch lane.
4. **Classical-text evidence pipeline (1–2 weeks, biggest payoff).** معجم البلدان from
   OpenITI as an uploaded, checksummed source through the existing OCR/name pipeline;
   analysis layers propose place/region associations with passage-backed
   `spatial_evidence`; reviewers accept → `documented`. Extends the current
   platform-inference verb for spatial claims; no shortcut past review.
5. **Periods + crosswalk (3–4 days).** PeriodO JSON artifact drives a timeline filter
   and `valid_from/valid_to` sanity display on regions and name rows; external ids
   (Wikidata/GeoNames/Pleiades/WHG) surface as a small crosswalk panel on the place page,
   all read-only.

## Flagship features (leverage analysis)

The staged roadmap is plumbing — source → table. The leverage appears in what the
*combinations* enable. Each flagship below names the feature, the sources that fuel it,
the existing schema it moves through, and why it is extraordinary rather than a
commodity map-with-dots. The strategic thread: dawha's moat is provenance-first
knowledge with human review, so external sources exist to serve reviewed claims — not
to replace them.

### 1) طبقات — the atlas as a time machine

- Concept: a period slider on `/places` (أنطولوجيا: الخلافة الراشدة، الأمويون،
  العباسيون، الأيوبيون، المماليك، العثمانيون). Regions and counters respond to the
  selected epoch instead of showing one timeless total.
- Sources at work: **PeriodO** supplies the period vocabulary with start/end coverage;
  **WHG** crosswalk only as identity aid; no geometry from any source.
- Architecture path: `/api/v1/map?period=<id>` (or `from/to` window) feeds the same
  derivation the `placeCount` subquery uses today, adding
  `geographic_associations.time_from/time_to` predicates — and extended counters
  (`personCount`, `familyCount` from `entity_type`; that column is free text, so adding
  counted types is data, not schema). All counters stay server-derived through
  `ReferencePredicate`; viewport math never enters a component.
- Extraordinary because: genealogy apps show dots; this shows *how knowledge about a
  place thickens over time* — and it thickens visibly as the corpus of reviewed sources
  grows.
- Guardrails: e2e journey 08 gains a period-param assertion; unit tests extend the
  existing per-reader-count equality invariant (what a moderated region shows must equal
  what the place layer yields for the same reader + window).

### 2) المَتْن — every place becomes a dossier

- Concept: place page section «شهادة النصوص» — what classical literature says about
  this place, ordered by era, each row tying a `source_passages` card + accepted page
  crop to an accepted `spatial_evidence`.
- Sources at work: **OpenITI** texts (uploaded, checksummed, OCR'd) feed passage
  extraction; **GeoNames/Wikidata** pins appear side-by-side as candidate coordinates in
  the review lane; **Shamela MCP** optionally serves review-time citation lookups.
- Architecture path: the name-review crop UX already exists — reuse it for spatial
  evidence; `spatial_evidence` rows carry `evidence_text_ar` + `source_passage_id` +
  `review_status`, so the dossier readout is a query, not a pipeline.
- Pilot-before-pipeline: ship the readout on 5 hand-reviewed places (مكة، المدينة،
  الطائف، اليمامة/الهفوف، العلا) built from manually uploaded excerpts. The pipeline
  (Stage 4 of the roadmap) then amplifies, not creates.
- Extraordinary because: no Arabic-heritage product gives passage-to-page
  traceability for place claims at this granularity; this is the provenance chamber
  made readable.

### 3) رِحْل — biographical routes on the map

- Concept: `migration_events` is already schema-ready (`from_place_id`, `to_place_id`,
  time windows, status, claim, source). Person pages draw مسالك الأعيان over the atlas:
  "خرج من نجد إلى المدينة" becomes a drawn route with epochs.
- Sources at work: **OpenITI** biographical dictionaries (`طبقات`, `رجال`) feed
  association proposals (relation types `lived_in`, `originated_in`, `mentioned_at`
  already exist); **gazetteer candidates** suggest coordinates for a destination place
  the reviewer must confirm or discard.
- Pilot: seed ~10 human-reviewed `migration_events` from one well-documented biography
  whose movements are verified against the uploaded source.
- Extraordinary because: a name becomes a journey; families see scholars' actual
  movement, all review-gated.

### 4) مِفْتاح الأسماء — toponym search that speaks every register

- Concept: one search box that understands historical attested variants as first-class
  names: يَثْرِب → المدينة، حَجَر → الهفوف/الأحساء. Search matches classical names,
  colloquial variants, and modern names in both directions.
- Sources at work: **GeoNames** `alternateNames` as draft rows; **classical name
  attestations** from Stage-4 texts (`historical_place_names.name_type='historical'`);
  **Wikidata** labels for modern canonical names.
- Architecture path: `historical_place_names` + `person_aliases`-style normalization
  feeds the existing trigram retrieval indexes (`normalized_name_ar` GIN already stands
  up).
- Extraordinary because: Arabic toponym variants are the *actual* retrieval blocker in
  heritage search — uni-variant search is why users can't find things today.
- Guardrail: before any pipeline adds rows, add a names-variant retrieval benchmark
  guarded by `make verify` so recall gains are measured, not assumed.

### 5) المُقارِن — review as adjudication, not data entry

- Concept: no "import and pray" gazetteer lane. The system proposes *pairings* —
  classical token ↔ our place ↔ gazetteer pin — ranked by PostGIS
  nearest-neighbor/point-in-ring plus name-normalizer score. The reviewer chooses:
  link, alias, merge-candidate, or reject. Each confirm is provenance-stamped.
- Sources at work: **Wikidata + GeoNames** as candidate pools; our regions provide the
  rings that sanity-check pin placement (sanity flags only, never auto-reject).
- Architecture path: pairing-proposal SQL + a review-lane surface (the tree-import
  suggestion-review pattern already validates this UX shape in this codebase).
- Extraordinary because: it prevents the silent gazetteer bloat that kills most
  heritage maps, and it converts external data into *disciplined* rows.
- Synergy: the same matcher serves family-tree importers (place-of-event candidates
  from sources the user uploaded).

### 6) صحيفة الأثر — the license/trust dashboard

- Concept: an admin readout of every import: source, license text, per-text dependency
  verdict, fetch checksum date, imported-row counts by status. Flags anything missing
  license info before it can move past `platform_inferred`.
- Sources at work: OpenITI per-text licenses, WHG per-dataset licenses, GeoNames
  attribution requirements — the messy reality made visible.
- Extraordinary because: for a heritage platform, being visibly license-clean is a
  market promise; today that state is invisible.

### The compounding loop

These are not six parallel projects — each stage's byproduct feeds the next:

```
reference layer (F0) → atlas stops looking bare
  → pairings (5) → clean place + alias seeds
    → aliases (4) → search jumps → name-pipeline recall improves
      → classical texts review faster (dossier pilot grows) (2)
        → routes (3) + epoch counters (1) meaningful
          → timeline filter reads correctly
            → more contributors upload sources → loop
```

Two inhibitors deserve respect: **matcher precision** — machine proposals hand off to
human review by design long before they approach 100%, so the review lane turns the
precision ceiling into throughput rather than a bug report; and **license
heterogeneity** — the OpenITI per-text reality makes dashboard (6) a prerequisite for
any volume import, not an afterthought.

### Sequencing: pilot-first, pipeline-second

Human-curated demos prove each flagship ships value even before its pipeline exists:
- Dossier: 5 hand-reviewed places (doc-driven, zero new infrastructure).
- Routes: ~10 reviewed `migration_events` from one biographical case.
- طبقات: a curated PeriodO→era mapping of ~30 periods reviewed once.

Then the pipelines amplify pilots into volume — and measurement (journey assertions,
per-reader count equality tests, retrieval benchmarks) guards each promotion.

## Explicitly out of scope

- Bulk crawling of Shamela or any site against its etiquette.
- Importing modern administrative borders or boundary datasets as region geometry.
- Publishing extracted text beyond what a per-text license allows.
- Auto-publishing any imported row; everything lands review-gated.
- Replacing the committed NE 50m coastline reference with anything online.
