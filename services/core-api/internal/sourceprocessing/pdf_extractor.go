package sourceprocessing

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxRenderedPixels    = 16_000_000
	maxChartPageAxis     = 250_000
	maxChartRegionPixels = 16_000_000
	chartAnalysisDPI     = 240
	maxPageTextBytes     = 1 << 20
	maxDocumentTextBytes = 128 << 20
	maxCommandOutput     = 24 << 20
	pageCommandLimit     = 45 * time.Second
	pdfProcessingLimit   = 30 * time.Minute
)

// PDFExtractor uses Poppler for physical-page inspection and native PDF text,
// then Tesseract's Arabic model as an independent reading. PDF bytes stay in
// the worker; Process later passes extracted page text to its configured AI
// service for candidate extraction. The original PDF remains the review authority.
type PDFExtractor struct {
	PDFInfo   string
	PDFToText string
	PDFToPPM  string
	Tesseract string
	DPI       int
	// CarrierLocaliser holds the thresholds the name carrier localiser uses. It
	// is configuration so a run can record which thresholds produced a result.
	CarrierLocaliser CarrierLocaliserConfig
}

// ChartPDFPage keeps the private source PDF open in a mode-0700 temporary
// directory and renders bounded regions directly from the PDF at fixed DPI.
// This avoids shrinking a large chart to fit a UI-sized image before tiling.
type ChartPDFPage struct {
	PDFToPPM     string
	PDFPath      string
	TempDir      string
	PageNumber   int
	PageCount    int
	DPI          int
	Width        int
	Height       int
	WidthPoints  float64
	HeightPoints float64
	// sourceImage is set when the page came from a raster document rather than
	// a PDF. Its tiles are cropped locally instead of rasterized by pdftoppm.
	sourceImage image.Image
}

func (p *ChartPDFPage) Close() error {
	if p == nil || p.TempDir == "" {
		return nil
	}
	err := os.RemoveAll(p.TempDir)
	p.TempDir = ""
	return err
}

