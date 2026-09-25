package researchagent

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestResearchQuestionCandidatesGenerateAndConvertIdempotently(t *testing.T) {
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
	originQuestionID := uuid.New()
	personID := uuid.New()
	treeID := uuid.New()
	versionID := uuid.New()
	runID := uuid.New()
	gapID := uuid.New()
	secondaryGapID := uuid.New()
	recommendationID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث المرشحين'), ($3, $4, 'باحث مرشح آخر')`, actorID, candidateTestEmail(actorID, "actor"), viewerID, candidateTestEmail(viewerID, "viewer")); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []uuid.UUID{actorID, viewerID})
	defer pool.Exec(ctx, `DELETE FROM audit_log WHERE actor_id = ANY($1::uuid[])`, []uuid.UUID{actorID, viewerID})
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher'), ($2, 'researcher')`, actorID, viewerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, priority, created_by) VALUES ($1, 'السؤال الأصل', 'open', 'normal', $2)`, originQuestionID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM open_questions WHERE id = $1`, originQuestionID)
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة المرشحين', 'public', $2)`, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM trees WHERE id = $1`, treeID)
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_agent_runs WHERE id = $1`, runID)
	if _, err := pool.Exec(ctx, `INSERT INTO research_agent_runs (id, requested_by, question_id, query, normalized_query, entity_type, entity_id, tree_id, tree_version_id, status, resolution, execution_mode, planner_version, algorithm_version, qualification_policy_version, report, step_count, evidence_count, gap_count, recommendation_count, started_at, completed_at) VALUES ($1, $2, $3, 'سؤال اختبار', 'سؤال اختبار', 'person', $4, $5, $6, 'succeeded', 'unresolved', 'synchronous', $7, $8, $9, '{}'::jsonb, 11, 0, 1, 1, now(), now())`, runID, actorID, originQuestionID, personID, treeID, versionID, PlannerVersion, AlgorithmVersion, QualificationPolicy); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM research_agent_gaps WHERE id = ANY($1::uuid[])`, []uuid.UUID{gapID, secondaryGapID})
	defer pool.Exec(ctx, `DELETE FROM research_agent_recommendations WHERE id = $1`, recommendationID)
	if _, err := pool.Exec(ctx, `INSERT INTO research_agent_gaps (id, run_id, kind, description_ar, severity, metadata) VALUES ($1, $2, 'missing_source_evidence', 'لا توجد عبارات مصدرية مستقلة تدعم السؤال.', 'high', '{}'::jsonb), ($3, $2, 'missing_geography', 'لا توجد إشارات جغرافية مؤهلة.', 'low', '{}'::jsonb)`, gapID, runID, secondaryGapID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO research_agent_recommendations (id, run_id, action, rationale_ar, priority, metadata) VALUES ($1, $2, 'توسيع البحث المصدري', 'ابحث عن مصدر مستقل جديد.', 'high', '{}'::jsonb)`, recommendationID, runID); err != nil {
		t.Fatal(err)
	}
	service := NewService(pool)
	candidates, err := service.GenerateQuestionCandidates(ctx, actorID.String(), runID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("unexpected candidate count: %+v", candidates)
	}
	primaryCandidate := candidateForGap(candidates, gapID)
	secondaryCandidate := candidateForGap(candidates, secondaryGapID)
	if primaryCandidate.OriginRecommendationID != recommendationID.String() || primaryCandidate.Status != "proposed" {
		t.Fatalf("unexpected primary candidate: %+v", primaryCandidate)
	}
	if primaryCandidate.TitleAR == "" || primaryCandidate.DescriptionAR == "" || primaryCandidate.Priority != "high" {
		t.Fatalf("candidate template is incomplete: %+v", primaryCandidate)
	}
	repeated, err := service.GenerateQuestionCandidates(ctx, actorID.String(), runID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated) != 2 || repeated[0].ID != candidates[0].ID || repeated[1].ID != candidates[1].ID {
		t.Fatalf("generation was not idempotent: %+v", repeated)
	}
	listed, err := service.ListQuestionCandidates(ctx, actorID.String(), runID.String(), "proposed")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || len(candidateForGap(listed, gapID).Reviews) != 0 {
		t.Fatalf("unexpected candidate list: %+v", listed)
	}
	converted, err := service.ReviewQuestionCandidate(ctx, actorID.String(), primaryCandidate.ID, ReviewQuestionCandidateInput{Decision: "converted", TitleAR: "سؤال مصادر مستقل", Priority: "high", NoteAR: "راجع البوابات يدوياً."})
	if err != nil {
		t.Fatal(err)
	}
	if converted.Status != "converted" || converted.QuestionID == "" || len(converted.Reviews) != 1 || converted.Reviews[0].Decision != "converted" {
		t.Fatalf("candidate was not converted: %+v", converted)
	}
	defer pool.Exec(ctx, `DELETE FROM open_questions WHERE id = $1`, converted.QuestionID)
	var questionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM open_questions WHERE id = $1 AND title_ar = $2`, converted.QuestionID, "سؤال مصادر مستقل").Scan(&questionCount); err != nil {
		t.Fatal(err)
	}
	if questionCount != 1 {
		t.Fatalf("converted question count = %d, want 1", questionCount)
	}
	var linkedEntityCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM question_entities WHERE question_id = $1 AND entity_type = 'person' AND entity_id = $2`, converted.QuestionID, personID).Scan(&linkedEntityCount); err != nil {
		t.Fatal(err)
	}
	if linkedEntityCount != 1 {
		t.Fatalf("converted question entity link count = %d, want 1", linkedEntityCount)
	}
	if _, err := service.ReviewQuestionCandidate(ctx, actorID.String(), primaryCandidate.ID, ReviewQuestionCandidateInput{Decision: "dismissed"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected repeated review conflict, got %v", err)
	}
	dismissed, err := service.ReviewQuestionCandidate(ctx, actorID.String(), secondaryCandidate.ID, ReviewQuestionCandidateInput{Decision: "dismissed", NoteAR: "ليست أولوية حالياً."})
	if err != nil {
		t.Fatal(err)
	}
	if dismissed.Status != "dismissed" || len(dismissed.Reviews) != 1 {
		t.Fatalf("candidate was not dismissed: %+v", dismissed)
	}
	if _, err := pool.Exec(ctx, `UPDATE trees SET visibility = 'private' WHERE id = $1`, treeID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListQuestionCandidates(ctx, viewerID.String(), runID.String(), ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected private candidate access to be forbidden, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE trees SET visibility = 'public' WHERE id = $1`, treeID); err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND entity_id IN ($2, $3, $4, $5)`, actorID, runID, primaryCandidate.ID, secondaryCandidate.ID, converted.QuestionID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 4 {
		t.Fatalf("candidate audit count = %d, want 4", auditCount)
	}
}

func TestResearchQuestionCandidatesRequireResearchRole(t *testing.T) {
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
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مستخدم غير مخول')`, actorID, candidateTestEmail(actorID, "unauthorized")); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actorID)
	if _, err := NewService(pool).GenerateQuestionCandidates(context.Background(), actorID.String(), uuid.NewString()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected unauthorized generation to be forbidden, got %v", err)
	}
}

