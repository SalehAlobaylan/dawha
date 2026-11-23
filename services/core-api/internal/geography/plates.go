// Plate register (plan 032-A): the atlas's read side over the transforms the
// 030-D writer published into sources.metadata.
//
// The reader's contract is strict on purpose. 030-D measured its transform,
// rejected an affine for the right reason, and PUBLISHED the conic's own
// parameters; anything that reads them must validate the same shapes the
// writer validated, refuse the same shapes, and never silently degrade: a
// plate without a usable transform is a register row with an honest caption
// and no warping — never an approximate overlay with a screenshot's own
// confidence.
//
// The shapes here are the artifact's own (`infra/local/build-plate-georef.py`):
// metadata.georef carries a `frame` of outer/inner strips (the inner rectangle
// is the neatline the overlay clips to), a fitted conic `model`, a held-out
// `fit` block (the measured floor: rms/max residuals per axis), and the
// honesty block (certainty/needsReview/reviewReasons). The Dixon plate stores
// `method: "none"` with its own reason — the register lists it with that
// caption and no transform.
//
// What a validated transform guarantees, and what it does not: the stored
// conic reproduces the writer's own held-out RMS (the floor under every
// position derived through it); certainty stays `approximate`; and NOTHING
// here produces geometry — plates are images transformed for display, diras
// are rows promoted by reviewers.
package geography

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
)

var (
	// ErrPlateNotFound: the register was asked for a plate key it does not
	// hold. The atlas layer answers 404, never an empty success.
	ErrPlateNotFound = errors.New("plate not found")
	// ErrPlateTransformInvalid is a stored transform that does not pass the
	// reader's own validation. It is the honest refusal the register renders
	// as a caption (the overlay control disables itself with it), never a
	// silently-ignored payload.
	ErrPlateTransformInvalid = errors.New("plate transform is invalid")
)

// PlateTransform is the validated conic model, ready to evaluate in both
// directions. The JSON shape is the 030-D artifact's own; the validation is
// the reader's contract.
type PlateTransform struct {
	Affine           []float64 `json:"affine,omitempty"`
	Kind             string    `json:"kind"`
	ApexX            float64   `json:"apexX"`
	ApexY            float64   `json:"apexY"`
	ConeConstant     float64   `json:"coneConstant"`
	CentralLongitude float64   `json:"centralLongitude"`
	// LatC0/LatC1 are the artifact's `latitude.coef` (lat = c0*r + c1).
	LatC0 float64 `json:"latC0"`
	LatC1 float64 `json:"latC1"`
	// PixelsPerDegree is the artifact's pxPerDegLat; it validates the model's
	// scale against the frame and reports the badge's readable figure.
	PixelsPerDegree float64 `json:"pixelsPerDegree"`
}

// PlateFrame is the stored frame: the printed sheet's outer strips and the
// neatline's inner strips, in the artifact's own numbers. The neatline (the
// inner rectangle) is the clip the overlay honours; the outer strips locate
// the margin labelling.
type PlateFrame struct {
	OuterLeft   float64 `json:"outerLeft"`
	InnerLeft   float64 `json:"innerLeft"`
	InnerRight  float64 `json:"innerRight"`
	OuterRight  float64 `json:"outerRight"`
	OuterTop    float64 `json:"outerTop"`
	InnerTop    float64 `json:"innerTop"`
	InnerBottom float64 `json:"innerBottom"`
	OuterBottom float64 `json:"outerBottom"`
}

// Neatline returns the inner rectangle the overlay clips to: plate pixels
// [left, right) × [top, bottom).
func (f PlateFrame) Neatline() (left, top, right, bottom float64) {
	return f.InnerLeft, f.InnerTop, f.InnerRight, f.InnerBottom
}

// PlateFit is the artifact's held-out fit block: the measured floor under
// every position derived through the transform. The register and the tile
// headers report it; nothing treats it as an accuracy promise.
type PlateFit struct {
	LonRMSDeg float64 `json:"lonRmsDeg"`
	LatRMSDeg float64 `json:"latRmsDeg"`
	LonMaxDeg float64 `json:"lonMaxDeg"`
	LatMaxDeg float64 `json:"latMaxDeg"`
}

