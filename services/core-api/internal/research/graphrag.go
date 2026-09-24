package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	GraphOperationCommonAncestor = "common_ancestor_path"
	GraphOperationEvidence       = "evidence_connection"
	GraphOperationBranchClaims   = "branch_claims"
	GraphOperationSourceEntities = "source_entities"
	GraphOperationGeographic     = "geographic_path"
	GraphAlgorithmVersion        = "graphrag-v1"
	GraphDefaultDepth            = 2
	GraphMaxDepth                = 3
	GraphMaxPaths                = 5
	GraphMaxEdges                = 200
)

var graphOperations = map[string]struct{}{
	GraphOperationCommonAncestor: {},
	GraphOperationEvidence:       {},
	GraphOperationBranchClaims:   {},
	GraphOperationSourceEntities: {},
	GraphOperationGeographic:     {},
}

var graphEntityTypes = map[string]struct{}{
	"person": {},
	"family": {},
	"branch": {},
	"place":  {},
}

const graphQueryTimeout = 2 * time.Second

type graphRetrievalResult struct {
	Paths []GraphPath
	Stats GraphStats
}

func normalizeGraphInput(input QueryInput) (QueryInput, error) {
	input.GraphOperation = strings.ToLower(strings.TrimSpace(input.GraphOperation))
	input.GraphStartType = strings.ToLower(strings.TrimSpace(input.GraphStartType))
	input.GraphEndType = strings.ToLower(strings.TrimSpace(input.GraphEndType))
	input.GraphStartID = strings.TrimSpace(input.GraphStartID)
	input.GraphEndID = strings.TrimSpace(input.GraphEndID)
	if input.GraphOperation == "" {
		if input.GraphStartType != "" || input.GraphStartID != "" || input.GraphEndType != "" || input.GraphEndID != "" || input.GraphMaxDepth != 0 {
			return QueryInput{}, ErrValidation
		}
		return input, nil
	}
	if _, ok := graphOperations[input.GraphOperation]; !ok {
		return QueryInput{}, ErrValidation
	}
	if input.GraphMaxDepth < 0 {
		return QueryInput{}, ErrValidation
	}
	if input.GraphMaxDepth == 0 {
		input.GraphMaxDepth = GraphDefaultDepth
	}
	if input.GraphMaxDepth > GraphMaxDepth {
		input.GraphMaxDepth = GraphMaxDepth
	}
	if input.GraphStartID == "" {
		switch input.GraphOperation {
		case GraphOperationSourceEntities:
			input.GraphStartID = input.SourceID
		case GraphOperationBranchClaims, GraphOperationCommonAncestor, GraphOperationEvidence, GraphOperationGeographic:
			if input.EntityID != "" {
				input.GraphStartID = input.EntityID
			} else {
				input.GraphStartID = input.PersonID
			}
		}
	}
	if input.GraphOperation == GraphOperationGeographic && input.GraphEndID == "" && input.PlaceID != "" {
		input.GraphEndID = input.PlaceID
	}
	if input.GraphStartType == "" {
		switch input.GraphOperation {
		case GraphOperationSourceEntities:
			input.GraphStartType = "source"
		default:
			if input.EntityType != "" && input.GraphStartID == input.EntityID {
				input.GraphStartType = input.EntityType
			} else {
				input.GraphStartType = "person"
			}
		}
	}
	if input.GraphOperation == GraphOperationCommonAncestor && input.GraphEndType == "" {
		input.GraphEndType = "person"
	}
	if input.GraphOperation == GraphOperationEvidence && input.GraphEndType == "" {
		input.GraphEndType = input.GraphStartType
	}
	if input.GraphOperation == GraphOperationGeographic && input.GraphEndID != "" && input.GraphEndType == "" {
		input.GraphEndType = "place"
	}
	if input.GraphStartID == "" || input.GraphStartType == "" {
		return QueryInput{}, ErrValidation
	}
	startID, err := normalizeGraphUUID(input.GraphStartID)
	if err != nil {
		return QueryInput{}, err
	}
	input.GraphStartID = startID
	if input.GraphEndID != "" {
		endID, endErr := normalizeGraphUUID(input.GraphEndID)
		if endErr != nil {
			return QueryInput{}, endErr
		}
		input.GraphEndID = endID
	}
	if input.GraphEndType != "" && input.GraphEndID == "" {
		return QueryInput{}, ErrValidation
	}
	if err := validateGraphEndpointCombination(input); err != nil {
		return QueryInput{}, err
	}
	return input, nil
}

func normalizeGraphUUID(value string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", ErrValidation
	}
	return parsed.String(), nil
}

func validateGraphEndpointCombination(input QueryInput) error {
	switch input.GraphOperation {
	case GraphOperationCommonAncestor:
		if input.GraphStartType != "person" || input.GraphEndType != "person" || input.GraphEndID == "" {
			return ErrValidation
		}
	case GraphOperationEvidence:
		if !isGraphEntityType(input.GraphStartType) || input.GraphEndType != input.GraphStartType || input.GraphEndID == "" {
			return ErrValidation
		}
	case GraphOperationBranchClaims:
		if !isGraphSelectionType(input.GraphStartType) || input.GraphEndID != "" {
			return ErrValidation
		}
	case GraphOperationSourceEntities:
		if input.GraphStartType != "source" || input.GraphEndID != "" {
			return ErrValidation
		}
	case GraphOperationGeographic:
		if !isGraphSelectionType(input.GraphStartType) && input.GraphStartType != "place" {
			return ErrValidation
		}
		if input.GraphEndID != "" && input.GraphEndType != "place" {
			return ErrValidation
		}
	}
	return nil
}

func isGraphEntityType(value string) bool {
	_, ok := graphEntityTypes[value]
	return ok
}

func isGraphSelectionType(value string) bool {
	return value == "person" || value == "family" || value == "branch"
}

