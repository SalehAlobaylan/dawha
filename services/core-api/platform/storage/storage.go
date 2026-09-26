// Package storage is the one place that knows how source bytes are kept.
//
// There are two adapters, and the choice between them is configuration rather
// than a code path:
//
//   - LocalStore, the DEVELOPMENT adapter. Bytes live in a directory. It needs no
//     bucket, no endpoint, no credentials and no service, which is why the local
//     stack, the CI database job and the browser suite all use it.
//   - S3Store, the PRODUCTION adapter. Bytes live in an S3-compatible bucket and
//     download access is a presigned URL. It is constructed only when it is
//     configured; see NewS3 and ErrNotConfigured.
//
// The interface is four methods because that is the whole surface the
// application needs. What is deliberately NOT in it is any notion of a
// permanent public URL: a raw storage key is not an address a client may be
// handed. Access is minted (Store.SignedURL) after an authorization decision the
// caller cannot see, and it expires.
package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

// Object is what a store knows about one object it holds.
//
// ContentType is what the caller declared and the upload boundary verified; it
// is advisory for reads, because a store that persists the bytes does not
// necessarily persist the label with them.
type Object struct {
	Key         string
	ContentType string
	Size        int64
}

// Store is the interface both adapters implement, and the interface the
// contract suite in contract_test.go runs against both of them.
type Store interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) (Object, error)
	Get(ctx context.Context, key string) (io.ReadCloser, Object, error)
	SignedURL(ctx context.Context, key string, expires time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
}

var (
	// ErrInvalidKey is a key that is empty, absolute, or escapes its root. Both
	// adapters reject these before touching a filesystem or a bucket, because a
	// key is attacker-influenced (it embeds a filename) and neither the local
	// filesystem nor an object key namespace should be the thing that decides how
	// far a request can reach.
	ErrInvalidKey = errors.New("storage key is invalid")

	// ErrNotFound is an object the store does not hold.
	ErrNotFound = errors.New("storage object was not found")

	// ErrNotConfigured is an adapter that was selected but not set up: an S3
	// driver with no bucket, an unknown driver name, a local store with no
	// signing secret. It is deliberately a distinct error from "the request
	// failed", because the fix for it is configuration, not a retry - and a
	// deployment that silently degraded to an unsigned or local store would be
	// the failure this plan is about.
	ErrNotConfigured = errors.New("storage is not configured")

	// ErrSignerNotConfigured is a local store with no signing secret, asked to
	// mint a signed URL. LocalStore can hold and serve bytes perfectly well
	// without one; it just cannot hand out a capability URL, and it says so
	// rather than returning an unsigned link.
	ErrSignerNotConfigured = errors.New("storage signing is not configured")

	// ErrReadOnly is retained for callers that still branch on it. LocalStore no
	// longer returns it: signed access is implemented, not refused.
	ErrReadOnly = errors.New("local storage does not support signed URLs")

	// ErrExpired is a signature presented after its expiry.
	ErrExpired = errors.New("storage signature has expired")
)

// DefaultSignedURLExpiry is how long a minted download link lives when the caller
// does not say. Short enough that a link pasted into a chat expires before it is
// abused, long enough that a browser follows a redirect inside it.
const DefaultSignedURLExpiry = 2 * time.Minute

// MaxSignedURLExpiry is the ceiling a caller may ask for. Without it, "expires
// in" is a number a request supplies, and a request that supplies 100 years gets
// a capability that is not one.
const MaxSignedURLExpiry = 15 * time.Minute

// NormalizeExpiry clamps a requested expiry into (0, MaxSignedURLExpiry] and
// defaults a non-positive one. It returns the error rather than a number when
// the request is out of range by an order of magnitude, because silently
// shortening a 30-day request to 15 minutes is its own kind of surprise.
func NormalizeExpiry(requested time.Duration) (time.Duration, error) {
	if requested <= 0 {
		return DefaultSignedURLExpiry, nil
	}
	if requested > MaxSignedURLExpiry {
		return 0, errors.New("storage: requested expiry is longer than the maximum signed URL lifetime")
	}
	return requested, nil
}

// ValidKey is the single key rule, shared by both adapters so that "a key is
// valid" cannot mean one thing locally and another in a bucket.
//
// A key is a slash-separated relative path with no empty, "." or ".." component,
// no leading slash, and no control character. It is not a URL, so no escaping
// happens here; each adapter escapes at the boundary it needs to.
func ValidKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || len(key) > 1024 {
		return false
	}
	if strings.ContainsAny(key, "\\\x00") {
		return false
	}
	for _, character := range key {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	for _, component := range strings.Split(key, "/") {
		switch component {
		case "", ".", "..":
			return false
		}
	}
	return true
}
