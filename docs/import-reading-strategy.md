# The import reading strategy: what the documents actually are, and what follows

**Status: measurement-driven recommendation. It proposes three plans; it does not implement
them.** Written 2026-10-21 as an addendum to `docs/ocr-and-import-decision.md`, which remains
the authoritative statement of what a reliable import needs and of the pilot as built.

That document is thorough and, in the parts that matter, correct: it separates reading from
connection from interpretation, refuses to let machine output become fact, requires independent
annotation before any accuracy claim, and records its own hosted-vision runs as failures.

This document adds four things it does not have:

1. **A measurement of the one chart we actually have**, which changes the architecture.
2. **The published evidence** for why the hosted-vision route produced the numbers it did.
3. **An audit of what the import actually writes**, which finds the pilot closed most of the
   materialization gaps and left two that matter.
4. **The Arabic reading and confidence evidence** that the provider shortlist in §3 of the
   decision document deliberately leaves unqualified.

---

## 1. The measurement, and why it changes the plan

The pilot has one document: the Jasser chart, authorised by SHA-256 for the hosted-vision
experiment. It sits on disk, private, and it is A4. Everything below is measured on it, with
no annotation and no model calls.

| Property | Measured | How |
| --- | --- | --- |
| Pages / size | 1, A4 (595×842 pt) | `pdfinfo` |
| SHA-256 | `5735335e…07fd` | matches `JasserChartSHA256`, `chart_analysis.go:29` |
| Fonts | **0** | `pdffonts` — no text layer at all |
| Vector paths | **0** | PDF object scan — nothing for a vector extractor to read |
| Images | 2 | one **1708×2592 JPEG at 236 ppi**, one 240×90 logo |
| `pdftotext` output | 1 code point | it is a pure scan |
| Ink segments found, 300 DPI | **9,345** | replica of `detectOrthogonalLineSegments` |
| Segments kept | **1,000** | `MaxLineCandidates`, `line_detector.go:12` |
| Segments **discarded** | **8,345 (89.3 %)** | |
| Same at 400 DPI | 12,065 found, 1,000 kept (**91.7 %** discarded) | |
| Same at 240 DPI | 7,069 found, 1,000 kept (**85.9 %** discarded) | |

### 1.1 It is not a box-and-line family tree

Look at the document. It is a **decorative painted tree** — a photograph-quality illustration
with brown branches, green grass, a blue sky and a waterfall — and the names are written in
black Arabic **on coloured fruit and leaf shapes** hanging from the branches.

There are no connector lines. There are no name boxes. The structure of the genealogy is
carried by the **painted foliage**, and the parent/child relation is implied by which branch a
fruit hangs from.

This matters enormously, and it invalidates the framing this document started with:

- **Native PDF vector geometry cannot help this document.** The file has zero vector content
  because it is a scan. And a *digitally authored* version of the same decorative tree would
  also have no connector lines to extract — the branches are painted. Plan 019 as originally
  written addresses a document class that this corpus may not contain at all.
- **A raster line detector is the wrong instrument entirely.** The detector thresholds at
  grey < 150 and looks for orthogonal ink runs. On a photograph of a landscape, what it finds is
  the landscape.

### 1.2 The truncation is not merely lossy — it is spatially destructive

At 300 DPI, of the 9,345 candidate segments, the 1,000 that survive are distributed like this
(rows = page quarters top to bottom, columns = page quarters left to right; `kept/discarded`):

```
     102/72      207/553     282/639     148/268
      38/296       0/977       0/980       0/243
      53/263       0/887       0/798       0/297
     170/398       0/768       0/517       0/389
```

**The right three-quarters of the page below the top quarter have zero surviving candidates.**
The sort before truncation is by `fixed` (`line_detector.go:46-51`), which is the row for
horizontal runs and the column for vertical ones, so the survivors are whatever has the
smallest row-or-column index. The truncation is systematic, not random.

Two more measurements make it worse:

- The **median candidate is 54 px** at 300 DPI (≈4.6 mm). That is a letter stroke or a grass
  edge, not a connector.
- **Only 10.2 % of the long segments survive.** 42.8 % of candidates are ≥60 px — long enough
  to be a box edge or a connector — and the cap preferentially discards them, because long
  segments are fewer and spread across the page.

### 1.3 Raising the DPI makes it worse

