# Plan 022: Make name candidates measurable, and use the local judge where it earns its place

> **Executor instructions**: Liquid `d1:3b` is a **judge over a bounded candidate set**, never a
> reader, a normalizer, or an authority. Build the deterministic candidate pipeline and the
> decision snapshot first. The separate active row may show the judge's bounded choice, but it
> must not change candidate order, prefill the editor, or accept a name. Shadow mode remains the
> measurement path; no confidence threshold may accept a name, an identity, or a relationship.
> Leave the work uncommitted.

## Status

- **Priority**: P2 — quality improvement on top of the review feature, not a replacement for it
- **Effort**: L
- **Risk**: MEDIUM–HIGH — automation bias, a local model's unmeasured quality, a noisy label
  source, and the temptation to claim accuracy from a small sample
- **Depends on**: plan 019 (readings exist), plan 021 (the review bench and decision ledger)
- **Category**: AI quality / review assistance
- **Planned at**: 2026-10-24
- **Status**: IMPLEMENTED (code complete, measurement pending) — the deterministic pipeline,
  decision evidence, expanded replay report, bounded local Liquid adapter/endpoints/flags, shadow
  sampling, and separate active row are implemented (uncommitted) and tested with fake
  transports, plus one manual local endpoint smoke test. Local inference is approved and
  `.env.example` enables active mode. The remaining gate is the ≥300-decision shadow sample
  across three pre-registered source classes and the go/no-go result. No model quality, ordering,
  percentage, or lift claim is made.

**Shadow collection preparation, 2026-10-27.** The reviewer confirmed the source-class
assignments before review. The private class map was frozen at 14:33:24 Asia/Riyadh; its SHA-256
is `861c26da1a33473e54e204c153ee3f8ccad61c7e656b519faeb3fd9a57a677f2`. It assigns two files to
`clean_print`, four to `calligraphic_dense`, and two genuine screenshots to
`low_resolution_screenshot`; the remaining files are unclassified. File identifiers, chart
contents, and the map remain outside this repository.

The eleven private charts have a review queue of 6,138 detected regions; 1,925 have local
Tesseract readings. In the three registered classes the queue contains, respectively, 2,382,
2,109, and 432 regions, with 1,190, 546, and 67 local readings. These are machine hypotheses,
not gold labels, and some regions may not contain readable names. Hosted chart vision is
disabled for this run. The app uses local Liquid in `shadow` mode with sampling set to 1 and
images disabled. A crop-and-judge smoke test wrote no decision.

The pre-review `name-eval` run found **0 decisions**, **0 judged decisions with truth**, and
`ready=false`. This is **no result**, not a failed or passed quality evaluation. The human
review batch and the comparison remain outstanding; reviewer-time non-inferiority also needs a
baseline or A/B period. The shadow snapshot is now frozen from the initial reading and prior
file context before the reviewer types, so a correction cannot become judge input for its own
evaluation. Accepted readings are recorded as decided text in the ledger. The sample gate also
excludes active, displayed, used, and unregistered-class decisions.

**Pilot report, 2026-10-27 14:43 Asia/Riyadh.** The requested JSON report has nine real
decisions: eight accepted or corrected names with snapshots and unseen shadow outputs, plus one
rejection without a truth label. The eight qualifying decisions are all in
`calligraphic_dense`; `clean_print` and `low_resolution_screenshot` have none. The gate remains
`ready=false` and the go/no-go outcome is **no result**: the 300-name and class-coverage gates
are unmet. The candidate and judge rates are not measurable at this sample size. Keep the
deterministic ranking and collect the remaining human reviews before evaluating quality.

**Follow-up after 111 review actions, 2026-10-27.** The reviewer continued to nine
acceptances, 71 corrections, 14 deferrals, and 17 rejections. There are 80 human
truth labels and 79 qualifying shadow labels; one accepted reading lacks judge
output and remains a measurement gap. All qualifying labels are still in
`calligraphic_dense`, so the sample and class gates remain unmet. The initial
top-eight pool contains the human answer in 18 of the 80 truth-labelled cases;
these are counts below the rate floor, not a quality percentage or adoption claim.

The user reported the unavailable image-reader action and selected a staged
plan for the full local pipeline. Plans [023](023-local-reading-capabilities-and-cohorts.md),
[024](024-durable-local-name-reading.md), [025](025-name-context-and-shadow-preparation.md),
and [026](026-name-review-repair-and-verification.md) preserve this baseline while
adding explicit capabilities, durable local processing, stronger candidates, and
review repair tools. The missing old judge result must not be backfilled using
post-decision context. The outcome remains **no result** under the original rule.

**Implementation status, 2026-10-26.** The measurement foundation and the bounded judge path are
in place; the evaluation is not.
`apps/web/src/lib/arabicNames.ts` derives display/canonical/compact/particle/skeleton keys, ranks
a bounded candidate pool with source/rank/file evidence, and abstains when nothing is plausible.
`apps/web/src/lib/nameReviewEvidence.ts` builds the at-most-12-entry snapshot, including the
separately shown AI candidates and the display value a selected candidate must match. Migration
`0053_source_tree_suggestion_decision_context.sql` adds the snapshot, selected candidate, input
method, judge-output slot, versions, and duration to the append-only ledger; the server validates
them and writes them in the decision transaction. A rejected snapshot retries the decision once
without evidence and is reported as a measurement gap.

