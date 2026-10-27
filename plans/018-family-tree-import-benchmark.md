# Plan 018: Build the corpus, produce the gold, and measure the state that actually exists

> **Executor instructions**: Every item here is a measurement that has never been run. The
> harness is written and tested; the corpus, the gold annotations and the report do not exist.
> Do not produce an accuracy claim that the harness did not compute, and do not qualify a
> pipeline that the report did not score.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: MEDIUM — the risk is not in the code, it is in the annotation labour
- **Depends on**: plan 003 (the format contract), plan 008 (verification), plan 014 (the
  measurement precedent this plan copies)
- **Category**: measurement
- **Planned at**: 2026-10-21
- **Current state, 2026-10-22**: the corpus holds twelve documents (eleven charts, including
  the web-published آل الشيخ GIF, the آل الصباح vector chart and three chat-supplied PNGs), plus
  the reference book; the paired name-level check is implemented
  (`CompareCarrierTexts`, `ocr-eval --compare`) and its first Jasser run paired zero names
  because the proposals are letter fragments at the current render. That is recorded in
  `docs/import-reading-strategy.md` §1.9 as a proposal-granularity finding, not as agreement.
  The **product** has since moved to source-resolution tiled reading: آل الصباح yields 3,014
  proposals with a median height of 36 px, and the reader misreads the calligraphic font even
  at 1,200 DPI (§1.10). The **corpus arm in `ocr-eval` still evaluates the captured page
  images**, so its numbers describe the preview path, not the product's tiled path; adding a
  source-resolution arm is open. There is still no adjudicated gold and no scored report.

## In plain language

**What we are trying to find out.** We have built a machine that reads a family-tree PDF and
lists the names on it. On the one chart we have tested against — the Jasser tree — it found 39
names, and a person counting the same chart can see several hundred. So we know it is not good
enough. What we do **not** know is how good it is on other families' charts, because families
make charts very differently: some are painted pictures like the Jasser tree, some are boxes
and typed names, some are photographs of a printed sheet.

**We do not need answer keys for the charts.** This is the important correction. The answer key
is a *test*, not an input to the feature — if you already had the answer key you would
already have the finished tree, and you would not need the machine. So the corpus is
collected without keys, and it is used for what it can be used for without keys:

- **Building and testing the review screen.** You cannot test whether a suggestions screen is
  usable without charts to put in it. No key needed.
- **Measuring the machine against itself.** The corrections a reviewer makes while using the
  screen *are* the answer key, produced for free by real use. Every suggestion a reviewer
  accepted, corrected or rejected is a labelled example. This is where the accuracy number
  comes from, and it costs no annotation campaign.
- **The one free check we can do today.** We have a second rendering of the Jasser chart: a
  different diagram, the same facts. Run the reader on both. If it finds the same names in
  both, it is consistent. The overlap between the two result sets estimates the reader's
  accuracy **with no human labelling at all**, because we know both diagrams must yield the
  same name set. This is the cheapest useful number in the whole area, and it is available
  now.

**What we need, concretely:**

- **Six to ten more family charts**, from different families, with permission. Half painted
  and half boxes-and-names, so both known kinds of chart are represented. No answer key
  needed for any of them.
- **The second version of the Jasser chart**, which is the paired consistency check.
- **One person who reads Arabic**, doing what they are already doing in the review screen.
  Every correction they make is the measurement.

**What we do not need:** a specialist genealogy knowledge for the name labels, a labelling
campaign, a held-out split, or any of the machinery this plan used to assume. The tooling for
the paired check is the harness in `tools/ocr-eval`, which already scores two prediction files
against each other and against a key when one exists.

**Why the paired check is not the whole answer.** Two renderings of one source are not two
independent sources, and a reader can miss the same name in both. The check bounds the error
rather than eliminating it, and the report must say so. It is the first number, not the last.

**Why this is worth doing at all.** The platform's whole claim is that a machine output is
never a fact. If we skip this and ship on the strength of one chart, the next family that
uploads their tree gets garbage with no signal that it is garbage.



## Why this matters

`docs/ocr-and-import-decision.md` is a thorough design document with a working supervised
pilot behind it, and it is explicit that **no accuracy claim is supported**. The harness that
would produce one — `tools/ocr-eval/evaluate.py`, `run_baselines.py`, and the blind
label/adjudicate UI in `label.html` / `label.js` — has never been run against a corpus. There
is no adjudicated gold file, no held-out chart, and no comparison report anywhere in the
repository or on the machine.

