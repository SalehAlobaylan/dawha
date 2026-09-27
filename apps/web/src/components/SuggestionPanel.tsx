import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, FileText, LoaderCircle, MessageSquareText, Send, ShieldAlert, X } from "lucide-react";
import { FormEvent, useState } from "react";
import { ApiError, fetchSuggestions, reviewSuggestion, submitSuggestion } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { SuggestionChangeSet, SuggestionChangeTarget, SuggestionRecord } from "../types";

type SuggestionPanelProps = {
  treeId: string;
  versionId: string;
  versionNumber: number;
  nodeId: string;
  nodeName: string;
  /**
   * The person this node names. The panel does not read it from the suggestion: a
   * change set is a change to records, and the one identifier the panel can state
   * without the reviewer pasting it is the person the proposal is attached to. It is
   * the default of the `person` target, it is what the preview shows, and it stays
   * editable in the form above.
   */
  personId: string;
  canReview: boolean;
};

type ChangeRefusal = { field: string; message: string } | null;

export function SuggestionPanel({ treeId, versionId, versionNumber, nodeId, nodeName, personId, canReview }: SuggestionPanelProps) {
  const queryClient = useQueryClient();
  const [text, setText] = useState("");
  const [reviewNote, setReviewNote] = useState("");
  const [questionTitle, setQuestionTitle] = useState("");
  const [message, setMessage] = useState("");
  const [changeBlocked, setChangeBlocked] = useState(false);
  const [changeRefusal, setChangeRefusal] = useState<ChangeRefusal>(null);
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
      setChangeBlocked(false);
      setChangeRefusal(null);
      await queryClient.invalidateQueries({ queryKey: ["suggestions", treeId] });
    },
    onError: (error, variables) => {
      const withChangeSet = Boolean(variables.input.change_set);
      setChangeRefusal(withChangeSet && error instanceof ApiError && error.status === 400 && error.field ? { field: error.field, message: error.message } : null);
      // A refusal that arrived with a change set is about the right to write to the
      // research tables, not about the right to review this tree. The panel says which
      // one it was, and stops offering the button the service has already refused,
      // while the plain decision stays available - it is what the panel did before
      // this composer existed, and removing it would take away the one path a
      // reviewer without a global write role has.
      if (withChangeSet && error instanceof ApiError && error.status === 403) {
        setChangeBlocked(true);
      }
      setMessage(changeSetError(error, withChangeSet));
    },
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
  const decideWithChange = (suggestion: SuggestionRecord, changeSet: SuggestionChangeSet) => {
    setMessage("");
    reviewMutation.mutate({ id: suggestion.id, input: { decision: "accepted", note_ar: reviewNote || undefined, change_set: changeSet } });
  };
  const queue = queueQuery.data ?? [];
  const pending = queue.filter((item) => item.status === "pending");

  return (
    <section className="suggestion-panel">
      <div className="suggestion-panel-head"><div className="suggestion-panel-icon"><MessageSquareText size={17} /></div><div><div className="eyebrow">مشاركة دون تعديل مباشر</div><h2>اقترح إضافة لـ{nodeName}</h2><p>اكتب ملاحظتك بالعربية كما تراها. لن تغيّر نسخة الشجرة، وسيراجعها صاحب الشجرة أو متعاونوه.</p></div><span className="suggestion-version">النسخة {versionNumber}</span></div>
      {message ? <div className="suggestion-panel-message" role="status">{message}</div> : null}
      <form className="suggestion-submit-form" onSubmit={submit}><label className="composer-label">اقتراحك<textarea required value={text} onChange={(event) => setText(event.target.value)} rows={4} maxLength={10000} placeholder="اكتب الفقرة أو التصحيح الذي تقترحه، مع أي سياق يساعد المراجع." /></label><div className="suggestion-submit-footer"><small>النص الأصلي يبقى كما كتبته، دون تنسخ آلي أو استنتاج.</small><button className="primary-button" type="submit" disabled={submitMutation.isPending}>{submitMutation.isPending ? <LoaderCircle className="spin" size={14} /> : <Send size={14} />} أرسل المقترح</button></div></form>
      {canReview ? <div className="suggestion-review-section"><div className="suggestion-review-heading"><div><div className="detail-label">طابور المراجعة</div><small>{pending.length} مقترحاً بانتظار القرار</small></div><FileText size={16} /></div><div className="suggestion-review-fields"><label className="composer-label">ملاحظة القرار<textarea value={reviewNote} onChange={(event) => setReviewNote(event.target.value)} rows={2} placeholder="سبب القرار أو سياق إضافي" /></label><label className="composer-label">عنوان السؤال عند التحويل<input value={questionTitle} onChange={(event) => setQuestionTitle(event.target.value)} placeholder="اختياري؛ يبنى من المقترح" /></label></div>{queueQuery.isPending ? <p className="suggestion-empty">جارٍ فتح الطابور…</p> : queueQuery.error ? <div className="suggestion-panel-error" role="alert">{suggestionError(queueQuery.error)}</div> : queue.length > 0 ? <div className="suggestion-queue">{queue.map((item) => <SuggestionReviewRow item={item} key={item.id} personId={personId} refusal={changeRefusal} changeBlocked={changeBlocked} onAccept={() => decide(item, "accepted")} onReject={() => decide(item, "rejected")} onConvert={() => decide(item, "converted")} onAcceptWithChange={(changeSet) => decideWithChange(item, changeSet)} pending={reviewMutation.isPending} />)}</div> : <p className="suggestion-empty">لا توجد مقترحات لهذه الشجرة بعد.</p>}</div> : null}
    </section>
  );
}

