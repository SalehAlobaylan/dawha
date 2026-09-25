package evidence

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type atomicFixture struct {
	ownerID   uuid.UUID
	personID  uuid.UUID
	sourceID  uuid.UUID
	passageID uuid.UUID
}

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

func seedAtomicFixture(t *testing.T, pool *pgxpool.Pool) *atomicFixture {
	t.Helper()
	ctx := context.Background()
	fixture := &atomicFixture{ownerID: uuid.New(), personID: uuid.New(), sourceID: uuid.New(), passageID: uuid.New()}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'صاحب المصدر')`, fixture.ownerID, "atomic-owner-"+fixture.ownerID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'شخصية الذرّية', 'شخصية الذرّية', $2)`, fixture.personID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر الذرّية', 'book', 'private', $2)`, fixture.sourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, 'مقطع الذرّية', 'مقطع الذرّية')`, fixture.passageID, fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`DELETE FROM claim_evidence WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM claim_counter_evidence WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM claim_versions WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM claims WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM source_statements WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM sources WHERE id = $1`, []any{fixture.sourceID}},
			{`DELETE FROM people WHERE id = $1`, []any{fixture.personID}},
			{`DELETE FROM audit_log WHERE actor_id = $1`, []any{fixture.ownerID}},
			{`DELETE FROM users WHERE id = $1`, []any{fixture.ownerID}},
		} {
			if _, err := pool.Exec(cleanupCtx, statement.query, statement.args...); err != nil {
				t.Errorf("cleanup failed for %q: %v", statement.query, err)
			}
		}
	})
	return fixture
}

// A statement and its audit event form one history boundary: when the audit write
// fails, the statement must not survive.
func TestStatementIsRolledBackWhenTheAuditInsertFails(t *testing.T) {
	pool := openAtomicPool(t)
	fixture := seedAtomicFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)

	created, err := service.CreateStatement(ctx, fixture.sourceID.String(), fixture.ownerID.String(), SourceStatementInput{SourcePassageID: fixture.passageID.String(), StatementTextAR: "عبارة ذرية سليمة", ReviewStatus: "unreviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Statements) != 1 {
		t.Fatalf("expected the statement to be recorded, got %+v", created.Statements)
	}
	if countRows(t, pool, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'source_statement_recorded'`, fixture.ownerID) != 1 {
		t.Fatal("the healthy statement write did not leave an audit event")
	}

	installWriteFault(t, pool, "audit_log", "action", "source_statement_recorded")
	if _, err := service.CreateStatement(ctx, fixture.sourceID.String(), fixture.ownerID.String(), SourceStatementInput{SourcePassageID: fixture.passageID.String(), StatementTextAR: "عبارة ذرية مرفوضة", ReviewStatus: "unreviewed"}); err == nil {
		t.Fatal("expected the failing audit write to surface an error")
	}
	if countRows(t, pool, `SELECT count(*) FROM source_statements WHERE statement_text_ar = 'عبارة ذرية مرفوضة'`) != 0 {
		t.Fatal("a statement survived a failed audit write")
	}
	detail, err := service.GetSourceForActor(ctx, fixture.sourceID.String(), fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Statements) != 1 {
		t.Fatalf("the rolled back statement is still readable: %+v", detail.Statements)
	}
}

// A claim, its first version snapshot, and its audit event are one history boundary.
func TestClaimIsRolledBackWithItsVersionAndAuditEvent(t *testing.T) {
	pool := openAtomicPool(t)
	fixture := seedAtomicFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	input := func(note string) CreateClaimInput {
		return CreateClaimInput{SubjectType: "person", SubjectID: fixture.personID.String(), Predicate: "father_of", ObjectType: "person", ObjectID: fixture.personID.String(), Status: "unresolved", NotesAR: note}
	}

	created, err := service.CreateClaim(ctx, fixture.ownerID.String(), input("ادعاء ذري"))
	if err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_versions WHERE claim_id = $1 AND version_number = 1`, created.ID) != 1 {
		t.Fatal("the healthy claim write did not leave a version row")
	}

	installWriteFault(t, pool, "audit_log", "action", "claim_created")
	auditsBefore := countRows(t, pool, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'claim_created'`, fixture.ownerID)
	if _, err := service.CreateClaim(ctx, fixture.ownerID.String(), input("ادعاء بلا سجل")); err == nil {
		t.Fatal("expected the failing audit write to surface an error")
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE notes_ar = 'ادعاء بلا سجل'`) != 0 {
		t.Fatal("a claim survived a failed audit write")
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_versions WHERE change_reason_ar = 'ادعاء بلا سجل'`) != 0 {
		t.Fatal("a claim version survived a failed audit write")
	}
	if countRows(t, pool, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'claim_created'`, fixture.ownerID) != auditsBefore {
		t.Fatal("a claim audit event survived without its claim")
	}
}

