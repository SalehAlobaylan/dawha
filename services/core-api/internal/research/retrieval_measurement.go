package research

// The retrieval measurement: three arms over one corpus and one labelled
// question set, differing only in the retrieval path.
//
// # Why this file exists
//
// IMPLEMENTATION_PLAN.md:1458 sets Phase 20's acceptance criterion - "multi-hop
// research questions improve over vector-only RAG" - and nothing in this
// repository could test it. The reason docs/phase-status.md recorded was that
// there is "no embedding retrieval in the query path" and that closing the
// blocker needed "a labelled question set *and a second retrieval path*". The
// first half of that was wrong: `retrieveVectorPassages` has existed all along
// and `rag_service.go` has always called both legs. The second half was right
// about the data and wrong about the cause: `source_passages` held three rows and
// none had an embedding, because the seed is pure SQL.
//
// So this file supplies the three things that were missing: a corpus with
// embeddings (db/seeds/002_retrieval_corpus.sql plus internal/embeddingbackfill),
// a labelled question set (testdata/retrieval_measurement_cases.jsonl), and the
// three arms to run over them.
//
// # What it does not do
//
// It does not change retrieval. The ranking, the fusion weights, the rerank call
// and every threshold are the shipped ones, called through the same functions the
// service calls. A leg that is weak shows up here as a weak number, which is the
// point: a measurement that could be improved by editing the thing it measures is
// not a measurement.
//
// # The arms
//
//   - vector-only     `retrieveVectorPassages` alone, ordered by vector score.
//   - hybrid          lexical + vector + `fuseAndRerank`, which is the shipped
//                     path in `execute`.
//   - graph-augmented the hybrid, plus the graph citations `execute` appends.
//
// The graph arm reproduces `execute`'s ordering rather than a better one, and
// that is deliberate. `graphPassageCitations` appends graph citations AFTER the
// fused list and assigns them `Rerank: 0.5, Combined: 0.5` without re-sorting, so
// in the shipped product a graph passage can only reach the top five when the
// hybrid leg returned fewer than five candidates. An arm that merged graph
// evidence into the ranking would be measuring an improvement nobody shipped.
//
// # The four files' separation of duties
//
//   retrieval_measurement_cases.jsonl  the labels. Durable; versioned by
//                                      RetrievalMeasurementVersion below.
//   db/seeds/002_retrieval_corpus.sql  the corpus. Durable.
//   this file                         the harness. Disposable.
//   docs/retrieval-measurement.md      the report, regenerated from the run.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// RetrievalMeasurementVersion versions the corpus and its labels together.
	//
	// It is on the report and in the file's own assertions for the same reason
	// evaluation/thresholds.py versions its thresholds: two reports are only
	// comparable when the thing they measured is the same thing, and a corpus
	// that changed silently produces two reports that look comparable and are
	// not. Bump it when a passage, a label, or a judged set changes.
	RetrievalMeasurementVersion = "plan-014-retrieval-v1"

	// retrievalMeasurementEnv is the opt-in, for the same reason
	// DAWHA_GRAPH_BENCH is: this writes a report and measures a database, and
	// neither belongs in the acceptance gate.
	retrievalMeasurementEnv = "DAWHA_RETRIEVAL_BENCH"
	// retrievalMeasurementReportEnv is where the machine-readable report lands.
	retrievalMeasurementReportEnv = "DAWHA_RETRIEVAL_BENCH_REPORT"
	// retrievalMeasurementAIEnv names the AI service. Unlike the graph
	// benchmark this command REQUIRES it: the two legs under measurement are an
	// embedding call and a rerank call, and a stub in their place would measure
	// the stub.
	retrievalMeasurementAIEnv = "AI_RESEARCH_URL"

	defaultRetrievalMeasurementReport = "docs/benchmarks/retrieval-arms.json"

	// retrievalMeasurementSchema is the isolated schema the corpus is provisioned
	// into, and the reason the corpus is not a seed.
	//
	// `make db-seed` applies everything in db/seeds/, so a measurement corpus
	// living there lands in the developer's demo database, in the E2E stack and
	// in `make verify-full` - and it did, until it broke
	// apps/web/e2e/journeys/09-research-query.spec.ts, whose "a query about
	// nothing in the corpus" query shares a pg_trgm character trigram with three
	// of the forty-two new passages. The fix is not a shorter query in that
	// journey, it is that measurement data is not the product's demo data.
	//
	// So the corpus is applied into a schema of its own, measured there, and the
	// schema is dropped on the way out. That is the same shape as the graph
	// benchmark's `p004_fixture_*` schemas, and it gives three properties the
	// command would not otherwise have: the run adds nothing to the database it is
	// pointed at, a crashed run cannot leave half a corpus behind (the next run
	// drops the schema before creating it), and the measurement is hermetic - the
	// same numbers on a freshly migrated database as on a seeded one.
	retrievalMeasurementSchema = "dawha_retrieval_measurement"

	// retrievalCorpusFile is the corpus, next to the labels that judge it.
	retrievalCorpusFile = "testdata/retrieval_corpus.sql"

	// retrievalKeepCorpusEnv leaves the provisioned schema in place. For reading
	// the corpus in psql after a run, and nothing else.
	retrievalKeepCorpusEnv = "DAWHA_RETRIEVAL_KEEP_CORPUS"

	// retrievalDepth is the metric depth every arm is scored at. Five because
	// that is the number a reader of a research result actually sees, and because
	// `execute` hands the model at most twenty contexts and the answer cites at
	// most five.
	retrievalDepth = 5
)

// Case classes. The split is the measurement, not bookkeeping: Phase 20's
// criterion is about multi-hop questions, so a corpus where the multi-hop subset
// behaves like the average settles nothing.
const (
	retrievalClassMultiHop       = "multi_hop"
	retrievalClassSingleHop      = "single_hop"
	retrievalClassLexicalControl = "lexical_control"
)

var retrievalClasses = []string{retrievalClassMultiHop, retrievalClassSingleHop, retrievalClassLexicalControl}

// retrievalCase is one labelled question.
type retrievalCase struct {
	ID         string   `json:"id"`
	Class      string   `json:"class"`
	Hops       int      `json:"hops"`
	HopsAR     string   `json:"hops_ar"`
	ReviewedBy string   `json:"reviewed_by"`
	Reason     string   `json:"reason"`
	Query      string   `json:"query"`
	Relevant   []string `json:"relevant"`
	// Graph is the traversal this question implies, if any. A case with no graph
	// is scored on the two passage arms alone, and saying so is part of the
	// report rather than an omission from it.
	Graph *retrievalCaseGraph `json:"graph,omitempty"`
}

// retrievalCaseGraph is the traversal a multi-hop question needs.
type retrievalCaseGraph struct {
	Operation string `json:"operation"`
	StartType string `json:"start_type"`
	StartID   string `json:"start_id"`
	EndType   string `json:"end_type,omitempty"`
	EndID     string `json:"end_id,omitempty"`
	MaxDepth  int    `json:"max_depth,omitempty"`
}

