package geography

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The map used to read every visible row of four tables and apply the status, place
// and year filters afterwards, in Go. The database therefore built, joined and
// shipped rows the caller had already decided not to see, and the response grew with
// the dataset.
//
// These tests are about that change and about what it must not have changed. The
// fixture is deliberately larger than the seeded demo data, because on a handful of
// features "the database shipped every row" and "the database shipped the rows the
// caller asked for" produce the same numbers and a test over them proves nothing.

// scaledMap is one map fixture plus the bookkeeping a test needs to say what the
// right answer is without asking the code under test.
type scaledMap struct {
	actorID string
	// placeID and placeLongitude/placeLatitude line up by index, so a test can say
	// which places fall inside a box without re-deriving the arithmetic.
	placeID        []uuid.UUID
	placeLongitude []float64
	placeLatitude  []float64
	// associationID lines up with associationStatus, associationPlace and
	// associationYear.
	associationID     []uuid.UUID
	associationStatus []string
	associationPlace  []uuid.UUID
	associationYear   []int
	migrationID       []uuid.UUID
	migrationStatus   []string
	migrationPlace    []uuid.UUID
	migrationYear     []int
	regionID          []uuid.UUID
	regionDisputed    []bool
	regionYear        []int
	regionLongitude   []float64
	regionLatitude    []float64
}

// statuses are the five drawn statuses the three evidence layers can carry.
var scaledStatuses = []string{"documented", "interpreted", "platform_inferred", "disputed", "unresolved"}

