package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/claims"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("evidence database is unavailable")
	ErrNotFound            = errors.New("evidence resource not found")
	ErrForbidden           = errors.New("evidence access is forbidden")
	ErrValidation          = errors.New("evidence input is invalid")
	ErrConflict            = errors.New("evidence state conflict")
)

type CreateSourceInput struct {
	TitleAR             string `json:"title_ar"`
	AuthorAR            string `json:"author_ar"`
	SourceType          string `json:"source_type"`
	PublicationDateFrom string `json:"publication_date_from"`
	PublicationDateTo   string `json:"publication_date_to"`
	EditionAR           string `json:"edition_ar"`
	CitationAR          string `json:"citation_ar"`
	LocationAR          string `json:"location_ar"`
	DependencyStatus    string `json:"dependency_status"`
}

type SourceView struct {
	ID                  string    `json:"id"`
	TitleAR             string    `json:"titleAr"`
	AuthorAR            string    `json:"authorAr,omitempty"`
	SourceType          string    `json:"sourceType"`
	PublicationDateFrom string    `json:"publicationDateFrom,omitempty"`
	PublicationDateTo   string    `json:"publicationDateTo,omitempty"`
	EditionAR           string    `json:"editionAr,omitempty"`
	CitationAR          string    `json:"citationAr,omitempty"`
	LocationAR          string    `json:"locationAr,omitempty"`
	DependencyStatus    string    `json:"dependencyStatus"`
	Visibility          string    `json:"visibility"`
	CreatedBy           string    `json:"createdBy"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	PassageCount        int       `json:"passageCount"`
	StatementCount      int       `json:"statementCount"`
}

type SourcePassageInput struct {
	SequenceNumber int    `json:"sequence_number"`
	PageNumber     *int   `json:"page_number"`
	LocatorAR      string `json:"locator_ar"`
	TextAR         string `json:"text_ar"`
}

type SourcePassageView struct {
	ID             string    `json:"id"`
	SourceID       string    `json:"sourceId"`
	SourceFileID   string    `json:"sourceFileId,omitempty"`
	SequenceNumber int       `json:"sequenceNumber"`
	PageNumber     *int      `json:"pageNumber,omitempty"`
	LocatorAR      string    `json:"locatorAr,omitempty"`
	StartOffset    *int      `json:"startOffset,omitempty"`
	EndOffset      *int      `json:"endOffset,omitempty"`
	TextAR         string    `json:"textAr"`
	CreatedAt      time.Time `json:"createdAt"`
}

type SourceStatementInput struct {
	SourcePassageID string `json:"source_passage_id"`
	StatementTextAR string `json:"statement_text_ar"`
	LocatorAR       string `json:"locator_ar"`
	ReviewStatus    string `json:"review_status"`
}

type SourceStatementView struct {
	ID               string    `json:"id"`
	SourceID         string    `json:"sourceId"`
	SourceFileID     string    `json:"sourceFileId,omitempty"`
	SourcePassageID  string    `json:"sourcePassageId,omitempty"`
	StatementTextAR  string    `json:"statementTextAr"`
	LocatorAR        string    `json:"locatorAr,omitempty"`
	ExtractionMethod string    `json:"extractionMethod"`
	ReviewStatus     string    `json:"reviewStatus"`
	CreatedBy        string    `json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
}

type SourceDetail struct {
	Source            SourceView              `json:"source"`
	Passages          []SourcePassageView     `json:"passages"`
	Statements        []SourceStatementView   `json:"statements"`
	Dependencies      []SourceDependencyView  `json:"dependencies"`
	DependencySummary SourceDependencySummary `json:"dependencySummary"`
}

type CreateClaimInput struct {
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	Predicate   string `json:"predicate"`
	ObjectType  string `json:"object_type"`
	ObjectID    string `json:"object_id"`
	TimeFrom    string `json:"time_from"`
	TimeTo      string `json:"time_to"`
	PlaceID     string `json:"place_id"`
	Status      string `json:"status"`
	NotesAR     string `json:"notes_ar"`
}

type AddEvidenceInput struct {
	SourceStatementID string `json:"source_statement_id"`
	SourcePassageID   string `json:"source_passage_id"`
	EvidenceNoteAR    string `json:"evidence_note_ar"`
	Relation          string `json:"relation"`
}

