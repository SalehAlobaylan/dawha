package sourceprocessing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// This digest is the exact family chart the owner explicitly allowed for
	// hosted vision. Filenames, source titles and client flags do not grant access.
	JasserChartSHA256            = "5735335ee7b21c1c285882ba2411cb77d2615937e6e2188a8c35cbe8705307fd"
	ChartAnalysisPipelineVersion = "arabic-family-chart-vision-v8"
	ChartAnalysisHostedProvider  = "deepseek-chat-completions-inline-user-id"
	chartTileSize                = 512
	chartTileOverlap             = 128
	chartMaxProviderCalls        = 160
	chartMaxNameOccurrences      = 6000
	chartMaxReadBatchImages      = 8
	chartMaxReadBatchBytes       = 14 << 20
)

var ErrChartAnalysisBudget = errors.New("chart analysis reached its request budget")

type ChartAnalysisPayload struct {
	RunID        string `json:"run_id"`
	SourceFileID string `json:"source_file_id"`
}

type ChartTileBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type chartUnitGeometry struct {
	Bounds           ChartTileBounds `json:"bounds,omitempty"`
	PageWidth        int             `json:"pageWidth"`
	PageHeight       int             `json:"pageHeight"`
	PageWidthPoints  float64         `json:"pageWidthPoints,omitempty"`
	PageHeightPoints float64         `json:"pageHeightPoints,omitempty"`
	RenderDPI        int             `json:"renderDpi"`
}

type chartStoredTile struct {
	Bounds            ChartTileBounds  `json:"bounds"`
	Reading           ChartTileReading `json:"reading"`
	Subdivided        bool             `json:"subdivided,omitempty"`
	Incomplete        bool             `json:"incomplete,omitempty"`
	SubdivisionReason string           `json:"subdivisionReason,omitempty"`
}

type ChartBranchRegion struct {
	ID             string       `json:"id"`
	PageNumber     int          `json:"pageNumber"`
	Polygon        []ChartPoint `json:"polygon"`
	Label          string       `json:"label,omitempty"`
	ReviewerEdited bool         `json:"reviewerEdited"`
	ReviewStatus   string       `json:"reviewStatus"`
}

type ChartNameOccurrence struct {
	ID                  string   `json:"id"`
	SourceOccurrenceIDs []string `json:"sourceOccurrenceIds,omitempty"`
	SourceTileIDs       []string `json:"sourceTileIds,omitempty"`
	PageNumber          int      `json:"pageNumber"`
	Region              ChartBox `json:"region"`
	BranchID            string   `json:"branchId,omitempty"`
	Legibility          string   `json:"legibility"`
	ReadingStatus       string   `json:"readingStatus"`
	TextAR              string   `json:"textAr"`
	Alternatives        []string `json:"alternatives"`
	UncertainSpans      []string `json:"uncertainSpans"`
	ReviewStatus        string   `json:"reviewStatus"`
	ReviewerNoteAR      string   `json:"reviewerNoteAr,omitempty"`
}

type ChartConnectorHypothesis struct {
	ID          string       `json:"id"`
	PageNumber  int          `json:"pageNumber"`
	Points      []ChartPoint `json:"points"`
	Kind        string       `json:"kind"`
	Status      string       `json:"status"`
	StartNameID string       `json:"startNameId,omitempty"`
	EndNameID   string       `json:"endNameId,omitempty"`
}

type ChartTileView struct {
	ID                string          `json:"id"`
	Page              int             `json:"pageNumber"`
	Bounds            ChartTileBounds `json:"bounds"`
	Status            string          `json:"status"`
	Incomplete        bool            `json:"incomplete"`
	ErrorCode         string          `json:"errorCode,omitempty"`
	SubdivisionReason string          `json:"subdivisionReason,omitempty"`
}

type ChartQualityReport struct {
	Status                   string   `json:"status"`
	Reasons                  []string `json:"reasons"`
	TileCount                int      `json:"tileCount"`
	ProcessedTileCount       int      `json:"processedTileCount"`
	FailedTileCount          int      `json:"failedTileCount"`
	IncompleteTileCount      int      `json:"incompleteTileCount"`
	FailedUnitCount          int      `json:"failedUnitCount"`
	PendingUnitCount         int      `json:"pendingUnitCount"`
	NameOccurrenceCount      int      `json:"nameOccurrenceCount"`
	UnreadableNameCount      int      `json:"unreadableNameCount"`
	UnreviewedNameCount      int      `json:"unreviewedNameCount"`
	UnreviewedBranchCount    int      `json:"unreviewedBranchCount"`
	UnresolvedConnectorCount int      `json:"unresolvedConnectorCount"`
	ProviderCallCount        int      `json:"providerCallCount"`
	InputImageBytes          int64    `json:"inputImageBytes"`
	PrecisionTarget          float64  `json:"precisionTarget"`
	RecallTarget             float64  `json:"recallTarget"`
	BenchmarkQualified       bool     `json:"benchmarkQualified"`
	MaxLocateOutputTokens    int      `json:"maxLocateOutputTokens"`
	MaxReadOutputTokens      int      `json:"maxReadOutputTokens"`
	MaxOutputTokensPerRun    int      `json:"maxOutputTokensPerRun"`
}

type ChartAnalysisReviewPayload struct {
	ExpectedRevision        int                        `json:"expectedRevision"`
	Branches                []ChartBranchRegion        `json:"branches"`
	Names                   []ChartNameOccurrence      `json:"names"`
	Connectors              []ChartConnectorHypothesis `json:"connectors"`
	ReviewedTileIDs         []string                   `json:"reviewedTileIds"`
	ResidualRegionsReviewed bool                       `json:"residualRegionsReviewed"`
	CoverageConfirmed       bool                       `json:"coverageConfirmed"`
}

type ChartAnalysisView struct {
	ID               string                      `json:"id"`
	SourceID         string                      `json:"sourceId"`
	SourceFileID     string                      `json:"sourceFileId"`
	Status           string                      `json:"status"`
	QualityStatus    string                      `json:"qualityStatus"`
	ModelVersion     string                      `json:"modelVersion"`
	PipelineConfig   json.RawMessage             `json:"pipelineConfig"`
	PageCount        int                         `json:"pageCount"`
	PageWidth        int                         `json:"pageWidth"`
	PageHeight       int                         `json:"pageHeight"`
	PageWidthPoints  float64                     `json:"pageWidthPoints"`
	PageHeightPoints float64                     `json:"pageHeightPoints"`
	ReviewRevision   int                         `json:"reviewRevision"`
	Tiles            []ChartTileView             `json:"tiles"`
	Branches         []ChartBranchRegion         `json:"branches"`
	Names            []ChartNameOccurrence       `json:"names"`
	Connectors       []ChartConnectorHypothesis  `json:"connectors"`
	ResidualRegions  []ChartBox                  `json:"residualRegions"`
	Review           *ChartAnalysisReviewPayload `json:"review,omitempty"`
	Quality          ChartQualityReport          `json:"quality"`
	ErrorCode        string                      `json:"errorCode,omitempty"`
	CreatedAt        time.Time                   `json:"createdAt"`
	UpdatedAt        time.Time                   `json:"updatedAt"`
}

type chartRunRow struct {
	ID, SourceID, SourceFileID                   string
	Status, QualityStatus, ModelVersion          string
	PipelineConfig                               []byte
	PageCount, ProviderCallCount, ReviewRevision int
	InputImageBytes                              int64
	ErrorCode                                    pgtype.Text
	CreatedAt, UpdatedAt                         time.Time
}

type chartUnitRow struct {
	ID, UnitKey, UnitKind, Status string
	PageNumber                    int
	InputGeometry, OutputPayload  []byte
	ErrorCode                     pgtype.Text
}

