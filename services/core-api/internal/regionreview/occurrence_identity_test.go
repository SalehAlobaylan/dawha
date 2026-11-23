package regionreview

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

func TestOccurrenceIdentityRequiresModeratorAndPreservesHistory(t *testing.T) {
	fixture := testsupport.New(t)
	reader := actor.Register(t, fixture, "قارئ الشاهد")
	moderator := actor.Register(t, fixture, "مراجع الشاهد")
	fixture.GrantRole(moderator.User.ID, "moderator")
	source, item, revision, occurrence, tribe := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fixture.Exec(`INSERT INTO sources (id,title_ar,source_type,visibility,created_by)
		VALUES ($1,'شاهد الربط','book','public',$2)`, source, moderator.User.ID)
	fixture.Exec(`INSERT INTO atlas_source_items (id,source_id,item_key) VALUES ($1,$2,'page-1')`, item, source)
	fixture.Exec(`INSERT INTO atlas_source_item_revisions
		(id,item_id,revision_key,locator_ar,external_url,rights_ar,credit_ar)
		VALUES ($1,$2,'v1','ص ١','https://example.org/page','رابط','المؤلف')`, revision, item)
	fixture.Exec(`INSERT INTO atlas_source_occurrences
		(id,item_revision_id,occurrence_key,original_text,display_ar,language,reading_method,published)
		VALUES ($1,$2,'one','مطير','مطير','ar','original',true)`, occurrence, revision)
	fixture.Exec(`INSERT INTO tribes (id,canonical_name_ar,normalized_name_ar,visibility,created_by)
		VALUES ($1,'مطير الاختبار','مطير الاختبار','private',$2)`, tribe, moderator.User.ID)

	service := NewService(fixture.Pool(), nil)
	review := OccurrenceIdentityInput{TribeID: tribe.String(), NoteAR: "قارنت الاسم بقراءة أخرى"}
	if err := service.ReviewOccurrenceIdentity(fixture.Ctx(), reader.User.ID, occurrence.String(), review); !errors.Is(err, ErrForbidden) {
		t.Fatalf("reader review: %v", err)
	}
	if err := service.ReviewOccurrenceIdentity(fixture.Ctx(), moderator.User.ID, occurrence.String(), review); !errors.Is(err, ErrValidation) {
		t.Fatalf("private target review: %v", err)
	}
	fixture.Exec(`UPDATE tribes SET visibility='public' WHERE id=$1`, tribe)
	if err := service.ReviewOccurrenceIdentity(fixture.Ctx(), moderator.User.ID, occurrence.String(), review); err != nil {
		t.Fatal(err)
	}
	if err := service.ReviewOccurrenceIdentity(fixture.Ctx(), moderator.User.ID, occurrence.String(), OccurrenceIdentityInput{NoteAR: "أعيد إلى المراجعة"}); err != nil {
		t.Fatal(err)
	}

	var unresolved bool
	var decisions int
	err := fixture.Pool().QueryRow(fixture.Ctx(), `
		SELECT tribe_id IS NULL, (SELECT count(*) FROM atlas_occurrence_identity_decisions WHERE occurrence_id=$1)
		FROM atlas_source_occurrences WHERE id=$1`, occurrence).Scan(&unresolved, &decisions)
	if err != nil || !unresolved || decisions != 2 {
		t.Fatalf("unresolved=%v decisions=%d err=%v", unresolved, decisions, err)
	}
	place := uuid.New()
	fixture.Exec(`INSERT INTO places (id,canonical_name_ar,normalized_name_ar,place_type,visibility,created_by)
		VALUES ($1,'بريدة الاختبار','بريده الاختبار','city','public',$2)`, place, moderator.User.ID)
	err = service.ReviewOccurrenceIdentity(fixture.Ctx(), moderator.User.ID, occurrence.String(),
		OccurrenceIdentityInput{PlaceID: place.String(), TribeID: tribe.String(), NoteAR: "اختيار متعارض"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("two targets: %v", err)
	}
	err = service.ReviewOccurrenceIdentity(fixture.Ctx(), moderator.User.ID, occurrence.String(),
		OccurrenceIdentityInput{PlaceID: place.String(), NoteAR: "سجل موضع"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := geography.NewService(fixture.Pool()).SourceOccurrences(fixture.Ctx(), "", geography.OccurrenceInput{ID: occurrence.String()})
	if err != nil || len(page.Occurrences) != 1 || page.Occurrences[0].PlaceID != place.String() || page.Occurrences[0].TribeID != "" {
		t.Fatalf("typed target result: %+v %v", page, err)
	}

}
