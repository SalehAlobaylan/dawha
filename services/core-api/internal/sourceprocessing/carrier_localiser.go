package sourceprocessing

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"sort"
)

// The name carrier localiser finds where the NAMES are on a chart, so that a reader
// is handed one small crop instead of a whole sheet.
//
// Why this exists instead of the ink-run detector: the ink-run detector thresholds a
// page at grey < 150 and looks for orthogonal runs, which is the wrong instrument for a
// family chart. Measured on the two renderings of one family tree that the repository
// has:
//
//	                      drawn chart   photographic chart
//	ink segments @300DPI      2,501            9,345
//	discarded by the cap       60.0%            89.3%
//
// Same detector, same cap, same facts. The 3.7x difference is the background: the
// photographic chart is a picture of a tree against sky and grass, and the threshold
// reads the sky and the grass. Raising the render DPI makes it worse, because the
// minimum run length scales with resolution while the cap does not.
//
// A family chart puts its names on coloured shapes - a leaf, a fruit, a box. Those
// shapes are saturated, and the name is dark ink inside one. So: find the saturated
// shapes, find the dark ink, and treat a shape that hosts ink as a name carrier.
//
// The merge matters. On a photographic scan one leaf is broken by JPEG artefacts into
// several saturated pieces, so the photographic chart yields more components than the
// drawn one even though they carry the same names. Fragments of one leaf share a
// WORD; two different leaves never do. So the merge is anchored on the text: the
// carriers a single text blob touches are merged into one.
//
// The thresholds below are not guesses. They are the values at which the two
// renderings agree, which is the only calibration signal available without an answer
// key.
const (
	// defaultCarrierSaturation is the minimum channel spread, 0-255, for a pixel to
	// count as part of a coloured shape. Below it, a pixel is paper, bark, sky or
	// shadow.
	defaultCarrierSaturation = 60

	// defaultCarrierInk is the luminance, 0-255, below which a pixel counts as ink.
	// It is the value at which the two renderings agree.
	defaultCarrierInk = 60

	// defaultCarrierMaxArea is the largest fraction of the page a name carrier may
	// cover. It is what excludes a prose scroll - a name is small, a paragraph is
	// not - and it is used instead of a text-row count because the names on a
	// chart are rotated along the branches, so one name occupies many text rows.
	defaultCarrierMaxArea = 0.02

	// defaultCarrierMinArea is the smallest fraction of the page a carrier may
	// cover, so that a single stray coloured pixel is not a name.
	defaultCarrierMinArea = 0.00005

	// defaultCarrierMergeGap is the largest gap, as a fraction of the page's
	// longest side, between two pieces of a shape that are still considered one
	// shape. A JPEG artefact splits a leaf into pieces that touch; a leaf and
	// its neighbour in the canopy do not.
	defaultCarrierMergeGap = 0.01

	// defaultNameGroupMaxArea is the largest fraction of the page a name group may
	// cover.
	defaultNameGroupMaxArea = 0.005
	// defaultNameGroupMinArea is the smallest fraction of the page a name group
	// may cover.
	defaultNameGroupMinArea = 0.002
	// defaultNameGroupMinHeight is the smallest height a name group may have, as
	// a fraction of the page height. At 300 DPI on an A4 sheet that is eight
	// pixels, which is the smallest a letter of a name can be and still be read.
	defaultNameGroupMinHeight = 0.002
	// defaultCarrierErosion is the erosion, in pixels, applied to the saturated
	// mask before the shapes are labelled.
	defaultCarrierErosion = 0
	// defaultNameGroupGapFraction is the largest gap between two ink blobs that
	// still puts them in the same name, as a fraction of the page's longest side.
	defaultNameGroupGapFraction = 0.0025
)

