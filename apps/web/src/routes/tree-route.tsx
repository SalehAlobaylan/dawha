import { useParams } from "@tanstack/react-router";
import { TreePage } from "./tree";

export function TreeByIdRoute() {
  const { treeId } = useParams({ from: "/tree/$treeId" });
  return <TreePage routeTreeId={treeId} />;
}

export function TreeVersionRoute() {
  const { treeId, versionId } = useParams({ from: "/tree/$treeId/versions/$versionId" });
  return <TreePage routeTreeId={treeId} routeVersionId={versionId} />;
}