func TestQuestionCandidateTemplatesAndReviewValidation(t *testing.T) {
	cases := []struct {
		kind     string
		title    string
		priority string
	}{
		{kind: "missing_source_evidence", title: "بحث عن مصادر مستقلة تدعم السؤال", priority: "high"},
		{kind: "missing_counter_evidence", title: "فحص أدلة مضادة وروايات مقابلة", priority: "normal"},
		{kind: "missing_chronology", title: "مراجعة التسلسل الزمني غير المحسوم", priority: "normal"},
	}
	for _, testCase := range cases {
		title, description, priority := candidateTemplate(questionCandidateGap{Kind: testCase.kind, DescriptionAR: "فجوة اختبار", Severity: testCase.priority})
		if title != testCase.title || priority != testCase.priority || description == "" {
			t.Fatalf("unexpected template for %s: title=%q priority=%q description=%q", testCase.kind, title, priority, description)
		}
	}
	if _, err := validateQuestionCandidateReview(ReviewQuestionCandidateInput{Decision: "converted", DescriptionAR: "x"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected short converted description to fail validation, got %v", err)
	}
	if _, err := validateQuestionCandidateReview(ReviewQuestionCandidateInput{Decision: "converted", TitleAR: "سؤال صالح", Priority: "normal"}); err != nil {
		t.Fatalf("expected valid review input, got %v", err)
	}
}

func candidateForGap(candidates []QuestionCandidate, gapID uuid.UUID) QuestionCandidate {
	for _, candidate := range candidates {
		if candidate.OriginGapID == gapID.String() {
			return candidate
		}
	}
	return QuestionCandidate{}
}

func candidateTestEmail(id uuid.UUID, suffix string) string {
	return "research-question-candidate-" + suffix + "-" + id.String() + "@example.test"
}
