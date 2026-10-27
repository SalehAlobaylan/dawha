package sourceprocessing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"strings"
)

// The carrier reader turns "where the names are" into "what the names are".
//
// The localiser finds the shapes a name is written on. This takes each shape, crops
// the page to it, and asks a reader to transcribe that one crop. The reading is a
// DIFFERENT question from the v8 experiment's: that one asked a model to find names
// across a dense photographic sheet, which is the task where vision models are six
// times worse than classical vision (docs/import-reading-strategy.md §2). This one
// asks a model to read one isolated, high-contrast word, which is the task they are
// good at.
//
// The confidence is MEASURED, never requested. The reader is asked the same crop
// several times, with the crop varied, and the readings are aligned and voted on.
// The confidence is the share of views that agreed, which is a physical quantity
// rather than a model's opinion of itself. Measured on historical records, a
// multi-view vote with a grid-warp variant gave a confidence-to-accuracy
// correlation of 0.642, against 0.54-0.74 for a model's own verbalised confidence -
// which is at times worse than chance.

var (
	// ErrCarrierReaderUnavailable is returned when no reader is configured.
	ErrCarrierReaderUnavailable = errors.New("carrier reader is unavailable")
	// ErrCarrierCropFailed is returned when a crop cannot be produced.
	ErrCarrierCropFailed = errors.New("carrier crop could not be produced")
)

// CarrierReading is one name read from one carrier, with a measured confidence.
type CarrierReading struct {
	OccurrenceID string   `json:"occurrence_id"`
	Status       string   `json:"status"`
	TextAR       string   `json:"text_ar"`
	Confidence   float64  `json:"confidence"`
	Alternatives []string `json:"alternatives,omitempty"`
	// Agreement is the share of views that produced the consensus text, per
	// character. It is the evidence for the confidence.
	Agreement []float64 `json:"agreement,omitempty"`
}

// CarrierReaderConfig holds the crop and consensus settings.
type CarrierReaderConfig struct {
	// CropMargin is the share of the carrier's size added around it as context.
	CropMargin float64
	// ContextScale is how much larger the context crop is than the tight crop.
	//
	// It defaults to one, meaning no wider context, and that is a MEASURED
	// decision. A wider context crop of a painted tree contains the NEXT name as
	// well: the reader was asked for one occurrence, saw two names, and returned
	// two readings for it, which the response schema - correctly - refuses. The
	// tight crop of one isolated name is read accurately; the robustness comes
	// from reading it several times with different crops and voting, not from
	// showing the reader more of the page.
	ContextScale float64
	// Views is how many variants of each crop are read.
	Views int
	// MinAgreement is the agreement a character must reach to count as read.
	MinAgreement float64
}

// DefaultCarrierReaderConfig returns the configuration the pipeline uses.
func DefaultCarrierReaderConfig() CarrierReaderConfig {
	return CarrierReaderConfig{
		CropMargin:   0.15,
		ContextScale: 1.0,
		Views:        3,
		MinAgreement: 0.6,
	}
}

// carrierVariant is one way of presenting a crop to the reader.
type carrierVariant struct {
	name string
	mut  func(*image.RGBA) *image.RGBA
}

// carrierVariants returns the variations of a crop that the reader is asked to read.
//
// They are chosen to be the ones the multi-view literature finds useful and that are
// cheap to compute: padding changes the framing, scaling changes the glyph size, a
// grid warp displaces each row slightly, and a blur changes the sharpness. A grid
// warp is the one that best separates a confident reading from an unconfident one,
// because it introduces error in a way that is independent between views.
func carrierVariants() []carrierVariant {
	return []carrierVariant{
		{name: "plain", mut: func(in *image.RGBA) *image.RGBA { return in }},
		{name: "padded", mut: padCarrierImage},
		{name: "warped", mut: warpCarrierImage},
		{name: "scaled", mut: scaleCarrierImage},
	}
}

