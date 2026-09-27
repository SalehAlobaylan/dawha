package ai

import (
	"context"
	"encoding/json"
	"math"
	"unicode/utf8"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
)

// THE COST MODEL.
//
// It is the only place in this repository where a price or a weight is expressed.
// Everything that reports a cost - the live counter, the reconciliation test, the
// measurement command, and the counterfactual of a call that never happened -
// goes through AttributedCost below. A second table that could disagree with this
// one is the failure this file exists to prevent, so there is deliberately no
// other one, and a test asserts the enumeration is total so a new operation or a
// new model cannot slip through unpriced.
//
// WHAT THE UNIT IS.
//
// One "dawha work unit" (dwu) is the work of carrying one thousand runes of this
// service's own request payload at unit weight. It is NOT money. It is NOT tokens.
// It is NOT an invoice, and no number derived from it may be described as any of
// those. Two facts make that non-negotiable rather than cautious:
//
//   - The only embedding provider this repository may use returns a SHA-512
//     digest of its input rather than a semantic vector (see the vector_baseline
//     note in services/ai-research/evaluation/evaluate.py), so a token count
//     taken from this repository's provider is a count of characters.
//   - No provider is configured at all, so there is no price list to read.
//
// A dwu is therefore a normalized, provider-independent weight: the thing the
// routing criterion actually needs, which is a way to say "this call was more
// expensive than that one" and to price a call that did not happen.
//
// WHY THE SIZE TERM IS THIS REPOSITORY'S OWN PAYLOAD.
//
// A cost that has to be comparable across models cannot depend on the model, and
// the counterfactual cost of a call that DID NOT HAPPEN has to be computable at
// all - which rules out anything derived from a provider response, because there
// is no response for a call nobody made. So the size term is the rune length of
// the JSON request this package already builds, which both arms of the comparison
// have. That is what makes the counterfactual a number rather than a story.
//
// WHAT THE WEIGHTS ARE.
//
// Declared, not measured. There is no invoice in this repository to measure them
// against. They are reviewable in one place, and a reader who wants a real figure
// replaces this table, re-runs the same counters, and gets it - which is the whole
// design: the mechanism does not change when a real provider arrives.

// payloadRunesPerUnit is the payload size that costs one unit at unit weight.
const payloadRunesPerUnit = 1000

// The model names this cost model prices. They are the names the deterministic
// provider answers with (services/ai-research/app/main.py) and the name the Go
// fallback stamps on its own decision. Anything else is unpriced.
const (
	modelNameDeterministicFoundation = "deterministic-foundation"
	modelNameDeterministicRouting    = "deterministic-semantic-control-v1"
	modelNameGoFallbackRouting       = "go-deterministic-routing-v1"
)

var (
	// operationWeight is the relative price of one operation as a multiplier on
	// the size term. Read it as a declaration of intent, checked in one place by
	// the ordering a reviewer can hold in their head: fold a name, decide a route,
	// classify, embed, resolve, extract, rerank, and then - last and by a factor
	// of two over the next - synthesize.
	operationWeights = map[telemetry.AIOperation]float64{
		// Name normalization is string folding. No model runs.
		telemetry.AIOperationNormalizeName: 0.25,
		// The routing decision itself. It gates everything downstream, so it is
		// cheap against what it decides and dear against folding a name.
		telemetry.AIOperationRoute:           0.5,
		telemetry.AIOperationClassify:        0.5,
		telemetry.AIOperationEmbed:           1.0,
		telemetry.AIOperationResolveEntity:   1.25,
		telemetry.AIOperationExtractClaims:   1.5,
		telemetry.AIOperationExtractEntities: 1.5,
		telemetry.AIOperationContradiction:   2.0,
		// Rerank scores up to a hundred documents against the question.
		telemetry.AIOperationRerank: 2.0,
		// The one call the cheap route exists to avoid: it carries the question
		// and up to twenty retrieved passages into a single generated answer.
		telemetry.AIOperationResearchQuery: 4.0,
		// An endpoint this package does not know. Charged at unit weight so a new
		// endpoint that forgets to declare a weight shows up as a real figure
		// rather than as silence, and so the enumeration can be tested as total.
		telemetry.AIOperationOther: 1.0,
	}

	// modelWeight is what one unit of work costs under one model. This is the
	// table a provider swap moves, and moving it is the ONLY change a swap needs
	// here: the counter, the labels and the counterfactual are already written
	// against the closed model enumeration.
	modelWeights = map[telemetry.AIModel]float64{
		// One deterministic implementation reached by two endpoints. The
		// endpoints carry the cost, not the names, so both are 1.0.
		telemetry.AIModelDeterministicFoundation: 1.0,
		telemetry.AIModelDeterministicRouting:    1.0,
		// The Go fallback route runs no model: it is string matching in this
		// package. Zero is the honest weight, and it is why a fallback run
		// reports a call that cost nothing.
		telemetry.AIModelGoFallbackRouting: 0.0,
		// A model this table does not price. Charged at unit weight, so the series
		// is the SIZE of the request rather than a price, and labelled unpriced
		// so a reader cannot mistake it for one.
		telemetry.AIModelUnpriced: 1.0,
	}

	// pricedModelNames maps a model name a response reported onto the closed
	// enumeration. A provider is free to answer with any string it likes, so the
	// name is never used as a label directly: an unrecognised one becomes
	// AIModelUnpriced, which is visible rather than a guess.
	pricedModelNames = map[string]telemetry.AIModel{
		modelNameDeterministicFoundation: telemetry.AIModelDeterministicFoundation,
		modelNameDeterministicRouting:    telemetry.AIModelDeterministicRouting,
		modelNameGoFallbackRouting:       telemetry.AIModelGoFallbackRouting,
	}
)

