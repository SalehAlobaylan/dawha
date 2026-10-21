package sourceprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	MaxTreeDraftNodes  = 2000
	MaxTreeDraftEdges  = 5000
	MaxConnectorPoints = 300
)

type TreeDraftPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// TreeDraftNode is an occurrence on the source page, not a canonical person.
// Equal names deliberately remain separate until a researcher resolves them.
type TreeDraftNode struct {
	ID             string     `json:"id"`
	NameAR         string     `json:"nameAr"`
	PageEvidenceID string     `json:"pageEvidenceId"`
	Region         PageRegion `json:"region"`
	Included       bool       `json:"included"`
	Reviewed       bool       `json:"reviewed"`
	Status         string     `json:"status"`
	NoteAR         string     `json:"noteAr,omitempty"`
}

// TreeDraftEdge records a source interpretation. A reviewed edge requires a
// reviewer-traced connector path or the reviewer must leave it unresolved.
type TreeDraftEdge struct {
	ID                   string           `json:"id"`
	SubjectNodeID        string           `json:"subjectNodeId"`
	ObjectNodeID         string           `json:"objectNodeId"`
	Predicate            string           `json:"predicate"`
	PageEvidenceID       string           `json:"pageEvidenceId"`
	Path                 []TreeDraftPoint `json:"path"`
	Status               string           `json:"status"`
	SecondReviewRequired bool             `json:"secondReviewRequired"`
	ReviewerCount        int              `json:"reviewerCount"`
	ReviewedByMe         bool             `json:"reviewedByMe"`
	NoteAR               string           `json:"noteAr,omitempty"`
}

type TreeDraftPage struct {
	PageNumber              int          `json:"pageNumber"`
	PageEvidenceID          string       `json:"pageEvidenceId"`
	Width                   float64      `json:"width"`
	Height                  float64      `json:"height"`
	LineCandidates          []PageRegion `json:"lineCandidates"`
	LineCandidatesTruncated bool         `json:"lineCandidatesTruncated"`
}

