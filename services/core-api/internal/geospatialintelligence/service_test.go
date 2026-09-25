package geospatialintelligence

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestGeospatialIntelligenceRunSeparatesSourceAndInferredLayers(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث جغرافي')`, actorID, geospatialTestEmail(actorID)); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, actorID)
	defer pool.Exec(ctx, `DELETE FROM audit_log WHERE actor_id = $1`, actorID)
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role = 'researcher'`, actorID)
	viewerID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث جغرافي آخر')`, viewerID, geospatialTestEmail(viewerID)+"-viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, viewerID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role = 'researcher'`, viewerID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, viewerID)
	treeID, versionID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة جغرافية', 'public', $2)`, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	questionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, priority, created_by) VALUES ($1, 'سؤال جغرافي', 'open', 'normal', $2)`, questionID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM open_questions WHERE id = $1`, questionID)
	personID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, birth_date_from, birth_date_to, identity_status, created_by) VALUES ($1, 'شخص جغرافي', 'شخص جغرافي', '1150-01-01', '1250-12-31', 'reviewed', $2)`, personID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM people WHERE id = $1`, personID)
	defer pool.Exec(ctx, `DELETE FROM trees WHERE id = $1`, treeID)
	nodeID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخص جغرافي', 1)`, nodeID, versionID, personID); err != nil {
		t.Fatal(err)
	}
	placeIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	placeNames := []string{"موضع الاختبار الأول", "موضع الاختبار الثاني", "موضع الاختبار الثالث"}
	coordinates := [][2]float64{{46.6753, 24.7136}, {49.5658, 25.3647}, {38.1333, 26.6084}}
	for index, placeID := range placeIDs {
		if _, err := pool.Exec(ctx, `INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, created_by) VALUES ($1, $2, $3, 'city', ST_SetSRID(ST_MakePoint($4, $5), 4326), $6)`, placeID, placeNames[index], placeNames[index], coordinates[index][0], coordinates[index][1], actorID); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM places WHERE id = ANY($1::uuid[])`, placeIDs)
	sourceIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for index, sourceID := range sourceIDs {
		if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, $2, 'book', 'independent', 'public', $3)`, sourceID, "مصدر جغرافي "+string(rune('A'+index)), actorID); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM sources WHERE id = ANY($1::uuid[])`, sourceIDs)
	statementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'ذكر النص موضع الاختبار الأول وموضع الاختبار الثاني ثم موضع الاختبار الثالث، وكلها مورد جغرافي.', 'accepted', $3)`, statementID, sourceIDs[0], actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM source_statements WHERE id = $1`, statementID)
	associationIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	associationData := [][2]int{{0, 0}, {1, 1}, {2, 2}}
	for index, pair := range associationData {
		if _, err := pool.Exec(ctx, `INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, time_from, time_to, status, certainty, source_id, created_by) VALUES ($1, 'person', $2, $3, 'documented_in', '1200-01-01', '1205-12-31', 'interpreted', 'approximate', $4, $5)`, associationIDs[index], personID, placeIDs[pair[0]], sourceIDs[index%2], actorID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE geographic_associations SET time_from = '1206-01-01', time_to = '1209-12-31' WHERE id = $1`, associationIDs[1]); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM geographic_associations WHERE id = ANY($1::uuid[])`, associationIDs)
	evidenceIDs := make([]uuid.UUID, 0, len(associationIDs))
	for _, associationID := range associationIDs {
		evidenceID := uuid.New()
		evidenceIDs = append(evidenceIDs, evidenceID)
		if _, err := pool.Exec(ctx, `INSERT INTO spatial_evidence (id, geographic_association_id, review_status, created_by) VALUES ($1, $2, 'accepted', $3)`, evidenceID, associationID, actorID); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM spatial_evidence WHERE id = ANY($1::uuid[])`, evidenceIDs)
	migrationID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, created_by) VALUES ($1, 'person', $2, $3, $4, '1206-01-01', '1209-12-31', 'platform_inferred', 'uncertain', $5, $6)`, migrationID, personID, placeIDs[0], placeIDs[2], sourceIDs[0], actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM migration_events WHERE id = $1`, migrationID)
	migrationEvidenceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO spatial_evidence (id, migration_event_id, review_status, created_by) VALUES ($1, $2, 'needs_review', $3)`, migrationEvidenceID, migrationID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM spatial_evidence WHERE id = $1`, migrationEvidenceID)
	defer pool.Exec(ctx, `DELETE FROM geospatial_intelligence_runs WHERE requested_by = $1`, actorID)
	defer pool.Exec(ctx, `DELETE FROM platform_findings WHERE finding_type = $1 AND created_by = $2`, FindingTypeConflict, actorID)
	service := NewService(pool)
	run, err := service.StartRun(ctx, actorID.String(), RunInput{EntityType: "person", EntityID: personID.String(), QuestionID: questionID.String(), TreeID: treeID.String(), TreeVersionID: versionID.String(), RadiusKM: 500, MaximumRecords: 500})
	if err != nil {
		t.Fatal(err)
	}
	if run.ReportStatus != ReportStatusSucceeded || run.FindingCount != 1 || run.Report.PlaceResolution.ResolvedCount < 3 || len(run.Report.Clusters) != 1 || len(run.Report.MigrationHypotheses) < 1 {
		t.Fatalf("unexpected geospatial run: %+v", run)
	}
	if run.Report.MigrationHypotheses[0].Status != "platform_hypothesis" || run.Report.MigrationHypotheses[0].Layer != "platform_inferred" {
		t.Fatalf("inferred migration was not labeled: %+v", run.Report.MigrationHypotheses[0])
	}
	if _, err := pool.Exec(ctx, `UPDATE trees SET visibility = 'private' WHERE id = $1`, treeID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRun(ctx, viewerID.String(), run.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected private geospatial run to be forbidden, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE trees SET visibility = 'public' WHERE id = $1`, treeID); err != nil {
		t.Fatal(err)
	}
	findings, err := service.ListFindings(ctx, actorID.String(), run.ID, "needs_review")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Layer != "platform_inferred" {
		t.Fatalf("unexpected geospatial findings: %+v", findings)
	}
	reviewed, err := service.ReviewFinding(ctx, actorID.String(), findings[0].ID, ReviewInput{Decision: "investigate", CreateQuestion: true, QuestionTitleAR: "مراجعة تعارض جغرافي"})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != "investigating" {
		t.Fatalf("unexpected review status: %+v", reviewed)
	}
	var reviewQuestionID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT question_id FROM platform_finding_reviews WHERE finding_id = $1 ORDER BY created_at DESC LIMIT 1`, findings[0].ID).Scan(&reviewQuestionID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM open_questions WHERE id = $1`, reviewQuestionID)
	sourceRun, err := service.StartRun(ctx, actorID.String(), RunInput{EntityType: "source", EntityID: sourceIDs[0].String(), QuestionID: questionID.String(), RadiusKM: 500, MaximumRecords: 500})
	if err != nil {
		t.Fatal(err)
	}
	if sourceRun.ReportStatus != ReportStatusSucceeded || len(sourceRun.Report.SourceGeography) != 1 || sourceRun.Report.SourceGeography[0].ResolvedMentions < 3 {
		t.Fatalf("unexpected source geography report: %+v", sourceRun)
	}
}

func geospatialTestEmail(id uuid.UUID) string {
	return "geospatial-intelligence-" + id.String() + "@example.test"
}
