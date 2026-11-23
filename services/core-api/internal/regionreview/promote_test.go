package regionreview

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// TestPromoteCandidateIsOnceOnly pins the 030-E promotion lane's own rules:
// the accepted candidate promotes ONCE, its provenance rides BOTH sides of
// the join (the region knows its candidate; the candidate names its region),
// and the second call returns the same answer rather than a second row.
func TestPromoteCandidateIsOnceOnly(t *testing.T) {
	fixture := testsupport.New(t)
	reviewer := actor.Register(t, fixture, "مراجِع الترقية")
	secondReviewer := actor.Register(t, fixture, "المراجع المستقل")
	fixture.GrantRole(secondReviewer.User.ID, "moderator")
	fixture.GrantRole(reviewer.User.ID, "moderator")

	// The fixture's accepted candidate: an accepted reading over a real row.
	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة الترقية', 'archive_record', 'public', $2::jsonb, $3)`,
		sourceID, promotePlateMetadata("promote-plate"), reviewer.User.ID)
	candidateID := uuid.New()
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
		VALUES ($1, 'sources/plates/promote-1.jpg', 'promote-1.jpg', 'image/jpeg', 100, 'chk-promote-1')`, sourceID)
	fixture.Exec(`INSERT INTO external_region_candidates
		(id, source_id, plate_key, occurrence_id, status, label_text_ar, label_read_by, crop_px)
		VALUES ($1, $2, 'promote-plate', 'occ-1', 'accepted', 'بلاد الشَّرِيك', $3,
		'{"x":10,"y":10,"w":40,"h":30}'::jsonb)`,
		candidateID, sourceID, reviewer.User.ID)

	service := NewService(fixture.Pool(), nil)
	if _, err := service.PromoteCandidate(fixture.Ctx(), reviewer.User.ID, candidateID.String()); err != ErrValidation {
		t.Fatalf("a printed name alone must not promote: %v", err)
	}
	input := BoundaryReviewInput{SourceID: sourceID.String(), Locator: "ص 42", ValidFrom: "1900-01-01", ValidTo: "1910-12-31", WKT: "POLYGON((40 20,41 20,41 21,40 21,40 20))"}
	if err := service.ReviewBoundary(fixture.Ctx(), reviewer.User.ID, candidateID.String(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PromoteCandidate(fixture.Ctx(), reviewer.User.ID, candidateID.String()); err != ErrValidation {
		t.Fatalf("one boundary reviewer must not promote: %v", err)
	}
	mismatch := input
	mismatch.ValidTo = "1911-12-31"
	if err := service.ReviewBoundary(fixture.Ctx(), secondReviewer.User.ID, candidateID.String(), mismatch); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PromoteCandidate(fixture.Ctx(), reviewer.User.ID, candidateID.String()); err != ErrValidation {
		t.Fatalf("mismatched dates must not promote: %v", err)
	}
	if err := service.ReviewBoundary(fixture.Ctx(), secondReviewer.User.ID, candidateID.String(), input); err != nil {
		t.Fatal(err)
	}

	first, err := service.PromoteCandidate(fixture.Ctx(), reviewer.User.ID, candidateID.String())
	if err != nil {
		t.Fatalf("first promote: %v", err)
	}
	if !first.Promoted || first.RegionID == "" {
		t.Fatalf("first promote carried no region: %+v", first)
	}

	var from, to, locator string
	err = fixture.Pool().QueryRow(fixture.Ctx(), `SELECT valid_from::text, valid_to::text, boundary_locator_ar
		FROM historical_regions WHERE id=$1`, first.RegionID).Scan(&from, &to, &locator)
	if err != nil || from != input.ValidFrom || to != input.ValidTo || locator != input.Locator {
		t.Fatalf("boundary provenance: %q %q %q %v", from, to, locator, err)
	}

	// The second promote returns the SAME region — the once-only rule, named
	// in the response rather than silenced.
	second, err := service.PromoteCandidate(fixture.Ctx(), reviewer.User.ID, candidateID.String())
	if err != nil {
		t.Fatalf("second promote: %v", err)
	}
	if second.RegionID != first.RegionID {
		t.Fatalf("second promote wrote a duplicate: %s vs %s", first.RegionID, second.RegionID)
	}
}

// TestPromoteCandidateGuardsTheUnreadRow pins the reading floor: a candidate
// without a reviewer's reading refuses, because promoting an unread row
// would be the precision the plate never vouched for.
func TestPromoteCandidateGuardsTheUnreadRow(t *testing.T) {
	fixture := testsupport.New(t)
	reviewer := actor.Register(t, fixture, "مراجِع الترقية الثاني")
	fixture.GrantRole(reviewer.User.ID, "moderator")

	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, metadata, created_by)
		VALUES ($1, 'لوحة الترقية', 'archive_record', $2::jsonb, $3)`,
		sourceID, promotePlateMetadata("promote-plate-unread"), reviewer.User.ID)
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
		VALUES ($1, 'sources/plates/promote-unread.jpg', 'promote-unread.jpg', 'image/jpeg', 100, 'chk-unread')`, sourceID)
	candidateID := uuid.New()
	fixture.Exec(`INSERT INTO external_region_candidates
		(id, source_id, plate_key, occurrence_id, status, label_text_ar, crop_px)
		VALUES ($1, $2, 'promote-plate-unread', 'occ-unread', 'accepted', '', '{"x":5,"y":5,"w":20,"h":20}'::jsonb)`,
		candidateID, sourceID)

	_, err := NewService(fixture.Pool(), nil).PromoteCandidate(fixture.Ctx(), reviewer.User.ID, candidateID.String())
	if err != ErrValidation {
		t.Fatalf("unread promote = %v, want ErrValidation", err)
	}
}

// promotePlateMetadata is the plate-side metadata every promotion test needs:
// the graticule model + the honest certainty block the 030-D artifact keeps.
func promotePlateMetadata(plateKey string) string {
	return `{"plate_key": "` + plateKey + `", "georef": {"method": "graticule_labels", "certainty": "approximate", "needsReview": false, "reviewReasons": [], "model": {"kind": "equidistant_conic", "apex": {"x": 400, "y": -1400}, "coneConstant": 0.5, "centralLongitude": 45, "pxPerDegLat": 100, "latitude": {"coef": [-0.01, 30]}}, "frame": {"outer_left": 0, "inner_left": 5, "inner_top": 5, "outer_right": 320, "outer_bottom": 240, "inner_right": 315, "inner_bottom": 235}}}`
}
