package jobworker

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

// The consumer loop. Claim, dispatch, complete or fail - and the three ways that
// can go wrong that matter: a handler that does not exist, a handler that fails,
// and a handler that has lost its claim.

// recordingHandler is a handler whose outcome the test dictates, so the loop's
// response to each can be observed without a real analysis behind it.
type recordingHandler struct {
	jobType   string
	handled   []string
	err       error
	recovered []string
}

func (h *recordingHandler) JobTypes() []string { return []string{h.jobType} }

func (h *recordingHandler) Handle(_ context.Context, lease jobs.Lease, job jobs.JobView) error {
	h.handled = append(h.handled, job.ID)
	return h.err
}

func (h *recordingHandler) RequeueRecovered(_ context.Context, recovered []jobs.JobView) error {
	for _, job := range recovered {
		h.recovered = append(h.recovered, job.ID)
	}
	return nil
}

func newTestWorker(t *testing.T, fixture *testsupport.Fixture, handlers ...Handler) *Worker {
	t.Helper()
	queue := jobs.NewService(fixture.Pool())
	queue.LeaseDuration = 5 * time.Second
	queue.HeartbeatInterval = 200 * time.Millisecond
	worker, err := New(Config{
		Jobs:             queue,
		WorkerID:         "p006-loop-worker",
		Handlers:         handlers,
		PollInterval:     20 * time.Millisecond,
		RecoveryInterval: 20 * time.Millisecond,
		StaleAfter:       50 * time.Millisecond,
		Logger:           log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("build the worker: %v", err)
	}
	return worker
}

// TestTheLoopDrainsWhatItIsRegisteredFor is the dispatch contract: a job of a
// registered type is claimed and finished, and a job of a type nobody registered
// is left exactly where it was rather than being failed by a worker that cannot
// help.
func TestTheLoopDrainsWhatItIsRegisteredFor(t *testing.T) {
	fixture := testsupport.New(t)
	queue := jobs.NewService(fixture.Pool())
	mine := &recordingHandler{jobType: "p006_loop_mine_" + fixture.Tag()}
	theirs := &recordingHandler{jobType: "p006_loop_theirs_" + fixture.Tag()}
	worker := newTestWorker(t, fixture, mine, theirs)

	enqueued := enqueueLoopJob(t, fixture, queue, mine.jobType)
	foreign := enqueueLoopJob(t, fixture, queue, theirs.jobType)

	ctx, cancel := context.WithCancel(fixture.Ctx())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	waitFor(t, 30*time.Second, func() bool { return len(mine.handled) == 1 })
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("run: %v", err)
	}

	finished, err := queue.Get(fixture.Ctx(), enqueued.ID)
	if err != nil {
		t.Fatalf("read the drained job: %v", err)
	}
	if finished.Status != "succeeded" {
		t.Fatalf("job = %s (%s), want succeeded", finished.Status, finished.LastError)
	}
	// The job the loop was not registered for is untouched, and its worker - which
	// was registered for it in this test but shares the loop - got it only because
	// the loop asked for its type.
	if len(theirs.handled) != 1 {
		t.Fatalf("the other handler ran %d time(s), want 1", len(theirs.handled))
	}
	other, err := queue.Get(fixture.Ctx(), foreign.ID)
	if err != nil {
		t.Fatalf("read the other job: %v", err)
	}
	if other.Status != "succeeded" {
		t.Fatalf("the other job = %s, want succeeded", other.Status)
	}
}

