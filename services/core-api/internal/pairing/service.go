// Package pairing is the adjudication lane for machine-proposed external
// place pairings (plan 028-F). The rule the package exists to enforce: a
// candidate row is never rendered as a fact. The only fact-producing actions
// are the moderator's explicit decisions, and each decision's effect is
// exactly the effect the reviewer chose — alias writes a historical name,
// link writes a crosswalk, reject and defer write nothing but the decision.
package pairing

import (
	"context"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("pairing database is unavailable")
	ErrUnauthenticated     = errors.New("pairing lane requires a signed-in session")
	ErrForbidden           = errors.New("pairing lane is moderator-readable")
	ErrNotFound            = errors.New("pairing candidate not found")
	ErrValidation          = errors.New("pairing input is invalid")
	ErrAlreadyDecided      = errors.New("pairing candidate already decided")
	ErrCannotReopen        = errors.New("pairing candidate cannot be reopened")
)

// Effects are the reviewer’s explicit choices. The status they produce is the
// materialisation of the choice, not a separate decision.
const (
	EffectAlias  = "alias"
	EffectLink   = "link"
	EffectReject = "reject"
	EffectDefer  = "defer"
	EffectReopen = "reopen"
)

const ListLimit = 100
const MaxDecisionNoteRunes = 1000

type Candidate struct {
	ID                   string   `json:"id"`
	SourceID             string   `json:"sourceId"`
	SourceTitle          string   `json:"sourceTitle,omitempty"`
	SourceLicense        string   `json:"sourceLicense,omitempty"`
	ExternalKey          string   `json:"externalKey"`
	OccurrenceID         string   `json:"occurrenceId"`
	OurPlaceID           string   `json:"ourPlaceId,omitempty"`
	OurPlaceName         string   `json:"ourPlaceName,omitempty"`
	NormalizedCandidates string   `json:"normalizedCandidates"`
	Longitude            *float64 `json:"longitude,omitempty"`
	Latitude             *float64 `json:"latitude,omitempty"`
	NameSimilarity       float64  `json:"nameSimilarity"`
	DistanceKm           *float64 `json:"distanceKm,omitempty"`
	Rank                 *int     `json:"rank,omitempty"`
	CoordinateDeltaKm    *float64 `json:"coordinateDeltaKm,omitempty"`
	Status               string   `json:"status"`
	DecidedBy            string   `json:"decidedBy,omitempty"`
	DecidedAt            string   `json:"decidedAt,omitempty"`
	DecisionNoteAR       string   `json:"decisionNoteAr,omitempty"`
	CreatedAt            string   `json:"createdAt"`
}

type ListParams struct {
	Status  string
	Query   string
	PlaceID string
}

