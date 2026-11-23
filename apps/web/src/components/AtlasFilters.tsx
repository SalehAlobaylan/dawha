import { Link } from "@tanstack/react-router";
import { CalendarDays, Compass, ExternalLink, Layers, LocateFixed, RotateCcw, Search, SlidersHorizontal, X } from "lucide-react";
import { useMemo, useState, type CSSProperties } from "react";
import type { AtlasPlateRegisterRow } from "../lib/api";
import { movementModeLabels, movementModes, movementSeasonLabels, movementSeasons, type MovementMode, type MovementSeason } from "../lib/movement-events";
import type { MapFeature } from "../types";
import { countStatuses, filterFeatures, periodPresets, type AtlasFilters as AtlasFilterValues, type PeriodPreset } from "../lib/atlas";
import { atlasEras, eraOfWindow } from "../lib/atlas-periods";
import { hasPlateTribeLabels } from "../lib/plate-atlas";
import {
  formatMapCount,
  mapKindVocabulary,
  mapKinds,
  mapStatusVocabulary,
  mapStatuses,
  type MapKind,
  type MapStatus,
} from "../lib/map-vocabulary";

type PlateScope = "all" | "peninsula" | "regional";

const plateDisplayMeta: Readonly<Record<string, { eraLabel: string; scope: "peninsula" | "regional"; regionTag?: string }>> = {
  "arabia-600ce-tribes": { eraLabel: "٦٠٠م", scope: "peninsula", regionTag: "الجزيرة" },
  "berghaus-1835-arabia": { eraLabel: "١٨٣٥", scope: "peninsula", regionTag: "الجزيرة" },
  "dickson-1914-tribal": { eraLabel: "١٩١٤", scope: "peninsula", regionTag: "قبائل + حدود" },
  "admiralty-1916-tribal": { eraLabel: "١٩١٦", scope: "peninsula", regionTag: "قبائل" },
  "admiralty-1916-districts": { eraLabel: "١٩١٦", scope: "peninsula", regionTag: "أقاليم ومدن" },
  "ottoman-asir-pre1919": { eraLabel: "قبل ١٩١٩", scope: "regional", regionTag: "عسير وتهامة" },
  "iraq-1930-hashimi": { eraLabel: "١٩٣٠", scope: "regional", regionTag: "العراق والبادية" },
  "thomas-1931-rub-al-khali": { eraLabel: "١٩٣١", scope: "regional", regionTag: "الربع الخالي وعُمان" },
  "yemen-south-arabia-tribes": { eraLabel: "مرجعية", scope: "regional", regionTag: "اليمن والجنوب" },
};

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
  /** The plate register (plan 032-A/B): every plate and its honest state. */
  plateRows: AtlasPlateRegisterRow[];
  /** Which plates are ON — the URL's own list. */
  plateKeys: string[];
  /** The overlay's shared opacity, 5..100. */
  plateOpacity: number;
  onTogglePlate: (plateKey: string) => void;
  onFocusPlate?: (plateKey: string) => void;
  onPlateOpacity: (value: number) => void;
  plateRegisterPending: boolean;
  plateRegisterFailed: boolean;
  /** Archival Ottoman Harita 93677 (c. 1910) Najd & Arabia GIS polygons layer. */
  ottomanHaritaOn?: boolean;
  onToggleOttomanHarita?: () => void;
  onFocusOttomanHarita?: () => void;
  /** The movement filters (plan 033-C): the roads' own mode + season. */
  movementMode: string;
  season: string;
  onToggleMovementMode: (mode: string) => void;
  onToggleSeason: (season: string) => void;
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
  plateRows,
  plateKeys,
  plateOpacity,
  onTogglePlate,
  onFocusPlate,
  onPlateOpacity,
  plateRegisterPending,
  plateRegisterFailed,
  ottomanHaritaOn = false,
  onToggleOttomanHarita,
  onFocusOttomanHarita,
  movementMode,
  season,
  onToggleMovementMode,
  onToggleSeason,
}: AtlasFiltersProps) {
  const [plateScope, setPlateScope] = useState<PlateScope>("all");
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
  // Renderable plates first so the user sees interactive maps before disabled entries.
  const sortedPlates = useMemo(
    () => [...plateRows].sort((left, right) => Number(Boolean(right.transform)) - Number(Boolean(left.transform))),
    [plateRows],
  );
  const visiblePlates = useMemo(() => {
    if (plateScope === "all") return sortedPlates;
    return sortedPlates.filter((row) => (plateDisplayMeta[row.plateKey]?.scope ?? "peninsula") === plateScope);
  }, [sortedPlates, plateScope]);
  const activeOverlayCount = plateKeys.length + (ottomanHaritaOn ? 1 : 0);

  return (
    <aside className="atlas-filters" aria-label="تصفية الأطلس">
      <div className="atlas-filters-top">
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

        <label className="atlas-search-field">
          <Search size={14} aria-hidden="true" />
          <input
            type="search"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="ابحث بموضع أو كيان…"
            aria-label="ابحث بموضع أو كيان"
          />
          {query ? (
            <button
              type="button"
              className="atlas-search-clear"
              aria-label="مسح البحث"
              onClick={() => onQueryChange("")}
            >
              <X size={13} />
            </button>
          ) : null}
        </label>

        <div className="atlas-filter-summary" aria-live="polite">
          <strong>{formatMapCount(filteredCount)}</strong>
          <span>إشارة بعد التصفية</span>
        </div>
      </div>

      {/* لوحات المرجع: flagship visual map overlays placed right near the top */}
      <details className="atlas-filter-section" open>
        <summary className="atlas-filter-section-head">
          <span className="atlas-filter-section-title">
            <Layers size={13} aria-hidden="true" />
            لوحات المرجع
          </span>
          <small>{activeOverlayCount > 0 ? `${activeOverlayCount} مفعّلة` : `${sortedPlates.length} خريطة`}</small>
        </summary>
        <div className="atlas-filter-section-body">
          {plateRegisterPending ? (
            <p className="atlas-filter-empty">قراءة سجل اللوحات…</p>
          ) : plateRegisterFailed ? (
            <p className="atlas-filter-empty">تعذّرت قراءة سجل اللوحات؛ الطبقة متوقفة.</p>
          ) : sortedPlates.length === 0 ? (
            <p className="atlas-filter-empty">لا توجد لوحات مرجعية منشورة بعد.</p>
          ) : (
            <>
              {sortedPlates.length > 3 ? (
                <div className="atlas-plate-scope-tabs" role="tablist" aria-label="تصنيف اللوحات التاريخية">
                  {([
                    ["all", "الكل"],
                    ["peninsula", "الجزيرة"],
                    ["regional", "إقليمية"],
                  ] as const).map(([scopeKey, scopeLabel]) => (
                    <button
                      key={scopeKey}
                      type="button"
                      role="tab"
                      aria-selected={plateScope === scopeKey}
                      className={`atlas-plate-scope-tab${plateScope === scopeKey ? " atlas-plate-scope-tab-active" : ""}`}
                      onClick={() => setPlateScope(scopeKey)}
                    >
                      {scopeLabel}
                    </button>
                  ))}
                </div>
              ) : null}
              <div className="atlas-plate-rows" role="group" aria-label="طبقات اللوحات المرجعية">
                {visiblePlates.map((row) => {
                  const on = plateKeys.includes(row.plateKey);
                  const renderable = Boolean(row.transform);
                  const meta = plateDisplayMeta[row.plateKey];
                  const yearLabel = meta?.eraLabel || (row.plateDate ? row.plateDate.slice(0, 4) : "");
                  return (
                    <div key={row.plateKey} className={`atlas-plate-card${on ? " atlas-plate-card-on" : ""}${!renderable ? " atlas-plate-card-disabled" : ""}`}>
                      <button
                        type="button"
                        className={`atlas-plate-toggle${on ? " atlas-plate-toggle-on" : ""}`}
                        aria-pressed={on}
                        disabled={!renderable}
                        onClick={() => onTogglePlate(row.plateKey)}
                        title={renderable ? undefined : row.disabledCaption}
                      >
                        <span className="atlas-plate-toggle-title">{row.titleAr || row.plateKey}</span>
                        {yearLabel ? <small className="atlas-plate-year">{yearLabel}</small> : null}
                      </button>
                      <div className="atlas-plate-card-foot">
                        {renderable ? (
                          <span className="atlas-plate-review-chip">
                            {meta?.regionTag ? `${meta.regionTag} · ` : ""}
                            {row.transform?.needsReview ? "تقريبية" : (row.transform?.certainty === "approximate" ? "تقريبية" : row.transform?.certainty)}
                          </span>
                        ) : (
                          <span className="atlas-plate-disabled-chip">معطّلة · بلا إحداثيات</span>
                        )}
                        {renderable && onFocusPlate ? (
                          <button
                            type="button"
                            className="atlas-plate-fit-btn"
                            title="تركيز الخريطة على حدود هذه اللوحة"
                            aria-label={`تركيز على ${row.titleAr || row.plateKey}`}
                            onClick={() => onFocusPlate(row.plateKey)}
                          >
                            <LocateFixed size={13} />
                            <span>تركيز</span>
                          </button>
                        ) : !renderable && row.sourceId ? (
                          <Link
                            className="atlas-plate-fit-btn atlas-plate-source-link"
                            to="/sources"
                            search={{ sourceId: row.sourceId }}
                          >
                            <ExternalLink size={12} />
                            <span>سجل المصدر</span>
                          </Link>
                        ) : null}
                      </div>
                      {!renderable && row.disabledCaption ? (
                        <p className="atlas-plate-disabled-note">{row.disabledCaption}</p>
                      ) : null}
                    </div>
                  );
                })}
                {onToggleOttomanHarita && plateScope !== "regional" ? (
                  <div className={`atlas-plate-card atlas-plate-card-vector${ottomanHaritaOn ? " atlas-plate-card-on" : ""}`}>
                    <button
                      type="button"
                      className={`atlas-plate-toggle${ottomanHaritaOn ? " atlas-plate-toggle-on" : ""}`}
                      aria-pressed={ottomanHaritaOn}
                      onClick={onToggleOttomanHarita}
                    >
                      <span className="atlas-plate-toggle-title">حدود خريطة ٩٣٦٧٧ العثمانية (نجد والأحساء)</span>
                      <small className="atlas-plate-year">١٩١٠</small>
                    </button>
                    <div className="atlas-plate-card-foot">
                      <span className="atlas-plate-review-chip atlas-plate-ottoman-chip">٣٦ نطاقاً + إحصاء عثماني</span>
                      {onFocusOttomanHarita ? (
                        <button
                          type="button"
                          className="atlas-plate-fit-btn"
                          title="تركيز الخريطة على نطاق خريطة ٩٣٦٧٧ العثمانية"
                          onClick={onFocusOttomanHarita}
                        >
                          <LocateFixed size={13} />
                          <span>تركيز</span>
                        </button>
                      ) : null}
                    </div>
                  </div>
                ) : null}
              </div>
              {sortedPlates.some((row) => row.transform && plateKeys.includes(row.plateKey) && hasPlateTribeLabels(row)) ? (
                <p className="atlas-filter-hint">أسماء القبائل معرّبة من اللوحة التاريخية. قرّب الخريطة لإظهار المزيد.</p>
              ) : null}
              <label className="atlas-plate-opacity">
                <span>الشفافية</span>
                <input
                  type="range"
                  min={5}
                  max={100}
                  step={5}
                  value={plateOpacity}
                  disabled={plateKeys.length === 0}
                  aria-label="شفافية طبقة اللوحات"
                  onChange={(event) => onPlateOpacity(Number(event.target.value))}
                />
                <b>{plateOpacity}%</b>
              </label>
            </>
          )}
        </div>
      </details>

      {/* الحالة والنوع: clean 2-column chip grids */}
      <details className="atlas-filter-section" open>
        <summary className="atlas-filter-section-head">
          <span className="atlas-filter-section-title">
            <SlidersHorizontal size={13} aria-hidden="true" />
            الحالة والنوع
          </span>
          <small>{statuses.length}/{mapStatuses.length} · {kinds.length}/{mapKinds.length}</small>
        </summary>
        <div className="atlas-filter-section-body">
          <div className="atlas-sublabel">
            <span>حالة التوثيق</span>
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
                  <span className="atlas-pill-text">{copy.label}</span>
                  <b aria-label={`${formatMapCount(statusCounts[status])} إشارة`}>{formatMapCount(statusCounts[status])}</b>
                </button>
              );
            })}
          </div>

          <div className="atlas-sublabel">
            <span>نوع المعلم</span>
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
                  <span className="atlas-pill-text">{mapKindVocabulary[kind]}</span>
                  <b>{formatMapCount(kindCounts[kind])}</b>
                </button>
              );
            })}
          </div>
        </div>
      </details>

      {/* الفترة والعهود: combined temporal lens */}
      <details className="atlas-filter-section" open>
        <summary className="atlas-filter-section-head">
          <span className="atlas-filter-section-title">
            <CalendarDays size={13} aria-hidden="true" />
            الفترة والعهود
          </span>
          <small>{eraActive ? eraActive.label : bounds ? `${fromYear}—${toYear}` : "بلا تواريخ"}</small>
        </summary>
        <div className="atlas-filter-section-body">
          {bounds ? (
            <div className="atlas-period-box">
              <div className="atlas-year-readout" aria-live="polite">
                <span>النطاق الزمني</span>
                <strong>{formatMapCount(fromYear)} — {formatMapCount(toYear)}</strong>
              </div>
              <div className="atlas-range" style={{ "--from": `${Math.min(100, Math.max(0, ((fromYear - bounds.min) / rangeSpan) * 100))}%`, "--to": `${Math.min(100, Math.max(0, ((toYear - bounds.min) / rangeSpan) * 100))}%` } as CSSProperties}>
                <div className="atlas-range-track" aria-hidden="true" />
                <div className="atlas-range-fill" aria-hidden="true" />
                <input
                  type="range"
                  min={bounds.min}
                  max={bounds.max}
                  value={Math.min(bounds.max, Math.max(bounds.min, fromYear))}
                  aria-label="من سنة"
                  onChange={(event) => onYearChange(Math.min(Number(event.target.value), toYear), toYear)}
                />
                <input
                  type="range"
                  min={bounds.min}
                  max={bounds.max}
                  value={Math.min(bounds.max, Math.max(bounds.min, toYear))}
                  aria-label="إلى سنة"
                  onChange={(event) => onYearChange(fromYear, Math.max(Number(event.target.value), fromYear))}
                />
              </div>
              {presets.length > 0 ? (
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
              ) : null}
            </div>
          ) : (
            <p className="atlas-filter-empty">لا تحمل الإشارات المعروضة تواريخ. يبقى كل شيء ظاهراً.</p>
          )}

          <div className="atlas-sublabel">
            <span>العهود التاريخية</span>
            <small>{eraActive ? eraActive.label : "الكل"}</small>
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
                  onClick={() => {
                    if (pressed && bounds) {
                      onYearChange(bounds.min, bounds.max);
                    } else {
                      onYearChange(era.from, era.to);
                    }
                  }}
                >
                  <span>{era.label}</span>
                  <small>{`${era.from}—${era.to}`}</small>
                </button>
              );
            })}
          </div>
        </div>
      </details>

      {/* مرشحات الحركة: clearly separated transport modes vs seasons */}
      <details className="atlas-filter-section" open>
        <summary className="atlas-filter-section-head">
          <span className="atlas-filter-section-title">
            <Compass size={13} aria-hidden="true" />
            مرشحات الحركة
          </span>
          <small>{movementMode ? movementModeLabels[movementMode as MovementMode] : season ? movementSeasonLabels[season as MovementSeason] : "الكل"}</small>
        </summary>
        <div className="atlas-filter-section-body" role="group" aria-label="مرشحات الحركة">
          <div className="atlas-sublabel"><span>وسيلة التنقل</span></div>
          <div className="atlas-movement-chips">
            {movementModes.map((mode) => (
              <button
                type="button"
                key={mode}
                className={`atlas-movement-pill${movementMode === mode ? " atlas-movement-pill-active" : ""}`}
                aria-pressed={movementMode === mode}
                onClick={() => onToggleMovementMode(mode)}
              >
                {movementModeLabels[mode]}
              </button>
            ))}
          </div>
          <div className="atlas-sublabel"><span>الموسم</span></div>
          <div className="atlas-movement-chips">
            {movementSeasons.map((movementSeason) => (
              <button
                type="button"
                key={movementSeason}
                className={`atlas-movement-pill${season === movementSeason ? " atlas-movement-pill-active" : ""}`}
                aria-pressed={season === movementSeason}
                onClick={() => onToggleSeason(movementSeason)}
              >
                {movementSeasonLabels[movementSeason]}
              </button>
            ))}
          </div>
        </div>
      </details>
    </aside>
  );
}
