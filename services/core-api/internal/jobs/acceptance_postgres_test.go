package jobs

import (
	"errors"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

// Job claiming is where two workers can take the same work twice, so the tests
// below pin the two guarantees the rest of the platform leans on: a claim hands
// the job to exactly one worker, and a job nobody finished is retried and then
// dies loudly instead of vanishing.
func TestClaimHandsTheJobToExactlyOneWorker(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	jobType := "p004_claim_" + fixture.Tag()
	enqueued, err := service.Enqueue(fixture.Ctx(), EnqueueInput{
		Type:           jobType,
		Payload:        []byte(`{"fixture":"claim"}`),
		IdempotencyKey: fixture.Unique("key"),
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if enqueued.Job.Status != "queued" {
		t.Fatalf("enqueued status = %s, want queued", enqueued.Job.Status)
	}

	first, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: "p004-worker-one", Type: jobType})
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if first.ID != enqueued.Job.ID {
		t.Fatalf("claimed job %s, want the enqueued %s", first.ID, enqueued.Job.ID)
	}
	if first.Status != "running" || first.LockedBy != "p004-worker-one" {
		t.Fatalf("claimed job = %s/%s, want running under p004-worker-one", first.Status, first.LockedBy)
	}

	// A second worker asking for the same type must get nothing, not a copy.
	// Nothing claimable is reported as ErrNotFound, which is what lets a worker
	// tell "no work yet" apart from "the claim itself broke".
	second, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: "p004-worker-two", Type: jobType})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("second claim = %+v/%v, want ErrNotFound", second, err)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE id = $1 AND status = 'running' AND locked_by = $2`, first.ID, "p004-worker-one"); got != 1 {
		t.Fatalf("running job rows = %d, want 1", got)
	}

	// Only the holder of the lock may finish the job.
	if _, err := service.Complete(fixture.Ctx(), first.ID, CompleteInput{WorkerID: "p004-worker-two"}); err == nil {
		t.Fatal("a worker that does not hold the lock completed the job")
	}
	completed, err := service.Complete(fixture.Ctx(), first.ID, CompleteInput{WorkerID: "p004-worker-one"})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != "succeeded" || completed.LockedBy != "" {
		t.Fatalf("completed job = %s/%q, want succeeded with the lock released", completed.Status, completed.LockedBy)
	}
}

func TestClaimRespectsTheTypeAndTheRunTime(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	now := "p004_scheduled_" + fixture.Tag()
	future := "p004_ready_" + fixture.Tag()

	if _, err := service.Enqueue(fixture.Ctx(), EnqueueInput{
		Type:           now,
		Payload:        []byte(`{}`),
		IdempotencyKey: fixture.Unique("key"),
	}); err != nil {
		t.Fatalf("enqueue a delayed job: %v", err)
	}
	if _, err := service.Enqueue(fixture.Ctx(), EnqueueInput{
		Type:           future,
		Payload:        []byte(`{}`),
		IdempotencyKey: fixture.Unique("key"),
	}); err != nil {
		t.Fatalf("enqueue a ready job: %v", err)
	}
	fixture.Exec(`UPDATE jobs SET run_at = now() + interval '1 hour' WHERE type = $1`, now)

	if claimed, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: "p004-scheduled-worker", Type: now}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim of a job scheduled an hour out = %+v/%v, want ErrNotFound", claimed, err)
	}
	ready, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: "p004-ready-worker", Type: future})
	if err != nil {
		t.Fatalf("claim a ready job: %v", err)
	}
	if ready.ID == "" {
		t.Fatal("the ready job was not claimed")
	}
	// A worker asking for every type still cannot see the delayed one.
	fixture.Exec(`UPDATE jobs SET locked_by = NULL, locked_at = NULL, status = 'queued', attempts = 0 WHERE id = $1`, ready.ID)
	mixed, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: "p004-mixed-worker"})
	if err != nil {
		t.Fatalf("claim without a type filter: %v", err)
	}
	if mixed.Type == now {
		t.Fatalf("the unfiltered claim took the delayed job of type %s", now)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE type = $1 AND status = 'running'`, now); got != 0 {
		t.Fatalf("running jobs of the delayed type = %d, want 0", got)
	}
}

func TestRecoverReturnsAnAbandonedJobToTheQueue(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	jobType := "p004_recover_" + fixture.Tag()
	enqueued, err := service.Enqueue(fixture.Ctx(), EnqueueInput{
		Type:           jobType,
		Payload:        []byte(`{}`),
		IdempotencyKey: fixture.Unique("key"),
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: "p004-vanishing-worker", Type: jobType})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != enqueued.Job.ID {
		t.Fatalf("claimed %s, want %s", claimed.ID, enqueued.Job.ID)
	}
	// The worker dies without completing: nothing but a stale lock is left.
	fixture.Exec(`UPDATE jobs SET locked_at = now() - interval '2 hours' WHERE id = $1`, claimed.ID)

	recovered, err := service.RecoverStale(fixture.Ctx(), time.Hour)
	if err != nil {
		t.Fatalf("recover stale jobs: %v", err)
	}
	if recovered.Recovered < 1 {
		t.Fatalf("recovered %d job(s), want the abandoned one", recovered.Recovered)
	}
	after, err := service.Get(fixture.Ctx(), claimed.ID)
	if err != nil {
		t.Fatalf("read the recovered job: %v", err)
	}
	if after.Status != "queued" || after.LockedBy != "" {
		t.Fatalf("recovered job = %s/%q, want queued with no lock", after.Status, after.LockedBy)
	}
	// Recovery is scoped to genuinely stale locks: a job a worker is still
	// holding right now is left alone.
	if _, err := service.Enqueue(fixture.Ctx(), EnqueueInput{
		Type:           "p004_fresh_" + fixture.Tag(),
		Payload:        []byte(`{}`),
		IdempotencyKey: fixture.Unique("key"),
	}); err != nil {
		t.Fatalf("enqueue a fresh job: %v", err)
	}
	fresh, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: "p004-live-worker", Type: "p004_fresh_" + fixture.Tag()})
	if err != nil {
		t.Fatalf("claim the fresh job: %v", err)
	}
	if _, err := service.RecoverStale(fixture.Ctx(), time.Hour); err != nil {
		t.Fatalf("second recovery: %v", err)
	}
	live, err := service.Get(fixture.Ctx(), fresh.ID)
	if err != nil {
		t.Fatalf("read the live job: %v", err)
	}
	if live.Status != "running" || live.LockedBy != "p004-live-worker" {
		t.Fatalf("live job = %s/%q, want it still running under its worker", live.Status, live.LockedBy)
	}
}

func TestIdempotentEnqueueReturnsTheOriginalJob(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	key := fixture.Unique("key")
	jobType := "p004_idempotent_" + fixture.Tag()
	payload := []byte(`{"attempt":1}`)

	first, err := service.Enqueue(fixture.Ctx(), EnqueueInput{Type: jobType, Payload: payload, IdempotencyKey: key})
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	// A retried request must not create a second unit of work.
	second, err := service.Enqueue(fixture.Ctx(), EnqueueInput{Type: jobType, Payload: payload, IdempotencyKey: key})
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if second.Job.ID != first.Job.ID {
		t.Fatalf("idempotent enqueue created %s alongside %s", second.Job.ID, first.Job.ID)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE idempotency_key = $1`, key); got != 1 {
		t.Fatalf("jobs for the idempotency key = %d, want 1", got)
	}
	if second.Created {
		t.Fatal("a retried enqueue reported that it created the job")
	}
}
