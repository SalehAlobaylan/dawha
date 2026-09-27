package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// THE SYNTHETIC COST WORKLOAD.
//
// What this measures: the ROUTING LOGIC'S COST BEHAVIOUR. It takes every labelled
// routing case, asks this repository's own fallback route what it would do with
// that question, prices both arms of the comparison with the cost model, and
// reports attributed cost per route with the model named.
//
// What this does NOT measure, in the report's own words and not only in a document
// beside it: a reduction in real spend. There is no production traffic, no
// configured provider and no invoice. The workload is a fixture set the same
// author wrote, so the question mix is that author's imagination, and a figure
// computed from it says what the routing code does with that imagination. It does
// not say what routing does to a bill.
//
// The report names that distinction in fields a reader cannot miss - what_this_is,
// what_this_is_not, and reduction.is_a_measured_reduction_in_spend - because the
// failure this command exists to prevent is a reader quoting difference_units as a
// saving.
//
// It is NOT in `make verify` or `make verify-full`, for the same reason
// `graph-benchmark` and `retrieval-report` are not: it writes a report rather than
// gating a tree. Its ASSERTIONS are in `make verify` - the reconciliation, the
// route-is-a-label property, and the cost model's totality are ordinary tests in
// cost_test.go and run on every build.
//
//	make cost-report
//	DAWHA_COST_REPORT_PATH=/tmp/cost.json make cost-report

// The sentences the report has to carry. They are constants rather than inline
// strings so that a change to one of them is a visible diff against a sentence
// somebody may have quoted.
const (
	costReportWhatThisIs = "A SYNTHETIC WORKLOAD. It runs this repository's own routing logic over a labelled " +
		"routing set and prices both arms of the comparison with the cost model, so it measures the ROUTING " +
		"LOGIC'S COST BEHAVIOUR: which routes spend model work, how much, and what the all-deep baseline " +
		"would have cost for the same payloads."

	costReportIsNotTraffic = "It is NOT a measurement of a reduction in real spend. There is no production " +
		"traffic, no configured provider and no invoice in this repository, and this report has no way to " +
		"acquire any of the three."

	costReportIsNotMoney = "It is NOT measured in money, and no field in this report may be quoted as an " +
		"amount paid or an amount saved. The unit is a dawha work unit, defined in " +
		"services/core-api/internal/ai/cost.go."

	costReportIsNotQuestionMix = "It is NOT measured over a real question mix. The question mix is the " +
		"fixture set, which the same author wrote, so the proportions of cheap and deep questions below are " +
		"that author's proportions and not a deployment's."

	costReportDoesNotCloseCriterion = "It does NOT close IMPLEMENTATION_PLAN.md:1368. The `criterion` field " +
		"below says so, and says what would."

	costReportLabelLimitation = "Every case was hand-written by an author of this repository, and the route, " +
		"query type and reason code in each case are that author's labels rather than a reviewed judgement " +
		"about a real question. Grouping attributed cost by a labelled route therefore groups it by the " +
		"author's expectation, and `attributed_cost_by_decision` is the same figure grouped by what the code " +
		"actually did. The two differ by the cases in `label_decision_divergences`. Neither grouping is a " +
		"statement about a real user, and this report must not be quoted as one."

	costReportDifferenceIsNotASaving = "This difference is what routing was worth ON THIS FIXTURE SET under " +
		"THIS cost model, in work units that are not money. It is not a saving, a budget, a forecast or a " +
		"result. The criterion is about expensive model calls in production, and this report has no " +
		"production traffic. The counterfactual is exact - the same function over the same payload - and it " +
		"is exact about the wrong thing."
)

// costReportDivergenceNote explains the label/decision divergence list, including
// when it is empty. An empty array beside two identical groupings is the kind of
// thing a reader either skips or reads as a claim; it is neither.
func costReportDivergenceNote(divergences []map[string]any) string {
	if len(divergences) > 0 {
		return fmt.Sprintf("%d case(s) reach a different ROUTE from the one their label says, so the two "+
			"groupings above are different numbers. Both are reported and neither is the whole truth. "+
			"Query-type and reason-code disagreements are NOT in this list: they do not change which "+
			"route a case takes, and therefore do not change what it costs. The evaluation reports those "+
			"separately, in the routing group's `misroutes`.", len(divergences))
	}
	return "Empty, and that is a result rather than an absence: over these thirty-three cases the Go " +
		"fallback's route agrees with the reviewed label on every one, so grouping attributed cost by " +
		"label and grouping it by decision give the same numbers. The label and the code still disagree " +
		"about QUERY TYPE on four cases, which the evaluation reports in the routing group's " +
		"`misroutes`; a query type does not change which route a case takes, so it does not move a cost " +
		"figure. The two implementations of the decision also disagree with EACH OTHER on rt-019 - " +
		"which implementation is routed to ignore a punctuation-only query - and that is recorded in " +
		"TestTheGoFallbackAgreesWithTheReviewedLabelsTheEvaluationUses."
}

