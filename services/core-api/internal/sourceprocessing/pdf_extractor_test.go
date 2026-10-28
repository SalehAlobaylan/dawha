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
	"strings"
	"testing"
	"time"
)

func TestChartPDFPageRendersDirectSourceRegionsWithoutScaleTo(t *testing.T) {
	temporaryDir := t.TempDir()
	pdfInfo := writeTool(t, temporaryDir, "pdfinfo", "printf 'Pages: 1\\n'")
	pdfToText := writeTool(t, temporaryDir, "pdftotext", `printf '%s' '<doc><page width="0.3" height="0.3"></page></doc>'`)
	pngPath := filepath.Join(temporaryDir, "fixture.png")
	var encoded bytes.Buffer
	fixture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	fixture.Set(0, 0, color.White)
	if err := png.Encode(&encoded, fixture); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pngPath, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	pdftoppm := writeTool(t, temporaryDir, "pdftoppm", `printf '%s\n' "$*" > "$0.args"
cp '`+pngPath+`' "${18}.png"`)
	extractor := &PDFExtractor{PDFInfo: pdfInfo, PDFToText: pdfToText, PDFToPPM: pdftoppm}
	page, err := extractor.OpenChartPageForAnalysis(context.Background(), []byte("%PDF-1.7\\nfixture"), 1)
	if err != nil {
		t.Fatalf("open chart page: %v", err)
	}
	defer page.Close()
	if page.Width != 1 || page.Height != 1 || page.DPI != 240 || page.WidthPoints != .3 || page.HeightPoints != .3 {
		t.Fatalf("page transform = %+v, want one pixel at fixed 240 DPI from 0.3pt source", page)
	}
	if _, err := page.RenderRegion(context.Background(), ChartTileBounds{Width: 1, Height: 1}); err != nil {
		t.Fatalf("render source region: %v", err)
	}
	arguments, err := os.ReadFile(pdftoppm + ".args")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(arguments), "-scale-to") || !strings.Contains(string(arguments), "-x 0 -y 0 -W 1 -H 1") {
		t.Fatalf("Poppler did not render an exact source-space tile: %s", arguments)
	}
}

