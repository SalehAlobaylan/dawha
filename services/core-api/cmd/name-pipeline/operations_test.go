package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorCommandsAuthenticateOnlyToCoreAndRedactPrivateFields(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000024"
	file := "00000000-0000-4000-8000-000000000025"
	token := "fixture-session-token-0123456789"
	privateName := "private-family-name"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			if r.Header.Get("Cookie") != "" {
				t.Error("session sent before service verification")
			}
			json.NewEncoder(w).Encode(map[string]string{"service": "core-api"})
			return
		}
		calls++
		if !strings.Contains(r.Header.Get("Cookie"), token) {
			t.Error("missing authentication")
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "sourceFileId": file, "status": "queued", "stage": "queued", "detected": 3, "errorCode": privateName, "config": map[string]string{"readerVersion": privateName}})
	}))
	defer server.Close()
	session := filepath.Join(t.TempDir(), "session")
	if err := os.WriteFile(session, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	cursor := filepath.Join(t.TempDir(), "cursor.json")
	t.Setenv("NAME_PIPELINE_CURSOR_FILE", cursor)
	for _, args := range [][]string{{"prepare", "--file", file}, {"status"}, {"resume"}, {"cancel"}, {"local-reread", "--suggestion", file}} {
		args = append(args, "--api-url", server.URL, "--session-file", session, "--json")
		var output bytes.Buffer
		if code := runOperatorCommand(args, &output); code != 0 {
			t.Fatalf("operator exit=%d output=%s", code, output.String())
		}
		for _, secret := range []string{id, file, privateName, token, session, server.URL} {
			if strings.Contains(output.String(), secret) {
				t.Fatalf("private operator output contained %q", secret)
			}
		}
	}
	if calls != 5 {
		t.Fatalf("calls=%d", calls)
	}
	info, err := os.Stat(cursor)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("cursor was not private")
	}
}
func TestOperatorRefusesWrongServiceAndExposedSessionFile(t *testing.T) {
	apiCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("sent credential to unverified service")
		}
		if r.URL.Path != "/healthz" {
			apiCalls++
		}
		json.NewEncoder(w).Encode(map[string]string{"service": "ai-research"})
	}))
	defer server.Close()
	session := filepath.Join(t.TempDir(), "session")
	os.WriteFile(session, []byte("fixture-session-token-0123456789"), 0600)
	var output bytes.Buffer
	code := runOperatorCommand([]string{"status", "--api-url", server.URL, "--session-file", session, "--json"}, &output)
	if code != 3 || apiCalls != 0 {
		t.Fatal("wrong service accepted")
	}
	os.Chmod(session, 0644)
	output.Reset()
	if code = runOperatorCommand([]string{"status", "--api-url", server.URL, "--session-file", session}, &output); code != 2 {
		t.Fatal("public session file accepted")
	}
	if _, err := localCoreURL("https://provider.example"); err == nil {
		t.Fatal("remote operator endpoint accepted")
	}
}
func TestDoctorRecognizesTheImplementedLocalReader(t *testing.T) {
	result := diagnose(context.Background(), func(string) string { return "" }, doctorDependencies{Probe: func(context.Context) sourceprocessing.LocalReaderStatus {
		return sourceprocessing.LocalReaderStatus{Reason: "configured", Version: sourceprocessing.LocalCropReaderVersion, Languages: []string{"ara"}}
	}, Health: func(context.Context) bool { return true }})
	if result.ExitCode != 0 || !result.Reader.LocalReader.Ready {
		t.Fatalf("doctor=%+v", result)
	}
}

func TestEvidenceOperatorPrintsOnlyAggregateLexiconData(t *testing.T) {
	token := "fixture-private-session-token-025"
	file := "00000000-0000-4000-8000-000000000025"
	name := "private-file-name"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			if r.Header.Get("Cookie") != "" {
				t.Error("credential sent before identity")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"service": "core-api"})
			return
		}
		if !strings.Contains(r.Header.Get("Cookie"), token) {
			t.Error("missing session")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": []map[string]any{{"value": name, "count": 302}}, "revision": file, "totalOccurrences": 302, "totalSpellings": 1})
	}))
	defer server.Close()
	session := filepath.Join(t.TempDir(), "session")
	if err := os.WriteFile(session, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runOperatorCommand([]string{"lexicon", "--file", file, "--api-url", server.URL, "--session-file", session, "--json"}, &output); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), `"occurrences":302`) {
		t.Fatal("missing counts")
	}
	for _, private := range []string{token, file, name, session} {
		if strings.Contains(output.String(), private) {
			t.Fatal("private lexicon data printed")
		}
	}
}

func TestOperatorFreezesAPrivateVersionContractWithoutOverwritingIt(t *testing.T) {
	contract := map[string]string{"pool": sourceprocessing.NamePoolVersion, "normalizer": "ar-name-keys-v1", "evidence": sourceprocessing.NameEvidenceVersion, "generation": sourceprocessing.NameGenerationVersion, "lexicon": sourceprocessing.NameLexiconVersion, "renderer": sourceprocessing.NameRendererVersion, "geometry": sourceprocessing.NameGeometryVersion, "preprocessing": sourceprocessing.NamePreprocessingVersion, "reader": "fixture-v2", "judgeModel": "d1:3b", "judgePrompt": "name-judge-liquid-v1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_ = json.NewEncoder(w).Encode(map[string]string{"service": "core-api"})
		} else {
			_ = json.NewEncoder(w).Encode(contract)
		}
	}))
	defer server.Close()
	session := filepath.Join(t.TempDir(), "session")
	if err := os.WriteFile(session, []byte("fixture-private-session-token-025"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "contract.json")
	args := []string{"evaluation-contract", "--file", "00000000-0000-4000-8000-000000000025", "--api-url", server.URL, "--session-file", session, "--contract-file", path}
	var output bytes.Buffer
	if code := runOperatorCommand(args, &output); code != 0 {
		t.Fatalf("contract failed: %d %s", code, output.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("contract permissions")
	}
	if code := runOperatorCommand(args, &output); code != 2 {
		t.Fatal("frozen contract overwritten")
	}
}
