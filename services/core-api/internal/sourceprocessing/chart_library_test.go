package sourceprocessing

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
)

func TestChartLibraryKeepsUploadedWorkAndProtectsPrivateReviewData(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	ctx := fixture.Ctx()
	fixture.Exec(`UPDATE sources SET visibility='private' WHERE id=$1`, sourceID)
	for index, status := range []string{"suggested", "deferred", "accepted", "corrected", "rejected", "suggested"} {
		fixture.Exec(`INSERT INTO source_tree_suggestions(source_id,source_file_id,occurrence_id,page_number,region,reading_method,reading_model,status,superseded_at)
			VALUES($1,$2,$3,1,'{"x":0.1,"y":0.1,"width":0.1,"height":0.1}','fixture','fixture',$4,CASE WHEN $5 THEN now() ELSE NULL END)`, sourceID, fileID, fixture.Unique("occurrence"), status, index == 5)
	}
	ownerItems, err := service.ListChartLibrary(ctx, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	var found *ChartLibraryItem
	for index := range ownerItems {
		if ownerItems[index].FileID == fileID {
			found = &ownerItems[index]
		}
	}
	if found == nil || !found.CanReview || found.TotalNames != 5 || found.PendingNames != 1 || found.DeferredNames != 1 || found.ReviewedNames != 3 {
		t.Fatalf("uploaded chart and active review counts = %+v", found)
	}
	other := actor.RegisterAndLogin(t, fixture, "زائر المكتبة")
	for _, viewer := range []string{"", other.User.ID} {
		items, err := service.ListChartLibrary(ctx, viewer)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.SourceID == sourceID {
				t.Fatalf("private chart leaked to viewer %q: %+v", viewer, item)
			}
		}
	}
	fixture.Exec(`UPDATE sources SET visibility='public' WHERE id=$1`, sourceID)
	publicItems, err := service.ListChartLibrary(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	foundPublic := false
	for _, item := range publicItems {
		if item.SourceID == sourceID {
			foundPublic = true
			if item.CanReview || item.FileID != "" || item.DraftID != "" || item.TotalNames != 0 || item.PendingNames != 0 {
				t.Fatalf("public source title exposed private file/review metadata: %+v", item)
			}
		}
	}
	if !foundPublic {
		t.Fatal("public uploaded chart disappeared from the library")
	}
}

func TestChartLibraryIncludesSourcesWithoutFiles(t *testing.T) {
	service, fixture, sourceID, fileID, ownerID := suggestionsFixture(t)
	fixture.Exec(`DELETE FROM source_files WHERE id=$1`, fileID)
	items, err := service.ListChartLibrary(fixture.Ctx(), ownerID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.SourceID == sourceID {
			if item.FileID != "" || !item.CanReview {
				t.Fatalf("source awaiting upload = %+v", item)
			}
			return
		}
	}
	t.Fatal("a saved chart without a file disappeared")
}
