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

## In plain language

This plan is the one that needs people rather than code, so here is what it is without the
jargon.

**What we are trying to find out.** We have built a machine that reads a family-tree PDF and
lists the names on it. On the one chart we have tested it against — the Jasser tree — it found
39 names, and a person looking at the same chart can count several hundred. So we know the
machine is not good enough. What we do **not** know is *how* bad it is on other families'
charts, because families make charts very differently: some are painted pictures like the
Jasser tree, some are boxes and typed names, some are photographs of a printed sheet.

**What we need: example charts.** A folder of around 30–50 real family-tree PDFs from
different families, plus some pages from lineage books. Each one needs the family's permission
to use. We have exactly one chart today, and one chart cannot tell us whether the reader works
on charts in general or only on that one.

**What we need: the answer key.** For each example chart, a human writes down the correct
answer — which name is where, and which line joins what. Then we run the machine on the same
chart and compare. If the machine says 39 names and the answer key says 404, we know the
machine found 9 %. If it says 390, we know it found 95 %. Without the answer key we have no
way to tell a good result from a bad one, because the machine's own confidence score is not
proof of anything — it is the machine grading its own work.

**Why two people, and why a third.** Two people label the same chart separately, without
seeing each other's work, so that one missed name does not become the answer key. Where they
disagree, a third person decides. The tooling for all of this already exists:
`tools/ocr-eval/label.html` is a browser page that runs entirely on your own machine, draws
boxes and connector paths, and produces the answer key file.

**If annotator time is the bottleneck.** One person plus a machine's proposed labels, with the
person confirming or correcting every one, is about half the work and produces a usable key —
with one honest caveat: a key built from a machine's suggestions cannot fairly judge *that
same machine*. It can judge the local reader, or a different provider. The labeler already
supports the two-reader workflow, and the machine-proposal variant is a change to one of the
two readers, not to the tool.

**The smallest version that is worth doing.** Six charts is enough to start: three painted or
photographic charts like the Jasser tree, and three boxes-and-typed-names charts, so that both
known kinds of chart are represented. Five of them are used while building; one is held back
untouched to test with at the end. Books can wait — they are a different problem and a
different track.

**Why it is worth the trouble.** The platform's whole claim is that a machine output is never
a fact. If we skip this and ship the reader on the strength of one chart, the next family that
uploads their tree gets garbage with no signal that it is garbage — and that is exactly the
failure the platform is supposed to prevent.

**What is not needed.** No engineering, no infrastructure, no new tooling, and no specialist
genealogy knowledge for the *name* labels. The people who label names need to read Arabic. The
people who label connections benefit from understanding family trees, and can be the same
people.

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
- **A documented private-corpus location convention**, so a second machine or a second
  operator does not invent one. It must be outside the working tree, and the convention must
  say so in `docs/ocr-benchmark-format.md`.

### Step 2: Collect the corpus

`docs/ocr-and-import-decision.md:447` sets the target: roughly **30–50 distinct family charts**
and **200–300 pages from at least 10 book editions**, if available. Requirements:

- **Permission in writing for every document**, from the person who owns it. The hosted
  comparison in the decision document is scoped to one explicitly authorised file by SHA-256
  (`internal/sourceprocessing/chart_analysis.go:29`); the same discipline applies to local
  annotation, and a family tree can contain living people (`PRODUCT.md:1402`).
- **Cover the strata the decision document names**, not only the easy ones: vector PDFs, clean
  scans, low-resolution copies, huge sheets, rotation, cross-page branches, repeated names,
  spouse/parent conventions, damaged lines, historical print, footnotes/columns, name chains,
  negation/uncertainty, and genuinely unreadable regions. Handwriting is a separate stratum
  and is excluded unless it is separately represented.
- **The private manifest** in the format `docs/ocr-benchmark-format.md:59-76` records, per
  document: `document_id`, `source_class` (`chart` | `book` | `mixed`), `split`
  (`development` | `held_out`), and per page a `page_id`, an `image_path` relative to the
  manifest, and — for the third baseline — reviewer-corrected `branches`.
- **Split by document and template, not by page.** Related charts, related editions, rescans and
  synthetic variants stay together. `MIN_HELD_OUT_DOCUMENTS = 3` in `evaluate.py:26` is a
  floor, not a target; the decision document asks for three to five held-out charts beyond
  development, and the report must state the real count.
- **Render every physical page from the original at native source quality.** Do not upscale a
  poor scan. Record the SHA-256 of the exact rendered bytes — the labeler will refuse a
  different image of the same dimensions.

### Step 3: Produce the adjudicated gold

This is the part that cannot be automated, and it is the part that makes the rest mean
something. Requirements:

- **Two Arabic-capable annotators** independently label every page, blind to each other and to
  any machine output, using `tools/ocr-eval/label.html`. Each records every visible name
  occurrence separately — repeated identical names are separate occurrences — with the exact
  visible Arabic, a `readable` / `partial` / `unreadable` status, and the unreadable span when
  partial. Each marks branch regions, traces connector paths, and records a relationship only
  with explicit visible evidence text.
- **A genealogy/domain adjudicator** loads both independent exports and the exact source
  images, and resolves every difference. The tool already requires an explicit adjudicator
  decision on every relationship even where both readers agreed; do not work around it.
- **The adjudicated JSONL is the `--gold` file.** Keep the adjudicator id, the two independent
  annotator ids, and the per-item decision ledger. The evaluator's
  `pending_independent_review` status is deliberate and must not be overridden by this plan.
- **Say plainly who labelled what.** If the same person wrote the corpus and the labels, the
  report says so. Plan 014 required exactly this of its own corpus and its lesson stands.

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
- [ ] An adjudicated gold file exists, produced by two independent annotators and an
      adjudicator, with the decision ledger retained.
- [ ] All three local baselines are scored against that gold, in one report, with per-document
      results, coverage and abstention reported beside precision.
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

- **Stop if the annotation labour cannot be staffed.** Two Arabic-capable annotators and an
  adjudicator are the critical path, and `docs/ocr-and-import-decision.md:586` already records
  that reviewer availability is unknown. If it cannot be staffed, that is the finding: report
  it, name what a smaller defensible scope would be, and leave the accuracy question open.
  Do not substitute one annotator, or a model, for the second reader.
- Stop if the corpus cannot be assembled with explicit permission. Do not use the authorised
  hosted-vision file as a substitute corpus.
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
