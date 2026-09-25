package search

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

var (
	ErrDatabaseUnavailable = errors.New("search database is unavailable")
	ErrValidation          = errors.New("search input is invalid")
	ErrAIUnavailable       = errors.New("semantic embedding service is unavailable")
)

const semanticEmbeddingTimeout = 10 * time.Second

type Input struct {
	Query     string
	Kind      string
	Status    string
	PersonID  string
	PlaceID   string
	SourceID  string
	EntityID  string
	FromYear  int
	ToYear    int
	Limit     int
	Embedding string
}

type Result struct {
	ID               string  `json:"id"`
	Kind             string  `json:"kind"`
	Title            string  `json:"title"`
	Subtitle         string  `json:"subtitle,omitempty"`
	Body             string  `json:"body,omitempty"`
	Status           string  `json:"status,omitempty"`
	Score            float64 `json:"score"`
	Route            string  `json:"route,omitempty"`
	SourceID         string  `json:"sourceId,omitempty"`
	PassageID        string  `json:"passageId,omitempty"`
	StatementID      string  `json:"statementId,omitempty"`
	LocatorAR        string  `json:"locatorAr,omitempty"`
	PageNumber       *int    `json:"pageNumber,omitempty"`
	ReviewStatus     string  `json:"reviewStatus,omitempty"`
	DependencyStatus string  `json:"dependencyStatus,omitempty"`
	MatchKind        string  `json:"matchKind,omitempty"`
	EmbeddingModel   string  `json:"embeddingModel,omitempty"`
}

type Group struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Items []Result `json:"items"`
}

type Response struct {
	Query           string  `json:"query"`
	NormalizedQuery string  `json:"normalizedQuery"`
	Groups          []Group `json:"groups"`
	Total           int     `json:"total"`
	EmbeddingModel  string  `json:"embeddingModel,omitempty"`
}

type EmbeddingProvider interface {
	Embed(context.Context, ai.EmbeddingRequest) (ai.EmbeddingResponse, error)
}

type Service struct {
	Pool *pgxpool.Pool
	AI   EmbeddingProvider
}

type dbExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool, providers ...EmbeddingProvider) *Service {
	var provider EmbeddingProvider
	if len(providers) > 0 {
		provider = providers[0]
	}
	return &Service{Pool: pool, AI: provider}
}

func (s *Service) Search(ctx context.Context, input Input) (Response, error) {
	if err := s.ready(); err != nil {
		return Response{}, err
	}
	input, normalized, vector, err := validateInput(input)
	if err != nil {
		return Response{}, err
	}
	embeddingModel := ""
	if vector != nil {
		embeddingModel = "caller_provided"
	}
	if input.Kind == "semantic" && vector == nil {
		generatedVector, generatedModel, embedErr := s.embedQuery(ctx, normalized)
		if embedErr != nil {
			return Response{}, embedErr
		}
		vector = &generatedVector
		embeddingModel = generatedModel
	}
	groups := make([]Group, 0, 6)
	if input.Kind == "" || input.Kind == "all" || input.Kind == "names" {
		items, err := s.searchNames(ctx, normalized, input)
		if err != nil {
			return Response{}, err
		}
		groups = appendGroup(groups, "names", "الأسماء والكيانات", items)
	}
	if input.Kind == "" || input.Kind == "all" || input.Kind == "sources" {
		items, err := s.searchSources(ctx, normalized, input)
		if err != nil {
			return Response{}, err
		}
		groups = appendGroup(groups, "sources", "المصادر", items)
	}
	if input.Kind == "" || input.Kind == "all" || input.Kind == "claims" {
		items, err := s.searchClaims(ctx, normalized, input)
		if err != nil {
			return Response{}, err
		}
		groups = appendGroup(groups, "claims", "الادعاءات", items)
	}
	if input.Kind == "" || input.Kind == "all" || input.Kind == "passages" {
		items, err := s.searchPassages(ctx, normalized, input)
		if err != nil {
			return Response{}, err
		}
		groups = appendGroup(groups, "passages", "المقاطع", items)
	}
	if input.Kind == "" || input.Kind == "all" || input.Kind == "questions" {
		items, err := s.searchQuestions(ctx, normalized, input)
		if err != nil {
			return Response{}, err
		}
		groups = appendGroup(groups, "questions", "الأسئلة المفتوحة", items)
	}
	if vector != nil && (input.Kind == "" || input.Kind == "all" || input.Kind == "passages" || input.Kind == "semantic") {
		items, err := s.semanticPassages(ctx, *vector, input, embeddingModel)
		if err != nil {
			return Response{}, err
		}
		groups = appendGroup(groups, "semantic", "تشابه دلالي", items)
	}
	total := 0
	for _, group := range groups {
		total += len(group.Items)
	}
	return Response{Query: strings.TrimSpace(input.Query), NormalizedQuery: normalized, Groups: groups, Total: total, EmbeddingModel: embeddingModel}, nil
}

