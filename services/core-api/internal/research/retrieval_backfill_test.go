package research

// The retrieval legs need data to score, and until plan 014 the seeded corpus
// had none: db/seeds/ is pure SQL, and the only writer of
// `source_passages.embedding` is the source-processing worker, which cannot be
// pointed at seeded passages (it resolves work through `source_files` and
// INSERTs a passage per extracted page). So `retrieveVectorPassages` had never
// returned anything in this repository, every test and demo had silently run the
// lexical leg alone, and docs/phase-status.md recorded the result as "there is
// no embedding retrieval in the query path" - which was wrong about the code and
// right about the data.
//
// These tests are the assertion half of the fix: after internal/embeddingbackfill
// has run, the vector leg returns results for a real Arabic query. They are in
// the ordinary acceptance suite, so a change that empties the column again - a
// seed that stops carrying the corpus, a backfill that stops verifying what it
// wrote, a migration that drops the column - fails a gate rather than waiting
// for somebody to notice that a benchmark has gone quiet.

import (
	"context"
	"math"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/embeddingbackfill"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// embedProviderStub returns a fixed-width deterministic vector derived from the
// text it is given, so the vector leg has something to rank by without a running
// AI service. It is not a semantic embedding and does not pretend to be one: the
// point of these tests is that the column is populated and the leg reads it, not
// that a stub can judge Arabic.
type embedProviderStub struct {
	dimensions int
	calls      int
}

func (s *embedProviderStub) NormalizeName(context.Context, string) (ai.NameNormalization, error) {
	return ai.NameNormalization{}, nil
}

func (s *embedProviderStub) Embed(_ context.Context, input ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	s.calls++
	values := make([]float64, input.Dimensions)
	for index := range values {
		angle := 2 * math.Pi * float64(index+len(input.Text)) / float64(input.Dimensions)
		values[index] = math.Cos(angle)
	}
	return ai.EmbeddingResponse{Embedding: values, Dimensions: input.Dimensions, Model: "backfill-test-stub"}, nil
}

func (s *embedProviderStub) Classify(context.Context, ai.ClassificationRequest) (ai.ClassificationResponse, error) {
	return ai.ClassificationResponse{}, nil
}

func (s *embedProviderStub) ExtractEntities(context.Context, ai.ExtractionRequest) (ai.EntityExtractionResponse, error) {
	return ai.EntityExtractionResponse{}, nil
}

func (s *embedProviderStub) ExtractClaims(context.Context, ai.ExtractionRequest) (ai.ClaimExtractionResponse, error) {
	return ai.ClaimExtractionResponse{}, nil
}

func (s *embedProviderStub) ResolveEntity(context.Context, ai.EntityResolutionRequest) (ai.EntityResolutionResponse, error) {
	return ai.EntityResolutionResponse{}, nil
}

func (s *embedProviderStub) AnalyzeContradiction(context.Context, ai.ContradictionRequest) (ai.ContradictionResponse, error) {
	return ai.ContradictionResponse{}, nil
}

func (s *embedProviderStub) Rerank(_ context.Context, request ai.RerankRequest) (ai.RerankResponse, error) {
	documents := make([]ai.RerankedDocument, 0, len(request.Documents))
	for _, document := range request.Documents {
		documents = append(documents, ai.RerankedDocument{ID: document.ID, Score: 1, Excerpt: document.Text})
	}
	return ai.RerankResponse{Documents: documents}, nil
}

func (s *embedProviderStub) ResearchQuery(context.Context, ai.ResearchQueryRequest) (ai.ResearchQueryResponse, error) {
	return ai.ResearchQueryResponse{}, nil
}

// TestVectorLegReturnsResultsAfterTheBackfill is the test the plan asks for: the
// seeded corpus carries embeddings after the backfill, and the vector leg returns
// a non-empty result for a real query.
func TestVectorLegReturnsResultsAfterTheBackfill(t *testing.T) {
	fixture := testsupport.New(t)
	ctx := fixture.Ctx()
	provider := &embedProviderStub{dimensions: embeddingbackfill.Dimensions}

	before := fixture.Count(`SELECT count(*) FROM source_passages WHERE embedding IS NOT NULL`)
	if before != 0 {
		t.Fatalf("the fixture's seeded corpus already had %d embedded passages, so this test is not measuring the backfill", before)
	}

	result, err := embeddingbackfill.New(fixture.Pool(), provider).Run(ctx, embeddingbackfill.SeedScope())
	if err != nil {
		t.Fatalf("backfill the seeded corpus: %v", err)
	}
	if result.Written != result.Before.Total || result.After.Missing != 0 {
		t.Fatalf("the backfill left %d of %d passages unembedded", result.After.Missing, result.After.Total)
	}
	// Every stored vector has the width the column declares. A vector of the
	// wrong width would make `<=>` raise, and the leg would return an error
	// rather than a wrong answer - but only once somebody ran a query.
	wrongWidth := fixture.Count(`SELECT count(*) FROM source_passages WHERE embedding IS NOT NULL AND vector_dims(embedding) <> $1`, embeddingbackfill.Dimensions)
	if wrongWidth != 0 {
		t.Fatalf("%d passages hold a vector that is not %d dimensions", wrongWidth, embeddingbackfill.Dimensions)
	}

	// A real Arabic question, normalized the way the query path normalizes it.
	const question = "من كان والد عبدالله بن محمد؟"
	normalized := identity.NormalizeArabicName(question)
	vector := providerVector(t, provider, normalized)

	service := &Service{Pool: fixture.Pool(), AI: provider}
	results, err := service.retrieveVectorPassages(ctx, retrievalContext{Input: QueryInput{Question: question}, Normalized: normalized, Vector: vector})
	if err != nil {
		t.Fatalf("the vector leg over an embedded corpus: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("the vector leg returned nothing for a real query over a corpus that carries embeddings: it is still running lexical-only, or reading a different column")
	}
	for _, citation := range results {
		if citation.PassageID == "" {
			t.Fatalf("a vector result carries no passage id, so a reader cannot open it: %+v", citation)
		}
		if citation.Score.Vector <= 0 {
			t.Fatalf("a result was returned with vector score %v, and the query filters on `vector_score > 0`", citation.Score.Vector)
		}
	}

	// The same leg on the same corpus before the backfill returned nothing at
	// all. That is the bug this whole plan is about, so it is pinned rather than
	// described: if `retrieveVectorPassages` ever stops needing the column, the
	// assertion above is measuring something else and this comparison is what
	// says so.
	if _, err := fixture.Pool().Exec(ctx, `UPDATE source_passages SET embedding = NULL`); err != nil {
		t.Fatalf("clear the embeddings again: %v", err)
	}
	empty, err := service.retrieveVectorPassages(ctx, retrievalContext{Input: QueryInput{Question: question}, Normalized: normalized, Vector: vector})
	if err != nil {
		t.Fatalf("the vector leg over an unembedded corpus: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("the vector leg returned %d results with no embedded passage to score, so the column is not what it reads", len(empty))
	}
}

// TestHybridPathScoresBothLegsSeparately pins the other half of the plan's
// premise: the hybrid path persists lexical, vector and rerank scores separately,
// so a difference between the legs is visible in the evidence rather than
// averaged away before anybody can see it.
func TestHybridPathScoresBothLegsSeparately(t *testing.T) {
	fixture := testsupport.New(t)
	ctx := fixture.Ctx()
	provider := &embedProviderStub{dimensions: embeddingbackfill.Dimensions}
	if _, err := embeddingbackfill.New(fixture.Pool(), provider).Run(ctx, embeddingbackfill.SeedScope()); err != nil {
		t.Fatalf("backfill the seeded corpus: %v", err)
	}

	const question = "من كان والد عبدالله؟"
	normalized := identity.NormalizeArabicName(question)
	vector := providerVector(t, provider, normalized)
	retrieval := retrievalContext{Input: QueryInput{Question: question}, Normalized: normalized, Vector: vector}

	service := &Service{Pool: fixture.Pool(), AI: provider}
	lexical, err := service.retrieveLexicalPassages(ctx, retrieval)
	if err != nil {
		t.Fatalf("lexical leg: %v", err)
	}
	byVector, err := service.retrieveVectorPassages(ctx, retrieval)
	if err != nil {
		t.Fatalf("vector leg: %v", err)
	}
	if len(lexical) == 0 || len(byVector) == 0 {
		t.Fatalf("the two legs returned %d lexical and %d vector candidates; the measurement this repository needed needs both to be non-empty", len(lexical), len(byVector))
	}
	fused, stats, err := service.fuseAndRerank(ctx, retrieval, lexical, byVector)
	if err != nil {
		t.Fatalf("fuse and rerank: %v", err)
	}
	if stats.LexicalCandidates != len(lexical) || stats.VectorCandidates != len(byVector) {
		t.Fatalf("fusion reported %d lexical and %d vector candidates, wanted %d and %d", stats.LexicalCandidates, stats.VectorCandidates, len(lexical), len(byVector))
	}
	bothLegs := 0
	for _, citation := range fused {
		if citation.Score.Lexical > 0 && citation.Score.Vector > 0 {
			bothLegs++
			// The shipped weights, asserted rather than assumed: a change to them
			// is a change to what a reader is shown as evidence, and it has to be
			// a deliberate one.
			want := citation.Score.Lexical*0.55 + citation.Score.Vector*0.45
			if math.Abs(citation.Score.Combined-want) > 1e-9 {
				t.Fatalf("a passage scored by both legs has combined %v, which is not 0.55*%v + 0.45*%v", citation.Score.Combined, citation.Score.Lexical, citation.Score.Vector)
			}
		}
	}
	if bothLegs == 0 {
		t.Fatal("no passage was scored by both legs, so the fusion weights were not exercised")
	}
}

// providerVector is the vector the stub would hand back for a piece of text,
// obtained by asking it rather than by recomputing it here: a test that computed
// the vector itself would keep passing if the stub and the query path disagreed
// about how a query vector is made.
func providerVector(t *testing.T, provider *embedProviderStub, normalized string) []float32 {
	t.Helper()
	response, err := provider.Embed(context.Background(), ai.EmbeddingRequest{Text: normalized, Dimensions: embeddingbackfill.Dimensions})
	if err != nil {
		t.Fatalf("build a query vector: %v", err)
	}
	values := make([]float32, len(response.Embedding))
	for index, value := range response.Embedding {
		values[index] = float32(value)
	}
	if len(values) != embeddingbackfill.Dimensions {
		t.Fatalf("the stub returned %d dimensions, wanted %d", len(values), embeddingbackfill.Dimensions)
	}
	return values
}

// vectorLegCorpusCoverage is the sentence the report quotes: how much of a scope
// the vector leg can even see. It is a helper rather than a test so the
// measurement command and the tests count the same rows the same way.
func vectorLegCorpusCoverage(ctx context.Context, pool *pgxpool.Pool) (total, embedded int, err error) {
	err = pool.QueryRow(ctx, `
		SELECT count(*), count(sp.embedding)
		FROM source_passages sp
		JOIN sources src ON src.id = sp.source_id
		JOIN LATERAL (
			SELECT id FROM source_statements
			WHERE source_passage_id = sp.id AND review_status = 'accepted'
			ORDER BY created_at LIMIT 1
		) ss ON TRUE
		WHERE COALESCE(src.metadata->>'synthetic', 'false') = 'true'
	`).Scan(&total, &embedded)
	return total, embedded, err
}

func TestVectorLegCorpusCoverageCountsOnlyRetrievablePassages(t *testing.T) {
	fixture := testsupport.New(t)
	total, embedded, err := vectorLegCorpusCoverage(fixture.Ctx(), fixture.Pool())
	if err != nil {
		t.Fatalf("count the retrievable corpus: %v", err)
	}
	if total == 0 {
		t.Fatal("the seeded corpus has no passage with an accepted statement, so neither leg can return anything")
	}
	if embedded != 0 {
		t.Fatalf("%d of %d retrievable passages were already embedded, so the backfill test would not be measuring a backfill", embedded, total)
	}
	// A passage with no accepted statement is invisible to both legs
	// (`WHERE ... AND ss.id IS NOT NULL`), so it must not be counted as corpus.
	retrievable := fixture.Count(`SELECT count(*) FROM source_passages sp JOIN sources src ON src.id = sp.source_id JOIN LATERAL (SELECT id FROM source_statements WHERE source_passage_id = sp.id AND review_status = 'accepted' ORDER BY created_at LIMIT 1) ss ON TRUE WHERE COALESCE(src.metadata->>'synthetic','false') = 'true'`)
	allPassages := fixture.Count(`SELECT count(*) FROM source_passages sp JOIN sources src ON src.id = sp.source_id WHERE COALESCE(src.metadata->>'synthetic','false') = 'true'`)
	if retrievable >= allPassages {
		t.Fatalf("the coverage helper counted %d of %d passages as retrievable, so it is not applying the accepted-statement rule the legs apply", retrievable, allPassages)
	}
	if total != retrievable {
		t.Fatalf("the coverage helper reported %d retrievable passages and the direct count says %d", total, retrievable)
	}
}

// TestRetrievalCorpusIsNormalizedByTheSameFunction is the assertion behind a
// sentence in both seed files.
//
// `source_passages.normalized_text_ar` is what both passage legs compare a
// normalized query against, and what the embedding backfill embeds. It therefore
// has to be exactly what `identity.NormalizeArabicName(text_ar)` produces, and it
// is hand-written in SQL, where a hand-written value is a plausible thing to get
// subtly wrong: the demo seed said "اشاره الى" where the normalizer gives
// "اشارات الي", which is a row nothing could match and a vector built from a
// spelling no query can carry. `cmd/embedding-backfill` refuses to embed such a
// passage, which is how it was found; this test is what stops it coming back.
//
// It runs over the fixture, so it covers db/seeds/001_demo.sql on every
// `make verify` with a database. The measurement run checks the same invariant
// over the whole seeded corpus including 002_retrieval_corpus.sql, because that
// is the one the labels are written against.
func TestRetrievalCorpusIsNormalizedByTheSameFunction(t *testing.T) {
	fixture := testsupport.New(t)
	rows, err := fixture.Pool().Query(fixture.Ctx(), `SELECT id::text, text_ar, normalized_text_ar FROM source_passages ORDER BY id`)
	if err != nil {
		t.Fatalf("read the seeded passages: %v", err)
	}
	defer rows.Close()
	checked := 0
	for rows.Next() {
		var id, text, normalized string
		if err := rows.Scan(&id, &text, &normalized); err != nil {
			t.Fatalf("scan a seeded passage: %v", err)
		}
		checked++
		if want := identity.NormalizeArabicName(text); want != normalized {
			t.Errorf("passage %s has normalized_text_ar %q, and identity.NormalizeArabicName(%q) is %q. A passage whose normalized column is not what the normalizer produces is one no query can match and no vector can be compared against", id, normalized, text, want)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the seeded passages: %v", err)
	}
	if checked == 0 {
		t.Fatal("the fixture holds no seeded passages, so this test checked nothing")
	}
	t.Logf("%d seeded passages carry the normalizer's own output", checked)
}
