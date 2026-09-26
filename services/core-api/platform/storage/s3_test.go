package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// stub is a test double for the four S3 calls the adapter makes.
//
// It is NOT an S3 emulator, and it is not pretending to be one. It is a
// predictable record of what the adapter asked for, so the adapter's own logic -
// key rules, size accounting, error mapping, expiry clamping - is tested against
// something, without a bucket, a credential, an endpoint or a network. What
// remains the AWS SDK's responsibility is the wire protocol, and the argument for
// trusting it there is that it is the SDK, not that this file resembles it.
//
// What the stub DOES reproduce is the parts of S3's shape the adapter depends on:
// a presigned URL carries the object key and a signature and an expiry, the
// signature is derived from the key and the expiry, and a missing key comes back
// as a typed NoSuchKey rather than a string.
type stub struct {
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
	// presignExpiry records what the adapter asked for, so a test can assert the
	// clamp rather than infer it from a URL.
	presignExpiry map[string]time.Duration
	putKeys       []string
	deleted       []string
}

func newStub() *stub {
	return &stub{
		objects:       map[string][]byte{},
		types:         map[string]string{},
		presignExpiry: map[string]time.Duration{},
	}
}

func (s *stub) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if in == nil || in.Bucket == nil || *in.Bucket == "" {
		return nil, fmt.Errorf("stub: PutObject without a bucket")
	}
	key := aws.ToString(in.Key)
	if key == "" {
		return nil, fmt.Errorf("stub: PutObject without a key")
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = body
	s.types[key] = aws.ToString(in.ContentType)
	s.putKeys = append(s.putKeys, key)
	return &s3.PutObjectOutput{}, nil
}

func (s *stub) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := aws.ToString(in.Key)
	s.mu.Lock()
	defer s.mu.Unlock()
	body, ok := s.objects[key]
	if !ok {
		// The typed error the adapter's mapS3Error translates. A stub that
		// returned a plain error would leave that translation untested.
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(s.types[key]),
	}, nil
}

func (s *stub) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	key := aws.ToString(in.Key)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	return &s3.DeleteObjectOutput{}, nil
}

func (s *stub) PresignGetObject(_ context.Context, in *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	key := aws.ToString(in.Key)
	options := s3.PresignOptions{Expires: DefaultSignedURLExpiry}
	for _, apply := range optFns {
		apply(&options)
	}
	s.mu.Lock()
	s.presignExpiry[key] = options.Expires
	s.mu.Unlock()
	// S3's presigned GET shape: the key in the path, a signature over the key and
	// the expiry, and the expiry itself in the query.
	signature := hmacHex([]byte("stub-secret"), key+"\n"+strconv.FormatInt(int64(options.Expires/time.Second), 10))
	query := url.Values{}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Expires", strconv.FormatInt(int64(options.Expires/time.Second), 10))
	query.Set("X-Amz-Signature", signature)
	return &v4.PresignedHTTPRequest{
		Method:       "GET",
		URL:          "https://s3.test.invalid/" + aws.ToString(in.Bucket) + "/" + key + "?" + encodeInOrder(query),
		SignedHeader: http.Header{},
	}, nil
}

func hmacHex(key []byte, message string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// encodeInOrder keeps the query in a stable order so two presigns of the same key
// and expiry produce byte-identical URLs, which is what the contract's
// determinism assertion depends on.
func encodeInOrder(query url.Values) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(query.Get(key)))
	}
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += "&"
		}
		out += part
	}
	return out
}

var _ s3API = (*stub)(nil)

// The adapter-specific tests. The contract above is shared; these are the
// properties that are true of one adapter and not the other.

func TestS3StoreReportsTheSizeTheUploadBoundaryCompares(t *testing.T) {
	// The upload path deletes the object unless Object.Size equals the number of
	// bytes it handed over. PutObjectOutput has no length, so an adapter that
	// reported zero would make every single upload fail in production and pass
	// locally - which is the exact shape of bug the contract suite exists to
	// prevent, and the reason this is asserted explicitly too.
	double := newStub()
	store := newS3WithAPI("bucket", double)
	content := []byte("0123456789")
	object, err := store.Put(context.Background(), "sources/demo/size.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if object.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", object.Size, len(content))
	}
}

func TestS3StoreReportsTheSizeOfANonSeekableBody(t *testing.T) {
	double := newStub()
	store := newS3WithAPI("bucket", double)
	// A pipe is the honest worst case: it cannot be seeked, so the only way to
	// know the length is to count what went past.
	reader, writer := io.Pipe()
	go func() {
		_, _ = writer.Write([]byte("streamed content"))
		_ = writer.Close()
	}()
	object, err := store.Put(context.Background(), "sources/demo/stream.txt", reader, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if object.Size != int64(len("streamed content")) {
		t.Fatalf("size = %d, want %d", object.Size, len("streamed content"))
	}
}

func TestS3StoreMapsAMissingKeyToErrNotFound(t *testing.T) {
	store := newS3WithAPI("bucket", newStub())
	_, _, err := store.Get(context.Background(), "sources/demo/missing.txt")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("get of a missing key returned %v, want ErrNotFound", err)
	}
}