// loadRetrievalCases reads and validates the labelled question set.
//
// Every check here is one a reviewer would otherwise have to do by hand, and each
// one is a way the corpus and the labels can drift apart silently: a case with no
// judged passage would score as a miss for every arm, a case with no reviewer
// would be an unlabelled question dressed as a labelled one, and an unknown class
// would fall out of every subset and quietly shrink the multi-hop count.
func loadRetrievalCases(t *testing.T) ([]retrievalCase, string) {
	t.Helper()
	path := filepath.Join("testdata", "retrieval_measurement_cases.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	digest := sha256.Sum256(raw)

	lines := strings.Split(string(raw), "\n")
	cases := make([]retrievalCase, 0, len(lines))
	seen := map[string]struct{}{}
	for number, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var item retrievalCase
		if err := json.Unmarshal([]byte(trimmed), &item); err != nil {
			t.Fatalf("%s line %d does not parse: %v", path, number+1, err)
		}
		switch {
		case item.ID == "":
			t.Fatalf("%s line %d has no id", path, number+1)
		case item.Query == "":
			t.Fatalf("%s line %d (%s) has no query", path, number+1, item.ID)
		case len(item.Relevant) == 0:
			t.Fatalf("%s line %d (%s) has an empty judged set, so every arm would score it as a miss for the same reason", path, number+1, item.ID)
		case item.ReviewedBy == "":
			t.Fatalf("%s line %d (%s) has no reviewed_by, so it is an unlabelled question wearing a label", path, number+1, item.ID)
		case item.Reason == "":
			t.Fatalf("%s line %d (%s) has no reason, so nobody can tell later why the passage was judged relevant", path, number+1, item.ID)
		}
		if _, found := seen[item.ID]; found {
			t.Fatalf("%s line %d repeats the id %s", path, number+1, item.ID)
		}
		seen[item.ID] = struct{}{}
		known := false
		for _, class := range retrievalClasses {
			if item.Class == class {
				known = true
			}
		}
		if !known {
			t.Fatalf("%s line %d (%s) has the class %q, which is not one of %v; an unknown class falls out of every subset", path, number+1, item.ID, item.Class, retrievalClasses)
		}
		if item.Class == retrievalClassMultiHop && item.Hops < 2 {
			t.Fatalf("%s line %d (%s) is classed multi_hop with %d hops; the criterion is about questions that need two or more", path, number+1, item.ID, item.Hops)
		}
		if item.Graph != nil {
			if _, ok := graphOperations[item.Graph.Operation]; !ok {
				t.Fatalf("%s line %d (%s) names the graph operation %q, which the service does not implement", path, number+1, item.ID, item.Graph.Operation)
			}
			if item.Graph.StartID == "" || item.Graph.StartType == "" {
				t.Fatalf("%s line %d (%s) has a graph hop with no start", path, number+1, item.ID)
			}
		}
		duplicated := map[string]struct{}{}
		for _, id := range item.Relevant {
			if _, found := duplicated[id]; found {
				t.Fatalf("%s line %d (%s) lists the passage %s twice", path, number+1, item.ID, id)
			}
			duplicated[id] = struct{}{}
			if _, err := uuid.Parse(id); err != nil {
				t.Fatalf("%s line %d (%s) judges the passage %q, which is not an id", path, number+1, item.ID, id)
			}
		}
		cases = append(cases, item)
	}
	if len(cases) < 20 {
		t.Fatalf("%s holds %d cases; the plan asks for at least 20, and a smaller set cannot show a difference if one exists", path, len(cases))
	}
	multiHop := 0
	for _, item := range cases {
		if item.Class == retrievalClassMultiHop {
			multiHop++
		}
	}
	if multiHop == 0 {
		t.Fatal("the question set holds no multi-hop case, so it cannot speak to the criterion it exists to test")
	}
	return cases, hex.EncodeToString(digest[:])
}

// retrievalArm is one retrieval path over one question.
type retrievalArm string

const (
	retrievalArmVector   retrievalArm = "vector_only"
	retrievalArmHybrid   retrievalArm = "hybrid"
	retrievalArmGraph    retrievalArm = "graph_augmented"
	retrievalArmReversed retrievalArm = "reversed"
)

// retrievalArms are the four runs. The fourth is not an arm anybody would ship;
// it exists so the harness can be shown to notice a bad ranking. See
// TestRetrievalHarnessIsNotVacuous.
var retrievalArms = []retrievalArm{retrievalArmVector, retrievalArmHybrid, retrievalArmGraph, retrievalArmReversed}

// retrievalMetrics is one arm's numbers over one subset of the questions.
type retrievalMetrics struct {
	Cases            int       `json:"cases"`
	Answered         int       `json:"answered"`
	Empty            int       `json:"returned_nothing"`
	RecallAt5        float64   `json:"recall_at_5"`
	MRR              float64   `json:"mrr"`
	RecallAt5Samples []float64 `json:"recall_at_5_per_case"`
	// EmptyCases names the questions an arm returned nothing for, because "three
	// questions were empty" is not actionable and the three are.
	EmptyCases []string `json:"returned_nothing_cases,omitempty"`
	// GraphPassagesAdded is how many passages the graph leg contributed that the
	// hybrid did not already have. It is the number that says whether the third
	// arm did anything at all.
	GraphPassagesAdded int `json:"graph_passages_added"`
}

// retrievalArmResult is one arm's per-question outcome, so a reader can see which
// question moved rather than only the average.
type retrievalArmResult struct {
	CaseID   string   `json:"case_id"`
	Class    string   `json:"class"`
	Returned []string `json:"returned_passages"`
	Relevant []string `json:"relevant_passages"`
	Recall   float64  `json:"recall_at_5"`
	Rank     int      `json:"first_relevant_rank"`
	// GraphAdded names the passages the graph leg contributed, and GraphRelevant
	// how many of them were judged relevant. An arm that adds passages and none
	// of them answer the question is a finding, not a rounding error.
	GraphAdded    []string `json:"graph_passages_added,omitempty"`
	GraphRelevant int      `json:"graph_relevant"`
	GraphStatus   string   `json:"graph_status,omitempty"`
	GraphNote     string   `json:"graph_note,omitempty"`
}

// retrievalReport is the whole run.
type retrievalReport struct {
	GeneratedAt string                              `json:"generated_at"`
	Command     string                              `json:"command"`
	Commit      string                              `json:"commit,omitempty"`
	Corpus      retrievalCorpusReport               `json:"corpus"`
	Measurement retrievalMeasurementShape           `json:"measurement"`
	Arms        map[retrievalArm]retrievalArmReport `json:"arms"`
	Comparison  retrievalComparison                 `json:"comparison"`
	Decision    retrievalDecision                   `json:"decision"`
	Provider    retrievalProviderReport             `json:"provider"`
	// Corrupt is set when an arm answered nothing, so a reader is told the number
	// beside it rather than having to notice.
	Corrupt []string `json:"corrupt_arms,omitempty"`
	// Warnings are things the run noticed about itself.
	Warnings []string `json:"warnings,omitempty"`
}

type retrievalCorpusReport struct {
	Version         string   `json:"version"`
	CasesFile       string   `json:"cases_file"`
	CasesSHA256     string   `json:"cases_sha256"`
	Passages        int      `json:"passages"`
	Retrievable     int      `json:"retrievable_passages"`
	Embedded        int      `json:"embedded_passages"`
	Sources         int      `json:"sources"`
	DistinctJudged  int      `json:"distinct_judged_passages"`
	Cases           int      `json:"cases"`
	MultiHop        int      `json:"multi_hop_cases"`
	SingleHop       int      `json:"single_hop_cases"`
	LexicalControls int      `json:"lexical_control_cases"`
	ReviewedBy      []string `json:"reviewed_by"`
	AuthorCaveat    string   `json:"author_caveat"`
	JudgedMissing   []string `json:"judged_passages_not_in_corpus,omitempty"`
	Unretrievable   []string `json:"judged_passages_with_no_accepted_statement,omitempty"`
}

