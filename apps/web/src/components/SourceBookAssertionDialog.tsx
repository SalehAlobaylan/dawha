import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BookOpen, Check, FileText, Quote, Save, X } from "lucide-react";
import { FormEvent, useEffect, useState } from "react";
import { ApiError, createBookAssertion, fetchBookAssertions, fetchSourcePagePreview, reviewBookAssertion, updateBookAssertion } from "../lib/api";
import type { BookAssertion, BookAssertionInput, BookAssertionPredicate } from "../types";

interface QuoteSeed {
  start: number;
  end: number;
  text: string;
}

interface SourceBookAssertionDialogProps {
  fileId: string;
  pageEvidenceId: string;
  pageNumber: number;
  pageReviewRevisionId: string;
  expectedReviewRevision: number;
  quote: QuoteSeed;
  onClose: () => void;
  onReviewed: () => Promise<void>;
}

const predicates: Array<[BookAssertionPredicate, string]> = [
  ["parent_of", "والد / والدة"],
  ["spouse_of", "زوج / زوجة"],
  ["sibling_of", "أخ / أخت"],
  ["descendant_of", "من ذرية"],
  ["reported_relationship", "علاقة مذكورة دون تحديد"],
  ["mention_only", "ذكر اسم فقط"],
];

export function SourceBookAssertionDialog({ fileId, pageEvidenceId, pageNumber, pageReviewRevisionId, expectedReviewRevision, quote, onClose, onReviewed }: SourceBookAssertionDialogProps) {
  const queryClient = useQueryClient();
  const [assertion, setAssertion] = useState<BookAssertion | null>(null);
  const [quoteSelection, setQuoteSelection] = useState<QuoteSeed>(quote);
  const [subject, setSubject] = useState("");
  const [predicate, setPredicate] = useState<BookAssertionPredicate>("parent_of");
  const [object, setObject] = useState("");
  const [printedPage, setPrintedPage] = useState("");
  const [attribution, setAttribution] = useState("");
  const [polarity, setPolarity] = useState<BookAssertionInput["polarity"]>("affirmed");
  const [certainty, setCertainty] = useState<BookAssertionInput["certainty"]>("asserted");
  const [note, setNote] = useState("");
  const [message, setMessage] = useState("");
  const assertionsQuery = useQuery({ queryKey: ["book-assertions", fileId], queryFn: () => fetchBookAssertions(fileId) });
  const previewQuery = useQuery({ queryKey: ["source-page-preview", fileId, pageNumber], queryFn: () => fetchSourcePagePreview(fileId, pageNumber) });
  const [previewURL, setPreviewURL] = useState("");
  useEffect(() => {
    if (!previewQuery.data) { setPreviewURL(""); return; }
    const url = URL.createObjectURL(previewQuery.data);
    setPreviewURL(url);
    return () => URL.revokeObjectURL(url);
  }, [previewQuery.data]);

  const makeInput = (expectedReviewRevisionValue: number): BookAssertionInput => ({
    pageEvidenceId,
    pageReviewRevisionId,
    expectedReviewRevision: expectedReviewRevisionValue,
    quoteStartCodepoint: quoteSelection.start,
    quoteEndCodepoint: quoteSelection.end,
    quoteTextAr: quoteSelection.text,
    subjectMentionAr: subject,
    predicate,
    objectMentionAr: object,
    printedPageLabel: printedPage,
    attributionAr: attribution,
    polarity,
    certainty,
    noteAr: note,
  });

  const saveMutation = useMutation({
    mutationFn: () => assertion
      ? updateBookAssertion(assertion.id, { ...makeInput(expectedReviewRevision), expectedRevision: assertion.revision })
      : createBookAssertion(fileId, makeInput(expectedReviewRevision)),
    onSuccess: async (saved) => {
      setAssertion(saved);
      setMessage(saved.revision === 1 ? "حُفظت العبارة كمسودة غير معتمدة." : "حُفظت نسخة جديدة من المسودة.");
      await queryClient.invalidateQueries({ queryKey: ["book-assertions", fileId] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const reviewMutation = useMutation({
    mutationFn: (decision: "reviewed" | "rejected") => reviewBookAssertion(assertion?.id ?? "", { expectedRevision: assertion?.revision ?? 0, decision }),
    onSuccess: async (reviewed) => {
      setAssertion(reviewed);
      setMessage(reviewed.state === "reviewed" ? "أضيفت العبارة إلى عبارات المصدر مع حفظ مرجع الصفحة." : "رُفضت المسودة وحُفظ قرار المراجع.");
      await queryClient.invalidateQueries({ queryKey: ["book-assertions", fileId] });
      if (reviewed.state === "reviewed") await onReviewed();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });

  const selectExisting = (value: BookAssertion) => {
    setAssertion(value);
    setQuoteSelection({ start: value.quoteStartCodepoint, end: value.quoteEndCodepoint, text: value.quoteTextAr });
    setSubject(value.subjectMentionAr);
    setPredicate(value.predicate);
    setObject(value.objectMentionAr);
    setPrintedPage(value.printedPageLabel ?? "");
    setAttribution(value.attributionAr ?? "");
    setPolarity(value.polarity);
    setCertainty(value.certainty);
    setNote(value.noteAr ?? "");
    setMessage(value.pageReviewRevisionId === pageReviewRevisionId ? "تم فتح العبارة المرتبطة بالمراجعة الحالية." : "هذه العبارة مرتبطة بمراجعة نص أقدم؛ افتح نص تلك المراجعة لمتابعتها.");
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    saveMutation.mutate();
  };

  const editable = !assertion || (assertion.state === "draft" && assertion.pageReviewRevisionId === pageReviewRevisionId);
  const pageAssertions = (assertionsQuery.data ?? []).filter((item) => item.pageEvidenceId === pageEvidenceId);

  return (
    <div className="source-book-dialog-backdrop" role="presentation" onClick={onClose}>
      <section className="source-book-dialog" role="dialog" aria-modal="true" aria-labelledby="source-book-dialog-title" dir="rtl" onClick={(event) => event.stopPropagation()} onKeyDown={(event) => {
        if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); onClose(); return; }
        if (event.key === "Tab") {
          const focusable = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled)"));
          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          if (event.shiftKey && event.currentTarget.ownerDocument.activeElement === first) { event.preventDefault(); last?.focus(); }
          else if (!event.shiftKey && event.currentTarget.ownerDocument.activeElement === last) { event.preventDefault(); first?.focus(); }
        }
      }}>
        <header className="source-book-dialog-head">
          <div><div className="eyebrow"><BookOpen size={13} /> دليل كتاب</div><h3 id="source-book-dialog-title">ترميز عبارة من الصفحة {pageNumber}</h3><p>اربط كل تفسير بالنص الظاهر. أسماء الأشخاص هنا تظل إشارات غير موصولة بهويات المنصة.</p></div>
          <button className="secondary-button" type="button" autoFocus onClick={onClose} aria-label="إغلاق"><X size={16} /></button>
        </header>

        {pageAssertions.length > 0 ? <div className="source-book-existing">
          <div className="source-book-section-title"><FileText size={14} /><strong>عبارات سبق تسجيلها في هذه الصفحة</strong></div>
          {pageAssertions.map((item) => <button className={`source-book-existing-row${assertion?.id === item.id ? " is-active" : ""}`} type="button" key={item.id} onClick={() => selectExisting(item)}><span><q>{item.quoteTextAr}</q><small>{assertionStateLabel(item.state)} · النسخة {item.revision}</small></span><span>{item.subjectMentionAr} · {predicateLabel(item.predicate)} · {item.objectMentionAr}</span></button>)}
        </div> : null}

        <div className="source-book-context-grid">
          <div className="source-book-quote-card">
            <div className="source-book-section-title"><Quote size={14} /><strong>النص الحرفي المحدد</strong><small>موضعه داخل مراجعة الصفحة: {quoteSelection.start}–{quoteSelection.end} نقطة Unicode</small></div>
            <blockquote>{quoteSelection.text}</blockquote>
            <p>الموضع: الصفحة المصورة {pageNumber} · مراجعة النسخ رقم {assertion?.pageReviewRevisionId === pageReviewRevisionId ? expectedReviewRevision : assertion?.pageReviewRevisionId ? "أقدم" : expectedReviewRevision}. لن تُقبل العبارة إذا تغير نص المراجعة.</p>
          </div>
          <div className="source-book-page-image">
            <div className="source-book-section-title"><FileText size={14} /><strong>صورة الصفحة للمقارنة</strong></div>
            {previewURL ? <img src={previewURL} alt={`صورة صفحة الكتاب ${pageNumber}`} /> : <p role={previewQuery.error ? "alert" : "status"}>{previewQuery.isPending ? "جارٍ تحميل صورة الصفحة…" : errorMessage(previewQuery.error)}</p>}
            <small>النص المحدد مرتبط بمراجعة النسخ؛ قارن ألفاظه بالصورة الأصلية قبل الاعتماد.</small>
          </div>
        </div>

        {editable ? <form className="source-book-assertion-form" onSubmit={submit}>
          <div className="source-book-form-grid">
            <label className="composer-label">الاسم المذكور أولاً<input required maxLength={200} value={subject} onChange={(event) => setSubject(event.target.value)} placeholder="كما ورد في الاقتباس" /></label>
            <label className="composer-label">نوع العلاقة<select value={predicate} onChange={(event) => setPredicate(event.target.value as BookAssertionPredicate)}>{predicates.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
            <label className="composer-label">الاسم المذكور الآخر<input maxLength={200} value={object} onChange={(event) => setObject(event.target.value)} placeholder="اتركه فارغاً عند ذكر اسم فقط" disabled={predicate === "mention_only"} required={predicate !== "mention_only" && predicate !== "reported_relationship"} /></label>
            <label className="composer-label">رقم الصفحة المطبوع<input maxLength={100} value={printedPage} onChange={(event) => setPrintedPage(event.target.value)} placeholder="اختياري" /></label>
            <label className="composer-label">نسبة القول<input maxLength={300} value={attribution} onChange={(event) => setAttribution(event.target.value)} placeholder="المؤلف أو الراوي كما ورد" /></label>
            <label className="composer-label">دلالة العبارة<select value={polarity} onChange={(event) => setPolarity(event.target.value as BookAssertionInput["polarity"])}><option value="affirmed">إثبات</option><option value="negated">نفي</option></select></label>
            <label className="composer-label">درجة الجزم<select value={certainty} onChange={(event) => setCertainty(event.target.value as BookAssertionInput["certainty"])}><option value="asserted">قول مباشر</option><option value="reported">رواية منقولة</option><option value="uncertain">صيغة غير جازمة</option></select></label>
          </div>
          <label className="composer-label">ملاحظة المراجع<textarea rows={2} maxLength={2000} value={note} onChange={(event) => setNote(event.target.value)} placeholder="اختياري: التباس أو سياق قريب" /></label>
          {message ? <p className="source-book-message" role="status">{message}</p> : null}
          <div className="source-book-actions">
            <button className="secondary-button" type="submit" disabled={saveMutation.isPending || !subject.trim() || (!object.trim() && predicate !== "mention_only" && predicate !== "reported_relationship")}><Save size={14} />{saveMutation.isPending ? "جارٍ الحفظ…" : assertion ? "حفظ نسخة جديدة" : "حفظ كمسودة للمراجعة"}</button>
            {assertion?.state === "draft" ? <><button className="primary-button" type="button" disabled={saveMutation.isPending || reviewMutation.isPending} onClick={() => reviewMutation.mutate("reviewed")}><Check size={14} /> اعتمادها كعبارة مصدر</button><button className="secondary-button" type="button" disabled={saveMutation.isPending || reviewMutation.isPending} onClick={() => reviewMutation.mutate("rejected")}>رفض المسودة</button></> : null}
          </div>
        </form> : <div className="source-book-terminal-state"><strong>{assertion?.state === "reviewed" ? "عبارة المصدر معتمدة للمراجعة البحثية." : assertion?.state === "rejected" ? "رُفضت هذه المسودة." : "هذه العبارة لا تطابق مراجعة النص المفتوحة."}</strong><small>لم تُنشأ هوية شخص أو علاقة في شجرة الأسرة من هذه العبارة.</small>{assertion?.sourceStatementId ? <small>مرجع عبارة المصدر: {assertion.sourceStatementId}</small> : null}</div>}

        {message && !editable ? <p className="source-book-message" role="status">{message}</p> : null}
        {assertionsQuery.error ? <p className="source-book-message" role="alert">{errorMessage(assertionsQuery.error)}</p> : null}
        <footer className="source-book-dialog-footer"><span>المراجعة تنشئ نص مصدر موثقاً فقط؛ تحويل الأسماء إلى أشخاص أو قبول النسب قرار بحثي منفصل.</span><button className="secondary-button" type="button" onClick={onClose}>إغلاق</button></footer>
      </section>
    </div>
  );
}

function predicateLabel(value: BookAssertionPredicate): string {
  return predicates.find(([key]) => key === value)?.[1] ?? value;
}

function assertionStateLabel(value: BookAssertion["state"]): string {
  if (value === "reviewed") return "مراجعة معتمدة";
  if (value === "rejected") return "مرفوضة";
  return "مسودة";
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 409) return "تغيرت مراجعة الصفحة أو المسودة منذ فتحها. أغلق النافذة وأعد فتح النص الأحدث.";
  return error instanceof Error ? error.message : "تعذر حفظ العبارة.";
}
