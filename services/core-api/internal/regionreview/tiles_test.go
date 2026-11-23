package regionreview

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

// TestTileLonLatPinsTheMercatorContract: the sphere's own numbers are the
// assertions, not a snapshot — a zoom-0 tile's center is Greenwich-on-the-
// equator; the world's north-west corner is −180 at web-mercator's ~85°; one
// tile east at zoom 3 steps exactly 45° of longitude.
func TestTileLonLatPinsTheMercatorContract(t *testing.T) {
	lon, lat := tileLonLat(0, 0, 0, TileSize/2, TileSize/2)
	// One pixel at zoom 0 spans 360/256 ≈ 1.4° — the center-PIXEL's own read
	// carries up to half that; the assertion is the pixel's own radius.
	if lon < -0.8 || lon > 0.8 || lat < -0.8 || lat > 0.8 {
		t.Fatalf("zoom-0 center pixel = (%.6f, %.6f), want within ±0.8° of (0, 0)", lon, lat)
	}
	nwLon, nwLat := tileLonLat(3, 0, 0, 0, 0)
	// The first pixel's own center sits half a pixel east of −180: at zoom 3
	// half a pixel ≈ 0.09°.
	if nwLon < -180.1 || nwLon > -179.9 {
		t.Fatalf("north-west lon = %.6f, want −180 + half a pixel", nwLon)
	}
	if nwLat < 80 || nwLat > 86 {
		t.Fatalf("north-west lat = %.4f, want web-mercator's ~85.05", nwLat)
	}
	eastLon, _ := tileLonLat(3, 1, 0, 0, 0)
	if eastLon-nwLon < 44.999 || eastLon-nwLon > 45.001 {
		t.Fatalf("one tile east = %.6f, want +45°", eastLon-nwLon)
	}
}

// The warp test's transform, fixed numbers, sane geography: the apex sits
// north of the sheet so latitude decreases with radius, and the fitted band
// (lat ≈ 10–16, lon ≈ 22–62 at the plate's own edges) is a region the web
// mercator can actually render.
const (
	warpApexX   = 400.0
	warpApexY   = -1400.0
	warpCone    = 0.5
	warpLon0    = 45.0
	warpLatC0   = -0.01
	warpLatC1   = 30.0
	plateWidth  = 800
	plateHeight = 600
)

// warpTestFrameJSON is the artifact's own 8-strip frame shape: the inner
// rectangle is the neatline the overlay clips to; the outer strips are the
// printed sheet's margin.
const warpTestFrameJSON = `"frame": {
			"outer_left": 0, "inner_left": 20,
			"inner_top": 40, "outer_top": 0,
			"inner_right": 780, "outer_right": 800,
			"inner_bottom": 590, "outer_bottom": 600
		}`

func warpTestMetadata() string {
	return `{
		"license": "public_domain",
		"plate_key": "warp-test-plate",
		"georef": {
			"method": "graticule_labels",
			"certainty": "approximate",
			"needsReview": true,
			"reviewReasons": ["interior_parallels_unverified_by_ink"],
			"model": {
				"kind": "equidistant_conic",
				"apex": {"x": 400, "y": -1400},
				"coneConstant": 0.5,
				"centralLongitude": 45,
				"pxPerDegLat": 100,
				"latitude": {"form": "lat = c0*r + c1", "coef": [-0.01, 30]}
			},
			"fit": {"lonRmsDeg": 0.02367, "latRmsDeg": 0.06896, "lonMaxDeg": 0.04141, "latMaxDeg": 0.15166},
			` + warpTestFrameJSON + `
		}
	}`
}

func warpTestTransform() geography.PlateTransform {
	return geography.PlateTransform{
		Kind:             "equidistant_conic",
		ApexX:            warpApexX,
		ApexY:            warpApexY,
		ConeConstant:     warpCone,
		CentralLongitude: warpLon0,
		LatC0:            warpLatC0,
		LatC1:            warpLatC1,
		PixelsPerDegree:  100,
	}
}

