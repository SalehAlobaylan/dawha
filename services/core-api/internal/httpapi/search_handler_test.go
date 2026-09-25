package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/search"
)

func TestSearchRouteIsPublic(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=عبدالله", nil)
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestSearchRejectsInvalidNumericFilter(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=سؤال&from_year=later", nil)
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestSearchErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{search.ErrValidation, http.StatusBadRequest},
		{search.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
		{search.ErrAIUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeSearchError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
