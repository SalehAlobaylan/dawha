package sourceprocessing

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"testing"
)

// encodeTestRaster builds a real PNG of the requested size, with a light page
// and a dark mark, so the image arm is exercised on bytes rather than a
// hypothetical decode.
func encodeTestRaster(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, color.RGBA{R: 246, G: 244, B: 238, A: 255})
		}
	}
	for y := height / 3; y < height/3+height/20; y++ {
		for x := width / 5; x < width/2; x++ {
			canvas.Set(x, y, color.RGBA{R: 25, G: 25, B: 25, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("could not encode the raster fixture: %v", err)
	}
	return buffer.Bytes()
}

func TestInspectImageSourceMeasuresThePage(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		inspection, ok := inspectImageSource(testImageBytes(t, format))
		if !ok {
			t.Fatalf("a real %s image was refused", format)
		}
		if inspection.Width != 16 || inspection.Height != 12 {
			t.Fatalf("%s dimensions = %dx%d, want 16x12", format, inspection.Width, inspection.Height)
		}
	}
	if _, ok := inspectImageSource([]byte("not an image")); ok {
		t.Fatal("text was accepted as an image")
	}
	if _, ok := inspectImageSource(pngHeaderWithSize(t, 9000, 9000)); ok {
		t.Fatal("an 81-megapixel image passed the source bound")
	}
}

func TestRenderImagePageBoundsTheLongestSide(t *testing.T) {
	content := encodeTestRaster(t, 400, 800)
	rendered, inspection, err := renderImagePage(content, 200)
	if err != nil {
		t.Fatalf("renderImagePage: %v", err)
	}
	if inspection.Width != 400 || inspection.Height != 800 {
		t.Fatalf("the source dimensions changed: %+v", inspection)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(rendered))
	if err != nil {
		t.Fatalf("the rendered page is not an image: %v", err)
	}
	if config.Width != 100 || config.Height != 200 {
		t.Fatalf("rendered page = %dx%d, want 100x200", config.Width, config.Height)
	}
}

// A raster document is one page. It has no native text layer, so extraction
// either reads the page with Tesseract or keeps the page for a human to
// transcribe - but it never reports a missing page as a successful empty one.
func TestImageExtractionProducesOneReviewablePage(t *testing.T) {
	content := encodeTestRaster(t, 240, 160)
	extractor := &PDFExtractor{Tesseract: ""}
	pages, err := extractor.Extract(context.Background(), ExtractInput{
		Reader:      bytes.NewReader(content),
		ContentType: "image/png",
		Filename:    "شجرة.png",
	})
	if err != nil {
		t.Fatalf("image extraction: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected one page, got %d", len(pages))
	}
	page := pages[0]
	if page.Width != 240 || page.Height != 160 {
		t.Fatalf("page dimensions = %vx%v, want 240x160", page.Width, page.Height)
	}
	if len(page.Readings) != 1 || page.Readings[0].Method != "tesseract_unavailable" || !page.Readings[0].Selected {
		t.Fatalf("expected the page to be kept with a selected unavailable reading, got %+v", page.Readings)
	}
}

func TestImagePageCountAndPreviewComeFromTheImage(t *testing.T) {
	content := encodeTestRaster(t, 300, 600)
	extractor := &PDFExtractor{}
	if count := extractor.PageCount(context.Background(), content); count != 1 {
		t.Fatalf("PageCount = %d, want 1", count)
	}
	preview, err := extractor.RenderPage(context.Background(), content, 1)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(preview))
	if err != nil || config.Width < 1 || config.Height < 1 {
		t.Fatalf("the preview is not a decodable image: %v", err)
	}
	if _, err := extractor.RenderPage(context.Background(), content, 2); err == nil {
		t.Fatal("an image has no second page")
	}
}

func TestChartPageTilesARasterSource(t *testing.T) {
	content := encodeTestRaster(t, 200, 120)
	extractor := &PDFExtractor{}
	page, err := extractor.OpenChartPageForAnalysis(context.Background(), content, 1)
	if err != nil {
		t.Fatalf("OpenChartPageForAnalysis: %v", err)
	}
	defer page.Close()
	if page.Width != 200 || page.Height != 120 {
		t.Fatalf("page tile space = %dx%d, want 200x120", page.Width, page.Height)
	}
	tile, err := page.RenderRegion(context.Background(), ChartTileBounds{X: 10, Y: 10, Width: 40, Height: 30})
	if err != nil {
		t.Fatalf("RenderRegion: %v", err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(tile))
	if err != nil || config.Width != 40 || config.Height != 30 {
		t.Fatalf("tile = %dx%d, %v, want 40x30", config.Width, config.Height, err)
	}
	if _, err := page.RenderRegion(context.Background(), ChartTileBounds{X: 180, Y: 10, Width: 40, Height: 30}); err == nil {
		t.Fatal("a tile outside the page was accepted")
	}
}

func TestImageExtractionRunsTheSameReadersAsAPDF(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("Tesseract is not installed, so the shared reading path cannot be exercised")
	}
	content := encodeTestRaster(t, 600, 400)
	extractor := &PDFExtractor{Tesseract: "tesseract", CarrierLocaliser: DefaultCarrierLocaliserConfig()}
	pages, err := extractor.Extract(context.Background(), ExtractInput{
		Reader:      bytes.NewReader(content),
		ContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("image extraction: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected one page, got %d", len(pages))
	}
	var sawOCR, sawLocaliser bool
	for _, reading := range pages[0].Readings {
		if reading.Method == "tesseract_ocr" && reading.Selected {
			sawOCR = true
		}
		if reading.Method == carrierReadingMethod {
			sawLocaliser = true
		}
	}
	if !sawOCR {
		t.Fatalf("the image was not read by Tesseract: %+v", pages[0].Readings)
	}
	if !sawLocaliser {
		t.Fatalf("the localiser did not run on the image: %+v", pages[0].Readings)
	}
}