// PlateTransformation is the validated whole: the artifact method
// (graticule_labels / none), the model, the frame, the fit, and the honesty
// block.
type PlateTransformation struct {
	Method        string         `json:"method"`
	Transform     PlateTransform `json:"transform"`
	Frame         PlateFrame     `json:"frame"`
	Fit           PlateFit       `json:"fit"`
	Certainty     string         `json:"certainty"`
	NeedsReview   bool           `json:"needsReview"`
	ReviewReasons []string       `json:"reviewReasons"`
}

// PlateRegisterRow is one plate in the atlas's register: its source identity,
// its transform (nil when the plate has none — the Dixon plate), and the
// honesty fields the overlay control renders.
type PlateRegisterRow struct {
	Captions    []PlateCaption `json:"captions,omitempty"`
	SourceID    string         `json:"sourceId"`
	PlateKey    string         `json:"plateKey"`
	TitleAR     string         `json:"titleAr"`
	License     string         `json:"license"`
	PlateDate   string         `json:"plateDate,omitempty"`
	PlateWidth  int            `json:"plateWidth,omitempty"`
	PlateHeight int            `json:"plateHeight,omitempty"`
	// ControlPoints is the badge's floor count: the printed degree labels the
	// georeference was fitted on (the artifact's own control points), or what
	// the Dickson plate honestly has — zero control points and no transform.
	ControlPoints int                  `json:"controlPoints"`
	Transform     *PlateTransformation `json:"transform,omitempty"`
	// DisabledCaption carries the honest caption when there is no overlay:
	// WHY the control is off, in the artifact's own words from the metadata,
	// never a bare boolean.
	DisabledCaption string `json:"disabledCaption,omitempty"`
}

