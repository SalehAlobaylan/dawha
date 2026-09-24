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
	retrieval := retrievalContext{Input: input, Normalized: normalized, ActorID: actorID, Vector: vector, QueryType: "general"}
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
	graphPaths := make([]GraphPath, 0)
	graphStats := GraphStats{}
	graphPassageCount := 0
	if input.GraphOperation != "" {
		graphResult, graphErr := s.retrieveGraph(ctx, input, actorID)
		if graphErr != nil {
			return QueryResult{}, graphErr
		}
		if validationErr := validateGraphPaths(graphResult.Paths, input.GraphMaxDepth); validationErr != nil {
			return QueryResult{}, ErrGraphUnavailable
		}
		graphPaths = graphResult.Paths
		graphStats = graphResult.Stats
		graphCitations := graphPassageCitations(graphPaths, retrieval.Passages)
		graphPassageCount = len(graphCitations)
		retrieval.Passages = append(retrieval.Passages, graphCitations...)
	}
	evidence := evidencePackage(retrieval)
	if err := evidence.Validate(); err != nil {
		return QueryResult{}, ErrValidation
	}
	routing, err := s.routeResearch(ctx, input, retrieval.Passages)
	if err != nil {
		return QueryResult{}, err
	}
	if !allowedQueryType(routing.QueryType) {
		routing.QueryType = "general"
	}
	retrieval.QueryType = routing.QueryType
	conflicts := claimConflicts(retrieval.Claims)
	if len(retrieval.Passages) > 0 && (routing.Route == ai.RoutingRouteDeep || routing.PotentialContradiction) {
		detected, detectErr := s.detectStatementConflicts(ctx, retrieval.Passages)
		if detectErr != nil {
			return QueryResult{}, researchAIError(detectErr)
		}
		conflicts = append(conflicts, detected...)
	}
	hasGraphEvidence := graphPassageCount > 0
	insufficient := len(retrieval.Passages) == 0 && !hasGraphEvidence
	answer := safeInsufficientAnswer
	synthesisModel := ""
	synthesisAttempted := false
	if !insufficient {
		switch routing.Route {
		case ai.RoutingRouteDeep:
			synthesisAttempted = true
			contexts := make([]ai.SourceContext, 0, min(len(retrieval.Passages), 20))
			for _, passage := range retrieval.Passages {
				if len(contexts) == 20 {
					break
				}
				contexts = append(contexts, ai.SourceContext{ID: citationContextID(passage), Title: passage.Title, Text: passage.Excerpt})
			}
			response, researchErr := s.AI.ResearchQuery(ctx, ai.ResearchQueryRequest{Query: input.Question, Contexts: contexts})
			if researchErr != nil {
				return QueryResult{}, researchAIError(researchErr)
			}
			if validCitationSet(response.Citations, retrieval.Passages) {
				answer = response.Answer
				synthesisModel = response.Model
			} else {
				answer = "توجد أدلة مصدرية، لكن التلخيص الآلي لم يجتز تحقق الإسناد؛ راجع الأدلة مباشرة."
			}
		case ai.RoutingRouteIgnore:
			answer = "تم تجاهل هذا الطلب على أساس إشارات تشغيلية فقط؛ لا تُستخدم هذه الإجابة كقاعدة تاريخية."
		default:
			answer = "توجد مواد مرتبطة بالسؤال؛ راجع المقتطفات المصدرية أدناه. لم يُستخدم مسار تفكير عميق لهذه العملية."
		}
	}
	if len(conflicts) > 0 && !insufficient {
		answer = "توجد روايات أو ادعاءات متعارضة. " + answer
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
		QueryType:            routing.QueryType,
		Answer:               answer,
		InsufficientEvidence: insufficient,
		ModelVersion:         synthesisModel,
		Routing: RoutingInfo{
			Route:                  routing.Route,
			QueryType:              routing.QueryType,
			ReasonCode:             routing.ReasonCode,
			Model:                  routing.Model,
			Fallback:               routing.Fallback,
			OperationalScore:       routing.OperationalScore,
			SourceBearing:          routing.SourceBearing,
			PotentialContradiction: routing.PotentialContradiction,
			ContinueInvestigation:  routing.ContinueInvestigation,
			SynthesisAttempted:     synthesisAttempted,
		},
		Citations:  allCitations,
		Layers:     layers,
		Conflicts:  conflicts,
		Retrieval:  stats,
		GraphPaths: graphPaths,
		GraphStats: graphStats,
	}, nil
}

