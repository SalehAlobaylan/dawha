package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

// Request IDs are how one HTTP request, the AI calls it makes and the job it
// enqueues are found in the logs afterwards. Three properties matter and all
// three are enforced here rather than left to each caller.
//
// A client may supply one, and that is useful: a browser, a proxy and a support
// ticket all speak the same id. But a client-supplied id is attacker-influenced
// text that ends up in a log line AND, if it were used as a metric label, in the
// cardinality of a time series. So an id is only accepted when it is short,
// printable and drawn from a small alphabet; anything else is replaced. The
// replacement is logged in the response header the client can see, so a
// misbehaving proxy is visible rather than mysterious.
//
// The alphabet is 32 characters - lowercase hex plus a dash - which means no
// newline, no quote and no space. That is a log-injection defence as much as a
// cardinality one: a request id that cannot contain a newline cannot forge a log
// line.
const (
	requestIDHeader       = "X-Request-ID"
	maxRequestIDLength    = 64
	requestIDFallbackName = "request"
)

type requestIDKey struct{}

// NewRequestID returns a fresh id. It is time-ordered and random: the time
// prefix makes a log easy to read, the random suffix means two processes
// starting in the same microsecond do not collide.
func NewRequestID() string {
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		// crypto/rand failing is not something to paper over. A constant suffix
		// would make two requests share an id, which is worse than an ugly one.
		return fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102T150405.000000"), atomic.AddUint64(&idFallback, 1))
	}
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405.000000"), hex.EncodeToString(suffix[:]))
}

var idFallback uint64

// AcceptRequestID returns the request id to use for a request: the client's, if
// it is well formed, and a fresh one if it is absent or not.
//
// The rejection is silent on the wire and loud in the tests: the client always
// gets an id back in the header, and a caller that supplied a bad one has it
// replaced. There is no 400, because refusing a request over its correlation id
// would be a strange failure for an operator to debug at 3am.
func AcceptRequestID(supplied string) string {
	if isAcceptableRequestID(supplied) {
		return supplied
	}
	return NewRequestID()
}

func isAcceptableRequestID(value string) bool {
	if value == "" || len(value) > maxRequestIDLength {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '-' || character == '_' || character == '.':
		default:
			return false
		}
	}
	return true
}

// WithRequestID returns a context carrying the id. Everything downstream - a
// handler, an AI client, a job enqueue - reads it from here rather than being
// passed a string, so a new call site cannot forget to thread it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the id in a context, or "" when there is none. "" is a
// legitimate answer: background work with no request behind it has no request id,
// and pretending otherwise with a placeholder would put a constant in a field
// whose whole purpose is to say "these lines are the same request".
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// RequestIDFromHeader is the header name, exported because the middleware, the
// workers and the tests all need to agree on it and a string literal in four
// places is four chances to typo a correlation.
const RequestIDHeader = requestIDHeader

// WithLogger returns a logger that carries the request id from ctx on every line
// it writes.
//
// This is the mechanism the whole correlation story rests on: a handler that
// logs through this logger cannot produce a line without the id, because the id
// is bound to the logger rather than passed to each call. A call site that wanted
// no id would have to reach past it deliberately.
func WithLogger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	id := RequestID(ctx)
	if id == "" {
		return logger
	}
	return logger.With(slog.String("request_id", id))
}

// PropagateRequestID copies the request id out of ctx and into a payload map.
//
// This is the seam where correlation crosses a process boundary. A job enqueued
// inside a request carries the id in its payload, and the worker that claims it
// reads it back, so the line that fails at 3am in the source-processing worker
// can be traced to the upload that caused it. It is a plain map operation rather
// than a struct field so that it works for every job payload in the repository,
// including the ones defined in five different packages.
func PropagateRequestID(ctx context.Context, payload map[string]any) {
	if payload == nil {
		return
	}
	if id := RequestID(ctx); id != "" {
		payload[RequestIDPayloadKey] = id
	}
}

// RequestIDPayloadKey is the payload key the request id travels under.
const RequestIDPayloadKey = "request_id"

// RequestIDFromPayload reads a request id back out of a job payload. It answers
// "" for a payload that never had one, which is the case for every job enqueued
// before this existed and for background work.
func RequestIDFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[RequestIDPayloadKey]
	if !ok {
		return ""
	}
	id, ok := value.(string)
	if !ok {
		return ""
	}
	// The payload is a JSON object that a worker parses, and a value in it came
	// from a database row at some point. It gets the same treatment a header does.
	if !isAcceptableRequestID(id) {
		return ""
	}
	return strings.TrimSpace(id)
}