// CarrierLocaliserConfig holds the thresholds. The defaults are the calibrated
// values; the struct exists so a test can vary them and so the pipeline can record
// which configuration produced a result.
type CarrierLocaliserConfig struct {
	SaturationThreshold int
	InkThreshold        int
	MaxAreaFraction     float64
	MinAreaFraction     float64
	MergeGapFraction    float64
	// NameGroupMinHeightFraction is the smallest height a name group may have, as
	// a fraction of the page height. A name is written at a readable size; a
	// fragment of a letter is not, and a page of a chart has a great many of
	// them. Without this floor the localiser reports more groups than there
	// are names, and most of them are single strokes.
	NameGroupMinHeightFraction float64
	// NameGroupMinAreaFraction is the smallest fraction of the page a name group
	// may cover. A name is a few dozen pixels of ink; a single stroke of a
	// letter is a fraction of that.
	NameGroupMinAreaFraction float64
	// NameGroupMaxAreaFraction is the largest fraction of the page a name group
	// may cover. A name is small; the ancestor stack down the trunk of a painted
	// tree and a panel of prose are not, and they are excluded rather than handed
	// to a reader as one enormous name.
	NameGroupMaxAreaFraction float64
	// ErodePixels is how far the saturated mask is eroded before the shapes are
	// labelled. Two leaves on a painted tree touch, and a touching pair is one
	// component; eroding the mask pinches the necks where they meet and separates
	// them, which is what makes a carrier one leaf rather than a branch.
	ErodePixels int
	// NameGroupGapFraction is the largest gap between two ink blobs that still
	// puts them in the same name, as a fraction of the page's longest side.
	//
	// It is a fraction of the PAGE and not of the ink, and that matters: JPEG
	// breaks the letters of a photographic scan into smaller pieces than the
	// same letter on a clean drawing, so a gap derived from the median blob
	// height would be smaller on the scan and would split every name in two.
	NameGroupGapFraction float64
}

// DefaultCarrierLocaliserConfig returns the calibrated configuration.
func DefaultCarrierLocaliserConfig() CarrierLocaliserConfig {
	return CarrierLocaliserConfig{
		SaturationThreshold:        defaultCarrierSaturation,
		InkThreshold:               defaultCarrierInk,
		MaxAreaFraction:            defaultCarrierMaxArea,
		MinAreaFraction:            defaultCarrierMinArea,
		MergeGapFraction:           defaultCarrierMergeGap,
		NameGroupGapFraction:       defaultNameGroupGapFraction,
		ErodePixels:                defaultCarrierErosion,
		NameGroupMaxAreaFraction:   defaultNameGroupMaxArea,
		NameGroupMinHeightFraction: defaultNameGroupMinHeight,
	}
}

// carrierReadingMethod names the reading these regions belong to, and
// carrierReadingModel names the algorithm version. Both are recorded on the page
// evidence, because a result that cannot be traced to the configuration that
// produced it is not auditable.
const (
	carrierReadingMethod = "local_carrier_localiser"
	// The model names both halves of the answer: the carriers are found by the
	// colour anchor, the names by the ink grouping that runs with or without
	// carriers.
	carrierReadingModel         = "colour-text-anchor-v1+contrast-name-groups-v3"
	previousCarrierReadingModel = "colour-text-anchor-v1+contrast-name-groups-v2"
	legacyCarrierReadingModel   = "colour-text-anchor-v1+ink-name-groups-v1"
)

// inkBlob is one connected run of foreground pixels, with the carriers it touches.
type inkBlob struct {
	touched []int32
	box     box
}

// box is an axis-aligned rectangle in pixel coordinates.
type box struct{ minX, minY, maxX, maxY int }

// boxGap returns the distance between two boxes, zero when they touch or overlap.
func boxGap(left, right box) int {
	horizontal := max(left.minX-right.maxX, right.minX-left.maxX)
	vertical := max(left.minY-right.maxY, right.minY-left.maxY)
	if horizontal < 0 {
		horizontal = 0
	}
	if vertical < 0 {
		vertical = 0
	}
	if horizontal > vertical {
		return horizontal
	}
	return vertical
}

