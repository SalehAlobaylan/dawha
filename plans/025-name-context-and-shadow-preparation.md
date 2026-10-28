# Plan 025: Build complete file context and prepare shadow evidence before human input

> **Executor instructions:** Preserve the existing 111 review actions and their
> candidate/judge evidence exactly. Any new generation or evidence semantics
> belong to a new explicit version. Leave work uncommitted; no AI attribution.
>
> **Drift check:** Written at `f0fcb6e` with uncommitted Plan 022 code. Run
> `git diff --stat f0fcb6e..HEAD -- apps/web/src services/core-api` and inspect
> `git status --short`; compare these excerpts and the completed prerequisite
> contracts with the live source before implementing.

## Status

- **Priority:** P1
- **Effort:** L
- **Risk:** HIGH — label leakage and evaluation cohort integrity
- **Depends on:** [023](023-local-reading-capabilities-and-cohorts.md) and
  [024](024-durable-local-name-reading.md)
- **Category:** candidate quality / evaluation / processing
- **Planned at:** commit `f0fcb6e`, 2026-10-27
- **Status:** DONE

## Why this matters

The current sample has 80 accepted/corrected names; the initial saved top-eight
list contains the human answer in 18 cases. Counts below the 100-label rate floor
are diagnostic examples, not a quality estimate. A judge cannot choose an answer
that never entered its bounded pool, so generation must improve before spending
more effort on ranking.

The bench also discards file-name frequencies and starts judging only when a
name becomes active. One of the current 80 truth labels lacks judge output.
The solution must make evidence preparation durable without asking reviewers to
wait for an invisible model or letting their final answer enter its own input.

## Current state and evidence

- `SourceSuggestionReview.tsx:230–238` loads only 200 accepted and 200 corrected
  suggestions, then `uniqueNames` removes duplicate normalized spellings.
  `rememberName:551–554` removes duplicates again and caps the list at 500.
- `arabicNames.ts:244` increments `fileCount` for repeated file occurrences;
  lines 270 and 281 initialize/use that count. The actual UI supplies deduplicated
  names, so this signal is effectively one per spelling. The accepted/corrected
  query is ordered by machine confidence (`tree_suggestions.go:306`), not complete
  lexical coverage or correction recency.
- `SourceSuggestionReview.tsx:493–500` freezes the shadow pool from the initial
  machine reading; keep that anti-leakage property. Interactive chips can continue
  changing while the reviewer types.
- The judge effect waits 600 ms and then calls the model (`:508–549`). Submit
  attaches output only if the matching result has already returned (`:614–631`).
  Fast review therefore creates a measurement gap despite a real human label.
- `name_judge.go:389` reads live file context for every request. Merely moving
  that function into a worker after saving would leak the newly accepted or
  corrected name into its own judge context.
- Migration 0053 stores one `candidate_snapshot`, selected candidate, versions,
  duration, and `judge_output`. Under current shadow mode the snapshot represents
  the initial pool; active/off paths use the decision-time visible pool. A new
  evidence schema must make those roles explicit rather than silently conflating them.

Relevant excerpts:

```tsx
// SourceSuggestionReview.tsx:235–238
const names = [...accepted, ...corrected]
  .map((suggestion) => suggestion.decidedTextAr || suggestion.suggestedTextAr)
  .filter(Boolean);
setKnownNames(uniqueNames(names));
```

```go
// name_judge.go:389–397
fileContext, err := s.judgeFileContext(ctx, source.sourceFileID)
// ... request state includes that context, not just the candidate list.
```

Follow `nameReviewEvidence.test.ts`, `arabicNames.test.ts`,
`name_judge_test.go`, and `cmd/name-eval/main_test.go` for keys, fake transports,
snapshot validation, and offline paired-report tests. Keep the ranker algorithm
in one implementation; this plan does not call for a second independent Go copy.

`PRODUCT.md` requires provenance and human-governed truth. The Plan 022 rule
requires 300 qualified shadow decisions across three registered classes,
measurable recall at least 70%, a 10-point top-1 gain with a positive paired
interval, better absent-truth abstention, no increase in wrong answered top-1,
file/rare-name coverage, time non-inferiority, and exact privacy approval. Those
requirements are not reduced because the pilot is small.

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

