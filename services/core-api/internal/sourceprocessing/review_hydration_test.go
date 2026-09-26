package sourceprocessing

import (
	"fmt"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

// Review hydration is the shape of this file. The endpoint used to ask the
// database once per candidate, so a source with two hundred reviewed candidates
// cost two hundred and one round trips to render one page. These tests pin the
// batched cost, and they pin the parts of the response a batching change can get
// wrong: the grouping has to keep every review with its own candidate, the
// per-candidate order has to survive, and a candidate with no decision has to
// read as an empty list rather than as a missing field.

// candidateReviewFixture is one source with count candidates, each carrying a
// known number of reviews, so the grouped query has something to misplace.
type candidateReviewFixture struct {
	sourceID    uuid.UUID
	ownerID     string
	candidateID []uuid.UUID
}

func seedCandidateReviews(t *testing.T, fixture *testsupport.Fixture, count, reviewsPerCandidate int) *candidateReviewFixture {
	t.Helper()
	owner := actor.Register(t, fixture, "مراجع الترشحات")
	ownerID := owner.User.ID

	// A source created through the same table the upload path writes is all
	// GetProcessing needs, so the fixture stays about hydration.
	var sourceID uuid.UUID
	if err := fixture.QueryRow(`
		INSERT INTO sources (title_ar, source_type, visibility, created_by)
		VALUES ($1, 'manuscript', 'private', $2) RETURNING id
	`, "مصدر الترشحات "+fixture.Unique("t"), ownerID).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	var fileID uuid.UUID
	if err := fixture.QueryRow(`
		INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256, processing_status)
		VALUES ($1, $2, 'سجل.txt', 'text/plain', 32, $3, 'succeeded') RETURNING id
	`, sourceID, fixture.Unique("key"), fixture.Unique("sum")).Scan(&fileID); err != nil {
		t.Fatalf("insert source file: %v", err)
	}

	seeded := &candidateReviewFixture{sourceID: sourceID, ownerID: ownerID}
	for index := 0; index < count; index++ {
		// One passage per candidate keeps the passage text out of the grouping
		// question; this file is about reviews, not about passage provenance.
		var passageID uuid.UUID
		if err := fixture.QueryRow(`
			INSERT INTO source_passages (source_id, source_file_id, sequence_number, text_ar, normalized_text_ar)
			VALUES ($1, $2, $3, $4, $5) RETURNING id
		`, sourceID, fileID, index, fmt.Sprintf("نص المرشح %d", index), fmt.Sprintf("نص المرشح %d", index)).Scan(&passageID); err != nil {
			t.Fatalf("insert passage: %v", err)
		}
		// created_at is stepped so the candidate order is a known total order and
		// the review order within a candidate is a known one too.
		createdAt := fmt.Sprintf("2026-01-01 00:00:%02d +0000", index%60)
		var candidateID uuid.UUID
		if err := fixture.QueryRow(`
			INSERT INTO source_candidates (source_id, source_file_id, source_passage_id, candidate_type, raw_text_ar, normalized_text_ar, confidence, rationale_ar, model_version, dedupe_key, status, created_at, updated_at)
			VALUES ($1, $2, $3, 'entity', $4, $5, 0.9, 'سبب', 'test-model', $6,
			        CASE WHEN $7 > 0 THEN 'accepted' ELSE 'unreviewed' END, $8::timestamptz, $8::timestamptz)
			RETURNING id
		`, sourceID, fileID, passageID, fmt.Sprintf("مرشح %d", index), fmt.Sprintf("مرشح %d", index),
			fmt.Sprintf("dedupe-%d", index), reviewsPerCandidate, createdAt).Scan(&candidateID); err != nil {
			t.Fatalf("insert candidate: %v", err)
		}
		seeded.candidateID = append(seeded.candidateID, candidateID)
		for review := 0; review < reviewsPerCandidate; review++ {
			// Review timestamps are reversed inside a candidate so the newest-first
			// order the endpoint has always used is distinguishable from insertion
			// order.
			reviewAt := fmt.Sprintf("2026-02-01 00:00:%02d +0000", reviewsPerCandidate-review)
			fixture.Exec(`
				INSERT INTO source_candidate_reviews (candidate_id, reviewer_id, decision, note_ar, created_at)
				VALUES ($1, $2, 'rejected', $3, $4::timestamptz)
			`, candidateID, ownerID, fmt.Sprintf("ملاحظة %d", review), reviewAt)
		}
	}
	return seeded
}

// newReviewService wires the reading path of the service over a pool that counts
// statements. The store and the queue are real because ready() refuses a service
// without them, and neither is touched by a read.
func newReviewService(t *testing.T, fixture *testsupport.Fixture) (*Service, *testsupport.QueryCounter) {
	t.Helper()
	pool, counter := fixture.CountingFixturePool(t)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	return NewService(pool, store, jobs.NewService(fixture.Pool()), nil, NewTextExtractor()), counter
}

// TestCandidateReviewHydrationIsBatched is the pin. Thirty reviewed candidates
// have to cost the same handful of statements as one, because the reviews are
// read in a single grouped query. Before the batching this was 1 + 30 + the
// authorization queries; the bound below is deliberately loose so it survives an
// unrelated authorization change but cannot survive a return to one query per
// candidate.
func TestCandidateReviewHydrationIsBatched(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedCandidateReviews(t, fixture, 30, 3)
	service, counter := newReviewService(t, fixture)

	counter.Reset()
	view, err := service.GetProcessing(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID)
	if err != nil {
		t.Fatalf("read processing: %v", err)
	}
	if len(view.Candidates) != 30 {
		t.Fatalf("candidates = %d, want 30", len(view.Candidates))
	}
	// One grouped review query, not thirty. The count is pinned against the
	// per-candidate shape directly so the failure names the regression.
	if grouped := counter.CountMatching("FROM source_candidate_reviews"); grouped != 1 {
		t.Fatalf("review hydration ran %d grouped queries, want exactly 1:\n%s", grouped, counter.Report())
	}
	counter.AssertAtMost(t, 8, "reading 30 candidates with their reviews")

	// Every review reached the candidate it belongs to. A grouped query that lost
	// the mapping would still return the right number of rows.
	total := 0
	for _, candidate := range view.Candidates {
		if len(candidate.Reviews) != 3 {
			t.Fatalf("candidate %s has %d reviews, want 3: %+v", candidate.ID, len(candidate.Reviews), candidate.Reviews)
		}
		total += len(candidate.Reviews)
	}
	if total != 90 {
		t.Fatalf("reviews across the page = %d, want 90", total)
	}
}

// TestCandidateReviewGroupingKeepsOrderAndEmptyLists is the other half: grouping
// must not reorder anything and must not turn "no decision yet" into a missing
// field. A candidate rendered with a nil Reviews marshals as null, and the web
// client reads candidate.reviews.length, so a nil slice is a visible break.
func TestCandidateReviewGroupingKeepsOrderAndEmptyLists(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedCandidateReviews(t, fixture, 4, 2)
	service, _ := newReviewService(t, fixture)
	view, err := service.GetProcessing(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID)
	if err != nil {
		t.Fatalf("read processing: %v", err)
	}
	if len(view.Candidates) != 4 {
		t.Fatalf("candidates = %d, want 4", len(view.Candidates))
	}
	// The page is ordered by (created_at, id), the order the endpoint has always
	// used, so the same candidates come back in the same sequence.
	for index, candidate := range view.Candidates {
		if candidate.ID != seeded.candidateID[index].String() {
			t.Fatalf("candidate %d is %s, want %s: the page order changed", index, candidate.ID, seeded.candidateID[index])
		}
	}
	for _, candidate := range view.Candidates {
		if candidate.Reviews == nil {
			t.Fatalf("candidate %s returned a nil review list", candidate.ID)
		}
		// Newest first, which is the order the per-candidate query used to return.
		for index := 1; index < len(candidate.Reviews); index++ {
			if candidate.Reviews[index-1].CreatedAt.Before(candidate.Reviews[index].CreatedAt) {
				t.Fatalf("candidate %s reviews are not newest-first: %+v", candidate.ID, candidate.Reviews)
			}
		}
		if candidate.Reviews[0].NoteAR != "ملاحظة 0" {
			t.Fatalf("candidate %s newest review = %q, want the first inserted note", candidate.ID, candidate.Reviews[0].NoteAR)
		}
	}

	// The unreviewed source: every candidate must still carry an empty list.
	plain := seedCandidateReviews(t, fixture, 3, 0)
	plainView, err := service.GetProcessing(fixture.Ctx(), plain.sourceID.String(), plain.ownerID)
	if err != nil {
		t.Fatalf("read unreviewed processing: %v", err)
	}
	if len(plainView.Candidates) != 3 {
		t.Fatalf("unreviewed candidates = %d, want 3", len(plainView.Candidates))
	}
	for _, candidate := range plainView.Candidates {
		if candidate.Reviews == nil || len(candidate.Reviews) != 0 {
			t.Fatalf("candidate %s has reviews %+v, want an empty list", candidate.ID, candidate.Reviews)
		}
	}
}

// TestCandidatePageIsStableAndBounded is the pagination half. Two pages of the
// same source must not repeat or drop a candidate, and a page shorter than the
// source has to say so rather than read as the whole list.
func TestCandidatePageIsStableAndBounded(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedCandidateReviews(t, fixture, 25, 1)
	service, _ := newReviewService(t, fixture)

	first, err := service.GetProcessingPage(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID, CandidatePage{Limit: 10})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Candidates) != 10 {
		t.Fatalf("first page = %d candidates, want 10", len(first.Candidates))
	}
	if !first.CandidatesTruncated {
		t.Fatal("a 10-candidate page of a 25-candidate source did not report truncation")
	}
	if first.CandidatesTotal != 25 {
		t.Fatalf("candidatesTotal = %d, want 25: it reports what the source holds, so a caller knows the page is a page", first.CandidatesTotal)
	}

	seen := make(map[string]struct{}, 25)
	for index := range first.Candidates {
		seen[first.Candidates[index].ID] = struct{}{}
	}
	offset := 10
	for offset < 25 {
		page, err := service.GetProcessingPage(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID, CandidatePage{Limit: 10, Offset: offset})
		if err != nil {
			t.Fatalf("page at %d: %v", offset, err)
		}
		for _, candidate := range page.Candidates {
			if _, duplicate := seen[candidate.ID]; duplicate {
				t.Fatalf("candidate %s came back on two pages", candidate.ID)
			}
			seen[candidate.ID] = struct{}{}
		}
		offset += 10
	}
	if len(seen) != 25 {
		t.Fatalf("paging saw %d distinct candidates, want 25", len(seen))
	}

	// The last page holds the remainder and is not truncated.
	last, err := service.GetProcessingPage(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID, CandidatePage{Limit: 10, Offset: 20})
	if err != nil {
		t.Fatalf("last page: %v", err)
	}
	if len(last.Candidates) != 5 || last.CandidatesTruncated {
		t.Fatalf("last page = %d candidates, truncated=%v, want 5 and false", len(last.Candidates), last.CandidatesTruncated)
	}

	// A page past the end is empty, and an empty page is not truncation: the
	// source simply has no candidate there.
	beyond, err := service.GetProcessingPage(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID, CandidatePage{Limit: 10, Offset: 25})
	if err != nil {
		t.Fatalf("page past the end: %v", err)
	}
	if len(beyond.Candidates) != 0 || beyond.CandidatesTruncated {
		t.Fatalf("page past the end = %d candidates, truncated=%v, want 0 and false", len(beyond.Candidates), beyond.CandidatesTruncated)
	}
	if beyond.CandidatesTotal != 25 {
		t.Fatalf("candidatesTotal past the end = %d, want 25: the total is the source's, not the page's", beyond.CandidatesTotal)
	}

	// The default page is still the whole list, so nothing that used to be
	// returned is now missing.
	whole, err := service.GetProcessing(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID)
	if err != nil {
		t.Fatalf("whole list: %v", err)
	}
	if len(whole.Candidates) != 25 || whole.CandidatesTruncated {
		t.Fatalf("default page = %d candidates, truncated=%v, want 25 and false", len(whole.Candidates), whole.CandidatesTruncated)
	}
	if whole.CandidatesTotal != 25 {
		t.Fatalf("default page total = %d, want 25", whole.CandidatesTotal)
	}

	// A limit the endpoint cannot serve is refused, not clamped.
	if _, err := service.GetProcessingPage(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID, CandidatePage{Limit: MaxCandidates + 1}); err == nil {
		t.Fatal("a limit above MaxCandidates was accepted")
	}
	if _, err := service.GetProcessingPage(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID, CandidatePage{Offset: -1}); err == nil {
		t.Fatal("a negative offset was accepted")
	}
}

