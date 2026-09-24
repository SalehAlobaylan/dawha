import { Link } from "@tanstack/react-router";
import { ArrowUpLeft, MapPinned, Navigation, Plus, Route, Sparkles } from "lucide-react";
import { useState } from "react";
import type { PlaceRecord } from "../types";
import { StatusBadge } from "./StatusBadge";

export function MapPreview({ places }: { places: PlaceRecord[] }) {
  const [selected, setSelected] = useState(places[0]);
  const [layers, setLayers] = useState({ documented: true, inferred: true, disputed: true });

  return (
    <section className="map-preview-section" id="places-preview">
      <div className="map-preview-copy">
        <div className="eyebrow">الجغرافيا التاريخية</div>
        <h2>الجغرافيا ذاكرة، لا حدوداً قاطعة</h2>
        <p>اعرض المواضع والتبعثرات بوصفها درجات حضور. النقطة ليست بالضرورة حداً، والمسار ليس بالضرورة يقيناً.</p>
        <div className="map-layer-controls">
          <span className="map-control-label">طبقات الخريطة</span>
          <MapLayerToggle label="موثق" active={layers.documented} onClick={() => setLayers((current) => ({ ...current, documented: !current.documented }))} tone="source" />
          <MapLayerToggle label="مقترح" active={layers.inferred} onClick={() => setLayers((current) => ({ ...current, inferred: !current.inferred }))} tone="interpretation" />
          <MapLayerToggle label="متنازع عليه" active={layers.disputed} onClick={() => setLayers((current) => ({ ...current, disputed: !current.disputed }))} tone="disputed" />
        </div>
        <Link to="/places" className="inline-link">افتح خريطة المواضع <ArrowUpLeft size={14} /></Link>
      </div>
      <div className="map-preview-visual">
        <div className="map-paper-grid" />
        <svg className="map-routes" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
          <path d="M25 30 C35 36, 43 45, 49 54 S57 64, 65 34" />
          <path d="M49 54 C42 60, 38 64, 36 68" />
        </svg>
        {places.map((place) => (
          <button
            type="button"
            key={place.id}
            className={`map-marker map-marker-${place.status === "موثق" ? "source" : place.status === "متنازع عليه" ? "disputed" : "interpretation"}${selected.id === place.id ? " map-marker-active" : ""}`}
            style={{ left: `${place.x}%`, top: `${place.y}%` }}
            onClick={() => setSelected(place)}
            aria-label={place.name}
          >
            <span />
          </button>
        ))}
        <div className="map-label map-label-riyadh">الرياض</div>
        <div className="map-label map-label-ahsa">الأحساء</div>
        <div className="map-label map-label-hijaz">حجاز</div>
        <div className="map-scale"><span /> 200 كم تقريباً</div>
        <div className="map-selected-card">
          <div className="map-selected-icon"><MapPinned size={16} /></div>
          <div><strong>{selected.name}</strong><span>{selected.period} · {selected.evidence} إشارات</span></div>
          <StatusBadge tone={selected.status === "موثق" ? "source" : selected.status === "متنازع عليه" ? "disputed" : "interpretation"} compact>{selected.status}</StatusBadge>
        </div>
      </div>
      <div className="map-stats-strip">
        <div><Route size={15} /><span>4 مسارات مقترحة</span></div>
        <div><Navigation size={15} /><span>مناطق حضور متداخلة</span></div>
        <div><Sparkles size={15} /><span>استنتاجات تبقى قابلة للمراجعة</span></div>
        <button type="button" className="map-add-button"><Plus size={15} /> إضافة موضع</button>
      </div>
    </section>
  );
}

function MapLayerToggle({ label, active, onClick, tone }: { label: string; active: boolean; onClick: () => void; tone: "source" | "interpretation" | "disputed" }) {
  return (
    <button type="button" className={`map-layer-toggle${active ? " map-layer-active" : ""}`} onClick={onClick}>
      <span className={`layer-dot layer-dot-${tone}`} />
      {label}
      {active ? <span className="layer-check">✓</span> : null}
    </button>
  );
}
