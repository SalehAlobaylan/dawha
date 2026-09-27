package research

// AN UNSATISFIABLE FILTER MUST NOT LOOK LIKE A QUESTION WITH NO EVIDENCE.
//
// `loadWorkspaceClaims` restricted the question's claims to the workspace's
// subject with
//
//	AND ($2 = '' OR ((c.subject_type = $2 AND c.subject_id = $3::uuid) OR ...))
//
// The test for "no subject" compared $2 with the empty string. $2 is the subject's
// TYPE and is never NULL, so that part worked. $3 is the subject's ID and IS NULL
// whenever no subject was resolved - and a name with no id cannot match any claim,
// so a filter that cannot be satisfied returned nothing. The workspace then
// rendered a question with claims as a question with no claims, which is the
// failure mode `search_sources` in the research agent had, in the function next
// door.
//
// It is worth being exact about reachability, because it changes what this fix is
// for. Through the HTTP boundary the pair cannot be half-specified today:
// `?entity_type=person` with no `entity_id` is refused by validateWorkspaceInput
// before a query is built, and resolveWorkspaceEntity sets both halves or neither.
// So this was a LATENT trap in a package-private function rather than a live
// incident - but the correctness of a private function rested on an invariant
// enforced two functions away, and the query was written to be unsatisfiable
// rather than merely wrong. `loadWorkspaceEvidence` and `loadWorkspaceSources`
// were correct only by accident: their empty-string test happened to land on the
// non-NULL argument. All three now share one fragment and one argument builder, so
// the accident cannot be a third answer.
//
// Three states, and the third is the one this test exists for:
//
//   - no subject named and none resolved: NO constraint. The question's claims
//     come back, with their evidence and sources.
//   - a subject type and a real id: the filter narrows to that subject.
//   - a subject type and no id: REFUSED. It cannot be satisfied, and returning
//     the question's whole material would make `?entity_type=person` a way to
//     read a question without naming a subject.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
)

// subjectFixture is one question with two claims: one about the seeded person and
// one about a place, each with an accepted statement behind it. The question has
// no question_sources row, so the only route from a claim to a source is through
// claim_evidence - which means a restriction that excluded every claim would
// empty the evidence list and the source list with it, and look like a question
// with nothing behind it.
type subjectFixture struct {
	questionID   uuid.UUID
	personID     uuid.UUID
	placeID      uuid.UUID
	personClaim  uuid.UUID
	placeClaim   uuid.UUID
	otherPerson  uuid.UUID
	otherClaim   uuid.UUID
	statementIDs []uuid.UUID
	sourceID     uuid.UUID
	// subjectlessQuestionID has ONE linked claim, the place-subject one, and no
	// question_entities row. That is the state resolveWorkspaceEntity cannot
	// resolve anything from, and it is the case the fix is about.
	subjectlessQuestionID uuid.UUID
	subjectlessClaimID    uuid.UUID
	ownerID               string
}