// RenderRegion uses the original PDF as the render source and keeps the
// crop dimensions in the page's fixed-DPI pixel coordinate space.
func (p *ChartPDFPage) RenderRegion(ctx context.Context, region ChartTileBounds) ([]byte, error) {
	if p == nil || region.X < 0 || region.Y < 0 || region.Width <= 0 || region.Height <= 0 || region.X+region.Width > p.Width || region.Y+region.Height > p.Height || int64(region.Width)*int64(region.Height) > maxChartRegionPixels {
		return nil, ErrValidation
	}
	if p.sourceImage != nil {
		return cropImageRegion(p.sourceImage, region)
	}
	if p.PDFPath == "" || p.PDFToPPM == "" {
		return nil, ErrValidation
	}
	if _, err := exec.LookPath(p.PDFToPPM); err != nil {
		return nil, ErrPDFToolsUnavailable
	}
	prefix := filepath.Join(p.TempDir, fmt.Sprintf("region-%d-%d-%d-%d-%d", p.PageNumber, region.X, region.Y, region.Width, region.Height))
	if _, err := runBounded(ctx, pageCommandLimit, p.PDFToPPM, "-f", strconv.Itoa(p.PageNumber), "-l", strconv.Itoa(p.PageNumber), "-singlefile", "-r", strconv.Itoa(p.DPI), "-x", strconv.Itoa(region.X), "-y", strconv.Itoa(region.Y), "-W", strconv.Itoa(region.Width), "-H", strconv.Itoa(region.Height), "-png", p.PDFPath, prefix); err != nil {
		return nil, pdfCommandError(err, p.PageNumber)
	}
	data, err := os.ReadFile(prefix + ".png")
	if err != nil || len(data) == 0 || len(data) > maxChartRegionPixels*4 {
		return nil, ErrPDFProcessingFailed
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != region.Width || config.Height != region.Height {
		return nil, ErrPDFProcessingFailed
	}
	return data, nil
}

// OpenChartPageForAnalysis prepares a fixed-DPI page coordinate system but
// does not rasterize the whole page. Each analysis tile is rendered from the
// original PDF, preserving small content on oversized charts.
func (e *PDFExtractor) OpenChartPageForAnalysis(ctx context.Context, content []byte, pageNumber int) (*ChartPDFPage, error) {
	if e == nil || pageNumber < 1 || pageNumber > MaxPages || len(content) < 5 || int64(len(content)) > MaxUploadBytes {
		return nil, ErrValidation
	}
	if inspection, ok := inspectImageSource(content); ok {
		// A raster chart has exactly one page, and its pixel size is the page
		// size. The tile coordinate system is the image itself, so a crop is a
		// local operation with no external tool.
		//
		// The bound is the SOURCE bound, not the render bound: the upload
		// contract accepts a 40-megapixel photograph, and a page that large is
		// read by cropping it into tiles, not by refusing it after the upload
		// succeeded. A 16-megapixel cap here would silently turn every
		// high-resolution image into a page with no names.
		if pageNumber != 1 || int64(inspection.Width)*int64(inspection.Height) > maxSourceImagePixels {
			return nil, ErrValidation
		}
		decoded, _, err := decodeImageSource(content)
		if err != nil {
			return nil, err
		}
		dpi := chartAnalysisDPI
		return &ChartPDFPage{
			sourceImage:  decoded,
			PageNumber:   1,
			PageCount:    1,
			DPI:          dpi,
			Width:        inspection.Width,
			Height:       inspection.Height,
			WidthPoints:  float64(inspection.Width) * 72 / float64(dpi),
			HeightPoints: float64(inspection.Height) * 72 / float64(dpi),
		}, nil
	}
	if !hasPDFHeader(content) {
		return nil, ErrValidation
	}
	if e.PDFInfo == "" || e.PDFToText == "" || e.PDFToPPM == "" {
		return nil, ErrPDFToolsUnavailable
	}
	for _, binary := range []string{e.PDFInfo, e.PDFToText, e.PDFToPPM} {
		if _, err := exec.LookPath(binary); err != nil {
			return nil, ErrPDFToolsUnavailable
		}
	}
	temporaryDir, err := os.MkdirTemp("", "dawha-pdf-chart-tiles-*")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(temporaryDir, 0o700); err != nil {
		_ = os.RemoveAll(temporaryDir)
		return nil, err
	}
	pdfPath := filepath.Join(temporaryDir, "source.pdf")
	if err := os.WriteFile(pdfPath, content, 0o600); err != nil {
		_ = os.RemoveAll(temporaryDir)
		return nil, err
	}
	info, err := runBounded(ctx, pageCommandLimit, e.PDFInfo, pdfPath)
	if err != nil {
		_ = os.RemoveAll(temporaryDir)
		return nil, pdfCommandError(err, 0)
	}
	pageCount, err := parsePDFPageCount(info)
	if err != nil || pageCount < 1 || pageCount > MaxPages || pageNumber > pageCount {
		_ = os.RemoveAll(temporaryDir)
		return nil, ErrValidation
	}
	bbox, err := runBounded(ctx, pageCommandLimit, e.PDFToText, "-f", strconv.Itoa(pageNumber), "-l", strconv.Itoa(pageNumber), "-bbox-layout", "-enc", "UTF-8", pdfPath, "-")
	if err != nil {
		_ = os.RemoveAll(temporaryDir)
		return nil, pdfCommandError(err, pageNumber)
	}
	_, widthPoints, heightPoints, err := parsePDFBBox(bbox)
	if err != nil || widthPoints <= 0 || heightPoints <= 0 {
		_ = os.RemoveAll(temporaryDir)
		return nil, ErrValidation
	}
	width := int(math.Ceil(widthPoints * chartAnalysisDPI / 72))
	height := int(math.Ceil(heightPoints * chartAnalysisDPI / 72))
	if width < 1 || height < 1 || width > maxChartPageAxis || height > maxChartPageAxis {
		_ = os.RemoveAll(temporaryDir)
		return nil, ErrValidation
	}
	return &ChartPDFPage{PDFToPPM: e.PDFToPPM, PDFPath: pdfPath, TempDir: temporaryDir, PageNumber: pageNumber, PageCount: pageCount, DPI: chartAnalysisDPI, Width: width, Height: height, WidthPoints: widthPoints, HeightPoints: heightPoints}, nil
}

func NewPDFExtractor() *PDFExtractor {
	return &PDFExtractor{
		PDFInfo:          executable("PDFINFO_BINARY", "pdfinfo"),
		PDFToText:        executable("PDFTOTEXT_BINARY", "pdftotext"),
		PDFToPPM:         executable("PDFTOPPM_BINARY", "pdftoppm"),
		Tesseract:        executable("TESSERACT_BINARY", "tesseract"),
		DPI:              300,
		CarrierLocaliser: DefaultCarrierLocaliserConfig(),
	}
}

func executable(env, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(env)); value != "" {
		return value
	}
	return fallback
}

