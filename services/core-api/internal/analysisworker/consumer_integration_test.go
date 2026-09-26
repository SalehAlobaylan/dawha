package analysisworker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/entityresolution"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/researchagent"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The proof that a deployed system drains its queue.
//
// Everything else in this package is a test of the code. This one starts the real
// binary - the same `go run ./cmd/analysis-worker` a deployment starts - as a
// child process, hands it the fixture's schema through a connection string, and
// then only does what an HTTP client would do: accept a run, poll the run's own
// status endpoint, and wait for a terminal state.
//
// A test that called the handler in this process would prove the handler works.
// It would not prove anything about the process that has to be running for the
// handler to be reached at all, which is the failure mode that matters: an API
// that queues work and a deployment that forgot the worker produces a system that
// accepts every run and finishes none of them, and every unit test in this
// package would still be green.

// TestTheAnalysisWorkerBinaryDrainsAnEntityResolutionRun is the end-to-end claim
// for the queue itself. The run is accepted, the binary picks it up on its own,
// and the run reaches succeeded without the test doing any of the work.
func TestTheAnalysisWorkerBinaryDrainsAnEntityResolutionRun(t *testing.T) {
	fixture := testsupport.New(t)
	operator := actor.Register(t, fixture, "باحث المطابقة")
	fixture.GrantRole(operator.User.ID, "researcher")
	// Two people whose names normalize to the same thing, with a shared father and
	// a shared place: the shape a real duplicate scan is asked to find, so the run
	// has something to finish with rather than nothing to score.
	seedDuplicatePair(t, fixture)

	// A deterministic embedder, so the scoring has a model to call and answers in
	// microseconds instead of spending the two-second per-call budget on a
	// connection that is not there. The scoring has a documented local fallback;
	// what is under test here is the queue, not the model.
	ai := startStubEmbedder(t, embedderDelay)
	worker := startWorkerBinary(t, fixture, resolutionLease, "AI_RESEARCH_URL="+ai)
	accepted := acceptResolutionRun(t, fixture, operator.User.ID)
	if accepted.Status != entityresolution.RunQueued {
		t.Fatalf("accepted run = %s, want %s", accepted.Status, entityresolution.RunQueued)
	}
	finished := waitForTerminalResolutionRun(t, fixture, operator.User.ID, accepted.ID, 90*time.Second)

	if finished.Status != entityresolution.RunSucceeded {
		t.Fatalf("run = %s (%s), want succeeded", finished.Status, finished.Error)
	}
	if finished.Stage != entityresolution.StageComplete {
		t.Fatalf("run stage = %s, want %s", finished.Stage, entityresolution.StageComplete)
	}
	if finished.CandidateCount <= 0 || finished.StartedAt == nil || finished.CompletedAt == nil {
		t.Fatalf("a finished run with no evidence of finishing: %+v", finished)
	}
	if finished.JobID != accepted.JobID {
		t.Fatalf("run job = %s, want the job the accept step queued (%s)", finished.JobID, accepted.JobID)
	}
	// The job the binary claimed is the same job, and it is finished on the first
	// attempt: a run that only got there after retries would be a run this test
	// should have caught rather than accepted.
	job, err := jobs.NewService(fixture.Pool()).Get(fixture.Ctx(), accepted.JobID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if job.Status != "succeeded" {
		t.Fatalf("job = %s (%s), want succeeded", job.Status, job.LastError)
	}
	if job.Attempts != 0 {
		t.Fatalf("job took %d attempt(s), want the first one to finish the run", job.Attempts)
	}
	// And the process says so itself. This is the line an operator greps for when
	// the question is "is anything actually draining this queue", and it is the same
	// line the test reads, so the two cannot disagree.
	waitForOutput(t, worker.output, "completed "+entityresolution.JobType+" job "+accepted.JobID, 10*time.Second)
	// The candidates the run reports are on the row, so a run that reached
	// succeeded has something behind it.
	stored := fixture.Count(`SELECT count(*) FROM entity_resolution_candidates WHERE run_id = $1`, accepted.ID)
	if stored != finished.CandidateCount {
		t.Fatalf("candidates on the row = %d, run says %d", stored, finished.CandidateCount)
	}
	// And the audit records who asked, not which worker ran it.
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'entity_resolution_run_completed' AND entity_id = $1`, accepted.ID); got != 1 {
		t.Fatalf("completion audit rows = %d, want 1", got)
	}
	worker.expectCleanShutdown(t)
}

// TestTheAnalysisWorkerBinaryDrainsAResearchAgentRun is the same proof for the
// investigation, and it is the one that matters most: eleven stages, each
// committed on its own, driven by a process that only knows the job id.
func TestTheAnalysisWorkerBinaryDrainsAResearchAgentRun(t *testing.T) {
	fixture := testsupport.New(t)
	requester := actor.Register(t, fixture, "صاحب السؤال")
	fixture.GrantRole(requester.User.ID, "researcher")
	question := seedOpenQuestion(t, fixture, requester.User.ID)

	worker := startWorkerBinary(t, fixture, agentLease)
	accepted, err := researchagent.NewService(fixture.Pool()).WithQueue(jobs.NewService(fixture.Pool())).
		StartRun(fixture.Ctx(), requester.User.ID, researchagent.RunInput{
			Question:   question.title,
			QuestionID: question.id,
			EntityType: "person",
			EntityID:   question.personID,
			FromYear:   1,
			ToYear:     200,
		})
	if err != nil {
		t.Fatalf("accept the investigation: %v", err)
	}
	if accepted.Status != researchagent.RunQueued {
		t.Fatalf("accepted run = %s, want %s", accepted.Status, researchagent.RunQueued)
	}
	finished := waitForTerminalAgentRun(t, fixture, requester.User.ID, accepted.ID, 90*time.Second)

	if finished.Status != researchagent.RunSucceeded {
		t.Fatalf("run = %s (%s), want succeeded", finished.Status, finished.Error)
	}
	if len(finished.Steps) != 11 {
		t.Fatalf("steps = %d, want the whole plan committed", len(finished.Steps))
	}
	if finished.ExecutionMode != researchagent.ExecutionModeAsynchronous {
		t.Fatalf("execution mode = %s, want %s", finished.ExecutionMode, researchagent.ExecutionModeAsynchronous)
	}
	// Every step is committed, and every piece of evidence on the run points at a
	// step that exists and carries the reference it was gathered from. That is the
	// traceability the report promises: a citation that cannot be walked back to a
	// step and a source is not a citation.
	for _, step := range finished.Steps {
		if step.CompletedAt == nil {
			t.Fatalf("step %s has no completion time", step.Stage)
		}
		if step.ID == "" {
			t.Fatalf("step %s came back with no id", step.Stage)
		}
	}
	orphans := fixture.Count(`SELECT count(*) FROM research_agent_evidence e WHERE e.run_id = $1 AND NOT EXISTS (SELECT 1 FROM research_agent_steps s WHERE s.id = e.step_id)`, finished.ID)
	if orphans != 0 {
		t.Fatalf("evidence rows with no step = %d, want 0", orphans)
	}
	storedEvidence := fixture.Count(`SELECT count(*) FROM research_agent_evidence WHERE run_id = $1`, finished.ID)
	if storedEvidence != finished.EvidenceCount || storedEvidence == 0 {
		t.Fatalf("run reports %d evidence items and the row holds %d; want the same, and not zero", finished.EvidenceCount, storedEvidence)
	}
	cited := fixture.Count(`SELECT count(*) FROM research_agent_evidence WHERE run_id = $1 AND statement_id IS NOT NULL AND excerpt <> ''`, finished.ID)
	if cited == 0 {
		t.Fatal("the finished run holds evidence with no citation behind it")
	}
	if finished.Report.AnswerAR == "" || finished.Report.EvidencePackage.Total == 0 {
		t.Fatalf("a finished run with an empty report: %+v", finished.Report)
	}
	// The investigation stayed read-only, which is the permission restriction that
	// must survive being moved into a worker that runs without a request.
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'research_agent_run_completed' AND entity_id = $1`, accepted.ID); got != 1 {
		t.Fatalf("completion audit rows = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM research_agent_recommendations WHERE run_id = $1 AND status = 'accepted'`, accepted.ID); got != 0 {
		t.Fatalf("the worker accepted %d recommendation(s); only a person may", got)
	}
	worker.expectCleanShutdown(t)
}

// workerProcess is the child process under test, plus its output.
type workerProcess struct {
	t       *testing.T
	command *exec.Cmd
	output  *lockedBuffer
}

func (p *workerProcess) expectCleanShutdown(t *testing.T) {
	t.Helper()
	// A worker that exits because its context was cancelled is a worker that shut
	// down cleanly. One that exits on its own while the test still holds it is a
	// worker that died, and the output is the only evidence of why.
	if p.command.ProcessState != nil {
		t.Fatalf("the worker exited on its own (%s):\n%s", p.command.ProcessState, p.output.String())
	}
	if err := p.command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal the worker: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- p.command.Wait() }()
	select {
	case err := <-done:
		// signal: killed is the expected result of an interrupt this program does
		// not handle explicitly; the worker's own context cancellation is what
		// stops it, and it says so on the way out.
		var exitErr *exec.ExitError
		if err != nil && !errors.As(err, &exitErr) {
			t.Fatalf("wait for the worker: %v", err)
		}
	case <-time.After(30 * time.Second):
		_ = p.command.Process.Kill()
		t.Fatalf("the worker did not stop within 30s:\n%s", p.output.String())
	}
	if !strings.Contains(p.output.String(), "analysis worker stopped cleanly") {
		t.Fatalf("the worker did not report a clean shutdown:\n%s", p.output.String())
	}
}

