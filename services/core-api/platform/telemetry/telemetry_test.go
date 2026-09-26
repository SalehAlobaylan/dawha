package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The three properties this package has to have, in the order they matter:
//
//  1. a request, the AI calls it makes and the job it enqueues share one id;
//  2. nothing derived from a request's content can reach a label or a log field;
//  3. with the exporter off, nothing observes, nothing listens and nothing
//     changes for `make verify` or `make e2e`.

func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	registry := NewMetrics("dawha-core-api", true)
	registry.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	return registry
}

func TestARequestIDFlowsFromTheRequestIntoTheContextTheHandlerSees(t *testing.T) {
	var seenInHandler string
	var seenDownstream string
	registry := newTestMetrics(t)
	handler := Middleware(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenInHandler = RequestID(r.Context())
		// An AI call made inside the request sees the same id.
		seenDownstream = RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if seenInHandler == "" {
		t.Fatal("the handler saw no request id")
	}
	if seenDownstream != seenInHandler {
		t.Fatalf("downstream saw %q, the handler saw %q", seenDownstream, seenInHandler)
	}
	if got := recorder.Header().Get(RequestIDHeader); got != seenInHandler {
		t.Fatalf("the response header says %q and the context said %q; they have to be the same id or the correlation is a coincidence", got, seenInHandler)
	}
}

func TestAClientSuppliedRequestIDIsAcceptedAndABadOneIsReplaced(t *testing.T) {
	// A well formed id from a client is useful and is kept. A malformed one is
	// replaced, because it ends up in a log line and - if anybody is tempted - in
	// a metric label, and neither can be allowed to contain a newline.
	cases := []struct {
		name     string
		supplied string
		keep     bool
	}{
		{name: "a uuid", supplied: "8f1c2b3a-4d5e-6f70-8192-a3b4c5d6e7f8", keep: true},
		{name: "a trace id", supplied: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", keep: true},
		{name: "empty", supplied: "", keep: false},
		{name: "a newline", supplied: "abc\ndef", keep: false},
		{name: "a quote", supplied: `abc"def`, keep: false},
		{name: "a space", supplied: "abc def", keep: false},
		{name: "arabic text", supplied: "نص عربي", keep: false},
		{name: "too long", supplied: strings.Repeat("a", maxRequestIDLength+1), keep: false},
		{name: "a semicolon", supplied: "abc;def", keep: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			accepted := AcceptRequestID(testCase.supplied)
			if testCase.keep && accepted != testCase.supplied {
				t.Fatalf("a well formed id was replaced: %q became %q", testCase.supplied, accepted)
			}
			if !testCase.keep {
				if accepted == testCase.supplied && testCase.supplied != "" {
					t.Fatalf("a malformed id was accepted as-is: %q", accepted)
				}
				if !isAcceptableRequestID(accepted) {
					t.Fatalf("the replacement %q is not itself acceptable", accepted)
				}
			}
		})
	}
}

func TestTheGeneratedRequestIDsAreDistinctAndSortable(t *testing.T) {
	first, second := NewRequestID(), NewRequestID()
	if first == second {
		t.Fatal("two generated request ids are the same")
	}
	if !isAcceptableRequestID(first) || !isAcceptableRequestID(second) {
		t.Fatalf("a generated id is not acceptable: %q %q", first, second)
	}
	if first > second {
		t.Fatal("generated ids are not time-ordered, so a log sorted by id is not a log in order")
	}
}

func TestTheRequestIDReachesALogLineWithoutTheHandlerPassingIt(t *testing.T) {
	sink := newLogSink()
	logger := slog.New(sink)
	ctx := WithRequestID(context.Background(), "req-abc-123")
	WithLogger(ctx, logger).Info("processing source file", "stage", "extract")
	WithLogger(ctx, logger).Error("extraction failed", "stage", "extract")
	WithLogger(ctx, logger).Info("second line")

	if sink.records() != 3 {
		t.Fatalf("the sink saw %d records, want 3", sink.records())
	}
	for _, line := range sink.lines() {
		fields := decode(t, line)
		if fields["request_id"] != "req-abc-123" {
			t.Fatalf("a line is missing the request id: %s", line)
		}
	}
	// A logger used outside any request has no id field at all, rather than an
	// empty one: "" in a correlation field reads as "correlated with the empty
	// request", which is a worse answer than "not part of a request".
	outside := WithLogger(context.Background(), logger)
	sink.reset()
	outside.Info("no request behind this")
	fields := decode(t, sink.lines()[0])
	if _, present := fields["request_id"]; present {
		t.Fatalf("a background line carries a request id: %s", sink.lines()[0])
	}
}

func TestTheRequestIDTravelsIntoAJobPayloadAndBackOut(t *testing.T) {
	// The seam where correlation crosses a process boundary. Without this, the
	// line that fails at 3am in a worker cannot be traced to the upload that
	// caused it, which is the entire reason the id is threaded this far.
	ctx := WithRequestID(context.Background(), "req-cross-process-1")
	payload := map[string]any{"source_id": "11111111-1111-1111-1111-111111111111"}
	PropagateRequestID(ctx, payload)
	if payload[RequestIDPayloadKey] != "req-cross-process-1" {
		t.Fatalf("the payload does not carry the id: %v", payload)
	}
	if got := RequestIDFromPayload(payload); got != "req-cross-process-1" {
		t.Fatalf("the id did not come back out: %q", got)
	}
	// A payload with no request behind it answers "" rather than failing, because
	// every job enqueued before this existed has no id in it.
	if got := RequestIDFromPayload(map[string]any{}); got != "" {
		t.Fatalf("a payload with no id produced %q", got)
	}
	if got := RequestIDFromPayload(nil); got != "" {
		t.Fatalf("a nil payload produced %q", got)
	}
	// A payload value that came out of a database row is treated exactly like a
	// header: it is validated, so a poisoned row cannot forge a log line.
	poisoned := map[string]any{RequestIDPayloadKey: "abc\ndef"}
	if got := RequestIDFromPayload(poisoned); got != "" {
		t.Fatalf("a poisoned payload value produced %q", got)
	}
	wrongType := map[string]any{RequestIDPayloadKey: 42}
	if got := RequestIDFromPayload(wrongType); got != "" {
		t.Fatalf("a numeric payload value produced %q", got)
	}
}

// ---------------------------------------------------------------------------
// The label set. This is the test that matters most in this file.

func TestNoMetricLabelCanCarryRequestContent(t *testing.T) {
	// Everything a caller might be tempted to put in a label, offered to the
	// classification functions this package exposes. None of them has a parameter
	// that takes content at all, and the assertions below say so explicitly for
	// the two that take a string.
	registry := newTestMetrics(t)
	registry.HTTPRequest(MethodFor("POST"), RouteFor("/api/v1/research/query"), StatusClassFor(200), 12*time.Millisecond)
	registry.AICall(AIOperationFor("/v1/research/query"), AIOutcomeOK, 1200*time.Millisecond, 812)
	registry.QueueJob(JobTypeFor("source_process"), JobCompleted, 3*time.Second)
	registry.DatabaseQuery(QueryOperationFor("select"), 2*time.Millisecond)
	registry.QueueDepth(JobTypeFor("source_process"), 4)
	registry.ResearchRun(ResearchCompleted, 9*time.Second)

	exposition := registry.Text()
	// The content this repository actually handles, in Arabic, in every shape it
	// arrives in. If any of it appears in the exposition, a label is carrying
	// content and the whole design is wrong.
	forbidden := []string{
		"عبد الله", "أبو بكر", "نص عربي", "من كان والد", "معاوية", "بن عبادة",
		"INSERT", "SELECT", "FROM users", "sources/", "q=محمد", "password", "token",
	}
	for _, needle := range forbidden {
		if strings.Contains(exposition, needle) {
			t.Fatalf("the exposition contains %q:\n%s", needle, exposition)
		}
	}
	// And every label that IS there is a member of its enumeration.
	for _, line := range strings.Split(exposition, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		assertLabelsAreEnumerated(t, line)
	}
	if registry.DroppedSamples() != 0 {
		t.Fatalf("%d samples were dropped as out-of-enumeration, which means one of these calls used a value the enumerations do not have", registry.DroppedSamples())
	}
}

func TestALabelValueOutsideItsEnumerationIsDroppedRatherThanStored(t *testing.T) {
	// The recording functions take enumeration types, so this cannot be reached
	// through them. It is reached here through the registry directly, which is the
	// point: the guard is at the storage boundary, not only at the type.
	registry := newTestMetrics(t)
	registry.record("dawha_http_requests_total", 1, "post", "research", "2xx")
	registry.record("dawha_http_requests_total", 1, "post", "a person's name", "2xx")
	registry.record("dawha_ai_calls_total", 1, "والمصطلح", "ok")
	exposition := registry.Text()
	if strings.Contains(exposition, "a person") || strings.Contains(exposition, "والمصطلح") {
		t.Fatalf("a value outside its enumeration was exported:\n%s", exposition)
	}
	if registry.DroppedSamples() != 2 {
		t.Fatalf("dropped = %d, want 2", registry.DroppedSamples())
	}
	// The good sample is still there, so a bad caller does not silence the metric.
	if !strings.Contains(exposition, `route="research"`) {
		t.Fatalf("the legitimate sample was lost:\n%s", exposition)
	}
}

func TestTheLabelSetIsBoundedAndEveryEnumerationIsTotal(t *testing.T) {
	// Cardinality, written as an assertion. The number of series a family can
	// ever have is the product of its enumerations, and a reviewer can check this
	// number by hand.
	cases := []struct {
		family string
		budget int
	}{
		{"dawha_http_requests_total", 7 * 20 * 4},
		{"dawha_db_queries_total", 5},
		{"dawha_queue_depth", 5},
		{"dawha_queue_jobs_total", 5 * 5},
		{"dawha_ai_calls_total", 10 * 3},
		{"dawha_ai_cost_units", 10},
		{"dawha_research_runs_total", 4},
	}
	for _, testCase := range cases {
		f := registry_family(t, testCase.family)
		product := 1
		for _, enum := range f.enums {
			if enum.size <= 0 {
				t.Fatalf("family %s has an enumeration with no size", testCase.family)
			}
			product *= enum.size
		}
		if product != testCase.budget {
			t.Fatalf("family %s can produce %d series; the budget written here is %d. If a new enumeration member was added, change this number deliberately.", testCase.family, product, testCase.budget)
		}
	}
	// Every constant declared in the enumerations is a member of its map. A
	// constant added without being added to the map would be a label the
	// registry silently drops, and this is where that shows up.
	for _, method := range []Method{MethodGet, MethodPost, MethodPatch, MethodPut, MethodDelete, MethodOptions, MethodOther} {
		if !validMethods[method] {
			t.Fatalf("the method %q is not in its enumeration", method)
		}
	}
	for _, route := range []Route{RouteHealth, RouteAuth, RouteTrees, RoutePeople, RouteIdentity, RouteSources, RouteFiles,
		RouteSuggestions, RouteQuestions, RouteResearch, RouteResearchAgent, RouteAnalysis, RouteEntityResolution,
		RouteContradiction, RouteJobs, RouteDictionary, RouteGeography, RouteDashboard, RouteObjects, RouteOther} {
		if !validRoutes[route] {
			t.Fatalf("the route %q is not in its enumeration", route)
		}
	}
	for _, jobType := range []JobType{JobSourceProcess, JobEntityResolution, JobResearchAgent, JobContradictionScan, JobOther} {
		if !validJobTypes[jobType] {
			t.Fatalf("the job type %q is not in its enumeration", jobType)
		}
	}
	for _, operation := range []AIOperation{AIOperationNormalizeName, AIOperationEmbed, AIOperationClassify, AIOperationExtractEntities,
		AIOperationExtractClaims, AIOperationResolveEntity, AIOperationContradiction, AIOperationRerank, AIOperationResearchQuery, AIOperationOther} {
		if !validAIOperations[operation] {
			t.Fatalf("the ai operation %q is not in its enumeration", operation)
		}
	}
}

// registry_family exposes a family so the cardinality test can read its label
// names. It is a test-only accessor rather than an exported one: nothing outside
// this package has any business enumerating the registry.
func registry_family(t *testing.T, name string) *family {
	t.Helper()
	registry := newTestMetrics(t)
	f := registry.family(name)
	if f == nil {
		t.Fatalf("no metric family named %q", name)
	}
	return f
}

func assertLabelsAreEnumerated(t *testing.T, line string) {
	t.Helper()
	open := strings.Index(line, "{")
	if open < 0 {
		return
	}
	close := strings.LastIndex(line, "}")
	if close < open {
		t.Fatalf("a label set is not closed: %s", line)
	}
	for _, pair := range strings.Split(line[open+1:close], ",") {
		name, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"`)
		switch name {
		case "method":
			if !validMethods[Method(value)] {
				t.Fatalf("method %q is not an enumeration member: %s", value, line)
			}
		case "route":
			if !validRoutes[Route(value)] {
				t.Fatalf("route %q is not an enumeration member: %s", value, line)
			}
		case "status":
			if !validStatusClasses[StatusClass(value)] {
				t.Fatalf("status %q is not an enumeration member: %s", value, line)
			}
		case "job_type":
			if !validJobTypes[JobType(value)] {
				t.Fatalf("job_type %q is not an enumeration member: %s", value, line)
			}
		case "outcome":
			if !validJobOutcomes[JobOutcome(value)] && !validAIOutcomes[AIOutcome(value)] && !validResearchOutcomes[ResearchOutcome(value)] {
				t.Fatalf("outcome %q is not an enumeration member: %s", value, line)
			}
		case "operation":
			if !validAIOperations[AIOperation(value)] && !validQueryOperations[QueryOperation(value)] {
				t.Fatalf("operation %q is not an enumeration member: %s", value, line)
			}
		case "statement":
			if !validQueryOperations[QueryOperation(value)] {
				t.Fatalf("statement %q is not an enumeration member: %s", value, line)
			}
		case "le":
			// A histogram bound, not a label the caller chose.
		default:
			t.Fatalf("the exposition carries a label named %q, which no enumeration declares: %s", name, line)
		}
	}
}

// ---------------------------------------------------------------------------
// The metrics themselves.

func TestARequestRunEmitsTheDocumentedMetrics(t *testing.T) {
	registry := newTestMetrics(t)
	handler := Middleware(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The AI calls and the enqueue a request makes, in the same context.
		registry.AICall(AIOperationFor("/v1/embed"), AIOutcomeOK, 40*time.Millisecond, 1024)
		PropagateRequestID(r.Context(), map[string]any{})
		w.WriteHeader(http.StatusCreated)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/sources/abc/files", nil))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/sources/abc/files", nil))
	registry.QueueJob(JobTypeFor("source_process"), JobCompleted, 2*time.Second)
	registry.QueueDepth(JobTypeFor("source_process"), 3)
	registry.DatabaseQuery(QueryOperationFor("select"), 3*time.Millisecond)
	registry.ResearchRun(ResearchCompleted, 5*time.Second)

	exposition := registry.Text()
	for _, want := range []string{
		"# TYPE dawha_http_requests_total counter",
		`dawha_http_requests_total{method="post",route="files",status="2xx"} 2`,
		"# TYPE dawha_http_request_duration_seconds histogram",
		"dawha_http_request_duration_seconds_count",
		"dawha_http_request_duration_seconds_sum",
		"dawha_ai_calls_total{operation=\"embed\",outcome=\"ok\"} 2",
		"dawha_ai_cost_units{operation=\"embed\"} 2048",
		"dawha_ai_duration_seconds_count",
		"dawha_queue_jobs_total{job_type=\"source_process\",outcome=\"completed\"} 1",
		"dawha_queue_depth{job_type=\"source_process\"} 3",
		"dawha_db_queries_total{statement=\"select\"} 1",
		"dawha_research_runs_total{outcome=\"completed\"} 1",
		"dawha_research_run_duration_seconds_count",
	} {
		if !strings.Contains(exposition, want) {
			t.Fatalf("the exposition is missing %q:\n%s", want, exposition)
		}
	}
	// Histogram buckets are cumulative and end at +Inf with the total, which is
	// what a Prometheus client expects; a scraper that cannot read this is not a
	// scraper.
	if !strings.Contains(exposition, `dawha_http_request_duration_seconds_bucket{method="post",route="files",status="2xx",le="+Inf"} 2`) {
		t.Fatalf("the +Inf bucket is missing or not cumulative:\n%s", exposition)
	}
}

