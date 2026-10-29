// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { routeTree } from "../route-tree";
import * as api from "../lib/api";
import { demoDashboard } from "../data/demo";
import type { ChartLibraryItem, TreeSummary } from "../types";

vi.mock("../lib/api", async (original) => ({ ...await original<typeof import("../lib/api")>(), fetchChartLibrary: vi.fn(), fetchPublicTrees: vi.fn(), fetchWorkspaceTrees: vi.fn(), fetchPublishedTrees: vi.fn(), fetchSessionStatus: vi.fn(), fetchLatestNameReadingRun: vi.fn(), fetchDashboard: vi.fn(), fetchSources: vi.fn(), readSourceTreeSuggestions: vi.fn(), createSourceTreeDraft: vi.fn() }));
vi.mock("../components/SourceSuggestionReview", () => ({ SourceSuggestionReview: ({ fileId, onClose, onContinue, initialQueueMode }: { fileId: string; onClose: () => void; onContinue?: () => void; initialQueueMode?: string }) => <div role="dialog" aria-label="مراجعة الأسماء" data-queue={initialQueueMode}>{fileId}<button onClick={onClose}>إغلاق المراجعة</button><button onClick={onContinue}>الوصلات والشجرة</button></div> }));
const charts: ChartLibraryItem[] = Array.from({ length: 8 }, (_, i) => ({ sourceId: `source-${i}`, fileId: `file-${i}`, titleAr: `مشجر الأسرة ${i + 1}`, filename: `chart-${i + 1}.pdf`, visibility: "private", canReview: true, updatedAt: "2026-10-28", processingStatus: "succeeded", totalNames: 10, pendingNames: i === 7 ? 0 : 8, deferredNames: i === 7 ? 2 : 0, reviewedNames: 2, draftId: i === 7 ? "saved-draft" : undefined }));
const tree = { id: "digital-1", name: "شجرة رقمية محفوظة", description: "", visibility: "private", latestState: "draft", people: 4, relationships: 3 } as TreeSummary;
let host: HTMLDivElement; let root: Root | undefined; let client: QueryClient;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true); vi.stubGlobal("scrollTo", vi.fn());
  vi.mocked(api.fetchChartLibrary).mockResolvedValue(charts);
  vi.mocked(api.fetchDashboard).mockResolvedValue(demoDashboard);
  vi.mocked(api.fetchSources).mockResolvedValue([]);
  vi.mocked(api.fetchPublicTrees).mockResolvedValue([tree]);
  vi.mocked(api.fetchWorkspaceTrees).mockResolvedValue([tree]);
  vi.mocked(api.fetchPublishedTrees).mockResolvedValue([{ ...tree, id: "public-1", name: "شجرة عامة منشورة", visibility: "public", latestState: "published", latestVersionId: "sealed-1", latestVersionNumber: 1 }]);
  vi.mocked(api.fetchSessionStatus).mockResolvedValue(true);
  vi.mocked(api.fetchLatestNameReadingRun).mockResolvedValue(null);
});
afterEach(async () => { if (root) await act(async () => root?.unmount()); root = undefined; client?.clear(); host?.remove(); vi.resetAllMocks(); vi.unstubAllGlobals(); });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 20)); }); }
async function render(path: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => root!.render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>));
  await settle(); await settle(); return router;
}
function button(label: string) { return [...host.querySelectorAll("button")].find((item) => item.textContent?.includes(label))!; }

it("lists every uploaded chart and digital tree instead of silently opening one tree", async () => {
  await render("/tree");
  expect(host.querySelectorAll(".tree-library-row")).toHaveLength(9);
  expect(host.textContent).toContain("مشجر الأسرة 8");
  expect(host.querySelectorAll('a[href^="/tree/charts/file-"][href*="/names"]')).toHaveLength(8);
  expect(host.querySelector('a[href="/tree/charts/file-7"]')).not.toBeNull();
  expect(host.querySelector('a[href="/tree/digital-1"]')).not.toBeNull();
  expect(host.querySelector('.sidebar a[href="/names"]')).toBeNull();
});

