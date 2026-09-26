package research

// The graph benchmark: what the bounded source-dependency graph actually costs
// in PostgreSQL at, below and above the 200-node/200-edge bounds in
// graphrag.go, and how stable the answer is when the same graph is asked for
// twice.
//
// It exists because IMPLEMENTATION_PLAN.md:1619-1646 defers Neo4j behind a
// measurement gate and ARCHITECTURE.md:605-632 calls it a future derived
// projection. A decision that is waiting for evidence needs the evidence
// recorded somewhere a reviewer can read it, and this file is where it is
// produced: docs/graph-benchmark.md holds the table, and the run that filled it
// is reproduced by `make graph-benchmark`.
//
// Two rules shape what is measured here.
//
// First, no production algorithm changes. The query under test is the exact
// string the service runs, with the same parameters, the same read-only
// repeatable-read transaction and the same 2s statement timeout, and the
// community detection measured is the same Go function. Where the benchmark
// needs something the product does not have - the unbounded traversal cost, the
// EXPLAIN plan - it runs its own query in the test file and says so in the
// report, so a reader can never mistake a measurement aid for the shipped path.
//
// Second, the graph is synthetic. Sources carry a marker title and a reserved
// public visibility, they exist only inside a fixture schema that is dropped
// when the test ends, and no real source, person or document is involved. The
// point of the measurement is the shape of the graph, not its contents.
//
// The file holds two tests with different jobs:
//
//   - TestGraphSourceDependencyNeighborhoodStaysBoundedAcrossGraphSizes runs in
//     the normal acceptance suite. It is the assertion: whatever the machine,
//     output stays inside the bounds, truncation is reported when the graph is
//     larger than the bounds, and the same graph twice produces the same path.
//   - TestGraphSourceDependencyBenchmark is gated on DAWHA_GRAPH_BENCH and is
//     the measurement. It times the operations, records allocations, captures
//     the query plan, checks the result is stable, and writes the JSON report
//     the decision document quotes. It is not part of `make verify` or
//     `make verify-full` because a number that moves is not a gate, and because
//     it is a measurement command a reader runs deliberately.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// graphBenchmarkEnv is the opt-in. The measurement is deliberately not part
	// of the acceptance gate: latency on one laptop is not a release signal, and
	// a gate that fails when the machine is busy teaches people to ignore gates.
	graphBenchmarkEnv = "DAWHA_GRAPH_BENCH"
	// graphBenchmarkIterationsEnv overrides the sample count. The percentile
	// method is nearest-rank, so a small sample is honest about what it does
	// not know: p95 of 20 samples is the 19th slowest observation.
	graphBenchmarkIterationsEnv = "DAWHA_GRAPH_BENCH_ITERATIONS"
	// graphBenchmarkReportEnv is where the machine-readable report is written.
	graphBenchmarkReportEnv = "DAWHA_GRAPH_BENCH_REPORT"

	defaultGraphBenchmarkIterations = 20
	defaultGraphBenchmarkReport     = "docs/benchmarks/graph-source-dependency.json"

	// graphBenchmarkMaxDepth is the depth every scenario is asked for. The size
	// axis is what this benchmark varies; depth is held constant so a p95 is
	// comparable across scenarios. Depth truncation is asserted separately, in
	// TestGraphSourceDependencyNeighborhoodIsBoundedAndPrivate.
	graphBenchmarkMaxDepth = GraphDefaultDepth
)

// graphBenchmarkShape is one synthetic public-source dependency graph.
//
// BranchCount sources depend on the root. When Width is greater than zero, each
// of those is itself the source of Width further dependencies, so the graph has
// real depth rather than being a star.
//
// ParallelEdges gives every branch a second, differently-typed dependency
// statement on the branch before it, which is the only way a graph of this shape
// gets an edge set denser than its node set. It matters: a wide layered graph
// on its own trips the NODE bound first and returns 199 edges, so the EDGE
// bound is never exercised. ExtraEdges adds the same kind of statement but only
// for the first few branches, which is how the "at the bound" scenario reaches
// exactly 200 edges without changing the node count.
type graphBenchmarkShape struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	BranchCount   int    `json:"branch_count"`
	Width         int    `json:"width"`
	ParallelEdges int    `json:"parallel_edges"`
	ExtraEdges    int    `json:"extra_edges"`

	AvailableNodes int `json:"available_nodes"`
	AvailableEdges int `json:"available_edges"`
}

// graphBenchmarkShapes are the three graphs the decision rests on: below the
// bounds, exactly at them, and above both of them.
func graphBenchmarkShapes() []graphBenchmarkShape {
	return []graphBenchmarkShape{
		{
			Name:           "below-50-nodes-49-edges",
			Description:    "root plus 49 direct dependencies: a quarter of the node bound and a quarter of the edge bound, and the case where truncation must NOT be reported",
			BranchCount:    49,
			AvailableNodes: 50,
			AvailableEdges: 49,
		},
		{
			Name:           "at-200-nodes-200-edges",
			Description:    "root plus 199 direct dependencies and one extra dependency statement between two of them: exactly both bounds, and the case where a bound that fired here would carry no information",
			BranchCount:    199,
			ExtraEdges:     1,
			AvailableNodes: 200,
			AvailableEdges: 200,
		},
		{
			Name:           "above-1201-nodes-1599-edges",
			Description:    "root plus 400 direct dependencies, each with 2 of its own and a second dependency statement on its predecessor: six times the node bound and eight times the edge bound, dense enough inside the bounded node set that both limits fire",
			BranchCount:    400,
			Width:          2,
			ParallelEdges:  1,
			AvailableNodes: 1201,
			// 400 root edges + 800 branch edges + 399 parallel edges. The
			// parallel ones are branch[i] -> branch[i-1] for i = 1..399, and
			// there are 399 of them because branch[0] is the root and is not a
			// branch. The unbounded probe below is what caught the 1600 this
			// used to say.
			AvailableEdges: 1599,
		},
	}
}

// graphBenchmarkMeasurement is the timing and allocation record for one
// operation.
//
// FirstMS is reported separately from the percentiles rather than dropped from
// the sample. The first call against a freshly seeded graph pays for a cold plan
// and a cold buffer cache, and it is real - it is what the first user after a
// deploy waits for - but averaging it into a p95 would make the steady state look
// worse than it is. Keeping it visible lets a reader decide which number they
// care about, which is the only honest way to handle it.
//
// BytesPerOp and AllocsPerOp are Go-side heap growth for the call - the process,
// not PostgreSQL - measured with runtime.ReadMemStats around the whole timed
// loop and divided by the sample count.
type graphBenchmarkMeasurement struct {
	Samples     int     `json:"samples"`
	FirstMS     float64 `json:"first_call_ms"`
	MinMS       float64 `json:"min_ms"`
	P50MS       float64 `json:"p50_ms"`
	P95MS       float64 `json:"p95_ms"`
	MaxMS       float64 `json:"max_ms"`
	MeanMS      float64 `json:"mean_ms"`
	BytesPerOp  int64   `json:"bytes_per_op"`
	AllocsPerOp int64   `json:"allocs_per_op"`
}

