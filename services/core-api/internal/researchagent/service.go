package researchagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Service struct {
	Pool *pgxpool.Pool
	// Jobs is what turns an investigation into queued work. A service without it
	// can still read runs, but it cannot accept one, and refusing to accept is
	// better than accepting an investigation nobody has agreed to finish.
	Jobs *jobs.Service
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

// WithQueue is the constructor the API uses. The queue is a separate call rather
// than a constructor argument so a caller that genuinely only wants to read runs
// has to say so, instead of passing nil and finding out later.
func (s *Service) WithQueue(queue *jobs.Service) *Service {
	s.Jobs = queue
	return s
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) readyToQueue() error {
	if err := s.ready(); err != nil {
		return err
	}
	if s.Jobs == nil {
		return ErrQueueUnavailable
	}
	return nil
}

// StartRun accepts an investigation and hands it to the queue.
//
// This is the whole of the request thread now. The permission checks, the scope
// resolution and the question lookup all happen here, because they are the
// answers a caller can be given immediately and because they are the checks that
// must be settled before anything is written. Everything slow - decomposing the
// question, searching sources, walking the graph, gathering counter-evidence -
// happens in a worker, against a lease, where a stage can fail and be resumed
// instead of taking the request down with it.
//
// The run row and the job row are written in one transaction. A run can therefore
// never be accepted with nothing queued behind it, and a job can never exist for
// a run that was refused.
func (s *Service) StartRun(ctx context.Context, actorID string, input RunInput) (Run, error) {
	if err := s.readyToQueue(); err != nil {
		return Run{}, err
	}
	input, err := validateInput(input)
	if err != nil {
		return Run{}, err
	}
	if input.EntityType == "source" && input.SourceID == "" {
		input.SourceID = input.EntityID
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return Run{}, ErrForbidden
	}
	// Authorization is decided before anything is read about the target, because
	// resolving the scope answers questions an unauthorized caller has no business
	// asking: whether a person exists, whether a tree is published, whether a source
	// is public. Refusing first means the refusal does not carry those answers.
	if err := requireRole(ctx, s.Pool, actorUUID); err != nil {
		return Run{}, err
	}
	// The scope is resolved before the transaction opens, because it is four reads
	// against rows the run does not own, and holding a transaction open across them
	// would pin a connection for the length of a slow lookup.
	scope, err := loadScope(ctx, s.Pool, input, actorUUID)
	if err != nil {
		return Run{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	// And decided again inside the transaction, before the first write. The check
	// above establishes the order of the refusals; this one establishes that the run
	// row is not written by an actor whose role was revoked between the two reads,
	// which is the guarantee the plan's authorization rule is actually about.
	if err := requireRole(ctx, tx, actorUUID); err != nil {
		return Run{}, err
	}
	questionUUID, err := optionalUUID(input.QuestionID)
	if err != nil {
		return Run{}, ErrValidation
	}
	if questionUUID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM open_questions WHERE id = $1)`, questionUUID).Scan(&exists); err != nil {
			return Run{}, err
		}
		if !exists {
			return Run{}, ErrNotFound
		}
	}
	runID := uuid.New()
	// The scope goes on the row, not just into this process's memory. A run that is
	// resumed is resumed by a worker that never saw the request, and the source, the
	// place and the year range the investigation was asked about are not derivable
	// from the question text.
	runScope := Scope{EntityType: input.EntityType, EntityID: input.EntityID, TreeID: scope.TreeID, TreeVersionID: scope.TreeVersionID, SourceID: input.SourceID, PersonID: input.PersonID, PlaceID: input.PlaceID, FromYear: input.FromYear, ToYear: input.ToYear}
	if _, err := tx.Exec(ctx, `INSERT INTO research_agent_runs (id, requested_by, question_id, query, normalized_query, entity_type, entity_id, tree_id, tree_version_id, status, stage, resolution, execution_mode, planner_version, algorithm_version, qualification_policy_version, scope) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'queued', 'queued', 'unresolved', 'asynchronous', $10, $11, $12, $13)`, runID, actorUUID, questionUUID, input.Question, identity.NormalizeArabicName(input.Question), input.EntityType, input.EntityID, nullableString(scope.TreeID), nullableString(scope.TreeVersionID), PlannerVersion, AlgorithmVersion, QualificationPolicy, mustJSON(runScope)); err != nil {
		return Run{}, err
	}
	enqueued, err := s.Jobs.EnqueueTx(ctx, tx, jobs.EnqueueInput{
		Type:           JobType,
		Payload:        mustJSON(map[string]string{"run_id": runID.String()}),
		Priority:       5,
		IdempotencyKey: "research-agent-run:" + runID.String(),
		MaxAttempts:    3,
	})
	if err != nil {
		return Run{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE research_agent_runs SET job_id = $1, updated_at = now() WHERE id = $2`, enqueued.Job.ID, runID); err != nil {
		return Run{}, err
	}
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `SELECT created_at FROM research_agent_runs WHERE id = $1`, runID).Scan(&createdAt); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return Run{
		ID:                         runID.String(),
		RequestedBy:                actorUUID.String(),
		QuestionID:                 input.QuestionID,
		Query:                      input.Question,
		NormalizedQuery:            identity.NormalizeArabicName(input.Question),
		EntityType:                 input.EntityType,
		EntityID:                   input.EntityID,
		TreeID:                     scope.TreeID,
		TreeVersionID:              scope.TreeVersionID,
		Status:                     RunQueued,
		Resolution:                 ResolutionUnresolved,
		ExecutionMode:              ExecutionModeAsynchronous,
		PlannerVersion:             PlannerVersion,
		AlgorithmVersion:           AlgorithmVersion,
		QualificationPolicyVersion: QualificationPolicy,
		JobID:                      enqueued.Job.ID,
		Stage:                      StageQueued,
		CreatedAt:                  createdAt,
		UpdatedAt:                  createdAt,
		Steps:                      []Step{},
		Evidence:                   []EvidenceRef{},
		Gaps:                       []Gap{},
		Recommendations:            []Recommendation{},
	}, nil
}

