package temporalanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Pool *pgxpool.Pool
}

type analysisResult struct {
	Reference    ReferencePopulation
	ReportStatus string
	Findings     []findingDraft
}

type findingDraft struct {
	RelationshipID string
	ParentPersonID string
	ChildPersonID  string
	ClaimID        string
	Comparison     Comparison
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

func (s *Service) StartRun(ctx context.Context, actorID string, input StartRunInput) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	input, err := validateStartInput(input)
	if err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return Run{}, err
	}
	actorUUID, _ := uuid.Parse(strings.TrimSpace(actorID))
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	scope, err := loadTreeScope(ctx, tx, input.TreeID, input.TreeVersionID, input.TargetPersonID)
	if err != nil {
		return Run{}, err
	}
	if scope.Visibility != "public" {
		return Run{}, ErrForbidden
	}
	if scope.VersionState != "published" {
		return Run{}, ErrValidation
	}
	if !scope.TargetPresent {
		return Run{}, ErrValidation
	}
	rows, truncated, err := loadGenerationEdges(ctx, tx, input.TreeVersionID, input.TargetPersonID)
	if err != nil {
		return Run{}, err
	}
	analysis := analyzeGenerationEdges(scope, rows, input.TargetPersonID, input.MinReferenceSize, truncated)
	runID := uuid.New()
	questionArg, err := optionalUUID(input.QuestionID)
	if err != nil {
		return Run{}, ErrValidation
	}
	if questionArg != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM open_questions WHERE id = $1)`, questionArg).Scan(&exists); err != nil {
			return Run{}, err
		}
		if !exists {
			return Run{}, ErrNotFound
		}
	}
	startedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO temporal_analysis_runs
			(id, requested_by, question_id, tree_id, tree_version_id, target_person_id, status, report_status,
			 execution_mode, algorithm_version, qualification_policy_version, min_reference_size,
			 reference_population, finding_count, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'succeeded', $7, 'synchronous', $8, $9, $10, $11, $12, $13, $13)
	`, runID, actorUUID, questionArg, input.TreeID, input.TreeVersionID, input.TargetPersonID, analysis.ReportStatus,
		AlgorithmVersion, QualificationPolicyVersion, input.MinReferenceSize, mustJSON(analysis.Reference), len(analysis.Findings), startedAt); err != nil {
		return Run{}, err
	}
	for _, draft := range analysis.Findings {
		if err := persistFinding(ctx, tx, runID, actorUUID, draft, analysis.Reference, questionArg); err != nil {
			return Run{}, err
		}
	}
	if err := writeAudit(ctx, tx, actorUUID, "temporal_analysis_run_completed", "temporal_analysis_run", runID, nil, map[string]any{
		"reportStatus": analysis.ReportStatus,
		"findingCount": len(analysis.Findings),
		"algorithm":    AlgorithmVersion,
		"policy":       QualificationPolicyVersion,
	}, ""); err != nil {
		return Run{}, err
	}
	completedAt := startedAt
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return Run{
		ID:                         runID.String(),
		RequestedBy:                actorID,
		QuestionID:                 input.QuestionID,
		TreeID:                     input.TreeID,
		TreeVersionID:              input.TreeVersionID,
		TargetPersonID:             input.TargetPersonID,
		Status:                     "succeeded",
		ReportStatus:               analysis.ReportStatus,
		ExecutionMode:              "synchronous",
		AlgorithmVersion:           AlgorithmVersion,
		QualificationPolicyVersion: QualificationPolicyVersion,
		MinReferenceSize:           input.MinReferenceSize,
		ReferencePopulation:        analysis.Reference,
		FindingCount:               len(analysis.Findings),
		CreatedAt:                  startedAt,
		StartedAt:                  &startedAt,
		CompletedAt:                &completedAt,
		UpdatedAt:                  startedAt,
	}, nil
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return Run{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return Run{}, ErrNotFound
	}
	result, err := scanRun(s.Pool.QueryRow(ctx, temporalRunSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if err := requireTreeAccess(ctx, s.Pool, result.TreeID, actorID); err != nil {
		return Run{}, err
	}
	return result, nil
}

func (s *Service) GetLatestRun(ctx context.Context, actorID, questionID, treeID, treeVersionID, targetPersonID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return Run{}, err
	}
	filters := []struct {
		column string
		value  string
	}{
		{column: "question_id", value: questionID},
		{column: "tree_id", value: treeID},
		{column: "tree_version_id", value: treeVersionID},
		{column: "target_person_id", value: targetPersonID},
	}
	query := temporalRunSelect + ` WHERE 1 = 1
		AND EXISTS (
			SELECT 1 FROM trees accessible_tree
			WHERE accessible_tree.id = temporal_analysis_runs.tree_id
			  AND (accessible_tree.visibility = 'public' OR accessible_tree.owner_id = $1 OR EXISTS (
				SELECT 1 FROM tree_collaborators accessible_collaborator
				WHERE accessible_collaborator.tree_id = accessible_tree.id AND accessible_collaborator.user_id = $1
			  ))
		)`
	actor, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return Run{}, ErrForbidden
	}
	args := make([]any, 1, len(filters)+1)
	args[0] = actor
	for _, filter := range filters {
		if strings.TrimSpace(filter.value) == "" {
			continue
		}
		id, err := uuid.Parse(strings.TrimSpace(filter.value))
		if err != nil {
			return Run{}, ErrValidation
		}
		args = append(args, id)
		query += fmt.Sprintf(" AND %s = $%d", filter.column, len(args))
	}
	query += ` ORDER BY created_at DESC LIMIT 1`
	result, err := scanRun(s.Pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return result, err
}

