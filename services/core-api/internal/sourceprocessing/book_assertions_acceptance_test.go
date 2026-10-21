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

func TestBookAssertionKeepsExactArabicQuoteAndOnlyCreatesReviewedSourceStatement(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "مراجع كتاب")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{TitleAR: "كتاب أنساب " + fixture.Unique("b"), SourceType: "book"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := uploader.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{Filename: "book.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7\\nfixture")})
	if err != nil {
		t.Fatalf("upload source: %v", err)
	}
	var runID string
	if err := fixture.QueryRow(`SELECT id::text FROM source_processing_runs WHERE source_file_id = $1`, file.ID).Scan(&runID); err != nil {
		t.Fatalf("find run: %v", err)
	}
	fixture.Exec(`INSERT INTO source_page_evidence (run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count) VALUES ($1, $2, $3, 1, 100, 200, 'readable', 'pdf_text', 'قال النص', '[]'::jsonb, '[]'::jsonb, 0)`, runID, source.Source.ID, file.ID)
	var pageID string
	if err := fixture.QueryRow(`SELECT id::text FROM source_page_evidence WHERE source_file_id = $1 AND page_number = 1`, file.ID).Scan(&pageID); err != nil {
		t.Fatalf("find page evidence: %v", err)
	}
	service := NewService(fixture.Pool(), nil, nil, nil, nil)
	review, err := service.SavePageReview(ctx, file.ID, owner.User.ID, 1, PageReviewInput{ExpectedPageEvidenceID: pageID, Status: "partial", TextAR: "قال النص إن سالم بن أحمد والد ناصر"})
	if err != nil {
		t.Fatalf("save transcription: %v", err)
	}
	quote := "سالم بن أحمد والد ناصر"
	start := len([]rune("قال النص إن "))
	input := BookAssertionInput{PageEvidenceID: pageID, PageReviewRevisionID: review.ID, ExpectedReviewRevision: review.RevisionNumber, QuoteStartCodepoint: start, QuoteEndCodepoint: start + len([]rune(quote)), QuoteTextAR: quote, SubjectMentionAR: "سالم بن أحمد", Predicate: "parent_of", ObjectMentionAR: "ناصر", PrintedPageLabel: "١٢", AttributionAR: "المؤلف", Polarity: "affirmed", Certainty: "reported", NoteAR: "نص مطبوع جزئياً"}
	wrong := input
	wrong.QuoteEndCodepoint++
	if _, err := service.CreateBookAssertion(ctx, file.ID, owner.User.ID, wrong); !errors.Is(err, ErrValidation) {
		t.Fatalf("mismatched span = %v, want validation error", err)
	}
	assertion, err := service.CreateBookAssertion(ctx, file.ID, owner.User.ID, input)
	if err != nil {
		t.Fatalf("create assertion: %v", err)
	}
	if assertion.State != "draft" || assertion.OffsetUnit != "unicode_codepoints_half_open" || assertion.QuoteTextAR != quote || assertion.SourceStatementID != "" {
		t.Fatalf("created assertion = %+v; want staged exact quote", assertion)
	}
	updatedInput := input
	updatedInput.ExpectedReviewRevision = review.RevisionNumber
	updatedInput.Predicate = "reported_relationship"
	updated, err := service.UpdateBookAssertion(ctx, assertion.ID, owner.User.ID, UpdateBookAssertionInput{ExpectedRevision: assertion.Revision, BookAssertionInput: updatedInput})
	if err != nil {
		t.Fatalf("revise draft assertion: %v", err)
	}
	if updated.Revision != 2 || updated.Predicate != "reported_relationship" {
		t.Fatalf("updated assertion = %+v", updated)
	}
	if _, err := service.UpdateBookAssertion(ctx, assertion.ID, owner.User.ID, UpdateBookAssertionInput{ExpectedRevision: assertion.Revision, BookAssertionInput: updatedInput}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale assertion update = %v, want conflict", err)
	}
	accepted, err := service.ReviewBookAssertion(ctx, assertion.ID, owner.User.ID, ReviewBookAssertionInput{ExpectedRevision: updated.Revision, Decision: "reviewed"})
	if err != nil {
		t.Fatalf("review assertion: %v", err)
	}
	if accepted.State != "reviewed" || accepted.SourceStatementID == "" {
		t.Fatalf("review result = %+v; want linked source statement", accepted)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_statements WHERE id = $1 AND source_page_evidence_id = $2 AND source_page_review_revision_id = $3 AND statement_text_ar = $4 AND review_status = 'accepted'`, accepted.SourceStatementID, pageID, review.ID, quote); got != 1 {
		t.Fatalf("grounded accepted statements = %d, want one", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM claims WHERE created_by = $1`, owner.User.ID); got != 0 {
		t.Fatalf("book extraction created %d canonical claims, want none", got)
	}
	listed, err := service.ListBookAssertions(ctx, file.ID, owner.User.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != accepted.ID {
		t.Fatalf("list assertions = %+v, %v", listed, err)
	}
}

func TestBookAssertionRejectsNonBookSource(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "مراجع وثيقة")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{TitleAR: "وثيقة عائلية " + fixture.Unique("b"), SourceType: "family_document"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := uploader.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{Filename: "family.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7\\nfixture")})
	if err != nil {
		t.Fatalf("upload source: %v", err)
	}
	service := NewService(fixture.Pool(), nil, nil, nil, nil)
	if _, err := service.CreateBookAssertion(ctx, file.ID, owner.User.ID, BookAssertionInput{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("book workflow for family chart = %v, want validation error", err)
	}
}
