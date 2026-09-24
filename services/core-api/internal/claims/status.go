package claims

import "fmt"

type Status string

const (
	Documented        Status = "documented"
	Supported         Status = "supported"
	Contested         Status = "contested"
	Disputed          Status = "disputed"
	Inferred          Status = "inferred"
	PlatformGenerated Status = "platform_generated"
	Unresolved        Status = "unresolved"
	Contradicted      Status = "contradicted"
	Rejected          Status = "rejected"
	Superseded        Status = "superseded"
	Unknown           Status = "unknown"
)

var validStatuses = map[Status]struct{}{
	Documented: {}, Supported: {}, Contested: {}, Disputed: {}, Inferred: {}, PlatformGenerated: {}, Unresolved: {}, Contradicted: {}, Rejected: {}, Superseded: {}, Unknown: {},
}

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	if _, ok := validStatuses[status]; !ok {
		return "", fmt.Errorf("unknown claim status %q", value)
	}
	return status, nil
}

func CanTransition(from, to Status, reviewed bool) bool {
	if _, ok := validStatuses[from]; !ok {
		return false
	}
	if _, ok := validStatuses[to]; !ok {
		return false
	}
	if from == to {
		return true
	}
	if from == PlatformGenerated && (to == Documented || to == Supported) {
		return reviewed
	}
	if from == Rejected || from == Superseded {
		return to == Unknown
	}
	if to == Rejected || to == Superseded {
		return reviewed
	}
	if from == Unresolved || from == Unknown {
		return to != Documented || reviewed
	}
	return true
}
