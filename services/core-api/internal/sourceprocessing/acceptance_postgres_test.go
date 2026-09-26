package sourceprocessing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
)

// The source journey a researcher actually walks: attach a text file, let the
// worker read it, and then decide what the model proposed. Every step runs
// against the isolated fixture schema with a local store and a stub provider, so
// the outcome is the same on every machine and no external service is needed.
func TestUploadProcessAndReviewATextSource(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "رافع المصدر")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	queue := jobs.NewService(fixture.Pool())
	service := NewService(fixture.Pool(), store, queue, newStubProvider(), NewTextExtractor())
	ctx := fixture.Ctx()

	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "مخطوط " + fixture.Unique("s"),
		SourceType: "manuscript",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	// The format contract from the source-format plan: a UTF-8 text file is
	// accepted, and the page separator is the form-feed character.
	content := []byte("قال ابن سعد: أبو بكر هو والد عبدالله.\fوفي الصفحة الثانية: هاجر سعد إلى الرياض.")
	file, err := service.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "سجل.txt",
		ContentType: "text/plain",
		Content:     content,
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if file.ProcessingStatus != "queued" {
		t.Fatalf("uploaded file status = %s, want queued", file.ProcessingStatus)
	}
	if file.ChecksumSHA256 == "" || file.ByteSize != int64(len(content)) {
		t.Fatalf("uploaded file = %d bytes / %q, want the size and digest of what was sent", file.ByteSize, file.ChecksumSHA256)
	}
	// The upload enqueued exactly one unit of work, keyed by the file, so a
	// retried upload request cannot double-process the document.
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE type = $1 AND payload->>'source_file_id' = $2`,
		SourceProcessJobType, file.ID); got != 1 {
		t.Fatalf("queued processing jobs = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_processing_runs WHERE source_file_id = $1 AND status = 'queued'`, file.ID); got != 1 {
		t.Fatalf("queued processing runs = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'source_file_uploaded' AND entity_id = $1`, file.ID); got != 1 {
		t.Fatalf("upload audit rows = %d, want 1", got)
	}

	job, err := queue.Claim(ctx, jobs.ClaimInput{WorkerID: "p004-source-worker", Type: SourceProcessJobType})
	if err != nil {
		t.Fatalf("claim the processing job: %v", err)
	}
	claim := Claim{Job: job, Lease: job.Lease("p004-source-worker")}
	if err := service.Process(ctx, claim); err != nil {
		t.Fatalf("process: %v", err)
	}
	if _, err := queue.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: "p004-source-worker", LeaseToken: claim.Lease.Token}); err != nil {
		t.Fatalf("complete the processing job: %v", err)
	}

	// The extracted text is stored with its page provenance, which is what makes
	// a later quotation checkable.
	passages := fixture.Count(`SELECT count(*) FROM source_passages WHERE source_file_id = $1`, file.ID)
	if passages != 2 {
		t.Fatalf("extracted passages = %d, want one per page", passages)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE source_file_id = $1 AND embedding IS NOT NULL`, file.ID); got != 2 {
		t.Fatalf("passages with an embedding = %d, want 2", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_processing_runs WHERE source_file_id = $1 AND status = 'succeeded' AND stage = 'complete'`, file.ID); got != 1 {
		t.Fatalf("succeeded runs = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_files WHERE id = $1 AND processing_status = 'succeeded'`, file.ID); got != 1 {
		t.Fatalf("succeeded files = %d, want 1", got)
	}
	// Processing the same job again is a no-op rather than a second extraction.
	if err := service.Process(ctx, claim); err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE source_file_id = $1`, file.ID); got != 2 {
		t.Fatalf("passages after a reprocess = %d, want 2", got)
	}

	processing, err := service.GetProcessing(ctx, source.Source.ID, owner.User.ID)
	if err != nil {
		t.Fatalf("read processing: %v", err)
	}
	if len(processing.Runs) != 1 || processing.Runs[0].Status != "succeeded" {
		t.Fatalf("processing runs = %+v, want one succeeded run", processing.Runs)
	}
	if len(processing.Candidates) == 0 {
		t.Fatal("the run recorded no candidate for the reviewer to decide on")
	}
	// Nothing the model proposed has been decided: every candidate is still
	// waiting for a reviewer, and the claim it extracted is explicitly queued.
	for _, item := range processing.Candidates {
		if item.Status == "accepted" || item.Status == "rejected" {
			t.Fatalf("candidate %s arrived already decided as %s: the model never decides", item.ID, item.Status)
		}
		if item.CandidateType == "claim" && item.Status != "needs_review" {
			t.Fatalf("claim candidate status = %s, want needs_review", item.Status)
		}
	}
	candidate := processing.Candidates[0]

	// A plain registrant cannot review candidates; that is a platform grant.
	intruder := actor.Register(t, fixture, "باحث بلا صلاحية")
	if _, err := service.ReviewCandidate(ctx, candidate.ID, intruder.User.ID, ReviewInput{Decision: "accepted"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("review by an ungranted researcher = %v, want ErrForbidden", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_candidate_reviews WHERE candidate_id = $1`, candidate.ID); got != 0 {
		t.Fatalf("reviews recorded by a refused reviewer = %d, want 0", got)
	}

	// Reviewing candidates is a platform grant on top of owning the source, so
	// the reviewer needs the researcher role before the decision is accepted.
	fixture.GrantRole(owner.User.ID, "researcher")
	accepted, err := service.ReviewCandidate(ctx, candidate.ID, owner.User.ID, ReviewInput{
		Decision: "accepted",
		NoteAR:   "مطابق للسجل الممسوح " + fixture.Tag(),
	})
	if err != nil {
		t.Fatalf("review candidate: %v", err)
	}
	if accepted.Status != "accepted" {
		t.Fatalf("reviewed candidate status = %s, want accepted", accepted.Status)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_candidate_reviews WHERE candidate_id = $1 AND decision = 'accepted'`, candidate.ID); got != 1 {
		t.Fatalf("accepted review rows = %d, want 1", got)
	}
	// A decision is final: re-reviewing would rewrite a record a reader may
	// already have cited.
	if _, err := service.ReviewCandidate(ctx, candidate.ID, owner.User.ID, ReviewInput{Decision: "rejected"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second review = %v, want ErrConflict", err)
	}
}

func TestUploadRefusesAnUnsupportedFormatWithTheAcceptedList(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "رافع المصدر")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	service := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), newStubProvider(), NewTextExtractor())
	ctx := fixture.Ctx()

	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "مخطوط " + fixture.Unique("s"),
		SourceType: "manuscript",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	_, err = service.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "سجل.pdf",
		ContentType: "application/pdf",
		Content:     []byte("%PDF-1.7\nbinary"),
	})
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("pdf upload = %v, want ErrUnsupportedContent", err)
	}
	// The refusal names the format it refused and the ones it accepts, so the
	// caller can act on it without reading the source.
	for _, fragment := range []string{"application/pdf", "text/*", "application/json", "application/xml"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("refusal %q does not mention %q", err.Error(), fragment)
		}
	}
	if got := fixture.Count(`SELECT count(*) FROM source_files WHERE source_id = $1`, source.Source.ID); got != 0 {
		t.Fatalf("a refused upload still stored %d file row(s)", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM jobs WHERE type = $1`, SourceProcessJobType); got != 0 {
		t.Fatalf("a refused upload queued %d job(s)", got)
	}
}

