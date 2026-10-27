package sourceprocessing

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
)

// suggestionsFixture uploads a file and returns the service and the ids a suggestion
// needs.
func suggestionsFixture(t *testing.T) (*Service, *testsupport.Fixture, string, string, string) {
	t.Helper()
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "قارئ شجرة")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{TitleAR: "شجرة " + fixture.Unique("s"), SourceType: "family_document"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := uploader.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{Filename: "tree.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7\\nfixture")})
	if err != nil {
		t.Fatalf("upload source: %v", err)
	}
	service := NewService(fixture.Pool(), nil, nil, nil, nil)
	return service, fixture, source.Source.ID, file.ID, owner.User.ID
}

func TestASuggestionIsQueuedByConfidenceAndNeverDecidedWithoutAReviewer(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()

	err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-000001", PageNumber: 1, SuggestedTextAR: "عبدالله", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.95, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1", Alternatives: []string{"عبد اللـه", "عبيد الله"}},
		{OccurrenceID: "carrier-000002", PageNumber: 1, SuggestedTextAR: "صالح", Region: Region{X: 0.2, Y: 0.3, Width: 0.04, Height: 0.01}, Confidence: 0.40, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	})
	if err != nil {
		t.Fatalf("create suggestions: %v", err)
	}

	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	if len(queue) != 2 {
		t.Fatalf("expected two suggestions, got %d", len(queue))
	}
	// The most certain reading comes first: the confidence orders the queue.
	if queue[0].SuggestedTextAR != "عبدالله" || queue[1].SuggestedTextAR != "صالح" {
		t.Fatalf("the queue is not ordered by confidence: %+v", queue)
	}
	if queue[0].Confidence != 0.95 {
		t.Fatalf("expected the measured confidence to be kept, got %v", queue[0].Confidence)
	}
	if queue[0].Status != "suggested" {
		t.Fatalf("a new suggestion must be undecided, got %q", queue[0].Status)
	}
	// The reader's alternatives are hypotheses for one-click correction, not
	// additional readings and not decisions. Losing them would make a person
	// retype a name the reader already got half right.
	if len(queue[0].Alternatives) != 2 || queue[0].Alternatives[1] != "عبيد الله" {
		t.Fatalf("the reader's alternatives were not kept: %+v", queue[0].Alternatives)
	}

	// A suggestion is not a person, and it creates no canonical record.
	var peopleBefore, peopleAfter int
	if err := fixture.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&peopleBefore); err != nil {
		t.Fatalf("count people: %v", err)
	}
	if err := fixture.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&peopleAfter); err != nil {
		t.Fatalf("count people: %v", err)
	}
	if peopleAfter != peopleBefore {
		t.Fatalf("a suggestion created %d people; a suggestion is not a person", peopleAfter-peopleBefore)
	}
}

func TestADecisionKeepsTheMachinesReadingBesideTheReviewersAnswer(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()

	err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-000001", PageNumber: 1, SuggestedTextAR: "عبداللـه", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.8, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	})
	if err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	suggestionID := queue[0].ID

	decided, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "عبدالله", NoteAR: "الحرف الأخير مقطوع"})
	if err != nil {
		t.Fatalf("decide suggestion: %v", err)
	}
	if decided.Status != SuggestionCorrected {
		t.Fatalf("expected a corrected suggestion, got %q", decided.Status)
	}
	if decided.DecidedTextAR != "عبدالله" {
		t.Fatalf("expected the reviewer's text, got %q", decided.DecidedTextAR)
	}
	// The machine's reading is kept, not overwritten.
	if decided.SuggestedTextAR != "عبداللـه" {
		t.Fatalf("the machine's reading was overwritten: %q", decided.SuggestedTextAR)
	}
	if decided.DecidedBy != ownerID {
		t.Fatalf("expected the decision to name the reviewer, got %q", decided.DecidedBy)
	}

	// The ledger keeps the decision, and it keeps what the machine said.
	var ledgerSuggested, ledgerDecided string
	if err := fixture.QueryRow(`SELECT suggested_text_ar, COALESCE(decided_text_ar, '') FROM source_tree_suggestion_decisions WHERE suggestion_id = $1`, suggestionID).Scan(&ledgerSuggested, &ledgerDecided); err != nil {
		t.Fatalf("read the decision ledger: %v", err)
	}
	if ledgerSuggested != "عبداللـه" || ledgerDecided != "عبدالله" {
		t.Fatalf("the ledger did not keep both readings: %q / %q", ledgerSuggested, ledgerDecided)
	}

	// A decided suggestion leaves the queue.
	queue, err = service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	if len(queue) != 0 {
		t.Fatalf("expected the queue to be empty, got %d", len(queue))
	}
}

