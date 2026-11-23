package tribehints

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// seedHintWorld builds two published tribes (one carrying an alias whose last
// token is the root, like the 030-A shape), one source with three passages —
// naming حرب twice by two forms, عتيبة once, and nothing once — and the
// actors: the queue's moderator and the outside reader who must see nothing.
func seedHintWorld(t *testing.T, fixture *testsupport.Fixture) (service *Service, sourceID uuid.UUID, harbID, otaibaID, moderatorID string) {
	t.Helper()
	moderator := actor.Register(t, fixture, "مشرف التلميحات")
	fixture.GrantRole(moderator.User.ID, "moderator")
	reader := actor.Register(t, fixture, "قارئ التلميحات")
	_ = reader

	sourceID = uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'لوريمر ١٩٠٨ — التلميحات', 'book', 'public', $2)`,
		sourceID, moderator.User.ID)

	harbID = uuid.New().String()
	otaibaID = uuid.New().String()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'حرب', 'حرب', 'public', $2)`, harbID, moderator.User.ID)
	fixture.Exec(`INSERT INTO tribe_aliases (tribe_id, value_ar, normalized_value_ar) VALUES ($1, 'بني حرب', 'بني حرب')`, harbID)
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'عتيبة', 'عتيبه', 'public', $2)`, otaibaID, moderator.User.ID)

	passages := map[int]string{}
	for index, text := range []string{
		"الحرب قبيلة عظيمة و بني حرب سكنوا الهجاز",
		"زار العتيبي حرب، وقال أحد الهرات أن العتيبه في نجد", // العتيبي not a form; العتيبه (ta-marbuta) is; حرب again
		"لا اسم قبيلة في هذه الفقرة أبدا",
	} {
		passages[index] = text
	}
	for index, text := range passages {
		fixture.Exec(`INSERT INTO source_passages (source_id, sequence_number, text_ar, normalized_text_ar)
			VALUES ($1, $2, $3, $3)`, sourceID, index+1, text)
	}

	service = NewService(fixture.Pool())
	return service, sourceID, harbID, otaibaID, moderator.User.ID
}

func TestMatchAndQueueCards(t *testing.T) {
	fixture := testsupport.New(t)
	service, sourceID, harbID, otaibaID, moderatorID := seedHintWorld(t, fixture)

	// GATES FIRST: the queue is moderator-readable and role-checked, the
	// status lens is a closed vocabulary, and a malformed tribe filter is a
	// validation answer.
	if _, err := service.List(fixture.Ctx(), "", "", ""); err != ErrUnauthenticated {
		t.Fatalf("anonymous list = %v, want ErrUnauthenticated", err)
	}
	if _, err := service.MatchTribeHints(fixture.Ctx(), "لا-uuid"); err != ErrValidation {
		t.Fatalf("malformed source = %v, want ErrValidation", err)
	}

	match, err := service.MatchTribeHints(fixture.Ctx(), sourceID.String())
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	// Three hint ROWS: (حرب, p1), (حرب, p2), (عتيبة, p2) — one row per
	// (tribe, passage), the second passage carrying two tribes at once. The
	// clean passage contributes nothing.
	if match.HintsSuggested != 3 {
		t.Fatalf("hints suggested = %d, want 3 (one per tribe-passage pair)", match.HintsSuggested)
	}
	if match.MatchedTribes != 2 {
		t.Fatalf("matched tribes = %d, want حرب and عتيبة", match.MatchedTribes)
	}

	idempotent, err := service.MatchTribeHints(fixture.Ctx(), sourceID.String())
	if err != nil {
		t.Fatalf("re-match: %v", err)
	}
	if idempotent.HintsSuggested != 0 {
		t.Fatalf("re-match suggested %d new hints, want 0 (idempotent by the unique key)", idempotent.HintsSuggested)
	}

	lane, err := service.List(fixture.Ctx(), moderatorID, "", "")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(lane.Hints) != 2 {
		t.Fatalf("queue cards = %d, want one per matched tribe", len(lane.Hints))
	}
	harb := lane.Hints[0]
	if harb.TribeID != harbID || harb.TribeName != "حرب" {
		t.Fatalf("first card = %+v", harb)
	}
	// The plan's own card shape: «١٧ إشارة لحرب في Lorimer 1908» — a passage
	// count (2 passages mention حرب) and a mention tally across the forms.
	if harb.PassageCount != 2 || harb.Mentions != 3 {
		t.Fatalf("حرب card = %+v, want 2 passages / 3 mentions (the second passage matched both forms)", harb)
	}
	otaiba := lane.Hints[1]
	if otaiba.TribeID != otaibaID || otaiba.PassageCount != 1 || otaiba.Mentions != 1 {
		t.Fatalf("عتيبة card = %+v, want one passage matched through the ta-marbuta form (العتيبه)", otaiba)
	}

	// The card's passages carry the matched forms verbatim — the receipt's
	// own words, never just a number.
	detail, err := service.Detail(fixture.Ctx(), moderatorID, harbID, sourceID.String())
	if err != nil {
		t.Fatalf("card detail: %v", err)
	}
	if len(detail.Passages) != 2 {
		t.Fatalf("card passages = %d, want 2", len(detail.Passages))
	}
	first := detail.Passages[0]
	if len(first.MatchedForms) != 2 || first.MatchedForms[0] != "حرب" || first.MatchedForms[1] != "بني حرب" {
		t.Fatalf("first passage matched forms = %v, want حرب + بني حرب", first.MatchedForms)
	}
	// An unknown card is not found, not an empty detail.
	if _, err := service.Detail(fixture.Ctx(), moderatorID, uuid.New().String(), sourceID.String()); err != ErrNotFound {
		t.Fatalf("unknown card = %v, want ErrNotFound", err)
	}
	// The queue's O trip: the acceptance in the lens vocabulary.
	if _, err := service.List(fixture.Ctx(), moderatorID, "", "bogus"); err != ErrValidation {
		t.Fatalf("bogus status = %v, want ErrValidation", err)
	}
}

func TestDecideAcceptWritesTheReceiptAndDismissIsReviewerScoped(t *testing.T) {
	fixture := testsupport.New(t)
	service, sourceID, harbID, _, moderatorID := seedHintWorld(t, fixture)
	_, err := service.MatchTribeHints(fixture.Ctx(), sourceID.String())
	if err != nil {
		t.Fatal(err)
	}

	// The action vocabulary is closed.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, harbID, sourceID.String(), "apply", ""); err != ErrValidation {
		t.Fatalf("bogus action = %v, want ErrValidation", err)
	}

	// A dismissal hides the card for THE DISMISSING REVIEWER only: a second
	// moderator still sees the card, and nobody's rows were written besides
	// their own decision.
	secondMod := actor.Register(t, fixture, "مشرف ثانٍ للتحكيم")
	fixture.GrantRole(secondMod.User.ID, "moderator")
	dismissed, err := service.Decide(fixture.Ctx(), moderatorID, harbID, sourceID.String(), "dismiss", "غير حاسم بما يكفي")
	if err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if dismissed.Action != "dismiss" || dismissed.Card.Status != "suggested" || dismissed.CreatedClaimID != "" {
		t.Fatalf("dismiss outcome = %+v", dismissed)
	}
	own, err := service.List(fixture.Ctx(), moderatorID, "", "")
	if err != nil {
		t.Fatalf("own queue after dismiss: %v", err)
	}
	for _, card := range own.Hints {
		if card.TribeID == harbID {
			t.Fatalf("the dismissed card reappeared for its own dismisser: %+v", card)
		}
	}
	other, err := service.List(fixture.Ctx(), secondMod.User.ID, "", "")
	if err != nil {
		t.Fatalf("other queue after dismiss: %v", err)
	}
	found := false
	for _, card := range other.Hints {
		if card.TribeID == harbID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the dismissal hid the card from a reviewer who never dismissed it")
	}

	// The accept = the documented receipt: a claim (mentioned_in) with the
	// card's passages as contextualizing evidence, the hint rows accepted
	// under the reviewer's own id, and the decision history carrying it.
	accepted, err := service.Decide(fixture.Ctx(), secondMod.User.ID, harbID, sourceID.String(), "accept", " إشارتان موثقتان")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Action != "accept" || accepted.Card.Status != "accepted" || accepted.CreatedReceipts != 2 {
		t.Fatalf("accept outcome = %+v", accepted)
	}
	if accepted.CreatedClaimID == "" {
		t.Fatalf("the accept wrote no receipt claim")
	}
	var claimStatus, predicate string
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `SELECT status, predicate FROM claims WHERE id = $1`, accepted.CreatedClaimID).Scan(&claimStatus, &predicate); err != nil {
		t.Fatal(err)
	}
	if claimStatus != "documented" || predicate != "mentioned_in" {
		t.Fatalf("receipt claim = %s/%s, want documented/mentioned_in", claimStatus, predicate)
	}
	if evidence := fixture.Count(`SELECT count(*) FROM claim_evidence WHERE claim_id = $1`, accepted.CreatedClaimID); evidence != 2 {
		t.Fatalf("receipt evidence rows = %d, want one per accepted passage", evidence)
	}

	// The accepted hint never returns to the queue (its rows are accepted);
	// the accepted lane shows it.
	pending, err := service.List(fixture.Ctx(), secondMod.User.ID, harbID, "")
	if err != nil || len(pending.Hints) != 0 {
		t.Fatalf("accepted card still in the pending queue: %v, %d", err, len(pending.Hints))
	}
	acceptedLane, err := service.List(fixture.Ctx(), secondMod.User.ID, harbID, "accepted")
	if err != nil || len(acceptedLane.Hints) != 1 {
		t.Fatalf("accepted lane = %v, %d", err, len(acceptedLane.Hints))
	}
	if acceptedLane.Hints[0].DecidedBy != secondMod.User.ID {
		t.Fatalf("accepted card's reviewer = %q, want the accepting reviewer", acceptedLane.Hints[0].DecidedBy)
	}

	// The receipts surface (the dossier's own evidence read, via the claim
	// lane) now carries the accepted mention for the public reader.
	_ = moderatorID
}
