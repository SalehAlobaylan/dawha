package regionreview

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pngDecode(encoded []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(encoded))
}

func TestParseCropBoxAcceptsTheDetectorShapeAndRefusesTheRest(t *testing.T) {
	valid, err := parseCropBox([]byte(`{"x":1434,"y":3720,"w":404,"h":78}`))
	if err != nil {
		t.Fatalf("valid crop box: %v", err)
	}
	if valid != (CropBox{X: 1434, Y: 3720, W: 404, H: 78}) {
		t.Fatalf("parsed box = %+v", valid)
	}

	for name, raw := range map[string]string{
		"zero width":    `{"x":1,"y":1,"w":0,"h":5}`,
		"zero height":   `{"x":1,"y":1,"w":5,"h":0}`,
		"negative x":    `{"x":-1,"y":1,"w":5,"h":5}`,
		"negative y":    `{"x":1,"y":-1,"w":5,"h":5}`,
		"fractional":    `{"x":1.5,"y":1,"w":5,"h":5}`,
		"not an object": `[1434,3720,404,78]`,
		"empty":         ``,
	} {
		if _, err := parseCropBox([]byte(raw)); err == nil {
			t.Fatalf("%s: parseCropBox accepted %q", name, raw)
		}
	}
}

func TestTightBoundsShowsTheDetectorBoxExactlyAndClampsToThePlate(t *testing.T) {
	// A box inside the plate is served with no padding: padding would change
	// the pixels the score components describe.
	box := CropBox{X: 100, Y: 80, W: 80, H: 40}
	tight, err := tightBounds(box, 7163, 7170)
	if err != nil {
		t.Fatalf("interior box: %v", err)
	}
	if tight != box {
		t.Fatalf("interior tight = %+v, want the box unchanged", tight)
	}

	// A box hanging over the edges is clamped into the plate, never off it.
	edge, err := tightBounds(CropBox{X: 0, Y: 0, W: 50, H: 30}, 7163, 7170)
	if err != nil {
		t.Fatalf("corner box: %v", err)
	}
	if edge != (CropBox{X: 0, Y: 0, W: 50, H: 30}) {
		t.Fatalf("corner tight = %+v", edge)
	}
	overhang, err := tightBounds(CropBox{X: 7000, Y: 7100, W: 400, H: 400}, 7163, 7170)
	if err != nil {
		t.Fatalf("overhanging box: %v", err)
	}
	if overhang.W != 7163-7000 || overhang.H != 7170-7100 {
		t.Fatalf("overhanging tight = %+v, want it clamped to the plate", overhang)
	}

	for name, box := range map[string]CropBox{
		"zero width":  {X: 1, Y: 1, W: 0, H: 5},
		"zero height": {X: 1, Y: 1, W: 5, H: 0},
		"negative":    {X: -1, Y: 1, W: 5, H: 5},
	} {
		if _, err := tightBounds(box, 7163, 7170); err == nil {
			t.Fatalf("%s: tightBounds accepted %+v", name, box)
		}
	}
}

func TestContextBoundsGrowsToTheNeighbourhoodAndClampsToThePlate(t *testing.T) {
	// A small label grows to at least the context minima, centred on the box.
	box := CropBox{X: 1000, Y: 1000, W: 80, H: 40}
	context := contextBounds(box, 7163, 7170)
	if context.W < contextMinWidth || context.H < contextMinHeight {
		t.Fatalf("context = %+v, want at least %d×%d", context, contextMinWidth, contextMinHeight)
	}
	centre := func(b CropBox) (int, int) { return b.X + b.W/2, b.Y + b.H/2 }
	if cx, _ := centre(context); cx < box.X || cx > box.X+box.W {
		t.Fatalf("context centre x = %d, outside the box %+v", cx, box)
	}
	if _, cy := centre(context); cy < box.Y || cy > box.Y+box.H {
		t.Fatalf("context centre y = %d, outside the box %+v", cy, box)
	}

	// A box against the edge keeps its context inside the plate.
	corner := contextBounds(CropBox{X: 0, Y: 0, W: 100, H: 50}, 7163, 7170)
	if corner.X < 0 || corner.Y < 0 {
		t.Fatalf("corner context = %+v, left the plate", corner)
	}
	if corner.X+corner.W > 7163 || corner.Y+corner.H > 7170 {
		t.Fatalf("corner context = %+v, left the plate", corner)
	}

	// The bounds are response bounds: a huge box shrinks to the cap instead
	// of shipping a plate-sized image.
	huge := contextBounds(CropBox{X: 100, Y: 100, W: 4000, H: 3000}, 7163, 7170)
	if huge.W > plateCropMaxSide || huge.H > plateCropMaxSide {
		t.Fatalf("huge context = %+v, over the side cap %d", huge, plateCropMaxSide)
	}
	if int64(huge.W)*int64(huge.H) > plateCropMaxPixels {
		t.Fatalf("huge context = %+v, over the area cap %d", huge, plateCropMaxPixels)
	}
	if huge.W < 1 || huge.H < 1 {
		t.Fatalf("huge context = %+v, empty", huge)
	}
}

func TestContextBoundsHonoursTheAreaCapOnWideBoxes(t *testing.T) {
	// 1600×1600 square is inside the side cap but over the area cap
	// (2_560_000 > 2_500_000); the cap must bite.
	context := contextBounds(CropBox{X: 500, Y: 500, W: 320, H: 320}, 7163, 7170)
	if int64(context.W)*int64(context.H) > plateCropMaxPixels {
		t.Fatalf("wide context = %+v, over the area cap", context)
	}
}

func TestCropBoxJSONRoundTripsThroughTheResponseEncoding(t *testing.T) {
	// The response's cropPx must decode back to the box the detector wrote,
	// because the workbench overlays the projected centre at this geometry.
	box := CropBox{X: 1434, Y: 3720, W: 404, H: 78}
	encoded, err := json.Marshal(box)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := parseCropBox(encoded)
	if err != nil || parsed != box {
		t.Fatalf("round trip = %+v, %v", parsed, err)
	}
}

func TestEncodePNGCropsToTheRequestedRectangleOfPixels(t *testing.T) {
	// A 3×2 image whose pixels are (x,y): a crop must carry the source
	// pixels' colours, not re-coloured ones, so the reviewer reads the same
	// ink the detector ranked.
	source := image.NewRGBA(image.Rect(0, 0, 3, 2))
	source.Set(1, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	source.Set(2, 0, color.RGBA{R: 40, G: 50, B: 60, A: 255})
	encoded, err := encodePNG(source, image.Rect(1, 0, 3, 1))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := pngDecode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 1 {
		t.Fatalf("crop bounds = %+v, want 2×1", decoded.Bounds())
	}
	if got := decoded.At(0, 0); got != source.At(1, 0) {
		t.Fatalf("pixel (0,0) = %v, want %v", got, source.At(1, 0))
	}
	if got := decoded.At(1, 0); got != source.At(2, 0) {
		t.Fatalf("pixel (1,0) = %v, want %v", got, source.At(2, 0))
	}
}