`services/ai-research/app/name_judge.py` implements the provider side: a 1–12-option judge,
ASCII option keys mapped back to display values, pinned model/prompt, strict probabilities and
confidence, `unclear` abstention, an optional tight-crop image, a bounded TTL cache, and
`POST /v1/names/judge`. `internal/sourceprocessing/name_judge.go` and the HTTP routes implement
the core side: config, candidate validation, file context, crop measurement, the exact tight
crop, response validation, and `503`/`422` mapping. The web bench fetches the config, samples
shadow mode deterministically, debounces one request per stable candidate set, keeps shadow
invisible, and renders only a separate active row. A missing provider never blocks a decision.

`cmd/name-eval` and `make name-eval` replay the ledger and report recall@8 (plus recall among any
shown candidate), no-candidate cases, all-wrong candidate sets, top-1/top-3/MRR, typed rate,
median decision time, missing snapshots, stale normalizers, candidate-source coverage,
file-class/resolution/page/confidence/date breakdowns, judge abstention split by truth presence,
judge versus deterministic top-1/top-3, a paired bootstrap interval, calibration buckets, and a
300-decision class-map gate. It prints no candidate or file names.

What is **not** established: no shadow run has produced 300 decisions; the pilot has only eight
truth-labelled decisions in one class. The code default is `NAME_JUDGE_MODE=off`; `.env.example` enables the
separate active row for local Liquid with `NAME_JUDGE_IMAGES=false`. That row is an opinion, not
an adopted ranker. The privacy decision, data sent, pre-registered go/no-go rule, class-map
format, and known limitations are in
[`docs/name-judge-privacy-and-evaluation.md`](../docs/name-judge-privacy-and-evaluation.md).

## In plain language

The review bench already puts the source crop beside the machine's reading and an Arabic
editor. It also offers spelling candidates: the reader's alternatives, an on-demand vision
model's list, names already accepted in this file, and a small common-name lexicon. Those
candidates are ranked today by a simple string-distance rule in the browser, and the decision
ledger records only the machine reading and the reviewer's final text.

That is enough to review a chart. It is not enough to know whether the ranking helped, whether a
model could rank better, or whether a suggestion should have been sent straight to manual
entry.

This plan does three things, in this order:

1. **Make the candidate set measurable.** Persist what the reviewer actually saw, which
   candidate they chose, and how long it took.
2. **Build the deterministic pipeline.** Normalize Arabic names with several matching keys,
   generate a better candidate pool, and rank it with evidence — not only edit distance.
3. **Add Liquid as an optional judge.** It may choose among the candidates, or say “unclear.” It
   runs in shadow mode first: it makes a recommendation, we record it, but the reviewer still
   sees the existing order. Only after enough decisions does it get a visible row.

The benefit is fewer keystrokes, more one-click acceptances, and an honest “no candidate is
good enough” path. It is not a claim that the machine can now read the calligraphic chart.

## Why this matters

There are two different ceilings, and they must not be confused:

- **Candidate recall** — is the correct name even in the list? If the true name is `جاسم` and
  the list contains only `علي`, `عبد`, `عيد`, `عبدالله`, and `عبيد`, no judge can select
  `جاسم`. The only correct behavior is to abstain and send the reviewer to manual entry.
- **Candidate ranking** — given that the correct name is in the list, is it near the top?

Liquid can improve the second. It cannot improve the first. A pipeline that spends its effort on
Liquid before measuring recall is optimizing the easier half of the problem.

There is also a difference between “closer to the machine's reading” and “closer to the ink.”
Text-only Liquid sees the former, not the latter. If the reader misread the name, ranking by
similarity to that misreading optimizes the wrong target. The pipeline therefore needs evidence
beyond string distance: reader agreement, source reliability, file context, crop quality, and —
only behind an experimental flag — the crop itself.

## Current state at planning time (superseded by the status above)

- `apps/web/src/lib/arabicNames.ts` normalizes with one aggressive key and ranks candidates by
  exact/prefix/contains/Levenshtein score, with a fixed source tie-break (`reader` → `ai` →
  `file` → `common`).
- `SourceSuggestionReview.tsx` generates at most eight candidates in the browser. The on-demand
  AI candidates live only in component state; they are deliberately not persisted.
- `source_tree_suggestion_decisions` stores the machine reading, the reviewer's text, the
  decision, the reviewer and the note. It does **not** store the candidate set, the shown order,
  the clicked candidate, the input method, the judge output, or the time to decision.
- `SuggestionAccuracy` measures whether the reader was accepted unchanged. It does not measure
  candidate recall or ranking quality.
- The source crop endpoint already gives the reviewer the tight and context crops from the
  original at source resolution. The same tight crop is sent to the candidate vision model.
- The AI service has one chart provider (the DeepSeek vision adapter). At planning time there
  was no judge client, name-judge endpoint, or judge flag; the bounded local Liquid path now
  exists.
- `apps/web/src/lib/arabicNames.test.ts` covers a handful of normalization and ranking cases; it
  does not measure candidate recall on real decisions.

The existing 8-candidate limit is right. Liquid supports many more options, but the reviewer's
attention is the scarce resource. The judge must work over a short list.

## Scope

**In scope**:

- `apps/web/src/lib/arabicNames.ts` and its tests — multi-key normalization, candidate pool,
  deterministic ranking features, snapshot construction
- `apps/web/src/components/SourceSuggestionReview.tsx` — candidate snapshot, input-method
  tracking, shadow call, active judge row
- `apps/web/src/lib/api.ts`, `apps/web/src/types.ts` — decision snapshot and judge contracts
- `services/core-api/internal/sourceprocessing/` — decision snapshot validation, judge endpoint,
  crop reuse, feature-flag configuration
