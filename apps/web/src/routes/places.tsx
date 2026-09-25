import { Link } from "@tanstack/react-router";
import { ArrowLeft, CalendarDays, ChevronLeft, CircleAlert, Layers3, MapPinned, Plus, Route, Search, SearchX, SlidersHorizontal } from "lucide-react";
import { useState } from "react";
import { places } from "../data/demo";
import { MapLibreCanvas } from "../components/MapLibreCanvas";
import { HistoricalMapPanel } from "../components/HistoricalMapPanel";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";
import type { PlaceRecord } from "../types";

const allPeriods = "كل الفترات";

/**
 * A place matches a search when its name, kind or period contains it. The index
 * is a local list of a few places, so this is a filter over what the page
 * already has rather than a new query.
 */
function matchesQuery(place: PlaceRecord, query: string): boolean {
  const needle = query.trim();
  if (needle.length === 0) return true;
  return place.name.includes(needle) || place.type.includes(needle) || place.period.includes(needle);
}

export function PlacesPage() {
  const [selected, setSelected] = useState(places[0]);
  const [period, setPeriod] = useState(allPeriods);
  const [query, setQuery] = useState("");
  const [activeLayers, setActiveLayers] = useState(["موثق", "مقترح", "متنازع عليه"]);

  // The period options are the periods this index actually holds, so every
  // option in the control filters something. A bucket no place can be in is a
  // control that lies about what it does.
  const periodOptions = [...new Set(places.map((place) => place.period))];
  const visiblePlaces = places.filter((place) => (period === allPeriods || place.period === period) && matchesQuery(place, query));

  const toggleLayer = (layer: string) => setActiveLayers((current) => current.includes(layer) ? current.filter((item) => item !== layer) : [...current, layer]);

  return (
    <div className="page-stack">
      <TopBar eyebrow="الجغرافيا / المواضع" title="الموضع جزء من السؤال" description="الاحضور، والهجرة، والحدود — كلها درجات حضور، وليست خطوطاً قاطعة." />
      <section className="places-toolbar"><div className="place-breadcrumb"><MapPinned size={16} /><span>فهرس المواضع</span><ChevronLeft size={14} /><strong>الرياض وما حوله</strong></div><div className="place-toolbar-actions"><label className="place-period"><CalendarDays size={15} /><select aria-label="تصفية بالفترة" value={period} onChange={(event) => setPeriod(event.target.value)}>{[allPeriods, ...periodOptions].map((option) => <option key={option}>{option}</option>)}</select></label><button className="secondary-button" type="button"><SlidersHorizontal size={15} /> تصفية</button><button className="primary-button" type="button"><Plus size={15} /> إضافة موضع</button></div></section>

      <section className="places-workspace">
        <aside className="places-sidebar">
          <div className="eyebrow">الموضع المحدد</div><div className="place-detail-title"><span className="place-large-mark"><MapPinned size={21} /></span><div><h2>{selected.name}</h2><p>{selected.type} · {selected.period}</p></div></div><StatusBadge tone={selected.status === "موثق" ? "source" : selected.status === "متنازع عليه" ? "disputed" : "interpretation"}>{selected.status}</StatusBadge>
          <div className="place-detail-stats"><div><strong>{selected.evidence}</strong><span>إشارات</span></div><div><strong>03</strong><span>أدوار</span></div><div><strong>02</strong><span>أسئلة</span></div></div>
          <div className="place-detail-block"><div className="detail-label">ماذا نعرف؟</div><p>تظهر إشارات لهذا الموضع موزعة على أكثر من مصدر، مع فارق في التسميات والتواريخ.</p></div>
          <div className="place-detail-block"><div className="detail-label">ماذا لا نعرف؟</div><p>لم تُحسم حدود الحضور، ولم تُثبت كل الروايات كاعتماد مستقل.</p></div>
          <Link to="/research" className="detail-cta">افتح المواضع في البحث <ArrowLeft size={15} /></Link>
        </aside>
        <div className="full-map-panel"><div className="full-map-header"><div><div className="eyebrow">خريطة طبقية</div><h2>حضور، اقتراح، وخلاف</h2></div><div className="map-legend-inline"><span><i className="legend-dot legend-interpretation" /> موثق</span><span><i className="legend-dot legend-question" /> مقترح</span><span><i className="legend-dot legend-disputed" /> متنازع</span></div></div><div className="full-map-canvas"><MapLibreCanvas places={places} selectedId={selected.id} onSelect={setSelected} /><div className="map-paper-grid map-grid-overlay" /><div className="map-terrain terrain-one" /><div className="map-terrain terrain-two" /><svg className="full-map-routes" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true"><path d="M14 66 C24 58, 29 46, 39 49 S55 63, 65 34 S78 25, 88 18" /><path d="M39 49 C48 47, 56 49, 65 34" /><path d="M65 34 C70 49, 74 63, 85 72" /></svg>{places.map((place) => <button type="button" key={place.id} className={`map-marker map-marker-${place.status === "موثق" ? "source" : place.status === "متنازع عليه" ? "disputed" : "interpretation"}${selected.id === place.id ? " map-marker-active" : ""}`} style={{ left: `${place.x}%`, top: `${place.y}%` }} onClick={() => setSelected(place)} aria-label={place.name}><span /></button>)}<span className="map-region-label region-najd">نجد</span><span className="map-region-label region-ahsa">الأحساء</span><span className="map-region-label region-hijaz">حجاز</span><div className="map-route-label"><Route size={13} /> هجرة مقترحة</div><div className="map-scale-full"><span /> 200 كم تقريباً</div></div><div className="map-bottom-bar"><div className="map-bottom-note"><CircleAlert size={14} /><span>المسارات المنقطة استنتاجات، وليست حدوداً تاريخية.</span></div><div className="map-layer-pills">{["موثق", "مقترح", "متنازع عليه"].map((layer) => <button type="button" key={layer} className={`map-layer-pill${activeLayers.includes(layer) ? " map-layer-pill-active" : ""}`} onClick={() => toggleLayer(layer)}><span className={`layer-dot layer-dot-${layer === "موثق" ? "source" : layer === "مقترح" ? "interpretation" : "disputed"}`} />{layer}</button>)}</div></div></div>
      </section>

      <HistoricalMapPanel />

      <section className="places-index-section"><div className="places-index-head"><div><div className="eyebrow">قائمة المواضع</div><h2>ابنِ زمنك المفتوح</h2><p>اختر موضعاً، أو ابحث عن موضع قديم لا تزال تسميته غير مستقرة.</p></div><label className="library-search"><Search size={15} /><input aria-label="ابحث عن موضع" placeholder="ابحث عن موضع" value={query} onChange={(event) => setQuery(event.target.value)} /></label></div><div className="place-index-grid">{visiblePlaces.map((place) => <button type="button" className="place-index-card" key={place.id} onClick={() => setSelected(place)}><span className={`place-index-icon place-icon-${place.status === "موثق" ? "source" : place.status === "متنازع عليه" ? "disputed" : "interpretation"}`}><MapPinned size={17} /></span><span className="place-index-copy"><strong>{place.name}</strong><small>{place.type} · {place.period}</small></span><span className="place-index-count">{place.evidence} إشارات</span></button>)}</div>{visiblePlaces.length === 0 ? <div className="empty-search"><SearchX size={20} /><strong>لا توجد نتائج مطابقة</strong><span>جرّب اسماً آخر، أو أعد الفتح على كل الفترات.</span></div> : null}</section>

      <section className="map-method-note"><Layers3 size={18} /><div><strong>كيف نعرض عدم اليقين؟</strong><span>نستخدم نقاط حضور، مناطق تقريبية، وطبقات منفصلة للحقائق والادعاءات والاستنتاجات.</span></div><Link to="/research">اقرأ منهجية البحث <ArrowLeft size={14} /></Link></section>
    </div>
  );
}
