package sourceprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEntityReferencesFindSeededAlias(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	references, err := (&Service{Pool: pool}).entityReferences(context.Background(), "أبو بكر")
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 1 || references[0].Type != "person" || references[0].ID != "10000000-0000-0000-0000-000000000001" {
		t.Fatalf("unexpected references: %+v", references)
	}
	if len(references[0].Aliases) != 1 || references[0].Aliases[0] != "أبو بكر" {
		t.Fatalf("unexpected aliases: %+v", references[0].Aliases)
	}
}

func TestResolveEntityUsesSeededAlias(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	aiURL := os.Getenv("AI_RESEARCH_URL")
	if databaseURL == "" || aiURL == "" {
		t.Skip("DATABASE_URL and AI_RESEARCH_URL are required")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	service := &Service{Pool: pool, AI: ai.NewHTTPClient(aiURL)}
	link, err := service.resolveEntity(context.Background(), "أبو بكر")
	if err != nil {
		t.Fatal(err)
	}
	if link.Type != "person" || link.ID != "10000000-0000-0000-0000-000000000001" || link.Score != 0.95 {
		t.Fatalf("unexpected link: %+v", link)
	}
}

// TestLegacyQueuedBinaryFileFailsDeterministically is the migration coverage for
// files queued before the format contract tightened: the worker refuses them
// with a reason a reader can act on, the refusal is identical on every attempt,
// nothing is extracted, and neither the run, the file, nor the job can be read as
// a success.
func TestLegacyQueuedBinaryFileFailsDeterministically(t *testing.T) {
	fixture := newLegacyFileFixture(t, "application/pdf", []byte("%PDF-1.7\n%legacy-binary\x00\x01"))
	service := NewService(fixture.pool, fixture.store, jobs.NewService(fixture.pool), nil, NewTextExtractor())
	job := jobs.JobView{ID: fixture.jobID.String(), Type: SourceProcessJobType, Payload: fixture.payload, Status: "running", MaxAttempts: 1}

	err := service.Process(context.Background(), job)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("first attempt = %v, want ErrUnsupportedContent", err)
	}
	fixture.assertRefused(t, err.Error())

	// A retry of the same job fails the same way instead of drifting.
	retry := service.Process(context.Background(), job)
	if !errors.Is(retry, ErrUnsupportedContent) || retry.Error() != err.Error() {
		t.Fatalf("second attempt = %v, want the same refusal as %v", retry, err)
	}
	fixture.assertRefused(t, retry.Error())

	failed, failErr := jobs.NewService(fixture.pool).Fail(context.Background(), fixture.jobID.String(), jobs.FailInput{WorkerID: fixture.workerID, Error: err.Error()})
	if failErr != nil {
		t.Fatal(failErr)
	}
	if failed.Status == "succeeded" {
		t.Fatalf("job status = %s, want a terminal failure", failed.Status)
	}
	if !strings.Contains(failed.LastError, "text/*") {
		t.Fatalf("job last error %q does not teach the supported formats", failed.LastError)
	}
}