// ProcessRun walks the stages of one investigation, committing each one.
//
// The stages are the unit of progress. Each is executed, then its step row and
// its evidence are committed together, so the database holds what the agent found
// before the next stage starts. A failure after the fifth stage therefore leaves
// five steps a reader can look at and a sixth that never ran - and the next
// attempt resumes from there rather than starting the investigation again and
// asking the same eleven questions.
//
// The run only reaches succeeded in the same transaction that writes the report,
// the gaps and the recommendations, after the last stage. Every earlier write
// leaves it reading as running, which is the honest state for work in progress.
func (s *Service) ProcessRun(ctx context.Context, claim jobs.Lease, runID string) error {
	if err := s.readyToQueue(); err != nil {
		return err
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return ErrNotFound
	}
	heartbeat, workCtx, err := s.Jobs.StartHeartbeat(ctx, claim)
	if err != nil {
		return err
	}
	defer func() { _ = heartbeat.Stop() }()
	ctx = workCtx

	input, scope, requestedBy, ok, err := s.claimRun(ctx, id, claim)
	if err != nil {
		return s.refuseIfLost(heartbeat, err)
	}
	if !ok {
		// Another attempt owns this run, or it is already finished. There is
		// nothing to do and nothing that should be reported as a failure.
		return nil
	}
	plan, terms := decomposeQuestion(input)
	stage := stageContext{Input: input, ActorUUID: mustUUID(requestedBy), Terms: terms, TreeID: scope.TreeID, TreeVersion: scope.TreeVersionID}
	candidateSourceIDs, err := loadCandidateSourceIDs(ctx, s.Pool, input)
	if err != nil {
		return s.refuseIfLost(heartbeat, s.markRunFailed(ctx, claim, id, err))
	}
	stage.SourceIDs = candidateSourceIDs

	// The stages already on the row are the ones this run has finished. Reading
	// them back rather than keeping them in memory is what makes a resumed attempt
	// a resume: the process that failed is not the process that continues.
	completed, err := s.completedSteps(ctx, id)
	if err != nil {
		return s.refuseIfLost(heartbeat, s.markRunFailed(ctx, claim, id, err))
	}
	allEvidence, err := s.runEvidence(ctx, id)
	if err != nil {
		return s.refuseIfLost(heartbeat, s.markRunFailed(ctx, claim, id, err))
	}
	stageInput := map[string]any{"entityType": input.EntityType, "entityId": input.EntityID, "questionId": input.QuestionID, "treeVersionId": scope.TreeVersionID}
	for _, planStep := range plan {
		if completed[planStep.Stage] {
			if planStep.Stage == StageSearchSources {
				stage.SourceIDs = sourceIDsFromEvidence(stage.SourceIDs, allEvidence)
			}
			continue
		}
		if err := heartbeat.Check(); err != nil {
			return err
		}
		stepStarted := time.Now().UTC()
		refs, output, unresolved, stageErr := executeStage(ctx, s.Pool, stage, planStep.Stage)
		if stageErr != nil {
			return s.refuseIfLost(heartbeat, s.markRunFailed(ctx, claim, id, stageErr))
		}
		if err := s.persistStage(ctx, claim, id, planStep, stageInput, output, unresolved, stepStarted, refs); err != nil {
			return s.refuseIfLost(heartbeat, s.markRunFailed(ctx, claim, id, err))
		}
		allEvidence = mergeEvidence(allEvidence, refs)
		completed[planStep.Stage] = true
		if planStep.Stage == StageSearchSources {
			stage.SourceIDs = sourceIDsFromEvidence(stage.SourceIDs, refs)
		}
	}
	if err := heartbeat.Check(); err != nil {
		return err
	}
	if err := s.finalizeRun(ctx, claim, id, input, scope, requestedBy, plan, terms, allEvidence); err != nil {
		return s.refuseIfLost(heartbeat, s.markRunFailed(ctx, claim, id, err))
	}
	return nil
}

