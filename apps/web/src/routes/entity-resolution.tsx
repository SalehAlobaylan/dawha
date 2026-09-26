import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { GitMerge, LoaderCircle, RefreshCw, ShieldCheck, Undo2 } from "lucide-react";
import { useEffect, useState } from "react";
import { ApiError, fetchEntityResolutionCandidates, fetchEntityResolutionMerges, fetchEntityResolutionRun, mergeEntityResolutionCandidate, reverseEntityResolutionMerge, reviewEntityResolutionCandidate, runEntityResolution } from "../lib/api";
import type { EntityResolutionCandidate, EntityResolutionEntityType, EntityResolutionMerge } from "../types";
import { SectionHeading } from "../components/SectionHeading";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

export function EntityResolutionPage() {
  const queryClient = useQueryClient();
  const [entityType, setEntityType] = useState<EntityResolutionEntityType>("person");
  const [status, setStatus] = useState("pending");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [runningId, setRunningId] = useState("");
  // A scan is queued work, not a request that blocks. The button accepts a run and
  // the page watches it from here, so the browser is never the thing holding a scan
  // open - and closing the tab does not lose the run, because the run is a row the
  // API and the worker both know about.
  const runQuery = useQuery({
    queryKey: ["entity-resolution-run", runningId],
    queryFn: () => fetchEntityResolutionRun(runningId),
    enabled: runningId !== "",
    refetchInterval: (query) => {
      const run = query.state.data;
      return run?.status === "queued" || run?.status === "running" ? 1500 : false;
    },
  });
  useEffect(() => {
    const run = runQuery.data;
    if (!run) return;
    if (run.status === "succeeded") {
      setMessage(`اكتمل الفحص ${run.algorithmVersion} ووجد ${run.candidateCount} مرشحاً.`);
      setRunningId("");
      void queryClient.invalidateQueries({ queryKey: ["entity-resolution-candidates"] });
    } else if (run.status === "failed") {
      setError(run.error || "تعذر إكمال فحص مطابقة الهوية.");
      setRunningId("");
    }
  }, [runQuery.data, queryClient]);
  const scanPending = runQuery.isFetching || runQuery.data?.status === "queued" || runQuery.data?.status === "running";
  const candidatesQuery = useQuery({
    queryKey: ["entity-resolution-candidates", status, entityType],
    queryFn: () => fetchEntityResolutionCandidates(status === "all" ? undefined : status, entityType === "all" ? undefined : entityType),
  });
  const mergesQuery = useQuery({ queryKey: ["entity-resolution-merges"], queryFn: fetchEntityResolutionMerges });
  const runMutation = useMutation({
    mutationFn: () => runEntityResolution(entityType),
    onSuccess: (run) => {
      setError("");
      setRunningId(run.id);
      setMessage("أُدرج الفحص في طابور العمل، والنتيجة تظهر هنا عند انتهائه.");
    },
    onError: (value) => setError(entityResolutionError(value)),
  });
  const reviewMutation = useMutation({
    mutationFn: ({ candidate, decision }: { candidate: EntityResolutionCandidate; decision: "approve" | "reject" | "defer" }) => reviewEntityResolutionCandidate(candidate.id, { decision, expected_version: candidate.candidateVersion }),
    onSuccess: async () => {
      setMessage("حُفظ قرار المراجعة في سجل المرشح.");
      await queryClient.invalidateQueries({ queryKey: ["entity-resolution-candidates"] });
    },
    onError: (value) => setError(entityResolutionError(value)),
  });
  const mergeMutation = useMutation({
    mutationFn: ({ candidate, input }: { candidate: EntityResolutionCandidate; input: { survivor_entity_id: string; reason_ar: string; expected_candidate_version: number; confirm: boolean } }) => mergeEntityResolutionCandidate(candidate.id, input),
    onSuccess: async () => {
      setMessage("سُجلت عملية الدمج القابلة للعكس.");
      await queryClient.invalidateQueries({ queryKey: ["entity-resolution-candidates"] });
      await queryClient.invalidateQueries({ queryKey: ["entity-resolution-merges"] });
    },
    onError: (value) => setError(entityResolutionError(value)),
  });
  const reverseMutation = useMutation({
    mutationFn: ({ merge, reason }: { merge: EntityResolutionMerge; reason: string }) => reverseEntityResolutionMerge(merge.id, reason),
    onSuccess: async () => {
      setMessage("أُعيد السجل إلى وضعه السابق.");
      await queryClient.invalidateQueries({ queryKey: ["entity-resolution-merges"] });
      await queryClient.invalidateQueries({ queryKey: ["entity-resolution-candidates"] });
    },
    onError: (value) => setError(entityResolutionError(value)),
  });
  const candidates = candidatesQuery.data ?? [];
  const merges = mergesQuery.data ?? [];

  return (
    <div className="page-stack">
      <TopBar eyebrow="مكتب الهوية / مطابقة الكيانات" title="راجع التشابه قبل أن تدمج" description="النظام يجمع إشارات الاسم والقرابة والموضع والزمن، لكنه لا يدمج سجلين من تلقاء نفسه." />
      <section className="entity-resolution-toolbar">
        <div className="entity-resolution-toolbar-copy"><ShieldCheck size={18} /><span>قرار الدمج يتطلب موافقة بشرية، وسجلاً قابلاً للعكس.</span></div>
        <label className="composer-label">نوع الكيان<select value={entityType} onChange={(event) => setEntityType(event.target.value as EntityResolutionEntityType)}><option value="person">الأشخاص</option><option value="family">العائلات</option><option value="all">الكل</option></select></label>
        <button className="primary-button" type="button" onClick={() => runMutation.mutate()} disabled={runMutation.isPending || scanPending}>{runMutation.isPending || scanPending ? <LoaderCircle className="spin" size={15} /> : <RefreshCw size={15} />} {runMutation.isPending || scanPending ? "جارٍ الفحص…" : "فحص المرشحات"}</button>
      </section>
      {message ? <div className="entity-resolution-message" role="status">{message}</div> : null}
      {error ? <div className="entity-resolution-error" role="alert">{error}</div> : null}
      <div className="entity-resolution-layout">
        <main>
          <SectionHeading eyebrow="طابور المراجعة" title="المرشحون المحتملون" description="اقرأ الإشارات المتطابقة والمتناقضة معاً قبل اتخاذ قرار." />
          <div className="entity-resolution-filter"><span>الحالة</span><select value={status} onChange={(event) => setStatus(event.target.value)}><option value="pending">بانتظار المراجعة</option><option value="approved">مقبول</option><option value="rejected">مرفوض</option><option value="deferred">مؤجل</option><option value="reopened">أُعيد فتحه</option><option value="merged">مدمج</option><option value="all">الكل</option></select></div>
          {candidatesQuery.isPending ? <p className="entity-resolution-empty">جارٍ تحميل المرشحين…</p> : candidatesQuery.error ? <div className="entity-resolution-error">{entityResolutionError(candidatesQuery.error)}</div> : candidates.length === 0 ? <p className="entity-resolution-empty">لا يوجد مرشحون في هذه الحالة.</p> : <div className="entity-resolution-candidate-list">{candidates.map((candidate) => <CandidateCard key={candidate.id} candidate={candidate} pending={reviewMutation.isPending || mergeMutation.isPending} onReview={(decision) => reviewMutation.mutate({ candidate, decision })} onMerge={(input) => mergeMutation.mutate({ candidate, input })} />)}</div>}
        </main>
        <aside className="entity-resolution-side">
          <SectionHeading eyebrow="سجل الدمج" title="عمليات قابلة للعكس" description="لا تحذف السجلات الأصلية." />
          {mergesQuery.isPending ? <p className="entity-resolution-empty">جارٍ فتح السجل…</p> : mergesQuery.error ? <div className="entity-resolution-error">{entityResolutionError(mergesQuery.error)}</div> : merges.length === 0 ? <p className="entity-resolution-empty">لم تُسجل عمليات دمج بعد.</p> : <div className="entity-resolution-merge-list">{merges.map((merge) => <MergeRow key={merge.id} merge={merge} pending={reverseMutation.isPending} onReverse={() => { const reason = window.prompt("سبب عكس الدمج؟"); if (reason?.trim()) reverseMutation.mutate({ merge, reason: reason.trim() }); }} />)}</div>}
        </aside>
      </div>
    </div>
  );
}

