package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
)

func TestMapRoutesArePublic(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/map", nil)
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestMapErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{geography.ErrValidation, http.StatusBadRequest},
		{geography.ErrNotFound, http.StatusNotFound},
		{geography.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeGeographyError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
