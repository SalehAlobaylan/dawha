# Plan 012: Stop the test-data leak, and stop citing sources that do not answer

> **Executor instructions**: Two defects with the same shape — a system that reports success while something quietly wrong is happening. Fix the leak at its source and make the gate catch it next time; fix the citation filter and raise its threshold to the honest value.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: plans 004, 009
- **Category**: test-infra/correctness
- **Planned at**: commit `b34c6fe`, 2026-09-26

## Why this matters

**The leak.** 593 of the 580 users in the local development database are synthetic test rows created by test fixtures that insert into the shared `public` schema and never delete them. `people` is 157 against a seed of ~119, `trees` 78, `audit_log` 977. Every full-suite run against the shared database adds more, and nothing fails. A developer reading their own database is reading a distorted record of the product, and a fixture that leaks today hides the next one.

**The citation.** `services/ai-research/app/main.py:607-620` treats any shared token as grounding, so a context that shares only a preposition with the question becomes a citation. Measured `citations.unsupported_refusal = 0.5` on plan 009's fixtures, and the threshold was set *at* that measurement with the defect recorded in `KNOWN_DEFECTS` rather than fixed. A gate pinned to a known-wrong value cannot catch a regression, and this one guards a safety property: a research answer must not cite a source that does not answer the question.

## Current state

- Leaking fixtures span at least `internal/evidence`, `internal/research`, `internal/researchagent`, `internal/temporalanalysis`, `internal/geospatialintelligence`, `internal/visibility`, `internal/dictionary`, `internal/sourceprocessing`, and `internal/httpapi`. They share one pattern: `INSERT INTO users ... @example.test` plus a hand-written or absent `t.Cleanup`.
- `services/core-api/internal/testsupport/` already has the machinery worth reusing: a per-test migrated+seeded schema (`fixture.go`, `schema.go`), an actor helper, and `audit.go`. The browser suite has a proven dependency-ordered cleanup (`apps/web/e2e/cleanup.sql` plus a global teardown) that deletes in FK order, nulls nullable references, loops to convergence, and proves nothing was orphaned.
- `services/core-api/tools/dbtestguard` already fails on a database-gated skip, an unmigrated schema, a leaked `p004_fixture_*` schema, and a required package that ran no test. It is the gate that should catch a leak.
- `services/ai-research/app/main.py:596-620` — `research_query` ranks contexts by shared-token count and cites every context with a non-empty intersection.
- `services/ai-research/evaluation/citation_cases.jsonl` — 6 reviewed Arabic fixtures; `cit-003` is the one that must produce no citation because nothing sent answers the question.
- `services/ai-research/evaluation/thresholds.py` — versioned thresholds; `citations.unsupported_refusal` currently 0.5.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Shared-database run | `COMPOSE_PROJECT_NAME=dawha DATABASE_URL=... go run ./tools/dbtestguard -- ./...` | exit 0, and **zero synthetic rows left behind** |
| AI eval | `make ai-eval` | exit 0, `citations.unsupported_refusal` at 1.0 |

## Scope

**In scope**:
- The leaking test fixtures, and a shared cleanup helper in `internal/testsupport` they all use
- `services/core-api/tools/dbtestguard` and/or `internal/testsupport/audit.go` — the leak check
- `services/ai-research/app/main.py` — the citation filter only
- `services/ai-research/evaluation/` — fixtures, thresholds, report text
- Documentation of the synthetic-user convention

**Out of scope**:
- Moving every DB test onto the per-test schema helper. That is a large, separate refactor; the goal here is that nothing leaks and that a leak fails.
- Changing the `rerank` scoring, the retrieval path, or anything else in `app/main.py`.
- Deleting anything in the development database that is not a synthetic test row.

## Steps

### Step 1: One cleanup helper, used by every fixture

