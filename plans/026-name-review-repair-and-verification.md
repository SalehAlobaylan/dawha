# Plan 026: Give reviewers history, deferred work, and region-repair tools

> **Executor instructions:** Preserve every original human decision and machine
> reading. Corrections and reopen actions are new events, never rewrites. Leave
> all changes uncommitted, with no AI or agent attribution.
>
> **Drift check:** Written at `f0fcb6e` with uncommitted Plan 021/022 code. Run
> `git diff --stat f0fcb6e..HEAD -- apps/web/src services/core-api db/migrations`
> and inspect `git status --short`. Compare these excerpts and completed
> prerequisite contracts before implementation.

## Status

- **Priority:** P1 for deferred/history repair; P2 for split/merge tools
- **Effort:** L — release the history path before advanced geometry tools
- **Risk:** HIGH — append-only history, geometry lineage, evaluation counting
- **Depends on:** [023](023-local-reading-capabilities-and-cohorts.md) and
  [024](024-durable-local-name-reading.md); evaluation joins follow
  [025](025-name-context-and-shadow-preparation.md)
- **Category:** reviewer tooling / correctness / verification
- **Planned at:** commit `f0fcb6e`, 2026-10-27
- **Status:** DONE — append-only reopen/revision events, deferred/history queue
  views with Arabic bench controls, immutable normalized region revisions with
  preview/cancel/confirm, split/merge lineage and superseded exclusion,
  first/latest evaluation truth policies with pinned cutoff, local reviewer
  assistance, isolated browser journey, and the guarded
  `name-pipeline-verify` gate. Existing 111 actions and the frozen class map were
  not rewritten.

## Why this matters

The reviewer has deferred 14 regions and rejected 17 in the 111-action pilot.
The current bench removes these items and has no return path; the backend also
refuses a subsequent decision. A bad detection rectangle can likewise only be
worked around by manual transcription. Reviewers need to recover difficult or
mistaken work and repair the evidence they read.

The intended result is a small set of concrete tools: resume deferred work,
inspect/correct a previous decision, adjust a crop, split/merge detected regions,
compare local reading variants, and request an explicit local reread. Each tool
must preserve provenance and keep publication/import decisions human-governed.

## Current state and conventions

- `SourceSuggestionReview.tsx:63` has only `all|unreadable|uncertain|page` filters.
  Initial, additional, and reload fetches select only `status="suggested"`.
- Saving any decision removes the item from the local list at line 670.
  `describeDecision:1326` says “أُجّل الموضع إلى مراجعة لاحقة.”
- `tree_suggestions.go:398–400` refuses a decision unless status is `suggested`.
  Deferral therefore has no implemented later-review path.
- The page map selects existing rectangles at
  `SourceSuggestionReview.tsx:1065–1075`; it cannot repair a rectangle.
  `suggestion_crops.go:154` renders the stored region as-is.
- Migration 0051 calls the decision ledger append-only. Preserve that contract.
- `sourceprocessing/page_reviews.go:18–87` is the existing revision exemplar:
  `ExpectedRevision`, a source-evidence row lock, immutable inserted revision,
  and a conflict on stale browser state. Reuse that shape for suggestion review.
- The browser suite currently has no journey opening the name bench; Plan 022's
  acceptance notes acknowledge this. `apps/web/e2e/fixtures.ts` supplies actor
  and session helpers; the test stack has separate ports and fake-provider seams.

```go
// tree_suggestions.go:398–400
if status != "suggested" {
    return SuggestionView{}, ErrSuggestionAlreadyDecided
}
```

```go
// page_reviews.go, existing optimistic revision pattern
if input.ExpectedRevision != latestRevision {
    return PageReviewRevision{}, ErrConflict
}
```

`PRODUCT.md` §4 requires provenance and human-governed publication; Plan 021
requires every reviewer decision to remain recorded. Reopening must not erase an
earlier answer, silently create a person, or count one physical occurrence twice
as independent evaluation truth.

## Isolated database verification preflight

Run database-writing tests only on a dedicated local test database. Plan 023
adds the guarded `name-pipeline-db-verify` runner before integration work begins. The operator
must set `PIPELINE_TEST_DB_NAME` and `PIPELINE_TEST_DATABASE_URL`; the name must
end in `_pipeline_test`. Never default either variable to the review database.
Use fresh temporary test storage, not `.data/source-storage`.

