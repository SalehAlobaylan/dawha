package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geospatialintelligence"
)

func TestGeospatialIntelligenceRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/geospatial-intelligence/runs", body: `{"entity_type":"person","entity_id":"00000000-0000-0000-0000-000000000001","tree_id":"00000000-0000-0000-0000-000000000002","tree_version_id":"00000000-0000-0000-0000-000000000003"}`},
		{method: http.MethodGet, path: "/api/v1/geospatial-intelligence/runs/latest"},
		{method: http.MethodGet, path: "/api/v1/geospatial-intelligence/runs/00000000-0000-0000-0000-000000000004"},
		{method: http.MethodGet, path: "/api/v1/geospatial-intelligence/findings"},
		{method: http.MethodPost, path: "/api/v1/geospatial-intelligence/findings/00000000-0000-0000-0000-000000000005/review", body: `{"decision":"investigate"}`},
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

func TestGeospatialIntelligenceErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{geospatialintelligence.ErrValidation, http.StatusBadRequest},
		{geospatialintelligence.ErrForbidden, http.StatusForbidden},
		{geospatialintelligence.ErrNotFound, http.StatusNotFound},
		{geospatialintelligence.ErrConflict, http.StatusConflict},
		{geospatialintelligence.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeGeospatialIntelligenceError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