func TestS3StoreAsksTheSdkForExactlyTheSharedCeiling(t *testing.T) {
	// MaxSignedURLExpiry is 15 minutes. The SDK would sign for up to a day; the
	// adapter asks for 15, so the ceiling is a property of this repository and
	// not of whichever cloud provider somebody deploys onto. Asserted on what the
	// adapter asks for, because the returned URL is the SDK's business.
	double := newStub()
	store := newS3WithAPI("bucket", double)
	if _, err := store.SignedURL(context.Background(), "sources/demo/file.txt", MaxSignedURLExpiry); err != nil {
		t.Fatalf("SignedURL at the ceiling: %v", err)
	}
	double.mu.Lock()
	asked := double.presignExpiry["sources/demo/file.txt"]
	double.mu.Unlock()
	if asked != MaxSignedURLExpiry {
		t.Fatalf("the adapter asked the sdk for %s, want the shared ceiling %s", asked, MaxSignedURLExpiry)
	}
	if _, err := store.SignedURL(context.Background(), "sources/demo/file.txt", 18*time.Hour); err == nil {
		t.Fatal("an eighteen hour signed url was accepted; the ceiling is the same one every adapter enforces")
	}
}

func TestS3StoreFailsClosedWithoutABucketOrARegion(t *testing.T) {
	// The whole point of NewS3's error handling: a deployment that selects the s3
	// driver and forgets a bucket gets a startup failure naming the bucket, not a
	// service that writes to a directory.
	cases := []struct {
		name   string
		config S3Config
		wants  string
	}{
		{"no bucket", S3Config{Region: "eu-west-1"}, "S3_BUCKET"},
		{"no region", S3Config{Bucket: "dawha"}, "S3_REGION"},
		{"half a credential", S3Config{Bucket: "dawha", Region: "eu-west-1", AccessKeyID: "only-the-id"}, "both S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewS3(context.Background(), testCase.config)
			if !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("NewS3 returned %v, want ErrNotConfigured", err)
			}
			if !strings.Contains(err.Error(), testCase.wants) {
				t.Fatalf("the error does not name what is missing: %v (wanted %q)", err, testCase.wants)
			}
		})
	}
}

func TestLocalSignerRefusesAnythingItDidNotIssue(t *testing.T) {
	signer, err := NewLocalSigner("https://dawha.example.invalid", testSecret())
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign("sources/demo/file.txt", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	expires := parsed.Query().Get("expires")
	signature := parsed.Query().Get("signature")

	if err := signer.Verify("sources/demo/file.txt", expires, signature); err != nil {
		t.Fatalf("the signer refuses its own signature: %v", err)
	}
	// Editing any one of the three inputs has to invalidate the other two, or the
	// signature is decoration.
	if err := signer.Verify("sources/demo/other.txt", expires, signature); err == nil {
		t.Fatal("a signature issued for one key verified another")
	}
	if err := signer.Verify("sources/demo/file.txt", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10), signature); err == nil {
		t.Fatal("a signature verified against an extended expiry")
	}
	tampered := "A" + signature[1:]
	if signature[0] == 'A' {
		tampered = "B" + signature[1:]
	}
	if err := signer.Verify("sources/demo/file.txt", expires, tampered); err == nil {
		t.Fatal("a tampered signature verified")
	}
	if err := signer.Verify("sources/demo/file.txt", "not-a-timestamp", signature); err == nil {
		t.Fatal("a non-numeric expiry verified")
	}
}

func TestLocalSignerRefusesAnExpiredLink(t *testing.T) {
	signer, err := NewLocalSigner("https://dawha.example.invalid", testSecret())
	if err != nil {
		t.Fatal(err)
	}
	// A clock the test controls, so this is not a test that sleeps for a minute.
	signer.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	signed, err := signer.Sign("sources/demo/file.txt", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(signed)
	if err := signer.Verify("sources/demo/file.txt", parsed.Query().Get("expires"), parsed.Query().Get("signature")); err != nil {
		t.Fatalf("the link is not valid on the second it was issued: %v", err)
	}
	signer.now = func() time.Time { return time.Unix(1_700_000_000+61, 0) }
	if err := signer.Verify("sources/demo/file.txt", parsed.Query().Get("expires"), parsed.Query().Get("signature")); !errors.Is(err, ErrExpired) {
		t.Fatalf("a link one second past its expiry returned %v, want ErrExpired", err)
	}
}

func TestLocalSignerRefusesAShortSecret(t *testing.T) {
	if _, err := NewLocalSigner("https://dawha.example.invalid", []byte("short")); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("a five byte signing secret was accepted: %v", err)
	}
	if _, err := NewLocalSigner("", testSecret()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("a signer with no base url was accepted: %v", err)
	}
	if _, err := NewLocalSigner("not-a-url", testSecret()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("a signer with a relative base url was accepted: %v", err)
	}
}

