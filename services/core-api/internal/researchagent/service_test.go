package researchagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResearchAgentRunProducesTraceableBoundedPackage(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	ctx := context.Background()
	actorID := uuid.New()
	viewerID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث الوكيل'), ($3, $4, 'باحث آخر')`, actorID, researchAgentTestEmail(actorID, "actor"), viewerID, researchAgentTestEmail(viewerID, "viewer")); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []uuid.UUID{actorID, viewerID})
	defer pool.Exec(ctx, `DELETE FROM audit_log WHERE actor_id = ANY($1::uuid[])`, []uuid.UUID{actorID, viewerID})
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher'), ($2, 'researcher')`, actorID, viewerID); err != nil {
		t.Fatal(err)
	}
	treeID, versionID := uuid.New(), uuid.New()
	personID, relatedPersonID := uuid.New(), uuid.New()
	questionID := uuid.New()
	sourceID, privateSourceID, dependencySourceID := uuid.New(), uuid.New(), uuid.New()
	placeID := uuid.New()
	defer pool.Exec(ctx, `DELETE FROM places WHERE id = $1`, placeID)
	statementID, privateStatementID := uuid.New(), uuid.New()
	claimID, counterClaimID := uuid.New(), uuid.New()
	relationshipID := uuid.New()
	associationID := uuid.New()
	defer pool.Exec(ctx, `DELETE FROM research_agent_runs WHERE requested_by = ANY($1::uuid[])`, []uuid.UUID{actorID, viewerID})
	defer pool.Exec(ctx, `DELETE FROM geographic_associations WHERE id = $1`, associationID)
	defer pool.Exec(ctx, `DELETE FROM tree_relationships WHERE id = $1`, relationshipID)
	defer pool.Exec(ctx, `DELETE FROM claims WHERE id = ANY($1::uuid[])`, []uuid.UUID{claimID, counterClaimID})
	defer pool.Exec(ctx, `DELETE FROM source_dependencies WHERE source_id = ANY($1::uuid[]) OR depends_on_source_id = ANY($1::uuid[])`, []uuid.UUID{sourceID, dependencySourceID})
	defer pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, []uuid.UUID{sourceID, privateSourceID, dependencySourceID})
	defer pool.Exec(ctx, `DELETE FROM people WHERE id = ANY($1::uuid[])`, []uuid.UUID{personID, relatedPersonID})
	defer pool.Exec(ctx, `DELETE FROM open_questions WHERE id = $1`, questionID)
	defer pool.Exec(ctx, `DELETE FROM trees WHERE id = $1`, treeID)
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة وكيل البحث', 'public', $2)`, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, 'عبد الله', 'عبد الله', 'reviewed', $2), ($3, 'الشيخ محمد', 'الشيخ محمد', 'reviewed', $2)`, personID, actorID, relatedPersonID); err != nil {
		t.Fatal(err)
	}
	subjectNodeID, objectNodeID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'عبد الله', 1), ($4, $2, $5, 'الشيخ محمد', 2)`, subjectNodeID, versionID, personID, objectNodeID, relatedPersonID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, priority, created_by) VALUES ($1, 'ما موضع هجرة عبد الله؟', 'open', 'high', $2)`, questionID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, 'مصدر مستقل', 'book', 'independent', 'public', $3), ($2, 'مصدر خاص', 'book', 'independent', 'private', $3), ($4, 'مصدر تابع', 'book', 'independent', 'public', $3)`, sourceID, privateSourceID, actorID, dependencySourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'ذكرت الهجرة موضع المدينة فيمصدر مستقل.', 'accepted', $3), ($4, $5, 'ذكرت الهجرة موضع خاص.', 'accepted', $3)`, statementID, sourceID, actorID, privateStatementID, privateSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, created_by) VALUES ($1, 'مدينة الاختبار', 'مدينة الاختبار', 'city', $2)`, placeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, source_id, created_by) VALUES ($1, 'person', $2, $3, 'lived_in', 'documented', $4, $5)`, associationID, personID, placeID, sourceID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by) VALUES ($1, 'person', $2, 'lived_in', 'place', $3, 'supported', 'ادعاء داعم', $4), ($5, 'person', $2, 'lived_in', 'place', $3, 'disputed', 'ادعاء متعارض', $4)`, claimID, personID, placeID, actorID, counterClaimID); err != nil {
		t.Fatal(err)
	}
	counterStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'العبارة المضادة.', 'accepted', $3)`, counterStatementID, sourceID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM source_statements WHERE id = $1`, counterStatementID)
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $4), ($3, $5, 'contradicts', $4)`, claimID, statementID, counterClaimID, actorID, counterStatementID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_dependencies (id, source_id, depends_on_source_id, dependency_type, evidence_ar, status) VALUES ($1, $2, $3, 'derived_from', 'تسلسل منشور', 'confirmed')`, uuid.New(), dependencySourceID, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by) VALUES ($1, $2, $3, $4, 'parent_of', 'interpreted', $5, $6)`, relationshipID, versionID, subjectNodeID, objectNodeID, sourceID, actorID); err != nil {
		t.Fatal(err)
	}
	queue := jobs.NewService(pool)
	service := NewService(pool).WithQueue(queue)
	accepted, err := service.StartRun(ctx, actorID.String(), RunInput{Question: "ما موضع هجرة عبد الله؟", QuestionID: questionID.String(), EntityType: "person", EntityID: personID.String(), TreeID: treeID.String(), TreeVersionID: versionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	// Accepting an investigation must not pretend it is finished. The run is
	// queued, the stages have not run, and there is nothing to read yet.
	if accepted.Status != RunQueued || accepted.Stage != StageQueued || accepted.JobID == "" {
		t.Fatalf("accepted run = %+v, want queued with a job behind it", accepted)
	}
	if len(accepted.Steps) != 0 || accepted.EvidenceCount != 0 || accepted.Report.AnswerAR != "" {
		t.Fatalf("a queued run reported work: %+v", accepted)
	}
	if got := countRunRows(t, pool, `SELECT count(*) FROM research_agent_steps WHERE run_id = $1`, accepted.ID); got != 0 {
		t.Fatalf("steps on an accepted run = %d, want 0", got)
	}

	// The queue is what turns an accepted run into an answer, so the test drives
	// it the way a worker would: claim, process, complete.
	run := driveAgentRun(t, pool, queue, service, accepted.ID)
	if !hasLayer(run.Evidence, "source_statement") || !hasLayer(run.Evidence, "tree_interpretation") || !hasLayer(run.Evidence, "geographic_signal") || !hasLayer(run.Evidence, "source_dependency") || !hasCounterEvidence(run.Evidence) {
		t.Fatalf("missing bounded evidence layers: %+v", run.Evidence)
	}
	if hasSourceID(run.Evidence, privateSourceID.String()) {
		t.Fatalf("private source leaked into agent evidence: %+v", run.Evidence)
	}
	var relationshipStatus, claimStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tree_relationships WHERE id = $1`, relationshipID).Scan(&relationshipStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM claims WHERE id = $1`, claimID).Scan(&claimStatus); err != nil {
		t.Fatal(err)
	}
	if relationshipStatus != "interpreted" || claimStatus != "supported" {
		t.Fatalf("agent modified source interpretation: relationship=%s claim=%s", relationshipStatus, claimStatus)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND entity_id = $2 AND action = 'research_agent_run_completed'`, actorID, run.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("agent audit count = %d, want 1", auditCount)
	}
	stored, err := service.GetRun(ctx, actorID.String(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != run.ID || len(stored.Steps) != 11 || len(stored.Evidence) == 0 || len(stored.Gaps) == 0 {
		t.Fatalf("stored agent run is incomplete: %+v", stored)
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privateSourceID.String()) {
		t.Fatalf("private source id leaked from stored run: %s", encoded)
	}
	acceptedSource, err := service.StartRun(ctx, actorID.String(), RunInput{Question: "ما موضع الهجرة؟", QuestionID: questionID.String(), EntityType: "source", EntityID: sourceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	sourceRun := driveAgentRun(t, pool, queue, service, acceptedSource.ID)
	if !hasLayer(sourceRun.Evidence, "source_statement") || !hasLayer(sourceRun.Evidence, "geographic_signal") {
		t.Fatalf("source agent scope lost qualified evidence: %+v", sourceRun.Evidence)
	}
	if _, err := pool.Exec(ctx, `UPDATE trees SET visibility = 'private' WHERE id = $1`, treeID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRun(ctx, viewerID.String(), run.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected private run to be forbidden, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE trees SET visibility = 'public' WHERE id = $1`, treeID); err != nil {
		t.Fatal(err)
	}
	latest, err := service.GetLatestRun(ctx, actorID.String(), questionID.String(), "person", personID.String())
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != run.ID {
		t.Fatalf("latest agent run = %s, want %s", latest.ID, run.ID)
	}
}

