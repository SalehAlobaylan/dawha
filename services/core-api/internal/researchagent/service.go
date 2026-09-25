package researchagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) StartRun(ctx context.Context, actorID string, input RunInput) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	input, err := validateInput(input)
	if err != nil {
		return Run{}, err
	}
	if input.EntityType == "source" && input.SourceID == "" {
		input.SourceID = input.EntityID
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return Run{}, ErrForbidden
	}
	if err := requireRole(ctx, s.Pool, actorUUID); err != nil {
		return Run{}, err
	}
	scope, err := loadScope(ctx, s.Pool, input, actorUUID)
	if err != nil {
		return Run{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	questionUUID, err := optionalUUID(input.QuestionID)
	if err != nil {
		return Run{}, ErrValidation
	}
	if questionUUID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM open_questions WHERE id = $1)`, questionUUID).Scan(&exists); err != nil {
			return Run{}, err
		}
		if !exists {
			return Run{}, ErrNotFound
		}
	}
	plan, terms := decomposeQuestion(input)
	stage := stageContext{Input: input, ActorUUID: actorUUID, Terms: terms, TreeID: scope.TreeID, TreeVersion: scope.TreeVersionID}
	candidateSourceIDs, err := loadCandidateSourceIDs(ctx, tx, input)
	if err != nil {
		return Run{}, err
	}
	stage.SourceIDs = candidateSourceIDs
	runID := uuid.New()
	startedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO research_agent_runs (id, requested_by, question_id, query, normalized_query, entity_type, entity_id, tree_id, tree_version_id, status, resolution, execution_mode, planner_version, algorithm_version, qualification_policy_version, started_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'running', 'unresolved', 'synchronous', $10, $11, $12, $13)`, runID, actorUUID, questionUUID, input.Question, identity.NormalizeArabicName(input.Question), input.EntityType, input.EntityID, nullableString(scope.TreeID), nullableString(scope.TreeVersionID), PlannerVersion, AlgorithmVersion, QualificationPolicy, startedAt); err != nil {
		return Run{}, err
	}
	steps := make([]Step, 0, len(plan))
	allEvidence := make([]EvidenceRef, 0)
	stageInput := map[string]any{"entityType": input.EntityType, "entityId": input.EntityID, "questionId": input.QuestionID, "treeVersionId": scope.TreeVersionID}
	for _, planStep := range plan {
		stepStarted := time.Now().UTC()
		refs, output, unresolved, stageErr := executeStage(ctx, tx, stage, planStep.Stage)
		stepCompleted := time.Now().UTC()
		if stageErr != nil {
			return Run{}, stageErr
		}
		stepID := uuid.New()
		status := "succeeded"
		if unresolved {
			status = "unresolved"
		}
		step := Step{ID: stepID.String(), Order: planStep.Order, Stage: planStep.Stage, Tool: planStep.Tool, Status: status, Input: stageInput, Output: output, EvidenceCount: len(refs), StartedAt: stepStarted, CompletedAt: &stepCompleted}
		if _, err := tx.Exec(ctx, `INSERT INTO research_agent_steps (id, run_id, step_order, stage, tool_name, status, input, output, evidence_count, started_at, completed_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, stepID, runID, planStep.Order, planStep.Stage, planStep.Tool, status, mustJSON(stageInput), mustJSON(output), len(refs), stepStarted, stepCompleted); err != nil {
			return Run{}, err
		}
		for _, item := range refs {
			item.StepID = stepID.String()
			if err := persistEvidence(ctx, tx, runID, item); err != nil {
				return Run{}, err
			}
		}
		allEvidence = mergeEvidence(allEvidence, refs)
		steps = append(steps, step)
		if planStep.Stage == StageSearchSources {
			stage.SourceIDs = sourceIDsFromEvidence(stage.SourceIDs, refs)
		}
	}
	packageData := buildEvidencePackage(allEvidence)
	gaps, recommendations := buildGaps(runID.String(), input, allEvidence, packageData.SourceCount, countEvidence(allEvidence, "research_claim"), countEvidence(allEvidence, "source_dependency"), countEvidence(allEvidence, "tree_interpretation"), countEvidence(allEvidence, "geographic_signal"), countEvidence(allEvidence, "temporal_signal"))
	resolution := ResolutionSucceeded
	if len(gaps) > 0 {
		resolution = ResolutionUnresolved
	}
	report := Report{AnswerAR: answerText(packageData, resolution), Plan: plan, Terms: terms, Scope: Scope{EntityType: input.EntityType, EntityID: input.EntityID, TreeID: scope.TreeID, TreeVersionID: scope.TreeVersionID, SourceID: input.SourceID, PersonID: input.PersonID, PlaceID: input.PlaceID, FromYear: input.FromYear, ToYear: input.ToYear}, EvidencePackage: packageData, AllowedActions: []string{"read_sources", "inspect_graph", "inspect_geography", "inspect_chronology", "inspect_claims", "inspect_dependencies"}, RestrictedActions: []string{"publish_tree", "accept_claim", "merge_people", "resolve_dispute", "modify_source_evidence"}, UnresolvedReasons: unresolvedReasons(gaps), GeneratedAt: time.Now().UTC()}
	for _, gap := range gaps {
		if _, err := tx.Exec(ctx, `INSERT INTO research_agent_gaps (id, run_id, kind, description_ar, severity, metadata) VALUES ($1, $2, $3, $4, $5, $6)`, uuid.MustParse(gap.ID), runID, gap.Kind, gap.DescriptionAR, gap.Severity, mustJSON(gap.Metadata)); err != nil {
			return Run{}, err
		}
	}
	for _, recommendation := range recommendations {
		if _, err := tx.Exec(ctx, `INSERT INTO research_agent_recommendations (id, run_id, action, rationale_ar, priority, metadata) VALUES ($1, $2, $3, $4, $5, $6)`, uuid.MustParse(recommendation.ID), runID, recommendation.Action, recommendation.RationaleAR, recommendation.Priority, mustJSON(recommendation.Metadata)); err != nil {
			return Run{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE research_agent_runs SET status = 'succeeded', resolution = $1, report = $2, step_count = $3, evidence_count = $4, gap_count = $5, recommendation_count = $6, completed_at = now(), updated_at = now() WHERE id = $7`, resolution, mustJSON(report), len(steps), len(allEvidence), len(gaps), len(recommendations), runID); err != nil {
		return Run{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "research_agent_run_completed", "research_agent_run", runID, nil, map[string]any{"resolution": resolution, "stepCount": len(steps), "evidenceCount": len(allEvidence), "gapCount": len(gaps)}, ""); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	completedAt := time.Now().UTC()
	return Run{ID: runID.String(), RequestedBy: actorUUID.String(), QuestionID: input.QuestionID, Query: input.Question, NormalizedQuery: identity.NormalizeArabicName(input.Question), EntityType: input.EntityType, EntityID: input.EntityID, TreeID: scope.TreeID, TreeVersionID: scope.TreeVersionID, Status: "succeeded", Resolution: resolution, ExecutionMode: "synchronous", PlannerVersion: PlannerVersion, AlgorithmVersion: AlgorithmVersion, QualificationPolicyVersion: QualificationPolicy, Report: report, StepCount: len(steps), EvidenceCount: len(allEvidence), GapCount: len(gaps), RecommendationCount: len(recommendations), CreatedAt: startedAt, StartedAt: &startedAt, CompletedAt: &completedAt, UpdatedAt: completedAt, Steps: steps, Evidence: allEvidence, Gaps: gaps, Recommendations: recommendations}, nil
}

func executeStage(ctx context.Context, q queryer, stage stageContext, stageName string) ([]EvidenceRef, map[string]any, bool, error) {
	output := map[string]any{}
	switch stageName {
	case StageDecompose:
		output["terms"] = stage.Terms
		output["readOnly"] = true
		return nil, output, len(stage.Terms) == 0, nil
	case StageSearchSources:
		items, err := searchSources(ctx, q, stage)
		return items, map[string]any{"count": len(items), "sourceIds": sourceIDsFromEvidence(nil, items)}, len(items) == 0, err
	case StageSearchGraph:
		items, err := searchGraph(ctx, q, stage)
		return items, map[string]any{"count": len(items), "treeVersionId": stage.TreeVersion}, len(items) == 0, err
	case StageInspectGeography:
		items, err := inspectGeography(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageInspectChronology:
		items, err := inspectChronology(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageCompareClaims:
		items, err := compareClaims(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageSourceDependency:
		items, err := inspectSourceDependency(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageCounterEvidence:
		items, err := retrieveCounterEvidence(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageEvidencePackage:
		return nil, map[string]any{"readOnly": true}, false, nil
	case StageMissingEvidence, StageRecommendation:
		return nil, map[string]any{"readOnly": true}, false, nil
	default:
		return nil, output, true, ErrValidation
	}
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, actorUUID(actorID)); err != nil {
		return Run{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return Run{}, ErrNotFound
	}
	result, err := scanRun(s.Pool.QueryRow(ctx, runSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if err := s.requireRunAccess(ctx, s.Pool, result, actorID); err != nil {
		return Run{}, err
	}
	if err := s.loadDetails(ctx, &result); err != nil {
		return Run{}, err
	}
	return result, nil
}

func (s *Service) GetLatestRun(ctx context.Context, actorID, questionID, entityType, entityID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, actorUUID(actorID)); err != nil {
		return Run{}, err
	}
	filters := []struct{ column, value string }{{"question_id", questionID}, {"entity_type", entityType}, {"entity_id", entityID}}
	query := runSelect + ` WHERE 1 = 1`
	args := make([]any, 0, len(filters))
	for _, filter := range filters {
		if strings.TrimSpace(filter.value) == "" {
			continue
		}
		if filter.column == "entity_type" {
			args = append(args, strings.ToLower(strings.TrimSpace(filter.value)))
		} else {
			parsed, parseErr := uuid.Parse(strings.TrimSpace(filter.value))
			if parseErr != nil {
				return Run{}, ErrValidation
			}
			args = append(args, parsed)
		}
		query += fmt.Sprintf(" AND %s = $%d", filter.column, len(args))
	}
	query += ` ORDER BY created_at DESC LIMIT 50`
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return Run{}, err
	}
	defer rows.Close()
	for rows.Next() {
		result, scanErr := scanRun(rows)
		if scanErr != nil {
			return Run{}, scanErr
		}
		if accessErr := s.requireRunAccess(ctx, s.Pool, result, actorID); accessErr == nil {
			if err := s.loadDetails(ctx, &result); err != nil {
				return Run{}, err
			}
			return result, nil
		} else if !errors.Is(accessErr, ErrForbidden) && !errors.Is(accessErr, ErrNotFound) {
			return Run{}, accessErr
		}
	}
	if err := rows.Err(); err != nil {
		return Run{}, err
	}
	return Run{}, ErrNotFound
}

func (s *Service) loadDetails(ctx context.Context, run *Run) error {
	stepRows, err := s.Pool.Query(ctx, `SELECT id, step_order, stage, tool_name, status, input, output, evidence_count, error, started_at, completed_at FROM research_agent_steps WHERE run_id = $1 ORDER BY step_order`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer stepRows.Close()
	run.Steps = make([]Step, 0)
	for stepRows.Next() {
		var step Step
		var id pgtype.UUID
		var inputData, outputData []byte
		var stepError pgtype.Text
		var completed pgtype.Timestamptz
		if err := stepRows.Scan(&id, &step.Order, &step.Stage, &step.Tool, &step.Status, &inputData, &outputData, &step.EvidenceCount, &stepError, &step.StartedAt, &completed); err != nil {
			return err
		}
		step.ID = uuidString(id)
		step.Input = decodeMap(inputData)
		step.Output = decodeMap(outputData)
		step.Error = textValue(stepError)
		if completed.Valid {
			value := completed.Time
			step.CompletedAt = &value
		}
		run.Steps = append(run.Steps, step)
	}
	if err := stepRows.Err(); err != nil {
		return err
	}
	evidenceRows, err := s.Pool.Query(ctx, `SELECT id, step_id, layer, stance, reference_type, reference_id, COALESCE(source_id::text, ''), COALESCE(statement_id::text, ''), COALESCE(claim_id::text, ''), excerpt, metadata FROM research_agent_evidence WHERE run_id = $1 ORDER BY created_at, id`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer evidenceRows.Close()
	run.Evidence = make([]EvidenceRef, 0)
	for evidenceRows.Next() {
		var item EvidenceRef
		var id, stepID pgtype.UUID
		var referenceID, sourceID, statementID, claimID string
		var metadata []byte
		if err := evidenceRows.Scan(&id, &stepID, &item.Layer, &item.Stance, &item.ReferenceType, &referenceID, &sourceID, &statementID, &claimID, &item.Excerpt, &metadata); err != nil {
			return err
		}
		item.ID = uuidString(id)
		item.StepID = uuidString(stepID)
		item.ReferenceID = referenceID
		item.SourceID = sourceID
		item.StatementID = statementID
		item.ClaimID = claimID
		item.Metadata = decodeMap(metadata)
		run.Evidence = append(run.Evidence, item)
	}
	if err := evidenceRows.Err(); err != nil {
		return err
	}
	gapRows, err := s.Pool.Query(ctx, `SELECT id, kind, description_ar, severity, status, metadata FROM research_agent_gaps WHERE run_id = $1 ORDER BY created_at, id`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer gapRows.Close()
	run.Gaps = make([]Gap, 0)
	for gapRows.Next() {
		var item Gap
		var id pgtype.UUID
		var metadata []byte
		if err := gapRows.Scan(&id, &item.Kind, &item.DescriptionAR, &item.Severity, &item.Status, &metadata); err != nil {
			return err
		}
		item.ID = uuidString(id)
		item.Metadata = decodeMap(metadata)
		run.Gaps = append(run.Gaps, item)
	}
	if err := gapRows.Err(); err != nil {
		return err
	}
	recommendationRows, err := s.Pool.Query(ctx, `SELECT id, action, rationale_ar, priority, status, metadata FROM research_agent_recommendations WHERE run_id = $1 ORDER BY created_at, id`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer recommendationRows.Close()
	run.Recommendations = make([]Recommendation, 0)
	for recommendationRows.Next() {
		var item Recommendation
		var id pgtype.UUID
		var metadata []byte
		if err := recommendationRows.Scan(&id, &item.Action, &item.RationaleAR, &item.Priority, &item.Status, &metadata); err != nil {
			return err
		}
		item.ID = uuidString(id)
		item.Metadata = decodeMap(metadata)
		run.Recommendations = append(run.Recommendations, item)
	}
	return recommendationRows.Err()
}

func (s *Service) requireRunAccess(ctx context.Context, q queryer, run Run, actorID string) error {
	if run.TreeID != "" {
		actor, err := uuid.Parse(strings.TrimSpace(actorID))
		if err != nil {
			return ErrForbidden
		}
		var allowed bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trees t JOIN tree_versions tv ON tv.tree_id = t.id AND tv.id = $3 WHERE t.id = $1 AND tv.state = 'published' AND (t.visibility = 'public' OR t.owner_id = $2 OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2)))`, run.TreeID, actor, run.TreeVersionID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return ErrForbidden
		}
	}
	if run.EntityType == "source" {
		actor, err := uuid.Parse(strings.TrimSpace(actorID))
		if err != nil {
			return ErrForbidden
		}
		var allowed bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sources WHERE id = $1 AND (visibility = 'public' OR created_by = $2))`, run.EntityID, actor).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return ErrForbidden
		}
	}
	return nil
}

func validateInput(input RunInput) (RunInput, error) {
	input.Question = strings.TrimSpace(input.Question)
	input.QuestionID = strings.TrimSpace(input.QuestionID)
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.EntityID = strings.TrimSpace(input.EntityID)
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.PersonID = strings.TrimSpace(input.PersonID)
	input.PlaceID = strings.TrimSpace(input.PlaceID)
	if input.Question == "" || len([]rune(input.Question)) > 4000 {
		return RunInput{}, ErrValidation
	}
	if input.EntityType == "" {
		input.EntityType = "person"
	}
	if input.EntityType != "person" && input.EntityType != "family" && input.EntityType != "branch" && input.EntityType != "place" && input.EntityType != "source" {
		return RunInput{}, ErrValidation
	}
	for _, value := range []string{input.QuestionID, input.EntityID, input.TreeID, input.TreeVersionID, input.SourceID, input.PersonID, input.PlaceID} {
		if value == "" {
			continue
		}
		if _, err := uuid.Parse(value); err != nil {
			return RunInput{}, ErrValidation
		}
	}
	if (input.TreeID == "") != (input.TreeVersionID == "") {
		return RunInput{}, ErrValidation
	}
	if input.EntityID == "" {
		return RunInput{}, ErrValidation
	}
	if input.EntityType == "source" && input.SourceID != "" && input.SourceID != input.EntityID {
		return RunInput{}, ErrValidation
	}
	if input.FromYear < 0 || input.ToYear < 0 || (input.FromYear > 0 && input.ToYear > 0 && input.FromYear > input.ToYear) {
		return RunInput{}, ErrValidation
	}
	return input, nil
}

func loadScope(ctx context.Context, q queryer, input RunInput, actor uuid.UUID) (Scope, error) {
	scope := Scope{EntityType: input.EntityType, EntityID: input.EntityID, TreeID: input.TreeID, TreeVersionID: input.TreeVersionID, SourceID: input.SourceID, PersonID: input.PersonID, PlaceID: input.PlaceID, FromYear: input.FromYear, ToYear: input.ToYear}
	if input.TreeID != "" {
		var visibility, state string
		if err := q.QueryRow(ctx, `SELECT t.visibility, tv.state FROM trees t JOIN tree_versions tv ON tv.tree_id = t.id WHERE t.id = $1 AND tv.id = $2`, input.TreeID, input.TreeVersionID).Scan(&visibility, &state); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Scope{}, ErrNotFound
			}
			return Scope{}, err
		}
		if visibility != "public" || state != "published" {
			return Scope{}, ErrForbidden
		}
		if input.EntityType == "person" {
			var targetPresent bool
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tree_nodes WHERE tree_version_id = $1 AND person_id = $2)`, input.TreeVersionID, input.EntityID).Scan(&targetPresent); err != nil {
				return Scope{}, err
			}
			if !targetPresent {
				return Scope{}, ErrValidation
			}
		}
	}
	if input.EntityType == "source" {
		if err := requireSourceAccess(ctx, q, input.EntityID, actor); err != nil {
			return Scope{}, err
		}
	}
	if input.SourceID != "" && input.SourceID != input.EntityID {
		if err := requireSourceAccess(ctx, q, input.SourceID, actor); err != nil {
			return Scope{}, err
		}
	}
	var entityExists bool
	var entityQuery string
	switch input.EntityType {
	case "person":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM people WHERE id = $1)`
	case "family":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM families WHERE id = $1)`
	case "branch":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1)`
	case "place":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM places WHERE id = $1)`
	case "source":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM sources WHERE id = $1)`
	default:
		return Scope{}, ErrValidation
	}
	if err := q.QueryRow(ctx, entityQuery, input.EntityID).Scan(&entityExists); err != nil {
		return Scope{}, err
	}
	if !entityExists {
		return Scope{}, ErrNotFound
	}
	return scope, nil
}

func requireSourceAccess(ctx context.Context, q queryer, sourceID string, actor uuid.UUID) error {
	var visibility string
	var owner pgtype.UUID
	if err := q.QueryRow(ctx, `SELECT visibility, created_by FROM sources WHERE id = $1`, sourceID).Scan(&visibility, &owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if visibility != "public" && (!owner.Valid || uuid.UUID(owner.Bytes).String() != actor.String()) {
		return ErrForbidden
	}
	return nil
}

func requireRole(ctx context.Context, q queryer, actor uuid.UUID) error {
	var allowed bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role = ANY($2::text[]))`, actor, []string{"researcher", "moderator", "admin"}).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func actorUUID(value string) uuid.UUID {
	parsed, _ := uuid.Parse(strings.TrimSpace(value))
	return parsed
}

func optionalUUID(value string) (any, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	return uuid.Parse(strings.TrimSpace(value))
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func loadCandidateSourceIDs(ctx context.Context, q queryer, input RunInput) ([]uuid.UUID, error) {
	items := make([]uuid.UUID, 0)
	seen := make(map[string]struct{})
	add := func(value string) {
		parsed, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return
		}
		if _, found := seen[parsed.String()]; found {
			return
		}
		seen[parsed.String()] = struct{}{}
		items = append(items, parsed)
	}
	add(input.SourceID)
	if input.EntityType == "source" {
		return items, nil
	}
	questionID, err := optionalUUID(input.QuestionID)
	if err != nil {
		return nil, err
	}
	questionUUID, _ := questionID.(uuid.UUID)
	var questionArg any
	if questionID != nil {
		questionArg = questionUUID
	}
	entityUUID, _ := uuid.Parse(input.EntityID)
	rows, err := q.Query(ctx, `
		SELECT source_id::text FROM question_sources WHERE question_id = $1
		UNION
		SELECT ss.source_id::text FROM question_claims qc JOIN claim_evidence ce ON ce.claim_id = qc.claim_id JOIN source_statements ss ON ss.id = ce.source_statement_id WHERE qc.question_id = $1
		UNION
		SELECT ss.source_id::text FROM claims c JOIN claim_evidence ce ON ce.claim_id = c.id JOIN source_statements ss ON ss.id = ce.source_statement_id WHERE c.subject_id = $2 OR c.object_id = $2
		LIMIT 100
	`, questionArg, entityUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		add(value)
	}
	return items, rows.Err()
}

func persistEvidence(ctx context.Context, tx pgx.Tx, runID uuid.UUID, item EvidenceRef) error {
	_, err := tx.Exec(ctx, `INSERT INTO research_agent_evidence (run_id, step_id, layer, stance, reference_type, reference_id, source_id, statement_id, claim_id, excerpt, metadata) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT DO NOTHING`, runID, item.StepID, item.Layer, item.Stance, item.ReferenceType, item.ReferenceID, nullableUUIDValue(item.SourceID), nullableUUIDValue(item.StatementID), nullableUUIDValue(item.ClaimID), item.Excerpt, mustJSON(item.Metadata))
	return err
}

func writeAudit(ctx context.Context, tx pgx.Tx, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))`, actor, action, entityType, entityID, mustJSON(before), mustJSON(after), reason)
	return err
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func decodeMap(data []byte) map[string]any {
	result := map[string]any{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &result)
	}
	return result
}

const runSelect = `
	SELECT id::text, requested_by::text, COALESCE(question_id::text, ''), query, normalized_query, entity_type, entity_id::text,
	       COALESCE(tree_id::text, ''), COALESCE(tree_version_id::text, ''), status, resolution, execution_mode,
	       planner_version, algorithm_version, qualification_policy_version, report, step_count, evidence_count, gap_count,
	       recommendation_count, COALESCE(error, ''), created_at, started_at, completed_at, updated_at
	FROM research_agent_runs`

func scanRun(row pgx.Row) (Run, error) {
	var result Run
	var questionID, treeID, treeVersionID, runError pgtype.Text
	var reportData []byte
	var startedAt, completedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.RequestedBy, &questionID, &result.Query, &result.NormalizedQuery, &result.EntityType, &result.EntityID, &treeID, &treeVersionID, &result.Status, &result.Resolution, &result.ExecutionMode, &result.PlannerVersion, &result.AlgorithmVersion, &result.QualificationPolicyVersion, &reportData, &result.StepCount, &result.EvidenceCount, &result.GapCount, &result.RecommendationCount, &runError, &result.CreatedAt, &startedAt, &completedAt, &result.UpdatedAt); err != nil {
		return Run{}, err
	}
	result.QuestionID = textValue(questionID)
	result.TreeID = textValue(treeID)
	result.TreeVersionID = textValue(treeVersionID)
	result.Error = textValue(runError)
	if len(reportData) > 0 {
		_ = json.Unmarshal(reportData, &result.Report)
	}
	if result.Report.Plan == nil {
		result.Report.Plan = []PlanStep{}
	}
	if result.Report.Terms == nil {
		result.Report.Terms = []string{}
	}
	if result.Report.AllowedActions == nil {
		result.Report.AllowedActions = []string{}
	}
	if result.Report.RestrictedActions == nil {
		result.Report.RestrictedActions = []string{}
	}
	if result.Report.UnresolvedReasons == nil {
		result.Report.UnresolvedReasons = []string{}
	}
	if result.Report.EvidencePackage.Evidence == nil {
		result.Report.EvidencePackage.Evidence = []EvidenceRef{}
	}
	if startedAt.Valid {
		value := startedAt.Time
		result.StartedAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		result.CompletedAt = &value
	}
	return result, nil
}

