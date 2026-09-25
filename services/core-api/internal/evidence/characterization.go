package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	SourceCharacterizationAlgorithmVersion     = "source-characterization-v1"
	SourceCharacterizationQualificationVersion = "source-characterization-qualified-v1"
	SourceCharacterizationMaximumStatements    = 200
	SourceCharacterizationMaximumClaims        = 200
	SourceCharacterizationMaximumDependencies  = 100
	SourceCharacterizationMaximumPlaces        = 100
	SourceCharacterizationMaximumCorroborators = 100
)

type SourceCharacterizationInput struct {
	SourceID   string `json:"source_id"`
	QuestionID string `json:"question_id"`
	ClaimID    string `json:"claim_id"`
	PlaceID    string `json:"place_id"`
}

type SourceCharacterizationScope struct {
	SourceID   string `json:"sourceId"`
	QuestionID string `json:"questionId,omitempty"`
	ClaimID    string `json:"claimId,omitempty"`
	PlaceID    string `json:"placeId,omitempty"`
}

type SourceCharacterizationRun struct {
	ID                         string                           `json:"id"`
	SourceID                   string                           `json:"sourceId"`
	RequestedBy                string                           `json:"requestedBy"`
	QuestionID                 string                           `json:"questionId,omitempty"`
	Status                     string                           `json:"status"`
	ReportStatus               string                           `json:"reportStatus"`
	ReviewStatus               string                           `json:"reviewStatus"`
	ExecutionMode              string                           `json:"executionMode"`
	AlgorithmVersion           string                           `json:"algorithmVersion"`
	QualificationPolicyVersion string                           `json:"qualificationPolicyVersion"`
	ModelVersion               string                           `json:"modelVersion,omitempty"`
	InputFingerprint           string                           `json:"inputFingerprint"`
	Scope                      SourceCharacterizationScope      `json:"scope"`
	Report                     SourceCharacterizationReport     `json:"report"`
	FindingCount               int                              `json:"findingCount"`
	Error                      string                           `json:"error,omitempty"`
	CreatedAt                  time.Time                        `json:"createdAt"`
	StartedAt                  *time.Time                       `json:"startedAt,omitempty"`
	CompletedAt                *time.Time                       `json:"completedAt,omitempty"`
	UpdatedAt                  time.Time                        `json:"updatedAt"`
	Evidence                   []SourceCharacterizationEvidence `json:"evidence"`
	Reviews                    []SourceCharacterizationReview   `json:"reviews"`
}

type SourceCharacterizationReport struct {
	Source        SourceCharacterizationSource        `json:"source"`
	Attributes    []SourceCharacterizationAttribute   `json:"attributes"`
	Corroboration SourceCharacterizationCorroboration `json:"corroboration"`
	Limitations   []string                            `json:"limitations"`
	GeneratedAt   time.Time                           `json:"generatedAt"`
}

type SourceCharacterizationSource struct {
	ID                     string `json:"id"`
	TitleAR                string `json:"titleAr"`
	AuthorAR               string `json:"authorAr,omitempty"`
	SourceType             string `json:"sourceType"`
	PublicationDateFrom    string `json:"publicationDateFrom,omitempty"`
	PublicationDateTo      string `json:"publicationDateTo,omitempty"`
	EditionAR              string `json:"editionAr,omitempty"`
	CitationAR             string `json:"citationAr,omitempty"`
	LocationAR             string `json:"locationAr,omitempty"`
	DependencyStatus       string `json:"dependencyStatus"`
	Visibility             string `json:"visibility"`
	PassageCount           int    `json:"passageCount"`
	StatementCount         int    `json:"statementCount"`
	AcceptedStatementCount int    `json:"acceptedStatementCount"`
}

type SourceCharacterizationAttribute struct {
	Key         string   `json:"key"`
	LabelAR     string   `json:"labelAr"`
	Value       string   `json:"value"`
	State       string   `json:"state"`
	RationaleAR string   `json:"rationaleAr"`
	EvidenceIDs []string `json:"evidenceIds"`
}

type SourceCharacterizationCorroboration struct {
	IndependentSourceCount int                                  `json:"independentSourceCount"`
	Sources                []SourceCharacterizationCorroborator `json:"sources"`
}

type SourceCharacterizationCorroborator struct {
	SourceID      string   `json:"sourceId"`
	SourceTitleAR string   `json:"sourceTitleAr"`
	ClaimIDs      []string `json:"claimIds"`
	EvidenceCount int      `json:"evidenceCount"`
}

type SourceCharacterizationEvidence struct {
	ID                 string         `json:"id"`
	AttributeKey       string         `json:"attributeKey"`
	Relation           string         `json:"relation"`
	SourcePassageID    string         `json:"sourcePassageId,omitempty"`
	SourceStatementID  string         `json:"sourceStatementId,omitempty"`
	ClaimID            string         `json:"claimId,omitempty"`
	PlaceID            string         `json:"placeId,omitempty"`
	SourceDependencyID string         `json:"sourceDependencyId,omitempty"`
	RelatedSourceID    string         `json:"relatedSourceId,omitempty"`
	ExcerptAR          string         `json:"excerptAr"`
	Position           int            `json:"position"`
	Metadata           map[string]any `json:"metadata"`
}

