package sourceprocessing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const LocalCropReaderVersion = "local-tesseract-crop-v1"
const NamePreprocessingVersion = "bounded-crop-variants-v1"

type CropObservations struct {
	NameHeight int     `json:"nameHeightPx"`
	Contrast   float64 `json:"contrast"`
	Clipped    bool    `json:"clipped"`
	Coloured   bool    `json:"coloured"`
	Reason     string  `json:"reason,omitempty"`
}
type LocalVariantReading struct {
	Variant      string   `json:"variant"`
	Segmentation int      `json:"segmentation"`
	RawText      string   `json:"rawText"`
	EngineScore  *float64 `json:"engineScore,omitempty"`
	Outcome      string   `json:"outcome"`
	ErrorCode    string   `json:"errorCode,omitempty"`
	DurationMS   int64    `json:"durationMs"`
	Bytes        int      `json:"bytes"`
}
type LocalCropReading struct {
	ReaderVersion        string                `json:"readerVersion"`
	PreprocessingVersion string                `json:"preprocessingVersion"`
	Outcome              string                `json:"outcome"`
	Text                 string                `json:"text"`
	Alternatives         []string              `json:"alternatives"`
	Variants             []LocalVariantReading `json:"variants"`
	Quality              CropObservations      `json:"quality"`
	Agreement            float64               `json:"measuredAgreement"`
}
type LocalCropReader interface {
	ReadCrop(context.Context, []byte, CropObservations) (LocalCropReading, error)
}
type TesseractCropReader struct {
	Binary   string
	Budget   time.Duration
	setupMu  sync.Mutex
	setupAt  time.Time
	setupErr error
}

var localOCRSlots = make(chan struct{}, 2)

