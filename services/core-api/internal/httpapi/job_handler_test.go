package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
)

func TestJobRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/jobs", ""},
		{http.MethodPost, "/api/v1/jobs", `{"type":"source_process"}`},
		{http.MethodPost, "/api/v1/jobs/claim", `{"worker_id":"worker-1"}`},
		{http.MethodPost, "/api/v1/jobs/00000000-0000-0000-0000-000000000001/complete", `{"worker_id":"worker-1"}`},
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

func TestJobErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{jobs.ErrValidation, http.StatusBadRequest},
		{jobs.ErrNotFound, http.StatusNotFound},
		{jobs.ErrForbidden, http.StatusForbidden},
		{jobs.ErrConflict, http.StatusConflict},
		{jobs.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeJobError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
