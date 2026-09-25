package geospatialintelligence

import (
	"errors"
	"time"
)

const (
	AlgorithmVersion           = "geospatial-intelligence-v1"
	QualificationPolicyVersion = "qualified-geography-v1"
	DefaultRadiusKM            = 250.0
	MaximumRadiusKM            = 2000.0
	MaximumRecords             = 5000
	MaximumReportItems         = 200
	ReportStatusSucceeded      = "succeeded"
	ReportStatusInsufficient   = "insufficient_evidence"
	ReportStatusFailed         = "failed"
	FindingTypeConflict        = "geographic_association_conflict"
	FindingTypeSourceConflict  = "source_geography_conflict"
)

var (
	ErrDatabaseUnavailable = errors.New("geospatial intelligence database is unavailable")
	ErrValidation          = errors.New("geospatial intelligence input is invalid")
	ErrForbidden           = errors.New("geospatial intelligence access is forbidden")
	ErrNotFound            = errors.New("geospatial intelligence resource was not found")
	ErrConflict            = errors.New("geospatial intelligence resource changed")
)

type RunInput struct {
	EntityType     string  `json:"entity_type"`
	EntityID       string  `json:"entity_id"`
	QuestionID     string  `json:"question_id"`
	TreeID         string  `json:"tree_id"`
	TreeVersionID  string  `json:"tree_version_id"`
	RadiusKM       float64 `json:"radius_km"`
	MaximumRecords int     `json:"maximum_records"`
}

type Run struct {
	ID                         string     `json:"id"`
	RequestedBy                string     `json:"requestedBy"`
	QuestionID                 string     `json:"questionId,omitempty"`
	EntityType                 string     `json:"entityType"`
	EntityID                   string     `json:"entityId"`
	EntityName                 string     `json:"entityName"`
	TreeID                     string     `json:"treeId,omitempty"`
	TreeVersionID              string     `json:"treeVersionId,omitempty"`
	VersionNumber              int        `json:"versionNumber,omitempty"`
	VersionState               string     `json:"versionState,omitempty"`
	TreeVisibility             string     `json:"treeVisibility,omitempty"`
	Status                     string     `json:"status"`
	ReportStatus               string     `json:"reportStatus"`
	ExecutionMode              string     `json:"executionMode"`
	AlgorithmVersion           string     `json:"algorithmVersion"`
	QualificationPolicyVersion string     `json:"qualificationPolicyVersion"`
	Scope                      Scope      `json:"scope"`
	Report                     Report     `json:"report"`
	FindingCount               int        `json:"findingCount"`
	Error                      string     `json:"error,omitempty"`
	CreatedAt                  time.Time  `json:"createdAt"`
	StartedAt                  *time.Time `json:"startedAt,omitempty"`
	CompletedAt                *time.Time `json:"completedAt,omitempty"`
	UpdatedAt                  time.Time  `json:"updatedAt"`
}

type Scope struct {
	EntityType         string   `json:"entityType"`
	EntityID           string   `json:"entityId"`
	EntityName         string   `json:"entityName"`
	TreeID             string   `json:"treeId,omitempty"`
	TreeVersionID      string   `json:"treeVersionId,omitempty"`
	VersionNumber      int      `json:"versionNumber,omitempty"`
	VersionState       string   `json:"versionState,omitempty"`
	TreeVisibility     string   `json:"treeVisibility,omitempty"`
	RadiusKM           float64  `json:"radiusKm"`
	MaximumRecords     int      `json:"maximumRecords"`
	SourceIDs          []string `json:"sourceIds"`
	QualificationNotes []string `json:"qualificationNotes"`
}

type Report struct {
	PlaceResolution          PlaceResolution           `json:"placeResolution"`
	Disambiguation           []DisambiguationCandidate `json:"disambiguation"`
	Clusters                 []SpatialCluster          `json:"clusters"`
	MigrationHypotheses      []MigrationHypothesis     `json:"migrationHypotheses"`
	GeographicContradictions []GeographicContradiction `json:"geographicContradictions"`
	SourceGeography          []SourceGeography         `json:"sourceGeography"`
	Limitations              []string                  `json:"limitations"`
}

type PlaceResolution struct {
	ResolvedCount   int            `json:"resolvedCount"`
	UnresolvedCount int            `json:"unresolvedCount"`
	MentionCount    int            `json:"mentionCount"`
	Mentions        []PlaceMention `json:"mentions"`
}

type PlaceMention struct {
	ID             string   `json:"id"`
	Mention        string   `json:"mention"`
	NormalizedName string   `json:"normalizedName"`
	SourceID       string   `json:"sourceId"`
	SourceTitle    string   `json:"sourceTitle,omitempty"`
	StatementID    string   `json:"statementId,omitempty"`
	PassageID      string   `json:"passageId,omitempty"`
	PlaceID        string   `json:"placeId,omitempty"`
	PlaceName      string   `json:"placeName,omitempty"`
	Resolution     string   `json:"resolution"`
	Reason         string   `json:"reason"`
	CandidateIDs   []string `json:"candidateIds,omitempty"`
	SourceLayer    string   `json:"sourceLayer"`
	HistoricalName bool     `json:"historicalName"`
	ValidFrom      string   `json:"validFrom,omitempty"`
	ValidTo        string   `json:"validTo,omitempty"`
}

