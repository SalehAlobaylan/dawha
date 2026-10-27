package sourceprocessing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

// suffix is a short unique token for test names on the shared database.
func suffix() string {
	id := uuid.New()
	return id.String()[:8]
}

// importTestDraft bundles everything one import test needs: a reviewed tree
// draft over a real source file, ready for the dry run.
type importTestDraft struct {
	fixture  *testsupport.Fixture
	service  *Service
	ctx      context.Context
	sourceID string
	draftID  string
	ownerID  string
	revision int
	nameA    string
	nameB    string
}

// newReviewedImportDraft creates a source, uploads the stand-in file, and
// walks a two-node one-edge graph through the full two-reviewer ledger so the
// import's preconditions are genuinely met.
func newReviewedImportDraft(t *testing.T, nameA, nameB string) *importTestDraft {
	t.Helper()
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "مالك المسودة")
	peer := actor.RegisterAndLogin(t, fixture, "المراجع الثاني")
	fixture.GrantRole(peer.User.ID, "researcher")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{TitleAR: "توثيق اختبار " + fixture.Unique("d"), SourceType: "family_document"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	file, err := uploader.Upload(ctx, source.Source.ID, owner.User.ID, UploadInput{Filename: "family.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7\nfixture")})
	if err != nil {
		t.Fatalf("upload source: %v", err)
	}
	var runID string
	if err := fixture.QueryRow(`SELECT id::text FROM source_processing_runs WHERE source_file_id = $1`, file.ID).Scan(&runID); err != nil {
		t.Fatalf("find run: %v", err)
	}
	fixture.Exec(`UPDATE source_processing_runs SET status = 'succeeded', stage = 'complete', page_count = 1 WHERE id = $1`, runID)
	regions := fmt.Sprintf(`[
		{"method":"tesseract_ocr","kind":"word","text":%q,"x":0.70,"y":0.10,"width":0.08,"height":0.02},
		{"method":"tesseract_ocr","kind":"word","text":%q,"x":0.70,"y":0.30,"width":0.08,"height":0.02}
	]`, nameA, nameB)
	fixture.Exec(`INSERT INTO source_page_evidence (run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count) VALUES ($1, $2, $3, 1, 100, 200, 'readable', 'tesseract_ocr', 'نص التركيبات', '[]'::jsonb, $4::jsonb, 2)`, runID, source.Source.ID, file.ID, regions)
	service := NewService(fixture.Pool(), nil, nil, nil, nil)
	draft, err := service.CreateTreeDraft(ctx, file.ID, owner.User.ID, "مسودة اختبار")
	if err != nil {
		t.Fatalf("create tree draft: %v", err)
	}
	if len(draft.Nodes) < 2 {
		t.Fatalf("expected two proposed names, got %+v", draft.Nodes)
	}
	names := []string{nameA, nameB}
	nodes := make([]TreeDraftNode, 0, 2)
	for index, node := range []TreeDraftNode{draft.Nodes[0], draft.Nodes[1]} {
		node.NameAR = names[index]
		node.Included, node.Reviewed, node.Status = true, true, "reviewed"
		nodes = append(nodes, node)
	}
	draft.Edges = []TreeDraftEdge{{ID: uuid.NewString(), SubjectNodeID: nodes[0].ID, ObjectNodeID: nodes[1].ID, Predicate: "parent_of", PageEvidenceID: nodes[0].PageEvidenceID, Path: []TreeDraftPoint{{X: .74, Y: .13}, {X: .74, Y: .20}, {X: .74, Y: .32}}, Status: "reviewed", SecondReviewRequired: true}}
	saved, err := service.SaveTreeDraft(ctx, draft.ID, owner.User.ID, SaveTreeDraftInput{ExpectedRevision: draft.Revision, TitleAR: draft.TitleAR, Nodes: nodes, Edges: draft.Edges})
	if err != nil {
		t.Fatalf("save reviewed graph: %v", err)
	}
	secondSaved, err := service.SaveTreeDraft(ctx, draft.ID, peer.User.ID, SaveTreeDraftInput{ExpectedRevision: saved.Revision, TitleAR: draft.TitleAR, Nodes: saved.Nodes, Edges: saved.Edges})
	if err != nil {
		t.Fatalf("record independent second review: %v", err)
	}
	if secondSaved.Edges[0].ReviewerCount != 2 {
		t.Fatalf("second review ledger = %+v; want two distinct reviewers", secondSaved.Edges)
	}
	return &importTestDraft{
		fixture:  fixture,
		service:  service,
		ctx:      ctx,
		sourceID: source.Source.ID,
		draftID:  draft.ID,
		ownerID:  owner.User.ID,
		revision: secondSaved.Revision,
		nameA:    nameA,
		nameB:    nameB,
	}
}

