package testsupport

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaPrefix marks every schema this package creates. A leftover with this
// prefix means a fixture did not clean up, and TestSchemaPrefixDoesNotLeak
// fails the run when one survives.
const SchemaPrefix = "p004_fixture_"

// Fixture is an isolated PostgreSQL schema holding one test's data.
//
// Isolation is by schema rather than by row: each test gets its own migrated
// and seeded schema, and the whole schema is dropped in t.Cleanup. That makes
// the tests order-independent, rerunnable with -count=N, safe to run in
// parallel with other packages, and unable to leave anything behind in the
// shared development database regardless of which foreign keys the production
// code happens to have. Fixtures still create unique users, trees, and sources
// per test, so a test never reads another test's rows.
type Fixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	ctx    context.Context
	schema string
	// tag is a per-fixture marker embedded in every synthetic value, so a test
	// that writes outside the fixture schema is still identifiable.
	tag string
}

// New opens an isolated, migrated and seeded schema for this test.
//
// The gate contract mirrors what tools/dbtestguard enforces in CI:
//
//   - DATABASE_URL unset: skip, because the fast gate deliberately starts no
//     database.
//   - DATABASE_URL set but the schema is not migrated: fail, because a
//     reachable database that cannot answer a query is a broken environment,
//     not a reason to skip.
func New(t *testing.T) *Fixture {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip(DBSkipMessage)
	}
	repositoryRoot, err := FindRepositoryRoot(".")
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	state := InspectSchema(t.Context(), databaseURL, migrationsDir(repositoryRoot))
	if state.ConnectError != nil {
		t.Fatalf("DATABASE_URL is set but the database could not be reached: %v", state.ConnectError)
	}
	if len(state.MissingMigrations) > 0 {
		t.Fatalf("DATABASE_URL is set but %d migration(s) are not applied (run `make db-migrate`): %s",
			len(state.MissingMigrations), strings.Join(state.MissingMigrations, ", "))
	}
	if len(state.MissingExtensions) > 0 {
		t.Fatalf("DATABASE_URL is set but required extension(s) are missing (run `make db-migrate`): %s",
			strings.Join(state.MissingExtensions, ", "))
	}

	fixture := &Fixture{
		t:      t,
		ctx:    t.Context(),
		schema: SchemaPrefix + randomToken(t),
		tag:    randomToken(t),
	}
	fixture.build(t, databaseURL, repositoryRoot)
	t.Cleanup(fixture.drop)
	return fixture
}

// migrationsDir honours DAWHA_MIGRATIONS_DIR so a caller that keeps the
// migrations outside the repository can still point the fixtures at them.
func migrationsDir(repositoryRoot string) string {
	if override := strings.TrimSpace(os.Getenv("DAWHA_MIGRATIONS_DIR")); override != "" {
		return override
	}
	return repositoryRoot + "/db/migrations"
}

func (f *Fixture) build(t *testing.T, databaseURL, repositoryRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 90*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to %s: %v", databaseURL, err)
	}
	defer admin.Close(context.Background())

	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{f.schema}.Sanitize()); err != nil {
		t.Fatalf("create fixture schema: %v", err)
	}
	// public stays on the search path so the shared extensions (postgis,
	// vector, pg_trgm) and their types resolve, while every table the fixture
	// creates lands in the fixture schema.
	if _, err := admin.Exec(ctx, `SET search_path TO `+pgx.Identifier{f.schema}.Sanitize()+`, public`); err != nil {
		t.Fatalf("set fixture search_path: %v", err)
	}
	script, err := FixtureScript(repositoryRoot, f.schema)
	if err != nil {
		t.Fatalf("build fixture script: %v", err)
	}
	if _, err := admin.Exec(ctx, script); err != nil {
		t.Fatalf("migrate fixture schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	config.MaxConns = 8
	config.MinConns = 1
	config.ConnConfig.RuntimeParams["search_path"] = f.schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open fixture pool: %v", err)
	}
	f.pool = pool
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping fixture pool: %v", err)
	}
}

func (f *Fixture) drop() {
	if err := f.dropNow(); err != nil {
		f.t.Errorf("drop fixture schema %s: %v", f.schema, err)
	}
}

