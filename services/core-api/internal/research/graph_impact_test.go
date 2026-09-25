package research

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
)

func TestGraphRelationshipImpactIsDeterministicAndAIIndependent(t *testing.T) {
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
	input := GraphRelationshipImpactInput{TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003", RelationshipID: "b3000000-0000-0000-0000-000000000001", MaxDepth: GraphMaxDepth}
	first, err := service.GraphRelationshipImpact(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, first.RunID)
	second, err := service.GraphRelationshipImpact(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, second.RunID)
	if first.PathID != second.PathID || len(first.Nodes) != len(second.Nodes) || len(first.Edges) != len(second.Edges) || first.RootRelationshipID != input.RelationshipID {
		t.Fatalf("impact result was not deterministic: %+v / %+v", first, second)
	}
	if len(first.Nodes) != 3 || len(first.Edges) != 2 || first.BoundedDownstreamNodeCount != 2 || first.Status != "contested" || !first.StructuralOnly || first.AlgorithmVersion != GraphRelationshipImpactAlgorithm {
		t.Fatalf("unexpected impact result: %+v", first)
	}
	if first.Edges[0].ID != input.RelationshipID && first.Edges[1].ID != input.RelationshipID {
		t.Fatalf("selected relationship was not retained: %+v", first.Edges)
	}
	shallow, err := service.GraphRelationshipImpact(context.Background(), GraphRelationshipImpactInput{TreeID: input.TreeID, TreeVersionID: input.TreeVersionID, RelationshipID: input.RelationshipID, MaxDepth: 1}, "")
	if err != nil || !shallow.Truncated || len(shallow.Nodes) != 2 || len(shallow.Edges) != 1 || len(shallow.TruncationReasons) == 0 {
		t.Fatalf("relationship impact depth cap was not reported: %+v", shallow)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, shallow.RunID)
	var provenance []byte
	if err := pool.QueryRow(context.Background(), `SELECT node_provenance FROM research_graph_paths WHERE run_id = $1`, first.RunID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	var values []map[string]any
	if err := json.Unmarshal(provenance, &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 || values[0]["label"] != nil || values[0]["id"] == "" {
		t.Fatalf("node provenance was not ID-only: %+v", values)
	}
}

func TestGraphRelationshipImpactRejectsInvalidInput(t *testing.T) {
	if _, err := normalizeGraphRelationshipImpactInput(GraphRelationshipImpactInput{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected missing scope validation error, got %v", err)
	}
	input, err := normalizeGraphRelationshipImpactInput(GraphRelationshipImpactInput{TreeID: " b0000000-0000-0000-0000-000000000001 ", TreeVersionID: " b1000000-0000-0000-0000-000000000003 ", RelationshipID: " b3000000-0000-0000-0000-000000000001 ", MaxDepth: 99})
	if err != nil || input.MaxDepth != GraphMaxDepth || input.TreeID == "" || input.TreeVersionID == "" || input.RelationshipID == "" {
		t.Fatalf("unexpected normalized impact input: %+v, %v", input, err)
	}
	if _, err := normalizeGraphRelationshipImpactInput(GraphRelationshipImpactInput{TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003", RelationshipID: "b3000000-0000-0000-0000-000000000001", MaxDepth: -1}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected negative depth validation error, got %v", err)
	}
}

func TestGraphRelationshipImpactRejectsWrongRelationshipScope(t *testing.T) {
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
	_, err = service.GraphRelationshipImpact(context.Background(), GraphRelationshipImpactInput{TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000001", RelationshipID: "b3000000-0000-0000-0000-000000000001"}, "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected mismatched relationship scope to be hidden, got %v", err)
	}
}
