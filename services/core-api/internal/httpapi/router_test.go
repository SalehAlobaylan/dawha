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
