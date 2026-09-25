package search

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

func TestValidateInputDefaults(t *testing.T) {
	input, normalized, vector, err := validateInput(Input{Query: "  عَبْدُ الله  "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Limit != 20 || normalized != "عبد الله" || vector != nil {
		t.Fatalf("unexpected normalized input: %+v, %q, %v", input, normalized, vector)
	}
}

func TestValidateInputRejectsInvalidFilters(t *testing.T) {
	if _, _, _, err := validateInput(Input{Query: "سؤال", Kind: "unknown"}); err != ErrValidation {
		t.Fatalf("expected kind validation error, got %v", err)
	}
	if _, _, _, err := validateInput(Input{Query: "سؤال", PersonID: "not-a-uuid"}); err != ErrValidation {
		t.Fatalf("expected id validation error, got %v", err)
	}
	if _, _, _, err := validateInput(Input{Query: "سؤال", Embedding: "[1,2]"}); err != ErrValidation {
		t.Fatalf("expected embedding validation error, got %v", err)
	}
	if _, _, _, err := validateInput(Input{Query: "!!!", Kind: "semantic"}); err != ErrValidation {
		t.Fatalf("expected normalized semantic query validation error, got %v", err)
	}
	zero, _ := json.Marshal(make([]float64, 1536))
	if _, _, _, err := validateInput(Input{Query: "سؤال", Embedding: string(zero)}); err != ErrValidation {
		t.Fatalf("expected zero embedding validation error, got %v", err)
	}
}

func TestAppendGroupSkipsEmptyResults(t *testing.T) {
	groups := appendGroup(nil, "names", "الأسماء", nil)
	if len(groups) != 0 {
		t.Fatalf("expected empty groups to be skipped: %#v", groups)
	}
	groups = appendGroup(groups, "names", "الأسماء", []Result{{ID: "1"}})
	if len(groups) != 1 || groups[0].Key != "names" {
		t.Fatalf("unexpected groups: %#v", groups)
	}
}

type embeddingStub struct {
	response ai.EmbeddingResponse
	err      error
	requests []ai.EmbeddingRequest
}

func (s *embeddingStub) Embed(_ context.Context, input ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	s.requests = append(s.requests, input)
	return s.response, s.err
}

func TestEmbedQueryUsesNormalizedArabic(t *testing.T) {
	stub := &embeddingStub{response: ai.EmbeddingResponse{Embedding: testEmbedding(), Dimensions: 1536, Model: "test-embed"}}
	service := &Service{AI: stub}
	vector, model, err := service.embedQuery(context.Background(), "عبد الله")
	if err != nil {
		t.Fatal(err)
	}
	if model != "test-embed" || len(vector.Slice()) != 1536 {
		t.Fatalf("unexpected embedding result: model=%q dimensions=%d", model, len(vector.Slice()))
	}
	if len(stub.requests) != 1 || stub.requests[0].Text != "عبد الله" || stub.requests[0].Dimensions != 1536 {
		t.Fatalf("unexpected embedding request: %+v", stub.requests)
	}
}

func TestEmbedQueryRejectsUnavailableOrInvalidProvider(t *testing.T) {
	service := &Service{AI: &embeddingStub{err: ai.ErrUnavailable}}
	if _, _, err := service.embedQuery(context.Background(), "نص"); err != ErrAIUnavailable {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	service = &Service{AI: &embeddingStub{response: ai.EmbeddingResponse{Embedding: []float64{1}, Dimensions: 1, Model: "bad"}}}
	if _, _, err := service.embedQuery(context.Background(), "نص"); err != ErrAIUnavailable {
		t.Fatalf("expected invalid embedding error, got %v", err)
	}
	service = &Service{AI: &embeddingStub{response: ai.EmbeddingResponse{Embedding: make([]float64, 1536), Dimensions: 1536, Model: "zero"}}}
	if _, _, err := service.embedQuery(context.Background(), "نص"); err != ErrAIUnavailable {
		t.Fatalf("expected zero embedding error, got %v", err)
	}
}

func TestSemanticSearchUsesPublicProvenance(t *testing.T) {
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

	publicSourceID := uuid.New()
	privateSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, dependency_status, publication_date_from, publication_date_to) VALUES ($1, 'مصدر عام', 'book', 'public', 'independent', '1200-01-01', '1300-01-01'), ($2, 'مصدر خاص', 'book', 'private', 'independent', '1200-01-01', '1300-01-01')`, publicSourceID, privateSourceID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, []uuid.UUID{publicSourceID, privateSourceID})
	claimID := uuid.New()
	personID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status) VALUES ($1, 'person', $2, 'parent_of', 'person', $3, 'supported')`, claimID, personID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM claims WHERE id = $1`, claimID)

	publicPassageID := uuid.New()
	unreviewedPassageID := uuid.New()
	privatePassageID := uuid.New()
	publicVector := make([]float32, 1536)
	publicVector[0] = 1
	privateVector := make([]float32, 1536)
	privateVector[1] = 1
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar, embedding) VALUES ($1, $3, 1, 7, 'صفحة ٧', 'مقطع عام', 'مقطع عام', $5), ($2, $3, 2, 8, 'صفحة ٨', 'مقطع غير مراجَع', 'مقطع غير مراجَع', $5), ($4, $6, 1, 9, 'صفحة ٩', 'مقطع خاص', 'مقطع خاص', $7)`, publicPassageID, unreviewedPassageID, publicSourceID, privatePassageID, pgvector.NewVector(publicVector), privateSourceID, pgvector.NewVector(privateVector)); err != nil {
		t.Fatal(err)
	}
	statementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status) VALUES ($1, $2, $3, 'عبارة مقبولة', 'accepted')`, statementID, publicSourceID, publicPassageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_passage_id, source_statement_id, relation) VALUES ($1, $2, $3, 'supports')`, claimID, publicPassageID, statementID); err != nil {
		t.Fatal(err)
	}
	malformedClaimID := uuid.New()
	malformedPersonID := uuid.New()
	malformedStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status) VALUES ($1, 'person', $2, 'parent_of', 'person', $3, 'supported')`, malformedClaimID, malformedPersonID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM claims WHERE id = $1`, malformedClaimID)
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status) VALUES ($1, $2, $3, 'عبارة خاصة غير متسقة', 'accepted')`, malformedStatementID, privateSourceID, publicPassageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_passage_id, source_statement_id, relation) VALUES ($1, $2, $3, 'supports')`, malformedClaimID, publicPassageID, malformedStatementID); err != nil {
		t.Fatal(err)
	}

	stub := &embeddingStub{response: ai.EmbeddingResponse{Embedding: testEmbedding(), Dimensions: 1536, Model: "test-embed"}}
	service := &Service{Pool: pool, AI: stub}
	response, err := service.Search(ctx, Input{Query: "  عَبْدُ الله  ", Kind: "semantic", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if response.EmbeddingModel != "test-embed" || len(response.Groups) != 1 || response.Groups[0].Key != "semantic" {
		t.Fatalf("unexpected semantic response: %+v", response)
	}
	if len(stub.requests) != 1 || stub.requests[0].Text != "عبد الله" {
		t.Fatalf("semantic query was not normalized: %+v", stub.requests)
	}
	if len(response.Groups[0].Items) != 2 {
		t.Fatalf("expected two public passages, got %+v", response.Groups[0].Items)
	}
	var accepted, unreviewed bool
	for _, item := range response.Groups[0].Items {
		if item.SourceID == privateSourceID.String() || item.PassageID == privatePassageID.String() {
			t.Fatalf("private passage leaked: %+v", item)
		}
		if item.PassageID == publicPassageID.String() {
			if item.StatementID != statementID.String() || item.ReviewStatus != "accepted" || item.PageNumber == nil || *item.PageNumber != 7 || item.LocatorAR != "صفحة ٧" || item.DependencyStatus != "independent" {
				t.Fatalf("missing passage provenance: %+v", item)
			}
			accepted = true
		}
		if item.PassageID == unreviewedPassageID.String() && item.ReviewStatus == "unreviewed" {
			unreviewed = true
		}
	}
	if !accepted || !unreviewed {
		t.Fatalf("review provenance was not preserved: %+v", response.Groups[0].Items)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status) VALUES ($1, $2, 'cites', 'needs_review')`, publicSourceID, privateSourceID); err != nil {
		t.Fatal(err)
	}
	dependencyResponse, err := service.Search(ctx, Input{Query: "عبد الله", Kind: "semantic", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if dependencyResponse.Groups[0].Items[0].DependencyStatus != "likely_dependent" {
		t.Fatalf("needs-review dependency was not reflected: %+v", dependencyResponse.Groups[0].Items[0])
	}
	if _, err := pool.Exec(ctx, `UPDATE source_dependencies SET status = 'confirmed' WHERE source_id = $1 AND depends_on_source_id = $2`, publicSourceID, privateSourceID); err != nil {
		t.Fatal(err)
	}
	confirmedResponse, err := service.Search(ctx, Input{Query: "عبد الله", Kind: "semantic", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if confirmedResponse.Groups[0].Items[0].DependencyStatus != "derived" {
		t.Fatalf("confirmed dependency was not reflected: %+v", confirmedResponse.Groups[0].Items[0])
	}

	filtered, err := service.Search(ctx, Input{Query: "عبد الله", Kind: "semantic", PersonID: personID.String(), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Groups) != 1 || len(filtered.Groups[0].Items) != 1 || filtered.Groups[0].Items[0].PassageID != publicPassageID.String() {
		t.Fatalf("person filter was not applied: %+v", filtered.Groups)
	}
	malformedFiltered, err := service.Search(ctx, Input{Query: "عبد الله", Kind: "semantic", PersonID: malformedPersonID.String(), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(malformedFiltered.Groups) != 0 {
		t.Fatalf("inconsistent private evidence influenced public results: %+v", malformedFiltered.Groups)
	}
	fromFiltered, err := service.Search(ctx, Input{Query: "عبد الله", Kind: "semantic", FromYear: 1250, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(fromFiltered.Groups) != 1 || len(fromFiltered.Groups[0].Items) != 2 {
		t.Fatalf("from-year filter was not applied: %+v", fromFiltered.Groups)
	}
	toFiltered, err := service.Search(ctx, Input{Query: "عبد الله", Kind: "semantic", ToYear: 1150, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(toFiltered.Groups) != 0 {
		t.Fatalf("out-of-range to-year filter returned results: %+v", toFiltered.Groups)
	}
}

func testEmbedding() []float64 {
	values := make([]float64, 1536)
	values[0] = 1
	return values
}
