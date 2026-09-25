package research

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
)

func TestGraphBranchStructureComparisonIsDeterministicAndStructural(t *testing.T) {
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
	input := GraphBranchStructureComparisonInput{FromTreeID: "b0000000-0000-0000-0000-000000000001", FromTreeVersionID: "b1000000-0000-0000-0000-000000000003", FromRootNodeID: "b2000000-0000-0000-0000-000000000001", ToTreeID: "b0000000-0000-0000-0000-000000000001", ToTreeVersionID: "b1000000-0000-0000-0000-000000000001", ToRootNodeID: "b2000000-0000-0000-0000-000000000004", MaxDepth: GraphMaxDepth}
	first, err := service.GraphBranchStructureComparison(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, first.RunID)
	second, err := service.GraphBranchStructureComparison(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, second.RunID)
	if first.InputFingerprint != second.InputFingerprint || first.From.PathID != second.From.PathID || first.To.PathID != second.To.PathID || first.Delta != (GraphBranchStructureDelta{}) {
		t.Fatalf("comparison was not deterministic: %+v / %+v", first, second)
	}
	if first.From.NodeCount != 3 || first.To.NodeCount != 3 || first.From.EdgeCount != 2 || first.To.EdgeCount != 2 || first.From.LeafCount != 1 || first.To.LeafCount != 1 || first.Status != "contested" || !first.StructuralOnly {
		t.Fatalf("unexpected branch comparison: %+v", first)
	}
	if first.From.TreeScope.TreeID != input.FromTreeID || first.To.TreeScope.TreeVersionID != input.ToTreeVersionID {
		t.Fatalf("comparison scope was not retained: %+v", first)
	}
	sameScope := input
	sameScope.ToTreeID = input.FromTreeID
	sameScope.ToTreeVersionID = input.FromTreeVersionID
	sameScope.ToRootNodeID = input.FromRootNodeID
	sameResult, err := service.GraphBranchStructureComparison(context.Background(), sameScope, "")
	if err != nil || sameResult.From.PathID == sameResult.To.PathID {
		t.Fatalf("same-scope comparison did not retain separate path identities: %+v", sameResult)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, sameResult.RunID)
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
	if err != nil || detail.Comparison == nil || len(detail.GraphPaths) != 2 {
		t.Fatalf("comparison did not round-trip through history: %v", err)
	}
	for _, path := range detail.GraphPaths {
		for _, node := range path.Nodes {
			if node.Label != "" || node.PersonID != "" {
				t.Fatalf("comparison path exposed identity data: %+v", node)
			}
		}
		for _, edge := range path.Edges {
			if edge.SourceID != "" {
				t.Fatalf("comparison path exposed source provenance: %+v", edge)
			}
		}
	}
	shallowInput := input
	shallowInput.MaxDepth = 1
	shallow, err := service.GraphBranchStructureComparison(context.Background(), shallowInput, "")
	if err != nil || !shallow.Truncated.From || !shallow.Truncated.To || len(shallow.TruncationReasons) == 0 {
		t.Fatalf("comparison depth truncation was not reported: %+v", shallow)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, shallow.RunID)
}

func TestNormalizeGraphBranchStructureComparisonInput(t *testing.T) {
	if _, err := normalizeGraphBranchStructureComparisonInput(GraphBranchStructureComparisonInput{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected empty comparison validation error, got %v", err)
	}
	input, err := normalizeGraphBranchStructureComparisonInput(GraphBranchStructureComparisonInput{FromTreeID: " b0000000-0000-0000-0000-000000000001 ", FromTreeVersionID: " b1000000-0000-0000-0000-000000000003 ", FromRootNodeID: " b2000000-0000-0000-0000-000000000001 ", ToTreeID: " b0000000-0000-0000-0000-000000000001 ", ToTreeVersionID: " b1000000-0000-0000-0000-000000000001 ", ToRootNodeID: " b2000000-0000-0000-0000-000000000005 ", MaxDepth: 99})
	if err != nil || input.MaxDepth != GraphMaxDepth {
		t.Fatalf("unexpected normalized comparison input: %+v, %v", input, err)
	}
	if _, err := normalizeGraphBranchStructureComparisonInput(GraphBranchStructureComparisonInput{FromTreeID: "b0000000-0000-0000-0000-000000000001", FromTreeVersionID: "b1000000-0000-0000-0000-000000000003", FromRootNodeID: "b2000000-0000-0000-0000-000000000001", ToTreeID: "b0000000-0000-0000-0000-000000000001", ToTreeVersionID: "b1000000-0000-0000-0000-000000000001", ToRootNodeID: "b2000000-0000-0000-0000-000000000004", MaxDepth: -1}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected negative comparison depth validation error, got %v", err)
	}
}

func TestGraphBranchStructureComparisonRejectsRootVersionMismatch(t *testing.T) {
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
	_, err = service.GraphBranchStructureComparison(context.Background(), GraphBranchStructureComparisonInput{FromTreeID: "b0000000-0000-0000-0000-000000000001", FromTreeVersionID: "b1000000-0000-0000-0000-000000000003", FromRootNodeID: "b2000000-0000-0000-0000-000000000001", ToTreeID: "b0000000-0000-0000-0000-000000000001", ToTreeVersionID: "b1000000-0000-0000-0000-000000000001", ToRootNodeID: "b2000000-0000-0000-0000-000000000001"}, "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected mismatched root/version to be hidden, got %v", err)
	}
}
