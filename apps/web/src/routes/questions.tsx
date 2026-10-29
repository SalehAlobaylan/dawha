import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowLeft, CircleHelp, MessageCircleQuestion, Plus, Search } from "lucide-react";
import { useState, type FormEvent } from "react";
import { createQuestion, fetchQuestions, fetchSessionStatus } from "../lib/api";
import { questionPriorityLabel, questionStatusLabel } from "../lib/question-labels";
import { StatusBadge } from "../components/StatusBadge";
import type { OpenQuestionRecord } from "../types";

const filters = [{ id: "all", label: "الكل" }, { id: "high", label: "عالية الأولوية" }, { id: "under_investigation", label: "قيد التحقيق" }, { id: "open", label: "مفتوحة" }, { id: "resolved", label: "محسومة مؤقتاً" }, { id: "archived", label: "مؤرشفة" }] as const;
export function QuestionsPage() {
  const search = useSearch({ from: "/research" });
  const navigate = useNavigate();
  const questionsQuery = useQuery({ queryKey: ["questions"], queryFn: fetchQuestions });
  const questions = questionsQuery.data ?? [];
  const query = search.create ? "" : search.q ?? "";
  const activeFilter = search.filter ?? "all";
  const visible = questions.filter((question) => question.titleAr.includes(query.trim()) && (activeFilter === "all" || activeFilter === "high" ? activeFilter === "all" || question.priority === "high" : question.status === activeFilter)).sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
  const high = questions.filter((question) => question.priority === "high" && question.status !== "archived").length;
  const count = (value: number) => questionsQuery.isSuccess ? value.toLocaleString("ar-SA") : "—";
  const update = (next: typeof search) => { void navigate({ to: "/research", search: next, replace: true, resetScroll: false }); };
  return <>
    <section className="questions-hero"><div><div className="eyebrow">دفتر الأسئلة</div><h2>ما الذي<br /><em>يستحق أن يُفتح؟</em></h2><p>{count(questions.length)} سؤالاً بين يديك الآن. بعضها قديم، وبعضها جديد، والجميع أكثر أمانة من إجابة متعجلة.</p><div className="questions-hero-actions"><button className="primary-button" type="button" onClick={() => update({ ...search, create: true })}><Plus size={15} /> افتح سؤالاً</button><span><MessageCircleQuestion size={14} /> {count(high)} تحتاج أولوية</span></div></div><div className="questions-orbit"><div className="question-orbit-ring ring-a" /><div className="question-orbit-ring ring-b" /><div className="question-orbit-core">؟</div><span className="orbit-label orbit-label-a">أدلة</span><span className="orbit-label orbit-label-b">فجوات</span><span className="orbit-label orbit-label-c">إمكانات</span></div></section>
    {search.create ? <NewQuestionForm key={search.q ?? "new"} initialTitle={search.q ?? ""} onCancel={() => update({ ...search, create: undefined, q: undefined })} /> : null}
    <section className="questions-index-section research-question-index"><div className="questions-index-head"><div><div className="eyebrow">دفتر البحث</div><h2>جميع أسئلتك البحثية</h2><p>تابع الحالة والأولوية، ثم افتح التحقيق لمراجعة أدلته وسجله.</p></div><label className="library-search"><Search size={15} /><input aria-label="البحث في الأسئلة" value={query} onChange={(event) => update({ ...search, q: event.target.value || undefined })} placeholder="ابحث في الأسئلة" /></label></div>
      <div className="question-filter-bar"><div className="question-filter-tabs">{filters.map((filter) => <button type="button" key={filter.id} aria-pressed={activeFilter === filter.id} className={activeFilter === filter.id ? "question-filter-active" : ""} onClick={() => update({ ...search, filter: filter.id === "all" ? undefined : filter.id })}>{filter.label}</button>)}</div><span className="question-sort">{count(visible.length)} سؤال</span></div>
      <div className="question-table"><div className="question-table-head"><span>السؤال</span><span>الحالة</span><span>الأولوية</span><span>آخر تحديث</span><span /></div>
        {questionsQuery.isPending ? <p className="question-empty" role="status">جارٍ تحميل الأسئلة…</p> : questionsQuery.error ? <div className="question-workspace-error" role="alert">تعذر تحميل دفتر البحث. <button className="inline-link" type="button" onClick={() => void questionsQuery.refetch()}>إعادة المحاولة</button></div> : visible.map((question) => <Link to="/research/$questionId" params={{ questionId: question.id }} className="question-table-row" key={question.id}><span className="question-table-title"><span className="question-table-icon"><CircleHelp size={15} /></span><span><strong>{question.titleAr}</strong><small>{question.claimCount} ادعاءات · {question.sourceCount} مصادر</small></span></span><StatusBadge tone={question.status === "under_investigation" ? "finding" : "question"} compact>{questionStatusLabel(question.status)}</StatusBadge><span className={`priority-label priority-${question.priority === "high" ? "high" : "normal"}`}>{questionPriorityLabel(question.priority)}</span><span className="question-table-updated">{new Date(question.updatedAt).toLocaleDateString("ar")}</span><span className="question-row-open">فتح التحقيق <ArrowLeft size={15} /></span></Link>)}
        {questionsQuery.isSuccess && !visible.length ? <div className="empty-search"><Search size={20} /><strong>{questions.length ? "لا توجد أسئلة بهذه المواصفات" : "دفتر البحث جاهز لسؤالك الأول"}</strong><span>{questions.length ? "وسّع البحث أو غيّر التصفية." : "افتح سؤالاً محدداً، ثم اجمع مصادره وقراراته في تحقيق واحد."}</span>{questions.length ? <button className="inline-link" type="button" onClick={() => update({})}>عرض جميع الأسئلة</button> : null}</div> : null}
      </div>
    </section>
  </>;
}

