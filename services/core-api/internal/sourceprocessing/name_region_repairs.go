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

// A region revision is evidence about geometry, not a correction of the
// machine's reading. The original rectangle and every reading stay in place;
// the revision says which rectangle the reviewer asked to read next. Derived
// crops are keyed by geometry, so a new revision gets new artifacts without
// deleting the originals.

// SuggestionRegionInput is one proposed rectangle and the geometry it was drawn
// on. Coordinate metadata is optional for a first preview and, when present,
// must agree with the stored frame exactly.
type SuggestionRegionInput struct {
	ExpectedGeometryRevision int    `json:"expectedGeometryRevision"`
	Region                   Region `json:"region"`
	CoordinateSpace          string `json:"coordinateSpace,omitempty"`
	AnalysisDPI              int    `json:"analysisDpi,omitempty"`
	SourceChecksum           string `json:"sourceChecksum,omitempty"`
	FrameWidth               int    `json:"frameWidth,omitempty"`
	FrameHeight              int    `json:"frameHeight,omitempty"`
	NoteAR                   string `json:"noteAr,omitempty"`
}

type SuggestionRegionPreview struct {
	GeometryRevision int              `json:"geometryRevision"`
	Region           Region           `json:"region"`
	CoordinateSpace  string           `json:"coordinateSpace"`
	AnalysisDPI      int              `json:"analysisDpi"`
	SourceChecksum   string           `json:"sourceChecksum"`
	FrameWidth       int              `json:"frameWidth"`
	FrameHeight      int              `json:"frameHeight"`
	TightBounds      ChartTileBounds  `json:"tightBounds"`
	ContextBounds    ChartTileBounds  `json:"contextBounds"`
	Quality          CropObservations `json:"quality"`
	TightPNG         []byte           `json:"tightPng"`
	ContextPNG       []byte           `json:"contextPng"`
}

// validateNormalizedRegion is deliberately pure: the same arithmetic guards a
// preview, a confirmation, and a split/merge child. A rectangle that is empty,
// inverted, non-finite, or off the page is refused rather than clamped, because
// a clamped rectangle would silently read pixels the reviewer did not choose.
func validateNormalizedRegion(region Region) error {
	for _, value := range []float64{region.X, region.Y, region.Width, region.Height} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return ErrValidation
		}
	}
	if region.Width <= 0 || region.Height <= 0 || region.X < 0 || region.Y < 0 {
		return ErrValidation
	}
	if region.X+region.Width > 1.000001 || region.Y+region.Height > 1.000001 {
		return ErrValidation
	}
	return nil
}

func sameRegion(a, b Region) bool {
	const epsilon = 1e-9
	return math.Abs(a.X-b.X) <= epsilon && math.Abs(a.Y-b.Y) <= epsilon &&
		math.Abs(a.Width-b.Width) <= epsilon && math.Abs(a.Height-b.Height) <= epsilon
}

func regionContains(outer, inner Region) bool {
	const epsilon = 1e-9
	return inner.X+epsilon >= outer.X && inner.Y+epsilon >= outer.Y &&
		inner.X+inner.Width <= outer.X+outer.Width+epsilon &&
		inner.Y+inner.Height <= outer.Y+outer.Height+epsilon
}

func regionsOverlap(a, b Region) bool {
	left := math.Max(a.X, b.X)
	top := math.Max(a.Y, b.Y)
	right := math.Min(a.X+a.Width, b.X+b.Width)
	bottom := math.Min(a.Y+a.Height, b.Y+b.Height)
	return right-left > 1e-9 && bottom-top > 1e-9
}

func regionUnion(regions []Region) Region {
	if len(regions) == 0 {
		return Region{}
	}
	union := regions[0]
	for _, region := range regions[1:] {
		right := math.Max(union.X+union.Width, region.X+region.Width)
		bottom := math.Max(union.Y+union.Height, region.Y+region.Height)
		union.X = math.Min(union.X, region.X)
		union.Y = math.Min(union.Y, region.Y)
		union.Width = right - union.X
		union.Height = bottom - union.Y
	}
	return union
}

func (s *Service) reviewGeometry(ctx context.Context, suggestionID string) (int, bool, error) {
	var revision int
	var superseded bool
	err := s.Pool.QueryRow(ctx, `SELECT geometry_revision, superseded_at IS NOT NULL FROM source_tree_suggestions WHERE id=$1`, suggestionID).Scan(&revision, &superseded)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, ErrSuggestionNotFound
	}
	return revision, superseded, err
}

