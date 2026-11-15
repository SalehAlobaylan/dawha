import { CalendarDays, RotateCcw, Search } from "lucide-react";
import { useMemo, type CSSProperties } from "react";
import type { MapFeature } from "../types";
import { countStatuses, filterFeatures, periodPresets, type AtlasFilters as AtlasFilterValues, type PeriodPreset } from "../lib/atlas";
import { atlasEras, eraOfWindow } from "../lib/atlas-periods";
import {
  formatMapCount,
  mapKindVocabulary,
  mapKinds,
  mapStatusVocabulary,
  mapStatuses,
  type MapKind,
  type MapStatus,
} from "../lib/map-vocabulary";

interface AtlasFiltersProps {
  features: MapFeature[];
  filteredCount: number;
  statuses: MapStatus[];
  kinds: MapKind[];
  bounds: { min: number; max: number } | null;
  fromYear: number;
  toYear: number;
  query: string;
  active: boolean;
  onToggleStatus: (status: MapStatus) => void;
  onToggleKind: (kind: MapKind) => void;
  onYearChange: (from: number, to: number) => void;
  onPreset: (preset: PeriodPreset) => void;
  onQueryChange: (value: string) => void;
  onReset: () => void;
}

export function AtlasFiltersPanel({
  features,
  filteredCount,
  statuses,
  kinds,
  bounds,
  fromYear,
  toYear,
  query,
  active,
  onToggleStatus,
  onToggleKind,
  onYearChange,
  onPreset,
  onQueryChange,
  onReset,
}: AtlasFiltersProps) {
  const base = useMemo<AtlasFilterValues>(() => ({ statuses: [...mapStatuses], kinds, fromYear, toYear, query }), [kinds, fromYear, toYear, query]);
  const statusCounts = useMemo(() => countStatuses(filterFeatures(features, base)), [features, base]);
  const kindCounts = useMemo(() => {
    const allKinds = filterFeatures(features, { ...base, kinds: [...mapKinds] });
    const counts: Record<MapKind, number> = { place: 0, association: 0, migration: 0, region: 0 };
    for (const feature of allKinds) counts[feature.kind] += 1;
    return counts;
  }, [features, base]);
  const presets = useMemo(() => periodPresets(bounds), [bounds]);
  const eraActive = useMemo(() => eraOfWindow(fromYear, toYear), [fromYear, toYear]);
  const rangeSpan = bounds ? Math.max(1, bounds.max - bounds.min) : 1;

  return (
    <aside className="atlas-filters" aria-label="تصفية الأطلس">
      <div className="atlas-panel-head">
        <div>
          <div className="eyebrow">التصفية</div>
          <h2>ما تراه الآن</h2>
        </div>
        <button type="button" className="atlas-reset" onClick={onReset} disabled={!active}>
          <RotateCcw size={13} />
          إعادة ضبط
        </button>
      </div>

      <div className="atlas-filter-group">
        <div className="atlas-filter-label">
          <span>الحالة</span>
          <small>{statuses.length} / {mapStatuses.length}</small>
        </div>
        <div className="atlas-status-pills">
          {mapStatuses.map((status) => {
            const copy = mapStatusVocabulary[status];
            const pressed = statuses.includes(status);
            return (
              <button
                type="button"
                key={status}
                className={`atlas-status-pill${pressed ? " atlas-status-pill-active" : ""}${copy.seal ? " atlas-status-pill-seal" : ""}${copy.hollow ? " atlas-status-pill-hollow" : ""}`}
                aria-pressed={pressed}
                onClick={() => onToggleStatus(status)}
                title={copy.description}
              >
                <span className="atlas-status-glyph" aria-hidden="true" style={{ color: copy.stroke }}>{copy.glyph}</span>
                <span>{copy.label}</span>
                <b aria-label={`${formatMapCount(statusCounts[status])} إشارة`}>{formatMapCount(statusCounts[status])}</b>
              </button>
            );
          })}
        </div>
      </div>

      <div className="atlas-filter-group">
        <div className="atlas-filter-label">
          <span>النوع</span>
          <small>{kinds.length} / {mapKinds.length}</small>
        </div>
        <div className="atlas-kind-pills">
          {mapKinds.map((kind) => {
            const pressed = kinds.includes(kind);
            return (
              <button
                type="button"
                key={kind}
                className={`atlas-kind-pill${pressed ? " atlas-kind-pill-active" : ""}`}
                aria-pressed={pressed}
                onClick={() => onToggleKind(kind)}
              >
                {mapKindVocabulary[kind]}
                <b>{formatMapCount(kindCounts[kind])}</b>
              </button>
            );
          })}
        </div>
      </div>

      <div className="atlas-filter-group">
        <div className="atlas-filter-label">
          <span>الفترة</span>
          <small>{bounds ? "بالسنة" : "لا تواريخ"}</small>
        </div>
        {bounds ? (
          <>
            <div className="atlas-year-readout" aria-live="polite">
              <CalendarDays size={13} />
              <span>{formatMapCount(fromYear)} — {formatMapCount(toYear)}</span>
            </div>
            <div className="atlas-range" style={{ "--from": `${((fromYear - bounds.min) / rangeSpan) * 100}%`, "--to": `${((toYear - bounds.min) / rangeSpan) * 100}%` } as CSSProperties}>
              <div className="atlas-range-track" aria-hidden="true" />
              <div className="atlas-range-fill" aria-hidden="true" />
              <input
                type="range"
                min={bounds.min}
                max={bounds.max}
                value={fromYear}
                aria-label="من سنة"
                onChange={(event) => onYearChange(Math.min(Number(event.target.value), toYear), toYear)}
              />
              <input
                type="range"
                min={bounds.min}
                max={bounds.max}
                value={toYear}
                aria-label="إلى سنة"
                onChange={(event) => onYearChange(fromYear, Math.max(Number(event.target.value), fromYear))}
              />
            </div>
            <div className="atlas-presets" aria-label="فترات مقترحة">
              {presets.map((preset) => (
                <button
                  type="button"
                  key={preset.id}
                  className={`atlas-preset${fromYear === preset.from && toYear === preset.to ? " atlas-preset-active" : ""}`}
                  onClick={() => onPreset(preset)}
                >
                  {preset.label}
                </button>
              ))}
            </div>
          </>
        ) : (
          <p className="atlas-filter-empty">لا تحمل الإشارات المعروضة تواريخ. يبقى كل شيء ظاهراً.</p>
        )}
      </div>

      {/* The era chips are the vocabulary on top of the same window: a chip
          sets from/to, and the scrubber carries whatever a chip set. */}
      <div className="atlas-filter-group">
        <div className="atlas-filter-label">
          <span>العهود</span>
          <small>{eraActive ? eraActive.label : "بلا عهد"}</small>
        </div>
        <div className="atlas-eras" aria-label="العهود المعايَرة">
          {atlasEras.map((era) => {
            const pressed = eraActive?.id === era.id;
            return (
              <button
                type="button"
                key={era.id}
                className={`atlas-era-pill${pressed ? " atlas-era-pill-active" : ""}`}
                aria-pressed={pressed}
                onClick={() => onYearChange(era.from, era.to)}
              >
                {era.label}
                <small>{`${era.from}—${era.to}`}</small>
              </button>
            );
          })}
        </div>
      </div>

      <label className="atlas-search-field">
        <Search size={14} />
        <input
          type="search"
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder="ابحث بموضع أو كيان…"
          aria-label="ابحث بموضع أو كيان"
        />
      </label>

      <div className="atlas-filter-summary" aria-live="polite">
        <strong>{formatMapCount(filteredCount)}</strong>
        <span>إشارة بعد التصفية</span>
      </div>
    </aside>
  );
}
