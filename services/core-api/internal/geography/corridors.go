// Subject corridors (plan 033-B): the recorded routes' read side.
//
// The corridor is the 0063-contract table's own rows: ordered stops with the
// legs the sources name (day counts + camel/horse/foot/sea), the season it
// runs (صيف/شتاء), the years it operated, and a source that MUST exist (the
// plan's own acceptance — a corridor nobody sourced is an invented
// trajectory, refused at the schema's door and again here: the read joins the
// source and carries its title so the panel can show it).
//
// Bounded: LIMIT 20 corridors + truncated (the subject layer draws a
// handful; the response says when its own read was cut).
package geography

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
)

// MaxSubjectCorridors bounds one subject's corridor read.
const MaxSubjectCorridors = 20

// MovementCorridorStop is one ordered stop: its place, the leg INTO it (the
// day count + the mode the source names), and its own note.
type MovementCorridorStop struct {
	Sequence  int     `json:"sequence"`
	PlaceID   string  `json:"placeId"`
	PlaceName string  `json:"placeName"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	// The leg INTO this stop from the previous one: nil on the sequence's
	// own head — the schema keeps the head legless and the read says so.
	DayCountFromPrev *int   `json:"dayCountFromPrev,omitempty"`
	ModeFromPrev     string `json:"modeFromPrev,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

// MovementCorridorRow is one corridor with its ordered stops.
type MovementCorridorRow struct {
	ID          string                 `json:"id"`
	TitleAR     string                 `json:"titleAr"`
	Season      string                 `json:"season,omitempty"`
	ValidFrom   int                    `json:"validFrom,omitempty"`
	ValidTo     int                    `json:"validTo,omitempty"`
	Status      string                 `json:"status"`
	Certainty   string                 `json:"certainty"`
	SourceID    string                 `json:"sourceId"`
	SourceTitle string                 `json:"sourceTitle,omitempty"`
	Decided     bool                   `json:"decided"`
	Stops       []MovementCorridorStop `json:"stops"`
}

// SubjectCorridorsResponse is the bounded corridor read.
type SubjectCorridorsResponse struct {
	SubjectType string                `json:"subjectType"`
	SubjectID   string                `json:"subjectId"`
	Corridors   []MovementCorridorRow `json:"corridors"`
	Limit       int                   `json:"limit"`
	Truncated   bool                  `json:"truncated"`
}

// SubjectCorridors reads one subject's corridors; the optional season/mode
// filters (033-C) narrow it honestly: season matches the corridor's own
// column, mode matches ANY leg the stops carry. Empty = no filter — the
// read's own rows change nothing.
// The subject must itself be readable (the same policy kind the movement
// reads answer: person/tribe), the corridor's source too, and each stop's
// place — a stop whose place went private is hidden and the corridor's
// geometry says which legs survived (a polyline can only draw stops it can
// read).
func (s *Service) SubjectCorridors(ctx context.Context, subjectType, subjectID, actorID string, season, mode string) (SubjectCorridorsResponse, error) {
	if s.Pool == nil {
		return SubjectCorridorsResponse{}, ErrDatabaseUnavailable
	}
	subjectType = strings.TrimSpace(subjectType)
	if subjectType != "person" && subjectType != "tribe" && subjectType != "family" {
		return SubjectCorridorsResponse{}, ErrValidation
	}
	subjectUUID, err := uuid.Parse(subjectID)
	if err != nil {
		return SubjectCorridorsResponse{}, ErrValidation
	}

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return SubjectCorridorsResponse{}, err
	}
	params := visibility.NewParams()
	subjectReference := params.Add(subjectUUID.String())
	subjectPredicate := policy.ReferencePredicate(params, subjectType, "tr.id")
	if subjectType == "person" {
		subjectPredicate = policy.PersonPredicate(params, "tr.id")
	}
	limitReference := params.Add(MaxSubjectCorridors + 1)
	// The subject's real relation names: 'people' is NOT 'peoples' — the read
	// rides the schema's own plurals (person→people, tribe→tribes, family→families).
	subjectTable := map[string]string{"person": "people", "tribe": "tribes", "family": "families"}[subjectType]

	// The 033-C filter words, narrowed to their own vocabularies at the door:
	// anything else is a typo, not a season.
	if season != "summer" && season != "winter" {
		season = ""
	}
	if mode != "camel" && mode != "horse" && mode != "foot" && mode != "sea" {
		mode = ""
	}
	seasonClause := "TRUE"
	if season != "" {
		seasonClause = "m.season = " + params.Add(season)
	}
	modeClause := "TRUE"
	if mode != "" {
		modeClause = "EXISTS (SELECT 1 FROM movement_corridor_stops mode_stops WHERE mode_stops.corridor_id = m.id AND mode_stops.mode_from_prev = " + params.Add(mode) + ")"
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT m.id::text, COALESCE(m.title_ar, ''), COALESCE(m.season, ''),
		       COALESCE(m.valid_from, 0), COALESCE(m.valid_to, 0),
		       m.status, m.certainty,
		       COALESCE(src.id::text, ''), COALESCE(src.title_ar, ''),
		       m.decided_at IS NOT NULL,
		       COALESCE((
		           SELECT jsonb_agg(jsonb_build_object(
		               'sequence', st.sequence_number,
		               'placeId', st.place_id::text,
		               'placeName', p.canonical_name_ar,
		               'longitude', ST_X(p.geometry),
		               'latitude', ST_Y(p.geometry),
		               'dayCountFromPrev', st.day_count_from_prev,
		               'modeFromPrev', COALESCE(st.mode_from_prev, ''),
		               'notes', COALESCE(st.notes_ar, '')) ORDER BY st.sequence_number)
		           FROM movement_corridor_stops st
		           JOIN places p ON p.id = st.place_id
		           WHERE st.corridor_id = m.id
		             AND p.geometry IS NOT NULL
		             AND `+policy.ReferencePredicate(params, "place", "p.id")+`
		       ), '[]'::jsonb)
		FROM movement_corridors m
		JOIN sources src ON src.id = m.source_id
		LEFT JOIN LATERAL (
			SELECT tr.id FROM `+subjectTable+` tr WHERE tr.id = m.subject_id
		) tr ON TRUE
		WHERE m.subject_type = '`+subjectType+`' AND m.subject_id = `+subjectReference+`
		  AND tr.id IS NOT NULL
		  AND `+subjectPredicate+`
		  AND `+policy.SourcePredicate(params, "src.id")+`
		  AND `+seasonClause+`
		  AND `+modeClause+`
		ORDER BY m.created_at DESC, m.id
		LIMIT `+limitReference,
		params.Args()...)
	if err != nil {
		return SubjectCorridorsResponse{}, err
	}
	defer rows.Close()

	response := SubjectCorridorsResponse{
		SubjectType: subjectType,
		SubjectID:   subjectUUID.String(),
		Corridors:   []MovementCorridorRow{},
		Limit:       MaxSubjectCorridors,
	}
	for rows.Next() {
		row := MovementCorridorRow{}
		var stopsJSON []byte
		if err := rows.Scan(&row.ID, &row.TitleAR, &row.Season,
			&row.ValidFrom, &row.ValidTo,
			&row.Status, &row.Certainty,
			&row.SourceID, &row.SourceTitle,
			&row.Decided, &stopsJSON); err != nil {
			return SubjectCorridorsResponse{}, err
		}
		if err := json.Unmarshal(stopsJSON, &row.Stops); err != nil {
			return SubjectCorridorsResponse{}, err
		}
		response.Corridors = append(response.Corridors, row)
	}
	if err := rows.Err(); err != nil {
		return SubjectCorridorsResponse{}, err
	}
	if len(response.Corridors) > MaxSubjectCorridors {
		response.Corridors = response.Corridors[:MaxSubjectCorridors]
		response.Truncated = true
	}
	return response, nil
}
