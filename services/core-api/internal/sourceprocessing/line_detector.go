package sourceprocessing

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"sort"
)

const MaxLineCandidates = 1000

type runSpan struct {
	start, end, fixed int
	horizontal        bool
}

// detectOrthogonalLineSegments returns geometry candidates from a rendered page.
// It deliberately does not join intersections into parent/child edges: scan
// strokes, box borders and real connectors can look alike without chart context.
func detectOrthogonalLineSegments(data []byte) ([]PageRegion, error) {
	imageData, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrValidation
	}
	bounds := imageData.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 || int64(width)*int64(height) > maxRenderedPixels {
		return nil, ErrValidation
	}
	ink := make([]bool, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := imageData.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			gray := (299*uint64(r>>8) + 587*uint64(g>>8) + 114*uint64(b>>8)) / 1000
			ink[y*width+x] = gray < 150
		}
	}
	minHorizontal := max(28, width/100)
	minVertical := max(28, height/100)
	horizontal := collectRuns(ink, width, height, true, minHorizontal)
	vertical := collectRuns(ink, width, height, false, minVertical)
	spans := mergeRuns(horizontal, 4)
	spans = append(spans, mergeRuns(vertical, 4)...)
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].fixed == spans[j].fixed {
			return spans[i].start < spans[j].start
		}
		return spans[i].fixed < spans[j].fixed
	})
	if len(spans) > MaxLineCandidates {
		spans = spans[:MaxLineCandidates]
	}
	regions := make([]PageRegion, 0, len(spans))
	for _, span := range spans {
		if span.end-span.start < 1 {
			continue
		}
		// The use of a positive extent keeps SVG overlays visible at any display size.
		if span.horizontal {
			y := float64(span.fixed) / float64(height)
			regions = append(regions, PageRegion{Kind: "connector_segment", X: float64(span.start) / float64(width), Y: y, Width: float64(span.end-span.start) / float64(width), Height: math.Max(1/float64(height), 0.001)})
		} else {
			x := float64(span.fixed) / float64(width)
			regions = append(regions, PageRegion{Kind: "connector_segment", X: x, Y: float64(span.start) / float64(height), Width: math.Max(1/float64(width), 0.001), Height: float64(span.end-span.start) / float64(height)})
		}
	}
	return regions, nil
}

func collectRuns(pixels []bool, width, height int, horizontal bool, minimum int) []runSpan {
	fixedLimit, scanLimit := width, height
	if horizontal {
		fixedLimit, scanLimit = height, width
	}
	spans := make([]runSpan, 0)
	for fixed := 0; fixed < fixedLimit; fixed++ {
		start, lastInk, gaps := -1, -1, 0
		for scan := 0; scan <= scanLimit; scan++ {
			ink := false
			if scan < scanLimit {
				x, y := fixed, scan
				if horizontal {
					x, y = scan, fixed
				}
				ink = pixels[y*width+x]
			}
			if ink {
				if start < 0 {
					start = scan
				}
				lastInk, gaps = scan, 0
				continue
			}
			if start >= 0 && lastInk >= 0 && gaps < 2 && scan < scanLimit {
				gaps++
				continue
			}
			if start >= 0 && lastInk-start+1 >= minimum {
				spans = append(spans, runSpan{start: start, end: lastInk + 1, fixed: fixed, horizontal: horizontal})
			}
			start, lastInk, gaps = -1, -1, 0
		}
	}
	return spans
}

func mergeRuns(input []runSpan, tolerance int) []runSpan {
	if len(input) == 0 {
		return nil
	}
	sort.Slice(input, func(i, j int) bool {
		if input[i].fixed == input[j].fixed {
			return input[i].start < input[j].start
		}
		return input[i].fixed < input[j].fixed
	})
	merged := make([]runSpan, 0, len(input))
	for _, current := range input {
		match := -1
		for index := len(merged) - 1; index >= 0; index-- {
			prior := merged[index]
			if current.fixed-prior.fixed > tolerance {
				break
			}
			overlap := min(current.end, prior.end) - max(current.start, prior.start)
			shorter := min(current.end-current.start, prior.end-prior.start)
			if overlap > 0 && float64(overlap)/float64(shorter) >= 0.7 {
				match = index
				break
			}
		}
		if match < 0 {
			merged = append(merged, current)
			continue
		}
		prior := &merged[match]
		prior.start = min(prior.start, current.start)
		prior.end = max(prior.end, current.end)
		prior.fixed = (prior.fixed + current.fixed) / 2
	}
	return merged
}
