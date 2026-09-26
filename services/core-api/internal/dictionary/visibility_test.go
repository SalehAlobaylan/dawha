package dictionary

import (
	"context"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type visibilityFixture struct {
	ownerID            uuid.UUID
	collaboratorID     uuid.UUID
	draftPersonID      uuid.UUID
	publishedPersonID  uuid.UUID
	publicSourceID     uuid.UUID
	privateSourceID    uuid.UUID
	publicClaimID      uuid.UUID
	privateClaimID     uuid.UUID
	publicQuestionID   uuid.UUID
	researchQuestionID uuid.UUID
	placeID            uuid.UUID

	unevidencedClaimID uuid.UUID
}

func TestDictionaryHidesPrivateDraftPeopleAndKeepsPublishedOnes(t *testing.T) {
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
	fixture := seedVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	anonymous, err := service.ListIndex(ctx, "people", "اختبار", "")
	if err != nil {
		t.Fatal(err)
	}
	ids := indexIDs(anonymous)
	if !ids[fixture.publishedPersonID] {
		t.Fatalf("published person is missing from the anonymous index: %+v", anonymous.Items)
	}
	if ids[fixture.draftPersonID] {
		t.Fatalf("private draft person leaked into the anonymous index: %+v", anonymous.Items)
	}

	ownerView, err := service.ListIndex(ctx, "people", "اختبار", fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	if !indexIDs(ownerView)[fixture.draftPersonID] {
		t.Fatalf("the owner cannot see the person they created: %+v", ownerView.Items)
	}

	collaboratorView, err := service.ListIndex(ctx, "people", "اختبار", fixture.collaboratorID.String())
	if err != nil {
		t.Fatal(err)
	}
	if !indexIDs(collaboratorView)[fixture.draftPersonID] {
		t.Fatalf("a tree collaborator cannot see the draft person: %+v", collaboratorView.Items)
	}

	strangerView, err := service.ListIndex(ctx, "people", "اختبار", uuid.New().String())
	if err != nil {
		t.Fatal(err)
	}
	if indexIDs(strangerView)[fixture.draftPersonID] {
		t.Fatalf("an unrelated actor saw the draft person: %+v", strangerView.Items)
	}
}

func TestDictionaryDetailRejectsHiddenPersonById(t *testing.T) {
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
	fixture := seedVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	if _, err := service.Get(ctx, "people", fixture.draftPersonID.String(), ""); err != ErrNotFound {
		t.Fatalf("anonymous draft person read = %v, want %v", err, ErrNotFound)
	}
	if _, err := service.Get(ctx, "people", uuid.New().String(), ""); err != ErrNotFound {
		t.Fatalf("missing person read = %v, want %v", err, ErrNotFound)
	}
	ownerDetail, err := service.Get(ctx, "people", fixture.draftPersonID.String(), fixture.ownerID.String())
	if err != nil {
		t.Fatalf("owner could not read the draft person: %v", err)
	}
	if ownerDetail.NameAR != "شخصية اختبار القاموس" {
		t.Fatalf("unexpected owner detail: %+v", ownerDetail)
	}
	published, err := service.Get(ctx, "people", fixture.publishedPersonID.String(), "")
	if err != nil {
		t.Fatalf("anonymous published person read failed: %v", err)
	}
	if len(published.PublishedTrees) != 1 {
		t.Fatalf("published tree reference is missing: %+v", published.PublishedTrees)
	}
}

func TestDictionaryDetailScopesClaimsQuestionsAndPlacePeople(t *testing.T) {
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
	fixture := seedVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	detail, err := service.Get(ctx, "people", fixture.publishedPersonID.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	claims := claimIDs(detail.Claims)
	if !claims[fixture.publicClaimID] {
		t.Fatalf("public claim is missing from the person page: %+v", detail.Claims)
	}
	if claims[fixture.privateClaimID] {
		t.Fatalf("private evidence claim leaked on the public person page: %+v", detail.Claims)
	}
	questions := questionIDs(detail.Questions)
	if !questions[fixture.publicQuestionID] || questions[fixture.researchQuestionID] {
		t.Fatalf("unexpected anonymous question references: %+v", detail.Questions)
	}
	sources := referenceIDs(detail.Sources)
	if !sources[fixture.publicSourceID] || sources[fixture.privateSourceID] {
		t.Fatalf("unexpected anonymous source references: %+v", detail.Sources)
	}

	place, err := service.Get(ctx, "places", fixture.placeID.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	people := referenceIDs(place.People)
	if !people[fixture.publishedPersonID] {
		t.Fatalf("published person is missing from the place page: %+v", place.People)
	}
	if people[fixture.draftPersonID] {
		t.Fatalf("draft person leaked on the public place page: %+v", place.People)
	}
	if claims := claimIDs(place.Claims); claims[fixture.privateClaimID] {
		t.Fatalf("private claim leaked on the public place page: %+v", place.Claims)
	}
}

func TestDictionaryDisputedClaimIndexIsScoped(t *testing.T) {
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
	fixture := seedVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	anonymous, err := service.ListIndex(ctx, "disputed-claims", "اختبار", "")
	if err != nil {
		t.Fatal(err)
	}
	ids := indexIDs(anonymous)
	if ids[fixture.privateClaimID] {
		t.Fatalf("a claim resting on a private source leaked into the public index: %+v", anonymous.Items)
	}
	if ids[fixture.unevidencedClaimID] {
		t.Fatalf("an unevidenced claim leaked into the public index: %+v", anonymous.Items)
	}
	owner, err := service.ListIndex(ctx, "disputed-claims", "اختبار", fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	ownerIDs := indexIDs(owner)
	if !ownerIDs[fixture.privateClaimID] || !ownerIDs[fixture.unevidencedClaimID] {
		t.Fatalf("owner lost the research claims: %+v", owner.Items)
	}
}

func indexIDs(response IndexResponse) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool, len(response.Items))
	for _, item := range response.Items {
		if parsed, err := uuid.Parse(item.ID); err == nil {
			ids[parsed] = true
		}
	}
	return ids
}

func claimIDs(items []ClaimReference) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if parsed, err := uuid.Parse(item.ID); err == nil {
			ids[parsed] = true
		}
	}
	return ids
}

func questionIDs(items []QuestionReference) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if parsed, err := uuid.Parse(item.ID); err == nil {
			ids[parsed] = true
		}
	}
	return ids
}

