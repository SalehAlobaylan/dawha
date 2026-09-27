package research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestDetectGraphSourceDependencyCommunitiesIsDeterministicAndDeduplicatesEdges(t *testing.T) {
	path := GraphPath{Nodes: []GraphNode{{ID: "a", Type: "source"}, {ID: "b", Type: "source"}, {ID: "c", Type: "source"}, {ID: "d", Type: "source"}}, Edges: []GraphEdge{{ID: "edge-1", FromNodeID: "a", ToNodeID: "b", Status: "confirmed"}, {ID: "edge-2", FromNodeID: "b", ToNodeID: "a", Status: "needs_review"}, {ID: "edge-3", FromNodeID: "b", ToNodeID: "c", Status: "confirmed"}, {ID: "edge-4", FromNodeID: "d", ToNodeID: "d", Status: "confirmed"}}}
	first, err := detectGraphSourceDependencyCommunities(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := detectGraphSourceDependencyCommunities(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.PartitionFingerprint != second.PartitionFingerprint || !reflect.DeepEqual(first.Communities, second.Communities) {
		t.Fatalf("community partition was not deterministic: first=%+v second=%+v", first, second)
	}
	if first.PartitionCount != 2 || len(first.Communities) != 1 || first.SubthresholdCommunityCount != 1 {
		t.Fatalf("unexpected community partition: %+v", first)
	}
	duplicateReduced := path
	duplicateReduced.Edges = []GraphEdge{path.Edges[0], path.Edges[2], path.Edges[3]}
	deduplicated, err := detectGraphSourceDependencyCommunities(duplicateReduced, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.PartitionFingerprint != deduplicated.PartitionFingerprint {
		t.Fatalf("parallel dependency records changed the structural partition: first=%s reduced=%s", first.PartitionFingerprint, deduplicated.PartitionFingerprint)
	}
}

func TestDetectGraphSourceDependencyCommunitiesKeepsDenseClustersSeparate(t *testing.T) {
	clusters := [][]string{{"a", "b", "c", "d"}, {"e", "f", "g", "h"}}
	nodes := make([]GraphNode, 0, 8)
	edges := make([]GraphEdge, 0, 26)
	position := 0
	for _, cluster := range clusters {
		for _, member := range cluster {
			nodes = append(nodes, GraphNode{ID: member, Type: "source", Position: position})
			position++
		}
	}
	edgeID := 0
	addPair := func(left, right string) {
		edges = append(edges, GraphEdge{ID: "edge-" + string(rune('a'+edgeID)), FromNodeID: left, ToNodeID: right, Status: "confirmed"})
		edgeID++
		edges = append(edges, GraphEdge{ID: "edge-" + string(rune('a'+edgeID)), FromNodeID: right, ToNodeID: left, Status: "confirmed"})
		edgeID++
	}
	for _, cluster := range clusters {
		for left := 0; left < len(cluster); left++ {
			for right := left + 1; right < len(cluster); right++ {
				addPair(cluster[left], cluster[right])
			}
		}
	}
	addPair("d", "e")
	analysis, err := detectGraphSourceDependencyCommunities(GraphPath{Nodes: nodes, Edges: edges}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Communities) != 2 || analysis.PartitionCount != 2 {
		t.Fatalf("dense clusters were not preserved: %+v", analysis.Communities)
	}
	for _, community := range analysis.Communities {
		if community.Size != 4 {
			t.Fatalf("unexpected dense cluster size: %+v", community)
		}
	}
}

// TestDetectGraphSourceDependencyCommunitiesPinsTheAnswerOnATiedGraph is the
// regression test for the partition itself, as opposed to its determinism.
//
// The greedy loop is free to represent a community however it likes internally,
// and the partition it returns is not free at all. A star is the worst case for
// that freedom: every candidate merge has the same gain, so which pair wins is
// decided entirely by the tie-break, and a tie-break that moved would silently
// repartition a perfectly symmetric graph. The expected values below are what
// this loop returned before its working set changed, asserted field by field, so
// making the loop cheaper has to be a deliberate edit to this test rather than
// something that happens to come out the other side.
//
// The graph is asymmetric on purpose: one extra parallel statement between leaf-3
// and leaf-1, which is enough to make {leaf-1, leaf-3} its own community. The
// partition is therefore not the obvious one, and a loop that drifted towards an
// obvious answer would be caught rather than agreed with.
func TestDetectGraphSourceDependencyCommunitiesPinsTheAnswerOnATiedGraph(t *testing.T) {
	nodes := []GraphNode{{ID: "root", Type: "source"}}
	edges := []GraphEdge{}
	for index := 1; index <= 6; index++ {
		leaf := fmt.Sprintf("leaf-%d", index)
		nodes = append(nodes, GraphNode{ID: leaf, Type: "source", Depth: 1})
		edges = append(edges, GraphEdge{ID: fmt.Sprintf("edge-%d", index), Type: "source_dependency", FromNodeID: "root", ToNodeID: leaf, Predicate: "cites", Status: "confirmed"})
	}
	edges = append(edges, GraphEdge{ID: "edge-extra", Type: "source_dependency", FromNodeID: "leaf-3", ToNodeID: "leaf-1", Predicate: "likely_paraphrase", Status: "needs_review"})
	path := GraphPath{Nodes: nodes, Edges: edges}

	analysis, err := detectGraphSourceDependencyCommunities(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.PartitionFingerprint != "1dbc8c268c7700fee99203e8f52cbce03c6f391747664d022f672e0ad02aedf2" {
		t.Fatalf("the tied star now partitions as %s, want 1dbc8c268c7700fee99203e8f52cbce03c6f391747664d022f672e0ad02aedf2 (communities %s)",
			analysis.PartitionFingerprint, communitySourceIDs(analysis))
	}
	if analysis.PartitionCount != 2 || analysis.SubthresholdCommunityCount != 0 || analysis.LargestCommunitySize != 5 {
		t.Fatalf("unexpected partition shape: %+v", analysis)
	}
	want := []GraphSourceDependencyCommunity{
		{CommunityID: "91940250-323a-54e3-a231-2c711475000f", SourceIDs: []string{"leaf-1", "leaf-3"}, Size: 2, InternalEdgeCount: 1, ExternalEdgeCount: 2, EdgeStatusCounts: []GraphSourceDependencyStatusCount{{Status: "needs_review", Count: 1}}},
		{CommunityID: "3721ae5a-8e2c-5d6d-be27-56b7228df8f6", SourceIDs: []string{"leaf-2", "leaf-4", "leaf-5", "leaf-6", "root"}, Size: 5, InternalEdgeCount: 4, ExternalEdgeCount: 2, EdgeStatusCounts: []GraphSourceDependencyStatusCount{{Status: "confirmed", Count: 4}}},
	}
	if !reflect.DeepEqual(analysis.Communities, want) {
		t.Fatalf("the tied star now reports different communities.\n got: %+v\nwant: %+v", analysis.Communities, want)
	}

	// The partition is a property of the graph, not of the order the rows
	// arrived in. Reversing both lists must not move it, and this is the
	// property that lets the fingerprint be compared across two databases.
	reversedNodes := make([]GraphNode, 0, len(nodes))
	for index := len(nodes) - 1; index >= 0; index-- {
		reversedNodes = append(reversedNodes, nodes[index])
	}
	reversedEdges := make([]GraphEdge, 0, len(edges))
	for index := len(edges) - 1; index >= 0; index-- {
		reversedEdges = append(reversedEdges, edges[index])
	}
	reversed, err := detectGraphSourceDependencyCommunities(GraphPath{Nodes: reversedNodes, Edges: reversedEdges}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(analysis, reversed) {
		t.Fatalf("reversing the node and edge order changed the partition.\n  forward:  %+v\n  reversed: %+v", analysis, reversed)
	}
}

func communitySourceIDs(analysis graphSourceDependencyCommunityAnalysis) string {
	parts := make([]string, 0, len(analysis.Communities))
	for _, community := range analysis.Communities {
		parts = append(parts, strings.Join(community.SourceIDs, "+"))
	}
	return strings.Join(parts, " | ")
}

func TestNormalizeGraphSourceDependencyCommunitiesInput(t *testing.T) {
	if _, err := normalizeGraphSourceDependencyCommunitiesInput(GraphSourceDependencyCommunitiesInput{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected empty source communities input validation error, got %v", err)
	}
	input, err := normalizeGraphSourceDependencyCommunitiesInput(GraphSourceDependencyCommunitiesInput{SourceID: " 30000000-0000-0000-0000-000000000002 ", MaxDepth: 99, MinCommunitySize: 99})
	if err != nil || input.MaxDepth != GraphMaxDepth || input.MinCommunitySize != GraphSourceCommunityMaxSize {
		t.Fatalf("unexpected normalized source communities input: %+v, %v", input, err)
	}
	if _, err := normalizeGraphSourceDependencyCommunitiesInput(GraphSourceDependencyCommunitiesInput{SourceID: "30000000-0000-0000-0000-000000000002", MinCommunitySize: -1}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected negative community size validation error, got %v", err)
	}
}

func TestGraphSourceDependencyCommunitiesRoundTripAndVisibility(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	rootSourceID := uuid.MustParse("30000000-0000-0000-0000-000000000002")
	firstSourceID := uuid.MustParse("30000000-0000-0000-0000-000000000001")
	secondSourceID := uuid.New()
	thirdSourceID := uuid.New()
	privateTargetID := uuid.New()
	privateRootID := uuid.New()
	tempSources := []uuid.UUID{secondSourceID, thirdSourceID, privateTargetID, privateRootID}
	dependencyIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sources (id, title_ar, source_type, visibility)
		VALUES ($1, 'مصدر مجتمع أ', 'article', 'public'),
		       ($2, 'مصدر مجتمع ب', 'article', 'public'),
		       ($3, 'هدف خاص للمجتمعات', 'article', 'private'),
		       ($4, 'جذر خاص للمجتمعات', 'article', 'private')
	`, secondSourceID, thirdSourceID, privateTargetID, privateRootID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE id IN (SELECT run_id FROM research_graph_source_dependency_communities WHERE root_source_id = $1)`, rootSourceID)
		_, _ = pool.Exec(ctx, `DELETE FROM source_dependencies WHERE id = ANY($1::uuid[])`, dependencyIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, tempSources)
	}()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_dependencies (id, source_id, depends_on_source_id, dependency_type, status)
		VALUES ($1, $5, $6, 'derived_from', 'confirmed'),
		       ($2, $6, $7, 'cites', 'needs_review'),
		       ($3, $7, $6, 'shared_origin', 'confirmed'),
		       ($4, $5, $8, 'cites', 'confirmed')
	`, dependencyIDs[0], dependencyIDs[1], dependencyIDs[2], dependencyIDs[3], firstSourceID, secondSourceID, thirdSourceID, privateTargetID); err != nil {
		t.Fatal(err)
	}

	service := &Service{Pool: pool}
	input := GraphSourceDependencyCommunitiesInput{SourceID: rootSourceID.String(), MaxDepth: GraphMaxDepth, MinCommunitySize: 2}
	first, err := service.GraphSourceDependencyCommunities(ctx, input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_runs WHERE id = $1`, first.RunID)
	second, err := service.GraphSourceDependencyCommunities(ctx, input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_runs WHERE id = $1`, second.RunID)
	if first.Summary.PartitionFingerprint != second.Summary.PartitionFingerprint || first.Summary.PathID != second.Summary.PathID || first.Summary.InputFingerprint != second.Summary.InputFingerprint {
		t.Fatalf("community snapshot was not deterministic: first=%+v second=%+v", first.Summary, second.Summary)
	}
	if first.Path.Operation != GraphOperationSourceCommunities || first.AlgorithmVersion != GraphSourceCommunitiesAlgorithm || !first.StructuralOnly || first.Summary.ReportedCommunityCount < 1 || !first.Summary.CycleDetected || first.Summary.Status != "partial" {
		t.Fatalf("unexpected community result: %+v", first)
	}
	for _, node := range first.Path.Nodes {
		if node.ID == privateTargetID.String() {
			t.Fatalf("private source appeared in community path: %+v", node)
		}
		if node.Label != "" || node.PersonID != "" || node.TreeNodeID != "" {
			t.Fatalf("community path exposed identity metadata: %+v", node)
		}
	}
	for _, edge := range first.Path.Edges {
		if edge.FromNodeID == privateTargetID.String() || edge.ToNodeID == privateTargetID.String() {
			t.Fatalf("private source appeared in community edge: %+v", edge)
		}
		if edge.SourceID != "" || edge.ClaimID != "" || edge.StatementID != "" || edge.PassageID != "" {
			t.Fatalf("community path exposed evidence provenance: %+v", edge)
		}
	}
	for _, community := range first.Summary.Communities {
		for _, sourceID := range community.SourceIDs {
			if sourceID == privateTargetID.String() {
				t.Fatalf("private source appeared in community members: %+v", community)
			}
		}
	}
	if len(first.Path.EvidenceRefs) != 0 {
		t.Fatalf("community path unexpectedly became evidence-backed: %+v", first.Path.EvidenceRefs)
	}
	if _, err := service.GraphSourceDependencyCommunities(ctx, GraphSourceDependencyCommunitiesInput{SourceID: privateRootID.String()}, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private community root was not hidden: %v", err)
	}

	researcherID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	roleExisted := false
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role = 'researcher')`, researcherID).Scan(&roleExisted); err != nil {
		t.Fatal(err)
	}
	if !roleExisted {
		if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, researcherID); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role = 'researcher'`, researcherID)
	}
	detail, err := service.GetRun(ctx, researcherID.String(), first.RunID)
	if err != nil || detail.SourceDependencyCommunities == nil || detail.SourceDependencyCommunities.Limits.MinCommunitySize != input.MinCommunitySize || len(detail.GraphPaths) != 1 || detail.GraphPaths[0].Operation != GraphOperationSourceCommunities {
		t.Fatalf("community result did not round-trip through history: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sources SET visibility = 'private' WHERE id = $1`, secondSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRun(ctx, researcherID.String(), first.RunID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("private community history was not filtered: %v", err)
	}
}
