package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const SourceNameReadingJobType = "source_name_reading"

var errNameRunCancelled = errors.New("name reading run is cancelled")

type NameReadingConfig struct {
	ReaderAlgorithm   string                 `json:"readerAlgorithm"`
	ArabicDataVersion string                 `json:"arabicDataVersion"`
	Renderer          string                 `json:"rendererVersion"`
	Geometry          string                 `json:"geometryVersion"`
	Localiser         string                 `json:"localiserVersion"`
	Preprocessing     string                 `json:"preprocessingVersion"`
	Reader            string                 `json:"readerVersion"`
	Engine            string                 `json:"engineVersion"`
	Mode              string                 `json:"readerMode"`
	Privacy           string                 `json:"privacyPolicy"`
	CropBudgetMS      int64                  `json:"cropBudgetMs"`
	LocaliserSettings CarrierLocaliserConfig `json:"localiserSettings"`
}
type NameReadingRun struct {
	ID                  string             `json:"id"`
	FileID              string             `json:"sourceFileId"`
	Status              string             `json:"status"`
	Stage               string             `json:"stage"`
	Config              NameReadingConfig  `json:"config"`
	Pages               int                `json:"pages"`
	Detected            int                `json:"detected"`
	Named               int                `json:"named"`
	Unreadable          int                `json:"unreadable"`
	Unavailable         int                `json:"unavailable"`
	SourcePixelWarnings int                `json:"sourcePixelWarnings"`
	ManualReview        int                `json:"manualReview"`
	CompletedUnits      int                `json:"completedUnits"`
	FailedUnits         int                `json:"failedUnits"`
	TotalUnits          int                `json:"totalUnits"`
	CacheHits           int                `json:"cacheHits"`
	Timings             map[string]int64   `json:"timings"`
	ErrorCode           string             `json:"errorCode,omitempty"`
	UpdatedAt           time.Time          `json:"updatedAt"`
	JobID               string             `json:"-"`
	Checksum            string             `json:"-"`
	RequestedBy         string             `json:"-"`
	SuggestionID        string             `json:"-"`
	Generation          int                `json:"-"`
	TargetGeometry      *NameReadingTarget `json:"-"`
}

type NameReadingTarget struct {
	Page             int    `json:"page"`
	Region           Region `json:"region"`
	GeometryRevision int    `json:"geometryRevision"`
}
type nameReadingPayload struct {
	RunID      string `json:"runId"`
	FileID     string `json:"source_file_id"`
	SourceID   string `json:"source_id"`
	Generation int    `json:"generation"`
}
type nameReadingWork struct {
	Claim      Claim
	RunID      string
	Generation int
	content    []byte
	page       *ChartPDFPage
}
type nameUnitPayload struct {
	Tile    nameReadingTile `json:"tile"`
	Regions []PageRegion    `json:"regions,omitempty"`
	Region  Region          `json:"region,omitempty"`
	Outcome string          `json:"outcome,omitempty"`
}
type nameUnit struct {
	ID, Key, Stage, Status, FrameID, SuggestionID string
	Payload                                       nameUnitPayload
	Attempts                                      int
}

