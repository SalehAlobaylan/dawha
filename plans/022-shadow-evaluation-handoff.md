# Plan 022 — shadow-evaluation handoff

> **Later follow-up, 2026-10-27:** The reviewer reached 111 actions: 80 accepted
> or corrected names, 14 deferrals, and 17 rejections. There are 79 qualifying
> shadow labels, all in `calligraphic_dense`; the evaluation remains not ready.
> Plans 023–026 in the [plan index](README.md) now specify the user-selected
> staged local pipeline. Preserve the existing evidence and its cohort.

> **Pilot update, 2026-10-27 14:43 Asia/Riyadh:** The class map was confirmed
> and frozen, the private corpus was queued, and a human reviewed nine regions.
> Eight accepted or corrected names have shadow evidence, all in one class; one
> rejection has no truth label. The outcome is no result and further human
> review is deferred. The counts below describe the earlier handoff snapshot;
> see [Plan 022](022-name-candidate-adjudication.md) for the current record.

Status: **implementation is code-complete; the evaluation has not been run.**
This file is a handoff for another agent. It explains what is missing, what is
forbidden, and the exact path to produce a real result.

## 1. The objective

Plan 022 added a bounded name judge. The remaining work is **not code**:

1. Collect at least **300 accepted-or-corrected reviewer decisions** with a
   candidate snapshot and a judge output.
2. Cover at least three pre-registered source classes:
   `clean_print`, `calligraphic_dense`, `low_resolution_screenshot`.
3. Run `make name-eval` with a class map.
4. Apply the pre-registered go/no-go rule and record the result — including a
   **no-result** if the rule fails or the sample cannot be collected.

Do not claim quality, lift, adoption, or a ranking improvement before that
happens. Do not use the judge's own output as the truth label. Do not insert
synthetic decisions into the ledger. A fabricated sample is worse than no
sample.

## 2. Current state (verified 2026-10-27)

### Code

- Local provider: Liquid `d1:3b` through Ollama at
  `http://127.0.0.1:11434/v1/systemone`.
- `NAME_JUDGE_PROVIDER=liquid` is the default.
- `.env.example` has `NAME_JUDGE_MODE=active`; the code default remains `off`
  when the environment is absent.
- `NAME_JUDGE_IMAGES=false`.
- The active row is a separate opinion only: no reorder, no prefill, no
  percentage, no auto-accept.
- The optional hosted Jev adapter remains, selected only with
  `NAME_JUDGE_PROVIDER=jev`.

### Verification already run

- Python suite: 78 passed; `ruff` clean.
- Web suite: 77 passed; TypeScript clean.
- Go: full `./...` suite passes.
- `make verify`: passes.
- `make verify-full`: passes.
- `make db-verify`: 901 tests, 895 passed, 0 failed, 6 unrelated skips,
  0 database skips.
- `make e2e`: all 30 journeys pass.
- `make generated-check`, `make docs-check`, `git diff --check`: clean.

### Data

- Local database:
  `postgres://dawha:dawha_local@localhost:55432/dawha`
- 2 users, 5 sources, **1** source file, **0** suggestions, **0** decisions.
- The one source file is the private Jasser chart:
  - source: `2ecc30fa-21ce-4e58-aeec-6991351ee695`
  - source file: `bdb811aa-6da7-4d73-8883-5b6ee52f79ac`
  - file: `شجرة أسرة الجاسر ١٤٣٢.pdf`
- Private corpus: `~/.dawha/corpus`
  - `manifest.json` lists 11 chart pages.
  - `sources.json` records styles and provenance, **not gold names**.
  - `predictions/carrier_localiser.jsonl` has regions only.
  - `predictions/carrier_vision.jsonl` has no useful readings.
  - There is no adjudicated gold set and no reviewer decision set.
- `make name-eval` currently reports 0 decisions and `ready=false`.

The two `[ ]` items in `plans/022-name-candidate-adjudication.md` are blocked on
real reviewer truth. They are not an implementation failure.

## 3. Non-negotiable constraints

- The judge is a **bounded judge**, not a reader, normalizer, authority, or
  writer. It may choose one of 1–12 supplied candidates or `unclear`.
- Never auto-accept a name, identity, or relationship from a confidence
  threshold.
- Shadow mode is invisible and must not reorder chips. Active mode is a
  separate row only.
- No percentage is shown before calibration.
- Do not send source IDs, family IDs, account IDs, reviewer identity, page
  images, or API keys to the provider. Image mode stays off unless a separate
  decision is recorded.
- A missing or malformed judge must never block a review decision.
- Keep Plan 018–021 changes. Leave the work uncommitted.
- If you commit anything, the human is the only author. No AI/agent
  attribution, no `Co-Authored-By` trailer.

## 4. Recommended path: build the review queue, then a human decides

This is the only path that produces a real Plan 022 result. The agent can do
everything except the decisions.

### Step 0 — start the local model and services

```sh
/opt/homebrew/bin/ollama serve       # or: brew services start ollama
/opt/homebrew/bin/ollama list        # d1:3b must be listed
```

Run the app in **shadow** mode for data collection, not active mode:

