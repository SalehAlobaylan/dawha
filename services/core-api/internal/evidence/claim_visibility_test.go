package evidence

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type claimVisibilityFixture struct {
	ownerID           uuid.UUID
	strangerID        uuid.UUID
	personID          uuid.UUID
	publicClaimID     uuid.UUID
	privateClaimID    uuid.UUID
	emptyClaimID      uuid.UUID
	publicSourceID    uuid.UUID
	privateSourceID   uuid.UUID
	publicStatementID uuid.UUID
}

func TestClaimReadsFollowTheVisibilityPolicy(t *testing.T) {
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
	fixture := seedClaimVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	anonymousList, err := service.ListClaims(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	anonymousIDs := claimSummaryIDs(anonymousList)
	if !anonymousIDs[fixture.publicClaimID] {
		t.Fatalf("public claim is missing from the anonymous list: %+v", anonymousList)
	}
	for _, hidden := range []uuid.UUID{fixture.privateClaimID, fixture.emptyClaimID} {
		if anonymousIDs[hidden] {
			t.Fatalf("claim %s leaked into the anonymous list: %+v", hidden, anonymousList)
		}
	}

	ownerList, err := service.ListClaims(ctx, fixture.ownerID.String())
	if err != nil {
		t.Fatal(err)
	}
	ownerIDs := claimSummaryIDs(ownerList)
	if !ownerIDs[fixture.publicClaimID] || !ownerIDs[fixture.privateClaimID] || !ownerIDs[fixture.emptyClaimID] {
		t.Fatalf("owner lost a claim: %+v", ownerList)
	}

	strangerList, err := service.ListClaims(ctx, fixture.strangerID.String())
	if err != nil {
		t.Fatal(err)
	}
	strangerIDs := claimSummaryIDs(strangerList)
	if !strangerIDs[fixture.publicClaimID] {
		t.Fatalf("stranger lost the public claim: %+v", strangerList)
	}
	if strangerIDs[fixture.privateClaimID] || strangerIDs[fixture.emptyClaimID] {
		t.Fatalf("stranger saw a research claim: %+v", strangerList)
	}

	// A direct id read answers exactly like a missing claim, so the endpoint cannot
	// be used to probe whether a private claim exists.
	for _, hidden := range []uuid.UUID{fixture.privateClaimID, fixture.emptyClaimID, uuid.New()} {
		if _, err := service.GetClaim(ctx, hidden.String(), ""); !errors.Is(err, ErrNotFound) {
			t.Fatalf("anonymous claim read = %v, want %v", err, ErrNotFound)
		}
		if _, err := service.GetClaim(ctx, hidden.String(), fixture.strangerID.String()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("stranger claim read = %v, want %v", err, ErrNotFound)
		}
	}
	if _, err := service.GetClaim(ctx, "not-a-uuid", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed claim id = %v, want %v", err, ErrNotFound)
	}

	publicView, err := service.GetClaim(ctx, fixture.publicClaimID.String(), "")
	if err != nil {
		t.Fatalf("anonymous public claim read failed: %v", err)
	}
	if len(publicView.Evidence) != 1 || publicView.Evidence[0].SourceID != fixture.publicSourceID.String() {
		t.Fatalf("unexpected public evidence: %+v", publicView.Evidence)
	}
	if publicView.Evidence[0].StatementTextAR == "" || publicView.Evidence[0].SourceTitleAR == "" {
		t.Fatalf("public evidence lost its provenance: %+v", publicView.Evidence[0])
	}

	ownerView, err := service.GetClaim(ctx, fixture.privateClaimID.String(), fixture.ownerID.String())
	if err != nil {
		t.Fatalf("owner could not read the private claim: %v", err)
	}
	if len(ownerView.Evidence) != 1 || ownerView.Evidence[0].SourceID != fixture.privateSourceID.String() {
		t.Fatalf("owner lost the private evidence: %+v", ownerView.Evidence)
	}
	if ownerView.Evidence[0].StatementTextAR == "" {
		t.Fatalf("private evidence lost its text for the owner: %+v", ownerView.Evidence[0])
	}
}

func TestAddEvidenceReturnsAScopedClaim(t *testing.T) {
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
	fixture := seedClaimVisibilityFixture(t, ctx, pool)
	service := NewService(pool)

	view, err := service.AddEvidence(ctx, fixture.publicClaimID.String(), fixture.ownerID.String(), AddEvidenceInput{SourceStatementID: fixture.publicStatementID.String(), Relation: "supports"})
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != fixture.publicClaimID.String() || len(view.Evidence) != 2 {
		t.Fatalf("unexpected claim after linking evidence: %+v", view)
	}
	if _, err := service.AddEvidence(ctx, fixture.publicClaimID.String(), fixture.strangerID.String(), AddEvidenceInput{SourceStatementID: fixture.publicStatementID.String(), Relation: "supports"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unrelated actor linked evidence to a claim: %v", err)
	}
}

func claimSummaryIDs(items []ClaimSummary) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if parsed, err := uuid.Parse(item.ID); err == nil {
			ids[parsed] = true
		}
	}
	return ids
}

func seedClaimVisibilityFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *claimVisibilityFixture {
	t.Helper()
	fixture := &claimVisibilityFixture{ownerID: uuid.New(), strangerID: uuid.New(), personID: uuid.New()}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $3, 'مالك ادعاء'), ($2, $4, 'غريب ادعاء')`, fixture.ownerID, fixture.strangerID, "claim-owner-"+fixture.ownerID.String()+"@dawha.test", "claim-stranger-"+fixture.strangerID.String()+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered'), ($2, 'registered')`, fixture.ownerID, fixture.strangerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'شخصية ادعاء', 'شخصية ادعاء', $2)`, fixture.personID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.publicSourceID = uuid.New()
	fixture.privateSourceID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر ادعاء عام', 'book', 'public', $3), ($2, 'مصدر ادعاء خاص', 'book', 'private', $3)`, fixture.publicSourceID, fixture.privateSourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	publicPassageID := uuid.New()
	privatePassageID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $3, 1, 'مقطع ادعاء عام', 'مقطع ادعاء عام'), ($2, $4, 1, 'مقطع ادعاء خاص', 'مقطع ادعاء خاص')`, publicPassageID, privatePassageID, fixture.publicSourceID, fixture.privateSourceID); err != nil {
		t.Fatal(err)
	}
	fixture.publicStatementID = uuid.New()
	privateStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $3, $5, 'عبارة ادعاء عامة', 'accepted', $7), ($2, $4, $6, 'عبارة ادعاء خاصة', 'accepted', $7)`, fixture.publicStatementID, privateStatementID, fixture.publicSourceID, fixture.privateSourceID, publicPassageID, privatePassageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.publicClaimID = uuid.New()
	fixture.privateClaimID = uuid.New()
	fixture.emptyClaimID = uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES
			($1, 'person', $4, 'father_of', 'person', $4, 'supported', $5),
			($2, 'person', $4, 'father_of', 'person', $4, 'disputed', $5),
			($3, 'person', $4, 'father_of', 'person', $4, 'unresolved', $5)`, fixture.publicClaimID, fixture.privateClaimID, fixture.emptyClaimID, fixture.personID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($4, $5, 'supports', $3)`, fixture.publicClaimID, fixture.publicStatementID, fixture.ownerID, fixture.privateClaimID, privateStatementID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupClaimVisibilityFixture(t, pool, fixture)
	})
	return fixture
}

func cleanupClaimVisibilityFixture(t *testing.T, pool *pgxpool.Pool, fixture *claimVisibilityFixture) {
	t.Helper()
	ctx := context.Background()
	actors := []uuid.UUID{fixture.ownerID, fixture.strangerID}
	for _, statement := range []string{
		`DELETE FROM claim_evidence WHERE claim_id IN (SELECT id FROM claims WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM claim_versions WHERE claim_id IN (SELECT id FROM claims WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM claims WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM source_statements WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM sources WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM people WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM audit_log WHERE actor_id = ANY($1::uuid[])`,
		`DELETE FROM users WHERE id = ANY($1::uuid[])`,
	} {
		if _, err := pool.Exec(ctx, statement, actors); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM source_passages WHERE normalized_text_ar LIKE 'مقطع ادعاء%'`); err != nil {
		t.Errorf("cleanup failed for source passages: %v", err)
	}
}
