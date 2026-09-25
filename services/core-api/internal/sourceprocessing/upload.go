package sourceprocessing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewService(pool *pgxpool.Pool, store storage.Store, queue JobEnqueuer, provider ai.Provider, extractor Extractor) *Service {
	if extractor == nil {
		extractor = NewTextExtractor()
	}
	return &Service{Pool: pool, Store: store, Jobs: queue, AI: provider, Extractor: extractor}
}

func (s *Service) Upload(ctx context.Context, sourceID, actorID string, input UploadInput) (FileView, error) {
	if err := s.ready(); err != nil {
		return FileView{}, err
	}
	sourceUUID, actorUUID, err := parseIDs(sourceID, actorID)
	if err != nil {
		return FileView{}, err
	}
	input, err = validateUploadInput(input)
	if err != nil {
		return FileView{}, err
	}
	allowed, err := canManageSource(ctx, s.Pool, sourceUUID, actorUUID)
	if err != nil {
		return FileView{}, err
	}
	if !allowed {
		return FileView{}, ErrForbidden
	}
	fileID := uuid.New()
	key := fmt.Sprintf("sources/%s/%s-%s", sourceUUID, fileID, safeFilename(input.Filename))
	object, err := s.Store.Put(ctx, key, bytes.NewReader(input.Content), input.ContentType)
	if err != nil {
		return FileView{}, ErrStorageUnavailable
	}
	if object.Size != int64(len(input.Content)) {
		_ = s.Store.Delete(ctx, key)
		return FileView{}, ErrStorageUnavailable
	}
	digest := sha256.Sum256(input.Content)
	checksum := hex.EncodeToString(digest[:])
	payload, err := json.Marshal(JobPayload{SourceID: sourceUUID.String(), SourceFileID: fileID.String()})
	if err != nil {
		_ = s.Store.Delete(ctx, key)
		return FileView{}, err
	}
	enqueueInput := jobs.EnqueueInput{
		Type:           SourceProcessJobType,
		Payload:        payload,
		Priority:       0,
		IdempotencyKey: "source_process:" + fileID.String(),
		MaxAttempts:    3,
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		_ = s.Store.Delete(ctx, key)
		return FileView{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_files (id, source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256, processing_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'queued')
	`, fileID, sourceUUID, key, input.Filename, input.ContentType, len(input.Content), checksum); err != nil {
		_ = s.Store.Delete(ctx, key)
		return FileView{}, err
	}
	runID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_processing_runs (id, source_id, source_file_id, status, stage)
		VALUES ($1, $2, $3, 'queued', 'queued')
	`, runID, sourceUUID, fileID); err != nil {
		_ = s.Store.Delete(ctx, key)
		return FileView{}, err
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_file_uploaded", "source_file", fileID, nil, map[string]any{
		"sourceId": sourceUUID.String(), "filename": input.Filename, "mimeType": input.ContentType, "byteSize": len(input.Content), "checksumSha256": checksum,
	}); err != nil {
		_ = s.Store.Delete(ctx, key)
		return FileView{}, err
	}
	if transactional, ok := s.Jobs.(TransactionalJobEnqueuer); ok {
		enqueued, err := transactional.EnqueueTx(ctx, tx, enqueueInput)
		if err != nil {
			_ = s.Store.Delete(ctx, key)
			return FileView{}, ErrQueueUnavailable
		}
		if _, err := tx.Exec(ctx, `UPDATE source_processing_runs SET job_id = $1, updated_at = now() WHERE id = $2`, enqueued.Job.ID, runID); err != nil {
			_ = s.Store.Delete(ctx, key)
			return FileView{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			_ = s.Store.Delete(ctx, key)
			return FileView{}, err
		}
		return s.getFile(ctx, fileID)
	}
	if err := tx.Commit(ctx); err != nil {
		_ = s.Store.Delete(ctx, key)
		return FileView{}, err
	}
	enqueued, err := s.Jobs.Enqueue(ctx, enqueueInput)
	if err != nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE source_files SET processing_status = 'failed', processing_error = $1 WHERE id = $2`, "job enqueue failed", fileID)
		_, _ = s.Pool.Exec(ctx, `UPDATE source_processing_runs SET status = 'failed', stage = 'queue', error = $1, updated_at = now() WHERE id = $2`, "job enqueue failed", runID)
		return FileView{}, ErrQueueUnavailable
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE source_processing_runs SET job_id = $1, updated_at = now() WHERE id = $2`, enqueued.Job.ID, runID); err != nil {
		return FileView{}, err
	}
	return s.getFile(ctx, fileID)
}

func validateUploadInput(input UploadInput) (UploadInput, error) {
	input.Filename = strings.TrimSpace(input.Filename)
	if input.Filename == "" || !utf8.ValidString(input.Filename) || len([]rune(input.Filename)) > 255 || len(input.Content) == 0 || int64(len(input.Content)) > MaxUploadBytes {
		return UploadInput{}, ErrValidation
	}
	contentType, err := resolveContentType(input.ContentType, input.Content)
	if err != nil {
		return UploadInput{}, err
	}
	input.ContentType = contentType
	return input, nil
}

// V1 source format contract. The extraction worker reads text only, so the
// accepted matrix is text plus the two structured text formats, and the API,
// the worker, the error copy, and the UI copy all read it from here. Adding a
// format means adding an extractor, not a row.
var supportedContentTypes = []string{"text/*", "application/json", "application/xml"}