func (e *PDFExtractor) Extract(ctx context.Context, input ExtractInput) ([]Page, error) {
	if e == nil || input.Reader == nil {
		return nil, errExtractorUnavailable
	}
	if normalizeContentType(input.ContentType) != "application/pdf" {
		if isImageContentType(input.ContentType) {
			data, err := io.ReadAll(io.LimitReader(input.Reader, MaxUploadBytes+1))
			if err != nil {
				return nil, err
			}
			if len(data) < 8 || int64(len(data)) > MaxUploadBytes {
				return nil, ErrValidation
			}
			ctx, cancel := context.WithTimeout(ctx, pdfProcessingLimit)
			defer cancel()
			if err := ctx.Err(); err != nil {
				return nil, pdfContextError(err)
			}
			return e.extractImage(ctx, data)
		}
		return NewTextExtractor().Extract(ctx, input)
	}
	ctx, cancel := context.WithTimeout(ctx, pdfProcessingLimit)
	defer cancel()
	data, err := io.ReadAll(io.LimitReader(input.Reader, MaxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) < 5 || int64(len(data)) > MaxUploadBytes || !hasPDFHeader(data) {
		return nil, ErrValidation
	}
	if err := ctx.Err(); err != nil {
		return nil, pdfContextError(err)
	}
	if e.PDFInfo == "" || e.PDFToText == "" {
		return nil, ErrPDFToolsUnavailable
	}
	if _, err := exec.LookPath(e.PDFInfo); err != nil {
		return nil, ErrPDFToolsUnavailable
	}
	if _, err := exec.LookPath(e.PDFToText); err != nil {
		return nil, ErrPDFToolsUnavailable
	}
	temporaryDir, err := os.MkdirTemp("", "dawha-pdf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporaryDir)
	if err := os.Chmod(temporaryDir, 0o700); err != nil {
		return nil, err
	}
	pdfPath := filepath.Join(temporaryDir, "source.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return nil, err
	}
	infoOutput, err := runBounded(ctx, pageCommandLimit, e.PDFInfo, pdfPath)
	if err != nil {
		return nil, pdfCommandError(err, 0)
	}
	physicalPages, err := parsePDFPageCount(infoOutput)
	if err != nil || physicalPages < 1 || physicalPages > MaxPages {
		return nil, ErrValidation
	}

	pages := make([]Page, 0, physicalPages)
	totalRegions := 0
	totalTextBytes := 0
	for number := 1; number <= physicalPages; number++ {
		if err := ctx.Err(); err != nil {
			return appendUnprocessedPages(pages, number, physicalPages), pdfContextError(err)
		}
		nativeOutput, nativeErr := runBounded(ctx, pageCommandLimit, e.PDFToText,
			"-f", strconv.Itoa(number), "-l", strconv.Itoa(number), "-bbox-layout", "-enc", "UTF-8", pdfPath, "-",
		)
		if nativeErr != nil {
			return appendUnprocessedPages(pages, number, physicalPages), pdfCommandError(nativeErr, number)
		}
		nativeReading, width, height, err := parsePDFBBox(nativeOutput)
		if err != nil {
			return appendUnprocessedPages(pages, number, physicalPages), fmt.Errorf("%w: page %d layout is invalid", ErrPDFProcessingFailed, number)
		}
		if err := accountReading(nativeReading, &totalRegions, &totalTextBytes); err != nil {
			return appendUnprocessedPages(pages, number, physicalPages), err
		}
		page := Page{Number: number, Width: width, Height: height}
		if strings.TrimSpace(nativeReading.Text) != "" {
			nativeReading.Method = "pdf_text"
			nativeReading.Model = "poppler-pdftotext"
			nativeReading.Selected = true
			page.Text = nativeReading.Text
			page.Readings = append(page.Readings, nativeReading)
		}

		// OCR all pages, including digitally searchable PDFs. Keeping both readings
		// lets a reviewer catch a stale or incorrect hidden OCR/text layer.
		if e.PDFToPPM != "" && e.Tesseract != "" {
			ocrReading, lineReading, carrierReading, renderErr := e.ocrPage(ctx, pdfPath, temporaryDir, number, input.RenderDPI)
			if renderErr != nil {
				// A readable native page can still be reviewed if the optional OCR
				// toolchain is missing. A scan with no native text cannot.
				if len(page.Readings) == 0 {
					return appendUnprocessedPages(pages, number, physicalPages), renderErr
				}
				page.Readings = append(page.Readings, PageReading{
					Method: "tesseract_unavailable", Model: "tesseract-ara+eng",
				})
				if lineReading.Method != "" {
					page.Readings = append(page.Readings, lineReading)
				}
				if carrierReading.Method != "" {
					page.Readings = append(page.Readings, carrierReading)
				}
			} else {
				if err := accountReading(ocrReading, &totalRegions, &totalTextBytes); err != nil {
					if len(page.Readings) == 0 {
						return appendUnprocessedPages(pages, number, physicalPages), err
					}
					page.Readings = append(page.Readings, PageReading{Method: "tesseract_unavailable", Model: "tesseract-ara+eng"})
					pages = append(pages, page)
					return appendUnprocessedPages(pages, number+1, physicalPages), err
				}
				ocrReading.Method = "tesseract_ocr"
				ocrReading.Model = "tesseract-ara+eng"
				if len(page.Readings) == 0 {
					ocrReading.Selected = true
					page.Text = ocrReading.Text
				}
				page.Readings = append(page.Readings, ocrReading)
				if lineReading.Method != "" {
					priorRegions, priorTextBytes := totalRegions, totalTextBytes
					if err := accountReading(lineReading, &totalRegions, &totalTextBytes); err != nil {
						totalRegions, totalTextBytes = priorRegions, priorTextBytes
						lineReading = PageReading{Method: "local_line_detection_unavailable", Model: "orthogonal-run-v1"}
					}
					page.Readings = append(page.Readings, lineReading)
				}
				if carrierReading.Method != "" {
					priorRegions, priorTextBytes := totalRegions, totalTextBytes
					if err := accountReading(carrierReading, &totalRegions, &totalTextBytes); err != nil {
						totalRegions, totalTextBytes = priorRegions, priorTextBytes
						carrierReading = PageReading{Method: carrierReadingMethod + "_unavailable", Model: carrierReadingModel}
					}
					page.Readings = append(page.Readings, carrierReading)
				}
			}
		} else if len(page.Readings) == 0 {
			return appendUnprocessedPages(pages, number, physicalPages), ErrArabicOCRUnavailable
		} else {
			page.Readings = append(page.Readings, PageReading{
				Method: "tesseract_unavailable", Model: "tesseract-ara+eng",
			})
		}
		if len(page.Readings) == 0 {
			page.Readings = []PageReading{{Method: "unreadable", Model: "local-pdf-pipeline", Selected: true}}
		}
		totalTextBytes += len(page.Text)
		if totalTextBytes > maxDocumentTextBytes {
			return appendUnprocessedPages(pages, number, physicalPages), ErrValidation
		}
		page.EndOffset = utf8.RuneCountInString(page.Text)
		pages = append(pages, page)
	}
	return pages, nil
}

// RenderPage creates a bounded local PNG preview of one physical page. The
// caller must authorize the source before invoking it; the resulting image is
// a private derivative of the original and is never sent to a model provider.
// PageCount returns the number of pages in a PDF, or zero when it cannot be read.
func (e *PDFExtractor) PageCount(ctx context.Context, content []byte) int {
	if e == nil || len(content) < 5 {
		return 0
	}
	if _, ok := inspectImageSource(content); ok {
		return 1
	}
	if e.PDFInfo == "" {
		return 0
	}
	temporaryDir, err := os.MkdirTemp("", "dawha-pagecount-")
	if err != nil {
		return 0
	}
	defer os.RemoveAll(temporaryDir)
	path := filepath.Join(temporaryDir, "document.pdf")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return 0
	}
	if _, err := exec.LookPath(e.PDFInfo); err != nil {
		return 0
	}
	output, err := runBounded(ctx, pageCommandLimit, e.PDFInfo, path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "Pages:") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "Pages:"))
		if len(fields) == 0 {
			return 0
		}
		count, err := strconv.Atoi(fields[0])
		if err != nil || count < 1 || count > MaxPages {
			return 0
		}
		return count
	}
	return 0
}

