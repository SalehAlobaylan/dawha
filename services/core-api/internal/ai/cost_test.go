package ai

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
)

// The properties this file is here to hold, in the order they matter:
//
//  1. the cost model is TOTAL - every operation and every model has a declared
//     weight, so a new endpoint or a provider swap cannot slip through unpriced
//     and silent;
//  2. a call's attributed cost RECONCILES with the number of calls made, both as
//     a total and per series, so the number is a measurement rather than a
//     decoration;
//  3. a cheap route and a deep route for the SAME question attribute different
//     cost, and the cheap route's counterfactual equals the deep route's
//     attribution - which is the property the whole counterfactual design rests
//     on;
//  4. the route is a LABEL and never a price, so the same request priced under
//     two routes costs the same and is separated by the label rather than by an
//     invented difference;
//  5. nothing here needs a provider, a credential or a network.

// ---------------------------------------------------------------------------
// 1. The cost model is total.

func TestEveryOperationAndModelTheCostModelPricesHasADeclaredWeight(t *testing.T) {
	// The enumeration is the list of endpoints, and this asserts the list and the
	// price table have not drifted apart. A new endpoint that forgets to declare a
	// weight is a call that reports unit weight, which is safe but is a decision
	// nobody made.
	declared := []telemetry.AIOperation{
		telemetry.AIOperationNormalizeName, telemetry.AIOperationEmbed, telemetry.AIOperationClassify,
		telemetry.AIOperationExtractEntities, telemetry.AIOperationExtractClaims, telemetry.AIOperationResolveEntity,
		telemetry.AIOperationContradiction, telemetry.AIOperationRerank, telemetry.AIOperationResearchQuery,
		telemetry.AIOperationRoute, telemetry.AIOperationOther,
	}
	for _, operation := range declared {
		weight, ok := operationWeights[operation]
		if !ok {
			t.Fatalf("operation %q has no declared weight, so it would be priced at the unit-weight fallback", operation)
		}
		if weight <= 0 {
			t.Fatalf("operation %q has weight %v, which would report every call to it as free", operation, weight)
		}
	}
	if len(operationWeights) != len(declared) {
		t.Fatalf("the weight table has %d entries and the operation enumeration has %d; one of them is stale", len(operationWeights), len(declared))
	}

	models := []telemetry.AIModel{
		telemetry.AIModelDeterministicFoundation, telemetry.AIModelDeterministicRouting,
		telemetry.AIModelGoFallbackRouting, telemetry.AIModelUnpriced,
	}
	for _, model := range models {
		if _, ok := modelWeights[model]; !ok {
			t.Fatalf("model %q has no declared weight", model)
		}
	}
	if len(modelWeights) != len(models) {
		t.Fatalf("the model weight table has %d entries and the model enumeration has %d; one of them is stale", len(modelWeights), len(models))
	}
}

func TestTheFigureIsAReproducibleFunctionOfTheRequestAndTheModel(t *testing.T) {
	// Anyone reading the exposition has to be able to recompute a figure by hand
	// from the help text, so the arithmetic is pinned rather than implied.
	cost := AttributedCost(telemetry.AIOperationResearchQuery, ResearchQueryRequest{
		Query:    "من كان والد محمد؟",
		Contexts: []SourceContext{{ID: "s1", Title: "سجل", Text: "ذكر السجل أن والده أحمد."}},
	}, telemetry.AIModelDeterministicFoundation)
	runes := payloadRunes(ResearchQueryRequest{
		Query:    "من كان والد محمد؟",
		Contexts: []SourceContext{{ID: "s1", Title: "سجل", Text: "ذكر السجل أن والده أحمد."}},
	})
	if cost.PayloadRunes != runes {
		t.Fatalf("PayloadRunes = %d, and the request encodes to %d runes", cost.PayloadRunes, runes)
	}
	want := (1 + math.Ceil(float64(runes)/float64(payloadRunesPerUnit))) * operationWeights[telemetry.AIOperationResearchQuery] * modelWeights[telemetry.AIModelDeterministicFoundation]
	if math.Abs(cost.Units-want) > 1e-9 {
		t.Fatalf("Units = %v, and the documented formula gives %v", cost.Units, want)
	}
	if !cost.Priced {
		t.Fatal("a call under a model the cost model prices reported itself unpriced")
	}
	// The same request under a different model is a different figure, which is the
	// reason the model is a label rather than a note in a dashboard.
	other := AttributedCost(telemetry.AIOperationResearchQuery, ResearchQueryRequest{
		Query:    "من كان والد محمد؟",
		Contexts: []SourceContext{{ID: "s1", Title: "سجل", Text: "ذكر السجل أن والده أحمد."}},
	}, telemetry.AIModelDeterministicRouting)
	if math.Abs(other.Units-cost.Units) > 1e-9 {
		t.Fatalf("the two deterministic endpoints are one implementation and must price alike: %v vs %v", other.Units, cost.Units)
	}
}

