package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
)

func TestDoctorExitCodesAndAggregateOutput(t *testing.T) {
	for _, test := range []struct {
		health bool
		judge  string
		want   int
	}{{true, "off", 2}, {false, "shadow", 3}, {false, "off", 2}} {
		result := diagnose(context.Background(), func(key string) string {
			if key == "NAME_JUDGE_MODE" {
				return test.judge
			}
			return ""
		}, doctorDependencies{
			Probe: func(context.Context) sourceprocessing.LocalReaderStatus {
				return sourceprocessing.LocalReaderStatus{Reason: "local_reader_not_implemented"}
			},
			Health: func(context.Context) bool { return test.health }, Evidence: func(context.Context) evidenceGaps {
				return evidenceGaps{Available: true, Decisions: 111, TruthLabels: 80}
			},
		})
		// Stage 023 deliberately does not permit installed tools or an injected probe
		// to assert an adapter exists. Readiness will be implemented in Plan 024.
		want := test.want
		if result.ExitCode != want {
			t.Fatalf("exit=%d want=%d", result.ExitCode, want)
		}
		encoded, _ := json.Marshal(result)
		for _, secret := range []string{"sourceId", "reviewerId", "apiKey", "image_base64"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("unexpected private field %s", secret)
			}
		}
	}
}
func TestDoctorHealthUsesNoInferenceOrRedirects(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/healthz" || r.ContentLength > 0 {
			t.Error("unexpected request")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if !serviceHealthy(context.Background(), server.Client(), server.URL) || calls != 1 {
		t.Fatal("health probe failed")
	}
}
