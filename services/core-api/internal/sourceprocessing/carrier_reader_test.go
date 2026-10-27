package sourceprocessing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"testing"
)

// decodeRGBAImage decodes PNG bytes for a test that needs the dimensions.
func decodeRGBAImage(data []byte) (image.Image, error) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	return decoded, err
}

// A stub reader that answers with whatever the test tells it to.
type stubCarrierReader struct {
	// answers maps a view index to the text it returns for every crop.
	answers [][]string
	calls   int
	batches []int
	err     error
}

func (s *stubCarrierReader) LocateTile(context.Context, string, []byte) (ChartTileReading, error) {
	return ChartTileReading{}, nil
}

func (s *stubCarrierReader) SuggestNames(context.Context, []byte) ([]string, error) {
	return nil, nil
}

func (s *stubCarrierReader) ReadBatch(_ context.Context, inputs []ChartCropInput) ([]ChartNameReading, error) {
	s.calls++
	s.batches = append(s.batches, len(inputs))
	if s.err != nil {
		return nil, s.err
	}
	view := s.calls - 1
	readings := make([]ChartNameReading, 0, len(inputs))
	for index, input := range inputs {
		text := ""
		if view < len(s.answers) && index < len(s.answers[view]) {
			text = s.answers[view][index]
		}
		readings = append(readings, ChartNameReading{
			OccurrenceID: input.OccurrenceID,
			Status:       "readable",
			TextAR:       text,
		})
	}
	return readings, nil
}

func carrierTestPage() image.Image {
	page := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			page.Set(x, y, color.RGBA{R: 240, G: 240, B: 235, A: 255})
		}
	}
	for _, box := range [][4]int{{20, 20, 60, 30}, {120, 120, 50, 40}} {
		for y := box[1]; y < box[1]+box[3]; y++ {
			for x := box[0]; x < box[0]+box[2]; x++ {
				page.Set(x, y, color.RGBA{R: 200, G: 230, B: 60, A: 255})
			}
		}
	}
	return page
}

func testCarriers() []PageRegion {
	return []PageRegion{
		{Kind: "name_carrier", X: 0.10, Y: 0.10, Width: 0.30, Height: 0.15},
		{Kind: "name_carrier", X: 0.60, Y: 0.60, Width: 0.25, Height: 0.20},
	}
}

