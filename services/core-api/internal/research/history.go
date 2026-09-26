package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// publicSourceExists is the one statement the deliberately public source dependency
// graph asks about a source, written as a fragment so the single-source decision
// and the batched one read the same rule.
const publicSourceExists = `EXISTS (SELECT 1 FROM sources s WHERE s.id = %s AND s.visibility = 'public')`

type ResearchRunSummary struct {
	ID                   string    `json:"id"`
	QuestionID           string    `json:"questionId,omitempty"`
	Query                string    `json:"query"`
	Status               string    `json:"status"`
	InsufficientEvidence bool      `json:"insufficientEvidence"`
	CitationCount        int       `json:"citationCount"`
	GraphPathCount       int       `json:"graphPathCount"`
	GraphOperation       string    `json:"graphOperation,omitempty"`
	GraphMaxDepth        int       `json:"graphMaxDepth,omitempty"`
	GraphTruncated       bool      `json:"graphTruncated"`
	AnswerAR             string    `json:"answerAr,omitempty"`
	ModelVersion         string    `json:"modelVersion,omitempty"`
	Route                string    `json:"route,omitempty"`
	SynthesisAttempted   bool      `json:"synthesisAttempted"`
	Error                string    `json:"error,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type ResearchRunContext struct {
	ScopeType string `json:"scopeType"`
	ScopeID   string `json:"scopeId"`
	Role      string `json:"role"`
}

type ResearchRunDetail struct {
	ResearchRunSummary
	Contexts                     []ResearchRunContext                      `json:"contexts"`
	GraphPaths                   []GraphPath                               `json:"graphPaths"`
	Comparison                   *GraphBranchStructureComparisonResult     `json:"comparison,omitempty"`
	AncestorFrontier             *GraphAncestorFrontierSummary             `json:"ancestorFrontier,omitempty"`
	SourceDependencyNeighborhood *GraphSourceDependencyNeighborhoodSummary `json:"sourceDependencyNeighborhood,omitempty"`
	SourceDependencyCommunities  *GraphSourceDependencyCommunitiesSummary  `json:"sourceDependencyCommunities,omitempty"`
}

type historyExecutor interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) ListRuns(ctx context.Context, actorID, questionID string) ([]ResearchRunSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	questionUUID, err := uuid.Parse(strings.TrimSpace(questionID))
	if err != nil {
		return nil, ErrValidation
	}
	policy, err := loadHistoryPolicy(ctx, s.Pool, actorID)
	if err != nil {
		return nil, err
	}
	// Research run metadata is research-only data. An unauthorized caller is refused
	// before the summary query runs, so the answer never depends on whether the run
	// or the question exists and the response cannot be an existence oracle.
	if !policy.CanViewResearchHistory() {
		return nil, ErrForbidden
	}
	includeAnswer := true
	summaries, err := listRuns(ctx, s.Pool, questionUUID, includeAnswer)
	if err != nil {
		return nil, err
	}
	return s.filterResearchRunSummaries(ctx, actorID, summaries)
}

func (s *Service) GetRun(ctx context.Context, actorID, runID string) (ResearchRunDetail, error) {
	if err := s.ready(); err != nil {
		return ResearchRunDetail{}, err
	}
	runUUID, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return ResearchRunDetail{}, ErrNotFound
	}
	policy, err := loadHistoryPolicy(ctx, s.Pool, actorID)
	if err != nil {
		return ResearchRunDetail{}, err
	}
	// The refusal happens before the summary is loaded, so an anonymous or
	// unregistered caller cannot read query text, status, model, route, counts or
	// timestamps, and a denied run is indistinguishable from a missing one.
	if !policy.CanViewResearchHistory() {
		return ResearchRunDetail{}, ErrForbidden
	}
	includeAnswer := true
	allowed, accessErr := s.canAccessPersistedRun(ctx, runUUID, actorID)
	if accessErr != nil {
		return ResearchRunDetail{}, accessErr
	}
	if !allowed {
		return ResearchRunDetail{}, ErrForbidden
	}
	summary, err := scanRunSummary(s.Pool.QueryRow(ctx, `
		SELECT rr.id, rr.question_id, rr.query, rr.status, rr.insufficient_evidence,
		       count(DISTINCT re.id), CASE WHEN $2 THEN count(DISTINCT rgp.id) ELSE 0 END, CASE WHEN $2 THEN COALESCE(rr.graph_operation, '') ELSE '' END, CASE WHEN $2 THEN rr.graph_max_depth ELSE NULL END, CASE WHEN $2 THEN COALESCE(rr.graph_truncated, false) ELSE false END,
		       CASE WHEN $2 THEN COALESCE(ra.answer, '') ELSE '' END,
		       COALESCE(rr.model_version, ''), COALESCE(rr.semantic_route, ''), rr.synthesis_attempted,
		       CASE WHEN $2 THEN COALESCE(rr.error, '') ELSE '' END, rr.created_at, rr.updated_at
		FROM research_runs rr
		LEFT JOIN research_answers ra ON ra.run_id = rr.id
		LEFT JOIN research_evidence re ON re.run_id = rr.id
		LEFT JOIN research_graph_paths rgp ON rgp.run_id = rr.id
		WHERE rr.id = $1
		GROUP BY rr.id, ra.answer
	`, runUUID, includeAnswer))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResearchRunDetail{}, ErrNotFound
		}
		return ResearchRunDetail{}, err
	}
	contexts := make([]ResearchRunContext, 0)
	graphPaths := make([]GraphPath, 0)
	var comparison *GraphBranchStructureComparisonResult
	var ancestorFrontier *GraphAncestorFrontierSummary
	var sourceDependencyNeighborhood *GraphSourceDependencyNeighborhoodSummary
	var sourceDependencyCommunities *GraphSourceDependencyCommunitiesSummary
	if includeAnswer {
		contexts, err = runContexts(ctx, s.Pool, runUUID)
		if err != nil {
			return ResearchRunDetail{}, err
		}
		graphPaths, err = runGraphPaths(ctx, s.Pool, runUUID)
		if err != nil {
			return ResearchRunDetail{}, err
		}
		graphPaths, err = s.filterPersistedGraphPaths(ctx, actorID, graphPaths)
		if err != nil {
			return ResearchRunDetail{}, err
		}
		summary.GraphPathCount = len(graphPaths)
		if len(graphPaths) == 0 {
			summary.GraphOperation = ""
			summary.GraphMaxDepth = 0
			summary.GraphTruncated = false
		}
		comparison, err = runGraphComparison(ctx, s.Pool, runUUID)
		if err != nil {
			return ResearchRunDetail{}, err
		}
		ancestorFrontier, err = runGraphAncestorFrontier(ctx, s.Pool, runUUID)
		if err != nil {
			return ResearchRunDetail{}, err
		}
		if graphPathsContainOperation(graphPaths, GraphOperationSourceDependency) {
			sourceDependencyNeighborhood, err = runGraphSourceDependencyNeighborhood(ctx, s.Pool, runUUID)
			if err != nil {
				return ResearchRunDetail{}, err
			}
		}
		if graphPathsContainOperation(graphPaths, GraphOperationSourceCommunities) {
			sourceDependencyCommunities, err = runGraphSourceDependencyCommunities(ctx, s.Pool, runUUID)
			if err != nil {
				return ResearchRunDetail{}, err
			}
		}
	}
	return ResearchRunDetail{ResearchRunSummary: summary, Contexts: contexts, GraphPaths: graphPaths, Comparison: comparison, AncestorFrontier: ancestorFrontier, SourceDependencyNeighborhood: sourceDependencyNeighborhood, SourceDependencyCommunities: sourceDependencyCommunities}, nil
}

func graphPathsContainOperation(paths []GraphPath, operation string) bool {
	for _, path := range paths {
		if path.Operation == operation {
			return true
		}
	}
	return false
}

func (s *Service) filterResearchRunSummaries(ctx context.Context, actorID string, summaries []ResearchRunSummary) ([]ResearchRunSummary, error) {
	return s.filterResearchRunSummariesWithExecutor(ctx, s.Pool, actorID, summaries)
}

// filterResearchRunSummariesWithExecutor hides every run whose authorization fails
// and returns the rest, in the order they were given.
//
// The authorization itself is resolved once for the whole page: the runs name
// their trees, evidence sources and graph sources in three set-based reads, the
// policy decides every one of those resources in two more, and the all-or-nothing
// rule is then applied per run in memory. It used to be one role lookup plus three
// structural reads plus one statement per resource, for every run in the page -
// a hundred runs cost several hundred round trips to render one list. The verdicts
// are unchanged: same grants, same all-or-nothing rule, same error propagation -
// only the number of statements changed.
func (s *Service) filterResearchRunSummariesWithExecutor(ctx context.Context, executor historyExecutor, actorID string, summaries []ResearchRunSummary) ([]ResearchRunSummary, error) {
	filtered := make([]ResearchRunSummary, 0, len(summaries))
	if len(summaries) == 0 {
		return filtered, nil
	}
	actorUUID, err := parseRunActor(actorID)
	if err != nil {
		return nil, err
	}
	policy, err := resolveResearchScope(ctx, executor, actorUUID)
	if err != nil {
		return nil, err
	}
	runIDs := make([]uuid.UUID, 0, len(summaries))
	for _, summary := range summaries {
		// A summary whose id is not a uuid cannot name a run, so nothing can
		// vouch for it and it is dropped rather than passed on.
		if runID := optionalUUID(summary.ID); runID != uuid.Nil {
			runIDs = append(runIDs, runID)
		}
	}
	resources, err := collectRunResources(ctx, executor, runIDs)
	if err != nil {
		return nil, err
	}
	verdicts, err := resolveRunVisibility(ctx, executor, policy, resources)
	if err != nil {
		return nil, err
	}
	for _, summary := range summaries {
		runID := optionalUUID(summary.ID)
		scoped, found := resources[runID]
		if !found {
			// The run names no resource at all, or the id was not one. Either way
			// there is nothing the reader could be denied, and a run with no
			// resources is exactly the run the per-run walk allowed.
			if found || runID != uuid.Nil {
				filtered = append(filtered, summary)
			}
			continue
		}
		allowed, allowErr := verdicts.allows(scoped)
		if allowErr != nil {
			return nil, allowErr
		}
		if allowed {
			filtered = append(filtered, summary)
		}
	}
	return filtered, nil
}

// parseRunActor turns the actor identifier into a uuid, mapping a malformed one
// onto the package's own forbidden error so a list cannot be told apart from a
// call that named nobody.
func parseRunActor(actorID string) (uuid.UUID, error) {
	if strings.TrimSpace(actorID) == "" {
		return uuid.Nil, nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return uuid.Nil, ErrForbidden
	}
	return parsed, nil
}

// runResources is everything a persisted run names that the policy has a say
// about. The three lists are the three different rules: a tree is scoped by the
// tree policy, an evidence source by the source policy, and a source that appears
// only in the graph by the deliberately public source contract.
type runResources struct {
	trees        []uuid.UUID
	sources      []uuid.UUID
	graphSources []uuid.UUID
}

// collectRunResources reads the resources of every named run in three statements
// rather than three per run. Each statement keeps the exact predicate the per-run
// walk used, so which resources a run is judged on has not changed.
func collectRunResources(ctx context.Context, executor historyExecutor, runIDs []uuid.UUID) (map[uuid.UUID]runResources, error) {
	resources := make(map[uuid.UUID]runResources, len(runIDs))
	if len(runIDs) == 0 {
		return resources, nil
	}
	for _, runID := range runIDs {
		resources[runID] = runResources{}
	}
	rows, err := executor.Query(ctx, `SELECT run_id::text, tree_id::text FROM research_graph_paths WHERE run_id = ANY($1) AND tree_id IS NOT NULL`, runIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var runID, treeID string
		if err := rows.Scan(&runID, &treeID); err != nil {
			rows.Close()
			return nil, err
		}
		scoped := resources[optionalUUID(runID)]
		scoped.trees = append(scoped.trees, optionalUUID(treeID))
		resources[optionalUUID(runID)] = scoped
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = executor.Query(ctx, `SELECT run_id::text, source_id::text FROM research_graph_path_evidence WHERE run_id = ANY($1) AND source_id IS NOT NULL AND (statement_id IS NOT NULL OR passage_id IS NOT NULL OR claim_id IS NOT NULL)`, runIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var runID, sourceID string
		if err := rows.Scan(&runID, &sourceID); err != nil {
			rows.Close()
			return nil, err
		}
		scoped := resources[optionalUUID(runID)]
		scoped.sources = append(scoped.sources, optionalUUID(sourceID))
		resources[optionalUUID(runID)] = scoped
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = executor.Query(ctx, `
		SELECT run_id::text, identifier
		FROM (
			SELECT rgp.run_id, node->>'id' AS identifier
			FROM research_graph_paths rgp
			CROSS JOIN LATERAL jsonb_array_elements(COALESCE(rgp.nodes, '[]'::jsonb)) node
			WHERE rgp.run_id = ANY($1) AND rgp.operation IN ($2, $3)
			UNION
			SELECT edge.run_id, edge.from_node_id::text
			FROM research_graph_edges edge
			JOIN research_graph_paths rgp ON rgp.id = edge.path_id
			WHERE edge.run_id = ANY($1) AND rgp.operation IN ($2, $3)
			UNION
			SELECT edge.run_id, edge.to_node_id::text
			FROM research_graph_edges edge
			JOIN research_graph_paths rgp ON rgp.id = edge.path_id
			WHERE edge.run_id = ANY($1) AND rgp.operation IN ($2, $3)
			UNION
			SELECT neighborhood.run_id, neighborhood.root_source_id::text
			FROM research_graph_source_dependency_neighborhoods neighborhood
			WHERE neighborhood.run_id = ANY($1)
			UNION
			SELECT community.run_id, community.root_source_id::text
			FROM research_graph_source_dependency_communities community
			WHERE community.run_id = ANY($1)
			UNION
			SELECT context.run_id, context.scope_id::text
			FROM research_run_contexts context
			JOIN research_runs run ON run.id = context.run_id
			WHERE context.run_id = ANY($1) AND context.scope_type = 'source' AND run.graph_operation IN ($2, $3)
		) identifiers
		WHERE identifier IS NOT NULL
	`, runIDs, GraphOperationSourceDependency, GraphOperationSourceCommunities)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var runID, sourceID string
		if err := rows.Scan(&runID, &sourceID); err != nil {
			rows.Close()
			return nil, err
		}
		scoped := resources[optionalUUID(runID)]
		scoped.graphSources = append(scoped.graphSources, optionalUUID(sourceID))
		resources[optionalUUID(runID)] = scoped
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return resources, nil
}

// runVisibility is the resolved access of every resource a batch of runs named.
type runVisibility struct {
	trees         map[uuid.UUID]visibility.Access
	sources       map[uuid.UUID]visibility.Access
	publicSources map[uuid.UUID]bool
}

// resolveRunVisibility decides every resource the runs named in three statements.
func resolveRunVisibility(ctx context.Context, executor historyExecutor, policy visibility.Policy, resources map[uuid.UUID]runResources) (runVisibility, error) {
	var treeIDs, sourceIDs, graphSourceIDs []uuid.UUID
	for _, scoped := range resources {
		treeIDs = append(treeIDs, scoped.trees...)
		sourceIDs = append(sourceIDs, scoped.sources...)
		graphSourceIDs = append(graphSourceIDs, scoped.graphSources...)
	}
	trees, err := policy.Trees(ctx, executor, treeIDs)
	if err != nil {
		return runVisibility{}, err
	}
	sources, err := policy.Sources(ctx, executor, sourceIDs)
	if err != nil {
		return runVisibility{}, err
	}
	published, err := canViewPersistedPublicSources(ctx, executor, graphSourceIDs)
	if err != nil {
		return runVisibility{}, err
	}
	return runVisibility{trees: trees, sources: sources, publicSources: published}, nil
}

// allows is the all-or-nothing rule from db/migrations/0004 atomic provenance: a
// run whose sources are not all visible stays hidden, and one tree or one
// evidence source the reader may not see is enough. A run that names nothing is
// visible, which is the same answer the per-run walk gave.
func (v runVisibility) allows(scoped runResources) (bool, error) {
	for _, treeID := range scoped.trees {
		if !v.trees[treeID].Allowed() {
			return false, nil
		}
	}
	for _, sourceID := range scoped.sources {
		if !v.sources[sourceID].Allowed() {
			return false, nil
		}
	}
	for _, sourceID := range scoped.graphSources {
		if sourceID == uuid.Nil {
			return false, nil
		}
		if !v.publicSources[sourceID] {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) canAccessPersistedRun(ctx context.Context, runID uuid.UUID, actorID string) (bool, error) {
	return s.canAccessPersistedRunWithExecutor(ctx, s.Pool, runID, actorID)
}

// canAccessPersistedRunWithExecutor answers for one run through the same set-based
// path the list uses, so the single-run and the list decision cannot be two
// different rules.
func (s *Service) canAccessPersistedRunWithExecutor(ctx context.Context, executor historyExecutor, runID uuid.UUID, actorID string) (bool, error) {
	if runID == uuid.Nil {
		return false, nil
	}
	actorUUID, err := parseRunActor(actorID)
	if err != nil {
		return false, err
	}
	// The policy is resolved once and reused for every tree and source in the run so
	// the all-or-nothing walk costs one role lookup instead of one per resource.
	policy, err := resolveResearchScope(ctx, executor, actorUUID)
	if err != nil {
		return false, err
	}
	resources, err := collectRunResources(ctx, executor, []uuid.UUID{runID})
	if err != nil {
		return false, err
	}
	verdicts, err := resolveRunVisibility(ctx, executor, policy, resources)
	if err != nil {
		return false, err
	}
	return verdicts.allows(resources[runID])
}

// filterPersistedGraphPaths redacts a run's graph paths under the caller's policy.
//
// The paths used to be walked one edge and one evidence ref at a time, each asking
// the policy about one source, so a run with five paths of two hundred edges cost
// hundreds of round trips to answer one request. Every source, tree and public-only
// graph source the paths name is now collected first and decided in three
// statements; the redaction rules below then run in memory. The rules, their order
// and their outcomes are unchanged: a hidden tree drops the path, a hidden endpoint
// drops a source-dependency path, a hidden edge source is blanked, and hidden
// evidence is either dropped or blanked according to whether it is direct.
func (s *Service) filterPersistedGraphPaths(ctx context.Context, actorID string, paths []GraphPath) ([]GraphPath, error) {
	actorUUID, err := parseRunActor(actorID)
	if err != nil {
		return nil, err
	}
	// Resolved once so walking the paths does not repeat the role lookup per edge.
	policy, err := resolveResearchScope(ctx, s.Pool, actorUUID)
	if err != nil {
		return nil, err
	}
	grants, err := resolveGraphPathVisibility(ctx, s.Pool, policy, paths)
	if err != nil {
		return nil, err
	}
	filtered := make([]GraphPath, 0, len(paths))
	for _, path := range paths {
		if path.TreeScope.TreeID != "" {
			if !grants.trees[optionalUUID(path.TreeScope.TreeID)].Allowed() {
				continue
			}
		}
		if isSourceDependencyGraphOperation(path.Operation) {
			visible, visibleErr := graphSourceDependencyPathVisible(path, grants.publicSources)
			if visibleErr != nil {
				return nil, visibleErr
			}
			if !visible {
				continue
			}
		}
		for index := range path.Edges {
			if path.Edges[index].SourceID == "" {
				continue
			}
			if !grants.sources[optionalUUID(path.Edges[index].SourceID)].Allowed() {
				path.Edges[index].SourceID = ""
			}
		}
		keptEvidence := make([]GraphEvidenceRef, 0, len(path.EvidenceRefs))
		for _, evidence := range path.EvidenceRefs {
			if evidence.SourceID != "" {
				if !grants.sources[optionalUUID(evidence.SourceID)].Allowed() {
					if graphEvidenceIsDirect(evidence) {
						continue
					}
					evidence.SourceID = ""
					evidence.Title = ""
					evidence.Excerpt = ""
					evidence.LocatorAR = ""
				}
			}
			keptEvidence = append(keptEvidence, evidence)
		}
		path.EvidenceRefs = keptEvidence
		path.EvidenceBacked = false
		for _, evidence := range path.EvidenceRefs {
			if graphEvidenceIsDirect(evidence) {
				path.EvidenceBacked = true
				break
			}
		}
		path.StructuralOnly = !path.EvidenceBacked
		path.Explanation = graphExplanation(path.Operation, path.Status, path.StructuralOnly)
		filtered = append(filtered, path)
	}
	return filtered, nil
}

// graphPathGrants is the resolved access of everything a run's graph paths name.
type graphPathGrants struct {
	trees         map[uuid.UUID]visibility.Access
	sources       map[uuid.UUID]visibility.Access
	publicSources map[uuid.UUID]bool
}

// resolveGraphPathVisibility decides every tree, source and graph source the paths
// name in three statements.
func resolveGraphPathVisibility(ctx context.Context, executor historyExecutor, policy visibility.Policy, paths []GraphPath) (graphPathGrants, error) {
	var treeIDs, sourceIDs, graphSourceIDs []uuid.UUID
	for _, path := range paths {
		if path.TreeScope.TreeID != "" {
			treeIDs = append(treeIDs, optionalUUID(path.TreeScope.TreeID))
		}
		sourceDependency := isSourceDependencyGraphOperation(path.Operation)
		for _, node := range path.Nodes {
			if node.Type != "source" {
				continue
			}
			if sourceDependency {
				graphSourceIDs = append(graphSourceIDs, optionalUUID(node.ID))
			}
		}
		for _, edge := range path.Edges {
			if sourceDependency {
				graphSourceIDs = append(graphSourceIDs, optionalUUID(edge.FromNodeID), optionalUUID(edge.ToNodeID))
			}
			if edge.SourceID != "" {
				sourceIDs = append(sourceIDs, optionalUUID(edge.SourceID))
			}
		}
		for _, evidence := range path.EvidenceRefs {
			if evidence.SourceID != "" {
				sourceIDs = append(sourceIDs, optionalUUID(evidence.SourceID))
			}
		}
	}
	trees, err := policy.Trees(ctx, executor, treeIDs)
	if err != nil {
		return graphPathGrants{}, err
	}
	sources, err := policy.Sources(ctx, executor, sourceIDs)
	if err != nil {
		return graphPathGrants{}, err
	}
	published, err := canViewPersistedPublicSources(ctx, executor, graphSourceIDs)
	if err != nil {
		return graphPathGrants{}, err
	}
	return graphPathGrants{trees: trees, sources: sources, publicSources: published}, nil
}

// graphSourceDependencyPathVisible keeps the deliberately public source dependency
// graph contract on a path: every source the path touches must be published. A node
// or edge endpoint that is not a source id at all is refused, as it always was.
func graphSourceDependencyPathVisible(path GraphPath, publicSources map[uuid.UUID]bool) (bool, error) {
	add := func(value string) bool {
		sourceID := optionalUUID(value)
		if sourceID == uuid.Nil {
			return false
		}
		return publicSources[sourceID]
	}
	for _, node := range path.Nodes {
		if node.Type == "source" && !add(node.ID) {
			return false, nil
		}
	}
	for _, edge := range path.Edges {
		if !add(edge.FromNodeID) || !add(edge.ToNodeID) {
			return false, nil
		}
	}
	return true, nil
}

// canViewPersistedPublicSources keeps the deliberately public source dependency graph
// contract for many sources in one statement: a public source is readable by
// everyone and nothing else is. The predicate is the single fragment below, so the
// rule has one spelling rather than one per caller.
func canViewPersistedPublicSources(ctx context.Context, executor historyExecutor, sourceIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	viewable := make(map[uuid.UUID]bool, len(sourceIDs))
	if len(sourceIDs) == 0 {
		return viewable, nil
	}
	rows, err := executor.Query(ctx, `
		SELECT requested.id, `+fmt.Sprintf(publicSourceExists, "requested.id")+`
		FROM unnest($1::uuid[]) AS requested(id)
	`, sourceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var allowed bool
		if err := rows.Scan(&id, &allowed); err != nil {
			return nil, err
		}
		viewable[id] = allowed
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// An id the server did not answer for is not viewable: the contract is
	// affirmative, so silence denies.
	for _, sourceID := range sourceIDs {
		if _, found := viewable[sourceID]; !found {
			viewable[sourceID] = false
		}
	}
	return viewable, nil
}

// loadHistoryPolicy resolves the central policy for a history caller and maps a
// malformed actor onto the package's own forbidden error, so the handler keeps its
// existing status mapping.
func loadHistoryPolicy(ctx context.Context, executor historyExecutor, actorID string) (visibility.Policy, error) {
	policy, err := visibility.Load(ctx, executor, actorID)
	if err != nil {
		if errors.Is(err, visibility.ErrForbidden) {
			return visibility.Policy{}, ErrForbidden
		}
		return visibility.Policy{}, err
	}
	return policy, nil
}

// resolveResearchScope fills in the research role the central policy needs. The actor
// identifier arrives as a uuid here, so the role lookup is run separately instead of
// re-parsing a string.
func resolveResearchScope(ctx context.Context, executor historyExecutor, actorID uuid.UUID) (visibility.Policy, error) {
	return loadHistoryPolicy(ctx, executor, actorID.String())
}

func listRuns(ctx context.Context, executor historyExecutor, questionID uuid.UUID, includeAnswer bool) ([]ResearchRunSummary, error) {
	rows, err := executor.Query(ctx, `
		SELECT rr.id, rr.question_id, rr.query, rr.status, rr.insufficient_evidence,
		       count(DISTINCT re.id), CASE WHEN $2 THEN count(DISTINCT rgp.id) ELSE 0 END, CASE WHEN $2 THEN COALESCE(rr.graph_operation, '') ELSE '' END, CASE WHEN $2 THEN rr.graph_max_depth ELSE NULL END, CASE WHEN $2 THEN COALESCE(rr.graph_truncated, false) ELSE false END,
		       CASE WHEN $2 THEN COALESCE(ra.answer, '') ELSE '' END,
		       COALESCE(rr.model_version, ''), COALESCE(rr.semantic_route, ''), rr.synthesis_attempted,
		       CASE WHEN $2 THEN COALESCE(rr.error, '') ELSE '' END, rr.created_at, rr.updated_at
		FROM research_runs rr
		LEFT JOIN research_answers ra ON ra.run_id = rr.id
		LEFT JOIN research_evidence re ON re.run_id = rr.id
		LEFT JOIN research_graph_paths rgp ON rgp.run_id = rr.id
		WHERE rr.question_id = $1
		GROUP BY rr.id, ra.answer
		ORDER BY rr.created_at DESC
		LIMIT 100
	`, questionID, includeAnswer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ResearchRunSummary, 0)
	for rows.Next() {
		item, scanErr := scanResearchRunSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func runContexts(ctx context.Context, executor historyExecutor, runID uuid.UUID) ([]ResearchRunContext, error) {
	rows, err := executor.Query(ctx, `SELECT scope_type, scope_id, role FROM research_run_contexts WHERE run_id = $1 ORDER BY scope_type, role, scope_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ResearchRunContext, 0)
	for rows.Next() {
		var item ResearchRunContext
		var scopeID pgtype.UUID
		if err := rows.Scan(&item.ScopeType, &scopeID, &item.Role); err != nil {
			return nil, err
		}
		item.ScopeID = uuidText(scopeID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func runGraphPaths(ctx context.Context, executor historyExecutor, runID uuid.UUID) ([]GraphPath, error) {
	rows, err := executor.Query(ctx, `
		SELECT id, operation, status, explanation, depth, truncated, evidence_backed, structural_only,
		       COALESCE(tree_id::text, ''), COALESCE(tree_version_id::text, ''), COALESCE(tree_version_number, 0), COALESCE(tree_version_state, ''), COALESCE(algorithm_version, ''), nodes
		FROM research_graph_paths
		WHERE run_id = $1
		ORDER BY created_at, id
		LIMIT 5
	`, runID)
	if err != nil {
		return nil, err
	}
	paths := make([]GraphPath, 0)
	for rows.Next() {
		var path GraphPath
		var id pgtype.UUID
		var treeID, treeVersionID, versionState, algorithmVersion string
		var versionNumber, depth int32
		var nodes []byte
		if err := rows.Scan(&id, &path.Operation, &path.Status, &path.Explanation, &depth, &path.Truncated, &path.EvidenceBacked, &path.StructuralOnly, &treeID, &treeVersionID, &versionNumber, &versionState, &algorithmVersion, &nodes); err != nil {
			rows.Close()
			return nil, err
		}
		path.ID = uuidText(id)
		path.Depth = int(depth)
		path.TreeScope = GraphTreeScope{TreeID: treeID, TreeVersionID: treeVersionID, VersionNumber: int(versionNumber), VersionState: versionState}
		path.AlgorithmVersion = algorithmVersion
		if err := json.Unmarshal(nodes, &path.Nodes); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range paths {
		edges, edgeErr := runGraphEdges(ctx, executor, runID, optionalUUID(paths[index].ID))
		if edgeErr != nil {
			return nil, edgeErr
		}
		evidence, evidenceErr := runGraphEvidence(ctx, executor, runID, optionalUUID(paths[index].ID))
		if evidenceErr != nil {
			return nil, evidenceErr
		}
		paths[index].Edges = edges
		paths[index].EvidenceRefs = evidence
	}
	return paths, nil
}

func runGraphEdges(ctx context.Context, executor historyExecutor, runID, pathID uuid.UUID) ([]GraphEdge, error) {
	rows, err := executor.Query(ctx, `
		SELECT id, edge_reference_id, edge_type, from_node_id, to_node_id, path_from_node_id, path_to_node_id, COALESCE(predicate, ''), COALESCE(status, ''), COALESCE(certainty, ''),
		       COALESCE(source_id::text, ''), COALESCE(claim_id::text, ''), COALESCE(statement_id::text, ''), COALESCE(passage_id::text, ''),
		       COALESCE(tree_relationship_id::text, ''), COALESCE(migration_event_id::text, ''), COALESCE(from_place_id::text, ''), COALESCE(to_place_id::text, ''), ordinal
		FROM research_graph_edges
		WHERE run_id = $1 AND path_id = $2
		ORDER BY ordinal
		LIMIT 200
	`, runID, pathID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]GraphEdge, 0)
	for rows.Next() {
		var item GraphEdge
		var id, referenceID, pathFromNodeID, pathToNodeID pgtype.UUID
		var ordinal int32
		if err := rows.Scan(&id, &referenceID, &item.Type, &item.FromNodeID, &item.ToNodeID, &pathFromNodeID, &pathToNodeID, &item.Predicate, &item.Status, &item.Certainty, &item.SourceID, &item.ClaimID, &item.StatementID, &item.PassageID, &item.TreeRelationshipID, &item.MigrationEventID, &item.FromPlaceID, &item.ToPlaceID, &ordinal); err != nil {
			return nil, err
		}
		item.ID = uuidText(referenceID)
		if item.ID == "" {
			item.ID = uuidText(id)
		}
		item.PathFromNodeID = uuidText(pathFromNodeID)
		item.PathToNodeID = uuidText(pathToNodeID)
		item.Position = int(ordinal - 1)
		items = append(items, item)
	}
	return items, rows.Err()
}

func runGraphEvidence(ctx context.Context, executor historyExecutor, runID, pathID uuid.UUID) ([]GraphEvidenceRef, error) {
	rows, err := executor.Query(ctx, `
		SELECT reference_id, reference_type, COALESCE(metadata->>'layer', ''), COALESCE(relation, ''), COALESCE(source_id::text, ''), COALESCE(claim_id::text, ''), COALESCE(statement_id::text, ''), COALESCE(passage_id::text, ''),
		       COALESCE(review_status, ''), COALESCE(status, ''), COALESCE(certainty, ''), COALESCE(title_ar, ''), COALESCE(excerpt_ar, ''), COALESCE(locator_ar, ''), page_number
		FROM research_graph_path_evidence
		WHERE run_id = $1 AND path_id = $2
		ORDER BY ordinal
		LIMIT 200
	`, runID, pathID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]GraphEvidenceRef, 0)
	for rows.Next() {
		var item GraphEvidenceRef
		var pageNumber pgtype.Int4
		if err := rows.Scan(&item.ID, &item.Type, &item.Layer, &item.Relation, &item.SourceID, &item.ClaimID, &item.StatementID, &item.PassageID, &item.ReviewStatus, &item.Status, &item.Certainty, &item.Title, &item.Excerpt, &item.LocatorAR, &pageNumber); err != nil {
			return nil, err
		}
		if pageNumber.Valid {
			value := int(pageNumber.Int32)
			item.PageNumber = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanResearchRunSummary(rows pgx.Rows) (ResearchRunSummary, error) {
	var item ResearchRunSummary
	var id, questionID pgtype.UUID
	var graphOperation, modelVersion, route, runError pgtype.Text
	var graphMaxDepth pgtype.Int4
	var graphTruncated pgtype.Bool
	if err := rows.Scan(&id, &questionID, &item.Query, &item.Status, &item.InsufficientEvidence, &item.CitationCount, &item.GraphPathCount, &graphOperation, &graphMaxDepth, &graphTruncated, &item.AnswerAR, &modelVersion, &route, &item.SynthesisAttempted, &runError, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ResearchRunSummary{}, err
	}
	item.ID = uuidText(id)
	item.QuestionID = uuidText(questionID)
	item.GraphOperation = textValue(graphOperation)
	if graphMaxDepth.Valid {
		item.GraphMaxDepth = int(graphMaxDepth.Int32)
	}
	item.GraphTruncated = graphTruncated.Valid && graphTruncated.Bool
	item.ModelVersion = textValue(modelVersion)
	item.Route = textValue(route)
	item.Error = textValue(runError)
	return item, nil
}

func scanRunSummary(row pgx.Row) (ResearchRunSummary, error) {
	var item ResearchRunSummary
	var id, questionID pgtype.UUID
	var graphOperation, modelVersion, route, runError pgtype.Text
	var graphMaxDepth pgtype.Int4
	var graphTruncated pgtype.Bool
	if err := row.Scan(&id, &questionID, &item.Query, &item.Status, &item.InsufficientEvidence, &item.CitationCount, &item.GraphPathCount, &graphOperation, &graphMaxDepth, &graphTruncated, &item.AnswerAR, &modelVersion, &route, &item.SynthesisAttempted, &runError, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ResearchRunSummary{}, err
	}
	item.ID = uuidText(id)
	item.QuestionID = uuidText(questionID)
	item.GraphOperation = textValue(graphOperation)
	if graphMaxDepth.Valid {
		item.GraphMaxDepth = int(graphMaxDepth.Int32)
	}
	item.GraphTruncated = graphTruncated.Valid && graphTruncated.Bool
	item.ModelVersion = textValue(modelVersion)
	item.Route = textValue(route)
	item.Error = textValue(runError)
	return item, nil
}
