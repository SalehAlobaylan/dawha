package research

import (
	"context"
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
)

type researchProviderStub struct {
	rerank        ai.RerankResponse
	contradiction ai.ContradictionResponse
}

func (researchProviderStub) NormalizeName(context.Context, string) (ai.NameNormalization, error) {
	return ai.NameNormalization{}, nil
}

func (researchProviderStub) Embed(context.Context, ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	return ai.EmbeddingResponse{}, nil
}

func (researchProviderStub) Classify(context.Context, ai.ClassificationRequest) (ai.ClassificationResponse, error) {
	return ai.ClassificationResponse{}, nil
}

func (researchProviderStub) ExtractEntities(context.Context, ai.ExtractionRequest) (ai.EntityExtractionResponse, error) {
	return ai.EntityExtractionResponse{}, nil
}

func (researchProviderStub) ExtractClaims(context.Context, ai.ExtractionRequest) (ai.ClaimExtractionResponse, error) {
	return ai.ClaimExtractionResponse{}, nil
}

func (researchProviderStub) ResolveEntity(context.Context, ai.EntityResolutionRequest) (ai.EntityResolutionResponse, error) {
	return ai.EntityResolutionResponse{}, nil
}

func (provider researchProviderStub) AnalyzeContradiction(context.Context, ai.ContradictionRequest) (ai.ContradictionResponse, error) {
	return provider.contradiction, nil
}

func (provider researchProviderStub) Rerank(context.Context, ai.RerankRequest) (ai.RerankResponse, error) {
	return provider.rerank, nil
}

func (researchProviderStub) ResearchQuery(context.Context, ai.ResearchQueryRequest) (ai.ResearchQueryResponse, error) {
	return ai.ResearchQueryResponse{}, nil
}

func TestValidateQueryInput(t *testing.T) {
	valid, err := validateQueryInput(QueryInput{Question: "  سؤال  ", SourceID: " 30000000-0000-0000-0000-000000000001 ", FromYear: 1990, ToYear: 2020})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if valid.Question != "سؤال" || valid.SourceID != "30000000-0000-0000-0000-000000000001" {
		t.Fatalf("unexpected normalized input: %+v", valid)
	}

	for _, input := range []QueryInput{
		{},
		{Question: stringsWithRunes(2001)},
		{Question: "سؤال", SourceID: "not-a-uuid"},
		{Question: "سؤال", FromYear: 2020, ToYear: 1990},
	} {
		if _, err := validateQueryInput(input); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected validation error for %+v, got %v", input, err)
		}
	}
}

func TestValidCitationSet(t *testing.T) {
	passages := []Citation{{PassageID: "passage-1"}, {PassageID: "passage-2"}}
	if !validCitationSet([]ai.ResearchCitation{{SourceID: "passage-1"}, {SourceID: "passage-2"}}, passages) {
		t.Fatal("expected known passage citations to pass")
	}
	if validCitationSet([]ai.ResearchCitation{{SourceID: "passage-3"}}, passages) {
		t.Fatal("expected unknown passage citation to fail")
	}
	if validCitationSet(nil, passages) {
		t.Fatal("expected empty citations to fail")
	}
}

func TestClaimConflicts(t *testing.T) {
	conflicts := claimConflicts([]Citation{{ID: "claim-1", Status: "contested", SourceID: "source-1"}})
	if len(conflicts) != 1 || conflicts[0].Type != "research_claim" || conflicts[0].SourceIDs[0] != "source-1" {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
}

func TestStatementConflictsOnlyKeepKnownReferences(t *testing.T) {
	service := &Service{AI: researchProviderStub{contradiction: ai.ContradictionResponse{Pairs: []ai.ContradictionPair{{LeftID: "statement-1", RightID: "statement-2", Status: "needs_review", Rationale: "تعارض"}, {LeftID: "statement-1", RightID: "unknown", Status: "needs_review", Rationale: "must be ignored"}}}}}
	conflicts, err := service.detectStatementConflicts(context.Background(), []Citation{{StatementID: "statement-1", Excerpt: "أ"}, {StatementID: "statement-2", Excerpt: "ب"}})
	if err != nil {
		t.Fatalf("unexpected contradiction error: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].LeftID != "statement-1" || conflicts[0].RightID != "statement-2" {
		t.Fatalf("unexpected filtered conflicts: %+v", conflicts)
	}
}

func TestFuseAndRerank(t *testing.T) {
	service := &Service{AI: researchProviderStub{rerank: ai.RerankResponse{Documents: []ai.RerankedDocument{{ID: "passage-2", Score: 0.95}, {ID: "passage-1", Score: 0.4}}}}}
	passages, stats, err := service.fuseAndRerank(context.Background(), retrievalContext{Input: QueryInput{Question: "سؤال"}}, []Citation{{PassageID: "passage-1", Score: Score{Lexical: 0.8}}, {PassageID: "passage-2", Score: Score{Lexical: 0.2}}}, []Citation{{PassageID: "passage-1", Score: Score{Vector: 0.9}}, {PassageID: "passage-2", Score: Score{Vector: 0.1}}})
	if err != nil {
		t.Fatalf("unexpected rerank error: %v", err)
	}
	if len(passages) != 2 || passages[0].PassageID != "passage-2" || passages[0].Rank != 1 {
		t.Fatalf("unexpected reranked passages: %+v", passages)
	}
	if stats.FusedCandidates != 2 || stats.RerankedCandidates != 2 {
		t.Fatalf("unexpected retrieval stats: %+v", stats)
	}
}

func TestResearchServiceRejectsMissingDependencies(t *testing.T) {
	if _, err := (&Service{}).Query(context.Background(), QueryInput{Question: "سؤال"}, ""); !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("expected database error, got %v", err)
	}
}

func stringsWithRunes(count int) string {
	result := make([]rune, count)
	for index := range result {
		result[index] = 'س'
	}
	return string(result)
}