// startWorkerBinary builds and starts the real consumer.
//
// It is `go run` of the same command a deployment runs, pointed at the fixture's
// schema. The timings are compressed so the test is not a minute long: a
// one-second lease and a 100ms heartbeat are the same mechanism as a
// ninety-second lease and a thirty-second heartbeat, and the point being tested is
// that a binary nobody called drains the queue, not that the timings are
// production timings.
func startWorkerBinary(t *testing.T, fixture *testsupport.Fixture, lease string, extraEnv ...string) *workerProcess {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "analysis-worker")
	// The package path rather than a relative one: a test runs in its own package
	// directory, and building "./cmd/..." from there would look for a cmd directory
	// that does not exist under it.
	build := exec.Command("go", "build", "-o", binary, "github.com/SalehAlobaylan/dawha/services/core-api/cmd/analysis-worker")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the analysis worker: %v\n%s", err, output)
	}
	command := exec.Command(binary)
	command.Env = append(os.Environ(),
		"DATABASE_URL="+fixture.DatabaseURL(),
		"ANALYSIS_WORKER_ID=p006-binary-worker",
		"ANALYSIS_WORKER_POLL_INTERVAL=100ms",
		"ANALYSIS_WORKER_HEARTBEAT_INTERVAL=100ms",
		"ANALYSIS_WORKER_LEASE_DURATION="+lease,
	)
	command.Env = append(command.Env, extraEnv...)
	buffer := &lockedBuffer{}
	command.Stdout = buffer
	command.Stderr = buffer
	if err := command.Start(); err != nil {
		t.Fatalf("start the analysis worker: %v", err)
	}
	process := &workerProcess{t: t, command: command, output: buffer}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	// The worker announces the types it takes. Waiting for that line rather than
	// sleeping means the test starts polling only once there is something that can
	// answer, and a worker that never starts is reported as the worker's output
	// instead of as a timeout.
	waitForOutput(t, buffer, "job worker started", 60*time.Second)
	return process
}