func TestUploadRefusesAnActorWhoDoesNotOwnTheSource(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب المصدر")
	stranger := actor.Register(t, fixture, "رافع آخر")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	service := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), newStubProvider(), NewTextExtractor())
	ctx := fixture.Ctx()

	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "مخطوط " + fixture.Unique("s"),
		SourceType: "manuscript",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := service.Upload(ctx, source.Source.ID, stranger.User.ID, UploadInput{
		Filename:    "سجل.txt",
		ContentType: "text/plain",
		Content:     []byte("نص"),
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("upload by a stranger = %v, want ErrForbidden", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_files WHERE source_id = $1`, source.Source.ID); got != 0 {
		t.Fatalf("a refused upload still stored %d file row(s)", got)
	}
}

// stubProvider answers the four calls the processing pipeline makes with fixed,
// reviewable values. It stands in for the deterministic ai-research service so
// this test needs no network and no credentials, and so the extracted result is
// the same on every run.
type stubProvider struct{}

func newStubProvider() ai.Provider { return stubProvider{} }

func (stubProvider) NormalizeName(_ context.Context, value string) (ai.NameNormalization, error) {
	return ai.NameNormalization{Original: value, Normalized: value, Method: "deterministic_arabic_normalization", PreservesOriginal: true}, nil
}

func (stubProvider) Embed(_ context.Context, request ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	values := make([]float64, request.Dimensions)
	for index := range values {
		values[index] = float64(index+1) / float64(request.Dimensions)
	}
	return ai.EmbeddingResponse{Embedding: values, Dimensions: len(values), Model: "p004-stub", Deterministic: true}, nil
}

func (stubProvider) Classify(context.Context, ai.ClassificationRequest) (ai.ClassificationResponse, error) {
	return ai.ClassificationResponse{Model: "p004-stub", ReviewRequired: true}, nil
}

func (stubProvider) ExtractEntities(_ context.Context, request ai.ExtractionRequest) (ai.EntityExtractionResponse, error) {
	return ai.EntityExtractionResponse{
		Model:          "p004-stub",
		ReviewRequired: true,
		Entities: []ai.EntityCandidate{{
			Text:       "عبد الله",
			EntityType: "person",
			Confidence: 0.55,
			Status:     "unreviewed",
			Rationale:  "مرشّح لغوي يحتاج مراجعة.",
		}},
	}, nil
}

func (stubProvider) ExtractClaims(_ context.Context, request ai.ExtractionRequest) (ai.ClaimExtractionResponse, error) {
	return ai.ClaimExtractionResponse{
		Model:          "p004-stub",
		ReviewRequired: true,
		Claims: []ai.ClaimCandidate{{
			SubjectText: "أبو بكر",
			Predicate:   "أبو",
			ObjectText:  "عبد الله",
			Confidence:  0.45,
			Status:      "needs_review",
			Rationale:   "علاقة مستخرجة من مؤشرات لغوية.",
		}},
	}, nil
}

func (stubProvider) ResolveEntity(_ context.Context, request ai.EntityResolutionRequest) (ai.EntityResolutionResponse, error) {
	matches := make([]ai.EntityMatch, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		matches = append(matches, ai.EntityMatch{
			CandidateID:   candidate.ID,
			CandidateName: candidate.Name,
			Score:         0,
			MatchedOn:     "none",
		})
	}
	return ai.EntityResolutionResponse{Matches: matches, Model: "p004-stub", ReviewRequired: true}, nil
}

func (stubProvider) AnalyzeContradiction(context.Context, ai.ContradictionRequest) (ai.ContradictionResponse, error) {
	return ai.ContradictionResponse{Model: "p004-stub", ReviewRequired: true}, nil
}

func (stubProvider) Rerank(_ context.Context, request ai.RerankRequest) (ai.RerankResponse, error) {
	documents := make([]ai.RerankedDocument, 0, len(request.Documents))
	for _, document := range request.Documents {
		documents = append(documents, ai.RerankedDocument{ID: document.ID, Score: 0, Excerpt: document.Text})
	}
	return ai.RerankResponse{Documents: documents, Model: "p004-stub"}, nil
}

func (stubProvider) ResearchQuery(context.Context, ai.ResearchQueryRequest) (ai.ResearchQueryResponse, error) {
	return ai.ResearchQueryResponse{Answer: "صياغة أولية.", Model: "p004-stub", ReviewRequired: true}, nil
}
