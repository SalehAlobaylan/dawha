package contradiction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Pool *pgxpool.Pool
	Jobs *jobs.Service
}

func NewService(pool *pgxpool.Pool, queue *jobs.Service) *Service {
	return &Service{Pool: pool, Jobs: queue}
}

var operatorRoles = []string{"researcher", "moderator", "admin"}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) StartRun(ctx context.Context, actorID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if s.Jobs == nil {
		return Run{}, ErrQueueUnavailable
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), operatorRoles...); err != nil {
		return Run{}, err
	}
	runID := uuid.New()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	var result Run
	if err := tx.QueryRow(ctx, `
		INSERT INTO contradiction_runs (id, requested_by, status, algorithm_version)
		VALUES ($1, $2, 'queued', $3)
		RETURNING id::text, requested_by::text, status, algorithm_version, created_at, updated_at
	`, runID, actorID, AlgorithmVersion).Scan(&result.ID, &result.RequestedBy, &result.Status, &result.AlgorithmVersion, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return Run{}, err
	}
	jobResult, err := s.Jobs.EnqueueTx(ctx, tx, jobs.EnqueueInput{
		Type:           JobType,
		Payload:        encodeJobPayload(result.ID),
		Priority:       5,
		IdempotencyKey: "contradiction-scan:" + result.ID,
		MaxAttempts:    3,
	})
	if err != nil {
		return Run{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE contradiction_runs SET job_id = $1, updated_at = now() WHERE id = $2`, jobResult.Job.ID, runID); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	result.JobID = jobResult.Job.ID
	return result, nil
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), operatorRoles...); err != nil {
		return Run{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return Run{}, ErrNotFound
	}
	var result Run
	var requestedBy, jobID, status, algorithmVersion string
	var runError pgtype.Text
	var startedAt, completedAt pgtype.Timestamptz
	if err := s.Pool.QueryRow(ctx, `
		SELECT id::text, requested_by::text, COALESCE(job_id::text, ''), status, algorithm_version, finding_count, error, created_at, started_at, completed_at, updated_at
		FROM contradiction_runs WHERE id = $1
	`, id).Scan(&result.ID, &requestedBy, &jobID, &status, &algorithmVersion, &result.FindingCount, &runError, &result.CreatedAt, &startedAt, &completedAt, &result.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	result.RequestedBy = requestedBy
	result.JobID = jobID
	result.Status = status
	result.AlgorithmVersion = algorithmVersion
	result.Error = textValue(runError)
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

func (s *Service) ListFindings(ctx context.Context, actorID, runID, status string) ([]Finding, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), operatorRoles...); err != nil {
		return nil, err
	}
	query := findingSelect + ` WHERE f.temporal_run_id IS NULL`
	args := make([]any, 0, 2)
	if strings.TrimSpace(runID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(runID))
		if err != nil {
			return nil, ErrNotFound
		}
		args = append(args, id)
		query += fmt.Sprintf(" AND f.run_id = $%d", len(args))
	}
	if strings.TrimSpace(status) != "" {
		status = strings.ToLower(strings.TrimSpace(status))
		if !validFindingStatus(status) {
			return nil, ErrValidation
		}
		args = append(args, status)
		query += fmt.Sprintf(" AND f.status = $%d", len(args))
	}
	query += ` ORDER BY f.created_at DESC LIMIT 100`
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Finding, 0)
	for rows.Next() {
		finding, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		if err := populateFinding(ctx, s.Pool, &finding); err != nil {
			return nil, err
		}
		result = append(result, finding)
	}
	return result, rows.Err()
}

func (s *Service) GetFinding(ctx context.Context, actorID, findingID string) (Finding, error) {
	if err := s.ready(); err != nil {
		return Finding{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), operatorRoles...); err != nil {
		return Finding{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(findingID))
	if err != nil {
		return Finding{}, ErrNotFound
	}
	return s.getFinding(ctx, s.Pool, id)
}

func (s *Service) getFinding(ctx context.Context, q queryer, id uuid.UUID) (Finding, error) {
	finding, err := scanFinding(q.QueryRow(ctx, findingSelect+` WHERE f.id = $1 AND f.temporal_run_id IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Finding{}, ErrNotFound
	}
	if err != nil {
		return Finding{}, err
	}
	if err := populateFinding(ctx, q, &finding); err != nil {
		return Finding{}, err
	}
	return finding, nil
}

