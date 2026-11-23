package sourceprocessing

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestReviewedNameCarriesTheTribeNisbaCardAsASuggestionOnly runs the نِسْبَة
// hook end to end over the real reading pipeline: the machine reads the crop,
// the review context carries the كُنية card, and NOTHING is written — the
// card is a link into the dossier, and the decision ledger's schema and rows
// are exactly what the reviewer's own actions leave behind (plan 031-F: no
// migration, no assertion of membership).
func TestReviewedNameCarriesTheTribeNisbaCardAsASuggestionOnly(t *testing.T) {
	fixture := newNameRunFixture(t)
	f := fixture.f
	owner := fixture.actor.User.ID

	// Two tribes in the dictionary: حرب (with an alias whose last token is the
	// root) and عتيبة (the canonical's ta-marbuta form, matched through the
	// nisba's root). Their fixture-schema rows are this test's own.
	harb := uuid.New()
	f.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'حرب', 'حرب', 'public', $2)`, harb, owner)
	f.Exec(`INSERT INTO tribe_aliases (tribe_id, value_ar, normalized_value_ar) VALUES ($1, 'بني حرب', 'بني حرب')`, harb)
	otaiba := uuid.New()
	f.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'عتيبة', 'عتيبه', 'public', $2)`, otaiba, owner)

	// One more region so the pipeline reads three names: a nisba of حرب, a
	// nisba of عتيبة (the ta-marbuta path), and a name carrying no nisba the
	// dictionary knows.
	fixture.service.DetectNameTile = func([]byte, CarrierLocaliserConfig) ([]PageRegion, error) {
		fixture.detectorCalls.Add(1)
		return []PageRegion{
			{Kind: "name_group", X: .05, Y: .2, Width: .25, Height: .1},
			{Kind: "name_group", X: .4, Y: .2, Width: .25, Height: .1},
			{Kind: "name_group", X: .75, Y: .2, Width: .2, Height: .1},
		}, nil
	}
	readings := map[int]string{
		1: "سعيد الحربي",
		2: "عبدالله العتيبي",
		3: "سعيد بن سعيد",
	}
	fixture.reader.read = func(_ context.Context, index int) (LocalCropReading, error) {
		text := readings[index]
		return LocalCropReading{Outcome: "readable", Text: text, Alternatives: []string{}, Variants: []LocalVariantReading{{Variant: "fixture", Outcome: "readable", RawText: text}}, Agreement: 1}, nil
	}

	fixture.prepare(t)
	fixture.process(t, fixture.claim(t, "local-reader"))
	queue, err := fixture.service.ListSuggestions(f.Ctx(), fixture.fileID, owner, "", 10, 0)
	if err != nil || len(queue) != 3 {
		t.Fatalf("queue: %v %d", err, len(queue))
	}

	// Anchor the cards on the READINGS themselves (the pipeline's suggestion
	// order is its own), then pin the trio of outcomes: الحربي → حرب,
	// العتيبي → عتيبة through the ta-marbuta root, and a name with no
	// dictionary nisba → no card at all.
	expectations := map[string]TribalNisbaSuggestion{
		"سعيد الحربي":     {TribeName: "حرب", NisbaForm: "الحربي"},
		"عبدالله العتيبي": {TribeName: "عتيبة", NisbaForm: "العتيبي"},
	}
	var harbSuggestionID string
	for _, suggestion := range queue {
		context, err := fixture.service.BeginNameReview(f.Ctx(), suggestion.ID, owner)
		if err != nil {
			t.Fatalf("begin review %s: %v", suggestion.ID, err)
		}
		card := context.TribalSuggestion
		if context.Reading == "سعيد بن سعيد" {
			if card != nil {
				t.Fatalf("the name with no dictionary nisba carried a card: %+v", card)
			}
			continue
		}
		want, known := expectations[context.Reading]
		if !known {
			t.Fatalf("unexpected reading %q from the fixture", context.Reading)
		}
		if card == nil {
			t.Fatalf("the reading %q carries no tribe card", context.Reading)
		}
		if card.TribeName != want.TribeName || card.NisbaForm != want.NisbaForm {
			t.Fatalf("the reading %q card = %+v, want %s via %s", context.Reading, card, want.TribeName, want.NisbaForm)
		}
		if context.Reading == "سعيد الحربي" {
			harbSuggestionID = suggestion.ID
		}
	}
	if harbSuggestionID == "" {
		t.Fatalf("no suggestion carried the الحربي reading")
	}

	// SUGGESTION-ONLY, pinned twice: no decision exists for any of the three
	// suggestions (the card writes nothing, the ledger stays the reviewer's),
	// and the dictionary grew by exactly the rows this test planted.
	if decisions := f.Count(`SELECT count(*) FROM source_tree_suggestion_decisions`); decisions != 0 {
		t.Fatalf("the hook recorded %d decisions; it must record none", decisions)
	}
	if tribes := f.Count(`SELECT count(*) FROM tribes WHERE id IN ($1, $2)`, harb, otaiba); tribes != 2 {
		t.Fatalf("the dictionary grew by %d unexpected rows", tribes)
	}

	// The CACHED path keeps the card alive: the second BeginNameReview reads
	// the stored exposure and STILL recomputes the suggestion — a card is
	// never frozen into a stored context, so a tribe added after the first
	// read still earns its card.
	again, err := fixture.service.BeginNameReview(f.Ctx(), harbSuggestionID, owner)
	if err != nil {
		t.Fatalf("re-begin (cached): %v", err)
	}
	if again.TribalSuggestion == nil || again.TribalSuggestion.TribeName != "حرب" {
		t.Fatalf("the cached context lost the كُنية card: %+v", again.TribalSuggestion)
	}
}
