package dictionary

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// NormalizeArabicName maps ة to ه, so a name carrying ة is a different string from
// the same name carrying ه. ListIndex hands every branch the term after that
// normalization, and the only column a term can then be compared against correctly
// is the one that holds the same normalization. The disputed-claims branch compared
// it against the raw canonical name instead, so a person called مباركة answered in
// the people index and not in that one.
//
// Both directions are tested, because a fix that normalized the term and then still
// matched the raw column passes one and fails the other. The behaviour the test
// pins is a decision rather than a bug: a row whose canonical spelling differs from
// the term only by ة/ه is found by BOTH spellings of the term, because the column the
// match reads is the normalized one and the term is normalized on the way in. The
// name of the test says so, because "one spelling finds the row and the other does
// not" is exactly what a reader has to be told is not what happens.

const (
	// personRaw is the person's own spelling, and personTermAsIs is the normalized
	// spelling of the same word: مباركة normalizes to مباركه.
	personRaw      = "مباركة بنت سعيد بن ميمونة"
	personTerm     = "مباركة"
	personTermAsIs = "مباركه"
	familyRaw      = "عشيرة مباركة"
	familyTerm     = "عشيرة مباركة"
	familyTermAsIs = "عشيره مباركه"
	tribeRaw       = "قبيلة أمامة"
	tribeTerm      = "قبيلة أمامة"
	tribeTermAsIs  = "قبيله امامه"
	branchRaw      = "فرع مكة"
	branchTerm     = "فرع مكة"
	branchTermAsIs = "فرع مكه"
	// The place index reads two name columns: the place's own and the historical name
	// it carries. The historical one is the column the branch changed.
	placeRaw         = "وادي مصنعة"
	historicalRaw    = "قرية أم حليمة القديمة"
	historicalTerm   = "قرية أم حليمة"
	historicalAsIs   = "قريه ام حليمه"
	absentTerm       = "قرية_اختبار_غائبة"
	absentTermAsIs   = "قريه اختبار غائبه"
	claimSubjectTerm = "مباركة"
)

// nameIndex is one people-facing index and the row it must answer with, named by the
// two spellings a caller can type for it.
type nameIndex struct {
	kind string
	// id is the row the index must return for both terms.
	id uuid.UUID
	// term carries ة, termAsIs carries ه. Both must find the same row.
	term     string
	termAsIs string
}

// TestEveryNameIndexAnswersForBothSpellingsOfATaMarbutaName walks every index a
// reader can name a person, a reference or a person's claim through, and requires
// each to answer for both spellings of a name its own row carries. The cross-index
// half of the fix - that every index resolves one term the same way - is
// TestPeopleFacingIndexesResolveOneTermIdentically in internal/httpapi, which
// drives the same corpus through the routes.
func TestEveryNameIndexAnswersForBothSpellingsOfATaMarbutaName(t *testing.T) {
	fixture := testsupport.New(t)
	rows := seedTaMarbutaFixture(t, fixture)
	service := NewService(fixture.Pool())
	ctx := fixture.Ctx()

	for _, index := range rows.indexes() {
		for _, spelling := range []struct {
			label string
			term  string
		}{{"the canonical spelling", index.term}, {"the normalized spelling", index.termAsIs}} {
			found, err := service.ListIndex(ctx, index.kind, spelling.term, "")
			if err != nil {
				t.Fatalf("%s: read with %s %q: %v", index.kind, spelling.label, spelling.term, err)
			}
			if !indexIDs(found)[index.id] {
				// Reported and not fatal, so a reader sees every index that
				// disagreed rather than the first one: the disagreement between
				// indexes is the whole defect, and stopping at one of them hides
				// the rest.
				t.Errorf("%s: %s %q did not find the row: %+v", index.kind, spelling.label, spelling.term, found.Items)
			}
		}
		// A term the corpus does not carry answers in no index, in either spelling.
		for _, term := range []string{absentTerm, absentTermAsIs} {
			found, err := service.ListIndex(ctx, index.kind, term, "")
			if err != nil {
				t.Fatalf("%s: read with the absent term %q: %v", index.kind, term, err)
			}
			if len(found.Items) != 0 {
				t.Errorf("%s: the absent term %q answered with %+v", index.kind, term, found.Items)
			}
		}
	}
}

// TestDisputedClaimsIndexKeepsItsStatusFilter pins the half of the change that is
// not about normalization: a claim carrying the same name that is not disputed does
// not answer, so a match cannot be explained by the status filter being lost.
func TestDisputedClaimsIndexKeepsItsStatusFilter(t *testing.T) {
	fixture := testsupport.New(t)
	rows := seedTaMarbutaFixture(t, fixture)
	service := NewService(fixture.Pool())

	found, err := service.ListIndex(fixture.Ctx(), "disputed-claims", claimSubjectTerm, "")
	if err != nil {
		t.Fatalf("disputed-claims read: %v", err)
	}
	ids := indexIDs(found)
	if !ids[rows.disputedID] {
		t.Fatalf("the disputed claim is missing: %+v", found.Items)
	}
	if ids[rows.supportedID] {
		t.Fatalf("a supported claim answered the disputed-claims index: %+v", found.Items)
	}
}

