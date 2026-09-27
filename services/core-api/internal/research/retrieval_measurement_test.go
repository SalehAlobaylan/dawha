package research

// The three tests that keep the retrieval measurement honest.
//
//   - TestRetrievalMeasurementCasesAreWellFormed runs in the ordinary acceptance
//     suite and needs no database. It checks the labelled question set against
//     itself, so a case cannot quietly lose its judged passages, its reviewer or
//     its class.
//   - TestRetrievalHarnessIsNotVacuous is the proof the plan asks for: a
//     deliberately broken arm has to measure worse than the good one, or every
//     other number in the report is a number about nothing.
//   - TestRetrievalMeasurementReport is the measurement, gated on
//     DAWHA_RETRIEVAL_BENCH. It is what `make retrieval-report` runs.
//
// The gate is in the report, not in these tests: `applyRetrievalDecision` has no
// branch that marks Phase 20's criterion met unless the measured difference
// exceeds the stated variance AND the provider's embedding is semantic, so a
// future run on a better provider cannot quietly turn a null result into a pass.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/embeddingbackfill"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

// TestRetrievalMeasurementCasesAreWellFormed checks the question set with no
// database at all, so a broken fixture fails the fast gate rather than waiting
// for somebody to run a measurement and read the number.
func TestRetrievalMeasurementCasesAreWellFormed(t *testing.T) {
	cases, digest := loadRetrievalCases(t)
	if digest == "" {
		t.Fatal("the question set produced no digest, so two reports could not be compared")
	}
	if RetrievalMeasurementVersion == "" {
		t.Fatal("the measurement carries no version, so a report cannot say which corpus and labels it measured")
	}
	for _, class := range retrievalClasses {
		found := 0
		for _, item := range cases {
			if item.Class == class {
				found++
			}
		}
		if found == 0 {
			t.Fatalf("the question set holds no %s case; the class is measured, so an empty one is a hole in the report rather than an absence of data", class)
		}
	}
	// Every multi-hop case that names a traversal has to name one the service
	// implements, and a case with no traversal is a case the graph arm cannot be
	// asked about. Both are recorded rather than left for a reader to infer.
	withGraph, withoutGraph := 0, 0
	for _, item := range cases {
		if item.Graph == nil {
			withoutGraph++
			continue
		}
		withGraph++
		if item.Graph.Operation == "" {
			t.Fatalf("case %s has a graph block with no operation", item.ID)
		}
	}
	if withGraph == 0 {
		t.Fatal("no case names a graph traversal, so the graph-augmented arm would be the hybrid arm under another name")
	}
	t.Logf("%d cases: %d with a graph traversal, %d without", len(cases), withGraph, withoutGraph)
}

