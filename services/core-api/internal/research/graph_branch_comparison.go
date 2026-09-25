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

type GraphBranchStructureComparisonInput struct {
	FromTreeID        string `json:"from_tree_id"`
	FromTreeVersionID string `json:"from_tree_version_id"`
	FromRootNodeID    string `json:"from_root_node_id"`
	ToTreeID          string `json:"to_tree_id"`
	ToTreeVersionID   string `json:"to_tree_version_id"`
	ToRootNodeID      string `json:"to_root_node_id"`
	MaxDepth          int    `json:"max_depth,omitempty"`
}

type GraphBranchStructureComparisonLimits struct {
	MaxDepth        int `json:"maxDepth"`
	MaxNodesPerSide int `json:"maxNodesPerSide"`
	MaxEdgesPerSide int `json:"maxEdgesPerSide"`
}

type GraphBranchStructureDepthCount struct {
	Depth int `json:"depth"`
	Count int `json:"count"`
}

type GraphBranchStructureChildCount struct {
	ChildCount int `json:"childCount"`
	NodeCount  int `json:"nodeCount"`
}

type GraphBranchStructureStatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type GraphBranchStructureSide struct {
	PathID              string                            `json:"pathId"`
	TreeScope           GraphTreeScope                    `json:"treeScope"`
	RootNodeID          string                            `json:"rootNodeId"`
	NodeCount           int                               `json:"nodeCount"`
	EdgeCount           int                               `json:"edgeCount"`
	MaxDepth            int                               `json:"maxDepth"`
	LeafCount           int                               `json:"leafCount"`
	NodesByDepth        []GraphBranchStructureDepthCount  `json:"nodesByDepth"`
	ChildCountHistogram []GraphBranchStructureChildCount  `json:"childCountHistogram"`
	EdgeStatusCounts    []GraphBranchStructureStatusCount `json:"edgeStatusCounts"`
	Truncated           bool                              `json:"truncated"`
	TruncationReasons   []string                          `json:"truncationReasons"`
	CycleDetected       bool                              `json:"cycleDetected"`
}

type GraphBranchStructureDelta struct {
	NodeCount int `json:"nodeCount"`
	EdgeCount int `json:"edgeCount"`
	MaxDepth  int `json:"maxDepth"`
	LeafCount int `json:"leafCount"`
}

type GraphBranchStructureTruncation struct {
	From bool `json:"from"`
	To   bool `json:"to"`
}

type GraphBranchStructureComparisonResult struct {
	RunID             string                               `json:"runId"`
	CreatedAt         time.Time                            `json:"createdAt"`
	Operation         string                               `json:"operation"`
	AlgorithmVersion  string                               `json:"algorithmVersion"`
	StructuralOnly    bool                                 `json:"structuralOnly"`
	InputFingerprint  string                               `json:"inputFingerprint"`
	From              GraphBranchStructureSide             `json:"from"`
	To                GraphBranchStructureSide             `json:"to"`
	Delta             GraphBranchStructureDelta            `json:"delta"`
	Limits            GraphBranchStructureComparisonLimits `json:"limits"`
	Truncated         GraphBranchStructureTruncation       `json:"truncated"`
	TruncationReasons []string                             `json:"truncationReasons"`
	Status            string                               `json:"status"`
	Explanation       string                               `json:"explanation"`
}

type graphBranchScope struct {
	TreeID        string
	TreeVersionID string
	RootNodeID    string
}

type graphBranchSideResult struct {
	Path               GraphPath
	RootNodeID         string
	NodesTruncated     bool
	EdgesTruncated     bool
	TraversalSaturated bool
	DepthTruncated     bool
	CycleDetected      bool
}

