package ai

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
)

const (
	RoutingRouteIgnore = "ignore"
	RoutingRouteCheap  = "cheap"
	RoutingRouteDeep   = "deep"

	RoutingReasonNoise                 = "noise"
	RoutingReasonSimpleLookup          = "simple_lookup"
	RoutingReasonSourceContext         = "source_context"
	RoutingReasonMultiStep             = "multi_step"
	RoutingReasonContradictionSignal   = "contradiction_signal"
	RoutingReasonOperationRequiresDeep = "operation_requires_deep"
	RoutingReasonUncertainty           = "uncertainty"

	routeTimeout = 800 * time.Millisecond
)

var (
	routingOperations = map[string]struct{}{
		"research":            {},
		"search":              {},
		"suggestion":          {},
		"source_processing":   {},
		"duplicate_detection": {},
		"contradiction":       {},
	}
	routingQueryTypes = map[string]struct{}{
		"source_evidence": {},
		"identity":        {},
		"relationship":    {},
		"geography":       {},
		"general":         {},
	}
	routingReasonCodes = map[string]struct{}{
		RoutingReasonNoise:                 {},
		RoutingReasonSimpleLookup:          {},
		RoutingReasonSourceContext:         {},
		RoutingReasonMultiStep:             {},
		RoutingReasonContradictionSignal:   {},
		RoutingReasonOperationRequiresDeep: {},
		RoutingReasonUncertainty:           {},
	}
)

type RoutingRequest struct {
	Text        string `json:"text"`
	Context     string `json:"context,omitempty"`
	Operation   string `json:"operation"`
	SourceCount int    `json:"source_count"`
}

type RoutingDecision struct {
	Route                  string  `json:"route"`
	QueryType              string  `json:"query_type"`
	ReasonCode             string  `json:"reason_code"`
	SourceBearing          bool    `json:"source_bearing"`
	PotentialContradiction bool    `json:"potential_contradiction"`
	ContinueInvestigation  bool    `json:"continue_investigation"`
	OperationalScore       float64 `json:"operational_score"`
	Model                  string  `json:"model"`
	Fallback               bool    `json:"fallback"`
	ReviewRequired         bool    `json:"review_required"`
}

type RouteProvider interface {
	Route(context.Context, RoutingRequest) (RoutingDecision, error)
}

var _ RouteProvider = (*HTTPProvider)(nil)
var _ RouteProvider = (*Client)(nil)

func (p *HTTPProvider) Route(ctx context.Context, input RoutingRequest) (RoutingDecision, error) {
	input, err := normalizeRoutingRequest(input)
	if err != nil {
		return RoutingDecision{}, err
	}
	var result RoutingDecision
	if err := p.post(ctx, "/v1/route", input, &result); err != nil {
		return RoutingDecision{}, err
	}
	if err := ValidateRoutingDecision(result); err != nil {
		return RoutingDecision{}, err
	}
	return result, nil
}

