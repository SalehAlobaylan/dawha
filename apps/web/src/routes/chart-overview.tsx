import { useChartWorkspace } from "../lib/use-chart-workspace";
import { Link, useParams } from "@tanstack/react-router";
import { ArrowLeft, FileText, GitBranch, ScanSearch } from "lucide-react";
import { ChartWorkspaceHeader, ChartWorkspaceUnavailable } from "../components/ChartWorkspace";

export function ChartOverviewPage() {
  const { fileId } = useParams({ from: "/tree/charts/$fileId" });
  const { query, item } = useChartWorkspace(fileId);
  if (!item) return <ChartWorkspaceUnavailable fileId={fileId} query={query} />;
  const remaining = item.pendingNames + item.deferredNames;
  const queue = item.pendingNames ? "pending" : item.deferredNames ? "deferred" : item.totalNames ? "history" : "pending";
  const reading = item.readingStatus === "queued" || item.readingStatus === "running";
  const imported = item.draftState === "imported" && Boolean(item.treeId);
  const needsPreparation = item.totalNames === 0;
  const nextIsNames = !imported && (remaining > 0 || needsPreparation);
  const importSearch = { sourceId: item.sourceId, fileId, draftId: item.draftId };

  return <div className="page-stack chart-workspace-page">
    <ChartWorkspaceHeader item={item} current="overview" />
    <section className="workbench-review-brief chart-workspace-next" aria-labelledby="chart-next-heading"><div><div className="eyebrow">الخطوة التالية</div><h2 id="chart-next-heading">{imported ? "شجرتك الرقمية جاهزة" : item.processingStatus === "failed" ? "تحتاج قراءة الملف إلى إعادة محاولة" : nextIsNames ? reading && needsPreparation ? "جارٍ إعداد الأسماء" : needsPreparation ? "جهّز أسماء المشجر للمراجعة" : "تابع مراجعة الأسماء على الأصل" : "راجع الوصلات وأنشئ الشجرة"}</h2><p>{imported ? "يمكنك فتح الشجرة الرقمية أو العودة إلى قرارات الأسماء والوصلات المحفوظة." : remaining ? `${item.pendingNames.toLocaleString("ar-SA")} بانتظار المراجعة و${item.deferredNames.toLocaleString("ar-SA")} مؤجلاً. قراراتك السابقة محفوظة.` : reading ? "يمكنك متابعة حالة القراءة دون بدء تشغيل جديد." : needsPreparation ? "افتح مراجعة الأسماء لبدء إعداد الاقتراحات أو متابعة قراءتها." : "اكتملت قائمة الأسماء الحالية. تحقّق من الوصلات قبل إنشاء الشجرة."}</p></div>{imported ? <Link className="primary-button" to="/tree/$treeId" params={{ treeId: item.treeId! }}>فتح الشجرة <ArrowLeft size={16} /></Link> : item.processingStatus === "failed" ? <Link className="primary-button" to="/import" search={importSearch}>متابعة معالجة الملف <ArrowLeft size={16} /></Link> : nextIsNames ? <Link className="primary-button" to="/tree/charts/$fileId/names" params={{ fileId }} search={{ queue }}><ScanSearch size={16} /> {needsPreparation ? "إعداد الأسماء" : "متابعة المراجعة"}</Link> : <Link className="primary-button" to="/import" search={importSearch}><GitBranch size={16} /> الوصلات والشجرة</Link>}</section>
    <dl className="chart-review-totals" aria-label="تقدم مراجعة المشجر"><div><dt>اقتراحات الأسماء</dt><dd>{item.totalNames.toLocaleString("ar-SA")}</dd></div><div><dt>بانتظار المراجعة</dt><dd>{item.pendingNames.toLocaleString("ar-SA")}</dd></div><div><dt>مؤجلة</dt><dd>{item.deferredNames.toLocaleString("ar-SA")}</dd></div><div><dt>قرارات محفوظة</dt><dd><Link to="/tree/charts/$fileId/names" params={{ fileId }} search={{ queue: "history" }}>{item.reviewedNames.toLocaleString("ar-SA")} <span>فتح السجل <ArrowLeft size={13} /></span></Link></dd></div></dl>
    <section className="chart-workspace-progress" aria-labelledby="chart-progress-heading"><h2 id="chart-progress-heading">مراحل العمل على المشجر</h2><ol>
      <li><FileText size={21} /><div><h3>الملف الأصلي</h3><p>{item.filename}</p><span>{item.processingStatus === "succeeded" ? "اكتملت معالجة الملف" : item.processingStatus === "failed" ? "تعذرت المعالجة — الملف الأصلي محفوظ" : "جارٍ تجهيز الملف"} · {item.visibility === "private" ? "مصدر خاص" : "مصدر عام"}</span></div><Link className="inline-link" to="/sources" search={{ sourceId: item.sourceId }}>فتح المصدر <ArrowLeft size={14} /></Link></li>
      <li><ScanSearch size={21} /><div><h3>الأسماء والقرارات</h3><p>{remaining ? `${remaining.toLocaleString("ar-SA")} اسماً تحتاج إلى قرار، بما فيها المؤجلة.` : item.totalNames ? "اكتملت مراجعة الأسماء الحالية. يمكنك فتح السجل." : reading ? "القراءة جارية." : "لم تُجهّز اقتراحات الأسماء بعد."}</p></div><Link className="inline-link" to="/tree/charts/$fileId/names" params={{ fileId }} search={{ queue }}>{remaining ? "متابعة الأسماء" : item.totalNames ? "سجل الأسماء" : "إعداد الأسماء"} <ArrowLeft size={14} /></Link></li>
      <li><GitBranch size={21} /><div><h3>الوصلات والشجرة</h3><p>{imported ? "أُنشئت شجرة رقمية من هذا المشجر." : item.draftState === "retracted" ? "سُجل التراجع عن الاستيراد السابق." : item.draftId ? "توجد مسودة وصلات محفوظة. تابع من حيث توقفت." : "لم تُنشأ مسودة وصلات بعد."}</p></div><Link className="inline-link" to="/import" search={importSearch}>{item.draftId ? "فتح المسودة المحفوظة" : "مراجعة الوصلات"} <ArrowLeft size={14} /></Link></li>
    </ol></section>
    <Link className="inline-link" to="/tree" search={{ view: "review" }}>العودة إلى قائمة المراجعة <ArrowLeft size={15} /></Link>
  </div>;
}