// Cost is one attributed figure. Units is the number; everything else is the
// provenance that makes the number mean something.
type Cost struct {
	Operation telemetry.AIOperation
	Model     telemetry.AIModel
	Units     float64
	// Priced is false when the model is one this cost model does not price. The
	// Units are then the size term alone, and a report carrying that figure has
	// to say so rather than calling it a cost.
	Priced bool
	// PayloadRunes is the size the figure was derived from, so a reader can
	// recompute it by hand: Units == (1 + ceil(PayloadRunes/1000)) *
	// operationWeight * modelWeight.
	PayloadRunes int
}

// ModelFor maps a model name a response reported onto the closed enumeration. An
// empty name - which is what a failed call has - is unpriced, because a call whose
// outcome is unknown is not evidence that a known model served it.
func ModelFor(reported string) telemetry.AIModel {
	if model, ok := pricedModelNames[reported]; ok {
		return model
	}
	return telemetry.AIModelUnpriced
}

// AttributedCost prices one call.
//
// input is the request as this package sends it, and it is the only size input:
// the same function prices a call that happened and a call that did not, from the
// same arguments, which is the property the counterfactual depends on.
func AttributedCost(operation telemetry.AIOperation, input any, model telemetry.AIModel) Cost {
	runes := payloadRunes(input)
	size := 1.0 + math.Ceil(float64(runes)/float64(payloadRunesPerUnit))
	weight, ok := operationWeights[operation]
	if !ok {
		// Unreachable while the enumeration is total, which a test asserts. If it
		// ever happens, charging unit weight is the safe direction: a call
		// reported as ordinary rather than a call reported as free.
		weight = 1.0
	}
	modelFactor, ok := modelWeights[model]
	if !ok {
		modelFactor = 1.0
	}
	return Cost{
		Operation:    operation,
		Model:        model,
		Units:        size * weight * modelFactor,
		Priced:       model != telemetry.AIModelUnpriced,
		PayloadRunes: runes,
	}
}

// payloadRunes is the rune length of the JSON encoding of a request. A request
// that will not encode contributes no size term, which charges it the one unit
// every call costs - the same as a call with an empty body, which is the honest
// reading of a request this package could not serialize.
func payloadRunes(input any) int {
	encoded, err := json.Marshal(input)
	if err != nil {
		return 0
	}
	return utf8.RuneCount(encoded)
}

// ---------------------------------------------------------------------------
// Attribution: which route a call belongs to.

// routeContextKey is unexported and of its own type, so nothing outside this
// package can put a route on a context or read one off it.
type routeContextKey struct{}

// WithRoute attributes every model call made under ctx to a routing decision.
//
// This is operational classification and it goes nowhere near a stored status. It
// is a metric label, read once at the moment of the call, written to one counter,
// and never persisted, never read back and never consulted by anything that
// decides a status. A caller that has been told what route a question takes puts
// that route on the context and every downstream call is attributed to it, which
// is the entire reason a cost figure can answer "did routing save anything" at
// all: a total with no route behind it cannot.
func WithRoute(ctx context.Context, route string) context.Context {
	return context.WithValue(ctx, routeContextKey{}, telemetry.AIRouteFor(route))
}

// RouteFromContext reports the route a context attributes its calls to, or
// AIRouteUnrouted when none was set. Absent is a value, not an error: most calls
// in this repository are not made under a routing decision.
func RouteFromContext(ctx context.Context) telemetry.AIRoute {
	if ctx == nil {
		return telemetry.AIRouteUnrouted
	}
	if route, ok := ctx.Value(routeContextKey{}).(telemetry.AIRoute); ok {
		return route
	}
	return telemetry.AIRouteUnrouted
}