// graphBenchmarkPlanScan is one node of an EXPLAIN plan, kept only for the rows
// and relations a reader needs to judge the plan.
type graphBenchmarkPlanScan struct {
	NodeType           string  `json:"node_type"`
	Relation           string  `json:"relation,omitempty"`
	Index              string  `json:"index,omitempty"`
	ActualRows         int64   `json:"actual_rows"`
	ActualLoops        int64   `json:"actual_loops"`
	ActualTotalTimeMS  float64 `json:"actual_total_time_ms"`
	SharedHitBlocks    int64   `json:"shared_hit_blocks"`
	SharedReadBlocks   int64   `json:"shared_read_blocks"`
	SortMethod         string  `json:"sort_method,omitempty"`
	SortSpaceUsedBytes int64   `json:"sort_space_used_bytes,omitempty"`
}

// graphBenchmarkQueryPlan is the recorded plan for the shipped query.
type graphBenchmarkQueryPlan struct {
	Note             string                   `json:"note"`
	PlanningMS       float64                  `json:"planning_ms"`
	ExecutionMS      float64                  `json:"execution_ms"`
	SharedHitBlocks  int64                    `json:"shared_hit_blocks"`
	SharedReadBlocks int64                    `json:"shared_read_blocks"`
	NodeTypes        map[string]int           `json:"node_types"`
	Scans            []graphBenchmarkPlanScan `json:"scans"`
	TopNodes         []graphBenchmarkPlanScan `json:"top_nodes"`
	SharedBuffersHit bool                     `json:"shared_buffers_used"`
	Root             *graphBenchmarkPlanScan  `json:"root,omitempty"`
	StatementTimeout string                   `json:"statement_timeout"`
}

// graphBenchmarkUnboundedProbe is the cost of the same traversal with no bounds
// at all. It exists because a bounded query that answers in a millisecond says
// nothing about where the bound hurts, and a decision document that omits this
// number cannot say how much headroom PostgreSQL has left.
type graphBenchmarkUnboundedProbe struct {
	Note            string  `json:"note"`
	ReachableNodes  int     `json:"reachable_nodes"`
	ReachableEdges  int     `json:"reachable_edges"`
	ElapsedMS       float64 `json:"elapsed_ms"`
	TimedOut        bool    `json:"timed_out"`
	OvershootFactor float64 `json:"overshoot_factor_vs_max_nodes"`
	// Failed means the probe itself errored. It is recorded separately from
	// TimedOut because a broken measurement aid must never be reported as a
	// property of the database.
	Failed string `json:"failed,omitempty"`
	// MatchesExpectation says the unbounded traversal agreed with the arithmetic
	// in graphBenchmarkShapes. A disagreement means the recorded numbers and the
	// graph that produced them are describing different things, and a reader
	// should not trust either.
	MatchesExpectation bool `json:"matches_expected_shape"`
}

// graphBenchmarkStability is what repeated requests returned, and whether the
// same graph rebuilt from scratch returns the same answer.
//
// The two questions are different and the difference is the interesting part.
// Within one database the neighborhood is fully deterministic: the path id, the
// input fingerprint, the edge-set fingerprint and the community partition
// fingerprint are all equal across every repeat. Across two databases holding
// equivalent graphs, the first three can differ, because source_dependencies.id
// is a random uuid and the query orders candidate edges by it. The recorded
// ReseedReproduction is what the run measured, and it is a record rather than an
// assertion on purpose: a fingerprint that is stable inside a database and not
// across databases is a property of the schema, and calling it a failure or a
// success would be inventing a verdict the code does not express.
type graphBenchmarkStability struct {
	PathID               string `json:"path_id"`
	InputFingerprint     string `json:"input_fingerprint"`
	EdgeSetFingerprint   string `json:"edge_set_fingerprint"`
	PartitionFingerprint string `json:"partition_fingerprint,omitempty"`
	RepeatedRequests     int    `json:"repeated_requests"`
	Stable               bool   `json:"stable_within_the_run"`
	ReseedReproduction   string `json:"reseed_reproduction"`
}

// graphBenchmarkScenario is one row of the recorded table.
type graphBenchmarkScenario struct {
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	RootSourceID      string                    `json:"root_source_id"`
	AvailableNodes    int                       `json:"available_nodes"`
	AvailableEdges    int                       `json:"available_edges"`
	ReturnedNodes     int                       `json:"returned_nodes"`
	ReturnedEdges     int                       `json:"returned_edges"`
	NodeRetention     float64                   `json:"node_retention"`
	EdgeRetention     float64                   `json:"edge_retention"`
	Requests          int                       `json:"requests"`
	TruncatedRequests int                       `json:"truncated_requests"`
	TruncationRate    float64                   `json:"truncation_rate"`
	Truncated         bool                      `json:"truncated"`
	TruncationReasons []string                  `json:"truncation_reasons"`
	Status            string                    `json:"status"`
	Retrieval         graphBenchmarkMeasurement `json:"retrieval"`
	Neighborhood      graphBenchmarkMeasurement `json:"neighborhood"`
	CommunityDetect   graphBenchmarkMeasurement `json:"community_detection"`
	Communities       graphBenchmarkMeasurement `json:"communities"`
	PartitionCount    int                       `json:"partition_count"`
	LargestCommunity  int                       `json:"largest_community"`
	// InferredMergeIterations is the number of community merges the greedy
	// modularity loop performed, derived as nodes minus surviving partitions:
	// every merge removes exactly one state, and the loop only ends when no
	// merge has positive gain. It is an inference from the output, not a counter
	// read out of the function, and it is here because it is the variable that
	// actually drives the community-detection cost - the loop rebuilds its
	// between-community edge map once per merge.
	InferredMergeIterations int                          `json:"inferred_merge_iterations"`
	QueryPlan               graphBenchmarkQueryPlan      `json:"query_plan"`
	Unbounded               graphBenchmarkUnboundedProbe `json:"unbounded_probe"`
	Stability               graphBenchmarkStability      `json:"stability"`
}

