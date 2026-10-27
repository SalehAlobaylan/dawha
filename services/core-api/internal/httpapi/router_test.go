package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The dashboard used to answer 200 with a static payload for every caller. It now
// answers a dependency error unless DEMO_MODE is on, and demo_mode_test.go is
// where that behaviour is pinned in full. What is left here is the smallest thing
// that would have caught the regression: the response is JSON either way, so a
// client that switches on content type still works, and the status is the one the
// setting implies.
func TestDashboardEndpointRespectsDemoMode(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		demo     bool
		wantCode int
	}{
		{name: "demo mode off", demo: false, wantCode: http.StatusServiceUnavailable},
		{name: "demo mode on", demo: true, wantCode: http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)

			NewRouter(Dependencies{DemoMode: testCase.demo}).ServeHTTP(recorder, request)

			if recorder.Code != testCase.wantCode {
				t.Fatalf("expected status %d, got %d", testCase.wantCode, recorder.Code)
			}
			if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
			}
		})
	}
}

func TestDevQuickSignInRouteRequiresExplicitConfiguration(t *testing.T) {
	newRequest := func() *http.Request {
		request := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/v1/auth/dev/quick-sign-in", nil)
		request.RemoteAddr = "127.0.0.1:1234"
		request.Header.Set("Origin", "http://localhost:5173")
		return request
	}

	disabled := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(disabled, newRequest())
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("disabled quick sign-in status = %d, want %d", disabled.Code, http.StatusNotFound)
	}

	enabled := httptest.NewRecorder()
	NewRouter(Dependencies{DevQuickSignIn: true}).ServeHTTP(enabled, newRequest())
	if enabled.Code != http.StatusServiceUnavailable {
		t.Fatalf("enabled quick sign-in without a database status = %d, want %d", enabled.Code, http.StatusServiceUnavailable)
	}
}

func TestNormalizeNameEndpoint(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/normalize-name", strings.NewReader(`{"value":"عَبْدُ الله"}`))
	request.Header.Set("Content-Type", "application/json")

	NewRouter(Dependencies{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"normalized":"عبد الله"`) {
		t.Fatalf("unexpected response %s", recorder.Body.String())
	}
}

// The layer catalogue is a static response carrying the same demo label the
// dashboard carried, so it is behind the same setting: off is a dependency error.
func TestResearchLayersEndpointRespectsDemoMode(t *testing.T) {
	off := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(off, httptest.NewRequest(http.MethodGet, "/api/v1/research/layers", nil))
	if off.Code != http.StatusServiceUnavailable {
		t.Fatalf("with demo mode off the status is %d, want %d", off.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(off.Body.String(), "label_ar") {
		t.Fatalf("a refused layer catalogue still carried the layers: %s", off.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/research/layers", nil)

	NewRouter(Dependencies{DemoMode: true}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("X-Data-Source"); got != "demo" {
		t.Fatalf("X-Data-Source = %q, want demo", got)
	}
	for _, layer := range []string{"source_statement", "research_claim", "tree_interpretation", "platform_finding", "open_question"} {
		if !strings.Contains(recorder.Body.String(), layer) {
			t.Fatalf("response missing layer %q", layer)
		}
	}
}

// The CORS preflight must advertise every method the router actually serves. Three
// source routes save a tree draft, a chart-analysis review and a book assertion with
// PUT, and a browser preflight that does not list PUT is refused before the request
// is sent, so the route exists and is unreachable from the web app.
//
// The list is stated here rather than read from the router, because http.ServeMux
// exposes no way to enumerate its patterns. A new method on a source route needs
// adding to this list, and to the header, in the same change.
func TestCORSPreflightAdvertisesEveryMethodTheRouterServes(t *testing.T) {
	router := NewRouter(Dependencies{})

	advertised := map[string]bool{}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		advertised[method] = false
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "http://localhost:8080/api/v1/source-tree-drafts/00000000-0000-0000-0000-000000000000", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(recorder, request)

	header := recorder.Header().Get("Access-Control-Allow-Methods")
	if header == "" {
		t.Fatal("preflight did not return Access-Control-Allow-Methods")
	}
	for method := range advertised {
		if strings.Contains(header, method) {
			advertised[method] = true
		}
	}

	for method, present := range advertised {
		if !present {
			t.Errorf("preflight does not advertise %s; allowed methods are %q", method, header)
		}
	}
}