func (e *PDFExtractor) RenderPage(ctx context.Context, content []byte, pageNumber int) ([]byte, error) {
	if e == nil || pageNumber < 1 || pageNumber > MaxPages || len(content) < 5 || int64(len(content)) > MaxUploadBytes {
		return nil, ErrValidation
	}
	if _, ok := inspectImageSource(content); ok {
		if pageNumber != 1 {
			return nil, ErrValidation
		}
		preview, _, err := renderImagePage(content, maxPreviewImageAxis)
		return preview, err
	}
	if !hasPDFHeader(content) {
		return nil, ErrValidation
	}
	if e.PDFInfo == "" || e.PDFToPPM == "" {
		return nil, ErrPDFToolsUnavailable
	}
	if _, err := exec.LookPath(e.PDFInfo); err != nil {
		return nil, ErrPDFToolsUnavailable
	}
	if _, err := exec.LookPath(e.PDFToPPM); err != nil {
		return nil, ErrPDFToolsUnavailable
	}
	temporaryDir, err := os.MkdirTemp("", "dawha-pdf-preview-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporaryDir)
	if err := os.Chmod(temporaryDir, 0o700); err != nil {
		return nil, err
	}
	pdfPath := filepath.Join(temporaryDir, "source.pdf")
	if err := os.WriteFile(pdfPath, content, 0o600); err != nil {
		return nil, err
	}
	info, err := runBounded(ctx, pageCommandLimit, e.PDFInfo, pdfPath)
	if err != nil {
		return nil, pdfCommandError(err, 0)
	}
	pageCount, err := parsePDFPageCount(info)
	if err != nil || pageCount < 1 || pageCount > MaxPages || pageNumber > pageCount {
		return nil, ErrValidation
	}
	prefix := filepath.Join(temporaryDir, "preview")
	if _, err := runBounded(ctx, pageCommandLimit, e.PDFToPPM, "-f", strconv.Itoa(pageNumber), "-l", strconv.Itoa(pageNumber), "-singlefile", "-scale-to", "2400", "-png", pdfPath, prefix); err != nil {
		return nil, pdfCommandError(err, pageNumber)
	}
	preview, err := os.ReadFile(prefix + ".png")
	if err != nil || len(preview) == 0 || len(preview) > maxRenderedPixels*4 {
		return nil, ErrPDFProcessingFailed
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(preview))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > maxRenderedPixels {
		return nil, ErrPDFProcessingFailed
	}
	return preview, nil
}

// RenderPageForChart is a compatibility helper for small charts. Large pages
// must use OpenChartPageForAnalysis and direct source-PDF tiles.
func (e *PDFExtractor) RenderPageForChart(ctx context.Context, content []byte, pageNumber int) ([]byte, int, int, int, error) {
	page, err := e.OpenChartPageForAnalysis(ctx, content, pageNumber)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer page.Close()
	if int64(page.Width)*int64(page.Height) > maxRenderedPixels {
		return nil, 0, 0, page.PageCount, ErrValidation
	}
	data, err := page.RenderRegion(ctx, ChartTileBounds{Width: page.Width, Height: page.Height})
	if err != nil {
		return nil, 0, 0, page.PageCount, err
	}
	return data, page.Width, page.Height, page.PageCount, nil
}

// appendUnprocessedPages keeps physical-page coverage explicit after a page or
// document failure. A reviewer can still open the original and save a manual
// transcription for these pages; no missing page is represented as an empty
// successful extraction.
func appendUnprocessedPages(pages []Page, firstNumber, lastNumber int) []Page {
	for number := firstNumber; number <= lastNumber; number++ {
		pages = append(pages, Page{
			Number: number,
			Readings: []PageReading{{
				Method: "not_processed", Model: "local-pdf-pipeline", Selected: true,
			}},
		})
	}
	return pages
}

func hasPDFHeader(content []byte) bool {
	if len(content) > 1024 {
		content = content[:1024]
	}
	return bytes.Contains(content, []byte("%PDF-"))
}

func (e *PDFExtractor) ocrPage(ctx context.Context, pdfPath, temporaryDir string, number, requestedDPI int) (PageReading, PageReading, PageReading, error) {
	if _, err := exec.LookPath(e.PDFToPPM); err != nil {
		return PageReading{}, PageReading{}, PageReading{}, ErrArabicOCRUnavailable
	}
	if _, err := exec.LookPath(e.Tesseract); err != nil {
		return PageReading{}, PageReading{}, PageReading{}, ErrArabicOCRUnavailable
	}
	pagePrefix := filepath.Join(temporaryDir, fmt.Sprintf("page-%03d", number))
	imagePath := pagePrefix + ".png"
	dpi := requestedDPI
	if dpi == 0 {
		dpi = e.DPI
	}
	if dpi < 100 || dpi > 400 {
		dpi = 300
	}
	_, err := runBounded(ctx, pageCommandLimit, e.PDFToPPM,
		"-f", strconv.Itoa(number), "-l", strconv.Itoa(number), "-singlefile", "-r", strconv.Itoa(dpi), "-scale-to", "4000", "-png", pdfPath, pagePrefix,
	)
	if err != nil {
		_ = os.Remove(imagePath)
		return PageReading{}, PageReading{}, PageReading{}, fmt.Errorf("%w: page %d could not be rendered", ErrArabicOCRUnavailable, number)
	}
	defer os.Remove(imagePath)
	imageFile, err := os.Open(imagePath)
	if err != nil {
		return PageReading{}, PageReading{}, PageReading{}, fmt.Errorf("%w: page %d image could not be read", ErrArabicOCRUnavailable, number)
	}
	imageBytes, err := io.ReadAll(io.LimitReader(imageFile, maxRenderedPixels*4+1))
	closeErr := imageFile.Close()
	if err != nil || closeErr != nil || len(imageBytes) == 0 || len(imageBytes) > maxRenderedPixels*4 {
		return PageReading{}, PageReading{}, PageReading{}, ErrValidation
	}
	return e.readRenderedImage(ctx, imageBytes, number)
}

// readRenderedImage runs the three independent local passes on one rendered
// page: the ink-run line detector, the name localiser, and Tesseract. It is
// shared by the PDF path (after pdftoppm) and the image path (after the source
// image is bounded), so both formats are read by exactly the same instruments.
func (e *PDFExtractor) readRenderedImage(ctx context.Context, imageBytes []byte, pageNumber int) (PageReading, PageReading, PageReading, error) {
	lineReading := PageReading{Method: "local_line_detection", Model: "orthogonal-run-v1"}
	var err error
	lineReading.Regions, err = detectOrthogonalLineSegments(imageBytes)
	if err != nil {
		lineReading = PageReading{Method: "local_line_detection_unavailable", Model: "orthogonal-run-v1"}
	}
	// The name carrier localiser runs on the same rendered page, for the same
	// reason: it is a deterministic pass with no external call, and it is what
	// finds WHERE the names are. The ink-run detector above finds the page's
	// geometry, which on a photographic chart is the landscape.
	carrierReading := PageReading{Method: carrierReadingMethod, Model: carrierReadingModel}
	carrierReading.Regions, err = DetectNameCarriers(imageBytes, e.CarrierLocaliser)
	if err != nil {
		carrierReading = PageReading{Method: carrierReadingMethod + "_unavailable", Model: carrierReadingModel}
	}
	commandContext, cancel := context.WithTimeout(ctx, pageCommandLimit)
	defer cancel()
	command := exec.CommandContext(commandContext, e.Tesseract, "stdin", "stdout", "-l", "ara+eng", "--psm", "11", "tsv")
	command.Env = documentToolEnvironment()
	command.Stdin = bytes.NewReader(imageBytes)
	output := &boundedBuffer{limit: maxCommandOutput}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
			return PageReading{}, lineReading, PageReading{}, fmt.Errorf("%w: page %d", ErrPDFProcessingTimedOut, pageNumber)
		}
		return PageReading{}, lineReading, PageReading{}, fmt.Errorf("%w: page %d; check Tesseract ara+eng data", ErrArabicOCRUnavailable, pageNumber)
	}
	reading, err := parseTesseractTSV(output.Bytes())
	if err != nil {
		return PageReading{}, lineReading, PageReading{}, fmt.Errorf("%w: page %d output was invalid", ErrArabicOCRUnavailable, pageNumber)
	}
	return reading, lineReading, carrierReading, nil
}

