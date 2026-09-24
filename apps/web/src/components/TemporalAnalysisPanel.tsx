import { useMutation } from "@tanstack/react-query";
import { BarChart3, Check, CircleHelp, Search, X } from "lucide-react";
import { useState } from "react";
import { ApiError, fetchTemporalFindings, reviewTemporalFinding, startTemporalAnalysis } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { ResearchWorkspaceSnapshot, TemporalAnalysisRun, TemporalFinding } from "../types";

interface TemporalAnalysisPanelProps {
  snapshot: ResearchWorkspaceSnapshot;
  canRun: boolean;
}

export function TemporalAnalysisPanel({ snapshot, canRun }: TemporalAnalysisPanelProps) {
  const [run, setRun] = useState<TemporalAnalysisRun | null>(null);
  const [findings, setFindings] = useState<TemporalFinding[]>([]);
  const [message, setMessage] = useState("");
  const treeContext = snapshot.treeContexts[0];
  const targetPersonID = snapshot.context.entityType === "person" ? (snapshot.context.entityId ?? "") : "";
  const canAnalyze = Boolean(canRun && treeContext && isUuid(treeContext.treeId) && isUuid(treeContext.treeVersionId) && isUuid(targetPersonID));
  const startMutation = useMutation({
    mutationFn: () => {
      if (!treeContext) throw new Error("لا يوجد سياق شجرة صالح للتحليل.");
      return startTemporalAnalysis({ tree_id: treeContext.treeId, tree_version_id: treeContext.treeVersionId, target_person_id: targetPersonID, question_id: snapshot.context.questionId, min_reference_size: 20 });
    },
    onSuccess: async (value) => {
      setRun(value);
      setMessage(value.reportStatus === "insufficient_reference" ? "البيانات المؤهلة لا تكفي بعد لمقارنة موثوقة." : "اكتمل التحليل الزمني على البيانات المؤهلة.");
      if (value.findingCount > 0) {
        const response = await fetchTemporalFindings(value.id);
        setFindings(response);
      } else {
        setFindings([]);
      }
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const reviewMutation = useMutation({
    mutationFn: ({ finding, decision }: { finding: TemporalFinding; decision: "dismiss" | "confirm" | "investigate" }) => reviewTemporalFinding(finding.id, { decision }),
    onSuccess: (updated) => {
      setFindings((current) => current.map((finding) => finding.id === updated.id ? updated : finding));
      setMessage("حُفظ قرار المراجعة.");
    },
    onError: (error) => setMessage(errorMessage(error)),
  });

  return (
    <section className="temporal-analysis-panel" aria-label="التحليل الزمني والإحصائي">
      <div className="temporal-analysis-head">
        <div><div className="eyebrow">الإحصاء الزمني</div><h3>أنماط الفواصل الزمنية</h3><p>الفحص يعرض فواصل الأجيال على المجموعة المؤهلة؛ لا يحوّل النتيجة إلى احتمال تاريخي.</p></div>
        <button className="secondary-button" type="button" onClick={() => startMutation.mutate()} disabled={!canAnalyze || startMutation.isPending}><BarChart3 size={14} /> {startMutation.isPending ? "جارٍ التحليل…" : "حلّل الفواصل"}</button>
      </div>
      {!canAnalyze ? <p className="temporal-analysis-hint"><CircleHelp size={13} /> يتطلب التحليل سياق شخص وشجرة منشورة قابلة للوصول.</p> : null}
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {run ? <TemporalReport run={run} findings={findings} onReview={(finding, decision) => reviewMutation.mutate({ finding, decision })} reviewPending={reviewMutation.isPending} /> : <p className="evidence-empty">لم يبدأ تحليل إحصائي بعد.</p>}
    </section>
  );
}

function TemporalReport({ run, findings, onReview, reviewPending }: { run: TemporalAnalysisRun; findings: TemporalFinding[]; onReview: (finding: TemporalFinding, decision: "dismiss" | "confirm" | "investigate") => void; reviewPending: boolean }) {
  const population = run.referencePopulation;
  return (
    <div className="temporal-report">
      <div className="temporal-report-summary"><span><strong>{population.referenceEdgeCount}</strong> علاقة مرجعية</span><span><strong>{population.candidateEdgeCount}</strong> علاقة مرشحة</span><span><StatusBadge tone={run.reportStatus === "insufficient_reference" ? "question" : "finding"}>{run.reportStatus === "insufficient_reference" ? "مرجع غير كافٍ" : "اكتمل المقارنة"}</StatusBadge></span></div>
      {population.referenceBandAvailable ? <small className="temporal-reference-band">النطاق المرجعي: Q1 {formatYears(population.q1Years)} · الوسيط {formatYears(population.medianYears)} · Q3 {formatYears(population.q3Years)}</small> : <small className="temporal-reference-band">لا يوجد نطاق مرجعي صالح بعد.</small>}
      {population.excludedCounts && Object.keys(population.excludedCounts).length ? <small className="temporal-exclusions">استُبعدت: {Object.entries(population.excludedCounts).map(([key, value]) => `${exclusionLabel(key)} (${value})`).join(" · ")}</small> : null}
      {findings.length ? findings.map((finding) => <article className="temporal-finding" key={finding.id}><div className="temporal-finding-head"><div><strong>{finding.titleAr}</strong><small>{finding.comparison.relation === "above_reference_range" ? "فوق النطاق المرجعي" : "تحت النطاق المرجعي"}</small></div><StatusBadge tone="finding">{finding.status}</StatusBadge></div><p>{finding.explanationAr}</p><div className="temporal-finding-meta"><span>النطاق: {formatYears(finding.comparison.observed.lowerYears)}–{formatYears(finding.comparison.observed.upperYears)} سنة</span><span>المقارنة: Q1 {formatYears(finding.comparison.q1Years)} · الوسيط {formatYears(finding.comparison.medianYears)} · Q3 {formatYears(finding.comparison.q3Years)}</span></div><div className="temporal-finding-actions"><button className="primary-button" type="button" disabled={reviewPending} onClick={() => onReview(finding, "investigate")}><Search size={13} /> تحقيق</button><button className="secondary-button" type="button" disabled={reviewPending} onClick={() => onReview(finding, "confirm")}><Check size={13} /> قبول</button><button className="text-button" type="button" disabled={reviewPending} onClick={() => onReview(finding, "dismiss")}><X size={13} /> إخفاء</button></div></article>) : <p className="evidence-empty">{run.reportStatus === "insufficient_reference" ? "لا توجد مقارنة كافية؛ لم تُصنَّف أي علاقة كـ«طبيعية»." : "لم يخرج الفاصل عن نطاق المقارنة الحالية."}</p>}
      <small className="temporal-policy">السياسة {run.qualificationPolicyVersion} · الخوارزمية {run.algorithmVersion} · النتيجة تستدعي التحقيق، لا الحكم بالخطأ.</small>
    </div>
  );
}

function formatYears(value: number): string {
  return `${value.toFixed(1)}`;
}

function exclusionLabel(value: string): string {
  const labels: Record<string, string> = { unreviewed_identity: "هوية غير مراجعة", missing_source: "مصدر مفقود", private_source: "مصدر خاص", dependent_source: "مصدر مرتبط", missing_claim: "ادعاء مفقود", unaccepted_evidence: "دليل غير مقبول", counter_evidence: "دليل مضاد", invalid_dates: "تواريخ غير صالحة" };
  return labels[value] ?? value;
}

function isUuid(value: string | undefined): boolean {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value));
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "تعذر إكمال التحليل الزمني.";
}
