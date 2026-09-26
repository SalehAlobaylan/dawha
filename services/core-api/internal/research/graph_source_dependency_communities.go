package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GraphSourceDependencyCommunitiesInput struct {
	SourceID         string `json:"source_id"`
	MaxDepth         int    `json:"max_depth,omitempty"`
	MinCommunitySize int    `json:"min_community_size,omitempty"`
}

type GraphSourceDependencyCommunitiesLimits struct {
	MaxDepth         int `json:"maxDepth"`
	MaxNodes         int `json:"maxNodes"`
	MaxEdges         int `json:"maxEdges"`
	MinCommunitySize int `json:"minCommunitySize"`
}

type GraphSourceDependencyCommunity struct {
	CommunityID       string                             `json:"communityId"`
	SourceIDs         []string                           `json:"sourceIds"`
	Size              int                                `json:"size"`
	InternalEdgeCount int                                `json:"internalEdgeCount"`
	ExternalEdgeCount int                                `json:"externalEdgeCount"`
	EdgeStatusCounts  []GraphSourceDependencyStatusCount `json:"edgeStatusCounts"`
}

type GraphSourceDependencyCommunitiesSummary struct {
	RootSourceID               string                                 `json:"rootSourceId"`
	PathID                     string                                 `json:"pathId"`
	InputFingerprint           string                                 `json:"inputFingerprint"`
	EdgeSetFingerprint         string                                 `json:"edgeSetFingerprint"`
	PartitionFingerprint       string                                 `json:"partitionFingerprint"`
	PartitionCount             int                                    `json:"partitionCount"`
	ReportedCommunityCount     int                                    `json:"reportedCommunityCount"`
	SubthresholdCommunityCount int                                    `json:"subthresholdCommunityCount"`
	LargestCommunitySize       int                                    `json:"largestCommunitySize"`
	MaxDepthReached            int                                    `json:"maxDepthReached"`
	Limits                     GraphSourceDependencyCommunitiesLimits `json:"limits"`
	Communities                []GraphSourceDependencyCommunity       `json:"communities"`
	Truncated                  bool                                   `json:"truncated"`
	TruncationReasons          []string                               `json:"truncationReasons"`
	CycleDetected              bool                                   `json:"cycleDetected"`
	Status                     string                                 `json:"status"`
}

type GraphSourceDependencyCommunitiesResult struct {
	RunID            string                                  `json:"runId"`
	CreatedAt        time.Time                               `json:"createdAt"`
	Operation        string                                  `json:"operation"`
	AlgorithmVersion string                                  `json:"algorithmVersion"`
	StructuralOnly   bool                                    `json:"structuralOnly"`
	Limits           GraphSourceDependencyCommunitiesLimits  `json:"limits"`
	Summary          GraphSourceDependencyCommunitiesSummary `json:"summary"`
	Path             GraphPath                               `json:"path"`
}

type graphSourceDependencyCommunityAnalysis struct {
	Communities                []GraphSourceDependencyCommunity
	PartitionFingerprint       string
	PartitionCount             int
	SubthresholdCommunityCount int
	LargestCommunitySize       int
}

type graphSourceDependencyCommunityState struct {
	members map[string]struct{}
	degree  int
}

type graphSourceDependencyCommunityPair struct {
	left  string
	right string
}

type graphSourceDependencyUniqueEdge struct {
	pair   graphSourceDependencyCommunityPair
	status string
}