// dropNow removes the fixture schema and everything in it. It opens its own
// connection because the pool is closed first, and because the post-run leak
// audit in tools/dbtestguard calls the same path after the whole suite ended.
func (f *Fixture) dropNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, os.Getenv(DatabaseURLEnv))
	if err != nil {
		return err
	}
	defer admin.Close(context.Background())
	_, err = admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{f.schema}.Sanitize()+` CASCADE`)
	return err
}

// FixtureScript concatenates every migration followed by the synthetic seed,
// with the fixture schema pinned on the search path throughout. It is one
// statement string so the whole schema is built in a single round trip.
func FixtureScript(repositoryRoot, schema string) (string, error) {
	searchPath := `SET search_path TO ` + pgx.Identifier{schema}.Sanitize() + `, public;` + "\n"
	var builder strings.Builder
	builder.WriteString(searchPath)
	migrationFiles, err := filepathGlob(repositoryRoot + "/db/migrations/*.sql")
	if err != nil {
		return "", err
	}
	if len(migrationFiles) == 0 {
		return "", fmt.Errorf("no migration files in %s/db/migrations", repositoryRoot)
	}
	for _, file := range migrationFiles {
		contents, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		builder.Write(contents)
		builder.WriteString("\n;\n")
	}
	seed, err := os.ReadFile(repositoryRoot + "/db/seeds/001_demo.sql")
	if err != nil {
		return "", err
	}
	builder.WriteString(searchPath)
	builder.Write(seed)
	builder.WriteString("\n;\n")
	return builder.String(), nil
}

// Pool is the fixture's pool. Every unqualified table name resolves to the
// fixture schema, so production code needs no awareness of this.
func (f *Fixture) Pool() *pgxpool.Pool { return f.pool }

// Ctx is the test's context.
func (f *Fixture) Ctx() context.Context { return f.ctx }

// Schema is the isolated schema name, for assertions and diagnostics.
func (f *Fixture) Schema() string { return f.schema }

// Tag is this fixture's unique marker.
func (f *Fixture) Tag() string { return f.tag }

// Unique returns a value no other fixture in the database can collide with.
func (f *Fixture) Unique(prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, f.tag, randomToken(f.t))
}

// Email is a unique address inside the reserved plan004.test domain.
func (f *Fixture) Email() string { return f.Unique("user") + "@plan004.test" }

// RegisterUser creates a user through the production auth service, so the
// fixture exercises the same registration path a real signup takes.
func (f *Fixture) RegisterUser(displayName string) auth.User {
	f.t.Helper()
	user, err := auth.NewService(f.pool).Register(f.ctx, f.Email(), displayName, f.Unique("pw"))
	if err != nil {
		f.t.Fatalf("register fixture user: %v", err)
	}
	return user
}

// Auth returns the auth service bound to the fixture pool.
func (f *Fixture) Auth() *auth.Service { return auth.NewService(f.pool) }

// Login returns a live session token for an already registered user, which is
// what the HTTP layer reads out of the session cookie.
func (f *Fixture) Login(email, password string) string {
	f.t.Helper()
	_, token, err := f.Auth().Login(f.ctx, email, password)
	if err != nil {
		f.t.Fatalf("login fixture user %s: %v", email, err)
	}
	return token
}

// RegisterAndLogin is the common case: a fresh account with a live session.
func (f *Fixture) RegisterAndLogin(displayName string) (auth.User, string) {
	f.t.Helper()
	password := f.Unique("pw")
	user, err := f.Auth().Register(f.ctx, f.Email(), displayName, password)
	if err != nil {
		f.t.Fatalf("register fixture user: %v", err)
	}
	return user, f.Login(user.Email, password)
}

// QueryRow is a fixture-scoped convenience for assertions.
func (f *Fixture) QueryRow(query string, args ...any) pgx.Row {
	return f.pool.QueryRow(f.ctx, query, args...)
}

// Exec is a fixture-scoped convenience for setup and assertions.
func (f *Fixture) Exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		f.t.Fatalf("exec %s: %v", strings.Fields(query)[0], err)
	}
}

// Count returns the row count of a fixture-scoped query.
func (f *Fixture) Count(query string, args ...any) int {
	f.t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, query, args...).Scan(&count); err != nil {
		f.t.Fatalf("count %s: %v", strings.Fields(query)[0], err)
	}
	return count
}
