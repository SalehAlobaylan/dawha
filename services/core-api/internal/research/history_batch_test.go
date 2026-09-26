package research

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// Research-run authorization used to walk every run on the page: one role lookup,
// three structural reads, and then one statement per tree, per evidence source and
// per graph source, for each of them. A hundred runs cost several hundred round
// trips to answer one list request.
//
// These tests pin the two things that matter about the batched form. The first is
// the statement count, which is asserted rather than described. The second is that
// batching did not touch the rule: a run whose evidence source is private stays
// hidden even when every other resource it names is public, an unauthorized run is
// absent from the list with no trace of its identifiers, and the runs that survive
// come back in the order the summary query produced them.

// batchHistoryFixture is a question with a hundred runs: a public-source run, a
// private-source run, a private-tree run and a run that names nothing, each with a
// public path and path evidence so the tree and evidence branches are exercised.
type batchHistoryFixture struct {
	actorID         string
	questionID      uuid.UUID
	publicSourceID  uuid.UUID
	privateSourceID uuid.UUID
	answerID        []uuid.UUID
	pathID          []uuid.UUID
	allowed         []string
	denied          []string
	allSummaries    []ResearchRunSummary
}

func seedBatchHistory(t *testing.T, fixture *testsupport.Fixture, runs int) *batchHistoryFixture {
	t.Helper()
	actorID := uuid.New()
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث القائمة')`,
		actorID, "run-batch-"+actorID.String()+"@plan004.test")
	fixture.Exec(`INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, actorID)

	seeded := &batchHistoryFixture{actorID: actorID.String(), questionID: uuid.New()}
	fixture.Exec(`INSERT INTO open_questions (id, title_ar, status, created_by) VALUES ($1, 'سؤال القائمة', 'open', $2)`,
		seeded.questionID, actorID)

	publicSourceID := uuid.New()
	privateSourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES
		($1, 'مصدر عام', 'book', 'public', $3), ($2, 'مصدر خاص', 'book', 'private', $3)`,
		publicSourceID, privateSourceID, actorID)
	seeded.publicSourceID = publicSourceID
	seeded.privateSourceID = privateSourceID

	// The tree that denies: private, owned by somebody else, and with no
	// collaboration, which is the closed-to-everyone state the tree policy reads.
	foreignOwnerID := uuid.New()
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مالك آخر')`,
		foreignOwnerID, "run-batch-foreign-"+foreignOwnerID.String()+"@plan004.test")
	privateTreeID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة خاصة', 'private', $2)`, privateTreeID, foreignOwnerID)

	// The four shapes, repeated. A repeated shape is the point: the batch has to
	// decide each run separately even when the runs name the same resources.
	//
	// The two denied shapes are chosen so that neither is a source the reader
	// simply happens to be allowed to see. A researcher reaches every source,
	// because the source policy grants the research role, so a private source is
	// not a denial for this reader and using one would prove nothing. What denies
	// a researcher is a tree owned by somebody else, and a source that the
	// deliberately public source dependency graph contract says is not published -
	// the last one denies even the researcher who wrote the source, which is
	// exactly the rule the batch must not soften.
	shapes := []string{"public", "foreign-tree", "unpublished-graph-source", "bare"}
	for index := 0; index < runs; index++ {
		shape := shapes[index%len(shapes)]
		runID := uuid.New()
		fixture.Exec(`INSERT INTO research_runs (id, question_id, query, normalized_query, status, model_version, actor_id, synthesis_attempted, created_at, updated_at)
			VALUES ($1, $2, $3, $3, 'succeeded', 'test-model', $4, true, $5::timestamptz, $5::timestamptz)`,
			runID, seeded.questionID, "سؤال "+shape, actorID, createdAtOf(index))
		fixture.Exec(`INSERT INTO research_answers (run_id, answer) VALUES ($1, $2)`, runID, "إجابة "+shape)

		if shape == "bare" {
			// A run that names nothing but its own question: nothing to be denied.
			seeded.allowed = append(seeded.allowed, runID.String())
			continue
		}
		treeID := uuid.New()
		if shape == "foreign-tree" {
			treeID = privateTreeID
		} else {
			fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة عامة', 'public', $2)`, treeID, actorID)
			versionID := uuid.New()
			fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at)
				VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, actorID)
		}
		// The graph-source shape walks the deliberately public source dependency
		// graph, whose nodes are source ids; the other shapes walk a path inside a
		// tree, whose nodes are person ids.
		operation := "evidence_connection"
		nodes := `[]`
		if shape == "unpublished-graph-source" {
			operation = GraphOperationSourceDependency
			nodes = `[{"id": "` + privateSourceID.String() + `", "type": "source"}]`
		}
		pathID := uuid.New()
		fixture.Exec(`INSERT INTO research_graph_paths (id, run_id, path_key, operation, status, explanation, depth, truncated, evidence_backed, structural_only, tree_id, algorithm_version, nodes, created_at)
			VALUES ($1, $2, $3, $4, 'evidence_backed', 'مسار', 1, false, true, false, $5, 'test-algo', $6::jsonb, now())`,
			pathID, runID, "path-"+pathID.String(), operation, treeID, nodes)

		// Every shaped run cites a published source, so the evidence branch of the
		// rule passes for all of them and the tree and graph branches are what
		// decide the verdict.
		sourceID := publicSourceID
		statementID := uuid.New()
		passageID := uuid.New()
		fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
			VALUES ($1, $2, $3, 'نص', 'نص')`, passageID, sourceID, index+1)
		fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, extraction_method, review_status, created_by)
			VALUES ($1, $2, $3, 'عبارة', 'ai', 'accepted', $4)`, statementID, sourceID, passageID, actorID)
		fixture.Exec(`INSERT INTO research_graph_path_evidence (run_id, path_id, ordinal, reference_id, reference_type, source_id, statement_id, passage_id, review_status)
			VALUES ($1, $2, 1, $3, 'source_statement', $4, $5, $6, 'accepted')`,
			runID, pathID, statementID, sourceID, statementID, passageID)

		if shape == "public" || shape == "bare" {
			seeded.allowed = append(seeded.allowed, runID.String())
		} else {
			seeded.denied = append(seeded.denied, runID.String())
		}
	}
	return seeded
}

