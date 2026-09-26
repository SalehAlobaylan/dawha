package sourceprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The heartbeats of a long job. Extraction is the slowest thing in the platform
// - a page is three model calls - and it is exactly the work that used to be
// able to run past the window the queue considered a job alive. The three tests
// below are the three things that has to be true afterwards: the worker renews
// while it is busy, it finds out it lost the job before it writes anything, and
// what it wrote is never applied twice.

// TestAWorkerThatKeepsHeartbeatingFinishesAJobThatOutlivesItsLease is the
// ordinary case, and the one the fifteen-minute window used to get wrong. The
// lease handed out at claim time is deliberately shorter than the work, so the
// only way this finishes is if the worker kept proving it was alive.
func TestAWorkerThatKeepsHeartbeatingFinishesAJobThatOutlivesItsLease(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب المصدر البطيء")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	queue := jobs.NewService(fixture.Pool())
	// Ninety seconds of lease would be untestable, so the test says what it means
	// in one line instead: a short lease, a heartbeat that comes back faster, and
	// a provider slower than both.
	queue.LeaseDuration = 900 * time.Millisecond
	queue.HeartbeatInterval = 100 * time.Millisecond
	provider := &slowProvider{delay: 120 * time.Millisecond}
	service := NewService(fixture.Pool(), store, queue, provider, NewTextExtractor())
	ctx := fixture.Ctx()

	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "مخطوط " + fixture.Unique("s"),
		SourceType: "manuscript",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := service.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "سجل.txt",
		ContentType: "text/plain",
		// Four pages, three model calls each: comfortably longer than one lease.
		Content: []byte(strings.Join([]string{
			"قال ابن سعد: أبو بكر هو والد عبدالله.",
			"ويذكرNext the second page names the same father.",
			"وفي الصفحة التالية: هاجر سعد إلى الرياض.",
			"然后 الصفحة الأخيرة تكرر الاسم والقرابة.",
		}, "\f")),
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	claimedJob, err := queue.Claim(ctx, jobs.ClaimInput{WorkerID: "p006-slow-worker", Type: SourceProcessJobType})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	job := claimedJob
	claim := Claim{Job: job, Lease: job.Lease("p006-slow-worker")}
	// A snapshot of the claim's own heartbeat, taken while the work is still
	// running, is the direct evidence that a renewal happened. It cannot be read
	// off the finished job, because completing releases the claim and with it the
	// fencing state.
	var heartbeatDuringWork *time.Time
	provider.onCall = func(call int) {
		if call != 4 {
			return
		}
		var beat time.Time
		if err := fixture.QueryRow(`SELECT heartbeat_at FROM jobs WHERE id = $1`, job.ID).Scan(&beat); err == nil {
			heartbeatDuringWork = &beat
		}
	}
	started := time.Now()
	if err := service.Process(ctx, claim); err != nil {
		t.Fatalf("process: %v", err)
	}
	elapsed := time.Since(started)
	if _, err := queue.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: "p006-slow-worker", LeaseToken: claim.Lease.Token}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// The work really did outlast the lease it was claimed under, so the
	// heartbeat is what carried it, not a generous deadline.
	if provider.calls() < 12 {
		t.Fatalf("model calls = %d, want the whole document, so the job outlasted one lease", provider.calls())
	}
	if claimedJob.LockedAt == nil {
		t.Fatal("the claim came with no lock timestamp")
	}
	if elapsed <= queue.LeaseDuration {
		t.Fatalf("the work took %s, which is not longer than the %s lease, so this test proved nothing", elapsed, queue.LeaseDuration)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE source_file_id = $1`, file.ID); got != 4 {
		t.Fatalf("passages = %d, want one per page", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_processing_runs WHERE source_file_id = $1 AND status = 'succeeded' AND stage = 'complete'`, file.ID); got != 1 {
		t.Fatalf("succeeded runs = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_candidates WHERE source_file_id = $1`, file.ID); got != 8 {
		t.Fatalf("candidates = %d, want one entity and one claim per page", got)
	}
	if heartbeatDuringWork == nil {
		t.Fatal("the job row had no heartbeat while the work was running, so the claim was never renewed")
	}
	if !heartbeatDuringWork.After(*claimedJob.LockedAt) {
		t.Fatalf("heartbeat %s, want it later than the claim's own lock at %s", heartbeatDuringWork, claimedJob.LockedAt)
	}
}

