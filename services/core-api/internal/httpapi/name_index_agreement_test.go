package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/ratelimit"
	"github.com/google/uuid"
)

// The cross-index half of the ة/ه fix.
//
// identity.NormalizeArabicName maps ة to ه. Every one of the routes below receives a
// term that has already been through it, and every one of them is a place a reader
// can look a person up. A route that compared that term against a raw canonical name
// therefore answered for a name in a way its neighbours did not: مباركة was found in
// the people index, the families, the tribes, the branches, the places, the name
// search - and was not found in the disputed-claims index, because that branch read
// people.canonical_name_ar.
//
// Nothing about that disagreement is visible in any single route's own test, which is
// why it shipped. Each route looked correct against a corpus whose names avoided ة,
// and the corpus in the demo seed is Arabic family names, which is exactly the shape
// that hides it. This test drives the real routes over one corpus and requires them to
// return the same verdict for the same term, so the disagreement cannot come back
// without one route's own test going red too.
//
// Two spellings, both directions: the term as the record spells it and the same term
// after normalization. A predicate that reads a raw column fails the second and a
// predicate that re-normalizes the stored column at query time fails the first, which
// is why both are here and why the verdict is required to be the same for each.

const (
	agreementPersonRaw  = "مباركة بنت سعيد بن ميمونة"
	agreementPersonTerm = "مباركة"
	agreementPersonAsIs = "مباركه"
	agreementFamilyTerm = "عشيرة مباركة"
	agreementFamilyAsIs = "عشيره مباركه"
	agreementPlaceTerm  = "قرية أم حليمة"
	agreementPlaceAsIs  = "قريه ام حليمه"
	agreementTribeTerm  = "قبيلة أمامة"
	agreementTribeAsIs  = "قبيله امامه"
	agreementBranchTerm = "فرع مكة"
	agreementBranchAsIs = "فرع مكه"
	agreementAbsentTerm = "قرية_اختبار_غائبة"
	agreementAbsentAsIs = "قريه اختبار غائبه"
)

// peopleFacingIndex is one route a reader can name a person through, and the row it
// must answer with. Adding a route here is how a new index joins the contract.
type peopleFacingIndex struct {
	label string
	// path is the route, with %s standing for the term.
	path string
	// id is the row the route must carry for a term the corpus holds.
	id uuid.UUID
	// term carries ة, termAsIs carries ه.
	term     string
	termAsIs string
	// group is the search response group to read, empty for a dictionary index.
	group string
	// exact says the route matches a name with an ILIKE and no more, which is what
	// lets a term the corpus does not carry be required to find nothing. The search
	// names stage does not: it ranks by trigram similarity, so an unrelated term is a
	// legitimate low-score answer there and demanding silence from it would be
	// demanding a particular answer from pg_trgm's cost model.
	exact bool
}

func agreementIndexes(rows *agreementFixture) []peopleFacingIndex {
	return []peopleFacingIndex{
		{label: "dictionary people", path: "/api/v1/dictionary?kind=people&q=%s", id: rows.personID, term: agreementPersonTerm, termAsIs: agreementPersonAsIs, exact: true},
		{label: "dictionary families", path: "/api/v1/dictionary?kind=families&q=%s", id: rows.familyID, term: agreementFamilyTerm, termAsIs: agreementFamilyAsIs, exact: true},
		{label: "dictionary tribes", path: "/api/v1/dictionary?kind=tribes&q=%s", id: rows.tribeID, term: agreementTribeTerm, termAsIs: agreementTribeAsIs, exact: true},
		{label: "dictionary branches", path: "/api/v1/dictionary?kind=branches&q=%s", id: rows.branchID, term: agreementBranchTerm, termAsIs: agreementBranchAsIs, exact: true},
		{label: "dictionary places", path: "/api/v1/dictionary?kind=places&q=%s", id: rows.placeID, term: agreementPlaceTerm, termAsIs: agreementPlaceAsIs, exact: true},
		{label: "dictionary disputed-claims", path: "/api/v1/dictionary?kind=disputed-claims&q=%s", id: rows.claimID, term: agreementPersonTerm, termAsIs: agreementPersonAsIs, exact: true},
		{label: "search names (person)", path: "/api/v1/search?kind=names&q=%s", group: "names", id: rows.personID, term: agreementPersonTerm, termAsIs: agreementPersonAsIs},
		{label: "search names (place historical name)", path: "/api/v1/search?kind=names&q=%s", group: "names", id: rows.placeID, term: agreementPlaceTerm, termAsIs: agreementPlaceAsIs},
		{label: "search claims (subject name)", path: "/api/v1/search?kind=claims&q=%s", group: "claims", id: rows.claimID, term: agreementPersonTerm, termAsIs: agreementPersonAsIs, exact: true},
	}
}

