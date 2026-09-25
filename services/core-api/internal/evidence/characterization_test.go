package evidence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestValidateSourceCharacterizationInput(t *testing.T) {
	input, err := validateSourceCharacterizationInput(SourceCharacterizationInput{
		SourceID: "  00000000-0000-0000-0000-000000000001  ",
		ClaimID:  " 00000000-0000-0000-0000-000000000002 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.SourceID != "00000000-0000-0000-0000-000000000001" || input.ClaimID != "00000000-0000-0000-0000-000000000002" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
	if _, err := validateSourceCharacterizationInput(SourceCharacterizationInput{SourceID: "not-a-uuid"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestSourceCharacterizationReportKeepsUncertaintyExplicit(t *testing.T) {
	source := SourceView{
		ID:                  "00000000-0000-0000-0000-000000000001",
		TitleAR:             "مخطوط",
		SourceType:          "manuscript",
		CitationAR:          "مجلد ١، صفحة ٢",
		PublicationDateFrom: "1900-01-01",
		Visibility:          "public",
	}
	analysis := characterizationAnalysis{
		Statements: []characterizationStatement{{ID: "statement-1", Text: "عبارة مقبولة"}},
	}
	report := buildSourceCharacterizationReport(source, analysis, map[string][]string{
		"accepted_evidence": {"evidence-1"},
	}, time.Now().UTC())
	states := make(map[string]string)
	for _, attribute := range report.Attributes {
		states[attribute.Key] = attribute.State
	}
	if states["source_role"] != "unknown" || states["temporal_relation"] != "unknown" || states["author_proximity"] != "unknown" {
		t.Fatalf("uncertain attributes were not explicit: %+v", states)
	}
	if states["citation_metadata"] != "observed" || states["accepted_evidence"] != "observed" {
		t.Fatalf("observed attributes were not marked: %+v", states)
	}
	if sourceCharacterizationFindingCount(report) != 0 {
		t.Fatalf("unexpected finding count: %d", sourceCharacterizationFindingCount(report))
	}
}

func TestSourceCharacterizationRunAndReview(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()

	ownerID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث المصدر')`, ownerID, characterizationTestEmail(ownerID)); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)

	sourceID := uuid.New()
	corroboratorID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, citation_ar, source_type, visibility, created_by) VALUES ($1, 'المصدر الأول', 'مجلد ١، صفحة ٢', 'book', 'public', $3), ($2, 'المصدر المؤكد', 'مجلد ٢، صفحة ٤', 'book', 'public', $3)`, sourceID, corroboratorID, ownerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, []uuid.UUID{sourceID, corroboratorID})

	passageID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, 'نص المصدر', 'نص المصدر')`, passageID, sourceID); err != nil {
		t.Fatal(err)
	}
	statementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, 'عبارة مقبولة من المصدر', 'accepted', $4)`, statementID, sourceID, passageID, ownerID); err != nil {
		t.Fatal(err)
	}
	corroboratorStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'عبارة مطابقة من مصدر آخر', 'accepted', $3)`, corroboratorStatementID, corroboratorID, ownerID); err != nil {
		t.Fatal(err)
	}
	claimID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'parent_of', 'person', $3, 'supported', $4)`, claimID, uuid.New(), uuid.New(), ownerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM claims WHERE id = $1`, claimID)
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($1, $4, 'supports', $3)`, claimID, statementID, ownerID, corroboratorStatementID); err != nil {
		t.Fatal(err)
	}

	service := NewService(pool)
	run, err := service.StartSourceCharacterization(ctx, ownerID.String(), SourceCharacterizationInput{SourceID: sourceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" || run.ReportStatus != "succeeded" || run.ReviewStatus != "needs_review" {
		t.Fatalf("unexpected run state: %+v", run)
	}
	if run.FindingCount != 0 || run.Report.Source.AcceptedStatementCount != 1 {
		t.Fatalf("unexpected report summary: %+v", run)
	}
	if run.Report.Corroboration.IndependentSourceCount != 1 || len(run.Report.Corroboration.Sources) != 1 {
		t.Fatalf("unexpected corroboration: %+v", run.Report.Corroboration)
	}
	if len(run.Evidence) == 0 || run.Evidence[0].AttributeKey == "" {
		t.Fatalf("expected evidence references: %+v", run.Evidence)
	}
	anonymous, err := service.GetLatestSourceCharacterization(ctx, "", SourceCharacterizationInput{SourceID: sourceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if anonymous.ID != run.ID {
		t.Fatalf("anonymous public read returned the wrong run: %+v", anonymous)
	}

	repeated, err := service.StartSourceCharacterization(ctx, ownerID.String(), SourceCharacterizationInput{SourceID: sourceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != run.ID {
		t.Fatalf("characterization was not idempotent: first=%s second=%s", run.ID, repeated.ID)
	}

	if _, err := pool.Exec(ctx, `UPDATE source_statements SET review_status = 'rejected' WHERE id = $1`, statementID); err != nil {
		t.Fatal(err)
	}
	changed, err := service.StartSourceCharacterization(ctx, ownerID.String(), SourceCharacterizationInput{SourceID: sourceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID == run.ID || changed.ReportStatus != "insufficient_evidence" {
		t.Fatalf("source evidence change was not reflected: %+v", changed)
	}

	confirmed, err := service.ReviewSourceCharacterization(ctx, ownerID.String(), run.ID, ReviewSourceCharacterizationInput{Decision: "confirm", NoteAR: "تمت المراجعة."})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ReviewStatus != "confirmed" || len(confirmed.Reviews) != 1 {
		t.Fatalf("unexpected confirmation: %+v", confirmed)
	}
	if _, err := service.ReviewSourceCharacterization(ctx, ownerID.String(), run.ID, ReviewSourceCharacterizationInput{Decision: "confirm"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected review conflict, got %v", err)
	}
	if _, err := service.ReviewSourceCharacterization(ctx, ownerID.String(), run.ID, ReviewSourceCharacterizationInput{Decision: "reopen"}); err != nil {
		t.Fatal(err)
	}
	dismissed, err := service.ReviewSourceCharacterization(ctx, ownerID.String(), run.ID, ReviewSourceCharacterizationInput{Decision: "dismiss"})
	if err != nil {
		t.Fatal(err)
	}
	if dismissed.ReviewStatus != "dismissed" {
		t.Fatalf("unexpected dismissal: %+v", dismissed)
	}
}

func TestSourceCharacterizationRedactsPrivateRelatedSources(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()

	ownerID := uuid.New()
	viewerID := uuid.New()
	for _, actor := range []struct {
		id    uuid.UUID
		email string
	}{{ownerID, characterizationTestEmail(ownerID)}, {viewerID, characterizationTestEmail(viewerID)}} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مستخدم المصدر')`, actor.id, actor.email); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []uuid.UUID{ownerID, viewerID})

	publicSourceID := uuid.New()
	privateSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر عام', 'book', 'public', $3), ($2, 'مصدر خاص', 'book', 'private', $3)`, publicSourceID, privateSourceID, ownerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, []uuid.UUID{publicSourceID, privateSourceID})
	claimID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'parent_of', 'person', $3, 'supported', $4)`, claimID, uuid.New(), uuid.New(), ownerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM claims WHERE id = $1`, claimID)
	publicStatementID := uuid.New()
	privateStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'عبارة عامة', 'accepted', $5), ($3, $4, 'عبارة خاصة', 'accepted', $5)`, publicStatementID, publicSourceID, privateStatementID, privateSourceID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $4), ($1, $3, 'supports', $4)`, claimID, publicStatementID, privateStatementID, ownerID); err != nil {
		t.Fatal(err)
	}

	service := NewService(pool)
	run, err := service.StartSourceCharacterization(ctx, ownerID.String(), SourceCharacterizationInput{SourceID: publicSourceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if run.Report.Corroboration.IndependentSourceCount != 1 {
		t.Fatalf("owner did not receive authorized corroboration: %+v", run.Report.Corroboration)
	}
	filtered, err := service.GetSourceCharacterizationRun(ctx, viewerID.String(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Report.Corroboration.IndependentSourceCount != 0 || len(filtered.Report.Corroboration.Sources) != 0 {
		t.Fatalf("private related source leaked: %+v", filtered.Report.Corroboration)
	}
	for _, evidence := range filtered.Evidence {
		if evidence.RelatedSourceID == privateSourceID.String() {
			t.Fatalf("private related source leaked in evidence: %+v", evidence)
		}
	}
}

func TestSourceCharacterizationReviewValidation(t *testing.T) {
	if _, err := validateSourceCharacterizationReview(ReviewSourceCharacterizationInput{Decision: " CONFIRM ", NoteAR: " ملاحظة "}); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSourceCharacterizationReview(ReviewSourceCharacterizationInput{Decision: "accepted"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func characterizationTestEmail(id uuid.UUID) string {
	return fmt.Sprintf("source-characterization-%s@example.test", id.String())
}
