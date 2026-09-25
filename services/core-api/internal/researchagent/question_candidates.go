package researchagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const MaximumQuestionCandidates = 50

type QuestionCandidate struct {
	ID                     string                    `json:"id"`
	RunID                  string                    `json:"runId"`
	OriginQuestionID       string                    `json:"originQuestionId,omitempty"`
	OriginGapID            string                    `json:"originGapId"`
	OriginRecommendationID string                    `json:"originRecommendationId,omitempty"`
	TitleAR                string                    `json:"titleAr"`
	DescriptionAR          string                    `json:"descriptionAr"`
	Priority               string                    `json:"priority"`
	Status                 string                    `json:"status"`
	Metadata               map[string]any            `json:"metadata"`
	GeneratedBy            string                    `json:"generatedBy"`
	QuestionID             string                    `json:"questionId,omitempty"`
	ReviewedBy             string                    `json:"reviewedBy,omitempty"`
	ReviewedAt             *time.Time                `json:"reviewedAt,omitempty"`
	ReviewNoteAR           string                    `json:"reviewNoteAr,omitempty"`
	CreatedAt              time.Time                 `json:"createdAt"`
	UpdatedAt              time.Time                 `json:"updatedAt"`
	Reviews                []QuestionCandidateReview `json:"reviews"`
}

