package research

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type WorkspaceInput struct {
	QuestionID    string `json:"questionId"`
	EntityType    string `json:"entityType"`
	EntityID      string `json:"entityId"`
	TreeID        string `json:"treeId"`
	TreeVersionID string `json:"treeVersionId"`
}

type WorkspaceContext struct {
	QuestionID       string   `json:"questionId"`
	QuestionTitleAR  string   `json:"questionTitleAr"`
	QuestionDetailAR string   `json:"questionDetailAr,omitempty"`
	QuestionStatus   string   `json:"questionStatus"`
	QuestionPriority string   `json:"questionPriority"`
	EntityType       string   `json:"entityType,omitempty"`
	EntityID         string   `json:"entityId,omitempty"`
	EntityNameAR     string   `json:"entityNameAr,omitempty"`
	EntityAliasesAR  []string `json:"entityAliasesAr,omitempty"`
	TreeID           string   `json:"treeId,omitempty"`
	TreeVersionID    string   `json:"treeVersionId,omitempty"`
}

type WorkspacePermissions struct {
	CanRunResearch                     bool `json:"canRunResearch"`
	CanRunTemporalAnalysis             bool `json:"canRunTemporalAnalysis"`
	CanRunGeospatialAnalysis           bool `json:"canRunGeospatialAnalysis"`
	CanRunResearchAgent                bool `json:"canRunResearchAgent"`
	CanReviewResearchQuestionCandidate bool `json:"canReviewResearchQuestionCandidate"`
	CanCreateClaim                     bool `json:"canCreateClaim"`
	CanDisputeClaim                    bool `json:"canDisputeClaim"`
	CanLinkEvidence                    bool `json:"canLinkEvidence"`
	CanCreateQuestion                  bool `json:"canCreateQuestion"`
	CanManageSelectedQuestion          bool `json:"canManageSelectedQuestion"`
	CanAttachFinding                   bool `json:"canAttachFinding"`
	CanReviewFinding                   bool `json:"canReviewFinding"`
	CanReviewTemporalFinding           bool `json:"canReviewTemporalFinding"`
	CanReviewGeospatialFinding         bool `json:"canReviewGeospatialFinding"`
	CanAddNote                         bool `json:"canAddNote"`
	CanReviewIdentityCandidate         bool `json:"canReviewIdentityCandidate"`
	CanMergeIdentity                   bool `json:"canMergeIdentity"`
}

type WorkspaceEvidence struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Relation         string `json:"relation"`
	SourceID         string `json:"sourceId,omitempty"`
	SourceTitleAR    string `json:"sourceTitleAr,omitempty"`
	StatementID      string `json:"statementId,omitempty"`
	StatementTextAR  string `json:"statementTextAr,omitempty"`
	PassageID        string `json:"passageId,omitempty"`
	PassageTextAR    string `json:"passageTextAr,omitempty"`
	LocatorAR        string `json:"locatorAr,omitempty"`
	PageNumber       *int   `json:"pageNumber,omitempty"`
	ReviewStatus     string `json:"reviewStatus,omitempty"`
	DependencyStatus string `json:"dependencyStatus,omitempty"`
}

type WorkspaceClaim struct {
	ID          string              `json:"id"`
	SubjectType string              `json:"subjectType"`
	SubjectID   string              `json:"subjectId"`
	Predicate   string              `json:"predicate"`
	ObjectType  string              `json:"objectType"`
	ObjectID    string              `json:"objectId"`
	TimeFrom    string              `json:"timeFrom,omitempty"`
	TimeTo      string              `json:"timeTo,omitempty"`
	PlaceID     string              `json:"placeId,omitempty"`
	Status      string              `json:"status"`
	NotesAR     string              `json:"notesAr,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Evidence    []WorkspaceEvidence `json:"evidence"`
}

type WorkspaceSource struct {
	ID               string `json:"id"`
	TitleAR          string `json:"titleAr"`
	AuthorAR         string `json:"authorAr,omitempty"`
	SourceType       string `json:"sourceType"`
	DependencyStatus string `json:"dependencyStatus"`
	Visibility       string `json:"visibility"`
	CitationAR       string `json:"citationAr,omitempty"`
	LocationAR       string `json:"locationAr,omitempty"`
	PassageCount     int    `json:"passageCount"`
	StatementCount   int    `json:"statementCount"`
}

type WorkspaceTreeNode struct {
	ID          string `json:"id"`
	PersonID    string `json:"personId"`
	DisplayName string `json:"displayNameAr"`
	SortOrder   int    `json:"sortOrder"`
}

type WorkspaceTreeRelationship struct {
	ID              string `json:"id"`
	SubjectPersonID string `json:"subjectPersonId"`
	ObjectPersonID  string `json:"objectPersonId"`
	Predicate       string `json:"predicate"`
	Status          string `json:"status"`
	SourceID        string `json:"sourceId,omitempty"`
	SourceTitleAR   string `json:"sourceTitleAr,omitempty"`
}

type WorkspaceTreeContext struct {
	TreeID        string                      `json:"treeId"`
	TreeNameAR    string                      `json:"treeNameAr"`
	Visibility    string                      `json:"visibility"`
	TreeVersionID string                      `json:"treeVersionId"`
	VersionNumber int                         `json:"versionNumber"`
	State         string                      `json:"state"`
	TargetPresent bool                        `json:"targetPresent"`
	Nodes         []WorkspaceTreeNode         `json:"nodes"`
	Relationships []WorkspaceTreeRelationship `json:"relationships"`
}

type WorkspaceTimelineEvent struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	LabelAR     string   `json:"labelAr"`
	DateFrom    string   `json:"dateFrom,omitempty"`
	DateTo      string   `json:"dateTo,omitempty"`
	Approximate bool     `json:"approximate"`
	Layer       string   `json:"layer"`
	Status      string   `json:"status"`
	PlaceID     string   `json:"placeId,omitempty"`
	ClaimID     string   `json:"claimId,omitempty"`
	SourceIDs   []string `json:"sourceIds,omitempty"`
}

type WorkspaceQuestion struct {
	ID            string    `json:"id"`
	TitleAR       string    `json:"titleAr"`
	DescriptionAR string    `json:"descriptionAr,omitempty"`
	Status        string    `json:"status"`
	Priority      string    `json:"priority"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ClaimCount    int       `json:"claimCount"`
	SourceCount   int       `json:"sourceCount"`
}