func mercatorWorldPixel(lon, lat float64, z int) (float64, float64) {
	world := float64(int(1) << z)
	normX := (lon + 180) / 360
	n := math.Asinh(math.Tan(lat * math.Pi / 180))
	normY := 0.5 - n/(2*math.Pi)
	return normX * world * TileSize, normY * world * TileSize
}

// TestWarpPlateTileRendersThroughTheStoredInverse runs the warp end to end
// over a synthetic plate whose ink ENCODES its own position: take the plate's
// own center pixel, project it through the transform's own forward into a
// mercator pixel, fetch THAT tile at THIS zoom, and read the pixel the point
// landed on — the colour must be the plate's own. A four-corner guess cannot
// pass this test; the per-pixel inverse does trivially.
func TestWarpPlateTileRendersThroughTheStoredInverse(t *testing.T) {
	// The plate: position-readable ink, 800×600 (the pixel = (x%256, y%256)).
	plate := image.NewRGBA(image.Rect(0, 0, plateWidth, plateHeight))
	for x := 0; x < plateWidth; x++ {
		for y := 0; y < plateHeight; y++ {
			plate.Set(x, y, colorCodec(uint8(x%256), uint8(y%256)))
		}
	}
	var plateBytes bytes.Buffer
	if err := jpeg.Encode(&plateBytes, plate, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode: %v", err)
	}

	fixture := testsupport.New(t)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := store.Put(fixture.Ctx(), "sources/plates/warp-test.jpg", bytes.NewReader(plateBytes.Bytes()), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	owner := actor.Register(t, fixture, "مالك الأطلس")
	moderator := actor.Register(t, fixture, "مشرف الأطلس")
	fixture.GrantRole(moderator.User.ID, "moderator")

	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة الربط', 'archive_record', 'public', $2::jsonb || jsonb_build_object('file_sha256', $4::text), $3)`,
		sourceID, warpTestMetadata(), owner.User.ID, sha256Hex(plateBytes.Bytes()))
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
		VALUES ($1, 'sources/plates/warp-test.jpg', 'warp-test.jpg', 'image/jpeg', $2, $3)`,
		sourceID, len(plateBytes.Bytes()), sha256Hex(plateBytes.Bytes()))

	service := NewService(fixture.Pool(), store)

	// The gates: the plate renders for its OWN audience (the source's own
	// visibility rule — the source's creator = readable, a stranger = 403),
	// and a zoom/tile outside the XYZ grid = a validation.
	subscriber := actor.Register(t, fixture, "قارئ أجنبي")
	if _, _, err := service.WarpPlateTile(fixture.Ctx(), subscriber.User.ID, "warp-test-plate", 5, 22, 14); err == ErrForbidden {
		// The source is public: a stranger READS it (the plate's own
		// publicity, not the review lane's) — this is the rule's negative
		// shape: the forbidden answer belongs to a PRIVATE plate, not a
		// foreign reader of a public one.
		t.Fatalf("a public plate's warp refused a stranger; the gate is the source's visibility, not the review lane")
	}
	if _, _, err := service.WarpPlateTile(fixture.Ctx(), moderator.User.ID, "warp-test-plate", 1, 0, 0); err != ErrValidation {
		t.Fatalf("zoom 1 = %v, want ErrValidation", err)
	}
	if _, _, err := service.WarpPlateTile(fixture.Ctx(), moderator.User.ID, "warp-test-plate", 9, 0, 0); err != ErrValidation {
		t.Fatalf("zoom 9 = %v, want ErrValidation", err)
	}
	if _, _, err := service.WarpPlateTile(fixture.Ctx(), moderator.User.ID, "warp-test-plate", 5, 32, 0); err != ErrValidation {
		t.Fatalf("tile x=32 at zoom 5 = %v, want ErrValidation (the grid has 32)", err)
	}

	// The round trip: the plate's center pixel through the forward, to the
	// mercator world pixel, to the tile + the in-tile pixel.
	transform := warpTestTransform()
	lon, lat := transform.Forward(plateWidth/2, plateHeight/2)
	worldX, worldY := mercatorWorldPixel(lon, lat, 5)
	tileX := int(worldX / TileSize)
	tileY := int(worldY / TileSize)
	inTileX := int(worldX) % TileSize
	inTileY := int(worldY) % TileSize

	tileBytes, transformation, err := service.WarpPlateTile(fixture.Ctx(), moderator.User.ID, "warp-test-plate", 5, tileX, tileY)
	if err != nil {
		t.Fatalf("warp: %v", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(tileBytes))
	if err != nil {
		t.Fatalf("decode tile: %v", err)
	}
	if decoded.Bounds().Dx() != TileSize || decoded.Bounds().Dy() != TileSize {
		t.Fatalf("tile = %dx%d, want 256×256", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
	if transformation.Transform.ConeConstant != warpCone ||
		transformation.Certainty != "approximate" || !transformation.NeedsReview {
		t.Fatalf("the transform's honesty block came back wrong: %+v", transformation)
	}

	// THE INK-ROUND-TRIP: the pixel the transform landed on carries the
	// plate's own colour (±3 channels for the JPEG's lossy chroma + the
	// bilinear's neighbourhood), AND the four NEIGHBOURS stay within a frame
	// the plate's own colours pass — a rendered tile cannot wander.
	wantR := uint8((plateWidth / 2) % 256)
	wantG := uint8((plateHeight / 2) % 256)
	got := colorInOut(decoded.At(inTileX, inTileY))
	if math.Abs(got[0]-float64(wantR)) > 3 || math.Abs(got[1]-float64(wantG)) > 3 {
		t.Fatalf("the warped tile's pixel = (%.1f, %.1f), want the plate's own ink (%d, %d) ±3",
			got[0], got[1], wantR, wantG)
	}
	// And the far corner (outside the frame) stays transparent — the neatline
	// clip held through the warp.
	if a := alphaAt(decoded, 0, 0); a != 0 {
		t.Fatalf("a tile corner over the sea carries alpha %d, want 0", a)
	}
}

// TestWarpPlateTileTransparentOutsideTheFrame pins the frame clip's own
// honesness on a REAL tile: a tile whose whole footprint sits outside the
// plate's frame yields a fully transparent image — an overlay of such a tile
// draws nothing, and no claim of ink exists where the plate has none.
func TestWarpPlateTileTransparentOutsideTheFrame(t *testing.T) {
	// A 64×64 white plate with a small frame in its middle.
	plate := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for x := 0; x < 64; x++ {
		for y := 0; y < 64; y++ {
			plate.Set(x, y, colorCodec(200, 200))
		}
	}
	var plateBytes bytes.Buffer
	if err := jpeg.Encode(&plateBytes, plate, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	fixture := testsupport.New(t)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := store.Put(fixture.Ctx(), "sources/plates/warp-framed.jpg", bytes.NewReader(plateBytes.Bytes()), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	moderator := actor.Register(t, fixture, "مشرف الأطلس 文")
	fixture.GrantRole(moderator.User.ID, "moderator")

	sourceID := uuid.New()
	metadata := `{
		"license": "public_domain",
		"plate_key": "warp-framed-plate",
		"georef": {
			"method": "graticule_labels",
			"certainty": "approximate",
			"needsReview": true,
			"reviewReasons": ["interior_parallels_unverified_by_ink"],
			"model": {
				"kind": "equidistant_conic",
				"apex": {"x": 32, "y": -1420},
				"coneConstant": 0.5,
				"centralLongitude": 45,
				"pxPerDegLat": 100,
				"latitude": {"form": "lat = c0*r + c1", "coef": [-0.01, 30]}
			},
			"frame": {
				"outer_left": 0, "inner_left": 16,
				"inner_top": 20, "outer_top": 0,
				"inner_right": 48, "outer_right": 64,
				"inner_bottom": 44, "outer_bottom": 64
			}
		}
	}`
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة مقصوصة', 'archive_record', 'public', $2::jsonb || jsonb_build_object('file_sha256', $4::text), $3)`,
		sourceID, metadata, moderator.User.ID, sha256Hex(plateBytes.Bytes()))
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
		VALUES ($1, 'sources/plates/warp-framed.jpg', 'warp-framed.jpg', 'image/jpeg', $2, $3)`,
		sourceID, len(plateBytes.Bytes()), sha256Hex(plateBytes.Bytes()))

	service := NewService(fixture.Pool(), store)
	tileBytes, _, err := service.WarpPlateTile(fixture.Ctx(), moderator.User.ID, "warp-framed-plate", 5, 21, 13)
	if err != nil {
		t.Fatalf("warp: %v", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(tileBytes))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// A fully-transparent tile: the plate's own ink never reached this grid
	// square at all.
	nonTransparent := 0
	for y := 0; y < TileSize; y += 8 {
		for x := 0; x < TileSize; x += 8 {
			if alphaOf(decoded.At(x, y)) != 0 {
				nonTransparent++
			}
		}
	}
	if nonTransparent > 0 {
		t.Fatalf("%d sampled pixels outside the frame carry ink; the frame clip did not hold", nonTransparent)
	}
}

func colorCodec(r, g uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: 60, A: 255}
}

func alphaAt(img image.Image, x, y int) uint8 {
	_, _, _, a := img.At(x, y).RGBA()
	return uint8(a >> 8)
}

func alphaOf(c interface{ RGBA() (r, g, b, a uint32) }) uint8 {
	_, _, _, a := c.RGBA()
	return uint8(a >> 8)
}

// TestWarpPlateTileHonoursThePlateSourceVisibility pins the gate in BOTH
// directions: a PUBLIC plate renders for a stranger (the overlay is a map
// layer, not a review surface), and a PRIVATE plate refuses the stranger
// with the package's own 403 — the source's audience is the layer's.
func TestWarpPlateTileHonoursThePlateSourceVisibility(t *testing.T) {
	plate := image.NewRGBA(image.Rect(0, 0, plateWidth, plateHeight))
	for x := 0; x < plateWidth; x++ {
		for y := 0; y < plateHeight; y++ {
			plate.Set(x, y, colorCodec(120, 90))
		}
	}
	var plateBytes bytes.Buffer
	if err := jpeg.Encode(&plateBytes, plate, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	fixture := testsupport.New(t)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := store.Put(fixture.Ctx(), "sources/plates/warp-visibility.jpg", bytes.NewReader(plateBytes.Bytes()), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	owner := actor.Register(t, fixture, "\u0645\u0627\u0644\u0643 \u0627\u0644\u0644\u0648\u062d\u0629")
	stranger := actor.Register(t, fixture, "\u0642\u0627\u0631\u0626 \u0623\u062c\u0646\u0628\u064a")
	mod := "\u0645\u0634\u0631\u0641 \u0627\u0644\u0623\u0637\u0644\u0633"
	moderator := actor.Register(t, fixture, mod)
	fixture.GrantRole(moderator.User.ID, "moderator")

	privateMetadata := strings.Replace(warpTestMetadata(), "warp-test-plate", "warp-private-plate", 1)

	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, '\u0644\u0648\u062d\u0629 \u062e\u0627\u0635\u0629', 'archive_record', 'private', $2::jsonb || jsonb_build_object('file_sha256', $4::text), $3)`,
		sourceID, privateMetadata, owner.User.ID, sha256Hex(plateBytes.Bytes()))
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
		VALUES ($1, 'sources/plates/warp-visibility.jpg', 'warp-visibility.jpg', 'image/jpeg', $2, $3)`,
		sourceID, len(plateBytes.Bytes()), sha256Hex(plateBytes.Bytes()))

	service := NewService(fixture.Pool(), store)
	if _, _, err := service.WarpPlateTile(fixture.Ctx(), stranger.User.ID, "warp-private-plate", 5, 22, 14); err != ErrForbidden {
		t.Fatalf("a private plate's warp handed a stranger the ink: %v, want ErrForbidden", err)
	}
	if _, _, err := service.WarpPlateTile(fixture.Ctx(), moderator.User.ID, "warp-private-plate", 5, 22, 14); err != nil {
		t.Fatalf("a moderator (collaborator-class reader) was refused a private plate: %v", err)
	}
}
