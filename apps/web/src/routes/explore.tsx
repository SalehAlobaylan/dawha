import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowLeft, Compass, GitBranch, Search } from "lucide-react";
import { TopBar } from "../components/TopBar";
import { fetchPublishedTrees } from "../lib/api";

export function ExplorePage() {
  const { q = "" } = useSearch({ from: "/explore" });
  const navigate = useNavigate();
  const query = useQuery({ queryKey: ["trees", "public"], queryFn: fetchPublishedTrees, staleTime: 0 });
  const needle = q.trim().normalize("NFKC");
  const trees = (query.data ?? []).filter((tree) => `${tree.name} ${tree.description}`.normalize("NFKC").includes(needle));

  return <div className="page-stack tree-library-page explore-page">
    <TopBar eyebrow="اكتشف / الأشجار" title="تصفح الأشجار" description="استكشف الأشجار التي نشرها أصحابها عبر دَوْحة، وافتح نسخها المنشورة." />
    <div className="tree-library-explanation"><Compass size={18} /><p>هنا الأشجار العامة المنشورة. مشجراتك المرفوعة ومسوداتك وأشجارك المشتركة تجدها في أشجاري.</p><Link className="inline-link" to="/tree"><GitBranch size={16} /> أشجاري</Link></div>
    <div className="tree-library-toolbar"><label className="tree-library-search"><Search size={18} /><input aria-label="البحث في الأشجار المنشورة" placeholder="ابحث باسم شجرة أو وصفها" value={q} onChange={(event) => { void navigate({ to: "/explore", search: { q: event.target.value || undefined }, replace: true, resetScroll: false }); }} /></label><Link className="inline-link" to="/sources">تصفح المصادر <ArrowLeft size={15} /></Link></div>
    {query.isPending ? <p className="library-loading" role="status">جارٍ تحميل الأشجار المنشورة…</p> : query.error ? <div className="evidence-workspace-error" role="alert">تعذر تحميل الأشجار المنشورة. <button className="inline-link" type="button" onClick={() => void query.refetch()}>إعادة المحاولة</button></div> : <>
      <p className="tree-library-result-count">{trees.length.toLocaleString("ar-SA")} {q ? "نتيجة" : "شجرة منشورة"}</p>
      {trees.length ? <div className="tree-library-list" aria-label="الأشجار المنشورة عبر المنصة">
        <div className="tree-library-list-head" aria-hidden="true"><span>الشجرة</span><span>النسخة المنشورة</span><span>التصفح</span></div>
        {trees.map((tree) => <article className="tree-library-row" key={tree.id}>
          <div className="tree-library-record"><GitBranch size={22} /><div><h2><Link to="/tree/$treeId/versions/$versionId" params={{ treeId: tree.id, versionId: tree.latestVersionId }} search={{ from: "explore" }}>{tree.name}</Link></h2><p>{tree.description || "شجرة عامة منشورة"}</p></div></div>
          <div className="tree-library-progress"><strong>الإصدار {tree.latestVersionNumber.toLocaleString("ar-SA")}</strong><span>{tree.people.toLocaleString("ar-SA")} أشخاص · {tree.relationships.toLocaleString("ar-SA")} علاقات</span></div>
          <div className="tree-library-row-actions"><Link className="secondary-button" to="/tree/$treeId/versions/$versionId" params={{ treeId: tree.id, versionId: tree.latestVersionId }} search={{ from: "explore" }}>تصفح الشجرة <ArrowLeft size={15} /></Link></div>
        </article>)}
      </div> : <div className="tree-library-empty"><Compass size={28} /><h2>{q ? "لا توجد أشجار مطابقة" : "لا توجد أشجار منشورة بعد"}</h2><p>{q ? "جرّب اسماً آخر أو اعرض جميع الأشجار المنشورة." : "تظهر هنا الأشجار العامة بعد نشر أول نسخة منها."}</p>{q ? <Link className="secondary-button" to="/explore" search={{}}>عرض الكل</Link> : <Link className="secondary-button" to="/tree">افتح أشجاري</Link>}</div>}
    </>}
  </div>;
}
