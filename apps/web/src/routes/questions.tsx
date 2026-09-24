import { Link } from "@tanstack/react-router";
import { ArrowLeft, CircleHelp, Filter, MessageCircleQuestion, Plus, Search, SlidersHorizontal, Sparkles } from "lucide-react";
import { useMemo, useState } from "react";
import { demoDashboard } from "../data/demo";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

const filters = ["الكل", "عالية الأولوية", "قيد التحقيق", "مفتوحة", "مؤرشفة"];

export function QuestionsPage() {
  const [activeFilter, setActiveFilter] = useState("الكل");
  const [query, setQuery] = useState("");
  const questions = useMemo(() => {
    const normalized = query.trim();
    return demoDashboard.openQuestions.filter((question) => {
      const matchesQuery = !normalized || question.title.includes(normalized);
      const matchesFilter = activeFilter === "الكل" || (activeFilter === "عالية الأولوية" ? question.priority === "عالية" : question.status === activeFilter);
      return matchesQuery && matchesFilter;
    });
  }, [activeFilter, query]);

  return (
    <div className="page-stack">
      <TopBar eyebrow="الأسئلة / البحث المفتوح" title="الخلاف مساحة، لا مشكلة" description="الأسئلة المفتوحة تحفظ ما لا يستطيع الدليل الحالي حسمه، وتمنح البحث اتجاهه." />
      <section className="questions-hero"><div><div className="eyebrow">دفتر الأسئلة</div><h2>ما الذي<br /><em>يستحق أن يُفتح؟</em></h2><p>89 سؤالاً بين يديك الآن. بعضها قديم، وبعضها جديد، والجميع أكثر أمانة من إجابة متعجلة.</p><div className="questions-hero-actions"><button className="primary-button" type="button"><Plus size={15} /> افتح سؤالاً</button><span><MessageCircleQuestion size={14} /> 17 تحتاج أولوية</span></div></div><div className="questions-orbit"><div className="question-orbit-ring ring-a" /><div className="question-orbit-ring ring-b" /><div className="question-orbit-core">؟</div><span className="orbit-label orbit-label-a">أدلة</span><span className="orbit-label orbit-label-b">فجوات</span><span className="orbit-label orbit-label-c">إمكانات</span></div></section>

      <section className="question-metrics"><div><strong>٨٩</strong><span>سؤال مفتوح</span><small>+6 هذا الشهر</small></div><div><strong>١٧</strong><span>عالية الأولوية</span><small>تحتاج باحثاً</small></div><div><strong>٢٣</strong><span>تحت التحقيق</span><small>نشاط حديث</small></div><div><strong>٤١</strong><span>مؤرشفة</span><small>يمكن إعادة فتحها</small></div><div className="question-metric-note"><Sparkles size={17} /><span>السؤال الجيد لا يغلق الباب؛ يجعل الخطوة التالية واضحة.</span></div></section>

      <section className="questions-index-section"><div className="questions-index-head"><div><div className="eyebrow">قائمة البحث</div><h2>أسئلة مفتوحة</h2></div><div className="questions-index-tools"><label className="library-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="ابحث في الأسئلة" /></label><button className="icon-button" type="button" aria-label="تصفية"><SlidersHorizontal size={17} /></button></div></div><div className="question-filter-bar"><div className="question-filter-tabs">{filters.map((filter) => <button type="button" key={filter} className={activeFilter === filter ? "question-filter-active" : ""} onClick={() => setActiveFilter(filter)}>{filter}</button>)}</div><span className="question-sort"><Filter size={14} /> مرتبة حسب الأولوية</span></div><div className="question-table"><div className="question-table-head"><span>السؤال</span><span>الحالة</span><span>الأولوية</span><span>النشاط</span><span /></div>{questions.map((question) => <Link to="/research" className="question-table-row" key={question.id}><span className="question-table-title"><span className="question-table-icon"><CircleHelp size={15} /></span><span><strong>{question.title}</strong><small>{question.claimCount} ادعاءات مرتبطة</small></span></span><StatusBadge tone={question.status === "قيد التحقيق" ? "finding" : "question"} compact>{question.status}</StatusBadge><span className={`priority-label priority-${question.priority === "عالية" ? "high" : "normal"}`}>{question.priority}</span><span className="question-table-updated">{question.updatedAt}</span><ArrowLeft size={15} className="question-row-arrow" /></Link>)}{questions.length === 0 ? <div className="empty-search"><Search size={20} /><strong>لا يوجد سؤال بهذه المواصفات</strong><span>أعد توسيع البحث أو افتح سؤالاً جديداً.</span></div> : null}</div></section>

      <section className="question-workflow"><div className="workflow-copy"><div className="eyebrow">كيف يعمل السؤال؟</div><h2>افتح، قارن، ارجع.</h2><p>لا تنتهي المهمة بإجابة. تنتهي بما إذا كانت الأدلة كافية، وما الخطوة التي تستحق التسجيل.</p><Link to="/research" className="inline-link">ابدأ تحقيقاً <ArrowLeft size={14} /></Link></div><div className="workflow-steps"><div><span>٠١</span><strong>اطرح</strong><small>صيغة قابلة للبحث</small></div><div><span>٠٢</span><strong>اربط</strong><small>ادعاءات وأدلة وخلافات</small></div><div><span>٠٣</span><strong>احتفظ</strong><small>بالتاريخ والفجوات</small></div></div></section>
    </div>
  );
}
