package entityresolution

import (
	"errors"
	"time"
)

const (
	AlgorithmVersion     = "entity-resolution-v1"
	NormalizationVersion = "arabic-normalization-v1"

	// The run states, spelled once. RunQueued is the state a run is in from the
	// moment it is accepted until a worker has finished every stage of it; it is
	// the whole point of moving the work behind the queue, and it is a state a
	// client can safely render as "accepted, not finished".
	RunQueued    = "queued"
	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"

	// The stages a run passes through, in the order it passes through them. The
	// first checkpoint is committed before scoring starts so a stalled run says
	// which part it stalled in.
	StageQueued    = "queued"
	StageScoring   = "scoring"
	StageComplete  = "complete"
	StageRunFailed = "failed"
)

// Terminal reports whether a run state is one no worker will move again. A client
// polling a run stops on these, and a run that is not terminal is not a result.
func Terminal(status string) bool {
	return status == RunSucceeded || status == RunFailed
}

type EntityType string

const (
	EntityPerson EntityType = "person"
	EntityFamily EntityType = "family"
	EntityAll    EntityType = "all"
)

type MatchClass string

const (
	LikelyDifferent MatchClass = "likely_different"
	PossibleMatch   MatchClass = "possible_match"
	StrongCandidate MatchClass = "strong_candidate"
)

type ReviewStatus string

const (
	ReviewPending  ReviewStatus = "pending"
	ReviewApproved ReviewStatus = "approved"
	ReviewRejected ReviewStatus = "rejected"
	ReviewDeferred ReviewStatus = "deferred"
	ReviewReopened ReviewStatus = "reopened"
	ReviewMerged   ReviewStatus = "merged"
)

var (
	ErrDatabaseUnavailable = errors.New("entity resolution database is unavailable")
	ErrValidation          = errors.New("entity resolution input is invalid")
	ErrForbidden           = errors.New("entity resolution access is forbidden")
	ErrNotFound            = errors.New("entity resolution resource not found")
	ErrConflict            = errors.New("entity resolution resource changed")
	// ErrQueueUnavailable is what a service with no queue says. It is separate
	// from ErrDatabaseUnavailable because the two need different answers: one
	// means nothing is configured, the other means a run was asked for and there
	// is nobody who has agreed to finish it.
	ErrQueueUnavailable = errors.New("entity resolution queue is unavailable")
)

type RunInput struct {
	EntityType EntityType `json:"entity_type"`
}

type Run struct {
	ID                   string     `json:"id"`
	RequestedBy          string     `json:"requestedBy"`
	EntityType           EntityType `json:"entityType"`
	Status               string     `json:"status"`
	AlgorithmVersion     string     `json:"algorithmVersion"`
	NormalizationVersion string     `json:"normalizationVersion"`
	ModelVersion         string     `json:"modelVersion,omitempty"`
	CandidateCount       int        `json:"candidateCount"`
	Error                string     `json:"error,omitempty"`
	// JobID, Stage and the two timestamps are additive. Every key a client was
	// already reading is still there with the same meaning, and a run that is
	// queued reads as queued through the existing "status" rather than through a
	// new field.
	JobID       string     `json:"jobId,omitempty"`
	Stage       string     `json:"stage,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type Signal struct {
	Kind   string  `json:"kind"`
	Detail string  `json:"detail"`
	Score  float64 `json:"score"`
}

type Candidate struct {
	ID                  string             `json:"id"`
	RunID               string             `json:"runId"`
	EntityType          EntityType         `json:"entityType"`
	LeftEntityID        string             `json:"leftEntityId"`
	RightEntityID       string             `json:"rightEntityId"`
	LeftNameAR          string             `json:"leftNameAr"`
	RightNameAR         string             `json:"rightNameAr"`
	MatchClass          MatchClass         `json:"matchClass"`
	Score               float64            `json:"score"`
	ScoreComponents     map[string]float64 `json:"scoreComponents"`
	MatchingSignals     []Signal           `json:"matchingSignals"`
	ConflictingSignals  []Signal           `json:"conflictingSignals"`
	ExplanationAR       string             `json:"explanationAr"`
	ReviewStatus        ReviewStatus       `json:"reviewStatus"`
	CandidateVersion    int                `json:"candidateVersion"`
	RequiresHumanReview bool               `json:"requiresHumanReview"`
	ReviewedBy          string             `json:"reviewedBy,omitempty"`
	ReviewedAt          *time.Time         `json:"reviewedAt,omitempty"`
	ReviewNoteAR        string             `json:"reviewNoteAr,omitempty"`
	CreatedAt           time.Time          `json:"createdAt"`
	UpdatedAt           time.Time          `json:"updatedAt"`
}

type ReviewInput struct {
	Decision        string `json:"decision"`
	NoteAR          string `json:"note_ar"`
	ExpectedVersion int    `json:"expected_version"`
}

type MergeInput struct {
	SurvivorEntityID         string `json:"survivor_entity_id"`
	ReasonAR                 string `json:"reason_ar"`
	ExpectedCandidateVersion int    `json:"expected_candidate_version"`
	Confirm                  bool   `json:"confirm"`
}

type Merge struct {
	ID               string     `json:"id"`
	CandidateID      string     `json:"candidateId"`
	EntityType       EntityType `json:"entityType"`
	SurvivorID       string     `json:"survivorId"`
	MergedID         string     `json:"mergedId"`
	State            string     `json:"state"`
	RequestedBy      string     `json:"requestedBy"`
	ReasonAR         string     `json:"reasonAr"`
	AppliedAt        time.Time  `json:"appliedAt"`
	ReversedBy       string     `json:"reversedBy,omitempty"`
	ReversedAt       *time.Time `json:"reversedAt,omitempty"`
	ReversalReasonAR string     `json:"reversalReasonAr,omitempty"`
}