func (s *Service) nameReadingConfig(ctx context.Context) NameReadingConfig {
	status := s.ReadingConfiguration(ctx)
	engine := "unavailable"
	for _, tool := range status.LocalReader.Tools {
		if tool.Name == "tesseract" && tool.Available {
			engine = tool.Version
		}
	}
	mode := "off"
	if s.readingSettings().Mode == "local" {
		mode = "local"
	}
	reader := NewLocalCropReader()
	budget := reader.Budget.Milliseconds()
	if configured, ok := s.LocalCropReader.(*TesseractCropReader); ok && configured.Budget > 0 {
		budget = configured.Budget.Milliseconds()
	}
	config := NameReadingConfig{ReaderAlgorithm: LocalCropReaderVersion, ArabicDataVersion: status.LocalReader.ArabicDataVersion, Renderer: NameRendererVersion, Geometry: NameGeometryVersion, Localiser: carrierReadingModel, Preprocessing: NamePreprocessingVersion, Reader: LocalCropReaderVersion, Engine: engine, Mode: mode, Privacy: "local_only", CropBudgetMS: budget, LocaliserSettings: DefaultCarrierLocaliserConfig()}
	config.Reader = fullNameReaderVersion(config)
	return config
}
func fullNameReaderVersion(config NameReadingConfig) string {
	config.Reader = LocalCropReaderVersion
	return LocalCropReaderVersion + "+" + hashNameValue(config)[:16]
}
func (s *Service) PrepareNameReading(ctx context.Context, fileID, actorID string) (NameReadingRun, error) {
	return s.enqueueNameReading(ctx, fileID, actorID, "")
}
func (s *Service) RereadSuggestionLocally(ctx context.Context, suggestionID, actorID string) (NameReadingRun, error) {
	source, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false)
	if err != nil {
		return NameReadingRun{}, err
	}
	if s.readingSettings().Mode != "local" {
		return NameReadingRun{}, ErrCarrierReaderUnavailable
	}
	return s.enqueueNameReading(ctx, source.sourceFileID.String(), actorID, suggestionID)
}
func (s *Service) enqueueNameReading(ctx context.Context, fileID, actorID, suggestionID string) (NameReadingRun, error) {
	if s == nil || s.Pool == nil {
		return NameReadingRun{}, ErrDatabaseUnavailable
	}
	if s.Jobs == nil {
		return NameReadingRun{}, ErrQueueUnavailable
	}
	file, actor, err := parseIDs(fileID, actorID)
	if err != nil {
		return NameReadingRun{}, err
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return NameReadingRun{}, err
	}
	if !allowed {
		return NameReadingRun{}, ErrForbidden
	}
	config := s.nameReadingConfig(ctx)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return NameReadingRun{}, err
	}
	defer tx.Rollback(ctx)
	var source uuid.UUID
	var checksum, mime string
	if err = tx.QueryRow(ctx, `SELECT source_id,checksum_sha256,mime_type FROM source_files WHERE id=$1 FOR UPDATE`, file).Scan(&source, &checksum, &mime); err != nil {
		return NameReadingRun{}, ErrNotFound
	}
	if !isImageContentType(mime) && normalizeContentType(mime) != "application/pdf" {
		return NameReadingRun{}, ErrUnsupportedDocument
	}
	geometry := ""
	var targetJSON []byte
	if suggestionID != "" {
		var region []byte
		var targetFile uuid.UUID
		var target NameReadingTarget
		err = tx.QueryRow(ctx, `SELECT source_file_id,region,page_number,geometry_revision FROM source_tree_suggestions WHERE id=$1`, suggestionID).Scan(&targetFile, &region, &target.Page, &target.GeometryRevision)
		if err != nil || targetFile != file {
			return NameReadingRun{}, ErrNotFound
		}
		if json.Unmarshal(region, &target.Region) != nil || validateNormalizedRegion(target.Region) != nil {
			return NameReadingRun{}, ErrValidation
		}
		targetJSON = mustJSON(target)
		geometry = hashNameValue(target)
	}
	fingerprint := hashNameValue(struct {
		Checksum, Target, Geometry string
		Config                     NameReadingConfig
	}{checksum, suggestionID, geometry, config})
	var existing string
	err = tx.QueryRow(ctx, `SELECT id::text FROM source_name_reading_runs WHERE source_file_id=$1 AND config_hash=$2 ORDER BY created_at DESC LIMIT 1`, file, fingerprint).Scan(&existing)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return NameReadingRun{}, err
		}
		return s.nameReadingRun(ctx, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return NameReadingRun{}, err
	}
	id := uuid.NewString()
	encoded, _ := json.Marshal(config)
	_, err = tx.Exec(ctx, `INSERT INTO source_name_reading_runs(id,source_file_id,source_id,requested_by,source_checksum,config_hash,config,suggestion_id,target_geometry) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9)`, id, file, source, actor, checksum, fingerprint, encoded, suggestionID, targetJSON)
	if err != nil {
		return NameReadingRun{}, err
	}
	if err = s.enqueueNameJob(ctx, tx, id, 1); err != nil {
		return NameReadingRun{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return NameReadingRun{}, err
	}
	return s.nameReadingRun(ctx, id)
}
func (s *Service) enqueueNameJob(ctx context.Context, tx pgx.Tx, id string, generation int) error {
	transactional, ok := s.Jobs.(TransactionalJobEnqueuer)
	if !ok {
		return ErrQueueUnavailable
	}
	var fileID, sourceID string
	if err := tx.QueryRow(ctx, `SELECT source_file_id::text,source_id::text FROM source_name_reading_runs WHERE id=$1`, id).Scan(&fileID, &sourceID); err != nil {
		return err
	}
	payload, _ := json.Marshal(nameReadingPayload{RunID: id, Generation: generation, FileID: fileID, SourceID: sourceID})
	enqueued, err := transactional.EnqueueTx(ctx, tx, jobs.EnqueueInput{Type: SourceNameReadingJobType, Payload: payload, MaxAttempts: 3, IdempotencyKey: fmt.Sprintf("name-reading:%s:%d", id, generation)})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE source_name_reading_runs SET job_id=$2,status='queued',cancel_requested=false,error_code='',updated_at=now() WHERE id=$1`, id, enqueued.Job.ID)
	return err
}
func (s *Service) NameReadingRun(ctx context.Context, id, actorID string) (NameReadingRun, error) {
	if s == nil || s.Pool == nil {
		return NameReadingRun{}, ErrDatabaseUnavailable
	}
	if _, err := uuid.Parse(id); err != nil {
		return NameReadingRun{}, ErrNotFound
	}
	run, err := s.nameReadingRun(ctx, id)
	if err != nil {
		return run, err
	}
	allowed, err := s.canReview(ctx, uuid.MustParse(run.FileID), actorID)
	if err != nil {
		return NameReadingRun{}, err
	}
	if !allowed {
		return NameReadingRun{}, ErrForbidden
	}
	return run, nil
}
func (s *Service) LatestNameReadingRun(ctx context.Context, fileID, actorID string) (NameReadingRun, error) {
	if s == nil || s.Pool == nil {
		return NameReadingRun{}, ErrDatabaseUnavailable
	}
	file, err := uuid.Parse(fileID)
	if err != nil {
		return NameReadingRun{}, ErrNotFound
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return NameReadingRun{}, err
	}
	if !allowed {
		return NameReadingRun{}, ErrForbidden
	}
	var id string
	err = s.Pool.QueryRow(ctx, `SELECT id::text FROM source_name_reading_runs WHERE source_file_id=$1 AND suggestion_id IS NULL ORDER BY created_at DESC LIMIT 1`, file).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return NameReadingRun{}, ErrNotFound
	}
	if err != nil {
		return NameReadingRun{}, err
	}
	return s.nameReadingRun(ctx, id)
}
func (s *Service) nameReadingRun(ctx context.Context, id string) (NameReadingRun, error) {
	var run NameReadingRun
	var config, timings, target []byte
	err := s.Pool.QueryRow(ctx, `SELECT id::text,source_file_id::text,status,stage,config,page_count,detected,named,unreadable,unavailable,cache_hits,timings,error_code,updated_at,COALESCE(job_id::text,''),source_checksum,COALESCE(requested_by::text,''),COALESCE(suggestion_id::text,''),generation,
 (SELECT count(*) FROM source_name_reading_units WHERE run_id=r.id),
 (SELECT count(*) FROM source_name_reading_units WHERE run_id=r.id AND status='succeeded'),
 (SELECT count(*) FROM source_name_reading_units WHERE run_id=r.id AND status='failed'),
 (SELECT count(*) FROM source_name_reading_units u WHERE u.run_id=r.id AND u.payload->>'outcome'='manual_review'),
 (SELECT count(*) FROM source_name_reading_units u JOIN source_name_page_frames f ON f.id=u.frame_id WHERE u.run_id=r.id AND u.stage='read' AND COALESCE((u.payload->'region'->>'height')::double precision,0)*f.height_px<18),target_geometry
 FROM source_name_reading_runs r WHERE id=$1`, id).Scan(&run.ID, &run.FileID, &run.Status, &run.Stage, &config, &run.Pages, &run.Detected, &run.Named, &run.Unreadable, &run.Unavailable, &run.CacheHits, &timings, &run.ErrorCode, &run.UpdatedAt, &run.JobID, &run.Checksum, &run.RequestedBy, &run.SuggestionID, &run.Generation, &run.TotalUnits, &run.CompletedUnits, &run.FailedUnits, &run.ManualReview, &run.SourcePixelWarnings, &target)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, ErrNotFound
	}
	if err != nil {
		return run, err
	}
	if json.Unmarshal(config, &run.Config) != nil || json.Unmarshal(timings, &run.Timings) != nil {
		return run, ErrValidation
	}
	if len(target) != 0 && json.Unmarshal(target, &run.TargetGeometry) != nil {
		return run, ErrValidation
	}
	return run, nil
}
func (s *Service) CancelNameReading(ctx context.Context, id, actorID string) (NameReadingRun, error) {
	return s.controlNameReading(ctx, id, actorID, false)
}
func (s *Service) ResumeNameReading(ctx context.Context, id, actorID string) (NameReadingRun, error) {
	return s.controlNameReading(ctx, id, actorID, true)
}
func (s *Service) controlNameReading(ctx context.Context, id, actorID string, resume bool) (NameReadingRun, error) {
	run, err := s.NameReadingRun(ctx, id, actorID)
	if err != nil {
		return run, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return run, err
	}
	defer tx.Rollback(ctx)
	// Match worker lock order: job, then run. Cancellation revokes the job token.
	if _, err = tx.Exec(ctx, `SELECT id FROM jobs WHERE id=$1 AND type=$2 FOR UPDATE`, run.JobID, SourceNameReadingJobType); err != nil {
		return run, err
	}
	var status, jobID string
	var generation int
	if err = tx.QueryRow(ctx, `SELECT status,COALESCE(job_id::text,''),generation FROM source_name_reading_runs WHERE id=$1 FOR UPDATE`, id).Scan(&status, &jobID, &generation); err != nil {
		return run, err
	}
	if jobID != run.JobID {
		return run, ErrConflict
	}
	if resume {
		if status == "queued" || status == "running" || status == "succeeded" {
			return run, nil
		}
		if hashNameValue(run.Config) != hashNameValue(s.nameReadingConfig(ctx)) {
			return run, ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE source_name_reading_runs SET generation=generation+1 WHERE id=$1`, id); err != nil {
			return run, err
		}
		if err = s.enqueueNameJob(ctx, tx, id, generation+1); err != nil {
			return run, err
		}
	} else {
		if status != "queued" && status != "running" {
			return run, nil
		}
		_, err = tx.Exec(ctx, `UPDATE jobs SET status='failed',last_error='name_reading_cancelled',locked_by=NULL,locked_at=NULL,lease_token=NULL,lease_expires_at=NULL,heartbeat_at=NULL,updated_at=now() WHERE id=$1 AND type=$2 AND status IN ('queued','running')`, jobID, SourceNameReadingJobType)
		if err != nil {
			return run, err
		}
		_, err = tx.Exec(ctx, `UPDATE source_name_reading_runs SET cancel_requested=true,status='cancelled',updated_at=now() WHERE id=$1`, id)
		if err != nil {
			return run, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return run, err
	}
	return s.nameReadingRun(ctx, id)
}