type EvidenceView struct {
	ID                string `json:"id"`
	Relation          string `json:"relation"`
	EvidenceNoteAR    string `json:"evidenceNoteAr,omitempty"`
	SourceID          string `json:"sourceId,omitempty"`
	SourceStatementID string `json:"sourceStatementId,omitempty"`
	SourcePassageID   string `json:"sourcePassageId,omitempty"`
	SourceTitleAR     string `json:"sourceTitleAr,omitempty"`
	DependencyStatus  string `json:"dependencyStatus,omitempty"`
	StatementTextAR   string `json:"statementTextAr,omitempty"`
	PassageTextAR     string `json:"passageTextAr,omitempty"`
}

type ClaimView struct {
	ID          string         `json:"id"`
	SubjectType string         `json:"subjectType"`
	SubjectID   string         `json:"subjectId"`
	Predicate   string         `json:"predicate"`
	ObjectType  string         `json:"objectType"`
	ObjectID    string         `json:"objectId"`
	TimeFrom    string         `json:"timeFrom,omitempty"`
	TimeTo      string         `json:"timeTo,omitempty"`
	PlaceID     string         `json:"placeId,omitempty"`
	Status      string         `json:"status"`
	NotesAR     string         `json:"notesAr,omitempty"`
	CreatedBy   string         `json:"createdBy"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Evidence    []EvidenceView `json:"evidence"`
}

type ClaimSummary struct {
	ID            string    `json:"id"`
	SubjectType   string    `json:"subjectType"`
	Predicate     string    `json:"predicate"`
	ObjectType    string    `json:"objectType"`
	Status        string    `json:"status"`
	EvidenceCount int       `json:"evidenceCount"`
	CreatedAt     time.Time `json:"createdAt"`
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

func (s *Service) ListSources(ctx context.Context) ([]SourceView, error) {
	return s.listSources(ctx, nil)
}

func (s *Service) ListSourcesForActor(ctx context.Context, actorID string) ([]SourceView, error) {
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return nil, ErrForbidden
	}
	return s.listSources(ctx, &actorUUID)
}

func (s *Service) listSources(ctx context.Context, actorID *uuid.UUID) ([]SourceView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	query := sourceListQuery + ` WHERE s.visibility = 'public'`
	args := []any{}
	if actorID != nil {
		query += ` OR s.created_by = $1 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $1 AND ur.role IN ('researcher', 'moderator', 'admin'))`
		args = append(args, *actorID)
	}
	query += ` ORDER BY s.updated_at DESC, s.id`
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SourceView, 0)
	for rows.Next() {
		item, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetSource(ctx context.Context, sourceID string) (SourceDetail, error) {
	return s.getSource(ctx, sourceID, nil)
}

func (s *Service) GetSourceForActor(ctx context.Context, sourceID, actorID string) (SourceDetail, error) {
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return SourceDetail{}, ErrForbidden
	}
	return s.getSource(ctx, sourceID, &actorUUID)
}

func (s *Service) getSource(ctx context.Context, sourceID string, actorID *uuid.UUID) (SourceDetail, error) {
	if err := s.ready(); err != nil {
		return SourceDetail{}, err
	}
	sourceUUID, err := uuid.Parse(sourceID)
	if err != nil {
		return SourceDetail{}, ErrNotFound
	}
	var source SourceView
	if actorID == nil {
		source, err = s.sourceByID(ctx, s.Pool, sourceUUID)
	} else {
		source, err = s.sourceByIDAny(ctx, s.Pool, sourceUUID)
		if err == nil && source.Visibility != "public" {
			allowed, accessErr := canViewSource(ctx, s.Pool, sourceUUID, *actorID)
			if accessErr != nil {
				return SourceDetail{}, accessErr
			}
			if !allowed {
				return SourceDetail{}, ErrForbidden
			}
		}
	}
	if err != nil {
		return SourceDetail{}, err
	}
	passages, err := s.passages(ctx, s.Pool, sourceUUID)
	if err != nil {
		return SourceDetail{}, err
	}
	statements, err := s.statements(ctx, s.Pool, sourceUUID)
	if err != nil {
		return SourceDetail{}, err
	}
	dependencyGraph, err := s.loadSourceDependencyGraph(ctx, s.Pool, sourceUUID, actorID)
	if err != nil {
		return SourceDetail{}, err
	}
	return SourceDetail{Source: source, Passages: passages, Statements: statements, Dependencies: dependencyGraph.Items, DependencySummary: dependencyGraph.Summary}, nil
}

func (s *Service) CreateSource(ctx context.Context, actorID string, input CreateSourceInput) (SourceDetail, error) {
	if err := s.ready(); err != nil {
		return SourceDetail{}, err
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return SourceDetail{}, ErrForbidden
	}
	input, err = validateSourceInput(input)
	if err != nil {
		return SourceDetail{}, err
	}
	from, to, err := parseDateRange(input.PublicationDateFrom, input.PublicationDateTo)
	if err != nil {
		return SourceDetail{}, err
	}
	sourceID := uuid.New()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceDetail{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO sources
			(id, title_ar, author_ar, source_type, publication_date_from, publication_date_to, edition_ar, citation_ar, location_ar, dependency_status, visibility, created_by)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10, 'private', $11)
	`, sourceID, input.TitleAR, input.AuthorAR, input.SourceType, from, to, input.EditionAR, input.CitationAR, input.LocationAR, input.DependencyStatus, actorUUID); err != nil {
		return SourceDetail{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "source_created", "source", sourceID, nil, input); err != nil {
		return SourceDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceDetail{}, err
	}
	return s.GetSourceForActor(ctx, sourceID.String(), actorID)
}

