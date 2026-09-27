package ai

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// THE GO FALLBACK AND THE PYTHON PROVIDER ARE TWO IMPLEMENTATIONS OF ONE DECISION.
//
// internal/ai/routing.go's FallbackRoute runs when the route call is unavailable;
// services/ai-research/app/main.py's routing_decision is the provider that answers
// it. They are the same decision written twice, in two languages, and they share a
// VOCABULARY rather than a function: a shared runtime between a Go API and a
// Python service is a dependency this repository should not take, and the two
// languages cannot be merged either. So the decision stays written twice and the
// agreement between the two is a test.
//
// WHY THAT IS NOT A COMMENT.
//
// A comment is an assertion nobody runs. The failure this file exists to prevent
// is concrete: rt-019, a query carrying nothing but punctuation. Go's
// normalizer folded punctuation to nothing and ignored the query; the provider
// kept the punctuation, matched no term, and routed it cheap. The same question
// therefore took one path with the AI service up and another with it down, and
// the only thing in the build that noticed was a declared list in a test file -
// which was right, and which had to be read to be believed.
//
// THE TEST NOW DERIVES BOTH DECISIONS.
//
// The declared list could only ever be as good as whoever wrote it. The test
// below runs the provider itself, over the same thirty-three labelled cases, and
// compares the two decisions field by field for every one of them. The
// divergence set is therefore derived rather than declared, and a new divergence
// fails the build instead of waiting to be noticed.
//
// THE AUTHORITY, since a reader should not have to guess which number came from
// where.
//
//   - For a ROUTE, the labelled case is the authority: expected_route in
//     routing_cases.jsonl says what a question of that shape should do, and both
//     implementations are measured against it. Three cases disagree with their
//     own label and are kept; see knownGoFallbackLabelDivergences.
//   - For the NORMALIZATION the two implementations share, this package's
//     identity.NormalizeArabicName is the specification, and the provider's
//     normalized_routing_text is that function's step list. Its comment carries
//     the same list.
//   - Neither implementation is the authority over the other. The provider
//     answers when it is up; the fallback answers when it is not; and the
//     property that matters is that a question does not change route because of
//     which one answered.
//
// It reads the fixture from the sibling service rather than duplicating it,
// because duplicating thirty-three cases is how two copies drift. A core-api
// checkout with no ai-research directory beside it skips: the evaluation is the
// thing that needs the set, and a routing package that will not build without a
// Python service's data files is the wrong dependency direction.
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
	// Fallback marks a case that is routed through the fallback path, which is
	// what the Go side has for every case and what the provider is asked for on
	// exactly these cases.
	Fallback bool `json:"fallback"`
}

// providerDecision is what the Python provider decided for one case.
type providerDecision struct {
	CaseID     string `json:"case_id"`
	Route      string `json:"route"`
	QueryType  string `json:"query_type"`
	ReasonCode string `json:"reason_code"`
}

// providerRoutingDecisions asks the ai-research service what IT decides for
// every labelled case, by running the module that exists for the purpose.
//
// It is a subprocess rather than an HTTP call because the point is to compare
// two implementations, and an HTTP round trip would compare this package's client
// against the provider, which is a third thing. It is the same interpreter the
// evaluation runs on, so there is one provider and one set of fixtures.
//
// Skips, rather than fails, when the interpreter is not beside this package. The
// same rule as the fixture reader above, and for the same reason: the evaluation
// is what needs the Python service, and a Go package must not require one to
// build. `make test` runs both halves - `go test ./...` and the service's own
// pytest - so a checkout that has the service installed runs both comparisons.
func providerRoutingDecisions(t *testing.T) map[string]providerDecision {
	t.Helper()
	serviceDir := filepath.Join("..", "..", "..", "ai-research")
	interpreter := filepath.Join(serviceDir, ".venv", "bin", "python")
	if _, err := os.Stat(interpreter); err != nil {
		t.Skipf("the ai-research interpreter is not beside this package (%s); `make install` creates it and the service's own tests cover the provider half", err)
	}
	// Absolute, because the command runs in the service directory and os/exec
	// resolves a relative Path against the working directory it was given, not
	// against the process's own. Resolved from the package directory, the same
	// path would climb out of the repository and fail with a bare ENOENT.
	absolute, err := filepath.Abs(interpreter)
	if err != nil {
		t.Fatalf("resolve the ai-research interpreter: %v", err)
	}
	command := exec.Command(absolute, "-m", "evaluation.provider_routes")
	command.Dir = serviceDir
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("ask the provider for its decisions: %v\n%s", err, stderr.String())
	}
	var answers []providerDecision
	if err := json.Unmarshal(output, &answers); err != nil {
		t.Fatalf("the provider's decisions are not JSON: %v\n%s", err, output)
	}
	if len(answers) == 0 {
		t.Fatal("the provider returned no decisions; an empty set would agree with everything")
	}
	byCase := make(map[string]providerDecision, len(answers))
	for _, answer := range answers {
		byCase[answer.CaseID] = answer
	}
	return byCase
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
//   - rt-012: the relationship term "رواية" (an account or a lineage) matches
//     "الرواية" in a question about an account rather than a lineage.
//   - rt-025: a substring false positive in the query-type scorer. The identity term
//     "لقب" (surname) occurs inside the Arabic word for tribe, so a question about a
//     tribe's migration is scored as an identity question.
//
// These are a shared weakness of ONE vocabulary, which is what sharing a
// vocabulary means by construction: a defect in the term tables is a defect in
// both. They are not a disagreement between the two implementations, and
// TestTheGoFallbackAndTheProviderReachTheSameDecisionOnEveryLabelledCase says so
// by deriving both decisions rather than inferring it from two reports.
var knownGoFallbackLabelDivergences = map[string]string{
	"rt-010": "query_type: the relationship term علاقة does not match the plural العلاقات, so the source term نص inside النص decides alone",
	"rt-012": "query_type: the relationship term رواية (an account) matches الرواية in a question about an account rather than a lineage",
	"rt-025": "query_type: the identity term لقب occurs inside the word for tribe, in this package's term table as in the provider's",
}

