package sourceprocessing

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

// carrierBackground is the paper colour in the fixture.
var carrierBackground = color.RGBA{R: 240, G: 240, B: 235, A: 255}

// carrierInk is the colour of ink in the fixture. It is dark AND saturated, which
// is what the pixels of a letter look like on a scan: the anti-aliased edge of a
// letter carries some of the colour of the shape it is written on, so it belongs
// to that shape's component.
var carrierInk = color.RGBA{R: 25, G: 85, B: 15, A: 255}

// carrierShapeFill is the colour of a shape in the fixture.
var carrierShapeFill = color.RGBA{R: 200, G: 230, B: 60, A: 255}

// regionsOfKind returns the regions of one kind, because the localiser returns two
// kinds: the CARRIERS, which are the coloured shapes, and the NAME GROUPS, which are
// the runs of ink that are the names.
func regionsOfKind(regions []PageRegion, kind string) []PageRegion {
	filtered := make([]PageRegion, 0, len(regions))
	for _, region := range regions {
		if region.Kind == kind {
			filtered = append(filtered, region)
		}
	}
	return filtered
}

// A carrier is a coloured shape with dark ink inside it. The synthetic page below is
// the smallest thing that distinguishes the localiser from the ink-run detector: a
// coloured shape on a photographic-looking background, with a name written on it.
func syntheticCarrierPage(t *testing.T, shapes []carrierShape, background color.RGBA) []byte {
	t.Helper()
	// The page is large relative to the shapes, because a name carrier is a small
	// fraction of a page and the localiser rejects anything larger than 2% of it.
	const width, height = 600, 450
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, background)
		}
	}
	for _, shape := range shapes {
		for y := shape.top; y < shape.top+shape.height; y++ {
			for x := shape.left; x < shape.left+shape.width; x++ {
				if x < 0 || y < 0 || x >= width || y >= height {
					continue
				}
				canvas.Set(x, y, shape.fill)
			}
		}
		// A word is not a rule across the whole shape. It is a run of short dark
		// strokes in the middle of it, so it does not cut the shape in two.
		//
		// The ink takes the colour of whatever it is written on, which is what a
		// scan does: a letter over a coloured shape carries some of that shape's
		// colour and is part of the shape's component, and a letter over bare
		// paper is black and belongs to nothing. That is how one word can reach
		// two separate pieces of one shape.
		wordLeft := shape.wordLeft
		wordRight := shape.wordRight
		if wordLeft == 0 && wordRight == 0 {
			wordLeft = shape.left + shape.width/4
			wordRight = shape.left + 3*shape.width/4
		}
		paint := func(x, y int) {
			if x < 0 || y < 0 || x >= width || y >= height {
				return
			}
			red, _, _, _ := canvas.At(x, y).RGBA()
			if red == uint32(shape.fill.R)*257 || red == uint32(carrierShapeFill.R)*257 {
				canvas.Set(x, y, shape.ink)
				return
			}
			canvas.Set(x, y, color.RGBA{R: 10, G: 10, B: 10, A: 255})
		}
		for x := wordLeft; x < wordRight; x++ {
			for _, thickness := range []int{0, 1, 2} {
				paint(x, shape.top+shape.textRow+thickness)
			}
			// a stem every few pixels, so the ink breaks into letter-like strokes
			if (x-wordLeft)%6 == 0 {
				for thickness := 3; thickness <= 7; thickness++ {
					paint(x, shape.top+shape.textRow-thickness)
				}
			}
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("could not encode the synthetic page: %v", err)
	}
	return buffer.Bytes()
}

type carrierShape struct {
	left, top, width, height int
	textRow                  int
	fill                     color.RGBA
	ink                      color.RGBA
	// wordLeft and wordRight place the ink, in absolute pixels. Zero means the
	// middle half of the shape. They are absolute so that one word can span two
	// separate shapes, which is what a fragmented leaf looks like.
	wordLeft, wordRight int
}

