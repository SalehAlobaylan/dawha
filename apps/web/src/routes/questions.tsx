import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, CircleHelp, Filter, MessageCircleQuestion, Plus, Search, SlidersHorizontal, Sparkles } from "lucide-react";
import { useMemo, useState } from "react";
import { fetchQuestions } from "../lib/api";
import { questionPriorityLabel, questionStatusLabel } from "../lib/question-labels";
import { QuestionWorkspace } from "../components/QuestionWorkspace";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

/**
 * The index reads the questions it lists from the API.
 *
 * It used to read them from `demoDashboard.openQuestions`, which meant the table
 * showed three invented questions with invented ids, and the four metric tiles
 * showed invented counts, on a page whose workspace underneath was already
 * talking to the database. A reader saw a real workspace and an invented index
 * side by side, and the index was the one that looked like a summary of the
 * work.
 *
 * A failure now shows the failure. It does not fall back to the demo, because a
 * count that did not come from the database is not a count about the research
 * (`routes/index.tsx` says the same thing about the dashboard, for the same
 * reason).
 */
const filters = [
  { id: "all", label: "الكل" },
  { id: "high", label: "عالية الأولوية" },
  { id: "under_investigation", label: "قيد التحقيق" },
  { id: "open", label: "مفتوحة" },
  { id: "archived", label: "مؤرشفة" },
] as const;

type FilterId = (typeof filters)[number]["id"];

const countFormat = new Intl.NumberFormat("ar-EG");

