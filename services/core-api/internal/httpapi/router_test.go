package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardEndpoint(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)

	NewRouter(Dependencies{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
	}
}

func TestNormalizeNameEndpoint(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/normalize-name", strings.NewReader(`{"value":"عَبْدُ الله"}`))
	request.Header.Set("Content-Type", "application/json")

	NewRouter(Dependencies{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"normalized":"عبد الله"`) {
		t.Fatalf("unexpected response %s", recorder.Body.String())
	}
}

func TestResearchLayersEndpoint(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/research/layers", nil)

	NewRouter(Dependencies{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	for _, layer := range []string{"source_statement", "research_claim", "tree_interpretation", "platform_finding", "open_question"} {
		if !strings.Contains(recorder.Body.String(), layer) {
			t.Fatalf("response missing layer %q", layer)
		}
	}
}
