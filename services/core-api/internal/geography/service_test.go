package geography

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateInput(t *testing.T) {
	input, err := validateInput(MapInput{FromYear: 1100, ToYear: 1250, Status: "disputed"})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.Status != "disputed" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
	if input.Limit != MaxMapFeatures {
		t.Fatalf("default limit = %d, want the map bound %d", input.Limit, MaxMapFeatures)
	}
	if _, err := validateInput(MapInput{FromYear: 1300, ToYear: 1200}); err != ErrValidation {
		t.Fatalf("expected range validation error, got %v", err)
	}
	if _, err := validateInput(MapInput{Status: "invented"}); err != ErrValidation {
		t.Fatalf("expected status validation error, got %v", err)
	}
	if _, err := validateInput(MapInput{Limit: -1}); err != ErrValidation {
		t.Fatalf("expected a negative limit to be refused, got %v", err)
	}
	if _, err := validateInput(MapInput{Limit: MaxMapFeatures + 1}); err != ErrValidation {
		t.Fatalf("expected a limit above the bound to be refused, got %v", err)
	}
	// A viewport is a rectangle or nothing. An inverted or out-of-range one is a
	// filter that would answer "no features", which is indistinguishable from a map
	// that is empty because of the policy.
	for name, viewport := range map[string]Viewport{
		"inverted longitude":   {MinLongitude: 30, MaxLongitude: 10, MinLatitude: 0, MaxLatitude: 10},
		"inverted latitude":    {MinLongitude: 10, MaxLongitude: 30, MinLatitude: 30, MaxLatitude: 10},
		"longitude past 180":   {MinLongitude: 10, MaxLongitude: 190, MinLatitude: 0, MaxLatitude: 10},
		"latitude past 90":     {MinLongitude: 10, MaxLongitude: 30, MinLatitude: 0, MaxLatitude: 100},
		"longitude under -180": {MinLongitude: -190, MaxLongitude: 10, MinLatitude: 0, MaxLatitude: 10},
	} {
		if _, err := validateInput(MapInput{Viewport: &viewport}); err != ErrValidation {
			t.Fatalf("%s: got %v, want %v", name, err, ErrValidation)
		}
	}
	if _, err := validateInput(MapInput{Viewport: &Viewport{MinLongitude: 10, MaxLongitude: 30, MinLatitude: 0, MaxLatitude: 10}}); err != nil {
		t.Fatalf("a well-formed viewport was refused: %v", err)
	}
}

