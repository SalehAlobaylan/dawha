import { createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { AppShell } from "./components/AppShell";
import { HomePage } from "./routes/index";
import { LoginPage } from "./routes/login";
import { PlacesPage } from "./routes/places";
import { QuestionsPage } from "./routes/questions";
import { ResearchPage } from "./routes/research";
import { SourcesPage } from "./routes/sources";
import { TreePage } from "./routes/tree";

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

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  treeRoute,
  researchRoute,
  sourcesRoute,
  placesRoute,
  questionsRoute,
  loginRoute,
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