- `services/core-api/internal/sourceprocessing/{tree_suggestions.go,name_judge.go,
  types.go}` and tests; new file-lexicon, candidate-preparation, and judge-request
  files/tests in the same package
- `services/core-api/internal/httpapi/{router.go,source_processing_handler.go}`
  and tests; worker dispatch and the new cmd/name-pipeline/ directory
- `services/core-api/cmd/name-eval/`
- New additive migrations after the current highest migration; relevant queries
  and generated output only when required
- `apps/web/src/{lib/arabicNames.ts,lib/nameReviewEvidence.ts,lib/nameJudge.ts,
  lib/api.ts,types.ts,components/SourceSuggestionReview.tsx}` and matching tests
- Relevant evaluation documentation, Make targets, and plan status rows

**Out of scope:** modifying existing human labels/snapshots, backfilling the
missing old judge result, admitting arbitrary invented names, a model-trained
ranker, autoacceptance, active ordering, confidence percentages, hosted images,
and identity/relationship inference.

## Steps

### 1. Serve a complete, bounded file lexicon with counts

Add authenticated `GET /api/v1/source-files/{fileID}/name-lexicon`, using the
normal source-review authorization. Aggregate only human accepted/corrected text
from that file. Return display spellings, matching keys, occurrence counts,
spelling variants, and an immutable/as-of context revision. Do not include machine
readings that a human never accepted, other files, or account/family data.

Define a deterministic bounded response and explicit truncation metadata; do not
silently read the first 200 decision rows. Preserve display variants and distinguish
matching equivalence from identity. Context sent to Liquid still has its existing
20-name bound; a larger internal lexicon is not permission to send an entire file.

Adapt `NameCandidateGroups` to consume explicit file counts without manufacturing
duplicate strings. Retain string-array compatibility where existing callers need
it. Bump the candidate-pool version; store the lexicon revision in new evidence.
Apply new context to the next initial pool, never mutate a pool already frozen.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_PACKAGES="./internal/sourceprocessing ./internal/httpapi"` → exit 0.

Expected: Fixtures with over 200 corrections and repeated spellings retain the
expected counts after reopening the bench. Unaccepted text and another actor's
private file never enter the response. Ranker tests prove counts affect the
specified tie-break without erasing source display spelling.

### 2. Generate candidates from local readings and bounded evidence

Combine Plan 024's independently recorded local alternatives, the file lexicon,
and the existing common-name list. Keep at most eight ranked chips and twelve
total bounded judge options. Allow visual-confusion variants only when supported
by an existing lexicon/reader entry. Keep no-candidate/manual-entry as real outcomes.

Version every generation recipe and record which source/variant supplied each
candidate. Add explicit weak/unsupported-reading reasons for diagnostics; do not
turn an uncalibrated score into an automatic name decision. Do not insert the
reviewer's typed correction into the **initial evaluation pool**.

**Verify:** Run `npm --workspace apps/web run test && npm --workspace apps/web run typecheck` → exit 0.

Expected: Regression fixtures cover missing-truth cases, competing spellings,
rare/file names, duplicate alternatives, empty readings, and bounds. Existing
normalization/display tests still pass. All new comparisons select a new cohort.

### 3. Store initial preparation separately from final visible evidence

Add immutable candidate-preparation records and a new evidence-schema version.
They record the initial candidate snapshot/signature, machine reading and
alternatives, exact file-context revision, coordinate/crop metadata, all relevant
versions, privacy posture, and creation time. Capture this before reviewer input
in off/shadow/active modes; judging is optional, initial preparation is not truth.

The final decision-time visible candidate snapshot and selected chip remain
separate interaction evidence. Evaluation uses the initial preparation, not a
post-typing list. A selected chip that appeared later may be recorded in the
visible snapshot without being retroactively added to the baseline pool.
Keep the existing ledger fields/legacy meaning readable; do not migrate the old
111 actions into a new interpretation or infer unavailable provenance.

Use a short authorized transaction to freeze the entire bounded request context
while the suggestion is still undecided. If a decision wins the race first,
reject late preparation; retain an honest evidence gap. Do not read live file
context later to reconstruct what might have been available earlier.

For **blind** eligibility, operational `suggested` status is insufficient. Pin
the initial preparation to the occurrence's first review exposure, before human
editing, and require no prior human decision anywhere in that occurrence's
geometry/split/merge lineage. Capture/freeze before returning the initial review
preparation to the editor. If preparation is unavailable, manual review still
proceeds and the missing initial evidence stays a gap. Never establish a new
blind boundary by reopening the same occurrence.

Reopened or repaired occurrences may reuse eligible original frozen evidence
under the original cohort/cutoff, or produce new **operational/development**
preparations. Their prior accepted/corrected text must not qualify as unseen
truth merely because a new request predates a second decision. A prior deferral
also creates review history; do not treat its reopening as a fresh held-out
exposure. Classify eligibility explicitly and retain the reason.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_PACKAGES="./internal/sourceprocessing ./internal/httpapi"` → exit 0.

