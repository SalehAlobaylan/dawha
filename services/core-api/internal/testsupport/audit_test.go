package testsupport

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The audit is the mechanism the CI database job relies on to refuse a green
// tick for a suite that never touched PostgreSQL. These cases pin the decision
// table; a guard that cannot fail is not a guard.
func TestAuditFailsWhenADatabaseBackedTestSkipped(t *testing.T) {
	result := Audit(AuditInput{
		ExitCode: 0,
		Events: []Event{
			{Action: "run", Package: "svc/internal/jobs", Test: "TestClaim"},
			{Action: "skip", Package: "svc/internal/jobs", Test: "TestClaim"},
			{Action: "output", Package: "svc/internal/jobs", Test: "TestClaim", Output: "    service_integration_test.go:15: " + DBSkipMessage + "\n"},
		},
		RequiredPackages: []string{"svc/internal/jobs"},
		Schema:           SchemaState{URL: "postgres://db/dawha"},
	})
	if result.OK {
		t.Fatal("audit passed a run whose only database-backed test skipped")
	}
	if len(result.DatabaseSkips) != 1 || result.DatabaseSkips[0] != "svc/internal/jobs.TestClaim" {
		t.Fatalf("database skips = %v, want the skipped test reported", result.DatabaseSkips)
	}
	if !containsSubstring(result.Problems, "1 database-backed test(s) skipped") {
		t.Fatalf("problems = %v, want the silent skip named", result.Problems)
	}
}

func TestAuditFailsWhenGoTestFailed(t *testing.T) {
	result := Audit(AuditInput{
		ExitCode: 1,
		Events: []Event{
			{Action: "run", Package: "svc/internal/jobs", Test: "TestClaim"},
			{Action: "fail", Package: "svc/internal/jobs", Test: "TestClaim"},
		},
		RequiredPackages: []string{"svc/internal/jobs"},
		Schema:           SchemaState{URL: "postgres://db/dawha"},
	})
	if result.OK || !containsSubstring(result.Problems, "go test exited with code 1") {
		t.Fatalf("audit = %+v, want the non-zero exit reported", result)
	}
}

func TestAuditFailsWhenDatabaseURLIsSetButTheSchemaIsMissing(t *testing.T) {
	result := Audit(AuditInput{
		ExitCode: 0,
		Events: []Event{
			{Action: "run", Package: "svc/internal/jobs", Test: "TestClaim"},
			{Action: "pass", Package: "svc/internal/jobs", Test: "TestClaim"},
		},
		RequiredPackages: []string{"svc/internal/jobs"},
		Schema: SchemaState{
			URL:               "postgres://db/dawha",
			MissingMigrations: []string{"0038_suggestion_change_sets"},
		},
	})
	if result.OK {
		t.Fatal("audit passed a run whose database was never migrated")
	}
	if !containsSubstring(result.Problems, "0038_suggestion_change_sets") {
		t.Fatalf("problems = %v, want the unapplied migration named", result.Problems)
	}
}

func TestAuditFailsWhenTheDatabaseCannotBeReached(t *testing.T) {
	result := Audit(AuditInput{
		Schema: SchemaState{URL: "postgres://db/dawha", ConnectError: errString("connection refused")},
	})
	if result.OK || !containsSubstring(result.Problems, "could not be reached") {
		t.Fatalf("audit = %+v, want an unreachable DATABASE_URL reported", result)
	}
}

func TestAuditFailsWhenARequiredPackageRanNothing(t *testing.T) {
	result := Audit(AuditInput{
		Events: []Event{
			{Action: "run", Package: "svc/internal/jobs", Test: "TestClaim"},
			{Action: "pass", Package: "svc/internal/jobs", Test: "TestClaim"},
		},
		RequiredPackages: []string{"svc/internal/jobs", "svc/internal/questions"},
		Schema:           SchemaState{URL: "postgres://db/dawha"},
	})
	if result.OK || !containsSubstring(result.Problems, "svc/internal/questions ran no test") {
		t.Fatalf("audit = %+v, want the missing required package reported", result)
	}
}

func TestAuditPassesAFullyExercisedRun(t *testing.T) {
	result := Audit(AuditInput{
		Events: []Event{
			{Action: "run", Package: "svc/internal/jobs", Test: "TestClaim"},
			{Action: "pass", Package: "svc/internal/jobs", Test: "TestClaim"},
			{Action: "run", Package: "svc/internal/questions", Test: "TestDispute"},
			{Action: "pass", Package: "svc/internal/questions", Test: "TestDispute"},
		},
		RequiredPackages: []string{"svc/internal/jobs", "svc/internal/questions"},
		Schema:           SchemaState{URL: "postgres://db/dawha", AppliedMigrations: 38, MissingExtensions: nil},
	})
	if !result.OK {
		t.Fatalf("audit = %+v, want a clean pass", result)
	}
	if result.Total != 2 || result.Passed != 2 || result.Skipped != 0 {
		t.Fatalf("counts = %+v, want two passing tests", result)
	}
	if result.Schema.InstalledExtension != len(RequiredExtensions) {
		t.Fatalf("installed extensions = %d, want %d", result.Schema.InstalledExtension, len(RequiredExtensions))
	}
}

