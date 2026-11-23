// Package tribehints is the record-hint lane of the tribal dossiers (plan
// 031-C). The ingest lanes land passages that quote tribes; this lane matches
// those passages against the dictionary's own names and carries the matches
// as SUGGESTIONS in a bounded queue. The rules it keeps, in order:
//
//   - hints never auto-apply: a match is a proposal, and its only road to a
//     documented receipt is a named reviewer's accept, which writes a claim
//     (predicate `mentioned_in`) with the passage as its contextualizing
//     evidence — exactly the receipt the dossier's badge trio counts;
//   - a dismissal is REVIEWER-SCOPED: it hides the card from the dismissing
//     reviewer and from nobody else, and it cannot overwrite an accepted hint;
//   - the card is the queue's own unit — a (tribe, source) pair, worded the
//     way the plan's own example puts it («١٧ إشارة لحرب في Lorimer 1908»); a
//     per-passage row would flood the queue with the same claim nine times.
package tribehints

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("tribe hints database is unavailable")
	ErrUnauthenticated     = errors.New("tribe hints require a signed-in session")
	ErrForbidden           = errors.New("tribe hints lane is moderator-readable")
	ErrNotFound            = errors.New("tribe hint not found")
	ErrValidation          = errors.New("tribe hints input is invalid")
)

const (
	// CardLimit bounds one hint-queue read; HintPassagesLimit bounds one
	// card's passage window. Both say when they cut.
	CardLimit         = 100
	HintPassagesLimit = 20
	MaxNoteRunes      = 1000
)

type HintCard struct {
	TribeID     string `json:"tribeId"`
	TribeName   string `json:"tribeName"`
	SourceID    string `json:"sourceId"`
	SourceTitle string `json:"sourceTitle"`
	// PassageCount is the pending passages behind the card; Mentions is the
	// sum of the forms each passage matched — the «١٧ إشارة» number.
	PassageCount int    `json:"passageCount"`
	Mentions     int    `json:"mentions"`
	Status       string `json:"status"`
	DecidedBy    string `json:"decidedBy,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

type HintListResponse struct {
	Hints     []HintCard `json:"hints"`
	TribeID   string     `json:"tribeId,omitempty"`
	Status    string     `json:"status"`
	Limit     int        `json:"limit"`
	Truncated bool       `json:"truncated"`
}

// HintPassage is one passage of a card: where the mention sits and what
// matched, verbatim — the receipt's own words, not just a count.
type HintPassage struct {
	PassageID    string   `json:"passageId"`
	PageNumber   *int     `json:"pageNumber,omitempty"`
	Locator      string   `json:"locator,omitempty"`
	PassageText  string   `json:"passageText,omitempty"`
	MatchedForms []string `json:"matchedForms"`
	HintID       string   `json:"hintId"`
	Status       string   `json:"status"`
}

type HintCardDetail struct {
	Card      HintCard      `json:"card"`
	Passages  []HintPassage `json:"passages"`
	Limit     int           `json:"limit"`
	Truncated bool          `json:"truncated"`
}

type MatchResult struct {
	SourceID       string              `json:"sourceId"`
	HintsSuggested int                 `json:"hintsSuggested"`
	MatchedTribes  int                 `json:"matchedTribes"`
	PerTribe       []MatchedTribeTally `json:"perTribe"`
}

type MatchedTribeTally struct {
	TribeID   string `json:"tribeId"`
	TribeName string `json:"tribeName"`
	Mentions  int    `json:"mentions"`
}

// CardDecision is what a reviewer's answer wrote: the card, the action, and —
// for an accept — the documented claim the receipts now hang from.
type CardDecision struct {
	Card            HintCard `json:"card"`
	Action          string   `json:"action"`
	CreatedClaimID  string   `json:"createdClaimId,omitempty"`
	CreatedReceipts int      `json:"createdReceipts"`
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

// MatchTribeHints runs the deterministic mention matcher over ONE source's
// passages: the dictionary's own forms (canonical + aliases, normalized)
// matched against a passage's normalized text, its raw text, and its own
// Arabic locator — the ingest's entry index, so an English-OCR'd passage
// whose locator names its entry (the way 030-B's Lorimer rows carry
// «بداية مدخل حرب») still earns its mention. One hint row per (tribe,
// passage), carrying the forms it matched. Idempotent by construction: the
// unique key plus ON CONFLICT DO NOTHING — a re-run adds the forms only new
// aliases carry and never touches a decided hint's state. A measured edge,
// stated rather than hidden: a ta-marbuta form matches raw 'ة' spellings only
// through the normalized column, because the SAME normalizer that wrote the
// passages' normalized_text_ar also normalized the dictionary's forms — a
// raw-column hit is a bonus, never the basis.
func (s *Service) MatchTribeHints(ctx context.Context, sourceID string) (MatchResult, error) {
	if s.Pool == nil {
		return MatchResult{}, ErrDatabaseUnavailable
	}
	sourceUUID, err := uuid.Parse(sourceID)
	if err != nil {
		return MatchResult{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return MatchResult{}, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		WITH forms AS (
			SELECT tr.id AS tribe_id, tr.canonical_name_ar AS form,
			       tr.normalized_name_ar AS normalized_form
			FROM tribes tr
			WHERE tr.normalized_name_ar <> ''
			UNION ALL
			SELECT tr.id, ta.value_ar, ta.normalized_value_ar
			FROM tribes tr JOIN tribe_aliases ta ON ta.tribe_id = tr.id
			WHERE ta.normalized_value_ar <> ''
		)
		INSERT INTO tribe_passage_hints (tribe_id, source_passage_id, matched_forms)
		SELECT form.tribe_id, sp.id, ARRAY_REMOVE(ARRAY_AGG(DISTINCT form.form), NULL)
		FROM source_passages sp
		JOIN forms form ON (position(form.normalized_form IN sp.normalized_text_ar) > 0
		                 OR position(form.normalized_form IN sp.text_ar) > 0
		                 OR position(form.normalized_form IN COALESCE(sp.locator_ar, '')) > 0)
		WHERE sp.source_id = $1
		GROUP BY form.tribe_id, sp.id
		ON CONFLICT (tribe_id, source_passage_id) DO NOTHING`, sourceUUID)
	if err != nil {
		return MatchResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MatchResult{}, err
	}

	result := MatchResult{SourceID: sourceUUID.String(), HintsSuggested: int(tag.RowsAffected())}
	rows, err := s.Pool.Query(ctx, `
		SELECT tr.id::text, tr.canonical_name_ar, count(*)::int
		FROM tribe_passage_hints h
		JOIN tribes tr ON tr.id = h.tribe_id
		JOIN source_passages sp ON sp.id = h.source_passage_id
		WHERE sp.source_id = $1
		GROUP BY tr.id, tr.canonical_name_ar
		ORDER BY tr.canonical_name_ar`, sourceUUID)
	if err != nil {
		return MatchResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tally MatchedTribeTally
		if err := rows.Scan(&tally.TribeID, &tally.TribeName, &tally.Mentions); err != nil {
			return MatchResult{}, err
		}
		result.MatchedTribes++
		result.PerTribe = append(result.PerTribe, tally)
	}
	if err := rows.Err(); err != nil {
		return MatchResult{}, err
	}
	return result, nil
}