// The version row is the claim's history; a version insert that fails must take the
// claim with it.
func TestClaimIsRolledBackWhenTheVersionInsertFails(t *testing.T) {
	pool := openAtomicPool(t)
	fixture := seedAtomicFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)

	installWriteFault(t, pool, "claim_versions", "change_reason_ar", "سبب يفشل الإصدار")
	_, err := service.CreateClaim(ctx, fixture.ownerID.String(), CreateClaimInput{
		SubjectType: "person", SubjectID: fixture.personID.String(), Predicate: "father_of", ObjectType: "person",
		ObjectID: fixture.personID.String(), Status: "unresolved", NotesAR: "سبب يفشل الإصدار",
	})
	if err == nil {
		t.Fatal("expected the failing version write to surface an error")
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE notes_ar = 'سبب يفشل الإصدار'`) != 0 {
		t.Fatal("a claim survived a failed version write")
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_versions WHERE change_reason_ar = 'سبب يفشل الإصدار'`) != 0 {
		t.Fatal("a claim version survived its own failing write")
	}
	if countRows(t, pool, `SELECT count(*) FROM audit_log WHERE action = 'claim_created' AND after_value->>'notes_ar' = 'سبب يفشل الإصدار'`) != 0 {
		t.Fatal("a claim audit event survived a failed version write")
	}
}

// An evidence link and its audit event are one boundary: a claim never shows evidence
// the audit log cannot explain.
func TestEvidenceLinkIsRolledBackWhenTheAuditInsertFails(t *testing.T) {
	pool := openAtomicPool(t)
	fixture := seedAtomicFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	statementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, 'عبارة دليل', 'accepted', $4)`, statementID, fixture.sourceID, fixture.passageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	claim, err := service.CreateClaim(ctx, fixture.ownerID.String(), CreateClaimInput{
		SubjectType: "person", SubjectID: fixture.personID.String(), Predicate: "father_of", ObjectType: "person",
		ObjectID: fixture.personID.String(), Status: "unresolved",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddEvidence(ctx, claim.ID, fixture.ownerID.String(), AddEvidenceInput{SourceStatementID: statementID.String(), Relation: "supports"}); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_evidence WHERE claim_id = $1`, claim.ID) != 1 {
		t.Fatal("the healthy evidence link did not land")
	}

	installWriteFault(t, pool, "audit_log", "action", "claim_evidence_linked")
	if _, err := service.AddEvidence(ctx, claim.ID, fixture.ownerID.String(), AddEvidenceInput{SourceStatementID: statementID.String(), Relation: "contextualizes", EvidenceNoteAR: "دليل مرفوض"}); err == nil {
		t.Fatal("expected the failing audit write to surface an error")
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_evidence WHERE claim_id = $1 AND evidence_note_ar = 'دليل مرفوض'`, claim.ID) != 0 {
		t.Fatal("an evidence link survived a failed audit write")
	}
	view, err := service.GetClaim(ctx, claim.ID, fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Evidence) != 1 {
		t.Fatalf("the claim shows evidence the audit log never recorded: %+v", view.Evidence)
	}
}
