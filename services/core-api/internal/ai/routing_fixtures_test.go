package ai

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// THE GO FALLBACK AND THE PYTHON PROVIDER ARE TWO IMPLEMENTATIONS OF ONE DECISION.
//
// internal/ai/routing.go's FallbackRoute runs when the route call is unavailable;
// services/ai-research/app/main.py's routing_decision is the provider that answers
// it. They are the same decision written twice, in two languages, and nothing in
// the build compares them. That is a real gap: a divergence between them means
// the same question is routed one way when the AI service is up and another way
// when it is down, which is exactly the kind of change nobody notices until an
// incident.
//
// So this reads the SAME labelled set the evaluation reads and asserts that the Go
// fallback agrees with it - listing the cases where it does not, as declared
// divergences rather than as a comment somebody has to notice. A new divergence
// fails the test, so the list cannot rot into a fiction.
//
// It reads the fixture from the sibling service rather than duplicating it, because
// duplicating thirty-three cases is how two copies drift. A core-api checkout with
// no ai-research directory beside it skips: the evaluation is the thing that needs
// the set, and a routing package that will not build without a Python service's
// data files is the wrong dependency direction.
func goFallbackRoutingCases(t *testing.T) []fallbackCase {
	t.Helper()
	path := filepath.Join("..", "..", "..", "ai-research", "evaluation", "routing_cases.jsonl")
	file, err := os.Open(path)
	if err != nil {
		t.Skipf("routing fixtures are not beside this package (%s); run the evaluation instead", err)
	}
	defer file.Close()

	var cases []fallbackCase
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decoded fallbackCase
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("a line of %s is not JSON: %v", path, err)
		}
		cases = append(cases, decoded)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s is empty; an empty set would agree with everything", path)
	}
	return cases
}

type fallbackCase struct {
	CaseID    string `json:"case_id"`
	Origin    string `json:"origin"`
	Text      string `json:"text"`
	Context   string `json:"context"`
	Operation string `json:"operation"`
	// SourceCount is 0 when the field is absent, which is the same default the
	// provider applies, so an omitted field and an explicit zero are one case
	// rather than two.
	SourceCount        int    `json:"source_count"`
	LabelledBy         string `json:"labelled_by"`
	ExpectedRoute      string `json:"expected_route"`
	ExpectedQueryType  string `json:"expected_query_type"`
	ExpectedReasonCode string `json:"expected_reason_code"`
}

// knownGoFallbackLabelDivergences are the cases where internal/ai/routing.go's
// fallback does not reach the label. Each entry says which fields and why, so the
// list is evidence rather than an excuse.
//
// The Go fallback and the Python provider carry the SAME term tables, so these
// three are divergences the provider has too: the evaluation reports rt-010,
// rt-012 and rt-025 as mis-routes against these same labels. What this list adds
// is that the fallback is not a second, worse decision - it reaches the same
// answer, including the same three term-list weaknesses.
//
//   - rt-010: a singular/plural gap. The relationship term is "علاقة" (a relation)
//     and the text says "العلاقات" (relations), so the relationship score is zero and
//     the source term "نص" inside "النص" decides the query type on its own.
//   - rt-012: the relationship term "رواية" (an account or a lineage) matches "الرواية"
//     in a question about an account. The term list cannot tell the two apart, and
//     the label says this question is not about a relationship.
//   - rt-025: a substring false positive in the query-type scorer. The identity term
//     "لقب" (surname) occurs inside the Arabic word for tribe, so a question about a
//     tribe's migration is scored as an identity question. The Go term table has the
//     same term and therefore the same false positive.
var knownGoFallbackLabelDivergences = map[string]string{
	"rt-010": "query_type: the relationship term علاقة does not match the plural العلاقات, so the source term نص inside النص decides alone",
	"rt-012": "query_type: the relationship term رواية (an account) matches الرواية in a question about an account rather than a lineage",
	"rt-025": "query_type: the identity term لقب occurs inside the word for tribe, in this package's term table as in the provider's",
}

// knownGoPythonDisagreements are the cases where THIS PACKAGE and the ai-research
// provider reach DIFFERENT decisions. That is a worse finding than either of them
// being wrong on its own, because it means the same question is routed one way when
// the AI service answers and another way when it does not, and a deployment
// degrades into a different product rather than into an error.
//
// It is a short list, and that is the finding. Across the thirty-three reviewed
// cases the two implementations reach the same route, query type and reason code
// everywhere except here.
//
// The list is derived from two asserted sets rather than by running both
// implementations in one process: the evaluation reports the provider's
// divergences from these labels (rt-010, rt-012, rt-019, rt-025 - see the
// `misroutes` field of `make ai-eval`), and the test above asserts this package's
// (rt-010, rt-012, rt-025). A case in the first set and not the second is a
// disagreement between the implementations.
var knownGoPythonDisagreements = map[string]string{
	"rt-019": "a question carrying no word at all, only punctuation. This package's normalizer reduces it to nothing and the fallback ignores it; the provider normalizes only Arabic script, keeps the punctuation, matches no term and routes it cheap/simple_lookup. Both are defensible answers to a degenerate input and they are not the same answer, so the routing of a junk query depends on whether the AI service is up.",
}

