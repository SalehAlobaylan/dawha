package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ReprocessInput struct {
	RenderDPI int `json:"renderDpi"`
}

type ReextractPageInput struct {
	PageNumber             int    `json:"pageNumber"`
	ExpectedPageEvidenceID string `json:"expectedPageEvidenceId"`
	PageReviewRevisionID   string `json:"pageReviewRevisionId"`
	ExpectedRevision       int    `json:"expectedRevision"`
}

// ReprocessFile queues a new immutable PDF reading run. The prior run, page
// readings, human corrections, and decisions remain available for comparison.
func (s *Service) ReprocessFile(ctx context.Context, fileID, actorID string, input ReprocessInput) (ProcessingRunView, error) {
	if input.RenderDPI == 0 {
		input.RenderDPI = 400
	}
	if input.RenderDPI != 300 && input.RenderDPI != 400 {
		return ProcessingRunView{}, ErrValidation
	}
	return s.enqueueSourceRun(ctx, fileID, actorID, JobPayload{RunKind: "document", RenderDPI: input.RenderDPI}, map[string]any{"renderDpi": input.RenderDPI, "ocr": "local_poppler_tesseract", "trigger": "user_reprocess"}, nil)
}

// ReextractReviewedPage runs text extraction against one exact human-reviewed
// transcription revision. It does not rerun OCR or replace the page image/readings.
func (s *Service) ReextractReviewedPage(ctx context.Context, fileID, actorID string, input ReextractPageInput) (ProcessingRunView, error) {
	if input.PageNumber < 1 || input.PageNumber > MaxPages || input.ExpectedRevision < 1 {
		return ProcessingRunView{}, ErrValidation
	}
	pageEvidenceID, err := uuid.Parse(strings.TrimSpace(input.ExpectedPageEvidenceID))
	if err != nil {
		return ProcessingRunView{}, ErrValidation
	}
	revisionID, err := uuid.Parse(strings.TrimSpace(input.PageReviewRevisionID))
	if err != nil {
		return ProcessingRunView{}, ErrValidation
	}
	payload := JobPayload{RunKind: "reviewed_page", PageReviewRevisionID: revisionID.String()}
	config := map[string]any{"extractionSource": "human_review", "pageNumber": input.PageNumber, "pageEvidenceId": pageEvidenceID.String(), "pageReviewRevisionId": revisionID.String(), "reviewRevision": input.ExpectedRevision}
	return s.enqueueSourceRun(ctx, fileID, actorID, payload, config, &reviewedPageInput{PageEvidenceID: pageEvidenceID, RevisionID: revisionID, PageNumber: input.PageNumber, RevisionNumber: input.ExpectedRevision})
}

type reviewedPageInput struct {
	PageEvidenceID uuid.UUID
	RevisionID     uuid.UUID
	PageNumber     int
	RevisionNumber int
}

