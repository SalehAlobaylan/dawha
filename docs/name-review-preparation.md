# Counted context and frozen name-review evidence

Plan 025 adds `ar-name-pool-v2`, `local-evidence-v1` generation and the
`name-review-v2` evidence schema. These versions keep new preparation separate
from the original 111 actions. No historical snapshot, human text or missing
judge result is rewritten. Name matching remains a spelling aid, never an
identity assertion or permission to accept a name.

## Review flow

The official workbench first requests a server-frozen review context, computes
its initial pool with the existing TypeScript ranker, and posts that bounded pool
for validation and storage. The editor and source images are held until this
short handshake finishes. It runs in off, shadow and active modes. A failed
preparation releases manual review and records a gap; it does not start another
initial preparation after editing begins.

The server locks the occurrence while freezing the machine reading, independent
local variants, file lexicon revision, frame, region, tight bounds, processing
versions and privacy posture. The same review lock serializes human saves and
lineage history checks. It never holds a transaction while calling the model.
The first exposure is permanent. Completion must occur before the decision and
within its 15-second preparation deadline. Retries return the same preparation;
a different snapshot cannot replace it. Reopening never creates a fresh blind
boundary. Successful original evidence may be reused under its original version
and cutoff, counting the occurrence only once.

The candidate ranker remains in `apps/web/src/lib/arabicNames.ts`; there is no Go
copy of its scoring algorithm. Server validation checks bounded snapshots,
published matching keys, supported source provenance, counts and version
references. Initial generation consumes only the frozen context, never the typed
correction. It uses at most eight ranked chips and twelve total judge options.
Additional independent reader options are explicitly marked unshown, with no
claimed display position. Final interactive chips may respond to typing; their
snapshot and selected chip remain separate interaction evidence.

`source_name_candidate_preparations` stores the initial pool, signature,
provenance and versions. The existing decision ledger stores the final visible
snapshot for new clients. `source_name_decision_evidence` links the roles and
records missing preparation or an active review posture. New initial comparisons
never trust a browser-supplied model result. A saved acceptance references the
frozen machine reading the reviewer saw, even if later processing changes the
queue row.

## Complete counted file context

`GET /api/v1/source-files/{fileID}/name-lexicon` requires source-review permission.
It aggregates the latest accepted/corrected human decision per occurrence in that
file. It does not use undecided machine text or another file's names. Display
variants, matching keys and occurrence counts remain distinct. Response order is
deterministic; the revision hashes the aggregate and cutoff and retains its
original `asOf` time when reused.

The response permits 2,048 matching entries and eight display variants per entry,
with a 1-MiB serialization budget. An aggregation scan cap of 10,000 distinct
spellings, omitted entries, unusable spellings and variant truncation are
explicit. Truncated context is operational evidence and cannot establish a new
blind held-out observation. The ranker accepts weighted file entries directly;
string-array callers still work. Duplicate variants do not manufacture repeated
occurrences. Matching equivalence does not replace the human display spelling.

The common-name list is shared JSON consumed by the TypeScript ranker and the
server's source validator. Local alternatives retain their actual variant and
reader versions. Unsupported/empty readings, unavailable local attempts and
missing geometry remain diagnostic outcomes. No new name is invented by a
visual-confusion transform: a candidate must have existing reader, file or
common-lexicon support. Key ties use stable code-point order.

## Durable invisible judging

After an initial pool is saved, the server may atomically enqueue a local shadow
request. A savepoint isolates optional queue failure: it cannot discard a valid
initial pool. Each occurrence has its own request/result reference, even when two
payloads are identical and a local computation cache can reuse work.

The request contains the frozen reading, at most five alternatives, at most
20 prior file names, numeric crop measurements and at most twelve candidates.
No source/family/account/reviewer identifier, internal request ID or page image
is sent to the provider. Hosted image reading remains disabled. The new durable
path accepts the locally approved Liquid adapter and pins its model/prompt
identity. The worker passes the stored identity as an internal request
precondition; the AI service checks its actual adapter before inference or cache
lookup. Unknown or mismatched identity is rejected without a provider call.
Older AI services reject the new request field rather than ignoring the check.
This transport metadata never enters the provider payload or changes the stored
frozen payload/signature. A mismatch produces a diagnostic gap, not substituted
evidence.

One existing-queue worker bounds inference concurrency. It reads only the frozen
payload, uses lease heartbeats and fences the immutable result write. It does not
query a human answer or rebuild file context after saving. A decision can reference
a request that is still running; completion cannot change that decision. Model
attempts are bounded to 110 seconds and three queue attempts. Unreferenced work
expires before execution after seven days; referenced frozen evidence remains
available. Failed requests remain missing results, never fabricated answers.

Navigation cancels obsolete display requests. It does not cancel durable judge
work. The review UI neither subscribes to shadow status nor fetches shadow output;
there is no shadow progress, reorder, prefill or percentage. Off/shadow DOM
comparison is tested. Ordinary local OCR controls are described in
[the local pipeline guide](local-name-reading-pipeline.md).

## Development and held-out registration

The existing 80 human truth labels are development/regression data. All eleven
current files are excluded from unseen validation, including their related
renderings. Their original ledger and class map remain unchanged. The private
protocol is frozen under `~/.dawha/name-eval/plan025-evaluation-preregistration.json`.
No held-out files or decisions are invented to fill this split.

