# Phase 18's cost criterion: the design, and what was measured here

**Status:** the design is complete and executable as written. The criterion
"measurable reduction in expensive model calls"
(`IMPLEMENTATION_PLAN.md:1368`) is **OPEN**, and the number this repository can
produce is **not** the number that would close it. Both facts are in the machine
readable report, not only here.

**What exists to make the measurement possible:** a cost model in
`services/core-api/internal/ai/cost.go` that prices any call from the request
this service builds, and a counter,
`dawha_ai_cost_units{operation,route,model}`, that attributes that price to the
operation, to the JEV route whose decision the call was made under, and to the
model that answered.

**What was measured here:** `make cost-report` writes
`docs/benchmarks/cost-attribution.json` — a **synthetic** workload over the
labelled routing set, reporting attributed cost per route with the model named.
It measures the routing logic's cost behaviour. It does not measure a reduction in
real spend, and the report says so in its own output.

**Why the criterion is open:** this repository has no production traffic, no
configured model provider and no invoice. Nothing in the code can acquire any of
the three.

## The cost model, and what the unit is

One **dawha work unit** (dwu) is the work of carrying one thousand runes of this
service's own JSON request payload at unit weight, multiplied by the declared
weight of the operation and of the model:

```text
units = (1 + ceil(payload_runes / 1000)) * operation_weight * model_weight
```

`cost.go` is the only place a weight is written. Both tables are there; a test
asserts the operation and model enumerations are total against them, so a new
endpoint or a new model cannot slip through unpriced and silent.

**The unit is deliberately not money, and no figure derived from it may be
described as money.** Three things force that:

- The only embedding provider this repository may use returns a SHA-512 digest of
  its input rather than a semantic vector — the same finding that made Phase 20's
  criterion open in `docs/retrieval-measurement.md`. A token count taken from
  this repository's provider is a count of characters.
- No provider is configured, so there is no price list to read.
- A figure about tokens is still a figure about tokens. Naming it honestly costs
  one paragraph and buys a number nobody can mistake for a bill.

**The weights are declared, not measured.** There is no invoice to measure them
against. A reader who wants a real figure replaces the table and re-runs the same
counters; nothing else changes.

### Why the size term is this repository's own payload

A cost that must be comparable across models cannot depend on the model, and the
counterfactual cost of a call that **never happened** has to be computable at all
— which rules out anything derived from a provider response, because there is no
response for a call nobody made. So the size term is the rune length of the
request this package already builds, which **both** arms of the comparison have.
That is what makes the counterfactual a number rather than a story, and it is why
`AttributedCost` takes a request and a model rather than a usage report.

### The three labels, and what each is for

| Label | Why it is there |
| --- | --- |
| `operation` | the endpoint, as an enumeration. Never a path, never the input. |
| `route` | the JEV route in whose context the call was made. Two routes for the same request are **two series**, so "did routing save anything" has a shape to ask. |
| `model` | why a figure from one model is never silently added to a figure from another. It is also how a provider swap is detected: a name the cost model does not price becomes `model="unpriced"`, and that series is the request **size**, not a price. |

`dawha_ai_cost_units` is written **only** when the exporter is on, which is off by
default (`TELEMETRY_METRICS_ENABLED`). No collector, no new service, and a provider
with no telemetry registry does not even marshal its request to price it.

## The guardrail, which cost pressure is not allowed to touch

`IMPLEMENTATION_PLAN.md:1362` says Jev output is operational classification and
must never be stored as historical confidence. Two properties hold that, and this
plan added machinery on both sides of the line, so both are asserted:

- **A route reaches a metric label and nowhere else.** `ai.WithRoute` puts a route
  on a context; `RouteFromContext` reads it once, at the moment of a call;
  `Metrics.AICost` writes it to one counter. Nothing in `internal/ai` persists a
  route, and nothing reads one back. The context key is unexported and of its own
  type, so no other package can put a route on a context.
- **A route can never become a stored status.** `db/migrations/0018_semantic_control.sql`
  holds `CHECK` constraints over the three route values and the seven reason
  codes, and `internal/ai` contains no SQL.

`TestTheGoFallbackStaysOperationalClassification` asserts the half the guardrail
owns: the decision carries no field that means anything about the past, it always
carries `review_required`, and the only way out of the package is the counter's
label.

**A cost figure is not a reason to weaken any of this.** If the measurement ever
wanted a per-question cost to be stored, or a cost threshold to feed a status, the
right answer is to stop measuring, not to relax the constraint. The one thing this
plan did *not* do is make the route a price multiplier: the same request priced
under two routes costs the same, and the difference between them is attributable
solely because they are separate series. `TestTheRouteIsALabelAndNotAPrice`
fails if that ever changes.

## The design: "measurable reduction in expensive model calls"

Written so an operator can execute it without re-deriving it. **Every number in
this section is fixed before the data exists**, which is the whole point: a
threshold chosen after the data arrives is not a threshold.

### What is compared against what

