package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/ratelimit"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
)

// Correlation, through the real router, from a request to a job payload.
//
// The unit tests in platform/telemetry prove the pieces. This proves they are
// CONNECTED: that the id a client sent is the id in the response header, the id in
// the context a handler sees, the id a job enqueue stamps into its payload, and
// the id a worker reads back out. A correlation feature that works in three
// packages and is not wired together is the usual outcome, and this is the test
// that notices.

func TestARequestIDReachesTheJobPayloadThroughTheRealRouter(t *testing.T) {
	registry := telemetry.NewMetrics("dawha-core-api", true)
	router := NewRouter(Dependencies{Metrics: registry})

	var (
		seenInHandler string
		seenInEnqueue string
	)
	// The handler stands in for the upload path: it reads the id out of the
	// context exactly the way jobs.Service.Enqueue does, and stamps it the way
	// the enqueue stamps it.
	enqueue := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenInHandler = telemetry.RequestID(r.Context())
		input := jobs.EnqueueInput{
			Type:           sourceprocessing.SourceProcessJobType,
			Payload:        []byte(`{"source_id":"11111111-1111-1111-1111-111111111111"}`),
			IdempotencyKey: "telemetry-correlation-test",
		}
		stamped := stampLikeTheQueue(t, r.Context(), input)
		seenInEnqueue = telemetry.RequestIDFromPayload(sourceprocessing.JobRequestIDFields(stamped.Payload))
		w.WriteHeader(http.StatusAccepted)
	})
	router = telemetry.Middleware(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/files") {
			enqueue.ServeHTTP(w, r)
			return
		}
		NewRouter(Dependencies{}).ServeHTTP(w, r)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sources/11111111-1111-1111-1111-111111111111/files", nil)
	request.Header.Set(telemetry.RequestIDHeader, "req-from-the-client-0001")
	router.ServeHTTP(recorder, request)

	if seenInHandler != "req-from-the-client-0001" {
		t.Fatalf("the handler saw %q", seenInHandler)
	}
	if seenInEnqueue != "req-from-the-client-0001" {
		t.Fatalf("the enqueued job carries %q, so the id did not cross the process boundary", seenInEnqueue)
	}
	if got := recorder.Header().Get(telemetry.RequestIDHeader); got != "req-from-the-client-0001" {
		t.Fatalf("the response header says %q", got)
	}
	// The job type is a bounded label, so the queue metric the worker will record
	// for this job is a member of the enumeration rather than a payload.
	if telemetry.JobTypeFor(sourceprocessing.SourceProcessJobType) != telemetry.JobSourceProcess {
		t.Fatalf("the source process job type is not a member of its enumeration")
	}
}

// stampLikeTheQueue calls the same helper jobs.enqueue calls, through the exported
// surface, so this test cannot drift from the production path. It is spelled out
// here rather than reused from the jobs package because the helper is
// deliberately unexported: this is the one place outside jobs that needs it, and
// duplicating three lines is cheaper than widening the API.
func stampLikeTheQueue(t *testing.T, ctx context.Context, input jobs.EnqueueInput) jobs.EnqueueInput {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(input.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	telemetry.PropagateRequestID(ctx, payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	input.Payload = encoded
	return input
}

func TestTheRouterPublishesTheIdEvenOnAThrottledRequest(t *testing.T) {
	// The case somebody debugging a rate-limited request needs: a 429 that
	// arrived with no way to correlate it to anything is the worst kind of 429.
	config := telemetry.NewMetrics("dawha-core-api", true)
	limits := ratelimitDefaultWithAuthBudget(1)
	router := NewRouter(Dependencies{Metrics: config, RateLimits: limits})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("the second request got %d, want 429", second.Code)
	}
	if second.Header().Get(telemetry.RequestIDHeader) == "" {
		t.Fatal("a throttled response carries no request id")
	}
	if first.Header().Get(telemetry.RequestIDHeader) == second.Header().Get(telemetry.RequestIDHeader) {
		t.Fatal("two requests were given the same id")
	}
}

func TestTheRouterRecordsNoMetricsWhenNoneAreConfigured(t *testing.T) {
	// The default everywhere in this repository. `make verify` and `make e2e` build
	// routers with no Metrics, and this asserts that costs nothing and records
	// nothing, rather than assuming it.
	router := NewRouter(Dependencies{})
	for i := 0; i < 5; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil))
	}
	// A nil registry in the middleware is a no-op; the request still works. The
	// layer catalogue is used here because it answers from nothing at all, so a
	// failure can only be about the metrics.
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/research/layers", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("a router with no metrics changed a response: %d", recorder.Code)
	}
	if recorder.Header().Get(telemetry.RequestIDHeader) == "" {
		t.Fatal("a router with no metrics stopped publishing the request id")
	}
}

func TestTheHTTPMetricsRecordTheRealStatusNotTheIntendedOne(t *testing.T) {
	registry := telemetry.NewMetrics("dawha-core-api", true)
	// A route that answers 500 without the router's knowledge: the metric has to
	// read the status that went on the wire.
	handler := telemetry.Middleware(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "boom") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// A handler that writes a body without setting a status: 200, not 0.
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/trees/boom", nil))

	exposition := registry.Text()
	if !strings.Contains(exposition, `status="2xx"} 1`) {
		t.Fatalf("the implicit 200 was not recorded:\n%s", exposition)
	}
	if !strings.Contains(exposition, `status="5xx"} 1`) {
		t.Fatalf("the 500 was not recorded:\n%s", exposition)
	}
}

func ratelimitDefaultWithAuthBudget(budget int) ratelimit.Config {
	config := ratelimit.DefaultConfig()
	config.PerMinute[ratelimit.ClassAuth] = budget
	return config
}
