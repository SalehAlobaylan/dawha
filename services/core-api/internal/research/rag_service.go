package research

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
)

const safeInsufficientAnswer = "الأدلة المتاحة غير كافية للإجابة بثقة؛ راجع المصادر أو وسّع نطاق البحث."

func (s *Service) Query(ctx context.Context, input QueryInput, actorID string) (QueryResult, error) {
	if s == nil || s.Pool == nil {
		return QueryResult{}, ErrDatabaseUnavailable
	}
	if s.AI == nil {
		return QueryResult{}, ErrAIUnavailable
	}
	input, err := validateQueryInput(input)
	if err != nil {
		return QueryResult{}, err
	}
	actorID = strings.TrimSpace(actorID)
	if actorID != "" {
		if _, err := uuid.Parse(actorID); err != nil {
			return QueryResult{}, ErrForbidden
		}
	}
	runID, createdAt, err := s.startRun(ctx, input, actorID)
	if err != nil {
		return QueryResult{}, err
	}
	result, err := s.execute(ctx, input, actorID)
	if err != nil {
		_ = s.failRun(ctx, runID, err)
		return QueryResult{}, err
	}
	if err := s.persistRun(ctx, runID, result); err != nil {
		_ = s.failRun(ctx, runID, err)
		return QueryResult{}, err
	}
	result.RunID = runID
	result.CreatedAt = createdAt
	return result, nil
}

func (s *Service) execute(ctx context.Context, input QueryInput, actorID string) (QueryResult, error) {
	if s == nil || s.Pool == nil {
		return QueryResult{}, ErrDatabaseUnavailable
	}
	if s.AI == nil {
		return QueryResult{}, ErrAIUnavailable
	}
	normalized := identity.NormalizeArabicName(input.Question)
	classification, err := s.AI.Classify(ctx, ai.ClassificationRequest{
		Text:   input.Question,
		Labels: []string{"source_evidence", "identity", "relationship", "geography", "general"},
	})
	if err != nil {
		return QueryResult{}, researchAIError(err)
	}
	queryType := "general"
	if len(classification.Candidates) > 0 && allowedQueryType(classification.Candidates[0].Label) {
		queryType = classification.Candidates[0].Label
	}
	embedding, err := s.AI.Embed(ctx, ai.EmbeddingRequest{Text: normalized, Dimensions: 1536})
	if err != nil {
		return QueryResult{}, researchAIError(err)
	}
	if len(embedding.Embedding) != 1536 {
		return QueryResult{}, ErrAIUnavailable
	}
	vector := make([]float32, len(embedding.Embedding))
	for index, value := range embedding.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return QueryResult{}, ErrAIUnavailable
		}
		vector[index] = float32(value)
	}
	retrieval := retrievalContext{Input: input, Normalized: normalized, ActorID: actorID, Vector: vector, QueryType: queryType, ModelVersion: embedding.Model}
	lexical, err := s.retrieveLexicalPassages(ctx, retrieval)
	if err != nil {
		return QueryResult{}, err
	}
	vectorResults, err := s.retrieveVectorPassages(ctx, retrieval)
	if err != nil {
		return QueryResult{}, err
	}
	passages, stats, err := s.fuseAndRerank(ctx, retrieval, lexical, vectorResults)
	if err != nil {
		return QueryResult{}, researchAIError(err)
	}
	retrieval.Passages = passages
	retrieval.Claims, err = s.retrieveClaims(ctx, retrieval)
	if err != nil {
		return QueryResult{}, err
	}
	retrieval.Trees, err = s.retrieveTreeInterpretations(ctx, retrieval)
	if err != nil {
		return QueryResult{}, err
	}
	retrieval.Findings, err = s.retrieveFindings(ctx, retrieval)
	if err != nil {
		return QueryResult{}, err
	}
	retrieval.Questions, err = s.retrieveQuestions(ctx, retrieval)
	if err != nil {
		return QueryResult{}, err
	}
	evidence := evidencePackage(retrieval)
	if err := evidence.Validate(); err != nil {
		return QueryResult{}, ErrValidation
	}
	conflicts := claimConflicts(retrieval.Claims)
	if len(passages) > 0 {
		detected, detectErr := s.detectStatementConflicts(ctx, retrieval.Passages)
		if detectErr != nil {
			return QueryResult{}, researchAIError(detectErr)
		}
		conflicts = append(conflicts, detected...)
	}
	insufficient := len(passages) == 0
	answer := safeInsufficientAnswer
	synthesisModel := ""
	if !insufficient {
		contexts := make([]ai.SourceContext, 0, min(len(passages), 20))
		for _, passage := range passages {
			if len(contexts) == 20 {
				break
			}
			contexts = append(contexts, ai.SourceContext{ID: passage.PassageID, Title: passage.Title, Text: passage.Excerpt})
		}
		response, researchErr := s.AI.ResearchQuery(ctx, ai.ResearchQueryRequest{Query: input.Question, Contexts: contexts})
		if researchErr != nil {
			return QueryResult{}, researchAIError(researchErr)
		}
		if validCitationSet(response.Citations, passages) {
			answer = response.Answer
			synthesisModel = response.Model
		} else {
			answer = "توجد أدلة مصدرية، لكن التلخيص الآلي لم يجتز تحقق الإسناد؛ راجع الأدلة مباشرة."
		}
	}
	if len(conflicts) > 0 && !insufficient {
		answer = "توجد روايات أو ادعاءات متعارضة. " + answer
	}
	if synthesisModel == "" {
		synthesisModel = retrieval.ModelVersion
	}
	layers := layeredEvidence(retrieval)
	allCitations := append([]Citation{}, retrieval.Passages...)
	allCitations = append(allCitations, retrieval.Claims...)
	allCitations = append(allCitations, retrieval.Trees...)
	allCitations = append(allCitations, retrieval.Findings...)
	allCitations = append(allCitations, retrieval.Questions...)
	stats.EvidenceCount = len(allCitations)
	return QueryResult{
		Query:                input.Question,
		NormalizedQuery:      normalized,
		QueryType:            queryType,
		Answer:               answer,
		InsufficientEvidence: insufficient,
		ModelVersion:         synthesisModel,
		Citations:            allCitations,
		Layers:               layers,
		Conflicts:            conflicts,
		Retrieval:            stats,
	}, nil
}

