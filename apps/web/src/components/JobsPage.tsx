import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Clock3, Database, Play, RefreshCw, ServerCog, XCircle } from "lucide-react";
import { FormEvent, useState } from "react";
import { claimJob, completeJob, enqueueJob, failJob, fetchJobs, recoverStaleJobs } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import { TopBar } from "./TopBar";
import type { JobView } from "../types";

export function JobsPage() {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState("");
  const [workerId, setWorkerId] = useState("manual-worker");
  const [type, setType] = useState("source_process");
  const [payload, setPayload] = useState('{"source_id":""}');
  const [priority, setPriority] = useState("0");
  const [maxAttempts, setMaxAttempts] = useState("3");
  const [idempotencyKey, setIdempotencyKey] = useState("");
  const [failure, setFailure] = useState("");
  const [message, setMessage] = useState("");
  const jobsQuery = useQuery({ queryKey: ["jobs", status], queryFn: () => fetchJobs(status || undefined) });
  const enqueueMutation = useMutation({
    mutationFn: () => enqueueJob({ type, payload: JSON.parse(payload), priority: Number(priority) || 0, max_attempts: Number(maxAttempts) || 3, idempotency_key: idempotencyKey || undefined }),
    onSuccess: async (result) => { setMessage(result.created ? "أُضيفت المهمة إلى الطابور." : "أُعيد استخدام المهمة نفسها عبر مفتاح التكرار."); setIdempotencyKey(""); await queryClient.invalidateQueries({ queryKey: ["jobs"] }); },
    onError: (error) => setMessage(jobError(error)),
  });
  const claimMutation = useMutation({
    mutationFn: () => claimJob(workerId),
    onSuccess: async (job) => { setMessage(`التقط العامل ${workerId} المهمة ${job.type}.`); await queryClient.invalidateQueries({ queryKey: ["jobs"] }); },
    onError: (error) => setMessage(jobError(error)),
  });
  const completeMutation = useMutation({
    mutationFn: (jobId: string) => completeJob(jobId, workerId),
    onSuccess: async () => { setMessage("اكتملت المهمة وأُغلق قفلها."); await queryClient.invalidateQueries({ queryKey: ["jobs"] }); },
    onError: (error) => setMessage(jobError(error)),
  });
  const failMutation = useMutation({
    mutationFn: (jobId: string) => failJob(jobId, workerId, failure || "failure reported by worker"),
    onSuccess: async () => { setMessage("سُجل الفشل؛ حُدد موعد المحاولة التالية أو انتقل إلى dead."); setFailure(""); await queryClient.invalidateQueries({ queryKey: ["jobs"] }); },
    onError: (error) => setMessage(jobError(error)),
  });
  const recoverMutation = useMutation({
    mutationFn: () => recoverStaleJobs(),
    onSuccess: async (result) => { setMessage(`استُعيدت ${result.recovered} مهمة عالقة.`); await queryClient.invalidateQueries({ queryKey: ["jobs"] }); },
    onError: (error) => setMessage(jobError(error)),
  });
  const submit = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); setMessage(""); enqueueMutation.mutate(); };
  const jobs = jobsQuery.data ?? [];

  return <div className="page-stack"><TopBar eyebrow="العمليات / PostgreSQL Jobs" title="طابور العمل، بلا Redis" description="الواجهة لا تعدّل حقائق المنتج مباشرة؛ بل جدولة معالجة قابلة لإعادة المحاولة، مع حفظ الفشل والقفل والاستعادة." /><section className="jobs-command"><div className="jobs-command-icon"><ServerCog size={19} /></div><div><div className="eyebrow">واجهة التشغيل</div><h2>مهام دائمة بلا Redis</h2><p>استخدم إضافة المهمة، والتقاطها، وإكمالها، وتسجيل فشلها، واستعادة القفل العالق عبر جدول jobs نفسه.</p></div><StatusBadge tone="interpretation">FOR UPDATE SKIP LOCKED</StatusBadge></section><div className="jobs-layout"><section className="jobs-enqueue-panel"><div className="jobs-panel-head"><div><div className="eyebrow">إضافة مهمة</div><h3>جدولة معالجة جديدة</h3></div><Play size={16} /></div><form className="jobs-form" onSubmit={submit}><label>النوع<input required value={type} onChange={(event) => setType(event.target.value)} placeholder="source_process" /></label><label>Payload JSON<textarea required value={payload} onChange={(event) => setPayload(event.target.value)} rows={5} /></label><div className="jobs-form-grid"><label>الأولوية<input type="number" min="-100" max="100" value={priority} onChange={(event) => setPriority(event.target.value)} /></label><label>المحاولات<input type="number" min="1" max="10" value={maxAttempts} onChange={(event) => setMaxAttempts(event.target.value)} /></label><label>مفتاح idempotency<input value={idempotencyKey} onChange={(event) => setIdempotencyKey(event.target.value)} placeholder="اختياري" /></label></div><button className="primary-button" type="submit" disabled={enqueueMutation.isPending}>{enqueueMutation.isPending ? "جارٍ الجدولة…" : "أضف إلى الطابور"}</button></form></section><section className="jobs-worker-panel"><div className="jobs-panel-head"><div><div className="eyebrow">عامل يدوي</div><h3>محاكاة worker</h3></div><RefreshCw size={16} /></div><label>معرّف العامل<input value={workerId} onChange={(event) => setWorkerId(event.target.value)} placeholder="worker-1" /></label><div className="jobs-worker-actions"><button className="secondary-button" type="button" onClick={() => claimMutation.mutate()} disabled={claimMutation.isPending}><Play size={14} /> التقط مهمة</button><button className="secondary-button" type="button" onClick={() => recoverMutation.mutate()} disabled={recoverMutation.isPending}><RefreshCw size={14} /> استعد العالقة</button></div><label>رسالة الفشل<input value={failure} onChange={(event) => setFailure(event.target.value)} placeholder="سبب الفشل" /></label><p className="jobs-worker-note">الفشل يستهلك محاولة، ويعيد جدولة المهمة وفق exponential backoff حتى max_attempts.</p></section></div>{message ? <div className="jobs-message" role="status">{message}</div> : null}{jobsQuery.error ? <div className="jobs-message jobs-error" role="alert">{jobError(jobsQuery.error)}</div> : null}<section className="jobs-list-panel"><div className="jobs-list-head"><div><div className="eyebrow">سجل المهام</div><h2>{jobs.length} مهمة</h2></div><select value={status} onChange={(event) => setStatus(event.target.value)} aria-label="تصفية الحالة"><option value="">كل الحالات</option><option value="queued">queued</option><option value="running">running</option><option value="succeeded">succeeded</option><option value="dead">dead</option></select></div><div className="jobs-list">{jobsQuery.isPending ? <p className="jobs-empty">جارٍ فتح الطابور…</p> : jobs.length === 0 ? <p className="jobs-empty">لا توجد مهام.</p> : jobs.map((job) => <JobRow job={job} key={job.id} onComplete={() => completeMutation.mutate(job.id)} onFail={() => failMutation.mutate(job.id)} pending={completeMutation.isPending || failMutation.isPending} />)}</div></section></div>;
}

