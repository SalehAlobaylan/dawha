// Command name-pipeline diagnoses local prerequisites without reading a source
// or asking a model to infer anything. Output contains aggregate metadata only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/jackc/pgx/v5"
)

type evidenceGaps struct {
	Available           bool `json:"available"`
	Decisions           int  `json:"decisions"`
	TruthLabels         int  `json:"truthLabels"`
	MissingSnapshots    int  `json:"missingSnapshots"`
	MissingJudgeOutputs int  `json:"missingJudgeOutputs"`
	LegacyPoolVersions  int  `json:"legacyPoolVersions"`
}
type judgePolicy struct {
	Mode   string  `json:"mode"`
	Images bool    `json:"imageEnabled"`
	Sample float64 `json:"sample"`
}
type diagnostic struct {
	JudgePolicy       judgePolicy                          `json:"judgePolicy"`
	Reader            sourceprocessing.ReadingCapabilities `json:"reader"`
	AIService         string                               `json:"aiService"`
	AIServiceRequired bool                                 `json:"aiServiceRequired"`
	Versions          map[string]string                    `json:"versions"`
	Evidence          evidenceGaps                         `json:"evidence"`
	Reasons           []string                             `json:"reasons"`
	ExitCode          int                                  `json:"exitCode"`
}
type doctorDependencies struct {
	Probe    func(context.Context) sourceprocessing.LocalReaderStatus
	Health   func(context.Context) bool
	Evidence func(context.Context) evidenceGaps
	Hosted   sourceprocessing.ReadingStatusProvider
}

func diagnose(ctx context.Context, getenv func(string) string, deps doctorDependencies) diagnostic {
	settings := sourceprocessing.ReadingSettingsFromEnvironment(getenv)
	service := sourceprocessing.Service{ReaderSettings: settings, ReaderStatus: deps.Hosted, LocalReadingProbe: deps.Probe}
	result := diagnostic{Reader: service.ReadingConfiguration(ctx), Versions: sourceprocessing.ReadingPipelineVersions(), Reasons: []string{}, AIService: "unreachable"}
	judge := sourceprocessing.NameJudgeSettingsFromEnvironment(getenv)
	result.JudgePolicy = judgePolicy{Mode: judge.Mode, Images: judge.ImageEnabled, Sample: judge.Sample}
	result.AIServiceRequired = settings.HostedEnabled || judge.Mode != "off"
	if deps.Health(ctx) {
		result.AIService = "reachable"
	}
	if deps.Evidence != nil {
		result.Evidence = deps.Evidence(ctx)
	}
	rawMode := strings.ToLower(strings.TrimSpace(getenv("NAME_READER_MODE")))
	if rawMode != "" && rawMode != "local" && rawMode != "off" && rawMode != "hosted" {
		result.Reasons = append(result.Reasons, "invalid_reader_mode")
		result.ExitCode = 2
	}
	if settings.Mode != "local" || !result.Reader.LocalReader.Ready {
		result.Reasons = append(result.Reasons, result.Reader.LocalReader.Reason)
		result.ExitCode = 2
	}
	if result.AIServiceRequired && (result.AIService != "reachable" || (settings.HostedEnabled && result.Reader.HostedReader.Reason == "temporarily_unavailable")) {
		result.Reasons = append(result.Reasons, "required_service_unreachable")
		result.ExitCode = 3
	}
	return result
}

func serviceHealthy(ctx context.Context, client *http.Client, baseURL string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	// Discard bounded health content; it is never forwarded into the report.
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return err == nil && response.StatusCode == http.StatusOK
}

func loadEvidence(ctx context.Context, databaseURL string) evidenceGaps {
	result := evidenceGaps{}
	if databaseURL == "" {
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return result
	}
	defer conn.Close(ctx)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return result
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT count(*),
 count(*) FILTER (WHERE decision IN ('accepted','corrected') AND COALESCE(decided_text_ar,'')<>''),
 count(*) FILTER (WHERE (e.schema_version IS NULL AND d.candidate_snapshot='[]'::jsonb) OR (e.schema_version IS NOT NULL AND p.id IS NULL)),
 count(*) FILTER (WHERE (e.schema_version IS NULL AND d.judge_output='{}'::jsonb) OR (e.schema_version IS NOT NULL AND r.id IS NULL)),
 count(*) FILTER (WHERE d.candidate_pool_version='legacy')
 FROM source_tree_suggestion_decisions d LEFT JOIN source_name_decision_evidence e ON e.decision_id=d.id LEFT JOIN source_name_candidate_preparations p ON p.id=e.preparation_id LEFT JOIN source_name_judge_requests q ON q.preparation_id=p.id LEFT JOIN source_name_judge_results r ON r.request_id=q.id`).Scan(&result.Decisions, &result.TruthLabels, &result.MissingSnapshots, &result.MissingJudgeOutputs, &result.LegacyPoolVersions)
	result.Available = err == nil
	return result
}

func run(args []string, output io.Writer) int {
	if len(args) > 0 && args[0] != "doctor" {
		return runOperatorCommand(args, output)
	}
	if len(args) == 0 {
		fmt.Fprintln(output, "usage: name-pipeline doctor --json")
		return 2
	}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(output)
	asJSON := flags.Bool("json", false, "write aggregate diagnostics as JSON")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return 2
	}
	baseURL := os.Getenv("AI_RESEARCH_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	// Disable redirects so metadata probes cannot follow a service to a provider.
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result := diagnose(ctx, os.Getenv, doctorDependencies{
		Probe:    sourceprocessing.ProbeLocalReadingTools,
		Hosted:   sourceprocessing.NewReadingStatusProvider(baseURL),
		Health:   func(ctx context.Context) bool { return serviceHealthy(ctx, client, baseURL) },
		Evidence: func(ctx context.Context) evidenceGaps { return loadEvidence(ctx, os.Getenv("DATABASE_URL")) },
	})
	if *asJSON {
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		if encoder.Encode(result) != nil {
			return 2
		}
	} else {
		fmt.Fprintf(output, "local reader: %s; AI service: %s; decisions: %d; exit: %d\n", result.Reader.LocalReader.Reason, result.AIService, result.Evidence.Decisions, result.ExitCode)
	}
	return result.ExitCode
}
func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
