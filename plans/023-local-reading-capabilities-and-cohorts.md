# Plan 023: Make reader availability explicit and keep evaluation versions separate

> **Executor instructions:** Implement this stage before changing the name reader or
> candidate pool. Preserve the current human review ledger and private corpus. Leave
> all work uncommitted. Never add AI or agent attribution to a commit or PR.
>
> **Drift check:** This plan was written at HEAD `f0fcb6e` with substantial uncommitted
> Plan 018–022 code. Run `git diff --stat f0fcb6e..HEAD -- apps/web/src services/core-api services/ai-research`
> and `git status --short`, then compare the excerpts below with the live files.
> A clean commit diff does not account for the existing uncommitted implementation.

## Status

- **Priority:** P1
- **Effort:** M
- **Risk:** MED — configuration and evaluation cohort boundaries
- **Depends on:** the current uncommitted Plan 019/021/022 implementation
- **Category:** correctness / tooling / evaluation
- **Planned at:** commit `f0fcb6e`, 2026-10-27
- **Status:** DONE
- **Next stages:** [024](024-durable-local-name-reading.md), then
  [025](025-name-context-and-shadow-preparation.md) and
  [026](026-name-review-repair-and-verification.md)

## Why this matters

The reviewer completed 111 actions: 9 acceptances, 71 corrections, 14 deferrals,
and 17 rejections. There are 80 human truth labels, 79 with complete shadow
evidence; all are in `calligraphic_dense`. The correct name appears in the saved
top-eight candidates for 18 of those 80 labels. These are diagnostic counts,
below the evaluation's rate floor, and do not establish model quality.

The hosted image reader was deliberately disabled for the collection session,
but its button still appears available. Meanwhile, the local text-only Liquid
judge remains configured. The product must distinguish those capabilities and
must preserve evaluation versions before improving processing.

The user selected a staged plan for the full local pipeline. This is stage 1;
it does not require collecting another 300 reviews before engineering can start.

## Current state and evidence

- `apps/web/src/components/SourceSuggestionReview.tsx:1128–1139` renders the
  image-reader action unconditionally; its only disabled condition is
  `disabled={aiLoading}`. At line 1346, every 503 becomes a configuration error.
- `services/ai-research/app/chart_vision.py:387` refuses calls without a key;
  lines 659–687 automatically try an environment value and then Keychain.
  The collection session currently disables this using a private runtime guard
  outside the repository. Replace that workaround with explicit configuration;
  do not copy private runtime files into the repository.
- `services/core-api/internal/sourceprocessing/name_judge.go:327–337` exposes
  judge configuration only. `Available: s.NameJudge != nil && settings.Mode != "off"`
  is configuration, not a provider-health check or image-reader capability.
- `services/core-api/internal/sourceprocessing/carrier_reader.go:372–403`
  retries a failed batch as individual crops for every error. The client already
  distinguishes `ErrChartVisionSetup` at `chart_vision_client.go:237–245`.
- Migration 0053 persists `candidate_pool_version`, but
  `cmd/name-eval/main.go:352–373` does not select it. The report can therefore mix
  future pool/reader versions unless this boundary is added.

Relevant code shapes:

```tsx
// SourceSuggestionReview.tsx:1132–1137
onClick={() => void requestAICandidates()}
disabled={aiLoading}
// ... label: اقترح بالذكاء الاصطناعي
```

```go
// carrier_reader.go:372–386
batch, err := provider.ReadBatch(ctx, inputs[start:end])
if err == nil { /* keep readings */ }
// Any error currently falls through to one request per crop.
for _, input := range inputs[start:end] { /* ... */ }
```

Follow the existing fake-HTTP test patterns in `name_judge_test.go` and the
environment-validation pattern `TestNameJudgeSettingsFailClosed`. Browser API
contracts use `vi.stubGlobal("fetch", ...)`, as in `nameJudgeApi.test.ts`.

## Product and privacy boundaries

`PRODUCT.md` §4 says: “AI must not silently convert them into established
genealogical facts.” An OCR reading is a hypothesis; only a real human supplies
the evaluation truth. Liquid remains a judge over 1–12 supplied candidates or
`unclear`, not an OCR reader.

- Keep `NAME_JUDGE_IMAGES=false`. No source/family/account/reviewer identifiers,
  page images, session credentials, or application keys go to a model provider.