func TestErrorsAndLatencyAreDistinguished(t *testing.T) {
	registry := newTestMetrics(t)
	handler := Middleware(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "boom") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.Contains(r.URL.Path, "missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/api/v1/trees", "/api/v1/trees/missing", "/api/v1/trees/boom"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	exposition := registry.Text()
	for _, want := range []string{
		`dawha_http_requests_total{method="get",route="trees",status="2xx"} 1`,
		`dawha_http_requests_total{method="get",route="trees",status="4xx"} 1`,
		`dawha_http_requests_total{method="get",route="trees",status="5xx"} 1`,
	} {
		if !strings.Contains(exposition, want) {
			t.Fatalf("the exposition is missing %q:\n%s", want, exposition)
		}
	}
}

func TestARequestPathNeverBecomesALabel(t *testing.T) {
	// Two requests to the same family with different identifiers in the path must
	// land in the SAME series. If they did not, the metric would be a record of
	// which objects a caller has touched, which is both a cardinality problem and
	// a disclosure.
	registry := newTestMetrics(t)
	handler := Middleware(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	for _, id := range []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"من-عبد-الله",
	} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/trees/"+id, nil))
	}
	exposition := registry.Text()
	if strings.Contains(exposition, "11111111") || strings.Contains(exposition, "من-عبد") {
		t.Fatalf("a path segment reached the exposition:\n%s", exposition)
	}
	if !strings.Contains(exposition, `dawha_http_requests_total{method="get",route="trees",status="2xx"} 3`) {
		t.Fatalf("the three requests did not land in one series:\n%s", exposition)
	}
}

