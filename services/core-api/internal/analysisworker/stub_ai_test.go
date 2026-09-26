package analysisworker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// startStubEmbedder is the smallest thing that answers the one call the
// entity-resolution scoring makes.
//
// The scoring asks for an embedding per distinct name, with a per-call budget and
// a documented local fallback. Pointing the worker at an address that refuses the
// connection would exercise the fallback but spend the whole budget doing it, which
// makes a queue test take a minute and a half; answering in microseconds keeps the
// test about the queue.
func startStubEmbedder(t *testing.T, delay time.Duration) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embed" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var request struct {
			Dimensions int `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Dimensions <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		values := make([]float64, request.Dimensions)
		for index := range values {
			values[index] = float64(index%7) + 1
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": values, "dimensions": request.Dimensions, "model": "p006-stub-embedder", "deterministic": true})
	}))
	t.Cleanup(server.Close)
	return server.URL
}
