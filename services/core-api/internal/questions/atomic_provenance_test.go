package questions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// installWriteFault makes the database reject one write, identified by a sentinel value
// in one column, so a mutation can be interrupted at a chosen step and its rollback
// observed. The trigger fires only for the sentinel and is dropped with the test, so
// every other writer in the shared database is unaffected.
func installWriteFault(t *testing.T, pool *pgxpool.Pool, table, column, sentinel string) {
	t.Helper()
	ctx := context.Background()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	function := fmt.Sprintf("dawha_fault_fn_%s", suffix)
	trigger := fmt.Sprintf("dawha_fault_trg_%s", suffix)
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			RAISE EXCEPTION 'injected write fault on %s.%s';
		END;
		$body$
	`, function, table, column)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW
		WHEN (NEW.%s::text = '%s') EXECUTE FUNCTION %s()
	`, trigger, table, column, strings.ReplaceAll(sentinel, "'", "''"), function)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := pool.Exec(cleanupCtx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, trigger, table)); err != nil {
			t.Errorf("dropping the fault trigger failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, function)); err != nil {
			t.Errorf("dropping the fault function failed: %v", err)
		}
	})
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func openAtomicPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	t.Cleanup(pool.Close)
	return pool
}

type questionFixture struct {
	actorID     uuid.UUID
	questionID  uuid.UUID
	updatedAt   time.Time
	questionRow func(t *testing.T, pool *pgxpool.Pool) (string, string, time.Time)
}

func seedQuestionFixture(t *testing.T, pool *pgxpool.Pool) *questionFixture {
	t.Helper()
	ctx := context.Background()
	actorID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'صاحب السؤال')`, actorID, "atomic-question-"+actorID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, actorID); err != nil {
		t.Fatal(err)
	}
	questionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, description_ar, status, priority, created_by) VALUES ($1, 'سؤال ذري', 'وصف ذري', 'open', 'normal', $2)`, questionID, actorID); err != nil {
		t.Fatal(err)
	}
	fixture := &questionFixture{actorID: actorID, questionID: questionID}
	fixture.questionRow = func(t *testing.T, pool *pgxpool.Pool) (string, string, time.Time) {
		t.Helper()
		var title, status string
		var updatedAt time.Time
		if err := pool.QueryRow(ctx, `SELECT title_ar, status, updated_at FROM open_questions WHERE id = $1`, questionID).Scan(&title, &status, &updatedAt); err != nil {
			t.Fatal(err)
		}
		return title, status, updatedAt
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`DELETE FROM question_notes WHERE created_by = $1`, []any{actorID}},
			{`DELETE FROM open_questions WHERE id = $1`, []any{questionID}},
			{`DELETE FROM audit_log WHERE actor_id = $1`, []any{actorID}},
			{`DELETE FROM users WHERE id = $1`, []any{actorID}},
		} {
			if _, err := pool.Exec(cleanupCtx, statement.query, statement.args...); err != nil {
				t.Errorf("cleanup failed for %q: %v", statement.query, err)
			}
		}
	})
	return fixture
}

// A question update and its audit event are one history boundary: when the audit write
// fails, the question must keep the state it had before the attempt.
func TestQuestionUpdateIsRolledBackWhenTheAuditInsertFails(t *testing.T) {
	pool := openAtomicPool(t)
	fixture := seedQuestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)

	if _, err := service.UpdateQuestion(ctx, fixture.questionID.String(), fixture.actorID.String(), UpdateQuestionInput{Priority: "high"}); err != nil {
		t.Fatal(err)
	}
	titleBefore, _, updatedAtBefore := fixture.questionRow(t, pool)

	installWriteFault(t, pool, "audit_log", "action", "question_updated")
	if _, err := service.UpdateQuestion(ctx, fixture.questionID.String(), fixture.actorID.String(), UpdateQuestionInput{TitleAR: "عنوان مرفوض", Status: "resolved"}); err == nil {
		t.Fatal("expected the failing audit write to surface an error")
	}
	titleAfter, statusAfter, updatedAtAfter := fixture.questionRow(t, pool)
	if titleAfter != titleBefore || statusAfter != "open" {
		t.Fatalf("the question kept the rejected update: %q %q", titleAfter, statusAfter)
	}
	if !updatedAtAfter.Equal(updatedAtBefore) {
		t.Fatal("the question timestamp moved without an audit event")
	}
	detail, err := service.GetQuestion(ctx, fixture.questionID.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, activity := range detail.Activity {
		if activity.After != nil && strings.Contains(string(activity.After), "عنوان مرفوض") {
			t.Fatalf("the rejected update is still in the question history: %+v", activity)
		}
	}
}

// A note, the question timestamp it touches, and the audit event commit together.
func TestQuestionNoteIsRolledBackWithItsAuditEvent(t *testing.T) {
	pool := openAtomicPool(t)
	fixture := seedQuestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)

	if _, err := service.AddNote(ctx, fixture.questionID.String(), fixture.actorID.String(), QuestionNoteInput{NoteAR: "ملاحظة سليمة"}); err != nil {
		t.Fatal(err)
	}
	_, _, updatedAtBefore := fixture.questionRow(t, pool)

	installWriteFault(t, pool, "audit_log", "action", "question_note_added")
	if _, err := service.AddNote(ctx, fixture.questionID.String(), fixture.actorID.String(), QuestionNoteInput{NoteAR: "ملاحظة مرفوضة"}); err == nil {
		t.Fatal("expected the failing audit write to surface an error")
	}
	if countRows(t, pool, `SELECT count(*) FROM question_notes WHERE note_ar = 'ملاحظة مرفوضة'`) != 0 {
		t.Fatal("a note survived a failed audit write")
	}
	if _, _, updatedAtAfter := fixture.questionRow(t, pool); !updatedAtAfter.Equal(updatedAtBefore) {
		t.Fatal("the question timestamp moved for a note that was rolled back")
	}
	detail, err := service.GetQuestion(ctx, fixture.questionID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Notes) != 1 {
		t.Fatalf("the rolled back note is still readable: %+v", detail.Notes)
	}
}

// A question and its audit event are one boundary: a failed audit leaves no question.
func TestQuestionCreationIsRolledBackWhenTheAuditInsertFails(t *testing.T) {
	pool := openAtomicPool(t)
	fixture := seedQuestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)

	installWriteFault(t, pool, "audit_log", "action", "question_created")
	if _, err := service.CreateQuestion(ctx, fixture.actorID.String(), CreateQuestionInput{TitleAR: "سؤال مرفوض"}); err == nil {
		t.Fatal("expected the failing audit write to surface an error")
	}
	if countRows(t, pool, `SELECT count(*) FROM open_questions WHERE title_ar = 'سؤال مرفوض'`) != 0 {
		t.Fatal("a question survived a failed audit write")
	}
}