// graphBenchmarkReport is the whole run, and the assertions the run makes about
// itself. The assertions are recorded next to the numbers so a reader can tell a
// measurement that held its bounds from one that did not.
type graphBenchmarkReport struct {
	GeneratedAt      string                   `json:"generated_at"`
	Command          string                   `json:"command"`
	Commit           string                   `json:"commit,omitempty"`
	GoVersion        string                   `json:"go_version"`
	NumCPU           int                      `json:"num_cpu"`
	ServerVersion    string                   `json:"postgres_server_version"`
	Iterations       int                      `json:"iterations"`
	PercentileMethod string                   `json:"percentile_method"`
	Requests         int                      `json:"total_requests"`
	TruncationRate   float64                  `json:"truncation_rate"`
	TruncationNote   string                   `json:"truncation_rate_note"`
	Bounds           graphBenchmarkBounds     `json:"bounds"`
	Scenarios        []graphBenchmarkScenario `json:"scenarios"`
	Assertions       graphBenchmarkAssertions `json:"assertions"`
}

type graphBenchmarkBounds struct {
	MaxDepth int `json:"max_depth"`
	MaxNodes int `json:"max_nodes"`
	MaxEdges int `json:"max_edges"`
}

type graphBenchmarkAssertions struct {
	BoundedOutput           bool `json:"bounded_output"`
	TruncationReportedAbove bool `json:"truncation_reported_above_the_bound"`
	NoTruncationAtTheBound  bool `json:"no_truncation_at_the_bound"`
	StableResults           bool `json:"stable_results"`
	// SeededShapeMatchesTraversal says every scenario's seeded arithmetic
	// agreed with what the unbounded traversal actually reached. It is the check
	// on the benchmark itself: without it, a table row could describe a graph
	// that was never built.
	SeededShapeMatchesTraversal bool `json:"seeded_shape_matches_traversal"`
}

// graphBenchmarkNamespace is the fixed prefix every synthetic source id in this
// file carries. It is a plan-009 constant: a future run that reuses it can be
// compared to a recorded one, and nothing outside the benchmark resolves it.
var graphBenchmarkNamespace = uuid.MustParse("9d1a4c62-0f5b-5e2a-9b7c-000000000009")

// graphBenchmarkSourceID is the deterministic id of one node in one scenario.
//
// The id is derived from the scenario name and the node's index, and it SORTS
// in index order. That is not cosmetic. The neighborhood query keeps the first
// GraphMaxNodes nodes ordered by (depth, source_id), so with unordered ids the
// retained set is a hash-ordered subset nobody can predict, and a measurement
// cannot say which edges survived truncation or which shape it actually
// measured. With ordered ids, the retained set is the root plus the first
// branches by index, and the arithmetic in graphBenchmarkShapes can be checked
// against the traversal.
func graphBenchmarkSourceID(scenario string, index int) uuid.UUID {
	sum := sha256.Sum256([]byte("dawha-graph-benchmark/" + scenario))
	raw := make([]byte, 16)
	copy(raw[0:8], sum[0:8])
	binary.BigEndian.PutUint64(raw[8:16], uint64(index))
	id, err := uuid.FromBytes(raw)
	if err != nil {
		// Unreachable: raw is always sixteen bytes. Panicking in a test helper
		// beats returning a zero id that would collide with a real row.
		panic("graph benchmark id: " + err.Error())
	}
	return id
}