type SourceCharacterizationReview struct {
	ID         string    `json:"id"`
	ReviewerID string    `json:"reviewerId"`
	Decision   string    `json:"decision"`
	NoteAR     string    `json:"noteAr,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ReviewSourceCharacterizationInput struct {
	Decision string `json:"decision"`
	NoteAR   string `json:"note_ar"`
}

type characterizationStatement struct {
	ID        string
	PassageID string
	Text      string
	LocatorAR string
}

type characterizationClaimEvidence struct {
	ID          string
	ClaimID     string
	StatementID string
	PassageID   string
	Predicate   string
	Relation    string
	ClaimStatus string
	Text        string
}

type characterizationDependency struct {
	ID                     string
	SourceID               string
	SourceTitleAR          string
	DependsOnSourceID      string
	DependsOnSourceTitleAR string
	DependencyType         string
	Status                 string
	EvidenceAR             string
}

type characterizationPlace struct {
	ID        string
	PlaceID   string
	PlaceName string
	Kind      string
	Relation  string
	Status    string
	Accepted  bool
}

type characterizationCorroborator struct {
	SourceID      string
	SourceTitleAR string
	ClaimIDs      []string
	EvidenceCount int
}

type characterizationEvidenceDraft struct {
	ID                 string
	AttributeKey       string
	Relation           string
	SourcePassageID    string
	SourceStatementID  string
	ClaimID            string
	PlaceID            string
	SourceDependencyID string
	RelatedSourceID    string
	ExcerptAR          string
	Position           int
	Metadata           map[string]any
}

type characterizationAnalysis struct {
	Statements    []characterizationStatement
	Claims        []characterizationClaimEvidence
	Dependencies  []characterizationDependency
	Places        []characterizationPlace
	Corroborators []characterizationCorroborator
}

func (s *Service) StartSourceCharacterization(ctx context.Context, actorID string, input SourceCharacterizationInput) (SourceCharacterizationRun, error) {
	if err := s.ready(); err != nil {
		return SourceCharacterizationRun{}, err
	}
	input, err := validateSourceCharacterizationInput(input)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return SourceCharacterizationRun{}, ErrForbidden
	}
	sourceUUID, err := uuid.Parse(input.SourceID)
	if err != nil {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	defer tx.Rollback(ctx)
	var lockedUpdatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM sources WHERE id = $1 FOR UPDATE`, sourceUUID).Scan(&lockedUpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SourceCharacterizationRun{}, ErrNotFound
		}
		return SourceCharacterizationRun{}, err
	}
	source, err := s.sourceByIDAny(ctx, tx, sourceUUID)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	allowed, err := canManageSource(ctx, tx, sourceUUID, actorUUID)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	if !allowed {
		return SourceCharacterizationRun{}, ErrForbidden
	}
	if err := validateSourceCharacterizationReferences(ctx, tx, input); err != nil {
		return SourceCharacterizationRun{}, err
	}
	analysis, err := loadSourceCharacterizationAnalysis(ctx, tx, sourceUUID, actorUUID, input)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	fingerprint := sourceCharacterizationFingerprint(input, source, analysis)
	existing, err := scanSourceCharacterizationRun(tx.QueryRow(ctx, sourceCharacterizationRunSelect+` WHERE source_id = $1 AND input_fingerprint = $2`, sourceUUID, fingerprint))
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return SourceCharacterizationRun{}, err
		}
		return s.GetSourceCharacterizationRun(ctx, actorID, existing.ID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SourceCharacterizationRun{}, err
	}
	runID := uuid.New()
	startedAt := time.Now().UTC()
	scope := SourceCharacterizationScope{SourceID: input.SourceID, QuestionID: input.QuestionID, ClaimID: input.ClaimID, PlaceID: input.PlaceID}
	if _, err := tx.Exec(ctx, `INSERT INTO source_characterization_runs (id, source_id, requested_by, question_id, status, report_status, review_status, execution_mode, algorithm_version, qualification_policy_version, input_fingerprint, scope, report, started_at) VALUES ($1, $2, $3, $4, 'running', NULL, 'needs_review', 'synchronous', $5, $6, $7, $8, '{}'::jsonb, $9)`, runID, sourceUUID, actorUUID, nullableUUIDValue(input.QuestionID), SourceCharacterizationAlgorithmVersion, SourceCharacterizationQualificationVersion, fingerprint, mustJSON(scope), startedAt); err != nil {
		return SourceCharacterizationRun{}, err
	}
	drafts := buildSourceCharacterizationEvidenceDrafts(source, analysis)
	evidenceByAttribute := make(map[string][]string)
	for index := range drafts {
		draft := &drafts[index]
		draft.ID = uuid.New().String()
		if _, err := tx.Exec(ctx, `INSERT INTO source_characterization_evidence (id, run_id, attribute_key, relation, source_passage_id, source_statement_id, claim_id, place_id, source_dependency_id, related_source_id, excerpt_ar, position, metadata) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`, draft.ID, runID, draft.AttributeKey, draft.Relation, nullableUUIDValue(draft.SourcePassageID), nullableUUIDValue(draft.SourceStatementID), nullableUUIDValue(draft.ClaimID), nullableUUIDValue(draft.PlaceID), nullableUUIDValue(draft.SourceDependencyID), nullableUUIDValue(draft.RelatedSourceID), draft.ExcerptAR, draft.Position, mustJSON(draft.Metadata)); err != nil {
			return SourceCharacterizationRun{}, err
		}
		evidenceByAttribute[draft.AttributeKey] = append(evidenceByAttribute[draft.AttributeKey], draft.ID)
	}
	report := buildSourceCharacterizationReport(source, analysis, evidenceByAttribute, time.Now().UTC())
	reportStatus := "succeeded"
	if len(analysis.Statements) == 0 {
		reportStatus = "insufficient_evidence"
	}
	findingCount := sourceCharacterizationFindingCount(report)
	if _, err := tx.Exec(ctx, `UPDATE source_characterization_runs SET status = 'succeeded', report_status = $1, report = $2, finding_count = $3, completed_at = now(), updated_at = now() WHERE id = $4`, reportStatus, mustJSON(report), findingCount, runID); err != nil {
		return SourceCharacterizationRun{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "source_characterization_run_completed", "source_characterization_run", runID, nil, map[string]any{"sourceId": input.SourceID, "reportStatus": reportStatus, "evidenceCount": len(drafts), "findingCount": findingCount, "reviewStatus": "needs_review"}); err != nil {
		return SourceCharacterizationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceCharacterizationRun{}, err
	}
	return s.GetSourceCharacterizationRun(ctx, actorID, runID.String())
}

