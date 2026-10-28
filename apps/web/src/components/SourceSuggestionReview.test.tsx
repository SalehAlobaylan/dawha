// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { SourceSuggestionReview } from "./SourceSuggestionReview";
import * as api from "../lib/api";
vi.mock("../lib/api", async (original) => ({ ...await original<typeof import("../lib/api")>(),
 fetchNameJudgeConfig: vi.fn(), fetchReadingCapabilities: vi.fn(), fetchSourceTreeSuggestions: vi.fn(),
 fetchSourceSuggestionAccuracy: vi.fn(), fetchSourcePagePreview: vi.fn(), fetchSourceSuggestionCrop: vi.fn(),
 fetchSourceSuggestionCandidates: vi.fn(), judgeSourceTreeSuggestion: vi.fn(),
 fetchNameLexicon: vi.fn(async()=>({ entries: [] })),
 beginNameReview: vi.fn(async(suggestionId:string)=>({ exposureId:`exposure-${suggestionId}`, reading:"محمد", alternatives:[],readers:[{value:"محمد",variant:"fixture",readerVersion:"fixture"}],lexicon:{entries:[]},canPrepare:true })),
 prepareNameCandidates:vi.fn(async(_id:string,exposureId:string,snapshot:unknown)=>({id:`preparation-${exposureId}`,signature:"a".repeat(64),snapshot})),
 sealNameReview:vi.fn(async()=>({sealed:true})),
 decideSourceTreeSuggestion:vi.fn(async()=>({suggestion:{},evidenceRecorded:true})),
 fetchLocalNameReadings: vi.fn(async () => []), fetchLatestLocalRereadRun: vi.fn(async () => null),
 fetchNameReviewSummary: vi.fn(async () => ({ total: 0, pending: 0, deferred: 0, accepted: 0, corrected: 0, rejected: 0, decided: 0, superseded: 0, decisionEvents: 0 })),
 fetchSourceSuggestionHistory: vi.fn(async () => null),
 reopenSourceSuggestion: vi.fn(),
 previewSourceSuggestionRegion: vi.fn(),
 reviseSourceSuggestionRegion: vi.fn(),
 splitSourceTreeSuggestion: vi.fn(),
 mergeSourceTreeSuggestions: vi.fn(),
}));
let root: Root | undefined;
let host: HTMLDivElement;
afterEach(async () => { if (root) await act(async () => root?.unmount()); host?.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
it("disables unavailable readers before click and keeps shadow invisible", async () => {
 vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
 vi.stubGlobal("matchMedia", () => ({ matches: true }));
 vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
 vi.stubGlobal("URL", { createObjectURL: () => "blob:fixture", revokeObjectURL: () => {} });
 vi.mocked(api.fetchSourceTreeSuggestions).mockImplementation(async (_file, status) => status === "suggested" ? [{ id: "suggestion-1", pageNumber: 1, suggestedTextAr: "محمد", alternatives: [], region: { x: 0.1, y: 0.1, width: 0.1, height: 0.1 }, confidence: 0.5, viewCount: 1, readingMethod: "fixture", readingModel: "fixture" }] as never : []);
 vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({ total: 0, measurable: false } as never);
 vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
 vi.mocked(api.fetchSourceSuggestionCrop).mockRejectedValue(new Error("no crop"));
 vi.mocked(api.fetchReadingCapabilities).mockResolvedValue({ mode: "local", localReader: { configured: true, ready: false, reason: "local_reader_not_implemented", languages: [], tools: [], version: "pending" }, hostedReader: { enabled: false, configured: false, ready: false, allowedForSource: false, reason: "disabled_by_policy", version: "fixture" } });
 const markup: string[] = [];
 for (const mode of ["off", "shadow"] as const) {
  vi.mocked(api.fetchNameJudgeConfig).mockResolvedValue({ mode, available: true, sample: 1, imageEnabled: false });
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => { root!.render(<SourceSuggestionReview fileId="file-1" onClose={() => {}} />); });
  const button = [...host.querySelectorAll("button")].find((element) => element.textContent?.includes("اقترح بالذكاء"));
  expect(button).toBeDefined(); expect(button?.disabled).toBe(true);
  expect(host.textContent).toContain("القراءة البصرية الخارجية معطّلة");
  const local = [...host.querySelectorAll("button")].find((element) => element.textContent?.includes("إعادة القراءة محلياً"));
  expect(local?.disabled).toBe(true);
  expect(host.querySelector("input")?.disabled).toBe(false);
  await act(async () => button?.click());
  expect(api.fetchSourceSuggestionCandidates).not.toHaveBeenCalled();
  markup.push(host.innerHTML);
  await act(async () => root!.unmount()); root = undefined; host.remove();
 }
 expect(markup[1]).toBe(markup[0]);
});

it("aborts an obsolete crop on file navigation while the next remains usable", async () => {
 vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
 vi.stubGlobal("matchMedia", () => ({ matches: true }));
 vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
 vi.mocked(api.fetchNameJudgeConfig).mockResolvedValue({ mode: "off", available: false, sample: 0, imageEnabled: false });
 vi.mocked(api.fetchSourceTreeSuggestions).mockImplementation(async (file, status) => status === "suggested" ? [{ id: file, pageNumber: 1, suggestedTextAr: "محمد", alternatives: [], region: { x: 0.1, y: 0.1, width: 0.1, height: 0.1 }, confidence: 0.5 }] as never : []);
 vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({ total: 0, measurable: false } as never);
 vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
 const signals = new Map<string, AbortSignal>();
 vi.mocked(api.fetchSourceSuggestionCrop).mockImplementation((id, signal) => new Promise((_resolve, reject) => { signals.set(id, signal!); signal!.addEventListener("abort", () => reject(new DOMException("obsolete", "AbortError"))); }));
 host = document.createElement("div"); document.body.append(host); root = createRoot(host);
 await act(async () => root!.render(<SourceSuggestionReview fileId="file-1" onClose={() => {}} />));
 expect(signals.get("file-1")?.aborted).toBe(false);
 await act(async () => root!.render(<SourceSuggestionReview fileId="file-2" onClose={() => {}} />));
 expect(signals.get("file-1")?.aborted).toBe(true);
 expect(signals.get("file-2")?.aborted).toBe(false);
 expect(host.querySelector("input")?.disabled).toBe(false);
});

it("does not present local view agreement as a correctness percentage", async () => {
 vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
 vi.stubGlobal("matchMedia", () => ({ matches: true }));
 vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
 vi.mocked(api.fetchNameJudgeConfig).mockResolvedValue({ mode: "shadow", available: false, sample: 1, imageEnabled: false });
 vi.mocked(api.fetchSourceTreeSuggestions).mockImplementation(async (_file, status) => status === "suggested" ? [{ id: "local-1", pageNumber: 1, suggestedTextAr: "محمد", alternatives: [], region: { x: .1, y: .1, width: .1, height: .1 }, confidence: 1, viewCount: 3, readingMethod: "local_tesseract_crop", readingModel: "fixture" }] as never : []);
 vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({ total: 0, measurable: false } as never);
 vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
 vi.mocked(api.fetchSourceSuggestionCrop).mockRejectedValue(new Error("no crop"));
 host = document.createElement("div"); document.body.append(host); root = createRoot(host);
 await act(async () => root!.render(<SourceSuggestionReview fileId="local-file" onClose={() => {}} />));
 const reading = host.querySelector(".name-bench-reading");
 expect(reading?.textContent).toContain("3 قراءات محلية تجريبية");
 expect(reading?.textContent).not.toContain("٪");
 expect(reading?.querySelector(".name-bench-confidence-track")).toBeNull();
 expect(host.querySelector("input")?.disabled).toBe(false);
});


it("freezes initial evidence before editing and never waits for a shadow result on save", async () => {
 vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
 vi.stubGlobal("matchMedia", () => ({ matches: true }));
 vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
 vi.mocked(api.fetchNameJudgeConfig).mockResolvedValue({ mode:"shadow",available:true,sample:1,imageEnabled:false });
 vi.mocked(api.fetchSourceTreeSuggestions).mockImplementation(async(_file,status)=>status==="suggested"?[{id:"freeze-1",pageNumber:1,suggestedTextAr:"محمد",alternatives:[],region:{x:.1,y:.1,width:.1,height:.1},confidence:.5,readingMethod:"fixture"}] as never:[]);
 vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({total:0,measurable:false} as never);
 vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
 vi.mocked(api.fetchSourceSuggestionCrop).mockRejectedValue(new Error("no crop"));
 host=document.createElement("div");document.body.append(host);root=createRoot(host);
 await act(async()=>root!.render(<SourceSuggestionReview fileId="freeze-file" onClose={()=>{}}/>));
 expect(api.prepareNameCandidates).toHaveBeenCalledWith("freeze-1","exposure-freeze-1",expect.any(Array),expect.any(AbortSignal));
 expect(host.querySelector("input")?.disabled).toBe(false);
 const accept=host.querySelector<HTMLButtonElement>(".name-bench-primary");
 expect(accept).toBeDefined();
 await act(async()=>accept?.click());
 expect(api.judgeSourceTreeSuggestion).not.toHaveBeenCalled();
 const evidence=vi.mocked(api.decideSourceTreeSuggestion).mock.calls.at(-1)?.[4];
 expect(evidence).toMatchObject({evidenceSchemaVersion:"name-review-v2",preparationId:"preparation-exposure-freeze-1",preparationSignature:"a".repeat(64)});
 expect(evidence?.judgeOutput).toBeUndefined();
});

const baseSuggestion = {
  status: "suggested" as const,
  reviewRevision: 0,
  geometryRevision: 0,
  superseded: false,
  pageNumber: 1,
  suggestedTextAr: "محمد",
  alternatives: [],
  region: { x: 0.1, y: 0.1, width: 0.1, height: 0.1 },
  confidence: 0.5,
  viewCount: 1,
  readingMethod: "fixture",
  readingModel: "fixture",
};

it("finds deferred work after reload and reopens it with its history intact", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  vi.stubGlobal("URL", { createObjectURL: () => "blob:fixture", revokeObjectURL: () => {} });
  const deferred = { ...baseSuggestion, id: "deferred-1", status: "deferred" as const, reviewRevision: 1, decidedTextAr: "أحمد" };
  const reopened = { ...baseSuggestion, id: "deferred-1", reviewRevision: 1 };
  let pending: typeof baseSuggestion[] = [];
  vi.mocked(api.fetchSourceTreeSuggestions).mockImplementation(async (_file, status) => {
    if (status === "suggested") return pending as never;
    if (status === "deferred") return [deferred] as never;
    return [] as never;
  });
  vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({ total: 1, measurable: false } as never);
  vi.mocked(api.fetchNameReviewSummary).mockResolvedValue({ total: 1, pending: 0, deferred: 1, accepted: 0, corrected: 1, rejected: 0, decided: 1, superseded: 0, decisionEvents: 0 });
  vi.mocked(api.fetchSourceSuggestionHistory).mockResolvedValue({
    suggestion: deferred,
    originalRegion: deferred.region,
    decisions: [{ id: "decision-1", reviewerId: "reviewer-1", decision: "corrected", suggestedTextAr: "محمد", decidedTextAr: "أحمد", revision: 1, reviewedAt: "2026-10-27T10:00:00Z" }],
    events: [{ id: "event-1", type: "decision", actorId: "reviewer-1", revision: 1, createdAt: "2026-10-27T10:00:00Z" }],
    regionRevisions: [],
    lineage: [],
    canReopen: true,
    canReviseRegion: false,
    canSplit: false,
    canMerge: false,
  } as never);
  vi.mocked(api.reopenSourceSuggestion).mockImplementation(async () => { pending = [reopened]; return reopened as never; });
  vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
  vi.mocked(api.fetchSourceSuggestionCrop).mockRejectedValue(new Error("no crop"));
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => { root!.render(<SourceSuggestionReview fileId="deferred-file" onClose={() => {}} />); });
  const deferredTab = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("مؤجلة"));
  await act(async () => { deferredTab?.click(); await Promise.resolve(); });
  expect(host.textContent).toContain("سجل المراجعة");
  expect(host.textContent).toContain("أحمد");
  const reopen = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("إعادة فتح الموضع"));
  expect(reopen).toBeDefined();
  await act(async () => { reopen?.click(); await Promise.resolve(); });
  expect(api.reopenSourceSuggestion).toHaveBeenCalledWith("deferred-1", 1);
  expect(host.textContent).toContain("أُعيد الموضع إلى قائمة الانتظار");
});

