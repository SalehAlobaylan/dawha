package geography

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The three tests in this file are plan 029's parity proofs: a feature layer
// may not draw one association twice because its evidence is 1:N, may not
// drop a migration whose route merely crosses the viewport, and may not draw
// an undated region as if the era window proved its presence.

// TestMapKeepsOneEvidenceFeaturePerAssociation pins the LATERAL best-evidence
// join: spatial_evidence is 1:N by schema (0006 only CHECKs that one parent FK
// holds), so the raw join used to multiply a documented association into one
// feature per evidence row — an inflated marker count for one real row. One
// feature per association, and its shown evidence is the accepted row when
// there is one, the ranking internal/geospatialintelligence already defined.
func TestMapKeepsOneEvidenceFeaturePerAssociation(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 2, 2, 0, 0)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)

	associationID := seeded.associationID[0] // status documented, public tree subject
	acceptedID := uuid.New()
	pendingID := uuid.New()
	fixture.Exec(`INSERT INTO spatial_evidence (id, geographic_association_id, evidence_text_ar, review_status, created_at, created_by)
		VALUES
		($1, $2, 'النص المقبول', 'accepted', now(), $3),
		($4, $2, 'النص المنتظر', 'needs_review', now() + interval '1 hour', $3)`,
		acceptedID, associationID, seeded.actorID, pendingID)

	response, err := service.List(fixture.Ctx(), MapInput{ActorID: seeded.actorID})
	if err != nil {
		t.Fatalf("read the map: %v", err)
	}
	features := 0
	for _, feature := range response.Features {
		if feature.Kind != "association" || feature.ID != associationID.String() {
			continue
		}
		features++
		if feature.EvidenceID != acceptedID.String() {
			t.Fatalf("association evidence = %q, want the accepted row %q (the best-evidence ranking, not an arbitrary join order)", feature.EvidenceID, acceptedID)
		}
		if feature.EvidenceText != "النص المقبول" {
			t.Fatalf("association evidence text = %q, want the accepted row's text", feature.EvidenceText)
		}
	}
	if features != 1 {
		t.Fatalf("one association with two evidence rows drew %d features, want exactly one", features)
	}
}

// TestMapKeepsOneEvidenceFeaturePerMigration is the same parity proof for the
// migration layer, whose evidence join had the same 1:N multiplier.
func TestMapKeepsOneEvidenceFeaturePerMigration(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 3, 1, 2, 0)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)

	migrationID := seeded.migrationID[0] // status documented
	rejectedID := uuid.New()
	acceptedID := uuid.New()
	fixture.Exec(`INSERT INTO spatial_evidence (id, migration_event_id, evidence_text_ar, review_status, created_at, created_by)
		VALUES
		($1, $2, 'النص المرفوض', 'rejected', now(), $3),
		($4, $2, 'النص المقبول', 'accepted', now() + interval '1 hour', $3)`,
		rejectedID, migrationID, seeded.actorID, acceptedID)

	response, err := service.List(fixture.Ctx(), MapInput{ActorID: seeded.actorID})
	if err != nil {
		t.Fatalf("read the map: %v", err)
	}
	features := 0
	for _, feature := range response.Features {
		if feature.Kind != "migration" || feature.ID != migrationID.String() {
			continue
		}
		features++
		if feature.EvidenceID != acceptedID.String() {
			t.Fatalf("migration evidence = %q, want the accepted row %q even though the rejected row is the older one the CASE grammar outranks", feature.EvidenceID, acceptedID)
		}
	}
	if features != 1 {
		t.Fatalf("one migration with two evidence rows drew %d features, want exactly one", features)
	}
}

