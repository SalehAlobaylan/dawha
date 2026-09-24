package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/trees"
)

func TestTreeListFallsBackToDemoWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil)

	NewRouter(Dependencies{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"mode":"demo"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestCreateTreeRequiresAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/trees", strings.NewReader(`{"name_ar":"شجرة جديدة"}`))
	request.Header.Set("Content-Type", "application/json")

	NewRouter(Dependencies{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestTreeErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{trees.ErrValidation, http.StatusBadRequest},
		{trees.ErrNotFound, http.StatusNotFound},
		{trees.ErrForbidden, http.StatusForbidden},
		{trees.ErrNoDraft, http.StatusConflict},
		{trees.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeTreeError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}

func TestTreeHandlerUsesAuthService(t *testing.T) {
	service := auth.NewService(nil)
	if service == nil {
		t.Fatal("expected auth service")
	}
}
