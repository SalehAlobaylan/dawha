package geography

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
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
	// ActorID is the reader the map is rendered for. The map is a public read path,
	// so an empty actor is the anonymous reader and sees only published rows; the
	// field exists so a research-only place is still reachable by the role that
	// wrote it, on the same map, instead of only through the dictionary.
	ActorID  string
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
	policy, err := visibility.Load(ctx, s.Pool, input.ActorID)
	if err != nil {
		return MapResponse{}, err
	}
	features, err := s.placeFeatures(ctx, policy)
	if err != nil {
		return MapResponse{}, err
	}
	associations, err := s.associationFeatures(ctx, policy)
	if err != nil {
		return MapResponse{}, err
	}
	migrations, err := s.migrationFeatures(ctx, policy)
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

// GetPlace renders the map around one place. A research-only place answers exactly
// like a place that does not exist, so the endpoint cannot be used to discover one
// or to read its geometry through the map.
func (s *Service) GetPlace(ctx context.Context, placeID string, input MapInput) (MapResponse, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(placeID))
	if err != nil {
		return MapResponse{}, ErrNotFound
	}
	input.PlaceID = parsed.String()
	policy, err := visibility.Load(ctx, s.Pool, input.ActorID)
	if err != nil {
		return MapResponse{}, err
	}
	access, err := policy.Reference(ctx, s.Pool, "place", parsed)
	if err != nil {
		return MapResponse{}, err
	}
	if !access.Allowed() {
		return MapResponse{}, ErrNotFound
	}
	return s.List(ctx, input)
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

