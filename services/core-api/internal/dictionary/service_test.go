package dictionary

import "testing"

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
	query, args := indexQuery("people", "عبد")
	if len(args) != 1 || args[0] != "عبد" {
		t.Fatalf("unexpected query args: %#v", args)
	}
	if !containsText(query, "person_aliases") || !containsText(query, "normalized_value_ar") {
		t.Fatalf("expected alias search in query: %s", query)
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