type PlateCaption struct {
	TribeID     string `json:"tribeId,omitempty"`
	ID          string `json:"id"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	PrintedName string `json:"printedName"`
	NameAR      string `json:"nameAr"`
}

type PlateRegisterResponse struct {
	Plates    []PlateRegisterRow `json:"plates"`
	Limit     int                `json:"limit"`
	Truncated bool               `json:"truncated"`
}

// PlateKeyExpression is the SQL expression that names a plate to the atlas's
// routes: the metadata's own plate_key when the generator wrote one, else the
// source's id. Both forms resolve everywhere (register rows and the
// tile/crop routes share it).
const PlateKeyExpression = `COALESCE(s.metadata->>'plate_key', s.id::text)`

// ValidatePlateTransformation reads a metadata-georef block the way the
// writer's own artifact pin it: the model is the fitted equidistant conic
// with a positive cone constant inside the open unit interval, a latitude
// ratio NEGATIVE (latitude must DECREASE as the radius grows — the conic apex
// sits north of the sheet), a pixels-per-degree scale a printed map plausibly
// lives at, and a frame whose inner neatline is a real rectangle inside the
// outer strips. Everything else is the invalid refusal — including the
// honest `method: "none"` rows, whose caption the register builds separately.
func ValidatePlateTransformation(raw []byte) (*PlateTransformation, error) {
	if len(raw) == 0 {
		return nil, ErrPlateTransformInvalid
	}
	var payload struct {
		Georef *struct {
			Method string `json:"method"`
			Model  *struct {
				Kind   string    `json:"kind"`
				Affine []float64 `json:"affine"`
				Apex   *struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				} `json:"apex"`
				ConeConstant     float64 `json:"coneConstant"`
				CentralLongitude float64 `json:"centralLongitude"`
				PixelsPerDegree  float64 `json:"pxPerDegLat"`
				Latitude         *struct {
					Form string    `json:"form"`
					Coef []float64 `json:"coef"`
				} `json:"latitude"`
			} `json:"model"`
			Frame *struct {
				OuterLeft   float64 `json:"outer_left"`
				InnerLeft   float64 `json:"inner_left"`
				InnerRight  float64 `json:"inner_right"`
				OuterRight  float64 `json:"outer_right"`
				OuterTop    float64 `json:"outer_top"`
				InnerTop    float64 `json:"inner_top"`
				InnerBottom float64 `json:"inner_bottom"`
				OuterBottom float64 `json:"outer_bottom"`
			} `json:"frame"`
			Fit *struct {
				LonRMSDeg    float64 `json:"lonRmsDeg"`
				HeldOutCount int     `json:"heldOutCount"`
				MaxErrorKm   float64 `json:"maxErrorKm"`
				MaxAllowedKm float64 `json:"maxAllowedKm"`
				LatRMSDeg    float64 `json:"latRmsDeg"`
				LonMaxDeg    float64 `json:"lonMaxDeg"`
				LatMaxDeg    float64 `json:"latMaxDeg"`
			} `json:"fit"`
			Certainty     string   `json:"certainty"`
			NeedsReview   bool     `json:"needsReview"`
			ReviewReasons []string `json:"reviewReasons"`
		} `json:"georef"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrPlateTransformInvalid
	}
	if payload.Georef == nil || payload.Georef.Model == nil || payload.Georef.Frame == nil {
		return nil, ErrPlateTransformInvalid
	}
	model, frame := payload.Georef.Model, payload.Georef.Frame
	// `method: "none"` is the writer's own honest refusal (the Dickson plate):
	// the plate renders NO overlay, whatever model bytes accidentally ride
	// along — the method is the contract, not the model's leftover shapes.
	if strings.TrimSpace(payload.Georef.Method) == "none" {
		return nil, ErrPlateTransformInvalid
	}
	transform := PlateTransform{Kind: model.Kind}
	switch model.Kind {
	case "equidistant_conic":
		if model.Apex == nil || model.Latitude == nil || len(model.Latitude.Coef) != 2 ||
			!isFinite(model.Apex.X) || !isFinite(model.Apex.Y) || !isFinite(model.ConeConstant) ||
			!isFinite(model.CentralLongitude) || model.ConeConstant <= 0 || model.ConeConstant >= 1 ||
			!isFinite(model.Latitude.Coef[0]) || !isFinite(model.Latitude.Coef[1]) || model.Latitude.Coef[0] >= 0 ||
			!isFinite(model.PixelsPerDegree) || model.PixelsPerDegree < 2 || model.PixelsPerDegree > 50_000 {
			return nil, ErrPlateTransformInvalid
		}
		transform.ApexX, transform.ApexY = model.Apex.X, model.Apex.Y
		transform.ConeConstant, transform.CentralLongitude = model.ConeConstant, model.CentralLongitude
		transform.LatC0, transform.LatC1 = model.Latitude.Coef[0], model.Latitude.Coef[1]
		transform.PixelsPerDegree = model.PixelsPerDegree
	case "affine":
		fit := payload.Georef.Fit
		if payload.Georef.Method != "control_points" || len(model.Affine) != 6 || fit == nil || fit.HeldOutCount < 3 ||
			!isFinite(fit.MaxErrorKm) || !isFinite(fit.MaxAllowedKm) || fit.MaxAllowedKm <= 0 || fit.MaxErrorKm < 0 ||
			fit.MaxErrorKm > fit.MaxAllowedKm {
			return nil, ErrPlateTransformInvalid
		}
		for _, coefficient := range model.Affine {
			if !isFinite(coefficient) {
				return nil, ErrPlateTransformInvalid
			}
		}
		a := model.Affine
		determinant := a[0]*a[4] - a[1]*a[3]
		if !isFinite(determinant) || determinant >= -1e-14 {
			return nil, ErrPlateTransformInvalid
		}
		transform.Affine = a
		transform.PixelsPerDegree = 1 / math.Hypot(a[3], a[4])
	default:
		return nil, ErrPlateTransformInvalid
	}
	// The neatline is a real rectangle inside positive pixels and inside the
	// outer strips; the outer strips are a real rectangle themselves. A frame
	// that fails these is a measurement the writer never made.
	if frame.InnerRight <= frame.InnerLeft || frame.InnerBottom <= frame.InnerTop {
		return nil, ErrPlateTransformInvalid
	}
	if frame.OuterRight <= frame.OuterLeft || frame.OuterBottom <= frame.OuterTop {
		return nil, ErrPlateTransformInvalid
	}
	values := []float64{frame.OuterLeft, frame.InnerLeft, frame.InnerRight, frame.OuterRight,
		frame.OuterTop, frame.InnerTop, frame.InnerBottom, frame.OuterBottom}
	for _, value := range values {
		if !isFinite(value) || value < 0 || value > 1_000_000 {
			return nil, ErrPlateTransformInvalid
		}
	}
	if frame.InnerLeft < frame.OuterLeft || frame.InnerRight > frame.OuterRight ||
		frame.InnerTop < frame.OuterTop || frame.InnerBottom > frame.OuterBottom {
		return nil, ErrPlateTransformInvalid
	}
	if model.Kind == "affine" {
		for _, point := range [][2]float64{
			{frame.InnerLeft, frame.InnerTop}, {frame.InnerLeft, frame.InnerBottom},
			{frame.InnerRight, frame.InnerTop}, {frame.InnerRight, frame.InnerBottom},
		} {
			lon, lat := transform.Forward(point[0], point[1])
			if !isFinite(lon) || !isFinite(lat) || math.Abs(lon) > 180 || math.Abs(lat) > 85 {
				return nil, ErrPlateTransformInvalid
			}
		}
	}
	reasons := payload.Georef.ReviewReasons
	if reasons == nil {
		reasons = []string{}
	}
	fit := PlateFit{}
	if payload.Georef.Fit != nil {
		fit = PlateFit{
			LonRMSDeg: payload.Georef.Fit.LonRMSDeg,
			LatRMSDeg: payload.Georef.Fit.LatRMSDeg,
			LonMaxDeg: payload.Georef.Fit.LonMaxDeg,
			LatMaxDeg: payload.Georef.Fit.LatMaxDeg,
		}
	}
	return &PlateTransformation{
		Method:    payload.Georef.Method,
		Transform: transform,
		Frame: PlateFrame{
			OuterLeft:   frame.OuterLeft,
			InnerLeft:   frame.InnerLeft,
			InnerRight:  frame.InnerRight,
			OuterRight:  frame.OuterRight,
			OuterTop:    frame.OuterTop,
			InnerTop:    frame.InnerTop,
			InnerBottom: frame.InnerBottom,
			OuterBottom: frame.OuterBottom,
		},
		Fit:           fit,
		Certainty:     payload.Georef.Certainty,
		NeedsReview:   payload.Georef.NeedsReview,
		ReviewReasons: reasons,
	}, nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// Forward evaluates the plate's own published forward formula at one pixel
