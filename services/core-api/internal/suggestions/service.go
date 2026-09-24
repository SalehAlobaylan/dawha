package suggestions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("suggestions database is unavailable")
	ErrNotFound            = errors.New("suggestion resource not found")
	ErrForbidden           = errors.New("suggestion access is forbidden")
	ErrValidation          = errors.New("suggestion input is invalid")
	ErrConflict            = errors.New("suggestion has already been reviewed")
)

type SubmitInput struct {
	TreeID    string `json:"tree_id"`
	VersionID string `json:"version_id"`
	NodeID    string `json:"node_id"`
	TextAR    string `json:"text_ar"`
}

type ReviewInput struct {
	Decision        string `json:"decision"`
	NoteAR          string `json:"note_ar"`
	QuestionTitleAR string `json:"question_title_ar"`
}

type ReviewView struct {
	ID           string    `json:"id"`
	ReviewerID   string    `json:"reviewerId"`
	ReviewerName string    `json:"reviewerName"`
	Decision     string    `json:"decision"`
	NoteAR       string    `json:"noteAr,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type SuggestionView struct {
	ID            string       `json:"id"`
	TreeID        string       `json:"treeId"`
	VersionID     string       `json:"versionId"`
	VersionNumber int          `json:"versionNumber"`
	NodeID        string       `json:"nodeId"`
	TreeName      string       `json:"treeName"`
	NodeName      string       `json:"nodeName"`
	TextAR        string       `json:"textAr"`
	Status        string       `json:"status"`
	QuestionID    string       `json:"questionId,omitempty"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
	ReviewCount   int          `json:"reviewCount"`
	Reviews       []ReviewView `json:"reviews"`
}

type Service struct {
	Pool *pgxpool.Pool
}

type dbExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) Submit(ctx context.Context, input SubmitInput, actorID string) (SuggestionView, error) {
	if err := s.ready(); err != nil {
		return SuggestionView{}, err
	}
	input, err := validateSubmitInput(input)
	if err != nil {
		return SuggestionView{}, err
	}
	var actorUUID *uuid.UUID
	if strings.TrimSpace(actorID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(actorID))
		if err != nil {
			return SuggestionView{}, ErrForbidden
		}
		actorUUID = &parsed
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SuggestionView{}, err
	}
	defer tx.Rollback(ctx)
	allowed, err := s.publicNodeExists(ctx, tx, input.TreeID, input.VersionID, input.NodeID)
	if err != nil {
		return SuggestionView{}, err
	}
	if !allowed {
		return SuggestionView{}, ErrNotFound
	}
	suggestionID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO suggestions (id, tree_id, version_id, node_id, submitted_by, text_ar)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, suggestionID, input.TreeID, input.VersionID, input.NodeID, nullableUUID(actorUUID), input.TextAR); err != nil {
		return SuggestionView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "suggestion_submitted", "suggestion", suggestionID, nil, map[string]any{
		"treeId": input.TreeID, "versionId": input.VersionID, "nodeId": input.NodeID, "textAr": input.TextAR,
	}); err != nil {
		return SuggestionView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SuggestionView{}, err
	}
	return s.Get(ctx, suggestionID.String())
}

