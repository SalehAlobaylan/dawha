package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/researchagent"
)

func TestResearchAgentRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/research-agent/runs", body: `{"question":"ما موضع هجرة عبد الله؟","entity_type":"person","entity_id":"00000000-0000-0000-0000-000000000001"}`},
		{method: http.MethodGet, path: "/api/v1/research-agent/runs/latest"},
		{method: http.MethodGet, path: "/api/v1/research-agent/runs/00000000-0000-0000-0000-000000000002"},
		{method: http.MethodPost, path: "/api/v1/research-agent/runs/00000000-0000-0000-0000-000000000002/question-candidates"},
		{method: http.MethodGet, path: "/api/v1/research-agent/runs/00000000-0000-0000-0000-000000000002/question-candidates"},
		{method: http.MethodPatch, path: "/api/v1/research-question-candidates/00000000-0000-0000-0000-000000000003", body: `{"decision":"dismissed"}`},
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

func TestResearchAgentErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{researchagent.ErrValidation, http.StatusBadRequest},
		{researchagent.ErrForbidden, http.StatusForbidden},
		{researchagent.ErrNotFound, http.StatusNotFound},
		{researchagent.ErrConflict, http.StatusConflict},
		{researchagent.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeResearchAgentError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
