import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { FileText, RefreshCw } from "lucide-react";
import { useState } from "react";
import { ApiError, fetchSourceProcessing, fetchSources } from "../lib/api";

const labels = { queued: "بانتظار المعالجة", running: "جارٍ تجهيز الملف", succeeded: "جاهز للمراجعة", failed: "تحتاج المعالجة إلى متابعة" };
export function SourceProcessingStatus() {
  const search = useSearch({ from: "/sources" });
  const navigate = useNavigate();
  const [status, setStatus] = useState("");
  const sources = useQuery({ queryKey: ["sources"], queryFn: fetchSources });
  const processing = useQuery({ queryKey: ["source-processing", search.sourceId], queryFn: () => fetchSourceProcessing(search.sourceId!), enabled: Boolean(search.sourceId), staleTime: 0, refetchInterval: (query) => query.state.data?.files.some((file) => file.processingStatus === "queued" || file.processingStatus === "running") ? 3000 : false });
  const files = processing.data?.files ?? [];
  const visible = files.filter((file) => !status || file.processingStatus === status);
  const error = sources.error ?? processing.error;
  return <section className="source-processing-overview"><div className="research-review-intro"><h2>تابع تجهيز مصادر مساحتك</h2><p>اختر مصدراً لترى ملفاته وحالة معالجتها. افتح الملف من المصدر لمراجعة الأسماء أو متابعة أي خطأ.</p></div>
    <div className="processing-status-toolbar"><label className="composer-label">المصدر<select aria-label="مصدر المعالجة" value={search.sourceId ?? ""} onChange={(event) => { setStatus(""); void navigate({ to: "/sources", search: { view: "processing", sourceId: event.target.value || undefined }, replace: true }); }}><option value="">اختر مصدراً محفوظاً</option>{sources.data?.map((source) => <option key={source.id} value={source.id}>{source.titleAr}</option>)}</select></label><label className="composer-label">حالة الملف<select aria-label="حالة الملف" value={status} onChange={(event) => setStatus(event.target.value)}><option value="">كل الحالات</option>{Object.entries(labels).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label><button className="secondary-button" type="button" onClick={() => { void sources.refetch(); if (search.sourceId) void processing.refetch(); }}><RefreshCw size={15} /> تحديث الحالة</button></div>
    {error ? <div className="entity-resolution-error" role="alert">{error instanceof ApiError && (error.status === 401 || error.status === 403) ? "تحتاج إلى تسجيل الدخول بحساب له حق مراجعة ملفات هذا المصدر." : "تعذر تحميل حالة المعالجة. أعد المحاولة."}</div> : sources.isPending || search.sourceId && processing.isPending ? <p role="status">جارٍ تحميل حالة الملفات…</p> : !search.sourceId ? <div className="research-access-note"><FileText size={23} /><h3>{sources.data?.length ? "اختر المصدر الذي تريد متابعته" : "لا توجد مصادر متاحة"}</h3><p>تظهر هنا ملفات المصادر المحفوظة في حسابك، دون الحاجة إلى إدارة طابور التشغيل يدوياً.</p></div> : <>
      <p className="tree-library-result-count">{visible.length.toLocaleString("ar-SA")} ملفات · {files.filter((file) => file.processingStatus === "failed").length.toLocaleString("ar-SA")} تحتاج متابعة</p>
      <div className="processing-file-list">{visible.map((file) => <article key={file.id}><FileText size={20} /><div><h3>{file.originalFilenameAr}</h3><p>{labels[file.processingStatus]}</p>{file.processingError ? <details><summary>تفاصيل تعذر المعالجة</summary><p>{file.processingError}</p></details> : null}</div><Link className="secondary-button" to="/sources" search={{ sourceId: file.sourceId }}>{file.processingStatus === "failed" ? "افتح المصدر لمعالجة الخطأ" : "افتح المصدر للمراجعة"}</Link></article>)}</div>
      {!visible.length ? <p className="entity-resolution-empty">{files.length ? "لا توجد ملفات بهذه الحالة." : "لم يُرفع ملف لهذا المصدر بعد. افتح المصدر لإرفاقه."}</p> : null}
      <Link className="inline-link" to="/sources" search={{ sourceId: search.sourceId }}>فتح مساحة المصدر</Link>
    </>}
  </section>;
}
