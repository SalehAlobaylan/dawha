# Plan 011: Prove the S3 adapter against MinIO, and record R2 as the production target

> **Executor instructions**: This is a verification plan, not a feature plan. The adapter already exists. Your job is to make it talk to a real S3 implementation, and to write down what a Cloudflare R2 cutover has to check. Storage is explicitly not a priority: keep the surface small and leave the default local stack alone.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: plan 008
- **Category**: verification/docs
- **Planned at**: commit `188fbaf`, 2026-09-26

## Why this matters

`services/core-api/platform/storage/s3.go` is complete and contract-tested against a test double, but it has never spoken to a real S3 implementation. The contract suite reproduces S3's *shape* (key in the path, signature over key and expiry, typed `NoSuchKey`) and nothing more, so SigV4 canonicalisation, size accounting, and presigning are unverified against a real endpoint. Plan 008 recorded that as its one storage residual: "a deployment must still run one real presign before trusting it".

The product decision is now made: **MinIO for local and integration verification, Cloudflare R2 for production.** Both speak the S3 API, so the same adapter serves both, and the differences that matter at cutover are documented rather than guessed.

## Current state

- `services/core-api/platform/storage/{storage.go,s3.go,config.go,contract_test.go,s3_test.go}` — the interface, the adapter, the config, the double, and the contract suite.
- `services/core-api/platform/storage/storage.go:41-47` — the `Store` interface: `Put`, `Get`, `SignedURL`, `Delete`.
- `docker-compose.yml` — one service, `db`. No object storage in the local stack.
- `Makefile` has `db-up`/`db-down`; `apps/web/e2e/stack.mjs` starts three processes and does not need object storage.
- `.env.example` documents every variable by name, including the local `dawha_local` development password convention.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Storage up | `make storage-up` | exit 0, MinIO healthy |
| Contract | `cd services/core-api && go test ./platform/storage -count=1` | all pass, MinIO cases included |
| Against MinIO | `STORAGE_TEST_ENDPOINT=... go test ./platform/storage -run MinIO -count=1 -v` | all pass |
| Fast gate | `make verify` | exit 0, and MinIO absent is not a failure |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<scratch> make verify-full` | exit 0 |

## Scope

**In scope**:
- `docker-compose.yml` and the `Makefile` (an opt-in storage service, never in the default stack)
- `services/core-api/platform/storage/` — the MinIO integration test and whatever the suite reveals
- `docs/` and `.env.example` — the R2 cutover record
- `infra/local/` if a helper script is needed

**Out of scope**:
- Changing the `Store` interface or the API's storage wiring.
- Multi-bucket, replication, lifecycle rules, CDN, or any R2 feature beyond what the adapter uses.
- Making MinIO part of `make dev`, `make e2e`, or `make verify`.
- Renaming the adapter away from `S3`; R2 is S3-compatible and renaming would be churn.

## Steps

### Step 1: Add MinIO as an opt-in local service

Add a `minio` service to `docker-compose.yml` behind an explicit opt-in (a compose `profiles: ["storage"]` entry, or an equally explicit mechanism) plus `make storage-up` / `make storage-down` targets. Requirements:

- **The default stack must be byte-for-byte unchanged.** `make db-up`, `npm run dev`, `make e2e` and `make verify` must not start, require, or wait for MinIO.
- Dev credentials follow the existing convention: a documented non-secret default in `.env.example` (the same way `POSTGRES_PASSWORD` has `dawha_local`), overridable by environment variable, never a real secret.
- A named volume for the bucket data, a healthcheck, and a pinned image tag — not `latest`.
- Document the console/API URLs and the ports in `.env.example` and the README's troubleshooting section, in the same style plan 008's README uses.

### Step 2: Run the existing contract suite against a real MinIO

The suite already exists and already runs against a double. Run the *same assertions* against MinIO:

- Keep the double-based tests unconditional, so `make verify` keeps its meaning with no MinIO running.
- Add a MinIO-backed test that is skipped unless an endpoint is configured, following the pattern plan 006 established with `AI_RESEARCH_URL`: an unset variable skips, a set one runs. `make storage-up` sets it.
- The MinIO case must cover the whole interface: put/get round trip with content and size accounting (including a non-seekable body), `ErrInvalidKey` for an escaping key, `ErrNotFound` for a missing key, delete then `ErrNotFound`, and a **presigned URL fetched over HTTP with a plain client** — that last one is the entire point of this plan, because it is what no double can prove.
- Report anything the real endpoint does that the double does not: error shapes, status codes, or size-accounting differences. If the adapter is wrong, fix the adapter; if the double is unfaithful, fix the double so the suite keeps its value. Both are in scope.

### Step 3: Write the R2 cutover record

Add a short decision record under `docs/` (match the convention `docs/graph-benchmark.md` established) covering:

- The decision: MinIO locally, Cloudflare R2 in production, one `S3Store` for both, because R2 speaks the S3 API.
- The exact configuration difference between them, stated as facts to verify rather than assumptions. At minimum: R2's account/endpoint/region shape, the credentials type, path-style versus virtual-hosted addressing, and presigned-URL behaviour. Cite the Cloudflare documentation you rely on by name; do not invent endpoints or claim a behaviour you have not checked.
- The cutover checklist: what must be run once against a real R2 bucket before production traffic — at minimum one presigned GET, one upload, and one `NoSuchKey` check — and who signs off.
- What stays explicitly unverified: anything this plan could not test, named plainly, so the next person does not read "MinIO passed" as "R2 passed".

Update the storage paragraph in `README.md` and the storage residual line in `plans/README.md` to match the decision. Do not touch other plans' entries.

## Test plan

- MinIO absent: `make verify` and `make verify-full` are unaffected, and the MinIO test skips with a message naming the variable.
- MinIO present: the full contract suite passes against the real endpoint, including a presigned URL fetched over HTTP.
- A deliberately broken adapter fails the MinIO suite, proving the suite is not vacuous.
- `docker compose config` resolves with and without the profile.

## Done criteria

- [ ] `make storage-up` brings up a healthy, pinned MinIO, and the default stack is unchanged without it.
- [ ] The existing storage contract suite passes against real MinIO, including a presigned URL fetched over HTTP.
- [ ] The MinIO test skips cleanly when no endpoint is configured, and `make verify` needs no MinIO.
- [ ] An R2 cutover record exists with a cited configuration difference and a named checklist.
- [ ] `make verify` and `make verify-full` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if MinIO cannot run on this machine (no Docker image available offline) — report it; do not substitute a hand-rolled HTTP fake and call it MinIO.
- Stop if a live MinIO handshake shows the adapter's SigV4 implementation is wrong — report the failing case rather than loosening the signature.
- Stop if verification fails twice.

## Maintenance notes

The contract suite is the durable asset here: one suite, two backends, and a double that must stay faithful. If R2 ever needs behaviour MinIO cannot reproduce, extend the double rather than adding a second suite.