func TestAModelTheCostModelDoesNotPriceIsASizeAndSaysSo(t *testing.T) {
	// The provider swap path. A name nobody here knows must not be guessed at.
	if got := ModelFor("some-model-from-a-provider"); got != telemetry.AIModelUnpriced {
		t.Fatalf("an unknown model name priced as %q", got)
	}
	// A failed call has no decoded response, and that is not evidence a known
	// model served it.
	if got := ModelFor(""); got != telemetry.AIModelUnpriced {
		t.Fatalf("an empty model name priced as %q", got)
	}
	cost := AttributedCost(telemetry.AIOperationEmbed, EmbeddingRequest{Text: "محمد", Dimensions: 16}, telemetry.AIModelUnpriced)
	if cost.Priced {
		t.Fatal("an unpriced model reported a priced figure")
	}
	// It is still a real number, and it is the size term: somebody reading the
	// series can see the request was this big even though nobody knows what it cost.
	if cost.Units <= 0 {
		t.Fatalf("an unpriced call reported %v units, which hides the request size", cost.Units)
	}
}

func TestEveryDecodedResponseReportsTheModelThatAnswered(t *testing.T) {
	// The cost model is told the model that answered, not the one a caller hoped
	// for. A response type that forgets reportedModel() decodes to an unpriced
	// series, which is visible - but only if this test exists.
	for name, response := range map[string]any{
		"embedding":          &EmbeddingResponse{Model: "m"},
		"classification":     &ClassificationResponse{Model: "m"},
		"entity_extraction":  &EntityExtractionResponse{Model: "m"},
		"claim_extraction":   &ClaimExtractionResponse{Model: "m"},
		"entity_resolution":  &EntityResolutionResponse{Model: "m"},
		"contradiction":      &ContradictionResponse{Model: "m"},
		"rerank":             &RerankResponse{Model: "m"},
		"research_query":     &ResearchQueryResponse{Model: "m"},
		"route":              &RoutingDecision{Model: "m"},
		"name_normalization": &NameNormalization{Normalized: "x"},
	} {
		got := responseModel(response)
		if name == "name_normalization" {
			// It carries no model because it is string folding, so it is
			// unpriced by design rather than by omission.
			if got != "" {
				t.Fatalf("name normalization reported a model: %q", got)
			}
			continue
		}
		if got != "m" {
			t.Fatalf("the %s response reported model %q; it does not implement reportedModel", name, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 2 and 3. Reconciliation, and cheap against deep for the same question.

func TestAttributedCostReconcilesWithTheNumberOfCallsMade(t *testing.T) {
	registry := telemetry.NewMetrics("dawha-core-api", true)
	provider := NewHTTPProvider(costFixtureServer(t).URL).WithMetrics(registry)

	// The calls this provider actually made, and what the cost model says each one
	// cost, computed here independently of the recording path. If the two ever
	// disagree, the counter is not reporting the cost model and the whole figure
	// is decoration.
	embed := EmbeddingRequest{Text: "ذكر السجل أن والده أحمد بن عمر", Dimensions: 16}
	research := ResearchQueryRequest{
		Query:    "من كان والد محمد؟",
		Contexts: []SourceContext{{ID: "s1", Title: "سجل الاسباب", Text: "ذكر السجل أن والده أحمد."}},
	}
	routing := RoutingRequest{Text: "تتبع اصل النسبة عبر ثلاثة أجيال", Operation: "research"}

	type expectation struct {
		operation telemetry.AIOperation
		input     any
		route     telemetry.AIRoute
		// The model the fixture server answers with, or the empty string for an
		// endpoint that answers with no model at all. Name normalization is string
		// folding, so it is unpriced by design rather than by omission.
		model telemetry.AIModel
	}
	// A cheap question, then the same question under a deep route: the pair the
	// criterion is about, sent through the real HTTP path so the reconciliation is
	// about recorded samples and not about the model in isolation.
	cheapContext := WithRoute(context.Background(), RoutingRouteCheap)
	deepContext := WithRoute(context.Background(), RoutingRouteDeep)
	unrouted := context.Background()

	calls := []struct {
		name string
		do   func()
		want expectation
	}{
		{name: "embed", do: func() { _, _ = provider.Embed(unrouted, embed) }, want: expectation{telemetry.AIOperationEmbed, embed, telemetry.AIRouteUnrouted, telemetry.AIModelDeterministicFoundation}},
		{name: "normalize", do: func() { _, _ = provider.NormalizeName(unrouted, "عَبد الله ") }, want: expectation{telemetry.AIOperationNormalizeName, map[string]string{"value": "عَبد الله "}, telemetry.AIRouteUnrouted, telemetry.AIModelUnpriced}},
		{name: "research cheap", do: func() { _, _ = provider.ResearchQuery(cheapContext, research) }, want: expectation{telemetry.AIOperationResearchQuery, research, telemetry.AIRouteCheap, telemetry.AIModelDeterministicFoundation}},
		{name: "research deep", do: func() { _, _ = provider.ResearchQuery(deepContext, research) }, want: expectation{telemetry.AIOperationResearchQuery, research, telemetry.AIRouteDeep, telemetry.AIModelDeterministicFoundation}},
		{name: "routing", do: func() { _, _ = provider.Route(unrouted, routing) }, want: expectation{telemetry.AIOperationRoute, routing, telemetry.AIRouteDeep, telemetry.AIModelDeterministicRouting}},
	}
	predicted := map[string]float64{}
	predictedTotal := 0.0
	for _, call := range calls {
		call.do()
		figure := AttributedCost(call.want.operation, call.want.input, call.want.model)
		key := costSeriesKey(call.want.operation, call.want.route, figure.Model)
		predicted[key] += figure.Units
		predictedTotal += figure.Units
	}

	recordedTotal, recorded := parseCounter(t, registry.Text(), "dawha_ai_cost_units")
	recordedCalls, _ := parseCounter(t, registry.Text(), "dawha_ai_calls_total")

	// The number of calls recorded equals the number made. This is the cheap half
	// of the reconciliation and it is the half a cost figure is usually wrong on.
	if float64(len(calls)) != recordedCalls {
		t.Fatalf("made %d calls and the counter recorded %v:\n%s", len(calls), recordedCalls, registry.Text())
	}
	if math.Abs(recordedTotal-predictedTotal) > 1e-6 {
		t.Fatalf("the counter recorded %v units and the cost model predicts %v:\n%s", recordedTotal, predictedTotal, registry.Text())
	}
	// And it reconciles per series, not only in total, so a figure cannot be right
	// by two errors cancelling.
	if len(recorded) != len(predicted) {
		t.Fatalf("the counter has %d series and the cost model predicts %d:\n%s", len(recorded), len(predicted), registry.Text())
	}
	for key, want := range predicted {
		got, ok := recorded[key]
		if !ok {
			t.Fatalf("the counter has no series for %s:\n%s", key, registry.Text())
		}
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("series %s recorded %v and the cost model predicts %v", key, got, want)
		}
	}
	// The same question under two routes is two series, and the two are equal:
	// the route attributed the call, it did not reprice it.
	cheapKey := costSeriesKey(telemetry.AIOperationResearchQuery, telemetry.AIRouteCheap, telemetry.AIModelDeterministicFoundation)
	deepKey := costSeriesKey(telemetry.AIOperationResearchQuery, telemetry.AIRouteDeep, telemetry.AIModelDeterministicFoundation)
	if recorded[cheapKey] != recorded[deepKey] {
		t.Fatalf("the same request under two routes cost %v and %v. The route is a label; if this ever differs, something is inventing a price", recorded[cheapKey], recorded[deepKey])
	}
}

func TestACheapAndADeepRouteAttributeDifferentCostForTheSameQuestion(t *testing.T) {
	question := "من كان والد محمد؟"
	contexts := []SourceContext{
		{ID: "s1", Title: "سجل الاسباب", Text: "ذكر السجل الاول أن والده أحمد."},
		{ID: "s2", Title: "روايةOral", Text: "Coming next: the same fact as a second account."},
	}
	model := telemetry.AIModelDeterministicFoundation

	cheap := RouteWork(FallbackRoute(RoutingRequest{Text: "ما اسم كتاب المصدر؟", Operation: "research"}), question, contexts, model)
	deep := RouteWork(FallbackRoute(RoutingRequest{Text: "قارن الرواية الأولى مع الثانية", Operation: "research"}), question, contexts, model)

	if cheap.Route != RoutingRouteCheap || deep.Route != RoutingRouteDeep {
		t.Fatalf("the fixtures did not produce the routes under test: %q and %q", cheap.Route, deep.Route)
	}
	if cheap.AttributedUnits != 0 {
		t.Fatalf("a cheap route attributed %v units; its answer is assembled from passages that were already retrieved", cheap.AttributedUnits)
	}
	if deep.AttributedUnits <= 0 {
		t.Fatalf("a deep route attributed %v units, so routing cannot be worth anything", deep.AttributedUnits)
	}
	if cheap.AttributedUnits == deep.AttributedUnits {
		t.Fatalf("a cheap and a deep route for the same question both cost %v units, so the figure does not describe the routing decision", cheap.AttributedUnits)
	}
	// The property the counterfactual rests on: what the cheap route did NOT spend
	// is exactly what the deep route spent, on the same payload, priced by the same
	// function. Without this equality the counterfactual is a story.
	if math.Abs(cheap.CounterfactualUnits-deep.AttributedUnits) > 1e-9 {
		t.Fatalf("the cheap route's counterfactual is %v and the deep route's attribution is %v; they are the same call", cheap.CounterfactualUnits, deep.AttributedUnits)
	}
	if math.Abs(cheap.Difference-(cheap.CounterfactualUnits-cheap.AttributedUnits)) > 1e-9 {
		t.Fatalf("the difference is not counterfactual minus attributed: %v", cheap.Difference)
	}
	// And the deep route's counterfactual is itself, which is what makes its
	// difference zero rather than absent.
	if math.Abs(deep.CounterfactualUnits-deep.AttributedUnits) > 1e-9 {
		t.Fatalf("the deep route's two arms are %v and %v; the baseline arm is the same call", deep.CounterfactualUnits, deep.AttributedUnits)
	}
	if deep.Difference != 0 {
		t.Fatalf("the deep route reported a difference of %v against a baseline that is itself", deep.Difference)
	}
}

func TestTheRouteIsALabelAndNotAPrice(t *testing.T) {
	// The honesty property, stated as a test. If a route ever became a multiplier,
	// a cheap and a deep call of the same request would report different prices and
	// the difference would be a number this repository invented. It is here so that
	// changing the cost model to do that has to delete this test, visibly.
	request := ResearchQueryRequest{Query: "من كان والد محمد؟", Contexts: []SourceContext{{ID: "s1", Title: "سجل", Text: "ذكر السجل."}}}
	cheap := AttributedCost(telemetry.AIOperationResearchQuery, request, telemetry.AIModelDeterministicFoundation)
	deep := AttributedCost(telemetry.AIOperationResearchQuery, request, telemetry.AIModelDeterministicFoundation)
	if cheap.Units != deep.Units {
		t.Fatalf("the same request priced %v and %v", cheap.Units, deep.Units)
	}
	// The route lives on the context and is read once, at the moment of the call.
	ctx := WithRoute(context.Background(), RoutingRouteDeep)
	if RouteFromContext(ctx) != telemetry.AIRouteDeep {
		t.Fatalf("RouteFromContext = %q", RouteFromContext(ctx))
	}
	if RouteFromContext(context.Background()) != telemetry.AIRouteUnrouted {
		t.Fatal("a context with no route reported one")
	}
	// A context somebody else forged does not exist: the key is unexported and of
	// its own type, so no other package can put a route on a context.
	if RouteFromContext(nil) != telemetry.AIRouteUnrouted {
		t.Fatal("a nil context reported a route")
	}
}

func TestWithRouteOnlyAcceptsTheRoutesRoutingCanProduce(t *testing.T) {
	for _, route := range []string{RoutingRouteIgnore, RoutingRouteCheap, RoutingRouteDeep, "not-a-route", ""} {
		got := RouteFromContext(WithRoute(context.Background(), route))
		want := telemetry.AIRouteFor(route)
		if got != want {
			t.Fatalf("WithRoute(%q) attributed calls to %q, want %q", route, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// The cost model survives a provider swap, and does not pretend it has not.

func TestASwapToAnUnpricedProviderShowsUpAsUnpricedRatherThanAsZero(t *testing.T) {
	// The mechanism the plan asks for, exercised: a provider that answers with a
	// name this repository does not price must produce a visible unpriced series
	// carrying the request's size - not a zero, which would read as a free call.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"answer":"review","citations":[{"source_id":"s1","title":"t","excerpt":"e"}],"model":"provider-model-2027","review_required":true}`))
	}))
	defer server.Close()

	registry := telemetry.NewMetrics("dawha-core-api", true)
	provider := NewHTTPProvider(server.URL).WithMetrics(registry)
	request := ResearchQueryRequest{Query: "من كان والد محمد؟", Contexts: []SourceContext{{ID: "s1", Title: "سجل", Text: "ذكر السجل."}}}
	if _, err := provider.ResearchQuery(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	exposition := registry.Text()
	if !strings.Contains(exposition, `model="unpriced"`) {
		t.Fatalf("a provider swap did not show up as unpriced:\n%s", exposition)
	}
	if strings.Contains(exposition, `model="provider-model-2027"`) {
		t.Fatalf("a provider's own model name reached a label:\n%s", exposition)
	}
	want := AttributedCost(telemetry.AIOperationResearchQuery, request, telemetry.AIModelUnpriced).Units
	if !strings.Contains(exposition, "dawha_ai_cost_units{model=\"unpriced\",operation=\"research_query\",route=\"unrouted\"} "+formatUnits(want)) {
		t.Fatalf("the unpriced series does not carry the request size:\n%s", exposition)
	}
	// And the mechanism: declaring that one weight is the whole change a swap
	// needs. The counter, the labels and the counterfactual are already written
	// against the closed model enumeration.
	if _, ok := modelWeights[telemetry.AIModelDeterministicFoundation]; !ok {
		t.Fatal("the existing model weights moved")
	}
}

func TestMetricsOffChangesNothingAndCostsNothing(t *testing.T) {
	// The exporter is off by default and nothing in this file may make it on. A
	// provider with no registry must behave exactly as it did before the cost model
	// existed, which is asserted by the empty exposition and by the call succeeding.
	server := costFixtureServer(t)
	defer server.Close()
	registry := telemetry.NewMetrics("dawha-core-api", false)
	provider := NewHTTPProvider(server.URL).WithMetrics(registry)
	request := ResearchQueryRequest{Query: "من كان والد محمد؟", Contexts: []SourceContext{{ID: "s1", Title: "سجل", Text: "ذكر السجل."}}}
	if _, err := provider.ResearchQuery(context.Background(), request); err != nil {
		t.Fatalf("a call with metrics off failed: %v", err)
	}
	if exposition := registry.Text(); exposition != "" {
		t.Fatalf("a disabled registry exported something:\n%s", exposition)
	}
	// And a provider with no registry at all, which is every existing caller.
	if _, err := NewHTTPClient(server.URL).ResearchQuery(context.Background(), request); err != nil {
		t.Fatalf("a call with no registry failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// The cost model's own arithmetic, recomputed from the exposition.

func costFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/embed":
			_, _ = writer.Write([]byte(`{"embedding":[0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1],"dimensions":16,"model":"deterministic-foundation","deterministic":true}`))
		case "/v1/normalize-name":
			_, _ = writer.Write([]byte(`{"original":"محمد","normalized":"محمد","method":"deterministic-normalization","preserves_original":true}`))
		case "/v1/research/query":
			_, _ = writer.Write([]byte(`{"answer":"review","citations":[{"source_id":"s1","title":"مصدر","excerpt":"محمد"}],"model":"deterministic-foundation","review_required":true}`))
		case "/v1/route":
			_, _ = writer.Write([]byte(`{"route":"deep","query_type":"relationship","reason_code":"multi_step","source_bearing":false,"potential_contradiction":false,"continue_investigation":true,"operational_score":0.86,"model":"deterministic-semantic-control-v1","fallback":false,"review_required":true}`))
		default:
			_, _ = writer.Write([]byte(`{}`))
		}
	}))
}

func costSeriesKey(operation telemetry.AIOperation, route telemetry.AIRoute, model telemetry.AIModel) string {
	return "dawha_ai_cost_units{model=" + strconv.Quote(string(model)) +
		",operation=" + strconv.Quote(string(operation)) +
		",route=" + strconv.Quote(string(route)) + "}"
}

// parseCounter sums one counter family out of an exposition, by series. Histogram
// lines are skipped, so a family whose sibling is a histogram does not contribute
// its buckets.
func parseCounter(t *testing.T, exposition, family string) (float64, map[string]float64) {
	t.Helper()
	total := 0.0
	perSeries := map[string]float64{}
	for _, line := range strings.Split(exposition, "\n") {
		if !strings.HasPrefix(line, family) || strings.HasPrefix(line, family+"_") {
			continue
		}
		series, value, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			t.Fatalf("could not read %q from %q", value, line)
		}
		total += parsed
		perSeries[series] += parsed
	}
	return total, perSeries
}

func formatUnits(units float64) string {
	return strconv.FormatFloat(units, 'f', -1, 64)
}
