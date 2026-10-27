# Plan 024: Prepare local name readings once and resume them as durable jobs

> **Executor instructions:** Work through the stages below in order. Preserve
> source originals and human decisions; machine outputs never become truth.
> Leave the work uncommitted. No AI/agent attribution is permitted.
>
> **Drift check:** Planned against `f0fcb6e` and uncommitted Plan 019/021/022 code.
> Run `git diff --stat f0fcb6e..HEAD -- services/core-api apps/web/src` and inspect
> `git status --short`. Compare the current-state excerpts and Plan 023's final
> contracts with the live code before changing anything.

## Status

- **Priority:** P1
- **Effort:** L — split implementation and review by the numbered steps
- **Risk:** MED–HIGH — job leases, private artifacts, coordinate systems
- **Depends on:** [Plan 023](023-local-reading-capabilities-and-cohorts.md)
- **Category:** processing / performance / tooling
- **Planned at:** commit `f0fcb6e`, 2026-10-27
- **Status:** TODO

## Why this matters

The normal name-reading route processes a whole chart synchronously, while the
API's response timeout is shorter than one possible provider request. Disabling
the hosted reader also aborts the normal path before detected regions are saved.
The review queue used for the current 111 actions was prepared through private
local scripts: a detector plus page-level Tesseract, with a separate crop pass
for screenshots. That is useful preparation, but not a repeatable product tool.

Build a local pipeline with reusable evidence, checkpoints, and explicit outcomes:

```text
immutable original -> source-space render -> regions -> tight/context crops
  -> bounded local variants -> local OCR readings/alternatives
  -> reviewable suggestions -> versioned candidate preparation (Plan 025)
```

More stages are justified where they produce inspectable, reusable evidence.
Do not add another model call merely to make the pipeline longer.

## Current state and conventions

- `internal/httpapi/source_processing_handler.go:438–449` calls
  `h.Service.ReadSuggestions(...)` and waits before returning 201.
- `cmd/api/main.go:118–121` sets `WriteTimeout: 15 * time.Second`.
  `apps/web/src/lib/api.ts:2308` waits 300 seconds for this operation;
  `chart_vision_client.go:106` permits 115 seconds per provider call.
- `tree_suggestions.go:808–866` loops over all pages and saves suggestions only
  after `ReadNamesOnPage` succeeds. In `name_reading.go:203–211`, a non-nil reader
  is invoked and any reading error discards the current tile's returned regions.
- `cmd/api/main.go:105` always constructs a non-nil HTTP vision adapter; pointer
  presence therefore cannot implement a detection-only configuration.
- `suggestion_crops.go:116` reads the entire original; `GetSuggestionCrop:144–174`
  prepares the page and renders both crops. `name_judge.go:376–403` independently
  opens the source even with images disabled. `pdf_extractor.go:153–192` writes
  another temporary original and runs `pdfinfo` and `pdftotext` for each open.
- Existing `jobs.Service.EnqueueTx` in `internal/jobs/service.go`, claim leases and heartbeat fencing
  already solve transactional enqueue and worker recovery. Follow the patterns
  in `sourceprocessing/reprocess.go`, `processor.go`, and
  `cmd/source-processing/main.go`; introduce no new queue infrastructure.
- `carrier_reader.go` already has bounded variants and consensus machinery, but
  its provider is a `ChartVisionProvider`, not a first-class local crop reader.
  Plan 019 §4 already called for a local reader and measured preprocessing.
  This stage finishes that intent instead of declaring it newly invented.

Load-bearing existing shape:

```go
// name_reading.go:203–211
if provider != nil {
    readings, err = ReadCarriers(ctx, provider, decoded, tileRegions, readerConfig)
    if err != nil { return nil, err }
}
```

Product rule from `PRODUCT.md` §4: “AI must not silently convert them into
established genealogical facts.” Save geometry and readings as machine evidence,
with method/version/outcome, independently of a person's eventual decision.

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

- `services/core-api/internal/sourceprocessing/{types.go,tree_suggestions.go,
  name_reading.go,carrier_reader.go,pdf_extractor.go,image_source.go,
  suggestion_crops.go,name_judge.go,reprocess.go}` and their tests
- New local-reader, reading-run, reading-unit, and artifact-store files/tests in
  `internal/sourceprocessing/`
