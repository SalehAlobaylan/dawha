package trees

import "testing"

func TestValidateForkInputDefaultsToPrivate(t *testing.T) {
	input, err := validateForkInput(ForkTreeInput{VersionID: " version-1 "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.VersionID != "version-1" || input.Visibility != "private" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
}

func TestValidateForkInputRejectsInvalidVisibility(t *testing.T) {
	if _, err := validateForkInput(ForkTreeInput{VersionID: "version-1", Visibility: "secret"}); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestBuildTreeDiffMatchesPeopleSemantically(t *testing.T) {
	fromTree := TreeSummary{ID: "from-tree", Name: "الأصل"}
	toTree := TreeSummary{ID: "to-tree", Name: "التفريع"}
	fromVersion := TreeVersionView{ID: "from-version", Number: 1}
	toVersion := TreeVersionView{ID: "to-version", Number: 1}
	fromNodes := []diffNode{
		{nodeID: "from-node-1", personID: "person-1", displayName: "عبدالله", years: "1100 — 1150", dateKey: "1100|1150||"},
		{nodeID: "from-node-2", personID: "person-2", displayName: "محمد", years: "1150 — 1200", dateKey: "1150|1200||"},
	}
	toNodes := []diffNode{
		{nodeID: "to-node-1", personID: "person-1", displayName: "عبدالله", years: "1100 — 1160", dateKey: "1100|1160||"},
		{nodeID: "to-node-2", personID: "person-2", displayName: "محمد", years: "1150 — 1200", dateKey: "1150|1200||"},
		{nodeID: "to-node-3", personID: "person-3", displayName: "سعد", years: "1180 — 1230", dateKey: "1180|1230||"},
	}
	fromRelationships := []diffRelationship{{id: "from-rel-1", subjectNodeID: "from-node-1", objectNodeID: "from-node-2", predicate: "parent_of", status: "interpreted", sourceID: "source-1"}}
	toRelationships := []diffRelationship{
		{id: "to-rel-1", subjectNodeID: "to-node-1", objectNodeID: "to-node-2", predicate: "parent_of", status: "disputed", sourceID: "source-2"},
		{id: "to-rel-2", subjectNodeID: "to-node-2", objectNodeID: "to-node-3", predicate: "parent_of", status: "interpreted"},
	}

	diff := buildTreeDiff(fromTree, fromVersion, fromNodes, fromRelationships, toTree, toVersion, toNodes, toRelationships)
	if len(diff.PeopleAdded) != 1 || diff.PeopleAdded[0].PersonID != "person-3" {
		t.Fatalf("unexpected added people: %+v", diff.PeopleAdded)
	}
	if len(diff.PeopleRemoved) != 0 {
		t.Fatalf("unexpected removed people: %+v", diff.PeopleRemoved)
	}
	if len(diff.DateChanges) != 1 || diff.DateChanges[0].PersonID != "person-1" {
		t.Fatalf("unexpected date changes: %+v", diff.DateChanges)
	}
	if len(diff.RelationshipChanges) != 1 || diff.RelationshipChanges[0].AfterStatus != "disputed" {
		t.Fatalf("unexpected relationship changes: %+v", diff.RelationshipChanges)
	}
	if len(diff.SourcesAdded) != 1 || len(diff.SourcesRemoved) != 1 {
		t.Fatalf("unexpected source changes: %+v", diff)
	}
	if len(diff.RelationshipsAdded) != 1 || diff.AffectedDescendants != 1 {
		t.Fatalf("unexpected relationship additions or impact: %+v", diff)
	}
}
