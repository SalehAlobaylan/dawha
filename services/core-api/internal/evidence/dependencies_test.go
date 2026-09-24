package evidence

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestValidateSourceDependencyInput(t *testing.T) {
	input, err := validateSourceDependencyInput(CreateSourceDependencyInput{DependsOnSourceID: " 10000000-0000-0000-0000-000000000001 ", DependencyType: "LIKELY_PARAPHRASE", EvidenceAR: "  دليل  "})
	if err != nil {
		t.Fatal(err)
	}
	if input.DependencyType != "likely_paraphrase" || input.EvidenceAR != "دليل" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
	if _, err := validateSourceDependencyInput(CreateSourceDependencyInput{DependsOnSourceID: "10000000-0000-0000-0000-000000000001", DependencyType: "invalid"}); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestValidateSourceDependencyReviewInput(t *testing.T) {
	if _, err := validateSourceDependencyReviewInput(ReviewSourceDependencyInput{Decision: "confirmed", NoteAR: "مراجعة"}); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSourceDependencyReviewInput(ReviewSourceDependencyInput{Decision: "accepted"}); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestSourceDependencyDetectionAndReview(t *testing.T) {
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
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث الاعتماد')`, actorID, dependencyTestEmail(actorID)); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actorID)
	sourceID := uuid.New()
	targetID := uuid.New()
	for _, source := range []struct {
		id    uuid.UUID
		title string
	}{{sourceID, "المصدر الأول"}, {targetID, "المصدر الثاني"}} {
		if _, err := pool.Exec(context.Background(), `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, $2, 'book', 'public', $3)`, source.id, source.title, actorID); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(context.Background(), `DELETE FROM sources WHERE id IN ($1, $2)`, sourceID, targetID)
	text := strings.Repeat("هذه جملة عربية طويلة تتكرر في مصدرين لاختبار تحليل الاعتماد. ", 3)
	for _, source := range []uuid.UUID{sourceID, targetID} {
		if _, err := pool.Exec(context.Background(), `INSERT INTO source_passages (source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, 1, $2, $2)`, source, text); err != nil {
			t.Fatal(err)
		}
	}
	claimIDs := make([]uuid.UUID, 0, 2)
	for index := 0; index < 2; index++ {
		claimID := uuid.New()
		claimIDs = append(claimIDs, claimID)
		if _, err := pool.Exec(context.Background(), `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'parent_of', 'person', $3, 'supported', $4)`, claimID, uuid.New(), uuid.New(), actorID); err != nil {
			t.Fatal(err)
		}
		statementIDs := make([]uuid.UUID, 0, 2)
		for _, source := range []uuid.UUID{sourceID, targetID} {
			statementID := uuid.New()
			statementIDs = append(statementIDs, statementID)
			if _, err := pool.Exec(context.Background(), `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'عبارة مشتركة', 'accepted', $3)`, statementID, source, actorID); err != nil {
				t.Fatal(err)
			}
		}
		for _, statementID := range statementIDs {
			if _, err := pool.Exec(context.Background(), `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3)`, claimID, statementID, actorID); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer pool.Exec(context.Background(), `DELETE FROM claims WHERE id = ANY($1::uuid[])`, claimIDs)
	service := &Service{Pool: pool}
	graph, err := service.DetectSourceDependencies(context.Background(), sourceID.String(), actorID.String())
	if err != nil {
		t.Fatal(err)
	}
	if graph.DetectedCount != 1 || len(graph.Items) != 1 {
		t.Fatalf("unexpected detection result: %+v", graph)
	}
	dependency := graph.Items[0]
	if dependency.DependsOnSourceID != targetID.String() || dependency.DependencyType != "likely_paraphrase" || dependency.Status != "needs_review" || dependency.AlgorithmVersion != SourceDependencyAlgorithmVersion {
		t.Fatalf("unexpected dependency: %+v", dependency)
	}
	signals, _ := dependency.SignalData["signals"].([]any)
	hasSharedClaimSignal := false
	for _, value := range signals {
		if signal, ok := value.(map[string]any); ok && signal["type"] == "shared_claim_sequence" {
			hasSharedClaimSignal = true
		}
	}
	if !hasSharedClaimSignal {
		t.Fatalf("shared claim signal was omitted: %+v", dependency.SignalData)
	}
	second, err := service.DetectSourceDependencies(context.Background(), sourceID.String(), actorID.String())
	if err != nil {
		t.Fatal(err)
	}
	if second.DetectedCount != 0 {
		t.Fatalf("detection was not idempotent: %+v", second)
	}
	reviewed, err := service.ReviewSourceDependency(context.Background(), dependency.ID, actorID.String(), ReviewSourceDependencyInput{Decision: "confirmed", NoteAR: "تأكيد العلاقة"})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Summary.Confirmed != 1 || reviewed.Items[0].Status != "confirmed" {
		t.Fatalf("unexpected review result: %+v", reviewed)
	}
	detail, err := service.GetSourceForActor(context.Background(), sourceID.String(), actorID.String())
	if err != nil {
		t.Fatal(err)
	}
	if detail.Source.DependencyStatus != "derived" || len(detail.Dependencies) != 1 || detail.Dependencies[0].Status != "confirmed" {
		t.Fatalf("source did not expose reviewed dependency: %+v", detail)
	}
	var storedStatus string
	if err := pool.QueryRow(context.Background(), `SELECT dependency_status FROM sources WHERE id = $1`, sourceID).Scan(&storedStatus); err != nil {
		t.Fatal(err)
	}
	if storedStatus != "derived" {
		t.Fatalf("stored dependency status = %q, want derived", storedStatus)
	}
	var reviewCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM source_dependency_reviews WHERE dependency_id = $1`, dependency.ID).Scan(&reviewCount); err != nil {
		t.Fatal(err)
	}
	if reviewCount != 1 {
		t.Fatalf("review count = %d, want 1", reviewCount)
	}
}

func TestSourceDependencyManualCreateAndRejection(t *testing.T) {
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
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث الاعتماد')`, actorID, dependencyTestEmail(actorID)); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actorID)
	sourceID := uuid.New()
	targetID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'المصدر الأول', 'book', 'public', $3), ($2, 'المصدر الهدف', 'book', 'public', $3)`, sourceID, targetID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM sources WHERE id IN ($1, $2)`, sourceID, targetID)
	service := &Service{Pool: pool}
	graph, err := service.CreateSourceDependency(context.Background(), sourceID.String(), actorID.String(), CreateSourceDependencyInput{DependsOnSourceID: targetID.String(), DependencyType: "cites", EvidenceAR: "اقتباس مباشر"})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Items) != 1 || graph.Items[0].Status != "needs_review" {
		t.Fatalf("unexpected manual dependency: %+v", graph)
	}
	if _, err := service.CreateSourceDependency(context.Background(), sourceID.String(), actorID.String(), CreateSourceDependencyInput{DependsOnSourceID: targetID.String(), DependencyType: "cites"}); err != ErrConflict {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}
	reviewed, err := service.ReviewSourceDependency(context.Background(), graph.Items[0].ID, actorID.String(), ReviewSourceDependencyInput{Decision: "rejected"})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Summary.Rejected != 1 || reviewed.Items[0].Status != "rejected" {
		t.Fatalf("unexpected rejected dependency: %+v", reviewed)
	}
	var storedStatus string
	if err := pool.QueryRow(context.Background(), `SELECT dependency_status FROM sources WHERE id = $1`, sourceID).Scan(&storedStatus); err != nil {
		t.Fatal(err)
	}
	if storedStatus != "unknown" {
		t.Fatalf("stored status after rejection = %q, want unknown", storedStatus)
	}
}

