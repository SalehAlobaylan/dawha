package research

import (
	"context"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestGraphPersistenceAgainstDatabase(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	input, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationEvidence, GraphStartID: "10000000-0000-0000-0000-000000000001", GraphEndID: "10000000-0000-0000-0000-000000000002", GraphStartType: "person", GraphEndType: "person"})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Pool: pool}
	result, err := service.retrieveGraph(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	runIDText, _, err := service.startRun(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	runID := uuid.MustParse(runIDText)
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, runID)
	viewerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	var viewerRoleExists bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role = 'researcher')`, viewerID).Scan(&viewerRoleExists); err != nil {
		t.Fatal(err)
	}
	if !viewerRoleExists {
		if _, err := pool.Exec(context.Background(), `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, viewerID); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(context.Background(), `DELETE FROM user_roles WHERE user_id = $1 AND role = 'researcher'`, viewerID)
	}
	if err := service.persistRun(context.Background(), runIDText, QueryResult{Answer: "إجابة", GraphPaths: result.Paths, GraphStats: result.Stats}); err != nil {
		t.Fatal(err)
	}
	publicDetail, err := service.GetRun(context.Background(), "", runIDText)
	if err != nil {
		t.Fatal(err)
	}
	if len(publicDetail.GraphPaths) != 0 || publicDetail.GraphPathCount != 0 || publicDetail.GraphOperation != "" || len(publicDetail.Contexts) != 0 {
		t.Fatalf("public graph detail leaked metadata: %+v", publicDetail)
	}
	loaded, err := runGraphPaths(context.Background(), pool, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(result.Paths) {
		t.Fatalf("loaded paths = %d, want %d", len(loaded), len(result.Paths))
	}
	if err := validateGraphPaths(loaded, input.GraphMaxDepth); err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetRun(context.Background(), viewerID.String(), runIDText)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.GraphPaths) != len(result.Paths) || detail.GraphOperation != input.GraphOperation || detail.GraphMaxDepth != input.GraphMaxDepth {
		t.Fatalf("unexpected graph run detail: %+v", detail)
	}
}

func TestGraphPersistenceAllowsSharedEvidenceAcrossClaims(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	claimID := uuid.MustParse("69000000-0000-0000-0000-000000009001")
	defer pool.Exec(context.Background(), `DELETE FROM claims WHERE id = $1`, claimID)
	if _, err := pool.Exec(context.Background(), `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', '10000000-0000-0000-0000-000000000001', 'father_of', 'person', '10000000-0000-0000-0000-000000000002', 'supported', '00000000-0000-0000-0000-000000000001')`, claimID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, '50000000-0000-0000-0000-000000000001', 'supports', '00000000-0000-0000-0000-000000000001')`, claimID); err != nil {
		t.Fatal(err)
	}
	input, err := validateQueryInput(QueryInput{Question: "سؤال", GraphOperation: GraphOperationBranchClaims, GraphStartType: "person", GraphStartID: "10000000-0000-0000-0000-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Pool: pool}
	result, err := service.retrieveGraph(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	runIDText, _, err := service.startRun(context.Background(), input, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM research_runs WHERE id = $1`, runIDText)
	if err := service.persistRun(context.Background(), runIDText, QueryResult{Answer: "إجابة", GraphPaths: result.Paths, GraphStats: result.Stats}); err != nil {
		t.Fatal(err)
	}
}
