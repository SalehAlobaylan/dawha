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

func TestGeographicGraphTraversesMultipleEvents(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	firstEvent := uuid.MustParse("a1000000-0000-0000-0000-000000009001")
	secondEvent := uuid.MustParse("a1000000-0000-0000-0000-000000009002")
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, created_by)
		VALUES ($1, 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003', '1220-01-01', '1221-01-01', 'documented', 'precise', '30000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001'),
		       ($2, 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000002', '1221-01-01', '1222-01-01', 'documented', 'precise', '30000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001')
	`, firstEvent, secondEvent); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM migration_events WHERE id IN ($1, $2)`, firstEvent, secondEvent)
	input, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationGeographic, GraphStartType: "place", GraphStartID: "20000000-0000-0000-0000-000000000001", GraphEndType: "place", GraphEndID: "20000000-0000-0000-0000-000000000002", GraphMaxDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&Service{Pool: pool}).retrieveGraph(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) == 0 || len(result.Paths[0].Edges) != 2 || result.Paths[0].Depth != 2 {
		t.Fatalf("unexpected multi-hop geographic paths: %+v", result.Paths)
	}
}

func TestGraphPrivateTreeRequiresResourceAccess(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	viewerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ownerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name_ar) VALUES ($1, 'graph-private-owner@dawha.local', 'مالك شجرة الاختبار') ON CONFLICT (id) DO NOTHING`, ownerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	roleExisted := false
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role = 'researcher')`, viewerID).Scan(&roleExisted); err != nil {
		t.Fatal(err)
	}
	if !roleExisted {
		if _, err := pool.Exec(context.Background(), `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, viewerID); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(context.Background(), `DELETE FROM user_roles WHERE user_id = $1 AND role = 'researcher'`, viewerID)
	}
	treeID := uuid.MustParse("b0000000-0000-0000-0000-000000009001")
	versionID := uuid.MustParse("b1000000-0000-0000-0000-000000009001")
	nodeOne := uuid.MustParse("b2000000-0000-0000-0000-000000009001")
	nodeTwo := uuid.MustParse("b2000000-0000-0000-0000-000000009002")
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة خاصة للاختبار', 'private', $2)`, []any{treeID, ownerID}},
		{`INSERT INTO tree_versions (id, tree_id, version_number, state) VALUES ($1, $2, 1, 'published')`, []any{versionID, treeID}},
		{`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar) VALUES ($1, $2, '10000000-0000-0000-0000-000000000001', 'عبدالله بن محمد'), ($3, $2, '10000000-0000-0000-0000-000000000002', 'محمد بن سعد')`, []any{nodeOne, versionID, nodeTwo}},
		{`INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, created_by) VALUES ('b3000000-0000-0000-0000-000000009001', $1, $2, $3, 'parent_of', 'disputed', $4)`, []any{versionID, nodeOne, nodeTwo, ownerID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(context.Background(), statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(context.Background(), `DELETE FROM trees WHERE id = $1`, treeID)
	input, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationCommonAncestor, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndType: "person", GraphEndID: "10000000-0000-0000-0000-000000000002", TreeID: treeID.String(), TreeVersionID: versionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Pool: pool}
	result, err := service.retrieveGraph(context.Background(), input, viewerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 0 {
		t.Fatalf("unrelated researcher saw private tree paths: %+v", result.Paths)
	}
	treeItems, err := service.retrieveTreeInterpretations(context.Background(), retrievalContext{Input: input, Normalized: "عبدالله", ActorID: viewerID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(treeItems) != 0 {
		t.Fatalf("unrelated researcher saw private tree citations: %+v", treeItems)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'view', $3)`, treeID, viewerID, ownerID); err != nil {
		t.Fatal(err)
	}
	result, err = service.retrieveGraph(context.Background(), input, viewerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) == 0 {
		t.Fatal("resource collaborator did not see private tree path")
	}
	if result.Paths[0].Status != "contested" {
		t.Fatalf("disputed tree path status = %q", result.Paths[0].Status)
	}
	reverseInput, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationCommonAncestor, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000002", GraphEndType: "person", GraphEndID: "10000000-0000-0000-0000-000000000001", TreeID: treeID.String(), TreeVersionID: versionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	reverseResult, err := service.retrieveGraph(context.Background(), reverseInput, viewerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(reverseResult.Paths) == 0 || len(reverseResult.Paths[0].Edges) == 0 || reverseResult.Paths[0].Edges[0].FromNodeID == reverseResult.Paths[0].Edges[0].PathFromNodeID {
		t.Fatalf("edge direction lost traversal semantics: %+v", reverseResult.Paths)
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

func TestGraphPassageCitations(t *testing.T) {
	paths := []GraphPath{{
		EvidenceRefs: []GraphEvidenceRef{
			{ID: "50000000-0000-0000-0000-000000000001", Type: "source_statement", SourceID: "30000000-0000-0000-0000-000000000001", StatementID: "50000000-0000-0000-0000-000000000001", PassageID: "40000000-0000-0000-0000-000000000001", Title: "مصدر", Excerpt: "نص"},
			{ID: "50000000-0000-0000-0000-000000000002", Type: "source_statement", SourceID: "30000000-0000-0000-0000-000000000002", StatementID: "50000000-0000-0000-0000-000000000002", Title: "مصدر بلا مقطع", Excerpt: "نص"},
		},
	}}
	items := graphPassageCitations(paths, nil)
	if len(items) != 2 || items[0].PassageID != "40000000-0000-0000-0000-000000000001" || items[0].Layer != SourceStatement || items[1].StatementID != "50000000-0000-0000-0000-000000000002" || items[1].PassageID != "" {
		t.Fatalf("unexpected graph citations: %+v", items)
	}
	if len(graphPassageCitations(paths, items)) != 0 {
		t.Fatal("duplicate graph evidence was not suppressed")
	}
}

func TestSourceEntitiesDepthIsFixed(t *testing.T) {
	value, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationSourceEntities, GraphStartType: "source", GraphStartID: "30000000-0000-0000-0000-000000000001", GraphMaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if value.GraphMaxDepth != 3 {
		t.Fatalf("source entity depth = %d, want 3", value.GraphMaxDepth)
	}
}

func TestGraphIncludesPassageOnlyEvidence(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	sourceID := uuid.MustParse("39000000-0000-0000-0000-000000009001")
	passageID := uuid.MustParse("49000000-0000-0000-0000-000000009001")
	claimID := uuid.MustParse("69000000-0000-0000-0000-000000009002")
	defer pool.Exec(context.Background(), `DELETE FROM sources WHERE id = $1`, sourceID)
	defer pool.Exec(context.Background(), `DELETE FROM claims WHERE id = $1`, claimID)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO sources (id, title_ar, source_type) VALUES ($1, 'مصدر بمقطع فقط', 'book')`, []any{sourceID}},
		{`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, 'مقطع دون عبارة', 'مقطع دون عباره')`, []any{passageID, sourceID}},
		{`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', '10000000-0000-0000-0000-000000000001', 'father_of', 'person', '10000000-0000-0000-0000-000000000003', 'supported', '00000000-0000-0000-0000-000000000001')`, []any{claimID}},
		{`INSERT INTO claim_evidence (claim_id, source_passage_id, relation, created_by) VALUES ($1, $2, 'supports', '00000000-0000-0000-0000-000000000001')`, []any{claimID, passageID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(context.Background(), statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	input, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationBranchClaims, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&Service{Pool: pool}).retrieveGraph(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range result.Paths {
		for _, evidence := range path.EvidenceRefs {
			if evidence.Type == "source_passage" && evidence.PassageID == passageID.String() {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("passage-only evidence was omitted: %+v", result.Paths)
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