// retrievalMeasurementShape states what a difference would have to beat.
type retrievalMeasurementShape struct {
	Depth           int     `json:"depth"`
	MultiHopCases   int     `json:"multi_hop_cases"`
	VarianceMetric  string  `json:"variance_metric"`
	VarianceValue   float64 `json:"variance_value"`
	VarianceMeaning string  `json:"variance_meaning"`
	Method          string  `json:"method"`
}

type retrievalArmReport struct {
	All       retrievalMetrics     `json:"all"`
	MultiHop  retrievalMetrics     `json:"multi_hop"`
	SingleHop retrievalMetrics     `json:"single_hop"`
	Lexical   retrievalMetrics     `json:"lexical_control"`
	PerCase   []retrievalArmResult `json:"per_case"`
}

// retrievalComparison is the difference that matters, per subset.
type retrievalComparison struct {
	Basis string                   `json:"basis"`
	Rows  []retrievalComparisonRow `json:"rows"`
	Note  string                   `json:"note"`
}

type retrievalComparisonRow struct {
	Subset              string       `json:"subset"`
	Arm                 retrievalArm `json:"arm"`
	RecallAt5           float64      `json:"recall_at_5"`
	MRR                 float64      `json:"mrr"`
	DeltaRecallVsVector float64      `json:"delta_recall_vs_vector_only"`
	ExceedsVariance     bool         `json:"delta_exceeds_stated_variance"`
}

type retrievalDecision struct {
	Phase20Criterion string `json:"phase_20_criterion"`
	Outcome          string `json:"outcome"`
	Statement        string `json:"statement"`
	Corpus           string `json:"corpus_and_reviewer"`
	Variance         string `json:"variance"`
	SmallestCorpus   string `json:"smallest_corpus_that_would_conclude"`
}

type retrievalProviderReport struct {
	Model            string `json:"model"`
	Dimensions       int    `json:"dimensions"`
	Semantic         bool   `json:"semantic"`
	SemanticEvidence string `json:"semantic_evidence"`
}

// retrievalQueryOutcome is one arm's answer for one question.
type retrievalQueryOutcome struct {
	Passages    []Citation
	GraphAdded  []string
	GraphStatus string
	GraphNote   string
}

// runRetrievalArm scores one question with one arm.
//
// The three arms share this function so the only difference between them is which
// list is returned - that is what makes "the arms differ only in the retrieval
// path" a statement about the code rather than an intention.
func (s *Service) runRetrievalArm(ctx context.Context, arm retrievalArm, input QueryInput, normalized string, vector []float32) (retrievalQueryOutcome, error) {
	retrieval := retrievalContext{Input: input, Normalized: normalized, Vector: vector, QueryType: "general"}
	outcome := retrievalQueryOutcome{}

	switch arm {
	case retrievalArmVector:
		passages, err := s.retrieveVectorPassages(ctx, retrieval)
		if err != nil {
			return outcome, err
		}
		outcome.Passages = passages
		return outcome, nil

	case retrievalArmHybrid, retrievalArmReversed:
		lexical, err := s.retrieveLexicalPassages(ctx, retrieval)
		if err != nil {
			return outcome, err
		}
		byVector, err := s.retrieveVectorPassages(ctx, retrieval)
		if err != nil {
			return outcome, err
		}
		fused, _, err := s.fuseAndRerank(ctx, retrieval, lexical, byVector)
		if err != nil {
			return outcome, err
		}
		outcome.Passages = fused
		if arm == retrievalArmReversed {
			// The broken arm, and the only place in this file that reorders
			// anything. It is here so the harness can be shown to notice: a harness
			// that reports the same number for a good ranking and a reversed one is
			// measuring nothing, and every other number in the report would then be
			// as untrustworthy. It is not a proposal.
			reversed := make([]Citation, len(outcome.Passages))
			for index, citation := range outcome.Passages {
				reversed[len(outcome.Passages)-1-index] = citation
			}
			for index := range reversed {
				reversed[index].Rank = index + 1
			}
			outcome.Passages = reversed
		}
		return outcome, nil

	case retrievalArmGraph:
		lexical, err := s.retrieveLexicalPassages(ctx, retrieval)
		if err != nil {
			return outcome, err
		}
		byVector, err := s.retrieveVectorPassages(ctx, retrieval)
		if err != nil {
			return outcome, err
		}
		fused, _, err := s.fuseAndRerank(ctx, retrieval, lexical, byVector)
		if err != nil {
			return outcome, err
		}
		// A case that names no traversal is scored on the hybrid alone, because
		// that is what the product does with it: `execute` only calls
		// `retrieveGraph` when `input.GraphOperation` is set. Returning an empty
		// list here instead would have made the graph arm look catastrophic on
		// every single-hop question, which is a measurement bug and not a finding.
		outcome.Passages = fused
		if input.GraphOperation == "" {
			outcome.GraphStatus = "no traversal on this case, so this arm is the hybrid arm"
			return outcome, nil
		}
		graphResult, graphErr := s.retrieveGraph(ctx, input, "")
		if graphErr != nil {
			return outcome, graphErr
		}
		added := graphPassageCitations(graphResult.Paths, fused)
		outcome.GraphAdded = make([]string, 0, len(added))
		for _, citation := range added {
			if citation.PassageID != "" {
				outcome.GraphAdded = append(outcome.GraphAdded, citation.PassageID)
			} else if citation.StatementID != "" {
				outcome.GraphAdded = append(outcome.GraphAdded, citation.StatementID)
			}
		}
		outcome.Passages = append(fused, added...)
		outcome.GraphStatus = fmt.Sprintf("%s: %d path(s), %d evidence ref(s), %d citation(s) added",
			graphResult.Stats.Operation, graphResult.Stats.PathCount, graphResult.Stats.EvidenceCount, len(added))
		// Three different reasons a traversal can add nothing, and they are not
		// interchangeable, so the arm says which one it hit instead of reporting a
		// zero. Getting this wrong is how a broken graph leg gets reported as an
		// unhelpful one.
		outcome.GraphNote = retrievalGraphNote(graphResult, len(added), fused)
		return outcome, nil
	}
	return outcome, fmt.Errorf("unknown retrieval arm %q", arm)
}

// retrievalGraphNote names why a traversal contributed nothing, or that it did.
//
// The three cases are genuinely different and a reader acting on the number needs
// to know which one they are looking at:
//
//   - the traversal found no path at all, which usually means a filter upstream of
//     the query excluded every edge (the corpus has a claim whose status is
//     `unresolved`, and `evidence_connection` drops those);
//   - the traversal found paths whose evidence refs carry a layer but no passage
//     or statement id, which is the case for every `geographic_path` - the refs are
//     `geographic_event` rows, and `graphPassageCitations` skips a ref with
//     neither, so a migration can be shown but not cited;
//   - the traversal found citable passages and every one of them was already in
//     the hybrid's list. This is the structural one: the lexical leg returns every
//     passage sharing a trigram with the query, `fuseAndRerank` keeps thirty, and
//     `graphPassageCitations` appends only what the hybrid did not already have.
//     In the shipped ordering a graph passage can therefore reach the top five
//     only when the passage legs returned fewer than five candidates.
func retrievalGraphNote(result graphRetrievalResult, added int, fused []Citation) string {
	if added > 0 {
		return ""
	}
	if result.Stats.PathCount == 0 {
		return "the traversal returned no path: every edge between these endpoints is filtered out before the query runs, so the relation is documented in an accepted statement that retrieval can reach and traversal cannot"
	}
	citable := 0
	for _, path := range result.Paths {
		for _, evidence := range path.EvidenceRefs {
			if evidence.PassageID != "" || evidence.StatementID != "" {
				citable++
			}
		}
	}
	if citable == 0 {
		return fmt.Sprintf("the traversal returned %d path(s) whose evidence refs carry no passage or statement id (%s reports %d ref(s) of a non-citable layer), so the path can be shown to a reader but nothing in it can be cited as a passage", result.Stats.PathCount, result.Stats.Operation, result.Stats.EvidenceCount)
	}
	return fmt.Sprintf("the traversal cited %d citable passage(s) and all of them were already in the hybrid's %d results, so it added nothing: graph citations are appended after the fused list rather than merged into its ranking", citable, len(fused))
}

