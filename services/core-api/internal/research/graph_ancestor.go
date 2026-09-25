package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GraphAncestorFrontierInput struct {
	TreeID        string `json:"tree_id"`
	TreeVersionID string `json:"tree_version_id"`
	RootNodeID    string `json:"root_node_id"`
	MaxDepth      int    `json:"max_depth,omitempty"`
}

type GraphAncestorFrontierLimits struct {
	MaxDepth int `json:"maxDepth"`
	MaxNodes int `json:"maxNodes"`
	MaxEdges int `json:"maxEdges"`
}

type GraphAncestorDepthCount struct {
	Depth int `json:"depth"`
	Count int `json:"count"`
}

type GraphAncestorStatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type GraphAncestorFrontierSummary struct {
	TreeScope            GraphTreeScope             `json:"treeScope"`
	RootNodeID           string                     `json:"rootNodeId"`
	PathID               string                     `json:"pathId"`
	InputFingerprint     string                     `json:"inputFingerprint"`
	BoundedAncestorCount int                        `json:"boundedAncestorCount"`
	BoundedEdgeCount     int                        `json:"boundedEdgeCount"`
	MaxDepthReached      int                        `json:"maxDepthReached"`
	NodesByDepth         []GraphAncestorDepthCount  `json:"nodesByDepth"`
	EdgeStatusCounts     []GraphAncestorStatusCount `json:"edgeStatusCounts"`
	Truncated            bool                       `json:"truncated"`
	TruncationReasons    []string                   `json:"truncationReasons"`
	CycleDetected        bool                       `json:"cycleDetected"`
	Status               string                     `json:"status"`
}

type GraphAncestorFrontierResult struct {
	RunID            string                       `json:"runId"`
	CreatedAt        time.Time                    `json:"createdAt"`
	Operation        string                       `json:"operation"`
	AlgorithmVersion string                       `json:"algorithmVersion"`
	StructuralOnly   bool                         `json:"structuralOnly"`
	Limits           GraphAncestorFrontierLimits  `json:"limits"`
	Summary          GraphAncestorFrontierSummary `json:"summary"`
	Path             GraphPath                    `json:"path"`
}

type graphAncestorFrontierRow struct {
	Path               GraphPath
	NodesTruncated     bool
	EdgesTruncated     bool
	TraversalSaturated bool
	DepthTruncated     bool
	CycleDetected      bool
}

