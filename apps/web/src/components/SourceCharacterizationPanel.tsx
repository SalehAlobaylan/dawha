import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, CircleHelp, FileSearch, RefreshCw, ShieldAlert, X } from "lucide-react";
import { useState } from "react";
import { ApiError, fetchLatestSourceCharacterization, reviewSourceCharacterization, startSourceCharacterization } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { ReviewSourceCharacterizationInput, SourceCharacterizationAttribute, SourceCharacterizationDecision, SourceCharacterizationRun, SourceMetadata } from "../types";

interface SourceCharacterizationPanelProps {
  source: SourceMetadata;
}

export function SourceCharacterizationPanel({ source }: SourceCharacterizationPanelProps) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const [reviewNote, setReviewNote] = useState("");
  const latestQuery = useQuery({
    queryKey: ["source-characterization", source.id],
    queryFn: () => fetchLatestSourceCharacterization({ source_id: source.id }),
    enabled: Boolean(source.id),
    retry: false,
  });
  const startMutation = useMutation({
    mutationFn: () => startSourceCharacterization({ source_id: source.id }),
    onSuccess: (run) => {
      queryClient.setQueryData(["source-characterization", source.id], run);
      setMessage("اكتمل الملف الوصفي، وبقيت حالته قابلة للمراجعة.");
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const reviewMutation = useMutation({
    mutationFn: ({ runId, input }: { runId: string; input: ReviewSourceCharacterizationInput }) => reviewSourceCharacterization(runId, input),
    onSuccess: (run) => {
      queryClient.setQueryData(["source-characterization", source.id], run);
      setReviewNote("");
      setMessage("حُفظ قرار المراجعة، دون تغيير المصدر أو الادعاءات.");
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const run = latestQuery.data ?? null;
  const review = (decision: SourceCharacterizationDecision) => {
    if (!run) return;
    reviewMutation.mutate({ runId: run.id, input: { decision, note_ar: reviewNote || undefined } });
  };

  return (
    <section className="source-characterization-panel" aria-label="توصيف المصدر">
      <div className="source-characterization-head">
        <div>
          <div className="eyebrow">توصيف المصدر</div>
          <h3>ما الذي يمكن قوله عن هذا المصدر؟</h3>
          <p>ملف وصفي منفصل عن الحكم، يوضح ما رُصد وما يحتاج إلى مراجعة.</p>
        </div>
        <button className="secondary-button" type="button" onClick={() => startMutation.mutate()} disabled={startMutation.isPending}>
          <RefreshCw size={14} /> {startMutation.isPending ? "جارٍ التوصيف…" : run ? "تحديث التوصيف" : "توصيف المصدر"}
        </button>
      </div>
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {latestQuery.error ? <div className="evidence-workspace-error" role="alert">{errorMessage(latestQuery.error)}</div> : null}
      {latestQuery.isPending ? <p className="evidence-empty">جارٍ استعادة آخر ملف وصفي…</p> : null}
      {!latestQuery.isPending && !latestQuery.error && !run ? <div className="source-characterization-empty"><CircleHelp size={18} /><p>لا يوجد ملف توصيف لهذا المصدر بعد. ابدأ بفحصاً وصفياً، من دون تحويله إلى درجة ثقة.</p></div> : null}
      {run ? <SourceCharacterizationReport run={run} pending={reviewMutation.isPending} reviewNote={reviewNote} setReviewNote={setReviewNote} onReview={review} /> : null}
    </section>
  );
}

function SourceCharacterizationReport({ run, pending, reviewNote, setReviewNote, onReview }: { run: SourceCharacterizationRun; pending: boolean; reviewNote: string; setReviewNote: (value: string) => void; onReview: (decision: SourceCharacterizationDecision) => void }) {
  const attributes = run.report?.attributes ?? [];
  const corroboration = run.report?.corroboration;
  return (
    <div className="source-characterization-report">
      <div className="source-characterization-status">
        <div><StatusBadge tone={run.reportStatus === "insufficient_evidence" ? "question" : "source"}>{reportStatusLabel(run.reportStatus)}</StatusBadge><StatusBadge tone={reviewStatusTone(run.reviewStatus)}>{reviewStatusLabel(run.reviewStatus)}</StatusBadge></div>
        <small>{run.algorithmVersion} · {formatDate(run.report?.generatedAt || run.completedAt || run.createdAt)}</small>
      </div>
      <div className="source-characterization-notice"><ShieldAlert size={14} /><span>هذا الملف لا يعطي المصدر درجة «موثوق» أو «غير موثوق»، ولا يغير أي ادعاء أو مصدر.</span></div>
      <div className="source-characterization-attributes">
        {attributes.map((attribute) => <SourceCharacterizationAttributeCard attribute={attribute} key={attribute.key} />)}
      </div>
      <section className="source-characterization-corroboration">
        <div className="source-characterization-subhead"><FileSearch size={14} /><strong>التأكيد المستقل</strong><small>{corroboration?.independentSourceCount ?? 0} مصدراً مرتبطاً بالادعاءات</small></div>
        {corroboration?.sources.length ? <div className="source-characterization-corroborator-list">{corroboration.sources.map((source) => <div key={source.sourceId}><strong>{source.sourceTitleAr}</strong><small>{source.evidenceCount} رابطاً · {source.claimIds.length} ادعاء</small></div>)}</div> : <p className="evidence-empty">لا يوجد تأكيد مستقل محفوظ لهذا النطاق.</p>}
      </section>
      {run.report?.limitations.length ? <section className="source-characterization-limitations"><div className="source-characterization-subhead"><CircleHelp size={14} /><strong>حدود الملف</strong></div><ul>{run.report.limitations.map((limitation) => <li key={limitation}>{limitation}</li>)}</ul></section> : null}
      {run.evidence.length ? <details className="source-characterization-trace"><summary><FileSearch size={14} /> الأدلة القابلة للتتبع ({run.evidence.length})</summary><div className="source-characterization-trace-list">{run.evidence.map((evidence) => <div key={evidence.id}><strong>{evidence.attributeKey}</strong><p>{evidence.excerptAr}</p><small>{evidence.claimId ? `ادعاء ${evidence.claimId.slice(0, 8)} · ` : ""}{evidence.relatedSourceId ? `مصدر ${evidence.relatedSourceId.slice(0, 8)}` : evidence.sourceStatementId ? `عبارة ${evidence.sourceStatementId.slice(0, 8)}` : "مرجع مصدري"}</small></div>)}</div></details> : null}
      <div className="source-characterization-review">
        {run.reviewStatus === "needs_review" ? <><label className="composer-label">ملاحظة المراجعة<textarea value={reviewNote} onChange={(event) => setReviewNote(event.target.value)} rows={2} placeholder="ما الذي perlu حفظه مع هذا الملف؟" /></label><div><button className="primary-button" type="button" disabled={pending} onClick={() => onReview("confirm")}><Check size={13} /> تأكيد الملف</button><button className="secondary-button" type="button" disabled={pending} onClick={() => onReview("dismiss")}><X size={13} /> رفض الملف</button></div></> : <div><span>القرار المحفوظ: {reviewStatusLabel(run.reviewStatus)}</span><button className="text-button" type="button" disabled={pending} onClick={() => onReview("reopen")}><RefreshCw size={13} /> إعادة فتح المراجعة</button></div>}
      </div>
    </div>
  );
}

function SourceCharacterizationAttributeCard({ attribute }: { attribute: SourceCharacterizationAttribute }) {
  return <article className={`source-characterization-attribute source-characterization-attribute-${attribute.state}`}><div><strong>{attribute.labelAr}</strong><StatusBadge tone={attributeTone(attribute.state)}>{attributeStateLabel(attribute.state)}</StatusBadge></div><bdi dir="ltr">{attribute.value}</bdi><p>{attribute.rationaleAr}</p><small>{attribute.evidenceIds.length} مرجعاً</small></article>;
}

function attributeTone(state: SourceCharacterizationAttribute["state"]): "source" | "finding" | "question" {
  if (state === "observed") return "source";
  if (state === "needs_review") return "finding";
  return "question";
}

function attributeStateLabel(state: SourceCharacterizationAttribute["state"]): string {
  if (state === "observed") return "مرصود";
  if (state === "needs_review") return "يحتاج مراجعة";
  return "غير محدد";
}

function reportStatusLabel(status: SourceCharacterizationRun["reportStatus"]): string {
  if (status === "insufficient_evidence") return "أدلة غير كافية";
  if (status === "failed") return "فشل التوصيف";
  return "اكتمل التوصيف";
}

function reviewStatusLabel(status: SourceCharacterizationRun["reviewStatus"]): string {
  if (status === "confirmed") return "مؤكد";
  if (status === "dismissed") return "مرفوض";
  return "بانتظار المراجعة";
}

function reviewStatusTone(status: SourceCharacterizationRun["reviewStatus"]): "source" | "finding" | "disputed" {
  if (status === "confirmed") return "source";
  if (status === "dismissed") return "disputed";
  return "finding";
}

function formatDate(value: string): string {
  return new Date(value).toLocaleString("ar");
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "تعذر إكمال توصيف المصدر.";
}
