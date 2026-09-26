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

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
)

type sourceFileRecord struct {
	RunID      uuid.UUID
	SourceID   uuid.UUID
	FileID     uuid.UUID
	StorageKey string
	MimeType   string
	Filename   string
	Status     string
	RunStatus  string
	ByteSize   int64
	Checksum   string
}

func (s *Service) RequeueRecovered(ctx context.Context, recovered []jobs.JobView) error {
	for _, job := range recovered {
		if job.Type != SourceProcessJobType {
			continue
		}
		payload, err := parseJobPayload(job.Payload)
		if err != nil {
			return err
		}
		if _, err := s.Pool.Exec(ctx, `
			UPDATE source_processing_runs
			SET status = 'queued', stage = 'queued', started_at = NULL, error = NULL, updated_at = now()
			WHERE source_file_id = $1 AND status = 'running'
		`, payload.SourceFileID); err != nil {
			return err
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
	file, err := s.fileForProcessing(ctx, payload.SourceFileID)
	if err != nil {
		return err
	}
	if file.SourceID.String() != payload.SourceID {
		return ErrValidation
	}
	if file.Status == "succeeded" || file.RunStatus == "succeeded" {
		return nil
	}
	// A file queued before the format contract tightened is refused here, before
	// it is claimed, read, or sent to the model: the outcome is the same on every
	// attempt and the run can never reach a succeeded state.
	if !IsSupportedContentType(file.MimeType) {
		return s.failProcessing(ctx, claim, file.RunID, file.FileID, unsupportedContent(fmt.Sprintf("the queued file %q (%s) uses a format that cannot be extracted", file.Filename, normalizeContentType(file.MimeType))))
	}
	if s.AI == nil {
		return ErrAIUnavailable
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
	pages, err := s.Extractor.Extract(ctx, ExtractInput{Reader: io.TeeReader(reader, hash), ContentType: file.MimeType, Filename: file.Filename})
	if err != nil {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, err))
	}
	if hex.EncodeToString(hash.Sum(nil)) != file.Checksum {
		return s.refuseIfLost(heartbeat, s.failProcessing(ctx, claim, file.RunID, file.FileID, ErrValidation))
	}
	processedPages, err := s.buildPages(ctx, heartbeat, pages)
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

func (s *Service) fileForProcessing(ctx context.Context, fileID string) (sourceFileRecord, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(fileID))
	if err != nil {
		return sourceFileRecord{}, ErrValidation
	}
	var file sourceFileRecord
	var storageKey, mimeType, filename, status, runStatus, checksum pgtype.Text
	var runID, sourceID pgtype.UUID
	var byteSize pgtype.Int8
	if err := s.Pool.QueryRow(ctx, `
		SELECT r.id, r.source_id, r.source_file_id, f.storage_key, f.mime_type, f.original_filename_ar, f.processing_status, r.status, f.byte_size, f.checksum_sha256
		FROM source_processing_runs r
		JOIN source_files f ON f.id = r.source_file_id
		WHERE r.source_file_id = $1
	`, parsed).Scan(&runID, &sourceID, &file.FileID, &storageKey, &mimeType, &filename, &status, &runStatus, &byteSize, &checksum); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceFileRecord{}, ErrNotFound
		}
		return sourceFileRecord{}, err
	}
	file.RunID = uuidValue(runID)
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
	return file, nil
}

