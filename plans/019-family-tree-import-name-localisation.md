# Plan 019: Localise the name carriers instead of the ink runs, and read each one in isolation

> **Executor instructions**: Every item here replaces a measured defect. The measurements that
> justify them are in `docs/import-reading-strategy.md` §1, taken from the one chart the
> repository has. A change to this path that is not measured against the same gold, on the same
> documents, is a regression, not an improvement.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: HIGH — this replaces the geometry source every reviewed tree draft depends on
- **Depends on**: plan 018 (the corpus and the measured baseline this is compared against)
- **Category**: feature
- **Planned at**: 2026-10-21
- **Current state, 2026-10-22**: the colour localiser, the ink name groups and the hosted
  reader are implemented; the ink pass now runs with or without coloured shapes, and raster
  images (JPEG/PNG/GIF, including the web-published GIF charts) are first-class one-page
  documents. Measured on the expanded corpus, the open work is **grouping and content**: on
  the Jasser renderings the name groups are letter fragments at 4,000 px, and on the dense
  modern charts the proposals include prose panels and tables. See
  `docs/import-reading-strategy.md` §1.8–1.9.

## Why this matters

This plan was originally "recover chart geometry from the PDF's vector content." Measurement
killed that framing, and the reason is worth stating before the steps.

**The one chart the repository has is not a box-and-line family tree.** It is a decorative
painted tree — branches, grass, sky, a waterfall — with the names written in black Arabic on
coloured fruit and leaf shapes. Measured (`docs/import-reading-strategy.md` §1):

| Measured on the Jasser chart | Value |
| --- | --- |
| Fonts / text layer | **0** — it is a pure scan |
| Vector paths | **0** — a vector extractor has nothing to read |
| Embedded image | 1708×2592 JPEG at **236 ppi** |
| Ink segments found by the current detector at 300 DPI | **9,345** |
| Segments kept | **1,000** — **89.3 % discarded** |
| Surviving candidates in the right three-quarters below the top quarter | **zero** |
| Median candidate length | 54 px — a letter stroke, not a connector |
| Name carriers found by the implemented localiser at 300 DPI | **398** on the drawn chart, **388** on the photographic one — 2.6 % apart, and the two charts carry the same facts |
| Tesseract on a tight, legible 600 DPI crop of one name | `2`, `سد`, `!` — it cannot read the font |

So the current pipeline does three wrong things at once: it thresholds a photograph, it
truncates the result by page position, and it asks a local OCR engine to read a decorative
display face that is outside its training distribution.

**Two more defects fall out of the same measurement.** Raising the render DPI makes the
truncation *worse* — `minHorizontal = max(28, width/100)` scales with resolution while
`MaxLineCandidates` is a constant — so the product's "re-OCR at 400 DPI" remedy
(`apps/web/src/components/SourceProcessingPanel.tsx`) makes the geometry loss strictly worse:
85.9 % discarded at 240 DPI, 89.3 % at 300, 91.7 % at 400. And Tesseract on the whole page
returns 342 garbage "words", because a greyscale ink threshold sees the landscape.

**The right architecture**, which the measurement supports directly:

```
render at the reading resolution (300 DPI is enough)
   ↓
localise the name carriers      colour segmentation (pictorial) | vector geometry
                                 (box-and-line, digital) | raster detector (fallback)
   ↓
crop each carrier, mask to it
   ↓
transcribe each crop             vision model, one isolated word per call
   ↓
multi-view consensus per crop    measured confidence, not a model's opinion
```

Compare the hosted-vision run this replaces: it asked the model to **find** names across a
dense photographic sheet and got 39 against 500-plus (`docs/ocr-and-import-decision.md:50-57`).
The diagram-extraction literature says exactly why that must fail — classical CV reaches
98.8 % node F1 and 66.7 % edge F1 against 34.0 % and 10.7 % for the best vision-language
model (FlowExtract, APMS 2026; diagram2graph). The fix is not a better model; it is changing
the question from *find* to *transcribe*.

