package research

import (
	"errors"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("research database is unavailable")
	ErrAIUnavailable       = errors.New("research AI service is unavailable")
	ErrGraphUnavailable    = errors.New("research graph retrieval is unavailable")
	ErrValidation          = errors.New("research query is invalid")
	ErrForbidden           = errors.New("research access is forbidden")
	ErrNotFound            = errors.New("research resource is not found")
)

type QueryInput struct {
	Question       string `json:"question"`
	QuestionID     string `json:"question_id"`
	EntityType     string `json:"entity_type,omitempty"`
	EntityID       string `json:"entity_id,omitempty"`
	TreeID         string `json:"tree_id,omitempty"`
	TreeVersionID  string `json:"tree_version_id,omitempty"`
	SourceID       string `json:"source_id"`
	PersonID       string `json:"person_id"`
	PlaceID        string `json:"place_id"`
	FromYear       int    `json:"from_year"`
	ToYear         int    `json:"to_year"`
	GraphOperation string `json:"graph_operation,omitempty"`
	GraphStartType string `json:"graph_start_type,omitempty"`
	GraphStartID   string `json:"graph_start_id,omitempty"`
	GraphEndType   string `json:"graph_end_type,omitempty"`
	GraphEndID     string `json:"graph_end_id,omitempty"`
	GraphMaxDepth  int    `json:"graph_max_depth,omitempty"`
}

type Score struct {
	Lexical  float64 `json:"lexical,omitempty"`
	Vector   float64 `json:"vector,omitempty"`
	Combined float64 `json:"combined,omitempty"`
	Rerank   float64 `json:"rerank,omitempty"`
}

type Citation struct {
	Layer        Layer  `json:"layer"`
	Type         string `json:"type"`
	ID           string `json:"id"`
	SourceID     string `json:"sourceId,omitempty"`
	PassageID    string `json:"passageId,omitempty"`
	StatementID  string `json:"statementId,omitempty"`
	ClaimID      string `json:"claimId,omitempty"`
	FindingID    string `json:"findingId,omitempty"`
	QuestionID   string `json:"questionId,omitempty"`
	Title        string `json:"title"`
	Excerpt      string `json:"excerpt"`
	LocatorAR    string `json:"locatorAr,omitempty"`
	PageNumber   *int   `json:"pageNumber,omitempty"`
	ReviewStatus string `json:"reviewStatus,omitempty"`
	Status       string `json:"status,omitempty"`
	Rank         int    `json:"rank"`
	Score        Score  `json:"score"`
}

type LayeredEvidence struct {
	SourceStatements    []Citation `json:"sourceStatements"`
	ResearchClaims      []Citation `json:"researchClaims"`
	TreeInterpretations []Citation `json:"treeInterpretations"`
	PlatformFindings    []Citation `json:"platformFindings"`
	OpenQuestions       []Citation `json:"openQuestions"`
}

type Conflict struct {
	Type        string   `json:"type"`
	LeftID      string   `json:"leftId"`
	RightID     string   `json:"rightId"`
	Status      string   `json:"status"`
	Explanation string   `json:"explanation"`
	SourceIDs   []string `json:"sourceIds,omitempty"`
}

type RetrievalStats struct {
	LexicalCandidates  int `json:"lexicalCandidates"`
	VectorCandidates   int `json:"vectorCandidates"`
	FusedCandidates    int `json:"fusedCandidates"`
	RerankedCandidates int `json:"rerankedCandidates"`
	EvidenceCount      int `json:"evidenceCount"`
}

type GraphTreeScope struct {
	TreeID        string `json:"treeId,omitempty"`
	TreeVersionID string `json:"treeVersionId,omitempty"`
	VersionNumber int    `json:"versionNumber,omitempty"`
	VersionState  string `json:"versionState,omitempty"`
}

type GraphNode struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Label      string `json:"label,omitempty"`
	PersonID   string `json:"personId,omitempty"`
	TreeNodeID string `json:"treeNodeId,omitempty"`
	Position   int    `json:"position"`
}

type GraphEdge struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	FromNodeID         string `json:"fromNodeId"`
	ToNodeID           string `json:"toNodeId"`
	PathFromNodeID     string `json:"pathFromNodeId,omitempty"`
	PathToNodeID       string `json:"pathToNodeId,omitempty"`
	Predicate          string `json:"predicate,omitempty"`
	Status             string `json:"status,omitempty"`
	Certainty          string `json:"certainty,omitempty"`
	SourceID           string `json:"sourceId,omitempty"`
	ClaimID            string `json:"claimId,omitempty"`
	StatementID        string `json:"statementId,omitempty"`
	PassageID          string `json:"passageId,omitempty"`
	TreeRelationshipID string `json:"treeRelationshipId,omitempty"`
	MigrationEventID   string `json:"migrationEventId,omitempty"`
	FromPlaceID        string `json:"fromPlaceId,omitempty"`
	ToPlaceID          string `json:"toPlaceId,omitempty"`
	Position           int    `json:"position"`
}

