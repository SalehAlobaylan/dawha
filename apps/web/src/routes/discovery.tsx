import { Link, useSearch } from "@tanstack/react-router";
import { SearchPage } from "../components/SearchPage";
import { DictionaryPage } from "../components/DictionaryPage";
import { TopBar } from "../components/TopBar";

export function DiscoveryPage() {
  const search = useSearch({ from: "/search" });
  return <div className="page-stack discovery-page"><TopBar eyebrow="البحث / الفهرس" title="ابحث في الذاكرة" description="اعثر على اسم أو نص، ثم افتح سجله ومصادره في السياق الصحيح." />
    <nav className="workbench-tabs" aria-label="أقسام البحث والفهرس"><Link to="/search" search={{ q: search.q }} activeOptions={{ includeSearch: true }} className={!search.view ? "is-current" : ""}>البحث في المحتوى</Link><Link to="/search" search={{ q: search.q, view: "index", kind: "people" }} activeOptions={{ includeSearch: true }} className={search.view === "index" ? "is-current" : ""}>فهرس الأسماء والسجلات</Link></nav>
    {search.view === "index" ? <DictionaryPage key={`${search.kind}:${search.q}`} embedded /> : <SearchPage key={`${search.q}:${search.kind}:${search.status}`} embedded />}
  </div>;
}