// claimRun moves the run to running and proves the lease in the same statement,
// then hands back what the stages need that only the run row knows: the question as
// it was validated, and the scope that was checked.
//
// A run left reading as running belongs to a previous attempt at this same job -
// the job id is the only thing that drives a run, and this attempt holds a claim on
// that job, so the attempt that held the previous claim no longer has one. A run
// that failed is a retry's to pick up, which is what makes the stages it already
// committed a resume rather than a write-off. Taking either over is deliberate: a
// run nothing can claim would leave the job finishing with nothing behind it. A run
// under a different job, or one already succeeded, is somebody else's and is left
// alone.
func (s *Service) claimRun(ctx context.Context, id uuid.UUID, claim jobs.Lease) (RunInput, Scope, string, bool, error) {
	var questionID pgtype.Text
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return RunInput{}, Scope{}, "", false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE research_agent_runs
		SET status = 'running', stage = 'planning', job_id = $1, started_at = COALESCE(started_at, now()), error = NULL, updated_at = now()
		WHERE id = $2 AND job_id = $1 AND status <> 'succeeded'
	`, claim.JobID, id)
	if err != nil {
		return RunInput{}, Scope{}, "", false, err
	}
	if tag.RowsAffected() == 0 {
		return RunInput{}, Scope{}, "", false, nil
	}
	if err := s.Jobs.HoldLease(ctx, tx, claim); err != nil {
		return RunInput{}, Scope{}, "", false, err
	}
	var question, requestedBy string
	var scopeData []byte
	if err := tx.QueryRow(ctx, `
		SELECT query, requested_by::text, COALESCE(question_id::text, ''), scope
		FROM research_agent_runs WHERE id = $1
	`, id).Scan(&question, &requestedBy, &questionID, &scopeData); err != nil {
		return RunInput{}, Scope{}, "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RunInput{}, Scope{}, "", false, err
	}
	return inputFromRunRow(question, textValue(questionID), scopeData), decodeScope(scopeData), requestedBy, true, nil
}

// inputFromRunRow rebuilds the investigation's input from the row.
//
// It is the counterpart of putting the scope on the row in StartRun: the accept
// path writes everything a worker will need, and this reads it back. Rebuilding
// from the row rather than carrying the value in memory is the whole point - the
// process that accepts a run is not the process that finishes it.
func inputFromRunRow(question, questionID string, scopeData []byte) RunInput {
	scope := decodeScope(scopeData)
	return RunInput{
		Question:      question,
		QuestionID:    questionID,
		EntityType:    scope.EntityType,
		EntityID:      scope.EntityID,
		TreeID:        scope.TreeID,
		TreeVersionID: scope.TreeVersionID,
		SourceID:      scope.SourceID,
		PersonID:      scope.PersonID,
		PlaceID:       scope.PlaceID,
		FromYear:      scope.FromYear,
		ToYear:        scope.ToYear,
	}
}

func decodeScope(data []byte) Scope {
	scope := Scope{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &scope)
	}
	return scope
}

// completedSteps is the set of stages the run has already committed. A stage with
// a step row is a stage whose evidence was committed with it, because the two are
// written in one transaction - so this is a resume point that cannot be wrong.
func (s *Service) completedSteps(ctx context.Context, runID uuid.UUID) (map[string]bool, error) {
	rows, err := s.Pool.Query(ctx, `SELECT stage FROM research_agent_steps WHERE run_id = $1 AND status IN ('succeeded', 'unresolved')`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	done := map[string]bool{}
	for rows.Next() {
		var stage string
		if err := rows.Scan(&stage); err != nil {
			return nil, err
		}
		done[stage] = true
	}
	return done, rows.Err()
}

func (s *Service) runEvidence(ctx context.Context, runID uuid.UUID) ([]EvidenceRef, error) {
	rows, err := s.Pool.Query(ctx, `SELECT layer, stance, reference_type, reference_id, COALESCE(source_id::text, ''), COALESCE(statement_id::text, ''), COALESCE(claim_id::text, ''), excerpt, metadata FROM research_agent_evidence WHERE run_id = $1 ORDER BY created_at, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceRef, 0)
	for rows.Next() {
		var item EvidenceRef
		var metadata []byte
		if err := rows.Scan(&item.Layer, &item.Stance, &item.ReferenceType, &item.ReferenceID, &item.SourceID, &item.StatementID, &item.ClaimID, &item.Excerpt, &metadata); err != nil {
			return nil, err
		}
		item.Metadata = decodeMap(metadata)
		items = append(items, item)
	}
	return items, rows.Err()
}