// scoreRetrieval scores one outcome against one case.
//
// recall@5 is the fraction of the judged set inside the top five, and MRR is the
// reciprocal rank of the first judged passage. Both are over the case's whole
// judged set rather than over one "the" answer, because three of these cases have
// two defensible answers and a metric that only accepted the first would be
// measuring the corpus's tidiness.
func scoreRetrieval(caseItem retrievalCase, outcome retrievalQueryOutcome) retrievalArmResult {
	result := retrievalArmResult{
		CaseID:      caseItem.ID,
		Class:       caseItem.Class,
		Returned:    make([]string, 0, retrievalDepth),
		Relevant:    caseItem.Relevant,
		GraphAdded:  outcome.GraphAdded,
		GraphStatus: outcome.GraphStatus,
		GraphNote:   outcome.GraphNote,
		Rank:        0,
	}
	relevant := make(map[string]struct{}, len(caseItem.Relevant))
	for _, id := range caseItem.Relevant {
		relevant[id] = struct{}{}
	}
	hit := 0
	for index, citation := range outcome.Passages {
		id := citation.PassageID
		if index < retrievalDepth {
			result.Returned = append(result.Returned, id)
			if _, ok := relevant[id]; ok {
				hit++
				if result.Rank == 0 {
					result.Rank = index + 1
				}
			}
		}
		if _, ok := relevant[id]; ok && index >= retrievalDepth {
			// A judged passage beyond the depth is not a hit and not a miss; the
			// report says so rather than pretending recall@5 saw it.
			continue
		}
	}
	if len(caseItem.Relevant) > 0 {
		result.Recall = math.Round((float64(hit)/float64(len(caseItem.Relevant)))*10000) / 10000
	}
	for _, id := range outcome.GraphAdded {
		if _, ok := relevant[id]; ok {
			result.GraphRelevant++
		}
	}
	return result
}

// retrievalVariance is the bar a measured difference has to clear.
//
// It is the Wilson half-width of each arm's recall over the multi-hop subset,
// added:
//
//	variance = w_a + w_b
//
// Wilson rather than the plain binomial standard error, and the reason is that
// the plain one collapses to exactly zero when an arm scores 1.0. An arm that
// answered every question correctly would face no bar at all, which is precisely
// backwards: a perfect score on seventeen questions is the measurement most in
// need of an uncertainty interval. The Wilson interval never degenerates - at
// p=1 over seventeen cases its half-width is 0.092 - so the bar stays a real
// number at both ends of the scale.
//
// The two arms are scored over the SAME questions, so a paired comparison would
// be tighter and this is deliberately not one: treating them as independent
// samples over-states the noise, and a criterion about multi-hop research
// questions should not be met on a difference a tighter statistic would have
// called significant and this one would not.
//
// At recalls near a half the two terms are equal, so the bar is 1/sqrt(n): 0.243
// at seventeen multi-hop cases, 0.100 at a hundred, 0.050 at four hundred. One
// case in seventeen is worth six points of recall on its own, which is why most
// differences on a corpus this size are inside the bar. That number is the honest
// reason a twenty-question internal corpus settles nothing, and it is printed
// beside the comparison rather than buried in a method note.
func retrievalVariance(metrics retrievalMetrics, other retrievalMetrics) float64 {
	return retrievalBinomialError(metrics.RecallAt5, metrics.Cases) + retrievalBinomialError(other.RecallAt5, other.Cases)
}

// retrievalWilsonZ is the two-sided 95% normal quantile.
const retrievalWilsonZ = 1.959964

// retrievalBinomialError is the Wilson half-width of a proportion observed over
// n cases: the half-length of the 95% score interval for p.
func retrievalBinomialError(recall float64, cases int) float64 {
	if cases == 0 {
		return 0
	}
	n := float64(cases)
	p := math.Min(math.Max(recall, 0), 1)
	z2 := retrievalWilsonZ * retrievalWilsonZ
	denominator := 1 + z2/n
	centre := (p + z2/(2*n)) / denominator
	margin := retrievalWilsonZ * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / denominator
	if centre+margin > 1 {
		margin = 1 - centre
	}
	if centre-margin < 0 {
		margin = centre
	}
	return margin
}

// retrievalRound keeps three decimals, which is finer than any difference this
// corpus can resolve and coarser than the raw float noise.
func retrievalRound(value float64) float64 {
	return math.Round(value*1000) / 1000
}

// aggregateRetrieval reduces per-case results to the metrics for one subset.
//
// MRR is the mean of the reciprocal rank of the first judged passage, and a
// question where the arm returned nothing scores 0 for both metrics rather than
// being dropped: a question the arm cannot answer is the worst outcome, and
// excluding it would let an arm improve its average by answering fewer questions.
func aggregateRetrieval(results []retrievalArmResult) retrievalMetrics {
	metrics := retrievalMetrics{Cases: len(results), EmptyCases: []string{}}
	if len(results) == 0 {
		return metrics
	}
	recallTotal, mrrTotal := 0.0, 0.0
	for _, result := range results {
		if len(result.Returned) == 0 {
			metrics.Empty++
			metrics.EmptyCases = append(metrics.EmptyCases, result.CaseID)
			recallTotal += result.Recall
			continue
		}
		metrics.Answered++
		recallTotal += result.Recall
		if result.Rank > 0 {
			mrrTotal += 1 / float64(result.Rank)
		}
		metrics.RecallAt5Samples = append(metrics.RecallAt5Samples, result.Recall)
		metrics.GraphPassagesAdded += len(result.GraphAdded)
	}
	count := float64(len(results))
	metrics.RecallAt5 = retrievalRound(recallTotal / count)
	metrics.MRR = retrievalRound(mrrTotal / count)
	sort.Strings(metrics.EmptyCases)
	return metrics
}

