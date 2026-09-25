import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, BookOpen, BrainCircuit, CheckCircle2, CircleHelp, FileSearch, ListChecks, ShieldAlert, Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError, fetchLatestResearchAgentRun, startResearchAgent } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { ResearchAgentEvidenceRef, ResearchAgentRun, ResearchWorkspaceSnapshot, WorkspaceTreeContext } from "../types";

interface ResearchAgentPanelProps {
  snapshot: ResearchWorkspaceSnapshot;
  canRun: boolean;
}

export function ResearchAgentPanel({ snapshot, canRun }: ResearchAgentPanelProps) {
  const treeContext = useMemo(() => selectTreeContext(snapshot), [snapshot]);
  const personID = snapshot.context.entityType === "person" ? snapshot.context.entityId ?? "" : "";
  const questionID = snapshot.context.questionId;
  const [question, setQuestion] = useState(snapshot.context.questionTitleAr);
  const [run, setRun] = useState<ResearchAgentRun | null>(null);
  const [message, setMessage] = useState("");
  const queryClient = useQueryClient();
  const targetInTree = Boolean(treeContext && (treeContext.targetPresent || treeContext.nodes.some((node) => node.personId === personID)));
  const treeEligible = !treeContext || (treeContext.visibility === "public" && treeContext.state === "published" && targetInTree);
  const canAnalyze = Boolean(canRun && isUuid(questionID) && isUuid(personID) && snapshot.context.entityType === "person" && (!treeContext || (isUuid(treeContext.treeId) && isUuid(treeContext.treeVersionId) && treeEligible)));
  const scopeKey = [questionID, personID, treeContext?.treeId ?? "", treeContext?.treeVersionId ?? "", question].join(":");
  const scopeKeyRef = useRef(scopeKey);
  scopeKeyRef.current = scopeKey;
  const latestQuery = useQuery({
    queryKey: ["research-agent-latest", questionID, personID, treeContext?.treeVersionId],
    queryFn: () => fetchLatestResearchAgentRun({ question_id: questionID, entity_type: "person", entity_id: personID }),
    enabled: canAnalyze,
    staleTime: 60000,
    refetchOnWindowFocus: false,
  });
  const startMutation = useMutation({
    mutationFn: async ({ requestScopeKey }: { requestScopeKey: string }) => {
      if (requestScopeKey !== scopeKey) throw new Error("تغير سياق التحقيق أثناء الطلب.");
      return startResearchAgent({ question: question.trim(), question_id: questionID, entity_type: "person", entity_id: personID, tree_id: treeContext?.treeId, tree_version_id: treeContext?.treeVersionId });
    },
    onMutate: ({ requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setRun(null);
      setMessage("جارٍ تفكيك السؤال وجمع الأدلة المؤهلة…");
    },
    onSuccess: (value, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setRun(value);
      setMessage(runMessage(value));
      void queryClient.invalidateQueries({ queryKey: ["research-agent-latest", questionID, personID, treeContext?.treeVersionId] });
      void queryClient.invalidateQueries({ queryKey: ["research-workspace", questionID] });
    },
    onError: (error, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setMessage(errorMessage(error));
    },
  });
  useEffect(() => {
    setRun(null);
    setMessage("");
  }, [scopeKey]);
  useEffect(() => {
    if (latestQuery.data) {
      setRun(latestQuery.data);
      setMessage(runMessage(latestQuery.data));
    }
  }, [latestQuery.data]);
  const hint = eligibilityMessage(canRun, snapshot, treeContext, targetInTree);
  const isLoading = startMutation.isPending || latestQuery.isFetching;
  return <section className="research-agent-panel" aria-label="وكيل البحث" aria-busy={isLoading}>
    <div className="research-agent-head"><div><div className="eyebrow">تحقيق متعدد المراحل</div><h3>حزمة بحث قابلة للتتبع</h3><p>يجمع الوكيل طبقات الأدلة، يعرض التعارض، ويقترح الخطوة التالية دون حسم تاريخي.</p></div><div className="research-agent-actions"><label className="research-agent-question"><Sparkles size={13} /><input value={question} onChange={(event) => setQuestion(event.target.value)} aria-label="سؤال التحقيق" /></label><button className="primary-button" type="button" onClick={() => startMutation.mutate({ requestScopeKey: scopeKey })} disabled={!canAnalyze || startMutation.isPending || question.trim().length < 3}><BrainCircuit size={14} /> {startMutation.isPending ? "جارٍ التحقيق…" : "شغّل وكيل البحث"}</button></div></div>
    {!canAnalyze ? <p className="research-agent-hint" id="research-agent-hint"><CircleHelp size={13} /> {hint}</p> : null}
    {latestQuery.isError ? <div className="research-agent-error" role="alert">{errorMessage(latestQuery.error)}</div> : null}
    {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
    {run ? <ResearchAgentReport run={run} /> : latestQuery.isFetching ? <p className="evidence-empty">جارٍ استعادة آخر حزمة بحث محفوظة…</p> : <p className="evidence-empty">لم يبدأ تحقيق الوكيل بعد.</p>}
  </section>;
}

function ResearchAgentReport({ run }: { run: ResearchAgentRun }) {
  const report = run.report;
  const evidence = run.evidence ?? report.evidencePackage?.evidence ?? [];
  return <div className="research-agent-report">
    <div className="research-agent-summary"><div><span className="research-agent-answer-label"><CheckCircle2 size={14} /> الخلاصة</span><strong>{report.answerAr || "لم تُنتج حزمة إجابة نصية."}</strong></div><StatusBadge tone={run.resolution === "unresolved" ? "question" : "interpretation"}>{run.resolution === "unresolved" ? "غير محسومة" : "حزمة مكتملة"}</StatusBadge></div>
    <div className="research-agent-metrics"><span><strong>{run.stepCount}</strong> خطوة</span><span><strong>{run.evidenceCount}</strong> مادة</span><span><strong>{run.gapCount}</strong> فجوة</span><span><strong>{run.recommendationCount}</strong> توصية</span><span>{run.report.scope.entityType} · {run.entityId.slice(0, 8)}</span></div>
    <div className="research-agent-columns"><section><div className="research-agent-subhead"><ListChecks size={14} /><strong>مسار التحقيق</strong><small>كل خطوة مقروءة فقط</small></div><div className="research-agent-steps">{(run.steps ?? []).map((step) => <div className={`research-agent-step research-agent-step-${step.status}`} key={step.id}><span className="research-agent-step-index">{step.order}</span><div><strong>{step.stage}</strong><small>{step.tool} · {step.evidenceCount} مادة</small></div><StatusBadge tone={step.status === "unresolved" ? "question" : "source"}>{step.status === "succeeded" ? "تمت" : step.status === "unresolved" ? "دون حسم" : step.status}</StatusBadge></div>)}</div></section><section><div className="research-agent-subhead"><ShieldAlert size={14} /><strong>الفجوات والتوصيات</strong><small>لا تحسم تلقائياً</small></div><div className="research-agent-gap-list">{run.gaps?.length ? run.gaps.map((gap) => <div key={gap.id}><AlertTriangle size={13} /><span><strong>{gap.descriptionAr}</strong><small>{gap.kind} · {gap.severity}</small></span></div>) : <p className="evidence-empty">لم تُسجل فجوات صريحة.</p>}{run.recommendations?.length ? run.recommendations.map((item) => <div className="research-agent-recommendation" key={item.id}><ArrowLeft size={13} /><span><strong>{item.action}</strong><small>{item.rationaleAr}</small></span></div>) : null}</div></section></div>
    <section className="research-agent-evidence"><div className="research-agent-subhead"><FileSearch size={14} /><strong>الأدلة والمراجع</strong><small>{evidence.length} عنصراً قابلاً للتتبع</small></div><div className="research-agent-evidence-grid">{evidence.slice(0, 12).map((item) => <AgentEvidenceLine item={item} key={`${item.referenceType}-${item.referenceId}-${item.stance}`} />)}{evidence.length > 12 ? <small className="research-agent-truncated">عرض 12 من {evidence.length} عنصراً؛ التقرير المحفوظ يحتفظ بالحدود كاملة.</small> : null}{evidence.length === 0 ? <p className="evidence-empty">لا توجد أدلة مؤهلة في النطاق الحالي.</p> : null}</div></section>
    <div className="research-agent-policy"><BookOpen size={13} /><span>السياسة {run.qualificationPolicyVersion} · الخوارزمية {run.algorithmVersion} · الإجراءات المحظورة: {report.restrictedActions?.join("، ") || "غير محددة"}</span></div>
  </div>;
}

function AgentEvidenceLine({ item }: { item: ResearchAgentEvidenceRef }) {
  const title = typeof item.metadata.title === "string" ? item.metadata.title : item.referenceType;
  return <article className={`research-agent-evidence-line research-agent-stance-${item.stance}`}><div><span>{layerLabel(item.layer)}</span><StatusBadge tone={item.stance === "counter_evidence" ? "disputed" : item.stance === "hypothesis" ? "question" : "source"}>{stanceLabel(item.stance)}</StatusBadge></div><strong>{title}</strong><p>{item.excerpt || "دون مقتطف نصي."}</p><small><FileSearch size={11} /> {item.sourceId ? item.sourceId.slice(0, 8) : item.referenceId.slice(0, 8)} · {item.referenceType}</small></article>;
}

function selectTreeContext(snapshot: ResearchWorkspaceSnapshot): WorkspaceTreeContext | undefined {
  const { treeId, treeVersionId } = snapshot.context;
  if (treeId && treeVersionId) return snapshot.treeContexts.find((tree) => tree.treeId === treeId && tree.treeVersionId === treeVersionId);
  if (treeId) return snapshot.treeContexts.find((tree) => tree.treeId === treeId);
  if (treeVersionId) return snapshot.treeContexts.find((tree) => tree.treeVersionId === treeVersionId);
  return snapshot.treeContexts.find((tree) => tree.visibility === "public" && tree.state === "published") ?? snapshot.treeContexts[0];
}

function eligibilityMessage(canRun: boolean, snapshot: ResearchWorkspaceSnapshot, treeContext: WorkspaceTreeContext | undefined, targetInTree: boolean): string {
  if (!canRun) return "يتطلب تشغيل الوكيل صلاحية باحث أو مشرف أو مسؤول.";
  if (snapshot.context.entityType !== "person" || !snapshot.context.entityId) return "يتطلب تشغيل الوكيل سياق شخص محدداً.";
  if (!treeContext) return "لا يوجد سياق شجرة متاح؛ سيبقى فحص المسار البياني غير محسوم.";
  if (treeContext.visibility !== "public" || treeContext.state !== "published") return "يتطلب المسار الشجري نسخة عامة منشورة.";
  if (!targetInTree) return "الشخص المحدد غير موجود في نسخة الشجرة المختارة.";
  return "يتطلب تشغيل الوكيل سياق شخص وسؤالاً محدداً.";
}

function runMessage(run: ResearchAgentRun): string {
  if (run.status === "failed") return run.error || "فشل وكيل البحث.";
  if (run.resolution === "unresolved") return "اكتملت الحزمة مع فجوات أو حالات غير محسومة.";
  return "اكتملت الحزمة مع إبقاء الأدلة قابلة للمراجعة.";
}

function layerLabel(value: string): string {
  const labels: Record<string, string> = { source_statement: "عبارة مصدر", research_claim: "ادعاء", tree_interpretation: "تفسير شجرة", geographic_signal: "إشارة جغرافية", temporal_signal: "إشارة زمنية", source_dependency: "اعتماد مصدر", graph_structure: "بنية graph" };
  return labels[value] ?? value;
}

function stanceLabel(value: ResearchAgentEvidenceRef["stance"]): string {
  if (value === "counter_evidence") return "مضاد";
  if (value === "hypothesis") return "فرضية";
  if (value === "supports") return "داعم";
  return "سياق";
}

function isUuid(value: string | undefined): boolean {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value));
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return "يلزم تسجيل الدخول لتشغيل وكيل البحث.";
    if (error.status === 403) return "لا تملك صلاحية تشغيل وكيل البحث.";
    if (error.status === 503) return "خدمة وكيل البحث غير متاحة حالياً.";
    return error.message || "تعذر إكمال تحقيق وكيل البحث.";
  }
  return "تعذر إكمال تحقيق وكيل البحث.";
}
