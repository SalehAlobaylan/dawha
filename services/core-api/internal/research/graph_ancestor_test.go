package research

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
)

func TestGraphAncestorFrontierIsDeterministicAndAIIndependent(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	service := &Service{Pool: pool}
	input := GraphAncestorFrontierInput{TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003", RootNodeID: "b2000000-0000-0000-0000-000000000003", MaxDepth: GraphMaxDepth}
	first, err := service.GraphAncestorFrontier(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, first.RunID)
	second, err := service.GraphAncestorFrontier(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, second.RunID)
	if first.Summary.PathID != second.Summary.PathID || first.Summary.InputFingerprint != second.Summary.InputFingerprint || first.Summary.BoundedAncestorCount != 2 || first.Summary.BoundedEdgeCount != 2 || first.Summary.MaxDepthReached != 2 || first.Path.Status != "contested" || !first.StructuralOnly {
		t.Fatalf("unexpected ancestor frontier: %+v", first)
	}
	if first.Path.Edges[0].FromNodeID == first.Path.Edges[0].PathFromNodeID || first.Path.Edges[0].PathToNodeID == first.Path.Edges[0].ToNodeID {
		t.Fatalf("ancestor traversal direction was not preserved: %+v", first.Path.Edges)
	}
	for _, node := range first.Path.Nodes {
		if node.Label != "" || node.PersonID != "" {
			t.Fatalf("ancestor path exposed identity data: %+v", node)
		}
	}
	for _, edge := range first.Path.Edges {
		if edge.SourceID != "" {
			t.Fatalf("ancestor path exposed source provenance: %+v", edge)
		}
	}
	shallowInput := input
	shallowInput.MaxDepth = 1
	shallow, err := service.GraphAncestorFrontier(context.Background(), shallowInput, "")
	if err != nil || !shallow.Summary.Truncated || len(shallow.Summary.TruncationReasons) == 0 || shallow.Summary.BoundedAncestorCount != 1 {
		t.Fatalf("ancestor depth truncation was not reported: %+v", shallow)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, shallow.RunID)
	roleExisted := false
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = '00000000-0000-0000-0000-000000000001' AND role = 'researcher')`).Scan(&roleExisted); err != nil {
		t.Fatal(err)
	}
	if !roleExisted {
		if _, err := pool.Exec(context.Background(), `INSERT INTO user_roles (user_id, role) VALUES ('00000000-0000-0000-0000-000000000001', 'researcher')`); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(context.Background(), `DELETE FROM user_roles WHERE user_id = '00000000-0000-0000-0000-000000000001' AND role = 'researcher'`)
	}
	detail, err := service.GetRun(context.Background(), "00000000-0000-0000-0000-000000000001", first.RunID)
	if err != nil || detail.AncestorFrontier == nil || len(detail.GraphPaths) != 1 {
		t.Fatalf("ancestor frontier did not round-trip through history: %v", err)
	}
}

func TestGraphAncestorFrontierRootWithoutParent(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	service := &Service{Pool: pool}
	result, err := service.GraphAncestorFrontier(context.Background(), GraphAncestorFrontierInput{TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003", RootNodeID: "b2000000-0000-0000-0000-000000000001"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, result.RunID)
	if result.Summary.BoundedAncestorCount != 0 || result.Summary.BoundedEdgeCount != 0 || len(result.Path.Edges) != 0 || result.Summary.Status != "structural" || result.Path.Explanation == "" {
		t.Fatalf("unexpected root-without-parent result: %+v", result)
	}
}

func TestNormalizeGraphAncestorFrontierInput(t *testing.T) {
	if _, err := normalizeGraphAncestorFrontierInput(GraphAncestorFrontierInput{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected empty ancestor input validation error, got %v", err)
	}
	input, err := normalizeGraphAncestorFrontierInput(GraphAncestorFrontierInput{TreeID: " b0000000-0000-0000-0000-000000000001 ", TreeVersionID: " b1000000-0000-0000-0000-000000000003 ", RootNodeID: " b2000000-0000-0000-0000-000000000003 ", MaxDepth: 99})
	if err != nil || input.MaxDepth != GraphMaxDepth {
		t.Fatalf("unexpected normalized ancestor input: %+v, %v", input, err)
	}
	if _, err := normalizeGraphAncestorFrontierInput(GraphAncestorFrontierInput{TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003", RootNodeID: "b2000000-0000-0000-0000-000000000003", MaxDepth: -1}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected negative ancestor depth validation error, got %v", err)
	}
}

func TestGraphAncestorFrontierRejectsRootVersionMismatch(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	service := &Service{Pool: pool}
	_, err = service.GraphAncestorFrontier(context.Background(), GraphAncestorFrontierInput{TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000001", RootNodeID: "b2000000-0000-0000-0000-000000000003"}, "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected mismatched ancestor root/version to be hidden, got %v", err)
	}
}
