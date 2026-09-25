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
  OpenQuestionRecord,
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
  SourceDetail,
  SourceCharacterizationInput,
  SourceCharacterizationRun,
  ReviewSourceCharacterizationInput,
  SourceDependencyGraph,
  SourceFile,
  SourceMetadata,
  SourceProcessing,
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
} from "../types";

const apiBaseUrl = import.meta.env.VITE_API_URL ?? "";
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

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export async function fetchDashboard(): Promise<DashboardData> {
  if (!apiBaseUrl) {
    return demoDashboard;
  }

  try {
    const response = await fetch(`${apiBaseUrl}/api/v1/dashboard`, { signal: AbortSignal.timeout(2500) });
    if (!response.ok) {
      throw new Error(`Dashboard request failed with ${response.status}`);
    }
    return (await response.json()) as DashboardData;
  } catch {
    return { ...demoDashboard, mode: "demo" };
  }
}

export async function queryResearch(input: ResearchQueryInput): Promise<ResearchQueryResult> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API وخدمة البحث لتفعيل الاستعلام.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API لتفعيل تحليل أثر العلاقة.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API لتفعيل مقارنة بنية الفروع.", 503);
  }
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

export async function fetchResearchWorkspace(input: ResearchWorkspaceInput): Promise<ResearchWorkspaceSnapshot> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API لعرض مساحة البحث.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API لعرض سجل التحقيقات.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/research/questions/${encodeURIComponent(questionId)}/runs`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: ResearchRunSummary[] };
  return payload.items ?? [];
}

export async function fetchResearchRun(runId: string): Promise<ResearchRunDetail> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API لعرض تفاصيل التحقيق.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/research/runs/${encodeURIComponent(runId)}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchRunDetail;
}

export async function runEntityResolution(entityType: EntityResolutionEntityType): Promise<EntityResolutionRun> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API وخدمة مطابقة الهوية.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/runs/${runId}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as EntityResolutionRun;
}

export async function fetchEntityResolutionCandidates(status?: string, entityType?: string): Promise<EntityResolutionCandidate[]> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/entity-resolution/merges`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: EntityResolutionMerge[] };
  return payload.items ?? [];
}

export async function reviewEntityResolutionCandidate(candidateId: string, input: EntityResolutionReviewInput): Promise<EntityResolutionCandidate> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API وخدمة فحص التعارضات.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/contradictions/runs`, { method: "POST", credentials: "include" });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ContradictionRun;
}

export async function fetchContradictionRun(runId: string): Promise<ContradictionRun> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/contradictions/runs/${runId}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ContradictionRun;
}

export async function fetchContradictionFindings(runId?: string, status?: string): Promise<ContradictionFinding[]> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتشغيل التحليل الزمني.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/temporal-analysis/runs/${runId}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as TemporalAnalysisRun;
}

export async function fetchLatestTemporalAnalysisRun(query: TemporalAnalysisRunQuery): Promise<TemporalAnalysisRun | null> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    return [demoTreeDetail.tree];
  }

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

export async function fetchTree(treeId: string): Promise<TreeDetail> {
  if (treeId === demoTreeId) {
    return demoTreeDetail;
  }
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API لفتح هذه الشجرة.", 503);
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API لفتح هذه النسخة.", 503);
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتفعيل إنشاء الشجرة.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض المهام.", 503);
  }
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs${query}`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: JobView[] };
  return payload.items ?? [];
}

export async function enqueueJob(input: EnqueueJobInput): Promise<{ job: JobView; created: boolean }> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لإضافة المهمة.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as { job: JobView; created: boolean };
}

export async function claimJob(workerId: string): Promise<JobView> {
  if (!apiBaseUrl) throw new ApiError("شغّل Core API أولاً لالتقاط المهمة.", 503);
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/claim`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ worker_id: workerId }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as JobView;
}

export async function completeJob(jobId: string, workerId: string): Promise<JobView> {
  if (!apiBaseUrl) throw new ApiError("شغّل Core API أولاً لإكمال المهمة.", 503);
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/${jobId}/complete`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ worker_id: workerId }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as JobView;
}

