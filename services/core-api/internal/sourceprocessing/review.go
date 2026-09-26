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

// GetProcessing answers with the whole candidate list, which is the behaviour the
// endpoint had before it learned to page. A caller that wants a bounded page asks
// for one through GetProcessingPage.
func (s *Service) GetProcessing(ctx context.Context, sourceID, actorID string) (ProcessingView, error) {
	return s.GetProcessingPage(ctx, sourceID, actorID, CandidatePage{})
}

// GetProcessingPage answers with one page of candidates.
//
// The default page is deliberately the whole list. Every candidate the endpoint
// returned before is still returned, so no reviewed decision can disappear from
// the only client that reads this response; a caller that opts into a limit
// gets a bounded page and is told what the source holds in total.
func (s *Service) GetProcessingPage(ctx context.Context, sourceID, actorID string, page CandidatePage) (ProcessingView, error) {
	if err := s.ready(); err != nil {
		return ProcessingView{}, err
	}
	sourceUUID, actorUUID, err := parseIDs(sourceID, actorID)
	if err != nil {
		return ProcessingView{}, err
	}
	page, err = validateCandidatePage(page)
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
	pageResult, err := s.candidates(ctx, sourceUUID, page)
	if err != nil {
		return ProcessingView{}, err
	}
	return ProcessingView{
		SourceID:            sourceUUID.String(),
		Files:               files,
		Runs:                runs,
		Candidates:          pageResult.Candidates,
		CandidatesTotal:     pageResult.Total,
		CandidatesTruncated: pageResult.Truncated,
	}, nil
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

// candidatePage is one bounded page of candidates plus what the source holds in
// total, so a caller can tell a short list from a truncated one.
type candidatePageResult struct {
	Candidates []CandidateView
	Total      int
	Truncated  bool
}

// candidates reads the candidates of one source and the review history of all of
// them in two round trips.
//
// The review history used to be one query per candidate, which made the endpoint
// cost 1 + N statements for N candidates while returning exactly the same rows.
// The single grouped query below is bounded by the candidate ids the page
// produced, so a source with thousands of reviewed candidates costs the same two
// statements as a source with none.
func (s *Service) candidates(ctx context.Context, sourceID uuid.UUID, page CandidatePage) (candidatePageResult, error) {
	ids := make([]uuid.UUID, 0)
	// The total comes from a scalar aggregate joined to the page by LATERAL, so a
	// single statement reports both how many candidates the source holds and which
	// ids this page reads - including when the page is empty, where a window
	// function would have returned no row to read the count from. A separate COUNT
	// would cost a round trip per page for a number the page had to visit anyway.
	total := 0
	rows, err := s.Pool.Query(ctx, `
		SELECT totals.total, page.id
		FROM (SELECT count(*) AS total FROM source_candidates WHERE source_id = $1) totals
		LEFT JOIN LATERAL (
			SELECT c.id::text AS id
			FROM source_candidates c
			WHERE c.source_id = $1
			ORDER BY c.created_at, c.id
			OFFSET $2 LIMIT $3
		) page ON TRUE
	`, sourceID, page.Offset, page.Limit)
	if err != nil {
		return candidatePageResult{}, err
	}
	for rows.Next() {
		var id *string
		if err := rows.Scan(&total, &id); err != nil {
			rows.Close()
			return candidatePageResult{}, err
		}
		if id == nil {
			continue
		}
		parsed, parseErr := uuid.Parse(*id)
		if parseErr != nil {
			rows.Close()
			return candidatePageResult{}, parseErr
		}
		ids = append(ids, parsed)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return candidatePageResult{}, err
	}
	rows.Close()

	// The ids are read first because they are the argument of the grouped review
	// query. A page with no candidates therefore costs one statement, not two.
	items := make([]CandidateView, 0, len(ids))
	if len(ids) > 0 {
		rows, err := s.Pool.Query(ctx, candidateSelect+` WHERE c.id = ANY($1) ORDER BY c.created_at, c.id`, ids)
		if err != nil {
			return candidatePageResult{}, err
		}
		for rows.Next() {
			item, scanErr := scanCandidate(rows)
			if scanErr != nil {
				rows.Close()
				return candidatePageResult{}, scanErr
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return candidatePageResult{}, err
		}
		rows.Close()
		reviews, err := s.candidateReviews(ctx, ids)
		if err != nil {
			return candidatePageResult{}, err
		}
		for index := range items {
			parsed, parseErr := uuid.Parse(items[index].ID)
			if parseErr != nil {
				return candidatePageResult{}, parseErr
			}
			items[index].Reviews = reviews[parsed]
		}
	}
	// The page holds fewer candidates than the source does whenever the offset
	// plus the ids read stop short of the total. An empty page is not truncation:
	// it may simply be past the end of the list.
	return candidatePageResult{
		Candidates: items,
		Total:      total,
		Truncated:  page.Offset+len(ids) < total,
	}, nil
}

func (s *Service) getCandidate(ctx context.Context, candidateID uuid.UUID) (CandidateView, error) {
	item, err := scanCandidate(s.Pool.QueryRow(ctx, candidateSelect+` WHERE c.id = $1`, candidateID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CandidateView{}, ErrNotFound
		}
		return CandidateView{}, err
	}
	reviews, err := s.candidateReviews(ctx, []uuid.UUID{candidateID})
	if err != nil {
		return CandidateView{}, err
	}
	item.Reviews = reviews[candidateID]
	return item, nil
}

// candidateReviews loads the review history of every named candidate in one
// grouped query, keyed by candidate id.
//
// The grouping is done here rather than in SQL so the per-candidate order is the
// one the list always had, and every candidate the caller asked about is present
// in the map with an empty slice rather than absent: the response renders reviews
// as a list, and a candidate with no decision must read as "none yet" and not as
// a missing field.
func (s *Service) candidateReviews(ctx context.Context, candidateIDs []uuid.UUID) (map[uuid.UUID][]CandidateReviewView, error) {
	grouped := make(map[uuid.UUID][]CandidateReviewView, len(candidateIDs))
	for _, candidateID := range candidateIDs {
		grouped[candidateID] = make([]CandidateReviewView, 0)
	}
	if len(candidateIDs) == 0 {
		return grouped, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT candidate_id, id, reviewer_id, decision, note_ar, created_at
		FROM source_candidate_reviews WHERE candidate_id = ANY($1) ORDER BY candidate_id, created_at DESC, id DESC
	`, candidateIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item CandidateReviewView
		var candidateID, reviewerID pgtype.UUID
		var note pgtype.Text
		if err := rows.Scan(&candidateID, &item.ID, &reviewerID, &item.Decision, &note, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ReviewerID = uuidString(reviewerID)
		item.NoteAR = textValue(note)
		grouped[uuidValue(candidateID)] = append(grouped[uuidValue(candidateID)], item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grouped, nil
}

// validateCandidatePage bounds a requested page. The zero page is the whole list
// up to MaxCandidates, which is what the endpoint returned before it could page.
func validateCandidatePage(page CandidatePage) (CandidatePage, error) {
	if page.Offset < 0 {
		return CandidatePage{}, ErrValidation
	}
	if page.Limit < 0 || page.Limit > MaxCandidates {
		return CandidatePage{}, ErrValidation
	}
	if page.Limit == 0 {
		page.Limit = MaxCandidates
	}
	return page, nil
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
