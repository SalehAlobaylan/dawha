package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/analysisworker"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/entityresolution"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobworker"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/researchagent"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// What a client is promised when the work moves behind the queue.
//
// Both endpoints used to answer a request with a finished run. They now answer it
// with an accepted one, and the only thing that has to stay true for a client that
// was written against the old behaviour is this: the same status code, a body with
// the same fields, and a status endpoint that already existed which reaches the same
// terminal state. Everything below is that claim, asserted through the router rather
// than through the services, because the router is where a client's status code and
// JSON shape actually come from.

// queuedEntityResolutionResponse is the shape the identity-scan POST has always
// returned. Every field the old body had is here, and the two additions are
// omitempty so a client that does not know them is unaffected.
type queuedEntityResolutionResponse struct {
	ID                   string `json:"id"`
	RequestedBy          string `json:"requestedBy"`
	EntityType           string `json:"entityType"`
	Status               string `json:"status"`
	AlgorithmVersion     string `json:"algorithmVersion"`
	NormalizationVersion string `json:"normalizationVersion"`
	ModelVersion         string `json:"modelVersion,omitempty"`
	CandidateCount       int    `json:"candidateCount"`
	Error                string `json:"error,omitempty"`
	JobID                string `json:"jobId,omitempty"`
	Stage                string `json:"stage,omitempty"`
	CreatedAt            string `json:"createdAt"`
	StartedAt            string `json:"startedAt,omitempty"`
	CompletedAt          string `json:"completedAt,omitempty"`
	UpdatedAt            string `json:"updatedAt"`
}

