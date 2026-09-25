package suggestions

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
	Decision        string     `json:"decision"`
	NoteAR          string     `json:"note_ar"`
	QuestionTitleAR string     `json:"question_title_ar"`
	ChangeSet       *ChangeSet `json:"change_set,omitempty"`
}

// The targets an accepted suggestion may change. Each one names a typed domain record,
// so an approval is a structured change rather than an edit to the published tree.
const (
	ChangeTargetPerson       = "person"
	ChangeTargetRelationship = "relationship"
	ChangeTargetClaim        = "claim"
	ChangeTargetSourceLink   = "source_link"
)

// PersonChange records another name a reviewer confirmed for a person. The person
// already exists, so the change adds an alias and never rewrites the canonical name.
type PersonChange struct {
	PersonID  string `json:"person_id"`
	NameAR    string `json:"name_ar"`
	AliasType string `json:"alias_type,omitempty"`
}

// RelationshipChange records a relationship between two existing entities. The status
// is not part of the change: a relationship from a suggestion is always left unresolved
// until a researcher resolves it explicitly.
type RelationshipChange struct {
	SubjectType string `json:"subject_type,omitempty"`
	SubjectID   string `json:"subject_id"`
	ObjectType  string `json:"object_type,omitempty"`
	ObjectID    string `json:"object_id"`
	Predicate   string `json:"predicate"`
	ValidFrom   string `json:"valid_from,omitempty"`
	ValidTo     string `json:"valid_to,omitempty"`
}

// ClaimChange records a claim a reviewer approved. The claim status is not part of the
// change: an approved suggestion can propose a claim but never accept it.
type ClaimChange struct {
	SubjectType string `json:"subject_type,omitempty"`
	SubjectID   string `json:"subject_id"`
	ObjectType  string `json:"object_type,omitempty"`
	ObjectID    string `json:"object_id"`
	Predicate   string `json:"predicate"`
	PlaceID     string `json:"place_id,omitempty"`
	TimeFrom    string `json:"time_from,omitempty"`
	TimeTo      string `json:"time_to,omitempty"`
	NoteAR      string `json:"note_ar,omitempty"`
}

// SourceLinkChange attaches an existing source statement or passage to an existing
// claim as evidence.
type SourceLinkChange struct {
	ClaimID           string `json:"claim_id"`
	SourceStatementID string `json:"source_statement_id,omitempty"`
	SourcePassageID   string `json:"source_passage_id,omitempty"`
	Relation          string `json:"relation,omitempty"`
	NoteAR            string `json:"note_ar,omitempty"`
}