func checkRegionFrameIdentity(input SuggestionRegionInput, frame NamePageFrame) error {
	if input.SourceChecksum != "" && input.SourceChecksum != frame.Checksum {
		return ErrConflict
	}
	if input.CoordinateSpace != "" && input.CoordinateSpace != frame.Space {
		return ErrConflict
	}
	if input.AnalysisDPI != 0 && input.AnalysisDPI != frame.DPI {
		return ErrConflict
	}
	if input.FrameWidth != 0 && input.FrameWidth != frame.Width {
		return ErrConflict
	}
	if input.FrameHeight != 0 && input.FrameHeight != frame.Height {
		return ErrConflict
	}
	return nil
}

func regionPreviewFromCrop(input SuggestionRegionInput, revision int, artifact preparedNameCrop) (SuggestionRegionPreview, error) {
	tight, err := suggestionCropBounds(input.Region, artifact.Frame.Width, artifact.Frame.Height, "tight")
	if err != nil {
		return SuggestionRegionPreview{}, err
	}
	contextBounds, err := suggestionCropBounds(input.Region, artifact.Frame.Width, artifact.Frame.Height, "context")
	if err != nil {
		return SuggestionRegionPreview{}, err
	}
	return SuggestionRegionPreview{
		GeometryRevision: revision,
		Region:           input.Region,
		CoordinateSpace:  artifact.Frame.Space,
		AnalysisDPI:      artifact.Frame.DPI,
		SourceChecksum:   artifact.Frame.Checksum,
		FrameWidth:       artifact.Frame.Width,
		FrameHeight:      artifact.Frame.Height,
		TightBounds:      tight,
		ContextBounds:    contextBounds,
		Quality:          artifact.Quality,
		TightPNG:         artifact.Crop.TightPNG,
		ContextPNG:       artifact.Crop.ContextPNG,
	}, nil
}

// PreviewSuggestionRegion renders a proposed rectangle without appending a
// revision. Cancelling a preview therefore changes no review state; only
// derived crop cache entries (already keyed by geometry) may be prepared.
func (s *Service) PreviewSuggestionRegion(ctx context.Context, suggestionID, actorID string, input SuggestionRegionInput) (SuggestionRegionPreview, error) {
	source, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false)
	if err != nil {
		return SuggestionRegionPreview{}, err
	}
	if err = validateNormalizedRegion(input.Region); err != nil {
		return SuggestionRegionPreview{}, err
	}
	revision, superseded, err := s.reviewGeometry(ctx, suggestionID)
	if err != nil {
		return SuggestionRegionPreview{}, err
	}
	if superseded {
		return SuggestionRegionPreview{}, ErrSuggestionSuperseded
	}
	if input.ExpectedGeometryRevision != revision {
		return SuggestionRegionPreview{}, ErrConflict
	}
	source.region = input.Region
	artifact, err := s.prepareNameCrop(ctx, source, nil, nil)
	if err != nil {
		return SuggestionRegionPreview{}, err
	}
	if err = checkRegionFrameIdentity(input, artifact.Frame); err != nil {
		return SuggestionRegionPreview{}, err
	}
	return regionPreviewFromCrop(input, revision, artifact)
}

