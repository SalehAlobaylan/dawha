import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowLeft, FileText, GitBranch, LockKeyhole, Plus, ScanSearch, Search, UploadCloud } from "lucide-react";
import { useState, type FormEvent } from "react";
import { createTree, fetchChartLibrary, fetchWorkspaceTrees, fetchSessionStatus } from "../lib/api";
import { TopBar } from "../components/TopBar";
import type { ChartLibraryItem } from "../types";

import type { LibrarySearch } from "../lib/library-search";

export function TreeLibraryPage() {
  const search = useSearch({ from: "/tree" });
  const navigate = useNavigate();
  const chartsQuery = useQuery({ queryKey: ["chart-library"], queryFn: fetchChartLibrary, staleTime: 0, refetchInterval: (current) => current.state.data?.some((item) => item.readingStatus === "queued" || item.readingStatus === "running") ? 5000 : false });
  const treesQuery = useQuery({ queryKey: ["trees", "workspace"], queryFn: fetchWorkspaceTrees, staleTime: 0 });
  const sessionQuery = useQuery({ queryKey: ["workspace-session"], queryFn: fetchSessionStatus, staleTime: 0 });
  const charts = sessionQuery.data === false ? [] : (chartsQuery.data ?? []).filter((item) => item.canReview);
  const trees = sessionQuery.data === false ? [] : treesQuery.data ?? [];
  const view = search.view ?? "all";
  const reviewView = view === "review";
  const q = search.q ?? "";
  const update = (next: LibrarySearch) => { void navigate({ to: "/tree", search: next, replace: true, resetScroll: false }); };
  const matches = (value: string) => value.normalize("NFKC").includes(q.trim().normalize("NFKC"));
  const reviewable = charts.filter((item) => item.fileId && item.pendingNames + item.deferredNames > 0);
  const queue = reviewable.filter((item) => search.queue !== "deferred" || item.deferredNames > 0);
  const filteredCharts = view === "trees" ? [] : (reviewView ? queue : charts).filter((item) => matches(`${item.titleAr} ${item.filename ?? ""}`));
  const filteredTrees = reviewView || search.kind === "charts" ? [] : trees.filter((item) => matches(`${item.name} ${item.description}`));
  const pendingNames = reviewable.reduce((sum, item) => sum + item.pendingNames, 0);
  const deferredNames = reviewable.reduce((sum, item) => sum + item.deferredNames, 0);
  const loading = view === "review" ? chartsQuery.isPending : view === "trees" ? treesQuery.isPending : chartsQuery.isPending || treesQuery.isPending;
  const error = view === "review" ? chartsQuery.error : view === "trees" ? treesQuery.error : chartsQuery.error ?? treesQuery.error;
  const shownCount = filteredCharts.length + filteredTrees.length;
  const first = filteredCharts[0];
  const reviewQueue = (item: ChartLibraryItem) => search.queue === "deferred" || !item.pendingNames && item.deferredNames > 0 ? "deferred" as const : "pending" as const;

  return <div className="page-stack tree-library-page">
    <TopBar eyebrow="مساحة العمل" title="أشجاري" description="مشجراتك المرفوعة والأشجار التي تملكها أو تشارك فيها، من المراجعة إلى الشجرة الرقمية." />
    <div className="tree-library-actions"><Link className="primary-button" to="/import"><UploadCloud size={17} /> استيراد مشجر</Link><button className="secondary-button" type="button" aria-expanded={Boolean(search.create)} onClick={() => update({ ...search, create: search.create ? undefined : true })}><Plus size={16} /> شجرة جديدة</button></div>
    {search.create ? <NewTreeForm onCancel={() => update({ ...search, create: undefined })} /> : null}
    <nav className="workbench-tabs" aria-label="أقسام أشجاري">
      <Link to="/tree" search={{ q: search.q }} activeOptions={{ exact: true, includeSearch: true }} aria-current={view === "all" ? "page" : undefined} className={view === "all" ? "is-current" : ""}>كل العمل <span>{chartsQuery.isSuccess && treesQuery.isSuccess ? charts.length + trees.length : "—"}</span></Link>
      <Link to="/tree" search={{ q: search.q, view: "review" }} activeOptions={{ exact: true, includeSearch: true }} aria-current={reviewView ? "page" : undefined} className={reviewView ? "is-current" : ""}>مراجعة الأسماء <span>{chartsQuery.isSuccess ? reviewable.length : "—"}</span></Link>
      <Link to="/tree" search={{ q: search.q, view: "trees" }} activeOptions={{ exact: true, includeSearch: true }} aria-current={view === "trees" ? "page" : undefined} className={view === "trees" ? "is-current" : ""}>أشجار رقمية <span>{treesQuery.isSuccess ? trees.length : "—"}</span></Link>
    </nav>
    {sessionQuery.data === false ? <div className="library-access-note"><LockKeyhole size={18} /><div><strong>سجّل الدخول للوصول إلى عملك المحفوظ</strong><p>تظهر مشجراتك واقتراحات الأسماء والأشجار الخاصة والمشتركة هنا بعد تسجيل الدخول.</p></div><a className="inline-link" href={`/login?returnTo=${encodeURIComponent(`/tree${reviewView ? "?view=review" : view === "trees" ? "?view=trees" : ""}`)}`}>تسجيل الدخول <ArrowLeft size={15} /></a></div> : reviewView ? <section className="workbench-review-brief" aria-labelledby="review-queue-heading"><div><h2 id="review-queue-heading">قائمة مراجعة الأسماء</h2><p>{chartsQuery.isSuccess ? `${pendingNames.toLocaleString("ar-SA")} اسماً بانتظار المراجعة · ${deferredNames.toLocaleString("ar-SA")} مؤجلاً في ${reviewable.length.toLocaleString("ar-SA")} مشجرات.` : "نحمّل قوائم المراجعة المحفوظة…"} افتح المشجر لمراجعة أسمائه على الأصل.</p></div>{first?.fileId && !loading ? <Link className="primary-button" to="/tree/charts/$fileId/names" params={{ fileId: first.fileId }} search={{ queue: reviewQueue(first) }}><ScanSearch size={17} /> ابدأ المراجعة</Link> : null}</section> : <div className="tree-library-explanation"><FileText size={18} /><p>{view === "trees" ? "أشجارك الرقمية الخاصة والمشتركة. افتح الشجرة لتحرير مسودتها أو تصفح نسخها المنشورة." : "افتح أي مشجر لترى تقدمه وتتابع الأسماء والوصلات من مساحته الخاصة."}</p>{view === "all" ? <Link className="inline-link" to="/tree" search={{ view: "review" }}><ScanSearch size={16} /> مراجعة الأسماء</Link> : <Link className="inline-link" to="/explore">تصفح أشجار المنصة <ArrowLeft size={15} /></Link>}</div>}
    <div className="tree-library-toolbar"><label className="tree-library-search"><Search size={18} /><input aria-label="البحث في الأشجار والمشجرات" placeholder={reviewView ? "ابحث في مشجرات المراجعة" : "ابحث عن مشجر أو شجرة"} value={q} onChange={(event) => update({ ...search, q: event.target.value || undefined })} /></label>{reviewView ? <div className="workbench-queue-filters" role="group" aria-label="قائمة المراجعة"><button type="button" aria-pressed={!search.queue} onClick={() => update({ ...search, queue: undefined })}>تحتاج مراجعة</button><button type="button" aria-pressed={search.queue === "deferred"} onClick={() => update({ ...search, queue: "deferred" })}>المؤجلة <span>{chartsQuery.isSuccess ? deferredNames.toLocaleString("ar-SA") : "—"}</span></button></div> : search.kind === "charts" ? <button className="inline-link" type="button" onClick={() => update({ ...search, kind: undefined })}>مشجرات مرفوعة فقط · عرض الكل</button> : null}</div>
    {error ? <div className="evidence-workspace-error" role="alert">تعذر تحميل جزء من المكتبة. {error instanceof Error ? error.message : "حاول مرة أخرى."}<button className="inline-link" type="button" onClick={() => { if (view !== "trees") void chartsQuery.refetch(); if (!reviewView) void treesQuery.refetch(); }}>إعادة المحاولة</button></div> : null}
    {loading ? <p className="library-loading" role="status">جارٍ تحميل العمل المحفوظ…</p> : <>
      <p className="tree-library-result-count">{shownCount.toLocaleString("ar-SA")} {reviewView ? "مشجرات في قائمة المراجعة" : q ? "نتيجة" : "عنصراً محفوظاً"}{error ? " · القائمة غير مكتملة" : ""}</p>
      {shownCount ? <div className={`tree-library-list${reviewView ? " review-queue-list" : ""}`} aria-label={reviewView ? "مشجرات مراجعة الأسماء" : "الأشجار والمشجرات المحفوظة"}>
        {reviewView ? <div className="review-queue-head" aria-hidden="true"><span>المشجر</span><span>بانتظار المراجعة</span><span>مؤجلة</span><span>قرارات محفوظة</span><span>الإجراء</span></div> : <div className="tree-library-list-head" aria-hidden="true"><span>العنوان</span><span>المرحلة الحالية</span><span>متابعة العمل</span></div>}
        {filteredCharts.map((item) => reviewView ? <article className="tree-library-row review-queue-row" key={item.fileId}><ChartTitle item={item} /><div className="review-queue-number"><span>بانتظار المراجعة</span><strong>{item.pendingNames.toLocaleString("ar-SA")}</strong></div><div className="review-queue-number"><span>مؤجلة</span><strong>{item.deferredNames.toLocaleString("ar-SA")}</strong></div><div className="review-queue-number"><span>قرارات محفوظة</span><strong>{item.reviewedNames.toLocaleString("ar-SA")}</strong></div><div className="tree-library-row-actions"><Link className="primary-button" to="/tree/charts/$fileId/names" params={{ fileId: item.fileId! }} search={{ queue: reviewQueue(item) }}><ScanSearch size={15} /> متابعة المراجعة</Link></div></article> : <ChartLibraryRow key={item.fileId ?? item.sourceId} item={item} />)}
        {filteredTrees.map((tree) => <article className="tree-library-row" key={tree.id}><div className="tree-library-record"><GitBranch size={22} /><div><h2><Link to="/tree/$treeId" params={{ treeId: tree.id }}>{tree.name}</Link></h2><p>شجرة رقمية · {tree.visibility === "private" ? "خاصة" : tree.visibility === "unlisted" ? "غير مدرجة" : "عامة"}</p></div></div><div className="tree-library-progress"><strong>{tree.latestState === "published" ? "منشورة" : "مسودة"}</strong><span>{tree.people} أشخاص · {tree.relationships} علاقات</span></div><div className="tree-library-row-actions"><Link className="secondary-button" to="/tree/$treeId" params={{ treeId: tree.id }}>فتح الشجرة <ArrowLeft size={15} /></Link></div></article>)}
      </div> : !error ? <div className="tree-library-empty"><GitBranch size={28} /><h2>{q ? "لا توجد نتائج بهذا الاسم" : sessionQuery.data === false ? "مشجراتك الخاصة تظهر بعد تسجيل الدخول" : reviewView ? search.queue === "deferred" ? "لا توجد أسماء مؤجلة" : "لا توجد أسماء بانتظار المراجعة" : view === "trees" ? "لم تُنشأ شجرة رقمية بعد" : "لم يُحفظ عمل هنا بعد"}</h2><p>{q ? "جرّب كلمة أخرى، أو اعرض جميع العناصر." : reviewView ? "تجد المشجرات المكتملة والملفات التي تحتاج إعداد الأسماء في «كل العمل»." : "ابدأ باستيراد مشجر أو إنشاء شجرة رقمية، أو استكشف الأشجار المنشورة."}</p>{q ? <button className="secondary-button" type="button" onClick={() => update({ ...search, q: undefined })}>مسح البحث</button> : reviewView ? <Link className="secondary-button" to="/tree">كل العمل</Link> : <Link className="primary-button" to={sessionQuery.data === false ? "/explore" : "/import"}>{sessionQuery.data === false ? "تصفح الأشجار المنشورة" : "استيراد مشجر"}</Link>}</div> : null}
    </>}
  </div>;
}