it("preserves the original landing presentation with direct access to saved work", async () => {
  await render("/");
  expect(host.querySelector(".home-hero")).not.toBeNull();
  expect(host.querySelector(".hero-visual .hero-visual-symbol")?.textContent).toBe("؟");
  expect(host.querySelector(".hero-copy h2")?.textContent).toContain("السلالةُ لا تُقرأ");
  expect([...host.querySelectorAll(".hero-actions a")].map((link) => link.getAttribute("href"))).toEqual(["/tree", "/places"]);
  expect(host.querySelector(".metrics-grid")).not.toBeNull();
  expect(host.querySelector(".research-invite")).not.toBeNull();
  expect(host.querySelector(".site-footer")).not.toBeNull();
  expect(host.querySelectorAll(".tree-library-row")).toHaveLength(0);
  expect(host.querySelector('.sidebar a[href="/names"]')).toBeNull();
  expect(host.querySelector('a[href="/tree"]')).not.toBeNull();
});

it("keeps the landing artwork visible when dashboard data is unavailable", async () => {
  vi.mocked(api.fetchDashboard).mockRejectedValue(new Error("offline"));
  await render("/");
  expect(host.querySelector(".home-hero")).not.toBeNull();
  expect(host.querySelector(".dashboard-unavailable")).not.toBeNull();
  expect(host.querySelector(".metrics-grid")).toBeNull();
});

it("preserves the Sources presentation around the working evidence workspace", async () => {
  await render("/sources");
  expect(host.querySelectorAll(".source-stack-visual .source-sheet")).toHaveLength(3);
  expect(host.querySelector(".source-library-copy h2")?.textContent).toContain("لا نخلط بين النص");
  expect(host.querySelector(".source-library-section")).not.toBeNull();
  expect(host.querySelector(".source-principles")).not.toBeNull();
  expect(host.textContent).toContain("المكتبة المحفوظة");
  expect(host.querySelector('.source-library-hero a[href="/import"]')).not.toBeNull();
});

it("redirects the old names index into a distinct task queue inside My trees", async () => {
  vi.mocked(api.fetchChartLibrary).mockResolvedValue([...charts, { ...charts[0], sourceId: "completed", fileId: "completed", titleAr: "مشجر مكتمل", pendingNames: 0, deferredNames: 0, reviewedNames: 10 }]);
  const router = await render("/names");
  expect(router.state.location.pathname).toBe("/tree");
  expect(router.state.location.search).toMatchObject({ view: "review" });
  expect(host.querySelector('h1')?.textContent).toBe('أشجاري');
  expect(host.querySelectorAll(".review-queue-row")).toHaveLength(8);
  expect(host.textContent).not.toContain(tree.name);
  expect(host.textContent).not.toContain('مشجر مكتمل');
  expect(host.querySelector('.review-queue-row a[href^="/import"]')).toBeNull();
  await act(async () => button("المؤجلة").click()); await settle();
  expect(router.state.location.search).toMatchObject({ view: "review", queue: "deferred" });
  expect(host.querySelectorAll('.review-queue-row')).toHaveLength(1);
  expect(host.textContent).toContain("مشجر الأسرة 8");
  expect(host.querySelector('.review-queue-row-actions a, .review-queue-row .primary-button')?.getAttribute('href')).toBe('/tree/charts/file-7/names?queue=deferred');
});