type SuggestionReviewRowProps = {
  item: SuggestionRecord;
  personId: string;
  refusal: ChangeRefusal;
  changeBlocked: boolean;
  onAccept: () => void;
  onReject: () => void;
  onConvert: () => void;
  onAcceptWithChange: (changeSet: SuggestionChangeSet) => void;
  pending: boolean;
};

function SuggestionReviewRow({ item, personId, refusal, changeBlocked, onAccept, onReject, onConvert, onAcceptWithChange, pending }: SuggestionReviewRowProps) {
  const isPending = item.status === "pending";
  return <article className="suggestion-queue-item"><div className="suggestion-queue-item-head"><div><StatusBadge tone={item.status === "pending" ? "question" : item.status === "rejected" ? "disputed" : "source"}>{suggestionStatusLabel(item.status)}</StatusBadge><strong>{item.nodeName}</strong><small>{new Date(item.createdAt).toLocaleString("ar")}</small></div><span>{item.reviewCount} قرار</span></div><p>{item.textAr}</p>{item.questionId ? <small className="suggestion-question-link">أُنشئ سؤال: {item.questionId}</small> : null}{item.reviews.length > 0 ? <div className="suggestion-review-history">{item.reviews.map((review) => <div key={review.id}><span>{review.reviewerName} · {suggestionDecisionLabel(review.decision)}</span>{review.noteAr ? <small>{review.noteAr}</small> : null}</div>)}</div> : null}{isPending ? <div><div className="suggestion-actions"><button className="secondary-button" type="button" onClick={onAccept} disabled={pending}><Check size={13} /> قبول</button><button className="secondary-button" type="button" onClick={onConvert} disabled={pending}>حوّل إلى سؤال</button><button className="danger-button" type="button" onClick={onReject} disabled={pending}><X size={13} /> رفض</button></div><ChangeSetComposer personId={personId} refusal={refusal} blocked={changeBlocked} pending={pending} onAcceptWithChange={onAcceptWithChange} /></div> : null}</article>;
}

type ChangeDraft = {
  personId: string;
  target: SuggestionChangeTarget;
  aliasName: string;
  aliasType: string;
  subjectType: string;
  subjectId: string;
  objectType: string;
  objectId: string;
  predicate: string;
  validFrom: string;
  validTo: string;
  placeId: string;
  timeFrom: string;
  timeTo: string;
  claimNote: string;
  claimId: string;
  linkKind: "statement" | "passage";
  linkId: string;
  relation: string;
  linkNote: string;
};