// (the 030-D artifact's own forward string, made executable once):
//
//	lon = lon0 + degrees(atan2(px − apex.x, py − apex.y)) / n
//	lat = c0·hypot(px − apex.x, py − apex.y) + c1
func (t PlateTransform) Forward(px, py float64) (lon, lat float64) {
	if t.Kind == "affine" {
		a := t.Affine
		return a[0]*px + a[1]*py + a[2], a[3]*px + a[4]*py + a[5]
	}
	dx, dy := px-t.ApexX, py-t.ApexY
	theta := math.Atan2(dx, dy)
	lon = t.CentralLongitude + theta/t.ConeConstant*180/math.Pi
	lat = t.LatC0*math.Hypot(dx, dy) + t.LatC1
	return lon, lat
}

// Inverse evaluates the published inverse at one coordinate:
//
//	r  = (lat − c1)/c0
//	th = radians((lon − lon0)·n)
//	px = apex.x + r·sin(th)
//	py = apex.y + r·cos(th)
func (t PlateTransform) Inverse(lon, lat float64) (px, py float64) {
	if t.Kind == "affine" {
		a := t.Affine
		dx, dy := lon-a[2], lat-a[5]
		determinant := a[0]*a[4] - a[1]*a[3]
		return (a[4]*dx - a[1]*dy) / determinant, (-a[3]*dx + a[0]*dy) / determinant
	}
	r := (lat - t.LatC1) / t.LatC0
	theta := (lon - t.CentralLongitude) * t.ConeConstant * math.Pi / 180
	return t.ApexX + r*math.Sin(theta), t.ApexY + r*math.Cos(theta)
}