- `services/core-api/internal/httpapi/` — routes and error mapping
- `services/ai-research/app/` — local Liquid client and `/v1/names/judge`
- `db/migrations/` — additive columns on the decision ledger
- `tools/` or `services/ai-research/evaluation/` — offline candidate-quality report
- `docs/import-reading-strategy.md`, `docs/ocr-and-import-decision.md`,
  `plans/README.md` — status and the boundary of the claim

**Out of scope**:

- **Automatic acceptance.** The judge never changes a suggestion's status and never writes a
  person.
- **OCR or transcription improvements.** That is plan 019. Liquid cannot recover missing ink.
- **Automatic relationship inference.** The reviewer still draws and confirms every edge; the
  second-reviewer rule is unchanged.
- **Identity decisions.** Plan 021 owns the explicit “existing person / new person / not sure”
  choice. A later plan may reuse this judge to order identity candidates; it must not merge.
- **Liquid image mode in production.** It remains behind an experimental flag until measured.
- **Fine-tuning or training a model.** Liquid `d1:3b` runs locally as a zero-shot decision model;
  a ranker trained on the decision ledger remains the long-term comparison. The hosted Jev
  provider stays optional and would need a separate privacy decision.
- **A new review surface.** This is an addition to the existing bench.
- **Rewriting the candidate order while the reviewer types.** The list must not jump under the
  keyboard.

## Design

### 1. The candidate pipeline

```
suggestion + tight crop
  → display value + matching keys
  → candidate pool (reader, AI, file, common, bounded visual variants)
  → evidence per candidate
  → deterministic ranker
  → optional Liquid judge
  → policy: show, abstain, or manual
  → decision snapshot
```

The deterministic layer must work with Liquid switched off. It is the fallback, the baseline, and
the thing that keeps the bench usable when the provider is unavailable.

**Matching keys**, all derived without changing the display value:

- `display` — exactly what the reviewer sees and may accept.
- `canonical` — NFKC, diacritics/tatweel removed, أ/إ/آ/ٱ → ا, ى → ي, ة → ه, whitespace
  collapsed.
- `compact` — `canonical` with spaces and permitted joiners removed, so `عبد الله` and
  `عبدالله` meet.
- `particle` — a token-aware variant with and without `ال`, `آل`, `بن`, `بنت`, `أبو`; never
  used to rewrite the display.
- `skeleton` — a dotless visual-shape key, used as a weak feature only, never as the sole
  equality key.

`آل` is not `ال`. `عبد الله` is not necessarily `عبدالله` when displayed, even if they match for
search. Keep the display spelling and the file's convention.

**Evidence per candidate**:

- best matching key and which one;
- edit distance, character n-gram similarity, prefix/token-prefix/containment;
- reader rank and reader view agreement;
- AI candidate rank, if the AI list was fetched;
- whether the candidate is already accepted in this file, and how often;
- whether it is a common name or a rare family name;
- crop quality: name height in source pixels, source resolution, sharpness/contrast if available;
- reader confidence and alternative count;
- source reliability, learned from decisions when there is enough data.

**Candidate generation** must improve before ranking is judged. The pool may include bounded
visual variants only when the result is in the lexicon or the file, so the list does not fill
with invented strings. The current `جاسم` probe is the recall failure to keep in the fixture:
the right behavior is “no candidate,” not a confident wrong choice.

### 2. What Liquid is allowed to do

One judge call may answer up to two typed questions:

1. **`best`** — a `Choice` over the supplied candidates plus `unclear`. The options use ASCII
   keys (`c1` … `c12`) with the Arabic name and its evidence in the descriptions; the service
   maps the key back to the display value. This avoids option-order and escaping surprises and
   ensures Liquid cannot return a name outside the list.
2. **`readable`** — a `Noul` (“is the supplied crop clear enough to choose?”) only when image
   mode is enabled. Without the image, a readability answer would be a guess from metadata, so
   do not ask it.

The request state contains only what the decision needs:

- the machine reading and its alternatives;
- the candidate values, sources, ranks, and the evidence above;
- the file's accepted names, bounded to a short context window;
- crop dimensions/quality;
- the tight crop, only when image mode is enabled.

It contains no source ID, account ID, family ID, page image, or reviewer identity. Name strings
are data, not instructions.

The response is constrained to:

- `choice` — one of `c1` … `cN` or `unclear`;
- `probabilities` — one per option;
- `confidence` — recorded, not displayed as an accuracy claim;
- `readable` — optional, only in image mode.

Invalid, missing, duplicate, or out-of-range options are rejected by the AI service. The core
API rejects a judge response whose choice is not in the candidate set. There is no free-text
output from Liquid.

### 3. Shadow mode first

The judge must not influence the reviewer until it has been measured without influence.

- `off` — no Liquid calls.
- `shadow` — the web app calls the judge for a sampled subset of suggestions, stores the judge
  output in the decision snapshot, and does **not** display or reorder it.
- `active` — the judge result appears as a separate “رأي النموذج” row. It still does not
  reorder the normal chips, and it never fills the editor.
- A later, separate flag may allow reordering, but only after active measurement shows the
  separate row helps. Reordering is not part of this plan's first delivery.

Shadow mode gives us unbiased labels: the reviewer's decision was made without seeing the
judge's choice. That is the only honest way to compare the judge with the deterministic
baseline.

If the provider is unavailable, the shadow call is dropped; the bench behaves exactly as it does
today. In active mode a 503 hides the row and shows a small Arabic message; it never blocks the
decision.

### 4. Calibration and display