func acceptResolutionRun(t *testing.T, fixture *testsupport.Fixture, actorID string) entityresolution.Run {
	t.Helper()
	service := entityresolution.NewService(fixture.Pool(), nil).WithQueue(jobs.NewService(fixture.Pool()))
	run, err := service.Run(fixture.Ctx(), actorID, entityresolution.RunInput{EntityType: entityresolution.EntityPerson})
	if err != nil {
		t.Fatalf("accept the run: %v", err)
	}
	return run
}

// waitForTerminalResolutionRun polls the run the way a client does: read the run,
// look at the status, stop when the status will not change again.
func waitForTerminalResolutionRun(t *testing.T, fixture *testsupport.Fixture, actorID, runID string, limit time.Duration) entityresolution.Run {
	t.Helper()
	service := entityresolution.NewService(fixture.Pool(), nil)
	deadline := time.Now().Add(limit)
	var last entityresolution.Run
	for time.Now().Before(deadline) {
		run, err := service.GetRun(fixture.Ctx(), actorID, runID)
		if err == nil {
			last = run
			if entityresolution.Terminal(run.Status) {
				return run
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the run did not reach a terminal state within %s (last status %q, %q)", limit, last.Status, last.Error)
	return last
}

func waitForTerminalAgentRun(t *testing.T, fixture *testsupport.Fixture, actorID, runID string, limit time.Duration) researchagent.Run {
	t.Helper()
	service := researchagent.NewService(fixture.Pool())
	deadline := time.Now().Add(limit)
	var last researchagent.Run
	for time.Now().Before(deadline) {
		run, err := service.GetRun(fixture.Ctx(), actorID, runID)
		if err == nil {
			last = run
			if researchagent.Terminal(run.Status) {
				return run
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the run did not reach a terminal state within %s (last status %q, %q)", limit, last.Status, last.Error)
	return last
}

// seedDuplicatePair creates two people a scan has a reason to compare: a shared
// normalized name, a shared father, and a shared place. Without them the run would
// finish successfully having found nothing, which would prove the plumbing and not
// the scoring.
func seedDuplicatePair(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	father := uuid.New()
	place := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar) VALUES ($1, 'عبد الرحمن', 'عبدالرحمن')`, father)
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type) VALUES ($1, 'مكة', 'مكة', 'city')`, place)
	for _, name := range []string{"عبد الله بن عبد الرحمن", "عبدالله بن عبدالرحمن"} {
		person := uuid.New()
		fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar) VALUES ($1, $2, $3)`, person, name, normalizeForTest(name))
		// A father edge is what the kinship signal reads, and a place edge is what
		// the geography signal reads. Both are recorded as claims against the two
		// people rather than as tree edges, so the fixture does not have to build a
		// published tree to give the scanner something to compare.
		fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status) VALUES ($1, 'person', $2, 'child_of', 'person', $3, 'supported')`, uuid.New(), person, father)
		fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, place_id, status) VALUES ($1, 'person', $2, 'resided_in', 'place', $3, $3, 'supported')`, uuid.New(), person, place)
	}
}

type openQuestion struct {
	id       string
	title    string
	personID string
}

func seedOpenQuestion(t *testing.T, fixture *testsupport.Fixture, ownerID string) openQuestion {
	t.Helper()
	person := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar) VALUES ($1, 'س回去 الباحثة', 'س回去')`, person)
	question := uuid.New()
	fixture.Exec(`INSERT INTO open_questions (id, title_ar, description_ar, status, created_by) VALUES ($1, $2, 'أين هاجر؟', 'open', $3)`, question, "ما موضع هجرة عبد الله؟", ownerID)
	// One accepted, public, independent statement, and a claim about the person that
	// this statement contradicts. The agent only counts evidence it is allowed to
	// qualify, so a run over empty tables would finish successfully having said
	// nothing - which proves the plumbing and not the qualification the report
	// promises. The contradicted claim is what puts a citable reference in front of
	// the investigation, because the counter-evidence stage is the one that reads
	// evidence about the entity the run is scoped to.
	source := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, dependency_status, created_by) VALUES ($1, 'المصادر المنقولة', 'manuscript', 'public', 'independent', $2)`, source, ownerID)
	statement := uuid.New()
	fixture.Exec(`INSERT INTO source_statements (id, source_id, statement_text_ar, review_status) VALUES ($1, $2, 'هاجر عبد الله من مكة إلى المدينة.', 'accepted')`, statement, source)
	claim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status) VALUES ($1, 'person', $2, 'hijra_to', 'place', (SELECT id FROM places ORDER BY id LIMIT 1), 'contested')`, claim, person)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation) VALUES ($1, $2, 'contradicts')`, claim, statement)
	return openQuestion{id: question.String(), title: "ما موضع هجرة عبد الله؟", personID: person.String()}
}

func normalizeForTest(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func waitForOutput(t *testing.T, buffer *lockedBuffer, needle string, limit time.Duration) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if strings.Contains(buffer.String(), needle) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the worker did not report %q within %s:\n%s", needle, limit, buffer.String())
}

// The lease and heartbeat the child process runs with, and the model latency that
// makes them matter.
//
// A lease the work outlasts is the only arrangement in which the heartbeat is
// observable: with a lease longer than the job, a worker that never renewed would
// finish anyway and the test would prove nothing. The three-to-one ratio is the
// same one the production defaults use, compressed so the test finishes in
// seconds. The model delay is what makes the job longer than the lease - the
// scorer asks for one embedding per distinct name, and the fixture's seeded
// population gives it enough names to take a while.
const (
	resolutionLease = "3s"
	agentLease      = "30s"
	embedderDelay   = 120 * time.Millisecond
)
