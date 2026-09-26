package visibility

import (
	"context"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The batched accessors exist so a list endpoint can authorize a page of records
// without one statement per record. Batching is only allowed to change how many
// statements a decision costs, never the decision: these tests resolve the same
// records one id at a time and many ids at once, for every policy the matrix above
// defines, and require the answers to be identical - including the difference
// between a record that is missing and a record that is hidden, which is what the
// 404-not-403 contract is made of.

func TestBatchedTreeAccessMatchesSingleIdAccess(t *testing.T) {
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
	fixture := seedPolicyFixture(t, ctx, pool)

	policies := []struct {
		name   string
		policy Policy
	}{
		{"anonymous", Anonymous()},
		{"owner", mustPolicy(t, fixture.ownerID.String(), false)},
		{"collaborator", mustPolicy(t, fixture.collaboratorID.String(), false)},
		{"researcher", mustPolicy(t, fixture.researcherID.String(), true)},
		{"unrelated researcher", mustPolicy(t, fixture.unrelatedResearcherID.String(), true)},
		{"admin", mustPolicy(t, fixture.adminID.String(), true)},
	}
	// A published tree, a public tree with no published version, and a tree that
	// does not exist: public, closed and missing, which are three different answers.
	publishedTreeID := seedBatchTree(t, ctx, pool, "public", "published", fixture.ownerID)
	draftTreeID := seedBatchTree(t, ctx, pool, "public", "draft", fixture.ownerID)
	absentTreeID := uuid.New()

	sourceIDs := []uuid.UUID{fixture.publicSourceID, fixture.privateSourceID, uuid.New()}
	treeIDs := []uuid.UUID{publishedTreeID, draftTreeID, fixture.unpublishedTreeID, absentTreeID}

	for _, subject := range policies {
		batchedTrees, batchErr := subject.policy.Trees(ctx, pool, treeIDs)
		if batchErr != nil {
			t.Fatalf("%s batched trees: %v", subject.name, batchErr)
		}
		for _, treeID := range treeIDs {
			one, oneErr := subject.policy.Tree(ctx, pool, treeID)
			if oneErr != nil {
				t.Fatalf("%s tree %s: %v", subject.name, treeID, oneErr)
			}
			if batchedTrees[treeID] != one {
				t.Fatalf("%s tree %s: batched %s, single %s", subject.name, treeID, batchedTrees[treeID], one)
			}
		}
		batchedSources, sourceErr := subject.policy.Sources(ctx, pool, sourceIDs)
		if sourceErr != nil {
			t.Fatalf("%s batched sources: %v", subject.name, sourceErr)
		}
		for _, sourceID := range sourceIDs {
			one, oneErr := subject.policy.Source(ctx, pool, sourceID)
			if oneErr != nil {
				t.Fatalf("%s source %s: %v", subject.name, sourceID, oneErr)
			}
			if batchedSources[sourceID] != one {
				t.Fatalf("%s source %s: batched %s, single %s", subject.name, sourceID, batchedSources[sourceID], one)
			}
		}
	}

	// A missing record is missing in a batch, which is what keeps a batched caller
	// from answering 404 for a record it should answer 403 for, or the reverse.
	ownerPolicy := mustPolicy(t, fixture.ownerID.String(), false)
	trees, err := ownerPolicy.Trees(ctx, pool, treeIDs)
	if err != nil {
		t.Fatalf("batched trees: %v", err)
	}
	if trees[absentTreeID] != AccessMissing {
		t.Fatalf("a tree that does not exist = %s, want missing", trees[absentTreeID])
	}
	if trees[publishedTreeID] != AccessPublic {
		t.Fatalf("a published public tree = %s, want public", trees[publishedTreeID])
	}
	// The comparison above is only worth something if the batch actually contains
	// more than one verdict, so the fixture has to produce both a readable and an
	// unreadable tree. A test that only ever compared public to public would pass
	// with the rule replaced by TRUE.
	anonymousTrees, err := Anonymous().Trees(ctx, pool, treeIDs)
	if err != nil {
		t.Fatalf("batched trees for anonymous: %v", err)
	}
	readable, denied := 0, 0
	for _, treeID := range treeIDs {
		if anonymousTrees[treeID].Allowed() {
			readable++
		} else {
			denied++
		}
	}
	if readable == 0 || denied == 0 {
		t.Fatalf("the anonymous batch is not a mix of verdicts (%d readable, %d denied), so the comparison proves nothing", readable, denied)
	}
	// The nil uuid has no row, and resolveMany drops it rather than inventing an
	// answer for it.
	withNil, err := ownerPolicy.Trees(ctx, pool, []uuid.UUID{publishedTreeID, uuid.Nil})
	if err != nil {
		t.Fatalf("batched trees with a nil id: %v", err)
	}
	if _, answered := withNil[uuid.Nil]; answered {
		t.Fatal("a batch answered for the nil uuid")
	}
	if withNil[publishedTreeID] != AccessPublic {
		t.Fatalf("a batch carrying a nil id changed the answer for a real one: %s", withNil[publishedTreeID])
	}
	// An empty batch is not an error and costs no statement.
	empty, err := ownerPolicy.Trees(ctx, pool, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("an empty batch = %v / %v, want no answers and no error", empty, err)
	}
}

// TestBatchedAccessCostsOneStatement is the pin that keeps it batched. Six ids have
// to cost the same single statement as one, which is the whole point of the twin.
func TestBatchedAccessCostsOneStatement(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, counter, err := testsupport.CountingPool(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	fixture := seedPolicyFixture(t, ctx, pool)
	policy := mustPolicy(t, fixture.ownerID.String(), false)

	seeded := make([]uuid.UUID, 0, 6)
	seeded = append(seeded, fixture.publicSourceID, fixture.privateSourceID)
	for index := 0; index < 4; index++ {
		seeded = append(seeded, seedBatchSource(t, ctx, pool, fixture.ownerID))
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM sources WHERE id = ANY($1::uuid[])`, seeded[2:]); err != nil {
			t.Errorf("cleanup batch sources: %v", err)
		}
	})

	counter.Reset()
	if _, err := policy.Sources(ctx, pool, seeded); err != nil {
		t.Fatalf("batched sources: %v", err)
	}
	if count := counter.Count(); count != 1 {
		t.Fatalf("six sources cost %d statements, want 1:\n%s", count, counter.Report())
	}
}

// seedBatchTree adds a tree in one of the two states the tree policy reads: a
// public tree with a published version, or a public tree whose only version is a
// draft.
func seedBatchTree(t *testing.T, ctx context.Context, pool *pgxpool.Pool, visibility, state string, ownerID uuid.UUID) uuid.UUID {
	t.Helper()
	treeID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة دفعة', $2, $3)`, treeID, visibility, ownerID); err != nil {
		t.Fatal(err)
	}
	publishedBy := any(nil)
	if state == "published" {
		publishedBy = ownerID
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, $3, $4, CASE WHEN $4::uuid IS NULL THEN NULL ELSE now() END)`, versionID, treeID, state, publishedBy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM trees WHERE id = $1`, treeID); err != nil {
			t.Errorf("cleanup batch tree: %v", err)
		}
	})
	return treeID
}

func seedBatchSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID uuid.UUID) uuid.UUID {
	t.Helper()
	var sourceID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO sources (title_ar, source_type, visibility, created_by) VALUES ($1, 'book', 'private', $2) RETURNING id`, "مصدر دفعة", ownerID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	return sourceID
}
