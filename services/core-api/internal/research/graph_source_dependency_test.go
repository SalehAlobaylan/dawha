package research

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestGraphSourceDependencyNeighborhoodIsBoundedAndPrivate(t *testing.T) {
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
	sourceIDs := []uuid.UUID{secondSourceID, thirdSourceID, privateTargetID, privateRootID}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sources (id, title_ar, source_type, visibility)
		VALUES ($1, 'مصدر اعتماد تجريبي أ', 'article', 'public'),
		       ($2, 'مصدر اعتماد تجريبي ب', 'article', 'public'),
		       ($3, 'هدف خاص مخفي', 'article', 'private'),
		       ($4, 'جذر خاص مخفي', 'article', 'private')
	`, secondSourceID, thirdSourceID, privateTargetID, privateRootID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM source_dependencies WHERE source_id = ANY($1) OR depends_on_source_id = ANY($1)`, sourceIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1)`, sourceIDs)
	}()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status)
		VALUES ($1, $2, 'derived_from', 'confirmed'),
		       ($2, $3, 'cites', 'needs_review'),
		       ($3, $4, 'cites', 'confirmed'),
		       ($4, $5, 'cites', 'confirmed')
	`, firstSourceID, secondSourceID, thirdSourceID, rootSourceID, privateTargetID); err != nil {
		t.Fatal(err)
	}

	service := &Service{Pool: pool}
	first, err := service.GraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: rootSourceID.String(), MaxDepth: GraphMaxDepth}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_runs WHERE id = $1`, first.RunID)
	second, err := service.GraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: rootSourceID.String(), MaxDepth: GraphMaxDepth}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_runs WHERE id = $1`, second.RunID)

	if first.Summary.PathID != second.Summary.PathID || first.Summary.InputFingerprint != second.Summary.InputFingerprint || first.Summary.EdgeSetFingerprint != second.Summary.EdgeSetFingerprint {
		t.Fatalf("source dependency snapshot was not deterministic: first=%+v second=%+v", first.Summary, second.Summary)
	}
	if len(first.Path.Nodes) != 4 || first.Summary.BoundedUpstreamSourceCount != 3 || len(first.Path.Edges) != 4 || first.Summary.MaxDepthReached != GraphMaxDepth {
		t.Fatalf("unexpected source dependency neighborhood: %+v", first)
	}
	if !first.Summary.CycleDetected || first.Summary.Status != "partial" || !first.StructuralOnly || first.AlgorithmVersion != GraphSourceDependencyAlgorithm {
		t.Fatalf("cycle or structural status was not reported: %+v", first)
	}
	for _, node := range first.Path.Nodes {
		if node.Label != "" || node.PersonID != "" || node.TreeNodeID != "" {
			t.Fatalf("source neighborhood exposed identity metadata: %+v", node)
		}
	}
	for _, edge := range first.Path.Edges {
		if edge.SourceID != "" || edge.ClaimID != "" || edge.StatementID != "" || edge.PassageID != "" {
			t.Fatalf("source neighborhood exposed evidence provenance: %+v", edge)
		}
	}
	if len(first.Path.EvidenceRefs) != 0 {
		t.Fatalf("source neighborhood unexpectedly became evidence-backed: %+v", first.Path.EvidenceRefs)
	}
	if first.Path.Edges[0].FromNodeID == first.Path.Edges[0].ToNodeID || first.Path.Edges[0].PathFromNodeID != first.Path.Edges[0].FromNodeID || first.Path.Edges[0].PathToNodeID != first.Path.Edges[0].ToNodeID {
		t.Fatalf("source dependency direction was not preserved: %+v", first.Path.Edges[0])
	}

	shallow, err := service.GraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: rootSourceID.String(), MaxDepth: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_runs WHERE id = $1`, shallow.RunID)
	if !shallow.Summary.Truncated || shallow.Summary.BoundedUpstreamSourceCount != 1 || len(shallow.Path.Edges) != 1 {
		t.Fatalf("source dependency depth bound was not reported: %+v", shallow)
	}
	if len(shallow.Summary.TruncationReasons) == 0 || shallow.Summary.TruncationReasons[0] != "depth_limit" {
		t.Fatalf("source dependency truncation reason was not persisted: %+v", shallow.Summary)
	}

	if _, err := service.GraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: privateRootID.String()}, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private source root was not rejected: %v", err)
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
	if err != nil || detail.SourceDependencyNeighborhood == nil || len(detail.GraphPaths) != 1 {
		t.Fatalf("source dependency neighborhood did not round-trip through history: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sources SET visibility = 'private' WHERE id = $1`, secondSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRun(ctx, researcherID.String(), first.RunID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("private source graph history was not filtered: %v", err)
	}
}

