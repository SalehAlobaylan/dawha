package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/jackc/pgx/v5"
)

type frozenFixtureJudge struct {
	calls   atomic.Int32
	entered chan NameJudgeRequest
	release chan struct{}
}

func (j *frozenFixtureJudge) Judge(ctx context.Context, in NameJudgeRequest) (NameJudgeResult, error) {
	j.calls.Add(1)
	if j.entered != nil {
		j.entered <- in
	}
	if j.release != nil {
		select {
		case <-j.release:
		case <-ctx.Done():
			return NameJudgeResult{}, ctx.Err()
		}
	}
	choice := in.Candidates[0].Value
	return NameJudgeResult{Choice: &choice, Probabilities: map[string]float64{choice: .8, "unclear": .2}, Confidence: .8, Provider: "liquid", Model: "d1:3b", PromptVersion: "name-judge-liquid-v1"}, nil
}
func preparedReviewFixture(t *testing.T) (*nameRunFixture, []SuggestionView) {
	t.Helper()
	f := newNameRunFixture(t)
	f.prepare(t)
	f.process(t, f.claim(t, "local-reader"))
	f.service.NameJudgeMode = "shadow"
	f.service.NameJudgeSample = 1
	f.service.NameJudge = &frozenFixtureJudge{}
	in := NameEvaluationRegistration{Partition: "held_out", GroupKey: strings.Repeat("a", 64), Contract: f.service.NameEvaluationContract(f.f.Ctx())}
	if err := f.service.RegisterNameEvaluation(f.f.Ctx(), f.fileID, f.actor.User.ID, in); err != nil {
		t.Fatal(err)
	}
	queue, err := f.service.ListSuggestions(f.f.Ctx(), f.fileID, f.actor.User.ID, "", 50, 0)
	if err != nil || len(queue) != 2 {
		t.Fatalf("queue: %v %d", err, len(queue))
	}
	return f, queue
}
func initialFixtureSnapshot(value, source string) []SuggestionCandidateSnapshot {
	key := nameCanonical(value)
	return []SuggestionCandidateSnapshot{{Value: value, Key: key, CompactKey: nameCompact(key), ParticleKey: nameParticle(key), Sources: []string{source}, Ranks: map[string]int{source: 1}, Score: 100, Shown: true, Position: 1}}
}
func freezeReviewFixture(t *testing.T, f *nameRunFixture, id string) (NamePreparationContext, NameCandidatePreparation) {
	t.Helper()
	c, err := f.service.BeginNameReview(f.f.Ctx(), id, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.service.PrepareNameCandidates(f.f.Ctx(), id, f.actor.User.ID, PrepareNameCandidatesInput{ExposureID: c.ExposureID, PoolVersion: NamePoolVersion, Snapshot: initialFixtureSnapshot(c.Reading, "reader")})
	if err != nil {
		t.Fatal(err)
	}
	return c, p
}
func TestFileLexiconIsCompleteCountedPrivateAndStable(t *testing.T) {
	f := newNameRunFixture(t)
	// These are synthetic human-decision fixtures only inside testsupport's
	// dedicated migrated schema. They are never written to the review database.
	_, err := f.f.Pool().Exec(f.f.Ctx(), `WITH inserted AS (
 INSERT INTO source_tree_suggestions(source_id,source_file_id,occurrence_id,page_number,region,reading_method,reading_model,status,decided_text_ar,decided_by,decided_at)
 SELECT source_id,id,'lex-'||n,1,'{"x":0.1,"y":0.1,"width":0.1,"height":0.1}','fixture','fixture','corrected',CASE WHEN n<=40 THEN 'عبد الله' ELSE 'اسم '||n END,$2,clock_timestamp() FROM source_files CROSS JOIN generate_series(1,340)n WHERE id=$1 RETURNING id,decided_text_ar
 ) INSERT INTO source_tree_suggestion_decisions(suggestion_id,reviewer_id,decision,suggested_text_ar,decided_text_ar) SELECT id,$2,'corrected','',decided_text_ar FROM inserted`, f.fileID, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.NameLexicon(f.f.Ctx(), f.fileID, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.service.NameLexicon(f.f.Ctx(), f.fileID, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalOccurrences != 340 || len(first.Entries) != 301 || first.Truncated || first.Revision != again.Revision || !first.AsOf.Equal(again.AsOf) {
		t.Fatalf("incomplete/unstable lexicon: %+v", first)
	}
	found := false
	for _, e := range first.Entries {
		if e.Value == "عبد الله" {
			found = e.Count == 40
		}
	}
	if !found {
		t.Fatal("repeated spelling count discarded")
	}
	other := actor.RegisterAndLogin(t, f.f, "مراجع آخر")
	if _, err = f.service.NameLexicon(f.f.Ctx(), f.fileID, other.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("lexicon authorization: %v", err)
	}
}
func TestInitialPreparationAndLateJudgeAreIndependentOfTheHumanAnswer(t *testing.T) {
	f, queue := preparedReviewFixture(t)
	judge := &frozenFixtureJudge{entered: make(chan NameJudgeRequest, 1), release: make(chan struct{})}
	f.service.NameJudge = judge
	c, p := freezeReviewFixture(t, f, queue[0].ID)
	if c.PrivacyPolicy != "local_text_only" || c.JudgeMode != "shadow" || c.JudgeSample != 1 {
		t.Fatal("frozen privacy posture missing")
	}
	if !c.Eligible {
		t.Fatalf("fresh registered exposure ineligible: %s", c.EligibilityReason)
	}
	duplicate, err := f.service.PrepareNameCandidates(f.f.Ctx(), queue[0].ID, f.actor.User.ID, PrepareNameCandidatesInput{ExposureID: c.ExposureID, PoolVersion: NamePoolVersion, Snapshot: p.Snapshot})
	if err != nil || duplicate.ID != p.ID {
		t.Fatal("retry duplicated preparation")
	}
	worker := "frozen-model"
	job, err := f.queue.Claim(f.f.Ctx(), jobs.ClaimInput{WorkerID: worker, Type: SourceFrozenNameJudgeJobType})
	if err != nil {
		t.Fatal(err)
	}
	claim := Claim{Job: job, Lease: job.Lease(worker)}
	// A queued request must retain its original identity after configuration
	// changes, rather than adopting the worker's current environment.
	t.Setenv("NAME_JUDGE_PROVIDER", "jev")
	t.Setenv("LIQUID_MODEL", "changed-model")
	t.Setenv("LIQUID_PROMPT_VERSION", "changed-prompt")
	done := make(chan error, 1)
	go func() { done <- f.service.ProcessFrozenNameJudge(f.f.Ctx(), claim) }()
	var sent NameJudgeRequest
	select {
	case sent = <-judge.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	if sent.ExpectedIdentity == nil || *sent.ExpectedIdentity != (FrozenJudgeIdentity{Provider: "liquid", Model: "d1:3b", Prompt: "name-judge-liquid-v1"}) {
		t.Fatalf("frozen identity was not passed to the adapter: %+v", sent.ExpectedIdentity)
	}
	start := time.Now()
	_, err = f.service.DecideSuggestion(f.f.Ctx(), queue[0].ID, f.actor.User.ID, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "خالد", EvidenceSchemaVersion: NameEvidenceVersion, PreparationID: p.ID, PreparationSignature: p.Signature, CandidateSnapshot: initialFixtureSnapshot("خالد", "common"), InputMethod: "typed", NormalizerVersion: "ar-name-keys-v1", CandidatePoolVersion: NamePoolVersion, JudgeOutput: json.RawMessage(`{"invented":"browser output"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("human save waited for invisible model")
	}
	if f.f.Count(`SELECT count(*) FROM source_name_judge_results`) != 0 {
		t.Fatal("result fabricated before model returned")
	}
	if _, err = f.service.DecideSuggestion(f.f.Ctx(), queue[1].ID, f.actor.User.ID, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "أحمد"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range sent.State.FileContext {
		if name == "خالد" || name == "أحمد" {
			t.Fatal("post-freeze answer leaked")
		}
	}
	if len(sent.ImagePNG) != 0 || len(sent.State.FileContext) > 20 {
		t.Fatal("provider privacy/bound changed")
	}
	close(judge.release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = f.service.ProcessFrozenNameJudge(f.f.Ctx(), claim); err != nil {
		t.Fatal(err)
	}
	if judge.calls.Load() != 1 || f.f.Count(`SELECT count(*) FROM source_name_judge_results`) != 1 {
		t.Fatal("retry duplicated local inference or result")
	}
	var initial, visible, decided string
	if err = f.f.Pool().QueryRow(f.f.Ctx(), `SELECT p.snapshot->0->>'value',d.candidate_snapshot->0->>'value',d.decided_text_ar FROM source_name_decision_evidence e JOIN source_tree_suggestion_decisions d ON d.id=e.decision_id JOIN source_name_candidate_preparations p ON p.id=e.preparation_id`).Scan(&initial, &visible, &decided); err != nil {
		t.Fatal(err)
	}
	if initial != "محمد" || visible != "خالد" || decided != "خالد" {
		t.Fatal("initial, visible, or human evidence changed")
	}
	if _, err = f.service.BeginNameReview(f.f.Ctx(), queue[0].ID, f.actor.User.ID); !errors.Is(err, ErrSuggestionAlreadyDecided) {
		t.Fatalf("late preparation: %v", err)
	}
}
func TestReviewHistoryAndLineageCannotCreateAnotherBlindBoundary(t *testing.T) {
	for _, kind := range []string{"accepted_reopen", "deferred_reopen", "split_parent", "untracked_client", "expired_preparation"} {
		t.Run(kind, func(t *testing.T) {
			f, queue := preparedReviewFixture(t)
			if kind == "expired_preparation" {
				c, err := f.service.BeginNameReview(f.f.Ctx(), queue[0].ID, f.actor.User.ID)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.f.Pool().Exec(f.f.Ctx(), `UPDATE source_name_review_exposures SET deadline=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ExposureID)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.service.PrepareNameCandidates(f.f.Ctx(), queue[0].ID, f.actor.User.ID, PrepareNameCandidatesInput{ExposureID: c.ExposureID, PoolVersion: NamePoolVersion, Snapshot: initialFixtureSnapshot("محمد", "reader")})
				if !errors.Is(err, ErrConflict) {
					t.Fatalf("expired freeze: %v", err)
				}
				return
			}
			target := queue[0].ID
			if kind == "untracked_client" {
				if err := f.service.RecordLegacyNameExposure(f.f.Ctx(), queue[:1]); err != nil {
					t.Fatal(err)
				}
			} else {
				decision := SuggestionAccepted
				if kind == "deferred_reopen" {
					decision = SuggestionDeferred
				}
				if _, err := f.service.DecideSuggestion(f.f.Ctx(), target, f.actor.User.ID, SuggestionDecision{Decision: decision}); err != nil {
					t.Fatal(err)
				}
				if kind == "split_parent" {
					_, err := f.f.Pool().Exec(f.f.Ctx(), `INSERT INTO source_name_occurrence_lineage(parent_id,child_id,kind) VALUES($1,$2,'split')`, target, queue[1].ID)
					if err != nil {
						t.Fatal(err)
					}
					target = queue[1].ID
				} else {
					// Fake future Plan 026 operation: preserve the ledger, only reopen status.
					if _, err := f.f.Pool().Exec(f.f.Ctx(), `UPDATE source_tree_suggestions SET status='suggested' WHERE id=$1`, target); err != nil {
						t.Fatal(err)
					}
				}
			}
			c, p := freezeReviewFixture(t, f, target)
			if c.Eligible {
				t.Fatalf("history became blind: %s", kind)
			}
			var eligible bool
			if err := f.f.Pool().QueryRow(f.f.Ctx(), `SELECT eligible FROM source_name_candidate_preparations WHERE id=$1`, p.ID).Scan(&eligible); err != nil || eligible {
				t.Fatalf("stored eligibility: %v %t", err, eligible)
			}
		})
	}
}
func TestIdenticalOccurrencesKeepDistinctEvidenceAndRegistrationCannotMixGroups(t *testing.T) {
	f, queue := preparedReviewFixture(t)
	_, first := freezeReviewFixture(t, f, queue[0].ID)
	_, second := freezeReviewFixture(t, f, queue[1].ID)
	if first.ID == second.ID || f.f.Count(`SELECT count(*) FROM source_name_judge_requests`) != 2 {
		t.Fatal("distinct physical occurrences were deduplicated")
	}
	if f.f.Count(`SELECT count(DISTINCT signature) FROM source_name_judge_requests`) != 1 {
		t.Fatal("fixture requests should have identical computation payload")
	}
	in := NameEvaluationRegistration{Partition: "development", GroupKey: strings.Repeat("b", 64), Contract: f.service.NameEvaluationContract(f.f.Ctx())}
	if err := f.service.RegisterNameEvaluation(f.f.Ctx(), f.fileID, f.actor.User.ID, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("partition changed: %v", err)
	}
}

func TestEvaluationRegistrationIsIdempotentAndSharesAnUnchangedGroupContract(t *testing.T) {
	f := newNameRunFixture(t)
	in := NameEvaluationRegistration{Partition: "held_out", GroupKey: strings.Repeat("c", 64), Contract: f.service.NameEvaluationContract(f.f.Ctx())}
	for i := 0; i < 2; i++ {
		if err := f.service.RegisterNameEvaluation(f.f.Ctx(), f.fileID, f.actor.User.ID, in); err != nil {
			t.Fatalf("registration attempt %d: %v", i+1, err)
		}
	}
	source, err := evidence.NewService(f.f.Pool()).CreateSource(f.f.Ctx(), f.actor.User.ID, evidence.CreateSourceInput{TitleAR: "نسخة اختبار " + f.f.Unique("group"), SourceType: "family_document"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := f.service.Upload(f.f.Ctx(), source.Source.ID, f.actor.User.ID, UploadInput{Filename: "copy.png", ContentType: "image/png", Content: syntheticChartPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.RegisterNameEvaluation(f.f.Ctx(), file.ID, f.actor.User.ID, in); err != nil {
		t.Fatalf("second rendering rejected the same contract: %v", err)
	}
	if f.f.Count(`SELECT count(*) FROM source_name_evaluation_groups`) != 1 || f.f.Count(`SELECT count(*) FROM source_name_evaluation_members`) != 2 {
		t.Fatal("re-registration created duplicate groups or members")
	}
	in.Contract["reader"] = "changed-reader"
	if err = f.service.RegisterNameEvaluation(f.f.Ctx(), f.fileID, f.actor.User.ID, in); err == nil {
		t.Fatal("changed contract was accepted")
	}
}
func TestPreparationGapsAndUnsupportedCandidatesRemainHonest(t *testing.T) {
	f, queue := preparedReviewFixture(t)
	c, err := f.service.BeginNameReview(f.f.Ctx(), queue[0].ID, f.actor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	bad := initialFixtureSnapshot("اسم مخترع غير موجود", "reader")
	if _, err = f.service.PrepareNameCandidates(f.f.Ctx(), queue[0].ID, f.actor.User.ID, PrepareNameCandidatesInput{ExposureID: c.ExposureID, PoolVersion: NamePoolVersion, Snapshot: bad}); !errors.Is(err, ErrValidation) {
		t.Fatalf("invented reading admitted: %v", err)
	}
	if err = f.service.SealNameReview(f.f.Ctx(), queue[0].ID, f.actor.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.DecideSuggestion(f.f.Ctx(), queue[0].ID, f.actor.User.ID, SuggestionDecision{Decision: SuggestionCorrected, TextAR: "خالد", EvidenceSchemaVersion: NameEvidenceVersion}); err != nil {
		t.Fatal(err)
	}
	if f.f.Count(`SELECT count(*) FROM source_name_decision_evidence WHERE gap_reason='missing_initial_preparation'`) != 1 {
		t.Fatal("missing preparation was reconstructed")
	}
}

type failedFrozenQueue struct{ LeaseQueue }

func (failedFrozenQueue) EnqueueTx(context.Context, pgx.Tx, jobs.EnqueueInput) (jobs.EnqueueResult, error) {
	return jobs.EnqueueResult{}, ErrQueueUnavailable
}
func TestOptionalJudgeQueueFailurePreservesTheInitialPool(t *testing.T) {
	f, queue := preparedReviewFixture(t)
	f.service.Jobs = failedFrozenQueue{f.queue}
	_, p := freezeReviewFixture(t, f, queue[0].ID)
	if p.ID == "" || f.f.Count(`SELECT count(*) FROM source_name_candidate_preparations`) != 1 || f.f.Count(`SELECT count(*) FROM source_name_judge_requests`) != 0 {
		t.Fatal("optional judge failure discarded initial evidence")
	}
}
func TestFrozenJudgeLostLeaseCannotWriteALateResult(t *testing.T) {
	f, queue := preparedReviewFixture(t)
	judge := &frozenFixtureJudge{entered: make(chan NameJudgeRequest, 1), release: make(chan struct{})}
	f.service.NameJudge = judge
	freezeReviewFixture(t, f, queue[0].ID)
	worker := "lost-frozen-lease"
	job, err := f.queue.Claim(f.f.Ctx(), jobs.ClaimInput{WorkerID: worker, Type: SourceFrozenNameJudgeJobType})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.service.ProcessFrozenNameJudge(f.f.Ctx(), Claim{Job: job, Lease: job.Lease(worker)}) }()
	select {
	case <-judge.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("model did not start")
	}
	if _, err = f.f.Pool().Exec(f.f.Ctx(), `UPDATE jobs SET lease_token=gen_random_uuid() WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("lost lease did not cancel inference")
	}
	if !errors.Is(err, jobs.ErrLeaseLost) || f.f.Count(`SELECT count(*) FROM source_name_judge_results`) != 0 {
		t.Fatalf("lost lease wrote a result: %v", err)
	}
}

func TestKnownLegacyRenderingCannotBeEnrolledAsHeldOutUnderANewSource(t *testing.T) {
	f := newNameRunFixture(t)
	if _, err := f.f.Pool().Exec(f.f.Ctx(), `UPDATE source_files SET created_at=(SELECT installed_at-interval '1 day' FROM source_name_review_boundary WHERE version=$2) WHERE id=$1`, f.fileID, NameEvidenceVersion); err != nil {
		t.Fatal(err)
	}
	source, err := evidence.NewService(f.f.Pool()).CreateSource(f.f.Ctx(), f.actor.User.ID, evidence.CreateSourceInput{TitleAR: "نسخة مصدر اختبار " + f.f.Unique("render"), SourceType: "family_document"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := f.service.Upload(f.f.Ctx(), source.Source.ID, f.actor.User.ID, UploadInput{Filename: "copy.png", ContentType: "image/png", Content: syntheticChartPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	err = f.service.RegisterNameEvaluation(f.f.Ctx(), file.ID, f.actor.User.ID, NameEvaluationRegistration{Partition: "held_out", GroupKey: strings.Repeat("b", 64), Contract: f.service.NameEvaluationContract(f.f.Ctx())})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("known old rendering admitted as unseen: %v", err)
	}
}
