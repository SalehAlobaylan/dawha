package sourceprocessing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

func TestReview026LegacyReopenRejectsStaleFirstDecision(t *testing.T) {
	s, f, source, file, owner := suggestionsFixture(t)
	ctx := f.Ctx()
	if err := s.CreateSuggestions(ctx, mustUUID(source), mustUUID(file), nil, []SuggestionInput{{OccurrenceID: "legacy-review", PageNumber: 1, SuggestedTextAR: "صالح", Region: Region{X: .1, Y: .2, Width: .1, Height: .1}, ReadingMethod: "fixture", ReadingModel: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	q, err := s.ListSuggestions(ctx, file, owner, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	id := q[0].ID
	f.Exec(`UPDATE source_tree_suggestions SET status='accepted',decided_text_ar='صالح',decided_by=$2,decided_at=clock_timestamp() WHERE id=$1`, id, owner)
	f.Exec(`INSERT INTO source_tree_suggestion_decisions(suggestion_id,reviewer_id,decision,suggested_text_ar,decided_text_ar,review_revision) VALUES($1,$2,'accepted','صالح','صالح',0)`, id, owner)
	if _, err = s.ReopenSuggestion(ctx, id, owner, SuggestionReopenInput{ExpectedRevision: 0}); err != nil {
		t.Fatal(err)
	}
	_, err = s.DecideSuggestion(ctx, id, owner, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "خالد"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale client without expectedRevision accepted after legacy reopen: %v", err)
	}
	zero := 0
	if _, err = s.DecideSuggestion(ctx, id, owner, SuggestionDecision{Decision: SuggestionAccepted, ExpectedRevision: &zero}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale explicit first-decision token accepted: %v", err)
	}
	if f.Count(`SELECT count(*) FROM source_tree_suggestion_decisions WHERE suggestion_id=$1`, id) != 1 {
		t.Fatal("stale requests appended decisions")
	}

	// Emulate an item reopened before this fix, then execute the actual
	// operational migration and prove its old ledger/event bytes stay intact.
	f.Exec(`UPDATE source_tree_suggestions SET review_revision=0 WHERE id=$1`, id)
	f.Exec(`UPDATE source_name_review_events SET revision=0 WHERE suggestion_id=$1 AND event_type='reopened'`, id)
	oldEvidence := func() string {
		var encoded string
		if err := f.QueryRow(`SELECT jsonb_build_object('decision',to_jsonb(d),'event',to_jsonb(e))::text FROM source_tree_suggestion_decisions d JOIN source_name_review_events e ON e.suggestion_id=d.suggestion_id WHERE d.suggestion_id=$1 AND d.review_revision=0 AND e.event_type='reopened'`, id).Scan(&encoded); err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	before := oldEvidence()
	root, err := testsupport.FindRepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(root, "db/migrations/0060_name_legacy_reopen_tokens.sql"))
	if err != nil {
		t.Fatal(err)
	}
	f.Exec(string(migration))
	one := 1
	if _, err = s.DecideSuggestion(ctx, id, owner, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "خالد", ExpectedRevision: &one}); err != nil {
		t.Fatalf("migrated reopened legacy item could not be saved: %v", err)
	}
	if oldEvidence() != before {
		t.Fatal("operational migration rewrote old evidence")
	}
}
func TestReview026DecisionRejectsStaleGeometry(t *testing.T) {
	s, f, source, file, owner := repairFixture(t)
	ctx := f.Ctx()
	initial := Region{X: .1, Y: .2, Width: .1, Height: .1}
	if err := s.CreateSuggestions(ctx, mustUUID(source), mustUUID(file), nil, []SuggestionInput{{OccurrenceID: "stale-geometry", PageNumber: 1, SuggestedTextAR: "صالح", Region: initial, ReadingMethod: "fixture", ReadingModel: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	q, err := s.ListSuggestions(ctx, file, owner, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	shown := q[0]
	if _, err = s.ReviseSuggestionRegion(ctx, shown.ID, owner, SuggestionRegionInput{ExpectedGeometryRevision: 0, Region: Region{X: .5, Y: .5, Width: .1, Height: .1}}); err != nil {
		t.Fatal(err)
	}
	_, err = s.DecideSuggestion(ctx, shown.ID, owner, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "خالد", ExpectedRevision: &shown.ReviewRevision})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("decision from geometry revision 0 accepted on geometry revision 1: %v", err)
	}
	zero, one := 0, 1
	input := SuggestionDecision{Decision: SuggestionCorrected, TextAR: "خالد", ExpectedRevision: &shown.ReviewRevision, ExpectedGeometryRevision: &zero}
	if _, err = s.DecideSuggestion(ctx, shown.ID, owner, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("explicit stale geometry accepted: %v", err)
	}
	input.ExpectedGeometryRevision = &one
	if _, err = s.DecideSuggestion(ctx, shown.ID, owner, input); err != nil {
		t.Fatalf("current geometry rejected: %v", err)
	}
	history, err := s.SuggestionHistory(ctx, shown.ID, owner)
	if err != nil || len(history.Decisions) != 1 || history.Decisions[0].GeometryRevision == nil || *history.Decisions[0].GeometryRevision != 1 {
		t.Fatalf("decision geometry not retained: %v %+v", err, history.Decisions)
	}
	if f.Count(`SELECT count(*) FROM source_name_review_events e JOIN source_name_region_revisions g ON g.id=e.geometry_revision_id WHERE e.suggestion_id=$1 AND e.event_type='decision' AND g.revision=1`, shown.ID) != 1 {
		t.Fatal("decision event lost the geometry reference")
	}
}
func TestReview026QueuedRereadKeepsRequestedRegion(t *testing.T) {
	f := newNameRunFixture(t)
	f.prepare(t)
	f.process(t, f.claim(t, "initial"))
	ctx := f.f.Ctx()
	q, err := f.service.ListSuggestions(ctx, f.fileID, f.actor.User.ID, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	shown := q[0]
	run, err := f.service.RereadSuggestionLocally(ctx, shown.ID, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := f.service.RereadSuggestionLocally(ctx, shown.ID, f.actor.User.ID)
	if err != nil || duplicate.ID != run.ID || run.TargetGeometry == nil || !sameRegion(run.TargetGeometry.Region, shown.Region) {
		t.Fatalf("unchanged target lost its snapshot/deduplication: %v", err)
	}
	moved := shown.Region
	moved.X += .01
	if _, err = f.service.ReviseSuggestionRegion(ctx, shown.ID, f.actor.User.ID, SuggestionRegionInput{ExpectedGeometryRevision: 0, Region: moved}); err != nil {
		t.Fatal(err)
	}
	f.process(t, f.claim(t, "reread"))
	var encoded []byte
	if err = f.f.QueryRow(`SELECT payload->'region' FROM source_name_reading_units WHERE run_id=$1 AND stage='read'`, run.ID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var actual Region
	if err = json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	if !sameRegion(actual, shown.Region) {
		t.Fatalf("queued reread switched from requested region %+v to later region %+v", shown.Region, actual)
	}
	newRun, err := f.service.RereadSuggestionLocally(ctx, shown.ID, f.actor.User.ID)
	if err != nil || newRun.ID == run.ID || newRun.TargetGeometry == nil || newRun.TargetGeometry.GeometryRevision != 1 || !sameRegion(newRun.TargetGeometry.Region, moved) {
		t.Fatalf("revised target reused an obsolete run: %v %+v", err, newRun)
	}
}

func TestLegacyRereadWithoutFrozenGeometryCannotUseLiveRegion(t *testing.T) {
	f := newNameRunFixture(t)
	f.prepare(t)
	f.process(t, f.claim(t, "initial"))
	q, err := f.service.ListSuggestions(f.f.Ctx(), f.fileID, f.actor.User.ID, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.RereadSuggestionLocally(f.f.Ctx(), q[0].ID, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.f.Exec(`UPDATE source_name_reading_runs SET target_geometry=NULL WHERE id=$1`, run.ID)
	claim := f.claim(t, "legacy-reread")
	if err = f.service.ProcessNameReading(f.f.Ctx(), claim); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing snapshot rebuilt from live rectangle: %v", err)
	}
	if f.f.Count(`SELECT count(*) FROM source_name_reading_units WHERE run_id=$1`, run.ID) != 0 {
		t.Fatal("legacy run inferred a target")
	}
}