func (s *Service) GraphAncestorFrontier(ctx context.Context, input GraphAncestorFrontierInput, actorID string) (GraphAncestorFrontierResult, error) {
	if err := s.ready(); err != nil {
		return GraphAncestorFrontierResult{}, err
	}
	normalized, err := normalizeGraphAncestorFrontierInput(input)
	if err != nil {
		return GraphAncestorFrontierResult{}, err
	}
	actorUUID, err := graphActorUUID(actorID)
	if err != nil {
		return GraphAncestorFrontierResult{}, err
	}
	scope := graphBranchScope{TreeID: normalized.TreeID, TreeVersionID: normalized.TreeVersionID, RootNodeID: normalized.RootNodeID}
	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()
	tx, err := s.Pool.BeginTx(queryCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return GraphAncestorFrontierResult{}, graphRetrievalError(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(queryCtx, `SET LOCAL statement_timeout = '2000ms'`); err != nil {
		return GraphAncestorFrontierResult{}, graphRetrievalError(err)
	}
	if err := validateGraphBranchStructureScope(queryCtx, tx, scope, actorUUID); err != nil {
		return GraphAncestorFrontierResult{}, err
	}
	row, err := queryGraphAncestorFrontier(queryCtx, tx, scope, actorUUID, normalized.MaxDepth)
	if err != nil {
		return GraphAncestorFrontierResult{}, err
	}
	if err := tx.Rollback(context.Background()); err != nil {
		return GraphAncestorFrontierResult{}, graphRetrievalError(err)
	}
	runID, createdAt, err := s.startRun(ctx, QueryInput{Question: "إطار الأسلاف", GraphOperation: GraphOperationAncestorFrontier, GraphStartType: "tree_node", GraphStartID: normalized.RootNodeID, TreeID: normalized.TreeID, TreeVersionID: normalized.TreeVersionID, GraphMaxDepth: normalized.MaxDepth}, actorID)
	if err != nil {
		return GraphAncestorFrontierResult{}, err
	}
	summary := summarizeGraphAncestorFrontier(row, graphAncestorFrontierFingerprint(normalized))
	limits := GraphAncestorFrontierLimits{MaxDepth: normalized.MaxDepth, MaxNodes: GraphMaxNodes, MaxEdges: GraphMaxEdges}
	result := GraphAncestorFrontierResult{RunID: runID, CreatedAt: createdAt, Operation: GraphOperationAncestorFrontier, AlgorithmVersion: GraphAncestorFrontierAlgorithm, StructuralOnly: true, Limits: limits, Summary: summary, Path: row.Path}
	if len(row.Path.Edges) == 0 {
		row.Path.Explanation = "لم يُعثر على علاقة parent_of في النسخة المنشورة المحددة."
		result.Path = row.Path
		result.Summary.Status = "structural"
	}
	if err := s.persistGraphAncestorFrontierRun(ctx, runID, row, result); err != nil {
		_ = s.failRun(ctx, runID, err)
		return GraphAncestorFrontierResult{}, err
	}
	return result, nil
}

func normalizeGraphAncestorFrontierInput(input GraphAncestorFrontierInput) (GraphAncestorFrontierInput, error) {
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	input.RootNodeID = strings.TrimSpace(input.RootNodeID)
	if input.TreeID == "" || input.TreeVersionID == "" || input.RootNodeID == "" {
		return GraphAncestorFrontierInput{}, ErrValidation
	}
	var err error
	input.TreeID, err = normalizeGraphUUID(input.TreeID)
	if err != nil {
		return GraphAncestorFrontierInput{}, err
	}
	input.TreeVersionID, err = normalizeGraphUUID(input.TreeVersionID)
	if err != nil {
		return GraphAncestorFrontierInput{}, err
	}
	input.RootNodeID, err = normalizeGraphUUID(input.RootNodeID)
	if err != nil {
		return GraphAncestorFrontierInput{}, err
	}
	if input.MaxDepth < 0 {
		return GraphAncestorFrontierInput{}, ErrValidation
	}
	if input.MaxDepth == 0 {
		input.MaxDepth = GraphDefaultDepth
	}
	if input.MaxDepth > GraphMaxDepth {
		input.MaxDepth = GraphMaxDepth
	}
	return input, nil
}

func queryGraphAncestorFrontier(ctx context.Context, tx pgx.Tx, scope graphBranchScope, actorID uuid.UUID, maxDepth int) (graphAncestorFrontierRow, error) {
	row := tx.QueryRow(ctx, ancestorFrontierGraphQuery, scope.RootNodeID, nullableUUID(actorID), scope.TreeID, scope.TreeVersionID, maxDepth, GraphMaxNodes, GraphMaxEdges)
	return scanGraphAncestorFrontierRow(row, scope.RootNodeID)
}

func scanGraphAncestorFrontierRow(row pgx.Row, rootNodeID string) (graphAncestorFrontierRow, error) {
	var versionNumber, depth int32
	var treeID, treeVersionID, versionState, scannedRootNodeID string
	var truncated, nodesTruncated, edgesTruncated, traversalSaturated, depthTruncated, cycleDetected bool
	var nodes, edges []byte
	if err := row.Scan(&versionNumber, &treeID, &treeVersionID, &versionState, &scannedRootNodeID, &depth, &truncated, &nodesTruncated, &edgesTruncated, &traversalSaturated, &depthTruncated, &cycleDetected, &nodes, &edges); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return graphAncestorFrontierRow{}, ErrNotFound
		}
		return graphAncestorFrontierRow{}, graphRetrievalError(err)
	}
	path := GraphPath{Operation: GraphOperationAncestorFrontier, Status: "structural", Explanation: graphExplanation(GraphOperationAncestorFrontier, "structural", true), Depth: int(depth), Truncated: truncated, EvidenceBacked: false, StructuralOnly: true, TreeScope: GraphTreeScope{TreeID: treeID, TreeVersionID: treeVersionID, VersionNumber: int(versionNumber), VersionState: versionState}, AlgorithmVersion: GraphAncestorFrontierAlgorithm}
	if err := json.Unmarshal(nodes, &path.Nodes); err != nil {
		return graphAncestorFrontierRow{}, graphRetrievalError(err)
	}
	if err := json.Unmarshal(edges, &path.Edges); err != nil {
		return graphAncestorFrontierRow{}, graphRetrievalError(err)
	}
	path.EvidenceRefs = []GraphEvidenceRef{}
	path.ID = graphAncestorFrontierPathID(rootNodeID, path, nodes, edges)
	normalized := normalizeGraphPaths([]GraphPath{path})
	path = normalized[0]
	if hasContestedGraphStatus(path.Edges) {
		path.Status = "contested"
	} else if hasUnresolvedGraphStatus(path.Edges) || path.Truncated || cycleDetected {
		path.Status = "partial"
	}
	path.Explanation = graphExplanation(path.Operation, path.Status, path.StructuralOnly)
	return graphAncestorFrontierRow{Path: path, NodesTruncated: nodesTruncated, EdgesTruncated: edgesTruncated, TraversalSaturated: traversalSaturated, DepthTruncated: depthTruncated, CycleDetected: cycleDetected}, nil
}

