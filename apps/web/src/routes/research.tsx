import { Link, useSearch } from "@tanstack/react-router";
import { ArrowLeft, BookOpen, CheckCircle2, CircleAlert, FileSearch, Filter, GitBranch, Link2, Plus, Search, Send, Sparkles } from "lucide-react";
import { useMemo, useState } from "react";
import { sources } from "../data/demo";
import { queryGraphAncestorFrontier, queryGraphBranchStructureComparison, queryGraphRelationshipImpact, queryResearch } from "../lib/api";
import type { EpistemicTone, GraphAncestorFrontierResult, GraphBranchStructureComparisonResult, GraphOperation, GraphPath, GraphRelationshipImpactResult, GraphStats, ResearchCitation, ResearchQueryResult, ResearchRoute } from "../types";
import { EvidenceComparison, SourceCard } from "../components/EvidencePanels";
import { SectionHeading } from "../components/SectionHeading";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

const researchTabs = ["الكل", "ادعاءات", "مصادر", "أسئلة", "ملاحظات النظام"];

export function ResearchPage() {
  const contextSearch = useSearch({ from: "/research" });
  const [activeTab, setActiveTab] = useState("الكل");
  const [search, setSearch] = useState("");
  const [composerOpen, setComposerOpen] = useState(false);
  const [researchQuestion, setResearchQuestion] = useState("");
  const [researchResult, setResearchResult] = useState<ResearchQueryResult | null>(null);
  const [relationshipImpactResult, setRelationshipImpactResult] = useState<GraphRelationshipImpactResult | null>(null);
  const [branchComparisonResult, setBranchComparisonResult] = useState<GraphBranchStructureComparisonResult | null>(null);
  const [ancestorFrontierResult, setAncestorFrontierResult] = useState<GraphAncestorFrontierResult | null>(null);
  const [researchError, setResearchError] = useState("");
  const [researchLoading, setResearchLoading] = useState(false);
  const [graphOperation, setGraphOperation] = useState<GraphOperation | "">("");
  const [graphStartID, setGraphStartID] = useState("");
  const [graphEndID, setGraphEndID] = useState("");
  const [graphRelationshipID, setGraphRelationshipID] = useState("");
  const [graphTreeID, setGraphTreeID] = useState(() => isUuid(contextSearch.treeId) ? contextSearch.treeId || "" : "");
  const [graphTreeVersionID, setGraphTreeVersionID] = useState(() => isUuid(contextSearch.treeVersionId) ? contextSearch.treeVersionId || "" : "");
  const [graphRootNodeID, setGraphRootNodeID] = useState("");
  const [graphMaxDepth, setGraphMaxDepth] = useState(2);
  const [fromTreeID, setFromTreeID] = useState(() => isUuid(contextSearch.treeId) ? contextSearch.treeId || "" : "");
  const [fromTreeVersionID, setFromTreeVersionID] = useState(() => isUuid(contextSearch.treeVersionId) ? contextSearch.treeVersionId || "" : "");
  const [fromRootNodeID, setFromRootNodeID] = useState("");
  const [toTreeID, setToTreeID] = useState("");
  const [toTreeVersionID, setToTreeVersionID] = useState("");
  const [toRootNodeID, setToRootNodeID] = useState("");
  const contextualEntityType = isUuid(contextSearch.entityId) ? contextSearch.entityType : undefined;
  const contextualEntityID = isUuid(contextSearch.entityId) ? contextSearch.entityId : "";
  const graphStartIDValue = graphOperation === "source_entities" ? graphStartID : graphStartID || contextualEntityID;
  const graphTreeScoped = graphOperation === "shortest_relationship_path" || graphOperation === "connected_component" || graphOperation === "relationship_impact" || graphOperation === "ancestor_frontier";
  const graphEndRequired = graphOperation === "common_ancestor_path" || graphOperation === "evidence_connection" || graphOperation === "geographic_path" || graphOperation === "shortest_relationship_path";
  const visibleSources = useMemo(() => {
    const normalized = search.trim();
    if (!normalized) return sources.slice(0, 3);
    return sources.filter((source) => `${source.title} ${source.excerpt} ${source.type}`.includes(normalized)).slice(0, 3);
  }, [search]);

  const runResearch = async () => {
    const question = researchQuestion.trim();
    const relationshipImpactMode = graphOperation === "relationship_impact";
    const branchComparisonMode = graphOperation === "branch_structure_comparison";
    const ancestorFrontierMode = graphOperation === "ancestor_frontier";
    if ((!question && !relationshipImpactMode && !branchComparisonMode && !ancestorFrontierMode) || researchLoading) return;
    setResearchLoading(true);
    setResearchError("");
    try {
      if (ancestorFrontierMode) {
        setResearchResult(null);
        setRelationshipImpactResult(null);
        setBranchComparisonResult(null);
        setAncestorFrontierResult(await queryGraphAncestorFrontier({ tree_id: graphTreeID, tree_version_id: graphTreeVersionID, root_node_id: graphRootNodeID, max_depth: graphMaxDepth }));
      } else if (branchComparisonMode) {
        setResearchResult(null);
        setRelationshipImpactResult(null);
        setAncestorFrontierResult(null);
        setBranchComparisonResult(await queryGraphBranchStructureComparison({ from_tree_id: fromTreeID, from_tree_version_id: fromTreeVersionID, from_root_node_id: fromRootNodeID, to_tree_id: toTreeID, to_tree_version_id: toTreeVersionID, to_root_node_id: toRootNodeID, max_depth: graphMaxDepth }));
      } else if (relationshipImpactMode) {
        setResearchResult(null);
        setBranchComparisonResult(null);
        setAncestorFrontierResult(null);
        setRelationshipImpactResult(await queryGraphRelationshipImpact({ tree_id: graphTreeID, tree_version_id: graphTreeVersionID, relationship_id: graphRelationshipID, max_depth: graphMaxDepth }));
      } else {
        setRelationshipImpactResult(null);
        setBranchComparisonResult(null);
        setAncestorFrontierResult(null);
        const graphStartType = graphStartTypeFor(graphOperation, contextualEntityType);
        const graphInput = graphOperation ? {
          graph_operation: graphOperation,
          graph_start_type: graphStartType,
          graph_start_id: graphStartIDValue,
          graph_end_type: graphOperation === "geographic_path" ? "place" as const : graphOperation === "source_entities" ? undefined : graphStartType,
          graph_end_id: graphEndRequired ? graphEndID || undefined : undefined,
          graph_max_depth: graphMaxDepth,
          tree_id: graphTreeScoped ? graphTreeID || undefined : undefined,
          tree_version_id: graphTreeScoped ? graphTreeVersionID || undefined : undefined,
        } : {};
        setResearchResult(await queryResearch({ question, entity_type: contextualEntityType, entity_id: contextualEntityID || undefined, tree_id: isUuid(contextSearch.treeId) ? contextSearch.treeId : undefined, tree_version_id: isUuid(contextSearch.treeVersionId) ? contextSearch.treeVersionId : undefined, ...graphInput }));
      }
    } catch (error) {
      setResearchResult(null);
      setRelationshipImpactResult(null);
      setBranchComparisonResult(null);
      setAncestorFrontierResult(null);
      setResearchError(error instanceof Error ? error.message : "تعذر تشغيل البحث.");
    } finally {
      setResearchLoading(false);
    }
  };

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="مكتب البحث / مساحة نجم"
        title="ابنِ سياقك، خطوة خطوة"
        description="اجمع السؤال، المصادر، الادعاءات، والأدلة المضادة في مساحة واحدة قابلة للتتبع."
      />

      {contextSearch.entityId ? <div className="research-context-chip"><GitBranch size={14} /><span>السياق الحالي: {contextSearch.entityType ?? "person"} · {contextSearch.entityId}</span><Link to="/research">مسح السياق</Link></div> : null}

      <section className="research-command-bar">
        <div className="research-command-copy"><Sparkles size={17} /><span>مساعد البحث يسأل ويقترح، لكنه لا يحسم.</span></div>
        <button className="secondary-button" type="button" onClick={() => setComposerOpen((open) => !open)}><Plus size={15} /> إضافة ادعاء</button>
        <button className="primary-button" type="button" onClick={() => { setComposerOpen(false); document.getElementById("research-question")?.focus(); }}><Search size={15} /> ابدأ من سؤال</button>
      </section>

      {composerOpen ? (
        <section className="claim-composer">
          <div className="composer-head"><div><div className="eyebrow">ادعاء جديد</div><h2>اكتب الفرضية قبل أن تجد الدليل</h2></div><StatusBadge tone="claim">غير محسوم بعد</StatusBadge></div>
          <label className="composer-label">صياغة الادعاء<input placeholder="مثال: عبدالله ابن محمد بن سعد" /></label>
          <div className="composer-grid"><label className="composer-label">الموضوع<select defaultValue="person"><option value="person">شخص</option><option value="family">عائلة</option><option value="tribe">قبيلة</option></select></label><label className="composer-label">الحالة الأولية<select defaultValue="unresolved"><option value="unresolved">غير محسوم</option><option value="supported">مدعوم مبدئياً</option><option value="disputed">متنازع عليه</option></select></label></div>
          <div className="composer-actions"><button className="secondary-button" type="button" onClick={() => setComposerOpen(false)}>إلغاء</button><button className="primary-button" type="button" onClick={() => setComposerOpen(false)}>حفظ كمسودة <Send size={14} /></button></div>
        </section>
      ) : null}

      <div className="research-tabs" role="tablist" aria-label="أنواع المواد">
        {researchTabs.map((tab) => <button key={tab} type="button" className={`research-tab${activeTab === tab ? " research-tab-active" : ""}`} onClick={() => setActiveTab(tab)}>{tab}{tab === "ادعاءات" ? <span>24</span> : tab === "أسئلة" ? <span>89</span> : null}</button>)}
      </div>

      <div className="research-layout">
        <main className="research-main-column">
          <section className="research-focus-card">
            <div className="research-focus-head"><div><div className="eyebrow">السؤال النشط</div><h2>من كان والد عبدالله في هذه الروايات؟</h2></div><StatusBadge tone="question">قيد التحقيق</StatusBadge></div>
            <form className="research-query-form" onSubmit={(event) => { event.preventDefault(); void runResearch(); }}>
              <label htmlFor="research-question">{graphOperation === "relationship_impact" ? "حلّل أثر علاقة في تفسير منشور" : graphOperation === "branch_structure_comparison" ? "قارن شكل الفروع في نسختين" : graphOperation === "ancestor_frontier" ? "استكشف إطار الأسلاف" : "اسأل عن أدلة مصدرة"}</label>
              <div className="research-query-input-row">
                <input id="research-question" value={researchQuestion} onChange={(event) => setResearchQuestion(event.target.value)} placeholder={graphOperation === "relationship_impact" || graphOperation === "branch_structure_comparison" || graphOperation === "ancestor_frontier" ? "اختياري: أضف سياقاً للسجل" : "مثال: من كان والد عبدالله في هذه الروايات؟"} />
                <button className="primary-button" type="submit" disabled={researchLoading || (!researchQuestion.trim() && graphOperation !== "relationship_impact" && graphOperation !== "branch_structure_comparison" && graphOperation !== "ancestor_frontier")}>{researchLoading ? "جارٍ البحث…" : graphOperation === "ancestor_frontier" ? "استكشف الإطار" : graphOperation === "branch_structure_comparison" ? "قارن البنية" : graphOperation === "relationship_impact" ? "حلّل الأثر" : "ابحث في الأدلة"}<Search size={15} /></button>
              </div>
            </form>
            <div className="research-graph-mode">
              <div className="research-graph-mode-copy"><GitBranch size={15} /><div><strong>مسار 관계</strong><small>اجعل الاستعلام يستخدم بنية relationships محدودة، مع إبقاء الأدلة قابلة للتتبع.</small></div></div>
              <div className="research-graph-fields">
                <label>نوع المسار<select value={graphOperation} onChange={(event) => setGraphOperation(event.target.value as GraphOperation | "")}><option value="">بدون مسار رسومي</option><option value="common_ancestor_path">سلف مشترك</option><option value="shortest_relationship_path">أقصر مسار بين شخصين</option><option value="connected_component">مكوّن متصل داخل تفسير منشور</option><option value="relationship_impact">أثر علاقة على الأبناء</option><option value="branch_structure_comparison">مقارنة بنية فرعين</option><option value="ancestor_frontier">إطار الأسلاف</option><option value="evidence_connection">رابط أدلة بين كيانين</option><option value="branch_claims">ادعاءات حول فرع أو كيان</option><option value="source_entities">كيانات مرتبطة بمصدر</option><option value="geographic_path">مسار جغرافي</option></select></label>
                {graphOperation && graphOperation !== "branch_structure_comparison" && graphOperation !== "ancestor_frontier" ? <><label>{graphOperation === "relationship_impact" ? "معرف العلاقة" : graphOperation === "connected_component" ? "الشخص الجذر" : "نقطة البداية"}<input required value={graphOperation === "relationship_impact" ? graphRelationshipID : graphStartIDValue} onChange={(event) => graphOperation === "relationship_impact" ? setGraphRelationshipID(event.target.value) : setGraphStartID(event.target.value)} placeholder="معرف UUID" /></label>{graphOperation !== "branch_claims" && graphOperation !== "source_entities" && graphOperation !== "connected_component" && graphOperation !== "relationship_impact" ? <label>{graphOperation === "geographic_path" ? "المكان المرجعي" : graphOperation === "shortest_relationship_path" ? "الشخص الآخر" : "نقطة النهاية"}<input required={graphEndRequired} value={graphEndID} onChange={(event) => setGraphEndID(event.target.value)} placeholder="معرف UUID" /></label> : null}{graphOperation !== "source_entities" ? <label>أقصى عمق<select value={graphMaxDepth} onChange={(event) => setGraphMaxDepth(Number(event.target.value))}><option value={1}>1</option><option value={2}>2</option><option value={3}>3</option></select></label> : null}{graphTreeScoped ? <><label>معرف الشجرة<input required value={graphTreeID} onChange={(event) => setGraphTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input required value={graphTreeVersionID} onChange={(event) => setGraphTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label></> : null}</> : null}
                {graphOperation === "branch_structure_comparison" ? <GraphBranchComparisonFields fromTreeID={fromTreeID} fromTreeVersionID={fromTreeVersionID} fromRootNodeID={fromRootNodeID} toTreeID={toTreeID} toTreeVersionID={toTreeVersionID} toRootNodeID={toRootNodeID} onFromTreeID={setFromTreeID} onFromTreeVersionID={setFromTreeVersionID} onFromRootNodeID={setFromRootNodeID} onToTreeID={setToTreeID} onToTreeVersionID={setToTreeVersionID} onToRootNodeID={setToRootNodeID} /> : null}
                {graphOperation === "ancestor_frontier" ? <GraphAncestorFrontierFields treeID={graphTreeID} treeVersionID={graphTreeVersionID} rootNodeID={graphRootNodeID} onTreeID={setGraphTreeID} onTreeVersionID={setGraphTreeVersionID} onRootNodeID={setGraphRootNodeID} /> : null}
              </div>
            </div>
            {researchError ? <p className="research-query-error">{researchError}</p> : null}
            <p className="research-focus-lead">المصادر المتاحة لا تحسم اسم الأب. هل توجد علاقة اعتماد بين المصدرين، أم روايتان مستقلتان؟</p>
            <div className="focus-claims">
              <div className="focus-claim focus-claim-supported"><div className="focus-claim-top"><StatusBadge tone="claim" compact>ادعاء ١</StatusBadge><span>مدعوم مبدئياً</span></div><strong>محمد بن سعد</strong><p>المصدر الأول يذكر أن عبدالله يتصل بمحمد.</p><div className="focus-claim-footer"><span><BookOpen size={13} /> مصدر أولي</span><span>١ من ٢</span></div></div>
              <div className="focus-claim focus-claim-disputed"><div className="focus-claim-top"><StatusBadge tone="disputed" compact>ادعاء ٢</StatusBadge><span>متنازع عليه</span></div><strong>صالح</strong><p>رواية لاحقة تذكر اسماً والداً آخر، مع شبهة اعتماد.</p><div className="focus-claim-footer"><span><CircleAlert size={13} /> لم يُتحقق من الاستقلال</span><span>١ من ٢</span></div></div>
            </div>
            <div className="focus-actions"><button className="primary-button" type="button"><FileSearch size={15} /> ابحث عن مصدر جديد</button><button className="secondary-button" type="button"><GitBranch size={15} /> اعرض المسار</button><Link to="/questions" className="text-button">كل الأسئلة <ArrowLeft size={14} /></Link></div>
          </section>

          {researchResult ? <ResearchResultPanel result={researchResult} /> : null}
          {relationshipImpactResult ? <GraphRelationshipImpactPanel result={relationshipImpactResult} /> : null}
          {branchComparisonResult ? <GraphBranchComparisonPanel result={branchComparisonResult} /> : null}
          {ancestorFrontierResult ? <GraphAncestorFrontierPanel result={ancestorFrontierResult} /> : null}

          <EvidenceComparison />

          <section className="research-sources-section">
            <SectionHeading eyebrow="المصادر القريبة" title="ما يمكن فتحه والتحقق منه" description="الاقتباس الأصلي يبقى مرجعاً، حتى لو أضاف النظام ملاحظة." action="فتح المكتبة" />
            <div className="source-list">{visibleSources.map((source) => <SourceCard key={source.id} source={source} />)}</div>
            {visibleSources.length === 0 ? <div className="empty-search"><FileSearch size={18} /><span>لم نجد مصدراً بهذه العبارة. جرّب كلمة أقصر أو ابدأ سؤالاً جديداً.</span></div> : null}
          </section>
        </main>

        <aside className="research-side-column">
          <section className="research-side-panel research-filter-panel">
            <div className="side-panel-head"><div className="eyebrow">رشّح</div><Filter size={16} /></div>
            <h3>رشّح المواد</h3>
            <label className="small-search"><Search size={14} /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="ابحث في المصادر" /></label>
            <div className="side-filter-group"><span>نوع المصدر</span><label><input type="checkbox" defaultChecked /> مخطوطات وكتب <b>842</b></label><label><input type="checkbox" defaultChecked /> سجلات وأرشيف <b>211</b></label><label><input type="checkbox" /> أشجار منشورة <b>376</b></label></div>
            <div className="side-filter-group"><span>حالة المراجعة</span><label><input type="checkbox" defaultChecked /> يحتاج مراجعة <b>17</b></label><label><input type="checkbox" /> مفهرس <b>1,102</b></label></div>
          </section>
          <section className="research-side-panel next-step-panel"><div className="side-panel-head"><div className="eyebrow">اقتراح بحثي</div><Sparkles size={16} /></div><h3>ماذا تفحص بعد ذلك؟</h3><p>ابحث عن إشارات إلى الاعتماد بين «مجموع المصادر» و«تاريخ قبائل نجد»، قبل اعتبارهما دليلين مستقلين.</p><div className="next-step-list"><span><CheckCircle2 size={14} /> قارن صياغة الفقرتين</span><span><CheckCircle2 size={14} /> راجع تاريخ التأليف</span><span><CheckCircle2 size={14} /> افحص إحالات المصدر الثاني</span></div><button className="secondary-button" type="button"><Plus size={14} /> أضف إلى السؤال</button></section>
          <section className="research-side-panel activity-side-panel"><div className="side-panel-head"><div className="eyebrow">سجل النشاط</div><span className="live-indicator"><i /> مباشر</span></div><div className="activity-side-list"><div><span className="activity-side-dot source" /><p><strong>سُجلت</strong> عبارة من المصدر ١<small>منذ ١٢ دقيقة</small></p></div><div><span className="activity-side-dot finding" /><p><strong>أضاف النظام</strong> ملاحظة تعارض<small>منذ ساعة</small></p></div><div><span className="activity-side-dot question" /><p><strong>فُتح السؤال</strong> للمراجعة<small>منذ ساعتين</small></p></div></div></section>
        </aside>
      </div>
    </div>
  );
}

function graphStartTypeFor(operation: GraphOperation | "", entityType: "person" | "family" | "branch" | undefined): "person" | "family" | "branch" | "source" {
  if (operation === "source_entities") return "source";
  if (operation === "common_ancestor_path" || operation === "shortest_relationship_path" || operation === "connected_component") return "person";
  return entityType ?? "person";
}

function isUuid(value: string | undefined): boolean {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value));
}


