package research

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
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

// The corpus the fingerprint test writes twice. Four sources and five dependency
// statements, all public and all inside the depth bound, so the neighborhood is
// the whole graph and nothing is truncated.
//
// The ids are literals rather than uuid.New() because "the same corpus" has to
// mean the same sources. Two databases that invented different source ids would
// be two different graphs, and comparing fingerprints across them would prove
// nothing about the fingerprint.
//
// The two statements between a and b are deliberate: a dependency statement is
// identified by the pair of sources and its predicate, not by the pair alone, so
// a fingerprint that ignored the predicate would call two different graphs one.
var (
	graphFingerprintRootID     = uuid.MustParse("31000000-0000-0000-0000-000000000000")
	graphFingerprintFirstID    = uuid.MustParse("31000000-0000-0000-0000-000000000001")
	graphFingerprintSecondID   = uuid.MustParse("31000000-0000-0000-0000-000000000002")
	graphFingerprintThirdID    = uuid.MustParse("31000000-0000-0000-0000-000000000003")
	graphFingerprintStatements = []struct {
		from, to  uuid.UUID
		predicate string
		status    string
	}{
		{graphFingerprintRootID, graphFingerprintFirstID, "cites", "confirmed"},
		{graphFingerprintRootID, graphFingerprintSecondID, "derived_from", "needs_review"},
		{graphFingerprintFirstID, graphFingerprintSecondID, "cites", "confirmed"},
		{graphFingerprintFirstID, graphFingerprintSecondID, "likely_paraphrase", "confirmed"},
		{graphFingerprintSecondID, graphFingerprintThirdID, "shared_origin", "confirmed"},
	}
)