// ChangeSet is the reviewable description of the domain change an accepted suggestion
// approves. The target names which typed block carries the change and only that block
// may be present, so a change set is either valid and applied as structured rows or
// rejected as invalid before anything is written. The suggestion text stays the
// canonical record of what the contributor proposed; the change set records only what
// the reviewer decided to do about it.
type ChangeSet struct {
	Target       string              `json:"target"`
	Person       *PersonChange       `json:"person,omitempty"`
	Relationship *RelationshipChange `json:"relationship,omitempty"`
	Claim        *ClaimChange        `json:"claim,omitempty"`
	SourceLink   *SourceLinkChange   `json:"source_link,omitempty"`
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

// Review records a decision on a suggestion. An accepted suggestion that carries a
// change set has that change applied in the same transaction as the review, so the
// review can never be recorded without its change and the change can never exist
// without the review that authorized it. An acceptance without a change set records the
// proposer's text as an open question instead, which is a documented artifact rather
// than a status with nothing behind it. Nothing here publishes a tree, accepts a claim,
// or resolves a relationship: those stay explicit authorized actions.
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
	// The one validation point for a change set, before the transaction opens.
	var plan *changePlan
	if input.ChangeSet != nil {
		validated, err := validateChangeSet(*input.ChangeSet)
		if err != nil {
			return SuggestionView{}, err
		}
		plan = &validated
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
	if plan != nil {
		// canReview decides the review; applying a change set writes to the shared
		// research tables, which a tree review right does not reach. Applying a change
		// takes the same global write role the evidence service already requires, so a
		// collaborator on one tree cannot become a writer on the whole graph.
		globalWrite, err := canWriteGlobally(ctx, tx, reviewerUUID)
		if err != nil {
			return SuggestionView{}, err
		}
		if !globalWrite {
			return SuggestionView{}, ErrForbidden
		}
		applied, err := applyChangeSet(ctx, tx, *plan, reviewerUUID)
		if err != nil {
			return SuggestionView{}, err
		}
		reviewID, err := writeReview(ctx, tx, id, reviewerUUID, input)
		if err != nil {
			return SuggestionView{}, err
		}
		if err := recordChangeSet(ctx, tx, id, reviewID, *plan, applied, reviewerUUID); err != nil {
			return SuggestionView{}, err
		}
		if err := writeAudit(ctx, tx, &reviewerUUID, "suggestion_change_applied", "suggestion", id, map[string]any{"status": status, "textAr": textAR}, map[string]any{
			"decision": input.Decision, "reviewerId": reviewerUUID.String(), "target": plan.target,
			"change": plan.change, "resultType": applied.resultType, "resultId": applied.resultID.String(),
		}); err != nil {
			return SuggestionView{}, err
		}
		// A change set is the decision, so the suggestion points at no question.
		if err := settleReview(ctx, tx, id, reviewerUUID, input, nil, status); err != nil {
			return SuggestionView{}, err
		}
	} else {
		var questionID *uuid.UUID
		if input.Decision == "converted" || input.Decision == "accepted" {
			createdID, err := createSuggestionQuestion(ctx, tx, id, reviewerUUID, textAR, input.QuestionTitleAR, input.Decision)
			if err != nil {
				return SuggestionView{}, err
			}
			questionID = &createdID
		}
		if _, err := writeReview(ctx, tx, id, reviewerUUID, input); err != nil {
			return SuggestionView{}, err
		}
		if err := settleReview(ctx, tx, id, reviewerUUID, input, questionID, status); err != nil {
			return SuggestionView{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SuggestionView{}, err
	}
	return s.Get(ctx, id.String())
}

// settleReview stores the review decision on the suggestion and writes the audit event
// that closes the transaction. It runs after the change and the review row exist, so a
// failing audit write takes the whole decision with it.
func settleReview(ctx context.Context, tx pgx.Tx, id, reviewerUUID uuid.UUID, input ReviewInput, questionID *uuid.UUID, previousStatus string) error {
	if _, err := tx.Exec(ctx, `UPDATE suggestions SET status = $1, question_id = $2, updated_at = now() WHERE id = $3`, input.Decision, nullableUUID(questionID), id); err != nil {
		return err
	}
	questionIDValue := ""
	if questionID != nil {
		questionIDValue = questionID.String()
	}
	return writeAudit(ctx, tx, &reviewerUUID, "suggestion_reviewed", "suggestion", id, map[string]any{"status": previousStatus}, map[string]any{
		"decision": input.Decision, "noteAr": input.NoteAR, "questionId": questionIDValue,
	})
}

func writeReview(ctx context.Context, tx pgx.Tx, id, reviewerUUID uuid.UUID, input ReviewInput) (uuid.UUID, error) {
	reviewID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO suggestion_reviews (id, suggestion_id, reviewer_id, decision, note_ar) VALUES ($1, $2, $3, $4, NULLIF($5, ''))`, reviewID, id, reviewerUUID, input.Decision, input.NoteAR); err != nil {
		return uuid.Nil, err
	}
	return reviewID, nil
}

// createSuggestionQuestion records the proposer's text as an open question, so a review
// that changes nothing in the graph still leaves a traceable artifact instead of a bare
// status. The suggestion text is preserved verbatim as the question description.
func createSuggestionQuestion(ctx context.Context, tx pgx.Tx, id, reviewerUUID uuid.UUID, textAR, titleAR, decision string) (uuid.UUID, error) {
	createdID := uuid.New()
	title := strings.TrimSpace(titleAR)
	if title == "" {
		title = defaultQuestionTitle(textAR)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by)
		VALUES ($1, $2, $3, 'open', 'normal', $4)
	`, createdID, title, textAR, reviewerUUID); err != nil {
		return uuid.Nil, err
	}
	if err := writeAudit(ctx, tx, &reviewerUUID, "question_created_from_suggestion", "open_question", createdID, nil, map[string]any{
		"suggestionId": id.String(), "titleAr": title, "descriptionAr": textAR, "decision": decision,
	}); err != nil {
		return uuid.Nil, err
	}
	return createdID, nil
}

// recordChangeSet stores the approved change beside the review that authorized it, with
// the record the change produced, so the applied change can be read back directly.
func recordChangeSet(ctx context.Context, tx pgx.Tx, suggestionID, reviewID uuid.UUID, plan changePlan, applied appliedChange, reviewerID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO suggestion_change_sets (suggestion_id, review_id, target, change, result_type, result_id, applied_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, suggestionID, reviewID, plan.target, mustJSON(plan.change), applied.resultType, applied.resultID, reviewerID)
	return err
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

// canWriteGlobally reports whether the actor may write to the records a change set
// targets: person aliases, entity relationships, claims and claim evidence. The role set
// is the one evidence.AddEvidence and evidence.CreateClaim already require, so a change
// set can never open a write path the evidence service would refuse. canReview stays the
// gate for the decision itself.
func canWriteGlobally(ctx context.Context, q dbExecutor, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $1 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin'))
	`, actorID).Scan(&allowed)
	return allowed, err
}

// canManageClaim mirrors the evidence service rule for a claim: its creator, or a holder
// of a global write role, may attach evidence to it. A claim the actor may not manage
// answers exactly like a claim that does not exist, so the check cannot be used to probe
// which claims exist.
func canManageClaim(ctx context.Context, q dbExecutor, claimID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM claims c
			WHERE c.id = $1 AND (
				c.created_by = $2
				OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin'))
			)
		)
	`, claimID, actorID).Scan(&allowed)
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
	if input.ChangeSet != nil && input.Decision != "accepted" {
		// A change set only means something on an acceptance. A rejection or a conversion
		// carrying one is a review that contradicts itself.
		return ReviewInput{}, ErrValidation
	}
	return input, nil
}

// changePlan is a change set that passed validation: the typed blocks the target allows,
// with their identifiers parsed and their text normalized. The database is not consulted
// here, so a malformed change set is rejected before any transaction opens.
type changePlan struct {
	target       string
	change       ChangeSet
	person       *personPlan
	relationship *relationshipPlan
	claim        *claimPlan
	sourceLink   *sourceLinkPlan
}

type personPlan struct {
	personID  uuid.UUID
	nameAR    string
	aliasType string
}

type relationshipPlan struct {
	subjectType string
	subjectID   uuid.UUID
	objectType  string
	objectID    uuid.UUID
	predicate   string
	validFrom   pgtype.Date
	validTo     pgtype.Date
}

type claimPlan struct {
	subjectType string
	subjectID   uuid.UUID
	objectType  string
	objectID    uuid.UUID
	predicate   string
	placeArg    any
	timeFrom    pgtype.Date
	timeTo      pgtype.Date
	noteAR      string
}

type sourceLinkPlan struct {
	claimID      uuid.UUID
	statementArg any
	passageArg   any
	statementID  uuid.UUID
	passageID    uuid.UUID
	relation     string
	noteAR       string
}

// validateChangeSet accepts a change set only when the target is known, exactly the
// block that target names is present, and every field it carries is well typed. Anything
// else is a validation error, so an unstructured edit is never applied.
func validateChangeSet(change ChangeSet) (changePlan, error) {
	plan := changePlan{target: strings.TrimSpace(change.Target), change: change}
	switch plan.target {
	case ChangeTargetPerson:
		if change.Person == nil || change.Relationship != nil || change.Claim != nil || change.SourceLink != nil {
			return changePlan{}, ErrValidation
		}
		personID, err := parseChangeUUID(change.Person.PersonID)
		if err != nil {
			return changePlan{}, err
		}
		nameAR := strings.TrimSpace(change.Person.NameAR)
		aliasType := strings.TrimSpace(change.Person.AliasType)
		if aliasType == "" {
			aliasType = "alternative_name"
		}
		if nameAR == "" || len([]rune(nameAR)) > 300 || !contains(changeAliasTypes, aliasType) {
			return changePlan{}, ErrValidation
		}
		plan.person = &personPlan{personID: personID, nameAR: nameAR, aliasType: aliasType}
	case ChangeTargetRelationship:
		if change.Relationship == nil || change.Person != nil || change.Claim != nil || change.SourceLink != nil {
			return changePlan{}, ErrValidation
		}
		subjectType, subjectID, err := parseChangeEntity(change.Relationship.SubjectType, change.Relationship.SubjectID)
		if err != nil {
			return changePlan{}, err
		}
		objectType, objectID, err := parseChangeEntity(change.Relationship.ObjectType, change.Relationship.ObjectID)
		if err != nil {
			return changePlan{}, err
		}
		if subjectID == objectID {
			return changePlan{}, ErrValidation
		}
		predicate := strings.TrimSpace(change.Relationship.Predicate)
		validFrom, validTo, err := parseChangeDateRange(change.Relationship.ValidFrom, change.Relationship.ValidTo)
		if err != nil {
			return changePlan{}, err
		}
		if !contains(changePredicates, predicate) {
			return changePlan{}, ErrValidation
		}
		plan.relationship = &relationshipPlan{subjectType: subjectType, subjectID: subjectID, objectType: objectType, objectID: objectID, predicate: predicate, validFrom: validFrom, validTo: validTo}
	case ChangeTargetClaim:
		if change.Claim == nil || change.Person != nil || change.Relationship != nil || change.SourceLink != nil {
			return changePlan{}, ErrValidation
		}
		subjectType, subjectID, err := parseChangeEntity(change.Claim.SubjectType, change.Claim.SubjectID)
		if err != nil {
			return changePlan{}, err
		}
		objectType, objectID, err := parseChangeEntity(change.Claim.ObjectType, change.Claim.ObjectID)
		if err != nil {
			return changePlan{}, err
		}
		if subjectID == objectID {
			return changePlan{}, ErrValidation
		}
		predicate := strings.TrimSpace(change.Claim.Predicate)
		if !contains(changePredicates, predicate) {
			return changePlan{}, ErrValidation
		}
		placeArg, err := parseOptionalChangeUUID(change.Claim.PlaceID)
		if err != nil {
			return changePlan{}, err
		}
		timeFrom, timeTo, err := parseChangeDateRange(change.Claim.TimeFrom, change.Claim.TimeTo)
		if err != nil {
			return changePlan{}, err
		}
		noteAR := strings.TrimSpace(change.Claim.NoteAR)
		if len([]rune(noteAR)) > 5000 {
			return changePlan{}, ErrValidation
		}
		plan.claim = &claimPlan{subjectType: subjectType, subjectID: subjectID, objectType: objectType, objectID: objectID, predicate: predicate, placeArg: placeArg, timeFrom: timeFrom, timeTo: timeTo, noteAR: noteAR}
	case ChangeTargetSourceLink:
		if change.SourceLink == nil || change.Person != nil || change.Relationship != nil || change.Claim != nil {
			return changePlan{}, ErrValidation
		}
		claimID, err := parseChangeUUID(change.SourceLink.ClaimID)
		if err != nil {
			return changePlan{}, err
		}
		link := &sourceLinkPlan{claimID: claimID, noteAR: strings.TrimSpace(change.SourceLink.NoteAR)}
		if len([]rune(link.noteAR)) > 5000 {
			return changePlan{}, ErrValidation
		}
		statementID := strings.TrimSpace(change.SourceLink.SourceStatementID)
		passageID := strings.TrimSpace(change.SourceLink.SourcePassageID)
		if (statementID == "") == (passageID == "") {
			return changePlan{}, ErrValidation
		}
		if statementID != "" {
			parsed, err := parseChangeUUID(statementID)
			if err != nil {
				return changePlan{}, err
			}
			link.statementArg, link.statementID = parsed, parsed
		}
		if passageID != "" {
			parsed, err := parseChangeUUID(passageID)
			if err != nil {
				return changePlan{}, err
			}
			link.passageArg, link.passageID = parsed, parsed
		}
		link.relation = strings.TrimSpace(change.SourceLink.Relation)
		if link.relation == "" {
			link.relation = "supports"
		}
		if !contains(changeRelations, link.relation) {
			return changePlan{}, ErrValidation
		}
		plan.sourceLink = link
	default:
		return changePlan{}, ErrValidation
	}
	return plan, nil
}

var (
	changePredicates   = []string{"parent_of", "father_of", "mother_of", "spouse_of", "sibling_of", "son_of", "daughter_of", "brother_of", "sister_of", "born_in", "died_in", "resided_in"}
	changeEntityTypes  = []string{"person", "family", "branch", "tribe", "place"}
	changeAliasTypes   = []string{"alternative_name", "kunyah", "laqab", "nisbah", "source_spelling"}
	changeRelations    = []string{"supports", "contextualizes", "contradicts", "refutes"}
	changeEntityTables = map[string]string{"person": "people", "family": "families", "branch": "branches", "tribe": "tribes", "place": "places"}
)

func parseChangeUUID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, ErrValidation
	}
	return parsed, nil
}

