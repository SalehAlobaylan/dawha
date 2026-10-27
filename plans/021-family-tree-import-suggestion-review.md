# Plan 021: Put the machine's suggestions in the normal review screen

> **Executor instructions**: The machine suggests; a person decides. Every suggestion carries
> its evidence, every decision is recorded, and nothing is ever accepted automatically. The
> reviewer's corrections are the measurement — do not build a separate labelling campaign.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: MEDIUM — this is a review surface, and a review surface that quietly accepts
  machine output would break the platform's central rule
- **Depends on**: plan 019 (a reader that produces suggestions), plan 020 (identity matching and
  the research-graph write)
- **Category**: feature
- **Planned at**: 2026-10-21
- **Status, 2026-10-23**: partially implemented (uncommitted). The suggestion record, queue,
  decision ledger, source-resolution crop endpoint, stored reader alternatives, on-demand
  vision candidates, and the review workbench are present. Identity choices, relationship
  review, the single-file gate, and folding the bench into the normal draft dialog remain
  open.

## In plain language

Today, opening a family chart gives you a blank page and a drawing tool. You type every name
and draw every line yourself. The machine's reading of the chart is in a separate experimental
screen that only opens for one specific file, and its suggestions are deliberately kept away
from the import.

This plan makes the machine's reading the **starting point** of the normal review screen.
Instead of a blank page you get a list of what the machine thinks it saw: the names, where
they are on the page, and which existing people in the platform they might be. You accept the
right ones, correct the wrong ones, reject the rest. You still draw or confirm the
relationships, because the machine cannot know them.

**You are right that the answer keys are not the point.** They are not an input to the feature.
They are a *measurement*, and this plan makes the measurement free: every correction you make
while using the screen is recorded against the suggestion it corrected, so we learn how often
the machine was right from real use rather than from a labelling project.

## Why this matters

This is the feature. Everything else is preparation for it.

The pilot has the pieces but not the middle. The normal review path
(`apps/web/src/components/SourceTreeDraftDialog.tsx`) has **no suggestions at all** — a
reviewer types every name and draws every edge from scratch. The experimental path
(`SourceChartAnalysisDialog.tsx`) does have machine proposals, but it is gated to a single
SHA-256 (`apps/web/src/components/SourceProcessingPanel.tsx:266`), and its own hint says the
suggestions are "isolated from the import until they are qualified."

A reviewer who receives 400 machine-found names and a list of possible matches for each one
can finish a chart in minutes. A reviewer who receives a blank page needs hours. The
difference between those two is the difference between a feature families use and a feature
they try once.

Three properties have to hold, and they are the reason this is a plan rather than a UI change:

- **Suggestions must carry their evidence.** A proposed name with no crop and no coordinates
  is a thing to be doubted. A proposed name with the crop it came from is a thing to be
  checked, which takes a second instead of a minute.
- **Nothing is accepted automatically.** The repository's rule is that an AI output is never a
  fact: `candidateStatus()` (`internal/sourceprocessing/processor.go:1085`) forces every
  candidate to `needs_review`, and `internal/ai/client.go` rejects any response whose
  `review_required` is not true. A confidence score orders the queue. It does not authorise an
  acceptance.
- **The decisions are the measurement.** A reviewer's correction is a labelled example, and it
  is the only kind of label we will get at volume.

## What is implemented

The suggestion record, the review queue and the review screen are implemented.

**The record** — `db/migrations/0051_source_tree_suggestions.sql`:

- `source_tree_suggestions` — one machine reading of one place on a page. It carries where the
  reading came from (the page, the region, the method, the model), what the machine read, and
  what the reviewer decided, **beside** the machine's reading and never instead of it.
- `source_tree_suggestion_decisions` — the append-only ledger. Every correction a reviewer
  makes is a labelled example, so the feature produces its own ground truth: the answer key is
  the use of the feature, not a separate labelling project.

**The service** — `internal/sourceprocessing/tree_suggestions.go`: create the suggestions of a
reading run, list them as a queue ordered by confidence, record a decision, and report the
accuracy the decisions imply. A decided suggestion cannot be decided again. A correction must
carry the correction, and a rejection must carry no text. A suggestion creates no person.

**The reading** — `ReadSuggestions` runs the whole chain over a stored file: render the page,
find the name carriers, group the ink into names, read each name in isolation, and record what
it found. Without a reader it still records the carriers it found, which is the localiser's
recall measured against nothing.

