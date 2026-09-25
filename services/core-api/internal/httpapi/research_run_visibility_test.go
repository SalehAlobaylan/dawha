package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestResearchRunRoutesRefuseAnonymousCallers pins the public contract of the run
// history endpoints: research metadata is research-only data, and the refusal is the
// same whether or not the run exists.
func TestResearchRunRoutesRefuseAnonymousCallers(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	t.Cleanup(pool.Close)
	fixture := seedRunRouteFixture(t, ctx, pool)
	router := NewRouter(Dependencies{DB: pool})

	paths := []string{
		"/api/v1/research/runs/" + fixture.runID.String(),
		"/api/v1/research/runs/" + uuid.New().String(),
		"/api/v1/research/questions/" + fixture.questionID.String() + "/runs",
		"/api/v1/research/questions/" + uuid.New().String() + "/runs",
	}
	for _, path := range paths {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("anonymous GET %s = %d, want 403: %s", path, recorder.Code, recorder.Body.String())
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "research access is forbidden") {
			t.Fatalf("unexpected refusal body for %s: %s", path, body)
		}
		for _, secret := range []string{fixture.runID.String(), fixture.query, fixture.answer, "SELECT", "pgx"} {
			if strings.Contains(body, secret) {
				t.Fatalf("refusal body for %s leaked %q: %s", path, secret, body)
			}
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/research/runs/"+fixture.runID.String(), nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("anonymous run read = %d, want 403", recorder.Code)
	}
}

type runRouteFixture struct {
	researcherID uuid.UUID
	questionID   uuid.UUID
	runID        uuid.UUID
	query        string
	answer       string
}

func seedRunRouteFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *runRouteFixture {
	t.Helper()
	fixture := &runRouteFixture{researcherID: uuid.New(), questionID: uuid.New(), runID: uuid.New(), query: "سؤال_مسار_محمي", answer: "إجابة_مسار_محمية"}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث المسار')`, fixture.researcherID, "run-route-"+fixture.researcherID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, fixture.researcherID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, created_by) VALUES ($1, 'سؤال مسار محمي', 'open', $2)`, fixture.questionID, fixture.researcherID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO research_runs (id, question_id, query, normalized_query, status, model_version, actor_id, synthesis_attempted) VALUES ($1, $2, $3, $3, 'succeeded', 'test-model', $4, true)`, fixture.runID, fixture.questionID, fixture.query, fixture.researcherID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO research_answers (run_id, answer) VALUES ($1, $2)`, fixture.runID, fixture.answer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM research_answers WHERE run_id = $1`, fixture.runID},
			{`DELETE FROM research_evidence WHERE run_id = $1`, fixture.runID},
			{`DELETE FROM research_run_contexts WHERE run_id = $1`, fixture.runID},
			{`DELETE FROM research_runs WHERE id = $1`, fixture.runID},
			{`DELETE FROM open_questions WHERE id = $1`, fixture.questionID},
			{`DELETE FROM user_roles WHERE user_id = $1`, fixture.researcherID},
			{`DELETE FROM audit_log WHERE actor_id = $1`, fixture.researcherID},
			{`DELETE FROM users WHERE id = $1`, fixture.researcherID},
		} {
			if _, err := pool.Exec(context.Background(), cleanup.sql, cleanup.arg); err != nil {
				t.Errorf("cleanup failed for %q: %v", cleanup.sql, err)
			}
		}
	})
	return fixture
}
