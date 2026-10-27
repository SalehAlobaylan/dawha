package sourceprocessing

import "testing"

func TestSuggestionCropBoundsKeepTheNameAndRespectThePage(t *testing.T) {
	// The review screen asks for two things: a tight crop that shows the name,
	// and a context crop that shows where it sits among its neighbours. Both
	// must stay inside the source page, both must include the name they belong
	// to, and a malformed region must not turn into an unbounded render.
	cases := []struct {
		name       string
		region     Region
		pageWidth  int
		pageHeight int
		view       string
		wantErr    bool
		// A malformed full-page region is deliberately reduced to a readable
		// window; the full name cannot fit inside the response bound.
		containsName bool
	}{
		{
			name:         "a centred name",
			region:       Region{X: 0.4, Y: 0.4, Width: 0.2, Height: 0.2},
			pageWidth:    1000,
			pageHeight:   1000,
			view:         "tight",
			containsName: true,
		},
		{
			name:         "a context crop is wider than the name",
			region:       Region{X: 0.4, Y: 0.4, Width: 0.2, Height: 0.2},
			pageWidth:    1000,
			pageHeight:   1000,
			view:         "context",
			containsName: true,
		},
		{
			name:         "a name at the top-left edge",
			region:       Region{X: 0, Y: 0, Width: 0.1, Height: 0.1},
			pageWidth:    1000,
			pageHeight:   1000,
			view:         "context",
			containsName: true,
		},
		{
			name:         "a name at the bottom-right edge",
			region:       Region{X: 0.9, Y: 0.9, Width: 0.1, Height: 0.1},
			pageWidth:    1000,
			pageHeight:   1000,
			view:         "context",
			containsName: true,
		},
		{
			name:         "a malformed full-page region is bounded",
			region:       Region{X: 0, Y: 0, Width: 1, Height: 1},
			pageWidth:    4000,
			pageHeight:   4000,
			view:         "context",
			containsName: false,
		},
		{name: "an empty width", region: Region{X: 0.1, Y: 0.1, Width: 0, Height: 0.1}, pageWidth: 1000, pageHeight: 1000, view: "tight", wantErr: true},
		{name: "a region outside the page", region: Region{X: 0.95, Y: 0.1, Width: 0.1, Height: 0.1}, pageWidth: 1000, pageHeight: 1000, view: "tight", wantErr: true},
		{name: "an unknown view", region: Region{X: 0.1, Y: 0.1, Width: 0.1, Height: 0.1}, pageWidth: 1000, pageHeight: 1000, view: "preview", wantErr: true},
		{name: "an empty page", region: Region{X: 0.1, Y: 0.1, Width: 0.1, Height: 0.1}, pageWidth: 0, pageHeight: 1000, view: "tight", wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			bounds, err := suggestionCropBounds(testCase.region, testCase.pageWidth, testCase.pageHeight, testCase.view)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("a region the renderer cannot serve was accepted: %+v", bounds)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid region refused: %v", err)
			}
			if bounds.X < 0 || bounds.Y < 0 || bounds.Width < 1 || bounds.Height < 1 ||
				bounds.X+bounds.Width > testCase.pageWidth || bounds.Y+bounds.Height > testCase.pageHeight {
				t.Fatalf("crop escapes the page: %+v on %dx%d", bounds, testCase.pageWidth, testCase.pageHeight)
			}
			if int64(bounds.Width)*int64(bounds.Height) > suggestionCropMaxPixels ||
				bounds.Width > suggestionCropMaxSide || bounds.Height > suggestionCropMaxSide {
				t.Fatalf("crop exceeds the review response bound: %dx%d", bounds.Width, bounds.Height)
			}
			// The tight crop must contain the name; the context crop must be at
			// least as large as the tight crop. A crop that cuts the name is the
			// bug this endpoint exists to avoid.
			left := int(testCase.region.X * float64(testCase.pageWidth))
			top := int(testCase.region.Y * float64(testCase.pageHeight))
			right := int((testCase.region.X + testCase.region.Width) * float64(testCase.pageWidth))
			bottom := int((testCase.region.Y + testCase.region.Height) * float64(testCase.pageHeight))
			if testCase.containsName && (left < bounds.X || top < bounds.Y || right > bounds.X+bounds.Width || bottom > bounds.Y+bounds.Height) {
				t.Fatalf("crop does not include the name region: crop=%+v name=%d,%d..%d,%d", bounds, left, top, right, bottom)
			}
		})
	}
}

func TestAContextCropIsWiderThanATightCrop(t *testing.T) {
	// The point of the second crop is orientation. If it were the same size as
	// the tight crop it would cost a second render and show nothing new.
	region := Region{X: 0.4, Y: 0.4, Width: 0.1, Height: 0.05}
	tight, err := suggestionCropBounds(region, 1000, 1000, "tight")
	if err != nil {
		t.Fatalf("tight crop: %v", err)
	}
	contextCrop, err := suggestionCropBounds(region, 1000, 1000, "context")
	if err != nil {
		t.Fatalf("context crop: %v", err)
	}
	if contextCrop.Width <= tight.Width || contextCrop.Height <= tight.Height {
		t.Fatalf("context %dx%d is not wider than tight %dx%d", contextCrop.Width, contextCrop.Height, tight.Width, tight.Height)
	}
}
