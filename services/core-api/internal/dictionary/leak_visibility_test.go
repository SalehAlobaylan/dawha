package dictionary

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// leakedFixture carries a public person whose page is reachable, plus the
// research-only rows that must not reach that page: a private-source association, a
// private-source alias and a private-evidence claim.
type leakedFixture struct {
	ownerID                uuid.UUID
	collaboratorID         uuid.UUID
	strangerID             uuid.UUID
	researcherID           uuid.UUID
	publicPersonID         uuid.UUID
	draftPersonID          uuid.UUID
	publicPlaceID          uuid.UUID
	privatePlaceID         uuid.UUID
	associationLessPlaceID uuid.UUID
	publicSourceID         uuid.UUID
	privateSourceID        uuid.UUID
	privateClaimID         uuid.UUID
	publicClaimID          uuid.UUID
}

func TestPersonPlacesAndAliasesSkipResearchOnlyRows(t *testing.T) {
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
	fixture := seedLeakedFixture(t, ctx, pool)
	service := NewService(pool)

	detail, err := service.Get(ctx, "people", fixture.publicPersonID.String(), "")
	if err != nil {
		t.Fatalf("anonymous published person read failed: %v", err)
	}
	places := referenceIDs(detail.Places)
	if !places[fixture.publicPlaceID] {
		t.Fatalf("the source-backed public place is missing: %+v", detail.Places)
	}
	if !places[fixture.associationLessPlaceID] {
		t.Fatalf("a platform association with no source must stay visible: %+v", detail.Places)
	}
	if places[fixture.privatePlaceID] {
		t.Fatalf("a place reached through a private source leaked on the public person page: %+v", detail.Places)
	}

	aliases := aliasValues(detail.Aliases)
	if !aliases["لقب_عام"] {
		t.Fatalf("the public alias is missing: %+v", detail.Aliases)
	}
	if !aliases["لقب بلا مصدر"] {
		t.Fatalf("a platform alias with no source must stay visible: %+v", detail.Aliases)
	}
	if aliases["لقب_من_مصدر_خاص"] {
		t.Fatalf("an alias taken from a private source leaked on the public person page: %+v", detail.Aliases)
	}

	ownerDetail, err := service.Get(ctx, "people", fixture.publicPersonID.String(), fixture.ownerID.String())
	if err != nil {
		t.Fatalf("owner read failed: %v", err)
	}
	ownerPlaces := referenceIDs(ownerDetail.Places)
	if !ownerPlaces[fixture.privatePlaceID] {
		t.Fatalf("the owner lost the place reached through their own private source: %+v", ownerDetail.Places)
	}
	if !aliasValues(ownerDetail.Aliases)["لقب_من_مصدر_خاص"] {
		t.Fatalf("the owner lost the alias taken from their own private source: %+v", ownerDetail.Aliases)
	}
	if !claimIDs(ownerDetail.Claims)[fixture.privateClaimID] {
		t.Fatalf("the owner lost the private-evidence claim: %+v", ownerDetail.Claims)
	}

	collaboratorDetail, err := service.Get(ctx, "people", fixture.publicPersonID.String(), fixture.collaboratorID.String())
	if err != nil {
		t.Fatalf("collaborator read failed: %v", err)
	}
	if referenceIDs(collaboratorDetail.Places)[fixture.privatePlaceID] {
		t.Fatalf("a collaborator saw a place backed by someone else's private source: %+v", collaboratorDetail.Places)
	}
	if aliasValues(collaboratorDetail.Aliases)["لقب_من_مصدر_خاص"] {
		t.Fatalf("a collaborator saw an alias taken from someone else's private source: %+v", collaboratorDetail.Aliases)
	}

	researcherDetail, err := service.Get(ctx, "people", fixture.publicPersonID.String(), fixture.researcherID.String())
	if err != nil {
		t.Fatalf("researcher read failed: %v", err)
	}
	if !referenceIDs(researcherDetail.Places)[fixture.privatePlaceID] {
		t.Fatalf("a researcher lost the research-only place: %+v", researcherDetail.Places)
	}
	if !aliasValues(researcherDetail.Aliases)["لقب_من_مصدر_خاص"] {
		t.Fatalf("a researcher lost the research-only alias: %+v", researcherDetail.Aliases)
	}

	strangerDetail, err := service.Get(ctx, "people", fixture.publicPersonID.String(), fixture.strangerID.String())
	if err != nil {
		t.Fatalf("unrelated actor read failed: %v", err)
	}
	if referenceIDs(strangerDetail.Places)[fixture.privatePlaceID] {
		t.Fatalf("an unrelated actor saw a research-only place: %+v", strangerDetail.Places)
	}

	// The people index exposes an alias as the secondary name, so the same rule
	// applies to the index and to its alias count.
	anonymous, err := service.ListIndex(ctx, "people", "شخصية منشورة اختبار القاموس", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range anonymous.Items {
		if item.ID != fixture.publicPersonID.String() {
			continue
		}
		if strings.Contains(item.SecondaryAR, "لقب_من_مصدر_خاص") {
			t.Fatalf("the anonymous people index leaked a private-source alias: %+v", item)
		}
		if item.Count != 2 {
			t.Fatalf("the anonymous alias count still counts the private-source alias: %+v", item)
		}
	}
	owner, err := service.ListIndex(ctx, "people", "شخصية منشورة اختبار القاموس", fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range owner.Items {
		if item.ID == fixture.publicPersonID.String() && !strings.Contains(item.SecondaryAR, "لقب") {
			t.Fatalf("the owner lost the alias secondary name: %+v", item)
		}
	}
}

func TestDisputedClaimIndexHidesPrivatePersonNames(t *testing.T) {
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
	fixture := seedLeakedFixture(t, ctx, pool)
	service := NewService(pool)

	anonymous, err := service.ListIndex(ctx, "disputed-claims", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range anonymous.Items {
		if strings.Contains(item.NameAR, "رجل القاموس السر") {
			t.Fatalf("the anonymous disputed-claims index leaked a private draft person name: %+v", item)
		}
	}

	// The row itself is a public claim, so it stays listed; only the hidden name is
	// replaced by the identifier the row already exposes.
	// The private-evidence claim stays hidden: the public person and the draft person
	// are its subject and object, and its only evidence is a private source.
	if indexIDs(anonymous)[fixture.privateClaimID] {
		t.Fatalf("a private-evidence claim leaked into the anonymous index: %+v", anonymous.Items)
	}
	// The public claim stays listed, and the published name is still readable on it.
	found := false
	for _, item := range anonymous.Items {
		if item.ID != fixture.publicClaimID.String() {
			continue
		}
		found = true
		if !strings.Contains(item.NameAR, "الشخصية المنشورة اختبار القاموس") {
			t.Fatalf("the public person name is missing from a public claim label: %+v", item)
		}
	}
	if !found {
		t.Fatalf("the public claim disappeared from the anonymous index: %+v", anonymous.Items)
	}

	byDraftName, err := service.ListIndex(ctx, "disputed-claims", "رجل القاموس السر", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(byDraftName.Items) != 0 {
		t.Fatalf("searching by a private draft person name returned claims: %+v", byDraftName.Items)
	}

	owner, err := service.ListIndex(ctx, "disputed-claims", "رجل القاموس السر", fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	ownerFound := false
	for _, item := range owner.Items {
		if strings.Contains(item.NameAR, "رجل القاموس السر") {
			ownerFound = true
		}
	}
	if !ownerFound {
		t.Fatalf("the owner lost the private person name: %+v", owner.Items)
	}
}

func aliasValues(items []AliasView) map[string]bool {
	values := make(map[string]bool, len(items))
	for _, item := range items {
		values[item.ValueAR] = true
	}
	return values
}

func seedLeakedFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *leakedFixture {
	t.Helper()
	fixture := &leakedFixture{
		ownerID:        uuid.New(),
		collaboratorID: uuid.New(),
		strangerID:     uuid.New(),
		researcherID:   uuid.New(),
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, display_name_ar) VALUES
			($1, $5, 'مالك التسريب'),
			($2, $6, 'مشارك التسريب'),
			($3, $7, 'غريب التسريب'),
			($4, $8, 'باحث التسريب')`, fixture.ownerID, fixture.collaboratorID, fixture.strangerID, fixture.researcherID,
		"leak-owner-"+fixture.ownerID.String()+"@dawha.test",
		"leak-collaborator-"+fixture.collaboratorID.String()+"@dawha.test",
		"leak-stranger-"+fixture.strangerID.String()+"@dawha.test",
		"leak-researcher-"+fixture.researcherID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered'), ($2, 'registered'), ($3, 'registered'), ($4, 'researcher')`, fixture.ownerID, fixture.collaboratorID, fixture.strangerID, fixture.researcherID); err != nil {
		t.Fatal(err)
	}
	fixture.publicPersonID = uuid.New()
	fixture.draftPersonID = uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES
			($1, 'الشخصية المنشورة اختبار القاموس', 'الشخصية المنشورة اختبار القاموس', $3),
			($2, 'رجل القاموس السر', 'رجل القاموس السر', $3)`, fixture.publicPersonID, fixture.draftPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	publicTreeID := uuid.New()
	publicVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة التسريب العامة', 'public', $2)`, publicTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, publicVersionID, publicTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'منشورة', 1)`, uuid.New(), publicVersionID, fixture.publicPersonID); err != nil {
		t.Fatal(err)
	}

	fixture.publicPlaceID = uuid.New()
	fixture.privatePlaceID = uuid.New()
	fixture.associationLessPlaceID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type) VALUES ($1, 'موضع التسريب العام', 'موضع التسريب العام', 'city'), ($2, 'موضع التسريب الخاص', 'موضع التسريب الخاص', 'city'), ($3, 'موضع التسريب بلا مصدر', 'موضع التسريب بلا مصدر', 'city')`, fixture.publicPlaceID, fixture.privatePlaceID, fixture.associationLessPlaceID); err != nil {
		t.Fatal(err)
	}
	fixture.publicSourceID = uuid.New()
	fixture.privateSourceID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر التسريب العام', 'book', 'public', $3), ($2, 'مصدر التسريب الخاص', 'book', 'private', $3)`, fixture.publicSourceID, fixture.privateSourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	// One association per visibility class is enough: the public one and the one with
	// no source must stay, the private-source one must not.
	if _, err := pool.Exec(ctx, `INSERT INTO geographic_associations (entity_type, entity_id, place_id, relation_type, source_id, created_by) VALUES ('person', $1, $2, 'documented_in', $4, $5), ('person', $1, $3, 'documented_in', $6, $5), ('person', $1, $7, 'documented_in', NULL, $5)`, fixture.publicPersonID, fixture.publicPlaceID, fixture.privatePlaceID, fixture.publicSourceID, fixture.ownerID, fixture.privateSourceID, fixture.associationLessPlaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO person_aliases (person_id, value_ar, normalized_value_ar, alias_type, source_id) VALUES ($1, 'لقب بلا مصدر', 'لقب بلا مصدر', 'kunyah', NULL), ($1, 'لقب_عام', 'لقب عام', 'kunyah', $3), ($1, 'لقب_من_مصدر_خاص', 'لقب من مصدر خاص', 'kunyah', $2)`, fixture.publicPersonID, fixture.privateSourceID, fixture.publicSourceID); err != nil {
		t.Fatal(err)
	}
	privatePassageID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, 'مقطع التسريب الخاص', 'مقطع التسريب الخاص')`, privatePassageID, fixture.privateSourceID); err != nil {
		t.Fatal(err)
	}
	privateStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, 'عبارة التسريب الخاصة', 'accepted', $4)`, privateStatementID, fixture.privateSourceID, privatePassageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	// A public-evidence claim about the published person, and a private-evidence claim
	// naming the draft person, so the disputed-claims index has one row whose subject
	// name must be replaced by the identifier.
	publicStatementID := uuid.New()
	publicPassageID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, 'مقطع التسريب العام', 'مقطع التسريب العام')`, publicPassageID, fixture.publicSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, 'عبارة التسريب العامة', 'accepted', $4)`, publicStatementID, fixture.publicSourceID, publicPassageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.privateClaimID = uuid.New()
	fixture.publicClaimID = uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES
			($1, 'person', $2, 'father_of', 'person', $3, 'disputed', $4),
			($5, 'person', $6, 'father_of', 'person', $3, 'disputed', $4)`, fixture.privateClaimID, fixture.draftPersonID, fixture.publicPersonID, fixture.ownerID, fixture.publicClaimID, fixture.publicPersonID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($4, $5, 'supports', $3)`, fixture.privateClaimID, privateStatementID, fixture.ownerID, fixture.publicClaimID, publicStatementID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupLeakedFixture(t, pool, fixture)
	})
	return fixture
}

func cleanupLeakedFixture(t *testing.T, pool *pgxpool.Pool, fixture *leakedFixture) {
	t.Helper()
	ctx := context.Background()
	actors := []uuid.UUID{fixture.ownerID, fixture.collaboratorID, fixture.strangerID, fixture.researcherID}
	for _, statement := range []string{
		`DELETE FROM claim_evidence WHERE claim_id IN (SELECT id FROM claims WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM claims WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM person_aliases WHERE person_id IN (SELECT id FROM people WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM geographic_associations WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM source_statements WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM sources WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM tree_nodes WHERE person_id IN (SELECT id FROM people WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM tree_versions WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM trees WHERE owner_id = ANY($1::uuid[])`,
		`DELETE FROM people WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM audit_log WHERE actor_id = ANY($1::uuid[])`,
		`DELETE FROM user_roles WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM users WHERE id = ANY($1::uuid[])`,
	} {
		if _, err := pool.Exec(ctx, statement, actors); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
	for _, statement := range []string{
		`DELETE FROM source_passages WHERE normalized_text_ar LIKE 'مقطع التسريب%'`,
		`DELETE FROM places WHERE canonical_name_ar LIKE 'موضع التسريب%'`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
}
