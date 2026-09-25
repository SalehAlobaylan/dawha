package suggestions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChangeSetRejectsMalformedTargets(t *testing.T) {
	cases := []struct {
		name  string
		input ChangeSet
	}{
		{
			name:  "unknown target",
			input: ChangeSet{Target: "tree"},
		},
		{
			name:  "empty change set",
			input: ChangeSet{},
		},
		{
			name:  "target without its block",
			input: ChangeSet{Target: ChangeTargetClaim},
		},
		{
			name: "block that does not match the target",
			input: ChangeSet{
				Target: ChangeTargetClaim,
				Person: &PersonChange{PersonID: "10000000-0000-0000-0000-000000000001", NameAR: "لقب"},
			},
		},
		{
			name: "second block beside the target block",
			input: ChangeSet{
				Target: ChangeTargetClaim,
				Claim:  &ClaimChange{SubjectID: "10000000-0000-0000-0000-000000000001", ObjectID: "10000000-0000-0000-0000-000000000002", Predicate: "father_of"},
				Person: &PersonChange{PersonID: "10000000-0000-0000-0000-000000000001", NameAR: "لقب"},
			},
		},
		{
			name: "malformed identifier",
			input: ChangeSet{
				Target: ChangeTargetPerson,
				Person: &PersonChange{PersonID: "not-a-uuid", NameAR: "لقب"},
			},
		},
		{
			name: "unknown alias type",
			input: ChangeSet{
				Target: ChangeTargetPerson,
				Person: &PersonChange{PersonID: "10000000-0000-0000-0000-000000000001", NameAR: "لقب", AliasType: "nickname"},
			},
		},
		{
			name: "empty name",
			input: ChangeSet{
				Target: ChangeTargetPerson,
				Person: &PersonChange{PersonID: "10000000-0000-0000-0000-000000000001", NameAR: "  "},
			},
		},
		{
			name: "unknown predicate",
			input: ChangeSet{
				Target: ChangeTargetClaim,
				Claim:  &ClaimChange{SubjectID: "10000000-0000-0000-0000-000000000001", ObjectID: "10000000-0000-0000-0000-000000000002", Predicate: "شغله"},
			},
		},
		{
			name: "unknown entity type",
			input: ChangeSet{
				Target: ChangeTargetRelationship,
				Relationship: &RelationshipChange{
					SubjectType: "ship", SubjectID: "10000000-0000-0000-0000-000000000001",
					ObjectType: "person", ObjectID: "10000000-0000-0000-0000-000000000002", Predicate: "sibling_of",
				},
			},
		},
		{
			name: "relationship with itself",
			input: ChangeSet{
				Target: ChangeTargetRelationship,
				Relationship: &RelationshipChange{
					SubjectID: "10000000-0000-0000-0000-000000000001", ObjectID: "10000000-0000-0000-0000-000000000001", Predicate: "sibling_of",
				},
			},
		},
		{
			name: "inverted validity range",
			input: ChangeSet{
				Target: ChangeTargetClaim,
				Claim: &ClaimChange{
					SubjectID: "10000000-0000-0000-0000-000000000001", ObjectID: "10000000-0000-0000-0000-000000000002",
					Predicate: "father_of", TimeFrom: "1900-01-01", TimeTo: "1800-01-01",
				},
			},
		},
		{
			name: "malformed date",
			input: ChangeSet{
				Target: ChangeTargetClaim,
				Claim: &ClaimChange{
					SubjectID: "10000000-0000-0000-0000-000000000001", ObjectID: "10000000-0000-0000-0000-000000000002",
					Predicate: "father_of", TimeFrom: "من قرن",
				},
			},
		},
		{
			name: "source link with no reference",
			input: ChangeSet{
				Target:     ChangeTargetSourceLink,
				SourceLink: &SourceLinkChange{ClaimID: "60000000-0000-0000-0000-000000000001"},
			},
		},
		{
			name: "source link with two references",
			input: ChangeSet{
				Target: ChangeTargetSourceLink,
				SourceLink: &SourceLinkChange{
					ClaimID:           "60000000-0000-0000-0000-000000000001",
					SourceStatementID: "20000000-0000-0000-0000-000000000001",
					SourcePassageID:   "20000000-0000-0000-0000-000000000002",
				},
			},
		},
		{
			name: "source link with an unknown relation",
			input: ChangeSet{
				Target: ChangeTargetSourceLink,
				SourceLink: &SourceLinkChange{
					ClaimID: "60000000-0000-0000-0000-000000000001", SourceStatementID: "20000000-0000-0000-0000-000000000001", Relation: "proves",
				},
			},
		},
	}
	for _, testCase := range cases {
		if _, err := validateChangeSet(testCase.input); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s: expected a validation error, got %v", testCase.name, err)
		}
	}
}

