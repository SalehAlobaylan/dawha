package ai

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
)

// `routeWork` is DECLARED DATA about the switch internal/research/rag_service.go
// makes on a routing decision. It has to be declared rather than computed,
// because the two cannot share a source - and the reason it is declared rather
// than computed is not an accident, so it is stated here and asserted by the
// test below rather than left to be rediscovered.
//
// WHY THERE IS NO SHARED SOURCE.
//
// internal/research IMPORTS internal/ai (the RAG service calls
// ai.RoutingRouteDeep in its switch), so internal/ai cannot import
// internal/research: the dependency would be a cycle and the package would not
// build. The other direction - rag_service.go asking this package which
// operation a route costs - would compile, and it is the one this plan
// considered first. It was not taken, because the two switches do not answer the
// same question.
//
// routeWork answers "which model operation does this route cost?". The RAG
// service's switch answers "what does this route do?", and two of its three
// answers are not model calls at all: the ignore route and the cheap route each
// return one of two fixed Arabic sentences. Collapsing that into a shared
// function would mean putting the product's answer strings in the cost model, or
// putting the cost model's vocabulary in the RAG service, and either one makes
// the next reader believe the two are the same decision. They are not. One is a
// price table; the other is what the product says.
//
// WHAT THE TEST DOES INSTEAD.
//
// It reads the switch out of the RAG service's source and derives, per route,
// which model operations that route actually calls - so the map is checked
// against the code rather than against a comment about the code. It fails if
// either side moves: a new model call in another route, a route that stops
// synthesising, an operation the map names that the switch never calls, or a
// route nobody prices.
//
// The derivation reads the file rather than calling execute(), because execute
// needs a pool, a provider and a question, and what has to be checked here is a
// correspondence between two declarations - the kind of fact a caller cannot
// observe without also inventing the answer.

// ragSwitchCase is one case of the switch on a routing decision.
type ragSwitchCase struct {
	route string
	// operations are the model operations this case calls, in the order the case
	// calls them. A case that calls none is a case that spends nothing, which is
	// the fact the map has to agree with.
	operations []string
}

// aiClientCall matches a call through the service's AI client. It is
// deliberately narrow: `s.AI.<Operation>(` is the shape of a model call in that
// file, and matching a bare identifier would sweep in the routing call itself.
var aiClientCall = regexp.MustCompile(`\bs\.AI\.([A-Z][A-Za-z0-9]*)\(`)

// deriveRagRouteOperations reads the route switch out of the RAG service and
// returns what each route calls.
//
// The switch is located by its own statement, `switch routing.Route {`, and
// closed by counting braces, so a case that gained an `if` or a loop does not
// truncate the reading. A file that no longer has that switch fails the test
// rather than producing an empty map that agrees with anything.
func deriveRagRouteOperations(t *testing.T) []ragSwitchCase {
	t.Helper()
	path := filepath.Join("..", "research", "rag_service.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the RAG service: %v", err)
	}
	text := string(source)
	const marker = "switch routing.Route {"
	start := strings.Index(text, marker)
	if start < 0 {
		t.Fatalf("%s no longer switches on a routing decision. The cost model prices what that switch calls, so either the switch moved and routeWork has to follow it, or the cost model is pricing nothing.", path)
	}
	depth := 0
	end := -1
	for index := start + len(marker) - 1; index < len(text); index++ {
		switch text[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = index
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatalf("%s has an unterminated switch on the routing decision", path)
	}
	body := text[start+len(marker) : end]

	cases := make([]ragSwitchCase, 0, 3)
	for _, block := range splitSwitchCases(body) {
		route := routeConstantIn(block.header)
		if route == "" {
			// The default arm. It is a route too - the one every route this cost
			// model does not name falls into - and it is read as the cheap route's
			// because that is the route the switch reaches it from.
			route = RoutingRouteCheap
		}
		cases = append(cases, ragSwitchCase{route: route, operations: aiClientCalls(block.body)})
	}
	if len(cases) == 0 {
		t.Fatalf("%s switches on a routing decision with no case in it; routeWork cannot be checked against an empty switch", path)
	}
	return cases
}

// aiClientCalls returns the method names the body calls on the AI client.
func aiClientCalls(body string) []string {
	matches := aiClientCall.FindAllStringSubmatch(body, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	return names
}

type switchBlock struct {
	header string
	body   string
}

// splitSwitchCases breaks a switch body at its `case` arms. Arms are found at
// the start of a line, which is gofmt's shape for them, so a `case` word inside
// a string literal in a case body cannot be mistaken for a new arm.
func splitSwitchCases(body string) []switchBlock {
	lines := strings.Split(body, "\n")
	blocks := make([]switchBlock, 0, 3)
	current := switchBlock{}
	inCase := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "case ") || trimmed == "default:" {
			if inCase {
				blocks = append(blocks, current)
			}
			current = switchBlock{header: trimmed}
			inCase = true
			continue
		}
		if inCase {
			current.body += line + "\n"
		}
	}
	if inCase {
		blocks = append(blocks, current)
	}
	return blocks
}

// routeValuesByConstant maps the Go constant the RAG service names a route by
// onto the route's wire value, because the switch names the constant and
// routeWork is keyed by the value.
var routeValuesByConstant = map[string]string{
	"RoutingRouteIgnore": RoutingRouteIgnore,
	"RoutingRouteCheap":  RoutingRouteCheap,
	"RoutingRouteDeep":   RoutingRouteDeep,
}