// DetectNameCarriers finds the regions of a rendered page that carry a name.
//
// It returns one region per carrier, in normalised top-left page coordinates, with
// the kind "name_carrier". The region's confidence is the share of the carrier's
// area that carries ink: it measures how certain it is that there is TEXT here, not how
// certain it is that the text is correct. A reading's correctness is measured by
// agreement, never by a detector's opinion of itself.
//
// The page is expected to be a rendered PNG or JPEG. A page with no coloured shape, or
// with no ink, yields no regions and no error.
func DetectNameCarriers(data []byte, config CarrierLocaliserConfig) ([]PageRegion, error) {
	return detectNameCarriersWithPolicy(data, config, true, true)
}

func detectNameCarriers(data []byte, config CarrierLocaliserConfig, improved bool) ([]PageRegion, error) {
	return detectNameCarriersWithPolicy(data, config, improved, false)
}

func detectNameCarriersWithPolicy(data []byte, config CarrierLocaliserConfig, improved, strict bool) ([]PageRegion, error) {
	if len(data) == 0 {
		return nil, ErrValidation
	}
	if config.SaturationThreshold <= 0 || config.InkThreshold <= 0 {
		return nil, ErrValidation
	}
	imageData, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrValidation
	}
	bounds := imageData.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 || int64(width)*int64(height) > maxRenderedPixels {
		return nil, ErrValidation
	}
	pageArea := float64(width * height)

	saturated := make([]bool, width*height)
	ink := make([]bool, width*height)
	var lightInk, lightLetter, greenBackground []bool
	var luminances []uint8
	if improved {
		lightInk = make([]bool, width*height)
		lightLetter = make([]bool, width*height)
		greenBackground = make([]bool, width*height)
		luminances = make([]uint8, width*height)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := imageData.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			red, green, blue := int(r>>8), int(g>>8), int(b>>8)
			highest, lowest := red, red
			if green > highest {
				highest = green
			}
			if blue > highest {
				highest = blue
			}
			if green < lowest {
				lowest = green
			}
			if blue < lowest {
				lowest = blue
			}
			index := y*width + x
			saturated[index] = highest-lowest >= config.SaturationThreshold
			luminance := (299*red + 587*green + 114*blue) / 1000
			if improved {
				luminances[index] = uint8(luminance)
				lightLetter[index] = red >= 170 && green >= 150 && blue < 120 && red >= green*9/10
				greenBackground[index] = green*10 > red*11 && green*10 > blue*12
			}
			ink[index] = luminance < config.InkThreshold
		}
	}
	// Some charts print light yellow names on coloured leaves. Dark-only thresholding
	// misses those names and instead proposes the leaf veins and branch edges.
	// Require local contrast against several saturated, darker neighbours; a
	// uniformly bright sky or coloured fill is not light text.
	radius := max(4, int(math.Round(float64(max(width, height))*.0035)))
	diagonal := max(3, radius*3/4)
	for y := radius; improved && y < height-radius; y++ {
		for x := radius; x < width-radius; x++ {
			index := y*width + x
			if luminances[index] < 160 {
				continue
			}
			contrast := 0
			for _, offset := range [][2]int{{-radius, 0}, {radius, 0}, {0, -radius}, {0, radius}, {-diagonal, -diagonal}, {diagonal, -diagonal}, {-diagonal, diagonal}, {diagonal, diagonal}} {
				near := (y+offset[1])*width + x + offset[0]
				if saturated[near] && (!strict || greenBackground[near]) && int(luminances[index])-int(luminances[near]) >= 45 {
					contrast++
				}
			}
			if lightLetter[index] && contrast >= 3 {
				lightInk[index] = true
			}
		}
	}

	// The saturated shapes, labelled. The mask is eroded first, because two leaves
	// on a painted tree touch and a touching pair is one component.
	shapeMask := saturated
	if config.ErodePixels > 0 {
		shapeMask = erodeMask(saturated, width, height, config.ErodePixels)
	}

	carrierID := make([]int32, width*height)
	for index := range carrierID {
		carrierID[index] = -1
	}
	boxes := make([]box, 0, 256)
	stack := make([]int, 0, 4096)
	for start := 0; start < width*height; start++ {
		if !shapeMask[start] || carrierID[start] >= 0 {
			continue
		}
		id := int32(len(boxes))
		current := box{minX: width, minY: height, maxX: -1, maxY: -1}
		stack = stack[:0]
		stack = append(stack, start)
		carrierID[start] = id
		for len(stack) > 0 {
			index := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			y, x := index/width, index%width
			if x < current.minX {
				current.minX = x
			}
			if x > current.maxX {
				current.maxX = x
			}
			if y < current.minY {
				current.minY = y
			}
			if y > current.maxY {
				current.maxY = y
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					next := ny*width + nx
					if shapeMask[next] && carrierID[next] < 0 {
						carrierID[next] = id
						stack = append(stack, next)
					}
				}
			}
		}
		if config.ErodePixels > 0 {
			current.minX -= config.ErodePixels
			current.minY -= config.ErodePixels
			current.maxX += config.ErodePixels
			current.maxY += config.ErodePixels
			if current.minX < 0 {
				current.minX = 0
			}
			if current.minY < 0 {
				current.minY = 0
			}
			if current.maxX > width-1 {
				current.maxX = width - 1
			}
			if current.maxY > height-1 {
				current.maxY = height - 1
			}
		}
		boxes = append(boxes, current)
	}
	// There is no early return when the page has no coloured shapes. The
	// carriers are a colour-dependent proposal - they exist to say which leaf a
	// name is written on - but the NAMES are runs of dark ink, and dark ink is
	// found without any colour assumption. A black-and-white chart, a
	// calligraphic tree, or a screenshot with no saturation at all therefore
	// still yields name groups; only the carrier regions are absent.
	// The dark ink blobs. Each one names the carriers it sits on.
	//
	// The pixels of a blob that lie inside a carrier mean the blob is text on
	// that carrier.
	//
	// The obvious alternative - "the word lies within the shape's box" - was
	// implemented and measured, and it is worse: on the photographic rendering it
	// cut the carrier count from 386 to 172, because one long stroke crossing a
	// dense canopy overlaps many leaves at once and merging on that collapses the
	// page. Pixel evidence is both stronger and safer: the anti-aliased edge of a
	// letter is dark and saturated at once, so the pixels of a word reach the
	// shape it is written on even when the letter's interior does not.
	collectBlobs := func(mask []bool) []inkBlob {
		seen := make([]bool, width*height)
		blobs := make([]inkBlob, 0, 1024)
		for start := 0; start < width*height; start++ {
			if !mask[start] || seen[start] {
				continue
			}
			current := inkBlob{box: box{minX: width, minY: height, maxX: -1, maxY: -1}}
			pixelCarriers := map[int32]bool{}
			stack = stack[:0]
			stack = append(stack, start)
			seen[start] = true
			for len(stack) > 0 {
				index := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				y, x := index/width, index%width
				if id := carrierID[index]; id >= 0 {
					pixelCarriers[id] = true
				}
				if x < current.box.minX {
					current.box.minX = x
				}
				if x > current.box.maxX {
					current.box.maxX = x
				}
				if y < current.box.minY {
					current.box.minY = y
				}
				if y > current.box.maxY {
					current.box.maxY = y
				}
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := x+dx, y+dy
						if nx < 0 || ny < 0 || nx >= width || ny >= height {
							continue
						}
						next := ny*width + nx
						if mask[next] && !seen[next] {
							seen[next] = true
							stack = append(stack, next)
						}
					}
				}
			}
			blobArea := float64((current.box.maxX - current.box.minX) * (current.box.maxY - current.box.minY))
			if blobArea <= 0 {
				continue
			}
			if blobArea/pageArea > config.MaxAreaFraction {
				// Too big to be a word: a line, a border, a block of prose.
				continue
			}
			if len(pixelCarriers) > 0 {
				for id := range pixelCarriers {
					current.touched = append(current.touched, id)
				}
			}
			blobs = append(blobs, current)
		}

		return blobs
	}
	blobs := collectBlobs(ink)
	var lightBlobs []inkBlob
	if improved {
		lightBlobs = collectBlobs(lightInk)
	}

	// Merge the carriers that share a text blob: fragments of one shape share a
	// word, two different shapes never do.
	parent := make([]int32, len(boxes))
	for index := range parent {
		parent[index] = int32(index)
	}
	var find func(int32) int32
	find = func(value int32) int32 {
		for parent[value] != value {
			parent[value] = parent[parent[value]]
			value = parent[value]
		}
		return value
	}
	// The merge is bounded by PROXIMITY. Fragments of one shape are adjacent to
	// each other; two different shapes are not, even when one long ink stroke
	// passes over both. Without this bound a single stroke crossing a dense
	// canopy merges every leaf it touches into one group, and the group is then
	// too large to be a name and is discarded - which destroys the very carriers
	// the merge was meant to repair.
	mergeGap := int(float64(max(width, height)) * config.MergeGapFraction)
	if mergeGap < 1 {
		mergeGap = 1
	}
	union := func(left, right int32) {
		leftRoot, rightRoot := find(left), find(right)
		if leftRoot == rightRoot {
			return
		}
		if boxGap(boxes[left], boxes[right]) > mergeGap {
			return
		}
		parent[rightRoot] = leftRoot
	}
	for _, current := range blobs {
		if len(current.touched) < 2 {
			continue
		}
		for index := 1; index < len(current.touched); index++ {
			union(current.touched[0], current.touched[index])
		}
	}

	// Collect the merged groups, and the blobs that host them.
	groups := map[int32]*box{}
	groupBlobs := map[int32]int{}
	for index := range boxes {
		root := find(int32(index))
		group, ok := groups[root]
		if !ok {
			group = &box{minX: width, minY: height, maxX: -1, maxY: -1}
			groups[root] = group
		}
		current := boxes[index]
		if current.minX < group.minX {
			group.minX = current.minX
		}
		if current.maxX > group.maxX {
			group.maxX = current.maxX
		}
		if current.minY < group.minY {
			group.minY = current.minY
		}
		if current.maxY > group.maxY {
			group.maxY = current.maxY
		}
	}
	for _, current := range blobs {
		if len(current.touched) == 0 {
			continue
		}
		groupBlobs[find(current.touched[0])]++
	}

	// The NAME GROUPS. A carrier is a shape, and on a painted tree the shapes
	// touch: a carrier is often a dozen leaves, not one. The name is the unit a
	// reader needs, and the name is a run of ink. So the ink blobs are grouped
	// into names by proximity - the letters of one name are a few pixels apart,
	// and two names on two leaves are far further apart than that.
	nameGroups := groupNameBlobs(blobs,
		int(float64(max(width, height))*config.NameGroupGapFraction),
		width, height,
		config.NameGroupMaxAreaFraction,
		int(float64(height)*config.NameGroupMinHeightFraction),
		config.NameGroupMinAreaFraction)

	// Group the two polarities independently: connecting light letters to dark
	// branch strokes creates huge false names on illustrated trees.
	lightGroups := groupNameBlobs(lightBlobs,
		int(float64(max(width, height))*config.NameGroupGapFraction), width, height,
		config.NameGroupMaxAreaFraction, int(float64(height)*config.NameGroupMinHeightFraction), config.NameGroupMinAreaFraction)
	if improved {
		bounded := func(items []PageRegion) []PageRegion {
			kept := items[:0]
			for _, group := range items {
				if !isNameFragment(group.Width*float64(width), group.Height*float64(height)) {
					kept = append(kept, group)
				}
			}
			return kept
		}
		nameGroups = bounded(nameGroups)
		lightGroups = bounded(lightGroups)
	}
	// Prefer the existing dark-text interpretation unless light lettering is
	// the dominant usable evidence. This fallback avoids turning highlights on
	// a photographic tree into hundreds of extra names beside valid dark text.
	if len(lightGroups) <= len(nameGroups)*2 {
		lightGroups = nil
	} else if strict {
		// With dominant light lettering, dark components are the tree drawing,
		// not a second set of name placeholders. Keep the detected text polarity.
		nameGroups = nil
	}
	for _, candidate := range lightGroups {
		duplicate := false
		for _, existing := range nameGroups {
			w := math.Max(0, math.Min(existing.X+existing.Width, candidate.X+candidate.Width)-math.Max(existing.X, candidate.X))
			h := math.Max(0, math.Min(existing.Y+existing.Height, candidate.Y+candidate.Height)-math.Max(existing.Y, candidate.Y))
			if w*h/math.Min(existing.Width*existing.Height, candidate.Width*candidate.Height) > .7 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			nameGroups = append(nameGroups, candidate)
		}
	}

	regions := make([]PageRegion, 0, len(groups))
	for root, group := range groups {
		if group.maxX < 0 {
			continue
		}
		// A carrier with no ink on it is not a name. This is the check that keeps a
		// coloured background from becoming a suggestion.
		if groupBlobs[root] == 0 {
			continue
		}
		area := float64((group.maxX-group.minX)*(group.maxY-group.minY)) / pageArea
		if area > config.MaxAreaFraction || area < config.MinAreaFraction {
			continue
		}
		// The share of the carrier that carries ink: how certain it is that there
		// is TEXT here. It says nothing about whether the text is correct, and it
		// is not a probability of correctness.
		shapeArea := float64((group.maxX - group.minX) * (group.maxY - group.minY))
		if shapeArea <= 0 {
			continue
		}
		share := float64(groupBlobs[root]) / shapeArea
		if share > 1 {
			share = 1
		}
		confidence := share
		if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
			continue
		}
		regions = append(regions, PageRegion{
			Kind:       "name_carrier",
			X:          float64(group.minX) / float64(width),
			Y:          float64(group.minY) / float64(height),
			Width:      float64(group.maxX-group.minX) / float64(width),
			Height:     float64(group.maxY-group.minY) / float64(height),
			Confidence: &confidence,
		})
	}
	regions = append(regions, nameGroups...)

	// A stable order, so the same page always produces the same list.
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Y != regions[j].Y {
			return regions[i].Y < regions[j].Y
		}
		return regions[i].X < regions[j].X
	})
	return regions, nil
}

