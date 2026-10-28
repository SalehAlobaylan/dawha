package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The review screen lets a person read a name from the source, not from a
// thumbnail. The page preview is deliberately bounded for the browser; a name on
// a 59-inch wall chart is a dozen pixels in that preview and a person cannot
// check it. This endpoint renders the suggestion's own region and a wider context
// region from the original at analysis resolution, so the reviewer sees the same
// pixels the reader saw.
const (
	// suggestionCropMaxSide keeps one review response comfortably inside a
	// browser's memory. A context crop is a handful of neighbouring names, not a
	// second full-page preview.
	suggestionCropMaxSide = 1600
	// suggestionCropMaxPixels is the area bound that goes with the side bound.
	suggestionCropMaxPixels = 2_500_000
	// suggestionContextWidthScale and HeightScale make the context crop wide
	// enough to include the neighbouring names a person uses to orient the
	// active one. A tight crop alone does not answer "where am I?".
	suggestionContextWidthScale  = 5.0
	suggestionContextHeightScale = 4.0
	suggestionContextMinWidth    = 560
	suggestionContextMinHeight   = 300
)

// SuggestionCrop is the pair of source-resolution images the review screen needs
// for one suggestion. []byte is base64-encoded by encoding/json, so the browser
// receives two PNGs in one response and one page open.
type SuggestionCrop struct {
	TightPNG   []byte `json:"tightPng"`
	ContextPNG []byte `json:"contextPng"`
}

// suggestionSource is the source-side context both review reads need: the page,
// the normalized region, the original bytes, and the extractor that knows how to
// open them. Loading it once per request keeps the permission check and the
// storage-size check impossible to forget in a new endpoint.
type suggestionSource struct {
	sourceFileID  uuid.UUID
	pageNumber    int
	suggestedText string
	alternatives  []string
	region        Region
	content       []byte
	extractor     *PDFExtractor
	storageKey    string
	checksum      string
	mimeType      string
}

func (s *Service) loadSuggestionSource(ctx context.Context, suggestionID string, actorID string) (suggestionSource, error) {
	return s.loadSuggestionContext(ctx, suggestionID, actorID, true)
}
func (s *Service) loadSuggestionContext(ctx context.Context, suggestionID string, actorID string, withContent bool) (suggestionSource, error) {
	if s == nil || s.Pool == nil {
		return suggestionSource{}, ErrDatabaseUnavailable
	}
	parsedSuggestionID, err := uuid.Parse(suggestionID)
	if err != nil {
		return suggestionSource{}, ErrNotFound
	}
	parsedActorID, err := uuid.Parse(actorID)
	if err != nil {
		return suggestionSource{}, ErrForbidden
	}
	var sourceFileID uuid.UUID
	var pageNumber int
	var suggestedText string
	var alternativesBytes, regionBytes []byte
	var sourceID uuid.UUID
	var storageKey, mimeType, checksum string
	err = s.Pool.QueryRow(ctx, `
		SELECT sg.source_file_id, sg.page_number, sg.suggested_text_ar, sg.alternatives,
		       sg.region, sf.source_id, sf.storage_key, sf.mime_type, sf.checksum_sha256
		FROM source_tree_suggestions sg
		JOIN source_files sf ON sf.id = sg.source_file_id
		WHERE sg.id = $1
	`, parsedSuggestionID).Scan(
		&sourceFileID, &pageNumber, &suggestedText, &alternativesBytes,
		&regionBytes, &sourceID, &storageKey, &mimeType, &checksum,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return suggestionSource{}, ErrNotFound
	}
	if err != nil {
		return suggestionSource{}, err
	}
	var region Region
	if err := json.Unmarshal(regionBytes, &region); err != nil {
		return suggestionSource{}, ErrValidation
	}
	var alternatives []string
	if len(alternativesBytes) > 0 {
		if err := json.Unmarshal(alternativesBytes, &alternatives); err != nil {
			return suggestionSource{}, ErrValidation
		}
	}
	allowed, err := canReviewSource(ctx, s.Pool, sourceID, parsedActorID)
	if err != nil {
		return suggestionSource{}, err
	}
	if !allowed {
		return suggestionSource{}, ErrForbidden
	}
	if !isImageContentType(mimeType) && normalizeContentType(mimeType) != "application/pdf" {
		return suggestionSource{}, ErrUnsupportedDocument
	}
	extractor, ok := s.Extractor.(*PDFExtractor)
	if !ok || extractor == nil {
		extractor = NewPDFExtractor()
	}
	source := suggestionSource{sourceFileID: sourceFileID, pageNumber: pageNumber, suggestedText: suggestedText, alternatives: alternatives, region: region, extractor: extractor, storageKey: storageKey, checksum: checksum, mimeType: mimeType}
	if !withContent {
		return source, nil
	}
	content, err := s.readNameOriginal(ctx, source)
	if err != nil {
		return suggestionSource{}, err
	}

	return suggestionSource{
		sourceFileID:  sourceFileID,
		pageNumber:    pageNumber,
		suggestedText: suggestedText,
		alternatives:  alternatives,
		region:        region,
		content:       content,
		extractor:     extractor,
		storageKey:    storageKey, checksum: checksum, mimeType: mimeType,
	}, nil
}

// GetSuggestionCrop renders the tight and context crops for one suggestion, from
// the original source. It is a read-only operation and requires review
// permission on the suggestion's source.
func (s *Service) GetSuggestionCrop(ctx context.Context, suggestionID string, actorID string) (SuggestionCrop, error) {
	source, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false)
	if err != nil {
		return SuggestionCrop{}, err
	}
	artifact, err := s.prepareNameCrop(ctx, source, nil, nil)
	return artifact.Crop, err
}

