import type { SourceTreeSuggestionCandidateSnapshot, NameLexicon, NamePreparationContext, NameCandidatePreparation } from "../types";
import { demoDashboard, treeNodes } from "../data/demo";
import type {
  AddEvidenceInput,
  AddPersonInput,
  AddRelationshipInput,
  ClaimSummary,
  CollaboratorsResponse,
  CreateClaimInput,
  CreateDisputeInput,
  CreateQuestionInput,
  CreatePassageInput,
  CreateSourceInput,
  CreateStatementInput,
  CreateSourceDependencyInput,
  CreateTreeInput,
  ContradictionFinding,
  ContradictionReviewInput,
  ContradictionRun,
  DisputeClaimInput,
  DisputeDetail,
  DisputeRecord,
  DashboardData,
  DictionaryDetail,
  DictionaryIndexResponse,
  DictionaryKind,
  EntityResolutionCandidate,
  EntityResolutionEntityType,
  EntityResolutionMerge,
  EntityResolutionMergeInput,
  EntityResolutionReviewInput,
  EntityResolutionRun,
  EnqueueJobInput,
  ForkTreeInput,
  InvitationCreated,
  JobView,
  MapResponse,
  PlaceEvidenceResponse,
  TribeEvidenceResponse,
  PersonMovementResponse,
  PairingListParams,
  PairingListResponse,
  PairingHistoryResponse,
  PairingDecideResponse,
  PlaceExternalIDsResponse,
  OpenQuestionRecord,
  PersonAlias,
  PersonAliasInput,
  PlaceView,
  QuestionClaimInput,
  QuestionDetail,
  QuestionDisputeInput,
  QuestionEntityInput,
  QuestionFindingInput,
  QuestionNoteInput,
  QuestionSourceInput,
  ResearchClaim,
  GraphRelationshipImpactInput,
  GraphRelationshipImpactResult,
  GraphBranchStructureComparisonInput,
  GraphBranchStructureComparisonResult,
  GraphAncestorFrontierInput,
  GraphAncestorFrontierResult,
  GraphSourceDependencyNeighborhoodInput,
  GraphSourceDependencyNeighborhoodResult,
  GraphSourceDependencyCommunitiesInput,
  GraphSourceDependencyCommunitiesResult,
  ResearchQueryInput,
  ResearchQueryResult,
  ResearchRunDetail,
  ResearchRunSummary,
  ResearchWorkspaceInput,
  ResearchWorkspaceSnapshot,
  ReviewSourceDependencyInput,
  ReviewSuggestionInput,
  ReviewGeospatialFindingInput,
  ReviewTemporalFindingInput,
  SearchResponse,
  SourceCandidate,
  SourceTreeDraft,
  SaveSourceTreeDraftInput,
  SourceTreeDraftImportInput,
  SourceTreeDraftImportPreview,
  SourceTreeDraftImportResult,
  SourceTreeDraftRetractionResult,
  ChartAnalysisView,
  ChartAnalysisReviewPayload,
  SourcePageEvidence,
  SourcePageEvidenceIndex,
  SourcePageReviewInput,
  SourcePageReviewRevision,
  BookAssertion,
  BookAssertionInput,
  ReviewBookAssertionInput,
  SourceFileDownload,
  SourceDetail,
  SourceCharacterizationInput,
  SourceCharacterizationRun,
  ReviewSourceCharacterizationInput,
  SourceDependencyGraph,
  SourceFile,
  SourceMetadata,
  SourceProcessing,
  SourceProcessingRun,
  ReextractReviewedPageInput,
  StartTemporalAnalysisInput,
  StartGeospatialIntelligenceInput,
  TemporalAnalysisRun,
  TemporalAnalysisRunQuery,
  GeospatialIntelligenceRun,
  GeospatialFinding,
  ResearchAgentRun,
  ResearchAgentRunQuery,
  ResearchQuestionCandidate,
  ReviewResearchQuestionCandidateInput,
  StartResearchAgentInput,
  TemporalFinding,
  SuggestionRecord,
  SubmitSuggestionInput,
  TreeActivity,
  TreeDetail,
  TreeDiff,
  TreeInvitation,
  TreeSummary,
  UpdateDisputeInput,
  UpdatePermissionInput,
  UpdateQuestionInput,
  UpdateRelationshipInput,
  SourceSuggestionAccuracy,
  SourceTreeSuggestion,
  SourceTreeSuggestionDecision,
  SourceTreeSuggestionDecisionEvidence,
  NameJudgeCandidateInput,
  NameJudgeConfig,
  ReadingCapabilities,
  NameReadingRun,
  LocalNameReading,
  NameJudgeResult,
  NameReviewSummary,
  SourceSuggestionHistory,
  SourceSuggestionMergeInput,
  SourceSuggestionMergeResult,
  SourceSuggestionRegionInput,
  SourceSuggestionRegionPreview,
  SourceSuggestionSplitInput,
  SourceSuggestionSplitResult,
} from "../types";

// A fresh clone has to serve the Atlas on first try. In dev, the documented
// local pairing is web :5173 ↔ core-api :8080 (`.env.example`:
// CORE_API_PORT=8080), so fall back to that when VITE_API_URL is not set —
// without this, every API call goes to the vite origin, which 404s, and the
// whole app, Atlas included, renders its error states and looks broken. In
// production the app never guesses: a wrong guess would silently point every
// reader at somebody else's API, so a production build without VITE_API_URL
// keeps the honest empty base.
const apiBaseUrl = import.meta.env.VITE_API_URL ?? (import.meta.env.DEV ? "http://localhost:8080" : "");

/** The API origin for URL-shaped resources the map layer composes itself
 * (raster tile URLs) — components that add a rasterSource need the origin,
 * never the fetchers' internals. */
export function atlasTileUrlTemplate(plateKey: string): string {
  return `${apiBaseUrl}/api/v1/plates/${encodeURIComponent(plateKey)}/tiles/{z}/{x}/{y}.png`;
}
const demoTreeId = "tree-demo";

export const demoTreeDetail: TreeDetail = {
  tree: {
    id: demoTreeId,
    name: "شجرة بيت العنبر",
    description: "تفسير تجريبي لعلاقات محفوظة.",
    visibility: "public",
    ownerId: "demo-owner",
    updatedAt: new Date().toISOString(),
    latestVersionId: "demo-version-3",
    latestVersionNumber: 3,
    latestState: "published",
    people: demoDashboard.treePreview.people,
    relationships: demoDashboard.treePreview.relationships,
    unresolved: demoDashboard.treePreview.unresolved,
  },
  selectedVersion: {
    id: "demo-version-3",
    number: 3,
    state: "published",
    publicationNote: "النسخة الحالية، مع علاقات غير محسومة.",
    publishedAt: "2026-03-20T00:00:00Z",
    createdAt: "2026-03-20T00:00:00Z",
  },
  permissions: { canEdit: false, canPublish: false, canManageCollaborators: false, permissionLevel: "" },
  versions: [
    {
      id: "demo-version-3",
      number: 3,
      state: "published",
      publicationNote: "النسخة الحالية، مع علاقات غير محسومة.",
      publishedAt: "2026-03-20T00:00:00Z",
      createdAt: "2026-03-20T00:00:00Z",
    },
    {
      id: "demo-version-2",
      number: 2,
      state: "published",
      publicationNote: "مراجعة مصدر.",
      publishedAt: "2026-02-14T00:00:00Z",
      createdAt: "2026-02-14T00:00:00Z",
    },
    {
      id: "demo-version-1",
      number: 1,
      state: "published",
      publicationNote: "النسخة الأولى من التفسير.",
      publishedAt: "2026-01-10T00:00:00Z",
      createdAt: "2026-01-10T00:00:00Z",
    },
  ],
  nodes: treeNodes.map((node) => ({
    id: node.id,
    personId: node.personId,
    displayName: node.name,
    sortOrder: Number(node.id.replace("p-", "")),
    years: node.years,
    role: node.role,
    tone: node.tone,
    sourceCount: node.sourceCount,
    note: node.note,
  })),
  relationships: [
    { id: "r-1", subjectNodeId: "p-1", objectNodeId: "p-2", predicate: "parent_of", status: "interpreted" },
    { id: "r-2", subjectNodeId: "p-2", objectNodeId: "p-3", predicate: "parent_of", status: "unresolved" },
    { id: "r-3", subjectNodeId: "p-2", objectNodeId: "p-5", predicate: "parent_of", status: "disputed" },
    { id: "r-4", subjectNodeId: "p-5", objectNodeId: "p-6", predicate: "parent_of", status: "interpreted" },
  ],
};

export class ApiError extends Error {
  status: number;
  /**
   * The request field the service refused, when it named one. Only some routes send
   * it: the suggestion review sends the JSON path of a malformed change-set field, so
   * the composer can mark that input instead of guessing which one of four typed
   * blocks was wrong. A client that had to guess would be re-implementing the
   * service's rules, so an absent field is the honest answer and the caller decides
   * what to do with it.
   */
  field?: string;
  code?: string;

  constructor(message: string, status: number, field?: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.field = field;
    this.code = code;
  }
}

/**
 * Fetches the dashboard.
 *
 * Two rules, and the second one is the reason this function changed:
 *
 *  1. A same-origin API (or the one the dev server points at) answers like it
 *     answers; the payload's own mode label decides what the shell shows. A
 *     payload missing the mode label is treated as demo rather than as api,
 *     because the safe misreading of an unlabelled payload is the one that
 *     shows the warning.
 *  2. A failure is a FAILURE. It is thrown, not turned
 *     into demo data. The API answers this route with a 503 unless DEMO_MODE is
 *     on, and returning the demo instead would turn a deployment whose dashboard
 *     is unavailable into a deployment showing confident numbers nobody can tell
 *     are invented - which is the exact failure this endpoint's gate exists to
 *     prevent. A caller that wants to show something during an outage shows an
 *     error state, and the state says the data is missing.
 */
export async function fetchDashboard(): Promise<DashboardData> {
  const response = await fetch(`${apiBaseUrl}/api/v1/dashboard`, { signal: AbortSignal.timeout(2500) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as DashboardData;
  // Whatever the API says it is, the client believes it. A payload that arrives
  // without a mode is treated as demo rather than as api, because the safe
  // misreading of an unlabelled payload is the one that shows the warning.
  return { ...payload, mode: payload.mode === "api" ? "api" : "demo" };
}

/**
 * DataHygieneReport is the moderator license dashboard's payload: what it is,
 * plus the license posture of each source and the dependent counts that make
 * the missing licenses actionable rather than decorative.
 */
export interface DataHygieneSource {
  id: string;
  title_ar: string;
  author_ar: string | null;
  source_type: string;
  dependency_status: string;
  has_license: boolean;
  license: string | null;
  license_note: string | null;
  file_count: number;
  passage_count: number;
  association_count: number;
}

export interface DataHygieneReport {
  total_sources: number;
  missing_license_count: number;
  sources: DataHygieneSource[];
  limit: number;
  truncated: boolean;
}

/**
 * The dashboard is a live read of the sources table, gated by the moderator
 * role owned by the service, so its refusals are real: a 403 is the role, not
 * out-of-date data, and a failure is thrown rather than papered over — an
 * unavailable dashboard must not be redeemed by invented numbers.
 */
export async function fetchDataHygieneReport(): Promise<DataHygieneReport> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/data-hygiene`, { credentials: "include", signal: AbortSignal.timeout(6000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DataHygieneReport;
}

export async function queryResearch(input: ResearchQueryInput): Promise<ResearchQueryResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/query`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(20000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchQueryResult;
}

export async function queryGraphRelationshipImpact(input: GraphRelationshipImpactInput): Promise<GraphRelationshipImpactResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/graph/relationship-impact`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as GraphRelationshipImpactResult;
}

export async function queryGraphBranchStructureComparison(input: GraphBranchStructureComparisonInput): Promise<GraphBranchStructureComparisonResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/graph/branch-structure-comparison`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as GraphBranchStructureComparisonResult;
}

export async function queryGraphAncestorFrontier(input: GraphAncestorFrontierInput): Promise<GraphAncestorFrontierResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/graph/ancestor-frontier`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as GraphAncestorFrontierResult;
}

export async function queryGraphSourceDependencyNeighborhood(input: GraphSourceDependencyNeighborhoodInput): Promise<GraphSourceDependencyNeighborhoodResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/graph/source-dependency-neighborhood`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as GraphSourceDependencyNeighborhoodResult;
}

