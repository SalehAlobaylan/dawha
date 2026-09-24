import { createRootRoute, createRoute } from "@tanstack/react-router";
import { AppShell } from "./components/AppShell";
import { DictionaryPage } from "./components/DictionaryPage";
import { HomePage } from "./routes/index";
import { InvitationPage } from "./routes/invitation";
import { LoginPage } from "./routes/login";
import { PlacesPage } from "./routes/places";
import { QuestionsPage } from "./routes/questions";
import { ResearchPage } from "./routes/research";
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

const researchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/research",
  component: ResearchPage,
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
  sourcesRoute,
  placesRoute,
  questionsRoute,
  dictionaryRoute,
  loginRoute,
  invitationRoute,
]);