func pdfCommandError(err error, pageNumber int) error {
	if errors.Is(err, context.DeadlineExceeded) {
		if pageNumber > 0 {
			return fmt.Errorf("%w: page %d", ErrPDFProcessingTimedOut, pageNumber)
		}
		return ErrPDFProcessingTimedOut
	}
	if pageNumber > 0 {
		return fmt.Errorf("%w: page %d; upload a valid, unlocked PDF", ErrPDFProcessingFailed, pageNumber)
	}
	return fmt.Errorf("%w: upload a valid, unlocked PDF", ErrPDFProcessingFailed)
}

func pdfContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrPDFProcessingTimedOut
	}
	return err
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, ErrValidation
	}
	return b.Buffer.Write(p)
}

func runBounded(parent context.Context, limit time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, limit)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Env = documentToolEnvironment()
	output := &boundedBuffer{limit: maxCommandOutput}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return output.Bytes(), nil
}

func documentToolEnvironment() []string {
	values := []string{"OMP_THREAD_LIMIT=2", "TMPDIR=" + os.TempDir(), "HOME=" + os.TempDir()}
	for _, name := range []string{"PATH", "TESSDATA_PREFIX", "LANG", "LC_ALL", "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH"} {
		if value := os.Getenv(name); value != "" {
			values = append(values, name+"="+value)
		}
	}
	return values
}