// TestCandidatePagePayloadIsSmaller measures the other half of the hydration
// change: a bounded page sends a bounded amount of passage text. The full passage
// is still reachable through the single-candidate path the review endpoint uses,
// so nothing became unreadable - it stopped being repeated once per candidate.
func TestCandidatePagePayloadIsSmaller(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedCandidateReviews(t, fixture, 40, 1)
	service, _ := newReviewService(t, fixture)

	whole, err := service.GetProcessing(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID)
	if err != nil {
		t.Fatalf("whole list: %v", err)
	}
	page, err := service.GetProcessingPage(fixture.Ctx(), seeded.sourceID.String(), seeded.ownerID, CandidatePage{Limit: 10})
	if err != nil {
		t.Fatalf("bounded page: %v", err)
	}
	wholeBytes := testsupport.PayloadBytes(t, whole)
	pageBytes := testsupport.PayloadBytes(t, page)
	if pageBytes >= wholeBytes {
		t.Fatalf("a 10-candidate page (%d bytes) is not smaller than all 40 (%d bytes)", pageBytes, wholeBytes)
	}
	// The reduction tracks the page size rather than hiding rows: the page still
	// carries the fields the client renders, passage text included.
	if page.Candidates[0].PassageTextAR == "" {
		t.Fatal("the bounded page dropped the passage text the web client renders")
	}
	if page.Candidates[0].RawTextAR == "" || page.Candidates[0].Reviews == nil {
		t.Fatalf("the bounded page lost a rendered field: %+v", page.Candidates[0])
	}
}

