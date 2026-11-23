package tribepairings

import (
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// seedLane builds the pair's world: two published tribes with DIFFERENT names
// (the builder's own similarity is not re-tested here), a suggested candidate
// between them, and the actors: a moderator, the pair's second moderator, and
// a plain reader who must see nothing.
func seedLane(t *testing.T, fixture *testsupport.Fixture) (service *Service, candidateID, tribeAID, tribeBID, moderatorID, secondModID, readerID string) {
	t.Helper()
	owner := actor.Register(t, fixture, "مالك المسلك")
	fixture.GrantRole(owner.User.ID, "researcher")
	moderator := actor.Register(t, fixture, "مشرف المِسلك")
	fixture.GrantRole(moderator.User.ID, "moderator")
	second := actor.Register(t, fixture, "مشرف المِسلك الثاني")
	fixture.GrantRole(second.User.ID, "moderator")
	reader := actor.Register(t, fixture, "قارئ المِسلك")

	tribeA := uuid.New()
	tribeB := uuid.New()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'قبيلة الحرب الكبرى', 'قبيله الحرب الكبري', 'public', $2)`, tribeA, owner.User.ID)
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'الحرب', 'الحرب', 'public', $2)`, tribeB, owner.User.ID)

	candidate := uuid.New()
	fixture.Exec(`INSERT INTO tribe_duplicate_candidates
		(id, tribe_a_id, tribe_b_id, matched_a, matched_b, name_similarity, status)
		VALUES ($1, $2, $3, 'قبيلة الحرب الكبرى', 'الحرب', 0.72, 'suggested')`,
		candidate, tribeA, tribeB)

	return NewService(fixture.Pool()), candidate.String(), tribeA.String(), tribeB.String(), moderator.User.ID, second.User.ID, reader.User.ID
}

func TestLaneGateAndGuards(t *testing.T) {
	fixture := testsupport.New(t)
	service, candidateID, _, _, moderatorID, _, readerID := seedLane(t, fixture)

	if _, err := service.List(fixture.Ctx(), readerID, ""); err != ErrForbidden {
		t.Fatalf("reader list = %v, want ErrForbidden", err)
	}
	if _, err := service.List(fixture.Ctx(), "", ""); err != ErrUnauthenticated {
		t.Fatalf("anonymous list = %v, want ErrUnauthenticated", err)
	}
	if _, err := service.List(fixture.Ctx(), moderatorID, "bogus"); err != ErrValidation {
		t.Fatalf("bogus status = %v, want ErrValidation", err)
	}
	if _, err := service.Decide(fixture.Ctx(), readerID, candidateID, "reject", "", ""); err != ErrForbidden {
		t.Fatalf("reader decide = %v, want ErrForbidden", err)
	}
	// The canonical must be one of the pair: a third-party canonical would be
	// a merge by another name.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, candidateID, "link_alias", "00000000-0000-4000-8000-00000000dead", ""); err != ErrValidation {
		t.Fatalf("third-party canonical = %v, want ErrValidation", err)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, candidateID, "burn", "", ""); err != ErrValidation {
		t.Fatalf("unknown action = %v, want ErrValidation", err)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, uuid.New().String(), "reject", "", ""); err != ErrNotFound {
		t.Fatalf("unknown candidate = %v, want ErrNotFound", err)
	}
	if _, err := service.History(fixture.Ctx(), readerID, candidateID); err != ErrForbidden {
		t.Fatalf("reader history = %v, want ErrForbidden", err)
	}
}

