import { Link } from "@tanstack/react-router";
import { ArrowLeft, BookOpen, GitCompareArrows, MapPin, Plus, ShieldCheck } from "lucide-react";
import type { TreeNode, TreeRelationshipRecord } from "../types";
import { StatusBadge } from "./StatusBadge";

const defaultConnections = [
  ["p-1", "p-2"],
  ["p-2", "p-3"],
  ["p-2", "p-5"],
  ["p-2", "p-7"],
  ["p-5", "p-6"],
  ["p-5", "p-4"],
];

interface ResearchGraphProps {
  selected: string;
  onSelect: (node: TreeNode) => void;
  nodes?: TreeNode[];
  relationships?: TreeRelationshipRecord[];
  label?: string;
}

export function ResearchGraph({ selected, onSelect, nodes, relationships, label }: ResearchGraphProps) {
  const defaultNodes = useGraphNodes();
  const graphNodes = nodes ?? defaultNodes;
  const graphConnections = relationships?.map((relationship) => ({ from: relationship.subjectNodeId, to: relationship.objectNodeId, status: relationship.status })) ?? defaultConnections.map(([from, to]) => ({ from, to, status: "interpreted" }));
  const empty = nodes !== undefined && nodes.length === 0;

  return (
    <div className="research-graph" aria-label={label ?? "رسم شجرة تجريبي"}>
      <div className="graph-grid-lines" />
      <svg className="graph-lines" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
        <defs>
          <marker id="graph-arrow" markerWidth="5" markerHeight="5" refX="4" refY="2.5" orient="auto" markerUnits="strokeWidth">
            <path d="M0,0 L5,2.5 L0,5 z" />
          </marker>
        </defs>
        {graphConnections.map((connection) => {
          const source = graphNodes.find((node) => node.id === connection.from);
          const target = graphNodes.find((node) => node.id === connection.to);
          if (!source || !target) return null;
          return <line key={`${connection.from}-${connection.to}`} className={`graph-line graph-line-${connection.status}`} markerEnd="url(#graph-arrow)" x1={source.x} y1={source.y} x2={target.x} y2={target.y} />;
        })}
      </svg>
      {empty ? <div className="graph-empty"><strong>لا توجد أشخاص في هذه المسودة بعد</strong><span>أضف أول شخص لبدء تفسير الشجرة.</span></div> : null}
      {graphNodes.map((node) => (
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
      personId: "person-1",
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
      personId: "person-2",
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
      personId: "person-3",
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
      personId: "person-4",
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
      personId: "person-5",
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
      personId: "person-6",
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
      personId: "person-7",
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

export function EvidenceMiniList({ sourceCount }: { sourceCount: number }) {
  if (sourceCount === 0) {
    return <div className="mini-evidence-empty">لم تُربط بهذه العقدة مصادر بعد.</div>;
  }
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