func (s *Service) GraphSourceDependencyCommunities(ctx context.Context, input GraphSourceDependencyCommunitiesInput, actorID string) (GraphSourceDependencyCommunitiesResult, error) {
	if err := s.ready(); err != nil {
		return GraphSourceDependencyCommunitiesResult{}, err
	}
	normalized, err := normalizeGraphSourceDependencyCommunitiesInput(input)
	if err != nil {
		return GraphSourceDependencyCommunitiesResult{}, err
	}
	row, err := s.retrieveGraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: normalized.SourceID, MaxDepth: normalized.MaxDepth}, actorID)
	if err != nil {
		return GraphSourceDependencyCommunitiesResult{}, err
	}
	analysis, err := detectGraphSourceDependencyCommunities(row.Path, normalized.MinCommunitySize)
	if err != nil {
		return GraphSourceDependencyCommunitiesResult{}, err
	}
	runID, createdAt, err := s.startRun(ctx, QueryInput{Question: "مجتمعات اعتماد المصادر", GraphOperation: GraphOperationSourceCommunities, GraphStartType: "source", GraphStartID: normalized.SourceID, SourceID: normalized.SourceID, GraphMaxDepth: normalized.MaxDepth}, actorID)
	if err != nil {
		return GraphSourceDependencyCommunitiesResult{}, err
	}
	path := row.Path
	path.Operation = GraphOperationSourceCommunities
	path.AlgorithmVersion = GraphSourceCommunitiesAlgorithm
	path.Explanation = graphExplanation(path.Operation, path.Status, path.StructuralOnly)
	path.ID = graphSourceDependencyCommunityPathID(normalized.SourceID, path, analysis.PartitionFingerprint)
	limits := GraphSourceDependencyCommunitiesLimits{MaxDepth: normalized.MaxDepth, MaxNodes: GraphMaxNodes, MaxEdges: GraphMaxEdges, MinCommunitySize: normalized.MinCommunitySize}
	summary := summarizeGraphSourceDependencyCommunities(row, path, analysis, graphSourceDependencyCommunitiesInputFingerprint(normalized))
	summary.Limits = limits
	result := GraphSourceDependencyCommunitiesResult{RunID: runID, CreatedAt: createdAt, Operation: GraphOperationSourceCommunities, AlgorithmVersion: GraphSourceCommunitiesAlgorithm, StructuralOnly: true, Limits: limits, Summary: summary, Path: path}
	if err := s.persistGraphSourceDependencyCommunitiesRun(ctx, runID, row, result); err != nil {
		_ = s.failRun(ctx, runID, err)
		return GraphSourceDependencyCommunitiesResult{}, err
	}
	return result, nil
}

func normalizeGraphSourceDependencyCommunitiesInput(input GraphSourceDependencyCommunitiesInput) (GraphSourceDependencyCommunitiesInput, error) {
	input.SourceID = strings.TrimSpace(input.SourceID)
	if input.SourceID == "" {
		return GraphSourceDependencyCommunitiesInput{}, ErrValidation
	}
	parsed, err := normalizeGraphUUID(input.SourceID)
	if err != nil {
		return GraphSourceDependencyCommunitiesInput{}, err
	}
	input.SourceID = parsed
	if input.MaxDepth < 0 {
		return GraphSourceDependencyCommunitiesInput{}, ErrValidation
	}
	if input.MaxDepth == 0 {
		input.MaxDepth = GraphDefaultDepth
	}
	if input.MaxDepth > GraphMaxDepth {
		input.MaxDepth = GraphMaxDepth
	}
	if input.MinCommunitySize < 0 {
		return GraphSourceDependencyCommunitiesInput{}, ErrValidation
	}
	if input.MinCommunitySize == 0 {
		input.MinCommunitySize = 2
	}
	if input.MinCommunitySize > GraphSourceCommunityMaxSize {
		input.MinCommunitySize = GraphSourceCommunityMaxSize
	}
	return input, nil
}