// seedScaledMap builds a map big enough for the difference between "the database
// shipped every row" and "the database shipped the rows the caller asked for" to
// show up, and records what it created so a test can name the expected answer.
func seedScaledMap(t *testing.T, fixture *testsupport.Fixture, places, associations, migrations, regions int) *scaledMap {
	t.Helper()
	owner := actor.Register(t, fixture, "مالك الخريطة")
	ownerID := owner.User.ID
	// A researcher is the one reader who holds the identity write role set, so the
	// research-only places below are on this reader's map. An anonymous reader would
	// not see them, which is the visibility floor and is asserted elsewhere.
	fixture.GrantRole(ownerID, "researcher")

	// A public tree with a published version, so the people below read as public
	// and the map is not empty for a reader the person policy admits.
	treeID := uuid.New()
	versionID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة الخريطة', 'public', $2)`, treeID, ownerID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, ownerID)

	people := make([]uuid.UUID, 0, associations)
	for index := 0; index < associations; index++ {
		personID := uuid.New()
		fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, $2, $3, 'reviewed', $4)`,
			personID, fmt.Sprintf("شخص الخريطة %d", index), fmt.Sprintf("شخص الخريطة %d", index), ownerID)
		fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخص', $4)`,
			uuid.New(), versionID, personID, 0)
		people = append(people, personID)
	}

	seeded := &scaledMap{actorID: ownerID}
	for index := 0; index < places; index++ {
		placeID := uuid.New()
		// The places tile a grid, so a box covers a known share of them and a test
		// can say which ones by arithmetic rather than by reading the map.
		longitude := 10.0 + float64(index%100)/10.0
		latitude := 10.0 + float64(index/100)/10.0
		visibilityValue := "public"
		if index%17 == 0 {
			visibilityValue = "private"
		}
		fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
			VALUES ($1, $2, $3, 'city', ST_SetSRID(ST_MakePoint($4, $5), 4326), $6)`,
			placeID, fmt.Sprintf("موضع الخريطة %d", index), fmt.Sprintf("موضع الخريطة %d", index), longitude, latitude, visibilityValue)
		seeded.placeID = append(seeded.placeID, placeID)
		seeded.placeLongitude = append(seeded.placeLongitude, longitude)
		seeded.placeLatitude = append(seeded.placeLatitude, latitude)
	}

	for index := 0; index < associations; index++ {
		associationID := uuid.New()
		status := scaledStatuses[index%len(scaledStatuses)]
		// Half the associations start in the ninth century and half in the
		// sixteenth, so a year window has rows on both sides of it.
		year := 900
		if index%2 == 0 {
			year = 1500
		}
		placeID := seeded.placeID[index%len(seeded.placeID)]
		fixture.Exec(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, time_from, created_by)
			VALUES ($1, 'person', $2, $3, 'documented_in', $4, 'approximate', $5, $6)`,
			associationID, people[index%len(people)], placeID, status, fmt.Sprintf("%04d-01-01", year), ownerID)
		seeded.associationID = append(seeded.associationID, associationID)
		seeded.associationStatus = append(seeded.associationStatus, status)
		seeded.associationPlace = append(seeded.associationPlace, placeID)
		seeded.associationYear = append(seeded.associationYear, year)
	}

	for index := 0; index < migrations; index++ {
		migrationID := uuid.New()
		status := scaledStatuses[index%len(scaledStatuses)]
		year := 1000
		if index%2 == 0 {
			year = 1600
		}
		from := seeded.placeID[index%len(seeded.placeID)]
		to := seeded.placeID[(index+1)%len(seeded.placeID)]
		fixture.Exec(`INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, status, certainty, time_from, created_by)
			VALUES ($1, 'person', $2, $3, $4, $5, 'approximate', $6, $7)`,
			migrationID, people[index%len(people)], from, to, status, fmt.Sprintf("%04d-01-01", year), ownerID)
		seeded.migrationID = append(seeded.migrationID, migrationID)
		seeded.migrationStatus = append(seeded.migrationStatus, status)
		// A migration is drawn at its destination, which is the place the feature
		// carries, so the place filter has to be asked about the same column.
		seeded.migrationPlace = append(seeded.migrationPlace, to)
		seeded.migrationYear = append(seeded.migrationYear, year)
	}

	for index := 0; index < regions; index++ {
		regionID := uuid.New()
		disputed := index%3 == 0
		certainty := "approximate"
		if disputed {
			certainty = "disputed"
		}
		year := 800
		if index%2 == 0 {
			year = 1400
		}
		longitude := 10.0 + float64(index%100)/10.0
		latitude := 10.0 + float64(index/100)/10.0
		fixture.Exec(`INSERT INTO historical_regions (id, name_ar, normalized_name_ar, geometry, certainty, valid_from, created_by)
			VALUES ($1, $2, $3, ST_SetSRID(ST_MakePolygon(ST_MakeLine(ARRAY[ST_MakePoint($4, $5), ST_MakePoint($4 + 0.5, $5), ST_MakePoint($4 + 0.5, $5 + 0.5), ST_MakePoint($4, $5 + 0.5), ST_MakePoint($4, $5)])), 4326), $6, $7, $8)`,
			regionID, fmt.Sprintf("إقليم الخريطة %d", index), fmt.Sprintf("إقليم الخريطة %d", index), longitude, latitude, certainty, fmt.Sprintf("%04d-01-01", year), ownerID)
		seeded.regionID = append(seeded.regionID, regionID)
		seeded.regionDisputed = append(seeded.regionDisputed, disputed)
		seeded.regionYear = append(seeded.regionYear, year)
		// A region is drawn at the centroid of a polygon whose south-west corner is
		// the point above, so the centroid sits a quarter of a degree in from it.
		seeded.regionLongitude = append(seeded.regionLongitude, longitude+0.25)
		seeded.regionLatitude = append(seeded.regionLatitude, latitude+0.25)
	}
	return seeded
}

// mapIDs is the set of feature identifiers a response carries, which is the thing a
// filter is allowed to change and the thing a visibility rule is not.
func mapIDs(response MapResponse) map[string]Feature {
	byID := make(map[string]Feature, len(response.Features))
	for _, feature := range response.Features {
		byID[feature.ID] = feature
	}
	return byID
}