func TestSourceDependencyVisibilityHidesPrivateTarget(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	ownerID := uuid.New()
	targetOwnerID := uuid.New()
	for _, actor := range []struct {
		id    uuid.UUID
		email string
	}{{ownerID, dependencyTestEmail(ownerID)}, {targetOwnerID, dependencyTestEmail(targetOwnerID)}} {
		if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مستخدم')`, actor.id, actor.email); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, targetOwnerID)
	sourceID := uuid.New()
	targetID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر عام', 'book', 'public', $2), ($3, 'مصدر خاص', 'book', 'private', $4)`, sourceID, ownerID, targetID, targetOwnerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM sources WHERE id IN ($1, $2)`, sourceID, targetID)
	if _, err := pool.Exec(context.Background(), `INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, status) VALUES ($1, $2, 'cites', 'needs_review')`, sourceID, targetID); err != nil {
		t.Fatal(err)
	}
	service := &Service{Pool: pool}
	graph, err := service.ListSourceDependencies(context.Background(), sourceID.String(), ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Items) != 0 {
		t.Fatalf("private dependency target leaked: %+v", graph)
	}
	graph, err = service.ListSourceDependencies(context.Background(), sourceID.String(), targetOwnerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Items) != 1 || graph.Items[0].DependsOnSourceID != targetID.String() {
		t.Fatalf("authorized target owner could not view dependency: %+v", graph)
	}
}

func dependencyTestEmail(id uuid.UUID) string {
	return fmt.Sprintf("source-dependency-%s@example.test", id.String())
}