func (s *Service) ListFindings(ctx context.Context, actorID, runID, status string) ([]Finding, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return nil, err
	}
	actor, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return nil, ErrForbidden
	}
	query := temporalFindingSelect + ` WHERE f.temporal_run_id IS NOT NULL
		AND EXISTS (
			SELECT 1 FROM trees accessible_tree
			WHERE accessible_tree.id = temporal_run.tree_id
			  AND (accessible_tree.visibility = 'public' OR accessible_tree.owner_id = $1 OR EXISTS (
				SELECT 1 FROM tree_collaborators accessible_collaborator
				WHERE accessible_collaborator.tree_id = accessible_tree.id AND accessible_collaborator.user_id = $1
			  ))
		)`
	args := make([]any, 1, 3)
	args[0] = actor
	if strings.TrimSpace(runID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(runID))
		if err != nil {
			return nil, ErrNotFound
		}
		args = append(args, id)
		query += fmt.Sprintf(" AND f.temporal_run_id = $%d", len(args))
	}
	if strings.TrimSpace(status) != "" {
		status = strings.ToLower(strings.TrimSpace(status))
		if !validFindingStatus(status) {
			return nil, ErrValidation
		}
		args = append(args, status)
		query += fmt.Sprintf(" AND f.status = $%d", len(args))
	}
	query += ` ORDER BY f.created_at DESC LIMIT 200`
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Finding, 0)
	for rows.Next() {
		finding, scanErr := scanTemporalFinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if populateErr := populateTemporalFinding(ctx, s.Pool, &finding); populateErr != nil {
			return nil, populateErr
		}
		items = append(items, finding)
	}
	return items, rows.Err()
}