// TestMapDefaultFiltersKeepEveryFeature is the behaviour-change guard. The default
// read carries no status, place, year or viewport filter, so it has to return every
// feature the unfiltered map returned, and a test that only checked "some features
// came back" would not notice a filter that quietly dropped the rest.
func TestMapDefaultFiltersKeepEveryFeature(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 400, 1200, 400, 300)
	pool, counter := fixture.CountingFixturePool(t)
	service := NewService(pool)

	counter.Reset()
	response, err := service.List(fixture.Ctx(), MapInput{ActorID: seeded.actorID})
	if err != nil {
		t.Fatalf("read the map: %v", err)
	}
	if response.Truncated {
		t.Fatal("a map of 2300 features reported truncation against a bound of 5000")
	}
	if response.Limit != MaxMapFeatures {
		t.Fatalf("response limit = %d, want %d", response.Limit, MaxMapFeatures)
	}
	drawn := mapIDs(response)

	// Every row the fixture created has to be on the default map, by identifier. The
	// places are the only layer a filter could plausibly have narrowed - they carry
	// no dates, and a year predicate that required a date would have dropped every
	// one of them.
	for index, placeID := range seeded.placeID {
		feature, found := drawn[placeID.String()]
		if !found {
			t.Fatalf("place %d (%s) is missing from the default map", index, placeID)
		}
		if feature.Status != "documented" {
			t.Fatalf("place %d is drawn as %q, want documented", index, feature.Status)
		}
		if feature.Longitude == nil || *feature.Longitude != seeded.placeLongitude[index] {
			t.Fatalf("place %d moved: %+v", index, feature)
		}
	}
	for index, associationID := range seeded.associationID {
		feature, found := drawn[associationID.String()]
		if !found {
			t.Fatalf("association %d (%s) is missing from the default map", index, associationID)
		}
		if feature.Status != seeded.associationStatus[index] {
			t.Fatalf("association %d is drawn as %q, want %q", index, feature.Status, seeded.associationStatus[index])
		}
	}
	for index, migrationID := range seeded.migrationID {
		feature, found := drawn[migrationID.String()]
		if !found {
			t.Fatalf("migration %d (%s) is missing from the default map", index, migrationID)
		}
		if feature.Status != seeded.migrationStatus[index] {
			t.Fatalf("migration %d is drawn as %q, want %q", index, feature.Status, seeded.migrationStatus[index])
		}
	}
	for index, regionID := range seeded.regionID {
		feature, found := drawn[regionID.String()]
		if !found {
			t.Fatalf("region %d (%s) is missing from the default map", index, regionID)
		}
		want := "interpreted"
		if seeded.regionDisputed[index] {
			want = "disputed"
		}
		if feature.Status != want {
			t.Fatalf("region %d is drawn as %q, want %q", index, feature.Status, want)
		}
	}
	// The response is still the four layers concatenated in the order it always was.
	if len(response.Features) < len(seeded.placeID)+len(seeded.associationID)+len(seeded.migrationID)+len(seeded.regionID) {
		t.Fatalf("the default map returned %d features, fewer than the %d the fixture created",
			len(response.Features), len(seeded.placeID)+len(seeded.associationID)+len(seeded.migrationID)+len(seeded.regionID))
	}
	// The read is still four feature queries plus the role lookup.
	counter.AssertAtMost(t, 6, "reading the unfiltered map")
	t.Logf("STEP3 default queries=%d rowsFromDatabase=%d featuresReturned=%d payloadBytes=%d",
		counter.Count(), counter.Rows(), len(response.Features), testsupport.PayloadBytes(t, response))
}

