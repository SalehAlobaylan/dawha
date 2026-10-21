package sourceprocessing

import (
	"context"
	"io"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
)

func TestPDFReprocessingAndReviewedTranscriptionCreateVersionedCandidates(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "مراجع إعادة المعالجة")
	fixture.GrantRole(owner.User.ID, "researcher")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	queue := jobs.NewService(fixture.Pool())
	service := NewService(fixture.Pool(), store, queue, newStubProvider(), fixedPDFExtractor{text: "قراءة OCR: سالم بن ناصر"})
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "شجرة اختبار " + fixture.Unique("r"),
		SourceType: "family_document",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := service.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "شجرة.pdf",
		ContentType: "application/pdf",
		Content:     []byte("%PDF-1.7\nfixture"),
	})
	if err != nil {
		t.Fatalf("upload PDF: %v", err)
	}
	firstRunID := processNextSourceJob(t, ctx, service, queue, "reprocess-test-worker")
	firstDraft, err := service.CreateTreeDraft(ctx, file.ID, owner.User.ID, "مسودة التشغيل الأول")
	if err != nil {
		t.Fatalf("create first-run tree draft: %v", err)
	}
	if firstDraft.ProcessingRunID != firstRunID {
		t.Fatalf("first tree draft run = %s, want %s", firstDraft.ProcessingRunID, firstRunID)
	}

	processing, err := service.GetProcessing(ctx, source.Source.ID, owner.User.ID)
	if err != nil || len(processing.Candidates) == 0 {
		t.Fatalf("read first run candidates = %d, %v", len(processing.Candidates), err)
	}
	var preservedCandidateID string
	for _, candidate := range processing.Candidates {
		if candidate.CandidateType == "entity" && (candidate.Status == "unreviewed" || candidate.Status == "needs_review") {
			preservedCandidateID = candidate.ID
			break
		}
	}
	if preservedCandidateID == "" {
		t.Fatal("first run produced no undecided candidate to supersede")
	}
	if _, err := service.ReviewCandidate(ctx, preservedCandidateID, owner.User.ID, ReviewInput{Decision: "rejected", NoteAR: "قرار اختبار محفوظ"}); err != nil {
		t.Fatalf("review first-run candidate: %v", err)
	}

	secondRun, err := service.ReprocessFile(ctx, file.ID, owner.User.ID, ReprocessInput{RenderDPI: 400})
	if err != nil {
		t.Fatalf("queue higher-resolution rerun: %v", err)
	}
	if secondRun.ID == firstRunID || secondRun.RunKind != "document" || secondRun.PipelineConfig == nil {
		t.Fatalf("second run = %+v, want a distinct versioned document run", secondRun)
	}
	retriedRun, err := service.ReprocessFile(ctx, file.ID, owner.User.ID, ReprocessInput{RenderDPI: 400})
	if err != nil || retriedRun.ID != secondRun.ID {
		t.Fatalf("retry of active OCR request = %+v, %v; want existing run %s", retriedRun, err, secondRun.ID)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_processing_runs WHERE source_file_id = $1`, file.ID); got != 2 {
		t.Fatalf("processing run history has %d rows, want 2", got)
	}
	processNextSourceJob(t, ctx, service, queue, "reprocess-test-worker")
	var preservedStatus string
	if err := fixture.QueryRow(`SELECT status FROM source_candidates WHERE id = $1`, preservedCandidateID).Scan(&preservedStatus); err != nil || preservedStatus != "rejected" {
		t.Fatalf("prior reviewed candidate status = %q, %v; want rejected decision retained", preservedStatus, err)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_candidates WHERE source_file_id = $1 AND source_processing_run_id = $2 AND status = 'superseded'`, file.ID, firstRunID); got == 0 {
		t.Fatal("higher-resolution run did not retire the prior undecided candidates")
	}
	if got := fixture.Count(`SELECT count(*) FROM source_statements WHERE source_file_id = $1 AND review_status = 'superseded'`, file.ID); got == 0 {
		t.Fatal("higher-resolution run did not retire the statements for undecided prior candidates")
	}
	secondDraft, err := service.CreateTreeDraft(ctx, file.ID, owner.User.ID, "مسودة التشغيل الثاني")
	if err != nil {
		t.Fatalf("create rerun tree draft: %v", err)
	}
	if secondDraft.ID == firstDraft.ID || secondDraft.ProcessingRunID == firstDraft.ProcessingRunID {
		t.Fatalf("rerun draft = %+v; want a separate draft tied to run %s", secondDraft, firstRunID)
	}

	page, err := service.PageEvidence(ctx, file.ID, owner.User.ID, 1)
	if err != nil {
		t.Fatalf("read rerun page evidence: %v", err)
	}
	review, err := service.SavePageReview(ctx, file.ID, owner.User.ID, 1, PageReviewInput{
		ExpectedPageEvidenceID: page.PageEvidenceID,
		Status:                 "partial",
		TextAR:                 "النص البشري المصحح: سالم بن ناصر",
	})
	if err != nil {
		t.Fatalf("save page transcription: %v", err)
	}
	input := ReextractPageInput{PageNumber: 1, ExpectedPageEvidenceID: page.PageEvidenceID, PageReviewRevisionID: review.ID, ExpectedRevision: review.RevisionNumber}
	thirdRun, err := service.ReextractReviewedPage(ctx, file.ID, owner.User.ID, input)
	if err != nil {
		t.Fatalf("queue reviewed-text extraction: %v", err)
	}
	duplicateRequest, err := service.ReextractReviewedPage(ctx, file.ID, owner.User.ID, input)
	if err != nil || duplicateRequest.ID != thirdRun.ID {
		t.Fatalf("duplicate reviewed-text request = %+v, %v; want same run %s", duplicateRequest, err, thirdRun.ID)
	}
	if thirdRun.RunKind != "reviewed_page" || thirdRun.PageReviewRevisionID != review.ID {
		t.Fatalf("reviewed-page run = %+v, want exact human revision %s", thirdRun, review.ID)
	}
	processNextSourceJob(t, ctx, service, queue, "reprocess-test-worker")
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE source_processing_run_id = $1 AND source_page_evidence_id = $2 AND source_page_review_revision_id = $3 AND text_ar = $4`, thirdRun.ID, page.PageEvidenceID, review.ID, review.TextAR); got != 1 {
		t.Fatalf("reviewed transcription passages = %d, want one passage tied to the page and exact review revision", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_candidates WHERE source_processing_run_id = $1 AND status IN ('unreviewed', 'needs_review')`, thirdRun.ID); got == 0 {
		t.Fatal("reviewed transcription run produced no candidates for human review")
	}
	if got := fixture.Count(`SELECT count(*) FROM source_processing_runs WHERE source_file_id = $1`, file.ID); got != 3 {
		t.Fatalf("processing run history has %d rows, want 3", got)
	}
	// A terminal failure should permit another attempt from the same saved
	// transcription revision. Once that retry is queued, duplicate requests
	// must resolve to the same new attempt.
	fixture.Exec(`UPDATE source_processing_runs SET status = 'failed', stage = 'failed', error = 'fixture failure' WHERE id = $1`, thirdRun.ID)
	fixture.Exec(`UPDATE jobs SET status = 'dead', last_error = 'fixture failure' WHERE id = $1`, thirdRun.JobID)
	fourthRun, err := service.ReextractReviewedPage(ctx, file.ID, owner.User.ID, input)
	if err != nil {
		t.Fatalf("retry failed reviewed-text extraction: %v", err)
	}
	if fourthRun.ID == thirdRun.ID || fourthRun.PageReviewRevisionID != review.ID {
		t.Fatalf("retried run = %+v, want a distinct run pinned to review %s", fourthRun, review.ID)
	}
	retriedDuplicate, err := service.ReextractReviewedPage(ctx, file.ID, owner.User.ID, input)
	if err != nil || retriedDuplicate.ID != fourthRun.ID {
		t.Fatalf("duplicate failed-run retry = %+v, %v; want queued run %s", retriedDuplicate, err, fourthRun.ID)
	}
	processNextSourceJob(t, ctx, service, queue, "reprocess-test-worker")
	if got := fixture.Count(`SELECT count(*) FROM source_processing_runs WHERE source_file_id = $1`, file.ID); got != 4 {
		t.Fatalf("processing run history has %d rows, want 4 after retry", got)
	}
}

