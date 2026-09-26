package telemetry

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The pgx tracer that measures database latency.
//
// It is here, in the telemetry package, rather than in platform/db, because the
// thing it produces is a metric and the thing it reads is a pgx hook - and
// because putting it here means platform/db has no opinion about telemetry at all
// beyond "here is the tracer to install".
//
// The measurement is the statement KIND and the wall-clock time. Not the
// statement, not its arguments, not the table. pgx hands a tracer the SQL text and
// the arguments, and this file deliberately reads neither beyond the leading
// keyword: the arguments are the ids, names and text of whatever the caller was
// doing, and a database latency metric that holds any of it is a copy of the
// database in a time series store.

// queryTracer implements pgx.QueryTracer. The zero value is not usable;
// NewQueryTracer returns one bound to a registry.
type queryTracer struct {
	metrics *Metrics
	// started keeps the begin time per call. pgx hands TraceQueryEnd a context
	// derived from the one TraceQueryStart returned, and the tracer is shared by
	// every connection in the pool, so the value travels in the context rather
	// than in a field.
	started *queryStartKey
}

type queryStartKey struct{}

// NewQueryTracer returns a pgx tracer that records statement latency into a
// registry. A nil registry returns nil, and pgx accepts a nil tracer as "no
// tracer", which is how telemetry stays off by default without every caller
// having to check.
func NewQueryTracer(metrics *Metrics) pgx.QueryTracer {
	if !metrics.Enabled() {
		return nil
	}
	return &queryTracer{metrics: metrics, started: &queryStartKey{}}
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if t == nil || !t.metrics.Enabled() {
		return ctx
	}
	// Only the leading keyword is read. The rest of data.SQL is not looked at,
	// which is the whole point: a tracer that reads a statement to classify it is
	// one edit away from reading a statement to log it.
	operation := QueryOperationFor(leadingKeyword(data.SQL))
	return context.WithValue(ctx, t.started, querySample{operation: operation, at: time.Now()})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if t == nil || !t.metrics.Enabled() {
		return
	}
	sample, ok := ctx.Value(t.started).(querySample)
	if !ok {
		return
	}
	t.metrics.DatabaseQuery(sample.operation, time.Since(sample.at))
}

type querySample struct {
	operation QueryOperation
	at        time.Time
}

// maxKeywordLength is the longest first token this function will believe is a
// keyword. The longest SQL keyword is "intersect" at nine; anything longer is
// either a function call or the beginning of a comment, and both are "other".
const maxKeywordLength = 12

// leadingKeyword returns the first word of a statement, uppercased, or "" when
// there is not one.
//
// It is deliberately the only thing this package knows how to read out of a SQL
// string. It allocates nothing, it stops at the first token, and it never looks at
// what follows - so a statement carrying a person's name in a WHERE clause cannot
// reach a label, whatever somebody does to this function later.
func leadingKeyword(statement string) string {
	start := -1
	for index := 0; index < len(statement); index++ {
		character := statement[index]
		isSeparator := character == ' ' || character == '\t' || character == '\n' || character == '\r' || character == '(' || character == ';'
		if !isSeparator {
			if start < 0 {
				start = index
			}
			if index-start >= maxKeywordLength {
				return ""
			}
			continue
		}
		if start >= 0 {
			return strings.ToUpper(statement[start:index])
		}
	}
	if start >= 0 {
		return strings.ToUpper(statement[start:])
	}
	return ""
}
