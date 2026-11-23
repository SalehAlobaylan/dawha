// The comparison panel's own data read (plan 032-C): one map point per plate.
//
// The panel's purpose is "read the SAME point everywhere", so the read is a
// composition of stored truths: the stored conic's inverse (030-D) puts the
// point into the plate's own pixels, and the crop renders those actual pixels
// — no new data, no invented geometry. A plate without a usable transform
// returns this package's own refusal (032-A's reader) and the panel shows the
// honest caption instead of a crop nobody printed.
package regionreview

import (
	"context"
	"fmt"
	"image"
	"math"
)

// PointCropSide is the fixed side (in plate pixels) of a comparison crop:
// large enough to name the neighbourhood, small enough to stay a review-sized
// answer.
const PointCropSide = 480

// PointCropResponse is one point's read: the plate's own crop (base64 PNG;
// empty when the point sits outside the decoded sheet or outside the plate's
// neatline), the box it was served from, and the honesty block.
type PointCropResponse struct {
	PlateKey     string  `json:"plateKey"`
	Longitude    float64 `json:"longitude"`
	Latitude     float64 `json:"latitude"`
	PlatePointPx string  `json:"platePointPx"`
	CropPNG      []byte  `json:"cropPng"`
	CropBox      CropBox `json:"cropBox"`
	// InsideNeatline answers whether the point projects INSIDE the plate's
	// own neatline frame — false means this plate has nothing to say about
	// this point, which the panel states rather than hides.
	InsideNeatline bool          `json:"insideNeatline"`
	Plate          *PlateSummary `json:"plate,omitempty"`
}

// PlatePointCrop renders the comparison read for one plate at one map point.
// Moderator-readable, like every crop in this package.
func (s *Service) PlatePointCrop(ctx context.Context, actorID, plateKey string, lon, lat float64, side int) (PointCropResponse, error) {
	// The gate is the plate's own visibility, exactly like the tile warp: the
	// crop serves the plate's own pixels around one point, the same pixels the
	// XYZ layer renders for anyone who can read the plate. Review roles stay
	// for review surfaces (the candidate lane); this is the atlas's read.
	if lon < -180 || lon > 180 || lat < -85.05 || lat > 85.05 {
		return PointCropResponse{}, ErrValidation
	}
	if side <= 0 {
		side = PointCropSide
	}
	if side > plateCropMaxSide {
		return PointCropResponse{}, ErrValidation
	}

	transformation, storageKey, checksum, sourceID, err := s.plateTransformForOverlay(ctx, plateKey)
	if err != nil {
		return PointCropResponse{}, err
	}
	// The panel = a map read of the plate's own pixels: the plate's own
	// visibility gates the crop (see the warp's note).
	if err := s.requirePlateReadable(ctx, sourceID, actorID); err != nil {
		return PointCropResponse{}, err
	}
	source, err := s.decodedPlateImage(ctx, storageKey, checksum)
	if err != nil {
		return PointCropResponse{}, err
	}
	centerX, centerY := transformation.Transform.Inverse(lon, lat)

	frameLeft, frameTop, frameRight, frameBottom := transformation.Frame.Neatline()
	inside := centerX >= frameLeft && centerX < frameRight && centerY >= frameTop && centerY < frameBottom

	bounds := source.Bounds()
	response := PointCropResponse{
		PlateKey:       plateKey,
		Longitude:      lon,
		Latitude:       lat,
		PlatePointPx:   fmt.Sprintf("%.0f,%.0f", centerX, centerY),
		InsideNeatline: inside,
		Plate: &PlateSummary{
			SourceID:      sourceID,
			PlateKey:      plateKey,
			GeorefMethod:  transformation.Method,
			Certainty:     transformation.Certainty,
			NeedsReview:   transformation.NeedsReview,
			ReviewReasons: transformation.ReviewReasons,
		},
	}
	if !inside {
		// The stated absence: this plate has nothing at this point, and the
		// panel says so instead of showing a neighbourhood guess.
		return response, nil
	}

	half := side / 2
	left := clampInt(int(math.Floor(centerX))-half, 0, bounds.Dx()-1)
	top := clampInt(int(math.Floor(centerY))-half, 0, bounds.Dy()-1)
	width := minInt(side, bounds.Dx()-left)
	height := minInt(side, bounds.Dy()-top)
	response.CropBox = CropBox{X: left, Y: top, W: width, H: height}
	cropPNG, err := encodePNG(source, image.Rect(bounds.Min.X+left, bounds.Min.Y+top,
		bounds.Min.X+left+width, bounds.Min.Y+top+height))
	if err != nil {
		return PointCropResponse{}, err
	}
	response.CropPNG = cropPNG
	return response, nil
}