// TestTheGoFallbackAndTheProviderReachTheSameDecisionOnEveryLabelledCase is the
// pin. It derives BOTH decisions for every labelled case and requires the
// divergence set to be empty.
//
// The empty set is the property, and it is checked in two directions so neither
// side can drift alone: a case the provider decides that this package does not
// decide the same way, and a case this package decides that the provider does
// not. Both are named in the failure message, because "they disagree somewhere"
// is not a finding anybody can act on.
//
// There is no allow-list. A declared divergence is what this file used to carry,
// and a list is a thing a reader has to believe; deriving the set means a new
// divergence cannot be absorbed by adding a line. The vocabulary the two share
// is the place to fix a disagreement - the term tables, or the normalization -
// and both of those are visible in the failure message below.
func TestTheGoFallbackAndTheProviderReachTheSameDecisionOnEveryLabelledCase(t *testing.T) {
	cases := goFallbackRoutingCases(t)
	provider := providerRoutingDecisions(t)

	diverged := make([]string, 0)
	for _, testCase := range cases {
		answer, answered := provider[testCase.CaseID]
		if !answered {
			t.Fatalf("the provider returned no decision for %s. Every labelled case has to be answered by both implementations, or the comparison is over a smaller set than the labels.", testCase.CaseID)
		}
		here := FallbackRoute(RoutingRequest{
			Text:        testCase.Text,
			Context:     testCase.Context,
			Operation:   testCase.Operation,
			SourceCount: testCase.SourceCount,
		})
		var wrong []string
		if here.Route != answer.Route {
			wrong = append(wrong, "route")
		}
		if here.QueryType != answer.QueryType {
			wrong = append(wrong, "query_type")
		}
		if here.ReasonCode != answer.ReasonCode {
			wrong = append(wrong, "reason_code")
		}
		if len(wrong) == 0 {
			continue
		}
		diverged = append(diverged, strings.Join([]string{
			testCase.CaseID + " (" + strings.Join(wrong, ",") + ")",
			"go=" + here.Route + "/" + here.QueryType + "/" + here.ReasonCode,
			"provider=" + answer.Route + "/" + answer.QueryType + "/" + answer.ReasonCode,
		}, "  "))
	}
	if len(diverged) > 0 {
		sort.Strings(diverged)
		t.Fatalf("the Go fallback and the provider reach different decisions on %d of the %d labelled cases, so the route a question takes depends on whether the AI service is up:\n  %s\n\n"+
			"The shared vocabulary is where a fix belongs. This package's identity.NormalizeArabicName is the specification for the normalization and the provider's normalized_routing_text is its step list; "+
			"the term tables are duplicated in both files by design. Changing either side without the other is what this test is here to stop.",
			len(diverged), len(cases), strings.Join(diverged, "\n  "))
	}

	// A comparison over an empty set would pass. The provider has to have
	// answered every case, and the fixture has to still be the size it was.
	if len(provider) != len(cases) {
		t.Fatalf("the provider answered %d of the %d labelled cases, so the comparison above could pass without covering the set", len(provider), len(cases))
	}
}

// TestTheGoFallbackAgreesWithTheReviewedLabelsTheEvaluationUses holds this
// package against the labels directly, so a divergence from the provider can
// never be mistaken for an improvement.
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
