import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { AlertTriangle, ArrowLeft, BookOpen, BrainCircuit, Check, CheckCircle2, CircleHelp, ExternalLink, FileSearch, ListChecks, Plus, ShieldAlert, Sparkles, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError, fetchLatestResearchAgentRun, fetchResearchQuestionCandidates, generateResearchQuestionCandidates, reviewResearchQuestionCandidate, startResearchAgent } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { ResearchAgentEvidenceRef, ResearchAgentRun, ResearchQuestionCandidate, ResearchWorkspaceSnapshot, ReviewResearchQuestionCandidateInput, WorkspaceTreeContext } from "../types";

interface ResearchAgentPanelProps {
  snapshot: ResearchWorkspaceSnapshot;
  canRun: boolean;
  canReview: boolean;
}

export function ResearchAgentPanel({ snapshot, canRun, canReview }: ResearchAgentPanelProps) {
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
    {run ? <><ResearchAgentReport run={run} /><ResearchQuestionCandidates run={run} canReview={canReview} scopeKey={scopeKey} /></> : latestQuery.isFetching ? <p className="evidence-empty">جارٍ استعادة آخر حزمة بحث محفوظة…</p> : <p className="evidence-empty">لم يبدأ تحقيق الوكيل بعد.</p>}
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

function ResearchQuestionCandidates({ run, canReview, scopeKey }: { run: ResearchAgentRun; canReview: boolean; scopeKey: string }) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const scopeKeyRef = useRef(scopeKey);
  scopeKeyRef.current = scopeKey;
  const candidatesQuery = useQuery({
    queryKey: ["research-question-candidates", run.id],
    queryFn: () => fetchResearchQuestionCandidates(run.id),
    enabled: canReview,
    staleTime: 30000,
    refetchOnWindowFocus: false,
  });
  const generateMutation = useMutation({
    mutationFn: async ({ requestScopeKey }: { requestScopeKey: string }) => {
      if (requestScopeKey !== scopeKey) throw new Error("تغير سياق المرشحين أثناء الطلب.");
      return generateResearchQuestionCandidates(run.id);
    },
    onMutate: ({ requestScopeKey }) => {
      if (scopeKeyRef.current === requestScopeKey) setMessage("جارٍ تحويل الفجوات إلى مرشحي سؤال…");
    },
    onSuccess: (items, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      queryClient.setQueryData(["research-question-candidates", run.id], items);
      setMessage(items.length ? "أصبحت مرشحات الأسئلة جاهزة للمراجعة." : "لا توجد فجوات قابلة للتحويل إلى سؤال.");
    },
    onError: (error, { requestScopeKey }) => {
      if (scopeKeyRef.current === requestScopeKey) setMessage(candidateErrorMessage(error));
    },
  });
  const reviewMutation = useMutation({
    mutationFn: async ({ candidate, input, requestScopeKey }: { candidate: ResearchQuestionCandidate; input: ReviewResearchQuestionCandidateInput; requestScopeKey: string }) => {
      if (requestScopeKey !== scopeKey) throw new Error("تغير سياق المرشحين أثناء الطلب.");
      return reviewResearchQuestionCandidate(candidate.id, input);
    },
    onSuccess: (updated, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      queryClient.setQueryData<ResearchQuestionCandidate[]>(["research-question-candidates", run.id], (current) => (current ?? []).map((item) => item.id === updated.id ? updated : item));
      setMessage(updated.status === "converted" ? "حوّل المرشح إلى سؤال مفتوح." : "أُغلق المرشح دون تحويله إلى سؤال.");
      void queryClient.invalidateQueries({ queryKey: ["research-workspace", run.questionId] });
    },
    onError: (error, { requestScopeKey }) => {
      if (scopeKeyRef.current === requestScopeKey) setMessage(candidateErrorMessage(error));
    },
  });
  useEffect(() => {
    setMessage("");
  }, [scopeKey]);
  if (!canReview) {
    return <section className="research-question-candidates"><div className="research-agent-subhead"><CircleHelp size={14} /><strong>مرشحو الأسئلة</strong></div><p className="research-agent-hint">مراجعة مرشحي الأسئلة تتطلب صلاحية باحث أو مشرف أو مسؤول.</p></section>;
  }
  const candidates = candidatesQuery.data ?? [];
  return <section className="research-question-candidates"><div className="research-question-candidates-head"><div className="research-agent-subhead"><ListChecks size={14} /><strong>مرشحو الأسئلة</strong><small>الفجوات لا تتحول إلى سؤال دون مراجعة بشرية</small></div><button className="secondary-button" type="button" onClick={() => generateMutation.mutate({ requestScopeKey: scopeKey })} disabled={run.gapCount === 0 || generateMutation.isPending}><Plus size={13} /> {generateMutation.isPending ? "جارٍ الإنشاء…" : "اقترح أسئلة من الفجوات"}</button></div>{message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}{candidatesQuery.isError ? <div className="research-agent-error" role="alert">{candidateErrorMessage(candidatesQuery.error)}</div> : null}{candidates.length ? <div className="research-question-candidate-list">{candidates.map((candidate) => <ResearchQuestionCandidateCard candidate={candidate} key={candidate.id} pending={reviewMutation.isPending} onReview={(item, input) => reviewMutation.mutate({ candidate: item, input, requestScopeKey: scopeKey })} />)}</div> : candidatesQuery.isFetching ? <p className="evidence-empty">جارٍ استعادة المرشحات…</p> : <p className="evidence-empty">لم تُنشأ مرشحات بعد. استخدم زر الاقتراح بعد مراجعة الفجوات.</p>}</section>;
}