// seedGraphBenchmarkShape writes one synthetic graph into the fixture and
// returns the root source id. The titles are markers, not text: a performance
// document that quoted Arabic prose would be quoting something that could be
// mistaken for a real source, and nothing here reads them.
func seedGraphBenchmarkShape(t *testing.T, fixture *testsupport.Fixture, shape graphBenchmarkShape) uuid.UUID {
	t.Helper()
	root := graphBenchmarkSourceID(shape.Name, 0)
	ids := make([]uuid.UUID, 0, shape.AvailableNodes)
	ids = append(ids, root)
	branches := make([]uuid.UUID, 0, shape.BranchCount)
	leaves := make([]uuid.UUID, 0, shape.BranchCount*shape.Width)
	index := 1
	for branch := 0; branch < shape.BranchCount; branch++ {
		id := graphBenchmarkSourceID(shape.Name, index)
		index++
		branches = append(branches, id)
		ids = append(ids, id)
		for leaf := 0; leaf < shape.Width; leaf++ {
			leafID := graphBenchmarkSourceID(shape.Name, index)
			index++
			leaves = append(leaves, leafID)
			ids = append(ids, leafID)
		}
	}
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility)
		SELECT id, 'benchmark-' || $2, 'article', 'public' FROM unnest($1::uuid[]) AS source(id)`,
		ids, shape.Name)

	if shape.BranchCount > 0 {
		fixture.Exec(`INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status)
			SELECT $1, id, 'cites', 'confirmed' FROM unnest($2::uuid[]) AS source(id)`,
			root, branches)
	}
	if shape.Width > 0 {
		pairs := make([]string, 0, len(leaves))
		for position, branch := range branches {
			for leaf := 0; leaf < shape.Width; leaf++ {
				pairs = append(pairs, branch.String()+":"+leaves[position*shape.Width+leaf].String())
			}
		}
		insertGraphBenchmarkDependencyPairs(t, fixture, pairs, "derived_from", "needs_review")
	}
	if shape.ExtraEdges > 0 {
		pairs := make([]string, 0, shape.ExtraEdges)
		for extra := 1; extra <= shape.ExtraEdges && extra < len(branches); extra++ {
			pairs = append(pairs, branches[extra].String()+":"+branches[extra-1].String())
		}
		insertGraphBenchmarkDependencyPairs(t, fixture, pairs, parallelDependencyType, "confirmed")
	}
	if shape.ParallelEdges > 0 {
		pairs := make([]string, 0, len(branches))
		for position := 1; position < len(branches); position++ {
			pairs = append(pairs, branches[position].String()+":"+branches[position-1].String())
		}
		insertGraphBenchmarkDependencyPairs(t, fixture, pairs, parallelDependencyType, "confirmed")
	}
	if got := fixture.Count(`SELECT count(*) FROM sources WHERE id = ANY($1::uuid[])`, ids); got != len(ids) {
		t.Fatalf("scenario %s seeded %d sources, wanted %d", shape.Name, got, len(ids))
	}
	// ANALYZE, and it is not optional. A table that has just been written has no
	// statistics until autovacuum gets to it, and a query planned against
	// missing statistics is measuring the planner's guess rather than the graph.
	// This was found the hard way: the first run of the above-the-bound scenario
	// measured a p50 of 127ms while EXPLAIN ANALYZE of the same query on the same
	// rows measured 7ms, because the loop ran before autovacuum had analyzed the
	// freshly inserted rows. The comment in the report records it, because a
	// reader who reproduces this benchmark and skips ANALYZE will get a number
	// roughly eighteen times worse and no idea why.
	fixture.Exec(`ANALYZE sources`)
	fixture.Exec(`ANALYZE source_dependencies`)
	return root
}

// dropGraphBenchmarkShape removes one scenario's rows so the same graph can be
// seeded again. It exists to measure whether the answers survive a rebuild, and
// it only deletes rows this file created.
func dropGraphBenchmarkShape(t *testing.T, fixture *testsupport.Fixture, shape graphBenchmarkShape) {
	t.Helper()
	ids := make([]uuid.UUID, 0, shape.AvailableNodes)
	for index := 0; index < shape.AvailableNodes; index++ {
		ids = append(ids, graphBenchmarkSourceID(shape.Name, index))
	}
	fixture.Exec(`DELETE FROM source_dependencies WHERE source_id = ANY($1::uuid[]) OR depends_on_source_id = ANY($1::uuid[])`, ids)
	fixture.Exec(`DELETE FROM sources WHERE id = ANY($1::uuid[])`, ids)
}

// parallelDependencyType is the dependency type used for the second statement
// between the same pair of sources. It differs from the root's 'cites' because
// source_dependencies is unique on
// (source_id, depends_on_source_id, dependency_type) - which is also why a dense
// edge set in this benchmark has to be built from parallel statements rather
// than from repeated ones.
const parallelDependencyType = "likely_paraphrase"

// insertGraphBenchmarkDependencyPairs writes one row per "from:to" pair.
func insertGraphBenchmarkDependencyPairs(t *testing.T, fixture *testsupport.Fixture, pairs []string, dependencyType, status string) {
	t.Helper()
	if len(pairs) == 0 {
		return
	}
	from := make([]uuid.UUID, 0, len(pairs))
	to := make([]uuid.UUID, 0, len(pairs))
	for _, pair := range pairs {
		parts := strings.SplitN(pair, ":", 2)
		from = append(from, uuid.MustParse(parts[0]))
		to = append(to, uuid.MustParse(parts[1]))
	}
	fixture.Exec(`INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status)
		SELECT from_id, to_id, $3, $4 FROM unnest($1::uuid[], $2::uuid[]) AS pair(from_id, to_id)`,
		from, to, dependencyType, status)
}

// measureGraphBenchmark times `iterations` calls of run and reports the nearest
// rank percentiles plus the Go-side allocations the calls caused.
func measureGraphBenchmark(iterations int, run func(iteration int) error) (graphBenchmarkMeasurement, error) {
	if iterations < 1 {
		iterations = 1
	}
	samples := make([]float64, 0, iterations)
	// TotalAlloc is cumulative, so a collection before the loop changes nothing
	// about the delta; it is here so the delta is the workload's and not the
	// fixture's.
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	first := 0.0
	for iteration := 0; iteration < iterations; iteration++ {
		start := time.Now()
		if err := run(iteration); err != nil {
			return graphBenchmarkMeasurement{}, err
		}
		elapsed := float64(time.Since(start).Microseconds()) / 1000
		if iteration == 0 {
			first = elapsed
		}
		samples = append(samples, elapsed)
	}
	runtime.ReadMemStats(&after)
	sort.Float64s(samples)
	total := 0.0
	for _, sample := range samples {
		total += sample
	}
	measurement := graphBenchmarkMeasurement{
		Samples:     len(samples),
		FirstMS:     roundGraphBenchmark(first),
		MinMS:       roundGraphBenchmark(samples[0]),
		P50MS:       roundGraphBenchmark(graphBenchmarkPercentile(samples, 0.50)),
		P95MS:       roundGraphBenchmark(graphBenchmarkPercentile(samples, 0.95)),
		MaxMS:       roundGraphBenchmark(samples[len(samples)-1]),
		MeanMS:      roundGraphBenchmark(total / float64(len(samples))),
		BytesPerOp:  int64(after.TotalAlloc-before.TotalAlloc) / int64(len(samples)),
		AllocsPerOp: int64(after.Mallocs-before.Mallocs) / int64(len(samples)),
	}
	return measurement, nil
}

// graphBenchmarkPercentile is the nearest-rank percentile: the smallest sample
// at or above the requested fraction of the sorted sample. Interpolating would
// report a p95 no iteration ever produced, which is the number a reader would
// then quote.
func graphBenchmarkPercentile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(fraction*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func roundGraphBenchmark(value float64) float64 {
	return math.Round(value*1000) / 1000
}

// graphBenchmarkUnboundedTraversalQuery counts everything the same traversal
// would reach with no node, edge or saturation bound.
//
// It is a measurement aid and not the shipped path: production truncates, and
// that is the point of production. The query is written to be obviously the
// same traversal - same active-edge definition, same depth - with the limits
// removed, so the difference between the two numbers is the cost of the bound
// and not the cost of a different algorithm.
const graphBenchmarkUnboundedTraversalQuery = `
WITH RECURSIVE
active_edges AS (
  SELECT d.source_id, d.depends_on_source_id
  FROM source_dependencies d
  JOIN sources dependent ON dependent.id = d.source_id AND dependent.visibility = 'public'
  JOIN sources target ON target.id = d.depends_on_source_id AND target.visibility = 'public'
  WHERE d.status IN ('needs_review', 'confirmed') AND d.depends_on_source_id IS NOT NULL
),
walk (depth, frontier, visited) AS (
  SELECT 0, ARRAY[$1::uuid], ARRAY[$1::uuid]
  UNION ALL
  SELECT w.depth + 1, next_nodes.frontier, w.visited || next_nodes.frontier
  FROM walk w
  CROSS JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT e.depends_on_source_id), ARRAY[]::uuid[]) AS frontier
    FROM active_edges e
    WHERE e.source_id = ANY(w.frontier) AND NOT e.depends_on_source_id = ANY(w.visited)
  ) next_nodes
  WHERE w.depth < $2 AND COALESCE(array_length(next_nodes.frontier, 1), 0) > 0
),
visited_nodes AS (
  SELECT DISTINCT unnest(visited) AS source_id FROM walk
)
SELECT (SELECT count(*) FROM visited_nodes)::bigint,
       (SELECT count(*) FROM active_edges e JOIN visited_nodes v ON v.source_id = e.source_id)::bigint