export async function queryGraphSourceDependencyCommunities(input: GraphSourceDependencyCommunitiesInput): Promise<GraphSourceDependencyCommunitiesResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/graph/source-dependency-communities`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as GraphSourceDependencyCommunitiesResult;
}

export async function fetchResearchWorkspace(input: ResearchWorkspaceInput): Promise<ResearchWorkspaceSnapshot> {
  const params = new URLSearchParams();
  if (input.entityType) params.set("entity_type", input.entityType);
  if (input.entityId) params.set("entity_id", input.entityId);
  if (input.treeId) params.set("tree_id", input.treeId);
  if (input.treeVersionId) params.set("tree_version_id", input.treeVersionId);
  const query = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/research/questions/${encodeURIComponent(input.questionId)}/workspace${query ? `?${query}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(6000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchWorkspaceSnapshot;
}

export async function fetchResearchRuns(questionId: string): Promise<ResearchRunSummary[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/questions/${encodeURIComponent(questionId)}/runs`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: ResearchRunSummary[] };
  return payload.items ?? [];
}

export async function fetchResearchRun(runId: string): Promise<ResearchRunDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research/runs/${encodeURIComponent(runId)}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchRunDetail;
}

export async function runEntityResolution(entityType: EntityResolutionEntityType): Promise<EntityResolutionRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/runs`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ entity_type: entityType }),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as EntityResolutionRun;
}

export async function fetchEntityResolutionRun(runId: string): Promise<EntityResolutionRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/runs/${runId}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as EntityResolutionRun;
}

export async function fetchEntityResolutionCandidates(status?: string, entityType?: string): Promise<EntityResolutionCandidate[]> {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  if (entityType) params.set("entity_type", entityType);
  const query = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/candidates${query ? `?${query}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: EntityResolutionCandidate[] };
  return payload.items ?? [];
}

export async function fetchEntityResolutionMerges(): Promise<EntityResolutionMerge[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/merges`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: EntityResolutionMerge[] };
  return payload.items ?? [];
}

export async function reviewEntityResolutionCandidate(candidateId: string, input: EntityResolutionReviewInput): Promise<EntityResolutionCandidate> {
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/candidates/${candidateId}/review`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as EntityResolutionCandidate;
}

export async function mergeEntityResolutionCandidate(candidateId: string, input: EntityResolutionMergeInput): Promise<EntityResolutionMerge> {
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/candidates/${candidateId}/merge`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as EntityResolutionMerge;
}

export async function reverseEntityResolutionMerge(mergeId: string, reasonAr: string): Promise<EntityResolutionMerge> {
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/merges/${mergeId}/reverse`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reason_ar: reasonAr }),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as EntityResolutionMerge;
}

export async function startContradictionRun(): Promise<ContradictionRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/contradictions/runs`, { method: "POST", credentials: "include" });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ContradictionRun;
}

export async function fetchContradictionRun(runId: string): Promise<ContradictionRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/contradictions/runs/${runId}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ContradictionRun;
}

export async function fetchContradictionFindings(runId?: string, status?: string): Promise<ContradictionFinding[]> {
  const params = new URLSearchParams();
  if (runId) params.set("run_id", runId);
  if (status) params.set("status", status);
  const query = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/contradictions/findings${query ? `?${query}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: ContradictionFinding[] };
  return payload.items ?? [];
}

export async function reviewContradictionFinding(findingId: string, input: ContradictionReviewInput): Promise<ContradictionFinding> {
  const response = await fetch(`${apiBaseUrl}/api/v1/contradictions/findings/${findingId}/review`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ContradictionFinding;
}

export async function startTemporalAnalysis(input: StartTemporalAnalysisInput): Promise<TemporalAnalysisRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/temporal-analysis/runs`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TemporalAnalysisRun;
}

export async function fetchTemporalAnalysisRun(runId: string): Promise<TemporalAnalysisRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/temporal-analysis/runs/${runId}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TemporalAnalysisRun;
}

export async function fetchLatestTemporalAnalysisRun(query: TemporalAnalysisRunQuery): Promise<TemporalAnalysisRun | null> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value) params.set(key, value);
  }
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/temporal-analysis/runs/latest${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (response.status === 404) {
    return null;
  }
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TemporalAnalysisRun;
}

export async function fetchTemporalFindings(runId?: string, status?: string): Promise<TemporalFinding[]> {
  const params = new URLSearchParams();
  if (runId) params.set("run_id", runId);
  if (status) params.set("status", status);
  const query = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/temporal-analysis/findings${query ? `?${query}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: TemporalFinding[] };
  return payload.items ?? [];
}

export async function reviewTemporalFinding(findingId: string, input: ReviewTemporalFindingInput): Promise<TemporalFinding> {
  const response = await fetch(`${apiBaseUrl}/api/v1/temporal-analysis/findings/${findingId}/review`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TemporalFinding;
}

export async function fetchPublicTrees(): Promise<TreeSummary[]> {

  try {
    const response = await fetch(`${apiBaseUrl}/api/v1/trees`, {
      credentials: "include",
      signal: AbortSignal.timeout(2500),
    });
    if (!response.ok) {
      throw new ApiError("تعذر تحميل الأشجار", response.status);
    }
    const payload = (await response.json()) as { items?: TreeSummary[] };
    return payload.items ?? [];
  } catch (error) {
    if (error instanceof ApiError) {
      throw error;
    }
    throw new ApiError("تعذر الاتصال بخدمة الأشجار.", 503);
  }
}

async function fetchScopedTrees(scope: "workspace" | "public"): Promise<TreeSummary[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/trees?scope=${scope}`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const payload = (await response.json()) as { items?: TreeSummary[] };
  return payload.items ?? [];
}

export const fetchWorkspaceTrees = () => fetchScopedTrees("workspace");
export const fetchPublishedTrees = () => fetchScopedTrees("public");

export async function fetchChartLibrary(): Promise<import("../types").ChartLibraryItem[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/chart-library`, { credentials: "include", signal: AbortSignal.timeout(10000), cache: "no-store" });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const payload = await response.json() as { items?: import("../types").ChartLibraryItem[] };
  return payload.items ?? [];
}

export async function fetchTree(treeId: string): Promise<TreeDetail> {
  if (treeId === demoTreeId) {
    return demoTreeDetail;
  }

  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}`, {
    credentials: "include",
    signal: AbortSignal.timeout(3000),
  });
  if (!response.ok) {
    const message = await readErrorMessage(response);
    throw new ApiError(message, response.status);
  }
  return (await response.json()) as TreeDetail;
}

export async function fetchTreeVersion(treeId: string, versionId: string): Promise<TreeDetail> {
  if (treeId === demoTreeId) {
    const version = demoTreeDetail.versions.find((candidate) => candidate.id === versionId);
    if (!version) {
      throw new ApiError("النسخة التجريبية غير موجودة.", 404);
    }
    const nodes = version.number === 1 ? demoTreeDetail.nodes.slice(0, 3) : version.number === 2 ? demoTreeDetail.nodes.slice(0, 4) : demoTreeDetail.nodes;
    const relationships = version.number === 1 ? demoTreeDetail.relationships.slice(0, 2) : version.number === 2 ? demoTreeDetail.relationships.slice(0, 3) : demoTreeDetail.relationships;
    return {
      ...demoTreeDetail,
      tree: { ...demoTreeDetail.tree, people: nodes.length, relationships: relationships.length, unresolved: relationships.filter((relationship) => relationship.status === "unresolved").length },
      selectedVersion: version,
      permissions: { canEdit: false, canPublish: false, canManageCollaborators: false, permissionLevel: "" },
      nodes,
      relationships,
    };
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/versions/${versionId}`, {
    credentials: "include",
    signal: AbortSignal.timeout(3000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDetail;
}

export async function createTree(input: CreateTreeInput): Promise<TreeDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/trees`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDetail;
}

export async function addPerson(treeId: string, input: AddPersonInput): Promise<TreeDetail> {
  if (!apiBaseUrl || treeId === demoTreeId) {
    throw new ApiError("شغّل Core API وافتح مسودة حقيقية لتعديلها.", 409);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/people`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDetail;
}

/**
 * listPersonAliases reads the names a person is also known by, through the same
 * identity API that records them. The endpoint is gated on the identity role and
 * scopes every alias by the source it was taken from, exactly as the dictionary
 * page does, so the workspace never shows a spelling the public page would hide.
 */
export async function listPersonAliases(personId: string): Promise<PersonAlias[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/people/${personId}/aliases`, { credentials: "include" });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: PersonAlias[] };
  return payload.items ?? [];
}

/**
 * addPersonAlias records another name for a person. The API refuses it for a
 * person a published tree version already rests on, and refuses it for an account
 * without the identity role, so both refusals arrive here as an ApiError with the
 * server's own wording rather than as a silent no-op.
 */
export async function addPersonAlias(personId: string, input: PersonAliasInput): Promise<PersonAlias> {
  const response = await fetch(`${apiBaseUrl}/api/v1/people/${personId}/aliases`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PersonAlias;
}

export async function deletePersonAlias(aliasId: string, reasonAr: string): Promise<void> {
  const response = await fetch(`${apiBaseUrl}/api/v1/person-aliases/${aliasId}`, {
    method: "DELETE",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reason_ar: reasonAr }),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
}

export async function addRelationship(treeId: string, input: AddRelationshipInput): Promise<TreeDetail> {
  if (!apiBaseUrl || treeId === demoTreeId) {
    throw new ApiError("شغّل Core API وافتح مسودة حقيقية لتعديلها.", 409);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/relationships`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDetail;
}

export async function updateRelationship(treeId: string, relationshipId: string, input: UpdateRelationshipInput): Promise<TreeDetail> {
  if (!apiBaseUrl || treeId === demoTreeId) {
    throw new ApiError("هذه النسخة للقراءة فقط ولا تقبل تعديل الحالة.", 409);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/relationships/${relationshipId}`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDetail;
}

export async function publishTree(treeId: string, note: string): Promise<TreeDetail> {
  if (!apiBaseUrl || treeId === demoTreeId) {
    throw new ApiError("أنشئ شجرة محفوظة 먼저 لتجربة النشر.", 409);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/publish`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ note }),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDetail;
}

export async function forkTree(treeId: string, input: ForkTreeInput): Promise<TreeDetail> {
  requireRealTree(treeId);
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/fork`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDetail;
}

export async function fetchTreeDiff(treeId: string, params: { fromTreeId: string; fromVersionId: string; toVersionId: string }): Promise<TreeDiff> {
  requireRealTree(treeId);
  const search = new URLSearchParams({
    from_tree_id: params.fromTreeId,
    from_version_id: params.fromVersionId,
    to_version_id: params.toVersionId,
  });
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/diff?${search.toString()}`, {
    credentials: "include",
    signal: AbortSignal.timeout(4000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TreeDiff;
}

export async function fetchJobs(status?: string): Promise<JobView[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs${query}`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: JobView[] };
  return payload.items ?? [];
}

export async function enqueueJob(input: EnqueueJobInput): Promise<{ job: JobView; created: boolean }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { job: JobView; created: boolean };
}

export async function claimJob(workerId: string): Promise<JobView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/claim`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ worker_id: workerId }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as JobView;
}

export async function completeJob(jobId: string, workerId: string): Promise<JobView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/${jobId}/complete`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ worker_id: workerId }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as JobView;
}

