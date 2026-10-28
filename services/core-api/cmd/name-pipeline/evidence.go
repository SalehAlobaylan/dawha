package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	sp "github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/google/uuid"
)

// Operator-only diagnostics may describe invisible shadow work. No browser
// review surface subscribes to these summaries or receives a model result.
func runEvidenceOperator(args []string, out io.Writer) int {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseDefault := os.Getenv("NAME_PIPELINE_API_URL")
	if baseDefault == "" {
		baseDefault = "http://localhost:8080"
	}
	baseFlag := flags.String("api-url", baseDefault, "loopback core API")
	session := flags.String("session-file", os.Getenv("NAME_PIPELINE_SESSION_FILE"), "private session file")
	file := flags.String("file", "", "internal file selector")
	partition := flags.String("partition", "development", "development or held_out")
	group := flags.String("group", "", "opaque rendering group hash")
	contractPath := flags.String("contract-file", "", "private frozen version contract")
	_ = flags.Bool("json", false, "aggregate JSON")
	fail := func(message string, code int) int {
		_ = json.NewEncoder(out).Encode(map[string]any{"error": message, "exitCode": code})
		return code
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return fail("invalid operator arguments", 2)
	}
	if _, err := uuid.Parse(*file); err != nil {
		return fail("a valid internal file selector is required", 2)
	}
	base, err := localCoreURL(*baseFlag)
	if err != nil {
		return fail("local Core API URL required", 2)
	}
	token, err := readOperatorSession(*session)
	if err != nil {
		return fail("a private session file is required", 2)
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if !verifyCoreAPI(ctx, client, base) {
		return fail("local Core API unavailable or wrong service identity", 3)
	}
	method, path := http.MethodGet, "/api/v1/source-files/"+*file+"/name-evidence-status"
	var body []byte
	switch args[0] {
	case "lexicon":
		path = "/api/v1/source-files/" + *file + "/name-lexicon"
	case "evaluation-contract":
		path = "/api/v1/source-files/" + *file + "/name-evaluation-contract"
	case "evaluation-register":
		if *contractPath == "" || len(*group) != 64 || (*partition != "development" && *partition != "held_out") {
			return fail("private contract, rendering group and partition are required", 2)
		}
		info, err := os.Lstat(*contractPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
			return fail("private version contract required", 2)
		}
		raw, err := os.ReadFile(*contractPath)
		if err != nil {
			return fail("private version contract required", 2)
		}
		var contract map[string]string
		if json.Unmarshal(raw, &contract) != nil {
			return fail("invalid version contract", 2)
		}
		body, _ = json.Marshal(sp.NameEvaluationRegistration{Partition: *partition, GroupKey: *group, Contract: contract})
		method = http.MethodPost
		path = "/api/v1/source-files/" + *file + "/name-evaluation-registration"
	}
	request, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return fail("operator request failed", 3)
	}
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return fail("operator service unavailable", 3)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fail("operator request refused", 2)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return fail("invalid operator response", 3)
	}
	switch args[0] {
	case "lexicon":
		var lex sp.NameLexicon
		if json.Unmarshal(raw, &lex) != nil {
			return fail("invalid lexicon response", 3)
		}
		_ = json.NewEncoder(out).Encode(map[string]any{"occurrences": lex.TotalOccurrences, "spellings": lex.TotalSpellings, "entries": len(lex.Entries), "truncated": lex.Truncated, "omittedEntries": lex.OmittedEntries, "invalidSpellings": lex.InvalidSpellings, "version": sp.NameLexiconVersion})
	case "evaluation-contract":
		var contract map[string]string
		if json.Unmarshal(raw, &contract) != nil || len(contract) != 11 || contract["pool"] != sp.NamePoolVersion {
			return fail("invalid version contract", 3)
		}
		if *contractPath == "" {
			return fail("a private contract output file is required", 2)
		}
		f, err := os.OpenFile(*contractPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fail("refusing to overwrite a version contract", 2)
		}
		err = json.NewEncoder(f).Encode(contract)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return fail("could not store version contract", 2)
		}
		fmt.Fprintln(out, `{"contractSaved":true,"versions":11}`)
	case "evaluation-register":
		fmt.Fprintln(out, `{"registered":true}`)
	default:
		var stats map[string]int
		if json.Unmarshal(raw, &stats) != nil {
			return fail("invalid evidence summary", 3)
		}
		safe := map[string]int{}
		for _, key := range []string{"exposures", "preparations", "blindEligible", "judgeRequests", "judgeResults", "pendingOrMissingResults", "truncatedContexts"} {
			safe[key] = max(0, stats[key])
		}
		_ = json.NewEncoder(out).Encode(safe)
	}
	return 0
}
