package sourceprocessing

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateBookAssertionInputRequiresObjectForSpecificRelationships(t *testing.T) {
	base := BookAssertionInput{
		PageEvidenceID:         uuid.NewString(),
		PageReviewRevisionID:   uuid.NewString(),
		ExpectedReviewRevision: 1,
		QuoteStartCodepoint:    0,
		QuoteEndCodepoint:      1,
		QuoteTextAR:            "ن",
		SubjectMentionAR:       "سالم",
		Polarity:               "affirmed",
		Certainty:              "asserted",
	}

	for _, predicate := range []string{"parent_of", "spouse_of", "sibling_of", "descendant_of"} {
		t.Run(predicate, func(t *testing.T) {
			input := base
			input.Predicate = predicate
			if _, err := validateBookAssertionInput(input); err != ErrValidation {
				t.Fatalf("empty object mention error = %v, want validation error", err)
			}
		})
	}
}

func TestValidateBookAssertionInputAllowsUnspecifiedRelationshipAndClearsNameOnlyObject(t *testing.T) {
	base := BookAssertionInput{
		PageEvidenceID:         uuid.NewString(),
		PageReviewRevisionID:   uuid.NewString(),
		ExpectedReviewRevision: 1,
		QuoteStartCodepoint:    0,
		QuoteEndCodepoint:      1,
		QuoteTextAR:            "ن",
		SubjectMentionAR:       "سالم",
		Polarity:               "affirmed",
		Certainty:              "asserted",
	}

	input := base
	input.Predicate = "reported_relationship"
	if _, err := validateBookAssertionInput(input); err != nil {
		t.Fatalf("unspecified reported relationship: %v", err)
	}

	input.Predicate = "mention_only"
	input.ObjectMentionAR = "stale disabled-field value"
	validated, err := validateBookAssertionInput(input)
	if err != nil {
		t.Fatalf("name-only mention with stale object value: %v", err)
	}
	if validated.ObjectMentionAR != "" {
		t.Fatalf("name-only object mention = %q, want empty", validated.ObjectMentionAR)
	}
}
