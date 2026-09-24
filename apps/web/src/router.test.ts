import { createMemoryHistory } from "@tanstack/history";
import { createRouter } from "@tanstack/react-router";
import { describe, expect, it } from "vitest";
import { routeTree } from "./route-tree";

function makeRouter(path: string) {
  return createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [path] }),
  });
}

describe("tree routes", () => {
  it("builds addressable tree and version URLs", () => {
    const router = makeRouter("/tree");
    expect(router.buildLocation({ to: "/tree/$treeId", params: { treeId: "tree-1" } }).href).toBe("/tree/tree-1");
    expect(router.buildLocation({ to: "/tree/$treeId/versions/$versionId", params: { treeId: "tree-1", versionId: "version-2" } }).href).toBe("/tree/tree-1/versions/version-2");
  });

  it("builds an addressable invitation route", () => {
    const router = makeRouter("/invitation/token-1");
    expect(router.buildLocation({ to: "/invitation/$token", params: { token: "token-1" } }).href).toBe("/invitation/token-1");
  });

  it("builds the public dictionary route", () => {
    const router = makeRouter("/dictionary");
    expect(router.buildLocation({ to: "/dictionary" }).href).toBe("/dictionary");
  });

  it("keeps the static tree route separate from dynamic resources", () => {
    const router = makeRouter("/tree/tree-1/versions/version-2");
    expect(router.state.location.pathname).toBe("/tree/tree-1/versions/version-2");
  });
});
