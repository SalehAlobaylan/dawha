# Name judge: privacy decision and shadow-evaluation protocol

This document records the privacy decision and the shadow-evaluation protocol
for the bounded name judge. The default provider is a **local Liquid `d1:3b`**
decision model served by Ollama at `/v1/systemone`; a hosted Jev endpoint
remains optional. The local provider was approved by the operator for this
checkout. The 300-decision shadow evaluation has still not run, so no quality,
lift, or adoption claim is supported.

## Status

| Gate | Status | Consequence |
|---|---|---|
| Privacy/provider approval | **approved for local Liquid** | Inference stays on this machine by default. Do not send source, family, account, or reviewer ids. Keep `NAME_JUDGE_IMAGES=false` unless a separate image decision is recorded. |
| 300-decision shadow evaluation | **no result; human review pending** | The 2026-10-27 pilot has eight truth-labelled shadow decisions in one class and `ready=false`. No judge quality, lift, or adoption claim is supported. |
| Active recommendation row | **enabled for local Liquid by operator decision** | The separate `رأي النموذج` row may appear. It never accepts a name, prefills the editor, reorders chips, or shows a percentage. No judge output changes a decision by itself. |

The posture in `.env.example` is:

```text
NAME_JUDGE_PROVIDER=liquid
NAME_JUDGE_MODE=active
NAME_JUDGE_SAMPLE=0.2
NAME_JUDGE_IMAGES=false
LIQUID_BASE_URL=http://127.0.0.1:11434/v1/systemone
LIQUID_MODEL=d1:3b
LIQUID_PROMPT_VERSION=name-judge-liquid-v1
```

The code is fail-closed when the environment is absent: an unset
`NAME_JUDGE_MODE` means `off`, and a malformed mode becomes `off`. The active row
is a display decision, not an accuracy claim and not permission to let the judge
change the candidate order.

### Current preregistration and collection

On 2026-10-27 at 14:33:24 Asia/Riyadh, the human reviewer confirmed the class
assignments and the private map was frozen with SHA-256
`861c26da1a33473e54e204c153ee3f8ccad61c7e656b519faeb3fd9a57a677f2`. It assigns
two source files to `clean_print`, four to `calligraphic_dense`, and two genuine
screenshots to `low_resolution_screenshot`; the remaining corpus files are
unclassified. The map and source-file identifiers remain outside the repository.

All eleven local charts have a queue created from detected regions and local
Tesseract readings. The registered classes contain 2,382, 2,109, and 432
regions respectively, of which 1,190, 546, and 67 have local OCR text. These
figures are queue counts, not counts of correct or human-reviewed names; an
unreadable or non-name region must be deferred or rejected. A text-only local
Liquid smoke test succeeded without creating a decision. Hosted chart vision
was disabled for this collection run.

The pre-review report had zero decisions. At 14:43 Asia/Riyadh on the same day,
the pilot report had nine real decisions: eight accepted or corrected names
with snapshots and unseen shadow outputs, all from `calligraphic_dense`, and
one rejection without a truth label. The other two registered classes had zero
qualifying decisions. The gate is not ready, the sample is too small for rates,
and the registered evaluation, paired comparison, and reviewer-time criterion
remain unmeasured. The latter requires a baseline period or an A/B design even
after the shadow sample is collected. Until then the outcome is **no result**.

## What the judge is allowed to do

Liquid `d1:3b` is a **judge over a bounded candidate set**. It is not a reader,
a normalizer, an authority, or an identity/relationship source, and it is never
a writer. It receives candidates the review bench already built and may choose
exactly one supplied option or say `unclear`. The core rejects a choice outside
the supplied options and does not use a confidence threshold to accept a name.

The local provider is a decision model, not a chat model: the adapter sends a
`choice` question whose criteria are the ASCII keys `c1` … `c12` plus `unclear`.
The model returns one key and a probability for each criterion; the AI service
maps the key back to the display value. It cannot emit free text or a name that
was not already in the request. When image mode is on, a second `noul` question
asks whether the tight crop is legible. That readability answer is recorded but
never used to accept a name.

