import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, FileText, LoaderCircle, MessageSquareText, Send, X } from "lucide-react";
import { FormEvent, useState } from "react";
import { ApiError, fetchSuggestions, reviewSuggestion, submitSuggestion } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { SuggestionRecord } from "../types";

type SuggestionPanelProps = {
  treeId: string;
  versionId: string;
  versionNumber: number;
  nodeId: string;
  nodeName: string;
  canReview: boolean;
};

export function SuggestionPanel({ treeId, versionId, versionNumber, nodeId, nodeName, canReview }: SuggestionPanelProps) {
  const queryClient = useQueryClient();
  const [text, setText] = useState("");
  const [reviewNote, setReviewNote] = useState("");
  const [questionTitle, setQuestionTitle] = useState("");
  const [message, setMessage] = useState("");
  const queueQuery = useQuery({ queryKey: ["suggestions", treeId], queryFn: () => fetchSuggestions(treeId), enabled: canReview });
  const submitMutation = useMutation({
    mutationFn: () => submitSuggestion({ tree_id: treeId, version_id: versionId, node_id: nodeId, text_ar: text }),
    onSuccess: async () => {
      setText("");
      setMessage("وصل المقترح كما كتبته، وهو الآن في طابور المراجعة.");
      await queryClient.invalidateQueries({ queryKey: ["suggestions", treeId] });
    },
    onError: (error) => setMessage(suggestionError(error)),
  });
  const reviewMutation = useMutation({
    mutationFn: ({ id, input }: { id: string; input: Parameters<typeof reviewSuggestion>[1] }) => reviewSuggestion(id, input),
    onSuccess: async (reviewed) => {
      setMessage(reviewed.status === "converted" ? "حُوّل المقترح إلى سؤال مفتوح، وبقي النص الأصلي محفوظاً." : "حُفظ قرار المراجعة في سجل المقترح.");
      setReviewNote("");
      setQuestionTitle("");
      await queryClient.invalidateQueries({ queryKey: ["suggestions", treeId] });
    },
    onError: (error) => setMessage(suggestionError(error)),
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    submitMutation.mutate();
  };
  const decide = (suggestion: SuggestionRecord, decision: "accepted" | "rejected" | "converted") => {
    if (decision === "rejected" && !reviewNote.trim()) {
      setMessage("اكتب سبب الرفض قبل حفظه.");
      return;
    }
    setMessage("");
    reviewMutation.mutate({ id: suggestion.id, input: { decision, note_ar: reviewNote || undefined, question_title_ar: decision === "converted" ? questionTitle || undefined : undefined } });
  };
  const queue = queueQuery.data ?? [];
  const pending = queue.filter((item) => item.status === "pending");

  return (
    <section className="suggestion-panel">
      <div className="suggestion-panel-head"><div className="suggestion-panel-icon"><MessageSquareText size={17} /></div><div><div className="eyebrow">مشاركة دون تعديل مباشر</div><h2>اقترح إضافة لـ{nodeName}</h2><p>اكتب ملاحظتك بالعربية كما تراها. لن تغيّر نسخة الشجرة، وسيراجعها صاحب الشجرة أو متعاونوه.</p></div><span className="suggestion-version">النسخة {versionNumber}</span></div>
      {message ? <div className="suggestion-panel-message" role="status">{message}</div> : null}
      <form className="suggestion-submit-form" onSubmit={submit}><label className="composer-label">اقتراحك<textarea required value={text} onChange={(event) => setText(event.target.value)} rows={4} maxLength={10000} placeholder="اكتب الفقرة أو التصحيح الذي تقترحه، مع أي سياق يساعد المراجع." /></label><div className="suggestion-submit-footer"><small>النص الأصلي يبقى كما كتبته، دون تنسخ آلي أو استنتاج.</small><button className="primary-button" type="submit" disabled={submitMutation.isPending}>{submitMutation.isPending ? <LoaderCircle className="spin" size={14} /> : <Send size={14} />} أرسل المقترح</button></div></form>
      {canReview ? <div className="suggestion-review-section"><div className="suggestion-review-heading"><div><div className="detail-label">طابور المراجعة</div><small>{pending.length} مقترحاً بانتظار القرار</small></div><FileText size={16} /></div><div className="suggestion-review-fields"><label className="composer-label">ملاحظة القرار<textarea value={reviewNote} onChange={(event) => setReviewNote(event.target.value)} rows={2} placeholder="سبب القرار أو سياق إضافي" /></label><label className="composer-label">عنوان السؤال عند التحويل<input value={questionTitle} onChange={(event) => setQuestionTitle(event.target.value)} placeholder="اختياري؛ يبنى من المقترح" /></label></div>{queueQuery.isPending ? <p className="suggestion-empty">جارٍ فتح الطابور…</p> : queueQuery.error ? <div className="suggestion-panel-error" role="alert">{suggestionError(queueQuery.error)}</div> : queue.length > 0 ? <div className="suggestion-queue">{queue.map((item) => <SuggestionReviewRow item={item} key={item.id} onAccept={() => decide(item, "accepted")} onReject={() => decide(item, "rejected")} onConvert={() => decide(item, "converted")} pending={reviewMutation.isPending} />)}</div> : <p className="suggestion-empty">لا توجد مقترحات لهذه الشجرة بعد.</p>}</div> : null}
    </section>
  );
}

function SuggestionReviewRow({ item, onAccept, onReject, onConvert, pending }: { item: SuggestionRecord; onAccept: () => void; onReject: () => void; onConvert: () => void; pending: boolean }) {
  const isPending = item.status === "pending";
  return <article className="suggestion-queue-item"><div className="suggestion-queue-item-head"><div><StatusBadge tone={item.status === "pending" ? "question" : item.status === "rejected" ? "disputed" : "source"}>{suggestionStatusLabel(item.status)}</StatusBadge><strong>{item.nodeName}</strong><small>{new Date(item.createdAt).toLocaleString("ar")}</small></div><span>{item.reviewCount} قرار</span></div><p>{item.textAr}</p>{item.questionId ? <small className="suggestion-question-link">أُنشئ سؤال: {item.questionId}</small> : null}{item.reviews.length > 0 ? <div className="suggestion-review-history">{item.reviews.map((review) => <div key={review.id}><span>{review.reviewerName} · {suggestionDecisionLabel(review.decision)}</span>{review.noteAr ? <small>{review.noteAr}</small> : null}</div>)}</div> : null}{isPending ? <div className="suggestion-actions"><button className="secondary-button" type="button" onClick={onAccept} disabled={pending}><Check size={13} /> قبول</button><button className="secondary-button" type="button" onClick={onConvert} disabled={pending}>حوّل إلى سؤال</button><button className="danger-button" type="button" onClick={onReject} disabled={pending}><X size={13} /> رفض</button></div> : null}</article>;
}

function suggestionStatusLabel(status: SuggestionRecord["status"]): string {
  if (status === "accepted") return "مقبول";
  if (status === "rejected") return "مرفوض";
  if (status === "converted") return "محوّل إلى سؤال";
  return "بانتظار المراجعة";
}

function suggestionDecisionLabel(decision: string): string {
  if (decision === "accepted") return "قبله";
  if (decision === "rejected") return "رفضه";
  return "حوّله إلى سؤال";
}

function suggestionError(error: unknown): string {
  if (error instanceof ApiError && error.status === 401) return "سجّل الدخول لمراجعة المقترحات.";
  if (error instanceof ApiError && error.status === 403) return "ليست لديك صلاحية مراجعة هذه الشجرة.";
  return error instanceof Error ? error.message : "تعذر إكمال عملية المقترح.";
}
