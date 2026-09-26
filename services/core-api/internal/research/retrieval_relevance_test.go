package research

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// Relevance fixtures for the research retrieval stages.
//
// These are the stages the lexical candidate work touches: passage retrieval, claim
// retrieval and tree interpretation, each of which scores a candidate set and keeps
// the best of it. The fixtures pin what the stages return for an Arabic term that
// matches exactly, one that matches only through a similarity, and one that matches
// nothing, because a candidate-bound narrowing that is not invisible in the results
// is a change to what the platform asserts, not an optimization.
//
// The rule the stages are built on, and the reason the exact case is worth pinning
// separately: a stage keeps a candidate when its score is greater than zero, and a
// similarity of zero is not a match. A pre-filter built on a trigram operator is
// stronger than "greater than zero" - pg_trgm's operators are defined at a
// threshold of 0.3 - so a narrowing that used one and nothing else would drop the
// candidates between zero and the threshold. The "faint match" case below is that
// band, pinned so it cannot disappear quietly.

const (
	relevancePassageExact   = "قال ابن سعد إن أبا بكر هو والد عبد الله"
	relevancePassageFaint   = "ذكر المهاجر أبو بكر في مكة قبل الهجرة إلى المدينة"
	relevancePassageOther   = "ترجمة في الأندلس relating to the siege of Córdoba"
	relevanceClaimPredicate = "مُولَف"
	relevanceClaimNotes     = "ذكر ابن عمر أن النبي صلى الله عليه وسلم قال في الهجرة"
)

// retrievalRelevance is the corpus the retrieval fixtures run against, built in an
// isolated schema so a fixture measures its own data.
type retrievalRelevance struct {
	actorID        string
	exactPassageID uuid.UUID
	faintPassageID uuid.UUID
	otherPassageID uuid.UUID
	claimID        uuid.UUID
	exactTreeID    uuid.UUID
	versionID      uuid.UUID
	personID       uuid.UUID
	otherPersonID  uuid.UUID
	sourceID       uuid.UUID
}

