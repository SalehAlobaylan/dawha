package pairing

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

func seedPairing(t *testing.T, fixture *testsupport.Fixture) (sourceID, placeID, candidateID uuid.UUID, moderatorID, readerID string) {
	t.Helper()
	owner := actor.Register(t, fixture, "صاحب المقارن")
	fixture.GrantRole(owner.User.ID, "researcher")
	moderator := actor.Register(t, fixture, "مدير المقارن")
	fixture.GrantRole(moderator.User.ID, "moderator")
	reader := actor.Register(t, fixture, "قارئ المقارن")

	sourceID = uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'غزتار خارجي', 'website', 'public', jsonb_build_object('license', 'CC0'), $2)`, sourceID, owner.User.ID)

	placeID = uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility, created_by)
		VALUES ($1, 'المدينة المنورة', 'المدينه المنوره', 'city', ST_SetSRID(ST_MakePoint(39.6, 24.4), 4326), 'public', $2)`,
		placeID, owner.User.ID)

	candidateID = uuid.New()
	fixture.Exec(`INSERT INTO external_place_candidates
		(id, source_id, external_key, occurrence_id, our_place_id, normalized_candidates, coordinates, name_similarity, distance_km, rank, status)
		VALUES ($1, $2, 'Q35484', 'Q35484::يثرب', $3, 'يثرب', ST_SetSRID(ST_MakePoint(39.61, 24.41), 4326), 0.42, 1.2, 1, 'suggested')`,
		candidateID, sourceID, placeID)
	return sourceID, placeID, candidateID, moderator.User.ID, reader.User.ID
}

func TestPairingLaneGateAndAcceptEffects(t *testing.T) {
	fixture := testsupport.New(t)
	_, placeID, candidateID, moderatorID, readerID := seedPairing(t, fixture)
	service := NewService(fixture.Pool())

	// Non-moderators are refused the lane by role, anonymous included.
	if _, err := service.List(fixture.Ctx(), readerID, "suggested"); err != ErrForbidden {
		t.Fatalf("reader list error = %v, want ErrForbidden", err)
	}
	if _, err := service.List(fixture.Ctx(), "", "suggested"); err != ErrUnauthenticated {
		t.Fatalf("anonymous list error = %v, want ErrUnauthenticated", err)
	}
	if _, err := service.List(fixture.Ctx(), uuid.New().String(), "suggested"); err != ErrUnauthenticated {
		t.Fatalf("unknown-user list error = %v, want ErrUnauthenticated", err)
	}
	// An unknown status is validation, not an empty lane.
	if _, err := service.List(fixture.Ctx(), moderatorID, "bogus"); err != ErrValidation {
		t.Fatalf("bogus status error = %v, want ErrValidation", err)
	}

	// The moderator sees the proposal as a proposal, not as a fact.
	report, err := service.List(fixture.Ctx(), moderatorID, "suggested")
	if err != nil {
		t.Fatalf("moderator list: %v", err)
	}
	if len(report.Candidates) != 1 {
		t.Fatalf("suggested candidates = %d, want 1", len(report.Candidates))
	}
	if report.Candidates[0].Status != "suggested" {
		t.Fatalf("lane status = %q, want suggested", report.Candidates[0].Status)
	}

	// Alias-only accept: a historical name lands, a crosswalk does NOT.
	decided, err := service.Decide(fixture.Ctx(), moderatorID, candidateID.String(), EffectAlias, "اسم تاريخي من المصدر")
	if err != nil {
		t.Fatalf("alias decide: %v", err)
	}
	if decided.Candidate.Status != "alias_only" || decided.Effect != EffectAlias {
		t.Fatalf("alias decision = %+v", decided)
	}
	var names int
	if err := fixture.Pool().QueryRow(fixture.Ctx(),
		`SELECT count(*) FROM historical_place_names WHERE place_id = $1 AND normalized_name_ar = $2`,
		placeID, identity.NormalizeArabicName("يثرب")).Scan(&names); err != nil {
		t.Fatalf("count alias names: %v", err)
	}
	if names != 1 {
		t.Fatalf("alias names = %d, want 1", names)
	}
	var crosswalk int
	if err := fixture.Pool().QueryRow(fixture.Ctx(),
		`SELECT count(*) FROM place_external_ids WHERE place_id = $1`, placeID).Scan(&crosswalk); err != nil {
		t.Fatalf("count crosswalk: %v", err)
	}
	if crosswalk != 0 {
		t.Fatalf("alias crosswalk rows = %d, want 0 (alias writes no crosswalk)", crosswalk)
	}

	// The candidate is now decided: a second decision is a conflict, and the
	// lane no longer lists it as suggested.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, candidateID.String(), EffectLink, "ثانية"); err != ErrAlreadyDecided {
		t.Fatalf("second decide error = %v, want ErrAlreadyDecided", err)
	}
	again, err := service.List(fixture.Ctx(), moderatorID, "suggested")
	if err != nil {
		t.Fatalf("re-list suggested: %v", err)
	}
	if len(again.Candidates) != 0 {
		t.Fatalf("suggested after decide = %d, want 0", len(again.Candidates))
	}
	aliasLane, err := service.List(fixture.Ctx(), moderatorID, "alias_only")
	if err != nil {
		t.Fatalf("list alias_only: %v", err)
	}
	if len(aliasLane.Candidates) != 1 {
		t.Fatalf("alias_only candidates = %d, want 1", len(aliasLane.Candidates))
	}

	// An unknown candidate is not found, not empty.
	if _, err := service.Decide(fixture.Ctx(), moderatorID, uuid.New().String(), EffectLink, ""); err != ErrNotFound {
		t.Fatalf("unknown decide error = %v, want ErrNotFound", err)
	}
}