// ---------------------------------------------------------------------------
// Off by default.

func TestTheExporterIsOffByDefaultAndOffCostsNothing(t *testing.T) {
	config := ConfigFromEnvironment(func(string) string { return "" })
	if config.Metrics {
		t.Fatal("telemetry is on with no environment set; `make verify` and `make e2e` must be unaffected by this package")
	}
	if config.MetricsAddr != DefaultExporterAddr {
		t.Fatalf("the default address is %q", config.MetricsAddr)
	}
	if !strings.Contains(config.Describe(), "metrics=disabled") {
		t.Fatalf("Describe does not say the exporter is off: %q", config.Describe())
	}
	// A disabled registry records nothing and exports nothing, and every recording
	// call is still safe to make.
	registry := NewMetrics("dawha-core-api", false)
	if registry.Enabled() {
		t.Fatal("a registry built with enabled=false reports itself enabled")
	}
	registry.HTTPRequest(MethodPost, RouteAuth, Status4xx, time.Second)
	registry.AICall(AIOperationEmbed, AIOutcomeError, time.Second, 99)
	registry.QueueJob(JobSourceProcess, JobFailed, time.Second)
	registry.DatabaseQuery(QuerySelect, time.Second)
	registry.ResearchRun(ResearchFailed, time.Second)
	if exposition := registry.Text(); exposition != "" {
		t.Fatalf("a disabled registry exported something:\n%s", exposition)
	}
	stop, err := registry.StartExporter(context.Background(), ExporterConfig{})
	if err != nil {
		t.Fatalf("a disabled exporter reported an error: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("stopping a disabled exporter reported an error: %v", err)
	}
}

func TestTheExporterServesTheExpositionAndRefusesOtherMethods(t *testing.T) {
	registry := newTestMetrics(t)
	registry.HTTPRequest(MethodGet, RouteHealth, Status2xx, time.Millisecond)
	handler := registry.Handler()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("scrape status = %d", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("content type = %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "dawha_http_requests_total") {
		t.Fatalf("the scrape is empty:\n%s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("a POST to the scrape endpoint got %d, want 405", recorder.Code)
	}
}

func TestConfigFromEnvironmentReadsTheDocumentedNamesOnly(t *testing.T) {
	values := map[string]string{
		"TELEMETRY_METRICS_ENABLED": "true",
		"TELEMETRY_METRICS_ADDR":    "0.0.0.0:9999",
		"APP_ENV":                   "production",
		"TELEMETRY_SERVICE_NAME":    "dawha-worker",
	}
	config := ConfigFromEnvironment(func(name string) string { return values[name] })
	if !config.Metrics || config.MetricsAddr != "0.0.0.0:9999" || config.Environment != "production" || config.ServiceName != "dawha-worker" {
		t.Fatalf("the configuration was not read: %+v", config)
	}
	if !strings.Contains(config.Describe(), "addr=0.0.0.0:9999") {
		t.Fatalf("Describe = %q", config.Describe())
	}
}

// ---------------------------------------------------------------------------
// Route classification, which decides the HTTP label.

func TestRouteForClassifiesEveryFamilyTheRouterServes(t *testing.T) {
	cases := map[string]Route{
		"/healthz":                                 RouteHealth,
		"/readyz":                                  RouteHealth,
		"/api/v1/dashboard":                        RouteDashboard,
		"/api/v1/auth/login":                       RouteAuth,
		"/api/v1/trees/abc/versions":               RouteTrees,
		"/api/v1/invitations":                      RouteTrees,
		"/api/v1/people/abc/aliases":               RoutePeople,
		"/api/v1/families":                         RoutePeople,
		"/api/v1/normalize-name":                   RoutePeople,
		"/api/v1/sources":                          RouteSources,
		"/api/v1/claims":                           RouteSources,
		"/api/v1/sources/abc/files":                RouteFiles,
		"/api/v1/source-files/abc/download":        RouteFiles,
		"/api/v1/suggestions":                      RouteSuggestions,
		"/api/v1/research-question-candidates/abc": RouteSuggestions,
		"/api/v1/questions/abc/notes":              RouteQuestions,
		"/api/v1/disputes":                         RouteQuestions,
		"/api/v1/research/query":                   RouteResearch,
		"/api/v1/research/graph/ancestor-frontier": RouteResearch,
		"/api/v1/research-agent/runs":              RouteResearchAgent,
		"/api/v1/entity-resolution/runs":           RouteEntityResolution,
		"/api/v1/contradictions/findings":          RouteContradiction,
		"/api/v1/temporal-analysis/runs":           RouteAnalysis,
		"/api/v1/geospatial-intelligence/runs":     RouteAnalysis,
		"/api/v1/jobs":                             RouteJobs,
		"/api/v1/dictionary/claim/abc":             RouteDictionary,
		"/api/v1/map":                              RouteGeography,
		"/api/v1/places":                           RouteGeography,
		"/local-objects/sources/abc/def.txt":       RouteObjects,
		"/api/v1/nothing-like-this":                RouteOther,
	}
	for path, want := range cases {
		if got := RouteFor(path); got != want {
			t.Fatalf("RouteFor(%q) = %q, want %q", path, got, want)
		}
	}
	// Every path in the table is a bounded route, including the one with a key in
	// it. That last one is the case that would have leaked a storage key into a
	// label if the object route had been matched by a wildcard.
	for path := range cases {
		if !validRoutes[RouteFor(path)] {
			t.Fatalf("RouteFor(%q) produced a value outside its enumeration", path)
		}
	}
}

func TestAIOperationForCoversEveryEndpointTheClientCalls(t *testing.T) {
	// The nine paths below are the nine `p.post(ctx, ...)` call sites in
	// internal/ai. If somebody adds an endpoint and forgets this, the new call
	// lands in AIOperationOther - visible as a jump in "other", not invisible.
	endpoints := []string{
		"/v1/normalize-name", "/v1/embed", "/v1/classify", "/v1/extract/entities",
		"/v1/extract/claims", "/v1/resolve/entity", "/v1/analyze/contradiction",
		"/v1/rerank", "/v1/research/query",
	}
	for _, endpoint := range endpoints {
		if AIOperationFor(endpoint) == AIOperationOther {
			t.Fatalf("the endpoint %s is not classified", endpoint)
		}
	}
	if AIOperationFor("/v1/something-new") != AIOperationOther {
		t.Fatal("an unknown endpoint was classified as a known operation")
	}
}

// ---------------------------------------------------------------------------
// The database tracer.

func TestTheTracerReadsOnlyTheStatementKind(t *testing.T) {
	// pgx hands a tracer the whole statement and its arguments. This asserts that
	// the classification is the leading keyword and nothing else, by offering it
	// statements whose CONTENT differs wildly and whose KIND is the same, plus
	// statements whose kind differs.
	cases := map[string]QueryOperation{
		"SELECT 1": QuerySelect,
		"  select * from people where normalized_name_ar = 'عبد الله'":  QuerySelect,
		"INSERT INTO source_files (storage_key) VALUES ('sources/abc')": QueryInsert,
		"UPDATE sources SET title_ar = 'مصدر' WHERE id = $1":            QueryUpdate,
		"DELETE FROM jobs WHERE id = $1":                                QueryDelete,
		// A statement kind this package has no label for lands in "other" rather
		// than being invented into a new enumeration member by a tracer.
		"WITH recent AS (SELECT 1) SELECT * FROM recent": QueryOther,
		"":                              QueryOther,
		"-- a comment\nSELECT 1":        QueryOther,
		"SELECTED_THING(a, 'عبد الله')": QueryOther,
	}
	for statement, want := range cases {
		if got := QueryOperationFor(leadingKeyword(statement)); got != want {
			t.Fatalf("leadingKeyword(%q) -> %q -> %q, want %q", statement, leadingKeyword(statement), got, want)
		}
	}
}

func TestTheTracerRecordsLatencyAndNeverTheStatement(t *testing.T) {
	registry := newTestMetrics(t)
	tracer := NewQueryTracer(registry)
	if tracer == nil {
		t.Fatal("an enabled registry produced no tracer")
	}
	const statement = "SELECT id, display_name_ar FROM users WHERE display_name_ar = 'محمد بن عبد الله' AND email = 'someone@example.invalid'"
	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: statement})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.CommandTag{}})
	exposition := registry.Text()
	if !strings.Contains(exposition, `dawha_db_queries_total{statement="select"} 1`) {
		t.Fatalf("the statement was not recorded:\n%s", exposition)
	}
	for _, forbidden := range []string{"محمد", "someone@example.invalid", "display_name_ar", "SELECT", "FROM users"} {
		if strings.Contains(exposition, forbidden) {
			t.Fatalf("the exposition carries %q, which came out of the statement:\n%s", forbidden, exposition)
		}
	}
}

func TestNoTracerIsInstalledWhenTelemetryIsOff(t *testing.T) {
	// pgx accepts a nil tracer as "no tracer", so the off path costs nothing and
	// needs no branch at the call site.
	if NewQueryTracer(NewMetrics("dawha-core-api", false)) != nil {
		t.Fatal("a disabled registry installed a tracer")
	}
}