// byClass filters per-case results down to one class.
func byClass(results []retrievalArmResult, class string) []retrievalArmResult {
	filtered := make([]retrievalArmResult, 0, len(results))
	for _, result := range results {
		if result.Class == class {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// retrievalProviderSemantics measures whether the provider's embedding is
// semantic, and records the measurement rather than assuming it.
//
// This is not a side note. `DeterministicProvider.embed` is a SHA-512 digest of
// the normalized text spread across the requested dimensions, so two texts that
// mean the same thing have unrelated vectors unless they are the same string. A
// vector leg built on it can only rank a passage against a query whose normalized
// text it matches exactly, plus whatever subset of the corpus the hash happened
// to place at a positive cosine. The measurement below is the evidence for the
// report's central caveat, and it is cheap: embed one query and two passages and
// report the three cosines.
func retrievalProviderSemantics(ctx context.Context, provider ai.Provider, dimensions int, corpus []string) (retrievalProviderReport, error) {
	report := retrievalProviderReport{Dimensions: dimensions}
	query, err := retrievalEmbed(ctx, provider, "سؤال يقيس دلالة المتجه", dimensions)
	if err != nil {
		return report, err
	}
	self, err := retrievalEmbed(ctx, provider, "سؤال يقيس دلالة المتجه", dimensions)
	if err != nil {
		return report, err
	}
	report.Model = self.model
	same := retrievalCosine(query.vector, self.vector)
	others := make([]float64, 0, len(corpus))
	positive := 0
	for _, text := range corpus {
		embedded, embedErr := retrievalEmbed(ctx, provider, text, dimensions)
		if embedErr != nil {
			return report, embedErr
		}
		cosine := retrievalCosine(query.vector, embedded.vector)
		others = append(others, cosine)
		if cosine > 0 {
			positive++
		}
	}
	mean, min, max := 0.0, 0.0, 0.0
	if len(others) > 0 {
		mean, min, max = others[0], others[0], others[0]
		for _, value := range others {
			mean += value
			if value < min {
				min = value
			}
			if value > max {
				max = value
			}
		}
		mean /= float64(len(others))
	}
	// A semantic embedding puts related texts well above unrelated ones and keeps
	// the whole corpus on one side of zero far more often than not. A hash does
	// neither: it is orthogonal to everything and signs at random.
	semantic := same > 0.999 && mean > 0.15
	report.Semantic = semantic
	report.SemanticEvidence = fmt.Sprintf(
		"cosine(query, itself) = %.4f; cosine(query, %d other corpus texts) mean %.4f, min %.4f, max %.4f, and %d of %d of them came out positive",
		same, len(others), mean, min, max, positive, len(others))
	if len(others) > 0 && !semantic {
		report.SemanticEvidence += ". Those numbers are the signature of a hash, not of an embedding: a deterministic provider that hashes its input scores a query against a passage by coincidence"
	}
	return report, nil
}

type embeddedText struct {
	vector []float32
	model  string
}

func retrievalEmbed(ctx context.Context, provider ai.Provider, text string, dimensions int) (embeddedText, error) {
	response, err := provider.Embed(ctx, ai.EmbeddingRequest{Text: identity.NormalizeArabicName(text), Dimensions: dimensions})
	if err != nil {
		return embeddedText{}, err
	}
	if len(response.Embedding) != dimensions {
		return embeddedText{}, fmt.Errorf("the provider returned %d dimensions and %d were requested", len(response.Embedding), dimensions)
	}
	values := make([]float32, len(response.Embedding))
	for index, value := range response.Embedding {
		values[index] = float32(value)
	}
	return embeddedText{vector: values, model: response.Model}, nil
}

func retrievalCosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	dot, first, second := 0.0, 0.0, 0.0
	for index := range a {
		x, y := float64(a[index]), float64(b[index])
		dot += x * y
		first += x * x
		second += y * y
	}
	if first <= 0 || second <= 0 {
		return 0
	}
	return dot / (math.Sqrt(first) * math.Sqrt(second))
}

// provisionRetrievalCorpus builds the measurement schema - migrations, then the
// corpus - and returns a pool whose search_path resolves every unqualified table
// name to it.
//
// The pool is scoped, not the tables: the retrieval legs, the graph traversal and
// every helper in this file name tables without a schema, which is the right thing
// for production code and means the measurement cannot ask for a different schema
// than production does. Putting the measurement schema ahead of `public` on the
// search path is what makes it the corpus, and `public` stays on the path so
// pg_trgm's similarity(), pgvector's `<=>`, postgis's ST_MakePoint and the
// extension types resolve.
//
// The migrations are applied into the schema too, and that is not optional. The
// corpus file is INSERT statements against tables that have to exist somewhere;
// in an empty schema `INSERT INTO people` resolves to `public.people` and the
// corpus silently lands in the demo data instead of the measurement. This is the
// same shape as `testsupport.FixtureScript`, and for the same reason: a fixture
// needs the schema as well as the rows, or it is not a fixture.
//
// `db/seeds/001_demo.sql` is deliberately NOT applied. The measurement is
// hermetic: the same corpus on a freshly migrated database as on a seeded one,
// and no demo passage that no label judges sitting in the candidate pool.
func provisionRetrievalCorpus(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	root, err := testsupport.FindRepositoryRoot(".")
	if err != nil {
		return nil, fmt.Errorf("locate the repository root: %w", err)
	}
	script, err := retrievalCorpusScript(root)
	if err != nil {
		return nil, err
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", databaseURL, err)
	}
	defer admin.Close()
	// Dropped first, unconditionally: a schema left behind by a killed run would
	// otherwise collide with the ids in this file and the corpus would silently
	// come up short.
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{retrievalMeasurementSchema}.Sanitize()+` CASCADE`); err != nil {
		return nil, fmt.Errorf("clear a previous measurement schema: %w", err)
	}
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{retrievalMeasurementSchema}.Sanitize()); err != nil {
		return nil, fmt.Errorf("create the measurement schema: %w", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = retrievalMeasurementSchema + ",public"
	provisioner, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open the measurement pool: %w", err)
	}
	if _, err := provisioner.Exec(ctx, script); err != nil {
		provisioner.Close()
		return nil, fmt.Errorf("build the measurement schema: %w", err)
	}
	// The check is the one that would have caught the schema-resolution bug this
	// shape invites: the corpus has to be the corpus in the MEASUREMENT schema, and
	// not a handful of rows appended to somebody else's table.
	var provisioned int
	if err := provisioner.QueryRow(ctx, `SELECT count(*) FROM source_passages WHERE embedding IS NULL`).Scan(&provisioned); err != nil {
		provisioner.Close()
		return nil, fmt.Errorf("count the provisioned corpus: %w", err)
	}
	if provisioned == 0 {
		provisioner.Close()
		return nil, fmt.Errorf("%s applied without creating a single unembedded passage, so either it landed outside the %s schema or the demo seed is all that is there", retrievalCorpusFile, retrievalMeasurementSchema)
	}
	return provisioner, nil
}

// retrievalCorpusScript is the whole measurement schema as one statement string:
// the search path, every migration, the search path again, and the corpus.
//
// One round trip, because a hundred-odd `CREATE TABLE`s over a socket is a
// hundred-odd round trips otherwise, and the graph benchmark already learned what
// that costs when it did not ANALYZE before measuring.
func retrievalCorpusScript(root string) (string, error) {
	searchPath := `SET search_path TO ` + pgx.Identifier{retrievalMeasurementSchema}.Sanitize() + `, public;` + "\n"
	var builder strings.Builder
	builder.WriteString(searchPath)
	migrations, err := filepath.Glob(filepath.Join(root, "db", "migrations", "*.sql"))
	if err != nil {
		return "", fmt.Errorf("find the migrations: %w", err)
	}
	if len(migrations) == 0 {
		return "", errors.New("no migrations in db/migrations, so the measurement schema cannot be built")
	}
	sort.Strings(migrations)
	for _, file := range migrations {
		contents, readErr := os.ReadFile(file)
		if readErr != nil {
			return "", fmt.Errorf("read %s: %w", file, readErr)
		}
		builder.Write(contents)
		builder.WriteString("\n;\n")
	}
	corpus, err := os.ReadFile(retrievalCorpusFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", retrievalCorpusFile, err)
	}
	// The search path is written again because a migration may have changed it,
	// and the corpus must not inherit a schema it did not ask for.
	builder.WriteString(searchPath)
	builder.Write(corpus)
	builder.WriteString("\n;\n")
	return builder.String(), nil
}

// dropRetrievalCorpus removes the measurement schema, so the database is left
// exactly as it was found. It is the reason the command is safe to point at a
// developer's own database, which is also why it is not optional: a report that
// leaves 42 synthetic passages behind is a report that changed somebody's data.
func dropRetrievalCorpus(ctx context.Context, databaseURL string) error {
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", databaseURL, err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{retrievalMeasurementSchema}.Sanitize()+` CASCADE`); err != nil {
		return fmt.Errorf("drop the measurement schema: %w", err)
	}
	return nil
}