```sh
NAME_JUDGE_MODE=shadow \
NAME_JUDGE_SAMPLE=1 \
NAME_JUDGE_IMAGES=false \
npm run dev
```

`npm run dev:api` defaults to active, so the explicit `NAME_JUDGE_MODE=shadow`
must win. `npm run dev:ai` sets the local Liquid provider and endpoint. Confirm
in the authenticated app (the config endpoint requires a session), or with a
session cookie:

```text
GET /api/v1/name-judge/config
# expect mode=shadow, available=true, imageEnabled=false
```

### Step 1 — import the corpus pages into the product path

Use the normal source upload/processing path if possible; it builds the page
evidence, suggestions, and crops the bench expects. If there is no import CLI,
write a one-off importer that mirrors the product path:

- create a private `sources` row for each chart (`source_type='family_document'`,
  `visibility='private'`, `created_by` a real reviewer user);
- copy the original into `.data/source-storage/sources/<source_id>/` and record
  its SHA-256 and byte size;
- create a `source_files` row with `document_kind='family_chart'`,
  `mime_type`, `processing_status='queued'`;
- run the source-processing worker so it creates chart analysis, page evidence,
  and `source_tree_suggestions`.

Do **not** commit corpus files. `make ocr-corpus-check` must stay clean.

If the worker cannot produce suggestions because the reader is unavailable, the
bench still supports manual transcription. Do not insert empty suggestions as
truth; a suggestion with no candidate still needs a human-entered name and a
real decision row.

### Step 2 — choose three honest source classes

The class map is keyed by **source file UUID**, not document id, and must use
exactly these values:

```json
{
  "00000000-0000-0000-0000-000000000001": "clean_print",
  "00000000-0000-0000-0000-000000000002": "calligraphic_dense",
  "00000000-0000-0000-0000-000000000003": "low_resolution_screenshot"
}
```

Suggested honest assignment from the local corpus:

- `clean_print`: `jasser-1413` (drawn leaf, 202 ppi), `albalihi` (printed
  poster, 200 ppi), `alajlan` (printed leaf canopy), `alsabah` (vector boxed
  chart).
- `calligraphic_dense`: `alfaraj` (calligraphic plus leaf/oval),
  `alalsheikh` (GIF), `alsuqran` (96 ppi desert tree with table and branch
  calligraphy), `alhayyan` (branch plaques and trunk panel).
- `low_resolution_screenshot`: use a genuinely low-resolution screenshot, not a
  high-resolution source relabelled to fill the class. `tmp/pdfs/family-tree-review/page.png`
  (992×1404) is a local low-resolution rendering of the Jasser chart and is a
  better fit than a 300 ppi original. If you import it, give it its own source
  file id and classify it as `low_resolution_screenshot`.

Aim for at least 100 accepted-or-corrected decisions per class; the gate only
requires 300 total and at least one per class, but a per-class comparison is
the point. Freeze the map **before** looking at the report. Keep it outside the
repository, e.g.:

```sh
mkdir -p ~/.dawha/name-eval
cat > ~/.dawha/name-eval/source-classes.json <<'JSON'
{
  "<source-file-uuid>": "clean_print",
  "<source-file-uuid>": "calligraphic_dense",
  "<source-file-uuid>": "low_resolution_screenshot"
}
JSON
```

Record the map's date/hash in the plan or a local note.

### Step 3 — collect the decisions

- A human reviewer works the bench. The reviewer must see the source crop,
  machine reading, candidates, and the editor.
- **Shadow mode must stay invisible.** The reviewer must not see the judge's
  choice; that is what keeps the comparison honest.
- A decision counts as truth only when `decided_text_ar` is non-empty, i.e. the
  reviewer accepted or corrected the name. Rejected and deferred decisions do
  not count toward the 300 truth-labeled sample.
- Each counted decision needs:
  - a non-empty `candidate_snapshot`;
  - `normalizer_version='ar-name-keys-v1'`;
  - a `judge_output` with `mode='shadow'`, a provider/model/prompt version, and
    `shown=false`;
  - a real reviewer and a `decided_text_ar`.
- Wait for the judge request to finish before saving. The local model is fast,
  but a decision saved before the result arrives has no judge output and is a
  measurement gap, not a counted decision.
- Monitor the ledger:

```sh
docker compose exec -T db psql -U dawha -d dawha -P pager=off -c "
  SELECT count(*) AS decisions,
         count(*) FILTER (WHERE decided_text_ar IS NOT NULL AND decided_text_ar <> '') AS with_truth,
         count(*) FILTER (WHERE candidate_snapshot IS NOT NULL AND jsonb_array_length(candidate_snapshot) > 0) AS with_snapshot,
         count(*) FILTER (WHERE judge_output IS NOT NULL AND judge_output <> '{}'::jsonb) AS with_judge
  FROM source_tree_suggestion_decisions;"
```

If `with_judge` is much lower than `with_truth`, fix the timing/attachment path
before collecting more. Do not hand-write judge output.

### Step 4 — run the evaluation

```sh
DATABASE_URL=postgres://dawha:dawha_local@localhost:55432/dawha \
  make name-eval \
  NAME_EVAL_ARGS="--class-map $HOME/.dawha/name-eval/source-classes.json --json" \
  > ~/.dawha/name-eval/report.json
```

