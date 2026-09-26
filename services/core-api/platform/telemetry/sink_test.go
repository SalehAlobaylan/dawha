package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
)

// The log sink behind the correlation tests.
//
// A slog handler receives `logger.With(...)` as a NEW handler rather than as
// extra arguments to the next Handle call, so a sink that kept its records in its
// own fields would see nothing from a logger that had been given attributes. The
// records therefore live in a store every derived handler shares - which is also
// what a real handler does, and what makes this test able to say "the request id
// is on the line" rather than "the request id is on a handler nobody used".
type logSink struct {
	store *logStore
	bound map[string]any
}

type logStore struct {
	mu      sync.Mutex
	written []string
}

func newLogSink() *logSink { return &logSink{store: &logStore{}} }

func (s *logSink) Enabled(context.Context, slog.Level) bool { return true }

func (s *logSink) Handle(_ context.Context, record slog.Record) error {
	fields := map[string]any{}
	for name, value := range s.bound {
		fields[name] = value
	}
	record.Attrs(func(attr slog.Attr) bool {
		fields[attr.Key] = attr.Value.Any()
		return true
	})
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.written = append(s.store.written, string(encoded))
	return nil
}

func (s *logSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := map[string]any{}
	for name, value := range s.bound {
		merged[name] = value
	}
	for _, attr := range attrs {
		merged[attr.Key] = attr.Value.Any()
	}
	return &logSink{store: s.store, bound: merged}
}

func (s *logSink) WithGroup(string) slog.Handler { return s }

func (s *logSink) lines() []string {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return append([]string(nil), s.store.written...)
}

func (s *logSink) records() int {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return len(s.store.written)
}

func (s *logSink) reset() {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.written = nil
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		t.Fatalf("a log line is not json: %v (%s)", err, line)
	}
	return fields
}