type fixedPDFExtractor struct {
	text string
}

func (extractor fixedPDFExtractor) Extract(_ context.Context, input ExtractInput) ([]Page, error) {
	if _, err := io.Copy(io.Discard, input.Reader); err != nil {
		return nil, err
	}
	return []Page{{
		Number: 1, Width: 100, Height: 200, Text: extractor.text,
		Readings: []PageReading{{Method: "pdf_text", Model: "fixture", Text: extractor.text, Selected: true}},
	}}, nil
}

func processNextSourceJob(t *testing.T, ctx context.Context, service *Service, queue *jobs.Service, workerID string) string {
	t.Helper()
	job, err := queue.Claim(ctx, jobs.ClaimInput{WorkerID: workerID, Type: SourceProcessJobType})
	if err != nil {
		t.Fatalf("claim source processing job: %v", err)
	}
	claim := Claim{Job: job, Lease: job.Lease(workerID)}
	payload, err := parseJobPayload(job.Payload)
	if err != nil {
		t.Fatalf("parse claimed source job: %v", err)
	}
	if err := service.Process(ctx, claim); err != nil {
		t.Fatalf("process source job: %v", err)
	}
	if _, err := queue.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: workerID, LeaseToken: claim.Lease.Token}); err != nil {
		t.Fatalf("complete source job: %v", err)
	}
	return payload.RunID
}