## What is implemented, and what it measured

The reading path is implemented and it works end to end. The chain, on the one chart the
repository has:

| Stage | Result |
| --- | --- |
| Render at 300 DPI | the resolution the pipeline already uses |
| Saturated components | 411 carriers — the coloured shapes |
| Ink grouped by proximity, and filtered to what is name-sized | **493** name groups on the photographic chart, **474** on the drawn one — 4 % apart |
| Read each group in isolation, three times, and vote | the names |

**474 and 493 are in the right place.** The decision document says the chart carries "500-plus
visible names" (`docs/ocr-and-import-decision.md:55`), and the localiser finds 474 of them on
the clean drawing and 493 on the photograph. That is the first independent check of that
figure, made without an answer key, and it is pinned by a test.

Ten of the larger groups, read by the existing hosted reader:

`لبيب`, `الطفل`, `نور الحق`, `فيصل`, `حاكم`, `طارق`, `عبدالله`, `إبراهيم` — and one that was
checked against the crop by eye and was exactly right. The ten smallest groups were unreadable,
which is a *recall* question about the smallest names on the chart rather than a defect in the
path.

Three corrections the measurement forced, each of which was a design decision that looked
right and was not:

- **The unit is the NAME, not the shape.** The leaves of a painted tree touch, so a carrier is
  often a dozen of them, and reading a carrier hands the reader a branch of the tree. The
  name is a run of ink; grouping the ink by proximity gives the right unit.
- **The context crop must not be wider than the name.** A wider context crop contains the
  NEXT name as well, so the reader returned two readings for one occurrence and the response
  schema refused them. The tight crop is read accurately, and the robustness comes from
  reading it several times and voting.
- **A text block is not a name.** The ancestor stack down the trunk and the panels of prose
  come out as one enormous group, and an area cap excludes them.
- **A fragment of a letter is not a name either.** The first grouping produced 1,900 groups
  on the photographic chart, most of them single strokes of JPEG-shattered letters, and the
  reader returned nothing for any of them. A minimum height and a minimum area took it to 493,
  and the readings that came back were names.

The arm is `cmd/ocr-eval`, which writes a prediction file the harness scores like any other.

## Current state

- **The line detector is a run-length ink detector.**
  `internal/sourceprocessing/line_detector.go:22` thresholds grey < 150, collects horizontal
  and vertical runs longer than `max(28, dimension/100)`, merges them with a 4-pixel
  tolerance, and truncates to `MaxLineCandidates = 1000` (`:12`, `:52`). Its own comment at
  `:19-21` says it "deliberately does not join intersections into parent/child edges" because
  "scan strokes, box borders and real connectors can look alike without chart context." That
  caution is right and must survive.
- **Its output is already part of the evidence model.** The Tesseract reading's regions carry
  the line candidates (`pdf_extractor.go:433`), `source_page_evidence` stores the readings, and
  `tree_drafts.go:231-237` reads `page.LineCandidates` into the review surface, with
  `LineCandidatesTruncated` surfaced at `:237` and checked at `:1096`.
- **The render path is Poppler only.** `pdf_extractor.go:153-155` wires `pdfinfo`, `pdftotext`,
  `pdftoppm`; `:415` rasterises at a chosen DPI. Chart analysis renders at
  `chartAnalysisDPI` (`chart_analysis.go:30-37`), the document path at 300 DPI
  (`upload.go:66`).
- **The document class is already knowable but unrecorded.** `PageReading.Method` distinguishes
  `poppler-pdftotext` from `tesseract_ocr` (`types.go:263-272`), so digital versus scanned is
  computable from stored evidence. **Nothing records the chart style**, which is why the
  reading pipeline is aimed at the wrong document class.
- **The harness already measures the right things.** `evaluate.py` computes branch
  precision/recall by polygon IoU and connector precision/recall by discrete Fréchet distance,
  and the labeler traces real connector paths. Name occurrences are scored on
  `reference_id` + region IoU ≥ 0.5, and equal Arabic strings never establish identity.
