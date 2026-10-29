// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { routeTree } from "../route-tree";
import * as api from "../lib/api";
import type { SourceDetail, SourceFile, SourceProcessing, SourceTreeDraft, SourceTreeDraftImportPreview } from "../types";

vi.mock("../lib/api", async (original) => ({ ...await original<typeof import("../lib/api")>(),
  fetchSessionStatus: vi.fn(), createSource: vi.fn(), uploadSourceFile: vi.fn(), fetchSource: vi.fn(), fetchSourceProcessing: vi.fn(),
  fetchChartLibrary: vi.fn(),
  createSourceTreeDraft: vi.fn(), fetchSourceTreeDraft: vi.fn(), saveSourceTreeDraft: vi.fn(), previewSourceTreeDraftImport: vi.fn(), importSourceTreeDraft: vi.fn(),
}));
let root: Root | undefined;
let host: HTMLDivElement;
let client: QueryClient;
const source = { source: { id: "source-1", titleAr: "مشجر تجريبي", sourceType: "family_document" } } as SourceDetail;
const file = { id: "file-1", sourceId: "source-1", originalFilenameAr: "مشجر.pdf", mimeType: "application/pdf", processingStatus: "succeeded" } as SourceFile;
const processing: SourceProcessing = { sourceId: "source-1", files: [file], runs: [], candidates: [] };
const draft: SourceTreeDraft = {
  id: "draft-1", sourceId: "source-1", sourceFileId: "file-1", titleAr: "مشجر تجريبي", state: "draft", revision: 1,
  nodes: [{ id: "node-1", nameAr: "محمد", pageEvidenceId: "page-1", region: { kind: "name", text: "محمد", x: 0, y: 0, width: .1, height: .1 }, included: true, reviewed: true, status: "reviewed" }],
  edges: [], pages: [], warnings: [], createdAt: "", updatedAt: "",
};
const report: SourceTreeDraftImportPreview = { draftId: "draft-1", expectedRevision: 1, sourceFileChecksum: "checksum", newPeople: 1, linkedExisting: 0, collisions: [], relations: 0, stagedRelations: 0, statements: 1, aliases: 1, candidates: 0, excludedNodes: 0, warnings: [] };

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("scrollTo", vi.fn());
  vi.mocked(api.fetchSessionStatus).mockResolvedValue(true);
  vi.mocked(api.fetchChartLibrary).mockResolvedValue([]);
  vi.mocked(api.createSource).mockResolvedValue(source);
  vi.mocked(api.fetchSource).mockResolvedValue(source);
  vi.mocked(api.uploadSourceFile).mockResolvedValue(file);
  vi.mocked(api.fetchSourceProcessing).mockResolvedValue(processing);
  vi.mocked(api.createSourceTreeDraft).mockResolvedValue(draft);
  vi.mocked(api.fetchSourceTreeDraft).mockResolvedValue(draft);
  vi.mocked(api.previewSourceTreeDraftImport).mockResolvedValue(report);
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined; client?.clear(); host?.remove(); vi.resetAllMocks(); vi.unstubAllGlobals();
});
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 20)); }); }
async function render(path = "/import") {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) });
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => { root!.render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>); });
  await settle(); await settle();
  return router;
}
function button(label: string) { const result = [...host.querySelectorAll("button")].find((item) => item.textContent?.includes(label)); expect(result).toBeDefined(); return result!; }
async function click(label: string) { await act(async () => button(label).click()); await settle(); }
async function choose(name = "مشجر.pdf", type = "application/pdf") {
  const input = host.querySelector<HTMLInputElement>('input[type="file"]')!;
  Object.defineProperty(input, "files", { configurable: true, value: [new File(["test"], name, { type })] });
  await act(async () => input.dispatchEvent(new Event("change", { bubbles: true })));
}

it("rejects unsupported files and requires signing in before upload", async () => {
  vi.mocked(api.fetchSessionStatus).mockResolvedValue(false);
  await render();
  await choose("image.png", "image/png");
  expect(host.textContent).toContain("اختر ملف PDF للمشجر");
  await choose();
  expect(button("رفع ومتابعة").disabled).toBe(true);
  expect(host.textContent).toContain("سجّل الدخول لحفظ المشجر");
  expect(api.createSource).not.toHaveBeenCalled();
});