// The map filters used to be a Go function applied to every row the four feature
// queries returned, which made their edge cases unit-testable without a database.
// They are SQL now, so the only honest way to pin them is to let PostgreSQL evaluate
// the predicate. These are the cases the Go filter had, including the two a plain
// comparison gets wrong: a feature with no dates is kept under any year filter,
// because the platform does not know when the thing happened, and a region is drawn
// with a status derived from its certainty rather than read from a column.
func TestMapFilterPredicates(t *testing.T) {
	pool := openPredicatePool(t)
	ctx := context.Background()

	t.Run("year filter keeps the overlap rule", func(t *testing.T) {
		for _, testCase := range []struct {
			label   string
			from    string
			to      string
			fromIn  int
			toIn    int
			keep    bool
			because string
		}{
			{label: "overlaps the window", from: "1100-01-01", to: "1150-01-01", fromIn: 1120, toIn: 1140, keep: true, because: "the period covers the window"},
			{label: "starts after the window", from: "1300-01-01", to: "1350-01-01", fromIn: 1120, toIn: 1140, keep: false, because: "the period begins after the window ends"},
			{label: "ended before the window", from: "1000-01-01", to: "1050-01-01", fromIn: 1120, toIn: 1140, keep: false, because: "the period ended before the window begins"},
			{label: "no dates at all", from: "NULL", to: "NULL", fromIn: 1120, toIn: 1140, keep: true, because: "the platform does not know when it happened"},
			{label: "no start, end after the window", from: "NULL", to: "1150-01-01", fromIn: 1120, toIn: 1140, keep: true, because: "the period reaches into the window"},
			{label: "no start, end before the window", from: "NULL", to: "1050-01-01", fromIn: 1120, toIn: 1140, keep: false, because: "the period ended before the window begins"},
			{label: "no end, start before the window", from: "1000-01-01", to: "NULL", fromIn: 1120, toIn: 1140, keep: true, because: "the period began before the window"},
			{label: "no end, start after the window", from: "1300-01-01", to: "NULL", fromIn: 1120, toIn: 1140, keep: false, because: "the period begins after the window ends"},
			{label: "no filter keeps an unrelated period", from: "1300-01-01", to: "1350-01-01", keep: true, because: "no year filter is no year filter"},
			{label: "an upper bound alone keeps a later period", from: "1300-01-01", to: "1350-01-01", toIn: 1400, keep: true, because: "the period starts before the bound"},
			{label: "an upper bound alone keeps an earlier period", from: "0800-01-01", to: "0850-01-01", toIn: 1400, keep: true, because: "the period starts before the bound"},
			{label: "a lower bound alone drops an earlier period", from: "0800-01-01", to: "0850-01-01", fromIn: 1400, keep: false, because: "the period ended before the bound"},
		} {
			filters := newMapFilters(MapInput{FromYear: testCase.fromIn, ToYear: testCase.toIn}, visibility.NewParams())
			predicate := filters.timePredicate("test.from_date", "test.to_date")
			kept := evaluatePredicate(t, ctx, pool, filters, predicate, fmt.Sprintf(
				"SELECT %s AS kept FROM (VALUES (%s::date, %s::date)) AS test(from_date, to_date)", predicate, quoteLiteral(testCase.from), quoteLiteral(testCase.to)))
			if kept != testCase.keep {
				t.Fatalf("%s: kept=%v, want %v because %s\npredicate: %s", testCase.label, kept, testCase.keep, testCase.because, predicate)
			}
		}
	})

	t.Run("status filter compares the drawn status", func(t *testing.T) {
		drawn := "(CASE WHEN test.certainty = 'disputed' THEN 'disputed' ELSE 'interpreted' END)"
		for _, testCase := range []struct {
			certainty string
			status    string
			keep      bool
			because   string
		}{
			{certainty: "disputed", status: "disputed", keep: true, because: "a disputed region is drawn as disputed"},
			{certainty: "approximate", status: "disputed", keep: false, because: "an approximate region is not drawn as disputed"},
			{certainty: "approximate", status: "interpreted", keep: true, because: "an approximate region is drawn as interpreted"},
			{certainty: "disputed", status: "interpreted", keep: false, because: "a disputed region is not drawn as interpreted"},
			{certainty: "disputed", status: "", keep: true, because: "no status filter keeps both layers"},
			{certainty: "approximate", status: "", keep: true, because: "no status filter keeps both layers"},
			{certainty: "approximate", status: "documented", keep: false, because: "a region is never drawn as documented"},
		} {
			filters := newMapFilters(MapInput{Status: testCase.status}, visibility.NewParams())
			predicate := filters.statusPredicate(drawn)
			kept := evaluatePredicate(t, ctx, pool, filters, predicate, fmt.Sprintf(
				"SELECT %s AS kept FROM (VALUES ('%s')) AS test(certainty)", predicate, testCase.certainty))
			if kept != testCase.keep {
				t.Fatalf("certainty=%s status=%q: kept=%v, want %v because %s\npredicate: %s",
					testCase.certainty, testCase.status, kept, testCase.keep, testCase.because, predicate)
			}
		}
	})

	t.Run("place filter excludes a layer with no place", func(t *testing.T) {
		// A historical region has no place of its own. A place filter has to exclude
		// it, which is what a NULL place reference against a uuid does, and the
		// unset filter has to keep it.
		scoped := newMapFilters(MapInput{PlaceID: "20000000-0000-0000-0000-000000000002"}, visibility.NewParams())
		predicate := scoped.placePredicate("test.place_id")
		for _, testCase := range []struct {
			placeID string
			keep    bool
		}{
			{placeID: "20000000-0000-0000-0000-000000000002", keep: true},
			{placeID: "20000000-0000-0000-0000-000000000001", keep: false},
			{placeID: "NULL", keep: false},
		} {
			kept := evaluatePredicate(t, ctx, pool, scoped, predicate, fmt.Sprintf(
				"SELECT %s AS kept FROM (VALUES (%s::uuid)) AS test(place_id)", predicate, quoteLiteral(testCase.placeID)))
			if kept != testCase.keep {
				t.Fatalf("place %s: kept=%v, want %v", testCase.placeID, kept, testCase.keep)
			}
		}
		open := newMapFilters(MapInput{}, visibility.NewParams())
		openPredicate := open.placePredicate("test.place_id")
		kept := evaluatePredicate(t, ctx, pool, open, openPredicate,
			"SELECT "+openPredicate+" AS kept FROM (VALUES (NULL::uuid)) AS test(place_id)")
		if !kept {
			t.Fatal("an unset place filter excluded a layer that has no place")
		}
	})

	t.Run("viewport filter keeps only the box", func(t *testing.T) {
		viewport := &Viewport{MinLongitude: 10, MinLatitude: 10, MaxLongitude: 20, MaxLatitude: 15}
		for _, testCase := range []struct {
			longitude float64
			latitude  float64
			keep      bool
		}{
			{longitude: 15, latitude: 12, keep: true},
			{longitude: 10, latitude: 10, keep: true},
			{longitude: 20, latitude: 15, keep: true},
			{longitude: 9.9, latitude: 12, keep: false},
			{longitude: 15, latitude: 15.1, keep: false},
		} {
			filters := newMapFilters(MapInput{Viewport: viewport}, visibility.NewParams())
			predicate := filters.viewportPredicate("test.longitude", "test.latitude")
			kept := evaluatePredicate(t, ctx, pool, filters, predicate, fmt.Sprintf(
				"SELECT %s AS kept FROM (VALUES (%f, %f)) AS test(longitude, latitude)", predicate, testCase.longitude, testCase.latitude))
			if kept != testCase.keep {
				t.Fatalf("(%f, %f): kept=%v, want %v", testCase.longitude, testCase.latitude, kept, testCase.keep)
			}
		}
		// No viewport means no comparison at all, so the planner is not asked to
		// evaluate a geometry test the caller did not ask for.
		open := newMapFilters(MapInput{}, visibility.NewParams())
		if predicate := open.viewportPredicate("test.longitude", "test.latitude"); predicate != "TRUE" {
			t.Fatalf("an unset viewport = %s, want TRUE", predicate)
		}
	})
}

