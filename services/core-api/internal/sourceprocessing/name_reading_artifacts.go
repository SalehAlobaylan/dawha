package sourceprocessing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/singleflight"
)

const NameRendererVersion = "name-source-space-v2"
const NameGeometryVersion = "page-region-v1"
const maxNameArtifactBytes = int64(256 << 20)

var nameArtifactFlights singleflight.Group

type NamePageFrame struct {
	ID        string `json:"id"`
	FileID    string `json:"-"`
	Checksum  string `json:"-"`
	Page      int    `json:"page"`
	Space     string `json:"coordinateSpace"`
	DPI       int    `json:"analysisDpi"`
	Width     int    `json:"widthPx"`
	Height    int    `json:"heightPx"`
	PageCount int    `json:"pageCount"`
	Renderer  string `json:"rendererVersion"`
}

func (f NamePageFrame) valid() bool {
	return f.Page > 0 && f.Page <= f.PageCount && f.PageCount <= MaxPages && f.Width > 0 && f.Height > 0 && f.Width <= maxChartPageAxis && f.Height <= maxChartPageAxis && f.Renderer == NameRendererVersion && ((f.Space == "native_raster" && f.DPI == 0) || (f.Space == "pdf_analysis" && f.DPI == chartAnalysisDPI))
}
func frameFromPage(source suggestionSource, page *ChartPDFPage) NamePageFrame {
	space, dpi := "pdf_analysis", chartAnalysisDPI
	if isImageContentType(source.mimeType) {
		space, dpi = "native_raster", 0
	}
	return NamePageFrame{FileID: source.sourceFileID.String(), Checksum: source.checksum, Page: source.pageNumber, Space: space, DPI: dpi, Width: page.Width, Height: page.Height, PageCount: page.PageCount, Renderer: NameRendererVersion}
}
func hashNameValue(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
func (s *Service) readNameOriginal(ctx context.Context, source suggestionSource) ([]byte, error) {
	if s.Store == nil {
		return nil, ErrStorageUnavailable
	}
	reader, object, err := s.Store.Get(ctx, source.storageKey)
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, MaxUploadBytes+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || int64(len(data)) != object.Size || len(data) > MaxUploadBytes {
		return nil, ErrStorageUnavailable
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != source.checksum {
		return nil, ErrValidation
	}
	return data, nil
}
func (s *Service) openNameSource(ctx context.Context, source suggestionSource, frame *NamePageFrame) (*ChartPDFPage, error) {
	data := source.content
	var err error
	if data == nil {
		data, err = s.readNameOriginal(ctx, source)
		if err != nil {
			return nil, err
		}
	}
	if s.OpenNamePage != nil {
		return s.OpenNamePage(ctx, data, source.pageNumber)
	}
	if frame == nil || isImageContentType(source.mimeType) {
		return source.extractor.OpenChartPageForAnalysis(ctx, data, source.pageNumber)
	}
	if !frame.valid() || frame.Checksum != source.checksum || frame.Page != source.pageNumber {
		return nil, ErrValidation
	}
	directory, err := os.MkdirTemp("", "dawha-name-render-")
	if err != nil {
		return nil, err
	}
	filename := filepath.Join(directory, "source.pdf")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	return &ChartPDFPage{PDFToPPM: source.extractor.PDFToPPM, PDFPath: filename, TempDir: directory, PageNumber: frame.Page, PageCount: frame.PageCount, DPI: frame.DPI, Width: frame.Width, Height: frame.Height}, nil
}
func loadNameFrame(ctx context.Context, q jobsQuery, source suggestionSource) (NamePageFrame, error) {
	var frame NamePageFrame
	err := q.QueryRow(ctx, `SELECT id::text,source_file_id::text,source_checksum,page_number,coordinate_space,analysis_dpi,width_px,height_px,page_count,renderer_version FROM source_name_page_frames WHERE source_file_id=$1 AND source_checksum=$2 AND page_number=$3 AND renderer_version=$4`, source.sourceFileID, source.checksum, source.pageNumber, NameRendererVersion).Scan(&frame.ID, &frame.FileID, &frame.Checksum, &frame.Page, &frame.Space, &frame.DPI, &frame.Width, &frame.Height, &frame.PageCount, &frame.Renderer)
	if err == nil && !frame.valid() {
		return frame, ErrValidation
	}
	return frame, err
}

type jobsQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func saveNameFrame(ctx context.Context, tx pgx.Tx, frame NamePageFrame) (NamePageFrame, error) {
	if !frame.valid() {
		return frame, ErrValidation
	}
	err := tx.QueryRow(ctx, `INSERT INTO source_name_page_frames(source_file_id,source_checksum,page_number,coordinate_space,analysis_dpi,width_px,height_px,page_count,renderer_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(source_file_id,source_checksum,page_number,renderer_version) DO NOTHING RETURNING id::text`, frame.FileID, frame.Checksum, frame.Page, frame.Space, frame.DPI, frame.Width, frame.Height, frame.PageCount, frame.Renderer).Scan(&frame.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, loadErr := loadNameFrame(ctx, tx, suggestionSource{sourceFileID: uuid.MustParse(frame.FileID), checksum: frame.Checksum, pageNumber: frame.Page})
		if loadErr != nil {
			return frame, loadErr
		}
		if existing.Space != frame.Space || existing.DPI != frame.DPI || existing.Width != frame.Width || existing.Height != frame.Height {
			return frame, ErrConflict
		}
		return existing, nil
	}
	return frame, err
}

type nameCropDescriptor struct {
	Frame         NamePageFrame   `json:"frame"`
	Region        Region          `json:"region"`
	Geometry      string          `json:"geometryVersion"`
	Preprocessing string          `json:"preprocessingVersion"`
	TightBounds   ChartTileBounds `json:"tightBounds"`
	ContextBounds ChartTileBounds `json:"contextBounds"`
}
type preparedNameCrop struct {
	Crop        SuggestionCrop
	Frame       NamePageFrame
	Fingerprint string
	CacheHit    bool
	Quality     CropObservations
}

func artifactDescriptor(source suggestionSource, frame NamePageFrame) nameCropDescriptor {
	// File/checksum must be in the identity, even though neither is a provider field.
	tight, _ := suggestionCropBounds(source.region, frame.Width, frame.Height, "tight")
	contextBounds, _ := suggestionCropBounds(source.region, frame.Width, frame.Height, "context")
	return nameCropDescriptor{Frame: frame, Region: source.region, Geometry: NameGeometryVersion, Preprocessing: NamePreprocessingVersion, TightBounds: tight, ContextBounds: contextBounds}
}
func artifactFingerprint(source suggestionSource, frame NamePageFrame) string {
	return hashNameValue(struct {
		File, Checksum string
		Descriptor     nameCropDescriptor
	}{source.sourceFileID.String(), source.checksum, artifactDescriptor(source, frame)})
}

// Fetch authorization happens before singleflight or cache lookup. Cancelling a
// display subscriber does not cancel another subscriber's bounded preparation.
func (s *Service) prepareNameCrop(ctx context.Context, source suggestionSource, page *ChartPDFPage, work *nameReadingWork) (preparedNameCrop, error) {
	if page != nil || work != nil {
		return s.prepareNameCropLocked(ctx, source, page, work)
	}
	key := hashNameValue(struct {
		File, Checksum string
		Page           int
		Region         Region
	}{source.sourceFileID.String(), source.checksum, source.pageNumber, source.region})
	result := nameArtifactFlights.DoChan(key, func() (any, error) {
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
		return s.prepareNameCropLocked(shared, source, nil, work)
	})
	select {
	case <-ctx.Done():
		return preparedNameCrop{}, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return preparedNameCrop{}, value.Err
		}
		return value.Val.(preparedNameCrop), nil
	}
}
func (s *Service) prepareNameCropLocked(ctx context.Context, source suggestionSource, page *ChartPDFPage, work *nameReadingWork) (preparedNameCrop, error) {
	result := preparedNameCrop{}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	// Serialize first-frame preparation and identical renders across processes.
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,24))`, source.sourceFileID.String()+source.checksum+fmt.Sprint(source.pageNumber))
	if err != nil {
		return result, err
	}
	frame, err := loadNameFrame(ctx, tx, source)
	if errors.Is(err, pgx.ErrNoRows) {
		if page == nil {
			if work != nil {
				page, err = s.nameWorkPage(ctx, source, nil, work)
			} else {
				page, err = s.openNameSource(ctx, source, nil)
			}
			if err != nil {
				return result, err
			}
			if work == nil {
				defer page.Close()
			}
		}
		frame = frameFromPage(source, page)
		if err = s.nameWriteFence(ctx, tx, work); err != nil {
			return result, err
		}
		frame, err = saveNameFrame(ctx, tx, frame)
	}
	if err != nil {
		return result, err
	}
	result.Frame = frame
	result.Fingerprint = artifactFingerprint(source, frame)
	tightBounds, err := suggestionCropBounds(source.region, frame.Width, frame.Height, "tight")
	if err != nil {
		return result, err
	}
	result.Quality = CropObservations{NameHeight: int(source.region.Height*float64(frame.Height) + .5), Clipped: source.region.Width*float64(frame.Width) > float64(tightBounds.Width) || source.region.Height*float64(frame.Height) > float64(tightBounds.Height) || source.region.X <= 0 || source.region.Y <= 0 || source.region.X+source.region.Width >= .9999 || source.region.Y+source.region.Height >= .9999}
	var tightKey, contextKey string
	err = tx.QueryRow(ctx, `SELECT tight_key,context_key FROM source_name_crop_artifacts WHERE fingerprint=$1 AND source_file_id=$2 AND expires_at>now()`, result.Fingerprint, source.sourceFileID).Scan(&tightKey, &contextKey)
	if err == nil {
		result.Crop, err = s.readNameArtifact(ctx, tightKey, contextKey)
		if err == nil {
			result.CacheHit = true
			return result, tx.Commit(ctx)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if page == nil {
		if work != nil {
			page, err = s.nameWorkPage(ctx, source, &frame, work)
		} else {
			page, err = s.openNameSource(ctx, source, &frame)
		}
		if err != nil {
			return result, err
		}
		if work == nil {
			defer page.Close()
		}
	}
	if page.Width != frame.Width || page.Height != frame.Height {
		return result, ErrValidation
	}
	contextBounds, err := suggestionCropBounds(source.region, frame.Width, frame.Height, "context")
	if err != nil {
		return result, err
	}
	result.Crop.TightPNG, err = page.RenderRegion(ctx, tightBounds)
	if err != nil {
		return result, err
	}
	result.Crop.ContextPNG, err = page.RenderRegion(ctx, contextBounds)
	if err != nil {
		return result, err
	}
	if !validNameArtifactPNG(result.Crop.TightPNG) || !validNameArtifactPNG(result.Crop.ContextPNG) {
		return result, ErrValidation
	}
	if err = s.nameWriteFence(ctx, tx, work); err != nil {
		return result, err
	}
	// Lock only during the bounded cache write/budget eviction, not OCR/rendering.
	if _, err = tx.Exec(ctx, `SELECT id FROM source_files WHERE id=$1 FOR UPDATE`, source.sourceFileID); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM source_name_crop_artifacts WHERE fingerprint=$1`, result.Fingerprint); err != nil {
		return result, err
	}
	size := int64(len(result.Crop.TightPNG) + len(result.Crop.ContextPNG))
	if err = s.trimNameArtifacts(ctx, tx, source.sourceFileID, size); err != nil {
		return result, err
	}
	prefix := "_name-crops/" + result.Fingerprint + "/"
	tightKey, contextKey = prefix+"tight.png", prefix+"context.png"
	committed := false
	defer func() {
		if !committed {
			cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer stop()
			_ = s.Store.Delete(cleanup, tightKey)
			_ = s.Store.Delete(cleanup, contextKey)
		}
	}()
	for _, item := range []struct {
		key  string
		data []byte
	}{{tightKey, result.Crop.TightPNG}, {contextKey, result.Crop.ContextPNG}} {
		if _, err = s.Store.Put(ctx, item.key, bytes.NewReader(item.data), "image/png"); err != nil {
			return result, ErrStorageUnavailable
		}
	}
	descriptor, _ := json.Marshal(artifactDescriptor(source, frame))
	_, err = tx.Exec(ctx, `INSERT INTO source_name_crop_artifacts(fingerprint,source_file_id,frame_id,descriptor,tight_key,context_key,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(fingerprint) DO UPDATE SET expires_at=now()+interval '30 days'`, result.Fingerprint, source.sourceFileID, frame.ID, descriptor, tightKey, contextKey, size)
	if err != nil {
		return result, err
	}
	if err = s.nameWriteFence(ctx, tx, work); err != nil {
		return result, err
	}
	err = tx.Commit(ctx)
	committed = err == nil
	return result, err
}
func validNameArtifactPNG(data []byte) bool {
	if len(data) == 0 || len(data) > 10_000_000 {
		return false
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	return err == nil && config.Width <= suggestionCropMaxSide && config.Height <= suggestionCropMaxSide && config.Width > 0 && config.Height > 0 && int64(config.Width)*int64(config.Height) <= suggestionCropMaxPixels
}
func (s *Service) readNameArtifact(ctx context.Context, tightKey, contextKey string) (SuggestionCrop, error) {
	data := make([][]byte, 2)
	for index, key := range []string{tightKey, contextKey} {
		reader, object, err := s.Store.Get(ctx, key)
		if err != nil {
			return SuggestionCrop{}, ErrStorageUnavailable
		}
		content, readErr := io.ReadAll(io.LimitReader(reader, 10_000_001))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || object.Size != int64(len(content)) || !validNameArtifactPNG(content) {
			return SuggestionCrop{}, ErrStorageUnavailable
		}
		data[index] = content
	}
	return SuggestionCrop{TightPNG: data[0], ContextPNG: data[1]}, nil
}
func (s *Service) trimNameArtifacts(ctx context.Context, tx pgx.Tx, fileID uuid.UUID, needed int64) error {
	var total int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(byte_size),0) FROM source_name_crop_artifacts WHERE source_file_id=$1`, fileID).Scan(&total); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT fingerprint,tight_key,context_key,byte_size,expires_at<now() FROM source_name_crop_artifacts WHERE source_file_id=$1 ORDER BY expires_at,created_at`, fileID)
	if err != nil {
		return err
	}
	type entry struct {
		fingerprint, tight, context string
		size                        int64
		expired                     bool
	}
	entries := []entry{}
	for rows.Next() {
		var item entry
		if err := rows.Scan(&item.fingerprint, &item.tight, &item.context, &item.size, &item.expired); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range entries {
		if !item.expired && total+needed <= maxNameArtifactBytes {
			break
		}
		if _, err := tx.Exec(ctx, `DELETE FROM source_name_crop_artifacts WHERE fingerprint=$1`, item.fingerprint); err != nil {
			return err
		}
		if err := s.Store.Delete(ctx, item.tight); err != nil {
			return err
		}
		if err := s.Store.Delete(ctx, item.context); err != nil {
			return err
		}
		total -= item.size
	}
	return nil
}

// PruneNameArtifacts is bounded cache maintenance. It deletes only expired
// generated objects; originals and immutable reading hypotheses are untouched.
func (s *Service) PruneNameArtifacts(ctx context.Context) (int, error) {
	if s == nil || s.Pool == nil || s.Store == nil {
		return 0, ErrStorageUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT fingerprint,tight_key,context_key FROM source_name_crop_artifacts WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 64 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type artifact struct{ fp, tight, context string }
	items := []artifact{}
	for rows.Next() {
		var item artifact
		if err := rows.Scan(&item.fp, &item.tight, &item.context); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range items {
		prefix := "_name-crops/" + item.fp + "/"
		if item.tight != prefix+"tight.png" || item.context != prefix+"context.png" {
			return 0, ErrValidation
		}
		if err := s.Store.Delete(ctx, item.tight); err != nil {
			return 0, err
		}
		if err := s.Store.Delete(ctx, item.context); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM source_name_crop_artifacts WHERE fingerprint=$1`, item.fp); err != nil {
			return 0, err
		}
	}
	return len(items), tx.Commit(ctx)
}
