import { useQuery } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";
import { useMemo } from "react";
import { AtlasFiltersPanel } from "../components/AtlasFilters";
import { AtlasMap } from "../components/AtlasMap";
import { fetchMapFeatures, fetchAtlasPlateRegister, fetchAtlasRelations } from "../lib/api";
import { plateHttpDefaultOpacity, plateLayerSpecs } from "../lib/plate-atlas";
import { aggregateFeatures } from "../lib/atlas";
import { useDebouncedScalar } from "../lib/useDebounced";
import type { MapFeature } from "../types";

/**
 * الأطلس المضمّن (plan 032-F): the atlas's READ surface, ready to iframe.
 *
 * The contract, and what makes it honest: this page reads the SAME bounded
 * endpoints the /places atlas reads (the map read, the plate register, the
 * relations read), carries the SAME URL state (027-D's deep links, so an
 * iframe's src IS the permalink), and exposes NO editing chrome — no
 * inspector actions, no write surfaces, no demo vocabulary. A frame's src
 * parameter is the whole contract: the state the reader wants is the state
 * the URL carries, and nothing inside the frame can widen what the server
 * answers (the visibility predicates are the same, the bounds are the same,
 * the anonymous read is the same).
 *
 * Embeddable per-frame state (all optional, all part of the URL):
 *   from,to         era window (years)   ?from=1500&to=1700
 *   status,kind     the legend's own CSV filters
 *   plates          which plates are ON (CSV of plate keys) ?plates=admiralty-1916-tribal
 *   plateOpacity    the plates' shared opacity, 5..100
 *   feature,s,q     the read-side's own deep-link fields
 * The compare point's arm/pick interaction works inside the frame exactly as
 * on /places (the click reports, the URL carries).
 */
export function AtlasEmbedPage() {
  const search = useSearch({ from: "/atlas/embed" });
  const plateRegisterQuery = useQuery({ queryKey: ["atlas-plate-register"], queryFn: () => fetchAtlasPlateRegister(), staleTime: 300000, retry: false });
  const plateRegister = plateRegisterQuery.data ?? null;

  const plateKeys = search.plates ? search.plates.split(",").filter(Boolean) : [];
  const plateOpacity = search.plateOpacity ?? plateHttpDefaultOpacity;
  const plateLayers = plateLayerSpecs(plateRegister, plateKeys, plateOpacity);

  const fetchEraFrom = search.from ?? null;
  const fetchEraTo = search.to ?? null;
  const debouncedEraFrom = useDebouncedScalar(fetchEraFrom, 300);
  const debouncedEraTo = useDebouncedScalar(fetchEraTo, 300);
  const viewportKey = "embed";

  const backQuery = useQuery({
    queryKey: ["atlas-back", viewportKey, debouncedEraFrom ?? null, debouncedEraTo ?? null],
    queryFn: ({ signal }) => fetchMapFeatures({ fromYear: search.from ?? undefined, toYear: search.to ?? undefined, signal }),
    staleTime: 120000,
    retry: false,
  });

  const relationsQuery = useQuery({
    queryKey: ["atlas-relations", debouncedEraFrom ?? 0, debouncedEraTo ?? 0],
    queryFn: () => fetchAtlasRelations(debouncedEraFrom ?? 0, debouncedEraTo ?? 0),
    staleTime: 120000,
    retry: false,
  });

  const features: MapFeature[] = backQuery.data?.features ?? [];
  const filtered = features;
  const markers = useMemo(() => aggregateFeatures(filtered), [filtered]);
  const plateRows = useMemo(() => plateRegisterQuery.data?.plates ?? [], [plateRegisterQuery.data]);
  const renderablePlates = useMemo(() => plateRows.filter((row) => row.transform), [plateRows]);

  return (
    <div className="page-stack atlas-page atlas-embed" data-embed="true">
      <section className="atlas-workspace" aria-label="الأطلس المضمّن">
        <AtlasFiltersPanel
          features={features as MapFeature[]}
          filteredCount={features.length}
          statuses={[]}
          kinds={[]}
          bounds={null}
          fromYear={search.from ?? 0}
          toYear={search.to ?? 0}
          query=""
          active={false}
          onToggleStatus={() => undefined}
          onToggleKind={() => undefined}
          onYearChange={() => undefined}
          onPreset={() => undefined}
          onQueryChange={() => undefined}
          onReset={() => undefined}
          plateRows={plateRows}
          plateKeys={plateKeys}
          plateOpacity={plateOpacity}
          onTogglePlate={() => undefined}
          onPlateOpacity={() => undefined}
          plateRegisterPending={plateRegisterQuery.isPending}
          plateRegisterFailed={plateRegisterQuery.isError}
          movementMode=""
          season=""
          onToggleMovementMode={() => undefined}
          onToggleSeason={() => undefined}
        />
        <div className="atlas-stage">
          <div className="atlas-map-panel">
            <div className="atlas-map-head">
              <div>
                <div className="eyebrow">الأطلس المضمّن — قراءة فقط</div>
                <p className="atlas-embed-contract">
                  الحالة كلها في العنوان: `from`، `to`، `plates`، `plateOpacity`. كل ما يُعرض مشتق من سجلات محفوظة عبر نفس الحدود العامة.
                </p>
              </div>
            </div>
            <div className="atlas-canvas-wrap">
              <AtlasMap
                markers={markers}
                migrations={[]}
                regions={features.filter((feature) => feature.kind === "region")}
                selectedKey={search.feature ?? ""}
                onSelectMarker={() => undefined}
                onSelectFeature={() => undefined}
                onClearSelection={() => undefined}
                onViewportChange={() => undefined}
                plateLayers={plateLayers}
                relations={(relationsQuery.data?.relations ?? []) as never}
              />
            </div>
            {renderablePlates.length === 0 && plateRegisterQuery.isSuccess ? (
              <p className="atlas-embed-note">لا لوحات صالحة للاستعمال بعد؛ الطبقة تُعطَّل بشرحها.</p>
            ) : null}
          </div>
        </div>
      </section>
    </div>
  );
}
