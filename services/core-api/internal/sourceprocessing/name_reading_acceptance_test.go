package sourceprocessing

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
)

type countedNameStore struct {
	storage.Store
	originalKey   string
	originalReads atomic.Int32
}

func (s *countedNameStore) Get(ctx context.Context, key string) (io.ReadCloser, storage.Object, error) {
	if key == s.originalKey {
		s.originalReads.Add(1)
	}
	return s.Store.Get(ctx, key)
}

type fixtureNameReader struct {
	calls atomic.Int32
	read  func(context.Context, int) (LocalCropReading, error)
}

func (r *fixtureNameReader) ReadCrop(ctx context.Context, _ []byte, quality CropObservations) (LocalCropReading, error) {
	index := int(r.calls.Add(1))
	if r.read != nil {
		return r.read(ctx, index)
	}
	return LocalCropReading{Outcome: "readable", Text: "محمد", Alternatives: []string{}, Variants: []LocalVariantReading{{Variant: "fixture", Outcome: "readable", RawText: "محمد"}}, Quality: quality, Agreement: 1}, nil
}

type nameRunFixture struct {
	f             *testsupport.Fixture
	actor         actor.Actor
	service       *Service
	queue         *jobs.Service
	fileID        string
	store         *countedNameStore
	detectorCalls atomic.Int32
	reader        *fixtureNameReader
}

