package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
)

// Every identity route is authenticated before anything else happens: an
// anonymous caller is 401 without its body being read and without the database
// being touched, so there is no path here that answers differently for a caller
// with no session.
func TestIdentityRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/people"},
		{method: http.MethodPost, path: "/api/v1/people", body: `{"canonical_name_ar":"شخص"}`},
		{method: http.MethodGet, path: "/api/v1/people/00000000-0000-0000-0000-000000000001"},
		{method: http.MethodPatch, path: "/api/v1/people/00000000-0000-0000-0000-000000000001", body: `{"canonical_name_ar":"شخص","gender":"unknown"}`},
		{method: http.MethodDelete, path: "/api/v1/people/00000000-0000-0000-0000-000000000001"},
		{method: http.MethodGet, path: "/api/v1/people/00000000-0000-0000-0000-000000000001/aliases"},
		{method: http.MethodPost, path: "/api/v1/people/00000000-0000-0000-0000-000000000001/aliases", body: `{"value_ar":"لقب"}`},
		{method: http.MethodPatch, path: "/api/v1/person-aliases/00000000-0000-0000-0000-000000000002", body: `{"value_ar":"لقب"}`},
		{method: http.MethodDelete, path: "/api/v1/person-aliases/00000000-0000-0000-0000-000000000002"},
		{method: http.MethodGet, path: "/api/v1/families"},
		{method: http.MethodPost, path: "/api/v1/families", body: `{"canonical_name_ar":"بيت"}`},
		{method: http.MethodGet, path: "/api/v1/families/00000000-0000-0000-0000-000000000003"},
		{method: http.MethodPatch, path: "/api/v1/families/00000000-0000-0000-0000-000000000003", body: `{"canonical_name_ar":"بيت"}`},
		{method: http.MethodDelete, path: "/api/v1/families/00000000-0000-0000-0000-000000000003"},
		{method: http.MethodGet, path: "/api/v1/tribes"},
		{method: http.MethodPost, path: "/api/v1/tribes", body: `{"canonical_name_ar":"قبيلة"}`},
		{method: http.MethodGet, path: "/api/v1/tribes/00000000-0000-0000-0000-000000000004"},
		{method: http.MethodPatch, path: "/api/v1/tribes/00000000-0000-0000-0000-000000000004", body: `{"canonical_name_ar":"قبيلة"}`},
		{method: http.MethodDelete, path: "/api/v1/tribes/00000000-0000-0000-0000-000000000004"},
		{method: http.MethodGet, path: "/api/v1/branches"},
		{method: http.MethodPost, path: "/api/v1/branches", body: `{"family_id":"00000000-0000-0000-0000-000000000005","canonical_name_ar":"فرع"}`},
		{method: http.MethodGet, path: "/api/v1/branches/00000000-0000-0000-0000-000000000006"},
		{method: http.MethodPatch, path: "/api/v1/branches/00000000-0000-0000-0000-000000000006", body: `{"canonical_name_ar":"فرع"}`},
		{method: http.MethodDelete, path: "/api/v1/branches/00000000-0000-0000-0000-000000000006"},
		{method: http.MethodGet, path: "/api/v1/places"},
		{method: http.MethodPost, path: "/api/v1/places", body: `{"canonical_name_ar":"موضع","place_type":"city"}`},
		{method: http.MethodGet, path: "/api/v1/places/00000000-0000-0000-0000-000000000007"},
		{method: http.MethodPatch, path: "/api/v1/places/00000000-0000-0000-0000-000000000007", body: `{"canonical_name_ar":"موضع","place_type":"city"}`},
		{method: http.MethodDelete, path: "/api/v1/places/00000000-0000-0000-0000-000000000007"},
		{method: http.MethodGet, path: "/api/v1/entity-relationships"},
		{method: http.MethodPost, path: "/api/v1/entity-relationships", body: `{"subject_id":"00000000-0000-0000-0000-000000000008","predicate":"father_of","object_id":"00000000-0000-0000-0000-000000000009"}`},
		{method: http.MethodGet, path: "/api/v1/entity-relationships/00000000-0000-0000-0000-00000000000a"},
		{method: http.MethodPatch, path: "/api/v1/entity-relationships/00000000-0000-0000-0000-00000000000a", body: `{"predicate":"father_of","status":"unresolved"}`},
		{method: http.MethodDelete, path: "/api/v1/entity-relationships/00000000-0000-0000-0000-00000000000a"},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		if testCase.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		NewRouter(Dependencies{}).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected status %d, got %d (%s)", testCase.method, testCase.path, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
		}
	}
}

// The refusals are the interesting statuses here, so the mapping is pinned one by
// one. A published interpretation answers 409 and not 403: the request is
// well formed, the caller is allowed to make it, and the record's current state is
// what forbids it.
func TestIdentityErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{err: identity.ErrValidation, status: http.StatusBadRequest},
		{err: identity.ErrNotFound, status: http.StatusNotFound},
		{err: identity.ErrForbidden, status: http.StatusForbidden},
		{err: identity.ErrConflict, status: http.StatusConflict},
		{err: identity.ErrPublishedInterpretation, status: http.StatusConflict},
		{err: identity.ErrReferencedByInterpretation, status: http.StatusConflict},
		{err: identity.ErrMergedIdentity, status: http.StatusConflict},
		{err: identity.ErrPublishedReference, status: http.StatusConflict},
		{err: identity.ErrResearchReference, status: http.StatusConflict},
		{err: identity.ErrSettledInterpretation, status: http.StatusConflict},
		{err: identity.ErrDatabaseUnavailable, status: http.StatusServiceUnavailable},
		{err: errors.New("something else"), status: http.StatusInternalServerError},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeIdentityError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
	// A refusal body carries the error's own wording and nothing else, so it can
	// never be the way a caller reads the record it was refused.
	for _, refusal := range []error{identity.ErrPublishedInterpretation, identity.ErrPublishedReference, identity.ErrResearchReference} {
		recorder := httptest.NewRecorder()
		writeIdentityError(recorder, refusal)
		body := recorder.Body.String()
		if !strings.Contains(body, strings.Split(refusal.Error(), " ")[0]) {
			t.Fatalf("the refusal body does not explain itself: %s", body)
		}
		if strings.Contains(body, "canonical_name_ar") || strings.Contains(body, "person_id") {
			t.Fatalf("the refusal body carries record data: %s", body)
		}
	}
	// The two reference refusals have to read differently, because they ask for
	// different things: one says the row is public and unversioned, the other says
	// publish it first.
	if identity.ErrPublishedReference.Error() == identity.ErrResearchReference.Error() {
		t.Fatal("the two reference refusals share one message, so a caller cannot tell which applies")
	}
}
