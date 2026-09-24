import { Link } from "@tanstack/react-router";
import { ArrowLeft, BookOpen, CheckCircle2, CircleAlert, FileSearch, Filter, GitBranch, Plus, Search, Send, Sparkles } from "lucide-react";
import { useMemo, useState } from "react";
import { sources } from "../data/demo";
import { EvidenceComparison, SourceCard } from "../components/EvidencePanels";
import { SectionHeading } from "../components/SectionHeading";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

const researchTabs = ["الكل", "ادعاءات", "مصادر", "أسئلة", "ملاحظات النظام"];

export function ResearchPage() {
  const [activeTab, setActiveTab] = useState("الكل");
  const [search, setSearch] = useState("");
  const [composerOpen, setComposerOpen] = useState(false);
  const visibleSources = useMemo(() => {
    const normalized = search.trim();
    if (!normalized) return sources.slice(0, 3);
    return sources.filter((source) => `${source.title} ${source.excerpt} ${source.type}`.includes(normalized)).slice(0, 3);
  }, [search]);

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="مكتب البحث / مساحة نجم"
        title="ابنِ سياقك، خطوة خطوة"
        description="اجمع السؤال، المصادر، الادعاءات، والأدلة المضادة في مساحة واحدة قابلة للتتبع."
      />

      <section className="research-command-bar">
        <div className="research-command-copy"><Sparkles size={17} /><span>مساعد البحث يسأل ويقترح، لكنه لا يحسم.</span></div>
        <button className="secondary-button" type="button" onClick={() => setComposerOpen((open) => !open)}><Plus size={15} /> إضافة ادعاء</button>
        <button className="primary-button" type="button" onClick={() => setComposerOpen((open) => !open)}><Search size={15} /> ابدأ من سؤال</button>
      </section>

      {composerOpen ? (
        <section className="claim-composer">
          <div className="composer-head"><div><div className="eyebrow">ادعاء جديد</div><h2>اكتب الفرضية قبل أن تجد الدليل</h2></div><StatusBadge tone="claim">غير محسوم بعد</StatusBadge></div>
          <label className="composer-label">صياغة الادعاء<input placeholder="مثال: عبدالله ابن محمد بن سعد" /></label>
          <div className="composer-grid"><label className="composer-label">الموضوع<select defaultValue="person"><option value="person">شخص</option><option value="family">عائلة</option><option value="tribe">قبيلة</option></select></label><label className="composer-label">الحالة الأولية<select defaultValue="unresolved"><option value="unresolved">غير محسوم</option><option value="supported">مدعوم مبدئياً</option><option value="disputed">متنازع عليه</option></select></label></div>
          <div className="composer-actions"><button className="secondary-button" type="button" onClick={() => setComposerOpen(false)}>إلغاء</button><button className="primary-button" type="button" onClick={() => setComposerOpen(false)}>حفظ كمسودة <Send size={14} /></button></div>
        </section>
      ) : null}

      <div className="research-tabs" role="tablist" aria-label="أنواع المواد">
        {researchTabs.map((tab) => <button key={tab} type="button" className={`research-tab${activeTab === tab ? " research-tab-active" : ""}`} onClick={() => setActiveTab(tab)}>{tab}{tab === "ادعاءات" ? <span>24</span> : tab === "أسئلة" ? <span>89</span> : null}</button>)}
      </div>

      <div className="research-layout">
        <main className="research-main-column">
          <section className="research-focus-card">
            <div className="research-focus-head"><div><div className="eyebrow">السؤال النشط</div><h2>من كان والد عبدالله في هذه الروايات؟</h2></div><StatusBadge tone="question">قيد التحقيق</StatusBadge></div>
            <p className="research-focus-lead">المصادر المتاحة لا تحسم اسم الأب. هل توجد علاقة اعتماد بين المصدرين، أم روايتان مستقلتان؟</p>
            <div className="focus-claims">
              <div className="focus-claim focus-claim-supported"><div className="focus-claim-top"><StatusBadge tone="claim" compact>ادعاء ١</StatusBadge><span>مدعوم مبدئياً</span></div><strong>محمد بن سعد</strong><p>المصدر الأول يذكر أن عبدالله يتصل بمحمد.</p><div className="focus-claim-footer"><span><BookOpen size={13} /> مصدر أولي</span><span>١ من ٢</span></div></div>
              <div className="focus-claim focus-claim-disputed"><div className="focus-claim-top"><StatusBadge tone="disputed" compact>ادعاء ٢</StatusBadge><span>متنازع عليه</span></div><strong>صالح</strong><p>رواية لاحقة تذكر اسماً والداً آخر، مع شبهة اعتماد.</p><div className="focus-claim-footer"><span><CircleAlert size={13} /> لم يُتحقق من الاستقلال</span><span>١ من ٢</span></div></div>
            </div>
            <div className="focus-actions"><button className="primary-button" type="button"><FileSearch size={15} /> ابحث عن مصدر جديد</button><button className="secondary-button" type="button"><GitBranch size={15} /> اعرض المسار</button><Link to="/questions" className="text-button">كل الأسئلة <ArrowLeft size={14} /></Link></div>
          </section>

          <EvidenceComparison />

          <section className="research-sources-section">
            <SectionHeading eyebrow="المصادر القريبة" title="ما يمكن فتحه والتحقق منه" description="الاقتباس الأصلي يبقى مرجعاً، حتى لو أضاف النظام ملاحظة." action="فتح المكتبة" />
            <div className="source-list">{visibleSources.map((source) => <SourceCard key={source.id} source={source} />)}</div>
            {visibleSources.length === 0 ? <div className="empty-search"><FileSearch size={18} /><span>لم نجد مصدراً بهذه العبارة. جرّب كلمة أقصر أو ابدأ سؤالاً جديداً.</span></div> : null}
          </section>
        </main>

        <aside className="research-side-column">
          <section className="research-side-panel research-filter-panel">
            <div className="side-panel-head"><div className="eyebrow">رشّح</div><Filter size={16} /></div>
            <h3>رشّح المواد</h3>
            <label className="small-search"><Search size={14} /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="ابحث في المصادر" /></label>
            <div className="side-filter-group"><span>نوع المصدر</span><label><input type="checkbox" defaultChecked /> مخطوطات وكتب <b>842</b></label><label><input type="checkbox" defaultChecked /> سجلات وأرشيف <b>211</b></label><label><input type="checkbox" /> أشجار منشورة <b>376</b></label></div>
            <div className="side-filter-group"><span>حالة المراجعة</span><label><input type="checkbox" defaultChecked /> يحتاج مراجعة <b>17</b></label><label><input type="checkbox" /> مفهرس <b>1,102</b></label></div>
          </section>
          <section className="research-side-panel next-step-panel"><div className="side-panel-head"><div className="eyebrow">اقتراح بحثي</div><Sparkles size={16} /></div><h3>ماذا تفحص بعد ذلك؟</h3><p>ابحث عن إشارات إلى الاعتماد بين «مجموع المصادر» و«تاريخ قبائل نجد»، قبل اعتبارهما دليلين مستقلين.</p><div className="next-step-list"><span><CheckCircle2 size={14} /> قارن صياغة الفقرتين</span><span><CheckCircle2 size={14} /> راجع تاريخ التأليف</span><span><CheckCircle2 size={14} /> افحص إحالات المصدر الثاني</span></div><button className="secondary-button" type="button"><Plus size={14} /> أضف إلى السؤال</button></section>
          <section className="research-side-panel activity-side-panel"><div className="side-panel-head"><div className="eyebrow">سجل النشاط</div><span className="live-indicator"><i /> مباشر</span></div><div className="activity-side-list"><div><span className="activity-side-dot source" /><p><strong>سُجلت</strong> عبارة من المصدر ١<small>منذ ١٢ دقيقة</small></p></div><div><span className="activity-side-dot finding" /><p><strong>أضاف النظام</strong> ملاحظة تعارض<small>منذ ساعة</small></p></div><div><span className="activity-side-dot question" /><p><strong>فُتح السؤال</strong> للمراجعة<small>منذ ساعتين</small></p></div></div></section>
        </aside>
      </div>
    </div>
  );
}
