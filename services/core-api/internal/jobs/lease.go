package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultLeaseDuration is how long a claim stays valid without a renewal.
//
// It is deliberately a small multiple of DefaultHeartbeatInterval rather than a
// fraction of the fifteen-minute stale-recovery window. The recovery window
// answers "is this process gone"; the lease answers "does this worker still own
// the job", and a worker can only answer the second question within a minute if
// the first one is not what it depends on. A worker that misses a heartbeat
// loses its lease, is told so, and stops before it writes - and the job is
// retried rather than written twice.
const DefaultLeaseDuration = 90 * time.Second

// DefaultHeartbeatInterval is how often a worker proves it is still alive.
//
// The three-to-one ratio against DefaultLeaseDuration is the whole safety
// argument: two consecutive renewals can fail - a lost connection, a slow
// statement, a paused process - before the lease expires, so an ordinary blip
// does not fence a worker that is fine, while a worker that is genuinely gone
// is fenced long before the recovery window would have noticed.
const DefaultHeartbeatInterval = 30 * time.Second

// Lease is a worker's proof that it still owns a job.
//
// It is deliberately not the worker id. A worker id names a process; a lease
// names one claim of one job, and a process that restarts, runs twice, or is
// reclaimed after stale recovery gets a fresh token every time. Everything a
// worker writes goes through this value, so a worker holding an expired lease is
// rejected by the database instead of trusted because it recognised its own
// name.
type Lease struct {
	// JobID is the job the claim belongs to.
	JobID string `json:"jobId"`
	// WorkerID is the process that took the claim. It travels with the token
	// because a mismatch then names which of the two the caller got wrong.
	WorkerID string `json:"workerId"`
	// Token is the value the claim minted. It is compared, never parsed.
	Token string `json:"token"`
	// ExpiresAt is the deadline the claim was handed. The worker reads it to
	// know its slack; the database decides, not this field.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Valid reports whether the lease is complete enough to check. A lease with no
// token cannot fence anything, so a caller that builds one by hand is told so
// rather than sending a check that cannot fail the way it is supposed to.
func (l Lease) Valid() bool {
	return strings.TrimSpace(l.JobID) != "" && strings.TrimSpace(l.WorkerID) != "" && strings.TrimSpace(l.Token) != ""
}

func (l Lease) uuid() (uuid.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(l.JobID))
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return parsed, nil
}

// Renew extends the lease the worker still holds.
//
// The check and the extension are one statement: the row has to be running,
// locked to this worker, carrying this token, and its lease must not have
// expired. A worker that lost the lease gets ErrLeaseLost, which is the
// difference between "renewed" and "somebody else owns this now" - the
// distinction a worker needs before it commits anything.
func (s *Service) Renew(ctx context.Context, lease Lease) (JobView, error) {
	if err := s.ready(); err != nil {
		return JobView{}, err
	}
	lease, id, seconds, err := s.validateLease(lease)
	if err != nil {
		return JobView{}, err
	}
	job, err := scanJob(s.Pool.QueryRow(ctx, renewJobQuery, id, lease.WorkerID, seconds, lease.Token))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, s.leaseError(ctx, id, lease)
	}
	return job, err
}

