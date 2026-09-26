package researchagent

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

// The resume story. An investigation is eleven stages long and a stage can fail;
// what must be true afterwards is that the stages which did run are still there,
// that the retry continues from the first one that did not, and that the finished
// investigation is the eleven-stage plan rather than twenty-two stages' worth of
// repeats.

// TestAnInvestigationResumesFromWhereItStopped is the plan's own verification for
// this step: all the stages run, a stage fails in the middle, the run is retried,
// and the result is a complete and traceable investigation rather than a partial
// one or a duplicated one.
func TestAnInvestigationResumesFromWhereItStopped(t *testing.T) {
	fixture := testsupport.New(t)
	requester := actor.Register(t, fixture, "صاحب السؤال")
	fixture.GrantRole(requester.User.ID, "researcher")
	question := seedResolvableQuestion(t, fixture, requester.User.ID)
	queue := jobs.NewService(fixture.Pool())
	queue.HeartbeatInterval = 50 * time.Millisecond
	service := NewService(fixture.Pool()).WithQueue(queue)

	accepted, err := service.StartRun(fixture.Ctx(), requester.User.ID, question)
	if err != nil {
		t.Fatalf("accept the investigation: %v", err)
	}

	// The third stage is refused by the database, which is as close to a real
	// mid-investigation failure as a test can arrange deterministically: the stages
	// before it commit, it does not, and the process does not know why.
	refuseStageWrites(t, fixture, StageSearchGraph)
	lease := claimInvestigation(t, fixture, queue, accepted.JobID, "p006-agent-first")
	err = service.ProcessRun(fixture.Ctx(), lease, accepted.ID)
	if err == nil {
		t.Fatal("the investigation reported success although a stage write was refused")
	}
	if _, failErr := queue.Fail(fixture.Ctx(), lease.JobID, jobs.FailInput{WorkerID: lease.WorkerID, LeaseToken: lease.Token, Error: err.Error()}); failErr != nil {
		t.Fatalf("fail the investigation job: %v", failErr)
	}
	fixture.Exec(`UPDATE jobs SET run_at = now() WHERE id = $1`, accepted.JobID)

	failed, err := service.GetRun(fixture.Ctx(), requester.User.ID, accepted.ID)
	if err != nil {
		t.Fatalf("read the failed run: %v", err)
	}
	if failed.Status != RunFailed {
		t.Fatalf("run = %s, want %s", failed.Status, RunFailed)
	}
	if failed.Error == "" || !strings.Contains(failed.Error, "refused by test") {
		t.Fatalf("run error %q does not carry the reason the stage was refused for", failed.Error)
	}
	// The steps that ran are still on the row. This is the whole reason the stages
	// are committed one at a time, so it is the first thing asserted.
	committed := stageNames(t, fixture, accepted.ID)
	if len(committed) != 2 || committed[0] != StageDecompose || committed[1] != StageSearchSources {
		t.Fatalf("stages committed before the failure = %v, want the two before the refused one", committed)
	}
	if failed.EvidenceCount != 0 || len(failed.Gaps) != 0 {
		t.Fatalf("a failed investigation reported a package: %+v", failed)
	}
	// And no report was written, because a report is what says the work is done.
	if got := fixture.Count(`SELECT count(*) FROM research_agent_gaps WHERE run_id = $1`, accepted.ID); got != 0 {
		t.Fatalf("gaps on a failed investigation = %d, want 0", got)
	}

	// The retry, with the obstruction gone, finishes the investigation.
	allowStageWrites(t, fixture)
	fixture.Exec(`UPDATE jobs SET run_at = now() WHERE id = $1`, accepted.JobID)
	retry := claimInvestigation(t, fixture, queue, accepted.JobID, "p006-agent-retry")
	if err := service.ProcessRun(fixture.Ctx(), retry, accepted.ID); err != nil {
		t.Fatalf("the retry did not finish the investigation: %v", err)
	}
	if _, err := queue.Complete(fixture.Ctx(), retry.JobID, jobs.CompleteInput{WorkerID: retry.WorkerID, LeaseToken: retry.Token}); err != nil {
		t.Fatalf("complete the investigation job: %v", err)
	}

	finished, err := service.GetRun(fixture.Ctx(), requester.User.ID, accepted.ID)
	if err != nil {
		t.Fatalf("read the finished run: %v", err)
	}
	if finished.Status != RunSucceeded {
		t.Fatalf("the retried run = %s (%s)", finished.Status, finished.Error)
	}
	// Eleven steps, not fourteen: the three the first attempt committed were not run
	// again, which is what "resume" means here.
	if len(finished.Steps) != MaximumSteps {
		t.Fatalf("steps = %d, want the plan's %d with no repeats", len(finished.Steps), MaximumSteps)
	}
	for index, step := range finished.Steps {
		if step.Order != index+1 {
			t.Fatalf("step %d has order %d, want the plan's order", index, step.Order)
		}
	}
	// The stages the retry ran are the ones the first attempt did not.
	after := stageNames(t, fixture, accepted.ID)
	for _, stage := range []string{StageCompareClaims, StageCounterEvidence, StageEvidencePackage, StageRecommendation} {
		if !containsStage(after, stage) {
			t.Fatalf("stage %s missing from the resumed run: %v", stage, after)
		}
	}
	// Every citation still points at a step and a source, which is the traceability
	// the report promises and the thing a partial run would have broken.
	orphans := fixture.Count(`SELECT count(*) FROM research_agent_evidence e WHERE e.run_id = $1 AND NOT EXISTS (SELECT 1 FROM research_agent_steps s WHERE s.id = e.step_id)`, accepted.ID)
	if orphans != 0 {
		t.Fatalf("evidence rows with no step = %d, want 0", orphans)
	}
	stored := fixture.Count(`SELECT count(*) FROM research_agent_evidence WHERE run_id = $1`, accepted.ID)
	if stored != finished.EvidenceCount || stored == 0 {
		t.Fatalf("the run reports %d evidence items and the row holds %d", finished.EvidenceCount, stored)
	}
	if finished.Report.AnswerAR == "" {
		t.Fatal("the resumed investigation produced no answer")
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'research_agent_run_completed' AND entity_id = $1 AND actor_id = $2`, accepted.ID, requester.User.ID); got != 1 {
		t.Fatalf("completion audit rows naming the requester = %d, want 1: the failed attempt must not have written one", got)
	}
}

// TestAnInvestigationStaysReadOnlyWhenItRunsInAWorker is the permission restriction
// that has to survive being moved into a process that runs without a request
// behind it: a worker that finds a recommendation must still leave it suggested, and
// a gap must still be a gap.
func TestAnInvestigationStaysReadOnlyWhenItRunsInAWorker(t *testing.T) {
	fixture := testsupport.New(t)
	requester := actor.Register(t, fixture, "صاحب السؤال")
	fixture.GrantRole(requester.User.ID, "researcher")
	question := seedResolvableQuestion(t, fixture, requester.User.ID)
	queue := jobs.NewService(fixture.Pool())
	queue.HeartbeatInterval = 50 * time.Millisecond
	service := NewService(fixture.Pool()).WithQueue(queue)

	accepted, err := service.StartRun(fixture.Ctx(), requester.User.ID, question)
	if err != nil {
		t.Fatalf("accept the investigation: %v", err)
	}
	lease := claimInvestigation(t, fixture, queue, accepted.JobID, "p006-agent-readonly")
	if err := service.ProcessRun(fixture.Ctx(), lease, accepted.ID); err != nil {
		t.Fatalf("run the investigation: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM research_agent_recommendations WHERE run_id = $1 AND status <> 'suggested'`, accepted.ID); got != 0 {
		t.Fatalf("the worker moved %d recommendation(s) out of suggested; only a person may", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM research_agent_gaps WHERE run_id = $1 AND status <> 'open'`, accepted.ID); got != 0 {
		t.Fatalf("the worker closed %d gap(s); only a person may", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM open_questions WHERE id IN (SELECT question_id FROM research_question_candidates WHERE run_id = $1)`, accepted.ID); got != 0 {
		t.Fatal("the worker created an open question from a candidate without a person")
	}
	finished, err := service.GetRun(fixture.Ctx(), requester.User.ID, accepted.ID)
	if err != nil {
		t.Fatalf("read the run: %v", err)
	}
	if finished.ExecutionMode != ExecutionModeAsynchronous {
		t.Fatalf("execution mode = %s, want %s", finished.ExecutionMode, ExecutionModeAsynchronous)
	}
	// The report keeps naming the actions the agent may not take, which is the
	// statement a reader relies on to know the package is not a verdict.
	for _, restricted := range []string{"publish_tree", "accept_claim", "merge_people", "modify_source_evidence"} {
		if !containsStage(finished.Report.RestrictedActions, restricted) {
			t.Fatalf("the report no longer names %q as restricted: %v", restricted, finished.Report.RestrictedActions)
		}
	}
}

// TestAWorkerWithoutAClaimCannotWriteTheInvestigation is the fencing half of this
// step. A worker whose claim was taken over cannot commit a stage, and the stage it
// did not commit stays missing rather than being written by the wrong attempt.
func TestAWorkerWithoutAClaimCannotWriteTheInvestigation(t *testing.T) {
	fixture := testsupport.New(t)
	requester := actor.Register(t, fixture, "صاحب السؤال")
	fixture.GrantRole(requester.User.ID, "researcher")
	question := seedResolvableQuestion(t, fixture, requester.User.ID)
	queue := jobs.NewService(fixture.Pool())
	queue.HeartbeatInterval = 50 * time.Millisecond
	service := NewService(fixture.Pool()).WithQueue(queue)

	accepted, err := service.StartRun(fixture.Ctx(), requester.User.ID, question)
	if err != nil {
		t.Fatalf("accept the investigation: %v", err)
	}
	lease := claimInvestigation(t, fixture, queue, accepted.JobID, "p006-agent-displaced")
	fixture.Exec(`UPDATE jobs SET lease_token = gen_random_uuid(), locked_by = 'p006-taker' WHERE id = $1`, lease.JobID)

	err = service.ProcessRun(fixture.Ctx(), lease, accepted.ID)
	if err == nil {
		t.Fatal("a worker that lost its claim ran the investigation anyway")
	}
	if !errors.Is(err, jobs.ErrLeaseLost) && !errors.Is(err, jobs.ErrForbidden) {
		t.Fatalf("error = %v, want a lease refusal", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM research_agent_steps WHERE run_id = $1`, accepted.ID); got != 0 {
		t.Fatalf("a worker without the claim wrote %d stage(s)", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM research_agent_evidence WHERE run_id = $1`, accepted.ID); got != 0 {
		t.Fatalf("a worker without the claim wrote %d evidence row(s)", got)
	}
	// The run is not marked failed either, because the failure was never this
	// attempt's to record.
	var status string
	fixture.QueryRow(`SELECT status FROM research_agent_runs WHERE id = $1`, accepted.ID).Scan(&status)
	if status == RunFailed {
		t.Fatal("a displaced worker marked the run failed over the attempt that owns it now")
	}
}

func claimInvestigation(t *testing.T, fixture *testsupport.Fixture, queue *jobs.Service, jobID, workerID string) jobs.Lease {
	t.Helper()
	claimed, err := queue.Claim(fixture.Ctx(), jobs.ClaimInput{WorkerID: workerID, Type: JobType})
	if err != nil {
		t.Fatalf("claim the investigation job: %v", err)
	}
	if claimed.ID != jobID {
		t.Fatalf("claimed job %s, want the run's own %s", claimed.ID, jobID)
	}
	return claimed.Lease(workerID)
}

func stageNames(t *testing.T, fixture *testsupport.Fixture, runID string) []string {
	t.Helper()
	rows, err := fixture.Pool().Query(fixture.Ctx(), `SELECT stage FROM research_agent_steps WHERE run_id = $1 ORDER BY step_order`, runID)
	if err != nil {
		t.Fatalf("read the committed stages: %v", err)
	}
	defer rows.Close()
	names := make([]string, 0)
	for rows.Next() {
		var stage string
		if err := rows.Scan(&stage); err != nil {
			t.Fatal(err)
		}
		names = append(names, stage)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func containsStage(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// refuseStageWrites makes the database reject the step rows of one stage, and
// allowStageWrites removes the obstruction. Naming the stage rather than the row is
// what makes the failure land in the middle of the plan instead of at its edge.
func refuseStageWrites(t *testing.T, fixture *testsupport.Fixture, stage string) {
	t.Helper()
	fixture.Exec(`
		CREATE OR REPLACE FUNCTION p006_refuse_stage_writes() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.stage = TG_ARGV[0] THEN
				RAISE EXCEPTION 'stage write for % refused by test', TG_ARGV[0];
			END IF;
			RETURN NEW;
		END;
		$$
	`)
	fixture.Exec(`
		CREATE TRIGGER p006_refuse_stage_writes
		BEFORE INSERT ON research_agent_steps
		FOR EACH ROW EXECUTE FUNCTION p006_refuse_stage_writes('` + stage + `')
	`)
	t.Cleanup(func() { allowStageWrites(t, fixture) })
}

func allowStageWrites(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	// The test's context is already cancelled by the time a cleanup runs after a
	// failure, and a trigger that outlives its test would refuse the next
	// investigation's stages.
	if _, err := fixture.Pool().Exec(context.Background(), `DROP TRIGGER IF EXISTS p006_refuse_stage_writes ON research_agent_steps`); err != nil {
		t.Errorf("drop the refusal trigger: %v", err)
	}
}

// seedResolvableQuestion creates a person, an open question about them, and one
// public independent statement that contradicts a claim about them - the smallest
// world in which the investigation has something it is allowed to cite.
func seedResolvableQuestion(t *testing.T, fixture *testsupport.Fixture, ownerID string) RunInput {
	t.Helper()
	person := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar) VALUES ($1, 'عبد الله بن عبد الرحمن', 'عبداللهبنعبدالرحمن')`, person)
	place := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type) VALUES ($1, 'المدينة', 'المدينة', 'city')`, place)
	question := uuid.New()
	fixture.Exec(`INSERT INTO open_questions (id, title_ar, description_ar, status, created_by) VALUES ($1, 'أين هاجر عبد الله؟', 'أين كانت الهجرة؟', 'open', $2)`, question, ownerID)
	source := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, dependency_status, created_by) VALUES ($1, 'المصادر المنقولة', 'manuscript', 'public', 'independent', $2)`, source, ownerID)
	statement := uuid.New()
	fixture.Exec(`INSERT INTO source_statements (id, source_id, statement_text_ar, review_status) VALUES ($1, $2, 'هاجر عبد الله إلى المدينة.', 'accepted')`, statement, source)
	claim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, place_id, status) VALUES ($1, 'person', $2, 'hijra_to', 'place', $3, $3, 'contested')`, claim, person, place)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation) VALUES ($1, $2, 'contradicts')`, claim, statement)
	return RunInput{Question: "أين هاجر عبد الله؟", QuestionID: question.String(), EntityType: "person", EntityID: person.String()}
}
