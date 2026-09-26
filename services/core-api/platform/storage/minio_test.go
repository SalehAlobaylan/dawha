package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// The MinIO case. It is the SAME contract suite - storeContract, the same
// function, the same assertions - run against a real S3 implementation instead of
// the double in s3_test.go.
//
// Why that is worth a second entry point rather than a second suite: the double
// can only reproduce S3's SHAPE. It can check that the adapter asked for the right
// key, the right size, the right expiry and that it translates a missing key. It
// cannot check that a SigV4 signature the AWS SDK produced is a signature a
// server accepts, because nothing on the other end verifies it. Every question
// this file asks that the double cannot is about the server's answer:
//
//   - does the presigned URL, fetched by a plain HTTP client with no SDK and no
//     credentials, return the bytes?
//   - does the same URL stop working when one character of the signature, the
//     key, or the expiry changes?
//   - does the bucket refuse an unsigned read, which is what makes the presigned
//     read mean anything?
//
// If the answer is yes for the last one, the first three are only meaningful
// because the fourth holds. That is why the unsigned read is asserted here and
// not assumed.
//
// The opt-in. STORAGE_TEST_ENDPOINT is the switch, in the AI_RESEARCH_URL
// pattern plan 006 established: unset, the case skips and names the variable;
// set, it runs. `make storage-up` starts the endpoint, and no other target in the
// Makefile sets the variable, which is what keeps `make verify` free of MinIO.
//
// The credentials below are the non-secret development defaults compose already
// uses for the database, and they are read from the environment so a differently
// configured local MinIO needs no code change. No real credential belongs in this
// file.

// minioDefaultRegion is MinIO's default region and the one region name a local
// MinIO validates a signature against. AWS has regions; this has one, and a
// mismatch is a SignatureDoesNotMatch rather than a helpful error, so the
// integration test fixes it rather than inventing a variable nobody sets.
const minioDefaultRegion = "us-east-1"

// minioTestConfig is the endpoint plus the four things a store needs beside it.
// Empty endpoint is the whole opt-in, so it is read first and separately.
type minioTestConfig struct {
	endpoint  string
	bucket    string
	accessKey string
	secretKey string
}

func minioConfigFromEnvironment(t *testing.T) (minioTestConfig, bool) {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("STORAGE_TEST_ENDPOINT"))
	if endpoint == "" {
		t.Skip("STORAGE_TEST_ENDPOINT is unset, so there is no S3 endpoint to verify the adapter against. `make storage-up` starts a local MinIO and is the only thing that sets it.")
	}
	config := minioTestConfig{
		endpoint:  endpoint,
		bucket:    envOr("STORAGE_TEST_BUCKET", "dawha-storage-test"),
		accessKey: envOr("STORAGE_TEST_ACCESS_KEY_ID", "dawha_local"),
		secretKey: envOr("STORAGE_TEST_SECRET_ACCESS_KEY", "dawha_local"),
	}
	return config, true
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// newMinIOStore builds the PRODUCTION constructor against the local endpoint.
// NewS3, not newS3WithAPI: the point is that the whole production path - the
// endpoint override, the static credential provider, path-style addressing - is
// what a deployment uses, so it is what gets verified.
func newMinIOStore(t *testing.T, config minioTestConfig) *S3Store {
	t.Helper()
	store, err := NewS3(context.Background(), S3Config{
		Bucket:   config.bucket,
		Region:   minioDefaultRegion,
		Endpoint: config.endpoint,
		// Path style is not a preference for MinIO: virtual-hosted addressing
		// needs a wildcard DNS record for a bucket name in a hostname, which
		// 127.0.0.1 and localhost cannot have.
		AccessKeyID:     config.accessKey,
		SecretAccessKey: config.secretKey,
		UsePathStyle:    true,
	})
	if err != nil {
		t.Fatalf("the s3 adapter refused the endpoint %q: %v", config.endpoint, err)
	}
	return store
}

func TestS3StoreContractAgainstMinIO(t *testing.T) {
	config, ok := minioConfigFromEnvironment(t)
	if !ok {
		return
	}
	store := newMinIOStore(t, config)
	t.Cleanup(func() { removeMinIOKeys(t, store) })

	storeContract(t, "minio", func(t *testing.T) Store {
		return store
	})
}

// minioContractKeys are the keys storeContract writes. They are listed here so a
// run against a persistent volume leaves the bucket as it found it, which is what
// makes `make storage-up` + the suite repeatable rather than accumulating a
// directory of demo objects nobody can explain later.
//
// The signed-access case presigns a key it never writes, so it is absent from the
// list: there is nothing to delete.
var minioContractKeys = []string{
	"sources/demo/file.txt",
	"sources/demo/signed.txt",
}

func removeMinIOKeys(t *testing.T, store *S3Store) {
	t.Helper()
	ctx := context.Background()
	for _, key := range minioContractKeys {
		if err := store.Delete(ctx, key); err != nil && !errors.Is(err, ErrNotFound) {
			t.Logf("cleanup: deleting %s returned %v", key, err)
		}
	}
}

