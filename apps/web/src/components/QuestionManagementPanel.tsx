import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { addQuestionNote, fetchQuestion, fetchSources, linkQuestionSource, updateQuestion } from "../lib/api";
import type { OpenQuestionRecord } from "../types";

export function QuestionManagementPanel({ questionId, canManage }: { questionId: string; canManage: boolean }) {
  const client = useQueryClient();
  const detail = useQuery({ queryKey: ["question", questionId], queryFn: () => fetchQuestion(questionId) });
  const sources = useQuery({ queryKey: ["sources"], queryFn: fetchSources, enabled: canManage });
  const [note, setNote] = useState("");
  const [sourceId, setSourceId] = useState("");
  const [status, setStatus] = useState<OpenQuestionRecord["status"] | "">("");
  const [priority, setPriority] = useState<OpenQuestionRecord["priority"] | "">("");
  const [message, setMessage] = useState("");
  const mutation = useMutation({ mutationFn: async (action: "status" | "note" | "source") => {
    if (!detail.data) throw new Error("لم يكتمل تحميل السؤال.");
    if (action === "note") return addQuestionNote(questionId, { note_ar: note.trim() });
    if (action === "source") return linkQuestionSource(questionId, { source_id: sourceId, role: "context" });
    return updateQuestion(questionId, { status: status || detail.data.question.status, priority: priority || detail.data.question.priority });
  }, onSuccess: async (_, action) => { setMessage(action === "note" ? "حُفظت الملاحظة." : action === "source" ? "رُبط المصدر بالسؤال." : "حُفظت الحالة والأولوية."); if (action === "note") setNote(""); if (action === "source") setSourceId(""); await Promise.all([client.invalidateQueries({ queryKey: ["question", questionId] }), client.invalidateQueries({ queryKey: ["research-workspace", questionId] }), client.invalidateQueries({ queryKey: ["questions"] })]); } });
  if (detail.isPending) return <p role="status">جارٍ فتح تفاصيل السؤال…</p>;
  if (detail.error || !detail.data) return <p role="alert">تعذر فتح تفاصيل السؤال. <button type="button" onClick={() => void detail.refetch()}>إعادة المحاولة</button></p>;
  return <section className="workspace-panel question-management"><h2>حالة السؤال وملاحظاته</h2>{message ? <p role="status">{message}</p> : null}{mutation.error ? <p role="alert">{mutation.error instanceof Error ? mutation.error.message : "تعذر الحفظ."}</p> : null}
    {canManage ? <><form className="workspace-inline-form" onSubmit={(event) => { event.preventDefault(); mutation.mutate("status"); }}><label>الحالة<select aria-label="حالة السؤال" value={status || detail.data.question.status} onChange={(event) => setStatus(event.target.value as typeof status)}><option value="open">مفتوح</option><option value="under_investigation">قيد التحقيق</option><option value="resolved">محسوم مؤقتاً</option><option value="reopened">أُعيد فتحه</option><option value="archived">مؤرشف</option></select></label><label>الأولوية<select aria-label="أولوية السؤال" value={priority || detail.data.question.priority} onChange={(event) => setPriority(event.target.value as typeof priority)}><option value="low">منخفضة</option><option value="normal">عادية</option><option value="high">عالية</option></select></label><button className="secondary-button" disabled={mutation.isPending} type="submit">حفظ الحالة</button></form>
    <form className="workspace-inline-form" onSubmit={(event) => { event.preventDefault(); mutation.mutate("note"); }}><label>ملاحظة التحقيق<textarea aria-label="ملاحظة التحقيق" required rows={3} value={note} onChange={(event) => setNote(event.target.value)} /></label><button className="primary-button" type="submit" disabled={mutation.isPending || !note.trim()}>احفظ الملاحظة</button></form>
    <form className="workspace-inline-form" onSubmit={(event) => { event.preventDefault(); mutation.mutate("source"); }}><label>مصدر مرتبط<select aria-label="مصدر مرتبط بالسؤال" required value={sourceId} onChange={(event) => setSourceId(event.target.value)}><option value="">اختر مصدراً من المكتبة</option>{sources.data?.map((source) => <option key={source.id} value={source.id}>{source.titleAr}</option>)}</select></label><button className="secondary-button" disabled={mutation.isPending || !sourceId} type="submit">اربط المصدر</button></form></> : <p>يمكن لصاحب السؤال أو المشرف إدارة حالته وإضافة الملاحظات.</p>}
    <h3>الملاحظات المحفوظة</h3>{detail.data.notes.length ? detail.data.notes.map((item) => <p className="workspace-note-row" key={item.id}>{item.noteAr}</p>) : <p>لا توجد ملاحظات محفوظة بعد.</p>}
    <h3>المصادر المرتبطة</h3>{detail.data.sources.map((source) => <p key={source.sourceId}><Link className="inline-link" to="/sources" search={{ sourceId: source.sourceId }}>{source.titleAr}</Link></p>)}
  </section>;
}
