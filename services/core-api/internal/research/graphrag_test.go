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

func TestNormalizeShortestRelationshipInput(t *testing.T) {
	valid, err := validateQueryInput(QueryInput{
		Question:       "سؤال",
		GraphOperation: GraphOperationShortestPath,
		GraphStartType: "person",
		GraphStartID:   "10000000-0000-0000-0000-000000000001",
		GraphEndType:   "person",
		GraphEndID:     "10000000-0000-0000-0000-000000000002",
		TreeID:         "b0000000-0000-0000-0000-000000000001",
		TreeVersionID:  "b1000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if valid.GraphMaxDepth != GraphDefaultDepth {
		t.Fatalf("shortest path depth = %d, want %d", valid.GraphMaxDepth, GraphDefaultDepth)
	}
	if _, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationShortestPath, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndType: "person", GraphEndID: "10000000-0000-0000-0000-000000000001"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected same-endpoint validation error, got %v", err)
	}
	if _, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationShortestPath, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndType: "person", GraphEndID: "10000000-0000-0000-0000-000000000002"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected scoped shortest-path validation error, got %v", err)
	}
}

func TestNormalizeConnectedComponentInput(t *testing.T) {
	valid, err := validateQueryInput(QueryInput{
		Question:       "سؤال",
		GraphOperation: GraphOperationConnectedComponent,
		GraphStartType: "person",
		GraphStartID:   "10000000-0000-0000-0000-000000000001",
		TreeID:         "b0000000-0000-0000-0000-000000000001",
		TreeVersionID:  "b1000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if valid.GraphMaxDepth != GraphDefaultDepth || valid.GraphEndID != "" {
		t.Fatalf("unexpected connected component input: %+v", valid)
	}
	for _, input := range []QueryInput{
		{Question: "سؤال", GraphOperation: GraphOperationConnectedComponent, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001"},
		{Question: "سؤال", GraphOperation: GraphOperationConnectedComponent, GraphStartType: "family", GraphStartID: "10000000-0000-0000-0000-000000000001", TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000001"},
		{Question: "سؤال", GraphOperation: GraphOperationConnectedComponent, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndID: "10000000-0000-0000-0000-000000000002", TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000001"},
	} {
		if _, err := validateQueryInput(input); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected connected component validation error for %+v, got %v", input, err)
		}
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
	ownerResult, err := service.retrieveGraph(context.Background(), input, ownerID.String())
	if err != nil || len(ownerResult.Paths) == 0 || ownerResult.Paths[0].TreeScope.TreeID != treeID.String() {
		t.Fatalf("owner path scope was not persisted in retrieval: %+v", ownerResult.Paths)
	}
	runIDText, _, err := service.startRun(context.Background(), input, viewerID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, runIDText)
	if err := service.persistRun(context.Background(), runIDText, QueryResult{Answer: "إجابة", GraphPaths: ownerResult.Paths, GraphStats: ownerResult.Stats}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRun(context.Background(), viewerID.String(), runIDText); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unrelated researcher accessed private graph history: %v", err)
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
	detail, err := service.GetRun(context.Background(), viewerID.String(), runIDText)
	if err != nil || len(detail.GraphPaths) == 0 {
		t.Fatalf("resource collaborator could not read private graph history: %v", err)
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

func TestGraphShortestRelationshipPathIsBoundedAndDirectional(t *testing.T) {
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

	ownerID := uuid.New()
	treeID := uuid.New()
	versionID := uuid.New()
	personIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	nodeIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	edgeIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مالك شجرة المسار')`, ownerID, "shortest-path-"+ownerID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) SELECT p.id, p.name, p.name, $1 FROM (VALUES ($2::uuid, 'الشخص أ'), ($3::uuid, 'الشخص ب'), ($4::uuid, 'الشخص ج'), ($5::uuid, 'الشخص د'), ($6::uuid, 'الشخص هـ')) AS p(id, name)`, ownerID, personIDs[0], personIDs[1], personIDs[2], personIDs[3], personIDs[4]); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM people WHERE id = ANY($1::uuid[])`, personIDs)
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة المسار', 'public', $2)`, treeID, ownerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM trees WHERE id = $1`, treeID)
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state) VALUES ($1, $2, 1, 'published')`, versionID, treeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar) VALUES ($1, $3, $4, 'الشخص أ'), ($2, $3, $5, 'الشخص ب'), ($6, $3, $7, 'الشخص ج'), ($8, $3, $9, 'الشخص د'), ($10, $3, $11, 'الشخص هـ')`, nodeIDs[0], nodeIDs[1], versionID, personIDs[0], personIDs[1], nodeIDs[2], personIDs[2], nodeIDs[3], personIDs[3], nodeIDs[4], personIDs[4]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, created_by) VALUES ($1, $2, $3, $4, 'parent_of', 'interpreted', $5), ($6, $2, $4, $7, 'sibling_of', 'interpreted', $5), ($8, $2, $3, $9, 'parent_of', 'interpreted', $5), ($10, $2, $9, $11, 'parent_of', 'interpreted', $5), ($12, $2, $11, $7, 'parent_of', 'interpreted', $5)`, edgeIDs[0], versionID, nodeIDs[0], nodeIDs[2], ownerID, edgeIDs[1], nodeIDs[1], edgeIDs[2], nodeIDs[3], edgeIDs[3], nodeIDs[4], edgeIDs[4]); err != nil {
		t.Fatal(err)
	}

	input, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationShortestPath, GraphStartType: "person", GraphStartID: personIDs[0].String(), GraphEndType: "person", GraphEndID: personIDs[1].String(), TreeID: treeID.String(), TreeVersionID: versionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Pool: pool}
	result, err := service.retrieveGraph(ctx, input, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 1 || result.Paths[0].Depth != 2 || len(result.Paths[0].Nodes) != 3 || len(result.Paths[0].Edges) != 2 {
		t.Fatalf("unexpected shortest paths: %+v", result.Paths)
	}
	path := result.Paths[0]
	if path.AlgorithmVersion != GraphShortestPathAlgorithm || !path.StructuralOnly || path.EvidenceBacked || path.TreeScope.TreeID != treeID.String() || path.TreeScope.TreeVersionID != versionID.String() {
		t.Fatalf("unexpected shortest path metadata: %+v", path)
	}
	if path.Edges[0].Predicate != "parent_of" || path.Edges[1].Predicate != "sibling_of" {
		t.Fatalf("unexpected shortest path predicates: %+v", path.Edges)
	}
	reverseInput, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationShortestPath, GraphStartType: "person", GraphStartID: personIDs[1].String(), GraphEndType: "person", GraphEndID: personIDs[0].String(), TreeID: treeID.String(), TreeVersionID: versionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := service.retrieveGraph(ctx, reverseInput, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(reverse.Paths) != 1 || len(reverse.Paths[0].Edges) == 0 || reverse.Paths[0].Edges[0].FromNodeID == reverse.Paths[0].Edges[0].PathFromNodeID {
		t.Fatalf("shortest path traversal direction was lost: %+v", reverse.Paths)
	}
}

func TestGraphConnectedComponentIsVersionScopedAndBounded(t *testing.T) {
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

	ownerID := uuid.New()
	treeID := uuid.New()
	versionID := uuid.New()
	emptyVersionID := uuid.New()
	personIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	nodeIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	edgeIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مالك المكوّن')`, ownerID, "component-"+ownerID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) SELECT p.id, p.name, p.name, $1 FROM (VALUES ($2::uuid, 'جذر المكوّن'), ($3::uuid, 'الفرد الثاني'), ($4::uuid, 'الفرد الثالث'), ($5::uuid, 'الفرد الرابع'), ($6::uuid, 'فرد منفصل'), ($7::uuid, 'فرد منفصل آخر')) AS p(id, name)`, ownerID, personIDs[0], personIDs[1], personIDs[2], personIDs[3], personIDs[4], personIDs[5]); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM people WHERE id = ANY($1::uuid[])`, personIDs)
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة المكوّن', 'public', $2)`, treeID, ownerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM trees WHERE id = $1`, treeID)
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state) VALUES ($1, $2, 1, 'published'), ($3, $2, 2, 'published')`, versionID, treeID, emptyVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar) VALUES ($1, $3, $4, 'جذر المكوّن'), ($2, $3, $5, 'الفرد الثاني'), ($6, $3, $7, 'الفرد الثالث'), ($8, $3, $9, 'الفرد الرابع'), ($10, $3, $11, 'فرد منفصل'), ($12, $3, $13, 'فرد منفصل آخر')`, nodeIDs[0], nodeIDs[1], versionID, personIDs[0], personIDs[1], nodeIDs[2], personIDs[2], nodeIDs[3], personIDs[3], nodeIDs[4], personIDs[4], nodeIDs[5], personIDs[5]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, created_by) VALUES ($1, $2, $3, $4, 'parent_of', 'interpreted', $5), ($6, $2, $4, $7, 'sibling_of', 'unresolved', $5), ($8, $2, $7, $9, 'spouse_of', 'disputed', $5), ($10, $2, $9, $3, 'parent_of', 'interpreted', $5), ($11, $2, $12, $13, 'parent_of', 'interpreted', $5)`, edgeIDs[0], versionID, nodeIDs[0], nodeIDs[1], ownerID, edgeIDs[1], nodeIDs[2], edgeIDs[2], nodeIDs[3], edgeIDs[3], edgeIDs[4], nodeIDs[4], nodeIDs[5]); err != nil {
		t.Fatal(err)
	}

	input, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationConnectedComponent, GraphStartType: "person", GraphStartID: personIDs[0].String(), TreeID: treeID.String(), TreeVersionID: versionID.String(), GraphMaxDepth: GraphMaxDepth})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Pool: pool}
	result, err := service.retrieveGraph(ctx, input, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 1 || len(result.Paths[0].Nodes) != 4 || len(result.Paths[0].Edges) != 4 || result.Paths[0].Status != "contested" || !result.Paths[0].StructuralOnly || result.Paths[0].Truncated {
		t.Fatalf("unexpected connected component: %+v", result.Paths)
	}
	if result.Paths[0].TreeScope.TreeVersionID != versionID.String() || result.Paths[0].AlgorithmVersion != GraphComponentAlgorithm {
		t.Fatalf("component scope or algorithm mismatch: %+v", result.Paths[0])
	}
	for _, node := range result.Paths[0].Nodes {
		if node.ID == nodeIDs[4].String() || node.ID == nodeIDs[5].String() {
			t.Fatalf("disconnected node leaked into component: %+v", result.Paths[0].Nodes)
		}
	}
	repeat, err := service.retrieveGraph(ctx, input, "")
	if err != nil || len(repeat.Paths) != 1 || repeat.Paths[0].ID != result.Paths[0].ID {
		t.Fatalf("component result was not deterministic: %+v", repeat.Paths)
	}
	shallowInput, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationConnectedComponent, GraphStartType: "person", GraphStartID: personIDs[0].String(), TreeID: treeID.String(), TreeVersionID: versionID.String(), GraphMaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	shallow, err := service.retrieveGraph(ctx, shallowInput, "")
	if err != nil || len(shallow.Paths) != 1 || len(shallow.Paths[0].Nodes) != 3 || !shallow.Paths[0].Truncated {
		t.Fatalf("depth cap was not reported: %+v", shallow.Paths)
	}
	emptyInput, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationConnectedComponent, GraphStartType: "person", GraphStartID: personIDs[0].String(), TreeID: treeID.String(), TreeVersionID: emptyVersionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := service.retrieveGraph(ctx, emptyInput, "")
	if err != nil || len(empty.Paths) != 0 {
		t.Fatalf("component crossed the selected version: %+v", empty.Paths)
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
		{name: "connected_component_scoped", input: QueryInput{Question: "سؤال", GraphOperation: GraphOperationConnectedComponent, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphStartType: "person", TreeID: "b0000000-0000-0000-0000-000000000001", TreeVersionID: "b1000000-0000-0000-0000-000000000003", GraphMaxDepth: GraphMaxDepth}},
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
