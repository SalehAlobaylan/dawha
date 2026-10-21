package sourceprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
)

type sourceFileRecord struct {
	RunID                uuid.UUID
	SourceID             uuid.UUID
	FileID               uuid.UUID
	PageEvidenceID       uuid.UUID
	PageReviewRevisionID uuid.UUID
	RunKind              string
	RenderDPI            int
	StorageKey           string
	MimeType             string
	Filename             string
	Status               string
	RunStatus            string
	ByteSize             int64
	Checksum             string
	DocumentKind         string
}

func (s *Service) RequeueRecovered(ctx context.Context, recovered []jobs.JobView) error {
	for _, job := range recovered {
		switch job.Type {
		case SourceProcessJobType:
			payload, err := parseJobPayload(job.Payload)
			if err != nil {
				return err
			}
			if _, err := s.Pool.Exec(ctx, `
			UPDATE source_processing_runs
			SET status = 'queued', stage = 'queued', started_at = NULL, error = NULL, updated_at = now()
			WHERE id = COALESCE(NULLIF($1, '')::uuid, (SELECT id FROM source_processing_runs WHERE job_id = $2)) AND status = 'running'
			`, payload.RunID, job.ID); err != nil {
				return err
			}
		case SourceChartAnalysisJobType:
			var chartPayload ChartAnalysisPayload
			if err := json.Unmarshal(job.Payload, &chartPayload); err != nil {
				return ErrValidation
			}
			if _, err := s.Pool.Exec(ctx, `UPDATE source_chart_analysis_runs SET status = 'queued', error_code = NULL, updated_at = now() WHERE id = NULLIF($1,'')::uuid AND status = 'running'`, chartPayload.RunID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Claim is a job the worker holds: the row it claimed, and the lease that proves
// the claim is still its own.
//
// The two travel together because the row alone is not a permission. `payload`
// says what to process; the lease says this attempt is the one allowed to write
// the result. A worker that took over a reclaimed job has a different row state
// and a different token, and the processor is handed both so it can check the
// second one at every point where the first one would have been enough before.
type Claim struct {
	Job   jobs.JobView
	Lease jobs.Lease
}

// Process runs one claimed job to completion, or not at all.
//
// The lease, not the caller, decides how much of this function is allowed to
// happen. The claim is renewed on a timer for the whole run, the work is given a
// context that dies with the lease, and the transaction that writes passages and
// candidates proves the lease again as its first statement. So a worker that lost
// the job while it was embedding pages cannot reach the write at all, and the
// attempt ends as a retry rather than as a second copy of records the review
// queue would then show twice.
func (s *Service) Process(ctx context.Context, claim Claim) error {
	if claim.Job.Type != SourceProcessJobType {
		return ErrValidation
	}
	if s == nil || s.Pool == nil || s.Store == nil || s.Extractor == nil {
		return ErrDatabaseUnavailable
	}
	heartbeat, workCtx, err := s.startHeartbeat(ctx, claim)
	if err != nil {
		return err
	}
	defer func() { _ = heartbeat.Stop() }()
	// Everything below runs on the heartbeat's context, so a lost lease cancels
	// the in-flight AI call rather than waiting for it to finish before noticing.
	ctx = workCtx
	payload, err := parseJobPayload(claim.Job.Payload)
	if err != nil {
		return err
	}
	file, err := s.fileForProcessing(ctx, payload.SourceFileID, payload.RunID, claim.Job.ID)
	if err != nil {
		return err
	}
	if file.SourceID.String() != payload.SourceID {
		return ErrValidation
	}
	if payload.RunKind != "" && payload.RunKind != file.RunKind {
		return ErrValidation
	}
	if payload.RenderDPI != 0 && payload.RenderDPI != file.RenderDPI {
		return ErrValidation
	}
	if payload.PageReviewRevisionID != "" && payload.PageReviewRevisionID != file.PageReviewRevisionID.String() {
		return ErrValidation
	}
	if file.RunStatus == "succeeded" {
		return nil
	}
	// A legacy or otherwise unsupported file is refused before it is read or sent
	// to a model, so repeated attempts cannot change the outcome.
	if !IsSupportedContentType(file.MimeType) {
		return s.failProcessing(ctx, claim, file.RunID, file.FileID, unsupportedContent(fmt.Sprintf("the queued file %q (%s) uses a format that cannot be extracted", file.Filename, normalizeContentType(file.MimeType))))
	}
	if err := heartbeat.Check(); err != nil {
		return err
	}
	claimTag, err := s.Pool.Exec(ctx, `
		UPDATE source_processing_runs
		SET status = 'running', stage = 'extracting', job_id = $1, started_at = COALESCE(started_at, now()), error = NULL, updated_at = now()
		WHERE id = $2 AND (status IN ('queued', 'failed') OR (status = 'running' AND started_at < now() - interval '20 minutes'))
	`, claim.Job.ID, file.RunID)
	if err != nil {
		return err
	}
	if claimTag.RowsAffected() == 0 {
		var runStatus string
		if err := s.Pool.QueryRow(ctx, `SELECT status FROM source_processing_runs WHERE id = $1`, file.RunID).Scan(&runStatus); err != nil {
			return err
		}
		if runStatus == "running" || runStatus == "succeeded" {
			return nil
		}
		return ErrConflict
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE source_files SET processing_status = 'running', processing_error = NULL WHERE id = $1`, file.FileID); err != nil {
		return err
	}
	reader, object, err := s.Store.Get(ctx, file.StorageKey)
	if err != nil {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, err))
	}
	defer reader.Close()
	if object.Size != file.ByteSize {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, ErrValidation))
	}
	hash := sha256.New()
	var pages []Page
	if file.RunKind == "reviewed_page" {
		_, err = io.Copy(hash, reader)
		if err == nil && hex.EncodeToString(hash.Sum(nil)) == file.Checksum {
			var page Page
			page, err = s.reviewedPageForProcessing(ctx, file)
			if err == nil {
				pages = []Page{page}
			}
		} else if err == nil {
			err = ErrValidation
		}
	} else {
		pages, err = s.Extractor.Extract(ctx, ExtractInput{Reader: io.TeeReader(reader, hash), ContentType: file.MimeType, Filename: file.Filename, RenderDPI: file.RenderDPI})
	}
	if err != nil {
		// A page-level PDF failure can happen after earlier pages were read. Keep
		// those verified readings available for review; a partial extractor that
		// did not consume the whole object cannot pass the checksum fence below.
		if len(pages) > 0 && validateExtractedPages(pages) == nil && hex.EncodeToString(hash.Sum(nil)) == file.Checksum {
			if evidenceErr := s.persistPageEvidence(ctx, claim, file, pages); evidenceErr != nil {
				return s.refuseIfLost(heartbeat, evidenceErr)
			}
		}
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, err))
	}
	if err := validateExtractedPages(pages); err != nil {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, err))
	}
	if hex.EncodeToString(hash.Sum(nil)) != file.Checksum {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, ErrValidation))
	}
	if err := s.persistPageEvidence(ctx, claim, file, pages); err != nil {
		return s.refuseIfLost(heartbeat, err)
	}
	if s.AI == nil {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, ErrAIUnavailable))
	}
	processedPages, err := s.buildPages(ctx, heartbeat, pages, file.DocumentKind != "family_chart")
	if err != nil {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, err))
	}
	// The last check before the write. persistProcessedPages proves the lease again
	// inside its own transaction, so this one is a fast path that turns an obvious
	// "the lease is gone" into the same error the transaction would have produced
	// without doing any of the work first.
	if err := heartbeat.Check(); err != nil {
		return err
	}
	if err := s.persistProcessedPages(ctx, claim, file, processedPages); err != nil {
		return s.refuseIfLost(heartbeat, err)
	}
	return nil
}

// refuseIfLost replaces whatever the work reported with the lease loss, when the
// lease was lost.
//
// A worker whose claim was taken over sees its context cancelled mid-call, and
// the cancellation is what surfaces first - but the cancellation is a symptom.
// Reporting it would send a worker down the "this document failed" path and spend
// one of the job's attempts on a failure it had no right to record, while the
// actual fact - somebody else owns this job now - is the one the caller needs in
// order to stop rather than retry.
func (s *Service) refuseIfLost(heartbeat *jobs.Heartbeat, err error) error {
	if err == nil {
		return nil
	}
	if lost := heartbeat.Check(); lost != nil {
		return lost
	}
	return err
}

// startHeartbeat renews the claim for as long as the work runs. A processor
// without a queue cannot prove a claim, and a processor that cannot prove a claim
// must not be writing results, so the missing queue is a refusal rather than a
// silent downgrade to "just process it".
func (s *Service) startHeartbeat(ctx context.Context, claim Claim) (*jobs.Heartbeat, context.Context, error) {
	if s.Jobs == nil {
		return nil, nil, ErrQueueUnavailable
	}
	if !claim.Lease.Valid() {
		return nil, nil, ErrLeaseRequired
	}
	return s.Jobs.StartHeartbeat(ctx, claim.Lease)
}

func (s *Service) fileForProcessing(ctx context.Context, fileID, runID, jobID string) (sourceFileRecord, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(fileID))
	if err != nil {
		return sourceFileRecord{}, ErrValidation
	}
	var file sourceFileRecord
	var storageKey, mimeType, filename, status, runStatus, checksum, runKind, documentKind pgtype.Text
	var runUUID, sourceID, pageEvidenceID, pageReviewRevisionID pgtype.UUID
	var byteSize pgtype.Int8
	var renderDPI pgtype.Int4
	if err := s.Pool.QueryRow(ctx, `
		SELECT r.id, r.source_id, r.source_file_id, f.storage_key, f.mime_type, f.original_filename_ar, f.processing_status, r.status, f.byte_size, f.checksum_sha256, f.document_kind,
		       r.run_kind, COALESCE(NULLIF(r.pipeline_config->>'renderDpi', '')::integer, 300), r.input_page_review_revision_id, pr.source_page_evidence_id
		FROM source_processing_runs r
		JOIN source_files f ON f.id = r.source_file_id
		LEFT JOIN source_page_review_revisions pr ON pr.id = r.input_page_review_revision_id
		WHERE r.source_file_id = $1 AND r.job_id = NULLIF($3, '')::uuid
		  AND ($2 = '' OR r.id = NULLIF($2, '')::uuid)
	`, parsed, runID, jobID).Scan(&runUUID, &sourceID, &file.FileID, &storageKey, &mimeType, &filename, &status, &runStatus, &byteSize, &checksum, &documentKind, &runKind, &renderDPI, &pageReviewRevisionID, &pageEvidenceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceFileRecord{}, ErrNotFound
		}
		return sourceFileRecord{}, err
	}
	file.RunID = uuidValue(runUUID)
	file.PageEvidenceID = uuidValue(pageEvidenceID)
	file.PageReviewRevisionID = uuidValue(pageReviewRevisionID)
	file.RunKind = textValue(runKind)
	if renderDPI.Valid {
		file.RenderDPI = int(renderDPI.Int32)
	}
	file.SourceID = uuidValue(sourceID)
	file.StorageKey = textValue(storageKey)
	file.MimeType = textValue(mimeType)
	file.Filename = textValue(filename)
	file.Status = textValue(status)
	file.RunStatus = textValue(runStatus)
	if byteSize.Valid {
		file.ByteSize = byteSize.Int64
	}
	file.Checksum = textValue(checksum)
	file.DocumentKind = textValue(documentKind)
	return file, nil
}

func (s *Service) reviewedPageForProcessing(ctx context.Context, file sourceFileRecord) (Page, error) {
	if file.PageReviewRevisionID == uuid.Nil || file.PageEvidenceID == uuid.Nil {
		return Page{}, ErrValidation
	}
	var page Page
	var status string
	err := s.Pool.QueryRow(ctx, `
		SELECT p.page_number, p.page_width, p.page_height, r.status, r.transcription_text_ar
		FROM source_page_review_revisions r
		JOIN source_page_evidence p ON p.id = r.source_page_evidence_id
		WHERE r.id = $1 AND p.id = $2 AND p.source_file_id = $3
	`, file.PageReviewRevisionID, file.PageEvidenceID, file.FileID).Scan(&page.Number, &page.Width, &page.Height, &status, &page.Text)
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, err
	}
	if (status != "transcribed" && status != "partial") || strings.TrimSpace(page.Text) == "" {
		return Page{}, ErrValidation
	}
	page.PageEvidenceID = file.PageEvidenceID.String()
	page.ReviewRevisionID = file.PageReviewRevisionID.String()
	return page, nil
}

func pageEvidenceIDForPassage(ctx context.Context, tx pgx.Tx, file sourceFileRecord, page Page) (uuid.UUID, error) {
	if strings.TrimSpace(page.PageEvidenceID) != "" {
		pageID, err := uuid.Parse(page.PageEvidenceID)
		if err != nil {
			return uuid.Nil, ErrValidation
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM source_page_evidence WHERE id = $1 AND source_file_id = $2)`, pageID, file.FileID).Scan(&exists); err != nil {
			return uuid.Nil, err
		}
		if !exists {
			return uuid.Nil, ErrConflict
		}
		return pageID, nil
	}
	var pageID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM source_page_evidence WHERE run_id = $1 AND page_number = $2 ORDER BY created_at DESC, id DESC LIMIT 1`, file.RunID, page.Number).Scan(&pageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return pageID, err
}

func pageReviewRevisionForPassage(file sourceFileRecord, page Page) uuid.UUID {
	if strings.TrimSpace(page.ReviewRevisionID) != "" {
		if parsed, err := uuid.Parse(page.ReviewRevisionID); err == nil {
			return parsed
		}
	}
	if file.RunKind == "reviewed_page" {
		return file.PageReviewRevisionID
	}
	return uuid.Nil
}

func nullablePassageUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func supersedePriorCandidates(ctx context.Context, tx pgx.Tx, file sourceFileRecord, pages []processedPage) (int64, error) {
	if file.RunKind != "reviewed_page" {
		tag, err := tx.Exec(ctx, `
			UPDATE source_candidates
			SET status = 'superseded', updated_at = now()
			WHERE source_file_id = $1 AND source_processing_run_id IS DISTINCT FROM $2
			  AND status IN ('unreviewed', 'needs_review')
		`, file.FileID, file.RunID)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE source_statements
			SET review_status = 'superseded', updated_at = now()
			WHERE id IN (
				SELECT c.source_statement_id FROM source_candidates c
				WHERE c.source_file_id = $1 AND c.source_processing_run_id IS DISTINCT FROM $2
				  AND c.status = 'superseded' AND c.source_statement_id IS NOT NULL
			)
			AND review_status IN ('unreviewed', 'needs_review')
		`, file.FileID, file.RunID); err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}
	pageNumber := 0
	for _, page := range pages {
		if page.HasPassage {
			pageNumber = page.Page.Number
			break
		}
	}
	if pageNumber == 0 {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE source_candidates c
		SET status = 'superseded', updated_at = now()
		FROM source_passages p
		WHERE c.source_passage_id = p.id AND p.source_file_id = $1 AND p.page_number = $2
		  AND c.source_processing_run_id IS DISTINCT FROM $3
		  AND c.status IN ('unreviewed', 'needs_review')
	`, file.FileID, pageNumber, file.RunID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_statements ss
		SET review_status = 'superseded', updated_at = now()
		WHERE ss.review_status IN ('unreviewed', 'needs_review')
		  AND ss.id IN (
			SELECT c.source_statement_id FROM source_candidates c
			JOIN source_passages p ON p.id = c.source_passage_id
			WHERE c.source_file_id = $1 AND p.page_number = $2
			  AND c.source_processing_run_id IS DISTINCT FROM $3
			  AND c.status = 'superseded' AND c.source_statement_id IS NOT NULL
		  )
	`, file.FileID, pageNumber, file.RunID); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Service) buildPages(ctx context.Context, heartbeat *jobs.Heartbeat, pages []Page, extractCandidates bool) ([]processedPage, error) {
	processed := make([]processedPage, 0, len(pages))
	candidateTotal := 0
	for _, physicalPage := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		segments := splitPage(physicalPage, 3500)
		if len(segments) == 0 {
			segments = []Page{physicalPage}
		}
		for _, page := range segments {
			// Each page is three model calls and a few dozen row lookups, so a
			// document with many pages can run for minutes between two checks. The
			// heartbeat goroutine has been renewing the whole time; this is the read
			// of its verdict at a point where acting on it is still free.
			if err := heartbeat.Check(); err != nil {
				return nil, err
			}
			normalized := identity.NormalizeArabicName(page.Text)
			if normalized == "" {
				processed = append(processed, processedPage{Page: page})
				continue
			}
			if !extractCandidates {
				// Family-chart OCR remains page evidence only until chart reading is
				// qualified; noisy full-page text must not create platform candidates.
				processed = append(processed, processedPage{Page: page})
				continue
			}
			embedding, err := s.AI.Embed(ctx, ai.EmbeddingRequest{Text: normalized, Dimensions: EmbeddingDimensions})
			if err != nil {
				return nil, err
			}
			if len(embedding.Embedding) != EmbeddingDimensions {
				return nil, ErrValidation
			}
			values := make([]float32, len(embedding.Embedding))
			norm := 0.0
			for index, value := range embedding.Embedding {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return nil, ErrValidation
				}
				values[index] = float32(value)
				norm += value * value
			}
			if norm <= 1e-12 {
				return nil, ErrValidation
			}
			entities, err := s.AI.ExtractEntities(ctx, ai.ExtractionRequest{Text: page.Text})
			if err != nil {
				return nil, err
			}
			claims, err := s.AI.ExtractClaims(ctx, ai.ExtractionRequest{Text: page.Text})
			if err != nil {
				return nil, err
			}
			if len(entities.Entities) > MaxCandidatesPerPage || len(claims.Claims) > MaxCandidatesPerPage {
				return nil, ErrValidation
			}
			candidateTotal += len(entities.Entities) + len(claims.Claims)
			if candidateTotal > MaxCandidates {
				return nil, ErrValidation
			}
			processedEntities := make([]processedEntity, 0, len(entities.Entities))
			for _, candidate := range entities.Entities {
				link, err := s.resolveEntity(ctx, candidate.Text)
				if err != nil {
					return nil, err
				}
				processedEntities = append(processedEntities, processedEntity{Candidate: candidate, Link: link})
			}
			processedClaims := make([]processedClaim, 0, len(claims.Claims))
			for _, candidate := range claims.Claims {
				subject, err := s.resolveEntity(ctx, candidate.SubjectText)
				if err != nil {
					return nil, err
				}
				object, err := s.resolveEntity(ctx, candidate.ObjectText)
				if err != nil {
					return nil, err
				}
				processedClaims = append(processedClaims, processedClaim{Candidate: candidate, Subject: subject, Object: object})
			}
			modelVersions := []string{embedding.Model, entities.Model, claims.Model}
			processed = append(processed, processedPage{Page: page, Normalized: normalized, Embedding: values, Model: strings.Join(uniqueStrings(modelVersions), ","), Entities: processedEntities, Claims: processedClaims, HasPassage: true})
		}
	}
	if len(processed) == 0 || pageCountFromProcessed(processed) == 0 {
		return nil, ErrValidation
	}
	return processed, nil
}