func TestPairingSearchFilterReopenAndHistory(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, placeID, _, moderatorID, readerID := seedPairing(t, fixture)
	service := NewService(fixture.Pool())

	// Search is text across the fields a moderator scans: a place-name hit
	// returns the candidate, a nonsense hit returns nothing rather than all.
	second := uuid.New()
	fixture.Exec(`INSERT INTO external_place_candidates
		(id, source_id, external_key, occurrence_id, our_place_id, normalized_candidates, coordinates, name_similarity, status)
		VALUES ($1, $2, 'Q777', 'Q777::غائب', $3, 'غائب', ST_SetSRID(ST_MakePoint(39.6, 24.4), 4326), 0.1, 'suggested')`,
		second, sourceID, placeID)
	found, err := service.ListWithParams(fixture.Ctx(), moderatorID, ListParams{Status: "suggested", Query: "غائب"})
	if err != nil {
		t.Fatalf("search listed: %v", err)
	}
	if len(found.Candidates) != 1 || found.Candidates[0].ID != second.String() {
		t.Fatalf("search hit = %+v, want only the غائب candidate", found.Candidates)
	}
	none, err := service.ListWithParams(fixture.Ctx(), moderatorID, ListParams{Status: "suggested", Query: "لا-يوجد-هذا"})
	if err != nil {
		t.Fatalf("empty search listed: %v", err)
	}
	if len(none.Candidates) != 0 {
		t.Fatalf("empty search candidates = %d, want 0", len(none.Candidates))
	}
	byPlace, err := service.ListWithParams(fixture.Ctx(), moderatorID, ListParams{Status: "suggested", PlaceID: placeID.String()})
	if err != nil {
		t.Fatalf("place filter listed: %v", err)
	}
	if len(byPlace.Candidates) != 2 {
		t.Fatalf("place filter candidates = %d, want 2", len(byPlace.Candidates))
	}
	if _, err := service.ListWithParams(fixture.Ctx(), moderatorID, ListParams{Status: "suggested", PlaceID: "walnut"}); err != ErrValidation {
		t.Fatalf("bad place filter error = %v, want ErrValidation", err)
	}
	_ = readerID

	// Reopen is allowed for no-effect decisions only: reject then reopen
	// returns the candidate to suggested and appends both events.
	rejected, err := service.Decide(fixture.Ctx(), moderatorID, second.String(), EffectReject, "لا يقابل")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Candidate.Status != "rejected" {
		t.Fatalf("reject status = %q, want rejected", rejected.Candidate.Status)
	}
	reopened, err := service.Reopen(fixture.Ctx(), moderatorID, second.String(), "نراجع المصدر")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Candidate.Status != "suggested" {
		t.Fatalf("reopened status = %q, want suggested", reopened.Candidate.Status)
	}
	history, err := service.History(fixture.Ctx(), moderatorID, second.String())
	if err != nil {
		t.Fatalf("history listed: %v", err)
	}
	if len(history.History) != 2 {
		t.Fatalf("history events = %d, want 2 (reject + reopen)", len(history.History))
	}
	if history.History[0].Action != "reject" || history.History[0].ToStatus != "rejected" {
		t.Fatalf("first history event = %+v", history.History[0])
	}
	if history.History[1].Action != "reopen" || history.History[1].FromStatus != "rejected" || history.History[1].ToStatus != "suggested" {
		t.Fatalf("second history event = %+v", history.History[1])
	}

	// A status that already produced a fact cannot be reopened, and a non-
	// moderator cannot read the trail.
	linked, err := service.Decide(fixture.Ctx(), moderatorID, second.String(), EffectLink, "ربط موثق")
	if err != nil {
		t.Fatalf("link after reopen: %v", err)
	}
	if linked.Candidate.Status != "accepted" {
		t.Fatalf("link status = %q", linked.Candidate.Status)
	}
	if _, err := service.Reopen(fixture.Ctx(), moderatorID, second.String(), "محاولة"); err != ErrCannotReopen {
		t.Fatalf("fact-producing reopen error = %v, want ErrCannotReopen", err)
	}
	if _, err := service.History(fixture.Ctx(), readerID, second.String()); err != ErrForbidden {
		t.Fatalf("reader history error = %v, want ErrForbidden", err)
	}
}

