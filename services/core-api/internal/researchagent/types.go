package researchagent

import (
	"errors"
	"time"
)

const (
	PlannerVersion         = "research-agent-v1"
	AlgorithmVersion       = "research-agent-evidence-v1"
	QualificationPolicy    = "read-only-qualified-evidence-v1"
	MaximumEvidence        = 100
	MaximumSteps           = 11
	MaximumTerms           = 12
	ResolutionSucceeded    = "succeeded"
	ResolutionUnresolved   = "unresolved"
	StageDecompose         = "decompose_question"
	StageSearchSources     = "search_sources"
	StageSearchGraph       = "search_graph"
	StageInspectGeography  = "inspect_geography"
	StageInspectChronology = "inspect_chronology"
	StageCompareClaims     = "compare_claims"
	StageSourceDependency  = "inspect_source_dependency"
	StageCounterEvidence   = "retrieve_counter_evidence"
	StageEvidencePackage   = "generate_evidence_package"
	StageMissingEvidence   = "identify_missing_evidence"
	StageRecommendation    = "recommend_next_investigation"
)

var (
	ErrDatabaseUnavailable = errors.New("research agent database is unavailable")
	ErrValidation          = errors.New("research agent input is invalid")
	ErrForbidden           = errors.New("research agent access is forbidden")
	ErrNotFound            = errors.New("research agent resource was not found")
	ErrConflict            = errors.New("research agent resource changed")
)

type RunInput struct {
	Question      string `json:"question"`
	QuestionID    string `json:"question_id"`
	EntityType    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	TreeID        string `json:"tree_id"`
	TreeVersionID string `json:"tree_version_id"`
	SourceID      string `json:"source_id"`
	PersonID      string `json:"person_id"`
	PlaceID       string `json:"place_id"`
	FromYear      int    `json:"from_year"`
	ToYear        int    `json:"to_year"`
}

type Run struct {
	ID                         string           `json:"id"`
	RequestedBy                string           `json:"requestedBy"`
	QuestionID                 string           `json:"questionId,omitempty"`
	Query                      string           `json:"query"`
	NormalizedQuery            string           `json:"normalizedQuery"`
	EntityType                 string           `json:"entityType"`
	EntityID                   string           `json:"entityId"`
	TreeID                     string           `json:"treeId,omitempty"`
	TreeVersionID              string           `json:"treeVersionId,omitempty"`
	Status                     string           `json:"status"`
	Resolution                 string           `json:"resolution"`
	ExecutionMode              string           `json:"executionMode"`
	PlannerVersion             string           `json:"plannerVersion"`
	AlgorithmVersion           string           `json:"algorithmVersion"`
	QualificationPolicyVersion string           `json:"qualificationPolicyVersion"`
	Report                     Report           `json:"report"`
	StepCount                  int              `json:"stepCount"`
	EvidenceCount              int              `json:"evidenceCount"`
	GapCount                   int              `json:"gapCount"`
	RecommendationCount        int              `json:"recommendationCount"`
	Error                      string           `json:"error,omitempty"`
	CreatedAt                  time.Time        `json:"createdAt"`
	StartedAt                  *time.Time       `json:"startedAt,omitempty"`
	CompletedAt                *time.Time       `json:"completedAt,omitempty"`
	UpdatedAt                  time.Time        `json:"updatedAt"`
	Steps                      []Step           `json:"steps"`
	Evidence                   []EvidenceRef    `json:"evidence"`
	Gaps                       []Gap            `json:"gaps"`
	Recommendations            []Recommendation `json:"recommendations"`
}

type Report struct {
	AnswerAR          string          `json:"answerAr"`
	Plan              []PlanStep      `json:"plan"`
	Terms             []string        `json:"terms"`
	Scope             Scope           `json:"scope"`
	EvidencePackage   EvidencePackage `json:"evidencePackage"`
	AllowedActions    []string        `json:"allowedActions"`
	RestrictedActions []string        `json:"restrictedActions"`
	UnresolvedReasons []string        `json:"unresolvedReasons"`
	GeneratedAt       time.Time       `json:"generatedAt"`
}

type Scope struct {
	EntityType    string `json:"entityType"`
	EntityID      string `json:"entityId"`
	TreeID        string `json:"treeId,omitempty"`
	TreeVersionID string `json:"treeVersionId,omitempty"`
	SourceID      string `json:"sourceId,omitempty"`
	PersonID      string `json:"personId,omitempty"`
	PlaceID       string `json:"placeId,omitempty"`
	FromYear      int    `json:"fromYear,omitempty"`
	ToYear        int    `json:"toYear,omitempty"`
}

type PlanStep struct {
	Order         int    `json:"order"`
	Stage         string `json:"stage"`
	Tool          string `json:"tool"`
	ReadOnly      bool   `json:"readOnly"`
	DescriptionAR string `json:"descriptionAr"`
}

type Step struct {
	ID            string         `json:"id"`
	Order         int            `json:"order"`
	Stage         string         `json:"stage"`
	Tool          string         `json:"tool"`
	Status        string         `json:"status"`
	Input         map[string]any `json:"input"`
	Output        map[string]any `json:"output"`
	EvidenceCount int            `json:"evidenceCount"`
	Error         string         `json:"error,omitempty"`
	StartedAt     time.Time      `json:"startedAt"`
	CompletedAt   *time.Time     `json:"completedAt,omitempty"`
}

type EvidenceRef struct {
	ID            string         `json:"id"`
	StepID        string         `json:"stepId,omitempty"`
	Layer         string         `json:"layer"`
	Stance        string         `json:"stance"`
	ReferenceType string         `json:"referenceType"`
	ReferenceID   string         `json:"referenceId"`
	SourceID      string         `json:"sourceId,omitempty"`
	StatementID   string         `json:"statementId,omitempty"`
	ClaimID       string         `json:"claimId,omitempty"`
	Excerpt       string         `json:"excerpt"`
	Metadata      map[string]any `json:"metadata"`
}

type EvidencePackage struct {
	Total           int           `json:"total"`
	SupportCount    int           `json:"supportCount"`
	CounterCount    int           `json:"counterCount"`
	ContextCount    int           `json:"contextCount"`
	HypothesisCount int           `json:"hypothesisCount"`
	SourceCount     int           `json:"sourceCount"`
	Evidence        []EvidenceRef `json:"evidence"`
}

type Gap struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	DescriptionAR string         `json:"descriptionAr"`
	Severity      string         `json:"severity"`
	Status        string         `json:"status"`
	Metadata      map[string]any `json:"metadata"`
}

type Recommendation struct {
	ID          string         `json:"id"`
	Action      string         `json:"action"`
	RationaleAR string         `json:"rationaleAr"`
	Priority    string         `json:"priority"`
	Status      string         `json:"status"`
	Metadata    map[string]any `json:"metadata"`
}