- `services/core-api/cmd/source-processing/main.go`, `cmd/api/main.go`,
  the new cmd/name-pipeline/ directory, and narrowly required job/telemetry type registries
- `services/core-api/internal/httpapi/{router.go,source_processing_handler.go}`
  and tests; `platform/storage/` only if a bounded artifact primitive is missing
- New additive migration(s) after the current highest migration; never edit an
  applied migration. `db/queries/` and generated code only when generation requires it
- `apps/web/src/{types.ts,lib/api.ts,components/SourceProcessingPanel.tsx}`,
  its API tests, and name-bench crop fetch/cancellation code
- `Makefile`, `.env.example`, processing documentation, and plan status rows

**Out of scope:** enabling hosted reading, downloading a replacement model,
modifying source files, autoacceptance, relationships/identity/import rules,
changing historical labels/snapshots, and the candidate-ranker redesign.

## Steps

### 1. Store immutable reading runs, units, and source-space metadata

Add `source_name_reading_runs`, checkpointed `source_name_reading_units`, and
versioned reading/artifact records. A run pins source checksum, page coordinate
space, analysis DPI for PDFs, native raster dimensions, geometry version,
localiser/preprocessing/reader versions, and the privacy policy. Record counts,
stage timings, bounded errors, and `queued/running/partial/succeeded/failed/cancelled`.
`succeeded` means the declared stages completed, never that the names are correct.

Use a separate `name_reading_run_id` relation where needed: the existing
`source_tree_suggestions.processing_run_id` references **source_processing_runs**,
so it must not contain an unrelated name-reading-run identifier. Keep logical
occurrences separate from each run's reading hypotheses. A new reader version
must not rewrite the machine reading attached to an already decided suggestion.

