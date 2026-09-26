package telemetry

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The label vocabulary.
//
// Every label value in this file is a member of a fixed enumeration. That is not a
// style preference, it is the mechanism that makes "no query text, no source
// text, no person names" a property rather than a promise:
//
//   - a metric cannot carry a label this package does not define, because the
//     recording functions take the enumeration types and not strings;
//   - every enumeration is a compile-time constant, so the set of possible label
//     values is finite and known, which bounds cardinality;
//   - the values are operation names, outcomes and status classes - never
//     anything derived from a request's content.
//
// If a future change needs a label this file does not have, the change has to
// add an enumeration member and argue for its cardinality in a review. That is
// the intended friction.

type Method string
type Route string
type StatusClass string
type JobType string
type JobOutcome string
type AIOperation string
type AIOutcome string
type ResearchOutcome string
type QueryOperation string

// Method values. Anything else becomes MethodOther rather than being rejected,
// because a new HTTP verb should not fail a request.
const (
	MethodGet     Method = "get"
	MethodPost    Method = "post"
	MethodPatch   Method = "patch"
	MethodPut     Method = "put"
	MethodDelete  Method = "delete"
	MethodOptions Method = "options"
	MethodOther   Method = "other"
)

// Route values. One per route FAMILY, never per path: a path carries identifiers,
// and an identifier as a label value is unbounded cardinality and a record of
// which objects a caller has touched.
const (
	RouteHealth           Route = "health"
	RouteAuth             Route = "auth"
	RouteTrees            Route = "trees"
	RoutePeople           Route = "people"
	RouteIdentity         Route = "identity"
	RouteSources          Route = "sources"
	RouteFiles            Route = "files"
	RouteSuggestions      Route = "suggestions"
	RouteQuestions        Route = "questions"
	RouteResearch         Route = "research"
	RouteResearchAgent    Route = "research_agent"
	RouteAnalysis         Route = "analysis"
	RouteEntityResolution Route = "entity_resolution"
	RouteContradiction    Route = "contradiction"
	RouteJobs             Route = "jobs"
	RouteDictionary       Route = "dictionary"
	RouteGeography        Route = "geography"
	RouteDashboard        Route = "dashboard"
	RouteObjects          Route = "objects"
	RouteOther            Route = "other"
)

// StatusClass values. The class and not the code: there are a hundred status
// codes and five answers to "was this a success, a client problem or ours".
const (
	Status2xx StatusClass = "2xx"
	Status3xx StatusClass = "3xx"
	Status4xx StatusClass = "4xx"
	Status5xx StatusClass = "5xx"
)

// JobType values, one per job type the queue accepts. An unknown type is
// JobTypeOther: a new job type shows up as a jump in "other" rather than as an
// unbounded label, and a test asserts the real types are all present.
const (
	JobSourceProcess     JobType = "source_process"
	JobEntityResolution  JobType = "entity_resolution"
	JobResearchAgent     JobType = "research_agent"
	JobContradictionScan JobType = "contradiction_scan"
	JobOther             JobType = "other"
)

// JobOutcome values.
const (
	JobCompleted JobOutcome = "completed"
	JobFailed    JobOutcome = "failed"
	JobRetried   JobOutcome = "retried"
	JobRecovered JobOutcome = "recovered"
	JobAbandoned JobOutcome = "abandoned"
)

// AIOperation values, one per endpoint of the ai-research service. The
// enumeration IS the list of endpoints, so a new endpoint that is not instrumented
// is a visible omission rather than an invisible one.
const (
	AIOperationNormalizeName   AIOperation = "normalize_name"
	AIOperationEmbed           AIOperation = "embed"
	AIOperationClassify        AIOperation = "classify"
	AIOperationExtractEntities AIOperation = "extract_entities"
	AIOperationExtractClaims   AIOperation = "extract_claims"
	AIOperationResolveEntity   AIOperation = "resolve_entity"
	AIOperationContradiction   AIOperation = "contradiction"
	AIOperationRerank          AIOperation = "rerank"
	AIOperationResearchQuery   AIOperation = "research_query"
	AIOperationOther           AIOperation = "other"
)

// AIOutcome values.
const (
	AIOutcomeOK      AIOutcome = "ok"
	AIOutcomeError   AIOutcome = "error"
	AIOutcomeTimeout AIOutcome = "timeout"
)