// placeFeatures lists the places on the map. A place carries its own visibility
// since db/migrations/0039_reference_visibility.sql, so a research-only place is
// off the anonymous map and on the map of the role that wrote it.
func (s *Service) placeFeatures(ctx context.Context, policy visibility.Policy) ([]Feature, error) {
	params := visibility.NewParams()
	predicate := policy.ReferencePredicate(params, "place", "p.id")
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, 'place', p.id, p.canonical_name_ar, NULL::text, NULL::uuid, NULL::text,
		       'place', 'documented', NULL::text, NULL::date, NULL::date, NULL::uuid, NULL::text,
		       NULL::uuid, NULL::text, NULL::text, ST_X(p.geometry), ST_Y(p.geometry), NULL::float8, NULL::float8, NULL::float8, NULL::float8
		FROM places p
		WHERE p.geometry IS NOT NULL AND `+predicate+`
		ORDER BY p.canonical_name_ar
	`, params.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

// associationFeatures lists the associations drawn on the map.
//
// Every endpoint is scoped by the policy that governs it, and an association whose
// endpoint the reader may not see is dropped rather than drawn with a blank subject.
// That is the same rule internal/dictionary already applies to the identical rows -
// a hidden person's places contribute nothing to a page, and a hidden person
// contributes nothing to a place's people - and it is the only one of the two
// choices that cannot disclose anything: the feature's subject *is* the entity, so a
// row whose entity is hidden still asserts that the platform holds a geographic
// record about that entity, and an id or a "somebody" marker is a disclosure with
// the name removed. Dropping it means the public map says nothing at all about a
// person the policy says is not public, and a privileged actor still sees the row.
func (s *Service) associationFeatures(ctx context.Context, policy visibility.Policy) ([]Feature, error) {
	params := visibility.NewParams()
	placePredicate := policy.ReferencePredicate(params, "place", "p.id")
	familyPredicate := policy.ReferencePredicate(params, "family", "ef.id")
	tribePredicate := policy.ReferencePredicate(params, "tribe", "et.id")
	branchPredicate := policy.ReferencePredicate(params, "branch", "eb.id")
	// The person predicate is asked about the joined person row, so the fallback to
	// ga.entity_id::text can only be reached for a person the reader may see, and
	// only for an endpoint that is not a person at all.
	personPredicate := policy.PersonPredicate(params, "ep.id")
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
		LEFT JOIN branches eb ON ga.entity_type = 'branch' AND eb.id = ga.entity_id
		LEFT JOIN sources s ON s.id = ga.source_id
		LEFT JOIN spatial_evidence se ON se.geographic_association_id = ga.id
		WHERE p.geometry IS NOT NULL AND `+placePredicate+`
		  AND (ga.entity_type <> 'person' OR `+personPredicate+`)
		  AND (ga.entity_type <> 'family' OR `+familyPredicate+`)
		  AND (ga.entity_type <> 'tribe' OR `+tribePredicate+`)
		  AND (ga.entity_type <> 'branch' OR `+branchPredicate+`)
		  AND (s.id IS NULL OR s.visibility = 'public')
		ORDER BY ga.time_from NULLS LAST, ga.created_at DESC
	`, params.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

// migrationFeatures lists the migration arrows on the map. Both endpoints are places
// and the subject is an entity, so each is scoped by the policy that governs it and
// an arrow whose subject the reader may not see is dropped on the same terms as an
// association. The fallback to m.subject_id::text is therefore only reachable for a
// visible subject.
func (s *Service) migrationFeatures(ctx context.Context, policy visibility.Policy) ([]Feature, error) {
	params := visibility.NewParams()
	fromPredicate := policy.ReferencePredicate(params, "place", "pf.id")
	toPredicate := policy.ReferencePredicate(params, "place", "pt.id")
	subjectPerson := policy.PersonPredicate(params, "ep.id")
	subjectFamily := policy.ReferencePredicate(params, "family", "ef.id")
	subjectTribe := policy.ReferencePredicate(params, "tribe", "et.id")
	subjectBranch := policy.ReferencePredicate(params, "branch", "eb.id")
	rows, err := s.Pool.Query(ctx, `
		SELECT m.id, 'migration', COALESCE(m.to_place_id, m.from_place_id), COALESCE(pt.canonical_name_ar, pf.canonical_name_ar),
		       m.subject_type, m.subject_id, COALESCE(ep.canonical_name_ar, m.subject_id::text), 'migration', m.status, m.certainty,
		       m.time_from, m.time_to, m.source_id, s.title_ar, se.id, se.evidence_text_ar, se.review_status,
		       ST_X(COALESCE(pt.geometry, pf.geometry)), ST_Y(COALESCE(pt.geometry, pf.geometry)), ST_X(pf.geometry), ST_Y(pf.geometry), ST_X(pt.geometry), ST_Y(pt.geometry)
		FROM migration_events m
		LEFT JOIN places pf ON pf.id = m.from_place_id
		LEFT JOIN places pt ON pt.id = m.to_place_id
		LEFT JOIN people ep ON m.subject_type = 'person' AND ep.id = m.subject_id
		LEFT JOIN families ef ON m.subject_type = 'family' AND ef.id = m.subject_id
		LEFT JOIN tribes et ON m.subject_type = 'tribe' AND et.id = m.subject_id
		LEFT JOIN branches eb ON m.subject_type = 'branch' AND eb.id = m.subject_id
		LEFT JOIN sources s ON s.id = m.source_id
		LEFT JOIN spatial_evidence se ON se.migration_event_id = m.id
		WHERE (pf.geometry IS NOT NULL OR pt.geometry IS NOT NULL)
		  AND (pf.id IS NULL OR `+fromPredicate+`)
		  AND (pt.id IS NULL OR `+toPredicate+`)
		  AND (m.subject_type <> 'person' OR `+subjectPerson+`)
		  AND (m.subject_type <> 'family' OR `+subjectFamily+`)
		  AND (m.subject_type <> 'tribe' OR `+subjectTribe+`)
		  AND (m.subject_type <> 'branch' OR `+subjectBranch+`)
		  AND (s.id IS NULL OR s.visibility = 'public')
		ORDER BY m.time_from NULLS LAST, m.created_at DESC
	`, params.Args()...)
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
		WHERE r.geometry IS NOT NULL AND (s.id IS NULL OR s.visibility = 'public')
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
	input.ActorID = strings.TrimSpace(input.ActorID)
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