func (s *Service) retrieveGraph(ctx context.Context, input QueryInput, actorID string) (graphRetrievalResult, error) {
	if input.GraphOperation == "" {
		return graphRetrievalResult{Paths: []GraphPath{}}, nil
	}
	normalizedInput, err := normalizeGraphInput(input)
	if err != nil {
		return graphRetrievalResult{}, err
	}
	input = normalizedInput
	if s == nil || s.Pool == nil {
		return graphRetrievalResult{}, ErrDatabaseUnavailable
	}
	actorUUID := uuid.Nil
	if strings.TrimSpace(actorID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(actorID))
		if err != nil {
			return graphRetrievalResult{}, ErrForbidden
		}
		actorUUID = parsed
	}
	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()
	tx, err := s.Pool.BeginTx(queryCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return graphRetrievalResult{}, graphRetrievalError(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(queryCtx, `SET LOCAL statement_timeout = '2000ms'`); err != nil {
		return graphRetrievalResult{}, graphRetrievalError(err)
	}
	var rows pgx.Rows
	switch input.GraphOperation {
	case GraphOperationCommonAncestor:
		rows, err = tx.Query(queryCtx, commonAncestorGraphQuery, input.GraphStartID, input.GraphEndID, nullableUUID(actorUUID), nullableUUID(optionalUUID(input.TreeID)), nullableUUID(optionalUUID(input.TreeVersionID)), input.GraphMaxDepth, GraphMaxPaths)
	case GraphOperationEvidence:
		rows, err = tx.Query(queryCtx, evidenceConnectionGraphQuery, input.GraphStartID, input.GraphEndID, input.GraphStartType, nullableUUID(actorUUID), input.GraphMaxDepth, GraphMaxPaths, GraphMaxEdges)
	case GraphOperationBranchClaims:
		rows, err = tx.Query(queryCtx, branchClaimsGraphQuery, input.GraphStartID, input.GraphStartType, nullableUUID(actorUUID), GraphMaxEdges)
	case GraphOperationSourceEntities:
		rows, err = tx.Query(queryCtx, sourceEntitiesGraphQuery, input.GraphStartID, nullableUUID(actorUUID), GraphMaxPaths, GraphMaxEdges, input.GraphMaxDepth)
	case GraphOperationGeographic:
		rows, err = tx.Query(queryCtx, geographicGraphQuery, input.GraphStartID, input.GraphStartType, nullableUUID(optionalUUID(input.GraphEndID)), nullableUUID(actorUUID), input.FromYear, input.ToYear, GraphMaxPaths, GraphMaxEdges, input.GraphMaxDepth)
	default:
		return graphRetrievalResult{}, ErrValidation
	}
	if err != nil {
		return graphRetrievalResult{}, graphRetrievalError(err)
	}
	defer rows.Close()
	paths := make([]GraphPath, 0)
	for rows.Next() {
		path, scanErr := scanGraphQueryRow(rows, input.GraphOperation)
		if scanErr != nil {
			return graphRetrievalResult{}, graphRetrievalError(scanErr)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return graphRetrievalResult{}, graphRetrievalError(err)
	}
	paths = normalizeGraphPaths(paths)
	stats := graphStatsForPaths(input.GraphOperation, input.GraphMaxDepth, paths)
	return graphRetrievalResult{Paths: paths, Stats: stats}, nil
}

func optionalUUID(value string) uuid.UUID {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

func graphRetrievalError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return fmt.Errorf("%w: %w at position %d", ErrGraphUnavailable, err, postgresError.Position)
	}
	return fmt.Errorf("%w: %w", ErrGraphUnavailable, err)
}

func scanGraphQueryRow(rows pgx.Rows, operation string) (GraphPath, error) {
	var rowOperation, status string
	var versionNumber int32
	var treeID, treeVersionID, versionState pgtype.Text
	var depth int32
	var truncated, evidenceBacked, structuralOnly bool
	var nodes, edges, evidence []byte
	if err := rows.Scan(&rowOperation, &status, &versionNumber, &treeID, &treeVersionID, &versionState, &depth, &truncated, &evidenceBacked, &structuralOnly, &nodes, &edges, &evidence); err != nil {
		return GraphPath{}, err
	}
	path := GraphPath{
		Operation:        rowOperation,
		Status:           status,
		Explanation:      graphExplanation(rowOperation, status, structuralOnly),
		Depth:            int(depth),
		Truncated:        truncated,
		EvidenceBacked:   evidenceBacked,
		StructuralOnly:   structuralOnly,
		TreeScope:        GraphTreeScope{TreeID: textValue(treeID), TreeVersionID: textValue(treeVersionID), VersionNumber: int(versionNumber), VersionState: textValue(versionState)},
		AlgorithmVersion: GraphAlgorithmVersion,
	}
	if err := json.Unmarshal(nodes, &path.Nodes); err != nil {
		return GraphPath{}, err
	}
	if err := json.Unmarshal(edges, &path.Edges); err != nil {
		return GraphPath{}, err
	}
	if err := json.Unmarshal(evidence, &path.EvidenceRefs); err != nil {
		return GraphPath{}, err
	}
	path.ID = graphPathID(path, nodes, edges, evidence)
	return path, nil
}

func graphPathID(path GraphPath, nodes, edges, evidence []byte) string {
	key := strings.Join([]string{path.Operation, path.Status, fmt.Sprintf("%d", path.Depth), path.TreeScope.TreeID, path.TreeScope.TreeVersionID, string(nodes), string(edges), string(evidence)}, "|")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

func graphExplanation(operation, status string, structuralOnly bool) string {
	if structuralOnly {
		return "مسار بنيوي من تفسير منشور؛ يوضح بنية العلاقة ولا يثبت حقيقة تاريخية نهائية."
	}
	if operation == GraphOperationGeographic {
		return "مسار جغرافي مبني على أحداث مسجلة أو مستنتجة؛ يعرض الحالة والنسبة ولا يثبت مساراً تاريخياً نهائياً."
	}
	if status == "contested" {
		return "مسار أدلة يتضمن ادعاءات متنافسة؛ لا يحسم الحقيقة التاريخية."
	}
	return "مسار قابل للتتبع إلى أدلة وادعاءات مقبولة؛ ضمن الأدلة ولا يمثل حقيقة نهائية."
}

func normalizeGraphPaths(paths []GraphPath) []GraphPath {
	pathOverflow := len(paths) > GraphMaxPaths
	if pathOverflow {
		for index := GraphMaxPaths; index < len(paths); index++ {
			paths[index].Truncated = true
		}
		paths = paths[:GraphMaxPaths]
	}
	if pathOverflow && len(paths) > 0 {
		paths[0].Truncated = true
	}
	totalEdges := 0
	for index := range paths {
		if paths[index].Nodes == nil {
			paths[index].Nodes = []GraphNode{}
		}
		if paths[index].Edges == nil {
			paths[index].Edges = []GraphEdge{}
		}
		if paths[index].EvidenceRefs == nil {
			paths[index].EvidenceRefs = []GraphEvidenceRef{}
		}
		for nodeIndex := range paths[index].Nodes {
			paths[index].Nodes[nodeIndex].Label = boundedText(paths[index].Nodes[nodeIndex].Label, 4000)
		}
		for evidenceIndex := range paths[index].EvidenceRefs {
			paths[index].EvidenceRefs[evidenceIndex].Title = boundedText(paths[index].EvidenceRefs[evidenceIndex].Title, 1000)
			paths[index].EvidenceRefs[evidenceIndex].Excerpt = boundedText(paths[index].EvidenceRefs[evidenceIndex].Excerpt, 4000)
			paths[index].EvidenceRefs[evidenceIndex].LocatorAR = boundedText(paths[index].EvidenceRefs[evidenceIndex].LocatorAR, 1000)
		}
		sort.SliceStable(paths[index].EvidenceRefs, func(left, right int) bool {
			return graphEvidenceRefKey(paths[index].EvidenceRefs[left]) < graphEvidenceRefKey(paths[index].EvidenceRefs[right])
		})
		dedupedEvidence := make([]GraphEvidenceRef, 0, len(paths[index].EvidenceRefs))
		seenEvidence := make(map[string]struct{}, len(paths[index].EvidenceRefs))
		for _, evidence := range paths[index].EvidenceRefs {
			key := graphEvidenceRefKey(evidence)
			if _, exists := seenEvidence[key]; exists {
				continue
			}
			seenEvidence[key] = struct{}{}
			dedupedEvidence = append(dedupedEvidence, evidence)
		}
		paths[index].EvidenceRefs = dedupedEvidence
		if paths[index].Depth < 0 {
			paths[index].Depth = 0
		}
		if paths[index].Depth > GraphMaxDepth {
			paths[index].Depth = GraphMaxDepth
			paths[index].Truncated = true
		}
		if len(paths[index].Edges) > GraphMaxEdges {
			paths[index].Edges = paths[index].Edges[:GraphMaxEdges]
			paths[index].Truncated = true
		}
		if totalEdges+len(paths[index].Edges) > GraphMaxEdges {
			remaining := GraphMaxEdges - totalEdges
			if remaining < 0 {
				remaining = 0
			}
			if remaining < len(paths[index].Edges) {
				paths[index].Edges = paths[index].Edges[:remaining]
				paths[index].Truncated = true
			}
		}
		totalEdges += len(paths[index].Edges)
		if len(paths[index].EvidenceRefs) > GraphMaxEdges {
			paths[index].EvidenceRefs = paths[index].EvidenceRefs[:GraphMaxEdges]
			paths[index].Truncated = true
		}
		paths[index].EvidenceBacked = len(paths[index].EvidenceRefs) > 0
		paths[index].StructuralOnly = !paths[index].EvidenceBacked
		if paths[index].Status == "complete" {
			for _, evidence := range paths[index].EvidenceRefs {
				if evidence.Relation == "contradicts" || evidence.Relation == "refutes" || evidence.Relation == "counter_evidence" {
					paths[index].Status = "contested"
					break
				}
			}
		}
		paths[index].Explanation = graphExplanation(paths[index].Operation, paths[index].Status, paths[index].StructuralOnly)
	}
	return paths
}

func graphEvidenceRefKey(value GraphEvidenceRef) string {
	return strings.Join([]string{value.Type, value.ID, value.Relation, value.SourceID, value.StatementID, value.PassageID, value.ClaimID}, "|")
}

func graphTruncatedValue(stats GraphStats, hasPaths bool) any {
	if stats.Operation == "" && !hasPaths {
		return nil
	}
	return stats.Truncated
}

func persistGraphRun(ctx context.Context, tx pgx.Tx, runID string, result QueryResult) error {
	maxDepth := result.GraphStats.MaxDepth
	if maxDepth == 0 {
		maxDepth = GraphMaxDepth
	}
	if err := validateGraphPaths(result.GraphPaths, maxDepth); err != nil {
		return err
	}
	totalEdges := 0
	for _, path := range result.GraphPaths {
		totalEdges += len(path.Edges)
	}
	if totalEdges > GraphMaxEdges {
		return ErrValidation
	}
	for pathIndex := range result.GraphPaths {
		path := &result.GraphPaths[pathIndex]
		pathID := optionalUUID(path.ID)
		if pathID == uuid.Nil {
			pathID = uuid.New()
			path.ID = pathID.String()
		}
		pathKey := strings.TrimSpace(path.ID)
		if pathKey == "" {
			pathKey = uuid.NewString()
			path.ID = pathKey
		}
		if path.AlgorithmVersion == "" {
			path.AlgorithmVersion = GraphAlgorithmVersion
		}
		if path.Explanation == "" {
			path.Explanation = graphExplanation(path.Operation, path.Status, path.StructuralOnly)
		}
		treeID := optionalUUID(path.TreeScope.TreeID)
		treeVersionID := optionalUUID(path.TreeScope.TreeVersionID)
		pathMetadata := mustJSON(map[string]any{
			"nodeCount":      len(path.Nodes),
			"edgeCount":      len(path.Edges),
			"evidenceCount":  len(path.EvidenceRefs),
			"evidenceBacked": path.EvidenceBacked,
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO research_graph_paths (id, run_id, path_key, operation, status, explanation, depth, truncated, evidence_backed, structural_only, tree_id, tree_version_id, tree_version_number, tree_version_state, algorithm_version, nodes, node_provenance, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NULLIF($14, ''), $15, $16, $17, $18)
		`, pathID, runID, pathKey, path.Operation, path.Status, boundedText(path.Explanation, 2000), path.Depth, path.Truncated, path.EvidenceBacked, path.StructuralOnly, nullableUUID(treeID), nullableUUID(treeVersionID), nullableInt(path.TreeScope.VersionNumber), nullableString(path.TreeScope.VersionState), path.AlgorithmVersion, mustJSON(path.Nodes), mustJSON(path.Nodes), pathMetadata); err != nil {
			return err
		}
		for edgeIndex, edge := range path.Edges {
			if edgeIndex >= GraphMaxEdges {
				break
			}
			fromNodeID := optionalUUID(edge.FromNodeID)
			toNodeID := optionalUUID(edge.ToNodeID)
			if fromNodeID == uuid.Nil || toNodeID == uuid.Nil {
				return ErrValidation
			}
			edgeID := uuid.New()
			if _, err := tx.Exec(ctx, `
				INSERT INTO research_graph_edges (id, run_id, path_id, ordinal, edge_reference_id, edge_type, from_node_id, to_node_id, predicate, status, certainty, source_id, claim_id, statement_id, passage_id, tree_relationship_id, migration_event_id, from_place_id, to_place_id, metadata)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12, $13, $14, $15, $16, $17, $18, $19, $20)
			`, edgeID, runID, pathID, edgeIndex+1, nullableUUID(optionalUUID(edge.ID)), edge.Type, fromNodeID, toNodeID, nullableString(edge.Predicate), nullableString(edge.Status), nullableString(edge.Certainty), nullableUUID(optionalUUID(edge.SourceID)), nullableUUID(optionalUUID(edge.ClaimID)), nullableUUID(optionalUUID(edge.StatementID)), nullableUUID(optionalUUID(edge.PassageID)), nullableUUID(optionalUUID(edge.TreeRelationshipID)), nullableUUID(optionalUUID(edge.MigrationEventID)), nullableUUID(optionalUUID(edge.FromPlaceID)), nullableUUID(optionalUUID(edge.ToPlaceID)), mustJSON(map[string]any{"position": edge.Position})); err != nil {
				return err
			}
		}
		for evidenceIndex, evidence := range path.EvidenceRefs {
			if evidenceIndex >= GraphMaxEdges {
				break
			}
			referenceID := optionalUUID(evidence.ID)
			if referenceID == uuid.Nil {
				return ErrValidation
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO research_graph_path_evidence (run_id, path_id, ordinal, reference_id, reference_type, relation, source_id, claim_id, statement_id, passage_id, review_status, status, certainty, title_ar, excerpt_ar, locator_ar, page_number, metadata)
				VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''), NULLIF($16, ''), $17, $18)
			`, runID, pathID, evidenceIndex+1, referenceID, evidence.Type, nullableString(evidence.Relation), nullableUUID(optionalUUID(evidence.SourceID)), nullableUUID(optionalUUID(evidence.ClaimID)), nullableUUID(optionalUUID(evidence.StatementID)), nullableUUID(optionalUUID(evidence.PassageID)), nullableString(evidence.ReviewStatus), nullableString(evidence.Status), nullableString(evidence.Certainty), nullableString(boundedText(evidence.Title, 1000)), nullableString(boundedText(evidence.Excerpt, 4000)), nullableString(boundedText(evidence.LocatorAR, 1000)), pageNumberValue(evidence.PageNumber), mustJSON(map[string]any{"layer": evidence.Layer})); err != nil {
				return err
			}
		}
	}
	return nil
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func pageNumberValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func boundedText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func graphEvidenceCount(paths []GraphPath) int {
	count := 0
	for _, path := range paths {
		count += len(path.EvidenceRefs)
	}
	return count
}

func graphStatsForPaths(operation string, maxDepth int, paths []GraphPath) GraphStats {
	stats := GraphStats{Operation: operation, MaxDepth: maxDepth, AlgorithmVersion: GraphAlgorithmVersion}
	for _, path := range paths {
		stats.PathCount++
		stats.NodeCount += len(path.Nodes)
		stats.EdgeCount += len(path.Edges)
		stats.EvidenceCount += len(path.EvidenceRefs)
		if path.Truncated {
			stats.Truncated = true
			stats.PathsTruncated = true
			if len(path.Edges) >= GraphMaxEdges {
				stats.EdgesTruncated = true
			}
		}
	}
	if stats.EdgeCount > GraphMaxEdges {
		stats.EdgeCount = GraphMaxEdges
		stats.EdgesTruncated = true
		stats.Truncated = true
	}
	return stats
}

func validateGraphPaths(paths []GraphPath, maxDepth int) error {
	if maxDepth < 0 || maxDepth > GraphMaxDepth {
		return ErrValidation
	}
	if len(paths) > GraphMaxPaths {
		return ErrValidation
	}
	for _, path := range paths {
		if _, ok := graphOperations[path.Operation]; !ok || path.Status == "" || path.AlgorithmVersion == "" {
			return ErrValidation
		}
		if path.Depth < 0 || path.Depth > GraphMaxDepth || path.Depth > maxDepth {
			return ErrValidation
		}
		if len(path.Nodes) == 0 || len(path.Edges) > GraphMaxEdges || len(path.EvidenceRefs) > GraphMaxEdges {
			return ErrValidation
		}
		nodeIDs := make(map[string]struct{}, len(path.Nodes))
		for index, node := range path.Nodes {
			if node.ID == "" || node.Type == "" || node.Position != index {
				return ErrValidation
			}
			nodeIDs[node.ID] = struct{}{}
		}
		for index, edge := range path.Edges {
			if edge.ID == "" || edge.Type == "" || edge.FromNodeID == "" || edge.ToNodeID == "" || edge.Position != index {
				return ErrValidation
			}
			if _, exists := nodeIDs[edge.FromNodeID]; !exists {
				return ErrValidation
			}
			if _, exists := nodeIDs[edge.ToNodeID]; !exists {
				return ErrValidation
			}
		}
		if path.EvidenceBacked && path.StructuralOnly {
			return ErrValidation
		}
		if !path.EvidenceBacked && len(path.EvidenceRefs) > 0 {
			return ErrValidation
		}
	}
	return nil
}

const commonAncestorGraphQuery = `
WITH RECURSIVE
parameters AS (
  SELECT $1::uuid AS start_person_id, $2::uuid AS end_person_id, $3::uuid AS actor_id, $4::uuid AS tree_id, $5::uuid AS tree_version_id, $6::integer AS max_depth, $7::integer AS max_paths
),
visible_versions AS (
  SELECT tv.id AS tree_version_id, tv.tree_id, tv.version_number, tv.state, t.name_ar
  FROM tree_versions tv
  JOIN trees t ON t.id = tv.tree_id
  CROSS JOIN parameters p
  WHERE tv.state = 'published'
    AND (p.tree_id IS NULL OR t.id = p.tree_id)
    AND (p.tree_version_id IS NULL OR tv.id = p.tree_version_id)
    AND (t.visibility = 'public' OR (p.actor_id IS NOT NULL AND (t.owner_id = p.actor_id OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = p.actor_id) OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin')))))
),
start_nodes AS (
  SELECT v.tree_version_id, v.tree_id, v.version_number, v.state, v.name_ar, tn.id AS node_id, tn.person_id, tn.display_name_ar
  FROM visible_versions v
  JOIN tree_nodes tn ON tn.tree_version_id = v.tree_version_id
  CROSS JOIN parameters p
  WHERE tn.person_id = p.start_person_id
),
end_nodes AS (
  SELECT v.tree_version_id, v.tree_id, v.version_number, v.state, v.name_ar, tn.id AS node_id, tn.person_id, tn.display_name_ar
  FROM visible_versions v
  JOIN tree_nodes tn ON tn.tree_version_id = v.tree_version_id
  CROSS JOIN parameters p
  WHERE tn.person_id = p.end_person_id
),
start_walk (
  tree_version_id, tree_id, version_number, version_state, tree_name, node_id, person_id, display_name_ar, depth, node_ids, visited_ids, edge_ids
) AS (
  SELECT tree_version_id, tree_id, version_number, state, name_ar, node_id, person_id, display_name_ar, 0, ARRAY[node_id]::uuid[], ARRAY[node_id]::uuid[], ARRAY[]::uuid[]
  FROM start_nodes
  UNION ALL
  SELECT next_walk.tree_version_id, next_walk.tree_id, next_walk.version_number, next_walk.version_state, next_walk.tree_name, next_walk.next_node_id, next_walk.person_id, next_walk.display_name_ar, next_walk.next_depth, next_walk.next_node_ids, next_walk.next_visited_ids, next_walk.next_edge_ids
  FROM (
    SELECT sw.tree_version_id, sw.tree_id, sw.version_number, sw.version_state, sw.tree_name,
      tr.subject_node_id AS next_node_id, tn.person_id, tn.display_name_ar,
      sw.depth + 1 AS next_depth, sw.node_ids || tr.subject_node_id AS next_node_ids,
      sw.visited_ids || tr.subject_node_id AS next_visited_ids, sw.edge_ids || tr.id AS next_edge_ids
    FROM start_walk sw
     JOIN tree_relationships tr ON tr.tree_version_id = sw.tree_version_id AND tr.object_node_id = sw.node_id AND tr.predicate = 'parent_of'
     JOIN tree_nodes tn ON tn.id = tr.subject_node_id AND tn.tree_version_id = tr.tree_version_id
     LEFT JOIN sources edge_source ON edge_source.id = tr.source_id
     CROSS JOIN parameters p
     WHERE sw.depth < p.max_depth AND NOT tr.subject_node_id = ANY(sw.visited_ids)
       AND (edge_source.id IS NULL OR edge_source.visibility = 'public' OR (p.actor_id IS NOT NULL AND (edge_source.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin')) OR EXISTS (SELECT 1 FROM trees tree_owner WHERE tree_owner.id = sw.tree_id AND tree_owner.owner_id = p.actor_id) OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = sw.tree_id AND tc.user_id = p.actor_id))))

    ORDER BY sw.tree_version_id, sw.node_ids, tr.id
    LIMIT 200
  ) next_walk
),
end_walk (
  tree_version_id, tree_id, version_number, version_state, tree_name, node_id, person_id, display_name_ar, depth, node_ids, visited_ids, edge_ids
) AS (
  SELECT tree_version_id, tree_id, version_number, state, name_ar, node_id, person_id, display_name_ar, 0, ARRAY[node_id]::uuid[], ARRAY[node_id]::uuid[], ARRAY[]::uuid[]
  FROM end_nodes
  UNION ALL
  SELECT next_walk.tree_version_id, next_walk.tree_id, next_walk.version_number, next_walk.version_state, next_walk.tree_name, next_walk.next_node_id, next_walk.person_id, next_walk.display_name_ar, next_walk.next_depth, next_walk.next_node_ids, next_walk.next_visited_ids, next_walk.next_edge_ids
  FROM (
    SELECT ew.tree_version_id, ew.tree_id, ew.version_number, ew.version_state, ew.tree_name,
      tr.subject_node_id AS next_node_id, tn.person_id, tn.display_name_ar,
      ew.depth + 1 AS next_depth, ew.node_ids || tr.subject_node_id AS next_node_ids,
      ew.visited_ids || tr.subject_node_id AS next_visited_ids, ew.edge_ids || tr.id AS next_edge_ids
    FROM end_walk ew
     JOIN tree_relationships tr ON tr.tree_version_id = ew.tree_version_id AND tr.object_node_id = ew.node_id AND tr.predicate = 'parent_of'
     JOIN tree_nodes tn ON tn.id = tr.subject_node_id AND tn.tree_version_id = tr.tree_version_id
     LEFT JOIN sources edge_source ON edge_source.id = tr.source_id
     CROSS JOIN parameters p
     WHERE ew.depth < p.max_depth AND NOT tr.subject_node_id = ANY(ew.visited_ids)
       AND (edge_source.id IS NULL OR edge_source.visibility = 'public' OR (p.actor_id IS NOT NULL AND (edge_source.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin')) OR EXISTS (SELECT 1 FROM trees tree_owner WHERE tree_owner.id = ew.tree_id AND tree_owner.owner_id = p.actor_id) OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = ew.tree_id AND tc.user_id = p.actor_id))))

    ORDER BY ew.tree_version_id, ew.node_ids, tr.id
    LIMIT 200
  ) next_walk
),
reversed_end AS (
  SELECT ew.*, (SELECT array_agg(ordered.node_id ORDER BY ordered.ordinality DESC) FROM unnest(ew.node_ids) WITH ORDINALITY AS ordered(node_id, ordinality)) AS reversed_node_ids
  FROM end_walk ew
),
common_paths AS (
  SELECT DISTINCT ON (sw.tree_version_id, sw.node_id)
    sw.tree_version_id, sw.tree_id, sw.version_number, sw.version_state, sw.tree_name,
    sw.node_id AS common_node_id, sw.node_ids AS start_node_ids, ew.reversed_node_ids AS end_node_ids,
    sw.node_ids || COALESCE(ew.reversed_node_ids[2:cardinality(ew.reversed_node_ids)], ARRAY[]::uuid[]) AS node_ids,
    sw.depth + ew.depth AS path_depth,
    sw.node_ids[1] AS start_node_id,
    ew.node_ids[1] AS end_node_id
  FROM start_walk sw
  JOIN reversed_end ew ON ew.tree_version_id = sw.tree_version_id AND ew.node_id = sw.node_id
  CROSS JOIN parameters p
  WHERE sw.depth + ew.depth <= p.max_depth
  ORDER BY sw.tree_version_id, sw.node_id, sw.depth + ew.depth, sw.node_ids[1], ew.node_ids[1]
),
ranked_paths AS (
  SELECT cp.*, row_number() OVER (ORDER BY path_depth, version_number, common_node_id, start_node_id, end_node_id) AS path_order,
    count(*) OVER () AS candidate_count
  FROM common_paths cp
),
limited_paths AS (
  SELECT * FROM ranked_paths WHERE path_order <= $7
),
path_edges AS (
  SELECT lp.path_order, n.node_order, tr.id AS edge_id, n.node_id AS from_node_id,
    (SELECT n2.node_id FROM unnest(lp.node_ids) WITH ORDINALITY AS n2(node_id, ordinality) WHERE n2.ordinality = n.node_order + 1) AS to_node_id,
     tr.predicate, tr.status, tr.source_id,
     CASE WHEN tr.source_id IS NULL OR tr.visibility = 'public' OR ($3::uuid IS NOT NULL AND (tr.created_by = $3 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $3 AND ur.role IN ('researcher', 'moderator', 'admin')) OR EXISTS (SELECT 1 FROM trees tree_owner WHERE tree_owner.id = lp.tree_id AND tree_owner.owner_id = $3) OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = lp.tree_id AND tc.user_id = $3))) THEN tr.source_id::text ELSE NULL END AS visible_source_id,
     CASE WHEN tr.source_id IS NULL OR tr.visibility = 'public' OR ($3::uuid IS NOT NULL AND (tr.created_by = $3 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $3 AND ur.role IN ('researcher', 'moderator', 'admin')) OR EXISTS (SELECT 1 FROM trees tree_owner WHERE tree_owner.id = lp.tree_id AND tree_owner.owner_id = $3) OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = lp.tree_id AND tc.user_id = $3))) THEN tr.source_title ELSE NULL END AS visible_source_title

  FROM limited_paths lp
  CROSS JOIN LATERAL unnest(lp.node_ids) WITH ORDINALITY AS n(node_id, node_order)
  LEFT JOIN LATERAL (
     SELECT tr.subject_node_id, tr.object_node_id, tr.id, tr.predicate, tr.status, tr.source_id, s.visibility, s.created_by, s.title_ar AS source_title

    FROM tree_relationships tr
    LEFT JOIN sources s ON s.id = tr.source_id
     WHERE tr.tree_version_id = lp.tree_version_id
       AND tr.predicate = 'parent_of'
       AND ((tr.subject_node_id = n.node_id AND tr.object_node_id = (SELECT n2.node_id FROM unnest(lp.node_ids) WITH ORDINALITY AS n2(node_id, ordinality) WHERE n2.ordinality = n.node_order + 1)) OR (tr.subject_node_id = (SELECT n2.node_id FROM unnest(lp.node_ids) WITH ORDINALITY AS n2(node_id, ordinality) WHERE n2.ordinality = n.node_order + 1) AND tr.object_node_id = n.node_id))
       AND (s.id IS NULL OR s.visibility = 'public' OR ($3::uuid IS NOT NULL AND (s.created_by = $3 OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $3 AND ur.role IN ('researcher', 'moderator', 'admin')) OR EXISTS (SELECT 1 FROM trees tree_owner WHERE tree_owner.id = lp.tree_id AND tree_owner.owner_id = $3) OR EXISTS (SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = lp.tree_id AND tc.user_id = $3))))

    ORDER BY tr.id
    LIMIT 1
  ) tr ON TRUE
  WHERE n.node_order < cardinality(lp.node_ids)
),
path_nodes AS (
  SELECT lp.path_order, jsonb_agg(jsonb_build_object('id', tn.id::text, 'type', 'person', 'label', tn.display_name_ar, 'personId', tn.person_id::text, 'treeNodeId', tn.id::text, 'position', n.node_order - 1) ORDER BY n.node_order) AS nodes
  FROM limited_paths lp
  CROSS JOIN LATERAL unnest(lp.node_ids) WITH ORDINALITY AS n(node_id, node_order)
  JOIN tree_nodes tn ON tn.id = n.node_id AND tn.tree_version_id = lp.tree_version_id
  GROUP BY lp.path_order
),
path_edges_json AS (
  SELECT path_order, jsonb_agg(jsonb_build_object('id', edge_id::text, 'type', 'parent_of', 'fromNodeId', from_node_id::text, 'toNodeId', to_node_id::text, 'predicate', predicate, 'status', status, 'sourceId', visible_source_id, 'treeRelationshipId', edge_id::text, 'position', node_order - 1) ORDER BY node_order) AS edges
  FROM path_edges
  WHERE edge_id IS NOT NULL
  GROUP BY path_order
),
path_evidence AS (
  SELECT path_order, jsonb_agg(jsonb_build_object('id', edge_id::text, 'type', 'tree_relationship', 'layer', 'tree_interpretation', 'relation', 'supports', 'sourceId', visible_source_id, 'title', visible_source_title, 'status', status) ORDER BY node_order) AS refs
  FROM path_edges
  WHERE edge_id IS NOT NULL AND visible_source_id IS NOT NULL
  GROUP BY path_order
)
SELECT 'common_ancestor_path', CASE WHEN jsonb_array_length(COALESCE(pev.refs, '[]'::jsonb)) > 0 THEN 'evidence_backed' ELSE 'structural' END, lp.version_number::integer,
  lp.tree_id::text, lp.tree_version_id::text, lp.version_state,
  lp.path_depth::integer,
  (lp.candidate_count > $7 OR lp.path_depth >= $6),
  jsonb_array_length(COALESCE(pev.refs, '[]'::jsonb)) > 0, jsonb_array_length(COALESCE(pev.refs, '[]'::jsonb)) = 0,
  pn.nodes, COALESCE(pe.edges, '[]'::jsonb), COALESCE(pev.refs, '[]'::jsonb)
FROM limited_paths lp
LEFT JOIN path_nodes pn ON pn.path_order = lp.path_order
LEFT JOIN path_edges_json pe ON pe.path_order = lp.path_order
LEFT JOIN path_evidence pev ON pev.path_order = lp.path_order
ORDER BY lp.path_order
`

const evidenceConnectionGraphQuery = `
WITH RECURSIVE
parameters AS (
  SELECT $1::uuid AS start_id, $2::uuid AS end_id, $3::text AS endpoint_type, $4::uuid AS actor_id, $5::integer AS max_depth, $6::integer AS max_paths, $7::integer AS max_edges
),
visible_support AS (
  SELECT ce.claim_id, ss.id AS reference_id, ce.relation, ss.id AS statement_id, sp.id AS passage_id,
    COALESCE(ss.source_id, sp.source_id) AS source_id, s.title_ar,
    COALESCE(ss.statement_text_ar, sp.text_ar, '') AS excerpt,
    COALESCE(ss.locator_ar, sp.locator_ar) AS locator_ar, sp.page_number, ss.review_status
  FROM claim_evidence ce
  JOIN source_statements ss ON ss.id = ce.source_statement_id AND ss.review_status = 'accepted'
  LEFT JOIN source_passages sp ON sp.id = COALESCE(ce.source_passage_id, ss.source_passage_id)
  JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
  CROSS JOIN parameters p
  WHERE s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin'))))
),
visible_counter AS (
  SELECT cce.claim_id, ss.id AS reference_id, 'counter_evidence'::text AS relation, ss.id AS statement_id, sp.id AS passage_id,
    COALESCE(ss.source_id, sp.source_id) AS source_id, s.title_ar,
    COALESCE(ss.statement_text_ar, sp.text_ar, '') AS excerpt,
    COALESCE(ss.locator_ar, sp.locator_ar) AS locator_ar, sp.page_number, ss.review_status
  FROM claim_counter_evidence cce
  JOIN source_statements ss ON ss.id = cce.source_statement_id AND ss.review_status = 'accepted'
  LEFT JOIN source_passages sp ON sp.id = COALESCE(cce.source_passage_id, ss.source_passage_id)
  JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
  CROSS JOIN parameters p
  WHERE s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin'))))
),
visible_evidence AS (
  SELECT * FROM visible_support
  UNION ALL
  SELECT * FROM visible_counter
),
eligible_claims AS (
  SELECT c.*
  FROM claims c
  CROSS JOIN parameters p
  WHERE c.subject_type = p.endpoint_type AND c.object_type = p.endpoint_type
    AND c.subject_id <> c.object_id
    AND c.status NOT IN ('rejected', 'superseded', 'unknown', 'unresolved')
    AND EXISTS (SELECT 1 FROM visible_support vs WHERE vs.claim_id = c.id)
),
ranked_visible_evidence AS (
  SELECT ve.*, row_number() OVER (PARTITION BY ve.claim_id ORDER BY ve.relation, ve.source_id::text, ve.reference_id::text) AS evidence_rank
  FROM visible_evidence ve
),
claim_refs AS (
  SELECT ve.claim_id, jsonb_agg(jsonb_build_object('id', ve.reference_id::text, 'type', CASE WHEN ve.statement_id IS NOT NULL THEN 'source_statement' ELSE 'source_passage' END, 'layer', CASE WHEN ve.statement_id IS NOT NULL THEN 'source_statement' ELSE 'source_passage' END, 'relation', ve.relation, 'sourceId', ve.source_id::text, 'claimId', ve.claim_id::text, 'statementId', ve.statement_id::text, 'passageId', ve.passage_id::text, 'reviewStatus', ve.review_status, 'title', ve.title_ar, 'excerpt', ve.excerpt, 'locatorAr', ve.locator_ar, 'pageNumber', ve.page_number) ORDER BY ve.relation, ve.source_id::text, ve.reference_id::text) AS refs
  FROM ranked_visible_evidence ve
  WHERE ve.evidence_rank <= 50
  GROUP BY ve.claim_id
),
entity_labels AS (
  SELECT 'person'::text AS type, id, canonical_name_ar AS label FROM people WHERE merged_into_id IS NULL
  UNION ALL SELECT 'family', id, canonical_name_ar FROM families WHERE merged_into_id IS NULL
  UNION ALL SELECT 'branch', id, canonical_name_ar FROM branches
  UNION ALL SELECT 'place', id, canonical_name_ar FROM places
),
oriented_start AS (
  SELECT c.id, c.predicate, c.status, c.notes_ar,
    CASE WHEN c.subject_id = p.start_id THEN c.object_id ELSE c.subject_id END AS to_id
  FROM eligible_claims c
  CROSS JOIN parameters p
  WHERE c.subject_id = p.start_id OR c.object_id = p.start_id
),
walk (
  claim_id, to_id, predicate, status, notes_ar, depth, node_ids, visited_ids, claim_ids
) AS (
  SELECT seed.id, seed.to_id, seed.predicate, seed.status, seed.notes_ar, 1,
    ARRAY[p.start_id, seed.to_id]::uuid[], ARRAY[p.start_id, seed.to_id]::uuid[], ARRAY[seed.id]::uuid[]
  FROM (
    SELECT oc.id, oc.to_id, oc.predicate, oc.status, oc.notes_ar
    FROM oriented_start oc
    CROSS JOIN parameters p
    WHERE oc.to_id <> p.start_id
    ORDER BY oc.id
    LIMIT $7
  ) seed
  CROSS JOIN parameters p
  UNION ALL
  SELECT next_step.id, next_step.next_id, next_step.predicate, next_step.status, next_step.notes_ar, next_step.depth + 1,
    next_step.node_ids || next_step.next_id, next_step.visited_ids || next_step.next_id, next_step.claim_ids || next_step.id
  FROM (
    SELECT c.id, n.next_id, c.predicate, c.status, c.notes_ar, w.depth,
      w.node_ids, w.visited_ids, w.claim_ids
    FROM walk w
    JOIN eligible_claims c ON c.subject_id = w.to_id OR c.object_id = w.to_id
    CROSS JOIN parameters p
    CROSS JOIN LATERAL (SELECT CASE WHEN c.subject_id = w.to_id THEN c.object_id ELSE c.subject_id END AS next_id) n
    WHERE w.depth < p.max_depth AND c.subject_id <> c.object_id AND c.id <> ALL(w.claim_ids)
      AND n.next_id <> ALL(w.visited_ids)
    ORDER BY c.id
    LIMIT $7
  ) next_step
),
candidate_paths AS (
  SELECT DISTINCT ON (claim_ids) claim_id, to_id, predicate, status, notes_ar, depth, node_ids, visited_ids, claim_ids
  FROM walk
  WHERE to_id = $2
  ORDER BY claim_ids, depth
),
ranked_paths AS (
  SELECT cp.*, row_number() OVER (ORDER BY depth, claim_ids::text) AS path_order, count(*) OVER () AS candidate_count
  FROM candidate_paths cp
),
limited_paths AS (
  SELECT * FROM ranked_paths WHERE path_order <= $6
),
path_nodes AS (
  SELECT lp.path_order, jsonb_agg(jsonb_build_object('id', n.node_id::text, 'type', p.endpoint_type, 'label', COALESCE(el.label, ''), 'position', n.node_order - 1) ORDER BY n.node_order) AS nodes
  FROM limited_paths lp
  CROSS JOIN parameters p
  CROSS JOIN LATERAL unnest(lp.node_ids) WITH ORDINALITY AS n(node_id, node_order)
  LEFT JOIN entity_labels el ON el.type = p.endpoint_type AND el.id = n.node_id
  GROUP BY lp.path_order
),
path_edges AS (
  SELECT lp.path_order, c.id AS edge_id, c.predicate, c.status, c.notes_ar, c.subject_type, c.subject_id, c.object_type, c.object_id, n2.node_id AS from_node_id,
    (SELECT n2.node_id FROM unnest(lp.node_ids) WITH ORDINALITY AS n2(node_id, node_order) WHERE n2.node_order = n.node_order + 1) AS to_node_id,
    n.node_order - 1 AS position
  FROM limited_paths lp
  CROSS JOIN LATERAL unnest(lp.claim_ids) WITH ORDINALITY AS n(claim_id, node_order)
  JOIN eligible_claims c ON c.id = n.claim_id
  CROSS JOIN LATERAL unnest(lp.node_ids) WITH ORDINALITY AS n2(node_id, node_order)
  WHERE n2.node_order = n.node_order
),
path_edges_json AS (
  SELECT path_order, jsonb_agg(jsonb_build_object('id', edge_id::text, 'type', 'claim', 'fromNodeId', from_node_id::text, 'toNodeId', to_node_id::text, 'predicate', predicate, 'status', status, 'claimId', edge_id::text, 'position', position) ORDER BY position) AS edges,
    bool_or(status IN ('contested', 'disputed', 'contradicted')) AS contested,
    bool_or(status IN ('inferred', 'platform_generated')) AS partial
  FROM path_edges
  GROUP BY path_order
),
path_refs AS (
  SELECT lp.path_order, jsonb_agg(r.ref ORDER BY n.node_order, r.ref_order) AS refs
  FROM limited_paths lp
  CROSS JOIN LATERAL unnest(lp.claim_ids) WITH ORDINALITY AS n(claim_id, node_order)
  LEFT JOIN claim_refs cr ON cr.claim_id = n.claim_id
  CROSS JOIN LATERAL jsonb_array_elements(COALESCE(cr.refs, '[]'::jsonb)) WITH ORDINALITY AS r(ref, ref_order)
  GROUP BY lp.path_order
)
SELECT 'evidence_connection',
  CASE WHEN COALESCE(pe.contested, false) THEN 'contested' WHEN COALESCE(pe.partial, false) THEN 'partial' ELSE 'complete' END,
  0::integer,
  NULL::text, NULL::text, NULL::text,
  lp.depth::integer,
  (lp.candidate_count > $6 OR lp.depth >= $5),
  true, false,
  pn.nodes, COALESCE(pe.edges, '[]'::jsonb), COALESCE(pr.refs, '[]'::jsonb)
FROM limited_paths lp
LEFT JOIN path_nodes pn ON pn.path_order = lp.path_order
LEFT JOIN path_edges_json pe ON pe.path_order = lp.path_order
LEFT JOIN path_refs pr ON pr.path_order = lp.path_order
ORDER BY lp.path_order
`

const branchClaimsGraphQuery = `
WITH
parameters AS (
  SELECT $1::uuid AS start_id, $2::text AS entity_type, $3::uuid AS actor_id, $4::integer AS max_edges
),
visible_support AS (
  SELECT ce.claim_id, ss.id AS reference_id, ce.relation, ss.id AS statement_id, sp.id AS passage_id,
    COALESCE(ss.source_id, sp.source_id) AS source_id, s.title_ar,
    COALESCE(ss.statement_text_ar, sp.text_ar, '') AS excerpt,
    COALESCE(ss.locator_ar, sp.locator_ar) AS locator_ar, sp.page_number, ss.review_status
  FROM claim_evidence ce
  JOIN source_statements ss ON ss.id = ce.source_statement_id AND ss.review_status = 'accepted'
  LEFT JOIN source_passages sp ON sp.id = COALESCE(ce.source_passage_id, ss.source_passage_id)
  JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
  CROSS JOIN parameters p
  WHERE s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin'))))
),
visible_counter AS (
  SELECT cce.claim_id, ss.id AS reference_id, 'counter_evidence'::text AS relation, ss.id AS statement_id, sp.id AS passage_id,
    COALESCE(ss.source_id, sp.source_id) AS source_id, s.title_ar,
    COALESCE(ss.statement_text_ar, sp.text_ar, '') AS excerpt,
    COALESCE(ss.locator_ar, sp.locator_ar) AS locator_ar, sp.page_number, ss.review_status
  FROM claim_counter_evidence cce
  JOIN source_statements ss ON ss.id = cce.source_statement_id AND ss.review_status = 'accepted'
  LEFT JOIN source_passages sp ON sp.id = COALESCE(cce.source_passage_id, ss.source_passage_id)
  JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
  CROSS JOIN parameters p
  WHERE s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin'))))
),
visible_evidence AS (
  SELECT * FROM visible_support
  UNION ALL
  SELECT * FROM visible_counter
),
eligible_claims AS (
  SELECT c.*
  FROM claims c
  CROSS JOIN parameters p
  WHERE ((c.subject_type = p.entity_type AND c.subject_id = p.start_id) OR (c.object_type = p.entity_type AND c.object_id = p.start_id))
    AND c.status NOT IN ('rejected', 'superseded', 'unknown', 'unresolved')
    AND EXISTS (SELECT 1 FROM visible_support vs WHERE vs.claim_id = c.id)
),
limited_claims AS (
  SELECT * FROM eligible_claims ORDER BY id LIMIT $4
),
ranked_visible_evidence AS (
  SELECT ve.*, row_number() OVER (PARTITION BY ve.claim_id ORDER BY ve.relation, ve.source_id::text, ve.reference_id::text) AS evidence_rank
  FROM visible_evidence ve
  JOIN limited_claims lc ON lc.id = ve.claim_id
),
claim_refs AS (
  SELECT ve.claim_id, jsonb_agg(jsonb_build_object('id', ve.reference_id::text, 'type', CASE WHEN ve.statement_id IS NOT NULL THEN 'source_statement' ELSE 'source_passage' END, 'layer', CASE WHEN ve.statement_id IS NOT NULL THEN 'source_statement' ELSE 'source_passage' END, 'relation', ve.relation, 'sourceId', ve.source_id::text, 'claimId', ve.claim_id::text, 'statementId', ve.statement_id::text, 'passageId', ve.passage_id::text, 'reviewStatus', ve.review_status, 'title', ve.title_ar, 'excerpt', ve.excerpt, 'locatorAr', ve.locator_ar, 'pageNumber', ve.page_number) ORDER BY ve.relation, ve.source_id::text, ve.reference_id::text) AS refs
  FROM ranked_visible_evidence ve
  WHERE ve.evidence_rank <= 50
  GROUP BY ve.claim_id
),
entity_labels AS (
  SELECT 'person'::text AS type, id, canonical_name_ar AS label FROM people WHERE merged_into_id IS NULL
  UNION ALL SELECT 'family', id, canonical_name_ar FROM families WHERE merged_into_id IS NULL
  UNION ALL SELECT 'branch', id, canonical_name_ar FROM branches
  UNION ALL SELECT 'place', id, canonical_name_ar FROM places
),
path_nodes AS (
  SELECT jsonb_agg(jsonb_build_object('id', n.node_id::text, 'type', n.node_type, 'label', COALESCE(el.label, ''), 'position', n.node_order - 1) ORDER BY n.node_order) AS nodes
  FROM (
    SELECT p.start_id AS node_id, p.entity_type AS node_type, 1 AS node_order
    FROM parameters p
    UNION ALL
    SELECT CASE WHEN c.subject_type = p.entity_type AND c.subject_id = p.start_id THEN c.object_id ELSE c.subject_id END,
      CASE WHEN c.subject_type = p.entity_type AND c.subject_id = p.start_id THEN c.object_type ELSE c.subject_type END,
      row_number() OVER (ORDER BY c.id) + 1
    FROM limited_claims c
    CROSS JOIN parameters p
  ) n
  LEFT JOIN entity_labels el ON el.type = n.node_type AND el.id = n.node_id
),
path_edges AS (
  SELECT c.id AS edge_id, c.predicate, c.status, c.subject_type, c.subject_id, c.object_type, c.object_id,
    p.start_id AS from_node_id,
    CASE WHEN c.subject_type = p.entity_type AND c.subject_id = p.start_id THEN c.object_id ELSE c.subject_id END AS to_node_id,
    row_number() OVER (ORDER BY c.id) - 1 AS position
  FROM limited_claims c
  CROSS JOIN parameters p
),
path_refs AS (
  SELECT jsonb_agg(ref ORDER BY ref_order) AS refs
  FROM (
    SELECT cr.claim_id, ref.value AS ref, ref.ordinality AS ref_order
    FROM limited_claims c
    JOIN claim_refs cr ON cr.claim_id = c.id
    CROSS JOIN LATERAL jsonb_array_elements(cr.refs) WITH ORDINALITY AS ref(value, ordinality)
  ) refs
)
SELECT 'branch_claims',
  CASE WHEN bool_or(c.status IN ('contested', 'disputed', 'contradicted')) THEN 'contested' WHEN bool_or(c.status IN ('inferred', 'platform_generated')) THEN 'partial' ELSE 'complete' END,
  0::integer,
  NULL::text, NULL::text, NULL::text,
  1::integer,
  (SELECT count(*) FROM eligible_claims) > $4,
  true, false,
  (SELECT nodes FROM path_nodes),
  COALESCE((SELECT jsonb_agg(jsonb_build_object('id', edge_id::text, 'type', 'claim', 'fromNodeId', from_node_id::text, 'toNodeId', to_node_id::text, 'predicate', predicate, 'status', status, 'claimId', edge_id::text, 'position', position) ORDER BY position) FROM path_edges), '[]'::jsonb),
  COALESCE((SELECT refs FROM path_refs), '[]'::jsonb)
FROM limited_claims c
LIMIT 1
`

const sourceEntitiesGraphQuery = `
WITH
parameters AS (
  SELECT $1::uuid AS source_id, $2::uuid AS actor_id, $3::integer AS max_paths, $4::integer AS max_edges, $5::integer AS max_depth
),
visible_statements AS (
  SELECT ss.id AS statement_id, ss.source_id, ss.source_passage_id, ss.statement_text_ar, ss.locator_ar,
    s.id AS source_record_id, s.title_ar
  FROM source_statements ss
  JOIN sources s ON s.id = ss.source_id
  CROSS JOIN parameters p
  WHERE ss.source_id = p.source_id AND ss.review_status = 'accepted'
    AND (s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin')))))
  ORDER BY ss.id
  LIMIT $4
),
claim_links AS (
  SELECT DISTINCT ON (c.id, vs.statement_id, side.side)
    c.id AS claim_id, c.subject_type, c.subject_id, c.predicate, c.object_type, c.object_id, c.status, c.notes_ar,
    vs.statement_id, vs.source_record_id, vs.source_record_id AS source_id, vs.title_ar, vs.statement_text_ar, vs.locator_ar,
    side.side AS entity_side, side.entity_type, side.entity_id
  FROM visible_statements vs
  JOIN claim_evidence ce ON ce.source_statement_id = vs.statement_id OR ce.source_passage_id = vs.source_passage_id
  JOIN claims c ON c.id = ce.claim_id
  CROSS JOIN LATERAL (VALUES ('subject', c.subject_type, c.subject_id), ('object', c.object_type, c.object_id)) AS side(side, entity_type, entity_id)
  WHERE c.status NOT IN ('rejected', 'superseded', 'unknown', 'unresolved')
  ORDER BY c.id, vs.statement_id, side.side
),
ranked_source_evidence AS (
  SELECT c.id AS claim_id, ss.statement_id, ss.source_id, ss.source_passage_id, ce.relation, s.title_ar, ss.statement_text_ar, ss.locator_ar,
    row_number() OVER (PARTITION BY c.id ORDER BY ce.relation, ss.statement_id) AS evidence_rank
  FROM visible_statements ss
  JOIN claim_evidence ce ON ce.source_statement_id = ss.statement_id OR ce.source_passage_id = ss.source_passage_id
  JOIN claims c ON c.id = ce.claim_id
  JOIN sources s ON s.id = ss.source_id
  WHERE c.id IN (SELECT claim_id FROM claim_links)
),
claim_refs AS (
  SELECT claim_id,
    jsonb_agg(jsonb_build_object('id', statement_id::text, 'type', 'source_statement', 'layer', 'source_statement', 'relation', relation, 'sourceId', source_id::text, 'claimId', claim_id::text, 'statementId', statement_id::text, 'passageId', source_passage_id::text, 'reviewStatus', 'accepted', 'title', title_ar, 'excerpt', statement_text_ar, 'locatorAr', locator_ar) ORDER BY relation, statement_id) AS refs
  FROM ranked_source_evidence
  WHERE evidence_rank <= 50
  GROUP BY claim_id
),
entity_labels AS (
  SELECT 'person'::text AS type, id, canonical_name_ar AS label FROM people WHERE merged_into_id IS NULL
  UNION ALL SELECT 'family', id, canonical_name_ar FROM families WHERE merged_into_id IS NULL
  UNION ALL SELECT 'branch', id, canonical_name_ar FROM branches
  UNION ALL SELECT 'tribe', id, canonical_name_ar FROM tribes
  UNION ALL SELECT 'place', id, canonical_name_ar FROM places
),
ranked_links AS (
  SELECT cl.*, row_number() OVER (ORDER BY cl.claim_id, cl.statement_id, cl.entity_side) AS path_order,
    count(*) OVER () AS candidate_count
  FROM claim_links cl
  WHERE cl.entity_id IS NOT NULL
),
limited_links AS (
  SELECT * FROM ranked_links WHERE path_order <= $3
),
path_nodes AS (
  SELECT p.path_order, jsonb_build_array(
    jsonb_build_object('id', p.source_id::text, 'type', 'source', 'label', p.title_ar, 'position', 0),
    jsonb_build_object('id', p.statement_id::text, 'type', 'source_statement', 'label', p.statement_text_ar, 'position', 1),
    jsonb_build_object('id', p.claim_id::text, 'type', 'claim', 'label', p.predicate, 'position', 2),
    jsonb_build_object('id', p.entity_id::text, 'type', p.entity_type, 'label', COALESCE(el.label, ''), 'position', 3)
  ) AS nodes
  FROM limited_links p
  LEFT JOIN entity_labels el ON el.type = p.entity_type AND el.id = p.entity_id
),
path_edges AS (
  SELECT p.path_order, jsonb_build_array(
      jsonb_build_object('id', p.statement_id::text, 'type', 'contains_statement', 'fromNodeId', p.source_record_id::text, 'toNodeId', p.statement_id::text, 'sourceId', p.source_id::text, 'statementId', p.statement_id::text, 'position', 0),
      jsonb_build_object('id', p.claim_id::text, 'type', 'supports_claim', 'fromNodeId', p.statement_id::text, 'toNodeId', p.claim_id::text, 'sourceId', p.source_id::text, 'claimId', p.claim_id::text, 'statementId', p.statement_id::text, 'status', p.status, 'position', 1),
      jsonb_build_object('id', p.claim_id::text, 'type', 'claim_entity', 'fromNodeId', p.claim_id::text, 'toNodeId', p.entity_id::text, 'sourceId', p.source_id::text, 'claimId', p.claim_id::text, 'status', p.status, 'position', 2)
    ) AS edges
  FROM limited_links p
),
path_refs AS (
  SELECT p.path_order, COALESCE(cr.refs, jsonb_build_array(
    jsonb_build_object('id', p.statement_id::text, 'type', 'source_statement', 'layer', 'source_statement', 'relation', 'supports', 'sourceId', p.source_id::text, 'claimId', p.claim_id::text, 'statementId', p.statement_id::text, 'title', p.title_ar, 'excerpt', p.statement_text_ar, 'locatorAr', p.locator_ar)
  )) AS refs
  FROM limited_links p
  LEFT JOIN claim_refs cr ON cr.claim_id = p.claim_id
)
SELECT 'source_entities', CASE WHEN l.status IN ('contested', 'disputed', 'contradicted') THEN 'contested' WHEN l.status IN ('inferred', 'platform_generated') THEN 'partial' ELSE 'complete' END,
  0::integer, NULL::text, NULL::text, NULL::text, LEAST(3, $5)::integer,
  (l.candidate_count > $3), true, false,
  pn.nodes, pe.edges, pr.refs
FROM limited_links l
JOIN path_nodes pn ON pn.path_order = l.path_order
JOIN path_edges pe ON pe.path_order = l.path_order
JOIN path_refs pr ON pr.path_order = l.path_order
ORDER BY l.path_order
`

const geographicGraphQuery = `
WITH
parameters AS (
  SELECT $1::uuid AS subject_id, $2::text AS subject_type, $3::uuid AS end_place_id, $4::uuid AS actor_id, $5::integer AS from_year, $6::integer AS to_year, $7::integer AS max_paths, $8::integer AS max_edges, $9::integer AS max_depth
),
eligible_events AS (
  SELECT me.*
  FROM migration_events me
  CROSS JOIN parameters p
  WHERE ((p.subject_type = 'place' AND (me.from_place_id = p.subject_id OR me.to_place_id = p.subject_id)) OR (me.subject_type = p.subject_type AND me.subject_id = p.subject_id))
    AND (p.end_place_id IS NULL OR me.from_place_id = p.end_place_id OR me.to_place_id = p.end_place_id)
    AND (p.from_year = 0 OR me.time_to IS NULL OR EXTRACT(YEAR FROM me.time_to)::integer >= p.from_year)
    AND (p.to_year = 0 OR me.time_from IS NULL OR EXTRACT(YEAR FROM me.time_from)::integer <= p.to_year)
    AND (me.source_id IS NULL OR EXISTS (SELECT 1 FROM sources s WHERE s.id = me.source_id AND (s.visibility = 'public' OR (p.actor_id IS NOT NULL AND (s.created_by = p.actor_id OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = p.actor_id AND ur.role IN ('researcher', 'moderator', 'admin')))))))
  ORDER BY me.time_from NULLS LAST, me.time_to NULLS LAST, me.id
  LIMIT $8
),
limited_events AS (
  SELECT me.*, row_number() OVER (ORDER BY me.time_from NULLS LAST, me.time_to NULLS LAST, me.id) AS path_order,
    count(*) OVER () AS candidate_count
  FROM eligible_events me
),
path_rows AS (
  SELECT le.*, le.id AS event_id, le.source_id AS event_source_id
  FROM limited_events le
  WHERE le.path_order <= $7
),
entity_labels AS (
  SELECT 'person'::text AS type, id, canonical_name_ar AS label FROM people WHERE merged_into_id IS NULL
  UNION ALL SELECT 'family', id, canonical_name_ar FROM families WHERE merged_into_id IS NULL
  UNION ALL SELECT 'branch', id, canonical_name_ar FROM branches
  UNION ALL SELECT 'place', id, canonical_name_ar FROM places
)
SELECT 'geographic_path', CASE WHEN pr.status IN ('contested', 'disputed') THEN 'contested' WHEN pr.status IN ('interpreted', 'platform_inferred', 'unresolved') THEN 'partial' ELSE 'complete' END,
  0::integer, NULL::text, NULL::text, NULL::text, 1::integer,
  (pr.candidate_count > $7 OR pr.candidate_count > $9), (pr.source_id IS NOT NULL OR pr.claim_id IS NOT NULL), (pr.source_id IS NULL AND pr.claim_id IS NULL),
  jsonb_build_array(
    jsonb_build_object('id', pr.subject_id::text, 'type', pr.subject_type, 'label', COALESCE(subject_label.label, ''), 'position', 0),
    jsonb_build_object('id', COALESCE(pr.from_place_id, pr.id)::text, 'type', CASE WHEN pr.from_place_id IS NULL THEN 'unknown_place' ELSE 'place' END, 'label', COALESCE(from_label.label, ''), 'position', 1),
    jsonb_build_object('id', COALESCE(pr.to_place_id, pr.id)::text, 'type', CASE WHEN pr.to_place_id IS NULL THEN 'unknown_place' ELSE 'place' END, 'label', COALESCE(to_label.label, ''), 'position', 2)
  ),
  jsonb_build_array(jsonb_build_object('id', pr.id::text, 'type', 'migration_event', 'fromNodeId', COALESCE(pr.from_place_id, pr.id)::text, 'toNodeId', COALESCE(pr.to_place_id, pr.id)::text, 'predicate', 'migrated_to', 'status', pr.status, 'certainty', pr.certainty, 'sourceId', pr.source_id::text, 'claimId', pr.claim_id::text, 'migrationEventId', pr.id::text, 'fromPlaceId', pr.from_place_id::text, 'toPlaceId', pr.to_place_id::text, 'position', 0)),
  CASE WHEN pr.source_id IS NOT NULL OR pr.claim_id IS NOT NULL THEN jsonb_build_array(jsonb_build_object('id', pr.id::text, 'type', 'migration_event', 'layer', 'geographic_event', 'relation', 'event', 'sourceId', pr.source_id::text, 'claimId', pr.claim_id::text, 'status', pr.status, 'certainty', pr.certainty, 'title', 'حدث انتقال', 'excerpt', COALESCE(pr.notes_ar, ''))) ELSE '[]'::jsonb END
FROM path_rows pr
LEFT JOIN entity_labels subject_label ON subject_label.type = pr.subject_type AND subject_label.id = pr.subject_id
LEFT JOIN entity_labels from_label ON from_label.type = 'place' AND from_label.id = pr.from_place_id
LEFT JOIN entity_labels to_label ON to_label.type = 'place' AND to_label.id = pr.to_place_id
ORDER BY pr.path_order
`
