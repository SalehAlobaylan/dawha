// Plate tiles (plan 032-B): the warped overlay layer's own pixel pipeline.
//
// WHY tiles, and not maplibre's four-corner image source: the plate's own
// measured truth (030-D) is that its parallels BOW — an affine model placed
// the plate's own town symbols 0.5–0.8° south and was rejected for it, and a
// four-corner image source is that same affine-class guess rendered in the
// browser (its interior is a bilinear interpolation between its corners). So
// the warp happens HERE, per output pixel, through the very inverse the
// 030-D writer published. The tile is an IMAGE for display (stipulation 3),
// never region geometry: outside the plate's neatline frame it is
// transparent, and nothing becomes a `historical_regions` row through it.
package regionreview

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The XYZ contract: 256-px tiles inside the plate's useful zoom band.
const (
	TileSize   = 256
	MinTileZed = 2
	MaxTileZed = 7
)

// The warp's actor-readable gate rides the plate's own source visibility, not
// the moderation lane: the overlay is a map layer; review roles are for
// review surfaces. (see requirePlateReadable)

// WarpPlateTile renders one XYZ tile of the stored plate through the stored
// conic. Per output pixel: web-mercator → the stored inverse → the plate's
// own pixel, read bilinearly — and outside the neatline frame the output is
// transparent. The plate arrives through the same checksum-keyed cache the
// candidate crops use, so a reviewer clicking tiles pays the 50-megapixel
// decode once per plate, like the crop lane already does.
func (s *Service) WarpPlateTile(ctx context.Context, actorID, plateKey string, z, x, y int) ([]byte, geography.PlateTransformation, error) {
	if z < MinTileZed || z > MaxTileZed {
		return nil, geography.PlateTransformation{}, ErrValidation
	}
	maxIndex := 1 << z
	if x < 0 || x >= maxIndex || y < 0 || y >= maxIndex {
		return nil, geography.PlateTransformation{}, ErrValidation
	}

	transformation, storageKey, checksum, sourceID, err := s.plateTransformForOverlay(ctx, plateKey)
	if err != nil {
		return nil, geography.PlateTransformation{}, err
	}
	// The tile renders the plate's OWN pixels at the plate's OWN publicity:
	// the gate is the source's own visibility (a public-domain plate renders
	// for the anonymous map reader; a private plate renders for its own
	// audience), NOT the moderation lane — the overlay is a map layer, and
	// review roles are for review surfaces.
	if err := s.requirePlateReadable(ctx, sourceID, actorID); err != nil {
		return nil, geography.PlateTransformation{}, err
	}
	source, err := s.decodedPlateImage(ctx, storageKey, checksum)
	if err != nil {
		return nil, geography.PlateTransformation{}, err
	}

	frameLeft, frameTop, frameRight, frameBottom := transformation.Frame.Neatline()
	transform := transformation.Transform
	tile := image.NewNRGBA(image.Rect(0, 0, TileSize, TileSize))
	for pixelY := 0; pixelY < TileSize; pixelY++ {
		for pixelX := 0; pixelX < TileSize; pixelX++ {
			// Each output pixel's own center in web-mercator degrees.
			lon, lat := tileLonLat(z, x, y, pixelX, pixelY)
			plateX, plateY := transform.Inverse(lon, lat)
			if plateX < frameLeft || plateX >= frameRight || plateY < frameTop || plateY >= frameBottom {
				continue // outside the neatline: transparent
			}
			sampled, ok := sampleBilinear(source, plateX, plateY)
			if !ok {
				continue // the frame is wider than the decoded scan here: the honest clip
			}
			tile.SetNRGBA(pixelX, pixelY, sampled)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, tile); err != nil {
		return nil, geography.PlateTransformation{}, err
	}
	return buffer.Bytes(), *transformation, nil
}

// plateTransformForOverlay resolves one plate's stored transform through the
// geography reader's own validation (032-A's contract, the same validator, a
// second consumer) and returns the storage key and checksum the plate cache
// keys on — preferring the stored file over any sidecar copy (LATERAL, the
// crop query's own ordering).
func (s *Service) plateTransformForOverlay(ctx context.Context, plateKey string) (*geography.PlateTransformation, string, string, string, error) {
	var metadataBytes []byte
	var storageKey, checksum, sourceID *string
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(s.metadata, '{}'::jsonb)::text, sf.storage_key, sf.checksum_sha256, s.id::text
		FROM sources s
		LEFT JOIN LATERAL (
			SELECT f.storage_key, f.checksum_sha256
			FROM source_files f
			WHERE f.source_id = s.id AND f.checksum_sha256 = s.metadata->>'file_sha256'
			ORDER BY f.created_at, f.id
			LIMIT 1
		) sf ON true
		WHERE `+geography.PlateKeyExpression+` = $1`, plateKey).Scan(&metadataBytes, &storageKey, &checksum, &sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", "", ErrNotFound
		}
		return nil, "", "", "", err
	}
	transformation, err := geography.ValidatePlateTransformation([]byte(metadataBytes))
	if err != nil {
		return nil, "", "", "", err
	}
	if storageKey == nil {
		return nil, "", "", "", ErrCropUnavailable
	}
	return transformation, *storageKey, dereference(checksum), dereference(sourceID), nil
}

// requirePlateReadable carries the plate's own publicity to every rendered
// read: the source the plate lives in must be readable by THIS actor. A
// public-domain plate is a public layer; a private plate stays its own
// audience's.
func (s *Service) requirePlateReadable(ctx context.Context, sourceID, actorID string) error {
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return err
	}
	sourceUUID, err := uuid.Parse(sourceID)
	if err != nil {
		return ErrNotFound
	}
	access, err := policy.Source(ctx, s.Pool, sourceUUID)
	if err != nil {
		return err
	}
	if !access.Allowed() {
		return ErrForbidden
	}
	return nil
}

// tileLonLat is the per-pixel mercator read for (z, x, y) at the pixel's own
// position inside the 256-px tile: center-of-pixel sampling.
func tileLonLat(z, x, y, pixelX, pixelY int) (float64, float64) {
	world := float64(int(1) << z)
	normX := (float64(x)*TileSize + float64(pixelX) + 0.5) / (world * TileSize)
	normY := (float64(y)*TileSize + float64(pixelY) + 0.5) / (world * TileSize)
	lon := normX*360 - 180
	n := math.Pi - 2*math.Pi*normY
	lat := 180 / math.Pi * math.Atan(0.5*(math.Exp(n)-math.Exp(-n)))
	return lon, lat
}

// sampleBilinear reads one plate point bilinearly. `ok` is false outside the
// decoded scan — the frame may clip tighter than the scan but never wider
// without admitting that a corner of the sheet was not scanned.
func sampleBilinear(source image.Image, plateX, plateY float64) (color.NRGBA, bool) {
	bounds := source.Bounds()
	minX, minY := bounds.Min.X, bounds.Min.Y
	width, height := bounds.Dx(), bounds.Dy()
	fx := plateX - float64(minX)
	fy := plateY - float64(minY)
	if fx < 0 || fy < 0 || fx > float64(width-1) || fy > float64(height-1) {
		return color.NRGBA{}, false
	}
	x0 := clampInt(int(math.Floor(fx)), 0, width-1)
	y0 := clampInt(int(math.Floor(fy)), 0, height-1)
	x1 := clampInt(x0+1, 0, width-1)
	y1 := clampInt(y0+1, 0, height-1)
	dx := fx - math.Floor(fx)
	dy := fy - math.Floor(fy)

	a := colorInOut(source.At(minX+x0, minY+y0))
	b := colorInOut(source.At(minX+x1, minY+y0))
	c := colorInOut(source.At(minX+x0, minY+y1))
	d := colorInOut(source.At(minX+x1, minY+y1))

	var channels [4]uint8
	for channel := 0; channel < 4; channel++ {
		top := (1-dx)*a[channel] + dx*b[channel]
		bottom := (1-dx)*c[channel] + dx*d[channel]
		channels[channel] = clampUint8((1-dy)*top + dy*bottom)
	}
	out := color.NRGBA{R: channels[0], G: channels[1], B: channels[2], A: 255}
	return out, true
}

// colorInOut converts any decoded colour into four 0..255 float channels the
// sampler interpolates — one conversion per read, no per-row allocations.
func colorInOut(c color.Color) [4]float64 {
	r, g, b, a := c.RGBA()
	return [4]float64{float64(r >> 8), float64(g >> 8), float64(b >> 8), float64(a >> 8)}
}

func clampUint8(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 255 {
		return 255
	}
	return uint8(value)
}