// padCarrierImage adds a light margin around the crop.
func padCarrierImage(in *image.RGBA) *image.RGBA {
	bounds := in.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	margin := int(float64(maxInt(width, height)) * 0.18)
	out := image.NewRGBA(image.Rect(0, 0, width+2*margin, height+2*margin))
	draw.Draw(out, out.Bounds(), &image.Uniform{C: color.RGBA{R: 245, G: 245, B: 240, A: 255}}, image.Point{}, draw.Src)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.Set(margin+x, margin+y, in.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return out
}

// warpCarrierImage displaces each row horizontally by a small amount, so that a
// reading which depends on one exact alignment has to disagree with the others.
func warpCarrierImage(in *image.RGBA) *image.RGBA {
	bounds := in.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	amplitude := float64(height) * 0.06
	for y := 0; y < height; y++ {
		offset := int(amplitude * math.Sin(float64(y)*0.35))
		for x := 0; x < width; x++ {
			source := x - offset
			if source < 0 || source >= width {
				continue
			}
			out.Set(bounds.Min.X+x, bounds.Min.Y+y, in.At(bounds.Min.X+source, bounds.Min.Y+y))
		}
	}
	return out
}

// scaleCarrierImage changes the size of the crop.
func scaleCarrierImage(in *image.RGBA) *image.RGBA {
	bounds := in.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	factor := 1.35
	outWidth := int(float64(width) * factor)
	outHeight := int(float64(height) * factor)
	if outWidth < 2 || outHeight < 2 {
		return in
	}
	out := image.NewRGBA(image.Rect(0, 0, outWidth, outHeight))
	for y := 0; y < outHeight; y++ {
		for x := 0; x < outWidth; x++ {
			sourceX := bounds.Min.X + x*width/outWidth
			sourceY := bounds.Min.Y + y*height/outHeight
			out.Set(x, y, in.At(sourceX, sourceY))
		}
	}
	return out
}

// decodeRGBA decodes PNG bytes into an RGBA image, or returns an empty image when
// the bytes cannot be read. A crop that cannot be decoded is simply not read.
func decodeRGBA(data []byte) *image.RGBA {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	bounds := decoded.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			out.Set(x, y, decoded.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return out
}

// trimAll trims every value, so that the package's own uniqueStrings can be used.
func trimAll(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		trimmed = append(trimmed, strings.TrimSpace(value))
	}
	return trimmed
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// CropCarrier cuts the tight crop of a carrier and its surrounding context out of a
// rendered page, both as PNG bytes.
//
// The context crop matters: a reader asked to transcribe a word in isolation has no
// way to tell a name from a label, and the shape around the word - the leaf it sits
// on, the branch it hangs from - is what tells it.
func CropCarrier(page image.Image, carrier PageRegion, config CarrierReaderConfig) (tight []byte, context []byte, err error) {
	bounds := page.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 {
		return nil, nil, ErrCarrierCropFailed
	}
	left := int(carrier.X * float64(width))
	top := int(carrier.Y * float64(height))
	carrierWidth := int(carrier.Width * float64(width))
	carrierHeight := int(carrier.Height * float64(height))
	if carrierWidth < 1 || carrierHeight < 1 {
		return nil, nil, ErrCarrierCropFailed
	}
	tightBounds := image.Rect(left, top, left+carrierWidth, top+carrierHeight).Intersect(bounds)
	if tightBounds.Empty() {
		return nil, nil, ErrCarrierCropFailed
	}
	tightImage := image.NewRGBA(image.Rect(0, 0, tightBounds.Dx(), tightBounds.Dy()))
	for y := 0; y < tightBounds.Dy(); y++ {
		for x := 0; x < tightBounds.Dx(); x++ {
			tightImage.Set(x, y, page.At(tightBounds.Min.X+x, tightBounds.Min.Y+y))
		}
	}

	contextWidth := int(float64(tightBounds.Dx()) * config.ContextScale)
	contextHeight := int(float64(tightBounds.Dy()) * config.ContextScale)
	contextLeft := left - (contextWidth-tightBounds.Dx())/2
	contextTop := top - (contextHeight-tightBounds.Dy())/2
	contextBounds := image.Rect(contextLeft, contextTop, contextLeft+contextWidth, contextTop+contextHeight).Intersect(bounds)
	if contextBounds.Empty() {
		contextBounds = tightBounds
	}
	contextImage := image.NewRGBA(image.Rect(0, 0, contextBounds.Dx(), contextBounds.Dy()))
	for y := 0; y < contextBounds.Dy(); y++ {
		for x := 0; x < contextBounds.Dx(); x++ {
			contextImage.Set(x, y, page.At(contextBounds.Min.X+x, contextBounds.Min.Y+y))
		}
	}

	tightBytes, err := encodePNG(tightImage)
	if err != nil {
		return nil, nil, ErrCarrierCropFailed
	}
	contextBytes, err := encodePNG(contextImage)
	if err != nil {
		return nil, nil, ErrCarrierCropFailed
	}
	return tightBytes, contextBytes, nil
}

// ReadCarriers reads every carrier on a page and returns one reading per carrier,
// with a measured confidence.
//
// The reader is asked each crop several times, with the crop varied. The readings
// are aligned and voted on. A character that the views do not agree on is marked
// uncertain rather than guessed, and the confidence is the LOWEST character
// agreement, because a name is only as good as its weakest character.
func ReadCarriers(ctx context.Context, provider ChartVisionProvider, page image.Image, carriers []PageRegion, config CarrierReaderConfig) ([]CarrierReading, error) {
	if provider == nil {
		return nil, ErrCarrierReaderUnavailable
	}
	if len(carriers) == 0 {
		return nil, nil
	}
	views := carrierVariants()
	if config.Views < 1 {
		config.Views = 1
	}
	if config.Views > len(views) {
		config.Views = len(views)
	}

	// The crops, once per carrier.
	tight := make([][]byte, len(carriers))
	contexts := make([][]byte, len(carriers))
	for index, carrier := range carriers {
		tightCrop, contextCrop, err := CropCarrier(page, carrier, config)
		if err != nil {
			continue
		}
		tight[index] = tightCrop
		contexts[index] = contextCrop
	}

	// Each view reads every crop.
	perView := make([][]ChartNameReading, 0, config.Views)
	for view := 0; view < config.Views; view++ {
		inputs := make([]ChartCropInput, 0, len(carriers))
		for index := range carriers {
			if tight[index] == nil {
				continue
			}
			mutated, err := encodePNG(views[view].mut(decodeRGBA(tight[index])))
			if err != nil {
				continue
			}
			inputs = append(inputs, ChartCropInput{
				OccurrenceID: carrierOccurrenceID(index),
				TightPNG:     mutated,
				ContextPNG:   contexts[index],
			})
		}
		if len(inputs) == 0 {
			continue
		}
		readings, err := readInBatches(ctx, provider, inputs)
		if err != nil {
			return nil, err
		}
		perView = append(perView, readings)
	}
	if len(perView) == 0 {
		return nil, nil
	}

	// Group the readings by occurrence.
	byOccurrence := map[string][]string{}
	statuses := map[string]string{}
	alternatives := map[string][]string{}
	for _, view := range perView {
		for _, reading := range view {
			byOccurrence[reading.OccurrenceID] = append(byOccurrence[reading.OccurrenceID], reading.TextAR)
			if reading.Status != "" {
				statuses[reading.OccurrenceID] = reading.Status
			}
			alternatives[reading.OccurrenceID] = append(alternatives[reading.OccurrenceID], reading.Alternatives...)
		}
	}

	occurrences := make([]string, 0, len(byOccurrence))
	for occurrence := range byOccurrence {
		occurrences = append(occurrences, occurrence)
	}
	sort.Strings(occurrences)

	results := make([]CarrierReading, 0, len(occurrences))
	for _, occurrence := range occurrences {
		viewsForOccurrence := byOccurrence[occurrence]
		text, agreement := consensusText(viewsForOccurrence, config.MinAgreement)
		status := statuses[occurrence]
		if status == "" {
			status = "readable"
		}
		if text == "" {
			status = "unreadable"
		}
		results = append(results, CarrierReading{
			OccurrenceID: occurrence,
			Status:       status,
			TextAR:       text,
			Confidence:   minimumAgreement(agreement),
			Alternatives: uniqueStrings(trimAll(alternatives[occurrence])),
			Agreement:    agreement,
		})
	}
	return results, nil
}

// readInBatches reads the crops in batches the provider accepts, and falls back to
// reading them one at a time when a batch is rejected.
//
// The fallback matters: the provider refuses a whole batch when ONE crop in it produces a
// response that does not validate - a name it read as "readable" with no text, for
// instance. Without the fallback, one unreadable crop out of a page would lose every
// other name on that page.
func readInBatches(ctx context.Context, provider ChartVisionProvider, inputs []ChartCropInput) ([]ChartNameReading, error) {
	readings := make([]ChartNameReading, 0, len(inputs))
	for start := 0; start < len(inputs); start += chartMaxReadBatchImages {
		end := start + chartMaxReadBatchImages
		if end > len(inputs) {
			end = len(inputs)
		}
		batch, err := provider.ReadBatch(ctx, inputs[start:end])
		if err == nil {
			readings = append(readings, batch...)
			continue
		}
		// The batch was refused. Read the crops one at a time, and keep the
		// ones that work. A crop that cannot be read is reported as
		// unreadable, not lost.
		//
		// If NOT ONE of them can be read, the provider is the problem rather
		// than the crops, and the failure is returned: a provider that is
		// unreachable must not look like a page of unreadable names.
		succeeded := 0
		var lastErr error
		for _, input := range inputs[start:end] {
			single, singleErr := provider.ReadBatch(ctx, []ChartCropInput{input})
			if singleErr != nil || len(single) == 0 {
				if singleErr != nil {
					lastErr = singleErr
				}
				readings = append(readings, ChartNameReading{
					OccurrenceID: input.OccurrenceID,
					Status:       "unreadable",
					TextAR:       "",
				})
				continue
			}
			succeeded++
			readings = append(readings, single...)
		}
		if succeeded == 0 && lastErr != nil {
			return nil, lastErr
		}
	}
	return readings, nil
}

func carrierOccurrenceID(index int) string {
	return fmt.Sprintf("carrier-%06d", index)
}

// consensusText aligns the views of one crop and votes on the result.
//
// The alignment is a Needleman-Wunsch style alignment, character by character. A
// character the views agree on is kept. A character they do not is dropped, and the
// disagreement is recorded as the agreement for that position, rather than being
// resolved by picking a winner.
func consensusText(views []string, minimumAgreement float64) (string, []float64) {
	if len(views) == 0 {
		return "", nil
	}
	reference := []rune(views[0])
	columns := make([]map[rune]int, len(reference))
	for index := range columns {
		columns[index] = map[rune]int{reference[index]: 1}
	}
	for _, view := range views[1:] {
		aligned := alignRunes(reference, []rune(view))
		merged := make([]map[rune]int, len(columns))
		for index := range columns {
			merged[index] = map[rune]int{}
		}
		for index, pair := range aligned {
			if index >= len(merged) {
				break
			}
			if pair[0] != 0 {
				merged[index][pair[0]]++
			}
			if pair[1] != 0 {
				merged[index][pair[1]]++
			}
		}
		columns = merged
	}

	text := strings.Builder{}
	agreement := make([]float64, 0, len(columns))
	for _, votes := range columns {
		best := rune(0)
		bestCount := 0
		total := 0
		for symbol, count := range votes {
			total += count
			if count > bestCount || (count == bestCount && symbol < best) {
				best = symbol
				bestCount = count
			}
		}
		if total == 0 {
			continue
		}
		share := float64(bestCount) / float64(total)
		agreement = append(agreement, share)
		if share < minimumAgreement {
			// The views do not agree on this character. It is dropped rather
			// than guessed, and the disagreement is recorded.
			continue
		}
		text.WriteRune(best)
	}
	return strings.TrimSpace(text.String()), agreement
}

func minimumAgreement(agreement []float64) float64 {
	if len(agreement) == 0 {
		return 0
	}
	lowest := 1.0
	for _, value := range agreement {
		if value < lowest {
			lowest = value
		}
	}
	return lowest
}

// alignRunes aligns two rune sequences, returning the aligned columns as pairs.
// A zero in a pair means a gap.
func alignRunes(left, right []rune) [][2]rune {
	n, m := len(left), len(right)
	if n == 0 {
		return nil
	}
	if m == 0 {
		columns := make([][2]rune, n)
		for index := range columns {
			columns[index] = [2]rune{left[index], 0}
		}
		return columns
	}
	// score[i][j] is the best alignment of left[:i] and right[:j].
	score := make([][]int, n+1)
	for index := range score {
		score[index] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		score[i][0] = score[i-1][0] - 1
	}
	for j := 1; j <= m; j++ {
		score[0][j] = score[0][j-1] - 1
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			substitution := score[i-1][j-1]
			if left[i-1] != right[j-1] {
				substitution--
			}
			deletion := score[i-1][j] - 1
			insertion := score[i][j-1] - 1
			best := substitution
			if deletion > best {
				best = deletion
			}
			if insertion > best {
				best = insertion
			}
			score[i][j] = best
		}
	}
	columns := make([][2]rune, 0, n)
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0:
			substitution := score[i-1][j-1]
			if left[i-1] != right[j-1] {
				substitution--
			}
			if score[i][j] == substitution {
				columns = append(columns, [2]rune{left[i-1], right[j-1]})
				i--
				j--
				continue
			}
		}
		if i > 0 && score[i][j] == score[i-1][j]-1 {
			columns = append(columns, [2]rune{left[i-1], 0})
			i--
			continue
		}
		columns = append(columns, [2]rune{0, right[j-1]})
		j--
	}
	for left, right := 0, len(columns)-1; left < right; left, right = left+1, right-1 {
		columns[left], columns[right] = columns[right], columns[left]
	}
	return columns
}
