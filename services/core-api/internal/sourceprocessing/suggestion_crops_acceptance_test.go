package sourceprocessing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
)

// TestASuggestionCropComesFromTheOriginalSource pins the contract that makes
// review possible: the crop is rendered from the source file itself, not from the
// bounded page preview. A thumbnail cannot settle a calligraphic name.
func TestASuggestionCropComesFromTheOriginalSource(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "قارئ قصاصات")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	sourceService := evidence.NewService(fixture.Pool())
	ctx := fixture.Ctx()
	source, err := sourceService.CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "شجرة قصاصات " + fixture.Unique("s"),
		SourceType: "family_document",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	file, err := uploader.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "tree.png",
		ContentType: "image/png",
		Content:     syntheticChartPNG(t),
	})
	if err != nil {
		t.Fatalf("upload source: %v", err)
	}
	service := NewService(fixture.Pool(), store, nil, nil, NewPDFExtractor())
	if err := service.CreateSuggestions(ctx, mustUUID(source.Source.ID), mustUUID(file.ID), nil, []SuggestionInput{{
		OccurrenceID:    "carrier-p001-000001",
		PageNumber:      1,
		SuggestedTextAR: "أحمد",
		Region:          Region{X: 200.0 / 600.0, Y: 150.0 / 400.0, Width: 80.0 / 600.0, Height: 40.0 / 400.0},
		Confidence:      0.6,
		ViewCount:       3,
		ReadingMethod:   "local_carrier_localiser",
		ReadingModel:    "carrier-vision-v1",
	}}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, file.ID, owner.User.ID, "", 0, 0)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("expected one suggestion, got %d", len(queue))
	}
	crop, err := service.GetSuggestionCrop(ctx, queue[0].ID, owner.User.ID)
	if err != nil {
		t.Fatalf("get suggestion crop: %v", err)
	}
	tightWidth, tightHeight := decodePNGSize(t, crop.TightPNG)
	contextWidth, contextHeight := decodePNGSize(t, crop.ContextPNG)
	if tightWidth == 0 || tightHeight == 0 || contextWidth == 0 || contextHeight == 0 {
		t.Fatalf("crop returned an empty image: tight=%dx%d context=%dx%d", tightWidth, tightHeight, contextWidth, contextHeight)
	}
	if tightWidth >= contextWidth || tightHeight >= contextHeight {
		// The source is 600x400 and the name is 80x40: the tight crop should be
		// a name and its padding, and the context crop should show neighbours.
		t.Fatalf("context crop %dx%d does not show more than tight crop %dx%d", contextWidth, contextHeight, tightWidth, tightHeight)
	}

	// Candidates are requested for the SAME tight crop the reviewer sees. The
	// model is a spelling aid here; it receives no source metadata and writes
	// nothing.
	provider := &candidateNameProvider{}
	service.ChartVision = provider
	service.LocalReadingProbe = func(context.Context) LocalReaderStatus {
		return LocalReaderStatus{Reason: "configured", Version: LocalCropReaderVersion}
	}
	capabilities, err := service.SourceReadingCapabilities(ctx, file.ID, owner.User.ID)
	if err != nil || capabilities.HostedReader.Ready || !capabilities.LocalReader.Ready {
		t.Fatalf("default capabilities=%+v error=%v", capabilities, err)
	}
	if _, err := service.SuggestionCandidates(ctx, queue[0].ID, owner.User.ID); !errors.Is(err, ErrChartVisionPolicy) || len(provider.tightPNG) > 0 {
		t.Fatalf("disabled reader was invoked: %v", err)
	}
	// Only this isolated fixture explicitly approves hosted processing. The fake
	// provider never leaves this process; private source metadata is untouched.
	if _, err := fixture.Pool().Exec(ctx, `UPDATE sources SET metadata=metadata || '{"hostedChartVisionApproved":true}'::jsonb WHERE id=$1`, source.Source.ID); err != nil {
		t.Fatal(err)
	}
	service.ReaderSettings = ReadingSettings{Mode: "hosted", HostedEnabled: true}
	service.ReaderStatus = &stubReadingStatus{status: HostedReaderStatus{Enabled: true, Configured: true, Reason: "configured", Version: ChartVisionModel}}
	service.readingCache.at = time.Time{}
	candidates, err := service.SuggestionCandidates(ctx, queue[0].ID, owner.User.ID)
	if err != nil {
		t.Fatalf("suggest candidates: %v", err)
	}
	if len(candidates) != 2 || candidates[0] != "أحمد" || candidates[1] != "محمد" {
		t.Fatalf("candidates were not cleaned: %+v", candidates)
	}
	if !bytes.Equal(provider.tightPNG, crop.TightPNG) {
		t.Fatalf("the model saw a different crop than the review screen")
	}

	// Another reviewer must not be able to read the original through this route.
	other := actor.RegisterAndLogin(t, fixture, "مراجع آخر")
	if _, err := service.SourceReadingCapabilities(ctx, file.ID, other.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("source capability access leaked: %v", err)
	}
	if _, err := service.GetSuggestionCrop(ctx, queue[0].ID, other.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a reviewer without source access got %v, want ErrForbidden", err)
	}
}