func parseOptionalChangeUUID(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseChangeUUID(value)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func parseChangeEntity(entityType, entityID string) (string, uuid.UUID, error) {
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	if entityType == "" {
		entityType = "person"
	}
	if !contains(changeEntityTypes, entityType) {
		return "", uuid.Nil, ErrValidation
	}
	parsed, err := parseChangeUUID(entityID)
	if err != nil {
		return "", uuid.Nil, err
	}
	return entityType, parsed, nil
}

func parseChangeDateRange(fromValue, toValue string) (pgtype.Date, pgtype.Date, error) {
	parse := func(value string) (pgtype.Date, error) {
		value = strings.TrimSpace(value)
		if value == "" {
			return pgtype.Date{}, nil
		}
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return pgtype.Date{}, ErrValidation
		}
		return pgtype.Date{Time: parsed, Valid: true}, nil
	}
	from, err := parse(fromValue)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, err
	}
	to, err := parse(toValue)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, err
	}
	if from.Valid && to.Valid && to.Time.Before(from.Time) {
		return pgtype.Date{}, pgtype.Date{}, ErrValidation
	}
	return from, to, nil
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

// appliedChange names the record a change set produced, so the audit event and the
// stored change set can both point at it.
type appliedChange struct {
	resultType string
	resultID   uuid.UUID
}

