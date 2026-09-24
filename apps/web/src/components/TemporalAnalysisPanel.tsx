import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BarChart3, Check, CircleHelp, FileSearch, Search, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError, fetchLatestTemporalAnalysisRun, fetchTemporalFindings, reviewTemporalFinding, startTemporalAnalysis } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { ResearchWorkspaceSnapshot, TemporalAnalysisRun, TemporalFinding, WorkspaceTreeContext } from "../types";

interface TemporalAnalysisPanelProps {
  snapshot: ResearchWorkspaceSnapshot;
  canRun: boolean;
  canReview: boolean;
}

export function TemporalAnalysisPanel({ snapshot, canRun, canReview }: TemporalAnalysisPanelProps) {
  const [run, setRun] = useState<TemporalAnalysisRun | null>(null);
  const [findings, setFindings] = useState<TemporalFinding[]>([]);
  const [message, setMessage] = useState("");
  const queryClient = useQueryClient();
  const treeContext = useMemo(() => selectTreeContext(snapshot), [snapshot]);
  const targetPersonID = snapshot.context.entityType === "person" ? (snapshot.context.entityId ?? "") : "";
  const targetInTree = Boolean(treeContext && (treeContext.targetPresent || treeContext.nodes.some((node) => node.personId === targetPersonID)));
  const questionID = snapshot.context.questionId;
  const canAnalyze = Boolean(canRun && treeContext && treeContext.visibility === "public" && treeContext.state === "published" && isUuid(treeContext.treeId) && isUuid(treeContext.treeVersionId) && isUuid(targetPersonID) && targetInTree);
  const scopeKey = [questionID, treeContext?.treeId ?? "", treeContext?.treeVersionId ?? "", targetPersonID, treeContext?.visibility ?? "", treeContext?.state ?? "", treeContext?.targetPresent ?? false, canRun, canReview].join(":");
  const scopeKeyRef = useRef(scopeKey);
  scopeKeyRef.current = scopeKey;
  const latestRunQuery = useQuery({
    queryKey: ["temporal-analysis-latest", questionID, treeContext?.treeId, treeContext?.treeVersionId, targetPersonID],
    queryFn: () => fetchLatestTemporalAnalysisRun({ question_id: questionID, tree_id: treeContext?.treeId, tree_version_id: treeContext?.treeVersionId, target_person_id: targetPersonID }),
    enabled: canAnalyze && isUuid(questionID),
    staleTime: 60000,
    refetchOnWindowFocus: false,
  });
  const findingsQuery = useQuery({
    queryKey: ["temporal-analysis-findings", run?.id],
    queryFn: () => fetchTemporalFindings(run?.id),
    enabled: Boolean(run && run.findingCount > 0),
  });
  const startMutation = useMutation({
    mutationFn: async ({ requestScopeKey }: { requestScopeKey: string }) => {
      if (requestScopeKey !== scopeKey || !treeContext) throw new Error("تغير سياق التحليل أثناء الطلب.");
      return startTemporalAnalysis({ tree_id: treeContext.treeId, tree_version_id: treeContext.treeVersionId, target_person_id: targetPersonID, question_id: questionID, min_reference_size: 20 });
    },
    onMutate: ({ requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setRun(null);
      setFindings([]);
      setMessage("جارٍ تجهيز المجموعة المؤهلة…");
    },
    onSuccess: (value, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setRun(value);
      setMessage(runMessage(value));
      void queryClient.invalidateQueries({ queryKey: ["research-workspace", questionID] });
      void queryClient.invalidateQueries({ queryKey: ["temporal-analysis-latest", questionID, treeContext?.treeId, treeContext?.treeVersionId, targetPersonID] });
    },
    onError: (error, { requestScopeKey }) => {
      if (scopeKeyRef.current !== requestScopeKey) return;
      setMessage(errorMessage(error));
    },
  });
  const reviewMutation = useMutation({
    mutationFn: async ({ finding, decision, scopeKey: requestScopeKey }: { finding: TemporalFinding; decision: "dismiss" | "confirm" | "investigate"; scopeKey: string }) => {
      const updated = await reviewTemporalFinding(finding.id, decision === "investigate" ? { decision, create_question: true, question_title_ar: `مراجعة فاصل زمني: ${finding.titleAr}` } : { decision });
      return { updated, requestScopeKey };
    },
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
    if (!latestRunQuery.data) return;
    setRun(latestRunQuery.data);
    setMessage(runMessage(latestRunQuery.data));
  }, [latestRunQuery.data]);
  const activeRunID = run?.id;
  const activeFindingCount = run?.findingCount;
  useEffect(() => {
    if (!activeRunID || activeFindingCount === 0) {
      setFindings([]);
      return;
    }
    if (findingsQuery.data) setFindings(findingsQuery.data);
  }, [activeRunID, activeFindingCount, findingsQuery.data]);

  const eligibilityMessage = temporalEligibilityMessage(canRun, snapshot, treeContext, targetInTree);
  const scopeLabel = treeContext ? `${treeContext.treeNameAr} · نسخة ${treeContext.versionNumber}` : "سياق غير محدد";
  const isLoading = startMutation.isPending || latestRunQuery.isFetching;

  return (
    <section className="temporal-analysis-panel" aria-label="التحليل الزمني والإحصائي" aria-busy={isLoading}>
      <div className="temporal-analysis-head">
        <div><div className="eyebrow">الإحصاء الزمني</div><h3>أنماط الفواصل الزمنية</h3><p>الفحص يعرض فواصل الأجيال على المجموعة المؤهلة؛ لا يحوّل النتيجة إلى احتمال تاريخي.</p></div>
        <button className="secondary-button" type="button" aria-describedby={!canAnalyze ? "temporal-analysis-hint" : undefined} onClick={() => startMutation.mutate({ requestScopeKey: scopeKey })} disabled={!canAnalyze || startMutation.isPending}><BarChart3 size={14} /> {startMutation.isPending ? "جارٍ التحليل…" : "حلّل الفواصل"}</button>
      </div>
      {!canAnalyze ? <p className="temporal-analysis-hint" id="temporal-analysis-hint"><CircleHelp size={13} /> {eligibilityMessage}</p> : null}
      {latestRunQuery.isError ? <div className="evidence-workspace-message temporal-error" role="alert">{errorMessage(latestRunQuery.error)}</div> : null}
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {run ? <TemporalReport run={run} findings={findings} findingsLoading={Boolean(run.findingCount > 0 && findingsQuery.isPending)} findingsError={findingsQuery.isError ? errorMessage(findingsQuery.error) : ""} scopeLabel={scopeLabel} canReview={canReview} onReview={(finding, decision) => reviewMutation.mutate({ finding, decision, scopeKey })} reviewPending={reviewMutation.isPending} /> : latestRunQuery.isFetching ? <p className="evidence-empty">جارٍ استعادة آخر تحليل محفوظ…</p> : <p className="evidence-empty">لم يبدأ تحليل إحصائي بعد.</p>}
    </section>
  );
}

function TemporalReport({ run, findings, findingsLoading, findingsError, scopeLabel, canReview, onReview, reviewPending }: { run: TemporalAnalysisRun; findings: TemporalFinding[]; findingsLoading: boolean; findingsError: string; scopeLabel: string; canReview: boolean; onReview: (finding: TemporalFinding, decision: "dismiss" | "confirm" | "investigate") => void; reviewPending: boolean }) {
  const population = run.referencePopulation;
  const reportIsFailed = run.reportStatus === "failed";
  return (
    <div className="temporal-report">
      <div className="temporal-report-summary"><span><strong>{population.referenceEdgeCount}</strong> علاقة مرجعية</span><span><strong>{population.candidateEdgeCount}</strong> علاقة مرشحة</span><span><StatusBadge tone={run.reportStatus === "insufficient_reference" ? "question" : reportIsFailed ? "disputed" : "finding"}>{reportIsFailed ? "فشل التحليل" : run.reportStatus === "insufficient_reference" ? "مرجع غير كافٍ" : "اكتمل المقارنة"}</StatusBadge></span></div>
      <small className="temporal-scope">النطاق: {scopeLabel} · النسخة {population.versionNumber} · {population.targetInPopulation ? "الهدف مؤهل ضمن مجموعة المرشحة" : "الهدف غير مؤهل ضمن مجموعة المرشحة"} · علاقة الهدف مستبعدة من حساب النطاق المرجعي.</small>
      {population.truncated ? <small className="temporal-warning"><CircleHelp size={12} /> تم تجاوز حد حجم العينة؛ لا يُعدّ النطاق المرجعي كاملاً.</small> : null}
      {population.referenceBandAvailable ? <small className="temporal-reference-band">النطاق المرجعي: Q1 {formatYears(population.q1Years)} · الوسيط {formatYears(population.medianYears)} · Q3 {formatYears(population.q3Years)}</small> : <small className="temporal-reference-band">لا يوجد نطاق مرجعي صالح بعد.</small>}
      {population.excludedCounts && Object.keys(population.excludedCounts).length ? <small className="temporal-exclusions">استُبعدت: {Object.entries(population.excludedCounts).map(([key, value]) => `${exclusionLabel(key)} (${value})`).join(" · ")}</small> : null}
      {findingsError ? <small className="temporal-error" role="alert">{findingsError}</small> : findingsLoading ? <p className="evidence-empty">جارٍ تحميل ملاحظات هذا التحليل…</p> : findings.length ? findings.map((finding) => <TemporalFindingCard key={finding.id} finding={finding} canReview={canReview} onReview={onReview} reviewPending={reviewPending} />) : <p className="evidence-empty">{reportIsFailed ? "لم يكتمل التحليل؛ لا تُعرض نتيجة قابلة للتفسير." : run.reportStatus === "insufficient_reference" ? "لا توجد مقارنة كافية؛ لم تُصنَّف أي علاقة كـ«طبيعية»." : "لم يخرج الفاصل عن نطاق المقارنة الحالية."}</p>}
      <small className="temporal-policy">السياسة {run.qualificationPolicyVersion} · الخوارزمية {run.algorithmVersion} · النتيجة تستدعي التحقيق، لا الحكم بالخطأ.</small>
    </div>
  );
}

function TemporalFindingCard({ finding, canReview, onReview, reviewPending }: { finding: TemporalFinding; canReview: boolean; onReview: (finding: TemporalFinding, decision: "dismiss" | "confirm" | "investigate") => void; reviewPending: boolean }) {
  const investigationPending = finding.status === "investigating";
  return <article className="temporal-finding"><div className="temporal-finding-head"><div><strong>{finding.titleAr}</strong><small>{relationLabel(finding.comparison.relation)}</small></div><StatusBadge tone="finding">{statusLabel(finding.status)}</StatusBadge></div><p>{finding.explanationAr}</p><div className="temporal-finding-meta"><span>النطاق: {formatYears(finding.comparison.observed.lowerYears)}–{formatYears(finding.comparison.observed.upperYears)} سنة</span><span>المقارنة: Q1 {formatYears(finding.comparison.q1Years)} · الوسيط {formatYears(finding.comparison.medianYears)} · Q3 {formatYears(finding.comparison.q3Years)}</span><span>تواريخ:</span> <bdi dir="ltr">{finding.comparison.observed.parentBirthFrom} → {finding.comparison.observed.childBirthFrom}</bdi></div><div className="temporal-provenance"><FileSearch size={13} /><span>{finding.claimIds.length} ادعاء · {finding.entityIds.length} شخص مرتبط</span>{finding.claimIds.slice(0, 3).map((claimID) => <bdi dir="ltr" key={claimID}>{claimID.slice(0, 8)}</bdi>)}</div>{canReview ? <div className="temporal-finding-actions"><button className="primary-button" type="button" disabled={reviewPending || investigationPending} onClick={() => onReview(finding, "investigate")}><Search size={13} /> {investigationPending ? "قيد التحقيق" : "فتح تحقيق"}</button><button className="secondary-button" type="button" disabled={reviewPending} onClick={() => onReview(finding, "confirm")}><Check size={13} /> تأكيد الملاحظة</button><button className="text-button" type="button" disabled={reviewPending} onClick={() => onReview(finding, "dismiss")}><X size={13} /> رفض الملاحظة</button></div> : <small className="temporal-review-hint">مراجعة الملاحظة تتطلب صلاحية باحث.</small>}</article>;
}

function selectTreeContext(snapshot: ResearchWorkspaceSnapshot): WorkspaceTreeContext | undefined {
  const { treeId, treeVersionId } = snapshot.context;
  if (treeId && treeVersionId) {
    return snapshot.treeContexts.find((tree) => tree.treeId === treeId && tree.treeVersionId === treeVersionId);
  }
  if (treeId) {
    return snapshot.treeContexts.find((tree) => tree.treeId === treeId);
  }
  if (treeVersionId) {
    return snapshot.treeContexts.find((tree) => tree.treeVersionId === treeVersionId);
  }
  return snapshot.treeContexts.find((tree) => tree.visibility === "public" && tree.state === "published") ?? snapshot.treeContexts.find((tree) => tree.visibility === "public") ?? snapshot.treeContexts[0];
}

function temporalEligibilityMessage(canRun: boolean, snapshot: ResearchWorkspaceSnapshot, treeContext: WorkspaceTreeContext | undefined, targetInTree: boolean): string {
  if (!canRun) return "يتطلب التحليل صلاحية باحث أو مشرف أو مسؤول.";
  if (snapshot.context.entityType !== "person" || !snapshot.context.entityId) return "يتطلب التحليل سياق شخص محدداً.";
  if (!treeContext) return "لا يوجد سياق شجرة متوافق مع الشجرة المحددة.";
  if (treeContext.visibility !== "public") return "يتطلب التحليل شجرة عامة قابلة للوصول.";
  if (treeContext.state !== "published") return "يتطلب التحليل نسخة شجرة منشورة.";
  if (!targetInTree) return "الشخص المحدد غير موجود في نسخة الشجرة المختارة.";
  return "يتطلب التحليل سياق شخص وشجرة منشورة قابلة للوصول.";
}

function runMessage(run: TemporalAnalysisRun): string {
  if (run.reportStatus === "failed") return run.error || "فشل التحليل الزمني.";
  if (run.reportStatus === "insufficient_reference") return "البيانات المؤهلة لا تكفي بعد لمقارنة موثوقة.";
  return "اكتمل التحليل الزمني على البيانات المؤهلة.";
}

function relationLabel(value: TemporalFinding["comparison"]["relation"]): string {
  if (value === "above_reference_range") return "فوق النطاق المرجعي";
  if (value === "below_reference_range") return "تحت النطاق المرجعي";
  return "يتداخل مع النطاق المرجعي";
}

function statusLabel(value: TemporalFinding["status"]): string {
  const labels: Record<TemporalFinding["status"], string> = { needs_review: "تحتاج مراجعة", confirmed: "مؤكدة", dismissed: "مرفوضة", investigating: "قيد التحقيق" };
  return labels[value];
}

function formatYears(value: number): string {
  return value.toFixed(1);
}

function exclusionLabel(value: string): string {
  const labels: Record<string, string> = { unreviewed_identity: "هوية غير مراجعة", missing_source: "مصدر مفقود", private_source: "مصدر خاص", dependent_source: "مصدر مرتبط", missing_claim: "ادعاء مفقود", unaccepted_evidence: "دليل غير مقبول", counter_evidence: "دليل مضاد", invalid_dates: "تواريخ غير صالحة", invalid_chronology: "ترتيب زمني غير ممكن" };
  return labels[value] ?? value;
}

function isUuid(value: string | undefined): boolean {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value));
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return "يلزم تسجيل الدخول لتشغيل هذا التحليل.";
    if (error.status === 403) return "لا تملك صلاحية تشغيل هذا التحليل.";
    if (error.status === 503) return "خدمة التحليل غير متاحة حالياً.";
    return error.message || "تعذر إكمال التحليل الزمني.";
  }
  return "تعذر إكمال التحليل الزمني.";
}
