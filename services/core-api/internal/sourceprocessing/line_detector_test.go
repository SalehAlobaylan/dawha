package sourceprocessing

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDetectOrthogonalLineSegmentsReturnsPageRelativeHypotheses(t *testing.T) {
	imageData := image.NewGray(image.Rect(0, 0, 400, 300))
	for index := range imageData.Pix {
		imageData.Pix[index] = 255
	}
	for x := 40; x < 350; x++ {
		imageData.SetGray(x, 100, color.Gray{Y: 0})
	}
	for y := 100; y < 260; y++ {
		imageData.SetGray(200, y, color.Gray{Y: 0})
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, imageData); err != nil {
		t.Fatalf("encode fixture page: %v", err)
	}
	regions, err := detectOrthogonalLineSegments(encoded.Bytes())
	if err != nil {
		t.Fatalf("detect segments: %v", err)
	}
	var horizontal, vertical bool
	for _, region := range regions {
		if region.Kind != "connector_segment" {
			t.Fatalf("region kind = %q, want candidate only", region.Kind)
		}
		if region.Width > region.Height*3 && region.Width > .5 {
			horizontal = true
		}
		if region.Height > region.Width*3 && region.Height > .3 {
			vertical = true
		}
	}
	if !horizontal || !vertical {
		t.Fatalf("segment hypotheses = %+v; want a horizontal and vertical run", regions)
	}
	if len(regions) > 10 {
		t.Fatalf("line detector emitted %d duplicate runs", len(regions))
	}
}
