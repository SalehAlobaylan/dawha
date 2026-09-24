package entityresolution

import (
	"errors"
	"time"
)

const (
	AlgorithmVersion     = "entity-resolution-v1"
	NormalizationVersion = "arabic-normalization-v1"
)

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
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
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