Expected: A correction equal to a common/file name changes the final visible
snapshot but cannot alter the stored initial pool. Preparing after a committed
decision fails; preparing before the decision retains exactly its pre-decision
file context. Legacy snapshots remain byte-identical.
Add accept→reopen→prepare, defer→reopen→correct, and labelled-parent→split-child
tests: none creates a new blind observation or admits its own known answer into
an eligible initial context.
At this stage, use isolated fixture/fake review-history and geometry-lineage
metadata to test the eligibility rules. The actual reopen/split endpoints and
their full browser journeys are implemented and tested in Plan 026; do not make
Plan 025 depend on those later UI/API operations.

### 4. Run shadow judging durably from the frozen request

Add a server-owned immutable judge-request/result relation tied to the initial
preparation. Persist its bounded input before enqueuing a local judge job. An
internal request identifier may be passed within Dawha, never to the provider.
The worker reads the frozen payload only; it does not query the latest human
answer or rebuild file context. The provider payload remains text-only and
contains no source/family/account/reviewer identifiers or page images.

A future decision can reference this request even if the model is still running.
The result may finish after the decision because the complete input was fixed
beforehand. The evaluator joins by reference/signature/version and verifies
preparation time precedes the human decision. It must not trust a browser-written
model result as a replacement for the server-owned result. Failed requests remain
diagnostic gaps; no invented output or label is inserted.

