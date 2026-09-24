package research

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestNormalizeGraphInput(t *testing.T) {
	valid, err := validateQueryInput(QueryInput{
		Question:       "سؤال",
		GraphOperation: " COMMON_ANCESTOR_PATH ",
		GraphStartType: " PERSON ",
		GraphStartID:   " 10000000-0000-0000-0000-000000000001 ",
		GraphEndID:     "10000000-0000-0000-0000-000000000002",
		GraphMaxDepth:  99,
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if valid.GraphOperation != GraphOperationCommonAncestor || valid.GraphStartType != "person" || valid.GraphStartID != "10000000-0000-0000-0000-000000000001" || valid.GraphMaxDepth != GraphMaxDepth {
		t.Fatalf("unexpected normalized graph input: %+v", valid)
	}
}

func TestNormalizeGraphInputRejectsInvalidCombinations(t *testing.T) {
	cases := []QueryInput{
		{Question: "سؤال", GraphOperation: "unknown"},
		{Question: "سؤال", GraphOperation: GraphOperationCommonAncestor, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndID: "10000000-0000-0000-0000-000000000002", GraphStartType: "family", GraphEndType: "family"},
		{Question: "سؤال", GraphOperation: GraphOperationEvidence, GraphStartType: "family", GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndType: "person", GraphEndID: "10000000-0000-0000-0000-000000000002"},
		{Question: "سؤال", GraphOperation: GraphOperationSourceEntities, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001"},
		{Question: "سؤال", GraphOperation: GraphOperationBranchClaims, GraphStartType: "place", GraphStartID: "20000000-0000-0000-0000-000000000001"},
		{Question: "سؤال", GraphStartID: "10000000-0000-0000-0000-000000000001"},
	}
	for _, input := range cases {
		if _, err := validateQueryInput(input); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected validation error for %+v, got %v", input, err)
		}
	}
}

func TestGraphRetrievalSkipsOrdinaryQueries(t *testing.T) {
	result, err := (&Service{}).retrieveGraph(context.Background(), QueryInput{Question: "سؤال"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Paths == nil || len(result.Paths) != 0 || result.Stats.Operation != "" {
		t.Fatalf("unexpected ordinary graph result: %+v", result)
	}
}

func TestGraphPathInvariants(t *testing.T) {
	paths := []GraphPath{{
		ID:               "10000000-0000-0000-0000-000000000001",
		Operation:        GraphOperationEvidence,
		Status:           "complete",
		Explanation:      "مصفوفة",
		Depth:            1,
		EvidenceBacked:   true,
		AlgorithmVersion: GraphAlgorithmVersion,
		Nodes:            []GraphNode{{ID: "10000000-0000-0000-0000-000000000001", Type: "person", Position: 0}, {ID: "10000000-0000-0000-0000-000000000002", Type: "person", Position: 1}},
		Edges:            []GraphEdge{{ID: "60000000-0000-0000-0000-000000000001", Type: "claim", FromNodeID: "10000000-0000-0000-0000-000000000001", ToNodeID: "10000000-0000-0000-0000-000000000002", Position: 0}},
		EvidenceRefs:     []GraphEvidenceRef{{ID: "50000000-0000-0000-0000-000000000001", Type: "source_statement"}},
	}}
	if err := validateGraphPaths(paths, GraphDefaultDepth); err != nil {
		t.Fatalf("unexpected path validation error: %v", err)
	}
	paths[0].EvidenceBacked = true
	paths[0].StructuralOnly = true
	if err := validateGraphPaths(paths, GraphDefaultDepth); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected structural conflict, got %v", err)
	}
}

func TestNormalizeGraphPathsEnforcesPathLimit(t *testing.T) {
	paths := make([]GraphPath, GraphMaxPaths+1)
	for index := range paths {
		paths[index] = GraphPath{ID: uuid.NewString(), Operation: GraphOperationGeographic, Status: "complete", Depth: 1, AlgorithmVersion: GraphAlgorithmVersion, Nodes: []GraphNode{{ID: uuid.NewString(), Type: "place", Position: 0}}}
	}
	paths = normalizeGraphPaths(paths)
	if len(paths) != GraphMaxPaths {
		t.Fatalf("path count = %d, want %d", len(paths), GraphMaxPaths)
	}
}

func TestGraphQueriesAgainstDatabase(t *testing.T) {
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
	cases := []struct {
		name  string
		input QueryInput
	}{
		{name: "common", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationCommonAncestor, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndID: "10000000-0000-0000-0000-000000000002", GraphStartType: "person", GraphEndType: "person", GraphMaxDepth: GraphDefaultDepth}},
		{name: "common_scoped", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationCommonAncestor, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndID: "10000000-0000-0000-0000-000000000002", GraphStartType: "person", GraphEndType: "person", TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003", GraphMaxDepth: GraphDefaultDepth}},
		{name: "evidence", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationEvidence, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndID: "10000000-0000-0000-0000-000000000002", GraphStartType: "person", GraphEndType: "person", GraphMaxDepth: GraphDefaultDepth}},
		{name: "branch", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationBranchClaims, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphStartType: "person", GraphMaxDepth: GraphDefaultDepth}},
		{name: "source", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationSourceEntities, GraphStartID: "30000000-0000-0000-0000-000000000001", GraphStartType: "source", GraphMaxDepth: GraphDefaultDepth}},
		{name: "geographic", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationGeographic, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphStartType: "person", GraphEndID: "20000000-0000-0000-0000-000000000001", GraphEndType: "place", GraphMaxDepth: GraphDefaultDepth}},
		{name: "geographic_places", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationGeographic, GraphStartID: "20000000-0000-0000-0000-000000000002", GraphStartType: "place", GraphEndID: "20000000-0000-0000-0000-000000000001", GraphEndType: "place", GraphMaxDepth: GraphDefaultDepth}},
	}
	treeItems, treeErr := service.retrieveTreeInterpretations(context.Background(), retrievalContext{Input: QueryInput{Question: "عبدالله", TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003"}, Normalized: "عبدالله"})
	if treeErr != nil || len(treeItems) == 0 {
		t.Fatalf("scoped tree retrieval failed: %v", treeErr)
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input, validationErr := validateQueryInput(testCase.input)
			if validationErr != nil {
				t.Fatal(validationErr)
			}
			result, queryErr := service.retrieveGraph(context.Background(), input, "")
			if queryErr != nil {
				t.Fatal(queryErr)
			}
			if len(result.Paths) == 0 {
				t.Fatalf("expected graph paths for %s", testCase.name)
			}
			if err := validateGraphPaths(result.Paths, input.GraphMaxDepth); err != nil {
				t.Fatal(err)
			}
		})
	}
}
