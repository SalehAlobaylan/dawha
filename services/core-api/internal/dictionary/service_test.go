package dictionary

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
)

func TestValidIndexKind(t *testing.T) {
	for _, kind := range []string{"families", "tribes", "branches", "people", "places", "sources", "questions", "disputed-claims"} {
		if !validIndexKind(kind) {
			t.Fatalf("expected %q to be valid", kind)
		}
	}
	if validIndexKind("unknown") {
		t.Fatal("expected unknown kind to be invalid")
	}
}

func TestIndexQueryIncludesAliasSearch(t *testing.T) {
	query, args := indexQuery("people", "عبد", visibility.Anonymous())
	if len(args) != 3 || args[0] != "عبد" {
		t.Fatalf("unexpected query args: %#v", args)
	}
	if !containsText(query, "person_aliases") || !containsText(query, "normalized_value_ar") {
		t.Fatalf("expected alias search in query: %s", query)
	}
}

func TestAnonymousIndexQueryDeclaresPublicMembership(t *testing.T) {
	policy := visibility.Anonymous()
	cases := map[string][]string{
		"people":          {"person_aliases", "vis_version.state = 'published'", "vis_tree.visibility = 'public'"},
		"disputed-claims": {"claim_evidence", "vis_source.visibility = 'public'", "vis_source.visibility = 'private'"},
		"questions":       {"question_sources", "vis_source.visibility = 'public'"},
		"sources":         {"vis_source.visibility = 'public'"},
	}
	for kind, fragments := range cases {
		query, _ := indexQuery(kind, "", policy)
		for _, fragment := range fragments {
			if !containsText(query, fragment) {
				t.Fatalf("kind %s: expected %q in query: %s", kind, fragment, query)
			}
		}
	}
}

func TestOwnerIndexQueryIncludesOwnerGrant(t *testing.T) {
	policy, err := visibility.WithResearch("10000000-0000-0000-0000-000000000001", false)
	if err != nil {
		t.Fatal(err)
	}
	query, args := indexQuery("people", "", policy)
	if !containsText(query, "vis_person.created_by = $2::uuid") {
		t.Fatalf("expected owner grant in query: %s", query)
	}
	// The person grant and the alias source grant each bind the actor.
	if len(args) != 3 || args[1] == uuid.Nil || args[2] == uuid.Nil {
		t.Fatalf("expected both actor parameters to be bound: %#v", args)
	}
	if !containsText(query, "pa.source_id IS NULL") {
		t.Fatalf("expected the alias source rule in query: %s", query)
	}
}

func containsText(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