function CandidateCard({ candidate, pending, onReview, onMerge }: { candidate: EntityResolutionCandidate; pending: boolean; onReview: (decision: "approve" | "reject" | "defer") => void; onMerge: (input: { survivor_entity_id: string; reason_ar: string; expected_candidate_version: number; confirm: boolean }) => void }) {
  const [survivor, setSurvivor] = useState(candidate.leftEntityId);
  const [reason, setReason] = useState("");
  const [confirm, setConfirm] = useState(false);
  const canReview = candidate.reviewStatus === "pending" || candidate.reviewStatus === "deferred" || candidate.reviewStatus === "reopened";
  return <article className="entity-resolution-card"><div className="entity-resolution-card-head"><div><StatusBadge tone={candidate.matchClass === "strong_candidate" ? "source" : candidate.matchClass === "possible_match" ? "question" : "disputed"}>{matchLabel(candidate.matchClass)}</StatusBadge><strong>{candidate.leftNameAr}</strong><span>مقابل</span><strong>{candidate.rightNameAr}</strong></div><span className="entity-resolution-score">{Math.round(candidate.score * 100)}٪</span></div><p className="entity-resolution-explanation">{candidate.explanationAr}</p><div className="entity-resolution-signals"><SignalList title="مؤشرات متطابقة" signals={candidate.matchingSignals} tone="match" /><SignalList title="مؤشرات متعارضة" signals={candidate.conflictingSignals} tone="conflict" /></div><div className="entity-resolution-components">{Object.entries(candidate.scoreComponents).map(([name, value]) => <span key={name}>{componentLabel(name)}: {Math.round(value * 100)}٪</span>)}</div>{canReview ? <div className="entity-resolution-actions"><button className="primary-button" type="button" disabled={pending} onClick={() => onReview("approve")}><ShieldCheck size={14} /> اعتماد</button><button className="secondary-button" type="button" disabled={pending} onClick={() => onReview("defer")}>تأجيل</button><button className="danger-button" type="button" disabled={pending} onClick={() => onReview("reject")}>رفض</button></div> : null}{candidate.reviewStatus === "approved" ? <div className="entity-resolution-merge-form"><div className="entity-resolution-merge-heading"><GitMerge size={15} /><strong>دمج بعد موافقة صريحة</strong></div><label className="composer-label">السجل الأساسي<select value={survivor} onChange={(event) => setSurvivor(event.target.value)}><option value={candidate.leftEntityId}>{candidate.leftNameAr}</option><option value={candidate.rightEntityId}>{candidate.rightNameAr}</option></select></label><label className="composer-label">سبب الدمج<input value={reason} onChange={(event) => setReason(event.target.value)} placeholder="مثال: تطابق الاسم والقرابة وال史料" /></label><label className="entity-resolution-confirm"><input type="checkbox" checked={confirm} onChange={(event) => setConfirm(event.target.checked)} /> أفهم أن الدمج عملية مؤثرة وقابلة للعكس</label><button className="primary-button" type="button" disabled={pending || !reason.trim() || !confirm} onClick={() => onMerge({ survivor_entity_id: survivor, reason_ar: reason, expected_candidate_version: candidate.candidateVersion, confirm })}>تنفيذ الدمج</button></div> : null}</article>;
}

