package testsupport

import (
	"os"
	"strings"
	"testing"
)

// TestFixtureIsIsolatedAndDropped pins the isolation contract the rest of the
// acceptance suite relies on. A fixture must be invisible to the shared
// development database and to any other fixture, and it must leave nothing
// behind when the test ends.
func TestFixtureIsIsolatedAndDropped(t *testing.T) {
	databaseURL := databaseURLOrSkip(t)
	before, err := ListFixtureSchemas(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("list fixture schemas: %v", err)
	}

	fixture := New(t)
	schema := fixture.Schema()
	if !strings.HasPrefix(schema, SchemaPrefix) {
		t.Fatalf("fixture schema %q does not carry the %q prefix, so a leak cannot be attributed", schema, SchemaPrefix)
	}
	if live, err := ListFixtureSchemas(t.Context(), databaseURL); err != nil {
		t.Fatalf("list fixture schemas: %v", err)
	} else if !containsString(live, schema) {
		t.Fatalf("fixture schema %s was not created; visible fixture schemas: %v", schema, live)
	}

	// The synthetic seed landed in the fixture, so a test can rely on the
	// Arabic lineage without building it.
	if got := fixture.Count(`SELECT count(*) FROM people WHERE id = '10000000-0000-0000-0000-000000000001'`); got != 1 {
		t.Fatalf("seeded people rows = %d, want 1", got)
	}

	user := fixture.RegisterUser("باحث العزل")
	if got := fixture.Count(`SELECT count(*) FROM users WHERE id = $1`, user.ID); got != 1 {
		t.Fatalf("fixture user rows = %d, want 1", got)
	}
	// The public schema the developer works in is untouched by a fixture write.
	if got := fixture.Count(`SELECT count(*) FROM public.users WHERE id = $1`, user.ID); got != 0 {
		t.Fatalf("the fixture user leaked into public.users (%d rows)", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM public.users WHERE email LIKE '%@plan004.test'`); got != 0 {
		t.Fatalf("public.users holds %d fixture row(s); a fixture must not write outside its own schema", got)
	}

	// The cleanup this test is really about: dropping the schema takes the data
	// with it, so nothing is left in the shared database.
	if err := fixture.dropNow(); err != nil {
		t.Fatalf("drop fixture schema: %v", err)
	}
	after, err := ListFixtureSchemas(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("list fixture schemas: %v", err)
	}
	if containsString(after, schema) {
		t.Fatalf("fixture schema %s survived the drop; visible fixture schemas: %v", schema, after)
	}
	for _, name := range after {
		if !containsString(before, name) {
			t.Fatalf("the fixture left a schema behind: %s", name)
		}
	}
}

// TestFixturesDoNotShareRows proves two fixtures cannot see each other's data,
// which is what makes the suite order-independent and safe under -count=N.
func TestFixturesDoNotShareRows(t *testing.T) {
	first := New(t)
	second := New(t)
	if first.Schema() == second.Schema() {
		t.Fatalf("two fixtures share the schema %s", first.Schema())
	}
	firstUser := first.RegisterUser("باحث أول")
	secondUser := second.RegisterUser("باحث ثانٍ")
	if got := second.Count(`SELECT count(*) FROM users WHERE id = $1`, firstUser.ID); got != 0 {
		t.Fatalf("the second fixture sees %d row(s) of the first fixture's user", got)
	}
	if got := first.Count(`SELECT count(*) FROM users WHERE id = $1`, secondUser.ID); got != 0 {
		t.Fatalf("the first fixture sees %d row(s) of the second fixture's user", got)
	}
}

func databaseURLOrSkip(t *testing.T) string {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv(DatabaseURLEnv))
	if databaseURL == "" {
		t.Skip(DBSkipMessage)
	}
	return databaseURL
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
