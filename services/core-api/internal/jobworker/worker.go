// Package jobworker is the loop a queue consumer runs.
//
// It exists because a queue is only as good as the process that drains it, and
// the honest way to show a deployed system drains its queue is for the draining
// to be one piece of code with one claim/renew/complete cycle that every job type
// goes through. Two workers - source processing and the analyses - share this
// loop and differ only in the handlers they register, so a fix to the fencing
// cannot be made in one of them and forgotten in the other.
//
// The loop is deliberately small. It claims one job, hands the claim to a
// handler, and either completes the claim or fails it. Everything that makes it
// safe - the lease the handler holds, the heartbeat the handler renews, the
// owner check on the write - lives with the handler, because a handler is the
// only thing that knows when it is about to write.
package jobworker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
)

// Handler processes one claimed job under one lease.
//
// A handler that returns nil has finished the job. A handler that returns an
// error has not, and the loop decides what that means: an error that says the
// lease is gone is not a failure of the job, because the attempt that owns the
// job now will report on it.
type Handler interface {
	// JobTypes are the queue types this handler takes. A handler that claims more
	// than one type says so here rather than by inspecting the job.
	JobTypes() []string
	// Handle does the work. The lease is the handler's to renew and to check
	// before it writes, and it is the same lease the loop will use to complete or
	// fail the job.
	Handle(ctx context.Context, claim jobs.Lease, job jobs.JobView) error
	// RequeueRecovered puts the run rows of jobs that stale recovery handed back
	// into a state a worker can pick up. It is called with only the jobs this
	// worker owns, because with several consumers sharing the queue the first
	// one to run recovery is the only one that will see the job.
	RequeueRecovered(ctx context.Context, recovered []jobs.JobView) error
}

// Config is what a consumer binary configures. The zero value is not usable, and
// New rejects it rather than starting a loop that cannot claim anything.
type Config struct {
	// Jobs is the queue. Required.
	Jobs *jobs.Service
	// WorkerID names this process in the queue. Required, and it should say
	// something a reader can act on: a hostname, a pod name, a slot.
	WorkerID string
	// Handlers are the registered handlers, at least one.
	Handlers []Handler
	// PollInterval is how long to wait after finding no work. Zero means the
	// package default.
	PollInterval time.Duration
	// StaleAfter is the window a claim has to go unrenewed before recovery hands
	// it back. Zero means the package default, which is deliberately much longer
	// than a lease: a lease is how fast a worker learns it lost the job, and this
	// is how fast the queue gets the job back from a process that is gone without
	// saying so.
	StaleAfter time.Duration
	// RecoveryInterval is how often to run stale recovery. Zero means the package
	// default.
	RecoveryInterval time.Duration
	// Logger is where the loop reports. Nil means the standard logger.
	Logger *log.Logger
}

// Defaults for the loop's own timing. They are constants rather than fields so
// that a consumer which sets nothing behaves the same on every machine.
const (
	DefaultPollInterval     = 2 * time.Second
	DefaultStaleAfter       = 15 * time.Minute
	DefaultRecoveryInterval = time.Minute
)

// Run drains the queue until ctx is cancelled.
//
// It is the whole consumer. The loop claims one job at a time on purpose: a
// worker that had five jobs in flight would hold five leases to renew and five
// chances to lose one, and the throughput it would gain is not worth a fencing
// story that only holds for one job at a time.
func Run(ctx context.Context, config Config) error {
	worker, err := New(config)
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}

// Worker is a configured, validated consumer. New and Run are split so a caller
// - a test, or a process that wants to log its own startup line - can hold the
// worker it is about to run.
type Worker struct {
	jobs             *jobs.Service
	workerID         string
	handlers         map[string]Handler
	pollInterval     time.Duration
	staleAfter       time.Duration
	recoveryInterval time.Duration
	logger           *log.Logger
}

func New(config Config) (*Worker, error) {
	if config.Jobs == nil {
		return nil, errors.New("a job worker needs a queue")
	}
	workerID := strings.TrimSpace(config.WorkerID)
	if workerID == "" {
		return nil, errors.New("a job worker needs a worker id")
	}
	handlers := make(map[string]Handler, len(config.Handlers))
	for _, handler := range config.Handlers {
		if handler == nil {
			return nil, errors.New("a job worker cannot register a nil handler")
		}
		for _, jobType := range handler.JobTypes() {
			jobType = strings.TrimSpace(jobType)
			if jobType == "" {
				return nil, errors.New("a job worker handler registered an empty job type")
			}
			if owner, taken := handlers[jobType]; taken && owner != handler {
				return nil, fmt.Errorf("two handlers both claim the job type %q", jobType)
			}
			handlers[jobType] = handler
		}
	}
	if len(handlers) == 0 {
		return nil, errors.New("a job worker needs at least one handler")
	}
	worker := &Worker{
		jobs:             config.Jobs,
		workerID:         workerID,
		handlers:         handlers,
		pollInterval:     positiveOrDefault(config.PollInterval, DefaultPollInterval),
		staleAfter:       positiveOrDefault(config.StaleAfter, DefaultStaleAfter),
		recoveryInterval: positiveOrDefault(config.RecoveryInterval, DefaultRecoveryInterval),
		logger:           config.Logger,
	}
	if worker.logger == nil {
		worker.logger = log.Default()
	}
	return worker, nil
}

// WorkerID is the name this process claims work under.
func (w *Worker) WorkerID() string { return w.workerID }

