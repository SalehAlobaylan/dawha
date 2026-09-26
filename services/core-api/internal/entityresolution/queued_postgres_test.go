package entityresolution

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The queued lifecycle of a scan, end to end inside one test: accepted, worked,
// finished. Every assertion is about a state a caller can observe, because the
// whole point of moving the work behind the queue is that the states a caller sees
// are honest - accepted work reads as accepted, and finished work reads as
// finished.

// TestARunIsQueuedBeforeAnyWorkIsDone is the promise the HTTP response now makes.
// A call that used to return a finished run returns a queued one, in the same
// shape, and there is nothing behind it yet: no blocks, no candidates, and a run
// status that says so.
func TestARunIsQueuedBeforeAnyWorkIsDone(t *testing.T) {
	fixture := testsupport.New(t)
	researcher := actor.Register(t, fixture, "باحث المطابقة")
	fixture.GrantRole(researcher.User.ID, "researcher")
	queue := jobs.NewService(fixture.Pool())
	service := NewService(fixture.Pool(), nil).WithQueue(queue)

	run, err := service.Run(fixture.Ctx(), researcher.User.ID, RunInput{EntityType: EntityPerson})
	if err != nil {
		t.Fatalf("accept the run: %v", err)
	}
	if run.Status != RunQueued || run.Stage != StageQueued {
		t.Fatalf("accepted run = %s/%s, want %s/%s", run.Status, run.Stage, RunQueued, StageQueued)
	}
	if run.JobID == "" {
		t.Fatal("an accepted run has no job behind it")
	}
	if run.CandidateCount != 0 || run.ModelVersion != "" || run.Error != "" {
		t.Fatalf("a queued run reported a result: %+v", run)
	}
	if run.StartedAt != nil || run.CompletedAt != nil {
		t.Fatalf("a queued run has timestamps: %+v", run)
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_resolution_candidates WHERE run_id = $1`, run.ID); got != 0 {
		t.Fatalf("candidates on a queued run = %d, want 0", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_resolution_blocks WHERE run_id = $1`, run.ID); got != 0 {
		t.Fatalf("blocks on a queued run = %d, want 0", got)
	}
	// The job exists, is queued, and is the one the run points at.
	job, err := queue.Get(fixture.Ctx(), run.JobID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if job.Status != "queued" || job.Type != JobType {
		t.Fatalf("job = %s/%s, want queued/%s", job.Status, job.Type, JobType)
	}
	// Reading the run back gives the same answer, which is what a polling client
	// does before it has decided to stop.
	read, err := service.GetRun(fixture.Ctx(), researcher.User.ID, run.ID)
	if err != nil {
		t.Fatalf("read the run: %v", err)
	}
	if read.Status != RunQueued || Terminal(read.Status) {
		t.Fatalf("read run = %s, want a non-terminal %s", read.Status, RunQueued)
	}
}

// TestTheWorkerFinishesTheRunItQueued is the other half: claim, work, complete,
// and a run whose status, counts and candidates are all on the row. The scoring is
// deterministic and needs no model - it falls back to a local vector - so this is
// the same work the API used to do in the request thread.
func TestTheWorkerFinishesTheRunItQueued(t *testing.T) {
	fixture := testsupport.New(t)
	researcher := actor.Register(t, fixture, "باحث المطابقة")
	fixture.GrantRole(researcher.User.ID, "researcher")
	seedCompareablePair(t, fixture)
	queue := jobs.NewService(fixture.Pool())
	queue.HeartbeatInterval = 50 * time.Millisecond
	service := NewService(fixture.Pool(), nil).WithQueue(queue)

	run, err := service.Run(fixture.Ctx(), researcher.User.ID, RunInput{EntityType: EntityPerson})
	if err != nil {
		t.Fatalf("accept the run: %v", err)
	}
	driveScan(t, fixture, queue, service, run.ID, "p006-scan-worker")

	finished, err := service.GetRun(fixture.Ctx(), researcher.User.ID, run.ID)
	if err != nil {
		t.Fatalf("read the finished run: %v", err)
	}
	if finished.Status != RunSucceeded || finished.Stage != StageComplete {
		t.Fatalf("run = %s/%s, want %s/%s", finished.Status, finished.Stage, RunSucceeded, StageComplete)
	}
	if finished.CandidateCount <= 0 {
		t.Fatalf("the scan found %d candidates over a seeded duplicate pair", finished.CandidateCount)
	}
	if finished.StartedAt == nil || finished.CompletedAt == nil || finished.CompletedAt.Before(*finished.StartedAt) {
		t.Fatalf("the finished run has no usable timestamps: %+v", finished)
	}
	stored := fixture.Count(`SELECT count(*) FROM entity_resolution_candidates WHERE run_id = $1`, run.ID)
	if stored != finished.CandidateCount {
		t.Fatalf("candidates on the row = %d, the run says %d", stored, finished.CandidateCount)
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_resolution_candidates WHERE run_id = $1 AND review_status = 'pending'`, run.ID); got != stored {
		t.Fatalf("%d of %d candidates need review, want all of them: a scan never approves a merge by itself", got, stored)
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'entity_resolution_run_completed' AND entity_id = $1 AND actor_id = $2`, run.ID, researcher.User.ID); got != 1 {
		t.Fatalf("completion audit rows naming the requester = %d, want 1", got)
	}
}

// TestAFailedScanSaysWhyAndCanBeRetriedWithoutDuplicating covers both halves of
// the failure contract: the run is failed with the real reason, and the retry that
// follows does not produce a second copy of anything.
func TestAFailedScanSaysWhyAndCanBeRetriedWithoutDuplicating(t *testing.T) {
	fixture := testsupport.New(t)
	researcher := actor.Register(t, fixture, "باحث المطابقة")
	fixture.GrantRole(researcher.User.ID, "researcher")
	seedCompareablePair(t, fixture)
	queue := jobs.NewService(fixture.Pool())
	queue.HeartbeatInterval = 50 * time.Millisecond
	service := NewService(fixture.Pool(), nil).WithQueue(queue)

	run, err := service.Run(fixture.Ctx(), researcher.User.ID, RunInput{EntityType: EntityPerson})
	if err != nil {
		t.Fatalf("accept the run: %v", err)
	}
	// The scoring is blocked at the point where it would write, so the failure is
	// real and its reason is the database's.
	refuseCandidateWrites(t, fixture, "refused by test")
	job := claimScan(t, fixture, queue, run.JobID, "p006-failing-worker")
	err = service.ProcessRun(fixture.Ctx(), job, run.ID)
	if err == nil {
		t.Fatal("the scan reported success although its writes were refused")
	}
	// The worker hands the failure to the queue, which is what makes the job
	// claimable again. It presents the lease, because a worker that had lost its
	// claim has no say in what happens to the job.
	failScan(t, fixture, queue, job, err)
	failed, err := service.GetRun(fixture.Ctx(), researcher.User.ID, run.ID)
	if err != nil {
		t.Fatalf("read the failed run: %v", err)
	}
	if failed.Status != RunFailed || failed.Stage != StageRunFailed {
		t.Fatalf("run = %s/%s, want %s/%s", failed.Status, failed.Stage, RunFailed, StageRunFailed)
	}
	if failed.Error == "" {
		t.Fatal("a failed run gave no reason")
	}
	if !strings.Contains(failed.Error, "refused by test") {
		t.Fatalf("run error %q does not carry the reason the write was refused for", failed.Error)
	}
	if failed.CompletedAt == nil {
		t.Fatal("a failed run has no completion time")
	}
	// The blocks written before the refusal are still there, which is what makes
	// the retry a resume rather than a restart.
	if fixture.Count(`SELECT count(*) FROM entity_resolution_blocks WHERE run_id = $1`, run.ID) == 0 {
		t.Fatal("the failed run kept no checkpoint at all")
	}

	// The retry, with the obstruction gone, finishes the run - and the candidate
	// table holds one row per pair, because the run's own key is what makes the
	// second attempt idempotent.
	allowCandidateWrites(t, fixture)
	retry := claimScan(t, fixture, queue, run.JobID, "p006-retry-worker")
	if err := service.ProcessRun(fixture.Ctx(), retry, run.ID); err != nil {
		t.Fatalf("the retry did not finish the run: %v", err)
	}
	finished, err := service.GetRun(fixture.Ctx(), researcher.User.ID, run.ID)
	if err != nil {
		t.Fatalf("read the retried run: %v", err)
	}
	if finished.Status != RunSucceeded {
		t.Fatalf("the retried run = %s (%s)", finished.Status, finished.Error)
	}
	duplicates := fixture.Count(`
		SELECT count(*) FROM (
			SELECT entity_type, left_entity_id, right_entity_id
			FROM entity_resolution_candidates WHERE run_id = $1
			GROUP BY entity_type, left_entity_id, right_entity_id
			HAVING count(*) > 1
		) doubled
	`, run.ID)
	if duplicates != 0 {
		t.Fatalf("the retry wrote %d candidate twice", duplicates)
	}
}

// TestAnUnauthorizedActorCannotQueueAScan pins where the authorization decision
// happens. The check is inside the transaction and before the first write, so a
// refusal leaves nothing behind - not a run row, not a job, nothing to clean up and
// nothing to explain.
func TestAnUnauthorizedActorCannotQueueAScan(t *testing.T) {
	fixture := testsupport.New(t)
	ordinary := actor.Register(t, fixture, "مستخدم عادي")
	queue := jobs.NewService(fixture.Pool())
	service := NewService(fixture.Pool(), nil).WithQueue(queue)

	_, err := service.Run(fixture.Ctx(), ordinary.User.ID, RunInput{EntityType: EntityPerson})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("an ordinary user starting a scan = %v, want ErrForbidden", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_resolution_runs WHERE requested_by = $1`, ordinary.User.ID); got != 0 {
		t.Fatalf("a refused request left %d run row(s) behind", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE type = $1`, JobType); got != 0 {
		t.Fatalf("a refused request queued %d job(s)", got)
	}
	// The service without a queue says so rather than accepting a run it has
	// arranged for nobody to finish.
	if _, err := NewService(fixture.Pool(), nil).Run(fixture.Ctx(), ordinary.User.ID, RunInput{}); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("a service with no queue = %v, want ErrQueueUnavailable", err)
	}
}

