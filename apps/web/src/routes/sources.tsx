import { Link } from "@tanstack/react-router";
import { ArrowLeft, BookOpen, CheckCircle2, FileStack, Search, SlidersHorizontal, UploadCloud } from "lucide-react";
import { useMemo, useState } from "react";
import { sources } from "../data/demo";
import { SourceCard } from "../components/EvidencePanels";
import { SectionHeading } from "../components/SectionHeading";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

const sourceTypes = ["الكل", "مخطوط", "كتاب مطبوع", "سجل أرشيفي", "شجرة منشورة"];

export function SourcesPage() {
  const [query, setQuery] = useState("");
  const [type, setType] = useState("الكل");
  const filteredSources = useMemo(() => {
    const normalized = query.trim();
    return sources.filter((source) => {
      const matchesType = type === "الكل" || source.type === type;
      const matchesQuery = !normalized || `${source.title} ${source.excerpt} ${source.locator}`.includes(normalized);
      return matchesType && matchesQuery;
    });
  }, [query, type]);

  return (
    <div className="page-stack">
      <TopBar eyebrow="المكتبة / المصادر" title="المصدر أولاً" description="ما الذي يقوله النص بالضبط؟ افتح الاقتباس، ثم احكم على الادعاء الذي بُني فوقه." />
      <section className="source-library-hero">
        <div className="source-library-copy"><div className="eyebrow">مكتبة المصادر</div><h2>لا نخلط بين النص<br /><em>وما استنتجناه منه.</em></h2><p>كل مصدر يحتفظ بموضعه ونصه، ويظل الاقتباس جزءاً من قوة الدليل، لا عنصراً ثانوياً.</p><div className="library-hero-actions"><button className="primary-button" type="button"><UploadCloud size={15} /> أضف مصدراً</button><span><BookOpen size={14} /> 1,248 مصدراً مفهرساً</span></div></div>
        <div className="source-stack-visual"><div className="source-sheet source-sheet-back" /><div className="source-sheet source-sheet-middle" /><div className="source-sheet source-sheet-front"><span>المصدر</span><strong>ما ورد فيه</strong><i /></div><span className="source-visual-caption">طبقة النص<br />أصل الحجة</span></div>
      </section>

      <section className="source-stats-row"><div><strong>842</strong><span>مخطوطات وكتب</span></div><div><strong>211</strong><span>سجلات وأرشيف</span></div><div><strong>17</strong><span>مصدر يحتاج مراجعة</span></div><div><strong>94%</strong><span>لديها موضع اقتباس</span></div><StatusBadge tone="source">المصدر لا يصبح حقيقة تلقائياً</StatusBadge></section>

      <section className="source-library-section">
        <SectionHeading eyebrow="الفهرس" title="تصفّح المصادر" description="استخدم النص الأصلي والاسم المختصر معاً، فالتطابق اللغوي لا يكفي لتثبيت الهوية." />
        <div className="source-library-toolbar"><label className="library-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="ابحث بالعنوان، الاقتباس، أو الموضع" /></label><div className="source-type-tabs">{sourceTypes.map((item) => <button key={item} type="button" className={type === item ? "source-type-active" : ""} onClick={() => setType(item)}>{item}</button>)}</div><button className="icon-button" type="button" aria-label="خيارات التصفية"><SlidersHorizontal size={17} /></button></div>
        <div className="source-library-layout"><aside className="source-filter-aside"><div className="eyebrow">تصفية</div><h3>نوع المصدر</h3>{["مخطوط", "كتاب مطبوع", "سجل أرشيفي", "شجرة منشورة", "رسالة"].map((item, index) => <label className="aside-check" key={item}><input type="checkbox" defaultChecked={index < 3} />{item}<span>{[248, 194, 211, 376, 109][index]}</span></label>)}<div className="aside-rule" /><div className="eyebrow">حالة المصدر</div><label className="aside-check"><input type="checkbox" />مفهرس</label><label className="aside-check"><input type="checkbox" defaultChecked />قيد المراجعة</label><label className="aside-check"><input type="checkbox" />مشتق أو معتمد</label></aside><div className="source-results"><div className="results-summary"><span>{filteredSources.length} من 1,248 مصدراً</span><span>مرتبة حسب الصلة</span></div>{filteredSources.map((source) => <SourceCard source={source} key={source.id} />)}{filteredSources.length === 0 ? <div className="empty-search"><FileStack size={20} /><strong>لا توجد نتائج مطابقة</strong><span>جرّب توسيع البحث أو إزالة بعض الكلمات.</span></div> : null}</div></div>
      </section>

      <section className="source-principles"><div className="principle-mark"><CheckCircle2 size={20} /></div><div><div className="eyebrow">قاعدة المكتبة</div><h3>المصدر قد يكون قديماً أو مشتقاً، ولا يزال يستحق الفهم.</h3><p>نعرض درجة الاعتماد كإشارة قابلة للمراجعة، ولا نحذف المصدر لمجرد أن له شبهة.</p></div><Link to="/research" className="inline-link">افتح ملف البحث <ArrowLeft size={14} /></Link></section>
    </div>
  );
}