The provider sees:

- the machine reading and up to five reader alternatives;
- up to twenty names already accepted or corrected in the same source file;
- the candidate values, their source labels (`reader`, `ai`, `file`, `common`),
  and their ranks (1–12);
- crop measurements: tight width/height, source width/height, and name height;
- a boolean saying whether image mode is attached, inside the bounded state;
- only when `NAME_JUDGE_IMAGES=true`, one tight name crop encoded as PNG.

The provider never receives:

- a source id, source file id, family id, account id, tree id, or reviewer id;
- the reviewer's identity, role, session, or free-text note;
- the page preview or context crop;
- a candidate name that was not already in the bounded request;
- application API keys. Local Ollama inference needs no key. If
  `LIQUID_API_KEY` is set for a protected proxy, the provider authorization
  header carries only that key; no other key is sent.

The request models use `extra="forbid"`. A caller cannot add a source id or an
unexpected image field and have it forwarded. The core request is built from the
source file, not from browser metadata, and the browser sends only candidate
values.

`LIQUID_BASE_URL` is the **exact** System One endpoint. The adapter does not
guess a path or append `/v1/systemone`. `JEV_BASE_URL` remains the exact
OpenAI-compatible chat-completions endpoint when the optional hosted provider is
selected with `NAME_JUDGE_PROVIDER=jev`.

## Modes

- **off** — no judge request is made. The review bench, source crops, manual
  transcription, AI spelling candidates, and decisions work normally.
- **shadow** — a deterministic sample of suggestions is judged. Nothing is
  displayed and no chip changes order. A valid result is stored beside the
  decision with the bounded candidate set frozen from the initial machine
  reading and prior file context, before the reviewer enters a correction. The
  visible spelling chips may still respond to editing; later chips do not enter
  that suggestion's shadow snapshot. A
  provider failure or malformed output changes nothing visible and never blocks
  a decision.
- **active** — only a separately labelled `رأي النموذج` row may appear below the
  normal candidates. It never reorders the chips, never changes the `1`–`8`
  keyboard mapping, and never prefills or accepts the editor. The row hides when
  the draft changes or the result is stale. It shows no percentage.
  `unclear` focuses the editor. A missing local model hides the row or shows a
  quiet Arabic fallback; the decision still saves. Active mode is enabled in
  `.env.example` by the operator's decision; it is not evidence that the model
  is better than the deterministic ranker.

The judge output is not a suggestion. The decision ledger stores it in
`judge_output` with the mode, provider, pinned model/prompt, probabilities,
confidence, abstention, shown/used state, and latency. Unknown fields, an
unpinned model, a probability outside `[0,1]`, a choice outside the snapshot, or
a shadow result marked as shown are rejected.

## Image mode

`NAME_JUDGE_IMAGES=false` is the default even with the local provider. When it
is true, the core renders and sends only the tight crop the reviewer sees. If
rendering fails, the request falls back to text-only rather than making the
judge a hard dependency. Context crops and page images are not sent. Image mode
remains a separate experiment: do not turn it on for a real sample until the
text-only evaluation and a separate image decision have passed. Local inference
keeps the crop on this machine, but the crop is still review data and is not
sent by default.

## Pre-registered shadow evaluation

The rule below must be recorded before looking at the shadow result. The
`name-eval` command implements the measurements; it does not decide adoption.

### Sample

- At least **300 decided suggestions with candidate snapshots and judge
  outputs**.
- At least three pre-registered source classes: `clean_print`,
  `calligraphic_dense`, and `low_resolution_screenshot`.
