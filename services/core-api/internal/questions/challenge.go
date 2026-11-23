// الطعون — the challenge flow over the dossier's claims (plan 031-D).
//
// The rule the flow keeps: a challenge is a DISPUTE THREAD opened by a
// reader, using the existing dispute machinery (disputes + dispute_claims +
// the audit trail). The claim's status flips to `disputed` inside the
// challenge's own transaction, so every surface that reads the claim's row
// shows the dispute the moment it commits — the dossier's chip, the badge
// row, the disputed-claims index, the map's disputed register — there is no
// second state to drift out of sync.
//
// The adjudication closes the thread: a MODERATOR (not the challenge's own
// author, and not the loose collaboration set) settles the challenged claims
// with an explicit outcome (documented / rejected) and the resolution text.
// The dispute's own lifecycle (under_review, reopened → re-adjudicate) stays
// the existing machinery's; no dispute table is changed.
package questions

import (
	"context"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ChallengeClaimInput carries the طعن's own words: what the claim says and
// why the challenger disputes it. The adjudication answers this, side by side
// with the claim's evidence pair.
type ChallengeClaimInput struct {
	TitleAR       string `json:"titleAr"`
	DescriptionAR string `json:"descriptionAr"`
}

// AdjudicateChallengeInput settles a challenge: the outcome the challenged
// claims land in and the resolution's own words. `documented` keeps the claim
// (upheld); `rejected` retires it (overturned).
type AdjudicateChallengeInput struct {
	ClaimStatus  string `json:"claimStatus"`
	ResolutionAR string `json:"resolutionAr"`
}

// ChallengeClaim opens a dispute thread against one readable claim and flips
// the claim's own status to `disputed` in the same transaction. Any signed-in
// reader may challenge a documented claim; the thread is what the
// adjudication answers.
func (s *Service) ChallengeClaim(ctx context.Context, claimID, actorID string, input ChallengeClaimInput) (DisputeDetail, error) {
	if err := s.ready(); err != nil {
		return DisputeDetail{}, err
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return DisputeDetail{}, ErrForbidden
	}
	claimUUID, err := uuid.Parse(strings.TrimSpace(claimID))
	if err != nil {
		return DisputeDetail{}, ErrValidation
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return DisputeDetail{}, err
	}
	// The claim's reference is added BEFORE the predicate: the predicate's own
	// placeholder count rides on this one, and the visibility builder owns the
	// numbering.
	params := visibility.NewParams()
	claimReference := params.Add(claimUUID.String())
	claimPredicate := policy.ClaimPredicate(params, "c.id")
	var status string
	if err := s.Pool.QueryRow(ctx, `SELECT status FROM claims c WHERE c.id = `+claimReference+` AND `+claimPredicate,
		params.Args()...).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisputeDetail{}, ErrNotFound
		}
		return DisputeDetail{}, err
	}
	// A challenge answers a claim someone reviewed into a positive state. A
	// rejected/superseded claim needs no thread (its state is the answer), and
	// a disputed one has its thread — the dispute's own lifecycle owns both.
	if status != "documented" && status != "supported" {
		return DisputeDetail{}, ErrValidation
	}
	normalized, err := validateDisputeInput(CreateDisputeInput{TitleAR: input.TitleAR, DescriptionAR: input.DescriptionAR}, true)
	if err != nil {
		return DisputeDetail{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DisputeDetail{}, err
	}
	defer tx.Rollback(ctx)
	disputeID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO disputes (id, title_ar, description_ar, status, created_by)
		VALUES ($1, $2, NULLIF($3, ''), 'open', $4)`, disputeID, normalized.TitleAR, normalized.DescriptionAR, actorUUID); err != nil {
		return DisputeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO dispute_claims (dispute_id, claim_id, position) VALUES ($1, $2, 'opposes')`, disputeID, claimUUID); err != nil {
		return DisputeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE claims SET status = 'disputed', updated_at = now()
		WHERE id = $1 AND status IN ('documented', 'supported')`, claimUUID); err != nil {
		return DisputeDetail{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "claim_challenged", "claim", claimUUID, map[string]string{"from_status": status}, normalized); err != nil {
		return DisputeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DisputeDetail{}, err
	}
	return s.GetDispute(ctx, disputeID.String())
}

// AdjudicateChallenge resolves a challenged dispute and settles the claims the
// challenge opposed. Moderator-only: the challenger never closes their own
// thread, and the collaboration set never adjudicates — the review lanes'
// two-role rule is what decides here, like it decides everywhere the claim's
// standing changes.
func (s *Service) AdjudicateChallenge(ctx context.Context, disputeID, actorID string, input AdjudicateChallengeInput) (DisputeDetail, error) {
	if err := s.ready(); err != nil {
		return DisputeDetail{}, err
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return DisputeDetail{}, ErrForbidden
	}
	disputeUUID, err := uuid.Parse(strings.TrimSpace(disputeID))
	if err != nil {
		return DisputeDetail{}, ErrValidation
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return DisputeDetail{}, err
	}
	if !allowed {
		return DisputeDetail{}, ErrForbidden
	}
	outcome := strings.TrimSpace(input.ClaimStatus)
	resolution := strings.TrimSpace(input.ResolutionAR)
	if (outcome != "documented" && outcome != "rejected") || resolution == "" || len([]rune(resolution)) > 5000 {
		return DisputeDetail{}, ErrValidation
	}
	current, err := s.disputeByID(ctx, s.Pool, disputeUUID)
	if err != nil {
		return DisputeDetail{}, err
	}
	if current.Status != "open" && current.Status != "under_review" && current.Status != "reopened" {
		return DisputeDetail{}, ErrValidation
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DisputeDetail{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE disputes
		SET status = 'resolved', resolution_ar = NULLIF($1, ''), resolved_by = $2, updated_at = now()
		WHERE id = $3`, resolution, actorUUID, disputeUUID); err != nil {
		return DisputeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE claims SET status = $1, updated_at = now()
		WHERE status = 'disputed'
		  AND id IN (SELECT claim_id FROM dispute_claims WHERE dispute_id = $2 AND position = 'opposes')`,
		outcome, disputeUUID); err != nil {
		return DisputeDetail{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "dispute_adjudicated", "dispute", disputeUUID,
		map[string]string{"from_status": current.Status, "claim_outcome": outcome},
		AdjudicateChallengeInput{ClaimStatus: outcome, ResolutionAR: resolution}); err != nil {
		return DisputeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DisputeDetail{}, err
	}
	return s.GetDispute(ctx, disputeUUID.String())
}

// ClaimDispute reads the most recent dispute thread against one claim — the
// atlas's disputed-polygon side-by-side read (plan 032-D). The thread and its
// claims ride the actor's own visibility; a claim with no dispute thread yet,
// or one the actor cannot read, is the honest empty answer (ErrNotFound),
// never a half-visible thread.
func (s *Service) ClaimDispute(ctx context.Context, claimID, actorID string) (DisputeDetail, error) {
	if err := s.ready(); err != nil {
		return DisputeDetail{}, err
	}
	claimUUID, err := uuid.Parse(strings.TrimSpace(claimID))
	if err != nil {
		return DisputeDetail{}, ErrValidation
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return DisputeDetail{}, err
	}
	params := visibility.NewParams()
	claimReference := params.Add(claimUUID.String())
	claimPredicate := policy.ClaimPredicate(params, "c.id")
	var disputeID uuid.UUID
	err = s.Pool.QueryRow(ctx, `
		SELECT d.id FROM disputes d
		JOIN dispute_claims dc ON dc.dispute_id = d.id
		JOIN claims c ON c.id = dc.claim_id
		WHERE c.id = `+claimReference+` AND `+claimPredicate+`
		ORDER BY d.created_at DESC
		LIMIT 1`, params.Args()...).Scan(&disputeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisputeDetail{}, ErrNotFound
		}
		return DisputeDetail{}, err
	}
	return s.GetDispute(ctx, disputeID.String())
}
