package temporalanalysis

import (
	"errors"
	"time"
)

const (
	AlgorithmVersion           = "temporal-statistics-v1"
	QualificationPolicyVersion = "qualified-v1"
	MetricGenerationInterval   = "generation_interval_years"
	ComparisonMethodIQR        = "empirical_iqr_midpoint_v1"
	DefaultMinReferenceSize    = 20
	MaximumReferenceEdges      = 5000
	MaximumFindings            = 200
	ReportStatusSucceeded      = "succeeded"
	ReportStatusInsufficient   = "insufficient_reference"
	ReportStatusFailed         = "failed"
	FindingTypeGeneration      = "generation_interval_outside_reference_range"
	FindingRelationBelow       = "below_reference_range"
	FindingRelationAbove       = "above_reference_range"
	FindingRelationOverlap     = "overlaps_reference_range"
)

var (
	ErrDatabaseUnavailable = errors.New("temporal analysis database is unavailable")
	ErrValidation          = errors.New("temporal analysis input is invalid")
	ErrForbidden           = errors.New("temporal analysis access is forbidden")
	ErrNotFound            = errors.New("temporal analysis resource was not found")
	ErrConflict            = errors.New("temporal analysis resource changed")
)

type StartRunInput struct {
	TreeID           string `json:"tree_id"`
	TreeVersionID    string `json:"tree_version_id"`
	TargetPersonID   string `json:"target_person_id"`
	QuestionID       string `json:"question_id"`
	MinReferenceSize int    `json:"min_reference_size"`
}

type Run struct {
	ID                         string              `json:"id"`
	RequestedBy                string              `json:"requestedBy"`
	QuestionID                 string              `json:"questionId,omitempty"`
	TreeID                     string              `json:"treeId"`
	TreeVersionID              string              `json:"treeVersionId"`
	TargetPersonID             string              `json:"targetPersonId"`
	Status                     string              `json:"status"`
	ReportStatus               string              `json:"reportStatus"`
	ExecutionMode              string              `json:"executionMode"`
	AlgorithmVersion           string              `json:"algorithmVersion"`
	QualificationPolicyVersion string              `json:"qualificationPolicyVersion"`
	MinReferenceSize           int                 `json:"minReferenceSize"`
	ReferencePopulation        ReferencePopulation `json:"referencePopulation"`
	FindingCount               int                 `json:"findingCount"`
	Error                      string              `json:"error,omitempty"`
	CreatedAt                  time.Time           `json:"createdAt"`
	StartedAt                  *time.Time          `json:"startedAt,omitempty"`
	CompletedAt                *time.Time          `json:"completedAt,omitempty"`
	UpdatedAt                  time.Time           `json:"updatedAt"`
}

type ReferencePopulation struct {
	ScopeType              string         `json:"scopeType"`
	TreeID                 string         `json:"treeId"`
	TreeVersionID          string         `json:"treeVersionId"`
	VersionNumber          int            `json:"versionNumber"`
	VersionState           string         `json:"versionState"`
	ReferenceEdgeCount     int            `json:"referenceEdgeCount"`
	CandidateEdgeCount     int            `json:"candidateEdgeCount"`
	ExcludedCounts         map[string]int `json:"excludedCounts"`
	DatePolicy             string         `json:"datePolicy"`
	DependencyPolicy       string         `json:"dependencyPolicy"`
	SourcePolicy           string         `json:"sourcePolicy"`
	ClaimPolicy            string         `json:"claimPolicy"`
	TreePolicy             string         `json:"treePolicy"`
	TargetInPopulation     bool           `json:"targetInPopulation"`
	Truncated              bool           `json:"truncated"`
	ReferenceBandAvailable bool           `json:"referenceBandAvailable"`
	Q1Years                float64        `json:"q1Years"`
	MedianYears            float64        `json:"medianYears"`
	Q3Years                float64        `json:"q3Years"`
}

type IntervalObservation struct {
	LowerYears      float64 `json:"lowerYears"`
	UpperYears      float64 `json:"upperYears"`
	MidpointYears   float64 `json:"midpointYears"`
	ParentBirthFrom string  `json:"parentBirthFrom"`
	ParentBirthTo   string  `json:"parentBirthTo"`
	ChildBirthFrom  string  `json:"childBirthFrom"`
	ChildBirthTo    string  `json:"childBirthTo"`
}

type Comparison struct {
	Method      string              `json:"method"`
	ReferenceN  int                 `json:"referenceN"`
	Q1Years     float64             `json:"q1Years"`
	MedianYears float64             `json:"medianYears"`
	Q3Years     float64             `json:"q3Years"`
	Observed    IntervalObservation `json:"observed"`
	Relation    string              `json:"relation"`
}

type Finding struct {
	ID                         string              `json:"id"`
	RunID                      string              `json:"runId"`
	TemporalRunID              string              `json:"temporalRunId"`
	FindingType                string              `json:"findingType"`
	TitleAR                    string              `json:"titleAr"`
	ExplanationAR              string              `json:"explanationAr"`
	Status                     string              `json:"status"`
	Severity                   string              `json:"severity"`
	Signals                    map[string]any      `json:"signals"`
	ClaimIDs                   []string            `json:"claimIds"`
	EntityIDs                  []string            `json:"entityIds"`
	ReferencePopulation        ReferencePopulation `json:"referencePopulation"`
	Comparison                 Comparison          `json:"comparison"`
	AlgorithmVersion           string              `json:"algorithmVersion"`
	QualificationPolicyVersion string              `json:"qualificationPolicyVersion"`
	CreatedBy                  string              `json:"createdBy,omitempty"`
	ReviewedBy                 string              `json:"reviewedBy,omitempty"`
	ReviewedAt                 *time.Time          `json:"reviewedAt,omitempty"`
	ReviewNoteAR               string              `json:"reviewNoteAr,omitempty"`
	QuestionID                 string              `json:"questionId,omitempty"`
	CreatedAt                  time.Time           `json:"createdAt"`
	UpdatedAt                  time.Time           `json:"updatedAt"`
	Reviews                    []Review            `json:"reviews"`
}

type Review struct {
	ID         string    `json:"id"`
	ReviewerID string    `json:"reviewerId"`
	Decision   string    `json:"decision"`
	NoteAR     string    `json:"noteAr,omitempty"`
	QuestionID string    `json:"questionId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ReviewInput struct {
	Decision        string `json:"decision"`
	NoteAR          string `json:"note_ar"`
	CreateQuestion  bool   `json:"create_question"`
	QuestionTitleAR string `json:"question_title_ar"`
}
