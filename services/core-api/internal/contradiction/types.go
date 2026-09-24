package contradiction

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	JobType          = "contradiction_scan"
	AlgorithmVersion = "contradiction-detection-v1"
)

var (
	ErrDatabaseUnavailable = errors.New("contradiction database is unavailable")
	ErrValidation          = errors.New("contradiction input is invalid")
	ErrForbidden           = errors.New("contradiction access is forbidden")
	ErrNotFound            = errors.New("contradiction resource not found")
	ErrConflict            = errors.New("contradiction resource changed")
	ErrQueueUnavailable    = errors.New("contradiction queue is unavailable")
)

type Run struct {
	ID               string     `json:"id"`
	RequestedBy      string     `json:"requestedBy"`
	JobID            string     `json:"jobId,omitempty"`
	Status           string     `json:"status"`
	AlgorithmVersion string     `json:"algorithmVersion"`
	FindingCount     int        `json:"findingCount"`
	Error            string     `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type Finding struct {
	ID               string         `json:"id"`
	RunID            string         `json:"runId,omitempty"`
	FindingType      string         `json:"findingType"`
	TitleAR          string         `json:"titleAr"`
	ExplanationAR    string         `json:"explanationAr"`
	Status           string         `json:"status"`
	Severity         string         `json:"severity"`
	Signals          map[string]any `json:"signals"`
	ClaimIDs         []string       `json:"claimIds"`
	EntityIDs        []string       `json:"entityIds"`
	AlgorithmVersion string         `json:"algorithmVersion"`
	ModelVersion     string         `json:"modelVersion,omitempty"`
	CreatedBy        string         `json:"createdBy,omitempty"`
	ReviewedBy       string         `json:"reviewedBy,omitempty"`
	ReviewedAt       *time.Time     `json:"reviewedAt,omitempty"`
	ReviewNoteAR     string         `json:"reviewNoteAr,omitempty"`
	QuestionID       string         `json:"questionId,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	Reviews          []Review       `json:"reviews"`
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

type jobPayload struct {
	RunID string `json:"run_id"`
}

type findingInput struct {
	Key           string
	Type          string
	TitleAR       string
	ExplanationAR string
	Severity      string
	Signals       map[string]any
	ClaimIDs      []string
	EntityIDs     []string
}

func encodeJobPayload(runID string) json.RawMessage {
	encoded, _ := json.Marshal(jobPayload{RunID: runID})
	return encoded
}