`minHorizontal = max(28, width/100)` (`line_detector.go:40-41`) scales with resolution, so more
pixels produce more qualifying runs, while `MaxLineCandidates` is a constant 1,000. The
discarded fraction rises with DPI: **85.9 % at 240, 89.3 % at 300, 91.7 % at 400**.

The product currently offers "re-OCR at 400 DPI" as a remedy for a poor scan. On this chart
that remedy makes the geometry loss strictly worse. The 400 DPI option is right for *reading*
and wrong for *geometry*, and the pipeline offers it for both.

### 1.4 Colour segmentation is the correct localiser — and it is free

The name carriers are saturated colour blobs on a distinct background. Segmenting by
saturation and connectivity, with no model:

| Render | Saturated blobs | Leaf-sized (1.5k–20k px) | With an interior name mark |
| --- | --- | --- | --- |
| 300 DPI | 1,579 | 148 | **135 (91.2 %)** |
| 600 DPI | 1,701 | 493 | **404 (81.9 %)** |

Two things fall out:

1. **The chart really does carry several hundred names.** 404 detectable name carriers at 600
   DPI is consistent with the decision document's "500-plus visible names"
   (`docs/ocr-and-import-decision.md:55`). That figure is credible.
2. **Resolution before segmentation roughly triples the yield** (135 → 404). The embedded scan
   is 236 ppi, so 600 DPI is upsampling — no new information, but enough interpolation for
   neighbouring fruits to separate into distinct connected components. The pipeline currently
   renders at 300 DPI for chart analysis (`chartAnalysisDPI`, `chart_analysis.go`), which is
   why it finds a third as many.

### 1.5 Tesseract cannot read these names, and the reason is not scan quality

Crop a single fruit tightly at 600 DPI and the name is large, crisp, black-on-green, and
**perfectly legible to a human**. Tesseract's `ara` model still fails:

| Input | Tesseract output |
| --- | --- |
| Tight 600 DPI crop of "محمد" | `2` |
| Same, globally binarised | `سد`, `(سدا`, `نسدد` |
| Same, colour-masked, local threshold | `!` |
| Whole page, 300 DPI, `--psm 11` | 342 garbage "words" — digits and fragments |

The binarised image shows why: the decorative display face **fuses the letters into a single
connected mass**. Tesseract's Arabic model is trained on printed Naskh, where letters connect
in the standard way with distinguishable baselines and dots. This face connects them
differently, and that is outside its training distribution.

This is the opposite of a resolution problem and the opposite of what the KITAB numbers
predict for clean print. It is a **font** problem, and it is invisible in every benchmark
because no benchmark contains this font.

**Consequence.** For this document class the reader must be a vision model, not Tesseract —
and it must be asked the *right* question.

---

## 2. Why the hosted-vision route produced 8 %, and what the right question is

The v8 run asked the model to **find names across a dense photographic sheet**: 139 provider
calls, 39 name regions against 500-plus visible names, 21 connector paths, no branch proposals
(`docs/ocr-and-import-decision.md:50-57`).

The measurement explains it. The task was *"describe this sheet and list the names you can
find"*, on a page whose ink structure yields 9,345 competing candidates and whose background is
a photograph. The published gap says exactly this should fail:

| Task | Classical CV + OCR | Best vision-language model | Gap |
| --- | --- | --- | --- |
| Node detection (F1) | **98.8 %** | 34.0 % | 2.9× |
| **Edge detection (F1)** | **66.7 %** | **10.7 %** | **6.2×** |
| Edge precision at that operating point | **85.5 %** | — | — |
| Node detection (F1), LoRA-tuned VLM | — | 74.9 % | — |
| Edge detection (F1), LoRA-tuned VLM | — | 46.1 % | — |

FlowExtract (APMS 2026) on real ISO-5807 flowchart PDFs; diagram2graph with Qwen2.5-VL-3B +
LoRA. The diagram literature agrees: frontier VLMs "struggle with arrow-following and
next-step reasoning."

**But the fix is not "a better vision model". The fix is to change what the model is asked.**

The v8 run's mistake is architectural, not parametric: it asked the model to *localise* on a
hard sheet. The task vision models are actually good at is **transcribing one isolated,
high-contrast, already-cropped word** — which is exactly what §1.4 makes available, at ~400
calls instead of one per sheet, and where the reference numbers (Azure 0.13 CER, Gemini 0.13)
actually apply.