That leaves the whole area resting on two things: published benchmark numbers from corpora
that do not contain Arabic family charts, and a single one-page chart (the "Jasser" chart) whose
hosted-vision run is recorded at `docs/ocr-and-import-decision.md:50-57` as a failure — 39
detections against the chart's 500-plus visible names, no branch proposals.

`docs/import-reading-strategy.md` names the two facts that make this urgent rather than
tidy: the raster line detector truncates positionally at 1,000 segments per page
(`internal/sourceprocessing/line_detector.go:52`), which the chart's own scale guarantees it
will hit; and the hosted-vision route missed roughly 92 % of the names it was pointed at. Both
are numbers, and both are currently unmeasured on the corpus that matters.

This plan produces the numbers. It does not change a production path.

## Current state

- **The evaluator exists and is tested.** `tools/ocr-eval/evaluate.py` computes 22 metrics:
  raw and search-normalised CER, exact-name precision/recall, name-detection precision/recall,
  unreadable- and partial-label detection recall, branch precision/recall by polygon IoU,
  connector precision/recall by discrete Fréchet distance, relationship-structure and
  end-to-end relationship precision/recall, book assertion precision/recall, and page/tile
  coverage. It bootstraps a document-cluster 95 % interval, and it **always** returns
  `pending_independent_review` even when the point targets are met
  (`evaluate.py:543`), which `test_evaluate.py:131-139` pins.
- **The alignment rule is the right one.** A prediction counts only if it carries a
  `reference_id` from a blinded alignment pass *and* the region IoU is ≥ 0.5 on the same page.
  Equal Arabic strings never establish identity — `test_evaluate.py:21-36` and `:78-85` pin
  both the repeated-name case and the wrong-location case.
- **Three local baselines exist** (`run_baselines.py:261`): `full_page_tesseract`,
  `tiled_tesseract` (1024 px tiles, 256 px overlap) and `reviewed_branch_tesseract`
  (tiles restricted to reviewer-corrected branch regions). They call
  `tesseract stdin stdout -l ara --psm 11 tsv` and make no network request.
- **The labeler enforces the discipline the plan needs.** Two independent readers, blind to
  each other and to machine output; an adjudicator who must resolve every difference and must
  explicitly decide every relationship even when both readers agreed
  (`label.js:562`); export blocked until every page has two full reviews, the adjudicator id
  is set, and the loaded page image's SHA-256 matches the recorded digest.
- **The scoring contract is written down** in `docs/ocr-benchmark-format.md`, including the
  targets (≥ 99 % exact-name precision, ≥ 95 % recall for a declared chart class), the
  "two correct names out of 500 must report 100 % precision and 0.4 % recall" rule, and the
  instruction that a small corpus cannot demonstrate general reliability.
- **Nothing is wired up.** `tools/ocr-eval` is referenced by three documents and by **no**
  Makefile target, no `package.json` script and no CI job. Its two test commands
  (`python3 -m unittest discover -s tools/ocr-eval -p 'test_*.py'` and
  `node --test tools/ocr-eval/test_labeler.mjs`) are documented but not run by anything.
- **No corpus exists.** `docs/ocr-benchmark-format.md` closes with: "The first benchmark has
  not been run: no independent Jasser gold or hosted comparison is present in this repository."
- **The raster geometry path is over capacity for the target case.** `MaxLineCandidates = 1000`
  (`line_detector.go:12`) and `spans = spans[:MaxLineCandidates]` (`:52`), with the spans
  sorted by `(fixed, start)` before truncation, so the survivors are the ones nearest the
  page's top-left.
- **The digital/scanned split is unknown.** `PageReading.Method` already distinguishes
  `poppler-pdftotext` from `tesseract_ocr`
  (`internal/sourceprocessing/types.go:263-272`), so the class of every already-processed
  document is computable from stored evidence — but nobody has counted.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Harness tests | `python3 -m unittest discover -s tools/ocr-eval -p 'test_*.py'` | exit 0 |
| Labeler tests | `node --test tools/ocr-eval/test_labeler.mjs` | exit 0 |
| Baselines | `python3 tools/ocr-eval/run_baselines.py --manifest <private>/manifest.json --output-dir <private>/predictions` | writes three JSONL files |
| Score | `python3 tools/ocr-eval/evaluate.py --gold <private>/gold.jsonl --predictions full-page=<private>/predictions/full_page_tesseract.jsonl --predictions tiled=<private>/predictions/tiled_tesseract.jsonl --predictions reviewed-branches=<private>/predictions/reviewed_branch_tesseract.jsonl --output <private>/comparison.json` | writes the report |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |

