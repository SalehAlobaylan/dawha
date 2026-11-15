import * as maplibregl from "maplibre-gl";
import maplibreWorkerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";
import type { FeatureCollection, LineString, Point, Polygon } from "geojson";
import { useEffect, useMemo, useRef, useState } from "react";
import type { MapFeature } from "../types";
import { formatMapCount, mapStatusLabel, mapStatusVocabulary } from "../lib/map-vocabulary";
import {
  migrationArrowCollection,
  migrationCollection,
  regionCollection,
  statusMatchExpression,
  viewportForCoordinates,
  type AtlasMarker,
  type AtlasViewport,
} from "../lib/atlas";
import { coastlineLandCollection, coastlineLineCollection, COASTLINE_BOUNDS } from "../lib/coastline";
import { mapReferenceCaptions } from "../lib/map-reference";
import type { AtlasInferenceSeal } from "../lib/atlas-inference";
import "maplibre-gl/dist/maplibre-gl.css";

// maplibre derives its GeoJSON tile worker URL from `import.meta.url`. Vite's
// pre-bundle copies the entry but not the sibling worker file, so the worker
// 404s and every GeoJSON layer silently never paints (the DOM markers still
// float on top, and nothing in the console connects the two). Bundling the
// worker as an asset and pointing maplibre at it directly paints the layers in
// dev and production alike, before any hand-holding in vite config.
maplibregl.setWorkerUrl(maplibreWorkerUrl);

interface AtlasMapProps {
  markers: AtlasMarker[];
  migrations: MapFeature[];
  regions: MapFeature[];
  selectedKey: string;
  onSelectMarker: (marker: AtlasMarker) => void;
  onSelectFeature: (feature: MapFeature) => void;
  onClearSelection: () => void;
  onViewportChange: (viewport: AtlasViewport) => void;
  focus?: { longitude: number; latitude: number; key: string } | null;
  /** The URL names a feature or place; the arrival flight belongs to it, not to the peninsula fit. */
  skipInitialFit?: boolean;
  resetSignal?: number;
  inferences?: AtlasInferenceSeal[];
  selectedInferenceKey?: string;
  onSelectInference?: (seal: AtlasInferenceSeal) => void;
}

type OverlayPosition = { left: number; top: number; visible: boolean };
type OverlayPositions = {
  markers: Record<string, OverlayPosition>;
  inferences: Record<string, OverlayPosition>;
  regionLabels: Record<string, OverlayPosition>;
  captions: Record<string, OverlayPosition>;
  migration: OverlayPosition | null;
};
type AnyCollection = FeatureCollection<Point | LineString | Polygon>;

/**
 * The status colors are the vocabulary's, and the vocabulary is the only file
 * that holds a hex: the same fill and stroke the legend paints take this match
 * expression apart feature by feature. The builder lives in `lib/atlas.ts`
 * (tested — the silent-layer-death contract is a test there, not a hope);
 * only the maplibre typing lives here.
 */
const statusFills = statusMatchExpression((copy) => copy.fill, "rgba(45, 37, 32, 0.08)") as unknown as maplibregl.ExpressionSpecification;
const statusStrokes = statusMatchExpression((copy) => copy.stroke, "#8b8176") as unknown as maplibregl.ExpressionSpecification;

/**
 * The atlas canvas.
 *
 * MapLibre draws the two geometries that are actually geographic areas: region
 * polygons and migration lines. Markers are DOM buttons projected through
 * `map.project()` on every frame, which is what makes them keyboard-reachable and
 * what lets the monospace count badge keep a real font. They are not percentage
 * positions: panning the map moves them through the same projection the canvas
 * uses, so the two can never disagree about where Riyadh is.
 *
 * There is no tile layer and no border layer. The only geography drawn is the one
 * the API returned.
 */
