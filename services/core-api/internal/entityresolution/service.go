package entityresolution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Pool *pgxpool.Pool
	AI   ai.Provider
}

func NewService(pool *pgxpool.Pool, provider ai.Provider) *Service {
	return &Service{Pool: pool, AI: provider}
}

var runRoles = []string{"researcher", "moderator", "admin"}
var mergeRoles = []string{"moderator", "admin"}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) Run(ctx context.Context, actorID string, input RunInput) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	actorID = strings.TrimSpace(actorID)
	if err := requireRole(ctx, s.Pool, actorID, runRoles...); err != nil {
		return Run{}, err
	}
	entityType := EntityType(strings.ToLower(strings.TrimSpace(string(input.EntityType))))
	if entityType == "" {
		entityType = EntityAll
	}
	if entityType != EntityPerson && entityType != EntityFamily && entityType != EntityAll {
		return Run{}, ErrValidation
	}
	runID := uuid.New()
	var createdAt, updatedAt time.Time
	if err := s.Pool.QueryRow(ctx, `
		INSERT INTO entity_resolution_runs (id, requested_by, entity_type, status, algorithm_version, normalization_version)
		VALUES ($1, $2, $3, 'running', $4, $5)
		RETURNING created_at, updated_at
	`, runID, actorID, entityType, AlgorithmVersion, NormalizationVersion).Scan(&createdAt, &updatedAt); err != nil {
		return Run{}, err
	}
	snapshots, err := s.loadSnapshots(ctx, entityType)
	if err != nil {
		_ = s.markRunFailed(ctx, runID, err)
		return Run{}, err
	}
	candidates, blocks, modelVersion := s.generateCandidates(ctx, snapshots)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		_ = s.markRunFailed(ctx, runID, err)
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	for _, block := range blocks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO entity_resolution_blocks (run_id, entity_type, entity_id, block_type, block_key)
			VALUES ($1, $2, $3, $4, $5)
		`, runID, block.EntityType, block.EntityID, block.Kind, block.Key); err != nil {
			_ = s.markRunFailed(ctx, runID, err)
			return Run{}, err
		}
	}
	for _, candidate := range candidates {
		if _, err := tx.Exec(ctx, `
			INSERT INTO entity_resolution_candidates (run_id, entity_type, left_entity_id, right_entity_id, left_name_ar, right_name_ar, match_class, score, score_components, matching_signals, conflicting_signals, explanation_ar)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, runID, candidate.Left.EntityType, candidate.Left.ID, candidate.Right.ID, candidate.Left.Name, candidate.Right.Name, candidate.MatchClass, candidate.Score, mustJSON(candidate.ScoreComponents), mustJSON(candidate.MatchingSignals), mustJSON(candidate.ConflictingSignals), candidate.ExplanationAR); err != nil {
			_ = s.markRunFailed(ctx, runID, err)
			return Run{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE entity_resolution_runs
		SET status = 'succeeded', model_version = NULLIF($1, ''), candidate_count = $2, updated_at = now()
		WHERE id = $3
	`, modelVersion, len(candidates), runID); err != nil {
		_ = s.markRunFailed(ctx, runID, err)
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = s.markRunFailed(ctx, runID, err)
		return Run{}, err
	}
	return Run{ID: runID.String(), RequestedBy: actorID, EntityType: entityType, Status: "succeeded", AlgorithmVersion: AlgorithmVersion, NormalizationVersion: NormalizationVersion, ModelVersion: modelVersion, CandidateCount: len(candidates), CreatedAt: createdAt, UpdatedAt: updatedAt}, nil
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), runRoles...); err != nil {
		return Run{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return Run{}, ErrNotFound
	}
	return s.getRun(ctx, s.Pool, id)
}

func (s *Service) getRun(ctx context.Context, q queryer, id uuid.UUID) (Run, error) {
	var result Run
	var requestedBy string
	var entityType, status, algorithmVersion, normalizationVersion string
	var modelVersion, runError pgtype.Text
	if err := q.QueryRow(ctx, `
		SELECT id::text, requested_by::text, entity_type, status, algorithm_version, normalization_version, model_version, candidate_count, error, created_at, updated_at
		FROM entity_resolution_runs WHERE id = $1
	`, id).Scan(&result.ID, &requestedBy, &entityType, &status, &algorithmVersion, &normalizationVersion, &modelVersion, &result.CandidateCount, &runError, &result.CreatedAt, &result.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	result.RequestedBy = requestedBy
	result.EntityType = EntityType(entityType)
	result.Status = status
	result.AlgorithmVersion = algorithmVersion
	result.NormalizationVersion = normalizationVersion
	result.ModelVersion = textValue(modelVersion)
	result.Error = textValue(runError)
	return result, nil
}

func (s *Service) ListCandidates(ctx context.Context, actorID, status, entityType string) ([]Candidate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), runRoles...); err != nil {
		return nil, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	if status != "" && !validReviewStatus(status) {
		return nil, ErrValidation
	}
	if entityType != "" && entityType != string(EntityPerson) && entityType != string(EntityFamily) {
		return nil, ErrValidation
	}
	query := candidateSelect + ` WHERE 1 = 1`
	args := make([]any, 0, 2)
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND review_status = $%d", len(args))
	}
	if entityType != "" {
		args = append(args, entityType)
		query += fmt.Sprintf(" AND entity_type = $%d", len(args))
	}
	query += ` ORDER BY score DESC, created_at DESC LIMIT 100`
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Candidate, 0)
	for rows.Next() {
		candidate, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, rows.Err()
}

func (s *Service) ListMerges(ctx context.Context, actorID string) ([]Merge, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), runRoles...); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id::text, candidate_id::text, entity_type, survivor_id::text, merged_id::text, state,
		       requested_by::text, reason_ar, applied_at, reversed_by::text, reversed_at, reversal_reason_ar
		FROM entity_merges ORDER BY applied_at DESC LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Merge, 0)
	for rows.Next() {
		var item Merge
		var entityType, state, requestedBy, reason string
		var reversedBy, reversalReason pgtype.Text
		var reversedAt pgtype.Timestamptz
		if err := rows.Scan(&item.ID, &item.CandidateID, &entityType, &item.SurvivorID, &item.MergedID, &state, &requestedBy, &reason, &item.AppliedAt, &reversedBy, &reversedAt, &reversalReason); err != nil {
			return nil, err
		}
		item.EntityType = EntityType(entityType)
		item.State = state
		item.RequestedBy = requestedBy
		item.ReasonAR = reason
		item.ReversedBy = textValue(reversedBy)
		if reversedAt.Valid {
			value := reversedAt.Time
			item.ReversedAt = &value
		}
		item.ReversalReasonAR = textValue(reversalReason)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) GetCandidate(ctx context.Context, actorID, candidateID string) (Candidate, error) {
	if err := s.ready(); err != nil {
		return Candidate{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), runRoles...); err != nil {
		return Candidate{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(candidateID))
	if err != nil {
		return Candidate{}, ErrNotFound
	}
	return s.getCandidate(ctx, s.Pool, id)
}

func (s *Service) getCandidate(ctx context.Context, q queryer, id uuid.UUID) (Candidate, error) {
	row := q.QueryRow(ctx, candidateSelect+` WHERE id = $1`, id)
	candidate, err := scanCandidate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Candidate{}, ErrNotFound
	}
	return candidate, err
}

func (s *Service) ReviewCandidate(ctx context.Context, actorID, candidateID string, input ReviewInput) (Candidate, error) {
	if err := s.ready(); err != nil {
		return Candidate{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), runRoles...); err != nil {
		return Candidate{}, err
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(candidateID))
	if err != nil {
		return Candidate{}, ErrNotFound
	}
	decision, nextStatus, err := reviewDecision(input.Decision)
	if err != nil {
		return Candidate{}, err
	}
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if len([]rune(input.NoteAR)) > 4000 || input.ExpectedVersion < 0 {
		return Candidate{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Candidate{}, err
	}
	defer tx.Rollback(ctx)
	var currentStatus string
	var currentVersion int
	if err := tx.QueryRow(ctx, `SELECT review_status, candidate_version FROM entity_resolution_candidates WHERE id = $1 FOR UPDATE`, candidateUUID).Scan(&currentStatus, &currentVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Candidate{}, ErrNotFound
		}
		return Candidate{}, err
	}
	if input.ExpectedVersion > 0 && input.ExpectedVersion != currentVersion {
		return Candidate{}, ErrConflict
	}
	if currentStatus == string(ReviewMerged) || (decision == "approve" && currentStatus != string(ReviewPending) && currentStatus != string(ReviewDeferred) && currentStatus != string(ReviewReopened)) {
		return Candidate{}, ErrConflict
	}
	reviewedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE entity_resolution_candidates
		SET review_status = $1, reviewed_by = $2, reviewed_at = $3, review_note_ar = NULLIF($4, ''), candidate_version = candidate_version + 1, updated_at = now()
		WHERE id = $5
	`, nextStatus, actorID, reviewedAt, input.NoteAR, candidateUUID); err != nil {
		return Candidate{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO entity_resolution_reviews (candidate_id, reviewer_id, decision, note_ar, candidate_version)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5)
	`, candidateUUID, actorID, decision, input.NoteAR, currentVersion+1); err != nil {
		return Candidate{}, err
	}
	if err := writeAudit(ctx, tx, actorID, "entity_resolution_reviewed", "entity_resolution_candidate", candidateUUID, map[string]any{"reviewStatus": currentStatus}, map[string]any{"reviewStatus": nextStatus, "decision": decision, "noteAr": input.NoteAR}, input.NoteAR); err != nil {
		return Candidate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Candidate{}, err
	}
	return s.getCandidate(ctx, s.Pool, candidateUUID)
}

func (s *Service) MergeCandidate(ctx context.Context, actorID, candidateID string, input MergeInput) (Merge, error) {
	if err := s.ready(); err != nil {
		return Merge{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), mergeRoles...); err != nil {
		return Merge{}, err
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(candidateID))
	if err != nil {
		return Merge{}, ErrNotFound
	}
	survivorUUID, err := uuid.Parse(strings.TrimSpace(input.SurvivorEntityID))
	if err != nil {
		return Merge{}, ErrValidation
	}
	input.ReasonAR = strings.TrimSpace(input.ReasonAR)
	if !input.Confirm || input.ExpectedCandidateVersion <= 0 || input.ReasonAR == "" || len([]rune(input.ReasonAR)) > 4000 {
		return Merge{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Merge{}, err
	}
	defer tx.Rollback(ctx)
	var entityType, reviewStatus string
	var leftID, rightID uuid.UUID
	var version int
	if err := tx.QueryRow(ctx, `SELECT entity_type, left_entity_id, right_entity_id, review_status, candidate_version FROM entity_resolution_candidates WHERE id = $1 FOR UPDATE`, candidateUUID).Scan(&entityType, &leftID, &rightID, &reviewStatus, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merge{}, ErrNotFound
		}
		return Merge{}, err
	}
	if input.ExpectedCandidateVersion != version || reviewStatus != string(ReviewApproved) {
		return Merge{}, ErrConflict
	}
	if survivorUUID != leftID && survivorUUID != rightID {
		return Merge{}, ErrValidation
	}
	mergedUUID := leftID
	if survivorUUID == leftID {
		mergedUUID = rightID
	}
	leftState, err := readEntityState(ctx, tx, EntityType(entityType), leftID)
	if err != nil {
		return Merge{}, err
	}
	rightState, err := readEntityState(ctx, tx, EntityType(entityType), rightID)
	if err != nil {
		return Merge{}, err
	}
	if leftState.MergedInto.Valid || rightState.MergedInto.Valid {
		return Merge{}, ErrConflict
	}
	before := stateSnapshot(leftState, rightState)
	mergedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET identity_status = 'merged', merged_into_id = $1, merged_at = $2, updated_at = now() WHERE id = $3`, entityTable(EntityType(entityType))), survivorUUID, mergedAt, mergedUUID); err != nil {
		return Merge{}, err
	}
	after := make(map[string]map[string]any, len(before))
	for entityID, state := range before {
		after[entityID] = state
	}
	after[mergedUUID.String()] = map[string]any{"identity_status": "merged", "merged_into_id": survivorUUID.String(), "merged_at": mergedAt}
	var mergeID uuid.UUID
	var appliedAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO entity_merges (candidate_id, entity_type, survivor_id, merged_id, before_snapshot, after_snapshot, requested_by, reason_ar)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, applied_at
	`, candidateUUID, entityType, survivorUUID, mergedUUID, mustJSON(before), mustJSON(after), actorID, input.ReasonAR).Scan(&mergeID, &appliedAt); err != nil {
		return Merge{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE entity_resolution_candidates SET review_status = 'merged', candidate_version = candidate_version + 1, updated_at = now() WHERE id = $1`, candidateUUID); err != nil {
		return Merge{}, err
	}
	if err := writeAudit(ctx, tx, actorID, "entity_merged", entityType, mergedUUID, before[mergedUUID.String()], after[mergedUUID.String()], input.ReasonAR); err != nil {
		return Merge{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merge{}, err
	}
	return Merge{ID: mergeID.String(), CandidateID: candidateUUID.String(), EntityType: EntityType(entityType), SurvivorID: survivorUUID.String(), MergedID: mergedUUID.String(), State: "applied", RequestedBy: actorID, ReasonAR: input.ReasonAR, AppliedAt: appliedAt}, nil
}

