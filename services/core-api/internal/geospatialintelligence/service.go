package geospatialintelligence

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
	input, err := validateRunInput(input)
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
	scope, err := loadTreeScope(ctx, tx, input)
	if err != nil {
		return Run{}, err
	}
	if input.EntityType == "person" && (input.TreeID == "" || input.TreeVersionID == "") {
		return Run{}, ErrValidation
	}
	if input.TreeID != "" {
		if scope.TreeVisibility != "public" {
			return Run{}, ErrForbidden
		}
		if scope.VersionState != "published" {
			return Run{}, ErrValidation
		}
		if input.EntityType == "person" && !scope.TargetPresent {
			return Run{}, ErrValidation
		}
	}
	entityName, err := loadEntityName(ctx, tx, input.EntityType, input.EntityID)
	if err != nil {
		return Run{}, err
	}
	if input.EntityType == "source" {
		source, sourceErr := loadSource(ctx, tx, input.EntityID)
		if sourceErr != nil {
			return Run{}, sourceErr
		}
		if source.Visibility != "public" && source.CreatedBy != actorUUID.String() {
			return Run{}, ErrForbidden
		}
		entityName = source.Title
	}
	questionID, err := optionalUUID(input.QuestionID)
	if err != nil {
		return Run{}, ErrValidation
	}
	if questionID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM open_questions WHERE id = $1)`, questionID).Scan(&exists); err != nil {
			return Run{}, err
		}
		if !exists {
			return Run{}, ErrNotFound
		}
	}
	sourceIDs, err := loadSourceIDs(ctx, tx, input)
	if err != nil {
		return Run{}, err
	}
	sources, err := loadSourceRecords(ctx, tx, sourceIDs)
	if err != nil {
		return Run{}, err
	}
	statements, err := loadStatements(ctx, tx, sourceIDs, input.MaximumRecords)
	if err != nil {
		return Run{}, err
	}
	associations, err := loadAssociations(ctx, tx, input, input.MaximumRecords)
	if err != nil {
		return Run{}, err
	}
	migrations, err := loadMigrations(ctx, tx, input, input.MaximumRecords)
	if err != nil {
		return Run{}, err
	}
	places, err := loadPlaces(ctx, tx, input.MaximumRecords)
	if err != nil {
		return Run{}, err
	}
	names, err := loadHistoricalNames(ctx, tx, input.MaximumRecords)
	if err != nil {
		return Run{}, err
	}
	edges, err := loadSpatialEdges(ctx, tx, input.RadiusKM*1000, input.MaximumRecords+1)
	if err != nil {
		return Run{}, err
	}
	truncated := len(edges) > input.MaximumRecords
	if truncated {
		edges = edges[:input.MaximumRecords]
	}
	reportScope := Scope{EntityType: input.EntityType, EntityID: input.EntityID, EntityName: entityName, TreeID: scope.TreeID, TreeVersionID: scope.TreeVersionID, VersionNumber: scope.VersionNumber, VersionState: scope.VersionState, TreeVisibility: scope.TreeVisibility, RadiusKM: input.RadiusKM, MaximumRecords: input.MaximumRecords, SourceIDs: sourceIDs, QualificationNotes: []string{"public_published_scope", "independent_accepted_sources", "geometry_required_for_spatial_inference", "ambiguous_names_remain_unresolved", "inference_labeled_platform_hypothesis"}}
	analysisInput := analysisInput{Scope: reportScope, Places: places, Names: names, Statements: statements, Sources: sources, Associations: associations, Migrations: migrations, SpatialEdges: edges, RadiusKM: input.RadiusKM, MaximumRecords: input.MaximumRecords}
	report, findingDrafts := analyze(analysisInput)
	if truncated {
		report.Limitations = append(report.Limitations, "تم تجاوز حد مسارات التشابه المكاني؛ تم تحليل مجموعة جزئية.")
	}
	reportStatus := ReportStatusSucceeded
	if len(statements) == 0 && len(associations) == 0 && len(migrations) == 0 {
		reportStatus = ReportStatusInsufficient
	}
	runID := uuid.New()
	startedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO geospatial_intelligence_runs
			(id, requested_by, question_id, entity_type, entity_id, tree_id, tree_version_id, status, report_status,
			 execution_mode, algorithm_version, qualification_policy_version, scope, report, finding_count, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'succeeded', $8, 'synchronous', $9, $10, $11, $12, $13, $14, $14)
	`, runID, actorUUID, questionID, input.EntityType, input.EntityID, nullableString(scope.TreeID), nullableString(scope.TreeVersionID), reportStatus, AlgorithmVersion, QualificationPolicyVersion, mustJSON(reportScope), mustJSON(report), len(findingDrafts), startedAt); err != nil {
		return Run{}, err
	}
	for index, draft := range findingDrafts {
		findingID, persistErr := persistFinding(ctx, tx, runID, actorUUID, questionID, draft)
		if persistErr != nil {
			return Run{}, persistErr
		}
		if index < len(report.GeographicContradictions) {
			report.GeographicContradictions[index].FindingID = findingID
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE geospatial_intelligence_runs SET report = $1 WHERE id = $2`, mustJSON(report), runID); err != nil {
		return Run{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "geospatial_intelligence_run_completed", "geospatial_intelligence_run", runID, nil, map[string]any{"reportStatus": reportStatus, "findingCount": len(findingDrafts), "algorithm": AlgorithmVersion}, ""); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	completedAt := startedAt
	return Run{ID: runID.String(), RequestedBy: actorID, QuestionID: input.QuestionID, EntityType: input.EntityType, EntityID: input.EntityID, EntityName: entityName, TreeID: scope.TreeID, TreeVersionID: scope.TreeVersionID, VersionNumber: scope.VersionNumber, VersionState: scope.VersionState, TreeVisibility: scope.TreeVisibility, Status: "succeeded", ReportStatus: reportStatus, ExecutionMode: "synchronous", AlgorithmVersion: AlgorithmVersion, QualificationPolicyVersion: QualificationPolicyVersion, Scope: reportScope, Report: report, FindingCount: len(findingDrafts), CreatedAt: startedAt, StartedAt: &startedAt, CompletedAt: &completedAt, UpdatedAt: startedAt}, nil
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
	return result, nil
}

func (s *Service) GetLatestRun(ctx context.Context, actorID, questionID, entityType, entityID, treeVersionID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return Run{}, err
	}
	if entityType != "" && entityType != "person" && entityType != "source" && entityType != "place" {
		return Run{}, ErrValidation
	}
	filters := []struct {
		column string
		value  string
	}{{"question_id", questionID}, {"entity_type", entityType}, {"entity_id", entityID}, {"tree_version_id", treeVersionID}}
	query := runSelect + ` WHERE 1 = 1`
	args := make([]any, 0, len(filters))
	for _, filter := range filters {
		if strings.TrimSpace(filter.value) == "" {
			continue
		}
		if filter.column == "entity_type" {
			args = append(args, strings.ToLower(strings.TrimSpace(filter.value)))
		} else {
			parsed, err := uuid.Parse(strings.TrimSpace(filter.value))
			if err != nil {
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

func (s *Service) ListFindings(ctx context.Context, actorID, runID, status string) ([]Finding, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID)); err != nil {
		return nil, err
	}
	query := findingSelect + ` WHERE f.geospatial_run_id IS NOT NULL`
	args := make([]any, 0, 2)
	if strings.TrimSpace(runID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(runID))
		if err != nil {
			return nil, ErrNotFound
		}
		args = append(args, id)
		query += fmt.Sprintf(" AND f.geospatial_run_id = $%d", len(args))
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
		finding, scanErr := scanFinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		_, runErr := s.GetRun(ctx, actorID, finding.GeospatialRunID)
		if errors.Is(runErr, ErrForbidden) || errors.Is(runErr, ErrNotFound) {
			continue
		}
		if runErr != nil {
			return nil, runErr
		}
		if err := populateFinding(ctx, s.Pool, &finding); err != nil {
			return nil, err
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
	finding, err := scanFinding(s.Pool.QueryRow(ctx, findingSelect+` WHERE f.id = $1 AND f.geospatial_run_id IS NOT NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Finding{}, ErrNotFound
	}
	if err != nil {
		return Finding{}, err
	}
	if _, err := s.GetRun(ctx, actorID, finding.GeospatialRunID); err != nil {
		return Finding{}, err
	}
	if err := populateFinding(ctx, s.Pool, &finding); err != nil {
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
	if err := tx.QueryRow(ctx, `SELECT status, title_ar, explanation_ar, geospatial_run_id FROM platform_findings WHERE id = $1 AND geospatial_run_id IS NOT NULL FOR UPDATE`, id).Scan(&currentStatus, &title, &explanation, &runID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Finding{}, ErrNotFound
		}
		return Finding{}, err
	}
	if err := s.requireRunAccess(ctx, tx, Run{ID: uuidString(runID)}, actorID); err != nil {
		return Finding{}, err
	}
	if decision == "investigate" && currentStatus == "investigating" {
		input.CreateQuestion = false
	}
	questionID := ""
	if input.CreateQuestion {
		questionID = uuid.NewString()
		if _, err := tx.Exec(ctx, `INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by) VALUES ($1, $2, $3, 'under_investigation', 'high', $4)`, questionID, input.QuestionTitleAR, "سؤال فُتح من ملاحظة جغرافية: "+explanation, actorUUID); err != nil {
			return Finding{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO question_findings (question_id, finding_id) VALUES ($1, $2)`, questionID, id); err != nil {
			return Finding{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE platform_findings SET status = $1, reviewed_by = $2, reviewed_at = now(), review_note_ar = NULLIF($3, ''), updated_at = now() WHERE id = $4`, nextStatus, actorUUID, input.NoteAR, id); err != nil {
		return Finding{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO platform_finding_reviews (finding_id, reviewer_id, decision, note_ar, question_id) VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::uuid)`, id, actorUUID, decision, input.NoteAR, questionID); err != nil {
		return Finding{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "geospatial_finding_reviewed", "platform_finding", id, map[string]any{"status": currentStatus, "runId": uuidString(runID)}, map[string]any{"status": nextStatus, "decision": decision, "questionId": questionID}, input.NoteAR); err != nil {
		return Finding{}, err
	}
	finding, err := scanFinding(tx.QueryRow(ctx, findingSelect+` WHERE f.id = $1 AND f.geospatial_run_id IS NOT NULL`, id))
	if err != nil {
		return Finding{}, err
	}
	if err := populateFinding(ctx, tx, &finding); err != nil {
		return Finding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Finding{}, err
	}
	return finding, nil
}

func (s *Service) requireRunAccess(ctx context.Context, q queryer, run Run, actorID string) error {
	if run.EntityType == "" && run.ID != "" {
		if err := q.QueryRow(ctx, `SELECT entity_type, entity_id::text, COALESCE(tree_id::text, '') FROM geospatial_intelligence_runs WHERE id = $1`, run.ID).Scan(&run.EntityType, &run.EntityID, &run.TreeID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
	}
	if run.EntityType == "" {
		return ErrForbidden
	}
	if run.TreeID != "" {
		if err := requireTreeAccess(ctx, q, run.TreeID, actorID); err != nil {
			return err
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

func requireTreeAccess(ctx context.Context, q queryer, treeID, actorID string) error {
	actor, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return ErrForbidden
	}
	var allowed bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trees t WHERE t.id = $1 AND (t.visibility = 'public' OR t.owner_id = $2 OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2)))`, treeID, actor).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func validateRunInput(input RunInput) (RunInput, error) {
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.EntityID = strings.TrimSpace(input.EntityID)
	input.QuestionID = strings.TrimSpace(input.QuestionID)
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	if input.EntityType != "person" && input.EntityType != "source" && input.EntityType != "place" {
		return RunInput{}, ErrValidation
	}
	for _, value := range []string{input.EntityID, input.QuestionID, input.TreeID, input.TreeVersionID} {
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
	if input.RadiusKM == 0 {
		input.RadiusKM = DefaultRadiusKM
	}
	if input.RadiusKM < 10 || input.RadiusKM > MaximumRadiusKM {
		return RunInput{}, ErrValidation
	}
	if input.MaximumRecords == 0 {
		input.MaximumRecords = 500
	}
	if input.MaximumRecords < 50 || input.MaximumRecords > MaximumRecords {
		return RunInput{}, ErrValidation
	}
	return input, nil
}

func persistFinding(ctx context.Context, tx pgx.Tx, runID, actorID uuid.UUID, questionID any, draft findingDraft) (string, error) {
	findingID := uuid.New()
	checkKey := stableID("geospatial-intelligence", draft.Type, strings.Join(draft.EntityIDs, ","), strings.Join(draft.PlaceIDs, ","))
	signals := make(map[string]any, len(draft.Signals)+3)
	for key, value := range draft.Signals {
		signals[key] = value
	}
	signals["layer"] = "platform_inferred"
	signals["placeIds"] = draft.PlaceIDs
	signals["sourceIds"] = draft.SourceIDs
	if _, err := tx.Exec(ctx, `INSERT INTO platform_findings (id, finding_type, title_ar, explanation_ar, status, severity, signals, algorithm_version, created_by, geospatial_run_id, check_key) VALUES ($1, $2, $3, $4, 'needs_review', $5, $6, $7, $8, $9, $10)`, findingID, draft.Type, draft.TitleAR, draft.ExplanationAR, draft.Severity, mustJSON(signals), AlgorithmVersion, actorID, runID, checkKey); err != nil {
		return "", err
	}
	for _, entityID := range draft.EntityIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO finding_entities (finding_id, entity_type, entity_id) VALUES ($1, 'person', $2) ON CONFLICT DO NOTHING`, findingID, entityID); err != nil {
			return "", err
		}
	}
	for _, claimID := range draft.ClaimIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO finding_claims (finding_id, claim_id, relation) VALUES ($1, $2, 'concerns') ON CONFLICT DO NOTHING`, findingID, claimID); err != nil {
			return "", err
		}
	}
	if questionID != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO question_findings (question_id, finding_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, questionID, findingID); err != nil {
			return "", err
		}
	}
	return findingID.String(), nil
}

func writeAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))`, actorID, action, entityType, entityID, mustJSON(before), mustJSON(after), reason)
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
	return uuid.Parse(strings.TrimSpace(value))
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
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

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func normalizeReport(report Report) Report {
	if report.PlaceResolution.Mentions == nil {
		report.PlaceResolution.Mentions = []PlaceMention{}
	}
	if report.Disambiguation == nil {
		report.Disambiguation = []DisambiguationCandidate{}
	}
	if report.Clusters == nil {
		report.Clusters = []SpatialCluster{}
	}
	if report.MigrationHypotheses == nil {
		report.MigrationHypotheses = []MigrationHypothesis{}
	}
	if report.GeographicContradictions == nil {
		report.GeographicContradictions = []GeographicContradiction{}
	}
	if report.SourceGeography == nil {
		report.SourceGeography = []SourceGeography{}
	}
	if report.Limitations == nil {
		report.Limitations = []string{}
	}
	return report
}

const runSelect = `
	SELECT id::text, requested_by::text, COALESCE(question_id::text, ''), entity_type, entity_id::text,
	       COALESCE(tree_id::text, ''), COALESCE(tree_version_id::text, ''), status, COALESCE(report_status, ''), execution_mode,
	       algorithm_version, qualification_policy_version, scope, report, finding_count, COALESCE(error, ''), created_at, started_at, completed_at, updated_at
	FROM geospatial_intelligence_runs`

func scanRun(row pgx.Row) (Run, error) {
	var result Run
	var questionID, treeID, treeVersionID, runError pgtype.Text
	var scopeData, reportData []byte
	var startedAt, completedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.RequestedBy, &questionID, &result.EntityType, &result.EntityID, &treeID, &treeVersionID, &result.Status, &result.ReportStatus, &result.ExecutionMode, &result.AlgorithmVersion, &result.QualificationPolicyVersion, &scopeData, &reportData, &result.FindingCount, &runError, &result.CreatedAt, &startedAt, &completedAt, &result.UpdatedAt); err != nil {
		return Run{}, err
	}
	result.QuestionID = textValue(questionID)
	result.TreeID = textValue(treeID)
	result.TreeVersionID = textValue(treeVersionID)
	result.Error = textValue(runError)
	if len(scopeData) > 0 {
		_ = json.Unmarshal(scopeData, &result.Scope)
		result.EntityName = result.Scope.EntityName
		result.VersionNumber = result.Scope.VersionNumber
		result.VersionState = result.Scope.VersionState
		result.TreeVisibility = result.Scope.TreeVisibility
	}
	if len(reportData) > 0 {
		_ = json.Unmarshal(reportData, &result.Report)
	}
	result.Report = normalizeReport(result.Report)
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

const findingSelect = `
	SELECT f.id::text, f.geospatial_run_id::text, f.finding_type, f.title_ar, f.explanation_ar, f.status, f.severity,
	       COALESCE(f.signals, '{}'::jsonb), COALESCE(f.algorithm_version, ''), COALESCE(f.created_by::text, ''),
	       COALESCE(f.reviewed_by::text, ''), f.reviewed_at, COALESCE(f.review_note_ar, ''),
	       COALESCE((SELECT question_id::text FROM platform_finding_reviews WHERE finding_id = f.id AND question_id IS NOT NULL ORDER BY created_at DESC LIMIT 1),
	                (SELECT question_id::text FROM question_findings WHERE finding_id = f.id ORDER BY question_id LIMIT 1), ''),
	       f.created_at, f.updated_at
	FROM platform_findings f`

func scanFinding(row pgx.Row) (Finding, error) {
	var result Finding
	var signals []byte
	var reviewedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.GeospatialRunID, &result.FindingType, &result.TitleAR, &result.ExplanationAR, &result.Status, &result.Severity, &signals, &result.AlgorithmVersion, &result.CreatedBy, &result.ReviewedBy, &reviewedAt, &result.ReviewNoteAR, &result.QuestionID, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return Finding{}, err
	}
	result.RunID = result.GeospatialRunID
	result.Signals = map[string]any{}
	if len(signals) > 0 {
		_ = json.Unmarshal(signals, &result.Signals)
	}
	result.EntityIDs = []string{}
	result.PlaceIDs = []string{}
	result.SourceIDs = []string{}
	result.ClaimIDs = []string{}
	result.Reviews = []Review{}
	result.Layer = "platform_inferred"
	if reviewedAt.Valid {
		value := reviewedAt.Time
		result.ReviewedAt = &value
	}
	return result, nil
}

func populateFinding(ctx context.Context, q queryer, finding *Finding) error {
	entityRows, err := q.Query(ctx, `SELECT entity_id::text FROM finding_entities WHERE finding_id = $1 ORDER BY entity_id`, finding.ID)
	if err != nil {
		return err
	}
	for entityRows.Next() {
		var id string
		if err := entityRows.Scan(&id); err != nil {
			entityRows.Close()
			return err
		}
		finding.EntityIDs = append(finding.EntityIDs, id)
	}
	if err := entityRows.Err(); err != nil {
		entityRows.Close()
		return err
	}
	entityRows.Close()
	claimRows, err := q.Query(ctx, `SELECT claim_id::text FROM finding_claims WHERE finding_id = $1 ORDER BY claim_id`, finding.ID)
	if err != nil {
		return err
	}
	for claimRows.Next() {
		var id string
		if err := claimRows.Scan(&id); err != nil {
			claimRows.Close()
			return err
		}
		finding.ClaimIDs = append(finding.ClaimIDs, id)
	}
	if err := claimRows.Err(); err != nil {
		claimRows.Close()
		return err
	}
	claimRows.Close()
	reviewRows, err := q.Query(ctx, `SELECT id::text, reviewer_id::text, decision, COALESCE(note_ar, ''), COALESCE(question_id::text, ''), created_at FROM platform_finding_reviews WHERE finding_id = $1 ORDER BY created_at`, finding.ID)
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
	if value, ok := finding.Signals["placeIds"].([]any); ok {
		for _, item := range value {
			if text, ok := item.(string); ok {
				finding.PlaceIDs = append(finding.PlaceIDs, text)
			}
		}
	}
	if value, ok := finding.Signals["sourceIds"].([]any); ok {
		for _, item := range value {
			if text, ok := item.(string); ok {
				finding.SourceIDs = append(finding.SourceIDs, text)
			}
		}
	}
	if value, ok := finding.Signals["layer"].(string); ok && value != "" {
		finding.Layer = value
	}
	finding.QualificationPolicyVersion = QualificationPolicyVersion
	return nil
}
