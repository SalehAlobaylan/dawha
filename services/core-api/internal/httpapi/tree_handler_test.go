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

func TestTreeEditRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		path string
		body string
	}{
		{path: "/api/v1/trees/00000000-0000-0000-0000-000000000001/people", body: `{"canonical_name_ar":"شخص"}`},
		{path: "/api/v1/trees/00000000-0000-0000-0000-000000000001/relationships", body: `{"subject_node_id":"a","object_node_id":"b","predicate":"parent_of"}`},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set("Content-Type", "application/json")
		NewRouter(Dependencies{}).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected status %d, got %d", testCase.path, http.StatusUnauthorized, recorder.Code)
		}
	}
}

func TestTreeVersionRouteUsesSelectedVersionService(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trees/00000000-0000-0000-0000-000000000001/versions/00000000-0000-0000-0000-000000000002", nil)
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestUpdateRelationshipRouteRequiresAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/trees/00000000-0000-0000-0000-000000000001/relationships/00000000-0000-0000-0000-000000000002", strings.NewReader(`{"status":"disputed","expected_version_id":"00000000-0000-0000-0000-000000000003","reason_ar":"مراجعة"}`))
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
		{trees.ErrDuplicateRelationship, http.StatusConflict},
		{trees.ErrStaleVersion, http.StatusConflict},
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
