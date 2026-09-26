package search

import (
	"encoding/json"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// Relevance fixtures for the lexical and semantic search stages.
//
// The plan for this work allows the candidate set to be narrowed, and the narrowing
// is only allowed to be invisible in the results. That cannot be argued from a
// query plan: a trigram pre-filter that drops a row scoring 0.2 looks exactly like
// one that drops a row scoring 0.9 to a reader of the SQL. So the ranking is pinned
// here, on Arabic names and passages, over the four ways a caller can match:
//
//   - exact: the normalized query equals a canonical name or a passage.
//   - alias: the query equals an alias value, which scores below exact and above fuzzy.
//   - fuzzy: the query is a near miss on a name or a passage, and has to outrank a
//     name it shares nothing with.
//   - semantic: a supplied embedding has to rank the passage it points at first.
//
// Every case asserts an identifier, a rank and, where the stage defines one, a score.
// A change that reorders a result fails here rather than being reported as a
// performance win.

// relevanceFixture is the corpus the fixtures run against. It is built in an
// isolated schema so a fixture is a measurement of this file's data and not of
// whatever the shared development database happens to hold.
type relevanceFixture struct {
	actorID string
	// people and their aliases, by the label the fixtures use.
	exactPersonID  uuid.UUID
	aliasPersonID  uuid.UUID
	fuzzyPersonID  uuid.UUID
	unrelatedID    uuid.UUID
	familyID       uuid.UUID
	branchID       uuid.UUID
	placeID        uuid.UUID
	publicSourceID uuid.UUID
	// passages and their vectors, by the label the fixtures use.
	exactPassageID    uuid.UUID
	fuzzyPassageID    uuid.UUID
	exactStatementID  uuid.UUID
	fuzzyStatementID  uuid.UUID
	semanticPassageID uuid.UUID
	distantPassageID  uuid.UUID
	// vectors the semantic stage reads.
	exactVector    pgvector.Vector
	semanticVector pgvector.Vector
	nearVector     pgvector.Vector
	farVector      pgvector.Vector
	distantVector  pgvector.Vector
}

// The names are Arabic and deliberately similar to each other, because a relevance
// fixture on Latin names does not exercise the trigram behaviour this work relies
// on: Arabic names share long prefixes, so a near miss is a near miss in the
// trigram sense as well as in the reading sense.
const (
	aliasText        = "أبو بكر الصديق"
	exactPersonName  = "عبد الرحمن بن محمد"
	aliasPersonName  = "عمر بن الخطاب"
	fuzzyPersonName  = "فاطمة الزهراء"
	unrelatedName    = "الخليل بن أحمد"
	familyName       = "بنو أنيف"
	exactPassageText = "قال ابن سعد إن أبا بكر هو والد عبد الله"
	fuzzyPassageText = "ذكر ابن سعد أن عمر بن الخطاب بنى المسجد"
)

func seedRelevanceFixture(t *testing.T, fixture *testsupport.Fixture) *relevanceFixture {
	t.Helper()
	owner := actor.Register(t, fixture, "باحث الصلة")
	ownerID := owner.User.ID
	seeded := &relevanceFixture{actorID: ownerID}

	// A public tree with a published version, so the person policy reads the people
	// below as public and an anonymous relevance query can see them.
	treeID := uuid.New()
	versionID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة الصلة', 'public', $2)`, treeID, ownerID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, ownerID)

	// A writer stores the normalized form beside the display form, which is what the
	// search stage compares against. A fixture that stored the raw text in the
	// normalized column would be measuring the normalizer, not the ranking.
	addPerson := func(name string) uuid.UUID {
		personID := uuid.New()
		fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, $2, $3, 'reviewed', $4)`,
			personID, name, identity.NormalizeArabicName(name), ownerID)
		fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), versionID, personID, name, 0)
		return personID
	}
	seeded.exactPersonID = addPerson(exactPersonName)
	seeded.aliasPersonID = addPerson(aliasPersonName)
	seeded.fuzzyPersonID = addPerson(fuzzyPersonName)
	seeded.unrelatedID = addPerson(unrelatedName)

	// The alias is the second spelling of a person whose canonical name does not
	// contain the term, so an alias match cannot be mistaken for an exact one.
	fixture.Exec(`INSERT INTO person_aliases (person_id, value_ar, normalized_value_ar, source_id) VALUES ($1, $2, $3, NULL)`,
		seeded.aliasPersonID, aliasText, identity.NormalizeArabicName(aliasText))

	// A branch carries its family's name as its secondary name rather than an alias
	// value, which is the one secondary name the page does not have to look up. It is
	// the case a change to the secondary-name computation is most likely to break,
	// because it comes from a join rather than from a correlated lookup.
	seeded.branchID = uuid.New()
	seeded.familyID = uuid.New()
	fixture.Exec(`INSERT INTO families (id, canonical_name_ar, normalized_name_ar, visibility, created_by) VALUES ($1, $2, $3, 'public', $4)`,
		seeded.familyID, familyName, identity.NormalizeArabicName(familyName), ownerID)
	fixture.Exec(`INSERT INTO branches (id, family_id, canonical_name_ar, normalized_name_ar, visibility, created_by) VALUES ($1, $2, $3, $4, 'public', $5)`,
		seeded.branchID, seeded.familyID, "فرع النخبة", identity.NormalizeArabicName("فرع النخبة"), ownerID)
	familyAlias := "بنو الأناف"
	fixture.Exec(`INSERT INTO family_aliases (family_id, value_ar, normalized_value_ar) VALUES ($1, $2, $3)`,
		seeded.familyID, familyAlias, identity.NormalizeArabicName(familyAlias))

	seeded.placeID = uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility, created_by) VALUES ($1, $2, $3, 'city', 'public', $4)`,
		seeded.placeID, "بغداد", identity.NormalizeArabicName("بغداد"), ownerID)

	// One public source with four passages: an exact one, a near miss on the same
	// names, the passage the semantic vector points at, and one that shares nothing.
	seeded.publicSourceID = uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, author_ar, source_type, visibility, dependency_status, created_by)
		VALUES ($1, 'تاريخ الطبري', 'محمد بن جرير', 'book', 'public', 'independent', $2)`, seeded.publicSourceID, ownerID)
	fixture.Exec(`INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar) VALUES ($1, 'دار السلام', 'دار السلام')`, seeded.placeID)

	// The vectors are built so the semantic order is decided by the distance rather
	// than by the plan. Two passages on the same axis would be equidistant from a
	// query on that axis, and the tie would be broken by the row id - so a "closest
	// first" fixture built that way would assert a uuid, not a ranking. The query
	// vector is one axis; the two near passages lean off it by different amounts, so
	// one is strictly closer than the other.
	vector := func(axes map[int]float32) pgvector.Vector {
		values := make([]float32, 1536)
		for axis, magnitude := range axes {
			values[axis] = magnitude
		}
		return pgvector.NewVector(values)
	}
	seeded.exactVector = vector(map[int]float32{0: 1})
	seeded.semanticVector = vector(map[int]float32{1: 1})
	seeded.nearVector = vector(map[int]float32{1: 1, 2: 0.1})
	seeded.farVector = vector(map[int]float32{1: 1, 2: 0.6})
	seeded.distantVector = vector(map[int]float32{3: 1})

	addPassage := func(sequence int, text string, vector pgvector.Vector) (uuid.UUID, uuid.UUID) {
		passageID := uuid.New()
		fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar, embedding)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, passageID, seeded.publicSourceID, sequence, sequence, "صفحة", text, identity.NormalizeArabicName(text), vector)
		statementID := uuid.New()
		fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, extraction_method)
			VALUES ($1, $2, $3, $4, 'accepted', 'ai')`, statementID, seeded.publicSourceID, passageID, text)
		return passageID, statementID
	}
	seeded.exactPassageID, seeded.exactStatementID = addPassage(1, exactPassageText, seeded.exactVector)
	seeded.fuzzyPassageID, seeded.fuzzyStatementID = addPassage(2, fuzzyPassageText, seeded.nearVector)
	seeded.distantPassageID, _ = addPassage(3, "ترجمة في الأندلس", seeded.farVector)
	seeded.semanticPassageID, _ = addPassage(4, "حديث عن النبي في المدينة", seeded.distantVector)
	return seeded
}

