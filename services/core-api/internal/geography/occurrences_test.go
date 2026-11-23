package geography

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

func TestSourceOccurrencesKeepLiteralFormsAndPageOnlyVisibleResults(t *testing.T) {
	fixture := testsupport.New(t)
	public, private, hidden := uuid.New(), uuid.New(), uuid.New()
	for _, sample := range []struct {
		sourceID   uuid.UUID
		visibility string
		count      int
		published  bool
	}{{public, "public", 21, true}, {private, "private", 1, true}, {hidden, "public", 1, false}} {
		item, revision := uuid.New(), uuid.New()
		fixture.Exec(`INSERT INTO sources (id,title_ar,source_type,visibility)
			VALUES ($1,'اختبار البحث المكاني','book',$2)`, sample.sourceID, sample.visibility)
		fixture.Exec(`INSERT INTO atlas_source_items (id,source_id,item_key) VALUES ($1,$2,'page-1')`, item, sample.sourceID)
		fixture.Exec(`INSERT INTO atlas_source_item_revisions
			(id,item_id,revision_key,locator_ar,external_url,rights_ar,credit_ar)
			VALUES ($1,$2,'v1','ص ١','https://example.org/page','رابط','المؤلف')`, revision, item)
		fixture.Exec(`INSERT INTO atlas_source_occurrences
			(item_revision_id,occurrence_key,original_text,display_ar,language,reading_method,published)
			SELECT $1, n::text, 'بُرَيه اختبار', 'برية اختبار', 'ar', 'original', $3
			FROM generate_series(1,$2::integer) n`, revision, sample.count, sample.published)
	}

	service := NewService(fixture.Pool())
	input := OccurrenceInput{Query: "برية اختبار"}
	first, err := service.SourceOccurrences(fixture.Ctx(), "", input)
	if err != nil || len(first.Occurrences) != 20 || first.Next == "" {
		t.Fatalf("first page = %d, next=%q, err=%v", len(first.Occurrences), first.Next, err)
	}
	input.After = first.Next
	second, err := service.SourceOccurrences(fixture.Ctx(), "", input)
	if err != nil || len(second.Occurrences) != 1 || second.Next != "" {
		t.Fatalf("second page = %d, next=%q, err=%v", len(second.Occurrences), second.Next, err)
	}
	for _, row := range append(first.Occurrences, second.Occurrences...) {
		if row.Original != "بُرَيه اختبار" || row.URL != "https://example.org/page" {
			t.Fatalf("incorrect original or locator: %+v", row)
		}
	}
	var hiddenID string
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `SELECT o.id::text FROM atlas_source_occurrences o
		JOIN atlas_source_item_revisions r ON o.item_revision_id=r.id
		JOIN atlas_source_items i ON r.item_id=i.id WHERE i.source_id=$1`, private).Scan(&hiddenID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SourceOccurrences(fixture.Ctx(), "", OccurrenceInput{ID: hiddenID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private direct read: %v", err)
	}

	from := 1900
	dated, err := service.SourceOccurrences(fixture.Ctx(), "", OccurrenceInput{Query: input.Query, From: &from})
	if err != nil || len(dated.Occurrences) != 0 {
		t.Fatalf("undated records appeared in dated search: %+v %v", dated, err)
	}
}

func TestAtlasRelationshipRequiresBothSourcesReadable(t *testing.T) {
	fixture := testsupport.New(t)
	a, b, hidden := uuid.New(), uuid.New(), uuid.New()
	fixture.Exec(`INSERT INTO sources (id,title_ar,source_type,visibility)
		VALUES ($1,'الأصل','book','public'), ($2,'نسخة من الأصل','book','public'), ($3,'نسخة محجوبة','book','private')`, a, b, hidden)
	fixture.Exec(`INSERT INTO source_dependencies (source_id,depends_on_source_id,dependency_type,status)
		VALUES ($2,$1,'derived_from','confirmed'), ($3,$1,'derived_from','confirmed')`, a, b, hidden)
	service := NewService(fixture.Pool())
	pair, err := service.AtlasSourceRelationship(fixture.Ctx(), "", a.String(), b.String())
	if err != nil || pair.Status != "confirmed" || pair.Kind != "derived_from" {
		t.Fatalf("reverse dependency read: %+v %v", pair, err)
	}
	pair, err = service.AtlasSourceRelationship(fixture.Ctx(), "", a.String(), a.String())
	if err != nil || pair.Status != "same_source" {
		t.Fatalf("same source read: %+v %v", pair, err)
	}
	if _, err := service.AtlasSourceRelationship(fixture.Ctx(), "", a.String(), hidden.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private source read: %v", err)
	}
}
