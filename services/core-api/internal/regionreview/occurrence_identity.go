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

type OccurrenceIdentityInput struct {
	TribeID  string `json:"tribeId"`
	PlaceID  string `json:"placeId"`
	BranchID string `json:"branchId"`
	NoteAR   string `json:"noteAr"`
}

func (s *Service) ReviewOccurrenceIdentity(ctx context.Context, actorID, occurrenceID string, input OccurrenceIdentityInput) error {
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
	note := strings.TrimSpace(input.NoteAR)
	if note == "" || utf8.RuneCountInString(note) > 1000 {
		return ErrValidation
	}
	var tribe, place, branch *uuid.UUID
	var targetCount int
	for _, target := range []struct {
		raw string
		dst **uuid.UUID
	}{{input.TribeID, &tribe}, {input.PlaceID, &place}, {input.BranchID, &branch}} {
		if target.raw == "" {
			continue
		}
		id, err := uuid.Parse(target.raw)
		if err != nil {
			return ErrValidation
		}
		*target.dst = &id
		targetCount++
	}
	if targetCount > 1 {
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
	var sourceID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT i.source_id
		FROM atlas_source_occurrences o
		JOIN atlas_source_item_revisions r ON r.id = o.item_revision_id
		JOIN atlas_source_items i ON i.id = r.item_id
		WHERE o.id = $1 FOR UPDATE OF o`, occurrence).Scan(&sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	access, err := policy.Source(ctx, tx, sourceID)
	if err != nil {
		return err
	}
	if !access.Allowed() {
		return ErrForbidden
	}
	tag, err := tx.Exec(ctx, `
		UPDATE atlas_source_occurrences SET tribe_id = $2, place_id = $3, branch_id = $4
		WHERE id = $1 AND ($2::uuid IS NULL OR EXISTS (
		    SELECT 1 FROM tribes WHERE id = $2 AND visibility = 'public'
		)) AND ($3::uuid IS NULL OR EXISTS (
		    SELECT 1 FROM places WHERE id = $3 AND visibility = 'public'
		)) AND ($4::uuid IS NULL OR EXISTS (
		    SELECT 1 FROM branches WHERE id = $4 AND visibility = 'public'
		))`, occurrence, tribe, place, branch)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrValidation
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO atlas_occurrence_identity_decisions (occurrence_id, reviewer_id, tribe_id, place_id, branch_id, note_ar)
		VALUES ($1, $2, $3, $4, $5, $6)`, occurrence, reviewer, tribe, place, branch, note)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