export function AtlasMap({ markers, migrations, regions, selectedKey, onSelectMarker, onSelectFeature, onClearSelection, onViewportChange, focus, skipInitialFit = false, resetSignal = 0, inferences = [], selectedInferenceKey = "", onSelectInference }: AtlasMapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const repositionRef = useRef<() => void>(() => undefined);
  const fitRef = useRef<() => void>(() => undefined);
  const fitAllRef = useRef<() => void>(() => undefined);
  const resetPendingRef = useRef(false);
  const skipInitialFitRef = useRef(skipInitialFit);
  // One overlay state for everything the canvas projects: markers, inference
  // seals, region labels, reference captions and the selected migration's
  // caption all move on the same frame, so they reconcile in one commit
  // instead of four setStates per animation frame underneath the reader's
  // drag.
  const [overlay, setOverlay] = useState<OverlayPositions>({ markers: {}, inferences: {}, regionLabels: {}, captions: {}, migration: null });
  const [failed, setFailed] = useState(false);

  const markersRef = useRef(markers);
  const migrationsRef = useRef(migrations);
  const regionsRef = useRef(regions);
  const selectedKeyRef = useRef(selectedKey);
  const inferencesRef = useRef(inferences);
  const selectedInferenceKeyRef = useRef(selectedInferenceKey);
  const onSelectMarkerRef = useRef(onSelectMarker);
  const onSelectFeatureRef = useRef(onSelectFeature);
  const onClearSelectionRef = useRef(onClearSelection);
  const onViewportChangeRef = useRef(onViewportChange);
  const onSelectInferenceRef = useRef(onSelectInference);
  markersRef.current = markers;
  migrationsRef.current = migrations;
  regionsRef.current = regions;
  selectedKeyRef.current = selectedKey;
  inferencesRef.current = inferences;
  selectedInferenceKeyRef.current = selectedInferenceKey;
  onSelectMarkerRef.current = onSelectMarker;
  onSelectFeatureRef.current = onSelectFeature;
  onClearSelectionRef.current = onClearSelection;
  onViewportChangeRef.current = onViewportChange;
  onSelectInferenceRef.current = onSelectInference;

  const migrationsData = useMemo(() => migrationCollection(migrations), [migrations]);
  const arrowsData = useMemo(() => migrationArrowCollection(migrations), [migrations]);
  const regionsData = useMemo(() => regionCollection(regions), [regions]);

  useEffect(() => {
    if (!containerRef.current) return undefined;
    let map: maplibregl.Map;
    try {
      map = new maplibregl.Map({
        container: containerRef.current,
        style: {
          version: 8,
          sources: {},
          layers: [{ id: "atlas-background", type: "background", paint: { "background-color": "#ded1b9" } }],
        },
        center: [45.8, 22.8],
        zoom: 4.3,
        attributionControl: false,
        dragRotate: false,
      });
    } catch {
      setFailed(true);
      return undefined;
    }
    mapRef.current = map;
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "bottom-left");
    map.addControl(new maplibregl.ScaleControl({ maxWidth: 110, unit: "metric" }), "bottom-right");

    map.on("error", (event) => {
      if (event.error?.message.toLowerCase().includes("webgl")) setFailed(true);
    });

    const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;

    let fitted = false;
    const fitAll = () => {
      const points: [number, number][] = [];
      for (const marker of markersRef.current) points.push([marker.longitude, marker.latitude]);
      for (const region of regionsRef.current) if (region.ring) points.push(...region.ring);
      for (const migration of migrationsRef.current) {
        if (migration.fromLongitude !== undefined && migration.fromLatitude !== undefined) points.push([migration.fromLongitude, migration.fromLatitude]);
        if (migration.toLongitude !== undefined && migration.toLatitude !== undefined) points.push([migration.toLongitude, migration.toLatitude]);
      }
      // The whole peninsula, not just the features: a map clipped to whatever
      // the archive happens to hold reads as a slice, not as the land it is
      // about. The coastline is the reference it draws, so frame to it.
      points.push([COASTLINE_BOUNDS.minLongitude, COASTLINE_BOUNDS.minLatitude], [COASTLINE_BOUNDS.maxLongitude, COASTLINE_BOUNDS.maxLatitude]);
      const viewport = viewportForCoordinates(points);
      if (!viewport) {
        map.easeTo({ center: [45.8, 22.8], zoom: 4.3, duration: reducedMotion ? 0 : 450 });
        return;
      }
      map.fitBounds(
        [[viewport.minLongitude, viewport.minLatitude], [viewport.maxLongitude, viewport.maxLatitude]],
        { padding: 54, maxZoom: 6.4, duration: reducedMotion ? 0 : 450 },
      );
    };
    fitAllRef.current = fitAll;
    fitRef.current = () => {
      if (fitted) return;
      fitted = true;
      // A deep link owns the first flight (the reader asked for a feature, not
      // for a continent that then flies away): the focus flight covers the
      // arrival, and reset still frames the whole region when asked.
      if (skipInitialFitRef.current) {
        skipInitialFitRef.current = false;
        return;
      }
      fitAll();
    };

    map.on("load", () => {
      addAtlasImages(map);
      map.addSource("atlas-coastline-land", { type: "geojson", data: coastlineLandCollection() as AnyCollection });
      map.addSource("atlas-coastline-line", { type: "geojson", data: coastlineLineCollection() as AnyCollection });
      map.addSource("atlas-graticule", { type: "geojson", data: graticuleCollection() });
      map.addSource("atlas-regions", { type: "geojson", data: regionCollection(regionsRef.current) as AnyCollection });
      map.addSource("atlas-migrations", { type: "geojson", data: migrationCollection(migrationsRef.current) as AnyCollection });
      map.addSource("atlas-arrows", { type: "geojson", data: migrationArrowCollection(migrationsRef.current) as AnyCollection });

      // A faint coastline, for orientation only. It is geographic chrome, not
      // archive data: drawn soft, labelled approximate in the page note, never a
      // claim. No tiles, no borders — land only.
      map.addLayer({ id: "atlas-coastline-land", type: "fill", source: "atlas-coastline-land", paint: { "fill-color": "#c9b98d", "fill-opacity": 0.38 } });
      map.addLayer({ id: "atlas-coastline-line", type: "line", source: "atlas-coastline-line", paint: { "line-color": "#2d2520", "line-width": 1.1, "line-opacity": 0.45, "line-dasharray": [4, 2] } });

      // The graticule is coordinate chrome, not geography: it says where degrees
      // fall, and never pretends to be a border.
      map.addLayer({ id: "atlas-graticule-lines", type: "line", source: "atlas-graticule", paint: { "line-color": "rgba(45, 37, 32, 0.13)", "line-width": 0.7, "line-dasharray": [1, 3] } });

      // Regions are areas, not dots. A disputed region is hatched as well as
      // outlined: a border whose very existence is contested must look
      // contested. And a region that carries no dated validity under an era
      // window drops to the vocabulary's weakest voice — half the fill ink and
      // a dashed outline — because identical drawing under every era would
      // read as "this region existed in the era", which its NULL columns never
      // say. The voice stays legible on purpose: on the all-undated demo
      // vocabulary every chip weakens every region, and a region seen in the
      // weakest voice must still be seen to be a region. (The first cut of
      // this voice had a 0.05 fill and a 0.4 outline and a reader read the
      // layer as gone — the weakest voice is a state, not an absence.)
      map.addLayer({ id: "atlas-region-fill", type: "fill", source: "atlas-regions", paint: { "fill-color": statusFills, "fill-opacity": ["case", ["==", ["get", "dated"], "false"], 0.08, 0.16] as unknown as maplibregl.ExpressionSpecification } });
      map.addLayer({ id: "atlas-region-hatch", type: "fill", source: "atlas-regions", filter: ["==", ["get", "status"], "disputed"] as maplibregl.FilterSpecification, paint: { "fill-pattern": "atlas-hatch", "fill-opacity": 0.5 } });
      map.addLayer({ id: "atlas-region-outline", type: "line", source: "atlas-regions", filter: ["!=", ["get", "dated"], "false"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 1.6, "line-opacity": 0.8 } });
      map.addLayer({ id: "atlas-region-outline-undated", type: "line", source: "atlas-regions", filter: ["==", ["get", "dated"], "false"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 1.4, "line-opacity": 0.55, "line-dasharray": [2, 3] } });

      map.addLayer({ id: "atlas-migration-certain", type: "line", source: "atlas-migrations", filter: ["==", ["get", "certainty"], "precise"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 2, "line-opacity": 0.85 } });
      map.addLayer({ id: "atlas-migration-approximate", type: "line", source: "atlas-migrations", filter: ["==", ["get", "certainty"], "approximate"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 2, "line-opacity": 0.8, "line-dasharray": [3, 2] } });
      map.addLayer({ id: "atlas-migration-uncertain", type: "line", source: "atlas-migrations", filter: ["==", ["get", "certainty"], "uncertain"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 1.8, "line-opacity": 0.72, "line-dasharray": [1, 2] } });
      map.addLayer({ id: "atlas-migration-arrows", type: "symbol", source: "atlas-arrows", layout: { "icon-image": "atlas-arrow", "icon-size": 0.72, "icon-rotate": ["get", "bearing"] as unknown as maplibregl.ExpressionSpecification, "icon-rotation-alignment": "map", "icon-allow-overlap": true, "icon-ignore-placement": true } });

      const selectRegion = (event: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
        selectById(event, regionsRef.current, onSelectFeatureRef.current);
      };
      const selectMigration = (event: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
        selectById(event, migrationsRef.current, onSelectFeatureRef.current);
      };
      map.on("click", "atlas-region-fill", selectRegion);
      map.on("click", "atlas-migration-certain", selectMigration);
      map.on("click", "atlas-migration-approximate", selectMigration);
      map.on("click", "atlas-migration-uncertain", selectMigration);
      for (const layer of ["atlas-region-fill", "atlas-migration-certain", "atlas-migration-approximate", "atlas-migration-uncertain"]) {
        map.on("mouseenter", layer, () => { map.getCanvas().style.cursor = "pointer"; });
        map.on("mouseleave", layer, () => { map.getCanvas().style.cursor = ""; });
      }
      map.on("click", (event) => {
        const target = event.originalEvent.target as HTMLElement | null;
        if (target?.closest(".atlas-marker") || target?.closest(".atlas-inference-seal-pin")) return;
        const hits = map.queryRenderedFeatures(event.point, {
          layers: ["atlas-region-fill", "atlas-migration-certain", "atlas-migration-approximate", "atlas-migration-uncertain"],
        });
        if (!hits.length) onClearSelectionRef.current();
      });

      const emitViewport = () => {
        const bounds = map.getBounds();
        onViewportChangeRef.current({
          minLongitude: bounds.getWest(),
          minLatitude: bounds.getSouth(),
          maxLongitude: bounds.getEast(),
          maxLatitude: bounds.getNorth(),
        });
      };
      map.on("moveend", emitViewport);

      const updatePositions = () => {
        const canvas = map.getCanvas();
        const width = canvas.clientWidth;
        const height = canvas.clientHeight;
        const next: OverlayPositions = { markers: {}, inferences: {}, regionLabels: {}, captions: {}, migration: null };
        const project = (longitude: number, latitude: number, marginX: number, marginY: number): OverlayPosition => {
          const point = map.project([longitude, latitude]);
          return {
            left: point.x,
            top: point.y,
            visible: point.x > -marginX && point.y > -marginY && point.x < width + marginX && point.y < height + marginY,
          };
        };
        for (const marker of markersRef.current) {
          next.markers[marker.key] = project(marker.longitude, marker.latitude, 60, 60);
        }
        for (const region of regionsRef.current) {
          if (region.longitude === undefined || region.latitude === undefined) continue;
          next.regionLabels[region.id] = project(region.longitude, region.latitude, 80, 40);
        }
        for (const caption of mapReferenceCaptions) {
          next.captions[caption.label] = project(caption.longitude, caption.latitude, 60, 30);
        }
        const selectedMigration = migrationsRef.current.find((migration) => migration.id === selectedKeyRef.current);
        if (selectedMigration?.fromLongitude !== undefined && selectedMigration.fromLatitude !== undefined && selectedMigration.toLongitude !== undefined && selectedMigration.toLatitude !== undefined) {
          next.migration = project(
            (selectedMigration.fromLongitude + selectedMigration.toLongitude) / 2,
            (selectedMigration.fromLatitude + selectedMigration.toLatitude) / 2,
            60, 30,
          );
        }
        for (const seal of inferencesRef.current) {
          next.inferences[seal.key] = project(seal.longitude, seal.latitude, 60, 60);
        }
        setOverlay(next);
      };
      repositionRef.current = updatePositions;
      updatePositions();
      fitRef.current();
      emitViewport();
    });

    let frame = 0;
    const schedule = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        repositionRef.current();
      });
    };
    map.on("move", schedule);
    map.on("zoom", schedule);
    map.on("resize", schedule);

    const observer = new ResizeObserver(() => map.resize());
    observer.observe(containerRef.current);

    // A deep link may select a feature before the style has loaded. Keep the
    // request until the map can honour it, and never animate when the reader
    // has asked motion to stop. A mount-time focus also claims the first
    // flight: fitting the peninsula first and then flying to the feature is
    // two opposite flights — the feature flight is the only one.
    if (focus) {
      skipInitialFitRef.current = true;
      map.once("load", () => {
        map.easeTo({ center: [focus.longitude, focus.latitude], zoom: Math.max(map.getZoom(), 5.8), duration: reducedMotion ? 0 : 550 });
      });
    }

    return () => {
      if (frame) cancelAnimationFrame(frame);
      observer.disconnect();
      repositionRef.current = () => undefined;
      fitRef.current = () => undefined;
      fitAllRef.current = () => undefined;
      map.remove();
      mapRef.current = null;
    };
    // The map is created once. Data and handlers are read through refs, so a
    // filter change updates layers without tearing down the WebGL context.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (resetSignal <= 0) return;
    resetPendingRef.current = true;
    fitAllRef.current();
  }, [resetSignal]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    setSourceData(map, "atlas-migrations", migrationsData);
    setSourceData(map, "atlas-arrows", arrowsData);
    setSourceData(map, "atlas-regions", regionsData);
    repositionRef.current();
    if (resetPendingRef.current) {
      resetPendingRef.current = false;
      fitAllRef.current();
    } else {
      fitRef.current();
    }
  }, [arrowsData, migrationsData, regionsData, selectedKey, inferences]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !focus) return;
    const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
    map.easeTo({ center: [focus.longitude, focus.latitude], zoom: Math.max(map.getZoom(), 6.2), duration: reduced ? 0 : 600 });
  }, [focus]);

  const selectedMigrationPeriod = useMemo(() => {
    if (!selectedKey) return undefined;
    const migration = migrations.find((item) => item.id === selectedKey);
    return migration?.period ?? undefined;
  }, [migrations, selectedKey]);

  const visibleMarkers = markers.filter((marker) => overlay.markers[marker.key]?.visible);
  const visibleInferences = inferences.filter((seal) => overlay.inferences[seal.key]?.visible);
  const visibleRegionLabels = regions.filter((region) => overlay.regionLabels[region.id]?.visible);

  return (
    <div className="atlas-map" ref={containerRef} role="region" aria-label="خريطة الأطلس">
      <div className="atlas-marker-overlay">
        {visibleMarkers.map((marker) => {
          const position = overlay.markers[marker.key];
          const copy = mapStatusVocabulary[marker.status];
          const selected = selectedKey === marker.key;
          return (
            <button
              type="button"
              key={marker.key}
              className={`atlas-marker atlas-marker-${marker.status}${selected ? " atlas-marker-selected" : ""}${copy.seal ? " atlas-marker-seal" : ""}${copy.hollow ? " atlas-marker-hollow" : ""}`}
              style={{ left: position.left, top: position.top }}
              aria-label={`${marker.label}، ${mapStatusLabel(marker.status)}، ${formatMapCount(marker.count)} إشارة`}
              aria-pressed={selected}
              onClick={() => onSelectMarker(marker)}
            >
              <span className="atlas-marker-glyph" aria-hidden="true">{copy.glyph}</span>
              {marker.count > 1 ? <b className="atlas-marker-count" aria-hidden="true">{marker.count}</b> : null}
              <span className="atlas-marker-label" aria-hidden="true">{marker.label}</span>
            </button>
          );
        })}
        {visibleInferences.map((seal) => {
          const position = overlay.inferences[seal.key];
          const selected = selectedInferenceKey === seal.key;
          return (
            <button
              type="button"
              key={seal.key}
              className={`atlas-inference-seal-pin${selected ? " atlas-inference-seal-pin-selected" : ""}`}
              style={{ left: position.left, top: position.top }}
              aria-label={`${seal.kind === "cluster" ? "ختم تجمع استنتاجي" : "ختم ملاحظة استنتاجية"}: ${seal.label}`}
              aria-pressed={selected}
              onClick={() => onSelectInferenceRef.current?.(seal)}
            >
              <span className="atlas-inference-seal-glyph" aria-hidden="true">▣</span>
              {seal.count !== undefined && seal.count > 1 ? <b className="atlas-inference-seal-count" aria-hidden="true">{seal.count}</b> : null}
              <span className="atlas-inference-seal-label" aria-hidden="true">{seal.label}</span>
            </button>
          );
        })}
        {visibleRegionLabels.map((region) => {
          const position = overlay.regionLabels[region.id];
          return (
            <span className="atlas-region-label" style={{ left: position.left, top: position.top }} aria-hidden="true" key={region.id}>
              {region.placeName || region.entityName || ""}
            </span>
          );
        })}
        {mapReferenceCaptions.map((caption) => {
          const position = overlay.captions[caption.label];
          if (!position) return null;
          return (
            <span key={caption.label} className={`atlas-ref-caption atlas-ref-${caption.kind}`} style={{ left: position.left, top: position.top }} aria-hidden="true">
              {caption.label}
            </span>
          );
        })}
        {overlay.migration && selectedMigrationPeriod ? (
          <span className="atlas-migration-caption" style={{ left: overlay.migration.left, top: overlay.migration.top }} aria-hidden="true">
            هجرة · {selectedMigrationPeriod}
          </span>
        ) : null}
      </div>
      <div className="atlas-compass" aria-hidden="true"><span className="atlas-compass-arrow">▲</span><span className="atlas-compass-label">شمال</span></div>
      {failed ? <div className="atlas-map-fallback" role="status">تعذر رسم الخريطة في هذا المتصفح. القائمة والفاحصة ما زالتا تعملان.</div> : null}
    </div>
  );
}