func TestAnAcceptedReadingIsRecordedAsTheReviewersDecision(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-accept", PageNumber: 1, SuggestedTextAR: "صالح", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.8, ViewCount: 1, ReadingMethod: "local", ReadingModel: "test"},
	}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	if _, err := service.DecideSuggestion(ctx, queue[0].ID, ownerID, SuggestionDecision{Decision: SuggestionAccepted}); err != nil {
		t.Fatalf("accept suggestion: %v", err)
	}
	var ledgerText string
	if err := fixture.QueryRow(`SELECT COALESCE(decided_text_ar, '') FROM source_tree_suggestion_decisions WHERE suggestion_id = $1`, queue[0].ID).Scan(&ledgerText); err != nil {
		t.Fatalf("read decision ledger: %v", err)
	}
	if ledgerText != "صالح" {
		t.Fatalf("accepted ledger text = %q, want the accepted machine reading", ledgerText)
	}
}

func TestADecisionKeepsTheCandidateEvidenceWithoutTurningItIntoAName(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()

	err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-000001", PageNumber: 1, SuggestedTextAR: "عبداللـه", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.8, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	})
	if err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	suggestionID := queue[0].ID

	var peopleBefore int
	if err := fixture.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&peopleBefore); err != nil {
		t.Fatalf("count people before the decision: %v", err)
	}

	duration := 1234
	decided, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected,
		TextAR:   "عبدالله",
		CandidateSnapshot: []SuggestionCandidateSnapshot{
			{
				Value:       "عبدالله",
				Key:         "عبدالله",
				CompactKey:  "عبدالله",
				ParticleKey: "عبدالله",
				SkeletonKey: "عبدالله",
				Sources:     []string{"reader", "file"},
				Ranks:       map[string]int{"reader": 1, "file": 1},
				Score:       100,
				FileCount:   1,
				Shown:       true,
				Position:    1,
			},
			{
				Value:    "عبد اللطيف",
				Key:      "عبد اللطيف",
				Sources:  []string{"common"},
				Ranks:    map[string]int{"common": 2},
				Score:    70,
				Shown:    true,
				Position: 2,
			},
		},
		SelectedCandidate:    "عبدالله",
		InputMethod:          "candidate",
		JudgeOutput:          json.RawMessage(`{"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"عبدالله","probabilities":{"عبدالله":0.7,"عبد اللطيف":0.2,"unclear":0.1},"confidence":0.7,"abstained":false,"shown":false,"latencyMs":42}`),
		NormalizerVersion:    "ar-name-keys-v1",
		CandidatePoolVersion: "ar-name-pool-v1",
		DurationMS:           &duration,
	})
	if err != nil {
		t.Fatalf("decide suggestion with evidence: %v", err)
	}
	if decided.DecidedTextAR != "عبدالله" {
		t.Fatalf("the reviewer's answer changed: %q", decided.DecidedTextAR)
	}

	var selected, inputMethod, normalizerVersion, candidatePoolVersion, judgeChoice string
	var snapshotValue string
	var durationMS int
	if err := fixture.QueryRow(`
		SELECT candidate_snapshot->0->>'value',
		       COALESCE(selected_candidate, ''),
		       input_method,
		       normalizer_version,
		       candidate_pool_version,
		       judge_output->>'choice',
		       duration_ms
		FROM source_tree_suggestion_decisions
		WHERE suggestion_id = $1
	`, suggestionID).Scan(&snapshotValue, &selected, &inputMethod, &normalizerVersion, &candidatePoolVersion, &judgeChoice, &durationMS); err != nil {
		t.Fatalf("read decision evidence: %v", err)
	}
	if snapshotValue != "عبدالله" || selected != "عبدالله" || inputMethod != "candidate" {
		t.Fatalf("snapshot/selection = %q / %q / %q", snapshotValue, selected, inputMethod)
	}
	if normalizerVersion != "ar-name-keys-v1" || candidatePoolVersion != "ar-name-pool-v1" || judgeChoice != "عبدالله" || durationMS != duration {
		t.Fatalf("versions/judge/duration = %q / %q / %q / %d", normalizerVersion, candidatePoolVersion, judgeChoice, durationMS)
	}

	// Evidence is not a person and not a fact: the decision created no person.
	var peopleAfter int
	if err := fixture.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&peopleAfter); err != nil {
		t.Fatalf("count people after the decision: %v", err)
	}
	if peopleAfter != peopleBefore {
		t.Fatalf("the candidate evidence changed the people count from %d to %d", peopleBefore, peopleAfter)
	}
}