type TreeDraftView struct {
	ID              string          `json:"id"`
	SourceID        string          `json:"sourceId"`
	SourceFileID    string          `json:"sourceFileId"`
	ProcessingRunID string          `json:"processingRunId,omitempty"`
	TitleAR         string          `json:"titleAr"`
	State           string          `json:"state"`
	Revision        int             `json:"revision"`
	TreeID          string          `json:"treeId,omitempty"`
	TreeVersionID   string          `json:"treeVersionId,omitempty"`
	Nodes           []TreeDraftNode `json:"nodes"`
	Edges           []TreeDraftEdge `json:"edges"`
	Pages           []TreeDraftPage `json:"pages"`
	Warnings        []string        `json:"warnings"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

type SaveTreeDraftInput struct {
	ExpectedRevision int             `json:"expectedRevision"`
	TitleAR          string          `json:"titleAr"`
	Nodes            []TreeDraftNode `json:"nodes"`
	Edges            []TreeDraftEdge `json:"edges"`
}

type TreeDraftImportResult struct {
	DraftID           string   `json:"draftId"`
	TreeID            string   `json:"treeId"`
	TreeVersionID     string   `json:"treeVersionId"`
	ImportedPeople    int      `json:"importedPeople"`
	ImportedRelations int      `json:"importedRelations"`
	StagedRelations   int      `json:"stagedRelations"`
	Warnings          []string `json:"warnings"`
}

type TreeDraftRetractionResult struct {
	DraftID              string `json:"draftId"`
	TreeID               string `json:"treeId"`
	SupersedingVersionID string `json:"supersedingVersionId"`
	ExcludedNodes        int    `json:"excludedNodes"`
	ExcludedRelations    int    `json:"excludedRelations"`
}

type treeDraftPayload struct {
	Nodes    []TreeDraftNode `json:"nodes"`
	Edges    []TreeDraftEdge `json:"edges"`
	Pages    []TreeDraftPage `json:"pages"`
	Warnings []string        `json:"warnings,omitempty"`
}

type treeDraftRow struct {
	ID              uuid.UUID
	SourceID        uuid.UUID
	SourceFileID    uuid.UUID
	ProcessingRunID pgtype.UUID
	TitleAR         string
	State           string
	Revision        int
	TreeID          pgtype.UUID
	TreeVersionID   pgtype.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreateTreeDraft creates a conservative first graph from text boxes that the
// independent PDF readers already located. Boxes are suggestions only: every
// occurrence starts excluded and no relationship is inferred from position.
func (s *Service) CreateTreeDraft(ctx context.Context, fileID, actorID, titleAR string) (TreeDraftView, error) {
	if err := s.databaseReady(); err != nil {
		return TreeDraftView{}, err
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return TreeDraftView{}, err
	}
	titleAR = strings.TrimSpace(titleAR)
	if len([]rune(titleAR)) > 200 {
		return TreeDraftView{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDraftView{}, err
	}
	defer tx.Rollback(ctx)

	var sourceID pgtype.UUID
	var mimeType, filename, documentKind string
	if err := tx.QueryRow(ctx, `SELECT source_id, COALESCE(mime_type, ''), COALESCE(original_filename_ar, ''), document_kind FROM source_files WHERE id = $1 FOR SHARE`, fileUUID).Scan(&sourceID, &mimeType, &filename, &documentKind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeDraftView{}, ErrNotFound
		}
		return TreeDraftView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, uuidValue(sourceID), actorUUID)
	if err != nil {
		return TreeDraftView{}, err
	}
	if !allowed {
		return TreeDraftView{}, ErrForbidden
	}
	if normalizeContentType(mimeType) != "application/pdf" {
		return TreeDraftView{}, ErrUnsupportedDocument
	}
	var processingRunID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM source_processing_runs
		WHERE source_file_id = $1 AND run_kind = 'document' AND status = 'succeeded'
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, fileUUID).Scan(&processingRunID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeDraftView{}, ErrConflict
		}
		return TreeDraftView{}, err
	}
	var existing pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM source_tree_drafts WHERE source_file_id = $1 AND source_processing_run_id = $2 AND state = 'draft'`, fileUUID, processingRunID).Scan(&existing)
	if err == nil {
		_ = tx.Rollback(ctx)
		return s.GetTreeDraft(ctx, existing.String(), actorUUID.String())
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TreeDraftView{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, page_number, page_width, page_height, selected_method, regions
		FROM source_page_evidence WHERE source_file_id = $1 AND run_id = $2
		ORDER BY page_number
	`, fileUUID, processingRunID)
	if err != nil {
		return TreeDraftView{}, err
	}
	payload := treeDraftPayload{Nodes: []TreeDraftNode{}, Edges: []TreeDraftEdge{}, Pages: []TreeDraftPage{}}
	if documentKind == "family_chart" {
		payload.Warnings = append(payload.Warnings, "Family chart machine readings are unqualified; names must be entered and verified manually.")
	}
	for rows.Next() {
		var evidenceID pgtype.UUID
		var pageNumber int
		var width, height float64
		var method string
		var regionJSON []byte
		if err := rows.Scan(&evidenceID, &pageNumber, &width, &height, &method, &regionJSON); err != nil {
			rows.Close()
			return TreeDraftView{}, err
		}
		var regions []PageRegionEvidence
		if err := json.Unmarshal(regionJSON, &regions); err != nil {
			rows.Close()
			return TreeDraftView{}, ErrValidation
		}
		if documentKind != "family_chart" {
			for _, region := range proposeNameRegions(evidenceID.String(), pageNumber, method, regions) {
				payload.Nodes = append(payload.Nodes, region)
				if len(payload.Nodes) > MaxTreeDraftNodes {
					rows.Close()
					return TreeDraftView{}, ErrValidation
				}
			}
		}
		pageView := TreeDraftPage{PageNumber: pageNumber, PageEvidenceID: uuidValue(evidenceID).String(), Width: width, Height: height, LineCandidates: []PageRegion{}}
		for _, region := range regions {
			if region.Method == "local_line_detection" && region.Kind == "connector_segment" && validPageRegion(region.PageRegion) {
				pageView.LineCandidates = append(pageView.LineCandidates, region.PageRegion)
			}
		}
		pageView.LineCandidatesTruncated = len(pageView.LineCandidates) >= MaxLineCandidates
		payload.Pages = append(payload.Pages, pageView)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TreeDraftView{}, err
	}
	rows.Close()
	if len(payload.Nodes) == 0 {
		// Empty extraction is still useful: the reviewer can add source boxes by
		// hand after inspecting the original page. It is never described as empty
		// content or as a successful family-tree reading.
	}
	if titleAR == "" {
		titleAR = strings.TrimSuffix(filename, filepathExt(filename))
	}
	if strings.TrimSpace(titleAR) == "" {
		titleAR = "مسودة شجرة من المصدر"
	}
	draftID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO source_tree_drafts (id, source_id, source_file_id, source_processing_run_id, created_by, title_ar) VALUES ($1, $2, $3, $4, $5, $6)`, draftID, uuidValue(sourceID), fileUUID, processingRunID, actorUUID, titleAR); err != nil {
		return TreeDraftView{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_tree_draft_revisions (draft_id, revision_number, reviewer_id, payload) VALUES ($1, 1, $2, $3)`, draftID, actorUUID, mustJSON(payload)); err != nil {
		return TreeDraftView{}, err
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_tree_draft_created", "source_tree_draft", draftID, nil, map[string]any{"sourceFileId": fileUUID.String(), "processingRunId": processingRunID.String(), "proposedNodeCount": len(payload.Nodes)}); err != nil {
		return TreeDraftView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDraftView{}, err
	}
	return s.GetTreeDraft(ctx, draftID.String(), actorUUID.String())
}

// GetTreeDraft returns only drafts whose source the caller may review.
func (s *Service) GetTreeDraft(ctx context.Context, draftID, actorID string) (TreeDraftView, error) {
	if err := s.databaseReady(); err != nil {
		return TreeDraftView{}, err
	}
	draftUUID, actorUUID, err := parseIDs(draftID, actorID)
	if err != nil {
		return TreeDraftView{}, err
	}
	row, err := readTreeDraft(ctx, s.Pool, draftUUID)
	if err != nil {
		return TreeDraftView{}, err
	}
	allowed, err := canReviewSource(ctx, s.Pool, row.SourceID, actorUUID)
	if err != nil {
		return TreeDraftView{}, err
	}
	if !allowed {
		return TreeDraftView{}, ErrForbidden
	}
	var body []byte
	if err := s.Pool.QueryRow(ctx, `SELECT payload FROM source_tree_draft_revisions WHERE draft_id = $1 AND revision_number = $2`, row.ID, row.Revision).Scan(&body); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeDraftView{}, ErrNotFound
		}
		return TreeDraftView{}, err
	}
	view, err := treeDraftView(row, body)
	if err != nil {
		return TreeDraftView{}, err
	}
	if err := hydrateTreeDraftEdgeReviews(ctx, s.Pool, draftUUID, actorUUID, &view); err != nil {
		return TreeDraftView{}, err
	}
	return view, nil
}

// SaveTreeDraft appends a complete graph revision. Old geometry and decisions
// remain available, and stale browser copies cannot overwrite newer review.
func (s *Service) SaveTreeDraft(ctx context.Context, draftID, actorID string, input SaveTreeDraftInput) (TreeDraftView, error) {
	if err := s.databaseReady(); err != nil {
		return TreeDraftView{}, err
	}
	draftUUID, actorUUID, err := parseIDs(draftID, actorID)
	if err != nil {
		return TreeDraftView{}, err
	}
	input, err = validateTreeDraftInput(input)
	if err != nil {
		return TreeDraftView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDraftView{}, err
	}
	defer tx.Rollback(ctx)
	row, err := readTreeDraftForUpdate(ctx, tx, draftUUID)
	if err != nil {
		return TreeDraftView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, row.SourceID, actorUUID)
	if err != nil {
		return TreeDraftView{}, err
	}
	if !allowed {
		return TreeDraftView{}, ErrForbidden
	}
	if row.State != "draft" || input.ExpectedRevision != row.Revision {
		return TreeDraftView{}, ErrConflict
	}
	var priorBody []byte
	if err := tx.QueryRow(ctx, `SELECT payload FROM source_tree_draft_revisions WHERE draft_id = $1 AND revision_number = $2`, draftUUID, row.Revision).Scan(&priorBody); err != nil {
		return TreeDraftView{}, err
	}
	var prior treeDraftPayload
	if err := json.Unmarshal(priorBody, &prior); err != nil {
		return TreeDraftView{}, ErrValidation
	}
	priorNodes := make(map[string]TreeDraftNode, len(prior.Nodes))
	priorEdges := make(map[string]TreeDraftEdge, len(prior.Edges))
	for _, node := range prior.Nodes {
		priorNodes[node.ID] = node
	}
	for _, edge := range prior.Edges {
		priorEdges[edge.ID] = edge
	}
	currentNodes := make(map[string]TreeDraftNode, len(input.Nodes))
	for index := range input.Nodes {
		node := &input.Nodes[index]
		if previous, exists := priorNodes[node.ID]; exists && treeDraftNodeFingerprint(previous) != treeDraftNodeFingerprint(*node) {
			// Correcting the visible label or box is a new interpretation. Keep
			// the occurrence, but require it to be checked again before import.
			node.Included, node.Reviewed, node.Status = false, false, "proposed"
		}
		currentNodes[node.ID] = *node
	}
	for index := range input.Edges {
		edge := &input.Edges[index]
		if previous, exists := priorEdges[edge.ID]; exists && treeDraftEdgeFingerprint(previous, priorNodes) != treeDraftEdgeFingerprint(*edge, currentNodes) {
			// Do not carry a reviewed decision to a changed path, endpoint, label,
			// relation, or page. The old fingerprint remains in the audit ledger.
			edge.Status = "proposed"
		}
	}
	if err := validateReviewedEdgePaths(input.Nodes, input.Edges); err != nil {
		return TreeDraftView{}, err
	}
	if err := validateDraftEvidence(ctx, tx, row.SourceFileID, row.ProcessingRunID, input.Nodes, input.Edges); err != nil {
		return TreeDraftView{}, err
	}
	payload := treeDraftPayload{Nodes: input.Nodes, Edges: input.Edges, Pages: prior.Pages, Warnings: prior.Warnings}
	if _, err := tx.Exec(ctx, `INSERT INTO source_tree_draft_revisions (draft_id, revision_number, reviewer_id, payload) VALUES ($1, $2, $3, $4)`, draftUUID, row.Revision+1, actorUUID, mustJSON(payload)); err != nil {
		return TreeDraftView{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_tree_drafts SET title_ar = $1, current_revision = $2, updated_at = now() WHERE id = $3`, input.TitleAR, row.Revision+1, draftUUID); err != nil {
		return TreeDraftView{}, err
	}
	for _, edge := range input.Edges {
		if edge.Status != "reviewed" {
			continue
		}
		fingerprint := treeDraftEdgeFingerprint(edge, currentNodes)
		if _, err := tx.Exec(ctx, `INSERT INTO source_tree_draft_edge_reviews (draft_id, edge_id, edge_fingerprint, reviewer_id, revision_number) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (draft_id, edge_id, edge_fingerprint, reviewer_id) DO NOTHING`, draftUUID, uuid.MustParse(edge.ID), fingerprint, actorUUID, row.Revision+1); err != nil {
			return TreeDraftView{}, err
		}
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_tree_draft_reviewed", "source_tree_draft", draftUUID, map[string]any{"revision": row.Revision}, map[string]any{"revision": row.Revision + 1, "includedNodes": countIncludedNodes(input.Nodes), "reviewedEdges": countReviewedEdges(input.Edges)}); err != nil {
		return TreeDraftView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDraftView{}, err
	}
	return s.GetTreeDraft(ctx, draftUUID.String(), actorUUID.String())
}

// ImportTreeDraft creates a new private draft tree. It never merges people by
// name and leaves imported relationships unresolved in Dawha's tree model.
func (s *Service) ImportTreeDraft(ctx context.Context, draftID, actorID string, expectedRevision int) (TreeDraftImportResult, error) {
	if err := s.databaseReady(); err != nil {
		return TreeDraftImportResult{}, err
	}
	draftUUID, actorUUID, err := parseIDs(draftID, actorID)
	if err != nil {
		return TreeDraftImportResult{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDraftImportResult{}, err
	}
	defer tx.Rollback(ctx)
	row, err := readTreeDraftForUpdate(ctx, tx, draftUUID)
	if err != nil {
		return TreeDraftImportResult{}, err
	}
	allowed, err := canReviewSource(ctx, tx, row.SourceID, actorUUID)
	if err != nil {
		return TreeDraftImportResult{}, err
	}
	if !allowed {
		return TreeDraftImportResult{}, ErrForbidden
	}
	if row.State != "draft" || row.Revision != expectedRevision {
		return TreeDraftImportResult{}, fmt.Errorf("tree draft state or revision changed: %w", ErrConflict)
	}
	var body []byte
	if err := tx.QueryRow(ctx, `SELECT payload FROM source_tree_draft_revisions WHERE draft_id = $1 AND revision_number = $2`, draftUUID, row.Revision).Scan(&body); err != nil {
		return TreeDraftImportResult{}, err
	}
	view, err := treeDraftView(row, body)
	if err != nil {
		return TreeDraftImportResult{}, err
	}
	if err := validateImportableGraph(view.Nodes, view.Edges); err != nil {
		return TreeDraftImportResult{}, fmt.Errorf("tree draft still has unresolved review items: %w", err)
	}
	nodesByID := make(map[string]TreeDraftNode, len(view.Nodes))
	for _, node := range view.Nodes {
		nodesByID[node.ID] = node
	}
	for _, edge := range view.Edges {
		if edge.Status == "rejected" || !edge.SecondReviewRequired {
			continue
		}
		if edge.Status != "reviewed" {
			return TreeDraftImportResult{}, fmt.Errorf("a connector requiring independent review is not yet reviewed: %w", ErrConflict)
		}
		var reviewerCount int
		fingerprint := treeDraftEdgeFingerprint(edge, nodesByID)
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_tree_draft_edge_reviews WHERE draft_id = $1 AND edge_id = $2 AND edge_fingerprint = $3`, draftUUID, uuid.MustParse(edge.ID), fingerprint).Scan(&reviewerCount); err != nil {
			return TreeDraftImportResult{}, err
		}
		if reviewerCount < 2 {
			return TreeDraftImportResult{}, fmt.Errorf("a connector needs two distinct reviewers: %w", ErrConflict)
		}
	}

	var sourceTitle string
	if err := tx.QueryRow(ctx, `SELECT title_ar FROM sources WHERE id = $1`, row.SourceID).Scan(&sourceTitle); err != nil {
		return TreeDraftImportResult{}, err
	}
	treeID, versionID := uuid.New(), uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO trees (id, name_ar, description_ar, visibility, owner_id) VALUES ($1, $2, $3, 'private', $4)`, treeID, row.TitleAR, "مسودة خاصة مستوردة من مصدر: "+sourceTitle, actorUUID); err != nil {
		return TreeDraftImportResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`, versionID, treeID, actorUUID); err != nil {
		return TreeDraftImportResult{}, err
	}
	nodeIDMap := make(map[string]uuid.UUID)
	manifestNodes := make([]map[string]any, 0)
	sort.SliceStable(view.Nodes, func(i, j int) bool { return view.Nodes[i].ID < view.Nodes[j].ID })
	for index, node := range view.Nodes {
		if !node.Included {
			continue
		}
		personID, treeNodeID := uuid.New(), uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, gender, created_by) VALUES ($1, $2, $3, 'unknown', $4)`, personID, node.NameAR, identity.NormalizeArabicName(node.NameAR), actorUUID); err != nil {
			return TreeDraftImportResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, $5)`, treeNodeID, versionID, personID, node.NameAR, index); err != nil {
			return TreeDraftImportResult{}, err
		}
		nodeIDMap[node.ID] = treeNodeID
		manifestNodes = append(manifestNodes, map[string]any{"draftNodeId": node.ID, "treeNodeId": treeNodeID.String(), "personId": personID.String(), "nameAr": node.NameAR})
	}
	manifestEdges := make([]map[string]any, 0)
	result := TreeDraftImportResult{DraftID: draftUUID.String(), TreeID: treeID.String(), TreeVersionID: versionID.String(), ImportedPeople: len(nodeIDMap), Warnings: []string{}}
	for _, edge := range view.Edges {
		if edge.Status != "reviewed" {
			continue
		}
		if !supportedTreePredicate(edge.Predicate) {
			result.StagedRelations++
			result.Warnings = append(result.Warnings, "A reviewed relationship uses a meaning Dawha's tree model cannot yet represent; it remains in the source draft.")
			continue
		}
		subject, subjectOK := nodeIDMap[edge.SubjectNodeID]
		object, objectOK := nodeIDMap[edge.ObjectNodeID]
		if !subjectOK || !objectOK {
			return TreeDraftImportResult{}, ErrValidation
		}
		relationshipID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by) VALUES ($1, $2, $3, $4, $5, 'unresolved', $6, $7)`, relationshipID, versionID, subject, object, edge.Predicate, row.SourceID, actorUUID); err != nil {
			return TreeDraftImportResult{}, err
		}
		manifestEdges = append(manifestEdges, map[string]any{"draftEdgeId": edge.ID, "treeRelationshipId": relationshipID.String(), "subjectNodeId": subject.String(), "objectNodeId": object.String(), "predicate": edge.Predicate})
		result.ImportedRelations++
	}
	manifest := map[string]any{"sourceId": row.SourceID.String(), "sourceFileId": row.SourceFileID.String(), "draftRevision": row.Revision, "nodes": manifestNodes, "edges": manifestEdges}
	importID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO source_tree_draft_imports (id, draft_id, actor_id, tree_id, tree_version_id, manifest) VALUES ($1, $2, $3, $4, $5, $6)`, importID, draftUUID, actorUUID, treeID, versionID, mustJSON(manifest)); err != nil {
		return TreeDraftImportResult{}, err
	}
	for _, node := range view.Nodes {
		treeNodeID, included := nodeIDMap[node.ID]
		if !included {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO source_tree_import_node_evidence (import_id, tree_node_id, draft_node_id, page_evidence_id, label_ar, region) VALUES ($1, $2, $3, $4, $5, $6)`, importID, treeNodeID, uuid.MustParse(node.ID), uuid.MustParse(node.PageEvidenceID), node.NameAR, mustJSON(node.Region)); err != nil {
			return TreeDraftImportResult{}, err
		}
	}
	for _, edge := range view.Edges {
		for _, record := range manifestEdges {
			if record["draftEdgeId"] != edge.ID {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO source_tree_import_edge_evidence (import_id, tree_relationship_id, draft_edge_id, page_evidence_id, connector_path) VALUES ($1, $2, $3, $4, $5)`, importID, uuid.MustParse(record["treeRelationshipId"].(string)), uuid.MustParse(edge.ID), uuid.MustParse(edge.PageEvidenceID), mustJSON(edge.Path)); err != nil {
				return TreeDraftImportResult{}, err
			}
			break
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE source_tree_drafts SET state = 'imported', tree_id = $1, tree_version_id = $2, updated_at = now() WHERE id = $3`, treeID, versionID, draftUUID); err != nil {
		return TreeDraftImportResult{}, err
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_tree_draft_imported", "tree", treeID, nil, map[string]any{"draftId": draftUUID.String(), "treeVersionId": versionID.String(), "people": result.ImportedPeople, "relationships": result.ImportedRelations, "stagedRelationships": result.StagedRelations}); err != nil {
		return TreeDraftImportResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDraftImportResult{}, err
	}
	return result, nil
}

// RetractTreeDraft supersedes the imported version with a new draft that omits
// only rows owned by this import. It refuses when imported people have acquired
// other relationships, protecting later researcher work.
func (s *Service) RetractTreeDraft(ctx context.Context, draftID, actorID string) (TreeDraftRetractionResult, error) {
	if err := s.databaseReady(); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	draftUUID, actorUUID, err := parseIDs(draftID, actorID)
	if err != nil {
		return TreeDraftRetractionResult{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDraftRetractionResult{}, err
	}
	defer tx.Rollback(ctx)
	row, err := readTreeDraftForUpdate(ctx, tx, draftUUID)
	if err != nil {
		return TreeDraftRetractionResult{}, err
	}
	allowed, err := canReviewSource(ctx, tx, row.SourceID, actorUUID)
	if err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if !allowed {
		return TreeDraftRetractionResult{}, ErrForbidden
	}
	if row.State != "imported" || !row.TreeID.Valid || !row.TreeVersionID.Valid {
		return TreeDraftRetractionResult{}, ErrConflict
	}
	var importID pgtype.UUID
	var status string
	if err := tx.QueryRow(ctx, `SELECT id, status FROM source_tree_draft_imports WHERE draft_id = $1 FOR UPDATE`, draftUUID).Scan(&importID, &status); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if status == "retracted" {
		return TreeDraftRetractionResult{}, ErrConflict
	}
	var versionState string
	if err := tx.QueryRow(ctx, `SELECT state FROM tree_versions WHERE id = $1 FOR UPDATE`, uuidValue(row.TreeVersionID)).Scan(&versionState); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if versionState != "draft" {
		return TreeDraftRetractionResult{}, ErrConflict
	}
	var importedNodeIDs []uuid.UUID
	rows, err := tx.Query(ctx, `SELECT tree_node_id FROM source_tree_import_node_evidence WHERE import_id = $1 ORDER BY tree_node_id`, uuidValue(importID))
	if err != nil {
		return TreeDraftRetractionResult{}, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return TreeDraftRetractionResult{}, err
		}
		importedNodeIDs = append(importedNodeIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TreeDraftRetractionResult{}, err
	}
	rows.Close()
	var importedRelationIDs []uuid.UUID
	rows, err = tx.Query(ctx, `SELECT tree_relationship_id FROM source_tree_import_edge_evidence WHERE import_id = $1 ORDER BY tree_relationship_id`, uuidValue(importID))
	if err != nil {
		return TreeDraftRetractionResult{}, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return TreeDraftRetractionResult{}, err
		}
		importedRelationIDs = append(importedRelationIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TreeDraftRetractionResult{}, err
	}
	rows.Close()
	if len(importedNodeIDs) == 0 {
		return TreeDraftRetractionResult{}, ErrConflict
	}
	var dependentCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM tree_relationships
		WHERE tree_version_id = $1 AND (subject_node_id = ANY($2) OR object_node_id = ANY($2))
		  AND NOT (id = ANY($3))
	`, uuidValue(row.TreeVersionID), importedNodeIDs, importedRelationIDs).Scan(&dependentCount); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if dependentCount > 0 {
		return TreeDraftRetractionResult{}, ErrConflict
	}
	// Imported row content must still match its ledger. A researcher edit to an
	// imported name or relationship needs a deliberate manual resolution.
	var changedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM source_tree_import_node_evidence evidence
		JOIN tree_nodes node ON node.id = evidence.tree_node_id
		JOIN people person ON person.id = node.person_id
		WHERE evidence.import_id = $1 AND (node.display_name_ar <> evidence.label_ar OR person.canonical_name_ar <> evidence.label_ar)
	`, uuidValue(importID)).Scan(&changedCount); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if changedCount > 0 {
		return TreeDraftRetractionResult{}, ErrConflict
	}
	var changedRelations int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM source_tree_import_edge_evidence evidence
		JOIN tree_relationships relation ON relation.id = evidence.tree_relationship_id
		WHERE evidence.import_id = $1 AND (relation.status <> 'unresolved' OR relation.source_id <> $2)
	`, uuidValue(importID), row.SourceID).Scan(&changedRelations); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if changedRelations > 0 {
		return TreeDraftRetractionResult{}, ErrConflict
	}

	newVersionID := uuid.New()
	var nextVersion int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version_number), 0) + 1 FROM tree_versions WHERE tree_id = $1`, uuidValue(row.TreeID)).Scan(&nextVersion); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tree_versions SET state = 'archived' WHERE id = $1 AND state = 'draft'`, uuidValue(row.TreeVersionID)); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, $3, 'draft', $4)`, newVersionID, uuidValue(row.TreeID), nextVersion, actorUUID); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	var removedRelations int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tree_relationships WHERE tree_version_id = $1`, uuidValue(row.TreeVersionID)).Scan(&removedRelations); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	removedRelations = len(importedRelationIDs)
	importedNodes := make(map[uuid.UUID]bool, len(importedNodeIDs))
	for _, nodeID := range importedNodeIDs {
		importedNodes[nodeID] = true
	}
	rows, err = tx.Query(ctx, `SELECT id, person_id, display_name_ar, sort_order FROM tree_nodes WHERE tree_version_id = $1 ORDER BY sort_order, id`, uuidValue(row.TreeVersionID))
	if err != nil {
		return TreeDraftRetractionResult{}, err
	}
	nodeMap := make(map[uuid.UUID]uuid.UUID)
	type treeNodeRow struct {
		id, personID uuid.UUID
		display      string
		sort         int
	}
	var keptNodes []treeNodeRow
	for rows.Next() {
		var item treeNodeRow
		if err := rows.Scan(&item.id, &item.personID, &item.display, &item.sort); err != nil {
			rows.Close()
			return TreeDraftRetractionResult{}, err
		}
		if importedNodes[item.id] {
			continue
		}
		newNodeID := uuid.New()
		nodeMap[item.id] = newNodeID
		item.id = newNodeID
		keptNodes = append(keptNodes, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TreeDraftRetractionResult{}, err
	}
	rows.Close()
	for _, node := range keptNodes {
		if _, err := tx.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, $5)`, node.id, newVersionID, node.personID, node.display, node.sort); err != nil {
			return TreeDraftRetractionResult{}, err
		}
	}
	if len(nodeMap) > 0 {
		rows, err = tx.Query(ctx, `SELECT subject_node_id, object_node_id, predicate, status, source_id, created_by FROM tree_relationships WHERE tree_version_id = $1 AND NOT (id = ANY($2))`, uuidValue(row.TreeVersionID), importedRelationIDs)
		if err != nil {
			return TreeDraftRetractionResult{}, err
		}
		type relationRow struct {
			subject, object     uuid.UUID
			predicate, status   string
			sourceID, createdBy pgtype.UUID
		}
		var keptRelations []relationRow
		for rows.Next() {
			var item relationRow
			if err := rows.Scan(&item.subject, &item.object, &item.predicate, &item.status, &item.sourceID, &item.createdBy); err != nil {
				rows.Close()
				return TreeDraftRetractionResult{}, err
			}
			newSubject, subjectOK := nodeMap[item.subject]
			newObject, objectOK := nodeMap[item.object]
			if !subjectOK || !objectOK {
				continue
			}
			item.subject, item.object = newSubject, newObject
			keptRelations = append(keptRelations, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return TreeDraftRetractionResult{}, err
		}
		rows.Close()
		for _, relation := range keptRelations {
			if _, err := tx.Exec(ctx, `INSERT INTO tree_relationships (tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`, newVersionID, relation.subject, relation.object, relation.predicate, relation.status, relation.sourceID, relation.createdBy); err != nil {
				return TreeDraftRetractionResult{}, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE source_tree_draft_imports SET status = 'retracted', retracted_at = now() WHERE id = $1`, uuidValue(importID)); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_tree_import_node_evidence SET status = 'retracted' WHERE import_id = $1`, uuidValue(importID)); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_tree_import_edge_evidence SET status = 'retracted' WHERE import_id = $1`, uuidValue(importID)); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_tree_drafts SET state = 'retracted', updated_at = now() WHERE id = $1`, draftUUID); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_tree_draft_retracted", "tree", uuidValue(row.TreeID), map[string]any{"versionId": uuidValue(row.TreeVersionID).String()}, map[string]any{"supersedingVersionId": newVersionID.String(), "excludedNodes": len(importedNodeIDs), "excludedRelationships": len(importedRelationIDs)}); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDraftRetractionResult{}, err
	}
	return TreeDraftRetractionResult{DraftID: draftUUID.String(), TreeID: uuidValue(row.TreeID).String(), SupersedingVersionID: newVersionID.String(), ExcludedNodes: len(importedNodeIDs), ExcludedRelations: removedRelations}, nil
}