// ResearchOutcome values.
const (
	ResearchCompleted ResearchOutcome = "completed"
	ResearchFailed    ResearchOutcome = "failed"
	ResearchTruncated ResearchOutcome = "truncated"
	ResearchCancelled ResearchOutcome = "cancelled"
)

// QueryOperation values. The kind of statement and never the table: a table name
// is bounded today and unbounded the day a query is built from a variable, and
// this metric is about whether the DATABASE is slow, not which of four hundred
// tables it was slow on.
const (
	QuerySelect QueryOperation = "select"
	QueryInsert QueryOperation = "insert"
	QueryUpdate QueryOperation = "update"
	QueryDelete QueryOperation = "delete"
	QueryOther  QueryOperation = "other"
)

// The maps below are the authority on which values are legal. The exposition path
// re-checks against them, so a value that somehow got in is dropped from the
// output rather than printed.

var (
	validMethods = map[Method]bool{MethodGet: true, MethodPost: true, MethodPatch: true, MethodPut: true, MethodDelete: true, MethodOptions: true, MethodOther: true}
	validRoutes  = map[Route]bool{
		RouteHealth: true, RouteAuth: true, RouteTrees: true, RoutePeople: true, RouteIdentity: true,
		RouteSources: true, RouteFiles: true, RouteSuggestions: true, RouteQuestions: true, RouteResearch: true,
		RouteResearchAgent: true, RouteAnalysis: true, RouteEntityResolution: true, RouteContradiction: true,
		RouteJobs: true, RouteDictionary: true, RouteGeography: true, RouteDashboard: true,
		RouteObjects: true, RouteOther: true,
	}
	validStatusClasses = map[StatusClass]bool{Status2xx: true, Status3xx: true, Status4xx: true, Status5xx: true}
	validJobTypes      = map[JobType]bool{JobSourceProcess: true, JobEntityResolution: true, JobResearchAgent: true, JobContradictionScan: true, JobOther: true}
	validJobOutcomes   = map[JobOutcome]bool{JobCompleted: true, JobFailed: true, JobRetried: true, JobRecovered: true, JobAbandoned: true}
	validAIOperations  = map[AIOperation]bool{
		AIOperationNormalizeName: true, AIOperationEmbed: true, AIOperationClassify: true,
		AIOperationExtractEntities: true, AIOperationExtractClaims: true, AIOperationResolveEntity: true,
		AIOperationContradiction: true, AIOperationRerank: true, AIOperationResearchQuery: true,
		AIOperationOther: true,
	}
	validAIOutcomes       = map[AIOutcome]bool{AIOutcomeOK: true, AIOutcomeError: true, AIOutcomeTimeout: true}
	validResearchOutcomes = map[ResearchOutcome]bool{ResearchCompleted: true, ResearchFailed: true, ResearchTruncated: true, ResearchCancelled: true}
	validQueryOperations  = map[QueryOperation]bool{QuerySelect: true, QueryInsert: true, QueryUpdate: true, QueryDelete: true, QueryOther: true}
)

// MethodFor normalizes an HTTP method onto the enumeration.
func MethodFor(method string) Method {
	switch Method(strings.ToLower(strings.TrimSpace(method))) {
	case MethodGet:
		return MethodGet
	case MethodPost:
		return MethodPost
	case MethodPatch:
		return MethodPatch
	case MethodPut:
		return MethodPut
	case MethodDelete:
		return MethodDelete
	case MethodOptions:
		return MethodOptions
	default:
		return MethodOther
	}
}

// StatusClassFor buckets a status code.
func StatusClassFor(status int) StatusClass {
	switch {
	case status >= 200 && status < 300:
		return Status2xx
	case status >= 300 && status < 400:
		return Status3xx
	case status >= 400 && status < 500:
		return Status4xx
	default:
		return Status5xx
	}
}

// JobTypeFor normalizes a queue job type onto the enumeration.
func JobTypeFor(jobType string) JobType {
	candidate := JobType(strings.TrimSpace(jobType))
	if validJobTypes[candidate] {
		return candidate
	}
	return JobOther
}

