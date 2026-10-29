import { createRootRoute, createRoute, redirect } from "@tanstack/react-router";
import { AppShell } from "./components/AppShell";
import { DiscoveryPage } from "./routes/discovery";
import { HomePage } from "./routes/index";
import { InvitationPage } from "./routes/invitation";
import { LoginPage } from "./routes/login";
import { PlacesPage } from "./routes/places";
import { ResearchHubPage } from "./routes/research-hub";
import { parseResearchSearch, parseDiscoverySearch } from "./lib/research-navigation";
import { ResearchWorkspacePage } from "./routes/research-workspace";
import { SourcesPage } from "./routes/sources";
import { TreeByIdRoute, TreeVersionRoute } from "./routes/tree-route";
import { ChartImportPage } from "./routes/chart-import";
import { TreeLibraryPage } from "./routes/tree-library";
import { parseLibrarySearch, parseNameReviewSearch } from "./lib/library-search";
import { NameReviewPage } from "./routes/name-review";
import { ExplorePage } from "./routes/explore";
import { ChartOverviewPage } from "./routes/chart-overview";

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
  validateSearch: parseLibrarySearch,
  component: TreeLibraryPage,
});

const exploreRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/explore",
  validateSearch: (search: Record<string, unknown>): { q?: string } => ({ q: typeof search.q === "string" ? search.q || undefined : undefined }),
  component: ExplorePage,
});

const parseTreeOrigin = (search: Record<string, unknown>): { from?: "explore" } => ({ from: search.from === "explore" ? "explore" : undefined });

const namesRoute = createRoute({
  getParentRoute: () => rootRoute, path: "/names", validateSearch: parseLibrarySearch,
  beforeLoad: ({ search }) => { throw redirect({ to: "/tree", search: { q: search.q, view: "review", queue: search.queue }, replace: true }); },
});
const legacyNameReviewRoute = createRoute({
  getParentRoute: () => rootRoute, path: "/names/$fileId", validateSearch: parseNameReviewSearch,
  beforeLoad: ({ params, search }) => { throw redirect({ to: "/tree/charts/$fileId/names", params, search, replace: true }); },
});
const chartOverviewRoute = createRoute({ getParentRoute: () => rootRoute, path: "/tree/charts/$fileId", component: ChartOverviewPage });
const nameReviewRoute = createRoute({ getParentRoute: () => rootRoute, path: "/tree/charts/$fileId/names", validateSearch: parseNameReviewSearch, component: NameReviewPage });

const treeByIdRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tree/$treeId",
  validateSearch: parseTreeOrigin,
  component: TreeByIdRoute,
});

const treeVersionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tree/$treeId/versions/$versionId",
  validateSearch: parseTreeOrigin,
  component: TreeVersionRoute,
});

const researchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/research",
  validateSearch: parseResearchSearch,
  component: ResearchHubPage,
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
  validateSearch: parseResearchSearch,
  beforeLoad: ({ search }) => { throw redirect({ to: "/research", search: { ...search, view: "review", review: "identity" }, replace: true }); },
});

const contradictionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contradictions",
  validateSearch: parseResearchSearch,
  beforeLoad: ({ search }) => { throw redirect({ to: "/research", search: { ...search, view: "review", review: "contradictions" }, replace: true }); },
});

const sourcesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sources",
  validateSearch: (search: Record<string, unknown>): { sourceId?: string; view?: "processing" } => ({ sourceId: typeof search.sourceId === "string" ? search.sourceId : undefined, view: search.view === "processing" ? "processing" : undefined }),
  component: SourcesPage,
});

const chartImportRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/import",
  validateSearch: (search: Record<string, unknown>): { sourceId?: string; fileId?: string; draftId?: string } => {
    const sourceId = typeof search.sourceId === "string" && search.sourceId ? search.sourceId : undefined;
    return {
      sourceId,
      fileId: sourceId && typeof search.fileId === "string" && search.fileId ? search.fileId : undefined,
      draftId: sourceId && typeof search.draftId === "string" && search.draftId ? search.draftId : undefined,
    };
  },
  component: ChartImportPage,
});

const placesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/places",
  component: PlacesPage,
});

const questionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/questions",
  validateSearch: parseResearchSearch,
  beforeLoad: ({ search }) => { throw redirect({ to: "/research", search: { ...search, view: undefined }, replace: true }); },
});

const dictionaryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/dictionary",
  validateSearch: parseDiscoverySearch,
  beforeLoad: ({ search }) => { throw redirect({ to: "/search", search: { ...search, view: "index" }, replace: true }); },
});

const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/search",
  validateSearch: parseDiscoverySearch,
  component: DiscoveryPage,
});

const jobsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/jobs",
  beforeLoad: () => { throw redirect({ to: "/sources", search: { view: "processing" }, replace: true }); },
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
  exploreRoute,
  treeByIdRoute,
  treeVersionRoute,
  researchRoute,
  researchWorkspaceRoute,
  entityResolutionRoute,
  contradictionsRoute,
  sourcesRoute,
  chartImportRoute,
  namesRoute,
  legacyNameReviewRoute,
  chartOverviewRoute,
  nameReviewRoute,
  placesRoute,
  questionsRoute,
  dictionaryRoute,
  searchRoute,
  jobsRoute,
  loginRoute,
  invitationRoute,
]);