// StartChartAnalysis explicitly declares the file to be a family chart and
// queues the isolated experimental workflow. Hosted processing is permitted
// only for the exact owner-approved PDF digest.
func (s *Service) StartChartAnalysis(ctx context.Context, fileID, actorID string) (ChartAnalysisView, error) {
	if err := s.databaseReady(); err != nil {
		return ChartAnalysisView{}, err
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	defer tx.Rollback(ctx)
	var sourceUUID pgtype.UUID
	var checksum, mimeType, visibility string
	if err := tx.QueryRow(ctx, `
		SELECT f.source_id, COALESCE(f.checksum_sha256, ''), COALESCE(f.mime_type, ''), s.visibility
		FROM source_files f JOIN sources s ON s.id = f.source_id WHERE f.id = $1 FOR UPDATE OF f
	`, fileUUID).Scan(&sourceUUID, &checksum, &mimeType, &visibility); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChartAnalysisView{}, ErrNotFound
		}
		return ChartAnalysisView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, uuidValue(sourceUUID), actorUUID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	if !allowed {
		return ChartAnalysisView{}, ErrForbidden
	}
	if visibility != "private" || normalizeContentType(mimeType) != "application/pdf" || !strings.EqualFold(checksum, JasserChartSHA256) {
		return ChartAnalysisView{}, ErrUnsupportedDocument
	}
	var activeID pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM source_chart_analysis_runs WHERE source_file_id = $1 AND status IN ('queued','running') ORDER BY created_at DESC LIMIT 1`, fileUUID).Scan(&activeID)
	if err == nil {
		if err := tx.Rollback(ctx); err != nil {
			return ChartAnalysisView{}, err
		}
		return s.GetChartAnalysis(ctx, fileUUID.String(), actorUUID.String())
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ChartAnalysisView{}, err
	}
	if s.Jobs == nil {
		return ChartAnalysisView{}, ErrQueueUnavailable
	}
	transactional, ok := s.Jobs.(TransactionalJobEnqueuer)
	if !ok {
		return ChartAnalysisView{}, ErrQueueUnavailable
	}
	runID := uuid.New()
	pipeline := map[string]any{
		"version": ChartAnalysisPipelineVersion, "model": ChartVisionModel,
		"renderDpi": 240, "tileSize": chartTileSize, "tileOverlap": chartTileOverlap,
		"tileGrid": "fixed-75-percent-stride-512px-v2", "crowdedTileSubdivision": "once-half-split-with-overlap-v2",
		"preprocessing": "original-pdf-render-no-upscale-v1",
		"tilePrompt":    "chart-locate-dense-512px-v2", "namePrompt": "arabic-name-pair-read-v1",
		"readBatchSize": chartMaxReadBatchImages, "readBatchMaxImageBytes": chartMaxReadBatchBytes,
		"maxProviderCalls": chartMaxProviderCalls, "maxLocateOutputTokens": ChartLocateOutputTokenLimit,
		"maxReadOutputTokens": ChartReadOutputTokenLimit, "maxOutputTokensPerRun": chartMaxProviderCalls * ChartLocateOutputTokenLimit,
		"thinkingMode": "disabled", "reasoningEffort": "omitted-when-thinking-disabled",
		"responseFormat":  "json-object-with-local-schema-validation-v1",
		"cacheIsolation":  "opaque-unit-user-id-v1",
		"coordinateSpace": "normalized-original-page", "hostedProvider": ChartAnalysisHostedProvider,
	}
	if _, err := tx.Exec(ctx, `UPDATE source_files SET document_kind = 'family_chart' WHERE id = $1`, fileUUID); err != nil {
		return ChartAnalysisView{}, err
	}
	// Previously generated unreviewed OCR candidates are withdrawn as soon as
	// the file is identified as a chart. Accepted/rejected research decisions stay.
	if _, err := tx.Exec(ctx, `
		UPDATE source_statements SET review_status = 'superseded', updated_at = now()
		WHERE id IN (SELECT source_statement_id FROM source_candidates WHERE source_file_id = $1 AND status IN ('unreviewed','needs_review') AND source_statement_id IS NOT NULL)
		AND review_status IN ('unreviewed','needs_review')
	`, fileUUID); err != nil {
		return ChartAnalysisView{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_candidates SET status = 'superseded', updated_at = now() WHERE source_file_id = $1 AND status IN ('unreviewed','needs_review')`, fileUUID); err != nil {
		return ChartAnalysisView{}, err
	}
	payload, err := json.Marshal(ChartAnalysisPayload{RunID: runID.String(), SourceFileID: fileUUID.String()})
	if err != nil {
		return ChartAnalysisView{}, err
	}
	enqueued, err := transactional.EnqueueTx(ctx, tx, jobs.EnqueueInput{
		Type: SourceChartAnalysisJobType, Payload: payload, Priority: 0,
		IdempotencyKey: "source_chart_analysis_run:" + runID.String(), MaxAttempts: 3,
	})
	if err != nil {
		return ChartAnalysisView{}, ErrQueueUnavailable
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_chart_analysis_runs (id, source_id, source_file_id, created_by, job_id, source_checksum_sha256, status, quality_status, pipeline_config, model_version)
		VALUES ($1,$2,$3,$4,$5,$6,'queued','unqualified',$7,$8)
	`, runID, sourceUUID, fileUUID, actorUUID, enqueued.Job.ID, strings.ToLower(checksum), mustJSON(pipeline), ChartVisionModel); err != nil {
		return ChartAnalysisView{}, err
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_chart_analysis_started", "source_file", fileUUID, nil, map[string]any{
		"analysisRunId": runID.String(), "modelVersion": ChartVisionModel, "pipelineVersion": ChartAnalysisPipelineVersion,
		"hostedProvider": ChartAnalysisHostedProvider, "qualityStatus": "unqualified",
	}); err != nil {
		return ChartAnalysisView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChartAnalysisView{}, err
	}
	return s.GetChartAnalysisRun(ctx, runID.String(), actorUUID.String())
}

func (s *Service) GetChartAnalysis(ctx context.Context, fileID, actorID string) (ChartAnalysisView, error) {
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	var runID pgtype.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM source_chart_analysis_runs WHERE source_file_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`, fileUUID).Scan(&runID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChartAnalysisView{}, ErrNotFound
		}
		return ChartAnalysisView{}, err
	}
	return s.GetChartAnalysisRun(ctx, runID.String(), actorUUID.String())
}