export async function failJob(jobId: string, workerId: string, error: string): Promise<JobView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/${jobId}/fail`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ worker_id: workerId, error }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as JobView;
}

export async function recoverStaleJobs(olderThanSeconds = 900): Promise<{ recovered: number; jobs: JobView[] }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/recover-stale`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ older_than_seconds: olderThanSeconds }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as { recovered: number; jobs: JobView[] };
}

export async function fetchSearch(filters: { q: string; kind?: string; status?: string; personId?: string; placeId?: string; sourceId?: string; entityId?: string; fromYear?: number; toYear?: number; limit?: number; signal?: AbortSignal }): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: filters.q });
  if (filters.kind) params.set("kind", filters.kind);
  if (filters.status) params.set("status", filters.status);
  if (filters.personId) params.set("person_id", filters.personId);
  if (filters.placeId) params.set("place_id", filters.placeId);
  if (filters.sourceId) params.set("source_id", filters.sourceId);
  if (filters.entityId) params.set("entity_id", filters.entityId);
  if (filters.fromYear !== undefined) params.set("from_year", String(filters.fromYear));
  if (filters.toYear !== undefined) params.set("to_year", String(filters.toYear));
  if (filters.limit !== undefined) params.set("limit", String(filters.limit));
  const timeoutSignal = AbortSignal.timeout(filters.kind === "semantic" ? 12000 : 5000);
  const signal = filters.signal ? AbortSignal.any([timeoutSignal, filters.signal]) : timeoutSignal;
  const response = await fetch(`${apiBaseUrl}/api/v1/search?${params.toString()}`, { signal });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SearchResponse;
}

export interface MapFetchFilters {
  fromYear?: number;
  toYear?: number;
  status?: string;
  placeId?: string;
  viewport?: { minLongitude: number; minLatitude: number; maxLongitude: number; maxLatitude: number };
  movementMode?: string;
  season?: string;
  limit?: number;
  signal?: AbortSignal;
}

export async function fetchMapFeatures(filters: MapFetchFilters = {}): Promise<MapResponse> {
  const params = new URLSearchParams();
  if (filters.fromYear !== undefined) params.set("from_year", String(filters.fromYear));
  if (filters.toYear !== undefined) params.set("to_year", String(filters.toYear));
  if (filters.status) params.set("status", filters.status);
  if (filters.movementMode) params.set("movementMode", filters.movementMode);
  if (filters.season) params.set("season", filters.season);
  if (filters.placeId) params.set("place_id", filters.placeId);
  if (filters.limit !== undefined) params.set("limit", String(filters.limit));
  if (filters.viewport) {
    const { minLongitude, minLatitude, maxLongitude, maxLatitude } = filters.viewport;
    params.set("min_longitude", String(minLongitude));
    params.set("min_latitude", String(minLatitude));
    params.set("max_longitude", String(maxLongitude));
    params.set("max_latitude", String(maxLatitude));
  }
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const timeoutSignal = AbortSignal.timeout(5000);
  const signal = filters.signal ? AbortSignal.any([timeoutSignal, filters.signal]) : timeoutSignal;
  // credentials: a signed-in researcher's private places are on the map for them
  // and for nobody else, and that read needs the session cookie. Without it the
  // atlas would show the anonymous map even to the person who wrote the place.
  const response = await fetch(`${apiBaseUrl}/api/v1/map${suffix}`, { credentials: "include", signal });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as MapResponse;
}

export interface PlaceEvidenceOptions {
  /**
   * Ask for the moderation lane (`needs_review` rows alongside accepted ones).
   * The server re-checks the moderator role and answers 403 to anybody else,
   * so this flag is a request, not authority.
   */
  includeNeedsReview?: boolean;
  signal?: AbortSignal;
}

/**
 * One place's evidence dossier: the passages citing that place, each with its
 * source and review state, bounded server-side. Credentials are carried for
 * the same reason the map fetch does — the dossier is a policy answer, and a
 * signed-in reader must get their own answer, not the anonymous one.
 */
export async function fetchPlaceEvidence(placeID: string, options: PlaceEvidenceOptions = {}): Promise<PlaceEvidenceResponse> {
  const params = new URLSearchParams();
  if (options.includeNeedsReview) params.set("include", "needs_review");
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const timeoutSignal = AbortSignal.timeout(8000);
  const signal = options.signal ? AbortSignal.any([timeoutSignal, options.signal]) : timeoutSignal;
  const response = await fetch(`${apiBaseUrl}/api/v1/places/${encodeURIComponent(placeID)}/evidence${suffix}`, {
    credentials: "include",
    signal,
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PlaceEvidenceResponse;
}

/**
 * One person's documented مسالِك for the atlas subject layer (plan 028-D).
 * Credentials are carried: the layer is a policy answer, and a private
 * source's segments are invisible to everyone but its readers.
 */
export async function fetchPersonMovement(personID: string, signal?: AbortSignal): Promise<PersonMovementResponse> {  const timeoutSignal = AbortSignal.timeout(8000);
  const effectiveSignal = signal ? AbortSignal.any([timeoutSignal, signal]) : timeoutSignal;
  const response = await fetch(`${apiBaseUrl}/api/v1/persons/${encodeURIComponent(personID)}/movement`, {
    credentials: "include",
    signal: effectiveSignal,
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PersonMovementResponse;
}

/**
 * One tribe's receipt folio (plan 031-A): the accepted spatial/claim receipts
 * behind the tribe's dossier, bounded server-side. Credentials are carried:
 * the folio is a policy answer (a private source is not laundered through a
 * public tribe), and includeNeedsReview re-checks the moderator role at the
 * server, so the flag is a request, not authority.
 */
/**
 * The record-hints lane (plan 031-C): the ingest lanes' tribe-mention
 * suggestions as (tribe × source) cards in a bounded queue. A hint never
 * auto-applies; the accept = a named reviewer's documented receipt, the
 * dismissal hides the card for the dismissing reviewer only. The lane is
 * moderator-readable and the server re-checks the role.
 */
export interface TribeHintCard {
  tribeId: string;
  tribeName: string;
  sourceId: string;
  sourceTitle: string;
  passageCount: number;
  mentions: number;
  status: "suggested" | "accepted";
  decidedBy?: string;
  createdAt?: string;
}

export interface TribeHintListResponse {
  hints: TribeHintCard[];
  tribeId?: string;
  status: string;
  limit: number;
  truncated: boolean;
}

export interface TribeHintMatchResult {
  sourceId: string;
  hintsSuggested: number;
  matchedTribes: number;
  perTribe: Array<{ tribeId: string; tribeName: string; mentions: number }>;
}

export async function fetchTribeHints(tribeId: string, status = "suggested", signal?: AbortSignal): Promise<TribeHintListResponse> {
  const search = new URLSearchParams();
  search.set("tribe_id", tribeId);
  search.set("status", status);
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/tribe-hints?${search.toString()}`, {
    credentials: "include",
    signal: AbortSignal.any([AbortSignal.timeout(8000), signal ?? AbortSignal.timeout(0)]),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TribeHintListResponse;
}

export async function runTribeHintsMatching(sourceId: string): Promise<TribeHintMatchResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/tribe-hints/run`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sourceId }),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TribeHintMatchResult;
}

export async function decideTribeHint(tribeId: string, sourceId: string, action: "accept" | "dismiss", noteAr = ""): Promise<{ card: TribeHintCard; action: string; createdClaimId?: string; createdReceipts: number }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/tribe-hints/tribes/${encodeURIComponent(tribeId)}/sources/${encodeURIComponent(sourceId)}/decide`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action, noteAr }),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { card: TribeHintCard; action: string; createdClaimId?: string; createdReceipts: number };
}

/**
 * المِسلك — the tribe-duplicate lane (plan 031-E): the measured pair
 * proposals with the matched forms verbatim; the reviewer's three effects
 * (link-alias / canonical pointer / reject) touch no `tribes` row — the undo
 * is data, not surgery. Moderator-readable, role-checked at the server.
 */
export interface TribeDuplicateCandidate {
  id: string;
  tribeAId: string;
  tribeAName?: string;
  tribeBId: string;
  tribeBName?: string;
  matchedA: string;
  matchedB: string;
  nameSimilarity: number;
  status: "suggested" | "linked" | "pointed" | "rejected";
  decidedBy?: string;
  decisionNoteAr?: string;
  createdAt: string;
  canonical?: string;
}

export interface TribeDuplicateListResponse {
  candidates: TribeDuplicateCandidate[];
  status: string;
  limit: number;
  truncated: boolean;
}

export async function fetchTribeDuplicateCandidates(status = "suggested", signal?: AbortSignal): Promise<TribeDuplicateListResponse> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/tribe-pairings?status=${encodeURIComponent(status)}`, {
    credentials: "include",
    signal: AbortSignal.any([AbortSignal.timeout(8000), signal ?? AbortSignal.timeout(0)]),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TribeDuplicateListResponse;
}

export async function decideTribeDuplicate(candidateID: string, input: { action: "link_alias" | "canonize" | "reject"; canonical?: string; noteAr?: string }): Promise<{ candidate: TribeDuplicateCandidate; action: string }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/tribe-pairings/${encodeURIComponent(candidateID)}/decide`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { candidate: TribeDuplicateCandidate; action: string };
}

export async function fetchTribeEvidence(tribeID: string, options: PlaceEvidenceOptions = {}): Promise<TribeEvidenceResponse> {
  const params = new URLSearchParams();
  if (options.includeNeedsReview) params.set("include", "needs_review");
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const timeoutSignal = AbortSignal.timeout(8000);
  const signal = options.signal ? AbortSignal.any([timeoutSignal, options.signal]) : timeoutSignal;
  const response = await fetch(`${apiBaseUrl}/api/v1/tribes/${encodeURIComponent(tribeID)}/evidence${suffix}`, {
    credentials: "include",
    signal,
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TribeEvidenceResponse;
}

/**
 * The pairing lane's proposal list (plan 028-F). Moderator-only; a 403 is a
 * role answer and is surfaced as such by the lane page. The optional `query`
 * is a text scan across keys, readings, occurrence ids, source titles and our
 * canonical place names — the fields a moderator actually scans for.
 */
export async function fetchPairingCandidates(params: PairingListParams = {}): Promise<PairingListResponse> {
  const search = new URLSearchParams();
  search.set("status", params.status ?? "suggested");
  if (params.query?.trim()) search.set("q", params.query.trim());
  if (params.placeId?.trim()) search.set("place_id", params.placeId.trim());
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/place-pairings?${search.toString()}`, {
    credentials: "include",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PairingListResponse;
}

/** A moderator's explicit decision for one candidate. */
export async function decidePairingCandidate(candidateID: string, effect: "alias" | "link" | "reject" | "defer", noteAr = ""): Promise<PairingDecideResponse> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/place-pairings/${encodeURIComponent(candidateID)}/decide`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ effect, noteAr }),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PairingDecideResponse;
}

/**
 * Reopens a no-effect decision (`rejected` or `deferred`) back to `suggested`.
 * Fact-producing decisions (`accepted`, `alias_only`) deliberately cannot be
 * reopened this way, because their alias/crosswalk rows would survive the
 * reopen and the lane would no longer describe reality.
 */
export async function reopenPairingCandidate(candidateID: string, noteAr = ""): Promise<PairingDecideResponse> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/place-pairings/${encodeURIComponent(candidateID)}/reopen`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ noteAr }),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PairingDecideResponse;
}

