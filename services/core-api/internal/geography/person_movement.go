package geography

import (
	"context"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dates"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxPersonMovementRows bounds one subject layer. A person's مسالِك used to
// be read as "every migration event the person owns", so its size grew with
// the corpus; a bounded window is what the atlas can draw, and the response
// says when it was cut (stipulation 6, the same contract the map keeps).
const MaxPersonMovementRows = 50

type personMovementPlace struct {
	ID        string   `json:"id,omitempty"`
	Name      string   `json:"name,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
}

type PersonMovementEvent struct {
	ID             string              `json:"id"`
	Status         string              `json:"status"`
	Certainty      string              `json:"certainty,omitempty"`
	TimeFrom       string              `json:"timeFrom,omitempty"`
	TimeTo         string              `json:"timeTo,omitempty"`
	Period         string              `json:"period"`
	Notes          string              `json:"notes,omitempty"`
	SourceID       string              `json:"sourceId,omitempty"`
	SourceTitle    string              `json:"sourceTitle,omitempty"`
	SourceCitation string              `json:"sourceCitation,omitempty"`
	FromPlace      personMovementPlace `json:"fromPlace,omitempty"`
	ToPlace        personMovementPlace `json:"toPlace,omitempty"`
}

type PersonMovementResponse struct {
	PersonID   string                `json:"personId"`
	PersonName string                `json:"personName,omitempty"`
	Events     []PersonMovementEvent `json:"events"`
	Limit      int                   `json:"limit"`
	Truncated  bool                  `json:"truncated"`
}

// PersonMovement reads one person's documented مسالِك under the reader's
// policy: the person must be readable (PersonPredicate), the documenting
// source must be readable (SourcePredicate — a source that goes private hides
// its events), and each endpoint place must be readable (ReferencePredicate,
// subject to NULL places). Ordered by time_from with undated rows last, and
// bounded by MaxPersonMovementRows + truncated.
func (s *Service) PersonMovement(ctx context.Context, personID string, actorID string) (PersonMovementResponse, error) {
	if s.Pool == nil {
		return PersonMovementResponse{}, ErrDatabaseUnavailable
	}
	personUUID, err := uuid.Parse(personID)
	if err != nil {
		return PersonMovementResponse{}, ErrValidation
	}

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return PersonMovementResponse{}, err
	}
	params := visibility.NewParams()
	personReference := params.Add(personUUID.String())
	limitReference := params.Add(MaxPersonMovementRows + 1)

	rows, err := s.Pool.Query(ctx, `
		SELECT p.canonical_name_ar,
		       m.id, m.status, COALESCE(m.certainty, ''),
		       m.time_from, m.time_to, COALESCE(m.notes_ar, ''),
		       COALESCE(src.id::text, ''), COALESCE(src.title_ar, ''), COALESCE(src.citation_ar, ''),
		       COALESCE(pf.id::text, ''), pf.canonical_name_ar,
		       ST_X(pf.geometry), ST_Y(pf.geometry),
		       COALESCE(pt.id::text, ''), pt.canonical_name_ar,
		       ST_X(pt.geometry), ST_Y(pt.geometry)
		FROM migration_events m
		JOIN people p ON p.id = m.subject_id
		LEFT JOIN places pf ON pf.id = m.from_place_id
		LEFT JOIN places pt ON pt.id = m.to_place_id
		LEFT JOIN sources src ON src.id = m.source_id
		WHERE m.subject_type = 'person' AND m.subject_id = `+personReference+`
		  AND m.status = 'documented'
		  AND `+policy.PersonPredicate(params, "p.id")+`
		  AND (m.source_id IS NULL OR `+policy.SourcePredicate(params, "src.id")+`)
		  AND (pf.id IS NULL OR `+policy.ReferencePredicate(params, "place", "pf.id")+`)
		  AND (pt.id IS NULL OR `+policy.ReferencePredicate(params, "place", "pt.id")+`)
		ORDER BY m.time_from NULLS LAST, m.time_to NULLS LAST, m.id
		LIMIT `+limitReference,
		params.Args()...)
	if err != nil {
		return PersonMovementResponse{}, err
	}
	defer rows.Close()

	response := PersonMovementResponse{
		PersonID: personUUID.String(),
		Events:   []PersonMovementEvent{},
		Limit:    MaxPersonMovementRows,
	}
	for rows.Next() {
		event := PersonMovementEvent{}
		var personName string
		var timeFrom, timeTo pgtype.Date
		var fromID, toID pgtype.Text
		var fromName, toName pgtype.Text
		var fromLng, fromLat, toLng, toLat pgtype.Float8
		if err := rows.Scan(&personName, &event.ID, &event.Status, &event.Certainty,
			&timeFrom, &timeTo, &event.Notes,
			&event.SourceID, &event.SourceTitle, &event.SourceCitation,
			&fromID, &fromName, &fromLng, &fromLat,
			&toID, &toName, &toLng, &toLat); err != nil {
			return PersonMovementResponse{}, err
		}
		if response.PersonName == "" {
			response.PersonName = personName
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
		return PersonMovementResponse{}, err
	}
	if len(response.Events) > MaxPersonMovementRows {
		response.Events = response.Events[:MaxPersonMovementRows]
		response.Truncated = true
	}
	return response, nil
}

func formatMovementPeriod(from, to, certainty string) string {
	if from == "" && to == "" {
		return "غير محددة"
	}
	// Reuse the one date contract so the subject layer, the map and the tree
	// never invent three renderings of one row.
	return dates.FormatRangeWithCertainty(from, to, certainty)
}