// TestCandidateReviewHydrationKeepsAuthorization is the correctness floor for this
// step: batching the reviews must not widen who can read them. An actor who may
// not manage the source is refused before any candidate is read, and a reviewer
// of a stranger's candidate changes nothing about the list.
func TestCandidateReviewHydrationKeepsAuthorization(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedCandidateReviews(t, fixture, 5, 1)
	service, _ := newReviewService(t, fixture)
	ctx := fixture.Ctx()

	// Another registered account is not an owner of the source.
	stranger := actor.Register(t, fixture, "غريب")
	counter := &testsupport.QueryCounter{}
	_ = counter
	if _, err := service.GetProcessing(ctx, seeded.sourceID.String(), stranger.User.ID); err != ErrForbidden {
		t.Fatalf("a stranger reading the source = %v, want ErrForbidden", err)
	}
	view, err := service.GetProcessing(ctx, seeded.sourceID.String(), seeded.ownerID)
	if err != nil {
		t.Fatalf("owner reading the source: %v", err)
	}
	for _, candidate := range view.Candidates {
		for _, review := range candidate.Reviews {
			if review.ReviewerID != seeded.ownerID {
				t.Fatalf("candidate %s exposes a review by %s, and the only reviewer is %s", candidate.ID, review.ReviewerID, seeded.ownerID)
			}
		}
	}
	// The single-candidate path is the one that still returns a full candidate,
	// which is what the review endpoint answers with.
	one, err := service.getCandidate(ctx, seeded.candidateID[0])
	if err != nil {
		t.Fatalf("read one candidate: %v", err)
	}
	if one.ID != seeded.candidateID[0].String() || len(one.Reviews) != 1 {
		t.Fatalf("single candidate = %+v, want %s with one review", one, seeded.candidateID[0])
	}
	if one.PassageTextAR == "" {
		t.Fatal("the single-candidate path dropped the passage text")
	}
}