func (s *Service) GetFinding(ctx context.Context, actorID, findingID string) (Finding, error) {
	if err := s.ready(); err != nil {
		return Finding{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return Finding{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(findingID))
	if err != nil {
		return Finding{}, ErrNotFound
	}
	finding, err := scanTemporalFinding(s.Pool.QueryRow(ctx, temporalFindingSelect+` WHERE f.id = $1 AND f.temporal_run_id IS NOT NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Finding{}, ErrNotFound
	}
	if err != nil {
		return Finding{}, err
	}
	if err := requireRunAccess(ctx, s.Pool, finding.TemporalRunID, actorID); err != nil {
		return Finding{}, err
	}
	if err := populateTemporalFinding(ctx, s.Pool, &finding); err != nil {
		return Finding{}, err
	}
	return finding, nil
}

func (s *Service) ReviewFinding(ctx context.Context, actorID, findingID string, input ReviewInput) (Finding, error) {
	if err := s.ready(); err != nil {
		return Finding{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return Finding{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(findingID))
	if err != nil {
		return Finding{}, ErrNotFound
	}
	decision, nextStatus, err := reviewDecision(input.Decision)
	if err != nil {
		return Finding{}, err
	}
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	input.QuestionTitleAR = strings.TrimSpace(input.QuestionTitleAR)
	if len([]rune(input.NoteAR)) > 4000 || len([]rune(input.QuestionTitleAR)) > 200 || (input.CreateQuestion && input.QuestionTitleAR == "") {
		return Finding{}, ErrValidation
	}
	actorUUID, _ := uuid.Parse(strings.TrimSpace(actorID))
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Finding{}, err
	}
	defer tx.Rollback(ctx)
	var currentStatus, title, explanation string
	var runID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT status, temporal_run_id, title_ar, explanation_ar FROM platform_findings WHERE id = $1 AND temporal_run_id IS NOT NULL AND geospatial_run_id IS NULL FOR UPDATE`, id).Scan(&currentStatus, &runID, &title, &explanation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Finding{}, ErrNotFound
		}
		return Finding{}, err
	}
	if err := requireRunAccess(ctx, tx, uuidString(runID), actorID); err != nil {
		return Finding{}, err
	}
	if decision == "investigate" && currentStatus == "investigating" {
		input.CreateQuestion = false
	}
	questionID := ""
	if input.CreateQuestion {
		questionID = uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by)
			VALUES ($1, $2, $3, 'under_investigation', 'high', $4)
		`, questionID, input.QuestionTitleAR, "سؤال فُتح من ملاحظة الإحصاء الزمني: "+explanation, actorUUID); err != nil {
			return Finding{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO question_findings (question_id, finding_id) VALUES ($1, $2)`, questionID, id); err != nil {
			return Finding{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE platform_findings
		SET status = $1, reviewed_by = $2, reviewed_at = now(), review_note_ar = NULLIF($3, ''), updated_at = now()
		WHERE id = $4
	`, nextStatus, actorUUID, input.NoteAR, id); err != nil {
		return Finding{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform_finding_reviews (finding_id, reviewer_id, decision, note_ar, question_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::uuid)
	`, id, actorUUID, decision, input.NoteAR, questionID); err != nil {
		return Finding{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "temporal_finding_reviewed", "platform_finding", id, map[string]any{"status": currentStatus, "runId": uuidString(runID)}, map[string]any{"status": nextStatus, "decision": decision, "questionId": questionID}, input.NoteAR); err != nil {
		return Finding{}, err
	}
	finding, err := scanTemporalFinding(tx.QueryRow(ctx, temporalFindingSelect+` WHERE f.id = $1 AND f.temporal_run_id IS NOT NULL`, id))
	if err != nil {
		return Finding{}, err
	}
	if err := populateTemporalFinding(ctx, tx, &finding); err != nil {
		return Finding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Finding{}, err
	}
	return finding, nil
}

func validateStartInput(input StartRunInput) (StartRunInput, error) {
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	input.TargetPersonID = strings.TrimSpace(input.TargetPersonID)
	input.QuestionID = strings.TrimSpace(input.QuestionID)
	if input.MinReferenceSize == 0 {
		input.MinReferenceSize = DefaultMinReferenceSize
	}
	if input.MinReferenceSize < 3 || input.MinReferenceSize > 100 {
		return StartRunInput{}, ErrValidation
	}
	for _, value := range []string{input.TreeID, input.TreeVersionID, input.TargetPersonID} {
		if _, err := uuid.Parse(value); err != nil {
			return StartRunInput{}, ErrValidation
		}
	}
	if input.QuestionID != "" {
		if _, err := uuid.Parse(input.QuestionID); err != nil {
			return StartRunInput{}, ErrValidation
		}
	}
	return input, nil
}

func analyzeGenerationEdges(scope treeScope, rows []edgeRow, targetPersonID string, minimum int, truncated bool) analysisResult {
	reference := ReferencePopulation{
		ScopeType:        "tree_version",
		TreeID:           scope.TreeID,
		TreeVersionID:    scope.TreeVersionID,
		VersionNumber:    scope.VersionNumber,
		VersionState:     scope.VersionState,
		ExcludedCounts:   map[string]int{},
		DatePolicy:       "bounded_stored_date_ranges_with_parent_death_chronology",
		DependencyPolicy: "independent_sources_only",
		SourcePolicy:     "public_accepted_supporting_statement_evidence_only",
		ClaimPolicy:      "all_matching_supported_claims_without_counter_evidence",
		TreePolicy:       "published_public_tree_version",
		Truncated:        truncated,
	}
	referenceItems := make([]IntervalObservation, 0)
	targets := make([]struct {
		observation IntervalObservation
		row         edgeRow
	}, 0)
	candidateCount := 0
	for _, row := range rows {
		if row.ExclusionReason != "qualified" {
			reference.ExcludedCounts[row.ExclusionReason]++
			continue
		}
		observation, valid := buildIntervalObservation(row.ParentBirthFrom, row.ParentBirthTo, row.ChildBirthFrom, row.ChildBirthTo)
		if !valid {
			reference.ExcludedCounts["invalid_dates"]++
			continue
		}
		if !validGenerationChronology(row.ParentDeathFrom, row.ParentDeathTo, row.ChildBirthFrom) {
			reference.ExcludedCounts["invalid_chronology"]++
			continue
		}
		candidateCount++
		if row.ParentPersonID == targetPersonID || row.ChildPersonID == targetPersonID {
			targets = append(targets, struct {
				observation IntervalObservation
				row         edgeRow
			}{observation: observation, row: row})
			reference.TargetInPopulation = true
			continue
		}
		referenceItems = append(referenceItems, observation)
	}
	reference.CandidateEdgeCount = candidateCount
	reference.ReferenceEdgeCount = len(referenceItems)
	result := analysisResult{Reference: reference, ReportStatus: ReportStatusInsufficient}
	if truncated {
		return result
	}
	q1, median, q3, enough := referenceBand(referenceItems, minimum)
	reference.ReferenceBandAvailable = enough
	if enough {
		reference.Q1Years = q1
		reference.MedianYears = median
		reference.Q3Years = q3
	}
	if len(targets) == 0 || !enough {
		result.Reference = reference
		return result
	}
	result.Reference = reference
	for _, target := range targets {
		if len(result.Findings) >= MaximumFindings {
			break
		}
		comparison := compareInterval(target.observation, q1, median, q3)
		comparison.ReferenceN = len(referenceItems)
		if comparison.Relation == FindingRelationOverlap {
			continue
		}
		result.Findings = append(result.Findings, findingDraft{
			RelationshipID: target.row.RelationshipID,
			ParentPersonID: target.row.ParentPersonID,
			ChildPersonID:  target.row.ChildPersonID,
			ClaimID:        target.row.ClaimID,
			Comparison:     comparison,
		})
	}
	result.ReportStatus = ReportStatusSucceeded
	return result
}

func persistFinding(ctx context.Context, tx pgx.Tx, runID, actorID uuid.UUID, draft findingDraft, reference ReferencePopulation, questionID any) error {
	findingID := uuid.New()
	relationLabel := "فوق النطاق المرجعي"
	if draft.Comparison.Relation == FindingRelationBelow {
		relationLabel = "تحت النطاق المرجعي"
	}
	explanation := fmt.Sprintf("يقع النطاق الزمني المحتمل لهذا الفاصل %s مقارنةً بالنطاق Q1–Q3 (%.1f–%.1f سنة) في %d علاقة مؤهلة من نسخة الشجرة %d، وفق سياسة %s. هذه إشارة تستدعي التحقيق، ولا تمثل احتمالاً تاريخياً أو حكماً بأن العلاقة خاطئة.", relationLabel, draft.Comparison.Q1Years, draft.Comparison.Q3Years, draft.Comparison.ReferenceN, reference.VersionNumber, QualificationPolicyVersion)
	signals := map[string]any{
		"metric":                     MetricGenerationInterval,
		"qualificationPolicyVersion": QualificationPolicyVersion,
		"referencePopulation":        reference,
		"comparison":                 draft.Comparison,
		"parentPersonId":             draft.ParentPersonID,
		"childPersonId":              draft.ChildPersonID,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform_findings
			(id, finding_type, title_ar, explanation_ar, status, severity, signals, algorithm_version, created_by, temporal_run_id, check_key)
		VALUES ($1, $2, $3, $4, 'needs_review', 'medium', $5, $6, $7, $8, $9)
	`, findingID, FindingTypeGeneration, "فاصل جيل خارج نطاق المقارنة الحالية", explanation, mustJSON(signals), AlgorithmVersion, actorID, runID, "generation_interval:"+draft.RelationshipID); err != nil {
		return err
	}
	if draft.ClaimID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO finding_claims (finding_id, claim_id, relation) VALUES ($1, $2, 'concerns')`, findingID, draft.ClaimID); err != nil {
			return err
		}
	}
	if questionID != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO question_findings (question_id, finding_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, questionID, findingID); err != nil {
			return err
		}
	}
	if draft.ParentPersonID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO finding_entities (finding_id, entity_type, entity_id) VALUES ($1, 'person', $2)`, findingID, draft.ParentPersonID); err != nil {
			return err
		}
	}
	if draft.ChildPersonID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO finding_entities (finding_id, entity_type, entity_id) VALUES ($1, 'person', $2)`, findingID, draft.ChildPersonID); err != nil {
			return err
		}
	}
	return nil
}

