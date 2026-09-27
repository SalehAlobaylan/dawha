package search

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// validateInput normalizes the caller's query once, with the same normalizer the name
// columns were written with, and every stage below is handed that normalized string.
// A stage that then compares it against a raw name column cannot answer for a name
// carrying ة: مباركة and مباركه are different strings, and the term the caller typed
// has already become the second of them. That is what the claims stage did - it was
// the only name stage in this service reading a raw column, so a person the names
// stage found by ة could not be found by name from claim search, and a claim about
// them was unreachable.
//
// Both directions are asserted. A fix that normalized the term and then still read
// the raw column would pass the canonical-spelling case and fail the
// normalized-spelling one, so a single-direction test would not have caught the
// original defect either.

const (
	// rawPersonName carries ة; personTermAsIs is what identity.NormalizeArabicName
	// makes of the first word of it.
	rawPersonName  = "مباركة بنت سعيد بن ميمونة"
	personTerm     = "مباركة"
	personTermAsIs = "مباركه"
	// The hidden person carries ة too, so the policy case cannot be passed by a term
	// that happens to avoid the normalization.
	rawHiddenName  = "سودة بنت ميمون"
	hiddenTerm     = "سودة"
	hiddenTermAsIs = "سوده"
	rawPlaceName   = "وادي مصنعة"
	rawHistorical  = "قرية أم حليمة القديمة"
	historicalTerm = "قرية أم حليمة"
	historicalAsIs = "قريه ام حليمه"
	absentTerm     = "قرية_اختبار_غائبة"
	absentTermAsIs = "قريه اختبار غائبه"
	unrelatedName2 = "زيد بن عمرو بن كعب"
)

// TestNameStagesAnswerForBothSpellingsOfATaMarbutaName covers the names stage: the
// person row, whose column was already normalized, and the place's historical name,
// whose column was not.
func TestNameStagesAnswerForBothSpellingsOfATaMarbutaName(t *testing.T) {
	fixture := testsupport.New(t)
	rows := seedTaMarbutaSearchFixture(t, fixture)
	service := NewService(fixture.Pool())

	for _, expectation := range []struct {
		label    string
		term     string
		termAsIs string
		id       uuid.UUID
	}{
		{label: "person", term: personTerm, termAsIs: personTermAsIs, id: rows.personID},
		{label: "place historical name", term: historicalTerm, termAsIs: historicalAsIs, id: rows.placeID},
	} {
		// The two spellings normalize to one string, so the stage is handed the same
		// query twice and must answer the same way twice. This stage ranks by trigram
		// similarity, so it is not the place to assert that an unrelated term answers
		// with nothing; the exact-match stages are where that is pinned.
		answered := map[string][]string{}
		for _, spelling := range []struct {
			label string
			term  string
		}{{"the canonical spelling", expectation.term}, {"the normalized spelling", expectation.termAsIs}} {
			response, err := service.Search(fixture.Ctx(), Input{Query: spelling.term, Kind: "names", ActorID: rows.actorID})
			if err != nil {
				t.Fatalf("%s: search with %s %q: %v", expectation.label, spelling.label, spelling.term, err)
			}
			answered[spelling.label] = groupTitles(response, "names")
			if !groupCarries(response, "names", expectation.id) {
				t.Errorf("%s: %s %q did not find the row: %v", expectation.label, spelling.label, spelling.term, answered[spelling.label])
			}
		}
		if !sameStrings(answered["the canonical spelling"], answered["the normalized spelling"]) {
			t.Errorf("%s: the two spellings of one name produced different result sets: %v against %v",
				expectation.label, answered["the canonical spelling"], answered["the normalized spelling"])
		}
	}
}

// TestClaimStageAnswersForBothSpellingsOfTheSubjectName is the case the plan names:
// a claim is matched by the name of the person it is about, and that name was read
// raw.
func TestClaimStageAnswersForBothSpellingsOfTheSubjectName(t *testing.T) {
	fixture := testsupport.New(t)
	rows := seedTaMarbutaSearchFixture(t, fixture)
	service := NewService(fixture.Pool())

	for _, spelling := range []struct {
		label string
		term  string
	}{{"the canonical spelling", personTerm}, {"the normalized spelling", personTermAsIs}} {
		response, err := service.Search(fixture.Ctx(), Input{Query: spelling.term, Kind: "claims", ActorID: rows.actorID})
		if err != nil {
			t.Fatalf("claim search with %s %q: %v", spelling.label, spelling.term, err)
		}
		if !groupCarries(response, "claims", rows.disputedID) {
			t.Errorf("claims: %s %q did not find the claim about the person: %v", spelling.label, spelling.term, groupTitles(response, "claims"))
		}
		// The claim about a person the corpus does not carry is not an answer, so a
		// match cannot be explained by the claim filters being lost.
		if groupCarries(response, "claims", rows.unrelatedClaimID) {
			t.Errorf("claims: %s %q answered with the unrelated claim", spelling.label, spelling.term)
		}
	}
}