So the corrected pipeline for this class is:

```
render 600 DPI
   ↓
colour-segment the name carriers          (deterministic, ~404 blobs, 82% carry a name)
   ↓
crop each carrier, mask to the blob
   ↓
transcribe each crop with a vision model   (one isolated word per call)
   ↓
multi-view consensus per crop              (measured confidence, not a model's opinion)
```

Compare with what is built: render → ask the model to find names on the sheet → 39.

---

## 3. The structure is a separate problem, and it is the hard one

Nothing above recovers a *relationship*. The chart encodes descent in **painted branches**, and
the decision document already says the right thing at
`docs/ocr-and-import-decision.md:251-254`: "Nearest-neighbor geometry can propose, not
establish, kinship."

For a pictorial tree the question is harder still: a fruit's position and its branch tell you
an *ordering*, not necessarily a parent/child pair. The decision document's own §5 warns that
"a lineage link can mean descendant-of across omitted generations."

Three candidate sources of structure, in descending order of reliability:

1. **The painted branches themselves.** A branch tracer could follow the dark-brown branches
   against the sky and grass. This is a research problem, not a 20-minute job, and it is the
   honest open question in this area.
2. **The chart's own generational banding.** Where a chart orders generations top-to-bottom, the
   ordering is evidence of *generation*, not of a specific parent.
3. **Another source.** The family's own records, the lineage books, or the same chart in a
   box-and-line rendering.

Note the corollary: **the family tree PDF is a good source of *names* and a weak source of
*relationships* for this document class.** The decision document's own pilot design — reviewer
draws the edges, machine proposes the names — is closer to right than the reading pipeline it
is attached to.

---

## 4. What Arabic reading actually costs

The decision document's provider shortlist (`docs/ocr-and-import-decision.md:127-140`) is
careful and correct, and it explicitly declines to select a provider. This section supplies
the numbers behind that caution.

### 4.1 Printed Arabic

From KITAB-Bench (8,809 samples, 9 domains, 36 sub-domains), character error rate for Arabic
OCR:

| System | CER | Notes |
| --- | --- | --- |
| Gemini 2.0 Flash | **0.13** | best closed model |
| Azure Document Intelligence / Vision | **0.13** | best on diacritics and text elongation |
| Qaari (open) | 0.20 | |
| GPT-4o | ~0.31 | best at font handling |
| Qwen2.5-VL-7B | 1.20 | |
| Tesseract 5 | **0.54** | the local incumbent |
| EasyOCR | 0.58 | |
| PaddleOCR | 0.79 | |
| Surya | 4.95 | unusable on Arabic |

End-to-end document-to-markdown accuracy is lower than the OCR numbers: Gemini-2.0 Flash
reaches 65.65 % MARS and GPT-4o 65.12 %, while Qwen2-VL-7B collapses to 21.42 %.

**No single system wins across Arabic complexities**: Azure leads on diacritics and elongation,
GPT-4o on fonts and tilted text, Tesseract on clean single-column print. And **configuration
swings the score more than model choice does** — in one benchmark, changing image resolution
and output token limits moved a single model by 14 points.

§1.5 is the concrete counter-example to "Tesseract is fine for clean print": the Jasser names
are clean, large, high-contrast and still unreadable, because of the font.

### 4.2 The ranking implication

The gap between Tesseract at 0.54 and the best hosted models at 0.13 is real and fourfold. It
is **not** a reason to select a hosted provider yet, for three reasons that are all about this
corpus rather than the benchmark:

1. **The benchmark corpus is not this corpus.** Every number above is from public Arabic
   document collections that contain no family charts. Plan 018 measures the local baseline on
   the real corpus, against adjudicated gold.
2. **The chart is not running prose.** A chart is isolated words in large display faces. §1.5
   shows the failure mode is the *font*, and the published numbers say nothing about it.
3. **A provider decision is a privacy decision.**
   `docs/ocr-and-import-decision.md:513-515` requires that provider, region, retention
   behaviour and permitted data use be recorded before a route is enabled, and that local
   failures not silently trigger a cloud fallback. Adding a hosted provider is a
   data-processing decision about documents that may concern living people, and it is not the
   decision that plan 018 forces.

