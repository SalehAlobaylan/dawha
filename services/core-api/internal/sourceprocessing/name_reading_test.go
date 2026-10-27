package sourceprocessing

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// chartNameFixture builds a synthetic chart large enough to require source
// tiles. Each name is a dark bar on a saturated shape, which is the same two
// signals the localiser uses on a real chart.
func chartNameFixture(t *testing.T) *image.RGBA {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 4200, 4000))
	for y := 0; y < 4000; y++ {
		for x := 0; x < 4200; x++ {
			canvas.Set(x, y, color.RGBA{R: 250, G: 248, B: 242, A: 255})
		}
	}
	type point struct{ x, y int }
	points := []point{
		{500, 400},
		{1670, 800},  // centre just inside the second ownership column
		{3330, 1200}, // centre just inside the third ownership column
		{1000, 1660},
		{2500, 1680},
		{3800, 1700},
		{700, 3335}, // centre just inside the third ownership row
		{1800, 3400},
		{3600, 3600},
		{4140, 3900}, // against the right and bottom page edge
	}
	for _, p := range points {
		for y := p.y - 30; y < p.y+30; y++ {
			for x := p.x - 100; x < p.x+100; x++ {
				canvas.Set(x, y, color.RGBA{R: 224, G: 130, B: 170, A: 255})
			}
		}
		for y := p.y - 12; y < p.y+12; y++ {
			for x := p.x - 60; x < p.x+60; x++ {
				canvas.Set(x, y, color.RGBA{R: 18, G: 18, B: 18, A: 255})
			}
		}
	}
	return canvas
}

type recordingNameProvider struct {
	inputs [][]byte
}

func (p *recordingNameProvider) LocateTile(context.Context, string, []byte) (ChartTileReading, error) {
	return ChartTileReading{}, nil
}

func (p *recordingNameProvider) SuggestNames(context.Context, []byte) ([]string, error) {
	return nil, nil
}

func (p *recordingNameProvider) ReadBatch(_ context.Context, inputs []ChartCropInput) ([]ChartNameReading, error) {
	readings := make([]ChartNameReading, 0, len(inputs))
	for _, input := range inputs {
		p.inputs = append(p.inputs, input.TightPNG)
		readings = append(readings, ChartNameReading{
			OccurrenceID: input.OccurrenceID,
			Status:       "readable",
			TextAR:       "اسم",
		})
	}
	return readings, nil
}

func TestPlanNameReadingTilesPartitionsThePage(t *testing.T) {
	pages := [][2]int{{100, 100}, {1920, 1920}, {1921, 1000}, {5000, 5000}, {14174, 9922}, {2176, 2176}}
	for _, size := range pages {
		width, height := size[0], size[1]
		tiles := planNameReadingTiles(width, height)
		if len(tiles) == 0 {
			t.Fatalf("page %dx%d produced no tiles", width, height)
		}
		ownershipArea := 0
		for _, tile := range tiles {
			own := tile.Ownership
			render := tile.Render
			if own.Width < 1 || own.Height < 1 {
				t.Fatalf("page %dx%d has an empty ownership strip: %+v", width, height, own)
			}
			if render.X < 0 || render.Y < 0 || render.X+render.Width > width || render.Y+render.Height > height {
				t.Fatalf("page %dx%d has a render outside the page: %+v", width, height, render)
			}
			if render.X > own.X || render.Y > own.Y || render.X+render.Width < own.X+own.Width || render.Y+render.Height < own.Y+own.Height {
				t.Fatalf("page %dx%d has an ownership strip outside its render: own=%+v render=%+v", width, height, own, render)
			}
			if int64(render.Width)*int64(render.Height) > maxRenderedPixels {
				t.Fatalf("page %dx%d has a render over the pixel bound: %+v", width, height, render)
			}
			ownershipArea += own.Width * own.Height
		}
		if ownershipArea != width*height {
			t.Fatalf("page %dx%d ownership covers %d pixels, want %d", width, height, ownershipArea, width*height)
		}
		for left := 0; left < len(tiles); left++ {
			for right := left + 1; right < len(tiles); right++ {
				a, b := tiles[left].Ownership, tiles[right].Ownership
				if a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height {
					t.Fatalf("page %dx%d ownership strips overlap: %+v and %+v", width, height, a, b)
				}
			}
		}
	}
}

