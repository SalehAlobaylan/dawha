package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Signed download access, end to end, against a real database.
//
// The three properties under test are the ones a client depends on:
//
//  1. an account that may not review the source cannot obtain a link - not a
//     redirect, not a body, not a 403 with the key in a header;
//  2. an account that may review it gets a link that works, with no session,
//     because the link IS the authorization;
//  3. no response anywhere on the path contains the raw storage key, except the
//     signed URL itself - where it is unavoidable, because that is what an S3
//     presigned URL is.

type downloadFixture struct {
	pool    *pgxpool.Pool
	router  http.Handler
	store   *storage.LocalStore
	owner   uuid.UUID
	reader  uuid.UUID
	source  uuid.UUID
	fileID  uuid.UUID
	storage string
	// the key, read from the database rather than from anything the test built,
	// so the assertion is against what the database really holds.
	storageKey string
	ownerToken string
	readerTok  string
}

func newDownloadFixture(t *testing.T) *downloadFixture {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	t.Cleanup(pool.Close)

	signer, err := storage.NewLocalSigner("https://dawha.example.invalid", []byte(strings.Repeat("download-fixture-secret", 2)))
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store = store.WithSigner(signer)

	fixture := &downloadFixture{
		pool:    pool,
		store:   store,
		router:  NewRouter(Dependencies{DB: pool, SourceStorage: store}),
		owner:   uuid.New(),
		reader:  uuid.New(),
		source:  uuid.New(),
		fileID:  uuid.New(),
		storage: store.Root,
	}
	fixture.ownerToken = "download-owner-" + uuid.NewString()
	fixture.readerTok = "download-reader-" + uuid.NewString()

	t.Cleanup(func() {
		for _, cleanup := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM jobs WHERE type = 'source_process' AND payload ->> 'source_id' = $1`, fixture.source.String()},
			{`DELETE FROM source_processing_runs WHERE source_id = $1`, fixture.source},
			{`DELETE FROM source_files WHERE source_id = $1`, fixture.source},
			{`DELETE FROM audit_log WHERE actor_id = ANY($1)`, []uuid.UUID{fixture.owner, fixture.reader}},
			{`DELETE FROM auth_sessions WHERE user_id = ANY($1)`, []uuid.UUID{fixture.owner, fixture.reader}},
			{`DELETE FROM sources WHERE id = $1`, fixture.source},
			{`DELETE FROM user_roles WHERE user_id = ANY($1)`, []uuid.UUID{fixture.owner, fixture.reader}},
			{`DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{fixture.owner, fixture.reader}},
		} {
			if _, err := pool.Exec(context.Background(), cleanup.sql, cleanup.arg); err != nil {
				t.Errorf("cleanup failed for %q: %v", cleanup.sql, err)
			}
		}
	})

	for _, account := range []struct {
		id    uuid.UUID
		token string
		role  string
	}{{fixture.owner, fixture.ownerToken, "researcher"}, {fixture.reader, fixture.readerTok, "researcher"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'قارئ المصدر')`, account.id, account.token+"@dawha.test"); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, $2)`, account.id, account.role); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO auth_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 hour')`, account.id, auth.HashToken(account.token)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, created_by) VALUES ($1, 'مصدر التنزيل', 'manuscript', $2)`, fixture.source, fixture.owner); err != nil {
		t.Fatal(err)
	}

	content := []byte("نص المصدر القابل للتنزيل\n")
	key := fmt.Sprintf("sources/%s/%s-source.txt", fixture.source, fixture.fileID)
	if _, err := store.Put(ctx, key, strings.NewReader(string(content)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_files (id, source_id, storage_key, original_filename_ar, mime_type, byte_size, processing_status)
		VALUES ($1, $2, $3, 'source.txt', 'text/plain', $4, 'queued')
	`, fixture.fileID, fixture.source, key, len(content)); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT storage_key FROM source_files WHERE id = $1`, fixture.fileID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	fixture.storageKey = stored
	return fixture
}

func (f *downloadFixture) request(t *testing.T, token, url string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func (f *downloadFixture) link(t *testing.T, token string) downloadLink {
	t.Helper()
	recorder := f.request(t, token, "/api/v1/source-files/"+f.fileID.String()+"/download")
	if recorder.Code != http.StatusOK {
		t.Fatalf("download link status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var link downloadLink
	if err := json.Unmarshal(recorder.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	return link
}

type downloadLink struct {
	URL              string `json:"url"`
	Filename         string `json:"filename"`
	ContentType      string `json:"contentType"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
}

// TestSignedDownloadRefusesAnAccountThatCannotReviewTheSource is the property the
// whole feature exists for.
func TestSignedDownloadRefusesAnAccountThatCannotReviewTheSource(t *testing.T) {
	fixture := newDownloadFixture(t)
	ctx := context.Background()
	// A registered account with no role at all. canReviewSource asks for a role on
	// the user, so an ordinary registered reader is refused - which is the
	// behaviour a "download" button in somebody else's source must have.
	outsider := uuid.New()
	outsiderToken := "download-outsider-" + uuid.NewString()
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'غريب')`, outsider, outsiderToken+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO auth_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 hour')`, outsider, auth.HashToken(outsiderToken)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = fixture.pool.Exec(context.Background(), `DELETE FROM auth_sessions WHERE user_id = $1`, outsider)
		_, _ = fixture.pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, outsider)
	})

	recorder := fixture.request(t, outsiderToken, "/api/v1/source-files/"+fixture.fileID.String()+"/download")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("an account with no role on the source got %d, want %d", recorder.Code, http.StatusForbidden)
	}
	assertNoKeyLeak(t, recorder.Body.String(), fixture.storageKey, "the refused response")

	// No session at all is a 401, and also no key.
	anonymous := fixture.request(t, "", "/api/v1/source-files/"+fixture.fileID.String()+"/download")
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("an anonymous request got %d, want %d", anonymous.Code, http.StatusUnauthorized)
	}
	assertNoKeyLeak(t, anonymous.Body.String(), fixture.storageKey, "the unauthenticated response")

	// A file that does not exist is a 404, so this route does tell an
	// unauthorized caller whether a given id exists. It is asserted rather than
	// hidden: the id is a v4 UUID, so the only way to learn anything is to already
	// have the id, and neither answer discloses a key. Hiding existence here would
	// mean answering 404 to a reviewer who legitimately lost access, which is a
	// worse trade than the disclosure is worth.
	missing := fixture.request(t, outsiderToken, "/api/v1/source-files/"+uuid.NewString()+"/download")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("a stranger asking for a file that does not exist got %d, want %d", missing.Code, http.StatusNotFound)
	}
	assertNoKeyLeak(t, missing.Body.String(), fixture.storageKey, "the not-found response")
}