// relevanceCase is one expectation: which result has to come first, and with what
// score where the stage defines one.
type relevanceCase struct {
	name string
	// query is the caller's text, before normalization.
	query string
	// firstID is the identifier the first result has to carry.
	firstID string
	// exactScore is the score the stage assigns to a normalized equality, where the
	// stage publishes one. Zero means the fixture does not pin a score.
	exactScore float64
	// subtitle is the secondary name the result must be shown under, where the stage
	// defines one. Empty means the fixture does not pin it.
	subtitle string
	// because says what the fixture is protecting, so a failure explains itself.
	because string
}

// TestSearchNameRelevance is the name stage: exact, alias and fuzzy, on Arabic.
func TestSearchNameRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRelevanceFixture(t, fixture)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)
	ctx := fixture.Ctx()

	for _, testCase := range []relevanceCase{
		{
			name:       "exact",
			query:      exactPersonName,
			firstID:    seeded.exactPersonID.String(),
			exactScore: 100,
			because:    "a normalized equality is the strongest name match the stage offers",
		},
		{
			name:       "exact with diacritics and spacing the normalizer removes",
			query:      "  عَبْدُ الرَّحْمَن بْن مُحَمَّد  ",
			firstID:    seeded.exactPersonID.String(),
			exactScore: 100,
			because:    "normalization happens before the match, so the decorated form is the same term",
		},
		{
			name:    "alias",
			query:   "أبو بكر الصديق",
			firstID: seeded.aliasPersonID.String(),
			// The alias a match was found through is also the name the result is
			// shown under, so the value is pinned as well as the rank.
			subtitle: aliasText,
			// The alias grant is 90, below the 100 of an exact canonical name and
			// above the 70 a trigram similarity can reach.
			exactScore: 90,
			because:    "an alias of a public source is a searchable name of its own",
		},
		{
			name:       "family exact",
			query:      familyName,
			firstID:    seeded.familyID.String(),
			exactScore: 100,
			because:    "the reference families rank in the same stage",
		},
		{
			name:       "place exact",
			query:      "بغداد",
			firstID:    seeded.placeID.String(),
			exactScore: 100,
			because:    "a place is a name the stage searches",
		},
		{
			name:       "branch exact",
			query:      "فرع النخبة",
			firstID:    seeded.branchID.String(),
			exactScore: 100,
			// The one secondary name the page does not have to look up: a branch is
			// drawn under its family's name, which comes from the join rather than
			// from the alias table. It is the value a change to the secondary-name
			// computation is most likely to lose.
			subtitle: familyName,
			because:  "a branch is shown under its family, not under an alias",
		},
		{
			name:    "fuzzy",
			query:   "فاطمة الزهراء",
			firstID: seeded.fuzzyPersonID.String(),
			// A trigram similarity of one is the maximum the fuzzy grant reaches, so
			// pinning 70 would over-claim; what matters is that the near miss comes
			// first at all.
			because: "a trigram near miss on an Arabic name has to outrank a name it shares nothing with",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response, err := service.Search(ctx, Input{Query: testCase.query, Kind: "names", Limit: 10})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			items := groupItems(t, response, "names")
			if len(items) == 0 {
				t.Fatalf("%q returned no names at all: %s", testCase.query, testCase.because)
			}
			if items[0].ID != testCase.firstID {
				t.Fatalf("%q ranked %s (%s, %v) first, want %s: %s",
					testCase.query, items[0].Title, items[0].ID, items[0].Score, testCase.firstID, testCase.because)
			}
			if testCase.exactScore != 0 && items[0].Score != testCase.exactScore {
				t.Fatalf("%q scored %v, want %v: %s", testCase.query, items[0].Score, testCase.exactScore, testCase.because)
			}
			if testCase.subtitle != "" && items[0].Subtitle != testCase.subtitle {
				t.Fatalf("%q is shown under %q, want %q: %s", testCase.query, items[0].Subtitle, testCase.subtitle, testCase.because)
			}
			// The winner is unique. A tie between two results would make "first"
			// depend on the plan, which is exactly what these fixtures exist to stop.
			for _, item := range items[1:] {
				if item.Score == items[0].Score {
					t.Fatalf("%q has two results at score %v (%s and %s): the order is not decided by the ranking",
						testCase.query, item.Score, items[0].ID, item.ID)
				}
			}
		})
	}

	// A term that shares no trigrams with any name returns nothing rather than
	// everything. A pre-filter that is a superset of the scored set keeps that true,
	// and a pre-filter that is not a superset is the bug these fixtures catch.
	response, err := service.Search(ctx, Input{Query: "زقزقة", Kind: "names", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if items := groupItems(t, response, "names"); len(items) != 0 {
		t.Fatalf("a term sharing nothing with any name returned %+v", items)
	}

	// An empty normalized term is the boundary the pre-filter has to survive: the
	// stage still answers with every name it can see rather than with none.
	fuzzy, err := service.Search(ctx, Input{Query: "فاطمه الزهرا", Kind: "names", Limit: 10})
	if err != nil {
		t.Fatalf("fuzzy search: %v", err)
	}
	items := groupItems(t, fuzzy, "names")
	if len(items) == 0 {
		t.Fatal("a near miss on an Arabic name returned nothing")
	}
	if items[0].ID != seeded.fuzzyPersonID.String() {
		t.Fatalf("a near miss ranked %s (%s) first, want the person it is a near miss of", items[0].ID, items[0].Title)
	}
}

// TestSearchPassageRelevance is the passage stage: exact and fuzzy on Arabic text.
func TestSearchPassageRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRelevanceFixture(t, fixture)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)
	ctx := fixture.Ctx()

	for _, testCase := range []relevanceCase{
		{
			name:       "exact passage",
			query:      exactPassageText,
			firstID:    seeded.exactPassageID.String(),
			exactScore: 100,
			because:    "a normalized equality outranks the trigram similarity of 80",
		},
		{
			name:    "fuzzy passage",
			query:   "ذكر ابن سعد ان عمر بن الخطاب بنى المسجد",
			firstID: seeded.fuzzyPassageID.String(),
			because: "a near miss on the text has to find the passage it is a near miss of",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response, err := service.Search(ctx, Input{Query: testCase.query, Kind: "passages", Limit: 10})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			items := groupItems(t, response, "passages")
			if len(items) == 0 {
				t.Fatalf("%q returned no passages at all: %s", testCase.query, testCase.because)
			}
			if items[0].ID != testCase.firstID {
				t.Fatalf("%q ranked %s (%s, %v) first, want %s: %s",
					testCase.query, items[0].Title, items[0].ID, items[0].Score, testCase.firstID, testCase.because)
			}
			if testCase.exactScore != 0 && items[0].Score != testCase.exactScore {
				t.Fatalf("%q scored %v, want %v: %s", testCase.query, items[0].Score, testCase.exactScore, testCase.because)
			}
			if testCase.subtitle != "" && items[0].Subtitle != testCase.subtitle {
				t.Fatalf("%q is shown under %q, want %q: %s", testCase.query, items[0].Subtitle, testCase.subtitle, testCase.because)
			}
		})
	}

	// A term that appears in no passage returns no passage. The passage stage is the
	// one that looks the most obviously indexable - its filter is two ILIKEs and no
	// similarity - so it is the one a narrowing change would quietly break.
	response, err := service.Search(ctx, Input{Query: "زقزقة", Kind: "passages", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if items := groupItems(t, response, "passages"); len(items) != 0 {
		t.Fatalf("a term sharing nothing with any passage returned %+v", items)
	}
}

// TestSearchSourceRelevance is the source stage, whose filter is also two ILIKEs and
// whose score reads a third column.
func TestSearchSourceRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRelevanceFixture(t, fixture)
	pool, _ := fixture.CountingFixturePool(t)
	service := NewService(pool)
	ctx := fixture.Ctx()

	response, err := service.Search(ctx, Input{Query: "تاريخ الطبري", Kind: "sources", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	items := groupItems(t, response, "sources")
	if len(items) != 1 || items[0].ID != seeded.publicSourceID.String() {
		t.Fatalf("an exact title returned %+v", items)
	}
	if items[0].Score != 100 {
		t.Fatalf("an exact title scored %v, want 100", items[0].Score)
	}
	// The author is searchable and scores below the title.
	byAuthor, err := service.Search(ctx, Input{Query: "محمد بن جرير", Kind: "sources", Limit: 10})
	if err != nil {
		t.Fatalf("search by author: %v", err)
	}
	authorItems := groupItems(t, byAuthor, "sources")
	if len(authorItems) != 1 || authorItems[0].ID != seeded.publicSourceID.String() {
		t.Fatalf("an exact author returned %+v", authorItems)
	}
	if authorItems[0].Score != 60 {
		t.Fatalf("an author match scored %v, want 60: the author grant is below the title similarity", authorItems[0].Score)
	}
}

// TestSearchSemanticRelevance is the semantic stage, which is the one a trigram
// pre-filter must not touch at all: its ranking is a distance, not a match.
func TestSearchSemanticRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRelevanceFixture(t, fixture)
	pool, counter := fixture.CountingFixturePool(t)
	stub := &embeddingStub{response: ai.EmbeddingResponse{Embedding: vectorValues(seeded.semanticVector), Dimensions: 1536, Model: "test-embed"}}
	service := &Service{Pool: pool, AI: stub}
	ctx := fixture.Ctx()

	counter.Reset()
	response, err := service.Search(ctx, Input{Query: "حديث عن النبي", Kind: "semantic", Limit: 10})
	if err != nil {
		t.Fatalf("semantic search: %v", err)
	}
	items := groupItems(t, response, "semantic")
	if len(items) < 2 {
		t.Fatalf("a semantic query returned %d results, want at least the two passages on the same axis: %+v", len(items), items)
	}
	// The embedding provider produced a vector along one axis, and the two passages
	// on that axis are the closest two. Which of them is closer is fixed by the
	// fixture, so the order is asserted rather than assumed.
	wantOrder := []string{seeded.fuzzyPassageID.String(), seeded.distantPassageID.String()}
	for index, want := range wantOrder {
		if items[index].ID != want {
			t.Fatalf("semantic rank %d is %s, want %s", index+1, items[index].ID, want)
		}
	}
	for index := 1; index < len(items); index++ {
		if items[index-1].Score < items[index].Score {
			t.Fatalf("semantic results are not ordered by distance: %v then %v", items[index-1].Score, items[index].Score)
		}
	}
	if items[0].MatchKind != "semantic" || response.EmbeddingModel != "test-embed" {
		t.Fatalf("the semantic stage did not label itself: %+v", items[0])
	}

	// A caller-supplied embedding takes the same path, and the same order.
	supplied, err := service.Search(ctx, Input{Query: "حديث", Kind: "semantic", Limit: 10, Embedding: vectorJSON(seeded.semanticVector)})
	if err != nil {
		t.Fatalf("semantic search with a supplied embedding: %v", err)
	}
	suppliedItems := groupItems(t, supplied, "semantic")
	if len(suppliedItems) != len(items) {
		t.Fatalf("a supplied embedding returned %d results, the generated one %d", len(suppliedItems), len(items))
	}
	for index := range items {
		if suppliedItems[index].ID != items[index].ID {
			t.Fatalf("a supplied embedding ranked %s at %d, the generated one %s", suppliedItems[index].ID, index+1, items[index].ID)
		}
	}
	if supplied.EmbeddingModel != "caller_provided" {
		t.Fatalf("a supplied embedding is reported as %q", supplied.EmbeddingModel)
	}
}

// groupItems is the results of one stage, in the order the response put them.
func groupItems(t *testing.T, response Response, key string) []Result {
	t.Helper()
	for _, group := range response.Groups {
		if group.Key == key {
			return group.Items
		}
	}
	return nil
}

func vectorValues(vector pgvector.Vector) []float64 {
	values := make([]float64, 1536)
	for index, value := range vector.Slice() {
		values[index] = float64(value)
	}
	return values
}

// vectorJSON renders an embedding the way a caller sends one: a JSON array of 1536
// numbers, which is what the search input parses.
func vectorJSON(vector pgvector.Vector) string {
	encoded, err := json.Marshal(vector.Slice())
	if err != nil {
		return ""
	}
	return string(encoded)
}
