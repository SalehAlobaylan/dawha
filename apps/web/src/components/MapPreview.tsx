import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, CircleAlert, LoaderCircle, MapPinned, Route } from "lucide-react";
import { useMemo } from "react";
import { fetchMapFeatures } from "../lib/api";
import {
  aggregateFeatures,
  migrationLine,
  viewportForFeatures,
  type AtlasViewport,
} from "../lib/atlas";
import { formatMapCount, mapCertaintyDash, mapStatusCopy, mapStatusVocabulary } from "../lib/map-vocabulary";

/**
 * The landing-page atlas preview.
 *
 * It is a schematic, and it says so: a projected point plot of the same
 * `MapFeature[]` the full atlas reads, with no invented route, no fictional scale
 * and no toggle that filters nothing. A reader can see that the map exists and
 * follow it into the real surface.
 */
export function MapPreview() {
  const mapQuery = useQuery({
    queryKey: ["map-preview"],
    queryFn: ({ signal }) => fetchMapFeatures({ limit: 500, signal }),
    staleTime: 60000,
    refetchOnWindowFocus: false,
  });
  const features = useMemo(() => mapQuery.data?.features ?? [], [mapQuery.data]);
  const markers = useMemo(() => aggregateFeatures(features).slice(0, 24), [features]);
  const migrations = useMemo(() => features.filter((feature) => feature.kind === "migration"), [features]);
  const regions = useMemo(() => features.filter((feature) => feature.kind === "region" && feature.ring), [features]);
  const viewport = useMemo(() => viewportForFeatures(features), [features]);
  const project = useMemo(() => projector(viewport), [viewport]);
  const placeCount = useMemo(() => new Set(features.map((feature) => feature.placeId).filter(Boolean)).size, [features]);

  return (
    <section className="map-preview-section" id="places-preview">
      <div className="map-preview-copy">
        <div className="eyebrow">الجغرافيا التاريخية</div>
        <h2>الجغرافيا ذاكرة، لا حدوداً قاطعة</h2>
        <p>اعرض المواضع والهجرات بوصفها درجات حضور. النقطة ليست بالضرورة حداً، والمسار ليس بالضرورة يقيناً.</p>
        {mapQuery.isPending ? (
          <div className="map-preview-state"><LoaderCircle size={16} className="atlas-spin" /><span>جارٍ قراءة الأطلس…</span></div>
        ) : mapQuery.isError ? (
          <div className="map-preview-state" role="status"><CircleAlert size={16} /><span>الأطلس غير متاح الآن: {mapQuery.error instanceof Error ? mapQuery.error.message : "تعذر الاتصال بالخدمة."}</span></div>
        ) : features.length === 0 ? (
          <div className="map-preview-state"><MapPinned size={16} /><span>لم تُسجَّل إشارات جغرافية بعد. الأطلس يبدأ فارغاً، ولا نملؤه ببيانات توضيحية.</span></div>
        ) : (
          <div className="map-preview-stats" aria-label="ملخص الأطلس">
            <div><strong>{formatMapCount(features.length)}</strong><span>إشارة</span></div>
            <div><strong>{formatMapCount(placeCount)}</strong><span>موضعاً</span></div>
            <div><strong>{formatMapCount(migrations.length)}</strong><span>هجرة</span></div>
          </div>
        )}
        {mapQuery.data?.truncated ? <p className="map-preview-footnote">معاينة أول ٥٠٠ إشارة ضمن الحد؛ الأطلس الكامل يعرض ما يتسع له الإطار.</p> : null}
        <Link to="/places" className="inline-link map-preview-link">
          افتح الأطلس كاملاً <ArrowLeft size={15} />
        </Link>
      </div>
      <div className="map-preview-visual" aria-label="معاينة تخطيطية للأطلس">
        <div className="map-paper-grid" aria-hidden="true" />
        {features.length === 0 || !viewport ? (
          <div className="map-preview-empty" aria-hidden="true">
            <MapPinned size={26} />
            <span>لا نقاط بعد</span>
          </div>
        ) : (
          <>
            <svg className="map-preview-svg" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
              {regions.map((region) => {
                const ring = region.ring ?? [];
                const points = ring.map(([longitude, latitude]) => project(longitude, latitude)).map(([x, y]) => `${x},${y}`).join(" ");
                const copy = mapStatusCopy(region.status);
                return <polygon key={region.id} points={points} fill={copy.fill} fillOpacity="0.13" stroke={copy.stroke} strokeWidth="0.35" strokeDasharray={region.certainty === "disputed" ? "1.4 1.2" : undefined} />;
              })}
              {migrations.map((migration) => {
                const line = migrationLine(migration);
                if (!line) return null;
                const [from, to] = line.coordinates;
                const [x1, y1] = project(from[0], from[1]);
                const [x2, y2] = project(to[0], to[1]);
                const copy = mapStatusCopy(migration.status);
                return (
                  <g key={migration.id}>
                    <line x1={x1} y1={y1} x2={x2} y2={y2} stroke={copy.stroke} strokeWidth="0.45" strokeDasharray={mapCertaintyDash(migration.certainty).map((value) => value / 2).join(" ")} />
                    <circle cx={x2} cy={y2} r="0.8" fill={copy.stroke} />
                  </g>
                );
              })}
            </svg>
            {markers.map((marker) => {
              const [left, top] = project(marker.longitude, marker.latitude);
              const copy = mapStatusVocabulary[marker.status];
              return (
                <Link
                  key={marker.key}
                  to="/places"
                  search={{ feature: marker.key, place: marker.placeId }}
                  className={`map-preview-marker${copy.seal ? " map-preview-marker-seal" : ""}${copy.hollow ? " map-preview-marker-hollow" : ""}`}
                  style={{ left: `${left}%`, top: `${top}%`, color: copy.stroke }}
                  aria-label={`${marker.label}، ${copy.label}، ${formatMapCount(marker.count)} إشارة`}
                  title={`${marker.label} · ${copy.label}`}
                >
                  <span aria-hidden="true">{copy.glyph}</span>
                  {marker.count > 1 ? <b aria-hidden="true">{marker.count}</b> : null}
                </Link>
              );
            })}
            <div className="map-preview-caption" aria-hidden="true">
              <Route size={12} />
              مخطط إحداثي، لا خلفية بحدود اليوم
            </div>
          </>
        )}
      </div>
    </section>
  );
}

function projector(viewport: AtlasViewport | null): (longitude: number, latitude: number) => [number, number] {
  if (!viewport) return () => [50, 50];
  const longitudeSpan = Math.max(0.001, viewport.maxLongitude - viewport.minLongitude);
  const latitudeSpan = Math.max(0.001, viewport.maxLatitude - viewport.minLatitude);
  return (longitude, latitude) => {
    const x = 6 + ((longitude - viewport.minLongitude) / longitudeSpan) * 88;
    const y = 6 + ((viewport.maxLatitude - latitude) / latitudeSpan) * 88;
    return [Math.max(4, Math.min(96, x)), Math.max(4, Math.min(96, y))];
  };
}