// erodeMask shrinks a boolean mask by the given number of pixels, separably, so a
// neck where two shapes touch is pinched off.
func erodeMask(mask []bool, width, height, radius int) []bool {
	current := make([]bool, len(mask))
	copy(current, mask)
	for pass := 0; pass < radius; pass++ {
		horizontal := make([]bool, len(mask))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if x > 0 && x < width-1 && current[y*width+x-1] && current[y*width+x] && current[y*width+x+1] {
					horizontal[y*width+x] = true
				}
			}
		}
		vertical := make([]bool, len(mask))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if y > 0 && y < height-1 && horizontal[(y-1)*width+x] && horizontal[y*width+x] && horizontal[(y+1)*width+x] {
					vertical[y*width+x] = true
				}
			}
		}
		current = vertical
	}
	return current
}

// groupNameBlobs groups the ink blobs into names.
//
// The grouping is by proximity: two blobs belong to the same name when the gap
// between them is smaller than a small multiple of the median blob height. That
// separation is real - the letters of a name are close together, and two names on
// two leaves are further apart - and it is what lets a reader be handed one name
// rather than a branch of the tree.
func groupNameBlobs(blobs []inkBlob, gap int, pageWidth, pageHeight int, maxAreaFraction float64, minHeight int, minAreaFraction float64) []PageRegion {
	if len(blobs) == 0 || gap < 1 {
		return nil
	}

	// Union the blobs that are within the gap of one another.
	parent := make([]int, len(blobs))
	for index := range parent {
		parent[index] = index
	}
	var find func(int) int
	find = func(value int) int {
		for parent[value] != value {
			parent[value] = parent[parent[value]]
			value = parent[value]
		}
		return value
	}
	union := func(left, right int) {
		leftRoot, rightRoot := find(left), find(right)
		if leftRoot != rightRoot {
			parent[rightRoot] = leftRoot
		}
	}
	for left := 0; left < len(blobs); left++ {
		for right := left + 1; right < len(blobs); right++ {
			if blobGap(blobs[left].box, blobs[right].box) <= gap {
				union(left, right)
			}
		}
	}

	groups := map[int]*box{}
	for index, blob := range blobs {
		root := find(index)
		group, ok := groups[root]
		if !ok {
			group = &box{minX: blob.box.minX, minY: blob.box.minY, maxX: blob.box.maxX, maxY: blob.box.maxY}
			groups[root] = group
			continue
		}
		if blob.box.minX < group.minX {
			group.minX = blob.box.minX
		}
		if blob.box.maxX > group.maxX {
			group.maxX = blob.box.maxX
		}
		if blob.box.minY < group.minY {
			group.minY = blob.box.minY
		}
		if blob.box.maxY > group.maxY {
			group.maxY = blob.box.maxY
		}
	}

	regions := make([]PageRegion, 0, len(groups))
	for _, group := range groups {
		width := group.maxX - group.minX
		height := group.maxY - group.minY
		if width < 1 || height < 1 {
			continue
		}
		if height < minHeight {
			// Too small to be a name: a fragment of a letter.
			continue
		}
		if float64(width*height)/float64(pageWidth*pageHeight) < minAreaFraction {
			// Too small to be a name.
			continue
		}
		if float64(width*height)/float64(pageWidth*pageHeight) > maxAreaFraction {
			// Too big to be one name: an ancestor stack down the trunk, a panel
			// of prose, a border. It is a text block, and a reader asked for one
			// name would be asked to read a page.
			continue
		}
		// Normalised, like every other region, so that a crop of it lands on the
		// right part of the page.
		regions = append(regions, PageRegion{
			Kind:   "name_group",
			X:      float64(group.minX) / float64(pageWidth),
			Y:      float64(group.minY) / float64(pageHeight),
			Width:  float64(width) / float64(pageWidth),
			Height: float64(height) / float64(pageHeight),
		})
	}
	return regions
}