it("previews a region and only persists the geometry the reviewer saw", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  vi.stubGlobal("URL", { createObjectURL: () => "blob:fixture", revokeObjectURL: () => {} });
  const suggestion = { ...baseSuggestion, id: "region-1" };
  vi.mocked(api.fetchSourceTreeSuggestions).mockResolvedValue([suggestion] as never);
  vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({ total: 0, measurable: false } as never);
  vi.mocked(api.fetchNameReviewSummary).mockResolvedValue({ total: 1, pending: 1, deferred: 0, accepted: 0, corrected: 0, rejected: 0, decided: 0, superseded: 0, decisionEvents: 0 });
  vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
  vi.mocked(api.fetchSourceSuggestionCrop).mockRejectedValue(new Error("no crop"));
  vi.mocked(api.previewSourceSuggestionRegion).mockResolvedValue({
    geometryRevision: 0,
    region: suggestion.region,
    coordinateSpace: "native_raster",
    analysisDpi: 0,
    sourceChecksum: "a".repeat(64),
    frameWidth: 600,
    frameHeight: 400,
    tightBounds: { x: 60, y: 40, width: 60, height: 40 },
    contextBounds: { x: 30, y: 20, width: 120, height: 80 },
    quality: { nameHeightPx: 40, contrast: 0.9, clipped: false, coloured: false },
    tightPng: "ZmFrZQ==",
    contextPng: "ZmFrZQ==",
  });
  vi.mocked(api.reviseSourceSuggestionRegion).mockResolvedValue({ ...suggestion, geometryRevision: 1 } as never);
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => { root!.render(<SourceSuggestionReview fileId="region-file" onClose={() => {}} />); });
  const edit = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("تعديل الحدود"));
  await act(async () => { edit?.click(); });
  expect(host.textContent).toContain("حدود القراءة");
  const preview = [...host.querySelectorAll("button")].find((button) => button.textContent?.trim() === "معاينة");
  await act(async () => { preview?.click(); await Promise.resolve(); });
  expect(api.previewSourceSuggestionRegion).toHaveBeenCalledWith("region-1", { expectedGeometryRevision: 0, region: suggestion.region });
  const save = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("حفظ الحدود"));
  expect(save?.disabled).toBe(false);
  await act(async () => { save?.click(); await Promise.resolve(); });
  expect(api.reviseSourceSuggestionRegion).toHaveBeenCalledWith("region-1", expect.objectContaining({
    expectedGeometryRevision: 0,
    region: suggestion.region,
    sourceChecksum: "a".repeat(64),
  }));
  expect(host.textContent).toContain("حُدّثت حدود القراءة");
});

