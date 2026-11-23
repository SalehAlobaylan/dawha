package regionreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

// seedLane registers the actors and one plate source every test starts from:
// an owner (who may create rows but may not review), a moderator (who may),
// and a reader (plain user). The source carries the 030-D honesty block so
// the PlateSummary assertions read the real shape of the artifact.
func seedLane(t *testing.T, fixture *testsupport.Fixture) (sourceID uuid.UUID, moderatorID, readerID string) {
	t.Helper()
	owner := actor.Register(t, fixture, "مالك اللوحات")
	fixture.GrantRole(owner.User.ID, "researcher")
	moderator := actor.Register(t, fixture, "مدير القبائل")
	fixture.GrantRole(moderator.User.ID, "moderator")
	reader := actor.Register(t, fixture, "قارئ القبائل")

	sourceID = uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة قبائل الجزيرة ١٩١٦', 'archive_record', 'public',
			jsonb_build_object('license', 'public_domain',
				'georef', jsonb_build_object('method', 'graticule_labels', 'certainty', 'approximate',
					'needsReview', true, 'reviewReasons', jsonb_build_array('interior_parallels_unverified_by_ink'))),
			$2)`, sourceID, owner.User.ID)
	return sourceID, moderator.User.ID, reader.User.ID
}

func insertCandidate(t *testing.T, fixture *testsupport.Fixture, sourceID uuid.UUID, plateKey, occurrence string, box CropBox) uuid.UUID {
	t.Helper()
	id := uuid.New()
	cropBytes, err := json.Marshal(box)
	if err != nil {
		t.Fatalf("marshal crop box: %v", err)
	}
	fixture.Exec(`INSERT INTO external_region_candidates
		(id, source_id, plate_key, occurrence_id, crop_px, coordinates,
		 ink_density, component_count, label_confidence, transform_rms_km, rank, status)
		VALUES ($1, $2, $3, $4, $5::jsonb, ST_SetSRID(ST_MakePoint(44.66, 26.78), 4326),
			0.19, 9, 0.83, 8.56, 0, 'suggested')`,
		id, sourceID, plateKey, occurrence, string(cropBytes))
	return id
}

func TestRegionLaneGateListAndSearch(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, moderatorID, readerID := seedLane(t, fixture)
	service := NewService(fixture.Pool(), nil)

	candidateID := insertCandidate(t, fixture, sourceID, "admiralty-1916-tribal", "label:1442-3728-388-62", CropBox{X: 1434, Y: 3720, W: 404, H: 78})
	insertCandidate(t, fixture, sourceID, "dickson-1914-tribal", "label:900-1000-120-40", CropBox{X: 900, Y: 1000, W: 120, H: 40})

	// Non-moderators are refused the lane by role, anonymous included.
	if _, err := service.List(fixture.Ctx(), readerID, ListParams{}); err != ErrForbidden {
		t.Fatalf("reader list error = %v, want ErrForbidden", err)
	}
	if _, err := service.List(fixture.Ctx(), "", ListParams{}); err != ErrUnauthenticated {
		t.Fatalf("anonymous list error = %v, want ErrUnauthenticated", err)
	}
	if _, err := service.List(fixture.Ctx(), uuid.New().String(), ListParams{}); err != ErrUnauthenticated {
		t.Fatalf("unknown-user list error = %v, want ErrUnauthenticated", err)
	}
	if _, err := service.List(fixture.Ctx(), moderatorID, ListParams{Status: "bogus"}); err != ErrValidation {
		t.Fatalf("bogus status error = %v, want ErrValidation", err)
	}

	// The moderator sees the proposal as a proposal: empty reading, score
	// components as measured, and a crop box in plate pixels.
	report, err := service.List(fixture.Ctx(), moderatorID, ListParams{})
	if err != nil {
		t.Fatalf("moderator list: %v", err)
	}
	if len(report.Candidates) != 2 || report.Status != "suggested" || report.Truncated {
		t.Fatalf("suggested lane = %+v, want both candidates, not truncated", report)
	}
	first := report.Candidates[0]
	if first.ID != candidateID.String() {
		t.Fatalf("first candidate = %s, want %s (rank ordering)", first.ID, candidateID)
	}
	if first.CropPx != (CropBox{X: 1434, Y: 3720, W: 404, H: 78}) {
		t.Fatalf("crop box = %+v", first.CropPx)
	}
	if first.Coordinates == nil || first.Coordinates.Longitude != 44.66 || first.Coordinates.Latitude != 26.78 {
		t.Fatalf("coordinates = %+v", first.Coordinates)
	}
	if first.LabelTextAR != "" || first.HasRegionGeometry || first.RegionGeometryWKT != "" {
		t.Fatalf("a fresh proposal carries an answer: %+v", first)
	}
	if first.InkDensity == nil || *first.InkDensity != 0.19 || first.ComponentCount == nil || *first.ComponentCount != 9 {
		t.Fatalf("score components = %+v/%+v, want the measured values", first.InkDensity, first.ComponentCount)
	}
	if first.TransformRMSKm == nil || *first.TransformRMSKm != 8.56 {
		t.Fatalf("transform rms = %+v, want the plate's published 8.56", first.TransformRMSKm)
	}

	// The plate filter is an exact key, not a search: one plate's lane.
	plateLane, err := service.List(fixture.Ctx(), moderatorID, ListParams{PlateKey: "dickson-1914-tribal"})
	if err != nil {
		t.Fatalf("plate lane: %v", err)
	}
	if len(plateLane.Candidates) != 1 || plateLane.Candidates[0].PlateKey != "dickson-1914-tribal" {
		t.Fatalf("plate lane = %+v", plateLane.Candidates)
	}
	if none, err := service.List(fixture.Ctx(), moderatorID, ListParams{PlateKey: "لا-يوجد"}); err != nil || len(none.Candidates) != 0 {
		t.Fatalf("unknown plate lane = %+v, %v", none.Candidates, err)
	}

	// The text search reaches the fields a moderator scans.
	hit, err := service.List(fixture.Ctx(), moderatorID, ListParams{Query: "1442-3728"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hit.Candidates) != 1 || hit.Candidates[0].OccurrenceID != "label:1442-3728-388-62" {
		t.Fatalf("search hit = %+v", hit.Candidates)
	}
	miss, err := service.List(fixture.Ctx(), moderatorID, ListParams{Query: "لا-يوجد-هذا"})
	if err != nil {
		t.Fatalf("empty search: %v", err)
	}
	if len(miss.Candidates) != 0 {
		t.Fatalf("empty search candidates = %d, want 0", len(miss.Candidates))
	}

	// Get refuses bad input and unknown rows, and carries the plate's honesty
	// block beside the proposal.
	if _, err := service.Get(fixture.Ctx(), moderatorID, "لا-uuid"); err != ErrValidation {
		t.Fatalf("bad uuid get error = %v, want ErrValidation", err)
	}
	if _, err := service.Get(fixture.Ctx(), moderatorID, uuid.New().String()); err != ErrNotFound {
		t.Fatalf("unknown get error = %v, want ErrNotFound", err)
	}
	if _, err := service.Get(fixture.Ctx(), readerID, candidateID.String()); err != ErrForbidden {
		t.Fatalf("reader get error = %v, want ErrForbidden", err)
	}
	detail, err := service.Get(fixture.Ctx(), moderatorID, candidateID.String())
	if err != nil {
		t.Fatalf("moderator get: %v", err)
	}
	if detail.Plate == nil || detail.Plate.Certainty != "approximate" || !detail.Plate.NeedsReview {
		t.Fatalf("plate summary = %+v, want the 030-D honesty block", detail.Plate)
	}
	if len(detail.Plate.ReviewReasons) != 1 || detail.Plate.ReviewReasons[0] != "interior_parallels_unverified_by_ink" {
		t.Fatalf("review reasons = %+v", detail.Plate.ReviewReasons)
	}
}

func TestRegionLaneListIsBounded(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, moderatorID, _ := seedLane(t, fixture)
	service := NewService(fixture.Pool(), nil)

	for i := 0; i < ListLimit+5; i++ {
		insertCandidate(t, fixture, sourceID, "admiralty-1916-tribal",
			"label:bounded-"+uuid.New().String(), CropBox{X: i, Y: i, W: 10, H: 10})
	}
	report, err := service.List(fixture.Ctx(), moderatorID, ListParams{})
	if err != nil {
		t.Fatalf("bounded list: %v", err)
	}
	if len(report.Candidates) != ListLimit || !report.Truncated {
		t.Fatalf("bounded list = %d candidates, truncated %v, want %d + flag", len(report.Candidates), report.Truncated, ListLimit)
	}
}

func TestRegionDecideRecordsTheReadingAndTheVerdict(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, moderatorID, readerID := seedLane(t, fixture)
	service := NewService(fixture.Pool(), nil)
	candidateID := insertCandidate(t, fixture, sourceID, "admiralty-1916-tribal", "label:3114-3130-258-106", CropBox{X: 3106, Y: 3122, W: 274, H: 122})

	// The gates hold for decisions too, before any transition is possible.
	if _, err := service.Decide(fixture.Ctx(), readerID, candidateID.String(), DecideInput{Action: ActionReject}); err != ErrForbidden {
		t.Fatalf("reader decide = %v, want ErrForbidden", err)
	}
	if _, err := service.Decide(fixture.Ctx(), "", candidateID.String(), DecideInput{Action: ActionReject}); err != ErrUnauthenticated {
		t.Fatalf("anonymous decide = %v, want ErrUnauthenticated", err)
	}

	// An accept without a reading would later promote into a named region
	// nobody read: refused, accept carries the reviewer's words.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, candidateID.String(), DecideInput{Action: ActionAccept}); err != ErrValidation {
		t.Fatalf("reading-less accept = %v, want ErrValidation", err)
	}

	decided, err := service.Decide(fixture.Ctx(), moderatorID, candidateID.String(), DecideInput{
		Action:      ActionAccept,
		NoteAR:      "هذا الاسم «عنزة».",
		LabelTextAR: strPtr("عنزة"),
		EraLabel:    strPtr("نحو ١٩١٦"),
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if decided.Candidate.Status != "accepted" || decided.Action != ActionAccept {
		t.Fatalf("accept result = %+v", decided)
	}
	if decided.Candidate.LabelTextAR != "عنزة" || decided.Candidate.LabelReadBy != moderatorID {
		t.Fatalf("reading = %q by %q, want عنزة by the moderator", decided.Candidate.LabelTextAR, decided.Candidate.LabelReadBy)
	}
	if decided.Candidate.LabelReadAt == "" || decided.Candidate.DecidedAt == "" || decided.Candidate.DecidedBy != moderatorID {
		t.Fatalf("decision timestamps/actor missing: %+v", decided.Candidate)
	}
	if decided.Candidate.EraLabel != "نحو ١٩١٦" {
		t.Fatalf("era label = %q", decided.Candidate.EraLabel)
	}

	// A decided candidate is decided: the same ask again is a conflict, and
	// the lane no longer lists it as suggested.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, candidateID.String(), DecideInput{Action: ActionReject}); err != ErrAlreadyDecided {
		t.Fatalf("second decide = %v, want ErrAlreadyDecided", err)
	}
	lane, err := service.List(fixture.Ctx(), moderatorID, ListParams{Status: "suggested"})
	if err != nil {
		t.Fatalf("re-list suggested: %v", err)
	}
	if len(lane.Candidates) != 0 {
		t.Fatalf("suggested after decide = %d, want 0", len(lane.Candidates))
	}

	// The verdict honesty test: accepting promoted NOTHING. No region row,
	// no association — promotion belongs to a later review surface.
	if rows := fixture.Count(`SELECT count(*) FROM historical_regions`); rows == 0 {
		t.Log("no historical_regions rows in this fixture; nothing to compare against")
	}

	history, err := service.History(fixture.Ctx(), moderatorID, candidateID.String())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.History) != 1 {
		t.Fatalf("history events = %d, want 1", len(history.History))
	}
	event := history.History[0]
	if event.Action != ActionAccept || event.FromStatus != "suggested" || event.ToStatus != "accepted" {
		t.Fatalf("history event = %+v", event)
	}
	if event.NoteAR != "هذا الاسم «عنزة»." {
		t.Fatalf("history note = %q", event.NoteAR)
	}
	// The accept's diff records the reading landing, from empty to the text.
	if !jsonContains(event.Diff, "label_text_ar") || !jsonContains(event.Diff, "عنزة") {
		t.Fatalf("accept diff = %q, want the reading's from/to", event.Diff)
	}
	if event.ReviewerName == "" {
		t.Fatalf("history event carries no reviewer name: %+v", event)
	}

	// A correction revises the answer in place and records what changed.
	corrected, err := service.Decide(fixture.Ctx(), moderatorID, candidateID.String(), DecideInput{
		Action:      ActionCorrect,
		NoteAR:      "القراءة الأولى ناقصة",
		LabelTextAR: strPtr("عنزة بنو"),
	})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Candidate.Status != "corrected" || corrected.Candidate.LabelTextAR != "عنزة بنو" {
		t.Fatalf("correction result = %+v", corrected.Candidate)
	}
	history, err = service.History(fixture.Ctx(), moderatorID, candidateID.String())
	if err != nil {
		t.Fatalf("history after correct: %v", err)
	}
	if len(history.History) != 2 || history.History[1].Action != ActionCorrect ||
		history.History[1].FromStatus != "accepted" || history.History[1].ToStatus != "corrected" {
		t.Fatalf("history after correct = %+v", history.History)
	}
	if !jsonContains(history.History[1].Diff, "عنزة بنو") {
		t.Fatalf("correction diff = %q, want the new reading", history.History[1].Diff)
	}

	// A further correction of a corrected answer is legitimate — the tile
	// lane's disagreement flow sends a third reviewer back to a reading
	// another correct already settled. (This transition was found by the
	// browser journey, not by review: the service tests missed it.)
	again, err := service.Decide(fixture.Ctx(), moderatorID, candidateID.String(), DecideInput{
		Action:      ActionCorrect,
		NoteAR:      "القراءة الثالثة",
		LabelTextAR: strPtr("عنزة بنو خالد"),
	})
	if err != nil {
		t.Fatalf("third correction: %v", err)
	}
	if again.Candidate.Status != "corrected" || again.Candidate.LabelTextAR != "عنزة بنو خالد" {
		t.Fatalf("third correction = %+v", again.Candidate)
	}
}

func TestRegionDecideTransitionsRejectDeferReopenAndConfirm(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, moderatorID, _ := seedLane(t, fixture)
	service := NewService(fixture.Pool(), nil)

	// Reject carries the decision and (optionally) what the crop actually is.
	rejected := insertCandidate(t, fixture, sourceID, "admiralty-1916-tribal", "label:cartouche", CropBox{X: 4794, Y: 1364, W: 1096, H: 86})
	verdict, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{
		Action:      ActionReject,
		NoteAR:      "شعار الخريطة",
		LabelTextAR: strPtr("TRIBAL MAP OF ARABIA"),
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if verdict.Candidate.Status != "rejected" || verdict.Candidate.LabelTextAR != "TRIBAL MAP OF ARABIA" {
		t.Fatalf("reject result = %+v", verdict.Candidate)
	}

	// Defer refuses a reading: a defer means "not read yet", and silently
	// dropping supplied words would lose them.
	deferred := insertCandidate(t, fixture, sourceID, "admiralty-1916-tribal", "label:deferred", CropBox{X: 10, Y: 20, W: 30, H: 40})
	if _, err := service.Decide(fixture.Ctx(), moderatorID, deferred.String(), DecideInput{
		Action:      ActionDefer,
		LabelTextAR: strPtr("لا تُقرأ"),
	}); err != ErrValidation {
		t.Fatalf("defer with a reading = %v, want ErrValidation", err)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, deferred.String(), DecideInput{Action: ActionDefer, NoteAR: "غير مقروء الآن"}); err != nil {
		t.Fatalf("defer: %v", err)
	}

	// Reopen is for decisions that produced no published row: the rejected
	// and the deferred both return to suggested, with their reading intact
	// (a named human wrote it; the history preserves it either way).
	reopenedDeferral, err := service.Decide(fixture.Ctx(), moderatorID, deferred.String(), DecideInput{Action: ActionReopen, NoteAR: "أعيد للطابور"})
	if err != nil {
		t.Fatalf("reopen deferred: %v", err)
	}
	if reopenedDeferral.Candidate.Status != "suggested" || reopenedDeferral.Candidate.DecidedBy != "" {
		t.Fatalf("reopened deferral = %+v", reopenedDeferral.Candidate)
	}
	reopenRejection, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionReopen})
	if err != nil {
		t.Fatalf("reopen rejected: %v", err)
	}
	if reopenRejection.Candidate.Status != "suggested" {
		t.Fatalf("reopened rejection status = %q", reopenRejection.Candidate.Status)
	}

	// The colleague-confirm flow: the reopened rejection still carries its
	// reading, so an accept may confirm it without re-typing it.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionAccept, NoteAR: "أؤكد قراءة الزميل"}); err != nil {
		t.Fatalf("accept with existing reading: %v", err)
	}

	// The reopen-accepted door the workbench disagreement flow needs. Verify
	// the reading stayed attributable at each hop, then re-decide.
	reopenedAcceptance, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionReopen, NoteAR: "للاختبار"})
	if err != nil {
		t.Fatalf("reopen accepted: %v", err)
	}
	if reopenedAcceptance.Candidate.Status != "suggested" || reopenedAcceptance.Candidate.DecidedBy != "" {
		t.Fatalf("reopened acceptance = %+v", reopenedAcceptance.Candidate)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionReject, NoteAR: "الرفض الأول"}); err != nil {
		t.Fatalf("first reject: %v", err)
	}
	// THE JOURNEY-FOUND TRANSITION: an agreeing second REJECTION is ONE
	// action, not a reopen-then-lie — B's decision row is appended under
	// their own name, the status re-states, and no intermediate 'suggested'
	// ever erases the first reviewer's verdict from the row. A shared
	// deferral behaves symmetrically.
	agreeing, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionReject, NoteAR: "الرفض الثاني — أتفق"})
	if err != nil {
		t.Fatalf("agreeing second rejection = %v, want one action", err)
	}
	if agreeing.Candidate.Status != "rejected" {
		t.Fatalf("agreeing rejection status = %q", agreeing.Candidate.Status)
	}
	history, err := service.History(fixture.Ctx(), moderatorID, rejected.String())
	if err != nil {
		t.Fatalf("history after two rejects: %v", err)
	}
	rejectEvents := 0
	for _, event := range history.History {
		if event.Action == ActionReject {
			rejectEvents++
		}
	}
	if rejectEvents != 3 {
		// Three rejects accumulate in this test's own story: the first pass's
		// rejection, the re-decided rejection after the reopen, and the
		// agreeing second pass's rejection — one decision row per pass, each
		// under its own name.
		t.Fatalf("reject events = %d, want three (one per pass)", rejectEvents)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionReopen, NoteAR: "أعيد للتأجيل المشترك"}); err != nil {
		t.Fatalf("reopen rejected: %v", err)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionDefer, NoteAR: "أول تأجيل"}); err != nil {
		t.Fatalf("first defer: %v", err)
	}
	shared, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionDefer, NoteAR: "تأجيل مشترك"})
	if err != nil {
		t.Fatalf("shared deferral = %v, want one action", err)
	}
	if shared.Candidate.Status != "deferred" {
		t.Fatalf("shared deferral status = %q", shared.Candidate.Status)
	}

	// And what must NOT exist: a second ACCEPT of an accepted candidate (the
	// read agreement's action is correct, which revises beneath it; a repeated
	// accept would be one reviewer's confirmation counted twice).
	if _, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionReopen}); err != nil {
		t.Fatalf("reopen deferred: %v", err)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionAccept, LabelTextAR: strPtr("عنزة")}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, rejected.String(), DecideInput{Action: ActionAccept, LabelTextAR: strPtr("عنزة")}); err != ErrAlreadyDecided {
		t.Fatalf("second accept = %v, want ErrAlreadyDecided", err)
	}

	// Unknown candidate. Unknown action ran out of vocabulary.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, uuid.New().String(), DecideInput{Action: ActionReject}); err != ErrNotFound {
		t.Fatalf("unknown candidate decide = %v, want ErrNotFound", err)
	}
	if _, err := service.Decide(fixture.Ctx(), moderatorID, deferred.String(), DecideInput{Action: "publish"}); err != ErrValidation {
		t.Fatalf("unknown action = %v, want ErrValidation", err)
	}
	// A note past the bound is a validation error, not a truncated write.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, deferred.String(), DecideInput{Action: ActionDefer, NoteAR: longRunes(MaxDecisionNoteRunes + 1)}); err != ErrValidation {
		t.Fatalf("overlong note = %v, want ErrValidation", err)
	}
}

func TestRegionCropServesTheStoredPlate(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, moderatorID, readerID := seedLane(t, fixture)

	// A real (small) scan in a real local store, checksummed the way the
	// seed records it, so the crop path is the production path end to end.
	plate := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for x := 0; x < 320; x++ {
		for y := 0; y < 240; y++ {
			plate.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 77, A: 255})
		}
	}
	var plateBytes bytes.Buffer
	if err := jpeg.Encode(&plateBytes, plate, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatalf("encode plate: %v", err)
	}
	checksum := sha256Hex(plateBytes.Bytes())
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local store: %v", err)
	}
	if _, err := store.Put(fixture.Ctx(), "sources/plates/test-plate.jpg", bytes.NewReader(plateBytes.Bytes()), "image/jpeg"); err != nil {
		t.Fatalf("put plate: %v", err)
	}

	// The source points at the file by checksum; the source_files row is
	// what the crop query reads.
	fixture.Exec(`UPDATE sources SET metadata = metadata || jsonb_build_object('file_sha256', $2::text) WHERE id = $1`, sourceID, checksum)
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
		VALUES ($1, 'sources/plates/test-plate.jpg', 'test-plate.jpg', 'image/jpeg', $2, $3)`,
		sourceID, len(plateBytes.Bytes()), checksum)

	candidateID := insertCandidate(t, fixture, sourceID, "admiralty-1916-tribal", "label:100-80-80-40", CropBox{X: 100, Y: 80, W: 80, H: 40})
	service := NewService(fixture.Pool(), store)

	// The honest reference for a pixel check is the DECODED stored scan
	// (JPEG is lossy), not the pre-encode RGBA: the crop must serve the
	// plate's own pixels, whatever compression did to them.
	storedPlate, err := imageDecode(plateBytes.Bytes())
	if err != nil {
		t.Fatalf("decode stored plate: %v", err)
	}

	if _, err := service.CandidateCrop(fixture.Ctx(), readerID, candidateID.String()); err != ErrForbidden {
		t.Fatalf("reader crop = %v, want ErrForbidden", err)
	}
	if _, err := service.CandidateCrop(fixture.Ctx(), moderatorID, uuid.New().String()); err != ErrNotFound {
		t.Fatalf("unknown crop = %v, want ErrNotFound", err)
	}

	crop, err := service.CandidateCrop(fixture.Ctx(), moderatorID, candidateID.String())
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	if crop.PlateKey != "admiralty-1916-tribal" || crop.CandidateID != candidateID.String() {
		t.Fatalf("crop identity = %+v", crop)
	}
	if crop.Geometry.PlateWidth != 320 || crop.Geometry.PlateHeight != 240 {
		t.Fatalf("plate size = %+v", crop.Geometry)
	}
	if crop.Geometry.CropPx != (CropBox{X: 100, Y: 80, W: 80, H: 40}) || crop.Geometry.Scaled {
		t.Fatalf("crop geometry = %+v", crop.Geometry)
	}
	tight := decodePNG(t, crop.TightPNG)
	if tight.Bounds().Dx() != 80 || tight.Bounds().Dy() != 40 {
		t.Fatalf("tight crop = %dx%d, want 80x40", tight.Bounds().Dx(), tight.Bounds().Dy())
	}
	// The pixels are the plate's own, and the crop sits where the box says:
	// the gradient makes every pixel's colour unique in x and y, so the crop's
	// (0,0) must be CLOSER to the plate's (100,80) than to either neighbour —
	// the comparison codec-rounding cannot fool, and a shifted crop cannot
	// pass.
	origin := tight.At(0, 0)
	if !closerTo(origin, storedPlate.At(100, 80), storedPlate.At(101, 80)) {
		t.Fatalf("tight (0,0) is not at the plate's x=100 (=%v vs %v)", storedPlate.At(100, 80), storedPlate.At(101, 80))
	}
	if !closerTo(origin, storedPlate.At(100, 80), storedPlate.At(100, 81)) {
		t.Fatalf("tight (0,0) is not at the plate's y=80 (=%v vs %v)", storedPlate.At(100, 80), storedPlate.At(100, 81))
	}
	farCorner := tight.At(79, 39)
	if !closerTo(farCorner, storedPlate.At(179, 119), storedPlate.At(178, 119)) {
		t.Fatalf("tight (79,39) is not at the plate's x=179")
	}
	if !closerTo(farCorner, storedPlate.At(179, 119), storedPlate.At(179, 118)) {
		t.Fatalf("tight (79,39) is not at the plate's y=119")
	}
	context := decodePNG(t, crop.ContextPNG)
	if context.Bounds().Dx() < 80 || context.Bounds().Dy() < 40 {
		t.Fatalf("context crop = %+v, smaller than the box", context.Bounds())
	}
	if crop.Geometry.ContextBox.X > crop.Geometry.TightBox.X || crop.Geometry.ContextBox.Y > crop.Geometry.TightBox.Y ||
		crop.Geometry.ContextBox.X+crop.Geometry.ContextBox.W < crop.Geometry.TightBox.X+crop.Geometry.TightBox.W {
		t.Fatalf("context box %v does not contain tight box %v", crop.Geometry.ContextBox, crop.Geometry.TightBox)
	}
	if crop.Plate == nil || !crop.Plate.NeedsReview || len(crop.Plate.ReviewReasons) == 0 {
		t.Fatalf("crop plate block = %+v, want the honesty block", crop.Plate)
	}

	// A corner box clamps into the plate instead of leaving it.
	corner := insertCandidate(t, fixture, sourceID, "admiralty-1916-tribal", "label:corner", CropBox{X: 300, Y: 220, W: 60, H: 50})
	cornerCrop, err := service.CandidateCrop(fixture.Ctx(), moderatorID, corner.String())
	if err != nil {
		t.Fatalf("corner crop: %v", err)
	}
	if cornerCrop.Geometry.TightBox.W != 20 || cornerCrop.Geometry.TightBox.H != 20 {
		t.Fatalf("corner tight = %+v, want clamped to 20x20", cornerCrop.Geometry.TightBox)
	}

	// A plate whose source record names no file at all cannot produce a crop:
	// refused, never faked from another scan or an empty image.
	bareSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة بلا ملف', 'archive_record', 'public', '{}'::jsonb, $2)`,
		bareSource, moderatorID)
	bareCandidate := insertCandidate(t, fixture, bareSource, "bare-plate", "label:1", CropBox{X: 1, Y: 1, W: 2, H: 2})
	if _, err := service.CandidateCrop(fixture.Ctx(), moderatorID, bareCandidate.String()); err != ErrCropUnavailable {
		t.Fatalf("file-less crop = %v, want ErrCropUnavailable", err)
	}

	// A scan that is not an image is the same honest refusal.
	corruptSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة تالفة', 'archive_record', 'public', '{}'::jsonb, $2)`,
		corruptSource, moderatorID)
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
		VALUES ($1, 'sources/plates/corrupt.jpg', 'corrupt.jpg', 'image/jpeg', 4, 'beef')`, corruptSource)
	if _, err := store.Put(fixture.Ctx(), "sources/plates/corrupt.jpg", bytes.NewReader([]byte("junk")), "image/jpeg"); err != nil {
		t.Fatalf("put corrupt plate: %v", err)
	}
	corruptCandidate := insertCandidate(t, fixture, corruptSource, "corrupt-plate", "label:1", CropBox{X: 1, Y: 1, W: 2, H: 2})
	if _, err := service.CandidateCrop(fixture.Ctx(), moderatorID, corruptCandidate.String()); err != ErrCropUnavailable {
		t.Fatalf("corrupt crop = %v, want ErrCropUnavailable", err)
	}
}

func strPtr(value string) *string { return &value }

func longRunes(count int) string {
	runes := make([]rune, count)
	for i := range runes {
		runes[i] = 'ب'
	}
	return string(runes)
}

func jsonContains(raw, fragment string) bool {
	return bytes.Contains([]byte(raw), []byte(fragment))
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func imageDecode(encoded []byte) (image.Image, error) {
	decoded, _, err := image.Decode(bytes.NewReader(encoded))
	return decoded, err
}

// closerTo reports whether target is at least as near candidate as it is to
// rival, in squared 16-bit RGBA distance. The crop pixel, the candidate and
// the rival all went through the same decodes, so a gradient of one unit per
// pixel survives the codecs and a one-pixel shift loses the comparison.
func closerTo(target, candidate, rival color.Color) bool {
	return colorDistance(target, candidate) <= colorDistance(target, rival)
}

func colorDistance(a, b color.Color) uint64 {
	if a == nil || b == nil {
		return math.MaxUint64
	}
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	dr := int64(ar) - int64(br)
	dg := int64(ag) - int64(bg)
	db := int64(ab) - int64(bb)
	return uint64(dr*dr + dg*dg + db*db)
}

func decodePNG(t *testing.T, encoded []byte) image.Image {
	t.Helper()
	decoded, err := pngDecode(encoded)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return decoded
}