Liquid's `confidence` is the concentration of its option distribution, not the probability that it
is correct. It is stored, calibrated offline against reviewer decisions, and only then used for
routing. Until calibration exists:

- do not show a percentage;
- do not call a high-confidence choice “correct”;
- do not use a threshold to auto-accept;
- use a binary “مرشح واضح” / “لا مرشح واضح” only if the shadow data supports the threshold.

A judge that never abstains is not useful. A judge that abstains on every hard case is also not
useful. Both are measured.

## Steps

### Step 0: Settle policy, privacy, and the feature flag

- The operator approved local Liquid inference for this checkout. The state contains personal
  names, so it stays on this machine by default; a hosted provider remains a separate privacy
  decision. Record the decision before enabling the row.
- Decide the narrow exception to “AI candidates are never persisted”: the candidate snapshot is
  **review evidence**, not a suggestion and not a person. Without it, AI-candidate ranking cannot
  be measured. If the exception is refused, limit the plan to reader/file/common candidates and
  drop the AI-candidate claims.
- Add feature flags. The code remains fail-closed when the environment is absent:
  - `NAME_JUDGE_PROVIDER=liquid|jev` (local Liquid is the default)
  - `NAME_JUDGE_MODE=off|shadow|active`
  - `NAME_JUDGE_SAMPLE=0.2` (shadow only)
  - `NAME_JUDGE_IMAGES=false` (experimental; default off)
  - `LIQUID_BASE_URL`, `LIQUID_MODEL`, `LIQUID_PROMPT_VERSION`, optional `LIQUID_API_KEY`
  - `JEV_API_KEY`, `JEV_BASE_URL`, `JEV_MODEL`, `JEV_PROMPT_VERSION` for the optional host
- Keep secrets out of the browser and out of logs. Prefer environment configuration; a Keychain
  entry may follow the DeepSeek precedent if a hosted provider is used.
- Update `.env.example` and the README when the client is implemented. `.env.example` sets the
  local provider and active mode; `NAME_JUDGE_IMAGES` stays false.

### Step 1: Multi-key normalization and its tests

- Add `arabicNameKeys(value)` returning display/canonical/compact/particle/skeleton.
- Keep `normalizeArabicName` as the canonical key for compatibility.
- Add fixtures for:
  - `آل الصباح` / `ال الصباح` / `الصباح`;
  - `عبد الله` / `عبدالله`;
  - `على` / `علي`;
  - `حصة` / `حصه`;
  - `محمّد` / `محمد`;
  - `أحمد` / `محمد` / `محمود`;
  - `جاسم` / `حاسم`;
  - diacritics, tatweel, zero-width characters, Arabic-Indic digits.
- Assert that display values are never rewritten and that no two distinct names are merged by
  the canonical key alone.
- Version the normalizer (`normalizerVersion`) so a snapshot can be reproduced.

### Step 2: Candidate pool and deterministic ranking

- Extend `suggestArabicNames` to build a bounded pool from all sources, keeping source and rank.
- De-duplicate by the best matching key, not by raw display, but show the file's display
  spelling when available.
- Add evidence-aware scoring to the deterministic ranker. Do not let the common-name lexicon
  dominate a rare family name; a file name beats a common name on an exact match.
- Keep at most eight visible candidates. The pool may be larger internally, but the reviewer's
  list must stay small.
- Add a “no candidate” outcome when the best match is too weak. The editor remains focused and
  empty; the reviewer types.
- Add unit tests for the `أحمد` and `جاسم` probe shapes. The first should keep the truth in the
  list; the second should not pretend it has one.

### Step 3: Candidate snapshot in the decision ledger

Add an additive migration, `0053_source_tree_suggestion_decision_context.sql`, extending
`source_tree_suggestion_decisions`:

```sql
ALTER TABLE source_tree_suggestion_decisions
  ADD COLUMN candidate_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb
    CHECK (jsonb_typeof(candidate_snapshot) = 'array'),
  ADD COLUMN selected_candidate text,
  ADD COLUMN input_method text NOT NULL DEFAULT 'legacy'
    CHECK (input_method IN ('legacy', 'reader', 'candidate', 'typed', 'none')),
  ADD COLUMN judge_output jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(judge_output) = 'object'),
  ADD COLUMN normalizer_version text NOT NULL DEFAULT 'legacy',
  ADD COLUMN candidate_pool_version text NOT NULL DEFAULT 'legacy',
  ADD COLUMN duration_ms integer
    CHECK (duration_ms IS NULL OR duration_ms >= 0);
```

The snapshot is an array of at most 12 entries:

```json
[
  {
    "value": "أحمد",
    "key": "احمد",
    "sources": ["reader", "ai"],
    "ranks": {"reader": 2, "ai": 1},
    "score": 92,
    "fileCount": 3,
    "shown": true,
    "position": 1
  }
]
```

Rules:

- The server validates length, string bounds, source values, and positions.
- `selected_candidate` is null for a typed correction, a rejection, or a deferral; for a
  candidate or reader acceptance it must equal a `value` in the snapshot.
- `input_method` distinguishes what the reviewer did; the decision type still says what the
  result was.
- `judge_output` carries the mode, provider, model, prompt version, choice, probabilities,
  confidence, readable flag, latency, and whether the result was shown. It never carries an API
  key or source metadata.
- The snapshot is written in the same transaction as the decision and is append-only.
- Old clients can omit it; the defaults preserve compatibility.
- `make sqlc` and `make generated-check` after the migration.

This is the only way to answer “was the ranking good?” The current table cannot reconstruct what
the reviewer saw.

