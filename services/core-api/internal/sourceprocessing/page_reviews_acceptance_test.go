package sourceprocessing

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
)

func TestPageTranscriptionReviewIsAuditedAppendOnlyAndOptimistic(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "مراجع الصفحة")
	stranger := actor.Register(t, fixture, "مراجع بلا صلاحية")
	fixture.GrantRole(owner.User.ID, "researcher")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	uploadService := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "شجرة مراجعة " + fixture.Unique("s"),
		SourceType: "family_document",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := uploadService.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "شجرة.pdf",
		ContentType: "application/pdf",
		Content:     []byte("%PDF-1.7\\nreview fixture"),
	})
	if err != nil {
		t.Fatalf("upload source PDF: %v", err)
	}
	// Reading and correcting page evidence needs PostgreSQL only; an OCR worker,
	// object store, and AI provider are not required for review operations.
	service := NewService(fixture.Pool(), nil, nil, nil, nil)
	var runID string
	if err := fixture.QueryRow(`SELECT id::text FROM source_processing_runs WHERE source_file_id = $1`, file.ID).Scan(&runID); err != nil {
		t.Fatalf("find processing run: %v", err)
	}
	fixture.Exec(`
		INSERT INTO source_page_evidence
			(run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count)
		VALUES ($1, $2, $3, 1, 100, 200, 'readable', 'pdf_text', 'النص الآلي',
		        '[{"method":"pdf_text","model":"fixture","text":"النص الآلي","selected":true,"regions":[]}]'::jsonb,
		        '[]'::jsonb, 0)
	`, runID, source.Source.ID, file.ID)
	var originalEvidenceID string
	if err := fixture.QueryRow(`SELECT id::text FROM source_page_evidence WHERE source_file_id = $1 AND page_number = 1`, file.ID).Scan(&originalEvidenceID); err != nil {
		t.Fatalf("find original page evidence: %v", err)
	}

	if _, err := service.SavePageReview(ctx, file.ID, stranger.User.ID, 1, PageReviewInput{
		ExpectedPageEvidenceID: originalEvidenceID,
		Status:                 "transcribed", TextAR: "محاولة غير مصرح بها",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("review by ungranted user = %v, want ErrForbidden", err)
	}
	first, err := service.SavePageReview(ctx, file.ID, owner.User.ID, 1, PageReviewInput{
		ExpectedPageEvidenceID: originalEvidenceID,
		ExpectedRevision:       0,
		Status:                 "partial",
		TextAR:                 "نسب يدوي جزئي",
		NoteAR:                 "بقي اسم غير واضح",
	})
	if err != nil {
		t.Fatalf("save first transcription: %v", err)
	}
	if first.RevisionNumber != 1 || first.Status != "partial" || first.TextAR != "نسب يدوي جزئي" {
		t.Fatalf("first review = %+v", first)
	}
	if _, err := service.SavePageReview(ctx, file.ID, owner.User.ID, 1, PageReviewInput{
		ExpectedPageEvidenceID: originalEvidenceID,
		ExpectedRevision:       0,
		Status:                 "transcribed",
		TextAR:                 "نسخة متقادمة",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale review = %v, want ErrConflict", err)
	}
	second, err := service.SavePageReview(ctx, file.ID, owner.User.ID, 1, PageReviewInput{
		ExpectedPageEvidenceID: originalEvidenceID,
		ExpectedRevision:       1,
		Status:                 "transcribed",
		TextAR:                 "النسب بعد المراجعة",
	})
	if err != nil {
		t.Fatalf("save corrected transcription: %v", err)
	}
	if second.RevisionNumber != 2 {
		t.Fatalf("second revision number = %d, want 2", second.RevisionNumber)
	}
	page, err := service.PageEvidence(ctx, file.ID, owner.User.ID, 1)
	if err != nil {
		t.Fatalf("read page evidence: %v", err)
	}
	if page.Text != "النص الآلي" || page.CurrentReview == nil || page.CurrentReview.TextAR != "النسب بعد المراجعة" || page.ReviewCount != 2 || page.CurrentReview.SourcePageEvidenceID != page.PageEvidenceID {
		t.Fatalf("page evidence = %+v, want machine reading intact and revision 2 visible", page)
	}
	fixture.Exec(`
		INSERT INTO source_page_evidence
			(run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count)
		VALUES ($1, $2, $3, 1, 100, 200, 'readable', 'pdf_text', 'قراءة آلية أحدث',
		        '[{"method":"pdf_text","model":"fixture-v2","text":"قراءة آلية أحدث","selected":true,"regions":[]}]'::jsonb,
		        '[]'::jsonb, 0)
	`, runID, source.Source.ID, file.ID)
	latest, err := service.PageEvidence(ctx, file.ID, owner.User.ID, 1)
	if err != nil {
		t.Fatalf("read latest page evidence: %v", err)
	}
	if latest.PageEvidenceID == page.PageEvidenceID || latest.Text != "قراءة آلية أحدث" || latest.CurrentReview == nil || latest.CurrentReview.TextAR != "النسب بعد المراجعة" || latest.CurrentReview.SourcePageEvidenceID != page.PageEvidenceID || latest.ReviewCount != 0 {
		t.Fatalf("latest page evidence = %+v, want new machine reading and prior transcription visibly bound to the old reading", latest)
	}
	if _, err := service.SavePageReview(ctx, file.ID, owner.User.ID, 1, PageReviewInput{
		ExpectedPageEvidenceID: page.PageEvidenceID,
		ExpectedRevision:       0,
		Status:                 "transcribed",
		TextAR:                 "نسخة من نموذج قديم",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("review against stale page evidence = %v, want ErrConflict", err)
	}
	third, err := service.SavePageReview(ctx, file.ID, owner.User.ID, 1, PageReviewInput{
		ExpectedPageEvidenceID: latest.PageEvidenceID,
		ExpectedRevision:       0,
		Status:                 "transcribed",
		TextAR:                 "النسب بعد مراجعة القراءة الجديدة",
	})
	if err != nil {
		t.Fatalf("save transcription for latest page evidence: %v", err)
	}
	if third.RevisionNumber != 1 || third.SourcePageEvidenceID != latest.PageEvidenceID {
		t.Fatalf("latest evidence revision = %+v, want revision 1 bound to the new evidence row", third)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_page_evidence WHERE source_file_id = $1 AND page_number = 1`, file.ID); got != 2 {
		t.Fatalf("page evidence snapshots = %d, want two immutable readings", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_page_review_revisions r JOIN source_page_evidence p ON p.id = r.source_page_evidence_id WHERE p.source_file_id = $1 AND p.page_number = 1`, file.ID); got != 3 {
		t.Fatalf("stored page review revisions = %d, want all three append-only revisions", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'source_page_transcription_reviewed'`); got != 3 {
		t.Fatalf("transcription review audit rows = %d, want 3", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'source_page_transcription_reviewed' AND after_value::text LIKE '%' || $1 || '%'`, "النسب بعد المراجعة"); got != 0 {
		t.Fatalf("audit log contains raw transcription text in %d row(s)", got)
	}
	var originalMachineText string
	if err := fixture.QueryRow(`SELECT text_ar FROM source_page_evidence WHERE id = $1`, page.PageEvidenceID).Scan(&originalMachineText); err != nil {
		t.Fatalf("read original page snapshot: %v", err)
	}
	if originalMachineText != "النص الآلي" {
		t.Fatalf("original machine reading changed to %q after a retry snapshot", originalMachineText)
	}
	index, err := service.PageEvidenceIndex(ctx, file.ID, owner.User.ID)
	if err != nil || len(index.Pages) != 1 || index.Pages[0].ReviewCount != 3 {
		t.Fatalf("page evidence index = %+v, %v; want all three page revisions visible", index, err)
	}
}
