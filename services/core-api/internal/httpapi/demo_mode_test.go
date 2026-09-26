package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Demo mode, at the HTTP boundary.
//
// Three cases, and the third is the one that matters: the setting is off by
// default, the demo is explicit when it is on, and a database that is not there
// produces an ERROR rather than a confident set of numbers.

func TestTheDashboardIsADependencyErrorUnlessDemoModeIsOn(t *testing.T) {
	// The zero Dependencies value is what every existing caller and every
	// deployment that has not been told otherwise builds. It must not serve a
	// static payload that looks like real data.
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: a deployment with demo mode off must not be served an invented workspace", recorder.Code, http.StatusServiceUnavailable)
	}
	if got := recorder.Header().Get("X-Data-Source"); got != "" {
		t.Fatalf("the refused response claims its data source is %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("the refused response is cacheable: %q", got)
	}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
		Demo  bool   `json:"demo"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the body is not json: %v (%s)", err, recorder.Body.String())
	}
	if body.Code != "dashboard_unavailable" {
		t.Fatalf("code = %q", body.Code)
	}
	if body.Demo {
		t.Fatal("a refused response claims to be demo data")
	}
	// The sentence names the setting, because a 503 somebody cannot act on is a
	// mystery and a 503 that names its own remedy is a configuration note.
	if !strings.Contains(body.Error, "DEMO_MODE") {
		t.Fatalf("the error does not name the setting that would change the answer: %q", body.Error)
	}
	// And it leaks no workspace content: no metric, no question, no tree.
	for _, forbidden := range []string{"مساحة نجم", "1,248", "من كان والد عبدالله", "بيت العنبر"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("the refused response carries %q", forbidden)
		}
	}
}

func TestTheDashboardIsAnExplicitDemoWhenDemoModeIsOn(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{DemoMode: true}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: with demo mode on the demo IS the answer", recorder.Code)
	}
	if got := recorder.Header().Get("X-Data-Source"); got != "demo" {
		t.Fatalf("X-Data-Source = %q, want demo: a client that reads only headers must still be able to tell", got)
	}
	var body struct {
		Mode          string `json:"mode"`
		Demo          bool   `json:"demo"`
		WorkspaceName string `json:"workspaceName"`
		Metrics       []any  `json:"metrics"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// Three labels, not one: the header above, `mode` here, and the `demo` boolean
	// here. A payload that arrived with all three is a payload nobody can render
	// as real data by accident.
	if body.Mode != "demo" {
		t.Fatalf("mode = %q, want demo", body.Mode)
	}
	if !body.Demo {
		t.Fatal("the payload does not say it is demo")
	}
	if len(body.Metrics) == 0 || body.WorkspaceName == "" {
		t.Fatal("the demo payload is empty, so there is nothing to label")
	}
}

func TestTheDashboardIsUnavailableWithNoDatabaseEvenInDemoMode(t *testing.T) {
	// The dependency question, asked the way the plan asks it. With no pool at all
	// - the state a deployment is in when DATABASE_URL is missing - demo mode on
	// still serves the demo, and demo mode off serves the error. Neither serves
	// something that pretends to be a workspace read out of a database that is
	// not there, because there is no code path here that reads a database at all:
	// the payload is static, and that is exactly why it has to be behind a
	// setting rather than standing in for one.
	for _, demo := range []bool{false, true} {
		router := NewRouter(Dependencies{DemoMode: demo})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil))
		if demo && recorder.Code != http.StatusOK {
			t.Fatalf("with no database and demo mode on, the status is %d", recorder.Code)
		}
		if !demo && recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("with no database and demo mode off, the status is %d", recorder.Code)
		}
	}
	// A route that DOES need the database still reports the database as missing,
	// so the two are not confused with each other.
	ready := httptest.NewRecorder()
	NewRouter(Dependencies{DemoMode: true}).ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code == http.StatusOK {
		t.Fatalf("/readyz reports ready with no database: %d", ready.Code)
	}
}

func TestDemoModeIsReadFromTheEnvironmentAndIsOffUnlessItSaysOtherwise(t *testing.T) {
	// Every value a deployment could plausibly set, and what it means. The
	// asymmetry is the point: an unrecognised value is OFF, because the two
	// mistakes available here are not symmetric.
	cases := map[string]bool{
		"true": true, "TRUE": true, "1": true, "yes": true, "on": true, " true ": true,
		"": false, "0": false, "false": false, "no": false, "off": false,
		"t": false, "enabled": false, "maybe": false, "2": false, "TRUE ": true,
	}
	for value, want := range cases {
		if got := demoModeFromEnvironment(func(string) string { return value }); got != want {
			t.Fatalf("DEMO_MODE=%q was read as %v, want %v", value, got, want)
		}
	}
	if demoModeFromEnvironment(nil) {
		t.Fatal("a nil environment read as demo mode on")
	}
	if demoModeFromEnvironment(os.Getenv) {
		t.Fatalf("this process has DEMO_MODE=%q set, so the unset case is not being tested", os.Getenv("DEMO_MODE"))
	}
	if demoModeEnabled("yes") != true || demoModeEnabled("nonsense") != false {
		t.Fatal("demoModeEnabled and demoModeFromEnvironment disagree about the same words")
	}
}
