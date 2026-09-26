package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// The lease matrix. Every case below is a claim, a renewal, a completion or a
// failure, and each one is asked the same question: is this caller still the
// owner of this claim? The interesting cases are the ones where the worker id
// matches and the answer is still no, because that is exactly the situation a
// worker-string comparison cannot detect.

// TestRenewExtendsOnlyTheLiveLease covers the ordinary case: the owner renews,
// the deadline moves out, and a second renewal by the same worker keeps working
// for as long as the worker keeps proving it is alive.
func TestRenewExtendsOnlyTheLiveLease(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	jobType := "p006_renew_" + fixture.Tag()
	enqueued := enqueueLeaseJob(t, fixture, service, jobType)
	claimed := claimLeaseJob(t, fixture, service, jobType, "p006-renew-worker")
	if claimed.ID != enqueued.Job.ID {
		t.Fatalf("claimed %s, want %s", claimed.ID, enqueued.Job.ID)
	}
	lease := claimed.Lease("p006-renew-worker")
	if !lease.Valid() {
		t.Fatal("a claimed job did not come with a usable lease")
	}
	if claimed.LeaseExpiresAt == nil {
		t.Fatal("a claimed job came with no lease deadline")
	}

	renewed, err := service.Renew(fixture.Ctx(), lease)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if !renewed.LeaseExpiresAt.After(*claimed.LeaseExpiresAt) {
		t.Fatalf("renewed deadline %s, want later than the claim's %s", renewed.LeaseExpiresAt, claimed.LeaseExpiresAt)
	}
	if renewed.HeartbeatAt == nil {
		t.Fatal("a renewal did not record a heartbeat")
	}
	if renewed.LeaseToken != lease.Token {
		t.Fatalf("renewal changed the token from %s to %s", lease.Token, renewed.LeaseToken)
	}
	// The row agrees with the answer, which is what a second worker would read.
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE id = $1 AND lease_token = $2 AND lease_expires_at > now() AND heartbeat_at IS NOT NULL`, lease.JobID, lease.Token); got != 1 {
		t.Fatalf("rows with a live lease for this claim = %d, want 1", got)
	}
}

// TestRenewIsRefusedWithoutTheClaimToken is the fencing case. The worker id is
// right, the job is running, and the answer is still no - because the token on
// the row belongs to a different claim.
func TestRenewIsRefusedWithoutTheClaimToken(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	jobType := "p006_foreign_token_" + fixture.Tag()
	enqueueLeaseJob(t, fixture, service, jobType)
	claimed := claimLeaseJob(t, fixture, service, jobType, "p006-token-worker")

	forged := claimed.Lease("p006-token-worker")
	forged.Token = uuid.NewString()
	if _, err := service.Renew(fixture.Ctx(), forged); !errors.Is(err, ErrForbidden) {
		t.Fatalf("renew with somebody else's token = %v, want ErrForbidden", err)
	}
	// An unparseable token is the same answer: it is not this claim.
	empty := claimed.Lease("p006-token-worker")
	empty.Token = ""
	if _, err := service.Renew(fixture.Ctx(), empty); !errors.Is(err, ErrValidation) {
		t.Fatalf("renew with no token = %v, want ErrValidation", err)
	}
	// The real owner is untouched by the attempts above.
	if _, err := service.Renew(fixture.Ctx(), claimed.Lease("p006-token-worker")); err != nil {
		t.Fatalf("the real owner was refused: %v", err)
	}
}

// TestRecoveredJobRejectsTheWorkerThatLostIt is the stale-recovery case. A job
// is claimed, its worker vanishes, recovery requeues it, and a second worker
// takes it. The first worker then tries to finish the work it was in the middle
// of - with the correct worker id, the correct job id, and its old token.
func TestRecoveredJobRejectsTheWorkerThatLostIt(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	jobType := "p006_stale_" + fixture.Tag()
	enqueueLeaseJob(t, fixture, service, jobType)
	stale := claimLeaseJob(t, fixture, service, jobType, "p006-vanished-worker")
	staleLease := stale.Lease("p006-vanished-worker")

	// The worker is gone: its lock ages past the recovery window and the job is
	// handed back to the queue.
	fixture.Exec(`UPDATE jobs SET locked_at = now() - interval '2 hours' WHERE id = $1`, stale.ID)
	recovered, err := service.RecoverStale(fixture.Ctx(), time.Hour)
	if err != nil {
		t.Fatalf("recover stale: %v", err)
	}
	if recovered.Recovered < 1 {
		t.Fatalf("recovered %d job(s), want the abandoned claim", recovered.Recovered)
	}
	afterRecovery, err := service.Get(fixture.Ctx(), stale.ID)
	if err != nil {
		t.Fatalf("read the recovered job: %v", err)
	}
	if afterRecovery.Status != "queued" || afterRecovery.LeaseToken != "" {
		t.Fatalf("recovered job = %s with token %q, want queued with the lease cleared", afterRecovery.Status, afterRecovery.LeaseToken)
	}

	fixture.Exec(`UPDATE jobs SET run_at = now() WHERE id = $1`, stale.ID)
	second := claimLeaseJob(t, fixture, service, jobType, "p006-second-worker")
	if second.LeaseToken == staleLease.Token {
		t.Fatal("the reclaim reused the previous claim's token, so nothing is fenced")
	}

	// Every write the first worker could attempt is refused. The row stays
	// running under the second worker throughout, which is the observable proof
	// that nothing was applied behind the current owner's back.
	if _, err := service.Renew(fixture.Ctx(), staleLease); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stale renew = %v, want ErrForbidden", err)
	}
	if _, err := service.Complete(fixture.Ctx(), staleLease.JobID, CompleteInput{WorkerID: staleLease.WorkerID, LeaseToken: staleLease.Token}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stale complete = %v, want ErrForbidden", err)
	}
	if _, err := service.Fail(fixture.Ctx(), staleLease.JobID, FailInput{WorkerID: staleLease.WorkerID, LeaseToken: staleLease.Token, Error: "stale worker"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale fail = %v, want ErrConflict", err)
	}
	// The same claim replayed under the current worker's name, with the current
	// worker's id, is refused on the token alone. This is the check that keeps a
	// worker from requeueing the job out from under whoever is processing it.
	if _, err := service.Fail(fixture.Ctx(), second.ID, FailInput{WorkerID: "p006-second-worker", LeaseToken: staleLease.Token, Error: "not this claim"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("fail of a live job with a foreign token = %v, want ErrForbidden", err)
	}
	current, err := service.Get(fixture.Ctx(), staleLease.JobID)
	if err != nil {
		t.Fatalf("read the job after the stale writes: %v", err)
	}
	if current.Status != "running" || current.LockedBy != "p006-second-worker" || current.LeaseToken != second.LeaseToken {
		t.Fatalf("job = %s/%s/%s, want it still running under the second worker with its own token", current.Status, current.LockedBy, current.LeaseToken)
	}
	if current.Attempts != 1 {
		t.Fatalf("attempts = %d, want the stale failure not to have been counted", current.Attempts)
	}
}

// TestExpiredLeaseIsFencedEvenForTheMatchingWorker is the case a worker-string
// comparison cannot reach. Nothing has reclaimed the job, the worker id matches,
// and the owner itself is out of time: the lease ran out while the worker was
// busy. Completing then would be a bet that nobody arrived in the meantime, and
// a bet is not a guarantee.
func TestExpiredLeaseIsFencedEvenForTheMatchingWorker(t *testing.T) {
	fixture := testsupport.New(t)
	// A one-second lease makes the expiry observable inside a test without
	// pretending the production deadline is short.
	service := NewService(fixture.Pool())
	service.LeaseDuration = time.Second
	jobType := "p006_expired_" + fixture.Tag()
	enqueueLeaseJob(t, fixture, service, jobType)
	claimed := claimLeaseJob(t, fixture, service, jobType, "p006-slow-worker")
	lease := claimed.Lease("p006-slow-worker")

	waitFor(t, fixture, `SELECT lease_expires_at <= now() FROM jobs WHERE id = $1`, lease.JobID)

	if _, err := service.Renew(fixture.Ctx(), lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("renew of an expired lease = %v, want ErrLeaseLost", err)
	}
	// Even the operator-shaped call, which presents no token, is refused: it
	// identified itself correctly and is still out of time.
	if _, err := service.Complete(fixture.Ctx(), lease.JobID, CompleteInput{WorkerID: lease.WorkerID}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("complete of an expired lease = %v, want ErrLeaseLost", err)
	}
	if _, err := service.Fail(fixture.Ctx(), lease.JobID, FailInput{WorkerID: lease.WorkerID, Error: "too late"}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("fail of an expired lease = %v, want ErrLeaseLost", err)
	}
	current, err := service.Get(fixture.Ctx(), lease.JobID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if current.Status != "running" {
		t.Fatalf("job status = %s, want the expired worker to have changed nothing", current.Status)
	}
}

// TestSameWorkerIdReclaimStillFences is the same hazard from the other side: the
// reclaiming worker is the same process with the same id, which is what a
// restart looks like from the database. The token is what separates the two
// attempts, and a completion that omits it has to be refused for the expired
// lease rather than accepted on the strength of the name.
func TestSameWorkerIdReclaimStillFences(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	jobType := "p006_restart_" + fixture.Tag()
	enqueued := enqueueLeaseJob(t, fixture, service, jobType)
	first := claimLeaseJob(t, fixture, service, jobType, "p006-restarted-worker")
	firstLease := first.Lease("p006-restarted-worker")

	fixture.Exec(`UPDATE jobs SET locked_at = now() - interval '2 hours' WHERE id = $1`, first.ID)
	if _, err := service.RecoverStale(fixture.Ctx(), time.Hour); err != nil {
		t.Fatalf("recover stale: %v", err)
	}
	fixture.Exec(`UPDATE jobs SET run_at = now() WHERE id = $1`, first.ID)
	second := claimLeaseJob(t, fixture, service, jobType, "p006-restarted-worker")
	if second.LeaseToken == firstLease.Token {
		t.Fatal("a reclaim under the same worker id reused the token, so the old claim is not fenced")
	}
	if _, err := service.Complete(fixture.Ctx(), firstLease.JobID, CompleteInput{WorkerID: firstLease.WorkerID, LeaseToken: firstLease.Token}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stale complete after a same-id reclaim = %v, want ErrForbidden", err)
	}
	// Fail decides the owner in Go, from the row it holds FOR UPDATE, so this is
	// the one path where nothing but matchLease stands between a stale worker and
	// a requeue.
	if _, err := service.Fail(fixture.Ctx(), second.ID, FailInput{WorkerID: "p006-restarted-worker", LeaseToken: firstLease.Token, Error: "the previous attempt"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stale fail after a same-id reclaim = %v, want ErrForbidden", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE id = $1 AND status = 'running' AND attempts = 1`, second.ID); got != 1 {
		t.Fatal("the stale failure requeued a job the current claim is working on")
	}
	// The new attempt is the one that may finish the job.
	if _, err := service.Complete(fixture.Ctx(), second.ID, CompleteInput{WorkerID: "p006-restarted-worker", LeaseToken: second.LeaseToken}); err != nil {
		t.Fatalf("the current claim could not complete: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE id = $1 AND status = 'succeeded' AND lease_token IS NULL`, enqueued.Job.ID); got != 1 {
		t.Fatalf("succeeded jobs with the lease released = %d, want 1", got)
	}
}