// List returns the hint queue as cards — one per (tribe, source) pair, each
// with its pending passage count and its mention tally — bounded, optionally
// scoped to one tribe, and WITHOUT the cards the calling reviewer dismissed:
// a dismissal is their own hide, and the card stays for everyone else.
func (s *Service) List(ctx context.Context, actorID, tribeID, status string) (HintListResponse, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return HintListResponse{}, err
	}
	if !allowed {
		return HintListResponse{}, ErrForbidden
	}
	if status == "" {
		status = "suggested"
	}
	switch status {
	case "suggested", "accepted":
	default:
		return HintListResponse{}, ErrValidation
	}
	var tribeReference pgtype.UUID
	if strings.TrimSpace(tribeID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(tribeID))
		if err != nil {
			return HintListResponse{}, ErrValidation
		}
		tribeReference = pgtype.UUID{Bytes: parsed, Valid: true}
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT tr.id::text, tr.canonical_name_ar, src.id::text, COALESCE(src.title_ar, ''),
		       count(*)::int,
		       (sum(cardinality(h.matched_forms)))::int,
		       h.status,
		       COALESCE(min(h.decided_by::text), ''),
		       min(h.created_at)
		FROM tribe_passage_hints h
		JOIN tribes tr ON tr.id = h.tribe_id
		JOIN source_passages sp ON sp.id = h.source_passage_id
		JOIN sources src ON src.id = sp.source_id
		WHERE h.status = $1
		  AND ($2::uuid IS NULL OR tr.id = $2)
		  AND NOT EXISTS (SELECT 1 FROM tribe_passage_hint_decisions d
		                  WHERE d.tribe_id = h.tribe_id AND d.source_id = src.id
		                    AND d.reviewer_id = $3 AND d.action = 'dismiss')
		GROUP BY tr.id, tr.canonical_name_ar, src.id, src.title_ar, h.status
		ORDER BY min(h.created_at), tr.canonical_name_ar
		LIMIT $4`, status, tribeReference, reviewer, CardLimit+1)
	if err != nil {
		return HintListResponse{}, err
	}
	defer rows.Close()

	response := HintListResponse{Hints: []HintCard{}, Status: status, Limit: CardLimit}
	if tribeReference.Valid {
		response.TribeID = uuid.UUID(tribeReference.Bytes).String()
	}
	for rows.Next() {
		var card HintCard
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&card.TribeID, &card.TribeName, &card.SourceID, &card.SourceTitle,
			&card.PassageCount, &card.Mentions, &card.Status, &card.DecidedBy, &createdAt); err != nil {
			return HintListResponse{}, err
		}
		if createdAt.Valid {
			card.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		response.Hints = append(response.Hints, card)
	}
	if err := rows.Err(); err != nil {
		return HintListResponse{}, err
	}
	if len(response.Hints) > CardLimit {
		response.Hints = response.Hints[:CardLimit]
		response.Truncated = true
	}
	return response, nil
}

// Detail returns one card's passages, bounded: what matched where, verbatim,
// so the reviewer reads the mention itself before deciding.
func (s *Service) Detail(ctx context.Context, actorID, tribeID, sourceID string) (HintCardDetail, error) {
	_, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return HintCardDetail{}, err
	}
	if !allowed {
		return HintCardDetail{}, ErrForbidden
	}
	tribeUUID, err := uuid.Parse(tribeID)
	if err != nil {
		return HintCardDetail{}, ErrValidation
	}
	sourceUUID, err := uuid.Parse(sourceID)
	if err != nil {
		return HintCardDetail{}, ErrValidation
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT h.id::text, h.status, sp.id::text, sp.page_number, COALESCE(sp.locator_ar, ''),
		       COALESCE(sp.text_ar, ''), ARRAY(SELECT DISTINCT unnest(h.matched_forms))
		FROM tribe_passage_hints h
		JOIN source_passages sp ON sp.id = h.source_passage_id
		WHERE h.tribe_id = $1 AND sp.source_id = $2
		ORDER BY sp.sequence_number NULLS LAST, h.created_at
		LIMIT $3`, tribeUUID, sourceUUID, HintPassagesLimit+1)
	if err != nil {
		return HintCardDetail{}, err
	}
	defer rows.Close()

	detail := HintCardDetail{Passages: []HintPassage{}, Limit: HintPassagesLimit}
	for rows.Next() {
		var passage HintPassage
		var matchedForms []string
		var pageNumber pgtype.Int4
		if err := rows.Scan(&passage.HintID, &passage.Status, &passage.PassageID, &pageNumber,
			&passage.Locator, &passage.PassageText, &matchedForms); err != nil {
			return HintCardDetail{}, err
		}
		if pageNumber.Valid {
			page := int(pageNumber.Int32)
			passage.PageNumber = &page
		}
		passage.MatchedForms = matchedForms
		detail.Passages = append(detail.Passages, passage)
	}
	if err := rows.Err(); err != nil {
		return HintCardDetail{}, err
	}
	if len(detail.Passages) > HintPassagesLimit {
		detail.Passages = detail.Passages[:HintPassagesLimit]
		detail.Truncated = true
	}

	tribeName, sourceTitle, err := s.cardIdentity(ctx, tribeUUID, sourceUUID)
	if err != nil {
		return HintCardDetail{}, err
	}
	detail.Card = HintCard{TribeID: tribeUUID.String(), TribeName: tribeName, SourceID: sourceUUID.String(), SourceTitle: sourceTitle}
	return detail, nil
}

