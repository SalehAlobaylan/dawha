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
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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
	fixture := newLegacyFileFixture(t, "application/pdf", "legacy-source", []byte("%PDF-1.7\n%legacy-binary\x00\x01"))
	service := NewService(fixture.pool, fixture.store, jobs.NewService(fixture.pool), nil, NewTextExtractor())
	job := fixture.claim()

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

	failed, failErr := jobs.NewService(fixture.pool).Fail(context.Background(), fixture.jobID.String(), jobs.FailInput{WorkerID: fixture.workerID, LeaseToken: job.Lease.Token, Error: err.Error()})
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
	fixture := newLegacyFileFixture(t, "text/plain", "legacy-source", []byte("نص عربي\fصفحة ثانية"))
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
	service := NewService(fixture.pool, fixture.store, jobs.NewService(fixture.pool), nil, NewTextExtractor())
	processed := make([]processedPage, 0, len(pages))
	for _, page := range pages {
		processed = append(processed, processedPage{Page: page, Normalized: strings.TrimSpace(page.Text), Embedding: testEmbedding(), Model: "test-model"})
	}
	if err := service.persistProcessedPages(context.Background(), fixture.claim(), fixture.record(), processed); err != nil {
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
	t        *testing.T
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
	filename string
	content  []byte
	payload  json.RawMessage
}