// TestMigrationViewportKeepsARouteTheBoxOnlyCrosses pins the line viewport
// predicate. The migration's viewport test used the destination-or-origin
// *point*, so a route between two places vanished from any viewport that
// covered the middle of the route but neither endpoint — while the region
// layer already answered the identical question with an area predicate. A
// route is an area question, and the two layers answer it the same way.
func TestMigrationViewportKeepsARouteTheBoxOnlyCrosses(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 3, 1, 2, 0)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)

	// Migration 0 runs place (10.0, 10.0) → place (10.1, 10.0). The box below
	// covers the segment's midpoint and excludes both endpoints: no place is
	// inside it, so anything that comes back is the route itself.
	crossing := &Viewport{MinLongitude: 10.03, MinLatitude: 9.9, MaxLongitude: 10.07, MaxLatitude: 10.1}
	response, err := service.List(fixture.Ctx(), MapInput{ActorID: seeded.actorID, Viewport: crossing})
	if err != nil {
		t.Fatalf("read the map inside the crossing viewport: %v", err)
	}
	if len(response.Features) != 1 {
		t.Fatalf("a viewport only the route crosses returned %d features, want exactly the one migration", len(response.Features))
	}
	if response.Features[0].ID != seeded.migrationID[0].String() {
		t.Fatalf("viewport carried %q, want migration %q", response.Features[0].ID, seeded.migrationID[0])
	}

	// And a box nothing crosses stays empty: the wider test is still a filter,
	// not a keep-everything rule.
	farAway := &Viewport{MinLongitude: 40.0, MinLatitude: 40.0, MaxLongitude: 41.0, MaxLatitude: 41.0}
	response, err = service.List(fixture.Ctx(), MapInput{ActorID: seeded.actorID, Viewport: farAway})
	if err != nil {
		t.Fatalf("read the map inside the distant viewport: %v", err)
	}
	if len(response.Features) != 0 {
		t.Fatalf("a distant viewport returned %d features, want none", len(response.Features))
	}
}

// TestRegionDatedInWindowMarksTheUndatedFrame pins the undated-region marker.
// A region whose valid_from/valid_to are NULL passes every window's keep-all
// rule — the same no-dates convention every layer obeys — and must remain on
// the map under any window; hiding it would claim the region did not exist in
// the era. What the response owes instead is the marker that separates a
// record with its own dated validity from one that never dated itself, and it
// must be absent when no window was asked, because with no window there is no
// era claim to evaluate.
func TestRegionDatedInWindowMarksTheUndatedFrame(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.Register(t, fixture, "مالك مناطق التأريخ")
	fixture.GrantRole(owner.User.ID, "researcher")

	undatedID := uuid.New()
	datedID := uuid.New()
	fixture.Exec(`INSERT INTO historical_regions (id, name_ar, normalized_name_ar, geometry, certainty, valid_from, valid_to, created_by)
		VALUES
		($1, 'إقليم بلا تأريخ', 'إقليم بلا تأريخ', ST_GeomFromText('POLYGON((10 10, 11 10, 11 11, 10 11, 10 10))', 4326), 'approximate', NULL, NULL, $2),
		($3, 'إقليم مؤرَّخ', 'إقليم مؤرخ', ST_GeomFromText('POLYGON((12 10, 13 10, 13 11, 12 11, 12 10))', 4326), 'approximate', '0790-01-01', NULL, $2)`,
		undatedID, owner.User.ID, datedID)

	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)

	readRegion := func(t *testing.T, regionID uuid.UUID, fromYear, toYear int) *bool {
		t.Helper()
		response, err := service.List(fixture.Ctx(), MapInput{ActorID: owner.User.ID, FromYear: fromYear, ToYear: toYear})
		if err != nil {
			t.Fatalf("read the map: %v", err)
		}
		for _, feature := range response.Features {
			if feature.Kind == "region" && feature.ID == regionID.String() {
				return feature.DatedInWindow
			}
		}
		t.Fatalf("region %s is missing from the map (window %d—%d); the marker may never hide a region", regionID, fromYear, toYear)
		return nil
	}

	if dated := readRegion(t, undatedID, 800, 1000); dated == nil || *dated {
		t.Fatalf("an undated region under a window carries datedInWindow = %v, want false", dated)
	}
	if dated := readRegion(t, datedID, 800, 1000); dated == nil || !*dated {
		t.Fatalf("a dated region under a window carries datedInWindow = %v, want true (its emptiness inside the window is the era counters' sentence)", dated)
	}
	// No window, no era answer: the field is absent on both regions, which is
	// what keeps windowless responses byte-compatible with the contract.
	if dated := readRegion(t, undatedID, 0, 0); dated != nil {
		t.Fatalf("an undated region carries datedInWindow = %v with no window, want absent", dated)
	}
	if dated := readRegion(t, datedID, 0, 0); dated != nil {
		t.Fatalf("a dated region carries datedInWindow = %v with no window, want absent", dated)
	}
}
