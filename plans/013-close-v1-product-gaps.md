# Plan 013: Close the V1 product gaps the status review found

> **Executor instructions**: Three concrete gaps and two omissions in the record. Everything here is bounded work with a testable outcome. Do not redesign surfaces you touch, and do not turn a documentation correction into a feature.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: LOW
- **Depends on**: plans 001, 002, 005, 010
- **Category**: bug/feature/docs
- **Planned at**: commit `4c40e77`, 2026-09-27

## Why this matters

`docs/phase-status.md` marks 14 of the 18 V1 steps proved. The four that are not include two blocked by a single defective SQL predicate, one blocked because a shipped button cannot do what the API can, and two criteria the plan requires that the status document does not mention at all. None of this is architectural; it is a predicate, a form, and two sentences in a document.

## Current state

- `internal/dictionary/service.go` — the `families`, `tribes`, `branches`, `places` and `people` index branches match the normalized term against `normalized_name_ar`; the `disputed-claims` branch matches the same normalized term against the **raw** `ps.canonical_name_ar` / `po.canonical_name_ar`. `NormalizeArabicName` maps ة to ه, so a name containing ة matches in five indexes and not in the sixth. This is Blocker 3 in `docs/phase-status.md:500`.
- `apps/web/src/components/SuggestionPanel.tsx` sends `{ decision, note_ar }`. `internal/suggestions` accepts a typed `ChangeSet` with four targets (`person`, `relationship`, `claim`, `source_link`), so V1 step 12's "accepted suggestion can create domain changes" is reachable only through the API. Phase 8 is `partial` for this reason.
- `IMPLEMENTATION_PLAN.md:1145` lists "text/OCR extraction" in the Phase 14 pipeline. No OCR exists: `internal/sourceprocessing/upload.go` accepts `text/*`, `application/json` and `application/xml` and refuses a PDF with 415. The decision was deliberate (plan 003) but `docs/phase-status.md` marks Phase 14 `implemented` without saying so.
- `IMPLEMENTATION_PLAN.md:946` asks for "Hijri or Gregorian display support where practical". No `hijri` handling exists anywhere in `apps/web` or the API; the demo data carries period strings such as `قبل ١١٥٠هـ` as free text.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |
| Benchmarks | `make ai-eval` | exit 0 |
| Docs | `python3 infra/local/check-doc-links.py` | no broken local links |

## Scope

**In scope**:
- `services/core-api/internal/dictionary/service.go` and its tests
- `services/core-api/internal/search/service.go` and its tests, where the same raw/normalized asymmetry exists in claim-name matching
- `apps/web/src/components/SuggestionPanel.tsx`, `apps/web/src/lib/api.ts`, `apps/web/src/types.ts`
- `apps/web/e2e/journeys/06-suggestion.spec.ts` plus one new journey if that is the honest home for the composer
- `docs/phase-status.md`, and `IMPLEMENTATION_PLAN.md` only where a criterion is being restated as a decision

**Out of scope**:
- OCR, or any PDF/image ingestion. The format decision stands.
- A general date-conversion library, a calendar library, or a new dependency.
- Anything in plans 014/015.

## Steps

### Step 1: Fix the ة defect in every index that has it

Find every place a normalized search term is compared against a raw column, not only the one Blocker 3 names. `internal/search/service.go` has the same asymmetry in claim-name matching, and any other index branch with the pattern is in scope. Fix them so the term is compared against a normalized value, and decide per site whether that means matching the normalized column or normalizing the stored column in the predicate.

Requirements:

- **Match the columns, do not re-normalize at query time.** A `lower(replace(...))` over a stored column defeats the trigram index plan 007 justified and re-introduces the unindexed scan.
- Test both directions with a fixture whose name contains ة: it is found in the people index, the disputed-claims index, and claim-name search; the same person is still found when the term is already normalized; and a name that differs only by ة/ه does not match when the term is normalized but the row is not — or does, if you make it so, and the test says which.
- A test that pins the *disagreement* between the indexes would have caught this; add one that asserts every people-facing index resolves the same term the same way.

**Verify**: `DATABASE_URL=... go test ./internal/dictionary ./internal/search ./internal/httpapi -count=1` → all pass, and the new tests fail against the old predicates.

### Step 2: Compose a typed change set in the review panel

Add the minimum composer that lets a reviewer express each of the four typed targets the API already accepts. Requirements:

- Reuse the existing form components, the existing `ApiError` handling and the existing React Query invalidation patterns. No new styling system.
- Show what the acceptance will do **before** it is sent: target, identifiers, and the record it will create. A reviewer approving a change should not have to imagine it.
- Submit exactly the `ChangeSet` shape `internal/suggestions` validates, and surface its validation errors per field. The API is the authority; the form must not invent a second rule set.
- The composer must not offer publish, claim acceptance, merge, or dispute resolution — the API refuses all of them and the UI should not imply otherwise.
- A reviewer without a global write role must see why they cannot apply a change (the API answers 403), with the accept-without-change-set path still available, since that is what the panel does today.

**Verify**: a browser journey that accepts a suggestion **with a change set** and asserts the resulting record exists and the review is audited. The journey must fail against the panel as it stands today.

### Step 3: State the two omissions, and decide the date contract

Correct the record first, then decide:

- `docs/phase-status.md`: Phase 14 must say that the OCR stage of `IMPLEMENTATION_PLAN.md:1145` is not implemented, that this is a deliberate V1 decision, and where it is recorded. Do not leave a phase marked `implemented` while one of its pipeline stages is absent and unmentioned.
- The same file, for Phase 10, must state the actual state of date display: what is stored, what is rendered, and that no calendar conversion exists.
- Then make an explicit decision on Hijri/Gregorian display and record it with its reasoning:
  - **(a) Document a contract only** — the product stores approximate ranges and deliberately refuses to assert precision, so a conversion is a product decision, not a bug fix. Specify how an approximate range, an unknown bound, and a recorded period string are each rendered, and make the surfaces agree.
  - **(b) Implement conversion** — a single documented conversion helper with its own tests, applied only to values the schema marks as real dates, never to a free-text period, and never presented as more precise than the record is.
  Choose one, justify it in the document, and if (b), keep the dependency surface minimal and say what the source of the conversion is. Whatever you choose, the status document must end up matching the code.

**Verify**: `make verify`, `make verify-full`, `make e2e` all pass; the status document's Phase 10 and 14 rows match the code; the doc-link checker is clean.

## Test plan

- ة/h handling across every people-facing index, in both directions, with the old predicates shown failing.
- Change-set composition: valid for each target, validation errors surfaced, 403 explained, no forbidden action offered.
- Date rendering for an approximate range, an open bound, and a free-text period.
- The status document's claims re-checked against the code as part of the change, not after it.

## Done criteria

- [ ] No people-facing index compares a normalized term against a raw column.
- [ ] A reviewer can express all four typed change targets in the browser, and a journey proves one creates its record.
- [ ] Phase 14's absent OCR stage and Phase 10's absent date conversion are stated in the record, and the date-display decision is written down and matches the code.
- [ ] `make verify`, `make verify-full`, `make e2e` and `make ai-eval` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if fixing a raw/normalized comparison would require re-normalizing a stored column in place — that is a data migration and a product decision, not a query fix.
- Stop if the composer would need a second validation rule set to work; the API stays the only authority.
- Stop if the date decision turns out to need a third-party calendar dependency — report the smallest honest alternative instead.
- Stop if a verification gate fails twice.

## Maintenance notes

Two of the three gaps existed because a document asserted a state nobody re-derived from the code. When this plan changes a status row, re-derive the whole row.