func seedSubjectFilterFixture(t *testing.T, fixture *testsupport.Fixture) subjectFixture {
	t.Helper()
	unique := fixture.Unique("subj")
	owner := actor.Register(t, fixture, "باحث التصفية")

	questionID := uuid.New()
	if err := fixture.QueryRow(`INSERT INTO open_questions (id, title_ar, status, priority, created_by) VALUES ($1, $2, 'open', 'high', $3) RETURNING id`,
		questionID, "سؤال التصفية "+unique, owner.User.ID).Scan(&questionID); err != nil {
		t.Fatalf("insert open question: %v", err)
	}
	personID, placeID := uuid.New(), uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, $2, $3, 'reviewed', $4)`,
		personID, "عبد الله "+unique, "عبد الله", owner.User.ID)
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, $2, $3, 'reviewed', $4)`,
		uuid.New(), "سعد "+unique, "سعد", owner.User.ID)
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, created_by) VALUES ($1, $2, $3, 'city', $4)`,
		placeID, "الأحساء "+unique, "الأحساء", owner.User.ID)

	// The fixture's own public, independent source, so the test does not depend on
	// what the demo seed happens to contain.
	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, dependency_status, created_by) VALUES ($1, $2, 'book', 'public', 'independent', $3)`,
		sourceID, "مصدر التصفية "+unique, owner.User.ID)
	statements := make([]uuid.UUID, 0, 2)
	for index := 0; index < 2; index++ {
		statementID := uuid.New()
		fixture.Exec(`INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, 'accepted', $4)`,
			statementID, sourceID, "سكن عبد الله في الأحساء "+unique, owner.User.ID)
		statements = append(statements, statementID)
	}

	personClaim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'lived_in', 'place', $3, 'supported', $4)`,
		personClaim, personID, placeID, owner.User.ID)
	placeClaim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'place', $2, 'mentioned_in', 'person', $3, 'supported', $4)`,
		placeClaim, placeID, personID, owner.User.ID)
	// A third claim about somebody else, so the "narrows to that subject" direction
	// has something to exclude.
	otherPerson := uuid.New()
	if err := fixture.QueryRow(`SELECT id FROM people WHERE canonical_name_ar = $1`, "سعد "+unique).Scan(&otherPerson); err != nil {
		t.Fatalf("read the second person: %v", err)
	}
	otherClaim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'lived_in', 'place', $3, 'supported', $4)`,
		otherClaim, otherPerson, placeID, owner.User.ID)

	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($4, $2, 'supports', $3), ($5, $2, 'supports', $3)`,
		personClaim, statements[0], owner.User.ID, placeClaim, otherClaim)
	fixture.Exec(`INSERT INTO question_claims (question_id, claim_id, role) VALUES ($1, $2, 'concerns'), ($1, $3, 'supports'), ($1, $4, 'supports')`,
		questionID, personClaim, placeClaim, otherClaim)

	// The subjectless question: only the place-subject claim, which is the one
	// entity kind resolveWorkspaceEntity does not resolve, and no direct source
	// link, so the only route to a source runs through claim_evidence.
	subjectlessID := uuid.New()
	if err := fixture.QueryRow(`INSERT INTO open_questions (id, title_ar, status, priority, created_by) VALUES ($1, $2, 'open', 'high', $3) RETURNING id`,
		subjectlessID, "سؤال بلا موضوع "+unique, owner.User.ID).Scan(&subjectlessID); err != nil {
		t.Fatalf("insert the subjectless open question: %v", err)
	}
	onlyPlace := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'place', $2, 'mentioned_in', 'person', $3, 'supported', $4)`,
		onlyPlace, placeID, personID, owner.User.ID)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3)`, onlyPlace, statements[0], owner.User.ID)
	fixture.Exec(`INSERT INTO question_claims (question_id, claim_id, role) VALUES ($1, $2, 'concerns')`, subjectlessID, onlyPlace)

	return subjectFixture{questionID: questionID, personID: personID, placeID: placeID,
		personClaim: personClaim, placeClaim: placeClaim, otherPerson: otherPerson, otherClaim: otherClaim,
		statementIDs: statements, sourceID: sourceID, subjectlessQuestionID: subjectlessID, subjectlessClaimID: onlyPlace, ownerID: owner.User.ID}
}

func claimIDs(claims []WorkspaceClaim) []string {
	ids := make([]string, 0, len(claims))
	for _, claim := range claims {
		ids = append(ids, claim.ID)
	}
	return ids
}

func evidenceCount(claims []WorkspaceClaim) int {
	total := 0
	for _, claim := range claims {
		total += len(claim.Evidence)
	}
	return total
}

func containsID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestTheWorkspaceWithNoResolvedSubjectReturnsTheQuestionsClaims is the first
// state, and the regression this whole file is about.
//
// A question whose only linked claims are about a place has no person, family or
// branch subject to resolve, so the workspace runs with no subject at all. That is
// NO CONSTRAINT: the question's claims come back, each with the evidence behind it
// and the source it came from.
//
// Before the fix, loadWorkspaceEvidence and loadWorkspaceSources restricted on
// `c.subject_type = $n` with $n the empty string, which matches no claim, and both
// lists came back empty. The workspace then rendered claims whose `evidence` array
// was empty - indistinguishable from a claim that genuinely has none, and in
// direct contradiction of Phase 19's own criterion that every finding is traceable
// to evidence.
func TestTheWorkspaceWithNoResolvedSubjectReturnsTheQuestionsClaims(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedSubjectFilterFixture(t, fixture)
	service := NewService(fixture.Pool(), nil)
	ctx := fixture.Ctx()

	// The fixture must actually be in the state under test: a question with three
	// linked claims and no person/family/branch subject among them. Only the
	// place-subject claim resolves to nothing, so the question as a whole has no
	// resolvable subject.
	resolvedType, resolvedID, _, _, err := resolveWorkspaceEntity(ctx, service.Pool, visibility.Anonymous(), seeded.subjectlessQuestionID, WorkspaceInput{QuestionID: seeded.subjectlessQuestionID.String()})
	if err != nil {
		t.Fatalf("resolve the subject: %v", err)
	}
	if resolvedType != "" || resolvedID != "" {
		t.Skipf("this fixture resolved a subject (%s/%s), so it is not the no-subject case; a new case may be needed", resolvedType, resolvedID)
	}

	snapshot, err := service.Workspace(ctx, WorkspaceInput{QuestionID: seeded.subjectlessQuestionID.String()}, seeded.ownerID)
	if err != nil {
		t.Fatalf("workspace with no resolved subject: %v", err)
	}
	ids := claimIDs(snapshot.Claims)
	if len(ids) != 1 {
		t.Fatalf("the subjectless question returned %d claims, want its 1: %v", len(ids), ids)
	}
	if !containsID(ids, seeded.subjectlessClaimID.String()) {
		t.Fatalf("the question's own claim is missing from a workspace with no resolved subject: %v. No subject means no constraint, not no material", ids)
	}
	if evidenceCount(snapshot.Claims) == 0 {
		t.Fatalf("the workspace returned claims with no evidence at all: %+v. A broken restriction and a claim with no evidence must not look alike", snapshot.Claims)
	}
	if len(snapshot.Sources) == 0 {
		t.Fatalf("the workspace returned no sources for a question whose claims are backed by accepted statements: a restriction that excludes every claim empties this list too")
	}
}