/** The append-only decision trail for one candidate. Moderator-only. */
export async function fetchPairingHistory(candidateID: string, signal?: AbortSignal): Promise<PairingHistoryResponse> {
  const timeoutSignal = AbortSignal.timeout(8000);
  const effectiveSignal = signal ? AbortSignal.any([timeoutSignal, signal]) : timeoutSignal;
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/place-pairings/${encodeURIComponent(candidateID)}/history`, {
    credentials: "include",
    signal: effectiveSignal,
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PairingHistoryResponse;
}

/** The accepted crosswalk for one place, policy-filtered by readable sources. */
export async function fetchPlaceExternalIDs(placeID: string, signal?: AbortSignal): Promise<PlaceExternalIDsResponse> {
  const timeoutSignal = AbortSignal.timeout(8000);
  const effectiveSignal = signal ? AbortSignal.any([timeoutSignal, signal]) : timeoutSignal;
  const response = await fetch(`${apiBaseUrl}/api/v1/places/${encodeURIComponent(placeID)}/external-ids`, {
    credentials: "include",
    signal: effectiveSignal,
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PlaceExternalIDsResponse;
}

export interface CreatePlaceRequest {
  canonical_name_ar: string;
  place_type: string;
  notes_ar?: string;
  longitude?: number;
  latitude?: number;
}

export async function fetchPlaceIndex(query = "", signal?: AbortSignal): Promise<{ kind: string; query?: string; items: PlaceView[] }> {
  const params = new URLSearchParams();
  if (query.trim()) params.set("q", query.trim());
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/places${suffix}`, { credentials: "include", signal });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { kind: string; query?: string; items: PlaceView[] };
}

/**
 * Creates a place through the identity endpoint. The backend stores a new place
 * as private, so the atlas must refetch with the session to see it; it is not
 * silently promoted to the public map.
 */
export async function createPlace(input: CreatePlaceRequest): Promise<PlaceView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/places`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as PlaceView;
}

export async function startGeospatialIntelligence(input: StartGeospatialIntelligenceInput): Promise<GeospatialIntelligenceRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/geospatial-intelligence/runs`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as GeospatialIntelligenceRun;
}

export async function fetchLatestGeospatialIntelligenceRun(query: { question_id?: string; entity_type?: string; entity_id?: string; tree_version_id?: string }): Promise<GeospatialIntelligenceRun | null> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) if (value) params.set(key, value);
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/geospatial-intelligence/runs/latest${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (response.status === 404) return null;
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as GeospatialIntelligenceRun;
}

export async function fetchGeospatialFindings(runId?: string, status?: string): Promise<GeospatialFinding[]> {
  const params = new URLSearchParams();
  if (runId) params.set("run_id", runId);
  if (status) params.set("status", status);
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/geospatial-intelligence/findings${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const payload = (await response.json()) as { items?: GeospatialFinding[] };
  return payload.items ?? [];
}

export async function reviewGeospatialFinding(findingId: string, input: ReviewGeospatialFindingInput): Promise<GeospatialFinding> {
  const response = await fetch(`${apiBaseUrl}/api/v1/geospatial-intelligence/findings/${findingId}/review`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as GeospatialFinding;
}

export async function startResearchAgent(input: StartResearchAgentInput): Promise<ResearchAgentRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ResearchAgentRun;
}

export async function fetchResearchAgentRun(runId: string): Promise<ResearchAgentRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/${encodeURIComponent(runId)}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ResearchAgentRun;
}

export async function fetchLatestResearchAgentRun(query: ResearchAgentRunQuery): Promise<ResearchAgentRun | null> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) if (value) params.set(key, value);
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/latest${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (response.status === 404) return null;
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ResearchAgentRun;
}

export async function generateResearchQuestionCandidates(runId: string): Promise<ResearchQuestionCandidate[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/${encodeURIComponent(runId)}/question-candidates`, { method: "POST", credentials: "include", signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const payload = (await response.json()) as { items?: ResearchQuestionCandidate[] };
  return payload.items ?? [];
}

export async function fetchResearchQuestionCandidates(runId: string, status?: string): Promise<ResearchQuestionCandidate[]> {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/${encodeURIComponent(runId)}/question-candidates${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const payload = (await response.json()) as { items?: ResearchQuestionCandidate[] };
  return payload.items ?? [];
}

export async function reviewResearchQuestionCandidate(candidateId: string, input: ReviewResearchQuestionCandidateInput): Promise<ResearchQuestionCandidate> {
  const response = await fetch(`${apiBaseUrl}/api/v1/research-question-candidates/${encodeURIComponent(candidateId)}`, { method: "PATCH", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ResearchQuestionCandidate;
}

export async function fetchDictionaryIndex(kind: DictionaryKind, query = ""): Promise<DictionaryIndexResponse> {
  const params = new URLSearchParams({ kind, q: query });
  const response = await fetch(`${apiBaseUrl}/api/v1/dictionary?${params.toString()}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DictionaryIndexResponse;
}

export async function fetchDictionaryDetail(kind: "families" | "tribes" | "people" | "places", id: string): Promise<DictionaryDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/dictionary/${kind}/${id}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DictionaryDetail;
}

export async function submitSuggestion(input: SubmitSuggestionInput): Promise<SuggestionRecord> {
  const response = await fetch(`${apiBaseUrl}/api/v1/suggestions`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SuggestionRecord;
}

export async function fetchSuggestions(treeId: string): Promise<SuggestionRecord[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/suggestions`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: SuggestionRecord[] };
  return payload.items ?? [];
}

export async function reviewSuggestion(suggestionId: string, input: ReviewSuggestionInput): Promise<SuggestionRecord> {
  const response = await fetch(`${apiBaseUrl}/api/v1/suggestions/${suggestionId}`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    // The one route that can name the field it refused, so a malformed change set is
    // marked on the input the reviewer typed rather than reported as a bare failure.
    throw await readApiError(response);
  }
  return (await response.json()) as SuggestionRecord;
}

export async function fetchQuestions(): Promise<OpenQuestionRecord[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: OpenQuestionRecord[] };
  return payload.items ?? [];
}

export async function fetchQuestion(questionId: string): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function createQuestion(input: CreateQuestionInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function updateQuestion(questionId: string, input: UpdateQuestionInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function addQuestionNote(questionId: string, input: QuestionNoteInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}/notes`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function linkQuestionClaim(questionId: string, input: QuestionClaimInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}/claims`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function linkQuestionSource(questionId: string, input: QuestionSourceInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}/sources`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function linkQuestionDispute(questionId: string, input: QuestionDisputeInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}/disputes`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function linkQuestionEntity(questionId: string, input: QuestionEntityInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}/entities`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function linkQuestionFinding(questionId: string, input: QuestionFindingInput): Promise<QuestionDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}/findings`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function fetchDisputes(): Promise<DisputeRecord[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/disputes`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: DisputeRecord[] };
  return payload.items ?? [];
}

export async function createDispute(input: CreateDisputeInput): Promise<DisputeDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/disputes`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DisputeDetail;
}

export async function updateDispute(disputeId: string, input: UpdateDisputeInput): Promise<DisputeDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/disputes/${disputeId}`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DisputeDetail;
}

/**
 * The dossier claim's طعن (plan 031-D): opens a dispute thread against one
 * readable claim, and the claim's own row flips to `disputed` inside the
 * challenge's transaction — every surface reading the claim's chip shows the
 * dispute the moment it commits. Suggestion-free: the thread is what the
 * adjudication answers.
 */
export async function challengeClaim(claimID: string, input: { titleAr: string; descriptionAr?: string }): Promise<DisputeDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/claims/${encodeURIComponent(claimID)}/challenge`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DisputeDetail;
}

/**
 * Closes a challenged dispute (plan 031-D): the moderator's explicit outcome —
 * documented keeps the claim, rejected retires it — with the resolution's own
 * words required. The server re-checks the role; the challenge's author cannot
 * close their own thread.
 */
export async function adjudicateChallenge(disputeID: string, input: { claimStatus: "documented" | "rejected"; resolutionAr: string }): Promise<DisputeDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/disputes/${encodeURIComponent(disputeID)}/adjudicate`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DisputeDetail;
}

export async function linkDisputeClaim(disputeId: string, input: DisputeClaimInput): Promise<DisputeDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/disputes/${disputeId}/claims`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DisputeDetail;
}

export async function fetchSources(): Promise<SourceMetadata[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: SourceMetadata[] };
  return payload.items ?? [];
}

export async function fetchSource(sourceId: string): Promise<SourceDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDetail;
}

export async function fetchSourceDependencies(sourceId: string): Promise<SourceDependencyGraph> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/dependencies`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDependencyGraph;
}

export async function createSourceDependency(sourceId: string, input: CreateSourceDependencyInput): Promise<SourceDependencyGraph> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/dependencies`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDependencyGraph;
}

export async function detectSourceDependencies(sourceId: string): Promise<SourceDependencyGraph> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/dependencies/detect`, {
    method: "POST",
    credentials: "include",
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDependencyGraph;
}

export async function reviewSourceDependency(dependencyId: string, input: ReviewSourceDependencyInput): Promise<SourceDependencyGraph> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-dependencies/${dependencyId}/review`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDependencyGraph;
}

export async function startSourceCharacterization(input: SourceCharacterizationInput): Promise<SourceCharacterizationRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-characterization/runs`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceCharacterizationRun;
}

export async function fetchSourceCharacterizationRun(runId: string): Promise<SourceCharacterizationRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-characterization/runs/${encodeURIComponent(runId)}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceCharacterizationRun;
}

export async function fetchLatestSourceCharacterization(input: SourceCharacterizationInput): Promise<SourceCharacterizationRun | null> {
  const params = new URLSearchParams({ source_id: input.source_id });
  if (input.question_id) params.set("question_id", input.question_id);
  if (input.claim_id) params.set("claim_id", input.claim_id);
  if (input.place_id) params.set("place_id", input.place_id);
  const response = await fetch(`${apiBaseUrl}/api/v1/source-characterization/runs/latest?${params.toString()}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (response.status === 404) return null;
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceCharacterizationRun;
}

export async function reviewSourceCharacterization(runId: string, input: ReviewSourceCharacterizationInput): Promise<SourceCharacterizationRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-characterization/runs/${encodeURIComponent(runId)}/review`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceCharacterizationRun;
}

export async function fetchSourceProcessing(sourceId: string): Promise<SourceProcessing> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/processing`, {
    credentials: "include",
    signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceProcessing;
}

export async function fetchSourcePageEvidence(fileId: string, pageNumber: number): Promise<SourcePageEvidence> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/pages/${pageNumber}`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourcePageEvidence;
}

export async function reviewSourcePage(fileId: string, pageNumber: number, input: SourcePageReviewInput): Promise<SourcePageReviewRevision> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/pages/${pageNumber}/review`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourcePageReviewRevision;
}

export async function reprocessSourceFile(fileId: string, renderDpi: 300 | 400 = 400): Promise<SourceProcessingRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/reprocess`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ renderDpi }),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceProcessingRun;
}

export async function reextractReviewedPage(fileId: string, pageNumber: number, input: ReextractReviewedPageInput): Promise<SourceProcessingRun> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/pages/${pageNumber}/reextract-reviewed`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceProcessingRun;
}

export async function fetchSourcePageEvidenceIndex(fileId: string): Promise<SourcePageEvidenceIndex> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/pages`, {
    credentials: "include",
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourcePageEvidenceIndex;
}

export async function fetchSourcePagePreview(fileId: string, pageNumber: number): Promise<Blob> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/pages/${pageNumber}/preview`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return response.blob();
}

export async function startSourceChartAnalysis(fileId: string): Promise<ChartAnalysisView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/chart-analysis`, {
    method: "POST", credentials: "include", cache: "no-store", signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ChartAnalysisView;
}

export async function fetchSourceChartAnalysis(fileId: string): Promise<ChartAnalysisView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/chart-analysis`, {
    credentials: "include", cache: "no-store", signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ChartAnalysisView;
}

export async function retrySourceChartAnalysis(runId: string): Promise<ChartAnalysisView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-chart-analysis-runs/${encodeURIComponent(runId)}/retry`, {
    method: "POST", credentials: "include", cache: "no-store", signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ChartAnalysisView;
}

