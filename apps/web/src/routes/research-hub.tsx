import { Link, useSearch } from "@tanstack/react-router";
import { ResearchAccess } from "../components/ResearchAccess";
import { TopBar } from "../components/TopBar";
import { EvidenceResearchPanel } from "./research";
import { QuestionsPage } from "./questions";
import { EntityResolutionPage } from "./entity-resolution";
import { ContradictionsPage } from "./contradictions";

export function ResearchHubPage() {
  const search = useSearch({ from: "/research" });
  const view = search.view ?? "questions";
  return <div className="page-stack research-hub-page">
    <TopBar eyebrow="مكتب البحث / دَوْحة" title="مكتب البحث" description="سؤال واضح، وأدلة محفوظة، ومراجعة يمكن العودة إليها." />
    <nav className="workbench-tabs" aria-label="أقسام مكتب البحث">
      <Link to="/research" search={{}} className={view === "questions" ? "is-current" : ""} activeOptions={{ exact: true, includeSearch: true }}>الأسئلة</Link>
      <Link to="/research" search={{ ...search, view: "evidence", create: undefined, runId: undefined }} className={view === "evidence" ? "is-current" : ""} activeOptions={{ includeSearch: true }}>بحث الأدلة</Link>
      <Link to="/research" search={{ view: "review" }} className={view === "review" ? "is-current" : ""} activeOptions={{ includeSearch: true }}>مراجعة البيانات</Link>
    </nav>
    {view === "questions" ? <QuestionsPage /> : view === "evidence" ? <EvidenceResearchPanel /> : <>
      <div className="research-review-intro"><h2>راجع الهوية والروايات في مكان واحد</h2><p>المطابقة تقارن السجلات، وفحص التعارضات يقارن الادعاءات. كل قرار يبقى محفوظاً للمراجعة.</p></div>
      <nav className="review-tool-tabs" aria-label="نوع مراجعة البيانات"><Link to="/research" search={{ view: "review", review: "identity" }} className={search.review !== "contradictions" ? "is-current" : ""}>مطابقة الهوية</Link><Link to="/research" search={{ view: "review", review: "contradictions" }} className={search.review === "contradictions" ? "is-current" : ""}>تعارض الروايات</Link></nav>
      <ResearchAccess>{search.review === "contradictions" ? <ContradictionsPage embedded /> : <EntityResolutionPage embedded />}</ResearchAccess>
    </>}
  </div>;
}
