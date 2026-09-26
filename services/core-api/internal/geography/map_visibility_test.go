package geography

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The map is a public read path, so the person policy belongs in it exactly as it
// belongs in the dictionary index, the dictionary detail and the search index. An
// association and a migration are about their subject: the feature carries the
// subject's type, id and name, so a subject the reader may not see cannot be drawn
// with its name blanked and drawn with its id withheld - the row itself is the
// disclosure. These tests pin the rule, the choice, and the fact that the public map
// does not shrink by anything other than the leak itself.

type mapFixture struct {
	ownerID        uuid.UUID
	publishedID    uuid.UUID
	draftID        uuid.UUID
	placeID        uuid.UUID
	publishedAssoc string
	draftAssoc     string
	publishedMigr  string
	draftMigr      string
}

func openMapPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedMapFixture builds one public place with a published person and a research-only
// draft person attached to it, by association and by migration. The two people differ
// in exactly one way - membership of a published tree version - so the only thing the
// map may act on is the policy.
func seedMapFixture(t *testing.T, pool *pgxpool.Pool) *mapFixture {
	t.Helper()
	ctx := context.Background()
	fixture := &mapFixture{ownerID: uuid.New(), publishedID: uuid.New(), draftID: uuid.New(), placeID: uuid.New()}
	tag := fixture.ownerID.String()[:8]

	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'صاحب الخريطة')`,
		fixture.ownerID, "map-person-policy-"+tag+"@dawha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES
			($1, 'شخص منشور ' || $3, 'شخص منشور ' || $3, 'reviewed', $4),
			($2, 'شخص مسودة سرّي ' || $3, 'شخص مسودة سرّي ' || $3, 'unreviewed', $4)
	`, fixture.publishedID, fixture.draftID, tag, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	// The published person is in a published version of a public tree; the draft one
	// is in a draft, which is what the person policy reads.
	treeID := uuid.New()
	versionID := uuid.New()
	draftVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة ' || $2, 'public', $3)`, treeID, tag, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES
			($1, $2, 1, 'published', $3, now()),
			($4, $2, 2, 'draft', $3, NULL)
	`, versionID, treeID, fixture.ownerID, draftVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES
			($1, $2, $3, 'منشور', 0),
			($4, $5, $6, 'مسودة', 1)
	`, uuid.New(), versionID, fixture.publishedID, uuid.New(), draftVersionID, fixture.draftID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
		VALUES ($1, 'موضع الخريطة ' || $2, 'موضع الخريطة ' || $2, 'city', ST_SetSRID(ST_MakePoint(46.7, 24.7), 4326), 'public')
	`, fixture.placeID, tag); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر الخريطة ' || $2, 'book', 'public', $3)
	`, uuid.New(), tag, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	var sourceID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM sources WHERE title_ar = $1`, "مصدر الخريطة "+tag).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	fixture.publishedAssoc = uuid.New().String()
	fixture.draftAssoc = uuid.New().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, source_id, created_by) VALUES
			($1, 'person', $3, $5, 'documented_in', $6, $7),
			($2, 'person', $4, $5, 'documented_in', $6, $7)
	`, fixture.publishedAssoc, fixture.draftAssoc, fixture.publishedID, fixture.draftID, fixture.placeID, sourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.publishedMigr = uuid.New().String()
	fixture.draftMigr = uuid.New().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO migration_events (id, subject_type, subject_id, to_place_id, status, source_id, created_by) VALUES
			($1, 'person', $3, $5, 'documented', $6, $7),
			($2, 'person', $4, $5, 'documented', $6, $7)
	`, fixture.publishedMigr, fixture.draftMigr, fixture.publishedID, fixture.draftID, fixture.placeID, sourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		// One statement per placeholder count, in dependency order, so a delete
		// never has to guess how many arguments the one before it took.
		for _, statement := range []struct {
			query string
			arg   any
		}{
			{`DELETE FROM migration_events WHERE id = ANY($1::uuid[])`, []uuid.UUID{mustParse(t, fixture.publishedMigr), mustParse(t, fixture.draftMigr)}},
			{`DELETE FROM geographic_associations WHERE id = ANY($1::uuid[])`, []uuid.UUID{mustParse(t, fixture.publishedAssoc), mustParse(t, fixture.draftAssoc)}},
			{`DELETE FROM tree_nodes WHERE person_id = ANY($1::uuid[])`, []uuid.UUID{fixture.publishedID, fixture.draftID}},
			{`DELETE FROM tree_versions WHERE tree_id IN (SELECT id FROM trees WHERE owner_id = $1)`, fixture.ownerID},
			{`DELETE FROM trees WHERE owner_id = $1`, fixture.ownerID},
			{`DELETE FROM sources WHERE created_by = $1`, fixture.ownerID},
			{`DELETE FROM places WHERE id = $1`, fixture.placeID},
			{`DELETE FROM people WHERE id = ANY($1::uuid[])`, []uuid.UUID{fixture.publishedID, fixture.draftID}},
			{`DELETE FROM audit_log WHERE actor_id = $1`, fixture.ownerID},
			{`DELETE FROM user_roles WHERE user_id = $1`, fixture.ownerID},
			{`DELETE FROM users WHERE id = $1`, fixture.ownerID},
		} {
			if _, err := pool.Exec(cleanup, statement.query, statement.arg); err != nil {
				t.Errorf("cleanup failed for %q: %v", statement.query, err)
			}
		}
	})
	return fixture
}

func mustParse(t *testing.T, value string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func mapPayload(t *testing.T, response MapResponse) string {
	t.Helper()
	encoded, err := json.Marshal(response.Features)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// TestMapHidesResearchOnlyPersonEndpoints is the leak itself. A draft person's name
// and id reached the anonymous map through both an association and a migration; the
// anonymous payload must now contain neither, and a privileged actor must still see
// both.
func TestMapHidesResearchOnlyPersonEndpoints(t *testing.T) {
	pool := openMapPool(t)
	fixture := seedMapFixture(t, pool)
	service := NewService(pool)
	ctx := context.Background()

	anonymous, err := service.List(ctx, MapInput{})
	if err != nil {
		t.Fatal(err)
	}
	payload := mapPayload(t, anonymous)
	if !containsID(payload, fixture.publishedAssoc) {
		t.Fatalf("the published person's association is missing from the anonymous map: %s", payload)
	}
	if !containsID(payload, fixture.publishedMigr) {
		t.Fatalf("the published person's migration is missing from the anonymous map: %s", payload)
	}
	// Not the id, not the name, not anywhere in the payload.
	for _, secret := range []string{fixture.draftID.String(), "شخص مسودة سرّي"} {
		if contains(payload, secret) {
			t.Fatalf("the anonymous map disclosed %q: %s", secret, payload)
		}
	}
	if containsID(payload, fixture.draftAssoc) || containsID(payload, fixture.draftMigr) {
		t.Fatalf("an association or migration to a research-only person is on the anonymous map: %s", payload)
	}
	// A filtered map is the same payload, so the leak cannot come back through the
	// place filter either.
	filtered, err := service.GetPlace(ctx, fixture.placeID.String(), MapInput{})
	if err != nil {
		t.Fatal(err)
	}
	filteredPayload := mapPayload(t, filtered)
	if contains(filteredPayload, fixture.draftID.String()) || containsID(filteredPayload, fixture.draftAssoc) {
		t.Fatalf("the place map disclosed a research-only person: %s", filteredPayload)
	}
	if !containsID(filteredPayload, fixture.publishedAssoc) {
		t.Fatalf("the place map lost the published person's association: %s", filteredPayload)
	}

	privileged, err := service.List(ctx, MapInput{ActorID: fixture.ownerID.String()})
	if err != nil {
		t.Fatal(err)
	}
	privilegedPayload := mapPayload(t, privileged)
	for _, expected := range []string{fixture.draftAssoc, fixture.draftMigr, fixture.draftID.String(), "شخص مسودة سرّي"} {
		if !contains(privilegedPayload, expected) {
			t.Fatalf("the role that owns the draft person cannot see %q on its own map: %s", expected, privilegedPayload)
		}
	}
}

// TestMapKeepsEveryPublicAssociation is the guard on the fix itself: the public map
// must lose the row that was the leak and nothing else.
//
// It asserts by identifier and never by count. A count over the whole map is a
// measurement of the shared development database, and several packages insert
// places and associations into it while this test runs, so such an assertion is
// flaky for reasons that have nothing to do with the code under test. What is
// stable, and what this test checks instead, is the set: every public feature the
// policy allows has to be drawn with its payload intact, and the seeded person
// associations are checked one by one by their known identifiers.
func TestMapKeepsEveryPublicAssociation(t *testing.T) {
	pool := openMapPool(t)
	service := NewService(pool)
	ctx := context.Background()

	anonymous, err := service.List(ctx, MapInput{})
	if err != nil {
		t.Fatal(err)
	}
	features := anonymous.Features

	// Every seeded association and migration whose endpoint person is in a published
	// tree version is public knowledge and has to be drawn, with its name, its place
	// and its coordinates unchanged.
	publicPeople := map[string]bool{}
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT n.person_id
		FROM tree_nodes n JOIN tree_versions v ON v.id = n.tree_version_id
		WHERE v.state = 'published'
	`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		publicPeople[id.String()] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(publicPeople) == 0 {
		t.Fatal("no seeded person is in a published version, so this fixture proves nothing")
	}
	wanted := map[string]string{}
	associationRows, err := pool.Query(ctx, `
		SELECT ga.id, ep.canonical_name_ar
		FROM geographic_associations ga
		JOIN places p ON p.id = ga.place_id
		LEFT JOIN people ep ON ga.entity_type = 'person' AND ep.id = ga.entity_id
		LEFT JOIN sources s ON s.id = ga.source_id
		WHERE p.visibility = 'public' AND p.geometry IS NOT NULL
		  AND (s.id IS NULL OR s.visibility = 'public')
		  AND (ga.entity_type <> 'person' OR ep.id = ANY($1::uuid[]))
	`, uuidList(publicPeople))
	if err != nil {
		t.Fatal(err)
	}
	for associationRows.Next() {
		var id, name string
		if err := associationRows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		wanted[id] = name
	}
	associationRows.Close()
	if err := associationRows.Err(); err != nil {
		t.Fatal(err)
	}
	migrationRows, err := pool.Query(ctx, `
		SELECT m.id, ep.canonical_name_ar
		FROM migration_events m
		LEFT JOIN places pf ON pf.id = m.from_place_id
		LEFT JOIN places pt ON pt.id = m.to_place_id
		LEFT JOIN people ep ON m.subject_type = 'person' AND ep.id = m.subject_id
		LEFT JOIN sources s ON s.id = m.source_id
		WHERE (pf.id IS NULL OR pf.visibility = 'public') AND (pt.id IS NULL OR pt.visibility = 'public')
		  AND (pf.geometry IS NOT NULL OR pt.geometry IS NOT NULL)
		  AND (s.id IS NULL OR s.visibility = 'public')
		  AND (m.subject_type <> 'person' OR ep.id = ANY($1::uuid[]))
	`, uuidList(publicPeople))
	if err != nil {
		t.Fatal(err)
	}
	for migrationRows.Next() {
		var id, name string
		if err := migrationRows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		wanted[id] = name
	}
	migrationRows.Close()
	if err := migrationRows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(wanted) == 0 {
		t.Fatal("the fixture has no public person association, so this test proves nothing")
	}

	drawn := map[string]Feature{}
	for _, feature := range features {
		drawn[feature.ID] = feature
	}
	for id, name := range wanted {
		feature, ok := drawn[id]
		if !ok {
			t.Fatalf("the public feature %s (%s) is no longer on the anonymous map", id, name)
		}
		if feature.Kind != "association" && feature.Kind != "migration" {
			t.Fatalf("feature %s changed kind to %q", id, feature.Kind)
		}
		if feature.EntityName != name {
			t.Fatalf("feature %s lost its public name: %q, want %q", id, feature.EntityName, name)
		}
		if feature.PlaceName == "" || feature.Longitude == nil || feature.Latitude == nil {
			t.Fatalf("feature %s lost its place or coordinates: %+v", id, feature)
		}
	}

	// The two seeded person associations, by their known identifiers. The first
	// points at a person in a published version and is public; the second points at
	// a person that is in no published version, and its absence is the fix. Naming
	// both is what keeps this from being a quiet shrink: a reviewer can see exactly
	// which seeded row the person policy removed and why.
	seeded := []struct {
		id      string
		present bool
		reason  string
	}{
		{id: "a0000000-0000-0000-0000-000000000001", present: true, reason: "its person is in a published tree version"},
		{id: "a0000000-0000-0000-0000-000000000002", present: false, reason: "its person is in no published tree version, so it was never public"},
	}
	byID := map[string]Feature{}
	for _, feature := range features {
		byID[feature.ID] = feature
	}
	for _, testCase := range seeded {
		feature, ok := byID[testCase.id]
		if testCase.present && !ok {
			t.Fatalf("the seeded association %s is gone from the anonymous map, and %s", testCase.id, testCase.reason)
		}
		if !testCase.present && ok {
			t.Fatalf("the seeded association %s is still on the anonymous map even though %s: %+v", testCase.id, testCase.reason, feature)
		}
		if testCase.present && feature.EntityName == "" {
			t.Fatalf("the seeded public association %s lost its name: %+v", testCase.id, feature)
		}
	}
	// The seeded place features and the seeded migration are untouched, again by
	// identifier rather than by count.
	for _, id := range []string{
		"20000000-0000-0000-0000-000000000001",
		"20000000-0000-0000-0000-000000000002",
		"20000000-0000-0000-0000-000000000003",
		"20000000-0000-0000-0000-000000000004",
		"a1000000-0000-0000-0000-000000000001",
	} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("the seeded public feature %s is no longer on the anonymous map", id)
		}
	}
}

func uuidList(values map[string]bool) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(values))
	for value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			continue
		}
		ids = append(ids, parsed)
	}
	return ids
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func containsID(payload, id string) bool {
	return contains(payload, id)
}