// TestAWorkspaceSubjectFilterWithATypeAndNoIdIsRefused is the third state, and
// the one that has to be an error rather than a number.
//
// `?entity_type=person` with no `entity_id` names a filter that nothing can
// satisfy. validateWorkspaceInput already refused it at the HTTP boundary, and it
// still does; this test also pins the loaders, because their correctness used to
// rest on that boundary rather than on themselves.
//
// Returning the question's whole material here would be the wrong fix and the
// dangerous one: it would make `?entity_type=person` a way to read every claim on
// a question without naming a subject. A filter that cannot be satisfied has to
// say so.
func TestAWorkspaceSubjectFilterWithATypeAndNoIdIsRefused(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedSubjectFilterFixture(t, fixture)
	service := NewService(fixture.Pool(), nil)
	ctx := fixture.Ctx()

	// The request boundary.
	if _, err := service.Workspace(ctx, WorkspaceInput{QuestionID: seeded.questionID.String(), EntityType: "person"}, seeded.ownerID); !errors.Is(err, ErrValidation) {
		t.Fatalf("a subject type with no id = %v, want a refusal. It cannot be satisfied, and answering it with the question's whole material would let a caller read it without naming a subject", err)
	}
	// And the loaders themselves, so the refusal does not depend on a caller two
	// functions away getting the argument right.
	if _, err := loadWorkspaceClaims(ctx, service.Pool, seeded.questionID, "person", ""); !errors.Is(err, ErrValidation) {
		t.Fatalf("loadWorkspaceClaims with a type and no id = %v, want ErrValidation rather than an empty list", err)
	}
	if _, err := loadWorkspaceEvidence(ctx, service.Pool, seeded.questionID, "person", "", uuid.Nil, true); !errors.Is(err, ErrValidation) {
		t.Fatalf("loadWorkspaceEvidence with a type and no id = %v, want ErrValidation rather than an empty map", err)
	}
	if _, err := loadWorkspaceSources(ctx, service.Pool, seeded.questionID, "person", "", uuid.Nil, true); !errors.Is(err, ErrValidation) {
		t.Fatalf("loadWorkspaceSources with a type and no id = %v, want ErrValidation rather than an empty list", err)
	}
	// An id with no type is the same mistake mirrored, and it used to be silently
	// IGNORED - the empty-string test on the type let everything through, so a
	// caller who named an id and forgot the type got the whole question and no
	// filter. It is refused now.
	if _, err := loadWorkspaceClaims(ctx, service.Pool, seeded.questionID, "", seeded.personID.String()); !errors.Is(err, ErrValidation) {
		t.Fatalf("loadWorkspaceClaims with an id and no type = %v, want ErrValidation. This pair used to be silently ignored, which is a different answer again", err)
	}
}

