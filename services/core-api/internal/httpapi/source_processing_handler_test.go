package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
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
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
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

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/name-judge/config"},
		{http.MethodGet, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/name-lexicon"},
		{http.MethodGet, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/review-queue"},
		{http.MethodGet, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/review-summary"},
		{http.MethodGet, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/history"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/reopen"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/region-preview"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/region"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/split"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/merge"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/review-context"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/candidate-preparation"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/review-context/seal"},
		{http.MethodGet, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/name-evidence-status"},
		{http.MethodGet, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/name-evaluation-contract"},
		{http.MethodPost, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/name-evaluation-registration"},
		{http.MethodGet, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/reading-capabilities"},
		{http.MethodPost, "/api/v1/source-files/00000000-0000-0000-0000-000000000001/suggestions/read"},
		{http.MethodGet, "/api/v1/name-reading-runs/00000000-0000-0000-0000-000000000001"},
		{http.MethodPost, "/api/v1/name-reading-runs/00000000-0000-0000-0000-000000000001/cancel"},
		{http.MethodPost, "/api/v1/name-reading-runs/00000000-0000-0000-0000-000000000001/resume"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/local-reread"},
		{http.MethodGet, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/local-readings"},
		{http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/judge"},
	} {
		recorder = httptest.NewRecorder()
		NewRouter(Dependencies{}).ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, strings.NewReader(`{"candidates":[]}`)))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d", route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestNameJudgeConfigIsOffByDefaultAndActiveOnlyWhenConfigured(t *testing.T) {
	fixture := newSourceUploadFixture(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/name-judge/config", nil)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: fixture.token})
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{DB: fixture.pool, SourceStorage: fixture.store}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("off config status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var config struct {
		Mode         string  `json:"mode"`
		ImageEnabled bool    `json:"imageEnabled"`
		Available    bool    `json:"available"`
		Sample       float64 `json:"sample"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &config); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if config.Mode != "off" || config.Available || config.ImageEnabled {
		t.Fatalf("off config = %+v", config)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/judge", strings.NewReader(`{"candidates":[{"value":"محمد","source":"reader","rank":1}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: fixture.token})
	recorder = httptest.NewRecorder()
	NewRouter(Dependencies{DB: fixture.pool, SourceStorage: fixture.store}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("off judge status = %d, want 503: %s", recorder.Code, recorder.Body.String())
	}

	router := NewRouter(Dependencies{
		DB:            fixture.pool,
		SourceStorage: fixture.store,
		NameJudge:     configNameJudge{},
		NameJudgeSettings: sourceprocessing.NameJudgeSettings{
			Mode:   "shadow",
			Sample: 0.3,
		},
	})
	request = httptest.NewRequest(http.MethodGet, "/api/v1/name-judge/config", nil)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: fixture.token})
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("shadow config status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &config); err != nil {
		t.Fatalf("decode shadow config: %v", err)
	}
	if config.Mode != "shadow" || !config.Available || config.ImageEnabled || config.Sample != 0.3 {
		t.Fatalf("shadow config = %+v", config)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/source-tree-suggestions/00000000-0000-0000-0000-000000000001/judge", strings.NewReader(`{"candidates":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: fixture.token})
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("malformed judge candidates status = %d, want 422: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "jev") || strings.Contains(recorder.Body.String(), "provider") {
		t.Fatalf("the 422 exposed provider internals: %s", recorder.Body.String())
	}
}

type configNameJudge struct{}

func (configNameJudge) Judge(context.Context, sourceprocessing.NameJudgeRequest) (sourceprocessing.NameJudgeResult, error) {
	return sourceprocessing.NameJudgeResult{}, sourceprocessing.ErrNameJudgeUnavailable
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
		{name: "png", filename: "page.png", contentType: "image/png", content: []byte("\x89PNG\r\n\x1a\n"), named: "image/png"},
		{name: "jpeg", filename: "page.jpg", contentType: "image/jpeg", content: []byte("\xFF\xD8\xFF\xE0"), named: "image/jpeg"},
		{name: "tiff", filename: "page.tiff", contentType: "image/tiff", content: []byte("II*\x00binary"), named: "image/tiff"},
		{name: "binary", filename: "dump.bin", contentType: "application/octet-stream", content: []byte{0x00, 0x01, 0x02, 0xff}, named: "application/octet-stream"},
		{name: "word", filename: "notes.doc", contentType: "application/msword", content: []byte("binary"), named: "application/msword"},
		{name: "mislabelled zip", filename: "report.txt", contentType: "text/plain", content: []byte("PK\x03\x04\x00\x00binary"), named: "application/zip"},
		{name: "undeclared zip", filename: "report.zip", content: []byte("PK\x03\x04\x00\x00binary"), named: "application/zip"},
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
			for _, supported := range []string{"text/*", "application/json", "application/xml", "application/pdf"} {
				if !strings.Contains(body.Error, supported) {
					t.Fatalf("error %q does not teach the supported format %q", body.Error, supported)
				}
			}
			if len(body.SupportedContentTypes) != len(sourceprocessing.SupportedContentTypes()) || body.SupportedContentTypes[0] != "text/*" {
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
		{"scan.pdf", "application/pdf", []byte("%PDF-1.7\n% upload boundary fixture")},
		{"chart.png", "image/png", testSourceImage(t, "png")},
		{"chart.jpg", "image/jpeg", testSourceImage(t, "jpeg")},
		{"chart.gif", "image/gif", testSourceImage(t, "gif")},
		{"chart", "", testSourceImage(t, "png")},
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

// TestSourceProcessingErrorMapping pins which failures are the caller's fault
// and which are ours. A worker mistake reaches the API as an unmapped error, so
// it has to answer 500: reported as 415 it would tell a caller their file is the
// problem when the worker was.
func TestSourceProcessingErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "format refusal", err: sourceprocessing.ErrUnsupportedContent, status: http.StatusUnsupportedMediaType, code: "unsupported_source_format"},
		{name: "declared refusal", err: sourceprocessing.ErrUnsupportedDocument, status: http.StatusUnsupportedMediaType, code: "unsupported_source_format"},
		{name: "invalid input", err: sourceprocessing.ErrValidation, status: http.StatusBadRequest},
		{name: "already decided", err: sourceprocessing.ErrSuggestionAlreadyDecided, status: http.StatusConflict},
		{name: "superseded geometry", err: sourceprocessing.ErrSuggestionSuperseded, status: http.StatusConflict},
		{name: "stale revision", err: sourceprocessing.ErrConflict, status: http.StatusConflict},
		{name: "no review access", err: sourceprocessing.ErrForbidden, status: http.StatusForbidden},
		{name: "missing occurrence", err: sourceprocessing.ErrSuggestionNotFound, status: http.StatusNotFound},
		{name: "worker mistake", err: errors.New("extractor is unavailable"), status: http.StatusInternalServerError},
		{name: "database down", err: sourceprocessing.ErrDatabaseUnavailable, status: http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeSourceProcessingError(recorder, testCase.err)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, testCase.status, recorder.Body.String())
			}
			var body struct {
				Error                 string   `json:"error"`
				Code                  string   `json:"code"`
				SupportedContentTypes []string `json:"supportedContentTypes"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error == "" {
				t.Fatal("the body carries no reason")
			}
			if testCase.code == "" {
				if body.Code != "" || body.SupportedContentTypes != nil {
					t.Fatalf("body = %s, want a plain error", recorder.Body.String())
				}
				return
			}
			if body.Code != testCase.code || len(body.SupportedContentTypes) != len(sourceprocessing.SupportedContentTypes()) {
				t.Fatalf("body = %s, want the format payload", recorder.Body.String())
			}
			for _, supported := range body.SupportedContentTypes {
				if supported == "image/png" {
					return
				}
			}
			t.Fatalf("body = %s, want the raster formats in the payload", recorder.Body.String())
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
	t.Cleanup(func() { testsupport.CleanupSyntheticActors(t, pool, fixture.userID) })
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
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE type = 'source_process' AND payload->>'source_file_id' = $1`, fileID).Scan(&jobs); err != nil {
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

// testSourceImage encodes a small real image in an accepted raster format. The
// upload boundary decodes the header, so the fixture is a file rather than a
// signature prefix.
func testSourceImage(t *testing.T, format string) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(30 + x*5), G: uint8(80 + y*6), B: 45, A: 255})
		}
	}
	var buffer bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buffer, canvas)
	case "jpeg":
		err = jpeg.Encode(&buffer, canvas, nil)
	case "gif":
		err = gif.Encode(&buffer, canvas, nil)
	default:
		t.Fatalf("no encoder for %q", format)
	}
	if err != nil {
		t.Fatalf("could not encode a %s fixture: %v", format, err)
	}
	return buffer.Bytes()
}

func TestReaderErrorsExposeStableCodesWithoutProviderText(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{sourceprocessing.ErrChartVisionPolicy, 403, "disabled_by_policy"},
		{sourceprocessing.ErrChartVisionSetup, 503, "not_configured"},
		{sourceprocessing.ErrChartVisionUnavailable, 503, "temporarily_unavailable"},
		{sourceprocessing.ErrChartVisionOutputInvalid, 422, "unreadable_crop"},
	} {
		recorder := httptest.NewRecorder()
		writeSourceProcessingError(recorder, test.err)
		var body map[string]string
		if json.Unmarshal(recorder.Body.Bytes(), &body) != nil || recorder.Code != test.status || body["code"] != test.code {
			t.Fatalf("error response=%d %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestNamePreparationHTTPReturnsAcceptedAndPersistedRun(t *testing.T) {
	fixture := newSourceUploadFixture(t)
	upload := fixture.upload(t, "tree.png", "image/png", testSourceImage(t, "png"))
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload=%d %s", upload.Code, upload.Body.String())
	}
	var uploaded sourceprocessing.FileView
	if err := json.Unmarshal(upload.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/source-files/"+uploaded.ID+"/suggestions/read", nil)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: fixture.token})
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("prepare=%d %s", response.Code, response.Body.String())
	}
	var run sourceprocessing.NameReadingRun
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run.Status != "queued" || run.ID == "" {
		t.Fatalf("run=%+v", run)
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v1/name-reading-runs/"+run.ID, nil)
	get.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: fixture.token})
	response = httptest.NewRecorder()
	fixture.router.ServeHTTP(response, get)
	if response.Code != http.StatusOK {
		t.Fatalf("persisted status=%d %s", response.Code, response.Body.String())
	}
}

func TestNameReviewHTTPFreezesPreparationSeparatelyFromFinalEvidence(t *testing.T) {
	f := newSourceUploadFixture(t)
	upload := f.upload(t, "names.png", "image/png", testSourceImage(t, "png"))
	if upload.Code != 201 {
		t.Fatal("upload failed")
	}
	var file sourceprocessing.FileView
	if json.Unmarshal(upload.Body.Bytes(), &file) != nil {
		t.Fatal("invalid upload")
	}
	id := uuid.NewString()
	_, err := f.pool.Exec(context.Background(), `INSERT INTO source_tree_suggestions(id,source_id,source_file_id,occurrence_id,page_number,region,suggested_text_ar,reading_method,reading_model) VALUES($1,$2,$3,'initial-http',1,'{"x":0.1,"y":0.1,"width":0.1,"height":0.1}','محمد','fixture','fixture')`, id, f.source, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: f.token})
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		return w
	}
	begin := call(http.MethodPost, "/api/v1/source-tree-suggestions/"+id+"/review-context", nil)
	if begin.Code != 200 {
		t.Fatalf("context %d %s", begin.Code, begin.Body.String())
	}
	var c sourceprocessing.NamePreparationContext
	if json.Unmarshal(begin.Body.Bytes(), &c) != nil {
		t.Fatal("invalid context")
	}
	snapshot := []sourceprocessing.SuggestionCandidateSnapshot{{Value: "محمد", Key: "محمد", CompactKey: "محمد", ParticleKey: "محمد", Sources: []string{"reader"}, Score: 100, Shown: true, Position: 1}}
	prepared := call(http.MethodPost, "/api/v1/source-tree-suggestions/"+id+"/candidate-preparation", sourceprocessing.PrepareNameCandidatesInput{ExposureID: c.ExposureID, PoolVersion: sourceprocessing.NamePoolVersion, Snapshot: snapshot})
	if prepared.Code != 202 {
		t.Fatalf("prepare %d %s", prepared.Code, prepared.Body.String())
	}
	var p sourceprocessing.NameCandidatePreparation
	_ = json.Unmarshal(prepared.Body.Bytes(), &p)
	saved := call(http.MethodPost, "/api/v1/source-tree-suggestions/"+id+"/decision", sourceprocessing.SuggestionDecision{Decision: "corrected", TextAR: "خالد", EvidenceSchemaVersion: sourceprocessing.NameEvidenceVersion, PreparationID: p.ID, PreparationSignature: p.Signature, InputMethod: "typed"})
	if saved.Code != http.StatusCreated {
		t.Fatalf("save %d %s", saved.Code, saved.Body.String())
	}
	late := call(http.MethodPost, "/api/v1/source-tree-suggestions/"+id+"/review-context", nil)
	if late.Code != 409 {
		t.Fatalf("late context status %d", late.Code)
	}
}