// TestRetrievalHarnessIsNotVacuous is the check on the measurement itself.
//
// A harness that reports the same recall for a correct ranking and a reversed one
// is not measuring retrieval. So this seeds a small corpus, runs the good arm and
// the reversed arm over the same questions, and requires the good one to beat the
// reversed one. If a future change to the fusion, the rerank or the scoring made
// the ranking arbitrary, this fails.
func TestRetrievalHarnessIsNotVacuous(t *testing.T) {
	fixture := testsupport.New(t)
	ctx := fixture.Ctx()
	provider := &embedProviderStub{dimensions: embeddingbackfill.Dimensions}
	if _, err := embeddingbackfill.New(fixture.Pool(), provider).Run(ctx, embeddingbackfill.SeedScope()); err != nil {
		t.Fatalf("backfill the fixture corpus: %v", err)
	}
	service := &Service{Pool: fixture.Pool(), AI: provider}

	questions := []struct {
		query    string
		relevant string
	}{
		{"من كان والد عبدالله بن محمد؟", ""},
		{"ما الذي يقوله معجم الأنساب عن سالم بن نافع؟", ""},
		{"أين تولى راشد بن هلال الخراب؟", ""},
	}
	// The judged passage for each question is whichever seeded passage the lexical
	// leg ranks first for it. That is the point: the reversed arm must rank it
	// last, so the two arms cannot agree by accident on a corpus this small.
	judged := make([]string, 0, len(questions))
	for _, question := range questions {
		normalized := identity.NormalizeArabicName(question.query)
		vector := providerVector(t, provider, normalized)
		outcome, err := service.runRetrievalArm(ctx, retrievalArmHybrid, QueryInput{Question: question.query}, normalized, vector)
		if err != nil {
			t.Fatalf("the hybrid arm over %q: %v", question.query, err)
		}
		if len(outcome.Passages) < 2 {
			t.Skipf("the fixture corpus returned %d passage(s) for %q, so there is no ranking to reverse; this test needs a corpus where the good arm and the reversed arm can differ", len(outcome.Passages), question.query)
		}
		judged = append(judged, outcome.Passages[0].PassageID)
	}

	// The proof is on the reciprocal rank, not on recall@5, and that is the whole
	// subtlety: with fewer than six results, reversing a list moves every passage
	// but keeps them all inside the top five, so recall@5 cannot see the
	// difference and a reader would be told the harness works when it is blind to
	// order. MRR is measured at every position, so it does see it.
	good, bad := 0.0, 0.0
	for index, question := range questions {
		normalized := identity.NormalizeArabicName(question.query)
		vector := providerVector(t, provider, normalized)
		caseItem := retrievalCase{ID: question.query, Relevant: []string{judged[index]}}
		for arm, into := range map[retrievalArm]*float64{retrievalArmHybrid: &good, retrievalArmReversed: &bad} {
			outcome, err := service.runRetrievalArm(ctx, arm, QueryInput{Question: question.query}, normalized, vector)
			if err != nil {
				t.Fatalf("arm %s over %q: %v", arm, question.query, err)
			}
			result := scoreRetrieval(caseItem, outcome)
			if result.Rank == 0 {
				t.Fatalf("arm %s did not return the judged passage at all for %q, so there is no ranking to compare", arm, question.query)
			}
			*into += 1 / float64(result.Rank)
		}
	}
	count := float64(len(questions))
	if good/count <= bad/count {
		t.Fatalf("the reversed arm scored MRR %.3f and the good arm scored %.3f: the harness cannot tell a correct ranking from a broken one, so every number it produces is a number about nothing", bad/count, good/count)
	}
	t.Logf("good arm MRR %.3f, deliberately reversed arm MRR %.3f over %d questions", good/count, bad/count, len(questions))
}

// TestRetrievalMetricsCountWhatTheyClaim pins the two metrics, because a metric
// that quietly excluded the questions an arm could not answer would let an arm
// improve its average by answering fewer questions.
func TestRetrievalMetricsCountWhatTheyClaim(t *testing.T) {
	results := []retrievalArmResult{
		{CaseID: "a", Class: retrievalClassMultiHop, Recall: 1, Rank: 1, Returned: []string{"p1"}},
		{CaseID: "b", Class: retrievalClassMultiHop, Recall: 0, Returned: []string{"p2"}},
		{CaseID: "c", Class: retrievalClassMultiHop, Recall: 0},
	}
	metrics := aggregateRetrieval(results)
	if metrics.Cases != 3 {
		t.Fatalf("the metrics counted %d cases, wanted 3", metrics.Cases)
	}
	if metrics.Answered != 2 || metrics.Empty != 1 {
		t.Fatalf("the metrics reported %d answered and %d empty, wanted 2 and 1", metrics.Answered, metrics.Empty)
	}
	if metrics.RecallAt5 != 0.333 {
		t.Fatalf("recall@5 over three cases was %.3f, wanted 0.333: a question with no results must score 0 rather than be dropped", metrics.RecallAt5)
	}
	if metrics.MRR != 0.333 {
		t.Fatalf("MRR over three cases was %.3f, wanted 0.333 for one first-position hit out of three", metrics.MRR)
	}
	if len(metrics.EmptyCases) != 1 || metrics.EmptyCases[0] != "c" {
		t.Fatalf("the metrics named the empty cases %v, wanted [c]", metrics.EmptyCases)
	}
	empty := aggregateRetrieval(nil)
	if empty.Cases != 0 || empty.RecallAt5 != 0 {
		t.Fatalf("aggregating nothing produced %+v, wanted an empty result", empty)
	}
}