func (s *Service) GraphBranchStructureComparison(ctx context.Context, input GraphBranchStructureComparisonInput, actorID string) (GraphBranchStructureComparisonResult, error) {
	if err := s.ready(); err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	normalized, err := normalizeGraphBranchStructureComparisonInput(input)
	if err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	actorUUID, err := graphActorUUID(actorID)
	if err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	fromScope := graphBranchScope{TreeID: normalized.FromTreeID, TreeVersionID: normalized.FromTreeVersionID, RootNodeID: normalized.FromRootNodeID}
	toScope := graphBranchScope{TreeID: normalized.ToTreeID, TreeVersionID: normalized.ToTreeVersionID, RootNodeID: normalized.ToRootNodeID}
	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()
	tx, err := s.Pool.BeginTx(queryCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return GraphBranchStructureComparisonResult{}, graphRetrievalError(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(queryCtx, `SET LOCAL statement_timeout = '2000ms'`); err != nil {
		return GraphBranchStructureComparisonResult{}, graphRetrievalError(err)
	}
	if err := validateGraphBranchStructureScope(queryCtx, tx, fromScope, actorUUID); err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	if err := validateGraphBranchStructureScope(queryCtx, tx, toScope, actorUUID); err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	fromResult, err := queryGraphBranchStructureSide(queryCtx, tx, "from", fromScope, actorUUID, normalized.MaxDepth)
	if err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	toResult, err := queryGraphBranchStructureSide(queryCtx, tx, "to", toScope, actorUUID, normalized.MaxDepth)
	if err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	if err := tx.Rollback(context.Background()); err != nil {
		return GraphBranchStructureComparisonResult{}, graphRetrievalError(err)
	}
	runID, createdAt, err := s.startRun(ctx, QueryInput{Question: "مقارنة بنية الفروع", GraphOperation: GraphOperationBranchComparison, GraphStartType: "tree_node", GraphStartID: normalized.FromRootNodeID, TreeID: normalized.FromTreeID, TreeVersionID: normalized.FromTreeVersionID, GraphMaxDepth: normalized.MaxDepth}, actorID, researchRunContextInput{ScopeType: "tree", ScopeID: optionalUUID(normalized.ToTreeID), Role: "filter"}, researchRunContextInput{ScopeType: "tree_version", ScopeID: optionalUUID(normalized.ToTreeVersionID), Role: "filter"}, researchRunContextInput{ScopeType: "tree_node", ScopeID: optionalUUID(normalized.ToRootNodeID), Role: "graph_start"})
	if err != nil {
		return GraphBranchStructureComparisonResult{}, err
	}
	fromSummary := summarizeGraphBranchStructureSide(fromResult)
	toSummary := summarizeGraphBranchStructureSide(toResult)
	limits := GraphBranchStructureComparisonLimits{MaxDepth: normalized.MaxDepth, MaxNodesPerSide: GraphMaxNodes, MaxEdgesPerSide: GraphMaxEdges}
	result := GraphBranchStructureComparisonResult{RunID: runID, CreatedAt: createdAt, Operation: GraphOperationBranchComparison, AlgorithmVersion: GraphBranchComparisonAlgorithm, StructuralOnly: true, InputFingerprint: graphBranchComparisonFingerprint(normalized), From: fromSummary, To: toSummary, Delta: graphBranchStructureDelta(fromSummary, toSummary), Limits: limits, Truncated: GraphBranchStructureTruncation{From: fromSummary.Truncated, To: toSummary.Truncated}, TruncationReasons: graphBranchComparisonReasons(fromSummary, toSummary), Status: graphBranchComparisonStatus(fromSummary, toSummary), Explanation: graphExplanation(GraphOperationBranchComparison, "structural", true)}
	if err := s.persistGraphBranchStructureComparisonRun(ctx, runID, fromResult, toResult, result); err != nil {
		_ = s.failRun(ctx, runID, err)
		return GraphBranchStructureComparisonResult{}, err
	}
	return result, nil
}

func normalizeGraphBranchStructureComparisonInput(input GraphBranchStructureComparisonInput) (GraphBranchStructureComparisonInput, error) {
	values := []*string{&input.FromTreeID, &input.FromTreeVersionID, &input.FromRootNodeID, &input.ToTreeID, &input.ToTreeVersionID, &input.ToRootNodeID}
	for _, value := range values {
		*value = strings.TrimSpace(*value)
		if *value == "" {
			return GraphBranchStructureComparisonInput{}, ErrValidation
		}
		parsed, err := normalizeGraphUUID(*value)
		if err != nil {
			return GraphBranchStructureComparisonInput{}, err
		}
		*value = parsed
	}
	if input.MaxDepth < 0 {
		return GraphBranchStructureComparisonInput{}, ErrValidation
	}
	if input.MaxDepth == 0 {
		input.MaxDepth = GraphDefaultDepth
	}
	if input.MaxDepth > GraphMaxDepth {
		input.MaxDepth = GraphMaxDepth
	}
	return input, nil
}

func validateGraphBranchStructureScope(ctx context.Context, tx pgx.Tx, scope graphBranchScope, actorID uuid.UUID) error {
	var state, visibility string
	err := tx.QueryRow(ctx, `SELECT tv.state, t.visibility FROM tree_versions tv JOIN trees t ON t.id = tv.tree_id WHERE tv.id = $1 AND t.id = $2`, scope.TreeVersionID, scope.TreeID).Scan(&state, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "published" {
		return ErrValidation
	}
	if visibility != "public" {
		if actorID == uuid.Nil {
			return ErrForbidden
		}
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trees t WHERE t.id = $1 AND (t.owner_id = $2 OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2)))`, scope.TreeID, actorID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return ErrForbidden
		}
	}
	var rootExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tree_nodes WHERE id = $1 AND tree_version_id = $2)`, scope.RootNodeID, scope.TreeVersionID).Scan(&rootExists); err != nil {
		return err
	}
	if !rootExists {
		return ErrNotFound
	}
	return nil
}

func queryGraphBranchStructureSide(ctx context.Context, tx pgx.Tx, side string, scope graphBranchScope, actorID uuid.UUID, maxDepth int) (graphBranchSideResult, error) {
	row := tx.QueryRow(ctx, branchStructureSideQuery, scope.RootNodeID, nullableUUID(actorID), scope.TreeID, scope.TreeVersionID, maxDepth, GraphMaxNodes, GraphMaxEdges)
	return scanGraphBranchStructureSide(row, side, scope.RootNodeID)
}

func scanGraphBranchStructureSide(row pgx.Row, side, rootNodeID string) (graphBranchSideResult, error) {
	var versionNumber, depth int32
	var treeID, treeVersionID, versionState, scannedRootNodeID string
	var truncated, nodesTruncated, edgesTruncated, traversalSaturated, depthTruncated, cycleDetected bool
	var nodes, edges []byte
	if err := row.Scan(&versionNumber, &treeID, &treeVersionID, &versionState, &scannedRootNodeID, &depth, &truncated, &nodesTruncated, &edgesTruncated, &traversalSaturated, &depthTruncated, &cycleDetected, &nodes, &edges); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return graphBranchSideResult{}, ErrNotFound
		}
		return graphBranchSideResult{}, graphRetrievalError(err)
	}
	path := GraphPath{Operation: GraphOperationBranchComparison, Status: "structural", Explanation: graphExplanation(GraphOperationBranchComparison, "structural", true), Depth: int(depth), Truncated: truncated, EvidenceBacked: false, StructuralOnly: true, TreeScope: GraphTreeScope{TreeID: treeID, TreeVersionID: treeVersionID, VersionNumber: int(versionNumber), VersionState: versionState}, AlgorithmVersion: GraphBranchComparisonAlgorithm}
	if err := json.Unmarshal(nodes, &path.Nodes); err != nil {
		return graphBranchSideResult{}, graphRetrievalError(err)
	}
	if err := json.Unmarshal(edges, &path.Edges); err != nil {
		return graphBranchSideResult{}, graphRetrievalError(err)
	}
	path.EvidenceRefs = []GraphEvidenceRef{}
	path.ID = graphBranchPathID(side, rootNodeID, path, nodes, edges)
	normalized := normalizeGraphPaths([]GraphPath{path})
	path = normalized[0]
	if hasContestedGraphStatus(path.Edges) {
		path.Status = "contested"
	} else if hasUnresolvedGraphStatus(path.Edges) || path.Truncated || cycleDetected {
		path.Status = "partial"
	}
	path.Explanation = graphExplanation(path.Operation, path.Status, path.StructuralOnly)
	return graphBranchSideResult{Path: path, RootNodeID: scannedRootNodeID, NodesTruncated: nodesTruncated, EdgesTruncated: edgesTruncated, TraversalSaturated: traversalSaturated, DepthTruncated: depthTruncated, CycleDetected: cycleDetected}, nil
}