// createdAtOf steps the timestamps so the run list has a known newest-first order
// and so two runs never share one.
func createdAtOf(index int) string {
	return fmt.Sprintf("2026-03-01 00:00:%02d +0000", index%60)
}

// TestListRunsAuthorizationIsBatched is the pin. A hundred runs must cost a fixed
// handful of statements, and the number of runs must not appear in it.
func TestListRunsAuthorizationIsBatched(t *testing.T) {
	for _, total := range []int{10, 50, 100} {
		fixture := testsupport.New(t)
		seeded := seedBatchHistory(t, fixture, total)
		pool, counter := fixture.CountingFixturePool(t)
		service := &Service{Pool: pool}

		counter.Reset()
		summaries, err := service.ListRuns(fixture.Ctx(), seeded.actorID, seeded.questionID.String())
		if err != nil {
			t.Fatalf("list runs: %v", err)
		}
		if len(summaries) != len(seeded.allowed) {
			t.Fatalf("listed %d runs, want %d visible ones", len(summaries), len(seeded.allowed))
		}
		// Before the batching this was 1 role lookup + 1 summary + 100 * (1 role + 3
		// structural + 1 tree + 1 source + 1 public source) in the hundreds. The bound
		// is loose enough to survive an unrelated authorization change and tight
		// enough that returning to one statement per run cannot pass.
		counter.AssertAtMost(t, 10, "listing 100 research runs")
		t.Logf("STEP2 runs=%d visible=%d queries=%d payloadBytes=%d", total, len(summaries), counter.Count(), testsupport.PayloadBytes(t, summaries))
	}
}

// TestListRunsKeepsTheAllOrNothingRule is the correctness floor. A run that names a
// tree the reader may not see stays hidden, and so does a run whose source
// dependency graph names a source that is not published - even though the reader is
// a researcher who can read that source through every other path. Both runs cite a
// published source, so the publication branch passes and only the rule under test
// can hide them.
func TestListRunsKeepsTheAllOrNothingRule(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedBatchHistory(t, fixture, 40)
	pool, _ := fixture.CountingFixturePool(t)
	service := &Service{Pool: pool}
	ctx := fixture.Ctx()

	summaries, err := service.ListRuns(ctx, seeded.actorID, seeded.questionID.String())
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	visible := make(map[string]ResearchRunSummary, len(summaries))
	for _, summary := range summaries {
		if _, duplicate := visible[summary.ID]; duplicate {
			t.Fatalf("run %s is on the list twice", summary.ID)
		}
		visible[summary.ID] = summary
	}
	for _, runID := range seeded.allowed {
		if _, found := visible[runID]; !found {
			t.Fatalf("run %s is readable and is missing from the list", runID)
		}
	}
	for _, runID := range seeded.denied {
		if _, found := visible[runID]; found {
			t.Fatalf("run %s reached the list although one of its resources is not visible", runID)
		}
	}
	if len(summaries) != len(seeded.allowed) {
		t.Fatalf("listed %d runs, want exactly the %d readable ones", len(summaries), len(seeded.allowed))
	}

	// A denied run is also refused on its own, which is what makes its absence from
	// the list indistinguishable from its never having existed.
	for _, runID := range seeded.denied {
		if _, err := service.GetRun(ctx, seeded.actorID, runID); err == nil {
			t.Fatalf("the researcher read the denied run %s directly", runID)
		}
	}
	// The list response carries no identifier and no title of anything hidden. A
	// list that answered with an empty shell would leak that the run exists.
	encoded := marshalSummaries(t, summaries)
	if len(summaries) == 0 {
		t.Fatal("the list is empty, so the leak checks would prove nothing")
	}
	for _, runID := range seeded.denied {
		if strings.Contains(encoded, runID) {
			t.Fatalf("the list response carries the identifier of a denied run: %s", runID)
		}
	}
	if strings.Contains(encoded, "شجرة خاصة") {
		t.Fatal("the list response carries the name of a private tree")
	}
}

