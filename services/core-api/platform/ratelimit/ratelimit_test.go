package ratelimit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testConfig(budgets map[string]int) Config {
	config := DefaultConfig()
	for class, budget := range budgets {
		config.PerMinute[class] = budget
	}
	return config
}

func TestTheDocumentedDefaultsLeaveTheLocalDemoWorkflowAlone(t *testing.T) {
	// The STOP condition for this work is that rate limits must not break the
	// documented local demo workflow, and the workflow is: sign in, create a tree,
	// attach a source, ask a question, run a research query. This test is that
	// claim, written down as the number of requests it takes, so a future edit to
	// a default has to edit this test too.
	config := DefaultConfig()
	limiter, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	workflow := []struct {
		class    string
		requests int
	}{
		// Sign in and register a few times over a session.
		{ClassAuth, 10},
		// Create a tree, add people and relationships.
		{ClassDefault, 40},
		// Upload three documents and create a source.
		{ClassUpload, 5},
		// Submit two suggestions and review them.
		{ClassSuggestion, 4},
		// Ask a question, then run the research query and the graph analyses the
		// research page offers.
		{ClassResearch, 10},
	}
	for _, step := range workflow {
		budget := config.PerMinute[step.class]
		if step.requests >= budget {
			t.Fatalf("the demo workflow needs %d %s requests in a window and the budget is %d", step.requests, step.class, budget)
		}
		for i := 0; i < step.requests; i++ {
			if decision := limiter.Allow(step.class, "192.0.2.1"); !decision.Allowed {
				t.Fatalf("the demo workflow was throttled at %s request %d of %d: %+v", step.class, i+1, step.requests, decision)
			}
		}
	}
}

func TestAllowThenThrottle(t *testing.T) {
	limiter, err := New(testConfig(map[string]int{ClassDefault: 3}))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		decision := limiter.Allow(ClassDefault, "198.51.100.7")
		if !decision.Allowed {
			t.Fatalf("request %d was refused inside the budget", i+1)
		}
		if decision.Remaining != 2-i {
			t.Fatalf("request %d reported %d remaining, want %d", i+1, decision.Remaining, 2-i)
		}
	}
	refused := limiter.Allow(ClassDefault, "198.51.100.7")
	if refused.Allowed {
		t.Fatal("the fourth request was allowed inside a budget of three")
	}
	if refused.RetryAfter <= 0 {
		t.Fatalf("a refusal must say when to come back, got %s", refused.RetryAfter)
	}
	// A different client is a different budget. A shared limit would let one noisy
	// neighbour lock everybody else out.
	if decision := limiter.Allow(ClassDefault, "198.51.100.8"); !decision.Allowed {
		t.Fatal("a second client was refused by the first client's spending")
	}
}

func TestTheWindowRollsOver(t *testing.T) {
	config := testConfig(map[string]int{ClassDefault: 1})
	clock := time.Unix(1_700_000_000, 0)
	config.now = func() time.Time { return clock }
	limiter, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if !limiter.Allow(ClassDefault, "203.0.113.1").Allowed {
		t.Fatal("the first request was refused")
	}
	if limiter.Allow(ClassDefault, "203.0.113.1").Allowed {
		t.Fatal("the second request was allowed inside a budget of one")
	}
	clock = clock.Add(61 * time.Second)
	if !limiter.Allow(ClassDefault, "203.0.113.1").Allowed {
		t.Fatal("the request after the window rolled over was still refused")
	}
}

