package geospatialintelligence

import (
	"context"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

// loadSpatialEdges is the half of the pair-enumeration the cluster builder
// consumes, and it must answer through the same eligibility loadAssociations
// applies: the status list (an 'unresolved' row is not evidence two places
// agree on), a public independent source, and no live dependency. An edge
// whose endpoints cannot load is pure cost — buildClusters intersects it away
// after the database built it from the whole association table — and a query
// that ships it ships more than its callers can claim.
//
// TestSpatialEdgesCarryOnlyTheEligibleStatuses pins that with one place pair
// whose associations are the only variable: ineligible statuses produce zero
// edge rows, the loaded ones produce one, and a private source on either
// endpoint produces zero on both sides of the radius predicate.
func TestSpatialEdgesCarryOnlyTheEligibleStatuses(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	actorID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث اتجاهات')`,
		actorID, geospatialTestEmail(actorID)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testsupport.CleanupSyntheticActors(t, pool, actorID) })

	// Two places ~333 km apart in an empty part of the map, far from the
	// seeded peninsula, so the exact edge counts below read this test's own
	// rows and not the ambient seed's (Riyadh ↔ Al-Hasa is 300 km; its seeded
	// places must not enter the count).
	placeIDs := []uuid.UUID{uuid.New(), uuid.New()}
	coordinates := [][2]float64{{160.0, -10.0}, {163.0, -10.0}}
	for index, placeID := range placeIDs {
		if _, err := pool.Exec(ctx, `INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, created_by) VALUES ($1, 'موضع اتجاه', 'موضع اتجاه', 'city', ST_SetSRID(ST_MakePoint($2, $3), 4326), $4)`,
			placeID, coordinates[index][0], coordinates[index][1], actorID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM places WHERE id = ANY($1::uuid[])`, placeIDs) })
	sourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, 'مصدر اتجاه', 'book', 'independent', 'public', $2)`, sourceID, actorID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM sources WHERE id = $1`, sourceID) })

	privateSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, 'مصدر اتجاه خاص', 'book', 'independent', 'private', $2)`, privateSourceID, actorID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM sources WHERE id = $1`, privateSourceID) })

	dependentSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, 'مصدر اتجاه تابع', 'book', 'derived', 'public', $2)`, dependentSourceID, actorID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM sources WHERE id = $1`, dependentSourceID) })

	insertAssociation := func(status string, source any, place uuid.UUID) uuid.UUID {
		t.Helper()
		associationID := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, source_id, created_by)
			VALUES ($1, 'tribe', $2, $3, 'associated_with', $4, 'approximate', $5, $6)`,
			associationID, actorID, place, status, source, actorID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM geographic_associations WHERE id = $1`, associationID) })
		return associationID
	}

	countEdges := func(t *testing.T, radiusMeters float64) int {
		t.Helper()
		edges, err := loadSpatialEdges(ctx, pool, radiusMeters, 100)
		if err != nil {
			t.Fatalf("load spatial edges: %v", err)
		}
		return len(edges)
	}

	// A tribe entity is its own row and no person policy is in play, so the
	// source and status predicates are the whole eligibility question.
	tribeID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'قبيلة اتجاه', 'قبيلة اتجاه', $2)`, tribeID, actorID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM tribes WHERE id = $1`, tribeID) })

	insertAssociation("documented", sourceID, placeIDs[0]) // the first endpoint, always eligible
	secondAssociation := insertAssociation("unresolved", sourceID, placeIDs[1])
	setSecond := func(t *testing.T, statement string, argument any) {
		t.Helper()
		if _, err := pool.Exec(ctx, statement, argument, secondAssociation); err != nil {
			t.Fatal(err)
		}
	}

	// The second's status is squarely ineligible: an 'unresolved' row is not
	// evidence two places agree on.
	if count := countEdges(t, 400_000); count != 0 {
		t.Fatalf("an 'unresolved' endpoint produced %d edges, want 0 — the status list loadAssociations applies is what the edges answer to", count)
	}
	setSecond(t, "UPDATE geographic_associations SET status = $1 WHERE id = $2", "platform_inferred")
	if count := countEdges(t, 400_000); count != 1 {
		t.Fatalf("two eligible associations = %d edges, want the single pair", count)
	}

	// A private source on either endpoint drops the edge: loadAssociations
	// would not have loaded either association, so the edge must not exist.
	setSecond(t, "UPDATE geographic_associations SET source_id = $1 WHERE id = $2", privateSourceID)
	if count := countEdges(t, 400_000); count != 0 {
		t.Fatalf("a private-source endpoint produced %d edges, want 0", count)
	}

	// And a public source that is not 'independent' does not return the edge:
	// that gate is loadAssociations' too.
	setSecond(t, "UPDATE geographic_associations SET source_id = $1 WHERE id = $2", dependentSourceID)
	if count := countEdges(t, 400_000); count != 0 {
		t.Fatalf("a dependent-source endpoint produced %d edges, want 0", count)
	}
}
