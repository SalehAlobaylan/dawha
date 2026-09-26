package httpapi

import (
	"net/http"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dashboard"
)

// Demo mode: the one setting in this service that decides whether a synthetic
// answer may be served at all.
//
// The route used to return the demo dashboard unconditionally, with no database,
// no session and no setting. That is the failure the plan calls out by name: a
// static payload that looks exactly like real data is indistinguishable from real
// data to a reader, and a deployment whose database was down would serve
// confident numbers instead of an error. So the static response is now behind
// DEMO_MODE, and with it off the route answers a dependency error - the same 503
// shape every other unavailable dependency in this service uses, which a client
// already knows how to handle.
//
// DEMO_MODE is not an error fallback and it is not inferred. It is off unless it
// is set, the local development stack sets it explicitly, and the browser
// acceptance stack sets it explicitly. The default is the production posture.

const demoModeEnv = "DEMO_MODE"

// demoModeFromEnvironment reads DEMO_MODE. Unset, empty, "0", "false" and "no"
// are all off; anything else that reads as a truthy word is on. A value that is
// neither is off as well, because the failure mode of a typo here - a deployment
// serving synthetic data believing it is real - is worse than the failure mode of
// the opposite mistake.
func demoModeFromEnvironment(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return demoModeEnabled(getenv(demoModeEnv))
}

// dashboardHandler answers GET /api/v1/dashboard.
//
// It is its own type with no service behind it, which is the point: the static
// payload needs no database, no session and no dependency, and a handler that
// looked like it needed them would invite somebody to add a nil check to it one
// day and change what "off" means.
type dashboardHandler struct {
	// demo is the DEMO_MODE setting. False - the zero value, and the default
	// everywhere - means the route answers a dependency error.
	demo bool
}

func (h dashboardHandler) dashboard(w http.ResponseWriter, _ *http.Request) {
	// No-store in both branches. A cached demo dashboard outlives the process that
	// decided to serve it, which is how demo data becomes the thing a reader sees
	// after demo mode has been turned off.
	w.Header().Set("Cache-Control", "no-store")
	if !h.demo {
		// A 503 rather than a 404: the route exists and the thing missing is a
		// capability, not a resource. The body names the setting, so whoever is
		// deploying can act on it without reading this file - a 503 with a sentence
		// is a configuration problem and a 503 with a shrug is a mystery.
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "the dashboard has no real implementation yet; set DEMO_MODE=true to serve the labelled demo payload, or leave it off and treat this as a dependency that is not available",
			"code":  "dashboard_unavailable",
			"demo":  false,
		})
		return
	}
	// Labelled in three places at once - the header, the `mode` field and the
	// `demo` field - because a client that reads only one of them should still be
	// able to tell what it is holding.
	w.Header().Set("X-Data-Source", "demo")
	writeJSON(w, http.StatusOK, dashboard.Demo())
}

// demoModeEnabled parses a DEMO_MODE value. It is the single reader of the
// setting's words: demoModeFromEnvironment is the only thing that names the
// variable, and this is the only thing that decides what a value means, so the
// router, the environment and the tests cannot drift apart on it.
func demoModeEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
