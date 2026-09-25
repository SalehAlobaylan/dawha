import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleHelp, FileSearch, MapPinned, Route, Search, ShieldQuestion, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError, fetchGeospatialFindings, fetchLatestGeospatialIntelligenceRun, reviewGeospatialFinding, startGeospatialIntelligence } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { GeospatialFinding, GeospatialIntelligenceRun, ResearchWorkspaceSnapshot, WorkspaceTreeContext } from "../types";

interface GeospatialIntelligencePanelProps {
  snapshot: ResearchWorkspaceSnapshot;
  canRun: boolean;
  canReview: boolean;
}

type EntityMode = "person" | "source";

export function GeospatialIntelligencePanel({ snapshot, canRun, canReview }: GeospatialIntelligencePanelProps) {
  const [mode, setMode] = useState<EntityMode>(snapshot.context.entityType === "person" ? "person" : "source");
  const [sourceID, setSourceID] = useState(snapshot.sources[0]?.id ?? "");
  const [radius, setRadius] = useState("250");
  const [run, setRun] = useState<GeospatialIntelligenceRun | null>(null);
  const [findings, setFindings] = useState<GeospatialFinding[]>([]);
  const [message, setMessage] = useState("");
  const queryClient = useQueryClient();
  const treeContext = useMemo(() => selectTreeContext(snapshot), [snapshot]);
  const personID = snapshot.context.entityType === "person" ? snapshot.context.entityId ?? "" : "";
  const entityID = mode === "person" ? personID : sourceID;
  const source = snapshot.sources.find((item) => item.id === sourceID);
  const questionID = snapshot.context.questionId;
  const targetInTree = Boolean(treeContext?.targetPresent || treeContext?.nodes.some((node) => node.personId === personID));
  const treeEligible = mode === "source" || !treeContext || (treeContext.visibility === "public" && treeContext.state === "published" && targetInTree);
  const canAnalyze = Boolean(canRun && entityID && treeEligible && (mode === "source" || targetInTree) && isUuid(entityID) && (mode !== "person" || (isUuid(questionID) && Boolean(treeContext))));
  const scopedTreeVersion = mode === "person" ? treeContext?.treeVersionId ?? "" : "";
  const scopeKey = [mode, entityID, questionID, scopedTreeVersion, radius].join(":");
  const scopeKeyRef = useRef(scopeKey);
  scopeKeyRef.current = scopeKey;
  const latestQuery = useQuery({
    queryKey: ["geospatial-intelligence-latest", mode, entityID, questionID, scopedTreeVersion],
    queryFn: () => fetchLatestGeospatialIntelligenceRun({ question_id: questionID, entity_type: mode, entity_id: entityID, tree_version_id: mode === "person" ? treeContext?.treeVersionId : undefined }),
    enabled: canAnalyze && isUuid(questionID) && isUuid(entityID),
    staleTime: 60000,
    refetchOnWindowFocus: false,
  });
  const findingsQuery = useQuery({
    queryKey: ["geospatial-intelligence-findings", run?.id],
    queryFn: () => fetchGeospatialFindings(run?.id),
    enabled: Boolean(run && run.findingCount > 0),
  });
  const startMutation = useMutation({
    mutationFn: async ({ requestScopeKey }: { requestScopeKey: string }) => {
      if (requestScopeKey !== scopeKey || !entityID) throw new Error("تغير سياق التحليل أثناء الطلب.");
      return startGeospatialIntelligence({ entity_type: mode, entity_id: entityID, question_id: questionID, tree_id: mode === "person" ? treeContext?.treeId : undefined, tree_version_id: mode === "person" ? treeContext?.treeVersionId : undefined, radius_km: Number(radius) || 250, maximum_records: 500 });
    },
    onMutate: ({ requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setRun(null);
      setFindings([]);
      setMessage("جارٍ فحص الأسماء والمواضع المؤهلة…");
    },
    onSuccess: (value, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setRun(value);
      setMessage(runMessage(value));
      void queryClient.invalidateQueries({ queryKey: ["research-workspace", questionID] });
      void queryClient.invalidateQueries({ queryKey: ["geospatial-intelligence-latest", mode, entityID, questionID, scopedTreeVersion] });
    },
    onError: (error, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setMessage(errorMessage(error));
    },
  });
  const reviewMutation = useMutation({
    mutationFn: async ({ finding, decision, scopeKey: requestScopeKey }: { finding: GeospatialFinding; decision: "dismiss" | "confirm" | "investigate"; scopeKey: string }) => ({ updated: await reviewGeospatialFinding(finding.id, decision === "investigate" ? { decision, create_question: true, question_title_ar: `مراجعة ملاحظة جغرافية: ${finding.titleAr}` } : { decision }), requestScopeKey }),
    onSuccess: ({ updated, requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setFindings((current) => current.map((finding) => finding.id === updated.id ? updated : finding));
      setMessage(updated.status === "investigating" ? "فُتح سؤال متابعة الملاحظة." : "حُفظ قرار المراجعة.");
      void queryClient.invalidateQueries({ queryKey: ["research-workspace", questionID] });
    },
    onError: (error, { scopeKey: requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setMessage(errorMessage(error));
    },
  });
  useEffect(() => {
    setRun(null);
    setFindings([]);
    setMessage("");
  }, [scopeKey]);
  useEffect(() => {
    if (latestQuery.data) {
      setRun(latestQuery.data);
      setMessage(runMessage(latestQuery.data));
    }
  }, [latestQuery.data]);
  const activeRunID = run?.id;
  const activeFindingCount = run?.findingCount;
  useEffect(() => {
    if (!activeRunID || activeFindingCount === 0) {
      setFindings([]);
      return;
    }
    if (findingsQuery.data) setFindings(findingsQuery.data);
  }, [activeRunID, activeFindingCount, findingsQuery.data]);

  const hint = eligibilityMessage(canRun, mode, snapshot, treeContext, targetInTree, source);
  const isLoading = startMutation.isPending || latestQuery.isFetching;
  return (
    <section className="geospatial-intelligence-panel" aria-label="التحليل الجغرافي" aria-busy={isLoading}>
      <div className="geospatial-intelligence-head"><div><div className="eyebrow">الجغرافيا البحثية</div><h3>من الإشارة إلى التسلسل المحتمل</h3><p>تظهر النتائج الموثقة منفصلة عن الاستنتاجات؛ كل فرضية تحتاج إلى مراجعة.</p></div><div className="geospatial-intelligence-actions"><label><MapPinned size={13} /><select value={mode} onChange={(event) => setMode(event.target.value as EntityMode)} aria-label="نطاق التحليل الجغرافي"><option value="person">شخص</option><option value="source">مصدر</option></select></label>{mode === "source" ? <label><FileSearch size={13} /><select value={sourceID} onChange={(event) => setSourceID(event.target.value)} aria-label="المصدر"><option value="">اختر مصدراً</option>{snapshot.sources.map((item) => <option value={item.id} key={item.id}>{item.titleAr}</option>)}</select></label> : null}<label className="geospatial-radius"><span>نطاق (كم)</span><input type="number" min="10" max="2000" value={radius} onChange={(event) => setRadius(event.target.value)} /></label><button className="secondary-button" type="button" onClick={() => startMutation.mutate({ requestScopeKey: scopeKey })} disabled={!canAnalyze || startMutation.isPending}><Route size={14} /> {startMutation.isPending ? "جارٍ الفحص…" : "حلّل الجغرافيا"}</button></div></div>
      {!canAnalyze ? <p className="geospatial-intelligence-hint"><CircleHelp size={13} /> {hint}</p> : null}
      {latestQuery.isError ? <div className="geospatial-intelligence-error" role="alert">{errorMessage(latestQuery.error)}</div> : null}
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {run ? <GeospatialReport run={run} findings={findings} findingsLoading={Boolean(run.findingCount > 0 && findingsQuery.isPending)} findingsError={findingsQuery.isError ? errorMessage(findingsQuery.error) : ""} canReview={canReview} onReview={(finding, decision) => reviewMutation.mutate({ finding, decision, scopeKey })} reviewPending={reviewMutation.isPending} /> : latestQuery.isFetching ? <p className="evidence-empty">جارٍ استعادة آخر تحليل جغرافي محفوظ…</p> : <p className="evidence-empty">لم يبدأ تحليل جغرافي بعد.</p>}
    </section>
  );
}

function GeospatialReport({ run, findings, findingsLoading, findingsError, canReview, onReview, reviewPending }: { run: GeospatialIntelligenceRun; findings: GeospatialFinding[]; findingsLoading: boolean; findingsError: string; canReview: boolean; onReview: (finding: GeospatialFinding, decision: "dismiss" | "confirm" | "investigate") => void; reviewPending: boolean }) {
  const report = run.report;
  return <div className="geospatial-report"><div className="geospatial-report-summary"><span><strong>{report.placeResolution.resolvedCount}</strong> موضعاً محلولاً</span><span><strong>{report.placeResolution.unresolvedCount}</strong> إشارة غير محسومة</span><span><strong>{report.migrationHypotheses.length}</strong> تسلسلاً محتملاً</span><StatusBadge tone={run.reportStatus === "insufficient_evidence" ? "question" : "interpretation"}>{run.reportStatus === "insufficient_evidence" ? "أدلة غير كافية" : "اكتمل الفحص"}</StatusBadge></div><small className="geospatial-report-scope">النطاق: {run.entityName} · نصف قطر {formatNumber(run.scope.radiusKm)} كم · سياسة {run.qualificationPolicyVersion}</small>{report.limitations.length ? <small className="geospatial-report-limitations">{report.limitations.map((item) => <span key={item}>{item}</span>)}</small> : null}<div className="geospatial-report-grid"><section><h4>تحويل الأسماء والمواضع</h4>{report.placeResolution.mentions.length ? <div className="geospatial-mention-list">{report.placeResolution.mentions.slice(0, 8).map((mention) => <div key={mention.id}><strong>{mention.placeName ?? mention.mention}</strong><small>{mention.resolution === "resolved" ? "مصدر موثق" : "غير محسوم"} · {mention.sourceTitle ?? mention.sourceId.slice(0, 8)}</small></div>)}</div> : <p className="evidence-empty">لا توجد إشارات مكانية متاحة.</p>}{report.disambiguation.length ? <div className="geospatial-disambiguation-list"><small className="geospatial-unresolved-note">{report.disambiguation.length} اسم تاريخي يحتاج توضيحاً.</small>{report.disambiguation.slice(0, 4).map((item) => <div key={item.id}><strong>{item.mention}</strong><small>{item.candidates.map((candidate) => candidate.placeName).join(" · ")}</small></div>)}</div> : null}</section><section><h4>التسلسلات المكانية</h4>{report.migrationHypotheses.length ? report.migrationHypotheses.map((item) => <article className="geospatial-sequence" key={item.id}><div><strong>{item.subjectName}</strong><StatusBadge tone={item.layer === "source_backed" ? "source" : "interpretation"}>{item.layer === "source_backed" ? "مدعوم" : "فرضية"}</StatusBadge></div><p>{item.sequence.map((place) => place.placeName || "موضع غير مسمى").join(" ← ")}</p><small>{item.status} · {item.sourceIds.length} مصدر · Tree {item.treeVersionId?.slice(0, 8) ?? "—"}</small></article>) : <p className="evidence-empty">لم emerges تسلسل مكاني.</p>}</section><section><h4>تحليل جغرافيا المصادر</h4>{report.sourceGeography.length ? report.sourceGeography.map((item) => <div className="geospatial-source-row" key={item.sourceId}><FileSearch size={13} /><span><strong>{item.sourceTitle}</strong><small>{item.statementCount} عبارة · {item.placeNames.length ? item.placeNames.join("، ") : "بلا مواضع محسومة"}</small></span><bdi>{item.unresolvedCount}</bdi></div>) : <p className="evidence-empty">لا توجد مصادر جغرافية مؤهلة.</p>}</section><section><h4>التعارضات والملاحظات</h4>{findingsError ? <small className="geospatial-intelligence-error" role="alert">{findingsError}</small> : findingsLoading ? <p className="evidence-empty">جارٍ تحميل الملاحظات…</p> : findings.length ? findings.map((finding) => <GeospatialFindingCard key={finding.id} finding={finding} canReview={canReview} onReview={onReview} pending={reviewPending} />) : <p className="evidence-empty">لا توجد تعارضات جغرافية مكتشفة.</p>}</section></div>{report.clusters.length ? <section className="geospatial-cluster-section"><h4>تجمعات مكانية استنتاجية</h4>{report.clusters.map((cluster) => <div className="geospatial-cluster-row" key={cluster.id}><MapPinned size={13} /><span><strong>{cluster.placeNames.join(" · ")}</strong><small>{cluster.sourceBackedCount} إشارة مدعومة · {cluster.inferredCount} غير محسومة · نصف قطر {formatNumber(cluster.radiusKm)} km</small></span><StatusBadge tone="interpretation">فرضية</StatusBadge></div>)}</section> : null}</div>;
}

function GeospatialFindingCard({ finding, canReview, onReview, pending }: { finding: GeospatialFinding; canReview: boolean; onReview: (finding: GeospatialFinding, decision: "dismiss" | "confirm" | "investigate") => void; pending: boolean }) {
  const investigating = finding.status === "investigating";
  return <article className="geospatial-finding"><div><ShieldQuestion size={14} /><strong>{finding.titleAr}</strong><StatusBadge tone="finding">{finding.status === "needs_review" ? "تحتاج مراجعة" : finding.status === "investigating" ? "قيد التحقيق" : finding.status === "confirmed" ? "مؤكدة" : "مرفوضة"}</StatusBadge></div><p>{finding.explanationAr}</p><small>{finding.layer === "source_backed" ? "طبقة مدعومة" : "طبقة استنتاجية"} · {finding.sourceIds.length} مصدر</small>{canReview ? <div className="geospatial-finding-actions"><button className="primary-button" type="button" disabled={pending || investigating} onClick={() => onReview(finding, "investigate")}><Search size={13} /> {investigating ? "قيد التحقيق" : "فتح تحقيق"}</button><button className="text-button" type="button" disabled={pending} onClick={() => onReview(finding, "dismiss")}><X size={13} /> رفض</button></div> : null}</article>;
}

function selectTreeContext(snapshot: ResearchWorkspaceSnapshot): WorkspaceTreeContext | undefined {
  const { treeId, treeVersionId } = snapshot.context;
  if (treeId && treeVersionId) return snapshot.treeContexts.find((tree) => tree.treeId === treeId && tree.treeVersionId === treeVersionId);
  if (treeId) return snapshot.treeContexts.find((tree) => tree.treeId === treeId);
  if (treeVersionId) return snapshot.treeContexts.find((tree) => tree.treeVersionId === treeVersionId);
  return snapshot.treeContexts.find((tree) => tree.visibility === "public" && tree.state === "published") ?? snapshot.treeContexts[0];
}

function eligibilityMessage(canRun: boolean, mode: EntityMode, snapshot: ResearchWorkspaceSnapshot, treeContext: WorkspaceTreeContext | undefined, targetInTree: boolean, source: { visibility: string; dependencyStatus: string } | undefined): string {
  if (!canRun) return "يتطلب التحليل صلاحية باحث أو مشرف أو مسؤول.";
  if (mode === "source" && !source) return "اختر مصدراً متاحاً من مساحة البحث.";
  if (mode === "source" && source?.visibility !== "public") return "يتطلب تحليل المصدر مصدراً عاماً.";
  if (mode === "person" && (!snapshot.context.entityId || snapshot.context.entityType !== "person")) return "يتطلب التحليل شخصاً محدداً.";
  if (mode === "person" && treeContext && treeContext.visibility !== "public") return "يتطلب التحليل شجرة عامة قابلة للوصول.";
  if (mode === "person" && treeContext && treeContext.state !== "published") return "يتطلب التحليل نسخة شجرة منشورة.";
  if (mode === "person" && !targetInTree) return "الشخص المحدد غير موجود في نسخة الشجرة.";
  return "يتطلب التحليل مصدراً أو شخصاً مختاراً ضمن النطاق المتاح.";
}

function runMessage(run: GeospatialIntelligenceRun): string {
  if (run.reportStatus === "failed") return run.error || "فشل التحليل الجغرافي.";
  if (run.reportStatus === "insufficient_evidence") return "البيانات الجغرافية المؤهلة لا تكفي بعد.";
  return "اكتمل الفحص الجغرافي مع الفصل بين الموثق والاستنتاجي.";
}

function formatNumber(value: number): string {
  return value.toFixed(1);
}

function isUuid(value: string | undefined): boolean {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value));
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return "يلزم تسجيل الدخول لتشغيل هذا التحليل.";
    if (error.status === 403) return "لا تملك صلاحية تشغيل هذا التحليل.";
    if (error.status === 503) return "خدمة التحليل غير متاحة حالياً.";
    return error.message || "تعذر إكمال التحليل الجغرافي.";
  }
  return "تعذر إكمال التحليل الجغرافي.";
}
