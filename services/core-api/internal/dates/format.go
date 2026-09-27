// Package dates holds the one date-display contract this repository has.
//
// There is no calendar conversion here, and that is the decision rather than an
// omission. `IMPLEMENTATION_PLAN.md:947` asks for "Hijri or Gregorian display
// support where practical", and the honest answer for V1 is that neither is
// available to convert FROM: every temporal column in the schema is a PostgreSQL
// `date` with no calendar, no era and no per-record calendar column, and
// `migration_events` carries its own `certainty` in ('precise', 'approximate',
// 'uncertain'). A conversion applied to such a column would not render the
// record, it would assert a calendar the record never claimed, at the
// presentation layer, where no test could see it and no reviewer could refute
// it. The reasoning is written out in docs/phase-status.md under Phase 10.
//
// What this package does instead is make every surface say the same thing about
// the three cases the schema can actually hold:
//
//   - a range with both bounds: "1120 — 1185"
//   - an open bound: "من 1120" or "حتى 1185"
//   - no bound at all: "غير محددة"
//
// and, when the record grades its own certainty, say that too - a range the
// record calls approximate must not read like one it calls precise. One
// vocabulary, in one place, is what "the surfaces agree" means here: the tree
// node, the map feature and the period filter are three readers of one rule
// rather than three implementations of it.
package dates

import "strings"

// Unknown is what a record with no usable bound is rendered as. It is a word
// rather than a dash on purpose: a dash reads as "the value is empty", and this
// is a statement about what the record does not know.
const Unknown = "غير محددة"

// Certainty labels. `precise` and an absent certainty both render as nothing,
// because naming the default on every row is noise; the other two are named
// because they are the difference between a date and an estimate.
const (
	certaintyApproximate = "تقديرية"
	certaintyUncertain   = "غير مؤكدة"
)

// FormatRange renders two optional ISO dates as the product renders a period
// everywhere else. Either bound may be empty; the result is the same vocabulary
// in all three cases, so a reader who has seen one surface has seen them all.
//
// The year is taken from the ISO text rather than parsed into a time, because
// the stored value is a date with no calendar attached: reading the first four
// digits states exactly what the column holds and nothing more.
func FormatRange(from, to string) string {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	switch {
	case from == "" && to == "":
		return Unknown
	case from == "":
		return "حتى " + year(to)
	case to == "":
		return "من " + year(from)
	case year(from) == year(to):
		return year(from)
	default:
		return year(from) + " — " + year(to)
	}
}

// FormatRangeWithCertainty is FormatRange plus the record's own grading of it.
// A certainty the record does not state, or states as precise, adds nothing: the
// point is to mark the estimates, not to mark everything. A record with no bound
// is not decorated either - grading the precision of a range that is not there
// is noise, and "غير محددة" is already the whole of what is known.
func FormatRangeWithCertainty(from, to, certainty string) string {
	rendered := FormatRange(from, to)
	if rendered == Unknown {
		return rendered
	}
	switch strings.ToLower(strings.TrimSpace(certainty)) {
	case "approximate":
		return rendered + " (" + certaintyApproximate + ")"
	case "uncertain":
		return rendered + " (" + certaintyUncertain + ")"
	default:
		return rendered
	}
}

// Recorded renders a period a human wrote down rather than one the schema
// holds - "قبل ١١٥٠هـ", "القرن الثاني عشر". It is returned verbatim, and
// labelled as recorded text, because it is the one case where a calendar
// abbreviation in the string is the author's claim rather than the platform's.
// Converting it would be converting a sentence.
func Recorded(period string) string {
	period = strings.TrimSpace(period)
	if period == "" {
		return Unknown
	}
	return period
}

// year is the first four characters of an ISO date, or the whole value when it
// is already a bare year.
func year(value string) string {
	if len(value) >= 4 && isDigits(value[:4]) {
		return value[:4]
	}
	return value
}

func isDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return len(value) > 0
}