func newNameRunFixture(t *testing.T) *nameRunFixture {
	t.Helper()
	f := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, f, "مراجع إعداد محلي")
	source, err := evidence.NewService(f.Pool()).CreateSource(f.Ctx(), owner.User.ID, evidence.CreateSourceInput{TitleAR: "اختبار إعداد " + f.Unique("s"), SourceType: "family_document"})
	if err != nil {
		t.Fatal(err)
	}
	local, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := jobs.NewService(f.Pool())
	queue.HeartbeatInterval = 20 * time.Millisecond
	queue.LeaseDuration = 2 * time.Second
	service := NewService(f.Pool(), local, queue, nil, NewPDFExtractor())
	file, err := service.Upload(f.Ctx(), source.Source.ID, owner.User.ID, UploadInput{Filename: "fixture.png", ContentType: "image/png", Content: syntheticChartPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	var key string
	if err = f.Pool().QueryRow(f.Ctx(), `SELECT storage_key FROM source_files WHERE id=$1`, file.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	counted := &countedNameStore{Store: local, originalKey: key}
	service.Store = counted
	fixture := &nameRunFixture{f: f, actor: owner, service: service, queue: queue, fileID: file.ID, store: counted, reader: &fixtureNameReader{}}
	service.ReaderSettings = ReadingSettings{Mode: "local"}
	service.LocalCropReader = fixture.reader
	service.LocalReadingProbe = func(context.Context) LocalReaderStatus {
		return LocalReaderStatus{Reason: "configured", Version: LocalCropReaderVersion, Languages: []string{"ara"}, Tools: []ReadingToolStatus{{Name: "tesseract", Available: true, Version: "fixture-v1"}}}
	}
	service.DetectNameTile = func([]byte, CarrierLocaliserConfig) ([]PageRegion, error) {
		fixture.detectorCalls.Add(1)
		return []PageRegion{{Kind: "name_group", X: .1, Y: .2, Width: .15, Height: .1}, {Kind: "name_group", X: .6, Y: .2, Width: .15, Height: .1}}, nil
	}
	return fixture
}
func (f *nameRunFixture) claim(t *testing.T, worker string) Claim {
	t.Helper()
	job, err := f.queue.Claim(f.f.Ctx(), jobs.ClaimInput{WorkerID: worker, Type: SourceNameReadingJobType})
	if err != nil {
		t.Fatal(err)
	}
	return Claim{Job: job, Lease: job.Lease(worker)}
}
func (f *nameRunFixture) prepare(t *testing.T) NameReadingRun {
	t.Helper()
	run, err := f.service.PrepareNameReading(f.f.Ctx(), f.fileID, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}
func (f *nameRunFixture) process(t *testing.T, claim Claim) {
	t.Helper()
	if err := f.service.ProcessNameReading(f.f.Ctx(), claim); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ProcessNameReading(f.f.Ctx(), claim); err != nil {
		t.Fatalf("completed attempt was not idempotent: %v", err)
	}
	if _, err := f.queue.Complete(f.f.Ctx(), claim.Job.ID, jobs.CompleteInput{WorkerID: claim.Lease.WorkerID, LeaseToken: claim.Lease.Token}); err != nil {
		t.Fatal(err)
	}
}

func TestNamePreparationEnqueuesWithoutWaitingForTheReader(t *testing.T) {
	fixture := newNameRunFixture(t)
	fixture.reader.read = func(ctx context.Context, _ int) (LocalCropReading, error) {
		select {
		case <-ctx.Done():
			return LocalCropReading{}, ctx.Err()
		case <-time.After(16 * time.Second):
			return LocalCropReading{}, errors.New("fixture delayed reader")
		}
	}
	started := time.Now()
	run := fixture.prepare(t)
	duplicate := fixture.prepare(t)
	if time.Since(started) > time.Second || fixture.reader.calls.Load() != 0 || run.ID != duplicate.ID || run.Status != "queued" {
		t.Fatalf("enqueue waited, invoked OCR or duplicated: %+v", run)
	}
	if fixture.f.Count(`SELECT count(*) FROM jobs WHERE type=$1`, SourceNameReadingJobType) != 1 {
		t.Fatal("duplicate active job")
	}
}
func TestNameDetectionSurvivesUnavailableReaderAndResumeSkipsSuccess(t *testing.T) {
	fixture := newNameRunFixture(t)
	fixture.reader.read = func(_ context.Context, index int) (LocalCropReading, error) {
		if index == 2 {
			return LocalCropReading{Outcome: "reading_unavailable", Quality: CropObservations{Reason: "fixture_reader_unavailable"}}, nil
		}
		return LocalCropReading{Outcome: "readable", Text: "محمد", Agreement: 1}, nil
	}
	run := fixture.prepare(t)
	fixture.process(t, fixture.claim(t, "first"))
	saved, err := fixture.service.NameReadingRun(fixture.f.Ctx(), run.ID, fixture.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "partial" || saved.Detected != 2 || saved.Named != 1 || saved.Unavailable != 1 {
		t.Fatalf("lost detection or outcome: %+v", saved)
	}
	if fixture.f.Count(`SELECT count(*) FROM source_tree_suggestions WHERE source_file_id=$1`, fixture.fileID) != 2 {
		t.Fatal("geometry not reviewable")
	}
	if fixture.f.Count(`SELECT count(*) FROM source_tree_suggestions WHERE name_reading_run_id=$1 AND processing_run_id IS NOT NULL`, run.ID) != 0 {
		t.Fatal("mixed run identifier relations")
	}
	if _, err = fixture.service.ResumeNameReading(fixture.f.Ctx(), run.ID, fixture.actor.User.ID); err != nil {
		t.Fatal(err)
	}
	fixture.process(t, fixture.claim(t, "second"))
	saved, err = fixture.service.NameReadingRun(fixture.f.Ctx(), run.ID, fixture.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "succeeded" || saved.Named != 2 || saved.Unavailable != 0 || fixture.detectorCalls.Load() != 1 || fixture.reader.calls.Load() != 3 {
		t.Fatalf("resume repeated completed units or lost counts: %+v", saved)
	}
	if fixture.f.Count(`SELECT count(*) FROM source_name_readings WHERE run_id=$1`, run.ID) != 3 {
		t.Fatal("reading attempts were overwritten")
	}
}
func TestNameReadingRerunCannotRewriteADecidedMachineReading(t *testing.T) {
	fixture := newNameRunFixture(t)
	fixture.prepare(t)
	fixture.process(t, fixture.claim(t, "initial"))
	queue, err := fixture.service.ListSuggestions(fixture.f.Ctx(), fixture.fileID, fixture.actor.User.ID, "", 0, 0)
	if err != nil || len(queue) != 2 {
		t.Fatal(err)
	}
	original := queue[0]
	_, err = fixture.service.DecideSuggestion(fixture.f.Ctx(), original.ID, fixture.actor.User.ID, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "أحمد"})
	if err != nil {
		t.Fatal(err)
	}
	var before string
	fixture.f.Pool().QueryRow(fixture.f.Ctx(), `SELECT row_to_json(d)::text FROM source_tree_suggestion_decisions d WHERE suggestion_id=$1`, original.ID).Scan(&before)
	fixture.reader.read = func(context.Context, int) (LocalCropReading, error) {
		return LocalCropReading{Outcome: "readable", Text: "محمود", Agreement: 1}, nil
	}
	rerun, err := fixture.service.RereadSuggestionLocally(fixture.f.Ctx(), original.ID, fixture.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.process(t, fixture.claim(t, "reread"))
	var text, model, after string
	fixture.f.Pool().QueryRow(fixture.f.Ctx(), `SELECT suggested_text_ar,reading_model FROM source_tree_suggestions WHERE id=$1`, original.ID).Scan(&text, &model)
	fixture.f.Pool().QueryRow(fixture.f.Ctx(), `SELECT row_to_json(d)::text FROM source_tree_suggestion_decisions d WHERE suggestion_id=$1`, original.ID).Scan(&after)
	if text != original.SuggestedTextAR || model != original.ReadingModel || before != after {
		t.Fatal("reread changed decided evidence")
	}
	if fixture.f.Count(`SELECT count(*) FROM source_name_readings WHERE run_id=$1 AND text_ar='محمود'`, rerun.ID) != 1 {
		t.Fatal("new hypothesis was not retained separately")
	}
}
func TestNameRunCancellationFencesABlockedReader(t *testing.T) {
	fixture := newNameRunFixture(t)
	entered := make(chan struct{})
	fixture.reader.read = func(ctx context.Context, _ int) (LocalCropReading, error) {
		close(entered)
		<-ctx.Done()
		return LocalCropReading{}, ctx.Err()
	}
	run := fixture.prepare(t)
	claim := fixture.claim(t, "blocked")
	done := make(chan error, 1)
	go func() { done <- fixture.service.ProcessNameReading(fixture.f.Ctx(), claim) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never entered reader")
	}
	cancelled, err := fixture.service.CancelNameReading(fixture.f.Ctx(), run.ID, fixture.actor.User.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("cancel=%+v error=%v", cancelled, err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, jobs.ErrLeaseLost) && !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not stop the reader")
	}
	if fixture.f.Count(`SELECT count(*) FROM source_name_readings WHERE run_id=$1`, run.ID) != 0 {
		t.Fatal("cancelled worker wrote a reading")
	}
	if fixture.f.Count(`SELECT count(*) FROM source_tree_suggestions WHERE source_file_id=$1`, fixture.fileID) != 2 {
		t.Fatal("cancellation lost detection")
	}
}
func TestNameWorkerRecoveryResumesCheckpointWithoutOriginalReads(t *testing.T) {
	fixture := newNameRunFixture(t)
	entered := make(chan struct{})
	fixture.reader.read = func(ctx context.Context, index int) (LocalCropReading, error) {
		if index == 2 {
			close(entered)
			<-ctx.Done()
			return LocalCropReading{}, ctx.Err()
		}
		return LocalCropReading{Outcome: "readable", Text: "محمد", Agreement: 1}, nil
	}
	run := fixture.prepare(t)
	old := fixture.claim(t, "old")
	done := make(chan error, 1)
	workerCtx, stopWorker := context.WithCancel(fixture.f.Ctx())
	go func() { done <- fixture.service.ProcessNameReading(workerCtx, old) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never checkpointed first crop")
	}
	stopWorker()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("old worker did not stop")
	}
	_, err := fixture.f.Pool().Exec(fixture.f.Ctx(), `UPDATE jobs SET locked_at=now()-interval '5 minutes',lease_expires_at=now()-interval '1 minute',lease_token=gen_random_uuid() WHERE id=$1`, old.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	// RecoverStale deliberately uses SKIP LOCKED. An in-flight renewal can
	// still be releasing its database row after Stop; production retries its
	// recovery pass, so the fixture must assert bounded eventual recovery too.
	var recovered jobs.RecoverResult
	recoveryDeadline := time.Now().Add(3 * time.Second)
	for {
		recovered, err = fixture.queue.RecoverStaleOfType(fixture.f.Ctx(), time.Second, SourceNameReadingJobType)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Recovered > 0 || time.Now().After(recoveryDeadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if recovered.Recovered != 1 {
		t.Fatalf("expected one stale job, got %+v", recovered)
	}
	if err = fixture.service.RequeueNameReadingJobs(fixture.f.Ctx(), recovered.Jobs); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.f.Pool().Exec(fixture.f.Ctx(), `UPDATE jobs SET run_at=clock_timestamp()-interval '1 second' WHERE id=$1`, old.Job.ID); err != nil {
		t.Fatal(err)
	}
	var jobStatus string
	fixture.f.Pool().QueryRow(fixture.f.Ctx(), `SELECT status FROM jobs WHERE id=$1`, old.Job.ID).Scan(&jobStatus)
	if jobStatus != "queued" {
		t.Fatalf("recovered job status=%s", jobStatus)
	}
	fixture.reader.read = nil
	reads := fixture.store.originalReads.Load()
	fixture.process(t, fixture.claim(t, "new"))
	saved, err := fixture.service.NameReadingRun(fixture.f.Ctx(), run.ID, fixture.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "succeeded" || fixture.detectorCalls.Load() != 1 || fixture.reader.calls.Load() != 3 || fixture.store.originalReads.Load() != reads {
		t.Fatalf("restart repeated preparation: %+v reads=%d/%d", saved, reads, fixture.store.originalReads.Load())
	}
}

type cropMetadataJudge struct{ request NameJudgeRequest }

func (j *cropMetadataJudge) Judge(_ context.Context, request NameJudgeRequest) (NameJudgeResult, error) {
	j.request = request
	choice := request.Candidates[0].Value
	return NameJudgeResult{Choice: &choice, Probabilities: map[string]float64{choice: .8, "unclear": .2}, Confidence: .8, Provider: "liquid", Model: "fixture", PromptVersion: "fixture-v1"}, nil
}
func TestPreparedNameCropAndTextJudgeNeverReopenOriginal(t *testing.T) {
	fixture := newNameRunFixture(t)
	fixture.prepare(t)
	fixture.process(t, fixture.claim(t, "prepare"))
	queue, _ := fixture.service.ListSuggestions(fixture.f.Ctx(), fixture.fileID, fixture.actor.User.ID, "", 0, 0)
	if len(queue) != 2 {
		t.Fatal("missing queue")
	}
	before := fixture.store.originalReads.Load()
	for index := 0; index < 2; index++ {
		if _, err := fixture.service.GetSuggestionCrop(fixture.f.Ctx(), queue[0].ID, fixture.actor.User.ID); err != nil {
			t.Fatal(err)
		}
	}
	judge := &cropMetadataJudge{}
	fixture.service.NameJudge = judge
	fixture.service.NameJudgeMode = "shadow"
	if _, err := fixture.service.JudgeSuggestion(fixture.f.Ctx(), queue[0].ID, fixture.actor.User.ID, []JudgeCandidate{{Value: "محمد", Source: "reader", Rank: 1}}); err != nil {
		t.Fatal(err)
	}
	if fixture.store.originalReads.Load() != before || len(judge.request.ImagePNG) > 0 || judge.request.State.Crop.SourceWidthPx != 600 || judge.request.State.Crop.SourceHeightPx != 400 {
		t.Fatal("cached review or text judge reopened/misread the source")
	}
	other := actor.RegisterAndLogin(t, fixture.f, "مراجع دون صلاحية")
	if _, err := fixture.service.GetSuggestionCrop(fixture.f.Ctx(), queue[0].ID, other.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cache access=%v", err)
	}
}

func TestNameArtifactRetentionKeepsOriginalsAndHypotheses(t *testing.T) {
	fixture := newNameRunFixture(t)
	run := fixture.prepare(t)
	fixture.process(t, fixture.claim(t, "retention"))
	_, err := fixture.f.Pool().Exec(fixture.f.Ctx(), `UPDATE source_name_crop_artifacts SET expires_at=now()-interval '1 day'`)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := fixture.service.PruneNameArtifacts(fixture.f.Ctx())
	if err != nil || removed != 2 {
		t.Fatalf("pruned=%d error=%v", removed, err)
	}
	if fixture.f.Count(`SELECT count(*) FROM source_name_readings WHERE run_id=$1`, run.ID) != 2 {
		t.Fatal("retention removed immutable readings")
	}
	reader, _, err := fixture.store.Get(fixture.f.Ctx(), fixture.store.originalKey)
	if err != nil {
		t.Fatal("retention removed original")
	}
	reader.Close()
	queue, _ := fixture.service.ListSuggestions(fixture.f.Ctx(), fixture.fileID, fixture.actor.User.ID, "", 0, 0)
	if _, err := fixture.service.GetSuggestionCrop(fixture.f.Ctx(), queue[0].ID, fixture.actor.User.ID); err != nil {
		t.Fatalf("expired crop could not be prepared again: %v", err)
	}
}
func TestNameCoordinateMismatchIsRejectedBeforeAWrite(t *testing.T) {
	fixture := newNameRunFixture(t)
	run := fixture.prepare(t)
	claim := fixture.claim(t, "coordinates")
	bad := NamePageFrame{FileID: fixture.fileID, Checksum: run.Checksum, Page: 1, PageCount: 1, Space: "pdf_analysis", DPI: 300, Width: 600, Height: 400, Renderer: NameRendererVersion}
	err := fixture.service.persistNameDetection(fixture.f.Ctx(), &nameReadingWork{Claim: claim, RunID: run.ID, Generation: 1}, run, nameUnit{}, bad, nil, 0, false)
	if !errors.Is(err, ErrValidation) || fixture.f.Count(`SELECT count(*) FROM source_tree_suggestions WHERE source_file_id=$1`, fixture.fileID) != 0 {
		t.Fatalf("mismatched frame wrote geometry: %v", err)
	}
}
