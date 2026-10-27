package sourceprocessing

import (
	"bytes"
	"context"
	"image"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reading names at the resolution of the SOURCE, not the resolution of a page
// preview.
//
// The localiser was calibrated on a page rendered to 4000 pixels on its longest
// side. That is the right size for finding where names are on an ordinary sheet,
// and the wrong size for reading them on a wall chart. The آل الصباح chart is
// 59 inches wide; at 4000 pixels it is 68 DPI, and a name is a dozen pixels
// tall. The same chart is vector art, so the PDF can draw that name at any size
// - the detail is in the file, and the preview threw it away.
//
// This file renders the page in tiles at the analysis resolution (240 DPI), runs
// the localiser on each tile, and reads each name from the tile it was found on.
// The tiles overlap, and each name is assigned to exactly one tile by its CENTRE:
// a name whose centre is inside a tile's own strip is detected there with enough
// surrounding context to be complete, and no name is read twice. The result is
// the same product pipeline as before, with a sharper input.
const (
	// nameReadingTileAxis is the size of a reading tile, in source pixels. At
	// 240 DPI this is eight inches of a chart, enough for a few dozen names and
	// small enough that a tile stays far below the render memory bound.
	nameReadingTileAxis = 1920
	// nameReadingTileOverlap is how much source, in pixels, a tile renders around
	// its ownership strip. A name is assigned by its centre, so the overlap only
	// has to be wider than half a name: 256 pixels covers a long Arabic name at
	// 240 DPI with room to spare.
	nameReadingTileOverlap = 256
	// nameReadingTileMaxAreaFraction is the largest share of a TILE a name group
	// may cover. It is a tile fraction and not a page fraction on purpose: on a
	// wall chart a page fraction is so large that a paragraph would pass. A name
	// on a 1920-pixel tile is a few thousand pixels, well under one percent.
	nameReadingTileMaxAreaFraction = 0.01
	// nameReadingModel records that the regions were localised on source tiles.
	// The algorithm is the same; the input resolution is not, and a result that
	// cannot be traced to the input that produced it is not auditable.
	nameReadingModel = carrierReadingModel + "+source-tiles-v1"
)

// PageNameReading is one name region found on a page, with the reading of that
// region when a reader was configured.
type PageNameReading struct {
	Region  PageRegion     `json:"region"`
	Reading CarrierReading `json:"reading"`
	// Read reports whether a reader was asked about this region. It is false
	// when the localiser ran without a reader, which is a detection-only run and
	// must not be mistaken for a page of unreadable names.
	Read bool `json:"read"`
}

type nameReadingTile struct {
	// Ownership is the strip of the page this tile is responsible for. A name
	// belongs to the tile whose strip contains its centre.
	Ownership ChartTileBounds
	// Render is the larger area drawn from the source, so that a name centred in
	// Ownership is complete even when it crosses the strip's edge.
	Render ChartTileBounds
}

// planNameReadingTiles divides a page into tiles for source-resolution reading.
//
// A page that fits in one bounded render stays one tile. That matters for the
// many charts that are ordinary sheets: tiling them would cost several renders
// and change the calibrated localiser's view for no gain.
func planNameReadingTiles(width, height int) []nameReadingTile {
	if width < 1 || height < 1 {
		return nil
	}
	if int64(width)*int64(height) <= maxRenderedPixels {
		bounds := ChartTileBounds{Width: width, Height: height}
		return []nameReadingTile{{Ownership: bounds, Render: bounds}}
	}
	columns := nameReadingAxisSegments(width)
	rows := nameReadingAxisSegments(height)
	tiles := make([]nameReadingTile, 0, len(columns)*len(rows))
	for _, column := range columns {
		for _, row := range rows {
			renderLeft := max(0, column[0]-nameReadingTileOverlap)
			renderTop := max(0, row[0]-nameReadingTileOverlap)
			renderRight := min(width, column[1]+nameReadingTileOverlap)
			renderBottom := min(height, row[1]+nameReadingTileOverlap)
			tiles = append(tiles, nameReadingTile{
				Ownership: ChartTileBounds{X: column[0], Y: row[0], Width: column[1] - column[0], Height: row[1] - row[0]},
				Render:    ChartTileBounds{X: renderLeft, Y: renderTop, Width: renderRight - renderLeft, Height: renderBottom - renderTop},
			})
		}
	}
	return tiles
}

// nameReadingAxisSegments partitions one axis into owned strips of at most the
// tile stride. The render around a strip adds the overlap.
func nameReadingAxisSegments(length int) [][2]int {
	stride := nameReadingTileAxis - nameReadingTileOverlap
	if length <= stride {
		return [][2]int{{0, length}}
	}
	segments := make([][2]int, 0, length/stride+1)
	for start := 0; start < length; start += stride {
		end := min(start+stride, length)
		segments = append(segments, [2]int{start, end})
	}
	return segments
}

// nameReadingTileConfig expresses the page-calibrated thresholds in the tile's
// pixel space. The localiser's fractions describe the PAGE - the gap between
// letters of a name is calibrated against the page, not against whatever crop it
// is shown - so on a tile they have to be scaled by how much of the page the
// tile covers. Done wrong, a tiled run either splits every name or merges a
// branch, and neither failure is visible in the group count.
func nameReadingTileConfig(base CarrierLocaliserConfig, pageWidth, pageHeight, tileWidth, tileHeight int, tiled bool) CarrierLocaliserConfig {
	config := base
	if tileWidth > 0 {
		config.NameGroupGapFraction = base.NameGroupGapFraction * float64(max(pageWidth, pageHeight)) / float64(max(tileWidth, tileHeight))
	}
	if tileHeight > 0 {
		config.NameGroupMinHeightFraction = base.NameGroupMinHeightFraction * float64(pageHeight) / float64(tileHeight)
	}
	if tiled {
		// On a tile, a page fraction for the maximum group no longer excludes a
		// paragraph: the page is 50 tiles big, so a block of prose passes. Cap
		// the group against the tile instead.
		config.NameGroupMaxAreaFraction = nameReadingTileMaxAreaFraction
	}
	return config
}

// ReadNamesOnPage finds every name on a page at the sharpest resolution the
// document can provide, and reads each one in isolation.
//
// It returns regions in page-normalised coordinates, ordered top-to-bottom and
// then left-to-right, so the reviewer sees names in reading order. When provider
// is nil the regions are still returned, as detection without transcription.
func ReadNamesOnPage(ctx context.Context, page *ChartPDFPage, provider ChartVisionProvider, localiser CarrierLocaliserConfig, readerConfig CarrierReaderConfig) ([]PageNameReading, error) {
	if page == nil || page.Width < 1 || page.Height < 1 {
		return nil, ErrValidation
	}
	tiles := planNameReadingTiles(page.Width, page.Height)
	if len(tiles) == 0 {
		return nil, ErrValidation
	}
	tiled := len(tiles) > 1
	results := make([]PageNameReading, 0, 128)
	for _, tile := range tiles {
		tilePNG, err := renderNameReadingTile(ctx, page, tile.Render)
		if err != nil {
			return nil, err
		}
		config := nameReadingTileConfig(localiser, page.Width, page.Height, tile.Render.Width, tile.Render.Height, tiled)
		regions, err := DetectNameCarriers(tilePNG, config)
		if err != nil {
			return nil, err
		}
		owned := make([]PageRegion, 0, len(regions))
		tileRegions := make([]PageRegion, 0, len(regions))
		for _, region := range regions {
			if region.Kind != "name_group" {
				continue
			}
			// The group is in tile-normalised coordinates. Convert it to page
			// pixels twice: once for the ownership test and once for the
			// reviewer's region, which always refers to the whole page.
			left := float64(tile.Render.X) + region.X*float64(tile.Render.Width)
			top := float64(tile.Render.Y) + region.Y*float64(tile.Render.Height)
			width := region.Width * float64(tile.Render.Width)
			height := region.Height * float64(tile.Render.Height)
			centreX := left + width/2
			centreY := top + height/2
			if centreX < float64(tile.Ownership.X) || centreX >= float64(tile.Ownership.X+tile.Ownership.Width) ||
				centreY < float64(tile.Ownership.Y) || centreY >= float64(tile.Ownership.Y+tile.Ownership.Height) {
				continue
			}
			tileRegions = append(tileRegions, PageRegion{
				Kind:   "name_group",
				X:      (left - float64(tile.Render.X)) / float64(tile.Render.Width),
				Y:      (top - float64(tile.Render.Y)) / float64(tile.Render.Height),
				Width:  region.Width,
				Height: region.Height,
			})
			owned = append(owned, PageRegion{
				Kind:   "name_group",
				X:      left / float64(page.Width),
				Y:      top / float64(page.Height),
				Width:  width / float64(page.Width),
				Height: height / float64(page.Height),
			})
		}
		if len(owned) == 0 {
			continue
		}

		readings := make([]CarrierReading, 0, len(owned))
		if provider != nil {
			decoded, _, decodeErr := image.Decode(bytes.NewReader(tilePNG))
			if decodeErr != nil {
				return nil, ErrPDFProcessingFailed
			}
			readings, err = ReadCarriers(ctx, provider, decoded, tileRegions, readerConfig)
			if err != nil {
				return nil, err
			}
		}
		byIndex := make(map[int]CarrierReading, len(readings))
		for _, reading := range readings {
			index, ok := carrierOccurrenceIndex(reading.OccurrenceID)
			if !ok || index < 0 || index >= len(owned) {
				continue
			}
			byIndex[index] = reading
		}
		for index, region := range owned {
			item := PageNameReading{Region: region}
			if reading, ok := byIndex[index]; ok {
				item.Reading = reading
				item.Read = true
			}
			results = append(results, item)
		}
	}

	sort.SliceStable(results, func(left, right int) bool {
		if results[left].Region.Y != results[right].Region.Y {
			return results[left].Region.Y < results[right].Region.Y
		}
		return results[left].Region.X < results[right].Region.X
	})
	return results, nil
}

// renderNameReadingTile renders one tile, retrying once. A transient Poppler
// failure should not lose a page; a persistent one should be reported rather
// than silently returning a page with a band of names missing.
func renderNameReadingTile(ctx context.Context, page *ChartPDFPage, bounds ChartTileBounds) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		data, err := page.RenderRegion(ctx, bounds)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil, lastErr
}

// carrierOccurrenceIndex parses the index from a reader occurrence id.
func carrierOccurrenceIndex(occurrence string) (int, bool) {
	trimmed := strings.TrimPrefix(occurrence, "carrier-")
	if trimmed == occurrence {
		return 0, false
	}
	index, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return index, true
}