func TestClassesDoNotShareABudget(t *testing.T) {
	// A flood of research calls must not be able to spend the budget that
	// protects sign-in, and the reverse must hold too.
	limiter, err := New(testConfig(map[string]int{
		ClassAuth:     1,
		ClassResearch: 1,
		ClassDefault:  1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !limiter.Allow(ClassResearch, "192.0.2.5").Allowed {
		t.Fatal("the first research call was refused")
	}
	if limiter.Allow(ClassResearch, "192.0.2.5").Allowed {
		t.Fatal("the second research call was allowed inside a budget of one")
	}
	if !limiter.Allow(ClassAuth, "192.0.2.5").Allowed {
		t.Fatal("research spending refused a sign-in")
	}
	if limiter.Allow(ClassAuth, "192.0.2.5").Allowed {
		t.Fatal("the second sign-in was allowed inside a budget of one")
	}
	if !limiter.Allow(ClassDefault, "192.0.2.5").Allowed {
		t.Fatal("auth spending refused an unrelated request")
	}
}

func TestClassForChargesTheRightClass(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodPost, "/api/v1/auth/login", ClassAuth},
		{http.MethodPost, "/api/v1/auth/register", ClassAuth},
		{http.MethodGet, "/api/v1/auth/me", ClassAuth},
		{http.MethodPost, "/api/v1/auth/logout", ClassAuth},
		{http.MethodPost, "/api/v1/sources/abc/files", ClassUpload},
		{http.MethodPost, "/api/v1/sources", ClassUpload},
		{http.MethodPost, "/api/v1/suggestions", ClassSuggestion},
		{http.MethodPatch, "/api/v1/suggestions/abc", ClassSuggestion},
		{http.MethodPost, "/api/v1/research/query", ClassResearch},
		{http.MethodPost, "/api/v1/research/graph/relationship-impact", ClassResearch},
		{http.MethodPost, "/api/v1/research-agent/runs", ClassResearch},
		{http.MethodPost, "/api/v1/entity-resolution/runs", ClassResearch},
		{http.MethodPost, "/api/v1/contradictions/runs", ClassResearch},
		{http.MethodGet, "/api/v1/trees", ClassDefault},
		{http.MethodPost, "/api/v1/trees", ClassDefault},
		{http.MethodGet, "/healthz", ClassDefault},
		// A path that merely starts with the same letters is not the same route.
		{http.MethodPost, "/api/v1/researchers", ClassDefault},
		{http.MethodGet, "/api/v1/suggestions-not-really", ClassDefault},
		// A GET of a file is not an upload, and a suggestions LIST is still the
		// suggestion class.
		{http.MethodGet, "/api/v1/sources/abc/files", ClassDefault},
		{http.MethodGet, "/api/v1/suggestions", ClassSuggestion},
	}
	for _, testCase := range cases {
		if got := ClassFor(testCase.method, testCase.path); got != testCase.want {
			t.Fatalf("ClassFor(%s %s) = %q, want %q", testCase.method, testCase.path, got, testCase.want)
		}
	}
}

func TestClientIdentityDoesNotTrustForwardedHeaders(t *testing.T) {
	// If this honoured X-Forwarded-For, a client could invent one address per
	// request and the limiter would measure nothing at all.
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil)
	request.RemoteAddr = "192.0.2.10:5555"
	request.Header.Set("X-Forwarded-For", "203.0.113.99")
	request.Header.Set("X-Real-IP", "203.0.113.98")
	if got := ClientIdentity(request); got != "192.0.2.10" {
		t.Fatalf("ClientIdentity = %q, want the peer address 192.0.2.10", got)
	}
	// The IPv4-mapped IPv6 form is the same client, not a second one.
	mapped := httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil)
	mapped.RemoteAddr = "[::ffff:127.0.0.1]:5555"
	plain := httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil)
	plain.RemoteAddr = "127.0.0.1:5555"
	if ClientIdentity(mapped) != ClientIdentity(plain) {
		t.Fatalf("the mapped form is a different client from its own IPv4 address")
	}
}

