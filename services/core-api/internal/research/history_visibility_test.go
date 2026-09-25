package research

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type runHistoryFixture struct {
	ownerID           uuid.UUID
	registeredID      uuid.UUID
	strangerID        uuid.UUID
	questionID        uuid.UUID
	runID             uuid.UUID
	answerID          uuid.UUID
	runContextTreeID  uuid.UUID
	runContextVersion uuid.UUID
}

func TestResearchRunMetadataIsResearchOnly(t *testing.T) {
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
	fixture := seedRunHistoryFixture(t, ctx, pool)
	service := &Service{Pool: pool}

	// Anonymous and unregistered callers are refused before the run is read, and the
	// refusal is identical for a run that exists and one that does not.
	for _, actorID := range []string{"", fixture.registeredID.String(), fixture.strangerID.String()} {
		if _, err := service.GetRun(ctx, actorID, fixture.runID.String()); !errors.Is(err, ErrForbidden) {
			t.Fatalf("GetRun(%q) on an existing run = %v, want %v", actorID, err, ErrForbidden)
		}
		if _, err := service.GetRun(ctx, actorID, uuid.New().String()); !errors.Is(err, ErrForbidden) {
			t.Fatalf("GetRun(%q) on a missing run = %v, want %v", actorID, err, ErrForbidden)
		}
		if _, err := service.ListRuns(ctx, actorID, fixture.questionID.String()); !errors.Is(err, ErrForbidden) {
			t.Fatalf("ListRuns(%q) = %v, want %v", actorID, err, ErrForbidden)
		}
		if _, err := service.ListRuns(ctx, actorID, uuid.New().String()); !errors.Is(err, ErrForbidden) {
			t.Fatalf("ListRuns(%q) on a missing question = %v, want %v", actorID, err, ErrForbidden)
		}
	}

	detail, err := service.GetRun(ctx, fixture.ownerID.String(), fixture.runID.String())
	if err != nil {
		t.Fatalf("researcher could not read the run: %v", err)
	}
	if detail.Query != "سؤال_run_محمي" || detail.AnswerAR != "إجابة_run_محمية" {
		t.Fatalf("researcher lost the run content: %+v", detail.ResearchRunSummary)
	}
	if len(detail.Contexts) != 1 || detail.Contexts[0].ScopeID != fixture.runContextVersion.String() {
		t.Fatalf("unexpected run contexts: %+v", detail.Contexts)
	}
	if detail.CitationCount != 1 {
		t.Fatalf("unexpected citation count: %+v", detail.ResearchRunSummary)
	}

	summaries, err := service.ListRuns(ctx, fixture.ownerID.String(), fixture.questionID.String())
	if err != nil {
		t.Fatalf("researcher could not list runs: %v", err)
	}
	if len(summaries) != 1 || summaries[0].ID != fixture.runID.String() || summaries[0].Query != "سؤال_run_محمي" {
		t.Fatalf("unexpected run summaries: %+v", summaries)
	}

	if _, err := service.GetRun(ctx, "not-a-uuid", fixture.runID.String()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("malformed actor = %v, want %v", err, ErrForbidden)
	}
}

func TestForbiddenResearchHistoryCarriesNoRunData(t *testing.T) {
	// The error value the handler maps is a fixed string: it must not embed query
	// text, ids or internal errors, otherwise a refusal becomes a data channel.
	for _, err := range []error{ErrForbidden, ErrNotFound} {
		message := err.Error()
		for _, forbidden := range []string{"SELECT", "INSERT", "pgx", "http", "row"} {
			if strings.Contains(strings.ToLower(message), strings.ToLower(forbidden)) {
				t.Fatalf("error %q leaks internals: %s", err, message)
			}
		}
	}
	if ErrForbidden.Error() == ErrNotFound.Error() {
		t.Fatal("forbidden and not found must stay distinguishable internally")
	}
}

func seedRunHistoryFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *runHistoryFixture {
	t.Helper()
	fixture := &runHistoryFixture{ownerID: uuid.New(), registeredID: uuid.New(), strangerID: uuid.New(), questionID: uuid.New(), runID: uuid.New()}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, display_name_ar) VALUES
			($1, $4, 'باحث السجل'),
			($2, $5, 'مسجل بلا بحث'),
			($3, $6, 'غريب بلا بحث')`, fixture.ownerID, fixture.registeredID, fixture.strangerID,
		"run-history-researcher-"+fixture.ownerID.String()+"@dawha.test",
		"run-history-registered-"+fixture.registeredID.String()+"@dawha.test",
		"run-history-stranger-"+fixture.strangerID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher'), ($2, 'registered')`, fixture.ownerID, fixture.registeredID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, created_by) VALUES ($1, 'سؤال run محمي', 'open', $2)`, fixture.questionID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	treeID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة run خاصة', 'private', $2)`, treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`, versionID, treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO research_runs (id, question_id, query, normalized_query, status, model_version, actor_id, synthesis_attempted)
		VALUES ($1, $2, 'سؤال_run_محمي', 'سؤال_run_محمي', 'succeeded', 'test-model', $3, true)`, fixture.runID, fixture.questionID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO research_answers (run_id, answer) VALUES ($1, 'إجابة_run_محمية')`, fixture.runID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO research_evidence (run_id, layer, reference_type, reference_id, excerpt) VALUES ($1, 'open_question', 'question', $2, 'مقطع_run_محمي')`, fixture.runID, fixture.questionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO research_run_contexts (run_id, scope_type, scope_id, role) VALUES ($1, 'tree_version', $2, 'subject')`, fixture.runID, versionID); err != nil {
		t.Fatal(err)
	}
	fixture.runContextTreeID = treeID
	fixture.runContextVersion = versionID
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM research_run_contexts WHERE run_id = $1`, fixture.runID},
			{`DELETE FROM research_answers WHERE run_id = $1`, fixture.runID},
			{`DELETE FROM research_evidence WHERE run_id = $1`, fixture.runID},
			{`DELETE FROM research_graph_paths WHERE run_id = $1`, fixture.runID},
			{`DELETE FROM research_runs WHERE id = $1`, fixture.runID},
			{`DELETE FROM open_questions WHERE id = $1`, fixture.questionID},
			{`DELETE FROM tree_versions WHERE id = $1`, fixture.runContextVersion},
			{`DELETE FROM trees WHERE id = $1`, fixture.runContextTreeID},
			{`DELETE FROM audit_log WHERE actor_id = $1`, fixture.ownerID},
			{`DELETE FROM user_roles WHERE user_id = $1`, fixture.ownerID},
			{`DELETE FROM user_roles WHERE user_id = $1`, fixture.registeredID},
			{`DELETE FROM users WHERE id = $1`, fixture.ownerID},
			{`DELETE FROM users WHERE id = $1`, fixture.registeredID},
			{`DELETE FROM users WHERE id = $1`, fixture.strangerID},
		} {
			if _, err := pool.Exec(context.Background(), cleanup.sql, cleanup.arg); err != nil {
				t.Errorf("cleanup failed for %q: %v", cleanup.sql, err)
			}
		}
	})
	return fixture
}
