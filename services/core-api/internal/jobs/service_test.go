package jobs

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidateEnqueueDefaults(t *testing.T) {
	input, err := validateEnqueue(EnqueueInput{Type: "  source_process  "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Type != "source_process" || input.MaxAttempts != 3 || string(input.Payload) != `{}` {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
}

func TestValidateEnqueueRejectsInvalidPayload(t *testing.T) {
	_, err := validateEnqueue(EnqueueInput{Type: "source_process", Payload: json.RawMessage(`not-json`)})
	if err != ErrValidation {
		t.Fatalf("expected payload validation error, got %v", err)
	}
}

func TestBackoffIsBounded(t *testing.T) {
	if backoff(1) != time.Second || backoff(3) != 4*time.Second || backoff(20) != 512*time.Second {
		t.Fatalf("unexpected backoff values: %v %v %v", backoff(1), backoff(3), backoff(20))
	}
}

func TestValidateListFilter(t *testing.T) {
	filter, err := validateListFilter(ListFilter{Status: "queued", Limit: 10})
	if err != nil || filter.Status != "queued" || filter.Limit != 10 {
		t.Fatalf("unexpected filter: %+v, %v", filter, err)
	}
	if _, err := validateListFilter(ListFilter{Status: "unknown"}); err != ErrValidation {
		t.Fatalf("expected status validation error, got %v", err)
	}
}
