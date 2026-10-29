// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { routeTree } from "../route-tree";
import * as api from "../lib/api";

vi.mock("../lib/api", async (original) => ({ ...await original<typeof import("../lib/api")>(), fetchSessionStatus: vi.fn(), fetchResearchAccess: vi.fn(), fetchQuestions: vi.fn(), createQuestion: vi.fn(), fetchResearchWorkspace: vi.fn(), fetchQuestion: vi.fn(), updateQuestion: vi.fn(), addQuestionNote: vi.fn(), linkQuestionSource: vi.fn(), fetchSources: vi.fn(), fetchSourceProcessing: vi.fn(), fetchSearch: vi.fn(), fetchDictionaryIndex: vi.fn(), fetchDictionaryDetail: vi.fn(), fetchClaim: vi.fn(), queryResearch: vi.fn(), fetchEntityResolutionCandidates: vi.fn(), fetchEntityResolutionMerges: vi.fn(), fetchEntityResolutionRun: vi.fn(), runEntityResolution: vi.fn(), fetchContradictionFindings: vi.fn(), fetchContradictionRun: vi.fn(), startContradictionRun: vi.fn() }));
const questionId = "a0000000-0000-4000-8000-000000000001";
const question = { id: questionId, titleAr: "سؤال محفوظ للتحقيق", descriptionAr: "السياق", status: "under_investigation", priority: "high", createdAt: "2026-10-29", updatedAt: "2026-10-29", claimCount: 0, sourceCount: 0 };
const workspace = { context: { questionId, questionTitleAr: question.titleAr, questionStatus: question.status }, permissions: { canRunResearch: true, canManageSelectedQuestion: true, canCreateQuestion: true }, claims: [], sources: [], treeContexts: [], mapFeatures: [], timeline: [], questions: [], disputes: [], notes: [], findings: [], identityCandidates: [], history: [] };
const questionDetail = { question, notes: [], sources: [], claims: [], entities: [], disputes: [], findings: [], activity: [] };
let host: HTMLDivElement; let root: Root | undefined; let client: QueryClient;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true); vi.stubGlobal("scrollTo", vi.fn());
  vi.mocked(api.fetchSessionStatus).mockResolvedValue(true);
  vi.mocked(api.fetchResearchAccess).mockResolvedValue({ signedIn: true, canReview: true, canMerge: false });
  vi.mocked(api.fetchQuestions).mockResolvedValue([question] as never);
  vi.mocked(api.createQuestion).mockResolvedValue(questionDetail as never);
  vi.mocked(api.fetchResearchWorkspace).mockResolvedValue(workspace as never);
  vi.mocked(api.fetchQuestion).mockResolvedValue(questionDetail as never);
  vi.mocked(api.fetchSources).mockResolvedValue([{ id: "source-1", titleAr: "مصدر محفوظ" }] as never);
  vi.mocked(api.fetchSourceProcessing).mockResolvedValue({ sourceId: "source-1", files: [{ id: "file-1", sourceId: "source-1", originalFilenameAr: "مشجر.pdf", processingStatus: "failed", processingError: "خطأ محفوظ" }], runs: [], candidates: [] } as never);
  vi.mocked(api.fetchSearch).mockResolvedValue({ query: "محمد", normalizedQuery: "محمد", total: 3, groups: [{ key: "names", label: "الأسماء", items: [{ id: "person-1", kind: "person", title: "محمد بن سعد", score: 1 }] }, { key: "sources", label: "المصادر", items: [{ id: "source-1", kind: "source", title: "مصدر محفوظ", score: 1 }] }, { key: "questions", label: "الأسئلة", items: [{ id: questionId, kind: "question", title: question.titleAr, score: 1 }] }] });
  vi.mocked(api.fetchDictionaryIndex).mockResolvedValue({ kind: "people", query: "", items: [{ id: "person-1", kind: "person", nameAr: "محمد بن سعد", count: 0 }] });
  vi.mocked(api.fetchDictionaryDetail).mockResolvedValue({ id: "person-1", kind: "person", nameAr: "محمد بن سعد", aliases: [], historicalNames: [], branches: [], places: [], families: [], people: [], tribes: [], publishedTrees: [{ id: "tree-1", nameAr: "شجرة منشورة", versionId: "published-1", versionNumber: 1 }], sources: [], migrations: [], claims: [], questions: [] });
  vi.mocked(api.fetchEntityResolutionCandidates).mockResolvedValue([]);
  vi.mocked(api.fetchEntityResolutionMerges).mockResolvedValue([]);
  vi.mocked(api.fetchContradictionFindings).mockResolvedValue([]);
});
afterEach(async () => { if (root) await act(async () => root?.unmount()); root = undefined; client?.clear(); host?.remove(); vi.resetAllMocks(); vi.unstubAllGlobals(); });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 25)); }); }
async function render(path: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => root!.render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>));
  await settle(); await settle(); return router;
}
function button(label: string) { return [...host.querySelectorAll<HTMLButtonElement>("button")].find((item) => item.textContent?.includes(label))!; }
async function fill(selector: string, value: string) {
  const input = host.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector)!;
  const prototype = input instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  await act(async () => { Object.getOwnPropertyDescriptor(prototype, "value")!.set!.call(input, value); input.dispatchEvent(new Event("input", { bubbles: true })); });
}
async function click(label: string) { await act(async () => button(label).click()); await settle(); }