func graphAncestorFrontierPathID(rootNodeID string, path GraphPath, nodes, edges []byte) string {
	key := strings.Join([]string{path.Operation, path.TreeScope.TreeID, path.TreeScope.TreeVersionID, rootNodeID, string(nodes), string(edges)}, "|")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

func summarizeGraphAncestorFrontier(row graphAncestorFrontierRow, fingerprint string) GraphAncestorFrontierSummary {
	depthCounts := make(map[int]int)
	for _, node := range row.Path.Nodes {
		depthCounts[node.Depth]++
	}
	depths := make([]int, 0, len(depthCounts))
	for depth := range depthCounts {
		depths = append(depths, depth)
	}
	sort.Ints(depths)
	nodesByDepth := make([]GraphAncestorDepthCount, 0, len(depths))
	for _, depth := range depths {
		nodesByDepth = append(nodesByDepth, GraphAncestorDepthCount{Depth: depth, Count: depthCounts[depth]})
	}
	statusCounts := make(map[string]int)
	for _, edge := range row.Path.Edges {
		statusCounts[edge.Status]++
	}
	statuses := make([]string, 0, len(statusCounts))
	for status := range statusCounts {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	edgeStatusCounts := make([]GraphAncestorStatusCount, 0, len(statuses))
	for _, status := range statuses {
		edgeStatusCounts = append(edgeStatusCounts, GraphAncestorStatusCount{Status: status, Count: statusCounts[status]})
	}
	reasons := make([]string, 0, 3)
	if row.DepthTruncated {
		reasons = append(reasons, "depth_limit")
	}
	if row.NodesTruncated || row.TraversalSaturated {
		reasons = append(reasons, "node_limit")
	}
	if row.EdgesTruncated {
		reasons = append(reasons, "edge_limit")
	}
	status := row.Path.Status
	if status == "" {
		status = "structural"
	}
	ancestorCount := len(row.Path.Nodes) - 1
	if ancestorCount < 0 {
		ancestorCount = 0
	}
	rootID := ""
	if len(row.Path.Nodes) > 0 {
		rootID = row.Path.Nodes[0].ID
	}
	return GraphAncestorFrontierSummary{TreeScope: row.Path.TreeScope, RootNodeID: rootID, PathID: row.Path.ID, InputFingerprint: fingerprint, BoundedAncestorCount: ancestorCount, BoundedEdgeCount: len(row.Path.Edges), MaxDepthReached: row.Path.Depth, NodesByDepth: nodesByDepth, EdgeStatusCounts: edgeStatusCounts, Truncated: row.Path.Truncated, TruncationReasons: reasons, CycleDetected: row.CycleDetected, Status: status}
}

func graphAncestorFrontierFingerprint(input GraphAncestorFrontierInput) string {
	value := strings.Join([]string{input.TreeID, input.TreeVersionID, input.RootNodeID, fmt.Sprintf("%d", input.MaxDepth), fmt.Sprintf("%d", GraphMaxNodes), fmt.Sprintf("%d", GraphMaxEdges), GraphAncestorFrontierAlgorithm}, "|")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) persistGraphAncestorFrontierRun(ctx context.Context, runID string, row graphAncestorFrontierRow, result GraphAncestorFrontierResult) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	stats := GraphStats{Operation: GraphOperationAncestorFrontier, PathCount: 1, NodeCount: len(row.Path.Nodes), EdgeCount: len(row.Path.Edges), Truncated: row.Path.Truncated, PathsTruncated: row.Path.Truncated, MaxDepth: result.Limits.MaxDepth, AlgorithmVersion: GraphAncestorFrontierAlgorithm}
	if err := persistGraphRun(ctx, tx, runID, QueryResult{GraphPaths: []GraphPath{row.Path}, GraphStats: stats}); err != nil {
		return err
	}
	pathID, err := lookupGraphPathID(ctx, tx, runID, row.Path.ID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO research_graph_ancestor_frontiers (run_id, path_id, tree_id, tree_version_id, root_node_id, algorithm_version, input_fingerprint, limits, summary, status, truncated, truncation_reasons) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, runID, pathID, result.Summary.TreeScope.TreeID, result.Summary.TreeScope.TreeVersionID, result.Summary.RootNodeID, result.AlgorithmVersion, result.Summary.InputFingerprint, mustJSON(result.Limits), mustJSON(result.Summary), result.Summary.Status, result.Summary.Truncated, mustJSON(result.Summary.TruncationReasons)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE research_runs SET status = 'succeeded', graph_truncated = $1, updated_at = now() WHERE id = $2`, result.Summary.Truncated, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func runGraphAncestorFrontier(ctx context.Context, executor historyExecutor, runID uuid.UUID) (*GraphAncestorFrontierSummary, error) {
	var inputFingerprint, status, pathKey string
	var summary, reasons []byte
	var truncated bool
	err := executor.QueryRow(ctx, `SELECT a.input_fingerprint, a.summary, a.status, a.truncated, a.truncation_reasons, p.path_key FROM research_graph_ancestor_frontiers a JOIN research_graph_paths p ON p.id = a.path_id WHERE a.run_id = $1`, runID).Scan(&inputFingerprint, &summary, &status, &truncated, &reasons, &pathKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := &GraphAncestorFrontierSummary{PathID: pathKey, InputFingerprint: inputFingerprint, Status: status, Truncated: truncated}
	if err := json.Unmarshal(summary, result); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(reasons, &result.TruncationReasons); err != nil {
		return nil, err
	}
	result.PathID = pathKey
	result.InputFingerprint = inputFingerprint
	result.Status = status
	result.Truncated = truncated
	return result, nil
}

const ancestorFrontierGraphQuery = `
WITH RECURSIVE
parameters AS (
  SELECT $1::uuid AS root_node_id, $2::uuid AS actor_id, $3::uuid AS tree_id, $4::uuid AS tree_version_id,
         $5::integer AS max_depth, $6::integer AS max_nodes, $7::integer AS max_edges
),
visible_version AS (
  SELECT tv.id AS tree_version_id, tv.tree_id, tv.version_number, tv.state AS version_state
  FROM tree_versions tv
  JOIN trees t ON t.id = tv.tree_id
  CROSS JOIN parameters p
  WHERE tv.id = p.tree_version_id
    AND t.id = p.tree_id
    AND tv.state = 'published'
    AND (t.visibility = 'public' OR (p.actor_id IS NOT NULL AND (t.owner_id = p.actor_id OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = p.actor_id))))
),
root_node AS (
  SELECT tn.id AS node_id
  FROM visible_version v
  JOIN tree_nodes tn ON tn.tree_version_id = v.tree_version_id
  CROSS JOIN parameters p
  WHERE tn.id = p.root_node_id
),
parent_edges AS (
  SELECT tr.id AS edge_id, tr.subject_node_id AS parent_node_id, tr.object_node_id AS child_node_id, tr.predicate, tr.status
  FROM tree_relationships tr
  JOIN visible_version v ON v.tree_version_id = tr.tree_version_id
  WHERE tr.predicate = 'parent_of'
    AND EXISTS (SELECT 1 FROM tree_nodes parent_node WHERE parent_node.id = tr.subject_node_id AND parent_node.tree_version_id = tr.tree_version_id)
    AND EXISTS (SELECT 1 FROM tree_nodes child_node WHERE child_node.id = tr.object_node_id AND child_node.tree_version_id = tr.tree_version_id)
),
bfs (depth, frontier, visited, saturated) AS (
  SELECT 0, ARRAY[node_id]::uuid[], ARRAY[node_id]::uuid[], false FROM root_node
  UNION ALL
  SELECT next_bfs.depth, next_bfs.frontier, next_bfs.visited, next_bfs.saturated
  FROM (
    SELECT b.depth + 1 AS depth, next_nodes.frontier, b.visited || next_nodes.frontier AS visited, b.saturated OR next_nodes.candidate_count > $6 AS saturated
    FROM bfs b
    CROSS JOIN LATERAL (
      SELECT COALESCE(array_agg(candidate.node_id ORDER BY candidate.node_id), ARRAY[]::uuid[]) AS frontier, count(*)::integer AS candidate_count
      FROM (
        SELECT DISTINCT e.parent_node_id AS node_id
        FROM parent_edges e
        WHERE e.child_node_id = ANY(b.frontier) AND NOT e.parent_node_id = ANY(b.visited)
        ORDER BY e.parent_node_id
        LIMIT ($6 + 1)
      ) candidate
    ) next_nodes
    WHERE b.depth < $5 AND next_nodes.candidate_count > 0
  ) next_bfs
),
level_nodes AS (
  SELECT node_id, 0 AS depth FROM root_node
  UNION ALL
  SELECT unnest(frontier) AS node_id, depth FROM bfs
),
reachable_nodes AS (
  SELECT node_id, min(depth)::integer AS depth FROM level_nodes GROUP BY node_id
),
ranked_nodes AS (
  SELECT node_id, depth, row_number() OVER (ORDER BY depth, node_id) AS node_order, count(*) OVER () AS node_count
  FROM reachable_nodes
),
bounded_nodes AS (
  SELECT node_id, depth, node_order FROM ranked_nodes WHERE node_order <= $6
),
frontier_edges AS (
  SELECT e.edge_id, e.parent_node_id, e.child_node_id, e.predicate, e.status,
         row_number() OVER (ORDER BY e.edge_id) AS edge_order, count(*) OVER () AS edge_count
  FROM parent_edges e
  JOIN bounded_nodes parent_node ON parent_node.node_id = e.parent_node_id
  JOIN bounded_nodes child_node ON child_node.node_id = e.child_node_id
),
bounded_edges AS (
  SELECT edge_id, parent_node_id, child_node_id, predicate, status, edge_order FROM frontier_edges WHERE edge_order <= $7
),
ancestor_state AS (
  SELECT
    (SELECT COALESCE(max(depth), 0)::integer FROM bounded_nodes) AS ancestor_depth,
    (SELECT count(*) > $6 FROM ranked_nodes) AS nodes_truncated,
    (SELECT count(*) > $7 FROM frontier_edges) AS edges_truncated,
    COALESCE((SELECT bool_or(saturated) FROM bfs), false) AS traversal_saturated,
    EXISTS (SELECT 1 FROM bfs b WHERE b.depth = $5 AND EXISTS (SELECT 1 FROM parent_edges e WHERE e.child_node_id = ANY(b.frontier) AND NOT e.parent_node_id = ANY(b.visited))) AS depth_truncated,
    EXISTS (SELECT 1 FROM bfs b JOIN parent_edges e ON e.child_node_id = ANY(b.frontier) WHERE e.parent_node_id = ANY(b.visited)) AS cycle_detected
),
ancestor_nodes AS (
  SELECT jsonb_agg(jsonb_build_object('id', tn.id::text, 'type', 'person', 'depth', bn.depth, 'position', bn.node_order - 1) ORDER BY bn.node_order) AS nodes
  FROM bounded_nodes bn JOIN tree_nodes tn ON tn.id = bn.node_id
),
ancestor_edges AS (
  SELECT jsonb_agg(jsonb_build_object('id', edge_id::text, 'type', 'tree_relationship', 'fromNodeId', parent_node_id::text, 'toNodeId', child_node_id::text, 'pathFromNodeId', child_node_id::text, 'pathToNodeId', parent_node_id::text, 'predicate', predicate, 'status', status, 'treeRelationshipId', edge_id::text, 'position', edge_order - 1) ORDER BY edge_order) AS edges
  FROM bounded_edges
)
SELECT v.version_number::integer, v.tree_id::text, v.tree_version_id::text, v.version_state,
       r.node_id::text, st.ancestor_depth,
       (st.nodes_truncated OR st.edges_truncated OR st.traversal_saturated OR st.depth_truncated),
       st.nodes_truncated, st.edges_truncated, st.traversal_saturated, st.depth_truncated, st.cycle_detected,
       COALESCE(an.nodes, '[]'::jsonb), COALESCE(ae.edges, '[]'::jsonb)
FROM visible_version v CROSS JOIN root_node r CROSS JOIN ancestor_state st
LEFT JOIN ancestor_nodes an ON TRUE
LEFT JOIN ancestor_edges ae ON TRUE
`