func TestNameReadingTileConfigKeepsPageScaleThresholds(t *testing.T) {
	base := DefaultCarrierLocaliserConfig()
	pageWidth, pageHeight := 14174, 9922
	tileWidth, tileHeight := 2176, 2176
	config := nameReadingTileConfig(base, pageWidth, pageHeight, tileWidth, tileHeight, true)
	wantGap := int(base.NameGroupGapFraction * float64(pageWidth))
	gotGap := int(config.NameGroupGapFraction * float64(tileWidth))
	if gotGap != wantGap {
		t.Fatalf("tile gap = %d px, want the page's %d px", gotGap, wantGap)
	}
	wantMinHeight := int(base.NameGroupMinHeightFraction * float64(pageHeight))
	gotMinHeight := int(config.NameGroupMinHeightFraction * float64(tileHeight))
	if gotMinHeight != wantMinHeight {
		t.Fatalf("tile minimum height = %d px, want the page's %d px", gotMinHeight, wantMinHeight)
	}
	if config.NameGroupMaxAreaFraction != nameReadingTileMaxAreaFraction {
		t.Fatalf("tiled maximum group area = %v, want %v", config.NameGroupMaxAreaFraction, nameReadingTileMaxAreaFraction)
	}
	whole := nameReadingTileConfig(base, 2000, 3000, 2000, 3000, false)
	if whole.NameGroupMaxAreaFraction != base.NameGroupMaxAreaFraction {
		t.Fatalf("a one-tile page changed the calibrated maximum area: %v", whole.NameGroupMaxAreaFraction)
	}
}

func TestReadNamesOnPageReadsEachNameOnceFromSourceTiles(t *testing.T) {
	canvas := chartNameFixture(t)
	page := &ChartPDFPage{
		sourceImage: canvas,
		PageNumber:  1,
		PageCount:   1,
		DPI:         chartAnalysisDPI,
		Width:       4200,
		Height:      4000,
	}
	provider := &recordingNameProvider{}
	readings, err := ReadNamesOnPage(context.Background(), page, provider, DefaultCarrierLocaliserConfig(), DefaultCarrierReaderConfig())
	if err != nil {
		t.Fatalf("ReadNamesOnPage: %v", err)
	}
	if len(readings) != 10 {
		t.Fatalf("found %d names, want 10 (one per fixture mark; a different count means a tile boundary lost or duplicated one): %+v", len(readings), readings)
	}
	for index, reading := range readings {
		if !reading.Read {
			t.Fatalf("name %d was detected but never read", index)
		}
		if reading.Reading.TextAR != "اسم" {
			t.Fatalf("name %d reading = %q, want the provider's consensus", index, reading.Reading.TextAR)
		}
		if reading.Region.X < 0 || reading.Region.Y < 0 || reading.Region.X+reading.Region.Width > 1 || reading.Region.Y+reading.Region.Height > 1 {
			t.Fatalf("name %d region is outside the page: %+v", index, reading.Region)
		}
		if index > 0 {
			previous := readings[index-1].Region
			if reading.Region.Y < previous.Y || (reading.Region.Y == previous.Y && reading.Region.X < previous.X) {
				t.Fatalf("names are not in reading order at %d: %+v then %+v", index, previous, reading.Region)
			}
		}
	}
	if len(provider.inputs) != 10*DefaultCarrierReaderConfig().Views {
		t.Fatalf("the provider saw %d crops, want 10 names times %d views", len(provider.inputs), DefaultCarrierReaderConfig().Views)
	}
	for index, tight := range provider.inputs {
		config, _, err := image.DecodeConfig(bytes.NewReader(tight))
		if err != nil {
			t.Fatalf("crop %d is not an image: %v", index, err)
		}
		if config.Width < 100 || config.Height < 20 {
			t.Fatalf("crop %d is %dx%d; a crop this small means the reader was handed a fragment instead of the name", index, config.Width, config.Height)
		}
	}
}

func TestOpenChartPageAcceptsAnImageAboveThePreviewBound(t *testing.T) {
	// The upload contract accepts up to 40 megapixels, and the reading path must
	// accept the same documents it accepted at upload. The old 16-megapixel
	// check here refused a 24-megapixel photograph after it had been stored,
	// which looked like a chart with no names rather than a rejected file.
	canvas := image.NewRGBA(image.Rect(0, 0, 4100, 4100))
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	extractor := NewPDFExtractor()
	page, err := extractor.OpenChartPageForAnalysis(context.Background(), buffer.Bytes(), 1)
	if err != nil {
		t.Fatalf("a %.1f-megapixel image was refused: %v", float64(4100*4100)/1_000_000, err)
	}
	defer page.Close()
	if page.Width != 4100 || page.Height != 4100 {
		t.Fatalf("page size = %dx%d, want the source's 4100x4100", page.Width, page.Height)
	}
}