func (s *Service) enqueueSourceRun(ctx context.Context, fileID, actorID string, payload JobPayload, pipelineConfig map[string]any, reviewed *reviewedPageInput) (ProcessingRunView, error) {
	if err := s.ready(); err != nil {
		return ProcessingRunView{}, err
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return ProcessingRunView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ProcessingRunView{}, err
	}
	defer tx.Rollback(ctx)
	var sourceID pgtype.UUID
	var mimeType string
	if err := tx.QueryRow(ctx, `SELECT source_id, mime_type FROM source_files WHERE id = $1 FOR UPDATE`, fileUUID).Scan(&sourceID, &mimeType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProcessingRunView{}, ErrNotFound
		}
		return ProcessingRunView{}, err
	}
	sourceUUID := uuidValue(sourceID)
	allowed, err := canManageSource(ctx, tx, sourceUUID, actorUUID)
	if err != nil {
		return ProcessingRunView{}, err
	}
	if !allowed {
		return ProcessingRunView{}, ErrForbidden
	}
	if normalizeContentType(mimeType) != "application/pdf" && !isImageContentType(mimeType) {
		return ProcessingRunView{}, ErrValidation
	}
	if reviewed != nil {
		var latestRevision int
		var actualPage int
		var actualFileID uuid.UUID
		var reviewStatus, transcription string
		if err := tx.QueryRow(ctx, `
			SELECT p.source_file_id, p.page_number, r.revision_number, r.status, r.transcription_text_ar
			FROM source_page_review_revisions r
			JOIN source_page_evidence p ON p.id = r.source_page_evidence_id
			WHERE r.id = $1 AND p.id = $2
			FOR SHARE OF r, p
		`, reviewed.RevisionID, reviewed.PageEvidenceID).Scan(&actualFileID, &actualPage, &latestRevision, &reviewStatus, &transcription); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ProcessingRunView{}, ErrNotFound
			}
			return ProcessingRunView{}, err
		}
		if actualFileID != fileUUID || actualPage != reviewed.PageNumber || latestRevision != reviewed.RevisionNumber || (reviewStatus != "transcribed" && reviewStatus != "partial") || strings.TrimSpace(transcription) == "" {
			return ProcessingRunView{}, ErrConflict
		}
		var maxRevision int
		if err := tx.QueryRow(ctx, `SELECT max(revision_number) FROM source_page_review_revisions WHERE source_page_evidence_id = $1`, reviewed.PageEvidenceID).Scan(&maxRevision); err != nil {
			return ProcessingRunView{}, err
		}
		if maxRevision != reviewed.RevisionNumber {
			return ProcessingRunView{}, ErrConflict
		}
	}
	var existingRunID pgtype.UUID
	var existingRunStatus string
	if reviewed != nil {
		err := tx.QueryRow(ctx, `SELECT id, status FROM source_processing_runs WHERE source_file_id = $1 AND run_kind = 'reviewed_page' AND input_page_review_revision_id = $2 ORDER BY created_at DESC, id DESC LIMIT 1`, fileUUID, reviewed.RevisionID).Scan(&existingRunID, &existingRunStatus)
		if err == nil && existingRunStatus != "failed" {
			if err := tx.Commit(ctx); err != nil {
				return ProcessingRunView{}, err
			}
			return s.processingRun(ctx, uuidValue(existingRunID))
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ProcessingRunView{}, err
		}
	}
	var activeRunID pgtype.UUID
	var activeReviewRevisionID pgtype.UUID
	var activeRunKind string
	var activeRenderDPI int
	err = tx.QueryRow(ctx, `
		SELECT r.id, r.run_kind, COALESCE(NULLIF(r.pipeline_config->>'renderDpi', '')::integer, 300), r.input_page_review_revision_id
		FROM source_processing_runs r LEFT JOIN jobs j ON j.id = r.job_id
		WHERE r.source_file_id = $1 AND (r.status IN ('queued', 'running') OR j.status IN ('queued', 'running'))
		ORDER BY r.created_at DESC, r.id DESC LIMIT 1
	`, fileUUID).Scan(&activeRunID, &activeRunKind, &activeRenderDPI, &activeReviewRevisionID)
	if err == nil {
		if reviewed == nil && activeRunKind == "document" && activeRenderDPI == payload.RenderDPI {
			if err := tx.Commit(ctx); err != nil {
				return ProcessingRunView{}, err
			}
			return s.processingRun(ctx, uuidValue(activeRunID))
		}
		if reviewed != nil && activeRunKind == "reviewed_page" && uuidValue(activeReviewRevisionID) == reviewed.RevisionID {
			if err := tx.Commit(ctx); err != nil {
				return ProcessingRunView{}, err
			}
			return s.processingRun(ctx, uuidValue(activeRunID))
		}
		return ProcessingRunView{}, ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ProcessingRunView{}, err
	}
	var latestStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM source_processing_runs WHERE source_file_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1 FOR UPDATE`, fileUUID).Scan(&latestStatus); err == nil {
		if latestStatus != "succeeded" && latestStatus != "failed" {
			return ProcessingRunView{}, ErrConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ProcessingRunView{}, err
	}
	runID := uuid.New()
	payload.SourceID = sourceUUID.String()
	payload.SourceFileID = fileUUID.String()
	payload.RunID = runID.String()
	if reviewed != nil {
		payload.PageReviewRevisionID = reviewed.RevisionID.String()
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return ProcessingRunView{}, err
	}
	configBytes, err := json.Marshal(pipelineConfig)
	if err != nil {
		return ProcessingRunView{}, err
	}
	var pageReviewRevision any
	if reviewed != nil {
		pageReviewRevision = reviewed.RevisionID
		payload.PageReviewRevisionID = reviewed.RevisionID.String()
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_processing_runs (id, source_id, source_file_id, status, stage, run_kind, pipeline_config, triggered_by, input_page_review_revision_id)
		VALUES ($1, $2, $3, 'queued', 'queued', $4, $5, $6, $7)
	`, runID, sourceUUID, fileUUID, payload.RunKind, configBytes, actorUUID, pageReviewRevision); err != nil {
		return ProcessingRunView{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_files SET processing_status = 'queued', processing_error = NULL, processed_at = NULL WHERE id = $1`, fileUUID); err != nil {
		return ProcessingRunView{}, err
	}
	action := "source_processing_requeued"
	if reviewed != nil {
		action = "source_page_revision_reextraction_queued"
	}
	if err := writeAudit(ctx, tx, &actorUUID, action, "source_processing_run", runID, nil, map[string]any{"sourceFileId": fileUUID.String(), "runKind": payload.RunKind, "pipelineConfig": pipelineConfig}); err != nil {
		return ProcessingRunView{}, err
	}
	transactional, ok := s.Jobs.(TransactionalJobEnqueuer)
	if !ok {
		return ProcessingRunView{}, ErrQueueUnavailable
	}
	key := "source_process_run:" + runID.String()
	enqueued, err := transactional.EnqueueTx(ctx, tx, jobs.EnqueueInput{Type: SourceProcessJobType, Payload: payloadBytes, Priority: 0, IdempotencyKey: key, MaxAttempts: 3})
	if err != nil {
		return ProcessingRunView{}, ErrQueueUnavailable
	}
	if _, err := tx.Exec(ctx, `UPDATE source_processing_runs SET job_id = $1, updated_at = now() WHERE id = $2`, enqueued.Job.ID, runID); err != nil {
		return ProcessingRunView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProcessingRunView{}, err
	}
	return s.processingRun(ctx, runID)
}

func (s *Service) processingRun(ctx context.Context, runID uuid.UUID) (ProcessingRunView, error) {
	var item ProcessingRunView
	var sourceID, fileID, jobID, actorID, reviewID pgtype.UUID
	var config []byte
	var model, errorText pgtype.Text
	var started, completed pgtype.Timestamptz
	err := s.Pool.QueryRow(ctx, `
		SELECT id, source_id, source_file_id, job_id, status, stage, run_kind, pipeline_config, triggered_by, input_page_review_revision_id,
		       page_count, passage_count, candidate_count, model_version, error, started_at, completed_at, created_at, updated_at
		FROM source_processing_runs WHERE id = $1
	`, runID).Scan(&item.ID, &sourceID, &fileID, &jobID, &item.Status, &item.Stage, &item.RunKind, &config, &actorID, &reviewID, &item.PageCount, &item.PassageCount, &item.CandidateCount, &model, &errorText, &started, &completed, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProcessingRunView{}, ErrNotFound
	}
	if err != nil {
		return ProcessingRunView{}, err
	}
	item.SourceID = uuidString(sourceID)
	item.SourceFileID = uuidString(fileID)
	item.JobID = uuidString(jobID)
	item.TriggeredBy = uuidString(actorID)
	item.PageReviewRevisionID = uuidString(reviewID)
	item.PipelineConfig = append(json.RawMessage(nil), config...)
	item.ModelVersion = textValue(model)
	item.Error = textValue(errorText)
	if started.Valid {
		item.StartedAt = &started.Time
	}
	if completed.Valid {
		item.CompletedAt = &completed.Time
	}
	return item, nil
}