// persistStage commits one stage: the step row, its evidence, and the run's
// progress - all in one transaction that begins by proving the lease.
//
// One transaction per stage is the whole resume story. There is no window in which
// a step says it ran but its evidence is missing, and no window in which the
// evidence is on the row but the step that explains it is not.
func (s *Service) persistStage(ctx context.Context, claim jobs.Lease, runID uuid.UUID, planStep PlanStep, stageInput, output map[string]any, unresolved bool, startedAt time.Time, refs []EvidenceRef) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE research_agent_runs SET stage = $1, updated_at = now() WHERE id = $2 AND job_id = $3`, planStep.Stage, runID, claim.JobID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errRunSuperseded
	}
	status := "succeeded"
	if unresolved {
		status = "unresolved"
	}
	completedAt := time.Now().UTC()
	stepID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO research_agent_steps (id, run_id, step_order, stage, tool_name, status, input, output, evidence_count, started_at, completed_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, stepID, runID, planStep.Order, planStep.Stage, planStep.Tool, status, mustJSON(stageInput), mustJSON(output), len(refs), startedAt, completedAt); err != nil {
		return err
	}
	for _, item := range refs {
		item.StepID = stepID.String()
		if err := persistEvidence(ctx, tx, runID, item); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// finalizeRun writes the report and moves the run to its terminal state.
//
// Everything that makes a run readable as an answer - the report, the gaps, the
// recommendations, the resolution - goes in one transaction, together with the
// status change. That is what makes "succeeded" mean "all of it is there": there
// is no state in which the run reads as succeeded and the report is missing.
func (s *Service) finalizeRun(ctx context.Context, claim jobs.Lease, runID uuid.UUID, input RunInput, scope Scope, requestedBy string, plan []PlanStep, terms []string, allEvidence []EvidenceRef) error {
	packageData := buildEvidencePackage(allEvidence)
	gaps, recommendations := buildGaps(runID.String(), input, allEvidence, packageData.SourceCount, countEvidence(allEvidence, "research_claim"), countEvidence(allEvidence, "source_dependency"), countEvidence(allEvidence, "tree_interpretation"), countEvidence(allEvidence, "geographic_signal"), countEvidence(allEvidence, "temporal_signal"))
	resolution := ResolutionSucceeded
	if len(gaps) > 0 {
		resolution = ResolutionUnresolved
	}
	report := Report{AnswerAR: answerText(packageData, resolution), Plan: plan, Terms: terms, Scope: scope, EvidencePackage: packageData, AllowedActions: []string{"read_sources", "inspect_graph", "inspect_geography", "inspect_chronology", "inspect_claims", "inspect_dependencies"}, RestrictedActions: []string{"publish_tree", "accept_claim", "merge_people", "resolve_dispute", "modify_source_evidence"}, UnresolvedReasons: unresolvedReasons(gaps), GeneratedAt: time.Now().UTC()}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.Jobs.HoldLease(ctx, tx, claim); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE research_agent_runs SET stage = 'report', updated_at = now() WHERE id = $1 AND job_id = $2`, runID, claim.JobID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errRunSuperseded
	}
	for _, gap := range gaps {
		if _, err := tx.Exec(ctx, `INSERT INTO research_agent_gaps (id, run_id, kind, description_ar, severity, metadata) VALUES ($1, $2, $3, $4, $5, $6)`, uuid.MustParse(gap.ID), runID, gap.Kind, gap.DescriptionAR, gap.Severity, mustJSON(gap.Metadata)); err != nil {
			return err
		}
	}
	for _, recommendation := range recommendations {
		if _, err := tx.Exec(ctx, `INSERT INTO research_agent_recommendations (id, run_id, action, rationale_ar, priority, metadata) VALUES ($1, $2, $3, $4, $5, $6)`, uuid.MustParse(recommendation.ID), runID, recommendation.Action, recommendation.RationaleAR, recommendation.Priority, mustJSON(recommendation.Metadata)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE research_agent_runs SET status = 'succeeded', stage = 'complete', resolution = $1, report = $2, step_count = $3, evidence_count = $4, gap_count = $5, recommendation_count = $6, completed_at = now(), updated_at = now() WHERE id = $7
	`, resolution, mustJSON(report), len(plan), len(allEvidence), len(gaps), len(recommendations), runID); err != nil {
		return err
	}
	// The audit names the actor who asked for the investigation rather than the
	// worker that ran it: the worker changed nothing a person did not already ask
	// to be investigated, and an audit row that named the worker would hide the
	// requester.
	if err := writeAudit(ctx, tx, mustUUID(requestedBy), "research_agent_run_completed", "research_agent_run", runID, nil, map[string]any{"resolution": resolution, "stepCount": len(plan), "evidenceCount": len(allEvidence), "gapCount": len(gaps), "executionMode": ExecutionModeAsynchronous}, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RequeueRecovered puts the run of a job that stale recovery handed back where a
// worker can pick it up.
//
// The stages already committed are kept. A recovered run resumes from the first
// stage it never finished, which is the whole reason the stages are committed
// separately; resetting the run to queued without touching them is what makes a
// retry a resume rather than a second investigation.
func (s *Service) RequeueRecovered(ctx context.Context, recovered []jobs.JobView) error {
	for _, job := range recovered {
		if job.Type != JobType {
			continue
		}
		if _, err := s.Pool.Exec(ctx, `
			UPDATE research_agent_runs
			SET status = 'queued', stage = 'queued', started_at = NULL, error = NULL, updated_at = now()
			WHERE id = $1 AND status = 'running' AND started_at < now() - interval '20 minutes'
		`, job.ID); err != nil {
			return err
		}
	}
	return nil
}

// markRunFailed records a failure with the reason the worker actually hit, and
// only while this job is still the run's job and the run has not succeeded. A
// displaced worker reporting a failure over the attempt that is now making
// progress would be the partial-work-as-done outcome this change exists to
// prevent, and would spend one of the job's three attempts doing it.
func (s *Service) markRunFailed(ctx context.Context, claim jobs.Lease, runID uuid.UUID, cause error) error {
	message := "research agent run failed"
	if cause != nil {
		message = "research agent run failed: " + cause.Error()
	}
	if len([]rune(message)) > 4000 {
		message = "research agent run failed"
	}
	if !claim.Valid() {
		return ErrQueueUnavailable
	}
	if err := s.Jobs.HoldLease(ctx, s.Pool, claim); err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `
		UPDATE research_agent_runs
		SET status = 'failed', stage = 'failed', error = $1, completed_at = now(), updated_at = now()
		WHERE id = $2 AND job_id = $3 AND status <> 'succeeded'
	`, message, runID, claim.JobID); err != nil {
		return err
	}
	return cause
}

// refuseIfLost replaces a work error with the lease loss when the lease was the
// reason. A displaced worker sees its context cancelled and would otherwise report
// a database failure, which is a symptom; the fact that matters is that the run is
// somebody else's now.
func (s *Service) refuseIfLost(heartbeat *jobs.Heartbeat, err error) error {
	if err == nil {
		return nil
	}
	if lost := heartbeat.Check(); lost != nil {
		return lost
	}
	return err
}

func mustUUID(value string) uuid.UUID {
	parsed, _ := uuid.Parse(strings.TrimSpace(value))
	return parsed
}

// errRunSuperseded says the run is not this attempt's to work on.
var errRunSuperseded = errors.New("research agent run is owned by another attempt")

func executeStage(ctx context.Context, q queryer, stage stageContext, stageName string) ([]EvidenceRef, map[string]any, bool, error) {
	output := map[string]any{}
	switch stageName {
	case StageDecompose:
		output["terms"] = stage.Terms
		output["readOnly"] = true
		return nil, output, len(stage.Terms) == 0, nil
	case StageSearchSources:
		items, err := searchSources(ctx, q, stage)
		return items, map[string]any{"count": len(items), "sourceIds": sourceIDsFromEvidence(nil, items)}, len(items) == 0, err
	case StageSearchGraph:
		items, err := searchGraph(ctx, q, stage)
		return items, map[string]any{"count": len(items), "treeVersionId": stage.TreeVersion}, len(items) == 0, err
	case StageInspectGeography:
		items, err := inspectGeography(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageInspectChronology:
		items, err := inspectChronology(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageCompareClaims:
		items, err := compareClaims(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageSourceDependency:
		items, err := inspectSourceDependency(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageCounterEvidence:
		items, err := retrieveCounterEvidence(ctx, q, stage)
		return items, map[string]any{"count": len(items)}, len(items) == 0, err
	case StageEvidencePackage:
		return nil, map[string]any{"readOnly": true}, false, nil
	case StageMissingEvidence, StageRecommendation:
		return nil, map[string]any{"readOnly": true}, false, nil
	default:
		return nil, output, true, ErrValidation
	}
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, actorUUID(actorID)); err != nil {
		return Run{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return Run{}, ErrNotFound
	}
	result, err := scanRun(s.Pool.QueryRow(ctx, runSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if err := s.requireRunAccess(ctx, s.Pool, result, actorID); err != nil {
		return Run{}, err
	}
	if err := s.loadDetails(ctx, &result); err != nil {
		return Run{}, err
	}
	return result, nil
}

func (s *Service) GetLatestRun(ctx context.Context, actorID, questionID, entityType, entityID string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if err := requireRole(ctx, s.Pool, actorUUID(actorID)); err != nil {
		return Run{}, err
	}
	filters := []struct{ column, value string }{{"question_id", questionID}, {"entity_type", entityType}, {"entity_id", entityID}}
	query := runSelect + ` WHERE 1 = 1`
	args := make([]any, 0, len(filters))
	for _, filter := range filters {
		if strings.TrimSpace(filter.value) == "" {
			continue
		}
		if filter.column == "entity_type" {
			args = append(args, strings.ToLower(strings.TrimSpace(filter.value)))
		} else {
			parsed, parseErr := uuid.Parse(strings.TrimSpace(filter.value))
			if parseErr != nil {
				return Run{}, ErrValidation
			}
			args = append(args, parsed)
		}
		query += fmt.Sprintf(" AND %s = $%d", filter.column, len(args))
	}
	query += ` ORDER BY created_at DESC LIMIT 50`
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return Run{}, err
	}
	defer rows.Close()
	for rows.Next() {
		result, scanErr := scanRun(rows)
		if scanErr != nil {
			return Run{}, scanErr
		}
		if accessErr := s.requireRunAccess(ctx, s.Pool, result, actorID); accessErr == nil {
			if err := s.loadDetails(ctx, &result); err != nil {
				return Run{}, err
			}
			return result, nil
		} else if !errors.Is(accessErr, ErrForbidden) && !errors.Is(accessErr, ErrNotFound) {
			return Run{}, accessErr
		}
	}
	if err := rows.Err(); err != nil {
		return Run{}, err
	}
	return Run{}, ErrNotFound
}

func (s *Service) loadDetails(ctx context.Context, run *Run) error {
	stepRows, err := s.Pool.Query(ctx, `SELECT id, step_order, stage, tool_name, status, input, output, evidence_count, error, started_at, completed_at FROM research_agent_steps WHERE run_id = $1 ORDER BY step_order`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer stepRows.Close()
	run.Steps = make([]Step, 0)
	for stepRows.Next() {
		var step Step
		var id pgtype.UUID
		var inputData, outputData []byte
		var stepError pgtype.Text
		var completed pgtype.Timestamptz
		if err := stepRows.Scan(&id, &step.Order, &step.Stage, &step.Tool, &step.Status, &inputData, &outputData, &step.EvidenceCount, &stepError, &step.StartedAt, &completed); err != nil {
			return err
		}
		step.ID = uuidString(id)
		step.Input = decodeMap(inputData)
		step.Output = decodeMap(outputData)
		step.Error = textValue(stepError)
		if completed.Valid {
			value := completed.Time
			step.CompletedAt = &value
		}
		run.Steps = append(run.Steps, step)
	}
	if err := stepRows.Err(); err != nil {
		return err
	}
	evidenceRows, err := s.Pool.Query(ctx, `SELECT id, step_id, layer, stance, reference_type, reference_id, COALESCE(source_id::text, ''), COALESCE(statement_id::text, ''), COALESCE(claim_id::text, ''), excerpt, metadata FROM research_agent_evidence WHERE run_id = $1 ORDER BY created_at, id`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer evidenceRows.Close()
	run.Evidence = make([]EvidenceRef, 0)
	for evidenceRows.Next() {
		var item EvidenceRef
		var id, stepID pgtype.UUID
		var referenceID, sourceID, statementID, claimID string
		var metadata []byte
		if err := evidenceRows.Scan(&id, &stepID, &item.Layer, &item.Stance, &item.ReferenceType, &referenceID, &sourceID, &statementID, &claimID, &item.Excerpt, &metadata); err != nil {
			return err
		}
		item.ID = uuidString(id)
		item.StepID = uuidString(stepID)
		item.ReferenceID = referenceID
		item.SourceID = sourceID
		item.StatementID = statementID
		item.ClaimID = claimID
		item.Metadata = decodeMap(metadata)
		run.Evidence = append(run.Evidence, item)
	}
	if err := evidenceRows.Err(); err != nil {
		return err
	}
	gapRows, err := s.Pool.Query(ctx, `SELECT id, kind, description_ar, severity, status, metadata FROM research_agent_gaps WHERE run_id = $1 ORDER BY created_at, id`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer gapRows.Close()
	run.Gaps = make([]Gap, 0)
	for gapRows.Next() {
		var item Gap
		var id pgtype.UUID
		var metadata []byte
		if err := gapRows.Scan(&id, &item.Kind, &item.DescriptionAR, &item.Severity, &item.Status, &metadata); err != nil {
			return err
		}
		item.ID = uuidString(id)
		item.Metadata = decodeMap(metadata)
		run.Gaps = append(run.Gaps, item)
	}
	if err := gapRows.Err(); err != nil {
		return err
	}
	recommendationRows, err := s.Pool.Query(ctx, `SELECT id, action, rationale_ar, priority, status, metadata FROM research_agent_recommendations WHERE run_id = $1 ORDER BY created_at, id`, uuid.MustParse(run.ID))
	if err != nil {
		return err
	}
	defer recommendationRows.Close()
	run.Recommendations = make([]Recommendation, 0)
	for recommendationRows.Next() {
		var item Recommendation
		var id pgtype.UUID
		var metadata []byte
		if err := recommendationRows.Scan(&id, &item.Action, &item.RationaleAR, &item.Priority, &item.Status, &metadata); err != nil {
			return err
		}
		item.ID = uuidString(id)
		item.Metadata = decodeMap(metadata)
		run.Recommendations = append(run.Recommendations, item)
	}
	return recommendationRows.Err()
}

func (s *Service) requireRunAccess(ctx context.Context, q queryer, run Run, actorID string) error {
	if run.TreeID != "" {
		actor, err := uuid.Parse(strings.TrimSpace(actorID))
		if err != nil {
			return ErrForbidden
		}
		var allowed bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trees t JOIN tree_versions tv ON tv.tree_id = t.id AND tv.id = $3 WHERE t.id = $1 AND tv.state = 'published' AND (t.visibility = 'public' OR t.owner_id = $2 OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = $2)))`, run.TreeID, actor, run.TreeVersionID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return ErrForbidden
		}
	}
	if run.EntityType == "source" {
		actor, err := uuid.Parse(strings.TrimSpace(actorID))
		if err != nil {
			return ErrForbidden
		}
		var allowed bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sources WHERE id = $1 AND (visibility = 'public' OR created_by = $2))`, run.EntityID, actor).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return ErrForbidden
		}
	}
	return nil
}

func validateInput(input RunInput) (RunInput, error) {
	input.Question = strings.TrimSpace(input.Question)
	input.QuestionID = strings.TrimSpace(input.QuestionID)
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.EntityID = strings.TrimSpace(input.EntityID)
	input.TreeID = strings.TrimSpace(input.TreeID)
	input.TreeVersionID = strings.TrimSpace(input.TreeVersionID)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.PersonID = strings.TrimSpace(input.PersonID)
	input.PlaceID = strings.TrimSpace(input.PlaceID)
	if input.Question == "" || len([]rune(input.Question)) > 4000 {
		return RunInput{}, ErrValidation
	}
	if input.EntityType == "" {
		input.EntityType = "person"
	}
	if input.EntityType != "person" && input.EntityType != "family" && input.EntityType != "branch" && input.EntityType != "place" && input.EntityType != "source" {
		return RunInput{}, ErrValidation
	}
	for _, value := range []string{input.QuestionID, input.EntityID, input.TreeID, input.TreeVersionID, input.SourceID, input.PersonID, input.PlaceID} {
		if value == "" {
			continue
		}
		if _, err := uuid.Parse(value); err != nil {
			return RunInput{}, ErrValidation
		}
	}
	if (input.TreeID == "") != (input.TreeVersionID == "") {
		return RunInput{}, ErrValidation
	}
	if input.EntityID == "" {
		return RunInput{}, ErrValidation
	}
	if input.EntityType == "source" && input.SourceID != "" && input.SourceID != input.EntityID {
		return RunInput{}, ErrValidation
	}
	if input.FromYear < 0 || input.ToYear < 0 || (input.FromYear > 0 && input.ToYear > 0 && input.FromYear > input.ToYear) {
		return RunInput{}, ErrValidation
	}
	return input, nil
}

func loadScope(ctx context.Context, q queryer, input RunInput, actor uuid.UUID) (Scope, error) {
	scope := Scope{EntityType: input.EntityType, EntityID: input.EntityID, TreeID: input.TreeID, TreeVersionID: input.TreeVersionID, SourceID: input.SourceID, PersonID: input.PersonID, PlaceID: input.PlaceID, FromYear: input.FromYear, ToYear: input.ToYear}
	if input.TreeID != "" {
		var visibility, state string
		if err := q.QueryRow(ctx, `SELECT t.visibility, tv.state FROM trees t JOIN tree_versions tv ON tv.tree_id = t.id WHERE t.id = $1 AND tv.id = $2`, input.TreeID, input.TreeVersionID).Scan(&visibility, &state); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Scope{}, ErrNotFound
			}
			return Scope{}, err
		}
		if visibility != "public" || state != "published" {
			return Scope{}, ErrForbidden
		}
		if input.EntityType == "person" {
			var targetPresent bool
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tree_nodes WHERE tree_version_id = $1 AND person_id = $2)`, input.TreeVersionID, input.EntityID).Scan(&targetPresent); err != nil {
				return Scope{}, err
			}
			if !targetPresent {
				return Scope{}, ErrValidation
			}
		}
	}
	if input.EntityType == "source" {
		if err := requireSourceAccess(ctx, q, input.EntityID, actor); err != nil {
			return Scope{}, err
		}
	}
	if input.SourceID != "" && input.SourceID != input.EntityID {
		if err := requireSourceAccess(ctx, q, input.SourceID, actor); err != nil {
			return Scope{}, err
		}
	}
	var entityExists bool
	var entityQuery string
	switch input.EntityType {
	case "person":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM people WHERE id = $1)`
	case "family":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM families WHERE id = $1)`
	case "branch":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1)`
	case "place":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM places WHERE id = $1)`
	case "source":
		entityQuery = `SELECT EXISTS (SELECT 1 FROM sources WHERE id = $1)`
	default:
		return Scope{}, ErrValidation
	}
	if err := q.QueryRow(ctx, entityQuery, input.EntityID).Scan(&entityExists); err != nil {
		return Scope{}, err
	}
	if !entityExists {
		return Scope{}, ErrNotFound
	}
	return scope, nil
}

