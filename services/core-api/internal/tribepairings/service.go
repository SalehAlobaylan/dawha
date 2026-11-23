// Package tribepairings is مِسلك التكرار — the tribe-duplicate lane (plan
// 031-E). Two tribes whose names read alike are a crosswalk problem, not a
// merge: plan 005's recorded deferral stands, and the package's whole charter
// is the negative space of a merge. The candidate carries its measured
// components and both matched forms; the reviewer's allowed effects are exactly
// three — link the loser's form as an alias of the chosen canonical (an
// INSERT into tribe_aliases), point the loser at the canonical (an append-only
// entity_relationship row), or reject the pairing — and NO statement here
// deletes or updates a `tribes` row. The undo of anything this lane writes is
// data, not surgery.
package tribepairings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("tribe pairings database is unavailable")
	ErrUnauthenticated     = errors.New("tribe pairings lane requires a signed-in session")
	ErrForbidden           = errors.New("tribe pairings lane is moderator-readable")
	ErrNotFound            = errors.New("tribe duplicate candidate not found")
	ErrValidation          = errors.New("tribe pairings input is invalid")
	ErrAlreadyDecided      = errors.New("tribe duplicate candidate already decided")
)

// Actions are the lane's three effects, the decision table's own CHECK.
const (
	ActionLinkAlias = "link_alias" // the loser's form became an alias of the canonical
	ActionCanonize  = "canonize"   // an append-only pointer relationship
	ActionReject    = "reject"
)

const (
	ListLimit    = 100
	MaxNoteRunes = 1000
)

