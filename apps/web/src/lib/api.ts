import { demoDashboard, treeNodes } from "../data/demo";
import type { AddPersonInput, AddRelationshipInput, CreateTreeInput, DashboardData, TreeDetail, TreeSummary } from "../types";

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
  permissions: { canEdit: false, canPublish: false },
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
  if (!apiBaseUrl || treeId === demoTreeId) {
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
  if (!apiBaseUrl || treeId === demoTreeId) {
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
      permissions: { canEdit: false, canPublish: false },
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

async function readErrorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { error?: string };
    return body.error ?? "تعذر إكمال العملية.";
  } catch {
    return "تعذر إكمال العملية.";
  }
}