type DecisionHistoryEvent struct {
	ID           string `json:"id"`
	CandidateID  string `json:"candidateId"`
	ReviewerID   string `json:"reviewerId"`
	ReviewerName string `json:"reviewerName,omitempty"`
	Action       string `json:"action"`
	FromStatus   string `json:"fromStatus"`
	ToStatus     string `json:"toStatus"`
	NoteAR       string `json:"noteAr,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

type HistoryResponse struct {
	CandidateID string                 `json:"candidateId"`
	History     []DecisionHistoryEvent `json:"history"`
	Limit       int                    `json:"limit"`
	Truncated   bool                   `json:"truncated"`
}

type ListResponse struct {
	Candidates []Candidate `json:"candidates"`
	Status     string      `json:"status"`
	Limit      int         `json:"limit"`
	Truncated  bool        `json:"truncated"`
}

type DecideResponse struct {
	Candidate Candidate `json:"candidate"`
	Effect    string    `json:"effect"`
}

type ExternalID struct {
	PlaceID        string `json:"placeId"`
	SourceID       string `json:"sourceId"`
	SourceTitle    string `json:"sourceTitle,omitempty"`
	ExternalKey    string `json:"externalKey"`
	DecidedBy      string `json:"decidedBy,omitempty"`
	DecidedAt      string `json:"decidedAt,omitempty"`
	DecisionNoteAR string `json:"decisionNoteAr,omitempty"`
}

type ExternalIDsResponse struct {
	PlaceID     string       `json:"placeId"`
	ExternalIDs []ExternalID `json:"externalIds"`
	Limit       int          `json:"limit"`
	Truncated   bool         `json:"truncated"`
}

type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) actorModerator(ctx context.Context, actorID string) (uuid.UUID, bool, error) {
	if s.Pool == nil {
		return uuid.Nil, false, ErrDatabaseUnavailable
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil || actorUUID == uuid.Nil {
		return uuid.Nil, false, ErrUnauthenticated
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, actorUUID).Scan(&exists); err != nil {
		return uuid.Nil, false, err
	}
	if !exists {
		return uuid.Nil, false, ErrUnauthenticated
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return uuid.Nil, false, err
	}
	return actorUUID, allowed, nil
}

// List returns one status lane of proposals, bounded. Moderator-only.
func (s *Service) List(ctx context.Context, actorID, status string) (ListResponse, error) {
	return s.ListWithParams(ctx, actorID, ListParams{Status: status})
}

// ListWithParams returns one status lane with an optional text search and one
// optional our-place filter. The text search matches external keys, candidate
// readings, occurrence ids, source titles and our canonical place names —
// the fields a moderator actually scans for.
func (s *Service) ListWithParams(ctx context.Context, actorID string, params ListParams) (ListResponse, error) {
	var err error
	var allowed bool
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return ListResponse{}, err
	}
	if !allowed {
		return ListResponse{}, ErrForbidden
	}
	if params.Status == "" {
		params.Status = "suggested"
	}
	switch params.Status {
	case "suggested", "accepted", "alias_only", "rejected", "deferred":
	default:
		return ListResponse{}, ErrValidation
	}
	queryText := strings.TrimSpace(params.Query)
	var placeUUID pgtype.UUID
	if strings.TrimSpace(params.PlaceID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(params.PlaceID))
		if err != nil {
			return ListResponse{}, ErrValidation
		}
		placeUUID = pgtype.UUID{Bytes: parsed, Valid: true}
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT c.id, c.source_id, COALESCE(src.title_ar, ''), COALESCE(src.metadata->>'license', ''), c.external_key, c.occurrence_id,
		       COALESCE(c.our_place_id::text, ''), COALESCE(p.canonical_name_ar, ''),
		       c.normalized_candidates, ST_X(c.coordinates), ST_Y(c.coordinates),
		       c.name_similarity, c.distance_km, c.rank, c.coordinate_delta_km, c.status,
		       COALESCE(c.decided_by::text, ''), COALESCE(c.decided_at::text, ''),
		       COALESCE(c.decision_note_ar, ''), c.created_at
		FROM external_place_candidates c
		LEFT JOIN sources src ON src.id = c.source_id
		LEFT JOIN places p ON p.id = c.our_place_id
		WHERE c.status = $1
		  AND ($2::text = '' OR c.external_key ILIKE '%' || $2 || '%'
		    OR c.occurrence_id ILIKE '%' || $2 || '%'
		    OR c.normalized_candidates ILIKE '%' || $2 || '%'
		    OR src.title_ar ILIKE '%' || $2 || '%' OR p.canonical_name_ar ILIKE '%' || $2 || '%')
		  AND ($3::uuid IS NULL OR c.our_place_id = $3)
		ORDER BY c.rank NULLS LAST, c.created_at, c.id
		LIMIT $4`, params.Status, queryText, placeUUID, ListLimit+1)
	if err != nil {
		return ListResponse{}, err
	}
	defer rows.Close()

	response := ListResponse{Candidates: []Candidate{}, Status: params.Status, Limit: ListLimit}
	for rows.Next() {
		var candidate Candidate
		var lng, lat pgtype.Float8
		var dist, delta pgtype.Float8
		var rank pgtype.Int4
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&candidate.ID, &candidate.SourceID, &candidate.SourceTitle, &candidate.SourceLicense,
			&candidate.ExternalKey, &candidate.OccurrenceID,
			&candidate.OurPlaceID, &candidate.OurPlaceName,
			&candidate.NormalizedCandidates, &lng, &lat,
			&candidate.NameSimilarity, &dist, &rank, &delta, &candidate.Status,
			&candidate.DecidedBy, &candidate.DecidedAt,
			&candidate.DecisionNoteAR, &createdAt); err != nil {
			return ListResponse{}, err
		}
		if lng.Valid {
			v := lng.Float64
			candidate.Longitude = &v
		}
		if lat.Valid {
			v := lat.Float64
			candidate.Latitude = &v
		}
		if dist.Valid {
			v := dist.Float64
			candidate.DistanceKm = &v
		}
		if delta.Valid {
			v := delta.Float64
			candidate.CoordinateDeltaKm = &v
		}
		if rank.Valid {
			v := int(rank.Int32)
			candidate.Rank = &v
		}
		if createdAt.Valid {
			candidate.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		response.Candidates = append(response.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return ListResponse{}, err
	}
	if len(response.Candidates) > ListLimit {
		response.Candidates = response.Candidates[:ListLimit]
		response.Truncated = true
	}
	return response, nil
}