func TestResearchAgentRejectsInvalidAndUnauthorizedRuns(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	actorID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مستخدم غير مخول')`, actorID, researchAgentTestEmail(actorID, "unauthorized")); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actorID)
	service := NewService(pool).WithQueue(jobs.NewService(pool))
	entityID := uuid.NewString()
	if _, err := service.StartRun(context.Background(), actorID.String(), RunInput{Question: "سؤال", EntityID: entityID, EntityType: "person"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected unauthorized run to be forbidden, got %v", err)
	}
	actorUUID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث صالح')`, actorUUID, researchAgentTestEmail(actorUUID, "valid")); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actorUUID)
	if _, err := pool.Exec(context.Background(), `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, actorUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartRun(context.Background(), actorUUID.String(), RunInput{Question: "سؤال", EntityID: "not-a-uuid", EntityType: "person"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected invalid uuid to fail validation, got %v", err)
	}
}

func hasLayer(evidence []EvidenceRef, layer string) bool {
	for _, item := range evidence {
		if item.Layer == layer {
			return true
		}
	}
	return false
}

func hasCounterEvidence(evidence []EvidenceRef) bool {
	for _, item := range evidence {
		if item.Stance == "counter_evidence" {
			return true
		}
	}
	return false
}

func hasSourceID(evidence []EvidenceRef, sourceID string) bool {
	for _, item := range evidence {
		if item.SourceID == sourceID {
			return true
		}
	}
	return false
}

func researchAgentTestEmail(id uuid.UUID, suffix string) string {
	return "research-agent-" + suffix + "-" + id.String() + "@example.test"
}

// driveAgentRun does what the worker process does for one accepted
// investigation, and returns the finished run.
//
// It is here so the existing assertions about the eleven stages, the evidence
// layers, the gaps and the audit keep testing what they were written to test. The
// claim/renew/complete cycle is the worker's, not this helper's - which is why
// the claim carries a real lease and the completion presents its token, and why
// the returned run is read back from the database rather than assembled in memory:
// a run that reported results it did not persist would pass an in-memory
// assertion and fail a reader.
func driveAgentRun(t *testing.T, pool *pgxpool.Pool, queue *jobs.Service, service *Service, runID string) Run {
	t.Helper()
	ctx := context.Background()
	job, err := queue.Claim(ctx, jobs.ClaimInput{WorkerID: "p006-agent-worker", Type: JobType})
	if err != nil {
		t.Fatalf("claim the investigation job: %v", err)
	}
	if job.LeaseToken == "" {
		t.Fatal("the claim came with no lease, so nothing downstream can be fenced")
	}
	lease := job.Lease("p006-agent-worker")
	if err := service.ProcessRun(ctx, lease, runID); err != nil {
		t.Fatalf("process the investigation: %v", err)
	}
	if _, err := queue.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: "p006-agent-worker", LeaseToken: lease.Token}); err != nil {
		t.Fatalf("complete the investigation job: %v", err)
	}
	run, err := service.GetRun(ctx, jobOwner(t, pool, runID), runID)
	if err != nil {
		t.Fatalf("read the finished run: %v", err)
	}
	if run.Status != RunSucceeded {
		t.Fatalf("run status = %s (%s), want succeeded", run.Status, run.Error)
	}
	if run.ExecutionMode != ExecutionModeAsynchronous {
		t.Fatalf("execution mode = %s, want %s", run.ExecutionMode, ExecutionModeAsynchronous)
	}
	if run.Resolution != ResolutionUnresolved || run.StepCount != 11 || run.EvidenceCount == 0 || run.GapCount == 0 {
		t.Fatalf("unexpected agent run: %+v", run)
	}
	if len(run.Steps) != 11 || run.Steps[0].Stage != StageDecompose || run.Steps[10].Stage != StageRecommendation {
		t.Fatalf("unexpected agent steps: %+v", run.Steps)
	}
	return run
}

func jobOwner(t *testing.T, pool *pgxpool.Pool, runID string) string {
	t.Helper()
	var owner string
	if err := pool.QueryRow(context.Background(), `SELECT requested_by::text FROM research_agent_runs WHERE id = $1`, runID).Scan(&owner); err != nil {
		t.Fatalf("read the run requester: %v", err)
	}
	return owner
}

func countRunRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	return count
}