func openPredicatePool(t *testing.T) *pgxpool.Pool {
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

// evaluatePredicate lets the database decide one filter expression over a one-row
// VALUES list. The filters are SQL fragments built from a visibility.Params, so
// evaluating them in Go would mean re-implementing the very expression under test.
//
// A NULL answer counts as not kept, because that is how a WHERE clause reads it. The
// place filter is the case that produces one: a layer with no place of its own makes
// the predicate NULL rather than false, and a row whose predicate is NULL is not
// selected. Reading that as "kept" here would test a different expression than the
// one the map runs.
func evaluatePredicate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, filters mapFilters, predicate, statement string) bool {
	t.Helper()
	var kept *bool
	if err := pool.QueryRow(ctx, statement, filters.params.Args()...).Scan(&kept); err != nil {
		t.Fatalf("evaluate %s: %v", predicate, err)
	}
	return kept != nil && *kept
}

// quoteLiteral turns a fixture value into a SQL literal. The filter tests build
// their input rows from a VALUES list, and an unquoted "20000000-0000-0000-0000-..." or
// "1100-01-01" is arithmetic to PostgreSQL rather than a uuid or a date.
func quoteLiteral(value string) string {
	if value == "NULL" {
		return value
	}
	return "'" + value + "'"
}

func atoi(t *testing.T, value string) int {
	t.Helper()
	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}
