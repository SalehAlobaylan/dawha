package sourceprocessing

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os/exec"
)

// A family chart arrives in whatever the family has kept: a scanned PDF, a
// photograph of a wall chart, a screenshot of a website, a PNG exported from a
// drawing program. The pipeline used to accept only PDF and text. This file is
// the image arm: a raster image is a document with exactly one page, and the
// page is the image.
//
// The image is measured in pixels, not points. Regions are normalised
// coordinates, so nothing downstream needs a physical page size; the pixel size
// is kept so that a reviewer can see the page's real dimensions.
//
// The original image is never modified. Previews and OCR readings are bounded
// derivatives: a phone photograph can be 50 megapixels, and both Tesseract and
// the reviewer's browser are better served by a bounded page. The bound is a
// render bound, not an evidence bound - the stored original remains the
// authority, exactly as with a PDF.

const (
	// maxSourceImagePixels bounds what the worker will decode from an original
	// image. Go decodes a PNG or JPEG into memory in full, so this is the
	// memory bound as much as a format bound. A 40-megapixel photograph - the
	// largest current phone camera - fits; a gigapixel panorama does not, and
	// refusing it early is better than exhausting the worker.
	maxSourceImagePixels = 40_000_000
	// maxReadingImageAxis is the longest side handed to OCR and to the
	// localiser. It matches the -scale-to 4000 the PDF path already uses, so
	// the two paths measure names at the same scale.
	maxReadingImageAxis = 4000
	// maxPreviewImageAxis is the longest side of a page preview. It matches
	// the -scale-to 2400 of the PDF preview.
	maxPreviewImageAxis = 2400
)

// imageContentTypes is the raster set the source contract accepts. GIF is
// included because a published family tree is often one GIF embedded in a web
// page: https://www.obaikan.net/ALALSHEEKH.htm is exactly that, a FrontPage
// page whose entire body is <img src="images/ALALSHEEKH.gif">.
var imageContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
}

// isImageContentType reports whether a stored media type is a raster page.
func isImageContentType(value string) bool {
	return imageContentTypes[normalizeContentType(value)]
}

// imageSource is the measured shape of a raster document.
type imageSource struct {
	Width  int
	Height int
	Format string
}

// inspectImageSource decides whether the bytes are a readable raster image of
// an acceptable size. It decodes only the header, so it is cheap enough to run
// at the upload boundary.
func inspectImageSource(content []byte) (imageSource, bool) {
	if len(content) < 8 || int64(len(content)) > MaxUploadBytes {
		return imageSource{}, false
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || config.Width < 1 || config.Height < 1 {
		return imageSource{}, false
	}
	if int64(config.Width)*int64(config.Height) > maxSourceImagePixels {
		return imageSource{}, false
	}
	switch format {
	case "jpeg", "png", "gif":
		return imageSource{Width: config.Width, Height: config.Height, Format: format}, true
	default:
		return imageSource{}, false
	}
}

// decodeImageSource decodes a raster document into RGBA. The pixel bound is
// checked before the decode so that a hostile header cannot make the worker
// allocate an unbounded image.
func decodeImageSource(content []byte) (*image.RGBA, imageSource, error) {
	inspection, ok := inspectImageSource(content)
	if !ok {
		return nil, imageSource{}, ErrValidation
	}
	decoded, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, imageSource{}, ErrValidation
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != inspection.Width || bounds.Dy() != inspection.Height {
		return nil, imageSource{}, ErrValidation
	}
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), decoded, bounds.Min, draw.Src)
	return canvas, inspection, nil
}

// renderImagePage fits a raster document into a bounded PNG. The original is
// kept elsewhere; this is the page a reader or a reviewer sees.
func renderImagePage(content []byte, maxAxis int) ([]byte, imageSource, error) {
	canvas, inspection, err := decodeImageSource(content)
	if err != nil {
		return nil, imageSource{}, err
	}
	if maxAxis < 1 {
		maxAxis = maxPreviewImageAxis
	}
	bounds := canvas.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width > maxAxis || height > maxAxis {
		scale := float64(maxAxis) / float64(max(width, height))
		width = max(1, int(float64(width)*scale))
		height = max(1, int(float64(height)*scale))
		canvas = scaleImageBox(canvas, width, height)
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		return nil, imageSource{}, ErrPDFProcessingFailed
	}
	return buffer.Bytes(), inspection, nil
}