function JobRow({ job, onComplete, onFail, pending }: { job: JobView; onComplete: () => void; onFail: () => void; pending: boolean }) {
  return <div className="jobs-row"><span className={`jobs-status-icon jobs-status-icon-${job.status}`}>{job.status === "succeeded" ? <CheckCircle2 size={15} /> : job.status === "dead" ? <AlertTriangle size={15} /> : job.status === "running" ? <Clock3 size={15} /> : <Database size={15} />}</span><span className="jobs-row-copy"><strong>{job.type}</strong><small>{job.id} · {job.attempts}/{job.maxAttempts} محاولة</small>{job.lastError ? <p>{job.lastError}</p> : null}</span><span className="jobs-row-status"><StatusBadge tone={job.status === "dead" ? "disputed" : job.status === "succeeded" ? "source" : "claim"}>{job.status}</StatusBadge><small>{new Date(job.updatedAt).toLocaleString("ar")}</small></span>{job.status === "running" ? <span className="jobs-row-actions"><button className="icon-button" type="button" onClick={onComplete} disabled={pending} aria-label="إكمال المهمة"><CheckCircle2 size={14} /></button><button className="icon-button jobs-icon-danger" type="button" onClick={onFail} disabled={pending} aria-label="فشل المهمة"><XCircle size={14} /></button></span> : null}</div>;
}

function jobError(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر تنفيذ عملية المهمة.";
}
