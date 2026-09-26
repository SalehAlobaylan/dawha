package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestMapInputReadsTheNewFilters is the handler half of the map change: the bound
// and the viewport have to reach the service, and a half-sent box has to be refused
// rather than completed with a guess - a guessed edge moves the map's horizon, and
// the caller would never learn it.
func TestMapInputReadsTheNewFilters(t *testing.T) {
	handler := geographyHandler{}

	input, ok := handler.mapInput(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet,
		"/api/v1/map?from_year=1000&to_year=1250&status=disputed&place_id=20000000-0000-0000-0000-000000000002&limit=25"+
			"&min_longitude=10&min_latitude=20&max_longitude=30&max_latitude=40", nil))
	if !ok {
		t.Fatal("a complete map query was refused")
	}
	if input.FromYear != 1000 || input.ToYear != 1250 || input.Status != "disputed" || input.Limit != 25 {
		t.Fatalf("map input = %+v", input)
	}
	if input.PlaceID != "20000000-0000-0000-0000-000000000002" {
		t.Fatalf("map place = %q", input.PlaceID)
	}
	if input.Viewport == nil {
		t.Fatal("the viewport did not reach the service")
	}
	if *input.Viewport != (geography.Viewport{MinLongitude: 10, MinLatitude: 20, MaxLongitude: 30, MaxLatitude: 40}) {
		t.Fatalf("map viewport = %+v", *input.Viewport)
	}

	// No parameters at all is the whole map, which is the default the endpoint has
	// always answered with.
	plain, ok := handler.mapInput(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/map", nil))
	if !ok {
		t.Fatal("an unparameterised map query was refused")
	}
	if plain.Viewport != nil || plain.Limit != 0 || plain.Status != "" || plain.FromYear != 0 {
		t.Fatalf("a bare map query = %+v, want no filters at all", plain)
	}

	for _, testCase := range []struct {
		name  string
		query string
	}{
		{name: "a half-sent viewport", query: "/api/v1/map?min_longitude=10&max_longitude=30"},
		{name: "a non-numeric edge", query: "/api/v1/map?min_longitude=north"},
		{name: "a non-numeric limit", query: "/api/v1/map?limit=many"},
		{name: "a non-numeric from_year", query: "/api/v1/map?from_year=early"},
	} {
		recorder := httptest.NewRecorder()
		if _, ok := handler.mapInput(recorder, httptest.NewRequest(http.MethodGet, testCase.query, nil)); ok {
			t.Fatalf("%s was accepted", testCase.name)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected status %d, got %d", testCase.name, http.StatusBadRequest, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "error") {
			t.Fatalf("%s: the refusal does not say why: %s", testCase.name, recorder.Body.String())
		}
	}
}