func (s *Service) cardIdentity(ctx context.Context, tribeID, sourceID uuid.UUID) (string, string, error) {
	var tribeName string
	if err := s.Pool.QueryRow(ctx, `SELECT canonical_name_ar FROM tribes WHERE id = $1`, tribeID).Scan(&tribeName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	var sourceTitle string
	if err := s.Pool.QueryRow(ctx, `SELECT COALESCE(title_ar, '') FROM sources WHERE id = $1`, sourceID).Scan(&sourceTitle); err != nil {
		return "", "", err
	}
	return tribeName, sourceTitle, nil
}

// Decide applies one reviewer's answer to a whole card (a (tribe, source)
// pair). An accept writes the documented receipt — a claim of predicate
// `mentioned_in` with the card's suggested passages as contextualizing
// evidence, all in the same transaction as the hint rows' own acceptance —
// and a dismiss records the reviewer's answer and hides the card for THEM
// only; the hint rows stay suggested so another reviewer still sees the work.
func (s *Service) Decide(ctx context.Context, actorID, tribeID, sourceID, action, note string) (CardDecision, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return CardDecision{}, err
	}
	if !allowed {
		return CardDecision{}, ErrForbidden
	}
	tribeUUID, err := uuid.Parse(tribeID)
	if err != nil {
		return CardDecision{}, ErrValidation
	}
	sourceUUID, err := uuid.Parse(sourceID)
	if err != nil {
		return CardDecision{}, ErrValidation
	}
	switch action {
	case "accept", "dismiss":
	default:
		return CardDecision{}, ErrValidation
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > MaxNoteRunes {
		return CardDecision{}, ErrValidation
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return CardDecision{}, err
	}
	defer tx.Rollback(ctx)

	var hintCount, pendingCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE h.status = 'suggested')
		FROM tribe_passage_hints h
		WHERE h.tribe_id = $1
		  AND h.source_passage_id IN (SELECT sp.id FROM source_passages sp WHERE sp.source_id = $2)`,
		tribeUUID, sourceUUID).Scan(&hintCount, &pendingCount); err != nil {
		return CardDecision{}, err
	}
	if hintCount == 0 {
		return CardDecision{}, ErrNotFound
	}
	if pendingCount == 0 {
		return CardDecision{}, ErrValidation
	}

	createdClaim := ""
	createdReceipts := 0
	if action == "accept" {
		claimID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO claims
			(id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
			SELECT $1, 'tribe', tr.id, 'mentioned_in', 'source', src.id, 'documented',
			       'إشارة موثقة من طابور التلميحات — إثبات مسمّى من مراجع',
			       $2
			FROM tribes tr, sources src
			WHERE tr.id = $3 AND src.id = $4`, claimID, reviewer, tribeUUID, sourceUUID); err != nil {
			return CardDecision{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO claim_evidence (id, claim_id, source_passage_id, relation, created_by)
			SELECT gen_random_uuid(), $1, h.source_passage_id, 'contextualizes', $2
			FROM tribe_passage_hints h
			WHERE h.tribe_id = $3 AND h.status = 'suggested'
			  AND h.source_passage_id IN (SELECT sp.id FROM source_passages sp WHERE sp.source_id = $4)`,
			claimID, reviewer, tribeUUID, sourceUUID); err != nil {
			return CardDecision{}, err
		}
		tag, err := tx.Exec(ctx, `UPDATE tribe_passage_hints
			SET status = 'accepted', decided_by = $1, decided_at = now(), updated_at = now()
			WHERE tribe_id = $2 AND status = 'suggested'
			  AND source_passage_id IN (SELECT sp.id FROM source_passages sp WHERE sp.source_id = $3)`,
			reviewer, tribeUUID, sourceUUID)
		if err != nil {
			return CardDecision{}, err
		}
		createdReceipts = int(tag.RowsAffected())
		createdClaim = claimID.String()
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tribe_passage_hint_decisions
		(hint_id, tribe_id, source_id, reviewer_id, action, note_ar)
		SELECT h.id, h.tribe_id, $1, $2, $3, $4
		FROM tribe_passage_hints h
		WHERE h.tribe_id = $5 AND h.source_passage_id IN (SELECT sp.id FROM source_passages sp WHERE sp.source_id = $6)`,
		sourceUUID, reviewer, action, note, tribeUUID, sourceUUID); err != nil {
		return CardDecision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CardDecision{}, err
	}

	tribeName, sourceTitle, err := s.cardIdentity(ctx, tribeUUID, sourceUUID)
	if err != nil {
		return CardDecision{}, err
	}
	status := "suggested"
	decidedBy := ""
	if action == "accept" {
		status = "accepted"
		decidedBy = reviewer.String()
	}
	card := HintCard{TribeID: tribeUUID.String(), TribeName: tribeName, SourceID: sourceUUID.String(), SourceTitle: sourceTitle, Status: status, PassageCount: hintCount, DecidedBy: decidedBy}
	return CardDecision{Card: card, Action: action, CreatedClaimID: createdClaim, CreatedReceipts: createdReceipts}, nil
}