// TestAWorkerThatLostItsLeaseCommitsNothing is the fencing case, and the one the
// plan's stop conditions are about. The lease is pulled out from under a worker
// that is midway through a document; the attempt must end as a refusal that
// leaves no passage, no candidate, and no success behind - and the outcome has to
// be visible as a refusal rather than as a job that quietly succeeded.
func TestAWorkerThatLostItsLeaseCommitsNothing(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب المصدر المسروق")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	queue := jobs.NewService(fixture.Pool())
	// A long lease, so the loss cannot be a timeout: the row is changed by
	// somebody else, which is what a reclaim actually looks like.
	queue.LeaseDuration = 10 * time.Minute
	queue.HeartbeatInterval = 50 * time.Millisecond
	provider := &slowProvider{delay: 60 * time.Millisecond}
	service := NewService(fixture.Pool(), store, queue, provider, NewTextExtractor())
	ctx := fixture.Ctx()

	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "مخطوط " + fixture.Unique("s"),
		SourceType: "manuscript",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := service.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "سجل.txt",
		ContentType: "text/plain",
		Content:     []byte(strings.Join([]string{"صفحة أولى عن أبي بكر.", "صفحة ثانية عن هجرة سعد.", "صفحة ثالثة عن نسبه.", "صفحة رابعة عن مسكنه."}, "\f")),
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	job, err := queue.Claim(ctx, jobs.ClaimInput{WorkerID: "p006-displaced-worker", Type: SourceProcessJobType})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claim := Claim{Job: job, Lease: job.Lease("p006-displaced-worker")}
	// The first model call is the signal that the worker is inside the work, and
	// the takeover happens right after it, which is the worst possible moment and
	// the one worth testing.
	provider.onFirstCall(func() {
		fixture.Exec(`UPDATE jobs SET lease_token = gen_random_uuid(), locked_by = 'p006-taker' WHERE id = $1`, job.ID)
	})

	err = service.Process(ctx, claim)
	if err == nil {
		t.Fatal("a worker that lost the job reported success")
	}
	if !errors.Is(err, jobs.ErrLeaseLost) && !errors.Is(err, jobs.ErrForbidden) {
		t.Fatalf("error = %v, want the lease loss reported rather than a document failure", err)
	}
	// The refusal is legible as a refusal, and it is a lease refusal rather than a
	// document failure - the worker is not being told its document is bad, it is
	// being told the job is no longer its own.
	if !errors.Is(err, jobs.ErrForbidden) && !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("error = %v, want a lease refusal", err)
	}
	var unrecorded *leaseRefusal
	if errors.As(err, &unrecorded) {
		t.Fatalf("error = %v, want the plain lease loss; there was no document failure to go unrecorded here", err)
	}

	// Nothing landed. No passage, no candidate, no statement, and neither the run
	// nor the file reads as a success or as a failure this attempt decided.
	for name, count := range map[string]int{
		"source_passages":   fixture.Count(`SELECT count(*) FROM source_passages WHERE source_file_id = $1`, file.ID),
		"source_candidates": fixture.Count(`SELECT count(*) FROM source_candidates WHERE source_file_id = $1`, file.ID),
		"source_statements": fixture.Count(`SELECT count(*) FROM source_statements WHERE source_file_id = $1`, file.ID),
		"completed audits":  fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'source_processing_completed' AND entity_id = $1`, file.ID),
	} {
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0: a worker without the claim wrote something", name, count)
		}
	}
	var runStatus, runStage string
	var fileStatus string
	if err := fixture.QueryRow(`SELECT r.status, r.stage, f.processing_status FROM source_processing_runs r JOIN source_files f ON f.id = r.source_file_id WHERE r.source_file_id = $1`, file.ID).Scan(&runStatus, &runStage, &fileStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus == "succeeded" || runStage == "complete" || fileStatus == "succeeded" {
		t.Fatalf("run/file = %s/%s %s, want nothing this attempt did readable as a success", runStatus, runStage, fileStatus)
	}
	// The job is still the new owner's to finish, with the same attempt count: the
	// displaced worker neither advanced nor killed it.
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE id = $1 AND status = 'running' AND locked_by = 'p006-taker' AND attempts = 0`, job.ID); got != 1 {
		t.Fatal("the displaced worker changed the job the new owner is holding")
	}
	if _, err := queue.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: "p006-displaced-worker", LeaseToken: claim.Lease.Token}); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatalf("the displaced worker completing its old claim = %v, want a refusal", err)
	}
}

