package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/suggestions"
)

func TestSuggestionReviewRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/trees/00000000-0000-0000-0000-000000000001/suggestions"},
		{method: http.MethodPatch, path: "/api/v1/suggestions/00000000-0000-0000-0000-000000000001", body: `{"decision":"accepted"}`},
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

func TestSuggestionPublicSubmitDoesNotRequireAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/suggestions", strings.NewReader(`{"tree_id":"00000000-0000-0000-0000-000000000001","version_id":"00000000-0000-0000-0000-000000000002","node_id":"00000000-0000-0000-0000-000000000003","text_ar":"مقترح عام"}`))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestSuggestionErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{suggestions.ErrValidation, http.StatusBadRequest},
		{suggestions.ErrNotFound, http.StatusNotFound},
		{suggestions.ErrForbidden, http.StatusForbidden},
		{suggestions.ErrConflict, http.StatusConflict},
		{suggestions.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
		// A change-set refusal is a FieldError that unwraps to ErrValidation, so it has
		// to map to the same 400 rather than falling through to a 500. A client that
		// cannot tell a malformed change set from a broken endpoint cannot mark the
		// field the reviewer mistyped.
		{suggestions.FieldError{Field: "change_set.person.person_id", Message: "person_id must be a uuid"}, http.StatusBadRequest},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeSuggestionError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}

// TestSuggestionValidationRefusalNamesTheField is the transport half of the composer
// contract: the field travels in the body so the form can mark the input, and only
// when the service named one.
func TestSuggestionValidationRefusalNamesTheField(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeSuggestionError(recorder, suggestions.FieldError{Field: "change_set.claim.predicate", Message: "predicate must be one of father_of"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", recorder.Code)
	}
	var body struct {
		Error string `json:"error"`
		Field string `json:"field"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("body: %v (%s)", err, recorder.Body.String())
	}
	if body.Field != "change_set.claim.predicate" {
		t.Fatalf("field = %q, want change_set.claim.predicate", body.Field)
	}
	if body.Error != "predicate must be one of father_of" {
		t.Fatalf("error = %q, want the service's own message", body.Error)
	}

	// A refusal that names no field carries no field key, so a client cannot mistake
	// an empty string for a field called "".
	recorder = httptest.NewRecorder()
	writeSuggestionError(recorder, suggestions.ErrValidation)
	var plain struct {
		Error string `json:"error"`
		Field string `json:"field"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &plain); err != nil {
		t.Fatalf("body: %v", err)
	}
	if plain.Field != "" {
		t.Fatalf("an unnamed refusal reported field %q", plain.Field)
	}
}