/**
 * The composer for the change set a review may carry.
 *
 * Three rules shape it, and all three come from what the service does rather than
 * from what a form could decide:
 *
 *  1. The service is the only validator. Nothing here checks a uuid, a predicate or a
 *     date: the form sends the typed block, the service refuses it, and the refusal
 *     names the field it is about, which is marked on that input in the service's own
 *     words. A rule here would be a second opinion that can disagree with the one
 *     that decides, and the closed vocabularies below are offered as datalist hints
 *     rather than as enums the form would refuse to let a reviewer leave.
 *  2. The acceptance is described before it is sent: the target, the identifiers it
 *     writes against, and the record that will exist afterwards, in the state the
 *     service leaves it in. A reviewer approving a change should not imagine it.
 *  3. Only the four typed targets the service accepts are offered, and nothing here
 *     offers publish, claim acceptance, a merge or a dispute resolution - a change
 *     set never does any of those, so a button that implied otherwise would be a
 *     lie about the API rather than a feature.
 */
function ChangeSetComposer({ personId, refusal, blocked, pending, onAcceptWithChange }: { personId: string; refusal: ChangeRefusal; blocked: boolean; pending: boolean; onAcceptWithChange: (changeSet: SuggestionChangeSet) => void }) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<ChangeDraft>({
    personId,
    target: "person",
    aliasName: "",
    aliasType: "alternative_name",
    subjectType: "person",
    subjectId: personId,
    objectType: "person",
    objectId: "",
    predicate: "",
    validFrom: "",
    validTo: "",
    placeId: "",
    timeFrom: "",
    timeTo: "",
    claimNote: "",
    claimId: "",
    linkKind: "statement",
    linkId: "",
    relation: "supports",
    linkNote: "",
  });
  const set = <K extends keyof ChangeDraft>(key: K, value: ChangeDraft[K]) => setDraft((current) => ({ ...current, [key]: value }));
  const changeSet = buildChangeSet(draft);
  const preview = describeChangeSet(changeSet);

  if (!open) {
    return <button className="text-button change-composer-toggle" type="button" onClick={() => setOpen(true)} disabled={pending}><FileText size={13} /> قبول مع تغيير موثّق</button>;
  }

  return <div className="change-composer">
    <div className="change-composer-head"><span className="detail-label">تغيير موثّق مع القبول</span><small>ما تكتبه هنا يُكتب في سجلات البحث، ويُسجَّل في سجل التدقيق مع اسمك.</small></div>
    {blocked ? <div className="suggestion-panel-error" role="alert"><ShieldAlert size={13} /> تطبيق التغيير يتطلب دور كتابة عام على مستوى المنصة، وهذا الدور غير متاح لحسابك.قرار القبول أو التحويل أو الرفض ما زال متاحاً في الأزرار أعلاه.</div> : null}
    <label className="composer-label">هدف التغيير<select value={draft.target} onChange={(event) => set("target", event.target.value as SuggestionChangeTarget)}><option value="person">اسم إضافي لشخص</option><option value="relationship">علاقة بين كيانين</option><option value="claim">ادعاء جديد</option><option value="source_link">إسناد مصدر إلى ادعاء</option></select><FieldError refusal={refusal} path="change_set.target" /></label>
    {draft.target === "person" ? <>
      <label className="composer-label">معرّف الشخص<input value={draft.personId} onChange={(event) => set("personId", event.target.value)} /><FieldError refusal={refusal} path="change_set.person.person_id" /></label>
      <label className="composer-label">الاسم البديل<input value={draft.aliasName} onChange={(event) => set("aliasName", event.target.value)} placeholder="الاسم كما ورد في المصدر" /><FieldError refusal={refusal} path="change_set.person.name_ar" /></label>
      <label className="composer-label">نوع اللقب<select value={draft.aliasType} onChange={(event) => set("aliasType", event.target.value)}><option value="alternative_name">اسم بديل</option><option value="kunyah">كنية</option><option value="laqab">لقب</option><option value="nisbah">نسبة</option><option value="source_spelling">رسم في المصدر</option></select><FieldError refusal={refusal} path="change_set.person.alias_type" /></label>
    </> : null}
    {draft.target === "relationship" || draft.target === "claim" ? <>
      <div className="composer-grid"><label className="composer-label">نوع الطرف الأول<select value={draft.subjectType} onChange={(event) => set("subjectType", event.target.value)}><option value="person">شخص</option><option value="family">عائلة</option><option value="branch">فرع</option><option value="tribe">قبيلة</option><option value="place">موضع</option></select><FieldError refusal={refusal} path="change_set.relationship.subject.type" /><FieldError refusal={refusal} path="change_set.claim.subject.type" /></label><label className="composer-label">معرّف الطرف الأول<input value={draft.subjectId} onChange={(event) => set("subjectId", event.target.value)} /><FieldError refusal={refusal} path="change_set.relationship.subject.id" /><FieldError refusal={refusal} path="change_set.claim.subject.id" /></label></div>
      <div className="composer-grid"><label className="composer-label">نوع الطرف الثاني<select value={draft.objectType} onChange={(event) => set("objectType", event.target.value)}><option value="person">شخص</option><option value="family">عائلة</option><option value="branch">فرع</option><option value="tribe">قبيلة</option><option value="place">موضع</option></select><FieldError refusal={refusal} path="change_set.relationship.object.type" /><FieldError refusal={refusal} path="change_set.claim.object.type" /></label><label className="composer-label">معرّف الطرف الثاني<input value={draft.objectId} onChange={(event) => set("objectId", event.target.value)} /><FieldError refusal={refusal} path="change_set.relationship.object.id" /><FieldError refusal={refusal} path="change_set.relationship.object_id" /><FieldError refusal={refusal} path="change_set.claim.object.id" /><FieldError refusal={refusal} path="change_set.claim.object_id" /></label></div>
      <label className="composer-label">{draft.target === "relationship" ? "العلاقة" : "صفة الادعاء"}<input list="change-composer-predicates" value={draft.predicate} onChange={(event) => set("predicate", event.target.value)} /><datalist id="change-composer-predicates">{PREDICATES.map((value) => <option key={value} value={value} />)}</datalist><FieldError refusal={refusal} path="change_set.relationship.predicate" /><FieldError refusal={refusal} path="change_set.claim.predicate" /></label>
      {draft.target === "relationship" ? <div className="composer-grid"><label className="composer-label">الصلاحية من<input type="date" value={draft.validFrom} onChange={(event) => set("validFrom", event.target.value)} /><FieldError refusal={refusal} path="change_set.relationship.valid_from" /></label><label className="composer-label">الصلاحية إلى<input type="date" value={draft.validTo} onChange={(event) => set("validTo", event.target.value)} /><FieldError refusal={refusal} path="change_set.relationship.valid_to" /></label></div> : <>
        <div className="composer-grid"><label className="composer-label">من تاريخ<input type="date" value={draft.timeFrom} onChange={(event) => set("timeFrom", event.target.value)} /><FieldError refusal={refusal} path="change_set.claim.time_from" /></label><label className="composer-label">إلى تاريخ<input type="date" value={draft.timeTo} onChange={(event) => set("timeTo", event.target.value)} /><FieldError refusal={refusal} path="change_set.claim.time_to" /></label></div>
        <label className="composer-label">الموضع (اختياري)<input value={draft.placeId} onChange={(event) => set("placeId", event.target.value)} /><FieldError refusal={refusal} path="change_set.claim.place_id" /></label>
        <label className="composer-label">ملاحظة الادعاء<textarea value={draft.claimNote} onChange={(event) => set("claimNote", event.target.value)} rows={2} /><FieldError refusal={refusal} path="change_set.claim.note_ar" /></label>
      </>}
    </> : null}
    {draft.target === "source_link" ? <>
      <label className="composer-label">معرّف الادعاء<input value={draft.claimId} onChange={(event) => set("claimId", event.target.value)} /><FieldError refusal={refusal} path="change_set.source_link.claim_id" /></label>
      <div className="composer-grid"><label className="composer-label">نوع الإسناد<select value={draft.linkKind} onChange={(event) => set("linkKind", event.target.value as "statement" | "passage")}><option value="statement">عبارة من مصدر</option><option value="passage">مقطع من مصدر</option></select></label><label className="composer-label">معرّف العبارة أو المقطع<input value={draft.linkId} onChange={(event) => set("linkId", event.target.value)} /><FieldError refusal={refusal} path="change_set.source_link.source_statement_id" /><FieldError refusal={refusal} path="change_set.source_link.source_passage_id" /></label></div>
      <label className="composer-label">علاقة الإسناد<select value={draft.relation} onChange={(event) => set("relation", event.target.value)}><option value="supports">يدعم</option><option value="contextualizes">يضع في سياقه</option><option value="contradicts">يناقض</option><option value="refutes">يفند</option></select><FieldError refusal={refusal} path="change_set.source_link.relation" /></label>
      <label className="composer-label">ملاحظة الإسناد<textarea value={draft.linkNote} onChange={(event) => set("linkNote", event.target.value)} rows={2} /><FieldError refusal={refusal} path="change_set.source_link.note_ar" /></label>
    </> : null}
    <div className="change-composer-preview">
      <span className="detail-label">ما سيحدث عند القبول</span>
      <p>{preview.result}</p>
      <ul>{preview.identifiers.map((entry) => <li key={entry.label}><span>{entry.label}</span><bdi>{entry.value}</bdi></li>)}</ul>
      <small className="change-composer-note">لن يُنشر أي شجرة، ولن تُقبل أي ادعاء، ولن تُدمج كيانات، ولن تُحل علاقة قائمة: تغيّر واحد موقّع من الخدمة، في الحالة التي تتركها هي.</small>
    </div>
    <div className="suggestion-actions"><button className="primary-button" type="button" onClick={() => onAcceptWithChange(changeSet)} disabled={pending || blocked}><Check size={13} /> قبّل مع التغيير</button><button className="text-button" type="button" onClick={() => setOpen(false)} disabled={pending}>إلغاء</button></div>
  </div>;
}