| | |
| --- | --- |
| **Observed arm** | the route the deployment's own routing actually chose, per research query, over the window. |
| **Counterfactual arm** | the same queries answered by the deep path, over the same window. |
| **Quantity** | attributed cost in dwu, from `dawha_ai_cost_units{operation="research_query",route=...,model=...}`. |
| **Primary quantity** | the **rate of expensive calls**: `dawha_ai_calls_total{operation="research_query"}` per 1,000 research queries, which is the criterion's own wording and is independent of the cost model's weights. |

The primary quantity is the rate, not the total. A total falls when traffic falls,
and `docs/phase-status.md` already records traffic volume as a confound on every
other number in this repository.

### The counterfactual is computable, not hypothetical

For a research query the deployment routed to `cheap` or `ignore`, the
counterfactual cost of answering it on the deep path is **the same function over
the same payload**:

```text
counterfactual_units(run) = ai.AttributedCost(
    telemetry.AIOperationResearchQuery,
    ai.ResearchQueryRequest{Query: run.Query, Contexts: <the run's citations>},
    model,
).Units
```

The inputs are all present: `research_runs.query` and `research_runs.semantic_route`
are stored on the run, and the passages a deep route would have synthesised from
are the run's own citations. Nothing has to be estimated and nothing has to be
guessed at a provider's tokenizer.

Two series make the cheap side free to obtain rather than a per-row join:

- `dawha_ai_cost_units{operation="research_query",route="deep",...}` — what was
  actually spent on synthesis, and
- the routing call's own attributed cost,
  `dawha_ai_cost_units{operation="route",route=<the route decided>,...}` — one
  series per route decided, because the routing call is attributed to the route it
  produced. That is the whole content of its result, and it is what makes the
  count of non-deep routing decisions obtainable without a join.

The sum over a window is then:

```text
reduction_units = Σ over non-deep runs of counterfactual_units(run)
                - dawha_ai_cost_units{operation="research_query"} over the window
```

### The window, the sample size, the threshold

| | |
| --- | --- |
| **Window** | **28 consecutive days**, taken as the most recent complete 28-day period the deployment has counters for. A fixed length, so the window cannot be chosen after the fact. |
| **Sample size** | **at least 1,000 research queries** in the window, of which at least 200 must have taken the deep path. Below either floor the estimate is not reported at all — reported as `insufficient sample`, which is a failure, not a pass. |
| **Primary threshold** | the deep-path call rate over the window is **at least 20% lower** than the counterfactual all-deep rate, and the 95% confidence interval on the difference excludes zero. |
| **Secondary (money-free) threshold** | the reduction in attributed dwu, computed by the formula above, is **at least 20%** of the counterfactual. |
| **Excluded from a pass** | a result in which the 95% interval includes zero, or in which the window contains a price change or a traffic change of more than 25% between its two halves. Those are reported as inconclusive. |

The 20% floor is a **deliberate floor, not a target**, on the same terms
`evaluation/thresholds.py` sets for the AI quality groups: passing it means
routing did not fail, and a green result here is not a claim that 20% is good
enough. Where the real reduction lands against 20% is a finding to report, in
either direction.

### The confounds, and what to do about each

| Confound | Why it moves the number | Control |
| --- | --- | --- |
| **Question mix over time** | A week of place questions is cheap; a week of relationship questions is deep. A change in the mix is a change in cost with no change in routing. | Report the per-route distribution alongside the rate. If the distribution shifts by more than 10 percentage points between the window's two halves, the result is **inconclusive** until a longer window is used. |
| **Provider price changes** | The weights in `cost.go` are declared. A provider repricing mid-window makes one window's dwu mean something different from another's. | The primary quantity is the **call rate**, which no price change touches. The dwu figure is reported with the `model_weights` table that produced it, and a window spanning a price change is reported as **inconclusive** for the secondary threshold only. |
| **Traffic volume** | A total falls when traffic falls. | The primary quantity is a rate per 1,000 queries. A window whose traffic varies more than 25% between halves is reported as **inconclusive**. |
| **Fallback rate** | When the route call is unavailable the Go fallback decides, and `TestTheGoFallbackAgreesWithTheReviewedLabelsTheEvaluationUses` records that the two implementations disagree on one case. | Report `dawha_ai_cost_units{operation="route",route=...}` by route and the fallback share, so a window dominated by fallback is visible. |
| **Routing changes mid-window** | A new term list changes what `deep` means. | The window must not contain a change to `internal/ai/routing.go` or to the provider's term lists. One that does is **inconclusive**. |

### What closes the criterion

The real-traffic measurement above, at the threshold and over the window fixed
here, read off the counters this repository already exports. **Nothing in the
repository needs to change to produce it**, which is why there is no code left to
write when the traffic arrives — only a decision nobody here can make.

**Who collects it:** the operator of a deployment. This repository has no
deployment, no production traffic and no configured provider.

## The synthetic workload, and what it measured

`make cost-report` runs `TestCostMeasurementReport`
(`services/core-api/internal/ai/cost_report_test.go`) over the thirty-three
labelled cases in `services/ai-research/evaluation/routing_cases.jsonl`, asks this
repository's own `FallbackRoute` what it would do with each, and prices both arms.

