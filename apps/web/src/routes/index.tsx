import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Check, GitBranch, Sparkles } from "lucide-react";
import { demoDashboard, places } from "../data/demo";
import { fetchDashboard } from "../lib/api";
import { ActivityFeed, LayerStack, OpenQuestionsPreview } from "../components/ResearchPanels";
import { MapPreview } from "../components/MapPreview";
import { SectionHeading } from "../components/SectionHeading";
import { TopBar } from "../components/TopBar";
import { TreeWorkspace } from "../components/TreeWorkspace";

/**
 * The landing page.
 *
 * Two states, and the difference between them is the whole point of this
 * component:
 *
 *  - `mode: "demo"` - the numbers are synthetic, and a banner says so on screen
 *    before any of them is read. The data is still rendered, because a demo you
 *    cannot look at teaches nobody anything; it is labelled, so nobody mistakes it
 *    for a workspace.
 *  - a failure - no numbers at all, and a panel that says the dashboard is
 *    unavailable. NOT the demo. A deployment whose database is down must not show
 *    a confident invented workspace, and the previous version of this page did
 *    exactly that, because `useQuery({ data = demoDashboard })` substituted a
 *    default for an error the user was never told about.
 */
export function HomePage() {
  const { data, isError, error, isPending } = useQuery({ queryKey: ["dashboard"], queryFn: fetchDashboard });

  if (isError) {
    return (
      <div className="page-stack">
        <TopBar
          eyebrow="الثلاثاء، ١٦ ربيع الآخر ١٤٤٨هـ"
          title="صباح البحث، نجم"
          description="من هنا تبدأ مساحة العمل التي تحفظ الأدلة، والخلافات، وما لم يُحسم بعد."
        />
        <section className="panel dashboard-unavailable" role="alert">
          <div className="dashboard-unavailable-mark" aria-hidden="true">؟</div>
          <h2>لوحة المساحة غير متاحة</h2>
          <p>
            لم نتمكن من قراءة بيانات المساحة، ولم نعرض بيانات توضيحية مكانها. رقم لا
            يأتي من قاعدة البيانات ليس رقماً عن بحثك.
          </p>
          <p className="dashboard-unavailable-detail">
            {error instanceof Error ? error.message : "تعذر الاتصال بالخدمة."}
          </p>
        </section>
      </div>
    );
  }

  // Pending shows the demo only when there is no data at all yet, and it is
  // labelled exactly as the demo is everywhere else.
  const dashboard = data ?? demoDashboard;
  const isDemo = isPending || dashboard.mode === "demo";

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="الثلاثاء، ١٦ ربيع الآخر ١٤٤٨هـ"
        title="صباح البحث، نجم"
        description="من هنا تبدأ مساحة العمل التي تحفظ الأدلة، والخلافات، وما لم يُحسم بعد."
      />

      {isDemo ? (
        <section className="demo-banner" role="status" data-testid="demo-banner">
          <strong>بيانات توضيحية</strong>
          <span>
            هذه أرقام من مساحة نموذج، وليست من بحثك. اضبط <code>DEMO_MODE=false</code>{" "}
            لإخفاءها.
          </span>
        </section>
      ) : null}

      <section className="home-hero">
        <div className="hero-copy">
          <div className="hero-kicker"><span className="hero-kicker-line" /> منصة بحث النسب العربية</div>
          <h2>السلالةُ لا تُقرأ<br /><em>في سطر واحد.</em></h2>
          <p>تتبّع ما يقوله المصدر، وما يدّعيه الباحث، وما تمثله الشجرة، وما يبقى مفتوحاً — دون أن تُجبر التاريخ على جواب واحد.</p>
          <div className="hero-actions">
            <Link to="/research" className="primary-button primary-button-large">ابدأ بحثاً <ArrowLeft size={17} /></Link>
            <Link to="/tree" className="secondary-button secondary-button-large">استكشف شجرة <GitBranch size={16} /></Link>
          </div>
          <div className="hero-footnote"><Check size={14} /> مساحة آمنة للفصل بين المعرفة المؤكدة وادعاءات البحث.</div>
        </div>
        <div className="hero-visual" aria-label="طبقات معرفة دَوْحة">
          <div className="hero-visual-header">
            <span>دفتر بحث / ٠١</span>
            <span className="hero-visual-status"><i /> يعمل</span>
          </div>
          <div className="hero-visual-center">
            <div className="orbit orbit-one" />
            <div className="orbit orbit-two" />
            <div className="hero-visual-symbol">؟</div>
            <div className="visual-annotation annotation-source">المصدر</div>
            <div className="visual-annotation annotation-claim">ادعاء</div>
            <div className="visual-annotation annotation-tree">شجرة</div>
            <div className="visual-annotation annotation-question">سؤال</div>
          </div>
          <div className="hero-visual-bottom">
            <div><span className="visual-bottom-label">المبدأ</span><strong>أقرب إلى الأدلة، أبعد عن التسرّع.</strong></div>
            <Sparkles size={17} />
          </div>
        </div>
      </section>

      <section className="metrics-grid" aria-label="مؤشرات المساحة">
        {dashboard.metrics.map((metric, index) => (
          <article className={`metric-card metric-card-${metric.tone}${index === 0 ? " metric-card-featured" : ""}`} key={metric.label}>
            <div className="metric-card-top"><span>{metric.label}</span><span className="metric-index">٠{index + 1}</span></div>
            <strong>{metric.value}</strong>
            <div className="metric-card-bottom"><span>{metric.detail}</span><span className="metric-spark">↗</span></div>
          </article>
        ))}
      </section>

      <section className="home-grid home-grid-primary">
        <div className="panel activity-panel">
          <SectionHeading eyebrow="نبض البحث" title="آخر ما يستحق النظر" description="مواد جديدة، أو قديمة، لكنها تفتح سؤالاً جديداً." action="كل النشاط" />
          <ActivityFeed activities={dashboard.activity} />
        </div>
        <LayerStack data={dashboard} />
      </section>

      <OpenQuestionsPreview data={dashboard} />

      <TreeWorkspace data={dashboard} />

      <MapPreview places={places} />

      <section className="research-invite" id="dictionary">
        <div className="research-invite-pattern" />
        <div className="research-invite-copy">
          <div className="eyebrow">ابنِ سياقك الخاص</div>
          <h2>لكل بحث أثر.<br /><span>ولكل أثر مصدر.</span></h2>
          <p>ابدأ من شجرة منشورة، أو من سؤال لم يجد أحد جوابه بعد. دَوْحة تحفظ الطريق بينهما.</p>
          <div className="research-invite-actions">
            <Link to="/research" className="primary-button primary-button-large">افتح مكتب البحث <ArrowLeft size={16} /></Link>
            <Link to="/sources" className="text-button">تصفّح المصادر <ArrowLeft size={15} /></Link>
          </div>
        </div>
        <div className="research-invite-stats">
          <div><strong>٠٥</strong><span>طبقات معرفية</span></div>
          <div><strong>١٧</strong><span>ادعاءً متنازعاً عليه</span></div>
          <div><strong>∞</strong><span>سؤال يستحق المتابعة</span></div>
        </div>
      </section>

      <footer className="site-footer">
        <div className="footer-brand"><span className="footer-mark">د</span><span>دَوْحة</span></div>
        <p>نحفظ الأسئلة بقدر ما نحفظ الإجابات.</p>
        <div className="footer-links"><a href="#privacy">الخصوصية</a><a href="#sources">المصادر</a><a href="#status">حالة المنصة</a></div>
      </footer>
    </div>
  );
}
