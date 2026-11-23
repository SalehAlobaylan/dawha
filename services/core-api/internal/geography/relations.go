// Relation flow arcs (plan 032-E): the atlas's read over entity_relationships.
//
// The arcs are the RECORDED relationships, not an inference: each endpoint
// resolves to its documented place anchor through the same claims lane the
// atlas's markers already read — the endpoint entity's claim carrying a
// readable, located place. The predicate set is DATA-DRIVEN (whatever the DB
// records; alliance/marriage/raid are the expected shapes, no new schema),
// and the era window rides the relationship's own valid_from/valid_to. An
// endpoint with no anchored place is reported as unanchored — the layer lists
// it honestly instead of inventing a midpoint.
//
// Bounded: LIMIT 200 + truncated (the plan's own 200-edge discipline).
// Visibility: the anchoring CLAIM rides the policy's claim predicate and the
// anchor PLACE the reference predicate, mirroring the markers' contract
// exactly, so no coordinate reaches the map through a read the actor could
// not have had.
package geography

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
)

// AtlasRelationsLimit is the arc layer's bound.
const AtlasRelationsLimit = 200

// AtlasRelationEndpoint is one end of a relation arc: who, and where the map
// may draw it (Anchored=false carries no coordinates — the panel states the
// arc is not drawn and why).
type AtlasRelationEndpoint struct {
	EntityType string  `json:"entityType"`
	EntityID   string  `json:"entityId"`
	Label      string  `json:"label"`
	Longitude  float64 `json:"longitude,omitempty"`
	Latitude   float64 `json:"latitude,omitempty"`
	Anchored   bool    `json:"anchored"`
}

// AtlasRelationRow is one recorded relationship with its anchor resolution.
type AtlasRelationRow struct {
	ID        string                `json:"id"`
	Predicate string                `json:"predicate"`
	Status    string                `json:"status"`
	Subject   AtlasRelationEndpoint `json:"subject"`
	Object    AtlasRelationEndpoint `json:"object"`
	ValidFrom string                `json:"validFrom,omitempty"`
	ValidTo   string                `json:"validTo,omitempty"`
}

// AtlasRelationsResponse is the bounded arc read.
type AtlasRelationsResponse struct {
	Relations []AtlasRelationRow `json:"relations"`
	Limit     int                `json:"limit"`
	Truncated bool               `json:"truncated"`
}

// anchorFromJSON reads one anchor payload ({lon, lat}, or the SQL NULL's
// literal "null") into the endpoint's coordinates. A null or unreadable
// anchor is the honest unanchored state — never a zero-coordinate guess.
func anchorFromJSON(entityType, entityID, label string, payload []byte) AtlasRelationEndpoint {
	endpoint := AtlasRelationEndpoint{EntityType: entityType, EntityID: entityID, Label: label}
	var anchor *struct {
		Lon float64 `json:"lon"`
		Lat float64 `json:"lat"`
	}
	if json.Unmarshal(payload, &anchor) == nil && anchor != nil {
		endpoint.Longitude = anchor.Lon
		endpoint.Latitude = anchor.Lat
		endpoint.Anchored = true
	}
	return endpoint
}