/**
 * The closed vocabularies the service accepts, offered as a hint the reviewer can
 * ignore rather than as a list the form would refuse to let them leave. The service
 * answers for the vocabulary; this list is here to save typing.
 */
const PREDICATES = ["parent_of", "father_of", "mother_of", "spouse_of", "sibling_of", "son_of", "daughter_of", "brother_of", "sister_of", "born_in", "died_in", "resided_in"];

/**
 * FieldError puts the service's refusal on the input it is about. The path is the JSON
 * path the caller sent, so this component is a lookup and not a rule: it renders a
 * message the service wrote, for a field the service named, and it renders nothing
 * when the service named no field.
 */
function FieldError({ refusal, path }: { refusal: ChangeRefusal; path: string }) {
  if (!refusal || refusal.field !== path) return null;
  return <small className="composer-field-error" role="alert">{refusal.message}</small>;
}

/**
 * buildChangeSet sends exactly the shape internal/suggestions validates: one target and
 * the one typed block that target names, with the optional keys left out when the
 * reviewer left them empty. Nothing is filled in that the reviewer did not type, because
 * the service - not this function - decides what a change means.
 */
function buildChangeSet(draft: ChangeDraft): SuggestionChangeSet {
  const text = (value: string) => value.trim();
  switch (draft.target) {
    case "person":
      return { target: "person", person: { person_id: text(draft.personId), name_ar: text(draft.aliasName), ...(text(draft.aliasType) ? { alias_type: text(draft.aliasType) } : {}) } };
    case "relationship":
      return {
        target: "relationship",
        relationship: {
          subject_type: text(draft.subjectType),
          subject_id: text(draft.subjectId),
          object_type: text(draft.objectType),
          object_id: text(draft.objectId),
          predicate: text(draft.predicate),
          ...(text(draft.validFrom) ? { valid_from: text(draft.validFrom) } : {}),
          ...(text(draft.validTo) ? { valid_to: text(draft.validTo) } : {}),
        },
      };
    case "claim":
      return {
        target: "claim",
        claim: {
          subject_type: text(draft.subjectType),
          subject_id: text(draft.subjectId),
          object_type: text(draft.objectType),
          object_id: text(draft.objectId),
          predicate: text(draft.predicate),
          ...(text(draft.placeId) ? { place_id: text(draft.placeId) } : {}),
          ...(text(draft.timeFrom) ? { time_from: text(draft.timeFrom) } : {}),
          ...(text(draft.timeTo) ? { time_to: text(draft.timeTo) } : {}),
          ...(text(draft.claimNote) ? { note_ar: text(draft.claimNote) } : {}),
        },
      };
    case "source_link":
      return {
        target: "source_link",
        source_link: {
          claim_id: text(draft.claimId),
          // Exactly one of the two, because the service refuses a link carrying both and
          // a link carrying neither. Which one is the reviewer's choice, expressed by
          // the reference they typed into the field they were shown.
          ...(draft.linkKind === "passage" ? { source_passage_id: text(draft.linkId) } : { source_statement_id: text(draft.linkId) }),
          ...(text(draft.relation) ? { relation: text(draft.relation) } : {}),
          ...(text(draft.linkNote) ? { note_ar: text(draft.linkNote) } : {}),
        },
      };
  }
}