// applyChangeSet writes the approved change. The target decides which typed rows are
// written: an alias for a person, an unresolved relationship between existing entities,
// an unresolved claim with its first version snapshot, or evidence linking a source to a
// claim. A target never publishes a tree, accepts a claim or resolves a relationship,
// and a change that names a record which does not exist is refused before any write.
func applyChangeSet(ctx context.Context, tx pgx.Tx, plan changePlan, reviewerID uuid.UUID) (appliedChange, error) {
	switch plan.target {
	case ChangeTargetPerson:
		return applyPersonChange(ctx, tx, *plan.person)
	case ChangeTargetRelationship:
		return applyRelationshipChange(ctx, tx, *plan.relationship, reviewerID)
	case ChangeTargetClaim:
		return applyClaimChange(ctx, tx, *plan.claim, reviewerID)
	case ChangeTargetSourceLink:
		return applySourceLinkChange(ctx, tx, *plan.sourceLink, reviewerID)
	default:
		return appliedChange{}, ErrValidation
	}
}

func applyPersonChange(ctx context.Context, tx pgx.Tx, plan personPlan) (appliedChange, error) {
	if err := requireEntity(ctx, tx, "person", plan.personID); err != nil {
		return appliedChange{}, err
	}
	aliasID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO person_aliases (id, person_id, value_ar, normalized_value_ar, alias_type)
		VALUES ($1, $2, $3, $4, $5)
	`, aliasID, plan.personID, plan.nameAR, identity.NormalizeArabicName(plan.nameAR), plan.aliasType); err != nil {
		return appliedChange{}, err
	}
	return appliedChange{resultType: "person_alias", resultID: aliasID}, nil
}

func applyRelationshipChange(ctx context.Context, tx pgx.Tx, plan relationshipPlan, reviewerID uuid.UUID) (appliedChange, error) {
	if err := requireEntity(ctx, tx, plan.subjectType, plan.subjectID); err != nil {
		return appliedChange{}, err
	}
	if err := requireEntity(ctx, tx, plan.objectType, plan.objectID); err != nil {
		return appliedChange{}, err
	}
	relationshipID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO entity_relationships (id, subject_type, subject_id, predicate, object_type, object_id, valid_from, valid_to, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'unresolved', $9)
	`, relationshipID, plan.subjectType, plan.subjectID, plan.predicate, plan.objectType, plan.objectID, plan.validFrom, plan.validTo, reviewerID); err != nil {
		return appliedChange{}, err
	}
	return appliedChange{resultType: "entity_relationship", resultID: relationshipID}, nil
}

