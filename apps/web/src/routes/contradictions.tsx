import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, RefreshCw, ShieldQuestion } from "lucide-react";
import { useState } from "react";
import { ApiError, fetchContradictionFindings, fetchContradictionRun, reviewContradictionFinding, startContradictionRun } from "../lib/api";
import type { ContradictionFinding } from "../types";
import { SectionHeading } from "../components/SectionHeading";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

export function ContradictionsPage() {
  const queryClient = useQueryClient();
  const [runId, setRunId] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const findingsQuery = useQuery({ queryKey: ["contradiction-findings", runId], queryFn: () => fetchContradictionFindings(runId || undefined, "needs_review") });
  const runQuery = useQuery({
    queryKey: ["contradiction-run", runId],
    queryFn: () => fetchContradictionRun(runId),
    enabled: Boolean(runId),
    refetchInterval: (query) => {
      const run = query.state.data;
      return run?.status === "queued" || run?.status === "running" ? 1500 : false;
    },
  });
  const startMutation = useMutation({
    mutationFn: startContradictionRun,
    onSuccess: async (run) => {
      setRunId(run.id);
      setMessage("أُدرج الفحص في طابور العمل.");
      setError("");
      await queryClient.invalidateQueries({ queryKey: ["contradiction-findings"] });
    },
    onError: (value) => setError(contradictionError(value)),
  });
  const reviewMutation = useMutation({
    mutationFn: ({ finding, input }: { finding: ContradictionFinding; input: Parameters<typeof reviewContradictionFinding>[1] }) => reviewContradictionFinding(finding.id, input),
    onSuccess: async (finding) => {
      setMessage(finding.questionId ? "حُفظت الملاحظة وفُتح لها سؤال." : "حُفظت مراجعة الملاحظة.");
      await queryClient.invalidateQueries({ queryKey: ["contradiction-findings"] });
    },
    onError: (value) => setError(contradictionError(value)),
  });
  const findings = findingsQuery.data ?? [];
  const run = runQuery.data;

  return <div className="page-stack"><TopBar eyebrow="مكتب المراجعة" title="افحص التعارضات قبل أن تتحول إلى يقين" description="الفحص ينشئ ملاحظات قابلة للمراجعة، ولا يغيّر الادعاءات أو المصادر تلقائياً." /><section className="contradiction-toolbar"><div><ShieldQuestion size={18} /><span>الفحص يعمل في الخلفية عبر طابور PostgreSQL.</span></div><button className="primary-button" type="button" onClick={() => startMutation.mutate()} disabled={startMutation.isPending}>{startMutation.isPending ? <LoaderCircle className="spin" size={15} /> : <RefreshCw size={15} />} {startMutation.isPending ? "جارٍ الجدولة…" : "فحص التعارضات"}</button></section>{message ? <div className="entity-resolution-message" role="status">{message}</div> : null}{error ? <div className="entity-resolution-error" role="alert">{error}</div> : null}{run ? <div className="contradiction-run-status"><span>آخر تشغيل: {run.status}</span><span>{run.findingCount} ملاحظة</span>{run.error ? <span>{run.error}</span> : null}</div> : null}<SectionHeading eyebrow="ملاحظات النظام" title="نتائج الفحص" description="افحص الإشارات والادعاءات المرتبطة قبل الرفض أو التأكيد." />{findingsQuery.isPending ? <p className="entity-resolution-empty">جارٍ تحميل الملاحظات…</p> : findingsQuery.error ? <div className="entity-resolution-error">{contradictionError(findingsQuery.error)}</div> : findings.length === 0 ? <p className="entity-resolution-empty">لا توجد ملاحظات بانتظار المراجعة.</p> : <div className="contradiction-finding-list">{findings.map((finding) => <FindingCard key={finding.id} finding={finding} pending={reviewMutation.isPending} onReview={(input) => reviewMutation.mutate({ finding, input })} />)}</div>}</div>;
}

function FindingCard({ finding, pending, onReview }: { finding: ContradictionFinding; pending: boolean; onReview: (input: { decision: "dismiss" | "confirm" | "investigate" | "reopen"; note_ar?: string; create_question?: boolean; question_title_ar?: string }) => void }) {
  const [note, setNote] = useState("");
  const [question, setQuestion] = useState(false);
  const [questionTitle, setQuestionTitle] = useState("");
  return <article className="contradiction-finding-card"><div className="contradiction-finding-head"><div><StatusBadge tone={finding.severity === "high" ? "disputed" : "finding"}>{severityLabel(finding.severity)}</StatusBadge><strong>{finding.titleAr}</strong></div><span>{finding.findingType}</span></div><p>{finding.explanationAr}</p><div className="contradiction-signal-box"><strong>إشارات الفحص</strong><pre>{JSON.stringify(finding.signals, null, 2)}</pre></div><div className="contradiction-links"><span>ادعاءات: {finding.claimIds.length ? finding.claimIds.join("، ") : "لا توجد"}</span><span>كيانات: {finding.entityIds.length ? finding.entityIds.join("، ") : "لا توجد"}</span></div><div className="contradiction-review-form"><input value={note} onChange={(event) => setNote(event.target.value)} placeholder="ملاحظة المراجعة (اختياري)" /><label><input type="checkbox" checked={question} onChange={(event) => setQuestion(event.target.checked)} /> فتح سؤال</label>{question ? <input value={questionTitle} onChange={(event) => setQuestionTitle(event.target.value)} placeholder="عنوان السؤال" /> : null}<div className="contradiction-actions"><button className="secondary-button" type="button" disabled={pending} onClick={() => onReview({ decision: "dismiss", note_ar: note || undefined })}>رفض</button><button className="secondary-button" type="button" disabled={pending} onClick={() => onReview({ decision: "investigate", note_ar: note || undefined, create_question: question, question_title_ar: questionTitle || undefined })}>تحقيق</button><button className="primary-button" type="button" disabled={pending} onClick={() => onReview({ decision: "confirm", note_ar: note || undefined })}>تأكيد</button></div></div></article>;
}

function severityLabel(value: ContradictionFinding["severity"]): string {
  if (value === "high") return "خطورة عالية";
  if (value === "medium") return "خطورة متوسطة";
  return "خطورة منخفضة";
}

function contradictionError(value: unknown): string {
  if (value instanceof ApiError) {
    if (value.status === 401) return "سجّل الدخول لمراجعة ملاحظات النظام.";
    if (value.status === 403) return "تحتاج إلى صلاحية researcher أو moderator.";
    return value.message;
  }
  return value instanceof Error ? value.message : "تعذر إكمال فحص التعارضات.";
}
