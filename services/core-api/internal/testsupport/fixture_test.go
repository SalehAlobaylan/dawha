package testsupport

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/jackc/pgx/v5"
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

	user := register(t, fixture, "باحث العزل")
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
	// Only the schemas this test owns are asserted on. Other packages run in parallel
	// against the same database, so the *count* of visible fixture schemas can move
	// either way while this test runs - a parallel fixture starting is not a leak and
	// a parallel fixture finishing is not this drop reaching further than it owns -
	// and a count is therefore not a measurement of anything. What this drop is
	// responsible for is the schema it created, and the sweep below reports exactly
	// that. The post-run leak audit in tools/dbtestguard is what checks the whole
	// database once every package has finished.
	if leaked := leakedFixtureSchemas(after, []string{schema}); len(leaked) > 0 {
		t.Fatalf("fixture schemas this test owns survived the drop: %v (the baseline was %d visible schemas); visible now: %v", leaked, len(before), after)
	}
	// The sweep is the assertion, so the sweep has to be able to fail. This is that
	// proof, and it is here rather than in a comment because a detector nobody has
	// seen fail is a detector nobody should trust: a schema with this test's own
	// prefix and tag is created on purpose, and the sweep has to report it.
	leaked := SchemaPrefix + "selfcheck_" + fixture.Tag()
	if _, err := fixture.pool.Exec(t.Context(), `CREATE SCHEMA `+pgx.Identifier{leaked}.Sanitize()); err != nil {
		t.Fatalf("create the deliberately leaked schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := fixture.pool.Exec(cleanupCtx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{leaked}.Sanitize()+` CASCADE`); err != nil {
			t.Errorf("dropping the deliberately leaked schema failed: %v", err)
		}
	})
	visible, err := ListFixtureSchemas(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("list fixture schemas: %v", err)
	}
	if reported := leakedFixtureSchemas(visible, []string{leaked}); len(reported) != 1 || reported[0] != leaked {
		t.Fatalf("the leak sweep did not report the schema it was given: reported %v, visible %v", reported, visible)
	}
}

// leakedFixtureSchemas returns the named schemas that are present in visible. It is
// the whole assertion TestFixtureIsIsolatedAndDropped makes, kept as a function so
// the test can prove it reports a leak rather than only that it reports nothing.
func leakedFixtureSchemas(visible, owned []string) []string {
	present := make(map[string]bool, len(visible))
	for _, name := range visible {
		present[name] = true
	}
	reported := make([]string, 0, len(owned))
	for _, name := range owned {
		if present[name] {
			reported = append(reported, name)
		}
	}
	return reported
}

// TestFixturesDoNotShareRows proves two fixtures cannot see each other's data,
// which is what makes the suite order-independent and safe under -count=N.
func TestFixturesDoNotShareRows(t *testing.T) {
	first := New(t)
	second := New(t)
	if first.Schema() == second.Schema() {
		t.Fatalf("two fixtures share the schema %s", first.Schema())
	}
	firstUser := register(t, first, "باحث أول")
	secondUser := register(t, second, "باحث ثانٍ")
	if got := second.Count(`SELECT count(*) FROM users WHERE id = $1`, firstUser.ID); got != 0 {
		t.Fatalf("the second fixture sees %d row(s) of the first fixture's user", got)
	}
	if got := first.Count(`SELECT count(*) FROM users WHERE id = $1`, secondUser.ID); got != 0 {
		t.Fatalf("the first fixture sees %d row(s) of the second fixture's user", got)
	}
}

// register creates an account the way the auth service does. This package's own
// test cannot use internal/testsupport/actor - that package imports this one -
// so it reaches for the production auth service directly instead.
func register(t *testing.T, fixture *Fixture, displayName string) auth.User {
	t.Helper()
	user, err := auth.NewService(fixture.Pool()).Register(fixture.Ctx(), fixture.Email(), displayName, fixture.Unique("pw"))
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}
	return user
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
