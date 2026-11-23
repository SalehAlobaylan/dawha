package geography

import (
	"strings"
	"testing"
)

// TestValidatePlateTransformationAcceptsTheArtifactOwnShape pins the reader's
// contract on the writer's real artifact: the Admiralty 1916 conic — apex
// north of the sheet, cone constant inside (0,1), NEGATIVE latitude ratio,
// the printed 4-degree step's pixel scale, and the artifact's own 8-strip
// frame (the neatline inside the margin) with its held-out fit residuals.
// The validator accepts exactly this shape, and this shape only.
func TestValidatePlateTransformationAcceptsTheArtifactOwnShape(t *testing.T) {
	metadata := `{
		"license": "public_domain",
		"plate_key": "admiralty-1916-tribal",
		"georef": {
			"method": "graticule_labels",
			"certainty": "approximate",
			"needsReview": true,
			"reviewReasons": ["interior_parallels_unverified_by_ink"],
			"model": {
				"kind": "equidistant_conic",
				"apex": {"x": 3416.88, "y": -23190.58},
				"coneConstant": 0.444164,
				"standardParallelDeg": 26.37,
				"centralLongitude": 45.5126,
				"latitude": {"form": "lat = c0*r + c1", "coef": [-0.004322437, 140.780974604]},
				"pxPerDegLat": 231.351
			},
			"fit": {
				"lonRmsDeg": 0.02367, "latRmsDeg": 0.06896,
				"lonMaxDeg": 0.04141, "latMaxDeg": 0.15166
			},
			"frame": {
				"outer_left": 370.0, "inner_left": 514.5,
				"inner_top": 513.5, "outer_top": 369.0,
				"inner_right": 6604.5, "outer_right": 6748.5,
				"inner_bottom": 6604.5, "outer_bottom": 6750.0
			},
			"labels": {"top": [1, 2], "bottom": [3], "left": [4, 5], "right": [6]}
		}
	}`
	transformation, err := ValidatePlateTransformation([]byte(metadata))
	if err != nil {
		t.Fatalf("the artifact's own shape was refused: %v", err)
	}
	if transformation.Method != "graticule_labels" || transformation.Certainty != "approximate" || !transformation.NeedsReview {
		t.Fatalf("the honesty block was lost: %+v", transformation)
	}
	if len(transformation.ReviewReasons) != 1 || transformation.ReviewReasons[0] != "interior_parallels_unverified_by_ink" {
		t.Fatalf("review reasons were lost: %+v", transformation.ReviewReasons)
	}
	if transformation.Transform.ConeConstant != 0.444164 || transformation.Transform.PixelsPerDegree != 231.351 {
		t.Fatalf("the conic's parameters were lost: %+v", transformation.Transform)
	}
	if transformation.Fit.LatRMSDeg != 0.06896 || transformation.Fit.LonRMSDeg != 0.02367 {
		t.Fatalf("the held-out fit floor was lost: %+v", transformation.Fit)
	}
	// The control-point count is the REGISTER's own read (the badge's floor):
	// the printed label strips the conic was fitted on.
	if count := plateControlPointCount([]byte(metadata)); count != 6 {
		t.Fatalf("control point count = %d, want 6 (2 top + 1 bottom + 2 left + 1 right)", count)
	}
	// The frame's neatline reads as the INNER strips: the clip the overlay
	// honours, exactly the geometry the warp tests pin pixel-for-pixel.
	left, top, right, bottom := transformation.Frame.Neatline()
	if left != 514.5 || top != 513.5 || right != 6604.5 || bottom != 6604.5 {
		t.Fatalf("neatline = (%.1f, %.1f, %.1f, %.1f), want the inner strips", left, top, right, bottom)
	}
}