func newLegacyFileFixture(t *testing.T, mimeType, filename string, content []byte) *legacyFileFixture {
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
		t:        t,
		pool:     pool,
		store:    store,
		userID:   uuid.New(),
		sourceID: uuid.New(),
		fileID:   uuid.New(),
		runID:    uuid.New(),
		jobID:    uuid.New(),
		workerID: "legacy-format-worker",
		mimeType: mimeType,
		filename: filename,
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'queued')
	`, fixture.fileID, fixture.sourceID, fixture.fileKey, filename, mimeType, len(content), hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	fixture.payload, err = json.Marshal(JobPayload{SourceID: fixture.sourceID.String(), SourceFileID: fixture.fileID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (id, type, payload, status, locked_by, lease_token, lease_expires_at, heartbeat_at, max_attempts)
		VALUES ($1, 'source_process', $2, 'running', $3, gen_random_uuid(), now() + interval '90 seconds', now(), 1)
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

// claim reads back the fixture's job row and hands it over as a real claim.
//
// The lease is read from the row rather than invented, because the whole point
// of the tests that use this is that the processor only writes when the row says
// it still may. A fixture that faked the token would let them pass without the
// fence being there.
func (f *legacyFileFixture) claim() Claim {
	f.t.Helper()
	var token string
	var expiresAt time.Time
	if err := f.pool.QueryRow(context.Background(), `SELECT lease_token::text, lease_expires_at FROM jobs WHERE id = $1`, f.jobID).Scan(&token, &expiresAt); err != nil {
		f.t.Fatalf("read the fixture job lease: %v", err)
	}
	if token == "" {
		f.t.Fatal("the fixture job has no lease token, so nothing downstream can be fenced")
	}
	deadline := expiresAt
	return Claim{
		Job: jobs.JobView{ID: f.jobID.String(), Type: SourceProcessJobType, Payload: f.payload, Status: "running", MaxAttempts: 1},
		Lease: jobs.Lease{
			JobID:     f.jobID.String(),
			WorkerID:  f.workerID,
			Token:     token,
			ExpiresAt: &deadline,
		},
	}
}

func (f *legacyFileFixture) record() sourceFileRecord {
	digest := sha256.Sum256(f.content)
	return sourceFileRecord{
		RunID:      f.runID,
		SourceID:   f.sourceID,
		FileID:     f.fileID,
		StorageKey: f.fileKey,
		MimeType:   f.mimeType,
		Filename:   f.filename,
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

// TestProcessReportsAnUnrecordedFormatRefusal is the stuck-state guard: when the
// database refuses to record the failure, the run and the file would otherwise
// keep looking unfinished while their job dies. The returned error has to say so
// rather than pass as a clean refusal, which the caller would record as handled.
func TestProcessReportsAnUnrecordedFormatRefusal(t *testing.T) {
	fixture := newLegacyFileFixture(t, "application/pdf", "unrecorded-marking.pdf", []byte("%PDF-1.7\n%legacy-binary\x00\x01"))
	refuseFailureMarking(t, fixture.pool, fixture.filename)
	service := NewService(fixture.pool, fixture.store, jobs.NewService(fixture.pool), nil, NewTextExtractor())
	job := fixture.claim()

	err := service.Process(context.Background(), job)
	if err == nil {
		t.Fatal("Process succeeded, want an error the caller cannot read as a clean refusal")
	}
	if errors.Is(err, ErrUnsupportedContent) || errors.Is(err, ErrUnsupportedDocument) {
		t.Fatalf("error = %v, want it kept apart from a clean format refusal", err)
	}
	var unsupported *UnsupportedContentError
	if errors.As(err, &unsupported) {
		t.Fatalf("error = %v, want it kept apart from a clean format refusal", err)
	}
	var unrecorded *unrecordedFailureError
	if !errors.As(err, &unrecorded) {
		t.Fatalf("error = %v, want an *unrecordedFailureError", err)
	}
	var database *pgconn.PgError
	if !errors.As(err, &database) {
		t.Fatalf("error = %v, want the database failure on the error chain", err)
	}
	if !errors.Is(unrecorded.Refusal(), ErrUnsupportedContent) {
		t.Fatalf("refusal = %v, want the original cause kept for the reader", unrecorded.Refusal())
	}
	for _, fragment := range []string{fixture.filename, "text/*", fixture.mimeType, "refused by test"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not mention %q", err.Error(), fragment)
		}
	}
	// The run is left untouched, which is exactly why the caller has to learn that
	// nothing was recorded: its job dies while it still reads as unfinished.
	var runStatus string
	if err := fixture.pool.QueryRow(context.Background(), `SELECT status FROM source_processing_runs WHERE id = $1`, fixture.runID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus == "failed" {
		t.Fatalf("run status = %s, want the refused marking left unrecorded", runStatus)
	}
}

// TestFailProcessingReturnsTheCauseWhenTheMarkingWorks keeps the normal path
// identical: a refusal that was recorded is returned unchanged.
func TestFailProcessingReturnsTheCauseWhenTheMarkingWorks(t *testing.T) {
	fixture := newLegacyFileFixture(t, "application/pdf", "legacy-source", []byte("%PDF-1.7\n%legacy-binary\x00\x01"))
	service := NewService(fixture.pool, fixture.store, jobs.NewService(fixture.pool), nil, NewTextExtractor())
	cause := unsupportedContent("a recorded refusal")
	if err := service.failProcessing(context.Background(), fixture.claim(), fixture.runID, fixture.fileID, cause); err != cause {
		t.Fatalf("failProcessing = %v, want the cause unchanged", err)
	}
	var runStatus, runError, fileStatus, fileError string
	if err := fixture.pool.QueryRow(context.Background(), `SELECT r.status, r.error, f.processing_status, f.processing_error FROM source_processing_runs r JOIN source_files f ON f.id = r.source_file_id WHERE r.id = $1`, fixture.runID).Scan(&runStatus, &runError, &fileStatus, &fileError); err != nil {
		t.Fatal(err)
	}
	if runStatus != "failed" || fileStatus != "failed" || runError != cause.Error() || fileError != cause.Error() {
		t.Fatalf("recorded = %s/%s %q %q, want failed/failed with the reason on both", runStatus, fileStatus, runError, fileError)
	}
}

// TestFailProcessingReportsAFailedMarkingOnTheFile covers the same guard from
// the other statement: the run is marked and the file marking is the one the
// database rejects, so the caller gets a database error on the chain rather than
// a refusal nobody wrote down. The two statements cannot be made to fail at once
// any more - failProcessing proves the claim first, and a claim cannot be proved
// without a working connection - so the half that can fail is failed on purpose.
func TestFailProcessingReportsAFailedMarkingOnTheFile(t *testing.T) {
	fixture := newLegacyFileFixture(t, "application/pdf", "legacy-source", []byte("%PDF-1.7\n%legacy-binary\x00\x01"))
	refuseFileMarking(t, fixture.pool, "will not record")
	service := NewService(fixture.pool, fixture.store, jobs.NewService(fixture.pool), nil, NewTextExtractor())
	cause := unsupportedContent("a refusal the file table will not record")

	err := service.failProcessing(context.Background(), fixture.claim(), fixture.runID, fixture.fileID, cause)
	if err == nil {
		t.Fatal("failProcessing returned nil, want the failure to be visible")
	}
	if errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("error = %v, want it kept apart from a clean format refusal", err)
	}
	var unrecorded *unrecordedFailureError
	if !errors.As(err, &unrecorded) {
		t.Fatalf("error = %v, want an *unrecordedFailureError", err)
	}
	if !errors.Is(unrecorded.Refusal(), ErrUnsupportedContent) {
		t.Fatalf("refusal = %v, want the original cause kept for the reader", unrecorded.Refusal())
	}
	var fileStatus string
	if err := fixture.pool.QueryRow(context.Background(), `SELECT processing_status FROM source_files WHERE id = $1`, fixture.fileID).Scan(&fileStatus); err != nil {
		t.Fatal(err)
	}
	if fileStatus == "failed" {
		t.Fatalf("file status = %s, want the refused marking left unrecorded", fileStatus)
	}
}

// TestFailProcessingRefusesToRecordWithoutALiveClaim is the fencing half of the
// same function. A worker that lost the lease does not get to decide the run
// failed, and the error says so in a shape the caller can act on: the lease loss
// is on the chain, the document's own failure is kept for the reader, and neither
// is reported as a recorded refusal.
func TestFailProcessingRefusesToRecordWithoutALiveClaim(t *testing.T) {
	fixture := newLegacyFileFixture(t, "application/pdf", "legacy-source", []byte("%PDF-1.7\n%legacy-binary\x00\x01"))
	service := NewService(fixture.pool, fixture.store, jobs.NewService(fixture.pool), nil, NewTextExtractor())
	claim := fixture.claim()
	// Somebody else owns the job now, under a live lease of their own.
	if _, err := fixture.pool.Exec(context.Background(), `UPDATE jobs SET lease_token = gen_random_uuid(), locked_by = 'plan006-other-worker' WHERE id = $1`, claim.Lease.JobID); err != nil {
		t.Fatal(err)
	}
	cause := unsupportedContent("a refusal this worker may not record")

	err := service.failProcessing(context.Background(), claim, fixture.runID, fixture.fileID, cause)
	if err == nil {
		t.Fatal("failProcessing recorded a failure for a claim it no longer holds")
	}
	if !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("error = %v, want the lease loss on the chain so the worker stops", err)
	}
	if errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("error = %v, want the document failure kept off the chain", err)
	}
	var refused leaseRefusal
	if !errors.As(err, &refused) {
		t.Fatalf("error = %v, want a leaseRefusal", err)
	}
	if !errors.Is(refused.Refusal(), ErrUnsupportedContent) {
		t.Fatalf("refusal = %v, want the original cause kept for the reader", refused.Refusal())
	}
	// Nothing was written: the run still reads as unfinished rather than as
	// failed by an attempt that was not allowed to judge it.
	var runStatus, fileStatus string
	if err := fixture.pool.QueryRow(context.Background(), `SELECT r.status, f.processing_status FROM source_processing_runs r JOIN source_files f ON f.id = r.source_file_id WHERE r.id = $1`, fixture.runID).Scan(&runStatus, &fileStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus == "failed" || fileStatus == "failed" {
		t.Fatalf("run/file = %s/%s, want both untouched by a worker without the claim", runStatus, fileStatus)
	}
}

// refuseFileMarking makes the database reject the failure update of one source
// file, and only that file, so a test can make the second of the two marking
// statements fail while the first succeeds.
func refuseFileMarking(t *testing.T, pool *pgxpool.Pool, marker string) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS plan006_refuse_file_marking ON source_files`); err != nil {
			t.Errorf("drop trigger: %v", err)
		}
		if _, err := pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS plan006_refuse_file_marking()`); err != nil {
			t.Errorf("drop function: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION plan006_refuse_file_marking() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.processing_error LIKE '%' || TG_ARGV[0] || '%' THEN
				RAISE EXCEPTION 'source file failure marking refused by test';
			END IF;
			RETURN NEW;
		END;
		$$;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER plan006_refuse_file_marking
		BEFORE UPDATE ON source_files
		FOR EACH ROW EXECUTE FUNCTION plan006_refuse_file_marking(%s);
	`, pgxQuoteLiteral(marker))); err != nil {
		t.Fatal(err)
	}
}

// refuseFailureMarking makes the database reject exactly one run update: the
// failure record of a run whose file name carries the marker. Every other run
// keeps updating normally, and the trigger is dropped when the test ends.
func refuseFailureMarking(t *testing.T, pool *pgxpool.Pool, marker string) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS plan003_refuse_failure_marking ON source_processing_runs`); err != nil {
			t.Errorf("drop trigger: %v", err)
		}
		if _, err := pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS plan003_refuse_failure_marking()`); err != nil {
			t.Errorf("drop function: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION plan003_refuse_failure_marking() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.error LIKE '%' || TG_ARGV[0] || '%' THEN
				RAISE EXCEPTION 'source processing failure marking refused by test';
			END IF;
			RETURN NEW;
		END;
		$$;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER plan003_refuse_failure_marking
		BEFORE UPDATE ON source_processing_runs
		FOR EACH ROW EXECUTE FUNCTION plan003_refuse_failure_marking(%s);
	`, pgxQuoteLiteral(marker))); err != nil {
		t.Fatal(err)
	}
}

// pgxQuoteLiteral keeps the trigger marker a SQL literal rather than a
// parameter, which a trigger function cannot take.
func pgxQuoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