## Scope

**In scope**:

- `tools/ocr-eval/` — wiring, a manifest validator, an alignment pass, and whatever the
  measurement needs
- `docs/ocr-benchmark-format.md` — corrections the run reveals
- `docs/import-reading-strategy.md` — the measured numbers replacing the projected ones
- `docs/ocr-and-import-decision.md` — the status line and §10, corrected by what was measured
- `docs/phase-status.md` — Phase 14's benchmark row, corrected by what was measured
- `Makefile` — the documented commands
- `.github/workflows/ci.yml` — the harness tests, and only the harness tests
- `docs/` — a benchmark report, with the numbers and the limitations

**Out of scope**:

- **Any production path change.** No extractor, no detector, no worker, no upload contract and
  no review surface changes. Plan 019 does the production change; this plan produces the
  numbers it will be measured against.
- **Any hosted provider.** No DeepSeek, no Gemini, no Azure, no Google. The local baselines run
  offline; the hosted comparison is a separate, explicitly authorised exercise and is not
  required by this plan.
- **Committing any family document, crop, name, label or prediction file.** The corpus and
  every artefact derived from it stay in a permissioned private location outside the
  repository. `docs/ocr-benchmark-format.md` already requires this and this plan enforces it.
- **Changing the targets.** 99 % precision / 95 % recall are the stated pilot targets
  (`docs/ocr-benchmark-format.md`). Measuring against them is in scope; moving them is not.
- Automatic connector interpretation, automatic book-assertion extraction, and canonical
  identity linking. All three remain open and none of them is a measurement.

**Changed by the privacy decision of 2026-10-21.** `docs/ocr-and-import-decision.md` §11
records that hosted processing is permitted for these documents, and
`docs/import-reading-strategy.md` §1.5 shows the local reader cannot read the display face
this corpus uses — so a hosted reader is expected to be part of the production path, not merely
a challenger. **The hosted arms are therefore in scope for this plan's comparison**, and the
comparison must cover, on the same documents and the same gold: the local baseline, the hosted
readers, and the localiser from plan 019. What stays out of scope is *selecting* a provider
before the report exists, and changing the production path.

## Steps

### Step 1: Make the harness runnable, reproducible and guarded

The harness works but nothing runs it. Requirements:

- A `make ocr-benchmark-test` target that runs both harness test suites, and a
  `make ocr-benchmark` target that documents the two measurement commands with their local
  prerequisites (Pillow; Tesseract with the `ara` model; Poppler's `pdfinfo`/`pdftotext`/
  `pdftoppm`). Neither target may reference a path inside the repository that holds private
  data.
- The harness tests run in CI. They are pure and offline, and a broken evaluator must fail the
  build. `jsdom` is already hoisted into the root `node_modules`, so the labeler test resolves.
- **A guard that the private corpus cannot be committed.** The corpus, gold files, predictions
  and the comparison report live outside the repository. Add a check that fails if any file
  matching the corpus and gold shapes is tracked by git. A `.gitignore` entry alone is not
  enough — the plan 012 precedent is a leaked corpus in `db/seeds/`, and it must not recur.
- **The corpus location is fixed, and it is outside the working tree.** `~/.dawha/corpus/`,
  with `sources/` for the originals, `pages/` for the renders the harness reads,
  `manifest.json`, `predictions/` and `reports/`. `docs/ocr-benchmark-format.md` records the
  layout, the render command, and the reading-resolution decision. The guard must fail if anything under
  that path is tracked by git — a `.gitignore` entry is not a guard, and the plan 012 precedent
  is a corpus that leaked into `db/seeds/` and shipped.
- **A render step, so the corpus is reproducible.** The originals are the PDFs; the harness
  reads page images. Rendering them from the original at a fixed DPI is part of collecting the
  corpus, not a manual chore, and the render must be recorded so a rerun produces the same
  bytes the labeler fingerprinted.

### Step 2: Collect the corpus — without answer keys

The corpus is for building and testing the review screen and for the paired consistency
check. **No document in it needs an answer key.** Requirements:

- **Six to ten family charts** to start, not thirty to fifty. Half of them painted or
  photographic like the Jasser tree, half boxes-and-typed-names, so both known kinds of chart
  are represented. More can be added later; the number that matters is the number of *styles*.