Before any database test command in this plan, run this guard in the same shell:

```sh
set -eu
python3 - <<'CHECK'
import os, re
from urllib.parse import urlparse
name = os.environ.get("PIPELINE_TEST_DB_NAME", "")
url = urlparse(os.environ.get("PIPELINE_TEST_DATABASE_URL", ""))
assert re.fullmatch(r"[a-z][a-z0-9_]*_pipeline_test", name), "dedicated test database required"
assert url.scheme in {"postgres", "postgresql"}, "PostgreSQL test URL required"
assert not url.query and not url.fragment, "test URL overrides are forbidden"
assert url.path.lstrip("/") == name, "test database URL/name mismatch"
assert url.hostname in {"localhost", "127.0.0.1", "::1"}, "local test database required"
CHECK
PIPELINE_TEST_STORAGE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/dawha-pipeline-test.XXXXXX")
export PIPELINE_TEST_STORAGE_DIR
```

Create the named database if absent using the local development database role;
never drop/recreate an existing database automatically. Apply migrations and seed
with both `POSTGRES_DB="$PIPELINE_TEST_DB_NAME"` and
`DATABASE_URL="$PIPELINE_TEST_DATABASE_URL"` set. For the full acceptance gate:

```sh
make name-pipeline-db-verify NAME_PIPELINE_TEST_GATE=verify-full
```

Use the stage-023 guarded runner for database tests; it also clears inherited
PostgreSQL connection/service/options environment overrides before launching
children. Do not bypass it with an unvalidated connection string.

Expect exit 0 and no database skips. Test-only fixtures must never become labels
in the real evaluation ledger. Preserve a failing test database for diagnosis;
cleanup may touch only that explicitly isolated scope.

## Scope

**Modify only:**

- `services/core-api/internal/sourceprocessing/{tree_suggestions.go,types.go,
  suggestion_crops.go}` and tests; new review-event/region-revision files/tests
- `services/core-api/internal/httpapi/{router.go,source_processing_handler.go}`
  and tests; `cmd/name-eval/` and the new cmd/name-pipeline/ directory
- New additive migrations after the current highest migration; necessary queries
  and generated output
- `apps/web/src/components/SourceSuggestionReview.tsx`, new focused history and
  region-editor components, `lib/api.ts`, `types.ts`, related tests and styles
- New `apps/web/e2e/journeys/` name-bench journeys, test fixtures/stack/README
- `Makefile`, relevant reading/evaluation docs, and plan status rows

**Out of scope:** deleting history, rewriting source files, model/relationship
autoacceptance, publishing/importing a family tree, hosted vision enablement,
and a general redesign of the application shell.

## Steps

### 1. Model explicit reopening and decision revisions

Add an append-only suggestion-review event/revision model. Events record a human
actor, event type, previous decision/revision, geometry/reading/preparation
references, and timestamp. Reopening changes the operational queue state but
retains all earlier decision rows. Subsequent decisions explicitly supersede a
prior decision; never edit or delete its text, candidate snapshot, or judge output.

Use a row lock and `expectedRevision` for reopen and decide. Preserve the existing
first-decision contract for old clients; reject stale competing writes with 409.
Two simultaneous requests cannot append two current decisions for one revision.
Only a reviewer with normal source access may inspect history or reopen work.

Define evaluation truth policy explicitly: the original Plan 022 cohort remains
on its original first-human-decision/cutoff semantics. A new evaluation may choose
the latest valid human revision as of a pinned cutoff. Neither policy counts all
revisions as independent names. Expose superseded counts separately.
Reopening never resets blind-evaluation eligibility. Reuse an eligible original
initial preparation under its original cohort, or classify new post-history
preparation as operational/development evidence. Apply this to deferred items and
split/merge descendants as well as previously accepted/corrected occurrences.
An old answer cannot become its own file-context hint in a new supposedly blind
evaluation. Plan 025's first-exposure/lineage eligibility contract is authoritative.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_PACKAGES="./internal/sourceprocessing ./internal/httpapi"` → exit 0.

Expected: Isolated transaction tests cover defer→reopen→correct, mistaken
acceptance→reopen→new answer, stale forms, concurrent actions, actor denial, and
byte-identical prior history. Evaluation tests count one occurrence under each
explicit truth policy.

