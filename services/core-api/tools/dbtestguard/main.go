// Command dbtestguard runs the Go test suite and refuses to call it a database
// pass unless the database was really exercised.
//
// It exists because `go test ./...` without DATABASE_URL quietly skips every
// database-backed test, and a green tick from that run means nothing. The
// guard runs the suite with the caller's arguments, streams the output so the
// developer still sees it, and then audits the result:
//
//   - a non-zero `go test` exit fails;
//   - DATABASE_URL set but the schema not migrated, or a required extension
//     missing, fails - that is a broken environment, not an empty test run;
//   - a test that skipped with testsupport.DBSkipMessage while DATABASE_URL was
//     set fails, which is the silent skip this command exists to catch;
//   - a package listed in the required manifest that ran no test fails, so a
//     deleted or emptied package cannot read as a pass.
//
// It refuses to run without DATABASE_URL, because an audit with no database to
// check is exactly the vacuous pass being designed out.
//
// Usage:
//
//	DATABASE_URL=postgres://dawha:dawha_local@localhost:55432/dawha \
//	  go run ./tools/dbtestguard -- ./...
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		migrationsDir = flag.String("migrations", "", "directory holding the db/migrations/*.sql files (default: found by walking up from the working directory)")
		requiredFile  = flag.String("require", "", "manifest of packages that must run a database-backed test (default: internal/testsupport/testdata/db-required-packages.txt)")
		auditPath     = flag.String("audit", "", "optional path for the machine-readable audit report")
		quiet         = flag.Bool("quiet", false, "suppress the streamed go test output and print only the audit")
	)
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "dbtestguard: DATABASE_URL is not set. Running the suite without a database means every database-backed test skips, which is the failure this command exists to catch.")
		return 2
	}

	root, err := repositoryRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbtestguard: %v\n", err)
		return 2
	}
	if *migrationsDir == "" {
		*migrationsDir = filepath.Join(root, "db", "migrations")
	}
	if *requiredFile == "" {
		*requiredFile = filepath.Join(root, "services", "core-api", "internal", "testsupport", "testdata", "db-required-packages.txt")
	}
	required := readPackages(*requiredFile)

	packages := flag.Args()
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	state := testsupport.InspectSchema(context.Background(), databaseURL, *migrationsDir)
	// The schema is inspected before the suite so a broken environment is
	// reported in the same place whether or not any test ran.
	exitCode, events, err := runSuite(packages, *quiet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbtestguard: %v\n", err)
		return 2
	}

	result := testsupport.Audit(testsupport.AuditInput{
		ExitCode:         exitCode,
		Events:           events,
		RequiredPackages: required,
		Schema:           state,
	})
	if !state.Connected() {
		result.OK = false
	}
	// The suite has finished, so nothing can legitimately be mid-fixture. Any
	// schema still standing is a test that did not clean up after itself in the
	// database the developer also uses.
	if state.Connected() {
		leaked, err := listFixtureSchemas()
		if err != nil {
			result.OK = false
			result.Problems = append(result.Problems, fmt.Sprintf("audit fixture schemas: %v", err))
		} else if len(leaked) > 0 {
			result.OK = false
			result.Problems = append(result.Problems, fmt.Sprintf(
				"%d fixture schema(s) survived the run, so the suite left data in the shared database: %s",
				len(leaked), strings.Join(leaked, ", ")))
		}
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbtestguard: encode audit: %v\n", err)
		return 2
	}
	if *auditPath != "" {
		if err := os.WriteFile(*auditPath, append(encoded, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "dbtestguard: write audit report: %v\n", err)
			return 2
		}
	}
	fmt.Printf("dbtestguard: %d tests, %d passed, %d failed, %d skipped (database skips %d)\n",
		result.Total, result.Passed, result.Failed, result.Skipped, len(result.DatabaseSkips))
	if !result.OK {
		fmt.Fprintln(os.Stderr, "dbtestguard: the run is NOT a valid database pass:")
		for _, problem := range result.Problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", problem)
		}
		if len(result.DatabaseSkips) > 0 {
			fmt.Fprintf(os.Stderr, "  skipped on the database gate: %s\n", strings.Join(result.DatabaseSkips, " "))
		}
		return 1
	}
	fmt.Fprintln(os.Stdout, "dbtestguard: the database suite really ran against the migrated schema")
	return 0
}

// runSuite runs `go test -json` over packages and returns the exit code plus the
// parsed events. The raw output is echoed unless quiet, so the developer still
// reads the normal test log.
func runSuite(packages []string, quiet bool) (int, []testsupport.Event, error) {
	arguments := append([]string{"test", "-json", "-count=1"}, packages...)
	command := exec.Command("go", arguments...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return 0, nil, err
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return 0, nil, err
	}
	events := []testsupport.Event{}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event testsupport.Event
		if err := json.Unmarshal(line, &event); err != nil {
			// A line that is not JSON is not a test event; pass it through
			// rather than dropping output the developer needs to see.
			if !quiet {
				fmt.Println(strings.TrimRight(string(line), "\n"))
			}
			continue
		}
		if event.Action == "output" && !quiet {
			fmt.Print(strings.TrimRight(event.Output, "\n") + "\n")
		}
		events = append(events, event)
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	if scanErr != nil {
		return 0, nil, scanErr
	}
	if waitErr != nil {
		var exitError *exec.ExitError
		if ok := asExitError(waitErr, &exitError); ok {
			return exitError.ExitCode(), events, nil
		}
		return 0, nil, waitErr
	}
	return 0, events, nil
}

func asExitError(err error, target **exec.ExitError) bool {
	exitError, ok := err.(*exec.ExitError)
	if ok {
		*target = exitError
	}
	return ok
}

// listFixtureSchemas reports the acceptance fixtures that outlived the suite.
func listFixtureSchemas() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return testsupport.ListFixtureSchemas(ctx, strings.TrimSpace(os.Getenv("DATABASE_URL")))
}

func repositoryRoot() (string, error) {
	working, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return testsupport.FindRepositoryRoot(working)
}

func readPackages(path string) []string {
	contents, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbtestguard: read required manifest %s: %v\n", path, err)
		return nil
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