// TestAWorkerThatLostItsLeaseMidwayLeavesTheRunForTheNextAttempt pins what a
// refusal must not do to the attempt counter, because that is what decides
// whether the job is retried or killed. A worker that lost its claim and reported
// a failure anyway would spend one of the job's three lives on a failure it was
// not entitled to record, and three of those kill a document that was fine.
func TestAWorkerThatLostItsLeaseMidwayLeavesTheRunForTheNextAttempt(t *testing.T) {
	fixture := testsupport.New(t)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	queue := jobs.NewService(fixture.Pool())
	queue.LeaseDuration = 10 * time.Minute
	queue.HeartbeatInterval = 50 * time.Millisecond
	legacy := newQueuedTextFile(t, fixture, store, "late.txt", []byte("نص عربي\fصفحة ثانية"))

	job, err := queue.Claim(fixture.Ctx(), jobs.ClaimInput{WorkerID: legacy.workerID, Type: SourceProcessJobType})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claim := Claim{Job: job, Lease: job.Lease(legacy.workerID)}
	// The same document, extracted and ready, so the only thing between this
	// attempt and a write is the fence.
	pages, err := NewTextExtractor().Extract(fixture.Ctx(), ExtractInput{
		Reader:      strings.NewReader("نص عربي\fصفحة ثانية"),
		ContentType: "text/plain",
		Filename:    legacy.fileKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	processed := make([]processedPage, 0, len(pages))
	for _, page := range pages {
		processed = append(processed, processedPage{Page: page, Normalized: strings.TrimSpace(page.Text), Embedding: testEmbedding(), Model: "p006-stub"})
	}
	fixture.Exec(`UPDATE jobs SET lease_token = gen_random_uuid(), locked_by = 'p006-taker' WHERE id = $1`, job.ID)

	service := NewService(legacy.pool, store, queue, nil, NewTextExtractor())
	err = service.persistProcessedPages(fixture.Ctx(), claim, legacy.record(), processed)
	if !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("persist under a lost claim = %v, want ErrLeaseLost", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE source_file_id = $1`, legacy.fileID); got != 0 {
		t.Fatalf("passages = %d, want 0: the fenced write applied anyway", got)
	}
	if _, err := queue.Fail(fixture.Ctx(), job.ID, jobs.FailInput{WorkerID: legacy.workerID, LeaseToken: claim.Lease.Token, Error: err.Error()}); err == nil {
		t.Fatal("the displaced worker spent an attempt on a failure it had no right to record")
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE id = $1 AND attempts = 0 AND status = 'running'`, job.ID); got != 1 {
		t.Fatal("the displaced worker's failure was counted against the job")
	}
	// The new owner can finish it, and the result is a single set of records.
	fresh := Claim{Job: job, Lease: jobs.Lease{JobID: job.ID, WorkerID: "p006-taker", Token: readLeaseToken(t, fixture, job.ID)}}
	if err := service.persistProcessedPages(fixture.Ctx(), fresh, legacy.record(), processed); err != nil {
		t.Fatalf("the current owner could not write: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE source_file_id = $1`, legacy.fileID); got != len(pages) {
		t.Fatalf("passages = %d, want %d from the owner that was allowed to write", got, len(pages))
	}
	if got := fixture.Count(`SELECT count(*) FROM source_processing_runs WHERE id = $1 AND status = 'succeeded'`, legacy.runID); got != 1 {
		t.Fatal("the run is not complete after the owner that was allowed to write finished")
	}
}

// slowProvider is the deterministic provider with a delay, and a hook for the
// first call. It exists so "slow work" and "a takeover at a chosen moment" are
// things a test can state rather than hope for.
type slowProvider struct {
	stubProvider
	delay     time.Duration
	firstRun  atomic.Bool
	onFirst   func()
	onCall    func(int)
	callCount atomic.Int64
}

func (p *slowProvider) wait(ctx context.Context) error {
	call := int(p.callCount.Add(1))
	if p.onFirst != nil && p.firstRun.CompareAndSwap(false, true) {
		p.onFirst()
	}
	if p.onCall != nil {
		p.onCall(call)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(p.delay):
		return nil
	}
}

func (p *slowProvider) onFirstCall(fn func()) { p.onFirst = fn }

func (p *slowProvider) calls() int { return int(p.callCount.Load()) }

func (p *slowProvider) Embed(ctx context.Context, request ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	if err := p.wait(ctx); err != nil {
		return ai.EmbeddingResponse{}, err
	}
	values := make([]float64, request.Dimensions)
	for index := range values {
		values[index] = float64(index+1) / float64(request.Dimensions)
	}
	return ai.EmbeddingResponse{Embedding: values, Dimensions: len(values), Model: "p006-slow-stub", Deterministic: true}, nil
}

func (p *slowProvider) ExtractEntities(ctx context.Context, request ai.ExtractionRequest) (ai.EntityExtractionResponse, error) {
	if err := p.wait(ctx); err != nil {
		return ai.EntityExtractionResponse{}, err
	}
	return p.stubProvider.ExtractEntities(ctx, request)
}

func (p *slowProvider) ExtractClaims(ctx context.Context, request ai.ExtractionRequest) (ai.ClaimExtractionResponse, error) {
	if err := p.wait(ctx); err != nil {
		return ai.ClaimExtractionResponse{}, err
	}
	return p.stubProvider.ExtractClaims(ctx, request)
}

func (p *slowProvider) ResolveEntity(ctx context.Context, request ai.EntityResolutionRequest) (ai.EntityResolutionResponse, error) {
	if err := p.wait(ctx); err != nil {
		return ai.EntityResolutionResponse{}, err
	}
	return p.stubProvider.ResolveEntity(ctx, request)
}

// queuedTextFile is the smallest document the processor accepts, wired into the
// fixture schema with a real queued job behind it. The tests that need a
// ready-to-write document use it so they can get to the fence without first
// having to get through extraction.
type queuedTextFile struct {
	pool     *pgxpool.Pool
	sourceID uuid.UUID
	runID    uuid.UUID
	fileID   uuid.UUID
	workerID string
	fileKey  string
	content  []byte
}

func newQueuedTextFile(t *testing.T, fixture *testsupport.Fixture, store storage.Store, filename string, content []byte) *queuedTextFile {
	t.Helper()
	ctx := fixture.Ctx()
	file := &queuedTextFile{
		pool:     fixture.Pool(),
		sourceID: uuid.New(),
		runID:    uuid.New(),
		fileID:   uuid.New(),
		workerID: "p006-queued-worker",
		content:  content,
	}
	userID := uuid.New()
	file.fileKey = "sources/" + file.sourceID.String() + "/" + file.fileID.String() + "-source"
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM jobs WHERE payload->>'source_file_id' = $1`, file.fileID.String()},
			{`DELETE FROM source_passages WHERE source_file_id = $1`, file.fileID},
			{`DELETE FROM source_candidates WHERE source_file_id = $1`, file.fileID},
			{`DELETE FROM source_statements WHERE source_file_id = $1`, file.fileID},
			{`DELETE FROM source_processing_runs WHERE id = $1`, file.runID},
			{`DELETE FROM source_files WHERE id = $1`, file.fileID},
			{`DELETE FROM sources WHERE id = $1`, file.sourceID},
			{`DELETE FROM users WHERE id = $1`, userID},
		} {
			if _, err := fixture.Pool().Exec(context.Background(), cleanup.sql, cleanup.arg); err != nil {
				t.Errorf("cleanup %q: %v", cleanup.sql, err)
			}
		}
	})
	if _, err := store.Put(ctx, file.fileKey, strings.NewReader(string(content)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'صاحب ملف الطابور')`, userID, fixture.Email())
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, created_by) VALUES ($1, 'مصدر الطابور', 'manuscript', $2)`, file.sourceID, userID)
	digest := sha256.Sum256(content)
	fixture.Exec(`
		INSERT INTO source_files (id, source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256, processing_status)
		VALUES ($1, $2, $3, $4, 'text/plain', $5, $6, 'queued')
	`, file.fileID, file.sourceID, file.fileKey, filename, len(content), hex.EncodeToString(digest[:]))
	payload, err := json.Marshal(JobPayload{SourceID: file.sourceID.String(), SourceFileID: file.fileID.String()})
	if err != nil {
		t.Fatal(err)
	}
	fixture.Exec(`INSERT INTO jobs (type, payload, status, max_attempts) VALUES ($1, $2, 'queued', 3)`, SourceProcessJobType, payload)
	fixture.Exec(`
		INSERT INTO source_processing_runs (source_id, source_file_id, status, stage)
		VALUES ($1, $2, 'queued', 'queued')
	`, file.sourceID, file.fileID)
	if err := fixture.QueryRow(`SELECT id FROM source_processing_runs WHERE source_file_id = $1`, file.fileID).Scan(&file.runID); err != nil {
		t.Fatal(err)
	}
	return file
}

func (f *queuedTextFile) record() sourceFileRecord {
	digest := sha256.Sum256(f.content)
	return sourceFileRecord{
		RunID:      f.runID,
		SourceID:   f.sourceID,
		FileID:     f.fileID,
		StorageKey: f.fileKey,
		MimeType:   "text/plain",
		Filename:   "late.txt",
		Status:     "queued",
		RunStatus:  "queued",
		ByteSize:   int64(len(f.content)),
		Checksum:   hex.EncodeToString(digest[:]),
	}
}

func readLeaseToken(t *testing.T, fixture *testsupport.Fixture, jobID string) string {
	t.Helper()
	var token string
	if err := fixture.QueryRow(`SELECT lease_token::text FROM jobs WHERE id = $1`, jobID).Scan(&token); err != nil {
		t.Fatalf("read the lease token: %v", err)
	}
	return token
}
