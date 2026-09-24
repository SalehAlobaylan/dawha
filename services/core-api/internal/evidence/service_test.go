package evidence

import "testing"

func TestValidateSourceInputDefaultsDependency(t *testing.T) {
	input, err := validateSourceInput(CreateSourceInput{TitleAR: "  مصدر عربي  ", SourceType: "manuscript"})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.TitleAR != "مصدر عربي" || input.DependencyStatus != "unknown" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
}

func TestValidateSourceInputRejectsUnknownType(t *testing.T) {
	if _, err := validateSourceInput(CreateSourceInput{TitleAR: "مصدر", SourceType: "secret"}); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestValidateStatementInputDefaultsReviewStatus(t *testing.T) {
	input, err := validateStatementInput(SourceStatementInput{StatementTextAR: "  نص المصدر  "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.StatementTextAR != "نص المصدر" || input.ReviewStatus != "unreviewed" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
}

func TestValidateClaimInputAcceptsUnresolvedClaim(t *testing.T) {
	input, err := validateClaimInput(CreateClaimInput{
		SubjectType: "person",
		SubjectID:   "00000000-0000-0000-0000-000000000001",
		Predicate:   "father_of",
		ObjectType:  "person",
		ObjectID:    "00000000-0000-0000-0000-000000000002",
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Status != "unresolved" {
		t.Fatalf("unexpected default status: %+v", input)
	}
}

func TestValidateEvidenceInputRequiresReference(t *testing.T) {
	if _, err := validateEvidenceInput(AddEvidenceInput{Relation: "supports"}); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestParseDateRangeRejectsInvertedRange(t *testing.T) {
	if _, _, err := parseDateRange("1200-01-01", "1100-01-01"); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}
