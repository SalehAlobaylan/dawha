package testsupport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// InspectSchema connects to databaseURL and reports whether the acceptance
// suite can actually run there: every db/migrations version recorded in
// schema_migrations, and every extension from 0001_extensions.sql installed.
//
// The public schema is inspected on purpose. The integration fixtures in this
// package build their own isolated schema, so a migrated public schema is the
// only evidence that `make db-migrate` ran.
func InspectSchema(ctx context.Context, databaseURL, migrationsDir string) SchemaState {
	state := SchemaState{URL: databaseURL}
	if strings.TrimSpace(databaseURL) == "" {
		return state
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		state.ConnectError = err
		return state
	}
	defer conn.Close(context.Background())

	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		// A reachable database with no migration table has never been migrated.
		// That is a missing schema, not an unreachable server, and the audit
		// reports it as such rather than as a connection problem.
		state.MissingMigrations, _ = MigrationVersions(migrationsDir)
		state.ConnectError = nil
		return state
	}
	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			state.ConnectError = err
			return state
		}
		applied[version] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		state.ConnectError = err
		return state
	}
	state.AppliedMigrations = len(applied)

	wanted, err := MigrationVersions(migrationsDir)
	if err != nil {
		state.ConnectError = err
		return state
	}
	for _, version := range wanted {
		if !applied[version] {
			state.MissingMigrations = append(state.MissingMigrations, version)
		}
	}

	installed := map[string]bool{}
	extensionRows, err := conn.Query(ctx, `SELECT extname FROM pg_extension`)
	if err != nil {
		state.ConnectError = fmt.Errorf("read pg_extension: %w", err)
		return state
	}
	for extensionRows.Next() {
		var name string
		if err := extensionRows.Scan(&name); err != nil {
			extensionRows.Close()
			state.ConnectError = err
			return state
		}
		installed[name] = true
	}
	extensionRows.Close()
	if err := extensionRows.Err(); err != nil {
		state.ConnectError = err
		return state
	}
	for _, extension := range RequiredExtensions {
		if !installed[extension] {
			state.MissingExtensions = append(state.MissingExtensions, extension)
		}
	}
	return state
}

// MigrationVersions returns the sorted versions of every db/migrations file.
func MigrationVersions(migrationsDir string) ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations in %s: %w", migrationsDir, err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		versions = append(versions, strings.TrimSuffix(name, ".sql"))
	}
	sort.Strings(versions)
	return versions, nil
}

// FindRepositoryRoot walks up from start looking for the db/migrations directory
// that the repository root owns. Tests run with their package directory as the
// working directory, so the migration files live a fixed number of levels up
// but the exact depth differs per package; walking up keeps one code path.
func FindRepositoryRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(current, "db", "migrations")); err == nil && info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no db/migrations directory above %s", start)
		}
		current = parent
	}
}

// ListFixtureSchemas returns the fixture schemas currently present. The
// acceptance suite uses it to prove no fixture survived its test, both from
// inside a test and from the post-run audit in tools/dbtestguard where the
// whole suite has already finished and no other package can be mid-fixture.
func ListFixtureSchemas(ctx context.Context, databaseURL string) ([]string, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.Background())
	rows, err := conn.Query(ctx, `SELECT nspname FROM pg_namespace WHERE nspname LIKE $1 ORDER BY nspname`, SchemaPrefix+"%")
	if err != nil {
		return nil, err
	}
	schemas := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		schemas = append(schemas, name)
	}
	rows.Close()
	return schemas, rows.Err()
}