func readTreeDraft(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id uuid.UUID) (treeDraftRow, error) {
	var row treeDraftRow
	err := q.QueryRow(ctx, `SELECT id, source_id, source_file_id, source_processing_run_id, title_ar, state, current_revision, tree_id, tree_version_id, created_at, updated_at FROM source_tree_drafts WHERE id = $1`, id).
		Scan(&row.ID, &row.SourceID, &row.SourceFileID, &row.ProcessingRunID, &row.TitleAR, &row.State, &row.Revision, &row.TreeID, &row.TreeVersionID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return treeDraftRow{}, ErrNotFound
	}
	return row, err
}

func readTreeDraftForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (treeDraftRow, error) {
	var row treeDraftRow
	err := tx.QueryRow(ctx, `SELECT id, source_id, source_file_id, source_processing_run_id, title_ar, state, current_revision, tree_id, tree_version_id, created_at, updated_at FROM source_tree_drafts WHERE id = $1 FOR UPDATE`, id).
		Scan(&row.ID, &row.SourceID, &row.SourceFileID, &row.ProcessingRunID, &row.TitleAR, &row.State, &row.Revision, &row.TreeID, &row.TreeVersionID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return treeDraftRow{}, ErrNotFound
	}
	return row, err
}

func treeDraftView(row treeDraftRow, body []byte) (TreeDraftView, error) {
	var payload treeDraftPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return TreeDraftView{}, ErrValidation
	}
	if payload.Nodes == nil {
		payload.Nodes = []TreeDraftNode{}
	}
	if payload.Edges == nil {
		payload.Edges = []TreeDraftEdge{}
	}
	if payload.Pages == nil {
		payload.Pages = []TreeDraftPage{}
	}
	warnings := append(draftWarnings(payload.Nodes, payload.Edges, payload.Pages), payload.Warnings...)
	view := TreeDraftView{ID: row.ID.String(), SourceID: row.SourceID.String(), SourceFileID: row.SourceFileID.String(), ProcessingRunID: uuidString(row.ProcessingRunID), TitleAR: row.TitleAR, State: row.State, Revision: row.Revision, Nodes: payload.Nodes, Edges: payload.Edges, Warnings: warnings, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	view.Pages = payload.Pages
	if row.TreeID.Valid {
		view.TreeID = uuidValue(row.TreeID).String()
	}
	if row.TreeVersionID.Valid {
		view.TreeVersionID = uuidValue(row.TreeVersionID).String()
	}
	return view, nil
}