**Conclusion.** The hosted providers are now in scope, not merely challengers. The privacy
decision recorded in `docs/ocr-and-import-decision.md` §11 permits hosted processing for these
documents, and §1.5 shows the local reader cannot handle this font class at all. So the
comparison is not "local versus cloud" — it is **"which hosted reader, and does it beat the
local baseline where the local baseline works"** — and plan 018 measures that directly.

### 4.2.1 The privacy decision, and what it changes

**Decision, 2026-10-21: hosted processing is permitted.** The product owner has determined that
these family documents carry no high-privacy or sensitive content, and that documents may be
sent to hosted OCR and vision providers. Consequences, and they are substantial:

- **The hosted-vision adapter stops being an experiment.** The pilot currently pins one file
  by SHA-256 and treats every other document as ineligible
  (`internal/sourceprocessing/chart_analysis.go:29`). That restriction can be lifted to a
  policy rather than a hard-coded digest.
- **Plan 019's per-crop transcription arm gets a real reader**, rather than an interface with
  nothing behind it. That is the difference between a plan and a working feature.
- **The provider is selected by measurement, not by preference.** With several candidates now
  permissible, plan 018's comparison becomes the selection mechanism.
- **One caveat, stated once.** A 1432 AH chart contains grandchildren who are alive today.
  `PRODUCT.md:1402` and §43 require living-person rules for *published* trees, which is a
  separate decision from the reading path — but the reading path now handles documents about
  living people, and that is worth one line in the privacy record rather than an assumption.

Two rules from `docs/ocr-and-import-decision.md:513-515` survive the decision and are not
optional: a provider, region, retention behaviour and permitted-data-use must be **recorded
before the route is enabled**, and a local failure must **not** silently fall through to a
cloud route.

### 4.3 Books are a different reading problem, and it is not solved by a general model

For historical manuscripts the picture collapses. On the Muharaf corpus (1,334 line images),
Tesseract achieves 69.2 % CER and the best vision model 70.4 % — statistically
indistinguishable — and the best OCR-conditioned correction reaches 60.6 % CER. That study's
central finding is the **OCR-prior recoverability principle**: giving a vision model the OCR
text alongside the image helps only when the OCR output remains visually and textually
recoverable, and actively *hurts* when the prior is script-mismatched.

For **printed** books the ceiling is much higher but only with supervision. The OpenITI and
JSTOR study of Arabic periodicals found that all evaluated commercial Arabic OCR systems
performed below 75 % accuracy, making manual correction "prohibitive" — and then showed that
**Kraken**, a segmentationless sequence-to-sequence engine, reached **97.41 % character
accuracy** and 98.46 % Arabic-script-only accuracy on the same material, with only
**800–1,000 transcribed lines** of training data for that document's typeface.

The ceiling for Arabic printed text is very high, and what unlocks it is **typeface-specific
supervision**, not a bigger general model. §1.5 is the same lesson from the other direction:
the font is the whole problem.

---

## 5. Confidence is the hard part, and the model cannot supply it

- **Verbalized self-confidence** separates correct from incorrect extractions at AUROC
  **0.54–0.74** — at times worse than chance (ConfBench; Perception/Layout/Validation).
- **Mean token log-probability** reaches 0.705 and **self-consistency** over five samples
  0.744, but all three "collapse toward all-positive behaviour at any practical threshold":
  at threshold 0.5 they retain recall 1.000, meaning they flag everything or nothing
  (ExtractConf).
- **What works is disagreement-based confidence** — cross-view and cross-model disagreement
  combined with OCR and image-quality signals — reaching **0.928** AUROC.
- **VLMs fail by fabrication, pipeline OCR by omission**, and capped CER/WER hides the
  difference: uncapped-to-capped inflation reached 23× on full suites and 107.6× on degraded
  sets (Kim et al. 2026). On a chart, a fabricated name is indistinguishable from a real one.

This repository has already made the right call on the substance: `PageReading.Confidence` is
documented at `internal/sourceprocessing/types.go:260-262` as "the provider's raw score,
retained for comparison rather than treated as a calibrated probability," and the UI says so
in Arabic. The gap is that **there is no calibrated score to compare it against**.

**Multi-view consensus** is how to get one. Generate *k* augmented variants of the same crop
(padding, blur, grid-warp), transcribe each, align them with a Needleman–Wunsch-style aligner,
take a per-column vote; the confidence is the vote fraction — a physical quantity, not a
model's opinion. Measured on 622 historical death records: **+4 percentage points** of field
accuracy over a single pass, recovering 4–7 of the 7 name fields the single-pass baseline
missed entirely, with grid-warp giving the strongest confidence–accuracy correlation
(0.642).

