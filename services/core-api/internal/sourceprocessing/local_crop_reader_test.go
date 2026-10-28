package sourceprocessing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func localReaderFixturePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 120, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 120; x++ {
			value := color.White
			if x > 25 && x < 90 && y > 10 && y < 35 {
				value = color.Black
			}
			img.Set(x, y, value)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func fakeCropTesseract(t *testing.T, body string) *TesseractCropReader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture-tesseract")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return &TesseractCropReader{Binary: path, Budget: 3 * time.Second}
}

const cropTSVHeader = "level\\tpage_num\\tblock_num\\tpar_num\\tline_num\\tword_num\\tleft\\ttop\\twidth\\theight\\tconf\\ttext\\n1\\t1\\t0\\t0\\t0\\t0\\t0\\t0\\t120\\t50\\t-1\\t\\n"

func TestLocalCropVariantsKeepProvenanceDisagreementAndRawScores(t *testing.T) {
	reader := fakeCropTesseract(t, "if [ \"$1\" = \"--list-langs\" ]; then echo ara; exit; fi\ntext='محمد'; case \"$1\" in *1.png) text='أحمد';; esac\nprintf '"+cropTSVHeader+"5\\t1\\t1\\t1\\t1\\t1\\t5\\t5\\t60\\t20\\t88.5\\t%s\\n' \"$text\"\n")
	result, err := reader.ReadCrop(context.Background(), localReaderFixturePNG(t), CropObservations{NameHeight: 24})
	if err != nil || result.Outcome != "readable" || result.Text != "محمد" || len(result.Variants) != 3 || len(result.Alternatives) != 1 || result.Alternatives[0] != "أحمد" || result.Quality.Reason != "variant_disagreement" || result.Quality.Clipped {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if result.Agreement != 2.0/3.0 || result.Variants[0].EngineScore == nil || *result.Variants[0].EngineScore != .885 || result.ReaderVersion != LocalCropReaderVersion {
		t.Fatal("engine score was mixed with agreement or version lost")
	}
	for _, variant := range result.Variants {
		if variant.Bytes < 1 || variant.DurationMS < 0 || variant.Variant == "" {
			t.Fatal("variant provenance missing")
		}
	}
}
func TestLocalCropRoutesTinyInkAndMissingArabicWithoutInference(t *testing.T) {
	reader := fakeCropTesseract(t, "if [ \"$1\" = \"--list-langs\" ]; then echo eng; exit; fi\nexit 99\n")
	data := localReaderFixturePNG(t)
	result, err := reader.ReadCrop(context.Background(), data, CropObservations{NameHeight: 5})
	if err != nil || result.Outcome != "manual_review" || len(result.Variants) != 0 {
		t.Fatalf("tiny result=%+v %v", result, err)
	}
	result, err = reader.ReadCrop(context.Background(), data, CropObservations{NameHeight: 24})
	if err != nil || result.Outcome != "reading_unavailable" || result.Quality.Reason != "arabic_reader_unavailable" {
		t.Fatalf("unavailable result=%+v %v", result, err)
	}
}
func TestLocalCropEmptyOutputIsUnreadableAndBudgetIsBounded(t *testing.T) {
	reader := fakeCropTesseract(t, "if [ \"$1\" = \"--list-langs\" ]; then echo ara; exit; fi\nprintf '"+cropTSVHeader+"'\n")
	result, err := reader.ReadCrop(context.Background(), localReaderFixturePNG(t), CropObservations{NameHeight: 24})
	if err != nil || result.Outcome != "unreadable" || len(result.Variants) != 3 {
		t.Fatalf("empty result=%+v %v", result, err)
	}
	blocked := fakeCropTesseract(t, "if [ \"$1\" = \"--list-langs\" ]; then echo ara; exit; fi\nexec /bin/sleep 5\n")
	blocked.Budget = 60 * time.Millisecond
	started := time.Now()
	result, err = blocked.ReadCrop(context.Background(), localReaderFixturePNG(t), CropObservations{NameHeight: 24})
	if err != nil || time.Since(started) > time.Second || result.Outcome != "reading_unavailable" || result.Quality.Reason != "crop_budget_exhausted" {
		t.Fatalf("budget result=%+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = blocked.ReadCrop(ctx, localReaderFixturePNG(t), CropObservations{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
}
func TestNameArtifactIdentityPinsCoordinatesGeometryAndRenderer(t *testing.T) {
	source := suggestionSource{checksum: strings.Repeat("a", 64), region: Region{X: .1, Y: .1, Width: .2, Height: .1}}
	frame := NamePageFrame{Page: 1, PageCount: 1, Space: "pdf_analysis", DPI: 240, Width: 1000, Height: 800, Renderer: NameRendererVersion}
	if !frame.valid() {
		t.Fatal("valid PDF frame rejected")
	}
	original := artifactFingerprint(source, frame)
	changed := frame
	changed.DPI = 300
	if changed.valid() || artifactFingerprint(source, changed) == original {
		t.Fatal("mixed PDF DPI was accepted/reused")
	}
	changed = frame
	changed.Renderer = "another-renderer"
	if artifactFingerprint(source, changed) == original {
		t.Fatal("renderer did not invalidate")
	}
	changedSource := source
	changedSource.checksum = strings.Repeat("b", 64)
	if artifactFingerprint(changedSource, frame) == original {
		t.Fatal("checksum did not invalidate")
	}
	changedSource = source
	changedSource.region.X = .2
	if artifactFingerprint(changedSource, frame) == original {
		t.Fatal("geometry did not invalidate")
	}
}

func TestLocalReaderIdentityChangesWithEngineDataOrBudget(t *testing.T) {
	config := NameReadingConfig{ReaderAlgorithm: LocalCropReaderVersion, Renderer: NameRendererVersion, Geometry: NameGeometryVersion, Preprocessing: NamePreprocessingVersion, Engine: "5.5.2", ArabicDataVersion: "fixture-data-v1", Mode: "local", Privacy: "local_only", CropBudgetMS: 12000}
	original := fullNameReaderVersion(config)
	for _, change := range []func(*NameReadingConfig){func(c *NameReadingConfig) { c.Engine = "5.6.0" }, func(c *NameReadingConfig) { c.ArabicDataVersion = "fixture-data-v2" }, func(c *NameReadingConfig) { c.CropBudgetMS = 15000 }, func(c *NameReadingConfig) { c.Preprocessing = "variants-v2" }} {
		changed := config
		change(&changed)
		if fullNameReaderVersion(changed) == original {
			t.Fatal("mixed processing versions share a reader identity")
		}
	}
	config.Reader = original
	if fullNameReaderVersion(config) != original {
		t.Fatal("reader identity is not stable")
	}
}

func TestLocalCropClippingRetainsRawVariantsForManualReview(t *testing.T) {
	reader := fakeCropTesseract(t, "if [ \"$1\" = \"--list-langs\" ]; then echo ara; exit; fi\nprintf '"+cropTSVHeader+"5\\t1\\t1\\t1\\t1\\t1\\t5\\t5\\t60\\t20\\t90\\tمحمد\\n'\n")
	result, err := reader.ReadCrop(context.Background(), localReaderFixturePNG(t), CropObservations{NameHeight: 24, Clipped: true})
	if err != nil || result.Outcome != "manual_review" || result.Text != "" || result.Quality.Reason != "source_or_crop_clipped" || len(result.Variants) != 2 || len(result.Alternatives) != 1 {
		t.Fatalf("clipping result=%+v error=%v", result, err)
	}
}

type deniedLocalReaderTransport struct{ calls int }

func (d *deniedLocalReaderTransport) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls++
	return nil, errors.New("network transport denied")
}
func TestLocalCropReaderNeverFallsBackToNetwork(t *testing.T) {
	for _, name := range []string{"LIQUID_API_KEY", "DEEPSEEK_API_KEY", "JEV_API_KEY"} {
		t.Setenv(name, "")
	}
	denied := &deniedLocalReaderTransport{}
	previous := http.DefaultTransport
	http.DefaultTransport = denied
	t.Cleanup(func() { http.DefaultTransport = previous })
	reader := fakeCropTesseract(t, "if [ \"$1\" = \"--list-langs\" ]; then echo ara; exit; fi\nprintf '"+cropTSVHeader+"5\\t1\\t1\\t1\\t1\\t1\\t5\\t5\\t60\\t20\\t90\\tمحمد\\n'\n")
	result, err := reader.ReadCrop(context.Background(), localReaderFixturePNG(t), CropObservations{NameHeight: 24})
	if err != nil || result.Outcome != "readable" {
		t.Fatalf("local result=%+v error=%v", result, err)
	}
	unavailable := fakeCropTesseract(t, "exit 1\n")
	result, err = unavailable.ReadCrop(context.Background(), localReaderFixturePNG(t), CropObservations{NameHeight: 24})
	if err != nil || result.Outcome != "reading_unavailable" {
		t.Fatalf("unavailable result=%+v error=%v", result, err)
	}
	if denied.calls != 0 {
		t.Fatal("local crop processing attempted a network request")
	}
}
