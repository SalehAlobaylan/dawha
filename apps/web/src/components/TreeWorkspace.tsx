import { Link } from "@tanstack/react-router";
import { ArrowUpLeft, CircleHelp, GitCompareArrows, Plus } from "lucide-react";
import type { DashboardData } from "../types";
import { ResearchGraph, TreeSummary } from "./ResearchGraph";
import { SectionHeading } from "./SectionHeading";
import { StatusBadge } from "./StatusBadge";

export function TreeWorkspace({ data }: { data: DashboardData }) {
  return (
    <section className="tree-workspace" id="tree-preview">
      <SectionHeading
        eyebrow="شجرة مفتوحة"
        title="ابدأ من العلاقات، لا من إجابة"
        description={`افتح نسخة ${data.treePreview.title} المنشورة، ثم انتقل إلى مصدرها أو إلى السؤال الذي يتعارض معها.`}
        action="كل الأشجار"
      />
      <div className="tree-workspace-grid">
        <div className="tree-canvas-panel">
          <div className="tree-canvas-toolbar">
            <div className="toolbar-tabs">
              <button className="toolbar-tab toolbar-tab-active" type="button">الجيل الثاني</button>
              <button className="toolbar-tab" type="button">الكل <span>48</span></button>
            </div>
            <div className="toolbar-actions">
              <button type="button" className="mini-icon-button" aria-label="تكبير">+</button>
              <button type="button" className="mini-icon-button" aria-label="تصغير">−</button>
              <button type="button" className="mini-icon-button" aria-label="ملخص">⌘</button>
            </div>
          </div>
          <ResearchGraph selected="p-2" onSelect={() => undefined} />
        </div>
        <aside className="tree-preview-side">
          <TreeSummary />
          <div className="tree-side-note">
            <StatusBadge tone="question">4 صلات لم تُحسم</StatusBadge>
            <p>يمكن أن تحتوي النسخة المنشورة على علاقات غير محسومة. ألّا يُقرأ عدم الحسم كغياب دليل.</p>
            <Link to="/questions" className="inline-link">افتح أسئلة الشجرة <ArrowUpLeft size={14} /></Link>
          </div>
          <div className="tree-side-actions">
            <button type="button" className="secondary-button"><GitCompareArrows size={15} /> مقارنة</button>
            <button type="button" className="secondary-button"><Plus size={15} /> توثيق ملاحظة</button>
          </div>
        </aside>
      </div>
      <div className="tree-footnote">
        <CircleHelp size={14} />
        <span>هذه معاينة توضيحية. الأرقام والكيانات المعروضة اصطناعية وغير مخصصة لأشخاص أحياء.</span>
      </div>
    </section>
  );
}