### Step 4: Replay harness and baseline

- Add a read-only report under `services/core-api/cmd/name-eval` (the implemented
  `make name-eval`). It reads decisions, not source files.
- Compute, with confidence intervals:
  - candidate recall@8: the decided text is in the snapshot;
  - top-1, top-3, MRR of the shown order;
  - typed rate and median time to decision;
  - abstention rate and the cases where every candidate is wrong;
  - candidate-source coverage: reader, AI, file, common;
  - results grouped by source/file class, page, resolution, reader confidence, and date.
- Do not recompute the past with a new normalizer. Use the snapshot's keys. A replay with newer
  context is not the same task and will overstate the result.
- Split evaluation by source file and time, never by random crop. A reviewer's earlier decisions
  are in the same file and leak into later ones.
- The report is internal. It does not expose names, and it does not produce a public accuracy
  claim.

**Gate before Liquid:** if candidate recall@8 on readable names is too low to give the judge a
fair chance, stop and improve candidate generation. The threshold is set from the first
baseline and recorded before the Liquid shadow run; a reasonable starting target is ≥70%, with
90% as the direction of travel. A judge cannot fix a candidate generator that misses the name.

### Step 5: Liquid adapter in the AI service

- Add `services/ai-research/app/name_judge.py` with:
  - Pydantic request/response models with `extra="forbid"`;
  - a `NameJudge` protocol;
  - a `LiquidNameJudge` HTTP adapter for Ollama's `/v1/systemone`, plus the optional
    `JevNameJudge` chat-completions adapter, both using the existing `httpx` dependency;
  - a fake transport for tests.
- Add `POST /v1/names/judge` to the AI service. Extend the existing request-body bound middleware
  from `/v1/vision/chart/` to `/v1/names/`.
- The endpoint accepts a bounded state, candidates, mode, and optional image. It returns only a
  choice, probabilities, confidence, optional readability, model, and prompt version.
- Pin the Liquid model and prompt version. Do not use `latest`.
- Send a `choice` question whose criteria are the ASCII option keys `c1` … `c12` plus `unclear`,
  and map the answer back to names in the AI service. The core only sees the mapped result.
  With an image, add one `noul` readability question; it cannot accept a name.
- Validate:
  - 1–12 options;
  - unique keys;
  - every candidate value is a bounded Arabic string;
  - choice is one of the options or `unclear`;
  - probabilities cover exactly the options;
  - confidence is in `[0, 1]`;
  - no output can introduce a name outside the options.
- On a missing endpoint/model or unavailable local server, return 503 with a setup or
  unavailable message. Local inference needs no credential; `LIQUID_API_KEY` is optional for a
  protected proxy. On timeout, 503. On malformed provider output, 422. Never return a fabricated
  choice.
- Image mode sends only the tight crop as raw base64, and only when `NAME_JUDGE_IMAGES=true`. The
  image feature is experimental; a failure must fall back to text-only or to no judge, not to an
  unvalidated guess.
- Add a small in-process cache keyed by the state hash, option set hash, model, prompt version,
  and optional image hash. Cache TTL and size are bounded. No cross-user persistent cache.

Tests:

- the fake transport receives the exact state and options;
- a choice outside the options is rejected;
- duplicate or missing probabilities are rejected;
- a timeout and a 5xx become provider-unavailable, not a result;
- no API key, source ID, or family ID appears in a log or response;
- image mode is off by default and is not sent when disabled;
- the same crop + state is cached without leaking across different candidate sets.

### Step 6: Core judge endpoint

- Add a `NameJudge` interface in `internal/sourceprocessing`, implemented by an HTTP client to
  the AI service. Keep it separate from `ChartVisionProvider`; a judge is not a reader.
- Add `POST /api/v1/source-tree-suggestions/{id}/judge`:
  - requires review permission on the suggestion's source;
  - accepts at most 12 candidates, each with value/source/rank;
  - loads the suggestion's reading and alternatives;
  - renders the tight crop from the original only when image mode is enabled;
  - calls the AI service;
  - validates the choice against the supplied candidates;
  - returns the judgment and metadata with `Cache-Control: private, no-store`;
  - writes nothing.
- Add a read-only `GET /api/v1/name-judge/config` returning
  `{mode, imageEnabled, available}`. It contains no secrets and does not reveal provider
  credentials.
- The decision endpoint accepts the snapshot and judge output from the web, validates them, and
  stores them with the decision in the same transaction.
- Map errors: 403 permission, 404 suggestion, 422 malformed candidates, 503 judge unavailable.
  The UI must treat 503 as “no judge,” not as a failed review.

### Step 7: Web shadow mode and input tracking

- Fetch the judge config with the queue. If `off`, do nothing.
- In `shadow` mode, call the judge once when a suggestion becomes active, for the sampled
  subset. Do not reorder, label, or focus anything. If the draft or candidate set changes,
  invalidate the result.
- Build the candidate snapshot from every candidate actually shown: the ranked chips and the
  separate AI row, with source, rank, score, position, and `shown`.
- Track:
  - `inputMethod` (`reader`, `candidate`, `typed`, `none`);
  - `selectedCandidate` for chip/reader input;
  - `startedAt` when the suggestion becomes active;
  - `durationMs` when the decision is submitted.
- Send the snapshot, selected candidate, input method, duration, and judge output with the
  decision. A failed snapshot must not prevent the decision; it is evidence, not the decision
  itself. If the server rejects a malformed snapshot, retry once without it, record the decision
  as `legacy`/empty evidence, and log the defect. The quality report counts missing snapshots
  separately; they are a measurement gap, not a failed review.