// confirm gets the dry run's report and replays it at the apply, which is the
// client flow the service requires.
func (d *importTestDraft) confirm() (TreeDraftImportResult, error) {
	preview, err := d.service.PreviewTreeDraftImport(d.ctx, d.draftID, d.ownerID, d.revision)
	if err != nil {
		return TreeDraftImportResult{}, err
	}
	return d.service.ImportTreeDraft(d.ctx, d.draftID, d.ownerID, TreeDraftImportInput{
		ExpectedRevision:     d.revision,
		AcknowledgePreview:   true,
		SourceFileChecksum:   preview.SourceFileChecksum,
		ExpectedNewPeople:    preview.NewPeople,
		ExpectedLinkedPeople: preview.LinkedExisting,
		ExpectedCollisions:   len(preview.Collisions),
		ExpectedRelations:    preview.Relations,
		ExpectedStatements:   preview.Statements,
		ExpectedAliases:      preview.Aliases,
	})
}

// fixtureDB is the slice of the fixture the seeding helper needs.
type fixtureDB = testsupport.Fixture

func seedExistingPerson(tx fixtureDB, nameAR, ownerID string) (string, error) {
	var personID string
	err := tx.QueryRow(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, gender, created_by) VALUES ($1, $2, $3, 'unknown', $4) RETURNING id::text`, uuid.New(), nameAR, identity.NormalizeArabicName(nameAR), ownerID).Scan(&personID)
	return personID, err
}

func TestTreeDraftImportDryRunWritesNothingAndApplyMatchesTheReport(t *testing.T) {
	draft := newReviewedImportDraft(t, "سالم "+suffix()+" بن أحمد", "ناصر "+suffix())

	beforePeople := draft.fixture.Count(`SELECT count(*) FROM people WHERE created_by = $1`, draft.ownerID)
	preview, err := draft.service.PreviewTreeDraftImport(draft.ctx, draft.draftID, draft.ownerID, draft.revision)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if preview.NewPeople != 2 || preview.LinkedExisting != 0 || preview.Relations != 1 || preview.Statements != 1 || preview.Aliases != 2 || preview.Candidates != 0 {
		t.Fatalf("dry run report = %+v", preview)
	}
	if draft.fixture.Count(`SELECT count(*) FROM people WHERE created_by = $1`, draft.ownerID) != beforePeople {
		t.Fatal("the dry run created a people row")
	}
	if draft.fixture.Count(`SELECT count(*) FROM source_tree_draft_imports WHERE draft_id = $1`, draft.draftID) != 0 {
		t.Fatal("the dry run imported a tree")
	}
	if draft.fixture.Count(`SELECT count(*) FROM source_statements WHERE source_id = $1`, draft.sourceID) != 0 {
		t.Fatal("the dry run wrote a statement")
	}
	if draft.fixture.Count(`SELECT count(*) FROM entity_relationships WHERE created_by = $1`, draft.ownerID) != 0 {
		t.Fatal("the dry run wrote a research-graph row")
	}
	if draft.fixture.Count(`SELECT count(*) FROM entity_resolution_runs WHERE algorithm_version = 'tree-import-link-v1' AND requested_by = $1`, draft.ownerID) != 0 {
		t.Fatal("the dry run opened a resolution run")
	}

	result, err := draft.confirm()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.ImportedPeople != preview.NewPeople || result.LinkedExistingPeople != preview.LinkedExisting || result.ImportedRelations != preview.Relations || result.Statements != preview.Statements || result.Aliases != preview.Aliases || result.Candidates != preview.Candidates {
		t.Fatalf("apply counts drifted from the dry run: report=%+v result=%+v", preview, result)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM source_statements WHERE source_id = $1 AND extraction_method = 'imported' AND review_status = 'accepted'`, draft.sourceID); got != 1 {
		t.Fatalf("imported statements = %d, want 1", got)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM entity_relationships WHERE created_by = $1 AND status = 'unresolved'`, draft.ownerID); got != 1 {
		t.Fatalf("research-graph rows = %d, want 1", got)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM person_aliases WHERE alias_type = 'source_spelling' AND source_id = $1`, draft.sourceID); got != 2 {
		t.Fatalf("source spellings kept = %d, want 2", got)
	}
	var locator string
	if err := draft.fixture.QueryRow(`SELECT locator_ar FROM source_statements WHERE source_id = $1 AND extraction_method = 'imported'`, draft.sourceID).Scan(&locator); err != nil || locator == "" {
		t.Fatalf("statement locator = %q, %v; want a draft-edge handle", locator, err)
	}
}

