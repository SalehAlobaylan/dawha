package regionreview

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AtlasClaimReceiptInput struct {
	ClaimID string `json:"claimId"`
	NoteAR  string `json:"noteAr"`
}

// AttachAtlasClaimReceipt adds the published reading as contextual evidence.
// The existing claim supplies the assertion, subject, status and dates.
func (s *Service) AttachAtlasClaimReceipt(ctx context.Context, actorID, occurrenceID string, input AtlasClaimReceiptInput) error {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	occurrence, err := uuid.Parse(occurrenceID)
	if err != nil {
		return ErrValidation
	}
	claim, err := uuid.Parse(input.ClaimID)
	note := strings.TrimSpace(input.NoteAR)
	if err != nil || note == "" || utf8.RuneCountInString(note) > 1000 {
		return ErrValidation
	}

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	access, err := policy.Claim(ctx, tx, claim)
	if err != nil {
		return err
	}
	if !access.Allowed() {
		return ErrNotFound
	}

	var source uuid.UUID
	var passage *uuid.UUID
	var original, locator string
	err = tx.QueryRow(ctx, `
		SELECT i.source_id, p.id, o.original_text, r.locator_ar || ' · ' || r.external_url
		FROM atlas_source_occurrences o
		JOIN atlas_source_item_revisions r ON r.id = o.item_revision_id
		JOIN atlas_source_items i ON i.id = r.item_id
		JOIN sources s ON s.id = i.source_id AND s.visibility = 'public'
		LEFT JOIN source_passages p ON p.id = o.source_passage_id AND p.source_id = i.source_id
		WHERE o.id = $1 AND o.published FOR UPDATE OF o`, occurrence).Scan(&source, &passage, &original, &locator)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var existing bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM atlas_claim_receipts WHERE occurrence_id=$1 AND claim_id=$2)`,
		occurrence, claim).Scan(&existing)
	if err != nil || existing {
		return err
	}

	var statement, evidence uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO source_statements (source_id, source_passage_id, statement_text_ar, locator_ar,
		       extraction_method, review_status, created_by)
		VALUES ($1,$2,$3,$4,'manual','accepted',$5) RETURNING id`,
		source, passage, original, locator, reviewer).Scan(&statement)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO claim_evidence (claim_id, source_statement_id, source_passage_id, evidence_note_ar, relation, created_by)
		VALUES ($1,$2,$3,$4,'contextualizes',$5) RETURNING id`, claim, statement, passage, note, reviewer).Scan(&evidence)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO atlas_claim_receipts (occurrence_id, claim_id, evidence_id) VALUES ($1,$2,$3)`,
		occurrence, claim, evidence)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