func graphBranchPathID(side, rootNodeID string, path GraphPath, nodes, edges []byte) string {
	key := strings.Join([]string{side, path.Operation, path.TreeScope.TreeID, path.TreeScope.TreeVersionID, rootNodeID, string(nodes), string(edges)}, "|")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

func hasContestedGraphStatus(edges []GraphEdge) bool {
	for _, edge := range edges {
		if edge.Status == "disputed" || edge.Status == "contested" || edge.Status == "contradicted" {
			return true
		}
	}
	return false
}

func hasUnresolvedGraphStatus(edges []GraphEdge) bool {
	for _, edge := range edges {
		if edge.Status == "unresolved" {
			return true
		}
	}
	return false
}

func summarizeGraphBranchStructureSide(result graphBranchSideResult) GraphBranchStructureSide {
	depthCounts := make(map[int]int)
	childCounts := make(map[string]int, len(result.Path.Nodes))
	for _, node := range result.Path.Nodes {
		depthCounts[nodePositionDepth(result.Path, node.ID)]++
		childCounts[node.ID] = 0
	}
	for _, edge := range result.Path.Edges {
		childCounts[edge.FromNodeID]++
	}
	statusCounts := make(map[string]int)
	for _, edge := range result.Path.Edges {
		statusCounts[edge.Status]++
	}
	nodesByDepth := make([]GraphBranchStructureDepthCount, 0, len(depthCounts))
	depths := make([]int, 0, len(depthCounts))
	for depth := range depthCounts {
		depths = append(depths, depth)
	}
	sort.Ints(depths)
	for _, depth := range depths {
		nodesByDepth = append(nodesByDepth, GraphBranchStructureDepthCount{Depth: depth, Count: depthCounts[depth]})
	}
	childHistogram := make(map[int]int)
	for _, childCount := range childCounts {
		childHistogram[childCount]++
	}
	childValues := make([]int, 0, len(childHistogram))
	for childCount := range childHistogram {
		childValues = append(childValues, childCount)
	}
	sort.Ints(childValues)
	childHistogramOut := make([]GraphBranchStructureChildCount, 0, len(childValues))
	for _, childCount := range childValues {
		childHistogramOut = append(childHistogramOut, GraphBranchStructureChildCount{ChildCount: childCount, NodeCount: childHistogram[childCount]})
	}
	statusKeys := make([]string, 0, len(statusCounts))
	for status := range statusCounts {
		statusKeys = append(statusKeys, status)
	}
	sort.Strings(statusKeys)
	statusOut := make([]GraphBranchStructureStatusCount, 0, len(statusKeys))
	for _, status := range statusKeys {
		statusOut = append(statusOut, GraphBranchStructureStatusCount{Status: status, Count: statusCounts[status]})
	}
	reasons := make([]string, 0, 3)
	if result.DepthTruncated {
		reasons = append(reasons, "depth_limit")
	}
	if result.NodesTruncated || result.TraversalSaturated {
		reasons = append(reasons, "node_limit")
	}
	if result.EdgesTruncated {
		reasons = append(reasons, "edge_limit")
	}
	leafCount := 0
	for _, node := range result.Path.Nodes {
		if childCounts[node.ID] == 0 {
			leafCount++
		}
	}
	return GraphBranchStructureSide{PathID: result.Path.ID, TreeScope: result.Path.TreeScope, RootNodeID: result.RootNodeID, NodeCount: len(result.Path.Nodes), EdgeCount: len(result.Path.Edges), MaxDepth: result.Path.Depth, LeafCount: leafCount, NodesByDepth: nodesByDepth, ChildCountHistogram: childHistogramOut, EdgeStatusCounts: statusOut, Truncated: result.Path.Truncated, TruncationReasons: reasons, CycleDetected: result.CycleDetected}
}

func nodePositionDepth(path GraphPath, nodeID string) int {
	for _, node := range path.Nodes {
		if node.ID == nodeID {
			return node.Depth
		}
	}
	return 0
}

func graphBranchStructureDelta(from, to GraphBranchStructureSide) GraphBranchStructureDelta {
	return GraphBranchStructureDelta{NodeCount: to.NodeCount - from.NodeCount, EdgeCount: to.EdgeCount - from.EdgeCount, MaxDepth: to.MaxDepth - from.MaxDepth, LeafCount: to.LeafCount - from.LeafCount}
}

func graphBranchComparisonStatus(from, to GraphBranchStructureSide) string {
	if from.Status() == "contested" || to.Status() == "contested" {
		return "contested"
	}
	if from.Truncated || to.Truncated || from.CycleDetected || to.CycleDetected {
		return "partial"
	}
	if len(from.EdgeStatusCounts) > 0 || len(to.EdgeStatusCounts) > 0 {
		for _, side := range []GraphBranchStructureSide{from, to} {
			for _, status := range side.EdgeStatusCounts {
				if status.Status == "unresolved" {
					return "partial"
				}
			}
		}
	}
	return "structural"
}

func (s GraphBranchStructureSide) Status() string {
	if hasContestedStatusCount(s.EdgeStatusCounts) {
		return "contested"
	}
	if hasUnresolvedStatusCount(s.EdgeStatusCounts) || s.Truncated || s.CycleDetected {
		return "partial"
	}
	return "structural"
}

func hasContestedStatusCount(counts []GraphBranchStructureStatusCount) bool {
	for _, item := range counts {
		if item.Status == "disputed" || item.Status == "contested" || item.Status == "contradicted" {
			return true
		}
	}
	return false
}

func hasUnresolvedStatusCount(counts []GraphBranchStructureStatusCount) bool {
	for _, item := range counts {
		if item.Status == "unresolved" {
			return true
		}
	}
	return false
}

func graphBranchComparisonReasons(from, to GraphBranchStructureSide) []string {
	seen := make(map[string]struct{})
	for _, reason := range append(append([]string{}, from.TruncationReasons...), to.TruncationReasons...) {
		seen[reason] = struct{}{}
	}
	reasons := make([]string, 0, len(seen))
	for reason := range seen {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return reasons
}

func graphBranchComparisonFingerprint(input GraphBranchStructureComparisonInput) string {
	value := strings.Join([]string{input.FromTreeID, input.FromTreeVersionID, input.FromRootNodeID, input.ToTreeID, input.ToTreeVersionID, input.ToRootNodeID, fmt.Sprintf("%d", input.MaxDepth), GraphBranchComparisonAlgorithm}, "|")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) persistGraphBranchStructureComparisonRun(ctx context.Context, runID string, fromResult, toResult graphBranchSideResult, result GraphBranchStructureComparisonResult) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	stats := GraphStats{Operation: GraphOperationBranchComparison, PathCount: 2, NodeCount: len(fromResult.Path.Nodes) + len(toResult.Path.Nodes), EdgeCount: len(fromResult.Path.Edges) + len(toResult.Path.Edges), Truncated: result.Truncated.From || result.Truncated.To, PathsTruncated: result.Truncated.From || result.Truncated.To, MaxDepth: result.Limits.MaxDepth, AlgorithmVersion: GraphBranchComparisonAlgorithm}
	if err := persistGraphRun(ctx, tx, runID, QueryResult{GraphPaths: []GraphPath{fromResult.Path, toResult.Path}, GraphStats: stats}); err != nil {
		return err
	}
	fromPathID, err := lookupGraphPathID(ctx, tx, runID, fromResult.Path.ID)
	if err != nil {
		return err
	}
	toPathID, err := lookupGraphPathID(ctx, tx, runID, toResult.Path.ID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO research_graph_comparisons (run_id, from_path_id, to_path_id, from_tree_id, from_tree_version_id, from_root_node_id, to_tree_id, to_tree_version_id, to_root_node_id, algorithm_version, input_fingerprint, limits, from_summary, to_summary, delta, status, truncated, truncation_reasons)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, runID, fromPathID, toPathID, result.From.TreeScope.TreeID, result.From.TreeScope.TreeVersionID, result.From.RootNodeID, result.To.TreeScope.TreeID, result.To.TreeScope.TreeVersionID, result.To.RootNodeID, result.AlgorithmVersion, result.InputFingerprint, mustJSON(result.Limits), mustJSON(result.From), mustJSON(result.To), mustJSON(result.Delta), result.Status, result.Truncated.From || result.Truncated.To, mustJSON(result.TruncationReasons)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE research_runs SET status = 'succeeded', graph_truncated = $1, updated_at = now() WHERE id = $2`, result.Truncated.From || result.Truncated.To, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lookupGraphPathID(ctx context.Context, tx pgx.Tx, runID, pathKey string) (uuid.UUID, error) {
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM research_graph_paths WHERE run_id = $1 AND path_key = $2`, runID, pathKey).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func runGraphComparison(ctx context.Context, executor historyExecutor, runID uuid.UUID) (*GraphBranchStructureComparisonResult, error) {
	var comparisonRunID uuid.UUID
	var createdAt time.Time
	var algorithmVersion, inputFingerprint, status string
	var limits, fromSummary, toSummary, delta, reasons []byte
	var truncated bool
	var fromPathKey, toPathKey string
	err := executor.QueryRow(ctx, `SELECT c.run_id, c.created_at, c.algorithm_version, c.input_fingerprint, c.limits, c.from_summary, c.to_summary, c.delta, c.status, c.truncated, c.truncation_reasons, fp.path_key, tp.path_key FROM research_graph_comparisons c JOIN research_graph_paths fp ON fp.id = c.from_path_id JOIN research_graph_paths tp ON tp.id = c.to_path_id WHERE c.run_id = $1`, runID).Scan(&comparisonRunID, &createdAt, &algorithmVersion, &inputFingerprint, &limits, &fromSummary, &toSummary, &delta, &status, &truncated, &reasons, &fromPathKey, &toPathKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := &GraphBranchStructureComparisonResult{RunID: comparisonRunID.String(), CreatedAt: createdAt, Operation: GraphOperationBranchComparison, AlgorithmVersion: algorithmVersion, StructuralOnly: true, InputFingerprint: inputFingerprint, Truncated: GraphBranchStructureTruncation{}, Status: status, Explanation: graphExplanation(GraphOperationBranchComparison, status, true)}
	if err := json.Unmarshal(limits, &result.Limits); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(fromSummary, &result.From); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(toSummary, &result.To); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(delta, &result.Delta); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(reasons, &result.TruncationReasons); err != nil {
		return nil, err
	}
	result.From.PathID = fromPathKey
	result.To.PathID = toPathKey
	result.Truncated = GraphBranchStructureTruncation{From: result.From.Truncated, To: result.To.Truncated}
	if truncated {
		result.Truncated.From = true
		result.Truncated.To = true
	}
	return result, nil
}

const branchStructureSideQuery = `
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
  SELECT tr.id AS edge_id, tr.subject_node_id, tr.object_node_id, tr.predicate, tr.status
  FROM tree_relationships tr
  JOIN visible_version v ON v.tree_version_id = tr.tree_version_id
  WHERE tr.predicate = 'parent_of'
    AND EXISTS (SELECT 1 FROM tree_nodes subject WHERE subject.id = tr.subject_node_id AND subject.tree_version_id = tr.tree_version_id)
    AND EXISTS (SELECT 1 FROM tree_nodes object WHERE object.id = tr.object_node_id AND object.tree_version_id = tr.tree_version_id)
),
bfs (depth, frontier, visited, saturated) AS (
  SELECT 0, ARRAY[node_id]::uuid[], ARRAY[node_id]::uuid[], false
  FROM root_node
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
        SELECT DISTINCT e.object_node_id AS node_id
        FROM parent_edges e
        WHERE e.subject_node_id = ANY(b.frontier)
          AND NOT e.object_node_id = ANY(b.visited)
        ORDER BY e.object_node_id
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
side_edges AS (
  SELECT e.edge_id, e.subject_node_id, e.object_node_id, e.predicate, e.status,
         row_number() OVER (ORDER BY e.edge_id) AS edge_order, count(*) OVER () AS edge_count
  FROM parent_edges e
  JOIN bounded_nodes subject ON subject.node_id = e.subject_node_id
  JOIN bounded_nodes object ON object.node_id = e.object_node_id
),
bounded_edges AS (
  SELECT edge_id, subject_node_id, object_node_id, predicate, status, edge_order
  FROM side_edges
  WHERE edge_order <= $7
),
side_state AS (
  SELECT
    (SELECT COALESCE(max(depth), 0)::integer FROM bounded_nodes) AS side_depth,
    (SELECT count(*) > $6 FROM ranked_nodes) AS nodes_truncated,
    (SELECT count(*) > $7 FROM side_edges) AS edges_truncated,
    COALESCE((SELECT bool_or(saturated) FROM bfs), false) AS traversal_saturated,
    EXISTS (SELECT 1 FROM bfs b WHERE b.depth = $5 AND EXISTS (SELECT 1 FROM parent_edges e WHERE e.subject_node_id = ANY(b.frontier) AND NOT e.object_node_id = ANY(b.visited))) AS depth_truncated,
    EXISTS (SELECT 1 FROM bfs b JOIN parent_edges e ON e.subject_node_id = ANY(b.frontier) WHERE e.object_node_id = ANY(b.visited)) AS cycle_detected
),
side_nodes AS (
  SELECT jsonb_agg(jsonb_build_object('id', tn.id::text, 'type', 'person', 'treeNodeId', tn.id::text, 'depth', bn.depth, 'position', bn.node_order - 1) ORDER BY bn.node_order) AS nodes
  FROM bounded_nodes bn
  JOIN tree_nodes tn ON tn.id = bn.node_id
),
side_edges_json AS (
  SELECT jsonb_agg(jsonb_build_object('id', edge_id::text, 'type', 'tree_relationship', 'fromNodeId', subject_node_id::text, 'toNodeId', object_node_id::text, 'pathFromNodeId', subject_node_id::text, 'pathToNodeId', object_node_id::text, 'predicate', predicate, 'status', status, 'treeRelationshipId', edge_id::text, 'position', edge_order - 1) ORDER BY edge_order) AS edges
  FROM bounded_edges
)
SELECT v.version_number::integer, v.tree_id::text, v.tree_version_id::text, v.version_state,
       r.node_id::text, st.side_depth,
       (st.nodes_truncated OR st.edges_truncated OR st.traversal_saturated OR st.depth_truncated),
       st.nodes_truncated, st.edges_truncated, st.traversal_saturated, st.depth_truncated, st.cycle_detected,
       COALESCE(sn.nodes, '[]'::jsonb), COALESCE(se.edges, '[]'::jsonb)
FROM visible_version v
CROSS JOIN root_node r
CROSS JOIN side_state st
LEFT JOIN side_nodes sn ON TRUE
LEFT JOIN side_edges_json se ON TRUE
`