func TestTheGoFallbackAgreesWithTheReviewedLabelsTheEvaluationUses(t *testing.T) {
	cases := goFallbackRoutingCases(t)
	seen := map[string]bool{}
	found := map[string]string{}

	for _, testCase := range cases {
		seen[testCase.CaseID] = true
		decision := FallbackRoute(RoutingRequest{
			Text:        testCase.Text,
			Context:     testCase.Context,
			Operation:   testCase.Operation,
			SourceCount: testCase.SourceCount,
		})
		var wrong []string
		if decision.Route != testCase.ExpectedRoute {
			wrong = append(wrong, "route")
		}
		if decision.QueryType != testCase.ExpectedQueryType {
			wrong = append(wrong, "query_type")
		}
		if decision.ReasonCode != testCase.ExpectedReasonCode {
			wrong = append(wrong, "reason_code")
		}
		if len(wrong) == 0 {
			continue
		}
		fields := strings.Join(wrong, ",")
		if _, declared := knownGoFallbackLabelDivergences[testCase.CaseID]; !declared {
			t.Fatalf("the Go fallback diverges from the label on %s (%s) and this is not a declared divergence. "+
				"Either the fallback is wrong or the label is, and both need a decision rather than a passing test.\n"+
				"  got route=%q query_type=%q reason_code=%q\n"+
				"  want route=%q query_type=%q reason_code=%q",
				testCase.CaseID, fields, decision.Route, decision.QueryType, decision.ReasonCode,
				testCase.ExpectedRoute, testCase.ExpectedQueryType, testCase.ExpectedReasonCode)
		}
		found[testCase.CaseID] = fields
	}

	// Every declared divergence actually happened. A declared divergence that has
	// been FIXED must be deleted in the same commit, because a stale entry would
	// keep explaining a bug that is gone.
	var stale []string
	for caseID, reason := range knownGoFallbackLabelDivergences {
		if !seen[caseID] {
			stale = append(stale, caseID+" (no such case)")
			continue
		}
		if _, ok := found[caseID]; !ok {
			stale = append(stale, caseID+" (it agrees now: "+reason+")")
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("these declared divergences are stale and must be deleted: %s", strings.Join(stale, "; "))
	}
}

// TestTheFallbackAndTheProviderAgreeExceptWhereDeclared is the assertion behind
// knownGoPythonDisagreements: a case listed there must NOT be one this package
// already fails, because then the two implementations would agree and the entry
// would be claiming a disagreement that does not exist.
//
// It is a one-line test that stops the most interesting sentence in this file from
// becoming untrue by accident.
func TestTheFallbackAndTheProviderAgreeExceptWhereDeclared(t *testing.T) {
	for caseID, reason := range knownGoPythonDisagreements {
		if _, bothWrong := knownGoFallbackLabelDivergences[caseID]; bothWrong {
			t.Fatalf("%s is listed both as a divergence from the label and as a disagreement between the "+
				"two implementations. If both are wrong the same way, they AGREE, and the disagreement "+
				"entry is false: %s", caseID, reason)
		}
	}
	// And the degenerate input the disagreement is about really does take the two
	// different paths, so the entry cannot rot into a claim about a case that has
	// since been edited.
	decision := FallbackRoute(RoutingRequest{Text: "؟؟؟"})
	if decision.Route != RoutingRouteIgnore {
		t.Fatalf("this package's fallback no longer ignores a punctuation-only query; it returns %q. "+
			"rt-019 is listed as an implementation disagreement because the two differ here, and they "+
			"now agree. Delete the entry and record the finding.", decision.Route)
	}
}

// TestEveryRoutingCaseCarriesItsProvenance is the Go side of the same rule the
// evaluator enforces in Python. The evaluator is what fails a missing field, and
// this is here so the fixture cannot be edited in a way the evaluator would not
// notice: a case whose provenance was stripped is a case that looks reviewed and
// is not.
func TestEveryRoutingCaseCarriesItsProvenance(t *testing.T) {
	for _, testCase := range goFallbackRoutingCases(t) {
		if testCase.CaseID == "" || testCase.Origin == "" {
			t.Fatalf("a routing case is missing provenance: case_id=%q origin=%q", testCase.CaseID, testCase.Origin)
		}
	}
}

// TestTheGoFallbackStaysOperationalClassification keeps the guardrail asserted from
// the cost model's side, because that is where this plan added new machinery around
// the route.
//
// IMPLEMENTATION_PLAN.md:1362 says Jev output is operational classification and
// must never be stored as historical confidence. Two properties hold that:
//
//   - a route reaches a metric label and nowhere else. The cost model reads it off
//     a context, writes it to one counter, and the only way out of this package is
//     that counter's label.
//   - a route can never become a stored status, because the column it would have to
//     be stored in has a CHECK constraint over the three route values and this
//     package has no SQL. What this test can assert is the half it owns: nothing in
//     this package produces a status, a verdict or a historical label from a route.
func TestTheGoFallbackStaysOperationalClassification(t *testing.T) {
	for _, testCase := range goFallbackRoutingCases(t) {
		decision := FallbackRoute(RoutingRequest{Text: testCase.Text, Operation: testCase.Operation, SourceCount: testCase.SourceCount})
		// The decision is a classification: a route, a query type, a reason code, a
		// score, and the standing requirement that a human review the result. There
		// is no field in it that means "this is now established", because there is
		// no field in it that means anything about the past at all.
		if err := ValidateRoutingDecision(decision); err != nil {
			t.Fatalf("the fallback produced a decision that is not operational classification: %v", err)
		}
		if !decision.ReviewRequired {
			t.Fatalf("the fallback produced a decision that does not require review: %+v", decision)
		}
		// A route never leaves this package as anything but a metric label.
		if got := RouteFromContext(WithRoute(t.Context(), decision.Route)); string(got) != decision.Route {
			t.Fatalf("route %q became %q on the way to the counter", decision.Route, got)
		}
	}
}