export function QuestionsPage() {
  const [activeFilter, setActiveFilter] = useState<FilterId>("all");
  const [query, setQuery] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  // The same query key the workspace uses, so the two share one request and one
  // cache entry instead of racing each other into two different lists.
  const questionsQuery = useQuery({ queryKey: ["questions"], queryFn: fetchQuestions });
  const questions = questionsQuery.data;

  const visible = useMemo(() => {
    const all = questions ?? [];
    const normalized = query.trim();
    return all.filter((question) => {
      const matchesQuery = !normalized || question.titleAr.includes(normalized);
      const matchesFilter =
        activeFilter === "all"
          ? true
          : activeFilter === "high"
            ? question.priority === "high"
            : question.status === activeFilter;
      return matchesQuery && matchesFilter;
    });
  }, [activeFilter, query, questions]);

  const metrics = useMemo(() => {
    const all = questions ?? [];
    return {
      total: all.length,
      high: all.filter((question) => question.priority === "high").length,
      underInvestigation: all.filter((question) => question.status === "under_investigation").length,
      archived: all.filter((question) => question.status === "archived").length,
      createdThisMonth: all.filter((question) => createdThisMonth(question.createdAt)).length,
    };
  }, [questions]);

  // The hero's button opens the one create form, which lives in the workspace
  // below, so the page scrolls to it rather than opening a second form here.
  const openCreateForm = () => {
    setCreateOpen(true);
    document.getElementById("question-workspace")?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  return (
    <div className="page-stack">
      <TopBar eyebrow="الأسئلة / البحث المفتوح" title="الخلاف مساحة، لا مشكلة" description="الأسئلة المفتوحة تحفظ ما لا يستطيع الدليل الحالي حسمه، وتمنح البحث اتجاهه." />
      <section className="questions-hero"><div><div className="eyebrow">دفتر الأسئلة</div><h2>ما الذي<br /><em>يستحق أن يُفتح؟</em></h2><p>{formatCount(metrics.total)} سؤالاً بين يديك الآن. بعضها قديم، وبعضها جديد، والجميع أكثر أمانة من إجابة متعجلة.</p><div className="questions-hero-actions"><button className="primary-button" type="button" onClick={openCreateForm}><Plus size={15} /> افتح سؤالاً</button><span><MessageCircleQuestion size={14} /> {formatCount(metrics.high)} تحتاج أولوية</span></div></div><div className="questions-orbit"><div className="question-orbit-ring ring-a" /><div className="question-orbit-ring ring-b" /><div className="question-orbit-core">؟</div><span className="orbit-label orbit-label-a">أدلة</span><span className="orbit-label orbit-label-b">فجوات</span><span className="orbit-label orbit-label-c">إمكانات</span></div></section>

      <QuestionWorkspace createOpen={createOpen} onCreateOpenChange={setCreateOpen} />

      <section className="question-metrics"><div><strong>{formatCount(metrics.total)}</strong><span>سؤال مفتوح</span><small>{formatCount(metrics.createdThisMonth)} هذا الشهر</small></div><div><strong>{formatCount(metrics.high)}</strong><span>عالية الأولوية</span><small>تحتاج باحثاً</small></div><div><strong>{formatCount(metrics.underInvestigation)}</strong><span>تحت التحقيق</span><small>نشاط حديث</small></div><div><strong>{formatCount(metrics.archived)}</strong><span>مؤرشفة</span><small>يمكن إعادة فتحها</small></div><div className="question-metric-note"><Sparkles size={17} /><span>السؤال الجيد لا يغلق الباب؛ يجعل الخطوة التالية واضحة.</span></div></section>

      <section className="questions-index-section"><div className="questions-index-head"><div><div className="eyebrow">قائمة البحث</div><h2>أسئلة مفتوحة</h2></div><div className="questions-index-tools"><label className="library-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="ابحث في الأسئلة" /></label><button className="icon-button" type="button" aria-label="تصفية"><SlidersHorizontal size={17} /></button></div></div><div className="question-filter-bar"><div className="question-filter-tabs">{filters.map((filter) => <button type="button" key={filter.id} className={activeFilter === filter.id ? "question-filter-active" : ""} onClick={() => setActiveFilter(filter.id)}>{filter.label}</button>)}</div><span className="question-sort"><Filter size={14} /> مرتبة حسب آخر تحديث</span></div><div className="question-table"><div className="question-table-head"><span>السؤال</span><span>الحالة</span><span>الأولوية</span><span>النشاط</span><span /></div>{questionsQuery.isPending ? <p className="question-empty">جارٍ التحميل…</p> : null}{questionsQuery.isError ? <div className="question-workspace-error" role="alert">{errorMessage(questionsQuery.error)}</div> : null}{questionsQuery.isSuccess ? visible.map((question) => <Link to="/research/$questionId" params={{ questionId: question.id }} className="question-table-row" key={question.id}><span className="question-table-title"><span className="question-table-icon"><CircleHelp size={15} /></span><span><strong>{question.titleAr}</strong><small>{question.claimCount} ادعاءات مرتبطة</small></span></span><StatusBadge tone={question.status === "under_investigation" ? "finding" : "question"} compact>{questionStatusLabel(question.status)}</StatusBadge><span className={`priority-label priority-${question.priority === "high" ? "high" : "normal"}`}>{questionPriorityLabel(question.priority)}</span><span className="question-table-updated">{new Date(question.updatedAt).toLocaleDateString("ar")}</span><ArrowLeft size={15} className="question-row-arrow" /></Link>) : null}{questionsQuery.isSuccess && visible.length === 0 ? <div className="empty-search"><Search size={20} /><strong>لا يوجد سؤال بهذه المواصفات</strong><span>أعد توسيع البحث أو افتح سؤالاً جديداً.</span></div> : null}</div></section>

      <section className="question-workflow"><div className="workflow-copy"><div className="eyebrow">كيف يعمل السؤال؟</div><h2>افتح، قارن، ارجع.</h2><p>لا تنتهي المهمة بإجابة. تنتهي بما إذا كانت الأدلة كافية، وما الخطوة التي تستحق التسجيل.</p><Link to="/research" className="inline-link">ابدأ تحقيقاً <ArrowLeft size={14} /></Link></div><div className="workflow-steps"><div><span>٠١</span><strong>اطرح</strong><small>صيغة قابلة للبحث</small></div><div><span>٠٢</span><strong>اربط</strong><small>ادعاءات وأدلة وخلافات</small></div><div><span>٠٣</span><strong>احتفظ</strong><small>بالتاريخ والفجوات</small></div></div></section>
    </div>
  );
}

function formatCount(value: number): string {
  return countFormat.format(value);
}

function createdThisMonth(value: string): boolean {
  const created = new Date(value);
  if (Number.isNaN(created.getTime())) return false;
  const now = new Date();
  return created.getFullYear() === now.getFullYear() && created.getMonth() === now.getMonth();
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر الاتصال بالخدمة.";
}