// SuggestionCandidates asks the configured vision model to suggest spellings for
// the suggestion's tight crop. It is deliberately on demand: the model gets one
// crop and no family context, returns hypotheses only, and nothing is persisted.
// The reviewer still has to choose or type the name.
func (s *Service) SuggestionCandidates(ctx context.Context, suggestionID string, actorID string) ([]string, error) {
	if err := s.requireHostedSuggestionReader(ctx, suggestionID, actorID); err != nil {
		return nil, err
	}
	source, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false)
	if err != nil {
		return nil, err
	}
	if s.ChartVision == nil {
		return nil, ErrChartVisionUnavailable
	}
	artifact, err := s.prepareNameCrop(ctx, source, nil, nil)
	if err != nil {
		return nil, err
	}
	candidates, err := s.ChartVision.SuggestNames(ctx, artifact.Crop.TightPNG)
	if err != nil {
		return nil, err
	}
	return cleanNameCandidates(candidates)
}

// cleanNameCandidates applies the same display bound as the rest of the bench.
// Eight names is already more than a person can compare; a provider that returns
// more has misunderstood the request rather than found more certainty.
func cleanNameCandidates(values []string) ([]string, error) {
	if len(values) > 8 {
		return nil, ErrValidation
	}
	seen := make(map[string]struct{}, len(values))
	names := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if len([]rune(name)) > 200 {
			return nil, ErrValidation
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

// suggestionCropBounds turns a normalized name region into two bounded source
// rectangles. It is pure so the arithmetic can be tested without a PDF, a
// database, or a stored object.
func suggestionCropBounds(region Region, pageWidth, pageHeight int, view string) (ChartTileBounds, error) {
	if pageWidth < 1 || pageHeight < 1 || view == "" {
		return ChartTileBounds{}, ErrValidation
	}
	if region.Width <= 0 || region.Height <= 0 || region.X < 0 || region.Y < 0 ||
		region.X+region.Width > 1.000001 || region.Y+region.Height > 1.000001 {
		return ChartTileBounds{}, ErrValidation
	}
	if view != "tight" && view != "context" {
		return ChartTileBounds{}, ErrValidation
	}
	left := int(math.Floor(region.X * float64(pageWidth)))
	top := int(math.Floor(region.Y * float64(pageHeight)))
	right := int(math.Ceil((region.X + region.Width) * float64(pageWidth)))
	bottom := int(math.Ceil((region.Y + region.Height) * float64(pageHeight)))
	left = clampInt(left, 0, pageWidth-1)
	top = clampInt(top, 0, pageHeight-1)
	right = clampInt(right, left+1, pageWidth)
	bottom = clampInt(bottom, top+1, pageHeight)
	nameWidth, nameHeight := right-left, bottom-top

	targetWidth, targetHeight := nameWidth, nameHeight
	if view == "context" {
		targetWidth = maxInt(nameWidth*int(suggestionContextWidthScale), suggestionContextMinWidth)
		targetHeight = maxInt(nameHeight*int(suggestionContextHeightScale), suggestionContextMinHeight)
	} else {
		padding := maxInt(4, maxInt(nameWidth, nameHeight)/5)
		targetWidth += 2 * padding
		targetHeight += 2 * padding
	}
	if targetWidth > pageWidth {
		targetWidth = pageWidth
	}
	if targetHeight > pageHeight {
		targetHeight = pageHeight
	}
	// Keep the name centred while the context grows around it, then clamp to the
	// page. Without the second clamp a name near an edge would produce a negative
	// origin and the renderer would refuse the crop.
	centreX := (left + right) / 2
	centreY := (top + bottom) / 2
	cropLeft := centreX - targetWidth/2
	cropTop := centreY - targetHeight/2
	cropLeft = clampInt(cropLeft, 0, maxInt(0, pageWidth-targetWidth))
	cropTop = clampInt(cropTop, 0, maxInt(0, pageHeight-targetHeight))
	cropWidth := minInt(targetWidth, pageWidth-cropLeft)
	cropHeight := minInt(targetHeight, pageHeight-cropTop)

	// The side and area bounds are deliberately applied to the RENDER, not to the
	// name. A malformed suggestion that covers half the page must not turn a
	// review click into a 16-megapixel response.
	if cropWidth > suggestionCropMaxSide || cropHeight > suggestionCropMaxSide ||
		int64(cropWidth)*int64(cropHeight) > suggestionCropMaxPixels {
		scale := math.Min(
			float64(suggestionCropMaxSide)/float64(cropWidth),
			float64(suggestionCropMaxSide)/float64(cropHeight),
		)
		scale = math.Min(scale, math.Sqrt(float64(suggestionCropMaxPixels)/float64(cropWidth*cropHeight)))
		cropWidth = maxInt(1, int(float64(cropWidth)*scale))
		cropHeight = maxInt(1, int(float64(cropHeight)*scale))
		// Re-centre the reduced crop; the name may be larger than the bound, but
		// the reviewer still sees its middle at the sharpest available detail.
		cropLeft = clampInt(centreX-cropWidth/2, 0, pageWidth-cropWidth)
		cropTop = clampInt(centreY-cropHeight/2, 0, pageHeight-cropHeight)
	}
	if cropWidth < 1 || cropHeight < 1 {
		return ChartTileBounds{}, ErrValidation
	}
	return ChartTileBounds{X: cropLeft, Y: cropTop, Width: cropWidth, Height: cropHeight}, nil
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