// TestMapFiltersExcludeBeforeTheResponse is the measurement, asserted. A filtered
// read used to ship every visible row of the four tables; it now ships the rows the
// filter kept, plus at most one probe row per layer that had to look ahead. A filter
// that moved back into Go would ship thousands of rows again.
//
// The expected set is derived from the unfiltered map rather than from a hand-written
// count. The fixture schema also carries the seeded demo rows, which no test owns, so
// a literal expected count would be a measurement of somebody else's data. The
// unfiltered map is proven complete by TestMapDefaultFiltersKeepEveryFeature, which
// checks every row the fixture created is on it, so filtering that set and comparing
// is an exact statement about what the filter removed.
func TestMapFiltersExcludeBeforeTheResponse(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 400, 1200, 400, 300)
	pool, counter := fixture.CountingFixturePool(t)
	service := NewService(pool)
	ctx := fixture.Ctx()

	// The unfiltered map, and the identifier set the fixture owns within it.
	counter.Reset()
	full, err := service.List(ctx, MapInput{ActorID: seeded.actorID})
	if err != nil {
		t.Fatalf("read the map: %v", err)
	}
	unfilteredShipped := counter.Rows() - 1
	owned := ownedIDs(seeded)
	unfilteredOwned := make([]Feature, 0, len(owned))
	for _, feature := range full.Features {
		if _, mine := owned[feature.ID]; mine {
			unfilteredOwned = append(unfilteredOwned, feature)
		}
	}
	if len(unfilteredOwned) != len(owned) {
		t.Fatalf("the unfiltered map holds %d of the %d features the fixture created", len(unfilteredOwned), len(owned))
	}

	for _, testCase := range []struct {
		name  string
		input MapInput
		keep  func(Feature) bool
	}{
		{
			name:  "status",
			input: MapInput{Status: "documented"},
			keep:  func(feature Feature) bool { return feature.Status == "documented" },
		},
		{
			name:  "years",
			input: MapInput{FromYear: 1000, ToYear: 1250},
			keep: func(feature Feature) bool {
				year := yearOf(feature)
				return year == 0 || year <= 1250
			},
		},
		{
			name:  "viewport",
			input: MapInput{Viewport: &Viewport{MinLongitude: 10, MinLatitude: 10, MaxLongitude: 15, MaxLatitude: 12}},
			keep: func(feature Feature) bool {
				if feature.Longitude == nil || feature.Latitude == nil {
					return false
				}
				return insideBox(*feature.Longitude, *feature.Latitude, 10, 10, 15, 12)
			},
		},
	} {
		counter.Reset()
		filtered, filterErr := service.List(ctx, MapInput{ActorID: seeded.actorID, Status: testCase.input.Status, FromYear: testCase.input.FromYear, ToYear: testCase.input.ToYear, Viewport: testCase.input.Viewport})
		if filterErr != nil {
			t.Fatalf("%s: %v", testCase.name, filterErr)
		}
		expected := make(map[string]struct{})
		for _, feature := range unfilteredOwned {
			if testCase.keep(feature) {
				expected[feature.ID] = struct{}{}
			}
		}
		if len(expected) == 0 || len(expected) == len(unfilteredOwned) {
			t.Fatalf("%s: the filter kept %d of %d rows, so it distinguishes nothing", testCase.name, len(expected), len(unfilteredOwned))
		}
		for _, feature := range filtered.Features {
			if _, mine := owned[feature.ID]; !mine {
				continue
			}
			if _, wanted := expected[feature.ID]; !wanted {
				t.Fatalf("%s: feature %s (%s) reached the response although the filter excludes it", testCase.name, feature.ID, feature.Status)
			}
			if !testCase.keep(feature) {
				t.Fatalf("%s: feature %s reached the response and does not satisfy the filter", testCase.name, feature.ID)
			}
		}
		for id := range expected {
			if _, mine := owned[id]; !mine {
				continue
			}
			if !containsIDIn(filtered, id) {
				t.Fatalf("%s: feature %s satisfies the filter and is missing from the response", testCase.name, id)
			}
		}
		// The property the change is about: the database built and sent the rows the
		// filter kept, plus at most one probe row per layer. A filter applied after
		// the read would send all of them.
		shipped := counter.Rows() - 1
		if shipped > int64(len(filtered.Features)+4) {
			t.Fatalf("%s: the database shipped %d rows to return %d features, want at most the features plus one probe row per layer",
				testCase.name, shipped, len(filtered.Features))
		}
		if shipped >= unfilteredShipped {
			t.Fatalf("%s: a filtered read shipped %d rows, the unfiltered read shipped %d: the filter did not reach the query",
				testCase.name, shipped, unfilteredShipped)
		}
		t.Logf("STEP3 %s queries=%d rowsFromDatabase=%d featuresReturned=%d unfilteredRows=%d payloadBytes=%d",
			testCase.name, counter.Count(), counter.Rows(), len(filtered.Features), unfilteredShipped, testsupport.PayloadBytes(t, filtered))
	}

	// A place filter narrows to one place and to the features drawn there, and the
	// regions go with it: a region has no place of its own to match.
	placeID := seeded.placeID[7].String()
	counter.Reset()
	scoped, err := service.List(ctx, MapInput{ActorID: seeded.actorID, PlaceID: placeID})
	if err != nil {
		t.Fatalf("place filter: %v", err)
	}
	if !containsIDIn(scoped, placeID) {
		t.Fatal("the place itself is not on its own map")
	}
	for _, feature := range scoped.Features {
		if feature.Kind == "region" {
			t.Fatalf("a historical region is on a place-scoped map: %+v", feature)
		}
		if feature.PlaceID != placeID {
			t.Fatalf("feature %s is drawn at %s, not the place that was asked for", feature.ID, feature.PlaceID)
		}
	}
	shipped := counter.Rows() - 1
	if shipped > int64(len(scoped.Features)+4) {
		t.Fatalf("a place-scoped map cost the database %d rows for %d features", shipped, len(scoped.Features))
	}
}