// TestRetrievalVarianceIsStatedBeforeItIsUsed checks the bar is what the report
// says it is, and that it is a number about cases rather than a constant.
func TestRetrievalVarianceIsStatedBeforeItIsUsed(t *testing.T) {
	// Two arms, both at 0.5 over 17 multi-hop cases: the widest honest bar. The
	// Wilson sum at p=0.5 is about 1.96/sqrt(n), so 0.43 at seventeen cases.
	wide := retrievalVariance(retrievalMetrics{RecallAt5: 0.5, Cases: 17}, retrievalMetrics{RecallAt5: 0.5, Cases: 17})
	if wide < 0.42 || wide > 0.44 {
		t.Fatalf("the stated variance at recall 0.5 over 17 cases is %.3f, which is not the ~0.43 the report describes", wide)
	}
	// The bar must never collapse, which is the reason Wilson is used instead of
	// the plain binomial standard error. Two perfect arms over seventeen cases
	// have a binomial standard error of exactly zero each, and a Wilson half-width
	// of 0.092 each, so the sum is 0.184 rather than 0.
	perfect := retrievalVariance(retrievalMetrics{RecallAt5: 1, Cases: 17}, retrievalMetrics{RecallAt5: 1, Cases: 17})
	if perfect < 0.18 || perfect > 0.19 {
		t.Fatalf("the bar between two perfect arms over 17 cases is %.3f, which is not the ~0.184 a non-degenerate interval gives", perfect)
	}
	// The corpus sizes the report names must actually clear the bar they are
	// quoted for, or the smallest-corpus statement is a guess with a number in it.
	for _, difference := range []float64{0.10, 0.05} {
		needed := retrievalCasesForBar(difference)
		wide := retrievalMetrics{RecallAt5: 0.5, Cases: needed}
		if got := retrievalVariance(wide, wide); got > difference {
			t.Fatalf("the report says about %d cases clear a %.2f difference, but at %d cases the bar is still %.3f", needed, difference, needed, got)
		}
		oneFewer := retrievalMetrics{RecallAt5: 0.5, Cases: needed - 1}
		if got := retrievalVariance(oneFewer, oneFewer); got <= difference {
			t.Fatalf("the report says about %d cases are needed for a %.2f difference, but %d already clears it at %.3f", needed, difference, needed-1, got)
		}
	}
	// A larger corpus shrinks it, which is the whole argument for wanting one.
	narrow := retrievalVariance(retrievalMetrics{RecallAt5: 0.5, Cases: 400}, retrievalMetrics{RecallAt5: 0.5, Cases: 400})
	if narrow >= wide {
		t.Fatalf("the variance over 400 cases (%.3f) is not smaller than over 17 (%.3f)", narrow, wide)
	}
	// A perfect arm against a hopeless one is a real difference, and the bar says
	// so: 0.092 plus 0.092, which a recall difference of 1.0 clears easily.
	clear := retrievalVariance(retrievalMetrics{RecallAt5: 1, Cases: 17}, retrievalMetrics{RecallAt5: 0, Cases: 17})
	if clear <= 0 {
		t.Fatalf("the bar between a perfect arm and a zero arm is %.3f, which would let any difference pass", clear)
	}
}