**The screen** — `apps/web/src/components/SourceSuggestionReview.tsx`: a full-screen name
workbench opened from source processing. Its three parts are the source-resolution tight and
context crops with a page map, the machine's reading with its stored alternatives, and a
keyboard-first Arabic editor. The editor accepts, corrects, rejects, or defers each suggestion;
`Enter` saves and advances, `a` accepts the reading, `r` rejects the place, `d` defers it,
`j`/`k` navigate without deciding, `1`–`8` choose a suggested name, and `/` focuses the field.
Accepted and corrected names become spelling candidates for later names in the same file. The
header reports the decision count and the unchanged-reading rate, and shows a dash plus
«لا نسبة بعد» instead of a percentage while the sample is too small to support one (the API's
floor is 100 accepted, corrected, or rejected decisions).

**The source crop** — `internal/sourceprocessing/suggestion_crops.go` and
`GET /api/v1/source-tree-suggestions/{id}/crop`: the reviewer sees the region rendered from the
original file, not the bounded page preview. The tight crop adds 20 % padding; the context crop
reaches five name-widths and four name-heights with a 560×300 px floor; both are capped at
1,600 px on the long side and 2.5 megapixels. A reviewer without permission on the source is
refused. `TestASuggestionCropComesFromTheOriginalSource` also pins that the candidate model
receives exactly the tight crop the reviewer sees.

**The alternatives** — migration `0052_source_tree_suggestion_alternatives.sql` adds
`alternatives jsonb`. `CreateSuggestions` and `ReadSuggestions` persist the reader's ranked
alternatives and return them with the suggestion, so an ambiguous reading is a one-click
correction rather than a single string the reviewer has to overwrite.

**The candidates** — `POST /api/v1/source-tree-suggestions/{id}/candidates` and
`/v1/vision/chart/candidates`: an on-demand request for at most eight spelling hypotheses for
one tight crop. It persists nothing, receives no source metadata, and never writes a name. The
UI labels the result a hypothesis and leaves the decision in the editor. One probe on a crop a
person reads as `أحمد` returned eight candidates including `أحمد`; a second probe on a crop a
person reads as `جاسم` returned `علي`, `على`, `عبد`, `عيد`, `عبدالله`, `عليا`, `علا`, `عبيد`
and missed `جاسم`. Those are probes, not an accuracy rate, and the screen does not report them
as one; they are why an unmatched model candidate has its own labelled row rather than being
folded into the ranked spelling chips.

## Current state

- **The suggestion workbench exists, but it is not yet the normal draft screen.** It opens
  from `SourceProcessingPanel.tsx`; `SourceTreeDraftDialog.tsx` still draws nodes and edges
  over the page preview without starting from the suggestions. Folding the two together is
  the remaining integration step, not a reason to remove either surface.
- **The normal review flow has no suggestions of its own.** `SourceTreeDraftDialog.tsx` draws
  nodes and edges over the page preview, includes and excludes nodes, and blocks the import on
  graph errors — an unreviewed name, a parent-of cycle, or an unclear edge with fewer than two
  reviewers.
- **The experimental flow has proposals, and is pinned to one file.** The chart-analysis job
  type, the unit ledger and the review-revision table exist
  (`db/migrations/0048_chart_vision_analysis.sql`, `0049_chart_subtile_units.sql`), and the
  adapter is enabled for exactly one SHA-256
  (`internal/sourceprocessing/chart_analysis.go:29`).
- **The reading pipeline produces regions, not suggestions.** `PageReading` and `PageRegion`
  (`internal/sourceprocessing/types.go:263-282`) carry a method, a model, a raw score and a
  normalised box. There is no proposal entity in the normal path.
- **The identity machinery to suggest *against* exists.** `entityReferences`
  (`internal/sourceprocessing/processor.go:725`) searches eight entity kinds by normalised
  name including aliases, and `entity_resolution_candidates` already carries a match class, a
  score and a review status with a priority-ordered queue
  (`db/migrations/0015_entity_resolution.sql:43`, `:68`).
- **Append-only review revisions already exist** for page transcriptions
  (`internal/sourceprocessing/page_reviews.go`), with optimistic concurrency and stale-edit
  refusal. The suggestion review needs the same shape.
- **The suggestion surface has a precedent elsewhere.** `apps/web/src/components/SuggestionPanel.tsx`
  is the platform's existing way of presenting machine proposals with an accept/reject path.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |

## Scope

**In scope**:

- `apps/web/src/components/SourceTreeDraftDialog.tsx` — the suggestion list becomes the
  starting point
- `apps/web/src/components/SourceChartAnalysisDialog.tsx` — the experimental surface is folded
  into the normal one, or deleted