// ReviseSuggestionRegion appends one immutable geometry revision and updates the
// operational rectangle. It does not touch suggested_text_ar, alternatives, or
// any decision: those are evidence of an earlier reading and stay readable.
func (s *Service) ReviseSuggestionRegion(ctx context.Context, suggestionID, actorID string, input SuggestionRegionInput) (SuggestionView, error) {
	if len(input.NoteAR) > 2000 {
		return SuggestionView{}, ErrValidation
	}
	if err := validateNormalizedRegion(input.Region); err != nil {
		return SuggestionView{}, err
	}
	source, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false)
	if err != nil {
		return SuggestionView{}, err
	}
	revision, superseded, err := s.reviewGeometry(ctx, suggestionID)
	if err != nil {
		return SuggestionView{}, err
	}
	if superseded {
		return SuggestionView{}, ErrSuggestionSuperseded
	}
	if input.ExpectedGeometryRevision != revision {
		return SuggestionView{}, ErrConflict
	}
	source.region = input.Region
	artifact, err := s.prepareNameCrop(ctx, source, nil, nil)
	if err != nil {
		return SuggestionView{}, err
	}
	if err = checkRegionFrameIdentity(input, artifact.Frame); err != nil {
		return SuggestionView{}, err
	}
	if artifact.Frame.Width < 1 || artifact.Frame.Height < 1 ||
		input.Region.Width*float64(artifact.Frame.Width) < 2 || input.Region.Height*float64(artifact.Frame.Height) < 2 {
		return SuggestionView{}, ErrValidation
	}

	parsed, err := uuid.Parse(suggestionID)
	if err != nil {
		return SuggestionView{}, ErrSuggestionNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SuggestionView{}, err
	}
	defer tx.Rollback(ctx)
	if _, _, _, _, err = s.lockReviewRow(ctx, tx, suggestionID); err != nil {
		return SuggestionView{}, err
	}
	var currentRegionJSON []byte
	var currentRevision int
	var currentStatus string
	var currentSuperseded bool
	if err = tx.QueryRow(ctx, `
		SELECT region, geometry_revision, status, superseded_at IS NOT NULL
		FROM source_tree_suggestions WHERE id=$1 FOR UPDATE
	`, parsed).Scan(&currentRegionJSON, &currentRevision, &currentStatus, &currentSuperseded); err != nil {
		return SuggestionView{}, err
	}
	if currentSuperseded {
		return SuggestionView{}, ErrSuggestionSuperseded
	}
	if currentStatus != "suggested" {
		return SuggestionView{}, ErrSuggestionAlreadyDecided
	}
	if currentRevision != input.ExpectedGeometryRevision {
		return SuggestionView{}, ErrConflict
	}
	var previous Region
	if err = json.Unmarshal(currentRegionJSON, &previous); err != nil {
		return SuggestionView{}, ErrValidation
	}
	if sameRegion(previous, input.Region) {
		return SuggestionView{}, ErrValidation
	}
	previousJSON, err := json.Marshal(previous)
	if err != nil {
		return SuggestionView{}, ErrValidation
	}
	regionJSON, err := json.Marshal(input.Region)
	if err != nil {
		return SuggestionView{}, ErrValidation
	}
	nextRevision := currentRevision + 1
	var revisionID string
	if err = tx.QueryRow(ctx, `
		INSERT INTO source_name_region_revisions (
			suggestion_id, revision, previous_region, region, coordinate_space,
			analysis_dpi, source_checksum, frame_width, frame_height, created_by
		) VALUES ($1,$2,$3::jsonb,$4::jsonb,$5,$6,$7,$8,$9,$10)
		RETURNING id::text
	`, parsed, nextRevision, previousJSON, regionJSON, artifact.Frame.Space,
		artifact.Frame.DPI, artifact.Frame.Checksum, artifact.Frame.Width, artifact.Frame.Height, actorID).Scan(&revisionID); err != nil {
		return SuggestionView{}, err
	}
	metadata, err := json.Marshal(map[string]any{
		"previousRegion":   previous,
		"region":           input.Region,
		"geometryRevision": nextRevision,
		"note":             strings.TrimSpace(input.NoteAR),
	})
	if err != nil {
		return SuggestionView{}, ErrValidation
	}
	var eventID string
	if err = tx.QueryRow(ctx, `
		INSERT INTO source_name_review_events (
			suggestion_id, actor_id, event_type, revision, geometry_revision_id, metadata
		) VALUES ($1,$2,'region_revised',$3,$4,$5::jsonb)
		RETURNING id::text
	`, parsed, actorID, currentRevision, revisionID, metadata).Scan(&eventID); err != nil {
		return SuggestionView{}, err
	}
	var updated int64
	tag, err := tx.Exec(ctx, `
		UPDATE source_tree_suggestions
		SET region=$2::jsonb, geometry_revision=$3, updated_at=now()
		WHERE id=$1 AND status='suggested' AND geometry_revision=$4 AND superseded_at IS NULL
	`, parsed, regionJSON, nextRevision, currentRevision)
	if err != nil {
		return SuggestionView{}, err
	}
	updated = tag.RowsAffected()
	if updated != 1 {
		return SuggestionView{}, ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return SuggestionView{}, err
	}
	return s.getSuggestion(ctx, parsed)
}