// JobTypes are the queue types this consumer takes, sorted so a startup log and a
// test assertion can both read them.
func (w *Worker) JobTypes() []string {
	types := make([]string, 0, len(w.handlers))
	for jobType := range w.handlers {
		types = append(types, jobType)
	}
	sortStrings(types)
	return types
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Printf("job worker started: %s (types %v)", w.workerID, w.JobTypes())
	lastRecovery := time.Now()
	for {
		if ctx.Err() != nil {
			break
		}
		if time.Since(lastRecovery) >= w.recoveryInterval {
			w.recoverStale(ctx)
			lastRecovery = time.Now()
		}
		claimed, ok := w.claimOne(ctx)
		if !ok {
			if !sleepContext(ctx, w.pollInterval) {
				break
			}
			continue
		}
		w.process(ctx, claimed)
	}
	w.logger.Printf("job worker stopped: %s", w.workerID)
	return ctx.Err()
}

// claimOne takes a single job of any type this consumer handles.
//
// It asks for the types one at a time rather than for "any job", because a
// worker that claims a job it has no handler for would have to fail a job it
// never understood - spending one of that job's three attempts to say "not me".
func (w *Worker) claimOne(ctx context.Context) (jobs.JobView, bool) {
	for _, jobType := range w.JobTypes() {
		job, err := w.jobs.Claim(ctx, jobs.ClaimInput{WorkerID: w.workerID, Type: jobType})
		if errors.Is(err, jobs.ErrNotFound) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return jobs.JobView{}, false
			}
			w.logger.Printf("claim %s: %v", jobType, err)
			return jobs.JobView{}, false
		}
		return job, true
	}
	return jobs.JobView{}, false
}

func (w *Worker) process(ctx context.Context, job jobs.JobView) {
	// The context the handler is given is the worker's own, not a per-job one, so
	// a handler that ignores cancellation is bounded by how long its own stages
	// take. Cancelling a slow handler on shutdown would abandon a claim that
	// could still have been completed, and the job would wait for recovery
	// instead of finishing.
	handler, known := w.handlers[job.Type]
	if !known {
		w.logger.Printf("no handler for %s job %s, leaving it queued", job.Type, job.ID)
		return
	}
	lease := job.Lease(w.workerID)
	err := handler.Handle(ctx, lease, job)
	if err == nil {
		w.complete(ctx, job, lease)
		return
	}
	if leaseLost(err) {
		// The claim is somebody else's now. The attempt that owns the job decides
		// whether it failed, and requeueing it here would put the same work in
		// flight twice.
		w.logger.Printf("abandon %s job %s: %v", job.Type, job.ID, err)
		return
	}
	w.logger.Printf("process %s job %s: %v", job.Type, job.ID, err)
	w.fail(ctx, job, lease, err)
}

// complete records the one thing a queue consumer has to be able to show: that a
// unit of accepted work reached a terminal state, and which process took it there.
// The queue's own row says the same thing, but a row is a thing you have to go and
// look at, and a worker whose log is silent about its successes is indistinguishable
// from one that is not running.
func (w *Worker) complete(ctx context.Context, job jobs.JobView, lease jobs.Lease) {
	if _, err := w.jobs.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: w.workerID, LeaseToken: lease.Token}); err != nil {
		w.logger.Printf("complete %s job %s: %v", job.Type, job.ID, err)
		return
	}
	w.logger.Printf("completed %s job %s", job.Type, job.ID)
}

func (w *Worker) fail(ctx context.Context, job jobs.JobView, lease jobs.Lease, cause error) {
	if _, err := w.jobs.Fail(ctx, job.ID, jobs.FailInput{WorkerID: w.workerID, LeaseToken: lease.Token, Error: cause.Error()}); err != nil {
		if leaseLost(err) {
			w.logger.Printf("fail %s job %s: the claim was taken while it was being failed", job.Type, job.ID)
			return
		}
		w.logger.Printf("fail %s job %s: %v", job.Type, job.ID, err)
	}
}

// recoverStale hands abandoned work back, one job type at a time.
//
// The narrowing is the point. With more than one consumer sharing the queue, an
// unfiltered recovery belongs to whichever process runs it first: it takes the job
// out of running, and the process that runs second never sees it. A consumer that
// owns the run row of a job type therefore has to be the one that recovers that
// type, or the run row is left reading as running work that nobody owns.
func (w *Worker) recoverStale(ctx context.Context) {
	for _, jobType := range w.JobTypes() {
		handler := w.handlers[jobType]
		recovered, err := w.jobs.RecoverStaleOfType(ctx, w.staleAfter, jobType)
		if err != nil {
			if ctx.Err() == nil {
				w.logger.Printf("recover stale %s jobs: %v", jobType, err)
			}
			continue
		}
		if recovered.Recovered == 0 {
			continue
		}
		if err := handler.RequeueRecovered(ctx, recovered.Jobs); err != nil {
			w.logger.Printf("reset recovered %s runs: %v", jobType, err)
		}
	}
}

// leaseLost reports whether an error means this worker no longer owns the job.
//
// Both errors are treated the same way on purpose. ErrForbidden is what a worker
// gets when the row is running under a different owner or a different claim, and
// ErrLeaseLost is what it gets when the claim simply ran out; from here they mean
// the same thing, which is that anything this attempt writes would be a second
// copy of somebody else's work.
func leaseLost(err error) bool {
	return errors.Is(err, jobs.ErrLeaseLost) || errors.Is(err, jobs.ErrForbidden)
}

func positiveOrDefault(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
