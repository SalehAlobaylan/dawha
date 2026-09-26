package testsupport

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QueryCounter records every statement a service sends, so a test can pin the
// number of round trips one endpoint costs.
//
// A count is the only measurement that survives a change in *how* a query is
// written. Row counts and timings both move when a fixture changes size, so a
// test that asserts either of them fails for a reason that has nothing to do
// with the code. A statement count is the thing the batching work is actually
// about: one grouped query instead of one query per row, or per run. Pinning
// the count turns "we removed the N+1" into something a later change cannot
// quietly undo.
type QueryCounter struct {
	mu    sync.Mutex
	stmts []string
}

// TraceQueryStart records the statement. The record happens here because the
// statement text only exists in the start event, and a query that failed still
// cost a round trip.
func (c *QueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stmts = append(c.stmts, data.SQL)
	return ctx
}

// TraceQueryEnd satisfies the rest of pgx.QueryTracer. The counter reads nothing
// from it.
func (c *QueryCounter) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
}

// Reset forgets the statements recorded so far, so a test can measure a warm-up
// or a setup phase separately from the call it is about.
func (c *QueryCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stmts = nil
}

// Count is how many statements have been recorded since the last Reset.
func (c *QueryCounter) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.stmts)
}

// Statements returns a copy of the recorded statements, in order.
func (c *QueryCounter) Statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.stmts...)
}

// StatementsMatching returns the recorded statements whose text matches pattern.
func (c *QueryCounter) StatementsMatching(pattern string) []string {
	matcher, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	matches := make([]string, 0)
	for _, statement := range c.Statements() {
		if matcher.MatchString(statement) {
			matches = append(matches, statement)
		}
	}
	return matches
}

// CountMatching returns how many recorded statements match pattern.
func (c *QueryCounter) CountMatching(pattern string) int {
	return len(c.StatementsMatching(pattern))
}

// Report renders the recorded statements for a failure message. A query-count
// assertion that fails should say what the extra round trips were, not only how
// many there were.
func (c *QueryCounter) Report() string {
	statements := c.Statements()
	if len(statements) == 0 {
		return "(no statements recorded)"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%d statement(s):\n", len(statements))
	for index, statement := range statements {
		collapsed := strings.Join(strings.Fields(statement), " ")
		if len(collapsed) > 160 {
			collapsed = collapsed[:160] + "…"
		}
		fmt.Fprintf(&builder, "  %d. %s\n", index+1, collapsed)
	}
	return builder.String()
}

// AssertAtMost fails the test when the counter recorded more than limit
// statements. It is the pin that keeps a batched hydration batched.
func (c *QueryCounter) AssertAtMost(t *testing.T, limit int, what string) {
	t.Helper()
	if count := c.Count(); count > limit {
		t.Fatalf("%s cost %d queries, want at most %d:\n%s", what, count, limit, c.Report())
	}
}

// AssertAtLeast fails the test when the counter recorded fewer than limit
// statements. It keeps a measurement honest in the other direction: a service
// that answered without asking the database at all would otherwise satisfy
// AssertAtMost.
func (c *QueryCounter) AssertAtLeast(t *testing.T, limit int, what string) {
	t.Helper()
	if count := c.Count(); count < limit {
		t.Fatalf("%s cost %d queries, want at least %d:\n%s", what, count, limit, c.Report())
	}
}

// CountingPool opens a pool that records every statement sent through it. The
// returned pool is an ordinary *pgxpool.Pool, so a service takes it exactly as
// it takes any other pool.
func CountingPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, *QueryCounter, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, nil, err
	}
	counter := &QueryCounter{}
	config.ConnConfig.Tracer = counter
	config.MaxConns = 4
	config.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return pool, counter, nil
}

// CountingFixturePool is CountingPool pointed at a fixture's isolated schema, so
// a query count is measured against data the test owns rather than against
// whatever the shared development database happens to hold.
func (f *Fixture) CountingFixturePool(t *testing.T) (*pgxpool.Pool, *QueryCounter) {
	t.Helper()
	pool, counter, err := CountingPool(f.ctx, f.DatabaseURL())
	if err != nil {
		t.Fatalf("open counting pool for fixture: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, counter
}

// PayloadBytes is the encoded size of a response value. It is the other half of
// a hydration measurement: a query count says how many round trips a call cost,
// and this says how much crossed the wire.
func PayloadBytes(t *testing.T, value any) int {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return len(encoded)
}
