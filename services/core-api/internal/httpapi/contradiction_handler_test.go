package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/contradiction"
)

func TestContradictionRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/contradictions/runs", body: ""},
		{method: http.MethodGet, path: "/api/v1/contradictions/findings", body: ""},
		{method: http.MethodPost, path: "/api/v1/contradictions/findings/00000000-0000-0000-0000-000000000001/review", body: `{"decision":"investigate"}`},
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

func TestContradictionErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{contradiction.ErrValidation, http.StatusBadRequest},
		{contradiction.ErrForbidden, http.StatusForbidden},
		{contradiction.ErrNotFound, http.StatusNotFound},
		{contradiction.ErrConflict, http.StatusConflict},
		{contradiction.ErrQueueUnavailable, http.StatusServiceUnavailable},
		{contradiction.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeContradictionError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