const researchLayerLabels: Record<ResearchCitation["layer"], string> = {
  source_statement: "عبارة المصدر",
  research_claim: "ادعاء بحثي",
  tree_interpretation: "تفسير الشجرة",
  platform_finding: "ملاحظة النظام",
  open_question: "سؤال مفتوح",
};

function researchLayerTone(layer: ResearchCitation["layer"]): EpistemicTone {
  if (layer === "source_statement") return "source";
  if (layer === "research_claim") return "claim";
  if (layer === "tree_interpretation") return "interpretation";
  if (layer === "platform_finding") return "finding";
  return "question";
}

function routeLabel(route: ResearchRoute): string {
  if (route === "ignore") return "تم تجاهل الطلب";
  if (route === "cheap") return "مسار سريع";
  return "مسار عميق";
}

function GraphAncestorFrontierFields({ treeID, treeVersionID, rootNodeID, onTreeID, onTreeVersionID, onRootNodeID }: { treeID: string; treeVersionID: string; rootNodeID: string; onTreeID: (value: string) => void; onTreeVersionID: (value: string) => void; onRootNodeID: (value: string) => void }) {
  return <div className="research-ancestor-frontier-fields"><label>معرف الشجرة<input required value={treeID} onChange={(event) => onTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input required value={treeVersionID} onChange={(event) => onTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label><label>عقدة الجذر<input required value={rootNodeID} onChange={(event) => onRootNodeID(event.target.value)} placeholder="tree_node UUID" /></label></div>;
}

function GraphAncestorFrontierPanel({ result }: { result: GraphAncestorFrontierResult }) {
  const reasonLabels: Record<string, string> = { depth_limit: "حد العمق", node_limit: "حد العقد", edge_limit: "حد العلاقات" };
  const stats: GraphStats = { operation: "ancestor_frontier", pathCount: 1, nodeCount: result.path.nodes.length, edgeCount: result.path.edges.length, evidenceCount: 0, truncated: result.summary.truncated, pathsTruncated: result.summary.truncated, edgesTruncated: result.summary.truncationReasons.includes("edge_limit"), maxDepth: result.limits.maxDepth, algorithmVersion: result.algorithmVersion };
  return <section className="research-result-card" aria-live="polite"><div className="research-result-head"><div><div className="eyebrow">إطار الأسلاف</div><h2>بنية الأسلاف المحدودة</h2></div><StatusBadge tone="interpretation">بنيوي فقط</StatusBadge></div><p className="research-result-answer">{result.path.explanation}</p><div className="research-result-stats"><span><strong>{result.summary.boundedAncestorCount}</strong> سلف ضمن النطاق</span><span><strong>{result.summary.boundedEdgeCount}</strong> علاقة</span><span><strong>{result.summary.maxDepthReached}</strong> عمق</span><span>{result.summary.truncated ? result.summary.truncationReasons.map((reason) => reasonLabels[reason] || reason).join(" · ") : "ضمن الحدود"}</span></div><GraphPathsPanel paths={[result.path]} stats={stats} /></section>;
}

function GraphBranchComparisonPanel({ result }: { result: GraphBranchStructureComparisonResult }) {
  const reasonLabels: Record<string, string> = { depth_limit: "حد العمق", node_limit: "حد العقد", edge_limit: "حد العلاقات" };
  return <section className="research-result-card" aria-live="polite"><div className="research-result-head"><div><div className="eyebrow">مقارنة بنية الفروع</div><h2>فروق الشكل داخل تفسيرين منشورين</h2></div><StatusBadge tone="interpretation">بنيوي فقط</StatusBadge></div><p className="research-result-answer">{result.explanation}</p><div className="research-result-stats"><span><strong>{result.delta.nodeCount >= 0 ? "+" : ""}{result.delta.nodeCount}</strong> فرق عقد</span><span><strong>{result.delta.edgeCount >= 0 ? "+" : ""}{result.delta.edgeCount}</strong> فرق علاقات</span><span><strong>{result.delta.leafCount >= 0 ? "+" : ""}{result.delta.leafCount}</strong> فرق أوراق</span><span>{result.truncated.from || result.truncated.to ? result.truncationReasons.map((reason) => reasonLabels[reason] || reason).join(" · ") : "ضمن الحدود"}</span></div><div className="research-branch-comparison-grid"><GraphBranchComparisonSide title="النسخة الأولى" side={result.from} /><GraphBranchComparisonSide title="النسخة الثانية" side={result.to} /></div></section>;
}

function GraphBranchComparisonSide({ title, side }: { title: string; side: GraphBranchStructureComparisonResult["from"] }) {
  return <article className="research-branch-comparison-side"><h3>{title}</h3><small>النطاق: {side.treeScope.treeId?.slice(0, 8)} · النسخة {side.treeScope.versionNumber}</small><div className="research-branch-comparison-metrics"><span><strong>{side.nodeCount}</strong> عقدة</span><span><strong>{side.edgeCount}</strong> علاقة</span><span><strong>{side.leafCount}</strong> ورقة</span><span><strong>{side.maxDepth}</strong> عمق</span></div><div className="research-branch-comparison-details"><span>حسب العمق: {side.nodesByDepth.map((item) => `${item.depth}:${item.count}`).join(" · ")}</span><span>حسب الأبناء: {side.childCountHistogram.map((item) => `${item.childCount}:${item.nodeCount}`).join(" · ")}</span><span>الحالات: {side.edgeStatusCounts.map((item) => `${item.status}:${item.count}`).join(" · ") || "بدون"}</span>{side.cycleDetected ? <span>تم اكتشاف دورة بنيوية</span> : null}</div>{side.truncated ? <p className="research-graph-warning"><CircleAlert size={13} /> تم قص النطاق عند الحد الآمن.</p> : null}</article>;
}

function GraphBranchComparisonFields({ fromTreeID, fromTreeVersionID, fromRootNodeID, toTreeID, toTreeVersionID, toRootNodeID, onFromTreeID, onFromTreeVersionID, onFromRootNodeID, onToTreeID, onToTreeVersionID, onToRootNodeID }: { fromTreeID: string; fromTreeVersionID: string; fromRootNodeID: string; toTreeID: string; toTreeVersionID: string; toRootNodeID: string; onFromTreeID: (value: string) => void; onFromTreeVersionID: (value: string) => void; onFromRootNodeID: (value: string) => void; onToTreeID: (value: string) => void; onToTreeVersionID: (value: string) => void; onToRootNodeID: (value: string) => void }) {
  return <div className="research-branch-comparison-fields"><strong>الجذر الأول</strong><label>معرف الشجرة<input required value={fromTreeID} onChange={(event) => onFromTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input required value={fromTreeVersionID} onChange={(event) => onFromTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label><label>عقدة الجذر<input required value={fromRootNodeID} onChange={(event) => onFromRootNodeID(event.target.value)} placeholder="tree_node UUID" /></label><strong>الجذر الثاني</strong><label>معرف الشجرة<input required value={toTreeID} onChange={(event) => onToTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input required value={toTreeVersionID} onChange={(event) => onToTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label><label>عقدة الجذر<input required value={toRootNodeID} onChange={(event) => onToRootNodeID(event.target.value)} placeholder="tree_node UUID" /></label></div>;
}

function GraphRelationshipImpactPanel({ result }: { result: GraphRelationshipImpactResult }) {
  const path: GraphPath = {
    id: result.pathId,
    operation: "relationship_impact",
    nodes: result.nodes,
    edges: result.edges,
    evidenceRefs: [],
    status: result.status,
    explanation: result.explanation,
    depth: result.depth,
    truncated: result.truncated,
    evidenceBacked: false,
    structuralOnly: result.structuralOnly,
    treeScope: result.treeScope,
    algorithmVersion: result.algorithmVersion,
  };
  const stats: GraphStats = {
    operation: "relationship_impact",
    pathCount: 1,
    nodeCount: result.nodes.length,
    edgeCount: result.edges.length,
    evidenceCount: 0,
    truncated: result.truncated,
    pathsTruncated: result.truncated,
    edgesTruncated: result.truncationReasons.includes("edge_limit"),
    maxDepth: result.limits.maxDepth,
    algorithmVersion: result.algorithmVersion,
  };
  const reasonLabels: Record<string, string> = { depth_limit: "حد العمق", node_limit: "حد العقد", edge_limit: "حد العلاقات" };
  return (
    <section className="research-result-card" aria-live="polite">
      <div className="research-result-head">
        <div><div className="eyebrow">أثر العلاقة downstream</div><h2>النطاق البنيوي المتأثر</h2></div>
        <StatusBadge tone="interpretation">بنيوي فقط</StatusBadge>
      </div>
      <p className="research-result-answer">{result.explanation}</p>
      <div className="research-result-stats"><span><strong>{result.boundedDownstreamNodeCount}</strong> فرد ضمن النطاق</span><span><strong>{result.edges.length}</strong> علاقة</span><span>عمق {result.limits.maxDepth}</span>{result.truncationReasons.length ? <span>{result.truncationReasons.map((reason) => reasonLabels[reason] || reason).join(" · ")}</span> : null}</div>
      <GraphPathsPanel paths={[path]} stats={stats} />
    </section>
  );
}

export function ResearchResultPanel({ result }: { result: ResearchQueryResult }) {
  const groups: Array<{ citations: ResearchCitation[]; label: string }> = [
    { citations: result.layers.sourceStatements, label: "عبارات المصدر" },
    { citations: result.layers.researchClaims, label: "ادعاءات البحث" },
    { citations: result.layers.treeInterpretations, label: "تفسيرات الشجرة" },
    { citations: result.layers.platformFindings, label: "ملاحظات النظام" },
    { citations: result.layers.openQuestions, label: "أسئلة مفتوحة" },
  ].filter((group) => group.citations.length > 0);
  const routing = result.routing;
  const synthesisLabel = routing
    ? routing.synthesisAttempted
      ? "تم تشغيل التلخيص"
      : "بلا تلخيص عميق"
    : result.modelVersion
      ? "تلخيص سابق"
      : "بلا تلخيص عميق";

  return (
    <section className="research-result-card" aria-live="polite">
      <div className="research-result-head">
        <div>
          <div className="eyebrow">نتيجة البحث المربوطة</div>
          <h2>{result.query}</h2>
        </div>
        <StatusBadge tone={result.insufficientEvidence ? "disputed" : "source"}>{result.insufficientEvidence ? "أدلة غير كافية" : "مرتبطة بمصادر"}</StatusBadge>
      </div>
      <p className="research-result-answer">{result.answer}</p>
      <div className="research-result-stats"><span><strong>{result.citations.length}</strong> مادة</span><span><strong>{result.retrieval.fusedCandidates}</strong> مرشح</span><span><strong>{result.conflicts.length}</strong> تعارض</span>{result.graphStats?.operation ? <span><strong>{result.graphStats.pathCount}</strong> مسار</span> : null}<span>{routing ? routeLabel(routing.route) : "مسار غير محدد"}</span><span>{synthesisLabel}</span></div>
      <GraphPathsPanel paths={result.graphPaths ?? []} stats={result.graphStats} />
      {result.conflicts.length > 0 ? <div className="research-conflict-list">{result.conflicts.map((conflict, index) => <div key={`${conflict.leftId}-${conflict.rightId}-${index}`}><CircleAlert size={15} /><span><strong>{conflict.status}</strong> {conflict.explanation}</span></div>)}</div> : null}
      <div className="research-evidence-groups">{groups.map((group) => <div className="research-evidence-group" key={group.label}><div className="research-evidence-group-title"><span>{group.label}</span><small>{group.citations.length}</small></div>{group.citations.map((citation) => <article className="research-evidence-item" key={`${citation.layer}-${citation.id}`}><div className="research-evidence-item-head"><StatusBadge tone={researchLayerTone(citation.layer)} compact>{researchLayerLabels[citation.layer]}</StatusBadge>{citation.rank > 0 ? <span>#{citation.rank}</span> : null}</div><strong>{citation.title || "بدون عنوان"}</strong><p>{citation.excerpt || "لا يوجد مقتطف متاح."}</p><small>{citation.locatorAr || citation.reviewStatus || citation.status || "بدون موقع"}</small></article>)}</div>)}</div>
    </section>
  );
}

export function GraphPathsPanel({ paths, stats }: { paths: GraphPath[]; stats?: GraphStats }) {
  if (!stats?.operation && paths.length === 0) return null;
  return (
    <section className="research-graph-panel" aria-label="مسارات العلاقات">
      <div className="research-graph-panel-head">
        <div><div className="eyebrow">مسار العلاقات</div><h3>بنية العلاقات القابلة للتتبع</h3></div>
        <span>{stats?.pathCount ?? paths.length} مسار · عمق {stats?.maxDepth ?? 0}{stats?.operation === "shortest_relationship_path" ? " · أقرب مسار" : ""}</span>
      </div>
      {paths.length ? paths.map((path) => (
        <article className="research-graph-path" key={path.id}>
          <div className="research-graph-path-head">
            <div><strong>{graphOperationLabel(path.operation)}</strong><small>{path.explanation}</small>{path.treeScope.treeId ? <small>النطاق: {path.treeScope.treeId.slice(0, 8)}{path.treeScope.versionNumber ? ` · النسخة ${path.treeScope.versionNumber}` : ""}</small> : null}</div>
            <StatusBadge tone={graphPathTone(path)}>{graphStatusLabel(path.status)}</StatusBadge>
          </div>
          <div className="research-graph-nodes" aria-label="العقد في المسار">
            {path.nodes.map((node, index) => <span key={`${path.id}-${node.id}-${index}`}><b>{node.label || node.id.slice(0, 8)}</b><small>{node.type}</small></span>)}
          </div>
          {path.edges.length ? <div className="research-graph-edges" aria-label="العلاقات في المسار">
            {path.edges.map((edge) => {
              const from = graphNodeLabel(path, edge.fromNodeId);
              const to = graphNodeLabel(path, edge.toNodeId);
              return <div key={`${path.id}-${edge.id}-${edge.position}`}><Link2 size={12} /><span><strong>{from}</strong> <small>— {graphPredicateLabel(edge.predicate || edge.type)} →</small> <strong>{to}</strong></span><small>{edge.pathFromNodeId && edge.pathFromNodeId !== edge.fromNodeId ? "اتجاه المسار معكوس · " : ""}{edge.status || "بدون حالة"}{edge.sourceId ? ` · ${edge.sourceId.slice(0, 8)}` : ""}</small></div>;
            })}
          </div> : null}
          {path.evidenceRefs.length ? <div className="research-graph-evidence">
            {path.evidenceRefs.map((evidence) => <details key={`${path.id}-${evidence.id}-${evidence.relation ?? ""}`}><summary><FileSearch size={12} /><span>{evidence.title || graphEvidenceTypeLabel(evidence.type, evidence.layer)}</span><small>{evidence.relation || evidence.reviewStatus || "مرجع"}</small></summary><p>{evidence.excerpt || "لا يوجد مقتطف متاح."}</p><small>{evidence.locatorAr || "بدون موقع"}{evidence.claimId ? ` · ادعاء ${evidence.claimId.slice(0, 8)}` : ""}</small></details>)}
          </div> : null}
          {path.truncated ? <p className="research-graph-warning"><CircleAlert size={13} /> تم قص المسار عند الحد الآمن؛ المتابعة تحتاج فحصاً إضافياً.</p> : null}
          {path.structuralOnly ? <p className="research-graph-warning"><CircleAlert size={13} /> هذا مسار بنيوي ولا يحتوي على دليل مصدرّي ظاهر.</p> : null}
        </article>
      )) : <div className="research-graph-empty"><CircleAlert size={16} /><span>لم يُعثر على مسار ضمن النطاق المحدد.</span></div>}
    </section>
  );
}

function graphNodeLabel(path: GraphPath, nodeId: string): string {
  return path.nodes.find((node) => node.id === nodeId)?.label || nodeId.slice(0, 8);
}

function graphOperationLabel(operation: GraphOperation): string {
  const labels: Record<GraphOperation, string> = { common_ancestor_path: "سلف مشترك", shortest_relationship_path: "أقصر مسار بين شخصين", connected_component: "مكوّن متصل", relationship_impact: "أثر العلاقة على الأبناء", branch_structure_comparison: "مقارنة بنية الفروع", ancestor_frontier: "إطار الأسلاف", source_dependency_neighborhood: "حيّز اعتماد المصادر", evidence_connection: "رابط أدلة", branch_claims: "ادعاءات حول كيان", source_entities: "كيانات مصدر", geographic_path: "مسار جغرافي" };
  return labels[operation];
}

function graphStatusLabel(status: string): string {
  if (status === "structural") return "بنيوي";
  if (status === "complete") return "مكتمل";
  if (status === "contested") return "متنازع عليه";
  if (status === "partial") return "جزئي";
  if (status === "evidence_backed") return "مرتبط بأدلة";
  if (status === "truncated") return "مقصور";
  if (status === "not_found") return "غير موجود";
  return status;
}

function graphPredicateLabel(value: string): string {
  if (value === "parent_of") return "أب/أم";
  if (value === "spouse_of") return "زوج/زوجة";
  if (value === "sibling_of") return "شقيق/شقيقة";
  if (value === "tree_relationship") return "علاقة شجرة";
  return value;
}

function graphEvidenceTypeLabel(type: string, layer?: string): string {
  if (layer === "tree_interpretation" || type === "tree_relationship") return "مرجع تفسير الشجرة";
  if (type === "source_statement") return "عبارة مصدر";
  if (type === "source_passage") return "مقطع مصدر";
  if (type === "migration_event") return "حدث انتقال";
  return type;
}

function graphPathTone(path: GraphPath): EpistemicTone {
  if (path.status === "contested") return "disputed";
  if (path.status === "partial" || path.structuralOnly) return "interpretation";
  return "source";
}