func TestCostMeasurementReport(t *testing.T) {
	if os.Getenv("DAWHA_COST_REPORT") != "1" {
		t.Skip("set DAWHA_COST_REPORT=1 to write the cost measurement report; see docs/cost-measurement.md")
	}
	target := os.Getenv("DAWHA_COST_REPORT_PATH")
	if target == "" {
		target = filepath.Join("..", "..", "..", "..", "docs", "benchmarks", "cost-attribution.json")
	}
	source := filepath.Join("..", "..", "..", "ai-research", "evaluation", "routing_cases.jsonl")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("reading %s: %v", source, err)
	}
	digest := sha256.Sum256(raw)

	cases := goFallbackRoutingCases(t)
	report := buildCostReport(t, cases, source, hex.EncodeToString(digest[:]))
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encoding the report: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("writing %s: %v", target, err)
	}
	t.Logf("cost measurement report written to %s", target)
	// The printed summary is what a person reads, so the refusal is in it too and
	// not only in the file.
	t.Logf("THIS IS: %s", costReportWhatThisIs)
	t.Logf("THIS IS NOT: %s", costReportIsNotTraffic)
	t.Logf("THIS IS NOT: %s", costReportIsNotMoney)
	t.Logf("attributed cost per labelled route: %s", mustJSON(report["attributed_cost_by_labelled_route"]))
	t.Logf("counterfactual: %s", mustJSON(report["reduction"]))
	t.Logf("reconciliation: %s", mustJSON(report["reconciliation"]))
	t.Logf("criterion: %s", mustJSON(report["criterion"]))
}

// pricedCase is one labelled case, both arms priced.
type pricedCase struct {
	CaseID               string  `json:"case_id"`
	Origin               string  `json:"origin"`
	Labelled             string  `json:"labelled_route"`
	Decided              string  `json:"decided_route"`
	Attributed           float64 `json:"attributed_units"`
	Counterfactual       float64 `json:"counterfactual_units"`
	SynthesisCalls       int     `json:"synthesis_calls"`
	DecisionMatchesLabel bool    `json:"decision_matches_label"`
}

type routeTotals struct {
	Cases           int     `json:"cases"`
	SynthesisCalls  int     `json:"synthesis_calls"`
	AttributedUnits float64 `json:"attributed_units"`
	MeanAttributed  float64 `json:"mean_attributed_units"`
	Model           string  `json:"model"`
	ModelPriced     bool    `json:"model_priced"`
	Operation       string  `json:"operation_when_expensive"`
}

