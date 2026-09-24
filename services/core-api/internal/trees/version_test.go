package trees

import (
	"errors"
	"testing"
	"time"
)

func TestPublishedVersionCannotBeChanged(t *testing.T) {
	version := NewDraft(1)
	now := time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC)

	if err := Publish(&version, "user-1", "first publication", now); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}
	if err := Publish(&version, "user-1", "second publication", now.Add(time.Hour)); !errors.Is(err, ErrAlreadyPublished) {
		t.Fatalf("expected immutable version error, got %v", err)
	}
	if version.PublishedBy != "user-1" {
		t.Fatalf("unexpected publisher %q", version.PublishedBy)
	}
}

func TestPublishRequiresActor(t *testing.T) {
	version := NewDraft(1)

	if err := Publish(&version, "", "", time.Now()); !errors.Is(err, ErrMissingActor) {
		t.Fatalf("expected missing actor error, got %v", err)
	}
}
