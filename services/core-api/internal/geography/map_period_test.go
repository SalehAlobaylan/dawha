package geography

import (
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dates"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// A map feature renders its own range through the product's one date contract
// rather than through whatever the client feels like slice out of an ISO string.
// Before this, the client took timeFrom and printed its first four characters -
// the year when a bound exists and nothing at all when it does not - so an open
// range read as a single year and a record with no dates read as a dash, which
// reads as "empty" rather than as "the record does not fix this".
//
// The test drives the shapes through the service rather than through the helper,
// because the claim is about what a client is handed: the raw columns are still
// there, because a client filters on them, and the rendered string has to agree
// with them.

func TestMapFeatureRendersItsPeriodThroughTheDateContract(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedPeriodMap(t, fixture)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)

	response, err := service.List(fixture.Ctx(), MapInput{ActorID: seeded.actorID})
	if err != nil {
		t.Fatalf("read the map: %v", err)
	}
	byID := mapIDs(response)
	for _, expectation := range []struct {
		label string
		id    uuid.UUID
		want  string
	}{
		{label: "a closed range", id: seeded.closed, want: "1120 — 1185 (تقديرية)"},
		{label: "an open end", id: seeded.openEnd, want: "من 1120 (تقديرية)"},
		{label: "an open start", id: seeded.openStart, want: "حتى 1185 (غير مؤكدة)"},
		{label: "no bound at all", id: seeded.unknown, want: dates.Unknown},
		{label: "a precise range is not decorated", id: seeded.precise, want: "1000 — 1050"},
	} {
		feature, found := byID[expectation.id.String()]
		if !found {
			t.Fatalf("%s: the feature is not on the map", expectation.label)
		}
		if feature.Period != expectation.want {
			t.Errorf("%s: period = %q, want %q", expectation.label, feature.Period, expectation.want)
		}
	}

	// Every feature carries a period, including the layers that hold no dates at
	// all: a place has no time. A field that is sometimes a string and sometimes
	// absent is a field the client has to guess about, and a guess is how the three
	// surfaces came to disagree in the first place.
	for _, feature := range response.Features {
		if strings.TrimSpace(feature.Period) == "" {
			t.Errorf("feature %s (%s) carries no period", feature.ID, feature.Kind)
		}
	}

	// And the rendered string never invents a calendar. No era marker and no
	// converted value: the columns hold an ISO date that says nothing about which
	// calendar it is in, so anything of the kind here would be an assertion the
	// record does not support. The reasoning is in internal/dates and in
	// docs/phase-status.md under Phase 10.
	for _, feature := range response.Features {
		if strings.Contains(feature.Period, "هـ") || strings.Contains(feature.Period, "هجري") {
			t.Errorf("feature %s renders a calendar its record does not carry: %q", feature.ID, feature.Period)
		}
	}
}

type periodMap struct {
	actorID string
	closed  uuid.UUID
	// The four remaining shapes the two date columns can hold.
	openEnd   uuid.UUID
	openStart uuid.UUID
	unknown   uuid.UUID
	precise   uuid.UUID
}

func seedPeriodMap(t *testing.T, fixture *testsupport.Fixture) *periodMap {
	t.Helper()
	owner := actor.Register(t, fixture, "مالك العقد")
	seeded := &periodMap{
		actorID:   owner.User.ID,
		closed:    uuid.New(),
		openEnd:   uuid.New(),
		openStart: uuid.New(),
		unknown:   uuid.New(),
		precise:   uuid.New(),
	}
	fixture.GrantRole(owner.User.ID, "researcher")

	// A public tree with a published version, so the people below read as public
	// and the map is not empty for the reader the person policy admits.
	treeID := uuid.New()
	versionID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة العقد', 'public', $2)`, treeID, owner.User.ID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, owner.User.ID)

	placeID := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility, geometry)
		VALUES ($1, 'موضع العقد', 'موضع العقد', 'city', 'public', ST_SetSRID(ST_MakePoint(45, 24), 4326))`, placeID)

	for _, row := range []struct {
		id        uuid.UUID
		from      any
		to        any
		certainty string
	}{
		{id: seeded.closed, from: "1120-01-01", to: "1185-12-31", certainty: "approximate"},
		{id: seeded.openEnd, from: "1120-01-01", to: nil, certainty: "approximate"},
		{id: seeded.openStart, from: nil, to: "1185-12-31", certainty: "uncertain"},
		{id: seeded.unknown, from: nil, to: nil, certainty: "approximate"},
		{id: seeded.precise, from: "1000-01-01", to: "1050-12-31", certainty: "precise"},
	} {
		personID := uuid.New()
		fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, $2, $2, 'reviewed', $3)`,
			personID, fixture.Unique("شخص العقد"), owner.User.ID)
		fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخص', 0)`,
			uuid.New(), versionID, personID)
		fixture.Exec(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, time_from, time_to, created_by)
			VALUES ($1, 'person', $2, $3, 'documented_in', 'documented', $4, $5, $6, $7)`,
			row.id, personID, placeID, row.certainty, row.from, row.to, owner.User.ID)
	}
	return seeded
}