export async function saveSourceChartAnalysisReview(runId: string, input: ChartAnalysisReviewPayload): Promise<ChartAnalysisView> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-chart-analysis-runs/${encodeURIComponent(runId)}/review`, {
    method: "PUT", credentials: "include", cache: "no-store", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input), signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ChartAnalysisView;
}

export async function createSourceTreeDraft(fileId: string, titleAr?: string): Promise<SourceTreeDraft> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/tree-drafts`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ titleAr: titleAr ?? "" }),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceTreeDraft;
}

export async function fetchSourceTreeDraft(draftId: string): Promise<SourceTreeDraft> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-drafts/${encodeURIComponent(draftId)}`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceTreeDraft;
}

export async function saveSourceTreeDraft(draftId: string, input: SaveSourceTreeDraftInput): Promise<SourceTreeDraft> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-drafts/${encodeURIComponent(draftId)}`, {
    method: "PUT",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceTreeDraft;
}

// previewSourceTreeDraftImport is the import's dry run: it computes the
// counts, the links and the candidates and writes nothing. The apply run
// refuses to proceed without this report carried back.
export async function previewSourceTreeDraftImport(draftId: string, expectedRevision: number): Promise<SourceTreeDraftImportPreview> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-drafts/${encodeURIComponent(draftId)}/import-preview`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expectedRevision }),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const result = (await response.json()) as SourceTreeDraftImportPreview;
  return { ...result, collisions: result.collisions ?? [], warnings: result.warnings ?? [] };
}

export async function importSourceTreeDraft(draftId: string, input: SourceTreeDraftImportInput): Promise<SourceTreeDraftImportResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-drafts/${encodeURIComponent(draftId)}/import`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceTreeDraftImportResult;
}

export async function retractSourceTreeDraft(draftId: string): Promise<SourceTreeDraftRetractionResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-drafts/${encodeURIComponent(draftId)}/retract`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceTreeDraftRetractionResult;
}

export async function createBookAssertion(fileId: string, input: BookAssertionInput): Promise<BookAssertion> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/book-assertions`, {
    method: "POST", credentials: "include", cache: "no-store", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input), signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as BookAssertion;
}

export async function updateBookAssertion(assertionId: string, input: BookAssertionInput & { expectedRevision: number }): Promise<BookAssertion> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-book-assertions/${encodeURIComponent(assertionId)}`, {
    method: "PUT", credentials: "include", cache: "no-store", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input), signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as BookAssertion;
}

export async function reviewBookAssertion(assertionId: string, input: ReviewBookAssertionInput): Promise<BookAssertion> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-book-assertions/${encodeURIComponent(assertionId)}/review`, {
    method: "POST", credentials: "include", cache: "no-store", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input), signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as BookAssertion;
}

export async function fetchBookAssertions(fileId: string): Promise<BookAssertion[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/book-assertions`, {
    credentials: "include", cache: "no-store", signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as BookAssertion[];
}

export async function createSourceFileDownloadLink(fileId: string): Promise<SourceFileDownload> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/download`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceFileDownload;
}

export async function uploadSourceFile(sourceId: string, file: File): Promise<SourceFile> {
  const body = new FormData();
  body.append("file", file);
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/files`, {
    method: "POST",
    credentials: "include",
    body,
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceFile;
}

export async function fetchSessionStatus(): Promise<boolean> {
  const response = await fetch(`${apiBaseUrl}/api/v1/auth/me`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (response.status === 401) return false;
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return true;
}

export async function fetchResearchAccess(): Promise<{ signedIn: boolean; canReview: boolean; canMerge: boolean }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/auth/me`, { credentials: "include", cache: "no-store", signal: AbortSignal.timeout(5000) });
  if (response.status === 401) return { signedIn: false, canReview: false, canMerge: false };
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const { roles = [] } = await response.json() as { roles?: string[] };
  return { signedIn: true, canReview: roles.some((role) => ["researcher", "moderator", "admin"].includes(role)), canMerge: roles.some((role) => ["moderator", "admin"].includes(role)) };
}

export async function reviewSourceCandidate(candidateId: string, decision: "accepted" | "rejected", noteAr?: string): Promise<SourceCandidate> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-candidates/${candidateId}/review`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ decision, note_ar: noteAr }),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceCandidate;
}

export async function createSource(input: CreateSourceInput): Promise<SourceDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDetail;
}

export async function createSourcePassage(sourceId: string, input: CreatePassageInput): Promise<SourceDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/passages`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDetail;
}

export async function createSourceStatement(sourceId: string, input: CreateStatementInput): Promise<SourceDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/statements`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDetail;
}

export async function fetchClaims(): Promise<ClaimSummary[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/claims`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: ClaimSummary[] };
  return payload.items ?? [];
}

export async function fetchClaim(claimId: string): Promise<ResearchClaim> {
  const response = await fetch(`${apiBaseUrl}/api/v1/claims/${claimId}`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchClaim;
}

export async function createClaim(input: CreateClaimInput): Promise<ResearchClaim> {
  const response = await fetch(`${apiBaseUrl}/api/v1/claims`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchClaim;
}

export async function addClaimEvidence(claimId: string, input: AddEvidenceInput): Promise<ResearchClaim> {
  const response = await fetch(`${apiBaseUrl}/api/v1/claims/${claimId}/evidence`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchClaim;
}

export async function fetchCollaborators(treeId: string): Promise<CollaboratorsResponse> {
  requireRealTree(treeId);
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/collaborators`, {
    credentials: "include",
    signal: AbortSignal.timeout(3000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as CollaboratorsResponse;
}

export async function createInvitation(treeId: string, input: { invitee_email: string; permission_level: "view" | "edit" | "review" }): Promise<InvitationCreated> {
  requireRealTree(treeId);
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/invitations`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as InvitationCreated;
}

export async function updateCollaboratorPermission(treeId: string, userId: string, input: UpdatePermissionInput): Promise<void> {
  requireRealTree(treeId);
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/collaborators/${userId}`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
}

export async function removeCollaborator(treeId: string, userId: string): Promise<void> {
  requireRealTree(treeId);
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/collaborators/${userId}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
}

export async function revokeInvitation(treeId: string, invitationId: string): Promise<void> {
  requireRealTree(treeId);
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/invitations/${invitationId}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
}

export async function fetchInvitations(): Promise<TreeInvitation[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/invitations`, {
    credentials: "include",
    signal: AbortSignal.timeout(3000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: TreeInvitation[] };
  return payload.items ?? [];
}

export async function acceptInvitation(token: string): Promise<{ invitationId: string; treeId: string; treeName: string; permissionLevel: "view" | "edit" | "review" }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/invitations/${encodeURIComponent(token)}/accept`, {
    method: "POST",
    credentials: "include",
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { invitationId: string; treeId: string; treeName: string; permissionLevel: "view" | "edit" | "review" };
}

export async function fetchTreeActivity(treeId: string): Promise<TreeActivity[]> {
  requireRealTree(treeId);
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/activity`, {
    credentials: "include",
    signal: AbortSignal.timeout(3000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: TreeActivity[] };
  return payload.items ?? [];
}

function requireRealTree(treeId: string): void {
  if (!apiBaseUrl || treeId === demoTreeId) {
    throw new ApiError("هذه الميزة تحتاج إلى شجرة محفوظة وCore API.", 409);
  }
}

async function readErrorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { error?: string };
    return body.error ?? "تعذر إكمال العملية.";
  } catch {
    return "تعذر إكمال العملية.";
  }
}

/**
 * The same reader as readErrorMessage, plus the `field` a route may name. Reading the
 * body twice is not possible - a Response body is consumed once - so this is the one
 * that parses, and readErrorMessage is the wrapper for every route that has no field
 * to report.
 */
async function readApiError(response: Response): Promise<ApiError> {
  try {
    const body = (await response.json()) as { error?: string; field?: string };
    return new ApiError(body.error ?? "تعذر إكمال العملية.", response.status, body.field);
  } catch {
    return new ApiError("تعذر إكمال العملية.", response.status);
  }
}

// The suggestion queue: the machine's readings of a file, the most certain
// first. The confidence orders the queue and decides nothing - every suggestion
// waits for a person.
export async function fetchSourceTreeSuggestions(fileId: string, status = "suggested", limit = 50, offset = 0): Promise<SourceTreeSuggestion[]> {
  const query = new URLSearchParams({ status, limit: String(limit), offset: String(offset) });
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/review-queue?${query.toString()}`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceTreeSuggestion[];
}

// The review bench asks for counts, not evidence. A summary is permission-
// scoped on the server and carries no name, image, or judge result.
export async function fetchNameReviewSummary(fileId: string): Promise<NameReviewSummary> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/review-summary`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const value = (await response.json()) as NameReviewSummary;
  if (typeof value.total !== "number" || typeof value.pending !== "number" || typeof value.deferred !== "number") {
    throw new ApiError("أعادت الخدمة ملخص مراجعة غير صالح.", 502);
  }
  return value;
}

// History is read from the append-only ledger. It is not reconstructed from the
// current queue row, so an earlier answer remains visible after a reopen.
export async function fetchSourceSuggestionHistory(suggestionId: string, signal?: AbortSignal): Promise<SourceSuggestionHistory> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/history`, {
    credentials: "include",
    cache: "no-store",
    signal: nameReadingSignal(signal, 10000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const value = (await response.json()) as SourceSuggestionHistory;
  if (!value.suggestion || !Array.isArray(value.decisions) || !Array.isArray(value.events) || !Array.isArray(value.regionRevisions)) {
    throw new ApiError("أعادت الخدمة سجلاً غير صالح.", 502);
  }
  return value;
}

// Reopening appends an event and returns the occurrence to pending work. It
// never deletes the decision it supersedes, so expectedRevision is mandatory.
export async function reopenSourceSuggestion(suggestionId: string, expectedRevision: number, noteAr = ""): Promise<SourceTreeSuggestion> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/reopen`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expectedRevision, noteAr }),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as SourceTreeSuggestion;
}

export async function previewSourceSuggestionRegion(
  suggestionId: string,
  input: SourceSuggestionRegionInput,
  signal?: AbortSignal,
): Promise<SourceSuggestionRegionPreview> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/region-preview`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: nameReadingSignal(signal, 30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const value = (await response.json()) as SourceSuggestionRegionPreview;
  if (!value.tightPng || !value.contextPng || typeof value.sourceChecksum !== "string") {
    throw new ApiError("أعادت الخدمة معاينة حدود غير صالحة.", 502);
  }
  return value;
}

export async function reviseSourceSuggestionRegion(
  suggestionId: string,
  input: SourceSuggestionRegionInput,
): Promise<SourceTreeSuggestion> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/region`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as SourceTreeSuggestion;
}

export async function splitSourceTreeSuggestion(
  suggestionId: string,
  input: SourceSuggestionSplitInput,
): Promise<SourceSuggestionSplitResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/split`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as SourceSuggestionSplitResult;
}

export async function mergeSourceTreeSuggestions(
  suggestionId: string,
  input: SourceSuggestionMergeInput,
): Promise<SourceSuggestionMergeResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/merge`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as SourceSuggestionMergeResult;
}

// The source-resolution crops for one suggestion. The endpoint returns two
// base64 PNGs in one response so the page is opened once; the browser gets an
// object URL for each and revokes them when the active name changes.
export interface SuggestionCropGeometry {
  pageWidth: number;
  pageHeight: number;
  tightRegion: { x: number; y: number; width: number; height: number };
  contextRegion: { x: number; y: number; width: number; height: number };
  reviewRegion?: { x: number; y: number; width: number; height: number };
  fragment: boolean;
}