func buildCostReport(t *testing.T, cases []fallbackCase, source, digest string) map[string]any {
	t.Helper()
	// The synthesis payload every case is priced against: the question itself, and
	// the material a deep route would have spent a call on. It is the SAME payload
	// for both arms, which is what makes the counterfactual a number rather than an
	// estimate - and it is a fixture, so it is a guess at a payload rather than a
	// real one. Both facts are in the report.
	contexts := []SourceContext{
		{ID: "s1", Title: "سجل الاسباب", Text: "ذكر السجل الاول ان والده احمد بن عمر."},
		{ID: "s2", Title: "رواية ثانية", Text: "the same relation as a second account."},
	}

	priced := make([]pricedCase, 0, len(cases))
	byLabel := map[string]*routeTotals{}
	byDecision := map[string]*routeTotals{}
	for _, testCase := range cases {
		decision := FallbackRoute(RoutingRequest{
			Text:        testCase.Text,
			Context:     testCase.Context,
			Operation:   testCase.Operation,
			SourceCount: testCase.SourceCount,
		})
		work := RouteWork(decision, testCase.Text, contexts, ConfiguredSynthesisModel)
		row := pricedCase{
			CaseID:               testCase.CaseID,
			Origin:               testCase.Origin,
			Labelled:             testCase.ExpectedRoute,
			Decided:              decision.Route,
			Attributed:           roundUnits(work.AttributedUnits),
			Counterfactual:       roundUnits(work.CounterfactualUnits),
			SynthesisCalls:       len(work.Attributed),
			DecisionMatchesLabel: decision.Route == testCase.ExpectedRoute,
		}
		// Grouped twice: once by what the fixture's author labelled, once by what
		// the code decided. The two groupings are different numbers and a reader is
		// entitled to both.
		add(byLabel, testCase.ExpectedRoute, decision.Route, row)
		add(byDecision, decision.Route, decision.Route, row)
		priced = append(priced, row)
	}

	totalAttributed, totalCounterfactual, totalCalls := 0.0, 0.0, 0
	for _, row := range priced {
		totalAttributed += row.Attributed
		totalCounterfactual += row.Counterfactual
		totalCalls += row.SynthesisCalls
	}
	sumOfByLabel := 0.0
	for _, totals := range byLabel {
		sumOfByLabel += totals.AttributedUnits
	}
	reconciles := absUnits(sumOfByLabel-totalAttributed) < 1e-6

	originCounts := map[string]int{}
	labellers := map[string]int{}
	for _, testCase := range cases {
		originCounts[testCase.Origin]++
		labellers[testCase.LabelledBy]++
	}
	divergences := []map[string]any{}
	for _, row := range priced {
		if !row.DecisionMatchesLabel {
			divergences = append(divergences, map[string]any{
				"case_id":              row.CaseID,
				"labelled_route":       row.Labelled,
				"decided_route":        row.Decided,
				"attributed_units":     row.Attributed,
				"counterfactual_units": row.Counterfactual,
			})
		}
	}

	report := map[string]any{
		"report":        "synthetic cost attribution over the labelled routing set",
		"produced_by":   "TestCostMeasurementReport in services/core-api/internal/ai",
		"command":       "make cost-report",
		"report_commit": costReportCommit(),

		"what_this_is":     costReportWhatThisIs,
		"what_this_is_not": []string{costReportIsNotTraffic, costReportIsNotMoney, costReportIsNotQuestionMix, costReportDoesNotCloseCriterion},

		"units": map[string]any{
			"name": "dawha work unit (dwu)",
			"definition": "the work of carrying one thousand runes of this service's own JSON request payload " +
				"at unit weight, multiplied by the declared weight of the operation and of the model",
			"not": "not money, not a provider's billing unit, not tokens, and not comparable across models " +
				"without the model label",
			"why_this_input": "The counterfactual cost of a call that never happened has to be computable, and " +
				"it cannot depend on a provider response that was never received. Both arms are priced from " +
				"the same payload by the same function, which is what makes the counterfactual a number " +
				"rather than a story.",
			"recompute_by_hand": "units == (1 + ceil(payload_runes / 1000)) * operation_weight * model_weight; " +
				"both tables are in services/core-api/internal/ai/cost.go",
		},

		"cost_model": map[string]any{
			"source":            "services/core-api/internal/ai/cost.go",
			"single_source":     true,
			"operation_weights": weightsByName(operationWeights),
			"model_weights":     weightsByName(modelWeights),
			"weights_are": "DECLARED, not measured. No provider in this repository bills anything, so there " +
				"is no invoice to measure them against, and a reader who wants a real figure replaces the " +
				"table and re-runs the same counters.",
		},

		"providers": map[string]any{
			"configured":          []string{"deterministic (services/ai-research/app/main.py DeterministicProvider)"},
			"real_provider":       false,
			"credential_required": false,
			"synthesis_model":     string(ConfiguredSynthesisModel),
			"routing_model":       "go-deterministic-routing-v1 (this package's fallback, which runs no model at all)",
			"when_a_real_provider_is_configured": "The same counters produce the real-traffic number with no " +
				"code change to the recording path. That is a MECHANISM, not a result: the cost model " +
				"would need that provider's weights in model_weights, and until it does, a series labelled " +
				"unpriced is the request SIZE and not a price. See docs/cost-measurement.md.",
		},

		"fixture_set": map[string]any{
			"path":                          source,
			"sha256":                        digest,
			"cases":                         len(cases),
			"by_origin":                     originCounts,
			"by_labeller":                   labellers,
			"labels":                        "internally_authored",
			"derived_from_a_real_query_set": false,
			"limitation":                    costReportLabelLimitation,
		},

		"workload": map[string]any{
			"routing_implementation": "services/core-api/internal/ai/routing.go FallbackRoute",
			"why_the_fallback": "It is this repository's routing decision, it needs no provider and no " +
				"network, and it is what a deployment actually runs when the route call is unavailable. " +
				"Its decisions are grouped separately from the reviewed labels, and the two groupings " +
				"differ by label_decision_divergences, which is reported rather than smoothed away.",
			"synthesis_payload": map[string]any{
				"contexts":      len(contexts),
				"source":        "a fixed two-passage payload in this test",
				"is_a_real_one": false,
				"note": "The same payload is priced in both arms. It is a fixture, so a real payload mix " +
					"would move the absolute figures - though not the shape, because both arms scale with it.",
			},
		},
		"attributed_cost_by_labelled_route": totalsByName(byLabel),
		"attributed_cost_by_decision":       totalsByName(byDecision),
		"label_decision_divergences":        divergences,
		"label_decision_divergences_note":   costReportDivergenceNote(divergences),

		"reduction": map[string]any{
			"counterfactual":                   "every labelled question answered by the deep path instead",
			"counterfactual_units":             roundUnits(totalCounterfactual),
			"attributed_units":                 roundUnits(totalAttributed),
			"difference_units":                 roundUnits(totalCounterfactual - totalAttributed),
			"is_a_measured_reduction_in_spend": false,
			"why_not":                          costReportDifferenceIsNotASaving,
		},

		"reconciliation": map[string]any{
			"cases_priced":                      len(cases),
			"synthesis_calls_priced":            totalCalls,
			"attributed_units_summed_from_rows": roundUnits(totalAttributed),
			"attributed_units_summed_by_route":  roundUnits(sumOfByLabel),
			"holds":                             reconciles,
			"live_reconciliation_test":          "TestAttributedCostReconcilesWithTheNumberOfCallsMade",
			"what_it_proves": "The per-route totals are the per-case figures added up, so no figure here is " +
				"an average nobody can trace to a case, and `per_case` carries every case so a reader can " +
				"add them up again. The LIVE equivalent - the recorded counter against what the cost " +
				"model independently predicts, per series - is the test named above, in cost_test.go, " +
				"which runs on every build and is not this command.",
		},

		"criterion": map[string]any{
			"name":                  "measurable reduction in expensive model calls",
			"source":                "IMPLEMENTATION_PLAN.md:1368",
			"status":                "OPEN",
			"closed_by_this_report": false,
			"what_would_close_it": "The real-traffic measurement designed in docs/cost-measurement.md: " +
				"dawha_ai_cost_units over a stated window of production traffic, priced by the same cost " +
				"model, compared against the same traffic's all-deep counterfactual, at the threshold and " +
				"over the window the design fixes BEFORE the data exists. A synthetic figure does not " +
				"close it and this report does not claim to.",
			"who_collects_it": "The operator of a deployment, from the same counters, over a window long " +
				"enough for the confounds the design names - question mix over time, provider price " +
				"changes, and traffic volume - to be held still or corrected. This repository has no such " +
				"deployment, no production traffic and no configured provider, so it cannot be collected here.",
		},

		"per_case":     priced,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if !reconciles {
		t.Errorf("the per-route totals do not reconcile with the per-case figures: %v against %v", sumOfByLabel, totalAttributed)
	}
	return report
}

func add(totals map[string]*routeTotals, key, decided string, row pricedCase) {
	total, ok := totals[key]
	if !ok {
		total = &routeTotals{Model: string(ConfiguredSynthesisModel), ModelPriced: true}
		totals[key] = total
	}
	total.Cases++
	total.SynthesisCalls += row.SynthesisCalls
	total.AttributedUnits = roundUnits(total.AttributedUnits + row.Attributed)
	total.MeanAttributed = roundUnits(total.AttributedUnits / float64(total.Cases))
	if operation, sendsWork := routeWork[decided]; sendsWork {
		total.Operation = string(operation)
	}
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "unencodable"
	}
	return string(encoded)
}

// costReportCommit records the tree the report was produced from, so a committed
// report can be traced to a commit rather than trusted. It is not invented when the
// command was not told one: a report that claims provenance it was not given is
// exactly the habit this phase is correcting.
func costReportCommit() string {
	if commit := os.Getenv("DAWHA_COST_REPORT_COMMIT"); commit != "" {
		return commit
	}
	return "unspecified (set DAWHA_COST_REPORT_COMMIT to record the commit)"
}

func weightsByName[T ~string](weights map[T]float64) map[string]float64 {
	out := map[string]float64{}
	for name, weight := range weights {
		out[string(name)] = weight
	}
	return out
}

func totalsByName(totals map[string]*routeTotals) map[string]any {
	out := map[string]any{}
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		total := totals[name]
		if total.Operation == "" {
			total.Operation = "none: this route sends nothing to a model"
		}
		out[name] = total
	}
	return out
}

func roundUnits(value float64) float64 {
	// Six decimal places, so a sum of a hundred figures is exact to a millionth and
	// the reconciliation below is a real check rather than a rounding coincidence.
	return float64(int64(value*1e6+0.5)) / 1e6
}

func absUnits(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
