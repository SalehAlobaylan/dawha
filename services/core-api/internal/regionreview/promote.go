package regionreview

import (
	"context"
	"errors"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type PromotionResponse struct {
	Promoted    bool   `json:"promoted"`
	CandidateID string `json:"candidateId"`
	RegionID    string `json:"regionId,omitempty"`
	LabelAR     string `json:"labelAr,omitempty"`
	EraLabel    string `json:"eraLabel,omitempty"`
}

type BoundaryReviewInput struct {
	SourceID     string `json:"sourceId"`
	Locator      string `json:"locator"`
	WKT          string `json:"wkt"`
	ValidFrom    string `json:"validFrom"`
	ValidTo      string `json:"validTo"`
	OccurrenceID string `json:"occurrenceId"`
}

// ReviewBoundary records a separately sourced boundary under the moderator's
// identity. Promotion requires two moderators to submit identical evidence.
func (s *Service) ReviewBoundary(ctx context.Context, actorID, candidateID string, input BoundaryReviewInput) error {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	if _, err := uuid.Parse(candidateID); err != nil {
		return ErrValidation
	}
	if _, err := uuid.Parse(input.SourceID); err != nil {
		return ErrValidation
	}
	locator := strings.TrimSpace(input.Locator)
	if locator == "" || len([]rune(locator)) > 500 || len(input.WKT) > 12000 {
		return ErrValidation
	}
	for _, raw := range []string{input.ValidFrom, input.ValidTo} {
		if raw != "" {
			if _, err := time.Parse(time.DateOnly, raw); err != nil {
				return ErrValidation
			}
		}
	}
	if input.ValidFrom != "" && input.ValidTo != "" && input.ValidFrom > input.ValidTo {
		return ErrValidation
	}
	if input.OccurrenceID != "" {
		if _, err := uuid.Parse(input.OccurrenceID); err != nil {
			return ErrValidation
		}
	}
	if err := s.requirePlateReadable(ctx, input.SourceID, actorID); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var alreadyPublished bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM historical_regions r WHERE r.candidate_id = c.id AND r.boundary_reviewed)
		FROM external_region_candidates c WHERE c.id = $1 FOR UPDATE`, candidateID).Scan(&alreadyPublished)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if alreadyPublished {
		return ErrAlreadyDecided
	}
	if input.OccurrenceID != "" {
		var belongs bool
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (
			    SELECT 1 FROM atlas_source_occurrences o
			    JOIN atlas_source_item_revisions r ON r.id = o.item_revision_id
			    JOIN atlas_source_items i ON i.id = r.item_id
			    WHERE o.id = $1 AND i.source_id = $2 AND o.published
			)`, input.OccurrenceID, input.SourceID).Scan(&belongs)
		if err != nil {
			return err
		}
		if !belongs {
			return ErrValidation
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO region_boundary_reviews (candidate_id, reviewer_id, source_id, locator, boundary, valid_from, valid_to, occurrence_id)
		VALUES ($1, $2, $3, $4, ST_GeomFromText($5, 4326), NULLIF($6::text, '')::date,
                NULLIF($7::text, '')::date, NULLIF($8::text, '')::uuid)
		ON CONFLICT (candidate_id, reviewer_id) DO UPDATE
		SET source_id = EXCLUDED.source_id, locator = EXCLUDED.locator,
		    boundary = EXCLUDED.boundary, valid_from = EXCLUDED.valid_from, valid_to = EXCLUDED.valid_to,
            occurrence_id = EXCLUDED.occurrence_id, reviewed_at = now()`, candidateID, reviewer, input.SourceID, locator, input.WKT, input.ValidFrom, input.ValidTo, input.OccurrenceID)
	var dbErr *pgconn.PgError
	if errors.As(err, &dbErr) && (dbErr.Code == "23514" || dbErr.Code == "23503" || dbErr.Code == "XX000" || dbErr.Code == "22023") {
		return ErrValidation
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) PromoteCandidate(ctx context.Context, actorID, candidateID string) (PromotionResponse, error) {
	id, err := uuid.Parse(candidateID)
	if err != nil {
		return PromotionResponse{}, ErrValidation
	}
	_, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return PromotionResponse{}, err
	}
	if !allowed {
		return PromotionResponse{}, ErrForbidden
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PromotionResponse{}, err
	}
	defer tx.Rollback(ctx)
	result := PromotionResponse{CandidateID: id.String()}
	err = tx.QueryRow(ctx, `
		SELECT label_text_ar, COALESCE(era_label, '')
		FROM external_region_candidates WHERE id = $1 AND status = 'accepted' FOR UPDATE`, id).
		Scan(&result.LabelAR, &result.EraLabel)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(result.LabelAR) == "" {
		return result, ErrValidation
	}

	err = tx.QueryRow(ctx, `
		SELECT id::text FROM historical_regions WHERE candidate_id = $1 AND boundary_reviewed`, id).Scan(&result.RegionID)
	if err == nil {
		result.Promoted = true
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO historical_regions
			(id, name_ar, normalized_name_ar, geometry, certainty, source_id, candidate_id, boundary_reviewed,
             valid_from, valid_to, boundary_locator_ar, boundary_occurrence_id)
		SELECT gen_random_uuid(), $2, $3, a.boundary, 'approximate', a.source_id, a.candidate_id, true,
		       a.valid_from, a.valid_to, a.locator, a.occurrence_id
		FROM region_boundary_reviews a
		JOIN region_boundary_reviews b ON b.candidate_id = a.candidate_id AND b.reviewer_id > a.reviewer_id
		  AND b.source_id = a.source_id AND b.locator = a.locator AND ST_Equals(b.boundary, a.boundary)
		  AND b.valid_from IS NOT DISTINCT FROM a.valid_from AND b.valid_to IS NOT DISTINCT FROM a.valid_to
		  AND b.occurrence_id IS NOT DISTINCT FROM a.occurrence_id
		JOIN sources src ON src.id = a.source_id AND src.visibility = 'public'
		WHERE a.candidate_id = $1
		ORDER BY a.reviewed_at DESC, b.reviewed_at DESC, a.reviewer_id, b.reviewer_id
		LIMIT 1 RETURNING id::text`, id, result.LabelAR, identity.NormalizeArabicName(result.LabelAR)).Scan(&result.RegionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrValidation
	}
	if err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `UPDATE external_region_candidates SET promoted_region_id = $2 WHERE id = $1`, id, result.RegionID); err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	result.Promoted = true
	return result, nil
}
