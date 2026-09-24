package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) GetProcessing(ctx context.Context, sourceID, actorID string) (ProcessingView, error) {
	if err := s.ready(); err != nil {
		return ProcessingView{}, err
	}
	sourceUUID, actorUUID, err := parseIDs(sourceID, actorID)
	if err != nil {
		return ProcessingView{}, err
	}
	allowed, err := canManageSource(ctx, s.Pool, sourceUUID, actorUUID)
	if err != nil {
		return ProcessingView{}, err
	}
	if !allowed {
		return ProcessingView{}, ErrForbidden
	}
	files, err := s.files(ctx, sourceUUID)
	if err != nil {
		return ProcessingView{}, err
	}
	runs, err := s.runs(ctx, sourceUUID)
	if err != nil {
		return ProcessingView{}, err
	}
	candidates, err := s.candidates(ctx, sourceUUID)
	if err != nil {
		return ProcessingView{}, err
	}
	return ProcessingView{SourceID: sourceUUID.String(), Files: files, Runs: runs, Candidates: candidates}, nil
}

func (s *Service) ReviewCandidate(ctx context.Context, candidateID, actorID string, input ReviewInput) (CandidateView, error) {
	if err := s.ready(); err != nil {
		return CandidateView{}, err
	}
	candidateUUID, reviewerUUID, err := parseIDs(candidateID, actorID)
	if err != nil {
		return CandidateView{}, err
	}
	input, err = validateReviewInput(input)
	if err != nil {
		return CandidateView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return CandidateView{}, err
	}
	defer tx.Rollback(ctx)
	var sourceID, sourceFileID, passageID, statementID pgtype.UUID
	var candidateType, status, rawText string
	var subjectEntityType, objectEntityType, predicateText pgtype.Text
	var subjectEntityID, objectEntityID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		SELECT source_id, source_file_id, source_passage_id, source_statement_id, candidate_type, status, raw_text_ar, predicate_ar, subject_entity_type, subject_entity_id, object_entity_type, object_entity_id
		FROM source_candidates WHERE id = $1 FOR UPDATE
	`, candidateUUID).Scan(&sourceID, &sourceFileID, &passageID, &statementID, &candidateType, &status, &rawText, &predicateText, &subjectEntityType, &subjectEntityID, &objectEntityType, &objectEntityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CandidateView{}, ErrNotFound
		}
		return CandidateView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, uuidValue(sourceID), reviewerUUID)
	if err != nil {
		return CandidateView{}, err
	}
	if !allowed {
		return CandidateView{}, ErrForbidden
	}
	if status == "accepted" || status == "rejected" {
		return CandidateView{}, ErrConflict
	}
	reviewID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_candidate_reviews (id, candidate_id, reviewer_id, decision, note_ar)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
	`, reviewID, candidateUUID, reviewerUUID, input.Decision, input.NoteAR); err != nil {
		return CandidateView{}, err
	}
	acceptedType := ""
	relationPredicate := textValue(predicateText)
	if relationPredicate == "" {
		relationPredicate = "extracted_relation"
	}
	var acceptedID *uuid.UUID
	if input.Decision == "accepted" {
		if !statementID.Valid {
			newStatementID := uuid.New()
			var pageNumber pgtype.Int4
			if err := tx.QueryRow(ctx, `SELECT page_number FROM source_passages WHERE id = $1`, passageID).Scan(&pageNumber); err != nil {
				return CandidateView{}, err
			}
			locator := ""
			if pageNumber.Valid {
				locator = "صفحة " + itoa(pageNumber.Int32)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_statements (id, source_id, source_file_id, source_passage_id, statement_text_ar, locator_ar, extraction_method, review_status, created_by)
				VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), 'ai', 'accepted', $7)
			`, newStatementID, sourceID, sourceFileID, passageID, rawText, locator, reviewerUUID); err != nil {
				return CandidateView{}, err
			}
			statementID = pgtype.UUID{Bytes: newStatementID, Valid: true}
			acceptedType = "source_statement"
			acceptedID = &newStatementID
		} else {
			if _, err := tx.Exec(ctx, `UPDATE source_statements SET review_status = 'accepted', updated_at = now() WHERE id = $1`, statementID); err != nil {
				return CandidateView{}, err
			}
			acceptedType = "source_statement"
			statementValue := uuidValue(statementID)
			acceptedID = &statementValue
		}
		if candidateType == "claim" && subjectEntityID.Valid && objectEntityID.Valid {
			claimID := uuid.New()
			if _, err := tx.Exec(ctx, `
				INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
				VALUES ($1, $2, $3, $4, $5, $6, 'unresolved', NULLIF($7, ''), $8)
			`, claimID, textValue(subjectEntityType), subjectEntityID, relationPredicate, textValue(objectEntityType), objectEntityID, input.NoteAR, reviewerUUID); err != nil {
				return CandidateView{}, err
			}
			snapshot := map[string]any{
				"subjectType": textValue(subjectEntityType), "subjectId": uuidValue(subjectEntityID).String(), "predicate": relationPredicate, "objectType": textValue(objectEntityType), "objectId": uuidValue(objectEntityID).String(), "status": "unresolved", "sourceCandidateId": candidateUUID.String(),
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO claim_versions (claim_id, version_number, snapshot, change_reason_ar, created_by)
				VALUES ($1, 1, $2, NULLIF($3, ''), $4)
			`, claimID, mustJSON(snapshot), input.NoteAR, reviewerUUID); err != nil {
				return CandidateView{}, err
			}
			if statementID.Valid {
				if _, err := tx.Exec(ctx, `
					INSERT INTO claim_evidence (claim_id, source_statement_id, source_passage_id, evidence_note_ar, relation, created_by)
					VALUES ($1, $2, $3, NULLIF($4, ''), 'supports', $5)
				`, claimID, statementID, passageID, input.NoteAR, reviewerUUID); err != nil {
					return CandidateView{}, err
				}
			}
			acceptedType = "claim"
			acceptedID = &claimID
		}
	} else if statementID.Valid {
		if _, err := tx.Exec(ctx, `UPDATE source_statements SET review_status = 'rejected', updated_at = now() WHERE id = $1`, statementID); err != nil {
			return CandidateView{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_candidates
		SET status = $1, source_statement_id = $2, reviewed_by = $3, reviewed_at = now(), review_note_ar = NULLIF($4, ''), accepted_record_type = NULLIF($5, ''), accepted_record_id = $6, updated_at = now()
		WHERE id = $7
	`, input.Decision, statementID, reviewerUUID, input.NoteAR, acceptedType, nullableUUIDValue(acceptedID), candidateUUID); err != nil {
		return CandidateView{}, err
	}
	acceptedRecordID := ""
	if acceptedID != nil {
		acceptedRecordID = acceptedID.String()
	}
	if err := writeAudit(ctx, tx, &reviewerUUID, "source_candidate_reviewed", "source_candidate", candidateUUID, map[string]any{"status": status}, map[string]any{
		"decision": input.Decision, "noteAr": input.NoteAR, "acceptedRecordType": acceptedType, "acceptedRecordId": acceptedRecordID,
	}); err != nil {
		return CandidateView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CandidateView{}, err
	}
	return s.getCandidate(ctx, candidateUUID)
}

func validateReviewInput(input ReviewInput) (ReviewInput, error) {
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if (input.Decision != "accepted" && input.Decision != "rejected") || len([]rune(input.NoteAR)) > 2000 {
		return ReviewInput{}, ErrValidation
	}
	return input, nil
}

func (s *Service) files(ctx context.Context, sourceID uuid.UUID) ([]FileView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, source_id, original_filename_ar, mime_type, byte_size, checksum_sha256, processing_status, processing_error, processed_at, created_at
		FROM source_files WHERE source_id = $1 ORDER BY created_at DESC
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]FileView, 0)
	for rows.Next() {
		var item FileView
		var fileSourceID pgtype.UUID
		var filename, mimeType, checksum, processingError pgtype.Text
		var size pgtype.Int8
		var processedAt pgtype.Timestamptz
		if err := rows.Scan(&item.ID, &fileSourceID, &filename, &mimeType, &size, &checksum, &item.ProcessingStatus, &processingError, &processedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.SourceID = uuidString(fileSourceID)
		item.OriginalFilenameAR = textValue(filename)
		item.MimeType = textValue(mimeType)
		if size.Valid {
			item.ByteSize = size.Int64
		}
		item.ChecksumSHA256 = textValue(checksum)
		item.ProcessingError = textValue(processingError)
		if processedAt.Valid {
			value := processedAt.Time
			item.ProcessedAt = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) runs(ctx context.Context, sourceID uuid.UUID) ([]ProcessingRunView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, source_id, source_file_id, job_id, status, stage, page_count, passage_count, candidate_count, model_version, error, started_at, completed_at, created_at, updated_at
		FROM source_processing_runs WHERE source_id = $1 ORDER BY created_at DESC
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ProcessingRunView, 0)
	for rows.Next() {
		var item ProcessingRunView
		var source, file, jobID pgtype.UUID
		var model, errText pgtype.Text
		var startedAt, completedAt pgtype.Timestamptz
		if err := rows.Scan(&item.ID, &source, &file, &jobID, &item.Status, &item.Stage, &item.PageCount, &item.PassageCount, &item.CandidateCount, &model, &errText, &startedAt, &completedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.SourceID = uuidString(source)
		item.SourceFileID = uuidString(file)
		item.JobID = uuidString(jobID)
		item.ModelVersion = textValue(model)
		item.Error = textValue(errText)
		if startedAt.Valid {
			value := startedAt.Time
			item.StartedAt = &value
		}
		if completedAt.Valid {
			value := completedAt.Time
			item.CompletedAt = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) candidates(ctx context.Context, sourceID uuid.UUID) ([]CandidateView, error) {
	rows, err := s.Pool.Query(ctx, candidateSelect+` WHERE c.source_id = $1 ORDER BY c.created_at, c.id LIMIT $2`, sourceID, MaxCandidates)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CandidateView, 0)
	for rows.Next() {
		item, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range items {
		id, err := uuid.Parse(items[index].ID)
		if err != nil {
			return nil, err
		}
		items[index].Reviews, err = s.candidateReviews(ctx, id)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *Service) getCandidate(ctx context.Context, candidateID uuid.UUID) (CandidateView, error) {
	item, err := scanCandidate(s.Pool.QueryRow(ctx, candidateSelect+` WHERE c.id = $1`, candidateID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CandidateView{}, ErrNotFound
		}
		return CandidateView{}, err
	}
	item.Reviews, err = s.candidateReviews(ctx, candidateID)
	return item, err
}

func (s *Service) candidateReviews(ctx context.Context, candidateID uuid.UUID) ([]CandidateReviewView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, reviewer_id, decision, note_ar, created_at
		FROM source_candidate_reviews WHERE candidate_id = $1 ORDER BY created_at DESC
	`, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CandidateReviewView, 0)
	for rows.Next() {
		var item CandidateReviewView
		var reviewerID pgtype.UUID
		var note pgtype.Text
		if err := rows.Scan(&item.ID, &reviewerID, &item.Decision, &note, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ReviewerID = uuidString(reviewerID)
		item.NoteAR = textValue(note)
		items = append(items, item)
	}
	return items, rows.Err()
}

const candidateSelect = `
	SELECT c.id, c.source_id, c.source_file_id, c.source_passage_id, c.source_statement_id,
	       c.candidate_type, c.raw_text_ar, c.normalized_text_ar, c.subject_text_ar, c.predicate_ar, c.object_text_ar,
	       c.proposed_entity_type, c.proposed_entity_id, c.proposed_entity_name_ar, c.proposed_match_score, c.proposed_match_matched_on,
	       c.subject_entity_type, c.subject_entity_id, c.object_entity_type, c.object_entity_id,
	       c.confidence, c.rationale_ar, sp.page_number, sp.text_ar, sp.locator_ar, c.model_version, c.status,
	       c.reviewed_by, c.reviewed_at, c.review_note_ar, c.accepted_record_type, c.accepted_record_id, c.created_at, c.updated_at
	FROM source_candidates c
	JOIN source_passages sp ON sp.id = c.source_passage_id`

func scanCandidate(row pgx.Row) (CandidateView, error) {
	var item CandidateView
	var sourceID, fileID, passageID, statementID pgtype.UUID
	var proposedType, proposedName, proposedMatched, subjectType, objectType, modelVersion, acceptedType, locator, rationale pgtype.Text
	var subjectText, predicate, objectText, reviewNote pgtype.Text
	var proposedID, subjectID, objectID, reviewedBy, acceptedID pgtype.UUID
	var score pgtype.Float8
	var pageNumber pgtype.Int4
	var reviewedAt pgtype.Timestamptz
	if err := row.Scan(&item.ID, &sourceID, &fileID, &passageID, &statementID, &item.CandidateType, &item.RawTextAR, &item.NormalizedTextAR, &subjectText, &predicate, &objectText, &proposedType, &proposedID, &proposedName, &score, &proposedMatched, &subjectType, &subjectID, &objectType, &objectID, &item.Confidence, &rationale, &pageNumber, &item.PassageTextAR, &locator, &modelVersion, &item.Status, &reviewedBy, &reviewedAt, &reviewNote, &acceptedType, &acceptedID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return CandidateView{}, err
	}
	item.SourceID = uuidString(sourceID)
	item.SourceFileID = uuidString(fileID)
	item.SourcePassageID = uuidString(passageID)
	item.SourceStatementID = uuidString(statementID)
	item.SubjectTextAR = textValue(subjectText)
	item.PredicateAR = textValue(predicate)
	item.ObjectTextAR = textValue(objectText)
	item.ProposedEntityType = textValue(proposedType)
	item.ProposedEntityID = uuidString(proposedID)
	item.ProposedEntityNameAR = textValue(proposedName)
	if score.Valid {
		value := score.Float64
		item.ProposedMatchScore = &value
	}
	item.ProposedMatchMatchedOn = textValue(proposedMatched)
	item.SubjectEntityType = textValue(subjectType)
	item.SubjectEntityID = uuidString(subjectID)
	item.ObjectEntityType = textValue(objectType)
	item.ObjectEntityID = uuidString(objectID)
	item.RationaleAR = textValue(rationale)
	if pageNumber.Valid {
		value := int(pageNumber.Int32)
		item.PageNumber = &value
	}
	item.LocatorAR = textValue(locator)
	item.ModelVersion = textValue(modelVersion)
	item.ReviewedBy = uuidString(reviewedBy)
	item.ReviewNoteAR = textValue(reviewNote)
	if reviewedAt.Valid {
		value := reviewedAt.Time
		item.ReviewedAt = &value
	}
	item.AcceptedRecordType = textValue(acceptedType)
	item.AcceptedRecordID = uuidString(acceptedID)
	return item, nil
}

func nullableUUIDValue(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return *value
}

func uuidValue(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func itoa(value int32) string {
	if value < 0 {
		return ""
	}
	return strconv.Itoa(int(value))
}