func (s *Service) buildPages(ctx context.Context, heartbeat *jobs.Heartbeat, pages []Page) ([]processedPage, error) {
	processed := make([]processedPage, 0, len(pages))
	candidateTotal := 0
	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Each page is three model calls and a few dozen row lookups, so a
		// document with many pages can run for minutes between two checks. The
		// heartbeat goroutine has been renewing the whole time; this is the read
		// of its verdict at a point where acting on it is still free.
		if err := heartbeat.Check(); err != nil {
			return nil, err
		}
		normalized := identity.NormalizeArabicName(page.Text)
		if normalized == "" {
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
		processed = append(processed, processedPage{Page: page, Normalized: normalized, Embedding: values, Model: strings.Join(uniqueStrings(modelVersions), ","), Entities: processedEntities, Claims: processedClaims})
	}
	if len(processed) == 0 {
		return nil, ErrValidation
	}
	return processed, nil
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
	var sequence int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence_number), 0) FROM source_passages WHERE source_id = $1`, lockedSource).Scan(&sequence); err != nil {
		return err
	}
	candidateCount := 0
	modelVersions := make([]string, 0, len(pages))
	for _, page := range pages {
		sequence++
		passageID := uuid.New()
		pageValue := int32(page.Page.Number)
		locator := fmt.Sprintf("صفحة %d", page.Page.Number)
		if _, err := tx.Exec(ctx, `
			INSERT INTO source_passages (id, source_id, source_file_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar, start_offset, end_offset, embedding)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, passageID, lockedSource, file.FileID, sequence, pageValue, locator, page.Page.Text, page.Normalized, page.Page.StartOffset, page.Page.EndOffset, pgvector.NewVector(page.Embedding)); err != nil {
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
				INSERT INTO source_candidates (source_id, source_file_id, source_passage_id, candidate_type, raw_text_ar, normalized_text_ar, proposed_entity_type, proposed_entity_id, proposed_entity_name_ar, proposed_match_score, proposed_match_matched_on, confidence, rationale_ar, payload, model_version, dedupe_key, status)
				VALUES ($1, $2, $3, 'entity', $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''), $9, NULLIF($10, ''), $11, $12, $13, $14, $15, $16)
			`, lockedSource, file.FileID, passageID, rawText, normalized, entity.Link.Type, nullableUUIDString(entity.Link.ID), entity.Link.Name, nullableScore(entity.Link), entity.Link.MatchedOn, entity.Candidate.Confidence, entity.Candidate.Rationale, mustJSON(map[string]any{"pageNumber": page.Page.Number, "passageId": passageID.String(), "entityType": entity.Candidate.EntityType, "status": entity.Candidate.Status}), page.Model, dedupeKey, candidateStatus(entity.Candidate.Status)); err != nil {
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
				INSERT INTO source_statements (id, source_id, source_file_id, source_passage_id, statement_text_ar, locator_ar, extraction_method, review_status)
				VALUES ($1, $2, $3, $4, $5, $6, 'ai', 'needs_review')
			`, statementID, lockedSource, file.FileID, passageID, page.Page.Text, locator); err != nil {
				return err
			}
			normalized := identity.NormalizeArabicName(rawText)
			dedupeKey := candidateDedupeKey("claim", normalized, sequence)
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_candidates (source_id, source_file_id, source_passage_id, source_statement_id, candidate_type, raw_text_ar, normalized_text_ar, subject_text_ar, predicate_ar, object_text_ar, subject_entity_type, subject_entity_id, object_entity_type, object_entity_id, confidence, rationale_ar, payload, model_version, dedupe_key, status)
				VALUES ($1, $2, $3, $4, 'claim', $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), $11, NULLIF($12, ''), $13, $14, $15, $16, $17, $18, $19)
			`, lockedSource, file.FileID, passageID, statementID, rawText, normalized, claim.Candidate.SubjectText, claim.Candidate.Predicate, claim.Candidate.ObjectText, claim.Subject.Type, nullableUUIDString(claim.Subject.ID), claim.Object.Type, nullableUUIDString(claim.Object.ID), claim.Candidate.Confidence, claim.Candidate.Rationale, mustJSON(map[string]any{"pageNumber": page.Page.Number, "passageId": passageID.String(), "status": claim.Candidate.Status}), page.Model, dedupeKey, candidateStatus(claim.Candidate.Status)); err != nil {
				return err
			}
			candidateCount++
		}
	}
	modelVersion := strings.Join(uniqueStrings(modelVersions), ",")
	if _, err := tx.Exec(ctx, `
		UPDATE source_processing_runs SET status = 'succeeded', stage = 'complete', page_count = $1, passage_count = $2, candidate_count = $3, model_version = NULLIF($4, ''), completed_at = now(), error = NULL, updated_at = now()
		WHERE id = $5
	`, pageCountFromProcessed(pages), len(pages), candidateCount, modelVersion, file.RunID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_files SET processing_status = 'succeeded', processing_error = NULL, processed_at = now() WHERE id = $1`, file.FileID); err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, nil, "source_processing_completed", "source_file", file.FileID, nil, map[string]any{"pageCount": pageCountFromProcessed(pages), "passageCount": len(pages), "candidateCount": candidateCount, "modelVersion": modelVersion}); err != nil {
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
