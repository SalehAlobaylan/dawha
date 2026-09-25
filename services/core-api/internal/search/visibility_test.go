package search

import (
	"context"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSearchHidesPrivateDraftPeopleAndResearchClaims(t *testing.T) {
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
	fixture := seedSearchVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	anonymousNames, err := service.Search(ctx, Input{Query: "بحث", Kind: "names"})
	if err != nil {
		t.Fatal(err)
	}
	names := resultIDs(anonymousNames, "person")
	if !names[fixture.publishedPersonID] {
		t.Fatalf("published person is missing from anonymous names: %+v", anonymousNames.Groups)
	}
	if names[fixture.draftPersonID] {
		t.Fatalf("private draft person leaked into anonymous names: %+v", anonymousNames.Groups)
	}

	ownerNames, err := service.Search(ctx, Input{ActorID: fixture.ownerID.String(), Query: "بحث", Kind: "names"})
	if err != nil {
		t.Fatal(err)
	}
	if !resultIDs(ownerNames, "person")[fixture.draftPersonID] {
		t.Fatalf("owner cannot see the person they created: %+v", ownerNames.Groups)
	}

	anonymousClaims, err := service.Search(ctx, Input{Query: "بحث", Kind: "claims"})
	if err != nil {
		t.Fatal(err)
	}
	claims := resultIDs(anonymousClaims, "claim")
	if !claims[fixture.publicClaimID] {
		t.Fatalf("public claim is missing from anonymous search: %+v", anonymousClaims.Groups)
	}
	if claims[fixture.privateClaimID] || claims[fixture.unevidencedClaimID] {
		t.Fatalf("research claim leaked into anonymous search: %+v", anonymousClaims.Groups)
	}

	anonymousQuestions, err := service.Search(ctx, Input{Query: "سؤال", Kind: "questions"})
	if err != nil {
		t.Fatal(err)
	}
	questions := resultIDs(anonymousQuestions, "question")
	if !questions[fixture.publicQuestionID] {
		t.Fatalf("public question is missing from anonymous search: %+v", anonymousQuestions.Groups)
	}
	if questions[fixture.researchQuestionID] {
		t.Fatalf("research-only question leaked into anonymous search: %+v", anonymousQuestions.Groups)
	}

	researcherClaims, err := service.Search(ctx, Input{ActorID: fixture.researcherID.String(), Query: "بحث", Kind: "claims"})
	if err != nil {
		t.Fatal(err)
	}
	researcherResult := resultIDs(researcherClaims, "claim")
	if !researcherResult[fixture.privateClaimID] || !researcherResult[fixture.unevidencedClaimID] {
		t.Fatalf("researcher lost the research claims: %+v", researcherClaims.Groups)
	}
}

// TestClaimNameMatchCannotProbeAHiddenPerson covers the text match inside the claim
// search: matching a claim by the name of one of its people is enough to learn that
// the name exists and is attached to a claim, so the person policy gates the match.
func TestClaimNameMatchCannotProbeAHiddenPerson(t *testing.T) {
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
	fixture := seedSearchVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	hidden, err := service.Search(ctx, Input{Query: "شخص مسود البحث", Kind: "claims"})
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Total != 0 {
		t.Fatalf("a private draft person name pulled claims into an anonymous result: %+v", hidden.Groups)
	}
	if len(hidden.Groups) != 0 {
		t.Fatalf("an empty private match still produced a group: %+v", hidden.Groups)
	}

	published, err := service.Search(ctx, Input{Query: "شخص منشور البحث", Kind: "claims"})
	if err != nil {
		t.Fatal(err)
	}
	if !resultIDs(published, "claim")[fixture.publicClaimID] {
		t.Fatalf("the public person name no longer finds its public claim: %+v", published.Groups)
	}

	owner, err := service.Search(ctx, Input{ActorID: fixture.ownerID.String(), Query: "شخص مسود البحث", Kind: "claims"})
	if err != nil {
		t.Fatal(err)
	}
	ownerIDs := resultIDs(owner, "claim")
	if !ownerIDs[fixture.publicClaimID] {
		t.Fatalf("the owner lost the claim attached to the person they created: %+v", owner.Groups)
	}
}

func resultIDs(response Response, kind string) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool)
	for _, group := range response.Groups {
		for _, item := range group.Items {
			if kind != "" && item.Kind != kind {
				continue
			}
			if parsed, err := uuid.Parse(item.ID); err == nil {
				ids[parsed] = true
			}
		}
	}
	return ids
}

type searchVisibilityFixture struct {
	ownerID            uuid.UUID
	researcherID       uuid.UUID
	draftPersonID      uuid.UUID
	publishedPersonID  uuid.UUID
	publicClaimID      uuid.UUID
	privateClaimID     uuid.UUID
	unevidencedClaimID uuid.UUID
	publicQuestionID   uuid.UUID
	researchQuestionID uuid.UUID
}

func seedSearchVisibilityFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *searchVisibilityFixture {
	t.Helper()
	fixture := &searchVisibilityFixture{ownerID: uuid.New(), researcherID: uuid.New()}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $3, 'مالك البحث'), ($2, $4, 'باحث البحث')`, fixture.ownerID, fixture.researcherID, "search-owner-"+fixture.ownerID.String()+"@dawha.test", "search-researcher-"+fixture.researcherID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered'), ($2, 'researcher')`, fixture.ownerID, fixture.researcherID); err != nil {
		t.Fatal(err)
	}
	fixture.draftPersonID = uuid.New()
	fixture.publishedPersonID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'شخص مسود البحث', 'شخص مسود البحث', $3), ($2, 'شخص منشور البحث', 'شخص منشور البحث', $3)`, fixture.draftPersonID, fixture.publishedPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	treeID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة بحث خاصة', 'private', $2)`, treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`, versionID, treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخصية بحث مسودة', 1)`, uuid.New(), versionID, fixture.draftPersonID); err != nil {
		t.Fatal(err)
	}
	publicTreeID := uuid.New()
	publicVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة بحث عامة', 'public', $2)`, publicTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, publicVersionID, publicTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخصية منشورة', 1)`, uuid.New(), publicVersionID, fixture.publishedPersonID); err != nil {
		t.Fatal(err)
	}

	publicSourceID := uuid.New()
	privateSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر بحث عام', 'book', 'public', $3), ($2, 'مصدر بحث خاص', 'book', 'private', $3)`, publicSourceID, privateSourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	publicPassageID := uuid.New()
	privatePassageID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $3, 1, 'مقطع بحث عام', 'مقطع بحث عام'), ($2, $4, 1, 'مقطع بحث خاص', 'مقطع بحث خاص')`, publicPassageID, privatePassageID, publicSourceID, privateSourceID); err != nil {
		t.Fatal(err)
	}
	publicStatementID := uuid.New()
	privateStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $3, $5, 'عبارة بحث عامة', 'accepted', $7), ($2, $4, $6, 'عبارة بحث خاصة', 'accepted', $7)`, publicStatementID, privateStatementID, publicSourceID, privateSourceID, publicPassageID, privatePassageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}

	fixture.publicClaimID = uuid.New()
	fixture.privateClaimID = uuid.New()
	fixture.unevidencedClaimID = uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by) VALUES
			($1, 'person', $4, 'parent_of', 'person', $5, 'supported', 'ادعاء بحث عام', $6),
			($2, 'person', $4, 'parent_of', 'person', $5, 'disputed', 'ادعاء بحث خاص', $6),
			($3, 'person', $4, 'parent_of', 'person', $5, 'unresolved', 'ادعاء بحث بلا دليل', $6)`, fixture.publicClaimID, fixture.privateClaimID, fixture.unevidencedClaimID, fixture.publishedPersonID, fixture.draftPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($4, $5, 'supports', $3)`, fixture.publicClaimID, publicStatementID, fixture.ownerID, fixture.privateClaimID, privateStatementID); err != nil {
		t.Fatal(err)
	}

	fixture.publicQuestionID = uuid.New()
	fixture.researchQuestionID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, created_by) VALUES ($1, 'سؤال بحث عام', 'open', $3), ($2, 'سؤال بحث خاص', 'open', $3)`, fixture.publicQuestionID, fixture.researchQuestionID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO question_sources (question_id, source_id, role) VALUES ($1, $2, 'supporting')`, fixture.publicQuestionID, publicSourceID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupSearchVisibilityFixture(t, pool, fixture)
	})
	return fixture
}

func cleanupSearchVisibilityFixture(t *testing.T, pool *pgxpool.Pool, fixture *searchVisibilityFixture) {
	t.Helper()
	ctx := context.Background()
	actors := []uuid.UUID{fixture.ownerID, fixture.researcherID}
	for _, statement := range []string{
		`DELETE FROM question_sources WHERE question_id IN (SELECT id FROM open_questions WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM question_claims WHERE question_id IN (SELECT id FROM open_questions WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM open_questions WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM claim_evidence WHERE claim_id IN (SELECT id FROM claims WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM claims WHERE created_by = ANY($1::uuid[])`,
	} {
		if _, err := pool.Exec(ctx, statement, actors); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
	for _, statement := range []string{
		`DELETE FROM source_statements WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM sources WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM tree_nodes WHERE person_id IN (SELECT id FROM people WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM tree_versions WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM trees WHERE owner_id = ANY($1::uuid[])`,
		`DELETE FROM people WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM users WHERE id = ANY($1::uuid[])`,
	} {
		if _, err := pool.Exec(ctx, statement, actors); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
	// The statements that reference a leftover passage go first so an interrupted
	// earlier run cannot block the cleanup.
	if _, err := pool.Exec(ctx, `DELETE FROM source_statements WHERE source_passage_id IN (SELECT id FROM source_passages WHERE normalized_text_ar LIKE 'مقطع بحث%')`); err != nil {
		t.Errorf("cleanup failed for leftover statements: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM source_passages WHERE normalized_text_ar LIKE 'مقطع بحث%'`); err != nil {
		t.Errorf("cleanup failed for source passages: %v", err)
	}
}