// TestTheLoopFailsAJobItsHandlerRefused is the failure path: a handler that
// returns an error spends one of the job's attempts, records the reason, and
// leaves the job claimable for a retry.
func TestTheLoopFailsAJobItsHandlerRefused(t *testing.T) {
	fixture := testsupport.New(t)
	queue := jobs.NewService(fixture.Pool())
	refusing := &recordingHandler{jobType: "p006_loop_refuse_" + fixture.Tag(), err: errors.New("the analysis could not read its target")}
	worker := newTestWorker(t, fixture, refusing)
	enqueued := enqueueLoopJob(t, fixture, queue, refusing.jobType)

	ctx, cancel := context.WithCancel(fixture.Ctx())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	waitFor(t, 30*time.Second, func() bool {
		current, err := queue.Get(fixture.Ctx(), enqueued.ID)
		return err == nil && current.Attempts == 1
	})
	cancel()
	<-done

	failed, err := queue.Get(fixture.Ctx(), enqueued.ID)
	if err != nil {
		t.Fatalf("read the failed job: %v", err)
	}
	if failed.Status != "queued" {
		t.Fatalf("job = %s, want requeued for a retry", failed.Status)
	}
	if failed.LastError != "the analysis could not read its target" {
		t.Fatalf("last error = %q, want the reason the handler gave", failed.LastError)
	}
}

// TestTheLoopAbandonsAJobItNoLongerOwns is the fencing path. A handler that comes
// back reporting a lost lease has said the job is somebody else's; the loop must
// neither fail it nor requeue it, because either would be one worker answering for
// another and would put the same work in flight twice.
func TestTheLoopAbandonsAJobItNoLongerOwns(t *testing.T) {
	fixture := testsupport.New(t)
	queue := jobs.NewService(fixture.Pool())
	displaced := &recordingHandler{jobType: "p006_loop_lost_" + fixture.Tag(), err: jobs.ErrLeaseLost}
	worker := newTestWorker(t, fixture, displaced)
	enqueued := enqueueLoopJob(t, fixture, queue, displaced.jobType)

	ctx, cancel := context.WithCancel(fixture.Ctx())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	waitFor(t, 30*time.Second, func() bool { return len(displaced.handled) == 1 })
	// Give the loop long enough to have done the wrong thing, if it were going to.
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done

	current, err := queue.Get(fixture.Ctx(), enqueued.ID)
	if err != nil {
		t.Fatalf("read the abandoned job: %v", err)
	}
	if current.Attempts != 0 {
		t.Fatalf("attempts = %d, want the abandonment not to spend one", current.Attempts)
	}
	if current.LastError != "" {
		t.Fatalf("last error = %q, want the job untouched", current.LastError)
	}
	if current.Status != "running" {
		t.Fatalf("job = %s, want it left for whoever owns the claim now", current.Status)
	}
}

// TestTheLoopRefusesAWorkerItCannotConfigure keeps a misconfigured deployment from
// looking like a working one: a worker with no handlers, no id, or two handlers
// fighting over a job type says so at startup instead of running quietly and
// finishing nothing.
func TestTheLoopRefusesAWorkerItCannotConfigure(t *testing.T) {
	fixture := testsupport.New(t)
	queue := jobs.NewService(fixture.Pool())
	shared := &recordingHandler{jobType: "p006_loop_shared_" + fixture.Tag()}
	for name, config := range map[string]Config{
		"no queue":    {WorkerID: "w", Handlers: []Handler{shared}},
		"no id":       {Jobs: queue, Handlers: []Handler{shared}},
		"no handlers": {Jobs: queue, WorkerID: "w"},
		"nil handler": {Jobs: queue, WorkerID: "w", Handlers: []Handler{nil}},
		"empty type":  {Jobs: queue, WorkerID: "w", Handlers: []Handler{&recordingHandler{jobType: "  "}}},
		"two owners":  {Jobs: queue, WorkerID: "w", Handlers: []Handler{shared, &recordingHandler{jobType: shared.jobType}}},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("New with %s = nil error, want a refusal", name)
		}
	}
}

func enqueueLoopJob(t *testing.T, fixture *testsupport.Fixture, queue *jobs.Service, jobType string) jobs.JobView {
	t.Helper()
	result, err := queue.Enqueue(fixture.Ctx(), jobs.EnqueueInput{
		Type:           jobType,
		Payload:        []byte(`{"loop":"p006"}`),
		IdempotencyKey: fixture.Unique("key"),
		MaxAttempts:    3,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return result.Job
}

func waitFor(t *testing.T, limit time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out after %s", limit)
}
