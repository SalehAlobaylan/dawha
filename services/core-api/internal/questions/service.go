package questions

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
	ErrDatabaseUnavailable = errors.New("questions database is unavailable")
	ErrNotFound            = errors.New("question resource not found")
	ErrForbidden           = errors.New("question access is forbidden")
	ErrValidation          = errors.New("question input is invalid")
)

type CreateQuestionInput struct {
	TitleAR       string `json:"title_ar"`
	DescriptionAR string `json:"description_ar"`
	Status        string `json:"status"`
	Priority      string `json:"priority"`
}

type UpdateQuestionInput struct {
	TitleAR       string `json:"title_ar"`
	DescriptionAR string `json:"description_ar"`
	Status        string `json:"status"`
	Priority      string `json:"priority"`
}

type QuestionNoteInput struct {
	NoteAR string `json:"note_ar"`
}

type QuestionClaimInput struct {
	ClaimID string `json:"claim_id"`
	Role    string `json:"role"`
}

type QuestionSourceInput struct {
	SourceID string `json:"source_id"`
	Role     string `json:"role"`
}

type QuestionDisputeInput struct {
	DisputeID string `json:"dispute_id"`
}

type CreateDisputeInput struct {
	TitleAR       string `json:"title_ar"`
	DescriptionAR string `json:"description_ar"`
	Status        string `json:"status"`
}

type UpdateDisputeInput struct {
	Status       string `json:"status"`
	ResolutionAR string `json:"resolution_ar"`
}

type DisputeClaimInput struct {
	ClaimID  string `json:"claim_id"`
	Position string `json:"position"`
}

type QuestionSummary struct {
	ID            string    `json:"id"`
	TitleAR       string    `json:"titleAr"`
	DescriptionAR string    `json:"descriptionAr,omitempty"`
	Status        string    `json:"status"`
	Priority      string    `json:"priority"`
	CreatedBy     string    `json:"createdBy"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ClaimCount    int       `json:"claimCount"`
	SourceCount   int       `json:"sourceCount"`
	DisputeCount  int       `json:"disputeCount"`
	NoteCount     int       `json:"noteCount"`
}

type QuestionClaimView struct {
	ClaimID     string `json:"claimId"`
	Role        string `json:"role"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Predicate   string `json:"predicate"`
	ObjectType  string `json:"objectType"`
	ObjectID    string `json:"objectId"`
	Status      string `json:"status"`
}

type QuestionSourceView struct {
	SourceID string `json:"sourceId"`
	Role     string `json:"role"`
	TitleAR  string `json:"titleAr"`
}

type QuestionDisputeView struct {
	DisputeID string `json:"disputeId"`
	TitleAR   string `json:"titleAr"`
	Status    string `json:"status"`
}

type QuestionNoteView struct {
	ID        string    `json:"id"`
	NoteAR    string    `json:"noteAr"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

type QuestionActivity struct {
	Action     string          `json:"action"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	CreatedAt  time.Time       `json:"createdAt"`
	After      json.RawMessage `json:"after,omitempty"`
}

type QuestionDetail struct {
	Question QuestionSummary       `json:"question"`
	Claims   []QuestionClaimView   `json:"claims"`
	Sources  []QuestionSourceView  `json:"sources"`
	Disputes []QuestionDisputeView `json:"disputes"`
	Notes    []QuestionNoteView    `json:"notes"`
	Activity []QuestionActivity    `json:"activity"`
}

type DisputeClaimView struct {
	ClaimID     string `json:"claimId"`
	Position    string `json:"position"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Predicate   string `json:"predicate"`
	ObjectType  string `json:"objectType"`
	ObjectID    string `json:"objectId"`
	Status      string `json:"status"`
}

type DisputeSummary struct {
	ID            string    `json:"id"`
	TitleAR       string    `json:"titleAr"`
	DescriptionAR string    `json:"descriptionAr,omitempty"`
	Status        string    `json:"status"`
	ResolutionAR  string    `json:"resolutionAr,omitempty"`
	CreatedBy     string    `json:"createdBy"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ClaimCount    int       `json:"claimCount"`
}

