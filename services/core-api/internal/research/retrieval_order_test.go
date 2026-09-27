package research

// Ordering is part of what this product asserts, so it is tested like one.
//
// `fuseAndRerank` merges the two passage legs into one candidate set, scores it
// and reranks it, and it did all of that through a `map[string]*Citation` that it
// then walked to build the slice it sorted. Go randomises map iteration and both
// sorts are stable, so every candidate that tied kept whatever order the map
// happened to produce: the same question against the same corpus returned the same
// passages in a different order on every call.
//
// The defined order is rerank score, then combined score, then passage id. The
// tests below are that sentence, executable.

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/embeddingbackfill"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// orderOf is the result order as a comparable value, so a failure prints two
// lists rather than "they differ".
func orderOf(citations []Citation) []string {
	ids := make([]string, 0, len(citations))
	for _, citation := range citations {
		ids = append(ids, citation.PassageID)
	}
	return ids
}

// seedOrderTies writes one public source carrying four passages with IDENTICAL
// text, each with an accepted statement, so every leg ties on all three of
// rerank, lexical and vector and the only thing that can order them is the
// tie-break. Identical text is what makes the vector scores identical too: the
// deterministic provider hashes its input, so equal text is equal vector.
//
// The ids are created in descending order on purpose. A map walk does not care
// about insertion order, so creating them backwards cannot accidentally satisfy
// the assertion.
func seedOrderTies(t *testing.T, fixture *testsupport.Fixture) []uuid.UUID {
	t.Helper()
	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata) VALUES ($1, 'مصدر التكرار', 'book', 'public', '{"synthetic":true}')`, sourceID)
	ids := make([]uuid.UUID, 0, 4)
	for index := 0; index < 4; index++ {
		id := uuid.New()
		ids = append(ids, id)
		fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, $3, 'عبارة واحدة تتكرر في أربعة مقاطع', 'عباره واحده تتكرر في اربعه مقاطع')`, id, sourceID, index+1)
		fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, extraction_method, review_status) VALUES ($1, $2, $3, 'عبارة مقبولة تتكرر', 'manual', 'accepted')`, uuid.New(), sourceID, id)
	}
	// The provider stub's Rerank gives every document the same score, which is the
	// worst case: nothing but the tie-break can order the result.
	provider := &embedProviderStub{dimensions: embeddingbackfill.Dimensions}
	if _, err := embeddingbackfill.New(fixture.Pool(), provider).Run(fixture.Ctx(), embeddingbackfill.SeedScope()); err != nil {
		t.Fatalf("embed the fixture corpus: %v", err)
	}
	return ids
}

// TestFusedOrderIsIdenticalAcrossRuns is the regression test for the artifact
// problem. The same question, the same corpus, the same provider, twice: the
// complete order must come back the same both times.
//
// Every leg of this ties by construction, so this fails against the code that
// walked a map - not on one run in ten but on essentially every run.
func TestFusedOrderIsIdenticalAcrossRuns(t *testing.T) {
	fixture := testsupport.New(t)
	seedOrderTies(t, fixture)
	provider := &embedProviderStub{dimensions: embeddingbackfill.Dimensions}
	service := &Service{Pool: fixture.Pool(), AI: provider}

	const question = "عبارة تتكرر في أربعة مقاطع"
	normalized := identity.NormalizeArabicName(question)
	vector := providerVector(t, provider, normalized)
	retrieval := retrievalContext{Input: QueryInput{Question: question}, Normalized: normalized, Vector: vector}

	lexical, err := service.retrieveLexicalPassages(fixture.Ctx(), retrieval)
	if err != nil {
		t.Fatalf("lexical leg: %v", err)
	}
	byVector, err := service.retrieveVectorPassages(fixture.Ctx(), retrieval)
	if err != nil {
		t.Fatalf("vector leg: %v", err)
	}
	if len(lexical) < 4 {
		t.Skipf("the two legs returned %d lexical and %d vector candidates; this test needs at least four so that the order has something to be unstable about", len(lexical), len(byVector))
	}

	first, _, err := service.fuseAndRerank(fixture.Ctx(), retrieval, lexical, byVector)
	if err != nil {
		t.Fatalf("first fusion: %v", err)
	}
	// Twelve runs rather than two. A map walk changes the order on the FIRST
	// comparison most of the time, but a test that only fails sometimes is a test
	// nobody can trust, and the whole point is that this is not a rare condition.
	firstOrder := orderOf(first)
	for attempt := 0; attempt < 12; attempt++ {
		again, _, err := service.fuseAndRerank(fixture.Ctx(), retrieval, lexical, byVector)
		if err != nil {
			t.Fatalf("fusion on attempt %d: %v", attempt, err)
		}
		if got := orderOf(again); !slices.Equal(firstOrder, got) {
			t.Fatalf("the same query over the same corpus returned a different order on attempt %d.\n  first: %s\n  again: %s", attempt, strings.Join(firstOrder, " "), strings.Join(got, " "))
		}
	}
	t.Logf("%d runs agreed on one order for %d tied candidates", 13, len(firstOrder))
}

// TestFusedOrderBreaksTiesByPassageID is the second half: the order is not merely
// stable, it is the documented one. Every candidate here ties on every score, so
// the expected order is passage id ascending and nothing else.
func TestFusedOrderBreaksTiesByPassageID(t *testing.T) {
	fixture := testsupport.New(t)
	ids := seedOrderTies(t, fixture)
	provider := &embedProviderStub{dimensions: embeddingbackfill.Dimensions}
	service := &Service{Pool: fixture.Pool(), AI: provider}

	const question = "عبارة تتكرر في أربعة مقاطع"
	normalized := identity.NormalizeArabicName(question)
	vector := providerVector(t, provider, normalized)
	retrieval := retrievalContext{Input: QueryInput{Question: question}, Normalized: normalized, Vector: vector}

	lexical, err := service.retrieveLexicalPassages(fixture.Ctx(), retrieval)
	if err != nil {
		t.Fatalf("lexical leg: %v", err)
	}
	byVector, err := service.retrieveVectorPassages(fixture.Ctx(), retrieval)
	if err != nil {
		t.Fatalf("vector leg: %v", err)
	}
	fused, _, err := service.fuseAndRerank(fixture.Ctx(), retrieval, lexical, byVector)
	if err != nil {
		t.Fatalf("fusion: %v", err)
	}
	// Scoped to this test's own four passages. The fixture also carries the demo
	// seed's rows, and those score differently, so a tie assertion over everything
	// in the result would be a statement about the fixture rather than about the
	// tie-break.
	created := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		created[id.String()] = struct{}{}
	}
	own := make([]Citation, 0, len(ids))
	for _, citation := range fused {
		if _, ok := created[citation.PassageID]; ok {
			own = append(own, citation)
		}
	}
	if len(own) < 4 {
		t.Skipf("only %d of this test's four passages came back; it needs four tied ones", len(own))
	}
	own = own[:4]

	// Every score really does tie, or the test is not testing the tie-break.
	for index := 1; index < len(own); index++ {
		if own[index].Score.Combined != own[0].Score.Combined || own[index].Score.Rerank != own[0].Score.Rerank {
			t.Fatalf("this test's candidates %d and %d do not tie (combined %v/%v, rerank %v/%v), so the fixture cannot test the tie-break",
				index-1, index, own[index-1].Score.Combined, own[index].Score.Combined, own[index-1].Score.Rerank, own[index].Score.Rerank)
		}
	}
	got := orderOf(own)
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	for index := range sorted {
		if got[index] != sorted[index] {
			t.Fatalf("four candidates that tie on every score came back as %s, and the documented order is passage id ascending: %s", strings.Join(got, " "), strings.Join(sorted, " "))
		}
	}
	t.Logf("four tied candidates ordered by passage id: %s", strings.Join(got, " "))
}

// TestFusedOrderKeepsTheScoreOrderFirst checks the tie-break did not become the
// primary key. A deterministic total order is only useful if it still puts the
// better candidate first, so this asserts the documented precedence rather than
// only the documented fallback.
func TestFusedOrderKeepsTheScoreOrderFirst(t *testing.T) {
	fixture := testsupport.New(t)
	seedOrderTies(t, fixture)
	provider := &embedProviderStub{dimensions: embeddingbackfill.Dimensions}
	service := &Service{Pool: fixture.Pool(), AI: provider}

	const question = "عبارة تتكرر في أربعة مقاطع"
	normalized := identity.NormalizeArabicName(question)
	vector := providerVector(t, provider, normalized)
	retrieval := retrievalContext{Input: QueryInput{Question: question}, Normalized: normalized, Vector: vector}

	lexical, err := service.retrieveLexicalPassages(fixture.Ctx(), retrieval)
	if err != nil {
		t.Fatalf("lexical leg: %v", err)
	}
	byVector, err := service.retrieveVectorPassages(fixture.Ctx(), retrieval)
	if err != nil {
		t.Fatalf("vector leg: %v", err)
	}
	if len(lexical) < 2 {
		t.Skipf("the lexical leg returned %d candidates; this test needs two", len(lexical))
	}
	fused, _, err := service.fuseAndRerank(fixture.Ctx(), retrieval, lexical, byVector)
	if err != nil {
		t.Fatalf("fusion: %v", err)
	}
	for index := 1; index < len(fused); index++ {
		previous, current := fused[index-1], fused[index]
		if previous.Score.Rerank < current.Score.Rerank {
			t.Fatalf("a lower rerank score came after a higher one: %+v then %+v", previous, current)
		}
		if previous.Score.Rerank == current.Score.Rerank && previous.Score.Combined < current.Score.Combined {
			t.Fatalf("equal rerank, and a lower combined score came second: %+v then %+v", previous, current)
		}
	}
	// Ranks are assigned from the final order, so a non-monotonic rank would mean
	// the slice was sorted after numbering.
	for index, citation := range fused {
		if citation.Rank != index+1 {
			t.Fatalf("the passage at position %d carries rank %d", index, citation.Rank)
		}
	}
}