// TestMapResponseIsBoundedAndSaysSo is the truncation half. A bound that does not
// announce itself turns "this is everything" into "this is what fitted", and a map
// that cannot tell the difference cannot be drawn correctly.
func TestMapResponseIsBoundedAndSaysSo(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 400, 1200, 400, 300)
	pool, counter := fixture.CountingFixturePool(t)
	service := NewService(pool)
	ctx := fixture.Ctx()

	// A bound that lands inside the first layer never reaches the second, so the map
	// has to say it is short even though it never asked the other layers.
	bounded, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: 10})
	if err != nil {
		t.Fatalf("bounded map: %v", err)
	}
	if len(bounded.Features) != 10 {
		t.Fatalf("bounded map = %d features, want 10", len(bounded.Features))
	}
	if !bounded.Truncated {
		t.Fatal("a 10-feature bound over a 2300-feature map did not report truncation")
	}
	if bounded.Limit != 10 {
		t.Fatalf("response limit = %d, want 10", bounded.Limit)
	}
	// The first ten features are the first ten of the unfiltered map, in order: the
	// bound cuts the tail, it does not reshuffle.
	full, err := service.List(ctx, MapInput{ActorID: seeded.actorID})
	if err != nil {
		t.Fatalf("full map: %v", err)
	}
	for index := range bounded.Features {
		if bounded.Features[index].ID != full.Features[index].ID {
			t.Fatalf("feature %d is %s on the bounded map and %s on the full one", index, bounded.Features[index].ID, full.Features[index].ID)
		}
	}

	// A bound that lands exactly on a layer boundary is the case a naive cut gets
	// wrong: the last query filled the page exactly, so "did the page fill up" says
	// no, and the map would report itself complete while the associations after it
	// are missing. The bound is spent, so the map has to say so.
	placeCount := 0
	for _, feature := range full.Features {
		if feature.Kind == "place" {
			placeCount++
		}
	}
	onBoundary, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: placeCount})
	if err != nil {
		t.Fatalf("map bounded at the layer boundary: %v", err)
	}
	if !onBoundary.Truncated {
		t.Fatalf("a bound of exactly %d places did not report truncation, but the layers after it hold more", placeCount)
	}
	if len(onBoundary.Features) != placeCount {
		t.Fatalf("a bound of %d returned %d features, want the bound", placeCount, len(onBoundary.Features))
	}
	for _, feature := range onBoundary.Features {
		if feature.Kind != "place" {
			t.Fatalf("a bound of %d reached the %q layer: %+v", placeCount, feature.Kind, feature)
		}
	}
	// One below the boundary truncates for the ordinary reason, and one below zero
	// rows is refused rather than wrapped.
	below, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: placeCount - 1})
	if err != nil {
		t.Fatalf("map bounded one below the boundary: %v", err)
	}
	if !below.Truncated || len(below.Features) != placeCount-1 {
		t.Fatalf("a bound of %d returned %d features, truncated=%v", placeCount-1, len(below.Features), below.Truncated)
	}
	// A bound equal to the whole map is the other boundary, and it is not truncation.
	exactly, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: len(full.Features)})
	if err != nil {
		t.Fatalf("map bounded at the whole map: %v", err)
	}
	if exactly.Truncated || len(exactly.Features) != len(full.Features) {
		t.Fatalf("a bound of the whole map (%d) returned %d features, truncated=%v", len(full.Features), len(exactly.Features), exactly.Truncated)
	}

	// A bound larger than the map is not truncation, and it is not an error.
	wide, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: MaxMapFeatures})
	if err != nil {
		t.Fatalf("wide map: %v", err)
	}
	if wide.Truncated || len(wide.Features) != len(full.Features) {
		t.Fatalf("a bound of %d returned %d features, truncated=%v; want the whole map",
			MaxMapFeatures, len(wide.Features), wide.Truncated)
	}

	// A bound the map cannot serve is refused rather than clamped, so a caller that
	// asked for too much learns about it.
	if _, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: MaxMapFeatures + 1}); err != ErrValidation {
		t.Fatalf("a limit above the bound = %v, want %v", err, ErrValidation)
	}
	// A bounded read is a bounded read: the database is asked for the bound, not for
	// the table.
	counter.Reset()
	if _, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: 10}); err != nil {
		t.Fatalf("bounded map: %v", err)
	}
	if rows := counter.Rows(); rows > 12 {
		t.Fatalf("a 10-feature map cost the database %d rows, so the bound did not reach the query", rows)
	}
}

