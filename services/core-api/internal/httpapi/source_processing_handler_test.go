package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSourceProcessingRoutesRequireAuthentication(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "source.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("نص عربي"))
	_ = writer.Close()
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/sources/00000000-0000-0000-0000-000000000001/files", &body)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(recorder, upload)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("upload status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/sources/00000000-0000-0000-0000-000000000001/processing", nil)
	recorder = httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("processing status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

// TestSourceUploadRefusesUnsupportedFormatsBeforeAnySideEffect pins the V1
// format contract at the HTTP boundary: a refused upload is refused before an
// object is stored, before a source_files row exists, and before a job is
// enqueued, and the answer tells the caller which formats to send instead.
func TestSourceUploadRefusesUnsupportedFormatsBeforeAnySideEffect(t *testing.T) {
	fixture := newSourceUploadFixture(t)
	cases := []struct {
		name        string
		filename    string
		contentType string
		content     []byte
		named       string
	}{
		{name: "pdf", filename: "scan.pdf", contentType: "application/pdf", content: []byte("%PDF-1.7\n%binary"), named: "application/pdf"},
		{name: "png", filename: "page.png", contentType: "image/png", content: []byte("\x89PNG\r\n\x1a\n"), named: "image/png"},
		{name: "jpeg", filename: "page.jpg", contentType: "image/jpeg", content: []byte("\xFF\xD8\xFF\xE0"), named: "image/jpeg"},
		{name: "tiff", filename: "page.tiff", contentType: "image/tiff", content: []byte("II*\x00binary"), named: "image/tiff"},
		{name: "binary", filename: "dump.bin", contentType: "application/octet-stream", content: []byte{0x00, 0x01, 0x02, 0xff}, named: "application/octet-stream"},
		{name: "word", filename: "notes.doc", contentType: "application/msword", content: []byte("binary"), named: "application/msword"},
		// The caller says text, the bytes say PDF: the claim is not trusted.
		{name: "mislabelled pdf", filename: "report.txt", contentType: "text/plain", content: []byte("%PDF-1.7\n%binary"), named: "application/pdf"},
		{name: "mislabelled zip", filename: "report.txt", contentType: "text/plain", content: []byte("PK\x03\x04\x00\x00binary"), named: "application/zip"},
		{name: "undeclared pdf", filename: "report.txt", content: []byte("%PDF-1.7\n%binary"), named: "application/pdf"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := fixture.upload(t, testCase.filename, testCase.contentType, testCase.content)
			if recorder.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusUnsupportedMediaType, recorder.Body.String())
			}
			var body struct {
				Error                 string   `json:"error"`
				Code                  string   `json:"code"`
				SupportedContentTypes []string `json:"supportedContentTypes"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %q: %v", recorder.Body.String(), err)
			}
			if body.Code != "unsupported_source_format" {
				t.Fatalf("code = %q, want unsupported_source_format", body.Code)
			}
			if !strings.Contains(body.Error, testCase.named) {
				t.Fatalf("error %q does not name the refused format %q", body.Error, testCase.named)
			}
			for _, supported := range []string{"text/*", "application/json", "application/xml"} {
				if !strings.Contains(body.Error, supported) {
					t.Fatalf("error %q does not teach the supported format %q", body.Error, supported)
				}
			}
			if len(body.SupportedContentTypes) != 3 || body.SupportedContentTypes[0] != "text/*" {
				t.Fatalf("supportedContentTypes = %v, want the shared matrix", body.SupportedContentTypes)
			}
			fixture.assertNoSideEffect(t)
		})
	}
}

// TestSourceUploadAcceptsEverySupportedFormat is the other half of the contract:
// every advertised format is stored, recorded, and queued.
func TestSourceUploadAcceptsEverySupportedFormat(t *testing.T) {
	cases := []struct {
		filename    string
		contentType string
		content     []byte
	}{
		{"notes.txt", "text/plain", []byte("نص عربي")},
		{"notes.txt", "text/plain; charset=utf-8", []byte("نص عربي")},
		{"notes.md", "text/markdown", []byte("# عنوان")},
		{"rows.csv", "text/csv", []byte("a,b\n1,2")},
		{"data.json", "application/json", []byte(`{"key":"قيمة"}`)},
		{"doc.xml", "application/xml", []byte("<doc>نص</doc>")},
		{"doc.xml", "text/xml", []byte("<doc>نص</doc>")},
		{"undeclared.txt", "", []byte("نص بلا نوع مصرح به")},
		{"undeclared.txt", "application/octet-stream", []byte("نص بلا نوع مصرح به")},
	}
	for _, testCase := range cases {
		t.Run(testCase.contentType+"/"+testCase.filename, func(t *testing.T) {
			fixture := newSourceUploadFixture(t)
			recorder := fixture.upload(t, testCase.filename, testCase.contentType, testCase.content)
			if recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
			}
			var view struct {
				ID               string `json:"id"`
				MimeType         string `json:"mimeType"`
				ProcessingStatus string `json:"processingStatus"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
				t.Fatal(err)
			}
			if view.ProcessingStatus != "queued" {
				t.Fatalf("processingStatus = %q, want queued", view.ProcessingStatus)
			}
			fixture.assertAccepted(t, view.ID)
		})
	}
}

