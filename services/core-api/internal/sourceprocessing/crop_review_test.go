package sourceprocessing

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

func TestCropGeometryUsesClampedSourceBounds(t *testing.T) {
	frame := NamePageFrame{Width: 768, Height: 1138}
	for _, region := range []Region{{X: .686, Y: .620, Width: 1.0 / 768, Height: 2.0 / 1138}, {X: .98, Y: .97, Width: .02, Height: .03}, {X: 0, Y: 0, Width: .04, Height: .03}} {
		geometry := cropGeometry(region, frame)
		for view, local := range map[string]Region{"tight": geometry.TightRegion, "context": geometry.ContextRegion} {
			bounds, err := suggestionCropBounds(region, frame.Width, frame.Height, view)
			if err != nil {
				t.Fatal(err)
			}
			if local.X < 0 || local.Y < 0 || local.X+local.Width > 1.000001 || local.Y+local.Height > 1.000001 {
				t.Fatalf("outside %s crop: %+v", view, local)
			}
			if diff := local.X*float64(bounds.Width) + float64(bounds.X) - region.X*768; diff < -.00001 || diff > .00001 {
				t.Fatalf("wrong crop origin: %v", diff)
			}
		}
	}
}

func TestLowResolutionChartProposalsHaveSourcePixelExtent(t *testing.T) {
	var path string
	for _, document := range readCorpus(t) {
		if document.DocumentID == "alsharmi" && len(document.Pages) > 0 {
			path = document.Pages[0]
		}
	}
	if path == "" {
		t.Skip("alsharmi corpus page is unavailable")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	regions, err := DetectNameCarriers(data, DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatal(err)
	}
	groups := regionsOfKind(regions, "name_group")
	if len(groups) == 0 {
		t.Fatal("light names were missed")
	}
	for _, group := range groups {
		if isNameFragment(group.Width*768, group.Height*1138) {
			t.Fatalf("fragment survived: %+v", group)
		}
	}
	// A known yellow name at the top of this chart. This is a localisation
	// regression only, not an accuracy score or a transcription answer key.
	found := false
	for _, group := range groups {
		if group.X*768 <= 420 && (group.X+group.Width)*768 >= 410 && group.Y*1138 <= 145 && (group.Y+group.Height)*1138 >= 140 {
			found = true
		}
	}
	if !found {
		t.Fatal("missed yellow lettering on the top-right leaf")
	}
	t.Logf("bounded proposals: %d", len(groups))
}

func TestLightLettersAreLocalisedWithoutProposingBlankLeaves(t *testing.T) {
	for _, hasInk := range []bool{false, true} {
		ink := color.RGBA{40, 90, 30, 255}
		if hasInk {
			ink = color.RGBA{235, 215, 30, 255}
		}
		page := syntheticCarrierPage(t, []carrierShape{{left: 40, top: 40, width: 60, height: 30, textRow: 15, fill: color.RGBA{40, 90, 30, 255}, ink: ink}}, carrierBackground)
		regions, err := DetectNameCarriers(page, DefaultCarrierLocaliserConfig())
		if err != nil {
			t.Fatal(err)
		}
		groups := regionsOfKind(regions, "name_group")
		if hasInk && len(groups) == 0 {
			t.Fatal("light lettering was missed")
		}
		if !hasInk && len(groups) != 0 {
			t.Fatalf("blank leaf became a name: %+v", groups)
		}
	}
}

func TestLightLetteringDoesNotAddDarkBranchPlaceholders(t *testing.T) {
	shapes := []carrierShape{}
	for _, left := range []int{40, 240, 440} {
		shapes = append(shapes, carrierShape{left: left, top: 40, width: 60, height: 30, textRow: 15, fill: color.RGBA{40, 90, 30, 255}, ink: color.RGBA{235, 215, 30, 255}})
	}
	data := syntheticCarrierPage(t, shapes, carrierBackground)
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	canvas := image.NewRGBA(decoded.Bounds())
	draw.Draw(canvas, canvas.Bounds(), decoded, image.Point{}, draw.Src)
	// Large enough to pass the old pixel floor, but only a piece of drawing.
	for y := 330; y < 342; y++ {
		for x := 80; x < 96; x++ {
			canvas.Set(x, y, color.Black)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	regions, err := DetectNameCarriers(encoded.Bytes(), DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatal(err)
	}
	groups := regionsOfKind(regions, "name_group")
	if len(groups) != 3 {
		t.Fatalf("expected the three light names only: %+v", groups)
	}
	for _, region := range groups {
		if region.Y > .5 {
			t.Fatalf("drawing was offered as a name: %+v", region)
		}
	}
}

func TestYellowBoxBordersOnBrownBranchesAreNotNames(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 600, 450))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: carrierBackground}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(100, 80, 160, 140), &image.Uniform{C: color.RGBA{140, 60, 20, 255}}, image.Point{}, draw.Src)
	for y := 90; y < 128; y++ {
		for x := 110; x < 148; x++ {
			if x < 113 || x >= 145 || y < 93 || y >= 125 {
				canvas.Set(x, y, color.RGBA{235, 215, 30, 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	regions, err := DetectNameCarriers(encoded.Bytes(), DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatal(err)
	}
	if groups := regionsOfKind(regions, "name_group"); len(groups) != 0 {
		t.Fatalf("gold box border was offered as a name: %+v", groups)
	}
}

func TestThinStemsAndDotsDoNotBecomeNames(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 600, 450))
	for y := 0; y < 450; y++ {
		for x := 0; x < 600; x++ {
			canvas.Set(x, y, carrierBackground)
		}
	}
	for _, box := range []image.Rectangle{image.Rect(40, 40, 42, 100), image.Rect(60, 120, 160, 122), image.Rect(200, 200, 202, 202)} {
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				canvas.Set(x, y, color.Black)
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	regions, err := DetectNameCarriers(encoded.Bytes(), DefaultCarrierLocaliserConfig())
	if err != nil {
		t.Fatal(err)
	}
	if groups := regionsOfKind(regions, "name_group"); len(groups) != 0 {
		t.Fatalf("decorative fragments became names: %+v", groups)
	}
	legacy, err := detectNameCarriers(encoded.Bytes(), DefaultCarrierLocaliserConfig(), false)
	if err != nil || len(regionsOfKind(legacy, "name_group")) == 0 {
		t.Fatalf("pinned v1 runs lost their original detector behavior: %v", err)
	}
}

func TestFragmentReviewKeepsTheMarkerOnTheSamePixels(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 560, 300))
	for _, region := range []Region{{X: .001, Y: .001, Width: .004, Height: .006}, {X: .99, Y: .99, Width: .005, Height: .005}, {X: .5, Y: .5, Width: .002, Height: .004}} {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, canvas); err != nil {
			t.Fatal(err)
		}
		content, local, err := fragmentReviewCrop(encoded.Bytes(), region)
		if err != nil {
			t.Fatal(err)
		}
		config, err := png.DecodeConfig(bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		if config.Width != 160 || config.Height != 100 {
			t.Fatalf("wrong review bounds: %+v", config)
		}
		if local.X < 0 || local.Y < 0 || local.X+local.Width > 1 || local.Y+local.Height > 1 {
			t.Fatalf("marker escaped review: %+v", local)
		}
		if got, want := local.Width*160, region.Width*560; got-want > .00001 || want-got > .00001 {
			t.Fatal("marker width changed")
		}
	}
}