func (s *Service) CreatePassage(ctx context.Context, sourceID, actorID string, input SourcePassageInput) (SourceDetail, error) {
	if err := s.ready(); err != nil {
		return SourceDetail{}, err
	}
	sourceUUID, actorUUID, err := parseActorResource(sourceID, actorID)
	if err != nil {
		return SourceDetail{}, err
	}
	input, err = validatePassageInput(input)
	if err != nil {
		return SourceDetail{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceDetail{}, err
	}
	defer tx.Rollback(ctx)
	allowed, err := canManageSource(ctx, tx, sourceUUID, actorUUID)
	if err != nil {
		return SourceDetail{}, err
	}
	if !allowed {
		return SourceDetail{}, ErrForbidden
	}
	var lockedSourceID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM sources WHERE id = $1 FOR UPDATE`, sourceUUID).Scan(&lockedSourceID); err != nil {
		return SourceDetail{}, err
	}
	sequence := input.SequenceNumber
	if sequence == 0 {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM source_passages WHERE source_id = $1`, sourceUUID).Scan(&sequence); err != nil {
			return SourceDetail{}, err
		}
	}
	passageID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)
	`, passageID, sourceUUID, sequence, input.PageNumber, input.LocatorAR, input.TextAR, identity.NormalizeArabicName(input.TextAR)); err != nil {
		return SourceDetail{}, mapConflict(err)
	}
	if err := writeAudit(ctx, tx, actorUUID, "source_passage_added", "source_passage", passageID, nil, map[string]any{"source_id": sourceUUID.String(), "sequence_number": sequence}); err != nil {
		return SourceDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceDetail{}, err
	}
	return s.GetSourceForActor(ctx, sourceUUID.String(), actorID)
}

// CreateStatement records a source statement. The statement row and its audit event
// share one transaction, so a failed audit never leaves a statement with no history.
func (s *Service) CreateStatement(ctx context.Context, sourceID, actorID string, input SourceStatementInput) (SourceDetail, error) {
	if err := s.ready(); err != nil {
		return SourceDetail{}, err
	}
	sourceUUID, actorUUID, err := parseActorResource(sourceID, actorID)
	if err != nil {
		return SourceDetail{}, err
	}
	input, err = validateStatementInput(input)
	if err != nil {
		return SourceDetail{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceDetail{}, err
	}
	defer tx.Rollback(ctx)
	allowed, err := canManageSource(ctx, tx, sourceUUID, actorUUID)
	if err != nil {
		return SourceDetail{}, err
	}
	if !allowed {
		return SourceDetail{}, ErrForbidden
	}
	var passageArg any
	if input.SourcePassageID != "" {
		passageUUID, err := uuid.Parse(input.SourcePassageID)
		if err != nil {
			return SourceDetail{}, ErrValidation
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM source_passages WHERE id = $1 AND source_id = $2)`, passageUUID, sourceUUID).Scan(&exists); err != nil {
			return SourceDetail{}, err
		}
		if !exists {
			return SourceDetail{}, ErrNotFound
		}
		passageArg = passageUUID
	}
	statementID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_statements
			(id, source_id, source_passage_id, statement_text_ar, locator_ar, extraction_method, review_status, created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), 'manual', $6, $7)
	`, statementID, sourceUUID, passageArg, input.StatementTextAR, input.LocatorAR, input.ReviewStatus, actorUUID); err != nil {
		return SourceDetail{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "source_statement_recorded", "source_statement", statementID, nil, input); err != nil {
		return SourceDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceDetail{}, err
	}
	return s.GetSourceForActor(ctx, sourceUUID.String(), actorID)
}

// ListClaims lists claims the actor may read. The same claim predicate that guards a
// direct read guards the list, so the two can never disagree.
func (s *Service) ListClaims(ctx context.Context, actorID string) ([]ClaimSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return nil, err
	}
	params := visibility.NewParams()
	claimPredicate := policy.ClaimPredicate(params, "c.id")
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id, c.subject_type, c.predicate, c.object_type, c.status, c.created_at,
		       (SELECT count(*) FROM claim_evidence ce WHERE ce.claim_id = c.id)
		     + (SELECT count(*) FROM claim_counter_evidence cce WHERE cce.claim_id = c.id)
		FROM claims c
		WHERE `+claimPredicate+`
		ORDER BY c.created_at DESC
		LIMIT 100
	`, params.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ClaimSummary, 0)
	for rows.Next() {
		var item ClaimSummary
		if err := rows.Scan(&item.ID, &item.SubjectType, &item.Predicate, &item.ObjectType, &item.Status, &item.CreatedAt, &item.EvidenceCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetClaim returns a claim the actor may read. A claim the actor cannot read answers
// exactly like a missing claim, so the endpoint is not an existence oracle. The
// evidence list keeps the distinction between source statements and research claims:
// attached evidence is only rendered when its source is visible to this actor.
func (s *Service) GetClaim(ctx context.Context, claimID, actorID string) (ClaimView, error) {
	if err := s.ready(); err != nil {
		return ClaimView{}, err
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return ClaimView{}, err
	}
	claimUUID, err := uuid.Parse(strings.TrimSpace(claimID))
	if err != nil {
		return ClaimView{}, ErrNotFound
	}
	access, err := policy.Claim(ctx, s.Pool, claimUUID)
	if err != nil {
		return ClaimView{}, err
	}
	if !access.Allowed() {
		return ClaimView{}, ErrNotFound
	}
	var item ClaimView
	var subjectID, objectID, placeID, createdBy pgtype.UUID
	var timeFrom, timeTo pgtype.Date
	var notes pgtype.Text
	if err := s.Pool.QueryRow(ctx, `
		SELECT id, subject_type, subject_id, predicate, object_type, object_id, time_from, time_to, place_id, status, notes_ar, created_by, created_at, updated_at
		FROM claims WHERE id = $1
	`, claimUUID).Scan(&item.ID, &item.SubjectType, &subjectID, &item.Predicate, &item.ObjectType, &objectID, &timeFrom, &timeTo, &placeID, &item.Status, &notes, &createdBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ClaimView{}, ErrNotFound
		}
		return ClaimView{}, err
	}
	item.SubjectID = uuidString(subjectID)
	item.ObjectID = uuidString(objectID)
	item.PlaceID = uuidString(placeID)
	item.NotesAR = textValue(notes)
	item.TimeFrom = dateText(timeFrom)
	item.TimeTo = dateText(timeTo)
	item.CreatedBy = uuidString(createdBy)
	item.Evidence, err = s.claimEvidence(ctx, policy, claimUUID)
	if err != nil {
		return ClaimView{}, err
	}
	return item, nil
}

// CreateClaim writes a claim, its first version snapshot, and the audit event as one
// unit. A claim without its version row, or a claim whose audit never landed, is a
// broken provenance record, so the three writes commit or vanish together.
func (s *Service) CreateClaim(ctx context.Context, actorID string, input CreateClaimInput) (ClaimView, error) {
	if err := s.ready(); err != nil {
		return ClaimView{}, err
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return ClaimView{}, ErrForbidden
	}
	input, err = validateClaimInput(input)
	if err != nil {
		return ClaimView{}, err
	}
	subjectID, _ := uuid.Parse(input.SubjectID)
	objectID, _ := uuid.Parse(input.ObjectID)
	placeArg, err := optionalUUID(input.PlaceID)
	if err != nil {
		return ClaimView{}, err
	}
	timeFrom, timeTo, err := parseDateRange(input.TimeFrom, input.TimeTo)
	if err != nil {
		return ClaimView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ClaimView{}, err
	}
	defer tx.Rollback(ctx)
	claimID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO claims
			(id, subject_type, subject_id, predicate, object_type, object_id, time_from, time_to, place_id, status, notes_ar, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12)
	`, claimID, input.SubjectType, subjectID, input.Predicate, input.ObjectType, objectID, timeFrom, timeTo, placeArg, input.Status, input.NotesAR, actorUUID); err != nil {
		return ClaimView{}, err
	}
	snapshot := map[string]any{"subject_type": input.SubjectType, "subject_id": input.SubjectID, "predicate": input.Predicate, "object_type": input.ObjectType, "object_id": input.ObjectID, "status": input.Status}
	if _, err := tx.Exec(ctx, `
		INSERT INTO claim_versions (claim_id, version_number, snapshot, change_reason_ar, created_by)
		VALUES ($1, 1, $2, NULLIF($3, ''), $4)
	`, claimID, mustJSON(snapshot), input.NotesAR, actorUUID); err != nil {
		return ClaimView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "claim_created", "claim", claimID, nil, snapshot); err != nil {
		return ClaimView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ClaimView{}, err
	}
	return s.GetClaim(ctx, claimID.String(), actorID)
}

// AddEvidence links a source statement or passage to a claim. The link row and its
// audit event commit together, so a claim never shows evidence that the audit log
// does not explain.
func (s *Service) AddEvidence(ctx context.Context, claimID, actorID string, input AddEvidenceInput) (ClaimView, error) {
	if err := s.ready(); err != nil {
		return ClaimView{}, err
	}
	claimUUID, actorUUID, err := parseActorResource(claimID, actorID)
	if err != nil {
		return ClaimView{}, err
	}
	input, err = validateEvidenceInput(input)
	if err != nil {
		return ClaimView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ClaimView{}, err
	}
	defer tx.Rollback(ctx)
	allowed, err := canManageClaim(ctx, tx, claimUUID, actorUUID)
	if err != nil {
		return ClaimView{}, err
	}
	if !allowed {
		return ClaimView{}, ErrForbidden
	}
	var statementArg, passageArg any
	var statementSource, passageSource uuid.UUID
	if input.SourceStatementID != "" {
		statementUUID, err := uuid.Parse(input.SourceStatementID)
		if err != nil {
			return ClaimView{}, ErrValidation
		}
		if err := tx.QueryRow(ctx, `SELECT source_id FROM source_statements WHERE id = $1`, statementUUID).Scan(&statementSource); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ClaimView{}, ErrNotFound
			}
			return ClaimView{}, err
		}
		statementArg = statementUUID
	}
	if input.SourcePassageID != "" {
		passageUUID, err := uuid.Parse(input.SourcePassageID)
		if err != nil {
			return ClaimView{}, ErrValidation
		}
		if err := tx.QueryRow(ctx, `SELECT source_id FROM source_passages WHERE id = $1`, passageUUID).Scan(&passageSource); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ClaimView{}, ErrNotFound
			}
			return ClaimView{}, err
		}
		passageArg = passageUUID
	}
	if statementArg != nil && passageArg != nil && statementSource != passageSource {
		return ClaimView{}, ErrValidation
	}
	evidenceID := uuid.New()
	if input.Relation == "contradicts" || input.Relation == "refutes" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO claim_counter_evidence (id, claim_id, source_statement_id, source_passage_id, note_ar, created_by)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		`, evidenceID, claimUUID, statementArg, passageArg, input.EvidenceNoteAR, actorUUID); err != nil {
			return ClaimView{}, err
		}
	} else if _, err := tx.Exec(ctx, `
		INSERT INTO claim_evidence (id, claim_id, source_statement_id, source_passage_id, evidence_note_ar, relation, created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)
	`, evidenceID, claimUUID, statementArg, passageArg, input.EvidenceNoteAR, input.Relation, actorUUID); err != nil {
		return ClaimView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "claim_evidence_linked", "claim", claimUUID, nil, input); err != nil {
		return ClaimView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ClaimView{}, err
	}
	return s.GetClaim(ctx, claimUUID.String(), actorID)
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

