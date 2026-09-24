import type { TreeNodeRecord, TreeRelationshipRecord } from "../types";

export interface LineageFocus {
  nodeIds: Set<string>;
  relationshipIds: Set<string>;
}

export function focusLineage(nodes: TreeNodeRecord[], relationships: TreeRelationshipRecord[], personId: string, maxDepth = 2): LineageFocus {
  const root = nodes.find((node) => node.personId === personId);
  const nodeIds = new Set<string>();
  const relationshipIds = new Set<string>();
  if (!root || maxDepth < 0) {
    return { nodeIds, relationshipIds };
  }

  const adjacency = new Map<string, string[]>();
  const addEdge = (from: string, to: string) => {
    const neighbors = adjacency.get(from) ?? [];
    neighbors.push(to);
    adjacency.set(from, neighbors);
  };
  for (const relationship of relationships) {
    if (relationship.predicate !== "parent_of") {
      continue;
    }
    addEdge(relationship.subjectNodeId, relationship.objectNodeId);
    addEdge(relationship.objectNodeId, relationship.subjectNodeId);
  }

  const queue: Array<{ id: string; depth: number }> = [{ id: root.id, depth: 0 }];
  while (queue.length > 0) {
    const current = queue.shift();
    if (!current || nodeIds.has(current.id)) {
      continue;
    }
    nodeIds.add(current.id);
    if (current.depth >= maxDepth) {
      continue;
    }
    for (const neighbor of adjacency.get(current.id) ?? []) {
      if (!nodeIds.has(neighbor)) {
        queue.push({ id: neighbor, depth: current.depth + 1 });
      }
    }
  }

  for (const relationship of relationships) {
    if (nodeIds.has(relationship.subjectNodeId) && nodeIds.has(relationship.objectNodeId)) {
      relationshipIds.add(relationship.id);
    }
  }
  return { nodeIds, relationshipIds };
}

export function filterUnresolvedRelationships(relationships: TreeRelationshipRecord[], showUnresolved: boolean): TreeRelationshipRecord[] {
  if (showUnresolved) {
    return relationships;
  }
  return relationships.filter((relationship) => relationship.status === "interpreted");
}