func TestMiddlewareAnswers429WithRetryAfterAndNoContent(t *testing.T) {
	limiter, err := New(testConfig(map[string]int{ClassAuth: 1}))
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	handler := Middleware(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("the first request got %d", first.Code)
	}
	if first.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("remaining = %q, want 0", first.Header().Get("X-RateLimit-Remaining"))
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("the second request got %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("a 429 without Retry-After tells the client nothing about when to return")
	}
	var payload map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &payload); err != nil {
		t.Fatalf("the 429 body is not json: %v", err)
	}
	if payload["class"] != ClassAuth {
		t.Fatalf("the 429 body does not name the class: %v", payload)
	}
	// The limiter's own answer must not carry the request. A body that echoed the
	// path or the query would be a place request content accumulates.
	body := second.Body.String()
	for _, forbidden := range []string{"password", "email", "query", "search"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("the 429 body mentions %q: %s", forbidden, body)
		}
	}
	if seen != 1 {
		t.Fatalf("the handler ran %d times, want 1: a throttled request must not reach it", seen)
	}
}

func TestDisabledIsAPassThrough(t *testing.T) {
	// The development override, asserted rather than assumed: it is a pass-through,
	// and it is the ONLY way to get one.
	limiter, err := New(Disabled())
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	handler := Middleware(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 500; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	}
	if seen != 500 {
		t.Fatalf("the disabled limiter served %d of 500 requests", seen)
	}
	// A default configuration is NOT disabled. The override is explicit.
	if DefaultConfig().Enabled != true {
		t.Fatal("the default configuration is disabled; the override must be explicit")
	}
}