const sourceListQuery = `
	SELECT s.id, s.title_ar, s.author_ar, s.source_type, s.publication_date_from, s.publication_date_to,
	       s.edition_ar, s.citation_ar, s.location_ar,
	       CASE
	         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'confirmed') THEN 'derived'
	         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'needs_review') THEN 'likely_dependent'
	         ELSE s.dependency_status
	       END,
	       s.visibility, s.created_by, s.created_at, s.updated_at,
	       (SELECT count(*) FROM source_passages sp WHERE sp.source_id = s.id),
	       (SELECT count(*) FROM source_statements ss WHERE ss.source_id = s.id)
	FROM sources s`

func (s *Service) sourceByID(ctx context.Context, q dbExecutor, sourceID uuid.UUID) (SourceView, error) {
	return s.sourceByIDWhere(ctx, q, sourceID, true)
}

func (s *Service) sourceByIDAny(ctx context.Context, q dbExecutor, sourceID uuid.UUID) (SourceView, error) {
	return s.sourceByIDWhere(ctx, q, sourceID, false)
}

func (s *Service) sourceByIDWhere(ctx context.Context, q dbExecutor, sourceID uuid.UUID, publicOnly bool) (SourceView, error) {
	query := sourceListQuery + ` WHERE s.id = $1`
	if publicOnly {
		query += ` AND s.visibility = 'public'`
	}
	item, err := scanSource(q.QueryRow(ctx, query, sourceID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SourceView{}, ErrNotFound
		}
		return SourceView{}, err
	}
	return item, nil
}