// TestPersistProcessedPagesKeepsPageAndLocatorProvenance is the other half of
// the worker path: an accepted text file still produces traceable passages.
func TestPersistProcessedPagesKeepsPageAndLocatorProvenance(t *testing.T) {
	fixture := newLegacyFileFixture(t, "text/plain", []byte("نص عربي\fصفحة ثانية"))
	pages, err := NewTextExtractor().Extract(context.Background(), ExtractInput{
		Reader:      strings.NewReader(string(fixture.content)),
		ContentType: "text/plain",
		Filename:    fixture.fileKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Number != 1 || pages[1].Number != 2 {
		t.Fatalf("unexpected extracted pages: %+v", pages)
	}
	service := &Service{Pool: fixture.pool}
	processed := make([]processedPage, 0, len(pages))
	for _, page := range pages {
		processed = append(processed, processedPage{Page: page, Normalized: strings.TrimSpace(page.Text), Embedding: testEmbedding(), Model: "test-model"})
	}
	if err := service.persistProcessedPages(context.Background(), fixture.record(), processed); err != nil {
		t.Fatal(err)
	}
	rows, err := fixture.pool.Query(context.Background(), `
		SELECT page_number, locator_ar, text_ar, start_offset, end_offset
		FROM source_passages WHERE source_file_id = $1 ORDER BY sequence_number
	`, fixture.fileID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		var pageNumber int32
		var locator, text string
		var start, end int
		if err := rows.Scan(&pageNumber, &locator, &text, &start, &end); err != nil {
			t.Fatal(err)
		}
		found++
		if pageNumber != int32(pages[found-1].Number) {
			t.Fatalf("page_number = %d, want %d", pageNumber, pages[found-1].Number)
		}
		if locator != fmt.Sprintf("صفحة %d", pages[found-1].Number) {
			t.Fatalf("locator = %q, want the page locator", locator)
		}
		if text != pages[found-1].Text || start != pages[found-1].StartOffset || end != pages[found-1].EndOffset {
			t.Fatalf("passage = %q [%d:%d], want %q [%d:%d]", text, start, end, pages[found-1].Text, pages[found-1].StartOffset, pages[found-1].EndOffset)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if found != len(pages) {
		t.Fatalf("passages = %d, want %d", found, len(pages))
	}
	var status, stage string
	if err := fixture.pool.QueryRow(context.Background(), `SELECT status, stage FROM source_processing_runs WHERE id = $1`, fixture.runID).Scan(&status, &stage); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || stage != "complete" {
		t.Fatalf("run = %s/%s, want succeeded/complete", status, stage)
	}
	var audited int
	if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'source_processing_completed' AND entity_id = $1`, fixture.fileID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Fatalf("completion audit rows = %d, want 1", audited)
	}
}

type legacyFileFixture struct {
	pool     *pgxpool.Pool
	store    *storage.LocalStore
	userID   uuid.UUID
	sourceID uuid.UUID
	fileID   uuid.UUID
	runID    uuid.UUID
	jobID    uuid.UUID
	workerID string
	fileKey  string
	mimeType string
	content  []byte
	payload  json.RawMessage
}

func newLegacyFileFixture(t *testing.T, mimeType string, content []byte) *legacyFileFixture {
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
	fixture := &legacyFileFixture{
		pool:     pool,
		store:    store,
		userID:   uuid.New(),
		sourceID: uuid.New(),
		fileID:   uuid.New(),
		runID:    uuid.New(),
		jobID:    uuid.New(),
		workerID: "legacy-format-worker",
		mimeType: mimeType,
		content:  content,
	}
	fixture.fileKey = "sources/" + fixture.sourceID.String() + "/" + fixture.fileID.String() + "-source"
	// Registered before the inserts so a failed setup cannot leak fixture rows.
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM jobs WHERE id = $1`, fixture.jobID},
			{`DELETE FROM source_passages WHERE source_file_id = $1`, fixture.fileID},
			{`DELETE FROM source_candidates WHERE source_file_id = $1`, fixture.fileID},
			{`DELETE FROM source_statements WHERE source_file_id = $1`, fixture.fileID},
			{`DELETE FROM source_processing_runs WHERE id = $1`, fixture.runID},
			{`DELETE FROM source_files WHERE id = $1`, fixture.fileID},
			{`DELETE FROM audit_log WHERE entity_id = $1`, fixture.fileID},
			{`DELETE FROM sources WHERE id = $1`, fixture.sourceID},
			{`DELETE FROM users WHERE id = $1`, fixture.userID},
		} {
			if _, err := pool.Exec(context.Background(), cleanup.sql, cleanup.arg); err != nil {
				t.Errorf("cleanup failed for %q: %v", cleanup.sql, err)
			}
		}
	})
	if _, err := store.Put(ctx, fixture.fileKey, strings.NewReader(string(content)), mimeType); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مصدر قديم')`, fixture.userID, "legacy-format-"+fixture.userID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, created_by) VALUES ($1, 'مصدر ثنائي قديم', 'archive_record', $2)`, fixture.sourceID, fixture.userID); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_files (id, source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256, processing_status)
		VALUES ($1, $2, $3, 'legacy-source', $4, $5, $6, 'queued')
	`, fixture.fileID, fixture.sourceID, fixture.fileKey, mimeType, len(content), hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	fixture.payload, err = json.Marshal(JobPayload{SourceID: fixture.sourceID.String(), SourceFileID: fixture.fileID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (id, type, payload, status, locked_by, max_attempts)
		VALUES ($1, 'source_process', $2, 'running', $3, 1)
	`, fixture.jobID, fixture.payload, fixture.workerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_processing_runs (id, source_id, source_file_id, job_id, status, stage)
		VALUES ($1, $2, $3, $4, 'queued', 'queued')
	`, fixture.runID, fixture.sourceID, fixture.fileID, fixture.jobID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f *legacyFileFixture) record() sourceFileRecord {
	digest := sha256.Sum256(f.content)
	return sourceFileRecord{
		RunID:      f.runID,
		SourceID:   f.sourceID,
		FileID:     f.fileID,
		StorageKey: f.fileKey,
		MimeType:   f.mimeType,
		Filename:   "legacy-source",
		Status:     "queued",
		RunStatus:  "queued",
		ByteSize:   int64(len(f.content)),
		Checksum:   hex.EncodeToString(digest[:]),
	}
}

// assertRefused pins the persisted outcome: a refusal is recorded on both the
// run and the file, names the refused format and the accepted ones, and leaves
// no passage behind.
func (f *legacyFileFixture) assertRefused(t *testing.T, message string) {
	t.Helper()
	ctx := context.Background()
	var runStatus, runStage, runError, fileStatus, fileError string
	if err := f.pool.QueryRow(ctx, `SELECT r.status, r.stage, r.error, f.processing_status, f.processing_error FROM source_processing_runs r JOIN source_files f ON f.id = r.source_file_id WHERE r.id = $1`, f.runID).Scan(&runStatus, &runStage, &runError, &fileStatus, &fileError); err != nil {
		t.Fatal(err)
	}
	if runStatus != "failed" || runStage != "failed" {
		t.Fatalf("run = %s/%s, want failed/failed", runStatus, runStage)
	}
	if fileStatus != "failed" {
		t.Fatalf("file status = %s, want failed", fileStatus)
	}
	for name, recorded := range map[string]string{"run error": runError, "file error": fileError} {
		if recorded != message {
			t.Fatalf("%s = %q, want %q", name, recorded, message)
		}
		for _, fragment := range []string{f.mimeType, "text/*", "application/json", "application/xml"} {
			if !strings.Contains(recorded, fragment) {
				t.Fatalf("%s %q does not mention %q", name, recorded, fragment)
			}
		}
	}
	var extracted int
	for _, check := range []struct {
		label string
		sql   string
	}{
		{"source_passages", `SELECT count(*) FROM source_passages WHERE source_file_id = $1`},
		{"source_candidates", `SELECT count(*) FROM source_candidates WHERE source_file_id = $1`},
		{"source_statements", `SELECT count(*) FROM source_statements WHERE source_file_id = $1`},
	} {
		if err := f.pool.QueryRow(ctx, check.sql, f.fileID).Scan(&extracted); err != nil {
			t.Fatal(err)
		}
		if extracted != 0 {
			t.Fatalf("%s rows = %d, want 0", check.label, extracted)
		}
	}
}

func testEmbedding() []float32 {
	values := make([]float32, EmbeddingDimensions)
	for index := range values {
		values[index] = float32(index+1) / float32(EmbeddingDimensions)
	}
	return values
}