type ChangePreview = { result: string; identifiers: { label: string; value: string }[] };

/**
 * describeChangeSet is what the reviewer reads before pressing the button: the record
 * that will exist, in the state the service writes it in, and the identifiers it will
 * be written against. It says «غير محسومة» for a relationship and for a claim because
 * that is the state applyChangeSet writes - a change set can never resolve a
 * relationship or accept a claim, and a preview that implied otherwise would be the
 * form promising something the API does not do.
 */
function describeChangeSet(changeSet: SuggestionChangeSet): ChangePreview {
  const short = (value: string) => (value.trim() ? `${value.trim().slice(0, 8)}…` : "—");
  const range = (from?: string, to?: string) => {
    if (from?.trim() && to?.trim()) return `، بين ${from.trim()} و${to.trim()}`;
    if (from?.trim()) return `، من ${from.trim()}`;
    if (to?.trim()) return `، حتى ${to.trim()}`;
    return "";
  };
  if (changeSet.person) {
    return {
      result: `سيُضاف «${changeSet.person.name_ar || "…"}» اسمًا بديلًا من نوع ${changeSet.person.alias_type ?? "alternative_name"} إلى الشخص ${short(changeSet.person.person_id)}. الاسم الجديد لا يمسّ الاسم المعتمد.`,
      identifiers: [{ label: "الشخص", value: changeSet.person.person_id || "—" }],
    };
  }
  if (changeSet.relationship) {
    const relationship = changeSet.relationship;
    return {
      result: `ستُسجَّل علاقة ${relationship.subject_type ?? "person"} ${short(relationship.subject_id)} ← ${relationship.predicate || "…"} → ${relationship.object_type ?? "person"} ${short(relationship.object_id)} بحالة «غير محسومة»${range(relationship.valid_from, relationship.valid_to)}.`,
      identifiers: [
        { label: "الطرف الأول", value: relationship.subject_id || "—" },
        { label: "الطرف الثاني", value: relationship.object_id || "—" },
      ],
    };
  }
  if (changeSet.claim) {
    const claim = changeSet.claim;
    return {
      result: `سيُسجَّل ادعاء ${claim.subject_type ?? "person"} ${short(claim.subject_id)} ← ${claim.predicate || "…"} → ${claim.object_type ?? "person"} ${short(claim.object_id)} بحالة «غير محسومة»${range(claim.time_from, claim.time_to)}${claim.place_id ? `، في الموضع ${short(claim.place_id)}` : ""}.`,
      identifiers: [
        { label: "موضوع الادعاء", value: claim.subject_id || "—" },
        { label: "طرف الادعاء", value: claim.object_id || "—" },
        ...(claim.place_id ? [{ label: "الموضع", value: claim.place_id }] : []),
      ],
    };
  }
  const link = changeSet.source_link;
  if (link) {
    const attached = link.source_passage_id !== undefined ? `مقطع ${short(link.source_passage_id)}` : `عبارة ${short(link.source_statement_id ?? "")}`;
    return {
      result: `ستُضاف ${attached} إلى الادعاء ${short(link.claim_id)} كإسناد «${link.relation ?? "supports"}».`,
      identifiers: [
        { label: "الادعاء", value: link.claim_id || "—" },
        { label: "المصدر", value: link.source_passage_id ?? link.source_statement_id ?? "—" },
      ],
    };
  }
  return { result: "لم يُعثر على تغيير.", identifiers: [] };
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

/**
 * changeSetError separates the two things a refusal can mean on this route, because
 * they are different refusals: the right to review this tree, and the right to write
 * to the research tables a change set writes. A refusal of the second says so, and
 * carries the service's own words - the field is marked on the input separately - so a
 * reviewer can act on the answer instead of guessing which input caused it.
 */
function changeSetError(error: unknown, withChangeSet: boolean): string {
  if (withChangeSet && error instanceof ApiError && error.status === 403) {
    return "رفضت الخدمة تطبيق التغيير: تطبيقه يتطلب دور كتابة عام على مستوى المنصة، وهو غير متاح لحسابك. القبول بلا تغيير ما زال متاحاً.";
  }
  if (withChangeSet && error instanceof ApiError && error.status === 400) {
    return `رفضت الخدمة التغيير: ${error.message}`;
  }
  return suggestionError(error);
}
