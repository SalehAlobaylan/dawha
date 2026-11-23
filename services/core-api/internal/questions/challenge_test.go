package questions

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// seedChallengeable builds one published tribe whose documented claim carries
// public evidence, plus the actors the arc names: the claim's owner, a plain
// challenger (any signed-in reader), and the adjudicating moderator.
func seedChallengeable(t *testing.T, fixture *testsupport.Fixture) (service *Service, tribeID, claimID, ownerID, challengerID, moderatorID uuid.UUID) {
	t.Helper()
	owner := actor.Register(t, fixture, "مالك المطالبة")
	fixture.GrantRole(owner.User.ID, "researcher")
	challenger := actor.Register(t, fixture, "طاعن المطالبة")
	fixture.GrantRole(challenger.User.ID, "researcher")
	moderator := actor.Register(t, fixture, "محكّم الخلاف")
	fixture.GrantRole(moderator.User.ID, "moderator")

	tribe := uuid.New()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'قبيلة الخلاف', 'قبيله الخلاف', 'public', $2)`, tribe, owner.User.ID)
	place := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility, created_by)
		VALUES ($1, 'دار الخلاف', 'دار الخلاف', 'region', 'public', $2)`, place, owner.User.ID)

	claim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
		VALUES ($1, 'tribe', $2, 'originated_in', 'place', $3, 'documented', 'مطالبة موثقة', $4)`,
		claim, tribe, place, owner.User.ID)

	// The claim's public evidence: one receipt on a public source.
	source := uuid.New()
	passage := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'لوريمر الخلاف (رحلة ٦١)', 'book', 'public', $2)`, source, owner.User.ID)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'شاهد الرحلة', 'شاهد الرحلة')`, passage, source)
	fixture.Exec(`INSERT INTO claim_evidence (id, claim_id, source_passage_id, relation, created_by)
		VALUES ($1, $2, $3, 'supports', $4)`, uuid.New(), claim, passage, owner.User.ID)

	return NewService(fixture.Pool()), tribe, claim, uuid.MustParse(owner.User.ID), uuid.MustParse(challenger.User.ID), uuid.MustParse(moderator.User.ID)
}

func TestClaimChallengeArc(t *testing.T) {
	fixture := testsupport.New(t)
	service, _, claimID, _, challengerID, moderatorID := seedChallengeable(t, fixture)

	// The challenger and the moderator are messages, not magic: bad uuids are
	// validations.
	if _, err := service.ChallengeClaim(fixture.Ctx(), "ولا شيء", challengerID.String(), ChallengeClaimInput{TitleAR: "الطعن"}); err != ErrValidation {
		t.Fatalf("bad claim id = %v, want ErrValidation", err)
	}
	if _, err := service.ChallengeClaim(fixture.Ctx(), claimID.String(), "ولا شيء", ChallengeClaimInput{TitleAR: "الطعن"}); err != ErrForbidden {
		t.Fatalf("bad actor = %v, want ErrForbidden", err)
	}

	// The arc: a reader challenges a documented claim; the claim's own row
	// flips inside the challenge's transaction.
	thread, err := service.ChallengeClaim(fixture.Ctx(), claimID.String(), challengerID.String(), ChallengeClaimInput{
		TitleAR:       "طعن في النسبة",
		DescriptionAR: "المصدر يقول غير ما تدّعيه",
	})
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if thread.Dispute.Status != "open" {
		t.Fatalf("thread status = %q, want open", thread.Dispute.Status)
	}
	if len(thread.Claims) != 1 || thread.Claims[0].Position != "opposes" || thread.Claims[0].Status != "disputed" {
		t.Fatalf("thread claims = %+v", thread.Claims)
	}

	// EVERY SURFACE, in the same moment: the claim row reads `disputed` now —
	// the dictionary's own claim read (the badge's carrier) is the row.
	var currentStatus string
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `SELECT status FROM claims WHERE id = $1`, claimID).Scan(&currentStatus); err != nil {
		t.Fatal(err)
	}
	if currentStatus != "disputed" {
		t.Fatalf("claim status after challenge = %q, want disputed", currentStatus)
	}

	// The challenger cannot close their own thread: adjudication is the
	// moderator's.
	if _, err := service.AdjudicateChallenge(fixture.Ctx(), thread.Dispute.ID, challengerID.String(), AdjudicateChallengeInput{
		ClaimStatus:  "documented",
		ResolutionAR: "حسم لصالح المطالبة",
	}); err != ErrForbidden {
		t.Fatalf("challenger adjudication = %v, want ErrForbidden", err)
	}

	// The outcome is spelled: only two states settle a challenge.
	if _, err := service.AdjudicateChallenge(fixture.Ctx(), thread.Dispute.ID, moderatorID.String(), AdjudicateChallengeInput{
		ClaimStatus:  "supported",
		ResolutionAR: "التسمية خاطئة",
	}); err != ErrValidation {
		t.Fatalf("invalid outcome = %v, want ErrValidation", err)
	}
	if _, err := service.AdjudicateChallenge(fixture.Ctx(), thread.Dispute.ID, moderatorID.String(), AdjudicateChallengeInput{
		ClaimStatus:  "documented",
		ResolutionAR: "",
	}); err != ErrValidation {
		t.Fatalf("missing resolution = %v, want ErrValidation", err)
	}

	// The moderator settles: the thread resolves with their name, and the
	// challenged claims land in the spelled outcome.
	settled, err := service.AdjudicateChallenge(fixture.Ctx(), thread.Dispute.ID, moderatorID.String(), AdjudicateChallengeInput{
		ClaimStatus:  "documented",
		ResolutionAR: "حسم لصالح المطالبة بعد قراءة الشاهد",
	})
	if err != nil {
		t.Fatalf("adjudication: %v", err)
	}
	if settled.Dispute.Status != "resolved" || settled.Dispute.ResolutionAR == "" {
		t.Fatalf("settled thread = %+v", settled.Dispute)
	}
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `SELECT status FROM claims WHERE id = $1`, claimID).Scan(&currentStatus); err != nil {
		t.Fatal(err)
	}
	if currentStatus != "documented" {
		t.Fatalf("claim status after adjudication = %q, want documented", currentStatus)
	}

	// A resolved thread does not adjudicate twice; its own reopen lifecycle
	// (upstream, unchanged) owns the second round.
	if _, err := service.AdjudicateChallenge(fixture.Ctx(), thread.Dispute.ID, moderatorID.String(), AdjudicateChallengeInput{
		ClaimStatus:  "rejected",
		ResolutionAR: "محاولة ثانية",
	}); err != ErrValidation {
		t.Fatalf("re-adjudication of a resolved thread = %v, want ErrValidation", err)
	}

	// The audit trail carries the arc.
	if events := fixture.Count(`SELECT count(*) FROM audit_log WHERE action IN ('claim_challenged', 'dispute_adjudicated') AND entity_id = $1`, thread.Dispute.ID); events == 0 {
		t.Fatalf("the challenge arc left no audit trail")
	}
}

func TestChallengeGuardsOnInvisibleAndSettledClaims(t *testing.T) {
	fixture := testsupport.New(t)
	service, _, claimID, ownerID, _, _ := seedChallengeable(t, fixture)

	// A settled claim needs no thread: its state is the answer. The gate says
	// validation (what is being asked cannot be asked of this row), not a
	// refusal that would name the state as secret.
	fixture.Exec(`UPDATE claims SET status = 'rejected' WHERE id = $1`, claimID)
	if _, err := service.ChallengeClaim(fixture.Ctx(), claimID.String(), uuid.New().String(), ChallengeClaimInput{TitleAR: "طعن"}); err != ErrValidation {
		t.Fatalf("challenging a rejected claim = %v, want ErrValidation", err)
	}

	// A claim that is not readable is not challengeable: to a PLAIN reader —
	// no roles at all — an evidence-less claim answers as a missing row, not a
	// refusal that names it. (A researcher's own grant is wider by design; the
	// gate under test here is the plain reader's.)
	plain := actor.Register(t, fixture, "قارئ بلا أدوار")
	hidden := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by)
		VALUES ($1, 'tribe', $2, 'originated_in', 'place', $3, 'documented', $4)`,
		hidden, uuid.New(), uuid.New(), ownerID)
	if _, err := service.ChallengeClaim(fixture.Ctx(), hidden.String(), plain.User.ID, ChallengeClaimInput{TitleAR: "طعن"}); err != ErrNotFound {
		t.Fatalf("challenging an unreadable claim = %v, want ErrNotFound", err)
	}
}