// retrievalCorpusFacts is the state of the corpus the run is scored against.
//
// Every field is read from the database rather than assumed from the fixture
// file, because the failure this whole plan exists to prevent is a report about a
// corpus that is not the one in the database.
type retrievalCorpusFacts struct {
	Passages        int
	Retrievable     int
	Embedded        int
	Sources         int
	NormalizedTexts []string
}

// readRetrievalCorpusFacts counts the seeded corpus and reads back the text each
// retrievable passage was embedded from, so the provider probe has real content
// to measure rather than a string this file made up.
func readRetrievalCorpusFacts(ctx context.Context, pool *pgxpool.Pool) (retrievalCorpusFacts, error) {
	facts := retrievalCorpusFacts{}
	rows, err := pool.Query(ctx, `
		SELECT sp.normalized_text_ar
		FROM source_passages sp
		JOIN sources src ON src.id = sp.source_id
		WHERE COALESCE(src.metadata->>'synthetic', 'false') = 'true'
		ORDER BY sp.id
	`)
	if err != nil {
		return facts, err
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return facts, err
		}
		facts.NormalizedTexts = append(facts.NormalizedTexts, text)
	}
	if err := rows.Err(); err != nil {
		return facts, err
	}
	facts.Passages = len(facts.NormalizedTexts)
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM source_passages sp
		JOIN sources src ON src.id = sp.source_id
		JOIN LATERAL (
			SELECT id FROM source_statements
			WHERE source_passage_id = sp.id AND review_status = 'accepted'
			ORDER BY created_at LIMIT 1
		) ss ON TRUE
		WHERE COALESCE(src.metadata->>'synthetic', 'false') = 'true'
	`).Scan(&facts.Retrievable); err != nil {
		return facts, err
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(sp.embedding)
		FROM source_passages sp
		JOIN sources src ON src.id = sp.source_id
		WHERE COALESCE(src.metadata->>'synthetic', 'false') = 'true'
	`).Scan(&facts.Embedded); err != nil {
		return facts, err
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM sources WHERE COALESCE(metadata->>'synthetic', 'false') = 'true'
	`).Scan(&facts.Sources); err != nil {
		return facts, err
	}
	return facts, nil
}

// measureRetrieval is the run. It is the body of the benchmark test and nothing
// else calls it.
func measureRetrieval(ctx context.Context, t *testing.T, pool *pgxpool.Pool, service *Service) retrievalReport {
	t.Helper()
	cases, casesDigest := loadRetrievalCases(t)
	facts, err := readRetrievalCorpusFacts(ctx, pool)
	if err != nil {
		t.Fatalf("read the corpus: %v", err)
	}

	report := retrievalReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Command:     "make retrieval-report",
		Arms:        map[retrievalArm]retrievalArmReport{},
		Warnings:    []string{},
	}
	report.Commit = retrievalCommit(t)
	report.Corpus = retrievalCorpusReport{
		Version:      RetrievalMeasurementVersion,
		CasesFile:    "services/core-api/internal/research/testdata/retrieval_measurement_cases.jsonl",
		CasesSHA256:  casesDigest,
		Passages:     facts.Passages,
		Retrievable:  facts.Retrievable,
		Embedded:     facts.Embedded,
		Sources:      facts.Sources,
		Cases:        len(cases),
		AuthorCaveat: "The same author wrote the corpus (db/seeds/002_retrieval_corpus.sql) and every label in the question set. There is no second reviewer. These are internal fixtures judged by the person who built the thing they judge, and every number below inherits that limitation.",
		ReviewedBy:   []string{},
	}
	judged := map[string]struct{}{}
	classifiers := map[string]int{}
	for _, item := range cases {
		classifiers[item.Class]++
		for _, id := range item.Relevant {
			judged[id] = struct{}{}
		}
	}
	report.Corpus.MultiHop = classifiers[retrievalClassMultiHop]
	report.Corpus.SingleHop = classifiers[retrievalClassSingleHop]
	report.Corpus.LexicalControls = classifiers[retrievalClassLexicalControl]
	report.Corpus.DistinctJudged = len(judged)
	reviewers := map[string]struct{}{}
	for _, item := range cases {
		reviewers[item.ReviewedBy] = struct{}{}
	}
	for reviewer := range reviewers {
		report.Corpus.ReviewedBy = append(report.Corpus.ReviewedBy, reviewer)
	}
	sort.Strings(report.Corpus.ReviewedBy)

	// The corpus has to be the corpus the labels were written against. A judged
	// passage that is not in the database is a corpus and a question set that
	// have drifted, and a measurement over it would be a measurement of nothing.
	rows, err := pool.Query(ctx, `
		SELECT sp.id::text, sp.embedding IS NOT NULL,
		       EXISTS (SELECT 1 FROM source_statements ss WHERE ss.source_passage_id = sp.id AND ss.review_status = 'accepted')
		FROM source_passages sp
		JOIN sources src ON src.id = sp.source_id
		WHERE COALESCE(src.metadata->>'synthetic', 'false') = 'true'
	`)
	if err != nil {
		t.Fatalf("read the corpus rows: %v", err)
	}
	defer rows.Close()
	inCorpus := map[string]struct{}{}
	unretrievable := map[string]struct{}{}
	for rows.Next() {
		var id string
		var embedded, retrievable bool
		if err := rows.Scan(&id, &embedded, &retrievable); err != nil {
			t.Fatalf("scan a corpus row: %v", err)
		}
		inCorpus[id] = struct{}{}
		if !retrievable {
			unretrievable[id] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the corpus rows: %v", err)
	}
	for id := range judged {
		if _, ok := inCorpus[id]; !ok {
			report.Corpus.JudgedMissing = append(report.Corpus.JudgedMissing, id)
		}
		if _, ok := unretrievable[id]; ok {
			report.Corpus.Unretrievable = append(report.Corpus.Unretrievable, id)
		}
	}
	sort.Strings(report.Corpus.JudgedMissing)
	sort.Strings(report.Corpus.Unretrievable)
	if len(report.Corpus.JudgedMissing) > 0 {
		t.Fatalf("%d judged passage(s) are not in the seeded corpus: %v", len(report.Corpus.JudgedMissing), report.Corpus.JudgedMissing)
	}
	if len(report.Corpus.Unretrievable) > 0 {
		t.Fatalf("%d judged passage(s) have no accepted statement, so no arm can return them: %v", len(report.Corpus.Unretrievable), report.Corpus.Unretrievable)
	}
	// The whole seeded corpus, not just the judged passages, has to carry the
	// normalizer's own output in `normalized_text_ar`. It is hand-written in SQL
	// and it is what both legs compare a query against, so a value the normalizer
	// would not produce is a row nothing can match - and a row outside the judged
	// set still changes what a ranked list looks like.
	if err := assertCorpusNormalized(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if facts.Embedded != facts.Passages {
		t.Fatalf("%d of %d seeded passages carry an embedding; run `go run ./cmd/embedding-backfill` against this database first, because an unembedded passage makes the vector leg return nothing and the whole comparison meaningless", facts.Embedded, facts.Passages)
	}

	providerReport, err := retrievalProviderSemantics(ctx, service.AI, 1536, facts.NormalizedTexts)
	if err != nil {
		t.Fatalf("measure the provider's embedding: %v", err)
	}
	report.Provider = providerReport
	if !providerReport.Semantic {
		report.Warnings = append(report.Warnings, "the configured provider's embedding is not semantic ("+providerReport.SemanticEvidence+"), so the vector-only arm is a hash ranking and every difference against it is a difference against a permutation rather than against an embedding model")
	}

	for _, arm := range retrievalArms {
		perCase := make([]retrievalArmResult, 0, len(cases))
		for _, item := range cases {
			input := QueryInput{Question: item.Query}
			if item.Graph != nil {
				input.GraphOperation = item.Graph.Operation
				input.GraphStartType = item.Graph.StartType
				input.GraphStartID = item.Graph.StartID
				input.GraphEndType = item.Graph.EndType
				input.GraphEndID = item.Graph.EndID
				input.GraphMaxDepth = item.Graph.MaxDepth
				input, err = normalizeGraphInput(input)
				if err != nil {
					t.Fatalf("case %s names the graph hop %q the service refuses: %v", item.ID, item.Graph.Operation, err)
				}
			}
			normalized := identity.NormalizeArabicName(item.Query)
			vector, err := retrievalEmbed(ctx, service.AI, normalized, 1536)
			if err != nil {
				t.Fatalf("embed the query for %s: %v", item.ID, err)
			}
			outcome, runErr := service.runRetrievalArm(ctx, arm, input, normalized, vector.vector)
			if runErr != nil {
				t.Fatalf("arm %s over case %s: %v", arm, item.ID, runErr)
			}
			perCase = append(perCase, scoreRetrieval(item, outcome))
		}
		report.Arms[arm] = retrievalArmReport{
			All:       aggregateRetrieval(perCase),
			MultiHop:  aggregateRetrieval(byClass(perCase, retrievalClassMultiHop)),
			SingleHop: aggregateRetrieval(byClass(perCase, retrievalClassSingleHop)),
			Lexical:   aggregateRetrieval(byClass(perCase, retrievalClassLexicalControl)),
			PerCase:   perCase,
		}
	}

	buildRetrievalComparison(&report)
	applyRetrievalDecision(&report)
	report.Corrupt = retrievalCorruptArms(report)
	return report
}

// assertCorpusNormalized refuses a corpus whose `normalized_text_ar` is not what
// `identity.NormalizeArabicName(text_ar)` produces.
//
// The value is hand-written in SQL, and it is the column both passage legs compare
// a normalized query against and the column the backfill embeds. A row whose two
// text columns disagree is a row no query can match and no vector can be compared
// against, and it would quietly depress every arm's recall - which is exactly the
// shape of a result that gets argued about instead of fixed.
func assertCorpusNormalized(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT sp.id::text, sp.text_ar, sp.normalized_text_ar
		FROM source_passages sp
		JOIN sources src ON src.id = sp.source_id
		WHERE COALESCE(src.metadata->>'synthetic', 'false') = 'true'
		ORDER BY sp.id
	`)
	if err != nil {
		return fmt.Errorf("read the seeded passages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, text, normalized string
		if err := rows.Scan(&id, &text, &normalized); err != nil {
			return fmt.Errorf("scan a seeded passage: %w", err)
		}
		if want := identity.NormalizeArabicName(text); want != normalized {
			return fmt.Errorf("passage %s has normalized_text_ar %q where identity.NormalizeArabicName(%q) is %q; the corpus cannot be measured until the seed and the normalizer agree", id, normalized, text, want)
		}
	}
	return rows.Err()
}