type DisambiguationCandidate struct {
	ID          string           `json:"id"`
	Mention     string           `json:"mention"`
	SourceID    string           `json:"sourceId"`
	StatementID string           `json:"statementId,omitempty"`
	Status      string           `json:"status"`
	Reason      string           `json:"reason"`
	Candidates  []PlaceCandidate `json:"candidates"`
}

type PlaceCandidate struct {
	PlaceID   string `json:"placeId"`
	PlaceName string `json:"placeName"`
	PlaceType string `json:"placeType"`
	ValidFrom string `json:"validFrom,omitempty"`
	ValidTo   string `json:"validTo,omitempty"`
}

type SpatialCluster struct {
	ID                string   `json:"id"`
	Layer             string   `json:"layer"`
	Status            string   `json:"status"`
	PlaceIDs          []string `json:"placeIds"`
	PlaceNames        []string `json:"placeNames"`
	AssociationIDs    []string `json:"associationIds"`
	EntityIDs         []string `json:"entityIds"`
	SourceIDs         []string `json:"sourceIds"`
	EvidenceIDs       []string `json:"evidenceIds"`
	CenterLatitude    float64  `json:"centerLatitude"`
	CenterLongitude   float64  `json:"centerLongitude"`
	RadiusKM          float64  `json:"radiusKm"`
	SourceBackedCount int      `json:"sourceBackedCount"`
	InferredCount     int      `json:"inferredCount"`
	ExplanationAR     string   `json:"explanationAr"`
}

type PlaceReference struct {
	PlaceID   string `json:"placeId"`
	PlaceName string `json:"placeName"`
	Layer     string `json:"layer"`
}

type MigrationHypothesis struct {
	ID             string           `json:"id"`
	SubjectType    string           `json:"subjectType"`
	SubjectID      string           `json:"subjectId"`
	SubjectName    string           `json:"subjectName"`
	Sequence       []PlaceReference `json:"sequence"`
	SourceIDs      []string         `json:"sourceIds"`
	EvidenceIDs    []string         `json:"evidenceIds"`
	ClaimIDs       []string         `json:"claimIds"`
	AssociationIDs []string         `json:"associationIds"`
	TreeVersionID  string           `json:"treeVersionId,omitempty"`
	Layer          string           `json:"layer"`
	Status         string           `json:"status"`
	ExplanationAR  string           `json:"explanationAr"`
}

type GeographicContradiction struct {
	ID            string   `json:"id"`
	FindingID     string   `json:"findingId,omitempty"`
	Type          string   `json:"type"`
	Layer         string   `json:"layer"`
	Status        string   `json:"status"`
	Severity      string   `json:"severity"`
	TitleAR       string   `json:"titleAr"`
	ExplanationAR string   `json:"explanationAr"`
	EntityIDs     []string `json:"entityIds"`
	PlaceIDs      []string `json:"placeIds"`
	SourceIDs     []string `json:"sourceIds"`
	ClaimIDs      []string `json:"claimIds"`
	ReviewNoteAR  string   `json:"reviewNoteAr,omitempty"`
}

type SourceGeography struct {
	SourceID          string   `json:"sourceId"`
	SourceTitle       string   `json:"sourceTitle"`
	StatementCount    int      `json:"statementCount"`
	MatchedPlaceIDs   []string `json:"matchedPlaceIds"`
	PlaceNames        []string `json:"placeNames"`
	ResolvedMentions  int      `json:"resolvedMentions"`
	UnresolvedCount   int      `json:"unresolvedCount"`
	DependencyStatus  string   `json:"dependencyStatus"`
	SourceLayer       string   `json:"sourceLayer"`
	QualificationNote string   `json:"qualificationNote"`
}

type ReviewInput struct {
	Decision        string `json:"decision"`
	NoteAR          string `json:"note_ar"`
	CreateQuestion  bool   `json:"create_question"`
	QuestionTitleAR string `json:"question_title_ar"`
}

type Review struct {
	ID         string    `json:"id"`
	ReviewerID string    `json:"reviewerId"`
	Decision   string    `json:"decision"`
	NoteAR     string    `json:"noteAr,omitempty"`
	QuestionID string    `json:"questionId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Finding struct {
	ID                         string         `json:"id"`
	RunID                      string         `json:"runId"`
	GeospatialRunID            string         `json:"geospatialRunId"`
	FindingType                string         `json:"findingType"`
	TitleAR                    string         `json:"titleAr"`
	ExplanationAR              string         `json:"explanationAr"`
	Status                     string         `json:"status"`
	Severity                   string         `json:"severity"`
	Layer                      string         `json:"layer"`
	Signals                    map[string]any `json:"signals"`
	EntityIDs                  []string       `json:"entityIds"`
	PlaceIDs                   []string       `json:"placeIds"`
	SourceIDs                  []string       `json:"sourceIds"`
	ClaimIDs                   []string       `json:"claimIds"`
	AlgorithmVersion           string         `json:"algorithmVersion"`
	QualificationPolicyVersion string         `json:"qualificationPolicyVersion"`
	QuestionID                 string         `json:"questionId,omitempty"`
	CreatedBy                  string         `json:"createdBy,omitempty"`
	ReviewedBy                 string         `json:"reviewedBy,omitempty"`
	ReviewedAt                 *time.Time     `json:"reviewedAt,omitempty"`
	ReviewNoteAR               string         `json:"reviewNoteAr,omitempty"`
	CreatedAt                  time.Time      `json:"createdAt"`
	UpdatedAt                  time.Time      `json:"updatedAt"`
	Reviews                    []Review       `json:"reviews"`
}
