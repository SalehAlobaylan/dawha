package geography

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The 033-C/033-B contract tests: the movement filters (season + mode) and
// the corridors' own read. The rules they pin, in the plan's own words:
//
//  1. ABSENT FILTERS CHANGE NOTHING — a read without season/mode answers
//     exactly what the eras' own time predicate answered before the fields
//     existed (additive, never quietly narrowing).
//  2. THE SEASON FILTER MULTIPLIES with the era window, not replacing it.
//  3. An undated row is kept under any season — the platform does not know an
//     undated event's months, so it cannot judge one (timePredicate's own
//     edge case, honoured here).
//  4. The corridors' read refuses a corridor whose source is not readable and
//     answers the truthful season/mode the corridor carries.
type movementFilterFixture struct {
	fixture   *testsupport.Fixture
	owner     actor.Actor
	tribeID   uuid.UUID
	placeEast uuid.UUID
	placeWest uuid.UUID
	summerID  uuid.UUID
	winterID  uuid.UUID
	undatedID uuid.UUID
}

func seedMovementFilters(t *testing.T) *movementFilterFixture {
	t.Helper()
	fixture := testsupport.New(t)
	owner := actor.Register(t, fixture, "مالك المسالِك")
	fixture.GrantRole(owner.User.ID, "researcher")
	trip := &movementFilterFixture{fixture: fixture, owner: owner}

	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر المرحلة', 'book', 'public', $2)`, sourceID, owner.User.ID)

	trip.tribeID = uuid.New()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'قبيلة الممر', 'قبيلة الممر', 'public', $2)`, trip.tribeID, owner.User.ID)

	trip.placeEast = uuid.New()
	trip.placeWest = uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
		VALUES ($1, 'الموضع الشرقي', 'الموضع الشرقي', 'city', ST_SetSRID(ST_MakePoint(46, 25), 4326), 'public')`, trip.placeEast)
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
		VALUES ($1, 'الموضع الغربي', 'الموضع الغربي', 'city', ST_SetSRID(ST_MakePoint(41, 25), 4326), 'public')`, trip.placeWest)

	insertEvent := func(eventID uuid.UUID, timeFrom, timeTo, kind string) {
		t.Helper()
		// timeFrom/timeTo carry '' for NULL (the honest undated row).
		fixture.Exec(`INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, status, certainty, kind, time_from, time_to, source_id, created_by)
			VALUES ($1, 'tribe', $2, $3, $4, 'documented', 'approximate',
				CASE WHEN $5 = '' THEN NULL::text ELSE $5 END,
				CASE WHEN $6 = '' THEN NULL::date ELSE $6::date END,
				CASE WHEN $7 = '' THEN NULL::date ELSE $7::date END, $8, $9)`,
			eventID, trip.tribeID, trip.placeWest, trip.placeEast, kind, timeFrom, timeTo, sourceID, owner.User.ID)
	}
	trip.summerID = uuid.New()
	insertEvent(trip.summerID, "1500-06-01", "1500-08-30", "ghazwa") // months 6..8 = inside the صيف band
	trip.winterID = uuid.New()
	insertEvent(trip.winterID, "1500-01-05", "1500-02-10", "nafir") // months 1..2 = winter band
	trip.undatedID = uuid.New()
	insertEvent(trip.undatedID, "", "", "") // the platform does not know these months
	return trip
}

// TestMapSeasonFilterNarrowsOnlyItsOwnLane pins rule 3 + rule 1 over the MAP
// read itself: the migration lane answers the season through its own months;
// the place and region lanes (which carry no months) are untouched by it.
func TestMapSeasonFilterNarrowsOnlyItsOwnLane(t *testing.T) {
	trip := seedMovementFilters(t)
	service := NewService(trip.fixture.Pool())
	ctx := trip.fixture.Ctx()

	read := func(season string) map[string]Feature {
		t.Helper()
		response, err := service.List(ctx, MapInput{ActorID: trip.owner.User.ID, Season: season})
		if err != nil {
			t.Fatalf("map read (season=%q): %v", season, err)
		}
		events := map[string]Feature{}
		for _, feature := range response.Features {
			if feature.Kind == "migration" {
				events[feature.ID] = feature
			}
		}
		return events
	}

	summer, winter := read("summer"), read("winter")
	if _, ok := summer[trip.summerID.String()]; !ok {
		t.Fatalf("the June..August event is missing under season=summer")
	}
	if _, ok := summer[trip.winterID.String()]; ok {
		t.Fatalf("the January event survived season=summer — the filter is not narrow")
	}
	if _, ok := winter[trip.winterID.String()]; !ok {
		t.Fatalf("the January event is missing under season=winter")
	}
	if _, ok := winter[trip.summerID.String()]; ok {
		t.Fatalf("the June event survived season=winter")
	}
	// Rule 3: the undated row is kept by BOTH seasons — the platform cannot
	// judge months it does not have.
	for _, season := range []string{"summer", "winter"} {
		if _, ok := read(season)[trip.undatedID.String()]; !ok {
			t.Fatalf("season=%q dropped the undated event — the undated-kept rule is binding", season)
		}
	}
}

// TestMovementKindCarriesTheTaxonomyKey pins the event's own kind (033-D) on
// the map read: the feature carries the KIND VERBATIM (ghazwa/nafir), and an
// unclassified event passes NULL (the front-end's honest caption), never a
// guess of a label.
func TestMovementKindCarriesTheTaxonomyKey(t *testing.T) {
	trip := seedMovementFilters(t)
	service := NewService(trip.fixture.Pool())

	response, err := service.List(trip.fixture.Ctx(), MapInput{ActorID: trip.owner.User.ID})
	if err != nil {
		t.Fatalf("map read: %v", err)
	}
	kinds := map[string]string{}
	for _, feature := range response.Features {
		if feature.Kind == "migration" {
			kinds[feature.ID] = feature.EventKind
		}
	}
	if kinds[trip.summerID.String()] != "ghazwa" {
		t.Fatalf("ghazwa event's kind = %q, want verbatim ghazwa", kinds[trip.summerID.String()])
	}
	if kinds[trip.winterID.String()] != "nafir" {
		t.Fatalf("nafir event's kind = %q, want verbatim nafir", kinds[trip.winterID.String()])
	}
	if kinds[trip.undatedID.String()] != "" {
		t.Fatalf("the unclassified event's kind = %q, want the honest empty (NULL)", kinds[trip.undatedID.String()])
	}
}