- **No PDF vector content is read anywhere.** `pdfplumber`, `GetDrawings`, `pdftocairo`,
  `page.lines`, `page.rects` and `svg` have no matches across `internal/` and `cmd/`.
- **The reading render is source-resolution tiles (2026-10-22).** `ReadNamesOnPage`
  (`name_reading.go`) opens the page at 240 DPI, splits it into 1,920-px tiles with a 256-px
  overlap, localises each tile with the page-calibrated thresholds expressed in tile pixels,
  and assigns each name to one tile by its centre. The آل الصباح wall chart is 54 tiles, 3,014
  proposed names with a median height of 36 px, against ~18×7 px at the old 4,000-px preview.
  `ReadSuggestions` uses this path and records
  `colour-text-anchor-v1+ink-name-groups-v1+source-tiles-v1`; the image opener now accepts the
  upload bound of 40 megapixels instead of refusing above 16, and the read result warns when
  the median name height is below 18 px (the three chat images measure 3, 11 and 5 px). This closes the
  *reading resolution* half of the plan, not the *reader* half: the hosted reader misreads the
  calligraphic chart even at 1,200 DPI, which is the STOP condition above and is recorded in
  `docs/import-reading-strategy.md` §1.10. Painted and leaf charts still fragment, and a larger
  grouping gap merges branches, so the grouping measurement remains open.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Harness tests | `python3 -m unittest discover -s tools/ocr-eval -p 'test_*.py'` | exit 0 |
| Labeler tests | `node --test tools/ocr-eval/test_labeler.mjs` | exit 0 |
| Baselines | `python3 tools/ocr-eval/run_baselines.py --manifest <private>/manifest.json --output-dir <private>/predictions` | writes the arm files |
| Score | `python3 tools/ocr-eval/evaluate.py --gold <private>/gold.jsonl --predictions <label>=<private>/predictions/<label>.jsonl ... --output <private>/comparison.json` | writes the report |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |

## Scope

**In scope**:

- `services/core-api/internal/sourceprocessing/` — a document-class record, a name-carrier
  localiser, the truncation fix, and the evidence-model changes they require
- `services/core-api/internal/sourceprocessing/line_detector.go` — kept as the fallback
- `services/core-api/internal/sourceprocessing/pdf_extractor.go` — render DPI and wiring
- `db/migrations/` — additive only, for the document class
- `tools/ocr-eval/` — new arms and whatever they need
- `apps/web/` — only if the review surface must show which arm produced a candidate
- `docs/` — the measured comparison and the corrections to the record
- `Makefile` — the new arms' commands

**Out of scope**:

- **Selecting a hosted provider.** `docs/ocr-and-import-decision.md` §11 records that hosted
  processing is permitted for these documents, so the vision-transcription arm **is** enabled
  with a provider in this plan. What stays out of scope is *choosing which* provider: that is
  plan 018's measurement, and this plan's arm reads whichever provider the configuration
  names, with the provider, model version and region recorded in the arm's output.
- **Automatic relationship inference.** Localising a name is not the same as deciding that a
  branch means *father-of*. The decision document's rule stands
  (`docs/ocr-and-import-decision.md:251-254`): geometry may propose, never establish. For a
  pictorial chart the structure is painted foliage and the open question is stated in
  `docs/import-reading-strategy.md` §3.
- **Qualifying any class.** Plan 018 measures; this plan is measured against it. The evaluator
  still returns `pending_independent_review`.
- **Cross-page connector stitching.** Open in the decision document, open here.
- **Changing the upload contract.** `application/pdf` is already accepted (`upload.go:151`).
- **The lineage-book path.**

## Steps

### Step 1: Record the document class, and make it a first-class fact

- **Add a chart-style classification** to the file record: `pictorial` (names on painted or
  coloured shapes), `boxed` (names in rectangles or boxes with drawn connectors), or
  `prose`. This is a *file* attribute, not a per-page one, because it is a property of how the
  document was made.