// TestValidatePlateTransformationRefusesTheShapes032ARefused: the affine's
// ghost, the mirrored latitude, the cone constants that write their own
// geometry, and the honest `method: "none"` — each refusal is a rule the
// writer already measured, not a reader's taste.
func TestValidatePlateTransformationRefusesTheShapes032ARefused(t *testing.T) {
	good := `{
		"georef": {
			"method": "graticule_labels",
			"model": {"kind": "equidistant_conic", "apex": {"x": 1, "y": -1}, "coneConstant": 0.444164,
				"centralLongitude": 45, "latitude": {"coef": [-0.0043, 140]}, "pxPerDegLat": 200},
			"frame": {"outer_left": 0, "inner_left": 1, "inner_top": 1, "outer_top": 0,
				"inner_right": 9, "outer_right": 10, "inner_bottom": 9, "outer_bottom": 10}
		}
	}`
	cases := []struct {
		name     string
		metadata string
	}{
		{"affine kind stows away", strings.Replace(good, "equidistant_conic", "affine", 1)},
		{"positive latitude mirrors the plate", strings.Replace(good, `[-0.0043, 140]`, `[0.0043, 140]`, 1)},
		{"cone constant of one degenerates", strings.Replace(good, "0.444164", "1", 1)},
		{"negative cone constant unfolds", strings.Replace(good, "0.444164", "-0.5", 1)},
		{"pixel scale is not a map", strings.Replace(good, "200", "999999", 1)},
		{"frame flips its own edges", strings.Replace(good, `"inner_left": 1,`, `"inner_left": 11,`, 1)},
		{"method: none refuses itself", strings.Replace(good, `"graticule_labels"`, `"none"`, 1)},
	}
	for _, testCase := range cases {
		if _, err := ValidatePlateTransformation([]byte(testCase.metadata)); err != ErrPlateTransformInvalid {
			t.Fatalf("%s: err = %v, want ErrPlateTransformInvalid", testCase.name, err)
		}
	}
}

// TestValidatePlateTransformationKeepsTheNoTransformPlateDocumented pins the
// Dickson plate's honest refuse: the plate DOES answer, captions itself from
// its own artifact's reason, and no transform pretends to exist.
func TestValidatePlateTransformationKeepsTheNoTransformPlateDocumented(t *testing.T) {
	metadata := `{
		"georef": {
			"method": "none",
			"reason": "planned route: anchor-town control points, fitted by a reviewer in E/F",
			"certainty": "none",
			"needsReview": true,
			"reviewReasons": ["no_graticule"]
		}
	}`
	caption := plateDisabledCaption([]byte(metadata), ErrPlateTransformInvalid)
	if !strings.Contains(caption, "anchor-town control points") {
		t.Fatalf("the artifact's own reason was not carried: %s", caption)
	}
	if _, err := ValidatePlateTransformation([]byte(metadata)); err != ErrPlateTransformInvalid {
		t.Fatalf("method none validated as a transform: %v", err)
	}
}

func TestAffineRegistrationNeedsHeldOutFitAndRoundTrips(t *testing.T) {
	metadata := `{"georef":{"method":"control_points","certainty":"approximate",
		"model":{"kind":"affine","affine":[0.01,0.001,40,0.001,-0.01,25]},
		"frame":{"outer_left":0,"inner_left":0,"outer_right":100,"inner_right":100,
		         "outer_top":0,"inner_top":0,"outer_bottom":100,"inner_bottom":100},
		"fit":{"heldOutCount":3,"maxErrorKm":1.2,"maxAllowedKm":2}}}`
	row, err := ValidatePlateTransformation([]byte(metadata))
	if err != nil {
		t.Fatal(err)
	}
	lon, lat := row.Transform.Forward(50, 20)
	x, y := row.Transform.Inverse(lon, lat)
	if x < 49.999999 || x > 50.000001 || y < 19.999999 || y > 20.000001 {
		t.Fatalf("affine inverse: %v, %v", x, y)
	}
	refused := strings.Replace(metadata, `"maxAllowedKm":2`, `"maxAllowedKm":1`, 1)
	if _, err := ValidatePlateTransformation([]byte(refused)); err != ErrPlateTransformInvalid {
		t.Fatalf("accepted a map exceeding its error budget: %v", err)
	}
}