func (s *Service) validateLease(lease Lease) (Lease, uuid.UUID, int, error) {
	lease.JobID = strings.TrimSpace(lease.JobID)
	lease.WorkerID = strings.TrimSpace(lease.WorkerID)
	lease.Token = strings.TrimSpace(lease.Token)
	if !lease.Valid() {
		return Lease{}, uuid.Nil, 0, ErrValidation
	}
	id, err := lease.uuid()
	if err != nil {
		return Lease{}, uuid.Nil, 0, err
	}
	seconds := int(s.leaseDuration() / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return lease, id, seconds, nil
}

// HoldLease proves - and extends - a claim from inside the caller's own
// transaction.
//
// This is the check that has to happen at the point where a result is written,
// not once at the start of the job. By the time a worker has embedded a
// document's pages, resolved its entities, or waited on a model, minutes have
// passed and the answer may have changed; renewing from a goroutine tells the
// worker it lost the job, but only the transaction that is about to write can
// refuse the write. Running it as the first statement of that transaction also
// means the check and the rows it protects commit or roll back together.
//
// A refusal is ErrLeaseLost in every case, because from the writer's side the
// only thing that matters is that it must not write.
func (s *Service) HoldLease(ctx context.Context, q Executor, lease Lease) error {
	lease, id, seconds, err := s.validateLease(lease)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx, renewJobExec, id, lease.WorkerID, seconds, lease.Token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return nil
}

// leaseError explains a refusal. The distinction that matters to a worker is
// simple: anything other than ErrNotFound means it must not write, and
// ErrLeaseLost is the one that means another worker may already be writing.
func (s *Service) leaseError(ctx context.Context, id uuid.UUID, lease Lease) error {
	var status, owner string
	var token pgtype.Text
	if err := s.Pool.QueryRow(ctx, `SELECT status, COALESCE(locked_by, ''), lease_token::text FROM jobs WHERE id = $1`, id).Scan(&status, &owner, &token); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != "running" {
		return ErrConflict
	}
	if owner != lease.WorkerID || textValue(token) != lease.Token {
		return ErrForbidden
	}
	// Running under this worker with this token, and the write was still refused:
	// the only remaining reason is the clock. The lease ran out before the
	// renewal arrived, so another worker was free to take the job, and the
	// answer is the same one a reclaimed job gets.
	return ErrLeaseLost
}

func (s *Service) leaseDuration() time.Duration {
	if s != nil && s.LeaseDuration > 0 {
		return s.LeaseDuration
	}
	return DefaultLeaseDuration
}

func (s *Service) heartbeatInterval() time.Duration {
	if s != nil && s.HeartbeatInterval > 0 {
		return s.HeartbeatInterval
	}
	return DefaultHeartbeatInterval
}

// Heartbeat renews a lease on a timer and remembers the moment it stopped being
// able to.
//
// The failure it exists for is the slow one. A worker in the middle of a page
// embedding, a database call, or an AI request has no natural place to check
// ownership, and by the time it comes back the answer may have changed. So the
// renewal runs on its own goroutine, and losing the lease cancels a context the
// work itself was given - which aborts the in-flight call instead of letting it
// run to completion and then discover it is not allowed to commit.
type Heartbeat struct {
	service  *Service
	lease    Lease
	interval time.Duration
	cancel   context.CancelCauseFunc
	done     chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	lost     error
}

// StartHeartbeat begins renewing lease until Stop is called or ctx is done.
//
// The returned context is the one the work must use: it is cancelled the moment
// the lease is lost, so a worker that kept its own context could keep going
// after losing the job and only find out when a write is refused. Cancelling
// makes the loss visible where it can still stop something.
func (s *Service) StartHeartbeat(ctx context.Context, lease Lease) (*Heartbeat, context.Context, error) {
	if err := s.ready(); err != nil {
		return nil, nil, err
	}
	if _, _, _, err := s.validateLease(lease); err != nil {
		return nil, nil, err
	}
	workCtx, cancel := context.WithCancelCause(ctx)
	heartbeat := &Heartbeat{
		service:  s,
		lease:    lease,
		interval: s.heartbeatInterval(),
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	go heartbeat.run(workCtx)
	return heartbeat, workCtx, nil
}

func (h *Heartbeat) run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.done:
			return
		case <-ticker.C:
			if err := h.renew(ctx); err != nil {
				h.recordLoss(err)
				return
			}
		}
	}
}

// renew gets a context of its own. A worker shutting down cancels the caller's
// context, and a renewal cancelled that way must not be reported as a lost
// lease: the two are indistinguishable from inside the query, and conflating
// them would tell a healthy worker it lost a job on the way out.
func (h *Heartbeat) renew(ctx context.Context) error {
	renewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.service.leaseDuration())
	defer cancel()
	_, err := h.service.Renew(renewCtx, h.lease)
	return err
}

func (h *Heartbeat) recordLoss(err error) {
	h.mu.Lock()
	if h.lost == nil {
		h.lost = err
	}
	h.mu.Unlock()
	if h.cancel != nil {
		h.cancel(err)
	}
}

// Lost reports the first failure that ended the heartbeat, or nil while the
// lease is still held. A worker calls this before it persists anything: the gap
// between two heartbeats can be minutes of embedding work, and a lease can only
// be relied on if it is checked at the point where relying on it would commit.
func (h *Heartbeat) Lost() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lost
}

// Check is Lost under the name a precondition reads better at. A nil
// Heartbeat is a worker that was never fenced, so it reports no loss rather than
// a panic: the fencing is the caller's contract, and the processor keeps working
// when there is nothing to fence against.
func (h *Heartbeat) Check() error {
	return h.Lost()
}

// Stop ends the heartbeat and waits for the loop to leave. It returns the loss
// that ended it, if any, so a caller can tell "stopped cleanly" from "stopped
// because the job was taken away".
func (h *Heartbeat) Stop() error {
	if h == nil {
		return nil
	}
	h.stopOnce.Do(func() {
		close(h.done)
		if h.cancel != nil {
			h.cancel(context.Canceled)
		}
	})
	return h.Lost()
}