func requireRunAccess(ctx context.Context, q queryer, runID, actorID string) error {
	var treeID string
	if err := q.QueryRow(ctx, `SELECT tree_id::text FROM temporal_analysis_runs WHERE id = $1`, runID).Scan(&treeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return requireTreeAccess(ctx, q, treeID, actorID)
}

func requireTreeAccess(ctx context.Context, q queryer, treeID, actorID string) error {
	actor, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return ErrForbidden
	}
	var allowed bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM trees t
			WHERE t.id = $1
			  AND (t.visibility = 'public' OR t.owner_id = $2 OR EXISTS (
				SELECT 1 FROM tree_collaborators tc
				WHERE tc.tree_id = t.id AND tc.user_id = $2
			  ))
		)
	`, treeID, actor).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func requireRole(ctx context.Context, q queryer, actorID string) error {
	actor, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return ErrForbidden
	}
	var allowed bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role = ANY($2::text[]))`, actor, []string{"researcher", "moderator", "admin"}).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func reviewDecision(value string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dismiss":
		return "dismiss", "dismissed", nil
	case "confirm":
		return "confirm", "confirmed", nil
	case "investigate":
		return "investigate", "investigating", nil
	case "reopen":
		return "reopen", "needs_review", nil
	default:
		return "", "", ErrValidation
	}
}

