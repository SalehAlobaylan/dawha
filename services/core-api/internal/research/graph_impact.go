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

type GraphRelationshipImpactInput struct {
	TreeID         string `json:"tree_id"`
	TreeVersionID  string `json:"tree_version_id"`
	RelationshipID string `json:"relationship_id"`
	MaxDepth       int    `json:"max_depth,omitempty"`
}

type GraphRelationshipImpactLimits struct {
	MaxDepth int `json:"maxDepth"`
	MaxNodes int `json:"maxNodes"`
	MaxEdges int `json:"maxEdges"`
}

type GraphRelationshipImpactResult struct {
	RunID                      string                        `json:"runId"`
	CreatedAt                  time.Time                     `json:"createdAt"`
	Operation                  string                        `json:"operation"`
	RootRelationshipID         string                        `json:"rootRelationshipId"`
	PathID                     string                        `json:"pathId"`
	TreeScope                  GraphTreeScope                `json:"treeScope"`
	BoundedDownstreamNodeCount int                           `json:"boundedDownstreamNodeCount"`
	Truncated                  bool                          `json:"truncated"`
	TruncationReasons          []string                      `json:"truncationReasons"`
	Limits                     GraphRelationshipImpactLimits `json:"limits"`
	StructuralOnly             bool                          `json:"structuralOnly"`
	AlgorithmVersion           string                        `json:"algorithmVersion"`
	Status                     string                        `json:"status"`
	Explanation                string                        `json:"explanation"`
	Depth                      int                           `json:"depth"`
	Nodes                      []GraphNode                   `json:"nodes"`
	Edges                      []GraphEdge                   `json:"edges"`
}

type graphRelationshipImpactRow struct {
	Path               GraphPath
	RootRelationshipID string
	NodesTruncated     bool
	EdgesTruncated     bool
	TraversalSaturated bool
	DepthTruncated     bool
}

func (s *Service) GraphRelationshipImpact(ctx context.Context, input GraphRelationshipImpactInput, actorID string) (GraphRelationshipImpactResult, error) {
	if err := s.ready(); err != nil {
		return GraphRelationshipImpactResult{}, err
	}
	normalized, err := normalizeGraphRelationshipImpactInput(input)
	if err != nil {
		return GraphRelationshipImpactResult{}, err
	}
	actorUUID, err := graphActorUUID(actorID)
	if err != nil {
		return GraphRelationshipImpactResult{}, err
	}
	if err := s.validateGraphRelationshipImpactTarget(ctx, normalized, actorUUID); err != nil {
		return GraphRelationshipImpactResult{}, err
	}
	runID, createdAt, err := s.startRun(ctx, QueryInput{
		Question:       "تحليل أثر العلاقة",
		GraphOperation: GraphOperationRelationshipImpact,
		GraphStartType: "relationship",
		GraphStartID:   normalized.RelationshipID,
		TreeID:         normalized.TreeID,
		TreeVersionID:  normalized.TreeVersionID,
		GraphMaxDepth:  normalized.MaxDepth,
	}, actorID)
	if err != nil {
		return GraphRelationshipImpactResult{}, err
	}
	row, err := s.retrieveGraphRelationshipImpact(ctx, normalized, actorUUID)
	if err != nil {
		_ = s.failRun(ctx, runID, err)
		return GraphRelationshipImpactResult{}, err
	}
	stats := graphStatsForPaths(GraphOperationRelationshipImpact, normalized.MaxDepth, []GraphPath{row.Path})
	if err := s.persistGraphRelationshipImpactRun(ctx, runID, row.Path, stats); err != nil {
		_ = s.failRun(ctx, runID, err)
		return GraphRelationshipImpactResult{}, err
	}
	reasons := graphRelationshipImpactTruncationReasons(row)
	downstreamCount := len(row.Path.Nodes) - 1
	if downstreamCount < 0 {
		downstreamCount = 0
	}
	return GraphRelationshipImpactResult{
		RunID:                      runID,
		CreatedAt:                  createdAt,
		Operation:                  GraphOperationRelationshipImpact,
		RootRelationshipID:         row.RootRelationshipID,
		PathID:                     row.Path.ID,
		TreeScope:                  row.Path.TreeScope,
		BoundedDownstreamNodeCount: downstreamCount,
		Truncated:                  row.Path.Truncated,
		TruncationReasons:          reasons,
		Limits:                     GraphRelationshipImpactLimits{MaxDepth: normalized.MaxDepth, MaxNodes: GraphMaxNodes, MaxEdges: GraphMaxEdges},
		StructuralOnly:             true,
		AlgorithmVersion:           GraphRelationshipImpactAlgorithm,
		Status:                     row.Path.Status,
		Explanation:                row.Path.Explanation,
		Depth:                      row.Path.Depth,
		Nodes:                      row.Path.Nodes,
		Edges:                      row.Path.Edges,
	}, nil
}