func populateFinding(ctx context.Context, q queryer, finding *Finding) error {
	linkRows, err := q.Query(ctx, `
		SELECT 'claim', claim_id::text FROM finding_claims WHERE finding_id = $1
		UNION ALL
		SELECT 'entity', entity_id::text FROM finding_entities WHERE finding_id = $1
		ORDER BY 1, 2
	`, finding.ID)
	if err != nil {
		return err
	}
	defer linkRows.Close()
	for linkRows.Next() {
		var kind, value string
		if err := linkRows.Scan(&kind, &value); err != nil {
			return err
		}
		if kind == "claim" {
			finding.ClaimIDs = append(finding.ClaimIDs, value)
		} else {
			finding.EntityIDs = append(finding.EntityIDs, value)
		}
	}
	if err := linkRows.Err(); err != nil {
		return err
	}
	reviewRows, err := q.Query(ctx, `
		SELECT id::text, reviewer_id::text, decision, COALESCE(note_ar, ''), COALESCE(question_id::text, ''), created_at
		FROM platform_finding_reviews WHERE finding_id = $1 ORDER BY created_at
	`, finding.ID)
	if err != nil {
		return err
	}
	defer reviewRows.Close()
	finding.Reviews = []Review{}
	for reviewRows.Next() {
		var review Review
		if err := reviewRows.Scan(&review.ID, &review.ReviewerID, &review.Decision, &review.NoteAR, &review.QuestionID, &review.CreatedAt); err != nil {
			return err
		}
		finding.Reviews = append(finding.Reviews, review)
	}
	return reviewRows.Err()
}

func (s *Service) ReviewFinding(ctx context.Context, actorID, findingID string, input ReviewInput) (Finding, error) {
	if err := s.ready(); err != nil {
		return Finding{}, err
	}
	if err := requireRole(ctx, s.Pool, strings.TrimSpace(actorID), operatorRoles...); err != nil {
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
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Finding{}, err
	}
	defer tx.Rollback(ctx)
	var currentStatus, findingType, title, explanation string
	if err := tx.QueryRow(ctx, `SELECT status, finding_type, title_ar, explanation_ar FROM platform_findings WHERE id = $1 AND temporal_run_id IS NULL FOR UPDATE`, id).Scan(&currentStatus, &findingType, &title, &explanation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Finding{}, ErrNotFound
		}
		return Finding{}, err
	}
	var questionID string
	if input.CreateQuestion {
		questionID = uuid.New().String()
		if _, err := tx.Exec(ctx, `
			INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by)
			VALUES ($1, $2, $3, 'under_investigation', 'high', $4)
		`, questionID, input.QuestionTitleAR, "سؤال فُتح من ملاحظة النظام: "+explanation, actorID); err != nil {
			return Finding{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO question_findings (question_id, finding_id) VALUES ($1, $2)`, questionID, id); err != nil {
			return Finding{}, err
		}
	}
	reviewedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE platform_findings
		SET status = $1, reviewed_by = $2, reviewed_at = $3, review_note_ar = NULLIF($4, ''), updated_at = now()
		WHERE id = $5
	`, nextStatus, actorID, reviewedAt, input.NoteAR, id); err != nil {
		return Finding{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform_finding_reviews (finding_id, reviewer_id, decision, note_ar, question_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::uuid)
	`, id, actorID, decision, input.NoteAR, questionID); err != nil {
		return Finding{}, err
	}
	if err := writeAudit(ctx, tx, actorID, "platform_finding_reviewed", "platform_finding", id, map[string]any{"status": currentStatus}, map[string]any{"status": nextStatus, "decision": decision, "questionId": questionID}, input.NoteAR); err != nil {
		return Finding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Finding{}, err
	}
	return s.getFinding(ctx, s.Pool, id)
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

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

const findingSelect = `
	SELECT f.id::text, COALESCE(f.run_id::text, ''), f.finding_type, f.title_ar, f.explanation_ar, f.status, f.severity,
	       COALESCE(f.signals, '{}'::jsonb), COALESCE(f.algorithm_version, ''), COALESCE(f.model_version, ''),
	       COALESCE(f.created_by::text, ''), COALESCE(f.reviewed_by::text, ''), f.reviewed_at, COALESCE(f.review_note_ar, ''),
	       COALESCE((SELECT question_id::text FROM platform_finding_reviews WHERE finding_id = f.id AND question_id IS NOT NULL ORDER BY created_at DESC LIMIT 1), ''),
	       f.created_at, f.updated_at
	FROM platform_findings f`

func scanFinding(row pgx.Row) (Finding, error) {
	var result Finding
	var signals []byte
	var reviewedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.RunID, &result.FindingType, &result.TitleAR, &result.ExplanationAR, &result.Status, &result.Severity, &signals, &result.AlgorithmVersion, &result.ModelVersion, &result.CreatedBy, &result.ReviewedBy, &reviewedAt, &result.ReviewNoteAR, &result.QuestionID, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return Finding{}, err
	}
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

func writeAudit(ctx context.Context, tx pgx.Tx, actorID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
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