- Local processing does not authorize hosted processing. No automatic hosted
  fallback and no credential discovery when hosted reading is disabled.
- Shadow remains invisible: no progress indicator for its work, choice, changed
  order, prefill, or percentage. Reader capability information is independent of
  judge mode and may be shown consistently in off/shadow/active modes.
- Do not rewrite the 111 actions, reconstruct the missing judge output, or alter
  the frozen private class map. The original Plan 022 gate remains unmet.

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

- `services/ai-research/app/chart_vision.py`, `app/main.py`, and their tests
- `services/core-api/internal/sourceprocessing/types.go`, `chart_vision_client.go`,
  `carrier_reader.go`, and corresponding tests
- New `sourceprocessing/reading_capabilities.go` and tests
- `services/core-api/internal/httpapi/router.go`, `source_processing_handler.go`,
  and their tests; `services/core-api/cmd/api/main.go`
- `services/core-api/cmd/name-eval/main.go` and `main_test.go`
- New command and tests at the planned path:

  ```text
  services/core-api/cmd/name-pipeline/main.go
  ```
- `apps/web/src/types.ts`, `lib/api.ts`, API contract tests,
  `components/SourceSuggestionReview.tsx`, and narrowly related styles
- `.env.example`, `package.json`, `Makefile`, `README.md`, relevant evaluation
  documentation, and these plan status rows
- New test-environment guard and unit tests at the planned path:

  ```text
  infra/local/check-name-pipeline-test-env.py
  ```

  Also register these plan files in `infra/local/check-doc-links.py`.
  The shared-service reuse guard also touches `infra/local/migration_test.sh`,
  because its direct Compose startup must not restart the review database
  container when a test database name is present.

**Out of scope:** provider enablement for private files, new OCR/model downloads,
reading-quality changes, history rewriting, job execution, identity/relationship
inference, and other application areas.

## Steps

### 0. Install the guarded integration runner before any database-writing test

Add `make name-pipeline-db-verify`, accepting explicit
`PIPELINE_TEST_DB_NAME`, `PIPELINE_TEST_DATABASE_URL`, and
`PIPELINE_TEST_STORAGE_DIR`; use `NAME_PIPELINE_TEST_PACKAGES` for the package list.
Support a whitelisted `NAME_PIPELINE_TEST_GATE=packages|verify-full|e2e` (default
`packages`). The latter two call the existing Make targets only after validation,
with Compose database name, database URL, and storage derived from the same
approved test configuration. Do not accept an arbitrary shell command as a gate.
Validate URL scheme, local host, exact URL/name equality, the `_pipeline_test`
database suffix, and a dedicated temporary storage directory before invoking any
database client. Reject URL query strings/fragments, including database, host,
or service overrides; validation must describe the database the driver will
actually use. Reject the real review database and source-storage directory.
Unset any inherited `DATABASE_URL`/`SOURCE_STORAGE_DIR`, then derive both only
from the validated test variables. Clear inherited PostgreSQL host/database/
service/options connection overrides from child environments; authenticate using
the explicit test configuration. Call the existing `tools/dbtestguard` so
integration skips cannot be reported as a pass. The target must not create or
drop databases automatically; migrate a dedicated test database only after this
validation. Unit commands below explicitly unset database/AI-service variables.

Register plans 023–026 in the documentation checker's explicit document list.
Do not add a reviewed exception for a real broken citation.

**Verify:** `test -f infra/local/test_name_pipeline_test_env.py && python3 -m unittest discover -s infra/local -p 'test_name_pipeline_test_env.py'`
→ exit 0. Cover missing variables, production/default database, URL/name mismatch,
query/fragment database/host/service overrides, inherited connection overrides,
nonlocal host, reused real storage, and valid dedicated configuration. Rejected
configurations must fail before a test database connection or fixture write.

### 1. Add explicit reader policy and source-authorized capabilities

Define and validate `NAME_READER_MODE=off|local|hosted`; default to local intent,
with no hosted fallback. Add a separate hosted-reader enable flag that defaults
false. A disabled hosted reader must not look in Keychain or load a hosted key.
Preserve the existing hosted adapter behind the explicit policy boundary.

