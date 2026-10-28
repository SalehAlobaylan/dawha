// Command name-eval reports what the review decisions say about the name
// candidate pipeline.
//
// It is deliberately a read-only report over source_tree_suggestion_decisions,
// not a second labelling project. The reviewer's decision is the label; the
// candidate snapshot is what they saw. A decision without a snapshot is a
// measurement gap, not evidence, and the report says so.
//
// The candidate keys in the snapshot are versioned evidence. This command
// reads them; it does not silently recompute an old decision with a newer
// normalizer. It only computes the truth keys for the reviewer's text with the
// same published normalizer version.
//
//	name-eval
//	name-eval --database-url postgres://... --since 2026-10-01T00:00:00Z
//	name-eval --json
//	name-eval --class-map source-classes.json
//
// The report never prints a candidate name, a decided name, or a file name. The
// optional class map maps a source file UUID to one of the pre-registered
// evaluation classes (clean_print, calligraphic_dense,
// low_resolution_screenshot); without it the 300-decision gate cannot pass.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
)

const normalizerVersion = "ar-name-keys-v1"

const (
	minMeasurableRate   = 100
	minShadowDecisions  = 300
	minBootstrapSamples = 2000
	// A judge cannot fix a candidate set that misses the truth. This is the
	// starting floor from the plan, not a measured threshold; raise it only by
	// recording the reason and the baseline.
	minCandidateRecall = 0.70
)

var expertSourceClasses = []string{
	"clean_print",
	"calligraphic_dense",
	"low_resolution_screenshot",
}

type candidateSnapshot struct {
	Value       string         `json:"value"`
	Key         string         `json:"key"`
	CompactKey  string         `json:"compactKey"`
	ParticleKey string         `json:"particleKey"`
	Source      string         `json:"source"`
	Sources     []string       `json:"sources"`
	Ranks       map[string]int `json:"ranks"`
	Score       float64        `json:"score"`
	Shown       bool           `json:"shown"`
	Position    int            `json:"position"`
}

type judgeOutput struct {
	Mode          string             `json:"mode"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	PromptVersion string             `json:"promptVersion"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence"`
	Abstained     bool               `json:"abstained"`
	Readable      *bool              `json:"readable"`
	Shown         bool               `json:"shown"`
	Used          bool               `json:"used"`
	LatencyMS     int                `json:"latencyMs"`
}

type cohortVersion struct {
	Recipe     string `json:"recipeVersion"`
	Evidence   string `json:"evidenceVersion"`
	Generation string `json:"generationVersion"`
	Renderer   string `json:"rendererVersion"`
	Lexicon    string `json:"lexiconVersion"`
	Partition  string `json:"partition"`
	Pool       string `json:"poolVersion"`
	Reader     string `json:"readerVersion"`
	Model      string `json:"judgeModel"`
	Prompt     string `json:"promptVersion"`
}

func rowVersion(row decisionRow) cohortVersion {
	legacy := func(value string) string {
		if value == "" {
			return "legacy"
		}
		return value
	}
	model, prompt := row.Judge.Model, row.Judge.PromptVersion
	if row.FrozenModel != "" {
		model = row.FrozenModel
	}
	if row.FrozenPrompt != "" {
		prompt = row.FrozenPrompt
	}
	return cohortVersion{Pool: legacy(row.PoolVersion), Reader: legacy(row.ReaderVersion), Model: legacy(model), Prompt: legacy(prompt), Evidence: legacy(row.EvidenceVersion), Generation: legacy(row.GenerationVersion), Renderer: legacy(row.RendererVersion), Lexicon: legacy(row.LexiconVersion), Partition: legacy(row.Partition), Recipe: legacy(row.RecipeVersion)}
}

func selectCohort(rows []decisionRow, selected cohortVersion) []decisionRow {
	result := make([]decisionRow, 0, len(rows))
	for _, row := range rows {
		version := rowVersion(row)
		if (selected.Recipe == "" || selected.Recipe == version.Recipe) && (selected.Evidence == "" || selected.Evidence == version.Evidence) && (selected.Generation == "" || selected.Generation == version.Generation) && (selected.Renderer == "" || selected.Renderer == version.Renderer) && (selected.Partition == "" || selected.Partition == version.Partition) &&
			(selected.Pool == "" || selected.Pool == version.Pool) && (selected.Reader == "" || selected.Reader == version.Reader) &&
			(selected.Model == "" || selected.Model == version.Model) && (selected.Prompt == "" || selected.Prompt == version.Prompt) {
			result = append(result, row)
		}
	}
	return result
}

type decisionRow struct {
	RecipeVersion                                                                  string
	SourceGroup                                                                    string
	ReadingDiagnostics                                                             []string
	EvidenceVersion, GenerationVersion, RendererVersion, LexiconVersion, Partition string
	ExposureID, FrozenModel, FrozenPrompt, InitialGap, JudgeGap                    string
	InitialValid, BlindEligible, ContextTruncated                                  bool
	SuggestionID                                                                   string
	ReviewRevision                                                                 int
	Superseded                                                                     bool
	PoolVersion                                                                    string
	ReaderVersion                                                                  string
	ID                                                                             string
	FileID                                                                         string
	FileName                                                                       string
	Decision                                                                       string
	DecidedText                                                                    string
	Candidates                                                                     []candidateSnapshot
	InputMethod                                                                    string
	NormalizerVersion                                                              string
	DurationMS                                                                     int
	Judge                                                                          judgeOutput
	PageNumber                                                                     int
	ReaderConfidence                                                               float64
	DecisionAt                                                                     time.Time
	MimeType                                                                       string
	PageWidth                                                                      float64
	PageHeight                                                                     float64
	SourceClass                                                                    string
}

type rate struct {
	Matches    int     `json:"matches"`
	Total      int     `json:"total"`
	Rate       float64 `json:"rate"`
	Lower      float64 `json:"lower"`
	Upper      float64 `json:"upper"`
	Measurable bool    `json:"measurable"`
}

type fileReport struct {
	FileID       string `json:"fileId"`
	FileName     string `json:"-"`
	Decisions    int    `json:"decisions"`
	WithSnapshot int    `json:"withSnapshot"`
	Recall       rate   `json:"recallAt8"`
	RecallAny    rate   `json:"recallAnyShown"`
	Top1         rate   `json:"top1"`
	Top3         rate   `json:"top3"`
	Typed        rate   `json:"typed"`
	MedianMS     *int   `json:"medianDurationMs,omitempty"`
}

type sourceCoverageReport struct {
	Source        string `json:"source"`
	Offered       rate   `json:"offered"`
	ContainsTruth rate   `json:"containsTruth"`
	OfferedRecall rate   `json:"offeredRecall"`
	JudgeTop1     rate   `json:"judgeTop1"`
	BaselineTop1  rate   `json:"baselineTop1"`
	JudgeAbstain  rate   `json:"judgeAbstain"`
}

type groupReport struct {
	Group        string `json:"group"`
	Decisions    int    `json:"decisions"`
	Recall       rate   `json:"recallAt8"`
	RecallAny    rate   `json:"recallAnyShown"`
	Top1         rate   `json:"top1"`
	Top3         rate   `json:"top3"`
	JudgeTop1    rate   `json:"judgeTop1Answered"`
	JudgeAbstain rate   `json:"judgeAbstain"`
}

type breakdownReport struct {
	Dimension string        `json:"dimension"`
	Groups    []groupReport `json:"groups"`
}

type judgeAbstentionReport struct {
	All          rate `json:"all"`
	TruthPresent rate `json:"truthPresent"`
	TruthAbsent  rate `json:"truthAbsent"`
}

