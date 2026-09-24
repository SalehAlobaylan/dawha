import { useQuery } from "@tanstack/react-query";
import { CalendarDays, ChevronLeft, CircleAlert, FileText, Layers3, MapPinned, Route, SlidersHorizontal } from "lucide-react";
import { useState } from "react";
import { fetchMapFeatures } from "../lib/api";
import { HistoricalMapCanvas } from "./HistoricalMapCanvas";
import { StatusBadge } from "./StatusBadge";
import type { MapFeature } from "../types";

const periods = [
  { value: "all", label: "كل الفترات" },
  { value: "before-1150", label: "قبل ١١٥٠" },
  { value: "1150-1250", label: "١١٥٠ — ١٢٥٠" },
  { value: "after-1250", label: "بعد ١٢٥٠" },
];

const layers = [
  { value: "", label: "كل الحالات" },
  { value: "documented", label: "موثق" },
  { value: "interpreted", label: "مقروء" },
  { value: "platform_inferred", label: "استنتاجي" },
  { value: "disputed", label: "متنازع عليه" },
  { value: "unresolved", label: "غير محسوم" },
];

export function HistoricalMapPanel() {
  const [period, setPeriod] = useState("all");
  const [status, setStatus] = useState("");
  const [selected, setSelected] = useState<MapFeature | null>(null);
  const filters = periodFilters(period);
  const mapQuery = useQuery({ queryKey: ["historical-map", period, status], queryFn: () => fetchMapFeatures({ ...filters, status: status || undefined }) });
  const features = mapQuery.data?.features ?? [];
  return (
    <section className="historical-map-panel">
      <div className="historical-map-head"><div><div className="eyebrow">خريطة دَوْحة / البيانات المحفوظة</div><h2>تابع المسار، لا الظل فقط</h2><p>النقاط والخطوط هنا من العلاقات الجغرافية المسجلة، مع إبقاء درجة اليقين قابلة للقراءة.</p></div><StatusBadge tone="source">MapLibre</StatusBadge></div>
      <div className="historical-map-toolbar"><label className="historical-period"><CalendarDays size={14} /><select value={period} onChange={(event) => { setPeriod(event.target.value); setSelected(null); }} aria-label="تصفية الفترة">{periods.map((item) => <option value={item.value} key={item.value}>{item.label}</option>)}</select></label><div className="historical-layer-pills">{layers.map((layer) => <button type="button" className={`historical-layer-pill${status === layer.value ? " historical-layer-pill-active" : ""}`} key={layer.value || "all"} onClick={() => { setStatus(layer.value); setSelected(null); }}>{layer.value === "documented" ? <MapPinned size={12} /> : layer.value === "platform_inferred" ? <SlidersHorizontal size={12} /> : null}{layer.label}</button>)}</div><span className="historical-feature-count">{features.length} إشارة</span></div>
      {mapQuery.isPending ? <div className="historical-map-state">جارٍ تحميل طبقات الخريطة…</div> : mapQuery.error ? <div className="historical-map-state historical-map-error" role="alert">{mapError(mapQuery.error)}</div> : <div className="historical-map-grid"><div className="historical-map-visual"><HistoricalMapCanvas features={features} selectedId={selected?.id ?? ""} onSelect={setSelected} /><div className="historical-map-legend"><span><i className="legend-dot legend-dot-source" /> موثق</span><span><i className="legend-dot legend-dot-interpretation" /> استنتاجي</span><span><i className="legend-dot legend-dot-disputed" /> متنازع</span><span><Route size={12} /> هجرة</span></div></div><aside className="historical-feature-inspector">{selected ? <FeatureInspector feature={selected} /> : <div className="historical-inspector-empty"><Layers3 size={19} /><strong>اختر إشارة من الخريطة</strong><span>اضغط نقطة أو خط هجرة لتقرأ مصدره ودليله ودرجة يقينه.</span></div>}</aside></div>}
      {!mapQuery.isPending && !mapQuery.error ? <div className="historical-feature-list">{features.slice(0, 12).map((feature) => <button type="button" className={`historical-feature-row${selected?.id === feature.id ? " historical-feature-row-active" : ""}`} key={feature.id} onClick={() => setSelected(feature)}><span className={`historical-feature-status historical-feature-status-${feature.status}`} /><span><strong>{feature.placeName || feature.entityName || feature.relationType || feature.kind}</strong><small>{feature.kind === "migration" ? "مسار هجرة" : feature.relationType || "نقطة موثقة"}</small></span><span className="historical-feature-period">{feature.timeFrom?.slice(0, 4) || "—"}</span><ChevronLeft size={14} /></button>)}</div> : null}
      <div className="historical-map-note"><CircleAlert size={14} /><span>الخطوط المنقطة استنتاجات زمنية أو مكانية، ولا تُعرض كحدود تاريخية مؤكدة.</span></div>
    </section>
  );
}

function FeatureInspector({ feature }: { feature: MapFeature }) {
  return <div className="historical-inspector"><div className="historical-inspector-head"><div><div className="eyebrow">{feature.kind === "migration" ? "مسار هجرة" : "إشارة جغرافية"}</div><h3>{feature.placeName || feature.entityName || feature.relationType}</h3></div><StatusBadge tone={feature.status === "documented" ? "source" : feature.status === "disputed" ? "disputed" : "interpretation"}>{statusLabel(feature.status)}</StatusBadge></div><div className="historical-inspector-facts"><div><span>الكيان</span><strong>{feature.entityName || "—"}</strong></div><div><span>الفترة</span><strong>{feature.timeFrom || "—"} — {feature.timeTo || "—"}</strong></div><div><span>اليقين</span><strong>{certaintyLabel(feature.certainty)}</strong></div><div><span>الدليل</span><strong>{feature.evidenceStatus ? evidenceStatusLabel(feature.evidenceStatus) : "دون دليل مكتمل"}</strong></div></div>{feature.evidenceText ? <div className="historical-evidence"><FileText size={14} /><p>{feature.evidenceText}</p></div> : null}{feature.sourceTitle ? <div className="historical-source"><FileText size={13} /><span>المصدر: {feature.sourceTitle}</span></div> : null}</div>;
}

function periodFilters(period: string): { fromYear?: number; toYear?: number } {
  if (period === "before-1150") return { toYear: 1149 };
  if (period === "1150-1250") return { fromYear: 1150, toYear: 1250 };
  if (period === "after-1250") return { fromYear: 1251 };
  return {};
}

function statusLabel(status: string): string {
  if (status === "documented") return "موثق";
  if (status === "interpreted") return "مقروء";
  if (status === "platform_inferred") return "استنتاجي";
  if (status === "disputed") return "متنازع عليه";
  return "غير محسوم";
}

function certaintyLabel(value?: string): string {
  if (value === "precise") return "دقيق";
  if (value === "approximate") return "تقريبي";
  if (value === "uncertain") return "غير مؤكد";
  return "غير محدد";
}

function evidenceStatusLabel(value: string): string {
  if (value === "accepted") return "مقبول";
  if (value === "rejected") return "مرفوض";
  if (value === "unreviewed") return "غير مراجع";
  return "يحتاج مراجعة";
}

function mapError(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر تحميل طبقات الخريطة.";
}