func TestGraphSourceDependencyCycleDetectionIsDirectional(t *testing.T) {
	path := GraphPath{Nodes: []GraphNode{{ID: "root", Type: "source"}, {ID: "a", Type: "source"}, {ID: "b", Type: "source"}}, Edges: []GraphEdge{{FromNodeID: "root", ToNodeID: "a"}, {FromNodeID: "root", ToNodeID: "b"}, {FromNodeID: "a", ToNodeID: "b"}}}
	if graphSourceDependencyHasCycle(path) {
		t.Fatal("a directed acyclic graph was reported as cyclic")
	}
	path.Edges = append(path.Edges, GraphEdge{FromNodeID: "b", ToNodeID: "root"})
	if !graphSourceDependencyHasCycle(path) {
		t.Fatal("a directed cycle was not detected")
	}
}

func TestGraphSourceDependencyNeighborhoodCapsNodeWork(t *testing.T) {
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
	rootSourceID := uuid.New()
	sourceIDs := make([]uuid.UUID, GraphMaxNodes+5)
	for index := range sourceIDs {
		sourceIDs[index] = uuid.New()
	}
	allSourceIDs := append([]uuid.UUID{rootSourceID}, sourceIDs...)
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type) SELECT id, 'مصدر حدّ الاعتماد', 'article' FROM unnest($1::uuid[]) AS source(id)`, allSourceIDs); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM source_dependencies WHERE source_id = $1`, rootSourceID)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, allSourceIDs)
	}()
	if _, err := pool.Exec(ctx, `INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status) SELECT $1, id, 'cites', 'confirmed' FROM unnest($2::uuid[]) AS source(id)`, rootSourceID, sourceIDs); err != nil {
		t.Fatal(err)
	}
	result, err := (&Service{Pool: pool}).GraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: rootSourceID.String(), MaxDepth: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_runs WHERE id = $1`, result.RunID)
	if len(result.Path.Nodes) != GraphMaxNodes || len(result.Path.Edges) != GraphMaxNodes-1 || !result.Summary.Truncated || len(result.Summary.TruncationReasons) == 0 {
		t.Fatalf("source dependency node bound was not enforced: %+v", result.Summary)
	}
}

func TestGraphSourceDependencyNeighborhoodCapsEdgeWork(t *testing.T) {
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
	rootSourceID := uuid.New()
	targetIDs := make([]uuid.UUID, 50)
	for index := range targetIDs {
		targetIDs[index] = uuid.New()
	}
	allSourceIDs := append([]uuid.UUID{rootSourceID}, targetIDs...)
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type) SELECT id, 'هدف حدّ الحواف', 'article' FROM unnest($1::uuid[]) AS source(id)`, allSourceIDs); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM source_dependencies WHERE source_id = $1`, rootSourceID)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, allSourceIDs)
	}()
	if _, err := pool.Exec(ctx, `INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status) SELECT $1, target.id, dependency_type, 'confirmed' FROM unnest($2::uuid[]) AS target(id) CROSS JOIN (VALUES ('cites'), ('derived_from'), ('likely_paraphrase'), ('shared_origin'), ('unknown')) AS types(dependency_type)`, rootSourceID, targetIDs); err != nil {
		t.Fatal(err)
	}
	result, err := (&Service{Pool: pool}).GraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: rootSourceID.String(), MaxDepth: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_runs WHERE id = $1`, result.RunID)
	edgeLimitReported := false
	for _, reason := range result.Summary.TruncationReasons {
		if reason == "edge_limit" {
			edgeLimitReported = true
		}
	}
	if len(result.Path.Edges) != GraphMaxEdges || !result.Summary.Truncated || !edgeLimitReported {
		t.Fatalf("source dependency edge bound was not enforced: %+v", result.Summary)
	}
}

func TestNormalizeGraphSourceDependencyNeighborhoodInput(t *testing.T) {
	if _, err := normalizeGraphSourceDependencyNeighborhoodInput(GraphSourceDependencyNeighborhoodInput{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected empty source dependency input validation error, got %v", err)
	}
	input, err := normalizeGraphSourceDependencyNeighborhoodInput(GraphSourceDependencyNeighborhoodInput{SourceID: " 30000000-0000-0000-0000-000000000002 ", MaxDepth: 99})
	if err != nil || input.MaxDepth != GraphMaxDepth {
		t.Fatalf("unexpected normalized source dependency input: %+v, %v", input, err)
	}
	if _, err := normalizeGraphSourceDependencyNeighborhoodInput(GraphSourceDependencyNeighborhoodInput{SourceID: "30000000-0000-0000-0000-000000000002", MaxDepth: -1}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected negative source dependency depth validation error, got %v", err)
	}
}