// An explicit clock check is needed even in a transaction that held a render
// lock for a while: PostgreSQL now() would still mean transaction start time.
func (s *Service) nameWriteFence(ctx context.Context, tx pgx.Tx, work *nameReadingWork) error {
	if work == nil {
		return nil
	}
	lease := work.Claim.Lease
	if !lease.Valid() || s.Jobs == nil {
		return ErrLeaseRequired
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT status='running' AND type=$4 AND locked_by=$2 AND lease_token::text=$3 AND lease_expires_at>clock_timestamp() FROM jobs WHERE id=$1 FOR UPDATE`, lease.JobID, lease.WorkerID, lease.Token, SourceNameReadingJobType).Scan(&valid)
	if err != nil || !valid {
		return jobs.ErrLeaseLost
	}
	if err = s.Jobs.HoldLease(ctx, tx, lease); err != nil {
		return err
	}
	var cancelled bool
	var jobID string
	var generation int
	var status string
	err = tx.QueryRow(ctx, `SELECT cancel_requested,COALESCE(job_id::text,''),generation,status FROM source_name_reading_runs WHERE id=$1 FOR UPDATE`, work.RunID).Scan(&cancelled, &jobID, &generation, &status)
	if err != nil {
		return err
	}
	if jobID != work.Claim.Job.ID || generation != work.Generation {
		return jobs.ErrLeaseLost
	}
	if cancelled || status == "cancelled" {
		return errNameRunCancelled
	}
	if status == "succeeded" {
		return ErrConflict
	}
	return nil
}
func (s *Service) nameTransaction(ctx context.Context, work *nameReadingWork, write func(pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.nameWriteFence(ctx, tx, work); err != nil {
		return err
	}
	if err = write(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) RequeueNameReadingJobs(ctx context.Context, recovered []jobs.JobView) error {
	for _, job := range recovered {
		if job.Type != SourceNameReadingJobType {
			continue
		}
		status := "queued"
		code := ""
		if job.Status == "dead" {
			status = "failed"
			code = "worker_attempts_exhausted"
		}
		_, err := s.Pool.Exec(ctx, `UPDATE source_name_reading_runs SET status=$2,error_code=$3,updated_at=now() WHERE job_id=$1 AND NOT cancel_requested AND status<>'succeeded'`, job.ID, status, code)
		if err != nil {
			return err
		}
	}
	return nil
}
func nameRunErrorCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "run_budget_exhausted"
	case errors.Is(err, ErrValidation):
		return "invalid_source_or_frame"
	case errors.Is(err, ErrStorageUnavailable):
		return "storage_unavailable"
	case errors.Is(err, ErrPDFToolsUnavailable):
		return "render_tools_unavailable"
	default:
		return "local_preparation_failed"
	}
}
func nameJobPayload(job jobs.JobView) (nameReadingPayload, error) {
	var payload nameReadingPayload
	err := json.Unmarshal(job.Payload, &payload)
	if err != nil || strings.TrimSpace(payload.RunID) == "" || payload.Generation < 1 {
		return payload, ErrValidation
	}
	if _, err := uuid.Parse(payload.RunID); err != nil {
		return payload, ErrValidation
	}
	return payload, nil
}

func (s *Service) LatestSuggestionNameReadingRun(ctx context.Context, suggestionID, actorID string) (NameReadingRun, error) {
	if _, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false); err != nil {
		return NameReadingRun{}, err
	}
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT id::text FROM source_name_reading_runs WHERE suggestion_id=$1 ORDER BY created_at DESC LIMIT 1`, suggestionID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return NameReadingRun{}, ErrNotFound
	}
	if err != nil {
		return NameReadingRun{}, err
	}
	return s.nameReadingRun(ctx, id)
}