func TestTreeDraftImportLinksExactNormalizedNameInsteadOfDuplicating(t *testing.T) {
	token := "سالمي" + suffix() + " بن أحمد"
	draft := newReviewedImportDraft(t, strings.ReplaceAll(token, " ", "  "), "ناصر "+suffix())
	personID, err := seedExistingPerson(*draft.fixture, token, draft.ownerID)
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}

	preview, err := draft.service.PreviewTreeDraftImport(draft.ctx, draft.draftID, draft.ownerID, draft.revision)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if preview.NewPeople != 1 || preview.LinkedExisting != 1 || len(preview.Collisions) != 1 || preview.Collisions[0].ExistingPersonID != personID {
		t.Fatalf("dry run did not propose the link: %+v against person %s", preview, personID)
	}
	result, err := draft.confirm()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.ImportedPeople != 1 || result.LinkedExistingPeople != 1 {
		t.Fatalf("apply result = %+v", result)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM people WHERE normalized_name_ar = $1`, identity.NormalizeArabicName(token)); got != 1 {
		t.Fatalf("people with the linked name = %d, want 1 (no duplicate row)", got)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM entity_relationships r JOIN people p ON p.id = r.subject_id WHERE p.id = $1 AND r.predicate = 'parent_of'`, personID); got != 1 {
		t.Fatalf("linked person endpoints = %d, want 1", got)
	}
	var verbatim string
	if err := draft.fixture.QueryRow(`SELECT a.value_ar FROM person_aliases a WHERE a.person_id = $1 AND a.alias_type = 'source_spelling'`, personID).Scan(&verbatim); err != nil {
		t.Fatalf("source spelling alias: %v", err)
	}
	if verbatim == token || identity.NormalizeArabicName(verbatim) == "" {
		t.Fatalf("alias verbatim = %q; want the un-normalised reading form", verbatim)
	}
	var status string
	if err := draft.fixture.QueryRow(`SELECT identity_status FROM people WHERE id = $1`, personID).Scan(&status); err != nil || status != "unreviewed" {
		t.Fatalf("linked person identity_status = %q, %v", status, err)
	}
}