// TestRetrievalDecisionCannotBeMetWithoutEvidence is the assertion on the
// decision code itself rather than on its output.
//
// `applyRetrievalDecision` is the only place the Phase 20 criterion can be
// marked met, and it has two conditions. This builds the two halves of a passing
// report and checks each half alone does not pass.
func TestRetrievalDecisionCannotBeMetWithoutEvidence(t *testing.T) {
	report := retrievalReport{
		Corpus: retrievalCorpusReport{
			Cases: 29, MultiHop: 17, SingleHop: 7, LexicalControls: 5,
			Passages: 45, Retrievable: 44, ReviewedBy: []string{"plan-014-author"},
		},
		Measurement: retrievalMeasurementShape{MultiHopCases: 17, VarianceValue: 0.171, VarianceMeaning: "the stated bar, carried into the decision so a met criterion quotes the same number the comparison did"},
		Arms:        map[retrievalArm]retrievalArmReport{},
	}
	multiHop := func(recall float64, graphAdded int) retrievalMetrics {
		return retrievalMetrics{Cases: 17, Answered: 17, RecallAt5: recall, MRR: recall, GraphPassagesAdded: graphAdded}
	}
	report.Arms[retrievalArmVector] = retrievalArmReport{MultiHop: multiHop(0.2, 0)}
	report.Arms[retrievalArmHybrid] = retrievalArmReport{MultiHop: multiHop(0.6, 0)}
	report.Arms[retrievalArmGraph] = retrievalArmReport{MultiHop: multiHop(0.6, 0)}
	report.Arms[retrievalArmReversed] = retrievalArmReport{MultiHop: multiHop(0.1, 0)}
	report.Comparison = retrievalComparison{Rows: []retrievalComparisonRow{
		{Subset: "multi_hop", Arm: retrievalArmHybrid, RecallAt5: 0.6, DeltaRecallVsVector: 0.4, ExceedsVariance: true},
		{Subset: "multi_hop", Arm: retrievalArmGraph, RecallAt5: 0.6, DeltaRecallVsVector: 0.4, ExceedsVariance: true},
	}}

	// A big measured difference with a hash provider must NOT be a pass: the
	// difference is real, but it is a difference against a permutation, and
	// calling that "multi-hop research questions improve over vector-only RAG"
	// would be the exact claim this plan exists to stop being unfalsifiable.
	report.Provider = retrievalProviderReport{Semantic: false, SemanticEvidence: "cosine(query, other) mean 0.0004"}
	applyRetrievalDecision(&report)
	if report.Decision.Outcome != "still_open" {
		t.Fatalf("a 0.4 recall difference against a non-semantic provider produced the outcome %q; the criterion must not be met against a hash", report.Decision.Outcome)
	}
	if !strings.Contains(report.Decision.Statement, "hash") {
		t.Fatalf("the decision does not name the reason: %s", report.Decision.Statement)
	}

	// A semantic provider with a difference inside the variance must also NOT pass.
	report.Provider = retrievalProviderReport{Semantic: true, SemanticEvidence: "cosine(query, other) mean 0.41"}
	report.Comparison.Rows[0].ExceedsVariance = false
	report.Comparison.Rows[1].ExceedsVariance = false
	applyRetrievalDecision(&report)
	if report.Decision.Outcome != "still_open" {
		t.Fatalf("a difference inside the stated variance produced the outcome %q", report.Decision.Outcome)
	}

	// Both together, and only then.
	report.Comparison.Rows[0].ExceedsVariance = true
	report.Comparison.Rows[1].ExceedsVariance = true
	applyRetrievalDecision(&report)
	if report.Decision.Outcome != "met_on_this_corpus" {
		t.Fatalf("a semantic provider and a difference beyond the variance produced the outcome %q", report.Decision.Outcome)
	}
	if report.Decision.Corpus == "" || report.Decision.Variance == "" || report.Decision.SmallestCorpus == "" {
		t.Fatalf("a met criterion must carry the corpus, the variance and the smallest corpus that would conclude; got %+v", report.Decision)
	}
	if !strings.Contains(report.Decision.Corpus, "plan-014-author") {
		t.Fatalf("a met criterion must name the reviewer next to the claim; got %q", report.Decision.Corpus)
	}
}

