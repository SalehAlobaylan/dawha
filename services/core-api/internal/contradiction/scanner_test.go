package contradiction

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestContradictionDateChecks(t *testing.T) {
	parentBirth := dateAt(1200)
	childBirth := dateAt(1100)
	if !datesAfter(parentBirth, dateAt(1210), childBirth, dateAt(1110)) {
		t.Fatal("expected parent birth after child to be detected")
	}
	if !deathBeforeBirth(dateAt(1080), dateAt(1090), childBirth, dateAt(1110)) {
		t.Fatal("expected parent death before child birth to be detected")
	}
	if datesAfter(childBirth, dateAt(1110), parentBirth, dateAt(1210)) {
		t.Fatal("did not expect reverse date relation to be flagged")
	}
}

func TestEventDateChecks(t *testing.T) {
	person := personFact{BirthFrom: dateAt(1100), DeathTo: dateAt(1200)}
	if !eventBeforeBirth(eventFact{To: dateAt(1090)}, person) {
		t.Fatal("expected event before birth")
	}
	if !eventAfterDeath(eventFact{From: dateAt(1210)}, person) {
		t.Fatal("expected event after death")
	}
	if eventBeforeBirth(eventFact{To: dateAt(1150)}, person) || eventAfterDeath(eventFact{From: dateAt(1150)}, person) {
		t.Fatal("did not expect in-range event to be flagged")
	}
}

func TestDuplicateIdentityFindingRequiresSharedTree(t *testing.T) {
	firstID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	secondID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	first := personFact{ID: firstID, Normalized: "عبدالله", Trees: map[string]struct{}{"tree-1": {}}}
	second := personFact{ID: secondID, Normalized: "عبدالله", Trees: map[string]struct{}{"tree-1": {}}}
	values := map[string]personFact{firstID.String(): first, secondID.String(): second}
	findings := make([]findingInput, 0)
	addDuplicateIdentityFindings(values, func(value findingInput) { findings = append(findings, value) })
	if len(findings) != 1 || findings[0].Type != "duplicate_identity_in_branch" {
		t.Fatalf("unexpected duplicate findings: %+v", findings)
	}
	second.Trees = map[string]struct{}{"tree-2": {}}
	values[secondID.String()] = second
	findings = nil
	addDuplicateIdentityFindings(values, func(value findingInput) { findings = append(findings, value) })
	if len(findings) != 0 {
		t.Fatalf("did not expect cross-tree duplicate finding: %+v", findings)
	}
}

func TestLoopFindingDetectsCycleOnce(t *testing.T) {
	versionID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	first := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	second := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	third := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	edges := []treeEdge{
		{ID: uuid.New(), VersionID: versionID, ParentID: first, ChildID: second},
		{ID: uuid.New(), VersionID: versionID, ParentID: second, ChildID: third},
		{ID: uuid.New(), VersionID: versionID, ParentID: third, ChildID: first},
	}
	findings := make([]findingInput, 0)
	addLoopFindings(edges, func(value findingInput) { findings = append(findings, value) })
	if len(findings) != 1 || findings[0].Type != "impossible_relationship_loop" {
		t.Fatalf("unexpected loop findings: %+v", findings)
	}
}

func dateAt(year int) pgtype.Date {
	return pgtype.Date{Time: time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC), Valid: true}
}
