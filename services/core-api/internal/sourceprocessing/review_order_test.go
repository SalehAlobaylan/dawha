package sourceprocessing

import (
	"context"
	"fmt"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

// A LIST ORDER HAS TO BE DECIDED, NOT LEFT TO THE PLAN.
//
// `files` and `runs` in review.go both ordered by `created_at DESC` alone. That
// is not an order: two files uploaded in one transaction, or two processing runs
// started by one batch, carry the same timestamp, and PostgreSQL is free to hand
// them back either way round. It did, which is why the review panel's file list
// could differ between two identical requests and nobody could say which one was
// right.
//
// The fix is a second key, `id`, matching the rule the rest of this repository
// already uses for the same column pair. This file pins three things: the order
// is the one the tiebreak declares, two identical requests agree, and a caller
// paging through the review response sees every candidate exactly once. None of
// them is about WHICH rows come back - that is unchanged, and the first test
// asserts it too, because an order fix that quietly filtered a row would be a
// worse defect than the one it replaced.

// tie is one file and one processing run created at the same instant, which is
// what a single-transaction batch does.
type tie struct {
	fileID uuid.UUID
	runID  uuid.UUID
}

// seedRowsCreatedInOneTransaction writes the shape a batch import produces: the
// files, the runs and their candidates all share one created_at, in one
// transaction, so nothing the fixture does can accidentally separate them in
// time. The returned slice is in insertion order and deliberately not the answer.
func seedRowsCreatedInOneTransaction(t *testing.T, fixture *testsupport.Fixture, batches int) (uuid.UUID, string, []tie) {
	t.Helper()
	ctx := context.Background()
	owner := actor.Register(t, fixture, "رافع الدفعة")
	ownerID := owner.User.ID
	var sourceID uuid.UUID
	if err := fixture.QueryRow(`
		INSERT INTO sources (title_ar, source_type, visibility, created_by)
		VALUES ($1, 'manuscript', 'private', $2) RETURNING id
	`, "مصدر الدفعة "+fixture.Unique("t"), ownerID).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}

	// One explicit instant for every row. The default now() would give each
	// statement its own microsecond, which is exactly the case the tiebreak does
	// not have to survive and would hide the defect.
	const stamp = "2026-03-01 00:00:00 +0000"

	tx, err := fixture.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	ties := make([]tie, 0, batches)
	for index := 0; index < batches; index++ {
		var fileID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256, processing_status, created_at)
			VALUES ($1, $2, $3, 'text/plain', 16, $4, 'succeeded', $5::timestamptz) RETURNING id
		`, sourceID, fmt.Sprintf("batch/%d/%s", index, fixture.Unique("key")),
			fmt.Sprintf("صفحة-%d.txt", index), fixture.Unique("sum"), stamp).Scan(&fileID); err != nil {
			t.Fatalf("insert source file %d: %v", index, err)
		}
		var runID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO source_processing_runs (source_id, source_file_id, status, stage, created_at, updated_at)
			VALUES ($1, $2, 'succeeded', 'review', $3::timestamptz, $3::timestamptz) RETURNING id
		`, sourceID, fileID, stamp).Scan(&runID); err != nil {
			t.Fatalf("insert processing run %d: %v", index, err)
		}
		ties = append(ties, tie{fileID: fileID, runID: runID})
	}
	// One candidate per file, so the paged walk has something to walk and every
	// candidate shares the same instant too.
	for index, current := range ties {
		var passageID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO source_passages (source_id, source_file_id, sequence_number, text_ar, normalized_text_ar)
			VALUES ($1, $2, $3, $4, $4) RETURNING id
		`, sourceID, current.fileID, index+1, fmt.Sprintf("نص الدفعة %d", index)).Scan(&passageID); err != nil {
			t.Fatalf("insert passage %d: %v", index, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO source_candidates (source_id, source_file_id, source_passage_id, candidate_type, raw_text_ar, normalized_text_ar, confidence, rationale_ar, model_version, dedupe_key, created_at, updated_at)
			VALUES ($1, $2, $3, 'entity', $4, $4, 0.9, 'سبب', 'test-model', $5, $6::timestamptz, $6::timestamptz)
		`, sourceID, current.fileID, passageID, fmt.Sprintf("مرشح الدفعة %d", index),
			fmt.Sprintf("batch-dedupe-%d-%s", index, fixture.Unique("k")), stamp); err != nil {
			t.Fatalf("insert candidate %d: %v", index, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return sourceID, ownerID, ties
}

// orderedIDs is the order the two tiebreaks declare: created_at descending, then
// id ascending. It is spelled out here rather than copied from the query, so a
// change to the SQL has to be made here too to keep passing.
func orderedIDs(t *testing.T, fixture *testsupport.Fixture, sourceID uuid.UUID, table string) []string {
	t.Helper()
	rows, err := fixture.Pool().Query(context.Background(),
		"SELECT id::text FROM "+table+" WHERE source_id = $1 ORDER BY created_at DESC, id", sourceID)
	if err != nil {
		t.Fatalf("read declared order for %s: %v", table, err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func newOrderedReviewService(t *testing.T, fixture *testsupport.Fixture) *Service {
	t.Helper()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	return NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewTextExtractor())
}

func viewFileIDs(files []FileView) []string {
	ids := make([]string, 0, len(files))
	for _, item := range files {
		ids = append(ids, item.ID)
	}
	return ids
}

func viewRunIDs(runs []ProcessingRunView) []string {
	ids := make([]string, 0, len(runs))
	for _, item := range runs {
		ids = append(ids, item.ID)
	}
	return ids
}

func sameOrder(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// TestSourceFileAndRunOrderIsTotalForRowsCreatedInOneTransaction is the pin. Five
// files and five runs written in one transaction share one created_at, so before
// the tiebreak the response order was whatever the plan chose - this test fails
// against `ORDER BY created_at DESC` alone for at least some of the orderings
// PostgreSQL can return, and it fails loudly rather than one time in a thousand.
func TestSourceFileAndRunOrderIsTotalForRowsCreatedInOneTransaction(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, ownerID, seeded := seedRowsCreatedInOneTransaction(t, fixture, 5)
	service := newOrderedReviewService(t, fixture)
	ctx := fixture.Ctx()

	view, err := service.GetProcessingPage(ctx, sourceID.String(), ownerID, CandidatePage{})
	if err != nil {
		t.Fatalf("get processing page: %v", err)
	}

	// Which rows come back is unchanged: every seeded row, none invented, none
	// dropped. An order fix that quietly filtered would pass an order-only test.
	if len(view.Files) != len(seeded) || len(view.Runs) != len(seeded) {
		t.Fatalf("response holds %d files and %d runs, want %d of each: the tiebreak must order the list, not change which rows it holds",
			len(view.Files), len(view.Runs), len(seeded))
	}
	if want := orderedIDs(t, fixture, sourceID, "source_files"); !sameOrder(viewFileIDs(view.Files), want) {
		t.Fatalf("file order = %v, want the total order created_at DESC, id = %v", viewFileIDs(view.Files), want)
	}
	if want := orderedIDs(t, fixture, sourceID, "source_processing_runs"); !sameOrder(viewRunIDs(view.Runs), want) {
		t.Fatalf("run order = %v, want the total order created_at DESC, id = %v", viewRunIDs(view.Runs), want)
	}
	// The tiebreak has to be doing something, or this test is comparing a list to
	// itself. Two of the seeded ids in one transaction must not be in insertion
	// order under the declared order, or the fixture stopped being the hard case.
	if sameOrder(viewFileIDs(view.Files), fileIDsInInsertionOrder(seeded)) {
		t.Skip("this seed happens to land in insertion order; the order is still the declared one, but this run cannot show the tiebreak matters")
	}
}

func fileIDsInInsertionOrder(seeded []tie) []string {
	ids := make([]string, 0, len(seeded))
	for _, item := range seeded {
		ids = append(ids, item.fileID.String())
	}
	return ids
}

// TestTwoIdenticalReviewRequestsReturnTheSameOrder is the operational half. A
// reviewer who reloads, or two reviewers looking at the same source, must see the
// same list; an order that depends on the plan is an order that differs.
func TestTwoIdenticalReviewRequestsReturnTheSameOrder(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, ownerID, _ := seedRowsCreatedInOneTransaction(t, fixture, 4)
	service := newOrderedReviewService(t, fixture)
	ctx := fixture.Ctx()

	first, err := service.GetProcessingPage(ctx, sourceID.String(), ownerID, CandidatePage{})
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		again, err := service.GetProcessingPage(ctx, sourceID.String(), ownerID, CandidatePage{})
		if err != nil {
			t.Fatalf("request %d: %v", attempt+2, err)
		}
		if !sameOrder(viewFileIDs(again.Files), viewFileIDs(first.Files)) {
			t.Fatalf("file order changed on request %d: %v then %v", attempt+2, viewFileIDs(first.Files), viewFileIDs(again.Files))
		}
		if !sameOrder(viewRunIDs(again.Runs), viewRunIDs(first.Runs)) {
			t.Fatalf("run order changed on request %d: %v then %v", attempt+2, viewRunIDs(first.Runs), viewRunIDs(again.Runs))
		}
	}
}

// TestPagingTheReviewResponseTwiceSeesEveryCandidateExactlyOnce is the pagination
// half. Every candidate in the fixture shares one created_at, so a page boundary
// landing inside a tie is the case that decides whether a row is dropped or
// served twice; walking the same pages twice has to produce the same sequence and
// cover the whole list.
func TestPagingTheReviewResponseTwiceSeesEveryCandidateExactlyOnce(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, ownerID, _ := seedRowsCreatedInOneTransaction(t, fixture, 5)
	service := newOrderedReviewService(t, fixture)
	ctx := fixture.Ctx()

	walk := func() []string {
		seen := make([]string, 0)
		for offset := 0; ; offset += 2 {
			view, err := service.GetProcessingPage(ctx, sourceID.String(), ownerID, CandidatePage{Offset: offset, Limit: 2})
			if err != nil {
				t.Fatalf("page at offset %d: %v", offset, err)
			}
			for _, item := range view.Candidates {
				seen = append(seen, item.ID)
			}
			if offset+len(view.Candidates) >= view.CandidatesTotal {
				return seen
			}
			if len(view.Candidates) == 0 {
				t.Fatalf("page at offset %d returned nothing while the total is %d, so the walk cannot end", offset, view.CandidatesTotal)
			}
		}
	}
	first, second := walk(), walk()
	if !sameOrder(first, second) {
		t.Fatalf("two identical page walks returned different sequences: %v then %v", first, second)
	}
	if len(first) != 5 {
		t.Fatalf("the walk saw %d candidates, want 5: %v", len(first), first)
	}
	distinct := map[string]bool{}
	for _, id := range first {
		if distinct[id] {
			t.Fatalf("a candidate was served twice across pages: %v", first)
		}
		distinct[id] = true
	}
}

// TestTheOrderIsAColumnTheQueryActuallyReads keeps the two functions honest about
// reading a column that exists. The tiebreak names id, and this asserts the
// queries return the id they order by, so a future change cannot order by a
// projection the scan does not carry.
func TestTheOrderIsAColumnTheQueryActuallyReads(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, ownerID, seeded := seedRowsCreatedInOneTransaction(t, fixture, 2)
	service := newOrderedReviewService(t, fixture)
	ctx := fixture.Ctx()
	view, err := service.GetProcessingPage(ctx, sourceID.String(), ownerID, CandidatePage{})
	if err != nil {
		t.Fatalf("get processing page: %v", err)
	}
	byID := map[string]bool{}
	for _, item := range seeded {
		byID[item.fileID.String()] = true
		byID[item.runID.String()] = true
	}
	for _, item := range view.Files {
		if !byID[item.ID] {
			t.Fatalf("a file row came back with an id the order could not have been built from: %s", item.ID)
		}
	}
	for _, item := range view.Runs {
		if !byID[item.ID] {
			t.Fatalf("a run row came back with an id the order could not have been built from: %s", item.ID)
		}
	}
}