function ChartTitle({ item }: { item: ChartLibraryItem }) {
  return <div className="tree-library-record"><FileText size={22} /><div><h2>{item.fileId ? <Link to="/tree/charts/$fileId" params={{ fileId: item.fileId }}>{item.titleAr}</Link> : <Link to="/sources" search={{ sourceId: item.sourceId }}>{item.titleAr}</Link>}</h2><p>{item.filename || "مشجر مرفوع"}</p>{item.fileId ? <Link className="chart-overview-link" to="/tree/charts/$fileId" params={{ fileId: item.fileId }}>نظرة عامة <ArrowLeft size={12} /></Link> : null}</div></div>;
}

function ChartLibraryRow({ item }: { item: ChartLibraryItem }) {
  const reviewing = item.pendingNames + item.deferredNames;
  const reading = item.readingStatus === "queued" || item.readingStatus === "running";
  const importSearch = { sourceId: item.sourceId, fileId: item.fileId, draftId: item.draftId };
  return <article className="tree-library-row"><ChartTitle item={item} /><div className="tree-library-progress"><strong>{item.draftState === "imported" ? "أُنشئت الشجرة" : reviewing ? `${reviewing.toLocaleString("ar-SA")} اسماً للمراجعة` : reading ? "جارٍ إعداد الأسماء" : item.totalNames ? "اكتملت مراجعة الأسماء" : item.fileId ? "تحتاج إعداد الأسماء" : "بانتظار الملف"}</strong><span>{item.reviewedNames ? `${item.reviewedNames.toLocaleString("ar-SA")} قرار محفوظ` : item.processingStatus === "failed" ? "تحتاج معالجة الملف إلى إعادة محاولة" : "مشجر مرفوع"}{item.deferredNames ? ` · ${item.deferredNames.toLocaleString("ar-SA")} مؤجل` : ""}</span></div><div className="tree-library-row-actions">{item.fileId ? <><Link className="primary-button" to="/tree/charts/$fileId" params={{ fileId: item.fileId }}>فتح مساحة المشجر <ArrowLeft size={15} /></Link><Link className="secondary-button" to="/tree/charts/$fileId/names" params={{ fileId: item.fileId }} search={{ queue: !item.pendingNames && item.deferredNames ? "deferred" : item.totalNames && !reviewing ? "history" : "pending" }}><ScanSearch size={15} /> {item.totalNames && !reviewing ? "سجل الأسماء" : "مراجعة الأسماء"}</Link></> : <Link className="secondary-button" to="/import" search={importSearch}>إرفاق ملف المشجر</Link>}</div></article>;
}