- `apps/web/src/components/SourceProcessingPanel.tsx` — the SHA-256 gate becomes a policy
- `apps/web/src/lib/api.ts`, `apps/web/src/types.ts` — the suggestion endpoints and types
- `services/core-api/internal/sourceprocessing/` — the suggestion entity, the review endpoint,
  and the decision log
- `services/core-api/internal/httpapi/` — the routes
- `db/migrations/` — additive only
- `docs/` — the correction of the status records

**Out of scope**:

- **Automatic relationship inference.** The machine suggests names and identity matches. The
  reviewer draws and confirms the connections. This plan does not change that.
- **Automatic acceptance of anything.** No confidence threshold may auto-accept a name, an
  identity, or a relationship. This is the platform's rule, not a preference.
- **Publishing a tree.** Unchanged.
- **The reading pipeline itself.** That is plan 019.
- **The identity write path.** That is plan 020; this plan consumes its candidates.

## Steps

### Step 1: A first-class suggestion entity

The machine's reading must become a stored, reviewable thing rather than a transient display.

- **A suggestion is an occurrence, not a name.** Two "محمد" on one chart are two suggestions,
  each with its own page, its own box and its own crop. The harness already refuses to align
  on equal strings (`tools/ocr-eval/evaluate.py`), and the product must do the same.
- **A suggestion carries its evidence**: the page evidence id, the page number, the normalised
  region, the crop it was read from, the reading method, the model and version, the raw score,
  and the confidence derived from agreement.
- **A suggestion is never a person.** It is a proposed occurrence of text at a place on a
  page. It becomes a person only through the reviewed import path (plan 020).
- **Additive migration.** A `source_tree_suggestions` table alongside the draft tables, with
  the occurrence id, the draft id, the page evidence id, the region, the raw text, the
  corrected text, the confidence, the decision and the reviewer.

### Step 2: Review the suggestions, one at a time, with the evidence beside them

- **Each suggestion shows the crop it came from**, at a size where the reviewer can read it,
  next to the machine's transcription and an editable Arabic field.
- **Four decisions: accept, correct, reject, defer.** Correct stores both the machine's value
  and the reviewer's, which is what makes the correction usable as a label.
- **Rejection leaves no trace in the canonical graph** — but it is kept, with its decision, in
  the suggestion log. A rejected suggestion is data, not noise.
- **The decisions are append-only**, on the same shape as
  `internal/sourceprocessing/page_reviews.go`, with optimistic concurrency: a stale edit is
  refused rather than overwriting someone else's decision.
- **Keyboard-driven.** A reviewer processing 400 names needs accept/correct/reject without the
  mouse. This is not a nicety; it is the difference between minutes and hours.

### Step 3: Offer the identity matches as choices, not as facts

- **For each accepted name, show the people it might already be**, from the matching the
  entity-resolution surface produces (plan 020), with the score, the match class, and the
  signals that produced it.
- **The reviewer chooses: this is that person / this is a new person / not sure.** "Not sure"
  must be a real option, and it must be the default for a low-scoring match.
- **Choosing "this is that person" links to the existing person. Choosing "new" creates one.**
  Nothing merges without that explicit choice, and the merge remains reversible through
  `entity_merges`.
- **The suggestion of a match is not a merge.** The two are different records with different
  reversibility, and the UI must not blur them.

### Step 4: Keep the machine's reading out of the relationship graph

- **The reviewer draws and confirms connections.** The machine may propose candidate
  connectors — the geometry exists — but the endpoint pair, the direction and the type are the
  reviewer's to choose, and a second distinct reviewer is required for an unclear one, exactly
  as the current draft flow does.
- **The reviewer may accept a whole drawn connector in one action**, but the action is a
  reviewed decision with a recorded reviewer, not an automatic interpretation.
- **This plan does not relax the rule that a relationship is always created `unresolved`**
  (`internal/identity/validation.go:33-35`).

### Step 5: Log the decisions, and report the accuracy from them

This is the step that makes the measurement free, and it is why the plan exists in this form.

- **Log every decision against the suggestion it decided**: the machine's value, the final
  value, the decision, the reviewer, and the timestamp.
- **Report, from the log**: how many suggestions were accepted unchanged, corrected, rejected
  and deferred, per document and per chart class. That is the accuracy number, and it needs no
  annotation campaign.
- **The number must carry its sample size.** A percentage computed from nine suggestions is
  not a percentage, and the report must refuse to present it as one.