func (s *Service) List(ctx context.Context, treeID, actorID string) ([]SuggestionView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	treeUUID, actorUUID, err := parseIDs(treeID, actorID)
	if err != nil {
		return nil, err
	}
	allowed, err := canReview(ctx, s.Pool, treeUUID, actorUUID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.Pool.Query(ctx, suggestionListQuery+` WHERE s.tree_id = $1 ORDER BY (s.status = 'pending') DESC, s.created_at DESC`, treeUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SuggestionView, 0)
	for rows.Next() {
		item, err := scanSuggestion(rows)
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
		items[index].Reviews, err = s.reviews(ctx, s.Pool, id)
		if err != nil {
			return nil, err
		}
		items[index].ReviewCount = len(items[index].Reviews)
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, suggestionID string) (SuggestionView, error) {
	if err := s.ready(); err != nil {
		return SuggestionView{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(suggestionID))
	if err != nil {
		return SuggestionView{}, ErrNotFound
	}
	item, err := s.get(ctx, s.Pool, id)
	if err != nil {
		return SuggestionView{}, err
	}
	item.Reviews, err = s.reviews(ctx, s.Pool, id)
	if err != nil {
		return SuggestionView{}, err
	}
	item.ReviewCount = len(item.Reviews)
	return item, nil
}

func (s *Service) Review(ctx context.Context, suggestionID, actorID string, input ReviewInput) (SuggestionView, error) {
	if err := s.ready(); err != nil {
		return SuggestionView{}, err
	}
	id, reviewerUUID, err := parseIDs(suggestionID, actorID)
	if err != nil {
		return SuggestionView{}, err
	}
	input, err = validateReviewInput(input)
	if err != nil {
		return SuggestionView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SuggestionView{}, err
	}
	defer tx.Rollback(ctx)
	var treeID uuid.UUID
	var status, textAR string
	if err := tx.QueryRow(ctx, `SELECT tree_id, status, text_ar FROM suggestions WHERE id = $1 FOR UPDATE`, id).Scan(&treeID, &status, &textAR); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SuggestionView{}, ErrNotFound
		}
		return SuggestionView{}, err
	}
	allowed, err := canReview(ctx, tx, treeID, reviewerUUID)
	if err != nil {
		return SuggestionView{}, err
	}
	if !allowed {
		return SuggestionView{}, ErrForbidden
	}
	if status != "pending" {
		return SuggestionView{}, ErrConflict
	}
	var questionID *uuid.UUID
	if input.Decision == "converted" {
		createdID := uuid.New()
		title := input.QuestionTitleAR
		if title == "" {
			title = defaultQuestionTitle(textAR)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by)
			VALUES ($1, $2, $3, 'open', 'normal', $4)
		`, createdID, title, textAR, reviewerUUID); err != nil {
			return SuggestionView{}, err
		}
		questionID = &createdID
		if err := writeAudit(ctx, tx, &reviewerUUID, "question_created_from_suggestion", "open_question", createdID, nil, map[string]any{
			"suggestionId": id.String(), "titleAr": title, "descriptionAr": textAR,
		}); err != nil {
			return SuggestionView{}, err
		}
	}
	reviewID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO suggestion_reviews (id, suggestion_id, reviewer_id, decision, note_ar) VALUES ($1, $2, $3, $4, NULLIF($5, ''))`, reviewID, id, reviewerUUID, input.Decision, input.NoteAR); err != nil {
		return SuggestionView{}, err
	}
	newStatus := input.Decision
	if _, err := tx.Exec(ctx, `UPDATE suggestions SET status = $1, question_id = $2, updated_at = now() WHERE id = $3`, newStatus, nullableUUID(questionID), id); err != nil {
		return SuggestionView{}, err
	}
	questionIDValue := ""
	if questionID != nil {
		questionIDValue = questionID.String()
	}
	if err := writeAudit(ctx, tx, &reviewerUUID, "suggestion_reviewed", "suggestion", id, map[string]any{"status": status}, map[string]any{
		"decision": input.Decision, "noteAr": input.NoteAR, "questionId": questionIDValue,
	}); err != nil {
		return SuggestionView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SuggestionView{}, err
	}
	return s.Get(ctx, id.String())
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) publicNodeExists(ctx context.Context, q dbExecutor, treeID, versionID, nodeID string) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM trees t
			JOIN tree_versions v ON v.tree_id = t.id
			JOIN tree_nodes n ON n.tree_version_id = v.id
			WHERE t.id = $1 AND t.visibility = 'public' AND v.id = $2 AND v.state = 'published' AND n.id = $3
		)
	`, treeID, versionID, nodeID).Scan(&allowed)
	return allowed, err
}

const suggestionListQuery = `
	SELECT s.id, s.tree_id, s.version_id, v.version_number, s.node_id, t.name_ar, n.display_name_ar,
	       s.text_ar, s.status, s.question_id, s.created_at, s.updated_at
	FROM suggestions s
	JOIN trees t ON t.id = s.tree_id
	JOIN tree_versions v ON v.id = s.version_id
	JOIN tree_nodes n ON n.id = s.node_id`

func scanSuggestion(row pgx.Row) (SuggestionView, error) {
	var item SuggestionView
	var questionID pgtype.UUID
	if err := row.Scan(&item.ID, &item.TreeID, &item.VersionID, &item.VersionNumber, &item.NodeID, &item.TreeName, &item.NodeName, &item.TextAR, &item.Status, &questionID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return SuggestionView{}, err
	}
	item.QuestionID = uuidString(&questionID)
	return item, nil
}

func (s *Service) get(ctx context.Context, q dbExecutor, suggestionID uuid.UUID) (SuggestionView, error) {
	item, err := scanSuggestion(q.QueryRow(ctx, suggestionListQuery+` WHERE s.id = $1`, suggestionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SuggestionView{}, ErrNotFound
		}
		return SuggestionView{}, err
	}
	return item, nil
}

func (s *Service) reviews(ctx context.Context, q dbExecutor, suggestionID uuid.UUID) ([]ReviewView, error) {
	rows, err := q.Query(ctx, `
		SELECT r.id, r.reviewer_id, u.display_name_ar, r.decision, r.note_ar, r.created_at
		FROM suggestion_reviews r
		JOIN users u ON u.id = r.reviewer_id
		WHERE r.suggestion_id = $1
		ORDER BY r.created_at DESC, r.id DESC
	`, suggestionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ReviewView, 0)
	for rows.Next() {
		var item ReviewView
		var reviewerID pgtype.UUID
		var note pgtype.Text
		if err := rows.Scan(&item.ID, &reviewerID, &item.ReviewerName, &item.Decision, &note, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ReviewerID = uuidString(&reviewerID)
		item.NoteAR = textValue(note)
		items = append(items, item)
	}
	return items, rows.Err()
}

func canReview(ctx context.Context, q dbExecutor, treeID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM trees t
			WHERE t.id = $1 AND (
				t.owner_id = $2
				OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2)
				OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('moderator', 'admin'))
			)
		)
	`, treeID, actorID).Scan(&allowed)
	return allowed, err
}

