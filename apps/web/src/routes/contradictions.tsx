import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, RefreshCw, ShieldQuestion } from "lucide-react";
import { useEffect, useState } from "react";
import { ApiError, fetchContradictionFindings, fetchContradictionRun, reviewContradictionFinding, startContradictionRun } from "../lib/api";
import type { ContradictionFinding } from "../types";
import { SectionHeading } from "../components/SectionHeading";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

export function ContradictionsPage({ embedded = false }: { embedded?: boolean }) {
  const queryClient = useQueryClient();
  const search = useSearch({ from: "/research" });
  const navigate = useNavigate();
  const runId = search.runId ?? "";
  const [status, setStatus] = useState("needs_review");
  const [questionId, setQuestionId] = useState("");
  const setRunId = (id: string) => { void navigate({ to: "/research", search: { ...search, view: "review", review: "contradictions", runId: id }, replace: true, resetScroll: false }); };
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const findingsQuery = useQuery({ queryKey: ["contradiction-findings", runId, status], queryFn: () => fetchContradictionFindings(runId || undefined, status || undefined) });
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
      setQuestionId(finding.questionId ?? "");
      setError("");
      setMessage(finding.questionId ? "حُفظت الملاحظة وفُتح لها سؤال." : "حُفظت مراجعة الملاحظة.");
      await queryClient.invalidateQueries({ queryKey: ["contradiction-findings"] });
    },
    onError: (value) => setError(contradictionError(value)),
  });
  const findings = findingsQuery.data ?? [];
  const run = runQuery.data;
  useEffect(() => {
    if (run?.status === "succeeded") void queryClient.invalidateQueries({ queryKey: ["contradiction-findings"] });
  }, [run?.id, run?.status, queryClient]);
  const running = Boolean(runId && (runQuery.isPending || run?.status === "queued" || run?.status === "running"));


  return <div className={embedded ? "research-review-tool" : "page-stack"}>{!embedded ? <TopBar eyebrow="مكتب المراجعة" title="افحص التعارضات قبل أن تتحول إلى يقين" description="الفحص ينشئ ملاحظات قابلة للمراجعة، ولا يغيّر الادعاءات أو المصادر تلقائياً." /> : null}<section className="contradiction-toolbar"><div><ShieldQuestion size={18} /><span>نفحص الروايات في الخلفية، وتبقى النتائج بحاجة إلى قرار بشري.</span></div><button className="primary-button" type="button" onClick={() => startMutation.mutate()} disabled={startMutation.isPending || running}>{startMutation.isPending ? <LoaderCircle className="spin" size={15} /> : <RefreshCw size={15} />} {startMutation.isPending ? "جارٍ الجدولة…" : "فحص التعارضات"}</button></section>{questionId ? <Link className="inline-link" to="/research/$questionId" params={{ questionId }}>فتح سؤال التحقيق المحفوظ</Link> : null}{message ? <div className="entity-resolution-message" role="status">{message}</div> : null}{error ? <div className="entity-resolution-error" role="alert">{error}</div> : null}{runQuery.error ? <p role="alert">تعذر تحديث حالة الفحص. <button type="button" onClick={() => void runQuery.refetch()}>إعادة المحاولة</button></p> : null}{run ? <div className="contradiction-run-status"><span>آخر تشغيل: {run.status === "succeeded" ? "اكتمل الفحص" : run.status === "failed" ? "تعذر الفحص" : "جارٍ الفحص"}</span><span>{run.findingCount} ملاحظة</span>{run.error ? <span>{run.error}</span> : null}</div> : null}<SectionHeading eyebrow="ملاحظات النظام" title="نتائج الفحص" description="افحص الإشارات والادعاءات المرتبطة قبل الرفض أو التأكيد." /><label className="composer-label contradiction-status-filter">حالة الملاحظات<select value={status} onChange={(event) => setStatus(event.target.value)}><option value="needs_review">بانتظار المراجعة</option><option value="confirmed">مؤكدة</option><option value="dismissed">مرفوضة</option><option value="investigating">قيد التحقيق</option><option value="">كل الحالات</option></select></label>{findingsQuery.isPending ? <p className="entity-resolution-empty">جارٍ تحميل الملاحظات…</p> : findingsQuery.error ? <div className="entity-resolution-error">{contradictionError(findingsQuery.error)}</div> : findings.length === 0 ? <p className="entity-resolution-empty">لا توجد ملاحظات بانتظار المراجعة.</p> : <div className="contradiction-finding-list">{findings.map((finding) => <FindingCard key={finding.id} finding={finding} pending={reviewMutation.isPending} onReview={(input) => reviewMutation.mutate({ finding, input })} />)}</div>}</div>;
}

function FindingCard({ finding, pending, onReview }: { finding: ContradictionFinding; pending: boolean; onReview: (input: { decision: "dismiss" | "confirm" | "investigate" | "reopen"; note_ar?: string; create_question?: boolean; question_title_ar?: string }) => void }) {
  const [note, setNote] = useState("");
  const [question, setQuestion] = useState(false);
  const [questionTitle, setQuestionTitle] = useState("");
  return <article className="contradiction-finding-card"><div className="contradiction-finding-head"><div><StatusBadge tone={finding.severity === "high" ? "disputed" : "finding"}>{severityLabel(finding.severity)}</StatusBadge><strong>{finding.titleAr}</strong></div><span>{finding.claimIds.length} ادعاءات مرتبطة</span></div><p>{finding.explanationAr}</p><details className="contradiction-signal-box"><summary>تفاصيل إشارات الفحص</summary><pre>{JSON.stringify(finding.signals, null, 2)}</pre></details><div className="contradiction-links"><span>الادعاءات المرتبطة: {finding.claimIds.length ? finding.claimIds.map((id, index) => <Link key={id} className="inline-link" to="/search" search={{ q: finding.titleAr, kind: "claims", claimId: id }}>افتح الادعاء {index + 1}</Link>) : "لا توجد"}</span><span>{finding.entityIds.length} كيانات في سياق الفحص</span></div><div className="contradiction-review-form"><input aria-label="ملاحظة مراجعة التعارض" value={note} onChange={(event) => setNote(event.target.value)} placeholder="ملاحظة المراجعة (اختياري)" /><label><input type="checkbox" checked={question} onChange={(event) => setQuestion(event.target.checked)} /> فتح سؤال</label>{question ? <input value={questionTitle} onChange={(event) => setQuestionTitle(event.target.value)} placeholder="عنوان السؤال" /> : null}<div className="contradiction-actions"><button className="secondary-button" type="button" disabled={pending} onClick={() => onReview({ decision: "dismiss", note_ar: note || undefined })}>رفض</button><button className="secondary-button" type="button" disabled={pending} onClick={() => onReview({ decision: "investigate", note_ar: note || undefined, create_question: question, question_title_ar: questionTitle || undefined })}>تحقيق</button><button className="primary-button" type="button" disabled={pending} onClick={() => onReview({ decision: "confirm", note_ar: note || undefined })}>تأكيد</button></div></div></article>;
}

function severityLabel(value: ContradictionFinding["severity"]): string {
  if (value === "high") return "خطورة عالية";
  if (value === "medium") return "خطورة متوسطة";
  return "خطورة منخفضة";
}

function contradictionError(value: unknown): string {
  if (value instanceof ApiError) {
    if (value.status === 401) return "سجّل الدخول لمراجعة ملاحظات النظام.";
    if (value.status === 403) return "تحتاج هذه المراجعة إلى صلاحية باحث أو مشرف.";
    return value.message;
  }
  return value instanceof Error ? value.message : "تعذر إكمال فحص التعارضات.";
}
