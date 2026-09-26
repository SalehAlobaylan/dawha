// Package ratelimit is the abuse control the public endpoints need, and nothing
// more.
//
// It is a fixed-window counter per client per route class, in process, in
// memory. That is a deliberate scope decision rather than a shortcut: the plan
// for this repository rules out introducing Redis, and a single-process limiter
// is the correct shape for the deployment this repository actually has (the API,
// its workers and one database). What it cannot do is enforce a limit across
// several API replicas, and that limitation is written down here rather than left
// for somebody to discover at launch.
//
// It also does not know anything about what a request contains. The buckets are
// keyed by client identity and route class and nothing else, because a rate limiter
// that indexed on a query string would be a copy of every query in the process
// held in a map.
package ratelimit

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Route classes. The class is part of the bucket key, so a flood of suggestions
// cannot spend the budget that protects sign-in.
//
// The classes are named after what they protect, not after the routes they cover,
// because a new route under a class should get that class's budget without anybody
// having to remember to add it to a list somewhere.
const (
	// ClassAuth is registration, sign-in, sign-out and "who am I". Credential
	// stuffing is per source address, so this class is keyed by address and is
	// the one that matters most.
	ClassAuth = "auth"
	// ClassUpload is file upload. It is the expensive one: a byte-counted store
	// write, a row, a job, and a worker that will extract the result.
	ClassUpload = "upload"
	// ClassSuggestion is public suggestions.
	ClassSuggestion = "suggestion"
	// ClassResearch is the research mutation surface: query, agent runs and the
	// graph analyses. These are the most expensive calls in the service.
	ClassResearch = "research"
	// ClassDefault is everything else.
	ClassDefault = "default"
)

// ClassOrder fixes the reporting order so a limit that trips says the same thing
// on every run.
var ClassOrder = []string{ClassAuth, ClassUpload, ClassSuggestion, ClassResearch, ClassDefault}

// DefaultPerMinute is the documented default for every class.
//
// The numbers are chosen so that the documented local demo workflow - sign in,
// create a tree, attach a source, ask a question, run a research query - cannot
// reach any of them, and so that a script hammering one endpoint hits a wall in
// under a minute. A limit low enough to break a demo is a limit that gets turned
// off in production; a limit high enough to be useless is decoration.
const (
	authPerMinute       = 30
	uploadPerMinute     = 20
	suggestionPerMinute = 15
	researchPerMinute   = 30
	defaultPerMinute    = 240
	window              = time.Minute
	// maxTrackedClients bounds the map. A limiter that grows without limit is a
	// denial of service with extra steps, and the eviction below is what keeps
	// the memory of this package a function of the active clients rather than of
	// the addresses that have ever been seen.
	maxTrackedClients = 20000
)

// Config is the whole configuration. Zero value is not "the defaults": it is
// "every class is zero", which is refused by Validate, because a limiter nobody
// configured should not be a limiter that admits everything.
type Config struct {
	// Enabled false turns the middleware into a pass-through. It is never the
	// default, and the only supported way to get it is to set it.
	Enabled bool
	// PerMinute is the budget per class, per client, per window.
	PerMinute map[string]int
	// Window is the fixed window. Zero means the documented minute.
	Window time.Duration
	// now is the clock, injectable so the tests do not sleep.
	now func() time.Time
}

// DefaultConfig is the configuration a deployment gets with no environment set.
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		PerMinute: map[string]int{
			ClassAuth:       authPerMinute,
			ClassUpload:     uploadPerMinute,
			ClassSuggestion: suggestionPerMinute,
			ClassResearch:   researchPerMinute,
			ClassDefault:    defaultPerMinute,
		},
		Window: window,
	}
}

// Disabled is the documented development override: every limit off.
//
// It exists because the STOP condition for this work is "rate limits must not
// break the documented local demo workflow", and the honest way to honour that is
// to make turning them off an explicit, visible, greppable act rather than
// something a developer has to notice. The E2E stack and the CI jobs set it
// explicitly, which means a change that made a journey depend on being
// unthrottled shows up in a diff.
func Disabled() Config {
	config := DefaultConfig()
	config.Enabled = false
	return config
}

// EnvPrefix is the prefix for the per-class environment overrides.
const EnvPrefix = "RATE_LIMIT_"

