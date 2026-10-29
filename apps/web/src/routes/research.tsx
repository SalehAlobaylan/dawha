import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowLeft, CircleAlert, FileSearch, GitBranch, Link2, Search } from "lucide-react";
import { useState } from "react";
import { queryGraphAncestorFrontier, queryGraphBranchStructureComparison, queryGraphRelationshipImpact, queryResearch } from "../lib/api";
import type { EpistemicTone, GraphAncestorFrontierResult, GraphBranchStructureComparisonResult, GraphOperation, GraphPath, GraphRelationshipImpactResult, GraphStats, ResearchCitation, ResearchQueryResult, ResearchRoute } from "../types";
import { StatusBadge } from "../components/StatusBadge";

export function EvidenceResearchPanel() {
  const contextSearch = useSearch({ from: "/research" });
  const navigate = useNavigate();
  const [researchQuestion, setResearchQuestion] = useState(contextSearch.q ?? "");
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
  const runResearch = async () => {
    const question = researchQuestion.trim();
    const relationshipImpactMode = graphOperation === "relationship_impact";
    const branchComparisonMode = graphOperation === "branch_structure_comparison";
    const ancestorFrontierMode = graphOperation === "ancestor_frontier";
    if ((!question && !relationshipImpactMode && !branchComparisonMode && !ancestorFrontierMode) || researchLoading) return;
    void navigate({ to: "/research", search: { ...contextSearch, view: "evidence", q: question || undefined }, replace: true, resetScroll: false });
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

  return <section className="research-evidence-workspace">
    {contextSearch.entityId ? <div className="research-context-chip"><GitBranch size={14} /><span>السياق المحدد محفوظ مع هذا البحث.</span><Link to="/research" search={{ view: "evidence" }}>مسح السياق</Link></div> : null}
    <section className="research-focus-card">
      <div className="research-focus-head"><div><div className="eyebrow">بحث في الأدلة</div><h2>ما الذي تقوله المصادر؟</h2></div><StatusBadge tone="question">الإجابة تتبع الدليل</StatusBadge></div>
            <form id="evidence-query-form" className="research-query-form" onSubmit={(event) => { event.preventDefault(); void runResearch(); }}>
              <label htmlFor="research-question">{graphOperation === "relationship_impact" ? "حلّل أثر علاقة في تفسير منشور" : graphOperation === "branch_structure_comparison" ? "قارن شكل الفروع في نسختين" : graphOperation === "ancestor_frontier" ? "استكشف إطار الأسلاف" : "اسأل عن أدلة مصدرة"}</label>
              <div className="research-query-input-row">
                <input id="research-question" value={researchQuestion} onChange={(event) => setResearchQuestion(event.target.value)} placeholder={graphOperation === "relationship_impact" || graphOperation === "branch_structure_comparison" || graphOperation === "ancestor_frontier" ? "اختياري: أضف سياقاً للسجل" : "مثال: من كان والد عبدالله في هذه الروايات؟"} />
                <button className="primary-button" type="submit" disabled={researchLoading || (!researchQuestion.trim() && graphOperation !== "relationship_impact" && graphOperation !== "branch_structure_comparison" && graphOperation !== "ancestor_frontier")}>{researchLoading ? "جارٍ البحث…" : graphOperation === "ancestor_frontier" ? "استكشف الإطار" : graphOperation === "branch_structure_comparison" ? "قارن البنية" : graphOperation === "relationship_impact" ? "حلّل الأثر" : "ابحث في الأدلة"}<Search size={15} /></button>
              </div>
            </form>
            <details className="research-advanced-analysis"><summary><GitBranch size={15} /> تحليل العلاقات المتقدم</summary>
            <div className="research-graph-mode">
              <div className="research-graph-mode-copy"><GitBranch size={15} /><div><strong>مسار العلاقات</strong><small>اختر التحليل عند الحاجة، وحدد السجلات والنسخة التي تريد فحصها.</small></div></div>
              <div className="research-graph-fields">
                <label>نوع المسار<select form="evidence-query-form" value={graphOperation} onChange={(event) => setGraphOperation(event.target.value as GraphOperation | "")}><option value="">بدون مسار رسومي</option><option value="common_ancestor_path">سلف مشترك</option><option value="shortest_relationship_path">أقصر مسار بين شخصين</option><option value="connected_component">مكوّن متصل داخل تفسير منشور</option><option value="relationship_impact">أثر علاقة على الأبناء</option><option value="branch_structure_comparison">مقارنة بنية فرعين</option><option value="ancestor_frontier">إطار الأسلاف</option><option value="evidence_connection">رابط أدلة بين كيانين</option><option value="branch_claims">ادعاءات حول فرع أو كيان</option><option value="source_entities">كيانات مرتبطة بمصدر</option><option value="geographic_path">مسار جغرافي</option></select></label>
                {graphOperation && graphOperation !== "branch_structure_comparison" && graphOperation !== "ancestor_frontier" ? <><label>{graphOperation === "relationship_impact" ? "معرف العلاقة" : graphOperation === "connected_component" ? "الشخص الجذر" : "نقطة البداية"}<input form="evidence-query-form" required value={graphOperation === "relationship_impact" ? graphRelationshipID : graphStartIDValue} onChange={(event) => graphOperation === "relationship_impact" ? setGraphRelationshipID(event.target.value) : setGraphStartID(event.target.value)} placeholder="معرف UUID" /></label>{graphOperation !== "branch_claims" && graphOperation !== "source_entities" && graphOperation !== "connected_component" && graphOperation !== "relationship_impact" ? <label>{graphOperation === "geographic_path" ? "المكان المرجعي" : graphOperation === "shortest_relationship_path" ? "الشخص الآخر" : "نقطة النهاية"}<input required={graphEndRequired} value={graphEndID} onChange={(event) => setGraphEndID(event.target.value)} placeholder="معرف UUID" /></label> : null}{graphOperation !== "source_entities" ? <label>أقصى عمق<select form="evidence-query-form" value={graphMaxDepth} onChange={(event) => setGraphMaxDepth(Number(event.target.value))}><option value={1}>1</option><option value={2}>2</option><option value={3}>3</option></select></label> : null}{graphTreeScoped ? <><label>معرف الشجرة<input form="evidence-query-form" required value={graphTreeID} onChange={(event) => setGraphTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input form="evidence-query-form" required value={graphTreeVersionID} onChange={(event) => setGraphTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label></> : null}</> : null}
                {graphOperation === "branch_structure_comparison" ? <GraphBranchComparisonFields fromTreeID={fromTreeID} fromTreeVersionID={fromTreeVersionID} fromRootNodeID={fromRootNodeID} toTreeID={toTreeID} toTreeVersionID={toTreeVersionID} toRootNodeID={toRootNodeID} onFromTreeID={setFromTreeID} onFromTreeVersionID={setFromTreeVersionID} onFromRootNodeID={setFromRootNodeID} onToTreeID={setToTreeID} onToTreeVersionID={setToTreeVersionID} onToRootNodeID={setToRootNodeID} /> : null}
                {graphOperation === "ancestor_frontier" ? <GraphAncestorFrontierFields treeID={graphTreeID} treeVersionID={graphTreeVersionID} rootNodeID={graphRootNodeID} onTreeID={setGraphTreeID} onTreeVersionID={setGraphTreeVersionID} onRootNodeID={setGraphRootNodeID} /> : null}
              </div>
            </div>
            {researchError ? <p className="research-query-error">{researchError}</p> : null}
            </details>
      {researchError ? <p className="research-query-error" role="alert">{researchError}</p> : null}
      {!researchResult && !relationshipImpactResult && !branchComparisonResult && !ancestorFrontierResult ? <p className="research-focus-lead">اكتب سؤالاً محدداً. ستظهر النتيجة مع مصادرها وما لم تستطع الأدلة حسمه.</p> : null}
    </section>
    {researchResult ? <ResearchResultPanel result={researchResult} /> : null}
    {relationshipImpactResult ? <GraphRelationshipImpactPanel result={relationshipImpactResult} /> : null}
    {branchComparisonResult ? <GraphBranchComparisonPanel result={branchComparisonResult} /> : null}
    {ancestorFrontierResult ? <GraphAncestorFrontierPanel result={ancestorFrontierResult} /> : null}
    <div className="research-evidence-footer"><Link className="inline-link" to="/research" search={{ create: true, q: researchQuestion || undefined }}>احفظ سؤالاً لمتابعة التحقيق <ArrowLeft size={14} /></Link><Link className="inline-link" to="/search" search={{ q: researchQuestion || undefined }}>البحث في السجلات والمصادر <ArrowLeft size={14} /></Link></div>
  </section>;
}

function graphStartTypeFor(operation: GraphOperation | "", entityType: "person" | "family" | "branch" | undefined): "person" | "family" | "branch" | "source" {
  if (operation === "source_entities") return "source";
  if (operation === "common_ancestor_path" || operation === "shortest_relationship_path" || operation === "connected_component") return "person";
  return entityType ?? "person";
}

function isUuid(value: string | undefined): boolean {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value));
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
  return <div className="research-ancestor-frontier-fields"><label>معرف الشجرة<input form="evidence-query-form" required value={treeID} onChange={(event) => onTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input form="evidence-query-form" required value={treeVersionID} onChange={(event) => onTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label><label>عقدة الجذر<input form="evidence-query-form" required value={rootNodeID} onChange={(event) => onRootNodeID(event.target.value)} placeholder="tree_node UUID" /></label></div>;
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
  return <div className="research-branch-comparison-fields"><strong>الجذر الأول</strong><label>معرف الشجرة<input form="evidence-query-form" required value={fromTreeID} onChange={(event) => onFromTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input form="evidence-query-form" required value={fromTreeVersionID} onChange={(event) => onFromTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label><label>عقدة الجذر<input form="evidence-query-form" required value={fromRootNodeID} onChange={(event) => onFromRootNodeID(event.target.value)} placeholder="tree_node UUID" /></label><strong>الجذر الثاني</strong><label>معرف الشجرة<input form="evidence-query-form" required value={toTreeID} onChange={(event) => onToTreeID(event.target.value)} placeholder="tree UUID" /></label><label>نسخة الشجرة<input form="evidence-query-form" required value={toTreeVersionID} onChange={(event) => onToTreeVersionID(event.target.value)} placeholder="tree_version UUID" /></label><label>عقدة الجذر<input form="evidence-query-form" required value={toRootNodeID} onChange={(event) => onToRootNodeID(event.target.value)} placeholder="tree_node UUID" /></label></div>;
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
      <div className="research-evidence-groups">{groups.map((group) => <div className="research-evidence-group" key={group.label}><div className="research-evidence-group-title"><span>{group.label}</span><small>{group.citations.length}</small></div>{group.citations.map((citation) => <article className="research-evidence-item" key={`${citation.layer}-${citation.id}`}><div className="research-evidence-item-head"><StatusBadge tone={researchLayerTone(citation.layer)} compact>{researchLayerLabels[citation.layer]}</StatusBadge>{citation.rank > 0 ? <span>#{citation.rank}</span> : null}</div><strong>{citation.title || "بدون عنوان"}</strong><p>{citation.excerpt || "لا يوجد مقتطف متاح."}</p><small>{citation.locatorAr || citation.reviewStatus || citation.status || "بدون موقع"}</small><CitationDestination citation={citation} /></article>)}</div>)}</div>
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
            {path.evidenceRefs.map((evidence) => <details key={`${path.id}-${evidence.id}-${evidence.relation ?? ""}`}><summary><FileSearch size={12} /><span>{evidence.title || graphEvidenceTypeLabel(evidence.type, evidence.layer)}</span><small>{evidence.relation || evidence.reviewStatus || "مرجع"}</small></summary><p>{evidence.excerpt || "لا يوجد مقتطف متاح."}</p>{evidence.sourceId ? <Link className="inline-link" to="/sources" search={{ sourceId: evidence.sourceId }}>فتح المصدر</Link> : null}<small>{evidence.locatorAr || "بدون موقع"}{evidence.claimId ? ` · ادعاء ${evidence.claimId.slice(0, 8)}` : ""}</small></details>)}
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
  const labels: Record<GraphOperation, string> = { common_ancestor_path: "سلف مشترك", shortest_relationship_path: "أقصر مسار بين شخصين", connected_component: "مكوّن متصل", relationship_impact: "أثر العلاقة على الأبناء", branch_structure_comparison: "مقارنة بنية الفروع", ancestor_frontier: "إطار الأسلاف", source_dependency_neighborhood: "حيّز اعتماد المصادر", source_dependency_communities: "مجتمعات اعتماد المصادر", evidence_connection: "رابط أدلة", branch_claims: "ادعاءات حول كيان", source_entities: "كيانات مصدر", geographic_path: "مسار جغرافي" };
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


function CitationDestination({ citation }: { citation: ResearchCitation }) {
  if (citation.sourceId) return <Link className="inline-link" to="/sources" search={{ sourceId: citation.sourceId }}>فتح المصدر <ArrowLeft size={13} /></Link>;
  if (citation.questionId || citation.layer === "open_question") return <Link className="inline-link" to="/research/$questionId" params={{ questionId: citation.questionId || citation.id }}>فتح السؤال <ArrowLeft size={13} /></Link>;
  if (citation.claimId || citation.layer === "research_claim") return <Link className="inline-link" to="/search" search={{ q: citation.title, kind: "claims", claimId: citation.claimId || citation.id }}>فتح الادعاء <ArrowLeft size={13} /></Link>;
  return null;
}
