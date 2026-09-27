package dates

import "testing"

// The three cases the schema can hold, and the vocabulary every surface must
// share. A date column with no bound, with one bound and with two bounds is the
// whole input space of this package; the test names all of it, because the
// disagreement this prevents is a surface rendering the same row three ways.
//
// The rendered years are the proleptic-Gregorian years the ISO column holds, with
// no era suffix. That is the point of the decision: nothing here converts, so
// nothing here can claim a calendar the record does not carry.
func TestFormatRangeCoversEveryShapeTheSchemaHolds(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want string
	}{
		{name: "a closed range", from: "1120-01-01", to: "1185-12-31", want: "1120 — 1185"},
		{name: "one year on both sides is one year", from: "1120-01-01", to: "1120-12-31", want: "1120"},
		{name: "an open start", from: "", to: "1185-12-31", want: "حتى 1185"},
		{name: "an open end", from: "1120-01-01", to: "", want: "من 1120"},
		{name: "no bound at all", from: "", to: "", want: Unknown},
		{name: "a bare year rather than a date", from: "1120", to: "1185", want: "1120 — 1185"},
		{name: "whitespace is not a bound", from: "  ", to: "1185-12-31", want: "حتى 1185"},
	}
	for _, testCase := range cases {
		if got := FormatRange(testCase.from, testCase.to); got != testCase.want {
			t.Errorf("%s: FormatRange(%q, %q) = %q, want %q", testCase.name, testCase.from, testCase.to, got, testCase.want)
		}
	}
}

// A range the record grades as an estimate must not read like one it grades as
// exact, and a record that states nothing must not be decorated. The certainty
// column is the record's own words about itself, so it is the only thing allowed
// next to the number.
func TestFormatRangeWithCertaintyNamesOnlyTheEstimates(t *testing.T) {
	// A record with no bound is not decorated: the certainty grades a range that is
	// not there, and "غير محددة" already says everything that is known.
	if got := FormatRangeWithCertainty("", "", "approximate"); got != Unknown {
		t.Errorf("an unbounded row with a certainty rendered %q, want %q", got, Unknown)
	}
	cases := []struct {
		certainty string
		want      string
	}{
		{certainty: "", want: "1120 — 1185"},
		{certainty: "precise", want: "1120 — 1185"},
		{certainty: "approximate", want: "1120 — 1185 (تقديرية)"},
		{certainty: "uncertain", want: "1120 — 1185 (غير مؤكدة)"},
		{certainty: "APPROXIMATE", want: "1120 — 1185 (تقديرية)"},
	}
	for _, testCase := range cases {
		if got := FormatRangeWithCertainty("1120-01-01", "1185-12-31", testCase.certainty); got != testCase.want {
			t.Errorf("certainty %q: got %q, want %q", testCase.certainty, got, testCase.want)
		}
	}
}

// A period a human wrote is returned verbatim, and a missing one is the same word
// as a missing bound rather than an empty cell. Converting a recorded sentence is
// the failure this whole decision is about, so the test pins the pass-through.
func TestRecordedReturnsTheAuthorsOwnWords(t *testing.T) {
	if got := Recorded("قبل ١١٥٠هـ"); got != "قبل ١١٥٠هـ" {
		t.Errorf("recorded period = %q, want it verbatim", got)
	}
	if got := Recorded("القرن الثاني عشر"); got != "القرن الثاني عشر" {
		t.Errorf("recorded period = %q, want it verbatim", got)
	}
	if got := Recorded("   "); got != Unknown {
		t.Errorf("blank recorded period = %q, want %q", got, Unknown)
	}
}

// The helper reads a year out of an ISO date rather than parsing one, so the
// shapes it can be handed are worth pinning: a full date, a bare year, and a
// value that is neither.
func TestYearReadsTheFirstFourDigitsOfAnISODate(t *testing.T) {
	cases := map[string]string{"1120-01-01": "1120", "1120": "1120", "112": "112", "من قرن": "من قرن"}
	for input, want := range cases {
		if got := year(input); got != want {
			t.Errorf("year(%q) = %q, want %q", input, got, want)
		}
	}
}
