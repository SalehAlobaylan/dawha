package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/entityresolution"
)

func TestEntityResolutionRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/entity-resolution/runs", body: `{"entity_type":"person"}`},
		{method: http.MethodGet, path: "/api/v1/entity-resolution/candidates", body: ""},
		{method: http.MethodPost, path: "/api/v1/entity-resolution/candidates/00000000-0000-0000-0000-000000000001/review", body: `{"decision":"approve"}`},
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

func TestEntityResolutionErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{entityresolution.ErrValidation, http.StatusBadRequest},
		{entityresolution.ErrForbidden, http.StatusForbidden},
		{entityresolution.ErrNotFound, http.StatusNotFound},
		{entityresolution.ErrConflict, http.StatusConflict},
		{entityresolution.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeEntityResolutionError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