type sourceUploadFixture struct {
	pool    *pgxpool.Pool
	store   *storage.LocalStore
	router  http.Handler
	userID  uuid.UUID
	source  uuid.UUID
	token   string
	storage string
}

func newSourceUploadFixture(t *testing.T) *sourceUploadFixture {
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
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := &sourceUploadFixture{
		pool:    pool,
		store:   store,
		router:  NewRouter(Dependencies{DB: pool, SourceStorage: store}),
		userID:  uuid.New(),
		source:  uuid.New(),
		token:   "source-upload-" + uuid.NewString(),
		storage: store.Root,
	}
	// Registered before the inserts so a failed setup cannot leak fixture rows.
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM jobs WHERE type = 'source_process' AND payload ->> 'source_id' = $1`, fixture.source.String()},
			{`DELETE FROM source_processing_runs WHERE source_id = $1`, fixture.source},
			{`DELETE FROM source_files WHERE source_id = $1`, fixture.source},
			{`DELETE FROM audit_log WHERE actor_id = $1`, fixture.userID},
			{`DELETE FROM auth_sessions WHERE user_id = $1`, fixture.userID},
			{`DELETE FROM sources WHERE id = $1`, fixture.source},
			{`DELETE FROM user_roles WHERE user_id = $1`, fixture.userID},
			{`DELETE FROM users WHERE id = $1`, fixture.userID},
		} {
			if _, err := pool.Exec(context.Background(), cleanup.sql, cleanup.arg); err != nil {
				t.Errorf("cleanup failed for %q: %v", cleanup.sql, err)
			}
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'رافع المصدر')`, fixture.userID, fixture.token+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, fixture.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 hour')`, fixture.userID, auth.HashToken(fixture.token)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, created_by) VALUES ($1, 'مصدر اختبار الصيغة', 'manuscript', $2)`, fixture.source, fixture.userID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f *sourceUploadFixture) upload(t *testing.T, filename, contentType string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sources/"+f.source.String()+"/files", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: f.token})
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func (f *sourceUploadFixture) assertNoSideEffect(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, check := range []struct {
		label string
		sql   string
	}{
		{"source_files row", `SELECT count(*) FROM source_files WHERE source_id = $1`},
		{"source_processing_runs row", `SELECT count(*) FROM source_processing_runs WHERE source_id = $1`},
		{"source_process job", `SELECT count(*) FROM jobs WHERE type = 'source_process' AND payload ->> 'source_id' = $1`},
	} {
		var count int
		if err := f.pool.QueryRow(ctx, check.sql, f.source).Scan(&count); err != nil {
			t.Fatalf("%s: %v", check.label, err)
		}
		if count != 0 {
			t.Fatalf("%s = %d, want 0", check.label, count)
		}
	}
	objects := 0
	err := filepath.WalkDir(f.storage, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			objects++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if objects != 0 {
		t.Fatalf("stored objects = %d, want 0", objects)
	}
}

func (f *sourceUploadFixture) assertAccepted(t *testing.T, fileID string) {
	t.Helper()
	ctx := context.Background()
	var mimeType, status string
	if err := f.pool.QueryRow(ctx, `SELECT mime_type, processing_status FROM source_files WHERE id = $1`, fileID).Scan(&mimeType, &status); err != nil {
		t.Fatalf("source_files row: %v", err)
	}
	if status != "queued" {
		t.Fatalf("processing_status = %q, want queued", status)
	}
	var jobs int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE type = 'source_process' AND idempotency_key = $1`, "source_process:"+fileID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", jobs)
	}
	var runs int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM source_processing_runs WHERE source_file_id = $1 AND status = 'queued'`, fileID).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("queued runs = %d, want 1", runs)
	}
	reader, _, err := f.store.Get(ctx, f.expectedKey(t, fileID))
	if err != nil {
		t.Fatalf("stored object: %v", err)
	}
	defer reader.Close()
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatal(err)
	}
}

func (f *sourceUploadFixture) expectedKey(t *testing.T, fileID string) string {
	t.Helper()
	var key string
	if err := f.pool.QueryRow(context.Background(), `SELECT storage_key FROM source_files WHERE id = $1`, fileID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "sources/"+f.source.String()+"/") {
		t.Fatalf("storage key %q is not scoped to the source", key)
	}
	return key
}
