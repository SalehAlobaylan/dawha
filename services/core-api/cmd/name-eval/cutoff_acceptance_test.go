package main

import (
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	sp "github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

func TestReview026CutoffSurvivesLaterMerge(t *testing.T) {
	f := testsupport.New(t)
	ctx := f.Ctx()
	owner := actor.RegisterAndLogin(t, f, "مراجع اختبار")
	src, err := evidence.NewService(f.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{TitleAR: "اختبار " + f.Unique("cutoff"), SourceType: "family_document"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := sp.NewService(f.Pool(), st, jobs.NewService(f.Pool()), nil, sp.NewTextExtractor())
	file, err := s.Upload(ctx, src.Source.ID, owner.User.ID, sp.UploadInput{Filename: "fixture.txt", ContentType: "text/plain", Content: []byte("fixture")})
	if err != nil {
		t.Fatal(err)
	}
	err = s.CreateSuggestions(ctx, uuid.MustParse(src.Source.ID), uuid.MustParse(file.ID), nil, []sp.SuggestionInput{{OccurrenceID: "a", PageNumber: 1, SuggestedTextAR: "صالح", Region: sp.Region{X: .1, Y: .1, Width: .1, Height: .1}, ReadingMethod: "fixture", ReadingModel: "fixture"}, {OccurrenceID: "b", PageNumber: 1, SuggestedTextAR: "خالد", Region: sp.Region{X: .3, Y: .1, Width: .1, Height: .1}, ReadingMethod: "fixture", ReadingModel: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.ListSuggestions(ctx, file.ID, owner.User.ID, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	decided, err := s.DecideSuggestion(ctx, q[0].ID, owner.User.ID, sp.SuggestionDecision{Decision: sp.SuggestionAccepted})
	if err != nil {
		t.Fatal(err)
	}
	var at time.Time
	if err = f.QueryRow(`SELECT clock_timestamp()`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	cutoff := at.Format(time.RFC3339Nano)
	before, err := loadDecisions(ctx, f.Pool(), "", cutoff)
	if err != nil {
		t.Fatal(err)
	}
	beforeReport := summarizeWithTruth(before, "", false, "first", cutoff)
	_, err = s.MergeSuggestions(ctx, q[0].ID, owner.User.ID, sp.SuggestionMergeInput{Members: []sp.SuggestionMergeMember{{ID: q[0].ID, ExpectedReviewRevision: decided.ReviewRevision}, {ID: q[1].ID}}, Region: sp.Region{X: .1, Y: .1, Width: .3, Height: .1}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := loadDecisions(ctx, f.Pool(), "", cutoff)
	if err != nil {
		t.Fatal(err)
	}
	afterReport := summarizeWithTruth(after, "", false, "first", cutoff)
	if beforeReport.Decisions != afterReport.Decisions {
		t.Fatalf("fixed cutoff changed after later merge: %d -> %d decisions", beforeReport.Decisions, afterReport.Decisions)
	}
	if beforeReport.Decisions != 1 || afterReport.SupersededDecisions != 0 {
		t.Fatal("pre-merge cutoff did not retain the original observation")
	}
	for _, policy := range []string{"first", "latest"} {
		if summarizeWithTruth(after, "", false, policy, cutoff).Decisions != 1 {
			t.Fatalf("%s cutoff lost a pre-merge observation", policy)
		}
	}
	current, err := loadDecisions(ctx, f.Pool(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if result := summarizeWithTruth(current, "", false, "first", ""); result.Decisions != 0 || result.SupersededDecisions != 1 {
		t.Fatal("current report failed to exclude superseded geometry")
	}
	if _, err = loadDecisions(ctx, f.Pool(), cutoff, cutoff); err != nil {
		t.Fatalf("combined since/cutoff parameters failed: %v", err)
	}
}
