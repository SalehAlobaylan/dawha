package questions

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
)

// The synthetic people every fixture schema carries from db/seeds/001_demo.sql.
const (
	seededPersonOne = "10000000-0000-0000-0000-000000000001"
	seededPersonTwo = "10000000-0000-0000-0000-000000000002"
)

// The open question and the dispute are the two places the product is allowed to
// say "we do not know yet", so the journeys below pin that a question can be
// opened, worked and left open, and that a dispute records competing positions
// instead of silently picking one.
func TestOpenQuestionCanBeCreatedWorkedAndLeftOpen(t *testing.T) {
	fixture := testsupport.New(t)
	asker := actor.RegisterAndLogin(t, fixture, "صاحب السؤال")
	service := NewService(fixture.Pool())
	title := "هل والد عبدالله في هذا السجل " + fixture.Tag() + "؟"

	question, err := service.CreateQuestion(fixture.Ctx(), asker.User.ID, CreateQuestionInput{
		TitleAR:       title,
		DescriptionAR: "روايتان تذكران أباً مختلفاً، والمصدر الأقدم لم يُراجع بعد.",
		Status:        "open",
		Priority:      "high",
	})
	if err != nil {
		t.Fatalf("create question: %v", err)
	}
	if question.Question.TitleAR != title || question.Question.Status != "open" {
		t.Fatalf("question = %+v, want the title and open status it was given", question.Question)
	}

	worked, err := service.AddNote(fixture.Ctx(), question.Question.ID, asker.User.ID, QuestionNoteInput{
		NoteAR: "الخطوة التالية: مطابقة الاسم المختصر " + fixture.Tag() + " بسجل_SL.",
	})
	if err != nil {
		t.Fatalf("add note: %v", err)
	}
	if len(worked.Notes) != 1 {
		t.Fatalf("notes = %d, want 1", len(worked.Notes))
	}
	if got := fixture.Count(`SELECT count(*) FROM open_questions WHERE id = $1 AND status = 'open'`, question.Question.ID); got != 1 {
		t.Fatalf("open question rows = %d, want 1 still open", got)
	}
	// The activity trail is what makes an unfinished question auditable rather
	// than a bare row.
	actions := map[string]bool{}
	for _, entry := range worked.Activity {
		actions[entry.Action] = true
	}
	for _, action := range []string{"question_created", "question_note_added"} {
		if !actions[action] {
			t.Fatalf("question activity %v is missing %q", actions, action)
		}
	}

	// A platform researcher may take the question further; it is not the
	// asker's private notebook, and an ordinary registrant is not a reviewer.
	second := actor.RegisterAndLogin(t, fixture, "باحث ثانٍ")
	if _, err := service.UpdateQuestion(fixture.Ctx(), question.Question.ID, second.User.ID, UpdateQuestionInput{
		Status:   "under_investigation",
		Priority: "normal",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("update by a plain registrant = %v, want ErrForbidden", err)
	}
	fixture.GrantRole(second.User.ID, "researcher")
	updated, err := service.UpdateQuestion(fixture.Ctx(), question.Question.ID, second.User.ID, UpdateQuestionInput{
		Status:   "under_investigation",
		Priority: "normal",
	})
	if err != nil {
		t.Fatalf("update question by a second researcher: %v", err)
	}
	if updated.Question.Status != "under_investigation" {
		t.Fatalf("status = %s, want under_investigation", updated.Question.Status)
	}
}

func TestDisputeKeepsBothPositionsOpen(t *testing.T) {
	fixture := testsupport.New(t)
	asker := actor.RegisterAndLogin(t, fixture, "صاحب الخلاف")
	service := NewService(fixture.Pool())
	supported := aClaim(t, fixture, asker.User.ID)
	contested := aClaim(t, fixture, asker.User.ID)

	dispute, err := service.CreateDispute(fixture.Ctx(), asker.User.ID, CreateDisputeInput{
		TitleAR:       "أب عبدالله: " + fixture.Tag(),
		DescriptionAR: "السجل الأول يذكر محمداً، والثاني يذكر سعيداً.",
		Status:        "open",
	})
	if err != nil {
		t.Fatalf("create dispute: %v", err)
	}
	if dispute.Dispute.Status != "open" {
		t.Fatalf("dispute status = %s, want open", dispute.Dispute.Status)
	}
	linked, err := service.LinkDisputeClaim(fixture.Ctx(), dispute.Dispute.ID, asker.User.ID, DisputeClaimInput{
		ClaimID:  supported.ID,
		Position: "supports",
	})
	if err != nil {
		t.Fatalf("link the supported position: %v", err)
	}
	if len(linked.Claims) != 1 || linked.Claims[0].Position != "supports" {
		t.Fatalf("dispute claims = %+v, want one supporting position", linked.Claims)
	}
	// The counterpart claim is recorded with its own position rather than
	// replacing the first one.
	if _, err := service.LinkDisputeClaim(fixture.Ctx(), dispute.Dispute.ID, asker.User.ID, DisputeClaimInput{
		ClaimID:  contested.ID,
		Position: "opposes",
	}); err != nil {
		t.Fatalf("link the opposing position: %v", err)
	}
	final, err := service.GetDispute(fixture.Ctx(), dispute.Dispute.ID)
	if err != nil {
		t.Fatalf("read the dispute: %v", err)
	}
	if len(final.Claims) != 2 {
		t.Fatalf("dispute claims = %d, want both positions kept", len(final.Claims))
	}
	if got := fixture.Count(`SELECT count(*) FROM disputes WHERE id = $1 AND status = 'open'`, dispute.Dispute.ID); got != 1 {
		t.Fatalf("open dispute rows = %d, want 1", got)
	}
	// Resolving is a decision with a stated reason, not a silent status flip.
	resolved, err := service.UpdateDispute(fixture.Ctx(), dispute.Dispute.ID, asker.User.ID, UpdateDisputeInput{
		Status:       "resolved",
		ResolutionAR: "رجّح السجل الأقدم، وبقي الخلاف محفوظاً للمراجعة.",
	})
	if err != nil {
		t.Fatalf("resolve dispute: %v", err)
	}
	if resolved.Dispute.Status != "resolved" {
		t.Fatalf("resolved status = %s, want resolved", resolved.Dispute.Status)
	}
	if got := fixture.Count(`SELECT count(*) FROM dispute_claims WHERE dispute_id = $1`, dispute.Dispute.ID); got != 2 {
		t.Fatalf("dispute claim rows after resolving = %d, want 2", got)
	}
}

func TestQuestionCanLinkItsDisputeAndKeepTheCount(t *testing.T) {
	fixture := testsupport.New(t)
	asker := actor.RegisterAndLogin(t, fixture, "صاحب السؤال")
	service := NewService(fixture.Pool())

	question, err := service.CreateQuestion(fixture.Ctx(), asker.User.ID, CreateQuestionInput{
		TitleAR:  "من كان والد(child " + fixture.Tag() + ")؟",
		Status:   "open",
		Priority: "normal",
	})
	if err != nil {
		t.Fatalf("create question: %v", err)
	}
	dispute, err := service.CreateDispute(fixture.Ctx(), asker.User.ID, CreateDisputeInput{
		TitleAR: "الروايتان " + fixture.Tag(),
		Status:  "open",
	})
	if err != nil {
		t.Fatalf("create dispute: %v", err)
	}
	linked, err := service.LinkDispute(fixture.Ctx(), question.Question.ID, asker.User.ID, QuestionDisputeInput{
		DisputeID: dispute.Dispute.ID,
	})
	if err != nil {
		t.Fatalf("link dispute: %v", err)
	}
	if len(linked.Disputes) != 1 || linked.Disputes[0].DisputeID != dispute.Dispute.ID {
		t.Fatalf("linked disputes = %+v, want the dispute it was given", linked.Disputes)
	}
	// The link is recorded from both sides so neither the question nor the
	// dispute can claim the other knows nothing about it.
	if got := fixture.Count(`SELECT count(*) FROM question_disputes WHERE question_id = $1 AND dispute_id = $2`,
		question.Question.ID, dispute.Dispute.ID); got != 1 {
		t.Fatalf("question_disputes rows = %d, want 1", got)
	}
	if _, err := service.LinkDispute(fixture.Ctx(), question.Question.ID, asker.User.ID, QuestionDisputeInput{
		DisputeID: "00000000-0000-4000-8000-000000000000",
	}); err == nil {
		t.Fatal("link a dispute id that does not exist succeeded")
	}
}

// aClaim creates a claim through the production evidence service so a dispute
// is attached to a real assertion rather than to a bare identifier. The two
// subjects are people from the synthetic seed every fixture schema carries.
func aClaim(t *testing.T, fixture *testsupport.Fixture, ownerID string) evidence.ClaimView {
	t.Helper()
	claim, err := evidence.NewService(fixture.Pool()).CreateClaim(fixture.Ctx(), ownerID, evidence.CreateClaimInput{
		SubjectType: "person",
		SubjectID:   seededPersonOne,
		Predicate:   "father_of",
		ObjectType:  "person",
		ObjectID:    seededPersonTwo,
		Status:      "disputed",
		NotesAR:     "أبوтен competing " + fixture.Tag(),
	})
	if err != nil {
		t.Fatalf("create claim: %v", err)
	}
	return claim
}
