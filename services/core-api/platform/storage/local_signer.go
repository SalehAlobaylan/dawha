package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LocalSigner mints and verifies the expiring, HMAC-signed URLs the local
// development adapter hands out in place of an S3 presigned URL.
//
// It exists so that "signed download access" is a property the local stack
// actually has, rather than a capability the development adapter refuses. A
// local store has no server of its own to serve a presigned URL, so the API
// serves it: the signed URL points back at this service's object route, and the
// route verifies the signature before it reads a byte.
//
// The properties that make it a capability rather than a hint:
//
//   - The signature covers the key AND the expiry, so neither can be edited.
//   - The comparison is constant time.
//   - The secret is never derived from anything guessable, and a development
//     secret that was not configured is a random value that dies with the
//     process, so every restart invalidates every link somebody copied out of a
//     terminal.
//   - A signature does not extend authority. Minting requires an authorization
//     decision upstream (see sourceprocessing.Service.SignedDownload); the
//     signature only says "this link was issued for this object, until then".
type LocalSigner struct {
	// BaseURL is the absolute externally reachable base of the API, including
	// any path prefix. No trailing slash.
	BaseURL string
	secret  []byte
	// now is the clock, injectable so a test can walk past an expiry without
	// sleeping.
	now func() time.Time
}

// ObjectPath is the route the signed URL points at. It is deliberately not under
// /api/v1: the object route is not part of the versioned API, it is a signed
// capability endpoint, and putting it in the versioned namespace would invite
// somebody to add a session check to it later and thereby make every signed link
// useless.
const ObjectPath = "/local-objects/"

// NewLocalSigner returns a signer for the given base URL and secret. A secret
// shorter than 32 bytes is refused: an HMAC key that short is a key somebody can
// brute force from one leaked URL, and the failure would be silent.
func NewLocalSigner(baseURL string, secret []byte) (*LocalSigner, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("%w: the local signer needs the API's external base URL", ErrNotConfigured)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: the local signer base URL must be absolute, got %q", ErrNotConfigured, baseURL)
	}
	if len(secret) < 32 {
		return nil, fmt.Errorf("%w: the local signing secret must be at least 32 bytes, got %d", ErrNotConfigured, len(secret))
	}
	copied := make([]byte, len(secret))
	copy(copied, secret)
	return &LocalSigner{BaseURL: trimmed, secret: copied, now: time.Now}, nil
}

// Sign returns the signed URL for a key, valid for the given duration.
//
// The key appears in the URL because there is nowhere else for it to be: an
// S3 presigned URL carries the key too. What never happens is an unauthorized
// caller learning the key - it is only ever disclosed inside a signature-bound
// URL whose minting was authorized.
func (s *LocalSigner) Sign(key string, expires time.Duration) (string, error) {
	if s == nil {
		return "", ErrSignerNotConfigured
	}
	if !ValidKey(key) {
		return "", ErrInvalidKey
	}
	expiry, err := NormalizeExpiry(expires)
	if err != nil {
		return "", err
	}
	deadline := s.now().Add(expiry).Unix()
	// A non-positive interval that rounds to "now" would produce a URL that is
	// born expired, which reads as a clock problem rather than a request problem.
	if deadline <= s.now().Unix() {
		deadline = s.now().Add(time.Second).Unix()
	}
	built := s.BaseURL + ObjectPath + escapeKeyPath(key) +
		"?expires=" + strconv.FormatInt(deadline, 10) +
		"&signature=" + s.signature(key, deadline)
	return built, nil
}

// Verify checks a key, an expiry and a signature presented by the object route.
//
// It is a pure function of its inputs and the secret: no state, no clock beyond
// `now`, no database. Everything the route decides about who may have the bytes
// was decided when the URL was minted.
func (s *LocalSigner) Verify(key, expiresRaw, signature string) error {
	if s == nil {
		return ErrSignerNotConfigured
	}
	if !ValidKey(key) {
		return ErrInvalidKey
	}
	deadline, err := strconv.ParseInt(strings.TrimSpace(expiresRaw), 10, 64)
	if err != nil {
		return fmt.Errorf("storage: the expiry is not a unix timestamp")
	}
	if s.now().Unix() > deadline {
		return ErrExpired
	}
	expected := s.signature(key, deadline)
	// Both sides are base64url of a 32-byte digest, so a length mismatch is a
	// malformed request rather than a wrong signature, and comparing lengths
	// first leaks nothing that a wrong signature would not.
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return errors.New("storage: the signature does not match")
	}
	return nil
}

func (s *LocalSigner) signature(key string, deadline int64) string {
	mac := hmac.New(sha256.New, s.secret)
	// The separator cannot appear in a valid key, so no pair of (key, deadline)
	// can be rearranged into a different pair with the same input.
	fmt.Fprintf(mac, "%s\n%d", key, deadline)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// escapeKeyPath escapes each key component while keeping the separators, so the
// key survives a round trip through a URL path. url.PathEscape escapes "/" too,
// which would turn the whole key into one segment and break the route.
func escapeKeyPath(key string) string {
	components := strings.Split(key, "/")
	for i, component := range components {
		components[i] = url.PathEscape(component)
	}
	return strings.Join(components, "/")
}
