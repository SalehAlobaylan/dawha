import { useParams, useSearch } from "@tanstack/react-router";
import { TreePage } from "./tree";

export function TreeByIdRoute() {
  const { treeId } = useParams({ from: "/tree/$treeId" });
  const { from } = useSearch({ from: "/tree/$treeId" });
  return <TreePage routeTreeId={treeId} browseMode={from === "explore"} />;
}

export function TreeVersionRoute() {
  const { treeId, versionId } = useParams({ from: "/tree/$treeId/versions/$versionId" });
  const { from } = useSearch({ from: "/tree/$treeId/versions/$versionId" });
  return <TreePage routeTreeId={treeId} routeVersionId={versionId} browseMode={from === "explore"} />;
}