it("keeps the original question artwork and one question index in the merged hub", async () => {
  await render("/questions");
  expect(host.querySelector("h1")?.textContent).toBe("مكتب البحث");
  expect(host.querySelector(".questions-orbit .question-orbit-core")?.textContent).toBe("؟");
  expect(host.querySelectorAll(".question-table-row")).toHaveLength(1);
  expect(host.querySelector(".question-workspace-grid")).toBeNull();
  expect(host.querySelector(".sidebar-tools")).toBeNull();
  expect([...host.querySelectorAll(".sidebar-research nav a")].map((link) => link.getAttribute("href"))).toEqual(["/research", "/search"]);
});
it("saves a real question before opening its investigation", async () => {
  const router = await render("/research?create=true");
  await fill('.research-create-question input', 'هل تتفق الروايات؟');
  await click('احفظ وافتح التحقيق');
  expect(api.createQuestion).toHaveBeenCalledWith({ title_ar: 'هل تتفق الروايات؟', description_ar: undefined, priority: 'normal' });
  expect(router.state.location.pathname).toBe(`/research/${questionId}`);
  expect(host.querySelector('[aria-label="أقسام التحقيق"]')).not.toBeNull();
});
it("keeps failed question creation open with its input intact", async () => {
  vi.mocked(api.createQuestion).mockRejectedValue(new Error('تعذر الحفظ'));
  const router = await render('/research?create=true');
  await fill('.research-create-question input', 'سؤال لا يضيع');
  await click('احفظ وافتح التحقيق');
  expect(router.state.location.pathname).toBe('/research');
  expect(host.querySelector<HTMLInputElement>('.research-create-question input')?.value).toBe('سؤال لا يضيع');
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('تعذر الحفظ');
});
it("merges search and the index and preserves exact result destinations", async () => {
  await render('/search?q=محمد');
  expect(host.querySelectorAll('.search-result-row')).toHaveLength(3);
  expect(host.querySelector('a[href="/search?view=index&kind=people&id=person-1"]')).not.toBeNull();
  expect(host.querySelector('a[href="/sources?sourceId=source-1"]')).not.toBeNull();
  expect(host.querySelector(`a[href="/research/${questionId}"]`)).not.toBeNull();
});
it("opens bookmarked index records directly and links their published versions", async () => {
  const router = await render('/dictionary?kind=people&id=person-1');
  expect(router.state.location.pathname).toBe('/search');
  expect(router.state.location.search).toMatchObject({ view: 'index', kind: 'people', id: 'person-1' });
  expect(api.fetchDictionaryDetail).toHaveBeenCalledWith('people', 'person-1');
  expect(host.querySelector('.dictionary-detail-head h2')?.textContent).toBe('محمد بن سعد');
  expect(host.querySelector('a[href="/tree/tree-1/versions/published-1?from=explore"]')).not.toBeNull();
});
it("moves Jobs to source processing without manual worker controls", async () => {
  const router = await render('/jobs');
  expect(router.state.location.pathname).toBe('/sources');
  expect(router.state.location.search.view).toBe('processing');
  expect(host.textContent).not.toContain('manual-worker');
  expect(host.textContent).not.toContain('Payload JSON');
  await act(async () => router.navigate({ to: '/sources', search: { view: 'processing', sourceId: 'source-1' } })); await settle();
  expect(host.textContent).toContain('مشجر.pdf');
  expect(host.textContent).toContain('تحتاج المعالجة إلى متابعة');
  expect(host.querySelector('a[href="/sources?sourceId=source-1"]')).not.toBeNull();
});
it("does not render review actions or request protected data without the role", async () => {
  vi.mocked(api.fetchResearchAccess).mockResolvedValue({ signedIn: true, canReview: false, canMerge: false });
  await render('/entity-resolution');
  expect(host.textContent).toContain('تحتاج هذه الأدوات إلى صلاحية باحث أو مشرف');
  expect(api.fetchEntityResolutionCandidates).not.toHaveBeenCalled();
  expect(api.fetchEntityResolutionMerges).not.toHaveBeenCalled();
});
it("allows research review but keeps merge execution limited to moderators", async () => {
  vi.mocked(api.fetchEntityResolutionCandidates).mockResolvedValue([{ id: 'candidate', leftEntityId: 'left', rightEntityId: 'right', leftNameAr: 'محمد', rightNameAr: 'محمد آخر', candidateVersion: 1, reviewStatus: 'approved', matchClass: 'possible_match', score: .6, matchingSignals: [], conflictingSignals: [], scoreComponents: {} }] as never);
  await render('/research?view=review');
  expect(host.querySelector('.entity-resolution-card')).not.toBeNull();
  expect(host.querySelector('.entity-resolution-merge-form')).toBeNull();
});
it("refreshes findings when a queued contradiction scan completes", async () => {
  vi.mocked(api.startContradictionRun).mockResolvedValue({ id: 'scan-1', status: 'queued' } as never);
  vi.mocked(api.fetchContradictionRun).mockResolvedValue({ id: 'scan-1', status: 'succeeded', findingCount: 1 } as never);
  vi.mocked(api.fetchContradictionFindings).mockImplementation(async (runId) => runId ? [{ id: 'finding', titleAr: 'تعارض يحتاج مراجعة', explanationAr: 'روايتان مختلفتان', severity: 'high', findingType: 'parent_conflict', signals: {}, claimIds: [], entityIds: [] }] as never : []);
  const router = await render('/contradictions');
  await click('فحص التعارضات'); await settle();
  expect(router.state.location.search.runId).toBe('scan-1');
  expect(host.textContent).toContain('تعارض يحتاج مراجعة');
  expect(host.textContent).toContain('اكتمل الفحص');
  expect(api.fetchContradictionFindings).toHaveBeenCalledWith('scan-1', 'needs_review');
});
it("saves a note inside the question and keeps source links contextual", async () => {
  const notes: Array<{ id: string; noteAr: string }> = [];
  vi.mocked(api.fetchQuestion).mockImplementation(async () => ({ ...questionDetail, notes }) as never);
  vi.mocked(api.addQuestionNote).mockImplementation(async (_id, input) => { notes.push({ id: 'note-1', noteAr: input.note_ar }); return { ...questionDetail, notes } as never; });
  await render(`/research/${questionId}?panel=manage`);
  await fill('textarea[aria-label="ملاحظة التحقيق"]', 'راجع تاريخ المصدر أولاً');
  await click('احفظ الملاحظة'); await settle();
  expect(api.addQuestionNote).toHaveBeenCalledWith(questionId, { note_ar: 'راجع تاريخ المصدر أولاً' });
  expect(host.textContent).toContain('راجع تاريخ المصدر أولاً');
});

