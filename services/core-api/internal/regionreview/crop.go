// Crop rendering for the region-review lane (plan 030-E). The reviewer's
// screen shows the crop the proposal came from — the plate's own pixels, not
// a re-rendering of them — beside the position the proposal's centre projects
// to. This file owns the two halves of that: the pure box arithmetic (which
// the tests pin without a database, a store, or a real scan) and the
// storage-backed render that turns one candidate row into two bounded images.
package regionreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"sync"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// parseCropBox reads a stored crop_px blob into a CropBox. The detector wrote
// integer pixel coordinates; anything else (floats from a future lane are
// fine — JSON numbers decode into int only when integral) is a proposal whose
// box cannot be rendered honestly, and it is refused rather than guessed.
func parseCropBox(raw []byte) (CropBox, error) {
	var box CropBox
	if err := json.Unmarshal(raw, &box); err != nil {
		return CropBox{}, ErrValidation
	}
	if box.W < 1 || box.H < 1 || box.X < 0 || box.Y < 0 {
		return CropBox{}, ErrValidation
	}
	return box, nil
}

// The render bounds, mirrored from the sourceprocessing suggestion crops: the
// bounds are deliberately applied to the RESPONSE, not to the proposal, so a
// malformed or oversized detector box can never turn a review click into an
// unbounded image. A context crop is a handful of neighbouring labels, not a
// second full-plate preview.
const (
	plateCropMaxSide   = 1600
	plateCropMaxPixels = 2_500_000
	// A tight crop alone does not answer "where am I on the plate?", so the
	// context crop grows the box until neighbouring labels appear.
	contextWidthScale  = 5.0
	contextHeightScale = 4.0
	contextMinWidth    = 560
	contextMinHeight   = 300
)

// contextBounds grows one crop box into the context crop around it, keeping
// the box centred, clamping to the plate, and then honouring the response
// bounds by shrinking the padding first (never the requested box below its
// own centre window). Pure so the arithmetic is testable without a scan.
func contextBounds(box CropBox, plateWidth, plateHeight int) CropBox {
	plateWidth = max(1, plateWidth)
	plateHeight = max(1, plateHeight)
	width := clampInt(box.W, 1, plateWidth)
	height := clampInt(box.H, 1, plateHeight)
	centreX := clampInt(box.X+width/2, 0, plateWidth)
	centreY := clampInt(box.Y+height/2, 0, plateHeight)

	targetWidth := maxInt(width*int(contextWidthScale), contextMinWidth)
	targetHeight := maxInt(height*int(contextHeightScale), contextMinHeight)

	// The response bounds: an area-and-side cap on the served context, applied
	// by scaling the window around the centre, never by moving it off the box.
	if targetWidth > plateCropMaxSide || targetHeight > plateCropMaxSide ||
		int64(targetWidth)*int64(targetHeight) > plateCropMaxPixels {
		scale := math.Min(
			float64(plateCropMaxSide)/float64(targetWidth),
			float64(plateCropMaxSide)/float64(targetHeight),
		)
		scale = math.Min(scale, math.Sqrt(plateCropMaxPixels/float64(targetWidth*targetHeight)))
		targetWidth = maxInt(1, int(float64(targetWidth)*scale))
		targetHeight = maxInt(1, int(float64(targetHeight)*scale))
	}

	targetWidth = minInt(targetWidth, plateWidth)
	targetHeight = minInt(targetHeight, plateHeight)
	left := clampInt(centreX-targetWidth/2, 0, plateWidth-targetWidth)
	top := clampInt(centreY-targetHeight/2, 0, plateHeight-targetHeight)
	return CropBox{X: left, Y: top, W: minInt(targetWidth, plateWidth-left), H: minInt(targetHeight, plateHeight-top)}
}

