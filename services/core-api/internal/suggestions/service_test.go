package suggestions

import "testing"

func TestValidateSubmitInputPreservesCanonicalText(t *testing.T) {
	text := "  أضفوا فقرة عن هذا الاسم.\nالصفحة 12  "
	input, err := validateSubmitInput(SubmitInput{
		TreeID:    "00000000-0000-0000-0000-000000000001",
		VersionID: "00000000-0000-0000-0000-000000000002",
		NodeID:    "00000000-0000-0000-0000-000000000003",
		TextAR:    text,
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.TextAR != text {
		t.Fatalf("canonical text changed: %q", input.TextAR)
	}
}

func TestValidateSubmitInputRejectsInvalidResource(t *testing.T) {
	_, err := validateSubmitInput(SubmitInput{TreeID: "not-a-uuid", VersionID: "v", NodeID: "n", TextAR: "مقترح"})
	if err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestValidateReviewInput(t *testing.T) {
	input, err := validateReviewInput(ReviewInput{Decision: " converted ", NoteAR: " ملاحظة ", QuestionTitleAR: " سؤال "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Decision != "converted" || input.NoteAR != "ملاحظة" || input.QuestionTitleAR != "سؤال" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
	if _, err := validateReviewInput(ReviewInput{Decision: "rejected"}); err != ErrValidation {
		t.Fatalf("expected rejection note validation error, got %v", err)
	}
}

func TestDefaultQuestionTitle(t *testing.T) {
	title := defaultQuestionTitle("مقترح طويل جداً يحتاج إلى سؤال")
	if title == "" {
		t.Fatal("expected a generated title")
	}
}