// claimScan claims the scan job. A jobID of "" means "whichever scan job the queue
// has", which is the only one in these fixtures; naming it explicitly when a test
// cares is what keeps a second job from being picked up by accident.
func claimScan(t *testing.T, fixture *testsupport.Fixture, queue *jobs.Service, jobID, workerID string) jobs.Lease {
	t.Helper()
	claimed, err := queue.Claim(fixture.Ctx(), jobs.ClaimInput{WorkerID: workerID, Type: JobType})
	if err != nil {
		t.Fatalf("claim the scan job: %v", err)
	}
	if jobID != "" && claimed.ID != jobID {
		t.Fatalf("claimed job %s, want the run's own %s", claimed.ID, jobID)
	}
	return claimed.Lease(workerID)
}

func driveScan(t *testing.T, fixture *testsupport.Fixture, queue *jobs.Service, service *Service, runID, workerID string) {
	t.Helper()
	lease := claimScan(t, fixture, queue, "", workerID)
	if err := service.ProcessRun(fixture.Ctx(), lease, runID); err != nil {
		t.Fatalf("process the run: %v", err)
	}
	if _, err := queue.Complete(fixture.Ctx(), lease.JobID, jobs.CompleteInput{WorkerID: workerID, LeaseToken: lease.Token}); err != nil {
		t.Fatalf("complete the scan job: %v", err)
	}
}

