package geography

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("geography database is unavailable")
	ErrNotFound            = errors.New("geography resource not found")
	ErrValidation          = errors.New("geography input is invalid")
)

type MapInput struct {
	FromYear int
	ToYear   int
	Status   string
	PlaceID  string
}

type Feature struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	PlaceID        string   `json:"placeId,omitempty"`
	PlaceName      string   `json:"placeName,omitempty"`
	EntityType     string   `json:"entityType,omitempty"`
	EntityID       string   `json:"entityId,omitempty"`
	EntityName     string   `json:"entityName,omitempty"`
	RelationType   string   `json:"relationType,omitempty"`
	Status         string   `json:"status"`
	Certainty      string   `json:"certainty,omitempty"`
	TimeFrom       string   `json:"timeFrom,omitempty"`
	TimeTo         string   `json:"timeTo,omitempty"`
	SourceID       string   `json:"sourceId,omitempty"`
	SourceTitle    string   `json:"sourceTitle,omitempty"`
	EvidenceID     string   `json:"evidenceId,omitempty"`
	EvidenceText   string   `json:"evidenceText,omitempty"`
	EvidenceStatus string   `json:"evidenceStatus,omitempty"`
	Longitude      *float64 `json:"longitude,omitempty"`
	Latitude       *float64 `json:"latitude,omitempty"`
	FromLongitude  *float64 `json:"fromLongitude,omitempty"`
	FromLatitude   *float64 `json:"fromLatitude,omitempty"`
	ToLongitude    *float64 `json:"toLongitude,omitempty"`
	ToLatitude     *float64 `json:"toLatitude,omitempty"`
}

type MapResponse struct {
	FromYear int       `json:"fromYear,omitempty"`
	ToYear   int       `json:"toYear,omitempty"`
	Status   string    `json:"status,omitempty"`
	Features []Feature `json:"features"`
}

type Service struct {
	Pool *pgxpool.Pool
}

type dbExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) List(ctx context.Context, input MapInput) (MapResponse, error) {
	if err := s.ready(); err != nil {
		return MapResponse{}, err
	}
	input, err := validateInput(input)
	if err != nil {
		return MapResponse{}, err
	}
	features, err := s.placeFeatures(ctx)
	if err != nil {
		return MapResponse{}, err
	}
	associations, err := s.associationFeatures(ctx)
	if err != nil {
		return MapResponse{}, err
	}
	migrations, err := s.migrationFeatures(ctx)
	if err != nil {
		return MapResponse{}, err
	}
	regions, err := s.regionFeatures(ctx)
	if err != nil {
		return MapResponse{}, err
	}
	features = append(features, associations...)
	features = append(features, migrations...)
	features = append(features, regions...)
	filtered := make([]Feature, 0, len(features))
	for _, feature := range features {
		if matches(feature, input) {
			filtered = append(filtered, feature)
		}
	}
	return MapResponse{FromYear: input.FromYear, ToYear: input.ToYear, Status: input.Status, Features: filtered}, nil
}