export async function fetchSourceSuggestionCrop(suggestionId: string, signal?: AbortSignal): Promise<{ tight: Blob; context: Blob; review?: Blob; geometry?: SuggestionCropGeometry }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/crop`, {
    credentials: "include",
    cache: "no-store",
    signal: nameReadingSignal(signal, 60000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { tightPng?: string; contextPng?: string; reviewPng?: string; geometry?: SuggestionCropGeometry };
  if (!payload.tightPng || !payload.contextPng) {
    throw new ApiError("لم تُرجع الخدمة صورة الاسم.", 502);
  }
  return { tight: base64PNG(payload.tightPng), context: base64PNG(payload.contextPng), review: payload.reviewPng ? base64PNG(payload.reviewPng) : undefined, geometry: payload.geometry };
}

// On-demand spelling candidates from the vision model for ONE tight crop. It is
// deliberately not automatic: every call is a paid model request and the answer
// is only a list of hypotheses for the reviewer.
export async function fetchSourceSuggestionCandidates(suggestionId: string): Promise<string[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/candidates`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(120000),
  });
  if (!response.ok) {
    const error = await response.json().catch(() => ({})) as { error?: string; code?: string };
    throw new ApiError(error.error || "تعذّر طلب القراءة.", response.status, undefined, error.code);
  }
  const payload = (await response.json()) as { candidates?: unknown };
  if (!Array.isArray(payload.candidates)) {
    throw new ApiError("لم تُرجع الخدمة قائمة اقتراحات صالحة.", 502);
  }
  return payload.candidates
    .filter((candidate): candidate is string => typeof candidate === "string")
    .map((candidate) => candidate.trim())
    .filter(Boolean)
    .slice(0, 8);
}

export async function fetchReadingCapabilities(fileId: string): Promise<ReadingCapabilities> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/reading-capabilities`, {
    credentials: "include", cache: "no-store", signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const value = await response.json() as ReadingCapabilities;
  if (!["off", "local", "hosted"].includes(value.mode) || !value.localReader || !value.hostedReader ||
      typeof value.localReader.ready !== "boolean" || typeof value.hostedReader.ready !== "boolean") {
    throw new ApiError("أعادت الخدمة حالة قراءة غير صالحة.", 502);
  }
  // A malformed or contradictory capability must never enable image transport.
  value.hostedReader.ready = value.mode === "hosted" && value.hostedReader.enabled === true &&
    value.hostedReader.configured === true && value.hostedReader.allowedForSource === true && value.hostedReader.ready;
  return value;
}

// The review bench asks once for the judge posture. Off and unavailable are
// normal states, not errors: the bench is fully usable without a judge. A judge the
// platform cannot reach — no API configured, or one that refuses — is therefore
// reported as "off", never thrown at the bench. An unparseable configuration is
// still an error, because that is the platform misreporting itself.
export async function fetchNameJudgeConfig(): Promise<NameJudgeConfig> {
  let response: Response;
  try {
    response = await fetch(`${apiBaseUrl}/api/v1/name-judge/config`, {
      credentials: "include",
      cache: "no-store",
      signal: AbortSignal.timeout(10000),
    });
  } catch {
    return { mode: "off", imageEnabled: false, available: false, sample: 0 };
  }
  if (!response.ok) {
    return { mode: "off", imageEnabled: false, available: false, sample: 0 };
  }
  const payload = (await response.json()) as Partial<NameJudgeConfig>;
  if (payload.mode !== "off" && payload.mode !== "shadow" && payload.mode !== "active") {
    throw new ApiError("أعادت الخدمة إعداد حكم أسماء غير صالح.", 502);
  }
  return {
    mode: payload.mode,
    imageEnabled: payload.imageEnabled === true,
    available: payload.available === true,
    sample: typeof payload.sample === "number" ? Math.max(0, Math.min(1, payload.sample)) : 0,
  };
}

// Ask the bounded judge for an opinion. Candidate values are the only payload:
// no source id, family id, account id, reviewer identity, or page image is sent
// from the browser. The core owns the experimental crop gate.
export async function judgeSourceTreeSuggestion(
  suggestionId: string,
  candidates: NameJudgeCandidateInput[],
): Promise<NameJudgeResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/judge`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ candidates }),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as Partial<NameJudgeResult>;
  const probabilities = payload.probabilities;
  if (
    (payload.choice !== null && typeof payload.choice !== "string") ||
    typeof payload.abstained !== "boolean" ||
    typeof payload.confidence !== "number" ||
    payload.confidence < 0 ||
    payload.confidence > 1 ||
    typeof payload.provider !== "string" ||
    typeof payload.model !== "string" ||
    typeof payload.promptVersion !== "string" ||
    !probabilities ||
    typeof probabilities !== "object" ||
    Object.values(probabilities).some((value) => typeof value !== "number" || value < 0 || value > 1)
  ) {
    throw new ApiError("أعادت الخدمة رأي نموذج غير صالح.", 502);
  }
  if (payload.abstained && payload.choice !== null) {
    throw new ApiError("أعادت الخدمة رأي نموذج غير متسق.", 502);
  }
  return {
    choice: payload.choice ?? null,
    abstained: payload.abstained,
    probabilities: probabilities as Record<string, number>,
    confidence: payload.confidence,
    readable: payload.readable === true ? true : payload.readable === false ? false : null,
    provider: payload.provider,
    model: payload.model,
    promptVersion: payload.promptVersion,
    cacheHit: payload.cacheHit === true,
  };
}

function base64PNG(value: string): Blob {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return new Blob([bytes], { type: "image/png" });
}

// Record what a reviewer decided about a suggestion. A correction must carry the
// correction; a rejection must carry no text. The optional evidence is what the
// reviewer saw at decision time: a candidate snapshot, the selected candidate,
// how it was entered, and an optional judge answer. It is evidence, not a new
// suggestion, and the server validates it before adding it to the ledger.
//
// A rejected snapshot must not lose the reviewer's decision. If the server
// rejects the evidence as malformed (400/422), retry exactly once without it and
// report that the evidence was not recorded. The report counts the missing
// snapshot as a measurement gap; the decision itself is still real.
export interface SourceTreeSuggestionDecisionResult {
  suggestion: SourceTreeSuggestion;
  evidenceRecorded: boolean;
}

function hasDecisionEvidence(evidence: SourceTreeSuggestionDecisionEvidence): boolean {
  return Boolean(
    evidence.candidateSnapshot?.length ||
    evidence.selectedCandidate ||
    evidence.inputMethod ||
    evidence.judgeOutput ||
    evidence.normalizerVersion ||
    evidence.candidatePoolVersion ||
    evidence.durationMs !== undefined,
  );
}

export async function decideSourceTreeSuggestion(
  suggestionId: string,
  decision: SourceTreeSuggestionDecision,
  textAr = "",
  noteAr = "",
  evidence: SourceTreeSuggestionDecisionEvidence = {},
): Promise<SourceTreeSuggestionDecisionResult> {
  const post = (includeEvidence: boolean) => fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/decision`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(includeEvidence ? { decision, textAr, noteAr, ...evidence } : {
      decision,
      textAr,
      noteAr,
      ...(evidence.evidenceSchemaVersion ? { evidenceSchemaVersion: evidence.evidenceSchemaVersion } : {}),
      ...(evidence.expectedRevision !== undefined ? { expectedRevision: evidence.expectedRevision } : {}),
      ...(evidence.expectedGeometryRevision !== undefined ? { expectedGeometryRevision: evidence.expectedGeometryRevision } : {}),
    }),
    signal: AbortSignal.timeout(30000),
  });

  let evidenceRecorded = true;
  let response = await post(true);
  if (!response.ok && (response.status === 400 || response.status === 422) && hasDecisionEvidence(evidence)) {
    // No names, text, or evidence are logged: only the fact that this decision
    // will be counted with a missing snapshot.
    console.warn(`source suggestion decision evidence was rejected (${response.status}); retrying once without it`);
    response = await post(false);
    evidenceRecorded = false;
  }
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return {
    suggestion: (await response.json()) as SourceTreeSuggestion,
    evidenceRecorded,
  };
}

// How often the reader was right without correction, from the decisions the
// reviewers made.
export async function fetchSourceSuggestionAccuracy(fileId: string): Promise<SourceSuggestionAccuracy> {
  const response = await fetch(`${apiBaseUrl}/api/v1/source-files/${encodeURIComponent(fileId)}/suggestions/accuracy`, {
    credentials: "include",
    cache: "no-store",
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceSuggestionAccuracy;
}

// Run the reading pipeline over a file and record the suggestions it finds.
export function nameReadingSignal(signal?: AbortSignal, deadlineMs = 10000): AbortSignal {
 return signal ? AbortSignal.any([signal, AbortSignal.timeout(deadlineMs)]) : AbortSignal.timeout(deadlineMs);
}

async function nameRunRequest(path: string, method = "GET", signal?: AbortSignal): Promise<NameReadingRun> {
 const response = await fetch(`${apiBaseUrl}/api/v1/${path}`, { method, credentials: "include", cache: "no-store", signal: nameReadingSignal(signal) });
 if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
 const result = await response.json() as NameReadingRun;
 if (typeof result.id !== "string" || !["queued", "running", "partial", "succeeded", "failed", "cancelled"].includes(result.status)) throw new ApiError("أعادت الخدمة حالة إعداد غير صالحة.", 502);
 return result;
}
export function readSourceTreeSuggestions(fileId: string, signal?: AbortSignal): Promise<NameReadingRun> {
 return nameRunRequest(`source-files/${encodeURIComponent(fileId)}/suggestions/read`, "POST", signal);
}
export async function fetchLatestNameReadingRun(fileId: string, signal?: AbortSignal): Promise<NameReadingRun | null> {
 try { return await nameRunRequest(`source-files/${encodeURIComponent(fileId)}/name-reading-runs/latest`, "GET", signal); }
 catch (error) { if (error instanceof ApiError && error.status === 404) return null; throw error; }
}
export function fetchNameReadingRun(runId: string, signal?: AbortSignal): Promise<NameReadingRun> {
 return nameRunRequest(`name-reading-runs/${encodeURIComponent(runId)}`, "GET", signal);
}
export function controlNameReadingRun(runId: string, action: "resume" | "cancel", signal?: AbortSignal): Promise<NameReadingRun> {
 return nameRunRequest(`name-reading-runs/${encodeURIComponent(runId)}/${action}`, "POST", signal);
}
export function rereadSourceSuggestionLocally(suggestionId: string, signal?: AbortSignal): Promise<NameReadingRun> {
 return nameRunRequest(`source-tree-suggestions/${encodeURIComponent(suggestionId)}/local-reread`, "POST", signal);
}
export async function fetchLocalNameReadings(suggestionId: string, signal?: AbortSignal): Promise<LocalNameReading[]> {
 const response = await fetch(`${apiBaseUrl}/api/v1/source-tree-suggestions/${encodeURIComponent(suggestionId)}/local-readings`, { credentials: "include", cache: "no-store", signal: nameReadingSignal(signal) });
 if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
 return await response.json() as LocalNameReading[];
}

export async function fetchLatestLocalRereadRun(suggestionId: string, signal?: AbortSignal): Promise<NameReadingRun | null> {
 try { return await nameRunRequest(`source-tree-suggestions/${encodeURIComponent(suggestionId)}/local-reread/latest`, "GET", signal); }
 catch (error) { if (error instanceof ApiError && error.status === 404) return null; throw error; }
}


async function nameReviewRequest<T>(path: string, method: string, body?: unknown, signal?: AbortSignal): Promise<T> {
 const response = await fetch(`${apiBaseUrl}/api/v1/${path}`, { method, credentials: "include", cache: "no-store",
  headers: body === undefined ? undefined : { "Content-Type": "application/json" },
  body: body === undefined ? undefined : JSON.stringify(body), signal: nameReadingSignal(signal, 6000) });
 if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
 return await response.json() as T;
}
export function fetchNameLexicon(fileId: string, signal?: AbortSignal) { return nameReviewRequest<NameLexicon>(`source-files/${encodeURIComponent(fileId)}/name-lexicon`, "GET", undefined, signal); }
export function beginNameReview(suggestionId: string, signal?: AbortSignal) { return nameReviewRequest<NamePreparationContext>(`source-tree-suggestions/${encodeURIComponent(suggestionId)}/review-context`, "POST", undefined, signal); }
export function prepareNameCandidates(suggestionId: string, exposureId: string, snapshot: SourceTreeSuggestionCandidateSnapshot[], signal?: AbortSignal) {
 return nameReviewRequest<NameCandidatePreparation>(`source-tree-suggestions/${encodeURIComponent(suggestionId)}/candidate-preparation`, "POST", { exposureId, poolVersion: "ar-name-pool-v2", snapshot }, signal);
}
export function sealNameReview(suggestionId: string) { return nameReviewRequest<{ sealed: boolean }>(`source-tree-suggestions/${encodeURIComponent(suggestionId)}/review-context/seal`, "POST"); }

// ---------------------------------------------------------------------------
// The region lanes (plans 030-E/F): machine-proposed plate labels, the tile
// workbench that organises them, and the decisions that answer them.
//
// The grammar these types carry is the archive's certainty grammar, not a
// general-purpose review queue: a candidate is a PROPOSAL with an empty reading
// until a named reviewer fills it, score components travel apart on purpose,
// every list is bounded with a truncated flag, and a tile retires only when
// two passes by different reviewers agree.
// ---------------------------------------------------------------------------

/** One machine-proposed plate label (plan 030-E). `labelTextAr` is empty until a reviewer reads it. */
export interface RegionCandidate {
  id: string;
  sourceId: string;
  sourceTitle?: string;
  fileSha256?: string;
  plateKey: string;
  occurrenceId: string;
  cropPx: { x: number; y: number; w: number; h: number };
  coordinates?: { longitude: number; latitude: number };
  hasRegionGeometry: boolean;
  regionGeometryWkt?: string;
  labelTextAr: string;
  labelReadBy?: string;
  labelReadAt?: string;
  eraLabel: string;
  inkDensity?: number;
  componentCount?: number;
  labelConfidence?: number;
  transformRmsKm?: number;
  rank?: number;
  status: "suggested" | "accepted" | "corrected" | "rejected" | "deferred";
  decidedBy?: string;
  decidedAt?: string;
  decisionNoteAr?: string;
  createdAt: string;
}

export interface RegionCandidateListResponse {
  candidates: RegionCandidate[];
  status: string;
  limit: number;
  truncated: boolean;
}

/** The plate's honesty block, carried from `sources.metadata.georef`. */
export interface RegionPlateSummary {
  sourceId: string;
  plateKey: string;
  titleAr?: string;
  license?: string;
  georefMethod?: string;
  certainty?: string;
  needsReview: boolean;
  reviewReasons: string[];
}

export interface RegionCandidateDetailResponse {
  candidate: RegionCandidate;
  plate?: RegionPlateSummary;
}

/** The tight and context crops rendered from the stored scan ([]byte fields arrive base64). */
export interface RegionCandidateCrop {
  candidateId: string;
  plateKey: string;
  tightPng: string;
  contextPng: string;
  geometry: {
    plateWidth: number;
    plateHeight: number;
    cropPx: { x: number; y: number; w: number; h: number };
    tightBox: { x: number; y: number; w: number; h: number };
    contextBox: { x: number; y: number; w: number; h: number };
    scaled: boolean;
  };
  plate?: RegionPlateSummary;
}

export interface RegionCandidateHistoryResponse {
  candidateId: string;
  history: Array<{
    id: string;
    candidateId: string;
    reviewerId: string;
    reviewerName?: string;
    action: "accept" | "correct" | "reject" | "defer" | "reopen";
    fromStatus: string;
    toStatus: string;
    noteAr?: string;
    diff?: string;
    createdAt: string;
  }>;
  limit: number;
  truncated: boolean;
}

export interface RegionDecideInput {
  action: "accept" | "correct" | "reject" | "defer" | "reopen";
  noteAr?: string;
  labelTextAr?: string;
  eraLabel?: string;
}

/** The candidate queue: one status lane, bounded (`limit` + `truncated`). */
export async function fetchRegionCandidates(params: { status?: string; plateKey?: string; query?: string } = {}): Promise<RegionCandidateListResponse> {
  const search = new URLSearchParams();
  search.set("status", params.status ?? "suggested");
  if (params.plateKey?.trim()) search.set("plate_key", params.plateKey.trim());
  if (params.query?.trim()) search.set("q", params.query.trim());
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates?${search.toString()}`, {
    credentials: "include",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionCandidateListResponse;
}

export async function fetchRegionCandidate(candidateID: string, signal?: AbortSignal): Promise<RegionCandidateDetailResponse> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates/${encodeURIComponent(candidateID)}`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(8000), signal]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionCandidateDetailResponse;
}

/**
 * The candidate's crop from the stored scan: the plate's own pixels rendered
 * tight (the box the detector ranked) and with context (the neighbourhood a
 * person orients by). A missing or corrupt scan is a 503 and surfaces as an
 * error, never as a silent empty image.
 */
export async function fetchRegionCandidateCrop(candidateID: string, signal?: AbortSignal): Promise<RegionCandidateCrop> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates/${encodeURIComponent(candidateID)}/crop`, {
    credentials: "include",
    cache: "no-store",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(20000), signal]) : AbortSignal.timeout(20000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionCandidateCrop;
}