// seedCompareablePair creates two people a scan has a reason to compare. A run over
// an empty database finishes successfully having found nothing, which proves the
// queue and not the scoring.
func seedCompareablePair(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	father := uuid.New()
	place := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar) VALUES ($1, 'عبد الرحمن', 'عبدالرحمن')`, father)
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type) VALUES ($1, 'مكة', 'مكة', 'city')`, place)
	for _, name := range []string{"عبد الله بن عبد الرحمن", "عبدالله بن عبدالرحمن"} {
		person := uuid.New()
		fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar) VALUES ($1, $2, $3)`, person, name, "عبداللهبنعبدالرحمن")
		fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, place_id, status) VALUES ($1, 'person', $2, 'child_of', 'person', $3, $4, 'supported')`, uuid.New(), person, father, place)
	}
}

// refuseCandidateWrites makes the database reject candidate inserts, and
// allowCandidateWrites removes the obstruction again. It is the closest thing to a
// real failure that is both deterministic and reversible: the scoring runs, the
// blocks are written, and then the write the run needs is refused.
func refuseCandidateWrites(t *testing.T, fixture *testsupport.Fixture, marker string) {
	t.Helper()
	fixture.Exec(`
		CREATE OR REPLACE FUNCTION p006_refuse_candidate_writes() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'candidate write % refused by test', TG_ARGV[0];
		END;
		$$
	`)
	fixture.Exec(`
		CREATE TRIGGER p006_refuse_candidate_writes
		BEFORE INSERT ON entity_resolution_candidates
		FOR EACH ROW EXECUTE FUNCTION p006_refuse_candidate_writes('` + marker + `')
	`)
	t.Cleanup(func() { allowCandidateWrites(t, fixture) })
}

func allowCandidateWrites(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	// The test's own context is already cancelled by the time a cleanup runs after a
	// failure, and a trigger that outlives its test would refuse the next one's
	// candidates. So the drop does not use the test context.
	if _, err := fixture.Pool().Exec(context.Background(), `DROP TRIGGER IF EXISTS p006_refuse_candidate_writes ON entity_resolution_candidates`); err != nil {
		t.Errorf("drop the refusal trigger: %v", err)
	}
}

// failScan does what the consumer loop does with a handler's error.
func failScan(t *testing.T, fixture *testsupport.Fixture, queue *jobs.Service, lease jobs.Lease, cause error) {
	t.Helper()
	failed, err := queue.Fail(fixture.Ctx(), lease.JobID, jobs.FailInput{WorkerID: lease.WorkerID, LeaseToken: lease.Token, Error: cause.Error()})
	if err != nil {
		t.Fatalf("fail the scan job: %v", err)
	}
	if failed.Status != "queued" {
		t.Fatalf("the failed job = %s, want requeued for a retry", failed.Status)
	}
	// The queue backs a retry off by a second. The backoff is tested where it
	// belongs; here it would only make the test wait, so the run time is brought
	// forward and the retry proceeds on the same terms.
	fixture.Exec(`UPDATE jobs SET run_at = now() WHERE id = $1`, lease.JobID)
}