// RouteFor classifies a request path onto the enumeration.
//
// The object route is matched first and exactly, because it is the one path whose
// suffix is attacker-influenced: a wildcard that swallowed `/local-objects/<key>`
// into RouteFiles would be fine, and one that put the KEY in a label would not.
func RouteFor(path string) Route {
	switch {
	case path == "/healthz" || path == "/readyz":
		return RouteHealth
	case strings.HasPrefix(path, RequestIDHeaderPath):
		return RouteObjects
	case strings.HasPrefix(path, "/api/v1/auth"):
		return RouteAuth
	case strings.HasPrefix(path, "/api/v1/dashboard"):
		return RouteDashboard
	case strings.HasPrefix(path, "/api/v1/dictionary"):
		return RouteDictionary
	case strings.HasPrefix(path, "/api/v1/map") || strings.HasPrefix(path, "/api/v1/places"):
		return RouteGeography
	case strings.HasPrefix(path, "/api/v1/suggestions") || strings.HasPrefix(path, "/api/v1/research-question-candidates"):
		return RouteSuggestions
	case strings.HasPrefix(path, "/api/v1/questions") || strings.HasPrefix(path, "/api/v1/disputes"):
		return RouteQuestions
	case strings.HasPrefix(path, "/api/v1/research-agent"):
		return RouteResearchAgent
	case strings.HasPrefix(path, "/api/v1/research"):
		return RouteResearch
	case strings.HasPrefix(path, "/api/v1/entity-resolution"):
		return RouteEntityResolution
	case strings.HasPrefix(path, "/api/v1/contradictions"):
		return RouteContradiction
	case strings.HasPrefix(path, "/api/v1/temporal-analysis") || strings.HasPrefix(path, "/api/v1/geospatial-intelligence"):
		return RouteAnalysis
	case strings.HasPrefix(path, "/api/v1/jobs"):
		return RouteJobs
	case strings.HasPrefix(path, "/api/v1/source-files") || strings.HasSuffix(path, "/files"):
		return RouteFiles
	case strings.HasPrefix(path, "/api/v1/sources") || strings.HasPrefix(path, "/api/v1/source-") || strings.HasPrefix(path, "/api/v1/claims"):
		return RouteSources
	case strings.HasPrefix(path, "/api/v1/people") || strings.HasPrefix(path, "/api/v1/families") ||
		strings.HasPrefix(path, "/api/v1/tribes") || strings.HasPrefix(path, "/api/v1/branches") ||
		strings.HasPrefix(path, "/api/v1/person-aliases") || strings.HasPrefix(path, "/api/v1/entity-relationships") ||
		strings.HasPrefix(path, "/api/v1/normalize-name"):
		return RoutePeople
	case strings.HasPrefix(path, "/api/v1/trees") || strings.HasPrefix(path, "/api/v1/invitations"):
		return RouteTrees
	default:
		return RouteOther
	}
}

// RequestIDHeaderPath is the prefix of the local adapter's object route. It
// mirrors storage.ObjectPath and is duplicated rather than imported because
// storage imports nothing from telemetry and a telemetry package that imports the
// storage package to learn a path is a cycle waiting for the next person.
const RequestIDHeaderPath = "/local-objects/"

// AIOperationFor maps an ai-research endpoint path onto the enumeration. An
// endpoint this file does not know is AIOperationOther, which is a visible
// "uninstrumented" bucket rather than a silent gap.
func AIOperationFor(path string) AIOperation {
	switch strings.TrimSpace(path) {
	case "/v1/normalize-name":
		return AIOperationNormalizeName
	case "/v1/embed":
		return AIOperationEmbed
	case "/v1/classify":
		return AIOperationClassify
	case "/v1/extract/entities":
		return AIOperationExtractEntities
	case "/v1/extract/claims":
		return AIOperationExtractClaims
	case "/v1/resolve/entity":
		return AIOperationResolveEntity
	case "/v1/analyze/contradiction":
		return AIOperationContradiction
	case "/v1/rerank":
		return AIOperationRerank
	case "/v1/research/query":
		return AIOperationResearchQuery
	default:
		return AIOperationOther
	}
}

// QueryOperationFor normalizes a statement kind.
func QueryOperationFor(operation string) QueryOperation {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "select":
		return QuerySelect
	case "insert":
		return QueryInsert
	case "update":
		return QueryUpdate
	case "delete":
		return QueryDelete
	default:
		return QueryOther
	}
}