- Source classes are assigned by a person in a JSON map keyed by source file id:

  ```json
  {
    "00000000-0000-0000-0000-000000000001": "clean_print",
    "00000000-0000-0000-0000-000000000002": "calligraphic_dense",
    "00000000-0000-0000-0000-000000000003": "low_resolution_screenshot"
  }
  ```

  Run `make name-eval NAME_EVAL_ARGS="--class-map path/to/source-classes.json"`.
  Without the map the gate reports `ready=false`; automatic MIME/resolution
  groups are not the pre-registered classes. `ready=true` only means the
  sample-size and class-coverage gate is met; it is not the go/no-go result, which
  still has to be read against the rule below.
  Only unseen, unused shadow outputs for the registered classes count toward
  this gate; active or unclassified decisions do not.

### Measurements

`make name-eval` reports, without printing candidate or file names:

- candidate recall@8, any-shown recall, no-candidate and all-wrong rates;
- deterministic top-1/top-3 and MRR;
- judge answered top-1/top-3, abstention overall, abstention when the truth is
  present in the pool, and abstention when it is absent;
- end-to-end judge top-1/top-3 on candidate-present decisions, treating
  abstention as not correct, against the deterministic baseline on the same
  rows;
- paired bootstrap 95% interval for the judge-minus-baseline top-1 difference;
- wrong answered top-1 for judge and baseline, with the count increase;
- the active-row use rate (`used` / `shown`), for the product measurement;
- calibration buckets for judge confidence;
- candidate-source coverage for `reader`, `ai`, `file`, and `common`, including
  judge top-1 versus deterministic top-1 on the decisions where that source
  supplied the truth (the file-specific/common-name check);
- breakdowns by file class, resolution, page, reader confidence, and date;
- rates only when the denominator is at least 100; otherwise the report prints
  counts and says no rate. The UI shows no percentage at all before calibration.

### Go/no-go rule

Use judge output to change ranking, show a percentage, or make a product claim
only if all of the following hold. The separate active row is already visible by
operator decision; that display does not satisfy this rule and does not make the
judge an adopted ranker.

1. The candidate pool is good enough for a judge to have a fair chance:
   measurable candidate recall@8 is at least **70%** (the plan's starting floor).
   `name-eval` blocks its sample gate below that floor.
2. On candidate-present decisions, judge top-1 improves by at least **10
   percentage points** over the deterministic baseline, and the 95% paired
   bootstrap interval **excludes no improvement** (`lower > 0`).
3. The judge abstains more often when the human correction is absent from the
   candidate pool than when it is present.
4. Wrong answered top-1 does not increase.
5. The improvement is not only on common names; inspect the source- and
   file-specific subsets.
6. Reviewer time is not worse. `name-eval` reports the median time and splits,
   but it cannot infer an A/B effect from shadow data alone; a baseline period or
   an explicit A/B by file/session is required.
7. The privacy gate is approved for the exact data being sent, including whether
   any private names or crops may leave the platform.

If the rule fails, keep the deterministic ranker and record the no-result. Do
not let the judge change ordering, show a percentage, or support an accuracy
claim because it is interesting. The separate active row may remain visible as
an opinion, but it must not be described as better.

## What is implemented where

- `services/ai-research/app/name_judge.py` — bounded provider adapter, pinned
  prompt/model checks, strict output validation, TTL cache, and
  `POST /v1/names/judge`.
- `services/core-api/internal/sourceprocessing/name_judge.go` — core interface,
  settings, file context, crop measurement/render, HTTP client, and response
  validation.
- `services/core-api/internal/httpapi/source_processing_handler.go` — config
  and judge routes; `503` means no judge, not a failed review; `422` means the
  candidate set or judge output was rejected.
- `apps/web/src/components/SourceSuggestionReview.tsx` — config fetch,
  deterministic shadow sampling, debounced judge request, separate active row,
  stale-result hiding, and judge evidence construction.
- `services/core-api/cmd/name-eval/main.go` — read-only evaluation report and
  shadow gate.

The AI service and core tests use a fake HTTP transport. No test calls a live
provider. The local environment has the approved `d1:3b` model and no private
evaluation sample; a manual smoke request may call the local endpoint, but no
test or report may claim quality from it.