function NewQuestionForm({ initialTitle, onCancel }: { initialTitle: string; onCancel: () => void }) {
  const [title, setTitle] = useState(initialTitle);
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<OpenQuestionRecord["priority"]>("normal");
  const session = useQuery({ queryKey: ["workspace-session"], queryFn: fetchSessionStatus });
  const navigate = useNavigate();
  const client = useQueryClient();
  const mutation = useMutation({ mutationFn: () => createQuestion({ title_ar: title.trim(), description_ar: description.trim() || undefined, priority }), onSuccess: async (created) => { await client.invalidateQueries({ queryKey: ["questions"] }); await navigate({ to: "/research/$questionId", params: { questionId: created.question.id } }); } });
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  return <form className="question-create-form research-create-question" onSubmit={submit}><h2>سؤال بحثي جديد</h2><div className="question-form-grid"><label className="composer-label">صياغة السؤال<input autoFocus required maxLength={500} value={title} onChange={(event) => setTitle(event.target.value)} placeholder="ما الذي تريد التحقق منه؟" /></label><label className="composer-label">الأولوية<select value={priority} onChange={(event) => setPriority(event.target.value as typeof priority)}><option value="low">منخفضة</option><option value="normal">عادية</option><option value="high">عالية</option></select></label><label className="composer-label">السياق والفجوة<textarea rows={3} value={description} onChange={(event) => setDescription(event.target.value)} placeholder="ما الذي تقوله الأدلة، وما الذي ما زال مفتوحاً؟" /></label></div>{mutation.error ? <p role="alert">{mutation.error instanceof Error ? mutation.error.message : "تعذر حفظ السؤال."}</p> : null}{session.data === false ? <p>سجّل الدخول لحفظ السؤال. <a className="inline-link" href="/login?returnTo=%2Fresearch%3Fcreate%3Dtrue">تسجيل الدخول</a></p> : null}<div className="composer-actions"><button className="primary-button" type="submit" disabled={mutation.isPending || session.data !== true}>{mutation.isPending ? "جارٍ الحفظ…" : "احفظ وافتح التحقيق"}</button><button className="secondary-button" type="button" onClick={onCancel}>إلغاء</button></div></form>;
}