type liftReport struct {
	Baseline              rate    `json:"baselineTop1"`
	Judge                 rate    `json:"judgeTop1"`
	Difference            float64 `json:"difference"`
	Lower                 float64 `json:"lower"`
	Upper                 float64 `json:"upper"`
	Resamples             int     `json:"resamples"`
	Measurable            bool    `json:"measurable"`
	ExcludesNoImprovement bool    `json:"excludesNoImprovement"`
}

type judgeWrongReport struct {
	Judge    rate `json:"judge"`
	Baseline rate `json:"baseline"`
	Increase int  `json:"increase"`
}

type judgeComparisonReport struct {
	CandidatePresent int              `json:"candidatePresent"`
	JudgeTop1        rate             `json:"judgeTop1"`
	BaselineTop1     rate             `json:"baselineTop1"`
	JudgeTop3        rate             `json:"judgeTop3"`
	BaselineTop3     rate             `json:"baselineTop3"`
	Lift             liftReport       `json:"lift"`
	WrongTop1        judgeWrongReport `json:"wrongTop1"`
}

type calibrationBucket struct {
	Bucket   string `json:"bucket"`
	Total    int    `json:"total"`
	Correct  int    `json:"correct"`
	Accuracy rate   `json:"accuracy"`
}

type classCount struct {
	Class     string `json:"class"`
	Decisions int    `json:"decisions"`
	WithTruth int    `json:"withTruth"`
}

type shadowGate struct {
	Required         int          `json:"required"`
	JudgedDecisions  int          `json:"judgedDecisions"`
	JudgedWithTruth  int          `json:"judgedWithTruth"`
	ClassMapProvided bool         `json:"classMapProvided"`
	Classes          []classCount `json:"classes"`
	Ready            bool         `json:"ready"`
	Reasons          []string     `json:"reasons,omitempty"`
}

type report struct {
	DistinctSourceGroups   int                    `json:"distinctSourceGroups"`
	PreparationDiagnostics map[string]int         `json:"preparationDiagnostics"`
	DistinctSources        int                    `json:"distinctSources"`
	DuplicateObservations  int                    `json:"duplicateObservations"`
	TruthPolicy            string                 `json:"truthPolicy"`
	Cutoff                 string                 `json:"cutoff,omitempty"`
	SupersededDecisions    int                    `json:"supersededDecisions"`
	CollapsedRevisions     int                    `json:"collapsedRevisions"`
	Cohort                 *cohortVersion         `json:"cohort,omitempty"`
	Cohorts                []report               `json:"cohorts,omitempty"`
	MixedVersions          bool                   `json:"mixedVersions"`
	GeneratedAt            string                 `json:"generatedAt"`
	Since                  string                 `json:"since,omitempty"`
	Normalizer             string                 `json:"normalizerVersion"`
	Decisions              int                    `json:"decisions"`
	WithSnapshot           int                    `json:"withSnapshot"`
	MissingSnapshot        int                    `json:"missingSnapshot"`
	StaleNormalizer        int                    `json:"staleNormalizer"`
	Recall                 rate                   `json:"recallAt8"`
	RecallAny              rate                   `json:"recallAnyShown"`
	NoCandidates           rate                   `json:"noCandidates"`
	AllWrong               rate                   `json:"allWrongCandidates"`
	Top1                   rate                   `json:"top1"`
	Top3                   rate                   `json:"top3"`
	MRR                    float64                `json:"mrr"`
	Typed                  rate                   `json:"typed"`
	MedianMS               *int                   `json:"medianDurationMs,omitempty"`
	JudgeTotal             int                    `json:"judgeTotal"`
	JudgeTop1              rate                   `json:"judgeTop1"`
	JudgeTop3              rate                   `json:"judgeTop3"`
	JudgeAbstain           rate                   `json:"judgeAbstain"`
	JudgeUsed              int                    `json:"judgeUsed"`
	JudgeActiveShown       int                    `json:"judgeActiveShown"`
	JudgeUseRate           rate                   `json:"judgeUseRate"`
	SourceCoverage         []sourceCoverageReport `json:"candidateSourceCoverage"`
	Breakdowns             []breakdownReport      `json:"breakdowns"`
	JudgeAbstention        judgeAbstentionReport  `json:"judgeAbstention"`
	JudgeComparison        judgeComparisonReport  `json:"judgeComparison"`
	Calibration            []calibrationBucket    `json:"judgeCalibration"`
	ShadowGate             shadowGate             `json:"shadowGate"`
	Files                  []fileReport           `json:"files"`
}

type pair struct {
	JudgeCorrect    bool
	BaselineCorrect bool
}

type decisionAnalysis struct {
	Row                   decisionRow
	HasTruth              bool
	Truth                 nameKeys
	ShownAny              []candidateSnapshot
	ShownTop              []candidateSnapshot
	AnyMatch              bool
	Top1Match             bool
	Top3Match             bool
	Rank                  int
	JudgePresent          bool
	JudgeAbstained        bool
	JudgeChoiceMatch      bool
	JudgeTop3Match        bool
	JudgeHasProbabilities bool
	JudgeAnswered         bool
	JudgeWithTruth        bool
}

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	sinceText := flag.String("since", "", "only decisions at or after this RFC3339 timestamp")
	cutoffText := flag.String("cutoff", "", "only decisions at or before this RFC3339 timestamp")
	truthPolicy := flag.String("truth-policy", "first", "evaluation truth per occurrence: first or latest")
	asJSON := flag.Bool("json", false, "write the report as JSON")
	classMapPath := flag.String("class-map", "", "JSON object mapping source file IDs to clean_print, calligraphic_dense, or low_resolution_screenshot")
	selected := cohortVersion{}
	flag.StringVar(&selected.Pool, "pool-version", "", "select a saved candidate pool version, or legacy")
	flag.StringVar(&selected.Reader, "reader-version", "", "select the saved immutable reading model version, or legacy")
	flag.StringVar(&selected.Model, "judge-model", "", "select a saved judge model, or legacy")
	flag.StringVar(&selected.Prompt, "prompt-version", "", "select a saved judge prompt version, or legacy")
	flag.StringVar(&selected.Recipe, "recipe-version", "", "select the complete frozen version contract hash")
	flag.StringVar(&selected.Evidence, "evidence-version", "", "select the initial evidence schema")
	flag.StringVar(&selected.Generation, "generation-version", "", "select the generation recipe")
	flag.StringVar(&selected.Renderer, "renderer-version", "", "select the source renderer")
	flag.StringVar(&selected.Partition, "partition", "", "select development or held_out; default reports partitions separately")
	flag.Parse()
	if selected.Partition != "" && selected.Partition != "development" && selected.Partition != "held_out" {
		fmt.Fprintln(os.Stderr, "name-eval: partition must be development or held_out")
		os.Exit(2)
	}
	if *truthPolicy != "first" && *truthPolicy != "latest" {
		fmt.Fprintln(os.Stderr, "name-eval: --truth-policy must be first or latest")
		os.Exit(2)
	}
	if *truthPolicy == "latest" && strings.TrimSpace(*cutoffText) == "" {
		fmt.Fprintln(os.Stderr, "name-eval: --truth-policy latest requires a pinned --cutoff")
		os.Exit(2)
	}
	if strings.TrimSpace(*cutoffText) != "" {
		if _, err := time.Parse(time.RFC3339, *cutoffText); err != nil {
			fmt.Fprintln(os.Stderr, "name-eval: --cutoff is not RFC3339")
			os.Exit(2)
		}
	}

	if strings.TrimSpace(*databaseURL) == "" {
		fmt.Fprintln(os.Stderr, "name-eval: --database-url or DATABASE_URL is required")
		os.Exit(2)
	}

	classMap := map[string]string{}
	if strings.TrimSpace(*classMapPath) != "" {
		parsed, err := loadClassMap(*classMapPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "name-eval: %v\n", err)
			os.Exit(2)
		}
		classMap = parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "name-eval: %v\n", err)
		os.Exit(2)
	}
	defer pool.Close()

	rows, err := loadDecisions(ctx, pool, *sinceText, *cutoffText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "name-eval: %v\n", err)
		os.Exit(2)
	}
	rows = selectCohort(rows, selected)
	applyClassMap(rows, classMap)
	result := summarizeWithTruth(rows, *sinceText, len(classMap) > 0, *truthPolicy, *cutoffText)
	result.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if *asJSON {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "name-eval: %v\n", err)
			os.Exit(2)
		}
		fmt.Println(string(encoded))
		return
	}
	printReport(result)
}

