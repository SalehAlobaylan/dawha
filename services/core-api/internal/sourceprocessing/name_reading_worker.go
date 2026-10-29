package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxNameRunRegions = 20000
const maxNameRunTiles = 4096

func (s *Service) nameWorkPage(ctx context.Context, source suggestionSource, frame *NamePageFrame, work *nameReadingWork) (*ChartPDFPage, error) {
	if work.page != nil && work.page.PageNumber == source.pageNumber {
		return work.page, nil
	}
	if work.page != nil {
		work.page.Close()
		work.page = nil
	}
	var err error
	if work.content == nil {
		work.content, err = s.readNameOriginal(ctx, source)
		if err != nil {
			return nil, err
		}
	}
	source.content = work.content
	work.page, err = s.openNameSource(ctx, source, frame)
	return work.page, err
}
func (s *Service) nameSource(ctx context.Context, run NameReadingRun, page int) (suggestionSource, error) {
	var source suggestionSource
	var checksum string
	source.sourceFileID = uuid.MustParse(run.FileID)
	source.pageNumber = page
	err := s.Pool.QueryRow(ctx, `SELECT storage_key,mime_type,checksum_sha256 FROM source_files WHERE id=$1`, run.FileID).Scan(&source.storageKey, &source.mimeType, &checksum)
	if err != nil {
		return source, err
	}
	if checksum != run.Checksum {
		return source, ErrValidation
	}
	source.checksum = checksum
	extractor, ok := s.Extractor.(*PDFExtractor)
	if !ok || extractor == nil {
		extractor = NewPDFExtractor()
	}
	source.extractor = extractor
	return source, nil
}
func (s *Service) ensureNameFrame(ctx context.Context, source suggestionSource, work *nameReadingWork) (NamePageFrame, error) {
	frame, err := loadNameFrame(ctx, s.Pool, source)
	if err == nil {
		return frame, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return frame, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return frame, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,24))`, source.sourceFileID.String()+source.checksum+fmt.Sprint(source.pageNumber)); err != nil {
		return frame, err
	}
	frame, err = loadNameFrame(ctx, tx, source)
	if err == nil {
		return frame, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return frame, err
	}
	var page *ChartPDFPage
	if work != nil {
		page, err = s.nameWorkPage(ctx, source, nil, work)
	} else {
		page, err = s.openNameSource(ctx, source, nil)
		if err == nil {
			defer page.Close()
		}
	}
	if err != nil {
		return frame, err
	}
	frame = frameFromPage(source, page)
	if err = s.nameWriteFence(ctx, tx, work); err != nil {
		return frame, err
	}
	frame, err = saveNameFrame(ctx, tx, frame)
	if err != nil {
		return frame, err
	}
	return frame, tx.Commit(ctx)
}

func (s *Service) ProcessNameReading(parent context.Context, claim Claim) (returned error) {
	if s == nil || s.Pool == nil || s.Store == nil {
		return ErrDatabaseUnavailable
	}
	if claim.Job.Type != SourceNameReadingJobType {
		return ErrValidation
	}
	payload, err := nameJobPayload(claim.Job)
	if err != nil {
		return err
	}
	heartbeat, ctx, err := s.startHeartbeat(parent, claim)
	if err != nil {
		return err
	}
	defer func() {
		if heartbeat.Lost() != nil {
			returned = jobs.ErrLeaseLost
		}
		heartbeat.Stop()
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	work := &nameReadingWork{Claim: claim, RunID: payload.RunID, Generation: payload.Generation}
	defer func() {
		if work.page != nil {
			work.page.Close()
		}
	}()
	run, err := s.nameReadingRun(ctx, work.RunID)
	if err != nil {
		return err
	}
	if run.Status == "succeeded" || run.Status == "cancelled" || (run.Status == "partial" && run.Stage == "complete") {
		return nil
	}
	if run.Config.Renderer != NameRendererVersion || run.Config.Geometry != NameGeometryVersion || run.Config.ReaderAlgorithm != LocalCropReaderVersion || run.Config.Reader != fullNameReaderVersion(run.Config) || run.Config.Privacy != "local_only" || run.Config.Preprocessing != NamePreprocessingVersion {
		return ErrValidation
	}
	defer func() {
		if returned != nil && !errors.Is(returned, jobs.ErrLeaseLost) && !errors.Is(returned, errNameRunCancelled) && parent.Err() == nil {
			saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer stop()
			_ = s.nameTransaction(saveCtx, work, func(tx pgx.Tx) error {
				_, err := tx.Exec(saveCtx, `UPDATE source_name_reading_runs SET status=CASE WHEN detected>0 THEN 'partial' ELSE 'failed' END,error_code=$2,updated_at=now() WHERE id=$1`, run.ID, nameRunErrorCode(returned))
				return err
			})
		}
	}()
	if err = s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE source_name_reading_runs SET status='running',stage='preparing',error_code='',updated_at=now() WHERE id=$1`, run.ID)
		return err
	}); err != nil {
		return err
	}
	source, err := s.nameSource(ctx, run, 1)
	if err != nil {
		return err
	}
	if run.SuggestionID != "" {
		// Legacy runs without a snapshot cannot safely reconstruct the requested
		// rectangle. A new reread gets a distinct fingerprint and frozen target.
		if run.TargetGeometry == nil || run.TargetGeometry.Page < 1 || run.TargetGeometry.Page > MaxPages || run.TargetGeometry.GeometryRevision < 0 || validateNormalizedRegion(run.TargetGeometry.Region) != nil {
			return ErrValidation
		}
		source.pageNumber = run.TargetGeometry.Page
		source.region = run.TargetGeometry.Region
		frame, err := s.ensureNameFrame(ctx, source, work)
		if err != nil {
			return err
		}
		err = s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
			payload, _ := json.Marshal(nameUnitPayload{Region: source.region})
			_, err := tx.Exec(ctx, `INSERT INTO source_name_reading_units(run_id,unit_key,stage,frame_id,suggestion_id,payload) VALUES($1,$2,'read',$3,$4,$5) ON CONFLICT(run_id,stage,unit_key) DO NOTHING`, run.ID, "reread:"+run.SuggestionID, frame.ID, run.SuggestionID, payload)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE source_name_reading_runs SET page_count=1,detected=1 WHERE id=$1`, run.ID)
			return err
		})
		if err != nil {
			return err
		}
	} else {
		frame, err := s.ensureNameFrame(ctx, source, work)
		if err != nil {
			return err
		}
		for pageNumber := 1; pageNumber <= frame.PageCount; pageNumber++ {
			if err = ctx.Err(); err != nil {
				return err
			}
			source.pageNumber = pageNumber
			pageFrame, err := s.ensureNameFrame(ctx, source, work)
			if err != nil {
				return err
			}
			tiles := planNameReadingTiles(pageFrame.Width, pageFrame.Height)
			if len(tiles) == 0 || len(tiles) > maxNameRunTiles {
				return ErrValidation
			}
			if err = s.planNameDetection(ctx, work, pageFrame, tiles); err != nil {
				return err
			}
			for index, tile := range tiles {
				key := fmt.Sprintf("p%04d-t%05d", pageNumber, index)
				unit, err := s.nameUnit(ctx, run.ID, "detect", key)
				if err != nil {
					return err
				}
				if unit.Status == "succeeded" {
					continue
				}
				started := time.Now()
				regions, cached, detectErr := s.reuseOrDetectNameTile(ctx, run, unit, source, pageFrame, tile, len(tiles) > 1, work)
				if detectErr != nil {
					_ = s.failNameUnit(ctx, work, unit.ID, nameRunErrorCode(detectErr), time.Since(started))
					return detectErr
				}
				if err = s.persistNameDetection(ctx, work, run, unit, pageFrame, regions, time.Since(started), cached); err != nil {
					return err
				}
			}
		}
	}
	if work.page != nil {
		work.page.Close()
		work.page = nil
	}
	if err = s.publishNameLocalisation(ctx, work, run); err != nil {
		return err
	}
	if err = s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE source_name_reading_runs SET stage='reading',updated_at=now() WHERE id=$1`, run.ID)
		return err
	}); err != nil {
		return err
	}
	units, err := s.pendingNameUnits(ctx, run.ID)
	if err != nil {
		return err
	}
	currentConfig := s.nameReadingConfig(ctx)
	currentCapabilities := s.ReadingConfiguration(ctx)
	canRead := run.Config.Mode == "local" && currentConfig.Reader == run.Config.Reader && currentCapabilities.LocalReader.Ready
	reader := s.LocalCropReader
	if reader == nil {
		local := NewLocalCropReader()
		local.Budget = time.Duration(run.Config.CropBudgetMS) * time.Millisecond
		reader = local
	}
	for _, unit := range units {
		if err = ctx.Err(); err != nil {
			return err
		}
		frame, err := s.frameByID(ctx, unit.FrameID)
		if err != nil {
			return err
		}
		if !nameFrameMatchesRun(run, frame) {
			return ErrValidation
		}
		source, err = s.nameSource(ctx, run, frame.Page)
		if err != nil {
			return err
		}
		source.region = unit.Payload.Region
		started := time.Now()
		quality := CropObservations{NameHeight: int(source.region.Height*float64(frame.Height) + .5), Clipped: source.region.X <= 0 || source.region.Y <= 0 || source.region.X+source.region.Width >= 1 || source.region.Y+source.region.Height >= 1}
		artifact := preparedNameCrop{Frame: frame, Fingerprint: artifactFingerprint(source, frame), Quality: quality}
		reading := LocalCropReading{Outcome: "reading_unavailable", Alternatives: []string{}, Variants: []LocalVariantReading{}, Quality: quality}
		if canRead {
			var cropErr error
			artifact, cropErr = s.prepareNameCrop(ctx, source, nil, work)
			if cropErr != nil {
				_ = s.failNameUnit(ctx, work, unit.ID, nameRunErrorCode(cropErr), time.Since(started))
				return cropErr
			}
			reading, err = reader.ReadCrop(ctx, artifact.Crop.TightPNG, artifact.Quality)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				reading.Outcome = "reading_unavailable"
				reading.Quality.Reason = nameRunErrorCode(err)
			}
		} else {
			reading.Quality.Reason = "reader_unavailable"
			if run.Config.Mode != "local" || s.readingSettings().Mode != "local" {
				reading.Quality.Reason = "disabled_by_policy"
			} else if currentConfig.Reader != run.Config.Reader {
				reading.Quality.Reason = "reader_configuration_changed"
			} else {
				reading.Quality.Reason = currentCapabilities.LocalReader.Reason
			}
		}

		if err = s.persistLocalNameReading(ctx, work, run, unit, artifact, reading, time.Since(started)); err != nil {
			return err
		}
	}
	return s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE source_name_reading_runs SET status=CASE WHEN EXISTS(SELECT 1 FROM source_name_reading_units WHERE run_id=$1 AND status<>'succeeded') THEN 'partial' ELSE 'succeeded' END,stage='complete',updated_at=now() WHERE id=$1`, run.ID)
		return err
	})
}
func (s *Service) frameByID(ctx context.Context, id string) (NamePageFrame, error) {
	var f NamePageFrame
	err := s.Pool.QueryRow(ctx, `SELECT id::text,source_file_id::text,source_checksum,page_number,coordinate_space,analysis_dpi,width_px,height_px,page_count,renderer_version FROM source_name_page_frames WHERE id=$1`, id).Scan(&f.ID, &f.FileID, &f.Checksum, &f.Page, &f.Space, &f.DPI, &f.Width, &f.Height, &f.PageCount, &f.Renderer)
	if err == nil && !f.valid() {
		err = ErrValidation
	}
	return f, err
}
func (s *Service) nameUnit(ctx context.Context, runID, stage, key string) (nameUnit, error) {
	var unit nameUnit
	var payload []byte
	err := s.Pool.QueryRow(ctx, `SELECT id::text,unit_key,stage,status,frame_id::text,COALESCE(suggestion_id::text,''),payload,attempts FROM source_name_reading_units WHERE run_id=$1 AND stage=$2 AND unit_key=$3`, runID, stage, key).Scan(&unit.ID, &unit.Key, &unit.Stage, &unit.Status, &unit.FrameID, &unit.SuggestionID, &payload, &unit.Attempts)
	if err == nil {
		err = json.Unmarshal(payload, &unit.Payload)
	}
	return unit, err
}
func (s *Service) pendingNameUnits(ctx context.Context, runID string) ([]nameUnit, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text,unit_key,stage,status,frame_id::text,COALESCE(suggestion_id::text,''),payload,attempts FROM source_name_reading_units WHERE run_id=$1 AND stage='read' AND status<>'succeeded' ORDER BY frame_id,unit_key`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []nameUnit{}
	for rows.Next() {
		var unit nameUnit
		var payload []byte
		if err := rows.Scan(&unit.ID, &unit.Key, &unit.Stage, &unit.Status, &unit.FrameID, &unit.SuggestionID, &payload, &unit.Attempts); err != nil {
			return nil, err
		}
		if json.Unmarshal(payload, &unit.Payload) != nil {
			return nil, ErrValidation
		}
		result = append(result, unit)
	}
	return result, rows.Err()
}
func (s *Service) planNameDetection(ctx context.Context, work *nameReadingWork, frame NamePageFrame, tiles []nameReadingTile) error {
	return s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_name_reading_units WHERE run_id=$1 AND stage='detect'`, work.RunID).Scan(&count); err != nil {
			return err
		}
		var existing int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_name_reading_units WHERE run_id=$1 AND stage='detect' AND frame_id=$2`, work.RunID, frame.ID).Scan(&existing); err != nil {
			return err
		}
		if count+len(tiles)-existing > maxNameRunTiles {
			return ErrValidation
		}
		for index, tile := range tiles {
			payload, _ := json.Marshal(nameUnitPayload{Tile: tile})
			if _, err := tx.Exec(ctx, `INSERT INTO source_name_reading_units(run_id,unit_key,stage,frame_id,payload) VALUES($1,$2,'detect',$3,$4) ON CONFLICT(run_id,stage,unit_key) DO NOTHING`, work.RunID, fmt.Sprintf("p%04d-t%05d", frame.Page, index), frame.ID, payload); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE source_name_reading_runs SET page_count=GREATEST(page_count,$2),stage='detecting',updated_at=now() WHERE id=$1`, work.RunID, frame.Page)
		return err
	})
}
func (s *Service) reuseOrDetectNameTile(ctx context.Context, run NameReadingRun, unit nameUnit, source suggestionSource, frame NamePageFrame, tile nameReadingTile, tiled bool, work *nameReadingWork) ([]PageRegion, bool, error) {
	var payload []byte
	err := s.Pool.QueryRow(ctx, `SELECT u.payload FROM source_name_reading_units u JOIN source_name_reading_runs r ON r.id=u.run_id WHERE r.source_file_id=$1 AND r.source_checksum=$2 AND r.id<>$3 AND u.stage='detect' AND u.unit_key=$4 AND u.frame_id=$5 AND u.status='succeeded' AND r.config->>'localiserVersion'=$6 AND r.config->'localiserSettings'=$7::jsonb ORDER BY u.updated_at DESC LIMIT 1`, run.FileID, run.Checksum, run.ID, unit.Key, frame.ID, run.Config.Localiser, mustJSON(run.Config.LocaliserSettings)).Scan(&payload)
	if err == nil {
		var old nameUnitPayload
		if json.Unmarshal(payload, &old) != nil {
			return nil, false, ErrValidation
		}
		return old.Regions, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	page, err := s.nameWorkPage(ctx, source, &frame, work)
	if err != nil {
		return nil, false, err
	}
	data, err := renderNameReadingTile(ctx, page, tile.Render)
	if err != nil {
		return nil, false, err
	}
	detector := s.DetectNameTile
	if detector == nil {
		detector = DetectNameCarriers
		// A durable run's detector version is immutable. Resuming a pre-upgrade
		// run must not label v2 proposals as v1 evidence.
		if run.Config.Localiser == legacyCarrierReadingModel {
			detector = func(data []byte, config CarrierLocaliserConfig) ([]PageRegion, error) {
				return detectNameCarriers(data, config, false)
			}
		} else if run.Config.Localiser == previousCarrierReadingModel {
			detector = func(data []byte, config CarrierLocaliserConfig) ([]PageRegion, error) {
				return detectNameCarriers(data, config, true)
			}
		}
	}
	config := nameReadingTileConfig(run.Config.LocaliserSettings, frame.Width, frame.Height, tile.Render.Width, tile.Render.Height, tiled)
	regions, err := detector(data, config)
	if err != nil {
		return nil, false, err
	}
	owned := []PageRegion{}
	for _, region := range regions {
		if region.Kind != "name_group" {
			continue
		}
		left, top := float64(tile.Render.X)+region.X*float64(tile.Render.Width), float64(tile.Render.Y)+region.Y*float64(tile.Render.Height)
		width, height := region.Width*float64(tile.Render.Width), region.Height*float64(tile.Render.Height)
		cx, cy := left+width/2, top+height/2
		if cx < float64(tile.Ownership.X) || cx >= float64(tile.Ownership.X+tile.Ownership.Width) || cy < float64(tile.Ownership.Y) || cy >= float64(tile.Ownership.Y+tile.Ownership.Height) {
			continue
		}
		normal := PageRegion{Kind: "name_group", X: left / float64(frame.Width), Y: top / float64(frame.Height), Width: width / float64(frame.Width), Height: height / float64(frame.Height)}
		if _, err := suggestionCropBounds(Region{X: normal.X, Y: normal.Y, Width: normal.Width, Height: normal.Height}, frame.Width, frame.Height, "tight"); err != nil {
			return nil, false, err
		}
		owned = append(owned, normal)
	}
	return owned, false, nil
}

func (s *Service) persistNameDetection(ctx context.Context, work *nameReadingWork, run NameReadingRun, unit nameUnit, frame NamePageFrame, regions []PageRegion, duration time.Duration, cached bool) error {
	if !nameFrameMatchesRun(run, frame) {
		return ErrValidation
	}
	return s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM source_name_reading_units WHERE id=$1 FOR UPDATE`, unit.ID).Scan(&status); err != nil {
			return err
		}
		if status == "succeeded" {
			return nil
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_name_reading_units WHERE run_id=$1 AND stage='read'`, run.ID).Scan(&count); err != nil {
			return err
		}
		if count+len(regions) > maxNameRunRegions {
			return ErrValidation
		}
		for _, item := range regions {
			region := Region{X: item.X, Y: item.Y, Width: item.Width, Height: item.Height}
			if _, err := suggestionCropBounds(region, frame.Width, frame.Height, "tight"); err != nil {
				return err
			}
			fingerprint := hashNameValue(struct {
				Frame   string
				Region  Region
				Version string
			}{frame.ID, region, NameGeometryVersion})
			occurrence := fmt.Sprintf("name-p%04d-%s", frame.Page, fingerprint[:32])
			var suggestionID string
			err := tx.QueryRow(ctx, `INSERT INTO source_tree_suggestions(source_id,source_file_id,occurrence_id,page_number,region,reading_method,reading_model,name_reading_run_id,localisation_pending) SELECT source_id,source_file_id,$2,$3,$4,'local_detection',$5,id,$6 FROM source_name_reading_runs WHERE id=$1 ON CONFLICT(source_file_id,occurrence_id) DO NOTHING RETURNING id::text`, run.ID, occurrence, frame.Page, mustJSON(region), run.Config.Localiser, run.SuggestionID == "").Scan(&suggestionID)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `SELECT id::text FROM source_tree_suggestions WHERE source_file_id=$1 AND occurrence_id=$2`, run.FileID, occurrence).Scan(&suggestionID)
			}
			if err != nil {
				return err
			}
			payload := mustJSON(nameUnitPayload{Region: region})
			if _, err := tx.Exec(ctx, `INSERT INTO source_name_reading_units(run_id,unit_key,stage,frame_id,suggestion_id,payload) VALUES($1,$2,'read',$3,$4,$5) ON CONFLICT(run_id,stage,unit_key) DO NOTHING`, run.ID, "r-"+fingerprint, frame.ID, suggestionID, payload); err != nil {
				return err
			}
		}
		unit.Payload.Regions = regions
		if _, err := tx.Exec(ctx, `UPDATE source_name_reading_units SET status='succeeded',error_code='',attempts=attempts+1,duration_ms=$2,payload=$3,updated_at=now() WHERE id=$1`, unit.ID, duration.Milliseconds(), mustJSON(unit.Payload)); err != nil {
			return err
		}
		hit := 0
		if cached {
			hit = 1
		}
		_, err := tx.Exec(ctx, `UPDATE source_name_reading_runs SET detected=(SELECT count(*) FROM source_name_reading_units WHERE run_id=$1 AND stage='read'),cache_hits=cache_hits+$2,timings=jsonb_set(timings,'{detectionMs}',to_jsonb(COALESCE((timings->>'detectionMs')::bigint,0)+$3::bigint)),updated_at=now() WHERE id=$1`, run.ID, hit, duration.Milliseconds())
		return err
	})
}
func (s *Service) failNameUnit(ctx context.Context, work *nameReadingWork, id, code string, duration time.Duration) error {
	return s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE source_name_reading_units SET status='failed',attempts=attempts+1,error_code=$2,duration_ms=$3,updated_at=now() WHERE id=$1 AND status<>'succeeded'`, id, code, duration.Milliseconds())
		return err
	})
}
func (s *Service) persistLocalNameReading(ctx context.Context, work *nameReadingWork, run NameReadingRun, unit nameUnit, artifact preparedNameCrop, reading LocalCropReading, duration time.Duration) error {
	if !nameFrameMatchesRun(run, artifact.Frame) || artifact.Frame.ID != unit.FrameID {
		return ErrValidation
	}
	return s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		if !validLocalNameReading(reading) {
			return ErrValidation
		}
		if reading.Outcome != "readable" && reading.Outcome != "unreadable" && reading.Outcome != "reading_unavailable" && reading.Outcome != "manual_review" {
			return ErrValidation
		}
		var status string
		var attempts int
		if err := tx.QueryRow(ctx, `SELECT status,attempts FROM source_name_reading_units WHERE id=$1 FOR UPDATE`, unit.ID).Scan(&status, &attempts); err != nil {
			return err
		}
		if status == "succeeded" {
			return nil
		}
		alternatives, err := cleanSuggestionAlternatives(reading.Alternatives)
		if err != nil {
			return err
		}
		if alternatives == nil {
			alternatives = []string{}
		}
		if reading.Variants == nil {
			reading.Variants = []LocalVariantReading{}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO source_name_readings(run_id,unit_id,suggestion_id,artifact_fingerprint,attempt,reader_version,preprocessing_version,outcome,text_ar,alternatives,variants,quality,measured_agreement) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, run.ID, unit.ID, unit.SuggestionID, artifact.Fingerprint, attempts+1, run.Config.Reader, run.Config.Preprocessing, reading.Outcome, boundCropText(reading.Text), mustJSON(alternatives), mustJSON(reading.Variants), mustJSON(reading.Quality), reading.Agreement); err != nil {
			return err
		}
		status = "succeeded"
		code := ""
		if reading.Outcome == "reading_unavailable" {
			status = "failed"
			code = reading.Quality.Reason
		}
		previousOutcome := unit.Payload.Outcome
		unit.Payload.Outcome = reading.Outcome
		if _, err = tx.Exec(ctx, `UPDATE source_name_reading_units SET status=$2,attempts=attempts+1,error_code=$3,duration_ms=$4,payload=$5,updated_at=now() WHERE id=$1`, unit.ID, status, code, duration.Milliseconds(), mustJSON(unit.Payload)); err != nil {
			return err
		}

		if reading.Outcome != "reading_unavailable" {
			if _, err = tx.Exec(ctx, `UPDATE source_tree_suggestions SET suggested_text_ar=$2,alternatives=$3,confidence=$4,view_count=$5,reading_method='local_tesseract_crop',reading_model=$6,name_reading_run_id=$7,updated_at=now() WHERE id=$1 AND status='suggested' AND superseded_at IS NULL AND (name_reading_run_id=$7 OR reading_method='local_detection')`, unit.SuggestionID, boundCropText(reading.Text), mustJSON(alternatives), reading.Agreement, max(1, len(reading.Variants)), run.Config.Reader, run.ID); err != nil {
				return err
			}
		}
		hit := 0
		if artifact.CacheHit {
			hit = 1
		}
		delta := func(outcome string) int {
			value := 0
			if reading.Outcome == outcome {
				value++
			}
			if previousOutcome == outcome {
				value--
			}
			return value
		}
		_, err = tx.Exec(ctx, `UPDATE source_name_reading_runs SET named=named+$2,unreadable=unreadable+$3,unavailable=unavailable+$4,cache_hits=cache_hits+$5,timings=jsonb_set(timings,'{readingMs}',to_jsonb(COALESCE((timings->>'readingMs')::bigint,0)+$6::bigint)),updated_at=now() WHERE id=$1`, run.ID, delta("readable"), delta("unreadable"), delta("reading_unavailable"), hit, duration.Milliseconds())
		return err
	})
}

func (s *Service) SuggestionLocalReadings(ctx context.Context, id, actorID string) ([]LocalCropReading, error) {
	if _, err := s.loadSuggestionContext(ctx, id, actorID, false); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT outcome,text_ar,alternatives,variants,quality,measured_agreement,reader_version,preprocessing_version FROM source_name_readings WHERE suggestion_id=$1 ORDER BY created_at DESC,id DESC LIMIT 10`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []LocalCropReading{}
	for rows.Next() {
		var item LocalCropReading
		var alternatives, variants, quality []byte
		if err := rows.Scan(&item.Outcome, &item.Text, &alternatives, &variants, &quality, &item.Agreement, &item.ReaderVersion, &item.PreprocessingVersion); err != nil {
			return nil, err
		}
		if json.Unmarshal(alternatives, &item.Alternatives) != nil || json.Unmarshal(variants, &item.Variants) != nil || json.Unmarshal(quality, &item.Quality) != nil {
			return nil, ErrValidation
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func nameFrameMatchesRun(run NameReadingRun, frame NamePageFrame) bool {
	return frame.valid() && frame.FileID == run.FileID && frame.Checksum == run.Checksum && frame.Renderer == run.Config.Renderer
}