- **It must be derived, not declared.** A user choosing "this is a family chart" is a hint, not
  a classification. The classifier is deterministic and its own output is recorded, alongside
  the evidence it used, so a reviewer can see why.
- **A deterministic classifier, with a reviewed override.** Signals available without a model:
  the ratio of saturated connected components to total ink components; whether the page
  contains orthogonal path structures at all; the fraction of the page covered by photographic
  gradient. The classification is a heuristic, it is recorded with its inputs, and the
  reviewer can override it. **A wrong classification must degrade to "both arms run", never to
  "the wrong arm only".**
- **The stored reading pipeline must record which arm produced each candidate**, so that a
  reviewer can weight it and a later run is comparable.

### Step 2: Build the name-carrier localiser

A new component that finds *where the names are*, replacing the ink-run detector as the
primary path.

- **Pictorial charts: colour segmentation.** Connected components over pixels whose channel
  spread exceeds a threshold, with an area filter, scored by the presence of an interior
  high-contrast mark. Measured on the two charts we have, at the 300 DPI the pipeline
  already renders: **398** carriers on the drawn chart and **388** on the photographic one.
  The two charts carry the same facts, so the agreement is the calibration signal, and the
  thresholds are chosen to hold it.

  **The merge is anchored on the text and bounded by proximity.** Fragments of one leaf share
  a word; two different leaves never do. Without the proximity bound, one long stroke crossing
  a dense canopy merges every leaf it touches, and the merged group is too large to be a name
  and is discarded — destroying the carriers the merge was meant to repair.

  **The association is by pixels, not by box.** A blob whose pixels lie inside a carrier is
  text on that carrier. The alternative — "the word lies within the shape's box" — is more
  robust in principle and measurably worse: on the photographic rendering it cut the carrier
  count from 386 to 172, because one long stroke overlaps many leaves at once.

  **No second render is required.** Measured as a fraction of page area, the carrier
  distribution is the same at 300 and 600 DPI, and 600 DPI finds about 7 % more components
  rather than three times as many. The 16 MP render cap is therefore not in the way, and the
  300 DPI the pipeline already produces is enough.
- **Box-and-line charts: native vector geometry where available.** For a digital PDF, the
  connectors and boxes are vector path operators in the file and should be read as such, not
  estimated from a rasterisation. Two source options — Poppler's `pdftocairo -svg` alongside
  the three binaries already wired, or a Go PDF content-stream parser — and the choice is made
  by measurement, not preference. The Go route avoids a new external dependency; the Poppler
  route is less code and inherits Poppler's interpretation.
- **Scans of box-and-line charts: the existing raster detector, but fixed.** It stays as the
  fallback, and it must not be the primary path for a scan with colour.
- **The coordinate space is canonical and explicit.** Normalised top-left `[0,1]` page
  coordinates, per `docs/ocr-and-import-decision.md:346-349`, with the transform from the
  PDF's own space recorded, not assumed. The localiser's output must land in the same space as
  every other path.
- **Every candidate keeps enough evidence to be reviewable**: the carrier's bounding box, the
  crop that was read, and the method that found it.

### Step 3: Fix the truncation, and stop the DPI from making it worse

- **Truncation must never be positional.** The current sort by `fixed`
  (`line_detector.go:46-51`) keeps the smallest row-or-column index, which measured as **zero
  surviving candidates in the right three-quarters of the page below the top quarter**. If
  candidates must be dropped, the rule must be defensible — by confidence, by area, or by
  completeness of the carrier — and the report must say *where* the loss is.
- **The minimum run length must not scale with resolution** while the cap does. The discarded
  fraction measured 85.9 % → 89.3 % → 91.7 % at 240 → 300 → 400 DPI, which means the
  product's 400 DPI remedy makes geometry strictly worse while improving reading. The two
  concerns need separate parameters.
- **Exceeding a limit is a visible incomplete state at the point of truncation**, not a
  downstream flag. The cap value must come from the corpus distribution plan 018 measures.