type DisputeDetail struct {
	Dispute DisputeSummary     `json:"dispute"`
	Claims  []DisputeClaimView `json:"claims"`
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

func (s *Service) ListQuestions(ctx context.Context) ([]QuestionSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, questionListQuery+` ORDER BY q.updated_at DESC, q.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionSummary, 0)
	for rows.Next() {
		item, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetQuestion(ctx context.Context, questionID string) (QuestionDetail, error) {
	if err := s.ready(); err != nil {
		return QuestionDetail{}, err
	}
	questionUUID, err := uuid.Parse(questionID)
	if err != nil {
		return QuestionDetail{}, ErrNotFound
	}
	question, err := s.questionByID(ctx, s.Pool, questionUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	claims, err := s.questionClaims(ctx, s.Pool, questionUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	sources, err := s.questionSources(ctx, s.Pool, questionUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	disputes, err := s.questionDisputes(ctx, s.Pool, questionUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	notes, err := s.questionNotes(ctx, s.Pool, questionUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	activity, err := s.questionActivity(ctx, s.Pool, questionUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	return QuestionDetail{Question: question, Claims: claims, Sources: sources, Disputes: disputes, Notes: notes, Activity: activity}, nil
}

func (s *Service) CreateQuestion(ctx context.Context, actorID string, input CreateQuestionInput) (QuestionDetail, error) {
	if err := s.ready(); err != nil {
		return QuestionDetail{}, err
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return QuestionDetail{}, ErrForbidden
	}
	input, err = validateCreateQuestionInput(input)
	if err != nil {
		return QuestionDetail{}, err
	}
	questionID := uuid.New()
	if _, err := s.Pool.Exec(ctx, `
		INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6)
	`, questionID, input.TitleAR, input.DescriptionAR, input.Status, input.Priority, actorUUID); err != nil {
		return QuestionDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "question_created", "open_question", questionID, nil, input); err != nil {
		return QuestionDetail{}, err
	}
	return s.GetQuestion(ctx, questionID.String())
}

func (s *Service) UpdateQuestion(ctx context.Context, questionID, actorID string, input UpdateQuestionInput) (QuestionDetail, error) {
	if err := s.ready(); err != nil {
		return QuestionDetail{}, err
	}
	questionUUID, actorUUID, err := parseActorResource(questionID, actorID)
	if err != nil {
		return QuestionDetail{}, err
	}
	allowed, err := canManageQuestion(ctx, s.Pool, questionUUID, actorUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	if !allowed {
		return QuestionDetail{}, ErrForbidden
	}
	current, err := s.questionByID(ctx, s.Pool, questionUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	merged := UpdateQuestionInput{TitleAR: current.TitleAR, DescriptionAR: current.DescriptionAR, Status: current.Status, Priority: current.Priority}
	if strings.TrimSpace(input.TitleAR) != "" {
		merged.TitleAR = input.TitleAR
	}
	if strings.TrimSpace(input.DescriptionAR) != "" {
		merged.DescriptionAR = input.DescriptionAR
	}
	if strings.TrimSpace(input.Status) != "" {
		merged.Status = input.Status
	}
	if strings.TrimSpace(input.Priority) != "" {
		merged.Priority = input.Priority
	}
	merged, err = validateUpdateQuestionInput(merged, false)
	if err != nil {
		return QuestionDetail{}, err
	}
	if _, err := s.Pool.Exec(ctx, `
		UPDATE open_questions
		SET title_ar = $1, description_ar = NULLIF($2, ''), status = $3, priority = $4, updated_at = now()
		WHERE id = $5
	`, merged.TitleAR, merged.DescriptionAR, merged.Status, merged.Priority, questionUUID); err != nil {
		return QuestionDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "question_updated", "open_question", questionUUID, current, merged); err != nil {
		return QuestionDetail{}, err
	}
	return s.GetQuestion(ctx, questionUUID.String())
}

func (s *Service) AddNote(ctx context.Context, questionID, actorID string, input QuestionNoteInput) (QuestionDetail, error) {
	if err := s.ready(); err != nil {
		return QuestionDetail{}, err
	}
	questionUUID, actorUUID, err := parseActorResource(questionID, actorID)
	if err != nil {
		return QuestionDetail{}, err
	}
	allowed, err := canManageQuestion(ctx, s.Pool, questionUUID, actorUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	if !allowed {
		return QuestionDetail{}, ErrForbidden
	}
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if input.NoteAR == "" || len([]rune(input.NoteAR)) > 5000 {
		return QuestionDetail{}, ErrValidation
	}
	noteID := uuid.New()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO question_notes (id, question_id, note_ar, created_by) VALUES ($1, $2, $3, $4)`, noteID, questionUUID, input.NoteAR, actorUUID); err != nil {
		return QuestionDetail{}, err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE open_questions SET updated_at = now() WHERE id = $1`, questionUUID); err != nil {
		return QuestionDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "question_note_added", "question_note", noteID, nil, input); err != nil {
		return QuestionDetail{}, err
	}
	return s.GetQuestion(ctx, questionUUID.String())
}

func (s *Service) LinkClaim(ctx context.Context, questionID, actorID string, input QuestionClaimInput) (QuestionDetail, error) {
	if err := s.ready(); err != nil {
		return QuestionDetail{}, err
	}
	questionUUID, actorUUID, err := parseActorResource(questionID, actorID)
	if err != nil {
		return QuestionDetail{}, err
	}
	allowed, err := canManageQuestion(ctx, s.Pool, questionUUID, actorUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	if !allowed {
		return QuestionDetail{}, ErrForbidden
	}
	claimUUID, err := validateClaimLink(input.ClaimID, input.Role, []string{"concerns", "supports", "opposes"})
	if err != nil {
		return QuestionDetail{}, err
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO question_claims (question_id, claim_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, questionUUID, claimUUID, input.Role); err != nil {
		return QuestionDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "question_claim_linked", "open_question", questionUUID, nil, input); err != nil {
		return QuestionDetail{}, err
	}
	return s.GetQuestion(ctx, questionUUID.String())
}

func (s *Service) LinkSource(ctx context.Context, questionID, actorID string, input QuestionSourceInput) (QuestionDetail, error) {
	if err := s.ready(); err != nil {
		return QuestionDetail{}, err
	}
	questionUUID, actorUUID, err := parseActorResource(questionID, actorID)
	if err != nil {
		return QuestionDetail{}, err
	}
	allowed, err := canManageQuestion(ctx, s.Pool, questionUUID, actorUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	if !allowed {
		return QuestionDetail{}, ErrForbidden
	}
	sourceUUID, err := validateSourceLink(input.SourceID, input.Role)
	if err != nil {
		return QuestionDetail{}, err
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO question_sources (question_id, source_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, questionUUID, sourceUUID, input.Role); err != nil {
		return QuestionDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "question_source_linked", "open_question", questionUUID, nil, input); err != nil {
		return QuestionDetail{}, err
	}
	return s.GetQuestion(ctx, questionUUID.String())
}

func (s *Service) LinkDispute(ctx context.Context, questionID, actorID string, input QuestionDisputeInput) (QuestionDetail, error) {
	if err := s.ready(); err != nil {
		return QuestionDetail{}, err
	}
	questionUUID, actorUUID, err := parseActorResource(questionID, actorID)
	if err != nil {
		return QuestionDetail{}, err
	}
	allowed, err := canManageQuestion(ctx, s.Pool, questionUUID, actorUUID)
	if err != nil {
		return QuestionDetail{}, err
	}
	if !allowed {
		return QuestionDetail{}, ErrForbidden
	}
	disputeUUID, err := uuid.Parse(strings.TrimSpace(input.DisputeID))
	if err != nil {
		return QuestionDetail{}, ErrValidation
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO question_disputes (question_id, dispute_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, questionUUID, disputeUUID); err != nil {
		return QuestionDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "question_dispute_linked", "open_question", questionUUID, nil, input); err != nil {
		return QuestionDetail{}, err
	}
	return s.GetQuestion(ctx, questionUUID.String())
}

func (s *Service) ListDisputes(ctx context.Context) ([]DisputeSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, disputeListQuery+` ORDER BY d.updated_at DESC, d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DisputeSummary, 0)
	for rows.Next() {
		item, err := scanDispute(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetDispute(ctx context.Context, disputeID string) (DisputeDetail, error) {
	if err := s.ready(); err != nil {
		return DisputeDetail{}, err
	}
	disputeUUID, err := uuid.Parse(disputeID)
	if err != nil {
		return DisputeDetail{}, ErrNotFound
	}
	dispute, err := s.disputeByID(ctx, s.Pool, disputeUUID)
	if err != nil {
		return DisputeDetail{}, err
	}
	claims, err := s.disputeClaims(ctx, s.Pool, disputeUUID)
	if err != nil {
		return DisputeDetail{}, err
	}
	return DisputeDetail{Dispute: dispute, Claims: claims}, nil
}

func (s *Service) CreateDispute(ctx context.Context, actorID string, input CreateDisputeInput) (DisputeDetail, error) {
	if err := s.ready(); err != nil {
		return DisputeDetail{}, err
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return DisputeDetail{}, ErrForbidden
	}
	input, err = validateDisputeInput(input, true)
	if err != nil {
		return DisputeDetail{}, err
	}
	disputeID := uuid.New()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO disputes (id, title_ar, description_ar, status, created_by) VALUES ($1, $2, NULLIF($3, ''), $4, $5)`, disputeID, input.TitleAR, input.DescriptionAR, input.Status, actorUUID); err != nil {
		return DisputeDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "dispute_created", "dispute", disputeID, nil, input); err != nil {
		return DisputeDetail{}, err
	}
	return s.GetDispute(ctx, disputeID.String())
}

func (s *Service) UpdateDispute(ctx context.Context, disputeID, actorID string, input UpdateDisputeInput) (DisputeDetail, error) {
	if err := s.ready(); err != nil {
		return DisputeDetail{}, err
	}
	disputeUUID, actorUUID, err := parseActorResource(disputeID, actorID)
	if err != nil {
		return DisputeDetail{}, err
	}
	allowed, err := canManageDispute(ctx, s.Pool, disputeUUID, actorUUID)
	if err != nil {
		return DisputeDetail{}, err
	}
	if !allowed {
		return DisputeDetail{}, ErrForbidden
	}
	current, err := s.disputeByID(ctx, s.Pool, disputeUUID)
	if err != nil {
		return DisputeDetail{}, err
	}
	merged := UpdateDisputeInput{Status: current.Status, ResolutionAR: current.ResolutionAR}
	if strings.TrimSpace(input.Status) != "" {
		merged.Status = strings.TrimSpace(input.Status)
	}
	if strings.TrimSpace(input.ResolutionAR) != "" {
		merged.ResolutionAR = strings.TrimSpace(input.ResolutionAR)
	}
	if !validDisputeStatus(merged.Status) || len([]rune(merged.ResolutionAR)) > 5000 {
		return DisputeDetail{}, ErrValidation
	}
	if merged.Status == "resolved" && merged.ResolutionAR == "" {
		return DisputeDetail{}, ErrValidation
	}
	if _, err := s.Pool.Exec(ctx, `
		UPDATE disputes
		SET status = $1, resolution_ar = NULLIF($2, ''), resolved_by = CASE WHEN $1 = 'resolved' THEN $3::uuid ELSE NULL::uuid END, updated_at = now()
		WHERE id = $4
	`, merged.Status, merged.ResolutionAR, actorUUID, disputeUUID); err != nil {
		return DisputeDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "dispute_updated", "dispute", disputeUUID, current, merged); err != nil {
		return DisputeDetail{}, err
	}
	return s.GetDispute(ctx, disputeUUID.String())
}

func (s *Service) LinkDisputeClaim(ctx context.Context, disputeID, actorID string, input DisputeClaimInput) (DisputeDetail, error) {
	if err := s.ready(); err != nil {
		return DisputeDetail{}, err
	}
	disputeUUID, actorUUID, err := parseActorResource(disputeID, actorID)
	if err != nil {
		return DisputeDetail{}, err
	}
	allowed, err := canManageDispute(ctx, s.Pool, disputeUUID, actorUUID)
	if err != nil {
		return DisputeDetail{}, err
	}
	if !allowed {
		return DisputeDetail{}, ErrForbidden
	}
	claimUUID, err := validateClaimLink(input.ClaimID, input.Position, []string{"concerns", "supports", "opposes", "mentions"})
	if err != nil {
		return DisputeDetail{}, err
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO dispute_claims (dispute_id, claim_id, position) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, disputeUUID, claimUUID, input.Position); err != nil {
		return DisputeDetail{}, err
	}
	if err := writeAudit(ctx, s.Pool, actorUUID, "dispute_claim_linked", "dispute", disputeUUID, nil, input); err != nil {
		return DisputeDetail{}, err
	}
	return s.GetDispute(ctx, disputeUUID.String())
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

const questionListQuery = `
	SELECT q.id, q.title_ar, q.description_ar, q.status, q.priority, q.created_by, q.created_at, q.updated_at,
	       (SELECT count(*) FROM question_claims qc WHERE qc.question_id = q.id),
	       (SELECT count(*) FROM question_sources qs WHERE qs.question_id = q.id),
	       (SELECT count(*) FROM question_disputes qd WHERE qd.question_id = q.id),
	       (SELECT count(*) FROM question_notes qn WHERE qn.question_id = q.id)
	FROM open_questions q`

func scanQuestion(row pgx.Row) (QuestionSummary, error) {
	var item QuestionSummary
	var description pgtype.Text
	var createdBy pgtype.UUID
	if err := row.Scan(&item.ID, &item.TitleAR, &description, &item.Status, &item.Priority, &createdBy, &item.CreatedAt, &item.UpdatedAt, &item.ClaimCount, &item.SourceCount, &item.DisputeCount, &item.NoteCount); err != nil {
		return QuestionSummary{}, err
	}
	item.DescriptionAR = textValue(description)
	item.CreatedBy = uuidString(createdBy)
	return item, nil
}

func (s *Service) questionByID(ctx context.Context, q dbExecutor, questionID uuid.UUID) (QuestionSummary, error) {
	item, err := scanQuestion(q.QueryRow(ctx, questionListQuery+` WHERE q.id = $1`, questionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QuestionSummary{}, ErrNotFound
		}
		return QuestionSummary{}, err
	}
	return item, nil
}

func (s *Service) questionClaims(ctx context.Context, q dbExecutor, questionID uuid.UUID) ([]QuestionClaimView, error) {
	rows, err := q.Query(ctx, `
		SELECT qc.claim_id, qc.role, c.subject_type, c.subject_id, c.predicate, c.object_type, c.object_id, c.status
		FROM question_claims qc JOIN claims c ON c.id = qc.claim_id
		WHERE qc.question_id = $1 ORDER BY c.created_at
	`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionClaimView, 0)
	for rows.Next() {
		var item QuestionClaimView
		var claimID, subjectID, objectID pgtype.UUID
		if err := rows.Scan(&claimID, &item.Role, &item.SubjectType, &subjectID, &item.Predicate, &item.ObjectType, &objectID, &item.Status); err != nil {
			return nil, err
		}
		item.ClaimID = uuidString(claimID)
		item.SubjectID = uuidString(subjectID)
		item.ObjectID = uuidString(objectID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) questionSources(ctx context.Context, q dbExecutor, questionID uuid.UUID) ([]QuestionSourceView, error) {
	rows, err := q.Query(ctx, `SELECT qs.source_id, qs.role, s.title_ar FROM question_sources qs JOIN sources s ON s.id = qs.source_id WHERE qs.question_id = $1 ORDER BY s.title_ar`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionSourceView, 0)
	for rows.Next() {
		var item QuestionSourceView
		var sourceID pgtype.UUID
		if err := rows.Scan(&sourceID, &item.Role, &item.TitleAR); err != nil {
			return nil, err
		}
		item.SourceID = uuidString(sourceID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) questionDisputes(ctx context.Context, q dbExecutor, questionID uuid.UUID) ([]QuestionDisputeView, error) {
	rows, err := q.Query(ctx, `SELECT qd.dispute_id, d.title_ar, d.status FROM question_disputes qd JOIN disputes d ON d.id = qd.dispute_id WHERE qd.question_id = $1 ORDER BY d.updated_at DESC`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionDisputeView, 0)
	for rows.Next() {
		var item QuestionDisputeView
		var disputeID pgtype.UUID
		if err := rows.Scan(&disputeID, &item.TitleAR, &item.Status); err != nil {
			return nil, err
		}
		item.DisputeID = uuidString(disputeID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) questionActivity(ctx context.Context, q dbExecutor, questionID uuid.UUID) ([]QuestionActivity, error) {
	rows, err := q.Query(ctx, `
		SELECT a.action, a.entity_type, a.entity_id, a.created_at, a.after_value
		FROM audit_log a
		WHERE (a.entity_type = 'open_question' AND a.entity_id = $1)
		   OR (a.entity_type = 'question_note' AND a.entity_id IN (SELECT id FROM question_notes WHERE question_id = $1))
		   OR (a.entity_type = 'dispute' AND a.entity_id IN (SELECT dispute_id FROM question_disputes WHERE question_id = $1))
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT 100
	`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionActivity, 0)
	for rows.Next() {
		var item QuestionActivity
		var entityID pgtype.UUID
		var after []byte
		if err := rows.Scan(&item.Action, &item.EntityType, &entityID, &item.CreatedAt, &after); err != nil {
			return nil, err
		}
		item.EntityID = uuidString(entityID)
		if len(after) > 0 {
			item.After = json.RawMessage(append([]byte(nil), after...))
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) questionNotes(ctx context.Context, q dbExecutor, questionID uuid.UUID) ([]QuestionNoteView, error) {
	rows, err := q.Query(ctx, `SELECT id, note_ar, created_by, created_at FROM question_notes WHERE question_id = $1 ORDER BY created_at DESC`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionNoteView, 0)
	for rows.Next() {
		var item QuestionNoteView
		var id, createdBy pgtype.UUID
		if err := rows.Scan(&id, &item.NoteAR, &createdBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.CreatedBy = uuidString(createdBy)
		items = append(items, item)
	}
	return items, rows.Err()
}

const disputeListQuery = `
	SELECT d.id, d.title_ar, d.description_ar, d.status, d.resolution_ar, d.created_by, d.created_at, d.updated_at,
	       (SELECT count(*) FROM dispute_claims dc WHERE dc.dispute_id = d.id)
	FROM disputes d`

func scanDispute(row pgx.Row) (DisputeSummary, error) {
	var item DisputeSummary
	var description, resolution pgtype.Text
	var createdBy pgtype.UUID
	if err := row.Scan(&item.ID, &item.TitleAR, &description, &item.Status, &resolution, &createdBy, &item.CreatedAt, &item.UpdatedAt, &item.ClaimCount); err != nil {
		return DisputeSummary{}, err
	}
	item.DescriptionAR = textValue(description)
	item.ResolutionAR = textValue(resolution)
	item.CreatedBy = uuidString(createdBy)
	return item, nil
}

func (s *Service) disputeByID(ctx context.Context, q dbExecutor, disputeID uuid.UUID) (DisputeSummary, error) {
	item, err := scanDispute(q.QueryRow(ctx, disputeListQuery+` WHERE d.id = $1`, disputeID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisputeSummary{}, ErrNotFound
		}
		return DisputeSummary{}, err
	}
	return item, nil
}

func (s *Service) disputeClaims(ctx context.Context, q dbExecutor, disputeID uuid.UUID) ([]DisputeClaimView, error) {
	rows, err := q.Query(ctx, `SELECT dc.claim_id, dc.position, c.subject_type, c.subject_id, c.predicate, c.object_type, c.object_id, c.status FROM dispute_claims dc JOIN claims c ON c.id = dc.claim_id WHERE dc.dispute_id = $1 ORDER BY c.created_at`, disputeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DisputeClaimView, 0)
	for rows.Next() {
		var item DisputeClaimView
		var claimID, subjectID, objectID pgtype.UUID
		if err := rows.Scan(&claimID, &item.Position, &item.SubjectType, &subjectID, &item.Predicate, &item.ObjectType, &objectID, &item.Status); err != nil {
			return nil, err
		}
		item.ClaimID = uuidString(claimID)
		item.SubjectID = uuidString(subjectID)
		item.ObjectID = uuidString(objectID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func canManageQuestion(ctx context.Context, q dbExecutor, questionID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM open_questions q WHERE q.id = $1 AND (q.created_by = $2 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('researcher', 'moderator', 'admin'))))`, questionID, actorID).Scan(&allowed)
	return allowed, err
}

func canManageDispute(ctx context.Context, q dbExecutor, disputeID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM disputes d WHERE d.id = $1 AND (d.created_by = $2 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('researcher', 'moderator', 'admin'))))`, disputeID, actorID).Scan(&allowed)
	return allowed, err
}

func validateCreateQuestionInput(input CreateQuestionInput) (CreateQuestionInput, error) {
	normalized, err := normalizeQuestionFields(input.TitleAR, input.DescriptionAR, input.Status, input.Priority, true)
	if err != nil {
		return CreateQuestionInput{}, err
	}
	return CreateQuestionInput{TitleAR: normalized.TitleAR, DescriptionAR: normalized.DescriptionAR, Status: normalized.Status, Priority: normalized.Priority}, nil
}

func validateUpdateQuestionInput(input UpdateQuestionInput, requireTitle bool) (UpdateQuestionInput, error) {
	return normalizeQuestionFields(input.TitleAR, input.DescriptionAR, input.Status, input.Priority, requireTitle)
}

func normalizeQuestionFields(title, description, status, priority string, requireTitle bool) (UpdateQuestionInput, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	status = strings.TrimSpace(status)
	priority = strings.TrimSpace(priority)
	if status == "" {
		status = "open"
	}
	if priority == "" {
		priority = "normal"
	}
	if (requireTitle && title == "") || len([]rune(title)) > 500 || len([]rune(description)) > 10000 || !validQuestionStatus(status) || !validPriority(priority) {
		return UpdateQuestionInput{}, ErrValidation
	}
	return UpdateQuestionInput{TitleAR: title, DescriptionAR: description, Status: status, Priority: priority}, nil
}

func validateDisputeInput(input CreateDisputeInput, requireTitle bool) (CreateDisputeInput, error) {
	input.TitleAR = strings.TrimSpace(input.TitleAR)
	input.DescriptionAR = strings.TrimSpace(input.DescriptionAR)
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "open"
	}
	if (requireTitle && input.TitleAR == "") || len([]rune(input.TitleAR)) > 500 || len([]rune(input.DescriptionAR)) > 10000 || !validDisputeStatus(input.Status) || input.Status == "resolved" {
		return CreateDisputeInput{}, ErrValidation
	}
	return input, nil
}

func validateClaimLink(value, role string, allowedRoles []string) (uuid.UUID, error) {
	claimUUID, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || !contains(allowedRoles, strings.TrimSpace(role)) {
		return uuid.Nil, ErrValidation
	}
	return claimUUID, nil
}

func validateSourceLink(value, role string) (uuid.UUID, error) {
	sourceUUID, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || !contains([]string{"context", "supporting", "counter_evidence", "missing"}, strings.TrimSpace(role)) {
		return uuid.Nil, ErrValidation
	}
	return sourceUUID, nil
}

func validQuestionStatus(value string) bool {
	return value == "open" || value == "under_investigation" || value == "resolved" || value == "reopened" || value == "archived"
}

func validDisputeStatus(value string) bool {
	return value == "open" || value == "under_review" || value == "resolved" || value == "reopened" || value == "archived"
}

func validPriority(value string) bool {
	return value == "low" || value == "normal" || value == "high"
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func parseActorResource(resourceID, actorID string) (uuid.UUID, uuid.UUID, error) {
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

func writeAudit(ctx context.Context, q dbExecutor, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) error {
	_, err := q.Exec(ctx, `INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value) VALUES ($1, $2, $3, $4, $5, $6)`, actorID, action, entityType, entityID, marshalValue(before), marshalValue(after))
	return err
}

func marshalValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
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
