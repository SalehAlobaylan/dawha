package embeddingbackfill

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// providerStub returns whatever the test tells it to, including the three things
// a real provider must never return. Each of them has to produce a refusal
// rather than a row, because a row with a 16-dimension vector, a NaN in it or a
// zero norm is a row that reads as embedded and scores nothing.
type providerStub struct {
	embedding []float64
	model     string
	err       error
	calls     int
	texts     []string
}

func (p *providerStub) NormalizeName(context.Context, string) (ai.NameNormalization, error) {
	return ai.NameNormalization{}, nil
}

func (p *providerStub) Embed(_ context.Context, input ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	p.calls++
	p.texts = append(p.texts, input.Text)
	if p.err != nil {
		return ai.EmbeddingResponse{}, p.err
	}
	return ai.EmbeddingResponse{Embedding: p.embedding, Dimensions: input.Dimensions, Model: p.model}, nil
}

func (p *providerStub) Classify(context.Context, ai.ClassificationRequest) (ai.ClassificationResponse, error) {
	return ai.ClassificationResponse{}, nil
}

func (p *providerStub) ExtractEntities(context.Context, ai.ExtractionRequest) (ai.EntityExtractionResponse, error) {
	return ai.EntityExtractionResponse{}, nil
}

func (p *providerStub) ExtractClaims(context.Context, ai.ExtractionRequest) (ai.ClaimExtractionResponse, error) {
	return ai.ClaimExtractionResponse{}, nil
}

func (p *providerStub) ResolveEntity(context.Context, ai.EntityResolutionRequest) (ai.EntityResolutionResponse, error) {
	return ai.EntityResolutionResponse{}, nil
}

func (p *providerStub) AnalyzeContradiction(context.Context, ai.ContradictionRequest) (ai.ContradictionResponse, error) {
	return ai.ContradictionResponse{}, nil
}

func (p *providerStub) Rerank(context.Context, ai.RerankRequest) (ai.RerankResponse, error) {
	return ai.RerankResponse{}, nil
}

func (p *providerStub) ResearchQuery(context.Context, ai.ResearchQueryRequest) (ai.ResearchQueryResponse, error) {
	return ai.ResearchQueryResponse{}, nil
}

// validVector is a normalized-width, finite, non-zero vector: the only shape this
// package is willing to store.
func validVector(dimensions int) []float64 {
	values := make([]float64, dimensions)
	for index := range values {
		// A walk around the unit circle, so the norm is 1 and no component is
		// zero for a width above 4.
		angle := 2 * math.Pi * float64(index) / float64(dimensions)
		values[index] = math.Cos(angle)
	}
	return values
}

// seedBackfillCorpus writes one source, two passages and their accepted
// statements, with no embeddings, and returns the passage ids.
//
// The two are not the whole seeded scope: testsupport's fixture already carries
// db/seeds/001_demo.sql, whose three passages are unembedded too. That is
// deliberate - the assertions below are about the rows this test created, so they
// are written against these ids and against deltas rather than against totals
// that another file's seed also contributes to. The three demo passages starting
// unembedded is the exact state docs/phase-status.md described, so a run here
// also reproduces the bug being fixed.
func seedBackfillCorpus(t *testing.T, fixture *testsupport.Fixture) (owned []uuid.UUID) {
	t.Helper()
	sourceID := uuid.New()
	first, second := uuid.New(), uuid.New()
	owned = []uuid.UUID{first, second}
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata) VALUES ($1, 'مصدر قياس', 'book', 'public', '{"synthetic":true}')`, sourceID)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES
		($1, $3, 1, 'يسجل العقد ان الوكيلين تصرفا في مال الغائب برضاه', 'يسجل العقد ان الوكيلين تصرفا في مال الغائب برضاه'),
		($2, $3, 2, 'يفصل القاضي بين دعوي المطالبتين ويؤجل الحكم', 'يفصل القاضي بين دعوي المطالبتين ويؤجل الحكم')`, first, second, sourceID)
	for _, id := range []uuid.UUID{first, second} {
		fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, extraction_method, review_status) VALUES ($1, $2, $3, 'عبارة مقبولة', 'manual', 'accepted')`, uuid.New(), sourceID, id)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE id = ANY($1::uuid[]) AND embedding IS NOT NULL`, owned); got != 0 {
		t.Fatalf("the fixture started with %d of these passages embedded, wanted 0", got)
	}
	return owned
}

