package evidence

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
)

// The synthetic people and places db/seeds/001_demo.sql puts in every fixture
// schema. Claims are attached to them so the acceptance journey exercises the
// same rows a real deployment would.
const (
	seededPersonOne   = "10000000-0000-0000-0000-000000000001"
	seededPersonTwo   = "10000000-0000-0000-0000-000000000002"
	seededPlaceRiyadh = "20000000-0000-0000-0000-000000000001"
)

func TestClaimEvidenceChainIsVersionedAndAudited(t *testing.T) {
	fixture := testsupport.New(t)
	researcher := actor.RegisterAndLogin(t, fixture, "باحث الدليل")
	service := NewService(fixture.Pool())

	source, err := service.CreateSource(fixture.Ctx(), researcher.User.ID, CreateSourceInput{
		TitleAR:          "سجل أسري " + fixture.Unique("s"),
		SourceType:       "manuscript",
		CitationAR:       "الجزء الثاني، الصفحة " + fixture.Tag(),
		DependencyStatus: "independent",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	passage, err := service.CreatePassage(fixture.Ctx(), source.Source.ID, researcher.User.ID, SourcePassageInput{
		SequenceNumber: 1,
		LocatorAR:      "صفحة ١٢",
		TextAR:         "ذكر السجل أن عبدالله بن محمد، وسكن في الرياض.",
	})
	if err != nil {
		t.Fatalf("create passage: %v", err)
	}
	statement, err := service.CreateStatement(fixture.Ctx(), source.Source.ID, researcher.User.ID, SourceStatementInput{
		SourcePassageID: passage.Passages[0].ID,
		StatementTextAR: "والد عبدالله هو محمد.",
		LocatorAR:       "صفحة ١٢، سطر ٣",
		ReviewStatus:    "needs_review",
	})
	if err != nil {
		t.Fatalf("create statement: %v", err)
	}
	if statement.Statements[0].ExtractionMethod != "manual" {
		t.Fatalf("extraction method = %s, want manual for a hand-written statement", statement.Statements[0].ExtractionMethod)
	}

	claim, err := service.CreateClaim(fixture.Ctx(), researcher.User.ID, CreateClaimInput{
		SubjectType: "person",
		SubjectID:   seededPersonOne,
		Predicate:   "father_of",
		ObjectType:  "person",
		ObjectID:    seededPersonTwo,
		Status:      "unresolved",
		PlaceID:     seededPlaceRiyadh,
		NotesAR:     "نسبة تقرأ من السجل " + fixture.Tag(),
	})
	if err != nil {
		t.Fatalf("create claim: %v", err)
	}
	if claim.Status != "unresolved" {
		t.Fatalf("claim status = %s, want unresolved; a source is not a fact", claim.Status)
	}
	// A new claim starts as version 1, so the reader can see it was never
	// quietly rewritten.
	if got := fixture.Count(`SELECT count(*) FROM claim_versions WHERE claim_id = $1 AND version_number = 1`, claim.ID); got != 1 {
		t.Fatalf("claim version rows = %d, want version 1", got)
	}

	supported, err := service.AddEvidence(fixture.Ctx(), claim.ID, researcher.User.ID, AddEvidenceInput{
		SourceStatementID: statement.Statements[0].ID,
		Relation:          "supports",
		EvidenceNoteAR:    "السجل يذكر الأب بالاسم.",
	})
	if err != nil {
		t.Fatalf("add supporting evidence: %v", err)
	}
	if len(supported.Evidence) != 1 || supported.Evidence[0].Relation != "supports" {
		t.Fatalf("evidence = %+v, want one supporting link", supported.Evidence)
	}
	// Evidence carries the text it rests on, so a reader never has to trust a
	// bare reference.
	if supported.Evidence[0].StatementTextAR == "" || supported.Evidence[0].SourceTitleAR == "" {
		t.Fatalf("evidence %+v lost the statement text or the source title", supported.Evidence[0])
	}

	contradicting, err := service.CreateStatement(fixture.Ctx(), source.Source.ID, researcher.User.ID, SourceStatementInput{
		SourcePassageID: passage.Passages[0].ID,
		StatementTextAR: "وقال السجل الآخر: والد عبدالله سعد.",
		LocatorAR:       "صفحة ١٢، هامش",
		ReviewStatus:    "needs_review",
	})
	if err != nil {
		t.Fatalf("create contradicting statement: %v", err)
	}
	disputed, err := service.AddEvidence(fixture.Ctx(), claim.ID, researcher.User.ID, AddEvidenceInput{
		SourceStatementID: contradicting.Statements[len(contradicting.Statements)-1].ID,
		Relation:          "contradicts",
		EvidenceNoteAR:    "رواية منافسة من مصدر آخر.",
	})
	if err != nil {
		t.Fatalf("add contradicting evidence: %v", err)
	}
	// Contradicting evidence is kept beside the supporting evidence rather than
	// replacing it, because the disagreement is the finding.
	relations := map[string]int{}
	for _, item := range disputed.Evidence {
		relations[item.Relation]++
	}
	if relations["supports"] != 1 || relations["contradicts"] != 1 {
		t.Fatalf("evidence relations = %v, want one supports and one contradicts", relations)
	}
	// Supporting and contradicting evidence live in separate tables, so a
	// reader cannot mistake one for the other and neither overwrites the other.
	if got := fixture.Count(`SELECT count(*) FROM claim_evidence WHERE claim_id = $1`, claim.ID); got != 1 {
		t.Fatalf("claim_evidence rows = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM claim_counter_evidence WHERE claim_id = $1`, claim.ID); got != 1 {
		t.Fatalf("claim_counter_evidence rows = %d, want 1", got)
	}
	// Every mutation of the chain left an audit row naming the claim.
	for _, action := range []string{"claim_created", "claim_evidence_linked"} {
		if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = $1 AND entity_id = $2`, action, claim.ID); got == 0 {
			t.Fatalf("audit_log holds no %s row for the claim", action)
		}
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'source_created' AND entity_id = $1`, source.Source.ID); got != 1 {
		t.Fatalf("source_created audit rows = %d, want 1", got)
	}
}

func TestPrivateSourceIsInvisibleToAnotherResearcher(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب المصدر")
	// A plain registrant, deliberately without the researcher role: that role is
	// a platform grant that widens the private-source boundary on purpose, so
	// granting it here would test the wrong thing.
	other := actor.RegisterAndLogin(t, fixture, "باحث آخر")
	service := NewService(fixture.Pool())

	created, err := service.CreateSource(fixture.Ctx(), owner.User.ID, CreateSourceInput{
		TitleAR:    "مسودة خاصة " + fixture.Unique("s"),
		SourceType: "manuscript",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	fixture.Exec(`UPDATE sources SET visibility = 'private' WHERE id = $1`, created.Source.ID)

	if _, err := service.GetSourceForActor(fixture.Ctx(), created.Source.ID, other.User.ID); err == nil {
		t.Fatal("another researcher read a private source")
	}
	if _, err := service.GetSourceForActor(fixture.Ctx(), created.Source.ID, owner.User.ID); err != nil {
		t.Fatalf("the owner cannot read their own private source: %v", err)
	}
	visible, err := service.ListSourcesForActor(fixture.Ctx(), other.User.ID)
	if err != nil {
		t.Fatalf("list sources for another researcher: %v", err)
	}
	for _, source := range visible {
		if source.ID == created.Source.ID {
			t.Fatal("a private source appeared in another researcher's library")
		}
	}
}

func TestClaimInputIsValidated(t *testing.T) {
	fixture := testsupport.New(t)
	researcher := actor.RegisterAndLogin(t, fixture, "باحث")
	service := NewService(fixture.Pool())

	if _, err := service.CreateClaim(fixture.Ctx(), researcher.User.ID, CreateClaimInput{
		SubjectType: "person",
		SubjectID:   "not-a-uuid",
		Predicate:   "father_of",
		ObjectType:  "person",
		ObjectID:    seededPersonTwo,
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("claim with a malformed subject = %v, want ErrValidation", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM claims WHERE created_by = $1`, researcher.User.ID); got != 0 {
		t.Fatalf("a refused claim was written anyway: %d rows", got)
	}
}