function NewTreeForm({ onCancel }: { onCancel: () => void }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [firstPersonName, setFirstPersonName] = useState("");
  const [visibility, setVisibility] = useState<"private" | "unlisted" | "public">("private");
  const mutation = useMutation({ mutationFn: () => createTree({ name_ar: name, description_ar: description, visibility, people: firstPersonName ? [{ canonical_name_ar: firstPersonName }] : [] }), onSuccess: async (created) => { await queryClient.invalidateQueries({ queryKey: ["trees"] }); await navigate({ to: "/tree/$treeId/versions/$versionId", params: { treeId: created.tree.id, versionId: created.selectedVersion.id } }); } });
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  return <form className="tree-library-create" onSubmit={submit}><h2>شجرة رقمية جديدة</h2><div className="evidence-form-grid"><label className="composer-label">اسم الشجرة<input required maxLength={200} value={name} onChange={(event) => setName(event.target.value)} /></label><label className="composer-label">الوصف<input value={description} onChange={(event) => setDescription(event.target.value)} /></label><label className="composer-label">الظهور<select value={visibility} onChange={(event) => setVisibility(event.target.value as typeof visibility)}><option value="private">خاصة</option><option value="unlisted">غير مدرجة</option><option value="public">عامة</option></select></label><label className="composer-label">أول شخص (اختياري)<input value={firstPersonName} onChange={(event) => setFirstPersonName(event.target.value)} /></label></div>{mutation.error ? <p className="evidence-workspace-error" role="status">{mutation.error instanceof Error && "status" in mutation.error && mutation.error.status === 401 ? "سجّل الدخول لإنشاء شجرة وحفظها." : mutation.error instanceof Error ? mutation.error.message : "تعذر حفظ الشجرة."}</p> : null}<div className="tree-library-actions"><button className="primary-button" type="submit" disabled={mutation.isPending}>{mutation.isPending ? "جارٍ الحفظ…" : "احفظ كمسودة"}</button><button className="secondary-button" type="button" onClick={onCancel}>إلغاء</button></div></form>;
}