function SignalList({ title, signals, tone }: { title: string; signals: EntityResolutionCandidate["matchingSignals"]; tone: "match" | "conflict" }) {
  return <div className={`entity-resolution-signal-list entity-resolution-signal-${tone}`}><strong>{title}</strong>{signals.length === 0 ? <span>لا توجد إشارة مسجلة.</span> : signals.map((signal, index) => <span key={`${signal.kind}-${index}`}>{signal.detail}</span>)}</div>;
}

function MergeRow({ merge, pending, onReverse }: { merge: EntityResolutionMerge; pending: boolean; onReverse: () => void }) {
  return <article className="entity-resolution-merge-row"><div><StatusBadge tone={merge.state === "applied" ? "disputed" : "source"}>{merge.state === "applied" ? "مفعّل" : "معكوس"}</StatusBadge><strong>{merge.survivorId} ← {merge.mergedId}</strong></div><small>{new Date(merge.appliedAt).toLocaleString("ar")}</small>{merge.state === "applied" ? <button className="secondary-button" type="button" disabled={pending} onClick={onReverse}><Undo2 size={14} /> عكس</button> : null}</article>;
}

function matchLabel(value: EntityResolutionCandidate["matchClass"]): string {
  if (value === "strong_candidate") return "مرشح قوي";
  if (value === "possible_match") return "تطابق محتمل";
  return "على الأرجح مختلف";
}

function componentLabel(value: string): string {
  const labels: Record<string, string> = { name: "الاسم", embedding: "التمثيل", relationship: "القرابة", geography: "الموضع", chronology: "الزمن", source_context: "المصادر" };
  return labels[value] ?? value;
}

function entityResolutionError(value: unknown): string {
  if (value instanceof ApiError) {
    if (value.status === 401) return "سجّل الدخول للوصول إلى طابور مطابقة الهوية.";
    if (value.status === 403) return "تحتاج إلى صلاحية researcher أو moderator لهذه العملية.";
    return value.message;
  }
  return value instanceof Error ? value.message : "تعذر إكمال عملية مطابقة الهوية.";
}
