package sourceprocessing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const SourceFrozenNameJudgeJobType = "source_frozen_name_judge"

// Canonical JSON hashing survives PostgreSQL jsonb key order. These signatures
// are internal evidence identities, never provider fields.
func FrozenNameJudgeSignature(payload json.RawMessage) string {
	var value any
	if json.Unmarshal(payload, &value) != nil {
		return ""
	}
	return hashNameValue(value)
}
func NamePreparationSignature(exposure, pool string, snapshot json.RawMessage) string {
	var value any
	if json.Unmarshal(snapshot, &value) != nil {
		return ""
	}
	return hashNameValue(map[string]any{"exposure": exposure, "pool": pool, "snapshot": value})
}

func (s *Service) enqueueFrozenNameJudge(ctx context.Context, tx pgx.Tx, prep NameCandidatePreparation, c NamePreparationContext, source suggestionSource) error {
	queue, ok := s.Jobs.(interface {
		EnqueueTx(context.Context, pgx.Tx, jobs.EnqueueInput) (jobs.EnqueueResult, error)
	})
	if !ok {
		return ErrQueueUnavailable
	}
	candidates := []JudgeCandidate{}
	for i, item := range prep.Snapshot {
		candidates = append(candidates, JudgeCandidate{Value: item.Value, Source: item.Sources[0], Rank: i + 1})
	}
	payload := nameJudgeAIRequest{State: FrozenNameJudgeState(c), Candidates: candidates, Mode: "shadow"}
	encoded := mustJSON(payload)
	signature := FrozenNameJudgeSignature(encoded)
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO source_name_judge_requests(preparation_id,signature,payload,identity) VALUES($1,$2,$3,$4) RETURNING id::text`, prep.ID, signature, encoded, mustJSON(nameJudgeIdentity())).Scan(&id)
	if err != nil {
		return err
	}
	var sourceID string
	if err = tx.QueryRow(ctx, `SELECT source_id::text FROM source_files WHERE id=$1`, source.sourceFileID).Scan(&sourceID); err != nil {
		return err
	}
	job, err := queue.EnqueueTx(ctx, tx, jobs.EnqueueInput{Type: SourceFrozenNameJudgeJobType, Payload: mustJSON(map[string]string{"judgeRequestId": id, "source_file_id": source.sourceFileID.String(), "source_id": sourceID}), MaxAttempts: 3, IdempotencyKey: "frozen-name-judge:" + id})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE source_name_judge_requests SET job_id=$2 WHERE id=$1`, id, job.Job.ID)
	return err
}
func readerEvidenceNames(values []NameReaderEvidence) []string {
	out := []string{}
	for _, v := range values {
		out = append(out, v.Value)
	}
	return out
}