func TestTreeDraftImportSecondOccurrenceOfOneNameBecomesANewPerson(t *testing.T) {
	token := "عبدالله" + suffix() + " بن محمد"
	draft := newReviewedImportDraft(t, token, token)
	if _, err := seedExistingPerson(*draft.fixture, token, draft.ownerID); err != nil {
		t.Fatalf("seed person: %v", err)
	}

	preview, err := draft.service.PreviewTreeDraftImport(draft.ctx, draft.draftID, draft.ownerID, draft.revision)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if preview.NewPeople != 1 || preview.LinkedExisting != 1 || len(preview.Warnings) == 0 {
		t.Fatalf("dry run did not keep the two occurrences apart: %+v", preview)
	}
	result, err := draft.confirm()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.ImportedPeople != 1 || result.LinkedExistingPeople != 1 {
		t.Fatalf("apply result = %+v", result)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM people WHERE normalized_name_ar = $1`, identity.NormalizeArabicName(token)); got != 2 {
		t.Fatalf("people named alike = %d, want 2 (one linked, one new)", got)
	}
	var linkedID, newID string
	if err := draft.fixture.QueryRow(`SELECT r.subject_id::text FROM entity_relationships r WHERE r.created_by = $1 AND r.predicate = 'parent_of'`, draft.ownerID).Scan(&linkedID); err != nil {
		t.Fatalf("linked endpoint: %v", err)
	}
	if err := draft.fixture.QueryRow(`SELECT r.object_id::text FROM entity_relationships r WHERE r.created_by = $1 AND r.predicate = 'parent_of'`, draft.ownerID).Scan(&newID); err != nil {
		t.Fatalf("new-person endpoint: %v", err)
	}
	if linkedID == newID {
		t.Fatal("the two occurrences collapsed onto one person")
	}
}

func TestTreeDraftImportProposesFuzzyCandidateAndNeverMerges(t *testing.T) {
	shared := "محمد" + suffix()
	draft := newReviewedImportDraft(t, shared+" صالح", shared+" سالم")
	existingID, err := seedExistingPerson(*draft.fixture, shared+" صالح العلي", draft.ownerID)
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}

	preview, err := draft.service.PreviewTreeDraftImport(draft.ctx, draft.draftID, draft.ownerID, draft.revision)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if preview.Candidates == 0 || preview.NewPeople != 2 || preview.LinkedExisting != 0 {
		t.Fatalf("dry run proposed no candidates: %+v", preview)
	}
	result, err := draft.confirm()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Candidates != preview.Candidates || result.ResolutionRunID == "" {
		t.Fatalf("apply result = %+v; want a resolution run", result)
	}
	var runCount int
	if err := draft.fixture.QueryRow(`SELECT count(*) FROM entity_resolution_runs WHERE id = $1 AND algorithm_version = 'tree-import-link-v1' AND stage = 'complete' AND status = 'succeeded'`, result.ResolutionRunID).Scan(&runCount); err != nil || runCount != 1 {
		t.Fatalf("resolution run = %d, %v; want one completed import run", runCount, err)
	}
	var candidateCount int
	if err := draft.fixture.QueryRow(`SELECT count(*) FROM entity_resolution_candidates WHERE run_id = $1 AND match_class = 'possible_match' AND left_entity_id < right_entity_id`, result.ResolutionRunID).Scan(&candidateCount); err != nil || candidateCount != result.Candidates {
		t.Fatalf("candidate rows = %d, %v; want %d ordered pairs", candidateCount, err, result.Candidates)
	}
	var featured int
	if err := draft.fixture.QueryRow(`SELECT count(*) FROM entity_resolution_candidates WHERE run_id = $1 AND (left_entity_id = $2 OR right_entity_id = $2)`, result.ResolutionRunID, existingID).Scan(&featured); err != nil || featured == 0 {
		t.Fatalf("candidate pairs do not feature the existing person: %d, %v", featured, err)
	}
	var status string
	if err := draft.fixture.QueryRow(`SELECT identity_status FROM people WHERE id = $1`, existingID).Scan(&status); err != nil || status != "unreviewed" {
		t.Fatalf("existing person identity_status = %q, %v", status, err)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM entity_merges WHERE survivor_id = $1 OR merged_id = $1`, existingID); got != 0 {
		t.Fatalf("the existing person was merged by the import: %d", got)
	}
}