type taMarbutaFixture struct {
	ownerID     uuid.UUID
	personID    uuid.UUID
	familyID    uuid.UUID
	tribeID     uuid.UUID
	branchID    uuid.UUID
	placeID     uuid.UUID
	disputedID  uuid.UUID
	supportedID uuid.UUID
}

// indexes is every dictionary index a reader can name a person, a reference or a
// person's claim through, with the row each one must answer with. Adding an index
// here is how a later index joins the same contract.
func (f *taMarbutaFixture) indexes() []nameIndex {
	return []nameIndex{
		{kind: "people", id: f.personID, term: personTerm, termAsIs: personTermAsIs},
		{kind: "families", id: f.familyID, term: familyTerm, termAsIs: familyTermAsIs},
		{kind: "tribes", id: f.tribeID, term: tribeTerm, termAsIs: tribeTermAsIs},
		{kind: "branches", id: f.branchID, term: branchTerm, termAsIs: branchTermAsIs},
		{kind: "places", id: f.placeID, term: historicalTerm, termAsIs: historicalAsIs},
		{kind: "disputed-claims", id: f.disputedID, term: claimSubjectTerm, termAsIs: personTermAsIs},
	}
}

func seedTaMarbutaFixture(t *testing.T, fixture *testsupport.Fixture) *taMarbutaFixture {
	t.Helper()
	rows := &taMarbutaFixture{
		ownerID:     uuid.New(),
		personID:    uuid.New(),
		familyID:    uuid.New(),
		tribeID:     uuid.New(),
		branchID:    uuid.New(),
		placeID:     uuid.New(),
		disputedID:  uuid.New(),
		supportedID: uuid.New(),
	}
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'صاحب القاموس')`, rows.ownerID, fixture.Email())

	// Every normalized column is written through the production normalizer rather than
	// copied from the canonical name, so the fixture holds the relation the predicate
	// depends on instead of asserting it.
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, $2, $3, $4)`,
		rows.personID, personRaw, identity.NormalizeArabicName(personRaw), rows.ownerID)
	fixture.Exec(`INSERT INTO families (id, canonical_name_ar, normalized_name_ar, visibility) VALUES ($1, $2, $3, 'public')`,
		rows.familyID, familyRaw, identity.NormalizeArabicName(familyRaw))
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility) VALUES ($1, $2, $3, 'public')`,
		rows.tribeID, tribeRaw, identity.NormalizeArabicName(tribeRaw))
	fixture.Exec(`INSERT INTO branches (id, family_id, canonical_name_ar, normalized_name_ar, visibility) VALUES ($1, $2, $3, $4, 'public')`,
		rows.branchID, rows.familyID, branchRaw, identity.NormalizeArabicName(branchRaw))
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility) VALUES ($1, $2, $3, 'city', 'public')`,
		rows.placeID, placeRaw, identity.NormalizeArabicName(placeRaw))
	// The historical name is the place index's second name column, and the branch now
	// reads its normalized column rather than its raw one.
	fixture.Exec(`INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar) VALUES ($1, $2, $3)`,
		rows.placeID, historicalRaw, identity.NormalizeArabicName(historicalRaw))

	// A published public tree is what makes the person readable by an anonymous
	// caller, so every case runs as an anonymous reader sees it.
	treeID := uuid.New()
	versionID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, $2, 'public', $3)`, treeID, fixture.Unique("شجرة"), rows.ownerID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, versionID, treeID, rows.ownerID)
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, 0)`,
		uuid.New(), versionID, rows.personID, personRaw)

	// A disputed claim reaches an anonymous reader only through evidence on a public
	// source, so the claim stage is exercised with the evidence a real reviewer has.
	sourceID := uuid.New()
	passageID := uuid.New()
	statementID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, $2, 'book', 'public', $3)`,
		sourceID, fixture.Unique("مصدر"), rows.ownerID)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, $3, $4)`,
		passageID, sourceID, fixture.Unique("مقطع"), fixture.Unique("مقطع"))
	fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, $4, 'accepted', $5)`,
		statementID, sourceID, passageID, fixture.Unique("عبارة"), rows.ownerID)
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'mother_of', 'person', $3, 'disputed', $4)`,
		rows.disputedID, rows.personID, rows.personID, rows.ownerID)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3)`,
		rows.disputedID, statementID, rows.ownerID)
	// A supported claim carries the same name, so a match cannot be explained by the
	// status filter being lost.
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'mother_of', 'person', $3, 'supported', $4)`,
		rows.supportedID, rows.personID, rows.personID, rows.ownerID)
	return rows
}