// PlateRegister reads the atlas's plate layer: every source that is a plate —
// decided by its stored georef block — with its validated transform or the
// honest disabled caption. Bounded.
func (s *Service) PlateRegister(ctx context.Context, actorID string) (PlateRegisterResponse, error) {
	if s.Pool == nil {
		return PlateRegisterResponse{}, ErrDatabaseUnavailable
	}
	if _, err := uuid.Parse(actorID); err != nil && strings.TrimSpace(actorID) != "" {
		return PlateRegisterResponse{}, ErrValidation
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return PlateRegisterResponse{}, err
	}
	params := visibility.NewParams()
	limitReference := params.Add(PlateRegisterLimit + 1)

	rows, err := s.Pool.Query(ctx, `
		SELECT s.id::text, `+PlateKeyExpression+`, COALESCE(s.title_ar, ''),
		       COALESCE(s.metadata->>'license', ''), COALESCE(s.publication_date_from::text, ''),
		       s.metadata,
		       (SELECT COALESCE(jsonb_agg(row_to_json(caption)), '[]'::jsonb)
		        FROM (
		            SELECT c.id, c.x, c.y, c.printed_name AS "printedName",
		                   COALESCE(rev.name_ar, c.name_ar) AS "nameAr",
		                   COALESCE(t.id::text, '') AS "tribeId"
		            FROM plate_captions c
                    LEFT JOIN tribes t ON t.id = c.tribe_id AND t.visibility = 'public'
		            LEFT JOIN LATERAL (
		                SELECT name_ar FROM plate_caption_revisions
		                WHERE caption_id = c.id AND published_at IS NOT NULL
		                ORDER BY published_at DESC, revision_order DESC LIMIT 1
		            ) rev ON true
		            WHERE c.source_id = s.id AND c.scan_sha256 = s.metadata->>'file_sha256'
		            ORDER BY c.catalog_order, c.id LIMIT 200
		        ) caption)
		FROM sources s
		WHERE s.metadata ? 'georef'
		  AND `+policy.SourcePredicate(params, "s.id")+`
		ORDER BY s.publication_date_from NULLS LAST, s.id
		LIMIT `+limitReference, params.Args()...)
	if err != nil {
		return PlateRegisterResponse{}, err
	}
	defer rows.Close()

	response := PlateRegisterResponse{Plates: []PlateRegisterRow{}, Limit: PlateRegisterLimit}
	for rows.Next() {
		var row PlateRegisterRow
		var metadataBytes []byte
		if err := rows.Scan(&row.SourceID, &row.PlateKey, &row.TitleAR, &row.License, &row.PlateDate, &metadataBytes, &row.Captions); err != nil {
			return PlateRegisterResponse{}, err
		}
		row.ControlPoints = plateControlPointCount(metadataBytes)
		transformation, transformErr := ValidatePlateTransformation(metadataBytes)
		if transformErr != nil {
			// The honest disable: WHY, in the artifact's own words, with the
			// plate still listed (the register never hides a plate it holds).
			row.DisabledCaption = plateDisabledCaption(metadataBytes, transformErr)
		} else {
			row.Transform = transformation
		}
		response.Plates = append(response.Plates, row)
	}
	if err := rows.Err(); err != nil {
		return PlateRegisterResponse{}, err
	}
	if len(response.Plates) > PlateRegisterLimit {
		response.Plates = response.Plates[:PlateRegisterLimit]
		response.Truncated = true
	}
	return response, nil
}

// PlateRegisterLimit bounds the atlas's plate register.
const PlateRegisterLimit = 20

// plateControlPointCount reads the artifact's printed-label strips: the
// degree labels whose anchor-derived values fitted the conic. A plate with
// `method: "none"` reports what it honestly has (zero).
func plateControlPointCount(metadata []byte) int {
	var payload struct {
		Georef *struct {
			Labels *struct {
				Top    []float64 `json:"top"`
				Bottom []float64 `json:"bottom"`
				Left   []float64 `json:"left"`
				Right  []float64 `json:"right"`
			} `json:"labels"`
		} `json:"georef"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil || payload.Georef == nil || payload.Georef.Labels == nil {
		return 0
	}
	labels := payload.Georef.Labels
	return len(labels.Top) + len(labels.Bottom) + len(labels.Left) + len(labels.Right)
}

// plateDisabledCaption converts a refused transform into the caption the
// overlay control shows: what the artifact itself says (a `georef.method:
// "none"` with its own reason — the Dickson plate's honest posture), else the
// reader's own refusal.
func plateDisabledCaption(metadata []byte, err error) string {
	var payload struct {
		Georef *struct {
			Method string `json:"method"`
			Reason string `json:"reason"`
		} `json:"georef"`
	}
	if unmarshalErr := json.Unmarshal(metadata, &payload); unmarshalErr == nil && payload.Georef != nil {
		if strings.TrimSpace(payload.Georef.Reason) != "" {
			return payload.Georef.Reason
		}
	}
	return fmt.Sprintf("المحوّل المخزَّن غير صالح (%s)؛ الطبقة متوقفة", err)
}
