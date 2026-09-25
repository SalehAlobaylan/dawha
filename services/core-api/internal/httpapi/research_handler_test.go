package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/research"
)

func TestResearchQueryRouteIsUnavailableWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/research/query", strings.NewReader(`{"question":"سؤال"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestResearchRelationshipImpactRouteIsUnavailableWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/research/graph/relationship-impact", strings.NewReader(`{"tree_id":"b0000000-0000-0000-0000-000000000001","tree_version_id":"b1000000-0000-0000-0000-000000000003","relationship_id":"b3000000-0000-0000-0000-000000000001"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestResearchBranchStructureComparisonRouteIsUnavailableWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/research/graph/branch-structure-comparison", strings.NewReader(`{"from_tree_id":"b0000000-0000-0000-0000-000000000001"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestResearchAncestorFrontierRouteIsUnavailableWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/research/graph/ancestor-frontier", strings.NewReader(`{"tree_id":"b0000000-0000-0000-0000-000000000001"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestResearchSourceDependencyNeighborhoodRouteIsUnavailableWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/research/graph/source-dependency-neighborhood", strings.NewReader(`{"source_id":"30000000-0000-0000-0000-000000000002"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestResearchSourceDependencyCommunitiesRouteIsUnavailableWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/research/graph/source-dependency-communities", strings.NewReader(`{"source_id":"30000000-0000-0000-0000-000000000002"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestResearchWorkspaceRoutesAreUnavailableWithoutDatabase(t *testing.T) {
	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/research/questions/80000000-0000-0000-0000-000000000001/workspace"},
		{method: http.MethodGet, path: "/api/v1/research/questions/80000000-0000-0000-0000-000000000001/runs"},
		{method: http.MethodGet, path: "/api/v1/research/runs/00000000-0000-0000-0000-000000000001"},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, nil)
		NewRouter(Dependencies{}).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s: expected status %d, got %d", testCase.method, testCase.path, http.StatusServiceUnavailable, recorder.Code)
		}
	}
}

func TestResearchErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{research.ErrValidation, http.StatusBadRequest},
		{research.ErrForbidden, http.StatusForbidden},
		{research.ErrAIUnavailable, http.StatusServiceUnavailable},
		{research.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
		{research.ErrGraphUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeResearchError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