func TestDetectNameCarriersFindsAColouredShapeWithInkOnIt(t *testing.T) {
	page := syntheticCarrierPage(t, []carrierShape{{
		left: 40, top: 40, width: 60, height: 30, textRow: 15,
		fill: carrierShapeFill,
		ink:  carrierInk,
	}}, color.RGBA{R: 240, G: 240, B: 235, A: 255})

	regions, err := DetectNameCarriers(page, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	carriers := regionsOfKind(regions, "name_carrier")
	if len(carriers) != 1 {
		t.Fatalf("expected one name carrier, got %d", len(carriers))
	}
	region := carriers[0]
	if region.Kind != "name_carrier" {
		t.Fatalf("expected kind %q, got %q", "name_carrier", region.Kind)
	}
	if region.Width <= 0 || region.Height <= 0 {
		t.Fatalf("expected a positive region, got %+v", region)
	}
	if region.Confidence == nil {
		t.Fatal("expected a confidence on the carrier")
	}
	if *region.Confidence <= 0 || *region.Confidence > 1 {
		t.Fatalf("expected a confidence within (0,1], got %v", *region.Confidence)
	}
}

func TestDetectNameCarriersIgnoresAColouredShapeWithNoInk(t *testing.T) {
	page := syntheticCarrierPage(t, []carrierShape{{
		left: 40, top: 40, width: 60, height: 30, textRow: 15,
		fill: carrierShapeFill,
		ink:  carrierInk,
	}}, color.RGBA{R: 240, G: 240, B: 235, A: 255})

	// A second page with the same shape but no ink on it.
	blank := syntheticCarrierPage(t, []carrierShape{{
		left: 40, top: 40, width: 60, height: 30, textRow: 15,
		fill: carrierShapeFill,
		ink:  color.RGBA{R: 200, G: 230, B: 60, A: 255},
	}}, color.RGBA{R: 240, G: 240, B: 235, A: 255})

	withInk, err := DetectNameCarriers(page, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	withoutInk, err := DetectNameCarriers(blank, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regionsOfKind(withoutInk, "name_carrier")) != 0 {
		t.Fatalf("expected no carrier for a shape with no ink, got %d", len(regionsOfKind(withoutInk, "name_carrier")))
	}
	if len(regionsOfKind(withInk, "name_carrier")) == 0 {
		t.Fatal("expected the inked shape to be found")
	}
}

// The merge of fragments of one shape is NOT pinned here, and the reason is worth
// stating. A fragment is a JPEG artefact: one leaf broken by compression into two
// pieces, with the word written across both, and the ink over each piece carrying
// some of that piece's colour. A synthetic page can imitate that, but only
// approximately, and a fixture that approximates the artefact ends up pinning the
// approximation rather than the behaviour.
//
// The behaviour is pinned where it actually happens, by
// TestNameCarrierLocaliserAgreesAcrossTwoRenderingsOfTheSameFacts: the
// photographic rendering in the corpus IS a JPEG artefact, and the localiser has
// to find the same carriers there that it finds in the clean drawn rendering.
func TestDetectNameCarriersDoesNotMergeShapesThatShareNoText(t *testing.T) {
	// Two separate shapes with no word spanning them stay two carriers. This is
	// the guard on the other side of the merge: a merge that is too eager turns a
	// dense canopy into one carrier.
	shapes := []carrierShape{
		{left: 40, top: 40, width: 25, height: 30, textRow: 15, fill: carrierShapeFill, ink: carrierInk},
		{left: 200, top: 40, width: 25, height: 30, textRow: 15, fill: carrierShapeFill, ink: carrierInk},
	}
	page := syntheticCarrierPage(t, shapes, carrierBackground)

	regions, err := DetectNameCarriers(page, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regionsOfKind(regions, "name_carrier")) != 2 {
		t.Fatalf("expected two separate carriers, got %d", len(regionsOfKind(regions, "name_carrier")))
	}
}

func TestDetectNameCarriersRejectsAPageThatIsNotAnImage(t *testing.T) {
	if _, err := DetectNameCarriers([]byte("not an image"), DefaultCarrierLocaliserConfig()); err == nil {
		t.Fatal("expected an error for content that is not an image")
	}
	if _, err := DetectNameCarriers(nil, DefaultCarrierLocaliserConfig()); err == nil {
		t.Fatal("expected an error for empty content")
	}
}

func TestDetectNameCarriersProducesRegionsInsideThePage(t *testing.T) {
	page := syntheticCarrierPage(t, []carrierShape{
		{left: 10, top: 10, width: 40, height: 20, textRow: 10, fill: carrierShapeFill, ink: carrierInk},
		{left: 120, top: 100, width: 50, height: 25, textRow: 12, fill: carrierShapeFill, ink: carrierInk},
	}, color.RGBA{R: 240, G: 240, B: 235, A: 255})

	regions, err := DetectNameCarriers(page, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	carriers := regionsOfKind(regions, "name_carrier")
	if len(carriers) != 2 {
		t.Fatalf("expected two carriers, got %d", len(carriers))
	}
	for _, region := range carriers {
		if region.X < 0 || region.Y < 0 {
			t.Fatalf("region starts outside the page: %+v", region)
		}
		if region.X+region.Width > 1.000001 || region.Y+region.Height > 1.000001 {
			t.Fatalf("region ends outside the page: %+v", region)
		}
		if region.Confidence != nil && (math.IsNaN(*region.Confidence) || *region.Confidence < 0 || *region.Confidence > 1) {
			t.Fatalf("confidence is not a probability: %v", *region.Confidence)
		}
	}
}

func TestCarrierLocaliserReadingIsRecordedWithItsMethodAndModel(t *testing.T) {
	page := syntheticCarrierPage(t, []carrierShape{{
		left: 40, top: 40, width: 60, height: 30, textRow: 15,
		fill: carrierShapeFill,
		ink:  carrierInk,
	}}, color.RGBA{R: 240, G: 240, B: 235, A: 255})

	reading, err := CarrierLocaliserReading(page, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reading.Method != carrierReadingMethod {
		t.Fatalf("expected method %q, got %q", carrierReadingMethod, reading.Method)
	}
	if reading.Model != carrierReadingModel {
		t.Fatalf("expected model %q, got %q", carrierReadingModel, reading.Model)
	}
	if len(regionsOfKind(reading.Regions, "name_carrier")) != 1 {
		t.Fatalf("expected one carrier on the reading, got %d", len(regionsOfKind(reading.Regions, "name_carrier")))
	}
}

func TestDescribeCarrierLocaliserRecordsTheThresholds(t *testing.T) {
	description := DescribeCarrierLocaliser(DefaultCarrierLocaliserConfig())
	for _, expected := range []string{"saturation=60", "ink=60", "maxArea=0.0200", "mergeGap=0.0100"} {
		if !bytes.Contains([]byte(description), []byte(expected)) {
			t.Fatalf("expected %q in the description, got %q", expected, description)
		}
	}
}

// A name group is a run of ink that is one name. The localiser returns both the
// CARRIERS, which are the coloured shapes, and the NAME GROUPS, which are the
// names, and the two are different units: on a painted tree the leaves touch, so
// one carrier is often a dozen of them.
func TestDetectNameCarriersSeparatesTheShapesFromTheNames(t *testing.T) {
	page := syntheticCarrierPage(t, []carrierShape{
		{left: 40, top: 40, width: 60, height: 30, textRow: 15, fill: carrierShapeFill, ink: carrierInk},
	}, carrierBackground)

	regions, err := DetectNameCarriers(page, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	carriers := regionsOfKind(regions, "name_carrier")
	groups := regionsOfKind(regions, "name_group")
	if len(carriers) != 1 {
		t.Fatalf("expected one carrier, got %d", len(carriers))
	}
	if len(groups) == 0 {
		t.Fatal("expected the ink to be grouped into at least one name")
	}
	for _, group := range groups {
		if group.Width <= 0 || group.Height <= 0 {
			t.Fatalf("a name group has no extent: %+v", group)
		}
		if group.X < 0 || group.Y < 0 || group.X+group.Width > 1.000001 || group.Y+group.Height > 1.000001 {
			t.Fatalf("a name group is outside the page: %+v", group)
		}
	}
}

// A black-and-white chart has no coloured shape to anchor a carrier to, but it
// still has names, and a name is a run of dark ink. This is the colour-independent
// half of the localiser: it is what makes a calligraphic tree, a printed chart,
// or a screenshot readable without pretending a leaf exists.
func TestDetectNameCarriersGroupsInkOnAPageWithNoColour(t *testing.T) {
	const width, height = 600, 450
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, color.RGBA{R: 248, G: 248, B: 246, A: 255})
		}
	}
	words := []struct{ left, top, width int }{
		{40, 60, 55},
		{220, 120, 70},
		{90, 260, 60},
	}
	for _, word := range words {
		for y := word.top; y < word.top+10; y++ {
			for x := word.left; x < word.left+word.width; x++ {
				canvas.Set(x, y, color.RGBA{R: 20, G: 20, B: 20, A: 255})
			}
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("could not encode the synthetic page: %v", err)
	}
	regions, err := DetectNameCarriers(buffer.Bytes(), DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regionsOfKind(regions, "name_carrier")) != 0 {
		t.Fatalf("a page with no colour should have no carriers, got %d", len(regionsOfKind(regions, "name_carrier")))
	}
	groups := regionsOfKind(regions, "name_group")
	if len(groups) != len(words) {
		t.Fatalf("expected %d name groups on the ink-only page, got %d: %+v", len(words), len(groups), groups)
	}
}

// A text block is not a name. The ancestor stack down the trunk of a painted tree
// and the panels of prose beside it come out as one enormous group, and a reader
// asked for one name would be asked to read a page.
func TestDetectNameCarriersExcludesAGroupThatIsTooLargeToBeAName(t *testing.T) {
	config := DefaultCarrierLocaliserConfig()
	config.NameGroupMaxAreaFraction = 0.0001
	page := syntheticCarrierPage(t, []carrierShape{
		{left: 40, top: 40, width: 60, height: 30, textRow: 15, fill: carrierShapeFill, ink: carrierInk},
	}, carrierBackground)

	regions, err := DetectNameCarriers(page, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, group := range regionsOfKind(regions, "name_group") {
		if group.Width*group.Height > config.NameGroupMaxAreaFraction {
			t.Fatalf("a name group larger than the limit survived: %+v", group)
		}
	}
}
