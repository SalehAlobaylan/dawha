package geography

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// seedTribeReceipts builds one tribe with the three receipt shapes across both
// lanes: accepted spatial evidence from a public source, accepted spatial
// evidence from a private source (the laundering trap), a pending spatial row,
// and a documented claim whose evidence rides a public passage and a statement.
func seedTribeReceipts(t *testing.T, fixture *testsupport.Fixture) (tribeID uuid.UUID, placeID uuid.UUID) {
	t.Helper()
	owner := actor.Register(t, fixture, "مالك لوحة القبيلة")

	tribe := uuid.New()
	place := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility, created_by)
		VALUES ($1, 'دار الحرب', 'دار الحرب', 'region', ST_SetSRID(ST_MakePoint(41.0, 24.0), 4326), 'public', $2)`,
		place, owner.User.ID)

	publicSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'لوريمر ١٩٠٨ — قبائل', 'book', 'public', $2)`, publicSource, owner.User.ID)
	privateSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر خصوصي للقبيلة', 'book', 'private', $2)`, privateSource, owner.User.ID)
	unknownSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر لا يفتحه القارئ', 'book', 'private', $2)`, unknownSource, owner.User.ID)

	publicPassage := uuid.New()
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 59157, 'بداية مدخل حرب', 'نص لوريمر عن دار حرب', 'نص لوريمر عن دار حرب')`, publicPassage, publicSource)
	privatePassage := uuid.New()
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 30, 'حرب — ازل', 'شاهد من المصدر الخصوصي', 'شاهد من المصدر الخصوصي')`, privatePassage, privateSource)
	pendingPassage := uuid.New()
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, 2, 31, 'حرب — ازل', 'شاهد بانتظار المراجعة', 'شاهد بانتظار المراجعة')`, pendingPassage, publicSource)

	// Three spatial receipts through the tribe's associations.
	publicAssociation := uuid.New()
	fixture.Exec(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, source_id, created_by)
		VALUES ($1, 'tribe', $2, $3, 'originated_in', 'documented', $4, $5)`,
		publicAssociation, tribe, place, publicSource, owner.User.ID)
	privateAssociation := uuid.New()
	fixture.Exec(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, source_id, created_by)
		VALUES ($1, 'tribe', $2, $3, 'documented_in', 'documented', 'approximate', $4, $5)`,
		privateAssociation, tribe, place, privateSource, owner.User.ID)

	fixture.Exec(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, evidence_text_ar, review_status, created_by)
		VALUES ($1, $2, $3, NULL, 'accepted', $4)`, uuid.New(), publicAssociation, publicPassage, owner.User.ID)
	fixture.Exec(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, evidence_text_ar, review_status, created_by)
		VALUES ($1, $2, $3, NULL, 'accepted', $4)`, uuid.New(), privateAssociation, privatePassage, owner.User.ID)
	pendingRow := uuid.New()
	fixture.Exec(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, evidence_text_ar, review_status, created_by)
		VALUES ($1, $2, $3, NULL, 'needs_review', $4)`, pendingRow, publicAssociation, pendingPassage, owner.User.ID)

	// A documented claim with two receipts: a passage and a statement.
	claim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, place_id, status, notes_ar, created_by)
		VALUES ($1, 'tribe', $2, 'originated_in', 'place', $3, $3, 'documented', 'تدعٍّ موثق بالنص', $4)`,
		claim, tribe, place, owner.User.ID)
	publicStatement := uuid.New()
	fixture.Exec(`INSERT INTO source_statements (id, source_id, statement_text_ar, extraction_method, review_status, created_by)
		VALUES ($1, $2, 'كلام لوريمر المستخرج', 'ocr', 'accepted', $3)`, publicStatement, publicSource, owner.User.ID)
	unknownPassage := uuid.New()
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, locator_ar, text_ar, normalized_text_ar)
		VALUES ($1, $2, 9, 'ص 99', 'شاهد من مصدر لا يفتحه القارئ', 'شاهد من مصدر لا يفتحه القارئ')`, unknownPassage, unknownSource)
	fixture.Exec(`INSERT INTO claim_evidence (id, claim_id, source_passage_id, relation, created_by)
		VALUES ($1, $2, $3, 'supports', $4)`, uuid.New(), claim, publicPassage, owner.User.ID)
	fixture.Exec(`INSERT INTO claim_evidence (id, claim_id, source_statement_id, relation, created_by)
		VALUES ($1, $2, $3, 'supports', $4)`, uuid.New(), claim, publicStatement, owner.User.ID)
	hidden := uuid.New()
	fixture.Exec(`INSERT INTO claim_evidence (id, claim_id, source_passage_id, relation, created_by)
		VALUES ($1, $2, $3, 'contradicts', $4)`, hidden, claim, unknownPassage, owner.User.ID)

	_ = owner
	return tribe, place
}

func TestTribeEvidenceKeepsAPrivateSourceInvisibleBehindAPublicTribe(t *testing.T) {
	fixture := testsupport.New(t)
	tribe, _ := seedTribeReceipts(t, fixture)
	service := NewService(fixture.Pool())

	// The default lane: only the accepted rows quoting sources the reader may
	// open. The private source's quote is not laundered through the tribe's
	// publicity — and the claim whose evidence set carries a private source is
	// not public as a unit (the claim system's own rule), so none of its rows
	// reach the plain reader either.
	report, err := service.TribeEvidence(fixture.Ctx(), tribe.String(), "", false)
	if err != nil {
		t.Fatalf("tribe folio: %v", err)
	}
	if report.Status != "accepted" {
		t.Fatalf("folio status = %q", report.Status)
	}
	if len(report.Evidence) != 1 {
		t.Fatalf("receipt rows = %d, want 1 (the public-source spatial receipt)", len(report.Evidence))
	}
	if report.Evidence[0].Kind != "spatial_evidence" || report.Evidence[0].SourceTitle != "لوريمر ١٩٠٨ — قبائل" {
		t.Fatalf("the visible receipt = %+v", report.Evidence[0])
	}

	// The moderation lane is the same records one level deeper — and only a
	// moderator may walk it.
	if _, err := service.TribeEvidence(fixture.Ctx(), tribe.String(), "", true); err != ErrPlaceEvidenceForbidden {
		t.Fatalf("anonymous needs-review lane = %v, want the moderation refusal", err)
	}
	moderator := actor.Register(t, fixture, "مشرف الورشة")
	fixture.GrantRole(moderator.User.ID, "moderator")
	moderated, err := service.TribeEvidence(fixture.Ctx(), tribe.String(), moderator.User.ID, true)
	if err != nil {
		t.Fatalf("moderator folio: %v", err)
	}
	if moderated.Status != "accepted+needs_review" {
		t.Fatalf("moderation lane status = %q", moderated.Status)
	}
	if len(moderated.Evidence) != 6 {
		t.Fatalf("moderator rows = %d, want 6 (3 spatial + 3 claims evidence)", len(moderated.Evidence))
	}

	// A malformed tribe id is a validation answer, not an empty folio.
	if _, err := service.TribeEvidence(fixture.Ctx(), "walnut", moderator.User.ID, false); err != ErrValidation {
		t.Fatalf("malformed tribe id error = %v, want ErrValidation", err)
	}
}
