package research

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ResearchRunSummary struct {
	ID                   string    `json:"id"`
	QuestionID           string    `json:"questionId,omitempty"`
	Query                string    `json:"query"`
	Status               string    `json:"status"`
	InsufficientEvidence bool      `json:"insufficientEvidence"`
	CitationCount        int       `json:"citationCount"`
	GraphPathCount       int       `json:"graphPathCount"`
	GraphOperation       string    `json:"graphOperation,omitempty"`
	GraphMaxDepth        int       `json:"graphMaxDepth,omitempty"`
	GraphTruncated       bool      `json:"graphTruncated"`
	AnswerAR             string    `json:"answerAr,omitempty"`
	ModelVersion         string    `json:"modelVersion,omitempty"`
	Route                string    `json:"route,omitempty"`
	SynthesisAttempted   bool      `json:"synthesisAttempted"`
	Error                string    `json:"error,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type ResearchRunContext struct {
	ScopeType string `json:"scopeType"`
	ScopeID   string `json:"scopeId"`
	Role      string `json:"role"`
}

type ResearchRunDetail struct {
	ResearchRunSummary
	Contexts   []ResearchRunContext `json:"contexts"`
	GraphPaths []GraphPath          `json:"graphPaths"`
}

type historyExecutor interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) ListRuns(ctx context.Context, actorID, questionID string) ([]ResearchRunSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	questionUUID, err := uuid.Parse(strings.TrimSpace(questionID))
	if err != nil {
		return nil, ErrValidation
	}
	includeAnswer, err := s.canViewResearchHistory(ctx, actorID)
	if err != nil {
		return nil, err
	}
	return listRuns(ctx, s.Pool, questionUUID, includeAnswer)
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (ResearchRunDetail, error) {
	if err := s.ready(); err != nil {
		return ResearchRunDetail{}, err
	}
	runUUID, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return ResearchRunDetail{}, ErrNotFound
	}
	includeAnswer, err := s.canViewResearchHistory(ctx, actorID)
	if err != nil {
		return ResearchRunDetail{}, err
	}
	summary, err := scanRunSummary(s.Pool.QueryRow(ctx, `
		SELECT rr.id, rr.question_id, rr.query, rr.status, rr.insufficient_evidence,
		       count(DISTINCT re.id), CASE WHEN $2 THEN count(DISTINCT rgp.id) ELSE 0 END, CASE WHEN $2 THEN COALESCE(rr.graph_operation, '') ELSE '' END, CASE WHEN $2 THEN rr.graph_max_depth ELSE NULL END, CASE WHEN $2 THEN COALESCE(rr.graph_truncated, false) ELSE false END,
		       CASE WHEN $2 THEN COALESCE(ra.answer, '') ELSE '' END,
		       COALESCE(rr.model_version, ''), COALESCE(rr.semantic_route, ''), rr.synthesis_attempted,
		       CASE WHEN $2 THEN COALESCE(rr.error, '') ELSE '' END, rr.created_at, rr.updated_at
		FROM research_runs rr
		LEFT JOIN research_answers ra ON ra.run_id = rr.id
		LEFT JOIN research_evidence re ON re.run_id = rr.id
		LEFT JOIN research_graph_paths rgp ON rgp.run_id = rr.id
		WHERE rr.id = $1
		GROUP BY rr.id, ra.answer
	`, runUUID, includeAnswer))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResearchRunDetail{}, ErrNotFound
		}
		return ResearchRunDetail{}, err
	}
	contexts := make([]ResearchRunContext, 0)
	graphPaths := make([]GraphPath, 0)
	if includeAnswer {
		contexts, err = runContexts(ctx, s.Pool, runUUID)
		if err != nil {
			return ResearchRunDetail{}, err
		}
		graphPaths, err = runGraphPaths(ctx, s.Pool, runUUID)
		if err != nil {
			return ResearchRunDetail{}, err
		}
	}
	return ResearchRunDetail{ResearchRunSummary: summary, Contexts: contexts, GraphPaths: graphPaths}, nil
}

func listRuns(ctx context.Context, executor historyExecutor, questionID uuid.UUID, includeAnswer bool) ([]ResearchRunSummary, error) {
	rows, err := executor.Query(ctx, `
		SELECT rr.id, rr.question_id, rr.query, rr.status, rr.insufficient_evidence,
		       count(DISTINCT re.id), CASE WHEN $2 THEN count(DISTINCT rgp.id) ELSE 0 END, CASE WHEN $2 THEN COALESCE(rr.graph_operation, '') ELSE '' END, CASE WHEN $2 THEN rr.graph_max_depth ELSE NULL END, CASE WHEN $2 THEN COALESCE(rr.graph_truncated, false) ELSE false END,
		       CASE WHEN $2 THEN COALESCE(ra.answer, '') ELSE '' END,
		       COALESCE(rr.model_version, ''), COALESCE(rr.semantic_route, ''), rr.synthesis_attempted,
		       CASE WHEN $2 THEN COALESCE(rr.error, '') ELSE '' END, rr.created_at, rr.updated_at
		FROM research_runs rr
		LEFT JOIN research_answers ra ON ra.run_id = rr.id
		LEFT JOIN research_evidence re ON re.run_id = rr.id
		LEFT JOIN research_graph_paths rgp ON rgp.run_id = rr.id
		WHERE rr.question_id = $1
		GROUP BY rr.id, ra.answer
		ORDER BY rr.created_at DESC
		LIMIT 100
	`, questionID, includeAnswer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ResearchRunSummary, 0)
	for rows.Next() {
		item, scanErr := scanResearchRunSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func runContexts(ctx context.Context, executor historyExecutor, runID uuid.UUID) ([]ResearchRunContext, error) {
	rows, err := executor.Query(ctx, `SELECT scope_type, scope_id, role FROM research_run_contexts WHERE run_id = $1 ORDER BY scope_type, role, scope_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ResearchRunContext, 0)
	for rows.Next() {
		var item ResearchRunContext
		var scopeID pgtype.UUID
		if err := rows.Scan(&item.ScopeType, &scopeID, &item.Role); err != nil {
			return nil, err
		}
		item.ScopeID = uuidText(scopeID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func runGraphPaths(ctx context.Context, executor historyExecutor, runID uuid.UUID) ([]GraphPath, error) {
	rows, err := executor.Query(ctx, `
		SELECT id, operation, status, explanation, depth, truncated, evidence_backed, structural_only,
		       COALESCE(tree_id::text, ''), COALESCE(tree_version_id::text, ''), COALESCE(tree_version_number, 0), COALESCE(tree_version_state, ''), COALESCE(algorithm_version, ''), nodes
		FROM research_graph_paths
		WHERE run_id = $1
		ORDER BY created_at, id
		LIMIT 5
	`, runID)
	if err != nil {
		return nil, err
	}
	paths := make([]GraphPath, 0)
	for rows.Next() {
		var path GraphPath
		var id pgtype.UUID
		var treeID, treeVersionID, versionState, algorithmVersion string
		var versionNumber, depth int32
		var nodes []byte
		if err := rows.Scan(&id, &path.Operation, &path.Status, &path.Explanation, &depth, &path.Truncated, &path.EvidenceBacked, &path.StructuralOnly, &treeID, &treeVersionID, &versionNumber, &versionState, &algorithmVersion, &nodes); err != nil {
			rows.Close()
			return nil, err
		}
		path.ID = uuidText(id)
		path.Depth = int(depth)
		path.TreeScope = GraphTreeScope{TreeID: treeID, TreeVersionID: treeVersionID, VersionNumber: int(versionNumber), VersionState: versionState}
		path.AlgorithmVersion = algorithmVersion
		if err := json.Unmarshal(nodes, &path.Nodes); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range paths {
		edges, edgeErr := runGraphEdges(ctx, executor, runID, optionalUUID(paths[index].ID))
		if edgeErr != nil {
			return nil, edgeErr
		}
		evidence, evidenceErr := runGraphEvidence(ctx, executor, runID, optionalUUID(paths[index].ID))
		if evidenceErr != nil {
			return nil, evidenceErr
		}
		paths[index].Edges = edges
		paths[index].EvidenceRefs = evidence
	}
	return paths, nil
}

func runGraphEdges(ctx context.Context, executor historyExecutor, runID, pathID uuid.UUID) ([]GraphEdge, error) {
	rows, err := executor.Query(ctx, `
		SELECT id, edge_reference_id, edge_type, from_node_id, to_node_id, COALESCE(predicate, ''), COALESCE(status, ''), COALESCE(certainty, ''),
		       COALESCE(source_id::text, ''), COALESCE(claim_id::text, ''), COALESCE(statement_id::text, ''), COALESCE(passage_id::text, ''),
		       COALESCE(tree_relationship_id::text, ''), COALESCE(migration_event_id::text, ''), COALESCE(from_place_id::text, ''), COALESCE(to_place_id::text, ''), ordinal
		FROM research_graph_edges
		WHERE run_id = $1 AND path_id = $2
		ORDER BY ordinal
		LIMIT 200
	`, runID, pathID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]GraphEdge, 0)
	for rows.Next() {
		var item GraphEdge
		var id, referenceID pgtype.UUID
		var ordinal int32
		if err := rows.Scan(&id, &referenceID, &item.Type, &item.FromNodeID, &item.ToNodeID, &item.Predicate, &item.Status, &item.Certainty, &item.SourceID, &item.ClaimID, &item.StatementID, &item.PassageID, &item.TreeRelationshipID, &item.MigrationEventID, &item.FromPlaceID, &item.ToPlaceID, &ordinal); err != nil {
			return nil, err
		}
		item.ID = uuidText(referenceID)
		if item.ID == "" {
			item.ID = uuidText(id)
		}
		item.Position = int(ordinal - 1)
		items = append(items, item)
	}
	return items, rows.Err()
}

func runGraphEvidence(ctx context.Context, executor historyExecutor, runID, pathID uuid.UUID) ([]GraphEvidenceRef, error) {
	rows, err := executor.Query(ctx, `
		SELECT reference_id, reference_type, COALESCE(metadata->>'layer', ''), COALESCE(relation, ''), COALESCE(source_id::text, ''), COALESCE(claim_id::text, ''), COALESCE(statement_id::text, ''), COALESCE(passage_id::text, ''),
		       COALESCE(review_status, ''), COALESCE(status, ''), COALESCE(certainty, ''), COALESCE(title_ar, ''), COALESCE(excerpt_ar, ''), COALESCE(locator_ar, ''), page_number
		FROM research_graph_path_evidence
		WHERE run_id = $1 AND path_id = $2
		ORDER BY ordinal
		LIMIT 200
	`, runID, pathID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]GraphEvidenceRef, 0)
	for rows.Next() {
		var item GraphEvidenceRef
		var pageNumber pgtype.Int4
		if err := rows.Scan(&item.ID, &item.Type, &item.Layer, &item.Relation, &item.SourceID, &item.ClaimID, &item.StatementID, &item.PassageID, &item.ReviewStatus, &item.Status, &item.Certainty, &item.Title, &item.Excerpt, &item.LocatorAR, &pageNumber); err != nil {
			return nil, err
		}
		if pageNumber.Valid {
			value := int(pageNumber.Int32)
			item.PageNumber = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanResearchRunSummary(rows pgx.Rows) (ResearchRunSummary, error) {
	var item ResearchRunSummary
	var id, questionID pgtype.UUID
	var graphOperation, modelVersion, route, runError pgtype.Text
	var graphMaxDepth pgtype.Int4
	var graphTruncated pgtype.Bool
	if err := rows.Scan(&id, &questionID, &item.Query, &item.Status, &item.InsufficientEvidence, &item.CitationCount, &item.GraphPathCount, &graphOperation, &graphMaxDepth, &graphTruncated, &item.AnswerAR, &modelVersion, &route, &item.SynthesisAttempted, &runError, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ResearchRunSummary{}, err
	}
	item.ID = uuidText(id)
	item.QuestionID = uuidText(questionID)
	item.GraphOperation = textValue(graphOperation)
	if graphMaxDepth.Valid {
		item.GraphMaxDepth = int(graphMaxDepth.Int32)
	}
	item.GraphTruncated = graphTruncated.Valid && graphTruncated.Bool
	item.ModelVersion = textValue(modelVersion)
	item.Route = textValue(route)
	item.Error = textValue(runError)
	return item, nil
}

func scanRunSummary(row pgx.Row) (ResearchRunSummary, error) {
	var item ResearchRunSummary
	var id, questionID pgtype.UUID
	var graphOperation, modelVersion, route, runError pgtype.Text
	var graphMaxDepth pgtype.Int4
	var graphTruncated pgtype.Bool
	if err := row.Scan(&id, &questionID, &item.Query, &item.Status, &item.InsufficientEvidence, &item.CitationCount, &item.GraphPathCount, &graphOperation, &graphMaxDepth, &graphTruncated, &item.AnswerAR, &modelVersion, &route, &item.SynthesisAttempted, &runError, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ResearchRunSummary{}, err
	}
	item.ID = uuidText(id)
	item.QuestionID = uuidText(questionID)
	item.GraphOperation = textValue(graphOperation)
	if graphMaxDepth.Valid {
		item.GraphMaxDepth = int(graphMaxDepth.Int32)
	}
	item.GraphTruncated = graphTruncated.Valid && graphTruncated.Bool
	item.ModelVersion = textValue(modelVersion)
	item.Route = textValue(route)
	item.Error = textValue(runError)
	return item, nil
}

func (s *Service) canViewResearchHistory(ctx context.Context, actorID string) (bool, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return false, nil
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return false, ErrForbidden
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('researcher', 'moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}