// A change set is a change, so it belongs to an acceptance. This is the decision-level
// half of the validation; the structure of the change set is checked once, in
// validateChangeSet, before the review transaction opens.
func TestChangeSetIsRefusedOnADecisionThatChangesNothing(t *testing.T) {
	change := &ChangeSet{
		Target: ChangeTargetClaim,
		Claim:  &ClaimChange{SubjectID: "10000000-0000-0000-0000-000000000001", ObjectID: "10000000-0000-0000-0000-000000000002", Predicate: "father_of"},
	}
	for _, decision := range []string{"rejected", "converted"} {
		input := ReviewInput{Decision: decision, NoteAR: "قرار", ChangeSet: change}
		if _, err := validateReviewInput(input); !errors.Is(err, ErrValidation) {
			t.Fatalf("a change set on a %s review: expected a validation error, got %v", decision, err)
		}
	}
	if _, err := validateReviewInput(ReviewInput{Decision: "accepted", ChangeSet: change}); err != nil {
		t.Fatalf("an accepted review with a change set was rejected: %v", err)
	}
}

func TestChangeSetAcceptsEveryTypedTarget(t *testing.T) {
	cases := []struct {
		name   string
		change ChangeSet
	}{
		{name: "person", change: ChangeSet{Target: ChangeTargetPerson, Person: &PersonChange{PersonID: "10000000-0000-0000-0000-000000000001", NameAR: "أبو الفضل"}}},
		{name: "relationship", change: ChangeSet{Target: ChangeTargetRelationship, Relationship: &RelationshipChange{SubjectID: "10000000-0000-0000-0000-000000000001", ObjectID: "10000000-0000-0000-0000-000000000002", Predicate: "father_of"}}},
		{name: "claim", change: ChangeSet{Target: ChangeTargetClaim, Claim: &ClaimChange{SubjectType: "person", SubjectID: "10000000-0000-0000-0000-000000000001", ObjectType: "person", ObjectID: "10000000-0000-0000-0000-000000000002", Predicate: "father_of", TimeFrom: "1900-01-01"}}},
		{name: "source link", change: ChangeSet{Target: ChangeTargetSourceLink, SourceLink: &SourceLinkChange{ClaimID: "60000000-0000-0000-0000-000000000001", SourceStatementID: "20000000-0000-0000-0000-000000000001", Relation: "supports"}}},
	}
	for _, testCase := range cases {
		plan, err := validateChangeSet(testCase.change)
		if err != nil {
			t.Fatalf("%s: unexpected validation error: %v", testCase.name, err)
		}
		if plan.target != testCase.change.Target {
			t.Fatalf("%s: target = %q, want %q", testCase.name, plan.target, testCase.change.Target)
		}
	}
	if _, err := validateReviewInput(ReviewInput{Decision: "accepted", ChangeSet: &cases[2].change}); err != nil {
		t.Fatalf("an accepted review with a typed change set was rejected: %v", err)
	}
	if _, err := validateReviewInput(ReviewInput{Decision: "accepted"}); err != nil {
		t.Fatalf("an accepted review without a change set was rejected: %v", err)
	}
}

type suggestionFixture struct {
	ownerID       uuid.UUID
	personID      uuid.UUID
	otherPersonID uuid.UUID
	claimID       uuid.UUID
	sourceID      uuid.UUID
	passageID     uuid.UUID
	statementID   uuid.UUID
	treeID        uuid.UUID
	versionID     uuid.UUID
	nodeID        uuid.UUID
}

