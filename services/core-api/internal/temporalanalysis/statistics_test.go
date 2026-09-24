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

func TestReferenceBandRequiresMinimumPopulation(t *testing.T) {
	if _, _, _, ok := referenceBand([]IntervalObservation{{MidpointYears: 20}}, 3); ok {
		t.Fatal("expected an insufficient reference population")
	}
}

func dateAt(year int) time.Time {
	return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
}
