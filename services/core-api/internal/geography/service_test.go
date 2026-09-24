package geography

import "testing"

func TestValidateInput(t *testing.T) {
	input, err := validateInput(MapInput{FromYear: 1100, ToYear: 1250, Status: "disputed"})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Status != "disputed" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
	if _, err := validateInput(MapInput{FromYear: 1300, ToYear: 1200}); err != ErrValidation {
		t.Fatalf("expected range validation error, got %v", err)
	}
	if _, err := validateInput(MapInput{Status: "invented"}); err != ErrValidation {
		t.Fatalf("expected status validation error, got %v", err)
	}
}

func TestMatchesPeriodAndPlace(t *testing.T) {
	feature := Feature{Kind: "place", PlaceID: "place-1", Status: "documented", TimeFrom: "1100-01-01", TimeTo: "1150-01-01"}
	if !matches(feature, MapInput{PlaceID: "place-1", FromYear: 1120, ToYear: 1140}) {
		t.Fatal("expected feature to overlap period")
	}
	if matches(feature, MapInput{FromYear: 1200, ToYear: 1250}) {
		t.Fatal("did not expect feature outside period")
	}
}
