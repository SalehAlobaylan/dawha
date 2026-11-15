package geography

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The dossier is the surface where a folklore trap lives: the place is public,
// so the folio feels like a public page, but every line on it quotes a page of
// some source — and a source the reader may not open must not be quoteable
// through a place that anyone can read. These tests pin that rule and the
// moderation lane's gate.
func seedDossier(t *testing.T, fixture *testsupport.Fixture) (placeID uuid.UUID, ownerID, readerID string) {
	t.Helper()
	owner := actor.Register(t, fixture, "صاحب المحتوى")
	fixture.GrantRole(owner.User.ID, "researcher")
	reader := actor.Register(t, fixture, "قارئ المحتوى")

	publicSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر عمومي المحتوى', 'book', 'public', $2)`, publicSource, owner.User.ID)
	privateSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر خصوصي المحتوى', 'book', 'private', $2)`, privateSource, owner.User.ID)

	place := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility, created_by)
		VALUES ($1, 'موضع المحتوى', 'موضع المحتوى', 'city', ST_SetSRID(ST_MakePoint(46.6, 24.6), 4326), 'public', $2)`,
		place, owner.User.ID)

	publicPassage := uuid.New()
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 10, 'ص 10', 'نص عمومي عن الموضع', 'نص عمومي عن الموضع')`, publicPassage, publicSource)
	privatePassage := uuid.New()
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 20, 'ص 20', 'نص خصوصي عن الموضع', 'نص خصوصي عن الموضع')`, privatePassage, privateSource)
	pendingPassage := uuid.New()
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, 2, 30, 'ص 30', 'نص بانتظار المراجعة', 'نص بانتظار المراجعة')`, pendingPassage, publicSource)

	personID := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by)
		VALUES ($1, 'شخص المحتوى', 'شخص المحتوى', 'reviewed', $2)`, personID, owner.User.ID)

	for _, item := range []struct {
		association uuid.UUID
		passage     uuid.UUID
		status      string
	}{
		{uuid.New(), publicPassage, "accepted"},
		{uuid.New(), privatePassage, "accepted"},
		{uuid.New(), pendingPassage, "needs_review"},
	} {
		association := item.association
		fixture.Exec(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, source_id, created_by)
			VALUES ($1, 'person', $2, $3, 'documented_in', 'documented', 'approximate', NULL, $4)`,
			association, personID, place, owner.User.ID)
		fixture.Exec(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, review_status, created_by)
			VALUES ($1, $2, $3, $4, $5)`, uuid.New(), association, item.passage, item.status, owner.User.ID)
	}
	return place, owner.User.ID, reader.User.ID
}

func TestPlaceEvidenceKeepsAPrivateSourceInvisibleBehindAPublicPlace(t *testing.T) {
	fixture := testsupport.New(t)
	place, ownerID, readerID := seedDossier(t, fixture)
	service := NewService(fixture.Pool())

	// The public-source folio is one line for the plain reader: the private
	// source's quote is not laundered by a public place. The anonymous reader
	// reads the same single row.
	for _, who := range []string{readerID, ""} {
		report, err := service.PlaceEvidence(fixture.Ctx(), place.String(), who, false)
		if err != nil {
			t.Fatalf("dossier for %q: %v", who, err)
		}
		if report.Status != "accepted" {
			t.Fatalf("accepted lane status = %q", report.Status)
		}
		if len(report.Evidence) != 1 {
			t.Fatalf("reader %q sees %d evidence rows, want exactly the public source's row", who, len(report.Evidence))
		}
		if report.Evidence[0].SourceTitle != "مصدر عمومي المحتوى" {
			t.Fatalf("the one visible row reads %q", report.Evidence[0].SourceTitle)
		}
		if report.Evidence[0].PassageText != "نص عمومي عن الموضع" {
			t.Fatalf("visible row's passage = %q", report.Evidence[0].PassageText)
		}
		if report.Evidence[0].PageNumber == nil || *report.Evidence[0].PageNumber != 10 {
			t.Fatalf("visible row's page = %v, want 10", report.Evidence[0].PageNumber)
		}
	}

	// The author's own sources are theirs to read through the same place: the
	// rule follows the reader's grants, not the place's reach. In the accepted
	// lane the author sees both of their rows; the pending row stays in its lane.
	authorReport, err := service.PlaceEvidence(fixture.Ctx(), place.String(), ownerID, false)
	if err != nil {
		t.Fatalf("author dossier: %v", err)
	}
	if len(authorReport.Evidence) != 2 {
		t.Fatalf("author accepted rows = %d, want 2 (public + own private)", len(authorReport.Evidence))
	}

	// The moderation lane is role-gated, and authorship is not a moderation
	// role: the person who wrote the content is refused the pending lane just
	// like any other researcher. Owning a record does not make the lane yours.
	if _, err := service.PlaceEvidence(fixture.Ctx(), place.String(), ownerID, true); err != ErrPlaceEvidenceForbidden {
		t.Fatalf("owner pending-lane error = %v, want ErrPlaceEvidenceForbidden", err)
	}
}

func TestPlaceEvidenceModerationLaneRefusesNonModerators(t *testing.T) {
	fixture := testsupport.New(t)
	place, _, readerID := seedDossier(t, fixture)
	service := NewService(fixture.Pool())

	// A plain reader asking for the pending lane is refused by role, not by
	// content — the refusal carries no rows to read between.
	if _, err := service.PlaceEvidence(fixture.Ctx(), place.String(), readerID, true); err != ErrPlaceEvidenceForbidden {
		t.Fatalf("plain reader pending-lane error = %v, want ErrPlaceEvidenceForbidden", err)
	}
	// A researcher is a write-role for identity, not a moderation role.
	researcher := actor.Register(t, fixture, "باحث ورد المحتوى")
	fixture.GrantRole(researcher.User.ID, "researcher")
	if _, err := service.PlaceEvidence(fixture.Ctx(), place.String(), researcher.User.ID, true); err != ErrPlaceEvidenceForbidden {
		t.Fatalf("researcher pending-lane error = %v, want ErrPlaceEvidenceForbidden", err)
	}
	// And the anonymous caller has no session to even attempt it with.
	if _, err := service.PlaceEvidence(fixture.Ctx(), place.String(), "", true); err != ErrPlaceEvidenceForbidden {
		t.Fatalf("anonymous pending-lane error = %v, want ErrPlaceEvidenceForbidden", err)
	}

	// A moderator asks for the lane and reads the deeper dossier through it:
	// the pending row AND both accepted rows, because the research band lifts
	// the source wall — the lane is the same records, one level deeper.
	moderator := actor.Register(t, fixture, "مدير ورد المحتوى")
	fixture.GrantRole(moderator.User.ID, "moderator")
	report, err := service.PlaceEvidence(fixture.Ctx(), place.String(), moderator.User.ID, true)
	if err != nil {
		t.Fatalf("moderator dossier: %v", err)
	}
	if report.Status != "accepted+needs_review" {
		t.Fatalf("moderation lane status = %q", report.Status)
	}
	if len(report.Evidence) != 3 {
		t.Fatalf("moderator rows = %d, want the full dossier (3)", len(report.Evidence))
	}
	pending := 0
	for _, row := range report.Evidence {
		if row.Status == "needs_review" {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("moderator pending rows = %d, want 1", pending)
	}

	// And the moderator reading the plain lane gets the accepted-only view,
	// same as every reader: the flag opens the lane, it does not rewrite the
	// default.
	moderatorAccepted, err := service.PlaceEvidence(fixture.Ctx(), place.String(), moderator.User.ID, false)
	if err != nil {
		t.Fatalf("moderator accepted lane: %v", err)
	}
	if moderatorAccepted.Status != "accepted" {
		t.Fatalf("moderator plain-lane status = %q, want accepted", moderatorAccepted.Status)
	}
	if len(moderatorAccepted.Evidence) != 2 {
		t.Fatalf("moderator accepted rows = %d, want the two accepted rows", len(moderatorAccepted.Evidence))
	}

	// An unknown place is a validation answer, not a leak of existence.
	if _, err := service.PlaceEvidence(fixture.Ctx(), "walnut", readerID, false); err != ErrValidation {
		t.Fatalf("malformed place id error = %v, want ErrValidation", err)
	}
}