func requireSourceAccess(ctx context.Context, q queryer, sourceID string, actor uuid.UUID) error {
	var visibility string
	var owner pgtype.UUID
	if err := q.QueryRow(ctx, `SELECT visibility, created_by FROM sources WHERE id = $1`, sourceID).Scan(&visibility, &owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if visibility != "public" && (!owner.Valid || uuid.UUID(owner.Bytes).String() != actor.String()) {
		return ErrForbidden
	}
	return nil
}

func requireRole(ctx context.Context, q queryer, actor uuid.UUID) error {
	var allowed bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role = ANY($2::text[]))`, actor, []string{"researcher", "moderator", "admin"}).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func actorUUID(value string) uuid.UUID {
	parsed, _ := uuid.Parse(strings.TrimSpace(value))
	return parsed
}

func optionalUUID(value string) (any, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	return uuid.Parse(strings.TrimSpace(value))
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func loadCandidateSourceIDs(ctx context.Context, q queryer, input RunInput) ([]uuid.UUID, error) {
	items := make([]uuid.UUID, 0)
	seen := make(map[string]struct{})
	add := func(value string) {
		parsed, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return
		}
		if _, found := seen[parsed.String()]; found {
			return
		}
		seen[parsed.String()] = struct{}{}
		items = append(items, parsed)
	}
	add(input.SourceID)
	if input.EntityType == "source" {
		return items, nil
	}
	questionID, err := optionalUUID(input.QuestionID)
	if err != nil {
		return nil, err
	}
	questionUUID, _ := questionID.(uuid.UUID)
	var questionArg any
	if questionID != nil {
		questionArg = questionUUID
	}
	entityUUID, _ := uuid.Parse(input.EntityID)
	rows, err := q.Query(ctx, `
		SELECT source_id::text FROM question_sources WHERE question_id = $1
		UNION
		SELECT ss.source_id::text FROM question_claims qc JOIN claim_evidence ce ON ce.claim_id = qc.claim_id JOIN source_statements ss ON ss.id = ce.source_statement_id WHERE qc.question_id = $1
		UNION
		SELECT ss.source_id::text FROM claims c JOIN claim_evidence ce ON ce.claim_id = c.id JOIN source_statements ss ON ss.id = ce.source_statement_id WHERE c.subject_id = $2 OR c.object_id = $2
		LIMIT 100
	`, questionArg, entityUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		add(value)
	}
	return items, rows.Err()
}

func persistEvidence(ctx context.Context, tx pgx.Tx, runID uuid.UUID, item EvidenceRef) error {
	_, err := tx.Exec(ctx, `INSERT INTO research_agent_evidence (run_id, step_id, layer, stance, reference_type, reference_id, source_id, statement_id, claim_id, excerpt, metadata) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT DO NOTHING`, runID, item.StepID, item.Layer, item.Stance, item.ReferenceType, item.ReferenceID, nullableUUIDValue(item.SourceID), nullableUUIDValue(item.StatementID), nullableUUIDValue(item.ClaimID), item.Excerpt, mustJSON(item.Metadata))
	return err
}

func writeAudit(ctx context.Context, tx pgx.Tx, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))`, actor, action, entityType, entityID, mustJSON(before), mustJSON(after), reason)
	return err
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func decodeMap(data []byte) map[string]any {
	result := map[string]any{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &result)
	}
	return result
}

