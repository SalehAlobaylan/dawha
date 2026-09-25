package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClientCallsAllStableContracts(t *testing.T) {
	paths := make(chan string, 20)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", request.Header.Get("Content-Type"))
		}
		paths <- request.URL.Path
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(body) == 0 {
			t.Error("request body is empty")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseForPath(request.URL.Path))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)
	client.Timeout = time.Second
	_, err := client.NormalizeName(context.Background(), " عَبد الله ")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Embed(context.Background(), EmbeddingRequest{Text: "عبدالله", Dimensions: 16})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Classify(context.Background(), ClassificationRequest{Text: "نص", Labels: []string{"أ", "ب"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Route(context.Background(), RoutingRequest{Text: "ما اسم المصدر؟"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ExtractEntities(context.Background(), ExtractionRequest{Text: "ذكر محمد"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ExtractClaims(context.Background(), ExtractionRequest{Text: "ذكر محمد"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ResolveEntity(context.Background(), EntityResolutionRequest{
		Name:       "محمد",
		Candidates: []EntityReference{{ID: "p1", Name: "محمد"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AnalyzeContradiction(context.Background(), ContradictionRequest{
		Statements: []Statement{{ID: "a", Text: "محمد"}, {ID: "b", Text: "محمد"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Rerank(context.Background(), RerankRequest{
		Query:     "محمد",
		Documents: []RerankDocument{{ID: "s1", Text: "محمد"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ResearchQuery(context.Background(), ResearchQueryRequest{
		Query:    "من هو محمد؟",
		Contexts: []SourceContext{{ID: "s1", Title: "مصدر", Text: "محمد"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"/v1/normalize-name",
		"/v1/embed",
		"/v1/classify",
		"/v1/route",
		"/v1/extract/entities",
		"/v1/extract/claims",
		"/v1/resolve/entity",
		"/v1/analyze/contradiction",
		"/v1/rerank",
		"/v1/research/query",
	}
	close(paths)
	for _, want := range expected {
		if got := <-paths; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	}
}

func TestHTTPProviderRetriesTransientFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) < 3 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseForPath("/v1/embed"))
	}))
	defer server.Close()

	provider := NewHTTPProvider(server.URL)
	provider.MaxRetries = 2
	provider.RetryBase = time.Millisecond
	result, err := provider.Embed(context.Background(), EmbeddingRequest{Text: "محمد", Dimensions: 16})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Embedding) != 16 || calls.Load() != 3 {
		t.Fatalf("embedding length = %d, calls = %d", len(result.Embedding), calls.Load())
	}
}

func TestValidateEmbeddingRejectsZeroVector(t *testing.T) {
	if err := validateEmbedding(EmbeddingResponse{Embedding: make([]float64, 16), Dimensions: 16, Model: "test"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestHTTPProviderRejectsMalformedSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"embedding":[0.1],"dimensions":16,"model":"test","deterministic":true}`)
	}))
	defer server.Close()

	_, err := NewHTTPClient(server.URL).Embed(context.Background(), EmbeddingRequest{Text: "محمد", Dimensions: 16})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestHTTPClientRejectsInvalidRequestsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewHTTPClient(server.URL).Embed(context.Background(), EmbeddingRequest{Text: "محمد", Dimensions: 1})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("calls = %d, want 0", calls.Load())
	}
}

func TestHTTPClientHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-request.Context().Done():
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)
	client.Timeout = 10 * time.Millisecond
	_, err := client.NormalizeName(context.Background(), "محمد")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

func responseForPath(path string) string {
	switch path {
	case "/v1/normalize-name":
		return `{"original":"محمد","normalized":"محمد","method":"deterministic-normalization","preserves_original":true}`
	case "/v1/embed":
		return `{"embedding":[0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1,0.1],"dimensions":16,"model":"test","deterministic":true}`
	case "/v1/classify":
		return `{"candidates":[{"label":"أ","confidence":0.8,"rationale":"review"}],"model":"test","review_required":true}`
	case "/v1/route":
		return `{"route":"cheap","query_type":"source_evidence","reason_code":"simple_lookup","source_bearing":false,"potential_contradiction":false,"continue_investigation":false,"operational_score":0.58,"model":"test","fallback":false,"review_required":true}`
	case "/v1/extract/entities":
		return `{"entities":[{"text":"محمد","entity_type":"person","confidence":0.8,"status":"unreviewed","rationale":"review"}],"model":"test","review_required":true}`
	case "/v1/extract/claims":
		return `{"claims":[{"subject_text":"محمد","predicate":"ذكر","confidence":0.5,"status":"unreviewed","rationale":"review"}],"model":"test","review_required":true}`
	case "/v1/resolve/entity":
		return `{"matches":[{"candidate_id":"p1","candidate_name":"محمد","score":0.95,"matched_on":"alias"}],"model":"test","review_required":true}`
	case "/v1/analyze/contradiction":
		return `{"has_candidate_contradiction":true,"pairs":[{"left_id":"a","right_id":"b","confidence":0.5,"rationale":"review","status":"needs_review"}],"model":"test","review_required":true}`
	case "/v1/rerank":
		return `{"documents":[{"id":"s1","score":0.8,"excerpt":"محمد"}],"model":"test"}`
	case "/v1/research/query":
		return `{"answer":"review","citations":[{"source_id":"s1","title":"مصدر","excerpt":"محمد"}],"model":"test","review_required":true}`
	default:
		return `{}`
	}
}
