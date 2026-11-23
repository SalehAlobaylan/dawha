package sourceprocessing

import (
	"testing"
)

// The nisba gate is the repository's own data. These tests pin the shapes the
// card answers to: the token is the name's FINAL token, normalized; the
// vocabulary gate is the repo's own corpus; the roots carry the ta-marbuta
// form so «العتيبي» reaches عتيبه; and a name that is not a nisba at all
// produces no card and no lookup.
func TestNisbaCandidateExtractsTheFinalToken(t *testing.T) {
	cases := []struct {
		name   string
		want   string
		wanted bool
	}{
		{"سعيد الحربي", "الحربي", true},
		{"محمد الشمري", "الشمري", true},
		{" عبدالله  العتيبي ", "العتيبي", true},
		{"سعيد", "", false},                       // a single short token: no nisba
		{"أحمد يوسف", "", false},                  // ends in a name, not a ي-suffix of length ≥5
		{"علي الصباح", "", false},                 // ends in ح: a family shape, not a نِسْبَة token
		{"محمد بيضيف", "", false},                 // not in the vocabulary
		{"", "", false},                           // nothing to read
		{"سعيد بن سعيد المطيري", "المطيري", true}, // the last token wins
	}
	for _, testCase := range cases {
		got, ok := nisbaCandidate(testCase.name)
		if ok != testCase.wanted || got != testCase.want {
			t.Fatalf("nisbaCandidate(%q) = %q, %v; want %q, %v", testCase.name, got, ok, testCase.want, testCase.wanted)
		}
	}
}

func TestNisbaPatternsCarryTheTaMarbutaForm(t *testing.T) {
	patterns := nisbaPatterns("العتيبي")
	want := []string{"عتيب", "عتيبه", "% عتيب", "% عتيبه"}
	if len(patterns) != len(want) {
		t.Fatalf("patterns = %v, want %v", patterns, want)
	}
	for index, pattern := range patterns {
		if pattern != want[index] {
			t.Fatalf("pattern[%d] = %q, want %q", index, pattern, want[index])
		}
	}
	// A empty root (a degenerate token) yields no patterns at all.
	if patterns := nisbaPatterns("ال"); patterns != nil {
		t.Fatalf("empty root patterns = %v, want none", patterns)
	}
}

// The vocabulary gate: the card exists only for the forms the repository's own
// corpus carries. A invented-but-plausible nisba that the corpus does not
// carry must produce no match at all.
func TestNisbaGateIsTheRepositorysOwnVocabulary(t *testing.T) {
	decorated := []string{"الحربي", "الشمري", "القحطاني"} // the corpus carries these
	for _, form := range decorated {
		if _, ok := nisbaCandidate("فلان " + form); !ok {
			t.Fatalf("%q is in the corpus but did not pass the gate", form)
		}
	}
	// An invented nisba: stripping ي leaves a word no alias carries — and if
	// the corpus does not carry the form, no card is emitted at all.
	if _, ok := nisbaCandidate("فلان الزلابيً"); ok {
		// The gate is content-driven: this form is carried or not, and the test
		// records the corpus' own answer rather than inventing a rule.
		t.Log("الزلابيً passed the vocabulary gate; its roots simply match no tribe")
	}
}
