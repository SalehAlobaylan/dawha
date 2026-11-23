package regionworkbench

import (
	"errors"
	"testing"
)

func TestCompareRetiresOnTwoAgreeingPasses(t *testing.T) {
	// The canonical retirement: A and B read the same candidate the same
	// way — accept with one reading — and the tile retires.
	candidates := []string{"c1", "c2", "c3"}
	first := Verdicts{
		"c1": Verdict{Action: ActionAccept, Reading: "عنزة"},
		"c2": Verdict{Action: ActionReject},
		"c3": Verdict{Action: ActionCorrect, Reading: "بني تميم"},
	}
	second := Verdicts{
		"c1": Verdict{Action: ActionCorrect, Reading: "عنزة"},
		"c2": Verdict{Action: ActionReject},
		"c3": Verdict{Action: ActionAccept, Reading: "بني تميم"},
	}
	agreement := Compare(candidates, "reviewer-a", "reviewer-b", first, second)
	if !agreement.Retires {
		t.Fatalf("agreement = %+v, want retirement", agreement)
	}
	if len(agreement.Confirmations) != 3 || len(agreement.Disagreements) != 0 ||
		len(agreement.Deferrals) != 0 || len(agreement.Unevaluated) != 0 {
		t.Fatalf("agreement detail = %+v", agreement)
	}
}

func TestCompareNeverRetiresOnOnePersonTwice(t *testing.T) {
	// The arithmetic itself refuses one reviewer counted twice, however well
	// the two passes agree — the claim gate makes this unreachable through
	// the service, and the arithmetic refuses it anyway.
	candidates := []string{"c1", "c2"}
	pass := Verdicts{
		"c1": Verdict{Action: ActionAccept, Reading: "شمّر"},
		"c2": Verdict{Action: ActionReject},
	}
	agreement := Compare(candidates, "reviewer-a", "reviewer-a", pass, pass)
	if agreement.Retires || !agreement.SelfReview {
		t.Fatalf("one person's pass twice = %+v, want SelfReview and no retirement", agreement)
	}
	// One pass alone is never a retirement either.
	if alone := Compare(candidates, "reviewer-a", "reviewer-b", pass, Verdicts{}); alone.Retires {
		t.Fatalf("a single pass retired: %+v", alone)
	}
}

func TestCompareNeverRetiresOnSharedDeferrals(t *testing.T) {
	// Two reviewers both unable to read a crop is honest, but a shared
	// inability is not a confirmation: the candidate stays deferred and the
	// tile returns to the queue naming it.
	candidates := []string{"c1"}
	unreadable := Verdicts{"c1": Verdict{Action: ActionDefer}}
	agreement := Compare(candidates, "reviewer-a", "reviewer-b", unreadable, unreadable)
	if agreement.Retires {
		t.Fatalf("shared deferral retired: %+v", agreement)
	}
	if len(agreement.Deferrals) != 1 || agreement.Deferrals[0] != "c1" {
		t.Fatalf("deferrals = %+v, want the candidate named", agreement.Deferrals)
	}
}

func TestCompareRefusesUnevaluatedAndMismatchedVerdicts(t *testing.T) {
	candidates := []string{"c1", "c2", "c3"}
	first := Verdicts{
		"c1": Verdict{Action: ActionAccept, Reading: "عنزة"},
		"c2": Verdict{Action: ActionReject},
		// c3 never answered.
	}
	second := Verdicts{
		"c1": Verdict{Action: ActionCorrect, Reading: "عنزة"},
		"c2": Verdict{Action: ActionReject},
		"c3": Verdict{Action: ActionReject},
	}
	agreement := Compare(candidates, "reviewer-a", "reviewer-b", first, second)
	if agreement.Retires {
		t.Fatalf("incomplete passes retired: %+v", agreement)
	}
	if len(agreement.Unevaluated) != 1 || agreement.Unevaluated[0] != "c3" {
		t.Fatalf("unevaluated = %+v, want c3", agreement.Unevaluated)
	}

	// A reading that differs by one word is a disagreement, not a
	// confirmation — the exact match is the whole point of two passes.
	mismatched := Verdicts{
		"c1": Verdict{Action: ActionCorrect, Reading: "عنزة بنو"},
		"c2": Verdict{Action: ActionReject},
		"c3": Verdict{Action: ActionReject},
	}
	fixed := Verdicts{
		"c1": Verdict{Action: ActionCorrect, Reading: "عنزة"},
		"c2": Verdict{Action: ActionReject},
		"c3": Verdict{Action: ActionReject},
	}
	if reconciled := Compare(candidates, "reviewer-a", "reviewer-b", mismatched, fixed); reconciled.Retires {
		t.Fatalf("different readings retired: %+v", reconciled)
	} else if len(reconciled.Disagreements) != 1 || reconciled.Disagreements[0] != "c1" {
		t.Fatalf("disagreements = %+v, want c1", reconciled.Disagreements)
	}

	// An empty tile has nothing to confirm: no technicality retirement.
	if empty := Compare(nil, "reviewer-a", "reviewer-b", Verdicts{}, Verdicts{}); empty.Retires {
		t.Fatalf("empty tile retired: %+v", empty)
	}
}

func TestIncompletePassErrorNamesTheWork(t *testing.T) {
	err := IncompletePassError{Missing: []string{"c1", "c2"}}
	if !errors.Is(err, IncompletePassError{}) && ErrIncompletePass.Error() == "" {
		t.Fatal("sentinel missing")
	}
	if err.Error() != "tile pass is incomplete: 2 candidates untouched" {
		t.Fatalf("error text = %q", err.Error())
	}
	if len(err.Missing) != 2 {
		t.Fatalf("missing = %v", err.Missing)
	}
}