it("runs evidence research, retains its query and opens the exact cited source", async () => {
  const citation = { id: 'statement-1', layer: 'source_statement', type: 'statement', title: 'نص من مصدر محفوظ', excerpt: 'ورد النسب في النص', sourceId: 'source-1', rank: 1, score: {} };
  vi.mocked(api.queryResearch).mockResolvedValue({ query: 'ما أصل الرواية؟', answer: 'وردت الرواية في مصدر محفوظ.', insufficientEvidence: true, citations: [citation], layers: { sourceStatements: [citation], researchClaims: [], treeInterpretations: [], platformFindings: [], openQuestions: [] }, conflicts: [], retrieval: { fusedCandidates: 1 } } as never);
  const router = await render('/research?view=evidence');
  expect(host.querySelector('.claim-composer')).toBeNull();
  await fill('#research-question', 'ما أصل الرواية؟');
  await click('ابحث في الأدلة'); await settle();
  expect(api.queryResearch).toHaveBeenCalledWith(expect.objectContaining({ question: 'ما أصل الرواية؟' }));
  expect(router.state.location.search.q).toBe('ما أصل الرواية؟');
  expect(host.querySelector('.research-result-answer')?.textContent).toBe('وردت الرواية في مصدر محفوظ.');
  expect(host.querySelector('.research-evidence-item a[href="/sources?sourceId=source-1"]')).not.toBeNull();
});

it("opens stored UUIDs with legacy version bits as real investigations, never demo content", async () => {
  const storedId = '80000000-0000-0000-0000-000000000001';
  vi.mocked(api.fetchResearchWorkspace).mockResolvedValue({ ...workspace, context: { ...workspace.context, questionId: storedId, questionTitleAr: 'سؤال حقيقي من قاعدة البيانات' } } as never);
  await render(`/research/${storedId}`);
  expect(api.fetchResearchWorkspace).toHaveBeenCalledWith(expect.objectContaining({ questionId: storedId }));
  expect(host.querySelector('h1')?.textContent).toBe('سؤال حقيقي من قاعدة البيانات');
  expect(host.querySelector('.demo-banner')).toBeNull();
});