func hydrateTreeDraftEdgeReviews(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, draftID, actorID uuid.UUID, view *TreeDraftView) error {
	rows, err := q.Query(ctx, `SELECT edge_id, edge_fingerprint, count(*)::integer, bool_or(reviewer_id = $2) FROM source_tree_draft_edge_reviews WHERE draft_id = $1 GROUP BY edge_id, edge_fingerprint`, draftID, actorID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type reviewSummary struct {
		count        int
		reviewedByMe bool
	}
	counts := make(map[string]reviewSummary)
	for rows.Next() {
		var edgeID uuid.UUID
		var fingerprint string
		var count int
		var actorReviewed bool
		if err := rows.Scan(&edgeID, &fingerprint, &count, &actorReviewed); err != nil {
			return err
		}
		counts[edgeID.String()+":"+fingerprint] = reviewSummary{count: count, reviewedByMe: actorReviewed}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	nodesByID := make(map[string]TreeDraftNode, len(view.Nodes))
	for _, node := range view.Nodes {
		nodesByID[node.ID] = node
	}
	for index := range view.Edges {
		edge := &view.Edges[index]
		summary := counts[edge.ID+":"+treeDraftEdgeFingerprint(*edge, nodesByID)]
		edge.ReviewerCount = summary.count
		edge.ReviewedByMe = summary.reviewedByMe
	}
	return nil
}

func validateTreeDraftInput(input SaveTreeDraftInput) (SaveTreeDraftInput, error) {
	input.TitleAR = strings.TrimSpace(input.TitleAR)
	if input.ExpectedRevision < 1 || input.TitleAR == "" || len([]rune(input.TitleAR)) > 200 || len(input.Nodes) > MaxTreeDraftNodes || len(input.Edges) > MaxTreeDraftEdges {
		return SaveTreeDraftInput{}, ErrValidation
	}
	ids := make(map[string]bool, len(input.Nodes))
	for index := range input.Nodes {
		node := &input.Nodes[index]
		node.ID = strings.TrimSpace(node.ID)
		node.NameAR = strings.TrimSpace(node.NameAR)
		node.PageEvidenceID = strings.TrimSpace(node.PageEvidenceID)
		node.Status = strings.TrimSpace(node.Status)
		node.NoteAR = strings.TrimSpace(node.NoteAR)
		if _, err := uuid.Parse(node.ID); err != nil || ids[node.ID] || node.NameAR == "" || len([]rune(node.NameAR)) > 200 || len([]rune(node.NoteAR)) > 1000 || node.Status != "proposed" && node.Status != "reviewed" && node.Status != "rejected" || !validPageRegion(node.Region) {
			return SaveTreeDraftInput{}, ErrValidation
		}
		if node.Reviewed {
			node.Status = "reviewed"
		}
		ids[node.ID] = true
	}
	edgeIDs := make(map[string]bool, len(input.Edges))
	for index := range input.Edges {
		edge := &input.Edges[index]
		edge.ID = strings.TrimSpace(edge.ID)
		edge.SubjectNodeID = strings.TrimSpace(edge.SubjectNodeID)
		edge.ObjectNodeID = strings.TrimSpace(edge.ObjectNodeID)
		edge.Predicate = strings.TrimSpace(edge.Predicate)
		edge.PageEvidenceID = strings.TrimSpace(edge.PageEvidenceID)
		edge.Status = strings.TrimSpace(edge.Status)
		edge.NoteAR = strings.TrimSpace(edge.NoteAR)
		if _, err := uuid.Parse(edge.ID); err != nil || edgeIDs[edge.ID] || !ids[edge.SubjectNodeID] || !ids[edge.ObjectNodeID] || edge.SubjectNodeID == edge.ObjectNodeID || !validStagedPredicate(edge.Predicate) || edge.Status != "proposed" && edge.Status != "reviewed" && edge.Status != "rejected" || len(edge.Path) > MaxConnectorPoints || len([]rune(edge.NoteAR)) > 1000 {
			return SaveTreeDraftInput{}, ErrValidation
		}
		edge.ReviewerCount = 0 // Hydrated from the append-only review ledger.
		edge.ReviewedByMe = false
		for _, point := range edge.Path {
			if !validUnit(point.X) || !validUnit(point.Y) {
				return SaveTreeDraftInput{}, ErrValidation
			}
		}
		edgeIDs[edge.ID] = true
	}
	return input, nil
}

func validateReviewedEdgePaths(nodes []TreeDraftNode, edges []TreeDraftEdge) error {
	nodesByID := make(map[string]TreeDraftNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
	}
	for _, edge := range edges {
		if edge.Status != "reviewed" {
			continue
		}
		if len(edge.Path) < 2 || edge.PageEvidenceID == "" {
			return ErrValidation
		}
		subject, subjectExists := nodesByID[edge.SubjectNodeID]
		object, objectExists := nodesByID[edge.ObjectNodeID]
		if !subjectExists || !objectExists || subject.PageEvidenceID != edge.PageEvidenceID || object.PageEvidenceID != edge.PageEvidenceID || !pathTouchesRegion(edge.Path[0], subject.Region, 0.035) || !pathTouchesRegion(edge.Path[len(edge.Path)-1], object.Region, 0.035) {
			return ErrValidation
		}
	}
	return nil
}

func treeDraftNodeFingerprint(node TreeDraftNode) string {
	return fingerprintJSON(struct {
		NameAR         string     `json:"nameAr"`
		PageEvidenceID string     `json:"pageEvidenceId"`
		Region         PageRegion `json:"region"`
	}{NameAR: node.NameAR, PageEvidenceID: node.PageEvidenceID, Region: node.Region})
}

func treeDraftEdgeFingerprint(edge TreeDraftEdge, nodes map[string]TreeDraftNode) string {
	subject, object := nodes[edge.SubjectNodeID], nodes[edge.ObjectNodeID]
	return fingerprintJSON(struct {
		SubjectNodeID      string           `json:"subjectNodeId"`
		ObjectNodeID       string           `json:"objectNodeId"`
		SubjectFingerprint string           `json:"subjectFingerprint"`
		ObjectFingerprint  string           `json:"objectFingerprint"`
		Predicate          string           `json:"predicate"`
		PageEvidenceID     string           `json:"pageEvidenceId"`
		Path               []TreeDraftPoint `json:"path"`
		NoteAR             string           `json:"noteAr"`
	}{SubjectNodeID: edge.SubjectNodeID, ObjectNodeID: edge.ObjectNodeID, SubjectFingerprint: treeDraftNodeFingerprint(subject), ObjectFingerprint: treeDraftNodeFingerprint(object), Predicate: edge.Predicate, PageEvidenceID: edge.PageEvidenceID, Path: edge.Path, NoteAR: edge.NoteAR})
}

func fingerprintJSON(value any) string {
	digest := sha256.Sum256(mustJSON(value))
	return hex.EncodeToString(digest[:])
}

func pathTouchesRegion(point TreeDraftPoint, region PageRegion, margin float64) bool {
	return point.X >= region.X-margin && point.X <= region.X+region.Width+margin && point.Y >= region.Y-margin && point.Y <= region.Y+region.Height+margin
}

func validateDraftEvidence(ctx context.Context, tx pgx.Tx, fileID uuid.UUID, runID pgtype.UUID, nodes []TreeDraftNode, edges []TreeDraftEdge) error {
	ids := make([]uuid.UUID, 0, len(nodes)+len(edges))
	for _, node := range nodes {
		id, err := uuid.Parse(node.PageEvidenceID)
		if err != nil {
			return ErrValidation
		}
		ids = append(ids, id)
	}
	for _, edge := range edges {
		if edge.PageEvidenceID == "" {
			continue
		}
		id, err := uuid.Parse(edge.PageEvidenceID)
		if err != nil {
			return ErrValidation
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ErrValidation
	}
	var invalid bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM unnest($1::uuid[]) requested(id) LEFT JOIN source_page_evidence p ON p.id = requested.id AND p.source_file_id = $2 AND ($3::uuid IS NULL OR p.run_id = $3) WHERE p.id IS NULL)`, ids, fileID, runID).Scan(&invalid); err != nil {
		return err
	}
	if invalid {
		return ErrValidation
	}
	return nil
}

func validateImportableGraph(nodes []TreeDraftNode, edges []TreeDraftEdge) error {
	byID := make(map[string]TreeDraftNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Included && (!node.Reviewed || node.Status != "reviewed") {
			return ErrConflict
		}
	}
	seen := make(map[string]bool)
	for _, edge := range edges {
		if edge.Status != "reviewed" {
			continue
		}
		subject, subjectOK := byID[edge.SubjectNodeID]
		object, objectOK := byID[edge.ObjectNodeID]
		if !subjectOK || !objectOK {
			return ErrValidation
		}
		if !subject.Included || !object.Included {
			return ErrConflict
		}
		key := edge.SubjectNodeID + ":" + edge.Predicate + ":" + edge.ObjectNodeID
		if seen[key] {
			return ErrValidation
		}
		seen[key] = true
	}
	if hasParentCycle(nodes, edges) {
		return ErrConflict
	}
	if countIncludedNodes(nodes) == 0 {
		return ErrValidation
	}
	return nil
}

func hasParentCycle(nodes []TreeDraftNode, edges []TreeDraftEdge) bool {
	adjacency := make(map[string][]string)
	for _, edge := range edges {
		if edge.Status == "reviewed" && edge.Predicate == "parent_of" {
			adjacency[edge.SubjectNodeID] = append(adjacency[edge.SubjectNodeID], edge.ObjectNodeID)
		}
	}
	state := make(map[string]uint8, len(nodes))
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, next := range adjacency[node] {
			if visit(next) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for _, node := range nodes {
		if visit(node.ID) {
			return true
		}
	}
	return false
}

func draftWarnings(nodes []TreeDraftNode, edges []TreeDraftEdge, pages []TreeDraftPage) []string {
	warnings := []string{}
	if countIncludedNodes(nodes) == 0 {
		warnings = append(warnings, "No people are selected for import.")
	}
	if hasParentCycle(nodes, edges) {
		warnings = append(warnings, "Reviewed parent relationships contain a cycle; resolve it before import.")
	}
	for _, node := range nodes {
		if node.Included && (!node.Reviewed || node.Status != "reviewed") {
			warnings = append(warnings, "Every included name needs an explicit human review.")
			break
		}
	}
	for _, edge := range edges {
		if edge.Status == "reviewed" && !supportedTreePredicate(edge.Predicate) {
			warnings = append(warnings, "Some reviewed relationship types remain staged because the tree model cannot represent them.")
			break
		}
	}
	for _, page := range pages {
		if page.LineCandidatesTruncated {
			warnings = append(warnings, "Local line detection reached its per-page candidate cap; connector coverage is incomplete.")
			break
		}
	}
	return warnings
}

func countIncludedNodes(nodes []TreeDraftNode) int {
	count := 0
	for _, node := range nodes {
		if node.Included {
			count++
		}
	}
	return count
}
func countReviewedEdges(edges []TreeDraftEdge) int {
	count := 0
	for _, edge := range edges {
		if edge.Status == "reviewed" {
			count++
		}
	}
	return count
}
func supportedTreePredicate(value string) bool {
	return value == "parent_of" || value == "spouse_of" || value == "sibling_of"
}
func validStagedPredicate(value string) bool {
	return supportedTreePredicate(value) || value == "descendant_of" || value == "unknown"
}
func validUnit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func validPageRegion(region PageRegion) bool {
	return validUnit(region.X) && validUnit(region.Y) && validUnit(region.Width) && validUnit(region.Height) && region.Width > 0 && region.Height > 0 && region.X+region.Width <= 1.001 && region.Y+region.Height <= 1.001
}

func proposeNameRegions(pageEvidenceID string, pageNumber int, selectedMethod string, regions []PageRegionEvidence) []TreeDraftNode {
	type word struct {
		text    string
		region  PageRegion
		centerY float64
	}
	words := make([]word, 0)
	for _, item := range regions {
		if item.Method != selectedMethod || item.Kind != "word" {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" || !validPageRegion(item.PageRegion) {
			continue
		}
		words = append(words, word{text: text, region: item.PageRegion, centerY: item.Y + item.Height/2})
	}
	if len(words) == 0 {
		return nil
	}
	heights := make([]float64, len(words))
	for index := range words {
		heights[index] = words[index].region.Height
	}
	sort.Float64s(heights)
	medianHeight := heights[len(heights)/2]
	rowTolerance := math.Max(0.003, medianHeight*0.65)
	sort.SliceStable(words, func(i, j int) bool {
		if math.Abs(words[i].centerY-words[j].centerY) < rowTolerance {
			return words[i].region.X > words[j].region.X
		}
		return words[i].centerY < words[j].centerY
	})
	type line struct {
		words  []word
		center float64
	}
	lines := make([]line, 0)
	for _, item := range words {
		if len(lines) == 0 || math.Abs(item.centerY-lines[len(lines)-1].center) > rowTolerance {
			lines = append(lines, line{center: item.centerY})
		}
		current := &lines[len(lines)-1]
		current.words = append(current.words, item)
		current.center = (current.center*float64(len(current.words)-1) + item.centerY) / float64(len(current.words))
	}
	nodes := make([]TreeDraftNode, 0)
	for _, row := range lines {
		sort.SliceStable(row.words, func(i, j int) bool { return row.words[i].region.X > row.words[j].region.X })
		clusters := make([][]word, 0)
		for _, item := range row.words {
			if len(clusters) == 0 {
				clusters = append(clusters, []word{item})
				continue
			}
			prior := clusters[len(clusters)-1][len(clusters[len(clusters)-1])-1]
			gap := prior.region.X - (item.region.X + item.region.Width)
			if gap > math.Max(0.018, medianHeight*3.2) {
				clusters = append(clusters, []word{item})
			} else {
				clusters[len(clusters)-1] = append(clusters[len(clusters)-1], item)
			}
		}
		for _, cluster := range clusters {
			parts := make([]string, 0, len(cluster))
			x0, y0, x1, y1 := 1.0, 1.0, 0.0, 0.0
			for _, item := range cluster {
				parts = append(parts, item.text)
				x0 = math.Min(x0, item.region.X)
				y0 = math.Min(y0, item.region.Y)
				x1 = math.Max(x1, item.region.X+item.region.Width)
				y1 = math.Max(y1, item.region.Y+item.region.Height)
			}
			label := strings.TrimSpace(strings.Join(parts, " "))
			if len([]rune(label)) > 160 || !containsArabicLetter(label) {
				continue
			}
			id := uuid.New().String()
			nodes = append(nodes, TreeDraftNode{ID: id, NameAR: label, PageEvidenceID: pageEvidenceID, Region: PageRegion{Kind: "name_proposal", Text: label, X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}, Included: false, Reviewed: false, Status: "proposed"})
		}
	}
	_ = pageNumber // Page provenance is carried by the immutable evidence id.
	return nodes
}

func containsArabicLetter(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) && (r >= 0x0600 && r <= 0x06FF || r >= 0x0750 && r <= 0x077F || r >= 0x08A0 && r <= 0x08FF) {
			return true
		}
	}
	return false
}

func filepathExt(filename string) string {
	if index := strings.LastIndex(filename, "."); index >= 0 {
		return filename[index:]
	}
	return ""
}
