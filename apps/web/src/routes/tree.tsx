import { Link } from "@tanstack/react-router";
import { ArrowLeft, GitCompareArrows, LockKeyhole, Share2, SlidersHorizontal } from "lucide-react";
import { useState } from "react";
import { treeNodes } from "../data/demo";
import { EvidenceMiniList, ResearchGraph } from "../components/ResearchGraph";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

export function TreePage() {
  const [selectedId, setSelectedId] = useState("p-2");
  const [showUnresolved, setShowUnresolved] = useState(true);
  const selected = treeNodes.find((node) => node.id === selectedId) ?? treeNodes[1];

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="شجرة بحث / تفسير منشور"
        title="شجرة بيت العنبر"
        description="نسخة v3 محفوظة بتاريخ ١٦ ربيع الآخر ١٤٤٨هـ. افتح علاقة لترى من أين جاءت."
      />
      <div className="tree-page-toolbar">
        <div className="toolbar-breadcrumb"><span>الأشجار</span><b>/</b><strong>شجرة بيت العنبر</strong></div>
        <div className="toolbar-button-group">
          <button className="secondary-button" type="button"><Share2 size={15} /> مشاركة</button>
          <button className="secondary-button" type="button"><GitCompareArrows size={15} /> مقارنة النسخ</button>
          <button className="primary-button" type="button">فتح البحث <ArrowLeft size={15} /></button>
        </div>
      </div>

      <div className="tree-layout">
        <section className="tree-explorer-panel">
          <div className="tree-explorer-head">
            <div>
              <div className="tree-status-line"><StatusBadge tone="interpretation">منشورة</StatusBadge><span>النسخة 3 من 3</span></div>
              <h2>الشجرة كما نشرها صاحبها</h2>
              <p>هذه العلاقات تشرح هذا العرض، ولا تمثل حقيقة تاريخية نهائية.</p>
            </div>
            <button className="icon-button" type="button" aria-label="فلترة الشجرة"><SlidersHorizontal size={17} /></button>
          </div>
          <div className="tree-filter-row">
            <button className={`filter-chip${showUnresolved ? " filter-chip-active" : ""}`} type="button" onClick={() => setShowUnresolved((visible) => !visible)}>
              <span className="filter-chip-dot filter-dot-disputed" /> إظهار غير المحسوم
            </button>
            <span className="filter-count">٤٨ شخصاً · ٥٧ علاقة</span>
          </div>
          <ResearchGraph selected={selectedId} onSelect={(node) => setSelectedId(node.id)} />
          <div className="tree-canvas-footer"><LockKeyhole size={13} /> النسخة المنشورة ثابتة. التعديلات الجديدة تنشئ نسخة مسودة.</div>
        </section>

        <aside className="node-detail-panel">
          <div className="node-detail-top">
            <div className="node-detail-avatar">{selected.name.slice(0, 1)}</div>
            <div>
              <div className="eyebrow">شخص محدد</div>
              <h2>{selected.name}</h2>
              <p>{selected.years} · {selected.role}</p>
            </div>
          </div>
          <div className="node-detail-alert">
            <StatusBadge tone={selected.tone}>{selected.tone === "disputed" ? "في центр سؤال" : "ضمن التفسير"}</StatusBadge>
            <span>{selected.sourceCount} إشارات مرتبطة</span>
          </div>
          <div className="detail-block">
            <div className="detail-label">ملاحظة الباحث</div>
            <p>{selected.note}</p>
          </div>
          <div className="detail-block">
            <div className="detail-label">ما تمثله هذه الشجرة</div>
            <p>تضع هذه النسخة Abdullah تحت فرع محمد، مع إبقاء علاقة والده محل نقاش بين التفسير والروايات الأخرى.</p>
          </div>
          <div className="detail-block">
            <div className="detail-label">المصادر القريبة</div>
            <EvidenceMiniList />
          </div>
          <Link to="/research" className="detail-cta">افتح هذا الشخص في مكتب البحث <ArrowLeft size={15} /></Link>
        </aside>
      </div>

      <section className="version-strip">
        <div><div className="eyebrow">سجل النسخ</div><h2>ما الذي تغيّر بين الإصدارات؟</h2><p>الاختلاف هنا ليس خطأً؛ قد يكون بداية لسؤال جديد.</p></div>
        <div className="version-timeline">
          <div className="version-item version-item-current"><span>v3</span><div><strong>النشر الحالي</strong><small>٤ علاقات أضيفت، ١ علاقة نُقلت إلى «متنازع عليها»</small></div></div>
          <div className="version-item"><span>v2</span><div><strong>مراجعة مصدر</strong><small>قبل ١٨ يوماً</small></div></div>
          <div className="version-item"><span>v1</span><div><strong>النسخة الأولى</strong><small>قبل ٤ أشهر</small></div></div>
        </div>
      </section>
    </div>
  );
}