// TestTheTwoSubjectStatesAreDistinguishable is the second state, and the
// difference the other two tests leave to be inferred: naming a subject narrows,
// and not naming one does not.
//
// Both directions are asserted, and the counts differ, so a fix that made the
// filter a no-op would fail the narrowing half and a fix that made it permanent
// would fail the other.
func TestTheTwoSubjectStatesAreDistinguishable(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedSubjectFilterFixture(t, fixture)
	service := NewService(fixture.Pool(), nil)
	ctx := fixture.Ctx()

	unfiltered, err := loadWorkspaceClaims(ctx, service.Pool, seeded.questionID, "", "")
	if err != nil {
		t.Fatalf("claims with no subject: %v", err)
	}
	if len(unfiltered) != 3 {
		t.Fatalf("with no subject the question returned %d claims, want all 3", len(unfiltered))
	}

	filtered, err := loadWorkspaceClaims(ctx, service.Pool, seeded.questionID, "person", seeded.personID.String())
	if err != nil {
		t.Fatalf("claims filtered to the person: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("naming the person returned %d claims, want the 2 that concern them: %v", len(filtered), claimIDs(filtered))
	}
	for _, claim := range filtered {
		if claim.ID == seeded.otherClaim.String() {
			t.Fatalf("naming a person returned a claim about somebody else: %v", claimIDs(filtered))
		}
	}
	if !containsID(claimIDs(filtered), seeded.personClaim.String()) {
		t.Fatalf("naming the person dropped the claim that is about them: %v", claimIDs(filtered))
	}
	// A subject's name is also an object, so the claim where they are the object
	// has to survive. A restriction that only read subject_type would lose it, and
	// a test that only checked the count would not notice.
	if !containsID(claimIDs(filtered), seeded.placeClaim.String()) {
		t.Fatalf("naming the person dropped the claim they are the OBJECT of: %v. The restriction has to read both sides of the claim", claimIDs(filtered))
	}
	if len(unfiltered) == len(filtered) {
		t.Fatalf("naming a subject changed nothing (%d claims either way), so the filter is not narrowing", len(unfiltered))
	}

	// And through the public surface, where a request that names no subject and one
	// that names a subject are different requests with different answers.
	byID, err := service.Workspace(ctx, WorkspaceInput{QuestionID: seeded.questionID.String(), EntityType: "person", EntityID: seeded.personID.String()}, seeded.ownerID)
	if err != nil {
		t.Fatalf("workspace for the named person: %v", err)
	}
	if len(byID.Claims) != 2 || !containsID(claimIDs(byID.Claims), seeded.personClaim.String()) {
		t.Fatalf("the workspace for a named person returned %v, want the 2 claims that concern them", claimIDs(byID.Claims))
	}
	// And the same distinction through the public surface, naming a different
	// person: two different subjects on one question have to be two different
	// answers, or the restriction is not reading the subject at all.
	byOther, err := service.Workspace(ctx, WorkspaceInput{QuestionID: seeded.questionID.String(), EntityType: "person", EntityID: seeded.otherPerson.String()}, seeded.ownerID)
	if err != nil {
		t.Fatalf("workspace for the other person: %v", err)
	}
	if !containsID(claimIDs(byOther.Claims), seeded.otherClaim.String()) {
		t.Fatalf("the workspace for the other person did not return that person's claim: %v", claimIDs(byOther.Claims))
	}
	if containsID(claimIDs(byOther.Claims), seeded.personClaim.String()) {
		t.Fatalf("the workspace for the other person returned the first person's claim: %v", claimIDs(byOther.Claims))
	}
	if sameOrder := fmt.Sprint(claimIDs(byID.Claims)) == fmt.Sprint(claimIDs(byOther.Claims)); sameOrder {
		t.Fatalf("naming two different people returned the same claims: %v", claimIDs(byID.Claims))
	}
}

// TestWorkspaceEntityArgsIsTheOneArgumentBuilder pins the three states at the one
// place they are decided, so the loaders cannot each grow their own idea of what
// "no subject" means. It is a table because the states are the contract.
func TestWorkspaceEntityArgsIsTheOneArgumentBuilder(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		entityType string
		entityID   string
		wantErr    bool
		wantNil    bool
	}{
		{name: "no subject named and none resolved", entityType: "", entityID: "", wantNil: true},
		{name: "a subject named", entityType: "person", entityID: "10000000-0000-0000-0000-000000000001", wantNil: false},
		{name: "a family named", entityType: "family", entityID: "10000000-0000-0000-0000-000000000001", wantNil: false},
		{name: "a type with no id cannot be satisfied", entityType: "person", entityID: "", wantErr: true},
		{name: "an id with no type is the same mistake mirrored", entityType: "", entityID: "10000000-0000-0000-0000-000000000001", wantErr: true},
	} {
		typeArg, idArg, err := workspaceEntityArgs(testCase.entityType, testCase.entityID)
		if testCase.wantErr {
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("%s: err = %v, want ErrValidation. A restriction that cannot be satisfied must be refused, not answered with nothing", testCase.name, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error %v", testCase.name, err)
		}
		if testCase.wantNil {
			// Both arguments NULL, so the restriction's "is this absent" test is
			// what answers, and the answer is "no restriction".
			if typeArg != nil || idArg != nil {
				t.Fatalf("%s: arguments = (%v, %v), want both absent so the restriction reads as no constraint", testCase.name, typeArg, idArg)
			}
			continue
		}
		if typeArg != testCase.entityType || idArg != testCase.entityID {
			t.Fatalf("%s: arguments = (%v, %v), want the named pair", testCase.name, typeArg, idArg)
		}
	}
}