Return preparation acceptance promptly and keep review non-blocking. Do not show
shadow status, progress, output, reordering, prefill, or a percentage. Bound
preparation concurrency and any lookahead window. A referenced durable request
continues after navigation; cancellation of obsolete display requests must not
erase evaluation evidence. Speculative unreferenced jobs may expire under an
explicit retention rule. Deduplicate **retries within one occurrence and initial
preparation**, not distinct physical occurrences. Identical payloads may share a
bounded local computation cache, but each occurrence retains its own authorized,
timestamped preparation/request/result reference and eligibility.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_PACKAGES="./internal/sourceprocessing ./internal/httpapi"` → exit 0.

Expected: With a deliberately delayed fake model, a human decision saves
immediately and later resolves to exactly one pre-decision judge request. A model
completion cannot change the human text. A context name accepted after freeze is
absent from the captured provider body. Shadow/off visible DOM remains equivalent.
Two distinct physical occurrences with identical bounded payloads retain two
evidence identities and count twice; retries or revisions of one count once.

### 5. Compare versions with development and held-out labels

Use the current 80 human labels as development/regression examples if inspecting
their corrections to change the pipeline. Record that use explicitly; do not
describe the same tuned-on labels as unseen validation. Pre-register a later
held-out batch/source split and a coherent version cohort before reviewing its
report. Keep the human-confirmed class-map bytes and original cohort intact.
Keep held-out source/rendering groups disjoint from development groups; another
rendering or split descendant of a labelled chart is not an unseen validation file.

Extend the offline report to use initial preparation for candidate recall and
the matching immutable judge output for comparisons. Report missing/late/stale
preparations, version mismatches, truncation, no-candidate cases, and distinct
source coverage. Preserve the 100-label rate floor and 300-label class gate.
Reader time still needs a baseline/A/B comparison; shadow timing alone proves
no reduction. Store private reports outside the repository and never print names.

**Verify:** Run `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./cmd/name-eval` → exit 0.

Expected: Report tests reject post-decision inputs, mismatched signatures,
mixed cohorts, duplicate retry/revision counting, and missing metadata. The missing old output
remains missing. A held-out cohort cannot include examples declared development.

## Verification and done criteria

| Command | Expected |
|---|---|
| `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing ./internal/httpapi ./cmd/name-eval ./cmd/name-pipeline` | pass |
| `npm --workspace apps/web run test && npm --workspace apps/web run typecheck` | pass |
| `cd services/ai-research && .venv/bin/pytest && .venv/bin/ruff check .` | pass, fake transports only |
| `env -u DATABASE_URL -u AI_RESEARCH_URL make verify` | pass |
| `make generated-check && make docs-check && git diff --check` | pass |
| database/E2E verification against a guarded dedicated test database | no fixtures in the real review ledger |

- [x] File counts and complete intended context survive bench reload.
- [x] New candidate recipes have explicit versions and bounded, traceable sources.
- [x] Initial evaluation pools and final visible pools have explicit, separate roles.
- [x] A fast human decision does not require waiting for an invisible judge.
- [x] Deferred model completion uses only input frozen before the human answer.
- [x] Reopening/repair never turns known truth into a new blind observation;
  distinct occurrences remain distinct even when computation payloads match.
- [x] Existing data is unchanged; new cohorts cannot silently mix with it.

## Stops and maintenance

Stop if the implementation requires rebuilding old requests after labels were
known, presenting tuned-on labels as held-out, weakening source authorization,
or exposing shadow work to the reviewer. Keep original decisions append-only;
no commit/push/PR or AI attribution is authorized.

Changes to readers, preprocessing, lexicons, normalizers, prompts, or model
versions create new evaluation cohorts. Preserve enough bounded provenance to
reproduce the exact input without rereading live human context.


## Implementation record — 2026-10-28

Implemented against `3de1de1` with all prior uncommitted work preserved. Migration
0057 adds new evidence relations beside the original ledger; it does not alter
or backfill any original decision. Work remains uncommitted.

- The authenticated file lexicon aggregates latest human decisions across the
  complete intended file context, retains occurrence counts and display variants,
  and persists a stable revision/as-of record. Limits are explicit: 2,048 keys,
  eight variants, 10,000 scanned distinct spellings and a 1-MiB response budget.
  Truncation/unusable spellings remain diagnostics and cannot silently qualify
  blind context. The weighted TypeScript input retains string-array compatibility.
- `ar-name-pool-v2` / `local-evidence-v1` combine the original machine reading,
  independently stored local variants, counted file context and the shared common
  dictionary. The scoring implementation remains TypeScript only. It uses eight
  ranked chips, twelve bounded judge options and stable key ties. Server-side
  validation records supported candidate/variant provenance without inventing
  strings or using the typed correction as initial input.
- The first-exposure handshake freezes context, validates/stores the initial
  snapshot, then releases the editor/source display. Manual review continues on
  failure with an honest gap. The `name-review-v2` relation separates initial
  preparation from the existing final-visible snapshot/selected-chip ledger.
  File/occurrence locking serializes saves and freeze; actual decision timestamps
  use the write clock. Late preparation is refused. New records retain explicit
  local-text-only privacy posture, judge mode and sampling metadata.
- Server-owned frozen requests/results use the existing PostgreSQL queue,
  heartbeat and lease fence. One worker bounds inference, retries are capped,
  and an optional outbox savepoint preserves the initial pool if enqueue fails.
  Results may finish after saving but cannot modify human text or rebuild live
  context. Distinct occurrences retain distinct evidence even for identical
  payloads; retries/revisions count once. Shadow has no visible subscription,
  status, result, ordering, prefill or percentage.
- Immutable source/rendering-group registration separates development and held-out
  data. It pins eleven version fields before exposure. Known old checksums/source
  identity, previous decisions, deferrals, lineage, active posture and legacy
  exposure all fail closed for new blind observations. Old legacy evidence stays
  readable; new legacy-client decisions cannot bypass the installed boundary.
  Actual reopen/repair endpoints remain Plan 026 work, with their metadata rules
  covered by isolated fixtures here.
- Operator lexicon/evidence summaries and private contract/enrollment commands use
  authenticated loopback Core API identity checks and redact selectors/names.
  The evaluator reads the immutable initial pool and matching server result,
  validates signatures/timing/versions, reports gaps/truncation/source groups,
  and separates complete recipe hashes and evaluation partitions. It preserves
  the original rate, class, paired-comparison and reviewer-time rules.

The private protocol declares all eleven current files and the eighty existing
truth labels development data; their related renderings cannot become unseen
validation. Held-out files/labels remain empty rather than being invented. The
human-confirmed class map is unchanged. A chronological counterfactual development
replay used the original readings/alternatives and only earlier human file names:
its top eight contained the truth in 23 of 80 cases versus 18 of 80 original
snapshots. These below-floor diagnostic counts establish no held-out quality.
No old request or missing judge result was reconstructed.

The product-path smoke prepared six pending occurrences across the registered
classes. Five had options and completed actual local shadow requests; one retained
an empty initial pool and made no judge call. A final additional operational
preparation verified explicit privacy metadata and completed its local request.
Request bodies were checked for the exact bounded text-only shape: no internal
IDs or images. All eleven development registrations used the normal authenticated
operator path. These actions created zero human decisions.

Initial validation exposed an HTTP fixture expecting 200 instead of the existing
201 decision response, a missing 409 mapping for late preparation, and an older
recovery fixture expecting immediate recovery despite `SKIP LOCKED`. The status
mapping is fixed. The recovery fixture now revokes its forced-expired token and
asserts bounded eventual recovery, matching the worker's retry contract. Failed
isolated logs/databases were retained; no assertions or database audits were
weakened. The final isolated database was reused only after confirming its
synthetic actors had already been cleaned up; it was never dropped or reset.

| Check | Result |
|---|---|
| Guarded `verify-full` | passed: 985 tests, 979 passed, 6 optional skips, 0 database skips; fixture audit passed |
| Final sourceprocessing/HTTP/evaluator/operator database gate | passed: 426 tests, 425 passed, 1 optional skip, 0 database skips; includes final privacy-metadata assertion |
| Web tests, ESLint, typecheck/build | 95 passed; all checks passed |
| Python and Ruff | 86 passed; lint passed |
| Guarded browser gate, separate test DB/storage | 30 journeys passed |
| Migration runner / AI evaluation | all five cases and all configured baselines passed |
| Test environment guard | 12 passed |
| `env -u DATABASE_URL -u AI_RESEARCH_URL make verify` | passed with the final changes |
| `generated-check`, `docs-check`, `git diff --check` | passed |
| Review/corpus preservation | all 111 original row/snapshot/judge fingerprints, source checksums, class-map bytes and HEAD unchanged; 12 recorded originals verified |
| Current read-only evaluation | 111 actions / 80 development truths; 0 current held-out truths; `ready=false`; original historical 79 complete shadow labels remain stored |

The app is running with shadow mode, sample 1, images false, local OCR and hosted
image reading disabled. See [the operating protocol](../docs/name-review-preparation.md).
Plan 025 is complete; Plan 026 is next. Plan 022's human measurement remains pending.


### Shared-workspace handoff

During the final post-completion check, a separate Cursor process began Plan 026
edits in the same checkout (migration 0058 and `name_review_history.go`, with
changes to `tree_suggestions.go`). Those changes were preserved. The passing
Plan 025 gates above cover migrations through 0057; the final combined-tree
`generated-check` detects the new 0058 schema before its generated output is
updated. Plan 026's owner must regenerate and verify that combined tree. No 0058
migration was applied to the review database by this implementation; the running
app and final 111-row preservation audit remain at the completed Plan 025 state.