function addAtlasImages(map: maplibregl.Map): void {
  if (!map.hasImage("atlas-arrow")) {
    const arrow = document.createElement("canvas");
    arrow.width = 24;
    arrow.height = 24;
    const context = arrow.getContext("2d");
    if (context) {
      context.fillStyle = "#2d2520";
      context.beginPath();
      context.moveTo(12, 2);
      context.lineTo(21, 20);
      context.lineTo(12, 15);
      context.lineTo(3, 20);
      context.closePath();
      context.fill();
      map.addImage("atlas-arrow", context.getImageData(0, 0, 24, 24));
    }
  }
  if (!map.hasImage("atlas-hatch")) {
    const hatch = document.createElement("canvas");
    hatch.width = 10;
    hatch.height = 10;
    const context = hatch.getContext("2d");
    if (context) {
      context.strokeStyle = "rgba(153, 68, 56, 0.55)";
      context.lineWidth = 1;
      context.beginPath();
      context.moveTo(-2, 10);
      context.lineTo(10, -2);
      context.stroke();
      map.addImage("atlas-hatch", context.getImageData(0, 0, 10, 10));
    }
  }
}

function graticuleCollection(): FeatureCollection<LineString, Record<string, never>> {
  const features: FeatureCollection<LineString, Record<string, never>>["features"] = [];
  for (let longitude = -180; longitude <= 180; longitude += 10) {
    features.push({ type: "Feature", geometry: { type: "LineString", coordinates: [[longitude, -80], [longitude, 80]] }, properties: {} });
  }
  for (let latitude = -80; latitude <= 80; latitude += 10) {
    features.push({ type: "Feature", geometry: { type: "LineString", coordinates: [[-180, latitude], [180, latitude]] }, properties: {} });
  }
  return { type: "FeatureCollection", features };
}

function setSourceData(map: maplibregl.Map, sourceID: string, data: AnyCollection): void {
  const source = map.getSource(sourceID);
  if (source && "setData" in source) (source as maplibregl.GeoJSONSource).setData(data);
}

function selectById(event: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }, features: MapFeature[], onSelect: (feature: MapFeature) => void): void {
  const id = event.features?.[0]?.properties?.id;
  const feature = features.find((item) => item.id === id);
  if (feature) onSelect(feature);
}