func TestConfigFromEnvironmentReadsTheDocumentedNamesOnly(t *testing.T) {
	values := map[string]string{
		"STORAGE_DRIVER":           "s3",
		"S3_BUCKET":                "dawha-production",
		"S3_REGION":                "eu-west-1",
		"S3_ENDPOINT":              "https://objects.example.invalid",
		"S3_ACCESS_KEY_ID":         "an-id",
		"S3_SECRET_ACCESS_KEY":     "a-secret",
		"S3_USE_PATH_STYLE":        "true",
		"STORAGE_SIGNING_BASE_URL": "https://dawha.example.invalid",
		"STORAGE_SIGNING_SECRET":   "0123456789abcdef0123456789abcdef",
		"SOURCE_STORAGE_DIR":       "/tmp/dawha-storage",
	}
	config := ConfigFromEnvironment(func(name string) string { return values[name] })
	if config.Driver != DriverS3 || config.S3.Bucket != "dawha-production" || config.S3.Region != "eu-west-1" {
		t.Fatalf("the s3 configuration was not read: %+v", config.S3)
	}
	if !config.S3.UsePathStyle {
		t.Fatal("S3_USE_PATH_STYLE=true was not read")
	}
	if string(config.SigningSecret) != values["STORAGE_SIGNING_SECRET"] {
		t.Fatal("the signing secret was not read")
	}
	empty := ConfigFromEnvironment(func(string) string { return "" })
	if empty.Driver != "" || empty.S3.Bucket != "" {
		t.Fatalf("an unset environment produced a configuration: %+v", empty)
	}
}

func TestNewResolvesTheDriverAndFailsClosed(t *testing.T) {
	t.Run("the default driver is local, with no configuration at all", func(t *testing.T) {
		store, err := New(context.Background(), Config{LocalRoot: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := LocalStoreFor(store); !ok {
			t.Fatalf("the default driver produced %T, want a local store", store)
		}
	})

	t.Run("the local driver with no directory is a configuration error", func(t *testing.T) {
		if _, err := New(context.Background(), Config{Driver: DriverLocal}); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("New = %v, want ErrNotConfigured", err)
		}
	})

	t.Run("an unknown driver is a configuration error, not a local fallback", func(t *testing.T) {
		// The failure this prevents: a typo in STORAGE_DRIVER quietly giving a
		// deployment a local store, and every upload going to a directory inside
		// the container.
		if _, err := New(context.Background(), Config{Driver: "gcs", LocalRoot: t.TempDir()}); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("New = %v, want ErrNotConfigured", err)
		}
	})

	t.Run("the s3 driver with nothing configured does not fall back to local", func(t *testing.T) {
		if _, err := New(context.Background(), Config{Driver: DriverS3, LocalRoot: t.TempDir()}); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("New = %v, want ErrNotConfigured", err)
		}
	})

	t.Run("a local store with no signing base url holds bytes and refuses links", func(t *testing.T) {
		store, err := New(context.Background(), Config{Driver: DriverLocal, LocalRoot: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Put(context.Background(), "sources/demo/file.txt", strings.NewReader("x"), "text/plain"); err != nil {
			t.Fatalf("put: %v", err)
		}
		if _, _, err := store.Get(context.Background(), "sources/demo/file.txt"); err != nil {
			t.Fatalf("get: %v", err)
		}
		if _, err := store.SignedURL(context.Background(), "sources/demo/file.txt", time.Minute); !errors.Is(err, ErrSignerNotConfigured) {
			t.Fatalf("SignedURL = %v, want ErrSignerNotConfigured", err)
		}
	})

	t.Run("a local store with a signing base url mints links", func(t *testing.T) {
		store, err := New(context.Background(), Config{
			Driver:         DriverLocal,
			LocalRoot:      t.TempDir(),
			SigningBaseURL: "https://dawha.example.invalid",
		})
		if err != nil {
			t.Fatal(err)
		}
		signed, err := store.SignedURL(context.Background(), "sources/demo/file.txt", time.Minute)
		if err != nil {
			t.Fatalf("SignedURL: %v", err)
		}
		if !strings.HasPrefix(signed, "https://dawha.example.invalid"+ObjectPath) {
			t.Fatalf("the signed url does not point at this service: %q", signed)
		}
	})
}
