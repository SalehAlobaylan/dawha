package geography

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type OccurrenceInput struct {
	Query, ID, TribeID, After string
	From, To                  *int
	IncludeUndated            bool
}

type SourceOccurrence struct {
	ID              string          `json:"id"`
	SourceID        string          `json:"sourceId"`
	ItemKey         string          `json:"itemKey,omitempty"`
	SourceSelector  json.RawMessage `json:"sourceSelector,omitempty"`
	ScanSHA256      string          `json:"scanSha256,omitempty"`
	TitleAR         string          `json:"titleAr"`
	Original        string          `json:"original"`
	DisplayAR       string          `json:"displayAr"`
	Language        string          `json:"language"`
	Method          string          `json:"method"`
	LocatorAR       string          `json:"locatorAr"`
	URL             string          `json:"url,omitempty"`
	CreditAR        string          `json:"creditAr,omitempty"`
	RightsAR        string          `json:"rightsAr,omitempty"`
	PeriodAR        string          `json:"periodAr,omitempty"`
	PublicationYear *int            `json:"publicationYear,omitempty"`
	DepictedFrom    *int            `json:"depictedFrom,omitempty"`
	DepictedTo      *int            `json:"depictedTo,omitempty"`
	ExcerptAR       string          `json:"excerptAr,omitempty"`
	NoteAR          string          `json:"noteAr,omitempty"`
	PlaceID         string          `json:"placeId,omitempty"`
	BranchID        string          `json:"branchId,omitempty"`
	BranchName      string          `json:"branchName,omitempty"`
	TribeID         string          `json:"tribeId,omitempty"`
	PlateKey        string          `json:"plateKey,omitempty"`
	Caption         string          `json:"caption,omitempty"`
}

type OccurrencePage struct {
	Occurrences []SourceOccurrence `json:"occurrences"`
	Next        string             `json:"next,omitempty"`
}