`

// probeGraphBenchmarkUnbounded runs the unbounded traversal under its own
// generous timeout and checks its count against the arithmetic in
// graphBenchmarkShapes.
//
// A timeout is recorded as a timeout and an error as an error, because they are
// different answers: a timeout means the traversal does not finish, which is
// itself a finding, and an error means the probe is broken, which is a finding
// about the probe and must not be read as a finding about PostgreSQL. The probe
// is a measurement aid, so when it fails the run fails rather than reporting a
// zero.
func probeGraphBenchmarkUnbounded(ctx context.Context, fixture *testsupport.Fixture, shape graphBenchmarkShape, root uuid.UUID) graphBenchmarkUnboundedProbe {
	probe := graphBenchmarkUnboundedProbe{Note: "benchmark-only recursive traversal with no node, edge or saturation bound; production truncates at GraphMaxNodes/GraphMaxEdges"}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	var nodes, edges int64
	err := fixture.Pool().QueryRow(probeCtx, graphBenchmarkUnboundedTraversalQuery, root, graphBenchmarkMaxDepth).Scan(&nodes, &edges)
	probe.ElapsedMS = roundGraphBenchmark(float64(time.Since(start).Microseconds()) / 1000)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, pgx.ErrNoRows) {
			probe.TimedOut = true
			return probe
		}
		probe.Failed = err.Error()
		return probe
	}
	probe.ReachableNodes = int(nodes)
	probe.ReachableEdges = int(edges)
	probe.MatchesExpectation = probe.ReachableNodes == shape.AvailableNodes && probe.ReachableEdges == shape.AvailableEdges
	if shape.AvailableNodes > 0 {
		probe.OvershootFactor = math.Round((float64(probe.ReachableNodes)/float64(GraphMaxNodes))*100) / 100
	}
	return probe
}

// explainGraphBenchmarkPlan runs the shipped query under EXPLAIN (ANALYZE,
// BUFFERS, FORMAT JSON) in the same read-only transaction shape the service
// uses, and reduces the plan to the parts a reviewer argues about.
func explainGraphBenchmarkPlan(ctx context.Context, fixture *testsupport.Fixture, root uuid.UUID) (graphBenchmarkQueryPlan, error) {
	plan := graphBenchmarkQueryPlan{
		Note:             "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) of the exact query text services/core-api/internal/research/graph_source_dependency.go runs, with the same five parameters and the same read-only repeatable-read transaction",
		StatementTimeout: "2000ms, as the service sets it",
		NodeTypes:        map[string]int{},
	}
	planCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := fixture.Pool().BeginTx(planCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return plan, err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(planCtx, `SET LOCAL statement_timeout = '2000ms'`); err != nil {
		return plan, err
	}
	var raw []byte
	if err := tx.QueryRow(planCtx, `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) `+sourceDependencyNeighborhoodGraphQuery,
		root, nil, graphBenchmarkMaxDepth, GraphMaxNodes, GraphMaxEdges).Scan(&raw); err != nil {
		return plan, err
	}
	var parsed []struct {
		Plan         graphBenchmarkExplainNode `json:"Plan"`
		PlanningTime float64                   `json:"Planning Time"`
		ExecTime     float64                   `json:"Execution Time"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return plan, err
	}
	if len(parsed) == 0 {
		return plan, fmt.Errorf("EXPLAIN returned no plan")
	}
	plan.PlanningMS = roundGraphBenchmark(parsed[0].PlanningTime)
	plan.ExecutionMS = roundGraphBenchmark(parsed[0].ExecTime)
	scans := []graphBenchmarkPlanScan{}
	collectGraphBenchmarkPlan(parsed[0].Plan, &scans, &plan)
	sort.SliceStable(scans, func(left, right int) bool {
		return scans[left].ActualTotalTimeMS > scans[right].ActualTotalTimeMS
	})
	plan.SharedBuffersHit = plan.SharedHitBlocks+plan.SharedReadBlocks > 0
	if len(scans) > 0 {
		root := scans[0]
		plan.Root = &root
	}
	if len(scans) > 8 {
		scans = scans[:8]
	}
	plan.Scans = scans
	plan.TopNodes = plan.Scans
	return plan, nil
}

type graphBenchmarkExplainNode struct {
	NodeType         string                      `json:"Node Type"`
	RelationName     string                      `json:"Relation Name"`
	IndexName        string                      `json:"Index Name"`
	ActualRows       int64                       `json:"Actual Rows"`
	ActualLoops      int64                       `json:"Actual Loops"`
	ActualTotalTime  float64                     `json:"Actual Total Time"`
	SharedHitBlocks  int64                       `json:"Shared Hit Blocks"`
	SharedReadBlocks int64                       `json:"Shared Read Blocks"`
	SortMethod       string                      `json:"Sort Method"`
	SortSpaceUsed    int64                       `json:"Sort Space Used"`
	Plans            []graphBenchmarkExplainNode `json:"Plans"`
}

func collectGraphBenchmarkPlan(node graphBenchmarkExplainNode, scans *[]graphBenchmarkPlanScan, plan *graphBenchmarkQueryPlan) {
	plan.NodeTypes[node.NodeType]++
	plan.SharedHitBlocks += node.SharedHitBlocks
	plan.SharedReadBlocks += node.SharedReadBlocks
	if node.RelationName != "" || node.IndexName != "" {
		*scans = append(*scans, graphBenchmarkPlanScan{
			NodeType:           node.NodeType,
			Relation:           node.RelationName,
			Index:              node.IndexName,
			ActualRows:         node.ActualRows,
			ActualLoops:        node.ActualLoops,
			ActualTotalTimeMS:  roundGraphBenchmark(node.ActualTotalTime),
			SharedHitBlocks:    node.SharedHitBlocks,
			SharedReadBlocks:   node.SharedReadBlocks,
			SortMethod:         node.SortMethod,
			SortSpaceUsedBytes: node.SortSpaceUsed,
		})
	}
	for _, child := range node.Plans {
		collectGraphBenchmarkPlan(child, scans, plan)
	}
}

// graphBenchmarkReportPath resolves where the JSON report lands. The default is
// inside the repository so the recorded run can be committed next to the
// decision document that quotes it.
func graphBenchmarkReportPath(t *testing.T) string {
	t.Helper()
	if override := os.Getenv(graphBenchmarkReportEnv); override != "" {
		return override
	}
	root, err := testsupport.FindRepositoryRoot(".")
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	return filepath.Join(root, defaultGraphBenchmarkReport)
}

func graphBenchmarkIterationCount(t *testing.T) int {
	t.Helper()
	raw := os.Getenv(graphBenchmarkIterationsEnv)
	if raw == "" {
		return defaultGraphBenchmarkIterations
	}
	var count int
	if _, err := fmt.Sscanf(raw, "%d", &count); err != nil || count < 1 {
		t.Fatalf("%s must be a positive integer, got %q", graphBenchmarkIterationsEnv, raw)
	}
	return count
}

