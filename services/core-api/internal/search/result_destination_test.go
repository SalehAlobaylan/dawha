package search

import (
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
	"testing"
)

func TestPassageSearchKeepsItsSourceDestination(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, passageID := uuid.New().String(), uuid.New().String()
	text := "نسب " + fixture.Tag()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility) VALUES ($1, 'مصدر الوجهة', 'book', 'public')`, sourceID)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar, locator_ar) VALUES ($1, $2, 1, $3, $3, 'ص ١٢')`, passageID, sourceID, text)
	response, err := NewService(fixture.Pool()).Search(fixture.Ctx(), Input{Query: text, Kind: "passages", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, group := range response.Groups {
		for _, item := range group.Items {
			if item.ID != passageID {
				continue
			}
			found = true
			if item.SourceID != sourceID || item.PassageID != passageID || item.LocatorAR != "ص ١٢" {
				t.Fatalf("result lost its destination: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("new passage did not appear")
	}
	fixture.Exec(`UPDATE sources SET visibility = 'private' WHERE id = $1`, sourceID)
	response, err = NewService(fixture.Pool()).Search(fixture.Ctx(), Input{Query: text, Kind: "passages", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range response.Groups {
		for _, item := range group.Items {
			if item.ID == passageID {
				t.Fatal("destination metadata bypassed source visibility")
			}
		}
	}
}
