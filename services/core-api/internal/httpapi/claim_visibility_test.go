package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestClaimReadRoutesAreScopedPerActor drives the real router so the handler, the
// service and the central visibility policy are exercised together.
func TestClaimReadRoutesAreScopedPerActor(t *testing.T) {
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
	fixture := seedRouteClaimFixture(t, ctx, pool)
	router := NewRouter(Dependencies{DB: pool})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/claims", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("anonymous claim list = %d, want 200", recorder.Code)
	}
	var list struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	anonymous := map[string]string{}
	for _, item := range list.Items {
		anonymous[item.ID] = item.Status
	}
	if _, ok := anonymous[fixture.publicClaimID.String()]; !ok {
		t.Fatalf("public claim is missing from the anonymous list: %s", recorder.Body.String())
	}
	if _, ok := anonymous[fixture.privateClaimID.String()]; ok {
		t.Fatalf("private claim leaked into the anonymous list: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), fixture.privateClaimID.String()) {
		t.Fatalf("anonymous claim list disclosed a private claim id: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "ادعاء الطريق الخاص") {
		t.Fatalf("anonymous claim list disclosed a private claim note: %s", recorder.Body.String())
	}

	// A denied claim and a missing claim must be indistinguishable.
	for _, claimID := range []string{fixture.privateClaimID.String(), fixture.emptyClaimID.String(), uuid.New().String()} {
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/claims/"+claimID, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("anonymous GET claim %s = %d, want 404", claimID, recorder.Code)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "evidence resource not found") {
			t.Fatalf("unexpected error body for %s: %s", claimID, body)
		}
		if strings.Contains(body, "ادعاء") || strings.Contains(body, fixture.privateSourceID.String()) {
			t.Fatalf("error body leaked claim data: %s", body)
		}
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/claims/"+fixture.publicClaimID.String(), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("anonymous public claim read = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "مصدر الطريق العام") {
		t.Fatalf("public claim lost its public source: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "مصدر الطريق الخاص") {
		t.Fatalf("public claim leaked a private source title: %s", recorder.Body.String())
	}
}

type routeClaimFixture struct {
	ownerID           uuid.UUID
	personID          uuid.UUID
	publicClaimID     uuid.UUID
	privateClaimID    uuid.UUID
	emptyClaimID      uuid.UUID
	publicSourceID    uuid.UUID
	privateSourceID   uuid.UUID
	publicStatementID uuid.UUID
}

func seedRouteClaimFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *routeClaimFixture {
	t.Helper()
	fixture := &routeClaimFixture{ownerID: uuid.New(), personID: uuid.New()}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مالك المسار')`, fixture.ownerID, "route-claim-"+fixture.ownerID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered')`, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'شخصية المسار', 'شخصية المسار', $2)`, fixture.personID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.publicSourceID = uuid.New()
	fixture.privateSourceID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر الطريق العام', 'book', 'public', $3), ($2, 'مصدر الطريق الخاص', 'book', 'private', $3)`, fixture.publicSourceID, fixture.privateSourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	publicPassageID := uuid.New()
	privatePassageID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $3, 1, 'مقطع المسار العام', 'مقطع المسار العام'), ($2, $4, 1, 'مقطع المسار الخاص', 'مقطع المسار الخاص')`, publicPassageID, privatePassageID, fixture.publicSourceID, fixture.privateSourceID); err != nil {
		t.Fatal(err)
	}
	fixture.publicStatementID = uuid.New()
	privateStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $3, $5, 'عبارة المسار العامة', 'accepted', $7), ($2, $4, $6, 'عبارة المسار الخاصة', 'accepted', $7)`, fixture.publicStatementID, privateStatementID, fixture.publicSourceID, fixture.privateSourceID, publicPassageID, privatePassageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.publicClaimID = uuid.New()
	fixture.privateClaimID = uuid.New()
	fixture.emptyClaimID = uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by) VALUES
			($1, 'person', $4, 'father_of', 'person', $4, 'supported', 'ادعاء الطريق العام', $5),
			($2, 'person', $4, 'father_of', 'person', $4, 'disputed', 'ادعاء الطريق الخاص', $5),
			($3, 'person', $4, 'father_of', 'person', $4, 'unresolved', 'ادعاء الطريق بلا دليل', $5)`, fixture.publicClaimID, fixture.privateClaimID, fixture.emptyClaimID, fixture.personID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($4, $5, 'supports', $3)`, fixture.publicClaimID, fixture.publicStatementID, fixture.ownerID, fixture.privateClaimID, privateStatementID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, statement := range []string{
			`DELETE FROM claim_evidence WHERE claim_id IN (SELECT id FROM claims WHERE created_by = $1)`,
			`DELETE FROM claims WHERE created_by = $1`,
			`DELETE FROM source_statements WHERE created_by = $1`,
			`DELETE FROM sources WHERE created_by = $1`,
			`DELETE FROM people WHERE created_by = $1`,
			`DELETE FROM audit_log WHERE actor_id = $1`,
			`DELETE FROM users WHERE id = $1`,
		} {
			if _, err := pool.Exec(context.Background(), statement, fixture.ownerID); err != nil {
				t.Errorf("cleanup failed for %q: %v", statement, err)
			}
		}
		if _, err := pool.Exec(context.Background(), `DELETE FROM source_passages WHERE normalized_text_ar LIKE 'مقطع المسار%'`); err != nil {
			t.Errorf("cleanup failed for source passages: %v", err)
		}
	})
	return fixture
}
