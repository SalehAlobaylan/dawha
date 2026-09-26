package geography

import (
	"context"
	"errors"
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

const (
	// MaxMapFeatures bounds one map response. The map used to return every visible
	// row of four tables, so its size grew with the dataset; a bounded response is
	// what lets a caller plan for it, and the response says when it was cut.
	MaxMapFeatures = 5000
	// maxPlaceFeatureRows bounds the geometry read that answers a viewport
	// question. It is deliberately far above MaxMapFeatures so a viewport never
	// truncates a response that is already inside the bound.
	maxPlaceFeatureRows = 20000
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
	// Viewport is the bounding box the caller is drawing. It is a filter the
	// database applies, not a post-hoc crop: a feature outside the box is never
	// built, joined or shipped.
	Viewport *Viewport
	// Limit bounds the response. Zero means MaxMapFeatures.
	Limit int
}

// Viewport is a longitude/latitude bounding box. It is all-or-nothing on purpose: a
// half-specified box is a filter whose meaning depends on which half was sent, and
// a map that guessed the other half would show a different world than the caller
// asked for.
type Viewport struct {
	MinLongitude float64
	MinLatitude  float64
	MaxLongitude float64
	MaxLatitude  float64
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
	// Limit is the bound that was applied, so a caller can tell a short map from a
	// map it asked to be short.
	Limit int `json:"limit,omitempty"`
	// Truncated reports that the map holds more features than Limit allowed. It is
	// the difference between "this is everything" and "this is what fitted", and a
	// map that cannot say which one it is cannot be drawn correctly.
	Truncated bool `json:"truncated"`
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

// mapFilters is the one set of predicates every feature query carries, built once
// from one input.
//
// It exists so the four queries cannot drift: each one asks for the same status,
// place, year and viewport conditions, and a change to the rule is a change in one
// place rather than four. The parameters live on the same visibility.Params the
// policy predicates use, so the visibility fragments and the map fragments
// interleave without renumbering each other.
type mapFilters struct {
	status   string
	placeID  string
	fromYear int
	toYear   int
	viewport *Viewport
	params   *visibility.Params
}

func newMapFilters(input MapInput, params *visibility.Params) mapFilters {
	return mapFilters{
		status:   input.Status,
		placeID:  input.PlaceID,
		fromYear: input.FromYear,
		toYear:   input.ToYear,
		viewport: input.Viewport,
		params:   params,
	}
}

// statusPredicate compares against the status the feature is *drawn* with, which is
// not always a column.
//
// A region is drawn as 'disputed' or 'interpreted' depending on its certainty, so
// a filter that compared the column would answer a different question from the one
// the caller asked: status=interpreted would return nothing, because a region's
// column holds 'approximate' or 'precise'. Passing the drawn-status expression is
// what keeps the inferred/documented/disputed layers separate and findable.
func (f mapFilters) statusPredicate(drawnStatus string) string {
	reference := f.params.Add(f.status)
	return "(" + reference + " = '' OR " + drawnStatus + " = " + reference + ")"
}

// placePredicate scopes to one place. A layer that has no place of its own - a
// historical region - is excluded by this filter exactly as it was before, because
// its place reference is NULL and no uuid compares equal to NULL.
func (f mapFilters) placePredicate(placeReference string) string {
	reference := f.params.Add(f.placeID)
	return "(" + reference + " = '' OR " + placeReference + " = " + reference + "::uuid)"
}

// timePredicate is the year filter, in SQL, with the same edge case the Go filter
// had.
//
// A feature carrying no dates at all is kept under any year filter, because a
// platform does not know when a place was. A plain `time_from <= to_year` would
// quietly drop those rows instead, so the expression checks for "no dates" first
// and only then applies the overlap test. This is the kind of narrowing a
// "same filter, faster" change is not allowed to make silently.
func (f mapFilters) timePredicate(fromReference, toReference string) string {
	fromYear := "COALESCE(EXTRACT(YEAR FROM " + fromReference + "), 0)"
	toYear := "COALESCE(EXTRACT(YEAR FROM " + toReference + "), 0)"
	fromFilter := f.params.Add(f.fromYear)
	toFilter := f.params.Add(f.toYear)
	return "((" + fromYear + " = 0 AND " + toYear + " = 0) OR ((" + toFilter + " = 0 OR " + fromYear + " <= " + toFilter + ") AND (" +
		fromFilter + " = 0 OR " + toYear + " = 0 OR " + toYear + " >= " + fromFilter + ")))"
}

// viewportPredicate keeps the features inside the box the caller is drawing. A
// feature with no position cannot be inside any box, so it is excluded rather than
// kept, and every layer's own WHERE already requires a geometry.
func (f mapFilters) viewportPredicate(longitudeReference, latitudeReference string) string {
	if f.viewport == nil {
		return "TRUE"
	}
	west := f.params.Add(f.viewport.MinLongitude)
	south := f.params.Add(f.viewport.MinLatitude)
	east := f.params.Add(f.viewport.MaxLongitude)
	north := f.params.Add(f.viewport.MaxLatitude)
	return "(" + longitudeReference + " IS NOT NULL AND " + latitudeReference + " IS NOT NULL AND " +
		longitudeReference + " >= " + west + " AND " + longitudeReference + " <= " + east + " AND " +
		latitudeReference + " >= " + south + " AND " + latitudeReference + " <= " + north + ")"
}

// list reads the map for one input.
//
// The four feature queries used to return every visible row of their table and the
// filters were applied afterwards, in Go, which meant the database built, joined and
// shipped rows the caller had already decided not to see. Each query now carries the
// same status, place, year and viewport predicates, and each carries its own bound:
// the response is assembled in the order it always was - places, then associations,
// then migrations, then regions - and the first Limit features of that sequence are
// what the caller gets, with Truncated saying whether the sequence continued.
//
// The order the four layers are concatenated in is part of the response contract, so
// the limit is applied across the concatenation rather than per layer: a per-layer
// limit would let the last layer push the response past the bound.
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
	features := make([]Feature, 0, 64)
	truncated := false
	for _, layer := range s.featureLayers() {
		remaining := input.Limit - len(features)
		if remaining <= 0 {
			// The bound is spent, but "the bound is spent" is not the same answer as
			// "there is nothing left". The next layer is asked for a single row, so
			// the map can say which of the two it is instead of guessing.
			probe, probeErr := layer.fetch(ctx, s, policy, newMapFilters(input, visibility.NewParams()), 1)
			if probeErr != nil {
				return MapResponse{}, probeErr
			}
			if len(probe) > 0 {
				truncated = true
				break
			}
			continue
		}
		// One row beyond what fits, which is how the layer says "there are more"
		// without the response ever having to count the whole table.
		rows, fetchErr := layer.fetch(ctx, s, policy, newMapFilters(input, visibility.NewParams()), remaining+1)
		if fetchErr != nil {
			return MapResponse{}, fetchErr
		}
		if len(rows) > remaining {
			features = append(features, rows[:remaining]...)
			truncated = true
			break
		}
		features = append(features, rows...)
	}
	return MapResponse{
		FromYear:  input.FromYear,
		ToYear:    input.ToYear,
		Status:    input.Status,
		Features:  features,
		Limit:     input.Limit,
		Truncated: truncated,
	}, nil
}

// featureLayer is one of the four kinds of feature on the map, in the order the
// response concatenates them.
type featureLayer struct {
	name  string
	fetch func(context.Context, *Service, visibility.Policy, mapFilters, int) ([]Feature, error)
}

func (s *Service) featureLayers() []featureLayer {
	return []featureLayer{
		{name: "places", fetch: func(ctx context.Context, service *Service, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
			return service.placeFeatures(ctx, policy, filters, limit)
		}},
		{name: "associations", fetch: func(ctx context.Context, service *Service, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
			return service.associationFeatures(ctx, policy, filters, limit)
		}},
		{name: "migrations", fetch: func(ctx context.Context, service *Service, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
			return service.migrationFeatures(ctx, policy, filters, limit)
		}},
		{name: "regions", fetch: func(ctx context.Context, service *Service, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
			return service.regionFeatures(ctx, policy, filters, limit)
		}},
	}
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
//
// The ordering ends in the id, which makes it a total order. That is not tidiness:
// the response is bounded, and a bound over an order with ties picks an arbitrary
// subset, so a repeated request could show a different place each time.
func (s *Service) placeFeatures(ctx context.Context, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
	limitReference := filters.params.Add(limit)
	longitude := "ST_X(p.geometry)"
	latitude := "ST_Y(p.geometry)"
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, 'place', p.id, p.canonical_name_ar, NULL::text, NULL::uuid, NULL::text,
		       'place', 'documented', NULL::text, NULL::date, NULL::date, NULL::uuid, NULL::text,
		       NULL::uuid, NULL::text, NULL::text, `+longitude+`, `+latitude+`, NULL::float8, NULL::float8, NULL::float8, NULL::float8
		FROM places p
		WHERE p.geometry IS NOT NULL AND `+policy.ReferencePredicate(filters.params, "place", "p.id")+`
		  AND `+filters.statusPredicate("'documented'")+`
		  AND `+filters.placePredicate("p.id")+`
		  AND `+filters.timePredicate("NULL::date", "NULL::date")+`
		  AND `+filters.viewportPredicate(longitude, latitude)+`
		ORDER BY p.canonical_name_ar, p.id
		LIMIT `+limitReference,
		filters.params.Args()...)
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
func (s *Service) associationFeatures(ctx context.Context, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
	limitReference := filters.params.Add(limit)
	longitude := "ST_X(p.geometry)"
	latitude := "ST_Y(p.geometry)"
	rows, err := s.Pool.Query(ctx, `
		SELECT ga.id, 'association', ga.place_id, p.canonical_name_ar, ga.entity_type, ga.entity_id,
		       COALESCE(ep.canonical_name_ar, ef.canonical_name_ar, et.canonical_name_ar, ga.entity_id::text),
		       ga.relation_type, ga.status, ga.certainty, ga.time_from, ga.time_to, ga.source_id, s.title_ar,
		       se.id, se.evidence_text_ar, se.review_status, `+longitude+`, `+latitude+`, NULL::float8, NULL::float8, NULL::float8, NULL::float8
		FROM geographic_associations ga
		JOIN places p ON p.id = ga.place_id
		LEFT JOIN people ep ON ga.entity_type = 'person' AND ep.id = ga.entity_id
		LEFT JOIN families ef ON ga.entity_type = 'family' AND ef.id = ga.entity_id
		LEFT JOIN tribes et ON ga.entity_type = 'tribe' AND et.id = ga.entity_id
		LEFT JOIN branches eb ON ga.entity_type = 'branch' AND eb.id = ga.entity_id
		LEFT JOIN sources s ON s.id = ga.source_id
		LEFT JOIN spatial_evidence se ON se.geographic_association_id = ga.id
		WHERE p.geometry IS NOT NULL AND `+policy.ReferencePredicate(filters.params, "place", "p.id")+`
		  AND (ga.entity_type <> 'person' OR `+policy.PersonPredicate(filters.params, "ep.id")+`)
		  AND (ga.entity_type <> 'family' OR `+policy.ReferencePredicate(filters.params, "family", "ef.id")+`)
		  AND (ga.entity_type <> 'tribe' OR `+policy.ReferencePredicate(filters.params, "tribe", "et.id")+`)
		  AND (ga.entity_type <> 'branch' OR `+policy.ReferencePredicate(filters.params, "branch", "eb.id")+`)
		  AND (s.id IS NULL OR s.visibility = 'public')
		  AND `+filters.statusPredicate("ga.status")+`
		  AND `+filters.placePredicate("ga.place_id")+`
		  AND `+filters.timePredicate("ga.time_from", "ga.time_to")+`
		  AND `+filters.viewportPredicate(longitude, latitude)+`
		ORDER BY ga.time_from NULLS LAST, ga.created_at DESC, ga.id
		LIMIT `+limitReference,
		filters.params.Args()...)
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
func (s *Service) migrationFeatures(ctx context.Context, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
	limitReference := filters.params.Add(limit)
	geometry := "COALESCE(pt.geometry, pf.geometry)"
	longitude := "ST_X(" + geometry + ")"
	latitude := "ST_Y(" + geometry + ")"
	rows, err := s.Pool.Query(ctx, `
		SELECT m.id, 'migration', COALESCE(m.to_place_id, m.from_place_id), COALESCE(pt.canonical_name_ar, pf.canonical_name_ar),
		       m.subject_type, m.subject_id, COALESCE(ep.canonical_name_ar, m.subject_id::text), 'migration', m.status, m.certainty,
		       m.time_from, m.time_to, m.source_id, s.title_ar, se.id, se.evidence_text_ar, se.review_status,
		       `+longitude+`, `+latitude+`, ST_X(pf.geometry), ST_Y(pf.geometry), ST_X(pt.geometry), ST_Y(pt.geometry)
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
		  AND (pf.id IS NULL OR `+policy.ReferencePredicate(filters.params, "place", "pf.id")+`)
		  AND (pt.id IS NULL OR `+policy.ReferencePredicate(filters.params, "place", "pt.id")+`)
		  AND (m.subject_type <> 'person' OR `+policy.PersonPredicate(filters.params, "ep.id")+`)
		  AND (m.subject_type <> 'family' OR `+policy.ReferencePredicate(filters.params, "family", "ef.id")+`)
		  AND (m.subject_type <> 'tribe' OR `+policy.ReferencePredicate(filters.params, "tribe", "et.id")+`)
		  AND (m.subject_type <> 'branch' OR `+policy.ReferencePredicate(filters.params, "branch", "eb.id")+`)
		  AND (s.id IS NULL OR s.visibility = 'public')
		  AND `+filters.statusPredicate("m.status")+`
		  AND `+filters.placePredicate("COALESCE(m.to_place_id, m.from_place_id)")+`
		  AND `+filters.timePredicate("m.time_from", "m.time_to")+`
		  AND `+filters.viewportPredicate(longitude, latitude)+`
		ORDER BY m.time_from NULLS LAST, m.created_at DESC, m.id
		LIMIT `+limitReference,
		filters.params.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

// regionFeatures lists the historical regions.
//
// A region is the one layer the platform drew rather than a source documented, so
// its drawn status is derived from its certainty: a disputed region is 'disputed'
// and every other one is 'interpreted'. The status filter compares that derived
// value, which is what keeps the inferred and the disputed layers addressable
// instead of collapsing them into the certainty column.
func (s *Service) regionFeatures(ctx context.Context, policy visibility.Policy, filters mapFilters, limit int) ([]Feature, error) {
	limitReference := filters.params.Add(limit)
	centroid := "ST_Centroid(r.geometry)"
	longitude := "ST_X(" + centroid + ")"
	latitude := "ST_Y(" + centroid + ")"
	drawnStatus := "(CASE WHEN r.certainty = 'disputed' THEN 'disputed' ELSE 'interpreted' END)"
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, 'region', NULL::uuid, r.name_ar, NULL::text, NULL::uuid, NULL::text, 'historical_region',
		       `+drawnStatus+`, r.certainty, r.valid_from, r.valid_to,
		       r.source_id, s.title_ar, NULL::uuid, NULL::text, NULL::text,
		       `+longitude+`, `+latitude+`, NULL::float8, NULL::float8, NULL::float8, NULL::float8
		FROM historical_regions r
		LEFT JOIN sources s ON s.id = r.source_id
		WHERE r.geometry IS NOT NULL AND (s.id IS NULL OR s.visibility = 'public')
		  AND `+filters.statusPredicate(drawnStatus)+`
		  AND `+filters.placePredicate("NULL::uuid")+`
		  AND `+filters.timePredicate("r.valid_from", "r.valid_to")+`
		  AND `+filters.viewportPredicate(longitude, latitude)+`
		ORDER BY r.name_ar, r.id
		LIMIT `+limitReference,
		filters.params.Args()...)
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
	if input.Limit < 0 || input.Limit > MaxMapFeatures {
		return MapInput{}, ErrValidation
	}
	if input.Limit == 0 {
		input.Limit = MaxMapFeatures
	}
	if input.Viewport != nil {
		if err := validateViewport(*input.Viewport); err != nil {
			return MapInput{}, err
		}
	}
	input.ActorID = strings.TrimSpace(input.ActorID)
	return input, nil
}

// validateViewport refuses a box that cannot describe an area. An inverted or
// out-of-range box would otherwise be a filter that silently returns nothing, and
// "nothing" is indistinguishable from "the map is empty because of the policy".
func validateViewport(viewport Viewport) error {
	if viewport.MinLongitude < -180 || viewport.MaxLongitude > 180 || viewport.MinLatitude < -90 || viewport.MaxLatitude > 90 {
		return ErrValidation
	}
	if viewport.MinLongitude > viewport.MaxLongitude || viewport.MinLatitude > viewport.MaxLatitude {
		return ErrValidation
	}
	return nil
}

func validStatus(status string) bool {
	return status == "" || status == "documented" || status == "interpreted" || status == "platform_inferred" || status == "disputed" || status == "unresolved"
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