// persistPageEvidence commits the physical PDF readings before candidate
// extraction. A downstream AI failure must not erase OCR work a reviewer can
// already inspect. The lease is checked in the transaction that records it.
func (s *Service) persistPageEvidence(ctx context.Context, claim Claim, file sourceFileRecord, pages []Page) error {
	containsEvidence := false
	for _, page := range pages {
		if len(page.Readings) > 0 {
			containsEvidence = true
			break
		}
	}
	if !containsEvidence {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if s.Jobs == nil {
		return ErrQueueUnavailable
	}
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	for _, page := range pages {
		if len(page.Readings) == 0 {
			continue
		}
		selectedMethod := "unknown"
		pageText := page.Text
		regions := make([]map[string]any, 0)
		for _, reading := range page.Readings {
			if reading.Selected {
				selectedMethod = reading.Method
				pageText = reading.Text
			}
			for _, region := range reading.Regions {
				regions = append(regions, map[string]any{
					"method": reading.Method, "kind": region.Kind, "text": region.Text,
					"x": region.X, "y": region.Y, "width": region.Width, "height": region.Height,
					"confidence": region.Confidence,
				})
			}
		}
		status := "readable"
		if strings.TrimSpace(pageText) == "" {
			status = "unreadable"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO source_page_evidence (run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, file.RunID, file.SourceID, file.FileID, page.Number, page.Width, page.Height, status, selectedMethod, pageText, mustJSON(page.Readings), mustJSON(regions), len(regions)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_processing_runs
		SET stage = 'extracting_candidates', page_count = GREATEST(page_count, $1), updated_at = now()
		WHERE id = $2 AND status = 'running'
	`, pageCountFromExtracted(pages), file.RunID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func splitPage(page Page, maxRunes int) []Page {
	if maxRunes <= 0 {
		maxRunes = 3500
	}
	runes := []rune(page.Text)
	if len(runes) <= maxRunes {
		page.StartOffset = 0
		page.EndOffset = len(runes)
		return []Page{page}
	}
	segments := make([]Page, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			for candidate := end; candidate > start+maxRunes*3/4; candidate-- {
				if runes[candidate-1] == '\n' || runes[candidate-1] == ' ' || runes[candidate-1] == '\t' {
					end = candidate
					break
				}
			}
		}
		segment := page
		segment.Text = string(runes[start:end])
		segment.StartOffset = start
		segment.EndOffset = end
		if start > 0 {
			segment.Readings = nil
		}
		segments = append(segments, segment)
		start = end
	}
	return segments
}

func validateExtractedPages(pages []Page) error {
	if len(pages) == 0 || len(pages) > MaxPages {
		return ErrValidation
	}
	totalRegions := 0
	totalTextBytes := 0
	lastPage := 0
	for _, page := range pages {
		if page.Number < 1 || page.Number > MaxPages || page.Number < lastPage || utf8.RuneCountInString(page.Text) > 200000 ||
			page.Width < 0 || page.Height < 0 || math.IsNaN(page.Width) || math.IsNaN(page.Height) || math.IsInf(page.Width, 0) || math.IsInf(page.Height, 0) ||
			len(page.Readings) > 8 {
			return ErrValidation
		}
		lastPage = page.Number
		totalTextBytes += len(page.Text)
		if totalTextBytes > maxDocumentTextBytes {
			return ErrValidation
		}
		selectedReadings := 0
		for _, reading := range page.Readings {
			if len(reading.Regions) > MaxPageRegions || len(reading.Text) > maxPageTextBytes || len(reading.Method) > 64 || len(reading.Model) > 256 {
				return ErrValidation
			}
			if reading.Selected {
				selectedReadings++
				if reading.Text != page.Text {
					return ErrValidation
				}
			}
			totalTextBytes += len(reading.Text)
			if totalTextBytes > maxDocumentTextBytes {
				return ErrValidation
			}
			if reading.Confidence != nil && (math.IsNaN(*reading.Confidence) || math.IsInf(*reading.Confidence, 0) || *reading.Confidence < 0 || *reading.Confidence > 1) {
				return ErrValidation
			}
			totalRegions += len(reading.Regions)
			if totalRegions > MaxDocumentRegions {
				return ErrValidation
			}
			for _, region := range reading.Regions {
				totalTextBytes += 2 * len(region.Text)
				if totalTextBytes > maxDocumentTextBytes {
					return ErrValidation
				}
				if math.IsNaN(region.X) || math.IsNaN(region.Y) || math.IsNaN(region.Width) || math.IsNaN(region.Height) ||
					math.IsInf(region.X, 0) || math.IsInf(region.Y, 0) || math.IsInf(region.Width, 0) || math.IsInf(region.Height, 0) ||
					region.X < 0 || region.Y < 0 || region.Width < 0 || region.Height < 0 ||
					region.X > 1 || region.Y > 1 || region.Width > 1 || region.Height > 1 ||
					region.X+region.Width > 1.001 || region.Y+region.Height > 1.001 {
					return ErrValidation
				}
				if region.Confidence != nil && (math.IsNaN(*region.Confidence) || math.IsInf(*region.Confidence, 0) || *region.Confidence < 0 || *region.Confidence > 1) {
					return ErrValidation
				}
			}
		}
		if len(page.Readings) > 0 && selectedReadings != 1 {
			return ErrValidation
		}
	}
	return nil
}

func (s *Service) resolveEntity(ctx context.Context, value string) (entityLink, error) {
	value = strings.TrimSpace(value)
	if value == "" || s.AI == nil {
		return entityLink{}, nil
	}
	references, err := s.entityReferences(ctx, value)
	if err != nil {
		return entityLink{}, err
	}
	if len(references) == 0 {
		return entityLink{}, nil
	}
	candidates := make([]ai.EntityReference, 0, len(references))
	byID := make(map[string]entityReference, len(references))
	for _, reference := range references {
		candidates = append(candidates, ai.EntityReference{ID: reference.ID, Name: reference.Name, Aliases: reference.Aliases})
		byID[reference.ID] = reference
	}
	result, err := s.AI.ResolveEntity(ctx, ai.EntityResolutionRequest{Name: value, Candidates: candidates})
	if err != nil {
		return entityLink{}, err
	}
	for _, match := range result.Matches {
		reference, found := byID[match.CandidateID]
		if !found || match.Score < 0.5 {
			continue
		}
		return entityLink{Type: reference.Type, ID: match.CandidateID, Name: reference.Name, Score: match.Score, MatchedOn: match.MatchedOn}, nil
	}
	return entityLink{}, nil
}

func (s *Service) entityReferences(ctx context.Context, value string) ([]entityReference, error) {
	normalized := identity.NormalizeArabicName(value)
	if normalized == "" {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT 'person', p.id, p.canonical_name_ar, '' FROM people p WHERE p.normalized_name_ar = $1
		UNION ALL
		SELECT 'person', p.id, p.canonical_name_ar, a.value_ar FROM person_aliases a JOIN people p ON p.id = a.person_id WHERE a.normalized_value_ar = $1
		UNION ALL
		SELECT 'family', f.id, f.canonical_name_ar, '' FROM families f WHERE f.normalized_name_ar = $1
		UNION ALL
		SELECT 'family', f.id, f.canonical_name_ar, a.value_ar FROM family_aliases a JOIN families f ON f.id = a.family_id WHERE a.normalized_value_ar = $1
		UNION ALL
		SELECT 'tribe', t.id, t.canonical_name_ar, '' FROM tribes t WHERE t.normalized_name_ar = $1
		UNION ALL
		SELECT 'tribe', t.id, t.canonical_name_ar, a.value_ar FROM tribe_aliases a JOIN tribes t ON t.id = a.tribe_id WHERE a.normalized_value_ar = $1
		UNION ALL
		SELECT 'place', p.id, p.canonical_name_ar, '' FROM places p WHERE p.normalized_name_ar = $1
		UNION ALL
		SELECT 'place', p.id, p.canonical_name_ar, h.name_ar FROM historical_place_names h JOIN places p ON p.id = h.place_id WHERE h.normalized_name_ar = $1
		LIMIT 100
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]entityReference, 0)
	indexes := map[string]int{}
	for rows.Next() {
		var kind, id, name, alias string
		if err := rows.Scan(&kind, &id, &name, &alias); err != nil {
			return nil, err
		}
		key := kind + ":" + id
		if index, found := indexes[key]; found {
			if alias != "" {
				items[index].Aliases = appendUnique(items[index].Aliases, alias)
			}
			continue
		}
		indexes[key] = len(items)
		reference := entityReference{Type: kind, ID: id, Name: name}
		if alias != "" {
			reference.Aliases = []string{alias}
		}
		items = append(items, reference)
	}
	return items, rows.Err()
}

// persistProcessedPages writes the accepted passages, statements and candidates
// for one file, and marks the run complete.
//
// The lease is proved as the first statement, inside the same transaction as
// everything that follows, so "I still own this job" and "these rows exist"
// commit or roll back together. Checking it before opening the transaction would
// leave a window in which the claim could be taken and this work applied anyway,
// which is the duplicate the review queue would then show as two candidates for
// one passage.
//
// Everything below the fence is unchanged and still idempotent in the sense the
// schema already guarantees: passages get fresh sequence numbers, and candidates
// carry the dedupe key built from their kind, normalised text and page, so a
// retry that does get here cannot double a candidate either.
func (s *Service) persistProcessedPages(ctx context.Context, claim Claim, file sourceFileRecord, pages []processedPage) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if s.Jobs == nil {
		return ErrQueueUnavailable
	}
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	var lockedSource uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM sources WHERE id = $1 FOR UPDATE`, file.SourceID).Scan(&lockedSource); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var documentKind string
	if err := tx.QueryRow(ctx, `SELECT document_kind FROM source_files WHERE id = $1 FOR UPDATE`, file.FileID).Scan(&documentKind); err != nil {
		return err
	}
	if documentKind == "family_chart" {
		// Close the race where an ordinary OCR job started before a reviewer
		// declared the file a chart. Its page evidence may commit, but generated
		// passages and candidates do not enter source review or search.
		for index := range pages {
			pages[index].HasPassage = false
			pages[index].Entities = nil
			pages[index].Claims = nil
			pages[index].Embedding = nil
			pages[index].Model = ""
		}
	}
	var sequence int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence_number), 0) FROM source_passages WHERE source_id = $1`, lockedSource).Scan(&sequence); err != nil {
		return err
	}
	candidateCount := 0
	passageCount := 0
	modelVersions := make([]string, 0, len(pages))
	for _, page := range pages {
		if !page.HasPassage {
			continue
		}
		sequence++
		passageCount++
		passageID := uuid.New()
		pageValue := int32(page.Page.Number)
		locator := fmt.Sprintf("صفحة %d", page.Page.Number)
		pageEvidenceID, err := pageEvidenceIDForPassage(ctx, tx, file, page.Page)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO source_passages (id, source_id, source_file_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar, start_offset, end_offset, embedding, source_processing_run_id, source_page_evidence_id, source_page_review_revision_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`, passageID, lockedSource, file.FileID, sequence, pageValue, locator, page.Page.Text, page.Normalized, page.Page.StartOffset, page.Page.EndOffset, pgvector.NewVector(page.Embedding), file.RunID, nullablePassageUUID(pageEvidenceID), nullablePassageUUID(pageReviewRevisionForPassage(file, page.Page))); err != nil {
			return err
		}
		if page.Model != "" {
			modelVersions = append(modelVersions, page.Model)
		}
		for _, entity := range page.Entities {
			rawText := strings.TrimSpace(entity.Candidate.Text)
			if rawText == "" {
				continue
			}
			normalized := identity.NormalizeArabicName(rawText)
			dedupeKey := candidateDedupeKey("entity", normalized, sequence)
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_candidates (source_id, source_file_id, source_passage_id, candidate_type, raw_text_ar, normalized_text_ar, proposed_entity_type, proposed_entity_id, proposed_entity_name_ar, proposed_match_score, proposed_match_matched_on, confidence, rationale_ar, payload, model_version, dedupe_key, status, source_processing_run_id)
				VALUES ($1, $2, $3, 'entity', $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''), $9, NULLIF($10, ''), $11, $12, $13, $14, $15, $16, $17)
			`, lockedSource, file.FileID, passageID, rawText, normalized, entity.Link.Type, nullableUUIDString(entity.Link.ID), entity.Link.Name, nullableScore(entity.Link), entity.Link.MatchedOn, entity.Candidate.Confidence, entity.Candidate.Rationale, mustJSON(map[string]any{"pageNumber": page.Page.Number, "passageId": passageID.String(), "entityType": entity.Candidate.EntityType, "status": entity.Candidate.Status}), page.Model, dedupeKey, candidateStatus(entity.Candidate.Status), file.RunID); err != nil {
				return err
			}
			candidateCount++
		}
		for _, claim := range page.Claims {
			rawText := claimRawText(claim.Candidate)
			if rawText == "" {
				continue
			}
			statementID := uuid.New()
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_statements (id, source_id, source_file_id, source_passage_id, statement_text_ar, locator_ar, extraction_method, review_status, source_processing_run_id)
				VALUES ($1, $2, $3, $4, $5, $6, 'ai', 'needs_review', $7)
			`, statementID, lockedSource, file.FileID, passageID, page.Page.Text, locator, file.RunID); err != nil {
				return err
			}
			normalized := identity.NormalizeArabicName(rawText)
			dedupeKey := candidateDedupeKey("claim", normalized, sequence)
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_candidates (source_id, source_file_id, source_passage_id, source_statement_id, candidate_type, raw_text_ar, normalized_text_ar, subject_text_ar, predicate_ar, object_text_ar, subject_entity_type, subject_entity_id, object_entity_type, object_entity_id, confidence, rationale_ar, payload, model_version, dedupe_key, status, source_processing_run_id)
				VALUES ($1, $2, $3, $4, 'claim', $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), $11, NULLIF($12, ''), $13, $14, $15, $16, $17, $18, $19, $20)
			`, lockedSource, file.FileID, passageID, statementID, rawText, normalized, claim.Candidate.SubjectText, claim.Candidate.Predicate, claim.Candidate.ObjectText, claim.Subject.Type, nullableUUIDString(claim.Subject.ID), claim.Object.Type, nullableUUIDString(claim.Object.ID), claim.Candidate.Confidence, claim.Candidate.Rationale, mustJSON(map[string]any{"pageNumber": page.Page.Number, "passageId": passageID.String(), "status": claim.Candidate.Status}), page.Model, dedupeKey, candidateStatus(claim.Candidate.Status), file.RunID); err != nil {
				return err
			}
			candidateCount++
		}
	}
	supersededCount, err := supersedePriorCandidates(ctx, tx, file, pages)
	if err != nil {
		return err
	}
	if supersededCount > 0 {
		if err := writeAudit(ctx, tx, nil, "source_processing_candidates_superseded", "source_file", file.FileID, nil, map[string]any{"runId": file.RunID.String(), "count": supersededCount, "runKind": file.RunKind}); err != nil {
			return err
		}
	}
	modelVersion := strings.Join(uniqueStrings(modelVersions), ",")
	completedPageCount := pageCountFromProcessed(pages)
	if file.RunKind == "reviewed_page" {
		completedPageCount = 1
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_processing_runs SET status = 'succeeded', stage = 'complete', page_count = $1, passage_count = $2, candidate_count = $3, model_version = NULLIF($4, ''), completed_at = now(), error = NULL, updated_at = now()
		WHERE id = $5
	`, completedPageCount, passageCount, candidateCount, modelVersion, file.RunID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_files SET processing_status = 'succeeded', processing_error = NULL, processed_at = now() WHERE id = $1`, file.FileID); err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, nil, "source_processing_completed", "source_file", file.FileID, nil, map[string]any{"runId": file.RunID.String(), "runKind": file.RunKind, "pageCount": completedPageCount, "passageCount": passageCount, "candidateCount": candidateCount, "modelVersion": modelVersion}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// failProcessing records a refusal on the run and the file and hands the cause
// back to the caller.
//
// The lease is checked here too, and for a sharper reason than in the success
// path. Marking a run failed is a write like any other, but it is a write about
// somebody else's work: a worker whose claim was taken over would be recording a
// failure over the attempt that is now making progress. So a worker that has lost
// the lease does not get to decide the run failed - the worker that owns it now
// will, when it fails on its own terms.
func (s *Service) failProcessing(ctx context.Context, claim Claim, runID, fileID uuid.UUID, cause error) error {
	message := safeProcessingError(cause)
	failures := make([]error, 0, 2)
	if err := s.holdLease(ctx, claim); err != nil {
		// A refused fence is not a failure of the job, it is the absence of the
		// right to write one. The claim is returned as the reason so the worker
		// stops rather than retrying a job it no longer holds.
		return leaseRefusal{cause: cause, refusal: err}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE source_processing_runs SET status = 'failed', stage = 'failed', error = $1, updated_at = now() WHERE id = $2`, message, runID); err != nil {
		failures = append(failures, fmt.Errorf("mark the processing run failed: %w", err))
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE source_files SET processing_status = 'failed', processing_error = $1 WHERE id = $2`, message, fileID); err != nil {
		failures = append(failures, fmt.Errorf("mark the source file failed: %w", err))
	}
	if len(failures) == 0 {
		return cause
	}
	return &unrecordedFailureError{cause: cause, failures: failures}
}

// holdLease proves the claim on the pool rather than inside a transaction. It is
// the check failProcessing needs before it opens one, and it is deliberately the
// same predicate HoldLease applies.
func (s *Service) holdLease(ctx context.Context, claim Claim) error {
	if s.Jobs == nil {
		return ErrQueueUnavailable
	}
	if !claim.Lease.Valid() {
		return ErrLeaseRequired
	}
	return s.Jobs.HoldLease(ctx, s.Pool, claim.Lease)
}

// leaseRefusal is a cause that could not be recorded because this attempt no
// longer owns the job. It keeps both halves readable: the work really did fail,
// and the failure really was not this worker's to write down. The shape mirrors
// unrecordedFailureError, so a caller reads both the same way - Unwrap is what
// stopped the write landing, Refusal is what went wrong with the document.
type leaseRefusal struct {
	cause   error
	refusal error
}

func (e leaseRefusal) Error() string {
	return fmt.Sprintf("%v was not recorded: %v", e.cause, e.refusal)
}

func (e leaseRefusal) Unwrap() error {
	return e.refusal
}

// Refusal is the lease loss, kept off the error chain so errors.Is against the
// original cause still answers "what went wrong with the document".
func (e leaseRefusal) Refusal() error {
	return e.cause
}

// unrecordedFailureError says the failure is known but could not be written
// down, which is a different outcome from a recorded refusal: left in the
// database as it is, the run keeps looking unfinished while its job dies. The
// refusal is kept for the reader, and the error chain carries the database
// failures instead of the refusal, so nothing can mistake the two apart.
type unrecordedFailureError struct {
	cause    error
	failures []error
}

func (e *unrecordedFailureError) Error() string {
	messages := make([]string, 0, len(e.failures))
	for _, failure := range e.failures {
		messages = append(messages, failure.Error())
	}
	return fmt.Sprintf("%v was not recorded: %s", e.cause, strings.Join(messages, "; "))
}

func (e *unrecordedFailureError) Unwrap() error {
	return errors.Join(e.failures...)
}

// Refusal is the failure that could not be recorded, kept readable without
// putting it back on the error chain.
func (e *unrecordedFailureError) Refusal() error {
	return e.cause
}

func safeProcessingError(cause error) string {
	switch {
	case errors.Is(cause, ErrPDFToolsUnavailable):
		return "PDF text processing tools are not installed on the source worker"
	case errors.Is(cause, ErrArabicOCRUnavailable):
		return "Arabic OCR is unavailable; check Tesseract and its ara+eng language data"
	case errors.Is(cause, ErrPDFProcessingTimedOut):
		return "PDF processing exceeded its time limit; review the original or upload a smaller document"
	case errors.Is(cause, ErrPDFProcessingFailed):
		return "A PDF page could not be read; upload a valid, unlocked copy and review the original"
	case errors.Is(cause, ErrAIUnavailable):
		return "AI extraction is unavailable; saved PDF readings remain available for review"
	case errors.Is(cause, ErrUnsupportedContent):
		return cause.Error()
	case errors.Is(cause, ErrUnsupportedDocument):
		return UnsupportedFormatSummary()
	case errors.Is(cause, ai.ErrValidation):
		return "AI response validation failed"
	default:
		return "source processing failed"
	}
}

func parseJobPayload(raw json.RawMessage) (JobPayload, error) {
	var payload JobPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return JobPayload{}, ErrValidation
	}
	if _, err := uuid.Parse(strings.TrimSpace(payload.SourceID)); err != nil {
		return JobPayload{}, ErrValidation
	}
	if _, err := uuid.Parse(strings.TrimSpace(payload.SourceFileID)); err != nil {
		return JobPayload{}, ErrValidation
	}
	if strings.TrimSpace(payload.RunID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(payload.RunID)); err != nil {
			return JobPayload{}, ErrValidation
		}
	}
	if payload.RunKind != "" && payload.RunKind != "document" && payload.RunKind != "reviewed_page" {
		return JobPayload{}, ErrValidation
	}
	if payload.RenderDPI != 0 && payload.RenderDPI != 300 && payload.RenderDPI != 400 {
		return JobPayload{}, ErrValidation
	}
	if payload.PageReviewRevisionID != "" {
		if _, err := uuid.Parse(strings.TrimSpace(payload.PageReviewRevisionID)); err != nil {
			return JobPayload{}, ErrValidation
		}
	}
	return payload, nil
}

func candidateDedupeKey(kind, normalized string, page int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", kind, normalized, page)))
	return hex.EncodeToString(digest[:])
}

func claimRawText(claim ai.ClaimCandidate) string {
	parts := []string{claim.SubjectText, claim.Predicate, claim.ObjectText}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			values = append(values, strings.TrimSpace(part))
		}
	}
	return strings.Join(values, " ")
}

func candidateStatus(value string) string {
	if value == "unreviewed" || value == "needs_review" {
		return value
	}
	return "needs_review"
}

func nullableScore(link entityLink) any {
	if link.ID == "" {
		return nil
	}
	return link.Score
}

func nullableUUIDString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func pageCountFromProcessed(pages []processedPage) int {
	if len(pages) == 0 {
		return 0
	}
	return pages[len(pages)-1].Page.Number
}

func pageCountFromExtracted(pages []Page) int {
	if len(pages) == 0 {
		return 0
	}
	return pages[len(pages)-1].Number
}
