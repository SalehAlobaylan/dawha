package trees

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidateCreateInputDefaultsVisibilityAndGender(t *testing.T) {
	input, err := validateCreateInput(CreateTreeInput{
		Name:   "  شجرة جديدة  ",
		People: []PersonInput{{CanonicalName: "  محمد  "}},
	})

	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Name != "شجرة جديدة" || input.Visibility != "private" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
	if input.People[0].CanonicalName != "محمد" || input.People[0].Gender != "unknown" {
		t.Fatalf("unexpected normalized person: %+v", input.People[0])
	}
}

func TestValidateCreateInputRejectsUnknownVisibility(t *testing.T) {
	_, err := validateCreateInput(CreateTreeInput{Name: "شجرة", Visibility: "secret"})
	if err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestParseDateAndFormatYears(t *testing.T) {
	date, err := parseDate("1180-04-12")
	if err != nil {
		t.Fatalf("unexpected date error: %v", err)
	}
	if !date.Valid || date.Time.Year() != 1180 {
		t.Fatalf("unexpected parsed date: %+v", date)
	}
	years := formatYears(date, pgtype.Date{}, pgtype.Date{Time: time.Date(1210, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}, pgtype.Date{})
	if years != "1180 — 1210" {
		t.Fatalf("unexpected years: %q", years)
	}
}

func TestPublishedOnlyForViewer(t *testing.T) {
	tree := TreeSummary{Visibility: "public", OwnerID: "owner"}
	if publishedOnlyForViewer(tree, "owner") {
		t.Fatal("owner should see the latest draft")
	}
	if !publishedOnlyForViewer(tree, "other") {
		t.Fatal("non-owner should see only published versions")
	}
	if !publishedOnlyForViewer(tree, "") {
		t.Fatal("anonymous visitor should see only published versions")
	}
	privateTree := TreeSummary{Visibility: "private", OwnerID: "owner"}
	if publishedOnlyForViewer(privateTree, "other") {
		t.Fatal("private trees should not be filtered as public versions")
	}
}

func TestValidatePersonInputNormalizesDefaults(t *testing.T) {
	person, err := validatePersonInput(PersonInput{CanonicalName: "  سارة  "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if person.CanonicalName != "سارة" || person.Gender != "unknown" {
		t.Fatalf("unexpected normalized person: %+v", person)
	}
}

func TestValidateRelationshipInputDefaultsStatus(t *testing.T) {
	relationship, err := validateRelationshipInput(AddRelationshipInput{
		SubjectNodeID: " subject ",
		ObjectNodeID:  " object ",
		Predicate:     "parent_of",
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if relationship.SubjectNodeID != "subject" || relationship.ObjectNodeID != "object" || relationship.Status != "interpreted" {
		t.Fatalf("unexpected normalized relationship: %+v", relationship)
	}
}

func TestValidateRelationshipInputRejectsUnknownValues(t *testing.T) {
	_, err := validateRelationshipInput(AddRelationshipInput{SubjectNodeID: "a", ObjectNodeID: "b", Predicate: "ancestor_of"})
	if err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestParsePersonDatesRejectsInvertedRange(t *testing.T) {
	_, _, _, _, err := parsePersonDates(PersonInput{
		BirthDateFrom: "1200-01-01",
		BirthDateTo:   "1100-01-01",
	})
	if err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}
