import { Link } from "@tanstack/react-router";
import { ArrowLeft, BookOpen, GitCompareArrows, MapPin, Plus, ShieldCheck } from "lucide-react";
import type { TreeNode } from "../types";
import { StatusBadge } from "./StatusBadge";

const connections = [
  ["p-1", "p-2"],
  ["p-2", "p-3"],
  ["p-2", "p-5"],
  ["p-2", "p-7"],
  ["p-5", "p-6"],
  ["p-5", "p-4"],
];

export function ResearchGraph({ selected, onSelect }: { selected: string; onSelect: (node: TreeNode) => void }) {
  const nodes = useGraphNodes();

  return (
    <div className="research-graph" aria-label="رسم شجرة تجريبي">
      <div className="graph-grid-lines" />
      <svg className="graph-lines" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
        {connections.map(([from, to]) => {
          const source = nodes.find((node) => node.id === from);
          const target = nodes.find((node) => node.id === to);
          if (!source || !target) return null;
          return <line key={`${from}-${to}`} x1={source.x} y1={source.y} x2={target.x} y2={target.y} />;
        })}
      </svg>
      {nodes.map((node) => (
        <button
          type="button"
          key={node.id}
          className={`graph-node graph-node-${node.tone}${selected === node.id ? " graph-node-selected" : ""}`}
          style={{ left: `${node.x}%`, top: `${node.y}%` }}
          onClick={() => onSelect(node)}
          aria-label={`فتح ${node.name}`}
        >
          <span className="graph-node-avatar">{node.name.slice(0, 1)}</span>
          <span className="graph-node-copy">
            <strong>{node.name}</strong>
            <small>{node.role}</small>
          </span>
          {node.tone === "disputed" ? <span className="node-alert">!</span> : null}
        </button>
      ))}
      <div className="graph-legend">
        <span><i className="legend-dot legend-interpretation" /> تفسير الشجرة</span>
        <span><i className="legend-dot legend-disputed" /> رواية متنازعة</span>
        <span><i className="legend-dot legend-question" /> غير محسوم</span>
      </div>
    </div>
  );
}

function useGraphNodes(): TreeNode[] {
  return [
    {
      id: "p-1",
      name: "محمد بن سعد",
      role: "الجد الأعلى",
      years: "١٠٨٠ — ١١٥٠هـ",
      x: 14,
      y: 44,
      tone: "interpretation",
      sourceCount: 5,
      note: "تظهره النسخة المنشورة كسلف مباشر لعبدالله.",
    },
    {
      id: "p-2",
      name: "عبدالله بن محمد",
      role: "الجيل الثاني",
      years: "تقريباً ١١٢٠ — ١٢١٠هـ",
      x: 39,
      y: 44,
      tone: "disputed",
      sourceCount: 4,
      note: "محور سؤال مفتوح بسبب روايتين مختلفتين عن والده.",
    },
    {
      id: "p-3",
      name: "أم عبدالله",
      role: "الجيل الثاني",
      years: "غير محددة",
      x: 39,
      y: 76,
      tone: "question",
      sourceCount: 1,
      note: "لم يتم توثيق تاريخها في المصادر المتاحة.",
    },
    {
      id: "p-4",
      name: "سعد بن عامر",
      role: "فرع محتمل",
      years: "تقريباً ١٠٤٠ — ١١٥٠هـ",
      x: 65,
      y: 18,
      tone: "disputed",
      sourceCount: 3,
      note: "مرشح مربوط في رواية، لم تُحسم صلته بالشجرة.",
    },
    {
      id: "p-5",
      name: "صالح بن عبدالله",
      role: "ابن متابع",
      years: "تقريباً ١١٥٠ — ١١٨٠هـ",
      x: 65,
      y: 44,
      tone: "claim",
      sourceCount: 2,
      note: "ادعاء قيد المراجعة، مصدره غير مستقل بعد.",
    },
    {
      id: "p-6",
      name: "مبارك بن صالح",
      role: "الجيل التالي",
      years: "تقريباً ١١٨٠ — ١٢٣٠هـ",
      x: 87,
      y: 44,
      tone: "interpretation",
      sourceCount: 3,
      note: "موجود في تفسير الشجرة المنشور.",
    },
    {
      id: "p-7",
      name: "نورة بنت عبدالله",
      role: "فرع جانبي",
      years: "غير محددة",
      x: 87,
      y: 76,
      tone: "source",
      sourceCount: 1,
      note: "مذكورة في هامش المصدر الأول فقط.",
    },
  ];
}

export function TreeSummary() {
  return (
    <div className="tree-summary">
      <div className="tree-summary-top">
        <div>
          <div className="eyebrow">النسخة المنشورة</div>
          <h2>شجرة بيت العنبر</h2>
          <p>تفسير قابل للتتبع، لا إجابة نهائية.</p>
        </div>
        <StatusBadge tone="interpretation">منشورة · v3</StatusBadge>
      </div>
      <div className="tree-summary-stats">
        <span><strong>48</strong> شخصاً</span>
        <span><strong>57</strong> علاقة</span>
        <span><strong className="text-disputed">4</strong> غير محسومة</span>
      </div>
      <div className="tree-summary-actions">
        <button className="secondary-button" type="button"><GitCompareArrows size={15} /> قارن النسخ</button>
        <button className="secondary-button" type="button"><MapPin size={15} /> الخريطة</button>
        <Link to="/research" className="primary-button"><Plus size={15} /> أضف ملاحظة</Link>
      </div>
    </div>
  );
}

export function EvidenceMiniList() {
  return (
    <div className="mini-evidence-list">
      <div className="mini-evidence-item">
        <span className="mini-evidence-icon source"><BookOpen size={14} /></span>
        <div><strong>المصدر الأول</strong><span>الجزء الثاني، ص ١٢١</span></div>
        <ShieldCheck size={15} className="evidence-ok" />
      </div>
      <div className="mini-evidence-item">
        <span className="mini-evidence-icon disputed"><BookOpen size={14} /></span>
        <div><strong>رواية منافسة</strong><span>تحتاج فحص الاعتماد</span></div>
        <ArrowLeft size={15} className="evidence-arrow" />
      </div>
    </div>
  );
}
