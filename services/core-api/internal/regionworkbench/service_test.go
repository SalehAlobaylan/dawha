package regionworkbench

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	rr "github.com/SalehAlobaylan/dawha/services/core-api/internal/regionreview"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// seedTile builds one plate source with one tile and two proposals inside it.
// The candidates are answered through the REAL regionreview.Decide service —
// the workbench's verdicts must come from that lane's history and from
// nowhere else, which is only testable if the test drives that lane for real.
func seedTile(t *testing.T, fixture *testsupport.Fixture) (service *Service, candidates *rr.Service, sourceID, tileID, firstID, secondID, readerID, candidate1, candidate2 string) {
	t.Helper()
	owner := actor.Register(t, fixture, "مالك اللوحات")
	fixture.GrantRole(owner.User.ID, "researcher")
	first := actor.Register(t, fixture, "المراجع الأول")
	fixture.GrantRole(first.User.ID, "moderator")
	second := actor.Register(t, fixture, "المراجع الثاني")
	fixture.GrantRole(second.User.ID, "moderator")
	reader := actor.Register(t, fixture, "قارئ اللوحات")

	sourceID = uuid.New().String()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة قبائل الجزيرة ١٩١٦', 'archive_record', 'public',
			jsonb_build_object('license', 'public_domain', 'file_sha256', 'test-sha',
				'georef', jsonb_build_object('method', 'graticule_labels', 'certainty', 'approximate',
					'needsReview', true, 'reviewReasons', jsonb_build_array('interior_parallels_unverified_by_ink'))),
			$2)`, sourceID, owner.User.ID)

	candidate1 = insertProposal(t, fixture, sourceID, "admiralty-1916-tribal", "label:100-80-80-40", CropBox{X: 100, Y: 80, W: 80, H: 40})
	candidate2 = insertProposal(t, fixture, sourceID, "admiralty-1916-tribal", "label:200-700-70-30", CropBox{X: 200, Y: 700, W: 70, H: 30})

	tileID = uuid.New().String()
	fixture.Exec(`INSERT INTO region_tile_tasks (id, source_id, plate_key, tile_id, box_px, status)
		VALUES ($1, $2, 'admiralty-1916-tribal', 'tile:admiralty-1916-tribal:r0c0',
			$3::jsonb, 'open')`,
		tileID, sourceID, `{"x":0,"y":0,"w":1024,"h":1024}`)

	service = NewService(fixture.Pool())
	candidates = rr.NewService(fixture.Pool(), nil)
	return service, candidates, sourceID, tileID, first.User.ID, second.User.ID, reader.User.ID, candidate1, candidate2
}

func insertProposal(t *testing.T, fixture *testsupport.Fixture, sourceID, plateKey, occurrence string, box CropBox) string {
	t.Helper()
	return insertProposalWithStatus(t, fixture, sourceID, plateKey, occurrence, box, "suggested")
}

func insertProposalWithStatus(t *testing.T, fixture *testsupport.Fixture, sourceID, plateKey, occurrence string, box CropBox, status string) string {
	t.Helper()
	id := uuid.New()
	fixture.Exec(`INSERT INTO external_region_candidates
		(id, source_id, plate_key, occurrence_id, crop_px, coordinates,
		 ink_density, component_count, label_confidence, transform_rms_km, rank, status)
		VALUES ($1, $2, $3, $4, $5::jsonb, ST_SetSRID(ST_MakePoint(44.66, 26.78), 4326),
			0.2, 7, 0.8, 8.56, 0, $6)`,
		id, sourceID, plateKey, occurrence, mustJSON(box), status)
	return id.String()
}

func TestTileGateAndGuards(t *testing.T) {
	fixture := testsupport.New(t)
	service, _, _, tileID, first, _, readerID, _, _ := seedTile(t, fixture)

	if _, err := service.List(fixture.Ctx(), readerID, ListParams{}); err != ErrForbidden {
		t.Fatalf("reader list = %v, want ErrForbidden", err)
	}
	if _, err := service.Get(fixture.Ctx(), readerID, tileID); err != ErrForbidden {
		t.Fatalf("reader get = %v, want ErrForbidden", err)
	}
	if _, err := service.Claim(fixture.Ctx(), "", tileID); err != ErrUnauthenticated {
		t.Fatalf("anonymous claim = %v, want ErrUnauthenticated", err)
	}
	if _, err := service.History(fixture.Ctx(), readerID, tileID); err != ErrForbidden {
		t.Fatalf("reader history = %v, want ErrForbidden", err)
	}
	if _, err := service.List(fixture.Ctx(), first, ListParams{Status: "bogus"}); err != ErrValidation {
		t.Fatalf("bogus status = %v, want ErrValidation", err)
	}
	if _, err := service.Get(fixture.Ctx(), first, "لا-uuid"); err != ErrValidation {
		t.Fatalf("bad uuid = %v, want ErrValidation", err)
	}
	if _, err := service.Claim(fixture.Ctx(), first, uuid.New().String()); err != ErrNotFound {
		t.Fatalf("unknown claim = %v, want ErrNotFound", err)
	}
	// A submit without a seat is a conflict, whatever it claims to carry.
	if _, err := service.Submit(fixture.Ctx(), first, tileID, ""); err != ErrConflict {
		t.Fatalf("unseated submit = %v, want ErrConflict", err)
	}
}

func TestTileClaimSeatAndReleaseRoundTrip(t *testing.T) {
	fixture := testsupport.New(t)
	service, _, _, tileID, first, secondID, _, _, _ := seedTile(t, fixture)

	claimed, err := service.Claim(fixture.Ctx(), first, tileID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.Status != "in_progress" || claimed.ClaimedBy != first {
		t.Fatalf("claimed = %+v", claimed)
	}

	// The seat is exclusive: a second reviewer is refused while the first
	// sits, and the first may resume their own claim.
	if _, err := service.Claim(fixture.Ctx(), secondID, tileID); err != ErrConflict {
		t.Fatalf("second claim = %v, want ErrConflict", err)
	}
	if again, err := service.Claim(fixture.Ctx(), first, tileID); err != nil || again.Status != "in_progress" {
		t.Fatalf("self re-claim = %+v, %v; want a quiet resume", again, err)
	}

	// Release is the deferral: the seat clears, the tile reopens, and the
	// history keeps the round trip.
	released, err := service.Release(fixture.Ctx(), first, tileID, "غير جاهز الآن")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Status != "open" || released.ClaimedBy != "" {
		t.Fatalf("released = %+v", released)
	}
	history, err := service.History(fixture.Ctx(), first, tileID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.History) != 3 || // claim, claim(resume), release
		history.History[2].Action != ActionRelease || history.History[2].NoteAR != "غير جاهز الآن" {
		t.Fatalf("history after release = %+v", history.History)
	}

	// Releasing without holding the seat is refused.
	if _, err := service.Release(fixture.Ctx(), secondID, tileID, "ليس مقعدي"); err != ErrConflict {
		t.Fatalf("release without seat = %v, want ErrConflict", err)
	}
}

func TestTileTwoPersonRetirement(t *testing.T) {
	fixture := testsupport.New(t)
	service, candidates, _, tileID, first, second, _, candidate1, candidate2 := seedTile(t, fixture)

	// First pass: claim, decide both candidates, submit.
	if _, err := service.Claim(fixture.Ctx(), first, tileID); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), first, candidate1, rr.DecideInput{
		Action: rr.ActionAccept, LabelTextAR: strPtr("عنزة"), NoteAR: "اسم قبيلة",
	}); err != nil {
		t.Fatalf("first decide c1: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), first, candidate2, rr.DecideInput{
		Action: rr.ActionReject, NoteAR: "اسم بلدة",
	}); err != nil {
		t.Fatalf("first decide c2: %v", err)
	}
	submitted, err := service.Submit(fixture.Ctx(), first, tileID, "القراءة الأولى")
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if submitted.Tile.Status != "submitted" || submitted.Tile.Passes != 1 {
		t.Fatalf("first submit = %+v", submitted)
	}
	if submitted.Tile.ClaimedBy != "" {
		t.Fatalf("first submit kept the seat: %+v", submitted.Tile)
	}

	if _, err := candidates.Decide(fixture.Ctx(), first, candidate1, rr.DecideInput{
		Action: rr.ActionCorrect, LabelTextAR: strPtr("شمّر"),
	}); err != nil {
		t.Fatalf("post-submit reading revision: %v", err)
	}

	// The submitter cannot take the second pass: one person counted twice is
	// exactly what two-person retirement refuses.
	if _, err := service.Claim(fixture.Ctx(), first, tileID); err != ErrSelfReview {
		t.Fatalf("self re-claim after submit = %v, want ErrSelfReview", err)
	}

	// An incomplete second pass is refused and names the gap. Declining to
	// record an answer on candidate2 (which the first pass rejected — the
	// agreeing path runs through the candidate lane's reopen) leaves it
	// unanswered, so the submit refuses with the work named.
	if _, err := service.Claim(fixture.Ctx(), second, tileID); err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate1, rr.DecideInput{
		Action: rr.ActionCorrect, LabelTextAR: strPtr("عنزة"),
	}); err != nil {
		t.Fatalf("second decide c1: %v", err)
	}
	_, err = service.Submit(fixture.Ctx(), second, tileID, "ناقص")
	var incomplete IncompletePassError
	if !errors.As(err, &incomplete) {
		t.Fatalf("incomplete submit = %v, want IncompletePassError", err)
	}
	if len(incomplete.Missing) != 1 || incomplete.Missing[0] != candidate2 {
		t.Fatalf("missing = %v, want the untouched candidate", incomplete.Missing)
	}

	// Completing the pass retires the tile: the read agreed with the first
	// pass verbatim; the rejection the first pass made is agreed by
	// reopen-then-reject — the candidate lane's own door, never the
	// workbench re-deciding anything.
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate2, rr.DecideInput{
		Action: rr.ActionReopen, NoteAR: "أتفق مع الرفض",
	}); err != nil {
		t.Fatalf("second reopen c2: %v", err)
	}
	if _, err := service.Submit(fixture.Ctx(), second, tileID, ""); !errors.As(err, &incomplete) ||
		len(incomplete.Missing) != 1 || incomplete.Missing[0] != candidate2 {
		t.Fatalf("submit after reopen = %v, want candidate %s unanswered", err, candidate2)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate2, rr.DecideInput{
		Action: rr.ActionReject, NoteAR: "اسم بلدة",
	}); err != nil {
		t.Fatalf("second decide c2: %v", err)
	}
	retired, err := service.Submit(fixture.Ctx(), second, tileID, "الاتفاق")
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if retired.Action != ActionConfirm || retired.Tile.Status != "retired" || retired.Tile.Passes != 2 {
		t.Fatalf("second submit = %+v", retired)
	}
	if len(retired.Agreement.Confirmations) != 2 {
		t.Fatalf("agreement = %+v", retired.Agreement)
	}
	if retired.OtherSubmitter != first {
		t.Fatalf("other submitter = %q, want %q", retired.OtherSubmitter, first)
	}

	// A retired tile takes no more moves.
	if _, err := service.Claim(fixture.Ctx(), second, tileID); err != ErrConflict {
		t.Fatalf("claim retired = %v, want ErrConflict", err)
	}

	// The history carries the whole story, oldest first: A's claim and
	// submit, B's claim and submit, then the comparison's own confirmation.
	history, err := service.History(fixture.Ctx(), second, tileID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	actions := []string{}
	for _, event := range history.History {
		actions = append(actions, event.Action)
	}
	want := []string{ActionClaim, ActionSubmit, ActionClaim, ActionSubmit, ActionConfirm}
	if len(actions) != len(want) {
		t.Fatalf("history = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("history[%d] = %s, want %s (all: %v)", i, actions[i], want[i], actions)
		}
	}
	// The submit event's snapshot records what that submitter asserted, with
	// the readings — the audit trail of the pass, not a second truth.
	for _, event := range history.History {
		if event.Action == ActionConfirm && !contains(event.Snapshot, "عنزة") {
			t.Fatalf("confirm snapshot = %q, want the agreed reading", event.Snapshot)
		}
	}
}

func TestTileDisagreementAndSharedDeferralReturnTheTile(t *testing.T) {
	fixture := testsupport.New(t)
	service, candidates, _, tileID, first, second, _, candidate1, candidate2 := seedTile(t, fixture)

	// First pass: reject the first crop, defer the second (it cannot be read).
	if _, err := service.Claim(fixture.Ctx(), first, tileID); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), first, candidate1, rr.DecideInput{
		Action: rr.ActionReject, NoteAR: "شعار",
	}); err != nil {
		t.Fatalf("first decide c1: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), first, candidate2, rr.DecideInput{
		Action: rr.ActionDefer, NoteAR: "غير مقروء الآن",
	}); err != nil {
		t.Fatalf("first decide c2: %v", err)
	}
	if _, err := service.Submit(fixture.Ctx(), first, tileID, ""); err != nil {
		t.Fatalf("first submit: %v", err)
	}

	// Second pass: agree the rejection through reopen-then-reject, but READ
	// the deferred crop — a disagreement. The tile returns to the queue with
	// the divergence named, and the reviewer's own decision stays exactly as
	// they made it.
	if _, err := service.Claim(fixture.Ctx(), second, tileID); err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate1, rr.DecideInput{
		Action: rr.ActionReopen, NoteAR: "أتفق مع الرفض",
	}); err != nil {
		t.Fatalf("second reopen c1: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate1, rr.DecideInput{
		Action: rr.ActionReject, NoteAR: "شعار",
	}); err != nil {
		t.Fatalf("second decide c1: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate2, rr.DecideInput{
		Action: rr.ActionReopen, NoteAR: "أقرأ الآن",
	}); err != nil {
		t.Fatalf("second reopen c2: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate2, rr.DecideInput{
		Action: rr.ActionAccept, LabelTextAR: strPtr("بني تميم"),
	}); err != nil {
		t.Fatalf("second decide c2: %v", err)
	}
	invalidated, err := service.Submit(fixture.Ctx(), second, tileID, "قرأتها")
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if invalidated.Action != ActionInvalidate || invalidated.Tile.Status != "open" {
		t.Fatalf("disagreement result = %+v", invalidated)
	}
	if len(invalidated.Agreement.Disagreements) != 1 || invalidated.Agreement.Disagreements[0] != candidate2 {
		t.Fatalf("disagreements = %+v, want the re-read candidate", invalidated.Agreement.Disagreements)
	}
	if len(invalidated.Agreement.Confirmations) != 1 || invalidated.Agreement.Confirmations[0] != candidate1 {
		t.Fatalf("confirmations = %+v, want the agreed rejection", invalidated.Agreement.Confirmations)
	}

	// The rework: A reclaims the invalidated tile (former submitters may
	// settle a disagreement — that is what an invalidation is FOR) and
	// defers instead of reading: retract B's verdict through the candidate
	// lane's reopen first; the workbench never re-decides a settled row.
	if _, err := service.Claim(fixture.Ctx(), first, tileID); err != nil {
		t.Fatalf("rework claim: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), first, candidate2, rr.DecideInput{
		Action: rr.ActionReopen, NoteAR: "أتراجع عن القراءة",
	}); err != nil {
		t.Fatalf("reopen c2: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), first, candidate2, rr.DecideInput{
		Action: rr.ActionDefer, NoteAR: "ما زلت غير قادر",
	}); err != nil {
		t.Fatalf("defer decide: %v", err)
	}
	reworked, err := service.Submit(fixture.Ctx(), first, tileID, "أجّلته مجدداً")
	if err != nil {
		t.Fatalf("rework submit: %v", err)
	}
	// A's defer against B's earlier accept is still a disagreement, not a
	// shared deferral: neither pass can confirm a candidate only one of them
	// read.
	if reworked.Action != ActionInvalidate || reworked.Tile.Status != "open" {
		t.Fatalf("rework result = %+v", reworked)
	}
	if len(reworked.Agreement.Disagreements) != 1 || reworked.Agreement.Disagreements[0] != candidate2 {
		t.Fatalf("rework disagreements = %+v, want the contested candidate", reworked.Agreement.Disagreements)
	}

	// B's answer to the rework: they also defer now (retracting their
	// accept). A deferral WITH a deferral is still not a retirement — a
	// shared inability is honest, but it is not two confirmations, and the
	// tile stays open with the deferral named for whenever the crop is
	// picked up again.
	if _, err := service.Claim(fixture.Ctx(), second, tileID); err != nil {
		t.Fatalf("B rework claim: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate2, rr.DecideInput{
		Action: rr.ActionReopen, NoteAR: "أتفق عليه: غير مقروءة",
	}); err != nil {
		t.Fatalf("B reopen decide: %v", err)
	}
	if _, err := candidates.Decide(fixture.Ctx(), second, candidate2, rr.DecideInput{
		Action: rr.ActionDefer, NoteAR: "أتّفق: غير مقروءة",
	}); err != nil {
		t.Fatalf("B defer decide: %v", err)
	}
	deferredResult, err := service.Submit(fixture.Ctx(), second, tileID, "أتّفق: غير مقروءة")
	if err != nil {
		t.Fatalf("deferred submit: %v", err)
	}
	if deferredResult.Action != ActionInvalidate || deferredResult.Tile.Status != "open" {
		t.Fatalf("deferral result = %+v", deferredResult)
	}
	if len(deferredResult.Agreement.Deferrals) != 1 || deferredResult.Agreement.Deferrals[0] != candidate2 {
		t.Fatalf("deferrals = %+v, want the deferred candidate", deferredResult.Agreement.Deferrals)
	}
	if len(deferredResult.Agreement.Confirmations) != 1 || deferredResult.Agreement.Confirmations[0] != candidate1 {
		t.Fatalf("confirmations = %+v, want the twice-agreed rejection", deferredResult.Agreement.Confirmations)
	}

	// The deferred candidate returns through the candidate lane's own door:
	// the workbench never re-decides anyone's answer, and the tile sits open
	// with its seat free until the deferred work is picked up again.
	deferredLane, err := service.Get(fixture.Ctx(), first, tileID)
	if err != nil {
		t.Fatalf("tile get after deferral: %v", err)
	}
	if deferredLane.Tile.Status != "open" || deferredLane.Tile.ClaimedBy != "" {
		t.Fatalf("tile after deferral = %+v", deferredLane.Tile)
	}
}

func strPtr(value string) *string { return &value }

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