// TestGraphSourceDependencyNeighborhoodStaysBoundedAcrossGraphSizes is the
// assertion half of this file, and it runs in the ordinary acceptance suite.
//
// The bounds in graphrag.go are only a claim until something fails when they
// stop holding. This walks the same three graphs the benchmark measures - below
// the bound, at it, above it - through the shipped operation and asserts four
// things: output never exceeds GraphMaxNodes/GraphMaxEdges; a graph above the
// bound reports truncation and says which limit bit; a graph exactly at the
// bound does NOT report truncation, because a bound that always fires is a
// bound that carries no information; and two requests for the same graph return
// the same path, so a reader of the neighborhood can cache it.
func TestGraphSourceDependencyNeighborhoodStaysBoundedAcrossGraphSizes(t *testing.T) {
	fixture := testsupport.New(t)
	service := &Service{Pool: fixture.Pool()}
	ctx := fixture.Ctx()

	for _, shape := range graphBenchmarkShapes() {
		shape := shape
		t.Run(shape.Name, func(t *testing.T) {
			root := seedGraphBenchmarkShape(t, fixture, shape)
			input := GraphSourceDependencyNeighborhoodInput{SourceID: root.String(), MaxDepth: graphBenchmarkMaxDepth}
			first, err := service.GraphSourceDependencyNeighborhood(ctx, input, "")
			if err != nil {
				t.Fatalf("source dependency neighborhood over a %d-node graph: %v", shape.AvailableNodes, err)
			}
			second, err := service.GraphSourceDependencyNeighborhood(ctx, input, "")
			if err != nil {
				t.Fatalf("repeat of the same neighborhood: %v", err)
			}
			communities, err := service.GraphSourceDependencyCommunities(ctx, GraphSourceDependencyCommunitiesInput{SourceID: root.String(), MaxDepth: graphBenchmarkMaxDepth}, "")
			if err != nil {
				t.Fatalf("source dependency communities over a %d-node graph: %v", shape.AvailableNodes, err)
			}
			repeated, err := service.GraphSourceDependencyCommunities(ctx, GraphSourceDependencyCommunitiesInput{SourceID: root.String(), MaxDepth: graphBenchmarkMaxDepth}, "")
			if err != nil {
				t.Fatalf("repeat of the same communities: %v", err)
			}

			if len(first.Path.Nodes) > GraphMaxNodes || len(first.Path.Edges) > GraphMaxEdges {
				t.Fatalf("output left its bounds at %d nodes and %d edges (bound is %d/%d)",
					len(first.Path.Nodes), len(first.Path.Edges), GraphMaxNodes, GraphMaxEdges)
			}
			if len(communities.Path.Nodes) > GraphMaxNodes || len(communities.Path.Edges) > GraphMaxEdges {
				t.Fatalf("community output left its bounds at %d nodes and %d edges (bound is %d/%d)",
					len(communities.Path.Nodes), len(communities.Path.Edges), GraphMaxNodes, GraphMaxEdges)
			}
			if first.Summary.PathID != second.Summary.PathID ||
				first.Summary.InputFingerprint != second.Summary.InputFingerprint ||
				first.Summary.EdgeSetFingerprint != second.Summary.EdgeSetFingerprint {
				t.Fatalf("the same graph returned two different neighborhoods: %s/%s vs %s/%s",
					first.Summary.PathID, first.Summary.EdgeSetFingerprint,
					second.Summary.PathID, second.Summary.EdgeSetFingerprint)
			}
			if communities.Summary.PartitionFingerprint != repeated.Summary.PartitionFingerprint {
				t.Fatalf("the same graph returned two different community partitions: %s vs %s",
					communities.Summary.PartitionFingerprint, repeated.Summary.PartitionFingerprint)
			}
			// A community partition is computed from a Go map iteration, so
			// this is the assertion that matters for it: the fingerprint is a
			// hash of a sorted partition, and it has to be equal across runs on
			// the same data or the fingerprint is worthless.

			atBound := shape.AvailableNodes == GraphMaxNodes && shape.AvailableEdges == GraphMaxEdges
			switch {
			case atBound:
				if first.Summary.Truncated {
					t.Fatalf("a graph exactly at the bounds (%d nodes, %d edges) reported truncation: %v",
						shape.AvailableNodes, shape.AvailableEdges, first.Summary.TruncationReasons)
				}
				if len(first.Path.Nodes) != GraphMaxNodes || len(first.Path.Edges) != GraphMaxEdges {
					t.Fatalf("a graph exactly at the bounds returned %d nodes and %d edges, wanted exactly %d/%d",
						len(first.Path.Nodes), len(first.Path.Edges), GraphMaxNodes, GraphMaxEdges)
				}
			case shape.AvailableNodes > GraphMaxNodes || shape.AvailableEdges > GraphMaxEdges:
				if !first.Summary.Truncated {
					t.Fatalf("a graph above the bounds (%d nodes, %d edges) did not report truncation",
						shape.AvailableNodes, shape.AvailableEdges)
				}
				for _, reason := range []string{"node_limit", "edge_limit"} {
					if !containsString(first.Summary.TruncationReasons, reason) {
						t.Fatalf("truncation reasons %v do not name %s, which the %d-node/%d-edge graph must have hit",
							first.Summary.TruncationReasons, reason, shape.AvailableNodes, shape.AvailableEdges)
					}
				}
				if len(first.Path.Nodes) != GraphMaxNodes || len(first.Path.Edges) != GraphMaxEdges {
					t.Fatalf("a truncated graph returned %d nodes and %d edges, wanted the caps %d/%d",
						len(first.Path.Nodes), len(first.Path.Edges), GraphMaxNodes, GraphMaxEdges)
				}
			default:
				if first.Summary.Truncated {
					t.Fatalf("a graph below the bounds (%d nodes, %d edges) reported truncation: %v",
						shape.AvailableNodes, shape.AvailableEdges, first.Summary.TruncationReasons)
				}
				if len(first.Path.Nodes) != shape.AvailableNodes {
					t.Fatalf("a graph below the bounds returned %d nodes, wanted all %d", len(first.Path.Nodes), shape.AvailableNodes)
				}
			}
			if communities.Summary.Truncated != first.Summary.Truncated {
				t.Fatalf("the community operation reported a different truncation verdict than the neighborhood: %v vs %v",
					communities.Summary.Truncated, first.Summary.Truncated)
			}
		})
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// TestGraphSourceDependencyBenchmark is the measurement half. It is gated on
// DAWHA_GRAPH_BENCH because it writes a report and asserts nothing a release
// should depend on: it is the command that fills in docs/graph-benchmark.md,
// not a gate. It still fails loudly when a bound stops holding, because a
// benchmark that quietly measured a runaway query would be worse than none.
func TestGraphSourceDependencyBenchmark(t *testing.T) {
	if os.Getenv(graphBenchmarkEnv) == "" {
		t.Skip("set DAWHA_GRAPH_BENCH=1 to measure the bounded source-dependency graph; it is a measurement command, not part of the acceptance suite")
	}
	fixture := testsupport.New(t)
	service := &Service{Pool: fixture.Pool()}
	ctx := fixture.Ctx()
	iterations := graphBenchmarkIterationCount(t)

	report := graphBenchmarkReport{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Command:          "make graph-benchmark",
		GoVersion:        runtime.Version(),
		NumCPU:           runtime.NumCPU(),
		Iterations:       iterations,
		PercentileMethod: "nearest rank: p50 is the slowest sample of the lower half, p95 is the ceil(0.95*n)-th slowest sample; no interpolation, so every reported percentile is an observation that happened",
		TruncationNote:   "the run-level truncation rate is over the measured neighborhood requests, and it is a property of the three chosen sizes rather than a measurement of how often real traffic truncates: a graph below or at the bounds cannot truncate and a graph above them must",
		Bounds:           graphBenchmarkBounds{MaxDepth: graphBenchmarkMaxDepth, MaxNodes: GraphMaxNodes, MaxEdges: GraphMaxEdges},
		Scenarios:        []graphBenchmarkScenario{},
	}
	if commit := graphBenchmarkCommit(); commit != "" {
		report.Commit = commit
	}
	if err := fixture.Pool().QueryRow(ctx, `SHOW server_version`).Scan(&report.ServerVersion); err != nil {
		t.Fatalf("read the server version: %v", err)
	}

	assertions := graphBenchmarkAssertions{BoundedOutput: true, TruncationReportedAbove: true, NoTruncationAtTheBound: true, StableResults: true, SeededShapeMatchesTraversal: true}
	for _, shape := range graphBenchmarkShapes() {
		shape := shape
		scenario, ok := measureGraphBenchmarkScenario(t, fixture, service, ctx, shape, iterations, &assertions)
		if !ok {
			continue
		}
		report.Scenarios = append(report.Scenarios, scenario)
		t.Logf("%-30s returned %d nodes / %d edges of %d / %d available, truncated=%v %v, retrieval p50=%.3fms p95=%.3fms, unbounded traversal %.3fms",
			scenario.Name, scenario.ReturnedNodes, scenario.ReturnedEdges, scenario.AvailableNodes, scenario.AvailableEdges,
			scenario.Truncated, scenario.TruncationReasons, scenario.Retrieval.P50MS, scenario.Retrieval.P95MS, scenario.Unbounded.ElapsedMS)
	}
	report.Assertions = assertions
	if len(report.Scenarios) == 0 {
		t.Fatal("the benchmark measured nothing")
	}
	requests := 0
	truncatedRequests := 0
	for _, scenario := range report.Scenarios {
		requests += scenario.Requests
		truncatedRequests += scenario.TruncatedRequests
	}
	report.Requests = requests
	if requests > 0 {
		report.TruncationRate = math.Round((float64(truncatedRequests)/float64(requests))*10000) / 10000
	}

	target := graphBenchmarkReportPath(t)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create the report directory: %v", err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encode the report: %v", err)
	}
	if err := os.WriteFile(target, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write the report: %v", err)
	}
	t.Logf("report written to %s", target)

	if !assertions.BoundedOutput {
		t.Error("a scenario returned more than GraphMaxNodes nodes or GraphMaxEdges edges")
	}
	if !assertions.NoTruncationAtTheBound {
		t.Error("a graph exactly at the bounds reported truncation")
	}
	if !assertions.TruncationReportedAbove {
		t.Error("a graph above the bounds did not report truncation")
	}
	if !assertions.StableResults {
		t.Error("the same graph returned a different result on a repeat request")
	}
	if !assertions.SeededShapeMatchesTraversal {
		t.Error("a seeded graph did not match the arithmetic that described it, so the numbers above describe a graph that was not built")
	}
}