// ---------------------------------------------------------------------------
// What each route costs, and what it would have cost without routing.

// routeWork is what each route sends to a model.
//
// It is DECLARED DATA describing the switch internal/research/rag_service.go makes
// on a routing decision, not a second implementation of it and not a replacement
// for it. A deep route spends one research_query call over the retrieved
// passages. A cheap route spends nothing, because its answer is assembled from
// passages that were already retrieved. An ignore route spends nothing, because an
// ignore decision is only reached when there is no source material - so the deep
// path was unreachable for it too and its counterfactual is zero as well, not
// because routing saved something but because there was nothing to save.
//
// It is declared rather than computed so the correspondence is a list a reviewer
// can read in one place, and so that changing the RAG service's switch is a
// review that has to touch this map. It is not on the request path: nothing in
// the request path calls it.
//
// If internal/research/rag_service.go changes which operations a route calls,
// THIS MAP IS WRONG until it is changed in the same commit. docs/cost-measurement.md
// carries that as a maintenance note.
var routeWork = map[string]telemetry.AIOperation{
	RoutingRouteDeep: telemetry.AIOperationResearchQuery,
}

// RouteCost is one routing decision, priced both ways.
type RouteCost struct {
	Route string
	Model telemetry.AIModel
	// Attributed is the work the decision caused, priced.
	Attributed []Cost
	// Counterfactual is the work the all-deep baseline would have caused, priced
	// by the same function from the same inputs.
	Counterfactual []Cost
	// The sums of the two, and their difference. The difference is what routing
	// was worth ON THIS WORKLOAD under THIS cost model. It is not money, and it is
	// not a measured reduction in spend.
	AttributedUnits     float64
	CounterfactualUnits float64
	Difference          float64
}

// RouteWork prices one routing decision and the counterfactual of not routing it.
//
// question and contexts are the synthesis request, exactly as the RAG service
// would build it, so the counterfactual arm is the same payload the deep path
// would really have sent rather than an estimate of it. model is the model that
// would serve the synthesis; the same figure is used for both arms so the
// comparison is never across two models.
func RouteWork(decision RoutingDecision, question string, contexts []SourceContext, model telemetry.AIModel) RouteCost {
	work := RouteCost{Route: decision.Route, Model: model}
	synthesis := ResearchQueryRequest{Query: question, Contexts: contexts}
	if operation, doesWork := routeWork[decision.Route]; doesWork {
		work.Attributed = append(work.Attributed, AttributedCost(operation, synthesis, model))
	}
	// The baseline is every question answered by the deep path. For a question
	// that took the deep path the two arms are the same call, which is what makes
	// the difference zero rather than absent.
	if decision.Route != RoutingRouteIgnore {
		work.Counterfactual = append(work.Counterfactual, AttributedCost(routeWork[RoutingRouteDeep], synthesis, model))
	}
	for _, cost := range work.Attributed {
		work.AttributedUnits += cost.Units
	}
	for _, cost := range work.Counterfactual {
		work.CounterfactualUnits += cost.Units
	}
	work.Difference = work.CounterfactualUnits - work.AttributedUnits
	return work
}

// modelNamer is implemented by every response type this package decodes that
// carries a model name, so the cost model is told the model that actually
// answered rather than the one a caller hoped for.
//
// A response type that forgets it decodes to a call attributed to AIModelUnpriced,
// which is visible in the exposition and therefore findable, rather than an
// unpriced call silently filed under the provider's default.
type modelNamer interface {
	reportedModel() string
}

// reportedModel reports the model the provider said produced a response. The
// method is unexported so a type outside this package cannot satisfy it by
// accident.
func (r EmbeddingResponse) reportedModel() string        { return r.Model }
func (r ClassificationResponse) reportedModel() string   { return r.Model }
func (r EntityExtractionResponse) reportedModel() string { return r.Model }
func (r ClaimExtractionResponse) reportedModel() string  { return r.Model }
func (r EntityResolutionResponse) reportedModel() string { return r.Model }
func (r ContradictionResponse) reportedModel() string    { return r.Model }
func (r RerankResponse) reportedModel() string           { return r.Model }
func (r ResearchQueryResponse) reportedModel() string    { return r.Model }
func (r RoutingDecision) reportedModel() string          { return r.Model }

// responseModel reports the model behind a decoded response, or the empty string
// when there is not one. The empty string prices as unpriced.
func responseModel(output any) string {
	if named, ok := output.(modelNamer); ok {
		return named.reportedModel()
	}
	return ""
}