func sourceIDsFromEvidence(existing []uuid.UUID, evidence []EvidenceRef) []uuid.UUID {
	seen := make(map[string]struct{}, len(existing))
	items := make([]uuid.UUID, 0, len(existing))
	for _, value := range existing {
		seen[value.String()] = struct{}{}
		items = append(items, value)
	}
	for _, item := range evidence {
		if item.SourceID == "" {
			continue
		}
		parsed, err := uuid.Parse(item.SourceID)
		if err != nil {
			continue
		}
		if _, found := seen[parsed.String()]; found {
			continue
		}
		seen[parsed.String()] = struct{}{}
		items = append(items, parsed)
	}
	return items
}

func countEvidence(evidence []EvidenceRef, layer string) int {
	count := 0
	for _, item := range evidence {
		if item.Layer == layer {
			count++
		}
	}
	return count
}

func answerText(packageData EvidencePackage, resolution string) string {
	if packageData.Total == 0 {
		return "لم تُجمع أدلة مؤهلة كافية؛ يبقى السؤال غير محسوم ويحتاج إلى توسيع النطاق."
	}
	if resolution == ResolutionUnresolved {
		return fmt.Sprintf("جمعت حزمة من %d مادة (%d داعمة و%d مضادة)، لكنها لا تحسم السؤال بسبب الفجوات المسجلة.", packageData.Total, packageData.SupportCount, packageData.CounterCount)
	}
	return fmt.Sprintf("جمعت حزمة قابلة للتتبع من %d مادة عبر %d مصادر؛ راجع الاقتباسات قبل أي قرار.", packageData.Total, packageData.SourceCount)
}

func unresolvedReasons(gaps []Gap) []string {
	reasons := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		reasons = append(reasons, gap.DescriptionAR)
	}
	return reasons
}