func NewLocalCropReader() *TesseractCropReader {
	budget := 12 * time.Second
	if value, err := time.ParseDuration(os.Getenv("NAME_READER_CROP_BUDGET")); err == nil && value >= time.Second && value <= 30*time.Second {
		budget = value
	}
	return &TesseractCropReader{Binary: executable("TESSERACT_BINARY", "tesseract"), Budget: budget}
}
func (r *TesseractCropReader) ReadCrop(parent context.Context, pngBytes []byte, observations CropObservations) (LocalCropReading, error) {
	result := LocalCropReading{ReaderVersion: LocalCropReaderVersion, PreprocessingVersion: NamePreprocessingVersion, Outcome: "reading_unavailable", Alternatives: []string{}, Variants: []LocalVariantReading{}, Quality: observations}
	if err := parent.Err(); err != nil {
		return result, err
	}
	if len(pngBytes) == 0 || len(pngBytes) > 10_000_000 {
		return result, ErrValidation
	}
	config, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > suggestionCropMaxPixels {
		return result, ErrValidation
	}
	if observations.NameHeight > 0 && observations.NameHeight < 9 {
		result.Outcome = "manual_review"
		result.Quality.Reason = "insufficient_source_pixels"
		return result, nil
	}
	budget := r.Budget
	if budget <= 0 || budget > 30*time.Second {
		budget = 12 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	select {
	case localOCRSlots <- struct{}{}:
		defer func() { <-localOCRSlots }()
	case <-ctx.Done():
		if parent.Err() != nil {
			return result, parent.Err()
		}
		result.Quality.Reason = "crop_budget_exhausted"
		return result, nil
	}
	r.setupMu.Lock()
	if time.Since(r.setupAt) > 30*time.Second {
		output, setupErr := runBounded(ctx, 2*time.Second, r.Binary, "--list-langs")
		if ctx.Err() != nil {
			r.setupMu.Unlock()
			if parent.Err() != nil {
				return result, parent.Err()
			}
			result.Quality.Reason = "crop_budget_exhausted"
			return result, nil
		}
		r.setupErr = nil
		if setupErr != nil || !strings.Contains(" "+strings.Join(strings.Fields(string(output)), " ")+" ", " ara ") {
			r.setupErr = ErrArabicOCRUnavailable
		}
		r.setupAt = time.Now()
	}
	setupErr := r.setupErr
	r.setupMu.Unlock()
	if setupErr != nil {
		result.Quality.Reason = "arabic_reader_unavailable"
		return result, nil
	}
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return result, ErrValidation
	}
	observations = observeNameCrop(decoded, observations)
	result.Quality = observations
	if observations.Contrast < 12 {
		result.Outcome = "manual_review"
		result.Quality.Reason = "low_source_contrast"
		return result, nil
	}
	directory, err := os.MkdirTemp("", "dawha-local-crop-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(directory)
	thirdID := "raw-psm6"
	third := decoded
	if observations.Coloured {
		thirdID = "carrier-mask-psm6"
		third = maskCropCarrier(decoded)
	}
	variants := []struct {
		id    string
		psm   int
		image image.Image
	}{{"raw-psm7", 7, decoded}, {"adaptive-psm7", 7, thresholdNameCrop(decoded)}, {thirdID, 6, third}}
	if observations.Clipped {
		variants = variants[:1]
		variants = append(variants, struct {
			id    string
			psm   int
			image image.Image
		}{"raw-psm6", 6, decoded})
	}
	values := []string{}
	successes := 0
	for index, variant := range variants {
		if ctx.Err() != nil {
			if parent.Err() != nil {
				return result, parent.Err()
			}
			result.Quality.Reason = "crop_budget_exhausted"
			break
		}
		started := time.Now()
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, variant.image); err != nil {
			return result, err
		}
		filename := filepath.Join(directory, strconv.Itoa(index)+".png")
		if err := os.WriteFile(filename, encoded.Bytes(), 0600); err != nil {
			return result, err
		}
		reading := LocalVariantReading{Variant: variant.id, Segmentation: variant.psm, Outcome: "reading_unavailable", Bytes: encoded.Len()}
		output, readErr := runBounded(ctx, budget, r.Binary, filename, "stdout", "-l", "ara", "--psm", strconv.Itoa(variant.psm), "tsv")
		reading.DurationMS = time.Since(started).Milliseconds()
		if parent.Err() != nil {
			return result, parent.Err()
		}
		if readErr != nil {
			reading.ErrorCode = "local_ocr_failed"
			if errors.Is(readErr, context.DeadlineExceeded) {
				reading.ErrorCode = "crop_budget_exhausted"
			}
		} else {
			parsed, parseErr := parseTesseractTSV(output)
			if parseErr != nil {
				reading.ErrorCode = "invalid_local_ocr_output"
			} else {
				successes++
				reading.RawText = boundCropText(parsed.Text)
				reading.EngineScore = parsed.Confidence
				reading.Outcome = "unreadable"
				if reading.RawText != "" {
					reading.Outcome = "readable"
					values = append(values, reading.RawText)
				}
			}
		}
		result.Variants = append(result.Variants, reading)
	}
	if len(values) > 0 {
		counts := map[string]int{}
		best := ""
		for _, value := range values {
			counts[value]++
			if best == "" || counts[value] > counts[best] {
				best = value
			}
		}
		result.Text = best
		result.Outcome = "readable"
		result.Agreement = float64(counts[best]) / float64(len(result.Variants))
		for _, value := range values {
			if value != best && !containsString(result.Alternatives, value) {
				result.Alternatives = append(result.Alternatives, value)
			}
		}
		if len(result.Alternatives) > 0 {
			result.Quality.Reason = "variant_disagreement"
		}
	} else if successes > 0 {
		result.Outcome = "unreadable"
	} else {
		if result.Quality.Reason == "" {
			result.Quality.Reason = "local_reader_failed"
		}
	}
	if observations.Clipped && successes > 0 {
		if result.Text != "" && !containsString(result.Alternatives, result.Text) {
			result.Alternatives = append([]string{result.Text}, result.Alternatives...)
		}
		result.Text = ""
		result.Agreement = 0
		result.Outcome = "manual_review"
		result.Quality.Reason = "source_or_crop_clipped"
	}
	return result, nil
}
func boundCropText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	return strings.TrimSpace(string(runes))
}
func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func observeNameCrop(in image.Image, initial CropObservations) CropObservations {
	var sum, squares float64
	coloured, total := 0, 0
	bounds := in.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 2 {
			r, g, b, _ := in.At(x, y).RGBA()
			v := float64(color.GrayModel.Convert(in.At(x, y)).(color.Gray).Y)
			sum += v
			squares += v * v
			total++
			if max(r, g, b)-min(r, g, b) > 10000 && max(r, g, b) > 25000 {
				coloured++
			}
		}
	}
	if total > 0 {
		initial.Contrast = math.Sqrt(math.Max(0, squares/float64(total)-math.Pow(sum/float64(total), 2)))
		initial.Coloured = float64(coloured)/float64(total) > .15
	}
	return initial
}
func thresholdNameCrop(in image.Image) image.Image {
	b := in.Bounds()
	w, h := b.Dx(), b.Dy()
	gray := image.NewGray(image.Rect(0, 0, w, h))
	sums := make([]int, (w+1)*(h+1))
	for y := 0; y < h; y++ {
		row := 0
		for x := 0; x < w; x++ {
			v := color.GrayModel.Convert(in.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y
			gray.SetGray(x, y, color.Gray{Y: v})
			row += int(v)
			sums[(y+1)*(w+1)+x+1] = sums[y*(w+1)+x+1] + row
		}
	}
	out := image.NewGray(gray.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			l, r, t, bt := max(0, x-12), min(w, x+13), max(0, y-12), min(h, y+13)
			mean := (sums[bt*(w+1)+r] - sums[t*(w+1)+r] - sums[bt*(w+1)+l] + sums[t*(w+1)+l]) / ((r - l) * (bt - t))
			v := uint8(255)
			if int(gray.GrayAt(x, y).Y) < mean-10 {
				v = 0
			}
			out.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return out
}
func maskCropCarrier(in image.Image) image.Image {
	b := in.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			value := in.At(b.Min.X+x, b.Min.Y+y)
			r, g, bl, _ := value.RGBA()
			if max(r, g, bl)-min(r, g, bl) > 10000 && min(r, g, bl) > 16000 {
				value = color.White
			}
			out.Set(x, y, value)
		}
	}
	return out
}

func validLocalNameReading(reading LocalCropReading) bool {
	if len([]rune(reading.Text)) > 200 || len(reading.Alternatives) > 5 || len(reading.Variants) > 3 || len(reading.Quality.Reason) > 80 || math.IsNaN(reading.Agreement) || math.IsInf(reading.Agreement, 0) || reading.Agreement < 0 || reading.Agreement > 1 {
		return false
	}
	if math.IsNaN(reading.Quality.Contrast) || math.IsInf(reading.Quality.Contrast, 0) || reading.Quality.Contrast < 0 || reading.Quality.Contrast > 255 {
		return false
	}
	for _, value := range reading.Alternatives {
		if len([]rune(value)) > 200 {
			return false
		}
	}
	for _, variant := range reading.Variants {
		if len([]rune(variant.RawText)) > 200 || variant.Bytes < 0 || variant.Bytes > 10_000_000 || variant.DurationMS < 0 || len(variant.Variant) > 64 || len(variant.ErrorCode) > 80 {
			return false
		}
		if variant.EngineScore != nil && (math.IsNaN(*variant.EngineScore) || math.IsInf(*variant.EngineScore, 0) || *variant.EngineScore < 0 || *variant.EngineScore > 1) {
			return false
		}
	}
	return true
}