func scanSource(row pgx.Row) (SourceView, error) {
	var item SourceView
	var author, edition, citation, location pgtype.Text
	var from, to pgtype.Date
	var createdBy pgtype.UUID
	var createdAt, updatedAt time.Time
	if err := row.Scan(&item.ID, &item.TitleAR, &author, &item.SourceType, &from, &to, &edition, &citation, &location, &item.DependencyStatus, &item.Visibility, &createdBy, &createdAt, &updatedAt, &item.PassageCount, &item.StatementCount); err != nil {
		return SourceView{}, err
	}
	item.AuthorAR = textValue(author)
	item.PublicationDateFrom = dateText(from)
	item.PublicationDateTo = dateText(to)
	item.EditionAR = textValue(edition)
	item.CitationAR = textValue(citation)
	item.LocationAR = textValue(location)
	item.CreatedBy = uuidString(createdBy)
	item.CreatedAt = createdAt
	item.UpdatedAt = updatedAt
	return item, nil
}

func (s *Service) passages(ctx context.Context, q dbExecutor, sourceID uuid.UUID) ([]SourcePassageView, error) {
	rows, err := q.Query(ctx, `
		SELECT id, source_id, source_file_id, sequence_number, page_number, locator_ar, start_offset, end_offset, text_ar, created_at
		FROM source_passages WHERE source_id = $1 ORDER BY sequence_number
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SourcePassageView, 0)
	for rows.Next() {
		var item SourcePassageView
		var id, source, file pgtype.UUID
		var page, startOffset, endOffset pgtype.Int4
		var locator pgtype.Text
		if err := rows.Scan(&id, &source, &file, &item.SequenceNumber, &page, &locator, &startOffset, &endOffset, &item.TextAR, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.SourceID = uuidString(source)
		item.SourceFileID = uuidString(file)
		if page.Valid {
			value := int(page.Int32)
			item.PageNumber = &value
		}
		if startOffset.Valid {
			value := int(startOffset.Int32)
			item.StartOffset = &value
		}
		if endOffset.Valid {
			value := int(endOffset.Int32)
			item.EndOffset = &value
		}
		item.LocatorAR = textValue(locator)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) statements(ctx context.Context, q dbExecutor, sourceID uuid.UUID) ([]SourceStatementView, error) {
	rows, err := q.Query(ctx, `
		SELECT id, source_id, source_file_id, source_passage_id, statement_text_ar, locator_ar, extraction_method, review_status, created_by, created_at
		FROM source_statements WHERE source_id = $1 ORDER BY created_at
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SourceStatementView, 0)
	for rows.Next() {
		var item SourceStatementView
		var id, source, file, passage, createdBy pgtype.UUID
		var locator pgtype.Text
		if err := rows.Scan(&id, &source, &file, &passage, &item.StatementTextAR, &locator, &item.ExtractionMethod, &item.ReviewStatus, &createdBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.SourceID = uuidString(source)
		item.SourceFileID = uuidString(file)
		item.SourcePassageID = uuidString(passage)
		item.LocatorAR = textValue(locator)
		item.CreatedBy = uuidString(createdBy)
		items = append(items, item)
	}
	return items, rows.Err()
}

// claimEvidence renders the evidence and counter-evidence attached to a claim. A row
// is kept only when its source is visible to the actor, so a research-only source
// cannot reach a public reader through a claim page. Counter-evidence keeps the
// "contradicts" relation so support and opposition stay distinguishable.
func (s *Service) claimEvidence(ctx context.Context, policy visibility.Policy, claimID uuid.UUID) ([]EvidenceView, error) {
	params := visibility.NewParams()
	claimRef := params.Add(claimID)
	sourcePredicate := policy.SourcePredicate(params, "s.id")
	dependencyStatus := `
		       CASE
		         WHEN s.id IS NULL THEN ''
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'confirmed') THEN 'derived'
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'needs_review') THEN 'likely_dependent'
		         ELSE s.dependency_status
		       END`
	rows, err := s.Pool.Query(ctx, `
		SELECT ce.id, ce.relation, ce.evidence_note_ar, ce.source_statement_id, ce.source_passage_id,
		       s.id, s.title_ar,`+dependencyStatus+`,
		       ss.statement_text_ar, sp.text_ar
		FROM claim_evidence ce
		LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
		LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
		LEFT JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
		WHERE ce.claim_id = `+claimRef+` AND (s.id IS NULL OR `+sourcePredicate+`)
		UNION ALL
		SELECT cce.id, 'contradicts', cce.note_ar, cce.source_statement_id, cce.source_passage_id,
		       s.id, s.title_ar,`+dependencyStatus+`,
		       ss.statement_text_ar, sp.text_ar
		FROM claim_counter_evidence cce
		LEFT JOIN source_statements ss ON ss.id = cce.source_statement_id
		LEFT JOIN source_passages sp ON sp.id = cce.source_passage_id
		LEFT JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
		WHERE cce.claim_id = `+claimRef+` AND (s.id IS NULL OR `+sourcePredicate+`)
		ORDER BY 1
	`, params.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceView, 0)
	for rows.Next() {
		var item EvidenceView
		var id, sourceID, statementID, passageID pgtype.UUID
		var note, title, dependencyStatus, statement, passage pgtype.Text
		if err := rows.Scan(&item.ID, &item.Relation, &note, &statementID, &passageID, &sourceID, &title, &dependencyStatus, &statement, &passage); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.EvidenceNoteAR = textValue(note)
		item.SourceID = uuidString(sourceID)
		item.SourceStatementID = uuidString(statementID)
		item.SourcePassageID = uuidString(passageID)
		item.SourceTitleAR = textValue(title)
		item.DependencyStatus = textValue(dependencyStatus)
		item.StatementTextAR = textValue(statement)
		item.PassageTextAR = textValue(passage)
		items = append(items, item)
	}
	return items, rows.Err()
}

func canManageSource(ctx context.Context, q dbExecutor, sourceID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sources s
			WHERE s.id = $1 AND (
				s.created_by = $2
				OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin'))
			)
		)
	`, sourceID, actorID).Scan(&allowed)
	return allowed, err
}

func canViewSource(ctx context.Context, q dbExecutor, sourceID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sources s
			WHERE s.id = $1 AND (
				s.visibility = 'public'
				OR s.created_by = $2
				OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin'))
			)
		)
	`, sourceID, actorID).Scan(&allowed)
	return allowed, err
}

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