func openSuggestionPool(t *testing.T) *pgxpool.Pool {
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

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// installWriteFault makes the database reject one write, identified by a sentinel value
// in one column, so a review can be interrupted at a chosen step and its rollback
// observed. The trigger fires only for the sentinel and is dropped with the test.
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

func seedSuggestionFixture(t *testing.T, pool *pgxpool.Pool) *suggestionFixture {
	t.Helper()
	ctx := context.Background()
	fixture := &suggestionFixture{
		ownerID: uuid.New(), personID: uuid.New(), otherPersonID: uuid.New(), claimID: uuid.New(),
		sourceID: uuid.New(), passageID: uuid.New(), statementID: uuid.New(),
		treeID: uuid.New(), versionID: uuid.New(), nodeID: uuid.New(),
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'راجع الشجرة')`, fixture.ownerID, "suggestion-reviewer-"+fixture.ownerID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'أبو عبد الله', 'ابو عبدالله', $3), ($2, 'عبد الله', 'عبدالله', $3)`, fixture.personID, fixture.otherPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر المقترح', 'book', 'public', $2)`, fixture.sourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, 'مقطع المقترح', 'مقطع المقترح')`, fixture.passageID, fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, 'عبارة المصدر', 'accepted', $4)`, fixture.statementID, fixture.sourceID, fixture.passageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'father_of', 'person', $3, 'unresolved', $4)`, fixture.claimID, fixture.personID, fixture.otherPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة المقترح', 'public', $2)`, fixture.treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, fixture.versionID, fixture.treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'أبو عبد الله', 0)`, fixture.nodeID, fixture.versionID, fixture.personID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`DELETE FROM suggestion_change_sets WHERE suggestion_id IN (SELECT id FROM suggestions WHERE tree_id = $1)`, []any{fixture.treeID}},
			{`DELETE FROM suggestion_reviews WHERE suggestion_id IN (SELECT id FROM suggestions WHERE tree_id = $1)`, []any{fixture.treeID}},
			{`DELETE FROM suggestions WHERE tree_id = $1`, []any{fixture.treeID}},
			{`DELETE FROM claim_evidence WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM claim_counter_evidence WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM claim_versions WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM claims WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM entity_relationships WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM person_aliases WHERE person_id IN (SELECT id FROM people WHERE id = ANY($1::uuid[]))`, []any{[]uuid.UUID{fixture.personID, fixture.otherPersonID}}},
			{`DELETE FROM open_questions WHERE created_by = $1`, []any{fixture.ownerID}},
			{`DELETE FROM tree_nodes WHERE tree_version_id = $1`, []any{fixture.versionID}},
			{`DELETE FROM tree_versions WHERE id = $1`, []any{fixture.versionID}},
			{`DELETE FROM trees WHERE id = $1`, []any{fixture.treeID}},
			{`DELETE FROM source_statements WHERE id = $1`, []any{fixture.statementID}},
			{`DELETE FROM source_passages WHERE id = $1`, []any{fixture.passageID}},
			{`DELETE FROM sources WHERE id = $1`, []any{fixture.sourceID}},
			{`DELETE FROM people WHERE id = ANY($1::uuid[])`, []any{[]uuid.UUID{fixture.personID, fixture.otherPersonID}}},
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