it("opens old file bookmarks in the inner review and closes to the chart overview", async () => {
  const router = await render("/names/file-7?queue=deferred");
  expect(router.state.location.pathname).toBe('/tree/charts/file-7/names');
  expect(host.querySelector('[role="dialog"]')?.textContent).toContain("file-7");
  expect(host.querySelector('[role="dialog"]')?.getAttribute('data-queue')).toBe('deferred');
  await act(async () => button("إغلاق المراجعة").click()); await settle();
  expect(router.state.location.pathname).toBe("/tree/charts/file-7");
  expect(host.querySelector('[role="dialog"]')).toBeNull();
  expect(host.querySelector('.chart-workspace-next h2')?.textContent).toContain('تابع مراجعة الأسماء');
  await act(async () => (host.querySelector('.chart-workspace-next a') as HTMLAnchorElement).click()); await settle();
  expect(host.querySelector('[role="dialog"]')).not.toBeNull();
  expect(host.querySelector('[role="dialog"]')?.getAttribute('data-queue')).toBe('deferred');
});

it("gives a chart its own overview, history, saved connections and source without starting work", async () => {
  await render('/tree/charts/file-7');
  expect(host.querySelector('h1')?.textContent).toBe('مشجر الأسرة 8');
  expect(host.querySelectorAll('.chart-workspace-tabs a')).toHaveLength(4);
  expect(host.querySelector('.chart-workspace-tabs a[href="/tree/charts/file-7/names"]')).not.toBeNull();
  expect(host.querySelector('a[href="/tree/charts/file-7/names?queue=history"]')).not.toBeNull();
  expect(host.querySelector('a[href="/import?sourceId=source-7&fileId=file-7&draftId=saved-draft"]')).not.toBeNull();
  expect(host.querySelector('a[href="/sources?sourceId=source-7"]')).not.toBeNull();
  expect(api.readSourceTreeSuggestions).not.toHaveBeenCalled();
  expect(api.createSourceTreeDraft).not.toHaveBeenCalled();
});

it("keeps completed and unprepared charts in all work, and digital trees in their own view", async () => {
  vi.mocked(api.fetchChartLibrary).mockResolvedValue([
    { ...charts[0], pendingNames: 0, deferredNames: 0, reviewedNames: 10 },
    { ...charts[1], pendingNames: 0, deferredNames: 0, totalNames: 0, reviewedNames: 0 },
  ]);
  const router = await render('/tree?view=review');
  expect(host.querySelectorAll('.tree-library-row')).toHaveLength(0);
  expect(host.textContent).toContain('لا توجد أسماء بانتظار المراجعة');
  await act(async () => router.navigate({ to: '/tree' })); await settle();
  expect(host.querySelectorAll('.tree-library-row')).toHaveLength(3);
  expect(host.querySelector('a[href="/tree/charts/file-0/names?queue=history"]')).not.toBeNull();
  await act(async () => router.navigate({ to: '/tree', search: { view: 'trees' } })); await settle();
  expect(host.querySelectorAll('.tree-library-row')).toHaveLength(1);
  expect(host.textContent).toContain(tree.name);
  expect(host.textContent).not.toContain(charts[0].titleAr);
});

it("requires explicit preparation for a chart without names", async () => {
  vi.mocked(api.fetchChartLibrary).mockResolvedValue([{ ...charts[0], totalNames: 0, pendingNames: 0, deferredNames: 0, reviewedNames: 0 }]);
  await render('/tree/charts/file-0/names');
  expect(host.querySelector('[role="dialog"]')).toBeNull();
  expect(host.textContent).toContain('إعداد اقتراحات الأسماء');
  expect(api.readSourceTreeSuggestions).not.toHaveBeenCalled();
});

it("explains sign-in instead of showing a misleading empty library", async () => {
  vi.mocked(api.fetchSessionStatus).mockResolvedValue(false);
  vi.mocked(api.fetchChartLibrary).mockResolvedValue([{ ...charts[0], fileId: undefined, canReview: false, pendingNames: 0, totalNames: 0 }]);
  await render("/tree");
  expect(host.textContent).toContain("سجّل الدخول للوصول إلى عملك المحفوظ");
  expect(host.querySelectorAll(".tree-library-row")).toHaveLength(0);
  expect(host.querySelector('.tree-library-empty a[href="/explore"]')).not.toBeNull();
  expect(host.querySelector('a[href="/names/file-0"]')).toBeNull();
});

