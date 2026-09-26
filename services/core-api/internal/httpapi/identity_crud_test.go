package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// This file drives the real router, so the handler, the identity service, the
// central visibility policy, the dictionary and the search service are exercised
// together. A guard proved only inside the service would not prove that the
// endpoint the product actually serves carries it.

// identitySession is a logged-in account together with the cookie the router
// reads, so a request is made exactly the way a browser would make it.
type identitySession struct {
	actor.Actor
	userID string
}

func newIdentitySession(t *testing.T, fixture *testsupport.Fixture, displayName, role string) identitySession {
	t.Helper()
	registered := actor.RegisterAndLogin(t, fixture, displayName)
	if role != "" {
		fixture.GrantRole(registered.User.ID, role)
	}
	return identitySession{Actor: registered, userID: registered.User.ID}
}

func (s identitySession) do(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	if s.Token != "" {
		request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: s.Token})
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func anonymous(router http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func decodeIdentityBody(t *testing.T, recorder *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
}

// TestIdentityCreatesResearchOnlyRows is the test the plan is judged on. A person
// and an alias written through the new API belong to no published tree version,
// so plan 001 keeps them invisible: an anonymous direct read is a 404, the
// anonymous dictionary index does not list the person or the alias, and an
// anonymous search finds neither the name nor the alias. The creator, who is the
// one actor the policy grants, still sees both.
func TestIdentityCreatesResearchOnlyRows(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	writer := newIdentitySession(t, fixture, "باحث الهوية", "researcher")

	name := "منشور بحث الذري " + fixture.Tag()
	aliasValue := "لقب البحث الذري " + fixture.Tag()

	created := writer.do(router, http.MethodPost, "/api/v1/people",
		`{"canonical_name_ar":"`+name+`","gender":"male","birth_date_from":"0700-01-01"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create person = %d, want 201: %s", created.Code, created.Body.String())
	}
	var person struct {
		ID             string `json:"id"`
		Kind           string `json:"kind"`
		NameAR         string `json:"nameAr"`
		IdentityStatus string `json:"identityStatus"`
	}
	decodeIdentityBody(t, created, &person)
	if person.ID == "" || person.NameAR != name {
		t.Fatalf("created person = %+v, want an id and the submitted name", person)
	}
	if person.IdentityStatus != "unreviewed" {
		t.Fatalf("a created person reports identity status %q, want unreviewed", person.IdentityStatus)
	}

	aliasResponse := writer.do(router, http.MethodPost, "/api/v1/people/"+person.ID+"/aliases",
		`{"value_ar":"`+aliasValue+`","alias_type":"kunyah"}`)
	if aliasResponse.Code != http.StatusCreated {
		t.Fatalf("create alias = %d, want 201: %s", aliasResponse.Code, aliasResponse.Body.String())
	}

	// The direct-id read a browser would do.
	if recorder := anonymous(router, http.MethodGet, "/api/v1/dictionary/people/"+person.ID); recorder.Code != http.StatusNotFound {
		t.Fatalf("anonymous dictionary detail = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
	if recorder := anonymous(router, http.MethodGet, "/api/v1/dictionary/people/"+uuid.NewString()); recorder.Code != http.StatusNotFound {
		t.Fatalf("anonymous dictionary detail for a missing person = %d, want the same 404: %s", recorder.Code, recorder.Body.String())
	}

	// The anonymous dictionary index, by the person's name and by its alias. An
	// alias is a search term as well as a label, so both are checked: without the
	// alias gate the alias would work as a probe proving the person exists. The
	// endpoint echoes the query back, so the assertion is on the items it returned
	// rather than on the whole body.
	for _, query := range []string{name, aliasValue} {
		recorder := anonymous(router, http.MethodGet, "/api/v1/dictionary?kind=people&q="+url.QueryEscape(query))
		if recorder.Code != http.StatusOK {
			t.Fatalf("anonymous dictionary index = %d, want 200: %s", recorder.Code, recorder.Body.String())
		}
		var index struct {
			Query string `json:"query"`
			Items []struct {
				ID          string `json:"id"`
				NameAR      string `json:"nameAr"`
				SecondaryAR string `json:"secondaryAr"`
			} `json:"items"`
		}
		decodeIdentityBody(t, recorder, &index)
		if len(index.Items) != 0 {
			t.Fatalf("the anonymous dictionary index returned %d items for %q: %s", len(index.Items), query, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), person.ID) {
			t.Fatalf("the created person leaked into the anonymous dictionary index for %q: %s", query, recorder.Body.String())
		}
	}

	// The anonymous search index, by the name and by the alias.
	for _, query := range []string{name, aliasValue} {
		recorder := anonymous(router, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(query))
		if recorder.Code != http.StatusOK {
			t.Fatalf("anonymous search = %d, want 200: %s", recorder.Code, recorder.Body.String())
		}
		var results struct {
			Groups []struct {
				Key   string `json:"key"`
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			} `json:"groups"`
		}
		decodeIdentityBody(t, recorder, &results)
		for _, group := range results.Groups {
			for _, item := range group.Items {
				if item.ID == person.ID {
					t.Fatalf("the created person leaked into anonymous search under %q: %s", group.Key, recorder.Body.String())
				}
			}
		}
		if strings.Contains(recorder.Body.String(), person.ID) {
			t.Fatalf("anonymous search disclosed the person id for %q: %s", query, recorder.Body.String())
		}
	}

	// The creator sees both, through the same public read paths. If the creator
	// could not, the write would have produced a row nobody can read.
	if recorder := writer.do(router, http.MethodGet, "/api/v1/dictionary/people/"+person.ID, ""); recorder.Code != http.StatusOK {
		t.Fatalf("the creator's dictionary detail = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if recorder := writer.do(router, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(name), ""); !strings.Contains(recorder.Body.String(), person.ID) {
		t.Fatalf("the creator's own search does not find the person they created: %s", recorder.Body.String())
	}
	if recorder := writer.do(router, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(aliasValue), ""); !strings.Contains(recorder.Body.String(), person.ID) {
		t.Fatalf("the creator's own search does not find the alias they recorded: %s", recorder.Body.String())
	}

	// An account with no right to write is refused the endpoint outright, before
	// the row is even looked at, so the endpoint says nothing about it.
	stranger := newIdentitySession(t, fixture, "غريب", "")
	if recorder := stranger.do(router, http.MethodGet, "/api/v1/people/"+person.ID, ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("an account with no identity role read the person = %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
	// A researcher holds the role, and still reads the row as missing rather than
	// as hidden. A research role is not a blanket bypass over the global people
	// table, so the identity endpoints cannot become a way to discover a research
	// record that belongs to somebody else.
	otherResearcher := newIdentitySession(t, fixture, "باحث آخر", "researcher")
	if recorder := otherResearcher.do(router, http.MethodGet, "/api/v1/people/"+person.ID, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("an unrelated researcher read the person = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
	// And no published interpretation was created as a side effect of the create.
	if got := fixture.Count(`SELECT count(*) FROM tree_nodes WHERE person_id = $1`, person.ID); got != 0 {
		t.Fatalf("the created person appears in %d tree nodes, want 0", got)
	}
}

// TestIdentityHTTPPermissionMatrix is guardrail three at the boundary: the status
// a caller gets per entity family, from an anonymous request to a moderator. A
// tree-scoped right is in the table on purpose - it is the case plan 002 learned
// the hard way.
func TestIdentityHTTPPermissionMatrix(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})

	registered := newIdentitySession(t, fixture, "مسجل", "")
	collaborator := newIdentitySession(t, fixture, "متعامل", "collaborator")
	researcher := newIdentitySession(t, fixture, "باحث", "researcher")
	moderator := newIdentitySession(t, fixture, "مشرف", "moderator")

	// A tree the registered account owns, with the collaborator account on it at
	// edit level. Neither holds a platform role beyond registered, so a write from
	// either of them would prove a tree right had become a global one.
	treeOwnerName := "شجرة " + fixture.Unique("t")
	created := registered.do(router, http.MethodPost, "/api/v1/trees",
		`{"name_ar":"`+treeOwnerName+`","visibility":"private","people":[{"canonical_name_ar":"شخص الشجرة"}]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create tree = %d: %s", created.Code, created.Body.String())
	}
	var tree struct {
		Tree struct {
			ID string `json:"id"`
		} `json:"tree"`
	}
	decodeIdentityBody(t, created, &tree)
	fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'edit', $3)`,
		tree.Tree.ID, registered.userID, registered.userID)
	fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'edit', $3)`,
		tree.Tree.ID, collaborator.userID, registered.userID)
	treeOnly := newTreeOnlyAccount(t, fixture, tree.Tree.ID, registered.userID)

	// The prerequisites are written by a researcher, so each row below is about the
	// session under test and nothing else.
	familyResponse := researcher.do(router, http.MethodPost, "/api/v1/families", `{"canonical_name_ar":"بيتPermissions`+fixture.Tag()+`"}`)
	if familyResponse.Code != http.StatusCreated {
		t.Fatalf("create family = %d: %s", familyResponse.Code, familyResponse.Body.String())
	}
	var family struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, familyResponse, &family)
	subjectResponse := researcher.do(router, http.MethodPost, "/api/v1/people", `{"canonical_name_ar":"موضوعMatrix`+fixture.Tag()+`"}`)
	if subjectResponse.Code != http.StatusCreated {
		t.Fatalf("create person = %d: %s", subjectResponse.Code, subjectResponse.Body.String())
	}
	var subject struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, subjectResponse, &subject)
	objectResponse := researcher.do(router, http.MethodPost, "/api/v1/people", `{"canonical_name_ar":"موضوعآخرMatrix`+fixture.Tag()+`"}`)
	if objectResponse.Code != http.StatusCreated {
		t.Fatalf("create second person = %d: %s", objectResponse.Code, objectResponse.Body.String())
	}
	var object struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, objectResponse, &object)

	// Each body is built per session, so two allowed sessions do not collide on a
	// uniqueness constraint and end up testing the wrong refusal.
	families := []struct {
		family string
		body   func(session string) string
	}{
		{family: "person", body: func(session string) string {
			return `{"canonical_name_ar":"شخص ` + session + ` ` + fixture.Tag() + `"}`
		}},
		// An alias write needs the role and reachability to the person, so the
		// allowed sessions first create a person of their own through the API and
		// then alias it. The denied sessions are refused before the person is even
		// considered, which is why the same body works for both.
		{family: "alias", body: func(session string) string {
			return `{"value_ar":"لقب ` + session + ` ` + fixture.Tag() + `","alias_type":"laqab"}`
		}},
		{family: "family", body: func(session string) string {
			return `{"canonical_name_ar":"بيت ` + session + ` ` + fixture.Tag() + `"}`
		}},
		{family: "tribe", body: func(session string) string {
			return `{"canonical_name_ar":"قبيلة ` + session + ` ` + fixture.Tag() + `"}`
		}},
		{family: "branch", body: func(session string) string {
			return `{"family_id":"` + family.ID + `","canonical_name_ar":"فرع ` + session + ` ` + fixture.Tag() + `"}`
		}},
		{family: "place", body: func(session string) string {
			return `{"canonical_name_ar":"موضع ` + session + ` ` + fixture.Tag() + `","place_type":"city","longitude":39.2,"latitude":21.5}`
		}},
		{family: "entity relationship", body: func(session string) string {
			return `{"subject_id":"` + subject.ID + `","predicate":"sibling_of","object_id":"` + object.ID + `"}`
		}},
	}
	sessions := []struct {
		name    string
		session *identitySession
		status  int
	}{
		{name: "unauthenticated", session: nil, status: http.StatusUnauthorized},
		{name: "registered with no platform role", session: &registered, status: http.StatusForbidden},
		{name: "a tree collaborator with no platform role", session: &treeOnly, status: http.StatusForbidden},
		{name: "platform collaborator", session: &collaborator, status: http.StatusCreated},
		{name: "researcher", session: &researcher, status: http.StatusCreated},
		{name: "moderator", session: &moderator, status: http.StatusCreated},
	}
	// The registered account owns the tree and also holds a tree_collaborators row,
	// so its denial covers the tree-owner shape; treeOnly covers the collaborator
	// shape on its own. Neither holding a platform role is the whole point.
	for _, family := range families {
		path := "/api/v1/people"
		aliasSubject := subject.ID
		switch family.family {
		case "alias":
			path = "/api/v1/people/" + subject.ID + "/aliases"
		case "family":
			path = "/api/v1/families"
		case "tribe":
			path = "/api/v1/tribes"
		case "branch":
			path = "/api/v1/branches"
		case "place":
			path = "/api/v1/places"
		case "entity relationship":
			path = "/api/v1/entity-relationships"
		}
		for _, session := range sessions {
			body := family.body(session.name)
			var recorder *httptest.ResponseRecorder
			if session.session == nil {
				recorder = httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(recorder, request)
			} else {
				if family.family == "alias" && session.status == http.StatusCreated {
					// The prerequisite person, written by the session under test so
					// the alias attempt is the only thing that differs.
					own := session.session.do(router, http.MethodPost, "/api/v1/people", `{"canonical_name_ar":"شخصAlias`+session.name+fixture.Tag()+`"}`)
					if own.Code != http.StatusCreated {
						t.Fatalf("%s could not create the person its alias needs: %s", session.name, own.Body.String())
					}
					var person struct {
						ID string `json:"id"`
					}
					decodeIdentityBody(t, own, &person)
					aliasSubject = person.ID
				}
				if family.family == "alias" {
					path = "/api/v1/people/" + aliasSubject + "/aliases"
					body = family.body(session.name)
				}
				recorder = session.session.do(router, http.MethodPost, path, body)
			}
			if recorder.Code != session.status {
				t.Fatalf("%s / %s = %d, want %d: %s", family.family, session.name, recorder.Code, session.status, recorder.Body.String())
			}
		}
	}

	// The read half of the surface is gated the same way, so the identity endpoints
	// cannot become a public reader of research rows.
	if recorder := anonymous(router, http.MethodGet, "/api/v1/people"); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous people list = %d, want 401", recorder.Code)
	}
	if recorder := registered.do(router, http.MethodGet, "/api/v1/people", ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("a registered account listed people = %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
	if recorder := researcher.do(router, http.MethodGet, "/api/v1/people", ""); recorder.Code != http.StatusOK {
		t.Fatalf("a researcher listed people = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

// newTreeOnlyAccount returns an account that holds a tree_collaborators row and
// nothing else, so the matrix can name that shape on its own.
func newTreeOnlyAccount(t *testing.T, fixture *testsupport.Fixture, treeID, inviterID string) identitySession {
	t.Helper()
	session := newIdentitySession(t, fixture, "مشارك شجرة فقط", "")
	fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'edit', $3)`,
		treeID, session.userID, inviterID)
	return session
}

// TestIdentityHTTPRefusals pins the status of every refusal the plan requires, at
// the boundary the product serves. Each case also proves the refusal changed
// nothing: the row is still what it was, and no audit event was written.
func TestIdentityHTTPRefusals(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	researcher := newIdentitySession(t, fixture, "باحث", "researcher")

	// A person a published tree version rests on. The tree is built through the
	// production route so the fixture is a real published interpretation.
	treeResponse := researcher.do(router, http.MethodPost, "/api/v1/trees",
		`{"name_ar":"شجرة منشورة`+fixture.Tag()+`","visibility":"public","people":[{"canonical_name_ar":"شخص منشور`+fixture.Tag()+`"}]}`)
	if treeResponse.Code != http.StatusCreated {
		t.Fatalf("create tree = %d: %s", treeResponse.Code, treeResponse.Body.String())
	}
	var tree struct {
		Tree struct {
			ID string `json:"id"`
		} `json:"tree"`
		SelectedVersion struct {
			ID string `json:"id"`
		} `json:"selectedVersion"`
		Nodes []struct {
			PersonID string `json:"personId"`
		} `json:"nodes"`
	}
	decodeIdentityBody(t, treeResponse, &tree)
	publishResponse := researcher.do(router, http.MethodPost, "/api/v1/trees/"+tree.Tree.ID+"/publish", `{"note":"نشر"}`)
	if publishResponse.Code != http.StatusOK {
		t.Fatalf("publish tree = %d: %s", publishResponse.Code, publishResponse.Body.String())
	}
	if len(tree.Nodes) != 1 {
		t.Fatalf("the published tree carries %d nodes, want 1", len(tree.Nodes))
	}
	publishedPersonID := tree.Nodes[0].PersonID

	// A family, a tribe, a branch and a place to aim the public-reference refusals
	// at, plus a research person to aim the duplicate-alias refusal at.
	familyResponse := researcher.do(router, http.MethodPost, "/api/v1/families", `{"canonical_name_ar":"بيت refusals `+fixture.Tag()+`"}`)
	var family struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, familyResponse, &family)
	tribeResponse := researcher.do(router, http.MethodPost, "/api/v1/tribes", `{"canonical_name_ar":"قبيلة refusals `+fixture.Tag()+`"}`)
	var tribe struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, tribeResponse, &tribe)
	branchResponse := researcher.do(router, http.MethodPost, "/api/v1/branches", `{"family_id":"`+family.ID+`","canonical_name_ar":"فرع refusals `+fixture.Tag()+`"}`)
	var branch struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, branchResponse, &branch)
	placeResponse := researcher.do(router, http.MethodPost, "/api/v1/places", `{"canonical_name_ar":"موضع refusals `+fixture.Tag()+`","place_type":"region"}`)
	var place struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, placeResponse, &place)
	researchPersonResponse := researcher.do(router, http.MethodPost, "/api/v1/people", `{"canonical_name_ar":"شخص refusals `+fixture.Tag()+`"}`)
	var researchPerson struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, researchPersonResponse, &researchPerson)
	if recorder := researcher.do(router, http.MethodPost, "/api/v1/people/"+researchPerson.ID+"/aliases",
		`{"value_ar":"لقب refusals `+fixture.Tag()+`","alias_type":"kunyah"}`); recorder.Code != http.StatusCreated {
		t.Fatalf("create alias = %d: %s", recorder.Code, recorder.Body.String())
	}
	auditBefore := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, researcher.userID)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		detail string
	}{
		{
			name: "update a person in a published version", method: http.MethodPatch,
			path: "/api/v1/people/" + publishedPersonID,
			body: `{"canonical_name_ar":"اسم جديد","gender":"male","reason_ar":"تصحيح"}`, status: http.StatusConflict,
			detail: "published interpretation",
		},
		{
			name: "delete a person in a published version", method: http.MethodDelete,
			path: "/api/v1/people/" + publishedPersonID, status: http.StatusConflict,
			detail: "tree interpretation",
		},
		{
			name: "add an alias to a person in a published version", method: http.MethodPost,
			path: "/api/v1/people/" + publishedPersonID + "/aliases",
			body: `{"value_ar":"أبو محمد","alias_type":"kunyah"}`, status: http.StatusConflict,
			detail: "published interpretation",
		},
		{
			name: "update a family", method: http.MethodPatch,
			path: "/api/v1/families/" + family.ID,
			body: `{"canonical_name_ar":"اسم آخر","reason_ar":"تصحيح"}`, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "delete a family", method: http.MethodDelete,
			path: "/api/v1/families/" + family.ID, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "update a tribe", method: http.MethodPatch,
			path: "/api/v1/tribes/" + tribe.ID,
			body: `{"canonical_name_ar":"اسم آخر","reason_ar":"تصحيح"}`, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "delete a tribe", method: http.MethodDelete,
			path: "/api/v1/tribes/" + tribe.ID, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "update a branch", method: http.MethodPatch,
			path: "/api/v1/branches/" + branch.ID,
			body: `{"canonical_name_ar":"اسم آخر","reason_ar":"تصحيح"}`, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "delete a branch", method: http.MethodDelete,
			path: "/api/v1/branches/" + branch.ID, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "update a place", method: http.MethodPatch,
			path: "/api/v1/places/" + place.ID,
			body: `{"canonical_name_ar":"اسم آخر","place_type":"city","reason_ar":"تصحيح"}`, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "delete a place", method: http.MethodDelete,
			path: "/api/v1/places/" + place.ID, status: http.StatusConflict,
			detail: "once they are published",
		},
		{
			name: "a duplicate alias", method: http.MethodPost,
			path: "/api/v1/people/" + researchPerson.ID + "/aliases",
			body: `{"value_ar":"لقب refusals ` + fixture.Tag() + `","alias_type":"kunyah"}`, status: http.StatusConflict,
			detail: "conflicts",
		},
		{
			name: "an unknown person", method: http.MethodPatch,
			path: "/api/v1/people/" + uuid.NewString(),
			body: `{"canonical_name_ar":"اسم","gender":"male","reason_ar":"تصحيح"}`, status: http.StatusNotFound,
			detail: "not found",
		},
		{
			name: "a malformed person id", method: http.MethodGet,
			path: "/api/v1/people/not-a-uuid", status: http.StatusNotFound,
			detail: "not found",
		},
		{
			name: "an unknown place", method: http.MethodDelete,
			path: "/api/v1/places/" + uuid.NewString(), status: http.StatusNotFound,
			detail: "not found",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := researcher.do(router, testCase.method, testCase.path, testCase.body)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, testCase.status, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), testCase.detail) {
				t.Fatalf("the body does not carry the reason: %s", recorder.Body.String())
			}
		})
	}
	if auditAfter := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, researcher.userID); auditAfter != auditBefore {
		t.Fatalf("the refused requests wrote %d audit events, want none", auditAfter-auditBefore)
	}
	// The published person and the four public reference rows are untouched.
	if got := fixture.Count(`SELECT count(*) FROM people WHERE id = $1 AND canonical_name_ar = $2`, publishedPersonID, "شخص منشور"+fixture.Tag()); got != 1 {
		t.Fatal("the published person changed through a refused request")
	}
	if got := fixture.Count(`SELECT count(*) FROM families WHERE id = $1 AND canonical_name_ar = $2`, family.ID, "بيت refusals "+fixture.Tag()); got != 1 {
		t.Fatal("the family changed through a refused request")
	}
	if got := fixture.Count(`SELECT count(*) FROM tribes WHERE id = $1 AND canonical_name_ar = $2`, tribe.ID, "قبيلة refusals "+fixture.Tag()); got != 1 {
		t.Fatal("the tribe changed through a refused request")
	}
	if got := fixture.Count(`SELECT count(*) FROM branches WHERE id = $1 AND canonical_name_ar = $2`, branch.ID, "فرع refusals "+fixture.Tag()); got != 1 {
		t.Fatal("the branch changed through a refused request")
	}
	if got := fixture.Count(`SELECT count(*) FROM places WHERE id = $1 AND canonical_name_ar = $2`, place.ID, "موضع refusals "+fixture.Tag()); got != 1 {
		t.Fatal("the place changed through a refused request")
	}
}

// TestIdentityHTTPValidatesItsInput covers the request contract: bounded bodies,
// no unknown fields, and the closed vocabularies. A field the API does not bind
// cannot be smuggled in, and a body that exceeds the bound is refused rather than
// read.
func TestIdentityHTTPValidatesItsInput(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	researcher := newIdentitySession(t, fixture, "باحث", "researcher")

	cases := []struct {
		name string
		path string
		body string
		want int
	}{
		{name: "a missing name", path: "/api/v1/people", body: `{"gender":"male"}`, want: http.StatusBadRequest},
		{name: "a gender outside the vocabulary", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص","gender":"other"}`, want: http.StatusBadRequest},
		{name: "an unparseable date", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص","birth_date_from":"0700"}`, want: http.StatusBadRequest},
		{name: "a reversed date range", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص","birth_date_from":"0750-01-01","birth_date_to":"0700-01-01"}`, want: http.StatusBadRequest},
		// identity_status and merged_into_id are not request fields at all, so a
		// caller cannot arrive pre-approved or pre-merged.
		{name: "identity_status is not bindable", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص","identity_status":"reviewed"}`, want: http.StatusBadRequest},
		{name: "merged_into_id is not bindable", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص","merged_into_id":"` + uuid.NewString() + `"}`, want: http.StatusBadRequest},
		{name: "an unknown field", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص","notes":"ملاحظة","note":"حقل غير معروف"}`, want: http.StatusBadRequest},
		{name: "a minimal person is accepted", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص"}`, want: http.StatusCreated},
		{name: "a place without a type", path: "/api/v1/places", body: `{"canonical_name_ar":"موضع"}`, want: http.StatusBadRequest},
		{name: "a place type outside the vocabulary", path: "/api/v1/places", body: `{"canonical_name_ar":"موضع","place_type":"hamlet"}`, want: http.StatusBadRequest},
		{name: "a longitude without a latitude", path: "/api/v1/places", body: `{"canonical_name_ar":"موضع","place_type":"city","longitude":39.2}`, want: http.StatusBadRequest},
		{name: "a latitude past the pole", path: "/api/v1/places", body: `{"canonical_name_ar":"موضع","place_type":"city","longitude":0,"latitude":91}`, want: http.StatusBadRequest},
		{name: "a relationship predicate outside the vocabulary", path: "/api/v1/entity-relationships", body: `{"subject_id":"` + uuid.NewString() + `","predicate":"mentor_of","object_id":"` + uuid.NewString() + `"}`, want: http.StatusBadRequest},
		{name: "a relationship status is not bindable on create", path: "/api/v1/entity-relationships", body: `{"subject_id":"` + uuid.NewString() + `","predicate":"father_of","object_id":"` + uuid.NewString() + `","status":"supported"}`, want: http.StatusBadRequest},
		{name: "a malformed body", path: "/api/v1/people", body: `{"canonical_name_ar":`, want: http.StatusBadRequest},
		{name: "a body with no content type", path: "/api/v1/people", body: `{"canonical_name_ar":"شخص"}`, want: http.StatusBadRequest},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			if testCase.name != "a body with no content type" {
				request.Header.Set("Content-Type", "application/json")
			}
			request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: researcher.Token})
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, testCase.want, recorder.Body.String())
			}
		})
	}

	// A body past the one-megabyte bound is refused whole. The reader is bounded,
	// so the tail of the body is never parsed and no person is written from the
	// part of it that was read.
	before := fixture.Count(`SELECT count(*) FROM people WHERE created_by = $1`, researcher.userID)
	oversized := `{"canonical_name_ar":"` + strings.Repeat("ش", 2<<20) + `"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/people", strings.NewReader(oversized))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: researcher.Token})
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("an oversized body = %d, want 400", recorder.Code)
	}
	if after := fixture.Count(`SELECT count(*) FROM people WHERE created_by = $1`, researcher.userID); after != before {
		t.Fatalf("the oversized body wrote %d people", after-before)
	}
}

// TestIdentityHTTPAuditsEveryMutation: the happy path writes exactly one audit
// event per mutation, with the actor, the action and the record it names.
func TestIdentityHTTPAuditsEveryMutation(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	researcher := newIdentitySession(t, fixture, "باحث", "researcher")

	personResponse := researcher.do(router, http.MethodPost, "/api/v1/people", `{"canonical_name_ar":"مدقق`+fixture.Tag()+`"}`)
	var person struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, personResponse, &person)
	aliasResponse := researcher.do(router, http.MethodPost, "/api/v1/people/"+person.ID+"/aliases", `{"value_ar":"لقب مدقق`+fixture.Tag()+`"}`)
	var alias struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, aliasResponse, &alias)
	if recorder := researcher.do(router, http.MethodPatch, "/api/v1/people/"+person.ID,
		`{"canonical_name_ar":"مدقق مصحح`+fixture.Tag()+`","gender":"male","reason_ar":"تصحيح إملائي"}`); recorder.Code != http.StatusOK {
		t.Fatalf("update person = %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder := researcher.do(router, http.MethodDelete, "/api/v1/person-aliases/"+alias.ID, `{"reason_ar":"لقب مكرر"}`); recorder.Code != http.StatusOK {
		t.Fatalf("delete alias = %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder := researcher.do(router, http.MethodDelete, "/api/v1/people/"+person.ID, `{"reason_ar":"سجل تجريبي"}`); recorder.Code != http.StatusOK {
		t.Fatalf("delete person = %d: %s", recorder.Code, recorder.Body.String())
	}

	cases := []struct {
		action     string
		entityType string
		entityID   string
	}{
		{action: "person_created", entityType: "person", entityID: person.ID},
		{action: "person_alias_created", entityType: "person_alias", entityID: alias.ID},
		{action: "person_updated", entityType: "person", entityID: person.ID},
		{action: "person_alias_deleted", entityType: "person_alias", entityID: alias.ID},
		{action: "person_deleted", entityType: "person", entityID: person.ID},
	}
	for _, testCase := range cases {
		got := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = $2 AND entity_type = $3 AND entity_id = $4`,
			researcher.userID, testCase.action, testCase.entityType, testCase.entityID)
		if got != 1 {
			t.Fatalf("audit events for %s = %d, want exactly 1", testCase.action, got)
		}
	}
	// The update recorded what it changed and why, and the delete recorded what was
	// removed, so the log can explain the record after it is gone.
	var reason string
	if err := fixture.QueryRow(`SELECT reason_ar FROM audit_log WHERE action = 'person_deleted' AND entity_id = $1`, person.ID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "سجل تجريبي" {
		t.Fatalf("the delete audit reason = %q, want the submitted one", reason)
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'person_updated' AND entity_id = $1 AND after_value->>'canonical_name_ar' = $2`,
		person.ID, "مدقق مصحح"+fixture.Tag()); got != 1 {
		t.Fatal("the update audit event does not carry the new name")
	}
	// And the rows are really gone, so the audit log is the only place they survive.
	if got := fixture.Count(`SELECT count(*) FROM people WHERE id = $1`, person.ID); got != 0 {
		t.Fatal("the person survived the delete")
	}
}

// TestReferenceRowsAreNotPublicUntilPublished is the reference-family half of the
// guarantee this plan is judged on, driven through the real router so the handler,
// the service, the central reference policy, the dictionary, the search service and
// the map are all exercised together. A family, tribe, branch or place created
// through the new API belongs to no version and is research-only: it is absent from
// the anonymous dictionary index, 404s on its anonymous detail and is absent from the
// anonymous map, and a role holder sees all of it.
func TestReferenceRowsAreNotPublicUntilPublished(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})
	writer := newIdentitySession(t, fixture, "باحث المراجع", "researcher")

	familyResponse := writer.do(router, http.MethodPost, "/api/v1/families",
		`{"canonical_name_ar":"بيت recherche`+fixture.Tag()+`","description_ar":"وصف"}`)
	if familyResponse.Code != http.StatusCreated {
		t.Fatalf("create family = %d: %s", familyResponse.Code, familyResponse.Body.String())
	}
	var family struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, familyResponse, &family)

	tribeResponse := writer.do(router, http.MethodPost, "/api/v1/tribes", `{"canonical_name_ar":"قبيلة recherche`+fixture.Tag()+`"}`)
	var tribe struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, tribeResponse, &tribe)

	branchResponse := writer.do(router, http.MethodPost, "/api/v1/branches",
		`{"family_id":"`+family.ID+`","canonical_name_ar":"فرع recherche`+fixture.Tag()+`"}`)
	var branch struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, branchResponse, &branch)

	placeResponse := writer.do(router, http.MethodPost, "/api/v1/places",
		`{"canonical_name_ar":"موضع recherche`+fixture.Tag()+`","place_type":"city","longitude":46.7,"latitude":24.7}`)
	var place struct {
		ID string `json:"id"`
	}
	decodeIdentityBody(t, placeResponse, &place)

	created := []struct {
		Kind       string
		DetailPath string
		IndexKind  string
		ID         string
		Name       string
	}{
		{Kind: "family", DetailPath: "/api/v1/dictionary/families/", IndexKind: "families", ID: family.ID, Name: "بيت recherche" + fixture.Tag()},
		{Kind: "tribe", DetailPath: "/api/v1/dictionary/tribes/", IndexKind: "tribes", ID: tribe.ID, Name: "قبيلة recherche" + fixture.Tag()},
		{Kind: "place", DetailPath: "/api/v1/dictionary/places/", IndexKind: "places", ID: place.ID, Name: "موضع recherche" + fixture.Tag()},
		// A branch has no dictionary detail kind, so it is pinned through the index
		// and through the family page that lists it.
		{Kind: "branch", IndexKind: "branches", ID: branch.ID, Name: "فرع recherche" + fixture.Tag()},
	}

	for _, row := range created {
		t.Run(row.Kind, func(t *testing.T) {
			// The anonymous dictionary index, by the row's own name.
			index := anonymous(router, http.MethodGet, "/api/v1/dictionary?kind="+row.IndexKind+"&q="+url.QueryEscape(row.Name))
			if index.Code != http.StatusOK {
				t.Fatalf("anonymous dictionary index = %d: %s", index.Code, index.Body.String())
			}
			var payload struct {
				Query string `json:"query"`
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			decodeIdentityBody(t, index, &payload)
			for _, item := range payload.Items {
				if item.ID == row.ID {
					t.Fatalf("the research-only %s is in the anonymous dictionary index: %s", row.Kind, index.Body.String())
				}
			}
			if strings.Contains(index.Body.String(), row.ID) {
				t.Fatalf("the anonymous dictionary index disclosed the research-only %s id: %s", row.Kind, index.Body.String())
			}
			// The anonymous detail, where the endpoint has one.
			if row.DetailPath != "" {
				detail := anonymous(router, http.MethodGet, row.DetailPath+row.ID)
				if detail.Code != http.StatusNotFound {
					t.Fatalf("anonymous %s detail = %d, want 404: %s", row.Kind, detail.Code, detail.Body.String())
				}
				missing := anonymous(router, http.MethodGet, row.DetailPath+uuid.NewString())
				if missing.Code != http.StatusNotFound {
					t.Fatalf("anonymous %s detail for a missing row = %d, want the same 404: %s", row.Kind, missing.Code, missing.Body.String())
				}
			}
			// The anonymous search index, by the row's own name.
			found := anonymous(router, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(row.Name))
			if found.Code != http.StatusOK {
				t.Fatalf("anonymous search = %d: %s", found.Code, found.Body.String())
			}
			if strings.Contains(found.Body.String(), row.ID) {
				t.Fatalf("anonymous search found the research-only %s: %s", row.Kind, found.Body.String())
			}
			// And the role that wrote it sees all of it.
			if owned := writer.do(router, http.MethodGet, "/api/v1/dictionary?kind="+row.IndexKind+"&q="+url.QueryEscape(row.Name), ""); owned.Code != http.StatusOK || !strings.Contains(owned.Body.String(), row.ID) {
				t.Fatalf("the writer's own dictionary index does not carry the %s: %d %s", row.Kind, owned.Code, owned.Body.String())
			}
		})
	}

	// The anonymous map. The place was written with a point, so it would be a marker
	// on the public map if it were published.
	mapResponse := anonymous(router, http.MethodGet, "/api/v1/map")
	if mapResponse.Code != http.StatusOK {
		t.Fatalf("anonymous map = %d: %s", mapResponse.Code, mapResponse.Body.String())
	}
	if strings.Contains(mapResponse.Body.String(), place.ID) {
		t.Fatalf("the research-only place is on the anonymous map: %s", mapResponse.Body.String())
	}
	// The map is filtered by name too, so a query for it finds nothing.
	narrowed := anonymous(router, http.MethodGet, "/api/v1/map?place_id="+place.ID)
	if narrowed.Code != http.StatusOK {
		t.Fatalf("anonymous map for a research-only place = %d: %s", narrowed.Code, narrowed.Body.String())
	}
	for _, feature := range mapFeatures(t, narrowed) {
		if feature.PlaceID == place.ID {
			t.Fatalf("the research-only place answered a map lookup: %s", narrowed.Body.String())
		}
	}
	// The role that wrote it sees it on the map, on the same endpoint.
	writerMap := writer.do(router, http.MethodGet, "/api/v1/map?place_id="+place.ID, "")
	if writerMap.Code != http.StatusOK {
		t.Fatalf("the writer's map = %d: %s", writerMap.Code, writerMap.Body.String())
	}
	foundOnMap := false
	for _, feature := range mapFeatures(t, writerMap) {
		if feature.PlaceID == place.ID {
			foundOnMap = true
		}
	}
	if !foundOnMap {
		t.Fatalf("the research-only place is missing from the writer's own map: %s", writerMap.Body.String())
	}
	// The place detail endpoint answers the same way the map list does.
	if recorder := anonymous(router, http.MethodGet, "/api/v1/map/places/"+place.ID); recorder.Code != http.StatusNotFound {
		t.Fatalf("anonymous place map detail = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
	if recorder := writer.do(router, http.MethodGet, "/api/v1/map/places/"+place.ID, ""); recorder.Code != http.StatusOK {
		t.Fatalf("the writer's place map detail = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	// A research-only place is kept off the page of a public person too, so the
	// person's place list cannot be the way round it.
	if got := fixture.Count(`SELECT count(*) FROM places WHERE id = $1 AND visibility = 'private'`, place.ID); got != 1 {
		t.Fatal("the created place is not research-only in the database")
	}
}

// TestSeedRowsStayPublicThroughTheReferenceMigration is the regression that matters
// most for the migration: the backfill has to leave every row that was public before
// it exactly as it was, in every read path that lists one. The counts are taken from
// a migrated, seeded database - the same state a developer's database is in - and the
// named seeded rows are checked one by one.
func TestSeedRowsStayPublicThroughTheReferenceMigration(t *testing.T) {
	fixture := testsupport.New(t)
	router := NewRouter(Dependencies{DB: fixture.Pool()})

	// Every reference table carries the column, the backfill published every seeded
	// row, and the column default is research-only so a writer that forgets the
	// argument cannot publish by accident.
	for _, table := range []string{"families", "tribes", "branches", "places"} {
		if got := fixture.Count(`SELECT count(*) FROM ` + table + ` WHERE visibility IS NULL`); got != 0 {
			t.Fatalf("%s has %d row(s) with no visibility", table, got)
		}
		if public, total := fixture.Count(`SELECT count(*) FROM `+table+` WHERE visibility = 'public'`), fixture.Count(`SELECT count(*) FROM `+table); public != total {
			t.Fatalf("%s: %d public of %d rows, want the backfill to have published every seeded row", table, public, total)
		}
		var columnDefault string
		if err := fixture.QueryRow(`SELECT column_default FROM information_schema.columns WHERE table_name = $1 AND column_name = 'visibility'`, table).Scan(&columnDefault); err != nil {
			t.Fatalf("%s has no visibility column: %v", table, err)
		}
		if !strings.Contains(columnDefault, "private") {
			t.Fatalf("%s.visibility defaults to %q, want the research-only value", table, columnDefault)
		}
		// The vocabulary is the two values sources.visibility already uses.
		if got := fixture.Count(`SELECT count(DISTINCT chk.check_clause) FROM `+table+` t JOIN pg_constraint con ON con.conrelid = $1::regclass AND con.contype = 'c' JOIN LATERAL (SELECT pg_get_constraintdef(con.oid) AS check_clause) chk ON true WHERE t.visibility NOT IN ('public','private')`, table); got != 0 {
			t.Fatalf("%s holds %d row(s) outside the two-value vocabulary", table, got)
		}
	}

	// Every seeded reference row is listed by the anonymous index, exactly as before.
	for _, kind := range []string{"families", "branches", "places"} {
		index := anonymous(router, http.MethodGet, "/api/v1/dictionary?kind="+kind)
		if index.Code != http.StatusOK {
			t.Fatalf("anonymous %s index = %d: %s", kind, index.Code, index.Body.String())
		}
		var payload struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		decodeIdentityBody(t, index, &payload)
		if len(payload.Items) != fixture.Count(`SELECT count(*) FROM `+dictionaryTable(kind)+` WHERE visibility = 'public'`) {
			t.Fatalf("the anonymous %s index lists %d rows, the database has %d public", kind, len(payload.Items), fixture.Count(`SELECT count(*) FROM `+dictionaryTable(kind)+` WHERE visibility = 'public'`))
		}
		if len(payload.Items) == 0 {
			t.Fatalf("the anonymous %s index is empty, so the backfill changed what a reader sees", kind)
		}
	}
	// The named seeded places are on the anonymous map, with their coordinates.
	mapResponse := anonymous(router, http.MethodGet, "/api/v1/map")
	if mapResponse.Code != http.StatusOK {
		t.Fatalf("anonymous map = %d: %s", mapResponse.Code, mapResponse.Body.String())
	}
	features := mapFeatures(t, mapResponse)
	placeFeatures := 0
	for _, feature := range features {
		if feature.Kind == "place" {
			placeFeatures++
			if feature.Longitude == nil || feature.Latitude == nil {
				t.Fatalf("a seeded place lost its coordinates: %+v", feature)
			}
		}
	}
	if want := fixture.Count(`SELECT count(*) FROM places WHERE visibility = 'public' AND geometry IS NOT NULL`); placeFeatures != want {
		t.Fatalf("the anonymous map draws %d place(s), the database has %d public place(s) with geometry", placeFeatures, want)
	}
	// A seeded place still answers its own map detail, and a seeded family still
	// answers its dictionary page with its branches and its people.
	seededPlace := "20000000-0000-0000-0000-000000000001"
	if recorder := anonymous(router, http.MethodGet, "/api/v1/map/places/"+seededPlace); recorder.Code != http.StatusOK {
		t.Fatalf("the seeded place map detail = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	seededFamily := "c0000000-0000-0000-0000-000000000001"
	detail := anonymous(router, http.MethodGet, "/api/v1/dictionary/families/"+seededFamily)
	if detail.Code != http.StatusOK {
		t.Fatalf("the seeded family detail = %d, want 200: %s", detail.Code, detail.Body.String())
	}
	if !strings.Contains(detail.Body.String(), "الفرع الأول") {
		t.Fatalf("the seeded family lost its public branch: %s", detail.Body.String())
	}
	// And the seeded data is still searchable by a reader with no session.
	found := anonymous(router, http.MethodGet, "/api/v1/search?q="+url.QueryEscape("الرياض"))
	if found.Code != http.StatusOK || !strings.Contains(found.Body.String(), seededPlace) {
		t.Fatalf("anonymous search lost the seeded place: %d %s", found.Code, found.Body.String())
	}
}

func dictionaryTable(kind string) string {
	switch kind {
	case "families":
		return "families"
	case "tribes":
		return "tribes"
	case "branches":
		return "branches"
	default:
		return "places"
	}
}

func mapFeatures(t *testing.T, recorder *httptest.ResponseRecorder) []struct {
	Kind      string
	PlaceID   string
	Longitude *float64
	Latitude  *float64
} {
	t.Helper()
	var payload struct {
		Features []struct {
			Kind      string   `json:"kind"`
			PlaceID   string   `json:"placeId"`
			Longitude *float64 `json:"longitude"`
			Latitude  *float64 `json:"latitude"`
		} `json:"features"`
	}
	decodeIdentityBody(t, recorder, &payload)
	items := make([]struct {
		Kind      string
		PlaceID   string
		Longitude *float64
		Latitude  *float64
	}, 0, len(payload.Features))
	for _, feature := range payload.Features {
		items = append(items, struct {
			Kind      string
			PlaceID   string
			Longitude *float64
			Latitude  *float64
		}{Kind: feature.Kind, PlaceID: feature.PlaceID, Longitude: feature.Longitude, Latitude: feature.Latitude})
	}
	return items
}