- Do not call the judge on every keystroke. Once per suggestion/candidate set is enough.
- Keep the normal chip order stable. A shadow result is invisible; an active result is a
  separate row.

### Step 8: Shadow evaluation and go/no-go

Run shadow mode on internal review sessions until there are at least 300 decided suggestions
with snapshots, across at least three source classes (clean print, calligraphic/dense, and
low-resolution screenshot). Pre-register the decision rule before looking at the result.

Compare:

- deterministic top-1 / top-3;
- Liquid top-1 / top-3;
- Liquid abstention on cases where the human correction is absent from the pool;
- Liquid abstention on cases where the truth is present;
- typed rate and time to decision;
- calibration of Liquid confidence;
- performance on rare names and file-specific names, not only common names.

A reasonable go/no-go rule:

- Liquid top-1 improves by at least 10 percentage points over the deterministic baseline on
  candidate-present decisions, and the 95% bootstrap interval excludes no improvement;
- Liquid abstains more often when the truth is absent than when it is present;
- no increase in wrong top-1 recommendations;
- the improvement is not only on common names;
- reviewer time is not worse.

If the rule fails, keep the deterministic ranker, keep shadow logging off, and record the
result. Liquid is not adopted because it is interesting; it is adopted only if it helps.

### Step 9: Active recommendation row

The separate row is enabled for local Liquid by the operator's decision in
`.env.example`. It is a display of an opinion, not an adopted ranker and not a
quality claim. Reordering, percentages, or automatic behavior still require the
shadow evaluation and a passing go/no-go result.

- Add a “رأي النموذج” row below the normal candidates. It shows the judge's candidate with a
  short evidence line, or “لا مرشح واضح” when it abstains.
- Do not reorder the existing chips. Do not prefill the editor. Do not change the `1`–`8`
  keyboard mapping. The judge row may have a separate key, but it is a choice, not an accept.
- If the judge result is stale (the draft changed), hide it.
- If the judge is unavailable, hide the row and keep the bench working.
- Show no percentage. If calibration supports a band, use “مرشح واضح” / “مرشح ضعيف” and a
  tooltip explaining the evidence.
- Log whether the reviewer used the judge row. That is the product measurement.
- Optionally run an A/B by file/session: baseline vs active row. Measure median time and
  correction rate; do not compare only click-through, because the top row attracts clicks.

### Step 10: Optional image mode, then identity follow-on

Image mode is a separate experiment, not part of the first activation. The flag and tight-crop
adapter are implemented, but `NAME_JUDGE_IMAGES=false` is the default and no image sample has
run:

- enable `NAME_JUDGE_IMAGES=true` only for an internal sample;
- compare text-only vs text+image on the same candidate sets;
- if it does not improve top-1 or abstention, disable it;
- if it does, keep it behind the flag until the provider's experimental image support is stable
  and the privacy review is complete.

After plan 021's identity choices exist, a follow-on may use a similarly shaped
`IdentityJudge` to order existing-person candidates using name, aliases, dates, places, and
kinship context. It must still offer “existing / new / not sure,” and it must not merge. That is
not part of this plan's acceptance.

## API contracts

### Decision snapshot

The decision endpoint keeps its current shape and adds optional evidence. The server still
derives `suggested_text_ar` and the reviewer; neither is accepted from the client.

```json
{
  "decision": "corrected",
  "textAr": "أحمد",
  "noteAr": "",
  "candidateSnapshot": [
    {
      "value": "أحمد",
      "key": "احمد",
      "sources": ["reader"],
      "ranks": {"reader": 2},
      "score": 92,
      "fileCount": 0,
      "shown": true,
      "position": 1
    }
  ],
  "selectedCandidate": "أحمد",
  "inputMethod": "candidate",
  "judgeOutput": {
    "mode": "shadow",
    "provider": "liquid",
    "model": "d1:3b",
    "promptVersion": "name-judge-liquid-v1",
    "choice": "أحمد",
    "confidence": 0.71,
    "abstained": false,
    "shown": false,
    "latencyMs": 240
  },
  "normalizerVersion": "ar-name-keys-v1",
  "candidatePoolVersion": "ar-name-pool-v1",
  "durationMs": 1840
}
```

Validation rules:

- at most 12 snapshot entries; each value ≤200 runes; keys bounded;
- unique `value` and unique `position`;
- `shown` entries have positions; hidden pool entries do not;
- `selectedCandidate` is null or matches a snapshot value;
- `inputMethod` is one of the enum values and is consistent with the selected candidate;
- `judgeOutput` is at most 16 KiB; its choice is `unclear` or a snapshot value;
- `durationMs` is bounded (for example, 0–3,600,000).

The snapshot is stored on the decision, not on `source_tree_suggestions`. It is evidence of
what the reviewer saw, not a new machine reading.

### Judge request

```json
{
  "candidates": [
    {"value": "أحمد", "source": "reader", "rank": 2},
    {"value": "محمد", "source": "reader", "rank": 1},
    {"value": "محمود", "source": "reader", "rank": 3}
  ]
}
```

The core adds the machine reading, alternatives, file context, crop dimensions, and optional
tight crop. No source or family identifiers are sent.

### Judge response

```json
{
  "choice": "أحمد",
  "abstained": false,
  "probabilities": {"أحمد": 0.6, "محمد": 0.3, "محمود": 0.1, "unclear": 0.0},
  "confidence": 0.6,
  "readable": null,
  "provider": "liquid",
  "model": "d1:3b",
  "promptVersion": "name-judge-liquid-v1",
  "cacheHit": false
}
```