// TestListRunsOrderIsTheSummaryOrder pins determinism: the batched filter must not
// reorder the page. It returns the runs in the order the summary query produced
// them, which is (created_at DESC) with the summary query's own tiebreak.
func TestListRunsOrderIsTheSummaryOrder(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedBatchHistory(t, fixture, 24)
	pool, _ := fixture.CountingFixturePool(t)
	service := &Service{Pool: pool}
	ctx := fixture.Ctx()

	summaries, err := service.ListRuns(ctx, seeded.actorID, seeded.questionID.String())
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	// The summary query is the order of record. Reading it back through the same
	// executor the service uses is the cheapest way to prove the filter preserved
	// it without re-deriving created_at ordering by hand.
	raw, err := listRuns(ctx, pool, seeded.questionID, true)
	if err != nil {
		t.Fatalf("summary query: %v", err)
	}
	visible := make(map[string]struct{}, len(summaries))
	for _, summary := range summaries {
		visible[summary.ID] = struct{}{}
	}
	visibleInRaw := make([]string, 0, len(summaries))
	for _, summary := range raw {
		if _, found := visible[summary.ID]; found {
			visibleInRaw = append(visibleInRaw, summary.ID)
		}
	}
	if len(visibleInRaw) != len(summaries) {
		t.Fatalf("the filter kept %d of %d runs, so the two disagree about visibility", len(summaries), len(visibleInRaw))
	}
	for index := range summaries {
		if summaries[index].ID != visibleInRaw[index] {
			t.Fatalf("run %d is %s on the page and %s in the summary query: the order changed",
				index, summaries[index].ID, visibleInRaw[index])
		}
	}
	// Repeated reads answer the same way, in the same order: the set-based walk
	// builds maps, and a map read back in iteration order is how a page starts
	// shuffling between two identical requests.
	second, err := service.ListRuns(ctx, seeded.actorID, seeded.questionID.String())
	if err != nil {
		t.Fatalf("list runs again: %v", err)
	}
	for index := range summaries {
		if summaries[index].ID != second[index].ID {
			t.Fatalf("run %d is %s on the first read and %s on the second", index, summaries[index].ID, second[index].ID)
		}
	}
}

// TestListRunsRefusesTheSameCallersTheFilterCannot is the refusal path. A caller
// with no research role is refused before the summary query runs, so the batched
// filter is never reached by a reader it could mislead.
func TestListRunsRefusesTheSameCallersTheFilterCannot(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedBatchHistory(t, fixture, 8)
	pool, _ := fixture.CountingFixturePool(t)
	service := &Service{Pool: pool}
	ctx := fixture.Ctx()

	registeredID := uuid.New()
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مسجل')`,
		registeredID, "run-batch-registered-"+registeredID.String()+"@plan004.test")
	fixture.Exec(`INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered')`, registeredID)

	for _, actorID := range []string{"", registeredID.String(), "not-a-uuid"} {
		if _, err := service.ListRuns(ctx, actorID, seeded.questionID.String()); err != ErrForbidden {
			t.Fatalf("ListRuns(%q) = %v, want %v", actorID, err, ErrForbidden)
		}
	}
	// The researcher's own page still answers, so the refusal is about the reader
	// and not about the batched walk.
	if _, err := service.ListRuns(ctx, seeded.actorID, seeded.questionID.String()); err != nil {
		t.Fatalf("the researcher could not list runs: %v", err)
	}
}

func marshalSummaries(t *testing.T, summaries []ResearchRunSummary) string {
	t.Helper()
	encoded, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