type Candidate struct {
	ID         string  `json:"id"`
	TribeAID   string  `json:"tribeAId"`
	TribeAName string  `json:"tribeAName,omitempty"`
	TribeBID   string  `json:"tribeBId"`
	TribeBName string  `json:"tribeBName,omitempty"`
	MatchedA   string  `json:"matchedA"`
	MatchedB   string  `json:"matchedB"`
	Similarity float64 `json:"nameSimilarity"`
	Status     string  `json:"status"`
	DecidedBy  string  `json:"decidedBy,omitempty"`
	DecidedAt  string  `json:"decidedAt,omitempty"`
	NoteAR     string  `json:"decisionNoteAr,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	Canonical  string  `json:"canonical,omitempty"`
}

type ListResponse struct {
	Candidates []Candidate `json:"candidates"`
	Status     string      `json:"status"`
	Limit      int         `json:"limit"`
	Truncated  bool        `json:"truncated"`
}

type DecideInput struct {
	// Canonical is the tribe the effects serve: the one whose identity the
	// other joins. Required for link_alias and canonize; forbidden for reject.
	Canonical string
	NoteAR    string
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
	Diff         string `json:"diff,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

type HistoryResponse struct {
	CandidateID string                 `json:"candidateId"`
	History     []DecisionHistoryEvent `json:"history"`
	Limit       int                    `json:"limit"`
	Truncated   bool                   `json:"truncated"`
}

type DecideResponse struct {
	Candidate Candidate `json:"candidate"`
	Action    string    `json:"action"`
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

const candidateColumns = `
	c.id::text, c.tribe_a_id::text, COALESCE(ta.canonical_name_ar, ''),
	c.tribe_b_id::text, COALESCE(tb.canonical_name_ar, ''),
	c.matched_a, c.matched_b, c.name_similarity, c.status,
	COALESCE(c.decided_by::text, ''), c.decided_at,
	COALESCE(c.decision_note_ar, ''), c.created_at`

func scanCandidate(scan func(dest ...any) error) (Candidate, error) {
	var candidate Candidate
	var decidedAt, createdAt pgtype.Timestamptz
	if err := scan(&candidate.ID, &candidate.TribeAID, &candidate.TribeAName,
		&candidate.TribeBID, &candidate.TribeBName,
		&candidate.MatchedA, &candidate.MatchedB, &candidate.Similarity, &candidate.Status,
		&candidate.DecidedBy, &decidedAt,
		&candidate.NoteAR, &createdAt); err != nil {
		return Candidate{}, err
	}
	if decidedAt.Valid {
		candidate.DecidedAt = decidedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	if createdAt.Valid {
		candidate.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	return candidate, nil
}

// List returns one status lane of duplicate candidates, bounded, ordered by
// the measured similarity (the queue's own ordering: the likeliest pairs
// first).
func (s *Service) List(ctx context.Context, actorID, status string) (ListResponse, error) {
	_, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return ListResponse{}, err
	}
	if !allowed {
		return ListResponse{}, ErrForbidden
	}
	if status == "" {
		status = "suggested"
	}
	switch status {
	case "suggested", "linked", "pointed", "rejected":
	default:
		return ListResponse{}, ErrValidation
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT `+candidateColumns+`
		FROM tribe_duplicate_candidates c
		LEFT JOIN tribes ta ON ta.id = c.tribe_a_id
		LEFT JOIN tribes tb ON tb.id = c.tribe_b_id
		WHERE c.status = $1
		ORDER BY c.name_similarity DESC, c.created_at, c.id
		LIMIT $2`, status, ListLimit+1)
	if err != nil {
		return ListResponse{}, err
	}
	defer rows.Close()

	response := ListResponse{Candidates: []Candidate{}, Status: status, Limit: ListLimit}
	for rows.Next() {
		candidate, err := scanCandidate(rows.Scan)
		if err != nil {
			return ListResponse{}, err
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

// Decide applies one of the lane's three effects to a suggested candidate. The
// canonical is the reviewer's pick and it MUST be one of the pair; the effects
// write an alias or a pointer relationship and then the candidate's own state,
// in one transaction, and nothing anywhere writes a `tribes` row.
func (s *Service) Decide(ctx context.Context, actorID, candidateID, action, canonical, note string) (DecideResponse, error) {
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
	switch action {
	case ActionLinkAlias, ActionCanonize, ActionReject:
	default:
		return DecideResponse{}, ErrValidation
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > MaxNoteRunes {
		return DecideResponse{}, ErrValidation
	}

	var tribeA, tribeB, status string
	if err := s.Pool.QueryRow(ctx, `
		SELECT tribe_a_id::text, tribe_b_id::text, status
		FROM tribe_duplicate_candidates WHERE id = $1`, candidateUUID).Scan(&tribeA, &tribeB, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DecideResponse{}, ErrNotFound
		}
		return DecideResponse{}, err
	}
	toStatus := map[string]string{ActionLinkAlias: "linked", ActionCanonize: "pointed", ActionReject: "rejected"}[action]

	diff := map[string]string{}
	var loserForm, loserNormalized string
	if action == ActionReject {
		if strings.TrimSpace(canonical) != "" {
			return DecideResponse{}, ErrValidation
		}
		diff["outcome"] = "rejected"
	} else {
		canonical = strings.TrimSpace(canonical)
		// The canonical must be one of the pair — pointing the loser at a
		// third party would be a merge by another name.
		var side string
		if canonical == tribeA {
			side = "b"
		} else if canonical == tribeB {
			side = "a"
		} else {
			return DecideResponse{}, ErrValidation
		}
		loserOfID := loserOf(canonical, tribeA, tribeB)
		loserForm, loserNormalized = s.candidateForm(ctx, candidateUUID, side)
		if action == ActionLinkAlias && (loserForm == "" || loserNormalized == "") {
			return DecideResponse{}, ErrValidation
		}
		diff["canonical"] = canonical
		diff["loser"] = loserOfID
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DecideResponse{}, err
	}
	defer tx.Rollback(ctx)

	// The effects, before the state: an alias INSERT and a relationship
	// INSERT — the lane's own vocabulary of non-merge actions. A merge (an
	// UPDATE or a DELETE on tribes) has no statement here by design.
	if action == ActionLinkAlias {
		if _, err := tx.Exec(ctx, `INSERT INTO tribe_aliases (tribe_id, value_ar, normalized_value_ar)
			VALUES ($1, $2, $3)
			ON CONFLICT (tribe_id, normalized_value_ar) DO NOTHING`, canonical, loserForm, loserNormalized); err != nil {
			return DecideResponse{}, err
		}
	}
	if action == ActionCanonize {
		if _, err := tx.Exec(ctx, `INSERT INTO entity_relationships
			(subject_type, subject_id, predicate, object_type, object_id, status, created_by)
			VALUES ('tribe', $1, 'canonical_pointer', 'tribe', $2, 'documented', $3)
			ON CONFLICT DO NOTHING`, loserOf(canonical, tribeA, tribeB), canonical, reviewer); err != nil {
			return DecideResponse{}, err
		}
	}

	tag, err := tx.Exec(ctx, `UPDATE tribe_duplicate_candidates
		SET status = $2, decided_by = $3, decided_at = now(), decision_note_ar = $4, updated_at = now()
		WHERE id = $1 AND status = 'suggested'`, candidateUUID, toStatus, reviewer, note)
	if err != nil {
		return DecideResponse{}, err
	}
	if tag.RowsAffected() != 1 {
		return DecideResponse{}, ErrAlreadyDecided
	}
	diffBytes, err := json.Marshal(diff)
	if err != nil {
		return DecideResponse{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tribe_duplicate_decisions
		(candidate_id, reviewer_id, action, from_status, to_status, note_ar, diff)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		candidateUUID, reviewer, action, status, toStatus, note, string(diffBytes)); err != nil {
		return DecideResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DecideResponse{}, err
	}

	candidate, err := s.getCandidate(ctx, candidateUUID)
	if err != nil {
		return DecideResponse{}, err
	}
	if action != ActionReject {
		candidate.Canonical = canonical
	}
	return DecideResponse{Candidate: candidate, Action: action}, nil
}

// candidateForm reads one side's own matched form — the words the link-alias
// effect inserts as the canonical's new alias — and its normalized value.
func (s *Service) candidateForm(ctx context.Context, candidateID uuid.UUID, side string) (string, string) {
	column := "matched_a"
	if side == "b" {
		column = "matched_b"
	}
	var form string
	if err := s.Pool.QueryRow(ctx, `SELECT `+column+` FROM tribe_duplicate_candidates WHERE id = $1`, candidateID).Scan(&form); err != nil {
		return "", ""
	}
	normalized := identity.NormalizeArabicName(strings.TrimSpace(form))
	if normalized == "" {
		return "", ""
	}
	return form, normalized
}

func loserOf(canonical, tribeA, tribeB string) string {
	if canonical == tribeA {
		return tribeB
	}
	return tribeA
}

// History returns the append-only audit trail for one candidate. Moderator-only.
func (s *Service) History(ctx context.Context, actorID, candidateID string) (HistoryResponse, error) {
	_, allowed, err := s.actorModerator(ctx, actorID)
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
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tribe_duplicate_candidates WHERE id = $1)`, candidateUUID).Scan(&exists); err != nil {
		return HistoryResponse{}, err
	}
	if !exists {
		return HistoryResponse{}, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT d.id::text, d.candidate_id::text, d.reviewer_id::text, COALESCE(u.display_name_ar, ''),
		       d.action, d.from_status, d.to_status, COALESCE(d.note_ar, ''), COALESCE(d.diff::text, ''), d.created_at
		FROM tribe_duplicate_decisions d
		LEFT JOIN users u ON u.id = d.reviewer_id
		WHERE d.candidate_id = $1
		ORDER BY d.created_at, d.id
		LIMIT $2`, candidateUUID, ListLimit+1)
	if err != nil {
		return HistoryResponse{}, err
	}
	defer rows.Close()
	response := HistoryResponse{CandidateID: candidateUUID.String(), History: []DecisionHistoryEvent{}, Limit: ListLimit}
	for rows.Next() {
		event := DecisionHistoryEvent{}
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&event.ID, &event.CandidateID, &event.ReviewerID, &event.ReviewerName, &event.Action,
			&event.FromStatus, &event.ToStatus, &event.NoteAR, &event.Diff, &createdAt); err != nil {
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
	row := s.Pool.QueryRow(ctx, `
		SELECT `+candidateColumns+`
		FROM tribe_duplicate_candidates c
		LEFT JOIN tribes ta ON ta.id = c.tribe_a_id
		LEFT JOIN tribes tb ON tb.id = c.tribe_b_id
		WHERE c.id = $1`, id)
	candidate, err := scanCandidate(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Candidate{}, ErrNotFound
		}
		return Candidate{}, err
	}
	return candidate, nil
}