// TestClaimStageKeepsItsPersonPolicy is the half of the change that is not about
// normalization: matching a claim by a name must not become a way to probe a person
// the caller cannot read, so a claim naming only a research-only person stays out of
// an anonymous answer and stays in the owner's.
func TestClaimStageKeepsItsPersonPolicy(t *testing.T) {
	fixture := testsupport.New(t)
	rows := seedTaMarbutaSearchFixture(t, fixture)
	service := NewService(fixture.Pool())

	anonymous, err := service.Search(fixture.Ctx(), Input{Query: hiddenTerm, Kind: "claims"})
	if err != nil {
		t.Fatalf("anonymous claim search: %v", err)
	}
	if groupCarries(anonymous, "claims", rows.hiddenClaimID) {
		t.Fatalf("a claim naming a research-only person answered an anonymous name search: %v", groupTitles(anonymous, "claims"))
	}
	owner, err := service.Search(fixture.Ctx(), Input{Query: hiddenTermAsIs, Kind: "claims", ActorID: rows.actorID})
	if err != nil {
		t.Fatalf("owner claim search: %v", err)
	}
	if !groupCarries(owner, "claims", rows.hiddenClaimID) {
		t.Fatalf("the owner lost the claim naming their own research-only person: %v", groupTitles(owner, "claims"))
	}
}

type taMarbutaSearchFixture struct {
	actorID           string
	personID          uuid.UUID
	hiddenPersonID    uuid.UUID
	unrelatedPersonID uuid.UUID
	placeID           uuid.UUID
	disputedID        uuid.UUID
	hiddenClaimID     uuid.UUID
	unrelatedClaimID  uuid.UUID
}

func groupCarries(response Response, key string, id uuid.UUID) bool {
	for _, group := range response.Groups {
		if group.Key != key {
			continue
		}
		for _, item := range group.Items {
			if item.ID == id.String() {
				return true
			}
		}
	}
	return false
}

func groupTitles(response Response, key string) []string {
	titles := make([]string, 0)
	for _, group := range response.Groups {
		if group.Key != key {
			continue
		}
		for _, item := range group.Items {
			titles = append(titles, item.Title)
		}
	}
	return titles
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func seedTaMarbutaSearchFixture(t *testing.T, fixture *testsupport.Fixture) *taMarbutaSearchFixture {
	t.Helper()
	owner := registerSearchActor(t, fixture, "صاحب البحث")
	rows := &taMarbutaSearchFixture{
		actorID:           owner,
		personID:          uuid.New(),
		hiddenPersonID:    uuid.New(),
		unrelatedPersonID: uuid.New(),
		placeID:           uuid.New(),
		disputedID:        uuid.New(),
		hiddenClaimID:     uuid.New(),
		unrelatedClaimID:  uuid.New(),
	}
	// Every normalized column is written through the production normalizer rather than
	// copied from the canonical name, so the fixture holds the relation the predicate
	// depends on instead of asserting it.
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES
		($1, $2, $3, $7),
		($4, $5, $6, $7),
		($8, $9, $10, $7)`,
		rows.personID, rawPersonName, identity.NormalizeArabicName(rawPersonName),
		rows.hiddenPersonID, rawHiddenName, identity.NormalizeArabicName(rawHiddenName),
		owner,
		rows.unrelatedPersonID, unrelatedName, identity.NormalizeArabicName(unrelatedName))
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility) VALUES ($1, $2, $3, 'city', 'public')`,
		rows.placeID, rawPlaceName, identity.NormalizeArabicName(rawPlaceName))
	fixture.Exec(`INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar) VALUES ($1, $2, $3)`,
		rows.placeID, rawHistorical, identity.NormalizeArabicName(rawHistorical))

	// The names stage is a public read, so the person needs a published public tree
	// version or the policy answers as though the person did not exist. The hidden
	// person deliberately has no tree version, which is the whole difference between
	// the two claims below.
	treeID := uuid.New()
	versionID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, $2, 'public', $3)`, treeID, fixture.Unique("شجرة"), owner)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, versionID, treeID, owner)
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, 0)`,
		uuid.New(), versionID, rows.personID, rawPersonName)

	// A claim is publicly readable through evidence on a public source, so all three
	// claims carry the same evidence and differ only in who they name.
	sourceID := uuid.New()
	passageID := uuid.New()
	statementID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, $2, 'book', 'public', $3)`,
		sourceID, fixture.Unique("مصدر"), owner)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, $3, $4)`,
		passageID, sourceID, fixture.Unique("مقطع"), fixture.Unique("مقطع"))
	fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, $4, 'accepted', $5)`,
		statementID, sourceID, passageID, fixture.Unique("عبارة"), owner)
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES
		($1, 'person', $2, 'mother_of', 'person', $2, 'disputed', $7),
		($3, 'person', $4, 'mother_of', 'person', $4, 'disputed', $7),
		($5, 'person', $6, 'mother_of', 'person', $6, 'disputed', $7)`,
		rows.disputedID, rows.personID,
		rows.hiddenClaimID, rows.hiddenPersonID,
		rows.unrelatedClaimID, rows.unrelatedPersonID,
		owner)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES
		($1, $4, 'supports', $5), ($2, $4, 'supports', $5), ($3, $4, 'supports', $5)`,
		rows.disputedID, rows.hiddenClaimID, rows.unrelatedClaimID, statementID, owner)
	return rows
}

func registerSearchActor(t *testing.T, fixture *testsupport.Fixture, displayName string) string {
	t.Helper()
	userID := uuid.New()
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, $3)`, userID, fixture.Email(), displayName)
	fixture.Exec(`INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered')`, userID)
	return userID.String()
}
