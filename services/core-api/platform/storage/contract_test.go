package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

// The contract suite. ONE suite, run against every adapter, because a contract
// that is written twice is two contracts that agree until the day somebody edits
// one of them.
//
// `make t.RunContract` style helpers below give each assertion the adapter's
// name in the failure message, so a failure says "s3: ..." rather than leaving
// somebody to work out which store the stack was configured with.

// storeContract is the behaviour every adapter owes its callers. It is written
// as behaviour, not as implementation: nothing here knows that one adapter is a
// directory and another is a bucket.
func storeContract(t *testing.T, name string, newStore func(t *testing.T) Store) {
	t.Run(name+"/put, get and delete round-trip the bytes and the size", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		content := []byte("نص عربي\nwith a second line\n")
		object, err := store.Put(ctx, "sources/demo/file.txt", strings.NewReader(string(content)), "text/plain")
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		if object.Size != int64(len(content)) {
			t.Fatalf("put reported size %d, want %d - the upload boundary deletes the object when the two disagree", object.Size, len(content))
		}
		if object.Key != "sources/demo/file.txt" {
			t.Fatalf("put reported key %q", object.Key)
		}
		reader, metadata, err := store.Get(ctx, "sources/demo/file.txt")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		got, err := io.ReadAll(reader)
		if err != nil {
			_ = reader.Close()
			t.Fatalf("read: %v", err)
		}
		if err := reader.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if string(got) != string(content) {
			t.Fatalf("content = %q, want %q", got, content)
		}
		if metadata.Size != int64(len(content)) {
			t.Fatalf("get reported size %d, want %d", metadata.Size, len(content))
		}
		if err := store.Delete(ctx, "sources/demo/file.txt"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, _, err := store.Get(ctx, "sources/demo/file.txt"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("after delete, get returned %v, want ErrNotFound", err)
		}
	})

	t.Run(name+"/a missing object is ErrNotFound, not a transport error", func(t *testing.T) {
		store := newStore(t)
		if _, _, err := store.Get(context.Background(), "sources/demo/never-written.txt"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get of a missing object returned %v, want ErrNotFound", err)
		}
	})

	t.Run(name+"/deleting an object that is not there succeeds", func(t *testing.T) {
		store := newStore(t)
		if err := store.Delete(context.Background(), "sources/demo/never-written.txt"); err != nil {
			t.Fatalf("delete of a missing object returned %v, want nil: the upload path deletes on every failure path and must not be blocked by a missing object", err)
		}
	})

	t.Run(name+"/keys that escape the store are refused", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		refused := []string{
			"",
			"../escape.txt",
			"sources/../../escape.txt",
			"/absolute.txt",
			"sources//double.txt",
			"sources/./dot.txt",
			"sources/..",
			"sources/\x00null.txt",
		}
		for _, key := range refused {
			if _, err := store.Put(ctx, key, strings.NewReader("x"), "text/plain"); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Put(%q) = %v, want ErrInvalidKey", key, err)
			}
			if _, _, err := store.Get(ctx, key); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Get(%q) = %v, want ErrInvalidKey", key, err)
			}
			if err := store.Delete(ctx, key); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Delete(%q) = %v, want ErrInvalidKey", key, err)
			}
			if _, err := store.SignedURL(ctx, key, time.Minute); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("SignedURL(%q) = %v, want ErrInvalidKey", key, err)
			}
		}
	})

	t.Run(name+"/signed access is expiring and key-bound", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		if _, err := store.Put(ctx, "sources/demo/signed.txt", strings.NewReader("signed"), "text/plain"); err != nil {
			t.Fatalf("put: %v", err)
		}
		signed, err := store.SignedURL(ctx, "sources/demo/signed.txt", time.Minute)
		if err != nil {
			t.Fatalf("SignedURL: %v", err)
		}
		parsed, err := url.Parse(signed)
		if err != nil {
			t.Fatalf("the signed url does not parse: %v (%q)", err, signed)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			t.Fatalf("the signed url is not absolute: %q", signed)
		}
		if !strings.Contains(signed, "sources/demo/signed.txt") {
			t.Fatalf("the signed url does not name its object: %q", signed)
		}
		// A presigned URL is a capability. The things that would turn it into a
		// permanent address are an expiry that does not expire and a signature
		// that does not bind what it authorises, and both are asserted here rather
		// than described in a comment.
		if parsed.Query().Get("X-Amz-Expires") == "" && parsed.Query().Get("expires") == "" {
			t.Fatalf("the signed url carries no expiry: %q", signed)
		}
		// The signature binds the key.
		differentKey, err := store.SignedURL(ctx, "sources/demo/other.txt", time.Minute)
		if err != nil {
			t.Fatalf("SignedURL: %v", err)
		}
		if signatureFor(signed) == signatureFor(differentKey) {
			t.Fatalf("the signature does not bind the key: %q and %q share it", signed, differentKey)
		}
		// The signature binds the expiry. A signature that ignored the lifetime
		// could be replayed with a longer one and the ceiling would be a comment.
		longer, err := store.SignedURL(ctx, "sources/demo/signed.txt", 2*time.Minute)
		if err != nil {
			t.Fatalf("SignedURL: %v", err)
		}
		if signatureFor(signed) == signatureFor(longer) {
			t.Fatalf("the signature does not bind the expiry: a one minute and a two minute link over the same key share it")
		}
		// Two presigns of the same key and expiry agree, or differ only in the
		// time value they were signed at and the signature that covers it.
		//
		// It is NOT the case that they must be byte-identical, and this is the
		// second thing a live endpoint taught this suite. A signature covers a
		// clock reading: SigV4 signs X-Amz-Date, and the local signer's deadline
		// is a unix second, so two presigns a microsecond apart can straddle a
		// second and MUST differ. A contract that demanded byte-identical URLs
		// would have been a contract no real signer can satisfy - it would have
		// passed here forever and failed on the first MinIO run that crossed a
		// second boundary. TestMinIOPresignedURLIsCheckedByTheServer is where the
		// second boundary is proved against a server that verifies signatures,
		// rather than argued about here.
		other, err := store.SignedURL(ctx, "sources/demo/signed.txt", time.Minute)
		if err != nil {
			t.Fatalf("SignedURL: %v", err)
		}
		switch difference := presignDifference(signed, other); difference {
		case "identical":
		case "only the signed time and its signature":
		default:
			t.Fatalf("two presigns of the same key and expiry %s, so the signature is not a function of the key, the expiry and the moment it was signed", difference)
		}
	})

	t.Run(name+"/an expiry outside the ceiling is refused, not quietly shortened", func(t *testing.T) {
		store := newStore(t)
		if _, err := store.SignedURL(context.Background(), "sources/demo/file.txt", 30*24*time.Hour); err == nil {
			t.Fatalf("a thirty day signed url was accepted; a request that supplies its own expiry can otherwise mint a permanent address")
		}
	})

	t.Run(name+"/a non-positive expiry becomes the documented default", func(t *testing.T) {
		store := newStore(t)
		if _, err := store.SignedURL(context.Background(), "sources/demo/file.txt", 0); err != nil {
			t.Fatalf("SignedURL with no expiry: %v", err)
		}
		if _, err := store.SignedURL(context.Background(), "sources/demo/file.txt", -time.Minute); err != nil {
			t.Fatalf("SignedURL with a negative expiry: %v", err)
		}
	})
}

