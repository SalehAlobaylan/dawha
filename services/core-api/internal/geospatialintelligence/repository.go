package geospatialintelligence

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type treeScope struct {
	TreeID         string
	TreeName       string
	TreeVersionID  string
	VersionNumber  int
	VersionState   string
	TreeVisibility string
	TargetPresent  bool
}

type placeRecord struct {
	ID         string
	Name       string
	Normalized string
	PlaceType  string
	Latitude   *float64
	Longitude  *float64
}

type historicalNameRecord struct {
	PlaceID    string
	Name       string
	Normalized string
	NameType   string
	ValidFrom  string
	ValidTo    string
	SourceID   string
}

type sourceRecord struct {
	ID               string
	Title            string
	DependencyStatus string
	Visibility       string
	CreatedBy        string
}

type statementRecord struct {
	ID               string
	SourceID         string
	SourceTitle      string
	PassageID        string
	Text             string
	ReviewStatus     string
	DependencyStatus string
	Visibility       string
}

type associationRecord struct {
	ID             string
	EntityType     string
	EntityID       string
	EntityName     string
	PlaceID        string
	PlaceName      string
	RelationType   string
	Status         string
	Certainty      string
	TimeFrom       string
	TimeTo         string
	ClaimID        string
	SourceID       string
	SourceTitle    string
	EvidenceID     string
	EvidenceStatus string
	Latitude       *float64
	Longitude      *float64
}

type migrationRecord struct {
	ID             string
	SubjectType    string
	SubjectID      string
	SubjectName    string
	FromPlaceID    string
	FromPlaceName  string
	ToPlaceID      string
	ToPlaceName    string
	TimeFrom       string
	TimeTo         string
	Status         string
	Certainty      string
	ClaimID        string
	SourceID       string
	SourceTitle    string
	EvidenceID     string
	EvidenceStatus string
	FromLatitude   *float64
	FromLongitude  *float64
	ToLatitude     *float64
	ToLongitude    *float64
}

type spatialEdge struct {
	FirstID  string
	SecondID string
	Distance float64
}

func loadTreeScope(ctx context.Context, q queryer, input RunInput) (treeScope, error) {
	if input.TreeID == "" && input.TreeVersionID == "" {
		return treeScope{}, nil
	}
	var result treeScope
	var targetPresent bool
	if err := q.QueryRow(ctx, `
		SELECT t.id::text, t.name_ar, t.visibility, tv.id::text, tv.version_number, tv.state,
		       CASE WHEN $4 = 'person' THEN EXISTS (
		         SELECT 1 FROM tree_nodes tn WHERE tn.tree_version_id = tv.id AND tn.person_id = $3
		       ) ELSE TRUE END
		FROM trees t
		JOIN tree_versions tv ON tv.tree_id = t.id
		WHERE t.id = $1 AND tv.id = $2
	`, input.TreeID, input.TreeVersionID, input.EntityID, input.EntityType).Scan(&result.TreeID, &result.TreeName, &result.TreeVisibility, &result.TreeVersionID, &result.VersionNumber, &result.VersionState, &targetPresent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return treeScope{}, ErrNotFound
		}
		return treeScope{}, err
	}
	result.TargetPresent = targetPresent
	return result, nil
}

func loadEntityName(ctx context.Context, q queryer, entityType, entityID string) (string, error) {
	var table string
	switch entityType {
	case "person":
		table = "people"
	case "source":
		return "", nil
	case "place":
		table = "places"
	default:
		return "", ErrValidation
	}
	var name string
	if err := q.QueryRow(ctx, `SELECT canonical_name_ar FROM `+table+` WHERE id = $1`, entityID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return name, nil
}

func loadSource(ctx context.Context, q queryer, sourceID string) (sourceRecord, error) {
	var result sourceRecord
	var createdBy pgtype.UUID
	if err := q.QueryRow(ctx, `SELECT id, title_ar, dependency_status, visibility, created_by FROM sources WHERE id = $1`, sourceID).Scan(&result.ID, &result.Title, &result.DependencyStatus, &result.Visibility, &createdBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceRecord{}, ErrNotFound
		}
		return sourceRecord{}, err
	}
	result.CreatedBy = uuidString(createdBy)
	return result, nil
}