func (s *Service) startRun(ctx context.Context, input QueryInput, actorID string) (string, time.Time, error) {
	questionID, err := optionalUUID(input.QuestionID)
	if err != nil {
		return "", time.Time{}, err
	}
	actorUUID, err := optionalUUID(actorID)
	if err != nil {
		return "", time.Time{}, ErrForbidden
	}
	runID := uuid.New()
	var createdAt time.Time
	if err := s.Pool.QueryRow(ctx, `
		INSERT INTO research_runs (id, question_id, actor_id, query, normalized_query, status)
		VALUES ($1, $2, $3, $4, $5, 'running')
		RETURNING created_at
	`, runID, questionID, actorUUID, input.Question, identity.NormalizeArabicName(input.Question)).Scan(&createdAt); err != nil {
		return "", time.Time{}, err
	}
	return runID.String(), createdAt, nil
}

func (s *Service) failRun(ctx context.Context, runID string, cause error) error {
	message := "research run failed"
	if errors.Is(cause, ErrAIUnavailable) {
		message = "research AI service is unavailable"
	}
	_, err := s.Pool.Exec(ctx, `UPDATE research_runs SET status = 'failed', error = $1, updated_at = now() WHERE id = $2`, message, runID)
	return err
}

func (s *Service) persistRun(ctx context.Context, runID string, result QueryResult) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, citation := range result.Citations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO research_evidence (run_id, layer, reference_type, reference_id, source_id, passage_id, statement_id, claim_id, finding_id, question_id, excerpt, rank, lexical_score, vector_score, rerank_score, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		`, runID, citation.Layer, citation.Type, citation.ID, nullableString(citation.SourceID), nullableString(citation.PassageID), nullableString(citation.StatementID), nullableString(citation.ClaimID), nullableString(citation.FindingID), nullableString(citation.QuestionID), citation.Excerpt, citation.Rank, citation.Score.Lexical, citation.Score.Vector, citation.Score.Rerank, mustJSON(map[string]any{"combined": citation.Score.Combined, "reviewStatus": citation.ReviewStatus, "locator": citation.LocatorAR, "pageNumber": citation.PageNumber})); err != nil {
			return err
		}
	}
	citations := result.Citations
	conflicts := result.Conflicts
	if _, err := tx.Exec(ctx, `
		INSERT INTO research_answers (run_id, answer, citations, conflicts, insufficient_evidence)
		VALUES ($1, $2, $3, $4, $5)
	`, runID, result.Answer, mustJSON(citations), mustJSON(conflicts), result.InsufficientEvidence); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE research_runs SET status = 'succeeded', query_type = $1, insufficient_evidence = $2, model_version = NULLIF($3, ''), updated_at = now()
		WHERE id = $4
	`, result.QueryType, result.InsufficientEvidence, result.ModelVersion, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateQueryInput(input QueryInput) (QueryInput, error) {
	input.Question = strings.TrimSpace(input.Question)
	if input.Question == "" || len([]rune(input.Question)) > 2000 || input.FromYear < 0 || input.ToYear < 0 || (input.FromYear > 0 && input.ToYear > 0 && input.FromYear > input.ToYear) {
		return QueryInput{}, ErrValidation
	}
	for _, value := range []struct{ target *string }{{&input.QuestionID}, {&input.SourceID}, {&input.PersonID}, {&input.PlaceID}} {
		*value.target = strings.TrimSpace(*value.target)
		if *value.target != "" {
			if _, err := uuid.Parse(*value.target); err != nil {
				return QueryInput{}, ErrValidation
			}
		}
	}
	return input, nil
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

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func evidencePackage(retrieval retrievalContext) EvidencePackage {
	evidence := EvidencePackage{SourceStatements: []Reference{}, ResearchClaims: []Reference{}, TreeInterpretations: []Reference{}, PlatformFindings: []Reference{}, OpenQuestions: []Reference{}}
	for _, citation := range retrieval.Passages {
		evidence.SourceStatements = append(evidence.SourceStatements, Reference{Layer: SourceStatement, Type: citation.Type, ID: citation.ID})
	}
	for _, citation := range retrieval.Claims {
		evidence.ResearchClaims = append(evidence.ResearchClaims, Reference{Layer: ResearchClaim, Type: citation.Type, ID: citation.ID})
	}
	for _, citation := range retrieval.Trees {
		evidence.TreeInterpretations = append(evidence.TreeInterpretations, Reference{Layer: TreeInterpretation, Type: citation.Type, ID: citation.ID})
	}
	for _, citation := range retrieval.Findings {
		evidence.PlatformFindings = append(evidence.PlatformFindings, Reference{Layer: PlatformFinding, Type: citation.Type, ID: citation.ID})
	}
	for _, citation := range retrieval.Questions {
		evidence.OpenQuestions = append(evidence.OpenQuestions, Reference{Layer: OpenQuestion, Type: citation.Type, ID: citation.ID})
	}
	return evidence
}

func layeredEvidence(retrieval retrievalContext) LayeredEvidence {
	return LayeredEvidence{SourceStatements: retrieval.Passages, ResearchClaims: retrieval.Claims, TreeInterpretations: retrieval.Trees, PlatformFindings: retrieval.Findings, OpenQuestions: retrieval.Questions}
}

func validCitationSet(citations []ai.ResearchCitation, passages []Citation) bool {
	allowed := make(map[string]struct{}, len(passages))
	for _, passage := range passages {
		allowed[passage.PassageID] = struct{}{}
	}
	if len(citations) == 0 {
		return false
	}
	for _, citation := range citations {
		if _, ok := allowed[citation.SourceID]; !ok {
			return false
		}
	}
	return true
}

func allowedQueryType(value string) bool {
	switch value {
	case "source_evidence", "identity", "relationship", "geography", "general":
		return true
	default:
		return false
	}
}

func researchAIError(err error) error {
	if errors.Is(err, ai.ErrUnavailable) || errors.Is(err, ai.ErrValidation) {
		return ErrAIUnavailable
	}
	return err
}

func (s *Service) detectStatementConflicts(ctx context.Context, passages []Citation) ([]Conflict, error) {
	statements := make([]ai.Statement, 0, min(len(passages), 10))
	for _, passage := range passages {
		if len(statements) == 10 {
			break
		}
		if passage.StatementID != "" {
			statements = append(statements, ai.Statement{ID: passage.StatementID, Text: passage.Excerpt})
		}
	}
	if len(statements) < 2 {
		return nil, nil
	}
	response, err := s.AI.AnalyzeContradiction(ctx, ai.ContradictionRequest{Statements: statements})
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(statements))
	for _, statement := range statements {
		allowed[statement.ID] = struct{}{}
	}
	conflicts := make([]Conflict, 0, len(response.Pairs))
	for _, pair := range response.Pairs {
		if _, leftOK := allowed[pair.LeftID]; !leftOK {
			continue
		}
		if _, rightOK := allowed[pair.RightID]; !rightOK {
			continue
		}
		conflicts = append(conflicts, Conflict{Type: "source_statement", LeftID: pair.LeftID, RightID: pair.RightID, Status: pair.Status, Explanation: pair.Rationale})
	}
	return conflicts, nil
}
