import type { DictionaryKind } from "../types";

export const dictionaryKinds: DictionaryKind[] = ["people", "families", "tribes", "branches", "places", "sources", "questions", "disputed-claims"];
export type ResearchSearch = {
  view?: "evidence" | "review";
  review?: "identity" | "contradictions";
  entityType?: "person" | "family" | "branch";
  entityId?: string;
  treeId?: string;
  treeVersionId?: string;
  q?: string;
  create?: boolean;
  runId?: string;
  filter?: "high" | "under_investigation" | "open" | "resolved" | "archived";
  panel?: "context" | "analysis" | "history" | "manage";
};
export function parseResearchSearch(search: Record<string, unknown>): ResearchSearch {
  return {
    view: search.view === "review" || search.view === "evidence" ? search.view : typeof search.entityId === "string" && search.entityId ? "evidence" : undefined,
    review: search.review === "identity" || search.review === "contradictions" ? search.review : undefined,
    entityType: search.entityType === "person" || search.entityType === "family" || search.entityType === "branch" ? search.entityType : undefined,
    entityId: stringValue(search.entityId), treeId: stringValue(search.treeId), treeVersionId: stringValue(search.treeVersionId),
    filter: search.filter === "high" || search.filter === "under_investigation" || search.filter === "open" || search.filter === "resolved" || search.filter === "archived" ? search.filter : undefined,
    q: stringValue(search.q), runId: stringValue(search.runId), create: search.create === true || search.create === "true" ? true : undefined,
    panel: search.panel === "context" || search.panel === "analysis" || search.panel === "history" || search.panel === "manage" ? search.panel : undefined,
  };
}
export type DiscoverySearch = { q?: string; view?: "index"; kind?: string; id?: string; status?: string; claimId?: string };
export function parseDiscoverySearch(search: Record<string, unknown>): DiscoverySearch {
  return { q: stringValue(search.q), view: search.view === "index" ? "index" : undefined, kind: stringValue(search.kind), id: stringValue(search.id), status: stringValue(search.status), claimId: stringValue(search.claimId) };
}
function stringValue(value: unknown) { return typeof value === "string" && value.trim() ? value.trim() : undefined; }