func (s *Service) searchNames(ctx context.Context, query string, input Input) ([]Result, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH results AS (
			SELECT p.id, 'person'::text AS kind, p.canonical_name_ar AS name,
			       COALESCE((SELECT pa.value_ar FROM person_aliases pa WHERE pa.person_id = p.id ORDER BY pa.created_at LIMIT 1), '') AS secondary,
			       p.identity_status AS status,
			       GREATEST(CASE WHEN p.normalized_name_ar = $1 THEN 100 ELSE 0 END,
			                CASE WHEN EXISTS (SELECT 1 FROM person_aliases pa WHERE pa.person_id = p.id AND pa.normalized_value_ar = $1) THEN 90 ELSE 0 END,
			                similarity(p.normalized_name_ar, $1) * 70) AS score
			FROM people p
			WHERE ($2 = '' OR p.id = $2::uuid) AND ($3 = '' OR EXISTS (SELECT 1 FROM geographic_associations ga WHERE ga.entity_type = 'person' AND ga.entity_id = p.id AND ga.place_id = $3::uuid))
			UNION ALL
			SELECT f.id, 'family', f.canonical_name_ar, COALESCE((SELECT fa.value_ar FROM family_aliases fa WHERE fa.family_id = f.id ORDER BY fa.created_at LIMIT 1), ''), NULL::text,
			       GREATEST(CASE WHEN f.normalized_name_ar = $1 THEN 100 ELSE 0 END, CASE WHEN EXISTS (SELECT 1 FROM family_aliases fa WHERE fa.family_id = f.id AND fa.normalized_value_ar = $1) THEN 90 ELSE 0 END, similarity(f.normalized_name_ar, $1) * 70)
			FROM families f WHERE ($2 = '' OR f.id = $2::uuid)
			UNION ALL
			SELECT t.id, 'tribe', t.canonical_name_ar, COALESCE((SELECT ta.value_ar FROM tribe_aliases ta WHERE ta.tribe_id = t.id ORDER BY ta.created_at LIMIT 1), ''), NULL::text,
			       GREATEST(CASE WHEN t.normalized_name_ar = $1 THEN 100 ELSE 0 END, CASE WHEN EXISTS (SELECT 1 FROM tribe_aliases ta WHERE ta.tribe_id = t.id AND ta.normalized_value_ar = $1) THEN 90 ELSE 0 END, similarity(t.normalized_name_ar, $1) * 70)
			FROM tribes t WHERE ($2 = '' OR t.id = $2::uuid)
			UNION ALL
			SELECT b.id, 'branch', b.canonical_name_ar, f.canonical_name_ar, NULL::text,
			       GREATEST(CASE WHEN b.normalized_name_ar = $1 THEN 100 ELSE 0 END, similarity(b.normalized_name_ar, $1) * 70)
			FROM branches b JOIN families f ON f.id = b.family_id WHERE ($2 = '' OR b.id = $2::uuid)
			UNION ALL
			SELECT p.id, 'place', p.canonical_name_ar, COALESCE((SELECT hp.name_ar FROM historical_place_names hp WHERE hp.place_id = p.id ORDER BY hp.created_at LIMIT 1), ''), p.place_type,
			       GREATEST(CASE WHEN p.normalized_name_ar = $1 THEN 100 ELSE 0 END, CASE WHEN EXISTS (SELECT 1 FROM historical_place_names hp WHERE hp.place_id = p.id AND hp.name_ar ILIKE '%' || $1 || '%') THEN 85 ELSE 0 END, similarity(p.normalized_name_ar, $1) * 70)
			FROM places p WHERE ($2 = '' OR p.id = $2::uuid) AND ($3 = '' OR p.id = $3::uuid)
		)
		SELECT id, kind, name, secondary, status, score FROM results WHERE score > 0 ORDER BY score DESC, name LIMIT $4
	`, query, input.EntityID, input.PlaceID, input.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Result, 0)
	for rows.Next() {
		var item Result
		var secondary, status pgtype.Text
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &secondary, &status, &item.Score); err != nil {
			return nil, err
		}
		item.Subtitle = textValue(secondary)
		item.Status = textValue(status)
		item.Route = "/dictionary"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) searchSources(ctx context.Context, query string, input Input) ([]Result, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id, s.title_ar, COALESCE(s.author_ar, ''), s.source_type,
		       GREATEST(CASE WHEN s.title_ar = $1 THEN 100 ELSE 0 END, similarity(s.title_ar, $1) * 70, CASE WHEN COALESCE(s.author_ar, '') ILIKE '%' || $1 || '%' THEN 60 ELSE 0 END) AS score
		FROM sources s
		WHERE ($1 = '' OR s.title_ar ILIKE '%' || $1 || '%' OR COALESCE(s.author_ar, '') ILIKE '%' || $1 || '%' OR COALESCE(s.citation_ar, '') ILIKE '%' || $1 || '%')
		  AND s.visibility = 'public'
		  AND ($2 = '' OR s.source_type = $2) AND ($3 = '' OR s.id = $3::uuid)
		ORDER BY score DESC, s.title_ar LIMIT $4
	`, query, input.Status, input.SourceID, input.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Result, 0)
	for rows.Next() {
		var item Result
		var author, status string
		if err := rows.Scan(&item.ID, &item.Title, &author, &status, &item.Score); err != nil {
			return nil, err
		}
		item.Kind = "source"
		item.Subtitle = author
		item.Status = status
		item.Route = "/sources"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) searchClaims(ctx context.Context, query string, input Input) ([]Result, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id, c.predicate, COALESCE(c.notes_ar, ''), c.status,
		       GREATEST(CASE WHEN c.predicate = $1 THEN 100 ELSE 0 END, similarity(c.predicate, $1) * 70, CASE WHEN COALESCE(c.notes_ar, '') ILIKE '%' || $1 || '%' THEN 60 ELSE 0 END) AS score
		FROM claims c
		WHERE ($1 = '' OR c.predicate ILIKE '%' || $1 || '%' OR COALESCE(c.notes_ar, '') ILIKE '%' || $1 || '%' OR EXISTS (SELECT 1 FROM people p WHERE p.id IN (c.subject_id, c.object_id) AND p.canonical_name_ar ILIKE '%' || $1 || '%'))
		  AND ($2 = '' OR c.status = $2)
		  AND ($3 = '' OR c.subject_id = $3::uuid OR c.object_id = $3::uuid)
		  AND ($4 = '' OR c.place_id = $4::uuid)
		  AND ($5 = '' OR EXISTS (SELECT 1 FROM claim_evidence ce WHERE ce.claim_id = c.id AND ce.source_statement_id IN (SELECT ss.id FROM source_statements ss WHERE ss.source_id = $5::uuid)))
		  AND ($7 = 0 OR c.time_from IS NULL OR EXTRACT(YEAR FROM c.time_from) <= $7)
		  AND ($6 = 0 OR c.time_to IS NULL OR EXTRACT(YEAR FROM c.time_to) >= $6)
		  AND NOT EXISTS (
			SELECT 1 FROM claim_evidence ce
			JOIN source_statements ss ON ss.id = ce.source_statement_id
			JOIN sources sx ON sx.id = ss.source_id
			WHERE ce.claim_id = c.id AND sx.visibility = 'private'
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM claim_counter_evidence cce
			JOIN source_statements ss ON ss.id = cce.source_statement_id
			JOIN sources sx ON sx.id = ss.source_id
			WHERE cce.claim_id = c.id AND sx.visibility = 'private'
		  )
		ORDER BY score DESC, c.updated_at DESC LIMIT $8
	`, query, input.Status, input.PersonID, input.PlaceID, input.SourceID, input.FromYear, input.ToYear, input.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Result, 0)
	for rows.Next() {
		var item Result
		var body, status string
		if err := rows.Scan(&item.ID, &item.Title, &body, &status, &item.Score); err != nil {
			return nil, err
		}
		item.Kind = "claim"
		item.Body = body
		item.Status = status
		item.Route = "/research"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) searchPassages(ctx context.Context, query string, input Input) ([]Result, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT sp.id, s.title_ar, sp.text_ar, sp.locator_ar, GREATEST(CASE WHEN sp.normalized_text_ar = $1 THEN 100 ELSE 0 END, similarity(sp.normalized_text_ar, $1) * 80, CASE WHEN sp.normalized_text_ar ILIKE '%' || $1 || '%' THEN 65 ELSE 0 END) AS score
		FROM source_passages sp JOIN sources s ON s.id = sp.source_id
		WHERE ($1 = '' OR sp.normalized_text_ar ILIKE '%' || $1 || '%' OR sp.text_ar ILIKE '%' || $1 || '%')
		  AND s.visibility = 'public'
		  AND ($2 = '' OR sp.source_id = $2::uuid)
		ORDER BY score DESC, sp.created_at DESC LIMIT $3
	`, query, input.SourceID, input.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Result, 0)
	for rows.Next() {
		var item Result
		var locator pgtype.Text
		if err := rows.Scan(&item.ID, &item.Title, &item.Body, &locator, &item.Score); err != nil {
			return nil, err
		}
		item.Kind = "passage"
		item.Subtitle = textValue(locator)
		item.Route = "/sources"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) searchQuestions(ctx context.Context, query string, input Input) ([]Result, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT q.id, q.title_ar, COALESCE(q.description_ar, ''), q.status,
		       GREATEST(CASE WHEN q.title_ar = $1 THEN 100 ELSE 0 END, similarity(q.title_ar, $1) * 70, CASE WHEN COALESCE(q.description_ar, '') ILIKE '%' || $1 || '%' THEN 55 ELSE 0 END) AS score
		FROM open_questions q
		WHERE ($1 = '' OR q.title_ar ILIKE '%' || $1 || '%' OR COALESCE(q.description_ar, '') ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR q.status = $2)
		ORDER BY score DESC, q.updated_at DESC LIMIT $3
	`, query, input.Status, input.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Result, 0)
	for rows.Next() {
		var item Result
		var body, status string
		if err := rows.Scan(&item.ID, &item.Title, &body, &status, &item.Score); err != nil {
			return nil, err
		}
		item.Kind = "question"
		item.Body = body
		item.Status = status
		item.Route = "/questions"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) embedQuery(ctx context.Context, query string) (pgvector.Vector, string, error) {
	if s.AI == nil {
		return pgvector.Vector{}, "", ErrAIUnavailable
	}
	embeddingContext, cancel := context.WithTimeout(ctx, semanticEmbeddingTimeout)
	defer cancel()
	result, err := s.AI.Embed(embeddingContext, ai.EmbeddingRequest{Text: query, Dimensions: 1536})
	if err != nil || result.Dimensions != 1536 || len(result.Embedding) != 1536 || strings.TrimSpace(result.Model) == "" {
		return pgvector.Vector{}, "", ErrAIUnavailable
	}
	values := make([]float32, len(result.Embedding))
	norm := 0.0
	for index, value := range result.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
			return pgvector.Vector{}, "", ErrAIUnavailable
		}
		values[index] = float32(value)
		norm += value * value
	}
	if norm <= 1e-12 {
		return pgvector.Vector{}, "", ErrAIUnavailable
	}
	return pgvector.NewVector(values), result.Model, nil
}

func (s *Service) semanticPassages(ctx context.Context, vector pgvector.Vector, input Input, embeddingModel string) ([]Result, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT sp.id::text, sp.source_id::text, s.title_ar, sp.text_ar,
		       COALESCE(statement.locator_ar, sp.locator_ar), sp.page_number,
		       COALESCE(statement.id::text, ''), COALESCE(statement.review_status, 'unreviewed'),
		       CASE
		         WHEN EXISTS (SELECT 1 FROM source_dependencies d WHERE d.source_id = s.id AND d.status = 'confirmed') THEN 'derived'
		         WHEN EXISTS (SELECT 1 FROM source_dependencies d WHERE d.source_id = s.id AND d.status = 'needs_review') THEN 'likely_dependent'
		         ELSE s.dependency_status
		       END,
		       1 - (sp.embedding <=> $1) AS score
		FROM source_passages sp
		JOIN sources s ON s.id = sp.source_id
		LEFT JOIN LATERAL (
			SELECT ss.id, ss.review_status, ss.locator_ar
			FROM source_statements ss
			WHERE ss.source_passage_id = sp.id AND ss.source_id = s.id
			ORDER BY CASE ss.review_status WHEN 'accepted' THEN 0 WHEN 'needs_review' THEN 1 ELSE 2 END, ss.created_at, ss.id
			LIMIT 1
		) statement ON TRUE
		WHERE sp.embedding IS NOT NULL AND s.visibility = 'public'
		  AND 1 - (sp.embedding <=> $1) > 0
		  AND ($2 = '' OR sp.source_id = $2::uuid)
		  AND ($3 = '' OR EXISTS (
			SELECT 1
			FROM claim_evidence ce
			JOIN claims c ON c.id = ce.claim_id
			WHERE (ce.source_passage_id = sp.id OR ce.source_statement_id IN (SELECT ss.id FROM source_statements ss WHERE ss.source_passage_id = sp.id AND ss.source_id = s.id))
			  AND (ce.source_statement_id IS NULL OR EXISTS (SELECT 1 FROM source_statements linked WHERE linked.id = ce.source_statement_id AND linked.source_id = s.id))
			  AND (c.subject_id = $3::uuid OR c.object_id = $3::uuid)
		  ))
		  AND ($4 = '' OR EXISTS (
			SELECT 1 FROM geographic_associations ga WHERE ga.source_id = s.id AND ga.place_id = $4::uuid
			UNION ALL
			SELECT 1 FROM migration_events me WHERE me.source_id = s.id AND (me.from_place_id = $4::uuid OR me.to_place_id = $4::uuid)
		  ))
		  AND ($6 = 0 OR s.publication_date_from IS NULL OR EXTRACT(YEAR FROM s.publication_date_from) <= $6)
		  AND ($5 = 0 OR s.publication_date_to IS NULL OR EXTRACT(YEAR FROM s.publication_date_to) >= $5)
		ORDER BY sp.embedding <=> $1, sp.id
		LIMIT $7
	`, vector, input.SourceID, input.PersonID, input.PlaceID, input.FromYear, input.ToYear, input.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Result, 0)
	for rows.Next() {
		var item Result
		var locator pgtype.Text
		var pageNumber pgtype.Int4
		if err := rows.Scan(&item.PassageID, &item.SourceID, &item.Title, &item.Body, &locator, &pageNumber, &item.StatementID, &item.ReviewStatus, &item.DependencyStatus, &item.Score); err != nil {
			return nil, err
		}
		if math.IsNaN(item.Score) || math.IsInf(item.Score, 0) {
			return nil, ErrAIUnavailable
		}
		item.ID = item.PassageID
		item.Kind = "passage"
		item.LocatorAR = textValue(locator)
		item.Subtitle = item.LocatorAR
		item.Status = item.ReviewStatus
		item.MatchKind = "semantic"
		item.EmbeddingModel = embeddingModel
		item.Route = "/sources"
		if pageNumber.Valid {
			value := int(pageNumber.Int32)
			item.PageNumber = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func validateInput(input Input) (Input, string, *pgvector.Vector, error) {
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" || len([]rune(input.Query)) > 500 || !validKind(input.Kind) || input.FromYear < 0 || input.ToYear < 0 || (input.FromYear > 0 && input.ToYear > 0 && input.FromYear > input.ToYear) {
		return Input{}, "", nil, ErrValidation
	}
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	for _, value := range []struct{ target *string }{{&input.PersonID}, {&input.PlaceID}, {&input.SourceID}, {&input.EntityID}} {
		*value.target = strings.TrimSpace(*value.target)
		if *value.target != "" {
			if _, err := uuid.Parse(*value.target); err != nil {
				return Input{}, "", nil, ErrValidation
			}
		}
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit < 1 || input.Limit > 50 {
		return Input{}, "", nil, ErrValidation
	}
	normalized := identity.NormalizeArabicName(input.Query)
	if input.Kind == "semantic" && normalized == "" {
		return Input{}, "", nil, ErrValidation
	}
	var vector *pgvector.Vector
	if input.Embedding != "" {
		parsed, err := parseVector(input.Embedding)
		if err != nil {
			return Input{}, "", nil, ErrValidation
		}
		vector = &parsed
	}
	return input, normalized, vector, nil
}

func validKind(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return kind == "" || kind == "all" || kind == "names" || kind == "sources" || kind == "claims" || kind == "passages" || kind == "questions" || kind == "semantic"
}

func parseVector(raw string) (pgvector.Vector, error) {
	var values []float32
	if err := json.Unmarshal([]byte(raw), &values); err != nil || len(values) != 1536 {
		return pgvector.Vector{}, ErrValidation
	}
	norm := 0.0
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value < -1 || value > 1 {
			return pgvector.Vector{}, ErrValidation
		}
		norm += float64(value) * float64(value)
	}
	if norm <= 1e-12 {
		return pgvector.Vector{}, ErrValidation
	}
	return pgvector.NewVector(values), nil
}

func appendGroup(groups []Group, key, label string, items []Result) []Group {
	if len(items) == 0 {
		return groups
	}
	return append(groups, Group{Key: key, Label: label, Items: items})
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