func (s *Service) SourceOccurrences(ctx context.Context, actorID string, input OccurrenceInput) (OccurrencePage, error) {
	const limit = 20
	result := OccurrencePage{Occurrences: []SourceOccurrence{}}
	if s.Pool == nil {
		return result, ErrDatabaseUnavailable
	}
	input.Query = strings.TrimSpace(input.Query)
	if utf8.RuneCountInString(input.Query) > 120 || (input.From != nil && input.To != nil && *input.From > *input.To) {
		return result, ErrValidation
	}
	for _, id := range []string{input.ID, input.TribeID, input.After} {
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return result, ErrValidation
			}
		}
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return result, err
	}
	params := visibility.NewParams()
	sourcePredicate := policy.SourcePredicate(params, "s.id")
	query := params.Add(input.Query)
	id := params.Add(input.ID)
	tribeID := params.Add(input.TribeID)
	after := params.Add(input.After)
	from := params.Add(input.From)
	to := params.Add(input.To)
	includeUndated := params.Add(input.IncludeUndated)

	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		WITH readable AS (
			SELECT s.* FROM sources s WHERE %s
		), occurrences AS (
			SELECT o.id, s.id AS source_id, i.item_key, o.source_selector, COALESCE(r.scan_sha256, '') AS scan_sha256, s.title_ar, o.original_text, o.display_ar, o.language, o.reading_method,
			       r.locator_ar, r.external_url, r.credit_ar, r.rights_ar, r.period_ar,
			       extract(year FROM s.publication_date_from)::integer AS publication_year,
			       r.depicted_from_year, r.depicted_to_year, COALESCE(p.text_ar, '') AS excerpt_ar, o.note_ar,
			       place.id AS place_id, branch.id AS branch_id, COALESCE(branch.canonical_name_ar, '') AS branch_name,
                   t.id AS tribe_id, '' AS plate_key, '' AS caption
			FROM atlas_source_occurrences o
			JOIN atlas_source_item_revisions r ON r.id = o.item_revision_id
			JOIN atlas_source_items i ON i.id = r.item_id
			JOIN readable s ON s.id = i.source_id
			LEFT JOIN source_passages p ON p.id = o.source_passage_id AND p.source_id = s.id
			LEFT JOIN places place ON place.id = o.place_id AND place.visibility = 'public'
            LEFT JOIN branches branch ON branch.id = o.branch_id AND branch.visibility = 'public'
			LEFT JOIN tribes t ON t.id = o.tribe_id AND t.visibility = 'public'
			WHERE o.published
			UNION ALL
			SELECT c.id, s.id, '', NULL::jsonb, c.scan_sha256, s.title_ar, c.printed_name, COALESCE(rev.name_ar, c.name_ar),
			       COALESCE(NULLIF(s.metadata->>'plate_language', ''), 'en'),
			       CASE WHEN COALESCE(s.metadata->>'plate_language', 'en') = 'ar' THEN 'original' ELSE 'transliteration' END,
			       concat('موضع على اللوحة · ', c.x, '، ', c.y), '',
			       COALESCE(s.author_ar, ''), COALESCE(s.metadata->>'license', ''), '',
			       extract(year FROM s.publication_date_from)::integer,
			       COALESCE(
			           NULLIF(s.metadata->>'depicted_from_year', '')::integer,
			           CASE WHEN s.metadata->>'plate_key' = 'admiralty-1916-tribal' THEN 1916 WHEN s.metadata->>'plate_key' = 'dickson-1914-tribal' THEN 1914 END
			       ),
			       COALESCE(
			           NULLIF(s.metadata->>'depicted_to_year', '')::integer,
			           CASE WHEN s.metadata->>'plate_key' = 'admiralty-1916-tribal' THEN 1916 WHEN s.metadata->>'plate_key' = 'dickson-1914-tribal' THEN 1914 END
			       ), '', '',
                   NULL::uuid, NULL::uuid, '', t.id,
			       CASE WHEN c.scan_sha256 = s.metadata->>'file_sha256'
                        AND s.metadata->'georef'->>'method' IN ('graticule_labels','control_points')
                        AND s.metadata->'georef'->'model'->>'kind' IN ('equidistant_conic', 'affine')
			            AND EXISTS (SELECT 1 FROM source_files f WHERE f.source_id = s.id AND f.checksum_sha256 = c.scan_sha256)
			            THEN %s ELSE '' END,
			       concat(%s, ':', c.x, '-', c.y)
			FROM plate_captions c
			JOIN readable s ON s.id = c.source_id
			LEFT JOIN tribes t ON t.id = c.tribe_id AND t.visibility = 'public'
			LEFT JOIN LATERAL (
			    SELECT name_ar FROM plate_caption_revisions
			    WHERE caption_id = c.id AND published_at IS NOT NULL
			    ORDER BY published_at DESC, revision_order DESC LIMIT 1
			) rev ON true
		), search AS (
			SELECT atlas_occurrence_search_key_v1(%s::text) AS needle
		), matched_tribes AS (
			SELECT o.tribe_id FROM occurrences o CROSS JOIN search
			WHERE o.tribe_id IS NOT NULL AND needle <> '' AND
			      (atlas_occurrence_search_key_v1(o.original_text) = needle OR atlas_occurrence_search_key_v1(o.display_ar) = needle)
			UNION
			SELECT a.tribe_id FROM tribe_aliases a
			JOIN tribes t ON t.id = a.tribe_id AND t.visibility = 'public'
			CROSS JOIN search WHERE needle <> '' AND atlas_occurrence_search_key_v1(a.value_ar) = needle
			UNION
			SELECT t.id FROM tribes t CROSS JOIN search
			WHERE t.visibility = 'public' AND needle <> '' AND atlas_occurrence_search_key_v1(t.canonical_name_ar) = needle
		), candidates AS (
			SELECT o.*, atlas_occurrence_search_key_v1(original_text) AS original_key,
			       atlas_occurrence_search_key_v1(display_ar) AS display_key,
			       needle, m.tribe_id IS NOT NULL AS matched_tribe
			FROM occurrences o CROSS JOIN search LEFT JOIN matched_tribes m ON m.tribe_id = o.tribe_id
			WHERE (NULLIF(%s::text, '') IS NULL OR o.id = NULLIF(%s::text, '')::uuid)
			  AND (NULLIF(%s::text, '') IS NULL OR o.tribe_id = NULLIF(%s::text, '')::uuid)
			  AND ((%s::integer IS NULL AND %s::integer IS NULL)
			       OR (%s::boolean AND o.depicted_from_year IS NULL AND o.depicted_to_year IS NULL)
			       OR ((o.depicted_from_year IS NOT NULL OR o.depicted_to_year IS NOT NULL)
			           AND (%s::integer IS NULL OR COALESCE(o.depicted_to_year, o.depicted_from_year) >= %s::integer)
			           AND (%s::integer IS NULL OR COALESCE(o.depicted_from_year, o.depicted_to_year) <= %s::integer)))
		), ranked AS (
			SELECT *, CASE WHEN original_key = needle THEN 0 WHEN display_key = needle THEN 1
                                  WHEN matched_tribe THEN 2 ELSE 3 END AS rank
			FROM candidates
			WHERE %s::text = '' OR (needle <> '' AND (matched_tribe OR strpos(original_key, needle) > 0 OR strpos(display_key, needle) > 0))
		)
		SELECT id::text, source_id::text, item_key, source_selector, scan_sha256, title_ar, original_text, display_ar, language, reading_method, locator_ar,
		       external_url, credit_ar, rights_ar, period_ar, publication_year, depicted_from_year, depicted_to_year,
		       excerpt_ar, note_ar, COALESCE(place_id::text, ''), COALESCE(branch_id::text, ''), branch_name, COALESCE(tribe_id::text, ''), plate_key, caption
		FROM ranked
		WHERE NULLIF(%s::text, '') IS NULL
		   OR (rank, id) > (SELECT rank, id FROM ranked WHERE id = NULLIF(%s::text, '')::uuid)
		ORDER BY rank, id
		LIMIT %d`, sourcePredicate, PlateKeyExpression, PlateKeyExpression,
		query, id, id, tribeID, tribeID, from, to, includeUndated, from, from, to, to, query, after, after, limit+1), params.Args()...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Occurrences, err = pgx.CollectRows(rows, pgx.RowToStructByPos[SourceOccurrence])
	if err != nil {
		return result, err
	}
	if input.ID != "" && len(result.Occurrences) == 0 {
		return result, ErrNotFound
	}
	if len(result.Occurrences) > limit {
		result.Occurrences = result.Occurrences[:limit]
		result.Next = result.Occurrences[limit-1].ID
	}
	return result, nil
}

type AtlasSourceRelationship struct {
	Status string `json:"status"`
	Kind   string `json:"kind,omitempty"`
}

func (s *Service) AtlasSourceRelationship(ctx context.Context, actorID, firstID, secondID string) (AtlasSourceRelationship, error) {
	if s.Pool == nil {
		return AtlasSourceRelationship{}, ErrDatabaseUnavailable
	}
	first, err := uuid.Parse(firstID)
	if err != nil {
		return AtlasSourceRelationship{}, ErrValidation
	}
	second, err := uuid.Parse(secondID)
	if err != nil {
		return AtlasSourceRelationship{}, ErrValidation
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return AtlasSourceRelationship{}, err
	}
	params := visibility.NewParams()
	a, b := params.Add(first), params.Add(second)
	visible := policy.SourcePredicate(params, "s.id")
	var result AtlasSourceRelationship
	err = s.Pool.QueryRow(ctx, `
		WITH visible AS (SELECT s.id FROM sources s WHERE s.id IN (`+a+`,`+b+`) AND `+visible+`)
		SELECT CASE WHEN `+a+`::uuid = `+b+`::uuid THEN 'same_source' ELSE COALESCE(d.status,'unknown') END,
		       COALESCE(d.dependency_type,'')
		FROM visible first JOIN visible second ON second.id = `+b+`
		LEFT JOIN LATERAL (
		    SELECT status, dependency_type FROM source_dependencies
		    WHERE ((source_id = first.id AND depends_on_source_id = second.id)
		        OR (source_id = second.id AND depends_on_source_id = first.id))
		      AND status IN ('confirmed','needs_review')
		    ORDER BY (status = 'confirmed') DESC,
		             (dependency_type IN ('shared_origin','derived_from')) DESC, id LIMIT 1
		) d ON true WHERE first.id = `+a, params.Args()...).Scan(&result.Status, &result.Kind)
	if err == pgx.ErrNoRows {
		return AtlasSourceRelationship{}, ErrNotFound
	}
	return result, err
}