func TestChartPDFPageRemovesTemporaryRendersBeforePageClose(t *testing.T) {
	for _, outcome := range []string{"success", "invalid PNG", "command failure", "cancellation"} {
		t.Run(outcome, func(t *testing.T) {
			dir := t.TempDir()
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
				t.Fatal(err)
			}
			fixture := filepath.Join(dir, "fixture.png")
			if err := os.WriteFile(fixture, encoded.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			script := `cp '` + fixture + `' "${18}.png"`
			switch outcome {
			case "invalid PNG":
				script = `printf broken > "${18}.png"`
			case "command failure":
				script += "\nexit 1"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if outcome == "cancellation" {
				script = `cp '` + fixture + `' "${18}.png"` + "\nwhile :; do :; done"
			}
			tool := writeTool(t, dir, "pdftoppm", script)
			page := &ChartPDFPage{TempDir: dir, PDFPath: "fixture.pdf", PDFToPPM: tool, Width: 2, Height: 1, PageNumber: 1, DPI: 240}
			defer page.Close()
			if outcome == "cancellation" {
				go func() {
					for ctx.Err() == nil {
						files, _ := filepath.Glob(filepath.Join(dir, "region-*.png"))
						if len(files) != 0 {
							cancel()
							return
						}
						time.Sleep(time.Millisecond)
					}
				}()
			}
			for x := 0; x < 2; x++ {
				data, err := page.RenderRegion(ctx, ChartTileBounds{X: x, Width: 1, Height: 1})
				if outcome == "success" && (err != nil || !bytes.Equal(data, encoded.Bytes())) {
					t.Fatalf("render lost bytes: %v", err)
				}
				if outcome != "success" && err == nil {
					t.Fatal("invalid render succeeded")
				}
				files, _ := filepath.Glob(filepath.Join(dir, "region-*.png"))
				if len(files) != 0 {
					t.Fatalf("temporary crop retained before page close: %v", files)
				}
			}
		})
	}
}

func TestParsePDFBBoxRetainsArabicTextAndNormalizedBoxes(t *testing.T) {
	reading, width, height, err := parsePDFBBox([]byte(`<doc><page width="100" height="200"><flow><block><line><word xMin="10" yMin="20" xMax="40" yMax="30">عبد</word><word xMin="45" yMin="20" xMax="70" yMax="30">الله</word></line></block></flow></page></doc>`))
	if err != nil {
		t.Fatalf("parse bbox: %v", err)
	}
	if reading.Text != "عبد الله" || width != 100 || height != 200 {
		t.Fatalf("parsed reading = %q at %gx%g, want Arabic text at 100x200", reading.Text, width, height)
	}
	if len(reading.Regions) != 2 {
		t.Fatalf("regions = %d, want two words", len(reading.Regions))
	}
	first := reading.Regions[0]
	if first.Text != "عبد" || first.X != 0.1 || first.Y != 0.1 || first.Width != 0.3 || first.Height != 0.05 {
		t.Fatalf("first normalized word box = %+v", first)
	}
}

func TestParsePDFBBoxAcceptsPopplerNamespacedXHTMLAndEmptyScannedPage(t *testing.T) {
	reading, width, height, err := parsePDFBBox([]byte(`<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><body><doc><page width="595.000000" height="842.000000"><flow><block><line><word xMin="10" yMin="20" xMax="40" yMax="30">عبد</word><word xMin="45" yMin="20" xMax="70" yMax="30">الله</word></line></block></flow></page></doc></body></html>`))
	if err != nil {
		t.Fatalf("parse Poppler XHTML: %v", err)
	}
	if reading.Text != "عبد الله" || width != 595 || height != 842 || len(reading.Regions) != 2 {
		t.Fatalf("parsed page = %+v at %gx%g, want Arabic words and two regions", reading, width, height)
	}

	emptyReading, emptyWidth, emptyHeight, err := parsePDFBBox([]byte(`<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><body><doc><page width="595.000000" height="842.000000"></page></doc></body></html>`))
	if err != nil {
		t.Fatalf("parse empty scanned page: %v", err)
	}
	if emptyReading.Text != "" || emptyWidth != 595 || emptyHeight != 842 {
		t.Fatalf("empty page = %+v at %gx%g, want dimensions without native text", emptyReading, emptyWidth, emptyHeight)
	}
}

func TestParseTesseractTSVRetainsWordBoxesAndRawConfidence(t *testing.T) {
	data := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"1\t1\t0\t0\t0\t0\t0\t0\t4000\t3000\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t100\t120\t80\t35\t95\tعبد\n" +
		"5\t1\t1\t1\t1\t2\t190\t120\t60\t35\t85\tالله\n"
	reading, err := parseTesseractTSV([]byte(data))
	if err != nil {
		t.Fatalf("parse TSV: %v", err)
	}
	if reading.Text != "عبد الله" || reading.Confidence == nil || math.Abs(*reading.Confidence-0.9) > 1e-9 {
		t.Fatalf("parsed OCR text/confidence = %q/%v, want two words and raw average 0.9", reading.Text, reading.Confidence)
	}
	if len(reading.Regions) != 2 || math.Abs(reading.Regions[0].X-0.025) > 1e-9 || math.Abs(reading.Regions[0].Y-0.04) > 1e-9 {
		t.Fatalf("OCR word boxes = %+v, want normalized positions", reading.Regions)
	}
}

func TestParseTesseractTSVRetainsUnescapedQuotesInRecognizedText(t *testing.T) {
	data := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"1\t1\t0\t0\t0\t0\t0\t0\t4000\t3000\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t100\t120\t80\t35\t95\t\"\n" +
		"5\t1\t1\t1\t1\t2\t190\t120\t60\t35\t85\tالله\n"
	reading, err := parseTesseractTSV([]byte(data))
	if err != nil {
		t.Fatalf("parse TSV with recognized quotes: %v", err)
	}
	if reading.Text != `" الله` || len(reading.Regions) != 2 {
		t.Fatalf("parsed OCR text/regions = %q/%d, want quote preserved and both rows parsed", reading.Text, len(reading.Regions))
	}
}

func TestPDFExtractorAccountsForPagesAfterAnOCRFailure(t *testing.T) {
	temporaryDir := t.TempDir()
	pdfInfo := writeTool(t, temporaryDir, "pdfinfo", "printf 'Pages: 3\\n'")
	pdfToText := writeTool(t, temporaryDir, "pdftotext", `
if [ "$2" = "1" ]; then
  printf '%s' '<doc><page width="100" height="200"><flow><block><line><word xMin="10" yMin="20" xMax="40" yMax="30">النسب</word></line></block></flow></page></doc>'
else
  printf '%s' '<doc><page width="100" height="200"></page></doc>'
fi
`)
	extractor := &PDFExtractor{
		PDFInfo:   pdfInfo,
		PDFToText: pdfToText,
		PDFToPPM:  filepath.Join(temporaryDir, "missing-pdftoppm"),
		Tesseract: filepath.Join(temporaryDir, "missing-tesseract"),
	}
	pages, err := extractor.Extract(context.Background(), ExtractInput{
		Reader:      strings.NewReader("%PDF-1.7\\nfixture"),
		ContentType: "application/pdf",
	})
	if err == nil || !strings.Contains(err.Error(), "Arabic OCR") {
		t.Fatalf("extract error = %v, want unavailable Arabic OCR", err)
	}
	if len(pages) != 3 {
		t.Fatalf("returned %d pages, want all three physical pages", len(pages))
	}
	if pages[0].Text != "النسب" || pages[0].Readings[0].Method != "pdf_text" {
		t.Fatalf("completed native reading = %+v, want preserved page 1", pages[0])
	}
	for index := 1; index < len(pages); index++ {
		if pages[index].Number != index+1 || len(pages[index].Readings) != 1 || pages[index].Readings[0].Method != "not_processed" || !pages[index].Readings[0].Selected {
			t.Fatalf("unprocessed page %d = %+v", index+1, pages[index])
		}
	}
	if err := validateExtractedPages(pages); err != nil {
		t.Fatalf("partial page evidence failed validation: %v", err)
	}
}

func TestPDFExtractorAccountsForScannedPagesWhenOCRToolsAreNotConfigured(t *testing.T) {
	temporaryDir := t.TempDir()
	pdfInfo := writeTool(t, temporaryDir, "pdfinfo", "printf 'Pages: 3\\n'")
	pdfToText := writeTool(t, temporaryDir, "pdftotext", `
if [ "$2" = "1" ]; then
  printf '%s' '<doc><page width="100" height="200"><flow><block><line><word xMin="10" yMin="20" xMax="40" yMax="30">النسب</word></line></block></flow></page></doc>'
else
  printf '%s' '<doc><page width="100" height="200"></page></doc>'
fi
`)
	extractor := &PDFExtractor{PDFInfo: pdfInfo, PDFToText: pdfToText}
	pages, err := extractor.Extract(context.Background(), ExtractInput{
		Reader:      strings.NewReader("%PDF-1.7\\nfixture"),
		ContentType: "application/pdf",
	})
	if !errors.Is(err, ErrArabicOCRUnavailable) {
		t.Fatalf("extract error = %v, want unavailable Arabic OCR", err)
	}
	if len(pages) != 3 {
		t.Fatalf("returned %d pages, want all three physical pages", len(pages))
	}
	if pages[0].Text != "النسب" || pages[0].Readings[0].Method != "pdf_text" {
		t.Fatalf("completed native reading = %+v, want preserved page 1", pages[0])
	}
	for index := 1; index < len(pages); index++ {
		if pages[index].Number != index+1 || pages[index].Readings[0].Method != "not_processed" {
			t.Fatalf("unprocessed page %d = %+v", index+1, pages[index])
		}
	}
}

func writeTool(t *testing.T, directory, name, body string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	script := "#!/bin/sh\nset -eu\n" + strings.TrimSpace(body) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	return path
}