// ---------------------------------------------------------------------------
// The registry.
//
// A hand-written exposition in the Prometheus text format, rather than a client
// library. The reasons are the same ones the plan gives for not introducing a
// queue: the format is a few hundred lines, the data model needed here is a
// counter, a gauge and a histogram with no exemplars and no native histograms,
// and adding a metrics dependency to a service whose only other dependencies are
// a PostgreSQL driver and an object-storage SDK is a trade this repository has no
// reason to make. It is OpenTelemetry-compatible in the sense that matters for
// this deployment: a Prometheus-format endpoint on a local address, and nothing
// else required to read it.

// DefaultBuckets are the latency buckets, in seconds. They span a sub-millisecond
// cache-shaped query to a twenty-second AI call, which is the range this service
// actually has: the histogram is shared, and a range tuned for one caller would
// make the other's distribution a single bar.
var DefaultBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20}

type metricKind string

const (
	kindCounter   metricKind = "counter"
	kindGauge     metricKind = "gauge"
	kindHistogram metricKind = "histogram"
)

// One time series: a rendered label set and its single value, or its histogram
// state. A family with N dimensions has at most (the product of its
// enumerations) series, which is the property that bounds the memory of this
// package and the cardinality of what it exports.
type series struct {
	labels   string // the rendered, sorted label set
	value    float64
	buckets  []float64
	counts   []uint64
	sum      float64
	total    uint64
	lastSeen time.Time
}

// labelEnum answers "is this value legal for this label". Each family carries one
// per label, so two families can both have a label called "outcome" and mean
// different enumerations by it: a queue outcome and a research outcome are not the
// same set, and a guard that treated them as one would let either through on
// either.
type labelEnum struct {
	// size is how many values the enumeration has. It is recorded rather than
	// derived so the cardinality of a family is a number in this file, and a
	// reviewer can check the bound by hand instead of trusting a comment.
	size  int
	valid func(value string) bool
}

type family struct {
	name       string
	kind       metricKind
	help       string
	labelNames []string
	enums      []labelEnum
	series     map[string]*series
}

// Metrics is the registry. It is safe for concurrent use, and when it is disabled
// every recording call is a single atomic load and a return, so leaving the
// instrumentation in place costs nothing on a machine with no collector.
type Metrics struct {
	mu        sync.Mutex
	enabled   bool
	service   string
	families  []*family
	byName    map[string]*family
	now       func() time.Time
	dropped   uint64
	lastReset time.Time
}

// NewMetrics builds a registry. With enabled false it records nothing and its
// exposition is empty, which is the default: nothing in this repository needs a
// collector, and `make verify` and `make e2e` must not start needing one.
func NewMetrics(service string, enabled bool) *Metrics {
	registry := &Metrics{
		enabled: enabled,
		service: strings.TrimSpace(service),
		byName:  map[string]*family{},
		now:     time.Now,
	}
	registry.register("dawha_http_requests_total", kindCounter,
		"HTTP requests handled, by method, route family and status class. The route is a family, never a path: a path carries identifiers.",
		[]string{"method", "route", "status"}, methods(), routes(), statuses())
	registry.register("dawha_http_request_duration_seconds", kindHistogram,
		"Wall-clock duration of an HTTP request, by method, route family and status class.",
		[]string{"method", "route", "status"}, methods(), routes(), statuses())
	registry.register("dawha_db_queries_total", kindCounter,
		"Database statements executed, by statement kind. Never by table or by statement text.",
		[]string{"statement"}, queryOperations())
	registry.register("dawha_db_query_duration_seconds", kindHistogram,
		"Wall-clock duration of a database statement, by statement kind.",
		[]string{"statement"}, queryOperations())
	registry.register("dawha_queue_depth", kindGauge,
		"Jobs waiting to be claimed, by job type.",
		[]string{"job_type"}, jobTypes())
	registry.register("dawha_queue_jobs_total", kindCounter,
		"Queue outcomes, by job type and outcome.",
		[]string{"job_type", "outcome"}, jobTypes(), jobOutcomes())
	registry.register("dawha_queue_job_duration_seconds", kindHistogram,
		"Wall-clock duration of a claimed job, by job type.",
		[]string{"job_type"}, jobTypes())
	registry.register("dawha_ai_calls_total", kindCounter,
		"Calls to the AI service, by operation and outcome. The operation is the endpoint name, never its input.",
		[]string{"operation", "outcome"}, aiOperations(), aiOutcomes())
	registry.register("dawha_ai_duration_seconds", kindHistogram,
		"Wall-clock duration of a call to the AI service, by operation.",
		[]string{"operation"}, aiOperations())
	registry.register("dawha_ai_cost_units", kindCounter,
		"AI work in the units the provider bills, by operation. A NUMBER, never a model name or a prompt.",
		[]string{"operation"}, aiOperations())
	registry.register("dawha_research_runs_total", kindCounter,
		"Research runs by terminal outcome.",
		[]string{"outcome"}, researchOutcomes())
	registry.register("dawha_research_run_duration_seconds", kindHistogram,
		"Wall-clock duration of a research run.",
		nil)
	return registry
}