func TestRunEmbedsTheWholeScopeAndIsIdempotent(t *testing.T) {
	fixture := testsupport.New(t)
	own := seedBackfillCorpus(t, fixture)
	provider := &providerStub{embedding: validVector(Dimensions), model: "stub-v1"}
	service := New(fixture.Pool(), provider)
	ctx := fixture.Ctx()

	first, err := service.Run(ctx, SeedScope())
	if err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if first.Before.Missing != first.Before.Total {
		t.Fatalf("the scope started with %d of %d passages already embedded, wanted none", first.Before.Embedded, first.Before.Total)
	}
	if first.Written != first.Before.Missing || first.Verified != first.Written {
		t.Fatalf("the first run wrote %d and verified %d for a scope of %d missing, wanted them equal", first.Written, first.Verified, first.Before.Missing)
	}
	if first.After.Missing != 0 || first.After.Embedded != first.After.Total {
		t.Fatalf("%d of %d passages carry an embedding after the run", first.After.Embedded, first.After.Total)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE id = ANY($1::uuid[]) AND embedding IS NOT NULL AND vector_dims(embedding) = $2`, own, Dimensions); got != len(own) {
		t.Fatalf("%d of this test's 2 passages hold a %d-dimension vector after the run", got, Dimensions)
	}
	// The text that got embedded is a stored normalized_text_ar, never a raw
	// text_ar. That is the whole reason the two columns have to agree: a vector
	// built from the raw text is one the query path cannot reproduce, because a
	// query is normalized before it is embedded.
	for _, text := range provider.texts {
		if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE normalized_text_ar = $1`, text); got != 1 {
			t.Fatalf("embedded %q, which is not exactly one stored normalized_text_ar", text)
		}
		if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE text_ar = $1 AND normalized_text_ar <> $1`, text); got != 0 {
			t.Fatalf("embedded %q, which is a raw text_ar rather than its normalized form", text)
		}
	}

	callsAfterFirst := provider.calls
	second, err := service.Run(ctx, SeedScope())
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if second.Written != 0 {
		t.Fatalf("the second run wrote %d rows, wanted 0: the backfill is not idempotent", second.Written)
	}
	if provider.calls != callsAfterFirst {
		t.Fatalf("the provider was called %d more times on a rerun: a rerun re-embedded rows that already had a vector", provider.calls-callsAfterFirst)
	}
	if second.After.Embedded != second.After.Total {
		t.Fatalf("%d of %d passages carry an embedding after the second run", second.After.Embedded, second.After.Total)
	}
}

// TestRunRefusesAVectorItCannotTrust is the loudness half. A vector that is the
// wrong width, non-finite or zero-length must stop the run with a row unwritten,
// because a row that looks embedded and cannot be compared is the failure this
// command exists to prevent: the measurement then comes out wrong and reports
// that it passed.
func TestRunRefusesAVectorItCannotTrust(t *testing.T) {
	notFinite := validVector(Dimensions)
	notFinite[7] = math.NaN()
	zeroNorm := make([]float64, Dimensions)

	cases := []struct {
		name    string
		stub    *providerStub
		wantsIn string
	}{
		{
			name:    "wrong dimension",
			stub:    &providerStub{embedding: validVector(Dimensions / 2)},
			wantsIn: "dimensions",
		},
		{
			name:    "not finite",
			stub:    &providerStub{embedding: notFinite},
			wantsIn: "non-finite",
		},
		{
			name:    "zero length",
			stub:    &providerStub{embedding: zeroNorm},
			wantsIn: "zero-length",
		},
		{
			name:    "provider failed",
			stub:    &providerStub{err: errors.New("ai unavailable")},
			wantsIn: "ai unavailable",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := testsupport.New(t)
			own := seedBackfillCorpus(t, fixture)
			service := New(fixture.Pool(), testCase.stub)
			_, err := service.Run(fixture.Ctx(), SeedScope())
			if err == nil {
				t.Fatal("the run succeeded on a vector it should have refused")
			}
			if !contains(err.Error(), testCase.wantsIn) {
				t.Fatalf("the failure %q does not name %q, so a reader cannot tell what was wrong", err, testCase.wantsIn)
			}
			if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE id = ANY($1::uuid[]) AND embedding IS NOT NULL`, own); got != 0 {
				t.Fatalf("%d of this test's passage(s) were written despite the refusal, wanted 0", got)
			}
		})
	}
}