// Decide applies one explicit effect to a suggested candidate.
func (s *Service) Decide(ctx context.Context, actorID, candidateID, effect, note string) (DecideResponse, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return DecideResponse{}, err
	}
	if !allowed {
		return DecideResponse{}, ErrForbidden
	}
	candidateUUID, err := uuid.Parse(candidateID)
	if err != nil {
		return DecideResponse{}, ErrValidation
	}
	switch effect {
	case EffectAlias, EffectLink, EffectReject, EffectDefer:
	default:
		return DecideResponse{}, ErrValidation
	}
	if err := validateNote(note); err != nil {
		return DecideResponse{}, err
	}
	note = strings.TrimSpace(note)

	var (
		sourceID, externalKey, occurrence string
		placeID                           pgtype.UUID
		reading, status                   string
	)
	switch effect {
	case EffectAlias, EffectLink:
		// The effect needs a target place to land on. A candidate without
		// one can only be rejected or deferred, never accepted into a vacuum.
		err = s.Pool.QueryRow(ctx, `
			SELECT source_id::text, external_key, occurrence_id, our_place_id, normalized_candidates, status
			FROM external_place_candidates WHERE id = $1::uuid`, candidateUUID.String()).Scan(&sourceID, &externalKey, &occurrence, &placeID, &reading, &status)
	default:
		err = s.Pool.QueryRow(ctx, `
			SELECT source_id::text, external_key, occurrence_id, our_place_id, normalized_candidates, status
			FROM external_place_candidates WHERE id = $1::uuid`, candidateUUID.String()).Scan(&sourceID, &externalKey, &occurrence, &placeID, &reading, &status)
	}
	if err != nil {
		return DecideResponse{}, ErrNotFound
	}
	if status != "suggested" {
		return DecideResponse{}, ErrAlreadyDecided
	}
	if (effect == EffectAlias || effect == EffectLink) && !placeID.Valid {
		return DecideResponse{}, ErrValidation
	}

	nextStatus := map[string]string{
		EffectAlias:  "alias_only",
		EffectLink:   "accepted",
		EffectReject: "rejected",
		EffectDefer:  "deferred",
	}[effect]

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DecideResponse{}, err
	}
	defer tx.Rollback(ctx)
	if effect == EffectAlias {
		normalized := identity.NormalizeArabicName(reading)
		if normalized == "" {
			return DecideResponse{}, ErrValidation
		}
		if _, err := tx.Exec(ctx, `INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar, name_type, source_id)
			VALUES ($1, $2, $3, 'alternative', $4)
			ON CONFLICT (place_id, name_type, normalized_name_ar) DO NOTHING`,
			uuidString(placeID), reading, normalized, sourceID); err != nil {
			return DecideResponse{}, err
		}
	}
	if effect == EffectLink {
		if _, err := tx.Exec(ctx, `INSERT INTO place_external_ids (place_id, source_id, external_key, decided_by, decided_at, decision_note_ar)
			VALUES ($1, $2, $3, $4, now(), $5)
			ON CONFLICT (place_id, source_id, external_key) DO NOTHING`,
			uuidString(placeID), sourceID, externalKey, reviewer, note); err != nil {
			return DecideResponse{}, err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE external_place_candidates
		SET status = $1, decided_by = $2, decided_at = now(), decision_note_ar = $3, updated_at = now()
		WHERE id = $4 AND status = 'suggested'`, nextStatus, reviewer, note, candidateUUID)
	if err != nil {
		return DecideResponse{}, err
	}
	if tag.RowsAffected() != 1 {
		return DecideResponse{}, ErrAlreadyDecided
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pairing_decision_history (candidate_id, reviewer_id, action, from_status, to_status, note_ar)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		candidateUUID, reviewer, effect, status, nextStatus, note); err != nil {
		return DecideResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DecideResponse{}, err
	}

	reloaded, err := s.getCandidate(ctx, candidateUUID)
	if err != nil {
		return DecideResponse{}, err
	}
	return DecideResponse{Candidate: reloaded, Effect: effect}, nil
}

func validateNote(note string) error {
	if len([]rune(note)) > MaxDecisionNoteRunes {
		return ErrValidation
	}
	return nil
}

func (s *Service) Reopen(ctx context.Context, actorID, candidateID, note string) (DecideResponse, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return DecideResponse{}, err
	}
	if !allowed {
		return DecideResponse{}, ErrForbidden
	}
	candidateUUID, err := uuid.Parse(candidateID)
	if err != nil {
		return DecideResponse{}, ErrValidation
	}
	if err := validateNote(note); err != nil {
		return DecideResponse{}, err
	}
	note = strings.TrimSpace(note)

	var fromStatus string
	if err := s.Pool.QueryRow(ctx, `SELECT status FROM external_place_candidates WHERE id = $1::uuid`, candidateUUID.String()).Scan(&fromStatus); err != nil {
		return DecideResponse{}, ErrNotFound
	}
	// Only no-effect decisions reopen. A candidate accepted as an alias or
	// link produced a fact rows; silently returning it to `suggested` would
	// not retract that fact, so the moderator lane keeps it decided.
	if fromStatus != "rejected" && fromStatus != "deferred" {
		return DecideResponse{}, ErrCannotReopen
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DecideResponse{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE external_place_candidates
		SET status = 'suggested', decided_by = NULL, decided_at = NULL, decision_note_ar = $1, updated_at = now()
		WHERE id = $2 AND status IN ('rejected', 'deferred')`, note, candidateUUID)
	if err != nil {
		return DecideResponse{}, err
	}
	if tag.RowsAffected() != 1 {
		return DecideResponse{}, ErrCannotReopen
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pairing_decision_history (candidate_id, reviewer_id, action, from_status, to_status, note_ar)
		VALUES ($1, $2, 'reopen', $3, 'suggested', $4)`,
		candidateUUID, reviewer, fromStatus, note); err != nil {
		return DecideResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DecideResponse{}, err
	}
	reopened, err := s.getCandidate(ctx, candidateUUID)
	if err != nil {
		return DecideResponse{}, err
	}
	return DecideResponse{Candidate: reopened, Effect: EffectReopen}, nil
}

// History returns the append-only audit trail for one candidate. Moderator-only:
// the lane itself is the same authority that made the decisions.
func (s *Service) History(ctx context.Context, actorID, candidateID string) (HistoryResponse, error) {
	var err error
	var allowed bool
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return HistoryResponse{}, err
	}
	if !allowed {
		return HistoryResponse{}, ErrForbidden
	}
	candidateUUID, err := uuid.Parse(candidateID)
	if err != nil {
		return HistoryResponse{}, ErrValidation
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM external_place_candidates WHERE id = $1::uuid)`, candidateUUID.String()).Scan(&exists); err != nil {
		return HistoryResponse{}, err
	}
	if !exists {
		return HistoryResponse{}, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT h.id, h.candidate_id, h.reviewer_id, COALESCE(u.display_name_ar, ''), h.action, h.from_status, h.to_status, COALESCE(h.note_ar, ''), h.created_at
		FROM pairing_decision_history h
		LEFT JOIN users u ON u.id = h.reviewer_id
		WHERE h.candidate_id = $1::uuid
		ORDER BY h.created_at, h.id
		LIMIT $2`, candidateUUID.String(), ListLimit+1)
	if err != nil {
		return HistoryResponse{}, err
	}
	defer rows.Close()
	response := HistoryResponse{CandidateID: candidateUUID.String(), History: []DecisionHistoryEvent{}, Limit: ListLimit}
	for rows.Next() {
		event := DecisionHistoryEvent{}
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&event.ID, &event.CandidateID, &event.ReviewerID, &event.ReviewerName, &event.Action, &event.FromStatus, &event.ToStatus, &event.NoteAR, &createdAt); err != nil {
			return HistoryResponse{}, err
		}
		if createdAt.Valid {
			event.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		response.History = append(response.History, event)
	}
	if err := rows.Err(); err != nil {
		return HistoryResponse{}, err
	}
	if len(response.History) > ListLimit {
		response.History = response.History[:ListLimit]
		response.Truncated = true
	}
	return response, nil
}

func (s *Service) getCandidate(ctx context.Context, id uuid.UUID) (Candidate, error) {
	var candidate Candidate
	var lng, lat pgtype.Float8
	var dist, delta pgtype.Float8
	var rank pgtype.Int4
	var createdAt pgtype.Timestamptz
	err := s.Pool.QueryRow(ctx, `
		SELECT c.id, c.source_id, COALESCE(src.title_ar, ''), COALESCE(src.metadata->>'license', ''), c.external_key, c.occurrence_id,
		       COALESCE(c.our_place_id::text, ''), COALESCE(p.canonical_name_ar, ''),
		       c.normalized_candidates, ST_X(c.coordinates), ST_Y(c.coordinates),
		       c.name_similarity, c.distance_km, c.rank, c.coordinate_delta_km, c.status,
		       COALESCE(c.decided_by::text, ''), COALESCE(c.decided_at::text, ''),
		       COALESCE(c.decision_note_ar, ''), c.created_at
		FROM external_place_candidates c
		LEFT JOIN sources src ON src.id = c.source_id
		LEFT JOIN places p ON p.id = c.our_place_id
		WHERE c.id = $1`, id).Scan(&candidate.ID, &candidate.SourceID, &candidate.SourceTitle, &candidate.SourceLicense,
		&candidate.ExternalKey, &candidate.OccurrenceID,
		&candidate.OurPlaceID, &candidate.OurPlaceName,
		&candidate.NormalizedCandidates, &lng, &lat,
		&candidate.NameSimilarity, &dist, &rank, &delta, &candidate.Status,
		&candidate.DecidedBy, &candidate.DecidedAt,
		&candidate.DecisionNoteAR, &createdAt)
	if err != nil {
		return Candidate{}, ErrNotFound
	}
	if lng.Valid {
		v := lng.Float64
		candidate.Longitude = &v
	}
	if lat.Valid {
		v := lat.Float64
		candidate.Latitude = &v
	}
	if dist.Valid {
		v := dist.Float64
		candidate.DistanceKm = &v
	}
	if delta.Valid {
		v := delta.Float64
		candidate.CoordinateDeltaKm = &v
	}
	if rank.Valid {
		v := int(rank.Int32)
		candidate.Rank = &v
	}
	if createdAt.Valid {
		candidate.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	return candidate, nil
}

// ExternalIDs returns the accepted crosswalk for one place, bounded, visible
// only through sources the caller may read (SourcePredicate). A place the
// caller cannot read has no readable crosswalk either.
func (s *Service) ExternalIDs(ctx context.Context, placeID, actorID string) (ExternalIDsResponse, error) {
	if s.Pool == nil {
		return ExternalIDsResponse{}, ErrDatabaseUnavailable
	}
	placeUUID, err := uuid.Parse(placeID)
	if err != nil {
		return ExternalIDsResponse{}, ErrValidation
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return ExternalIDsResponse{}, err
	}
	params := visibility.NewParams()
	placeReference := params.Add(placeUUID.String())
	limitReference := params.Add(ListLimit + 1)

	rows, err := s.Pool.Query(ctx, `
		SELECT x.place_id, x.source_id, COALESCE(src.title_ar, ''), x.external_key,
		       COALESCE(x.decided_by::text, ''), COALESCE(x.decided_at::text, ''),
		       COALESCE(x.decision_note_ar, '')
		FROM place_external_ids x
		JOIN places p ON p.id = x.place_id
		LEFT JOIN sources src ON src.id = x.source_id
		WHERE x.place_id = `+placeReference+`
		  AND `+policy.ReferencePredicate(params, "place", "p.id")+`
		  AND `+policy.SourcePredicate(params, "src.id")+`
		ORDER BY src.title_ar NULLS LAST, x.external_key
		LIMIT `+limitReference,
		params.Args()...)
	if err != nil {
		return ExternalIDsResponse{}, err
	}
	defer rows.Close()

	response := ExternalIDsResponse{PlaceID: placeUUID.String(), ExternalIDs: []ExternalID{}, Limit: ListLimit}
	for rows.Next() {
		var item ExternalID
		if err := rows.Scan(&item.PlaceID, &item.SourceID, &item.SourceTitle, &item.ExternalKey,
			&item.DecidedBy, &item.DecidedAt, &item.DecisionNoteAR); err != nil {
			return ExternalIDsResponse{}, err
		}
		response.ExternalIDs = append(response.ExternalIDs, item)
	}
	if err := rows.Err(); err != nil {
		return ExternalIDsResponse{}, err
	}
	if len(response.ExternalIDs) > ListLimit {
		response.ExternalIDs = response.ExternalIDs[:ListLimit]
		response.Truncated = true
	}
	return response, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