func validFindingStatus(value string) bool {
	return value == "needs_review" || value == "confirmed" || value == "dismissed" || value == "investigating"
}

const temporalRunSelect = `
	SELECT id::text, requested_by::text, COALESCE(question_id::text, ''), tree_id::text, tree_version_id::text,
	       target_person_id::text, status, report_status, execution_mode, algorithm_version,
	       qualification_policy_version, min_reference_size, reference_population, finding_count,
	       error, created_at, started_at, completed_at, updated_at
	FROM temporal_analysis_runs`

func scanRun(row pgx.Row) (Run, error) {
	var result Run
	var reportStatus, executionMode, runError pgtype.Text
	var referenceData []byte
	var startedAt, completedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.RequestedBy, &result.QuestionID, &result.TreeID, &result.TreeVersionID, &result.TargetPersonID, &result.Status, &reportStatus, &executionMode, &result.AlgorithmVersion, &result.QualificationPolicyVersion, &result.MinReferenceSize, &referenceData, &result.FindingCount, &runError, &result.CreatedAt, &startedAt, &completedAt, &result.UpdatedAt); err != nil {
		return Run{}, err
	}
	result.ReportStatus = textValue(reportStatus)
	result.ExecutionMode = textValue(executionMode)
	result.Error = textValue(runError)
	result.ReferencePopulation = ReferencePopulation{}
	if len(referenceData) > 0 {
		_ = json.Unmarshal(referenceData, &result.ReferencePopulation)
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

const temporalFindingSelect = `
	SELECT f.id::text, f.temporal_run_id::text, f.finding_type, f.title_ar, f.explanation_ar, f.status, f.severity,
	       COALESCE(f.signals, '{}'::jsonb), COALESCE(f.algorithm_version, ''), COALESCE(f.created_by::text, ''),
	       COALESCE(f.reviewed_by::text, ''), f.reviewed_at, COALESCE(f.review_note_ar, ''),
	       COALESCE((SELECT question_id::text FROM platform_finding_reviews WHERE finding_id = f.id AND question_id IS NOT NULL ORDER BY created_at DESC LIMIT 1), (SELECT question_id::text FROM question_findings WHERE finding_id = f.id ORDER BY question_id LIMIT 1), ''),
	       f.created_at, f.updated_at
	FROM platform_findings f
	JOIN temporal_analysis_runs temporal_run ON temporal_run.id = f.temporal_run_id`

func scanTemporalFinding(row pgx.Row) (Finding, error) {
	var result Finding
	var signals []byte
	var reviewedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.TemporalRunID, &result.FindingType, &result.TitleAR, &result.ExplanationAR, &result.Status, &result.Severity, &signals, &result.AlgorithmVersion, &result.CreatedBy, &result.ReviewedBy, &reviewedAt, &result.ReviewNoteAR, &result.QuestionID, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return Finding{}, err
	}
	result.RunID = result.TemporalRunID
	result.Signals = map[string]any{}
	if len(signals) > 0 {
		_ = json.Unmarshal(signals, &result.Signals)
	}
	result.ClaimIDs = []string{}
	result.EntityIDs = []string{}
	result.Reviews = []Review{}
	if reviewedAt.Valid {
		value := reviewedAt.Time
		result.ReviewedAt = &value
	}
	return result, nil
}