// TestHeartbeatCancelsTheWorkItWasGiven proves the failure is caught while it
// can still stop something, rather than at the end of a long write. The lease is
// revoked underneath the worker and the context the work was handed comes back
// cancelled.
func TestHeartbeatCancelsTheWorkItWasGiven(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	service.HeartbeatInterval = 20 * time.Millisecond
	jobType := "p006_heartbeat_" + fixture.Tag()
	enqueueLeaseJob(t, fixture, service, jobType)
	claimed := claimLeaseJob(t, fixture, service, jobType, "p006-heartbeat-worker")
	lease := claimed.Lease("p006-heartbeat-worker")

	heartbeat, workCtx, err := service.StartHeartbeat(fixture.Ctx(), lease)
	if err != nil {
		t.Fatalf("start heartbeat: %v", err)
	}
	t.Cleanup(func() { _ = heartbeat.Stop() })
	if workCtx.Err() != nil {
		t.Fatal("the work context was cancelled before anything happened")
	}

	// A reclaim is what an expired lease plus a new claim looks like, and it is
	// the moment the heartbeat must notice.
	fixture.Exec(`UPDATE jobs SET lease_token = gen_random_uuid(), locked_by = 'p006-other-worker' WHERE id = $1`, lease.JobID)

	select {
	case <-workCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the work context survived a revoked lease")
	}
	if !errors.Is(context.Cause(workCtx), ErrForbidden) && !errors.Is(context.Cause(workCtx), ErrLeaseLost) {
		t.Fatalf("work context cause = %v, want the lease loss on the chain", context.Cause(workCtx))
	}
	if err := heartbeat.Check(); err == nil {
		t.Fatal("a stopped heartbeat reported no loss")
	}
	if stopped := heartbeat.Stop(); stopped == nil {
		t.Fatal("Stop did not report the loss that ended the heartbeat")
	}
}

