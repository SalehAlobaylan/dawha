package sourceprocessing

import (
	"context"
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/search"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

func TestProposeNameRegionsGroupsWordsWithoutCreatingRelationships(t *testing.T) {
	evidenceID := uuid.NewString()
	regions := []PageRegionEvidence{
		{Method: "tesseract_ocr", PageRegion: PageRegion{Kind: "word", Text: "سالم", X: .70, Y: .10, Width: .08, Height: .02}},
		{Method: "tesseract_ocr", PageRegion: PageRegion{Kind: "word", Text: "بن", X: .60, Y: .10, Width: .04, Height: .02}},
		{Method: "tesseract_ocr", PageRegion: PageRegion{Kind: "word", Text: "أحمد", X: .48, Y: .10, Width: .09, Height: .02}},
		{Method: "pdf_text", PageRegion: PageRegion{Kind: "word", Text: "ignored second reading", X: .20, Y: .10, Width: .12, Height: .02}},
		{Method: "tesseract_ocr", PageRegion: PageRegion{Kind: "word", Text: "ناصر", X: .70, Y: .30, Width: .08, Height: .02}},
	}
	nodes := proposeNameRegions(evidenceID, 1, "tesseract_ocr", regions)
	if len(nodes) != 2 {
		t.Fatalf("proposed %d names, want two line groups: %+v", len(nodes), nodes)
	}
	if nodes[0].NameAR != "سالم بن أحمد" || nodes[1].NameAR != "ناصر" {
		t.Fatalf("proposed names = %q, %q; want visual Arabic order", nodes[0].NameAR, nodes[1].NameAR)
	}
	for _, node := range nodes {
		if node.Included || node.Reviewed || node.Status != "proposed" || node.PageEvidenceID != evidenceID {
			t.Fatalf("machine suggestion was treated as reviewed data: %+v", node)
		}
	}
}

func TestParentCycleBlocksImportAndUnknownRelationStaysStaged(t *testing.T) {
	nodes := []TreeDraftNode{{ID: uuid.NewString(), NameAR: "أ", Included: true, Reviewed: true, Status: "reviewed"}, {ID: uuid.NewString(), NameAR: "ب", Included: true, Reviewed: true, Status: "reviewed"}}
	edges := []TreeDraftEdge{{SubjectNodeID: nodes[0].ID, ObjectNodeID: nodes[1].ID, Predicate: "parent_of", Status: "reviewed"}, {SubjectNodeID: nodes[1].ID, ObjectNodeID: nodes[0].ID, Predicate: "parent_of", Status: "reviewed"}}
	if !hasParentCycle(nodes, edges) {
		t.Fatal("cycle was not detected")
	}
	if !errors.Is(validateImportableGraph(nodes, edges), ErrConflict) {
		t.Fatal("cyclic graph should not be importable")
	}
	if supportedTreePredicate("descendant_of") {
		t.Fatal("broad descendant relation must remain staged")
	}
}

func TestTreeDraftReviewFingerprintBindsLabelsEndpointsAndConnectorGeometry(t *testing.T) {
	nodes := map[string]TreeDraftNode{
		"a": {ID: "a", NameAR: "سالم", PageEvidenceID: "page-1", Region: PageRegion{Kind: "word", X: .1, Y: .1, Width: .1, Height: .03}},
		"b": {ID: "b", NameAR: "ناصر", PageEvidenceID: "page-1", Region: PageRegion{Kind: "word", X: .1, Y: .5, Width: .1, Height: .03}},
	}
	edge := TreeDraftEdge{ID: "edge", SubjectNodeID: "a", ObjectNodeID: "b", Predicate: "parent_of", PageEvidenceID: "page-1", Path: []TreeDraftPoint{{X: .15, Y: .13}, {X: .15, Y: .3}, {X: .15, Y: .5}}, Status: "reviewed"}
	original := treeDraftEdgeFingerprint(edge, nodes)
	changedPath := edge
	changedPath.Path = append([]TreeDraftPoint(nil), edge.Path...)
	changedPath.Path[1].X = .2
	if treeDraftEdgeFingerprint(changedPath, nodes) == original {
		t.Fatal("editing connector geometry retained the same review fingerprint")
	}
	changedNode := nodes["a"]
	changedNode.NameAR = "سليمان"
	nodes["a"] = changedNode
	if treeDraftEdgeFingerprint(edge, nodes) == original {
		t.Fatal("editing an endpoint label retained the same review fingerprint")
	}
}

// confirmTreeDraftImport is the client flow the apply now requires: run the
// dry run, then apply carrying the report's own preconditions back.
func confirmTreeDraftImport(service *Service, ctx context.Context, draftID, actorID string, revision int) (TreeDraftImportResult, error) {
	preview, err := service.PreviewTreeDraftImport(ctx, draftID, actorID, revision)
	if err != nil {
		return TreeDraftImportResult{}, err
	}
	return service.ImportTreeDraft(ctx, draftID, actorID, TreeDraftImportInput{
		ExpectedRevision:     revision,
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

func TestTreeDraftImportCreatesPrivateUnresolvedTreeAndCanBeRetracted(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "مالك شجرة المصدر")
	secondReviewer := actor.RegisterAndLogin(t, fixture, "مراجع مستقل")
	fixture.GrantRole(secondReviewer.User.ID, "researcher")
	stranger := actor.Register(t, fixture, "مستخدم آخر")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{TitleAR: "شجرة اختبار " + fixture.Unique("t"), SourceType: "family_document"})
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
	regions := `[
		{"method":"tesseract_ocr","kind":"word","text":"سالم","x":0.70,"y":0.10,"width":0.08,"height":0.02},
		{"method":"tesseract_ocr","kind":"word","text":"بن","x":0.60,"y":0.10,"width":0.04,"height":0.02},
		{"method":"tesseract_ocr","kind":"word","text":"أحمد","x":0.48,"y":0.10,"width":0.09,"height":0.02},
		{"method":"tesseract_ocr","kind":"word","text":"خالد","x":0.70,"y":0.30,"width":0.08,"height":0.02}
	]`
	fixture.Exec(`INSERT INTO source_page_evidence (run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count) VALUES ($1, $2, $3, 1, 100, 200, 'readable', 'tesseract_ocr', 'سالم بن أحمد خالد', '[]'::jsonb, $4::jsonb, 4)`, runID, source.Source.ID, file.ID, regions)
	service := NewService(fixture.Pool(), nil, nil, nil, nil)
	draft, err := service.CreateTreeDraft(ctx, file.ID, owner.User.ID, "شجرة آل سالم")
	if err != nil {
		t.Fatalf("create tree draft: %v", err)
	}
	var otherRunID, otherPageEvidenceID string
	if err := fixture.QueryRow(`INSERT INTO source_processing_runs (source_id, source_file_id, status, stage, run_kind, pipeline_config) VALUES ($1, $2, 'succeeded', 'complete', 'document', '{}'::jsonb) RETURNING id::text`, source.Source.ID, file.ID).Scan(&otherRunID); err != nil {
		t.Fatalf("create second processing run: %v", err)
	}
	fixture.Exec(`INSERT INTO source_page_evidence (run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count) VALUES ($1, $2, $3, 1, 100, 200, 'readable', 'tesseract_ocr', 'سالم بن أحمد خالد', '[]'::jsonb, $4::jsonb, 4)`, otherRunID, source.Source.ID, file.ID, regions)
	if err := fixture.QueryRow(`SELECT id::text FROM source_page_evidence WHERE run_id = $1 AND page_number = 1`, otherRunID).Scan(&otherPageEvidenceID); err != nil {
		t.Fatalf("find second-run page evidence: %v", err)
	}
	mixedRunNodes := append([]TreeDraftNode(nil), draft.Nodes...)
	mixedRunNodes[0].PageEvidenceID = otherPageEvidenceID
	if _, err := service.SaveTreeDraft(ctx, draft.ID, owner.User.ID, SaveTreeDraftInput{ExpectedRevision: draft.Revision, TitleAR: draft.TitleAR, Nodes: mixedRunNodes}); !errors.Is(err, ErrValidation) {
		t.Fatalf("save page evidence from another run of the same file = %v, want validation error", err)
	}
	privateProbe := "سالم خاص " + uuid.NewString()
	draft.Nodes[0].NameAR = privateProbe
	if len(draft.Nodes) != 2 || draft.Nodes[0].Included || len(draft.Edges) != 0 {
		t.Fatalf("initial draft should contain excluded name proposals and no inferred edges: %+v", draft)
	}
	if _, err := service.GetTreeDraft(ctx, draft.ID, stranger.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unauthorized draft read = %v, want forbidden", err)
	}
	for index := range draft.Nodes {
		draft.Nodes[index].Included, draft.Nodes[index].Reviewed, draft.Nodes[index].Status = true, true, "reviewed"
	}
	draft.Edges = []TreeDraftEdge{{ID: uuid.NewString(), SubjectNodeID: draft.Nodes[0].ID, ObjectNodeID: draft.Nodes[1].ID, Predicate: "parent_of", PageEvidenceID: draft.Nodes[0].PageEvidenceID, Path: []TreeDraftPoint{{X: .74, Y: .13}, {X: .74, Y: .20}, {X: .74, Y: .32}}, Status: "reviewed", SecondReviewRequired: true}}
	saved, err := service.SaveTreeDraft(ctx, draft.ID, owner.User.ID, SaveTreeDraftInput{ExpectedRevision: draft.Revision, TitleAR: draft.TitleAR, Nodes: draft.Nodes, Edges: draft.Edges})
	if err != nil {
		t.Fatalf("save reviewed graph: %v", err)
	}
	if saved.Revision != 2 || len(saved.Warnings) != 0 {
		t.Fatalf("saved graph = %+v", saved)
	}
	if _, err := service.SaveTreeDraft(ctx, draft.ID, owner.User.ID, SaveTreeDraftInput{ExpectedRevision: 1, TitleAR: draft.TitleAR, Nodes: draft.Nodes, Edges: draft.Edges}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale graph save = %v, want conflict", err)
	}
	if len(saved.Edges) != 1 || saved.Edges[0].ReviewerCount != 1 || !saved.Edges[0].ReviewedByMe {
		t.Fatalf("first edge review ledger = %+v, want one distinct reviewer", saved.Edges)
	}
	if _, err := confirmTreeDraftImport(service, ctx, draft.ID, owner.User.ID, saved.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("import without independent second review = %v, want conflict", err)
	}
	secondView, err := service.GetTreeDraft(ctx, draft.ID, secondReviewer.User.ID)
	if err != nil || len(secondView.Edges) != 1 || secondView.Edges[0].ReviewerCount != 1 || secondView.Edges[0].ReviewedByMe {
		t.Fatalf("second reviewer view = %+v, %v; want one prior reviewer and no prior decision by this actor", secondView.Edges, err)
	}
	secondSaved, err := service.SaveTreeDraft(ctx, draft.ID, secondReviewer.User.ID, SaveTreeDraftInput{ExpectedRevision: saved.Revision, TitleAR: draft.TitleAR, Nodes: saved.Nodes, Edges: saved.Edges})
	if err != nil {
		t.Fatalf("record independent second review: %v", err)
	}
	if secondSaved.Revision != 3 || secondSaved.Edges[0].ReviewerCount != 2 || !secondSaved.Edges[0].ReviewedByMe {
		t.Fatalf("second review ledger = %+v; want two distinct reviewers", secondSaved.Edges)
	}
	changedEdges := append([]TreeDraftEdge(nil), secondSaved.Edges...)
	changedEdges[0].Path = append([]TreeDraftPoint(nil), changedEdges[0].Path...)
	changedEdges[0].Path[1].Y = .22
	changedNodes := append([]TreeDraftNode(nil), secondSaved.Nodes...)
	for index := range changedNodes {
		if changedNodes[index].NameAR == privateProbe {
			changedNodes[index].Included, changedNodes[index].Reviewed, changedNodes[index].Status = true, true, "reviewed"
		}
	}
	changed, err := service.SaveTreeDraft(ctx, draft.ID, owner.User.ID, SaveTreeDraftInput{ExpectedRevision: secondSaved.Revision, TitleAR: draft.TitleAR, Nodes: changedNodes, Edges: changedEdges})
	if err != nil {
		t.Fatalf("save changed connector geometry: %v", err)
	}
	if changed.Edges[0].Status != "proposed" || changed.Edges[0].ReviewerCount != 0 || changed.Edges[0].ReviewedByMe {
		t.Fatalf("edited connector retained stale approvals: %+v", changed.Edges[0])
	}
	if _, err := service.ImportTreeDraft(ctx, draft.ID, owner.User.ID, TreeDraftImportInput{ExpectedRevision: changed.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("import after reviewed connector changed = %v, want conflict", err)
	}
	changedEdges = append([]TreeDraftEdge(nil), changed.Edges...)
	changedEdges[0].Status = "reviewed"
	firstReviewAfterEdit, err := service.SaveTreeDraft(ctx, draft.ID, owner.User.ID, SaveTreeDraftInput{ExpectedRevision: changed.Revision, TitleAR: draft.TitleAR, Nodes: changed.Nodes, Edges: changedEdges})
	if err != nil || firstReviewAfterEdit.Edges[0].ReviewerCount != 1 {
		t.Fatalf("review connector revision after edit = %+v, %v", firstReviewAfterEdit.Edges, err)
	}
	secondReviewAfterEdit, err := service.GetTreeDraft(ctx, draft.ID, secondReviewer.User.ID)
	if err != nil || secondReviewAfterEdit.Edges[0].ReviewerCount != 1 || secondReviewAfterEdit.Edges[0].ReviewedByMe {
		t.Fatalf("second reviewer view after geometry edit = %+v, %v", secondReviewAfterEdit.Edges, err)
	}
	finalReview, err := service.SaveTreeDraft(ctx, draft.ID, secondReviewer.User.ID, SaveTreeDraftInput{ExpectedRevision: firstReviewAfterEdit.Revision, TitleAR: draft.TitleAR, Nodes: firstReviewAfterEdit.Nodes, Edges: firstReviewAfterEdit.Edges})
	if err != nil || finalReview.Edges[0].ReviewerCount != 2 {
		t.Fatalf("independent second review after edit = %+v, %v", finalReview.Edges, err)
	}
	imported, err := confirmTreeDraftImport(service, ctx, draft.ID, owner.User.ID, finalReview.Revision)
	if err != nil {
		t.Fatalf("import reviewed graph with final edge decisions %+v: %v", finalReview.Edges, err)
	}
	if imported.ImportedPeople != 2 || imported.ImportedRelations != 1 || imported.StagedRelations != 0 {
		t.Fatalf("import result = %+v", imported)
	}
	nameSearch := search.NewService(fixture.Pool())
	ownerNames, err := nameSearch.Search(ctx, search.Input{ActorID: owner.User.ID, Query: privateProbe, Kind: "names"})
	if err != nil {
		t.Fatalf("owner search for private imported person: %v", err)
	}
	strangerNames, err := nameSearch.Search(ctx, search.Input{ActorID: stranger.User.ID, Query: privateProbe, Kind: "names"})
	if err != nil {
		t.Fatalf("stranger search for private imported person: %v", err)
	}
	containsImportedPerson := func(response search.Response) bool {
		for _, group := range response.Groups {
			for _, item := range group.Items {
				if item.Kind == "person" && item.Title == privateProbe {
					return true
				}
			}
		}
		return false
	}
	if !containsImportedPerson(ownerNames) || containsImportedPerson(strangerNames) {
		t.Fatalf("private imported person visibility leaked or was hidden from owner: owner=%+v stranger=%+v", ownerNames.Groups, strangerNames.Groups)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_versions WHERE tree_id = $1 AND state = 'draft'`, imported.TreeID); got != 1 {
		t.Fatalf("draft tree versions = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_nodes n JOIN tree_versions v ON v.id = n.tree_version_id WHERE v.tree_id = $1`, imported.TreeID); got != 2 {
		t.Fatalf("imported tree nodes = %d, want 2", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_relationships r JOIN tree_versions v ON v.id = r.tree_version_id WHERE v.tree_id = $1 AND r.status = 'unresolved' AND r.source_id = $2`, imported.TreeID, source.Source.ID); got != 1 {
		t.Fatalf("source-backed unresolved relationships = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_tree_import_node_evidence e JOIN source_tree_draft_imports i ON i.id = e.import_id WHERE i.draft_id = $1 AND e.status = 'active'`, draft.ID); got != 2 {
		t.Fatalf("node evidence links = %d, want 2", got)
	}
	retracted, err := service.RetractTreeDraft(ctx, draft.ID, owner.User.ID)
	if err != nil {
		t.Fatalf("retract imported draft: %v", err)
	}
	if retracted.SupersedingVersionID == "" || retracted.ExcludedNodes != 2 || retracted.ExcludedRelations != 1 {
		t.Fatalf("retraction result = %+v", retracted)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_versions WHERE tree_id = $1 AND state = 'draft'`, imported.TreeID); got != 1 {
		t.Fatalf("superseding drafts = %d, want exactly one", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_nodes WHERE tree_version_id = $1`, retracted.SupersedingVersionID); got != 0 {
		t.Fatalf("superseding version has %d imported nodes, want 0", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_tree_draft_imports WHERE draft_id = $1 AND status = 'retracted'`, draft.ID); got != 1 {
		t.Fatalf("retraction ledger rows = %d, want 1", got)
	}
}

func TestTreeDraftRetractionRefusesLaterRelationshipDependency(t *testing.T) {
	// The successful import journey above proves the clean case. The service's
	// dependency guard is exercised separately through a direct SQL fixture so a
	// future retraction change cannot cascade away a later research edit.
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "مراجع العلاقات")
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	uploader := NewService(fixture.Pool(), store, jobs.NewService(fixture.Pool()), nil, NewPDFExtractor())
	ctx := fixture.Ctx()
	source, err := evidence.NewService(fixture.Pool()).CreateSource(ctx, owner.User.ID, evidence.CreateSourceInput{TitleAR: "شجرة اعتماد " + fixture.Unique("t"), SourceType: "family_document"})
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
	fixture.Exec(`INSERT INTO source_page_evidence (run_id, source_id, source_file_id, page_number, page_width, page_height, extraction_status, selected_method, text_ar, readings, regions, region_count) VALUES ($1, $2, $3, 1, 100, 200, 'readable', 'tesseract_ocr', 'محمد خالد', '[]'::jsonb, '[{"method":"tesseract_ocr","kind":"word","text":"محمد","x":0.7,"y":0.1,"width":0.08,"height":0.02},{"method":"tesseract_ocr","kind":"word","text":"خالد","x":0.7,"y":0.3,"width":0.08,"height":0.02}]'::jsonb, 2)`, runID, source.Source.ID, file.ID)
	service := NewService(fixture.Pool(), nil, nil, nil, nil)
	draft, err := service.CreateTreeDraft(ctx, file.ID, owner.User.ID, "شجرة اعتماد")
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	for index := range draft.Nodes {
		draft.Nodes[index].Included, draft.Nodes[index].Reviewed, draft.Nodes[index].Status = true, true, "reviewed"
	}
	draft.Edges = []TreeDraftEdge{{ID: uuid.NewString(), SubjectNodeID: draft.Nodes[0].ID, ObjectNodeID: draft.Nodes[1].ID, Predicate: "parent_of", PageEvidenceID: draft.Nodes[0].PageEvidenceID, Path: []TreeDraftPoint{{X: .74, Y: .12}, {X: .74, Y: .32}}, Status: "reviewed"}}
	saved, err := service.SaveTreeDraft(ctx, draft.ID, owner.User.ID, SaveTreeDraftInput{ExpectedRevision: 1, TitleAR: draft.TitleAR, Nodes: draft.Nodes, Edges: draft.Edges})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}
	imported, err := confirmTreeDraftImport(service, ctx, draft.ID, owner.User.ID, saved.Revision)
	if err != nil {
		t.Fatalf("import draft: %v", err)
	}
	var versionID, subjectID, objectID string
	if err := fixture.QueryRow(`SELECT id::text FROM tree_versions WHERE tree_id = $1`, imported.TreeID).Scan(&versionID); err != nil {
		t.Fatalf("get tree version: %v", err)
	}
	if err := fixture.QueryRow(`SELECT subject_node_id::text, object_node_id::text FROM tree_relationships WHERE tree_version_id = $1`, versionID).Scan(&subjectID, &objectID); err != nil {
		t.Fatalf("get imported relationship: %v", err)
	}
	fixture.Exec(`INSERT INTO tree_relationships (tree_version_id, subject_node_id, object_node_id, predicate, status, created_by) VALUES ($1, $2, $3, 'sibling_of', 'unresolved', $4)`, versionID, subjectID, objectID, owner.User.ID)
	if _, err := service.RetractTreeDraft(ctx, draft.ID, owner.User.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("retraction with later relation dependency = %v, want conflict", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_relationships WHERE tree_version_id = $1`, versionID); got != 2 {
		t.Fatalf("later work was changed during rejected retraction; relationships = %d, want 2", got)
	}
}
