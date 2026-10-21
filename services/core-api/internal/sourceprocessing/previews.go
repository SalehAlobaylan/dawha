package sourceprocessing

import (
	"context"
	"errors"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// PagePreview renders the requested PDF page locally after checking source
// review access. The image is a short-lived response, never stored or sent to a
// hosted OCR or vision provider.
func (s *Service) PagePreview(ctx context.Context, fileID, actorID string, pageNumber int) ([]byte, error) {
	if err := s.databaseReady(); err != nil {
		return nil, err
	}
	if pageNumber < 1 || pageNumber > MaxPages {
		return nil, ErrValidation
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return nil, err
	}
	var sourceID pgtype.UUID
	var storageKey, mimeType string
	if err := s.Pool.QueryRow(ctx, `SELECT source_id, storage_key, COALESCE(mime_type, '') FROM source_files WHERE id = $1`, fileUUID).Scan(&sourceID, &storageKey, &mimeType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	allowed, err := canReviewSource(ctx, s.Pool, uuidValue(sourceID), actorUUID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	if normalizeContentType(mimeType) != "application/pdf" {
		return nil, ErrUnsupportedDocument
	}
	if s.Store == nil {
		return nil, ErrStorageUnavailable
	}
	reader, _, err := s.Store.Get(ctx, storageKey)
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, MaxUploadBytes+1))
	if err != nil || len(content) > MaxUploadBytes {
		return nil, ErrStorageUnavailable
	}
	renderer, ok := s.Extractor.(*PDFExtractor)
	if !ok || renderer == nil {
		renderer = NewPDFExtractor()
	}
	return renderer.RenderPage(ctx, content, pageNumber)
}
