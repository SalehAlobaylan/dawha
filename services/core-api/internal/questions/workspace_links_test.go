package questions

import (
	"context"
	"errors"
	"testing"
)

func TestValidateEntityLink(t *testing.T) {
	value := "10000000-0000-0000-0000-000000000001"
	if _, err := validateEntityLink("person", value); err != nil {
		t.Fatalf("expected valid entity link: %v", err)
	}
	for _, input := range []struct {
		entityType string
		entityID   string
	}{
		{entityType: "unknown", entityID: value},
		{entityType: "person", entityID: "not-a-uuid"},
	} {
		if _, err := validateEntityLink(input.entityType, input.entityID); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected validation error for %+v, got %v", input, err)
		}
	}
}

func TestQuestionLinksRequireDatabase(t *testing.T) {
	service := &Service{}
	if _, err := service.LinkEntity(context.Background(), "80000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000001", QuestionEntityInput{EntityType: "person", EntityID: "10000000-0000-0000-0000-000000000001"}); !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("expected database error, got %v", err)
	}
	if _, err := service.LinkFinding(context.Background(), "80000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000001", QuestionFindingInput{FindingID: "70000000-0000-0000-0000-000000000001"}); !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("expected database error, got %v", err)
	}
}