A response with `choice: null` or `abstained: true` means “no candidate is supported.” It is a
valid, useful answer. The AI service maps the provider's ASCII option keys back to display
values before returning, so the probabilities are keyed by the names the reviewer saw and the
core never trusts a free-form option key.

## UI behaviour

- Normal candidate chips keep their existing keyboard numbers and their current order until a
  separate reorder flag is approved.
- The judge row is visually separate and labelled as an opinion, not as the answer.
- The source crop, the machine reading, and the reader's alternatives remain visible. The judge
  never replaces them.
- `unclear` focuses the editor and shows “لا مرشح واضح. اكتب الاسم من الصورة.”
- A judge 503 shows a quiet “تعذّر الحصول على رأي النموذج” and leaves the editor usable.
- No spinner blocks the decision. The judge is optional.
- The mobile layout keeps the row below the chips and above the save controls; it must not
  push the crop off screen.

## Test plan

- **Normalizer:** all keys, display preservation, diacritics, `آل`, `عبدالله`, `على/علي`,
  `حصة/حصه`, and the two probe names.
- **Candidate pool:** the truth is retained in the `أحمد` shape; the `جاسم` shape ends in a
  no-candidate outcome rather than a confident wrong list.
- **Snapshot validation:** bound, duplicates, positions, selected candidate, input method,
  judge output, oversized payload, old client defaults.
- **Decision transaction:** a snapshot is written with the decision or not at all; a failed
  snapshot does not create a decided suggestion without evidence.
- **Judge endpoint permission:** an unauthorized reviewer is refused; a suggestion from another
  source cannot be judged.
- **Exact crop:** the judge receives the same tight crop the reviewer sees, just as the existing
  candidate test pins for the vision model.
- **Judge validation:** choice outside the options, duplicate probabilities, missing
  probabilities, invalid confidence, free-text output, timeout, 5xx, and missing credentials.
- **Shadow mode:** no visible change and no reordering; the judge output is recorded; a 503 is
  invisible.
- **Active mode:** the row appears only when configured; it does not fill the editor; `1`–`8`
  still select the deterministic chips; `unclear` focuses the field.
- **No authority:** no test path may set `status='accepted'` from a judge result; no path writes
  a person, alias, or relationship from a judge result.
- **Offline report:** metrics on fixtures are deterministic; a sample below the floor refuses to
  print a rate; splits are by file/time.
- **E2E:** the workbench still works with the AI service stopped; the judge row is absent or
  shows its Arabic fallback; the decision still saves.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, no fixme/skip |
| AI tests | `cd services/ai-research && .venv/bin/pytest` | exit 0 |
| Generated | `make sqlc && make generated-check` | no drift |
| Candidate report | `make name-eval` | read-only report; no names printed |
| Docs | `make docs-check` | exit 0 |
| Diff | `git diff --check` | exit 0 |

If a target name differs in the Makefile at execution time, use the target that runs the same
gate and record the actual command.

## Acceptance

- [x] The candidate snapshot is written transactionally whenever it is supplied, and the
      existing decision contract still accepts old clients. Missing snapshots are counted and
      reported as a measurement gap, not silently treated as evidence.
- [ ] Candidate recall@8 is measured and reported before the judge is allowed to change ordering,
      show a percentage, or support any quality claim. The report now computes it; the decision
      pilot sample is below the measurement floor, so no rate can be reported. The separate active row is already visible by
      operator decision and does not change ordering.
- [x] The deterministic ranker works with the judge off, with the judge unavailable, and with
      the AI service stopped. The judge is default-off and every judge failure path leaves the
      decision untouched.
- [x] The judge runs only on a bounded candidate set, returns only a listed option or `unclear`,
      and cannot write a name, person, identity, or relationship. The adapter and core reject a
      choice outside the options, free text, unpinned metadata, and malformed probabilities; tests
      use a fake transport. One manual smoke request against local `d1:3b` returned a valid
      bounded choice and is not a quality measurement.
- [x] Shadow mode changes no visible order and no visible state. The request is deterministic per
      suggestion, the result is stored only as decision evidence, and the chips are untouched.
- [ ] The evaluation compares Liquid with the deterministic baseline on at least 300 shadow
      decisions, split by source file/time, with confidence intervals. The report implements the
      comparison, class-map gate, calibration, and bootstrap; the pilot sample is too small.
- [x] Active mode shows a separate row only; it does not reorder the normal chips or auto-fill
      the editor. The code default is off; `.env.example` enables it for local Liquid by the
      operator's decision. The row is still an opinion, and ordering/percentages remain blocked
      on the evaluation.
- [x] No percentage is shown unless the sample and calibration support it; the same 100-decision
      honesty rule that the reader accuracy surface uses applies here. The active row shows no
      percentage at all, and `name-eval` prints no rate below 100.
- [x] No source ID, family ID, account ID, reviewer ID, API key, or page image (outside the
      opt-in tight crop) is sent to the judge. Request models forbid extra fields, the core builds
      the request from the source, and tests assert the absence of identifiers. Local Liquid needs
      no key; if `LIQUID_API_KEY` is set for a protected proxy, that header carries only it.
- [x] The image mode is off by default and is not required for the feature to work. The core
      falls back to text-only if rendering fails, and the AI route rejects an image when the
      flag is off.
