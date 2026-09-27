# Plan 016: Fix the remaining correctness, privacy and consistency defects

> **Executor instructions**: Every item here is a recorded defect with a named location and a known cause. Fix the cause, add the test that fails without the fix, and update the record. None of these may be closed by a comment.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: plans 001, 002, 006, 009, 012, 015
- **Category**: bug
- **Planned at**: commit `0ae4171`, 2026-09-27

## Why this matters

`docs/phase-status.md` ends with a residuals table, and this plan is the implementable half of
it. Each entry is a real defect, not a limitation of the product or a measurement this
repository cannot take: a behaviour that changes depending on whether a service is up, a
privacy predicate that is not applied on one read path, a query that can never match, a count
that can disagree with what was written, an order that is not total, and a duplicated decision
that two files must keep in agreement by hand.

The other residuals are deliberately out of scope and stay open: `S3Store` has never contacted
R2 (a cloud decision), rate limits are per-process and per-IP (a shared limiter is a decision
about infrastructure), the upload quarantine policy is unspecified, and Phase 20 and Phase 18's
criteria need data this repository does not have. OCR is now discussed in
`docs/ocr-and-import-decision.md`.

## Current state

- **Routing depends on whether the AI service is up.** `internal/ai/routing.go` and
  `services/ai-research/app/main.py` are two implementations of one decision in two languages.
  Across the 33 labelled cases they agree on every route except `rt-019`: a punctuation-only
  question, where Go's normalizer reduces the text to nothing and returns `ignore`, while the
  provider keeps the text and returns `cheap`.
- **The research workspace reads person aliases without source scoping.**
  `internal/research/workspace.go:437` selects `value_ar` from `person_aliases` with no source
  predicate. This is the same class of leak plan 001 closed everywhere else, and it is the one
  remaining gap in Phase 19.
- **`search_sources` can never match a NULL source id.**
  `internal/researchagent/stages.go:59-70` filters with `($1 = '' OR s.id = $1::uuid)`; when
  `$1` is NULL the predicate is NULL, so an agent run scoped to a person with no `source_id`
  finds no source statements at all.
- **A step can report more evidence than it wrote.** `internal/researchagent/service.go:418`
  counts `len(refs)` in memory while `persistEvidence` writes with `ON CONFLICT DO NOTHING` on
  `(run_id, reference_type, reference_id, stance)`, so a stage citing a reference another stage
  already cited reports one more item than exists.
- **Two lists have no total order.** `internal/sourceprocessing/review.go:217` and `:253` order
  by `created_at DESC` with no tiebreak, so two rows created in one transaction can come back in
  either order. Harmless today, unstable tomorrow, and it makes pagination a guess.
- **A cost model is duplicated as data.** `internal/ai/cost.go`'s `routeWork` describes
  `rag_service.go`'s switch rather than calling it — because `internal/research` is off-limits
  to the plan that wrote it. Two files must agree with no compiler checking them.
- **The extractor proposes spans that are much longer than the entity.**
  `services/ai-research/app/main.py` matches any run of four or more Arabic characters, which is
  why `extraction.entity_precision` measures 0.3333 and sits in `KNOWN_DEFECTS`.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Eval | `make ai-eval` | exit 0, with the recorded measurement updated honestly |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |

## Scope

**In scope**:
- `services/core-api/internal/ai/routing.go`, `internal/ai/cost.go` and their tests
- `services/core-api/internal/research/workspace.go` and its tests
- `services/core-api/internal/researchagent/{stages.go,service.go}` and their tests
- `services/core-api/internal/sourceprocessing/review.go` and its tests
- `services/ai-research/app/main.py` (the entity span rule only)
- `services/ai-research/evaluation/` — the recorded measurement and the note that describes it
- `docs/phase-status.md` and `docs/ocr-and-import-decision.md` (the pointer only)

**Out of scope**:
- `internal/research/rag_service.go`'s retrieval behaviour and anything in `internal/research` beyond the one alias query.
- R2, a shared rate limiter, the upload quarantine policy, OCR, and any retrieval threshold.
- Adding a routing label, changing what routing decides, or making a route a price multiplier.

## Steps

### Step 1: One routing decision, not two

Make the Go fallback and the provider agree, and keep them agreeing. Choose the shape and
justify it:

- **(a) One authority** — the Go decision is the fallback and the provider is authoritative;
  the evaluation compares against the provider's labels, and the fallback's divergence is a
  named, tested difference rather than an accident.