it("splits a pending occurrence into two contained regions", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  vi.stubGlobal("URL", { createObjectURL: () => "blob:fixture", revokeObjectURL: () => {} });
  const suggestion = { ...baseSuggestion, id: "split-1", region: { x: 0.2, y: 0.2, width: 0.4, height: 0.4 } };
  vi.mocked(api.fetchSourceTreeSuggestions).mockResolvedValue([suggestion] as never);
  vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({ total: 0, measurable: false } as never);
  vi.mocked(api.fetchNameReviewSummary).mockResolvedValue({ total: 1, pending: 1, deferred: 0, accepted: 0, corrected: 0, rejected: 0, decided: 0, superseded: 0, decisionEvents: 0 });
  vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
  vi.mocked(api.fetchSourceSuggestionCrop).mockRejectedValue(new Error("no crop"));
  vi.mocked(api.splitSourceTreeSuggestion).mockResolvedValue({ parent: { ...suggestion, superseded: true }, children: [] } as never);
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => { root!.render(<SourceSuggestionReview fileId="split-file" onClose={() => {}} />); });
  const split = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("تقسيم"));
  await act(async () => { split?.click(); });
  expect(host.textContent).toContain("تقسيم الموضع");
  const submit = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("قسّم واحفظ"));
  await act(async () => { submit?.click(); await Promise.resolve(); });
  expect(api.splitSourceTreeSuggestion).toHaveBeenCalledWith("split-1", expect.objectContaining({
    expectedReviewRevision: 0,
    expectedGeometryRevision: 0,
    regions: expect.any(Array),
  }));
  const regions = vi.mocked(api.splitSourceTreeSuggestion).mock.calls[0][1].regions;
  expect(regions).toHaveLength(2);
  expect(regions[0].x + regions[0].width).toBeLessThanOrEqual(0.6 + 1e-9);
  expect(regions[1].x).toBeGreaterThanOrEqual(0.4 - 1e-9);
});