func detectGraphSourceDependencyCommunities(path GraphPath, minCommunitySize int) (graphSourceDependencyCommunityAnalysis, error) {
	if minCommunitySize < 1 {
		return graphSourceDependencyCommunityAnalysis{}, ErrValidation
	}
	nodeIDs := make([]string, 0, len(path.Nodes))
	nodeSet := make(map[string]struct{}, len(path.Nodes))
	for _, node := range path.Nodes {
		if node.ID == "" {
			return graphSourceDependencyCommunityAnalysis{}, ErrValidation
		}
		if _, exists := nodeSet[node.ID]; !exists {
			nodeSet[node.ID] = struct{}{}
			nodeIDs = append(nodeIDs, node.ID)
		}
	}
	sort.Strings(nodeIDs)
	uniqueEdges := make(map[string]graphSourceDependencyUniqueEdge)
	edgePairs := make([]graphSourceDependencyCommunityPair, 0, len(path.Edges))
	for _, edge := range path.Edges {
		if _, exists := nodeSet[edge.FromNodeID]; !exists {
			return graphSourceDependencyCommunityAnalysis{}, ErrValidation
		}
		if _, exists := nodeSet[edge.ToNodeID]; !exists {
			return graphSourceDependencyCommunityAnalysis{}, ErrValidation
		}
		if edge.FromNodeID == edge.ToNodeID {
			continue
		}
		pair := graphSourceDependencyCommunityPair{left: edge.FromNodeID, right: edge.ToNodeID}
		if pair.left > pair.right {
			pair.left, pair.right = pair.right, pair.left
		}
		key := graphSourceDependencyPairKey(pair)
		if existing, exists := uniqueEdges[key]; exists {
			if edge.Status == "needs_review" {
				existing.status = "needs_review"
				uniqueEdges[key] = existing
			}
			continue
		}
		uniqueEdges[key] = graphSourceDependencyUniqueEdge{pair: pair, status: edge.Status}
		edgePairs = append(edgePairs, pair)
	}
	states := make(map[string]*graphSourceDependencyCommunityState, len(nodeIDs))
	nodeState := make(map[string]string, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		states[nodeID] = &graphSourceDependencyCommunityState{members: map[string]struct{}{nodeID: {}}}
		nodeState[nodeID] = nodeID
	}
	for _, edge := range edgePairs {
		states[edge.left].degree++
		states[edge.right].degree++
	}
	edgeCount := len(edgePairs)
	for {
		bestGain := math.Inf(-1)
		bestPair := graphSourceDependencyCommunityPair{}
		between := make(map[string]int)
		for _, edge := range edgePairs {
			leftState := nodeState[edge.left]
			rightState := nodeState[edge.right]
			if leftState == rightState {
				continue
			}
			if leftState > rightState {
				leftState, rightState = rightState, leftState
			}
			key := leftState + "\x00" + rightState
			between[key]++
		}
		for key, count := range between {
			parts := strings.SplitN(key, "\x00", 2)
			leftState, rightState := parts[0], parts[1]
			left := states[leftState]
			right := states[rightState]
			if left == nil || right == nil {
				continue
			}
			gain := float64(count)/float64(edgeCount) - (float64(left.degree)*float64(right.degree))/(2*float64(edgeCount)*float64(edgeCount))
			pairKey := graphSourceDependencyPairKey(graphSourceDependencyCommunityPair{left: leftState, right: rightState})
			if gain > bestGain+1e-12 || (math.Abs(gain-bestGain) <= 1e-12 && (bestPair.left == "" || pairKey < graphSourceDependencyPairKey(bestPair))) {
				bestGain = gain
				bestPair = graphSourceDependencyCommunityPair{left: leftState, right: rightState}
			}
		}
		if bestPair.left == "" || bestGain <= 1e-12 {
			break
		}
		if bestPair.left > bestPair.right {
			bestPair.left, bestPair.right = bestPair.right, bestPair.left
		}
		left := states[bestPair.left]
		right := states[bestPair.right]
		for nodeID := range right.members {
			left.members[nodeID] = struct{}{}
		}
		left.degree += right.degree
		members := make([]string, 0, len(left.members))
		for member := range left.members {
			members = append(members, member)
		}
		sort.Strings(members)
		newKey := graphSourceDependencyCommunityKey(members)
		for _, member := range members {
			nodeState[member] = newKey
		}
		delete(states, bestPair.left)
		delete(states, bestPair.right)
		states[newKey] = left
	}
	memberGroups := make([][]string, 0, len(states))
	for _, state := range states {
		members := make([]string, 0, len(state.members))
		for member := range state.members {
			members = append(members, member)
		}
		sort.Strings(members)
		memberGroups = append(memberGroups, members)
	}
	sort.Slice(memberGroups, func(left, right int) bool {
		return strings.Join(memberGroups[left], ",") < strings.Join(memberGroups[right], ",")
	})
	partitionKeyParts := make([]string, 0, len(memberGroups))
	for _, members := range memberGroups {
		partitionKeyParts = append(partitionKeyParts, strings.Join(members, ","))
	}
	partitionSum := sha256.Sum256([]byte(strings.Join(partitionKeyParts, "||")))
	analysis := graphSourceDependencyCommunityAnalysis{PartitionFingerprint: hex.EncodeToString(partitionSum[:]), PartitionCount: len(memberGroups), Communities: make([]GraphSourceDependencyCommunity, 0)}
	for _, members := range memberGroups {
		if len(members) > analysis.LargestCommunitySize {
			analysis.LargestCommunitySize = len(members)
		}
		if len(members) < minCommunitySize {
			analysis.SubthresholdCommunityCount++
			continue
		}
		memberSet := make(map[string]struct{}, len(members))
		for _, member := range members {
			memberSet[member] = struct{}{}
		}
		internalEdges := 0
		externalEdges := 0
		for _, edge := range edgePairs {
			leftIn := false
			rightIn := false
			_, leftIn = memberSet[edge.left]
			_, rightIn = memberSet[edge.right]
			if leftIn && rightIn {
				internalEdges++
				continue
			}
			if leftIn {
				externalEdges++
			}
			if rightIn {
				externalEdges++
			}
		}
		statusCounts := make(map[string]int)
		for _, edge := range uniqueEdges {
			if _, leftIn := memberSet[edge.pair.left]; !leftIn {
				continue
			}
			if _, rightIn := memberSet[edge.pair.right]; !rightIn {
				continue
			}
			statusCounts[edge.status]++
		}
		statuses := make([]string, 0, len(statusCounts))
		for status := range statusCounts {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		statusOut := make([]GraphSourceDependencyStatusCount, 0, len(statuses))
		for _, status := range statuses {
			statusOut = append(statusOut, GraphSourceDependencyStatusCount{Status: status, Count: statusCounts[status]})
		}
		communityIDKey := strings.Join([]string{GraphOperationSourceCommunities, GraphSourceCommunitiesAlgorithm, strings.Join(members, ",")}, "|")
		analysis.Communities = append(analysis.Communities, GraphSourceDependencyCommunity{CommunityID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(communityIDKey)).String(), SourceIDs: append([]string(nil), members...), Size: len(members), InternalEdgeCount: internalEdges, ExternalEdgeCount: externalEdges, EdgeStatusCounts: statusOut})
	}
	sort.Slice(analysis.Communities, func(left, right int) bool {
		return strings.Join(analysis.Communities[left].SourceIDs, ",") < strings.Join(analysis.Communities[right].SourceIDs, ",")
	})
	return analysis, nil
}

