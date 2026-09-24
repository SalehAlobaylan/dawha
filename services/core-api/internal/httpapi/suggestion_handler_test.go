package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/suggestions"
)

func TestSuggestionReviewRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/trees/00000000-0000-0000-0000-000000000001/suggestions"},
		{method: http.MethodPatch, path: "/api/v1/suggestions/00000000-0000-0000-0000-000000000001", body: `{"decision":"accepted"}`},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set("Content-Type", "application/json")
		NewRouter(Dependencies{}).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected status %d, got %d", testCase.method, testCase.path, http.StatusUnauthorized, recorder.Code)
		}
	}
}

func TestSuggestionPublicSubmitDoesNotRequireAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/suggestions", strings.NewReader(`{"tree_id":"00000000-0000-0000-0000-000000000001","version_id":"00000000-0000-0000-0000-000000000002","node_id":"00000000-0000-0000-0000-000000000003","text_ar":"مقترح عام"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestSuggestionErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{suggestions.ErrValidation, http.StatusBadRequest},
		{suggestions.ErrNotFound, http.StatusNotFound},
		{suggestions.ErrForbidden, http.StatusForbidden},
		{suggestions.ErrConflict, http.StatusConflict},
		{suggestions.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeSuggestionError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
