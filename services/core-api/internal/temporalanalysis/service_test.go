package temporalanalysis

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/contradiction"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestTemporalAnalysisRunUsesQualifiedReferencePopulation(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث الإحصاء')`, actorID, temporalTestEmail(actorID)); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, actorID)
	defer pool.Exec(ctx, `DELETE FROM audit_log WHERE actor_id = $1`, actorID)
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role = 'researcher'`, actorID)
	sourceID := uuid.New()
	treeID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, 'مصدر مؤهل', 'book', 'independent', 'public', $2)`, sourceID, actorID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM sources WHERE id = $1`, sourceID)
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة qualified-v1', 'public', $2)`, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, actorID); err != nil {
		t.Fatal(err)
	}
	people := []struct {
		id        uuid.UUID
		name      string
		birthFrom string
		birthTo   string
	}{
		{uuid.New(), "الأصل الأول", "1100-01-01", "1110-12-31"},
		{uuid.New(), "الفرع الأول", "1125-01-01", "1135-12-31"},
		{uuid.New(), "الأصل الثاني", "1110-01-01", "1120-12-31"},
		{uuid.New(), "الفرع الثاني", "1135-01-01", "1145-12-31"},
		{uuid.New(), "الأصل الثالث", "1120-01-01", "1130-12-31"},
		{uuid.New(), "الفرع الثالث", "1145-01-01", "1155-12-31"},
		{uuid.New(), "الأصل الشاذ", "1000-01-01", "1010-12-31"},
		{uuid.New(), "الفرع الشاذ", "1100-01-01", "1110-12-31"},
	}
	peopleIDs := make([]uuid.UUID, 0, len(people))
	for _, person := range people {
		peopleIDs = append(peopleIDs, person.id)
		if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, birth_date_from, birth_date_to, identity_status, created_by) VALUES ($1, $2, $3, $4, $5, 'reviewed', $6)`, person.id, person.name, person.name, person.birthFrom, person.birthTo, actorID); err != nil {
			t.Fatal(err)
		}
	}
	nodeIDs := make([]uuid.UUID, 0, len(people))
	for index, personID := range peopleIDs {
		nodeID := uuid.New()
		nodeIDs = append(nodeIDs, nodeID)
		if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, $5)`, nodeID, versionID, personID, people[index].name, index+1); err != nil {
			t.Fatal(err)
		}
	}
	claimIDs := make([]uuid.UUID, 0, 4)
	for _, pair := range [][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}} {
		relationshipID := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by) VALUES ($1, $2, $3, $4, 'parent_of', 'interpreted', $5, $6)`, relationshipID, versionID, nodeIDs[pair[0]], nodeIDs[pair[1]], sourceID, actorID); err != nil {
			t.Fatal(err)
		}
		claimID := uuid.New()
		statementID := uuid.New()
		claimIDs = append(claimIDs, claimID)
		if _, err := pool.Exec(ctx, `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'parent_of', 'person', $3, 'supported', $4)`, claimID, peopleIDs[pair[0]], peopleIDs[pair[1]], actorID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'عبارة مؤهلة', 'accepted', $3)`, statementID, sourceID, actorID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3)`, claimID, statementID, actorID); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM claims WHERE id = ANY($1::uuid[])`, claimIDs)
	defer pool.Exec(ctx, `DELETE FROM temporal_analysis_runs WHERE tree_version_id = $1`, versionID)
	defer pool.Exec(ctx, `DELETE FROM people WHERE id = ANY($1::uuid[])`, peopleIDs)
	defer pool.Exec(ctx, `DELETE FROM trees WHERE id = $1`, treeID)
	service := NewService(pool)
	run, err := service.StartRun(ctx, actorID.String(), StartRunInput{TreeID: treeID.String(), TreeVersionID: versionID.String(), TargetPersonID: peopleIDs[7].String(), MinReferenceSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != ReportStatusSucceeded || run.ReportStatus != ReportStatusSucceeded || run.FindingCount != 1 || run.ReferencePopulation.ReferenceEdgeCount != 3 {
		t.Fatalf("unexpected temporal run: %+v", run)
	}
	findings, err := service.ListFindings(ctx, actorID.String(), run.ID, "needs_review")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Comparison.Relation != FindingRelationAbove || findings[0].ReferencePopulation.ReferenceEdgeCount != 3 {
		t.Fatalf("unexpected temporal finding: %+v", findings)
	}
	encoded, err := json.Marshal(findings[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "probability") || strings.Contains(string(encoded), "historicalProbability") {
		t.Fatalf("finding contains a probability-like field: %s", encoded)
	}
	contradictionFindings, err := contradiction.NewService(pool, nil).ListFindings(ctx, actorID.String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range contradictionFindings {
		if finding.ID == findings[0].ID {
			t.Fatal("temporal finding leaked into contradiction findings")
		}
	}
	reviewed, err := service.ReviewFinding(ctx, actorID.String(), findings[0].ID, ReviewInput{Decision: "investigate", NoteAR: "يحتاج فحصاً"})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != "investigating" || len(reviewed.Reviews) != 1 {
		t.Fatalf("unexpected reviewed finding: %+v", reviewed)
	}
	var claimStatus, relationshipStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM claims WHERE id = $1`, claimIDs[3]).Scan(&claimStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM tree_relationships WHERE source_id = $1 AND object_node_id = $2`, sourceID, nodeIDs[7]).Scan(&relationshipStatus); err != nil {
		t.Fatal(err)
	}
	if claimStatus != "supported" || relationshipStatus != "interpreted" {
		t.Fatalf("analysis mutated source interpretation: claim=%s relationship=%s", claimStatus, relationshipStatus)
	}
	insufficient, err := service.StartRun(ctx, actorID.String(), StartRunInput{TreeID: treeID.String(), TreeVersionID: versionID.String(), TargetPersonID: peopleIDs[7].String(), MinReferenceSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	if insufficient.ReportStatus != ReportStatusInsufficient || insufficient.FindingCount != 0 {
		t.Fatalf("expected insufficient reference result, got %+v", insufficient)
	}
}

func temporalTestEmail(id uuid.UUID) string {
	return "temporal-analysis-" + id.String() + "@example.test"
}