An operator must assign every rendering of a chart to the same opaque group.
The server additionally checks known matching checksums and source identity.
A new held-out member must postdate installation of this protocol and precede
its first exposure/decision. Groups cannot change partition or version contract.
The contract pins pool, normalizer, evidence, generation, lexicon, renderer,
geometry, preprocessing, reader and judge model/prompt: eleven version fields.
Register the exact source class in a separate frozen held-out class map before
review. Do not edit the pilot's human-confirmed map. This is a provenance
registration; the system does not infer that two otherwise unidentified uploads
are renderings of the same chart.

History anywhere in an occurrence's repair/split/merge lineage, including a
prior deferral, prevents another blind observation. Active review posture also
disqualifies blind evidence. The new queue withholds unprepared reading text for
held-out files; a legacy queue exposure is recorded as untracked and cannot
establish a new blind boundary later. New legacy-client decisions after protocol
installation remain explicit missing-preparation gaps.

## Reopening, deferred work and geometry repair

Plan 026 adds the missing return path. Reopening appends a `reopened` event and
returns the occurrence to pending work; it never edits or deletes the earlier
decision row. The next decision carries `review_revision + 1` and a
`supersedes_decision_id`, so the ledger reads as a sequence: original machine
reading, first human answer, reopen, later answer. The current queue row is only
operational state; history is assembled from the ledger.

Legacy reopening promotes the operational revision-zero token to one, preserving
the original revision-zero decision. Saves carry both the review revision and
the geometry revision shown to the reviewer; stale rectangles return a conflict.
New decisions retain that geometry revision and its region-history reference.

`GET /api/v1/source-files/{fileID}/review-summary` returns counts. The review
queue accepts `status=suggested|deferred|history` (and individual decision
statuses), with stable ordering and offset pagination. A deferred occurrence is
visible after reload; a superseded split/merge parent remains in history but not
in pending work.

A region repair appends an immutable `source_name_region_revisions` row linked to
the source checksum, coordinate space, frame dimensions and analysis DPI. The
original detection rectangle and every reading stay in place; only derived crops
for the new geometry are prepared. `region-preview` is side-effect free on review
state, so cancel persists nothing. Split children and merged children get unique
occurrence identities and lineage edges; the parents are marked superseded and
excluded from pending work and evaluation cohorts.

Local rereads freeze the page, rectangle and geometry revision when queued.
Later repairs cannot change that input, and a revised rectangle gets a distinct
run. Legacy queued rereads without a stored target fail explicitly; request a
new reread rather than reconstructing their inputs from current geometry.

The evaluation policy is explicit. `name-eval --truth-policy first` preserves the
original first-human-decision semantics; `--truth-policy latest --cutoff
<rfc3339>` selects the latest valid revision inside a pinned window. Both count
one physical occurrence, report collapsed revisions and superseded geometry
separately, and never treat a reopen as a new blind observation.
With a cutoff, supersession is evaluated at that same timestamp, so a later
split or merge cannot remove observations from an earlier pinned report.

## Tools and reporting

Browser preparation uses the authenticated session. Operator tools use the same
private session-file and loopback service-identity checks as the local pipeline:

```sh
make name-pipeline-lexicon NAME_PIPELINE_ARGS="--file <file-uuid> --json"
make name-pipeline-evidence-status NAME_PIPELINE_ARGS="--file <file-uuid> --json"
make name-pipeline-evaluation-contract NAME_PIPELINE_ARGS="--file <file-uuid> --contract-file <private-new-file>"
make name-pipeline-evaluation-register NAME_PIPELINE_ARGS="--file <file-uuid> --partition development --group <opaque-sha256> --contract-file <private-contract>"
```

Use `held_out` only for genuinely new groups that meet the protocol. Contract
output is mode 0600 and refuses overwrite. Operator output contains aggregate
counts, never names, tokens or input selectors. Shadow diagnostics are for the
operator and are absent from the review flow.

`name-eval` uses new initial preparation for candidate recall and the matching
server-owned result for judge comparisons. It verifies occurrence/reference,
canonical signatures, versions, frozen context and pre-decision timestamps.
A valid result may finish later; a request created later is rejected. Empty
initial pools are no-candidate outcomes, not missing snapshots. It reports
missing/late/invalid preparation, missing results, truncation, weak-reading
reasons, distinct files/groups and duplicate revisions. A complete contract hash
separates cohorts, including geometry/preprocessing differences. Existing version
selectors remain; new selectors are `--evidence-version`, `--generation-version`,
`--renderer-version`, `--recipe-version` and `--partition development|held_out`.
Development observations cannot satisfy the held-out gate.

The 100-label rate floor, 300-label three-class gate and pre-registered go/no-go
rule are unchanged. Reviewer time still requires a registered baseline or A/B;
shadow timing alone establishes no improvement.

## Private development replay — 2026-10-28

A counterfactual replay used the existing eighty human truths, all from one file,
as development data. Each new pool was computed before consulting that row's
truth, using the original reading/alternatives and only earlier accepted names
from the same file. The shared TypeScript ranker produced the predictions; no
historical preparation, judge output or decision was created or rewritten.

The original top eight contained the answer in **18 of 80** cases; the new
counterfactual top eight contained it in **23 of 80**. These are diagnostic counts
below the rate floor, not a held-out result, accuracy estimate or adoption claim.
There are zero held-out observations. Private inputs and aggregate reports stay
outside the repository. Fresh independently reviewed sources are still needed
for the human evaluation in Plan 022.


The live operational check prepared seven pending occurrences in total, without
creating any human decisions: six local shadow requests completed and one empty
pool made no model call. The final additional preparation explicitly recorded
local-text-only policy, shadow mode and sample 1. All eleven current files were
registered as development through the authenticated tool. New privacy metadata
is never inferred into older immutable context. The preservation audit still
matches all 111 original actions and their candidate/judge evidence.