// Only immutable input is loaded. No query reads a human answer or rebuilds
// file context, even when completion follows the decision by minutes.
func (s *Service) ProcessFrozenNameJudge(parent context.Context, claim Claim) (returned error) {
	if s == nil || s.Pool == nil || s.Jobs == nil || s.NameJudge == nil {
		return ErrNameJudgeUnavailable
	}
	if claim.Job.Type != SourceFrozenNameJudgeJobType {
		return ErrValidation
	}
	var jobPayload struct {
		Request string `json:"judgeRequestId"`
	}
	if json.Unmarshal(claim.Job.Payload, &jobPayload) != nil {
		return ErrValidation
	}
	if _, err := uuid.Parse(jobPayload.Request); err != nil {
		return ErrValidation
	}
	heartbeat, ctx, err := s.startHeartbeat(parent, claim)
	if err != nil {
		return err
	}
	defer func() {
		if heartbeat.Lost() != nil {
			returned = jobs.ErrLeaseLost
		}
		heartbeat.Stop()
	}()
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	var payload, identityJSON []byte
	var signature, jobID string
	var expired, referenced, finished bool
	err = s.Pool.QueryRow(ctx, `SELECT q.payload,q.identity,q.signature,q.job_id::text,q.expires_at<clock_timestamp(),
 EXISTS(SELECT 1 FROM source_name_decision_evidence d WHERE d.preparation_id=q.preparation_id),
 EXISTS(SELECT 1 FROM source_name_judge_results r WHERE r.request_id=q.id)
 FROM source_name_judge_requests q WHERE q.id=$1`, jobPayload.Request).Scan(&payload, &identityJSON, &signature, &jobID, &expired, &referenced, &finished)
	if err != nil {
		return err
	}
	if jobID != claim.Job.ID {
		return jobs.ErrLeaseLost
	}
	if finished {
		return nil
	}
	if expired && !referenced {
		return ErrNameJudgeUnavailable
	}
	if FrozenNameJudgeSignature(payload) != signature {
		return ErrValidation
	}
	var frozen nameJudgeAIRequest
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&frozen) != nil || frozen.ImageBase64 != "" || frozen.Mode != "shadow" {
		return ErrValidation
	}
	if err = validateJudgeCandidates(frozen.Candidates); err != nil {
		return err
	}
	if !validJudgeText(frozen.State.Reading, true) || len(frozen.State.Alternatives) > 5 || len(frozen.State.FileContext) > 20 {
		return ErrValidation
	}
	for _, v := range append(append([]string{}, frozen.State.Alternatives...), frozen.State.FileContext...) {
		if !validJudgeText(v, false) {
			return ErrValidation
		}
	}
	var identity FrozenJudgeIdentity
	if json.Unmarshal(identityJSON, &identity) != nil || identity.Provider != "liquid" || strings.TrimSpace(identity.Model) == "" || strings.TrimSpace(identity.Prompt) == "" {
		return ErrValidation
	}
	started := time.Now()
	result, err := s.NameJudge.Judge(ctx, NameJudgeRequest{State: frozen.State, Candidates: frozen.Candidates, Mode: "shadow", ExpectedIdentity: &identity})
	if err != nil {
		return err
	}
	if err = validateNameJudgeResult(result, frozen.Candidates, false); err != nil {
		return err
	}
	if result.Provider != identity.Provider || result.Model != identity.Model || result.PromptVersion != identity.Prompt {
		return ErrNameJudgeOutputInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var valid bool
	lease := claim.Lease
	err = tx.QueryRow(ctx, `SELECT status='running' AND type=$4 AND locked_by=$2 AND lease_token::text=$3 AND lease_expires_at>clock_timestamp() FROM jobs WHERE id=$1 FOR UPDATE`, lease.JobID, lease.WorkerID, lease.Token, SourceFrozenNameJudgeJobType).Scan(&valid)
	if err != nil || !valid {
		return jobs.ErrLeaseLost
	}
	if err = s.Jobs.HoldLease(ctx, tx, lease); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO source_name_judge_results(request_id,signature,result,latency_ms) VALUES($1,$2,$3,$4) ON CONFLICT(request_id) DO NOTHING`, jobPayload.Request, signature, mustJSON(result), min(120000, time.Since(started).Milliseconds()))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) attachNameDecisionEvidence(ctx context.Context, tx pgx.Tx, decisionID, suggestionID string, input SuggestionDecision) error {
	if input.EvidenceSchemaVersion == "" {
		return nil
	}
	if input.EvidenceSchemaVersion != NameEvidenceVersion {
		return ErrValidation
	}
	gap := "missing_initial_preparation"
	var exposure, preparation any
	if input.PreparationID != "" {
		if _, err := uuid.Parse(input.PreparationID); err != nil {
			return ErrValidation
		}
		var id, exposureID, signature string
		var before bool
		err := tx.QueryRow(ctx, `SELECT p.id::text,p.exposure_id::text,p.signature,p.created_at<d.created_at AND e.created_at<=p.created_at
 FROM source_name_candidate_preparations p JOIN source_name_review_exposures e ON e.id=p.exposure_id
 CROSS JOIN source_tree_suggestion_decisions d WHERE p.id=$1 AND p.suggestion_id=$2 AND d.id=$3`, input.PreparationID, suggestionID, decisionID).Scan(&id, &exposureID, &signature, &before)
		if err != nil || signature != input.PreparationSignature || !before {
			return ErrValidation
		}
		preparation = id
		exposure = exposureID
		gap = ""
	} else {
		var exposureID string
		err := tx.QueryRow(ctx, `SELECT id::text FROM source_name_review_exposures WHERE suggestion_id=$1`, suggestionID).Scan(&exposureID)
		if err == nil {
			exposure = exposureID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if s.nameJudgeSettings().Mode == "active" || input.JudgeWasVisible {
		gap = "active_review"
	}
	_, err := tx.Exec(ctx, `INSERT INTO source_name_decision_evidence(decision_id,exposure_id,preparation_id,schema_version,gap_reason) VALUES($1,$2,$3,$4,$5)`, decisionID, exposure, preparation, NameEvidenceVersion, gap)
	return err
}

func (s *Service) NameEvidenceStatus(ctx context.Context, fileID, actorID string) (map[string]int, error) {
	file, err := uuid.Parse(fileID)
	if err != nil {
		return nil, ErrNotFound
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	var exposures, preparations, eligible, requests, results, gaps, truncated int
	err = s.Pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM source_name_review_exposures e JOIN source_tree_suggestions s ON s.id=e.suggestion_id WHERE s.source_file_id=$1),
 count(*),count(*) FILTER(WHERE p.eligible),count(q.id),count(r.id),count(*) FILTER(WHERE q.id IS NOT NULL AND r.id IS NULL),
 count(*) FILTER(WHERE (e.context->'lexicon'->>'truncated')::boolean)
 FROM source_name_candidate_preparations p JOIN source_tree_suggestions s ON s.id=p.suggestion_id
 JOIN source_name_review_exposures e ON e.id=p.exposure_id
 LEFT JOIN source_name_judge_requests q ON q.preparation_id=p.id LEFT JOIN source_name_judge_results r ON r.request_id=q.id WHERE s.source_file_id=$1`, fileID).Scan(&exposures, &preparations, &eligible, &requests, &results, &gaps, &truncated)
	return map[string]int{"exposures": exposures, "preparations": preparations, "blindEligible": eligible, "judgeRequests": requests, "judgeResults": results, "pendingOrMissingResults": gaps, "truncatedContexts": truncated}, err
}

func FrozenNameJudgeState(c NamePreparationContext) JudgeState {
	return JudgeState{Reading: c.Reading, Alternatives: boundedJudgeNames(readerEvidenceNames(c.Readers), 5), FileContext: lexiconJudgeNames(c.Lexicon), Crop: c.Crop}
}
func ValidateTextOnlyNameJudgeResult(result NameJudgeResult, candidates []JudgeCandidate) error {
	return validateNameJudgeResult(result, candidates, false)
}