// buildRetrievalComparison fills in the difference table and the stated variance.
func buildRetrievalComparison(report *retrievalReport) {
	basis := retrievalArmVector
	variance := retrievalVariance(report.Arms[retrievalArmHybrid].MultiHop, report.Arms[basis].MultiHop)
	graphVariance := retrievalVariance(report.Arms[retrievalArmGraph].MultiHop, report.Arms[basis].MultiHop)
	reversedVariance := retrievalVariance(report.Arms[retrievalArmReversed].MultiHop, report.Arms[basis].MultiHop)
	report.Measurement = retrievalMeasurementShape{
		Depth:           retrievalDepth,
		MultiHopCases:   report.Arms[basis].MultiHop.Cases,
		VarianceMetric:  "sum of the two arms' Wilson 95% half-widths for recall on the multi-hop subset, w_a + w_b, where w is the half-length of the score interval for a proportion observed over n cases",
		VarianceValue:   retrievalRound(variance),
		VarianceMeaning: fmt.Sprintf("a difference in multi-hop recall at or below %.3f is inside the noise of %d cases and is reported as no measurable difference. One case is worth %.3f on its own. The bar is deliberately conservative in two ways: it is an interval half-width rather than a point estimate, and the two arms are treated as independent samples even though they are scored over the same questions. A paired statistic would be tighter, and a criterion about multi-hop questions should not be met on a difference a tighter statistic would have called significant and this one would not.", variance, report.Arms[basis].MultiHop.Cases, 1/float64(maxInt(report.Arms[basis].MultiHop.Cases, 1))),
		Method:          "recall@5 is the fraction of each case's judged passages inside the arm's first five results; MRR is the mean reciprocal rank of the first judged passage; a question the arm cannot answer scores zero for both rather than being dropped, so an arm cannot raise its average by answering fewer questions",
	}
	report.Comparison = retrievalComparison{
		Basis: string(basis),
		Note:  "Delta is against the vector-only arm on the same subset. Exceeds variance is judged against the stated variance for that pair, not against the hybrid-vs-vector one, because a graph arm that clears the bar is a different claim from a hybrid arm that clears it.",
	}
	rows := []retrievalComparisonRow{}
	for _, subset := range []struct {
		name   string
		metric func(retrievalArmReport) retrievalMetrics
	}{
		{"all", func(a retrievalArmReport) retrievalMetrics { return a.All }},
		{"multi_hop", func(a retrievalArmReport) retrievalMetrics { return a.MultiHop }},
		{"single_hop", func(a retrievalArmReport) retrievalMetrics { return a.SingleHop }},
		{"lexical_control", func(a retrievalArmReport) retrievalMetrics { return a.Lexical }},
	} {
		base := subset.metric(report.Arms[basis])
		for _, arm := range []retrievalArm{retrievalArmHybrid, retrievalArmGraph, retrievalArmReversed} {
			metrics := subset.metric(report.Arms[arm])
			bar := variance
			switch arm {
			case retrievalArmGraph:
				bar = graphVariance
			case retrievalArmReversed:
				bar = reversedVariance
			}
			delta := retrievalRound(metrics.RecallAt5 - base.RecallAt5)
			rows = append(rows, retrievalComparisonRow{
				Subset:              subset.name,
				Arm:                 arm,
				RecallAt5:           metrics.RecallAt5,
				MRR:                 metrics.MRR,
				DeltaRecallVsVector: delta,
				ExceedsVariance:     delta > retrievalRound(bar),
			})
		}
	}
	report.Comparison.Rows = rows
}