func normalizeGraphRelationshipImpactInput(input GraphRelationshipImpactInput) (GraphRelationshipImpactInput, error) {
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	input.RelationshipID = strings.TrimSpace(input.RelationshipID)
	if input.TreeID == "" || input.TreeVersionID == "" || input.RelationshipID == "" {
		return GraphRelationshipImpactInput{}, ErrValidation
	}
	var err error
	input.TreeID, err = normalizeGraphUUID(input.TreeID)
	if err != nil {
		return GraphRelationshipImpactInput{}, err
	}
	input.TreeVersionID, err = normalizeGraphUUID(input.TreeVersionID)
	if err != nil {
		return GraphRelationshipImpactInput{}, err
	}
	input.RelationshipID, err = normalizeGraphUUID(input.RelationshipID)
	if err != nil {
		return GraphRelationshipImpactInput{}, err
	}
	if input.MaxDepth < 0 {
		return GraphRelationshipImpactInput{}, ErrValidation
	}
	if input.MaxDepth == 0 {
		input.MaxDepth = GraphDefaultDepth
	}
	if input.MaxDepth > GraphMaxDepth {
		input.MaxDepth = GraphMaxDepth
	}
	return input, nil
}

func graphActorUUID(actorID string) (uuid.UUID, error) {
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

func (s *Service) validateGraphRelationshipImpactTarget(ctx context.Context, input GraphRelationshipImpactInput, actorID uuid.UUID) error {
	var predicate, state, visibility string
	err := s.Pool.QueryRow(ctx, `
		SELECT tr.predicate, tv.state, t.visibility
		FROM tree_relationships tr
		JOIN tree_versions tv ON tv.id = tr.tree_version_id
		JOIN trees t ON t.id = tv.tree_id
		WHERE tr.id = $1 AND tr.tree_version_id = $2 AND t.id = $3
	`, input.RelationshipID, input.TreeVersionID, input.TreeID).Scan(&predicate, &state, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if predicate != "parent_of" || state != "published" {
		return ErrValidation
	}
	if visibility == "public" {
		return nil
	}
	if actorID == uuid.Nil {
		return ErrForbidden
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trees t WHERE t.id = $1 AND (t.owner_id = $2 OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2)))`, input.TreeID, actorID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Service) retrieveGraphRelationshipImpact(ctx context.Context, input GraphRelationshipImpactInput, actorID uuid.UUID) (graphRelationshipImpactRow, error) {
	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()
	tx, err := s.Pool.BeginTx(queryCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return graphRelationshipImpactRow{}, graphRetrievalError(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(queryCtx, `SET LOCAL statement_timeout = '2000ms'`); err != nil {
		return graphRelationshipImpactRow{}, graphRetrievalError(err)
	}
	row := tx.QueryRow(queryCtx, relationshipImpactGraphQuery, input.RelationshipID, nullableUUID(actorID), input.TreeID, input.TreeVersionID, input.MaxDepth, GraphMaxNodes, GraphMaxEdges)
	return scanGraphRelationshipImpactRow(row)
}

func scanGraphRelationshipImpactRow(row pgx.Row) (graphRelationshipImpactRow, error) {
	var operation, status string
	var versionNumber int32
	var treeID, treeVersionID, versionState, rootRelationshipID pgtype.Text
	var depth int32
	var truncated, nodesTruncated, edgesTruncated, traversalSaturated, depthTruncated, evidenceBacked, structuralOnly bool
	var nodes, edges, evidence []byte
	if err := row.Scan(&operation, &status, &versionNumber, &treeID, &treeVersionID, &versionState, &depth, &truncated, &nodesTruncated, &edgesTruncated, &traversalSaturated, &depthTruncated, &evidenceBacked, &structuralOnly, &nodes, &edges, &evidence, &rootRelationshipID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return graphRelationshipImpactRow{}, ErrNotFound
		}
		return graphRelationshipImpactRow{}, graphRetrievalError(err)
	}
	path := GraphPath{
		Operation:        operation,
		Status:           status,
		Explanation:      graphExplanation(operation, status, structuralOnly),
		Depth:            int(depth),
		Truncated:        truncated,
		EvidenceBacked:   evidenceBacked,
		StructuralOnly:   structuralOnly,
		TreeScope:        GraphTreeScope{TreeID: textValue(treeID), TreeVersionID: textValue(treeVersionID), VersionNumber: int(versionNumber), VersionState: textValue(versionState)},
		AlgorithmVersion: graphAlgorithmVersion(operation),
	}
	if err := json.Unmarshal(nodes, &path.Nodes); err != nil {
		return graphRelationshipImpactRow{}, graphRetrievalError(err)
	}
	if err := json.Unmarshal(edges, &path.Edges); err != nil {
		return graphRelationshipImpactRow{}, graphRetrievalError(err)
	}
	if err := json.Unmarshal(evidence, &path.EvidenceRefs); err != nil {
		return graphRelationshipImpactRow{}, graphRetrievalError(err)
	}
	path.ID = graphPathID(path, nodes, edges, evidence)
	return graphRelationshipImpactRow{Path: path, RootRelationshipID: textValue(rootRelationshipID), NodesTruncated: nodesTruncated, EdgesTruncated: edgesTruncated, TraversalSaturated: traversalSaturated, DepthTruncated: depthTruncated}, nil
}

func graphRelationshipImpactTruncationReasons(row graphRelationshipImpactRow) []string {
	reasons := make([]string, 0, 4)
	if row.DepthTruncated {
		reasons = append(reasons, "depth_limit")
	}
	if row.NodesTruncated || row.TraversalSaturated {
		reasons = append(reasons, "node_limit")
	}
	if row.EdgesTruncated {
		reasons = append(reasons, "edge_limit")
	}
	return reasons
}

func (s *Service) persistGraphRelationshipImpactRun(ctx context.Context, runID string, path GraphPath, stats GraphStats) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := persistGraphRun(ctx, tx, runID, QueryResult{GraphPaths: []GraphPath{path}, GraphStats: stats}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE research_runs SET status = 'succeeded', graph_truncated = $1, updated_at = now() WHERE id = $2`, stats.Truncated, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const relationshipImpactGraphQuery = `
WITH RECURSIVE
parameters AS (
  SELECT $1::uuid AS relationship_id, $2::uuid AS actor_id,
         $3::uuid AS tree_id, $4::uuid AS tree_version_id,
         $5::integer AS max_depth, $6::integer AS max_nodes, $7::integer AS max_edges
),
visible_versions AS (
  SELECT tv.id AS tree_version_id, tv.tree_id, tv.version_number, tv.state AS version_state, t.owner_id
  FROM tree_versions tv
  JOIN trees t ON t.id = tv.tree_id
  CROSS JOIN parameters p
  WHERE tv.id = p.tree_version_id
    AND t.id = p.tree_id
    AND tv.state = 'published'
    AND (t.visibility = 'public' OR (p.actor_id IS NOT NULL AND (t.owner_id = p.actor_id OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = p.actor_id))))
),
relationship_edges AS (
  SELECT tr.id AS edge_id, tr.tree_version_id,
         tr.subject_node_id AS semantic_from_node_id, tr.object_node_id AS semantic_to_node_id,
         tr.predicate, tr.status, tr.source_id,
         CASE WHEN tr.source_id IS NULL OR s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin')) OR v.owner_id = p.actor_id OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = v.tree_id AND tc.user_id = p.actor_id))) THEN tr.source_id ELSE NULL END AS visible_source_id,
         CASE WHEN tr.source_id IS NULL OR s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin')) OR v.owner_id = p.actor_id OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = v.tree_id AND tc.user_id = p.actor_id))) THEN s.title_ar ELSE NULL END AS visible_source_title
  FROM tree_relationships tr
  JOIN visible_versions v ON v.tree_version_id = tr.tree_version_id
  LEFT JOIN sources s ON s.id = tr.source_id
  CROSS JOIN parameters p
  WHERE tr.predicate = 'parent_of'
    AND EXISTS (SELECT 1 FROM tree_nodes subject WHERE subject.id = tr.subject_node_id AND subject.tree_version_id = tr.tree_version_id)
    AND EXISTS (SELECT 1 FROM tree_nodes object WHERE object.id = tr.object_node_id AND object.tree_version_id = tr.tree_version_id)
),
selected_relationship AS (
  SELECT r.*
  FROM relationship_edges r
  CROSS JOIN parameters p
  WHERE r.edge_id = p.relationship_id
),
root_node AS (
  SELECT tn.id AS node_id, tn.person_id, tn.display_name_ar
  FROM selected_relationship r
  JOIN tree_nodes tn ON tn.id = r.semantic_from_node_id
),
bfs (depth, frontier, visited, saturated) AS (
  SELECT 1, ARRAY[r.semantic_to_node_id]::uuid[], ARRAY[r.semantic_from_node_id, r.semantic_to_node_id]::uuid[], false
  FROM selected_relationship r
  UNION ALL
  SELECT next_bfs.depth, next_bfs.frontier, next_bfs.visited, next_bfs.saturated
  FROM (
    SELECT b.depth + 1 AS depth,
           next_nodes.frontier,
           b.visited || next_nodes.frontier AS visited,
           b.saturated OR next_nodes.candidate_count > $6 AS saturated
    FROM bfs b
    CROSS JOIN LATERAL (
      SELECT COALESCE(array_agg(candidate.node_id ORDER BY candidate.node_id), ARRAY[]::uuid[]) AS frontier,
             count(*)::integer AS candidate_count
      FROM (
        SELECT DISTINCT e.semantic_to_node_id AS node_id
        FROM relationship_edges e
        WHERE e.semantic_from_node_id = ANY(b.frontier)
          AND NOT e.semantic_to_node_id = ANY(b.visited)
        ORDER BY e.semantic_to_node_id
        LIMIT ($6 + 1)
      ) candidate
    ) next_nodes
    WHERE b.depth < $5 AND next_nodes.candidate_count > 0
  ) next_bfs
),
level_nodes AS (
  SELECT root_node.node_id, 0 AS depth
  FROM root_node
  UNION ALL
  SELECT unnest(b.frontier) AS node_id, b.depth
  FROM bfs b
),
reachable_nodes AS (
  SELECT node_id, min(depth)::integer AS depth
  FROM level_nodes
  GROUP BY node_id
),
ranked_nodes AS (
  SELECT node_id, depth, row_number() OVER (ORDER BY depth, node_id) AS node_order,
         count(*) OVER () AS node_count
  FROM reachable_nodes
),
bounded_nodes AS (
  SELECT node_id, depth, node_order
  FROM ranked_nodes
  WHERE node_order <= $6
),
impact_edges AS (
  SELECT e.edge_id, e.semantic_from_node_id, e.semantic_to_node_id, e.predicate, e.status,
         e.visible_source_id, e.visible_source_title,
         row_number() OVER (ORDER BY e.edge_id) AS edge_order,
         count(*) OVER () AS edge_count
  FROM relationship_edges e
  JOIN bounded_nodes subject ON subject.node_id = e.semantic_from_node_id
  JOIN bounded_nodes object ON object.node_id = e.semantic_to_node_id
),
bounded_edges AS (
  SELECT edge_id, semantic_from_node_id, semantic_to_node_id, predicate, status,
         visible_source_id, visible_source_title, edge_order
  FROM impact_edges
  WHERE edge_order <= $7
),
impact_state AS (
  SELECT
    (SELECT COALESCE(max(depth), 0)::integer FROM bounded_nodes) AS impact_depth,
    (SELECT count(*) > $6 FROM ranked_nodes) AS nodes_truncated,
    (SELECT count(*) > $7 FROM impact_edges) AS edges_truncated,
    COALESCE((SELECT bool_or(saturated) FROM bfs), false) AS traversal_saturated,
    EXISTS (
      SELECT 1
      FROM bfs b
      WHERE b.depth = $5
        AND EXISTS (
          SELECT 1
          FROM relationship_edges e
          WHERE e.semantic_from_node_id = ANY(b.frontier)
            AND NOT e.semantic_to_node_id = ANY(b.visited)
        )
    ) AS depth_truncated
),
impact_nodes AS (
  SELECT jsonb_agg(jsonb_build_object(
           'id', tn.id::text,
           'type', 'person',
           'label', left(tn.display_name_ar, 400),
           'personId', tn.person_id::text,
           'treeNodeId', tn.id::text,
           'position', bn.node_order - 1
         ) ORDER BY bn.node_order) AS nodes
  FROM bounded_nodes bn
  JOIN tree_nodes tn ON tn.id = bn.node_id
),
impact_edges_json AS (
  SELECT jsonb_agg(jsonb_build_object(
           'id', edge_id::text,
           'type', 'tree_relationship',
           'fromNodeId', semantic_from_node_id::text,
           'toNodeId', semantic_to_node_id::text,
           'pathFromNodeId', semantic_from_node_id::text,
           'pathToNodeId', semantic_to_node_id::text,
           'predicate', predicate,
           'status', status,
           'sourceId', visible_source_id,
           'treeRelationshipId', edge_id::text,
           'position', edge_order - 1
         ) ORDER BY edge_order) AS edges,
         bool_or(status IN ('disputed', 'contested', 'contradicted')) AS contested,
         bool_or(status = 'unresolved') AS partial
  FROM bounded_edges
),
impact_refs AS (
  SELECT jsonb_agg(jsonb_build_object(
           'id', edge_id::text,
           'type', 'tree_relationship',
           'layer', 'tree_interpretation',
           'relation', 'supports',
           'sourceId', visible_source_id,
           'title', visible_source_title,
           'status', status
         ) ORDER BY edge_order) AS refs
  FROM bounded_edges
  WHERE visible_source_id IS NOT NULL
)
SELECT 'relationship_impact',
       CASE WHEN COALESCE(ie.contested, false) THEN 'contested'
            WHEN COALESCE(ie.partial, false) OR st.nodes_truncated OR st.edges_truncated OR st.traversal_saturated OR st.depth_truncated THEN 'partial'
            ELSE 'structural' END,
       sv.version_number::integer, sv.tree_id::text, sv.tree_version_id::text, sv.version_state,
       st.impact_depth,
       (st.nodes_truncated OR st.edges_truncated OR st.traversal_saturated OR st.depth_truncated),
       st.nodes_truncated, st.edges_truncated, st.traversal_saturated, st.depth_truncated,
       false, true, inodes.nodes, COALESCE(ie.edges, '[]'::jsonb), COALESCE(irefs.refs, '[]'::jsonb), sr.edge_id::text
FROM selected_relationship sr
JOIN visible_versions sv ON sv.tree_version_id = sr.tree_version_id
CROSS JOIN impact_state st
LEFT JOIN impact_nodes inodes ON TRUE
LEFT JOIN impact_edges_json ie ON TRUE
LEFT JOIN impact_refs irefs ON TRUE
`
