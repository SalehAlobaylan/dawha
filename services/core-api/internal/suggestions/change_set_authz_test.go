package suggestions

import (
	"context"
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dictionary"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// addTreeReviewer adds a reviewer to the fixture tree. An empty role list makes a plain
// tree collaborator, who holds review rights over one tree and no global role at all.
func addTreeReviewer(t *testing.T, pool *pgxpool.Pool, fixture *suggestionFixture, label string, roles ...string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	reviewerID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, $3)`, reviewerID, "suggestion-"+label+"-"+reviewerID.String()+"@dawha.test", label); err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, $2)`, reviewerID, role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'review', $3)`, fixture.treeID, reviewerID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`DELETE FROM claim_evidence WHERE created_by = $1`, []any{reviewerID}},
			{`DELETE FROM claim_counter_evidence WHERE created_by = $1`, []any{reviewerID}},
			{`DELETE FROM claim_versions WHERE created_by = $1`, []any{reviewerID}},
			{`DELETE FROM claims WHERE created_by = $1`, []any{reviewerID}},
			{`DELETE FROM entity_relationships WHERE created_by = $1`, []any{reviewerID}},
			{`DELETE FROM open_questions WHERE created_by = $1`, []any{reviewerID}},
			{`DELETE FROM suggestion_change_sets WHERE applied_by = $1`, []any{reviewerID}},
			{`DELETE FROM suggestion_reviews WHERE reviewer_id = $1`, []any{reviewerID}},
			{`DELETE FROM tree_collaborators WHERE user_id = $1`, []any{reviewerID}},
			{`DELETE FROM audit_log WHERE actor_id = $1`, []any{reviewerID}},
			{`DELETE FROM users WHERE id = $1`, []any{reviewerID}},
		} {
			if _, err := pool.Exec(cleanupCtx, statement.query, statement.args...); err != nil {
				t.Errorf("cleanup failed for %q: %v", statement.query, err)
			}
		}
	})
	return reviewerID
}

// The single change-set validation point runs on the real review path, before the
// transaction opens, so a malformed change set never reaches an authorization or a write.
func TestMalformedChangeSetIsRefusedBeforeAnyWrite(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	suggestion := submitSuggestion(t, pool, fixture, "مقترح مشوّه")

	if _, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: "tree", Relationship: &RelationshipChange{SubjectID: fixture.personID.String(), ObjectID: fixture.otherPersonID.String(), Predicate: "parent_of"}},
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a malformed change set = %v, want %v", err, ErrValidation)
	}
	if suggestionStatus(t, pool, suggestion.ID) != "pending" {
		t.Fatal("a malformed change set moved the suggestion out of the queue")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id = $1`, suggestion.ID) != 0 {
		t.Fatal("a malformed change set was stored")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_reviews WHERE suggestion_id = $1`, suggestion.ID) != 0 {
		t.Fatal("a malformed change set recorded a review")
	}
	if countRows(t, pool, `SELECT count(*) FROM entity_relationships WHERE created_by = $1`, fixture.ownerID) != 0 {
		t.Fatal("a malformed change set still wrote a relationship")
	}
}

// A tree review right is not a global write right. A plain tree collaborator may decide a
// suggestion, but applying a change set writes to the shared research tables, so it takes
// the same global role the evidence service already requires.
func TestChangeSetNeedsAGlobalWriteRole(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	collaboratorID := addTreeReviewer(t, pool, fixture, "collaborator")

	cases := []struct {
		name  string
		alias string
		apply ReviewInput
	}{
		{name: "person", alias: "لقب بلا صلاحية", apply: ReviewInput{Decision: "accepted", ChangeSet: &ChangeSet{Target: ChangeTargetPerson, Person: &PersonChange{PersonID: fixture.personID.String(), NameAR: "لقب بلا صلاحية"}}}},
		{name: "relationship", apply: ReviewInput{Decision: "accepted", ChangeSet: &ChangeSet{Target: ChangeTargetRelationship, Relationship: &RelationshipChange{SubjectID: fixture.personID.String(), ObjectID: fixture.otherPersonID.String(), Predicate: "sibling_of"}}}},
		{name: "claim", apply: ReviewInput{Decision: "accepted", ChangeSet: &ChangeSet{Target: ChangeTargetClaim, Claim: &ClaimChange{SubjectID: fixture.personID.String(), ObjectID: fixture.otherPersonID.String(), Predicate: "father_of", NoteAR: "ادعاء بلا صلاحية"}}}},
		{name: "source link", apply: ReviewInput{Decision: "accepted", ChangeSet: &ChangeSet{Target: ChangeTargetSourceLink, SourceLink: &SourceLinkChange{ClaimID: fixture.claimID.String(), SourceStatementID: fixture.statementID.String(), Relation: "supports"}}}},
	}
	for _, testCase := range cases {
		suggestion := submitSuggestion(t, pool, fixture, "مقترح "+testCase.name)
		// Every target is checked, so one broken target cannot hide the other three.
		if _, err := service.Review(ctx, suggestion.ID, collaboratorID.String(), testCase.apply); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s: a plain tree collaborator applied a change set: %v", testCase.name, err)
		}
		if suggestionStatus(t, pool, suggestion.ID) != "pending" {
			t.Errorf("%s: the suggestion left the queue after a refused change set", testCase.name)
		}
		if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id = $1`, suggestion.ID) != 0 {
			t.Errorf("%s: a refused change set was stored", testCase.name)
		}
		if countRows(t, pool, `SELECT count(*) FROM suggestion_reviews WHERE suggestion_id = $1`, suggestion.ID) != 0 {
			t.Errorf("%s: a refused change set still recorded a review", testCase.name)
		}
	}
	if countRows(t, pool, `SELECT count(*) FROM person_aliases WHERE value_ar = 'لقب بلا صلاحية'`) != 0 {
		t.Fatal("a refused change set still wrote an alias")
	}
	if countRows(t, pool, `SELECT count(*) FROM entity_relationships WHERE created_by = $1`, collaboratorID) != 0 {
		t.Fatal("a refused change set still wrote a relationship")
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE created_by = $1`, collaboratorID) != 0 {
		t.Fatal("a refused change set still wrote a claim")
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_evidence WHERE created_by = $1`, collaboratorID) != 0 {
		t.Fatal("a refused change set still wrote evidence")
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_counter_evidence WHERE created_by = $1`, collaboratorID) != 0 {
		t.Fatal("a refused change set still wrote counter evidence")
	}
}

// The gate is on applying a change, not on reviewing. A plain tree collaborator keeps the
// whole review surface: the open-question artifact, a rejection, and a conversion.
func TestPlainCollaboratorKeepsTheReviewSurfaceWithoutAChangeSet(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	collaboratorID := addTreeReviewer(t, pool, fixture, "collaborator-queue")

	accepted := submitSuggestion(t, pool, fixture, "مقترح بلا تغيير")
	acceptedView, err := service.Review(ctx, accepted.ID, collaboratorID.String(), ReviewInput{Decision: "accepted", NoteAR: "مقبول"})
	if err != nil {
		t.Fatal(err)
	}
	if acceptedView.Status != "accepted" || acceptedView.QuestionID == "" {
		t.Fatalf("the collaborator lost the open-question artifact: %+v", acceptedView)
	}
	if countRows(t, pool, `SELECT count(*) FROM open_questions WHERE id = $1 AND description_ar = 'مقترح بلا تغيير'`, acceptedView.QuestionID) != 1 {
		t.Fatal("the open question did not keep the proposer's text")
	}

	rejected := submitSuggestion(t, pool, fixture, "مقترح مرفوض")
	rejectedView, err := service.Review(ctx, rejected.ID, collaboratorID.String(), ReviewInput{Decision: "rejected", NoteAR: "لا مصدر"})
	if err != nil {
		t.Fatal(err)
	}
	if rejectedView.Status != "rejected" {
		t.Fatalf("the collaborator lost the rejection: %+v", rejectedView)
	}

	converted := submitSuggestion(t, pool, fixture, "مقترح يتحول")
	convertedView, err := service.Review(ctx, converted.ID, collaboratorID.String(), ReviewInput{Decision: "converted", QuestionTitleAR: "سؤال من متعاون"})
	if err != nil {
		t.Fatal(err)
	}
	if convertedView.Status != "converted" || convertedView.QuestionID == "" {
		t.Fatalf("the collaborator lost the conversion: %+v", convertedView)
	}
	if countRows(t, pool, `SELECT count(*) FROM open_questions WHERE id = $1 AND title_ar = 'سؤال من متعاون'`, convertedView.QuestionID) != 1 {
		t.Fatal("the converted question was not created")
	}
	if countRows(t, pool, `SELECT count(*) FROM claims WHERE created_by = $1`, collaboratorID) != 0 {
		t.Fatal("a review without a change set created a claim")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id::text = ANY($1)`, []string{accepted.ID, rejected.ID, converted.ID}) != 0 {
		t.Fatal("a review without a change set recorded a change set")
	}
}