// A skip that needs more than PostgreSQL is not a silent database skip, so it
// belongs in OtherSkips and must not fail the audit.
func TestAuditToleratesASkipThatNeedsMoreThanTheDatabase(t *testing.T) {
	result := Audit(AuditInput{
		Events: []Event{
			{Action: "run", Package: "svc/internal/sourceprocessing", Test: "TestResolve"},
			{Action: "skip", Package: "svc/internal/sourceprocessing", Test: "TestResolve"},
			{Action: "output", Package: "svc/internal/sourceprocessing", Test: "TestResolve", Output: "    x_test.go:49: DATABASE_URL and AI_RESEARCH_URL are required\n"},
			{Action: "run", Package: "svc/internal/sourceprocessing", Test: "TestExtract"},
			{Action: "pass", Package: "svc/internal/sourceprocessing", Test: "TestExtract"},
		},
		RequiredPackages: []string{"svc/internal/sourceprocessing"},
		Schema:           SchemaState{URL: "postgres://db/dawha"},
	})
	if len(result.DatabaseSkips) != 0 {
		t.Fatalf("database skips = %v, want none", result.DatabaseSkips)
	}
	if len(result.OtherSkips) != 1 {
		t.Fatalf("other skips = %v, want the AI-gated skip recorded separately", result.OtherSkips)
	}
	if !result.OK {
		t.Fatalf("audit = %+v, want the run to pass", result)
	}
}

// TestDBSkipMessagesStayAuditable keeps the guard honest. If a test starts
// gating on DATABASE_URL with wording the audit does not recognise, that test
// would skip silently in CI, so the wording is pinned to DBSkipMessage.
func TestDBSkipMessagesStayAuditable(t *testing.T) {
	root, err := FindRepositoryRoot(".")
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	module := filepath.Join(root, "services", "core-api")
	skipCall := regexp.MustCompile(`t\.Skip\("([^"]*)"\)`)
	violations := []string{}
	err = filepath.WalkDir(module, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range skipCall.FindAllStringSubmatch(string(contents), -1) {
			message := match[1]
			if !strings.Contains(message, "DATABASE_URL") {
				continue
			}
			if strings.Contains(message, "AI_RESEARCH_URL") {
				continue
			}
			if message != DBSkipMessage {
				relative, _ := filepath.Rel(module, path)
				violations = append(violations, relative+": "+message)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", module, err)
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("tests gate on DATABASE_URL with wording the audit does not recognise.\n"+
			"Use testsupport.DBSkipMessage (%q) so a skip cannot pass as success, or name the\n"+
			"other environment the test also needs.\n%v", DBSkipMessage, violations)
	}
}

// TestRequiredPackageManifestMatchesTheTree keeps testdata/db-required-packages
// honest: a package that gains a database-backed test - by reading DATABASE_URL
// itself or by opening a testsupport fixture - has to be added to the manifest
// the CI audit enforces, and a package that loses one has to leave it.
func TestRequiredPackageManifestMatchesTheTree(t *testing.T) {
	root, err := FindRepositoryRoot(".")
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	module := filepath.Join(root, "services", "core-api")
	derived := map[string]bool{}
	err = filepath.WalkDir(module, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		directory := filepath.Dir(path)
		relative, err := filepath.Rel(module, directory)
		if err != nil {
			return err
		}
		slug := filepath.ToSlash(relative)
		// This package's own audit tests contain the search tokens, so it can
		// derive nothing about itself. It is listed in the manifest
		// unconditionally instead, and its database-backed fixture test is what
		// the CI audit checks it against.
		if slug == "internal/testsupport" {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !isDatabaseBackedTestFile(string(contents)) {
			return nil
		}
		derived["github.com/SalehAlobaylan/dawha/services/core-api/"+slug] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", module, err)
	}
	if len(derived) == 0 {
		t.Fatal("no package references the database URL; the manifest cannot be validated")
	}
	// testsupport is derived unconditionally above; see the walk for why.
	derived["github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"] = true
	listed := ReadRequiredPackages(t, filepath.Join(root, "services", "core-api", "internal", "testsupport", "testdata", "db-required-packages.txt"))
	listedSet := map[string]bool{}
	for _, name := range listed {
		listedSet[name] = true
	}
	missing := []string{}
	for name := range derived {
		if !listedSet[name] {
			missing = append(missing, name)
		}
	}
	stale := []string{}
	for name := range listedSet {
		if !derived[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Fatalf("packages with a DATABASE_URL test are missing from the required manifest: %v", missing)
	}
	if len(stale) > 0 {
		t.Fatalf("the required manifest lists packages with no DATABASE_URL test: %v", stale)
	}
}

// ReadRequiredPackages parses the required-package manifest, ignoring blank
// lines and # comments.
func ReadRequiredPackages(t *testing.T, path string) []string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	packages := []string{}
	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		packages = append(packages, trimmed)
	}
	return packages
}

// isDatabaseBackedTestFile reports whether a test file opts into PostgreSQL,
// either by reading the database URL itself or by opening a testsupport
// fixture. Both spellings count, so a new integration test cannot slip past the
// required manifest by using one helper instead of the other. The search tokens
// are built from DatabaseURLEnv so this file's own source does not look like a
// database-backed test to the checks it defines.
func isDatabaseBackedTestFile(text string) bool {
	envLookup := "Getenv(\"" + DatabaseURLEnv + "\")"
	return strings.Contains(text, envLookup) || strings.Contains(text, "testsupport.New(")
}

func containsSubstring(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

type stringError string

func (e stringError) Error() string { return string(e) }

func errString(message string) error { return stringError(message) }