it("retries a failed upload against the same saved source without creating duplicates", async () => {
  vi.mocked(api.uploadSourceFile).mockRejectedValueOnce(new Error("انقطع الاتصال"));
  const router = await render();
  await choose();
  await click("رفع ومتابعة");
  expect(host.textContent).toContain("انقطع الاتصال");
  expect(router.state.location.search).toMatchObject({ sourceId: "source-1" });
  await click("رفع ومتابعة");
  expect(api.createSource).toHaveBeenCalledTimes(1);
  expect(api.createSource).toHaveBeenCalledWith({ title_ar: "مشجر", source_type: "family_document" });
  expect(api.uploadSourceFile).toHaveBeenCalledTimes(2);
  expect(router.state.location.search).toMatchObject({ sourceId: "source-1", fileId: "file-1" });
  expect(host.textContent).toContain("المشجر جاهز للمراجعة");
});

it("resumes an uploaded file by URL and opens its review without another upload", async () => {
  const router = await render("/import?sourceId=source-1&fileId=file-1");
  expect(host.textContent).toContain("المشجر جاهز للمراجعة");
  await click("مراجعة الأسماء والوصلات");
  expect(router.state.location.search).toMatchObject({ draftId: "draft-1" });
  expect(host.querySelector('[role="dialog"]')?.textContent).toContain("راجع الأسماء والوصلات");
  expect(api.createSource).not.toHaveBeenCalled();
  expect(api.uploadSourceFile).not.toHaveBeenCalled();
});

it("shows a final summary even with no collisions and creates the tree only on confirmation", async () => {
  vi.mocked(api.importSourceTreeDraft).mockResolvedValue({ treeId: "tree-1", treeVersionId: "version-1", importedPeople: 1, importedRelations: 0, linkedExistingPeople: 0 } as never);
  await render("/import?sourceId=source-1&fileId=file-1&draftId=draft-1");
  await click("متابعة المراجعة");
  await click("متابعة إلى ملخص الشجرة");
  expect(host.textContent).toContain("ملخص الشجرة قبل الإنشاء");
  expect(api.importSourceTreeDraft).not.toHaveBeenCalled();
  await click("تأكيد وإنشاء الشجرة الخاصة");
  expect(api.importSourceTreeDraft).toHaveBeenCalledWith("draft-1", expect.objectContaining({ expectedRevision: 1, acknowledgePreview: true, expectedNewPeople: 1 }));
  expect(host.textContent).toContain("أُنشئت شجرتك الخاصة");
  expect(host.querySelector('a[href="/tree/tree-1"]')).not.toBeNull();
});

it("offers recovery instead of polling forever for a missing file", async () => {
  vi.mocked(api.fetchSourceProcessing).mockResolvedValue({ ...processing, files: [] });
  await render("/import?sourceId=source-1&fileId=missing");
  expect(host.textContent).toContain("لم نعثر على الملف في هذا المصدر");
  expect(host.querySelector('a[href="/sources?sourceId=source-1"]')).not.toBeNull();
});

it("saves edited names before the final summary and protects unsaved work on close", async () => {
  vi.mocked(api.saveSourceTreeDraft).mockImplementation(async (_id, input) => ({ ...draft, ...input, revision: 2 }));
  vi.mocked(api.previewSourceTreeDraftImport).mockResolvedValue({ ...report, expectedRevision: 2 });
  await render("/import?sourceId=source-1&fileId=file-1&draftId=draft-1");
  await click("متابعة المراجعة");
  const titleInput = host.querySelector<HTMLInputElement>('.source-tree-draft-sidebar .composer-label input')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(titleInput, "عنوان مراجع");
    titleInput.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => host.querySelector<HTMLButtonElement>('button[aria-label="إغلاق"]')!.click());
  expect(host.textContent).toContain("لديك تغييرات لم تُحفظ");
  expect(host.querySelector('[role="dialog"]')).not.toBeNull();
  await click("متابعة المراجعة");
  await click("متابعة إلى ملخص الشجرة");
  expect(api.saveSourceTreeDraft).toHaveBeenCalledWith("draft-1", expect.objectContaining({ titleAr: "عنوان مراجع", expectedRevision: 1 }));
  expect(api.previewSourceTreeDraftImport).toHaveBeenCalledWith("draft-1", 2);
  expect(host.textContent).toContain("ملخص الشجرة قبل الإنشاء");
  expect(api.importSourceTreeDraft).not.toHaveBeenCalled();
});