export async function failJob(jobId: string, workerId: string, error: string): Promise<JobView> {
  if (!apiBaseUrl) throw new ApiError("شغّل Core API أولاً لتسجيل فشل المهمة.", 503);
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/${jobId}/fail`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ worker_id: workerId, error }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as JobView;
}

export async function recoverStaleJobs(olderThanSeconds = 900): Promise<{ recovered: number; jobs: JobView[] }> {
  if (!apiBaseUrl) throw new ApiError("شغّل Core API أولاً لاستعادة المهام العالقة.", 503);
  const response = await fetch(`${apiBaseUrl}/api/v1/jobs/recover-stale`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ older_than_seconds: olderThanSeconds }) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as { recovered: number; jobs: JobView[] };
}

export async function fetchSearch(filters: { q: string; kind?: string; status?: string; personId?: string; placeId?: string; sourceId?: string; entityId?: string; fromYear?: number; toYear?: number; limit?: number; signal?: AbortSignal }): Promise<SearchResponse> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لبدء البحث.", 503);
  }
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

export async function fetchMapFeatures(filters: { fromYear?: number; toYear?: number; status?: string; placeId?: string } = {}): Promise<MapResponse> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض طبقات الخريطة.", 503);
  }
  const params = new URLSearchParams();
  if (filters.fromYear !== undefined) params.set("from_year", String(filters.fromYear));
  if (filters.toYear !== undefined) params.set("to_year", String(filters.toYear));
  if (filters.status) params.set("status", filters.status);
  if (filters.placeId) params.set("place_id", filters.placeId);
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await fetch(`${apiBaseUrl}/api/v1/map${suffix}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as MapResponse;
}

export async function startGeospatialIntelligence(input: StartGeospatialIntelligenceInput): Promise<GeospatialIntelligenceRun> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتشغيل التحليل الجغرافي.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/geospatial-intelligence/runs`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as GeospatialIntelligenceRun;
}

export async function fetchLatestGeospatialIntelligenceRun(query: { question_id?: string; entity_type?: string; entity_id?: string; tree_version_id?: string }): Promise<GeospatialIntelligenceRun | null> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) if (value) params.set(key, value);
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/geospatial-intelligence/runs/latest${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(5000) });
  if (response.status === 404) return null;
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as GeospatialIntelligenceRun;
}

export async function fetchGeospatialFindings(runId?: string, status?: string): Promise<GeospatialFinding[]> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/geospatial-intelligence/findings/${findingId}/review`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as GeospatialFinding;
}

export async function startResearchAgent(input: StartResearchAgentInput): Promise<ResearchAgentRun> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتشغيل وكيل البحث.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/${encodeURIComponent(runId)}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ResearchAgentRun;
}

export async function fetchLatestResearchAgentRun(query: ResearchAgentRunQuery): Promise<ResearchAgentRun | null> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) if (value) params.set(key, value);
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/latest${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (response.status === 404) return null;
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ResearchAgentRun;
}

export async function generateResearchQuestionCandidates(runId: string): Promise<ResearchQuestionCandidate[]> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/${encodeURIComponent(runId)}/question-candidates`, { method: "POST", credentials: "include", signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const payload = (await response.json()) as { items?: ResearchQuestionCandidate[] };
  return payload.items ?? [];
}

export async function fetchResearchQuestionCandidates(runId: string, status?: string): Promise<ResearchQuestionCandidate[]> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  const suffix = params.toString();
  const response = await fetch(`${apiBaseUrl}/api/v1/research-agent/runs/${encodeURIComponent(runId)}/question-candidates${suffix ? `?${suffix}` : ""}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  const payload = (await response.json()) as { items?: ResearchQuestionCandidate[] };
  return payload.items ?? [];
}

export async function reviewResearchQuestionCandidate(candidateId: string, input: ReviewResearchQuestionCandidateInput): Promise<ResearchQuestionCandidate> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/research-question-candidates/${encodeURIComponent(candidateId)}`, { method: "PATCH", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new ApiError(await readErrorMessage(response), response.status);
  return (await response.json()) as ResearchQuestionCandidate;
}

export async function fetchDictionaryIndex(kind: DictionaryKind, query = ""): Promise<DictionaryIndexResponse> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض الفهرس.", 503);
  }
  const params = new URLSearchParams({ kind, q: query });
  const response = await fetch(`${apiBaseUrl}/api/v1/dictionary?${params.toString()}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DictionaryIndexResponse;
}

