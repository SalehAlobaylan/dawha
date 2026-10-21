package sourceprocessing

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const MaxManualPageTextRunes = 200000

// SavePageReview appends a reviewer transcription without changing machine
// readings or any candidate already extracted from them. The evidence row lock
// serializes revisions and ExpectedRevision rejects stale browser forms.
func (s *Service) SavePageReview(ctx context.Context, fileID, actorID string, pageNumber int, input PageReviewInput) (PageReviewRevision, error) {
	if err := s.databaseReady(); err != nil {
		return PageReviewRevision{}, err
	}
	if pageNumber < 1 || pageNumber > MaxPages {
		return PageReviewRevision{}, ErrValidation
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return PageReviewRevision{}, err
	}
	input, err = validatePageReviewInput(input)
	if err != nil {
		return PageReviewRevision{}, err
	}
	expectedPageEvidenceID, err := uuid.Parse(input.ExpectedPageEvidenceID)
	if err != nil {
		return PageReviewRevision{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PageReviewRevision{}, err
	}
	defer tx.Rollback(ctx)

	var pageEvidenceID, sourceID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		SELECT p.id, p.source_id
		FROM source_page_evidence p
		WHERE p.source_file_id = $1 AND p.page_number = $2
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT 1
		FOR UPDATE OF p
	`, fileUUID, pageNumber).Scan(&pageEvidenceID, &sourceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PageReviewRevision{}, ErrNotFound
		}
		return PageReviewRevision{}, err
	}
	allowed, err := canReviewSource(ctx, tx, uuidValue(sourceID), actorUUID)
	if err != nil {
		return PageReviewRevision{}, err
	}
	if !allowed {
		return PageReviewRevision{}, ErrForbidden
	}
	if expectedPageEvidenceID != uuidValue(pageEvidenceID) {
		return PageReviewRevision{}, ErrConflict
	}
	var latestRevision int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(revision_number), 0)
		FROM source_page_review_revisions
		WHERE source_page_evidence_id = $1
	`, uuidValue(pageEvidenceID)).Scan(&latestRevision); err != nil {
		return PageReviewRevision{}, err
	}
	if input.ExpectedRevision != latestRevision {
		return PageReviewRevision{}, ErrConflict
	}

	newRevision := latestRevision + 1
	revisionID := uuid.New()
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO source_page_review_revisions
			(id, source_page_evidence_id, revision_number, reviewer_id, status, transcription_text_ar, note_ar)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
		RETURNING created_at
	`, revisionID, uuidValue(pageEvidenceID), newRevision, actorUUID, input.Status, input.TextAR, input.NoteAR).Scan(&createdAt); err != nil {
		return PageReviewRevision{}, err
	}
	textDigest := sha256.Sum256([]byte(input.TextAR))
	if err := writeAudit(ctx, tx, &actorUUID, "source_page_transcription_reviewed", "source_page_review_revision", revisionID,
		map[string]any{"pageEvidenceId": uuidValue(pageEvidenceID).String(), "revisionNumber": latestRevision},
		map[string]any{"pageEvidenceId": uuidValue(pageEvidenceID).String(), "revisionNumber": newRevision, "status": input.Status, "textSha256": fmt.Sprintf("%x", textDigest[:])},
	); err != nil {
		return PageReviewRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PageReviewRevision{}, err
	}
	return PageReviewRevision{
		ID:                   revisionID.String(),
		SourcePageEvidenceID: uuidValue(pageEvidenceID).String(),
		RevisionNumber:       newRevision,
		Status:               input.Status,
		TextAR:               input.TextAR,
		NoteAR:               input.NoteAR,
		ReviewerID:           actorUUID.String(),
		CreatedAt:            createdAt,
	}, nil
}

func validatePageReviewInput(input PageReviewInput) (PageReviewInput, error) {
	input.ExpectedPageEvidenceID = strings.TrimSpace(input.ExpectedPageEvidenceID)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.TextAR = strings.TrimSpace(input.TextAR)
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if input.ExpectedPageEvidenceID == "" || input.ExpectedRevision < 0 || len([]rune(input.TextAR)) > MaxManualPageTextRunes || len([]rune(input.NoteAR)) > 2000 {
		return PageReviewInput{}, ErrValidation
	}
	switch input.Status {
	case "unreadable":
		if input.TextAR != "" {
			return PageReviewInput{}, ErrValidation
		}
	case "transcribed", "partial":
		if input.TextAR == "" {
			return PageReviewInput{}, ErrValidation
		}
	default:
		return PageReviewInput{}, ErrValidation
	}
	return input, nil
}