func referenceIDs(items []ReferenceView) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if parsed, err := uuid.Parse(item.ID); err == nil {
			ids[parsed] = true
		}
	}
	return ids
}

func seedVisibilityFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *visibilityFixture {
	t.Helper()
	fixture := &visibilityFixture{ownerID: uuid.New(), collaboratorID: uuid.New()}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $3, 'مالك القاموس'), ($2, $4, 'مشارك القاموس')`, fixture.ownerID, fixture.collaboratorID, "dictionary-owner-"+fixture.ownerID.String()+"@dawha.test", "dictionary-collaborator-"+fixture.collaboratorID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered'), ($2, 'registered')`, fixture.ownerID, fixture.collaboratorID); err != nil {
		t.Fatal(err)
	}
	fixture.draftPersonID = uuid.New()
	fixture.publishedPersonID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'شخصية اختبار القاموس', 'شخصية اختبار القاموس', $3), ($2, 'شخصية اختبار القاموس المنشورة', 'شخصية اختبار القاموس المنشورة', $3)`, fixture.draftPersonID, fixture.publishedPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	treeID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة اختبار القاموس', 'private', $2)`, treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`, versionID, treeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'view', $3)`, treeID, fixture.collaboratorID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخصية اختبار القاموس', 1)`, uuid.New(), versionID, fixture.draftPersonID); err != nil {
		t.Fatal(err)
	}
	publicTreeID := uuid.New()
	publicVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة اختبار القاموس العامة', 'public', $2)`, publicTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, publicVersionID, publicTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخصية منشورة', 1)`, uuid.New(), publicVersionID, fixture.publishedPersonID); err != nil {
		t.Fatal(err)
	}

	fixture.placeID = uuid.New()
	// Published explicitly: the column default is research-only since
	// db/migrations/0039_reference_visibility.sql and this fixture reads a public
	// place page.
	if _, err := pool.Exec(ctx, `INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility) VALUES ($1, 'موضع اختبار القاموس', 'موضع اختبار القاموس', 'city', 'public')`, fixture.placeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO geographic_associations (entity_type, entity_id, place_id, relation_type, created_by) VALUES ('person', $1, $2, 'documented_in', $3), ('person', $4, $2, 'documented_in', $3)`, fixture.draftPersonID, fixture.placeID, fixture.ownerID, fixture.publishedPersonID); err != nil {
		t.Fatal(err)
	}

	fixture.publicSourceID = uuid.New()
	fixture.privateSourceID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر اختبار القاموس العام', 'book', 'public', $3), ($2, 'مصدر اختبار القاموس الخاص', 'book', 'private', $3)`, fixture.publicSourceID, fixture.privateSourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	publicPassageID := uuid.New()
	privatePassageID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $3, 1, 'مقطع القاموس العام', 'مقطع القاموس العام'), ($2, $4, 1, 'مقطع القاموس الخاص', 'مقطع القاموس الخاص')`, publicPassageID, privatePassageID, fixture.publicSourceID, fixture.privateSourceID); err != nil {
		t.Fatal(err)
	}
	publicStatementID := uuid.New()
	privateStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $3, $5, 'عبارة القاموس العامة', 'accepted', $7), ($2, $4, $6, 'عبارة القاموس الخاصة', 'accepted', $7)`, publicStatementID, privateStatementID, fixture.publicSourceID, fixture.privateSourceID, publicPassageID, privatePassageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}

	fixture.publicClaimID = uuid.New()
	fixture.privateClaimID = uuid.New()
	fixture.unevidencedClaimID = uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, place_id, status, created_by) VALUES
			($1, 'person', $4, 'father_of', 'person', $5, $6, 'supported', $7),
			($2, 'person', $4, 'father_of', 'person', $5, $6, 'disputed', $7),
			($3, 'person', $4, 'father_of', 'person', $5, $6, 'contested', $7)`, fixture.publicClaimID, fixture.privateClaimID, fixture.unevidencedClaimID, fixture.publishedPersonID, fixture.draftPersonID, fixture.placeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($4, $5, 'supports', $3)`, fixture.publicClaimID, publicStatementID, fixture.ownerID, fixture.privateClaimID, privateStatementID); err != nil {
		t.Fatal(err)
	}

	fixture.publicQuestionID = uuid.New()
	fixture.researchQuestionID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, created_by) VALUES ($1, 'سؤال اختبار القاموس العام', 'open', $3), ($2, 'سؤال اختبار القاموس البحثي', 'open', $3)`, fixture.publicQuestionID, fixture.researchQuestionID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO question_sources (question_id, source_id, role) VALUES ($1, $2, 'supporting')`, fixture.publicQuestionID, fixture.publicSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO question_claims (question_id, claim_id, role) VALUES ($1, $2, 'concerns')`, fixture.publicQuestionID, fixture.publicClaimID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupVisibilityFixture(t, pool, fixture)
	})
	return fixture
}

func cleanupVisibilityFixture(t *testing.T, pool *pgxpool.Pool, fixture *visibilityFixture) {
	t.Helper()
	ctx := context.Background()
	actors := []uuid.UUID{fixture.ownerID, fixture.collaboratorID}
	statements := []string{
		`DELETE FROM question_sources WHERE question_id IN (SELECT id FROM open_questions WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM question_claims WHERE question_id IN (SELECT id FROM open_questions WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM open_questions WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM claim_evidence WHERE claim_id IN (SELECT id FROM claims WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM claims WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM source_statements WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM sources WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM geographic_associations WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM tree_nodes WHERE person_id IN (SELECT id FROM people WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM tree_versions WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM trees WHERE owner_id = ANY($1::uuid[])`,
		`DELETE FROM people WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM users WHERE id = ANY($1::uuid[])`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, actors); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
	for _, statement := range []string{
		`DELETE FROM source_passages WHERE normalized_text_ar LIKE 'مقطع القاموس%'`,
		`DELETE FROM places WHERE canonical_name_ar LIKE 'موضع اختبار القاموس%'`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
}