func validateSourceInput(input CreateSourceInput) (CreateSourceInput, error) {
	input.TitleAR = strings.TrimSpace(input.TitleAR)
	input.AuthorAR = strings.TrimSpace(input.AuthorAR)
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.EditionAR = strings.TrimSpace(input.EditionAR)
	input.CitationAR = strings.TrimSpace(input.CitationAR)
	input.LocationAR = strings.TrimSpace(input.LocationAR)
	input.DependencyStatus = strings.TrimSpace(input.DependencyStatus)
	if input.DependencyStatus == "" {
		input.DependencyStatus = "unknown"
	}
	if input.TitleAR == "" || len([]rune(input.TitleAR)) > 500 || len([]rune(input.AuthorAR)) > 300 || len([]rune(input.EditionAR)) > 300 || len([]rune(input.CitationAR)) > 1000 || len([]rune(input.LocationAR)) > 500 || !validSourceType(input.SourceType) || !validDependencyStatus(input.DependencyStatus) {
		return CreateSourceInput{}, ErrValidation
	}
	return input, nil
}

func validatePassageInput(input SourcePassageInput) (SourcePassageInput, error) {
	input.LocatorAR = strings.TrimSpace(input.LocatorAR)
	input.TextAR = strings.TrimSpace(input.TextAR)
	if input.TextAR == "" || len([]rune(input.TextAR)) > 20000 || len([]rune(input.LocatorAR)) > 1000 || input.SequenceNumber < 0 {
		return SourcePassageInput{}, ErrValidation
	}
	if input.PageNumber != nil && *input.PageNumber < 0 {
		return SourcePassageInput{}, ErrValidation
	}
	return input, nil
}