func (s *Service) routeResearch(ctx context.Context, input QueryInput, passages []Citation) (ai.RoutingDecision, error) {
	request := ai.RoutingRequest{
		Text:        input.Question,
		Context:     researchRoutingContext(passages),
		Operation:   "research",
		SourceCount: len(passages),
	}
	provider, ok := s.AI.(ai.RouteProvider)
	if !ok {
		return ai.FallbackRoute(request), nil
	}
	routeCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	result, err := provider.Route(routeCtx, request)
	if err != nil {
		if ctx.Err() != nil {
			return ai.RoutingDecision{}, ctx.Err()
		}
		return ai.FallbackRoute(request), nil
	}
	if err := ai.ValidateRoutingDecision(result); err != nil {
		return ai.FallbackRoute(request), nil
	}
	return result, nil
}

func researchRoutingContext(passages []Citation) string {
	var builder strings.Builder
	for index, passage := range passages {
		if index >= 10 {
			break
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(passage.Title)
		builder.WriteString("\n")
		builder.WriteString(passage.Excerpt)
	}
	value := []rune(builder.String())
	if len(value) > 18000 {
		return string(value[:18000])
	}
	return string(value)
}

func (s *Service) startRun(ctx context.Context, input QueryInput, actorID string) (string, time.Time, error) {
	questionUUID, err := parseOptionalUUID(input.QuestionID)
	if err != nil {
		return "", time.Time{}, err
	}
	actorUUID, err := parseOptionalUUID(actorID)
	if err != nil {
		return "", time.Time{}, ErrForbidden
	}
	runID := uuid.New()
	var createdAt time.Time
	var graphOperation any
	var graphMaxDepth any
	if input.GraphOperation != "" {
		graphOperation = input.GraphOperation
		graphMaxDepth = input.GraphMaxDepth
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `
		INSERT INTO research_runs (id, question_id, actor_id, query, normalized_query, status, graph_operation, graph_max_depth)
		VALUES ($1, $2, $3, $4, $5, 'running', $6, $7)
		RETURNING created_at
	`, runID, nullableUUID(questionUUID), nullableUUID(actorUUID), input.Question, identity.NormalizeArabicName(input.Question), graphOperation, graphMaxDepth).Scan(&createdAt); err != nil {
		return "", time.Time{}, err
	}
	contexts := make([]struct {
		scopeType string
		scopeID   uuid.UUID
		role      string
	}, 0, 7)
	if questionUUID != uuid.Nil {
		contexts = append(contexts, struct {
			scopeType string
			scopeID   uuid.UUID
			role      string
		}{"question", questionUUID, "question"})
	}
	if input.EntityID != "" {
		entityUUID, parseErr := uuid.Parse(input.EntityID)
		if parseErr != nil {
			return "", time.Time{}, ErrValidation
		}
		contexts = append(contexts, struct {
			scopeType string
			scopeID   uuid.UUID
			role      string
		}{input.EntityType, entityUUID, "subject"})
	}
	if input.TreeID != "" {
		treeUUID, parseErr := uuid.Parse(input.TreeID)
		if parseErr != nil {
			return "", time.Time{}, ErrValidation
		}
		contexts = append(contexts, struct {
			scopeType string
			scopeID   uuid.UUID
			role      string
		}{"tree", treeUUID, "filter"})
	}
	if input.TreeVersionID != "" {
		versionUUID, parseErr := uuid.Parse(input.TreeVersionID)
		if parseErr != nil {
			return "", time.Time{}, ErrValidation
		}
		contexts = append(contexts, struct {
			scopeType string
			scopeID   uuid.UUID
			role      string
		}{"tree_version", versionUUID, "filter"})
	}
	if input.SourceID != "" {
		sourceUUID, parseErr := uuid.Parse(input.SourceID)
		if parseErr != nil {
			return "", time.Time{}, ErrValidation
		}
		contexts = append(contexts, struct {
			scopeType string
			scopeID   uuid.UUID
			role      string
		}{"source", sourceUUID, "filter"})
	}
	if input.GraphOperation != "" {
		startUUID, parseErr := uuid.Parse(input.GraphStartID)
		if parseErr != nil {
			return "", time.Time{}, ErrValidation
		}
		contexts = append(contexts, struct {
			scopeType string
			scopeID   uuid.UUID
			role      string
		}{input.GraphStartType, startUUID, "graph_start"})
		if input.GraphEndID != "" {
			endUUID, endErr := uuid.Parse(input.GraphEndID)
			if endErr != nil {
				return "", time.Time{}, ErrValidation
			}
			contexts = append(contexts, struct {
				scopeType string
				scopeID   uuid.UUID
				role      string
			}{input.GraphEndType, endUUID, "graph_end"})
		}
	}
	if input.PlaceID != "" {
		placeUUID, parseErr := uuid.Parse(input.PlaceID)
		if parseErr != nil {
			return "", time.Time{}, ErrValidation
		}
		contexts = append(contexts, struct {
			scopeType string
			scopeID   uuid.UUID
			role      string
		}{"place", placeUUID, "filter"})
	}
	for _, context := range contexts {
		if _, err := tx.Exec(ctx, `INSERT INTO research_run_contexts (run_id, scope_type, scope_id, role) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, runID, context.scopeType, context.scopeID, context.role); err != nil {
			return "", time.Time{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", time.Time{}, err
	}
	return runID.String(), createdAt, nil
}

func (s *Service) failRun(ctx context.Context, runID string, cause error) error {
	message := "research run failed"
	if errors.Is(cause, ErrAIUnavailable) {
		message = "research AI service is unavailable"
	}
	if errors.Is(cause, ErrGraphUnavailable) {
		message = "research graph retrieval is unavailable"
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
	if result.GraphStats.Operation != "" || len(result.GraphPaths) > 0 {
		if err := persistGraphRun(ctx, tx, runID, result); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE research_runs SET
			status = 'succeeded',
			query_type = $1,
			insufficient_evidence = $2,
			model_version = NULLIF($3, ''),
			semantic_route = NULLIF($4, ''),
			semantic_route_model = NULLIF($5, ''),
			semantic_route_reason = NULLIF($6, ''),
			semantic_route_fallback = $7,
			semantic_route_score = $8,
			semantic_route_source_bearing = $9,
			semantic_route_potential_contradiction = $10,
			semantic_route_continue_investigation = $11,
			synthesis_attempted = $12,
			graph_truncated = $13,
			updated_at = now()
		WHERE id = $14
	`, result.QueryType, result.InsufficientEvidence, result.ModelVersion, result.Routing.Route, result.Routing.Model, result.Routing.ReasonCode, result.Routing.Fallback, result.Routing.OperationalScore, result.Routing.SourceBearing, result.Routing.PotentialContradiction, result.Routing.ContinueInvestigation, result.Routing.SynthesisAttempted, graphTruncatedValue(result.GraphStats, len(result.GraphPaths) > 0), runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateQueryInput(input QueryInput) (QueryInput, error) {
	input.Question = strings.TrimSpace(input.Question)
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.EntityID = strings.TrimSpace(input.EntityID)
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	if input.Question == "" || len([]rune(input.Question)) > 2000 || input.FromYear < 0 || input.ToYear < 0 || (input.FromYear > 0 && input.ToYear > 0 && input.FromYear > input.ToYear) {
		return QueryInput{}, ErrValidation
	}
	for _, value := range []struct{ target *string }{{&input.QuestionID}, {&input.EntityID}, {&input.TreeID}, {&input.TreeVersionID}, {&input.SourceID}, {&input.PersonID}, {&input.PlaceID}, {&input.GraphStartID}, {&input.GraphEndID}} {
		*value.target = strings.TrimSpace(*value.target)
		if *value.target != "" {
			if _, err := uuid.Parse(*value.target); err != nil {
				return QueryInput{}, ErrValidation
			}
		}
	}
	if input.EntityType == "" && (input.EntityID != "" || input.PersonID != "") {
		input.EntityType = "person"
		if input.EntityID == "" {
			input.EntityID = input.PersonID
		}
	}
	if input.EntityType == "person" && input.EntityID == "" {
		input.EntityID = input.PersonID
	}
	if input.EntityType != "" && input.EntityID == "" {
		return QueryInput{}, ErrValidation
	}
	if input.EntityType != "" && input.EntityType != "person" && input.EntityType != "family" && input.EntityType != "branch" {
		return QueryInput{}, ErrValidation
	}
	if input.EntityType == "person" && input.PersonID != "" && input.EntityID != input.PersonID {
		return QueryInput{}, ErrValidation
	}
	if input.EntityType == "person" {
		input.PersonID = input.EntityID
	}
	return normalizeGraphInput(input)
}

func parseOptionalUUID(value string) (uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.Nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, ErrValidation
	}
	return parsed, nil
}

func nullableUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
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

func citationContextID(citation Citation) string {
	if citation.PassageID != "" {
		return citation.PassageID
	}
	if citation.StatementID != "" {
		return citation.StatementID
	}
	if citation.ClaimID != "" {
		return citation.ClaimID
	}
	return citation.ID
}

func validCitationSet(citations []ai.ResearchCitation, passages []Citation) bool {
	allowed := make(map[string]struct{}, len(passages))
	for _, passage := range passages {
		if contextID := citationContextID(passage); contextID != "" {
			allowed[contextID] = struct{}{}
		}
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