func submitSuggestion(t *testing.T, pool *pgxpool.Pool, fixture *suggestionFixture, text string) SuggestionView {
	t.Helper()
	service := NewService(pool)
	view, err := service.Submit(context.Background(), SubmitInput{
		TreeID: fixture.treeID.String(), VersionID: fixture.versionID.String(), NodeID: fixture.nodeID.String(), TextAR: text,
	}, fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func suggestionStatus(t *testing.T, pool *pgxpool.Pool, suggestionID string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM suggestions WHERE id = $1`, suggestionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// An accepted suggestion with a change set applies the change, records it beside the
// review, and links the original text, the reviewer, the change and the resulting record
// in one audit event. The claim it creates stays unresolved: acceptance never accepts a
// claim, and no tree is published.
func TestAcceptedSuggestionAppliesItsChangeSetInOneTransaction(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	suggestion := submitSuggestion(t, pool, fixture, "أضفوا أن أبا عبد الله والد عبد الله")

	reviewed, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		NoteAR:    "مؤكد من السجل",
		ChangeSet: &ChangeSet{Target: ChangeTargetClaim, Claim: &ClaimChange{SubjectType: "person", SubjectID: fixture.personID.String(), ObjectType: "person", ObjectID: fixture.otherPersonID.String(), Predicate: "father_of", NoteAR: "من المقترح"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != "accepted" || reviewed.ReviewCount != 1 {
		t.Fatalf("unexpected reviewed suggestion: %+v", reviewed)
	}
	if reviewed.QuestionID != "" {
		t.Fatalf("a change set review also created a question: %+v", reviewed)
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE created_by = $1 AND status = 'unresolved' AND notes_ar = 'من المقترح'`, fixture.ownerID) != 1 {
		t.Fatal("the approved claim was not recorded as unresolved")
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_versions WHERE created_by = $1 AND change_reason_ar = 'من المقترح'`, fixture.ownerID) != 1 {
		t.Fatal("the approved claim has no version row")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id = $1 AND target = 'claim' AND result_type = 'claim'`, suggestion.ID) != 1 {
		t.Fatal("the change set was not stored beside the review")
	}
	if countRows(t, pool, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'suggestion_change_applied' AND entity_id = $2 AND after_value->>'reviewerId' = $3 AND after_value->>'resultId' IS NOT NULL`, fixture.ownerID, suggestion.ID, fixture.ownerID.String()) != 1 {
		t.Fatal("the audit event does not link the text, reviewer, change and result")
	}
	var recordedText string
	if err := pool.QueryRow(ctx, `SELECT before_value->>'textAr' FROM audit_log WHERE action = 'suggestion_change_applied' AND entity_id = $1`, suggestion.ID).Scan(&recordedText); err != nil {
		t.Fatal(err)
	}
	if recordedText != "أضفوا أن أبا عبد الله والد عبد الله" {
		t.Fatalf("the audit event lost the original text: %q", recordedText)
	}
	if countRows(t, pool, `SELECT count(*) FROM tree_versions WHERE id = $1 AND state = 'published'`, fixture.versionID) != 1 {
		t.Fatal("the suggestion review disturbed the published version")
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE status <> 'unresolved' AND created_by = $1`, fixture.ownerID) != 0 {
		t.Fatal("the review accepted a claim instead of proposing one")
	}
}

// Every typed target writes its own record: an alias, an unresolved relationship, an
// evidence link and a claim.
func TestAcceptedSuggestionAppliesEveryTypedTarget(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)

	alias := submitSuggestion(t, pool, fixture, "لقب أبو الفضل")
	if _, err := service.Review(ctx, alias.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetPerson, Person: &PersonChange{PersonID: fixture.personID.String(), NameAR: "أبو الفضل"}},
	}); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM person_aliases WHERE person_id = $1 AND value_ar = 'أبو الفضل'`, fixture.personID) != 1 {
		t.Fatal("the approved alias was not recorded")
	}

	relationship := submitSuggestion(t, pool, fixture, "صلة القرابة غير موثقة")
	if _, err := service.Review(ctx, relationship.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetRelationship, Relationship: &RelationshipChange{SubjectID: fixture.personID.String(), ObjectID: fixture.otherPersonID.String(), Predicate: "sibling_of"}},
	}); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM entity_relationships WHERE created_by = $1 AND status = 'unresolved' AND predicate = 'sibling_of'`, fixture.ownerID) != 1 {
		t.Fatal("the approved relationship was not recorded as unresolved")
	}

	link := submitSuggestion(t, pool, fixture, "العبارة دليل على الادعاء")
	if _, err := service.Review(ctx, link.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetSourceLink, SourceLink: &SourceLinkChange{ClaimID: fixture.claimID.String(), SourceStatementID: fixture.statementID.String(), Relation: "supports"}},
	}); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_evidence WHERE claim_id = $1 AND source_statement_id = $2 AND created_by = $3`, fixture.claimID, fixture.statementID, fixture.ownerID) != 1 {
		t.Fatal("the approved evidence link was not recorded")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id::text = ANY($1)`, []string{alias.ID, relationship.ID, link.ID}) != 3 {
		t.Fatal("an applied change set is missing from the review record")
	}
}