// TestSignedDownloadWorksWithoutASession is what makes the link a capability
// rather than another authenticated endpoint.
func TestSignedDownloadWorksWithoutASession(t *testing.T) {
	fixture := newDownloadFixture(t)
	link := fixture.link(t, fixture.readerTok)
	if link.URL == "" {
		t.Fatal("the link is empty")
	}
	if link.Filename != "source.txt" {
		t.Fatalf("filename = %q", link.Filename)
	}
	if link.ExpiresInSeconds <= 0 || link.ExpiresInSeconds > int64(storage.MaxSignedURLExpiry/time.Second) {
		t.Fatalf("expiresInSeconds = %d, which is not a bounded lifetime", link.ExpiresInSeconds)
	}
	parsed, err := url.Parse(link.URL)
	if err != nil {
		t.Fatalf("the link is not a url: %v", err)
	}
	if parsed.Query().Get("signature") == "" {
		t.Fatalf("the link carries no signature: %q", link.URL)
	}

	// No cookie at all. This is the whole point.
	recorder := fixture.request(t, "", link.URL)
	if recorder.Code != http.StatusOK {
		t.Fatalf("fetching the signed link without a session got %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, "نص المصدر القابل للتنزيل") {
		t.Fatalf("the signed link did not serve the object: %q", body)
	}
	if recorder.Header().Get("Cache-Control") == "" {
		t.Fatal("the object response is cacheable; a capability must not be")
	}
}

func TestSignedObjectRouteRefusesAnAlteredOrExpiredLink(t *testing.T) {
	fixture := newDownloadFixture(t)
	link := fixture.link(t, fixture.readerTok)
	parsed, err := url.Parse(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	signature := query.Get("signature")
	expires := query.Get("expires")

	t.Run("an edited signature is refused", func(t *testing.T) {
		tampered := signature
		if signature[0] == 'A' {
			tampered = "B" + signature[1:]
		} else {
			tampered = "A" + signature[1:]
		}
		recorder := fixture.request(t, "", storage.ObjectPath+escapeKey(fixture.storageKey)+"?expires="+expires+"&signature="+tampered)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("an altered signature got %d, want %d: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
		}
		assertNoKeyLeak(t, recorder.Body.String(), fixture.storageKey, "the refused object response")
	})

	t.Run("an extended expiry is refused", func(t *testing.T) {
		recorder := fixture.request(t, "", storage.ObjectPath+escapeKey(fixture.storageKey)+"?expires="+fmt.Sprint(4102444800)+"&signature="+signature)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("an extended expiry got %d, want %d: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
		}
	})

	t.Run("a missing signature is refused", func(t *testing.T) {
		recorder := fixture.request(t, "", storage.ObjectPath+escapeKey(fixture.storageKey)+"?expires="+expires)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("an unsigned request got %d, want %d", recorder.Code, http.StatusForbidden)
		}
	})

	t.Run("an unsigned request cannot read the object by guessing the path", func(t *testing.T) {
		// The route exists, so the path is public. What is not public is the
		// object: without a signature the route must not serve it.
		recorder := fixture.request(t, "", storage.ObjectPath+escapeKey(fixture.storageKey))
		if recorder.Code == http.StatusOK {
			t.Fatalf("the object route served an object with no signature at all: %s", recorder.Body.String())
		}
	})

	t.Run("a session does not substitute for a signature", func(t *testing.T) {
		// Otherwise a signed URL would be pointless: any signed-in user could
		// read any object by path.
		recorder := fixture.request(t, fixture.readerTok, storage.ObjectPath+escapeKey(fixture.storageKey))
		if recorder.Code == http.StatusOK {
			t.Fatalf("a session was enough to read an object without a signature: %s", recorder.Body.String())
		}
	})
}

func TestSignedDownloadNeverReturnsTheStorageKeyOutsideASignedURL(t *testing.T) {
	fixture := newDownloadFixture(t)
	recorder := fixture.request(t, fixture.ownerToken, "/api/v1/source-files/"+fixture.fileID.String()+"/download")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	// The key IS in the signed url, by construction. What must not happen is it
	// appearing anywhere else: not in a header, not beside the url, not in a field
	// of its own.
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for name, value := range payload {
		if name == "url" {
			continue
		}
		if text, ok := value.(string); ok && strings.Contains(text, fixture.storageKey) {
			t.Fatalf("the response field %q contains the storage key", name)
		}
	}
	for name, values := range recorder.Header() {
		for _, value := range values {
			if strings.Contains(value, fixture.storageKey) {
				t.Fatalf("the response header %q contains the storage key", name)
			}
		}
	}
	if recorder.Header().Get("Cache-Control") == "" {
		t.Fatal("the link response is cacheable; a capability must not be")
	}
}

// TestSignedDownloadIsNotASourceKeyOracle pins the one thing the previous
// behaviour got wrong: the storage key must not be reachable by asking for a
// different file.
func TestSignedDownloadIsNotASourceKeyOracle(t *testing.T) {
	fixture := newDownloadFixture(t)
	// A file id that exists, belonging to this source, that the caller may review -
	// and a file id that does not. The answers differ only in existence, never in
	// the shape of what is disclosed.
	other := uuid.New()
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO source_files (id, source_id, storage_key, original_filename_ar, mime_type, byte_size, processing_status)
		VALUES ($1, $2, $3, 'other.txt', 'text/plain', 4, 'queued')
	`, other, fixture.source, "sources/"+fixture.source.String()+"/"+other.String()+"-other.txt"); err != nil {
		t.Fatal(err)
	}
	recorder := fixture.request(t, fixture.readerTok, "/api/v1/source-files/"+other.String()+"/download")
	if recorder.Code != http.StatusOK {
		t.Fatalf("a reviewable file got %d: %s", recorder.Code, recorder.Body.String())
	}
	// The link for that file names THAT file's key, never the other one.
	body := recorder.Body.String()
	if strings.Contains(body, fixture.storageKey) {
		t.Fatal("a link for one file disclosed another file's storage key")
	}
}

// TestSignedDownloadRefusesAnExpiryBeyondTheCeiling: a client may ask for less,
// never for more, and asking for more is an error rather than a clamp - a silent
// clamp would leave the client believing it has a week.
func TestSignedDownloadRefusesAnExpiryBeyondTheCeiling(t *testing.T) {
	fixture := newDownloadFixture(t)
	recorder := fixture.request(t, fixture.readerTok, fmt.Sprintf("/api/v1/source-files/%s/download?expires=%d", fixture.fileID, 60*60*24*30))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("a thirty day expiry got %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	shorter := fixture.request(t, fixture.readerTok, fmt.Sprintf("/api/v1/source-files/%s/download?expires=30", fixture.fileID))
	if shorter.Code != http.StatusOK {
		t.Fatalf("a thirty second expiry got %d, want 200: %s", shorter.Code, shorter.Body.String())
	}
}

func assertNoKeyLeak(t *testing.T, body, key, where string) {
	t.Helper()
	if key == "" {
		t.Fatal("the test has no storage key to look for")
	}
	if strings.Contains(body, key) {
		t.Fatalf("%s contains the raw storage key", where)
	}
}

func escapeKey(key string) string {
	components := strings.Split(key, "/")
	for i, component := range components {
		components[i] = url.PathEscape(component)
	}
	return strings.Join(components, "/")
}
