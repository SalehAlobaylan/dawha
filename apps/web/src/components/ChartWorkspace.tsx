import { Link } from "@tanstack/react-router";
import type { useChartWorkspace } from "../lib/use-chart-workspace";
import { TopBar } from "./TopBar";
import type { ChartLibraryItem } from "../types";

export function ChartWorkspaceNavigation({ item, current }: { item: ChartLibraryItem; current: "overview" | "names" | "connections" }) {
  if (!item.fileId) return null;
  return <nav className="workbench-tabs chart-workspace-tabs" aria-label="أقسام المشجر">
    <Link to="/tree/charts/$fileId" params={{ fileId: item.fileId }} activeOptions={{ exact: true }} className={current === "overview" ? "is-current" : ""}>نظرة عامة</Link>
    <Link to="/tree/charts/$fileId/names" params={{ fileId: item.fileId }} className={current === "names" ? "is-current" : ""}>مراجعة الأسماء</Link>
    <Link to="/import" search={{ sourceId: item.sourceId, fileId: item.fileId, draftId: item.draftId }} className={current === "connections" ? "is-current" : ""} aria-current={current === "connections" ? "page" : undefined}>الوصلات والشجرة</Link>
    <Link to="/sources" search={{ sourceId: item.sourceId }}>المصدر الأصلي</Link>
  </nav>;
}

export function ChartWorkspaceHeader({ item, current }: { item: ChartLibraryItem; current: "overview" | "names" }) {
  return <>
    <nav className="toolbar-breadcrumb chart-workspace-breadcrumb" aria-label="مسار المشجر"><Link to="/tree">أشجاري</Link><span>/</span><span>{current === "names" ? "مراجعة الأسماء" : "مساحة المشجر"}</span></nav>
    <TopBar eyebrow="أشجاري / مساحة المشجر" title={item.titleAr} description={item.filename ?? "الملف المحفوظ ومراحل مراجعته في مكان واحد."} />
    <ChartWorkspaceNavigation item={item} current={current} />
  </>;
}

export function ChartWorkspaceUnavailable({ fileId, query }: { fileId: string; query: ReturnType<typeof useChartWorkspace>["query"] }) {
  return <div className="page-stack chart-workspace-page"><Link className="inline-link" to="/tree">العودة إلى أشجاري</Link><h1>مساحة المشجر</h1>{query.isPending ? <p role="status">جارٍ فتح المشجر…</p> : query.error ? <p role="alert">تعذر تحميل المشجر. <button className="inline-link" type="button" onClick={() => void query.refetch()}>إعادة المحاولة</button></p> : <div className="library-access-note"><p>لم نعثر على ملف متاح للمراجعة بهذا الحساب. سجّل الدخول بالحساب الذي رفع المشجر أو الذي يملك حق مراجعته.</p><a className="primary-button" href={`/login?returnTo=${encodeURIComponent(`/tree/charts/${fileId}`)}`}>تسجيل الدخول</a></div>}</div>;
}
