package sourceprocessing

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SignedDownload is the only path from an authorized caller to the bytes of an
// uploaded file, and it is deliberately a service method rather than a handler
// that reaches into the store: the storage key never leaves this package, so
// there is no code path in which a handler could put one in a response, a log
// line or a template by accident.
//
// What happens, in order:
//
//  1. the file is looked up BY ID. The caller never supplies a key, so it cannot
//     ask for "whatever key you hold for source X" - it names a file, and the
//     database decides which key that is;
//  2. the actor is authorized to review the source the file belongs to;
//  3. only then is a capability minted, with a bounded lifetime.
//
// The row is read before the authorization check, and that is safe precisely
// because step 3 is the first thing that discloses anything: a denied caller gets
// an error and no key, no filename and no content type.
//
// The returned URL contains the key, because that is what an S3 presigned URL
// contains and there is nowhere else for it to be. What never happens is an
// unauthorized caller learning the key: it is disclosed only inside a signed,
// expiring URL whose minting required step 2 to pass.
func (s *Service) SignedDownload(ctx context.Context, fileID, actorID string, requestedExpiry time.Duration) (DownloadLink, error) {
	if err := s.ready(); err != nil {
		return DownloadLink{}, err
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return DownloadLink{}, err
	}
	var sourceID pgtype.UUID
	var key, filename, contentType pgtype.Text
	if err := s.Pool.QueryRow(ctx, `
		SELECT source_id, storage_key, original_filename_ar, mime_type
		FROM source_files
		WHERE id = $1
	`, fileUUID).Scan(&sourceID, &key, &filename, &contentType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DownloadLink{}, ErrNotFound
		}
		return DownloadLink{}, err
	}
	// canReviewSource, not canManageSource: reading the document a source
	// uploaded is a review action, and an account that can review the source's
	// other derived data can read its documents. Uploading and deleting stay at
	// manage.
	allowed, err := canReviewSource(ctx, s.Pool, uuid.UUID(sourceID.Bytes), actorUUID)
	if err != nil {
		return DownloadLink{}, err
	}
	if !allowed {
		return DownloadLink{}, ErrForbidden
	}
	storageKey := textValue(key)
	if storageKey == "" {
		// A row with no key is not a file anybody can fetch. Saying so is better
		// than minting a URL that 404s.
		return DownloadLink{}, ErrNotFound
	}
	expiry, err := storage.NormalizeExpiry(requestedExpiry)
	if err != nil {
		return DownloadLink{}, ErrValidation
	}
	signed, err := s.Store.SignedURL(ctx, storageKey, expiry)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrSignerNotConfigured), errors.Is(err, storage.ErrNotConfigured):
			// The store is real but cannot mint links. That is a configuration
			// problem, and the caller's answer is the same either way: not now.
			return DownloadLink{}, ErrStorageUnavailable
		case errors.Is(err, storage.ErrInvalidKey):
			return DownloadLink{}, ErrNotFound
		default:
			return DownloadLink{}, ErrStorageUnavailable
		}
	}
	return DownloadLink{
		URL:              signed,
		Filename:         textValue(filename),
		ContentType:      textValue(contentType),
		ExpiresInSeconds: int64(expiry / time.Second),
	}, nil
}

// DownloadLink is what the caller gets: a capability, and the metadata needed to
// name the file it points at. Deliberately no key, no bucket and no directory.
type DownloadLink struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	// ExpiresInSeconds is the lifetime the caller was granted, not the absolute
	// expiry, because the two clocks (this process and the store's) are not
	// assumed to agree.
	ExpiresInSeconds int64 `json:"expiresInSeconds"`
}

// OpenSignedObject is the other half of LocalSigner.Sign, and the only place in
// the service that reads an object without a session. That is what a capability
// URL means; it is why the signature covers the key, and why the route that calls
// this trusts nothing else about the request.
//
// It is not a general "fetch any object" endpoint: the caller supplies a key, and
// the only thing standing between that key and the bytes is the signature. A
// caller who has a valid link for one object still cannot read another, because
// the signature is over the key.
func (s *Service) OpenSignedObject(ctx context.Context, key, expires, signature string) (io.ReadCloser, storage.Object, error) {
	if s == nil || s.Store == nil {
		return nil, storage.Object{}, ErrStorageUnavailable
	}
	local, ok := storage.LocalStoreFor(s.Store)
	if !ok || local.Signer() == nil {
		// The S3 adapter does not serve objects through this service at all -
		// the presigned URL goes to S3. Reaching here means somebody wired the
		// route to a store that cannot answer it, which is a configuration error
		// rather than a not-found.
		return nil, storage.Object{}, ErrStorageUnavailable
	}
	if err := local.Signer().Verify(key, expires, signature); err != nil {
		if errors.Is(err, storage.ErrExpired) {
			return nil, storage.Object{}, ErrLinkExpired
		}
		return nil, storage.Object{}, ErrLinkNotAuthorized
	}
	reader, object, err := local.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, storage.Object{}, ErrNotFound
		}
		return nil, storage.Object{}, ErrStorageUnavailable
	}
	return reader, object, nil
}