// TestRetrievalMeasurementReport is the measurement, and the only test here that
// needs a database and a running AI service. It provisions the corpus, embeds it,
// scores the arms and drops the corpus again. `make retrieval-report` runs it.
func TestRetrievalMeasurementReport(t *testing.T) {
	if os.Getenv(retrievalMeasurementEnv) == "" {
		t.Skip("set " + retrievalMeasurementEnv + "=1 to measure the three retrieval arms; it writes a report, provisions a corpus and needs a database, so it is a measurement command and not part of the acceptance suite")
	}
	aiURL := strings.TrimSpace(os.Getenv(retrievalMeasurementAIEnv))
	if aiURL == "" {
		t.Fatalf("%s is set but %s is empty. The two legs under measurement ARE an embedding call and a rerank call, so this command has to run against the real provider; a stub in its place would measure the stub. Start it with: cd services/ai-research && .venv/bin/python -m uvicorn app.main:app --host 127.0.0.1 --port 8000", retrievalMeasurementEnv, retrievalMeasurementAIEnv)
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Fatalf("%s is set but DATABASE_URL is empty", retrievalMeasurementEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	// The corpus is provisioned, embedded, measured and dropped here, so the
	// command works on a database that has only db/seeds/001_demo.sql applied
	// and leaves the database it was pointed at exactly as it found it. The
	// teardown is deferred before anything can fail, because a run that measured
	// a corpus and then died would otherwise leave it behind.
	keepCorpus := strings.TrimSpace(os.Getenv(retrievalKeepCorpusEnv)) != ""
	pool, err := provisionRetrievalCorpus(ctx, databaseURL)
	if err != nil {
		t.Fatalf("provision the measurement corpus: %v", err)
	}
	defer func() {
		if keepCorpus {
			t.Logf("%s is set, so the %s schema is left in place", retrievalKeepCorpusEnv, retrievalMeasurementSchema)
			pool.Close()
			return
		}
		pool.Close()
		if err := dropRetrievalCorpus(context.Background(), databaseURL); err != nil {
			t.Errorf("drop the measurement corpus: %v", err)
		}
	}()
	t.Logf("provisioned the corpus into the %s schema", retrievalMeasurementSchema)

	// The embeddings, through the same contract the worker calls, and only for the
	// corpus just provisioned. `measureRetrieval` refuses to run over a corpus with
	// unembedded passages, so this is a precondition rather than a convenience.
	backfilled, err := embeddingbackfill.New(pool, providerFor(aiURL)).Run(ctx, embeddingbackfill.SeedScope())
	if err != nil {
		t.Fatalf("embed the provisioned corpus: %v", err)
	}
	t.Logf("embedded %d of %d provisioned passages (%d were already embedded)", backfilled.Written, backfilled.After.Total, backfilled.Before.Missing)

	service := &Service{Pool: pool, AI: providerFor(aiURL)}
	report := measureRetrieval(ctx, t, pool, service)
	target := writeRetrievalReport(t, report)

	for _, arm := range retrievalArms {
		metrics := report.Arms[arm]
		t.Logf("%-18s all recall@5 %.3f mrr %.3f empty %d | multi-hop %.3f/%.3f empty %d graph+ %d | single %.3f | lexical %.3f",
			arm, metrics.All.RecallAt5, metrics.All.MRR, metrics.All.Empty,
			metrics.MultiHop.RecallAt5, metrics.MultiHop.MRR, metrics.MultiHop.Empty, metrics.MultiHop.GraphPassagesAdded,
			metrics.SingleHop.RecallAt5, metrics.Lexical.RecallAt5)
	}
	t.Logf("stated variance on the multi-hop subset: %.3f over %d cases", report.Measurement.VarianceValue, report.Measurement.MultiHopCases)
	t.Logf("provider embedding semantic: %v (%s)", report.Provider.Semantic, report.Provider.SemanticEvidence)
	t.Logf("phase 20: %s -> %s", report.Decision.Outcome, report.Decision.Statement)
	t.Logf("report written to %s", target)

	// The harness is not allowed to be vacuous, and this is where that is proved
	// on the real corpus rather than on a fixture.
	good := report.Arms[retrievalArmHybrid].All.RecallAt5
	broken := report.Arms[retrievalArmReversed].All.RecallAt5
	if good <= broken {
		t.Errorf("the reversed arm scored %.3f and the hybrid scored %.3f over the real corpus: the harness cannot tell a correct ranking from a broken one, so the comparison above means nothing", broken, good)
	}
	if report.Arms[retrievalArmVector].All.Answered == 0 {
		t.Error("the vector-only arm returned nothing for every case; either the corpus is unembedded or the leg is broken, and both are findings rather than a number to publish")
	}
	if len(report.Corrupt) > 0 {
		t.Errorf("arm(s) %v answered nothing at all; the report records them as corrupt", report.Corrupt)
	}
}

// providerFor is the provider the measurement runs against: the real deterministic
// service, named once so the provisioning step and the scoring step cannot end up
// on different ones.
func providerFor(aiURL string) ai.Provider {
	return ai.NewHTTPClient(aiURL)
}