// A change set that names a record which does not exist is refused, and the suggestion
// stays pending with nothing written.
func TestAcceptedSuggestionRefusesAChangeSetForAMissingRecord(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	suggestion := submitSuggestion(t, pool, fixture, "أضفوا لقباً")

	if _, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetPerson, Person: &PersonChange{PersonID: uuid.New().String(), NameAR: "لقب"}},
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a change set for a missing person = %v, want %v", err, ErrNotFound)
	}
	if suggestionStatus(t, pool, suggestion.ID) != "pending" {
		t.Fatal("the suggestion left the queue after a refused change set")
	}
	if countRows(t, pool, `SELECT count(*) FROM person_aliases WHERE value_ar = 'لقب'`) != 0 {
		t.Fatal("a refused change set still wrote an alias")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id = $1`, suggestion.ID) != 0 {
		t.Fatal("a refused change set was stored")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_reviews WHERE suggestion_id = $1`, suggestion.ID) != 0 {
		t.Fatal("a refused change set still recorded a review")
	}
}

// The same change set twice fails on the database's own uniqueness rule, so the second
// approval writes nothing and its suggestion stays in the queue.
func TestDuplicateChangeSetLeavesTheSuggestionPending(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	change := ChangeSet{Target: ChangeTargetPerson, Person: &PersonChange{PersonID: fixture.personID.String(), NameAR: "أبو الفضل", AliasType: "kunyah"}}

	first := submitSuggestion(t, pool, fixture, "اللقب الأول")
	if _, err := service.Review(ctx, first.ID, fixture.ownerID.String(), ReviewInput{Decision: "accepted", ChangeSet: &change}); err != nil {
		t.Fatal(err)
	}
	second := submitSuggestion(t, pool, fixture, "اللقب المكرر")
	if _, err := service.Review(ctx, second.ID, fixture.ownerID.String(), ReviewInput{Decision: "accepted", ChangeSet: &change}); err == nil {
		t.Fatal("expected the duplicate change set to be refused")
	}
	if suggestionStatus(t, pool, second.ID) != "pending" {
		t.Fatal("a duplicate change set moved the suggestion out of the queue")
	}
	if countRows(t, pool, `SELECT count(*) FROM person_aliases WHERE person_id = $1 AND value_ar = 'أبو الفضل'`, fixture.personID) != 1 {
		t.Fatal("the duplicate change set wrote a second alias")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_reviews WHERE suggestion_id = $1`, second.ID) != 0 {
		t.Fatal("a duplicate change set still recorded a review")
	}
}

// A failing audit write takes the whole review with it: the applied change, the review
// row, the stored change set and the status change all disappear together.
func TestAcceptedSuggestionIsRolledBackWhenTheAuditInsertFails(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	suggestion := submitSuggestion(t, pool, fixture, "سجل خاص بالفشل")

	installWriteFault(t, pool, "audit_log", "action", "suggestion_reviewed")
	if _, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetClaim, Claim: &ClaimChange{SubjectID: fixture.personID.String(), ObjectID: fixture.otherPersonID.String(), Predicate: "father_of", NoteAR: "سبب الفشل"}},
	}); err == nil {
		t.Fatal("expected the failing audit write to surface an error")
	}
	if suggestionStatus(t, pool, suggestion.ID) != "pending" {
		t.Fatal("the suggestion left the queue after a failed audit write")
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE notes_ar = 'سبب الفشل'`) != 0 {
		t.Fatal("the applied claim survived a failed audit write")
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_versions WHERE change_reason_ar = 'سبب الفشل'`) != 0 {
		t.Fatal("the applied claim version survived a failed audit write")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id = $1`, suggestion.ID) != 0 {
		t.Fatal("the change set survived a failed audit write")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_reviews WHERE suggestion_id = $1`, suggestion.ID) != 0 {
		t.Fatal("the review survived a failed audit write")
	}
}

// A failing change-set write also rolls the review back, so the decision and the change
// are never separated.
func TestAcceptedSuggestionIsRolledBackWhenTheChangeSetWriteFails(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	suggestion := submitSuggestion(t, pool, fixture, "سجل خاص بفشل السجل")

	installWriteFault(t, pool, "suggestion_change_sets", "target", "claim")
	if _, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetClaim, Claim: &ClaimChange{SubjectID: fixture.personID.String(), ObjectID: fixture.otherPersonID.String(), Predicate: "father_of", NoteAR: "سبب فشل السجل"}},
	}); err == nil {
		t.Fatal("expected the failing change-set write to surface an error")
	}
	if suggestionStatus(t, pool, suggestion.ID) != "pending" {
		t.Fatal("the suggestion left the queue after a failed change-set write")
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE created_by = $1 AND notes_ar = 'سبب فشل السجل'`, fixture.ownerID) != 0 {
		t.Fatal("the applied claim survived a failed change-set write")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_reviews WHERE suggestion_id = $1`, suggestion.ID) != 0 {
		t.Fatal("the review survived a failed change-set write")
	}
}

// An acceptance with no change set records the proposer's text as an open question, so
// the decision still leaves a documented artifact. A rejection changes nothing, and a
// conversion keeps doing what it did.
func TestReviewDecisionsWithoutAChangeSet(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)

	accepted := submitSuggestion(t, pool, fixture, "مقترح بلا تغيير")
	reviewed, err := service.Review(ctx, accepted.ID, fixture.ownerID.String(), ReviewInput{Decision: "accepted", NoteAR: "مقبول مبدئياً"})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != "accepted" || reviewed.QuestionID == "" {
		t.Fatalf("an acceptance without a change set left no artifact: %+v", reviewed)
	}
	var description, title string
	if err := pool.QueryRow(ctx, `SELECT title_ar, description_ar FROM open_questions WHERE id = $1`, reviewed.QuestionID).Scan(&title, &description); err != nil {
		t.Fatal(err)
	}
	if description != "مقترح بلا تغيير" {
		t.Fatalf("the question did not keep the proposer's text: %q", description)
	}
	if countRows(t, pool, `SELECT count(*) FROM audit_log WHERE action = 'question_created_from_suggestion' AND entity_id = $1`, reviewed.QuestionID) != 1 {
		t.Fatal("the question artifact is not linked to the suggestion by an audit event")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id = $1`, accepted.ID) != 0 {
		t.Fatal("an acceptance without a change set recorded a change set")
	}

	rejected := submitSuggestion(t, pool, fixture, "مقترح مرفوض")
	rejectedView, err := service.Review(ctx, rejected.ID, fixture.ownerID.String(), ReviewInput{Decision: "rejected", NoteAR: "لا مصدر"})
	if err != nil {
		t.Fatal(err)
	}
	if rejectedView.Status != "rejected" || rejectedView.QuestionID != "" {
		t.Fatalf("a rejection changed more than its status: %+v", rejectedView)
	}

	converted := submitSuggestion(t, pool, fixture, "مقترح يتحول إلى سؤال")
	convertedView, err := service.Review(ctx, converted.ID, fixture.ownerID.String(), ReviewInput{Decision: "converted", QuestionTitleAR: "سؤال محدد"})
	if err != nil {
		t.Fatal(err)
	}
	if convertedView.Status != "converted" || convertedView.QuestionID == "" {
		t.Fatalf("a conversion left no question: %+v", convertedView)
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE created_by = $1`, fixture.ownerID) != 1 {
		t.Fatal("a review decision created a claim on its own")
	}
}

// A review of a suggestion that is not the reviewer's tree, or that was already
// reviewed, is refused exactly as before.
func TestReviewAuthorizationAndConflictAreUnchanged(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	claimChange := &ChangeSet{Target: ChangeTargetClaim, Claim: &ClaimChange{SubjectID: fixture.personID.String(), ObjectID: fixture.otherPersonID.String(), Predicate: "father_of"}}

	suggestion := submitSuggestion(t, pool, fixture, "مقترح لمالك آخر")
	strangerID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'غريب')`, strangerID, "suggestion-stranger-"+strangerID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM audit_log WHERE actor_id = $1`, strangerID); err != nil {
			t.Errorf("cleanup failed for the stranger: %v", err)
		}
		if _, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, strangerID); err != nil {
			t.Errorf("cleanup failed for the stranger: %v", err)
		}
	})
	if _, err := service.Review(ctx, suggestion.ID, strangerID.String(), ReviewInput{Decision: "accepted", ChangeSet: claimChange}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an unrelated reviewer applied a change: %v", err)
	}
	if _, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{Decision: "accepted", ChangeSet: claimChange}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{Decision: "accepted", ChangeSet: claimChange}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a reviewed suggestion was changed again: %v", err)
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE created_by = $1`, fixture.ownerID) != 2 {
		t.Fatal("a repeated review applied the change twice")
	}
}