export async function fetchRegionCandidateHistory(candidateID: string, signal?: AbortSignal): Promise<RegionCandidateHistoryResponse> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates/${encodeURIComponent(candidateID)}/history`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(8000), signal]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionCandidateHistoryResponse;
}

/** One reviewer decision: accept/correct carry the reading, reject optionally names the crop, defer carries none. */
export async function decideRegionCandidate(candidateID: string, input: RegionDecideInput): Promise<{ candidate: RegionCandidate; action: string }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates/${encodeURIComponent(candidateID)}/decide`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { candidate: RegionCandidate; action: string };
}

/** One tile task: a bounded square of a plate holding proposals. */
export interface RegionTile {
  id: string;
  sourceId: string;
  plateKey: string;
  tileId: string;
  boxPx: { x: number; y: number; w: number; h: number };
  status: "open" | "in_progress" | "submitted" | "retired";
  claimedBy?: string;
  claimedAt?: string;
  passes: number;
  createdAt: string;
  updatedAt: string;
}

export interface RegionTileListResponse {
  tiles: RegionTile[];
  status: string;
  limit: number;
  truncated: boolean;
}

/** The workbench's header counts: how many tile tasks sit in each lane. */
export interface RegionTileSummary {
  counts: Record<string, number>;
  statuses: string[];
}

/** One choosing surface row: a dictionary name the reviewer may pick. */
export interface RegionNameSuggestion {
  valueAr: string;
  normalizedAr: string;
  kind: "canonical" | "alias";
}

export interface RegionTileCandidate {
  answeredByMe: boolean;
  id: string;
  occurrenceId: string;
  cropPx: { x: number; y: number; w: number; h: number };
  status: RegionCandidate["status"];
  labelTextAr: string;
  labelReadBy?: string;
  decidedBy?: string;
}

export interface RegionTileDetailResponse {
  tile: RegionTile;
  claimedByMe: boolean;
  candidates: RegionTileCandidate[];
  limit: number;
  truncated: boolean;
}

export interface RegionTileSubmitResult {
  tile: RegionTile;
  action: "submit" | "confirm" | "invalidate";
  otherSubmitter?: string;
  agreement?: {
    retires: boolean;
    selfReview: boolean;
    confirmations: string[];
    deferrals: string[];
    disagreements: string[];
    unevaluated: string[];
  };
}

export interface RegionTileHistoryResponse {
  taskId: string;
  history: Array<{
    id: string;
    taskId: string;
    reviewerId: string;
    reviewerName?: string;
    action: "claim" | "release" | "submit" | "confirm" | "invalidate" | "reopen";
    fromStatus: string;
    toStatus: string;
    noteAr?: string;
    snapshot?: string;
    createdAt: string;
  }>;
  limit: number;
  truncated: boolean;
}

/** The tile queue: one status lane, bounded. The default lane is `open`. */
export async function fetchRegionTiles(params: { status?: string; plateKey?: string } = {}): Promise<RegionTileListResponse> {
  const search = new URLSearchParams();
  search.set("status", params.status ?? "open");
  if (params.plateKey?.trim()) search.set("plate_key", params.plateKey.trim());
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-workbench/tiles?${search.toString()}`, {
    credentials: "include",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionTileListResponse;
}

/** The header's own counts: one bounded read, no per-lane scans. */
export async function fetchRegionTileSummary(): Promise<RegionTileSummary> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-workbench/summary`, {
    credentials: "include",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionTileSummary;
}

/** The choosing surface's own names: the dictionary's bounded match. */
export async function fetchRegionNameSuggestions(query: string): Promise<RegionNameSuggestion[]> {
  const params = new URLSearchParams({ q: query });
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates/name-suggestions?${params.toString()}`, {
    credentials: "include",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { suggestions?: RegionNameSuggestion[] };
  return payload.suggestions ?? [];
}

export async function fetchRegionTile(taskID: string, signal?: AbortSignal): Promise<RegionTileDetailResponse> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-workbench/tiles/${encodeURIComponent(taskID)}`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(8000), signal]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionTileDetailResponse;
}

export async function claimRegionTile(taskID: string): Promise<RegionTile> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-workbench/tiles/${encodeURIComponent(taskID)}/claim`, {
    method: "POST",
    credentials: "include",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionTile;
}

/** Defers a held seat: the tile returns to the open queue, history keeps the round trip. */
export async function releaseRegionTile(taskID: string, noteAr = ""): Promise<RegionTile> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-workbench/tiles/${encodeURIComponent(taskID)}/release`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ noteAr }),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionTile;
}

/** Closes one pass over the tile; a second submit compares the two passes and retires or returns the tile. */
export async function submitRegionTile(taskID: string, noteAr = ""): Promise<RegionTileSubmitResult> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-workbench/tiles/${encodeURIComponent(taskID)}/submit`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ noteAr }),
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionTileSubmitResult;
}

export async function fetchRegionTileHistory(taskID: string, signal?: AbortSignal): Promise<RegionTileHistoryResponse> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-workbench/tiles/${encodeURIComponent(taskID)}/history`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(8000), signal]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as RegionTileHistoryResponse;
}

/* The atlas's plate register (plan 032-A): every plate source with its
 * validated transform, or the honest disabled caption. Shapes mirror the
 * core-api's geography.PlateRegisterResponse exactly — the register never
 * invents a number the server did not measure (the fit block is the
 * transform's own held-out floor, surfaced verbatim). */
export interface AtlasPlateFit {
  lonRmsDeg: number;
  latRmsDeg: number;
  lonMaxDeg: number;
  latMaxDeg: number;
}

export interface AtlasPlateTransform {
  affine?: [number, number, number, number, number, number];
  kind: string;
  apexX: number;
  apexY: number;
  coneConstant: number;
  centralLongitude: number;
  latC0: number;
  latC1: number;
  pixelsPerDegree: number;
}

export interface AtlasPlateFrame {
  outerLeft: number;
  innerLeft: number;
  innerRight: number;
  outerRight: number;
  outerTop: number;
  innerTop: number;
  innerBottom: number;
  outerBottom: number;
}

export interface AtlasPlateTransformation {
  method: string;
  transform: AtlasPlateTransform;
  frame: AtlasPlateFrame;
  fit: AtlasPlateFit;
  certainty: string;
  needsReview: boolean;
  reviewReasons: string[];
}

export interface PlateCaption {
  tribeId?: string;
  id: string;
  x: number;
  y: number;
  printedName: string;
  nameAr: string;
}