- **A correction is a training or tuning signal, not an automatic retraining trigger.** Fixing
  a reading requires changing the reader or the prompt, and that change is measured the same
  way — the log before and after.

### Step 6: Make the flow normal, and remove the single-file gate

- **The SHA-256 authorisation becomes a policy.** `chart_analysis.go:29` pins one digest for
  the hosted-vision experiment; with hosted processing permitted
  (`docs/ocr-and-import-decision.md` §11) the gate becomes a route configuration, and the
  provider, region and retention behaviour are recorded per route rather than per digest.
- **The experimental dialog is folded into the normal one.** Two suggestion surfaces that
  behave differently is the kind of duplication that produces a defect nobody can reproduce.
- **A chart with no suggestions still opens.** A document the reader cannot parse must still
  be reviewable by hand, with the same evidence, the same revision history and the same
  retraction. The machine failing must not make the document unreadable.

## Test plan

- **A suggestion without evidence is rejected by the API.** The endpoint validates that every
  suggestion carries a page evidence id and a region.
- **Two equal names on one page are two suggestions**, and neither can be accepted as the
  other.
- **The four decisions all work**, and a correction stores both values.
- **A stale decision is refused** with the same conflict behaviour as the page-review path.
- **Nothing is ever accepted without a reviewer.** A test asserts that no code path writes an
  accepted suggestion without `reviewed_by`.
- **A confidence score never drives acceptance.** A test asserts that a suggestion with
  confidence 1.0 and no reviewer decision is still undecided, and that the decision list is
  ordered by confidence but gated on the decision.
- **The identity choice is explicit.** A test asserts that no `entity_merges` row with
  `state='applied'` exists without a reviewer decision, and that "not sure" leaves the
  suggestion unlinked.
- **A rejected suggestion writes nothing to the canonical graph**, and is still present in the
  decision log.
- **The accuracy report is computed from the log**, and a report computed from too few
  decisions says so instead of quoting a percentage.
- **A chart with no suggestions still opens for hand review** and still imports.
- **The whole journey is tested against a real PostgreSQL** and leaves no synthetic actor
  behind.

## Done criteria

- [x] A suggestion is a stored, reviewable occurrence with its evidence, its reading method and
      its confidence.
- [ ] The normal draft screen itself opens on the machine's suggestions with the crop beside
      each one. The separate workbench does this today and is keyboard-driven; folding it into
      the draft screen remains open.
- [x] Accept, correct, reject and defer all work, and a correction stores both values.
- [ ] Identity matches are offered as explicit choices, and "not sure" is a real option.
- [ ] Connections remain the reviewer's to draw and confirm, with the second-reviewer rule
      unchanged.
- [x] Every decision is logged against its suggestion, and the accuracy report is computed from
      that log with its sample size stated.
- [x] No suggestion is ever accepted without a recorded reviewer. Identity and relationship
      acceptance are not part of the implemented surface yet; those criteria remain open.
- [ ] The single-file gate is gone and the experimental dialog is folded into the normal review
      flow. A source with no suggestions can still be opened for hand review from source
      processing today.
- [x] `docs/ocr-and-import-decision.md`, `docs/phase-status.md` Phase 14 and
      `docs/import-reading-strategy.md` state the corrected position for the implemented part.
- [ ] `make verify`, `make verify-full` and `make e2e` pass; `git diff --check` passes.
      `make verify`, `make e2e` and `git diff --check` passed on 2026-10-23; `make verify-full`
      was not run because it requires stopping the development database.
- [x] `plans/README.md` updated.

## STOP conditions

- Stop if making the suggestions the default would require auto-accepting a low-confidence
  suggestion to be usable. The answer is a better reader or a smaller chart, never a silent
  acceptance.
- Stop if the decision log cannot be written transactionally with the decision. A suggestion
  decided without a durable record is a suggestion nobody can learn from.
- Stop if folding the experimental dialog into the normal one would remove a capability that
  the normal one cannot yet do — such as the resumable unit ledger. Carry that capability
  across first; do not lose it.
- Stop if the accuracy report would be quoted from a sample too small to support it. The report
  must refuse instead.
- Stop if a verification gate fails twice.

## Maintenance notes

- The suggestion log grows with use and is the platform's most valuable dataset about how well
  the reader works. It is append-only, and it is not a cache.
- The confidence on a suggestion orders the queue and nothing else. If a future change makes
  it authorise an acceptance, the platform's central rule has been broken and the change is
  wrong.
- The suggestions and the identity candidates are different things with different
  reversibility. Merging the two surfaces will make one of them wrong.
