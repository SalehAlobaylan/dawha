package sourceprocessing

import (
	"errors"
	"sync"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
)

func findSuggestionByText(t *testing.T, views []SuggestionView, text string) SuggestionView {
	t.Helper()
	for _, view := range views {
		if view.SuggestedTextAR == text {
			return view
		}
	}
	t.Fatalf("suggestion %q was not in the queue: %+v", text, views)
	return SuggestionView{}
}

func TestDeferReopenAndCorrectionKeepEveryPriorAnswer(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-history-1", PageNumber: 1, SuggestedTextAR: "عبدالله", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.9, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1", Alternatives: []string{"عبد اللـه", "عبيد الله"}},
		{OccurrenceID: "carrier-history-2", PageNumber: 1, SuggestedTextAR: "صالح", Region: Region{X: 0.2, Y: 0.3, Width: 0.05, Height: 0.01}, Confidence: 0.6, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	}); err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil || len(queue) != 2 {
		t.Fatalf("list suggestions: %v (%d)", err, len(queue))
	}
	deferred := findSuggestionByText(t, queue, "عبدالله")
	accepted := findSuggestionByText(t, queue, "صالح")

	if _, err = service.DecideSuggestion(ctx, deferred.ID, ownerID, SuggestionDecision{Decision: SuggestionDeferred, NoteAR: "الخط غير واضح"}); err != nil {
		t.Fatalf("defer: %v", err)
	}
	if _, err = service.DecideSuggestion(ctx, accepted.ID, ownerID, SuggestionDecision{Decision: SuggestionAccepted}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	// A deferred occurrence leaves pending work but remains reachable on reload.
	pending, err := service.ListSuggestions(ctx, fileID, ownerID, "suggested", 0, 0)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending queue after defer: %v (%d)", err, len(pending))
	}
	deferredQueue, err := service.ListSuggestions(ctx, fileID, ownerID, "deferred", 0, 0)
	if err != nil || len(deferredQueue) != 1 || deferredQueue[0].ID != deferred.ID {
		t.Fatalf("deferred queue: %v (%+v)", err, deferredQueue)
	}
	historyQueue, err := service.ListSuggestions(ctx, fileID, ownerID, "history", 0, 0)
	if err != nil || len(historyQueue) != 2 {
		t.Fatalf("history queue: %v (%d)", err, len(historyQueue))
	}

	var firstDecisionID, firstCreatedAt, firstSnapshot string
	var firstRevision int
	if err = fixture.QueryRow(`
		SELECT id::text, created_at::text, candidate_snapshot::text, review_revision
		FROM source_tree_suggestion_decisions WHERE suggestion_id = $1
	`, deferred.ID).Scan(&firstDecisionID, &firstCreatedAt, &firstSnapshot, &firstRevision); err != nil {
		t.Fatalf("read first decision: %v", err)
	}
	if firstRevision != 1 {
		t.Fatalf("first decision revision = %d, want 1", firstRevision)
	}

	before := SuggestionHistory{}
	before, err = service.SuggestionHistory(ctx, deferred.ID, ownerID)
	if err != nil {
		t.Fatalf("history before reopen: %v", err)
	}
	if len(before.Decisions) != 1 || before.Decisions[0].ID != firstDecisionID || !before.CanReopen {
		t.Fatalf("deferred history = %+v", before)
	}
	if before.Decisions[0].SuggestedTextAR != "عبدالله" {
		t.Fatalf("machine reading missing from history: %+v", before.Decisions[0])
	}

	reopened, err := service.ReopenSuggestion(ctx, deferred.ID, ownerID, SuggestionReopenInput{ExpectedRevision: 1, NoteAR: "قرأت الموضع مرة أخرى"})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Status != "suggested" || reopened.ReviewRevision != 1 || reopened.DecidedTextAR != "" {
		t.Fatalf("reopened occurrence = %+v", reopened)
	}
	if reopened.SuggestedTextAR != "عبدالله" {
		t.Fatalf("reopen rewrote the machine reading: %q", reopened.SuggestedTextAR)
	}
	// A reading rerun after reopening must not replace the original machine
	// evidence under the reviewer. History keeps the decision either way; the
	// current queue row should still say what the human is reviewing.
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{{
		OccurrenceID: deferred.OccurrenceID, PageNumber: 1, SuggestedTextAR: "متغير",
		Region: Region{X: 0.3, Y: 0.3, Width: 0.05, Height: 0.01}, Confidence: 0.2, ReadingMethod: "rerun", ReadingModel: "rerun",
	}}); err != nil {
		t.Fatalf("rerun after reopen: %v", err)
	}
	afterRerun, err := service.getSuggestion(ctx, mustUUID(deferred.ID))
	if err != nil || afterRerun.SuggestedTextAR != "عبدالله" {
		t.Fatalf("a rerun overwrote reopened evidence: %v %+v", err, afterRerun)
	}
	if _, err = service.ReopenSuggestion(ctx, deferred.ID, ownerID, SuggestionReopenInput{ExpectedRevision: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second reopen of a pending occurrence = %v, want ErrConflict", err)
	}

	after, err := service.SuggestionHistory(ctx, deferred.ID, ownerID)
	if err != nil {
		t.Fatalf("history after reopen: %v", err)
	}
	if len(after.Decisions) != 1 || after.Decisions[0].ID != firstDecisionID {
		t.Fatalf("reopen deleted the first decision: %+v", after.Decisions)
	}
	var reopenEvent SuggestionReviewEvent
	foundReopen := false
	for _, event := range after.Events {
		if event.Type == "reopened" {
			reopenEvent = event
			foundReopen = true
		}
	}
	if !foundReopen || reopenEvent.PreviousDecisionID != firstDecisionID || reopenEvent.Revision != 1 {
		t.Fatalf("reopen event = %+v (found %t)", reopenEvent, foundReopen)
	}

	revisionOne := 1
	corrected, err := service.DecideSuggestion(ctx, deferred.ID, ownerID, SuggestionDecision{
		Decision:         SuggestionCorrected,
		TextAR:           "عبد اللّه",
		ExpectedRevision: &revisionOne,
		NoteAR:           "الألف الممدودة",
	})
	if err != nil {
		t.Fatalf("correct after reopen: %v", err)
	}
	if corrected.Status != SuggestionCorrected || corrected.ReviewRevision != 2 || corrected.DecidedTextAR != "عبد اللّه" {
		t.Fatalf("corrected occurrence = %+v", corrected)
	}

	// The original ledger row is byte-identical after the second decision.
	var currentCreatedAt, currentSnapshot string
	var currentRevision int
	if err = fixture.QueryRow(`
		SELECT created_at::text, candidate_snapshot::text, review_revision
		FROM source_tree_suggestion_decisions WHERE id = $1
	`, firstDecisionID).Scan(&currentCreatedAt, &currentSnapshot, &currentRevision); err != nil {
		t.Fatalf("re-read first decision: %v", err)
	}
	if currentCreatedAt != firstCreatedAt || currentSnapshot != firstSnapshot || currentRevision != firstRevision {
		t.Fatal("a later decision rewrote the first decision row")
	}

	final, err := service.SuggestionHistory(ctx, deferred.ID, ownerID)
	if err != nil {
		t.Fatalf("final history: %v", err)
	}
	if len(final.Decisions) != 2 {
		t.Fatalf("final history decisions = %d, want 2", len(final.Decisions))
	}
	if final.Decisions[0].ID != firstDecisionID || final.Decisions[0].DecidedTextAR != "" {
		t.Fatalf("first decision changed: %+v", final.Decisions[0])
	}
	if final.Decisions[1].Revision != 2 || final.Decisions[1].SupersedesDecisionID != firstDecisionID || final.Decisions[1].DecidedTextAR != "عبد اللّه" {
		t.Fatalf("second decision lineage = %+v", final.Decisions[1])
	}

	// A stale form cannot append a second current answer.
	if _, err = service.ReopenSuggestion(ctx, deferred.ID, ownerID, SuggestionReopenInput{ExpectedRevision: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale reopen = %v, want ErrConflict", err)
	}
	if _, err = service.DecideSuggestion(ctx, deferred.ID, ownerID, SuggestionDecision{
		Decision: SuggestionAccepted, ExpectedRevision: &revisionOne,
	}); !errors.Is(err, ErrSuggestionAlreadyDecided) && !errors.Is(err, ErrConflict) {
		t.Fatalf("stale decision = %v, want a conflict", err)
	}

	// A mistaken acceptance can be reopened and corrected; the accepted text
	// remains the first answer in history.
	if _, err = service.ReopenSuggestion(ctx, accepted.ID, ownerID, SuggestionReopenInput{ExpectedRevision: 1}); err != nil {
		t.Fatalf("reopen accepted: %v", err)
	}
	if _, err = service.DecideSuggestion(ctx, accepted.ID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "صالح محمد", ExpectedRevision: &revisionOne,
	}); err != nil {
		t.Fatalf("correct accepted: %v", err)
	}
	acceptedHistory, err := service.SuggestionHistory(ctx, accepted.ID, ownerID)
	if err != nil {
		t.Fatalf("accepted history: %v", err)
	}
	if len(acceptedHistory.Decisions) != 2 || acceptedHistory.Decisions[0].Decision != SuggestionAccepted || acceptedHistory.Decisions[0].DecidedTextAR != "صالح" || acceptedHistory.Decisions[1].DecidedTextAR != "صالح محمد" {
		t.Fatalf("accepted history = %+v", acceptedHistory.Decisions)
	}

	summary, err := service.NameReviewSummary(ctx, fileID, ownerID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Total != 2 || summary.Pending != 0 || summary.Deferred != 0 || summary.Corrected != 2 || summary.Decided != 2 || summary.DecisionEvents != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestReopenAndHistoryRequireSourceAccess(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-denied", PageNumber: 1, SuggestedTextAR: "ناصر", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.8, ReadingMethod: "local", ReadingModel: "test"},
	}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil || len(queue) != 1 {
		t.Fatalf("list suggestions: %v (%d)", err, len(queue))
	}
	if _, err = service.DecideSuggestion(ctx, queue[0].ID, ownerID, SuggestionDecision{Decision: SuggestionDeferred}); err != nil {
		t.Fatalf("defer: %v", err)
	}
	other := actor.RegisterAndLogin(t, fixture, "مراجع بلا وصول")
	if _, err = service.SuggestionHistory(ctx, queue[0].ID, other.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("history leak = %v, want ErrForbidden", err)
	}
	if _, err = service.ReopenSuggestion(ctx, queue[0].ID, other.User.ID, SuggestionReopenInput{ExpectedRevision: 1}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("reopen leak = %v, want ErrForbidden", err)
	}
	if _, err = service.NameReviewSummary(ctx, fileID, other.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("summary leak = %v, want ErrForbidden", err)
	}
	if _, err = service.DecideSuggestion(ctx, queue[0].ID, other.User.ID, SuggestionDecision{Decision: SuggestionAccepted}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("decision leak = %v, want ErrForbidden", err)
	}
}

func TestConcurrentDecisionsAppendExactlyOneCurrentAnswer(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-race", PageNumber: 1, SuggestedTextAR: "سعيد", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.8, ReadingMethod: "local", ReadingModel: "test"},
	}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil || len(queue) != 1 {
		t.Fatalf("list suggestions: %v (%d)", err, len(queue))
	}
	revisionZero := 0
	texts := []string{"سعيد", "سعد"}
	errs := make([]error, len(texts))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range texts {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errs[index] = service.DecideSuggestion(ctx, queue[0].ID, ownerID, SuggestionDecision{
				Decision: SuggestionCorrected, TextAR: texts[index], ExpectedRevision: &revisionZero,
			})
		}(index)
	}
	close(start)
	wait.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrSuggestionAlreadyDecided) && !errors.Is(err, ErrConflict) {
			t.Fatalf("concurrent decision failed with %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent decisions succeeded %d times: %v", successes, errs)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_tree_suggestion_decisions WHERE suggestion_id=$1`, queue[0].ID); count != 1 {
		t.Fatalf("decision ledger rows = %d, want 1", count)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_name_review_events WHERE suggestion_id=$1 AND event_type='decision'`, queue[0].ID); count != 1 {
		t.Fatalf("decision events = %d, want 1", count)
	}
	var revision, decisions int
	if err := fixture.QueryRow(`
		SELECT sg.review_revision, COUNT(d.id)
		FROM source_tree_suggestions sg
		LEFT JOIN source_tree_suggestion_decisions d ON d.suggestion_id=sg.id
		WHERE sg.id=$1 GROUP BY sg.review_revision
	`, queue[0].ID).Scan(&revision, &decisions); err != nil {
		t.Fatalf("read concurrent result: %v", err)
	}
	if revision != 1 || decisions != 1 {
		t.Fatalf("concurrent result revision=%d decisions=%d", revision, decisions)
	}
}