func (s *Service) GetChartAnalysisRun(ctx context.Context, runID, actorID string) (ChartAnalysisView, error) {
	if err := s.databaseReady(); err != nil {
		return ChartAnalysisView{}, err
	}
	runUUID, actorUUID, err := parseIDs(runID, actorID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	row, err := s.chartRun(ctx, s.Pool, runUUID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	allowed, err := canReviewSource(ctx, s.Pool, mustUUID(row.SourceID), actorUUID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	if !allowed {
		return ChartAnalysisView{}, ErrForbidden
	}
	return s.chartAnalysisView(ctx, row)
}

func (s *Service) RetryChartAnalysis(ctx context.Context, runID, actorID string) (ChartAnalysisView, error) {
	if err := s.databaseReady(); err != nil {
		return ChartAnalysisView{}, err
	}
	runUUID, actorUUID, err := parseIDs(runID, actorID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	defer tx.Rollback(ctx)
	row, err := s.chartRun(ctx, tx, runUUID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, mustUUID(row.SourceID), actorUUID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	if !allowed {
		return ChartAnalysisView{}, ErrForbidden
	}
	if row.Status != "partial" && row.Status != "failed" {
		return ChartAnalysisView{}, ErrConflict
	}
	if s.Jobs == nil {
		return ChartAnalysisView{}, ErrQueueUnavailable
	}
	transactional, ok := s.Jobs.(TransactionalJobEnqueuer)
	if !ok {
		return ChartAnalysisView{}, ErrQueueUnavailable
	}
	var sourceChecksum string
	if err := tx.QueryRow(ctx, `SELECT checksum_sha256 FROM source_files WHERE id = $1 FOR UPDATE`, row.SourceFileID).Scan(&sourceChecksum); err != nil {
		return ChartAnalysisView{}, err
	}
	if sourceChecksum != JasserChartSHA256 || sourceChecksum != strings.ToLower(sourceChecksum) {
		return ChartAnalysisView{}, ErrUnsupportedDocument
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_units SET status = 'pending', error_code = NULL WHERE run_id = $1 AND status = 'failed'`, runUUID); err != nil {
		return ChartAnalysisView{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_runs SET status = 'queued', error_code = NULL, completed_at = NULL, updated_at = now() WHERE id = $1`, runUUID); err != nil {
		return ChartAnalysisView{}, err
	}
	payload, _ := json.Marshal(ChartAnalysisPayload{RunID: runUUID.String(), SourceFileID: row.SourceFileID})
	enqueued, err := transactional.EnqueueTx(ctx, tx, jobs.EnqueueInput{Type: SourceChartAnalysisJobType, Payload: payload, Priority: 0, IdempotencyKey: "source_chart_analysis_retry:" + uuid.NewString(), MaxAttempts: 3})
	if err != nil {
		return ChartAnalysisView{}, ErrQueueUnavailable
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_runs SET job_id = $1 WHERE id = $2`, enqueued.Job.ID, runUUID); err != nil {
		return ChartAnalysisView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChartAnalysisView{}, err
	}
	return s.GetChartAnalysisRun(ctx, runUUID.String(), actorUUID.String())
}

func (s *Service) SaveChartAnalysisReview(ctx context.Context, runID, actorID string, input ChartAnalysisReviewPayload) (ChartAnalysisView, error) {
	if err := s.databaseReady(); err != nil {
		return ChartAnalysisView{}, err
	}
	runUUID, actorUUID, err := parseIDs(runID, actorID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize revisions so two reviewers cannot both save against the same
	// expected revision and receive a database error instead of a clean 409.
	var lockedID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM source_chart_analysis_runs WHERE id = $1 FOR UPDATE`, runUUID).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChartAnalysisView{}, ErrNotFound
		}
		return ChartAnalysisView{}, err
	}
	row, err := s.chartRun(ctx, tx, runUUID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, mustUUID(row.SourceID), actorUUID)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	if !allowed {
		return ChartAnalysisView{}, ErrForbidden
	}
	if row.Status != "succeeded" && row.Status != "partial" {
		return ChartAnalysisView{}, ErrConflict
	}
	current, err := s.chartAnalysisView(ctx, row)
	if err != nil {
		return ChartAnalysisView{}, err
	}
	if input.ExpectedRevision != row.ReviewRevision {
		return ChartAnalysisView{}, ErrConflict
	}
	input = normalizeChartReview(current, input)
	if err := validateChartAnalysisReview(current, input); err != nil {
		return ChartAnalysisView{}, err
	}
	revision := row.ReviewRevision + 1
	input.ExpectedRevision = revision
	if _, err := tx.Exec(ctx, `INSERT INTO source_chart_analysis_review_revisions (run_id, revision_number, reviewer_id, payload) VALUES ($1,$2,$3,$4)`, runUUID, revision, actorUUID, mustJSON(input)); err != nil {
		return ChartAnalysisView{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_runs SET current_review_revision = $2, updated_at = now() WHERE id = $1`, runUUID, revision); err != nil {
		return ChartAnalysisView{}, err
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_chart_analysis_reviewed", "source_file", mustUUID(row.SourceFileID), nil, map[string]any{
		"analysisRunId": runUUID.String(), "revision": revision, "nameCount": len(input.Names), "branchCount": len(input.Branches),
	}); err != nil {
		return ChartAnalysisView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChartAnalysisView{}, err
	}
	return s.GetChartAnalysisRun(ctx, runUUID.String(), actorUUID.String())
}

func validateChartAnalysisReview(current ChartAnalysisView, input ChartAnalysisReviewPayload) error {
	branchIDs := map[string]struct{}{}
	nameIDs := map[string]struct{}{}
	connectorIDs := map[string]struct{}{}
	for _, branch := range current.Branches {
		branchIDs[branch.ID] = struct{}{}
	}
	for _, name := range current.Names {
		nameIDs[name.ID] = struct{}{}
	}
	for _, connector := range current.Connectors {
		connectorIDs[connector.ID] = struct{}{}
	}
	if len(input.Branches) < len(branchIDs) || len(input.Names) < len(nameIDs) || len(input.Connectors) < len(connectorIDs) {
		return ErrValidation
	}
	tileIDs := map[string]struct{}{}
	for _, tile := range current.Tiles {
		tileIDs[tile.ID] = struct{}{}
	}
	seenTileIDs := map[string]struct{}{}
	for _, tileID := range input.ReviewedTileIDs {
		if _, ok := tileIDs[tileID]; !ok {
			return ErrValidation
		}
		if _, ok := seenTileIDs[tileID]; ok {
			return ErrValidation
		}
		seenTileIDs[tileID] = struct{}{}
	}
	if input.CoverageConfirmed && len(seenTileIDs) != len(tileIDs) {
		return ErrValidation
	}
	if input.CoverageConfirmed {
		if current.Quality.FailedUnitCount > 0 || current.Quality.PendingUnitCount > 0 {
			return ErrValidation
		}
		for _, tile := range current.Tiles {
			if tile.Status != "succeeded" {
				return ErrValidation
			}
		}
	}
	if input.ResidualRegionsReviewed && len(current.ResidualRegions) > 0 && !input.CoverageConfirmed {
		return ErrValidation
	}
	seen := map[string]struct{}{}
	allBranchIDs := map[string]struct{}{}
	for _, branch := range input.Branches {
		_, existing := branchIDs[branch.ID]
		if !existing && !validManualChartID(branch.ID) {
			return ErrValidation
		}
		if _, ok := seen[branch.ID]; ok || len(branch.Polygon) < 3 || len(branch.Polygon) > 100 || len([]rune(branch.Label)) > 120 {
			return ErrValidation
		}
		if branch.ReviewStatus != "needs_review" && branch.ReviewStatus != "confirmed" && branch.ReviewStatus != "rejected" {
			return ErrValidation
		}
		seen[branch.ID] = struct{}{}
		allBranchIDs[branch.ID] = struct{}{}
		for _, point := range branch.Polygon {
			if !unit(point[0]) || !unit(point[1]) {
				return ErrValidation
			}
		}
	}
	seen = map[string]struct{}{}
	reviewedNames := map[string]ChartNameOccurrence{}
	for _, name := range input.Names {
		_, existing := nameIDs[name.ID]
		if !existing && !validManualChartID(name.ID) {
			return ErrValidation
		}
		if name.BranchID != "" {
			if _, ok := allBranchIDs[name.BranchID]; !ok {
				return ErrValidation
			}
		}
		if _, ok := seen[name.ID]; ok || name.PageNumber != 1 || !validChartBox(name.Region) || len([]rune(name.TextAR)) > 200 {
			return ErrValidation
		}
		seen[name.ID] = struct{}{}
		reviewedNames[name.ID] = name
		if name.Legibility != "clear" && name.Legibility != "uncertain" && name.Legibility != "unreadable" {
			return ErrValidation
		}
		if !validReadingStatus(name.ReadingStatus) {
			return ErrValidation
		}
		if name.ReviewStatus != "needs_review" && name.ReviewStatus != "verified" && name.ReviewStatus != "rejected" && name.ReviewStatus != "unreadable" {
			return ErrValidation
		}
		if name.ReviewStatus == "verified" && strings.TrimSpace(name.TextAR) == "" {
			return ErrValidation
		}
	}
	for id := range nameIDs {
		if _, ok := seen[id]; !ok {
			return ErrValidation
		}
	}
	seen = map[string]struct{}{}
	for _, connector := range input.Connectors {
		_, existing := connectorIDs[connector.ID]
		if (!existing && !validManualChartID(connector.ID)) || connector.PageNumber != 1 || len(connector.Points) < 2 || len(connector.Points) > 300 {
			return ErrValidation
		}
		if connector.Kind != "line" && connector.Kind != "curve" && connector.Kind != "junction" && connector.Kind != "crossing" && connector.Kind != "uncertain" {
			return ErrValidation
		}
		if connector.Status != "needs_review" && connector.Status != "reviewed" && connector.Status != "uncertain" {
			return ErrValidation
		}
		if _, ok := seen[connector.ID]; ok {
			return ErrValidation
		}
		seen[connector.ID] = struct{}{}
		if connector.StartNameID != "" {
			if _, ok := seenNameID(input.Names, connector.StartNameID); !ok {
				return ErrValidation
			}
		}
		if connector.EndNameID != "" {
			if _, ok := seenNameID(input.Names, connector.EndNameID); !ok || connector.EndNameID == connector.StartNameID {
				return ErrValidation
			}
		}
		if connector.Status == "reviewed" && (connector.StartNameID == "" || connector.EndNameID == "") {
			return ErrValidation
		}
		if connector.Status == "reviewed" {
			startName := reviewedNames[connector.StartNameID]
			endName := reviewedNames[connector.EndNameID]
			if startName.ReviewStatus != "verified" || endName.ReviewStatus != "verified" {
				return ErrValidation
			}
		}
		for _, point := range connector.Points {
			if !unit(point[0]) || !unit(point[1]) {
				return ErrValidation
			}
		}
	}
	for id := range branchIDs {
		if _, ok := seenBranchID(input.Branches, id); !ok {
			return ErrValidation
		}
	}
	for id := range connectorIDs {
		if _, ok := seen[id]; !ok {
			return ErrValidation
		}
	}
	return nil
}

func validManualChartID(id string) bool {
	if !strings.HasPrefix(id, "manual_") {
		return false
	}
	_, err := uuid.Parse(strings.TrimPrefix(id, "manual_"))
	return err == nil
}

func seenNameID(names []ChartNameOccurrence, id string) (struct{}, bool) {
	for _, name := range names {
		if name.ID == id {
			return struct{}{}, true
		}
	}
	return struct{}{}, false
}

func seenBranchID(branches []ChartBranchRegion, id string) (struct{}, bool) {
	for _, branch := range branches {
		if branch.ID == id {
			return struct{}{}, true
		}
	}
	return struct{}{}, false
}

func invalidateChangedNameCrops(current ChartAnalysisView, input ChartAnalysisReviewPayload) ChartAnalysisReviewPayload {
	byID := make(map[string]ChartNameOccurrence, len(current.Names))
	for _, name := range current.Names {
		byID[name.ID] = name
	}
	for index := range input.Names {
		old, exists := byID[input.Names[index].ID]
		if !exists || old.Region == input.Names[index].Region {
			continue
		}
		input.Names[index].TextAR = ""
		input.Names[index].Alternatives = []string{}
		input.Names[index].UncertainSpans = []string{}
		input.Names[index].ReadingStatus = "unreadable"
		input.Names[index].ReviewStatus = "needs_review"
		input.Names[index].ReviewerNoteAR = ""
	}
	return input
}

func normalizeChartReview(current ChartAnalysisView, input ChartAnalysisReviewPayload) ChartAnalysisReviewPayload {
	input = invalidateChangedNameCrops(current, input)
	oldBranches := make(map[string]ChartBranchRegion, len(current.Branches))
	for _, branch := range current.Branches {
		oldBranches[branch.ID] = branch
	}
	for i := range input.Branches {
		previous, exists := oldBranches[input.Branches[i].ID]
		input.Branches[i].PageNumber = 1
		if input.Branches[i].ReviewStatus == "" {
			input.Branches[i].ReviewStatus = "needs_review"
		}
		input.Branches[i].ReviewerEdited = !exists || previous.ReviewerEdited || !sameChartPolygon(previous.Polygon, input.Branches[i].Polygon) || previous.Label != input.Branches[i].Label
	}
	oldNames := make(map[string]ChartNameOccurrence, len(current.Names))
	for _, name := range current.Names {
		oldNames[name.ID] = name
	}
	for i := range input.Names {
		input.Names[i].PageNumber = 1
		if previous, exists := oldNames[input.Names[i].ID]; exists {
			input.Names[i].SourceOccurrenceIDs = append([]string{}, previous.SourceOccurrenceIDs...)
			input.Names[i].SourceTileIDs = append([]string{}, previous.SourceTileIDs...)
		} else {
			input.Names[i].SourceOccurrenceIDs = []string{}
			input.Names[i].SourceTileIDs = []string{}
		}
	}
	return input
}

func sameChartPolygon(left, right []ChartPoint) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *Service) chartRun(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id uuid.UUID) (chartRunRow, error) {
	var row chartRunRow
	var idUUID, sourceUUID, fileUUID pgtype.UUID
	var config []byte
	err := q.QueryRow(ctx, `
		SELECT id, source_id, source_file_id, status, quality_status, model_version, pipeline_config,
		       page_count, provider_call_count, input_image_bytes, current_review_revision, error_code, created_at, updated_at
		FROM source_chart_analysis_runs WHERE id = $1
	`, id).Scan(&idUUID, &sourceUUID, &fileUUID, &row.Status, &row.QualityStatus, &row.ModelVersion, &config,
		&row.PageCount, &row.ProviderCallCount, &row.InputImageBytes, &row.ReviewRevision, &row.ErrorCode, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	row.ID, row.SourceID, row.SourceFileID = uuidValue(idUUID).String(), uuidValue(sourceUUID).String(), uuidValue(fileUUID).String()
	row.PipelineConfig = config
	return row, nil
}

func (s *Service) chartAnalysisView(ctx context.Context, row chartRunRow) (ChartAnalysisView, error) {
	view := ChartAnalysisView{
		ID: row.ID, SourceID: row.SourceID, SourceFileID: row.SourceFileID, Status: row.Status,
		QualityStatus: row.QualityStatus, ModelVersion: row.ModelVersion, PipelineConfig: row.PipelineConfig,
		PageCount: row.PageCount, ReviewRevision: row.ReviewRevision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		Tiles: []ChartTileView{}, Branches: []ChartBranchRegion{}, Names: []ChartNameOccurrence{}, Connectors: []ChartConnectorHypothesis{}, ResidualRegions: []ChartBox{},
		Quality: ChartQualityReport{Status: row.QualityStatus, Reasons: []string{"benchmark_not_qualified", "manual_source_review_required"}, ProviderCallCount: row.ProviderCallCount, InputImageBytes: row.InputImageBytes, PrecisionTarget: 0.99, RecallTarget: 0.95, BenchmarkQualified: false, MaxLocateOutputTokens: ChartLocateOutputTokenLimit, MaxReadOutputTokens: ChartReadOutputTokenLimit, MaxOutputTokensPerRun: chartMaxProviderCalls * ChartLocateOutputTokenLimit},
	}
	if row.ErrorCode.Valid {
		view.ErrorCode = row.ErrorCode.String
	}
	units, err := s.Pool.Query(ctx, `SELECT id, unit_key, unit_kind, page_number, status, input_geometry, output_payload, error_code FROM source_chart_analysis_units WHERE run_id = $1 ORDER BY page_number, unit_kind, unit_key`, mustUUID(row.ID))
	if err != nil {
		return ChartAnalysisView{}, err
	}
	defer units.Close()
	type tileRaw struct {
		id                                string
		bounds                            ChartTileBounds
		pageWidth, pageHeight             int
		pageWidthPoints, pageHeightPoints float64
		reading                           ChartTileReading
		status                            string
		incomplete                        bool
		errorCode                         string
	}
	tiles := []tileRaw{}
	readings := map[string]ChartNameReading{}
	for units.Next() {
		var item chartUnitRow
		var id pgtype.UUID
		if err := units.Scan(&id, &item.UnitKey, &item.UnitKind, &item.PageNumber, &item.Status, &item.InputGeometry, &item.OutputPayload, &item.ErrorCode); err != nil {
			return ChartAnalysisView{}, err
		}
		item.ID = uuidValue(id).String()
		if item.Status == "failed" {
			view.Quality.FailedUnitCount++
		} else if item.Status == "pending" || item.Status == "running" {
			view.Quality.PendingUnitCount++
		}
		errorCode := ""
		if item.ErrorCode.Valid {
			errorCode = item.ErrorCode.String
		}
		if item.UnitKind == "tile" || item.UnitKind == "subtile" {
			var geometry chartUnitGeometry
			if err := json.Unmarshal(item.InputGeometry, &geometry); err != nil {
				return ChartAnalysisView{}, ErrValidation
			}
			view.Tiles = append(view.Tiles, ChartTileView{ID: item.UnitKey, Page: item.PageNumber, Bounds: geometry.Bounds, Status: item.Status, ErrorCode: errorCode})
			if view.PageWidth == 0 {
				view.PageWidth, view.PageHeight = geometry.PageWidth, geometry.PageHeight
				view.PageWidthPoints, view.PageHeightPoints = geometry.PageWidthPoints, geometry.PageHeightPoints
			}
			if item.Status == "succeeded" {
				var stored chartStoredTile
				if err := json.Unmarshal(item.OutputPayload, &stored); err != nil {
					return ChartAnalysisView{}, ErrValidation
				}
				if stored.Incomplete {
					view.Tiles[len(view.Tiles)-1].Incomplete = true
					view.Quality.IncompleteTileCount++
				}
				view.Tiles[len(view.Tiles)-1].SubdivisionReason = stored.SubdivisionReason
				tiles = append(tiles, tileRaw{id: item.UnitKey, bounds: geometry.Bounds, pageWidth: geometry.PageWidth, pageHeight: geometry.PageHeight, pageWidthPoints: geometry.PageWidthPoints, pageHeightPoints: geometry.PageHeightPoints, reading: stored.Reading, status: item.Status, incomplete: stored.Incomplete, errorCode: errorCode})
			}
		} else if item.UnitKind == "reading_batch" && item.Status == "succeeded" {
			var output struct {
				Readings []ChartNameReading `json:"readings"`
			}
			if err := json.Unmarshal(item.OutputPayload, &output); err != nil {
				return ChartAnalysisView{}, ErrValidation
			}
			for _, reading := range output.Readings {
				readings[reading.OccurrenceID] = reading
			}
		}
	}
	if err := units.Err(); err != nil {
		return ChartAnalysisView{}, err
	}
	branches := []ChartBranchRegion{}
	labels := []ChartNameOccurrence{}
	connectors := []ChartConnectorHypothesis{}
	branchByTile := map[string][]string{}
	for _, tile := range tiles {
		ids := make([]string, len(tile.reading.Branches))
		for index, branch := range tile.reading.Branches {
			polygon := make([]ChartPoint, len(branch.Polygon))
			for pointIndex, point := range branch.Polygon {
				polygon[pointIndex] = ChartPoint{(float64(tile.bounds.X) + point[0]*float64(tile.bounds.Width)) / float64(tile.pageWidth), (float64(tile.bounds.Y) + point[1]*float64(tile.bounds.Height)) / float64(tile.pageHeight)}
			}
			candidate := ChartBranchRegion{ID: stableChartID(row.ID, tile.id, "branch", index), PageNumber: 1, Polygon: polygon, Label: branch.Label, ReviewStatus: "needs_review"}
			canonical := candidate.ID
			for existingIndex := range branches {
				if branchGeometryOverlap(branches[existingIndex].Polygon, candidate.Polygon) >= 0.8 {
					canonical = branches[existingIndex].ID
					break
				}
			}
			if canonical == candidate.ID {
				branches = append(branches, candidate)
			}
			ids[index] = canonical
		}
		branchByTile[tile.id] = ids
		for index, label := range tile.reading.Labels {
			box := ChartBox{
				X:      (float64(tile.bounds.X) + label.Box.X*float64(tile.bounds.Width)) / float64(tile.pageWidth),
				Y:      (float64(tile.bounds.Y) + label.Box.Y*float64(tile.bounds.Height)) / float64(tile.pageHeight),
				Width:  label.Box.Width * float64(tile.bounds.Width) / float64(tile.pageWidth),
				Height: label.Box.Height * float64(tile.bounds.Height) / float64(tile.pageHeight),
			}
			branchID := ""
			if label.BranchIndex != nil && *label.BranchIndex >= 0 && *label.BranchIndex < len(ids) {
				branchID = ids[*label.BranchIndex]
			}
			candidate := ChartNameOccurrence{ID: stableChartID(row.ID, tile.id, "label", index), SourceOccurrenceIDs: []string{stableChartID(row.ID, tile.id, "label", index)}, SourceTileIDs: []string{tile.id}, PageNumber: 1, Region: box, BranchID: branchID, Legibility: label.Legibility, ReadingStatus: "unreadable", Alternatives: []string{}, UncertainSpans: []string{}, ReviewStatus: "needs_review"}
			duplicateIndex := -1
			for existingIndex, existing := range labels {
				if boxOverlapSmaller(existing.Region, candidate.Region) >= 0.82 {
					duplicateIndex = existingIndex
					break
				}
			}
			if duplicateIndex < 0 {
				labels = append(labels, candidate)
			} else {
				labels[duplicateIndex].SourceOccurrenceIDs = append(labels[duplicateIndex].SourceOccurrenceIDs, candidate.SourceOccurrenceIDs...)
				labels[duplicateIndex].SourceTileIDs = append(labels[duplicateIndex].SourceTileIDs, candidate.SourceTileIDs...)
			}
		}
		for index, connector := range tile.reading.Connectors {
			points := make([]ChartPoint, len(connector.Points))
			for pointIndex, point := range connector.Points {
				points[pointIndex] = ChartPoint{(float64(tile.bounds.X) + point[0]*float64(tile.bounds.Width)) / float64(tile.pageWidth), (float64(tile.bounds.Y) + point[1]*float64(tile.bounds.Height)) / float64(tile.pageHeight)}
			}
			connectors = append(connectors, ChartConnectorHypothesis{ID: stableChartID(row.ID, tile.id, "connector", index), PageNumber: 1, Points: points, Kind: connector.Kind, Status: "needs_review"})
		}
		for _, residual := range tile.reading.ResidualRegions {
			view.ResidualRegions = append(view.ResidualRegions, ChartBox{X: (float64(tile.bounds.X) + residual.X*float64(tile.bounds.Width)) / float64(tile.pageWidth), Y: (float64(tile.bounds.Y) + residual.Y*float64(tile.bounds.Height)) / float64(tile.pageHeight), Width: residual.Width * float64(tile.bounds.Width) / float64(tile.pageWidth), Height: residual.Height * float64(tile.bounds.Height) / float64(tile.pageHeight)})
		}
	}
	for index := range labels {
		if reading, ok := readings[labels[index].ID]; ok {
			labels[index].TextAR, labels[index].ReadingStatus = reading.TextAR, reading.Status
			labels[index].Alternatives, labels[index].UncertainSpans = reading.Alternatives, reading.UncertainSpans
		}
	}
	view.Branches, view.Names, view.Connectors = branches, labels, connectors
	view.Quality.TileCount = len(view.Tiles)
	for _, tile := range view.Tiles {
		if tile.Status == "succeeded" {
			view.Quality.ProcessedTileCount++
		} else if tile.Status == "failed" {
			view.Quality.FailedTileCount++
		}
	}
	view.Quality.NameOccurrenceCount = len(labels)
	for _, branch := range branches {
		if branch.ReviewStatus != "confirmed" && branch.ReviewStatus != "rejected" {
			view.Quality.UnreviewedBranchCount++
		}
	}
	view.Quality.UnresolvedConnectorCount = len(connectors)
	for _, name := range labels {
		if name.ReadingStatus == "unreadable" {
			view.Quality.UnreadableNameCount++
		}
		if name.ReviewStatus == "needs_review" {
			view.Quality.UnreviewedNameCount++
		}
	}
	if row.ReviewRevision > 0 {
		var payload []byte
		if err := s.Pool.QueryRow(ctx, `SELECT payload FROM source_chart_analysis_review_revisions WHERE run_id = $1 AND revision_number = $2`, mustUUID(row.ID), row.ReviewRevision).Scan(&payload); err != nil {
			return ChartAnalysisView{}, err
		}
		var review ChartAnalysisReviewPayload
		if err := json.Unmarshal(payload, &review); err != nil {
			return ChartAnalysisView{}, ErrValidation
		}
		view.Review = &review
		view.Review.ExpectedRevision = row.ReviewRevision
		view.Branches = append([]ChartBranchRegion{}, review.Branches...)
		view.Names = append([]ChartNameOccurrence{}, review.Names...)
		view.Connectors = append([]ChartConnectorHypothesis{}, review.Connectors...)
	}
	view.Quality.UnreviewedNameCount = 0
	view.Quality.UnreadableNameCount = 0
	view.Quality.UnreviewedBranchCount = 0
	for _, name := range view.Names {
		if name.ReviewStatus == "needs_review" {
			view.Quality.UnreviewedNameCount++
		}
		if name.ReadingStatus == "unreadable" {
			view.Quality.UnreadableNameCount++
		}
	}
	for _, branch := range view.Branches {
		if branch.ReviewStatus != "confirmed" && branch.ReviewStatus != "rejected" {
			view.Quality.UnreviewedBranchCount++
		}
	}
	view.Quality.UnresolvedConnectorCount = 0
	for _, connector := range view.Connectors {
		if connector.Status != "reviewed" {
			view.Quality.UnresolvedConnectorCount++
		}
	}
	if view.Quality.ProcessedTileCount < view.Quality.TileCount {
		view.Quality.Reasons = append(view.Quality.Reasons, "page_coverage_incomplete")
	}
	if view.Quality.FailedUnitCount > 0 {
		view.Quality.Reasons = append(view.Quality.Reasons, "analysis_units_failed")
	}
	if view.Quality.IncompleteTileCount > 0 && (view.Review == nil || !view.Review.CoverageConfirmed) {
		view.Quality.Reasons = append(view.Quality.Reasons, "dense_regions_need_manual_subdivision")
	}
	if view.Quality.PendingUnitCount > 0 {
		view.Quality.Reasons = append(view.Quality.Reasons, "analysis_units_pending")
	}
	if view.Quality.UnreviewedNameCount > 0 {
		view.Quality.Reasons = append(view.Quality.Reasons, "reviewer_names_pending")
	}
	if view.Quality.UnreviewedBranchCount > 0 {
		view.Quality.Reasons = append(view.Quality.Reasons, "reviewer_branches_pending")
	}
	if view.Quality.UnresolvedConnectorCount > 0 {
		view.Quality.Reasons = append(view.Quality.Reasons, "connector_paths_pending")
	}
	if view.Review == nil || !view.Review.CoverageConfirmed {
		view.Quality.Reasons = append(view.Quality.Reasons, "full_page_coverage_not_human_confirmed")
	}
	if view.Review == nil || !view.Review.ResidualRegionsReviewed {
		view.Quality.Reasons = append(view.Quality.Reasons, "residual_regions_not_reviewed")
	}
	return view, nil
}

func (s *Service) ProcessJob(ctx context.Context, claim Claim) error {
	if claim.Job.Type == SourceChartAnalysisJobType {
		return s.ProcessChartAnalysis(ctx, claim)
	}
	return s.Process(ctx, claim)
}

func (s *Service) ProcessChartAnalysis(ctx context.Context, claim Claim) error {
	if claim.Job.Type != SourceChartAnalysisJobType || s == nil || s.Pool == nil || s.Store == nil || s.ChartVision == nil || s.Jobs == nil {
		return ErrChartVisionUnavailable
	}
	heartbeat, workCtx, err := s.startHeartbeat(ctx, claim)
	if err != nil {
		return err
	}
	defer func() { _ = heartbeat.Stop() }()
	ctx = workCtx
	var payload ChartAnalysisPayload
	if err := json.Unmarshal(claim.Job.Payload, &payload); err != nil {
		return ErrValidation
	}
	runID, err := uuid.Parse(payload.RunID)
	if err != nil {
		return ErrValidation
	}
	var sourceID pgtype.UUID
	var fileID pgtype.UUID
	var pipelineConfig []byte
	var pipelineModel string
	var storageKey, checksum, mimeType string
	if err := s.Pool.QueryRow(ctx, `
		SELECT r.source_id, r.source_file_id, r.pipeline_config, r.model_version, f.storage_key, f.checksum_sha256, f.mime_type
		FROM source_chart_analysis_runs r JOIN source_files f ON f.id = r.source_file_id WHERE r.id = $1
	`, runID).Scan(&sourceID, &fileID, &pipelineConfig, &pipelineModel, &storageKey, &checksum, &mimeType); err != nil {
		return err
	}
	if uuidValue(fileID).String() != payload.SourceFileID || checksum != JasserChartSHA256 || mimeType != "application/pdf" {
		return ErrUnsupportedDocument
	}
	if !chartPipelineMatchesCurrent(pipelineConfig, pipelineModel) {
		return s.failChartPipelineMismatch(ctx, claim, runID)
	}
	if err := s.setChartRunStatus(ctx, claim, runID, "running", "unqualified", ""); err != nil {
		return err
	}
	reader, object, err := s.Store.Get(ctx, storageKey)
	if err != nil {
		_ = s.setChartRunStatus(ctx, claim, runID, "failed", "manual_required", "source_unavailable")
		return ErrStorageUnavailable
	}
	content, readErr := io.ReadAll(io.LimitReader(reader, MaxUploadBytes+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(content) > MaxUploadBytes || object.Size != int64(len(content)) {
		_ = s.setChartRunStatus(ctx, claim, runID, "failed", "manual_required", "source_unavailable")
		return ErrStorageUnavailable
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != JasserChartSHA256 {
		_ = s.setChartRunStatus(ctx, claim, runID, "failed", "manual_required", "source_checksum_changed")
		return ErrValidation
	}
	extractor, ok := s.Extractor.(*PDFExtractor)
	if !ok || extractor == nil {
		extractor = NewPDFExtractor()
	}
	pdfPage, err := extractor.OpenChartPageForAnalysis(ctx, content, 1)
	if err != nil {
		_ = s.setChartRunStatus(ctx, claim, runID, "failed", "manual_required", "page_render_failed")
		return err
	}
	defer pdfPage.Close()
	if pdfPage.PageCount != 1 {
		_ = s.setChartRunStatus(ctx, claim, runID, "failed", "manual_required", "unsupported_page_count")
		return nil
	}
	pageWidth, pageHeight := pdfPage.Width, pdfPage.Height
	if err := s.setChartPageCount(ctx, claim, runID, pdfPage.PageCount); err != nil {
		return err
	}
	if chartTileCount(pageWidth, pageHeight) > 512 {
		return s.failChart(ctx, claim, runID, "tile_budget_exceeded")
	}
	grid := chartTileBounds(pageWidth, pageHeight)
	if len(grid) == 0 || len(grid) > 512 {
		return s.failChart(ctx, claim, runID, "tile_budget_exceeded")
	}
	for _, bounds := range grid {
		key := fmt.Sprintf("p1_x%d_y%d", bounds.X, bounds.Y)
		geometry := chartUnitGeometry{Bounds: bounds, PageWidth: pageWidth, PageHeight: pageHeight, PageWidthPoints: pdfPage.WidthPoints, PageHeightPoints: pdfPage.HeightPoints, RenderDPI: pdfPage.DPI}
		if err := s.ensureChartUnit(ctx, claim, runID, key, "tile", geometry); err != nil {
			return err
		}
	}
	visionHalted := false
	for _, bounds := range grid {
		key := fmt.Sprintf("p1_x%d_y%d", bounds.X, bounds.Y)
		if visionHalted {
			break
		}
		halt, err := s.processChartLocateUnit(ctx, heartbeat, claim, runID, pdfPage, key, bounds, "tile", true)
		if err != nil {
			return err
		}
		if halt {
			visionHalted = true
		}
	}
	occurrences, err := s.collectChartOccurrences(ctx, runID, pageWidth, pageHeight)
	if err != nil {
		return err
	}
	if len(occurrences) > chartMaxNameOccurrences {
		return s.failChart(ctx, claim, runID, "occurrence_budget_exceeded")
	}
	readBatches, err := s.makeChartReadBatchesFromPDF(ctx, runID, pdfPage, occurrences)
	if err != nil {
		return err
	}
	for _, batch := range readBatches {
		if err := s.ensureChartUnit(ctx, claim, runID, batch.Key, "reading_batch", chartUnitGeometry{PageWidth: pageWidth, PageHeight: pageHeight, PageWidthPoints: pdfPage.WidthPoints, PageHeightPoints: pdfPage.HeightPoints, RenderDPI: pdfPage.DPI}); err != nil {
			return err
		}
	}
	for _, batch := range readBatches {
		if visionHalted {
			break
		}
		unit, err := s.chartUnit(ctx, runID, batch.Key)
		if err != nil {
			return err
		}
		if unit.Status == "succeeded" {
			continue
		}
		if batch.ErrorCode != "" {
			if err := s.failChartUnit(ctx, claim, runID, batch.Key, batch.ErrorCode); err != nil {
				return err
			}
			continue
		}
		if err := s.setChartUnitRunning(ctx, claim, runID, batch.Key); err != nil {
			return err
		}
		var readings []ChartNameReading
		err = s.withChartVisionRetry(ctx, heartbeat, claim, runID, batch.InputBytes, func() error {
			var callErr error
			readings, callErr = s.ChartVision.ReadBatch(ctx, batch.Inputs)
			return callErr
		})
		if err != nil {
			_ = s.failChartUnit(ctx, claim, runID, batch.Key, chartErrorCode(err))
			if errors.Is(err, ErrChartVisionUnavailable) || errors.Is(err, ErrChartAnalysisBudget) {
				visionHalted = true
				break
			}
			continue
		}
		encoded, _ := json.Marshal(map[string]any{"readings": readings})
		if err := s.completeChartUnit(ctx, claim, runID, batch.Key, encoded); err != nil {
			return err
		}
	}
	var failed int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*)::integer FROM source_chart_analysis_units WHERE run_id = $1 AND status <> 'succeeded'`, runID).Scan(&failed); err != nil {
		return err
	}
	status, quality := "succeeded", "unqualified"
	failureCode := ""
	if failed > 0 {
		status, quality = "partial", "manual_required"
		_ = s.Pool.QueryRow(ctx, `SELECT error_code FROM source_chart_analysis_units WHERE run_id = $1 AND error_code IS NOT NULL ORDER BY unit_kind, unit_key LIMIT 1`, runID).Scan(&failureCode)
	}
	return s.setChartRunStatus(ctx, claim, runID, status, quality, failureCode)
}

type chartPipelineIdentity struct {
	Version        string `json:"version"`
	Model          string `json:"model"`
	HostedProvider string `json:"hostedProvider"`
}

func chartPipelineMatchesCurrent(config []byte, modelVersion string) bool {
	var identity chartPipelineIdentity
	if json.Unmarshal(config, &identity) != nil {
		return false
	}
	return identity.Version == ChartAnalysisPipelineVersion &&
		identity.Model == ChartVisionModel &&
		identity.HostedProvider == ChartAnalysisHostedProvider &&
		modelVersion == ChartVisionModel
}

// A recovered run must not mix results from different providers, prompts or
// privacy settings. Mark every unfinished unit visibly failed when its immutable
// pipeline identity no longer matches the code that would resume it.
func (s *Service) failChartPipelineMismatch(ctx context.Context, claim Claim, runID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_chart_analysis_units
		SET status = 'failed', error_code = 'pipeline_configuration_changed', updated_at = now()
		WHERE run_id = $1 AND status IN ('pending','running')
	`, runID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_chart_analysis_runs
		SET status = 'failed', quality_status = 'manual_required', error_code = 'pipeline_configuration_changed',
		    updated_at = now(), completed_at = now()
		WHERE id = $1 AND status IN ('queued','running')
	`, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// processChartLocateUnit resumes a tile checkpoint. A model result at the
// schema ceiling (or an explicit truncation note) gets one overlapping split
// pass. Child units are never split again; a still-crowded child is persisted
// as incomplete and blocks human coverage confirmation.
func (s *Service) processChartLocateUnit(ctx context.Context, heartbeat *jobs.Heartbeat, claim Claim, runID uuid.UUID, pdfPage *ChartPDFPage, key string, bounds ChartTileBounds, unitKind string, allowSubdivision bool) (bool, error) {
	geometry := chartUnitGeometry{Bounds: bounds, PageWidth: pdfPage.Width, PageHeight: pdfPage.Height, PageWidthPoints: pdfPage.WidthPoints, PageHeightPoints: pdfPage.HeightPoints, RenderDPI: pdfPage.DPI}
	if err := s.ensureChartUnit(ctx, claim, runID, key, unitKind, geometry); err != nil {
		return false, err
	}
	unit, err := s.chartUnit(ctx, runID, key)
	if err != nil {
		return false, err
	}
	if unit.Status == "succeeded" {
		var stored chartStoredTile
		if err := json.Unmarshal(unit.OutputPayload, &stored); err != nil {
			return false, ErrValidation
		}
		if stored.Subdivided {
			for index, child := range chartSubtileBounds(bounds) {
				childKey := fmt.Sprintf("%s_s%d", key, index)
				halt, err := s.processChartLocateUnit(ctx, heartbeat, claim, runID, pdfPage, childKey, child, "subtile", false)
				if err != nil || halt {
					return halt, err
				}
			}
		}
		return false, nil
	}
	if err := heartbeat.Check(); err != nil {
		return false, err
	}
	tilePNG, err := pdfPage.RenderRegion(ctx, bounds)
	if err != nil {
		if failErr := s.failChartUnit(ctx, claim, runID, key, "crop_failed"); failErr != nil {
			return false, failErr
		}
		return false, nil
	}
	if len(tilePNG) > 8<<20 {
		if failErr := s.failChartUnit(ctx, claim, runID, key, "source_tile_too_large"); failErr != nil {
			return false, failErr
		}
		return false, nil
	}
	if err := s.setChartUnitRunning(ctx, claim, runID, key); err != nil {
		return false, err
	}
	var reading ChartTileReading
	err = s.withChartVisionRetry(ctx, heartbeat, claim, runID, int64(len(tilePNG)), func() error {
		var callErr error
		reading, callErr = s.ChartVision.LocateTile(ctx, key, tilePNG)
		return callErr
	})
	if err != nil {
		if chartOutputNeedsSubdivision(err, bounds, allowSubdivision) {
			reason := "provider_output_invalid"
			if errors.Is(err, ErrChartVisionTruncated) {
				reason = "provider_output_truncated"
			}
			stored, marshalErr := json.Marshal(chartStoredTile{Bounds: bounds, Subdivided: true, SubdivisionReason: reason})
			if marshalErr != nil {
				return false, marshalErr
			}
			if completeErr := s.completeChartUnit(ctx, claim, runID, key, stored); completeErr != nil {
				return false, completeErr
			}
			for index, child := range chartSubtileBounds(bounds) {
				childKey := fmt.Sprintf("%s_s%d", key, index)
				halt, childErr := s.processChartLocateUnit(ctx, heartbeat, claim, runID, pdfPage, childKey, child, "subtile", false)
				if childErr != nil || halt {
					return halt, childErr
				}
			}
			return false, nil
		}
		if failErr := s.failChartUnit(ctx, claim, runID, key, chartErrorCode(err)); failErr != nil {
			return false, failErr
		}
		if errors.Is(err, ErrChartAnalysisBudget) || errors.Is(err, ErrChartVisionSetup) || errors.Is(err, ErrChartVisionUnavailable) {
			return true, nil
		}
		return false, nil
	}
	needsSubdivision := crowdedChartReading(reading)
	subdivide := allowSubdivision && needsSubdivision && max(bounds.Width, bounds.Height) > chartTileSize/2
	incomplete := !allowSubdivision && needsSubdivision
	subdivisionReason := ""
	if subdivide {
		subdivisionReason = chartSubdivisionReason(reading)
	}
	stored, err := json.Marshal(chartStoredTile{Bounds: bounds, Reading: reading, Subdivided: subdivide, Incomplete: incomplete, SubdivisionReason: subdivisionReason})
	if err != nil {
		return false, err
	}
	if err := s.completeChartUnit(ctx, claim, runID, key, stored); err != nil {
		return false, err
	}
	if subdivide {
		for index, child := range chartSubtileBounds(bounds) {
			childKey := fmt.Sprintf("%s_s%d", key, index)
			halt, err := s.processChartLocateUnit(ctx, heartbeat, claim, runID, pdfPage, childKey, child, "subtile", false)
			if err != nil || halt {
				return halt, err
			}
		}
	}
	return false, nil
}

func crowdedChartReading(reading ChartTileReading) bool {
	if len(reading.Labels) >= 250 {
		return true
	}
	note := strings.ToLower(reading.CompletenessNote)
	for _, marker := range []string{"truncat", "output limit", "too many", "crowded", "dense", "could not inspect all", "not all labels"} {
		if strings.Contains(note, marker) {
			return true
		}
	}
	return false
}

func chartSubdivisionReason(reading ChartTileReading) string {
	if len(reading.Labels) >= 250 {
		return "label_output_limit"
	}
	return "reader_reported_dense_or_truncated"
}

func chartSubtileBounds(parent ChartTileBounds) []ChartTileBounds {
	axis := func(start, size int) [][2]int {
		if size <= chartTileSize/2 {
			return [][2]int{{start, size}}
		}
		middle := size / 2
		overlap := max(1, size/8)
		return [][2]int{{start, middle + overlap}, {start + middle - overlap, size - middle + overlap}}
	}
	xs, ys := axis(parent.X, parent.Width), axis(parent.Y, parent.Height)
	children := make([]ChartTileBounds, 0, len(xs)*len(ys))
	for _, y := range ys {
		for _, x := range xs {
			children = append(children, ChartTileBounds{X: x[0], Y: y[0], Width: x[1], Height: y[1]})
		}
	}
	return children
}

func (s *Service) collectChartOccurrences(ctx context.Context, runID uuid.UUID, pageWidth, pageHeight int) ([]ChartNameOccurrence, error) {
	rows, err := s.Pool.Query(ctx, `SELECT unit_key, input_geometry, output_payload FROM source_chart_analysis_units WHERE run_id = $1 AND unit_kind IN ('tile','subtile') AND status = 'succeeded' ORDER BY page_number, unit_key`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := []ChartNameOccurrence{}
	for rows.Next() {
		var key string
		var input, output []byte
		if err := rows.Scan(&key, &input, &output); err != nil {
			return nil, err
		}
		var geometry chartUnitGeometry
		var stored chartStoredTile
		if json.Unmarshal(input, &geometry) != nil || json.Unmarshal(output, &stored) != nil {
			return nil, ErrValidation
		}
		branchIDs := make([]string, len(stored.Reading.Branches))
		for i := range branchIDs {
			branchIDs[i] = stableChartID(runID.String(), key, "branch", i)
		}
		for index, label := range stored.Reading.Labels {
			box := ChartBox{X: (float64(geometry.Bounds.X) + label.Box.X*float64(geometry.Bounds.Width)) / float64(pageWidth), Y: (float64(geometry.Bounds.Y) + label.Box.Y*float64(geometry.Bounds.Height)) / float64(pageHeight), Width: label.Box.Width * float64(geometry.Bounds.Width) / float64(pageWidth), Height: label.Box.Height * float64(geometry.Bounds.Height) / float64(pageHeight)}
			branchID := ""
			if label.BranchIndex != nil && *label.BranchIndex >= 0 && *label.BranchIndex < len(branchIDs) {
				branchID = branchIDs[*label.BranchIndex]
			}
			candidate := ChartNameOccurrence{ID: stableChartID(runID.String(), key, "label", index), SourceOccurrenceIDs: []string{stableChartID(runID.String(), key, "label", index)}, SourceTileIDs: []string{key}, PageNumber: 1, Region: box, BranchID: branchID, Legibility: label.Legibility, ReadingStatus: "unreadable", Alternatives: []string{}, UncertainSpans: []string{}, ReviewStatus: "needs_review"}
			duplicate := false
			for _, existing := range labels {
				if boxOverlapSmaller(existing.Region, candidate.Region) >= 0.82 {
					duplicate = true
					break
				}
			}
			if !duplicate {
				labels = append(labels, candidate)
			} else {
				for existingIndex := range labels {
					if boxOverlapSmaller(labels[existingIndex].Region, candidate.Region) >= 0.82 {
						labels[existingIndex].SourceOccurrenceIDs = append(labels[existingIndex].SourceOccurrenceIDs, candidate.SourceOccurrenceIDs...)
						labels[existingIndex].SourceTileIDs = append(labels[existingIndex].SourceTileIDs, candidate.SourceTileIDs...)
						break
					}
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortChartOccurrences(labels)
	return labels, nil
}

func sortChartOccurrences(items []ChartNameOccurrence) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && (items[j].Region.Y < items[j-1].Region.Y || (items[j].Region.Y == items[j-1].Region.Y && items[j].Region.X < items[j-1].Region.X)); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func (s *Service) withChartVisionRetry(ctx context.Context, heartbeat *jobs.Heartbeat, claim Claim, runID uuid.UUID, bytesCount int64, call func() error) error {
	for attempt := 0; attempt < 3; attempt++ {
		if err := heartbeat.Check(); err != nil {
			return err
		}
		if err := s.consumeChartCall(ctx, claim, runID, bytesCount); err != nil {
			return err
		}
		err := call()
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrChartVisionSetup) {
			return err
		}
		if !errors.Is(err, ErrChartVisionUnavailable) || attempt == 2 {
			return err
		}
		wait := time.Duration(attempt+1) * 350 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return ErrChartVisionUnavailable
}

func (s *Service) consumeChartCall(ctx context.Context, claim Claim, runID uuid.UUID, inputBytes int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE source_chart_analysis_runs SET provider_call_count = provider_call_count + 1, input_image_bytes = input_image_bytes + $2, updated_at = now() WHERE id = $1 AND provider_call_count < $3`, runID, inputBytes, chartMaxProviderCalls)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrChartAnalysisBudget
	}
	return tx.Commit(ctx)
}

func (s *Service) ensureChartUnit(ctx context.Context, claim Claim, runID uuid.UUID, key, kind string, geometry any) error {
	data, err := json.Marshal(geometry)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_chart_analysis_units (run_id, unit_key, unit_kind, page_number, input_geometry) VALUES ($1,$2,$3,1,$4) ON CONFLICT (run_id,unit_key) DO NOTHING`, runID, key, kind, data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) chartUnit(ctx context.Context, runID uuid.UUID, key string) (chartUnitRow, error) {
	var item chartUnitRow
	var id pgtype.UUID
	err := s.Pool.QueryRow(ctx, `SELECT id, unit_key, unit_kind, status, page_number, input_geometry, output_payload, error_code FROM source_chart_analysis_units WHERE run_id = $1 AND unit_key = $2`, runID, key).Scan(&id, &item.UnitKey, &item.UnitKind, &item.Status, &item.PageNumber, &item.InputGeometry, &item.OutputPayload, &item.ErrorCode)
	item.ID = uuidValue(id).String()
	return item, err
}

func (s *Service) setChartUnitRunning(ctx context.Context, claim Claim, runID uuid.UUID, key string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_units SET status = 'running', attempts = attempts + 1, error_code = NULL, updated_at = now() WHERE run_id = $1 AND unit_key = $2 AND status <> 'succeeded'`, runID, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) completeChartUnit(ctx context.Context, claim Claim, runID uuid.UUID, key string, output []byte) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_units SET status = 'succeeded', output_payload = $3, error_code = NULL, updated_at = now() WHERE run_id = $1 AND unit_key = $2`, runID, key, output); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) failChartUnit(ctx context.Context, claim Claim, runID uuid.UUID, key, code string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_units SET status = 'failed', error_code = $3, updated_at = now() WHERE run_id = $1 AND unit_key = $2 AND status <> 'succeeded'`, runID, key, code); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) setChartPageCount(ctx context.Context, claim Claim, runID uuid.UUID, pages int) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_runs SET page_count = $2, updated_at = now() WHERE id = $1`, runID, pages); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) setChartRunStatus(ctx context.Context, claim Claim, runID uuid.UUID, status, quality, errorCode string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim.Lease); err != nil {
		return err
	}
	var errorValue any
	if errorCode != "" {
		errorValue = errorCode
	}
	if _, err := tx.Exec(ctx, `UPDATE source_chart_analysis_runs SET status = $2, quality_status = $3, error_code = $4, updated_at = now(), completed_at = CASE WHEN $2 IN ('succeeded','partial','failed') THEN now() ELSE NULL END WHERE id = $1`, runID, status, quality, errorValue); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) failChart(ctx context.Context, claim Claim, runID uuid.UUID, code string) error {
	if err := s.setChartRunStatus(ctx, claim, runID, "failed", "manual_required", code); err != nil {
		return err
	}
	return nil
}

func chartErrorCode(err error) string {
	var outputError chartVisionOutputValidationError
	if errors.As(err, &outputError) {
		return "provider_output_invalid_" + outputError.category
	}
	switch {
	case errors.Is(err, ErrChartAnalysisBudget):
		return "request_budget_exhausted"
	case errors.Is(err, ErrChartVisionSetup):
		return "hosted_vision_setup_required"
	case errors.Is(err, ErrChartVisionRejected):
		return "provider_request_rejected"
	case errors.Is(err, ErrChartVisionTruncated):
		return "provider_output_truncated_after_subdivision"
	case errors.Is(err, ErrChartVisionUnavailable):
		return "provider_request_failed"
	case errors.Is(err, ErrChartVisionOutputInvalid):
		return "model_output_invalid"
	case errors.Is(err, ErrValidation):
		return "model_output_invalid"
	default:
		return "provider_request_failed"
	}
}

func chartOutputNeedsSubdivision(err error, bounds ChartTileBounds, allowSubdivision bool) bool {
	return allowSubdivision && max(bounds.Width, bounds.Height) > chartTileSize/2 &&
		(errors.Is(err, ErrChartVisionTruncated) || errors.Is(err, ErrChartVisionOutputInvalid))
}

func stableChartID(namespace, unit, kind string, index int) string {
	root, err := uuid.Parse(namespace)
	if err != nil {
		return uuid.NewSHA1(uuid.NameSpaceURL, []byte(namespace+":"+unit+":"+kind+":"+fmt.Sprint(index))).String()
	}
	return uuid.NewSHA1(root, []byte(unit+":"+kind+":"+fmt.Sprint(index))).String()
}

func chartTileBounds(width, height int) []ChartTileBounds {
	axis := func(size int) []int {
		out := []int{0}
		stride := chartTileSize - chartTileOverlap
		for out[len(out)-1]+chartTileSize < size {
			out = append(out, out[len(out)-1]+stride)
		}
		return out
	}
	var result []ChartTileBounds
	for _, y := range axis(height) {
		for _, x := range axis(width) {
			result = append(result, ChartTileBounds{X: x, Y: y, Width: min(chartTileSize, width-x), Height: min(chartTileSize, height-y)})
		}
	}
	return result
}

func chartTileCount(width, height int) int {
	axisCount := func(size int) int {
		if size <= 0 {
			return 0
		}
		if size <= chartTileSize {
			return 1
		}
		remaining := size - chartTileSize
		stride := chartTileSize - chartTileOverlap
		return 1 + (remaining+stride-1)/stride
	}
	x, y := axisCount(width), axisCount(height)
	if x == 0 || y == 0 || x > 512 || y > 512/x {
		return 513
	}
	return x * y
}

func cropImage(source image.Image, bounds ChartTileBounds) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, bounds.Width, bounds.Height))
	draw.Draw(result, result.Bounds(), source, image.Point{X: bounds.X, Y: bounds.Y}, draw.Src)
	return result
}

func encodePNG(imageData image.Image) ([]byte, error) {
	var output bytes.Buffer
	if err := png.Encode(&output, imageData); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func chartOccurrenceCrops(source image.Image, box ChartBox) ([]byte, []byte, error) {
	bounds := source.Bounds()
	x0 := int(math.Floor(box.X * float64(bounds.Dx())))
	y0 := int(math.Floor(box.Y * float64(bounds.Dy())))
	x1 := int(math.Ceil((box.X + box.Width) * float64(bounds.Dx())))
	y1 := int(math.Ceil((box.Y + box.Height) * float64(bounds.Dy())))
	x0, y0, x1, y1 = max(0, x0), max(0, y0), min(bounds.Dx(), x1), min(bounds.Dy(), y1)
	if x1 <= x0 || y1 <= y0 {
		return nil, nil, ErrValidation
	}
	tightBounds := ChartTileBounds{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
	marginX, marginY := max(16, (x1-x0)*2/3), max(16, (y1-y0)*2/3)
	contextBounds := ChartTileBounds{X: max(0, x0-marginX), Y: max(0, y0-marginY)}
	contextRight, contextBottom := min(bounds.Dx(), x1+marginX), min(bounds.Dy(), y1+marginY)
	contextBounds.Width, contextBounds.Height = contextRight-contextBounds.X, contextBottom-contextBounds.Y
	tight, err := encodePNG(cropImage(source, tightBounds))
	if err != nil {
		return nil, nil, err
	}
	contextPNG, err := encodePNG(cropImage(source, contextBounds))
	if err != nil {
		return nil, nil, err
	}
	if len(tight) > 4<<20 || len(contextPNG) > 4<<20 {
		return nil, nil, ErrValidation
	}
	return tight, contextPNG, nil
}

func (s *Service) makeChartReadBatchesFromPDF(ctx context.Context, runID uuid.UUID, pdfPage *ChartPDFPage, occurrences []ChartNameOccurrence) ([]chartReadBatch, error) {
	rows, err := s.Pool.Query(ctx, `SELECT unit_key, input_geometry FROM source_chart_analysis_units WHERE run_id = $1 AND unit_kind IN ('tile','subtile') AND status = 'succeeded' ORDER BY unit_key`, runID)
	if err != nil {
		return nil, err
	}
	geometries := map[string]chartUnitGeometry{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var geometry chartUnitGeometry
		if err := json.Unmarshal(raw, &geometry); err != nil {
			rows.Close()
			return nil, ErrValidation
		}
		geometries[key] = geometry
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	byTile := map[string][]ChartNameOccurrence{}
	unplaced := []ChartNameOccurrence{}
	for _, occurrence := range occurrences {
		bestKey, bestMargin := "", -1.0
		seen := map[string]struct{}{}
		for _, key := range occurrence.SourceTileIDs {
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			geometry, ok := geometries[key]
			if !ok {
				continue
			}
			margin := chartTileMargin(occurrence.Region, geometry.Bounds, pdfPage.Width, pdfPage.Height)
			if margin > bestMargin {
				bestKey, bestMargin = key, margin
			}
		}
		if bestKey == "" {
			unplaced = append(unplaced, occurrence)
			continue
		}
		byTile[bestKey] = append(byTile[bestKey], occurrence)
	}

	keys := make([]string, 0, len(byTile))
	for key := range byTile {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []chartReadBatch{}
	for _, key := range keys {
		geometry := geometries[key]
		tilePNG, err := pdfPage.RenderRegion(ctx, geometry.Bounds)
		if err != nil {
			for index, occurrence := range byTile[key] {
				result = append(result, chartReadBatch{Key: fmt.Sprintf("read_%s_render_%05d", key, index), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "source_tile_render_failed"})
			}
			continue
		}
		tileImage, err := png.Decode(bytes.NewReader(tilePNG))
		if err != nil || tileImage.Bounds().Dx() != geometry.Bounds.Width || tileImage.Bounds().Dy() != geometry.Bounds.Height {
			for index, occurrence := range byTile[key] {
				result = append(result, chartReadBatch{Key: fmt.Sprintf("read_%s_decode_%05d", key, index), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "source_tile_render_failed"})
			}
			continue
		}
		inputs := make([]ChartCropInput, 0, len(byTile[key]))
		for index, occurrence := range byTile[key] {
			tightBounds, contextBounds, ok := chartOccurrencePixelBounds(occurrence.Region, pdfPage.Width, pdfPage.Height)
			if !ok || !chartBoundsContain(geometry.Bounds, tightBounds) {
				result = append(result, chartReadBatch{Key: fmt.Sprintf("read_%s_crop_%05d", key, index), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "crop_failed"})
				continue
			}
			tightLocal := chartBoundsRelative(geometry.Bounds, tightBounds)
			tightPNG, err := encodePNG(cropImage(tileImage, tightLocal))
			if err != nil || len(tightPNG) > 4<<20 {
				result = append(result, chartReadBatch{Key: fmt.Sprintf("read_%s_tight_%05d", key, index), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "crop_failed"})
				continue
			}
			var contextPNG []byte
			if chartBoundsContain(geometry.Bounds, contextBounds) {
				contextPNG, err = encodePNG(cropImage(tileImage, chartBoundsRelative(geometry.Bounds, contextBounds)))
			} else {
				contextPNG, err = pdfPage.RenderRegion(ctx, contextBounds)
			}
			if err != nil || len(contextPNG) == 0 || len(contextPNG) > 4<<20 {
				result = append(result, chartReadBatch{Key: fmt.Sprintf("read_%s_context_%05d", key, index), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "context_crop_failed"})
				continue
			}
			inputs = append(inputs, ChartCropInput{OccurrenceID: occurrence.ID, TightPNG: tightPNG, ContextPNG: contextPNG})
		}
		result = append(result, makeChartReadBatchesFromInputs(inputs, key)...)
	}
	for index, occurrence := range unplaced {
		result = append(result, chartReadBatch{Key: fmt.Sprintf("read_unplaced_%05d", index), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "source_occurrence_unavailable"})
	}
	return result, nil
}

func chartTileMargin(box ChartBox, tile ChartTileBounds, pageWidth, pageHeight int) float64 {
	x0, y0 := box.X*float64(pageWidth), box.Y*float64(pageHeight)
	x1, y1 := (box.X+box.Width)*float64(pageWidth), (box.Y+box.Height)*float64(pageHeight)
	if x0 < float64(tile.X)-1 || y0 < float64(tile.Y)-1 || x1 > float64(tile.X+tile.Width)+1 || y1 > float64(tile.Y+tile.Height)+1 {
		return -1
	}
	return math.Min(math.Min(x0-float64(tile.X), float64(tile.X+tile.Width)-x1), math.Min(y0-float64(tile.Y), float64(tile.Y+tile.Height)-y1))
}

func chartOccurrencePixelBounds(box ChartBox, pageWidth, pageHeight int) (ChartTileBounds, ChartTileBounds, bool) {
	if !validChartBox(box) || pageWidth <= 0 || pageHeight <= 0 {
		return ChartTileBounds{}, ChartTileBounds{}, false
	}
	x0 := max(0, int(math.Floor(box.X*float64(pageWidth))))
	y0 := max(0, int(math.Floor(box.Y*float64(pageHeight))))
	x1 := min(pageWidth, int(math.Ceil((box.X+box.Width)*float64(pageWidth))))
	y1 := min(pageHeight, int(math.Ceil((box.Y+box.Height)*float64(pageHeight))))
	if x1 <= x0 || y1 <= y0 {
		return ChartTileBounds{}, ChartTileBounds{}, false
	}
	tight := ChartTileBounds{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
	marginX, marginY := max(16, tight.Width*2/3), max(16, tight.Height*2/3)
	contextX, contextY := max(0, x0-marginX), max(0, y0-marginY)
	contextRight, contextBottom := min(pageWidth, x1+marginX), min(pageHeight, y1+marginY)
	context := ChartTileBounds{X: contextX, Y: contextY, Width: contextRight - contextX, Height: contextBottom - contextY}
	return tight, context, context.Width > 0 && context.Height > 0 && int64(context.Width)*int64(context.Height) <= maxChartRegionPixels
}

func chartBoundsContain(outer, inner ChartTileBounds) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y && inner.X+inner.Width <= outer.X+outer.Width && inner.Y+inner.Height <= outer.Y+outer.Height
}

func chartBoundsRelative(outer, inner ChartTileBounds) ChartTileBounds {
	return ChartTileBounds{X: inner.X - outer.X, Y: inner.Y - outer.Y, Width: inner.Width, Height: inner.Height}
}

func makeChartReadBatchesFromInputs(inputs []ChartCropInput, prefix string) []chartReadBatch {
	batches := []chartReadBatch{}
	current := chartReadBatch{}
	flush := func() {
		if len(current.Inputs) == 0 {
			return
		}
		current.Key = fmt.Sprintf("read_%s_%05d", prefix, len(batches))
		batches = append(batches, current)
		current = chartReadBatch{}
	}
	for _, input := range inputs {
		inputBytes := int64(len(input.TightPNG) + len(input.ContextPNG))
		if inputBytes > chartMaxReadBatchBytes {
			flush()
			batches = append(batches, chartReadBatch{Key: fmt.Sprintf("read_%s_large_%05d", prefix, len(batches)), OccurrenceIDs: []string{input.OccurrenceID}, ErrorCode: "request_image_too_large"})
			continue
		}
		if len(current.Inputs) >= chartMaxReadBatchImages || current.InputBytes+inputBytes > chartMaxReadBatchBytes {
			flush()
		}
		current.Inputs = append(current.Inputs, input)
		current.OccurrenceIDs = append(current.OccurrenceIDs, input.OccurrenceID)
		current.InputBytes += inputBytes
	}
	flush()
	return batches
}

type chartReadBatch struct {
	Key           string
	OccurrenceIDs []string
	Inputs        []ChartCropInput
	InputBytes    int64
	ErrorCode     string
}

func makeChartReadBatches(page image.Image, occurrences []ChartNameOccurrence) []chartReadBatch {
	batches := []chartReadBatch{}
	current := chartReadBatch{}
	flush := func() {
		if len(current.OccurrenceIDs) == 0 {
			return
		}
		current.Key = fmt.Sprintf("read_%05d", len(batches))
		batches = append(batches, current)
		current = chartReadBatch{}
	}
	for _, occurrence := range occurrences {
		tight, contextCrop, err := chartOccurrenceCrops(page, occurrence.Region)
		if err != nil {
			flush()
			batches = append(batches, chartReadBatch{
				Key: fmt.Sprintf("read_%05d", len(batches)), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "crop_failed",
			})
			continue
		}
		input := ChartCropInput{OccurrenceID: occurrence.ID, TightPNG: tight, ContextPNG: contextCrop}
		inputBytes := int64(len(tight) + len(contextCrop))
		if inputBytes > chartMaxReadBatchBytes {
			flush()
			batches = append(batches, chartReadBatch{
				Key: fmt.Sprintf("read_%05d", len(batches)), OccurrenceIDs: []string{occurrence.ID}, ErrorCode: "request_image_too_large",
			})
			continue
		}
		if len(current.Inputs) >= chartMaxReadBatchImages || current.InputBytes+inputBytes > chartMaxReadBatchBytes {
			flush()
		}
		current.Inputs = append(current.Inputs, input)
		current.OccurrenceIDs = append(current.OccurrenceIDs, occurrence.ID)
		current.InputBytes += inputBytes
	}
	flush()
	return batches
}

func boxOverlapSmaller(a, b ChartBox) float64 {
	left, top := math.Max(a.X, b.X), math.Max(a.Y, b.Y)
	right, bottom := math.Min(a.X+a.Width, b.X+b.Width), math.Min(a.Y+a.Height, b.Y+b.Height)
	if right <= left || bottom <= top {
		return 0
	}
	intersection := (right - left) * (bottom - top)
	smaller := math.Min(a.Width*a.Height, b.Width*b.Height)
	if smaller <= 0 {
		return 0
	}
	return intersection / smaller
}

func branchGeometryOverlap(a, b []ChartPoint) float64 {
	return boxOverlapSmaller(polygonBounds(a), polygonBounds(b))
}
func polygonBounds(points []ChartPoint) ChartBox {
	minX, minY, maxX, maxY := 1.0, 1.0, 0.0, 0.0
	for _, p := range points {
		minX = math.Min(minX, p[0])
		minY = math.Min(minY, p[1])
		maxX = math.Max(maxX, p[0])
		maxY = math.Max(maxY, p[1])
	}
	return ChartBox{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

func mustUUID(value string) uuid.UUID { parsed, _ := uuid.Parse(value); return parsed }