// seedGraphFingerprintCorpus writes the corpus into one schema, in the order
// asked for.
//
// It never supplies source_dependencies.id, so each schema gets its own
// gen_random_uuid() row ids. That is the point of the test rather than an
// inconvenience: a fingerprint that survives two schemas has to survive
// different storage row ids, and seeding explicit ids here would remove the
// thing being measured. Insertion order is reversed in the second schema so an
// order-dependent fingerprint fails too, and not only a uuid-dependent one.
func seedGraphFingerprintCorpus(t *testing.T, fixture *testsupport.Fixture, reverse bool) {
	t.Helper()
	sourceIDs := []uuid.UUID{graphFingerprintRootID, graphFingerprintFirstID, graphFingerprintSecondID, graphFingerprintThirdID}
	if reverse {
		sourceIDs = []uuid.UUID{graphFingerprintThirdID, graphFingerprintSecondID, graphFingerprintFirstID, graphFingerprintRootID}
	}
	for _, sourceID := range sourceIDs {
		fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility)
			VALUES ($1, 'مصدر بصمة الجوار', 'article', 'public')`, sourceID)
	}
	statements := graphFingerprintStatements
	if reverse {
		statements = make([]struct {
			from, to  uuid.UUID
			predicate string
			status    string
		}, 0, len(graphFingerprintStatements))
		for index := len(graphFingerprintStatements) - 1; index >= 0; index-- {
			statements = append(statements, graphFingerprintStatements[index])
		}
	}
	for _, statement := range statements {
		fixture.Exec(`INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status)
			VALUES ($1, $2, $3, $4)`, statement.from, statement.to, statement.predicate, statement.status)
	}
	fixture.Exec(`ANALYZE sources`)
	fixture.Exec(`ANALYZE source_dependencies`)
}

// graphFingerprintEdgeRowIDs is the corpus's storage row ids, so the test can
// state as a fact that the two schemas really were seeded with different ones.
// Without this check a green result could mean the two schemas happened to be
// seeded identically, which would make the test a tautology.
//
// Scoped to the corpus's own four sources because every fixture schema also
// carries the demo seed, and one of its rows is somebody else's dependency.
func graphFingerprintEdgeRowIDs(t *testing.T, fixture *testsupport.Fixture) []string {
	t.Helper()
	corpus := []uuid.UUID{graphFingerprintRootID, graphFingerprintFirstID, graphFingerprintSecondID, graphFingerprintThirdID}
	rows, err := fixture.Pool().Query(fixture.Ctx(), `SELECT id::text FROM source_dependencies
		WHERE source_id = ANY($1::uuid[]) OR depends_on_source_id = ANY($1::uuid[])
		ORDER BY id::text`, corpus)
	if err != nil {
		t.Fatalf("read dependency row ids: %v", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan dependency row id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read dependency row ids: %v", err)
	}
	return ids
}

// graphFingerprintReading is everything a caller can compare between two
// measurements of a neighborhood: the identities and the graph content.
type graphFingerprintReading struct {
	pathID               string
	inputFingerprint     string
	edgeSetFingerprint   string
	partitionFingerprint string
	communityPathID      string
	nodes                int
	edges                int
	communities          int
}

func readGraphFingerprints(t *testing.T, service *Service, ctx context.Context) graphFingerprintReading {
	t.Helper()
	input := GraphSourceDependencyNeighborhoodInput{SourceID: graphFingerprintRootID.String(), MaxDepth: GraphMaxDepth}
	neighborhood, err := service.GraphSourceDependencyNeighborhood(ctx, input, "")
	if err != nil {
		t.Fatalf("source dependency neighborhood: %v", err)
	}
	communities, err := service.GraphSourceDependencyCommunities(ctx, GraphSourceDependencyCommunitiesInput{SourceID: graphFingerprintRootID.String(), MaxDepth: GraphMaxDepth, MinCommunitySize: 2}, "")
	if err != nil {
		t.Fatalf("source dependency communities: %v", err)
	}
	return graphFingerprintReading{
		pathID:               neighborhood.Summary.PathID,
		inputFingerprint:     neighborhood.Summary.InputFingerprint,
		edgeSetFingerprint:   neighborhood.Summary.EdgeSetFingerprint,
		partitionFingerprint: communities.Summary.PartitionFingerprint,
		communityPathID:      communities.Summary.PathID,
		nodes:                len(neighborhood.Path.Nodes),
		edges:                len(neighborhood.Path.Edges),
		communities:          communities.Summary.ReportedCommunityCount,
	}
}

func (reading graphFingerprintReading) String() string {
	return "path=" + reading.pathID + " edges=" + reading.edgeSetFingerprint + " partition=" + reading.partitionFingerprint + " communities=" + reading.communityPathID
}

// TestGraphSourceDependencyFingerprintIsTheSameGraphInTwoSchemas is the test
// that a fingerprint is a property of the graph.
//
// It builds the same corpus twice, in two separately created schemas on the same
// server, with different random source_dependencies row ids and in opposite
// insertion order, and requires the two measurements to be equal. A test that
// only compares two calls inside one database is the test that let a
// random-uuid fingerprint through: inside one database the rows do not change,
// so it cannot tell a fingerprint of the graph from a fingerprint of the row
// ids.
//
// Three things are asserted, and the third is the one that makes the first two
// mean something: the same corpus in two schemas is equal, the same corpus read
// repeatedly is equal, and a DIFFERENT neighborhood is not equal. Without the
// third, a fingerprint that returned a constant would pass this test.
func TestGraphSourceDependencyFingerprintIsTheSameGraphInTwoSchemas(t *testing.T) {
	first := testsupport.New(t)
	second := testsupport.New(t)
	seedGraphFingerprintCorpus(t, first, false)
	seedGraphFingerprintCorpus(t, second, true)

	// The premise, stated as an assertion: the two schemas hold the same graph
	// over different storage rows.
	firstRowIDs := graphFingerprintEdgeRowIDs(t, first)
	secondRowIDs := graphFingerprintEdgeRowIDs(t, second)
	if len(firstRowIDs) != len(graphFingerprintStatements) || len(secondRowIDs) != len(graphFingerprintStatements) {
		t.Fatalf("the corpus seeded %d and %d dependency rows, wanted %d in each schema", len(firstRowIDs), len(secondRowIDs), len(graphFingerprintStatements))
	}
	shared := 0
	for _, id := range firstRowIDs {
		for _, other := range secondRowIDs {
			if id == other {
				shared++
			}
		}
	}
	if shared != 0 {
		t.Fatalf("the two schemas were seeded with %d identical dependency row ids, so this run is not comparing two databases' worth of random ids", shared)
	}

	firstReading := readGraphFingerprints(t, &Service{Pool: first.Pool()}, first.Ctx())
	secondReading := readGraphFingerprints(t, &Service{Pool: second.Pool()}, second.Ctx())
	if firstReading.nodes != 4 || firstReading.edges != len(graphFingerprintStatements) {
		t.Fatalf("the corpus is not the graph this test meant to build: %d nodes and %d edges", firstReading.nodes, firstReading.edges)
	}
	if firstReading.String() != secondReading.String() {
		t.Fatalf("the same graph in two separately created schemas produced two different fingerprints.\n  first:  %s\n  second: %s", firstReading, secondReading)
	}

	// Repeated measurement of one corpus, in one database, must also be equal.
	// This is the property the old test asserted, kept because a content
	// fingerprint can still be unstable within a database if something in the
	// computation is order-dependent rather than id-dependent.
	for attempt := 0; attempt < 3; attempt++ {
		again := readGraphFingerprints(t, &Service{Pool: first.Pool()}, first.Ctx())
		if again.String() != firstReading.String() {
			t.Fatalf("reading %d of the same corpus returned different fingerprints.\n  first: %s\n  again: %s", attempt+1, firstReading, again)
		}
	}

	// The control. One more dependency statement is a different neighborhood, and
	// a different neighborhood must not share a fingerprint - otherwise everything
	// above would pass with a fingerprint that never changed.
	first.Exec(`INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status)
		VALUES ($1, $2, 'derived_from', 'confirmed')`, graphFingerprintSecondID, graphFingerprintFirstID)
	first.Exec(`ANALYZE source_dependencies`)
	changed := readGraphFingerprints(t, &Service{Pool: first.Pool()}, first.Ctx())
	if changed.edgeSetFingerprint == firstReading.edgeSetFingerprint {
		t.Fatalf("adding a dependency statement left the edge-set fingerprint at %s, so the fingerprint is not a property of the graph", changed.edgeSetFingerprint)
	}
	if changed.pathID == firstReading.pathID {
		t.Fatalf("adding a dependency statement left the path id at %s, so the path id is not a property of the graph", changed.pathID)
	}
	t.Logf("one corpus in two schemas: %s", firstReading)
	t.Logf("the same corpus with one more statement: %s", changed)
}