func loadClassMap(path string) (map[string]string, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read --class-map: %w", err)
	}
	var mapping map[string]string
	if err := json.Unmarshal(encoded, &mapping); err != nil {
		return nil, fmt.Errorf("--class-map is not a JSON object of file id to class: %w", err)
	}
	allowed := make(map[string]struct{}, len(expertSourceClasses))
	for _, class := range expertSourceClasses {
		allowed[class] = struct{}{}
	}
	for fileID, class := range mapping {
		if strings.TrimSpace(fileID) == "" {
			return nil, fmt.Errorf("--class-map contains an empty file id")
		}
		if _, ok := allowed[class]; !ok {
			return nil, fmt.Errorf("--class-map class %q for %s is not one of %s", class, fileID, strings.Join(expertSourceClasses, ", "))
		}
	}
	return mapping, nil
}

func applyClassMap(rows []decisionRow, mapping map[string]string) {
	for index := range rows {
		if class, ok := mapping[rows[index].FileID]; ok {
			rows[index].SourceClass = class
		}
	}
}

func loadDecisions(ctx context.Context, pool *pgxpool.Pool, since, cutoff string) ([]decisionRow, error) {
	// Both decisions and geometry retirement must use the same temporal boundary.
	var cutoffAt any
	if strings.TrimSpace(cutoff) != "" {
		parsed, err := time.Parse(time.RFC3339, cutoff)
		if err != nil {
			return nil, fmt.Errorf("--cutoff is not RFC3339: %w", err)
		}
		cutoffAt = parsed
	}
	query := `
		SELECT d.id::text,
		       sg.source_file_id::text,
		       COALESCE(sf.original_filename_ar, ''),
		       d.decision,
		       COALESCE(d.decided_text_ar, ''),
		       d.candidate_snapshot,
		       d.input_method,
		       d.normalizer_version,
               d.candidate_pool_version,
               COALESCE(NULLIF(sg.reading_model,''),'legacy'),
		       COALESCE(d.duration_ms, -1),
		       d.judge_output,
		       sg.page_number,
		       sg.confidence,
		       d.created_at,
		       COALESCE(sf.mime_type, ''),
		       COALESCE(spe.page_width, 0),
		       COALESCE(spe.page_height, 0),
		       sg.id::text,
		       d.review_revision,
		       sg.superseded_at IS NOT NULL AND ($1::timestamptz IS NULL OR sg.superseded_at <= $1),
 jsonb_build_object('boundaryAt',(SELECT installed_at FROM source_name_review_boundary WHERE version='name-review-v2'),'groupKey',COALESCE(eg.group_key,''),'decisionGap',COALESCE(de.gap_reason,''),'schema',COALESCE(de.schema_version,''),'preparationId',COALESCE(p.id::text,''),'exposureId',COALESCE(p.exposure_id::text,''),
 'suggestionId',COALESCE(p.suggestion_id::text,''),'actualSuggestionId',sg.id::text,'signature',COALESCE(p.signature,''),'snapshot',p.snapshot,'versions',p.versions,
 'context',ex.context,'lexiconRevision',lex.revision,'exposureAt',ex.created_at,'preparationAt',p.created_at,'requestAt',q.created_at,'resultAt',res.created_at,
 'requestPayload',q.payload,'requestSignature',COALESCE(q.signature,''),'requestIdentity',q.identity,'resultSignature',COALESCE(res.signature,''),'result',res.result,
 'latencyMs',COALESCE(res.latency_ms,0),'eligible',COALESCE(p.eligible,false),'partition',COALESCE(eg.partition,''),'registrationAt',em.created_at,'registrationContract',eg.contract)
		FROM source_tree_suggestion_decisions d
		JOIN source_tree_suggestions sg ON sg.id = d.suggestion_id
		JOIN source_files sf ON sf.id = sg.source_file_id
		LEFT JOIN source_page_evidence spe ON spe.id = sg.source_page_evidence_id
 LEFT JOIN source_name_decision_evidence de ON de.decision_id=d.id
 LEFT JOIN source_name_candidate_preparations p ON p.id=de.preparation_id
 LEFT JOIN source_name_review_exposures ex ON ex.id=p.exposure_id
 LEFT JOIN source_name_lexicon_revisions lex ON lex.id=ex.lexicon_id
 LEFT JOIN source_name_judge_requests q ON q.preparation_id=p.id
 LEFT JOIN source_name_judge_results res ON res.request_id=q.id
 LEFT JOIN source_name_evaluation_members em ON em.source_file_id=sg.source_file_id
 LEFT JOIN source_name_evaluation_groups eg ON eg.id=em.group_id
	`
	args := []any{cutoffAt}
	conditions := []string{}
	if strings.TrimSpace(since) != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return nil, fmt.Errorf("--since is not RFC3339: %w", err)
		}
		args = append(args, parsed)
		conditions = append(conditions, fmt.Sprintf("d.created_at >= $%d", len(args)))
	}
	if cutoffAt != nil {
		conditions = append(conditions, "d.created_at <= $1")
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY d.created_at, d.id`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]decisionRow, 0, 256)
	for rows.Next() {
		var row decisionRow
		var snapshotJSON, judgeJSON, initialJSON []byte
		if err := rows.Scan(
			&row.ID, &row.FileID, &row.FileName, &row.Decision, &row.DecidedText,
			&snapshotJSON, &row.InputMethod, &row.NormalizerVersion, &row.PoolVersion, &row.ReaderVersion, &row.DurationMS, &judgeJSON,
			&row.PageNumber, &row.ReaderConfidence, &row.DecisionAt, &row.MimeType,
			&row.PageWidth, &row.PageHeight, &row.SuggestionID, &row.ReviewRevision, &row.Superseded, &initialJSON,
		); err != nil {
			return nil, err
		}
		if len(snapshotJSON) > 0 {
			if err := json.Unmarshal(snapshotJSON, &row.Candidates); err != nil {
				return nil, fmt.Errorf("invalid legacy candidate snapshot")
			}
		}
		if len(judgeJSON) > 0 {
			_ = json.Unmarshal(judgeJSON, &row.Judge)
		}
		var initial initialEvidence
		if json.Unmarshal(initialJSON, &initial) != nil {
			return nil, fmt.Errorf("invalid initial evidence metadata")
		}
		applyInitialEvidence(&row, initial)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func summarize(rows []decisionRow, since string) report {
	return summarizeWithClassMap(rows, since, false)
}

func summarizeWithClassMap(rows []decisionRow, since string, classMapProvided bool) report {
	return summarizeWithTruth(rows, since, classMapProvided, "first", "")
}

// summarizeWithTruth makes the evaluation truth policy explicit. The original
// cohort keeps first-decision semantics; a new evaluation may select the latest
// human revision as of a pinned cutoff. Neither policy counts retries or later
// revisions as independent physical occurrences.
func summarizeWithTruth(rows []decisionRow, since string, classMapProvided bool, truthPolicy, cutoff string) report {
	uniqueRows, duplicateCount, supersededCount, collapsedCount := uniqueObservations(rows, truthPolicy)
	rows = uniqueRows
	result := summarizeCohort(rows, since, classMapProvided)
	result.TruthPolicy = truthPolicy
	result.Cutoff = cutoff
	result.DuplicateObservations = duplicateCount
	result.SupersededDecisions = supersededCount
	result.CollapsedRevisions = collapsedCount
	groups := make(map[cohortVersion][]decisionRow)
	for _, row := range rows {
		version := rowVersion(row)
		groups[version] = append(groups[version], row)
	}
	versions := make([]cohortVersion, 0, len(groups))
	for version := range groups {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		a, _ := json.Marshal(versions[i])
		b, _ := json.Marshal(versions[j])
		return string(a) < string(b)
	})
	for _, version := range versions {
		cohort := summarizeCohort(groups[version], since, classMapProvided)
		v := version
		cohort.Cohort = &v
		result.Cohorts = append(result.Cohorts, cohort)
	}
	result.MixedVersions = len(groups) > 1
	if result.MixedVersions {
		result.ShadowGate.Ready = false
		result.ShadowGate.Reasons = append(result.ShadowGate.Reasons, "mixed pipeline versions; select one coherent cohort with the version flags")
	}
	if len(versions) == 1 {
		result.Cohort = &versions[0]
	}
	return result
}

func summarizeCohort(rows []decisionRow, since string, classMapProvided bool) report {
	result := report{Normalizer: normalizerVersion, Since: since, PreparationDiagnostics: map[string]int{}}
	analyses := make([]decisionAnalysis, 0, len(rows))
	byFile := make(map[string]*fileReport)
	fileAnalyses := make(map[string][]decisionAnalysis)
	var allDurations []int

	for _, row := range rows {
		result.Decisions++
		file := byFile[row.FileID]
		if file == nil {
			file = &fileReport{FileID: row.FileID, FileName: row.FileName}
			byFile[row.FileID] = file
		}
		file.Decisions++
		if row.EvidenceVersion != "" {
			seenDiagnostic := map[string]bool{}
			for _, key := range row.ReadingDiagnostics {
				switch key {
				case "empty_machine_reading", "unsupported_reading", "local_reading_unavailable", "geometry_unavailable":
					if !seenDiagnostic[key] {
						result.PreparationDiagnostics[key]++
						seenDiagnostic[key] = true
					}
				}
			}

			result.PreparationDiagnostics["newEvidence"]++
			if row.InitialGap != "" {
				result.PreparationDiagnostics[row.InitialGap]++
			}
			if row.JudgeGap != "" {
				result.PreparationDiagnostics[row.JudgeGap]++
			}
			if !row.BlindEligible {
				result.PreparationDiagnostics["operational_or_development"]++
			}
			if row.ContextTruncated {
				result.PreparationDiagnostics["truncated_file_context"]++
			}
			if row.InitialValid && len(row.Candidates) == 0 {
				result.PreparationDiagnostics["no_candidates"]++
			}
		}
		if len(row.Candidates) == 0 && !row.InitialValid {
			result.MissingSnapshot++
			continue
		}
		if row.NormalizerVersion != normalizerVersion {
			// The snapshot's keys were produced by a different normalizer. The
			// truth side cannot be recomputed with today's keys without changing
			// the question, so this decision is a measurement gap rather than a
			// silently reinterpreted result.
			result.StaleNormalizer++
			continue
		}
		result.WithSnapshot++
		file.WithSnapshot++
		if row.DurationMS >= 0 {
			allDurations = append(allDurations, row.DurationMS)
		}
		analysis := analyzeDecision(row)
		analyses = append(analyses, analysis)
		fileAnalyses[row.FileID] = append(fileAnalyses[row.FileID], analysis)
	}

	result.DistinctSources = len(byFile)
	sourceGroups := map[string]bool{}
	for _, row := range rows {
		if row.SourceGroup != "" {
			sourceGroups[row.SourceGroup] = true
		}
	}
	result.DistinctSourceGroups = len(sourceGroups)
	result.Recall, result.RecallAny, result.NoCandidates, result.AllWrong, result.Top1, result.Top3, result.Typed, result.MRR = overallMetrics(analyses)
	result.MedianMS = median(allDurations)
	result.SourceCoverage = sourceCoverage(analyses)
	result.Breakdowns = buildBreakdowns(analyses)

	var judgeTotal, judgeAbstained, judgeAnswered, judgeTop1, judgeTop3, judgeTop3Total int
	var judgeUsed, judgeActiveShown int
	var truthPresent, truthPresentAbstained, truthAbsent, truthAbsentAbstained int
	var candidatePresent, candidateJudgeTop1, candidateBaselineTop1, candidateJudgeTop3, candidateBaselineTop3 int
	var answeredCandidatePresent, answeredJudgeWrong, answeredBaselineWrong int
	pairs := make([]pair, 0)
	calibration := make(map[string]struct {
		total   int
		correct int
	})
	for _, analysis := range analyses {
		if !analysis.JudgePresent {
			continue
		}
		judgeTotal++
		if analysis.Row.Judge.Mode == "active" && analysis.Row.Judge.Shown {
			judgeActiveShown++
			if analysis.Row.Judge.Used {
				judgeUsed++
			}
		}
		if analysis.JudgeAbstained {
			judgeAbstained++
		}
		if !analysis.HasTruth {
			continue
		}
		if analysis.JudgeAbstained {
			if analysis.AnyMatch {
				truthPresentAbstained++
			} else {
				truthAbsentAbstained++
			}
		}
		if analysis.AnyMatch {
			truthPresent++
		} else {
			truthAbsent++
		}
		if analysis.JudgeAnswered {
			judgeAnswered++
			if analysis.JudgeChoiceMatch {
				judgeTop1++
			}
			if analysis.JudgeHasProbabilities {
				judgeTop3Total++
				if analysis.JudgeTop3Match {
					judgeTop3++
				}
			}
			if analysis.Row.Judge.Choice != "" || analysis.Row.Judge.Probabilities != nil {
				if analysis.Row.Judge.Confidence != nil {
					bucket := confidenceBucket(*analysis.Row.Judge.Confidence)
					entry := calibration[bucket]
					entry.total++
					if analysis.JudgeChoiceMatch {
						entry.correct++
					}
					calibration[bucket] = entry
				}
			}
		}
		if analysis.AnyMatch {
			candidatePresent++
			if analysis.JudgeAnswered && analysis.JudgeChoiceMatch {
				candidateJudgeTop1++
			}
			if analysis.Top1Match {
				candidateBaselineTop1++
			}
			if analysis.JudgeAnswered && analysis.JudgeTop3Match {
				candidateJudgeTop3++
			}
			if analysis.Top3Match {
				candidateBaselineTop3++
			}
			pairs = append(pairs, pair{
				JudgeCorrect:    analysis.JudgeAnswered && analysis.JudgeChoiceMatch,
				BaselineCorrect: analysis.Top1Match,
			})
			if analysis.JudgeAnswered {
				answeredCandidatePresent++
				if !analysis.JudgeChoiceMatch {
					answeredJudgeWrong++
				}
				if !analysis.Top1Match {
					answeredBaselineWrong++
				}
			}
		}
	}
	result.JudgeTotal = judgeTotal
	result.JudgeTop1 = makeRate(judgeTop1, judgeAnswered)
	result.JudgeTop3 = makeRate(judgeTop3, judgeTop3Total)
	result.JudgeAbstain = makeRate(judgeAbstained, judgeTotal)
	result.JudgeUsed = judgeUsed
	result.JudgeActiveShown = judgeActiveShown
	result.JudgeUseRate = makeRate(judgeUsed, judgeActiveShown)
	result.JudgeAbstention = judgeAbstentionReport{
		All:          makeRate(judgeAbstained, judgeTotal),
		TruthPresent: makeRate(truthPresentAbstained, truthPresent),
		TruthAbsent:  makeRate(truthAbsentAbstained, truthAbsent),
	}
	lower, upper, difference := pairedBootstrap(pairs, minBootstrapSamples)
	result.JudgeComparison = judgeComparisonReport{
		CandidatePresent: candidatePresent,
		JudgeTop1:        makeRate(candidateJudgeTop1, candidatePresent),
		BaselineTop1:     makeRate(candidateBaselineTop1, candidatePresent),
		JudgeTop3:        makeRate(candidateJudgeTop3, candidatePresent),
		BaselineTop3:     makeRate(candidateBaselineTop3, candidatePresent),
		Lift: liftReport{
			Baseline:              makeRate(candidateBaselineTop1, candidatePresent),
			Judge:                 makeRate(candidateJudgeTop1, candidatePresent),
			Difference:            difference,
			Lower:                 lower,
			Upper:                 upper,
			Resamples:             minBootstrapSamples,
			Measurable:            candidatePresent >= minMeasurableRate,
			ExcludesNoImprovement: lower > 0,
		},
		WrongTop1: judgeWrongReport{
			Judge:    makeRate(answeredJudgeWrong, answeredCandidatePresent),
			Baseline: makeRate(answeredBaselineWrong, answeredCandidatePresent),
			Increase: answeredJudgeWrong - answeredBaselineWrong,
		},
	}
	result.Calibration = calibrationBuckets(calibration)
	result.ShadowGate = shadowGateReport(analyses, classMapProvided)
	for _, row := range rows {
		if row.Partition == "held_out" && row.InitialGap != "" {
			result.ShadowGate.Ready = false
			result.ShadowGate.Reasons = append(result.ShadowGate.Reasons, "unresolved initial preparation gaps in held-out cohort")
			break
		}
	}
	if result.Recall.Measurable && result.Recall.Rate < minCandidateRecall {
		result.ShadowGate.Ready = false
		result.ShadowGate.Reasons = append(result.ShadowGate.Reasons,
			fmt.Sprintf("candidate recall@8 is %.1f%%; the judge cannot choose a name the candidate pool missed (floor %.0f%%)",
				result.Recall.Rate*100, minCandidateRecall*100))
	}

	fileIDs := make([]string, 0, len(byFile))
	for id := range byFile {
		fileIDs = append(fileIDs, id)
	}
	sort.Strings(fileIDs)
	for _, id := range fileIDs {
		file := byFile[id]
		fileResult := fileMetrics(fileAnalyses[id])
		file.Recall = fileResult.recall
		file.RecallAny = fileResult.recallAny
		file.Top1 = fileResult.top1
		file.Top3 = fileResult.top3
		file.Typed = fileResult.typed
		file.MedianMS = fileResult.median
		result.Files = append(result.Files, *file)
	}
	return result
}

func overallMetrics(analyses []decisionAnalysis) (rate, rate, rate, rate, rate, rate, rate, float64) {
	var truthTotal, shownAnyTotal, ranked int
	var recallMatches, recallAnyMatches, noCandidateMatches, allWrongMatches int
	var top1, top3, typed int
	var reciprocalRank float64
	for _, analysis := range analyses {
		if !analysis.HasTruth {
			continue
		}
		truthTotal++
		if len(analysis.ShownAny) == 0 {
			noCandidateMatches++
			continue
		}
		shownAnyTotal++
		if analysis.AnyMatch {
			recallAnyMatches++
		} else {
			allWrongMatches++
		}
		if len(analysis.ShownTop) == 0 {
			continue
		}
		ranked++
		if analysis.Rank > 0 {
			recallMatches++
			reciprocalRank += 1 / float64(analysis.Rank)
		}
		if analysis.Top1Match {
			top1++
		}
		if analysis.Top3Match {
			top3++
		}
	}
	for _, analysis := range analyses {
		if analysis.HasTruth && analysis.Row.InputMethod == "typed" {
			typed++
		}
	}
	mrr := 0.0
	if ranked > 0 {
		mrr = reciprocalRank / float64(ranked)
	}
	return makeRate(recallMatches, truthTotal), makeRate(recallAnyMatches, truthTotal),
		makeRate(noCandidateMatches, truthTotal), makeRate(allWrongMatches, shownAnyTotal),
		makeRate(top1, ranked), makeRate(top3, ranked), makeRate(typed, truthTotal), mrr
}

type fileMetricsResult struct {
	recall    rate
	recallAny rate
	top1      rate
	top3      rate
	typed     rate
	median    *int
}

func fileMetrics(analyses []decisionAnalysis) fileMetricsResult {
	var truthTotal, ranked, recallMatches, recallAnyMatches, top1, top3, typed int
	var durations []int
	for _, analysis := range analyses {
		if analysis.Row.DurationMS >= 0 {
			durations = append(durations, analysis.Row.DurationMS)
		}
		if !analysis.HasTruth {
			continue
		}
		truthTotal++
		if analysis.Row.InputMethod == "typed" {
			typed++
		}
		if analysis.AnyMatch {
			recallAnyMatches++
		}
		if len(analysis.ShownTop) == 0 {
			continue
		}
		ranked++
		if analysis.Rank > 0 {
			recallMatches++
		}
		if analysis.Top1Match {
			top1++
		}
		if analysis.Top3Match {
			top3++
		}
	}
	return fileMetricsResult{
		recall:    makeRate(recallMatches, truthTotal),
		recallAny: makeRate(recallAnyMatches, truthTotal),
		top1:      makeRate(top1, ranked),
		top3:      makeRate(top3, ranked),
		typed:     makeRate(typed, truthTotal),
		median:    median(durations),
	}
}

func analyzeDecision(row decisionRow) decisionAnalysis {
	analysis := decisionAnalysis{Row: row}
	analysis.JudgePresent = row.Judge.present()
	analysis.JudgeAbstained = row.Judge.abstained()
	analysis.JudgeAnswered = analysis.JudgePresent && !analysis.JudgeAbstained && row.Judge.Choice != ""
	analysis.ShownAny = shownCandidates(row.Candidates, 0)
	analysis.ShownTop = shownCandidates(row.Candidates, 8)
	if row.DecidedText == "" {
		return analysis
	}
	analysis.HasTruth = true
	analysis.Truth = keysFor(row.DecidedText)
	analysis.AnyMatch = containsCandidate(analysis.ShownAny, analysis.Truth)
	analysis.Rank = candidateRank(analysis.ShownTop, analysis.Truth)
	analysis.Top1Match = analysis.Rank == 1
	analysis.Top3Match = analysis.Rank > 0 && analysis.Rank <= 3
	analysis.JudgeWithTruth = analysis.JudgePresent
	if analysis.JudgeAnswered {
		analysis.JudgeChoiceMatch = candidateMatches(
			candidateSnapshot{Value: row.Judge.Choice, Key: canonicalName(row.Judge.Choice)},
			analysis.Truth,
		)
		analysis.JudgeHasProbabilities = len(row.Judge.Probabilities) > 0
		if analysis.JudgeHasProbabilities {
			analysis.JudgeTop3Match = judgeTop3Contains(row.Judge.Probabilities, analysis.Truth)
		}
	}
	return analysis
}

func (j judgeOutput) present() bool {
	return j.Mode != "" || j.Provider != "" || j.Model != "" || j.PromptVersion != "" ||
		j.Choice != "" || j.Abstained || j.Confidence != nil || len(j.Probabilities) > 0
}

func (j judgeOutput) abstained() bool {
	return j.Abstained || j.Choice == "unclear"
}

func candidateRank(candidates []candidateSnapshot, truth nameKeys) int {
	for index, candidate := range candidates {
		if candidateMatches(candidate, truth) {
			return index + 1
		}
	}
	return 0
}

func judgeTop3Contains(probabilities map[string]float64, truth nameKeys) bool {
	type entry struct {
		value string
		score float64
	}
	entries := make([]entry, 0, len(probabilities))
	for value, score := range probabilities {
		if value == "unclear" || score <= 0 {
			continue
		}
		entries = append(entries, entry{value: value, score: score})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].score != entries[j].score {
			return entries[i].score > entries[j].score
		}
		return entries[i].value < entries[j].value
	})
	if len(entries) > 3 {
		entries = entries[:3]
	}
	for _, item := range entries {
		if candidateMatches(candidateSnapshot{Value: item.value, Key: canonicalName(item.value)}, truth) {
			return true
		}
	}
	return false
}

func sourceCoverage(analyses []decisionAnalysis) []sourceCoverageReport {
	sources := []string{"reader", "ai", "file", "common"}
	out := make([]sourceCoverageReport, 0, len(sources))
	var truthTotal int
	type sourceAccumulator struct {
		offered, contains, judged, abstained, judgeMatches, baselineMatches int
	}
	values := make(map[string]*sourceAccumulator, len(sources))
	for _, source := range sources {
		values[source] = &sourceAccumulator{}
	}
	for _, analysis := range analyses {
		if !analysis.HasTruth {
			continue
		}
		truthTotal++
		for _, source := range sources {
			value := values[source]
			if candidateWithSource(analysis.ShownAny, source) {
				value.offered++
			}
			if !candidateWithSourceMatches(analysis.ShownAny, source, analysis.Truth) {
				continue
			}
			value.contains++
			if analysis.Top1Match {
				value.baselineMatches++
			}
			if analysis.JudgePresent {
				value.judged++
				if analysis.JudgeAbstained {
					value.abstained++
				} else if analysis.JudgeChoiceMatch {
					value.judgeMatches++
				}
			}
		}
	}
	for _, source := range sources {
		value := values[source]
		out = append(out, sourceCoverageReport{
			Source:        source,
			Offered:       makeRate(value.offered, truthTotal),
			ContainsTruth: makeRate(value.contains, truthTotal),
			OfferedRecall: makeRate(value.contains, value.offered),
			// End-to-end over decisions where this source supplied the truth:
			// abstention counts as not correct, so a source-specific subset
			// cannot look better by refusing to answer.
			JudgeTop1:    makeRate(value.judgeMatches, value.contains),
			BaselineTop1: makeRate(value.baselineMatches, value.contains),
			JudgeAbstain: makeRate(value.abstained, value.judged),
		})
	}
	return out
}

func candidateWithSource(candidates []candidateSnapshot, source string) bool {
	for _, candidate := range candidates {
		for _, candidateSource := range candidateSources(candidate) {
			if candidateSource == source {
				return true
			}
		}
	}
	return false
}

func candidateWithSourceMatches(candidates []candidateSnapshot, source string, truth nameKeys) bool {
	for _, candidate := range candidates {
		for _, candidateSource := range candidateSources(candidate) {
			if candidateSource == source && candidateMatches(candidate, truth) {
				return true
			}
		}
	}
	return false
}

func candidateSources(candidate candidateSnapshot) []string {
	if len(candidate.Sources) > 0 {
		return candidate.Sources
	}
	if candidate.Source != "" {
		return []string{candidate.Source}
	}
	return nil
}

func buildBreakdowns(analyses []decisionAnalysis) []breakdownReport {
	dimensions := []struct {
		name string
		key  func(decisionAnalysis) string
	}{
		{"fileClass", func(analysis decisionAnalysis) string { return fileClass(analysis.Row.MimeType) }},
		{"resolution", func(analysis decisionAnalysis) string {
			return resolutionClass(analysis.Row.PageWidth, analysis.Row.PageHeight)
		}},
		{"page", func(analysis decisionAnalysis) string { return fmt.Sprintf("page %d", analysis.Row.PageNumber) }},
		{"readerConfidence", func(analysis decisionAnalysis) string {
			return readerConfidenceClass(analysis.Row.ReaderConfidence)
		}},
		{"date", func(analysis decisionAnalysis) string { return analysis.Row.DecisionAt.UTC().Format("2006-01-02") }},
	}
	out := make([]breakdownReport, 0, len(dimensions))
	for _, dimension := range dimensions {
		type accumulator struct {
			decisions                             int
			recallMatches, recallAnyMatches       int
			ranked, top1, top3                    int
			judged, judgeAbstained, judgeAnswered int
			judgeTop1                             int
		}
		groups := make(map[string]*accumulator)
		for _, analysis := range analyses {
			if !analysis.HasTruth {
				continue
			}
			key := dimension.key(analysis)
			group := groups[key]
			if group == nil {
				group = &accumulator{}
				groups[key] = group
			}
			group.decisions++
			if analysis.AnyMatch {
				group.recallAnyMatches++
			}
			if analysis.Rank > 0 {
				group.recallMatches++
			}
			if len(analysis.ShownTop) > 0 {
				group.ranked++
				if analysis.Top1Match {
					group.top1++
				}
				if analysis.Top3Match {
					group.top3++
				}
			}
			if analysis.JudgePresent {
				group.judged++
				if analysis.JudgeAbstained {
					group.judgeAbstained++
				}
				if analysis.JudgeAnswered {
					group.judgeAnswered++
					if analysis.JudgeChoiceMatch {
						group.judgeTop1++
					}
				}
			}
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		report := breakdownReport{Dimension: dimension.name}
		for _, key := range keys {
			group := groups[key]
			report.Groups = append(report.Groups, groupReport{
				Group:        key,
				Decisions:    group.decisions,
				Recall:       makeRate(group.recallMatches, group.decisions),
				RecallAny:    makeRate(group.recallAnyMatches, group.decisions),
				Top1:         makeRate(group.top1, group.ranked),
				Top3:         makeRate(group.top3, group.ranked),
				JudgeTop1:    makeRate(group.judgeTop1, group.judgeAnswered),
				JudgeAbstain: makeRate(group.judgeAbstained, group.judged),
			})
		}
		out = append(out, report)
	}
	return out
}

func fileClass(mimeType string) string {
	switch {
	case mimeType == "application/pdf":
		return "pdf"
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.TrimSpace(mimeType) == "":
		return "unknown"
	default:
		return "other"
	}
}

func resolutionClass(width, height float64) string {
	side := math.Max(width, height)
	switch {
	case side <= 0:
		return "unknown"
	case side < 800:
		return "small"
	case side < 1600:
		return "medium"
	default:
		return "large"
	}
}

func readerConfidenceClass(confidence float64) string {
	switch {
	case confidence < 0.45:
		return "low"
	case confidence < 0.8:
		return "medium"
	default:
		return "high"
	}
}

func confidenceBucket(confidence float64) string {
	switch {
	case confidence < 0.2:
		return "0.0-0.2"
	case confidence < 0.4:
		return "0.2-0.4"
	case confidence < 0.6:
		return "0.4-0.6"
	case confidence < 0.8:
		return "0.6-0.8"
	default:
		return "0.8-1.0"
	}
}

func calibrationBuckets(values map[string]struct {
	total   int
	correct int
}) []calibrationBucket {
	order := []string{"0.0-0.2", "0.2-0.4", "0.4-0.6", "0.6-0.8", "0.8-1.0"}
	out := make([]calibrationBucket, 0, len(order))
	for _, bucket := range order {
		value := values[bucket]
		if value.total == 0 {
			continue
		}
		out = append(out, calibrationBucket{
			Bucket:   bucket,
			Total:    value.total,
			Correct:  value.correct,
			Accuracy: makeRate(value.correct, value.total),
		})
	}
	return out
}

func shadowGateReport(analyses []decisionAnalysis, classMapProvided bool) shadowGate {
	gate := shadowGate{
		Required:         minShadowDecisions,
		ClassMapProvided: classMapProvided,
		Classes:          []classCount{},
	}
	counts := make(map[string]*classCount)
	for _, analysis := range analyses {
		if analysis.Row.Partition == "development" {
			continue
		}
		if analysis.Row.EvidenceVersion != "" && (!analysis.Row.InitialValid || !analysis.Row.BlindEligible || analysis.Row.Partition != "held_out") {
			continue
		}

		// An active or displayed recommendation cannot contribute to the blind
		// shadow sample, even when it has a valid judge output and human answer.
		if !analysis.JudgePresent || analysis.Row.Judge.Mode != "shadow" || analysis.Row.Judge.Shown || analysis.Row.Judge.Used {
			continue
		}
		if classMapProvided && !slices.Contains(expertSourceClasses, analysis.Row.SourceClass) {
			continue
		}
		gate.JudgedDecisions++
		if !analysis.HasTruth {
			continue
		}
		gate.JudgedWithTruth++
		class := analysis.Row.SourceClass
		if class == "" {
			class = fileClass(analysis.Row.MimeType) + "/" + resolutionClass(analysis.Row.PageWidth, analysis.Row.PageHeight)
		}
		entry := counts[class]
		if entry == nil {
			entry = &classCount{Class: class}
			counts[class] = entry
		}
		entry.Decisions++
		entry.WithTruth++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		gate.Classes = append(gate.Classes, *counts[key])
	}
	if !classMapProvided {
		gate.Reasons = append(gate.Reasons, "provide --class-map with clean_print, calligraphic_dense, and low_resolution_screenshot classifications")
	}
	if gate.JudgedWithTruth < minShadowDecisions {
		gate.Reasons = append(gate.Reasons, fmt.Sprintf("only %d of %d shadow decisions have a truth label", gate.JudgedWithTruth, minShadowDecisions))
	}
	missing := make([]string, 0, len(expertSourceClasses))
	for _, class := range expertSourceClasses {
		if counts[class] == nil || counts[class].WithTruth == 0 {
			missing = append(missing, class)
		}
	}
	if len(missing) > 0 {
		gate.Reasons = append(gate.Reasons, "missing pre-registered source classes: "+strings.Join(missing, ", "))
	}
	gate.Ready = classMapProvided && gate.JudgedWithTruth >= minShadowDecisions && len(missing) == 0
	return gate
}

func pairedBootstrap(pairs []pair, resamples int) (float64, float64, float64) {
	if len(pairs) == 0 || resamples < 1 {
		return 0, 0, 0
	}
	observedJudge := 0
	observedBaseline := 0
	for _, item := range pairs {
		if item.JudgeCorrect {
			observedJudge++
		}
		if item.BaselineCorrect {
			observedBaseline++
		}
	}
	difference := float64(observedJudge-observedBaseline) / float64(len(pairs))
	rng := uint64(0x9e3779b97f4a7c15)
	differences := make([]float64, 0, resamples)
	for sample := 0; sample < resamples; sample++ {
		judgeCorrect := 0
		baselineCorrect := 0
		for range pairs {
			rng = rng*6364136223846793005 + 1442695040888963407
			index := int(rng % uint64(len(pairs)))
			if pairs[index].JudgeCorrect {
				judgeCorrect++
			}
			if pairs[index].BaselineCorrect {
				baselineCorrect++
			}
		}
		differences = append(differences, float64(judgeCorrect-baselineCorrect)/float64(len(pairs)))
	}
	sort.Float64s(differences)
	return percentile(differences, 0.025), percentile(differences, 0.975), difference
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Round(p * float64(len(sorted)-1)))
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func shownCandidates(candidates []candidateSnapshot, limit int) []candidateSnapshot {
	out := make([]candidateSnapshot, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Shown {
			continue
		}
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func containsCandidate(candidates []candidateSnapshot, truth nameKeys) bool {
	for _, candidate := range candidates {
		if candidateMatches(candidate, truth) {
			return true
		}
	}
	return false
}

func makeRate(matches, total int) rate {
	if total == 0 {
		return rate{}
	}
	value := float64(matches) / float64(total)
	lower, upper := wilson(matches, total)
	return rate{
		Matches:    matches,
		Total:      total,
		Rate:       value,
		Lower:      lower,
		Upper:      upper,
		Measurable: total >= minMeasurableRate,
	}
}

// Wilson is a bounded interval for a binomial proportion. It is used to keep a
// small sample from looking like a precise rate.
func wilson(matches, total int) (float64, float64) {
	if total == 0 {
		return 0, 0
	}
	const z = 1.96
	n := float64(total)
	p := float64(matches) / n
	denominator := 1 + z*z/n
	centre := (p + z*z/(2*n)) / denominator
	margin := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / denominator
	return math.Max(0, centre-margin), math.Min(1, centre+margin)
}

func median(values []int) *int {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	middle := len(sorted) / 2
	value := sorted[middle]
	if len(sorted)%2 == 0 {
		value = (sorted[middle-1] + sorted[middle]) / 2
	}
	return &value
}

// keysFor mirrors the published normalizer for the reviewer's text. The
// candidate side is read from the snapshotted keys; this is only the truth
// side, which the ledger did not know when the decision was written.
type nameKeys struct {
	Canonical string
	Compact   string
	Particle  string
}

func keysFor(value string) nameKeys {
	canonical := canonicalName(value)
	return nameKeys{
		Canonical: canonical,
		Compact:   compactName(canonical),
		Particle:  particleName(canonical),
	}
}

func canonicalName(value string) string {
	value = norm.NFKC.String(value)
	var builder strings.Builder
	for _, character := range value {
		switch {
		case character == 0x200B || character == 0x200C || character == 0x200D ||
			character == 0x200E || character == 0x200F || character == 0x202A ||
			character == 0x202B || character == 0x202C || character == 0x202D ||
			character == 0x202E || character == 0x2060 || character == 0xFEFF:
			continue
		case character == 0x0640:
			continue
		case character >= 0x0610 && character <= 0x061A:
			continue
		case character >= 0x064B && character <= 0x065F:
			continue
		case character == 0x0670:
			continue
		case character >= 0x06D6 && character <= 0x06ED:
			continue
		case character == 0x0623 || character == 0x0625 || character == 0x0622 || character == 0x0671:
			builder.WriteRune('ا')
		case character == 0x0649:
			builder.WriteRune('ي')
		case character == 0x0629:
			builder.WriteRune('ه')
		case character >= 0x0660 && character <= 0x0669:
			builder.WriteRune('0' + (character - 0x0660))
		case character >= 0x06F0 && character <= 0x06F9:
			builder.WriteRune('0' + (character - 0x06F0))
		case unicode.IsSpace(character):
			builder.WriteRune(' ')
		default:
			builder.WriteRune(character)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func compactName(canonical string) string {
	var builder strings.Builder
	for _, character := range canonical {
		switch {
		case unicode.IsSpace(character):
			continue
		case character == '-' || character == '‐' || character == '‑' || character == '‒' ||
			character == '–' || character == '—':
			continue
		default:
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func particleName(canonical string) string {
	tokens := strings.Fields(canonical)
	particles := map[string]bool{"ال": true, "بن": true, "بنت": true, "ابو": true}
	output := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if particles[token] && len(output) < 2 {
			continue
		}
		output = append(output, token)
	}
	if len(output) == 0 {
		return canonical
	}
	if len([]rune(output[0])) > 2 && strings.HasPrefix(output[0], "ال") {
		output[0] = strings.TrimPrefix(output[0], "ال")
	}
	return strings.Join(output, " ")
}

func candidateMatches(candidate candidateSnapshot, truth nameKeys) bool {
	candidateKeys := nameKeys{
		Canonical: candidate.Key,
		Compact:   candidate.CompactKey,
		Particle:  candidate.ParticleKey,
	}
	if candidateKeys.Canonical == "" {
		candidateKeys = keysFor(candidate.Value)
	}
	if candidateKeys.Compact == "" {
		candidateKeys.Compact = compactName(candidateKeys.Canonical)
	}
	if candidateKeys.Particle == "" {
		candidateKeys.Particle = particleName(candidateKeys.Canonical)
	}
	return (truth.Canonical != "" && truth.Canonical == candidateKeys.Canonical) ||
		(truth.Compact != "" && truth.Compact == candidateKeys.Compact) ||
		(truth.Particle != "" && truth.Particle == candidateKeys.Particle)
}

func printReport(result report) {
	fmt.Printf("truth policy: %s", result.TruthPolicy)
	if result.Cutoff != "" {
		fmt.Printf(" (cutoff %s)", result.Cutoff)
	}
	fmt.Printf("; superseded decisions excluded: %d; revision collapses: %d\n", result.SupersededDecisions, result.CollapsedRevisions)
	fmt.Printf("distinct sources: %d; duplicate revisions/retries excluded: %d\n", result.DistinctSources, result.DuplicateObservations)
	diagnosticKeys := []string{}
	for key := range result.PreparationDiagnostics {
		diagnosticKeys = append(diagnosticKeys, key)
	}
	sort.Strings(diagnosticKeys)
	for _, key := range diagnosticKeys {
		fmt.Printf("initial evidence %s: %d\n", key, result.PreparationDiagnostics[key])
	}
	for _, cohort := range result.Cohorts {
		v := cohort.Cohort
		fmt.Printf("cohort pool=%s reader=%s judge=%s prompt=%s: decisions=%d gate-ready=%t\n", v.Pool, v.Reader, v.Model, v.Prompt, cohort.Decisions, cohort.ShadowGate.Ready)
	}

	measurable := "not measurable"
	if result.Recall.Measurable {
		measurable = "measurable"
	}
	fmt.Printf("name-candidate quality (%s, %s)\n", result.Normalizer, measurable)
	fmt.Printf("  decisions: %d, with snapshot: %d, missing snapshot: %d, stale normalizer: %d\n",
		result.Decisions, result.WithSnapshot, result.MissingSnapshot, result.StaleNormalizer)
	fmt.Printf("  candidate recall@8: %s\n", formatRate(result.Recall))
	fmt.Printf("  candidate recall, any shown: %s\n", formatRate(result.RecallAny))
	fmt.Printf("  no candidate offered: %s\n", formatRate(result.NoCandidates))
	fmt.Printf("  every shown candidate wrong: %s\n", formatRate(result.AllWrong))
	fmt.Printf("  top-1: %s\n", formatRate(result.Top1))
	fmt.Printf("  top-3: %s\n", formatRate(result.Top3))
	fmt.Printf("  MRR: %.3f\n", result.MRR)
	fmt.Printf("  typed: %s\n", formatRate(result.Typed))
	if result.MedianMS != nil {
		fmt.Printf("  median time to decision: %.1fs\n", float64(*result.MedianMS)/1000)
	}
	if result.JudgeTotal > 0 {
		fmt.Printf("  judge decisions: %d\n", result.JudgeTotal)
		fmt.Printf("  judge answered top-1: %s; abstained: %s\n",
			formatRate(result.JudgeTop1), formatRate(result.JudgeAbstain))
		if result.JudgeActiveShown > 0 {
			fmt.Printf("  active judge row used: %s\n", formatRate(result.JudgeUseRate))
		}
		fmt.Printf("  judge abstention, truth present: %s\n", formatRate(result.JudgeAbstention.TruthPresent))
		fmt.Printf("  judge abstention, truth absent: %s\n", formatRate(result.JudgeAbstention.TruthAbsent))
		fmt.Printf("  candidate-present judge top-1: %s vs deterministic %s\n",
			formatRate(result.JudgeComparison.JudgeTop1), formatRate(result.JudgeComparison.BaselineTop1))
		fmt.Printf("  candidate-present top-3: judge %s vs deterministic %s\n",
			formatRate(result.JudgeComparison.JudgeTop3), formatRate(result.JudgeComparison.BaselineTop3))
		fmt.Printf("  wrong answered top-1: judge %s vs deterministic %s (count increase %d)\n",
			formatRate(result.JudgeComparison.WrongTop1.Judge),
			formatRate(result.JudgeComparison.WrongTop1.Baseline),
			result.JudgeComparison.WrongTop1.Increase)
		lift := result.JudgeComparison.Lift
		if lift.Measurable {
			fmt.Printf("  judge top-1 lift: %+.1f points, 95%% bootstrap [%+.1f, %+.1f], excludes no improvement: %t\n",
				lift.Difference*100, lift.Lower*100, lift.Upper*100, lift.ExcludesNoImprovement)
		} else {
			fmt.Printf("  judge top-1 lift: %d candidate-present decisions (below %d; no rate)\n",
				lift.Judge.Total, minMeasurableRate)
		}
		if len(result.Calibration) > 0 {
			fmt.Println("  judge calibration:")
			for _, bucket := range result.Calibration {
				fmt.Printf("    %s: %s\n", bucket.Bucket, formatRate(bucket.Accuracy))
			}
		}
	}
	if len(result.SourceCoverage) > 0 {
		fmt.Println("  candidate source coverage:")
		for _, source := range result.SourceCoverage {
			fmt.Printf("    %s: offered %s; contains truth %s; offered recall %s; judge top-1 %s vs deterministic %s; judge abstain %s\n",
				source.Source, formatRate(source.Offered), formatRate(source.ContainsTruth), formatRate(source.OfferedRecall),
				formatRate(source.JudgeTop1), formatRate(source.BaselineTop1), formatRate(source.JudgeAbstain))
		}
	}
	for _, breakdown := range result.Breakdowns {
		if len(breakdown.Groups) == 0 {
			continue
		}
		fmt.Printf("  by %s:\n", breakdown.Dimension)
		for _, group := range breakdown.Groups {
			fmt.Printf("    %s: decisions=%d recall@8=%s top-1=%s judge-top-1=%s judge-abstain=%s\n",
				group.Group, group.Decisions, formatRate(group.Recall), formatRate(group.Top1),
				formatRate(group.JudgeTop1), formatRate(group.JudgeAbstain))
		}
	}
	if len(result.Files) > 0 {
		fmt.Println("  by file:")
		for _, file := range result.Files {
			fmt.Printf("    %s: decisions=%d snapshot=%d recall@8=%s recallAny=%s top-1=%s typed=%s\n",
				file.FileID, file.Decisions, file.WithSnapshot, formatRate(file.Recall),
				formatRate(file.RecallAny), formatRate(file.Top1), formatRate(file.Typed))
		}
	}
	gate := result.ShadowGate
	fmt.Printf("  shadow gate: %d/%d judged decisions with truth; ready=%t\n",
		gate.JudgedWithTruth, gate.Required, gate.Ready)
	if len(gate.Classes) > 0 {
		fmt.Println("    source classes:")
		for _, class := range gate.Classes {
			fmt.Printf("      %s: %d decisions\n", class.Class, class.WithTruth)
		}
	}
	for _, reason := range gate.Reasons {
		fmt.Printf("    blocker: %s\n", reason)
	}
}

func formatRate(value rate) string {
	if value.Total == 0 {
		return "—"
	}
	if !value.Measurable {
		return fmt.Sprintf("%d/%d (below %d; no rate)", value.Matches, value.Total, minMeasurableRate)
	}
	return fmt.Sprintf("%d/%d = %.1f%% [%.1f, %.1f]",
		value.Matches, value.Total, value.Rate*100, value.Lower*100, value.Upper*100)
}