func validateStatementInput(input SourceStatementInput) (SourceStatementInput, error) {
	input.SourcePassageID = strings.TrimSpace(input.SourcePassageID)
	input.StatementTextAR = strings.TrimSpace(input.StatementTextAR)
	input.LocatorAR = strings.TrimSpace(input.LocatorAR)
	input.ReviewStatus = strings.TrimSpace(input.ReviewStatus)
	if input.ReviewStatus == "" {
		input.ReviewStatus = "unreviewed"
	}
	if input.StatementTextAR == "" || len([]rune(input.StatementTextAR)) > 20000 || len([]rune(input.LocatorAR)) > 1000 || !validReviewStatus(input.ReviewStatus) {
		return SourceStatementInput{}, ErrValidation
	}
	return input, nil
}

func validateClaimInput(input CreateClaimInput) (CreateClaimInput, error) {
	input.SubjectType = strings.TrimSpace(input.SubjectType)
	input.Predicate = strings.TrimSpace(input.Predicate)
	input.ObjectType = strings.TrimSpace(input.ObjectType)
	input.Status = strings.TrimSpace(input.Status)
	input.NotesAR = strings.TrimSpace(input.NotesAR)
	if input.Status == "" {
		input.Status = string(claims.Unresolved)
	}
	if input.SubjectType == "" || input.Predicate == "" || input.ObjectType == "" || len([]rune(input.SubjectType)) > 80 || len([]rune(input.Predicate)) > 120 || len([]rune(input.ObjectType)) > 80 || len([]rune(input.NotesAR)) > 5000 {
		return CreateClaimInput{}, ErrValidation
	}
	if _, err := uuid.Parse(input.SubjectID); err != nil {
		return CreateClaimInput{}, ErrValidation
	}
	if _, err := uuid.Parse(input.ObjectID); err != nil {
		return CreateClaimInput{}, ErrValidation
	}
	if _, err := claims.ParseStatus(input.Status); err != nil {
		return CreateClaimInput{}, ErrValidation
	}
	return input, nil
}