// TestPeopleFacingIndexesResolveOneTermIdentically is the assertion the defect needed
// and did not have: for one term, every people-facing index returns the same verdict,
// and the verdict is reported per index so a disagreement names the two routes that
// differ rather than a bare failure.
func TestPeopleFacingIndexesResolveOneTermIdentically(t *testing.T) {
	fixture := testsupport.New(t)
	rows := seedAgreementFixture(t, fixture)
	router := NewRouter(Dependencies{DB: fixture.Pool(), RateLimits: ratelimit.Disabled()})
	indexes := agreementIndexes(rows)

	for _, spelling := range []struct {
		label string
		term  func(peopleFacingIndex) string
	}{
		{"the canonical spelling", func(index peopleFacingIndex) string { return index.term }},
		{"the normalized spelling", func(index peopleFacingIndex) string { return index.termAsIs }},
	} {
		verdicts := make([]string, 0, len(indexes))
		agreed := true
		for _, index := range indexes {
			term := spelling.term(index)
			found, err := indexCarries(router, index, term)
			if err != nil {
				t.Fatalf("%s: %s: %v", index.label, term, err)
			}
			verdicts = append(verdicts, fmt.Sprintf("%s=%t", index.label, found))
			if !found {
				agreed = false
			}
		}
		if !agreed {
			// Every index in this corpus holds the same name, so a single "not
			// found" is a disagreement by construction, and the list says which.
			t.Errorf("with %s the people-facing indexes did not agree: %v", spelling.label, verdicts)
		}
	}

	// The negative half. A term no row carries must find no row in any index that
	// matches a name exactly, which is the same agreement stated from the other side:
	// an index that matches too much is as much a disagreement as one that matches too
	// little. Both spellings are asked, and they are the same query by the time it
	// reaches a route - which is the property the two halves of this test share.
	for _, term := range []string{agreementAbsentTerm, agreementAbsentAsIs} {
		answered := make([]string, 0, len(indexes))
		for _, index := range indexes {
			if !index.exact {
				continue
			}
			found, err := indexCarries(router, index, term)
			if err != nil {
				t.Fatalf("%s: %s: %v", index.label, term, err)
			}
			if found {
				answered = append(answered, index.label)
			}
		}
		if len(answered) != 0 {
			t.Errorf("the absent term %q was answered by %v", term, answered)
		}
	}
}

// indexCarries asks one route whether its answer carries the row the index is
// supposed to answer with.
func indexCarries(router http.Handler, index peopleFacingIndex, term string) (bool, error) {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf(index.path, url.QueryEscape(term)), nil))
	if recorder.Code != http.StatusOK {
		return false, fmt.Errorf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	if index.group == "" {
		var response struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			return false, err
		}
		for _, item := range response.Items {
			if item.ID == index.id.String() {
				return true, nil
			}
		}
		return false, nil
	}
	var response struct {
		Groups []struct {
			Key   string `json:"key"`
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		return false, err
	}
	for _, group := range response.Groups {
		if group.Key != index.group {
			continue
		}
		for _, item := range group.Items {
			if item.ID == index.id.String() {
				return true, nil
			}
		}
	}
	return false, nil
}

type agreementFixture struct {
	personID uuid.UUID
	familyID uuid.UUID
	tribeID  uuid.UUID
	branchID uuid.UUID
	placeID  uuid.UUID
	claimID  uuid.UUID
	ownerID  uuid.UUID
}

const (
	agreementFamilyRaw = "عشيرة مباركة"
	agreementTribeRaw  = "قبيلة أمامة"
	agreementBranchRaw = "فرع مكة"
	agreementPlaceRaw  = "وادي مصنعة"
	agreementPlaceHis  = "قرية أم حليمة القديمة"
)

func seedAgreementFixture(t *testing.T, fixture *testsupport.Fixture) *agreementFixture {
	t.Helper()
	rows := &agreementFixture{
		personID: uuid.New(),
		familyID: uuid.New(),
		tribeID:  uuid.New(),
		branchID: uuid.New(),
		placeID:  uuid.New(),
		claimID:  uuid.New(),
		ownerID:  uuid.New(),
	}
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'صاحب الاتفاق')`, rows.ownerID, fixture.Email())
	// The normalized columns are written through the production normalizer, so the
	// corpus holds the relation every predicate in this test depends on.
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, $2, $3, $4)`,
		rows.personID, agreementPersonRaw, identity.NormalizeArabicName(agreementPersonRaw), rows.ownerID)
	fixture.Exec(`INSERT INTO families (id, canonical_name_ar, normalized_name_ar, visibility) VALUES ($1, $2, $3, 'public')`,
		rows.familyID, agreementFamilyRaw, identity.NormalizeArabicName(agreementFamilyRaw))
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility) VALUES ($1, $2, $3, 'public')`,
		rows.tribeID, agreementTribeRaw, identity.NormalizeArabicName(agreementTribeRaw))
	fixture.Exec(`INSERT INTO branches (id, family_id, canonical_name_ar, normalized_name_ar, visibility) VALUES ($1, $2, $3, $4, 'public')`,
		rows.branchID, rows.familyID, agreementBranchRaw, identity.NormalizeArabicName(agreementBranchRaw))
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility) VALUES ($1, $2, $3, 'city', 'public')`,
		rows.placeID, agreementPlaceRaw, identity.NormalizeArabicName(agreementPlaceRaw))
	fixture.Exec(`INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar) VALUES ($1, $2, $3)`,
		rows.placeID, agreementPlaceHis, identity.NormalizeArabicName(agreementPlaceHis))

	// A published public tree version is what makes the person readable by an
	// anonymous caller, so every route is exercised as an anonymous reader sees it.
	treeID := uuid.New()
	versionID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, $2, 'public', $3)`, treeID, fixture.Unique("شجرة"), rows.ownerID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, versionID, treeID, rows.ownerID)
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, 0)`,
		uuid.New(), versionID, rows.personID, agreementPersonRaw)

	// The claim is disputed and evidenced on a public source, so both claim routes
	// answer for it anonymously.
	sourceID := uuid.New()
	passageID := uuid.New()
	statementID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, $2, 'book', 'public', $3)`,
		sourceID, fixture.Unique("مصدر"), rows.ownerID)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, 1, $3, $4)`,
		passageID, sourceID, fixture.Unique("مقطع"), fixture.Unique("مقطع"))
	fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, $3, $4, 'accepted', $5)`,
		statementID, sourceID, passageID, fixture.Unique("عبارة"), rows.ownerID)
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, 'mother_of', 'person', $2, 'disputed', $3)`,
		rows.claimID, rows.personID, rows.ownerID)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3)`,
		rows.claimID, statementID, rows.ownerID)
	return rows
}