func parsePDFPageCount(data []byte) (int, error) {
	for _, line := range strings.Split(string(data), "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) == "Pages" {
			return strconv.Atoi(strings.TrimSpace(value))
		}
	}
	return 0, ErrValidation
}

type bboxWord struct {
	XMin float64 `xml:"xMin,attr"`
	YMin float64 `xml:"yMin,attr"`
	XMax float64 `xml:"xMax,attr"`
	YMax float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

type bboxPage struct {
	Width  float64
	Height float64
	Lines  [][]bboxWord
}

func parsePDFBBox(data []byte) (PageReading, float64, float64, error) {
	// Poppler wraps bbox output in XHTML with a default namespace. Match the
	// layout by local element names so both namespaced and bare test fixtures work.
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	var page *bboxPage
	var pages []bboxPage
	var lineWords []bboxWord
	var currentWord *bboxWord
	var wordText strings.Builder
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return PageReading{}, 0, 0, ErrValidation
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "page":
				if page != nil {
					return PageReading{}, 0, 0, ErrValidation
				}
				page = &bboxPage{}
				for _, attr := range value.Attr {
					switch attr.Name.Local {
					case "width":
						page.Width, err = strconv.ParseFloat(attr.Value, 64)
					case "height":
						page.Height, err = strconv.ParseFloat(attr.Value, 64)
					}
					if err != nil {
						return PageReading{}, 0, 0, ErrValidation
					}
				}
			case "line":
				if page != nil {
					lineWords = nil
				}
			case "word":
				if page != nil {
					currentWord = &bboxWord{}
					wordText.Reset()
					for _, attr := range value.Attr {
						switch attr.Name.Local {
						case "xMin":
							currentWord.XMin, err = strconv.ParseFloat(attr.Value, 64)
						case "yMin":
							currentWord.YMin, err = strconv.ParseFloat(attr.Value, 64)
						case "xMax":
							currentWord.XMax, err = strconv.ParseFloat(attr.Value, 64)
						case "yMax":
							currentWord.YMax, err = strconv.ParseFloat(attr.Value, 64)
						}
						if err != nil {
							return PageReading{}, 0, 0, ErrValidation
						}
					}
				}
			}
		case xml.CharData:
			if currentWord != nil {
				wordText.Write([]byte(value))
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "word":
				if currentWord != nil {
					currentWord.Text = wordText.String()
					lineWords = append(lineWords, *currentWord)
					currentWord = nil
				}
			case "line":
				if page != nil && len(lineWords) > 0 {
					page.Lines = append(page.Lines, lineWords)
					lineWords = nil
				}
			case "page":
				if page == nil {
					return PageReading{}, 0, 0, ErrValidation
				}
				pages = append(pages, *page)
				page = nil
			}
		}
	}
	if page != nil || len(pages) == 0 {
		return PageReading{}, 0, 0, ErrValidation
	}
	page = &pages[0]
	if page.Width <= 0 || page.Height <= 0 || math.IsInf(page.Width, 0) || math.IsInf(page.Height, 0) {
		return PageReading{}, 0, 0, ErrValidation
	}
	reading := PageReading{}
	for _, words := range page.Lines {
		lineText := make([]string, 0, len(words))
		for _, word := range words {
			text := strings.TrimSpace(word.Text)
			if text == "" {
				continue
			}
			lineText = append(lineText, text)
			if validBox(word.XMin, word.YMin, word.XMax-word.XMin, word.YMax-word.YMin, page.Width, page.Height) {
				reading.Regions = append(reading.Regions, PageRegion{
					Kind: "word", Text: text, X: word.XMin / page.Width, Y: word.YMin / page.Height,
					Width: (word.XMax - word.XMin) / page.Width, Height: (word.YMax - word.YMin) / page.Height,
				})
			}
		}
		if len(lineText) > 0 {
			reading.Text += strings.Join(lineText, " ") + "\n"
		}
	}
	reading.Text = strings.TrimSpace(reading.Text)
	if len(reading.Text) > maxPageTextBytes || len(reading.Regions) > MaxPageRegions {
		return PageReading{}, 0, 0, ErrValidation
	}
	return reading, page.Width, page.Height, nil
}