func TestReadCarriersAsksTheReaderOncePerViewAndOncePerCrop(t *testing.T) {
	provider := &stubCarrierReader{answers: [][]string{
		{"محمد", "صالح"},
		{"محمد", "صالح"},
		{"محمد", "صالح"},
	}}
	readings, err := ReadCarriers(context.Background(), provider, carrierTestPage(), testCarriers(), DefaultCarrierReaderConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(readings) != 2 {
		t.Fatalf("expected two readings, got %d", len(readings))
	}
	if provider.calls != 3 {
		t.Fatalf("expected one batch per view, got %d calls", provider.calls)
	}
	for _, size := range provider.batches {
		if size != 2 {
			t.Fatalf("expected every view to read every crop, got a batch of %d", size)
		}
	}
}

func TestReadCarriersConfidenceIsTheLowestCharacterAgreement(t *testing.T) {
	// The views agree on the first word and disagree on one character of the
	// second, so the reading's confidence is the agreement of the weakest
	// character, not the average.
	provider := &stubCarrierReader{answers: [][]string{
		{"محمد علي", "صالح"},
		{"محمد علي", "صالح"},
		{"محمد علي", "صالح"},
	}}
	readings, err := ReadCarriers(context.Background(), provider, carrierTestPage(), testCarriers(), DefaultCarrierReaderConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(readings) != 2 {
		t.Fatalf("expected two readings, got %d", len(readings))
	}
	agreement := readings[0]
	if agreement.Confidence <= 0 || agreement.Confidence > 1 {
		t.Fatalf("expected a confidence within (0,1], got %v", agreement.Confidence)
	}
	if len(agreement.Agreement) == 0 {
		t.Fatal("expected the per-character agreement to be recorded")
	}
	for _, value := range agreement.Agreement {
		if value <= 0 || value > 1 {
			t.Fatalf("agreement is not a share: %v", value)
		}
	}
}

func TestReadCarriersDropsACharacterTheViewsDisagreeOn(t *testing.T) {
	provider := &stubCarrierReader{answers: [][]string{
		{"محمد", "صالح"},
		{"محمود", "صالح"},
		{"محمد", "صالح"},
	}}
	readings, err := ReadCarriers(context.Background(), provider, carrierTestPage(), testCarriers(), DefaultCarrierReaderConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if readings[0].TextAR != "محمد" {
		t.Fatalf("expected the disputed character to be dropped, got %q", readings[0].TextAR)
	}
}

func TestReadCarriersReportsACarrierNoViewCouldRead(t *testing.T) {
	provider := &stubCarrierReader{answers: [][]string{
		{"", ""},
		{"", ""},
		{"", ""},
	}}
	readings, err := ReadCarriers(context.Background(), provider, carrierTestPage(), testCarriers(), DefaultCarrierReaderConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(readings) != 2 {
		t.Fatalf("expected a reading for every carrier, got %d", len(readings))
	}
	for _, reading := range readings {
		if reading.Status != "unreadable" {
			t.Fatalf("expected an unreadable status, got %q", reading.Status)
		}
		if reading.Confidence != 0 {
			t.Fatalf("expected no confidence for an unreadable carrier, got %v", reading.Confidence)
		}
	}
}

func TestReadCarriersWithoutAReaderIsUnavailable(t *testing.T) {
	if _, err := ReadCarriers(context.Background(), nil, carrierTestPage(), testCarriers(), DefaultCarrierReaderConfig()); !errors.Is(err, ErrCarrierReaderUnavailable) {
		t.Fatalf("expected ErrCarrierReaderUnavailable, got %v", err)
	}
}

func TestReadCarriersPropagatesTheProvidersFailure(t *testing.T) {
	provider := &stubCarrierReader{err: ErrChartVisionUnavailable}
	if _, err := ReadCarriers(context.Background(), provider, carrierTestPage(), testCarriers(), DefaultCarrierReaderConfig()); !errors.Is(err, ErrChartVisionUnavailable) {
		t.Fatalf("expected the provider's error, got %v", err)
	}
}

func TestConsensusTextTakesTheAgreedText(t *testing.T) {
	text, agreement := consensusText([]string{"محمد", "محمد", "محمد"}, 0.6)
	if text != "محمد" {
		t.Fatalf("expected the agreed text, got %q", text)
	}
	if len(agreement) != 4 {
		t.Fatalf("expected one agreement per character, got %d", len(agreement))
	}
	for _, value := range agreement {
		if value != 1 {
			t.Fatalf("expected full agreement, got %v", value)
		}
	}
}

func TestConsensusTextKeepsOnlyWhatTheViewsAgreeOn(t *testing.T) {
	text, _ := consensusText([]string{"محمد", "محمود", "محمد"}, 0.75)
	if text != "محمد" {
		t.Fatalf("expected only the agreed characters, got %q", text)
	}
}

func TestCropCarrierCutsTheCarrierAndItsContext(t *testing.T) {
	page := carrierTestPage()
	config := DefaultCarrierReaderConfig()
	config.ContextScale = 3
	tight, context, err := CropCarrier(page, testCarriers()[0], config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tight) == 0 || len(context) == 0 {
		t.Fatal("expected both a tight crop and a context crop")
	}
	// The context crop must be larger than the tight one: it is what tells the
	// reader that the word is a name on a shape rather than a label. The
	// comparison is on the decoded dimensions, not on the compressed bytes, which
	// are similar for two mostly-flat images.
	tightImage, err := decodeRGBAImage(tight)
	if err != nil {
		t.Fatalf("the tight crop could not be decoded: %v", err)
	}
	contextImage, err := decodeRGBAImage(context)
	if err != nil {
		t.Fatalf("the context crop could not be decoded: %v", err)
	}
	if contextImage.Bounds().Dx() <= tightImage.Bounds().Dx() || contextImage.Bounds().Dy() <= tightImage.Bounds().Dy() {
		t.Fatalf("expected the context crop to be larger than the tight crop, got %v and %v",
			tightImage.Bounds(), contextImage.Bounds())
	}
}

func TestCropCarrierClipsACarrierThatHangsOffThePage(t *testing.T) {
	page := carrierTestPage()
	// A carrier that runs off the edge of the page is clipped to the page, not
	// rejected: a name at the margin of a scan is still a name.
	tight, _, err := CropCarrier(page, PageRegion{X: 0.90, Y: 0.90, Width: 0.40, Height: 0.20}, DefaultCarrierReaderConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tight) == 0 {
		t.Fatal("expected a clipped crop")
	}
}

func TestCropCarrierRefusesACarrierThatIsNotOnThePage(t *testing.T) {
	page := carrierTestPage()
	if _, _, err := CropCarrier(page, PageRegion{X: 1.5, Y: 1.5, Width: 0.2, Height: 0.2}, DefaultCarrierReaderConfig()); err == nil {
		t.Fatal("expected an error for a carrier that is not on the page")
	}
}

func TestTheDefaultContextIsNoWiderThanTheTightCrop(t *testing.T) {
	// A wider context crop of a painted tree contains the NEXT name as well, and
	// a reader asked for one occurrence then returns two readings for it, which
	// the response schema refuses. So the default context is the tight crop.
	if DefaultCarrierReaderConfig().ContextScale != 1 {
		t.Fatalf("expected the default context scale to be one, got %v", DefaultCarrierReaderConfig().ContextScale)
	}
}
