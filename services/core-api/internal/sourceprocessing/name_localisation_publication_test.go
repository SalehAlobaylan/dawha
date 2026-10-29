package sourceprocessing

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func addLegacySpot(t *testing.T, f *nameRunFixture, key string, region Region) string {
	t.Helper()
	var source uuid.UUID
	if err := f.f.Pool().QueryRow(f.f.Ctx(), `SELECT source_id FROM source_files WHERE id=$1`, f.fileID).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if err := f.service.CreateSuggestions(f.f.Ctx(), source, uuid.MustParse(f.fileID), nil, []SuggestionInput{{OccurrenceID: key, PageNumber: 1, Region: region, SuggestedTextAR: "حسن", ReadingMethod: "local_carrier_tesseract", ReadingModel: legacyCarrierReadingModel}}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := f.f.Pool().QueryRow(f.f.Ctx(), `SELECT id::text FROM source_tree_suggestions WHERE source_file_id=$1 AND occurrence_id=$2`, f.fileID, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestNameLocalisationPublishesNewSpotsAndPreservesHumanWork(t *testing.T) {
	f := newNameRunFixture(t)
	ctx := f.f.Ctx()
	branch := addLegacySpot(t, f, "branch", Region{X: .3, Y: .4, Width: .001, Height: .003})
	corrected := addLegacySpot(t, f, "corrected", Region{X: .6, Y: .2, Width: .15, Height: .1})
	edited := addLegacySpot(t, f, "edited", Region{X: .2, Y: .7, Width: .1, Height: .1})
	reopened := addLegacySpot(t, f, "reopened", Region{X: .5, Y: .7, Width: .1, Height: .1})
	if _, err := f.service.DecideSuggestion(ctx, corrected, f.actor.User.ID, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "أحمد"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.f.Pool().Exec(ctx, `UPDATE source_tree_suggestions SET geometry_revision=1 WHERE id=$1`, edited); err != nil {
		t.Fatal(err)
	}
	if _, err := f.f.Pool().Exec(ctx, `UPDATE source_tree_suggestions SET review_revision=1 WHERE id=$1`, reopened); err != nil {
		t.Fatal(err)
	}
	var before string
	f.f.Pool().QueryRow(ctx, `SELECT row_to_json(d)::text FROM source_tree_suggestion_decisions d WHERE suggestion_id=$1`, corrected).Scan(&before)
	run := f.prepare(t)
	f.process(t, f.claim(t, "publish"))
	pending, err := f.service.ListSuggestions(ctx, f.fileID, f.actor.User.ID, "suggested", 200, 0)
	if err != nil || len(pending) != 3 {
		t.Fatalf("queue=%+v error=%v", pending, err)
	}
	for _, spot := range pending {
		if spot.ID == branch {
			t.Fatal("branch placeholder is still active")
		}
	}
	history, err := f.service.SuggestionHistory(ctx, branch, f.actor.User.ID)
	if err != nil || !history.Suggestion.Superseded || len(history.Decisions) != 0 || len(history.Events) != 1 || history.Events[0].Type != "redetected" {
		t.Fatalf("old evidence was lost or turned into a human rejection: %+v %v", history, err)
	}
	var after string
	f.f.Pool().QueryRow(ctx, `SELECT row_to_json(d)::text FROM source_tree_suggestion_decisions d WHERE suggestion_id=$1`, corrected).Scan(&after)
	if before != after {
		t.Fatal("human decision ledger changed")
	}
	if got := f.f.Count(`SELECT count(*) FROM source_tree_suggestions WHERE source_file_id=$1`, f.fileID); got != 6 {
		t.Fatalf("lost raw proposals: %d", got)
	}
	if got := f.f.Count(`SELECT count(*) FROM source_name_review_events WHERE event_type='redetected' AND metadata->>'runId'=$1`, run.ID); got != 2 {
		t.Fatalf("publication not idempotent: %d events", got)
	}
	summary, err := f.service.NameReviewSummary(ctx, f.fileID, f.actor.User.ID)
	if err != nil || summary.Pending != 3 || summary.Corrected != 1 || summary.Superseded != 2 {
		t.Fatalf("summary=%+v %v", summary, err)
	}
	library, err := f.service.ListChartLibrary(ctx, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range library {
		if item.FileID == f.fileID && (item.PendingNames != 3 || item.ReviewedNames != 1 || item.TotalNames != 4) {
			t.Fatalf("library disagrees: %+v", item)
		}
	}
}

func TestIncompleteNameLocalisationDoesNotReplaceTheQueue(t *testing.T) {
	f := newNameRunFixture(t)
	ctx := f.f.Ctx()
	id := addLegacySpot(t, f, "old", Region{X: .2, Y: .2, Width: .001, Height: .001})
	f.service.DetectNameTile = func([]byte, CarrierLocaliserConfig) ([]PageRegion, error) {
		return nil, errors.New("fixture detection failed")
	}
	run := f.prepare(t)
	claim := f.claim(t, "failed")
	if err := f.service.ProcessNameReading(ctx, claim); err == nil {
		t.Fatal("expected detection failure")
	}
	pending, err := f.service.ListSuggestions(ctx, f.fileID, f.actor.User.ID, "suggested", 200, 0)
	if err != nil || len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("failed pass replaced queue: %+v %v", pending, err)
	}
	if f.f.Count(`SELECT count(*) FROM source_name_review_events WHERE metadata->>'runId'=$1`, run.ID) != 0 {
		t.Fatal("failed pass published events")
	}
}

func TestStagedNameSpotsAreNotReviewableOrCounted(t *testing.T) {
	f := newNameRunFixture(t)
	ctx := f.f.Ctx()
	id := addLegacySpot(t, f, "staged", Region{X: .2, Y: .2, Width: .1, Height: .1})
	if _, err := f.f.Pool().Exec(ctx, `UPDATE source_tree_suggestions SET localisation_pending=true WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	pending, err := f.service.ListSuggestions(ctx, f.fileID, f.actor.User.ID, "suggested", 200, 0)
	if err != nil || len(pending) != 0 {
		t.Fatalf("staged queue=%+v %v", pending, err)
	}
	summary, err := f.service.NameReviewSummary(ctx, f.fileID, f.actor.User.ID)
	if err != nil || summary.Total != 0 {
		t.Fatalf("staged summary=%+v %v", summary, err)
	}
	if _, err := f.service.DecideSuggestion(ctx, id, f.actor.User.ID, SuggestionDecision{Decision: SuggestionRejected}); !errors.Is(err, ErrConflict) {
		t.Fatalf("decided a staged candidate: %v", err)
	}
	library, err := f.service.ListChartLibrary(ctx, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range library {
		if item.FileID == f.fileID && item.TotalNames != 0 {
			t.Fatalf("staged library=%+v", item)
		}
	}
}

func TestTinyRejectedSpotDoesNotSuppressAWholeWord(t *testing.T) {
	if sameNameLocalisation(Region{X: .2, Y: .2, Width: .001, Height: .001}, Region{X: .19, Y: .19, Width: .1, Height: .1}) {
		t.Fatal("tiny branch rejection suppressed a whole name")
	}
	if !sameNameLocalisation(Region{X: .2, Y: .2, Width: .1, Height: .1}, Region{X: .205, Y: .205, Width: .1, Height: .1}) {
		t.Fatal("duplicate word was not matched")
	}
}

func TestOlderNameScanCannotReplaceANewerRequestedScan(t *testing.T) {
	f := newNameRunFixture(t)
	ctx := f.f.Ctx()
	id := addLegacySpot(t, f, "old", Region{X: .2, Y: .2, Width: .001, Height: .001})
	older := f.prepare(t)
	reader := NewLocalCropReader()
	reader.Budget = time.Second
	f.service.LocalCropReader = reader
	newer := f.prepare(t)
	if older.ID == newer.ID {
		t.Fatal("fixture did not create a new scan")
	}
	if err := f.service.publishNameLocalisation(ctx, nil, older); err != nil {
		t.Fatal(err)
	}
	history, err := f.service.SuggestionHistory(ctx, id, f.actor.User.ID)
	if err != nil || history.Suggestion.Superseded || len(history.Events) != 0 {
		t.Fatalf("older scan replaced newer work: %+v %v", history, err)
	}
}
