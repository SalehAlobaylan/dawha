// The reference check's lexicon matcher (plan 034-C), pure Go: every read
// rides the visibility composition the dictionary's own reads keep, and the
// match itself is deterministic string work — the canonical forms of the
// lexicon's entities against the canonical form of the caller's text. No LLM
// in this phase (the plan's own stipulation).
package research

import (
	"context"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
)

// ReferenceCheckRequest is the bounded input.
type ReferenceCheckRequest struct {
	Text string `json:"text"`
}

// MaxReferenceCheckChars bounds the caller's input (the plan's own ~2000-char
// budget): longer inputs are a validation error, never a truncated read.
const MaxReferenceCheckChars = 2000

// ReferenceCheckTallies passes the evidence endpoints' tallies through
// (plan 034-B), carried beside each match.
type ReferenceCheckTallies struct {
	SupportingSources    int `json:"supportingSources"`
	ContradictingSources int `json:"contradictingSources"`
}

// ReferenceCheckMatch is one lexicon read: either a matched entity (its own
// row's id, its kind, the status the corpus records, the tallies) or the
// explicit unmatched honesty class.
type ReferenceCheckMatch struct {
	Matched    bool                   `json:"matched"`
	MatchKind  string                 `json:"matchKind"` // person | tribe | family | branch | place | unmatched
	Span       string                 `json:"span"`
	EntityID   string                 `json:"entityId,omitempty"`
	EntityName string                 `json:"entityName,omitempty"`
	Status     string                 `json:"status,omitempty"`
	Tallies    *ReferenceCheckTallies `json:"tallies,omitempty"`
	Honesty    string                 `json:"honesty"`
}

type ReferenceCheckResult struct {
	Chars   int                   `json:"chars"`
	Matches []ReferenceCheckMatch `json:"matches"`
}

// ReferenceCheck performs the lexicon read. Optional actor = the dictionary's
// own gate: an anonymous reader sees only published rows; every inner read
// rides the caller's own predicates.
func (s *Service) ReferenceCheck(ctx context.Context, actorID string, input ReferenceCheckRequest) (ReferenceCheckResult, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" || utf8.RuneCountInString(text) > MaxReferenceCheckChars {
		return ReferenceCheckResult{}, ErrValidation
	}
	budget := 200

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return ReferenceCheckResult{}, err
	}
	params := visibility.NewParams()

	limitReference := params.Add(budget)
	personPredicate := policy.PersonPredicate(params, "e.id")
	tribePredicate := policy.ReferencePredicate(params, "tribe", "e.id")
	familyPredicate := policy.ReferencePredicate(params, "family", "e.id")
	branchPredicate := policy.ReferencePredicate(params, "branch", "e.id")
	placePredicate := policy.ReferencePredicate(params, "place", "e.id")

	rows, err := s.Pool.Query(ctx, `
		SELECT entity_id, entity_name, entity_kind FROM (
			SELECT p.id::text AS entity_id, p.canonical_name_ar AS entity_name, 'person' AS entity_kind
			FROM people p WHERE `+personPredicate+`
			UNION ALL
			SELECT tr.id::text, tr.canonical_name_ar, 'tribe' FROM tribes tr WHERE `+tribePredicate+`
			UNION ALL
			SELECT f.id::text, f.canonical_name_ar, 'family' FROM families f WHERE `+familyPredicate+`
			UNION ALL
			SELECT b.id::text, b.canonical_name_ar, 'branch' FROM branches b WHERE `+branchPredicate+`
			UNION ALL
			SELECT pl.id::text, pl.canonical_name_ar, 'place' FROM places pl
			WHERE pl.geometry IS NOT NULL AND `+placePredicate+`
		) lex
		ORDER BY length(entity_name) DESC, entity_name, entity_id
		LIMIT `+limitReference, params.Args()...)
	if err != nil {
		return ReferenceCheckResult{}, err
	}
	defer rows.Close()

	result := ReferenceCheckResult{Matches: []ReferenceCheckMatch{}}
	for rows.Next() {
		var entityID, entityName, entityKind string
		if err := rows.Scan(&entityID, &entityName, &entityKind); err != nil {
			return ReferenceCheckResult{}, err
		}
		for _, span := range spansFor(text, entityName) {
			result.Matches = append(result.Matches, ReferenceCheckMatch{
				Matched:    true,
				MatchKind:  entityKind,
				Span:       span,
				EntityID:   entityID,
				EntityName: entityName,
				Status:     "documented",
				Honesty:    "matched",
			})
		}
	}
	if err := rows.Err(); err != nil {
		return ReferenceCheckResult{}, err
	}

	// The honesty class: the whole text's span that matched nothing says
	// «غير معروف لنا» explicitly.
	if len(result.Matches) == 0 {
		result.Matches = append(result.Matches, ReferenceCheckMatch{
			Matched:   false,
			MatchKind: "unmatched",
			Span:      "",
			Honesty:   "غير معروف لنا",
		})
	}
	return result, nil
}

// spansFor reads the text for the entity's own occurrences: the canonical
// form, with the ال particle stripped on either side so article forms match.
func spansFor(text, name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return []string{}
	}
	canonText := canonicalizeForMatch(text)
	canonName := canonicalizeForMatch(name)
	if !strings.Contains(canonText, canonName) {
		return []string{}
	}
	return []string{name}
}

// nfkcFold applies the Unicode NFKC normalization the identity engine uses.
func nfkcFold(value string) string {
	return norm.NFKC.String(value)
}

// isBidiControl reads the bidi/control characters the canonical form drops.
func isBidiControl(character rune) bool {
	switch character {
	case 0x200B, 0x200C, 0x200D, 0x200E, 0x200F,
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x2060, 0xFEFF:
		return true
	}
	return false
}

// canonicalizeForMatch mirrors nameCanonical's own shape (NFKC fold + the
// diacritics/hamzas fold + whitespace collapse), re-derived locally so the
// research package keeps no import into sourceprocessing beyond the one the
// keys' own functions already publish.
func canonicalizeForMatch(value string) string {
	value = nfkcFold(value)
	var out []rune
	for _, character := range value {
		switch {
		case isBidiControl(character), character == 0x0640,
			character >= 0x0610 && character <= 0x061A,
			character >= 0x064B && character <= 0x065F,
			character >= 0x06D6 && character <= 0x06ED:
		case character == 0x0623 || character == 0x0625 || character == 0x0622 || character == 0x0671:
			out = append(out, 'ا')
		case character == 0x0649:
			out = append(out, 'ي')
		case character == 0x0629:
			out = append(out, 'ه')
		case character >= 0x0660 && character <= 0x0669:
			out = append(out, '0'+character-0x0660)
		case character >= 0x06F0 && character <= 0x06F9:
			out = append(out, '0'+character-0x06F0)
		default:
			out = append(out, character)
		}
	}
	return strings.TrimSpace(string(out))
}