It needs no database, no network, no provider and no credential.

Recorded run: `docs/benchmarks/cost-attribution.json`, fixture set sha256 recorded
in the report alongside the commit that produced it.

What it found, on the fixture set: the `deep` route spends one `research_query`
call per question and the `cheap` and `ignore` routes spend nothing, so the
all-deep counterfactual for the same thirty-three questions is about twice the
attributed total. That is a fact about the shape of the routing decision.

**What that is not.** It is not a reduction in spend, it is not money, and it is
not over a question mix: the mix is thirty-three questions one author wrote. The
report says this in three fields a reader cannot miss — `what_this_is`,
`what_this_is_not`, and `reduction.is_a_measured_reduction_in_spend: false` — and
in the printed summary, so a reader who takes the number from the console output
takes the refusal with it.

The report also refuses the reading in its grouping: attributed cost is reported
**twice**, once grouped by the fixture's reviewed label and once by what the code
actually decided, with the cases that differ listed. The two groupings are equal
here, and the report says that is a result rather than an absence.

### The reconciliation, and what it is for

Two reconciliations, and they are different things:

- **Live** — `TestAttributedCostReconcilesWithTheNumberOfCallsMade` in `cost_test.go`
  makes real HTTP calls through a real `HTTPProvider` and asserts that the recorded
  total, **and every per-series value**, equals what the cost model independently
  predicts, and that the number of calls recorded equals the number made. It runs on
  every build. Without it the counter is a number nobody checked.
- **Synthetic** — the report's own `reconciliation` field asserts its per-route
  totals are its per-case figures added up, and `per_case` carries every case so a
  reader can add them up again.

The property that makes the counterfactual exact is
`TestACheapAndADeepRouteAttributeDifferentCostForTheSameQuestion`: the cheap
route's counterfactual **equals** the deep route's attribution, on the same
payload, priced by the same function. If that equality did not hold, the
counterfactual would be a story.

## If a real provider is ever configured

The mechanism, stated as a mechanism and not as a result:

1. A provider answers with its own model name, `ModelFor` maps it to
   `AIModelUnpriced`, and the series shows up labelled `unpriced` carrying the
   request size. That is the first thing a swap looks like, and it happens before
   anybody has updated a number. `TestASwapToAnUnpricedProviderShowsUpAsUnpricedRatherThanAsZero`
   exercises it.
2. That provider's weights go into `modelWeights` in `cost.go`. **That is the only
   change.** The counter, its three labels, the reconciliation, the counterfactual
   and the design above are already written against the closed model enumeration.
3. The counters then carry real traffic, and the design's procedure applies with no
   code change.

Step 2 requires a **spending decision** — a real provider, a credential, a price
list — which is outside this plan's scope and outside what this repository may do.

## Maintenance notes

- **`routeWork` in `cost.go` is checked against the RAG service, not trusted.**
  It is declared data describing the switch in `internal/research/rag_service.go`
  rather than a reimplementation of it, and the two cannot share a source:
  `internal/research` imports `internal/ai`, so the reverse import is a cycle, and
  collapsing the two switches into one function would mean putting the product's
  answer strings in the cost model or the cost model's vocabulary in the RAG
  service. Instead
  `TestTheCostModelPricesTheRoutesTheRagServiceActuallyRuns` reads that switch out
  of the RAG service's source, derives per route which model operations it calls,
  and requires `routeWork` and that derivation to agree **in both directions** — so
  a new model call on a route, a route that stops synthesising, or an operation
  the map names that the switch never calls all fail a build. The reason is stated
  beside the map as well.
- The routing labels are internally authored. `evaluate_routing.load_cases` refuses
  a case without provenance, and both reports carry the limitation in their own
  output. A number derived from them is agreement with an author's expectations.
- **Which implementation produced a routing number.** The labelled case is the
  authority for a route: `expected_route` in `routing_cases.jsonl` says what a
  question of that shape should do, and both implementations are measured against
  it. The normalization the two share has its specification in
  `internal/identity`'s `NormalizeArabicName`, mirrored step for step by the
  provider's `normalized_routing_text`. Neither implementation is the authority
  over the other: the provider answers when it is up, `ai.FallbackRoute` answers
  when it is not, and the property that matters is that a question does not change
  route because of which one answered.
  `make cost-report` asks this repository's own `FallbackRoute`; `make ai-eval`
  measures the provider. The two were separate implementations until plan 016
  aligned the shared vocabulary, and they are compared field by field over all
  thirty-three labelled cases by
  `TestTheGoFallbackAndTheProviderReachTheSameDecisionOnEveryLabelledCase`, which
  derives both decisions rather than declaring which cases disagree. The divergence
  set is now empty, so the number a cost report prices is the number the same
  question would take with the service up.
- `make cost-report` is deliberately **not** in `verify` or `verify-full`. A report
  is not a tree property. The assertions behind it are in `verify` on every build.