func populateTemporalFinding(ctx context.Context, q queryer, finding *Finding) error {
	linkRows, err := q.Query(ctx, `
		SELECT 'claim', claim_id::text FROM finding_claims WHERE finding_id = $1
		UNION ALL SELECT 'entity', entity_id::text FROM finding_entities WHERE finding_id = $1 ORDER BY 1, 2
	`, finding.ID)
	if err != nil {
		return err
	}
	for linkRows.Next() {
		var kind, value string
		if err := linkRows.Scan(&kind, &value); err != nil {
			linkRows.Close()
			return err
		}
		if kind == "claim" {
			finding.ClaimIDs = append(finding.ClaimIDs, value)
		} else {
			finding.EntityIDs = append(finding.EntityIDs, value)
		}
	}
	if err := linkRows.Err(); err != nil {
		linkRows.Close()
		return err
	}
	linkRows.Close()
	reviewRows, err := q.Query(ctx, `
		SELECT id::text, reviewer_id::text, decision, COALESCE(note_ar, ''), COALESCE(question_id::text, ''), created_at
		FROM platform_finding_reviews WHERE finding_id = $1 ORDER BY created_at
	`, finding.ID)
	if err != nil {
		return err
	}
	defer reviewRows.Close()
	for reviewRows.Next() {
		var review Review
		if err := reviewRows.Scan(&review.ID, &review.ReviewerID, &review.Decision, &review.NoteAR, &review.QuestionID, &review.CreatedAt); err != nil {
			return err
		}
		finding.Reviews = append(finding.Reviews, review)
	}
	if err := reviewRows.Err(); err != nil {
		return err
	}
	if value, ok := finding.Signals["referencePopulation"]; ok {
		encoded, _ := json.Marshal(value)
		_ = json.Unmarshal(encoded, &finding.ReferencePopulation)
	}
	if value, ok := finding.Signals["comparison"]; ok {
		encoded, _ := json.Marshal(value)
		_ = json.Unmarshal(encoded, &finding.Comparison)
	}
	finding.QualificationPolicyVersion = QualificationPolicyVersion
	return nil
}

func writeAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
	`, actorID, action, entityType, entityID, mustJSON(before), mustJSON(after), reason)
	return err
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func optionalUUID(value string) (any, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
