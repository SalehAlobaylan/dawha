package regionreview

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

func TestAtlasStageReplayAndChangedScanKeepIndependentReview(t *testing.T) {
	fixture := testsupport.New(t)
	reader := actor.Register(t, fixture, "قارئ مسودة الخريطة")
	moderator := actor.Register(t, fixture, "مراجع مسودة الخريطة")
	fixture.GrantRole(moderator.User.ID, "moderator")
	source, item, base := uuid.New(), uuid.New(), uuid.New()
	fixture.Exec(`INSERT INTO sources (id,title_ar,source_type,visibility,created_by)
		VALUES ($1,'خريطة اختبار المسودة','book','public',$2)`, source, moderator.User.ID)
	fixture.Exec(`INSERT INTO atlas_source_items (id,source_id,item_key) VALUES ($1,$2,'plate-1')`, item, source)
	fixture.Exec(`INSERT INTO atlas_source_item_revisions
		(id,item_id,revision_key,locator_ar,external_url,rights_ar,credit_ar)
		VALUES ($1,$2,'v1','ص ١','https://example.org/page','رابط','المؤلف')`, base, item)
	service := NewService(fixture.Pool(), nil)
	stage := AtlasStage{
		ItemKey: "plate-1", SHA256: strings.Repeat("a", 64), Method: "tesseract", Engine: "test",
		Frame:     json.RawMessage(`{"renderDpi":200}`),
		Proposals: []json.RawMessage{json.RawMessage(`{"original":"OCR sample","selector":{"pixelRect":[1,2,3,4]}}`)},
	}
	if _, err := service.ImportAtlasStage(fixture.Ctx(), reader.User.ID, source.String(), stage); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-moderator import: %v", err)
	}
	res, err := service.ImportAtlasStage(fixture.Ctx(), moderator.User.ID, source.String(), stage)
	if err != nil || res != 1 {
		t.Fatalf("first import: %d %v", res, err)
	}
	res, err = service.ImportAtlasStage(fixture.Ctx(), moderator.User.ID, source.String(), stage)
	if err != nil || res != 0 {
		t.Fatalf("replay: %d %v", res, err)
	}
	page, err := service.AtlasProposals(fixture.Ctx(), moderator.User.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, proposal := range page.Proposals {
		if proposal.SHA256 == stage.SHA256 {
			found = proposal.ID
		}
	}
	if found == "" {
		t.Fatal("imported proposal absent")
	}
	public := geography.NewService(fixture.Pool())
	result, err := public.SourceOccurrences(fixture.Ctx(), "", geography.OccurrenceInput{Query: "اسم اختبار القراءة"})
	if err != nil || len(result.Occurrences) != 0 {
		t.Fatalf("proposal exposed before review: %+v %v", result, err)
	}
	err = service.DecideAtlasProposal(fixture.Ctx(), moderator.User.ID, found, AtlasProposalDecision{
		Action: "publish", Original: "Reading from sheet", DisplayAR: "اسم اختبار القراءة", Language: "en", NoteAR: "راجعت الرسم",
	})
	if err != nil {
		t.Fatal(err)
	}
	stage.SHA256 = strings.Repeat("b", 64)
	res, err = service.ImportAtlasStage(fixture.Ctx(), moderator.User.ID, source.String(), stage)
	if err != nil || res != 1 {
		t.Fatalf("different scan import: %d %v", res, err)
	}
	result, err = public.SourceOccurrences(fixture.Ctx(), "", geography.OccurrenceInput{Query: "اسم اختبار القراءة"})
	if err != nil || len(result.Occurrences) != 1 || result.Occurrences[0].ScanSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("published old scan after replacement: %+v %v", result, err)
	}
	claim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id,subject_type,subject_id,predicate,object_type,object_id,status,created_by)
		VALUES ($1,'tribe',$2,'mentioned_at','place',$3,'unresolved',$4)`, claim, uuid.New(), uuid.New(), moderator.User.ID)
	receipt := AtlasClaimReceiptInput{ClaimID: claim.String(), NoteAR: "شاهد يوضح السياق"}
	for range 2 {
		if err := service.AttachAtlasClaimReceipt(fixture.Ctx(), moderator.User.ID, result.Occurrences[0].ID, receipt); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	var evidence int
	err = fixture.Pool().QueryRow(fixture.Ctx(), `
		SELECT status, (SELECT count(*) FROM claim_evidence WHERE claim_id=$1 AND relation='contextualizes')
		FROM claims WHERE id=$1`, claim).Scan(&status, &evidence)
	if err != nil || status != "unresolved" || evidence != 1 {
		t.Fatalf("claim status=%s receipt count=%d err=%v", status, evidence, err)
	}

}
