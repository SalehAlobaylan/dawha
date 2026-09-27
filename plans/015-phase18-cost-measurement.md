# Plan 015: Make Phase 18's cost criterion measurable, or say plainly that it cannot be

> **Executor instructions**: One criterion in the implementation plan requires a measurement this repository cannot take without production traffic. Your job is to build everything needed to take it, prove what can be proved here, and state the rest honestly. Do not manufacture a number that stands in for cost.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: plans 008, 009, 014
- **Category**: measurement
- **Planned at**: commit `4c40e77`, 2026-09-27

## Why this matters

`IMPLEMENTATION_PLAN.md:1368` sets Phase 18's first acceptance criterion: "measurable reduction in expensive model calls". Today the only measurement is route agreement on fifteen fixtures the same author wrote, plus a `synthesis_skipped` count — no cost measurement exists, and `docs/phase-status.md:316-330` says so.

That is the right call so far, but it leaves a criterion permanently open and a decision gate unmade: the plan wants to know whether the routing layer earns its place. Two things can be fixed without production traffic — the instrumentation that makes cost observable, and the provenance of the labels the routing is judged against. What cannot be fixed is the traffic itself, and the plan must end up saying which is which.

## Current state

- `internal/ai/routing.go` — the routing decision: query type, route (`ignore`/`cheap`/`deep`), reason code, `review_required`.
- `services/ai-research/app/main.py` — `routing_decision(fallback=True)`, the fallback path, and the deterministic provider.
- `evaluation/routing_cases.jsonl` — 15 cases with expected route, query type and reason code; `route_accuracy` 1.0, `query_type_accuracy` 0.8667.
- Telemetry from plan 008: `dawha_ai_calls_total{operation,outcome}`, `dawha_ai_duration_seconds`, and `dawha_ai_cost_units{operation}` — off by default (`TELEMETRY_METRICS_ENABLED`), no collector required.
- The model-provider abstraction in `internal/ai/client.go` carries a cost model per operation; there is no real provider configured, and adding one is out of scope.
- `docs/phase-status.md` records the gap and, importantly, that routing output is operational classification and never a historical status.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Eval | `make ai-eval` | exit 0, every group reported |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Metrics | the command this plan adds | emits a metrics sample |

## Scope

**In scope**:
- `services/core-api/internal/ai/` — routing, the cost model, and the counters that make a call's cost attributable
- `services/core-api/platform/telemetry/` — only if a counter is missing
- `services/ai-research/evaluation/` — the routing fixtures, their provenance, and the report
- `docs/` — the measurement design and the corrected Phase 18 row

**Out of scope**:
- Configuring, purchasing, or calling a real model provider.
- Changing what routing decides, or adding new routes.
- Anything that would let routing output influence a stored historical status. That guardrail is the phase's real requirement and it already holds; keep it holding.

## Steps

### Step 1: Make a call's cost attributable

A cost measurement needs three things bound to one call: which operation, which route produced it, and what it would have cost. Requirements:

- Every model call records the operation, the route that selected it, and a cost figure in the units the cost model already defines — with the model named, so a figure from one model is never compared against another.
- The counters must survive a provider swap: the cost model is the single place a price is expressed, and the telemetry reports the model it used.
- A test that a `cheap` route and a `deep` route for the same question produce different attributed cost, and that the total reconciles with the number of calls made. Without that reconciliation the number is decoration.
- The exporter stays off by default. No collector, no new service, and nothing that changes the default local loop.

### Step 2: Fix the provenance of the routing labels

The routing group is judged against fifteen cases its own author wrote, which is a real limitation whatever the accuracy number says. Requirements:

- Record, per case, where it came from and who labelled it. Cases derived from a real query set must say so and carry the query; cases written by hand must be marked as such rather than presented as a test set.
- Widen the set where it can be widened honestly: cases that exercise the fallback path, the `ignore` path, and the boundary between `cheap` and `deep` are the ones most likely to be under-represented in fifteen hand-written examples.
- Keep every existing case passing. Adding cases may lower the measured accuracy, and that is a finding to report, not a reason to drop the case.
- The report must carry the limitation in its own output, not only in prose beside it — a reader who opens the JSON must be able to see that the labels are internally authored.

### Step 3: Design the measurement, and take what can be taken here

Write the measurement design for "reduction in expensive model calls", as a document a future operator can execute without re-deriving it:

- What is compared against what: the routing decision's route, the counterfactual of always taking the deep path, over a stated window of real traffic.
- The exact counterfactual must be computable, not hypothetical. If a `cheap` route skips a call, the counterfactual cost of not routing is knowable from the same counters.
- The sample size, the window, and the threshold for "reduction" must be stated before the data exists, so the criterion cannot be met by choosing a convenient week afterwards.
- The confounds to control for: question mix over time, provider price changes, and a change in traffic volume.

Then take the measurement this repository can actually take, and say which it is:

- A **synthetic workload** over the labelled routing set, reporting attributed cost per route with the model named. This measures the routing logic's cost behaviour. It does **not** measure a reduction in real spend, and the report must not describe it as though it does.
- If a real provider is ever configured, the same counters produce the real number with no code change. State that as the mechanism, not as a result.

### Step 4: Update the record honestly

Rewrite Phase 18's row in `docs/phase-status.md` so a reader can tell, without reading further:

- what is measured here and what its number means;
- what remains unmeasured and why (no production traffic, no configured provider);
- exactly what data would close the criterion, and who has to collect it.

The criterion either stays open with that stated, or is met — and if it is met, the evidence must be the real-traffic measurement, not the synthetic workload.

## Test plan

- Cost attribution: per-route figures differ, reconcile with call counts, and name the model.
- Telemetry remains off by default and adds no collector.
- Routing fixtures: provenance recorded per case, existing cases unchanged, newly added cases that expose a mis-route are kept.
- The report states its own limitation in its machine-readable output.
- The Phase 18 row matches the code and the report.

## Done criteria

- [ ] Every model call's cost is attributable to an operation, a route and a named model, and the totals reconcile.
- [ ] Routing fixtures carry per-case provenance, and the report exposes that the labels are internally authored.
- [ ] A measurement design for real-traffic cost exists and is executable as written.
- [ ] Phase 18's first criterion is either met with real-traffic evidence, or explicitly open with the missing data named.
- [ ] `make verify`, `make verify-full` and `make ai-eval` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if any part of this requires a real model provider, a credential, or a spending decision — report the design without executing it.
- Stop if making cost attributable would require a second pricing source that could disagree with the cost model.
- Stop if verification fails twice.

## Maintenance notes

The guardrail this phase actually exists for — routing output is operational classification, never a historical status — is enforced by tests and must stay enforced. A cost measurement that pressured that boundary would be a worse outcome than no measurement.