func TestTreeDraftImportRecordsTheKunyahBesideTheReading(t *testing.T) {
	token := suffix()
	draft := newReviewedImportDraft(t, "أبو عبد الله بن "+token, "ناصر بن "+token)

	preview, err := draft.service.PreviewTreeDraftImport(draft.ctx, draft.draftID, draft.ownerID, draft.revision)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	// Two people, their source spellings, and one kunyah alias.
	if preview.Aliases != 3 {
		t.Fatalf("dry run alias count = %d, want 3", preview.Aliases)
	}
	result, err := draft.confirm()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Aliases != 3 {
		t.Fatalf("apply alias count = %d, want 3", result.Aliases)
	}
	var kunyah string
	if err := draft.fixture.QueryRow(`SELECT value_ar FROM person_aliases WHERE alias_type = 'kunyah' AND source_id = $1`, draft.sourceID).Scan(&kunyah); err != nil || kunyah != "أبو عبد الله" {
		t.Fatalf("kunyah alias = %q, %v; want the أبو عبد الله compound", kunyah, err)
	}
}

func TestTreeDraftImportRefusesToApplyWithoutTheDryRunReport(t *testing.T) {
	draft := newReviewedImportDraft(t, "سالم "+suffix()+" بن أحمد", "ناصر")

	if _, err := draft.service.ImportTreeDraft(draft.ctx, draft.draftID, draft.ownerID, TreeDraftImportInput{ExpectedRevision: draft.revision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("apply without an acknowledged dry run = %v, want conflict", err)
	}
	preview, err := draft.service.PreviewTreeDraftImport(draft.ctx, draft.draftID, draft.ownerID, draft.revision)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	// The data the report described changed underneath it: a person with the
	// exact same name appeared between the preview and the apply.
	if _, err := seedExistingPerson(*draft.fixture, draft.nameA, draft.ownerID); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	if _, err := draft.service.ImportTreeDraft(draft.ctx, draft.draftID, draft.ownerID, TreeDraftImportInput{
		ExpectedRevision:     draft.revision,
		AcknowledgePreview:   true,
		SourceFileChecksum:   preview.SourceFileChecksum,
		ExpectedNewPeople:    preview.NewPeople,
		ExpectedLinkedPeople: preview.LinkedExisting,
		ExpectedCollisions:   len(preview.Collisions),
		ExpectedRelations:    preview.Relations,
		ExpectedStatements:   preview.Statements,
		ExpectedAliases:      preview.Aliases,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("apply with an unmatched report = %v, want conflict", err)
	}
}

func TestTreeDraftRetractionReversesTheGraphButKeepsLaterResearch(t *testing.T) {
	draft := newReviewedImportDraft(t, "سالم "+suffix()+" بن أحمد", "ناصر")
	if _, err := draft.confirm(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// A later researcher touches one graph row after the import; the
	// retraction must leave that row in place and report it.
	var touchedID string
	if err := draft.fixture.QueryRow(`SELECT id::text FROM entity_relationships WHERE created_by = $1 AND status = 'unresolved' AND predicate = 'parent_of' LIMIT 1`, draft.ownerID).Scan(&touchedID); err != nil {
		t.Fatalf("find graph row: %v", err)
	}
	draft.fixture.Exec(`UPDATE entity_relationships SET status = 'documented', updated_at = now() WHERE id = $1`, touchedID)

	retracted, err := draft.service.RetractTreeDraft(draft.ctx, draft.draftID, draft.ownerID)
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if retracted.RetractedStatements != 1 || retracted.RetractedRelationships != 0 || retracted.RetractedAliases != 2 || retracted.PreservedGraphRows != 1 {
		t.Fatalf("retraction report = %+v", retracted)
	}
	var stillThere string
	if err := draft.fixture.QueryRow(`SELECT id::text FROM entity_relationships WHERE id = $1`, touchedID).Scan(&stillThere); err != nil {
		t.Fatalf("the later researcher's graph row was destroyed: %v", err)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM person_aliases WHERE source_id = $1`, draft.sourceID); got != 0 {
		t.Fatalf("source-scoped aliases = %d, want 0 after retraction", got)
	}
	if got := draft.fixture.Count(`SELECT count(*) FROM source_statements WHERE source_id = $1 AND extraction_method = 'imported'`, draft.sourceID); got != 0 {
		t.Fatalf("imported statements = %d, want 0 after retraction", got)
	}
}
