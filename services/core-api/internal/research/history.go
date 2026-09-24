package research

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ResearchRunSummary struct {
	ID                   string    `json:"id"`
	QuestionID           string    `json:"questionId,omitempty"`
	Query                string    `json:"query"`
	Status               string    `json:"status"`
	InsufficientEvidence bool      `json:"insufficientEvidence"`
	CitationCount        int       `json:"citationCount"`
	AnswerAR             string    `json:"answerAr,omitempty"`
	ModelVersion         string    `json:"modelVersion,omitempty"`
	Route                string    `json:"route,omitempty"`
	SynthesisAttempted   bool      `json:"synthesisAttempted"`
	Error                string    `json:"error,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type ResearchRunContext struct {
	ScopeType string `json:"scopeType"`
	ScopeID   string `json:"scopeId"`
	Role      string `json:"role"`
}

type ResearchRunDetail struct {
	ResearchRunSummary
	Contexts []ResearchRunContext `json:"contexts"`
}

type historyExecutor interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) ListRuns(ctx context.Context, actorID, questionID string) ([]ResearchRunSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	questionUUID, err := uuid.Parse(strings.TrimSpace(questionID))
	if err != nil {
		return nil, ErrValidation
	}
	includeAnswer, err := s.canViewResearchHistory(ctx, actorID)
	if err != nil {
		return nil, err
	}
	return listRuns(ctx, s.Pool, questionUUID, includeAnswer)
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (ResearchRunDetail, error) {
	if err := s.ready(); err != nil {
		return ResearchRunDetail{}, err
	}
	runUUID, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return ResearchRunDetail{}, ErrNotFound
	}
	includeAnswer, err := s.canViewResearchHistory(ctx, actorID)
	if err != nil {
		return ResearchRunDetail{}, err
	}
	summary, err := scanRunSummary(s.Pool.QueryRow(ctx, `
		SELECT rr.id, rr.question_id, rr.query, rr.status, rr.insufficient_evidence,
		       count(re.id), CASE WHEN $2 THEN COALESCE(ra.answer, '') ELSE '' END,
		       COALESCE(rr.model_version, ''), COALESCE(rr.semantic_route, ''), rr.synthesis_attempted,
		       CASE WHEN $2 THEN COALESCE(rr.error, '') ELSE '' END, rr.created_at, rr.updated_at
		FROM research_runs rr
		LEFT JOIN research_answers ra ON ra.run_id = rr.id
		LEFT JOIN research_evidence re ON re.run_id = rr.id
		WHERE rr.id = $1
		GROUP BY rr.id, ra.answer
	`, runUUID, includeAnswer))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResearchRunDetail{}, ErrNotFound
		}
		return ResearchRunDetail{}, err
	}
	contexts := make([]ResearchRunContext, 0)
	if includeAnswer {
		contexts, err = runContexts(ctx, s.Pool, runUUID)
		if err != nil {
			return ResearchRunDetail{}, err
		}
	}
	return ResearchRunDetail{ResearchRunSummary: summary, Contexts: contexts}, nil
}

func listRuns(ctx context.Context, executor historyExecutor, questionID uuid.UUID, includeAnswer bool) ([]ResearchRunSummary, error) {
	rows, err := executor.Query(ctx, `
		SELECT rr.id, rr.question_id, rr.query, rr.status, rr.insufficient_evidence,
		       count(re.id), CASE WHEN $2 THEN COALESCE(ra.answer, '') ELSE '' END,
		       COALESCE(rr.model_version, ''), COALESCE(rr.semantic_route, ''), rr.synthesis_attempted,
		       CASE WHEN $2 THEN COALESCE(rr.error, '') ELSE '' END, rr.created_at, rr.updated_at
		FROM research_runs rr
		LEFT JOIN research_answers ra ON ra.run_id = rr.id
		LEFT JOIN research_evidence re ON re.run_id = rr.id
		WHERE rr.question_id = $1
		GROUP BY rr.id, ra.answer
		ORDER BY rr.created_at DESC
		LIMIT 100
	`, questionID, includeAnswer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ResearchRunSummary, 0)
	for rows.Next() {
		item, scanErr := scanResearchRunSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func runContexts(ctx context.Context, executor historyExecutor, runID uuid.UUID) ([]ResearchRunContext, error) {
	rows, err := executor.Query(ctx, `SELECT scope_type, scope_id, role FROM research_run_contexts WHERE run_id = $1 ORDER BY scope_type, role, scope_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ResearchRunContext, 0)
	for rows.Next() {
		var item ResearchRunContext
		var scopeID pgtype.UUID
		if err := rows.Scan(&item.ScopeType, &scopeID, &item.Role); err != nil {
			return nil, err
		}
		item.ScopeID = uuidText(scopeID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanResearchRunSummary(rows pgx.Rows) (ResearchRunSummary, error) {
	var item ResearchRunSummary
	var id, questionID pgtype.UUID
	var modelVersion, route, runError pgtype.Text
	if err := rows.Scan(&id, &questionID, &item.Query, &item.Status, &item.InsufficientEvidence, &item.CitationCount, &item.AnswerAR, &modelVersion, &route, &item.SynthesisAttempted, &runError, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ResearchRunSummary{}, err
	}
	item.ID = uuidText(id)
	item.QuestionID = uuidText(questionID)
	item.ModelVersion = textValue(modelVersion)
	item.Route = textValue(route)
	item.Error = textValue(runError)
	return item, nil
}

func scanRunSummary(row pgx.Row) (ResearchRunSummary, error) {
	var item ResearchRunSummary
	var id, questionID pgtype.UUID
	var modelVersion, route, runError pgtype.Text
	if err := row.Scan(&id, &questionID, &item.Query, &item.Status, &item.InsufficientEvidence, &item.CitationCount, &item.AnswerAR, &modelVersion, &route, &item.SynthesisAttempted, &runError, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ResearchRunSummary{}, err
	}
	item.ID = uuidText(id)
	item.QuestionID = uuidText(questionID)
	item.ModelVersion = textValue(modelVersion)
	item.Route = textValue(route)
	item.Error = textValue(runError)
	return item, nil
}

func (s *Service) canViewResearchHistory(ctx context.Context, actorID string) (bool, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return false, nil
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return false, ErrForbidden
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('researcher', 'moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}