func TestAMalformedCandidateSnapshotIsRejected(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()

	err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-000001", PageNumber: 1, SuggestedTextAR: "محمد", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.9, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	})
	if err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	suggestionID := queue[0].ID

	tooMany := make([]SuggestionCandidateSnapshot, 0, maxDecisionCandidateSnapshot+1)
	for index := 0; index <= maxDecisionCandidateSnapshot; index++ {
		tooMany = append(tooMany, SuggestionCandidateSnapshot{
			Value: "اسم" + string(rune('a'+index)), Key: "اسم" + string(rune('a'+index)),
			Sources: []string{"common"}, Shown: true, Position: index + 1,
		})
	}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "محمد", CandidateSnapshot: tooMany,
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a snapshot over the bound = %v, want validation error", err)
	}

	one := []SuggestionCandidateSnapshot{{
		Value: "محمد", Key: "محمد", Sources: []string{"reader"}, Shown: true, Position: 1,
	}}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "محمد", CandidateSnapshot: one,
		SelectedCandidate: "خالد", InputMethod: "candidate",
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a selected candidate outside the snapshot = %v, want validation error", err)
	}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "محمد", CandidateSnapshot: one,
		InputMethod: "candidate",
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("candidate input with no selected candidate = %v, want validation error", err)
	}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "محمد", CandidateSnapshot: one,
		JudgeOutput: json.RawMessage(`{"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"خالد","probabilities":{"محمد":0.6,"unclear":0.4},"confidence":0.6,"abstained":false,"shown":false,"latencyMs":9}`),
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a judge choice outside the snapshot = %v, want validation error", err)
	}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "محمد", CandidateSnapshot: one,
		JudgeOutput: json.RawMessage(`{"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"محمد","confidence":0.6,"abstained":false,"shown":false,"latencyMs":9,"apiKey":"secret"}`),
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("unknown judge evidence fields = %v, want validation error", err)
	}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{
		Decision: SuggestionCorrected, TextAR: "محمد", CandidateSnapshot: one,
		JudgeOutput: json.RawMessage(`{"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"محمد","confidence":0.6,"abstained":false,"shown":true,"latencyMs":9}`),
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a shadow judge shown to the reviewer = %v, want validation error", err)
	}
}