export async function fetchDictionaryDetail(kind: "families" | "tribes" | "people" | "places", id: string): Promise<DictionaryDetail> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض صفحة القاموس.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/dictionary/${kind}/${id}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as DictionaryDetail;
}

export async function submitSuggestion(input: SubmitSuggestionInput): Promise<SuggestionRecord> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لإرسال المقترح.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض طابور المراجعة.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/trees/${treeId}/suggestions`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: SuggestionRecord[] };
  return payload.items ?? [];
}

export async function reviewSuggestion(suggestionId: string, input: ReviewSuggestionInput): Promise<SuggestionRecord> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لحفظ قرار المراجعة.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/suggestions/${suggestionId}`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SuggestionRecord;
}

export async function fetchQuestions(): Promise<OpenQuestionRecord[]> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض الأسئلة.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/questions`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: OpenQuestionRecord[] };
  return payload.items ?? [];
}

export async function fetchQuestion(questionId: string): Promise<QuestionDetail> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض السؤال.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/questions/${questionId}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as QuestionDetail;
}

export async function createQuestion(input: CreateQuestionInput): Promise<QuestionDetail> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لحفظ السؤال.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتحديث السؤال.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لإضافة ملاحظة.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لربط الادعاء.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لربط المصدر.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لربط الخلاف.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لربط الكيان.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لربط الملاحظة.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض الخلافات.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/disputes`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: DisputeRecord[] };
  return payload.items ?? [];
}

export async function createDispute(input: CreateDisputeInput): Promise<DisputeDetail> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لإنشاء خلاف.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتحديث الخلاف.", 503);
  }
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

export async function linkDisputeClaim(disputeId: string, input: DisputeClaimInput): Promise<DisputeDetail> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لربط ادعاء بالخلاف.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض مكتبة المصادر.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/sources`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: SourceMetadata[] };
  return payload.items ?? [];
}

export async function fetchSource(sourceId: string): Promise<SourceDetail> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض المصدر.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDetail;
}

export async function fetchSourceDependencies(sourceId: string): Promise<SourceDependencyGraph> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض اعتماد المصادر.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/dependencies`, { credentials: "include", signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDependencyGraph;
}

export async function createSourceDependency(sourceId: string, input: CreateSourceDependencyInput): Promise<SourceDependencyGraph> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لإضافة اعتماد مصدر.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لفحص اعتماد المصادر.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لمراجعة اعتماد المصدر.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتوصيف المصدر.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض توصيف المصدر.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/source-characterization/runs/${encodeURIComponent(runId)}`, { credentials: "include", signal: AbortSignal.timeout(7000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceCharacterizationRun;
}

export async function fetchLatestSourceCharacterization(input: SourceCharacterizationInput): Promise<SourceCharacterizationRun | null> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض آخر توصيف.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لمراجعة التوصيف.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض معالجة المصدر.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}/processing`, {
    credentials: "include",
    signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceProcessing;
}

export async function uploadSourceFile(sourceId: string, file: File): Promise<SourceFile> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لرفع الملف.", 503);
  }
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

export async function reviewSourceCandidate(candidateId: string, decision: "accepted" | "rejected", noteAr?: string): Promise<SourceCandidate> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لمراجعة المرشح.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لإضافة مصدر.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لإضافة مقطع.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لتسجيل عبارة مصدر.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض الادعاءات.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/claims`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  const payload = (await response.json()) as { items?: ClaimSummary[] };
  return payload.items ?? [];
}

export async function fetchClaim(claimId: string): Promise<ResearchClaim> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض الادعاء.", 503);
  }
  const response = await fetch(`${apiBaseUrl}/api/v1/claims/${claimId}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as ResearchClaim;
}

export async function createClaim(input: CreateClaimInput): Promise<ResearchClaim> {
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لحفظ ادعاء.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لربط الدليل.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لعرض الدعوات.", 503);
  }
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
  if (!apiBaseUrl) {
    throw new ApiError("شغّل Core API أولاً لقبول الدعوة.", 503);
  }
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