func signatureFor(signed string) string {
	parsed, err := url.Parse(signed)
	if err != nil {
		return signed
	}
	query := parsed.Query()
	if value := query.Get("X-Amz-Signature"); value != "" {
		return value
	}
	return query.Get("signature")
}

// signedTimeParameters are the query parameters whose values are a moment in time
// and are covered by the signature: SigV4 signs the second it signed at and the
// lifetime it was given, and the local signer signs the deadline it computed.
// Every one of them is derived from the clock, so every one of them is allowed to
// differ between two presigns of the same key and expiry.
var signedTimeParameters = []string{"X-Amz-Date", "X-Amz-Expires", "expires"}

// presignDifference reports how two presigns of the same object and expiry
// relate, in words the caller can put in a failure message.
//
// It returns "identical", "only the signed time and its signature" when the two
// URLs differ in nothing but a signed clock reading and the signature covering
// it, and otherwise a description of the parameters that differ. Anything in that
// third group is a signer that is putting something into the URL that the
// signature does not account for.
func presignDifference(first, second string) string {
	if first == second {
		return "identical"
	}
	firstURL, err := url.Parse(first)
	if err != nil {
		return fmt.Sprintf("do not parse (%v)", err)
	}
	secondURL, err := url.Parse(second)
	if err != nil {
		return fmt.Sprintf("do not parse (%v)", err)
	}
	firstQuery, secondQuery := firstURL.Query(), secondURL.Query()
	allowed := map[string]bool{"X-Amz-Signature": true, "signature": true}
	for _, name := range signedTimeParameters {
		allowed[name] = true
	}
	var unexpected []string
	for name := range firstQuery {
		if firstQuery.Get(name) == secondQuery.Get(name) {
			continue
		}
		if !allowed[name] {
			unexpected = append(unexpected, name)
		}
	}
	for name := range secondQuery {
		if _, inFirst := firstQuery[name]; !inFirst && !allowed[name] {
			unexpected = append(unexpected, name)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return "differ in " + strings.Join(unexpected, ", ")
	}
	return "only the signed time and its signature"
}

func TestLocalStoreContract(t *testing.T) {
	storeContract(t, "local", func(t *testing.T) Store {
		store, err := NewLocal(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		signer, err := NewLocalSigner("https://dawha.example.invalid", testSecret())
		if err != nil {
			t.Fatal(err)
		}
		return store.WithSigner(signer)
	})
}

func TestS3StoreContract(t *testing.T) {
	// The same suite, the same assertions, against the production adapter with a
	// test double underneath it. No bucket, no credentials, no endpoint, no
	// service: which is the point, because a contract test that needs a running
	// object store is a contract test nobody runs.
	storeContract(t, "s3", func(t *testing.T) Store {
		return newS3WithAPI("dawha-test-bucket", newStub())
	})
}

func testSecret() []byte {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte('a' + i%26)
	}
	return secret
}