func (m *Metrics) register(name string, kind metricKind, help string, labels []string, enums ...labelEnum) {
	if len(enums) != len(labels) {
		// A family with an unenumerated label is the one thing this package must
		// never build: it would be a label whose values nothing constrains.
		panic("telemetry: metric family " + name + " has " + strconv.Itoa(len(labels)) + " labels and " + strconv.Itoa(len(enums)) + " enumerations")
	}
	f := &family{name: name, kind: kind, help: help, labelNames: labels, enums: enums, series: map[string]*series{}}
	m.families = append(m.families, f)
	m.byName[name] = f
}

func methods() labelEnum {
	return labelEnum{size: len(validMethods), valid: func(value string) bool { return validMethods[Method(value)] }}
}

func routes() labelEnum {
	return labelEnum{size: len(validRoutes), valid: func(value string) bool { return validRoutes[Route(value)] }}
}

func statuses() labelEnum {
	return labelEnum{size: len(validStatusClasses), valid: func(value string) bool { return validStatusClasses[StatusClass(value)] }}
}

func jobTypes() labelEnum {
	return labelEnum{size: len(validJobTypes), valid: func(value string) bool { return validJobTypes[JobType(value)] }}
}

func jobOutcomes() labelEnum {
	return labelEnum{size: len(validJobOutcomes), valid: func(value string) bool { return validJobOutcomes[JobOutcome(value)] }}
}

func aiOperations() labelEnum {
	return labelEnum{size: len(validAIOperations), valid: func(value string) bool { return validAIOperations[AIOperation(value)] }}
}

func aiOutcomes() labelEnum {
	return labelEnum{size: len(validAIOutcomes), valid: func(value string) bool { return validAIOutcomes[AIOutcome(value)] }}
}

func researchOutcomes() labelEnum {
	return labelEnum{size: len(validResearchOutcomes), valid: func(value string) bool { return validResearchOutcomes[ResearchOutcome(value)] }}
}

func queryOperations() labelEnum {
	return labelEnum{size: len(validQueryOperations), valid: func(value string) bool { return validQueryOperations[QueryOperation(value)] }}
}

// Enabled reports whether recording is on. The instrumentation checks this so a
// disabled registry does no work at all.
func (m *Metrics) Enabled() bool {
	return m != nil && m.enabled
}

func (m *Metrics) family(name string) *family {
	if m == nil {
		return nil
	}
	return m.byName[name]
}

// record adds to a counter or sets a gauge. It re-validates the label values
// against the enumerations before they are stored: the recording API takes
// enumeration types, so this cannot normally fail, and if it does the sample is
// DROPPED rather than stored with an unbounded value.
func (m *Metrics) record(name string, delta float64, labels ...string) {
	if !m.Enabled() {
		return
	}
	f := m.family(name)
	if f == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.seriesFor(f, labels)
	if s == nil {
		return
	}
	s.value += delta
	s.lastSeen = m.now()
}

func (m *Metrics) set(name string, value float64, labels ...string) {
	if !m.Enabled() {
		return
	}
	f := m.family(name)
	if f == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.seriesFor(f, labels)
	if s == nil {
		return
	}
	s.value = value
	s.lastSeen = m.now()
}

func (m *Metrics) observe(name string, seconds float64, buckets []float64, labels ...string) {
	if !m.Enabled() {
		return
	}
	f := m.family(name)
	if f == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.seriesFor(f, labels)
	if s == nil {
		return
	}
	s.sum += seconds
	s.total++
	if s.buckets == nil {
		s.buckets = append([]float64(nil), buckets...)
		s.counts = make([]uint64, len(s.buckets))
	}
	for i, upper := range s.buckets {
		if seconds <= upper {
			s.counts[i]++
		}
	}
	s.lastSeen = m.now()
}