type GraphEvidenceRef struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Layer        string `json:"layer,omitempty"`
	Relation     string `json:"relation,omitempty"`
	SourceID     string `json:"sourceId,omitempty"`
	ClaimID      string `json:"claimId,omitempty"`
	StatementID  string `json:"statementId,omitempty"`
	PassageID    string `json:"passageId,omitempty"`
	ReviewStatus string `json:"reviewStatus,omitempty"`
	Status       string `json:"status,omitempty"`
	Certainty    string `json:"certainty,omitempty"`
	Title        string `json:"title,omitempty"`
	Excerpt      string `json:"excerpt,omitempty"`
	LocatorAR    string `json:"locatorAr,omitempty"`
	PageNumber   *int   `json:"pageNumber,omitempty"`
}

type GraphPath struct {
	ID               string             `json:"id"`
	Operation        string             `json:"operation"`
	Nodes            []GraphNode        `json:"nodes"`
	Edges            []GraphEdge        `json:"edges"`
	EvidenceRefs     []GraphEvidenceRef `json:"evidenceRefs"`
	Status           string             `json:"status"`
	Explanation      string             `json:"explanation"`
	Depth            int                `json:"depth"`
	Truncated        bool               `json:"truncated"`
	EvidenceBacked   bool               `json:"evidenceBacked"`
	StructuralOnly   bool               `json:"structuralOnly"`
	TreeScope        GraphTreeScope     `json:"treeScope"`
	AlgorithmVersion string             `json:"algorithmVersion"`
}

type GraphStats struct {
	Operation        string `json:"operation,omitempty"`
	PathCount        int    `json:"pathCount"`
	NodeCount        int    `json:"nodeCount"`
	EdgeCount        int    `json:"edgeCount"`
	EvidenceCount    int    `json:"evidenceCount"`
	Truncated        bool   `json:"truncated"`
	PathsTruncated   bool   `json:"pathsTruncated"`
	EdgesTruncated   bool   `json:"edgesTruncated"`
	MaxDepth         int    `json:"maxDepth"`
	AlgorithmVersion string `json:"algorithmVersion,omitempty"`
}

type RoutingInfo struct {
	Route                  string  `json:"route"`
	QueryType              string  `json:"queryType"`
	ReasonCode             string  `json:"reasonCode"`
	Model                  string  `json:"model"`
	Fallback               bool    `json:"fallback"`
	OperationalScore       float64 `json:"operationalScore"`
	SourceBearing          bool    `json:"sourceBearing"`
	PotentialContradiction bool    `json:"potentialContradiction"`
	ContinueInvestigation  bool    `json:"continueInvestigation"`
	SynthesisAttempted     bool    `json:"synthesisAttempted"`
}

type QueryResult struct {
	RunID                string          `json:"runId"`
	Query                string          `json:"query"`
	NormalizedQuery      string          `json:"normalizedQuery"`
	QueryType            string          `json:"queryType"`
	Answer               string          `json:"answer"`
	InsufficientEvidence bool            `json:"insufficientEvidence"`
	ModelVersion         string          `json:"modelVersion,omitempty"`
	Routing              RoutingInfo     `json:"routing"`
	CreatedAt            time.Time       `json:"createdAt"`
	Citations            []Citation      `json:"citations"`
	Layers               LayeredEvidence `json:"layers"`
	Conflicts            []Conflict      `json:"conflicts"`
	Retrieval            RetrievalStats  `json:"retrieval"`
	GraphPaths           []GraphPath     `json:"graphPaths"`
	GraphStats           GraphStats      `json:"graphStats"`
}

type Service struct {
	Pool *pgxpool.Pool
	AI   ai.Provider
}

func NewService(pool *pgxpool.Pool, provider ai.Provider) *Service {
	return &Service{Pool: pool, AI: provider}
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

type retrievalContext struct {
	Input      QueryInput
	Normalized string
	ActorID    string
	Vector     []float32
	Passages   []Citation
	Claims     []Citation
	Trees      []Citation
	Findings   []Citation
	Questions  []Citation
	Conflicts  []Conflict
	QueryType  string
}