func TestTheJudgeSeesTheReviewCropAndFileContextButWritesNothing(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "قارئ رأي النموذج")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	sourceService := evidence.NewService(fixture.Pool())
	ctx := fixture.Ctx()
	source, err := sourceService.CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "شجرة حكم الأسماء " + fixture.Unique("s"),
		SourceType: "family_document",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	file, err := uploader.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "tree.png",
		ContentType: "image/png",
		Content:     syntheticChartPNG(t),
	})
	if err != nil {
		t.Fatalf("upload source: %v", err)
	}
	service := NewService(fixture.Pool(), store, nil, nil, NewPDFExtractor())
	fileID := mustUUID(file.ID)
	sourceID := mustUUID(source.Source.ID)
	if err := service.CreateSuggestions(ctx, sourceID, fileID, nil, []SuggestionInput{
		{
			OccurrenceID: "carrier-p001-000001", PageNumber: 1, SuggestedTextAR: "سالم",
			Region:     Region{X: 200.0 / 600.0, Y: 150.0 / 400.0, Width: 80.0 / 600.0, Height: 40.0 / 400.0},
			Confidence: 0.9, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1",
		},
		{
			OccurrenceID: "carrier-p001-000002", PageNumber: 1, SuggestedTextAR: "أحمد",
			Alternatives: []string{"أحمد", "محمد"},
			Region:       Region{X: 300.0 / 600.0, Y: 200.0 / 400.0, Width: 80.0 / 600.0, Height: 40.0 / 400.0},
			Confidence:   0.6, ReadingMethod: "local_carrier_localiser", ReadingModel: "carrier-vision-v1",
		},
	}); err != nil {
		t.Fatalf("create suggestions: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, file.ID, owner.User.ID, "", 0, 0)
	if err != nil || len(queue) != 2 {
		t.Fatalf("list suggestions: %v (%d)", err, len(queue))
	}
	targetIndex := 0
	if queue[0].SuggestedTextAR != "أحمد" {
		targetIndex = 1
	}
	target := queue[targetIndex]
	other := queue[1-targetIndex]
	if _, err := service.DecideSuggestion(ctx, other.ID, owner.User.ID, SuggestionDecision{
		Decision: SuggestionAccepted,
		TextAR:   other.SuggestedTextAR,
	}); err != nil {
		t.Fatalf("accept the context suggestion: %v", err)
	}
	crop, err := service.GetSuggestionCrop(ctx, target.ID, owner.User.ID)
	if err != nil {
		t.Fatalf("get crop: %v", err)
	}
	provider := &recordingNameJudge{}
	service.NameJudge = provider
	service.NameJudgeMode = "active"
	service.NameJudgeSample = 1
	service.NameJudgeImageEnabled = true
	var decisionsBefore int
	if err := fixture.QueryRow(`SELECT COUNT(*) FROM source_tree_suggestion_decisions`).Scan(&decisionsBefore); err != nil {
		t.Fatalf("count decisions before: %v", err)
	}
	result, err := service.JudgeSuggestion(ctx, target.ID, owner.User.ID, []JudgeCandidate{
		{Value: "أحمد", Source: "reader", Rank: 1},
		{Value: "محمد", Source: "common", Rank: 2},
	})
	if err != nil {
		t.Fatalf("judge suggestion: %v", err)
	}
	if result.Choice == nil || *result.Choice != "محمد" || result.Abstained {
		t.Fatalf("judge result = %+v", result)
	}
	if provider.request.State.Reading != "أحمد" || len(provider.request.State.Alternatives) != 2 {
		t.Fatalf("judge state = %+v", provider.request.State)
	}
	if len(provider.request.State.FileContext) == 0 || provider.request.State.FileContext[0] != "سالم" {
		t.Fatalf("file context = %+v", provider.request.State.FileContext)
	}
	tightWidth, tightHeight := decodePNGSize(t, crop.TightPNG)
	if provider.request.State.Crop.WidthPx != tightWidth || provider.request.State.Crop.HeightPx != tightHeight {
		t.Fatalf("crop measurements = %+v, rendered tight crop = %dx%d", provider.request.State.Crop, tightWidth, tightHeight)
	}
	if !bytes.Equal(provider.request.ImagePNG, crop.TightPNG) {
		t.Fatal("the judge saw a crop different from the review screen")
	}
	var decisionsAfter int
	if err := fixture.QueryRow(`SELECT COUNT(*) FROM source_tree_suggestion_decisions`).Scan(&decisionsAfter); err != nil {
		t.Fatalf("count decisions after: %v", err)
	}
	if decisionsAfter != decisionsBefore {
		t.Fatalf("the judge wrote a decision: %d -> %d", decisionsBefore, decisionsAfter)
	}
	if _, err := service.JudgeSuggestion(ctx, target.ID, actor.RegisterAndLogin(t, fixture, "مراجع بلا وصول").User.ID, []JudgeCandidate{
		{Value: "أحمد", Source: "reader", Rank: 1},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unauthorized judge = %v, want ErrForbidden", err)
	}

	service.NameJudgeImageEnabled = false
	if _, err := service.JudgeSuggestion(ctx, target.ID, owner.User.ID, []JudgeCandidate{
		{Value: "أحمد", Source: "reader", Rank: 1},
		{Value: "محمد", Source: "common", Rank: 2},
	}); err != nil {
		t.Fatalf("text-only judge: %v", err)
	}
	if len(provider.request.ImagePNG) != 0 {
		t.Fatal("the judge received an image while image mode was off")
	}
}

type recordingNameJudge struct {
	request NameJudgeRequest
}

func (j *recordingNameJudge) Judge(_ context.Context, request NameJudgeRequest) (NameJudgeResult, error) {
	j.request = request
	choice := "محمد"
	return NameJudgeResult{
		Choice:        &choice,
		Probabilities: map[string]float64{"أحمد": 0.2, "محمد": 0.7, "unclear": 0.1},
		Confidence:    0.7,
		Provider:      "jev",
		Model:         "jev-pinned",
		PromptVersion: "name-judge-v1",
	}, nil
}

type candidateNameProvider struct {
	tightPNG []byte
}

func (p *candidateNameProvider) LocateTile(context.Context, string, []byte) (ChartTileReading, error) {
	return ChartTileReading{}, nil
}

func (p *candidateNameProvider) ReadBatch(context.Context, []ChartCropInput) ([]ChartNameReading, error) {
	return nil, nil
}

func (p *candidateNameProvider) SuggestNames(_ context.Context, tightPNG []byte) ([]string, error) {
	p.tightPNG = append([]byte(nil), tightPNG...)
	return []string{"أحمد", "محمد", "أحمد", " "}, nil
}

func syntheticChartPNG(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 600, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			canvas.Set(x, y, color.RGBA{R: 248, G: 242, B: 228, A: 255})
		}
	}
	for y := 155; y < 185; y++ {
		for x := 210; x < 270; x++ {
			canvas.Set(x, y, color.RGBA{R: 35, G: 30, B: 25, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("encode chart fixture: %v", err)
	}
	return buffer.Bytes()
}

func decodePNGSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode crop png: %v", err)
	}
	return config.Width, config.Height
}
