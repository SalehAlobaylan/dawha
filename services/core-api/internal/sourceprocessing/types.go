package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	SourceProcessJobType = "source_process"
	EmbeddingDimensions  = 1536
	MaxUploadBytes       = 25 << 20
	MaxPages             = 200
	MaxCandidates        = 5000
	MaxCandidatesPerPage = 100
)

var (
	ErrDatabaseUnavailable = errors.New("source processing database is unavailable")
	ErrStorageUnavailable  = errors.New("source processing storage is unavailable")
	ErrAIUnavailable       = errors.New("source processing AI service is unavailable")
	ErrQueueUnavailable    = errors.New("source processing queue is unavailable")
	ErrValidation          = errors.New("source processing input is invalid")
	ErrNotFound            = errors.New("source processing resource was not found")
	ErrForbidden           = errors.New("source processing access is forbidden")
	ErrConflict            = errors.New("source processing state conflict")
	ErrUnsupportedDocument = errors.New("source document format is not supported")
	// ErrLinkExpired and ErrLinkNotAuthorized are what a signed object URL says
	// when it is presented late or altered. They are separate errors because a
	// client can act on them differently - a retry for one, a re-request for the
	// other - and because collapsing them into "forbidden" would make a clock
	// problem look like an access-control problem in every log that records it.
	ErrLinkExpired       = errors.New("the signed link has expired")
	ErrLinkNotAuthorized = errors.New("the signed link is not valid")

	// ErrLeaseRequired is what a processor says when it was handed a job without
	// the claim that says the job is its own. Processing it anyway would be
	// processing it on the strength of a job id, which is a name rather than a
	// permission.
	ErrLeaseRequired = errors.New("source processing requires a job lease")
)

type JobPayload struct {
	SourceID     string `json:"source_id"`
	SourceFileID string `json:"source_file_id"`
}

// JobRequestIDFields reads the request id the enqueueing upload stamped into this
// job's payload.
//
// It is exported because the worker that claims the job is a different package,
// and the correlation between an upload and the extraction that failed on it is
// worth one exported function. It answers "" for a job enqueued before request
// ids existed, which is every job already sitting in the queue.
func JobRequestIDFields(payload []byte) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil
	}
	return decoded
}

type UploadInput struct {
	Filename    string
	ContentType string
	Content     []byte
}

type FileView struct {
	ID                 string     `json:"id"`
	SourceID           string     `json:"sourceId"`
	OriginalFilenameAR string     `json:"originalFilenameAr"`
	MimeType           string     `json:"mimeType"`
	ByteSize           int64      `json:"byteSize"`
	ChecksumSHA256     string     `json:"checksumSha256"`
	ProcessingStatus   string     `json:"processingStatus"`
	ProcessingError    string     `json:"processingError,omitempty"`
	ProcessedAt        *time.Time `json:"processedAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

type ProcessingRunView struct {
	ID             string     `json:"id"`
	SourceID       string     `json:"sourceId"`
	SourceFileID   string     `json:"sourceFileId"`
	JobID          string     `json:"jobId,omitempty"`
	Status         string     `json:"status"`
	Stage          string     `json:"stage"`
	PageCount      int        `json:"pageCount"`
	PassageCount   int        `json:"passageCount"`
	CandidateCount int        `json:"candidateCount"`
	ModelVersion   string     `json:"modelVersion,omitempty"`
	Error          string     `json:"error,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CandidateView struct {
	ID                     string                `json:"id"`
	SourceID               string                `json:"sourceId"`
	SourceFileID           string                `json:"sourceFileId"`
	SourcePassageID        string                `json:"sourcePassageId"`
	SourceStatementID      string                `json:"sourceStatementId,omitempty"`
	CandidateType          string                `json:"candidateType"`
	RawTextAR              string                `json:"rawTextAr"`
	NormalizedTextAR       string                `json:"normalizedTextAr"`
	SubjectTextAR          string                `json:"subjectTextAr,omitempty"`
	PredicateAR            string                `json:"predicateAr,omitempty"`
	ObjectTextAR           string                `json:"objectTextAr,omitempty"`
	ProposedEntityType     string                `json:"proposedEntityType,omitempty"`
	ProposedEntityID       string                `json:"proposedEntityId,omitempty"`
	ProposedEntityNameAR   string                `json:"proposedEntityNameAr,omitempty"`
	ProposedMatchScore     *float64              `json:"proposedMatchScore,omitempty"`
	ProposedMatchMatchedOn string                `json:"proposedMatchMatchedOn,omitempty"`
	SubjectEntityType      string                `json:"subjectEntityType,omitempty"`
	SubjectEntityID        string                `json:"subjectEntityId,omitempty"`
	ObjectEntityType       string                `json:"objectEntityType,omitempty"`
	ObjectEntityID         string                `json:"objectEntityId,omitempty"`
	Confidence             float64               `json:"confidence"`
	RationaleAR            string                `json:"rationaleAr"`
	PageNumber             *int                  `json:"pageNumber,omitempty"`
	PassageTextAR          string                `json:"passageTextAr"`
	LocatorAR              string                `json:"locatorAr,omitempty"`
	ModelVersion           string                `json:"modelVersion,omitempty"`
	Status                 string                `json:"status"`
	ReviewedBy             string                `json:"reviewedBy,omitempty"`
	ReviewedAt             *time.Time            `json:"reviewedAt,omitempty"`
	ReviewNoteAR           string                `json:"reviewNoteAr,omitempty"`
	AcceptedRecordType     string                `json:"acceptedRecordType,omitempty"`
	AcceptedRecordID       string                `json:"acceptedRecordId,omitempty"`
	CreatedAt              time.Time             `json:"createdAt"`
	UpdatedAt              time.Time             `json:"updatedAt"`
	Reviews                []CandidateReviewView `json:"reviews"`
}