func TestASuggestionCannotBeDecidedTwice(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()

	err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-000001", PageNumber: 1, SuggestedTextAR: "محمد", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.9, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	})
	if err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	if _, err := service.DecideSuggestion(ctx, queue[0].ID, ownerID, SuggestionDecision{Decision: SuggestionAccepted}); err != nil {
		t.Fatalf("first decision: %v", err)
	}
	if _, err := service.DecideSuggestion(ctx, queue[0].ID, ownerID, SuggestionDecision{Decision: SuggestionRejected}); !errors.Is(err, ErrSuggestionAlreadyDecided) {
		t.Fatalf("second decision = %v, want already decided", err)
	}
}

func TestACorrectionMustCarryTheCorrectionAndARejectionMustNot(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()

	err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-000001", PageNumber: 1, SuggestedTextAR: "محمد", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.9, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	})
	if err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	suggestionID := queue[0].ID

	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{Decision: SuggestionCorrected}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a correction with no text = %v, want validation error", err)
	}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{Decision: SuggestionRejected, TextAR: "محمد"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a rejection carrying text = %v, want validation error", err)
	}
	if _, err := service.DecideSuggestion(ctx, suggestionID, ownerID, SuggestionDecision{Decision: "invented"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("an unknown decision = %v, want validation error", err)
	}
}

func TestTheAccuracyIsMeasuredFromTheDecisionsAndRefusesToBeAPercentageTooSoon(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()

	suggestions := make([]SuggestionInput, 0, 4)
	for index := 0; index < 4; index++ {
		suggestions = append(suggestions, SuggestionInput{
			OccurrenceID: "carrier-" + string(rune('a'+index)), PageNumber: 1,
			SuggestedTextAR: "محمد", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01},
			Confidence: 0.9, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1",
		})
	}
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, suggestions); err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	// Two accepted, one corrected, one rejected.
	decisions := []SuggestionDecision{
		{Decision: SuggestionAccepted},
		{Decision: SuggestionAccepted},
		{Decision: SuggestionCorrected, TextAR: "محمود"},
		{Decision: SuggestionRejected},
	}
	for index, decision := range decisions {
		if _, err := service.DecideSuggestion(ctx, queue[index].ID, ownerID, decision); err != nil {
			t.Fatalf("decide suggestion %d: %v", index, err)
		}
	}

	accuracy, err := service.SuggestionAccuracy(ctx, fileID, ownerID)
	if err != nil {
		t.Fatalf("accuracy: %v", err)
	}
	if accuracy.Total != 4 || accuracy.Accepted != 2 || accuracy.Corrected != 1 || accuracy.Rejected != 1 {
		t.Fatalf("accuracy = %+v; want four decided, two accepted, one corrected, one rejected", accuracy)
	}
	// Two of the four decided readings were right without correction. A rejection
	// counts against the rate: the machine read something that was not right,
	// whether it read a name wrongly or read a shape that was not a name.
	if accuracy.Unchanged < 0.49 || accuracy.Unchanged > 0.51 {
		t.Fatalf("expected an unchanged rate of one half, got %v", accuracy.Unchanged)
	}
	// Four decisions is an anecdote, and the report says so rather than
	// presenting itself as a percentage.
	if accuracy.Measurable {
		t.Fatal("four decisions must not be reported as a measurable rate")
	}
}

func TestAReviewerWhoCannotReviewTheSourceIsRefused(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()
	outsider := actor.RegisterAndLogin(t, fixture, "غريب")

	err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{
		{OccurrenceID: "carrier-000001", PageNumber: 1, SuggestedTextAR: "محمد", Region: Region{X: 0.1, Y: 0.2, Width: 0.05, Height: 0.01}, Confidence: 0.9, ViewCount: 3, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1"},
	})
	if err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	if _, err := service.ListSuggestions(ctx, fileID, outsider.User.ID, "", 0, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an outsider listing the queue = %v, want forbidden", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	if _, err := service.DecideSuggestion(ctx, queue[0].ID, outsider.User.ID, SuggestionDecision{Decision: SuggestionAccepted}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an outsider deciding a suggestion = %v, want forbidden", err)
	}
}
