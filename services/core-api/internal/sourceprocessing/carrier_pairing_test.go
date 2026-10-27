package sourceprocessing

import (
	"math"
	"testing"
)

// Two renderings of the same chart read by the same reader agree on the names
// they can see. This is the name-level counterpart of the count-level check:
// twenty names on both sides can still be twenty different names.

func TestCompareCarrierTextsScoresIdenticalRenderingsAsAgreement(t *testing.T) {
	names := []string{"محمد بن علي", "عبد الله بن محمد", "سارة بنت أحمد"}
	agreement := CompareCarrierTexts(names, names, 0.8)
	if agreement.Matched != 3 || agreement.ExactMatched != 3 {
		t.Fatalf("identical readings = %+v, want three exact matches", agreement)
	}
	if agreement.Agreement != 1 {
		t.Fatalf("agreement = %v, want 1", agreement.Agreement)
	}
	if len(agreement.LeftOnly) != 0 || len(agreement.RightOnly) != 0 {
		t.Fatalf("identical readings produced unmatched names: %+v", agreement)
	}
}

func TestCompareCarrierTextsPairsASpellingVariant(t *testing.T) {
	// The two renderings are read independently; one may split a word, the
	// other may not. The normaliser folds the diacritics, and the trigram
	// similarity folds the spacing.
	left := []string{"عبدالله بن محمد", "خالد بن سعود"}
	right := []string{"عبد الله بن محمد", "خالد بن سعود"}
	agreement := CompareCarrierTexts(left, right, 0.8)
	if agreement.Matched != 2 || agreement.FuzzyMatched != 1 || agreement.ExactMatched != 1 {
		t.Fatalf("readings = %+v, want one exact and one fuzzy match", agreement)
	}
	if math.Abs(agreement.Agreement-1) > 0.0001 {
		t.Fatalf("agreement = %v, want 1 despite the spelling variant", agreement.Agreement)
	}
}

func TestCompareCarrierTextsReportsWhatOnlyOneRenderingSaw(t *testing.T) {
	left := []string{"محمد بن علي", "عبد الله بن محمد", "سارة بنت أحمد", "خالد بن سعود"}
	right := []string{"محمد بن علي", "عبد الله بن محمد", "سارة بنت أحمد"}
	agreement := CompareCarrierTexts(left, right, 0.8)
	if agreement.Matched != 3 {
		t.Fatalf("matched = %d, want 3", agreement.Matched)
	}
	if len(agreement.LeftOnly) != 1 || agreement.LeftOnly[0] != "خالد بن سعود" {
		t.Fatalf("left-only = %v, want the fourth name", agreement.LeftOnly)
	}
	if len(agreement.RightOnly) != 0 {
		t.Fatalf("right-only = %v, want none", agreement.RightOnly)
	}
	expected := float64(2*3) / float64(4+3)
	if math.Abs(agreement.Agreement-expected) > 0.0001 {
		t.Fatalf("agreement = %v, want %v", agreement.Agreement, expected)
	}
}

func TestCompareCarrierTextsDoesNotCallDifferentNamesAgreement(t *testing.T) {
	agreement := CompareCarrierTexts(
		[]string{"محمد بن علي", "خالد بن سعود"},
		[]string{"محمد بن حسن", "سلمان بن فهد"},
		0.8,
	)
	if agreement.Matched != 0 {
		t.Fatalf("different names were paired: %+v", agreement)
	}
	if agreement.Agreement != 0 {
		t.Fatalf("agreement = %v, want 0", agreement.Agreement)
	}
}

func TestCompareCarrierTextsPairsEachReadingOnce(t *testing.T) {
	// A repeated name on one side must not consume two names on the other. The
	// one-use rule is the same rule the import applies: two occurrences are two
	// people.
	agreement := CompareCarrierTexts(
		[]string{"محمد بن علي", "محمد بن علي"},
		[]string{"محمد بن علي"},
		0.8,
	)
	if agreement.Matched != 1 {
		t.Fatalf("matched = %d, want 1", agreement.Matched)
	}
	if len(agreement.LeftOnly) != 1 {
		t.Fatalf("left-only = %v, want one unpaired occurrence", agreement.LeftOnly)
	}
}

func TestCompareCarrierTextsIgnoresEmptyReadings(t *testing.T) {
	agreement := CompareCarrierTexts(
		[]string{"محمد بن علي", "   "},
		[]string{"محمد بن علي", ""},
		0.8,
	)
	if agreement.LeftReadings != 1 || agreement.RightReadings != 1 {
		t.Fatalf("readings = %d/%d, want 1/1", agreement.LeftReadings, agreement.RightReadings)
	}
	if agreement.Agreement != 1 {
		t.Fatalf("agreement = %v, want 1", agreement.Agreement)
	}
}

func TestCompareCarrierTextsWithNoReadingsIsNotAgreement(t *testing.T) {
	agreement := CompareCarrierTexts(nil, nil, 0.8)
	if agreement.Agreement != 0 || agreement.Matched != 0 {
		t.Fatalf("empty readings = %+v, want zero agreement", agreement)
	}
}
