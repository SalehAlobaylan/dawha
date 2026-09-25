// Database-backed tests: the isolation contract, the skip audit, and the
// fixtures both live here.
package testsupport

import (
	"fmt"
	"sort"
	"strings"
)

// Event is one record of `go test -json` output.
type Event struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// SchemaState describes what the audit found when it connected to DATABASE_URL.
type SchemaState struct {
	// URL is the database the audit inspected.
	URL string
	// ConnectError is non-nil when the database could not be reached at all.
	ConnectError error
	// MissingMigrations lists db/migrations versions absent from
	// schema_migrations in the public schema.
	MissingMigrations []string
	// AppliedMigrations is how many migration versions the database recorded.
	AppliedMigrations int
	// MissingExtensions lists required extensions that are not installed.
	MissingExtensions []string
}

// Connected reports whether the audit reached a database at all.
func (s SchemaState) Connected() bool {
	return strings.TrimSpace(s.URL) != "" && s.ConnectError == nil
}

// AuditInput is everything the audit needs to decide whether a Go run counts as
// a real database pass.
type AuditInput struct {
	// ExitCode is the exit status go test reported.
	ExitCode int
	// Events are the parsed `go test -json` records.
	Events []Event
	// RequiredPackages must each have contributed at least one test that
	// actually ran, so deleting or emptying a package cannot look like a pass.
	RequiredPackages []string
	// Schema is the state of the database the run was pointed at.
	Schema SchemaState
}

// AuditResult is the machine-readable outcome the CLI prints.
type AuditResult struct {
	OK                bool              `json:"ok"`
	ExitCode          int               `json:"exit_code"`
	Total             int               `json:"total"`
	Passed            int               `json:"passed"`
	Failed            int               `json:"failed"`
	Skipped           int               `json:"skipped"`
	DatabaseSkips     []string          `json:"database_skips"`
	OtherSkips        []string          `json:"other_skips"`
	RequiredPackages  []RequiredPackage `json:"required_packages"`
	Schema            SchemaReport      `json:"schema"`
	Problems          []string          `json:"problems"`
	DatabaseAvailable bool              `json:"database_available"`
}

// RequiredPackage is the per-package coverage the audit insists on.
type RequiredPackage struct {
	Package string `json:"package"`
	Ran     int    `json:"ran"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
}

// SchemaReport is the schema half of the audit, as JSON.
type SchemaReport struct {
	URL                string   `json:"url"`
	Connected          bool     `json:"connected"`
	MissingMigrations  []string `json:"missing_migrations"`
	MissingExtensions  []string `json:"missing_extensions"`
	AppliedMigrations  int      `json:"applied_migrations"`
	InstalledExtension int      `json:"installed_extensions"`
}

type testCounts struct {
	ran     int
	passed  int
	failed  int
	skipped int
	// skipOutput is the concatenated output of a skipped test, which is where
	// go test puts the t.Skip message.
	skipOutput map[string]string
}

// Audit decides whether a Go test run really exercised the database.
//
// It fails on four things, in this order:
//
//  1. go test itself reported a failure.
//  2. DATABASE_URL was set but the schema was not migrated (or the required
//     extensions are missing). A reachable but empty database is a broken
//     environment; treating it as "no tests to run" is how a broken pipeline
//     reads as green.
//  3. A test skipped with DBSkipMessage while DATABASE_URL was set. That is
//     the silent skip this gate exists to catch.
//  4. A package listed in the required manifest contributed no test that
//     actually ran.
func Audit(input AuditInput) AuditResult {
	result := AuditResult{
		OK:               true,
		ExitCode:         input.ExitCode,
		RequiredPackages: []RequiredPackage{},
		Problems:         []string{},
		DatabaseSkips:    []string{},
		OtherSkips:       []string{},
	}
	databaseAvailable := input.Schema.Connected()
	result.DatabaseAvailable = databaseAvailable

	counts := make(map[string]*testCounts)
	for _, event := range input.Events {
		if event.Test == "" {
			continue
		}
		entry := counts[event.Package]
		if entry == nil {
			entry = &testCounts{skipOutput: map[string]string{}}
			counts[event.Package] = entry
		}
		switch event.Action {
		case "output", "skip", "fail", "pass", "run":
		default:
			continue
		}
		switch event.Action {
		case "output":
			entry.skipOutput[event.Test] += event.Output
		case "run":
			entry.ran++
			result.Total++
		case "pass":
			entry.passed++
			result.Passed++
		case "fail":
			entry.failed++
			result.Failed++
		case "skip":
			entry.skipped++
			result.Skipped++
		}
	}

	if input.ExitCode != 0 {
		result.Problems = append(result.Problems, fmt.Sprintf("go test exited with code %d", input.ExitCode))
	}

	if input.Schema.URL != "" {
		switch {
		case input.Schema.ConnectError != nil:
			result.Problems = append(result.Problems, fmt.Sprintf(
				"DATABASE_URL is set but the database could not be reached: %v", input.Schema.ConnectError))
		case len(input.Schema.MissingMigrations) > 0:
			result.Problems = append(result.Problems, fmt.Sprintf(
				"DATABASE_URL is set but %d migration(s) are not applied: %s",
				len(input.Schema.MissingMigrations), strings.Join(input.Schema.MissingMigrations, ", ")))
		case len(input.Schema.MissingExtensions) > 0:
			result.Problems = append(result.Problems, fmt.Sprintf(
				"DATABASE_URL is set but required extension(s) are missing: %s",
				strings.Join(input.Schema.MissingExtensions, ", ")))
		}
	}

	packages := make([]string, 0, len(counts))
	for name := range counts {
		packages = append(packages, name)
	}
	sort.Strings(packages)
	for _, name := range packages {
		entry := counts[name]
		for test, output := range entry.skipOutput {
			label := name + "." + test
			if databaseAvailable && strings.Contains(output, DBSkipMessage) {
				result.DatabaseSkips = append(result.DatabaseSkips, label)
				continue
			}
			result.OtherSkips = append(result.OtherSkips, label)
		}
	}
	sort.Strings(result.DatabaseSkips)
	sort.Strings(result.OtherSkips)

	if len(result.DatabaseSkips) > 0 {
		result.Problems = append(result.Problems, fmt.Sprintf(
			"%d database-backed test(s) skipped while DATABASE_URL was set: %s",
			len(result.DatabaseSkips), strings.Join(result.DatabaseSkips, ", ")))
	}

	for _, required := range input.RequiredPackages {
		entry := counts[required]
		report := RequiredPackage{Package: required}
		if entry != nil {
			report.Ran = entry.ran - entry.skipped
			report.Passed = entry.passed
			report.Failed = entry.failed
			report.Skipped = entry.skipped
		}
		if report.Ran <= 0 {
			result.Problems = append(result.Problems, fmt.Sprintf(
				"required database package %s ran no test (it is listed in the required manifest but every test in it skipped or none exist)", required))
		}
		result.RequiredPackages = append(result.RequiredPackages, report)
	}

	result.Schema = SchemaReport{
		URL:                input.Schema.URL,
		Connected:          databaseAvailable,
		MissingMigrations:  emptyIfNil(input.Schema.MissingMigrations),
		MissingExtensions:  emptyIfNil(input.Schema.MissingExtensions),
		AppliedMigrations:  input.Schema.AppliedMigrations,
		InstalledExtension: len(RequiredExtensions) - len(input.Schema.MissingExtensions),
	}
	result.OK = len(result.Problems) == 0
	return result
}

func emptyIfNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
