package geography

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The subject layer is the surface where a folklore trap lives on the other
// side from the dossier: the person is public, so the route feels like a
// public page, but every segment names a source — and a source that goes
// private must take its segments with it. These tests pin that rule, the
// place rule beside it, and the ordering the canvas draws.
func seedMovement(t *testing.T, fixture *testsupport.Fixture) (personID uuid.UUID, publicSource, privateSource uuid.UUID, placeA, placeB uuid.UUID, ownerID, readerID string) {
	t.Helper()
	owner := actor.Register(t, fixture, "صاحب الرحلة")
	fixture.GrantRole(owner.User.ID, "researcher")
	reader := actor.Register(t, fixture, "قارئ الرحلة")

	publicSource = uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر الرحلة العمومي', 'book', 'public', $2)`, publicSource, owner.User.ID)
	privateSource = uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر الرحلة الخصوصي', 'book', 'private', $2)`, privateSource, owner.User.ID)

	placeA = uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility, created_by)
		VALUES ($1, 'موضع الرحلة أ', 'موضع الرحلة أ', 'city', ST_SetSRID(ST_MakePoint(46.6, 24.6), 4326), 'public', $2)`,
		placeA, owner.User.ID)
	placeB = uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility, created_by)
		VALUES ($1, 'موضع الرحلة ب', 'موضع الرحلة ب', 'city', ST_SetSRID(ST_MakePoint(47.6, 25.6), 4326), 'public', $2)`,
		placeB, owner.User.ID)

	personID = uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by)
		VALUES ($1, 'شخص الرحلة', 'شخص الرحلة', 'reviewed', $2)`, personID, owner.User.ID)
	treeID := uuid.New()
	versionID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة الرحلة', 'public', $2)`, treeID, owner.User.ID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, owner.User.ID)
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخص الرحلة', 0)`, uuid.New(), versionID, personID)

	// Two documented events: one per source, ordered by time_from.
	fixture.Exec(`INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, notes_ar, created_by)
		VALUES ($1, 'person', $2, $3, $4, '1180-01-01', '1181-01-01', 'documented', 'approximate', $5, 'نص الرحلة العمومية.', $6)`,
		uuid.New(), personID, placeA, placeB, publicSource, owner.User.ID)
	fixture.Exec(`INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, notes_ar, created_by)
		VALUES ($1, 'person', $2, $3, $4, '1182-01-01', '1183-01-01', 'documented', 'approximate', $5, 'نص الرحلة الخصوصية.', $6)`,
		uuid.New(), personID, placeB, placeA, privateSource, owner.User.ID)
	// A non-documented event must never appear in the subject layer.
	fixture.Exec(`INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, notes_ar, created_by)
		VALUES ($1, 'person', $2, $3, $4, '1184-01-01', '1185-01-01', 'platform_inferred', 'uncertain', $5, 'نص الاستنتاج.', $6)`,
		uuid.New(), personID, placeA, placeB, publicSource, owner.User.ID)
	return personID, publicSource, privateSource, placeA, placeB, owner.User.ID, reader.User.ID
}

func TestPersonMovementHidesAPrivateSourceEvent(t *testing.T) {
	fixture := testsupport.New(t)
	personID, publicSource, _, placeA, _, _, readerID := seedMovement(t, fixture)
	service := NewService(fixture.Pool())

	// In the seeded shape a plain reader sees exactly the public event, and
	// its endpoints carry coordinates. Tripping the public source to private
	// must remove the event entirely — the layer never draws a segment on a
	// source the reader may not open.
	for _, who := range []string{readerID, ""} {
		report, err := service.PersonMovement(fixture.Ctx(), personID.String(), who)
		if err != nil {
			t.Fatalf("movement for %q: %v", who, err)
		}
		if len(report.Events) != 1 {
			t.Fatalf("reader %q sees %d events, want exactly the public documented one", who, len(report.Events))
		}
		if report.Events[0].FromPlace.Longitude == nil || report.Events[0].ToPlace.Longitude == nil {
			t.Fatalf("event endpoints lack coordinates: %+v", report.Events[0])
		}
		if report.Events[0].FromPlace.ID != placeA.String() {
			t.Fatalf("event from = %s, want %s", report.Events[0].FromPlace.ID, placeA)
		}
		for _, event := range report.Events {
			if event.Status != "documented" {
				t.Fatalf("non-documented event in the subject layer: %q", event.Status)
			}
		}
	}

	// The source flips to private: its event disappears, with no other
	// change. This is the acceptance the plan pins.
	fixture.Exec(`UPDATE sources SET visibility = 'private' WHERE id = $1`, publicSource)
	report, err := service.PersonMovement(fixture.Ctx(), personID.String(), readerID)
	if err != nil {
		t.Fatalf("movement after source went private: %v", err)
	}
	if len(report.Events) != 0 {
		t.Fatalf("private source's events = %d, want 0", len(report.Events))
	}
}

func TestPersonMovementOrderingAndEndpointVisibility(t *testing.T) {
	fixture := testsupport.New(t)
	personID, _, _, placeA, _, _, readerID := seedMovement(t, fixture)
	service := NewService(fixture.Pool())

	// Order is time_from: the public event (1180) precedes any later one.
	report, err := service.PersonMovement(fixture.Ctx(), personID.String(), readerID)
	if err != nil {
		t.Fatalf("movement: %v", err)
	}
	if len(report.Events) != 1 || report.Events[0].TimeFrom != "1180-01-01" {
		t.Fatalf("unexpected ordered events: %+v", report.Events)
	}

	// A private endpoint hides the segment that needs it, even for the
	// public-source event.
	fixture.Exec(`UPDATE places SET visibility = 'private' WHERE id = $1`, placeA)
	hidden, err := service.PersonMovement(fixture.Ctx(), personID.String(), readerID)
	if err != nil {
		t.Fatalf("movement after place went private: %v", err)
	}
	if len(hidden.Events) != 0 {
		t.Fatalf("events through a private place = %d, want 0", len(hidden.Events))
	}

	// A malformed person id is a validation answer, not a leak.
	if _, err := service.PersonMovement(fixture.Ctx(), "walnut", readerID); err != ErrValidation {
		t.Fatalf("malformed person id error = %v, want ErrValidation", err)
	}
}