func TestPairingLinkWritesCrosswalkAndCrosswalkHidesPrivateSource(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, placeID, _, moderatorID, readerID := seedPairing(t, fixture)
	service := NewService(fixture.Pool())

	// A fresh candidate for the link arm.
	second := uuid.New()
	fixture.Exec(`INSERT INTO external_place_candidates
		(id, source_id, external_key, occurrence_id, our_place_id, normalized_candidates, coordinates, name_similarity, status)
		VALUES ($1, $2, 'Q35484', 'Q35484::طيبة', $3, 'طيبة', ST_SetSRID(ST_MakePoint(39.6, 24.4), 4326), 0.5, 'suggested')`,
		second, sourceID, placeID)

	decided, err := service.Decide(fixture.Ctx(), moderatorID, second.String(), EffectLink, "ربط موثق")
	if err != nil {
		t.Fatalf("link decide: %v", err)
	}
	if decided.Candidate.Status != "accepted" {
		t.Fatalf("link status = %q, want accepted", decided.Candidate.Status)
	}
	ids, err := service.ExternalIDs(fixture.Ctx(), placeID.String(), readerID)
	if err != nil {
		t.Fatalf("external ids: %v", err)
	}
	if len(ids.ExternalIDs) != 1 || ids.ExternalIDs[0].ExternalKey != "Q35484" {
		t.Fatalf("external ids = %+v, want the accepted Q35484", ids.ExternalIDs)
	}

	// The source goes private: the crosswalk row it backs disappears, while
	// the link decision itself stays in the lane history.
	fixture.Exec(`UPDATE sources SET visibility = 'private' WHERE id = $1`, sourceID)
	hidden, err := service.ExternalIDs(fixture.Ctx(), placeID.String(), readerID)
	if err != nil {
		t.Fatalf("external ids after private: %v", err)
	}
	if len(hidden.ExternalIDs) != 0 {
		t.Fatalf("crosswalk through a private source = %d, want 0", len(hidden.ExternalIDs))
	}

	// A reject writes the decision and no other row.
	third := uuid.New()
	fixture.Exec(`INSERT INTO external_place_candidates
		(id, source_id, external_key, occurrence_id, our_place_id, normalized_candidates, coordinates, name_similarity, status)
		VALUES ($1, $2, 'Q35484', 'Q35484::زائف', $3, 'زائف', ST_SetSRID(ST_MakePoint(39.6, 24.4), 4326), 0.1, 'suggested')`,
		third, sourceID, placeID)
	rejected, err := service.Decide(fixture.Ctx(), moderatorID, third.String(), EffectReject, "لا يقابل")
	if err != nil {
		t.Fatalf("reject decide: %v", err)
	}
	if rejected.Candidate.Status != "rejected" {
		t.Fatalf("reject status = %q", rejected.Candidate.Status)
	}
}