const runSelect = `
	SELECT id::text, requested_by::text, COALESCE(question_id::text, ''), query, normalized_query, entity_type, entity_id::text,
	       COALESCE(tree_id::text, ''), COALESCE(tree_version_id::text, ''), status, resolution, execution_mode,
	       planner_version, algorithm_version, qualification_policy_version, report, step_count, evidence_count, gap_count,
	       recommendation_count, COALESCE(error, ''), COALESCE(job_id::text, ''), stage, created_at, started_at, completed_at, updated_at
	FROM research_agent_runs`

func scanRun(row pgx.Row) (Run, error) {
	var result Run
	var questionID, treeID, treeVersionID, runError, jobID, stage pgtype.Text
	var reportData []byte
	var startedAt, completedAt pgtype.Timestamptz
	if err := row.Scan(&result.ID, &result.RequestedBy, &questionID, &result.Query, &result.NormalizedQuery, &result.EntityType, &result.EntityID, &treeID, &treeVersionID, &result.Status, &result.Resolution, &result.ExecutionMode, &result.PlannerVersion, &result.AlgorithmVersion, &result.QualificationPolicyVersion, &reportData, &result.StepCount, &result.EvidenceCount, &result.GapCount, &result.RecommendationCount, &runError, &jobID, &stage, &result.CreatedAt, &startedAt, &completedAt, &result.UpdatedAt); err != nil {
		return Run{}, err
	}
	result.QuestionID = textValue(questionID)
	result.TreeID = textValue(treeID)
	result.TreeVersionID = textValue(treeVersionID)
	result.Error = textValue(runError)
	result.JobID = textValue(jobID)
	result.Stage = textValue(stage)
	if len(reportData) > 0 {
		_ = json.Unmarshal(reportData, &result.Report)
	}
	if result.Report.Plan == nil {
		result.Report.Plan = []PlanStep{}
	}
	if result.Report.Terms == nil {
		result.Report.Terms = []string{}
	}
	if result.Report.AllowedActions == nil {
		result.Report.AllowedActions = []string{}
	}
	if result.Report.RestrictedActions == nil {
		result.Report.RestrictedActions = []string{}
	}
	if result.Report.UnresolvedReasons == nil {
		result.Report.UnresolvedReasons = []string{}
	}
	if result.Report.EvidencePackage.Evidence == nil {
		result.Report.EvidencePackage.Evidence = []EvidenceRef{}
	}
	if startedAt.Valid {
		value := startedAt.Time
		result.StartedAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		result.CompletedAt = &value
	}
	return result, nil
}