func seedRetrievalRelevance(t *testing.T, fixture *testsupport.Fixture) *retrievalRelevance {
	t.Helper()
	owner := actor.Register(t, fixture, "باحث الاسترجاع")
	ownerID := owner.User.ID
	seeded := &retrievalRelevance{actorID: ownerID}

	sourceID := uuid.New()
	seeded.sourceID = sourceID
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'تاريخ الطبري', 'book', 'public', $2)`, sourceID, ownerID)

	addPassage := func(sequence int, text string) (uuid.UUID, uuid.UUID) {
		passageID := uuid.New()
		fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $2, $3, $4, $5)`,
			passageID, sourceID, sequence, text, identity.NormalizeArabicName(text))
		statementID := uuid.New()
		fixture.Exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, extraction_method) VALUES ($1, $2, $3, $4, 'accepted', 'ai')`,
			statementID, sourceID, passageID, text)
		return passageID, statementID
	}
	exactPassageID, exactStatementID := addPassage(1, relevancePassageExact)
	seeded.exactPassageID = exactPassageID
	seeded.faintPassageID, _ = addPassage(2, relevancePassageFaint)
	seeded.otherPassageID, _ = addPassage(3, relevancePassageOther)

	// The person the claim and the relationship are about. It is created before both
	// because a claim's subject and a tree node's person are foreign keys, and a
	// fixture that invented the ids would fail on the constraint rather than on the
	// ranking.
	seeded.personID = uuid.New()
	otherPersonID := uuid.New()
	for _, person := range []struct {
		id   uuid.UUID
		name string
	}{{seeded.personID, "أبو بكر الصديق"}, {otherPersonID, "عمر بن الخطاب"}} {
		fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, $2, $3, 'reviewed', $4)`,
			person.id, person.name, identity.NormalizeArabicName(person.name), ownerID)
	}

	// A claim resting on the exact passage, so the claim stage has something visible
	// to score and its evidence rule is the published one.
	seeded.claimID = uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
		VALUES ($1, 'person', $2, $3, 'person', $4, 'supported', $5, $6)`,
		seeded.claimID, seeded.personID, relevanceClaimPredicate, otherPersonID, relevanceClaimNotes, ownerID)
	fixture.Exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, source_passage_id, relation) VALUES ($1, $2, $3, 'supports')`,
		seeded.claimID, exactStatementID, exactPassageID)

	seeded.otherPersonID = otherPersonID

	// A published public tree with a person in it, so the tree stage has a
	// relationship whose subject name is a searchable term.
	seeded.exactTreeID = uuid.New()
	seeded.versionID = uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة الاسترجاع', 'public', $2)`, seeded.exactTreeID, ownerID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`,
		seeded.versionID, seeded.exactTreeID, ownerID)
	subjectNodeID := uuid.New()
	objectNodeID := uuid.New()
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES
		($1, $2, $3, 'أبو بكر الصديق', 0), ($4, $2, $5, 'عمر بن الخطاب', 1)`,
		subjectNodeID, seeded.versionID, seeded.personID, objectNodeID, otherPersonID)
	fixture.Exec(`INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, created_by)
		VALUES ($1, $2, $3, $4, 'sibling_of', 'interpreted', $5)`, uuid.New(), seeded.versionID, subjectNodeID, objectNodeID, ownerID)
	return seeded
}

// TestRetrieveLexicalPassagesRelevance pins the passage stage over the band a
// trigram pre-filter is most likely to lose: a candidate whose similarity is above
// zero but below pg_trgm's operator threshold.
func TestRetrieveLexicalPassagesRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRetrievalRelevance(t, fixture)
	service := &Service{Pool: fixture.Pool()}
	ctx := fixture.Ctx()

	exact, err := service.retrieveLexicalPassages(ctx, retrievalContext{
		Input:      QueryInput{Question: relevancePassageExact},
		Normalized: identity.NormalizeArabicName(relevancePassageExact),
	})
	if err != nil {
		t.Fatalf("retrieve passages: %v", err)
	}
	if len(exact) == 0 {
		t.Fatal("an exact term returned no passage: the stage keeps a score above zero")
	}
	if exact[0].PassageID != seeded.exactPassageID.String() {
		t.Fatalf("an exact term ranked %s first, want %s", exact[0].PassageID, seeded.exactPassageID)
	}
	if exact[0].Score.Lexical < 1.0 {
		t.Fatalf("an exact term scored %v, want 1.0: a normalized equality is the stage's maximum", exact[0].Score.Lexical)
	}
	if exact[0].Layer != SourceStatement || exact[0].ReviewStatus != "accepted" {
		t.Fatalf("the retrieved passage lost its provenance: %+v", exact[0])
	}
	// Every candidate is a distinct passage, and the order is decided by the score.
	seen := map[string]struct{}{}
	for index, citation := range exact {
		if _, duplicate := seen[citation.PassageID]; duplicate {
			t.Fatalf("passage %s was returned twice", citation.PassageID)
		}
		seen[citation.PassageID] = struct{}{}
		if index > 0 && exact[index-1].Score.Lexical < citation.Score.Lexical {
			t.Fatalf("passages are not ordered by score: %v then %v", exact[index-1].Score.Lexical, citation.Score.Lexical)
		}
	}

	// A term that appears in no passage returns nothing. A pre-filter that is not a
	// superset of the scored set shows up here as a result for a term that matched
	// nothing.
	none, err := service.retrieveLexicalPassages(ctx, retrievalContext{
		Input:      QueryInput{Question: "زقزقة"},
		Normalized: "زقزقة",
	})
	if err != nil {
		t.Fatalf("retrieve passages: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("a term sharing nothing with any passage returned %d candidates: %+v", len(none), none)
	}

	// A term that is a fragment of one passage keeps that passage, at a similarity
	// below pg_trgm's 0.3 operator threshold if the fragment is short enough. This is
	// the band the narrowing has to preserve, so the fixture states which band it is
	// in rather than leaving it to a reader to work out.
	fragment := "أبو بكر"
	band, err := service.retrieveLexicalPassages(ctx, retrievalContext{
		Input:      QueryInput{Question: fragment},
		Normalized: identity.NormalizeArabicName(fragment),
	})
	if err != nil {
		t.Fatalf("retrieve passages: %v", err)
	}
	var faint float64
	for _, citation := range band {
		if citation.PassageID == seeded.faintPassageID.String() {
			faint = citation.Score.Lexical
		}
	}
	t.Logf("RELEVANCE fragment=%q faintCandidateScore=%.4f candidates=%d", fragment, faint, len(band))
	if faint == 0 {
		t.Fatalf("a passage that mentions the term is no longer a candidate: %+v", band)
	}
}

// TestRetrieveClaimsRelevance pins the claim stage, whose filter mixes two ILIKEs
// with a similarity and a word similarity. A pre-filter there has to cover all four.
func TestRetrieveClaimsRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRetrievalRelevance(t, fixture)
	service := &Service{Pool: fixture.Pool()}
	ctx := fixture.Ctx()

	// The stage is scoped to the fixture's own source. The claim stage reads claims
	// through their visible evidence, so an unscoped query would also answer with
	// the seeded demo claims, and "ranked first" would then be a statement about
	// somebody else's data. Scoping keeps the fixture a measurement of this file.
	scoped := QueryInput{Question: relevanceClaimPredicate, SourceID: seeded.sourceID.String()}

	// The predicate is the term, and it is found through the trigram similarity the
	// stage scores with.
	byPredicate, err := service.retrieveClaims(ctx, retrievalContext{
		Input:      scoped,
		Normalized: relevanceClaimPredicate,
	})
	if err != nil {
		t.Fatalf("retrieve claims: %v", err)
	}
	if len(byPredicate) == 0 {
		t.Fatal("the claim whose predicate is the term returned nothing")
	}
	if byPredicate[0].ID != seeded.claimID.String() {
		t.Fatalf("the claim stage ranked %s first, want %s", byPredicate[0].ID, seeded.claimID)
	}
	if byPredicate[0].ReviewStatus != "accepted" {
		t.Fatalf("the retrieved claim lost its review status: %+v", byPredicate[0])
	}

	// The term is found through the notes as well, which is a different branch of the
	// same filter and the one a predicate-only pre-filter would lose.
	noteFragment := "المهاجر"
	scoped.Question = noteFragment
	byNotes, err := service.retrieveClaims(ctx, retrievalContext{
		Input:      scoped,
		Normalized: noteFragment,
	})
	if err != nil {
		t.Fatalf("retrieve claims: %v", err)
	}
	if len(byNotes) == 0 {
		t.Fatal("a term that appears only in the notes returned no claim: the notes branch of the filter was lost")
	}
	if byNotes[0].ID != seeded.claimID.String() {
		t.Fatalf("the notes branch ranked %s first, want %s", byNotes[0].ID, seeded.claimID)
	}

	scoped.Question = "زقزقة"
	none, err := service.retrieveClaims(ctx, retrievalContext{Input: scoped, Normalized: "زقزقة"})
	if err != nil {
		t.Fatalf("retrieve claims: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("a term sharing nothing with any claim returned %d: %+v", len(none), none)
	}
}

// TestRetrieveTreeInterpretationsRelevance pins the tree stage, whose filter is two
// ILIKEs and two word similarities over a join to a published version.
func TestRetrieveTreeInterpretationsRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRetrievalRelevance(t, fixture)
	service := &Service{Pool: fixture.Pool()}
	ctx := fixture.Ctx()

	found, err := service.retrieveTreeInterpretations(ctx, retrievalContext{
		Input:      QueryInput{Question: "أبو بكر الصديق"},
		Normalized: identity.NormalizeArabicName("أبو بكر الصديق"),
	})
	if err != nil {
		t.Fatalf("retrieve tree interpretations: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("a published relationship whose subject is the term returned nothing")
	}
	if found[0].Status != "interpreted" {
		t.Fatalf("the retrieved relationship lost its status: %+v", found[0])
	}
	// A relationship in a draft version is not public, and the stage joins to
	// published versions only. Pinning that keeps a narrowing from dropping the join.
	draftVersionID := uuid.New()
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 2, 'draft', $3)`,
		draftVersionID, seeded.exactTreeID, seeded.actorID)
	subjectNodeID := uuid.New()
	objectNodeID := uuid.New()
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES
		($1, $2, $3, 'أبو بكر الصديق', 0), ($4, $2, $5, 'عمر بن الخطاب', 1)`,
		subjectNodeID, draftVersionID, seeded.personID, objectNodeID, seeded.otherPersonID)
	draftRelationshipID := uuid.New()
	fixture.Exec(`INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, created_by)
		VALUES ($1, $2, $3, $4, 'sibling_of', 'interpreted', $5)`, draftRelationshipID, draftVersionID, subjectNodeID, objectNodeID, seeded.actorID)

	after, err := service.retrieveTreeInterpretations(ctx, retrievalContext{
		Input:      QueryInput{Question: "أبو بكر الصديق"},
		Normalized: identity.NormalizeArabicName("أبو بكر الصديق"),
	})
	if err != nil {
		t.Fatalf("retrieve tree interpretations: %v", err)
	}
	for _, citation := range after {
		if citation.ID == draftRelationshipID.String() {
			t.Fatalf("a relationship in a draft version reached the stage: %+v", citation)
		}
	}
	if len(after) != len(found) {
		t.Fatalf("adding a draft changed the published result count: %d then %d", len(found), len(after))
	}
}

// TestRetrieveFindingsAndQuestionsRelevance pins the two open-ended stages, which
// take a term and return whatever the platform has not closed.
func TestRetrieveFindingsAndQuestionsRelevance(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedRetrievalRelevance(t, fixture)
	service := &Service{Pool: fixture.Pool()}
	ctx := fixture.Ctx()

	// A dismissed finding and an archived question carry the same term, so the two
	// stages have to keep excluding what the platform closed.
	findingID := uuid.New()
	fixture.Exec(`INSERT INTO platform_findings (id, title_ar, explanation_ar, finding_type, status) VALUES ($1, 'تعارض في رواية نسب الزمرّد', 'تعارض بين موضعين', 'contradiction', 'needs_review')`, findingID)
	dismissedFindingID := uuid.New()
	fixture.Exec(`INSERT INTO platform_findings (id, title_ar, explanation_ar, finding_type, status) VALUES ($1, 'مسألة مغلقة عن الزمرّد', 'أُغلقت', 'contradiction', 'dismissed')`, dismissedFindingID)
	questionID := uuid.New()
	fixture.Exec(`INSERT INTO open_questions (id, title_ar, description_ar, status, created_by) VALUES ($1, 'من هو صاحب الزمرّد؟', 'سؤال عن الزمرّد', 'open', $2)`, questionID, seeded.actorID)
	archivedQuestionID := uuid.New()
	fixture.Exec(`INSERT INTO open_questions (id, title_ar, description_ar, status, created_by) VALUES ($1, 'صاحب الزمرّد مؤرشف', 'أُرشف', 'archived', $2)`, archivedQuestionID, seeded.actorID)

	// The term appears only in the rows this fixture created. The findings and
	// questions stages take no source filter, and their word-similarity threshold is
	// low enough that a long unrelated explanation can match an Arabic term, so
	// these stages answer with the seeded demo rows too. The fixture therefore
	// asserts what it owns: that its own row is returned, and that what the platform
	// closed is not.
	term := "الزمرّد"
	normalized := identity.NormalizeArabicName(term)
	findings, err := service.retrieveFindings(ctx, retrievalContext{Input: QueryInput{Question: term}, Normalized: normalized})
	if err != nil {
		t.Fatalf("retrieve findings: %v", err)
	}
	seenFinding := false
	for _, citation := range findings {
		if citation.ID == dismissedFindingID.String() {
			t.Fatalf("a dismissed finding reached the stage: %+v", citation)
		}
		if citation.ID != findingID.String() {
			continue
		}
		seenFinding = true
		if citation.Layer != PlatformFinding || citation.FindingID != findingID.String() {
			t.Fatalf("the finding lost its identity: %+v", citation)
		}
	}
	if !seenFinding {
		t.Fatalf("the open finding about %q was not returned: %+v", term, findings)
	}

	questions, err := service.retrieveQuestions(ctx, retrievalContext{Input: QueryInput{Question: term}, Normalized: normalized})
	if err != nil {
		t.Fatalf("retrieve questions: %v", err)
	}
	seenQuestion := false
	for _, citation := range questions {
		if citation.ID == archivedQuestionID.String() {
			t.Fatalf("an archived question reached the stage: %+v", citation)
		}
		if citation.ID != questionID.String() {
			continue
		}
		seenQuestion = true
		if citation.Layer != OpenQuestion || citation.QuestionID != questionID.String() {
			t.Fatalf("the question lost its identity: %+v", citation)
		}
	}
	if !seenQuestion {
		t.Fatalf("the open question about %q was not returned: %+v", term, questions)
	}

	none, err := service.retrieveFindings(ctx, retrievalContext{Input: QueryInput{Question: "زقزقة"}, Normalized: "زقزقة"})
	if err != nil {
		t.Fatalf("retrieve findings: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("a term sharing nothing with any finding returned %+v", none)
	}
}