Persist detected geometry before optional transcription. A missing reader leaves
reviewable regions with `reading_unavailable`; illegible ink is `unreadable`;
these are different outcomes. Preserve partial completed units on later failure.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_PACKAGES="./internal/sourceprocessing ./internal/httpapi"` → exit 0.

Expected: Isolated integration tests prove detection survives a reader failure,
decided suggestions are untouched by a rerun, coordinate/DPI mismatches are
rejected, and a repeated unit write is idempotent.

### 2. Return a run promptly and execute through the existing worker

Change the normal read request to an enqueue operation returning 202 and a typed
run summary. Add authenticated run GET, cancel, and retry/resume routes. Use a
transactional enqueue key derived locally from file/checksum/config and scope
recovery to the new job type. Preserve heartbeat/lease fencing on every write.

Do not solve the timeout mismatch by raising the HTTP timeout. The browser polls
ordinary processing-stage progress and can continue reviewing completed regions.
Checkpoint units by page/tile/stage. Retry only failed/incomplete units; do not
redo successful tiles or create duplicate suggestions. Cancel at safe boundaries,
retain completed evidence, and prevent a lost lease from updating run state.

**Verify:** Run `make name-pipeline-db-verify NAME_PIPELINE_TEST_PACKAGES="./internal/sourceprocessing ./internal/httpapi"` → exit 0.

Expected: With a fake reader blocked longer than 15 seconds, enqueue still
responds promptly; worker restart resumes at the last incomplete unit; duplicate
enqueue yields one active run; cancel and lease-loss tests leave no extra writes.

### 3. Reuse private crop artifacts and remove source opens from text-only judging

Derive artifact keys locally from checksum, page/coordinate space, geometry
revision, DPI, crop bounds, renderer version, and preprocessing version. Store
bounded tight/context crops under the configured private source storage, with
retention and byte/pixel bounds. Keep originals read-only. Reauthorize every
artifact fetch against its source even on a cache hit; no public or permanent
unsigned URLs. Coalesce concurrent identical renders.

Persist source-space dimensions and crop measurements. A text-only judge reads
those measurements without opening/decoding the original or running PDF tools.
Do not substitute 300-DPI page-evidence dimensions for a 240-DPI analysis frame.
If metadata is absent, perform one authorized, bounded preparation and save the
correct coordinate-space metadata. The crop endpoint and local reread reuse the
same artifact identity; artifact fingerprints remain internal, not provider IDs.

**Verify:** Run `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing` → exit 0.

Expected: Fake storage/renderer counters show a second identical crop fetch
causes no new render; text-only judging causes zero original reads/PDF subprocesses
after preparation. Changing source checksum, geometry, DPI, or renderer invalidates
the artifact. Another actor without source access cannot retrieve a cached crop.

### 4. Implement a bounded local crop reader and inspectable preprocessing

Introduce a small reader interface for tight crops and a local Tesseract adapter,
with raw transcription, alternatives, variant/config identity, quality metadata,
and completion outcome. Keep the hosted adapter separate and disabled by default.
Use existing executable resolution and bounded subprocess helpers. Never use
Liquid as a reader or route a failed local crop to a hosted model implicitly.

Start with a small, pinned experiment set: raw padded crop; grayscale/local
threshold variant; and a second bounded segmentation configuration. At most three
OCR variants run per crop under one configured budget. For coloured carriers,
test carrier masking as an optional variant while retaining raw ink. Preprocessing
must not erase the original or be described as recovering missing pixels.
Store variant readings and disagreements; do not replace all alternatives with
one guessed string. Keep OCR engine scores distinct from measured view agreement;
neither authorizes acceptance or a correctness percentage.

Route by source name-height/contrast/clipping observations. A source with too few
pixels can go directly to manual review with an honest quality reason. Record
what was attempted and why; do not impose one preprocessing recipe on every class.
Limit CPU workers, subprocess time/output, image bytes/pixels, and retry budgets.

**Verify:** Run `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing` → exit 0.

Expected: Unit fixtures test variant provenance, clipping, empty output,
disagreement, timeout, cancellation, and unavailable Arabic data. No test calls a
live provider. An explicit private development benchmark records counts and
time/bytes/operations by variant without creating or modifying human decisions.

### 5. Expose preparation and diagnosis as tools

Extend the new name-pipeline command / Make targets with authenticated prepare, status,
resume, cancel, and local-reread operations. Operator summaries show stage counts,
errors, cache hits, source-pixel warnings, and timings; they print no names or
private identifiers. A source selector is an internal input, not a provider field.
The web processing panel shows the same run state and retryable stages.

Thread caller `AbortSignal`s through per-name crop/reread API helpers, combining
them with current deadlines. Abort obsolete display work on navigation; retain
stale-response guards. Unsubscribing from a shared durable job must not cancel
another reviewer’s run. Add only a small bounded prefetch window after cache reuse
exists. Durable shadow preparation is specified separately in Plan 025.

**Verify:** Run `npm --workspace apps/web run test && npm --workspace apps/web run typecheck` → exit 0.

Expected: Tests prove stale crop work is cancelled, active work remains usable,
run state survives reload, and a queue can be prepared through the product path
without the private preparation scripts or any hosted call.

## Verification and done criteria

Follow `carrier_reader_test.go` for fake reader call counters and
`reprocess_acceptance_test.go` / job lease tests for isolated run/transaction
coverage. Never put test-generated decisions in the real evaluation database.

| Command | Expected |
|---|---|
| `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing ./internal/httpapi ./cmd/name-pipeline ./internal/jobs` | pass |
| `npm --workspace apps/web run test && npm --workspace apps/web run typecheck` | pass |
| `env -u DATABASE_URL -u AI_RESEARCH_URL make verify` | pass |
| `make generated-check && make docs-check && git diff --check` | pass |
| guarded database verification using a dedicated test database | no data or fixtures in the review database |

- [ ] Local detection survives optional-reader failure and is immediately reviewable.
- [ ] Prepare requests enqueue; restart/resume/cancel are fenced and idempotent.
- [ ] Source/crop work is reused; a text-only judge no longer decodes the source.
- [ ] Local crop reading works with hosted credentials absent and network transport denied.
- [ ] Every hypothesis has an immutable processing/geometry/version identity.
- [ ] The original human labels, snapshots, source checksums, and class map are unchanged.

## Stops and maintenance

Stop if this requires overwriting a human decision, making a private artifact
public, mixing coordinate spaces, or reusing a new model's output as truth.
Leave changes uncommitted, preserve the existing working tree, and never add AI
attribution. No new infrastructure is required: PostgreSQL jobs and the existing
storage abstraction are the intended owners.

Future renderer or OCR upgrades invalidate relevant artifacts and create new
processing versions. Measure local reader quality as an experiment; no model or
preprocessing improvement is guaranteed by this plan.
