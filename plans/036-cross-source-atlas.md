# 036 — Search and compare Arabic tribal names across sources

**Status** IN PROGRESS · **Priority** P1 · **Pilot** al-Ahsa · **Depends on** plans 030–032 and 034

The local pilot runs through the atlas and public tribe dossiers. It includes 33 searchable citations, including the six names on figure 6, the two Hamza passages, and the Arabic map circa 600 CE. The 67 existing captions use the same search API with their current IDs and links. Moderators can confirm a public tribe, branch, or place, or leave the identity unresolved. Each decision records the reason.

Run `infra/local/import-atlas-source-catalogue.sh --dry-run` against a migrated local database to inspect a replay. Run it without the flag to apply the catalogue. The same inputs are part of `make db-seed`.

The 1929 sketch entries carry Qatar Digital Library's catalogue spellings. Its image endpoint denied the scan request. Direct transcription and alignment of that sheet require an accessible scan. The teaching, Asir, Oman, social, broadcast and 1935 entries open their individual locators. The fifth volume of *البدو* has a catalogue entry while its sheets are being located.

### Implemented in the review pass

- Extract proposals with `python3 infra/local/stage-atlas-source.py COPY --item ITEM_KEY`. Use `--page N` for PDFs and `--outline-render PNG` for a local raster rendered from an outlined SVG. Open the cited item in the atlas and upload the JSON under **استيراد قراءات للمراجعة**. Review candidates under **مراجعة قراءات المصادر**. A changed digest has its own review queue. A repeat import deduplicates identical readings in the same coordinate frame. Publishing retains the raw proposal, original selector, reviewer, and correction.
- Plate crops use the pinned scan digest and check the stored bytes. If the active scan changes, searches can still return the old reported reading and checksum. Only the current matching raster receives a map action.
- The workbench records two matching boundary decisions including the locator, optional occurrence ID, and validity dates. The promoted feature opens the cited occurrence from its inspector when available. An occurrence remains separate from a historical claim unless the evidence supports the claim.
- Moderators can attach a published occurrence to an existing claim as contextual evidence. The transaction creates a source statement and links the existing passage when available. Repeating the attachment preserves one receipt. The claim's assertion, dates, and status stay with its own review.
- Fit a new sheet with `python3 infra/local/fit-atlas-affine.py SCAN CONTROLS.json`. The controls JSON has a pixel `neatline`, named `fit` and `check` arrays with `x`, `y`, `lon`, and `lat`, an `intendedScale`, and a `maxErrorKm` budget. The result records the exact scan digest, coverage and held-out residuals. Dawha reads conic and qualified affine transforms. A regional trial on the circa-600 Arabic SVG gave about 94 km RMS and 116 km maximum error over three held-out city marks, above the chosen 40 km trial budget. Those checks used manually measured pixel centers and approximate city coordinates. The sheet is available as a cited source. [Wikimedia map record](https://commons.wikimedia.org/wiki/File:Map_of_Arabia_600_AD-ar.svg) · [UNESCO reference for al-Faw](https://whc.unesco.org/en/list/1712)

The Oman appendix now has eight searchable legend entries. Each gives the map symbol and legend row separately. The catalogue also records the Rihani travel passage and the two separately licensed Musil holdings. Direct comparisons show same-source readings, reviewed source links, and cases where independence remains unknown.

The full plan remains open for a directly checked 1929 transcription, validated new raster overlays, primary-sheet sourcing for catalogue-only leads, and claim-specific assessment of source independence.

**First release.** A reader types مطير with every map layer switched off, sees the Arabic and English source spellings, opens the exact figure or folio for each, compares two readings, and follows a confirmed link to the tribe dossier. A reader searching بريه can open Hamza's printed page and the BARRIYAH caption. Each source keeps its own date, spelling, and attribution. [Normalization design](../docs/atlas-source-normalization.md) · [Source register with page links](../docs/tribal-territory-sources.md)

## Release 1 — The al-Ahsa pilot

| Work package | Deliverable | Depends on |
|---|---|---|
| A. Catalogue and compatibility | Versioned source items, stable occurrences, date and rights fields, existing caption backfill | Existing migrations through 0077 |
| B. Reproducible pilot import | A checked source manifest, a dry run, and a repeatable import of figure 6, the 1929 sketch and two book pages | Pilot transcription can begin immediately. Database import follows A. |
| C. Search, identity review and access | Paginated source search, occurrence detail and reviewed dossier links | A. Use the pilot fixtures from B in integration tests. |
| D. Reader journey and release | Search from the atlas, compare citations, open source media where permitted, and navigate from a dossier back to the evidence | B and C |

### A. Give each occurrence a durable address

- Add an item under the existing bibliographic source for each figure, folio, sheet or timed segment. Give each item a stable source-scoped key. Keep its public citation and locator, creator/credit, depicted interval and precision, original date wording/calendar, publication date on the edition, and acquisition time separately. Split physical page index from printed page number. Preserve unknown values.
- Give item revisions their own IDs. For local imagery pin the exact file ID, digest and coordinate frame. For external-only material pin the item URL and locator, plus a provider revision if available. Keep the rights statement, credit and permitted uses with the specific copy. A public citation may be readable while its pixels are externally hosted.
- Give each name occurrence a UUID and stable import key, unique within its item. Keep per-revision selectors such as a page, text span, pixel rectangle, legend entry or timestamp. Require the parent item revision and matching frame for a pixel selector. Model the original form, script/language, reading method, proposed reading and published reading separately. Preserve the evidence for corrections.
- Bridge existing map captions to occurrences using their current UUIDs. Reuse published caption revisions and the existing moderation path for those rows. For prose, reference existing passages/statements. Store identity proposals separately from readings and require a valid target for a confirmed identity. Let an unresolved reading remain searchable once published.
- Backfill the existing register and all 67 Admiralty captions by exact scan checksum. Resolve old plate-key/pixel links to their historical occurrence; keep a lookup when a moderator moves a label. Rehearse both a populated upgrade and a clean install where the legacy seed runs after migrations. Fix the register's use of acquisition time for the displayed historical date. [Caption schema](../db/migrations/0075_reviewed_plate_evidence.sql) · [Legacy seed](../db/seeds/002_admiralty_plate_captions.sql) · [Register](../services/core-api/internal/geography/plates.go)

**Acceptance.** Existing deep links resolve to the same 67 readings. Draft revisions stay private. Two figures from the same PDF can be cited independently. A missing or mismatched raster yields an unavailable-image response while its authorized citation remains usable.

### B. Import the sources and establish the reference set

| Input | Data to import | Verification |
|---|---|---|
| Al-Qahtani, figure 6, printed/PDF p. 108 | Six Arabic occurrences: مطير، العوازم، العجمان، بنو هاجر، بنو خالد، آل مرة. Record 2010 as publication and 1871–1913 as study coverage. Serve the citation with an external page link. | Verify all six literal forms against the figure. Use the study interval as contextual coverage. The figure yields zero individual tribal boundaries. |
| Dickson, 3 August 1929, f. 228 | A distinct QDL item with its folio, rights and image credit. Transcribe the English forms corresponding to مطير، العوازم، العجمان from the sheet. | Check each transcription against the drawing. Record the red Kuwait boundary's meaning from the legend or archival description. Keep the other Dickson overview under its existing identity. |
| Hamza, 1933, printed pp. 192–193 | Two locators, viewer images 218 and 219. Retain بريه verbatim in a passage under مطير and link any reviewed classification to its passage. | Searching بريه or برية can open the exact page. بريدة remains a distinct city result. |

Use a checked manifest with source, item, revision and occurrence keys. Stage and validate before a transactional upsert per item. A dry run reports additions, unchanged records, changed evidence, unresolved identities and media restrictions. Replaying an unchanged manifest or retrying after interruption must preserve IDs and human decisions. A changed image creates a revision and flags its anchors for rechecking. Capture the printed form directly when the original uses Arabic. Keep OCR and transliteration as attributed proposals for other inputs.

**Acceptance.** Run the import twice and compare keys, counts, locators and review history. Change one fixture digest and verify the old permalink reproduces its cited revision and the new scan cannot silently inherit its coordinates. The two existing public dossier links for مطير and بنو خالد can guide confirmation on the new sheet. Keep unmatched groups accessible by name.

### C. Search the whole catalogue and review identity

- Add a bounded, cursor-paginated occurrence search and a detail read keyed by UUID, for example `GET /api/v1/atlas/occurrences` and `GET /api/v1/atlas/occurrences/{id}`. Support query, subject, date window, and explicit inclusion of undated evidence. Search active and inactive plates, external figures and passages. Apply source visibility and publication state before ranking, counting or paginating. Use a stable tie-breaker for equal scores.
- Return the literal form, reviewed display form, reading method, citation/locator, depicted date and precision, publication year, review state, permitted source actions, confirmed public target if present, and a map action only when the geometry is usable. Read published corrections from the same authority used by caption moderation. Keep the server authoritative for search.
- Derive versioned search keys without changing the original text. Share Go/TypeScript fixtures covering ة/ه, alif variants, punctuation, digits and Latin apostrophes. Rank exact source forms, then reviewed variants, then fuzzy suggestions. Surface the reason for an identity proposal, alternatives and an abstain action. Distinguish names of tribes, subgroups and places when confirming. Use public target visibility in search and dossier responses.
- Enforce the same item access rules on detail, crop, tile, thumbnail, embed and dossier reads, including direct requests. Replace raster fallback with exact revision/file binding. Test private-source suppression and public citation-only access independently. [Router](../services/core-api/internal/httpapi/router.go) · [Raster selection](../services/core-api/internal/regionreview/tiles.go) · [Go name normalizer](../services/core-api/internal/identity/normalization.go)

**Acceptance.** Test an item beyond the register's 20-plate limit and an occurrence after caption 200. Both must be searchable. Check two result pages for omissions or duplicates. An unpublished correction remains absent until publication. Unknown depicted dates cannot enter a dated query through publication or acquisition time.

### D. Ship the reader journey

- Add **البحث في جميع المصادر** to the atlas and link each confirmed public dossier to its source occurrences. Show the Arabic original once. For an English label, show the source spelling alongside the reviewed Arabic. Selecting a result opens a stable URL and its exact citation; link-only items open the provider at the cited page or timestamp.
- Add **قارن المصدرين** to compare two occurrences by ID. Show each one's spelling, period, interpretation and source locator; render media only where permitted. Coordinate-based comparison remains available for two registered sheets. A source with no transform still participates in a citation comparison.
- Exercise the journey on desktop and mobile, by keyboard, with direct-link reload and back navigation. Check the atlas, dossier and existing reviewer flows against the populated pilot. [Atlas route](../apps/web/src/routes/places.tsx) · [Caption panel](../apps/web/src/components/PlateCaptionPanel.tsx) · [Comparison panel](../apps/web/src/components/PlateComparePanel.tsx) · [Dossier reader](../services/core-api/internal/geography/tribe_evidence.go)

**Release decision.** Ship when all six names reach figure 6, all three checked Dickson matches can be compared, both Hamza locators open correctly, and the privacy, duplicate-import and legacy-link tests pass. Report misses, incorrect identities and broken locators individually. Six labels establish pilot behavior only.

## Release 2 — Extend the corpus and geographic use

| Order | Implementation | Evidence of completion |
|---|---|---|
| E. Broader intake | Drive extraction from item metadata. Add PDF text and raster proposals, SVG text or outlines, linked legend symbols, and timestamped transcripts. Retain raw output, engine version and reviewer edits. Deduplicate overlapping tile detections within the same image revision. | Exercise the Saudi teaching map as undated, the Oman numbered legend as two linked selectors, the two Ottoman crops as one reported parent with unresolved provenance, and the Asir broadcast at its timestamp. Track the fifth volume of *البدو* as a catalogue lead until a sheet is available. |
| F. Registration and period layers | Evaluate a transform per sheet using geographically distributed held-out controls. Record model, coverage and residuals in kilometres. Set a justified display scale per sheet. Extend model dispatch when a candidate cannot use the current conic. | Publish an overlay only where the error supports the intended scale. Schematic Asir material remains readable in source space. Attribute and date the circa-600 CE SVG independently. Use Rihani and Musil for identified localities and route context. |
| G. Reviewed geography and research answers | Link occurrences to claims through their statements or passages. Carry depicted bounds into reviewed place associations and region promotion. Distinguish name, affiliation, residence, seasonal use and route evidence. Attach claim-specific dependency evidence using the existing dependency review. | The 1935 آل مرة sketch has its own citation and approximate dīra interpretation. A travel line yields no tribal movement without an identified actor. Two copies of one sheet count as one witness; unknown independence is visible. A historical polygon requires matching boundary reviews from two moderators and retains its locator and valid period. |

## Rollout and verification

1. Apply additive migrations on a scratch database. Check backfill counts and foreign keys, then rerun the manifest import. Keep old atlas reads available while comparing records and links through both readers.
2. Run database-backed tests for A–C, then the new D browser journey alongside the existing map, folio and workbench journeys. Use the repository's `make verify`, `make verify-full` against the scratch database, and `make e2e` for release verification.
3. Switch the atlas and dossier entry points after the pilot checks pass. If the catalogue reader regresses, restore the previous entry points and retain imported revisions and decisions. Retry a failed import from its manifest. Correct a published reading by adding a revision.
