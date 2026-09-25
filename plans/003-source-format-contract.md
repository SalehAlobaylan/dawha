# Plan 003: Align the V1 source upload and extraction contract

> **Executor instructions**: Follow the steps and run every verification gate. Stop rather than silently expanding the product contract.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW–MED
- **Depends on**: none
- **Category**: correctness
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

The upload API currently accepts PDF, JPEG, PNG, TIFF, and octet-stream, then enqueues the file. The only implemented extractor accepts text, JSON, and XML. Historical source uploads therefore appear successful and consume storage/retries before failing. This contradicts the Phase 14 contract in `IMPLEMENTATION_PLAN.md:1133-1190` and the V1 user expectation.

## Current state

- `services/core-api/internal/sourceprocessing/upload.go:135-153` accepts the binary formats.
- `services/core-api/internal/sourceprocessing/extractor.go:19-21,69-72` rejects every non-text content type.
- `services/core-api/internal/httpapi/source_processing_handler.go` enqueues after upload and returns a queued processing view.
- `services/core-api/internal/sourceprocessing/extractor_test.go:41-49` already documents PDF as unsupported.
- The plan's pipeline includes text/OCR extraction, page segmentation, and traceable passages; only text extraction is implemented.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Focused | `go test ./internal/sourceprocessing ./internal/httpapi -count=1` | all pass |
| Full | `make verify` | exit 0 |
| DB | `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./... -count=1` | all pass |

## Scope

**In scope**:
- `services/core-api/internal/sourceprocessing/upload.go`
- `services/core-api/internal/sourceprocessing/extractor.go`
- `services/core-api/internal/httpapi/source_processing_handler.go`
- `apps/web/src/components/SourceProcessingPanel.tsx`
- Upload/extractor tests and API contract tests

**Out of scope**:
- Building a new OCR vendor or model integration.
- Changing passage/candidate schema without a migration design.
- Job lease work from plan 006.

## Git workflow

- Branch: `advisor/003-source-format-contract`.
- Commit one contract change and one test/UI adjustment; do not push unless instructed.

## Steps

### Step 1: Choose and document the V1 format matrix

Make an explicit decision: either (recommended for current V1) accept only `text/*`, `application/json`, and `application/xml`, or implement bounded PDF/OCR extraction before advertising binary support. Record the decision in the API error message, UI copy, and tests.

**Verify**: add a table-driven test enumerating every supported and rejected MIME type. Run `go test ./internal/sourceprocessing -count=1` → all pass.

### Step 2: Reject unsupported uploads at the boundary

If choosing text-only V1, return a validation/unsupported-media error before storing the object or enqueueing a job. Keep filename, size, UTF-8/content validation, and safe storage-key handling. Do not trust only the caller-supplied MIME string if a real content sniff is available.

**Verify**: handler tests assert unsupported uploads create no `source_files` row, no object, and no job. Run `go test ./internal/httpapi ./internal/sourceprocessing -count=1` → all pass.

### Step 3: Align worker and UI behavior

Update the worker error/status path and Source Processing UI so users see the supported formats before upload and a clear failure reason if an older queued file is unsupported. Preserve page/locator provenance for accepted text files.

**Verify**: `npm run typecheck && npm run lint` → exit 0.

### Step 4: Add migration/regression coverage

Add a test for a legacy queued binary file: it must fail deterministically, remain auditable, and not be reported as succeeded. No schema migration is required unless the chosen design adds a format capability field.

**Verify**: `make verify` and DB-backed `go test ./... -count=1` → all pass.

## Test plan

- Every accepted text MIME type succeeds and creates a job.
- PDF/image/octet-stream rejection at the HTTP boundary.
- Size, filename, invalid UTF-8, and mismatched content tests.
- Legacy queued unsupported file failure and status.
- UI copy/typecheck.

## Done criteria

- [ ] Advertised formats equal implemented formats.
- [ ] Unsupported uploads are rejected before storage/enqueue.
- [ ] No queued binary upload can be reported as successful.
- [ ] `make verify` and DB tests pass.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if implementing OCR requires an unreviewed external service, credential, or data-sharing decision.
- Stop if a MIME check would reject a currently documented supported format without an explicit product decision.
- Stop if verification fails twice.

## Maintenance notes

Keep one shared format matrix for API, worker, UI, and documentation. Any future OCR addition must preserve page numbers, offsets, and candidate traceability.