// applyRetrievalDecision writes the Phase 20 verdict, computed rather than
// asserted.
//
// The two ways out are both recorded honestly: a win by more than the stated
// variance marks the criterion met with the corpus size, the reviewer and the
// variance next to it, and anything else leaves it open with the reason named.
// There is no branch here that marks it met.
func applyRetrievalDecision(report *retrievalReport) {
	corpus := fmt.Sprintf("%d cases (%d multi-hop, %d single-hop, %d lexical control) over %d seeded passages, %d of them retrievable, labelled by %s", report.Corpus.Cases, report.Corpus.MultiHop, report.Corpus.SingleHop, report.Corpus.LexicalControls, report.Corpus.Passages, report.Corpus.Retrievable, strings.Join(report.Corpus.ReviewedBy, "; "))
	// The bar shrinks roughly as 1/sqrt(n), so the corpus a given difference needs
	// follows directly. It is stated for two differences rather than one because
	// "the corpus that would conclude" has no answer until somebody says what
	// difference is worth concluding, and guessing one number would be the same
	// error as picking a metric that flatters the result. Both figures below are
	// the Wilson sum at recall 0.5 against 0.5, which is the widest bar and so
	// the smallest corpus that could clear it.
	cases := maxInt(report.Measurement.MultiHopCases, 1)
	needTen := retrievalCasesForBar(0.10)
	needFive := retrievalCasesForBar(0.05)
	smallest := fmt.Sprintf("At %d multi-hop cases the bar is %.3f, and it shrinks roughly as 1/sqrt(n): clearing a ten-point difference needs about %d judged multi-hop cases per arm and clearing a five-point difference needs about %d. No synthetic corpus this repository can generate supplies those. They have to come from a production query log with judged results, which means real questions, relevance judgements by somebody who did not write the questions, and enough of them to matter. Until that exists the honest statement is that a corpus this size can rule an arm out and cannot rule one in.", cases, report.Measurement.VarianceValue, needTen, needFive)

	measured := fmt.Sprintf("Measured on the multi-hop subset: vector-only recall@5 %.3f (MRR %.3f), hybrid %.3f (MRR %.3f), graph-augmented %.3f (MRR %.3f), stated variance %.3f.",
		report.Arms[retrievalArmVector].MultiHop.RecallAt5, report.Arms[retrievalArmVector].MultiHop.MRR,
		report.Arms[retrievalArmHybrid].MultiHop.RecallAt5, report.Arms[retrievalArmHybrid].MultiHop.MRR,
		report.Arms[retrievalArmGraph].MultiHop.RecallAt5, report.Arms[retrievalArmGraph].MultiHop.MRR,
		report.Measurement.VarianceValue)
	best := retrievalArmHybrid
	if report.Comparison.row("multi_hop", retrievalArmGraph).ExceedsVariance {
		best = retrievalArmGraph
	}
	winning := report.Comparison.row("multi_hop", best)
	if winning.ExceedsVariance && report.Provider.Semantic {
		report.Decision = retrievalDecision{
			Phase20Criterion: "multi-hop research questions improve over vector-only RAG (IMPLEMENTATION_PLAN.md:1458)",
			Outcome:          "met_on_this_corpus",
			Statement:        measured + " " + fmt.Sprintf("%s beats vector-only on the multi-hop subset by %.3f recall, which is more than the stated variance of %.3f.", best, winning.DeltaRecallVsVector, report.Measurement.VarianceValue),
			Corpus:           corpus,
			Variance:         report.Measurement.VarianceMeaning,
			SmallestCorpus:   smallest,
		}
		return
	}
	reasons := []string{}
	if !report.Provider.Semantic {
		reasons = append(reasons, "the only embedding provider this repository may use returns a hash of the input rather than a semantic vector, so the vector-only arm is not an embedding baseline and a difference against it is a difference against a permutation")
	}
	if !winning.ExceedsVariance {
		reasons = append(reasons, fmt.Sprintf("neither the hybrid nor the graph-augmented arm beats vector-only on the multi-hop subset by more than the stated variance of %.3f (hybrid %+.3f, graph-augmented %+.3f)", report.Measurement.VarianceValue, report.Comparison.row("multi_hop", retrievalArmHybrid).DeltaRecallVsVector, report.Comparison.row("multi_hop", retrievalArmGraph).DeltaRecallVsVector))
	}
	if report.Arms[retrievalArmGraph].MultiHop.GraphPassagesAdded == 0 {
		reasons = append(reasons, "the graph leg contributed no passage to any multi-hop case, because execute appends graph citations after the fused list and a multi-hop case here already returns more than five candidates from the passage legs")
	}
	report.Decision = retrievalDecision{
		Phase20Criterion: "multi-hop research questions improve over vector-only RAG (IMPLEMENTATION_PLAN.md:1458)",
		Outcome:          "still_open",
		Statement:        measured + " The criterion is not met, and the reason that dominates the rest is: " + strings.Join(reasons, "; ") + ". This is recorded as a finding, not reframed.",
		Corpus:           corpus,
		Variance:         report.Measurement.VarianceMeaning,
		SmallestCorpus:   smallest,
	}
}

func (r retrievalComparison) row(subset string, arm retrievalArm) retrievalComparisonRow {
	for _, row := range r.Rows {
		if row.Subset == subset && row.Arm == arm {
			return row
		}
	}
	return retrievalComparisonRow{}
}

// retrievalCorruptArms names the arms that returned nothing anywhere, which is
// how a broken run is told apart from a bad result.
func retrievalCorruptArms(report retrievalReport) []string {
	corrupt := []string{}
	for _, arm := range retrievalArms {
		if report.Arms[arm].All.Answered == 0 {
			corrupt = append(corrupt, string(arm))
		}
	}
	return corrupt
}

// retrievalCasesForBar is how many multi-hop cases per arm bring the stated bar
// down to the given difference.
//
// It is found by asking `retrievalVariance` rather than by inverting a formula,
// because the bar is a Wilson sum and the closed form for it is a nuisance. The
// widest bar - two arms both at recall 0.5 - is the one searched, so the number
// returned is the smallest corpus that could clear that bar rather than a
// flattering one, and the search is a plain ascending scan so the answer is
// exactly the first `n` that clears it.
func retrievalCasesForBar(difference float64) int {
	if difference <= 0 {
		return 0
	}
	bar := func(cases int) float64 {
		wide := retrievalMetrics{RecallAt5: 0.5, Cases: cases}
		return retrievalVariance(wide, wide)
	}
	// The bar falls roughly as 1.96/sqrt(n), so this start point is within a
	// small factor and the scan from here is short.
	cases := int(math.Ceil(3.85 / (difference * difference)))
	if cases < 1 {
		cases = 1
	}
	for cases > 1 && bar(cases-1) <= difference {
		cases--
	}
	for bar(cases) > difference {
		cases++
	}
	return cases
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// writeRetrievalReport writes the JSON report beside the document that quotes it.
func writeRetrievalReport(t *testing.T, report retrievalReport) string {
	t.Helper()
	target := retrievalReportPath(t)
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
	return target
}

func retrievalReportPath(t *testing.T) string {
	t.Helper()
	if override := os.Getenv(retrievalMeasurementReportEnv); override != "" {
		return override
	}
	root, err := testsupport.FindRepositoryRoot(".")
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	return filepath.Join(root, defaultRetrievalMeasurementReport)
}

// retrievalCommit is the commit the run was made on, when there is one. A report
// without it cannot be traced to the code that produced it.
func retrievalCommit(t *testing.T) string {
	t.Helper()
	root, err := testsupport.FindRepositoryRoot(".")
	if err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
