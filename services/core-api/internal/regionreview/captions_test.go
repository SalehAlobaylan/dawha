package regionreview

import (
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

func TestCaptionDraftIsPrivateUntilPublished(t *testing.T) {
	fixture := testsupport.New(t)
	moderator := actor.Register(t, fixture, "مراجع")
	fixture.GrantRole(moderator.User.ID, "moderator")
	sum := strings.Repeat("a", 64)
	sourceID, captionID := uuid.New(), uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'لوحة', 'archive_record', 'public',
		    jsonb_build_object('plate_key', 'caption-test', 'georef', '{}'::jsonb, 'file_sha256', $2::text),
		    $3)`, sourceID, sum, moderator.User.ID)
	fixture.Exec(`INSERT INTO plate_captions(id, source_id, scan_sha256, x, y, printed_name, name_ar)
		VALUES ($1, $2, $3, 24, 56, 'BARRIYAH', 'برية')`, captionID, sourceID, sum)

	ctx := fixture.Ctx()
	review := NewService(fixture.Pool(), nil)
	atlas := geography.NewService(fixture.Pool())
	assertPublicName := func(want string) {
		t.Helper()
		register, err := atlas.PlateRegister(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, plate := range register.Plates {
			if plate.PlateKey == "caption-test" {
				if len(plate.Captions) != 1 || plate.Captions[0].NameAR != want {
					t.Fatalf("captions = %+v; want %q", plate.Captions, want)
				}
				return
			}
		}
		t.Fatal("caption test plate not found")
	}
	if err := review.ProposeCaption(ctx, moderator.User.ID, captionID.String(), "بريه"); err != nil {
		t.Fatal(err)
	}
	assertPublicName("برية")
	history, err := review.CaptionHistory(ctx, moderator.User.ID, captionID.String())
	if err != nil || len(history) != 1 || history[0].Published {
		t.Fatalf("draft history = %+v, err = %v", history, err)
	}
	if err := review.PublishCaption(ctx, moderator.User.ID, captionID.String(), history[0].ID); err != nil {
		t.Fatal(err)
	}
	assertPublicName("بريه")
}
