// Tribe movement (plan 033-A): the tribe-subject مسالِك, the person lane's
// own contract with an additive subject.
//
// The migration's CHECK (0071) now admits person/tribe/family subjects; this
// endpoint reads the TRIBE lane: the tribe must be readable (the policy's own
// tribe reference kind, the same predicate the dictionary's index answers),
// the documenting source must be readable (a private source is not laundered
// through a public tribe — the 031-A rule that governs every dossier read),
// and each endpoint place must be readable subject to NULL. Bounded by
// MaxTribeMovementRows + truncated, the person lane's own bound. Events now
// carry the taxonomy's kind (033-D) verbatim — unspecified stays an honest
// NULL the front-end captions as «حدث غير مُصنَّف بعد».
package geography

import (
	"context"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxTribeMovementRows bounds one tribe's مسالِك read, the person lane's own
// answer to the same question.
const MaxTribeMovementRows = 50

// TribeMovementPlace is one endpoint place, the person lane's own shape.
type TribeMovementPlace struct {
	ID        string   `json:"id,omitempty"`
	Name      string   `json:"name,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
}

// TribeMovementEvent is one recorded movement under the tribe's subject: the
// period, the taxonomy kind (033-D's own keys — verbatim, unclassified =
// empty), and the receipts the inspector reads.
type TribeMovementEvent struct {
	ID             string             `json:"id"`
	Status         string             `json:"status"`
	Kind           string             `json:"kind,omitempty"`
	Certainty      string             `json:"certainty,omitempty"`
	TimeFrom       string             `json:"timeFrom,omitempty"`
	TimeTo         string             `json:"timeTo,omitempty"`
	Period         string             `json:"period"`
	Notes          string             `json:"notes,omitempty"`
	SourceID       string             `json:"sourceId,omitempty"`
	SourceTitle    string             `json:"sourceTitle,omitempty"`
	SourceCitation string             `json:"sourceCitation,omitempty"`
	FromPlace      TribeMovementPlace `json:"fromPlace,omitempty"`
	ToPlace        TribeMovementPlace `json:"toPlace,omitempty"`
}

// TribeMovementResponse is the bounded read.
type TribeMovementResponse struct {
	TribeID   string               `json:"tribeId"`
	TribeName string               `json:"tribeName,omitempty"`
	Events    []TribeMovementEvent `json:"events"`
	Limit     int                  `json:"limit"`
	Truncated bool                 `json:"truncated"`
}

// TribeMovement reads one tribe's documented مسالِك under the reader's
// policy — the person movement's own read with the tribe's walls.
func (s *Service) TribeMovement(ctx context.Context, tribeID string, actorID string) (TribeMovementResponse, error) {
	if s.Pool == nil {
		return TribeMovementResponse{}, ErrDatabaseUnavailable
	}
	tribeUUID, err := uuid.Parse(tribeID)
	if err != nil {
		return TribeMovementResponse{}, ErrValidation
	}

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return TribeMovementResponse{}, err
	}
	params := visibility.NewParams()
	tribeReference := params.Add(tribeUUID.String())
	limitReference := params.Add(MaxTribeMovementRows + 1)

	rows, err := s.Pool.Query(ctx, `
		SELECT tr.canonical_name_ar,
		       m.id, m.status, COALESCE(m.kind, ''), COALESCE(m.certainty, ''),
		       m.time_from, m.time_to, COALESCE(m.notes_ar, ''),
		       COALESCE(src.id::text, ''), COALESCE(src.title_ar, ''), COALESCE(src.citation_ar, ''),
		       COALESCE(pf.id::text, ''), pf.canonical_name_ar,
		       ST_X(pf.geometry), ST_Y(pf.geometry),
		       COALESCE(pt.id::text, ''), pt.canonical_name_ar,
		       ST_X(pt.geometry), ST_Y(pt.geometry)
		FROM migration_events m
		JOIN tribes tr ON tr.id = m.subject_id
		LEFT JOIN places pf ON pf.id = m.from_place_id
		LEFT JOIN places pt ON pt.id = m.to_place_id
		LEFT JOIN sources src ON src.id = m.source_id
		WHERE m.subject_type = 'tribe' AND m.subject_id = `+tribeReference+`
		  AND m.status = 'documented'
		  AND `+policy.ReferencePredicate(params, "tribe", "tr.id")+`
		  AND (m.source_id IS NULL OR `+policy.SourcePredicate(params, "src.id")+`)
		  AND (pf.id IS NULL OR `+policy.ReferencePredicate(params, "place", "pf.id")+`)
		  AND (pt.id IS NULL OR `+policy.ReferencePredicate(params, "place", "pt.id")+`)
		ORDER BY m.time_from NULLS LAST, m.time_to NULLS LAST, m.id
		LIMIT `+limitReference,
		params.Args()...)
	if err != nil {
		return TribeMovementResponse{}, err
	}
	defer rows.Close()

	response := TribeMovementResponse{
		TribeID: tribeUUID.String(),
		Events:  []TribeMovementEvent{},
		Limit:   MaxTribeMovementRows,
	}
	for rows.Next() {
		event := TribeMovementEvent{}
		var tribeName string
		var timeFrom, timeTo pgtype.Date
		var fromID, toID pgtype.Text
		var fromName, toName pgtype.Text
		var fromLng, fromLat, toLng, toLat pgtype.Float8
		if err := rows.Scan(&tribeName, &event.ID, &event.Status, &event.Kind, &event.Certainty,
			&timeFrom, &timeTo, &event.Notes,
			&event.SourceID, &event.SourceTitle, &event.SourceCitation,
			&fromID, &fromName, &fromLng, &fromLat,
			&toID, &toName, &toLng, &toLat); err != nil {
			return TribeMovementResponse{}, err
		}
		if response.TribeName == "" {
			response.TribeName = tribeName
		}
		event.TimeFrom = dateValue(timeFrom)
		event.TimeTo = dateValue(timeTo)
		event.Period = formatMovementPeriod(event.TimeFrom, event.TimeTo, event.Certainty)
		if fromID.Valid {
			event.FromPlace.ID = fromID.String
			event.FromPlace.Name = textValue(fromName)
			if fromLng.Valid {
				v := fromLng.Float64
				event.FromPlace.Longitude = &v
			}
			if fromLat.Valid {
				v := fromLat.Float64
				event.FromPlace.Latitude = &v
			}
		}
		if toID.Valid {
			event.ToPlace.ID = toID.String
			event.ToPlace.Name = textValue(toName)
			if toLng.Valid {
				v := toLng.Float64
				event.ToPlace.Longitude = &v
			}
			if toLat.Valid {
				v := toLat.Float64
				event.ToPlace.Latitude = &v
			}
		}
		response.Events = append(response.Events, event)
	}
	if err := rows.Err(); err != nil {
		return TribeMovementResponse{}, err
	}
	if len(response.Events) > MaxTribeMovementRows {
		response.Events = response.Events[:MaxTribeMovementRows]
		response.Truncated = true
	}
	return response, nil
}
