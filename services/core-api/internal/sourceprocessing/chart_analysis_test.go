package sourceprocessing

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"
)

func TestChartAnalysisOnlyResumesWithTheExactProviderAndPrivacyConfiguration(t *testing.T) {
	current, err := json.Marshal(chartPipelineIdentity{
		Version: ChartAnalysisPipelineVersion, Model: ChartVisionModel, HostedProvider: ChartAnalysisHostedProvider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !chartPipelineMatchesCurrent(current, ChartVisionModel) {
		t.Fatal("the current pipeline identity must be resumable")
	}

	stale, _ := json.Marshal(chartPipelineIdentity{
		Version: "arabic-family-chart-vision-v5", Model: ChartVisionModel, HostedProvider: "deepseek-responses-inline-auto-cache",
	})
	if chartPipelineMatchesCurrent(stale, ChartVisionModel) {
		t.Fatal("a run created without documented DeepSeek cache isolation must not resume")
	}
	if chartPipelineMatchesCurrent(current, "another-model") {
		t.Fatal("a run with a different model version must not resume")
	}
	if chartPipelineMatchesCurrent([]byte("not-json"), ChartVisionModel) {
		t.Fatal("a malformed pipeline identity must not resume")
	}
}

func TestChartTileBoundsCoverEverySampledPagePointAndOverlap(t *testing.T) {
	const width, height = 2300, 1800
	tiles := chartTileBounds(width, height)
	if len(tiles) != 30 {
		t.Fatalf("got %d tiles, want 30 for fixed-stride 25%% overlap at the dense-chart size", len(tiles))
	}
	for _, tile := range tiles {
		if tile.Width <= 0 || tile.Height <= 0 || tile.X < 0 || tile.Y < 0 || tile.X+tile.Width > width || tile.Y+tile.Height > height {
			t.Fatalf("tile outside page: %+v", tile)
		}
	}
	for y := 0; y < height; y += 37 {
		for x := 0; x < width; x += 41 {
			covered := false
			for _, tile := range tiles {
				if x >= tile.X && y >= tile.Y && x < tile.X+tile.Width && y < tile.Y+tile.Height {
					covered = true
					break
				}
			}
			if !covered {
				t.Fatalf("page sample (%d,%d) was uncovered", x, y)
			}
		}
	}
	if boxOverlapSmaller(
		ChartBox{X: 0, Y: 0, Width: float64(chartTileSize) / width, Height: 1},
		ChartBox{X: float64(chartTileSize-chartTileOverlap) / width, Y: 0, Width: float64(chartTileSize) / width, Height: 1},
	) < .24 {
		t.Fatal("adjacent tiles lost the versioned overlap")
	}
	if count := chartTileCount(width, height); count != len(tiles) {
		t.Fatalf("bounded tile count = %d, generated %d tiles", count, len(tiles))
	}
}

func TestCrowdedChartTilesGetOneOverlappingSubdivisionPass(t *testing.T) {
	parent := ChartTileBounds{X: 100, Y: 200, Width: 1024, Height: 800}
	children := chartSubtileBounds(parent)
	if len(children) != 4 {
		t.Fatalf("got %d child regions, want four", len(children))
	}
	for _, child := range children {
		if child.X < parent.X || child.Y < parent.Y || child.X+child.Width > parent.X+parent.Width || child.Y+child.Height > parent.Y+parent.Height {
			t.Fatalf("child escaped parent: %+v", child)
		}
	}
	if !crowdedChartReading(ChartTileReading{Labels: make([]ChartLabelProposal, 250)}) {
		t.Fatal("schema-ceiling output must trigger one subdivision pass")
	}
	if !crowdedChartReading(ChartTileReading{CompletenessNote: "Could not inspect all labels in this dense region"}) {
		t.Fatal("explicit truncation note did not trigger subdivision")
	}
	if crowdedChartReading(ChartTileReading{Labels: make([]ChartLabelProposal, 249), CompletenessNote: "review required"}) {
		t.Fatal("ordinary incomplete review note should not trigger subdivision")
	}
}

func TestTruncatedAndInvalidDenseTileOutputsGetOneSubdivisionPass(t *testing.T) {
	bounds := ChartTileBounds{X: 0, Y: 0, Width: chartTileSize, Height: chartTileSize}
	if !chartOutputNeedsSubdivision(ErrChartVisionTruncated, bounds, true) {
		t.Fatal("truncated dense tile was not scheduled for one smaller pass")
	}
	if !chartOutputNeedsSubdivision(ErrChartVisionOutputInvalid, bounds, true) {
		t.Fatal("invalid dense tile output was not scheduled for one smaller pass")
	}
	if chartOutputNeedsSubdivision(ErrChartVisionOutputInvalid, bounds, false) {
		t.Fatal("a child tile received more than the single subdivision pass")
	}
	if chartOutputNeedsSubdivision(errors.New("transport failure"), bounds, true) {
		t.Fatal("a transient provider failure was mislabeled as a density issue")
	}
}

func TestChartOccurrencePixelBoundsKeepTightAndContextCropsInsidePage(t *testing.T) {
	tight, contextBounds, ok := chartOccurrencePixelBounds(ChartBox{X: 0, Y: .1, Width: .02, Height: .03}, 2000, 1000)
	if !ok || tight.X != 0 || contextBounds.X != 0 || contextBounds.Width <= tight.Width || contextBounds.Height <= tight.Height {
		t.Fatalf("tight/context crops = %+v/%+v ok=%v", tight, contextBounds, ok)
	}
	if contextBounds.X+contextBounds.Width > 2000 || contextBounds.Y+contextBounds.Height > 1000 {
		t.Fatalf("context escaped source page: %+v", contextBounds)
	}
}

func TestChartReadBatchesRespectCountAndByteBudgetsWithoutDroppingOccurrences(t *testing.T) {
	page := image.NewNRGBA(image.Rect(0, 0, 600, 300))
	for y := 0; y < page.Bounds().Dy(); y++ {
		for x := 0; x < page.Bounds().Dx(); x++ {
			page.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	names := make([]ChartNameOccurrence, 19)
	for i := range names {
		names[i] = ChartNameOccurrence{ID: stableChartID("chart", "tile", "label", i), Region: ChartBox{X: float64(i%9) / 10, Y: float64(i/9) / 3, Width: .04, Height: .08}}
	}
	batches := makeChartReadBatches(page, names)
	if len(batches) < 3 {
		t.Fatalf("got %d batches, expected count bound to split 19 occurrences", len(batches))
	}
	seen := map[string]bool{}
	for index, batch := range batches {
		if batch.Key != fmt.Sprintf("read_%05d", index) {
			t.Fatalf("batch key is not deterministic: %q", batch.Key)
		}
		if len(batch.Inputs) > chartMaxReadBatchImages || batch.InputBytes > chartMaxReadBatchBytes {
			t.Fatalf("batch exceeded request bounds: %d images, %d bytes", len(batch.Inputs), batch.InputBytes)
		}
		if len(batch.Inputs) != len(batch.OccurrenceIDs) {
			t.Fatalf("batch contains a failed crop or mismatched IDs: %+v", batch)
		}
		for _, id := range batch.OccurrenceIDs {
			if seen[id] {
				t.Fatalf("occurrence %s was scheduled twice", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != len(names) {
		t.Fatalf("scheduled %d/%d name occurrences", len(seen), len(names))
	}
}

func TestNormalizeChartReviewInvalidatesChangedCropAndPreservesOriginIDs(t *testing.T) {
	current := ChartAnalysisView{
		Branches: []ChartBranchRegion{{ID: "branch", PageNumber: 1, Polygon: []ChartPoint{{0, 0}, {1, 0}, {1, 1}}, ReviewStatus: "needs_review"}},
		Names:    []ChartNameOccurrence{{ID: "name", PageNumber: 1, Region: ChartBox{X: .1, Y: .2, Width: .1, Height: .1}, SourceOccurrenceIDs: []string{"tile-a-label-1", "tile-b-label-2"}, ReadingStatus: "readable", TextAR: "سالم", ReviewStatus: "verified"}},
	}
	input := ChartAnalysisReviewPayload{
		Names:    []ChartNameOccurrence{{ID: "name", PageNumber: 99, Region: ChartBox{X: .11, Y: .2, Width: .1, Height: .1}, SourceOccurrenceIDs: []string{"forged"}, ReadingStatus: "readable", TextAR: "سالم", ReviewStatus: "verified"}},
		Branches: []ChartBranchRegion{{ID: "branch", PageNumber: 99, Polygon: []ChartPoint{{0, 0}, {1, 0}, {1, 1}}}},
	}
	result := normalizeChartReview(current, input)
	name := result.Names[0]
	if name.PageNumber != 1 || name.TextAR != "" || name.ReadingStatus != "unreadable" || name.ReviewStatus != "needs_review" {
		t.Fatalf("changed crop retained stale model reading/review: %+v", name)
	}
	if len(name.SourceOccurrenceIDs) != 2 || name.SourceOccurrenceIDs[0] != "tile-a-label-1" {
		t.Fatalf("source provenance was not preserved: %+v", name.SourceOccurrenceIDs)
	}
	if result.Branches[0].PageNumber != 1 || result.Branches[0].ReviewStatus != "needs_review" {
		t.Fatalf("branch page/review state was not normalized: %+v", result.Branches[0])
	}
}

func TestChartResponseAndSetupErrorCodesAreConservative(t *testing.T) {
	if err := validateChartTileReading(ChartTileReading{Labels: []ChartLabelProposal{{Box: ChartBox{X: .8, Y: .2, Width: .3, Height: .1}, Legibility: "clear"}}}); err == nil {
		t.Fatal("out-of-tile label geometry was accepted")
	}
	if got := chartErrorCode(ErrChartVisionSetup); got != "hosted_vision_setup_required" {
		t.Fatalf("setup failure mapped to %q", got)
	}
	if got := chartErrorCode(ErrChartAnalysisBudget); got != "request_budget_exhausted" {
		t.Fatalf("budget failure mapped to %q", got)
	}
	if got := chartErrorCode(chartVisionOutputValidationError{category: "branch_reference"}); got != "provider_output_invalid_branch_reference" {
		t.Fatalf("provider output category mapped to %q", got)
	}
}