func graphSourceDependencyPairKey(pair graphSourceDependencyCommunityPair) string {
	return pair.left + "\x00" + pair.right
}

func graphSourceDependencyCommunityKey(members []string) string {
	return strings.Join(members, ",")
}

func graphSourceDependencyCommunityPathID(rootSourceID string, path GraphPath, partitionFingerprint string) string {
	nodes := mustJSON(path.Nodes)
	edges := mustJSON(path.Edges)
	key := strings.Join([]string{path.Operation, rootSourceID, path.Status, fmt.Sprintf("%d", path.Depth), fmt.Sprintf("%t", path.Truncated), partitionFingerprint, string(nodes), string(edges)}, "|")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

func graphSourceDependencyCommunitiesInputFingerprint(input GraphSourceDependencyCommunitiesInput) string {
	value := strings.Join([]string{input.SourceID, fmt.Sprintf("%d", input.MaxDepth), fmt.Sprintf("%d", GraphMaxNodes), fmt.Sprintf("%d", GraphMaxEdges), fmt.Sprintf("%d", input.MinCommunitySize), GraphSourceCommunitiesAlgorithm}, "|")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func summarizeGraphSourceDependencyCommunities(row graphSourceDependencyRow, path GraphPath, analysis graphSourceDependencyCommunityAnalysis, fingerprint string) GraphSourceDependencyCommunitiesSummary {
	status := path.Status
	if status == "" {
		status = "structural"
	}
	return GraphSourceDependencyCommunitiesSummary{RootSourceID: firstGraphNodeID(path), PathID: path.ID, InputFingerprint: fingerprint, EdgeSetFingerprint: graphSourceDependencyEdgeSetFingerprint(path.Edges), PartitionFingerprint: analysis.PartitionFingerprint, PartitionCount: analysis.PartitionCount, ReportedCommunityCount: len(analysis.Communities), SubthresholdCommunityCount: analysis.SubthresholdCommunityCount, LargestCommunitySize: analysis.LargestCommunitySize, MaxDepthReached: path.Depth, Communities: analysis.Communities, Truncated: path.Truncated, TruncationReasons: graphSourceDependencyTruncationReasons(row), CycleDetected: row.CycleDetected, Status: status}
}

func (s *Service) persistGraphSourceDependencyCommunitiesRun(ctx context.Context, runID string, row graphSourceDependencyRow, result GraphSourceDependencyCommunitiesResult) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockGraphSourceDependencySources(ctx, tx, result.Summary.RootSourceID, result.Path); err != nil {
		return err
	}
	stats := GraphStats{Operation: GraphOperationSourceCommunities, PathCount: 1, NodeCount: len(result.Path.Nodes), EdgeCount: len(result.Path.Edges), Truncated: result.Path.Truncated, PathsTruncated: result.Path.Truncated, MaxDepth: result.Limits.MaxDepth, AlgorithmVersion: GraphSourceCommunitiesAlgorithm}
	if err := persistGraphRun(ctx, tx, runID, QueryResult{GraphPaths: []GraphPath{result.Path}, GraphStats: stats}); err != nil {
		return err
	}
	pathID, err := lookupGraphPathID(ctx, tx, runID, result.Path.ID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO research_graph_source_dependency_communities (run_id, path_id, root_source_id, algorithm_version, input_fingerprint, edge_set_fingerprint, partition_fingerprint, limits, summary, status, truncated, truncation_reasons) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, runID, pathID, result.Summary.RootSourceID, result.AlgorithmVersion, result.Summary.InputFingerprint, result.Summary.EdgeSetFingerprint, result.Summary.PartitionFingerprint, mustJSON(result.Limits), mustJSON(result.Summary), result.Summary.Status, result.Summary.Truncated, mustJSON(result.Summary.TruncationReasons)); err != nil {
		return err
	}
	// The run is finished, so it is counted here and not where it was created: a
	// run that never finished is a run that failed, and counting successes at
	// the start would count questions nobody answered. RETURNING gives back the
	// elapsed time as the database measured it, which is the only clock that
	// saw the whole run.
	var elapsed float64
	if err := tx.QueryRow(ctx, `UPDATE research_runs SET status = 'succeeded', graph_truncated = $1, updated_at = now() WHERE id = $2 RETURNING extract(epoch from (now() - created_at))`, result.Summary.Truncated, runID).Scan(&elapsed); err != nil {
		return err
	}
	s.recordRun(telemetry.ResearchCompleted, result.Summary.Truncated, elapsed)
	return tx.Commit(ctx)
}

func runGraphSourceDependencyCommunities(ctx context.Context, executor historyExecutor, runID uuid.UUID) (*GraphSourceDependencyCommunitiesSummary, error) {
	var inputFingerprint, edgeSetFingerprint, partitionFingerprint, status, pathKey string
	var summary, reasons []byte
	var truncated bool
	err := executor.QueryRow(ctx, `SELECT c.input_fingerprint, c.edge_set_fingerprint, c.partition_fingerprint, c.summary, c.status, c.truncated, c.truncation_reasons, p.path_key FROM research_graph_source_dependency_communities c JOIN research_graph_paths p ON p.id = c.path_id WHERE c.run_id = $1`, runID).Scan(&inputFingerprint, &edgeSetFingerprint, &partitionFingerprint, &summary, &status, &truncated, &reasons, &pathKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := &GraphSourceDependencyCommunitiesSummary{PathID: pathKey, InputFingerprint: inputFingerprint, EdgeSetFingerprint: edgeSetFingerprint, PartitionFingerprint: partitionFingerprint, Status: status, Truncated: truncated}
	if err := json.Unmarshal(summary, result); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(reasons, &result.TruncationReasons); err != nil {
		return nil, err
	}
	result.PathID = pathKey
	result.InputFingerprint = inputFingerprint
	result.EdgeSetFingerprint = edgeSetFingerprint
	result.PartitionFingerprint = partitionFingerprint
	result.Status = status
	result.Truncated = truncated
	return result, nil
}