func (s *Service) GetPlace(ctx context.Context, placeID string, input MapInput) (MapResponse, error) {
	if _, err := uuid.Parse(strings.TrimSpace(placeID)); err != nil {
		return MapResponse{}, ErrNotFound
	}
	input.PlaceID = strings.TrimSpace(placeID)
	return s.List(ctx, input)
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) placeFeatures(ctx context.Context) ([]Feature, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, 'place', p.id, p.canonical_name_ar, NULL::text, NULL::uuid, NULL::text,
		       'place', 'documented', NULL::text, NULL::date, NULL::date, NULL::uuid, NULL::text,
		       NULL::uuid, NULL::text, NULL::text, ST_X(p.geometry), ST_Y(p.geometry), NULL::float8, NULL::float8, NULL::float8, NULL::float8
		FROM places p
		WHERE p.geometry IS NOT NULL
		ORDER BY p.canonical_name_ar
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

func (s *Service) associationFeatures(ctx context.Context) ([]Feature, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT ga.id, 'association', ga.place_id, p.canonical_name_ar, ga.entity_type, ga.entity_id,
		       COALESCE(ep.canonical_name_ar, ef.canonical_name_ar, et.canonical_name_ar, ga.entity_id::text),
		       ga.relation_type, ga.status, ga.certainty, ga.time_from, ga.time_to, ga.source_id, s.title_ar,
		       se.id, se.evidence_text_ar, se.review_status, ST_X(p.geometry), ST_Y(p.geometry), NULL::float8, NULL::float8, NULL::float8, NULL::float8
		FROM geographic_associations ga
		JOIN places p ON p.id = ga.place_id
		LEFT JOIN people ep ON ga.entity_type = 'person' AND ep.id = ga.entity_id
		LEFT JOIN families ef ON ga.entity_type = 'family' AND ef.id = ga.entity_id
		LEFT JOIN tribes et ON ga.entity_type = 'tribe' AND et.id = ga.entity_id
		LEFT JOIN sources s ON s.id = ga.source_id
		LEFT JOIN spatial_evidence se ON se.geographic_association_id = ga.id
		WHERE p.geometry IS NOT NULL
		ORDER BY ga.time_from NULLS LAST, ga.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

func (s *Service) migrationFeatures(ctx context.Context) ([]Feature, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT m.id, 'migration', COALESCE(m.to_place_id, m.from_place_id), COALESCE(pt.canonical_name_ar, pf.canonical_name_ar),
		       m.subject_type, m.subject_id, COALESCE(ep.canonical_name_ar, m.subject_id::text), 'migration', m.status, m.certainty,
		       m.time_from, m.time_to, m.source_id, s.title_ar, se.id, se.evidence_text_ar, se.review_status,
		       ST_X(COALESCE(pt.geometry, pf.geometry)), ST_Y(COALESCE(pt.geometry, pf.geometry)), ST_X(pf.geometry), ST_Y(pf.geometry), ST_X(pt.geometry), ST_Y(pt.geometry)
		FROM migration_events m
		LEFT JOIN places pf ON pf.id = m.from_place_id
		LEFT JOIN places pt ON pt.id = m.to_place_id
		LEFT JOIN people ep ON m.subject_type = 'person' AND ep.id = m.subject_id
		LEFT JOIN sources s ON s.id = m.source_id
		LEFT JOIN spatial_evidence se ON se.migration_event_id = m.id
		WHERE pf.geometry IS NOT NULL OR pt.geometry IS NOT NULL
		ORDER BY m.time_from NULLS LAST, m.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

func (s *Service) regionFeatures(ctx context.Context) ([]Feature, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, 'region', NULL::uuid, r.name_ar, NULL::text, NULL::uuid, NULL::text, 'historical_region',
		       CASE WHEN r.certainty = 'disputed' THEN 'disputed' ELSE 'interpreted' END, r.certainty, r.valid_from, r.valid_to,
		       r.source_id, s.title_ar, NULL::uuid, NULL::text, NULL::text,
		       ST_X(ST_Centroid(r.geometry)), ST_Y(ST_Centroid(r.geometry)), NULL::float8, NULL::float8, NULL::float8, NULL::float8
		FROM historical_regions r
		LEFT JOIN sources s ON s.id = r.source_id
		WHERE r.geometry IS NOT NULL
		ORDER BY r.name_ar
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

func scanFeatures(rows pgx.Rows) ([]Feature, error) {
	items := make([]Feature, 0)
	for rows.Next() {
		var item Feature
		var placeID, entityID, sourceID, evidenceID pgtype.UUID
		var entityType, entityName, relationType, sourceTitle, evidenceText, evidenceStatus pgtype.Text
		var certainty pgtype.Text
		var timeFrom, timeTo pgtype.Date
		var longitude, latitude, fromLongitude, fromLatitude, toLongitude, toLatitude pgtype.Float8
		if err := rows.Scan(&item.ID, &item.Kind, &placeID, &item.PlaceName, &entityType, &entityID, &entityName, &relationType, &item.Status, &certainty, &timeFrom, &timeTo, &sourceID, &sourceTitle, &evidenceID, &evidenceText, &evidenceStatus, &longitude, &latitude, &fromLongitude, &fromLatitude, &toLongitude, &toLatitude); err != nil {
			return nil, err
		}
		item.PlaceID = uuidString(placeID)
		item.EntityType = textValue(entityType)
		item.EntityID = uuidString(entityID)
		item.EntityName = textValue(entityName)
		item.RelationType = textValue(relationType)
		item.Certainty = textValue(certainty)
		item.TimeFrom = dateValue(timeFrom)
		item.TimeTo = dateValue(timeTo)
		item.SourceID = uuidString(sourceID)
		item.SourceTitle = textValue(sourceTitle)
		item.EvidenceID = uuidString(evidenceID)
		item.EvidenceText = textValue(evidenceText)
		item.EvidenceStatus = textValue(evidenceStatus)
		item.Longitude = floatValue(longitude)
		item.Latitude = floatValue(latitude)
		item.FromLongitude = floatValue(fromLongitude)
		item.FromLatitude = floatValue(fromLatitude)
		item.ToLongitude = floatValue(toLongitude)
		item.ToLatitude = floatValue(toLatitude)
		items = append(items, item)
	}
	return items, rows.Err()
}

func validateInput(input MapInput) (MapInput, error) {
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.FromYear < 0 || input.ToYear < 0 || (input.FromYear > 0 && input.ToYear > 0 && input.FromYear > input.ToYear) || !validStatus(input.Status) {
		return MapInput{}, ErrValidation
	}
	if input.PlaceID != "" {
		if _, err := uuid.Parse(strings.TrimSpace(input.PlaceID)); err != nil {
			return MapInput{}, ErrValidation
		}
		input.PlaceID = strings.TrimSpace(input.PlaceID)
	}
	return input, nil
}

func validStatus(status string) bool {
	return status == "" || status == "documented" || status == "interpreted" || status == "platform_inferred" || status == "disputed" || status == "unresolved"
}

func matches(feature Feature, input MapInput) bool {
	if input.Status != "" && feature.Status != input.Status {
		return false
	}
	if input.PlaceID != "" && feature.PlaceID != input.PlaceID {
		return false
	}
	if input.FromYear == 0 && input.ToYear == 0 {
		return true
	}
	from, to := yearFromDate(feature.TimeFrom), yearFromDate(feature.TimeTo)
	if from == 0 && to == 0 {
		return true
	}
	if input.ToYear > 0 && from > input.ToYear {
		return false
	}
	if input.FromYear > 0 && to > 0 && to < input.FromYear {
		return false
	}
	return true
}

func yearFromDate(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil {
		return 0
	}
	return year
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func dateValue(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func floatValue(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}