// TestLinkAliasEffectIsAnAliasInsertOnly is the lane's own law, pinned: the
// link-alias effect INSERTS the loser's form as the canonical's alias and
// NOTHING else — the loser's tribal row keeps its own canonical name
// (no UPDATE on tribes exists in this lane), and exactly this new alias row
// exists afterwards.
func TestLinkAliasEffectIsAnAliasInsertOnly(t *testing.T) {
	fixture := testsupport.New(t)
	service, candidateID, tribeAID, tribeBID, moderatorID, _, _ := seedLane(t, fixture)

	decided, err := service.Decide(fixture.Ctx(), moderatorID, candidateID, "link_alias", tribeBID, "الاسم الأشهر هو الحرب")
	if err != nil {
		t.Fatalf("link-alias decide: %v", err)
	}
	if decided.Candidate.Status != "linked" || decided.Action != "link_alias" {
		t.Fatalf("link-alias outcome = %+v", decided)
	}
	if decided.Candidate.Canonical != tribeBID {
		t.Fatalf("canonical = %q, want the reviewer's pick", decided.Candidate.Canonical)
	}

	// The effect: the loser's matched form now reads as an alias of the
	// canonical — and NOTHING else changed.
	if aliases := fixture.Count(`SELECT count(*) FROM tribe_aliases WHERE tribe_id = $1`, tribeBID); aliases != 1 {
		t.Fatalf("canonical's aliases = %d, want the one linked alias", aliases)
	}
	var value, normalized string
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `SELECT value_ar, normalized_value_ar FROM tribe_aliases WHERE tribe_id = $1`, tribeBID).Scan(&value, &normalized); err != nil {
		t.Fatal(err)
	}
	if value != "قبيلة الحرب الكبرى" {
		t.Fatalf("linked alias = %q, want the loser's own matched form verbatim", value)
	}
	// The loser's own row: untouched (its name is its own).
	var loserCanonical string
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `SELECT canonical_name_ar FROM tribes WHERE id = $1`, tribeAID).Scan(&loserCanonical); err != nil {
		t.Fatal(err)
	}
	if loserCanonical != "قبيلة الحرب الكبرى" {
		t.Fatalf("the loser's canonical = %q; this lane never rewrites a tribes row", loserCanonical)
	}

	// A second decide on the same candidate is a conflict; the decisions are
	// append-only.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, candidateID, "reject", "", ""); err != ErrAlreadyDecided {
		t.Fatalf("second decide = %v, want ErrAlreadyDecided", err)
	}
	history, err := service.History(fixture.Ctx(), moderatorID, candidateID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.History) != 1 || history.History[0].Action != "link_alias" {
		t.Fatalf("history = %+v", history.History)
	}
	if history.History[0].Diff == "" || !strings.Contains(history.History[0].Diff, "canonical") {
		t.Fatalf("decision diff = %q, want the canonical named", history.History[0].Diff)
	}
}

// TestCanonizeEffectIsAPointerRelationshipOnly pins the pointed effect: an
// append-only entity_relationship row names the loser as the canonical's
// shadow, and the tribe rows stay exactly as they were; the pointers' own
// dedupe keeps a decision's record from doubling.
func TestCanonizeEffectIsAPointerRelationshipOnly(t *testing.T) {
	fixture := testsupport.New(t)
	service, candidateID, tribeAID, tribeBID, _, secondModID, _ := seedLane(t, fixture)

	decided, err := service.Decide(fixture.Ctx(), secondModID, candidateID, "canonize", tribeAID, "الحرب الكبرى هو الأصل")
	if err != nil {
		t.Fatalf("canonize decide: %v", err)
	}
	if decided.Candidate.Status != "pointed" || decided.Candidate.Canonical != tribeAID {
		t.Fatalf("canonize outcome = %+v", decided)
	}
	pointers := fixture.Count(`SELECT count(*) FROM entity_relationships
		WHERE subject_type = 'tribe' AND subject_id = $1 AND predicate = 'canonical_pointer' AND object_id = $2`,
		tribeBID, tribeAID)
	if pointers != 1 {
		t.Fatalf("pointer relationships = %d, want 1", pointers)
	}
	// Tribes' rows continue to be 此 lane's untouchable: nothing added, nothing
	// renamed.
	if tribes := fixture.Count(`SELECT count(*) FROM tribe_aliases WHERE tribe_id IN ($1, $2)`, tribeAID, tribeBID); tribes != 0 {
		t.Fatalf("canonize wrote %d alias rows; the effect is the pointer only", tribes)
	}
	if _, err := service.Decide(fixture.Ctx(), secondModID, candidateID, "canonize", tribeAID, ""); err != ErrAlreadyDecided {
		t.Fatalf("second decide = %v, want ErrAlreadyDecided", err)
	}
}

// TestRejectEffectWritesOnlyTheDecision is the lane's quiet arm: the reject's
// own effect = the candidate's state and the history, and nothing in the
// dictionary (no aliases, no relationships).
func TestRejectEffectWritesOnlyTheDecision(t *testing.T) {
	fixture := testsupport.New(t)
	service, candidateID, tribeAID, tribeBID, moderatorID, _, _ := seedLane(t, fixture)

	decided, err := service.Decide(fixture.Ctx(), moderatorID, candidateID, "reject", "", "لا يوجد تكرار هنا")
	if err != nil {
		t.Fatalf("reject decide: %v", err)
	}
	if decided.Candidate.Status != "rejected" || decided.Candidate.Canonical != "" {
		t.Fatalf("reject outcome = %+v", decided)
	}
	if aliases := fixture.Count(`SELECT count(*) FROM tribe_aliases WHERE tribe_id IN ($1, $2)`, tribeAID, tribeBID); aliases != 0 {
		t.Fatalf("a reject wrote %d aliases", aliases)
	}
}
