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

  it("builds the job operations route", () => {
    const router = makeRouter("/jobs");
    expect(router.buildLocation({ to: "/jobs" }).href).toBe("/jobs");
  });

  it("builds the public search route", () => {
    const router = makeRouter("/search");
    expect(router.buildLocation({ to: "/search" }).href).toBe("/search");
  });

  it("builds the entity resolution route", () => {
    const router = makeRouter("/entity-resolution");
    expect(router.buildLocation({ to: "/entity-resolution" }).href).toBe("/entity-resolution");
  });

  it("builds the contradiction review route", () => {
    const router = makeRouter("/contradictions");
    expect(router.buildLocation({ to: "/contradictions" }).href).toBe("/contradictions");
  });

  it("builds the public dictionary route", () => {
    const router = makeRouter("/dictionary");
    expect(router.buildLocation({ to: "/dictionary" }).href).toBe("/dictionary");
  });

  it("keeps the static tree route separate from dynamic resources", () => {
    const router = makeRouter("/tree/tree-1/versions/version-2");
    expect(router.state.location.pathname).toBe("/tree/tree-1/versions/version-2");
  });

  it("builds contextual research workspace URLs", () => {
    const router = makeRouter("/research");
    expect(router.buildLocation({ to: "/research/$questionId", params: { questionId: "question-1" }, search: { entityType: "person", entityId: "person-1" } }).href).toBe("/research/question-1?entityType=person&entityId=person-1");
    expect(router.buildLocation({ to: "/research", search: { entityType: "person", entityId: "person-1" } }).href).toBe("/research?entityType=person&entityId=person-1");
  });
});