### 2. Make deferred work and history reachable in the bench

Add paginated pending/deferred/history views with stable ordering and counts.
“Defer” exits the current pending queue but appears in deferred work on reload.
History shows the original reading, human revisions, and an explicit reopen
action. “Undo” is a narrowly labelled reopen/superseding action, not a database
delete. Maintain Arabic keyboard shortcuts and clear 409 recovery.

Retain the current source-crop/editor workflow and focus behavior. Show a local
session summary: accepted/corrected/deferred/rejected counts and remaining work.
Do not show a judge result, shadow completion indicator, or derived confidence
percentage. Operator evidence-gap diagnostics remain outside the reviewer flow.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_GATE=e2e` → exit 0.

Expected: Browser tests defer a fixture, reload, find it, reopen it, save a
correction, and inspect retained history. A wrong-role actor cannot see/reopen it.
Saving and revisiting do not duplicate a person or a truth label.

### 3. Add bounded region adjustment and explicit local reread

Store immutable region revisions in normalized source coordinates, linked to the
original detection and its source checksum/coordinate space. A reviewer may adjust
the tight rectangle, preview its bounded tight/context crops, cancel, or confirm
the revision. Retain the original box and machine readings; a geometry change
invalidates only derived artifacts/requests for that geometry version.

Offer an explicit local reread using Plan 024's adapter. Show raw and processed
variants plus their proposed alternatives and quality reasons, preserving the
source as the authority. A new reading does not fill or accept the human editor
automatically. A final human correction is not sent as a reading prompt or used
to fabricate a machine proposal.

Start with rectangle adjustment and padding controls. Add split/merge only after
the revision path works: create child/derived occurrences with parent lineage,
unique identities, and explicit supersession. Never count parent and split
children together as independent observations in one evaluation cohort.
Children of previously reviewed geometry inherit the non-blind/development
eligibility of that lineage; a new child identifier cannot erase prior knowledge.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_PACKAGES="./internal/sourceprocessing ./internal/httpapi"` → exit 0.

Expected: Tests cover normalized bounds, rotation/coordinate-space consistency,
empty/oversized crops, cancel with no persisted change, cache invalidation,
original evidence retention, and a local-only reread with transport denied.
Split/merge tests verify lineage and evaluation exclusion of superseded geometry.

### 4. Add bounded reviewer assistance around the processing stages

Show actionable reader-stage outcomes: clipped crop, insufficient source pixels,
local reader unavailable, conflicting local readings, or ready for transcription.
Offer the relevant tool for that state: adjust crop, inspect variants, local
reread, enter text, defer, or reject a non-name. Do not present a model score as
correctness or automatically route a region to acceptance.