func TestConfigFromEnvironment(t *testing.T) {
	values := map[string]string{
		"RATE_LIMIT_AUTH_PER_MINUTE":   "7",
		"RATE_LIMIT_UPLOAD_PER_MINUTE": "0",
		"RATE_LIMIT_WINDOW":            "30s",
		"RATE_LIMIT_ENABLED":           "false",
	}
	config, err := ConfigFromEnvironment(func(name string) string { return values[name] })
	if err != nil {
		t.Fatal(err)
	}
	if config.PerMinute[ClassAuth] != 7 {
		t.Fatalf("auth budget = %d, want 7", config.PerMinute[ClassAuth])
	}
	if config.PerMinute[ClassUpload] != 0 {
		t.Fatalf("upload budget = %d, want 0 - zero is a legitimate way to close a class", config.PerMinute[ClassUpload])
	}
	if config.Window != 30*time.Second {
		t.Fatalf("window = %s", config.Window)
	}
	if config.Enabled {
		t.Fatal("RATE_LIMIT_ENABLED=false was not read")
	}
	unset, err := ConfigFromEnvironment(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if unset.PerMinute[ClassAuth] != authPerMinute || unset.Window != window || !unset.Enabled {
		t.Fatalf("an unset environment did not produce the documented defaults: %+v", unset)
	}
}

func TestConfigFromEnvironmentRefusesATypoRatherThanDefaulting(t *testing.T) {
	// A mistyped variable that silently became the default would be a limit that
	// looks configured and is not. Each of these has to be a startup failure that
	// names the variable.
	cases := []map[string]string{
		{"RATE_LIMIT_AUTH_PER_MINUTE": "many"},
		{"RATE_LIMIT_AUTH_PER_MINUTE": "-1"},
		{"RATE_LIMIT_AUTH_PER_MINUTE": "1.5"},
		{"RATE_LIMIT_WINDOW": "sixty seconds"},
		{"RATE_LIMIT_WINDOW": "-5s"},
	}
	for _, values := range cases {
		if _, err := ConfigFromEnvironment(func(name string) string { return values[name] }); err == nil {
			t.Fatalf("the configuration %v was accepted", values)
		}
	}
}

func TestNewRefusesAConfigurationThatAdmitsEverything(t *testing.T) {
	// A Config with no budgets at all is the zero value, which is what a caller
	// that forgot to call DefaultConfig passes. It must not become a limiter that
	// allows everything, because the failure that follows is an unbounded endpoint.
	if _, err := New(Config{Enabled: true, Window: time.Minute}); err == nil {
		t.Fatal("a limiter with no budgets was built")
	}
	if _, err := New(Config{Enabled: true, Window: time.Minute, PerMinute: map[string]int{ClassDefault: 10}}); err == nil {
		t.Fatal("a limiter missing four of the five classes was built")
	}
	if _, err := New(Config{Enabled: true, Window: time.Minute, PerMinute: map[string]int{
		ClassAuth: 1, ClassUpload: 1, ClassSuggestion: 1, ClassResearch: 1, ClassDefault: 1, "typo": 1,
	}}); err == nil {
		t.Fatal("a limiter with an unknown class was built")
	}
}

func TestAnUnknownClassIsStillBounded(t *testing.T) {
	// Defence in depth: if a route is classified into a class nobody budgeted, the
	// answer is the default class's budget rather than "allowed".
	limiter, err := New(testConfig(map[string]int{ClassDefault: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if !limiter.Allow("a-class-nobody-budgeted", "192.0.2.9").Allowed {
		t.Fatal("the first request in an unbudgeted class was refused")
	}
	second := limiter.Allow("a-class-nobody-budgeted", "192.0.2.9")
	if second.Allowed {
		t.Fatal("an unbudgeted class allowed everything")
	}
	if second.Class != ClassDefault {
		t.Fatalf("an unbudgeted class was charged to %q, want the default class", second.Class)
	}
}

func TestTheMapIsBoundedAndEvictsIdleWindows(t *testing.T) {
	config := testConfig(map[string]int{ClassDefault: 1})
	clock := time.Unix(1_700_000_000, 0)
	config.now = func() time.Time { return clock }
	limiter, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxTrackedClients+500; i++ {
		limiter.Allow(ClassDefault, fmt.Sprintf("198.51.100.%d.%d", i/256, i%256))
	}
	if tracked := limiter.Tracked(); tracked > maxTrackedClients+500 {
		t.Fatalf("tracked = %d", tracked)
	}
	// Move past every window, then one more request must trigger the sweep.
	clock = clock.Add(2 * window)
	limiter.Allow(ClassDefault, "198.51.100.255.255")
	if tracked := limiter.Tracked(); tracked > 10 {
		t.Fatalf("after every window closed the limiter still tracks %d clients; the bound is not doing anything", tracked)
	}
}

func TestConcurrentRequestsAreCountedOnceEach(t *testing.T) {
	// A limiter that loses updates under concurrency is a limiter with a budget
	// slightly larger than it claims, which is the kind of bug that only shows up
	// under load and is invisible in a sequential test.
	limiter, err := New(testConfig(map[string]int{ClassDefault: 50}))
	if err != nil {
		t.Fatal(err)
	}
	var allowed int64
	var mu sync.Mutex
	var group sync.WaitGroup
	for i := 0; i < 200; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if limiter.Allow(ClassDefault, "192.0.2.44").Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	group.Wait()
	if allowed != 50 {
		t.Fatalf("%d of 200 concurrent requests were allowed, want exactly 50", allowed)
	}
}

func TestDescribeIsStable(t *testing.T) {
	// Two restarts should log the same line, which means the class order has to be
	// fixed rather than map order.
	first := DefaultConfig().Describe()
	for i := 0; i < 20; i++ {
		if again := DefaultConfig().Describe(); again != first {
			t.Fatalf("Describe is not stable: %q then %q", first, again)
		}
	}
	if !strings.Contains(first, "auth=30") {
		t.Fatalf("Describe does not report the auth budget: %q", first)
	}
}

func TestReset(t *testing.T) {
	limiter, err := New(testConfig(map[string]int{ClassDefault: 1}))
	if err != nil {
		t.Fatal(err)
	}
	_ = limiter.Allow(ClassDefault, "192.0.2.1")
	if limiter.Allow(ClassDefault, "192.0.2.1").Allowed {
		t.Fatal("the budget was not spent")
	}
	limiter.Reset()
	if tracked := limiter.Tracked(); tracked != 0 {
		t.Fatalf("Reset left %d clients tracked", tracked)
	}
	if !limiter.Allow(ClassDefault, "192.0.2.1").Allowed {
		t.Fatal("Reset did not clear the counters")
	}
}
