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

func isSourceDependencyGraphOperation(operation string) bool {
	return operation == GraphOperationSourceDependency || operation == GraphOperationSourceCommunities
}

type GraphSourceDependencyNeighborhoodInput struct {
	SourceID string `json:"source_id"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

type GraphSourceDependencyNeighborhoodLimits struct {
	MaxDepth int `json:"maxDepth"`
	MaxNodes int `json:"maxNodes"`
	MaxEdges int `json:"maxEdges"`
}

type GraphSourceDependencyDepthCount struct {
	Depth int `json:"depth"`
	Count int `json:"count"`
}

type GraphSourceDependencyStatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type GraphSourceDependencyNeighborhoodSummary struct {
	RootSourceID               string                             `json:"rootSourceId"`
	PathID                     string                             `json:"pathId"`
	InputFingerprint           string                             `json:"inputFingerprint"`
	EdgeSetFingerprint         string                             `json:"edgeSetFingerprint"`
	BoundedUpstreamSourceCount int                                `json:"boundedUpstreamSourceCount"`
	BoundedEdgeCount           int                                `json:"boundedEdgeCount"`
	MaxDepthReached            int                                `json:"maxDepthReached"`
	NodesByDepth               []GraphSourceDependencyDepthCount  `json:"nodesByDepth"`
	EdgeStatusCounts           []GraphSourceDependencyStatusCount `json:"edgeStatusCounts"`
	Truncated                  bool                               `json:"truncated"`
	TruncationReasons          []string                           `json:"truncationReasons"`
	CycleDetected              bool                               `json:"cycleDetected"`
	Status                     string                             `json:"status"`
}

type GraphSourceDependencyNeighborhoodResult struct {
	RunID            string                                   `json:"runId"`
	CreatedAt        time.Time                                `json:"createdAt"`
	Operation        string                                   `json:"operation"`
	AlgorithmVersion string                                   `json:"algorithmVersion"`
	StructuralOnly   bool                                     `json:"structuralOnly"`
	Limits           GraphSourceDependencyNeighborhoodLimits  `json:"limits"`
	Summary          GraphSourceDependencyNeighborhoodSummary `json:"summary"`
	Path             GraphPath                                `json:"path"`
}

type graphSourceDependencyRow struct {
	Path               GraphPath
	NodesTruncated     bool
	EdgesTruncated     bool
	TraversalSaturated bool
	DepthTruncated     bool
	CycleDetected      bool
}

func (s *Service) GraphSourceDependencyNeighborhood(ctx context.Context, input GraphSourceDependencyNeighborhoodInput, actorID string) (GraphSourceDependencyNeighborhoodResult, error) {
	if err := s.ready(); err != nil {
		return GraphSourceDependencyNeighborhoodResult{}, err
	}
	normalized, err := normalizeGraphSourceDependencyNeighborhoodInput(input)
	if err != nil {
		return GraphSourceDependencyNeighborhoodResult{}, err
	}
	row, err := s.retrieveGraphSourceDependencyNeighborhood(ctx, normalized, actorID)
	if err != nil {
		return GraphSourceDependencyNeighborhoodResult{}, err
	}
	runID, createdAt, err := s.startRun(ctx, QueryInput{Question: "حيّز اعتماد المصادر", GraphOperation: GraphOperationSourceDependency, GraphStartType: "source", GraphStartID: normalized.SourceID, SourceID: normalized.SourceID, GraphMaxDepth: normalized.MaxDepth}, actorID)
	if err != nil {
		return GraphSourceDependencyNeighborhoodResult{}, err
	}
	summary := summarizeGraphSourceDependencyNeighborhood(row, graphSourceDependencyInputFingerprint(normalized))
	limits := GraphSourceDependencyNeighborhoodLimits{MaxDepth: normalized.MaxDepth, MaxNodes: GraphMaxNodes, MaxEdges: GraphMaxEdges}
	result := GraphSourceDependencyNeighborhoodResult{RunID: runID, CreatedAt: createdAt, Operation: GraphOperationSourceDependency, AlgorithmVersion: GraphSourceDependencyAlgorithm, StructuralOnly: true, Limits: limits, Summary: summary, Path: row.Path}
	if len(row.Path.Edges) == 0 {
		row.Path.Explanation = "لم يُعثر على علاقة اعتماد نشطة ضمن النطاق المرئي؛ هذا لا يثبت استقلال المصدر."
		result.Path = row.Path
	}
	if err := s.persistGraphSourceDependencyNeighborhoodRun(ctx, runID, row, result); err != nil {
		_ = s.failRun(ctx, runID, err)
		return GraphSourceDependencyNeighborhoodResult{}, err
	}
	return result, nil
}

func (s *Service) retrieveGraphSourceDependencyNeighborhood(ctx context.Context, normalized GraphSourceDependencyNeighborhoodInput, actorID string) (graphSourceDependencyRow, error) {
	actorUUID, err := graphActorUUID(actorID)
	if err != nil {
		return graphSourceDependencyRow{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()
	tx, err := s.Pool.BeginTx(queryCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return graphSourceDependencyRow{}, graphRetrievalError(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(queryCtx, `SET LOCAL statement_timeout = '2000ms'`); err != nil {
		return graphSourceDependencyRow{}, graphRetrievalError(err)
	}
	if err := validateGraphSourceDependencyRoot(queryCtx, tx, normalized.SourceID); err != nil {
		return graphSourceDependencyRow{}, err
	}
	row, err := queryGraphSourceDependencyNeighborhood(queryCtx, tx, normalized.SourceID, actorUUID, normalized.MaxDepth)
	if err != nil {
		return graphSourceDependencyRow{}, err
	}
	if err := tx.Rollback(context.Background()); err != nil {
		return graphSourceDependencyRow{}, graphRetrievalError(err)
	}
	return row, nil
}

func normalizeGraphSourceDependencyNeighborhoodInput(input GraphSourceDependencyNeighborhoodInput) (GraphSourceDependencyNeighborhoodInput, error) {
	input.SourceID = strings.TrimSpace(input.SourceID)
	if input.SourceID == "" {
		return GraphSourceDependencyNeighborhoodInput{}, ErrValidation
	}
	parsed, err := normalizeGraphUUID(input.SourceID)
	if err != nil {
		return GraphSourceDependencyNeighborhoodInput{}, err
	}
	input.SourceID = parsed
	if input.MaxDepth < 0 {
		return GraphSourceDependencyNeighborhoodInput{}, ErrValidation
	}
	if input.MaxDepth == 0 {
		input.MaxDepth = GraphDefaultDepth
	}
	if input.MaxDepth > GraphMaxDepth {
		input.MaxDepth = GraphMaxDepth
	}
	return input, nil
}

func validateGraphSourceDependencyRoot(ctx context.Context, tx pgx.Tx, sourceID string) error {
	var visibility string
	err := tx.QueryRow(ctx, `SELECT visibility FROM sources WHERE id = $1`, sourceID).Scan(&visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if visibility != "public" {
		return ErrNotFound
	}
	return nil
}

func queryGraphSourceDependencyNeighborhood(ctx context.Context, tx pgx.Tx, sourceID string, actorID uuid.UUID, maxDepth int) (graphSourceDependencyRow, error) {
	row := tx.QueryRow(ctx, sourceDependencyNeighborhoodGraphQuery, sourceID, nullableUUID(actorID), maxDepth, GraphMaxNodes, GraphMaxEdges)
	return scanGraphSourceDependencyNeighborhoodRow(row, sourceID)
}

func scanGraphSourceDependencyNeighborhoodRow(row pgx.Row, rootSourceID string) (graphSourceDependencyRow, error) {
	var depth int32
	var scannedRootID string
	var truncated, nodesTruncated, edgesTruncated, traversalSaturated, depthTruncated bool
	var nodes, edges []byte
	if err := row.Scan(&scannedRootID, &depth, &truncated, &nodesTruncated, &edgesTruncated, &traversalSaturated, &depthTruncated, &nodes, &edges); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return graphSourceDependencyRow{}, ErrNotFound
		}
		return graphSourceDependencyRow{}, graphRetrievalError(err)
	}
	path := GraphPath{Operation: GraphOperationSourceDependency, Status: "structural", Explanation: graphExplanation(GraphOperationSourceDependency, "structural", true), Depth: int(depth), Truncated: truncated, EvidenceBacked: false, StructuralOnly: true, AlgorithmVersion: GraphSourceDependencyAlgorithm}
	if err := json.Unmarshal(nodes, &path.Nodes); err != nil {
		return graphSourceDependencyRow{}, graphRetrievalError(err)
	}
	if err := json.Unmarshal(edges, &path.Edges); err != nil {
		return graphSourceDependencyRow{}, graphRetrievalError(err)
	}
	path.EvidenceRefs = []GraphEvidenceRef{}
	normalized := normalizeGraphPaths([]GraphPath{path})
	path = normalized[0]
	cycleDetected := graphSourceDependencyHasCycle(path)
	if path.Truncated || cycleDetected || hasSourceDependencyUnresolvedStatus(path.Edges) {
		path.Status = "partial"
	}
	path.Explanation = graphExplanation(path.Operation, path.Status, path.StructuralOnly)
	path.ID = graphSourceDependencyPathID(rootSourceID, path, nodes, edges, truncated, nodesTruncated, edgesTruncated, traversalSaturated, depthTruncated, cycleDetected)
	return graphSourceDependencyRow{Path: path, NodesTruncated: nodesTruncated, EdgesTruncated: edgesTruncated, TraversalSaturated: traversalSaturated, DepthTruncated: depthTruncated, CycleDetected: cycleDetected}, nil
}

func hasSourceDependencyUnresolvedStatus(edges []GraphEdge) bool {
	for _, edge := range edges {
		if edge.Status == "needs_review" || edge.Status == "unresolved" {
			return true
		}
	}
	return false
}

func graphSourceDependencyPathID(rootSourceID string, path GraphPath, nodes, edges []byte, truncated, nodesTruncated, edgesTruncated, traversalSaturated, depthTruncated, cycleDetected bool) string {
	key := strings.Join([]string{path.Operation, rootSourceID, path.Status, fmt.Sprintf("%d", path.Depth), fmt.Sprintf("%t", path.Truncated), fmt.Sprintf("%t", truncated), fmt.Sprintf("%t", nodesTruncated), fmt.Sprintf("%t", edgesTruncated), fmt.Sprintf("%t", traversalSaturated), fmt.Sprintf("%t", depthTruncated), fmt.Sprintf("%t", cycleDetected), string(nodes), string(edges)}, "|")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

func graphSourceDependencyHasCycle(path GraphPath) bool {
	if len(path.Nodes) == 0 || len(path.Edges) == 0 {
		return false
	}
	indegree := make(map[string]int, len(path.Nodes))
	adjacency := make(map[string][]string, len(path.Nodes))
	for _, node := range path.Nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range path.Edges {
		if _, exists := indegree[edge.FromNodeID]; !exists {
			continue
		}
		if _, exists := indegree[edge.ToNodeID]; !exists {
			continue
		}
		adjacency[edge.FromNodeID] = append(adjacency[edge.FromNodeID], edge.ToNodeID)
		indegree[edge.ToNodeID]++
	}
	queue := make([]string, 0, len(indegree))
	for nodeID, degree := range indegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}
	sort.Strings(queue)
	visited := 0
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		visited++
		for _, childID := range adjacency[nodeID] {
			indegree[childID]--
			if indegree[childID] == 0 {
				queue = append(queue, childID)
				sort.Strings(queue)
			}
		}
	}
	return visited != len(indegree)
}

func graphSourceDependencyTruncationReasons(row graphSourceDependencyRow) []string {
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
	return reasons
}

func summarizeGraphSourceDependencyNeighborhood(row graphSourceDependencyRow, fingerprint string) GraphSourceDependencyNeighborhoodSummary {
	depthCounts := make(map[int]int)
	for _, node := range row.Path.Nodes {
		depthCounts[node.Depth]++
	}
	depths := make([]int, 0, len(depthCounts))
	for depth := range depthCounts {
		depths = append(depths, depth)
	}
	sort.Ints(depths)
	nodesByDepth := make([]GraphSourceDependencyDepthCount, 0, len(depths))
	for _, depth := range depths {
		nodesByDepth = append(nodesByDepth, GraphSourceDependencyDepthCount{Depth: depth, Count: depthCounts[depth]})
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
	edgeStatusCounts := make([]GraphSourceDependencyStatusCount, 0, len(statuses))
	for _, status := range statuses {
		edgeStatusCounts = append(edgeStatusCounts, GraphSourceDependencyStatusCount{Status: status, Count: statusCounts[status]})
	}
	reasons := graphSourceDependencyTruncationReasons(row)
	status := row.Path.Status
	if status == "" {
		status = "structural"
	}
	upstreamCount := len(row.Path.Nodes) - 1
	if upstreamCount < 0 {
		upstreamCount = 0
	}
	return GraphSourceDependencyNeighborhoodSummary{RootSourceID: firstGraphNodeID(row.Path), PathID: row.Path.ID, InputFingerprint: fingerprint, EdgeSetFingerprint: graphSourceDependencyEdgeSetFingerprint(row.Path.Edges), BoundedUpstreamSourceCount: upstreamCount, BoundedEdgeCount: len(row.Path.Edges), MaxDepthReached: row.Path.Depth, NodesByDepth: nodesByDepth, EdgeStatusCounts: edgeStatusCounts, Truncated: row.Path.Truncated, TruncationReasons: reasons, CycleDetected: row.CycleDetected, Status: status}
}

func firstGraphNodeID(path GraphPath) string {
	if len(path.Nodes) == 0 {
		return ""
	}
	return path.Nodes[0].ID
}

func graphSourceDependencyEdgeSetFingerprint(edges []GraphEdge) string {
	values := make([]string, 0, len(edges))
	for _, edge := range edges {
		values = append(values, strings.Join([]string{edge.ID, edge.FromNodeID, edge.ToNodeID, edge.Predicate, edge.Status}, "|"))
	}
	sort.Strings(values)
	sum := sha256.Sum256([]byte(strings.Join(values, "||")))
	return hex.EncodeToString(sum[:])
}

func graphSourceDependencyInputFingerprint(input GraphSourceDependencyNeighborhoodInput) string {
	value := strings.Join([]string{input.SourceID, fmt.Sprintf("%d", input.MaxDepth), fmt.Sprintf("%d", GraphMaxNodes), fmt.Sprintf("%d", GraphMaxEdges), GraphSourceDependencyAlgorithm}, "|")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func lockGraphSourceDependencySources(ctx context.Context, tx pgx.Tx, rootSourceID string, path GraphPath) error {
	sourceSet := make(map[uuid.UUID]struct{}, len(path.Nodes)+len(path.Edges)+1)
	add := func(value string) {
		sourceID := optionalUUID(value)
		if sourceID != uuid.Nil {
			sourceSet[sourceID] = struct{}{}
		}
	}
	add(rootSourceID)
	for _, node := range path.Nodes {
		if node.Type == "source" {
			add(node.ID)
		}
	}
	for _, edge := range path.Edges {
		add(edge.FromNodeID)
		add(edge.ToNodeID)
	}
	sourceIDs := make([]uuid.UUID, 0, len(sourceSet))
	for sourceID := range sourceSet {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Slice(sourceIDs, func(left, right int) bool { return sourceIDs[left].String() < sourceIDs[right].String() })
	rows, err := tx.Query(ctx, `SELECT id FROM sources WHERE id = ANY($1::uuid[]) AND visibility = 'public' FOR SHARE`, sourceIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(sourceIDs) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) persistGraphSourceDependencyNeighborhoodRun(ctx context.Context, runID string, row graphSourceDependencyRow, result GraphSourceDependencyNeighborhoodResult) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockGraphSourceDependencySources(ctx, tx, result.Summary.RootSourceID, row.Path); err != nil {
		return err
	}
	stats := GraphStats{Operation: GraphOperationSourceDependency, PathCount: 1, NodeCount: len(row.Path.Nodes), EdgeCount: len(row.Path.Edges), Truncated: row.Path.Truncated, PathsTruncated: row.Path.Truncated, MaxDepth: result.Limits.MaxDepth, AlgorithmVersion: GraphSourceDependencyAlgorithm}
	if err := persistGraphRun(ctx, tx, runID, QueryResult{GraphPaths: []GraphPath{row.Path}, GraphStats: stats}); err != nil {
		return err
	}
	pathID, err := lookupGraphPathID(ctx, tx, runID, row.Path.ID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO research_graph_source_dependency_neighborhoods (run_id, path_id, root_source_id, algorithm_version, input_fingerprint, edge_set_fingerprint, limits, summary, status, truncated, truncation_reasons) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, runID, pathID, result.Summary.RootSourceID, result.AlgorithmVersion, result.Summary.InputFingerprint, result.Summary.EdgeSetFingerprint, mustJSON(result.Limits), mustJSON(result.Summary), result.Summary.Status, result.Summary.Truncated, mustJSON(result.Summary.TruncationReasons)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE research_runs SET status = 'succeeded', graph_truncated = $1, updated_at = now() WHERE id = $2`, result.Summary.Truncated, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func runGraphSourceDependencyNeighborhood(ctx context.Context, executor historyExecutor, runID uuid.UUID) (*GraphSourceDependencyNeighborhoodSummary, error) {
	var inputFingerprint, edgeSetFingerprint, status, pathKey string
	var summary, reasons []byte
	var truncated bool
	err := executor.QueryRow(ctx, `SELECT a.input_fingerprint, a.edge_set_fingerprint, a.summary, a.status, a.truncated, a.truncation_reasons, p.path_key FROM research_graph_source_dependency_neighborhoods a JOIN research_graph_paths p ON p.id = a.path_id WHERE a.run_id = $1`, runID).Scan(&inputFingerprint, &edgeSetFingerprint, &summary, &status, &truncated, &reasons, &pathKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := &GraphSourceDependencyNeighborhoodSummary{PathID: pathKey, InputFingerprint: inputFingerprint, EdgeSetFingerprint: edgeSetFingerprint, Status: status, Truncated: truncated}
	if err := json.Unmarshal(summary, result); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(reasons, &result.TruncationReasons); err != nil {
		return nil, err
	}
	result.PathID = pathKey
	result.InputFingerprint = inputFingerprint
	result.EdgeSetFingerprint = edgeSetFingerprint
	result.Status = status
	result.Truncated = truncated
	return result, nil
}

const sourceDependencyNeighborhoodGraphQuery = `
WITH RECURSIVE
parameters AS (
  SELECT $1::uuid AS root_source_id, $2::uuid AS actor_id, $3::integer AS max_depth, $4::integer AS max_nodes, $5::integer AS max_edges
),
visible_root AS (
  SELECT s.id AS source_id FROM sources s CROSS JOIN parameters p WHERE s.id = p.root_source_id AND s.visibility = 'public'
),
active_edges AS (
  SELECT d.id AS edge_id, d.source_id, d.depends_on_source_id, d.dependency_type, d.status
  FROM source_dependencies d
  JOIN sources dependent ON dependent.id = d.source_id AND dependent.visibility = 'public'
  JOIN sources target ON target.id = d.depends_on_source_id AND target.visibility = 'public'
  WHERE d.status IN ('needs_review', 'confirmed') AND d.depends_on_source_id IS NOT NULL
),
bfs (depth, frontier, visited, node_count, saturated) AS (
  SELECT 0, ARRAY[source_id]::uuid[], ARRAY[source_id]::uuid[], 1, false FROM visible_root
  UNION ALL
  SELECT next_bfs.depth, next_bfs.frontier, next_bfs.visited, next_bfs.node_count, next_bfs.saturated
  FROM (
    SELECT b.depth + 1 AS depth, next_nodes.frontier, b.visited || next_nodes.frontier AS visited,
           b.node_count + next_nodes.selected_count AS node_count, b.saturated OR next_nodes.overflow AS saturated
    FROM bfs b
    CROSS JOIN LATERAL (
      WITH candidates AS (
        SELECT DISTINCT e.depends_on_source_id AS source_id
        FROM active_edges e
        WHERE e.source_id = ANY(b.frontier) AND NOT e.depends_on_source_id = ANY(b.visited)
        ORDER BY e.depends_on_source_id
        LIMIT (GREATEST($4 - b.node_count, 0) + 1)
      )
      SELECT COALESCE(array_agg(selected.source_id ORDER BY selected.source_id), ARRAY[]::uuid[]) AS frontier,
             count(selected.source_id)::integer AS selected_count,
             (SELECT count(*) FROM candidates) > GREATEST($4 - b.node_count, 0) AS overflow
      FROM (
        SELECT source_id FROM candidates ORDER BY source_id LIMIT GREATEST($4 - b.node_count, 0)
      ) selected
    ) next_nodes
    WHERE b.depth < $3 AND next_nodes.selected_count > 0
  ) next_bfs
),
level_nodes AS (
  SELECT source_id, 0 AS depth FROM visible_root
  UNION ALL
  SELECT unnest(frontier) AS source_id, depth FROM bfs
),
reachable_nodes AS (
  SELECT source_id, min(depth)::integer AS depth FROM level_nodes GROUP BY source_id
),
ranked_nodes AS (
  SELECT source_id, depth, row_number() OVER (ORDER BY depth, source_id) AS node_order, count(*) OVER () AS node_count
  FROM reachable_nodes
),
bounded_nodes AS (
  SELECT source_id, depth, node_order FROM ranked_nodes WHERE node_order <= $4
),
edge_candidates AS (
  SELECT e.edge_id, e.source_id, e.depends_on_source_id, e.dependency_type, e.status
  FROM active_edges e
  JOIN bounded_nodes dependent ON dependent.source_id = e.source_id
  JOIN bounded_nodes target ON target.source_id = e.depends_on_source_id
  ORDER BY e.edge_id
  LIMIT ($5 + 1)
),
neighborhood_edges AS (
  SELECT edge_id, source_id, depends_on_source_id, dependency_type, status,
         row_number() OVER (ORDER BY edge_id) AS edge_order
  FROM edge_candidates
),
bounded_edges AS (
  SELECT edge_id, source_id, depends_on_source_id, dependency_type, status, edge_order FROM neighborhood_edges WHERE edge_order <= $5
),
neighborhood_state AS (
  SELECT
    (SELECT COALESCE(max(depth), 0)::integer FROM bounded_nodes) AS neighborhood_depth,
    (SELECT count(*) > $4 FROM ranked_nodes) AS nodes_truncated,
    (SELECT count(*) > $5 FROM neighborhood_edges) AS edges_truncated,
    COALESCE((SELECT bool_or(saturated) FROM bfs), false) AS traversal_saturated,
    EXISTS (SELECT 1 FROM bfs b WHERE b.depth = $3 AND EXISTS (SELECT 1 FROM active_edges e WHERE e.source_id = ANY(b.frontier) AND NOT e.depends_on_source_id = ANY(b.visited))) AS depth_truncated
),
neighborhood_nodes AS (
  SELECT jsonb_agg(jsonb_build_object('id', source_id::text, 'type', 'source', 'depth', depth, 'position', node_order - 1) ORDER BY node_order) AS nodes
  FROM bounded_nodes
),
neighborhood_edges_json AS (
  SELECT jsonb_agg(jsonb_build_object('id', edge_id::text, 'type', 'source_dependency', 'fromNodeId', source_id::text, 'toNodeId', depends_on_source_id::text, 'pathFromNodeId', source_id::text, 'pathToNodeId', depends_on_source_id::text, 'predicate', dependency_type, 'status', status, 'position', edge_order - 1) ORDER BY edge_order) AS edges
  FROM bounded_edges
)
SELECT r.source_id::text, st.neighborhood_depth,
       (st.nodes_truncated OR st.edges_truncated OR st.traversal_saturated OR st.depth_truncated),
       st.nodes_truncated, st.edges_truncated, st.traversal_saturated, st.depth_truncated,
       COALESCE(nn.nodes, '[]'::jsonb), COALESCE(ne.edges, '[]'::jsonb)
FROM visible_root r CROSS JOIN neighborhood_state st
LEFT JOIN neighborhood_nodes nn ON TRUE
LEFT JOIN neighborhood_edges_json ne ON TRUE
`