func parseTesseractTSV(data []byte) (PageReading, error) {
	rows := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(rows) < 2 {
		return PageReading{}, ErrValidation
	}
	type textLine struct {
		key   string
		words []string
	}
	lines := make([]textLine, 0)
	lineIndexes := make(map[string]int)
	reading := PageReading{}
	imageWidth, imageHeight := 0.0, 0.0
	totalConfidence, confidentWords := 0.0, 0
	for _, rawRow := range rows[1:] {
		if rawRow == "" {
			continue
		}
		// This is TSV, not CSV: OCR punctuation may contain quotes and the
		// final text cell can contain tabs. Split only the fixed 11 separators.
		row := strings.SplitN(rawRow, "\t", 12)
		if len(row) != 12 {
			return PageReading{}, ErrValidation
		}
		level, err := strconv.Atoi(row[0])
		if err != nil {
			return PageReading{}, err
		}
		if level == 1 {
			imageWidth, err = strconv.ParseFloat(row[8], 64)
			if err != nil {
				return PageReading{}, ErrValidation
			}
			imageHeight, err = strconv.ParseFloat(row[9], 64)
			if err != nil || imageWidth <= 0 || imageHeight <= 0 || math.IsNaN(imageWidth) || math.IsNaN(imageHeight) || math.IsInf(imageWidth, 0) || math.IsInf(imageHeight, 0) {
				return PageReading{}, ErrValidation
			}
			continue
		}
		if level != 5 || strings.TrimSpace(row[11]) == "" {
			continue
		}
		left, e1 := strconv.ParseFloat(row[6], 64)
		top, e2 := strconv.ParseFloat(row[7], 64)
		width, e3 := strconv.ParseFloat(row[8], 64)
		height, e4 := strconv.ParseFloat(row[9], 64)
		confidence, e5 := strconv.ParseFloat(row[10], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil {
			return PageReading{}, ErrValidation
		}
		key := strings.Join(row[1:5], ":")
		index, ok := lineIndexes[key]
		if !ok {
			index = len(lines)
			lineIndexes[key] = index
			lines = append(lines, textLine{key: key})
		}
		wordText := strings.TrimSpace(row[11])
		lines[index].words = append(lines[index].words, wordText)
		var confidenceValue *float64
		if confidence >= 0 {
			normalized := confidence / 100
			confidenceValue = &normalized
			totalConfidence += normalized
			confidentWords++
		}
		if imageWidth > 0 && imageHeight > 0 && validBox(left, top, width, height, imageWidth, imageHeight) {
			if len(reading.Regions) >= MaxPageRegions {
				return PageReading{}, ErrValidation
			}
			reading.Regions = append(reading.Regions, PageRegion{
				Kind: "word", Text: wordText, X: left / imageWidth, Y: top / imageHeight,
				Width: width / imageWidth, Height: height / imageHeight, Confidence: confidenceValue,
			})
		}
	}
	if confidentWords > 0 {
		average := totalConfidence / float64(confidentWords)
		reading.Confidence = &average
	}
	for _, line := range lines {
		if len(line.words) > 0 {
			reading.Text += strings.Join(line.words, " ") + "\n"
		}
	}
	reading.Text = strings.TrimSpace(reading.Text)
	if len(reading.Text) > maxPageTextBytes {
		return PageReading{}, ErrValidation
	}
	if imageWidth == 0 || imageHeight == 0 {
		return PageReading{}, fmt.Errorf("OCR image dimensions missing")
	}
	return reading, nil
}

func accountReading(reading PageReading, totalRegions, totalTextBytes *int) error {
	if len(reading.Text) > maxPageTextBytes || len(reading.Regions) > MaxPageRegions {
		return ErrValidation
	}
	*totalRegions += len(reading.Regions)
	*totalTextBytes += len(reading.Text)
	for _, region := range reading.Regions {
		// Region text is serialized in both the reading and the flattened page
		// index returned to the reviewer.
		*totalTextBytes += 2 * len(region.Text)
	}
	if *totalRegions > MaxDocumentRegions || *totalTextBytes > maxDocumentTextBytes {
		return ErrValidation
	}
	return nil
}

func validBox(x, y, width, height, pageWidth, pageHeight float64) bool {
	return pageWidth > 0 && pageHeight > 0 && width > 0 && height > 0 &&
		!math.IsNaN(x) && !math.IsNaN(y) && !math.IsNaN(width) && !math.IsNaN(height) &&
		x >= 0 && y >= 0 && x+width <= pageWidth+1 && y+height <= pageHeight+1
}

var _ Extractor = (*PDFExtractor)(nil)
