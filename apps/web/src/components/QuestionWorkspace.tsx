import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleHelp, Clock3, FileText, Link2, Plus, Save, ShieldAlert } from "lucide-react";
import { FormEvent, useEffect, useState } from "react";
import { addQuestionNote, ApiError, createDispute, createQuestion, fetchDisputes, fetchQuestion, fetchQuestions, linkQuestionClaim, linkQuestionDispute, linkQuestionSource, updateQuestion } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { OpenQuestionRecord, QuestionClaimLink, QuestionDetail, QuestionSourceLink } from "../types";

export function QuestionWorkspace() {
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState("");
  const [questionOpen, setQuestionOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<OpenQuestionRecord["priority"]>("normal");
  const [status, setStatus] = useState<OpenQuestionRecord["status"]>("open");
  const [note, setNote] = useState("");
  const [claimId, setClaimId] = useState("");
  const [claimRole, setClaimRole] = useState<QuestionClaimLink["role"]>("concerns");
  const [sourceId, setSourceId] = useState("");
  const [sourceRole, setSourceRole] = useState<QuestionSourceLink["role"]>("context");
  const [disputeId, setDisputeId] = useState("");
  const [disputeTitle, setDisputeTitle] = useState("");
  const [disputeDescription, setDisputeDescription] = useState("");
  const [message, setMessage] = useState("");
  const questionsQuery = useQuery({ queryKey: ["questions"], queryFn: fetchQuestions });
  const detailQuery = useQuery({ queryKey: ["question", selectedId], queryFn: () => fetchQuestion(selectedId), enabled: Boolean(selectedId) });
  const disputesQuery = useQuery({ queryKey: ["disputes"], queryFn: fetchDisputes });
  const refresh = async (id = selectedId) => {
    await queryClient.invalidateQueries({ queryKey: ["questions"] });
    if (id) {
      await queryClient.invalidateQueries({ queryKey: ["question", id] });
    }
  };
  const createQuestionMutation = useMutation({
    mutationFn: () => createQuestion({ title_ar: title, description_ar: description || undefined, priority }),
    onSuccess: async (created) => {
      setSelectedId(created.question.id);
      setQuestionOpen(false);
      setTitle("");
      setDescription("");
      setMessage("فُتح السؤال، وبقيت الحالات والأدلة قابلة للربط.");
      await refresh(created.question.id);
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const updateQuestionMutation = useMutation({
    mutationFn: () => updateQuestion(selectedId, { status, priority }),
    onSuccess: async (updated) => {
      queryClient.setQueryData(["question", selectedId], updated);
      setMessage("حُفظت حالة السؤال، وبقيت Transitions قابلة للتتبع.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const noteMutation = useMutation({
    mutationFn: () => addQuestionNote(selectedId, { note_ar: note }),
    onSuccess: async (updated) => {
      setNote("");
      queryClient.setQueryData(["question", selectedId], updated);
      setMessage("أُضيفت ملاحظة إلى سجل السؤال.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const claimMutation = useMutation({
    mutationFn: () => linkQuestionClaim(selectedId, { claim_id: claimId, role: claimRole }),
    onSuccess: async (updated) => {
      setClaimId("");
      queryClient.setQueryData(["question", selectedId], updated);
      setMessage("رُبط ادعاء بالسؤال.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const sourceMutation = useMutation({
    mutationFn: () => linkQuestionSource(selectedId, { source_id: sourceId, role: sourceRole }),
    onSuccess: async (updated) => {
      setSourceId("");
      queryClient.setQueryData(["question", selectedId], updated);
      setMessage("رُبط مصدر بالسؤال كسياق.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const disputeMutation = useMutation({
    mutationFn: () => linkQuestionDispute(selectedId, { dispute_id: disputeId }),
    onSuccess: async (updated) => {
      setDisputeId("");
      queryClient.setQueryData(["question", selectedId], updated);
      setMessage("رُبط الخلاف بالسؤال.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const createDisputeMutation = useMutation({
    mutationFn: () => createDispute({ title_ar: disputeTitle, description_ar: disputeDescription || undefined }),
    onSuccess: async () => {
      setDisputeTitle("");
      setDisputeDescription("");
      setMessage("أُنشئ خلاف مستقل ويمكن ربطه بالسؤال.");
      await queryClient.invalidateQueries({ queryKey: ["disputes"] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const detail = detailQuery.data;
  useEffect(() => {
    if (detail) {
      setStatus(detail.question.status);
      setPriority(detail.question.priority);
    }
  }, [detail]);
  const queryError = questionsQuery.error ?? detailQuery.error;
  const submitQuestion = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); createQuestionMutation.mutate(); };
  const submitStatus = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); updateQuestionMutation.mutate(); };
  const submitNote = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); noteMutation.mutate(); };
  const submitClaim = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); claimMutation.mutate(); };
  const submitSource = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); sourceMutation.mutate(); };
  const submitDispute = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); disputeMutation.mutate(); };
  const submitNewDispute = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); createDisputeMutation.mutate(); };

  return (
    <section className="question-workspace" id="question-workspace">
      <div className="question-workspace-head"><div><div className="eyebrow">مساحة السؤال المفتوح</div><h2>احتفظ بالخلاف، ثم حدّد الخطوة التالية</h2><p>السؤال لا يغلق باب البحث؛ سجل الحالة، الادعاءات، المصادر، والخلاف الذي يشرح لماذا لم نحسم.</p></div><button className="primary-button" type="button" onClick={() => setQuestionOpen((open) => !open)}><Plus size={15} /> افتح سؤالاً</button></div>
      {message ? <div className="question-workspace-message" role="status">{message}</div> : null}
      {queryError ? <div className="question-workspace-error" role="alert">{errorMessage(queryError)} {queryError instanceof ApiError && queryError.status === 503 ? <span>شغّل Core API وVITE_API_URL لتفعيل الإدارة.</span> : null}</div> : null}
      {questionOpen ? <form className="question-create-form" onSubmit={submitQuestion}><div className="question-form-title"><CircleHelp size={16} /><strong>صياغة سؤال قابل للبحث</strong></div><div className="question-form-grid"><label className="composer-label">العنوان<input required value={title} onChange={(event) => setTitle(event.target.value)} placeholder="مثال: هل Depends على مصدر آخر؟" /></label><label className="composer-label">الأولوية<select value={priority} onChange={(event) => setPriority(event.target.value as typeof priority)}><option value="low">منخفضة</option><option value="normal">عادية</option><option value="high">عالية</option></select></label><label className="composer-label">الوصف<textarea value={description} onChange={(event) => setDescription(event.target.value)} rows={2} placeholder="ما الفجوة أو التعارض؟" /></label></div><button className="primary-button" type="submit" disabled={createQuestionMutation.isPending}>{createQuestionMutation.isPending ? "جارٍ الحفظ…" : "احفظ السؤال"}</button></form> : null}
      <div className="question-workspace-grid"><div className="question-list-column"><div className="question-column-heading"><div><div className="detail-label">الأسئلة المحفوظة</div><small>{(questionsQuery.data ?? []).length} أسئلة</small></div><CircleHelp size={16} /></div>{questionsQuery.isPending ? <p className="question-empty">جارٍ التحميل…</p> : (questionsQuery.data ?? []).length > 0 ? <div className="question-api-list">{(questionsQuery.data ?? []).map((item) => <button className={`question-api-item${selectedId === item.id ? " question-api-item-active" : ""}`} type="button" key={item.id} onClick={() => setSelectedId(item.id)}><span><strong>{item.titleAr}</strong><small>{questionStatusLabel(item.status)} · {questionPriorityLabel(item.priority)}</small></span><b>{item.claimCount}</b></button>)}</div> : <p className="question-empty">لا توجد أسئلة محفوظة بعد.</p>}</div><div className="question-detail-column">{detail ? <QuestionDetailView detail={detail} status={status} setStatus={setStatus} priority={priority} setPriority={setPriority} note={note} setNote={setNote} claimId={claimId} setClaimId={setClaimId} claimRole={claimRole} setClaimRole={setClaimRole} sourceId={sourceId} setSourceId={setSourceId} sourceRole={sourceRole} setSourceRole={setSourceRole} disputeId={disputeId} setDisputeId={setDisputeId} disputes={disputesQuery.data ?? []} onStatus={submitStatus} onNote={submitNote} onClaim={submitClaim} onSource={submitSource} onDispute={submitDispute} statusPending={updateQuestionMutation.isPending} notePending={noteMutation.isPending} /> : detailQuery.isPending ? <p className="question-empty">جارٍ فتح السؤال…</p> : <div className="question-empty"><CircleHelp size={19} /><p>اختر سؤالاً أو افتح سؤالاً جديداً.</p></div>}</div></div>
      <form className="question-dispute-form" onSubmit={submitNewDispute}><div className="question-form-title"><ShieldAlert size={16} /><strong>خلاف مستقل</strong><small>سجّل الروايات المتعارضة قبل ربطها بسؤال.</small></div><div className="question-form-grid"><label className="composer-label">عنوان الخلاف<input required value={disputeTitle} onChange={(event) => setDisputeTitle(event.target.value)} placeholder="اسم الخلاف" /></label><label className="composer-label">الوصف<input value={disputeDescription} onChange={(event) => setDisputeDescription(event.target.value)} placeholder="ما الروايتان؟" /></label><button className="secondary-button" type="submit" disabled={createDisputeMutation.isPending}>{createDisputeMutation.isPending ? "جارٍ الإنشاء…" : "أنشئ خلافاً"}</button></div></form>
    </section>
  );
}

function QuestionDetailView({ detail, status, setStatus, priority, setPriority, note, setNote, claimId, setClaimId, claimRole, setClaimRole, sourceId, setSourceId, sourceRole, setSourceRole, disputeId, setDisputeId, disputes, onStatus, onNote, onClaim, onSource, onDispute, statusPending, notePending }: { detail: QuestionDetail; status: OpenQuestionRecord["status"]; setStatus: (value: OpenQuestionRecord["status"]) => void; priority: OpenQuestionRecord["priority"]; setPriority: (value: OpenQuestionRecord["priority"]) => void; note: string; setNote: (value: string) => void; claimId: string; setClaimId: (value: string) => void; claimRole: QuestionClaimLink["role"]; setClaimRole: (value: QuestionClaimLink["role"]) => void; sourceId: string; setSourceId: (value: string) => void; sourceRole: QuestionSourceLink["role"]; setSourceRole: (value: QuestionSourceLink["role"]) => void; disputeId: string; setDisputeId: (value: string) => void; disputes: Array<{ id: string; titleAr: string }>; onStatus: (event: FormEvent<HTMLFormElement>) => void; onNote: (event: FormEvent<HTMLFormElement>) => void; onClaim: (event: FormEvent<HTMLFormElement>) => void; onSource: (event: FormEvent<HTMLFormElement>) => void; onDispute: (event: FormEvent<HTMLFormElement>) => void; statusPending: boolean; notePending: boolean }) {
  return <div className="question-detail"><div className="question-detail-head"><div><div className="eyebrow">السؤال المحدد</div><h3>{detail.question.titleAr}</h3><p>{detail.question.descriptionAr || "دون وصف"}</p></div><StatusBadge tone={detail.question.status === "resolved" ? "source" : "question"}>{questionStatusLabel(detail.question.status)}</StatusBadge></div><form className="question-status-form" onSubmit={onStatus}><label className="composer-label">الحالة<select value={status} onChange={(event) => setStatus(event.target.value as typeof status)}><option value="open">مفتوح</option><option value="under_investigation">قيد التحقيق</option><option value="resolved">محسوم مؤقتاً</option><option value="reopened">أُعيد فتحه</option><option value="archived">مؤرشف</option></select></label><label className="composer-label">الأولوية<select value={priority} onChange={(event) => setPriority(event.target.value as typeof priority)}><option value="low">منخفضة</option><option value="normal">عادية</option><option value="high">عالية</option></select></label><button className="secondary-button" type="submit" disabled={statusPending}><Save size={14} /> حفظ الحالة</button></form><div className="question-link-grid"><form onSubmit={onClaim}><label className="composer-label">معرف ادعاء<input required value={claimId} onChange={(event) => setClaimId(event.target.value)} placeholder="claim UUID" /><select value={claimRole} onChange={(event) => setClaimRole(event.target.value as typeof claimRole)}><option value="concerns">يطرح السؤال</option><option value="supports">يدعم الحل</option><option value="opposes">يعارض الحل</option></select></label><button className="icon-button" type="submit" aria-label="ربط ادعاء"><Link2 size={14} /></button></form><form onSubmit={onSource}><label className="composer-label">معرف مصدر<input required value={sourceId} onChange={(event) => setSourceId(event.target.value)} placeholder="source UUID" /><select value={sourceRole} onChange={(event) => setSourceRole(event.target.value as typeof sourceRole)}><option value="context">سياق</option><option value="supporting">داعم</option><option value="counter_evidence">دليل مضاد</option><option value="missing">مفقود</option></select></label><button className="icon-button" type="submit" aria-label="ربط مصدر"><FileText size={14} /></button></form><form onSubmit={onDispute}><label className="composer-label">خلاف مرتبط<select required value={disputeId} onChange={(event) => setDisputeId(event.target.value)}><option value="">اختر خلافاً</option>{disputes.map((item) => <option key={item.id} value={item.id}>{item.titleAr}</option>)}</select></label><button className="icon-button" type="submit" aria-label="ربط خلاف"><Link2 size={14} /></button></form></div><div className="question-linked-summary"><div><span>ادعاءات</span><strong>{detail.claims.length}</strong></div><div><span>مصادر</span><strong>{detail.sources.length}</strong></div><div><span>خلافات</span><strong>{detail.disputes.length}</strong></div></div><div className="question-linked-list">{detail.claims.map((item) => <div key={item.claimId}><span>ادعاء</span><strong>{item.predicate}</strong><small>{item.role} · {item.status}</small></div>)}{detail.sources.map((item) => <div key={item.sourceId}><span>مصدر</span><strong>{item.titleAr}</strong><small>{item.role}</small></div>)}{detail.disputes.map((item) => <div key={item.disputeId}><span>خلاف</span><strong>{item.titleAr}</strong><small>{questionDisputeStatusLabel(item.status)}</small></div>)}</div><form className="question-note-form" onSubmit={onNote}><label className="composer-label">ملاحظة بحثية<input required value={note} onChange={(event) => setNote(event.target.value)} placeholder="ما الخطوة التالية أو ما ينقص؟" /></label><button className="secondary-button" type="submit" disabled={notePending}>{notePending ? "جارٍ الحفظ…" : "أضف ملاحظة"}</button></form><div className="question-note-list">{detail.notes.map((item) => <div key={item.id}><p>{item.noteAr}</p><small>{new Date(item.createdAt).toLocaleDateString("ar")}</small></div>)}</div><div className="question-activity"><div className="question-column-heading"><div><div className="detail-label">نشاط السؤال</div><small>سجل محفوظ للحالات والروابط</small></div><Clock3 size={15} /></div><div className="question-activity-list">{detail.activity.length > 0 ? detail.activity.map((item) => <div key={`${item.entityId}-${item.action}-${item.createdAt}`}><span>{activityLabel(item.action)}</span><small>{new Date(item.createdAt).toLocaleString("ar")}</small></div>) : <p className="question-empty">لا يوجد نشاط بعد.</p>}</div></div></div>;
}

function questionStatusLabel(status: string): string {
  if (status === "under_investigation") return "قيد التحقيق";
  if (status === "resolved") return "محسوم مؤقتاً";
  if (status === "reopened") return "أُعيد فتحه";
  if (status === "archived") return "مؤرشف";
  return "مفتوح";
}

function questionPriorityLabel(priority: string): string {
  if (priority === "high") return "عالية";
  if (priority === "low") return "منخفضة";
  return "عادية";
}

function questionDisputeStatusLabel(status: string): string {
  if (status === "under_review") return "قيد المراجعة";
  if (status === "resolved") return "محسوم";
  return "مفتوح";
}

function activityLabel(action: string): string {
  const labels: Record<string, string> = {
    question_created: "فُتح السؤال",
    question_updated: "تغيرت حالة السؤال",
    question_note_added: "أُضيفت ملاحظة",
    question_claim_linked: "رُبط ادعاء",
    question_source_linked: "رُبط مصدر",
    question_dispute_linked: "رُبط خلاف",
    dispute_created: "أُنشئ خلاف",
    dispute_updated: "تغيرت حالة الخلاف",
    dispute_claim_linked: "رُبط ادعاء بالخلاف",
  };
  return labels[action] ?? action;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر إكمال عملية السؤال.";
}