Add a helper to `internal/testsupport` that takes a `*testing.T` and the synthetic actor ids a fixture created, and removes everything reachable from them: null the nullable foreign keys, delete the NOT NULL dependents in dependency order, loop until a pass changes no rows, then assert that no row reachable from those actors remains. Model it on `apps/web/e2e/cleanup.sql`, which is already proven, and make it usable from any package.

Then route the leaking fixtures through it. Audit every `INSERT INTO users` in a `_test.go` file, and for each one either (a) it already cleans up, or (b) it now does, or (c) it is deliberately leaving a row and says why in a comment. Report the count in each category — the "deliberately leaving a row" bucket should be empty, and if it is not, that is a finding.

### Step 2: Make the gate catch the next leak

Extend the existing gate so a leaked synthetic row fails the run rather than accumulating. Scope it the way the browser teardown does — by the synthetic marker (`%@example.test`), not by a global row count — so it stays deterministic while packages run in parallel. It must name the offending rows. Prove it: deliberately leave a user behind in one test and show the gate fails with that user's email in the output, then remove the leak.

A global before/after count of `users`/`people`/`trees` is *not* acceptable here: packages run in parallel, so it is racy, and plan 005 already learned that lesson the hard way.

### Step 3: Sweep what is already there

Remove the 593 leaked rows from the developer's `dawha` database with the same helper logic, in dependency order, with a convergence loop and an orphan probe (a child row whose parent user is gone). Report the before/after counts for `users`, `people`, `trees`, `sources`, `claims`, `audit_log`, and the orphan count. Do not touch the real seeded rows or the seed data itself, and do not drop any schema you did not create.

### Step 4: Require a content token before citing

Change the citation filter in `research_query` so a context is cited only when it shares at least one **content** token with the question. Define "content token" as the existing tokenizer's output minus a small, explicit, documented stopword set (Arabic and English function words: prepositions, pronouns, conjunctions, articles). Keep the set small enough to review by eye, and put it next to the filter with a comment saying why a shared preposition is not grounding.

The answer text and the refusal path must not change: a question with no citable context still answers that it cannot build a reliable answer.

Fixtures: extend `citation_cases.jsonl` so both directions are pinned — a context sharing only a function word is not cited (already `cit-003`), a context sharing a content word is cited, and a context sharing a content word *and* a function word is still cited. Review them yourself and record `reviewed_by`.

### Step 5: Raise the threshold to the honest value

Set `citations.unsupported_refusal` to **1.0** and remove the entry from `KNOWN_DEFECTS` — the defect is fixed, not recorded. `make ai-eval` must pass at 1.0, and must still fail when a threshold is exceeded. If the fix cannot reach 1.0 on the reviewed fixtures, that is a STOP: report the measured value and the fixture that blocks it rather than lowering the threshold.

## Test plan

- A fixture that leaks fails the gate, naming the row.
- A full run against the shared database leaves zero synthetic users behind.
- The sweep's orphan probe returns zero, and the seeded rows are unchanged.
- Citation fixtures pin both directions; a function-word-only overlap is never cited.
- `unsupported_refusal` measures 1.0 and its threshold is 1.0.

## Done criteria

- [ ] No test fixture leaves a row in the shared schema, and the gate fails if one does.
- [ ] The 593 leaked rows are gone and the seeded data is untouched.
- [ ] A research answer cites only contexts sharing a content token with the question.
- [ ] `citations.unsupported_refusal` is 1.0 and that defect is no longer in `KNOWN_DEFECTS`.
- [ ] `make verify`, `make verify-full` and `make ai-eval` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if routing every leaking fixture through the helper would require changing what those tests assert — report the fixture and the conflict.
- Stop if the citation fix cannot reach 1.0 on reviewed fixtures — report the measured value and the blocking fixture.
- Stop if verification fails twice.

## Maintenance notes

The synthetic-user marker is now a contract, not a convention: anything a test creates must be marked synthetic and removed, and the gate checks. A fixture that genuinely needs a permanent row belongs in the seed, not in a test.