func loadPlace(ctx context.Context, q queryer, placeID string) (placeRecord, error) {
	var result placeRecord
	var latitude, longitude pgtype.Float8
	if err := q.QueryRow(ctx, `SELECT id::text, canonical_name_ar, normalized_name_ar, place_type, ST_Y(geometry), ST_X(geometry) FROM places WHERE id = $1`, placeID).Scan(&result.ID, &result.Name, &result.Normalized, &result.PlaceType, &latitude, &longitude); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return placeRecord{}, ErrNotFound
		}
		return placeRecord{}, err
	}
	result.Latitude = floatValue(latitude)
	result.Longitude = floatValue(longitude)
	return result, nil
}

func loadPlaces(ctx context.Context, q queryer, limit int) ([]placeRecord, error) {
	rows, err := q.Query(ctx, `
		SELECT id, canonical_name_ar, normalized_name_ar, place_type, ST_Y(geometry), ST_X(geometry)
		FROM places
		ORDER BY canonical_name_ar, id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]placeRecord, 0)
	for rows.Next() {
		var item placeRecord
		var id pgtype.UUID
		var latitude, longitude pgtype.Float8
		if err := rows.Scan(&id, &item.Name, &item.Normalized, &item.PlaceType, &latitude, &longitude); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.Latitude = floatValue(latitude)
		item.Longitude = floatValue(longitude)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadHistoricalNames(ctx context.Context, q queryer, limit int) ([]historicalNameRecord, error) {
	rows, err := q.Query(ctx, `
		SELECT h.place_id::text, h.name_ar, h.normalized_name_ar, h.name_type, h.valid_from, h.valid_to, COALESCE(h.source_id::text, '')
		FROM historical_place_names h
		LEFT JOIN sources s ON s.id = h.source_id
		WHERE s.id IS NULL OR s.visibility = 'public'
		ORDER BY h.normalized_name_ar, h.place_id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]historicalNameRecord, 0)
	for rows.Next() {
		var item historicalNameRecord
		var validFrom, validTo pgtype.Date
		if err := rows.Scan(&item.PlaceID, &item.Name, &item.Normalized, &item.NameType, &validFrom, &validTo, &item.SourceID); err != nil {
			return nil, err
		}
		item.ValidFrom = dateValue(validFrom)
		item.ValidTo = dateValue(validTo)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadSourceIDs(ctx context.Context, q queryer, input RunInput) ([]string, error) {
	var rows pgx.Rows
	var err error
	switch input.EntityType {
	case "source":
		return []string{input.EntityID}, nil
	case "person":
		rows, err = q.Query(ctx, `
			SELECT DISTINCT source_id FROM (
				SELECT ga.source_id FROM geographic_associations ga WHERE ga.entity_type = 'person' AND ga.entity_id = $1
				UNION ALL
				SELECT m.source_id FROM migration_events m WHERE m.subject_type = 'person' AND m.subject_id = $1
				UNION ALL
				SELECT tr.source_id FROM tree_relationships tr WHERE tr.tree_version_id = $2 AND tr.status = 'interpreted'
			) source_ids
			WHERE source_id IS NOT NULL
			ORDER BY source_id
			LIMIT $3
		`, input.EntityID, input.TreeVersionID, input.MaximumRecords)
	case "place":
		rows, err = q.Query(ctx, `
			SELECT DISTINCT source_id FROM (
				SELECT ga.source_id FROM geographic_associations ga WHERE ga.place_id = $1
				UNION ALL
				SELECT m.source_id FROM migration_events m WHERE m.from_place_id = $1 OR m.to_place_id = $1
			) source_ids
			WHERE source_id IS NOT NULL
			ORDER BY source_id
			LIMIT $2
		`, input.EntityID, input.MaximumRecords)
	default:
		return nil, ErrValidation
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]string, 0)
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if value := uuidString(id); value != "" {
			items = append(items, value)
		}
	}
	return items, rows.Err()
}

func loadSourceRecords(ctx context.Context, q queryer, ids []string) (map[string]sourceRecord, error) {
	items := make(map[string]sourceRecord)
	if len(ids) == 0 {
		return items, nil
	}
	values := make([]uuid.UUID, 0, len(ids))
	for _, value := range ids {
		parsed, err := uuid.Parse(value)
		if err != nil {
			continue
		}
		values = append(values, parsed)
	}
	rows, err := q.Query(ctx, `SELECT id, title_ar, dependency_status, visibility, created_by FROM sources WHERE id = ANY($1::uuid[])`, values)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item sourceRecord
		var id, createdBy pgtype.UUID
		if err := rows.Scan(&id, &item.Title, &item.DependencyStatus, &item.Visibility, &createdBy); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.CreatedBy = uuidString(createdBy)
		items[item.ID] = item
	}
	return items, rows.Err()
}

