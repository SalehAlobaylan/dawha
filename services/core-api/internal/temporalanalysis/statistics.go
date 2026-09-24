package temporalanalysis

import (
	"math"
	"sort"
	"time"
)

func buildIntervalObservation(parentBirthFrom, parentBirthTo, childBirthFrom, childBirthTo time.Time) (IntervalObservation, bool) {
	if parentBirthFrom.IsZero() || parentBirthTo.IsZero() || childBirthFrom.IsZero() || childBirthTo.IsZero() || parentBirthFrom.After(parentBirthTo) || childBirthFrom.After(childBirthTo) || parentBirthFrom.After(childBirthTo) {
		return IntervalObservation{}, false
	}
	lower := yearsBetween(parentBirthTo, childBirthFrom)
	upper := yearsBetween(parentBirthFrom, childBirthTo)
	if upper < 0 {
		return IntervalObservation{}, false
	}
	if lower < 0 {
		lower = 0
	}
	return IntervalObservation{
		LowerYears:      lower,
		UpperYears:      upper,
		MidpointYears:   (lower + upper) / 2,
		ParentBirthFrom: parentBirthFrom.Format("2006-01-02"),
		ParentBirthTo:   parentBirthTo.Format("2006-01-02"),
		ChildBirthFrom:  childBirthFrom.Format("2006-01-02"),
		ChildBirthTo:    childBirthTo.Format("2006-01-02"),
	}, true
}

func validGenerationChronology(parentDeathFrom, parentDeathTo, childBirthFrom time.Time) bool {
	if !parentDeathFrom.IsZero() && !parentDeathTo.IsZero() && parentDeathFrom.After(parentDeathTo) {
		return false
	}
	if !parentDeathTo.IsZero() && !childBirthFrom.IsZero() && parentDeathTo.Before(childBirthFrom) {
		return false
	}
	return true
}

func yearsBetween(from, to time.Time) float64 {
	return to.Sub(from).Hours() / 24 / 365.2425
}

func referenceBand(reference []IntervalObservation, minimum int) (float64, float64, float64, bool) {
	if len(reference) < minimum {
		return 0, 0, 0, false
	}
	midpoints := make([]float64, 0, len(reference))
	for _, item := range reference {
		midpoints = append(midpoints, item.MidpointYears)
	}
	sort.Float64s(midpoints)
	return quantile(midpoints, 0.25), quantile(midpoints, 0.5), quantile(midpoints, 0.75), true
}

func quantile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}
	position := percentile * float64(len(values)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return values[lower]
	}
	weight := position - float64(lower)
	return values[lower] + (values[upper]-values[lower])*weight
}

func compareInterval(observed IntervalObservation, q1, median, q3 float64) Comparison {
	relation := FindingRelationOverlap
	if observed.UpperYears < q1 {
		relation = FindingRelationBelow
	} else if observed.LowerYears > q3 {
		relation = FindingRelationAbove
	}
	return Comparison{
		Method:      ComparisonMethodIQR,
		ReferenceN:  0,
		Q1Years:     q1,
		MedianYears: median,
		Q3Years:     q3,
		Observed:    observed,
		Relation:    relation,
	}
}
