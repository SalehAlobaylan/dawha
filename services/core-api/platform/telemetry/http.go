package telemetry

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// The recording API. Every function here takes enumeration values, never strings
// and never anything derived from a request's content, so there is no call shape
// through which a query, a name or a document could reach a label.

// HTTPRequest records one handled request. It is called by the middleware, once
// per request, after the response is written, so the status is the real one rather
// than the one the handler intended.
func (m *Metrics) HTTPRequest(method Method, route Route, status StatusClass, duration time.Duration) {
	if !m.Enabled() {
		return
	}
	labels := []string{string(method), string(route), string(status)}
	m.record("dawha_http_requests_total", 1, labels...)
	m.observe("dawha_http_request_duration_seconds", duration.Seconds(), DefaultBuckets, labels...)
}

// DatabaseQuery records one statement. The operation is the statement KIND and
// never the statement text or the table: this metric answers "is the database
// slow", and a table name would make it answer "which of four hundred tables is
// slow", at a cardinality that a query built from a variable would make unbounded.
func (m *Metrics) DatabaseQuery(operation QueryOperation, duration time.Duration) {
	if !m.Enabled() {
		return
	}
	m.record("dawha_db_queries_total", 1, string(operation))
	m.observe("dawha_db_query_duration_seconds", duration.Seconds(), DefaultBuckets, string(operation))
}

// QueueDepth records how many jobs of a type are waiting. It is a gauge, not a
// counter: the useful question is "how deep is it now", and a counter of
// enqueues cannot answer it.
func (m *Metrics) QueueDepth(jobType JobType, depth int) {
	if !m.Enabled() {
		return
	}
	m.set("dawha_queue_depth", float64(depth), string(jobType))
}

// QueueJob records a terminal queue outcome and how long the job took.
func (m *Metrics) QueueJob(jobType JobType, outcome JobOutcome, duration time.Duration) {
	if !m.Enabled() {
		return
	}
	m.record("dawha_queue_jobs_total", 1, string(jobType), string(outcome))
	m.observe("dawha_queue_job_duration_seconds", duration.Seconds(), DefaultBuckets, string(jobType))
}

// AICall records one call to the AI service.
//
// costUnits is whatever the provider bills in - tokens, characters, requests -
// and it is a NUMBER on purpose. A cost is the one thing here that has to be
// comparable across calls, and it cannot be compared if it is a string that
// sometimes contains a model name.
func (m *Metrics) AICall(operation AIOperation, outcome AIOutcome, duration time.Duration, costUnits float64) {
	if !m.Enabled() {
		return
	}
	m.record("dawha_ai_calls_total", 1, string(operation), string(outcome))
	m.observe("dawha_ai_duration_seconds", duration.Seconds(), DefaultBuckets, string(operation))
	if costUnits > 0 {
		m.record("dawha_ai_cost_units", costUnits, string(operation))
	}
}

// ResearchRun records a research run's terminal outcome.
func (m *Metrics) ResearchRun(outcome ResearchOutcome, duration time.Duration) {
	if !m.Enabled() {
		return
	}
	m.record("dawha_research_runs_total", 1, string(outcome))
	m.observe("dawha_research_run_duration_seconds", duration.Seconds(), DefaultBuckets)
}

// ---------------------------------------------------------------------------
// The HTTP middleware.

// ResponseRecorder is the smallest wrapper that can report a status and a byte
// count without changing anything else about the response. It is here rather
// than in httpapi because the metric and the wrapper are the same concern.
type ResponseRecorder struct {
	http.ResponseWriter
	status  int
	written int64
	wrote   bool
}

// WriteHeader records the status the first time it is set. A second call is
// ignored, which is what net/http does anyway - and a second call that overwrote
// the recorded status would make the metric disagree with the wire.
func (r *ResponseRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.status = status
	r.wrote = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *ResponseRecorder) Write(payload []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	written, err := r.ResponseWriter.Write(payload)
	r.written += int64(written)
	return written, err
}

// Status reports the status written, defaulting to 200 for a handler that wrote
// a body without calling WriteHeader.
func (r *ResponseRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// Unwrap lets http.ResponseController reach the underlying writer, so a handler
// that needs a hijack or a flush still can.
func (r *ResponseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Middleware assigns a request id, publishes it on the response, puts it in the
// context for everything downstream, and records the request.
//
// The id is assigned HERE, above everything, so that a request rejected by CORS
// or by a rate limit still has one - which is the case somebody debugging a
// throttled request needs most.
func Middleware(metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := AcceptRequestID(r.Header.Get(RequestIDHeader))
		w.Header().Set(RequestIDHeader, id)
		ctx := WithRequestID(r.Context(), id)
		recorder := &ResponseRecorder{ResponseWriter: w}
		started := time.Now()
		next.ServeHTTP(recorder, r.WithContext(ctx))
		if metrics.Enabled() {
			metrics.HTTPRequest(
				MethodFor(r.Method),
				RouteFor(r.URL.Path),
				StatusClassFor(recorder.Status()),
				time.Since(started),
			)
		}
	})
}

// ---------------------------------------------------------------------------
// The exporter.

// ExporterConfig is where the endpoint goes and whether it is on.
//
// Off is the default and the reason is specific: `make verify` and `make e2e` must
// not be changed by this file. A second listener on a port somebody else already
// uses is a startup failure, and a metrics endpoint that is on by default is a
// port somebody else already uses.
type ExporterConfig struct {
	Enabled bool
	// Addr defaults to 127.0.0.1:9464 - loopback, and the port OpenTelemetry's
	// own SDKs use for a Prometheus endpoint. Loopback because a metrics
	// endpoint is an inventory of this service's traffic and belongs inside the
	// deployment's network, not on a public interface.
	Addr string
}

// DefaultExporterAddr is the address the exporter uses when none is given.
const DefaultExporterAddr = "127.0.0.1:9464"

// ExporterConfigFromEnvironment reads TELEMETRY_METRICS_ENABLED and
// TELEMETRY_METRICS_ADDR. Unset means off.
func ExporterConfigFromEnvironment(getenv func(string) string) ExporterConfig {
	if getenv == nil {
		return ExporterConfig{}
	}
	config := ExporterConfig{Addr: getenv("TELEMETRY_METRICS_ADDR")}
	switch value := getenv("TELEMETRY_METRICS_ENABLED"); value {
	case "1", "true", "yes", "on":
		config.Enabled = true
	}
	if config.Addr == "" {
		config.Addr = DefaultExporterAddr
	}
	return config
}

// Handler serves the exposition. It is an http.Handler so the exporter can be
// mounted on the main server or on its own listener, and so a test can scrape it
// without starting anything.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(m.Text()))
	})
}

// StartExporter serves the exposition on its own listener, and returns a function
// that stops it. It is called only when the configuration says the exporter is
// enabled, so the common case constructs nothing at all.
func (m *Metrics) StartExporter(ctx context.Context, config ExporterConfig) (func() error, error) {
	if !config.Enabled {
		return func() error { return nil }, nil
	}
	addr := config.Addr
	if addr == "" {
		addr = DefaultExporterAddr
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           m.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	failures := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			failures <- err
		}
		close(failures)
	}()
	stop := func() error {
		return server.Close()
	}
	select {
	case err := <-failures:
		if err != nil {
			return stop, err
		}
	case <-ctx.Done():
	}
	return stop, nil
}

// FormatCount renders a count for a log line, without importing strconv at every
// call site.
func FormatCount(value int) string { return strconv.Itoa(value) }