func (s *Service) GetSourceCharacterizationRun(ctx context.Context, actorID, runID string) (SourceCharacterizationRun, error) {
	if err := s.ready(); err != nil {
		return SourceCharacterizationRun{}, err
	}
	actorUUID, err := sourceCharacterizationViewerUUID(actorID)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	result, err := scanSourceCharacterizationRun(s.Pool.QueryRow(ctx, sourceCharacterizationRunSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	sourceID, err := uuid.Parse(result.SourceID)
	if err != nil {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	allowed, err := canViewSource(ctx, s.Pool, sourceID, actorUUID)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	if !allowed {
		return SourceCharacterizationRun{}, ErrForbidden
	}
	if err := s.loadSourceCharacterizationDetails(ctx, actorUUID, &result); err != nil {
		return SourceCharacterizationRun{}, err
	}
	return result, nil
}

func (s *Service) GetLatestSourceCharacterization(ctx context.Context, actorID string, input SourceCharacterizationInput) (SourceCharacterizationRun, error) {
	if err := s.ready(); err != nil {
		return SourceCharacterizationRun{}, err
	}
	actorUUID, err := sourceCharacterizationViewerUUID(actorID)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	input, err = validateSourceCharacterizationInput(input)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	sourceUUID, err := uuid.Parse(input.SourceID)
	if err != nil {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	if _, err := s.sourceByIDAny(ctx, s.Pool, sourceUUID); err != nil {
		return SourceCharacterizationRun{}, err
	}
	allowed, err := canViewSource(ctx, s.Pool, sourceUUID, actorUUID)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	if !allowed {
		return SourceCharacterizationRun{}, ErrForbidden
	}
	result, err := scanSourceCharacterizationRun(s.Pool.QueryRow(ctx, sourceCharacterizationRunSelect+` WHERE source_id = $1 AND ($2 = '' OR COALESCE(scope->>'questionId', '') = $2) AND ($3 = '' OR COALESCE(scope->>'claimId', '') = $3) AND ($4 = '' OR COALESCE(scope->>'placeId', '') = $4) ORDER BY created_at DESC LIMIT 1`, sourceUUID, input.QuestionID, input.ClaimID, input.PlaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	if err := s.loadSourceCharacterizationDetails(ctx, actorUUID, &result); err != nil {
		return SourceCharacterizationRun{}, err
	}
	return result, nil
}

func (s *Service) ReviewSourceCharacterization(ctx context.Context, actorID, runID string, input ReviewSourceCharacterizationInput) (SourceCharacterizationRun, error) {
	if err := s.ready(); err != nil {
		return SourceCharacterizationRun{}, err
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return SourceCharacterizationRun{}, ErrForbidden
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	input, err = validateSourceCharacterizationReview(input)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	defer tx.Rollback(ctx)
	result, err := scanSourceCharacterizationRun(tx.QueryRow(ctx, sourceCharacterizationRunSelect+` WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	sourceID, err := uuid.Parse(result.SourceID)
	if err != nil {
		return SourceCharacterizationRun{}, ErrNotFound
	}
	allowed, err := canManageSource(ctx, tx, sourceID, actorUUID)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	if !allowed {
		return SourceCharacterizationRun{}, ErrForbidden
	}
	nextStatus, err := sourceCharacterizationNextStatus(result.ReviewStatus, input.Decision)
	if err != nil {
		return SourceCharacterizationRun{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_characterization_runs SET review_status = $1, updated_at = now() WHERE id = $2`, nextStatus, id); err != nil {
		return SourceCharacterizationRun{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_characterization_reviews (run_id, reviewer_id, decision, note_ar) VALUES ($1, $2, $3, NULLIF($4, ''))`, id, actorUUID, input.Decision, input.NoteAR); err != nil {
		return SourceCharacterizationRun{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, "source_characterization_reviewed", "source_characterization_run", id, map[string]any{"reviewStatus": result.ReviewStatus}, map[string]any{"reviewStatus": nextStatus, "decision": input.Decision}); err != nil {
		return SourceCharacterizationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceCharacterizationRun{}, err
	}
	return s.GetSourceCharacterizationRun(ctx, actorID, id.String())
}

func validateSourceCharacterizationReferences(ctx context.Context, q dbExecutor, input SourceCharacterizationInput) error {
	if input.QuestionID != "" {
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM open_questions WHERE id = $1)`, input.QuestionID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	if input.ClaimID != "" {
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM claims WHERE id = $1)`, input.ClaimID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	if input.PlaceID != "" {
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM places WHERE id = $1)`, input.PlaceID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func sourceCharacterizationViewerUUID(actorID string) (uuid.UUID, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return uuid.Nil, nil
	}
	parsed, err := uuid.Parse(actorID)
	if err != nil {
		return uuid.Nil, ErrForbidden
	}
	return parsed, nil
}

func validateSourceCharacterizationInput(input SourceCharacterizationInput) (SourceCharacterizationInput, error) {
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.QuestionID = strings.TrimSpace(input.QuestionID)
	input.ClaimID = strings.TrimSpace(input.ClaimID)
	input.PlaceID = strings.TrimSpace(input.PlaceID)
	if input.SourceID == "" {
		return SourceCharacterizationInput{}, ErrValidation
	}
	if _, err := uuid.Parse(input.SourceID); err != nil {
		return SourceCharacterizationInput{}, ErrValidation
	}
	for _, value := range []string{input.QuestionID, input.ClaimID, input.PlaceID} {
		if value == "" {
			continue
		}
		if _, err := uuid.Parse(value); err != nil {
			return SourceCharacterizationInput{}, ErrValidation
		}
	}
	return input, nil
}

func nullableUUIDValue(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return parsed
}

func validateSourceCharacterizationReview(input ReviewSourceCharacterizationInput) (ReviewSourceCharacterizationInput, error) {
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if input.Decision != "confirm" && input.Decision != "dismiss" && input.Decision != "reopen" {
		return ReviewSourceCharacterizationInput{}, ErrValidation
	}
	if len([]rune(input.NoteAR)) > 2000 {
		return ReviewSourceCharacterizationInput{}, ErrValidation
	}
	return input, nil
}

func sourceCharacterizationNextStatus(current, decision string) (string, error) {
	if decision == "reopen" {
		if current == "needs_review" {
			return "", ErrConflict
		}
		return "needs_review", nil
	}
	if current != "needs_review" {
		return "", ErrConflict
	}
	if decision == "confirm" {
		return "confirmed", nil
	}
	return "dismissed", nil
}

func sourceCharacterizationFingerprint(input SourceCharacterizationInput, source SourceView, analysis characterizationAnalysis) string {
	payload := struct {
		Input    SourceCharacterizationInput `json:"input"`
		Source   SourceView                  `json:"source"`
		Analysis characterizationAnalysis    `json:"analysis"`
	}{Input: input, Source: source, Analysis: analysis}
	encoded, _ := json.Marshal(payload)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func loadSourceCharacterizationAnalysis(ctx context.Context, q dbExecutor, sourceID, actorID uuid.UUID, input SourceCharacterizationInput) (characterizationAnalysis, error) {
	statements, err := loadCharacterizationStatements(ctx, q, sourceID)
	if err != nil {
		return characterizationAnalysis{}, err
	}
	claims, err := loadCharacterizationClaims(ctx, q, sourceID, input.ClaimID, input.PlaceID, input.QuestionID)
	if err != nil {
		return characterizationAnalysis{}, err
	}
	dependencies, err := loadCharacterizationDependencies(ctx, q, sourceID, actorID)
	if err != nil {
		return characterizationAnalysis{}, err
	}
	places, err := loadCharacterizationPlaces(ctx, q, sourceID, input.PlaceID)
	if err != nil {
		return characterizationAnalysis{}, err
	}
	corroborators, err := loadCharacterizationCorroborators(ctx, q, sourceID, actorID, input.ClaimID, input.PlaceID, input.QuestionID)
	if err != nil {
		return characterizationAnalysis{}, err
	}
	return characterizationAnalysis{Statements: statements, Claims: claims, Dependencies: dependencies, Places: places, Corroborators: corroborators}, nil
}

func loadCharacterizationStatements(ctx context.Context, q dbExecutor, sourceID uuid.UUID) ([]characterizationStatement, error) {
	rows, err := q.Query(ctx, `SELECT ss.id::text, COALESCE(ss.source_passage_id::text, ''), ss.statement_text_ar, COALESCE(ss.locator_ar, '') FROM source_statements ss WHERE ss.source_id = $1 AND ss.review_status = 'accepted' ORDER BY ss.created_at, ss.id LIMIT $2`, sourceID, SourceCharacterizationMaximumStatements)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]characterizationStatement, 0)
	for rows.Next() {
		var item characterizationStatement
		if err := rows.Scan(&item.ID, &item.PassageID, &item.Text, &item.LocatorAR); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadCharacterizationClaims(ctx context.Context, q dbExecutor, sourceID uuid.UUID, claimID, placeID, questionID string) ([]characterizationClaimEvidence, error) {
	rows, err := q.Query(ctx, `
		SELECT ce.id::text, ce.claim_id::text, ce.source_statement_id::text, COALESCE(ce.source_passage_id::text, ''), c.predicate, ce.relation, c.status, ss.statement_text_ar
		FROM claim_evidence ce
		JOIN claims c ON c.id = ce.claim_id
		JOIN source_statements ss ON ss.id = ce.source_statement_id
		WHERE ss.source_id = $1 AND ss.review_status = 'accepted' AND ce.relation = 'supports'
		  AND ($2::uuid IS NULL OR ce.claim_id = $2::uuid)
		  AND ($3::uuid IS NULL OR c.place_id = $3::uuid)
		  AND ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM question_claims qc WHERE qc.question_id = $4::uuid AND qc.claim_id = c.id))
		ORDER BY ce.created_at, ce.id
		LIMIT $5
	`, sourceID, nullableUUIDValue(claimID), nullableUUIDValue(placeID), nullableUUIDValue(questionID), SourceCharacterizationMaximumClaims)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]characterizationClaimEvidence, 0)
	for rows.Next() {
		var item characterizationClaimEvidence
		if err := rows.Scan(&item.ID, &item.ClaimID, &item.StatementID, &item.PassageID, &item.Predicate, &item.Relation, &item.ClaimStatus, &item.Text); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadCharacterizationDependencies(ctx context.Context, q dbExecutor, sourceID, actorID uuid.UUID) ([]characterizationDependency, error) {
	rows, err := q.Query(ctx, `
		SELECT d.id::text, d.source_id::text, source.title_ar, COALESCE(d.depends_on_source_id::text, ''), COALESCE(target.title_ar, ''), d.dependency_type, d.status, COALESCE(d.evidence_ar, '')
		FROM source_dependencies d
		JOIN sources source ON source.id = d.source_id
		LEFT JOIN sources target ON target.id = d.depends_on_source_id
		WHERE (d.source_id = $1 OR d.depends_on_source_id = $1)
		  AND d.status <> 'rejected'
		  AND (source.visibility = 'public' OR source.created_by = $2 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')))
		  AND (target.id IS NULL OR target.visibility = 'public' OR target.created_by = $2 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')))
		ORDER BY d.created_at DESC, d.id
		LIMIT $3
	`, sourceID, actorID, SourceCharacterizationMaximumDependencies)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]characterizationDependency, 0)
	for rows.Next() {
		var item characterizationDependency
		if err := rows.Scan(&item.ID, &item.SourceID, &item.SourceTitleAR, &item.DependsOnSourceID, &item.DependsOnSourceTitleAR, &item.DependencyType, &item.Status, &item.EvidenceAR); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadCharacterizationPlaces(ctx context.Context, q dbExecutor, sourceID uuid.UUID, placeID string) ([]characterizationPlace, error) {
	rows, err := q.Query(ctx, `
		SELECT 'association', ga.id::text, ga.place_id::text, p.canonical_name_ar, ga.relation_type, ga.status,
		       EXISTS (SELECT 1 FROM spatial_evidence se WHERE se.geographic_association_id = ga.id AND se.review_status = 'accepted')
		FROM geographic_associations ga
		JOIN places p ON p.id = ga.place_id
		WHERE ga.source_id = $1
		  AND ($2::uuid IS NULL OR ga.place_id = $2::uuid)
		UNION ALL
		SELECT 'migration', m.id::text, COALESCE(m.to_place_id, m.from_place_id)::text, COALESCE(to_place.canonical_name_ar, from_place.canonical_name_ar, ''), 'migration', m.status,
		       EXISTS (SELECT 1 FROM spatial_evidence se WHERE se.migration_event_id = m.id AND se.review_status = 'accepted')
		FROM migration_events m
		LEFT JOIN places to_place ON to_place.id = m.to_place_id
		LEFT JOIN places from_place ON from_place.id = m.from_place_id
		WHERE m.source_id = $1
		  AND ($2::uuid IS NULL OR COALESCE(m.to_place_id, m.from_place_id) = $2::uuid)
		ORDER BY 1, 2
		LIMIT $3
	`, sourceID, nullableUUIDValue(placeID), SourceCharacterizationMaximumPlaces)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]characterizationPlace, 0)
	for rows.Next() {
		var item characterizationPlace
		if err := rows.Scan(&item.Kind, &item.ID, &item.PlaceID, &item.PlaceName, &item.Relation, &item.Status, &item.Accepted); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadCharacterizationCorroborators(ctx context.Context, q dbExecutor, sourceID, actorID uuid.UUID, claimID, placeID, questionID string) ([]characterizationCorroborator, error) {
	rows, err := q.Query(ctx, `
		WITH source_claims AS (
			SELECT DISTINCT ce.claim_id
			FROM claim_evidence ce
			JOIN claims c ON c.id = ce.claim_id
			JOIN source_statements ss ON ss.id = ce.source_statement_id
			WHERE ss.source_id = $1 AND ss.review_status = 'accepted' AND ce.relation = 'supports'
			  AND ($2::uuid IS NULL OR ce.claim_id = $2::uuid)
			  AND ($3::uuid IS NULL OR c.place_id = $3::uuid)
			  AND ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM question_claims qc WHERE qc.question_id = $4::uuid AND qc.claim_id = c.id))
		)
		SELECT other.id::text, other.title_ar, COUNT(DISTINCT ce.claim_id)::integer,
		       COALESCE(string_agg(DISTINCT ce.claim_id::text, ','), '')
		FROM source_claims sc
		JOIN claim_evidence ce ON ce.claim_id = sc.claim_id
		JOIN source_statements ss ON ss.id = ce.source_statement_id
		JOIN sources other ON other.id = ss.source_id
		WHERE ce.relation = 'supports' AND ss.review_status = 'accepted' AND other.id <> $1
		  AND (other.visibility = 'public' OR other.created_by = $5 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $5 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')))
		  AND NOT EXISTS (
			SELECT 1 FROM source_dependencies d
			WHERE d.status IN ('needs_review', 'confirmed')
			  AND ((d.source_id = $1 AND d.depends_on_source_id = other.id) OR (d.source_id = other.id AND d.depends_on_source_id = $1))
		  )
		GROUP BY other.id, other.title_ar
		ORDER BY COUNT(DISTINCT ce.claim_id) DESC, other.id
		LIMIT $6
	`, sourceID, nullableUUIDValue(claimID), nullableUUIDValue(placeID), nullableUUIDValue(questionID), actorID, SourceCharacterizationMaximumCorroborators)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]characterizationCorroborator, 0)
	for rows.Next() {
		var item characterizationCorroborator
		var claimIDs string
		if err := rows.Scan(&item.SourceID, &item.SourceTitleAR, &item.EvidenceCount, &claimIDs); err != nil {
			return nil, err
		}
		if claimIDs != "" {
			item.ClaimIDs = strings.Split(claimIDs, ",")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func buildSourceCharacterizationEvidenceDrafts(source SourceView, analysis characterizationAnalysis) []characterizationEvidenceDraft {
	drafts := make([]characterizationEvidenceDraft, 0)
	for index, statement := range analysis.Statements {
		drafts = append(drafts, characterizationEvidenceDraft{AttributeKey: "accepted_evidence", Relation: "describes", SourcePassageID: statement.PassageID, SourceStatementID: statement.ID, ExcerptAR: boundedCharacterizationText(statement.Text), Position: index, Metadata: map[string]any{"reviewStatus": "accepted", "locatorAr": statement.LocatorAR}})
	}
	for index, claim := range analysis.Claims {
		drafts = append(drafts, characterizationEvidenceDraft{AttributeKey: "claim_links", Relation: "supports", SourcePassageID: claim.PassageID, SourceStatementID: claim.StatementID, ClaimID: claim.ClaimID, ExcerptAR: boundedCharacterizationText(claim.Text), Position: index, Metadata: map[string]any{"claimEvidenceId": claim.ID, "predicate": claim.Predicate, "claimStatus": claim.ClaimStatus}})
	}
	for index, dependency := range analysis.Dependencies {
		relatedSourceID := dependency.DependsOnSourceID
		if dependency.SourceID != source.ID {
			relatedSourceID = dependency.SourceID
		}
		excerpt := dependency.EvidenceAR
		if excerpt == "" {
			excerpt = dependency.SourceTitleAR + " / " + dependency.DependsOnSourceTitleAR
		}
		drafts = append(drafts, characterizationEvidenceDraft{AttributeKey: "dependencies", Relation: "qualifies", SourceDependencyID: dependency.ID, RelatedSourceID: relatedSourceID, ExcerptAR: boundedCharacterizationText(excerpt), Position: index, Metadata: map[string]any{"status": dependency.Status, "dependencyType": dependency.DependencyType}})
	}
	for index, place := range analysis.Places {
		drafts = append(drafts, characterizationEvidenceDraft{AttributeKey: "geography", Relation: "describes", PlaceID: place.PlaceID, ExcerptAR: boundedCharacterizationText(place.PlaceName + " / " + place.Relation), Position: index, Metadata: map[string]any{"kind": place.Kind, "status": place.Status, "acceptedSpatialEvidence": place.Accepted}})
	}
	position := 0
	for _, corroborator := range analysis.Corroborators {
		for _, claimID := range corroborator.ClaimIDs {
			drafts = append(drafts, characterizationEvidenceDraft{AttributeKey: "independent_corroboration", Relation: "qualifies", ClaimID: claimID, RelatedSourceID: corroborator.SourceID, ExcerptAR: "مصدر مستقل آخر يرتبط بنفس الادعاء.", Position: position, Metadata: map[string]any{"corroboratingSourceId": corroborator.SourceID, "claimId": claimID}})
			position++
		}
	}
	return drafts
}

func buildSourceCharacterizationReport(source SourceView, analysis characterizationAnalysis, evidenceByAttribute map[string][]string, generatedAt time.Time) SourceCharacterizationReport {
	attributes := make([]SourceCharacterizationAttribute, 0, 8)
	attributes = append(attributes,
		SourceCharacterizationAttribute{Key: "source_role", LabelAR: "الدور المصدري", Value: "غير محدد", State: "unknown", RationaleAR: "نوع المصدر يصف شكل الحفظ، ولا يثبت وحده أن المصدر أولي أو ثانوي.", EvidenceIDs: []string{}},
		SourceCharacterizationAttribute{Key: "temporal_relation", LabelAR: "العلاقة الزمنية", Value: "غير محددة", State: "unknown", RationaleAR: "لا يوجد في هذا الإصدار نطاق زمني مرجعي كافٍ للمقارنة.", EvidenceIDs: []string{}},
		SourceCharacterizationAttribute{Key: "author_proximity", LabelAR: "قرب المؤلف", Value: "غير محدد", State: "unknown", RationaleAR: "بيانات المؤلف حرة، ولا يوجد كيان مؤلف يثبت علاقة القرب.", EvidenceIDs: []string{}},
		SourceCharacterizationAttribute{Key: "citation_metadata", LabelAR: "بيانات الاقتباس", Value: citationMetadataValue(source), State: citationMetadataState(source), RationaleAR: "تظهر حقول البيانات الوصفية المتاحة، من دون الحكم على جودتها.", EvidenceIDs: []string{}},
		SourceCharacterizationAttribute{Key: "accepted_evidence", LabelAR: "العبارات المقبولة", Value: fmt.Sprintf("%d عبارة", len(analysis.Statements)), State: observedState(len(analysis.Statements) > 0, false), RationaleAR: "تُحسب العبارات المقبولة من هذا المصدر فقط، ولا تتحول إلى حكم تاريخي.", EvidenceIDs: evidenceByAttribute["accepted_evidence"]},
		SourceCharacterizationAttribute{Key: "claim_links", LabelAR: "الادعاءات المرتبطة", Value: fmt.Sprintf("%d ارتباط", len(analysis.Claims)), State: observedState(len(analysis.Claims) > 0, false), RationaleAR: "تمثل روابط الأدلة المقبولة، ولا تثبت استقلالها.", EvidenceIDs: evidenceByAttribute["claim_links"]},
	)
	dependencyValue := fmt.Sprintf("%d علاقة", len(analysis.Dependencies))
	dependencyState := "unknown"
	if len(analysis.Dependencies) > 0 {
		dependencyState = "observed"
		if sourceDependencyNeedsReview(analysis.Dependencies) {
			dependencyState = "needs_review"
		}
	}
	attributes = append(attributes, SourceCharacterizationAttribute{Key: "dependencies", LabelAR: "الاعتماد المصدري", Value: dependencyValue, State: dependencyState, RationaleAR: "تسجل العلاقات 방향ها وحالة المراجعة، ولا تعني الحسم.", EvidenceIDs: evidenceByAttribute["dependencies"]})
	geographyState := "unknown"
	geographyRationale := "لم تُسجل إشارات جغرافية موثقة لهذا المصدر."
	if len(analysis.Places) > 0 {
		geographyState = "observed"
		geographyRationale = "تجمع المواضع من سجلات المصدر الجغرافية، ولا تستخدم حقل الموقع الحرر كدليل جغرافي."
		if sourcePlacesNeedReview(analysis.Places) {
			geographyState = "needs_review"
		}
	}
	attributes = append(attributes, SourceCharacterizationAttribute{Key: "geography", LabelAR: "السياق الجغرافي", Value: fmt.Sprintf("%d موضع", len(analysis.Places)), State: geographyState, RationaleAR: geographyRationale, EvidenceIDs: evidenceByAttribute["geography"]})
	corroborationState := "unknown"
	corroborationRationale := "لا توجد ادعاءات مرتبطة تكفي لقياس التأكيد المستقل."
	if len(analysis.Claims) > 0 {
		corroborationState = "observed"
		corroborationRationale = "يعد فقط مصادر أخرى مرتبطة بنفس الادعاءات، مع استبعاد العلاقات المؤكدة أو التي تحتاج مراجعة؛ ولا يصدر حكماً عن موثوقية المصدر."
	}
	attributes = append(attributes, SourceCharacterizationAttribute{Key: "independent_corroboration", LabelAR: "التأكيد المستقل", Value: fmt.Sprintf("%d مصدر", len(analysis.Corroborators)), State: corroborationState, RationaleAR: corroborationRationale, EvidenceIDs: evidenceByAttribute["independent_corroboration"]})
	corroborators := make([]SourceCharacterizationCorroborator, 0, len(analysis.Corroborators))
	for _, item := range analysis.Corroborators {
		corroborators = append(corroborators, SourceCharacterizationCorroborator{SourceID: item.SourceID, SourceTitleAR: item.SourceTitleAR, ClaimIDs: item.ClaimIDs, EvidenceCount: item.EvidenceCount})
	}
	limitations := []string{
		"لا يحدد هذا الملف المصدر كأول أو ثانوي، ولا يحسب درجة ثقة.",
		"لا يقارن المصدر بتاريخ حدث تاريخي لأن نطاق المقارنة غير متاح.",
		"لا يثبت قرب المؤلف لأن بيانات المؤلف لا تملك كياناً مستقلاً.",
		"حقل الموقع الحرر لا يعامل مكاناً موثقاً.",
	}
	if len(analysis.Statements) == 0 {
		limitations = append(limitations, "لا توجد عبارات مقبولة، لذلك يبقى الملف غير كافٍ للخصائص الاستدلالية.")
	}
	return SourceCharacterizationReport{Source: sourceCharacterizationSource(source, len(analysis.Statements)), Attributes: attributes, Corroboration: SourceCharacterizationCorroboration{IndependentSourceCount: len(corroborators), Sources: corroborators}, Limitations: limitations, GeneratedAt: generatedAt}
}

func sourceCharacterizationSource(source SourceView, acceptedCount int) SourceCharacterizationSource {
	return SourceCharacterizationSource{ID: source.ID, TitleAR: source.TitleAR, AuthorAR: source.AuthorAR, SourceType: source.SourceType, PublicationDateFrom: source.PublicationDateFrom, PublicationDateTo: source.PublicationDateTo, EditionAR: source.EditionAR, CitationAR: source.CitationAR, LocationAR: source.LocationAR, DependencyStatus: source.DependencyStatus, Visibility: source.Visibility, PassageCount: source.PassageCount, StatementCount: source.StatementCount, AcceptedStatementCount: acceptedCount}
}

func citationMetadataValue(source SourceView) string {
	fields := []string{source.CitationAR, source.EditionAR, source.AuthorAR, source.PublicationDateFrom, source.PublicationDateTo}
	provided := 0
	for _, value := range fields {
		if strings.TrimSpace(value) != "" {
			provided++
		}
	}
	if provided == 0 {
		return "غير متاحة"
	}
	return fmt.Sprintf("%d من حقول البيانات الوصفية متاحة", provided)
}

func citationMetadataState(source SourceView) string {
	if strings.TrimSpace(source.CitationAR) == "" && strings.TrimSpace(source.EditionAR) == "" && strings.TrimSpace(source.AuthorAR) == "" && strings.TrimSpace(source.PublicationDateFrom) == "" && strings.TrimSpace(source.PublicationDateTo) == "" {
		return "unknown"
	}
	return "observed"
}

func sourceCharacterizationFindingCount(report SourceCharacterizationReport) int {
	count := 0
	for _, attribute := range report.Attributes {
		if attribute.State == "needs_review" {
			count++
		}
	}
	return count
}

func observedState(observed, needsReview bool) string {
	if needsReview {
		return "needs_review"
	}
	if observed {
		return "observed"
	}
	return "unknown"
}

func sourceDependencyNeedsReview(items []characterizationDependency) bool {
	for _, item := range items {
		if item.Status == "needs_review" {
			return true
		}
	}
	return false
}

func sourcePlacesNeedReview(items []characterizationPlace) bool {
	for _, item := range items {
		if !item.Accepted || (item.Status != "documented" && item.Status != "interpreted") {
			return true
		}
	}
	return false
}

func boundedCharacterizationText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 1200 {
		return string(runes[:1200])
	}
	return value
}

func (s *Service) loadSourceCharacterizationDetails(ctx context.Context, actorID uuid.UUID, result *SourceCharacterizationRun) error {
	runID, err := uuid.Parse(result.ID)
	if err != nil {
		return ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text, attribute_key, relation, COALESCE(source_passage_id::text, ''), COALESCE(source_statement_id::text, ''), COALESCE(claim_id::text, ''), COALESCE(place_id::text, ''), COALESCE(source_dependency_id::text, ''), COALESCE(related_source_id::text, ''), excerpt_ar, position, metadata FROM source_characterization_evidence e WHERE e.run_id = $1 AND (e.related_source_id IS NULL OR EXISTS (SELECT 1 FROM sources related WHERE related.id = e.related_source_id AND (related.visibility = 'public' OR related.created_by = $2 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin'))))) ORDER BY e.attribute_key, e.position, e.created_at, e.id`, runID, actorID)
	if err != nil {
		return err
	}
	result.Evidence = make([]SourceCharacterizationEvidence, 0)
	visibleRelatedSourceIDs := make(map[string]bool)
	for rows.Next() {
		var item SourceCharacterizationEvidence
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.AttributeKey, &item.Relation, &item.SourcePassageID, &item.SourceStatementID, &item.ClaimID, &item.PlaceID, &item.SourceDependencyID, &item.RelatedSourceID, &item.ExcerptAR, &item.Position, &metadata); err != nil {
			rows.Close()
			return err
		}
		item.Metadata = map[string]any{}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &item.Metadata)
		}
		if item.RelatedSourceID != "" {
			visibleRelatedSourceIDs[item.RelatedSourceID] = true
		}
		result.Evidence = append(result.Evidence, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if err := s.filterSourceCharacterizationReport(ctx, actorID, &result.Report, visibleRelatedSourceIDs, result.Evidence); err != nil {
		return err
	}
	reviewRows, err := s.Pool.Query(ctx, `SELECT id::text, reviewer_id::text, decision, COALESCE(note_ar, ''), created_at FROM source_characterization_reviews WHERE run_id = $1 ORDER BY created_at DESC, id`, runID)
	if err != nil {
		return err
	}
	defer reviewRows.Close()
	result.Reviews = make([]SourceCharacterizationReview, 0)
	for reviewRows.Next() {
		var item SourceCharacterizationReview
		if err := reviewRows.Scan(&item.ID, &item.ReviewerID, &item.Decision, &item.NoteAR, &item.CreatedAt); err != nil {
			return err
		}
		result.Reviews = append(result.Reviews, item)
	}
	return reviewRows.Err()
}

func (s *Service) filterSourceCharacterizationReport(ctx context.Context, actorID uuid.UUID, report *SourceCharacterizationReport, visibleRelatedSourceIDs map[string]bool, evidence []SourceCharacterizationEvidence) error {
	evidenceIDs := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		evidenceIDs[item.ID] = true
	}
	for index := range report.Attributes {
		filteredIDs := make([]string, 0, len(report.Attributes[index].EvidenceIDs))
		for _, evidenceID := range report.Attributes[index].EvidenceIDs {
			if evidenceIDs[evidenceID] {
				filteredIDs = append(filteredIDs, evidenceID)
			}
		}
		report.Attributes[index].EvidenceIDs = filteredIDs
	}
	filteredSources := make([]SourceCharacterizationCorroborator, 0, len(report.Corroboration.Sources))
	for _, source := range report.Corroboration.Sources {
		if visibleRelatedSourceIDs[source.SourceID] {
			filteredSources = append(filteredSources, source)
			continue
		}
		sourceID, err := uuid.Parse(source.SourceID)
		if err != nil {
			continue
		}
		allowed, err := canViewSource(ctx, s.Pool, sourceID, actorID)
		if err != nil {
			return err
		}
		if allowed {
			filteredSources = append(filteredSources, source)
		}
	}
	report.Corroboration.Sources = filteredSources
	report.Corroboration.IndependentSourceCount = len(filteredSources)
	return nil
}

const sourceCharacterizationRunSelect = `
	SELECT id::text, source_id::text, requested_by::text, COALESCE(question_id::text, ''), status, COALESCE(report_status, ''), review_status,
	       execution_mode, algorithm_version, qualification_policy_version, COALESCE(model_version, ''), input_fingerprint, scope, report,
	       finding_count, COALESCE(error, ''), created_at, started_at, completed_at, updated_at
	FROM source_characterization_runs`

func scanSourceCharacterizationRun(row pgx.Row) (SourceCharacterizationRun, error) {
	var result SourceCharacterizationRun
	var questionID, reportStatus, modelVersion, runError pgtype.Text
	var scopeData, reportData []byte
	var startedAt, completedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.SourceID, &result.RequestedBy, &questionID, &result.Status, &reportStatus, &result.ReviewStatus, &result.ExecutionMode, &result.AlgorithmVersion, &result.QualificationPolicyVersion, &modelVersion, &result.InputFingerprint, &scopeData, &reportData, &result.FindingCount, &runError, &result.CreatedAt, &startedAt, &completedAt, &result.UpdatedAt); err != nil {
		return SourceCharacterizationRun{}, err
	}
	result.QuestionID = textValue(questionID)
	result.ReportStatus = textValue(reportStatus)
	result.ModelVersion = textValue(modelVersion)
	result.Error = textValue(runError)
	if len(scopeData) > 0 {
		_ = json.Unmarshal(scopeData, &result.Scope)
	}
	if len(reportData) > 0 {
		_ = json.Unmarshal(reportData, &result.Report)
	}
	if result.Report.Attributes == nil {
		result.Report.Attributes = []SourceCharacterizationAttribute{}
	}
	if result.Report.Corroboration.Sources == nil {
		result.Report.Corroboration.Sources = []SourceCharacterizationCorroborator{}
	}
	if result.Report.Limitations == nil {
		result.Report.Limitations = []string{}
	}
	if result.Evidence == nil {
		result.Evidence = []SourceCharacterizationEvidence{}
	}
	if result.Reviews == nil {
		result.Reviews = []SourceCharacterizationReview{}
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
