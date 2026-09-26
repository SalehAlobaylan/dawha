// Package analysisworker is the queue consumer for the two analyses that used to
// run inside HTTP requests.
//
// Entity resolution scores every person and family against every other one, and a
// research agent investigation walks eleven stages of evidence gathering. Both
// used to happen in the request thread, so a slow database or a slow model could
// outlive the request and hold its connection for the duration. They are the same
// kind of work - a run, a plan, stages, a report - so one process drains both,
// and the queue is what stops the two from being coupled to the API's lifetime.
package analysisworker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/entityresolution"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/researchagent"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler serves both job types. It is one type rather than two because the two
// analyses share the thing that matters: neither of them may write anything the
// worker holding the job is not entitled to write, and that rule is enforced by
// the lease rather than by either analysis.
type Handler struct {
	Resolution *entityresolution.Service
	Agent      *researchagent.Service
	// Logger is where a stage-level refusal is reported. Nil means no logging, so
	// a test can run the handler without arranging a logger.
	Logger *log.Logger
}

var _ interface {
	JobTypes() []string
	Handle(context.Context, jobs.Lease, jobs.JobView) error
	RequeueRecovered(context.Context, []jobs.JobView) error
} = (*Handler)(nil)

func (h *Handler) JobTypes() []string {
	return []string{entityresolution.JobType, researchagent.JobType}
}

// Handle dispatches on the job type. An unknown type is a validation failure
// rather than a shrug: the worker only claims types it registered, so an unknown
// one means the queue and the worker disagree, and silently doing nothing would
// leave a job marked succeeded with no work behind it.
func (h *Handler) Handle(ctx context.Context, lease jobs.Lease, job jobs.JobView) error {
	runID, err := RunIDFromPayload(job)
	if err != nil {
		return err
	}
	switch job.Type {
	case entityresolution.JobType:
		if h.Resolution == nil {
			return entityresolution.ErrQueueUnavailable
		}
		return h.Resolution.ProcessRun(ctx, lease, runID)
	case researchagent.JobType:
		if h.Agent == nil {
			return researchagent.ErrQueueUnavailable
		}
		return h.Agent.ProcessRun(ctx, lease, runID)
	default:
		return errors.New("the analysis worker was handed a job type it does not serve")
	}
}

func (h *Handler) RequeueRecovered(ctx context.Context, recovered []jobs.JobView) error {
	// Both resets are type-filtered inside their own service, so a worker that
	// recovers only its own types cannot disturb a run belonging to another.
	if h.Resolution != nil {
		if err := h.Resolution.RequeueRecovered(ctx, recovered); err != nil {
			return err
		}
	}
	if h.Agent != nil {
		if err := h.Agent.RequeueRecovered(ctx, recovered); err != nil {
			return err
		}
	}
	return nil
}

// RunIDFromPayload reads the run a job is for.
//
// A payload it cannot read is a validation failure and not a not-found: the job
// exists and is claimed, so failing it is correct, and failing it with "the
// payload is unreadable" is a sentence an operator can act on. Guessing a run
// would be worse - it would process whichever run happened to be first.
func RunIDFromPayload(job jobs.JobView) (string, error) {
	var payload struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return "", errors.New("the job payload is not a run reference")
	}
	if _, err := uuid.Parse(strings.TrimSpace(payload.RunID)); err != nil {
		return "", errors.New("the job payload does not name a run")
	}
	return payload.RunID, nil
}

// New builds the handler from the pieces a consumer binary has.
func New(pool *pgxpool.Pool, queue *jobs.Service, provider ai.Provider, logger *log.Logger) (*Handler, error) {
	if pool == nil || queue == nil {
		return nil, errors.New("the analysis worker needs a database and a queue")
	}
	return &Handler{
		Resolution: entityresolution.NewService(pool, provider).WithQueue(queue),
		Agent:      researchagent.NewService(pool).WithQueue(queue),
		Logger:     logger,
	}, nil
}