Two rules it must not break:

- **Route, do not blanket-apply.** OCR-conditioned correction helps only where the OCR prior is
  recoverable, and hurts where the prior is script-mismatched. Enable the second reader per
  region, not per document.
- **Bands, not bare numbers.** The corporate family-tree extraction work — the closest prior
  art — concluded that numerical confidence values "can appear more precise than they truly
  are," and recommended a **band-based** scheme. The repository already has the vocabulary:
  `entity_resolution_candidates` stores `match_class` beside its numeric `score`
  (`db/migrations/0015_entity_resolution.sql:43`), and its queue index orders by
  `(review_status, match_class, score DESC, created_at DESC)` (`:68`).
- **The model can never pre-approve.** `candidateStatus()`
  (`internal/sourceprocessing/processor.go:1085`) forces every candidate status to
  `needs_review` regardless of what the model returned, and `internal/ai/client.go` rejects any
  AI response whose `review_required` is not true. A high-confidence extraction is a candidate
  that sorts first in a review queue, not a fact.

---

## 6. What the import writes today, and the two gaps that remain

An earlier draft of this analysis asserted that the platform "cannot bootstrap a person from a
document," on the evidence that `internal/sourceprocessing/review.go:122-177` materialises only
a `source_statement` for an entity candidate. **That claim was true of the review path and is
no longer true of the platform.** The tree-draft import has its own materialisation. Auditing
it gives a much sharper result.

### 6.1 What the tree-draft import writes

| Record | Where | Assessment |
| --- | --- | --- |
| `trees` (private) + `tree_versions` (draft) | `tree_drafts.go:473`, `:476` | correct |
| `people` — one **new** row per included node, `gender='unknown'` | `tree_drafts.go:487` | **gap 1** |
| `tree_nodes` binding person to the draft version | `tree_drafts.go:490` | correct |
| `tree_relationships`, `status='unresolved'`, `source_id` set | `tree_drafts.go:513` | **gap 2** |
| `source_tree_draft_imports` manifest | `tree_drafts.go:521` | correct |
| `source_tree_import_node_evidence` — page evidence, label, region | `tree_drafts.go:529` | correct |
| `source_tree_import_edge_evidence` — page evidence, connector path | `tree_drafts.go:538` | correct |
| retraction ledger and `RetractTreeDraft` | `tree_drafts.go` | correct: reversible without a restore |
| `source_statements` | **nowhere** | **gap 2** |
| `person_aliases` | **nowhere** | **gap 3** |
| `entity_relationships` | **nowhere** | **gap 2** |

The pilot closed the materialisation, provenance and reversibility requirements. Three gaps
remain, and two of them are serious.

### 6.2 Gap 1 — imported people are never matched against existing people

Every included node becomes a **new** `people` row (`tree_drafts.go:487`). Nothing compares it
to the rest of the database. `entity_resolution_candidates` is written in exactly one place —
`internal/entityresolution/service.go:274` — from a run that sweeps an entire entity type,
because `RunInput` carries only `EntityType` and no scope
(`internal/entityresolution/types.go:76-78`). The import never proposes a candidate.

A family tree naming `محمد بن عبدالله`, who already exists because a cousin's tree was
imported last month, therefore creates a **second** person with the same normalised name and
no link to the first. Nothing warns anyone. The duplicate becomes visible only if somebody
later runs a full resolution sweep — by which time the imported person may already be a node
in a published tree.

This is the failure `docs/ocr-and-import-decision.md:66` names, and the pilot is on the wrong
side of it. The correct shape is the one the decision document names at `:64`: the import
produces **candidates for the existing entity-resolution surface**, where a human confirms a
merge explicitly and can reverse it.

### 6.3 Gap 2 — imported relationships never enter the research graph

An imported relationship is a `tree_relationships` row with `status='unresolved'` and a
`source_id` (`tree_drafts.go:513`). It lives in the tree version and nowhere else. It is not an
`entity_relationships` row, not a `claims` row, not a `claim_evidence` row, and there is no
`source_statements` row for it.

**The import does not operate the platform.** It produces a private draft tree a researcher can
look at. It adds nothing to the evidence-aware lineage graph that the research agent reads,
that hybrid retrieval cites, or that the contradiction detector compares. For a platform whose
thesis is that family trees and lineage books will supply the data that operates it, this is
the gap that decides whether the feature is a viewer or a source.