func (c *Client) Route(ctx context.Context, input RoutingRequest) (RoutingDecision, error) {
	input, err := normalizeRoutingRequest(input)
	if err != nil {
		return RoutingDecision{}, err
	}
	timeout := routeTimeout
	if c != nil && c.RouteTimeout > 0 {
		timeout = c.RouteTimeout
	}
	if c == nil || c.Provider == nil {
		return RoutingDecision{}, ErrUnavailable
	}
	routeProvider, ok := c.Provider.(RouteProvider)
	if !ok {
		return RoutingDecision{}, ErrUnavailable
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return callClient(c, callCtx, func(providerCtx context.Context) (RoutingDecision, error) {
		return routeProvider.Route(providerCtx, input)
	}, ValidateRoutingDecision)
}

func FallbackRoute(input RoutingRequest) RoutingDecision {
	normalized, err := normalizeRoutingRequest(input)
	if err != nil {
		return RoutingDecision{
			Route:                 RoutingRouteDeep,
			QueryType:             "general",
			ReasonCode:            RoutingReasonUncertainty,
			OperationalScore:      0.5,
			Model:                 "go-deterministic-routing-v1",
			Fallback:              true,
			ReviewRequired:        true,
			ContinueInvestigation: true,
		}
	}
	value := strings.ToLower(identity.NormalizeArabicName(normalized.Text))
	queryType := fallbackQueryType(value)
	sourceBearing := normalized.SourceCount > 0 || strings.TrimSpace(normalized.Context) != ""
	contradiction := routingContainsAny(value, fallbackContradictionTerms) || normalized.Operation == "contradiction"
	route := RoutingRouteCheap
	reasonCode := RoutingReasonSimpleLookup
	score := 0.58
	_, isNoise := fallbackNoise[value]
	if value == "" || (!sourceBearing && isNoise) {
		route = RoutingRouteIgnore
		reasonCode = RoutingReasonNoise
		score = 0.05
	} else if contradiction {
		route = RoutingRouteDeep
		reasonCode = RoutingReasonContradictionSignal
		score = 0.9
	} else if normalized.Operation == "duplicate_detection" || normalized.Operation == "contradiction" {
		route = RoutingRouteDeep
		reasonCode = RoutingReasonOperationRequiresDeep
		score = 0.82
	} else if routingContainsAny(value, fallbackDeepTerms) {
		route = RoutingRouteDeep
		reasonCode = RoutingReasonMultiStep
		score = 0.86
	} else if sourceBearing && normalized.SourceCount >= 2 && (queryType == "relationship" || queryType == "identity") {
		route = RoutingRouteDeep
		reasonCode = RoutingReasonMultiStep
		score = 0.8
	} else if sourceBearing {
		route = RoutingRouteCheap
		reasonCode = RoutingReasonSourceContext
		score = 0.7
	}
	return RoutingDecision{
		Route:                  route,
		QueryType:              queryType,
		ReasonCode:             reasonCode,
		SourceBearing:          sourceBearing,
		PotentialContradiction: contradiction,
		ContinueInvestigation:  route == RoutingRouteDeep || contradiction || normalized.SourceCount >= 2,
		OperationalScore:       score,
		Model:                  "go-deterministic-routing-v1",
		Fallback:               true,
		ReviewRequired:         true,
	}
}

func normalizeRoutingRequest(input RoutingRequest) (RoutingRequest, error) {
	input.Text = strings.TrimSpace(input.Text)
	input.Context = strings.TrimSpace(input.Context)
	if input.Operation == "" {
		input.Operation = "research"
	}
	if err := ValidateRoutingRequest(input); err != nil {
		return RoutingRequest{}, err
	}
	return input, nil
}

func ValidateRoutingRequest(input RoutingRequest) error {
	if input.Text == "" || utf8.RuneCountInString(input.Text) > 20000 || utf8.RuneCountInString(input.Context) > 20000 || input.SourceCount < 0 || input.SourceCount > 10000 {
		return ErrValidation
	}
	if _, ok := routingOperations[input.Operation]; !ok {
		return ErrValidation
	}
	return nil
}

func ValidateRoutingDecision(result RoutingDecision) error {
	if _, ok := map[string]struct{}{RoutingRouteIgnore: {}, RoutingRouteCheap: {}, RoutingRouteDeep: {}}[result.Route]; !ok {
		return ErrValidation
	}
	if _, ok := routingQueryTypes[result.QueryType]; !ok {
		return ErrValidation
	}
	if _, ok := routingReasonCodes[result.ReasonCode]; !ok {
		return ErrValidation
	}
	if result.Model == "" || !result.ReviewRequired || !validScore(result.OperationalScore) {
		return ErrValidation
	}
	return nil
}

var fallbackQueryTerms = map[string][]string{
	"source_evidence": {"مصدر", "دليل", "نص", "صفحة", "سجل", "مرجع", "اقتباس", "source", "evidence", "document", "record"},
	"identity":        {"هوية", "شخص", "اسم", "لقب", "اسماء", "alias", "identity", "person"},
	"relationship": {
		"والد", "والدة", "ابو", "أبو", "ابن", "بنت", "زوج", "قرابة", "نسب", "علاقة", "تناقض", "تعارض", "رواية", "روايات", "father", "parent", "family",
	},
	"geography": {"مكان", "مدينة", "قرية", "هاجر", "هجرة", "مسار", "جغراف", "place", "location"},
}

var fallbackDeepTerms = []string{
	"تحقق", "تحقيق", "قارن", "مقارنة", "تناقض", "تعارض", "تتبع", "اثبات", "إثبات", "اصل", "مسار", "بحث", "investigate", "compare", "contradiction", "trace", "prove", "explain",
}

var fallbackContradictionTerms = []string{"والد", "أبو", "ابو", "father", "والدة", "mother", "تناقض", "تعارض", "contradiction", "conflict"}

var fallbackNoise = map[string]struct{}{
	"مرحبا": {}, "السلام عليكم": {}, "شكرا": {}, "شكرا لكم": {}, "كيف حالك": {}, "spam": {}, "اعلان": {},
}

func fallbackQueryType(value string) string {
	scores := make(map[string]int, len(fallbackQueryTerms))
	for queryType, terms := range fallbackQueryTerms {
		for _, term := range terms {
			if strings.Contains(value, identity.NormalizeArabicName(term)) {
				scores[queryType]++
			}
		}
	}
	bestType := "general"
	bestScore := 0
	for _, queryType := range []string{"relationship", "source_evidence", "identity", "geography"} {
		if scores[queryType] > bestScore {
			bestType = queryType
			bestScore = scores[queryType]
		}
	}
	return bestType
}

func routingContainsAny(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, identity.NormalizeArabicName(term)) {
			return true
		}
	}
	return false
}