// TestTheIdentityScanPostKeepsItsStatusCodeAndShape is the backward-compatibility
// claim for the scan, stated as the two facts a client depends on: 201, and a body
// whose status says queued rather than pretending the work is done.
func TestTheIdentityScanPostKeepsItsStatusCodeAndShape(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	operator := newIdentitySession(t, fixture, "باحث المطابقة", "researcher")

	recorder := operator.do(router, http.MethodPost, "/api/v1/entity-resolution/runs", `{"entity_type":"person"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("start a scan = %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
	var accepted queuedEntityResolutionResponse
	decodeIdentityBody(t, recorder, &accepted)
	if accepted.Status != entityresolution.RunQueued {
		t.Fatalf("accepted scan = %q, want %q: a run that has not been worked must not read as finished", accepted.Status, entityresolution.RunQueued)
	}
	if accepted.ID == "" || accepted.JobID == "" {
		t.Fatalf("accepted scan = %+v, want a run id and the job behind it", accepted)
	}
	if accepted.EntityType != "person" || accepted.AlgorithmVersion != entityresolution.AlgorithmVersion || accepted.NormalizationVersion != entityresolution.NormalizationVersion {
		t.Fatalf("accepted scan lost the fields a client reads: %+v", accepted)
	}
	if accepted.CandidateCount != 0 || accepted.ModelVersion != "" {
		t.Fatalf("an accepted scan reported a result: %+v", accepted)
	}
	// The candidate list a client refreshes after a run is still empty rather than
	// 404: the review queue exists before anything has been scored into it.
	empty := operator.do(router, http.MethodGet, "/api/v1/entity-resolution/candidates?status=pending", "")
	if empty.Code != http.StatusOK || !json.Valid(empty.Body.Bytes()) {
		t.Fatalf("list candidates = %d, want 200 with a body: %s", empty.Code, empty.Body.String())
	}
}

// TestTheIdentityScanRunCanBePolledToATerminalState is the other half: the endpoint
// a client polls is the one that always existed, and polling it reaches a finished
// run without the client having to learn a new URL.
func TestTheIdentityScanRunCanBePolledToATerminalState(t *testing.T) {
	fixture := testsupport.New(t)
	startAnalysisConsumer(t, fixture)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	operator := newIdentitySession(t, fixture, "باحث المطابقة", "researcher")

	recorder := operator.do(router, http.MethodPost, "/api/v1/entity-resolution/runs", `{"entity_type":"person"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("start a scan = %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
	var accepted queuedEntityResolutionResponse
	decodeIdentityBody(t, recorder, &accepted)

	// Polling, which is exactly what the web client does: the same GET, until the
	// status stops changing.
	seen := make([]string, 0, 3)
	finished := pollRun(t, func() (queuedEntityResolutionResponse, string, bool) {
		read := operator.do(router, http.MethodGet, "/api/v1/entity-resolution/runs/"+accepted.ID, "")
		if read.Code != http.StatusOK {
			t.Fatalf("read the run = %d, want 200: %s", read.Code, read.Body.String())
		}
		var current queuedEntityResolutionResponse
		decodeIdentityBody(t, read, &current)
		seen = append(seen, current.Status)
		if entityresolution.Terminal(current.Status) {
			if current.Status != entityresolution.RunSucceeded {
				t.Fatalf("the run finished as %s (%s), want succeeded", current.Status, current.Error)
			}
			return current, current.Status, true
		}
		return current, current.Status, false
	}, 60*time.Second)

	// The run the client was handed is the run that finished, and the counts it was
	// promised are the counts on the row.
	if finished.CandidateCount <= 0 {
		t.Fatalf("the finished run reported %d candidates", finished.CandidateCount)
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_resolution_candidates WHERE run_id = $1`, accepted.ID); got != finished.CandidateCount {
		t.Fatalf("candidates on the row = %d, the run says %d", got, finished.CandidateCount)
	}
	if finished.StartedAt == "" || finished.CompletedAt == "" {
		t.Fatalf("the finished run has no timestamps: %+v", finished)
	}
	// The run only ever moves forward. A scan over a small database finishes between
	// two polls, so the sequence is allowed to be several queued reads and then a
	// terminal one; what it is not allowed to do is go back.
	rank := map[string]int{
		entityresolution.RunQueued:    0,
		entityresolution.RunRunning:   1,
		entityresolution.RunSucceeded: 2,
		entityresolution.RunFailed:    2,
	}
	for index := 1; index < len(seen); index++ {
		if rank[seen[index]] < rank[seen[index-1]] {
			t.Fatalf("the run moved backwards: %v", seen)
		}
	}
	if seen[0] != entityresolution.RunQueued {
		t.Fatalf("the first thing a client read was %q, want %q", seen[0], entityresolution.RunQueued)
	}
}

// TestTheResearchAgentPostKeepsItsStatusCodeAndShape is the same claim for the
// investigation, and it is the stricter one: the body is the whole report, so a
// client that reads `report` the moment it gets a response would read an empty one -
// which is why the status is what a client has to look at.
func TestTheResearchAgentPostKeepsItsStatusCodeAndShape(t *testing.T) {
	fixture := testsupport.New(t)
	startAnalysisConsumer(t, fixture)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	requester := newIdentitySession(t, fixture, "صاحب السؤال", "researcher")
	person := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar) VALUES ($1, 'عبد الله بن عبد الرحمن', 'عبداللهبنعبدالرحمن')`, person)
	question := uuid.New()
	fixture.Exec(`INSERT INTO open_questions (id, title_ar, description_ar, status, created_by) VALUES ($1, 'أين هاجر عبد الله؟', 'أين كانت الهجرة؟', 'open', $2)`, question, requester.userID)
	// One claim about the person that a public, independent statement contradicts, so
	// the investigation has something it is allowed to cite. An investigation over an
	// empty corpus finishes successfully with nothing to show, which proves the queue
	// and not the report.
	place := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type) VALUES ($1, 'المدينة', 'المدينة', 'city')`, place)
	source := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, dependency_status, created_by) VALUES ($1, 'المصادر المنقولة', 'manuscript', 'public', 'independent', $2)`, source, requester.userID)
	statement := uuid.New()
	fixture.Exec(`INSERT INTO source_statements (id, source_id, statement_text_ar, review_status) VALUES ($1, $2, 'هاجر عبد الله إلى المدينة.', 'accepted')`, statement, source)
	claim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, place_id, status) VALUES ($1, 'person', $2, 'hijra_to', 'place', $3, $3, 'contested')`, claim, person, place)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation) VALUES ($1, $2, 'contradicts')`, claim, statement)

	recorder := requester.do(router, http.MethodPost, "/api/v1/research-agent/runs",
		`{"question":"أين هاجر عبد الله؟","question_id":"`+question.String()+`","entity_type":"person","entity_id":"`+person.String()+`"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("start an investigation = %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
	var accepted researchagent.Run
	decodeIdentityBody(t, recorder, &accepted)
	if accepted.Status != researchagent.RunQueued {
		t.Fatalf("accepted investigation = %q, want %q", accepted.Status, researchagent.RunQueued)
	}
	if accepted.ID == "" || accepted.JobID == "" {
		t.Fatalf("accepted investigation = %+v, want a run id and the job behind it", accepted)
	}
	if accepted.Query == "" || accepted.EntityID != person.String() || accepted.PlannerVersion != researchagent.PlannerVersion {
		t.Fatalf("accepted investigation lost the fields a client reads: %+v", accepted)
	}
	if accepted.StepCount != 0 || accepted.EvidenceCount != 0 || accepted.Report.AnswerAR != "" {
		t.Fatalf("an accepted investigation reported a package: %+v", accepted)
	}
	// The fields a client reads for the resolution are the same for a queued run:
	// unresolved, and none of the counts moved.
	if accepted.Resolution != researchagent.ResolutionUnresolved {
		t.Fatalf("an accepted investigation resolved to %q", accepted.Resolution)
	}

	// And the status endpoint a client polls is the one that always existed.
	stored := pollRun(t, func() (researchagent.Run, string, bool) {
		read := requester.do(router, http.MethodGet, "/api/v1/research-agent/runs/"+accepted.ID, "")
		if read.Code != http.StatusOK {
			t.Fatalf("read the investigation = %d, want 200: %s", read.Code, read.Body.String())
		}
		var current researchagent.Run
		decodeIdentityBody(t, read, &current)
		if researchagent.Terminal(current.Status) {
			if current.Status != researchagent.RunSucceeded {
				t.Fatalf("the investigation finished as %s (%s), want succeeded", current.Status, current.Error)
			}
			return current, current.Status, true
		}
		return current, current.Status, false
	}, 60*time.Second)
	if len(stored.Steps) != researchagent.MaximumSteps {
		t.Fatalf("the finished investigation has %d steps, want the plan's %d", len(stored.Steps), researchagent.MaximumSteps)
	}
	if stored.EvidenceCount == 0 || stored.Report.AnswerAR == "" {
		t.Fatalf("the finished investigation reported no package: %+v", stored)
	}
	// The latest-run lookup a returning client uses finds it, which is how a page
	// reload during a queued run still ends up showing the result.
	latest := requester.do(router, http.MethodGet,
		"/api/v1/research-agent/runs/latest?question_id="+question.String()+"&entity_type=person&entity_id="+person.String(), "")
	if latest.Code != http.StatusOK {
		t.Fatalf("latest investigation = %d, want 200: %s", latest.Code, latest.Body.String())
	}
	var found researchagent.Run
	decodeIdentityBody(t, latest, &found)
	if found.ID != accepted.ID || found.Status != researchagent.RunSucceeded {
		t.Fatalf("the latest investigation is %s/%s, want the finished %s", found.ID, found.Status, accepted.ID)
	}
}

// startAnalysisConsumer runs the real consumer loop in this process for the length
// of a test.
//
// It is here because a status endpoint that never reaches a terminal state proves
// nothing about what a client does when one does, and something has to be draining
// the queue for that to happen. The loop is the deployed one; what is not under test
// here is that it lives in its own process, which is what internal/analysisworker
// proves by starting the binary and waiting.
func startAnalysisConsumer(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	queue := jobs.NewService(fixture.Pool())
	queue.HeartbeatInterval = 50 * time.Millisecond
	handler, err := analysisworker.New(fixture.Pool(), queue, nil, nil)
	if err != nil {
		t.Fatalf("build the analysis handler: %v", err)
	}
	ctx, cancel := context.WithCancel(fixture.Ctx())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := jobworker.Run(ctx, jobworker.Config{
			Jobs:     queue,
			WorkerID: "p006-http-consumer",
			Handlers: []jobworker.Handler{handler},
			Logger:   log.New(io.Discard, "", 0),
		}); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("the consumer stopped: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Error("the consumer did not stop within 20s")
		}
	})
}

// pollRun drives a status endpoint the way a client does: read the run, look at
// its status, stop when the status is one no worker will move again. It hands back
// the last body it read, so the assertions are made against what a client would
// actually be holding rather than against the value the test already had.
//
// It fails the test if the run never settles, because "still queued when the test
// ended" is exactly the outcome this change is not allowed to have.
func pollRun[T any](t *testing.T, read func() (T, string, bool), limit time.Duration) T {
	t.Helper()
	deadline := time.Now().Add(limit)
	var last T
	status := "never read"
	for time.Now().Before(deadline) {
		body, current, terminal := read()
		last = body
		status = current
		if terminal {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the run did not reach a terminal state within %s (last status %q)", limit, status)
	return last
}