The schema has had the slot since it was written: `source_statements.extraction_method` accepts
`'imported'` (`db/migrations/0003_research_and_jobs.sql:60`), and no code writes it —
`book_assertions.go:411` writes `'manual'`, `processor.go:876` and `review.go:141` write `'ai'`.

### 6.4 Gap 3 — the raw reading is destroyed and the classical name vocabulary is unused

`person_aliases` is written by `internal/identity/people.go:353` and
`internal/suggestions/service.go:912`, but **not** by the import. So:

- The **OCR'd spelling** is destroyed by normalisation at `tree_drafts.go:487`, which stores
  only `identity.NormalizeArabicName(node.NameAR)`. That function
  (`internal/identity/normalization.go:10`) strips the tatweel (`U+0640`), folds `أإآٱ→ا`,
  `ى→ي` and `ة→ه`. Right for matching, wrong for evidence: a later correction overwrites the
  reading, and a reviewer cannot see what was on the page. `alias_type='source_spelling'`
  exists precisely for this, and it is source-scoped
  (`internal/identity/validation.go:19-22`).
- The **classical Arabic name components** — *ism*, *kunyah*, *laqab*, *khitab*, *nisbah*,
  patronymic — have no home in the import, even though `person_aliases.alias_type` already
  accepts `kunyah`, `laqab`, `nisbah` and `source_spelling`. This is the vocabulary the
  manuscript-cataloguing literature uses, and it is the vocabulary §7 needs.

### 6.5 Gap 4 — there is no dry run

`ImportTreeDraft` (`tree_drafts.go:409`) writes the tree, the people, the nodes, the
relationships, the manifest and the evidence rows in one transaction. There is no
parse-validate-report phase. The requirement is explicit at
`docs/ocr-and-import-decision.md:396-398`, and `TreeDraftImportResult` already has the shape
for it (`ImportedPeople`, `ImportedRelations`, `StagedRelations`, `Warnings` —
`tree_drafts.go:497`, `:503-504`).

The collision count is the number a researcher most needs and currently cannot get, because
§6.2 says the import does not know.

---

## 7. Books: the database is the constraint set

The decision document's §6 is a good workflow for books and its §12 sequence is sound. One
architectural argument it does not make, and which changes the ordering of the work, is this:

**A lineage book's value is not its text. It is its nisba dictionary and its naming grammar.**
`Kitāb al-Ansāb` of al-Samʿānī indexes some 4,400 nisbas — the adjectival and relational
suffixes identifying origin, occupation, tribe and locality. The books cross-reference one
another by abbreviation, and the same abbreviation means different things in different works.

That makes the hard part of reading a book **semantic, not optical**. An unpointed reading of
`الصـ...` is hopeless in isolation. It is resolvable when exactly one `canonical_name_ar` in
the platform is within edit distance — and the platform's people, families, tribes and places
*are* that constraint set. The disambiguation resource is the database, and the database is
populated by the tree imports.

Three consequences:

1. **Books come after trees**, not alongside them. A book read before the trees are in has no
   constraint set and no way to disambiguate anything.
2. **The import must write to `person_aliases`** (§6.4), because that is where the nisba, the
   kunyah and the source spelling live.
3. **The OCR for books is a supervised track**, not a vendor selection (§4.3).

---

## 8. The recommendation

1. **Measure first, on a real corpus, and include the document *class*, not just the
   digital/scanned split.** The one chart we have is a pictorial tree, and no amount of
   per-file tuning would have revealed that from the PDF metadata. Plan 018 does this, and
   classifies each document before measuring it.
2. **Replace "recover the connectors" with "localise the name carriers, then read each in
   isolation."** For pictorial charts the localiser is colour segmentation (§1.4); for
   box-and-line charts it is vector geometry where available and raster detection otherwise.
   The reader is a vision model for pictorial charts, because Tesseract cannot read the font
   (§1.5). Plan 019 does this, as a **new arm in the existing harness**, measured against the
   existing arm rather than asserted.
