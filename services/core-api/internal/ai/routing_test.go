package ai

import (
	"strings"
	"testing"
)

func TestFallbackRouteUsesOperationalPaths(t *testing.T) {
	cases := []struct {
		name      string
		input     RoutingRequest
		route     string
		queryType string
		reason    string
	}{
		{name: "ignore", input: RoutingRequest{Text: "مرحبا"}, route: RoutingRouteIgnore, queryType: "general", reason: RoutingReasonNoise},
		{name: "cheap", input: RoutingRequest{Text: "ما اسم كتاب المصدر؟"}, route: RoutingRouteCheap, queryType: "source_evidence", reason: RoutingReasonSimpleLookup},
		{name: "deep", input: RoutingRequest{Text: "تتبع أصل النسبة عبر ثلاثة أجيال"}, route: RoutingRouteDeep, queryType: "relationship", reason: RoutingReasonMultiStep},
		{name: "contradiction", input: RoutingRequest{Text: "هل يوجد تناقض بين الروايتين؟"}, route: RoutingRouteDeep, queryType: "relationship", reason: RoutingReasonContradictionSignal},
		{name: "multi source", input: RoutingRequest{Text: "هل توجد علاقة بين المصدرين؟", SourceCount: 2}, route: RoutingRouteDeep, queryType: "relationship", reason: RoutingReasonMultiStep},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := FallbackRoute(testCase.input)
			if result.Route != testCase.route || result.QueryType != testCase.queryType || result.ReasonCode != testCase.reason {
				t.Fatalf("unexpected route: %+v", result)
			}
			if !result.Fallback || !result.ReviewRequired {
				t.Fatalf("expected explicit fallback and review: %+v", result)
			}
		})
	}
}

func TestValidateRoutingDecisionRejectsInvalidOperationalFields(t *testing.T) {
	valid := RoutingDecision{
		Route:            RoutingRouteCheap,
		QueryType:        "general",
		ReasonCode:       RoutingReasonSimpleLookup,
		OperationalScore: 0.5,
		Model:            "test",
		ReviewRequired:   true,
	}
	if err := ValidateRoutingDecision(valid); err != nil {
		t.Fatalf("expected valid decision: %v", err)
	}
	for _, invalid := range []RoutingDecision{
		{Route: "truth", QueryType: "general", ReasonCode: RoutingReasonSimpleLookup, Model: "test", ReviewRequired: true},
		{Route: RoutingRouteCheap, QueryType: "unknown", ReasonCode: RoutingReasonSimpleLookup, Model: "test", ReviewRequired: true},
		{Route: RoutingRouteCheap, QueryType: "general", ReasonCode: "free_form", Model: "test", ReviewRequired: true},
		{Route: RoutingRouteCheap, QueryType: "general", ReasonCode: RoutingReasonSimpleLookup, Model: "test", OperationalScore: 1.2, ReviewRequired: true},
		{Route: RoutingRouteCheap, QueryType: "general", ReasonCode: RoutingReasonSimpleLookup, Model: "test", ReviewRequired: false},
	} {
		if err := ValidateRoutingDecision(invalid); err == nil {
			t.Fatalf("expected invalid decision to fail: %+v", invalid)
		}
	}
}

func TestValidateRoutingRequest(t *testing.T) {
	if err := ValidateRoutingRequest(RoutingRequest{Text: "سؤال", Operation: "research"}); err != nil {
		t.Fatalf("expected valid request: %v", err)
	}
	if err := ValidateRoutingRequest(RoutingRequest{Text: strings.Repeat("س", 20000), Operation: "research"}); err != nil {
		t.Fatalf("expected Arabic rune limit to pass: %v", err)
	}
	for _, input := range []RoutingRequest{
		{Text: "سؤال", Operation: "unknown"},
		{Text: "", Operation: "research"},
		{Text: strings.Repeat("س", 20001), Operation: "research"},
		{Text: "سؤال", Operation: "research", SourceCount: -1},
	} {
		if err := ValidateRoutingRequest(input); err == nil {
			t.Fatalf("expected invalid request to fail: %+v", input)
		}
	}
}