- **The second rendering of the Jasser chart**, which is the paired check in Step 3.
- **Permission in writing for every document**, from the person who owns it. The hosted
  comparison in the decision document is scoped to one explicitly authorised file by SHA-256
  (`internal/sourceprocessing/chart_analysis.go:29`); the same discipline applies to local
  annotation, and a family tree can contain living people (`PRODUCT.md:1402`).
- **Cover the obvious strata**, not only the easy ones: vector PDFs, clean scans,
  low-resolution copies, huge sheets, rotation, repeated names, spouse/parent conventions, and
  damaged lines. Handwriting is a separate stratum and is excluded unless it is separately
  represented.
- **The private manifest** in the format `docs/ocr-benchmark-format.md:59-76` records, per
  document: `document_id`, `source_class` (`chart` | `book` | `mixed`), `split`
  (`development` | `held_out`), and per page a `page_id`, an `image_path` relative to the
  manifest, and — for the third baseline — reviewer-corrected `branches`.
- **Render every physical page from the original at native source quality.** Do not upscale a
  poor scan in the manifest. Record the SHA-256 of the exact rendered bytes — the labeler will
  refuse a different image of the same dimensions.

### Step 3: Two measurements that need no answer key

These replace the annotation campaign, and they are the reason this plan is now small.

**The paired consistency check.** We have two renderings of the Jasser chart: different
diagrams, the same facts. Run the reader over both. Because both must yield the same set of
names, the overlap between the two result sets is a direct estimate of the reader's
accuracy — a name found in both is likely correct, a name found in one only is a candidate
error on one side or the other.

- The report must state the caveat plainly: **two renderings of one source are not two
  independent sources**, and a reader that misses the same name in both is invisible to this
  check. It bounds the error; it does not eliminate it.
- The evaluator already supports scoring two prediction files against each other, and against a
  key when one exists. The paired check needs no new scoring machinery — only a way to say
  that two documents are renderings of the same facts, which belongs in the manifest.
- **The paired check is the first number in this area, and it is available now.**

**The corrections-as-key.** Every suggestion a reviewer accepts, corrects or rejects during
real use is a labelled example. Log the correction against the suggestion, and the accuracy
number accrues from use rather than from a labelling campaign.

- The logging must capture, per suggestion: the machine's proposed value, the reviewer's
  final value, the decision, and the reviewer. This is the same append-only revision shape the
  page-review path already uses
  (`internal/sourceprocessing/page_reviews.go`).
- **This is the durable answer to "we would not have answer keys anyway."** The corpus does not
  need keys; the review screen produces them.
- The report must say how many corrections the number is based on, and refuse to quote a
  percentage computed from a handful of suggestions.

**An answer key, if one ever appears, is used as-is.** The adjudicated-gold path in the
harness is already built and must keep working. It is now optional input, not a deliverable.

### Step 4: Run the three local baselines and score them

Run the commands in the table above, over the same corpus, with no network access, and score
with `evaluate.py`. Requirements:

- All three baselines are scored against the same gold, in one report, with the same bootstrap
  seed, so the arms are comparable.
- **The report is generated from the run, not written by hand.** Every number in it traces to a
  fixture.
- **Report per document, not only pooled.** Errors are clustered; a pooled number over a
  30-document corpus with one pathological chart is misleading.
- **The report must include the diagnostics the harness already computes**:
  `readable_name_omissions`, `duplicate_name_proposals`, `name_abstentions`,
  `unreadable_gold_name_occurrences`, `partial_gold_name_occurrences`, `page_coverage`,
  `tile_coverage`, `processing_seconds`, `cost_usd`, `provider_call_count`,
  `ocr_operation_count`, `input_image_bytes`. Coverage and abstention are reported *beside*
  precision, because refusing everything is not success.
- **A deliberately broken arm must be visibly worse**, so the harness is not vacuous. If the
  three baselines score within noise of each other, report that — "no measurable difference" is
  a legitimate outcome and the report must be able to say it.

### Step 5: Produce the digital/scanned split, and the coverage findings

This step is the one that decides what plan 019 is for, and it is the cheapest.

- **Classify every corpus document** as digital (native text layer) or scanned, from the
  stored `PageReading.Method` where the document has already been processed, and from
  `pdftotext` output where it has not. Record the class per document in the manifest.
- **Report every metric split by class.** The three baselines may behave completely differently
  on a native-text PDF and on a phone photograph, and a pooled number hides exactly the
  distinction that decides the architecture.