Keep background-run progress in the source-processing panel, separate from
per-name shadow judging. Do not add model/provider implementation details to the
review flow unless they explain an available action. Bound lookahead and cancel
obsolete visual requests according to Plan 024; referenced durable shadow work
continues according to Plan 025.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_GATE=e2e` → exit 0.

Expected: Keyboard and pointer journeys reach each tool, preserve draft text
across recoverable crop errors, and never apply stale alternatives to a new name.

### 5. Verify the actual end-to-end bench on isolated data

Add a one-command `name-pipeline-verify` target that refuses the real evaluation
database and source-storage directory. Require dedicated test-database and
storage configuration; validate the parsed database name before migration,
seeding, fixtures, or cleanup. Reuse the existing Compose/Go/Playwright stack;
all provider transports are fake/local test doubles.

Cover disabled/configured/transient reader states, prepare→partial→resume,
local alternatives, fast correction before judge completion, shadow invisibility,
defer/reopen/history, region repair, crop failure, rapid navigation, and permission
loss. Assert off versus shadow has the same judge-related visible DOM and editor
behavior. Include a delayed fake provider so evidence joining is exercised.

Add operator status output for counts by stage, timings/cache/operation totals,
version cohort, and missing evidence, with no names, images, secrets, or private
identifiers. Real private benchmark/review runs remain explicitly separate from
test fixtures; no synthetic decision becomes an evaluation label.

**Verify:** Run `make name-pipeline-verify` → exit 0.

Expected: The guarded target passes on its isolated database and fails before
any write when pointed at the evaluation database. Fixtures are cleaned only from
their own test scope. Existing bench helper and privacy tests still pass.

## Commands and done criteria

| Command | Expected |
|---|---|
| `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing ./internal/httpapi ./cmd/name-eval ./cmd/name-pipeline` | pass |
| `npm --workspace apps/web run test && npm --workspace apps/web run typecheck` | pass |
| `env -u DATABASE_URL -u AI_RESEARCH_URL make verify` | pass |
| `make generated-check && make docs-check && git diff --check` | pass |
| `make name-pipeline-verify` with dedicated validated test configuration | all new bench journeys pass, no review-data writes |

- [x] Deferred items survive reload and can be reopened.
- [x] Historical answers remain intact; correction/reopen actions are attributable revisions.
- [x] A bad rectangle can be repaired and reread locally with original evidence retained.
- [x] Split/merge lineage prevents duplicated evaluation observations.
- [x] Manual review is usable when every optional provider is unavailable.
- [x] The actual bench is covered by delayed/failing-provider browser journeys.
- [x] No existing human decision, source original, or class-map byte was overwritten.

## Stops and maintenance

Stop if an “undo” requires deleting an original decision, a region edit changes
source bytes, a test would touch real review data, or a proposed output reveals
shadow work to the reviewer. Do not bypass a failing concurrency or provenance
test to finish the UI. Preserve all existing working-tree changes and leave the
implementation uncommitted without AI attribution.

Future identity/import work must consume a deliberate current human revision,
not merely the latest machine reading. Every new crop/reading/decision revision
must remain traceable through history and report cohort/cutoff selection.

## Implementation notes — 2026-10-28

- Migration `0058_name_review_history_and_region_repairs.sql` adds
  `review_revision`, `geometry_revision`, `superseded_at`/`superseded_by_event_id`,
  decision revision/supersession columns, `source_name_region_revisions`, and the
  append-only `source_name_review_events` table. Applied migrations were not
  edited and the existing 111 actions remain unchanged.
- `tree_suggestions.go` keeps `source_tree_suggestions` as operational state and
  appends `decision` events; `name_review_history.go` adds reopen/history/summary
  and the shared row locks; `name_region_repairs.go` and
  `name_occurrence_repairs.go` add preview/confirm and split/merge lineage.
- The bench now has pending/deferred/history views, a session summary, retained
  history with explicit reopening, bounded region editing, split/merge controls,
  and reader-state assistance. A local reread still never fills or accepts the
  editor, and no hosted fallback is used.
- `name-eval` accepts `--truth-policy first|latest` and requires `--cutoff` for
  latest. One physical occurrence is counted once; collapsed revisions and
  superseded geometry are reported separately.
- `make name-pipeline-verify` runs the full gate and the browser journey against
  the validated `*_pipeline_test` database and temporary storage. The new
  `journeys/11-name-bench.spec.ts` covers defer → reload → reopen → correction →
  history and a wrong-role refusal.
- Verification run: `make name-pipeline-verify` passed (Go packages, web tests,
  migration checks, and 32 browser journeys); the review database still reports
  111 decisions and 11 source files.

## Review fixes — 2026-10-28

- Cutoff reports evaluate supersession at the same cutoff as decisions; a later
  split/merge cannot remove an earlier observation from its pinned report.
- Saves validate the displayed geometry revision and persist it on the new
  decision and event. The web evidence-fallback path retains both revision tokens.
- Legacy reopening retires revision zero without editing its decision. Migration
  0060 also promotes operational tokens for legacy items already reopened by
  0058, preserving their existing decisions and events.
- Migration 0059 adds nullable decision geometry and frozen reread targets.
  New rereads retain their enqueue-time page/rectangle/revision; old runs without
  that input fail explicitly instead of rebuilding it from live geometry.
- Verification: full Go unit suite; guarded database packages (474 passed, one
  existing live-AI test skipped, no database skips); focused legacy migration and
  geometry/cutoff regressions; 101 web tests, typecheck/lint; 32 isolated browser
  journeys; generated/docs checks and whitespace checks. All writes and browser
  fixtures used dedicated test databases/storage. The human review database and
  running services were left untouched; local rollout needs migrations 0059–0060
  followed by an API/worker restart. Work remains uncommitted.
