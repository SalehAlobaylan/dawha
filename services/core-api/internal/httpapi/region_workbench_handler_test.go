package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/regionworkbench"
)

func TestRegionWorkbenchErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{regionworkbench.ErrValidation, http.StatusBadRequest},
		{regionworkbench.ErrUnauthenticated, http.StatusUnauthorized},
		{regionworkbench.ErrForbidden, http.StatusForbidden},
		{regionworkbench.ErrNotFound, http.StatusNotFound},
		{regionworkbench.ErrConflict, http.StatusConflict},
		// One person counted twice is a workflow conflict, not a role break.
		{regionworkbench.ErrSelfReview, http.StatusConflict},
		{regionworkbench.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeRegionWorkbenchError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
		var payload map[string]string
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || payload["error"] == "" {
			t.Fatalf("error %v: body %q is not an error object", testCase.err, recorder.Body.String())
		}
	}

	// An incomplete pass names the work: the missing candidate ids travel in
	// the body beside the message.
	recorder := httptest.NewRecorder()
	writeRegionWorkbenchError(recorder, regionworkbench.IncompletePassError{Missing: []string{"a", "b"}})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("incomplete pass status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var payload struct {
		Error             string   `json:"error"`
		MissingCandidates []string `json:"missingCandidates"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("incomplete body: %v (%s)", err, recorder.Body.String())
	}
	if len(payload.MissingCandidates) != 2 || payload.MissingCandidates[0] != "a" {
		t.Fatalf("missing candidates = %v", payload.MissingCandidates)
	}
}

func TestRegionWorkbenchRoutesRefuseUnconfiguredService(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/admin/region-workbench/tiles", "", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/admin/region-workbench/tiles/00000000-0000-0000-0000-00000000e004", "", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/admin/region-workbench/tiles/00000000-0000-0000-0000-00000000e004/history", "", http.StatusServiceUnavailable},
		{http.MethodPost, "/api/v1/admin/region-workbench/tiles/00000000-0000-0000-0000-00000000e004/claim", "", http.StatusServiceUnavailable},
		{http.MethodPost, "/api/v1/admin/region-workbench/tiles/00000000-0000-0000-0000-00000000e004/release", "{}", http.StatusServiceUnavailable},
		{http.MethodPost, "/api/v1/admin/region-workbench/tiles/00000000-0000-0000-0000-00000000e004/submit", "{}", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/admin/region-workbench/tiles/00000000-0000-0000-0000-00000000e004/other", "", http.StatusNotFound},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		NewRouter(Dependencies{}).ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Fatalf("%s %s: status %d, want %d", testCase.method, testCase.path, recorder.Code, testCase.status)
		}
	}
}