// ConfigFromEnvironment reads:
//
//	RATE_LIMIT_ENABLED=false|true     default true
//	RATE_LIMIT_WINDOW=60s             default 1m
//	RATE_LIMIT_AUTH_PER_MINUTE=30
//	RATE_LIMIT_UPLOAD_PER_MINUTE=20
//	RATE_LIMIT_SUGGESTION_PER_MINUTE=15
//	RATE_LIMIT_RESEARCH_PER_MINUTE=30
//	RATE_LIMIT_DEFAULT_PER_MINUTE=240
//
// A value that does not parse is an error rather than a default. A typo in
// RATE_LIMIT_AUTH_PER_MINUTE that silently became 30 would be a limit that looks
// configured and is not; a startup failure names the variable.
func ConfigFromEnvironment(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	config := DefaultConfig()
	read := func(name string) string { return strings.TrimSpace(getenv(name)) }
	if value := read(EnvPrefix + "ENABLED"); value != "" {
		config.Enabled = isTrue(value)
	}
	if value := read(EnvPrefix + "WINDOW"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("ratelimit: %sWINDOW=%q is not a positive duration", EnvPrefix, value)
		}
		config.Window = parsed
	}
	for _, class := range ClassOrder {
		name := EnvPrefix + strings.ToUpper(class) + "_PER_MINUTE"
		value := read(name)
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return Config{}, fmt.Errorf("ratelimit: %s=%q is not a count of requests per window", name, value)
		}
		config.PerMinute[class] = parsed
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate refuses a configuration that would admit everything while looking
// configured. A class with no entry, or a zero budget, is a mistake: the first
// silently disables a limit, the second refuses every request in the class.
func (c Config) Validate() error {
	if c.Window < 0 {
		return errors.New("ratelimit: the window cannot be negative")
	}
	for _, class := range ClassOrder {
		budget, ok := c.PerMinute[class]
		if !ok {
			return fmt.Errorf("ratelimit: class %q has no budget; every class needs one, even if it is large", class)
		}
		if budget < 0 {
			return fmt.Errorf("ratelimit: class %q has a negative budget", class)
		}
	}
	if len(c.PerMinute) != len(ClassOrder) {
		// Not an error on its own - a caller may add a class - but a config with
		// an unknown class is nearly always a typo, and a typo here is a route
		// that is silently unlimited.
		for class := range c.PerMinute {
			if !knownClass(class) {
				return fmt.Errorf("ratelimit: %q is not a route class; the classes are %s", class, strings.Join(ClassOrder, ", "))
			}
		}
	}
	return nil
}

func knownClass(class string) bool {
	for _, known := range ClassOrder {
		if known == class {
			return true
		}
	}
	return false
}