func validateSubmitInput(input SubmitInput) (SubmitInput, error) {
	if strings.TrimSpace(input.TreeID) == "" || strings.TrimSpace(input.VersionID) == "" || strings.TrimSpace(input.NodeID) == "" || strings.TrimSpace(input.TextAR) == "" || len([]rune(input.TextAR)) > 10000 {
		return SubmitInput{}, ErrValidation
	}
	if _, err := uuid.Parse(strings.TrimSpace(input.TreeID)); err != nil {
		return SubmitInput{}, ErrValidation
	}
	if _, err := uuid.Parse(strings.TrimSpace(input.VersionID)); err != nil {
		return SubmitInput{}, ErrValidation
	}
	if _, err := uuid.Parse(strings.TrimSpace(input.NodeID)); err != nil {
		return SubmitInput{}, ErrValidation
	}
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.VersionID = strings.TrimSpace(input.VersionID)
	input.NodeID = strings.TrimSpace(input.NodeID)
	return input, nil
}

func validateReviewInput(input ReviewInput) (ReviewInput, error) {
	input.Decision = strings.TrimSpace(input.Decision)
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	input.QuestionTitleAR = strings.TrimSpace(input.QuestionTitleAR)
	if (input.Decision != "accepted" && input.Decision != "rejected" && input.Decision != "converted") || len([]rune(input.NoteAR)) > 5000 || len([]rune(input.QuestionTitleAR)) > 500 || (input.Decision == "rejected" && input.NoteAR == "") {
		return ReviewInput{}, ErrValidation
	}
	return input, nil
}

func defaultQuestionTitle(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return "مقترح جديد: " + string(runes)
}

func parseIDs(resourceID, actorID string) (uuid.UUID, uuid.UUID, error) {
	resourceUUID, err := uuid.Parse(strings.TrimSpace(resourceID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrForbidden
	}
	return resourceUUID, actorUUID, nil
}

func writeAudit(ctx context.Context, q dbExecutor, actorID *uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) error {
	var actorValue any
	if actorID != nil {
		actorValue = *actorID
	}
	_, err := q.Exec(ctx, `INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value) VALUES ($1, $2, $3, $4, $5, $6)`, actorValue, action, entityType, entityID, marshalValue(before), marshalValue(after))
	return err
}

func marshalValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func nullableUUID(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return *value
}

func uuidString(value *pgtype.UUID) string {
	if value == nil || !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