type QuestionCandidateReview struct {
	ID         string    `json:"id"`
	ReviewerID string    `json:"reviewerId"`
	Decision   string    `json:"decision"`
	QuestionID string    `json:"questionId,omitempty"`
	NoteAR     string    `json:"noteAr,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ReviewQuestionCandidateInput struct {
	Decision      string `json:"decision"`
	TitleAR       string `json:"title_ar"`
	DescriptionAR string `json:"description_ar"`
	Priority      string `json:"priority"`
	NoteAR        string `json:"note_ar"`
}

type questionCandidateGap struct {
	ID            string
	Kind          string
	DescriptionAR string
	Severity      string
}

type questionCandidateRecommendation struct {
	ID          string
	Action      string
	RationaleAR string
}

func (s *Service) GenerateQuestionCandidates(ctx context.Context, actorID, runID string) ([]QuestionCandidate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	actorUUID, err := parseCandidateActor(actorID)
	if err != nil {
		return nil, err
	}
	if err := requireRole(ctx, s.Pool, actorUUID); err != nil {
		return nil, err
	}
	runUUID, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return nil, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	run, err := scanRun(tx.QueryRow(ctx, runSelect+` WHERE id = $1 FOR UPDATE`, runUUID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.requireRunAccess(ctx, tx, run, actorID); err != nil {
		return nil, err
	}
	if run.Status != "succeeded" {
		return nil, ErrConflict
	}
	gaps, err := loadCandidateGaps(ctx, tx, runUUID)
	if err != nil {
		return nil, err
	}
	recommendations, err := loadCandidateRecommendations(ctx, tx, runUUID)
	if err != nil {
		return nil, err
	}
	inserted := 0
	for _, gap := range gaps {
		title, description, priority := candidateTemplate(gap)
		recommendationID, rationale := recommendationForGap(gap.Kind, recommendations)
		if rationale != "" {
			description = description + " التوصية: " + rationale
		}
		if len([]rune(description)) > 2000 {
			description = string([]rune(description)[:2000])
		}
		metadata := map[string]any{"gapKind": gap.Kind, "severity": gap.Severity}
		if recommendationID != "" {
			metadata["recommendationAction"] = recommendationForGapAction(gap.Kind, recommendations)
		}
		result, err := tx.Exec(ctx, `INSERT INTO research_question_candidates (run_id, origin_question_id, origin_gap_id, origin_recommendation_id, title_ar, description_ar, priority, dedupe_key, metadata, generated_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT (run_id, dedupe_key) DO NOTHING`, runUUID, nullableUUIDValue(run.QuestionID), gap.ID, nullableUUIDValue(recommendationID), title, description, priority, "gap:"+gap.ID, mustJSON(metadata), actorUUID)
		if err != nil {
			return nil, err
		}
		inserted += int(result.RowsAffected())
	}
	if inserted > 0 {
		if err := writeAudit(ctx, tx, actorUUID, "research_question_candidates_generated", "research_agent_run", runUUID, nil, map[string]any{"candidateCount": inserted, "runId": runUUID.String()}, ""); err != nil {
			return nil, err
		}
	}
	candidates, err := loadQuestionCandidates(ctx, tx, runUUID, "")
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return candidates, nil
}

func (s *Service) ListQuestionCandidates(ctx context.Context, actorID, runID, status string) ([]QuestionCandidate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	actorUUID, err := parseCandidateActor(actorID)
	if err != nil {
		return nil, err
	}
	if err := requireRole(ctx, s.Pool, actorUUID); err != nil {
		return nil, err
	}
	runUUID, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return nil, ErrNotFound
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && status != "proposed" && status != "converted" && status != "dismissed" {
		return nil, ErrValidation
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	run, err := scanRun(tx.QueryRow(ctx, runSelect+` WHERE id = $1`, runUUID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.requireRunAccess(ctx, tx, run, actorID); err != nil {
		return nil, err
	}
	candidates, err := loadQuestionCandidates(ctx, tx, runUUID, status)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return candidates, nil
}

func (s *Service) ReviewQuestionCandidate(ctx context.Context, actorID, candidateID string, input ReviewQuestionCandidateInput) (QuestionCandidate, error) {
	if err := s.ready(); err != nil {
		return QuestionCandidate{}, err
	}
	actorUUID, err := parseCandidateActor(actorID)
	if err != nil {
		return QuestionCandidate{}, err
	}
	if err := requireRole(ctx, s.Pool, actorUUID); err != nil {
		return QuestionCandidate{}, err
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(candidateID))
	if err != nil {
		return QuestionCandidate{}, ErrNotFound
	}
	input, err = validateQuestionCandidateReview(input)
	if err != nil {
		return QuestionCandidate{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return QuestionCandidate{}, err
	}
	defer tx.Rollback(ctx)
	candidate, run, err := loadQuestionCandidateWithRun(ctx, tx, candidateUUID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuestionCandidate{}, ErrNotFound
	}
	if err != nil {
		return QuestionCandidate{}, err
	}
	if err := s.requireRunAccess(ctx, tx, run, actorID); err != nil {
		return QuestionCandidate{}, err
	}
	if candidate.Status != "proposed" {
		return QuestionCandidate{}, ErrConflict
	}
	before := map[string]any{"status": candidate.Status}
	if input.Decision == "converted" {
		title := candidate.TitleAR
		if input.TitleAR != "" {
			title = input.TitleAR
		}
		description := candidate.DescriptionAR
		if input.DescriptionAR != "" {
			description = input.DescriptionAR
		}
		priority := candidate.Priority
		if input.Priority != "" {
			priority = input.Priority
		}
		questionID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by) VALUES ($1, $2, $3, 'open', $4, $5)`, questionID, title, description, priority, actorUUID); err != nil {
			return QuestionCandidate{}, err
		}
		if run.EntityType != "source" {
			if _, err := tx.Exec(ctx, `INSERT INTO question_entities (question_id, entity_type, entity_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, questionID, run.EntityType, run.EntityID); err != nil {
				return QuestionCandidate{}, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE research_question_candidates SET status = 'converted', question_id = $1, reviewed_by = $2, reviewed_at = now(), review_note_ar = NULLIF($3, ''), updated_at = now() WHERE id = $4`, questionID, actorUUID, input.NoteAR, candidateUUID); err != nil {
			return QuestionCandidate{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO research_question_candidate_reviews (candidate_id, reviewer_id, decision, question_id, note_ar) VALUES ($1, $2, 'converted', $3, NULLIF($4, ''))`, candidateUUID, actorUUID, questionID, input.NoteAR); err != nil {
			return QuestionCandidate{}, err
		}
		if err := writeAudit(ctx, tx, actorUUID, "research_question_candidate_converted", "research_question_candidate", candidateUUID, before, map[string]any{"questionId": questionID.String(), "titleAr": title, "entityType": run.EntityType, "entityId": run.EntityID}, input.NoteAR); err != nil {
			return QuestionCandidate{}, err
		}
		if err := writeAudit(ctx, tx, actorUUID, "question_created_from_research_candidate", "open_question", questionID, nil, map[string]any{"candidateId": candidateUUID.String(), "titleAr": title}, input.NoteAR); err != nil {
			return QuestionCandidate{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE research_question_candidates SET status = 'dismissed', reviewed_by = $1, reviewed_at = now(), review_note_ar = NULLIF($2, ''), updated_at = now() WHERE id = $3`, actorUUID, input.NoteAR, candidateUUID); err != nil {
			return QuestionCandidate{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO research_question_candidate_reviews (candidate_id, reviewer_id, decision, note_ar) VALUES ($1, $2, 'dismissed', NULLIF($3, ''))`, candidateUUID, actorUUID, input.NoteAR); err != nil {
			return QuestionCandidate{}, err
		}
		if err := writeAudit(ctx, tx, actorUUID, "research_question_candidate_dismissed", "research_question_candidate", candidateUUID, before, map[string]any{"status": "dismissed"}, input.NoteAR); err != nil {
			return QuestionCandidate{}, err
		}
	}
	updated, err := loadQuestionCandidate(ctx, tx, candidateUUID)
	if err != nil {
		return QuestionCandidate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QuestionCandidate{}, err
	}
	return updated, nil
}

func loadCandidateGaps(ctx context.Context, q queryer, runID uuid.UUID) ([]questionCandidateGap, error) {
	rows, err := q.Query(ctx, `SELECT id::text, kind, description_ar, severity FROM research_agent_gaps WHERE run_id = $1 ORDER BY created_at, id LIMIT $2`, runID, MaximumQuestionCandidates)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]questionCandidateGap, 0)
	for rows.Next() {
		var item questionCandidateGap
		if err := rows.Scan(&item.ID, &item.Kind, &item.DescriptionAR, &item.Severity); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadCandidateRecommendations(ctx context.Context, q queryer, runID uuid.UUID) ([]questionCandidateRecommendation, error) {
	rows, err := q.Query(ctx, `SELECT id::text, action, rationale_ar FROM research_agent_recommendations WHERE run_id = $1 ORDER BY created_at, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]questionCandidateRecommendation, 0)
	for rows.Next() {
		var item questionCandidateRecommendation
		if err := rows.Scan(&item.ID, &item.Action, &item.RationaleAR); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func recommendationForGap(kind string, recommendations []questionCandidateRecommendation) (string, string) {
	action := recommendationActionForGap(kind)
	for _, item := range recommendations {
		if item.Action == action {
			return item.ID, item.RationaleAR
		}
	}
	return "", ""
}

func recommendationForGapAction(kind string, recommendations []questionCandidateRecommendation) string {
	action := recommendationActionForGap(kind)
	for _, item := range recommendations {
		if item.Action == action {
			return item.Action
		}
	}
	return ""
}

func recommendationActionForGap(kind string) string {
	switch kind {
	case "missing_source_evidence":
		return "توسيع البحث المصدري"
	case "missing_counter_evidence":
		return "فحص الروايات المقابلة"
	case "source_dependency":
		return "مراجعة الاعتماد بين المصادر"
	case "missing_graph_path":
		return "توسيع نطاق الشجرة"
	default:
		return ""
	}
}

func candidateTemplate(gap questionCandidateGap) (string, string, string) {
	var title string
	switch gap.Kind {
	case "missing_source_evidence":
		title = "بحث عن مصادر مستقلة تدعم السؤال"
	case "missing_counter_evidence":
		title = "فحص أدلة مضادة وروايات مقابلة"
	case "source_dependency":
		title = "مراجعة اعتماد المصادر قبل زيادة وزن الأدلة"
	case "missing_graph_path":
		title = "توضيح المسار البنيوي داخل الشجرة"
	case "missing_geography":
		title = "توثيق الإشارات الجغرافية الناقصة"
	case "missing_chronology":
		title = "مراجعة التسلسل الزمني غير المحسوم"
	default:
		title = "فجوة بحث تحتاج إلى مراجعة"
	}
	description := "رصد وكيل البحث فجوة: " + gap.DescriptionAR + "."
	if gap.Severity == "high" {
		description += " الأولوية مرتفعة، وتستحق مراجعة باحث قبل تحويلها إلى سؤال مفتوح."
	} else {
		description += " يلزم جمع مراجعة بشرية قبل تحويلها إلى سؤال مفتوح."
	}
	return title, description, priorityForGap(gap.Severity)
}

func priorityForGap(severity string) string {
	switch severity {
	case "high":
		return "high"
	case "low":
		return "low"
	default:
		return "normal"
	}
}

func loadQuestionCandidates(ctx context.Context, q queryer, runID uuid.UUID, status string) ([]QuestionCandidate, error) {
	query := `
		SELECT id::text, run_id::text, COALESCE(origin_question_id::text, ''), origin_gap_id::text,
		       COALESCE(origin_recommendation_id::text, ''), title_ar, description_ar, priority, status,
		       metadata, generated_by::text, COALESCE(question_id::text, ''), COALESCE(reviewed_by::text, ''),
		       reviewed_at, COALESCE(review_note_ar, ''), created_at, updated_at
		FROM research_question_candidates
		WHERE run_id = $1`
	args := []any{runID}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY created_at, id`
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionCandidate, 0)
	for rows.Next() {
		item, err := scanQuestionCandidate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for index := range items {
		items[index].Reviews, err = loadQuestionCandidateReviews(ctx, q, uuid.MustParse(items[index].ID))
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func loadQuestionCandidateWithRun(ctx context.Context, q queryer, candidateID uuid.UUID, forUpdate bool) (QuestionCandidate, Run, error) {
	query := questionCandidateSelect + ` WHERE c.id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	candidate, err := scanQuestionCandidate(q.QueryRow(ctx, query, candidateID))
	if err != nil {
		return QuestionCandidate{}, Run{}, err
	}
	run, err := scanRun(q.QueryRow(ctx, runSelect+` WHERE id = $1`, candidate.RunID))
	return candidate, run, err
}

func loadQuestionCandidate(ctx context.Context, q queryer, candidateID uuid.UUID) (QuestionCandidate, error) {
	candidate, err := scanQuestionCandidate(q.QueryRow(ctx, questionCandidateSelect+` WHERE c.id = $1`, candidateID))
	if err != nil {
		return QuestionCandidate{}, err
	}
	candidate.Reviews, err = loadQuestionCandidateReviews(ctx, q, candidateID)
	return candidate, err
}

func loadQuestionCandidateReviews(ctx context.Context, q queryer, candidateID uuid.UUID) ([]QuestionCandidateReview, error) {
	rows, err := q.Query(ctx, `SELECT id::text, reviewer_id::text, decision, COALESCE(question_id::text, ''), COALESCE(note_ar, ''), created_at FROM research_question_candidate_reviews WHERE candidate_id = $1 ORDER BY created_at DESC`, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionCandidateReview, 0)
	for rows.Next() {
		var item QuestionCandidateReview
		if err := rows.Scan(&item.ID, &item.ReviewerID, &item.Decision, &item.QuestionID, &item.NoteAR, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const questionCandidateSelect = `
	SELECT c.id::text, c.run_id::text, COALESCE(c.origin_question_id::text, ''), c.origin_gap_id::text,
	       COALESCE(c.origin_recommendation_id::text, ''), c.title_ar, c.description_ar, c.priority, c.status,
	       c.metadata, c.generated_by::text, COALESCE(c.question_id::text, ''), COALESCE(c.reviewed_by::text, ''),
	       c.reviewed_at, COALESCE(c.review_note_ar, ''), c.created_at, c.updated_at
	FROM research_question_candidates c`

func scanQuestionCandidate(row pgx.Row) (QuestionCandidate, error) {
	var item QuestionCandidate
	var metadata []byte
	var reviewedAt pgtype.Timestamptz
	if err := row.Scan(&item.ID, &item.RunID, &item.OriginQuestionID, &item.OriginGapID, &item.OriginRecommendationID, &item.TitleAR, &item.DescriptionAR, &item.Priority, &item.Status, &metadata, &item.GeneratedBy, &item.QuestionID, &item.ReviewedBy, &reviewedAt, &item.ReviewNoteAR, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return QuestionCandidate{}, err
	}
	item.Metadata = map[string]any{}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &item.Metadata)
	}
	if reviewedAt.Valid {
		value := reviewedAt.Time
		item.ReviewedAt = &value
	}
	return item, nil
}

func validateQuestionCandidateReview(input ReviewQuestionCandidateInput) (ReviewQuestionCandidateInput, error) {
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.TitleAR = strings.TrimSpace(input.TitleAR)
	input.DescriptionAR = strings.TrimSpace(input.DescriptionAR)
	input.Priority = strings.ToLower(strings.TrimSpace(input.Priority))
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if input.Decision != "converted" && input.Decision != "dismissed" {
		return ReviewQuestionCandidateInput{}, ErrValidation
	}
	if input.Priority != "" && input.Priority != "low" && input.Priority != "normal" && input.Priority != "high" {
		return ReviewQuestionCandidateInput{}, ErrValidation
	}
	if len([]rune(input.TitleAR)) > 200 || len([]rune(input.DescriptionAR)) > 2000 || len([]rune(input.NoteAR)) > 2000 {
		return ReviewQuestionCandidateInput{}, ErrValidation
	}
	if input.Decision == "converted" {
		if input.TitleAR != "" && len([]rune(input.TitleAR)) < 3 {
			return ReviewQuestionCandidateInput{}, ErrValidation
		}
		if input.DescriptionAR != "" && len([]rune(input.DescriptionAR)) < 3 {
			return ReviewQuestionCandidateInput{}, ErrValidation
		}
	}
	return input, nil
}

func parseCandidateActor(actorID string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return uuid.Nil, ErrForbidden
	}
	return parsed, nil
}