// A reviewer who holds the global researcher role is not over-restricted: the change set
// applies exactly as it did before the gate existed.
func TestGlobalWriteRoleStillAppliesAChangeSet(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	researcherID := addTreeReviewer(t, pool, fixture, "researcher-reviewer", "researcher")

	suggestion := submitSuggestion(t, pool, fixture, "لقب المحقق")
	reviewed, err := service.Review(ctx, suggestion.ID, researcherID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetPerson, Person: &PersonChange{PersonID: fixture.personID.String(), NameAR: "لقب المحقق"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != "accepted" {
		t.Fatalf("unexpected reviewed suggestion: %+v", reviewed)
	}
	if countRows(t, pool, `SELECT count(*) FROM person_aliases WHERE person_id = $1 AND value_ar = 'لقب المحقق'`, fixture.personID) != 1 {
		t.Fatal("a researcher review did not apply the change set")
	}
	if countRows(t, pool, `SELECT count(*) FROM suggestion_change_sets WHERE suggestion_id = $1 AND applied_by = $2`, suggestion.ID, researcherID) != 1 {
		t.Fatal("the applied change set was not recorded against the researcher")
	}
}

// The source_link target obeys the same claim rule evidence.AddEvidence uses, checked
// before it looks at the claim, so an unmanageable claim and a missing one answer alike.
func TestSourceLinkChangeNeedsClaimManageRights(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	collaboratorID := addTreeReviewer(t, pool, fixture, "collaborator-link")
	researcherID := addTreeReviewer(t, pool, fixture, "researcher-link", "researcher")

	plan, err := validateChangeSet(ChangeSet{Target: ChangeTargetSourceLink, SourceLink: &SourceLinkChange{
		ClaimID: fixture.claimID.String(), SourceStatementID: fixture.statementID.String(), Relation: "supports",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applySourceLinkChange(ctx, pool, *plan.sourceLink, collaboratorID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a reviewer without claim rights linked evidence: %v", err)
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_evidence WHERE claim_id = $1`, fixture.claimID) != 0 {
		t.Fatal("a refused evidence link was written")
	}
	// A claim that does not exist answers the same way, so the check cannot be used to
	// probe which claims exist.
	if _, err := applySourceLinkChange(ctx, pool, *plan.sourceLink, collaboratorID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a missing claim answered differently: %v", err)
	}
	missingPlan, err := validateChangeSet(ChangeSet{Target: ChangeTargetSourceLink, SourceLink: &SourceLinkChange{
		ClaimID: uuid.New().String(), SourceStatementID: fixture.statementID.String(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applySourceLinkChange(ctx, pool, *missingPlan.sourceLink, collaboratorID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a missing claim was not refused: %v", err)
	}
	// A global research role manages any claim, exactly as in the evidence service.
	if _, err := applySourceLinkChange(ctx, pool, *plan.sourceLink, researcherID); err != nil {
		t.Fatalf("a researcher could not link evidence to an existing claim: %v", err)
	}
	if countRows(t, pool, `SELECT count(*) FROM claim_evidence WHERE claim_id = $1 AND created_by = $2`, fixture.claimID, researcherID) != 1 {
		t.Fatal("the researcher's evidence link was not written")
	}
}

// An alias applied through a change set carries no source, so the visibility policy treats
// it as a platform record and renders it on a public person for an anonymous reader. That
// coupling is intended, and this test pins it.
func TestAliasFromAChangeSetIsPublicOnAPublicPerson(t *testing.T) {
	pool := openSuggestionPool(t)
	fixture := seedSuggestionFixture(t, pool)
	ctx := context.Background()
	service := NewService(pool)
	suggestion := submitSuggestion(t, pool, fixture, "لقب عام")

	if _, err := service.Review(ctx, suggestion.ID, fixture.ownerID.String(), ReviewInput{
		Decision:  "accepted",
		ChangeSet: &ChangeSet{Target: ChangeTargetPerson, Person: &PersonChange{PersonID: fixture.personID.String(), NameAR: "لقب-platform", AliasType: "kunyah"}},
	}); err != nil {
		t.Fatal(err)
	}
	// The person reaches this page through the published public tree, and the alias has
	// no source, so plan 001 keeps it for an anonymous reader.
	detail, err := dictionary.NewService(pool).Get(ctx, "people", fixture.personID.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, alias := range detail.Aliases {
		if alias.ValueAR == "لقب-platform" {
			found = true
			if alias.Type != "kunyah" {
				t.Fatalf("the alias lost its type: %+v", alias)
			}
		}
	}
	if !found {
		t.Fatalf("an anonymous reader cannot see the accepted alias: %+v", detail.Aliases)
	}
	var sourceID *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT source_id FROM person_aliases WHERE person_id = $1 AND value_ar = 'لقب-platform'`, fixture.personID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if sourceID != nil {
		t.Fatal("the alias is not a platform record, so its visibility would depend on a source")
	}
}
