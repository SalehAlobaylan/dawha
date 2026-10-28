package sourceprocessing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
)

// repairFixture uploads a real synthetic image so preview and confirmation run
// through the same source-space renderer the bench uses, without any provider.
func repairFixture(t *testing.T) (*Service, *testsupport.Fixture, string, string, string) {
	t.Helper()
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "قارئ تصحيح الهندسة")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{
		TitleAR:    "شجرة تصحيح " + fixture.Unique("s"),
		SourceType: "family_document",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	file, err := uploader.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{
		Filename:    "tree.png",
		ContentType: "image/png",
		Content:     syntheticChartPNG(t),
	})
	if err != nil {
		t.Fatalf("upload source: %v", err)
	}
	service := NewService(fixture.Pool(), store, nil, nil, NewPDFExtractor())
	return service, fixture, source.Source.ID, file.ID, owner.User.ID
}

func TestRegionRepairPreviewsAndConfirmsWithoutLosingOriginalEvidence(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := repairFixture(t)
	ctx := fixture.Ctx()
	originalRegion := Region{X: 200.0 / 600.0, Y: 150.0 / 400.0, Width: 80.0 / 600.0, Height: 40.0 / 400.0}
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{{
		OccurrenceID:    "carrier-region-1",
		PageNumber:      1,
		SuggestedTextAR: "أحمد",
		Region:          originalRegion,
		Confidence:      0.7,
		ViewCount:       3,
		ReadingMethod:   "local_carrier_localiser",
		ReadingModel:    "carrier-vision-v1",
		Alternatives:    []string{"أحمد", "محمد"},
	}}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil || len(queue) != 1 {
		t.Fatalf("list suggestions: %v (%d)", err, len(queue))
	}
	suggestion := queue[0]
	adjusted := Region{X: 190.0 / 600.0, Y: 145.0 / 400.0, Width: 100.0 / 600.0, Height: 50.0 / 400.0}

	preview, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 0,
		Region:                   adjusted,
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.FrameWidth != 600 || preview.FrameHeight != 400 || preview.CoordinateSpace != "native_raster" || preview.AnalysisDPI != 0 {
		t.Fatalf("preview frame = %+v", preview)
	}
	if len(preview.TightPNG) == 0 || len(preview.ContextPNG) == 0 {
		t.Fatal("preview returned an empty crop")
	}
	if preview.SourceChecksum == "" || len(preview.SourceChecksum) != 64 {
		t.Fatalf("preview checksum = %q", preview.SourceChecksum)
	}
	tightWidth, tightHeight := decodePNGSize(t, preview.TightPNG)
	if tightWidth < 2 || tightHeight < 2 {
		t.Fatalf("preview tight crop = %dx%d", tightWidth, tightHeight)
	}

	// Preview and cancel do not persist a geometry revision or change the queue.
	if count := fixture.Count(`SELECT COUNT(*) FROM source_name_region_revisions WHERE suggestion_id=$1`, suggestion.ID); count != 0 {
		t.Fatalf("preview persisted %d region revisions", count)
	}
	var geometryRevision int
	var storedRegionJSON []byte
	if err := fixture.QueryRow(`SELECT geometry_revision, region FROM source_tree_suggestions WHERE id=$1`, suggestion.ID).Scan(&geometryRevision, &storedRegionJSON); err != nil {
		t.Fatalf("read geometry after preview: %v", err)
	}
	if geometryRevision != 0 {
		t.Fatalf("preview advanced geometry to %d", geometryRevision)
	}

	for _, invalid := range []Region{
		{X: 0.1, Y: 0.1, Width: 0, Height: 0.1},
		{X: -0.1, Y: 0.1, Width: 0.1, Height: 0.1},
		{X: 0.9, Y: 0.1, Width: 0.2, Height: 0.1},
		{X: 0.1, Y: 0.1, Width: 0.1, Height: 1.1},
	} {
		if _, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{ExpectedGeometryRevision: 0, Region: invalid}); !errors.Is(err, ErrValidation) {
			t.Fatalf("preview invalid region %+v = %v, want ErrValidation", invalid, err)
		}
	}
	if _, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{ExpectedGeometryRevision: 1, Region: adjusted}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale preview = %v, want ErrConflict", err)
	}
	wrongChecksum := strings.Repeat("0", 64)
	if _, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 0, Region: adjusted, SourceChecksum: wrongChecksum,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("checksum mismatch preview = %v, want ErrConflict", err)
	}
	if _, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 0, Region: adjusted, CoordinateSpace: "pdf_analysis",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("coordinate-space mismatch preview = %v, want ErrConflict", err)
	}
	if _, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 0, Region: adjusted, FrameWidth: 601, FrameHeight: 401,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("frame-dimension mismatch preview = %v, want ErrConflict", err)
	}
	if _, err := service.ReviseSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 0, Region: adjusted, SourceChecksum: wrongChecksum,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("checksum mismatch confirm = %v, want ErrConflict", err)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_name_region_revisions WHERE suggestion_id=$1`, suggestion.ID); count != 0 {
		t.Fatalf("a rejected confirm persisted %d region revisions", count)
	}

	confirmed, err := service.ReviseSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 0,
		Region:                   adjusted,
		CoordinateSpace:          preview.CoordinateSpace,
		AnalysisDPI:              preview.AnalysisDPI,
		SourceChecksum:           preview.SourceChecksum,
		FrameWidth:               preview.FrameWidth,
		FrameHeight:              preview.FrameHeight,
		NoteAR:                   "ضبط الحد الأيمن",
	})
	if err != nil {
		t.Fatalf("confirm region: %v", err)
	}
	if confirmed.GeometryRevision != 1 || sameRegion(confirmed.Region, originalRegion) {
		t.Fatalf("confirmed region = %+v", confirmed)
	}
	if confirmed.SuggestedTextAR != "أحمد" || len(confirmed.Alternatives) != 2 {
		t.Fatalf("geometry repair rewrote the reading: %+v", confirmed)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_name_region_revisions WHERE suggestion_id=$1`, suggestion.ID); count != 1 {
		t.Fatalf("region revisions = %d, want 1", count)
	}
	var previousRegion, revisionRegion []byte
	var checksum string
	var frameWidth, frameHeight int
	if err := fixture.QueryRow(`
		SELECT previous_region, region, source_checksum, frame_width, frame_height
		FROM source_name_region_revisions WHERE suggestion_id=$1 AND revision=1
	`, suggestion.ID).Scan(&previousRegion, &revisionRegion, &checksum, &frameWidth, &frameHeight); err != nil {
		t.Fatalf("read region revision: %v", err)
	}
	var storedPrevious, storedRevision Region
	if err := json.Unmarshal(previousRegion, &storedPrevious); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(revisionRegion, &storedRevision); err != nil {
		t.Fatal(err)
	}
	if !sameRegion(storedPrevious, originalRegion) || !sameRegion(storedRevision, adjusted) {
		t.Fatalf("region revision previous=%+v region=%+v", storedPrevious, storedRevision)
	}
	if checksum != preview.SourceChecksum || frameWidth != 600 || frameHeight != 400 {
		t.Fatalf("region revision identity = %q %dx%d", checksum, frameWidth, frameHeight)
	}

	// A later reading run must not overwrite a repaired geometry or the reading
	// the reviewer is working from. The repair is immutable evidence, and the
	// original machine reading stays beside it.
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{{
		OccurrenceID:    suggestion.OccurrenceID,
		PageNumber:      1,
		SuggestedTextAR: "متغير",
		Region:          Region{X: 0.05, Y: 0.05, Width: 0.02, Height: 0.02},
		Confidence:      0.1,
		ReadingMethod:   "rerun",
		ReadingModel:    "rerun",
	}}); err != nil {
		t.Fatalf("rerun after repair: %v", err)
	}
	unchanged, err := service.getSuggestion(ctx, mustUUID(suggestion.ID))
	if err != nil {
		t.Fatalf("read repaired suggestion: %v", err)
	}
	if unchanged.SuggestedTextAR != "أحمد" || unchanged.GeometryRevision != 1 || !sameRegion(unchanged.Region, adjusted) || unchanged.ReadingMethod != "local_carrier_localiser" {
		t.Fatalf("a rerun overwrote repaired evidence: %+v", unchanged)
	}

	history, err := service.SuggestionHistory(ctx, suggestion.ID, ownerID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !sameRegion(history.OriginalRegion, originalRegion) || len(history.RegionRevisions) != 1 {
		t.Fatalf("history original=%+v revisions=%+v", history.OriginalRegion, history.RegionRevisions)
	}
	if len(history.Events) != 1 || history.Events[0].Type != "region_revised" || history.Events[0].GeometryRevisionID == "" {
		t.Fatalf("region event = %+v", history.Events)
	}

	// A stale confirmation cannot append a second revision; a fresh one can.
	if _, err := service.ReviseSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{ExpectedGeometryRevision: 0, Region: adjusted}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale confirm = %v, want ErrConflict", err)
	}
	secondPreview, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 1,
		Region:                   Region{X: 195.0 / 600.0, Y: 148.0 / 400.0, Width: 95.0 / 600.0, Height: 45.0 / 400.0},
	})
	if err != nil {
		t.Fatalf("preview after confirmation: %v", err)
	}
	if secondPreview.GeometryRevision != 1 {
		t.Fatalf("second preview geometry = %d", secondPreview.GeometryRevision)
	}
	// A new geometry gets its own derived crop artifacts; the previous crop is
	// not deleted or reused as though it showed the new rectangle.
	if count := fixture.Count(`SELECT COUNT(*) FROM source_name_crop_artifacts WHERE source_file_id=$1`, mustUUID(fileID)); count < 2 {
		t.Fatalf("geometry repair did not create a distinct crop artifact: %d", count)
	}

	// A rectangle smaller than two source pixels is refused instead of being
	// silently rounded up to pixels the reviewer did not choose.
	if _, err := service.ReviseSuggestionRegion(ctx, suggestion.ID, ownerID, SuggestionRegionInput{
		ExpectedGeometryRevision: 1,
		Region:                   Region{X: 0.5, Y: 0.5, Width: 1.0 / 600.0, Height: 40.0 / 400.0},
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("sub-pixel region = %v, want ErrValidation", err)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_name_region_revisions WHERE suggestion_id=$1`, suggestion.ID); count != 1 {
		t.Fatalf("rejected revisions persisted: %d", count)
	}

	other := actor.RegisterAndLogin(t, fixture, "مراجع بلا وصول هندسي")
	if _, err := service.PreviewSuggestionRegion(ctx, suggestion.ID, other.User.ID, SuggestionRegionInput{ExpectedGeometryRevision: 1, Region: adjusted}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("preview by unauthorized actor = %v, want ErrForbidden", err)
	}
	if _, err := service.ReviseSuggestionRegion(ctx, suggestion.ID, other.User.ID, SuggestionRegionInput{ExpectedGeometryRevision: 1, Region: adjusted}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("confirm by unauthorized actor = %v, want ErrForbidden", err)
	}
}

func TestSplitAndMergeKeepLineageAndRetireParentsFromTheQueue(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := repairFixture(t)
	ctx := fixture.Ctx()
	parentRegion := Region{X: 0.2, Y: 0.2, Width: 0.4, Height: 0.4}
	if err := service.CreateSuggestions(ctx, mustUUID(sourceID), mustUUID(fileID), nil, []SuggestionInput{{
		OccurrenceID:    "carrier-split-parent",
		PageNumber:      1,
		SuggestedTextAR: "أحمد",
		Region:          parentRegion,
		Confidence:      0.8,
		ViewCount:       2,
		ReadingMethod:   "local_carrier_localiser",
		ReadingModel:    "carrier-vision-v1",
	}}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	queue, err := service.ListSuggestions(ctx, fileID, ownerID, "", 0, 0)
	if err != nil || len(queue) != 1 {
		t.Fatalf("list suggestions: %v (%d)", err, len(queue))
	}
	parentID := queue[0].ID
	children := []Region{
		{X: 0.22, Y: 0.22, Width: 0.18, Height: 0.35},
		{X: 0.42, Y: 0.22, Width: 0.16, Height: 0.35},
	}
	if _, err := service.SplitSuggestion(ctx, parentID, ownerID, SuggestionSplitInput{
		ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0,
		Regions: []Region{{X: 0.22, Y: 0.22, Width: 0.2, Height: 0.35}, {X: 0.32, Y: 0.22, Width: 0.1, Height: 0.35}},
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("overlapping split = %v, want ErrValidation", err)
	}
	if _, err := service.SplitSuggestion(ctx, parentID, ownerID, SuggestionSplitInput{
		ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0,
		Regions: []Region{{X: 0.22, Y: 0.22, Width: 0.18, Height: 0.35}, {X: 0.6, Y: 0.22, Width: 0.1, Height: 0.35}},
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("outside split = %v, want ErrValidation", err)
	}
	if _, err := service.SplitSuggestion(ctx, parentID, ownerID, SuggestionSplitInput{
		ExpectedReviewRevision: 1, ExpectedGeometryRevision: 0, Regions: children,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale split = %v, want ErrConflict", err)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_tree_suggestions WHERE source_file_id=$1`, mustUUID(fileID)); count != 1 {
		t.Fatalf("rejected splits created %d extra occurrences", count-1)
	}

	split, err := service.SplitSuggestion(ctx, parentID, ownerID, SuggestionSplitInput{
		ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0, Regions: children, NoteAR: "موضعان منفصلان",
	})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if !split.Parent.Superseded || len(split.Children) != 2 {
		t.Fatalf("split result = %+v", split)
	}
	for _, child := range split.Children {
		if child.Status != "suggested" || child.Superseded || child.SuggestedTextAR != "" {
			t.Fatalf("split child = %+v", child)
		}
		if child.ReadingMethod != "local_carrier_localiser" || child.ReadingModel != "carrier-vision-v1" {
			t.Fatalf("split child lost reader metadata: %+v", child)
		}
		if child.OccurrenceID == split.Parent.OccurrenceID || !strings.HasPrefix(child.OccurrenceID, "split-") {
			t.Fatalf("split child identity = %q", child.OccurrenceID)
		}
	}

	pending, err := service.ListSuggestions(ctx, fileID, ownerID, "suggested", 0, 0)
	if err != nil || len(pending) != 2 {
		t.Fatalf("pending after split: %v (%d)", err, len(pending))
	}
	for _, view := range pending {
		if view.ID == parentID {
			t.Fatal("a superseded parent remained in the pending queue")
		}
	}
	historyQueue, err := service.ListSuggestions(ctx, fileID, ownerID, "history", 0, 0)
	if err != nil || len(historyQueue) != 1 || historyQueue[0].ID != parentID {
		t.Fatalf("history queue after split: %v (%+v)", err, historyQueue)
	}
	parentHistory, err := service.SuggestionHistory(ctx, parentID, ownerID)
	if err != nil {
		t.Fatalf("parent history: %v", err)
	}
	if len(parentHistory.Lineage) != 2 || parentHistory.CanReopen || parentHistory.CanSplit || !parentHistory.Suggestion.Superseded {
		t.Fatalf("parent history = %+v", parentHistory)
	}
	if _, err := service.DecideSuggestion(ctx, parentID, ownerID, SuggestionDecision{Decision: SuggestionAccepted}); !errors.Is(err, ErrSuggestionSuperseded) {
		t.Fatalf("decide superseded parent = %v, want ErrSuggestionSuperseded", err)
	}
	if _, err := service.SplitSuggestion(ctx, parentID, ownerID, SuggestionSplitInput{
		ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0, Regions: children,
	}); !errors.Is(err, ErrSuggestionSuperseded) {
		t.Fatalf("second split of parent = %v, want ErrSuggestionSuperseded", err)
	}

	union := regionUnion(children)
	shrunk := union
	shrunk.Width -= 0.05
	_, err = service.MergeSuggestions(ctx, split.Children[0].ID, ownerID, SuggestionMergeInput{
		Members: []SuggestionMergeMember{
			{ID: split.Children[0].ID, ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0},
			{ID: split.Children[1].ID, ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0},
		},
		Region: shrunk,
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("merge region missing a member = %v, want ErrValidation", err)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_tree_suggestions WHERE source_file_id=$1`, mustUUID(fileID)); count != 3 {
		t.Fatalf("rejected merge created occurrences: %d", count)
	}

	merged, err := service.MergeSuggestions(ctx, split.Children[0].ID, ownerID, SuggestionMergeInput{
		Members: []SuggestionMergeMember{
			{ID: split.Children[0].ID, ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0},
			{ID: split.Children[1].ID, ExpectedReviewRevision: 0, ExpectedGeometryRevision: 0},
		},
		Region: union,
		NoteAR: "موضع واحد",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(merged.Superseded) != 2 || merged.Merged.Superseded || merged.Merged.SuggestedTextAR != "" {
		t.Fatalf("merge result = %+v", merged)
	}
	if !strings.HasPrefix(merged.Merged.OccurrenceID, "merge-") {
		t.Fatalf("merged identity = %q", merged.Merged.OccurrenceID)
	}
	pending, err = service.ListSuggestions(ctx, fileID, ownerID, "suggested", 0, 0)
	if err != nil || len(pending) != 1 || pending[0].ID != merged.Merged.ID {
		t.Fatalf("pending after merge: %v (%+v)", err, pending)
	}
	if count := fixture.Count(`SELECT COUNT(*) FROM source_name_occurrence_lineage WHERE child_id=$1 AND kind='merge'`, merged.Merged.ID); count != 2 {
		t.Fatalf("merge lineage edges = %d, want 2", count)
	}
	childHistory, err := service.SuggestionHistory(ctx, split.Children[0].ID, ownerID)
	if err != nil {
		t.Fatalf("child history: %v", err)
	}
	if len(childHistory.Lineage) != 2 || !childHistory.Suggestion.Superseded {
		t.Fatalf("child lineage = %+v", childHistory)
	}
	for _, view := range append(merged.Superseded, merged.Merged) {
		if view.SuggestedTextAR != "" {
			t.Fatalf("geometry repair fabricated a reading: %+v", view)
		}
	}
}