// TestMinIOPresignedURLIsCheckedByTheServer is the part of this plan that a test
// double cannot do: hand the URL to a plain HTTP client that has no credentials
// and no SDK, and find out whether the SERVER verifies the signature.
//
// Every assertion below is a negative one on purpose. A presigned GET that
// returns 200 proves very little on its own - it would also return 200 against a
// bucket that lets anybody read everything. So the file first proves the bucket
// refuses an unsigned read, and then proves that the presigned read is authorised
// by the signature and by nothing else: alter one character of the signature, the
// key, or the expiry, and the same client that was just served has to be refused,
// with the server's own error code as evidence of which check failed.
func TestMinIOPresignedURLIsCheckedByTheServer(t *testing.T) {
	config, ok := minioConfigFromEnvironment(t)
	if !ok {
		return
	}
	store := newMinIOStore(t, config)
	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()

	const key = "sources/demo/presigned.txt"
	content := []byte("presigned over plain HTTP\n")
	if _, err := store.Put(ctx, key, strings.NewReader(string(content)), "text/plain"); err != nil {
		t.Fatalf("put: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(ctx, key); err != nil && !errors.Is(err, ErrNotFound) {
			t.Logf("cleanup: deleting %s returned %v", key, err)
		}
	})

	// The unsigned read. MinIO creates a bucket with no policy attached, so this
	// is AccessDenied, and it is the control for everything after it: if this ever
	// returns 200 the presigned read below proves nothing and this file has to
	// fail.
	unsigned := fmt.Sprintf("%s/%s/%s", strings.TrimRight(config.endpoint, "/"), config.bucket, key)
	response, body, err := fetch(client, unsigned)
	if err != nil {
		t.Fatalf("the unsigned request did not complete, so the control is unknown: %v", err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("an unsigned GET of the object returned %d, so this bucket is not private and the presigned read below would prove nothing", response.StatusCode)
	}
	t.Logf("an unsigned read is refused: %d %s", response.StatusCode, s3ErrorCode(body))

	// The presigned read, fetched by a client that signs nothing.
	signed, err := store.SignedURL(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("the presigned url does not parse: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		t.Fatalf("the presigned url is not absolute: %q", signed)
	}
	query := parsed.Query()
	if query.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		t.Fatalf("the presigned url does not name SigV4: %q", signed)
	}
	if query.Get("X-Amz-Date") == "" || query.Get("X-Amz-Expires") == "" || query.Get("X-Amz-Signature") == "" {
		t.Fatalf("the presigned url is missing part of the signature: %q", signed)
	}

	response, body, err = fetch(client, signed)
	if err != nil {
		t.Fatalf("fetching the presigned url: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the presigned GET returned %d, not 200: %s", response.StatusCode, body)
	}
	if string(body) != string(content) {
		t.Fatalf("the presigned GET returned %q, want %q", body, content)
	}
	if got := response.Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("the presigned GET returned content type %q, want the text/plain the upload declared", got)
	}

	// One character of the signature. The canonical request the signature covers
	// includes the query parameters other than the signature itself, so this is
	// the smallest possible edit that must invalidate it.
	tampered := replaceOneCharacter(signed, "X-Amz-Signature")
	assertRefused(t, client, tampered, "SignatureDoesNotMatch", "a tampered signature")

	// The key. A signature that does not bind the object is a signature over
	// nothing, and this is the check that says so: the expiry and the credentials
	// are untouched and only the path changed.
	assertRefused(t, client, strings.Replace(signed, key, key+"-other", 1), "SignatureDoesNotMatch", "a key the signature was not issued for")

	// The signing second is inside the signature, and this is where that is proved
	// rather than assumed - twice, because either half alone is weak.
	//
	// First: two presigns of the same key and expiry, more than a second apart,
	// are NOT the same URL. A span longer than a second must cross a second
	// boundary, so this cannot be a coincidence, and the difference is confined to
	// the date and the signature. This is the property the double cannot have, and
	// it is why contract_test.go's determinism assertion is stated over a signing
	// second instead of over the whole URL.
	first, err := store.SignedURL(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	second, err := store.SignedURL(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if difference := presignDifference(first, second); difference != "only the signed time and its signature" {
		t.Fatalf("two presigns of the same key and expiry 1.1s apart are %s, want a difference confined to the signing second and its signature", difference)
	}
	if signatureFor(first) == signatureFor(second) {
		t.Fatal("the signature is identical in two different signing seconds, so the moment of signing is not covered by it")
	}
	t.Logf("the signing second is inside the signature: %s then %s", presignQueryValue(first, "X-Amz-Date"), presignQueryValue(second, "X-Amz-Date"))

	// Second: a URL whose X-Amz-Date has been moved by one second - still well
	// inside any reasonable clock-skew window - is refused. A server that ignored
	// the date would answer 200 here, and the pair above would be a curiosity
	// rather than a proof.
	assertRefused(t, client, shiftAmzDate(signed, 1), "SignatureDoesNotMatch", "a signing second that was not the one signed")

	// The expiry. Rewriting the signed expiry is not the same as waiting for it,
	// and both are asserted: this one proves the expiry is covered by the
	// signature, the next proves the server enforces the value it covers.
	assertRefused(t, client, replaceQueryValue(signed, "X-Amz-Expires", "86400"), "SignatureDoesNotMatch", "a rewritten expiry")

	// The expiry, enforced. A one second link, used inside the second and again
	// after it, so the refusal comes from the clock and not from an edit.
	short, err := store.SignedURL(ctx, key, time.Second)
	if err != nil {
		t.Fatalf("SignedURL at one second: %v", err)
	}
	response, _, err = fetch(client, short)
	if err != nil {
		t.Fatalf("fetching the one second presigned url: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("a one second presigned url was already refused (%d); the expiry test below would prove nothing", response.StatusCode)
	}
	time.Sleep(2 * time.Second)
	response, body, err = fetch(client, short)
	if err != nil {
		t.Fatalf("re-fetching the one second presigned url: %v", err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("a presigned url fetched %s after its one second expiry returned %d, not 403: a capability that does not expire is a public address", 2*time.Second, response.StatusCode)
	}
	t.Logf("an expired presigned url is refused: %d %s", response.StatusCode, s3ErrorCode(body))
}

// assertRefused is the negative case every tampering test shares: the server has
// to say no, and it has to say which check said no. The code is asserted rather
// than the status alone because a 403 with the wrong code is a different story - a
// refused key is not a refused signature, and conflating them is how a broken
// presign gets reported as working.
func assertRefused(t *testing.T, client *http.Client, signed, wantCode, what string) {
	t.Helper()
	response, body, err := fetch(client, signed)
	if err != nil {
		t.Fatalf("%s: the request did not complete: %v", what, err)
	}
	if response.StatusCode == http.StatusOK {
		t.Fatalf("%s: the server returned 200 and the bytes, so the signature is not being verified", what)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("%s: the server returned %d, want 403: %s", what, response.StatusCode, body)
	}
	if code := s3ErrorCode(body); code != wantCode {
		t.Fatalf("%s: the server answered %d %s, want %s: a refusal for a different reason is not a proof of this one", what, response.StatusCode, code, wantCode)
	}
}

// fetch is a deliberately plain client: no SDK, no signer, no retry, no
// credentials. Anything that authorises the request has to be in the URL.
func fetch(client *http.Client, target string) (*http.Response, []byte, error) {
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	return response, body, nil
}

// s3ErrorCode pulls the <Code> out of an S3 error document. The SDK's typed
// errors are not available to a plain client, and the code is the part that says
// which check refused.
func s3ErrorCode(body []byte) string {
	document := string(body)
	open := strings.Index(document, "<Code>")
	if open < 0 {
		return ""
	}
	rest := document[open+len("<Code>"):]
	closeAt := strings.Index(rest, "</Code>")
	if closeAt < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:closeAt])
}

// replaceOneCharacter changes the last character of a query parameter's value.
// The last character is deliberate: it is hex, so the result is still a
// well-formed signature that simply no longer matches the canonical request.
func replaceOneCharacter(signed, parameter string) string {
	parsed, err := url.Parse(signed)
	if err != nil {
		return signed
	}
	query := parsed.Query()
	value := query.Get(parameter)
	if value == "" {
		return signed
	}
	replacement := byte('0')
	if value[len(value)-1] == '0' {
		replacement = '1'
	}
	query.Set(parameter, value[:len(value)-1]+string(replacement))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// replaceQueryValue rewrites a signed query parameter, keeping the URL valid.
func replaceQueryValue(signed, parameter, value string) string {
	parsed, err := url.Parse(signed)
	if err != nil {
		return signed
	}
	query := parsed.Query()
	if query.Get(parameter) == "" {
		return signed
	}
	query.Set(parameter, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// shiftAmzDate moves the signing second by whole seconds, staying in the
// basic ISO 8601 basic format SigV4 uses (20060102T150405Z). A day boundary is
// handled by parsing rather than by string arithmetic, and an unparseable date is
// returned unchanged so the caller's assertion fails loudly instead of silently
// rewriting nothing.
func shiftAmzDate(signed string, seconds int) string {
	current := presignQueryValue(signed, "X-Amz-Date")
	moment, err := time.Parse("20060102T150405Z", current)
	if err != nil {
		return signed
	}
	return replaceQueryValue(signed, "X-Amz-Date", moment.Add(time.Duration(seconds)*time.Second).Format("20060102T150405Z"))
}

func presignQueryValue(signed, parameter string) string {
	parsed, err := url.Parse(signed)
	if err != nil {
		return ""
	}
	return parsed.Query().Get(parameter)
}