// TestHeartbeatKeepsALiveLeaseAlive is the other half: a worker that keeps
// renewing never loses the lease, and a job that stays alive is not recovered
// even when the recovery window is wide open around it.
func TestHeartbeatKeepsALiveLeaseAlive(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	service.HeartbeatInterval = 20 * time.Millisecond
	service.LeaseDuration = 2 * time.Second
	jobType := "p006_heartbeat_live_" + fixture.Tag()
	enqueueLeaseJob(t, fixture, service, jobType)
	claimed := claimLeaseJob(t, fixture, service, jobType, "p006-live-worker")
	lease := claimed.Lease("p006-live-worker")

	heartbeat, workCtx, err := service.StartHeartbeat(fixture.Ctx(), lease)
	if err != nil {
		t.Fatalf("start heartbeat: %v", err)
	}
	t.Cleanup(func() { _ = heartbeat.Stop() })

	// Well past the lease the worker was handed at claim time, so only a renewal
	// can be keeping it valid.
	time.Sleep(700 * time.Millisecond)
	if workCtx.Err() != nil {
		t.Fatalf("a heartbeating worker lost its lease: %v", context.Cause(workCtx))
	}
	current, err := service.Get(fixture.Ctx(), lease.JobID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if current.LeaseExpiresAt == nil || !current.LeaseExpiresAt.After(*claimed.LeaseExpiresAt) {
		t.Fatal("the lease deadline did not move out while the heartbeat was running")
	}
	// locked_at is what the fifteen-minute recovery window reads, and a heartbeat
	// refreshes it too, so a working job stays out of recovery.
	if current.LockedAt == nil || !current.LockedAt.After(*claimed.LockedAt) {
		t.Fatal("the heartbeat did not refresh locked_at, so recovery would reclaim a working job")
	}
	if _, err := service.RecoverStale(fixture.Ctx(), time.Nanosecond); err != nil {
		t.Fatalf("recover stale: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE id = $1 AND status = 'running'`, lease.JobID); got != 1 {
		t.Fatal("recovery reclaimed a job whose worker was heartbeating")
	}
	if err := heartbeat.Stop(); err != nil {
		t.Fatalf("a healthy heartbeat reported a loss on stop: %v", err)
	}
	if err := heartbeat.Check(); err != nil {
		t.Fatalf("a stopped heartbeat reported a loss: %v", err)
	}
}

// TestHeartbeatRefusesAnIncompleteLease keeps the fencing from being optional:
// a caller that cannot be fenced is told so at the point of starting work rather
// than discovering it at the first write.
func TestHeartbeatRefusesAnIncompleteLease(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	for name, lease := range map[string]Lease{
		"no job":     {WorkerID: "p006-worker", Token: uuid.NewString()},
		"no worker":  {JobID: uuid.NewString(), Token: uuid.NewString()},
		"no token":   {JobID: uuid.NewString(), WorkerID: "p006-worker"},
		"blank job":  {JobID: "   ", WorkerID: "p006-worker", Token: uuid.NewString()},
		"unparsable": {JobID: "not-a-uuid", WorkerID: "p006-worker", Token: uuid.NewString()},
	} {
		if _, _, err := service.StartHeartbeat(fixture.Ctx(), lease); err == nil {
			t.Fatalf("StartHeartbeat with %s = nil error, want a refusal", name)
		}
	}
}

func enqueueLeaseJob(t *testing.T, fixture *testsupport.Fixture, service *Service, jobType string) EnqueueResult {
	t.Helper()
	result, err := service.Enqueue(fixture.Ctx(), EnqueueInput{
		Type:           jobType,
		Payload:        []byte(`{"fixture":"lease"}`),
		IdempotencyKey: fixture.Unique("key"),
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return result
}

func claimLeaseJob(t *testing.T, fixture *testsupport.Fixture, service *Service, jobType, workerID string) JobView {
	t.Helper()
	claimed, err := service.Claim(fixture.Ctx(), ClaimInput{WorkerID: workerID, Type: jobType})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.LeaseToken == "" {
		t.Fatal("the claim came with no lease token, so nothing downstream can be fenced")
	}
	return claimed
}

// waitFor polls a boolean query until it holds. Everything that can make a lease
// observable here is the database's own clock, so the test waits on the
// database rather than on a sleep it hopes is long enough.
func waitFor(t *testing.T, fixture *testsupport.Fixture, query string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var holds bool
		if err := fixture.QueryRow(query, args...).Scan(&holds); err != nil {
			t.Fatalf("poll %s: %v", query, err)
		}
		if holds {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", query)
}