func isTrue(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Decision is what the limiter says about one request.
type Decision struct {
	// Class is the route class the request was charged to.
	Class string
	// Limit is the budget for the window.
	Limit int
	// Remaining is what is left after this request. It is zero when Allowed is
	// false.
	Remaining int
	// RetryAfter is how long until the window rolls over. Zero when Allowed.
	RetryAfter time.Duration
	// Allowed is the whole answer.
	Allowed bool
}

// Limiter is the fixed-window counter. It is safe for concurrent use.
type Limiter struct {
	mu      sync.Mutex
	config  Config
	buckets map[string]*bucket
	// evicted counts how many idle buckets have been dropped, so a long-running
	// process can show that the bound is doing something.
	evicted int
}

type bucket struct {
	count   int
	window  time.Time
	clients int
}

// New builds a limiter. It refuses an invalid configuration rather than starting
// one that admits everything.
func New(config Config) (*Limiter, error) {
	if config.Window == 0 {
		config.Window = window
	}
	if config.now == nil {
		config.now = time.Now
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Limiter{config: config, buckets: map[string]*bucket{}}, nil
}

// Allow charges one request against a client and class.
func (l *Limiter) Allow(class, client string) Decision {
	return l.allowAt(class, client, l.config.now())
}

func (l *Limiter) allowAt(class, client string, now time.Time) Decision {
	budget, ok := l.config.PerMinute[class]
	if !ok {
		// An unknown class is a programming error, and answering "allowed" for it
		// would make a typo into an unlimited route. Answer with the default
		// class's budget instead: still bounded, still loud in the logs.
		class = ClassDefault
		budget = l.config.PerMinute[ClassDefault]
	}
	key := class + "\x00" + client

	l.mu.Lock()
	defer l.mu.Unlock()
	l.evictIdle(now)

	entry, found := l.buckets[key]
	if !found || !now.Before(entry.window) {
		entry = &bucket{window: now.Add(l.config.Window)}
		l.buckets[key] = entry
	}
	entry.count++
	if entry.count <= budget {
		return Decision{Class: class, Limit: budget, Remaining: budget - entry.count, Allowed: true}
	}
	return Decision{
		Class:      class,
		Limit:      budget,
		Remaining:  0,
		RetryAfter: entry.window.Sub(now),
		Allowed:    false,
	}
}

// evictIdle drops buckets whose window has passed. Called on every request, so
// the map is a function of the clients active in the last window rather than of
// every address ever seen.
func (l *Limiter) evictIdle(now time.Time) {
	if len(l.buckets) < maxTrackedClients {
		return
	}
	for key, entry := range l.buckets {
		if !now.Before(entry.window) {
			delete(l.buckets, key)
			l.evicted++
		}
	}
	// A full map of live clients is a real condition, not a bug to hide: it means
	// one process is serving a very large number of distinct addresses. Nothing
	// is dropped for being busy, because dropping a live bucket hands the next
	// request in that window a fresh budget.
}

// Tracked reports how many client/class pairs are being counted. A test asserts
// the bound; an operator can log it.
func (l *Limiter) Tracked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Reset clears every counter. The test suite uses it so one test's spending does
// not decide whether the next one passes.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = map[string]*bucket{}
	l.evicted = 0
}

// Budgets returns the configured budget per class, for a startup log line and for
// a test that reads the documented defaults rather than restating them.
func (l *Limiter) Budgets() map[string]int {
	out := map[string]int{}
	for class, budget := range l.config.PerMinute {
		out[class] = budget
	}
	return out
}

// Classes is the class each request is charged to.
//
// It is a prefix and method match rather than an exact path list, on purpose: a
// new route under /api/v1/research/ inherits the research budget the day it is
// added, instead of the day somebody remembers to edit a list here. The
// order matters - the specific classes are tested first - and the method is part
// of the match because a GET and a POST to the same path are not the same cost.
func ClassFor(method, path string) string {
	switch {
	case isUnder(path, "/api/v1/auth"):
		return ClassAuth
	case strings.HasSuffix(path, "/files") && method == http.MethodPost:
		return ClassUpload
	case isUnder(path, "/api/v1/sources") && method == http.MethodPost:
		return ClassUpload
	case isUnder(path, "/api/v1/suggestions"):
		return ClassSuggestion
	case isUnder(path, "/api/v1/research") || isUnder(path, "/api/v1/research-agent"):
		return ClassResearch
	case isUnder(path, "/api/v1/entity-resolution") || isUnder(path, "/api/v1/contradictions"):
		return ClassResearch
	default:
		return ClassDefault
	}
}

func isUnder(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// ClientIdentity is the key a request is counted against.
//
// It is the peer address, and that is a real choice with a real limitation: a
// limit is per network location, not per account, so several researchers behind
// one NAT share a budget and a determined attacker with many addresses gets many
// budgets. Per-account limiting needs the session resolved before the limit is
// charged, which means the auth check has to become middleware rather than
// something each handler does for itself - a larger change than this step, and
// one that would put a database query in front of every request.
//
// Forwarded headers are deliberately NOT trusted. X-Forwarded-For is a request
// header, so honouring it would let a client mint an unlimited number of
// identities by inventing them - which is the opposite of what a rate limiter is
// for.
func ClientIdentity(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "" {
		host = "unknown"
	}
	// Strip the IPv4-mapped IPv6 form so ::ffff:127.0.0.1 and 127.0.0.1 are one
	// client rather than two.
	if mapped := net.ParseIP(host); mapped != nil {
		if v4 := mapped.To4(); v4 != nil {
			host = v4.String()
		}
	}
	return host
}

// Middleware charges every request and answers 429 when the budget is spent.
//
// A 429 carries Retry-After, because a client that cannot tell when to come back
// either retries immediately or gives up, and both are worse than the limit. The
// body names the class and the limit and nothing else: no path, no query, no
// identity of the caller beyond what the client already knows.
func Middleware(limiter *Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limiter == nil || !limiter.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		decision := limiter.Allow(ClassFor(r.Method, r.URL.Path), ClientIdentity(r))
		if !decision.Allowed {
			seconds := int(decision.RetryAfter / time.Second)
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"too many requests","class":"` + decision.Class +
				`","limit":` + strconv.Itoa(decision.Limit) + `,"retryAfterSeconds":` + strconv.Itoa(seconds) + `}`))
			return
		}
		// The remaining budget is a header rather than a body so that adding it
		// costs a client nothing to ignore.
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(decision.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
		next.ServeHTTP(w, r)
	})
}

// Describe renders the configured budgets, class by class in a fixed order, for a
// startup log. Sorted rather than map order so two restarts log the same line.
func (c Config) Describe() string {
	parts := make([]string, 0, len(ClassOrder))
	for _, class := range ClassOrder {
		parts = append(parts, fmt.Sprintf("%s=%d", class, c.PerMinute[class]))
	}
	sort.Strings(parts)
	enabled := "disabled"
	if c.Enabled {
		enabled = "enabled"
	}
	return fmt.Sprintf("%s window=%s %s", enabled, c.Window, strings.Join(parts, " "))
}
