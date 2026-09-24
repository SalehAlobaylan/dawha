import { describe, expect, it } from "vitest";
import type { TreeNodeRecord, TreeRelationshipRecord } from "../types";
import { filterUnresolvedRelationships, focusLineage } from "./tree-view";

const nodes: TreeNodeRecord[] = [
  { id: "a", personId: "person-a", displayName: "أ", sortOrder: 0, years: "", role: "", tone: "interpretation", sourceCount: 0, note: "" },
  { id: "b", personId: "person-b", displayName: "ب", sortOrder: 1, years: "", role: "", tone: "interpretation", sourceCount: 0, note: "" },
  { id: "c", personId: "person-c", displayName: "ج", sortOrder: 2, years: "", role: "", tone: "interpretation", sourceCount: 0, note: "" },
  { id: "d", personId: "person-d", displayName: "د", sortOrder: 3, years: "", role: "", tone: "interpretation", sourceCount: 0, note: "" },
];

const relationships: TreeRelationshipRecord[] = [
  { id: "ab", subjectNodeId: "a", objectNodeId: "b", predicate: "parent_of", status: "interpreted" },
  { id: "bc", subjectNodeId: "b", objectNodeId: "c", predicate: "parent_of", status: "unresolved" },
  { id: "ad", subjectNodeId: "a", objectNodeId: "d", predicate: "parent_of", status: "disputed" },
  { id: "cycle", subjectNodeId: "b", objectNodeId: "a", predicate: "parent_of", status: "interpreted" },
];

describe("tree view helpers", () => {
  it("focuses a cycle-safe two-generation lineage in both directions", () => {
    const focus = focusLineage(nodes, relationships, "person-b", 2);
    expect([...focus.nodeIds].sort()).toEqual(["a", "b", "c", "d"]);
    expect(focus.relationshipIds.has("ab")).toBe(true);
    expect(focus.relationshipIds.has("bc")).toBe(true);
    expect(focus.relationshipIds.has("ad")).toBe(true);
  });

  it("returns no focus for a missing person and respects depth zero", () => {
    expect(focusLineage(nodes, relationships, "missing").nodeIds.size).toBe(0);
    expect([...focusLineage(nodes, relationships, "person-b", 0).nodeIds]).toEqual(["b"]);
  });

  it("removes unresolved edges when the filter is disabled", () => {
    expect(filterUnresolvedRelationships(relationships, false).map((relationship) => relationship.id)).toEqual(["ab", "cycle"]);
    expect(filterUnresolvedRelationships(relationships, true)).toHaveLength(4);
  });
});