function ResearchQuestionCandidateCard({ candidate, pending, onReview }: { candidate: ResearchQuestionCandidate; pending: boolean; onReview: (candidate: ResearchQuestionCandidate, input: ReviewResearchQuestionCandidateInput) => void }) {
  const [title, setTitle] = useState(candidate.titleAr);
  const [priority, setPriority] = useState(candidate.priority);
  useEffect(() => {
    setTitle(candidate.titleAr);
    setPriority(candidate.priority);
  }, [candidate.id, candidate.titleAr, candidate.priority]);
  const converted = candidate.status === "converted";
  const dismissed = candidate.status === "dismissed";
  return <article className={`research-question-candidate research-question-candidate-${candidate.status}`}><div className="research-question-candidate-head"><div><span className="eyebrow">فجوة {typeof candidate.metadata.gapKind === "string" ? gapKindLabel(candidate.metadata.gapKind) : "بحث"}</span><StatusBadge tone={converted ? "source" : dismissed ? "disputed" : "question"}>{candidateStatusLabel(candidate.status)}</StatusBadge></div><small>{formatCandidateDate(candidate.createdAt)}</small></div>{candidate.status === "proposed" ? <input className="research-question-candidate-title" value={title} onChange={(event) => setTitle(event.target.value)} aria-label="عنوان المرشح" /> : <strong>{candidate.titleAr}</strong>}<p>{candidate.descriptionAr}</p><div className="research-question-candidate-meta"><span>الأولوية: {priorityLabel(candidate.priority)}</span><span>مراجعات: {candidate.reviews.length}</span>{candidate.originQuestionId ? <span>سؤال المصدر: {candidate.originQuestionId.slice(0, 8)}</span> : null}</div>{candidate.status === "proposed" ? <div className="research-question-candidate-actions"><label><span>الأولوية</span><select value={priority} onChange={(event) => setPriority(event.target.value as typeof priority)}><option value="low">منخفضة</option><option value="normal">عادية</option><option value="high">مرتفعة</option></select></label><button className="primary-button" type="button" disabled={pending || title.trim().length < 3} onClick={() => onReview(candidate, { decision: "converted", title_ar: title.trim(), priority })}><Check size={13} /> تحويل إلى سؤال</button><button className="text-button" type="button" disabled={pending} onClick={() => onReview(candidate, { decision: "dismissed" })}><X size={13} /> رفض</button></div> : converted && candidate.questionId ? <Link className="text-button" to="/research/$questionId" params={{ questionId: candidate.questionId }}><ExternalLink size={13} /> فتح السؤال المحفوظ</Link> : null}</article>;
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

function gapKindLabel(value: string): string {
  const labels: Record<string, string> = { missing_source_evidence: "أدلة مصدرية", missing_counter_evidence: "أدلة مضادة", source_dependency: "اعتماد مصادر", missing_graph_path: "مسار شجري", missing_geography: "جغرافيا", missing_chronology: "تسلسل زمني" };
  return labels[value] ?? "بحث";
}

function candidateStatusLabel(value: ResearchQuestionCandidate["status"]): string {
  if (value === "converted") return "محول إلى سؤال";
  if (value === "dismissed") return "مرفوض";
  return "بانتظار المراجعة";
}

function priorityLabel(value: ResearchQuestionCandidate["priority"]): string {
  if (value === "high") return "مرتفعة";
  if (value === "low") return "منخفضة";
  return "عادية";
}

function formatCandidateDate(value: string): string {
  return value.slice(0, 10);
}

function candidateErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return "يلزم تسجيل الدخول لمراجعة المرشحين.";
    if (error.status === 403) return "لا تملك صلاحية مراجعة مرشحي الأسئلة.";
    if (error.status === 409) return "سبق مراجعة هذا المرشح.";
    if (error.status === 503) return "خدمة مرشحي الأسئلة غير متاحة حالياً.";
    return error.message || "تعذر إكمال مراجعة مرشح السؤال.";
  }
  return "تعذر إكمال مراجعة مرشح السؤال.";
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