func validateEvidenceInput(input AddEvidenceInput) (AddEvidenceInput, error) {
	input.SourceStatementID = strings.TrimSpace(input.SourceStatementID)
	input.SourcePassageID = strings.TrimSpace(input.SourcePassageID)
	input.EvidenceNoteAR = strings.TrimSpace(input.EvidenceNoteAR)
	input.Relation = strings.TrimSpace(input.Relation)
	if input.Relation == "" {
		input.Relation = "supports"
	}
	if (input.SourceStatementID == "" && input.SourcePassageID == "") || len([]rune(input.EvidenceNoteAR)) > 5000 || !validEvidenceRelation(input.Relation) {
		return AddEvidenceInput{}, ErrValidation
	}
	if input.SourceStatementID != "" {
		if _, err := uuid.Parse(input.SourceStatementID); err != nil {
			return AddEvidenceInput{}, ErrValidation
		}
	}
	if input.SourcePassageID != "" {
		if _, err := uuid.Parse(input.SourcePassageID); err != nil {
			return AddEvidenceInput{}, ErrValidation
		}
	}
	return input, nil
}

func validSourceType(value string) bool {
	switch value {
	case "book", "manuscript", "family_document", "archive_record", "newspaper", "article", "research_paper", "oral_testimony", "website", "published_tree", "historical_registry", "other":
		return true
	default:
		return false
	}
}

func validDependencyStatus(value string) bool {
	return value == "unknown" || value == "independent" || value == "derived" || value == "likely_dependent"
}

func validReviewStatus(value string) bool {
	return value == "unreviewed" || value == "accepted" || value == "rejected" || value == "needs_review"
}

func validEvidenceRelation(value string) bool {
	return value == "supports" || value == "contextualizes" || value == "contradicts" || value == "refutes"
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

func parseDateRange(fromValue, toValue string) (pgtype.Date, pgtype.Date, error) {
	from, err := parseDate(fromValue)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, err
	}
	to, err := parseDate(toValue)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, err
	}
	if from.Valid && to.Valid && to.Time.Before(from.Time) {
		return pgtype.Date{}, pgtype.Date{}, ErrValidation
	}
	return from, to, nil
}

func parseDate(value string) (pgtype.Date, error) {
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

func optionalUUID(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, ErrValidation
	}
	return parsed, nil
}

func writeAudit(ctx context.Context, q dbExecutor, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) error {
	_, err := q.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, actorID, action, entityType, entityID, marshalValue(before), marshalValue(after))
	return err
}

func marshalValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func mapConflict(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return ErrConflict
	}
	return err
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

func dateText(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format("2006-01-02")
}