// routeConstantIn reads the ai.RoutingRoute value out of a case header.
func routeConstantIn(header string) string {
	prefix := "case ai."
	rest, found := strings.CutPrefix(header, prefix)
	if !found {
		return ""
	}
	end := strings.Index(rest, ":")
	if end < 0 {
		return ""
	}
	constant := strings.TrimSpace(rest[:end])
	value, known := routeValuesByConstant[constant]
	if !known {
		return ""
	}
	return value
}

// aiMethodNames maps every operation this cost model prices onto the method the
// RAG service would call on its AI client. It is declared rather than derived
// from the operation's own string, because the two do not always agree:
// the contradiction operation is the one exception, and its method is
// AnalyzeContradiction.
//
// The table is checked for totality against operationWeights below, so an
// operation added to the cost model has to be given a name here rather than
// quietly becoming uncheckable.
var aiMethodNames = map[telemetry.AIOperation]string{
	telemetry.AIOperationNormalizeName:   "NormalizeName",
	telemetry.AIOperationEmbed:           "Embed",
	telemetry.AIOperationClassify:        "Classify",
	telemetry.AIOperationExtractEntities: "ExtractEntities",
	telemetry.AIOperationExtractClaims:   "ExtractClaims",
	telemetry.AIOperationResolveEntity:   "ResolveEntity",
	telemetry.AIOperationContradiction:   "AnalyzeContradiction",
	telemetry.AIOperationRerank:          "Rerank",
	telemetry.AIOperationResearchQuery:   "ResearchQuery",
	telemetry.AIOperationRoute:           "Route",
	telemetry.AIOperationOther:           "",
}

// TestTheCostModelPricesTheRoutesTheRagServiceActuallyRuns is the pin.
//
// The two declarations are checked in both directions. Every model call the RAG
// service's switch makes on a route must be priced for that route, and every
// operation routeWork prices for a route must be one that switch actually
// calls. A one-directional check would pass the moment somebody added a second
// model call to the deep route: the map would still be a subset of the truth and
// the cost figure would be an undercount with nothing to notice.
func TestTheCostModelPricesTheRoutesTheRagServiceActuallyRuns(t *testing.T) {
	// The name table has to cover the cost model, or a new operation would be
	// invisible to this test rather than checked by it.
	for operation := range operationWeights {
		if _, named := aiMethodNames[operation]; !named {
			t.Fatalf("operation %q is priced but has no entry in aiMethodNames, so this test cannot check whether a route calls it. Add the method the AI client exposes for it.", operation)
		}
	}

	derived := deriveRagRouteOperations(t)
	// What the switch really does, grouped by route. The default arm is folded
	// into the cheap route because that is the route that reaches it, and a
	// separate entry for "the route nobody named" would let the map agree with
	// the switch by pricing a route the product does not have.
	called := map[string]map[string]bool{}
	for _, current := range derived {
		if _, seen := called[current.route]; !seen {
			called[current.route] = map[string]bool{}
		}
		for _, method := range current.operations {
			called[current.route][method] = true
		}
	}

	for route, methods := range called {
		operation, priced := routeWork[route]
		if len(methods) == 0 {
			// A route that calls no model must be priced as nothing. The ignore
			// route's counterfactual is zero for this reason and not because
			// routing saved anything.
			if priced {
				t.Fatalf("routeWork prices the %q route as %v, but the RAG service's switch for that route calls no model at all", route, operation)
			}
			continue
		}
		if !priced {
			t.Fatalf("the RAG service calls %v on the %q route and this cost model prices it as nothing. Both the attribution and the counterfactual come from routeWork, so every figure the report prints is an undercount for that route", sorted(methods), route)
		}
		method := aiMethodNames[operation]
		if !methods[method] {
			t.Fatalf("routeWork prices the %q route as %v, which the AI client exposes as %s, but the RAG service's switch for that route calls %v. Either the switch changed and this map did not, or the reverse; both make every figure in docs/cost-measurement.md wrong", route, operation, method, sorted(methods))
		}
		// And nothing beyond the priced operation. A route that called two
		// endpoints would be priced as one, and the subset check above would
		// still pass.
		if len(methods) != 1 || method == "" {
			t.Fatalf("the %q route calls %v in the RAG service but routeWork prices one operation for it. The cost model has to price every call the route makes, or its total is an undercount with no signal", route, sorted(methods))
		}
	}
	for route, operation := range routeWork {
		if _, seen := called[route]; !seen {
			t.Fatalf("routeWork prices the %q route as %v, but the RAG service's switch has no arm for that route, so one of the two is stale", route, operation)
		}
	}

	// The shape is not an accident and a reader should not have to derive it: one
	// route synthesises and the other two do not. The counterfactual arm prices
	// routeWork[deep] as what an all-deep baseline would have spent, so a second
	// entry makes that baseline depend on which route a question happened to take.
	synthesising := make([]string, 0, len(routeWork))
	for route := range routeWork {
		synthesising = append(synthesising, route)
	}
	sort.Strings(synthesising)
	if len(synthesising) != 1 || synthesising[0] != RoutingRouteDeep {
		t.Fatalf("routeWork prices %v as doing model work, want the deep route alone", synthesising)
	}
}

func sorted(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