- **(b) Shared vocabulary** — align the normalization so both reduce a punctuation-only query to
  the same thing, and keep both implementations.

Requirements: the punctuation case resolves deterministically; a test derives the two routes for
every labelled case and fails if the divergence set changes shape; and whichever authority you
choose is stated in the code and in the evaluation report, so a reader is never left guessing
which one a number came from. If the two languages must stay separate implementations, say
what keeps them in step, because today the answer is "a comment".

### Step 2: Scope the workspace's aliases

Apply the plan 001 rules to `internal/research/workspace.go`: a person's aliases must pass the
person policy, and an alias whose source is not visible to the actor must not appear — the same
shape `internal/dictionary` uses, with the same "no platform-record alias is public by accident"
care. A research-role actor keeps the view it has today for what it may read; a research-only
person it may not read must not surface through its aliases. Test both directions, and test that
an alias taken from a private source does not ride along with a visible person.

### Step 3: Fix the two research-agent defects

- `search_sources` must treat a NULL source id as "no source constraint" rather than as a
  predicate that can never be true. Write the predicate so the intent is readable, and test a
  run scoped to a person with a `source_id` **and** one without, asserting the second now finds
  material. Check the same pattern for any sibling stage that takes an optional id.
- The per-step evidence count must be what was written, not what was offered: derive it from the
  rows that persisted, and test a case where two stages cite the same reference and assert the
  step's count equals the number of distinct rows for that step.

### Step 4: Make the two lists totally ordered

Add an explicit tiebreak to both `review.go` queries so an order is defined rather than
incidental, matching what the repository already does elsewhere (`ORDER BY lexical_score DESC,
id`). Tests must pin the order for rows created in the same transaction, and pagination must be
stable across two identical requests. Do not change which rows are returned.

### Step 5: Pin the duplicated decision, and bound the extractor's spans

- Add a test that fails if `routeWork` and `rag_service.go`'s route switch disagree. If the two
  can be made to share one source without crossing the scope boundary, do that instead and say
  why not; if not, the test is the minimum and its name should say what it protects.
- Bound the extractor's proposed entity span to the name rather than any run of four or more
  characters, and score the exact span rather than containment, as the recorded note already
  prescribes. This is the one item in this plan that changes measured AI behaviour: `make
  ai-eval` must be re-run, the new `extraction.entity_precision` recorded, the entry removed
  from `KNOWN_DEFECTS` only if it is actually fixed, and the report's numbers updated with no
  threshold moved. If the fix does not reach a defensible precision, report the measured value
  rather than adjusting a threshold.

### Step 6: Update the record

Remove each fixed residual from `docs/phase-status.md` and add its test to the evidence for the
phase it belongs to. Leave the four deferred residuals in place with their reasons. Add the
pointer from Phase 14 to `docs/ocr-and-import-decision.md` if it is not already there.

## Test plan

- Routing: the punctuation case, and the divergence set for all labelled cases.
- Workspace: a private person's aliases absent, a visible person's private-source alias absent, a role holder's view unchanged.
- Research agent: a NULL source id finds material; a doubly-cited reference counts once.
- Ordering: rows from one transaction come back in the same order twice, and pagination is stable.
- The `routeWork` agreement test fails when the switch and the data disagree.
- Extraction: a long Arabic sentence yields the name, not the sentence; `make ai-eval` reflects it.

## Done criteria

- [ ] Routing no longer depends on whether the AI service is up, and the authority is documented.
- [ ] The research workspace applies the person and alias-source policies.
- [ ] A NULL source id no longer makes `search_sources` unmatchable, and a step's evidence count equals what it wrote.
- [ ] `source_files` and `source_processing_runs` have a total order, proven by a test.
- [ ] The `routeWork` duplication is pinned by a failing-if-disagreeing test.
- [ ] The extractor bounds its spans, and the recorded measurement is updated honestly.
- [ ] `make verify`, `make verify-full`, `make ai-eval` and `make e2e` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if aligning routing would require moving a decision between languages or adding a shared runtime dependency; report the smallest alternative.
- Stop if bounding the entity span would require changing the extraction response contract; report what does not.
- Stop if verification fails twice.

## Maintenance notes

Every item here was recorded rather than fixed, which is how a repository accumulates a residuals table. The tests added here are the reason they can now be deleted from it.