// TestMapKeepsTheEvidenceLayersDistinct is the phase-23 requirement: everything the
// platform inferred is labelled, and the source-backed and inferred layers stay
// separate. A status filter that compared a column instead of the drawn status would
// collapse them, which is the one way this change could quietly lose a layer.
func TestMapKeepsTheEvidenceLayersDistinct(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 40, 200, 100, 60)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)
	ctx := fixture.Ctx()

	full, err := service.List(ctx, MapInput{ActorID: seeded.actorID})
	if err != nil {
		t.Fatalf("read the map: %v", err)
	}
	byStatus := map[string]int{}
	for _, feature := range full.Features {
		byStatus[feature.Status]++
	}
	// The three layers have to be three populations, not one.
	for _, status := range []string{"documented", "interpreted", "platform_inferred", "disputed", "unresolved"} {
		if byStatus[status] == 0 {
			t.Fatalf("no feature is drawn as %q: the layers collapsed into %v", status, byStatus)
		}
	}

	for _, status := range []string{"documented", "interpreted", "platform_inferred", "disputed", "unresolved"} {
		filtered, filterErr := service.List(ctx, MapInput{ActorID: seeded.actorID, Status: status})
		if filterErr != nil {
			t.Fatalf("filter by %q: %v", status, filterErr)
		}
		if len(filtered.Features) != byStatus[status] {
			t.Fatalf("status=%q returned %d features, and the unfiltered map has %d of them", status, len(filtered.Features), byStatus[status])
		}
		for _, feature := range filtered.Features {
			if feature.Status != status {
				t.Fatalf("status=%q returned a feature drawn as %q", status, feature.Status)
			}
		}
	}

	// The region's certainty is the source of its drawn status and both have to
	// survive the filter, because a client decides what to draw from one and a
	// reviewer reads the other.
	disputed, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Status: "disputed"})
	if err != nil {
		t.Fatalf("filter by disputed: %v", err)
	}
	disputedRegions := 0
	for _, feature := range disputed.Features {
		if feature.Kind != "region" {
			continue
		}
		disputedRegions++
		if feature.Certainty != "disputed" {
			t.Fatalf("a region drawn as disputed has certainty %q", feature.Certainty)
		}
	}
	if disputedRegions == 0 {
		t.Fatal("no disputed region on a map that has them: the inferred and disputed layers merged")
	}
	interpreted, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Status: "interpreted"})
	if err != nil {
		t.Fatalf("filter by interpreted: %v", err)
	}
	interpretedRegions := 0
	for _, feature := range interpreted.Features {
		if feature.Kind != "region" {
			continue
		}
		interpretedRegions++
		if feature.Certainty == "disputed" {
			t.Fatalf("a region drawn as interpreted has certainty %q", feature.Certainty)
		}
	}
	if interpretedRegions == 0 {
		t.Fatal("no interpreted region on a map that has them")
	}
	if disputedRegions+interpretedRegions != len(seeded.regionID) {
		t.Fatalf("the region layers cover %d of %d regions", disputedRegions+interpretedRegions, len(seeded.regionID))
	}
}