type CandidateReviewView struct {
	ID         string    `json:"id"`
	ReviewerID string    `json:"reviewerId"`
	Decision   string    `json:"decision"`
	NoteAR     string    `json:"noteAr,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ProcessingView struct {
	SourceID string              `json:"sourceId"`
	Files    []FileView          `json:"files"`
	Runs     []ProcessingRunView `json:"runs"`
	// Candidates is the page asked for. With the default page it is every
	// candidate the source holds, up to MaxCandidates, which is what the
	// endpoint returned before it could page.
	Candidates []CandidateView `json:"candidates"`
	// CandidatesTotal is how many candidates the source holds, so a caller can
	// tell a complete list from a page without counting the array it received.
	CandidatesTotal int `json:"candidatesTotal"`
	// CandidatesTruncated reports that the page is shorter than the source, so a
	// reviewer reading the first page cannot mistake a bound for the whole set.
	CandidatesTruncated bool `json:"candidatesTruncated"`
}

// CandidatePage is one bounded page of the source's candidates. The zero value
// is the whole list, bounded by MaxCandidates.
type CandidatePage struct {
	// Limit is the maximum number of candidates to return. Zero means the whole
	// list. A limit above MaxCandidates is refused rather than clamped, so a
	// caller that asked for too much learns about it.
	Limit int
	// Offset is how many candidates to skip. The list is ordered by
	// (created_at, id), a total order, so an offset cannot return a row twice
	// or drop one between two pages.
	Offset int
}

type ReviewInput struct {
	Decision string `json:"decision"`
	NoteAR   string `json:"note_ar"`
}

type JobEnqueuer interface {
	Enqueue(context.Context, jobs.EnqueueInput) (jobs.EnqueueResult, error)
}

type TransactionalJobEnqueuer interface {
	EnqueueTx(context.Context, pgx.Tx, jobs.EnqueueInput) (jobs.EnqueueResult, error)
}

// LeaseQueue is the part of the queue a processor needs in order to be allowed
// to write. It is a separate interface from JobEnqueuer on purpose: enqueueing
// needs a name and a key, while processing needs proof of ownership, and a
// processor wired with only the first could still be handed work it may not
// commit.
type LeaseQueue interface {
	JobEnqueuer
	StartHeartbeat(context.Context, jobs.Lease) (*jobs.Heartbeat, context.Context, error)
	HoldLease(context.Context, jobs.Executor, jobs.Lease) error
}

type Extractor interface {
	Extract(context.Context, ExtractInput) ([]Page, error)
}

type ExtractInput struct {
	Reader      io.Reader
	ContentType string
	Filename    string
}

type Page struct {
	Number      int
	Text        string
	StartOffset int
	EndOffset   int
}

type Service struct {
	Pool  *pgxpool.Pool
	Store storage.Store
	// Jobs enqueues work and fences the writes that finish it. A processor
	// without it can accept uploads but cannot process them, which is the safe
	// way round: an upload is recoverable, a duplicate extraction is not.
	Jobs      LeaseQueue
	AI        ai.Provider
	Extractor Extractor
}

type processedEntity struct {
	Candidate ai.EntityCandidate
	Link      entityLink
}

type processedClaim struct {
	Candidate ai.ClaimCandidate
	Subject   entityLink
	Object    entityLink
}

type processedPage struct {
	Page       Page
	Normalized string
	Embedding  []float32
	Model      string
	Entities   []processedEntity
	Claims     []processedClaim
}

type entityLink struct {
	Type      string
	ID        string
	Name      string
	Score     float64
	MatchedOn string
}

type entityReference struct {
	Type    string
	ID      string
	Name    string
	Aliases []string
}
