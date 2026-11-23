// Two-person retirement arithmetic for the tile lane (plan 030-F).
//
// A tile retires when two passes by DIFFERENT reviewers answer every proposal
// inside it the same way. The rule is stipulation 2 of plan 030, restricted to
// people because a tile's proposals all share one source: two candidates from
// the same source are one confirmation, so source-independence is unavailable
// inside a tile by construction, and person-independence is what the lane
// enforces until the plan-034 reuse graph refines it.
//
// The pass shape is deliberately small: per candidate, the reviewer's action
// and (for a read) the reading they stood behind. Equality is exact — a
// reading that differs by one letter is a disagreement, not a confirmation.
package regionworkbench

// Verdict is one reviewer's answer to one candidate inside a tile, as decided
// through the candidate lane (`regionreview.Decide`) and derived from the
// decision history — never re-typed by the workbench.
type Verdict struct {
	Action  string `json:"action"` // accept | correct | reject | defer
	Reading string `json:"reading,omitempty"`
	Era     string `json:"era,omitempty"`
}

// The candidate lane's decide actions, mirrored here for the comparison's
// closed vocabulary.
const (
	ActionAccept  = "accept"
	ActionCorrect = "correct"
	ActionReject  = "reject"
	ActionDefer   = "defer"
)

// Verdicts maps candidate id → the reviewer's verdict for it.
type Verdicts map[string]Verdict

// Agreement is what the comparison of two passes produced. A tile retires
// only when Confirms is complete (every candidate present) and Deferrals and
// Disagreements are both empty. It rides the submit response, so its JSON
// shape is part of the workbench's transport contract.
type Agreement struct {
	// Retires is true when the two passes confirm each other on every
	// candidate: an exact reading match for reads (accept/correct are the same
	// "I read this" class — the distinction is which button was pressed, not
	// what was said), or a double rejection.
	Retires bool `json:"retires"`
	// SelfReview is set when the two passes carry the same submitter: one
	// person counted twice is never a retirement, whatever the verdicts say.
	SelfReview bool `json:"selfReview"`
	// Confirmations are the candidate ids the two passes agree on.
	Confirmations []string `json:"confirmations"`
	// Deferrals are candidates both passes deferred: a shared inability to
	// read is honest but is not a confirmation. The tile returns to the queue
	// and names them, so the deferred work returns via the plan-026 pattern
	// instead of retiring silently.
	Deferrals []string `json:"deferrals"`
	// Disagreements are the candidate ids the passes answered differently —
	// different actions, or different readings for the same action class.
	Disagreements []string `json:"disagreements"`
	// Unevaluated are candidates neither (or only one) pass answered. A pass
	// that skipped a candidate is incomplete, and a submitted pass is
	// validated at submit time for completeness — so a non-empty Unevaluated
	// here names a real hole the rework has to close.
	Unevaluated []string `json:"unevaluated"`
}

func isRead(action string) bool {
	return action == "accept" || action == "correct"
}

// Compare evaluates retirement. candidateIDs is the tile's full candidate
// membership; first and second are the two submitters' verdict sets, with
// their submitter ids — when the ids match, the arithmetic itself refuses:
// one person counted twice is never a two-person confirmation.
func Compare(candidateIDs []string, firstSubmitter, secondSubmitter string, first, second Verdicts) Agreement {
	agreement := Agreement{}
	if firstSubmitter != "" && firstSubmitter == secondSubmitter {
		agreement.SelfReview = true
		agreement.Retires = false
		agreement.Confirmations = []string{}
		agreement.Deferrals = []string{}
		agreement.Disagreements = []string{}
		agreement.Unevaluated = []string{}
		return agreement
	}
	agreement.Retires = true
	for _, id := range candidateIDs {
		firstVerdict, firstOK := first[id]
		secondVerdict, secondOK := second[id]
		if !firstOK || !secondOK {
			agreement.Unevaluated = append(agreement.Unevaluated, id)
			agreement.Retires = false
			continue
		}
		switch {
		case firstVerdict.Action == ActionDefer && secondVerdict.Action == ActionDefer:
			agreement.Deferrals = append(agreement.Deferrals, id)
			agreement.Retires = false
		case isRead(firstVerdict.Action) && isRead(secondVerdict.Action):
			if firstVerdict.Reading == secondVerdict.Reading && firstVerdict.Reading != "" {
				agreement.Confirmations = append(agreement.Confirmations, id)
			} else {
				agreement.Disagreements = append(agreement.Disagreements, id)
				agreement.Retires = false
			}
		case firstVerdict.Action == ActionReject && secondVerdict.Action == ActionReject:
			agreement.Confirmations = append(agreement.Confirmations, id)
		default:
			agreement.Disagreements = append(agreement.Disagreements, id)
			agreement.Retires = false
		}
	}
	if len(candidateIDs) == 0 {
		// An empty tile would retire on a technicality; there is nothing in it
		// to confirm, so it does not retire.
		agreement.Retires = false
	}
	if agreement.Confirmations == nil {
		agreement.Confirmations = []string{}
	}
	if agreement.Deferrals == nil {
		agreement.Deferrals = []string{}
	}
	if agreement.Disagreements == nil {
		agreement.Disagreements = []string{}
	}
	if agreement.Unevaluated == nil {
		agreement.Unevaluated = []string{}
	}
	return agreement
}