// TestMapOrderIsATotalOrder is the determinism half. A bounded response over an
// order with ties picks an arbitrary subset, so a repeated read has to answer with
// the same features in the same sequence.
func TestMapOrderIsATotalOrder(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedScaledMap(t, fixture, 60, 300, 150, 90)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)
	ctx := fixture.Ctx()

	for _, limit := range []int{0, 25, 137, MaxMapFeatures} {
		first, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: limit})
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		seen := make(map[string]struct{}, len(first.Features))
		for index, feature := range first.Features {
			if _, duplicate := seen[feature.ID]; duplicate {
				t.Fatalf("limit %d: feature %s came back twice", limit, feature.ID)
			}
			seen[feature.ID] = struct{}{}
			// The layers are concatenated in a fixed order, so no place may follow a
			// region and no association may precede a place.
			if index > 0 && layerRank(feature.Kind) < layerRank(first.Features[index-1].Kind) {
				t.Fatalf("feature %d is a %q after a %q: the layer order changed",
					index, feature.Kind, first.Features[index-1].Kind)
			}
		}
		for round := 0; round < 3; round++ {
			again, err := service.List(ctx, MapInput{ActorID: seeded.actorID, Limit: limit})
			if err != nil {
				t.Fatalf("limit %d round %d: %v", limit, round, err)
			}
			if len(again.Features) != len(first.Features) {
				t.Fatalf("limit %d round %d: %d features, first read had %d", limit, round, len(again.Features), len(first.Features))
			}
			for index := range first.Features {
				if again.Features[index].ID != first.Features[index].ID {
					t.Fatalf("limit %d round %d: feature %d is %s and was %s", limit, round, index, again.Features[index].ID, first.Features[index].ID)
				}
			}
		}
	}
}

func containsIDIn(response MapResponse, id string) bool {
	for _, feature := range response.Features {
		if feature.ID == id {
			return true
		}
	}
	return false
}

// ownedIDs is every feature the fixture created, which is the part of a response a
// test may reason about: the schema also carries the seeded demo rows, which belong
// to no test.
func ownedIDs(seeded *scaledMap) map[string]struct{} {
	owned := make(map[string]struct{}, len(seeded.placeID)+len(seeded.associationID)+len(seeded.migrationID)+len(seeded.regionID))
	for _, group := range [][]uuid.UUID{seeded.placeID, seeded.associationID, seeded.migrationID, seeded.regionID} {
		for _, id := range group {
			owned[id.String()] = struct{}{}
		}
	}
	return owned
}

// yearOf reads the year a feature is drawn at, the same way the Go filter did: a
// feature with no dates has no year, and no year is not zero evidence of one.
func yearOf(feature Feature) int {
	for _, value := range []string{feature.TimeFrom, feature.TimeTo} {
		if len(value) >= 4 {
			if year, err := strconv.Atoi(value[:4]); err == nil {
				return year
			}
		}
	}
	return 0
}

// layerRank is the order the four layers are concatenated in, so a test can assert
// the response still has that shape.
func layerRank(kind string) int {
	switch kind {
	case "place":
		return 0
	case "association":
		return 1
	case "migration":
		return 2
	case "region":
		return 3
	default:
		return 4
	}
}

func insideBox(longitude, latitude, west, south, east, north float64) bool {
	return longitude >= west && longitude <= east && latitude >= south && latitude <= north
}