Add authenticated `GET /api/v1/source-files/{fileID}/reading-capabilities`, using
the same source access check as the crop endpoint. Return typed local-reader
configured/ready/reason/version/language fields and hosted-reader
enabled/configured/allowed-for-source/reason fields. Separate configuration from
bounded, cached health checks. At this stage, report an unimplemented local
adapter honestly; installed Tesseract alone is not a completed reading path.

Use stable reason codes such as `disabled_by_policy`, `not_configured`,
`local_reader_not_implemented`, `tool_missing`, `language_missing`, and
`temporarily_unavailable`. Never include a key, account identity, or provider
exception text. Do not send a source file to a model to test readiness.

**Verify:** Run `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing ./internal/httpapi` → exit 0. Go and Python unit tests cover disabled mode with credentials present,
malformed mode, unavailable service, missing Arabic data, source-access denial,
and no outbound hosted transport in local/off modes; all pass.

### 2. Make the action tell the truth before the reviewer clicks

Fetch reader capabilities separately from `fetchNameJudgeConfig`. Disable the
hosted image action with an Arabic reason when it is unavailable. Do not label
the local text judge as an image reader. Distinguish unavailable configuration,
policy denial, transient outage, and an unreadable crop in API error codes and
Arabic copy. Keep manual correction usable through all these states.

Reserve a separately named “إعادة القراءة محلياً” action for the actual local
reader implemented in Plan 024; keep it disabled until that adapter is ready.

**Verify:** Run `npm --workspace apps/web run test && npm --workspace apps/web run typecheck` → exit 0.

Expected: Web API contract tests and component/browser tests assert the disabled
state before any click, no accidental provider request, and identical shadow/off
visibility for judge-related content. `npm --workspace apps/web run typecheck`
and `npm --workspace apps/web run test` exit 0.

### 3. Fail permanent errors once; retry only failures that can recover

In `readInBatches`, return immediately on setup/policy/authentication failures
and context cancellation. Use a bounded retry budget for transport/rate-limit
failures. Keep per-crop fallback only for a batch whose content/output validation
can plausibly isolate a bad crop. Preserve the distinction between a service
failure and ink the reader cannot read.

**Verify:** Run `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing` → exit 0.

Expected: A fake provider receiving eight crops gets exactly one call for a
setup failure, no further calls after cancellation, and bounded calls for
transient failures. A malformed single crop still permits other crops to survive.

### 4. Add operator diagnostics and cohort selection

Add `make name-pipeline-doctor` through `cmd/name-pipeline doctor --json`. It
reports local tool versions/Arabic-language presence, configured provider policy,
bounded service reachability, renderer/reader/pool versions, and aggregate
evidence gaps. It never prints source/reviewer identifiers, names, tokens,
images, or request bodies. Return exit 0 when required local capabilities are
ready, 2 for missing/invalid local prerequisites, and 3 for an unreachable required
service. No model inference or downloads occur in this command.

Extend `name-eval` to load pool version, immutable reader version, judge model,
and prompt version. Add `--pool-version`, `--reader-version`, `--judge-model`,
and `--prompt-version`. Report cohorts separately; if selected rows mix versions,
the combined adoption gate stays false until an explicit coherent cohort is
selected. Legacy rows remain visible and are never silently re-versioned.
Keep class-map requirements and the 100/300 floors intact. Preserve a private
baseline report before later stages, including its cutoff and class-map hash.

**Verify:** Run `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./cmd/name-eval ./cmd/name-pipeline` → exit 0.

Expected: Offline report tests mix pool/reader/model versions and assert that
they cannot jointly satisfy a gate. The existing single-version cohort still
works. Doctor tests use fake binaries/HTTP transports and never inspect secrets.

## Commands and completion criteria

Run from the repository root unless stated otherwise:

| Command | Expected |
|---|---|
| `cd services/core-api && env -u DATABASE_URL -u AI_RESEARCH_URL go test ./internal/sourceprocessing ./internal/httpapi ./cmd/name-eval ./cmd/name-pipeline` | all unit tests pass; no real evaluation writes |
| `cd services/ai-research && .venv/bin/pytest && .venv/bin/ruff check .` | pass |
| `npm --workspace apps/web run test && npm --workspace apps/web run typecheck` | pass |
| `env -u DATABASE_URL -u AI_RESEARCH_URL make verify` | pass |
| `make generated-check && make docs-check && git diff --check` | pass |