// measureGraphBenchmarkScenario measures one shape and folds what it finds into
// the run's assertions. It returns false when the shape could not be measured,
// so a broken shape is skipped with a reason rather than silently averaged in.
func measureGraphBenchmarkScenario(t *testing.T, fixture *testsupport.Fixture, service *Service, ctx context.Context, shape graphBenchmarkShape, iterations int, assertions *graphBenchmarkAssertions) (graphBenchmarkScenario, bool) {
	t.Helper()
	root := seedGraphBenchmarkShape(t, fixture, shape)
	neighborhoodInput := GraphSourceDependencyNeighborhoodInput{SourceID: root.String(), MaxDepth: graphBenchmarkMaxDepth}
	communitiesInput := GraphSourceDependencyCommunitiesInput{SourceID: root.String(), MaxDepth: graphBenchmarkMaxDepth}

	scenario := graphBenchmarkScenario{
		Name:           shape.Name,
		Description:    shape.Description,
		RootSourceID:   root.String(),
		AvailableNodes: shape.AvailableNodes,
		AvailableEdges: shape.AvailableEdges,
	}

	// The retrieval is measured on its own because it is the part a graph
	// database would replace. The operation is measured on its own because it
	// is the part a user waits for. The difference is the run bookkeeping, and
	// reporting only one of them would hide which half the number describes.
	retrieval, err := measureGraphBenchmark(iterations, func(int) error {
		_, err := service.retrieveGraphSourceDependencyNeighborhood(ctx, neighborhoodInput, "")
		return err
	})
	if err != nil {
		t.Fatalf("measure retrieval over %s: %v", shape.Name, err)
	}
	scenario.Retrieval = retrieval

	neighborhood, err := measureGraphBenchmark(iterations, func(int) error {
		_, err := service.GraphSourceDependencyNeighborhood(ctx, neighborhoodInput, "")
		return err
	})
	if err != nil {
		t.Fatalf("measure the neighborhood over %s: %v", shape.Name, err)
	}
	scenario.Neighborhood = neighborhood

	// Community detection is the pure Go greedy modularity over the bounded
	// path, measured on the same path every time so the number is the algorithm
	// and not a query. It is the trigger condition IMPLEMENTATION_PLAN.md:1619
	// names for reconsidering Neo4j, so it gets its own line rather than being
	// folded into the operation around it.
	row, err := service.retrieveGraphSourceDependencyNeighborhood(ctx, neighborhoodInput, "")
	if err != nil {
		t.Fatalf("retrieve the path to measure community detection over %s: %v", shape.Name, err)
	}
	detection, err := measureGraphBenchmark(iterations, func(int) error {
		_, err := detectGraphSourceDependencyCommunities(row.Path, 2)
		return err
	})
	if err != nil {
		t.Fatalf("measure community detection over %s: %v", shape.Name, err)
	}
	scenario.CommunityDetect = detection

	communities, err := measureGraphBenchmark(iterations, func(int) error {
		_, err := service.GraphSourceDependencyCommunities(ctx, communitiesInput, "")
		return err
	})
	if err != nil {
		t.Fatalf("measure the communities operation over %s: %v", shape.Name, err)
	}
	scenario.Communities = communities

	first, err := service.GraphSourceDependencyNeighborhood(ctx, neighborhoodInput, "")
	if err != nil {
		t.Fatalf("final neighborhood read for the stability check over %s: %v", shape.Name, err)
	}
	firstCommunities, err := service.GraphSourceDependencyCommunities(ctx, communitiesInput, "")
	if err != nil {
		t.Fatalf("final community read for the stability check over %s: %v", shape.Name, err)
	}
	repeated, err := service.GraphSourceDependencyNeighborhood(ctx, neighborhoodInput, "")
	if err != nil {
		t.Fatalf("repeat read for the stability check over %s: %v", shape.Name, err)
	}
	repeatedCommunities, err := service.GraphSourceDependencyCommunities(ctx, communitiesInput, "")
	if err != nil {
		t.Fatalf("repeat community read for the stability check over %s: %v", shape.Name, err)
	}
	scenario.Stability = graphBenchmarkStability{
		PathID:               first.Summary.PathID,
		InputFingerprint:     first.Summary.InputFingerprint,
		EdgeSetFingerprint:   first.Summary.EdgeSetFingerprint,
		PartitionFingerprint: firstCommunities.Summary.PartitionFingerprint,
		RepeatedRequests:     iterations + 2,
		Stable: first.Summary.PathID == repeated.Summary.PathID &&
			first.Summary.InputFingerprint == repeated.Summary.InputFingerprint &&
			first.Summary.EdgeSetFingerprint == repeated.Summary.EdgeSetFingerprint &&
			firstCommunities.Summary.PartitionFingerprint == repeatedCommunities.Summary.PartitionFingerprint,
	}

	scenario.ReturnedNodes = len(first.Path.Nodes)
	scenario.ReturnedEdges = len(first.Path.Edges)
	scenario.Truncated = first.Summary.Truncated
	scenario.TruncationReasons = first.Summary.TruncationReasons
	if scenario.TruncationReasons == nil {
		scenario.TruncationReasons = []string{}
	}
	scenario.Status = first.Summary.Status
	scenario.PartitionCount = firstCommunities.Summary.PartitionCount
	scenario.LargestCommunity = firstCommunities.Summary.LargestCommunitySize
	scenario.InferredMergeIterations = scenario.ReturnedNodes - firstCommunities.Summary.PartitionCount
	if shape.AvailableNodes > 0 {
		scenario.NodeRetention = math.Round((float64(scenario.ReturnedNodes)/float64(shape.AvailableNodes))*10000) / 10000
	}
	if shape.AvailableEdges > 0 {
		scenario.EdgeRetention = math.Round((float64(scenario.ReturnedEdges)/float64(shape.AvailableEdges))*10000) / 10000
	}
	// Every request in this scenario asks the same question of the same rows, so
	// the verdict is per-scenario rather than per-request. Counting the requests
	// is still what makes the truncation RATE meaningful: a reader should see
	// that truncation was 20 of 20 requests, not 1 of 20 sampled by accident.
	scenario.Requests = iterations + 2
	scenario.TruncatedRequests = 0
	if scenario.Truncated {
		scenario.TruncatedRequests = scenario.Requests
	}
	scenario.TruncationRate = float64(scenario.TruncatedRequests) / float64(scenario.Requests)

	plan, err := explainGraphBenchmarkPlan(ctx, fixture, root)
	if err != nil {
		t.Fatalf("explain the neighborhood query over %s: %v", shape.Name, err)
	}
	scenario.QueryPlan = plan
	scenario.Unbounded = probeGraphBenchmarkUnbounded(ctx, fixture, shape, root)
	scenario.Stability.ReseedReproduction = reseedGraphBenchmarkStability(t, fixture, service, ctx, shape, scenario.Stability)

	if !scenario.Unbounded.MatchesExpectation || scenario.Unbounded.Failed != "" || scenario.Unbounded.TimedOut {
		assertions.SeededShapeMatchesTraversal = false
	}
	if scenario.ReturnedNodes > GraphMaxNodes || scenario.ReturnedEdges > GraphMaxEdges {
		assertions.BoundedOutput = false
	}
	if !scenario.Stability.Stable {
		assertions.StableResults = false
	}
	atBound := shape.AvailableNodes == GraphMaxNodes && shape.AvailableEdges == GraphMaxEdges
	aboveBound := shape.AvailableNodes > GraphMaxNodes || shape.AvailableEdges > GraphMaxEdges
	if atBound && scenario.Truncated {
		assertions.NoTruncationAtTheBound = false
	}
	if aboveBound && !scenario.Truncated {
		assertions.TruncationReportedAbove = false
	}
	return scenario, true
}