// AtlasRelations reads one era's recorded relationship arcs (any era when
// both bounds are zero). The window overlaps the relationship's own years;
// a row without era bounds renders under every era. The bound is the plan's
// 200-edge discipline, and truncation is stated, not silent.
func (s *Service) AtlasRelations(ctx context.Context, actorID string, fromYear, toYear int) (AtlasRelationsResponse, error) {
	if s.Pool == nil {
		return AtlasRelationsResponse{}, ErrDatabaseUnavailable
	}
	if _, err := uuid.Parse(actorID); err != nil && strings.TrimSpace(actorID) != "" {
		return AtlasRelationsResponse{}, ErrValidation
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return AtlasRelationsResponse{}, err
	}
	params := visibility.NewParams()
	limitReference := params.Add(AtlasRelationsLimit + 1)
	fromReference := params.Add(fromYear)
	toReference := params.Add(toYear)

	rows, err := s.Pool.Query(ctx, `
		SELECT rel.id::text, rel.predicate, rel.status,
		       rel.subject_type, rel.object_type,
		       COALESCE(sp.canonical_name_ar, st.canonical_name_ar, ''),
		       COALESCE(op.canonical_name_ar, ot.canonical_name_ar, ''),
		       subject_anchor.anchor, object_anchor.anchor,
		       COALESCE(rel.valid_from::text, ''), COALESCE(rel.valid_to::text, '')
		FROM entity_relationships rel
		LEFT JOIN LATERAL (
			SELECT jsonb_build_object('lon', ST_X(p.geometry), 'lat', ST_Y(p.geometry))::text AS anchor
			FROM claims rc
			JOIN places p ON p.id = rc.place_id AND p.geometry IS NOT NULL
			WHERE ((rc.subject_type = rel.subject_type AND rc.subject_id = rel.subject_id)
			    OR (rc.object_type = rel.subject_type AND rc.object_id = rel.subject_id))
			  AND `+policy.ClaimPredicate(params, "rc.id")+`
			  AND `+policy.ReferencePredicate(params, "place", "p.id")+`
			ORDER BY rc.created_at DESC
			LIMIT 1
		) subject_anchor ON TRUE
		LEFT JOIN LATERAL (
			SELECT jsonb_build_object('lon', ST_X(p.geometry), 'lat', ST_Y(p.geometry))::text AS anchor
			FROM claims rc
			JOIN places p ON p.id = rc.place_id AND p.geometry IS NOT NULL
			WHERE ((rc.subject_type = rel.object_type AND rc.subject_id = rel.object_id)
			    OR (rc.object_type = rel.object_type AND rc.object_id = rel.object_id))
			  AND `+policy.ClaimPredicate(params, "rc.id")+`
			  AND `+policy.ReferencePredicate(params, "place", "p.id")+`
			ORDER BY rc.created_at DESC
			LIMIT 1
		) object_anchor ON TRUE
		LEFT JOIN people sp ON rel.subject_type = 'person' AND sp.id = rel.subject_id
		LEFT JOIN tribes st ON rel.subject_type = 'tribe' AND st.id = rel.subject_id
		LEFT JOIN people op ON rel.object_type = 'person' AND op.id = rel.object_id
		LEFT JOIN tribes ot ON rel.object_type = 'tribe' AND ot.id = rel.object_id
		WHERE (rel.valid_from IS NULL OR $`+toReference+` = 0 OR EXTRACT(YEAR FROM rel.valid_from)::int <= $`+toReference+`)
		  AND (rel.valid_to IS NULL OR $`+fromReference+` = 0 OR EXTRACT(YEAR FROM rel.valid_to)::int >= $`+fromReference+`)
		ORDER BY rel.created_at DESC, rel.id
		LIMIT `+limitReference, params.Args()...)
	if err != nil {
		return AtlasRelationsResponse{}, err
	}
	defer rows.Close()

	response := AtlasRelationsResponse{Relations: []AtlasRelationRow{}, Limit: AtlasRelationsLimit}
	for rows.Next() {
		var row AtlasRelationRow
		var subjectJSON, objectJSON []byte
		if err := rows.Scan(&row.ID, &row.Predicate, &row.Status,
			&row.Subject.EntityType, &row.Object.EntityType,
			&row.Subject.Label, &row.Object.Label,
			&subjectJSON, &objectJSON,
			&row.ValidFrom, &row.ValidTo); err != nil {
			return AtlasRelationsResponse{}, err
		}
		row.Subject = anchorFromJSON(row.Subject.EntityType, row.Subject.EntityID, row.Subject.Label, subjectJSON)
		row.Object = anchorFromJSON(row.Object.EntityType, row.Object.EntityID, row.Object.Label, objectJSON)
		response.Relations = append(response.Relations, row)
	}
	if err := rows.Err(); err != nil {
		return AtlasRelationsResponse{}, err
	}
	if len(response.Relations) > AtlasRelationsLimit {
		response.Relations = response.Relations[:AtlasRelationsLimit]
		response.Truncated = true
	}
	return response, nil
}