// undeclaredContentType is what a multipart part carries when the client does
// not name a format. It carries no claim to verify, so the bytes decide.
const undeclaredContentType = "application/octet-stream"

// ErrUnsupportedContent marks an upload that falls outside the format contract.
// It is never returned for a supported format, whatever the file contains.
var ErrUnsupportedContent = errors.New("source upload format is not supported")

// UnsupportedContentError explains which format was refused and why, and always
// names the formats a caller may send instead.
type UnsupportedContentError struct {
	Reason string
}

func (e *UnsupportedContentError) Error() string {
	reason := e.Reason
	if strings.TrimSpace(reason) == "" {
		reason = "the file format is not supported"
	}
	return reason + "; supported formats: " + strings.Join(SupportedContentTypes(), ", ")
}

func (e *UnsupportedContentError) Is(target error) bool {
	return target == ErrUnsupportedContent || target == ErrUnsupportedDocument
}

// SupportedContentTypes is the caller-facing matrix, used for the error payload
// and for the sentence that tells a refused caller what to send instead.
func SupportedContentTypes() []string {
	return append([]string(nil), supportedContentTypes...)
}

// IsSupportedContentType is the single decision on whether a media type is
// inside the contract, shared by the upload boundary and the extractor.
func IsSupportedContentType(value string) bool {
	normalized := normalizeContentType(value)
	if normalized == "" {
		return false
	}
	for _, supported := range supportedContentTypes {
		if strings.HasSuffix(supported, "/*") {
			if strings.HasPrefix(normalized, strings.TrimSuffix(supported, "*")) {
				return true
			}
			continue
		}
		if normalized == supported {
			return true
		}
	}
	return false
}

func unsupportedContent(reason string) error {
	return &UnsupportedContentError{Reason: reason}
}

// UnsupportedFormatSummary is the shared sentence for a refused format that has
// no more specific reason to give.
func UnsupportedFormatSummary() string {
	return (&UnsupportedContentError{}).Error()
}

// resolveContentType settles the media type of an upload without trusting the
// caller alone. An undeclared type is read from the content, and a declaration
// that contradicts the content is refused rather than stored.
func resolveContentType(declared string, content []byte) (string, error) {
	normalized := normalizeContentType(declared)
	detected := normalizeContentType(http.DetectContentType(content))
	effective := normalized
	if effective == "" || effective == undeclaredContentType {
		effective = detected
	}
	if !IsSupportedContentType(effective) {
		if normalized == "" || normalized == undeclaredContentType {
			return "", unsupportedContent(fmt.Sprintf("the file content is detected as %q", effective))
		}
		return "", unsupportedContent(fmt.Sprintf("the declared content type %q is not accepted", effective))
	}
	if !IsSupportedContentType(detected) {
		return "", unsupportedContent(fmt.Sprintf("the declared content type %q contradicts the detected content type %q", effective, detected))
	}
	if !utf8.Valid(content) {
		return "", unsupportedContent(fmt.Sprintf("the content type %q does not carry valid UTF-8 text", effective))
	}
	return effective, nil
}

func normalizeContentType(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
}

func safeFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.Map(func(character rune) rune {
		if character < 32 || character == '/' || character == '\\' {
			return '_'
		}
		return character
	}, value)
	if value == "" || value == "." || value == ".." {
		return "source"
	}
	return value
}

func parseIDs(resourceID, actorID string) (uuid.UUID, uuid.UUID, error) {
	resourceUUID, err := uuid.Parse(strings.TrimSpace(resourceID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrForbidden
	}
	return resourceUUID, actorUUID, nil
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	if s.Store == nil || s.Jobs == nil {
		return ErrStorageUnavailable
	}
	return nil
}

func (s *Service) getFile(ctx context.Context, fileID uuid.UUID) (FileView, error) {
	var item FileView
	var sourceID pgtype.UUID
	var filename, mimeType, checksum, processingError pgtype.Text
	var size pgtype.Int8
	var processedAt pgtype.Timestamptz
	if err := s.Pool.QueryRow(ctx, `
		SELECT id, source_id, original_filename_ar, mime_type, byte_size, checksum_sha256, processing_status, processing_error, processed_at, created_at
		FROM source_files WHERE id = $1
	`, fileID).Scan(&item.ID, &sourceID, &filename, &mimeType, &size, &checksum, &item.ProcessingStatus, &processingError, &processedAt, &item.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FileView{}, ErrNotFound
		}
		return FileView{}, err
	}
	item.SourceID = uuidString(sourceID)
	item.OriginalFilenameAR = textValue(filename)
	item.MimeType = textValue(mimeType)
	if size.Valid {
		item.ByteSize = size.Int64
	}
	item.ChecksumSHA256 = textValue(checksum)
	item.ProcessingError = textValue(processingError)
	if processedAt.Valid {
		value := processedAt.Time
		item.ProcessedAt = &value
	}
	return item, nil
}

func writeAudit(ctx context.Context, q auditExecutor, actorID *uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) error {
	_, err := q.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, actorID, action, entityType, entityID, marshalValue(before), marshalValue(after))
	return err
}

type auditExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func marshalValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