// TestRunRefusesAPassageWhoseTextColumnsDisagree is the seed defect this command
// found in the tree: `normalized_text_ar` is the column a query is compared
// against and the worker embeds, so a value the normalizer would not produce is
// a row whose matchability nobody can state.
func TestRunRefusesAPassageWhoseTextColumnsDisagree(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID := uuid.New()
	disagreeing := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata) VALUES ($1, 'مصدر قياس', 'book', 'public', '{"synthetic":true}')`, sourceID)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'يذكر النص أن عبدالله يتصل بمحمد بن سعد، مع إشارات إلى رواية أخرى.', 'يذكر النص ان عبدالله يتصل بمحمد بن سعد مع اشاره الى روايه اخري')`, disagreeing, sourceID)

	provider := &providerStub{embedding: validVector(Dimensions)}
	service := New(fixture.Pool(), provider)
	_, err := service.Run(fixture.Ctx(), SeedScope())
	if err == nil {
		t.Fatal("the run embedded a passage whose text_ar and normalized_text_ar disagree")
	}
	for _, wanted := range []string{"normalized_text_ar", "اشاره الى", "اشارات الي"} {
		if !contains(err.Error(), wanted) {
			t.Fatalf("the failure %q does not quote %q, so a reviewer cannot see which value is wrong", err, wanted)
		}
	}
	// The scope also holds the fixture's own seeded passages, which embed fine, so
	// the assertion is about the row that was refused rather than about the run
	// having made no provider call at all.
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE id = $1 AND embedding IS NOT NULL`, disagreeing); got != 0 {
		t.Fatal("the passage whose two text columns disagree was embedded anyway")
	}
	for _, text := range provider.texts {
		if strings.Contains(text, "اشاره") {
			t.Fatalf("the disagreeing text %q reached the provider, so it was embedded before anybody checked it", text)
		}
	}
}

// TestSeedScopeLeavesUnsownPassagesAlone is the reason the default scope is a
// marker rather than everything: a developer with real processed sources in the
// database should be able to run the seeded-corpus backfill without their own
// rows being rewritten.
func TestSeedScopeLeavesUnsownPassagesAlone(t *testing.T) {
	fixture := testsupport.New(t)
	seedBackfillCorpus(t, fixture)
	ownedSourceID := uuid.New()
	owned := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility) VALUES ($1, 'مصدر حقيقي', 'book', 'public')`, ownedSourceID)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, 'نص خاص بمصدر لم يسمه البذور', 'نص خاص بمصدر لم يسمه البذور')`, owned, ownedSourceID)
	fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, extraction_method, review_status) VALUES ($1, $2, $3, 'عبارة مقبولة', 'manual', 'accepted')`, uuid.New(), ownedSourceID, owned)

	service := New(fixture.Pool(), &providerStub{embedding: validVector(Dimensions)})
	seeded, err := service.Run(fixture.Ctx(), SeedScope())
	if err != nil {
		t.Fatalf("seeded-scope backfill: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE id = $1 AND embedding IS NOT NULL`, owned); got != 0 {
		t.Fatal("the seeded-scope backfill embedded a passage the seed did not create")
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE id = $1 AND embedding IS NOT NULL`, owned); got != 0 {
		t.Fatal("the seeded-scope backfill embedded a passage the seed did not create")
	}
	if seeded.After.Missing != 0 {
		t.Fatalf("the seeded scope still has %d unembedded passages after the run", seeded.After.Missing)
	}

	everything, err := service.Run(fixture.Ctx(), AllScope())
	if err != nil {
		t.Fatalf("all-scope backfill: %v", err)
	}
	if everything.Written != 1 {
		t.Fatalf("the all scope wrote %d rows, wanted the 1 the seed scope left", everything.Written)
	}
}

func TestCoverageCountsTheScope(t *testing.T) {
	fixture := testsupport.New(t)
	own := seedBackfillCorpus(t, fixture)
	service := New(fixture.Pool(), &providerStub{embedding: validVector(Dimensions)})
	scope := SeedScope()

	empty, err := service.Coverage(fixture.Ctx(), scope)
	if err != nil {
		t.Fatalf("coverage of an untouched scope: %v", err)
	}
	if empty.Embedded != 0 || empty.Missing != empty.Total {
		t.Fatalf("coverage reported %d of %d embedded before any run, wanted none", empty.Embedded, empty.Total)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE id = ANY($1::uuid[])`, own); got != len(own) {
		t.Fatalf("this test wrote %d passages, wanted %d", got, len(own))
	}
	if _, err := service.Run(fixture.Ctx(), scope); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	filled, err := service.Coverage(fixture.Ctx(), scope)
	if err != nil {
		t.Fatalf("coverage after the backfill: %v", err)
	}
	if filled.Total != empty.Total || filled.Embedded != filled.Total || filled.Missing != 0 {
		t.Fatalf("coverage went from %d/%d embedded to %d/%d, wanted the same total fully embedded", empty.Embedded, empty.Total, filled.Embedded, filled.Total)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