it("does not hide the digital trees when the chart index fails", async () => {
  vi.mocked(api.fetchChartLibrary).mockRejectedValue(new Error("انقطع الاتصال بالمشجرات"));
  await render("/tree");
  expect(host.querySelector('[role="alert"]')?.textContent).toContain("تعذر تحميل جزء من المكتبة");
  expect(host.textContent).toContain(tree.name);
  expect(host.textContent).not.toContain("لم يُحفظ عمل هنا بعد");
});


it("orders pages by workspace, discovery, then research, with direct name review", async () => {
  await render("/tree");
  const top = host.querySelector('.sidebar-workspace')!;
  expect([...top.querySelectorAll('a:not(.nav-child)')].map((link) => link.getAttribute('href'))).toEqual(['/', '/tree', '/places']);
  expect(top.querySelectorAll('a')).toHaveLength(3);
  expect(host.querySelector('.sidebar-workspace a[href="/tree"]')?.classList.contains('nav-link-active')).toBe(true);
  expect([...host.querySelectorAll('.sidebar-discovery a')].map((link) => link.getAttribute('href'))).toEqual(['/explore', '/sources']);
  expect(host.querySelector('.sidebar-research details')).toBeNull();
  expect([...host.querySelectorAll('.sidebar-research nav a')].map((link) => link.getAttribute('href'))).toEqual(['/research', '/search']);
  expect(host.querySelector('.sidebar-research a[href="/places"]')).toBeNull();
});

it("keeps review inside My trees instead of adding another primary destination", async () => {
  await render("/names/file-7");
  expect(host.querySelector('.sidebar-workspace a[href="/tree"]')?.classList.contains('nav-link-active')).toBe(true);
  expect(host.querySelector('.sidebar a[href="/names"]')).toBeNull();
  expect(host.querySelector('.chart-workspace-tabs .is-current')?.textContent).toBe('مراجعة الأسماء');
});

it("browses published trees separately and opens the exact published snapshot", async () => {
  vi.mocked(api.fetchSessionStatus).mockResolvedValue(false);
  await render("/explore");
  expect(host.querySelector('h1')?.textContent).toBe('تصفح الأشجار');
  expect(host.querySelectorAll('.tree-library-row')).toHaveLength(1);
  expect(host.querySelector('.tree-library-row')?.textContent).toContain('شجرة عامة منشورة');
  expect(host.querySelector('.tree-library-row a')?.getAttribute('href')).toBe('/tree/public-1/versions/sealed-1?from=explore');
  expect(host.textContent).not.toContain(tree.name);
  expect(host.textContent).not.toContain(charts[0].titleAr);
  expect(api.fetchWorkspaceTrees).not.toHaveBeenCalled();
  expect(api.fetchChartLibrary).not.toHaveBeenCalled();
});

it("keeps browse search in the URL without changing the workspace", async () => {
  const router = await render("/explore?q=missing");
  expect(host.querySelectorAll('.tree-library-row')).toHaveLength(0);
  expect(host.textContent).toContain('لا توجد أشجار مطابقة');
  await act(async () => (host.querySelector('.tree-library-empty a') as HTMLAnchorElement).click());
  await settle();
  expect(router.state.location.search.q).toBeUndefined();
  expect(host.querySelectorAll('.tree-library-row')).toHaveLength(1);
});

it("shows an explicit public-list failure rather than a private or synthetic fallback", async () => {
  vi.mocked(api.fetchPublishedTrees).mockRejectedValue(new Error('offline'));
  await render('/explore');
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('تعذر تحميل الأشجار المنشورة');
  expect(host.querySelectorAll('.tree-library-row')).toHaveLength(0);
  expect(host.textContent).not.toContain('لا توجد أشجار منشورة بعد');
});