3. **Stop investing in hosted vision as a *localiser*, and start investing in it as a
   *reader*.** The 8 % result is a category result, not a prompt result: the v8 run asked the
   model to *find* names across a dense photographic sheet, which is where VLMs are 6× worse
   than CV. The same hosted model is the right tool for *transcribing an isolated crop*, which
   is a different and much winnable job — and with hosted processing now permitted
   (`docs/ocr-and-import-decision.md` §11), that is the production path for this font class,
   because the local reader cannot read it at all (§1.5).
4. **Close the identity gap before the import is used at scale.** Plan 020: match imported
   people against existing ones through the existing entity-resolution surface, add a dry run
   that reports collisions before anything is written, preserve the raw reading as a
   `source_spelling` alias, and write the import into the research graph as `source_statements`
   with `extraction_method='imported'` plus `entity_relationships`.
5. **Give the review queue a measured confidence**, derived from cross-view agreement and
   expressed as a **band** (§5).
6. **Keep everything else in the decision document unchanged.** The dry-run, preview, explicit
   conflict handling, idempotency, reversibility, prohibitions and per-row provenance it
   requires are still required, and the pilot still does not satisfy all of them.

## 9. Two landmines that are not this plan's problem but will become someone's

- **There is no committed PDF fixture.** A search of the repository finds no `.pdf` under any
  test or testdata directory; the only real PDF on the machine is the Jasser chart in
  `.data/source-storage/`, which is private by design and gitignored. So CI cannot exercise
  the PDF extractor, the page-evidence path or the tree-draft path end to end. The corpus in
  plan 018 must stay private, but the repository needs at least one **synthetic** PDF fixture —
  generated by the test, not a real family document — so the extraction path has a regression
  test that does not depend on a private file.
- **`upload_test.go:186` pins the format matrix across two languages.**
  `TestSourceFormatPanelMatchesTheSharedMatrix` requires every accepted type in Go to appear
  in `apps/web/src/components/SourceProcessingPanel.tsx` and vice versa. The working tree has
  already changed that matrix (`upload.go:151` adds `application/pdf`). Whoever next adds
  image formats must change that test and the component in the same commit, and must decide
  whether the format matrix becomes a matrix with a **confidence dimension** rather than a
  yes/no list — which is what `docs/ocr-and-import-decision.md:96-101` asks for.

## 10. The honest risk in all of this

The plans below are gated on **two Arabic-capable annotators and an adjudicator**, and the
decision document already flags that "reviewer availability is unknown." Every accuracy number
in the literature above is from a public corpus that does not contain this corpus, and the
measurement above is one document with no gold annotation.

**If the annotation cannot be staffed, none of this can be qualified, and the pilot remains
unqualified.** That is the true critical path, and it is not an engineering task. It is worth
saying plainly, because the alternative — shipping on the published benchmark numbers and the
Jasser smoke run — would be exactly the kind of claim `docs/ocr-and-import-decision.md` was
written to prevent.

One more thing the measurement changes about the risk. The documents we now know about are
**pictorial**, and §1.5 shows a mainstream OCR engine cannot read their font. That is a
stronger argument for the vision-model route than any benchmark number, and a stronger
argument for the privacy decision being unavoidable rather than optional: if a hosted reader
is required to read the font, then §4.2's "keep it local" conclusion cannot hold for this
document class, and the data-processing decision has to be made explicitly.

## 11. The plans

| Plan | Scope | Gate |
| --- | --- | --- |
| `plans/018-family-tree-import-benchmark.md` | Collect the corpus, produce the adjudicated gold, **classify each document's chart style**, run the local baseline **and the hosted readers**, and report the measured state. | Measured state recorded; the class and path that can be supported are named; nothing is qualified without independent review. |
| `plans/019-family-tree-import-name-localisation.md` | Replace the ink-run line detector with a **name-carrier localiser** — colour segmentation for pictorial charts, native vector geometry for box-and-line digital charts, the raster detector only as a fallback — and add a **per-crop vision transcription arm** with a provider enabled, to the harness. | The new arms are measured against the same gold, on the same documents, as additional pipelines in the same report. |
| `plans/020-family-tree-import-identity-linking.md` | Match imported people against existing ones through the entity-resolution surface, add a dry run that reports collisions, preserve the raw reading and the classical name components, and write the import into the research graph. | No imported person is created without an explicit collision report; nothing enters the research graph without a `source_statements` row carrying `extraction_method='imported'` and a resolvable evidence id. |

`docs/ocr-and-import-decision.md` remains the decision document. This file is the evidence and
the recommendation that sit underneath it.