func sourceIDsFromEvidence(existing []uuid.UUID, evidence []EvidenceRef) []uuid.UUID {
	seen := make(map[string]struct{}, len(existing))
	items := make([]uuid.UUID, 0, len(existing))
	for _, value := range existing {
		seen[value.String()] = struct{}{}
		items = append(items, value)
	}
	for _, item := range evidence {
		if item.SourceID == "" {
			continue
		}
		parsed, err := uuid.Parse(item.SourceID)
		if err != nil {
			continue
		}
		if _, found := seen[parsed.String()]; found {
			continue
		}
		seen[parsed.String()] = struct{}{}
		items = append(items, parsed)
	}
	return items
}

func countEvidence(evidence []EvidenceRef, layer string) int {
	count := 0
	for _, item := range evidence {
		if item.Layer == layer {
			count++
		}
	}
	return count
}

func answerText(packageData EvidencePackage, resolution string) string {
	if packageData.Total == 0 {
		return "لم تُجمع أدلة مؤهلة كافية؛ يبقى السؤال غير محسوم ويحتاج إلى توسيع النطاق."
	}
	if resolution == ResolutionUnresolved {
		return fmt.Sprintf("جمعت حزمة من %d مادة (%d داعمة و%d مضادة)، لكنها لا تحسم السؤال بسبب الفجوات المسجلة.", packageData.Total, packageData.SupportCount, packageData.CounterCount)
	}
	return fmt.Sprintf("جمعت حزمة قابلة للتتبع من %d مادة عبر %d مصادر؛ راجع الاقتباسات قبل أي قرار.", packageData.Total, packageData.SourceCount)
}

func unresolvedReasons(gaps []Gap) []string {
	reasons := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		reasons = append(reasons, gap.DescriptionAR)
	}
	return reasons
}