- [x] Disabled hosted reading cannot load credentials or make a provider call.
- [x] The guarded integration runner is available before subsequent stages and
  refuses inherited real-database/storage settings before a write.
- [x] The UI does not offer a working-looking action for an unavailable reader.
- [x] Setup failures do not fan out to one failed request per crop.
- [x] Doctor output is aggregate-only and cohort mixing cannot pass evaluation.
- [x] Existing review rows and class-map bytes are unchanged.

## Git workflow, stops, and maintenance

Leave all changes uncommitted; preserve existing changes and untracked files.
No branch, commit, push, or PR is required. If later explicitly requested, use
the `codex/` branch prefix and the human's own identity without AI attribution.

Stop on mismatched source excerpts, an unapproved need to transmit private
images, or a proposal to backfill a judge result using context available only
after the human decision. A failing check must be diagnosed rather than weakening
the privacy or evaluation rules. Test fixtures belong only in an isolated test
database; never point a database-writing test at the real review database.

This stage deliberately keeps the provider configuration separate from health.
Future adapters must implement the same typed capability/reason contract. Local
reader readiness becomes true only when Plan 024 supplies a working adapter.

## Implementation record — 2026-10-27

Implemented against `3de1de1` after checking the drift from `f0fcb6e` and the live
source shapes. The changes remain uncommitted.

- Explicit reader policy replaces the private runtime credential-lookup guard.
  Both services default to local intent; hosted reading needs explicit mode and
  enablement, plus source-side approval for on-demand image requests. No private
  source approval or hosted provider enablement was performed.
- Source-authorized capabilities use bounded local tool probes and cached AI
  service metadata. The metadata GET is explicitly bodyless at the real Python
  middleware boundary; inference routes retain their original body limits.
- The bench disables unavailable readers before click, shows distinct Arabic
  reasons, and keeps manual edits usable. Off/shadow markup is identical in the
  component test. Local crop rereading is visibly unavailable until Plan 024.
- Permanent batch failures stop once, transient failures have two attempts, and
  only validation failures can fall back to individual crops.
- Doctor reports aggregate tools, policy, service status, versions and evidence
  gaps. Its current exit 2 correctly reports the unimplemented local adapter.
- Evaluation loads saved pool/reader/model/prompt versions, supports four cohort
  selectors, reports legacy evidence separately and blocks the combined gate
  when versions mix. Sample floors and pre-registered class coverage are intact.
- Private reports retain the cutoff and class-map hash. Fingerprints confirm all
  111 original actions and class-map bytes are unchanged; no truth was generated.

The first full database run was interrupted when the app launcher caused Compose
to recreate the shared PostgreSQL container. The failed isolated database/log
were retained. The guarded runner and migration verification now reuse the
running service without reconfiguring it; the next isolated run exposed an existing
cross-package fixture race in the visibility matrix's two whole-table counts;
the guarded runner now fixes Go package concurrency to one. The final run used
another fresh database, and no assertions or skip audits were weakened. It passed
with the shared database container unchanged.

Validation completed:

| Check | Result |
|---|---|
| Guard unit tests (also part of `make verify`) | 11 passed |
| Python suite and Ruff | 86 passed; lint passed |
| Web suite, ESLint, typecheck and build | 85 passed; all checks passed |
| `env -u DATABASE_URL -u AI_RESEARCH_URL make verify` | passed; later full gate also passed with the final changes |
| Guarded `verify-full` | passed: 930 Go tests, 924 passed, 6 optional skips, **0 database skips**; fixture audit passed |
| Migration verification and AI evaluation | all five migration cases and every metric group passed |
| Guarded `e2e` in a separate dedicated database/storage directory | 30 journeys passed |
| `generated-check`, `docs-check`, `git diff --check` | passed |
| Real evaluation replay and all four cohort selectors | 111 actions / 80 truths overall; selected coherent cohort has 79 truths; gate remains false |
| Review preservation | all 111 original row fingerprints and the frozen class-map SHA-256 match |

The live launcher now uses explicit local reader policy without the private
runtime monkeypatch, with `NAME_JUDGE_MODE=shadow`, sample 1 and images false.
No source was approved for hosted reading, no original file was changed, and no
new human decision or missing judge output was generated. The read-only doctor
returns exit 2 for the pending local adapter, as intended. Plan 024 is next.