it("merges selected pending occurrences with their revisions", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  vi.stubGlobal("URL", { createObjectURL: () => "blob:fixture", revokeObjectURL: () => {} });
  const first = { ...baseSuggestion, id: "merge-1", region: { x: 0.1, y: 0.1, width: 0.1, height: 0.1 } };
  const second = { ...baseSuggestion, id: "merge-2", region: { x: 0.1, y: 0.3, width: 0.1, height: 0.1 } };
  vi.mocked(api.fetchSourceTreeSuggestions).mockResolvedValue([first, second] as never);
  vi.mocked(api.fetchSourceSuggestionAccuracy).mockResolvedValue({ total: 0, measurable: false } as never);
  vi.mocked(api.fetchNameReviewSummary).mockResolvedValue({ total: 2, pending: 2, deferred: 0, accepted: 0, corrected: 0, rejected: 0, decided: 0, superseded: 0, decisionEvents: 0 });
  vi.mocked(api.fetchSourcePagePreview).mockRejectedValue(new Error("no preview"));
  vi.mocked(api.fetchSourceSuggestionCrop).mockRejectedValue(new Error("no crop"));
  vi.mocked(api.mergeSourceTreeSuggestions).mockResolvedValue({ merged: { ...first, id: "merged-1" }, superseded: [first, second] } as never);
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => { root!.render(<SourceSuggestionReview fileId="merge-file" onClose={() => {}} />); });
  const merge = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("دمج"));
  await act(async () => { merge?.click(); });
  expect(host.textContent).toContain("دمج المواضع");
  const boxes = [...host.querySelectorAll<HTMLInputElement>(".name-bench-merge-list input[type=checkbox]")];
  expect(boxes).toHaveLength(2);
  await act(async () => { boxes[1].click(); });
  const submit = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes("ادمج واحفظ"));
  await act(async () => { submit?.click(); await Promise.resolve(); });
  expect(api.mergeSourceTreeSuggestions).toHaveBeenCalledWith("merge-1", expect.objectContaining({
    members: [
      { id: "merge-1", expectedReviewRevision: 0, expectedGeometryRevision: 0 },
      { id: "merge-2", expectedReviewRevision: 0, expectedGeometryRevision: 0 },
    ],
    region: expect.objectContaining({ x: 0.1, y: 0.1, width: 0.1 }),
  }));
  const mergeRegion = vi.mocked(api.mergeSourceTreeSuggestions).mock.calls[0][1].region;
  expect(mergeRegion.height).toBeCloseTo(0.3, 9);
});
