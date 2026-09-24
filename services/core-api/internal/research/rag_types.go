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
	ErrValidation          = errors.New("research query is invalid")
	ErrForbidden           = errors.New("research access is forbidden")
)

type QueryInput struct {
	Question   string `json:"question"`
	QuestionID string `json:"question_id"`
	SourceID   string `json:"source_id"`
	PersonID   string `json:"person_id"`
	PlaceID    string `json:"place_id"`
	FromYear   int    `json:"from_year"`
	ToYear     int    `json:"to_year"`
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

type QueryResult struct {
	RunID                string          `json:"runId"`
	Query                string          `json:"query"`
	NormalizedQuery      string          `json:"normalizedQuery"`
	QueryType            string          `json:"queryType"`
	Answer               string          `json:"answer"`
	InsufficientEvidence bool            `json:"insufficientEvidence"`
	ModelVersion         string          `json:"modelVersion,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	Citations            []Citation      `json:"citations"`
	Layers               LayeredEvidence `json:"layers"`
	Conflicts            []Conflict      `json:"conflicts"`
	Retrieval            RetrievalStats  `json:"retrieval"`
}

type Service struct {
	Pool *pgxpool.Pool
	AI   ai.Provider
}

func NewService(pool *pgxpool.Pool, provider ai.Provider) *Service {
	return &Service{Pool: pool, AI: provider}
}

type retrievalContext struct {
	Input        QueryInput
	Normalized   string
	ActorID      string
	Vector       []float32
	Passages     []Citation
	Claims       []Citation
	Trees        []Citation
	Findings     []Citation
	Questions    []Citation
	Conflicts    []Conflict
	QueryType    string
	ModelVersion string
}