// reseedGraphBenchmarkStability deletes one scenario and builds it again, then
// compares the answers, and returns a sentence saying what happened.
//
// The point is that the fingerprints are compared across two databases' worth of
// rows rather than across two reads of one. source_dependencies.id is a random
// uuid and the neighborhood query orders candidate edges by that id, so the
// greedy community loop sees its edges in a different order in the rebuilt graph.
// Whether the partition survives that is a property of the schema and the
// algorithm, and the run records it instead of deciding it.
func reseedGraphBenchmarkStability(t *testing.T, fixture *testsupport.Fixture, service *Service, ctx context.Context, shape graphBenchmarkShape, before graphBenchmarkStability) string {
	t.Helper()
	dropGraphBenchmarkShape(t, fixture, shape)
	root := seedGraphBenchmarkShape(t, fixture, shape)
	after, err := service.GraphSourceDependencyCommunities(ctx, GraphSourceDependencyCommunitiesInput{SourceID: root.String(), MaxDepth: graphBenchmarkMaxDepth}, "")
	if err != nil {
		t.Fatalf("read the rebuilt graph for the reseed check over %s: %v", shape.Name, err)
	}
	neighborhood, err := service.GraphSourceDependencyNeighborhood(ctx, GraphSourceDependencyNeighborhoodInput{SourceID: root.String(), MaxDepth: graphBenchmarkMaxDepth}, "")
	if err != nil {
		t.Fatalf("read the rebuilt neighborhood for the reseed check over %s: %v", shape.Name, err)
	}
	outcome := "the rebuilt graph returned the same edge-set fingerprint and the same community partition"
	if neighborhood.Summary.EdgeSetFingerprint != before.EdgeSetFingerprint {
		outcome = "the rebuilt graph returned a DIFFERENT edge-set fingerprint while the same nodes and edges: source_dependencies.id is a random uuid and the query orders candidate edges by it"
	}
	if after.Summary.PartitionFingerprint != before.PartitionFingerprint {
		outcome += "; the community partition ALSO differed, so the greedy loop's tie-breaking depends on that random edge order"
	}
	return outcome
}

// graphBenchmarkCommit records the tree the measurement came from, so a number
// in the decision document can be traced to the code that produced it. It is
// best-effort: a tree with no git metadata is not a reason to fail a
// measurement.
func graphBenchmarkCommit() string {
	command := exec.CommandContext(context.Background(), "git", "rev-parse", "--short", "HEAD")
	command.Dir = "."
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
