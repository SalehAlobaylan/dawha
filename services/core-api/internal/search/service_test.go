package search

import "testing"

func TestValidateInputDefaults(t *testing.T) {
	input, normalized, vector, err := validateInput(Input{Query: "  عَبْدُ الله  "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Limit != 20 || normalized != "عبد الله" || vector != nil {
		t.Fatalf("unexpected normalized input: %+v, %q, %v", input, normalized, vector)
	}
}

func TestValidateInputRejectsInvalidFilters(t *testing.T) {
	if _, _, _, err := validateInput(Input{Query: "سؤال", Kind: "unknown"}); err != ErrValidation {
		t.Fatalf("expected kind validation error, got %v", err)
	}
	if _, _, _, err := validateInput(Input{Query: "سؤال", PersonID: "not-a-uuid"}); err != ErrValidation {
		t.Fatalf("expected id validation error, got %v", err)
	}
	if _, _, _, err := validateInput(Input{Query: "سؤال", Embedding: "[1,2]"}); err != ErrValidation {
		t.Fatalf("expected embedding validation error, got %v", err)
	}
}

func TestAppendGroupSkipsEmptyResults(t *testing.T) {
	groups := appendGroup(nil, "names", "الأسماء", nil)
	if len(groups) != 0 {
		t.Fatalf("expected empty groups to be skipped: %#v", groups)
	}
	groups = appendGroup(groups, "names", "الأسماء", []Result{{ID: "1"}})
	if len(groups) != 1 || groups[0].Key != "names" {
		t.Fatalf("unexpected groups: %#v", groups)
	}
}
