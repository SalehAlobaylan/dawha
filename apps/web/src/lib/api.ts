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
  CreateTreeInput,
  DisputeClaimInput,
  DisputeDetail,
  DisputeRecord,
  DashboardData,
  DictionaryDetail,
  DictionaryIndexResponse,
  DictionaryKind,
  ForkTreeInput,
  InvitationCreated,
  MapResponse,
  OpenQuestionRecord,
  QuestionClaimInput,
  QuestionDetail,
  QuestionDisputeInput,
  QuestionNoteInput,
  QuestionSourceInput,
  ReviewSuggestionInput,
  ResearchClaim,
  SourceDetail,
  SourceMetadata,
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
  const response = await fetch(`${apiBaseUrl}/api/v1/sources`, { signal: AbortSignal.timeout(3000) });
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
  const response = await fetch(`${apiBaseUrl}/api/v1/sources/${sourceId}`, { signal: AbortSignal.timeout(3000) });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response), response.status);
  }
  return (await response.json()) as SourceDetail;
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