export interface PlateCaptionRevision {
  id: string;
  nameAr: string;
  proposedBy: string;
  published: boolean;
}

export async function plateCaptionRevisions(captionId: string): Promise<PlateCaptionRevision[]> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/plate-captions/${encodeURIComponent(captionId)}/revisions`, {
    credentials: "include", signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as PlateCaptionRevision[];
}

export async function savePlateCaptionRevision(captionId: string, revision: { nameAr: string } | { id: string }) {
  const suffix = "id" in revision ? `/${encodeURIComponent(revision.id)}/publish` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/plate-captions/${encodeURIComponent(captionId)}/revisions${suffix}`, {
    credentials: "include", method: "POST", signal: AbortSignal.timeout(8000),
    headers: { "Content-Type": "application/json" },
    body: "nameAr" in revision ? JSON.stringify(revision) : undefined,
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
}

export interface AtlasPlateRegisterRow {
  sourceId: string;
  plateKey: string;
  titleAr: string;
  license: string;
  plateDate?: string;
  plateWidth?: number;
  plateHeight?: number;
  controlPoints: number;
  transform?: AtlasPlateTransformation;
  disabledCaption?: string;
  captions?: PlateCaption[];
}

export interface AtlasPlateRegisterResponse {
  plates: AtlasPlateRegisterRow[];
  limit: number;
  truncated: boolean;
}

export interface AtlasPlatePointCropResponse {
  plateKey: string;
  longitude: number;
  latitude: number;
  platePointPx: string;
  cropPng: string;
  cropBox: { x: number; y: number; w: number; h: number };
  insideNeatline: boolean;
  plate?: {
    sourceId?: string;
    plateKey: string;
    georefMethod?: string;
    certainty?: string;
    needsReview: boolean;
    reviewReasons: string[];
  };
}

/** The plate register is a public read (the plates' own publicity gates the
 * layer); a 403/401 is surfaced as the honest answer it is. Aborts cleanly
 * when the atlas unmounts mid-fetch. */
export async function fetchAtlasPlateRegister(signal?: AbortSignal): Promise<AtlasPlateRegisterResponse> {
  const timeoutSignal = AbortSignal.timeout(8000);
  const response = await fetch(`${apiBaseUrl}/api/v1/plates`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([timeoutSignal, signal]) : timeoutSignal,
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as AtlasPlateRegisterResponse;
}

/* الأطلس — مقارنة النقطة، طعون المضلّع، أروابط العلاقات (plan 032-C/D/E). */

export interface AtlasPlatePointCrop {
  plateKey: string;
  longitude: number;
  latitude: number;
  platePointPx: string;
  insideNeatline: boolean;
  cropPng: string;
  cropBox: { x: number; y: number; w: number; h: number };
  plate?: {
    plateKey: string;
    georefMethod?: string;
    certainty?: string;
    needsReview: boolean;
    reviewReasons: string[];
  };
}

/** One plate's read at one map point (plan 032-C). A plate without a usable
 * transform is refused server-side — the panel captions that refusal rather
 * than retrying, and no crop is fetched for it. */
export async function fetchAtlasPlatePointCrop(plateKey: string, longitude: number, latitude: number, signal?: AbortSignal, size?: number): Promise<AtlasPlatePointCrop> {
  const params = new URLSearchParams({ lon: String(longitude), lat: String(latitude) });
  if (size !== undefined) params.set("size", String(size));
  const response = await fetch(`${apiBaseUrl}/api/v1/plates/${encodeURIComponent(plateKey)}/crop?${params.toString()}`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(15000), signal]) : AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as AtlasPlatePointCrop;
}

export interface AtlasClaimChallengeDetail {
  dispute: {
    id: string;
    titleAr: string;
    descriptionAr?: string;
    status: string;
    resolutionAr?: string;
    claimCount: number;
  };
  claims: Array<{
    claimId: string;
    position: string;
    subjectType: string;
    subjectId: string;
    predicate: string;
    objectType: string;
    objectId: string;
    status: string;
  }>;
}

/** The claim's latest dispute thread — the atlas's disputed side-by-side
 * (plan 032-D). A claim with no dispute thread is an honest 404; the panel
 * says so instead of pretending calm. */
export async function fetchAtlasClaimChallenge(claimID: string, signal?: AbortSignal): Promise<AtlasClaimChallengeDetail> {
  const response = await fetch(`${apiBaseUrl}/api/v1/claims/${encodeURIComponent(claimID)}/challenge`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(8000), signal]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as AtlasClaimChallengeDetail;
}

export interface AtlasRelationArc {
  id: string;
  predicate: string;
  status: string;
  subject: { entityType: string; entityId: string; label: string; longitude: number | null; latitude: number | null };
  object: { entityType: string; entityId: string; label: string; longitude: number | null; latitude: number | null };
  validFrom?: string;
  validTo?: string;
}

export interface AtlasRelationsResponse {
  relations: AtlasRelationArc[];
  limit: number;
  truncated: boolean;
}

/** The era's recorded relationship arcs (plan 032-E): the layer the era
 * chips scope. Zero years means "any era" — the atlas's full read. */
export async function fetchAtlasRelations(fromYear: number, toYear: number, signal?: AbortSignal): Promise<AtlasRelationsResponse> {
  const params = new URLSearchParams();
  if (fromYear !== 0) params.set("from", String(fromYear));
  if (toYear !== 0) params.set("to", String(toYear));
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/atlas/relations${suffix}`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([AbortSignal.timeout(8000), signal]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as AtlasRelationsResponse;
}

/* ممرات الحركة (plan 033-B): أحد الموضوعات المسجَّلة أعمدة ممراته، بجلوس
 * المحطات المرتّبة التيermint تحمل مصدرها. الحدود كما دائماً، والأعمدة تُرسم
 * من محطاتها فقط. */
export interface MovementCorridorStop {
  sequence: number;
  placeId: string;
  placeName: string;
  longitude: number;
  latitude: number;
  dayCountFromPrev?: number;
  modeFromPrev: string;
  notes?: string;
}

export interface MovementCorridorRow {
  id: string;
  titleAr: string;
  season?: string;
  validFrom?: number;
  validTo?: number;
  status: string;
  certainty: string;
  sourceId: string;
  sourceTitle?: string;
  decided: boolean;
  stops: MovementCorridorStop[];
}

export interface SubjectCorridorsResponse {
  subjectType: string;
  subjectId: string;
  corridors: MovementCorridorRow[];
  limit: number;
  truncated: boolean;
}

/** One subject's recorded corridors (the ordered stops' own geometry, the
 * schema's own source rule), with the plan-C filters passed verbatim. */
export async function fetchSubjectCorridors(subjectType: string, subjectID: string, options: { season?: string; mode?: string; signal?: AbortSignal } = {}): Promise<SubjectCorridorsResponse> {
  const params = new URLSearchParams();
  if (options.season) params.set("season", options.season);
  if (options.mode) params.set("mode", options.mode);
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/subjects/${encodeURIComponent(subjectType)}/${encodeURIComponent(subjectID)}/corridors${suffix}`, {
    credentials: "include",
    signal: options.signal ? AbortSignal.any([AbortSignal.timeout(8000), options.signal]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SubjectCorridorsResponse;
}


/** The accepted candidate's own promotion (030-E's remaining surface): one
 * call writes the historical region row the candidate already earned, with
 * the reviewer's reading carried verbatim. Idempotent by the accepted-status
 * guard — a second promote of the same candidate returns the same row. */
export async function promoteRegionCandidate(candidateID: string): Promise<{ promoted: boolean; candidateId: string; regionId?: string; labelAr?: string; eraLabel?: string; note?: string }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates/${encodeURIComponent(candidateID)}/promote`, {
    method: "POST",
    credentials: "include",
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { promoted: boolean; candidateId: string; regionId?: string; labelAr?: string; eraLabel?: string; note?: string };
}

export interface AtlasSourceOccurrence {
  id: string;
  sourceId: string;
  itemKey?: string;
  scanSha256?: string;
  sourceSelector?: { figure?: { symbol: number }; legend?: { row: number }; pdfPage?: number; start?: string; end?: string };
  titleAr: string;
  original: string;
  displayAr: string;
  language: string;
  method: string;
  locatorAr: string;
  url?: string;
  creditAr?: string;
  rightsAr?: string;
  periodAr?: string;
  publicationYear?: number;
  depictedFrom?: number;
  depictedTo?: number;
  excerptAr?: string;
  noteAr?: string;
  tribeId?: string;
  placeId?: string;
  branchId?: string;
  branchName?: string;
  plateKey?: string;
  caption?: string;
}

export async function fetchAtlasOccurrences(input: {
  q?: string; tribe?: string; id?: string; after?: string; from?: number; to?: number; undated?: boolean;
}, signal?: AbortSignal): Promise<{ occurrences: AtlasSourceOccurrence[]; next?: string }> {
  const { id, ...filters } = input;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined) params.set(key, String(value));
  }
  const timeout = AbortSignal.timeout(8000);
  const path = id ? `/${encodeURIComponent(id)}` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/atlas/occurrences${path}?${params}`, {
    credentials: "include",
    signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return response.json();
}

export async function reviewAtlasOccurrenceIdentity(id: string, input: { tribeId?: string; placeId?: string; branchId?: string; noteAr: string }): Promise<void> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/atlas/occurrences/${encodeURIComponent(id)}/identity`, {
    method: "POST", credentials: "include", signal: AbortSignal.timeout(8000),
    headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
}

export interface AtlasReadingProposal {
  id: string;
  titleAr: string;
  locatorAr: string;
  url: string;
  sha256: string;
  method: string;
  engine: string;
  raw: { original: string; selector: Record<string, unknown> };
}

export async function fetchAtlasProposals(after?: string, signal?: AbortSignal): Promise<{ proposals: AtlasReadingProposal[]; next?: string }> {
  const params = new URLSearchParams(after ? { after } : {});
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/atlas/proposals?${params}`, { credentials: "include", signal });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return response.json();
}

export async function importAtlasProposals(sourceId: string, body: string): Promise<{ added: number }> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/atlas/sources/${encodeURIComponent(sourceId)}/proposals`, {
    method: "POST", credentials: "include", signal: AbortSignal.timeout(12_000),
    headers: { "Content-Type": "application/json" }, body,
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return response.json();
}

export async function decideAtlasProposal(id: string, input: {
  action: "publish" | "reject"; original: string; displayAr: string; language: string; noteAr: string;
}): Promise<void> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/atlas/proposals/${encodeURIComponent(id)}/decision`, {
    method: "POST", credentials: "include", signal: AbortSignal.timeout(8000),
    headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
}

export async function reviewAtlasBoundary(candidateId: string, input?: {
  sourceId: string; locator: string; wkt: string; validFrom: string; validTo: string; occurrenceId: string;
}): Promise<void> {
  const action = input ? "boundary" : "promote";
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/region-candidates/${encodeURIComponent(candidateId)}/${action}`, {
    method: "POST", credentials: "include", signal: AbortSignal.timeout(8000),
    headers: input ? { "Content-Type": "application/json" } : undefined,
    body: input ? JSON.stringify(input) : undefined,
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
}

export async function attachAtlasClaimReceipt(id: string, claimId: string, noteAr: string): Promise<void> {
  const response = await fetch(`${apiBaseUrl}/api/v1/admin/atlas/occurrences/${encodeURIComponent(id)}/claim-receipt`, {
    method: "POST", credentials: "include", signal: AbortSignal.timeout(8000),
    headers: { "Content-Type": "application/json" }, body: JSON.stringify({ claimId, noteAr }),
  });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
}

export async function fetchAtlasSourceRelationship(first: string, second: string, signal?: AbortSignal): Promise<{ status: string; kind?: string }> {
  const params = new URLSearchParams({ first, second });
  const response = await fetch(`${apiBaseUrl}/api/v1/atlas/source-relationship?${params}`, { credentials: "include", signal });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return response.json();
}