func TestAPreMigrationDecisionCanBeReopenedWithoutRewritingIt(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-legacy", PageNumber: 1, SuggestedTextAR: "صالح", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.8, ReadingMethod: "local", ReadingModel: "test"},
	}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil || len(queue) != 1 {
		t.Fatalf("list suggestions: %v (%d)", err, len(queue))
	}
	// Simulate a decision written before migration 0058. The row has no review
	// revision, but it is still a human answer and must remain reopenable.
	fixture.Exec(`UPDATE source_tree_suggestions SET status='accepted', decided_text_ar='صالح', decided_by=$2, decided_at=clock_timestamp() WHERE id=$1`, queue[0].ID, ownerID)
	var legacyDecisionID string
	if err := fixture.QueryRow(`
		INSERT INTO source_tree_suggestion_decisions (suggestion_id, reviewer_id, decision, suggested_text_ar, decided_text_ar, review_revision)
		VALUES ($1,$2,'accepted','صالح','صالح',0) RETURNING id::text
	`, queue[0].ID, ownerID).Scan(&legacyDecisionID); err != nil {
		t.Fatalf("insert legacy decision: %v", err)
	}
	history, err := service.SuggestionHistory(ctx, queue[0].ID, ownerID)
	if err != nil || !history.CanReopen {
		t.Fatalf("legacy history cannot be reopened: %v %+v", err, history)
	}
	reopened, err := service.ReopenSuggestion(ctx, queue[0].ID, ownerID, SuggestionReopenInput{ExpectedRevision: 0})
	if err != nil {
		t.Fatalf("reopen legacy decision: %v", err)
	}
	if reopened.ReviewRevision != 1 {
		t.Fatal("legacy reopen retained the stale first-decision token")
	}
	revisionOne := reopened.ReviewRevision
	updated, err := service.DecideSuggestion(ctx, queue[0].ID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "صالح محمد", ExpectedRevision: &revisionOne,
	})
	if err != nil {
		t.Fatalf("correct legacy decision: %v", err)
	}
	if updated.ReviewRevision != 2 || updated.DecidedTextAR != "صالح محمد" {
		t.Fatalf("legacy correction = %+v", updated)
	}
	final, err := service.SuggestionHistory(ctx, queue[0].ID, ownerID)
	if err != nil || len(final.Decisions) != 2 {
		t.Fatalf("legacy history = %v %+v", err, final)
	}
	if final.Decisions[0].ID != legacyDecisionID || final.Decisions[0].Revision != 0 || final.Decisions[0].DecidedTextAR != "صالح" {
		t.Fatalf("legacy decision changed: %+v", final.Decisions[0])
	}
	if final.Decisions[1].Revision != 2 || final.Decisions[1].SupersedesDecisionID != legacyDecisionID {
		t.Fatalf("legacy correction lineage: %+v", final.Decisions[1])
	}
	summary, err := service.NameReviewSummary(ctx, fileID, ownerID)
	if err != nil || summary.DecisionEvents != 1 {
		t.Fatalf("legacy summary = %v %+v", err, summary)
	}
}