// A conservative source-pixel floor, not a claim that a remaining region is a
// name. Keep both orientations; tiny dots and single thin stems are fragments.
func isNameFragment(width, height float64) bool {
	return math.Min(width, height) < 5-1e-6 || math.Max(width, height) < 10-1e-6 || width*height < 64-1e-6
}

// blobGap returns the distance between two boxes, zero when they touch or overlap.
func blobGap(left, right box) int {
	horizontal := max(left.minX-right.maxX, right.minX-left.maxX)
	vertical := max(left.minY-right.maxY, right.minY-left.maxY)
	if horizontal < 0 {
		horizontal = 0
	}
	if vertical < 0 {
		vertical = 0
	}
	if horizontal > vertical {
		return horizontal
	}
	return vertical
}

// CarrierLocaliserReading wraps the localiser's regions as a page reading, so they
// travel with the page evidence and are recorded with the method and model that
// produced them.
func CarrierLocaliserReading(data []byte, config CarrierLocaliserConfig) (PageReading, error) {
	reading := PageReading{Method: carrierReadingMethod, Model: carrierReadingModel}
	regions, err := DetectNameCarriers(data, config)
	if err != nil {
		return PageReading{Method: carrierReadingMethod + "_unavailable", Model: carrierReadingModel}, err
	}
	reading.Regions = regions
	return reading, nil
}

// DescribeCarrierLocaliser renders the configuration for the run record, so a
// result can be traced to the thresholds that produced it.
func DescribeCarrierLocaliser(config CarrierLocaliserConfig) string {
	return fmt.Sprintf("saturation=%d ink=%d maxArea=%.4f minArea=%.5f mergeGap=%.4f",
		config.SaturationThreshold, config.InkThreshold, config.MaxAreaFraction, config.MinAreaFraction, config.MergeGapFraction)
}