// seriesFor finds or creates the series for a label set, and returns nil when a
// label value is not one of the enumerations. Callers hold the lock.
func (m *Metrics) seriesFor(f *family, labels []string) *series {
	if !validLabels(f, labels) {
		// A label value outside its enumeration cannot come from the typed API,
		// so this is a bug in a caller rather than bad input. The sample is
		// dropped and counted, because storing it would put unbounded text into
		// an exported label - which is the one thing this package must never do.
		m.dropped++
		return nil
	}
	key, rendered := renderLabels(f.labelNames, labels)
	if s, ok := f.series[key]; ok {
		return s
	}
	s := &series{labels: rendered}
	f.series[key] = s
	return s
}

func renderLabels(names, values []string) (string, string) {
	type pair struct{ name, value string }
	pairs := make([]pair, 0, len(names))
	for i, name := range names {
		pairs = append(pairs, pair{name, values[i]})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })
	var key strings.Builder
	var rendered strings.Builder
	rendered.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			key.WriteByte(',')
			rendered.WriteByte(',')
		}
		key.WriteString(p.name)
		key.WriteByte('=')
		key.WriteString(p.value)
		rendered.WriteString(p.name)
		rendered.WriteString(`="`)
		rendered.WriteString(escapeLabelValue(p.value))
		rendered.WriteString(`"`)
	}
	rendered.WriteByte('}')
	return key.String(), rendered.String()
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return replacer.Replace(value)
}

// validLabels is the drop check: every label value must be a member of the
// enumeration ITS family declared for that label. A value outside it means a
// caller got hold of a string the type system should have prevented, and the
// sample is discarded rather than exported.
func validLabels(f *family, labels []string) bool {
	if len(labels) != len(f.labelNames) || len(labels) != len(f.enums) {
		return false
	}
	for i := range labels {
		if f.enums[i].valid == nil || !f.enums[i].valid(labels[i]) {
			return false
		}
	}
	return true
}

// DroppedSamples reports how many samples were discarded because a label value
// was not in its enumeration. A non-zero value is a bug in a caller, so it is
// exposed rather than logged: the number is the whole point.
func (m *Metrics) DroppedSamples() uint64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropped
}

// Text renders the Prometheus text exposition. It is the only thing the exporter
// serves, and it renders only families that have samples, so a disabled or idle
// registry produces a valid, empty document rather than a wall of zeros.
func (m *Metrics) Text() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out strings.Builder
	for _, f := range m.families {
		if len(f.series) == 0 {
			continue
		}
		out.WriteString("# HELP " + f.name + " " + f.help + "\n")
		out.WriteString("# TYPE " + f.name + " " + string(f.kind) + "\n")
		keys := make([]string, 0, len(f.series))
		for key := range f.series {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			s := f.series[key]
			switch f.kind {
			case kindHistogram:
				for i, upper := range s.buckets {
					out.WriteString(f.name + "_bucket" + mergeLabels(s.labels, "le", formatFloat(upper)) + " " + strconv.FormatUint(s.counts[i], 10) + "\n")
				}
				out.WriteString(f.name + "_bucket" + mergeLabels(s.labels, "le", "+Inf") + " " + strconv.FormatUint(s.total, 10) + "\n")
				out.WriteString(f.name + "_sum" + withOptionalLabels(s.labels) + " " + formatFloat(s.sum) + "\n")
				out.WriteString(f.name + "_count" + withOptionalLabels(s.labels) + " " + strconv.FormatUint(s.total, 10) + "\n")
			default:
				out.WriteString(f.name + s.labels + " " + formatFloat(s.value) + "\n")
			}
		}
	}
	return out.String()
}

// withOptionalLabels omits an empty label set rather than writing `{}`, which is
// what the Prometheus text format expects for an unlabelled series.
func withOptionalLabels(rendered string) string {
	if rendered == "{}" {
		return ""
	}
	return rendered
}

func mergeLabels(existing, name, value string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(existing, "{"), "}")
	if inner == "" {
		return "{" + name + `="` + escapeLabelValue(value) + `"}`
	}
	return "{" + inner + "," + name + `="` + escapeLabelValue(value) + `"}`
}

func formatFloat(value float64) string {
	if value == float64(int64(value)) && value < 1e15 && value > -1e15 {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}
