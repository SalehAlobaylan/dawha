// Command dbsweep removes every synthetic test row from a database and reports
// what it did.
//
// It exists because the fixtures were fixed before the database they had been
// quietly filling was cleaned, and a cleanup that only ever runs from inside a
// test cannot clean a database no test is running against. The logic is
// testsupport's, the same one every fixture's t.Cleanup uses, so "swept by hand"
// and "swept by a test" cannot mean different things.
//
// It is scoped by the synthetic markers in testsupport.SyntheticEmailSuffixes and
// never by a row count, so it cannot touch a seeded row however many of them
// there are, and it refuses to commit if anything reachable from a synthetic
// actor survives.
//
// Usage:
//
//	DATABASE_URL=postgres://dawha:dawha_local@localhost:55432/dawha \
//	  go run ./tools/dbsweep
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

func main() {
	os.Exit(run())
}

func run() int {
	flagDryRun := len(os.Args) > 1 && os.Args[1] == "-n"
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "dbsweep: DATABASE_URL is not set, so there is no database to clean.")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	before, err := snapshot(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbsweep: %v\n", err)
		return 2
	}
	if flagDryRun {
		reportSynthetic(ctx, databaseURL)
		return 0
	}
	report, err := testsupport.SweepSyntheticRows(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbsweep: %v\n", err)
		return 1
	}
	after, err := snapshot(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbsweep: %v\n", err)
		return 2
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbsweep: %v\n", err)
		return 2
	}
	fmt.Printf("dbsweep: %d synthetic actor(s), %d pass(es), %d orphan(s), %d deferrable foreign key(s), "+
		"%d cascade foreign key(s) the proof could not follow\n",
		report.Seeded, report.Passes, report.Orphans, report.SoftConstraints, report.SkippedCascadeConstraints)
	fmt.Println("dbsweep: before / after, by table:")
	for _, table := range testsupport.ReportedTables {
		fmt.Printf("  %-10s %8d -> %6d\n", table, before[table], after[table])
	}
	fmt.Println("dbsweep: rows removed, by table:")
	for _, table := range sortedKeys(report.DeletedPerTable) {
		if report.DeletedPerTable[table] == 0 {
			continue
		}
		fmt.Printf("  %-32s %8d\n", table, report.DeletedPerTable[table])
	}
	if file := strings.TrimSpace(os.Getenv("DBSWEEP_REPORT")); file != "" {
		if err := os.WriteFile(file, append(encoded, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "dbsweep: %v\n", err)
			return 2
		}
	}
	return 0
}

func reportSynthetic(ctx context.Context, databaseURL string) {
	left, reachable, err := testsupport.DescribeSyntheticRows(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbsweep: %v\n", err)
		return
	}
	fmt.Printf("dbsweep: %d synthetic user(s) present\n", len(left))
	for _, user := range left {
		fmt.Printf("  %s %s\n", user.ID, user.Email)
	}
	for _, table := range sortedTables(reachable) {
		fmt.Printf("  still reachable %-30s %v\n", table, reachable[table])
	}
}

func snapshot(ctx context.Context, databaseURL string) (map[string]int, error) {
	counts, err := testsupport.CountReportedTables(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return counts, nil
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedTables(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