Also run the human-readable report for the gate and the metrics. It prints no
candidate or file names.

Check:

- `shadowGate.ready=true`;
- `judgedWithTruth >= 300`;
- each of the three classes has at least one truth-labeled decision;
- `candidate recall@8` is measurable and at least 70%; if it is below, stop and
  improve candidate generation — a judge cannot choose a name the pool missed;
- the judge comparison is measurable (`candidatePresent >= 100`).

`ready=true` only means the sample gate is met. It is not a pass.

### Step 5 — apply the pre-registered rule

From `docs/name-judge-privacy-and-evaluation.md`:

1. Candidate recall@8 ≥ 70%.
2. Judge top-1 improves by at least 10 percentage points over the deterministic
   baseline on candidate-present decisions, and the 95% paired bootstrap
   interval excludes no improvement (`lower > 0`).
3. The judge abstains more often when the human correction is absent from the
   pool than when it is present.
4. Wrong answered top-1 does not increase.
5. The improvement is not only on common names; inspect the source/file
   coverage (`reader`, `ai`, `file`, `common`) and the rare/file-specific
   subsets.
6. Reviewer time is not worse. Shadow data alone cannot infer an A/B effect;
   a baseline period or an explicit A/B is required for this criterion.
7. Privacy approval covers the exact data sent. Local inference is approved;
   no IDs or images are sent; `NAME_JUDGE_IMAGES=false`.

If the rule passes, record the result and only then consider adopting model
ordering or a percentage. The active row already exists by operator decision;
that display alone is not adoption.

If the rule fails, keep the deterministic ranker and record the no-result. Do
not describe the judge as better. Keep shadow logging off for real work.

### Step 6 — record the result

Update:

- `plans/022-name-candidate-adjudication.md`
  - tick the three remaining `[ ]` items;
  - record the sample size, per-class counts, date, class-map hash, and the
    pass/fail/no-result;
  - do not add a quality claim that the report does not support.
- `docs/name-judge-privacy-and-evaluation.md`
  - replace “not run” with the measured result, or with an explicit
    “not run, blocked by X”;
  - keep the distinction between the active opinion row and an adopted ranker.
- `plans/README.md` and `docs/phase-status.md` if the status line changes.
- `docs/import-reading-strategy.md` / `docs/ocr-and-import-decision.md` only if
  the result changes their claims.

Do not commit private names, file IDs, the class map, or the raw report.

## 5. Alternative: use an existing labelled dataset

If another environment already has at least 300 real reviewer decisions with
candidate snapshots and judge outputs, point `name-eval` at that database and
run the same command with the class map. The decision semantics must match:
`source_tree_suggestion_decisions`, `candidate_snapshot`,
`normalizer_version='ar-name-keys-v1'`, and `judge_output` in shadow mode.
Do not import a dataset whose “truth” was produced by a model.

## 6. If the sample cannot be collected

Do not invent it. Record an explicit no-result:

- mark the three checklist items as not completed;
- state the blockers: zero truth-labeled decisions, no adjudicated gold, and no
  human review batch;
- keep `make name-eval` reporting `ready=false`;
- keep the active row described as an unmeasured opinion, not an adopted
  ranker;
- do not add a quality, lift, or adoption claim.

A no-result is an honest completion of the measurement question. A synthetic
300-row sample is not.

## 7. Key files

| File | Purpose |
|---|---|
| `services/ai-research/app/name_judge.py` | `LiquidNameJudge`, provider selection, cache, strict outputs |
| `services/core-api/internal/sourceprocessing/name_judge.go` | core judge client, settings, validation |
| `services/core-api/internal/sourceprocessing/tree_suggestions.go` | decision ledger, snapshot/evidence validation |
| `apps/web/src/components/SourceSuggestionReview.tsx` | review bench, shadow sampling, separate active row |
| `apps/web/src/lib/nameJudge.ts` | evidence construction |
| `apps/web/src/lib/nameReviewEvidence.ts` | candidate snapshot |
| `services/core-api/cmd/name-eval/main.go` | report, class map, 300-decision gate |
| `docs/name-judge-privacy-and-evaluation.md` | protocol and go/no-go rule |
| `plans/022-name-candidate-adjudication.md` | plan and checklist |
| `.env.example`, `package.json` | local provider and mode defaults |

## 8. Verification after any code change

```sh
cd services/ai-research && .venv/bin/pytest && .venv/bin/ruff check .
npm --workspace apps/web run test
npm --workspace apps/web run typecheck
cd services/core-api && go test ./...

make verify
COMPOSE_PROJECT_NAME=dawha \
  DATABASE_URL=postgres://dawha:dawha_local@localhost:55432/dawha \
  make verify-full
DATABASE_URL=postgres://dawha:dawha_local@localhost:55432/dawha make db-verify
COMPOSE_PROJECT_NAME=dawha make e2e
make generated-check
make docs-check
git diff --check
```

Leave the tree uncommitted unless the user explicitly asks for a commit. If a
commit is requested, use the human's identity only; never add AI attribution.