type WorkspaceDispute struct {
	ID            string    `json:"id"`
	TitleAR       string    `json:"titleAr"`
	DescriptionAR string    `json:"descriptionAr,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ClaimCount    int       `json:"claimCount"`
}

type WorkspaceNote struct {
	ID        string    `json:"id"`
	NoteAR    string    `json:"noteAr"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

type WorkspaceFinding struct {
	ID               string              `json:"id"`
	FindingType      string              `json:"findingType"`
	TitleAR          string              `json:"titleAr"`
	ExplanationAR    string              `json:"explanationAr"`
	Status           string              `json:"status"`
	Severity         string              `json:"severity"`
	Signals          map[string]any      `json:"signals"`
	ClaimIDs         []string            `json:"claimIds"`
	EntityIDs        []string            `json:"entityIds"`
	Evidence         []WorkspaceEvidence `json:"evidence"`
	Traceability     string              `json:"traceability"`
	AlgorithmVersion string              `json:"algorithmVersion,omitempty"`
}

type WorkspaceIdentityCandidate struct {
	ID          string    `json:"id"`
	RunID       string    `json:"runId"`
	EntityType  string    `json:"entityType"`
	LeftID      string    `json:"leftEntityId"`
	RightID     string    `json:"rightEntityId"`
	LeftNameAR  string    `json:"leftNameAr"`
	RightNameAR string    `json:"rightNameAr"`
	MatchClass  string    `json:"matchClass"`
	Score       float64   `json:"score"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type WorkspaceSnapshot struct {
	Context            WorkspaceContext             `json:"context"`
	Permissions        WorkspacePermissions         `json:"permissions"`
	Claims             []WorkspaceClaim             `json:"claims"`
	Sources            []WorkspaceSource            `json:"sources"`
	TreeContexts       []WorkspaceTreeContext       `json:"treeContexts"`
	MapFeatures        []geography.Feature          `json:"mapFeatures"`
	Timeline           []WorkspaceTimelineEvent     `json:"timeline"`
	Questions          []WorkspaceQuestion          `json:"questions"`
	Disputes           []WorkspaceDispute           `json:"disputes"`
	Notes              []WorkspaceNote              `json:"notes"`
	Findings           []WorkspaceFinding           `json:"findings"`
	IdentityCandidates []WorkspaceIdentityCandidate `json:"identityCandidates"`
	History            []ResearchRunSummary         `json:"history"`
}

type workspaceExecutor interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) Workspace(ctx context.Context, input WorkspaceInput, actorID string) (WorkspaceSnapshot, error) {
	if err := s.ready(); err != nil {
		return WorkspaceSnapshot{}, err
	}
	input, err := validateWorkspaceInput(input)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	actorUUID, err := parseOptionalUUID(actorID)
	if err != nil {
		return WorkspaceSnapshot{}, ErrForbidden
	}
	questionUUID, _ := uuid.Parse(input.QuestionID)
	canResearch, err := s.workspaceHasResearchRole(ctx, actorUUID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	canModerate, err := s.workspaceHasModeratorRole(ctx, actorUUID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	// The visibility policy this actor reads under, built from the role the two
	// queries above already resolved rather than from a second role lookup, so
	// the scope the workspace applies to a person's aliases is the scope the rest
	// of this snapshot is built with. It carries the research flag and NOT the
	// reference-write flag: the reference families are not part of this change,
	// and widening what a research role may read of a family is not this
	// function's decision to make.
	//
	// WithResearch is the right constructor here because a caller without a
	// session never carries the research role however the flag is passed, which
	// is the same rule the policy itself states.
	policy, err := visibility.WithResearch(actorUUID.String(), canResearch)
	if err != nil {
		return WorkspaceSnapshot{}, ErrForbidden
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	question, questionCreator, err := loadWorkspaceQuestion(ctx, tx, questionUUID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	entityType, entityID, entityName, aliases, err := resolveWorkspaceEntity(ctx, tx, policy, questionUUID, input)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	claims, err := loadWorkspaceClaims(ctx, tx, questionUUID, entityType, entityID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	evidence, err := loadWorkspaceEvidence(ctx, tx, questionUUID, entityType, entityID, actorUUID, canResearch)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	for index := range claims {
		claims[index].Evidence = evidence[claims[index].ID]
		if claims[index].Evidence == nil {
			claims[index].Evidence = []WorkspaceEvidence{}
		}
	}
	sources, err := loadWorkspaceSources(ctx, tx, questionUUID, entityType, entityID, actorUUID, canResearch)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	treeContexts, err := loadWorkspaceTrees(ctx, tx, input, entityType, entityID, actorUUID, canResearch)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	questions, err := loadWorkspaceQuestions(ctx, tx, questionUUID, actorUUID, canResearch)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	disputes, err := loadWorkspaceDisputes(ctx, tx, questionUUID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	notes, err := loadWorkspaceNotes(ctx, tx, questionUUID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	findings, err := loadWorkspaceFindings(ctx, tx, questionUUID, actorUUID, evidence, canResearch || actorUUID == questionCreator)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	candidates, err := loadWorkspaceCandidates(ctx, tx, entityType, entityID, canResearch)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	history, err := listRuns(ctx, tx, questionUUID, canResearch)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	history, err = s.filterResearchRunSummariesWithExecutor(ctx, tx, actorUUID.String(), history)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	mapFeatures, err := s.workspaceMapFeatures(ctx, entityType, entityID, claims)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	timeline := buildWorkspaceTimeline(ctx, tx, entityType, entityID, claims, mapFeatures)
	if err := tx.Commit(ctx); err != nil {
		return WorkspaceSnapshot{}, err
	}
	return WorkspaceSnapshot{
		Context:     WorkspaceContext{QuestionID: question.ID, QuestionTitleAR: question.TitleAR, QuestionDetailAR: question.DescriptionAR, QuestionStatus: question.Status, QuestionPriority: question.Priority, EntityType: entityType, EntityID: entityID, EntityNameAR: entityName, EntityAliasesAR: aliases, TreeID: input.TreeID, TreeVersionID: input.TreeVersionID},
		Permissions: buildWorkspacePermissions(actorUUID, questionCreator, canResearch, canModerate),
		Claims:      claims, Sources: sources, TreeContexts: treeContexts, MapFeatures: mapFeatures, Timeline: timeline, Questions: questions, Disputes: disputes, Notes: notes, Findings: findings, IdentityCandidates: candidates, History: history,
	}, nil
}

func validateWorkspaceInput(input WorkspaceInput) (WorkspaceInput, error) {
	input.QuestionID = strings.TrimSpace(input.QuestionID)
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.EntityID = strings.TrimSpace(input.EntityID)
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	if _, err := uuid.Parse(input.QuestionID); err != nil {
		return WorkspaceInput{}, ErrValidation
	}
	for _, value := range []struct{ target *string }{{&input.EntityID}, {&input.TreeID}, {&input.TreeVersionID}} {
		*value.target = strings.TrimSpace(*value.target)
		if *value.target != "" {
			if _, err := uuid.Parse(*value.target); err != nil {
				return WorkspaceInput{}, ErrValidation
			}
		}
	}
	if input.EntityID != "" && input.EntityType == "" {
		input.EntityType = "person"
	}
	if input.EntityType != "" && input.EntityID == "" {
		return WorkspaceInput{}, ErrValidation
	}
	if input.EntityType != "" && input.EntityType != "person" && input.EntityType != "family" && input.EntityType != "branch" {
		return WorkspaceInput{}, ErrValidation
	}
	return input, nil
}

func loadWorkspaceQuestion(ctx context.Context, q workspaceExecutor, questionID uuid.UUID) (WorkspaceQuestion, uuid.UUID, error) {
	var item WorkspaceQuestion
	var id, createdBy pgtype.UUID
	var description pgtype.Text
	if err := q.QueryRow(ctx, `SELECT id, title_ar, description_ar, status, priority, created_by, created_at, updated_at FROM open_questions WHERE id = $1`, questionID).Scan(&id, &item.TitleAR, &description, &item.Status, &item.Priority, &createdBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkspaceQuestion{}, uuid.Nil, ErrNotFound
		}
		return WorkspaceQuestion{}, uuid.Nil, err
	}
	item.ID = uuidText(id)
	item.DescriptionAR = textValue(description)
	return item, uuidOrNil(createdBy), nil
}

// resolveWorkspaceEntity resolves the workspace's subject and reads its names.
//
// The policy travels with it because the alias read needs one: a person's
// alternate spellings are only readable under the same two gates the people index
// applies, and that is a statement about the actor rather than about the subject.
// See workspaceEntityAliases.
func resolveWorkspaceEntity(ctx context.Context, q workspaceExecutor, policy visibility.Policy, questionID uuid.UUID, input WorkspaceInput) (string, string, string, []string, error) {
	entityType := input.EntityType
	entityID := input.EntityID
	if entityID == "" {
		var subjectType pgtype.Text
		var subjectID pgtype.UUID
		if err := q.QueryRow(ctx, `SELECT c.subject_type, c.subject_id FROM question_claims qc JOIN claims c ON c.id = qc.claim_id WHERE qc.question_id = $1 AND c.subject_type IN ('person', 'family', 'branch') ORDER BY qc.role, c.created_at LIMIT 1`, questionID).Scan(&subjectType, &subjectID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", nil, err
		}
		if !subjectType.Valid || !subjectID.Valid {
			if err := q.QueryRow(ctx, `SELECT entity_type, entity_id FROM question_entities WHERE question_id = $1 AND entity_type IN ('person', 'family', 'branch') ORDER BY entity_type, entity_id LIMIT 1`, questionID).Scan(&subjectType, &subjectID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return "", "", "", nil, err
			}
		}
		if subjectType.Valid && subjectID.Valid {
			entityType = textValue(subjectType)
			entityID = uuidText(subjectID)
		}
	}
	if entityID == "" {
		return "", "", "", []string{}, nil
	}
	name, err := workspaceEntityName(ctx, q, entityType, entityID)
	if err != nil {
		return "", "", "", nil, err
	}
	aliases, err := workspaceEntityAliases(ctx, q, policy, entityType, entityID)
	if err != nil {
		return "", "", "", nil, err
	}
	return entityType, entityID, name, aliases, nil
}

func workspaceEntityName(ctx context.Context, q workspaceExecutor, entityType, entityID string) (string, error) {
	id, err := uuid.Parse(entityID)
	if err != nil {
		return "", ErrValidation
	}
	var name pgtype.Text
	var query string
	switch entityType {
	case "person":
		query = `SELECT canonical_name_ar FROM people WHERE id = $1 AND merged_into_id IS NULL`
	case "family":
		query = `SELECT canonical_name_ar FROM families WHERE id = $1 AND merged_into_id IS NULL`
	case "branch":
		query = `SELECT canonical_name_ar FROM branches WHERE id = $1`
	default:
		return "", ErrValidation
	}
	if err := q.QueryRow(ctx, query, id).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return textValue(name), nil
}

// workspaceEntityAliases reads the alternate spellings of the workspace's
// subject.
//
// A PERSON's aliases are read through the same two gates internal/dictionary
// applies, and for the same reason: person_aliases can record the source a
// spelling was taken from, so an alias taken from a research-only source is a
// way to disclose a private source through a page that looks public. And a
// person the actor may not read is a person whose names it may not surface at
// all - not through the canonical name, and not through the list of spellings
// that follows it.
//
// The two gates, in the shape the dictionary uses:
//
//   - the person policy, so a research-only person the actor may not read does
//     not arrive through its aliases. Note that the research role is NOT a
//     blanket bypass over people (visibility.roleBypassesPeople is false), so
//     this is the same answer the people index gives.
//   - the alias's own source, with the rule that an alias carrying no source is
//     a PLATFORM record and stays. That is the asymmetry worth naming: "no
//     source" is not "no permission", it is a row the platform itself wrote.
//
// A research role keeps the view it had. visibility.Policy's source grants
// include a blanket pass for the research role, so an actor who may read a
// research-only source still sees the alias taken from it, and a research role
// therefore loses nothing here. What it loses is the aliases of a person it may
// not read - which is what the first gate says, and which is the last gap in
// Phase 19.
//
// A FAMILY's aliases are not scoped. family_aliases carries no source column,
// so there is nothing to scope and the rows follow their parent, exactly as
// internal/dictionary's own comment on that table says.
func workspaceEntityAliases(ctx context.Context, q workspaceExecutor, policy visibility.Policy, entityType, entityID string) ([]string, error) {
	id, err := uuid.Parse(entityID)
	if err != nil {
		return nil, ErrValidation
	}
	switch entityType {
	case "person":
		params := visibility.NewParams()
		personReference := params.Add(id)
		personPredicate := policy.PersonPredicate(params, personReference)
		aliasSource := policy.SourcePredicate(params, "pa.source_id")
		query := `SELECT pa.value_ar FROM person_aliases pa
			WHERE pa.person_id = ` + personReference + `
			  AND ` + personPredicate + `
			  AND (pa.source_id IS NULL OR ` + aliasSource + `)
			ORDER BY pa.created_at, pa.id`
		rows, err := q.Query(ctx, query, params.Args()...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := make([]string, 0)
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return items, rows.Err()
	case "family":
		rows, err := q.Query(ctx, `SELECT value_ar FROM family_aliases WHERE family_id = $1 ORDER BY created_at, id`, id)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := make([]string, 0)
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return items, rows.Err()
	default:
		return []string{}, nil
	}
}

// THE ENTITY RESTRICTION, AND WHY IT IS BUILT ONCE HERE.
//
// Three of this file's loaders - claims, evidence and sources - answer the same
// question about a different table: "of the material this question has, keep what
// concerns the workspace's subject". They used to answer it three times, and all
// three answers were built out of bare equalities against an argument that is
// NULL whenever the workspace resolved no subject:
//
//	($2 = '' OR ((c.subject_type = $2 AND c.subject_id = $3::uuid) OR ...))
//
// That shape is wrong twice over, and the two failures are different:
//
//   - It tests for the EMPTY STRING. The argument is not an empty string; it is
//     NULL, because a subject that was not resolved is passed as NULL. A
//     comparison of NULL with a string is NULL, never true, so a filter naming a
//     type and no id matched nothing at all and the workspace reported a question
//     with no claims as though the question had none. This is the same defect
//     `search_sources` in the research agent had.
//   - loadWorkspaceEvidence and loadWorkspaceSources had NO escape clause at all.
//     Their predicate was `c.subject_type = $n` with no "or no subject" branch, so
//     the ordinary case - a question whose subject did not resolve, which is a
//     question whose linked claims are about a place rather than a person -
//     silently excluded EVERY claim. The workspace then rendered claims with an
//     empty evidence list, which is exactly the shape of a claim that has no
//     evidence, and Phase 19's own criterion is that every finding is traceable
//     to one. A broken filter and an answer are not allowed to look alike.
//
// So the restriction is written once, in the form the rest of this file already
// uses for the same job - an explicit "is this argument absent" test, not an
// empty-string test - and the three loaders share it. The subject's type and id
// are the placeholders `$2` and `$3` in every query that uses it, so the fragment
// is one string rather than three that have to be read together to be believed.

// workspaceEntityRestriction is the shared restriction, for a claim row reached
// through the SQL expression claimReference. It names a claim's subject or object
// and asks whether it is the workspace's subject.
//
// A NULL $2 means the workspace resolved no subject, and that is "no restriction"
// rather than "exclude everything". A non-NULL $2 is a subject, and $3 is then
// non-NULL too, because workspaceEntityArgs refuses the half-specified pair
// before the query is built.
//
// The claim table is aliased vis_claim because the fragment is embedded in the
// caller's query: a caller's own `c` would shadow it, and the reference to the
// caller's row would then compare the inner claim's id with ITSELF and every claim
// would pass. That is not hypothetical - it is what the first version of this
// fragment did, and it made the restriction a no-op that widened rather than
// narrowed. The prefix is the same discipline internal/visibility uses for the
// same reason.
func workspaceEntityRestriction(claimReference string) string {
	return `(
		$2::text IS NULL
		OR EXISTS (
			SELECT 1 FROM claims vis_claim
			WHERE vis_claim.id = ` + claimReference + `
			  AND ((vis_claim.subject_type = $2 AND vis_claim.subject_id = $3::uuid)
			    OR (vis_claim.object_type = $2 AND vis_claim.object_id = $3::uuid))
		)
	)`
}

// workspaceEntityArgs turns the resolved subject into the two arguments the
// restriction above reads, and refuses a subject that is only half named.
//
// A type with no id cannot be satisfied by any claim, so a query that carried it
// would return nothing and the caller would read that as "this question has no
// material". It is an error instead, at the point the arguments are built, which
// is before any of the three loaders can turn a mistake into an empty answer.
//
// The pair is produced by resolveWorkspaceEntity, which sets both halves or
// neither, and by validateWorkspaceInput, which refuses a request that names one
// half. This function is the third gate, and it is the one that does not depend on
// either of the others: a future caller of any of the three loaders gets the
// refusal rather than an empty list.
func workspaceEntityArgs(entityType, entityID string) (any, any, error) {
	if entityType == "" && entityID == "" {
		return nil, nil, nil
	}
	if entityType == "" || entityID == "" {
		return nil, nil, ErrValidation
	}
	return entityType, entityID, nil
}

func loadWorkspaceClaims(ctx context.Context, q workspaceExecutor, questionID uuid.UUID, entityType, entityID string) ([]WorkspaceClaim, error) {
	entityTypeArg, entityIDArg, err := workspaceEntityArgs(entityType, entityID)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT c.id, c.subject_type, c.subject_id, c.predicate, c.object_type, c.object_id,
		       c.time_from, c.time_to, c.place_id, c.status, c.notes_ar, c.created_at, c.updated_at
		FROM claims c
		WHERE c.id IN (
			SELECT claim_id FROM question_claims WHERE question_id = $1
			UNION
			SELECT fc.claim_id FROM finding_claims fc
			JOIN question_findings qf ON qf.finding_id = fc.finding_id
			WHERE qf.question_id = $1
		)
		  AND `+workspaceEntityRestriction("c.id")+`
		ORDER BY c.updated_at DESC, c.id
		LIMIT 60
	`, questionID, entityTypeArg, entityIDArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceClaim, 0)
	for rows.Next() {
		var item WorkspaceClaim
		var id, subjectID, objectID, placeID pgtype.UUID
		var timeFrom, timeTo pgtype.Date
		var notes pgtype.Text
		if err := rows.Scan(&id, &item.SubjectType, &subjectID, &item.Predicate, &item.ObjectType, &objectID, &timeFrom, &timeTo, &placeID, &item.Status, &notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.SubjectID = uuidText(subjectID)
		item.ObjectID = uuidText(objectID)
		item.PlaceID = uuidText(placeID)
		item.TimeFrom = dateText(timeFrom)
		item.TimeTo = dateText(timeTo)
		item.NotesAR = textValue(notes)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadWorkspaceEvidence(ctx context.Context, q workspaceExecutor, questionID uuid.UUID, entityType, entityID string, actorUUID uuid.UUID, canResearch bool) (map[string][]WorkspaceEvidence, error) {
	entityTypeArg, entityIDArg, err := workspaceEntityArgs(entityType, entityID)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT ce.claim_id, ce.id, 'evidence', ce.relation,
		       COALESCE(ss.source_id, sp.source_id), s.title_ar, ss.id, ss.statement_text_ar,
		       sp.id, sp.text_ar, COALESCE(ss.locator_ar, sp.locator_ar), sp.page_number, ss.review_status,
		       CASE
		         WHEN s.id IS NULL THEN ''
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'confirmed') THEN 'derived'
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'needs_review') THEN 'likely_dependent'
		         ELSE s.dependency_status
		       END
		FROM claim_evidence ce
		LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
		LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
		LEFT JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
		WHERE ce.claim_id IN (
			SELECT claim_id FROM question_claims WHERE question_id = $1
			UNION
			SELECT fc.claim_id FROM finding_claims fc
			JOIN question_findings qf ON qf.finding_id = fc.finding_id
			WHERE qf.question_id = $1
		)
		  AND `+workspaceEntityRestriction("ce.claim_id")+`
		  AND (s.visibility = 'public' OR ($4::uuid IS NOT NULL AND (s.created_by = $4 OR $5)))
		  AND (ss.review_status = 'accepted' OR ss.id IS NULL OR $5)
		UNION ALL
		SELECT cce.claim_id, cce.id, 'counter_evidence', 'contradicts',
		       COALESCE(ss.source_id, sp.source_id), s.title_ar, ss.id, ss.statement_text_ar,
		       sp.id, sp.text_ar, COALESCE(ss.locator_ar, sp.locator_ar), sp.page_number, ss.review_status,
		       CASE
		         WHEN s.id IS NULL THEN ''
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'confirmed') THEN 'derived'
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'needs_review') THEN 'likely_dependent'
		         ELSE s.dependency_status
		       END
		FROM claim_counter_evidence cce
		LEFT JOIN source_statements ss ON ss.id = cce.source_statement_id
		LEFT JOIN source_passages sp ON sp.id = cce.source_passage_id
		LEFT JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
		WHERE cce.claim_id IN (
			SELECT claim_id FROM question_claims WHERE question_id = $1
			UNION
			SELECT fc.claim_id FROM finding_claims fc
			JOIN question_findings qf ON qf.finding_id = fc.finding_id
			WHERE qf.question_id = $1
		)
		  AND `+workspaceEntityRestriction("cce.claim_id")+`
		  AND (s.visibility = 'public' OR ($4::uuid IS NOT NULL AND (s.created_by = $4 OR $5)))
		  AND (ss.review_status = 'accepted' OR ss.id IS NULL OR $5)
	`, questionID, entityTypeArg, entityIDArg, nullableUUID(actorUUID), canResearch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[string][]WorkspaceEvidence)
	seen := make(map[string]map[string]struct{})
	for rows.Next() {
		var claimID, id, sourceID, statementID, passageID pgtype.UUID
		var kind, relation, sourceTitle, statementText, passageText, locator, reviewStatus, dependencyStatus pgtype.Text
		var pageNumber pgtype.Int4
		if err := rows.Scan(&claimID, &id, &kind, &relation, &sourceID, &sourceTitle, &statementID, &statementText, &passageID, &passageText, &locator, &pageNumber, &reviewStatus, &dependencyStatus); err != nil {
			return nil, err
		}
		item := WorkspaceEvidence{ID: uuidText(id), Kind: textValue(kind), Relation: textValue(relation), SourceID: uuidText(sourceID), SourceTitleAR: textValue(sourceTitle), StatementID: uuidText(statementID), StatementTextAR: textValue(statementText), PassageID: uuidText(passageID), PassageTextAR: textValue(passageText), LocatorAR: textValue(locator), ReviewStatus: textValue(reviewStatus), DependencyStatus: textValue(dependencyStatus)}
		if pageNumber.Valid {
			value := int(pageNumber.Int32)
			item.PageNumber = &value
		}
		key := uuidText(claimID)
		if seen[key] == nil {
			seen[key] = make(map[string]struct{})
		}
		if _, exists := seen[key][item.ID]; exists {
			continue
		}
		seen[key][item.ID] = struct{}{}
		items[key] = append(items[key], item)
	}
	return items, rows.Err()
}

func loadWorkspaceSources(ctx context.Context, q workspaceExecutor, questionID uuid.UUID, entityType, entityID string, actorUUID uuid.UUID, canResearch bool) ([]WorkspaceSource, error) {
	entityTypeArg, entityIDArg, err := workspaceEntityArgs(entityType, entityID)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT s.id, s.title_ar, s.author_ar, s.source_type,
		       CASE
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'confirmed') THEN 'derived'
		         WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'needs_review') THEN 'likely_dependent'
		         ELSE s.dependency_status
		       END,
		       s.visibility, s.citation_ar, s.location_ar,
		       (SELECT count(*) FROM source_passages sp WHERE sp.source_id = s.id),
		       (SELECT count(*) FROM source_statements ss WHERE ss.source_id = s.id)
		FROM sources s
		WHERE (s.visibility = 'public' OR ($4::uuid IS NOT NULL AND (s.created_by = $4 OR $5)))
		  AND (
			s.id IN (SELECT source_id FROM question_sources WHERE question_id = $1)
			OR EXISTS (
				SELECT 1 FROM claim_evidence ce
				LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
				LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
				WHERE ce.claim_id IN (
					SELECT claim_id FROM question_claims WHERE question_id = $1
					UNION
					SELECT fc.claim_id FROM finding_claims fc
					JOIN question_findings qf ON qf.finding_id = fc.finding_id
					WHERE qf.question_id = $1
				)
				  AND `+workspaceEntityRestriction("ce.claim_id")+`
				  AND COALESCE(ss.source_id, sp.source_id) = s.id
			)
			OR EXISTS (
				SELECT 1 FROM claim_counter_evidence ce
				LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
				LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
				WHERE ce.claim_id IN (
					SELECT claim_id FROM question_claims WHERE question_id = $1
					UNION
					SELECT fc.claim_id FROM finding_claims fc
					JOIN question_findings qf ON qf.finding_id = fc.finding_id
					WHERE qf.question_id = $1
				)
				  AND `+workspaceEntityRestriction("ce.claim_id")+`
				  AND COALESCE(ss.source_id, sp.source_id) = s.id
			)
		  )
		ORDER BY s.title_ar
		LIMIT 50
	`, questionID, entityTypeArg, entityIDArg, nullableUUID(actorUUID), canResearch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceSource, 0)
	for rows.Next() {
		var item WorkspaceSource
		var id pgtype.UUID
		var author, citation, location pgtype.Text
		if err := rows.Scan(&id, &item.TitleAR, &author, &item.SourceType, &item.DependencyStatus, &item.Visibility, &citation, &location, &item.PassageCount, &item.StatementCount); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.AuthorAR = textValue(author)
		item.CitationAR = textValue(citation)
		item.LocationAR = textValue(location)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadWorkspaceTrees(ctx context.Context, q workspaceExecutor, input WorkspaceInput, entityType, entityID string, actorUUID uuid.UUID, canResearch bool) ([]WorkspaceTreeContext, error) {
	items := make([]WorkspaceTreeContext, 0)
	if entityType != "person" || entityID == "" {
		return items, nil
	}
	personID, _ := uuid.Parse(entityID)
	rows, err := q.Query(ctx, `
		SELECT t.id, t.name_ar, t.visibility, tv.id, tv.version_number, tv.state,
		       EXISTS (SELECT 1 FROM tree_nodes target_node WHERE target_node.tree_version_id = tv.id AND target_node.person_id = $1)
		FROM tree_nodes tn
		JOIN tree_versions tv ON tv.id = tn.tree_version_id
		JOIN trees t ON t.id = tv.tree_id
		WHERE tn.person_id = $1
		  AND (t.visibility = 'public' OR ($2::uuid IS NOT NULL AND (t.owner_id = $2 OR EXISTS (
			SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2
		  ))))
		  AND ($3 = '' OR t.id = $4::uuid)
		  AND ($5 = '' OR tv.id = $6::uuid)
		ORDER BY (tv.state = 'published') DESC, tv.version_number DESC
		LIMIT 3
	`, personID, nullableUUID(actorUUID), input.TreeID, nullableString(input.TreeID), input.TreeVersionID, nullableString(input.TreeVersionID))
	if err != nil {
		return nil, err
	}
	versions := make([]WorkspaceTreeContext, 0)
	for rows.Next() {
		var item WorkspaceTreeContext
		var treeID, versionID pgtype.UUID
		if err := rows.Scan(&treeID, &item.TreeNameAR, &item.Visibility, &versionID, &item.VersionNumber, &item.State, &item.TargetPresent); err != nil {
			rows.Close()
			return nil, err
		}
		item.TreeID = uuidText(treeID)
		item.TreeVersionID = uuidText(versionID)
		versions = append(versions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, item := range versions {
		versionID, parseErr := uuid.Parse(item.TreeVersionID)
		if parseErr != nil {
			return nil, ErrValidation
		}
		nodes, nodeErr := loadWorkspaceTreeNodes(ctx, q, versionID)
		if nodeErr != nil {
			return nil, nodeErr
		}
		relationships, relationshipErr := loadWorkspaceTreeRelationships(ctx, q, versionID, actorUUID, canResearch)
		if relationshipErr != nil {
			return nil, relationshipErr
		}
		item.Nodes = nodes
		item.Relationships = relationships
		items = append(items, item)
	}
	return items, nil
}

func loadWorkspaceTreeNodes(ctx context.Context, q workspaceExecutor, versionID uuid.UUID) ([]WorkspaceTreeNode, error) {
	rows, err := q.Query(ctx, `SELECT id, person_id, display_name_ar, sort_order FROM tree_nodes WHERE tree_version_id = $1 ORDER BY sort_order, display_name_ar LIMIT 100`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceTreeNode, 0)
	for rows.Next() {
		var item WorkspaceTreeNode
		var id, personID pgtype.UUID
		if err := rows.Scan(&id, &personID, &item.DisplayName, &item.SortOrder); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.PersonID = uuidText(personID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadWorkspaceTreeRelationships(ctx context.Context, q workspaceExecutor, versionID, actorUUID uuid.UUID, canResearch bool) ([]WorkspaceTreeRelationship, error) {
	rows, err := q.Query(ctx, `
		SELECT tr.id, sn.person_id, object_node.person_id, tr.predicate, tr.status,
		       CASE WHEN s.id IS NULL OR s.visibility = 'public' OR ($2::uuid IS NOT NULL AND (s.created_by = $2 OR $3)) THEN tr.source_id ELSE NULL END,
		       CASE WHEN s.id IS NULL OR s.visibility = 'public' OR ($2::uuid IS NOT NULL AND (s.created_by = $2 OR $3)) THEN s.title_ar ELSE NULL END
		FROM tree_relationships tr
		JOIN tree_nodes sn ON sn.id = tr.subject_node_id
		JOIN tree_nodes object_node ON object_node.id = tr.object_node_id
		LEFT JOIN sources s ON s.id = tr.source_id
		WHERE tr.tree_version_id = $1
		ORDER BY tr.created_at
		LIMIT 200
	`, versionID, nullableUUID(actorUUID), canResearch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceTreeRelationship, 0)
	for rows.Next() {
		var item WorkspaceTreeRelationship
		var id, subjectID, objectID, sourceID pgtype.UUID
		var sourceTitle pgtype.Text
		if err := rows.Scan(&id, &subjectID, &objectID, &item.Predicate, &item.Status, &sourceID, &sourceTitle); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.SubjectPersonID = uuidText(subjectID)
		item.ObjectPersonID = uuidText(objectID)
		item.SourceID = uuidText(sourceID)
		item.SourceTitleAR = textValue(sourceTitle)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) workspaceMapFeatures(ctx context.Context, entityType, entityID string, claims []WorkspaceClaim) ([]geography.Feature, error) {
	response, err := geography.NewService(s.Pool).List(ctx, geography.MapInput{})
	if err != nil {
		return nil, err
	}
	items := make([]geography.Feature, 0)
	placeIDs := make(map[string]struct{}, len(claims))
	if entityID == "" {
		for _, claim := range claims {
			if claim.PlaceID != "" {
				placeIDs[claim.PlaceID] = struct{}{}
			}
		}
	}
	for _, feature := range response.Features {
		if entityID == "" {
			if _, ok := placeIDs[feature.PlaceID]; !ok {
				continue
			}
		} else if feature.EntityID != entityID || (entityType != "" && feature.EntityType != entityType) {
			continue
		}
		if len(items) >= 40 {
			break
		}
		items = append(items, feature)
	}
	return items, nil
}

func buildWorkspaceTimeline(ctx context.Context, q workspaceExecutor, entityType, entityID string, claims []WorkspaceClaim, features []geography.Feature) []WorkspaceTimelineEvent {
	items := make([]WorkspaceTimelineEvent, 0)
	if entityType == "person" && entityID != "" {
		var id pgtype.UUID
		var birthFrom, birthTo, deathFrom, deathTo pgtype.Date
		if err := q.QueryRow(ctx, `SELECT id, birth_date_from, birth_date_to, death_date_from, death_date_to FROM people WHERE id = $1`, entityID).Scan(&id, &birthFrom, &birthTo, &deathFrom, &deathTo); err == nil {
			if birthFrom.Valid || birthTo.Valid {
				items = append(items, WorkspaceTimelineEvent{ID: "person:" + uuidText(id) + ":birth", Kind: "birth", LabelAR: "نطاق الميلاد", DateFrom: dateText(birthFrom), DateTo: dateText(birthTo), Approximate: birthFrom.Valid && birthTo.Valid && dateText(birthFrom) != dateText(birthTo), Layer: "source_statement", Status: "documented"})
			}
			if deathFrom.Valid || deathTo.Valid {
				items = append(items, WorkspaceTimelineEvent{ID: "person:" + uuidText(id) + ":death", Kind: "death", LabelAR: "نطاق الوفاة", DateFrom: dateText(deathFrom), DateTo: dateText(deathTo), Approximate: deathFrom.Valid && deathTo.Valid && dateText(deathFrom) != dateText(deathTo), Layer: "source_statement", Status: "documented"})
			}
		}
	}
	for _, claim := range claims {
		if claim.TimeFrom == "" && claim.TimeTo == "" {
			continue
		}
		items = append(items, WorkspaceTimelineEvent{ID: "claim:" + claim.ID, Kind: "claim", LabelAR: claim.Predicate + " / " + claim.Status, DateFrom: claim.TimeFrom, DateTo: claim.TimeTo, Approximate: claim.TimeFrom != "" && claim.TimeTo != "" && claim.TimeFrom != claim.TimeTo, Layer: "research_claim", Status: claim.Status, ClaimID: claim.ID})
	}
	for _, feature := range features {
		if feature.TimeFrom == "" && feature.TimeTo == "" {
			continue
		}
		kind := "presence"
		if feature.Kind == "migration" {
			kind = "migration"
		}
		items = append(items, WorkspaceTimelineEvent{ID: "map:" + feature.ID, Kind: kind, LabelAR: feature.PlaceName, DateFrom: feature.TimeFrom, DateTo: feature.TimeTo, Approximate: feature.Certainty == "approximate" || feature.Certainty == "uncertain", Layer: "tree_interpretation", Status: feature.Status, PlaceID: feature.PlaceID, SourceIDs: sourceIDsFromFeature(feature)})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].DateFrom == items[j].DateFrom {
			return items[i].ID < items[j].ID
		}
		return items[i].DateFrom < items[j].DateFrom
	})
	return items
}

func loadWorkspaceQuestions(ctx context.Context, q workspaceExecutor, questionID uuid.UUID, actorUUID uuid.UUID, canResearch bool) ([]WorkspaceQuestion, error) {
	rows, err := q.Query(ctx, `
		SELECT q.id, q.title_ar, q.description_ar, q.status, q.priority, q.created_at, q.updated_at,
		       (SELECT count(*) FROM question_claims qc WHERE qc.question_id = q.id),
		       (SELECT count(*) FROM question_sources qs JOIN sources vis_s ON vis_s.id = qs.source_id WHERE qs.question_id = q.id AND (vis_s.visibility = 'public' OR ($2::uuid IS NOT NULL AND (vis_s.created_by = $2 OR $3))))
		FROM open_questions q
		WHERE q.id = $1 OR q.id IN (
			SELECT DISTINCT qc2.question_id FROM question_claims qc1
			JOIN question_claims qc2 ON qc2.claim_id = qc1.claim_id
			WHERE qc1.question_id = $1
		) OR q.id IN (
			SELECT DISTINCT qf2.question_id FROM question_findings qf1
			JOIN question_findings qf2 ON qf2.finding_id = qf1.finding_id
			WHERE qf1.question_id = $1
		) OR q.id IN (
			SELECT DISTINCT qe2.question_id FROM question_entities qe1
			JOIN question_entities qe2 ON qe2.entity_type = qe1.entity_type AND qe2.entity_id = qe1.entity_id
			WHERE qe1.question_id = $1
		)
		ORDER BY q.updated_at DESC
		LIMIT 30
	`, questionID, nullableUUID(actorUUID), canResearch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceQuestion, 0)
	for rows.Next() {
		var item WorkspaceQuestion
		var id pgtype.UUID
		var description pgtype.Text
		if err := rows.Scan(&id, &item.TitleAR, &description, &item.Status, &item.Priority, &item.CreatedAt, &item.UpdatedAt, &item.ClaimCount, &item.SourceCount); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.DescriptionAR = textValue(description)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadWorkspaceDisputes(ctx context.Context, q workspaceExecutor, questionID uuid.UUID) ([]WorkspaceDispute, error) {
	rows, err := q.Query(ctx, `
		SELECT d.id, d.title_ar, d.description_ar, d.status, d.created_at, d.updated_at,
		       (SELECT count(*) FROM dispute_claims dc WHERE dc.dispute_id = d.id)
		FROM disputes d
		JOIN question_disputes qd ON qd.dispute_id = d.id
		WHERE qd.question_id = $1
		ORDER BY d.updated_at DESC
	`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceDispute, 0)
	for rows.Next() {
		var item WorkspaceDispute
		var id pgtype.UUID
		var description pgtype.Text
		if err := rows.Scan(&id, &item.TitleAR, &description, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.ClaimCount); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.DescriptionAR = textValue(description)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadWorkspaceNotes(ctx context.Context, q workspaceExecutor, questionID uuid.UUID) ([]WorkspaceNote, error) {
	rows, err := q.Query(ctx, `SELECT id, note_ar, created_by, created_at FROM question_notes WHERE question_id = $1 ORDER BY created_at DESC`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceNote, 0)
	for rows.Next() {
		var item WorkspaceNote
		var id, createdBy pgtype.UUID
		if err := rows.Scan(&id, &item.NoteAR, &createdBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.CreatedBy = uuidText(createdBy)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadWorkspaceFindings(ctx context.Context, q workspaceExecutor, questionID, actorID uuid.UUID, evidence map[string][]WorkspaceEvidence, canResearch bool) ([]WorkspaceFinding, error) {
	items := make([]WorkspaceFinding, 0)
	if !canResearch {
		return items, nil
	}
	rows, err := q.Query(ctx, `
		SELECT pf.id, pf.finding_type, pf.title_ar, pf.explanation_ar, pf.status, COALESCE(pf.severity, 'medium'), pf.signals, COALESCE(pf.algorithm_version, '')
		FROM platform_findings pf
		WHERE (pf.id IN (SELECT finding_id FROM question_findings WHERE question_id = $1)
		   OR pf.id IN (
			SELECT fc.finding_id FROM finding_claims fc
			JOIN question_claims qc ON qc.claim_id = fc.claim_id
			WHERE qc.question_id = $1
		   ))
		  AND (
			(pf.temporal_run_id IS NULL AND pf.geospatial_run_id IS NULL)
			OR EXISTS (
				SELECT 1 FROM temporal_analysis_runs tr
				JOIN trees t ON t.id = tr.tree_id
				WHERE tr.id = pf.temporal_run_id
				  AND (t.visibility = 'public' OR ($2::uuid IS NOT NULL AND (t.owner_id = $2 OR EXISTS (
					SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2
				  ))))
			)
			OR EXISTS (
				SELECT 1 FROM geospatial_intelligence_runs gr
				LEFT JOIN trees gt ON gt.id = gr.tree_id
				WHERE gr.id = pf.geospatial_run_id
				  AND (gr.entity_type <> 'source' OR EXISTS (
					SELECT 1 FROM sources gs
					WHERE gs.id = gr.entity_id AND (gs.visibility = 'public' OR ($2::uuid IS NOT NULL AND gs.created_by = $2))
				  ))
				  AND (gr.tree_id IS NULL OR (gt.visibility = 'public' OR ($2::uuid IS NOT NULL AND (gt.owner_id = $2 OR EXISTS (
					SELECT 1 FROM tree_collaborators gtc WHERE gtc.tree_id = gt.id AND gtc.user_id = $2
				  )))))
			)
		  )
		ORDER BY pf.updated_at DESC
		LIMIT 30
	`, questionID, nullableUUID(actorID))
	if err != nil {
		return nil, err
	}
	baseItems := make([]WorkspaceFinding, 0)
	for rows.Next() {
		var item WorkspaceFinding
		var id pgtype.UUID
		var severity, algorithm pgtype.Text
		var signals []byte
		if err := rows.Scan(&id, &item.FindingType, &item.TitleAR, &item.ExplanationAR, &item.Status, &severity, &signals, &algorithm); err != nil {
			rows.Close()
			return nil, err
		}
		item.ID = uuidText(id)
		item.Severity = textValue(severity)
		item.AlgorithmVersion = textValue(algorithm)
		if len(signals) > 0 {
			_ = json.Unmarshal(signals, &item.Signals)
		}
		if item.Signals == nil {
			item.Signals = map[string]any{}
		}
		baseItems = append(baseItems, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range baseItems {
		item := &baseItems[index]
		claimRows, claimErr := q.Query(ctx, `SELECT claim_id FROM finding_claims WHERE finding_id = $1 ORDER BY claim_id`, item.ID)
		if claimErr != nil {
			return nil, claimErr
		}
		for claimRows.Next() {
			var claimID pgtype.UUID
			if err := claimRows.Scan(&claimID); err != nil {
				claimRows.Close()
				return nil, err
			}
			claim := uuidText(claimID)
			item.ClaimIDs = append(item.ClaimIDs, claim)
			item.Evidence = append(item.Evidence, evidence[claim]...)
		}
		claimRows.Close()
		entityRows, entityErr := q.Query(ctx, `SELECT entity_id FROM finding_entities WHERE finding_id = $1 ORDER BY entity_type, entity_id`, item.ID)
		if entityErr != nil {
			return nil, entityErr
		}
		for entityRows.Next() {
			var entityID pgtype.UUID
			if err := entityRows.Scan(&entityID); err != nil {
				entityRows.Close()
				return nil, err
			}
			item.EntityIDs = append(item.EntityIDs, uuidText(entityID))
		}
		entityRows.Close()
		if len(item.Evidence) > 0 {
			item.Traceability = "evidence"
		} else {
			item.Traceability = "signals_only"
		}
	}
	return baseItems, nil
}

func loadWorkspaceCandidates(ctx context.Context, q workspaceExecutor, entityType, entityID string, canResearch bool) ([]WorkspaceIdentityCandidate, error) {
	items := make([]WorkspaceIdentityCandidate, 0)
	if !canResearch || entityID == "" {
		return items, nil
	}
	rows, err := q.Query(ctx, `
		SELECT id, run_id, entity_type, left_entity_id, right_entity_id, left_name_ar, right_name_ar, match_class, score, review_status, created_at
		FROM entity_resolution_candidates
		WHERE entity_type = $1 AND (left_entity_id = $2 OR right_entity_id = $2)
		ORDER BY created_at DESC
		LIMIT 20
	`, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item WorkspaceIdentityCandidate
		var id, runID, leftID, rightID pgtype.UUID
		if err := rows.Scan(&id, &runID, &item.EntityType, &leftID, &rightID, &item.LeftNameAR, &item.RightNameAR, &item.MatchClass, &item.Score, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID = uuidText(id)
		item.RunID = uuidText(runID)
		item.LeftID = uuidText(leftID)
		item.RightID = uuidText(rightID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func buildWorkspacePermissions(actorUUID, questionCreator uuid.UUID, canResearch, canModerate bool) WorkspacePermissions {
	registered := actorUUID != uuid.Nil
	return WorkspacePermissions{CanRunResearch: true, CanRunTemporalAnalysis: canResearch, CanRunGeospatialAnalysis: canResearch, CanRunResearchAgent: canResearch, CanReviewResearchQuestionCandidate: canResearch, CanCreateClaim: registered, CanDisputeClaim: registered, CanLinkEvidence: registered || canResearch, CanCreateQuestion: registered, CanManageSelectedQuestion: registered && (actorUUID == questionCreator || canResearch), CanAttachFinding: canResearch || actorUUID == questionCreator, CanReviewFinding: canResearch, CanReviewTemporalFinding: canResearch, CanReviewGeospatialFinding: canResearch, CanAddNote: registered && (actorUUID == questionCreator || canResearch), CanReviewIdentityCandidate: canResearch, CanMergeIdentity: canModerate}
}

func (s *Service) workspaceHasResearchRole(ctx context.Context, actorUUID uuid.UUID) (bool, error) {
	if actorUUID == uuid.Nil {
		return false, nil
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('researcher', 'moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func (s *Service) workspaceHasModeratorRole(ctx context.Context, actorUUID uuid.UUID) (bool, error) {
	if actorUUID == uuid.Nil {
		return false, nil
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func uuidOrNil(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}

func dateText(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func sourceIDsFromFeature(feature geography.Feature) []string {
	if feature.SourceID == "" {
		return nil
	}
	return []string{feature.SourceID}
}
