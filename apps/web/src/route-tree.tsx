import { createRootRoute, createRoute } from "@tanstack/react-router";
import { AppShell } from "./components/AppShell";
import { ContradictionsPage } from "./routes/contradictions";
import { DictionaryPage } from "./components/DictionaryPage";
import { EntityResolutionPage } from "./routes/entity-resolution";
import { JobsPage } from "./components/JobsPage";
import { SearchPage } from "./components/SearchPage";
import { HomePage } from "./routes/index";
import { InvitationPage } from "./routes/invitation";
import { LoginPage } from "./routes/login";
import { PlacesPage } from "./routes/places";
import { QuestionsPage } from "./routes/questions";
import { ResearchPage } from "./routes/research";
import { ResearchWorkspacePage } from "./routes/research-workspace";
import { SourcesPage } from "./routes/sources";
import { TreePage } from "./routes/tree";
import { TreeByIdRoute, TreeVersionRoute } from "./routes/tree-route";

const rootRoute = createRootRoute({
  component: AppShell,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
});

const treeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tree",
  component: TreePage,
});

const treeByIdRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tree/$treeId",
  component: TreeByIdRoute,
});

const treeVersionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tree/$treeId/versions/$versionId",
  component: TreeVersionRoute,
});

type ResearchSearch = {
  entityType?: "person" | "family" | "branch";
  entityId?: string;
  treeId?: string;
  treeVersionId?: string;
};

function parseResearchSearch(search: Record<string, unknown>): ResearchSearch {
  const result: ResearchSearch = {};
  if (search.entityType === "person" || search.entityType === "family" || search.entityType === "branch") result.entityType = search.entityType;
  if (typeof search.entityId === "string") result.entityId = search.entityId;
  if (typeof search.treeId === "string") result.treeId = search.treeId;
  if (typeof search.treeVersionId === "string") result.treeVersionId = search.treeVersionId;
  return result;
}

const researchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/research",
  validateSearch: parseResearchSearch,
  component: ResearchPage,
});

const researchWorkspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/research/$questionId",
  validateSearch: parseResearchSearch,
  component: ResearchWorkspacePage,
});

const entityResolutionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/entity-resolution",
  component: EntityResolutionPage,
});

const contradictionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contradictions",
  component: ContradictionsPage,
});

const sourcesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sources",
  component: SourcesPage,
});

const placesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/places",
  component: PlacesPage,
});

const questionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/questions",
  component: QuestionsPage,
});

const dictionaryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/dictionary",
  component: DictionaryPage,
});

const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/search",
  component: SearchPage,
});

const jobsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/jobs",
  component: JobsPage,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
});

const invitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/invitation/$token",
  component: InvitationPage,
});

export const routeTree = rootRoute.addChildren([
  indexRoute,
  treeRoute,
  treeByIdRoute,
  treeVersionRoute,
  researchRoute,
  researchWorkspaceRoute,
  entityResolutionRoute,
  contradictionsRoute,
  sourcesRoute,
  placesRoute,
  questionsRoute,
  dictionaryRoute,
  searchRoute,
  jobsRoute,
  loginRoute,
  invitationRoute,
]);