// tightBounds is the proposal's own box clamped into the plate. The crop shows
// the exact pixels the detector ranked — no added padding, because padding
// would change the box the score components describe.
func tightBounds(box CropBox, plateWidth, plateHeight int) (CropBox, error) {
	if box.W < 1 || box.H < 1 || box.X < 0 || box.Y < 0 {
		return CropBox{}, ErrValidation
	}
	plateWidth = max(1, plateWidth)
	plateHeight = max(1, plateHeight)
	left := clampInt(box.X, 0, plateWidth-1)
	top := clampInt(box.Y, 0, plateHeight-1)
	right := clampInt(box.X+box.W, left+1, plateWidth)
	bottom := clampInt(box.Y+box.H, top+1, plateHeight)
	return CropBox{X: left, Y: top, W: right - left, H: bottom - top}, nil
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

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

// CropGeometry tells the workbench where the served pixels sit: the plate's
// own size, the detector's box, and the two served regions in the same pixel
// coordinates, so an overlay of the projected centre can be drawn to scale.
type CropGeometry struct {
	PlateWidth  int     `json:"plateWidth"`
	PlateHeight int     `json:"plateHeight"`
	CropPx      CropBox `json:"cropPx"`
	TightBox    CropBox `json:"tightBox"`
	ContextBox  CropBox `json:"contextBox"`
	// Scaled reports that the detector's box itself exceeded the response
	// bounds and the served crop was reduced around its centre. A proposal
	// that big is not wrong — the cartouche is — but the response must say
	// what it served rather than pretend it served the whole box.
	Scaled bool `json:"scaled"`
}

// CropResponse carries the two crops as PNGs ([]byte is base64-encoded by
// encoding/json), the same transport the suggestion crops use, plus the
// geometry and the plate honesty block.
type CropResponse struct {
	CandidateID string        `json:"candidateId"`
	PlateKey    string        `json:"plateKey"`
	TightPNG    []byte        `json:"tightPng"`
	ContextPNG  []byte        `json:"contextPng"`
	Geometry    CropGeometry  `json:"geometry"`
	Plate       *PlateSummary `json:"plate,omitempty"`
}

// decodedPlate is one plate held decoded in memory. A plate is a 7163×7170
// scan — rendering a crop per request would re-decode ~50 megapixels of JPEG
// every time a reviewer moves to the next label, so the lane keeps the most
// recent plate decoded. Keyed by storage key AND the recorded checksum, so a
// re-uploaded file under an old key can never be served from a stale decode.
type decodedPlate struct {
	key      string
	checksum string
	image    image.Image
}

// plateCache holds at most one decoded plate (~50 MP ≈ 150 MiB decoded), the
// whole known corpus of georeferenced plates being two. Evicting on insert
// keeps the bound trivial and honest: no sizing policy to get wrong.
type plateCache struct {
	mu      sync.Mutex
	current *decodedPlate
}

func (c *plateCache) get(key, checksum string) image.Image {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil && c.current.key == key && c.current.checksum == checksum {
		return c.current.image
	}
	return nil
}

func (c *plateCache) put(key, checksum string, image image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = &decodedPlate{key: key, checksum: checksum, image: image}
}

// CandidateCrop reads one candidate's crop from the stored plate. The pixels
// come from the source_files row the lane's checksum points at; the geometry
// is arithmetic on the detector's own box; nothing here decides what the ink
// says.
func (s *Service) CandidateCrop(ctx context.Context, actorID, candidateID string) (CropResponse, error) {
	var err error
	var allowed bool
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return CropResponse{}, err
	}
	if !allowed {
		return CropResponse{}, ErrForbidden
	}
	candidateUUID, err := uuid.Parse(candidateID)
	if err != nil {
		return CropResponse{}, ErrValidation
	}

	// The crop belongs to the exact scan used when the candidate was created.
	var cropBytes []byte
	var plateKey, sourceID, titleAR, license, metadataBytes string
	var storageKey, mimeType, fileChecksum *string
	err = s.Pool.QueryRow(ctx, `
		SELECT c.crop_px, c.plate_key, src.id::text, COALESCE(src.title_ar, ''),
		       COALESCE(src.metadata->>'license', ''), COALESCE(src.metadata, '{}'::jsonb)::text,
		       sf.storage_key, sf.mime_type, sf.checksum_sha256
		FROM external_region_candidates c
		JOIN sources src ON src.id = c.source_id
		LEFT JOIN LATERAL (
			SELECT f.storage_key, f.mime_type, f.checksum_sha256
			FROM source_files f
			WHERE f.source_id = c.source_id AND f.checksum_sha256 = c.scan_sha256
			ORDER BY f.created_at, f.id
			LIMIT 1
		) sf ON true
		WHERE c.id = $1::uuid`, candidateUUID).Scan(
		&cropBytes, &plateKey, &sourceID, &titleAR, &license, &metadataBytes,
		&storageKey, &mimeType, &fileChecksum)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CropResponse{}, ErrNotFound
		}
		return CropResponse{}, err
	}
	if err := s.requirePlateReadable(ctx, sourceID, actorID); err != nil {
		return CropResponse{}, err
	}
	box, err := parseCropBox(cropBytes)
	if err != nil {
		return CropResponse{}, ErrValidation
	}
	if storageKey == nil || !storage.ValidKey(*storageKey) {
		return CropResponse{}, ErrCropUnavailable
	}

	source, err := s.decodedPlateImage(ctx, *storageKey, dereference(fileChecksum))
	if err != nil {
		return CropResponse{}, err
	}
	bounds := source.Bounds()
	plateWidth, plateHeight := bounds.Dx(), bounds.Dy()

	tight, err := tightBounds(box, plateWidth, plateHeight)
	if err != nil {
		return CropResponse{}, ErrValidation
	}
	served := tight
	scaled := false
	if tight.W > plateCropMaxSide || tight.H > plateCropMaxSide ||
		int64(tight.W)*int64(tight.H) > plateCropMaxPixels {
		// A box that big (the cartouche, a future multi-line proposal) is
		// served as a centre window of cap size: the reviewer still sees the
		// middle of the ink at full detail, and the geometry says so.
		served = contextBounds(tight, plateWidth, plateHeight)
		scaled = served != tight
	}
	tightRect := image.Rect(bounds.Min.X+served.X, bounds.Min.Y+served.Y,
		bounds.Min.X+served.X+served.W, bounds.Min.Y+served.Y+served.H)
	tightPNG, err := encodePNG(source, tightRect)
	if err != nil {
		return CropResponse{}, err
	}
	context := contextBounds(tight, plateWidth, plateHeight)
	contextRect := image.Rect(bounds.Min.X+context.X, bounds.Min.Y+context.Y,
		bounds.Min.X+context.X+context.W, bounds.Min.Y+context.Y+context.H)
	contextPNG, err := encodePNG(source, contextRect)
	if err != nil {
		return CropResponse{}, err
	}

	plate := plateSummaryFromMetadata(sourceID, plateKey, titleAR, license, []byte(metadataBytes))
	return CropResponse{
		CandidateID: candidateUUID.String(),
		PlateKey:    plateKey,
		TightPNG:    tightPNG,
		ContextPNG:  contextPNG,
		Geometry: CropGeometry{
			PlateWidth:  plateWidth,
			PlateHeight: plateHeight,
			CropPx:      box,
			TightBox:    served,
			ContextBox:  context,
			Scaled:      scaled,
		},
		Plate: &plate,
	}, nil
}

func (s *Service) decodedPlateImage(ctx context.Context, key, checksum string) (image.Image, error) {
	if s.plates == nil {
		return nil, ErrCropUnavailable
	}
	if cached := s.cache.get(key, checksum); cached != nil {
		return cached, nil
	}
	reader, _, err := s.plates.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrCropUnavailable
		}
		return nil, err
	}
	defer reader.Close()
	hash := sha256.New()
	stream := io.TeeReader(reader, hash)
	decoded, _, err := image.Decode(stream)
	if err != nil {
		return nil, ErrCropUnavailable
	}
	if _, err := io.Copy(io.Discard, stream); err != nil || hex.EncodeToString(hash.Sum(nil)) != checksum {
		return nil, ErrCropUnavailable
	}
	s.cache.put(key, checksum, decoded)
	return decoded, nil
}

func encodePNG(source image.Image, rect image.Rectangle) ([]byte, error) {
	var sub image.Image
	if cropper, ok := source.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		sub = cropper.SubImage(rect)
	} else {
		canvas := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(canvas, canvas.Bounds(), source, rect.Min, draw.Src)
		sub = canvas
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, sub); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