- **Measure the raster line detector's ceiling on this corpus.** For each chart page, record
  the raw segment count before truncation against `MaxLineCandidates = 1000`, and how many
  segments were dropped. The truncation is positional, so record *where* the loss is as well
  as how much.
- **Report the name-occurrence count per chart page** against the gold. This is what turns
  "39 detections" from an anecdote into a rate.

### Step 6: Decide, and correct the record

- Name the class or classes the measured evidence supports, and the path each class should use.
  If the evidence supports no class, say that, and say what would change it.
- **State the next lever, chosen by the numbers.** If the corpus is mostly digital, native
  vector geometry is the lever (plan 019). If it is mostly scanned, plan 019 is the wrong next
  step and the lever is scan quality and multi-view consensus instead. Record which, and why.
- **Correct `docs/ocr-and-import-decision.md`**: its status line, §10 and the §12 sequence table
  must reflect that the benchmark has been run, with the numbers and the limits, rather than
  continuing to say it has not.
- **Correct `docs/phase-status.md`**: Phase 14's benchmark row must cite the report and the
  measured state, and must not imply a class is qualified that the report did not qualify.
- **Update `docs/import-reading-strategy.md`**: replace the projected numbers with the measured
  ones. Where the measurement contradicts it — including this plan's own claim about the
  raster ceiling — the measurement wins and the document says so.

## Test plan

- The harness tests run in CI and fail the build when broken, including the existing pins for
  occurrence-based alignment, the wrong-location case, the polygon-not-bbox branch scoring, the
  connector path check, and the assertion-polarity check.
- A deliberately corrupted gold file fails the evaluator loudly rather than scoring silently.
- A prediction file with a missing `reference_id` scores as unaligned and counts against
  precision, not as a pass.
- The private-corpus guard fails when a gold-shaped or prediction-shaped file is tracked by git.
- The report is regenerated from a run, and its numbers are traceable to the fixtures.
- The three baselines are scored against one gold in one report with one seed.

## Done criteria

- [ ] `make ocr-benchmark-test` runs both harness suites, and CI runs them.
- [ ] A corpus of permissioned charts and book pages exists in a private location outside the
      repository, with the manifest, and no artefact of it is tracked by git.
- [ ] The paired consistency check is run on the two renderings of the same chart, and the
      number is reported with its caveat stated: two renderings of one source are not two
      independent sources, so the check bounds the error rather than eliminating it.
- [ ] The corrections-as-key path records, per suggestion, the machine's value, the reviewer's
      final value, the decision and the reviewer, so accuracy accrues from real use.
- [ ] Every baseline is scored, in one report, with per-document results, coverage and
      abstention reported beside precision — against a key where one exists, and against each
      other where it does not.
- [ ] The digital/scanned split is reported, and every headline metric is split by class.
- [ ] The raster detector's truncation is measured against the corpus: segments dropped per
      page, and where.
- [ ] The next lever is chosen by the numbers, and it is recorded as chosen — or the evidence
      is recorded as insufficient, with what would change it.
- [ ] `docs/ocr-and-import-decision.md`, `docs/phase-status.md` and
      `docs/import-reading-strategy.md` state the measured state, not the projected one.
- [ ] `make verify`, `make verify-full` and `make e2e` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- **Stop if the corpus cannot be assembled with explicit permission.** Do not use the
  authorised hosted-vision file as a substitute corpus.
- **Do not stop the plan because the corpus has no answer key.** That is the expected state.
  The paired consistency check and the corrections-as-key path are the measurements; a gold
  file is optional input, not a deliverable.
- Stop if the report cannot be regenerated from a run, or if a number cannot be traced to a
  fixture.
- Stop if a verification gate fails twice.
- Stop if the measurement shows the corpus is too small or too homogeneous to support the
  classes the targets name. State the smallest corpus that would, and leave the question open
  with that named.

## Maintenance notes

- The evaluator's `PRECISION_TARGET` / `RECALL_TARGET` are pilot targets for a declared class,
  not universal thresholds, and `evaluate.py` deliberately never returns a qualified status.
  Changing either constant does not qualify anything.
- The corpus must be re-annotated when the corpus grows, because the split is by document and
  template: adding a document to `held_out` invalidates the earlier held-out numbers.
- The corpus and the gold are private. They are not in the repository and must not be. The
  repository holds the *contract*, not the *data* — `docs/ocr-benchmark-format.md` is the
  contract, and it is the part that must survive.
