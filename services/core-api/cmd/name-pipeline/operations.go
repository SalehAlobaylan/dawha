package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/google/uuid"
)

type operatorSummary struct {
	Status              string           `json:"status"`
	Stage               string           `json:"stage"`
	Pages               int              `json:"pages"`
	Detected            int              `json:"detected"`
	Named               int              `json:"named"`
	Unreadable          int              `json:"unreadable"`
	Unavailable         int              `json:"unavailable"`
	SourcePixelWarnings int              `json:"sourcePixelWarnings"`
	ManualReview        int              `json:"manualReview"`
	CompletedUnits      int              `json:"completedUnits"`
	TotalUnits          int              `json:"totalUnits"`
	FailedUnits         int              `json:"failedUnits"`
	CacheHits           int              `json:"cacheHits"`
	Timings             map[string]int64 `json:"timings"`
	ErrorCode           string           `json:"errorCode,omitempty"`
	ReaderVersion       string           `json:"readerVersion"`
}
type privateRunCursor struct{ API, Run string }

func summarizeOperatorRun(run sourceprocessing.NameReadingRun) operatorSummary {
	code := ""
	switch run.ErrorCode {
	case "run_budget_exhausted", "invalid_source_or_frame", "storage_unavailable", "render_tools_unavailable", "local_preparation_failed", "worker_attempts_exhausted":
		code = run.ErrorCode
	case "":
	default:
		code = "preparation_failed"
	}
	reader := run.Config.Reader
	if !validReaderVersion(reader) {
		reader = "other"
	}
	timings := map[string]int64{}
	for _, key := range []string{"detectionMs", "readingMs"} {
		if value, ok := run.Timings[key]; ok && value >= 0 {
			timings[key] = value
		}
	}
	return operatorSummary{Status: run.Status, Stage: run.Stage, Pages: run.Pages, Detected: run.Detected, Named: run.Named, Unreadable: run.Unreadable, Unavailable: run.Unavailable, ManualReview: run.ManualReview, SourcePixelWarnings: run.SourcePixelWarnings, CompletedUnits: run.CompletedUnits, TotalUnits: run.TotalUnits, FailedUnits: run.FailedUnits, CacheHits: run.CacheHits, Timings: timings, ErrorCode: code, ReaderVersion: reader}
}
func validReaderVersion(value string) bool {
	if value == sourceprocessing.LocalCropReaderVersion {
		return true
	}
	prefix := sourceprocessing.LocalCropReaderVersion + "+"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+16 {
		return false
	}
	for _, r := range value[len(prefix):] {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func readOperatorSession(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return "", errors.New("private session file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("private session file required")
	}
	value := strings.TrimSpace(string(data))
	if len(value) < 16 || strings.ContainsAny(value, "\r\n\t ;") {
		return "", errors.New("invalid session file")
	}
	return value, nil
}
func localCoreURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("local Core API URL required")
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return "", errors.New("local Core API URL required")
	}
	return strings.TrimRight(raw, "/"), nil
}
func operatorAPIRequest(ctx context.Context, client *http.Client, base, token, method, path string) (sourceprocessing.NameReadingRun, int) {
	var result sourceprocessing.NameReadingRun
	request, err := http.NewRequestWithContext(ctx, method, base+path, nil)
	if err != nil {
		return result, 3
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	}
	response, err := client.Do(request)
	if err != nil {
		return result, 3
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, 2
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || json.Unmarshal(body, &result) != nil {
		return result, 3
	}
	switch result.Status {
	case "queued", "running", "partial", "succeeded", "failed", "cancelled":
	default:
		return result, 3
	}
	if _, err = uuid.Parse(result.ID); err != nil {
		return result, 3
	}
	switch result.Stage {
	case "queued", "preparing", "detecting", "reading", "complete":
	default:
		return result, 3
	}
	return result, 0
}
func verifyCoreAPI(ctx context.Context, client *http.Client, base string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	var status struct {
		Service string `json:"service"`
	}
	return response.StatusCode == 200 && json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&status) == nil && status.Service == "core-api"
}
func saveOperatorCursor(path, base, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".cursor-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err = json.NewEncoder(file).Encode(privateRunCursor{base, id}); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
func loadOperatorCursor(path, base string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return "", errors.New("private run cursor missing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var cursor privateRunCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.API != base {
		return "", errors.New("run cursor does not match API")
	}
	if _, err = uuid.Parse(cursor.Run); err != nil {
		return "", err
	}
	return cursor.Run, nil
}
func runOperatorCommand(args []string, output io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "lexicon", "evidence-status", "evaluation-contract", "evaluation-register":
			return runEvidenceOperator(args, output)
		}
	}
	switch args[0] {
	case "prepare", "status", "resume", "cancel", "local-reread":
	default:
		fmt.Fprintln(output, "usage: name-pipeline doctor|prepare|status|resume|cancel|local-reread")
		return 2
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	rawBase := os.Getenv("NAME_PIPELINE_API_URL")
	if rawBase == "" {
		rawBase = "http://localhost:8080"
	}
	baseFlag := flags.String("api-url", rawBase, "local Core API URL")
	sessionPath := flags.String("session-file", os.Getenv("NAME_PIPELINE_SESSION_FILE"), "mode-0600 file containing the local session token")
	fileID := flags.String("file", "", "internal source file selector")
	runID := flags.String("run", "", "internal run selector; defaults to the private last-run cursor")
	suggestionID := flags.String("suggestion", "", "internal suggestion selector for local reread")
	asJSON := flags.Bool("json", false, "aggregate summary without names or identifiers")
	cursorPath := os.Getenv("NAME_PIPELINE_CURSOR_FILE")
	if cursorPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return 2
		}
		cursorPath = filepath.Join(home, ".dawha", "name-pipeline", "last-run.json")
	}
	fail := func(message string, code int) int {
		if *asJSON {
			_ = json.NewEncoder(output).Encode(map[string]any{"error": message, "exitCode": code})
		} else {
			fmt.Fprintln(output, message)
		}
		return code
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return fail("invalid operator arguments", 2)
	}
	base, err := localCoreURL(*baseFlag)
	if err != nil {
		return fail("local Core API URL required", 2)
	}
	for _, id := range []string{*fileID, *runID, *suggestionID} {
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return fail("invalid internal selector", 2)
			}
		}
	}
	token, err := readOperatorSession(*sessionPath)
	if err != nil {
		return fail("a private session file is required", 2)
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	// Verify service identity with metadata before sending a cookie or source selector.
	if !verifyCoreAPI(ctx, client, base) {
		return fail("local Core API is unavailable or has the wrong service identity", 3)
	}
	path, method := "", http.MethodGet
	switch args[0] {
	case "prepare":
		if *fileID == "" {
			return fail("a file selector is required", 2)
		}
		path = "/api/v1/source-files/" + *fileID + "/suggestions/read"
		method = http.MethodPost
	case "local-reread":
		if *suggestionID == "" {
			return fail("a suggestion selector is required", 2)
		}
		path = "/api/v1/source-tree-suggestions/" + *suggestionID + "/local-reread"
		method = http.MethodPost
	default:
		if *runID == "" && *fileID != "" && args[0] == "status" {
			path = "/api/v1/source-files/" + *fileID + "/name-reading-runs/latest"
		} else {
			if *runID == "" {
				*runID, err = loadOperatorCursor(cursorPath, base)
				if err != nil {
					return fail("a run selector or private cursor is required", 2)
				}
			}
			path = "/api/v1/name-reading-runs/" + *runID
			if args[0] != "status" {
				path += "/" + args[0]
				method = http.MethodPost
			}
		}
	}
	result, code := operatorAPIRequest(ctx, client, base, token, method, path)
	if code != 0 {
		return fail("the operation was refused or is unavailable", code)
	}
	if err = saveOperatorCursor(cursorPath, base, result.ID); err != nil {
		return fail("could not save the private run cursor", 2)
	}
	summary := summarizeOperatorRun(result)
	if *asJSON {
		_ = json.NewEncoder(output).Encode(summary)
	} else {
		encoded, _ := json.Marshal(summary)
		fmt.Fprintln(output, string(bytes.TrimSpace(encoded)))
	}
	if result.Status == "failed" || result.Status == "partial" {
		return 2
	}
	return 0
}
