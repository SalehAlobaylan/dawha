package regionreview

// names.go is the workbench's choosing surface (plan 035): the dictionary's
// own tribal names, served bounded, so a reviewer picks a name instead of
// retyping one. The machine proposes, the human reads — the picked value is
// still a plain read the reviewer confirms, never a written reading.

import (
	"context"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
)

// NameSuggestion is one name the choosing surface offers: the value as the
// dictionary spells it, the normalized form the search matches on, and the
// kind of row it came from.
type NameSuggestion struct {
	ValueAr      string `json:"valueAr"`
	NormalizedAr string `json:"normalizedAr"`
	Kind         string `json:"kind"` // canonical | alias
}

// NameSuggestionsResponse is the bounded result of one choosing read.
type NameSuggestionsResponse struct {
	Suggestions []NameSuggestion `json:"suggestions"`
	Limit       int              `json:"limit"`
	Truncated   bool             `json:"truncated"`
}

// NameSuggestionsLimit bounds one choosing read: a page the reviewer can
// actually see, and that names when it cut.
const NameSuggestionsLimit = 12

// NameSuggestions returns the dictionary's own tribal names matching the
// query, bounded, canonical rows before alias rows. The query is normalized
// the same way the stored columns are, so a typed prefix finds its rows; the
// lane never writes a reading itself — a chip fill is a read the reviewer
// confirms.
func (s *Service) NameSuggestions(ctx context.Context, actorID, query string) (NameSuggestionsResponse, error) {
	allowed := false
	var err error
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return NameSuggestionsResponse{}, err
	}
	if !allowed {
		return NameSuggestionsResponse{}, ErrForbidden
	}

	normalized := identity.NormalizeArabicName(strings.TrimSpace(query))
	response := NameSuggestionsResponse{Limit: NameSuggestionsLimit}
	// An empty query is the browse mode: "%%" matches everything, so the
	// choosing surface shows the dictionary's own first page — never empty.
	pattern := "%" + normalized + "%"
	rows, err := s.Pool.Query(ctx, `
		SELECT value_ar, normalized_ar, kind FROM (
		  SELECT c.canonical_name_ar AS value_ar, c.normalized_name_ar AS normalized_ar, 'canonical' AS kind
		    FROM tribes c
		   WHERE c.normalized_name_ar LIKE $1
		  UNION ALL
		  SELECT a.value_ar, a.normalized_value_ar, 'alias'
		    FROM tribe_aliases a
		   WHERE a.normalized_value_ar LIKE $1
		) variants
		ORDER BY kind, value_ar
		LIMIT $2`, pattern, NameSuggestionsLimit+1)
	if err != nil {
		return NameSuggestionsResponse{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var suggestion NameSuggestion
		var kind string
		if err := rows.Scan(&suggestion.ValueAr, &suggestion.NormalizedAr, &kind); err != nil {
			return NameSuggestionsResponse{}, err
		}
		suggestion.Kind = kind
		response.Suggestions = append(response.Suggestions, suggestion)
	}
	if err := rows.Err(); err != nil {
		return NameSuggestionsResponse{}, err
	}
	if len(response.Suggestions) > NameSuggestionsLimit {
		response.Suggestions = response.Suggestions[:NameSuggestionsLimit]
		response.Truncated = true
	}
	return response, nil
}
