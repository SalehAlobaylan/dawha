package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dictionary"
)

func TestDictionaryRoutesArePublic(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dictionary?kind=people", nil)
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestDictionaryErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{dictionary.ErrValidation, http.StatusBadRequest},
		{dictionary.ErrNotFound, http.StatusNotFound},
		{dictionary.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeDictionaryError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