func (s *Service) ReverseMerge(ctx context.Context, actorID, mergeID, reason string) (Merge, error) {
	if err := s.ready(); err != nil {
		return Merge{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), mergeRoles...); err != nil {
		return Merge{}, err
	}
	mergeUUID, err := uuid.Parse(strings.TrimSpace(mergeID))
	if err != nil {
		return Merge{}, ErrNotFound
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 4000 {
		return Merge{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Merge{}, err
	}
	defer tx.Rollback(ctx)
	var candidateID uuid.UUID
	var entityType, state, requestedBy, mergeReason string
	var survivorID, mergedID uuid.UUID
	var appliedAt time.Time
	var beforeJSON []byte
	if err := tx.QueryRow(ctx, `SELECT candidate_id, entity_type, survivor_id, merged_id, state, applied_at, before_snapshot, requested_by::text, reason_ar FROM entity_merges WHERE id = $1 FOR UPDATE`, mergeUUID).Scan(&candidateID, &entityType, &survivorID, &mergedID, &state, &appliedAt, &beforeJSON, &requestedBy, &mergeReason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merge{}, ErrNotFound
		}
		return Merge{}, err
	}
	if state != "applied" {
		return Merge{}, ErrConflict
	}
	var before map[string]map[string]any
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		return Merge{}, ErrConflict
	}
	stateBefore, ok := before[mergedID.String()]
	if !ok {
		return Merge{}, ErrConflict
	}
	current, err := readEntityState(ctx, tx, EntityType(entityType), mergedID)
	if err != nil {
		return Merge{}, err
	}
	if !current.MergedInto.Valid || uuid.UUID(current.MergedInto.Bytes) != survivorID {
		return Merge{}, ErrConflict
	}
	survivorState, err := readEntityState(ctx, tx, EntityType(entityType), survivorID)
	if err != nil {
		return Merge{}, err
	}
	if survivorState.MergedInto.Valid {
		return Merge{}, ErrConflict
	}
	restoredStatus, _ := stateBefore["identity_status"].(string)
	if restoredStatus == "" {
		restoredStatus = "unreviewed"
	}
	restoredMerged := stringValue(stateBefore["merged_into_id"])
	var restoredTime *time.Time
	if value := stringValue(stateBefore["merged_at"]); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, value)
		if parseErr != nil {
			return Merge{}, ErrConflict
		}
		restoredTime = &parsed
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET identity_status = $1, merged_into_id = NULLIF($2, '')::uuid, merged_at = $3, updated_at = now() WHERE id = $4`, entityTable(EntityType(entityType))), restoredStatus, restoredMerged, restoredTime, mergedID); err != nil {
		return Merge{}, err
	}
	reversedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE entity_merges SET state = 'reversed', reversed_by = $1, reversed_at = $2, reversal_reason_ar = $3 WHERE id = $4`, actorID, reversedAt, reason, mergeUUID); err != nil {
		return Merge{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE entity_resolution_candidates SET review_status = 'reopened', candidate_version = candidate_version + 1, reviewed_by = $1, reviewed_at = $2, review_note_ar = $3, updated_at = now() WHERE id = $4`, actorID, reversedAt, reason, candidateID); err != nil {
		return Merge{}, err
	}
	if err := writeAudit(ctx, tx, actorID, "entity_merge_reversed", entityType, mergedID, map[string]any{"mergedIntoId": survivorID.String()}, stateBefore, reason); err != nil {
		return Merge{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merge{}, err
	}
	return Merge{ID: mergeUUID.String(), CandidateID: candidateID.String(), EntityType: EntityType(entityType), SurvivorID: survivorID.String(), MergedID: mergedID.String(), State: "reversed", RequestedBy: requestedBy, ReasonAR: mergeReason, AppliedAt: appliedAt, ReversedBy: actorID, ReversedAt: &reversedAt, ReversalReasonAR: reason}, nil
}

func (s *Service) markRunFailed(ctx context.Context, runID uuid.UUID, cause error) error {
	_, err := s.Pool.Exec(ctx, `UPDATE entity_resolution_runs SET status = 'failed', error = $1, updated_at = now() WHERE id = $2`, "entity resolution run failed", runID)
	return err
}

func reviewDecision(value string) (string, ReviewStatus, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approve":
		return "approve", ReviewApproved, nil
	case "reject":
		return "reject", ReviewRejected, nil
	case "defer":
		return "defer", ReviewDeferred, nil
	case "reopen":
		return "reopen", ReviewReopened, nil
	default:
		return "", "", ErrValidation
	}
}

func validReviewStatus(value string) bool {
	switch value {
	case string(ReviewPending), string(ReviewApproved), string(ReviewRejected), string(ReviewDeferred), string(ReviewReopened), string(ReviewMerged):
		return true
	default:
		return false
	}
}

type entityState struct {
	ID         uuid.UUID
	Status     string
	MergedInto pgtype.UUID
	MergedAt   pgtype.Timestamptz
}

func readEntityState(ctx context.Context, tx pgx.Tx, entityType EntityType, id uuid.UUID) (entityState, error) {
	if entityType != EntityPerson && entityType != EntityFamily {
		return entityState{}, ErrValidation
	}
	var result entityState
	query := fmt.Sprintf(`SELECT identity_status, merged_into_id, merged_at FROM %s WHERE id = $1 FOR UPDATE`, entityTable(entityType))
	result.ID = id
	if err := tx.QueryRow(ctx, query, id).Scan(&result.Status, &result.MergedInto, &result.MergedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entityState{}, ErrNotFound
		}
		return entityState{}, err
	}
	return result, nil
}

func entityTable(entityType EntityType) string {
	if entityType == EntityFamily {
		return "families"
	}
	return "people"
}

func stateSnapshot(left, right entityState) map[string]map[string]any {
	return map[string]map[string]any{
		left.ID.String():  stateMap(left),
		right.ID.String(): stateMap(right),
	}
}

func stateMap(value entityState) map[string]any {
	result := map[string]any{"identity_status": value.Status}
	if value.MergedInto.Valid {
		result["merged_into_id"] = uuid.UUID(value.MergedInto.Bytes).String()
	} else {
		result["merged_into_id"] = ""
	}
	if value.MergedAt.Valid {
		result["merged_at"] = value.MergedAt.Time.Format(time.RFC3339Nano)
	} else {
		result["merged_at"] = ""
	}
	return result
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

const candidateSelect = `
	SELECT id::text, run_id::text, entity_type, left_entity_id::text, right_entity_id::text, left_name_ar, right_name_ar,
	       match_class, score, score_components, matching_signals, conflicting_signals, explanation_ar,
	       review_status, candidate_version, reviewed_by::text, reviewed_at, review_note_ar, created_at, updated_at
	FROM entity_resolution_candidates`

func scanCandidate(row pgx.Row) (Candidate, error) {
	var result Candidate
	var entityType, matchClass, reviewStatus string
	var scoreComponents, matchingSignals, conflictingSignals []byte
	var reviewedBy, reviewNote pgtype.Text
	var reviewedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.RunID, &entityType, &result.LeftEntityID, &result.RightEntityID, &result.LeftNameAR, &result.RightNameAR, &matchClass, &result.Score, &scoreComponents, &matchingSignals, &conflictingSignals, &result.ExplanationAR, &reviewStatus, &result.CandidateVersion, &reviewedBy, &reviewedAt, &reviewNote, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return Candidate{}, err
	}
	result.EntityType = EntityType(entityType)
	result.MatchClass = MatchClass(matchClass)
	result.ReviewStatus = ReviewStatus(reviewStatus)
	result.ScoreComponents = map[string]float64{}
	if len(scoreComponents) > 0 {
		_ = json.Unmarshal(scoreComponents, &result.ScoreComponents)
	}
	result.MatchingSignals = []Signal{}
	if len(matchingSignals) > 0 {
		_ = json.Unmarshal(matchingSignals, &result.MatchingSignals)
	}
	result.ConflictingSignals = []Signal{}
	if len(conflictingSignals) > 0 {
		_ = json.Unmarshal(conflictingSignals, &result.ConflictingSignals)
	}
	result.ReviewedBy = textValue(reviewedBy)
	if reviewedAt.Valid {
		value := reviewedAt.Time
		result.ReviewedAt = &value
	}
	result.ReviewNoteAR = textValue(reviewNote)
	result.RequiresHumanReview = result.ReviewStatus != ReviewMerged
	return result, nil
}

func writeAudit(ctx context.Context, tx pgx.Tx, actorID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
	`, actorID, action, entityType, entityID, mustJSON(before), mustJSON(after), reason)
	return err
}