func loadStatements(ctx context.Context, q queryer, sourceIDs []string, limit int) ([]statementRecord, error) {
	if len(sourceIDs) == 0 {
		return []statementRecord{}, nil
	}
	values := make([]uuid.UUID, 0, len(sourceIDs))
	for _, value := range sourceIDs {
		parsed, err := uuid.Parse(value)
		if err != nil {
			continue
		}
		values = append(values, parsed)
	}
	rows, err := q.Query(ctx, `
		SELECT ss.id, ss.source_id, s.title_ar, ss.source_passage_id, ss.statement_text_ar,
		       ss.review_status, s.dependency_status, s.visibility
		FROM source_statements ss
		JOIN sources s ON s.id = ss.source_id
		WHERE ss.source_id = ANY($1::uuid[])
		  AND ss.review_status = 'accepted'
		  AND s.visibility = 'public'
		  AND s.dependency_status = 'independent'
		  AND NOT EXISTS (
			SELECT 1 FROM source_dependencies sd
			WHERE sd.status IN ('needs_review', 'confirmed')
			  AND (sd.source_id = s.id OR sd.depends_on_source_id = s.id)
		  )
		ORDER BY ss.source_id, ss.created_at
		LIMIT $2
	`, values, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]statementRecord, 0)
	for rows.Next() {
		var item statementRecord
		var id, sourceID, passageID pgtype.UUID
		if err := rows.Scan(&id, &sourceID, &item.SourceTitle, &passageID, &item.Text, &item.ReviewStatus, &item.DependencyStatus, &item.Visibility); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.SourceID = uuidString(sourceID)
		item.PassageID = uuidString(passageID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadAssociations(ctx context.Context, q queryer, input RunInput, limit int) ([]associationRecord, error) {
	var rows pgx.Rows
	var err error
	where := ""
	args := []any{}
	switch input.EntityType {
	case "person":
		where = "ga.entity_type = 'person' AND ga.entity_id = $1"
		args = append(args, input.EntityID)
	case "source":
		where = "ga.source_id = $1"
		args = append(args, input.EntityID)
	case "place":
		where = "ga.place_id = $1"
		args = append(args, input.EntityID)
	default:
		return nil, ErrValidation
	}
	args = append(args, limit)
	query := `
		SELECT DISTINCT ON (ga.id)
		       ga.id, ga.entity_type, ga.entity_id,
		       COALESCE(ep.canonical_name_ar, ef.canonical_name_ar, et.canonical_name_ar, ga.entity_id::text),
		       ga.place_id, p.canonical_name_ar, ga.relation_type, ga.status, ga.certainty,
		       ga.time_from, ga.time_to, ga.claim_id, ga.source_id,
		       COALESCE(s.title_ar, ''), se.id, COALESCE(se.review_status, ''),
		       ST_Y(p.geometry), ST_X(p.geometry)
		FROM geographic_associations ga
		JOIN places p ON p.id = ga.place_id
		LEFT JOIN people ep ON ga.entity_type = 'person' AND ep.id = ga.entity_id
		LEFT JOIN families ef ON ga.entity_type = 'family' AND ef.id = ga.entity_id
		LEFT JOIN tribes et ON ga.entity_type = 'tribe' AND et.id = ga.entity_id
		LEFT JOIN sources s ON s.id = ga.source_id
		LEFT JOIN spatial_evidence se ON se.geographic_association_id = ga.id
		WHERE ` + where + `
		  AND ga.status IN ('documented', 'interpreted', 'platform_inferred')
		  AND p.geometry IS NOT NULL
		  AND s.visibility = 'public'
		  AND s.dependency_status = 'independent'
		  AND NOT EXISTS (
			SELECT 1 FROM source_dependencies sd
			WHERE sd.status IN ('needs_review', 'confirmed')
			  AND (sd.source_id = s.id OR sd.depends_on_source_id = s.id)
		  )
		ORDER BY ga.id, CASE WHEN se.review_status = 'accepted' THEN 0 WHEN se.review_status = 'needs_review' THEN 1 ELSE 2 END, se.created_at
		LIMIT $` + itoa(len(args)) + `
	`
	rows, err = q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]associationRecord, 0)
	for rows.Next() {
		var item associationRecord
		var id, entityID, placeID, claimID, sourceID, evidenceID pgtype.UUID
		var timeFrom, timeTo pgtype.Date
		var latitude, longitude pgtype.Float8
		if err := rows.Scan(&id, &item.EntityType, &entityID, &item.EntityName, &placeID, &item.PlaceName, &item.RelationType, &item.Status, &item.Certainty, &timeFrom, &timeTo, &claimID, &sourceID, &item.SourceTitle, &evidenceID, &item.EvidenceStatus, &latitude, &longitude); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.EntityID = uuidString(entityID)
		item.PlaceID = uuidString(placeID)
		item.ClaimID = uuidString(claimID)
		item.SourceID = uuidString(sourceID)
		item.EvidenceID = uuidString(evidenceID)
		item.TimeFrom = dateValue(timeFrom)
		item.TimeTo = dateValue(timeTo)
		item.Latitude = floatValue(latitude)
		item.Longitude = floatValue(longitude)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMigrations(ctx context.Context, q queryer, input RunInput, limit int) ([]migrationRecord, error) {
	var rows pgx.Rows
	var err error
	where := ""
	args := []any{}
	switch input.EntityType {
	case "person":
		where = "m.subject_type = 'person' AND m.subject_id = $1"
		args = append(args, input.EntityID)
	case "source":
		where = "m.source_id = $1"
		args = append(args, input.EntityID)
	case "place":
		where = "m.from_place_id = $1 OR m.to_place_id = $1"
		args = append(args, input.EntityID)
	default:
		return nil, ErrValidation
	}
	args = append(args, limit)
	query := `
		SELECT DISTINCT ON (m.id)
		       m.id, m.subject_type, m.subject_id,
		       COALESCE(ep.canonical_name_ar, m.subject_id::text),
		       m.from_place_id, COALESCE(pf.canonical_name_ar, ''),
		       m.to_place_id, COALESCE(pt.canonical_name_ar, ''),
		       m.time_from, m.time_to, m.status, m.certainty, m.claim_id,
		       m.source_id, COALESCE(s.title_ar, ''), se.id, COALESCE(se.review_status, ''),
		       ST_Y(pf.geometry), ST_X(pf.geometry), ST_Y(pt.geometry), ST_X(pt.geometry)
		FROM migration_events m
		LEFT JOIN places pf ON pf.id = m.from_place_id
		LEFT JOIN places pt ON pt.id = m.to_place_id
		LEFT JOIN people ep ON m.subject_type = 'person' AND ep.id = m.subject_id
		LEFT JOIN sources s ON s.id = m.source_id
		LEFT JOIN spatial_evidence se ON se.migration_event_id = m.id
		WHERE ` + where + `
		  AND m.status IN ('documented', 'interpreted', 'platform_inferred', 'disputed')
		  AND (s.id IS NULL OR (s.visibility = 'public' AND s.dependency_status = 'independent' AND NOT EXISTS (
			SELECT 1 FROM source_dependencies sd
			WHERE sd.status IN ('needs_review', 'confirmed')
			  AND (sd.source_id = s.id OR sd.depends_on_source_id = s.id)
		  )))
		ORDER BY m.id, CASE WHEN se.review_status = 'accepted' THEN 0 WHEN se.review_status = 'needs_review' THEN 1 ELSE 2 END, se.created_at
		LIMIT $` + itoa(len(args)) + `
	`
	rows, err = q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]migrationRecord, 0)
	for rows.Next() {
		var item migrationRecord
		var id, subjectID, fromPlaceID, toPlaceID, claimID, sourceID, evidenceID pgtype.UUID
		var timeFrom, timeTo pgtype.Date
		var fromLatitude, fromLongitude, toLatitude, toLongitude pgtype.Float8
		if err := rows.Scan(&id, &item.SubjectType, &subjectID, &item.SubjectName, &fromPlaceID, &item.FromPlaceName, &toPlaceID, &item.ToPlaceName, &timeFrom, &timeTo, &item.Status, &item.Certainty, &claimID, &sourceID, &item.SourceTitle, &evidenceID, &item.EvidenceStatus, &fromLatitude, &fromLongitude, &toLatitude, &toLongitude); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.SubjectID = uuidString(subjectID)
		item.FromPlaceID = uuidString(fromPlaceID)
		item.ToPlaceID = uuidString(toPlaceID)
		item.ClaimID = uuidString(claimID)
		item.SourceID = uuidString(sourceID)
		item.EvidenceID = uuidString(evidenceID)
		item.TimeFrom = dateValue(timeFrom)
		item.TimeTo = dateValue(timeTo)
		item.FromLatitude = floatValue(fromLatitude)
		item.FromLongitude = floatValue(fromLongitude)
		item.ToLatitude = floatValue(toLatitude)
		item.ToLongitude = floatValue(toLongitude)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadSpatialEdges(ctx context.Context, q queryer, radiusMeters float64, limit int) ([]spatialEdge, error) {
	// Two corrections live in this query. The columns are geometry (SRID 4326), so
	// a bare ST_DWithin measures degrees, and a "meters" argument would silently
	// pass for every pair of points on Earth: the predicate casts both sides to
	// geography, where the unit is metres. And the pair enumeration starts from
	// distinct places, not associations, so the radius prunes before the
	// association join can multiply a pair of places into a cross product.
	//
	// ST_Expand is only a GiST-indexed pre-test. It is widened by latitude before
	// the exact geography predicate runs, because one degree of longitude is
	// shorter than one degree of latitude away from the equator; a fixed
	// metres/111320 expansion would clip valid pairs in the north.
	//
	// The association pairs are unordered here: the place-pair enumeration runs
	// in place-id order, which is not the same order as association ids, so a
	// `first_association.id < second_association.id` filter drops the edge
	// whenever the association ids order opposite to their places. The edge
	// itself does not care which endpoint is first — buildClusters matches edges
	// against a byID set — so the ids are written with LEAST/GREATEST and
	// deduplicated with DISTINCT instead.
	//
	// Both endpoints carry the same eligibility loadAssociations applies (the
	// status list, a public independent source, no live dependency). Edges
	// whose endpoints no caller can load were pure cost: buildClusters would
	// have intersected them away, after the database had built them from the
	// whole association table.
	rows, err := q.Query(ctx, `
		WITH place_pairs AS (
			SELECT first_place.id AS first_place_id, second_place.id AS second_place_id
			FROM places first_place
			JOIN places second_place ON second_place.id >= first_place.id
			WHERE first_place.geometry IS NOT NULL
			  AND second_place.geometry IS NOT NULL
			  AND first_place.geometry && ST_Expand(
			      second_place.geometry,
			      LEAST($1 / 111320.0 / GREATEST(cos(radians(ST_Y(second_place.geometry))), 0.01), 180.0) + LEAST($1 / 111320.0, 0.5)
			  )
			  AND ST_DWithin(first_place.geometry::geography, second_place.geometry::geography, $1)
		)
		SELECT DISTINCT LEAST(first_association.id, second_association.id)::text,
		       GREATEST(first_association.id, second_association.id)::text,
		       ST_DistanceSphere(first_place.geometry, second_place.geometry) / 1000.0 AS distance_km
		FROM place_pairs
		JOIN geographic_associations first_association ON first_association.place_id = place_pairs.first_place_id
		JOIN geographic_associations second_association ON second_association.place_id = place_pairs.second_place_id
		JOIN places first_place ON first_place.id = place_pairs.first_place_id
		JOIN places second_place ON second_place.id = place_pairs.second_place_id
		WHERE first_association.id <> second_association.id
		  AND first_association.status IN ('documented', 'interpreted', 'platform_inferred')
		  AND second_association.status IN ('documented', 'interpreted', 'platform_inferred')
		  AND EXISTS (SELECT 1 FROM sources s1 WHERE s1.id = first_association.source_id AND s1.visibility = 'public' AND s1.dependency_status = 'independent'
		              AND NOT EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.status IN ('needs_review', 'confirmed') AND (sd.source_id = s1.id OR sd.depends_on_source_id = s1.id)))
		  AND EXISTS (SELECT 1 FROM sources s2 WHERE s2.id = second_association.source_id AND s2.visibility = 'public' AND s2.dependency_status = 'independent'
		              AND NOT EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.status IN ('needs_review', 'confirmed') AND (sd.source_id = s2.id OR sd.depends_on_source_id = s2.id)))
		ORDER BY distance_km, LEAST(first_association.id, second_association.id)::text, GREATEST(first_association.id, second_association.id)::text
		LIMIT $2
	`, radiusMeters, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]spatialEdge, 0)
	for rows.Next() {
		var item spatialEdge
		if err := rows.Scan(&item.FirstID, &item.SecondID, &item.Distance); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
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

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