- **A localiser that finds 400 carriers must be able to keep all 400.** The cap exists to
  bound a *different* thing — raw ink runs — and should not bound a much smaller, much more
  meaningful set.

### Step 4: Read each carrier in isolation, with a pluggable reader

- **The reader interface takes one cropped carrier and returns text plus a confidence**, with
  the crop, the method, the model version and the raw provider score recorded. It is an
  interface so that the local reader and a hosted reader are interchangeable — and so that the
  privacy decision of §4.2 in `docs/import-reading-strategy.md` stays a configuration choice
  rather than an architectural one.
- **The local reader must be tried first, and must be reported honestly.** Tesseract's `ara`
  model returned `2`, `سد` and `!` on a tight, legible 600 DPI crop of one name, because the
  decorative display face fuses the letters. The plan must record what the local reader
  achieves on this class rather than assuming it works.
- **The preprocessing that makes a local read plausible must be implemented and measured**:
  crop to the carrier, mask to the carrier's own saturated region, and binarise against a
  *local* threshold (the carrier's median luminance minus a delta), because a global threshold
  turns the coloured surface itself into ink. The measurement in
  `docs/import-reading-strategy.md` §1.5 shows this recovering a clean isolated word where the
  global threshold did not.
- **Multi-view consensus is the confidence mechanism.** Generate *k* augmented variants of the
  crop (padding, blur, grid-warp), read each, align with a Needleman–Wunsch-style aligner,
  and take a per-column vote; the confidence is the vote fraction. Measured on historical
  records: +4 pp accuracy and a 0.642 confidence–accuracy correlation. **The model is never
  asked to rate itself** — verbalized confidence is AUROC 0.54–0.74, at times worse than
  chance.
- **The reader is routed, not blanket-applied.** OCR-conditioned correction helps only where
  the OCR prior is recoverable and hurts where it is script-mismatched, so the second reader is
  enabled per region.

### Step 5: Add the arms to the harness and measure

- **`run_baselines.py` gains arms**, each producing the same prediction shape as the existing
  three: `nodes[]` with `id`, `page_id`, `name_ar`, `status` and normalised `region`, plus the
  `processing_seconds`, `ocr_operation_count` and `input_image_bytes` diagnostics. The arms:
  - `colour_carrier` — the new localiser plus the local reader
  - `colour_carrier_vision` — the same localiser plus a vision reader behind the pluggable
    interface. **A provider is enabled**, per the privacy decision at
    `docs/ocr-and-import-decision.md` §11, and the arm records the provider, model version and
    region in its output. The arm still reports "not configured" when no provider is
    configured, rather than failing.
  - `native_vector` — vector geometry, for the box-and-line digital class
  - the existing `full_page_tesseract`, `tiled_tesseract` and `reviewed_branch_tesseract` stay
    as the incumbent arms
- **The evaluator scores them with no change.** Each is a `--predictions label=path` argument.
  If the evaluator needs a change to score an arm, that is a finding about the evaluator and
  must be recorded as one.
- **The alignment pass still applies.** Carries `reference_id` only through the same blinded
  alignment the other arms use; equal name strings never establish identity.
- **Run all arms over the same corpus and the same gold, in one report, with one seed**, and
  report per class. The headline comparison is **name detection recall and exact-name
  precision per document class**, because the classes have different failure modes and a
  pooled number hides exactly the distinction that decides the architecture.
- **If the new arms are not better, say so and do not ship them as the default.** The raster
  detector stays. A negative result is a real finding.

### Step 6: Correct the record, honestly and specifically

- `docs/ocr-and-import-decision.md` §2, §5, §8 and §12; `docs/import-reading-strategy.md`
  §1 and §8; `docs/phase-status.md` Phase 14. Where the measurement contradicts the
  projection — including this plan's own earlier framing — the measurement wins.
- **Record the document class as a limitation of the pilot.** One chart has been measured. The
  corpus may contain box-and-line charts, prose charts, or a mixture, and the reading strategy
  must be chosen per document class. That is the finding, not a defect.

## Test plan

- **The localiser is unit-tested on a synthetic PDF** generated by the test — a coloured shape
  with a dark mark inside, on a textured background — asserting the carrier is found and its
  normalised region is correct. This is also the **committed PDF fixture** the repository
  currently lacks (`docs/import-reading-strategy.md` §9).
- **The classifier degrades safely.** A misclassified document runs both arms rather than the
  wrong one, and the record says which ran.
- **Truncation is never positional.** A test asserts that a dropped candidate is not
  systematically the bottom-right of the page, and that the loss is localised in the report.
- **DPI does not worsen truncation.** A test at 240, 300 and 400 DPI asserts the discarded
  fraction does not increase with resolution, or that each DPI uses its own measured limit.
- **The reader interface is provider-agnostic.** A test reader and a stub both satisfy it, and
  the vision arm reports "not configured" when no provider is set, rather than failing.
- **Confidence is derived from agreement, never from the model.** A test asserts the recorded
  confidence equals the computed vote fraction, and that a model self-reported score never
  reaches the field.
- **The masked binarisation recovers an isolated word** where a global threshold does not, on
  the synthetic fixture.
- **The harness scores the new arms with the unmodified evaluator**, and a deliberately broken
  arm is visibly worse.
- **Every existing tree-draft, page-evidence and reprocessing acceptance test still passes.**

## Done criteria

- [ ] Each file records its chart style, derived deterministically, with the evidence used, and
      a wrong classification degrades to running both arms.
- [ ] A colour-segmentation localiser finds the name carriers on the synthetic fixture and on
      the real chart, at the reading resolution, with a measured yield.
- [ ] Native vector geometry is read for box-and-line digital charts, with a pinned coordinate
      transform, and is a fallback rather than the only path.
- [ ] Truncation is non-positional, its limit is measured, and raising the render DPI no
      longer worsens it.
- [ ] Each candidate records the arm that produced it.
- [ ] The reader interface is provider-agnostic, the local reader is measured honestly on this
      class, and multi-view consensus produces the confidence.
- [ ] The new arms are scored by the unmodified evaluator, per document class, in one report.
- [ ] The measured comparison is written down, including the case where the new arms are not
      better.
- [ ] `docs/ocr-and-import-decision.md`, `docs/import-reading-strategy.md` and
      `docs/phase-status.md` carry the measured numbers and the document-class finding.
- [ ] `make verify`, `make verify-full` and `make e2e` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if plan 018's measurement shows the corpus is predominantly **boxed** rather than
  **pictorial**. The localiser is then wrong, and the vector path becomes the main event.
- Stop if the colour localiser cannot be made to beat the ink-run detector on carrier
  *recall* for the pictorial class. The raster path stays; do not ship a worse default with a
  better story.
- Stop if no reader can read the class's font — local or otherwise — and the honest answer
  becomes "this class requires manual transcription or a better original". That is a
  legitimate product outcome and must be recorded as one, not engineered around.
- Stop if the coordinate transform cannot be pinned by a test. A geometry path whose
  coordinates cannot be verified is not usable in a review surface.
- Stop if the change would require an always-on new service, a hosted provider, or a
  credential.
- Stop if a verification gate fails twice.

## Maintenance notes

- The render DPI is now a per-purpose parameter: reading wants high DPI, and geometry wants
  the resolution at which the structure separates. Conflating them is what made the 400 DPI
  remedy counterproductive.
- The document class is a property of the file, but a single file can contain both a chart and
  prose. The class must be re-derivable per page, and the page-level record must say which
  class was used.
- The colour localiser's thresholds are measured against the corpus, like every other
  parameter here, and are not to be tuned until a class stops appearing.
- The vector path and the raster path must both be kept. A scan has no vector content, and
  deleting the raster path would make scanned box-and-line charts unreadable rather than
  better.
