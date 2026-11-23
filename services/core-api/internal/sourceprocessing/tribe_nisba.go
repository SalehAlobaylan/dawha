// كُنية القبيلة — the nisba hook (plan 031-F).
//
// A reviewed name often ends in a نِسْبَة: «سعيد الحربي» carries حرب. When the
// machine's reading ends in a nisba form that (a) the app's own common-name
// vocabulary carries and (b) resolves to one of the dictionary's tribes, the
// review context carries a tribe-suggestion card: a LINK into the dossier,
// styled like the review screen's other candidate cards, and never an
// assertion of the person's membership — the card writes nothing, decides
// nothing, and its decision ledger is unchanged by design (the acceptance
// criterion is literally "decision ledger unchanged schema": no migration
// belongs to this phase).
package sourceprocessing

import (
	"context"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/jackc/pgx/v5"
)

// TribalNisbaSuggestion is the كُنية card: the matched token as the reading
// carries it, and the tribe the dictionary holds behind it.
type TribalNisbaSuggestion struct {
	TribeID   string `json:"tribeId"`
	TribeName string `json:"tribeName"`
	NisbaForm string `json:"nisbaForm"`
}

// nisbaCandidate extracts the reviewed name's nisba candidate: the final
// token, normalized, gated by the app's own common-name vocabulary. Returns
// false for names whose ending is not a vocabulary nisba — the gate is the
// repository's data, not an invented morphology rule.
func nisbaCandidate(name string) (string, bool) {
	normalized := identity.NormalizeArabicName(strings.TrimSpace(name))
	tokens := strings.Fields(normalized)
	if len(tokens) == 0 {
		return "", false
	}
	last := tokens[len(tokens)-1]
	// A نِسْبَة ends in ي; anything shorter than five letters is a name's own
	// ending (علي, يوسف), not a tribal attribution.
	if !strings.HasSuffix(last, "ي") || len([]rune(last)) < 5 {
		return "", false
	}
	for _, entry := range nameCandidateVocabulary {
		runes := []rune(identity.NormalizeArabicName(entry))
		if len(runes) > 0 && string(runes) == last {
			return last, true
		}
	}
	return "", false
}

// nisbaPatterns builds the lookup patterns for a nisba token: the root with
// the definite article and the final ي stripped, and its ta-marbuta form —
// «العتيبي» matches عتيبة (normalized عتيبه) through the second pattern. Both
// bare and space-prefixed variants, so «حرب» matches a canonical name and
// «قبيله حرب»/«بني حرب» match as an alias's final token.
func nisbaPatterns(last string) []string {
	root := strings.TrimSuffix(last, "ي")
	root = strings.TrimPrefix(root, "ال")
	if root == "" {
		return nil
	}
	withTa := root + "ه"
	return []string{root, withTa, "% " + root, "% " + withTa}
}

// tribalNisbaSuggestion resolves one reviewed name's كُنية card against the
// dictionary's tribes and aliases. It is a READ: no row is written anywhere,
// and the returned pointer means only what the screen says it means.
func (s *Service) tribalNisbaSuggestion(ctx context.Context, reviewedName string) (*TribalNisbaSuggestion, error) {
	if s.Pool == nil {
		return nil, nil
	}
	last, ok := nisbaCandidate(reviewedName)
	if !ok {
		return nil, nil
	}
	patterns := nisbaPatterns(last)
	if len(patterns) == 0 {
		return nil, nil
	}
	var tribeID, tribeName string
	err := s.Pool.QueryRow(ctx, `
		SELECT t.id::text, t.canonical_name_ar
		FROM tribes t
		WHERE t.normalized_name_ar = ANY($1::text[])
		   OR EXISTS (SELECT 1 FROM tribe_aliases ta WHERE ta.tribe_id = t.id
		              AND (ta.normalized_value_ar = ANY($1::text[])
		                   OR ta.normalized_value_ar LIKE ANY($1::text[])))
		ORDER BY t.canonical_name_ar
		LIMIT 1`, patterns).Scan(&tribeID, &tribeName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &TribalNisbaSuggestion{TribeID: tribeID, TribeName: tribeName, NisbaForm: last}, nil
}