func applyClaimChange(ctx context.Context, tx pgx.Tx, plan claimPlan, reviewerID uuid.UUID) (appliedChange, error) {
	if err := requireEntity(ctx, tx, plan.subjectType, plan.subjectID); err != nil {
		return appliedChange{}, err
	}
	if err := requireEntity(ctx, tx, plan.objectType, plan.objectID); err != nil {
		return appliedChange{}, err
	}
	if plan.placeArg != nil {
		if err := requireEntity(ctx, tx, "place", plan.placeArg.(uuid.UUID)); err != nil {
			return appliedChange{}, err
		}
	}
	claimID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, time_from, time_to, place_id, status, notes_ar, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'unresolved', NULLIF($10, ''), $11)
	`, claimID, plan.subjectType, plan.subjectID, plan.predicate, plan.objectType, plan.objectID, plan.timeFrom, plan.timeTo, plan.placeArg, plan.noteAR, reviewerID); err != nil {
		return appliedChange{}, err
	}
	snapshot := mustJSON(map[string]any{"subject_type": plan.subjectType, "subject_id": plan.subjectID.String(), "predicate": plan.predicate, "object_type": plan.objectType, "object_id": plan.objectID.String(), "status": "unresolved"})
	if _, err := tx.Exec(ctx, `
		INSERT INTO claim_versions (claim_id, version_number, snapshot, change_reason_ar, created_by)
		VALUES ($1, 1, $2, NULLIF($3, ''), $4)
	`, claimID, snapshot, plan.noteAR, reviewerID); err != nil {
		return appliedChange{}, err
	}
	return appliedChange{resultType: "claim", resultID: claimID}, nil
}

// applySourceLinkChange attaches a source statement or passage to a claim as evidence.
// The claim must be one the reviewer could attach evidence to through the evidence
// service, and the check runs before the claim is even looked up, so a refused link is a
// refusal to manage rather than a hint about which claims exist. The link travels on the
// caller's executor so the same query can run inside the suggestion review transaction or
// on a pool.
func applySourceLinkChange(ctx context.Context, q dbExecutor, plan sourceLinkPlan, reviewerID uuid.UUID) (appliedChange, error) {
	allowed, err := canManageClaim(ctx, q, plan.claimID, reviewerID)
	if err != nil {
		return appliedChange{}, err
	}
	if !allowed {
		return appliedChange{}, ErrForbidden
	}
	var statementSource, passageSource uuid.UUID
	if plan.statementArg != nil {
		if err := q.QueryRow(ctx, `SELECT source_id FROM source_statements WHERE id = $1`, plan.statementID).Scan(&statementSource); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return appliedChange{}, ErrNotFound
			}
			return appliedChange{}, err
		}
	}
	if plan.passageArg != nil {
		if err := q.QueryRow(ctx, `SELECT source_id FROM source_passages WHERE id = $1`, plan.passageID).Scan(&passageSource); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return appliedChange{}, ErrNotFound
			}
			return appliedChange{}, err
		}
	}
	if plan.statementArg != nil && plan.passageArg != nil && statementSource != passageSource {
		return appliedChange{}, ErrValidation
	}
	evidenceID := uuid.New()
	if plan.relation == "contradicts" || plan.relation == "refutes" {
		if _, err := q.Exec(ctx, `
			INSERT INTO claim_counter_evidence (id, claim_id, source_statement_id, source_passage_id, note_ar, created_by)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		`, evidenceID, plan.claimID, plan.statementArg, plan.passageArg, plan.noteAR, reviewerID); err != nil {
			return appliedChange{}, err
		}
		return appliedChange{resultType: "claim_counter_evidence", resultID: evidenceID}, nil
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO claim_evidence (id, claim_id, source_statement_id, source_passage_id, evidence_note_ar, relation, created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)
	`, evidenceID, plan.claimID, plan.statementArg, plan.passageArg, plan.noteAR, plan.relation, reviewerID); err != nil {
		return appliedChange{}, err
	}
	return appliedChange{resultType: "claim_evidence", resultID: evidenceID}, nil
}

func requireEntity(ctx context.Context, q dbExecutor, entityType string, entityID uuid.UUID) error {
	table, known := changeEntityTables[entityType]
	if !known {
		return ErrValidation
	}
	var exists bool
	if err := q.QueryRow(ctx, fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)", table), entityID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
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