- [x] `make verify`, `make verify-full`, `make db-verify`, `make e2e`, `make generated-check`,
      `make docs-check`, and `git diff --check` pass. Re-run on 2026-10-27 with
      `COMPOSE_PROJECT_NAME=dawha`; the Python suite passed 78 tests, the web suite 77, and
      `make e2e` passed all 30 journeys. The final `make db-verify` counted 901 tests (895
      passed, 0 failed, 6 unrelated skips, 0 database skips). One earlier `db-verify` run failed
      transiently inside `internal/sourceprocessing` after 184s; the package and the full gate
      both passed on immediate rerun, and the rerun above was clean. The failing test name was
      not preserved by the truncated output, so this is recorded as a flake to watch, not as a
      code fix. The existing browser suite still does not open the workbench, so the active-row
      fallback and shadow invisibility are covered by the web helper tests and the API/service
      tests rather than a browser journey. The pilot `make name-eval` report remains below the
      sample and class gates, so this is a tooling gate, not an accuracy claim.
- [x] `docs/import-reading-strategy.md`, `docs/ocr-and-import-decision.md`, and
      `plans/README.md` state the implemented status without claiming more than the data shows.

## Done criteria

- [x] The candidate pipeline is deterministic, testable, and independent of Liquid.
- [x] The decision ledger can answer candidate recall, top-1, top-3, abstention, input method,
      and time to decision through `make name-eval`.
- [x] The judge is integrated behind a provider interface, feature flags, and a fake transport.
      The code posture is fail-closed when unset; `.env.example` enables the local active row, and
      no test makes a live provider call.
- [ ] Shadow evaluation is complete and its result is recorded, including a “no” result.
- [x] The judge is not adopted as a ranker, and its output cannot change candidate order or accept
      a name. The separate active row is enabled for local Liquid by the operator; ordering,
      percentages, and any quality claim remain blocked until the shadow evaluation passes and a
      person records the decision.
- [x] A later reader, lexicon, or prompt change is re-evaluated against the same snapshot
      protocol rather than declared an improvement. The snapshot carries the normalizer and
      pool versions, and `name-eval` refuses to reinterpret a snapshot from another normalizer.

## STOP conditions

- Stop if the candidate snapshot cannot be written transactionally with the decision. Without
  it there is no measurement, and a judge without measurement is a preference.
- Stop if candidate recall is too low for a judge to have a fair chance. Improve candidate
  generation first; do not ask Liquid to choose among names that do not include the truth.
- Stop if the provider returns names outside the supplied options, free text, or unvalidated
  probabilities. That is not a judge; it is an unconstrained generator.
- Stop if the privacy review does not allow the names or the optional crop to leave the
  platform. A local ranker is the fallback.
- Stop if the shadow evaluation fails the pre-registered rule. Do not activate a judge because
  it is new.
- Stop if the judge would need to be in the critical path for the review to work. The bench must
  remain usable with no provider, no network, and no AI service.
- Stop if the active UI would reorder the chips or prefill the field without a separate,
  measured decision. That is an automation-bias change, not a ranking change.
- Stop if a verification gate fails twice.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Automation bias: the reviewer clicks the first or model-recommended name | Keep the machine reading and crop prominent; separate row; no auto-fill; shadow evaluation; log input method; A/B |
| Position bias in the labels | Shadow mode keeps the baseline order; log shown order; split by file/time; do not train on judge-influenced clicks without a control |
| Candidate recall is the real bottleneck | Measure recall first; improve the pool; allow “no candidate”; do not count judge top-1 on cases where the truth is absent |
| Liquid prefers common names and loses rare family names | Stratify by rarity; include file names and exact file matches; compare on rare-name subset; reject if the lift is only common names |
| Liquid confidence is mistaken for accuracy | Calibrate; store confidence; never display a raw percentage; use thresholds only after evaluation |
| Local inference privacy | Keep the model and state on this machine; minimize state; no IDs; optional crop default off; record the decision. A hosted provider (Jev) stays optional and needs a separate privacy decision |
| Provider drift or version change | Pin model and prompt; store versions; re-run shadow evaluation on upgrades; no `latest` |
| Latency or rate limits | One call per suggestion; cache; shadow sample; 10s timeout; hide on failure; never block the editor |
| Experimental image mode | Behind a flag; compare text-only vs image; do not ship unless the lift is measured; do not rely on it for production |
| Small sample | 100-decision floor for any rate; 300 shadow decisions for adoption; bootstrap intervals; record “no result” |
| Over-normalization merges distinct names | Multiple keys; display never rewritten; `آل`/`ال` and `ع/ة` fixtures; skeleton as a feature only |
| AI candidates are not persisted | Store the snapshot as review evidence, not as a suggestion; if refused, drop AI-candidate claims |

## Maintenance notes

- The candidate snapshot is evidence, not truth. A reviewer can be wrong; the product is
  assistance, not an answer key.
- The judge output is a hypothesis. It is never a name, an identity, or a relationship.
- Re-run the offline report after any normalizer, lexicon, candidate-pool, prompt, or model
  change. The old evaluation does not carry forward.
- Keep `normalizerVersion`, `candidatePoolVersion`, model, and prompt versions in every
  snapshot. Without them, a result cannot be reproduced.
- Do not use reviewer click-through alone as a success metric. A wrong first candidate can
  attract clicks; measure corrections and time as well.
- The reader's unchanged-rate and the candidate-quality metrics are different numbers. Do not
  combine them into one “accuracy” figure.
- Liquid is optional. If it is removed tomorrow, the deterministic bench and the decision ledger
  must still work.
- The plan's long-term comparison is a small local ranker trained on the decision ledger. Liquid is
  a zero-shot judge, not a substitute for learning from the decisions we already collect.