// scaleImageBox resizes with a box filter. Averaging matters for a photograph:
// nearest-neighbour downscaling turns a name into a broken picket fence, which
// is the opposite of what OCR needs. The cost is one pass over the source
// pixels, and the memory is only the destination image.
func scaleImageBox(source *image.RGBA, width, height int) *image.RGBA {
	sourceBounds := source.Bounds()
	sourceWidth, sourceHeight := sourceBounds.Dx(), sourceBounds.Dy()
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sourceY0 := y * sourceHeight / height
		sourceY1 := (y + 1) * sourceHeight / height
		if sourceY1 <= sourceY0 {
			sourceY1 = sourceY0 + 1
		}
		for x := 0; x < width; x++ {
			sourceX0 := x * sourceWidth / width
			sourceX1 := (x + 1) * sourceWidth / width
			if sourceX1 <= sourceX0 {
				sourceX1 = sourceX0 + 1
			}
			var red, green, blue, alpha, count uint64
			for sampleY := sourceY0; sampleY < sourceY1 && sampleY < sourceHeight; sampleY++ {
				row := source.Pix[sampleY*source.Stride:]
				for sampleX := sourceX0; sampleX < sourceX1 && sampleX < sourceWidth; sampleX++ {
					offset := sampleX * 4
					red += uint64(row[offset])
					green += uint64(row[offset+1])
					blue += uint64(row[offset+2])
					alpha += uint64(row[offset+3])
					count++
				}
			}
			if count == 0 {
				continue
			}
			offset := y*out.Stride + x*4
			out.Pix[offset] = uint8(red / count)
			out.Pix[offset+1] = uint8(green / count)
			out.Pix[offset+2] = uint8(blue / count)
			out.Pix[offset+3] = uint8(alpha / count)
		}
	}
	return out
}

// cropImageRegion cuts a pixel region out of a decoded raster page and encodes
// it as PNG. It is the raster counterpart of pdftoppm's -x/-y/-W/-H tile.
func cropImageRegion(source image.Image, region ChartTileBounds) ([]byte, error) {
	if source == nil || region.X < 0 || region.Y < 0 || region.Width <= 0 || region.Height <= 0 {
		return nil, ErrValidation
	}
	bounds := source.Bounds()
	if region.X+region.Width > bounds.Dx() || region.Y+region.Height > bounds.Dy() {
		return nil, ErrValidation
	}
	if int64(region.Width)*int64(region.Height) > maxChartRegionPixels {
		return nil, ErrValidation
	}
	cropped := image.NewRGBA(image.Rect(0, 0, region.Width, region.Height))
	draw.Draw(cropped, cropped.Bounds(), source, bounds.Min.Add(image.Pt(region.X, region.Y)), draw.Src)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, cropped); err != nil {
		return nil, ErrPDFProcessingFailed
	}
	return buffer.Bytes(), nil
}

// extractImage builds the single Page of a raster document. There is no native
// text layer in an image, so every reading comes from OCR; the line detector
// and the name localiser run on the same bounded page as they do for a PDF.
func (e *PDFExtractor) extractImage(ctx context.Context, content []byte) ([]Page, error) {
	if e == nil {
		return nil, errExtractorUnavailable
	}
	pageImage, inspection, err := renderImagePage(content, maxReadingImageAxis)
	if err != nil {
		return nil, err
	}
	page := Page{Number: 1, Width: float64(inspection.Width), Height: float64(inspection.Height)}
	tesseractAvailable := e.Tesseract != ""
	if tesseractAvailable {
		_, lookupErr := exec.LookPath(e.Tesseract)
		tesseractAvailable = lookupErr == nil
	}
	if !tesseractAvailable {
		page.Readings = []PageReading{{Method: "tesseract_unavailable", Model: "tesseract-ara+eng", Selected: true}}
		return []Page{page}, nil
	}
	ocrReading, lineReading, carrierReading, readErr := e.readRenderedImage(ctx, pageImage, 1)
	if readErr != nil {
		// An image that cannot be OCR'd is still a page a reviewer can look at
		// and transcribe. The PDF path makes the same distinction: a scan with
		// no native text and no OCR is a failed extraction, but its page is not
		// lost. The unavailable reading is SELECTED because it is the page's
		// only reading; an unselected page would fail the processor's
		// one-selected-reading invariant.
		page.Readings = []PageReading{{Method: "tesseract_unavailable", Model: "tesseract-ara+eng", Selected: true}}
		if lineReading.Method != "" {
			page.Readings = append(page.Readings, lineReading)
		}
		if carrierReading.Method != "" {
			page.Readings = append(page.Readings, carrierReading)
		}
		return []Page{page}, nil
	}
	totalRegions, totalTextBytes := 0, 0
	if err := accountReading(ocrReading, &totalRegions, &totalTextBytes); err != nil {
		return []Page{page}, err
	}
	ocrReading.Method = "tesseract_ocr"
	ocrReading.Model = "tesseract-ara+eng"
	ocrReading.Selected = true
	page.Text = ocrReading.Text
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
	page.EndOffset = len([]rune(page.Text))
	return []Page{page}, nil
}
