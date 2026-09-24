package temporalanalysis

import (
	"testing"
	"time"
)

func TestBuildIntervalObservationUsesBoundedStoredDates(t *testing.T) {
	parentFrom := dateAt(1100)
	parentTo := dateAt(1110)
	childFrom := dateAt(1130)
	childTo := dateAt(1140)
	observation, valid := buildIntervalObservation(parentFrom, parentTo, childFrom, childTo)
	if !valid {
		t.Fatal("expected a valid bounded interval")
	}
	if observation.LowerYears < 19.5 || observation.LowerYears > 20.5 || observation.UpperYears < 39.5 || observation.UpperYears > 40.5 {
		t.Fatalf("unexpected interval: %+v", observation)
	}
	if observation.ParentBirthFrom != "1100-01-01" || observation.ChildBirthTo != "1140-01-01" {
		t.Fatalf("date provenance was not retained: %+v", observation)
	}
}

func TestBuildIntervalObservationRejectsImpossibleChronology(t *testing.T) {
	if _, valid := buildIntervalObservation(dateAt(1140), dateAt(1150), dateAt(1100), dateAt(1110)); valid {
		t.Fatal("expected a parent born after the child to be rejected")
	}
	if _, valid := buildIntervalObservation(dateAt(1110), dateAt(1100), dateAt(1130), dateAt(1140)); valid {
		t.Fatal("expected an inverted parent range to be rejected")
	}
}

func TestReferenceBandAndComparison(t *testing.T) {
	reference := []IntervalObservation{
		{MidpointYears: 20, LowerYears: 19, UpperYears: 21},
		{MidpointYears: 25, LowerYears: 24, UpperYears: 26},
		{MidpointYears: 30, LowerYears: 29, UpperYears: 31},
		{MidpointYears: 35, LowerYears: 34, UpperYears: 36},
	}
	q1, median, q3, ok := referenceBand(reference, 3)
	if !ok || q1 >= median || median >= q3 {
		t.Fatalf("unexpected reference band: %v %v %v", q1, median, q3)
	}
	above := compareInterval(IntervalObservation{LowerYears: 50, UpperYears: 55}, q1, median, q3)
	if above.Relation != FindingRelationAbove {
		t.Fatalf("expected above-range comparison, got %+v", above)
	}
	overlap := compareInterval(IntervalObservation{LowerYears: q1 - 1, UpperYears: q1 + 1}, q1, median, q3)
	if overlap.Relation != FindingRelationOverlap {
		t.Fatalf("expected overlapping comparison, got %+v", overlap)
	}
}

func TestAnalyzeGenerationEdgesHandlesMultipleTargetRelationships(t *testing.T) {
	scope := treeScope{TreeID: "tree", TreeVersionID: "version", VersionNumber: 1, VersionState: "published"}
	rows := []edgeRow{
		{RelationshipID: "reference-1", ParentPersonID: "p1", ChildPersonID: "c1", ExclusionReason: "qualified", ParentBirthFrom: dateAt(1100), ParentBirthTo: dateAt(1110), ChildBirthFrom: dateAt(1130), ChildBirthTo: dateAt(1140)},
		{RelationshipID: "reference-2", ParentPersonID: "p2", ChildPersonID: "c2", ExclusionReason: "qualified", ParentBirthFrom: dateAt(1110), ParentBirthTo: dateAt(1120), ChildBirthFrom: dateAt(1140), ChildBirthTo: dateAt(1150)},
		{RelationshipID: "reference-3", ParentPersonID: "p3", ChildPersonID: "c3", ExclusionReason: "qualified", ParentBirthFrom: dateAt(1120), ParentBirthTo: dateAt(1130), ChildBirthFrom: dateAt(1150), ChildBirthTo: dateAt(1160)},
		{RelationshipID: "target-parent", ParentPersonID: "target", ChildPersonID: "c4", ExclusionReason: "qualified", ParentBirthFrom: dateAt(1000), ParentBirthTo: dateAt(1010), ChildBirthFrom: dateAt(1100), ChildBirthTo: dateAt(1110)},
		{RelationshipID: "target-child", ParentPersonID: "p4", ChildPersonID: "target", ExclusionReason: "qualified", ParentBirthFrom: dateAt(1020), ParentBirthTo: dateAt(1030), ChildBirthFrom: dateAt(1100), ChildBirthTo: dateAt(1110)},
	}
	result := analyzeGenerationEdges(scope, rows, "target", 3, false)
	if result.ReportStatus != ReportStatusSucceeded || len(result.Findings) != 2 || result.Reference.ReferenceEdgeCount != 3 || !result.Reference.TargetInPopulation {
		t.Fatalf("unexpected multi-target analysis: %+v", result)
	}
	truncated := analyzeGenerationEdges(scope, rows, "target", 3, true)
	if truncated.ReportStatus != ReportStatusInsufficient || len(truncated.Findings) != 0 || truncated.Reference.ReferenceBandAvailable {
		t.Fatalf("truncated analysis must not produce findings: %+v", truncated)
	}
}

func TestValidGenerationChronologyRejectsParentDeathBeforeChildBirth(t *testing.T) {
	if validGenerationChronology(time.Time{}, dateAt(1120), dateAt(1130)) {
		t.Fatal("expected parent death before child birth to be rejected")
	}
	if !validGenerationChronology(time.Time{}, dateAt(1140), dateAt(1130)) {
		t.Fatal("expected overlapping parent death and child birth to remain valid")
	}
}

func TestReferenceBandRequiresMinimumPopulation(t *testing.T) {
	if _, _, _, ok := referenceBand([]IntervalObservation{{MidpointYears: 20}}, 3); ok {
		t.Fatal("expected an insufficient reference population")
	}
}

func dateAt(year int) time.Time {
	return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
}
