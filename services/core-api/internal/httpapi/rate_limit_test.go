package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/ratelimit"
)

// Throttling, through the real router, with the real middleware.
//
// The unit tests in platform/ratelimit prove the counter. These prove three
// things only the router can: that the middleware is actually in the chain, that
// a throttled request does not reach a handler, and that a router built with no
// configuration is protected rather than open.

// routerWithLimits builds a router over a store-less dependency set, so the test
// measures the middleware rather than a handler. The handlers in these tests are
// reached with no database, so they answer with their own unavailable response -
// which travels through the limiter exactly like any other response, and is
// precisely what a throttled request must NOT produce.
func routerWithLimits(config ratelimit.Config) http.Handler {
	return NewRouter(Dependencies{RateLimits: config})
}

func TestTheRouterThrottlesAndTheHandlerNeverRuns(t *testing.T) {
	config := ratelimit.DefaultConfig()
	config.PerMinute[ratelimit.ClassDefault] = 2
	router := routerWithLimits(config)

	codes := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil))
		codes = append(codes, recorder.Code)
	}
	if codes[0] == http.StatusTooManyRequests || codes[1] == http.StatusTooManyRequests {
		t.Fatalf("the first two requests were throttled: %v", codes[:2])
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("the third request got %d, want %d: the first two prove the handler answered, so the third proves it did not", codes[2], http.StatusTooManyRequests)
	}
}

func TestTheRouterAnswersAThrottledRequestWithRetryAfter(t *testing.T) {
	config := ratelimit.DefaultConfig()
	config.PerMinute[ratelimit.ClassDefault] = 1
	router := routerWithLimits(config)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil))
	if first.Code == http.StatusTooManyRequests {
		t.Fatalf("the first request was throttled: %d", first.Code)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("a 429 without Retry-After")
	}
	if _, err := strconv.Atoi(second.Header().Get("Retry-After")); err != nil {
		t.Fatalf("Retry-After is not a number: %q", second.Header().Get("Retry-After"))
	}
	var payload struct {
		Error  string `json:"error"`
		Class  string `json:"class"`
		Limit  int    `json:"limit"`
		Retry  int    `json:"retryAfterSeconds"`
		Header string `json:"-"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &payload); err != nil {
		t.Fatalf("the 429 body is not json: %v (%s)", err, second.Body.String())
	}
	if payload.Error == "" || payload.Class != ratelimit.ClassDefault || payload.Limit != 1 {
		t.Fatalf("the 429 body does not describe the limit: %s", second.Body.String())
	}
	// The remaining-budget header is on the allowed response, so a client can see
	// it coming instead of discovering it as a 429.
	if first.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("X-RateLimit-Remaining = %q on the allowed response", first.Header().Get("X-RateLimit-Remaining"))
	}
	if first.Header().Get("X-RateLimit-Limit") != "1" {
		t.Fatalf("X-RateLimit-Limit = %q", first.Header().Get("X-RateLimit-Limit"))
	}
}

func TestTheRouterChargesAuthAndResearchSeparately(t *testing.T) {
	// The route classification has to be in the router, not only in the unit test
	// for ClassFor, or a route added tomorrow inherits the wrong budget.
	config := ratelimit.DefaultConfig()
	config.PerMinute[ratelimit.ClassAuth] = 1
	config.PerMinute[ratelimit.ClassResearch] = 1
	config.PerMinute[ratelimit.ClassDefault] = 1
	router := routerWithLimits(config)

	send := func(method, path string) int {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
		return recorder.Code
	}
	throttled := http.StatusTooManyRequests
	// Exhaust auth.
	send(http.MethodPost, "/api/v1/auth/login")
	if got := send(http.MethodPost, "/api/v1/auth/login"); got != throttled {
		t.Fatalf("the second sign-in got %d, want 429", got)
	}
	// Research and the default class are untouched by that spending.
	if got := send(http.MethodPost, "/api/v1/research/query"); got == throttled {
		t.Fatal("a research call was refused by auth spending")
	}
	if got := send(http.MethodPost, "/api/v1/research/query"); got != throttled {
		t.Fatalf("the second research call got %d, want 429", got)
	}
	if got := send(http.MethodGet, "/api/v1/trees"); got == throttled {
		t.Fatal("an unrelated request was refused by two exhausted classes")
	}
}

func TestARouterWithNoRateLimitConfigurationIsStillProtected(t *testing.T) {
	// The zero Config is what every existing caller passes. If that meant "no
	// limits", every test in the repository and every unmigrated deployment would
	// be running an unprotected API.
	router := NewRouter(Dependencies{})
	throttled := false
	for i := 0; i < 300; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
		if recorder.Code == http.StatusTooManyRequests {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Fatal("a router built with no rate limit configuration throttled nothing in 300 requests")
	}
}

func TestTheDisabledOverrideOpensTheRouter(t *testing.T) {
	// The documented development override, through the router, so it is the E2E
	// stack's escape hatch that is proved rather than assumed.
	router := NewRouter(Dependencies{RateLimits: ratelimit.Disabled()})
	for i := 0; i < 400; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
		if recorder.Code == http.StatusTooManyRequests {
			t.Fatalf("the disabled override throttled request %d", i+1)
		}
	}
}

func TestAnInvalidRateLimitConfigurationStopsTheRouterBeingBuilt(t *testing.T) {
	// A handler that ignored the error would be a limiter that admits everything
	// with one log line somebody reads once. Panicking here is the loud version of
	// refusing, and the process exits before it can serve.
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("an invalid rate limit configuration built a router")
		}
		if message := fmt.Sprint(recovered); message == "" {
			t.Fatal("the panic carries no message")
		}
	}()
	NewRouter(Dependencies{RateLimits: ratelimit.Config{
		Enabled: true,
		// A class nobody budgets, spelled almost like one that is: the exact
		// mistake a hand-written configuration makes, and the one that would
		// otherwise leave a route unlimited.
		PerMinute: map[string]int{
			ratelimit.ClassAuth: 1, ratelimit.ClassUpload: 1, ratelimit.ClassSuggestion: 1,
			ratelimit.ClassResearch: 1, ratelimit.ClassDefault: 1, "auth ": 1,
		},
	}})
}
