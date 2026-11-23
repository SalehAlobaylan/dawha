import * as maplibregl from "maplibre-gl";
import maplibreWorkerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";
import type { FeatureCollection, LineString, Point, Polygon } from "geojson";
import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import type { GeoJSON } from "geojson";
import type { MapFeature } from "../types";
import { formatMapCount, mapKindLabel, mapStatusLabel, mapStatusVocabulary } from "../lib/map-vocabulary";
import {
  defaultAtlasLayerVisibility,
  featureLabel,
  migrationArrowCollection,
  migrationCollection,
  regionCollection,
  relationArcCollection,
  statusMatchExpression,
  viewportForCoordinates,
  type AtlasLayerVisibility,
  type AtlasMarker,
  type AtlasPlateLayer,
  type AtlasPlateTribeLabel,
  type AtlasRelationArcFeature,
  type AtlasViewport,
} from "../lib/atlas";
import { coastlineLandCollection, coastlineLineCollection, COASTLINE_BOUNDS } from "../lib/coastline";
import { mapReferenceCaptions } from "../lib/map-reference";
import { plateNeatlineRing, plateSourceBounds } from "../lib/atlas";
import {
  mapLabelBox,
  mapLabelsOverlap,
  ottomanHarita1910Territories,
  ottomanHaritaCollection,
  placePlateTribeLabels,
  plateTerritoryCollection,
  type OttomanHaritaTerritory,
} from "../lib/plate-atlas";
import { atlasTileUrlTemplate } from "../lib/api";
import type { AtlasInferenceSeal } from "../lib/atlas-inference";
import "maplibre-gl/dist/maplibre-gl.css";

// maplibre derives its GeoJSON tile worker URL from `import.meta.url`. Vite's
// pre-bundle copies the entry but not the sibling worker file, so the worker
// 404s and every GeoJSON layer silently never paints (the DOM markers still
// float on top, and nothing in the console connects the two). Bundling the
// worker as an asset and pointing maplibre at it directly paints the layers in
// dev and production alike, before any hand-holding in vite config.
maplibregl.setWorkerUrl(maplibreWorkerUrl);

type HoverPreview = {
  id: string;
  kind: "region" | "migration" | "relation" | "corridor" | "marker" | "plateTerritory" | "ottomanTerritory";
  title: string;
  subtitle: string;
  statusLabel?: string;
  x: number;
  y: number;
};

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
  /** Fit the canvas to explicit geographic bounds (e.g. zoom-to-fit a plate). */
  fitBoundsRequest?: { bounds: [number, number, number, number]; key: string } | null;
  /** The URL names a feature or place; the arrival flight belongs to it, not to the peninsula fit. */
  skipInitialFit?: boolean;
  resetSignal?: number;
  inferences?: AtlasInferenceSeal[];
  selectedInferenceKey?: string;
  onSelectInference?: (seal: AtlasInferenceSeal) => void;
  /** The warped-plate overlays (plans 032-A/B): renderable plates only — a
   * plate whose transform failed validation stays in the register panel with
   * its own caption and never reaches this prop. */
  plateLayers?: AtlasPlateLayer[];
  selectedCaptionId?: string;
  selectedCaptionNameAr?: string;
  onSelectCaption?: (label: AtlasPlateTribeLabel) => void;
  selectedOttomanId?: string;
  onSelectOttomanTerritory?: (territory: OttomanHaritaTerritory) => void;
  /** The recorded relation arcs (plan 032-E): both endpoints resolved
   * server-side; an unanchored end draws nothing and the count says so. */
  relations?: AtlasRelationArcFeature[];
  /** The compare panel's point (plan 032-C): armed flag + the callback the
   * map's next background click feeds. The map reports a longitude/latitude;
   * the URL carries the chosen point. */
  compareArmed?: boolean;
  onComparePick?: (longitude: number, latitude: number) => void;
  comparePoint?: { longitude: number; latitude: number } | null;
  /** The movement corridors (plan 033-B): the recorded routes' own
   * geometry, drawn dashed in the corridor's own season's words. */
  corridors?: GeoJSON.FeatureCollection<GeoJSON.LineString>;
  /** Quick layer visibility toggles for visual decluttering. */
  layerVisibility?: AtlasLayerVisibility;
  /** Curtain swipe comparison: when non-null (0..100) and a plate is ON, peels
   * the active plate raster(s) horizontally over the documented atlas. */
  swipePercent?: number | null;
  onSwipeChange?: (percent: number) => void;
}

type OverlayPosition = { left: number; top: number; visible: boolean };
type OverlayPositions = {
  markers: Record<string, OverlayPosition>;
  inferences: Record<string, OverlayPosition>;
  regionLabels: Record<string, OverlayPosition>;
  captions: Record<string, OverlayPosition>;
  plateTribes: ReturnType<typeof placePlateTribeLabels>;
  ottomanLabels: Array<{ territory: OttomanHaritaTerritory; x: number; y: number }>;
  migration: OverlayPosition | null;
  compare: OverlayPosition | null;
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
export function AtlasMap({ markers, migrations, regions, selectedKey, onSelectMarker, onSelectFeature, onClearSelection, onViewportChange, focus, fitBoundsRequest = null, skipInitialFit = false, resetSignal = 0, inferences = [], selectedInferenceKey = "", onSelectInference, plateLayers = [], selectedCaptionId = "", selectedCaptionNameAr = "", onSelectCaption, selectedOttomanId = "", onSelectOttomanTerritory, relations = [], compareArmed = false, onComparePick, comparePoint, corridors, layerVisibility = defaultAtlasLayerVisibility, swipePercent = null, onSwipeChange }: AtlasMapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const swipeContainerRef = useRef<HTMLDivElement>(null);
  const swipeMapRef = useRef<maplibregl.Map | null>(null);
  const repositionRef = useRef<() => void>(() => undefined);
  const syncPlatesRef = useRef<() => void>(() => undefined);
  const syncVisibilityRef = useRef<() => void>(() => undefined);
  const fitRef = useRef<() => void>(() => undefined);
  const fitAllRef = useRef<() => void>(() => undefined);
  const resetPendingRef = useRef(false);
  const skipInitialFitRef = useRef(skipInitialFit);
  // One overlay state for everything the canvas projects: markers, inference
  // seals, region labels, reference captions and the selected migration's
  // caption all move on the same frame, so they reconcile in one commit
  // instead of four setStates per animation frame underneath the reader's
  // drag.
  const [overlay, setOverlay] = useState<OverlayPositions>({ markers: {}, inferences: {}, regionLabels: {}, captions: {}, plateTribes: [], ottomanLabels: [], migration: null, compare: null });
  const [hovered, setHovered] = useState<HoverPreview | null>(null);
  const [failed, setFailed] = useState(false);
  const [failedPlates, setFailedPlates] = useState<string[]>([]);
  const swipeActive = swipePercent !== null && plateLayers.length > 0;

  const markersRef = useRef(markers);
  const migrationsRef = useRef(migrations);
  const relationsRef = useRef(relations);
  const regionsRef = useRef(regions);
  const selectedKeyRef = useRef(selectedKey);
  const inferencesRef = useRef(inferences);
  const selectedInferenceKeyRef = useRef(selectedInferenceKey);
  const onSelectMarkerRef = useRef(onSelectMarker);
  const onSelectFeatureRef = useRef(onSelectFeature);
  const onClearSelectionRef = useRef(onClearSelection);
  const onViewportChangeRef = useRef(onViewportChange);
  const onSelectInferenceRef = useRef(onSelectInference);
  const onSelectCaptionRef = useRef(onSelectCaption);
  onSelectCaptionRef.current = onSelectCaption;
  const onSelectOttomanRef = useRef(onSelectOttomanTerritory);
  onSelectOttomanRef.current = onSelectOttomanTerritory;
  const ottomanData = useMemo(() => ottomanHaritaCollection(), []);
  const ottomanDataRef = useRef(ottomanData);
  ottomanDataRef.current = ottomanData;
  const onComparePickRef = useRef(onComparePick);
  onComparePickRef.current = onComparePick;
  const compareArmedRef = useRef(compareArmed);
  compareArmedRef.current = compareArmed;
  const comparePointRef = useRef(comparePoint);
  comparePointRef.current = comparePoint;
  const corridorsRef = useRef(corridors);
  corridorsRef.current = corridors;
  const layerVisibilityRef = useRef(layerVisibility);
  layerVisibilityRef.current = layerVisibility;
  const swipeActiveRef = useRef(swipeActive);
  swipeActiveRef.current = swipeActive;
  markersRef.current = markers;
  migrationsRef.current = migrations;
  relationsRef.current = relations;
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
  const plateLayersRef = useRef(plateLayers);
  plateLayersRef.current = plateLayers;
  const plateTribes = useMemo(() => plateLayers.flatMap((plate) => plate.tribeLabels ?? []), [plateLayers]);
  const plateTribesRef = useRef(plateTribes);
  plateTribesRef.current = plateTribes;
  const plateTerritoriesData = useMemo(() => plateTerritoryCollection(plateLayers), [plateLayers]);
  const plateTerritoriesDataRef = useRef(plateTerritoriesData);
  plateTerritoriesDataRef.current = plateTerritoriesData;

  useEffect(() => {
    repositionRef.current();
  }, [plateTribes]);

  // The overlay's own lifecycle (plan 032-B): each renderable plate gets ONE
  // raster source (XYZ URLs of the server's per-pixel warp) + one dashed
  // neatline outline in the plate register. Toggles add and remove; the
  // opacity slider repaints in place. Runs after style load and on every
  // spec change — the map never draws a plate the register did not validate.
  useEffect(() => {
    const sync = () => {
      const map = mapRef.current;
      if (!map?.getLayer("atlas-graticule-lines")) return;
      const specs = plateLayersRef.current;
      const wanted = new Set(specs.map((spec) => spec.key));
      setFailedPlates((current) => current.every((key) => wanted.has(key)) ? current : current.filter((key) => wanted.has(key)));
      // Remove the sources and layers of plates that are off or unlisted.
      const layerIds = map.getStyle().layers.map((layer) => layer.id);
      for (const layerId of layerIds) {
        const match = /^atlas-plate-(?:fill|frame)-(.+)$/.exec(layerId);
        if (match && !wanted.has(match[1])) {
          map.removeLayer(layerId);
        }
      }
      for (const sourceId of Object.keys(map.getStyle().sources)) {
        const match = /^atlas-plate-(?:frame-source-)?(.+)$/.exec(sourceId);
        if (match && match[1] && !wanted.has(match[1])) {
          map.removeSource(sourceId);
        }
      }
      for (const spec of specs) {
        const sourceId = `atlas-plate-${spec.key}`;
        try {
          if (!map.getSource(sourceId)) {
            map.addSource(sourceId, {
              type: "raster",
              tiles: [atlasTileUrlTemplate(spec.key)],
              tileSize: 256,
              minzoom: 2,
              maxzoom: 7,
              bounds: plateSourceBounds(spec.transform, spec.frame),
            });
          }
          const effectiveOpacity = swipeActiveRef.current ? 0 : spec.opacity;
          if (!map.getLayer(`atlas-plate-fill-${spec.key}`)) {
            map.addLayer(
              { id: `atlas-plate-fill-${spec.key}`, type: "raster", source: sourceId, paint: { "raster-opacity": effectiveOpacity, "raster-resampling": "linear" } },
              "atlas-graticule-lines",
            );
          } else {
            map.setPaintProperty(`atlas-plate-fill-${spec.key}`, "raster-opacity", effectiveOpacity);
          }
          const frameSourceId = `atlas-plate-frame-source-${spec.key}`;
          if (!map.getSource(frameSourceId)) {
            map.addSource(frameSourceId, { type: "geojson", data: plateNeatlineRing(spec) });
          }
          if (!map.getLayer(`atlas-plate-frame-${spec.key}`)) {
            // Dashed warm outline: the plate register's own shape — a plate's
            // footprint never wears a region's ring (stipulation 1), and it
            // sits directly above its own ink so the two move together.
            map.addLayer(
              { id: `atlas-plate-frame-${spec.key}`, type: "line", source: frameSourceId, paint: { "line-color": "rgba(122, 94, 52, 0.55)", "line-width": 1.2, "line-dasharray": [2, 2] } },
              `atlas-plate-fill-${spec.key}`,
            );
          }
        } catch {
          setFailedPlates((current) => current.includes(spec.key) ? current : [...current, spec.key]);
        }
      }
    };
    syncPlatesRef.current = sync;
    sync();
  }, [plateLayers, swipeActive]);

  useEffect(() => {
    const syncVisibility = () => {
      const map = mapRef.current;
      if (!map?.getLayer("atlas-graticule-lines")) return;
      const vis = layerVisibilityRef.current;
      const setLayerVis = (layerId: string, show: boolean) => {
        if (map.getLayer(layerId)) {
          map.setLayoutProperty(layerId, "visibility", show ? "visible" : "none");
        }
      };
      setLayerVis("atlas-graticule-lines", vis.graticule);
      setLayerVis("atlas-region-fill", vis.regions);
      setLayerVis("atlas-region-hatch", vis.regions);
      setLayerVis("atlas-region-outline", vis.regions);
      setLayerVis("atlas-region-outline-undated", vis.regions);
      setLayerVis("atlas-region-hover", vis.regions);
      setLayerVis("atlas-relations-arcs", vis.relations);
      setLayerVis("atlas-relations-arrowheads", vis.relations);
      setLayerVis("atlas-corridors-line", vis.corridors);
      setLayerVis("atlas-plate-territories-fill", vis.plateTerritories);
      setLayerVis("atlas-plate-territories-line", vis.plateTerritories);
      setLayerVis("atlas-plate-territories-hover-fill", vis.plateTerritories);
      setLayerVis("atlas-plate-territories-hover-line", vis.plateTerritories);
      setLayerVis("atlas-plate-territories-selected-fill", vis.plateTerritories);
      setLayerVis("atlas-plate-territories-selected-line", vis.plateTerritories);
      setLayerVis("atlas-ottoman-1910-fill", vis.ottomanHarita1910);
      setLayerVis("atlas-ottoman-1910-line", vis.ottomanHarita1910);
      setLayerVis("atlas-ottoman-1910-hover-fill", vis.ottomanHarita1910);
      setLayerVis("atlas-ottoman-1910-hover-line", vis.ottomanHarita1910);
      setLayerVis("atlas-ottoman-1910-selected-fill", vis.ottomanHarita1910);
      setLayerVis("atlas-ottoman-1910-selected-line", vis.ottomanHarita1910);
      repositionRef.current();
    };
    syncVisibilityRef.current = syncVisibility;
    syncVisibility();
  }, [layerVisibility]);

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
        transformRequest: (url) => ({
          url, credentials: url.includes("/api/v1/plates/") ? "include" : undefined,
        }),
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
      const sourceId = (event as typeof event & { sourceId?: string }).sourceId;
      const plate = plateLayersRef.current.find(({ key }) => sourceId === `atlas-plate-${key}`);
      if (plate) setFailedPlates((current) => current.includes(plate.key) ? current : [...current, plate.key]);
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
      map.addLayer({ id: "atlas-region-hover", type: "line", source: "atlas-regions", filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "line-color": "#2d2520", "line-width": 2.6, "line-opacity": 0.9 } });

      map.addLayer({ id: "atlas-migration-certain", type: "line", source: "atlas-migrations", filter: ["==", ["get", "certainty"], "precise"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 2, "line-opacity": 0.85 } });
      map.addLayer({ id: "atlas-migration-approximate", type: "line", source: "atlas-migrations", filter: ["==", ["get", "certainty"], "approximate"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 2, "line-opacity": 0.8, "line-dasharray": [3, 2] } });
      map.addLayer({ id: "atlas-migration-uncertain", type: "line", source: "atlas-migrations", filter: ["==", ["get", "certainty"], "uncertain"] as maplibregl.FilterSpecification, paint: { "line-color": statusStrokes, "line-width": 1.8, "line-opacity": 0.72, "line-dasharray": [1, 2] } });
      map.addLayer({ id: "atlas-migration-hover", type: "line", source: "atlas-migrations", filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "line-color": "#2d2520", "line-width": 3.4, "line-opacity": 0.9 } });
      map.addLayer({ id: "atlas-migration-arrows", type: "symbol", source: "atlas-arrows", layout: { "icon-image": "atlas-arrow", "icon-size": 0.72, "icon-rotate": ["get", "bearing"] as unknown as maplibregl.ExpressionSpecification, "icon-rotation-alignment": "map", "icon-allow-overlap": true, "icon-ignore-placement": true } });

      // الأروابط (032-E): the relations' RECORD as an arc — a dashed line + the
      // arrow sprite at its end bearing. Data-driven from the atlas read; the
      // layer states presence, never an inference about the relationship.
      // The layers are created HERE, inside the load handler — a loaded style
      // is the only place a layer's paint can be asked for.
      map.addSource("atlas-relations-arcs", { type: "geojson", data: relationArcCollection(relationsRef.current) as AnyCollection });
      map.addLayer({ id: "atlas-relations-arcs", type: "line", source: "atlas-relations-arcs", paint: { "line-color": "rgba(122, 94, 52, 0.5)", "line-width": 1.6, "line-dasharray": [4, 3] } });
      map.addLayer({ id: "atlas-relations-arrowheads", type: "symbol", source: "atlas-relations-arcs", layout: { "symbol-placement": "point", "icon-image": "atlas-arrow", "icon-size": 0.58, "icon-rotate": ["get", "bearing"] as unknown as maplibregl.ExpressionSpecification, "icon-rotation-alignment": "map", "icon-allow-overlap": true, "icon-ignore-placement": true } });

      // الممرات (033-B): the recorded routes' own line — dashed (the corridor
      // is a reading, never a constancy), its stops are the read's own places,
      // and the season rides the feature's own property for the filter chips.
      map.addSource("atlas-corridors", { type: "geojson", data: (corridorsRef.current ?? { type: "FeatureCollection", features: [] }) as AnyCollection });
      map.addLayer({ id: "atlas-corridors-line", type: "line", source: "atlas-corridors", paint: { "line-color": "#7a5e34", "line-width": 2.2, "line-dasharray": [6, 3] } });

      // حدود قبائل اللوحة المرقَّمة (plate register): fine dotted archival
      // terracotta lines mirroring Dickson's own printed dotted boundaries,
      // visually distinct from documented historical_regions.
      map.addSource("atlas-plate-territories", { type: "geojson", data: plateTerritoriesDataRef.current as AnyCollection });
      map.addLayer({ id: "atlas-plate-territories-fill", type: "fill", source: "atlas-plate-territories", paint: { "fill-color": "#9b5948", "fill-opacity": 0.08 } });
      map.addLayer({ id: "atlas-plate-territories-line", type: "line", source: "atlas-plate-territories", paint: { "line-color": "#9b5948", "line-width": 1.5, "line-opacity": 0.72, "line-dasharray": [2, 2] } });
      map.addLayer({ id: "atlas-plate-territories-hover-fill", type: "fill", source: "atlas-plate-territories", filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "fill-color": "#9b5948", "fill-opacity": 0.24 } });
      map.addLayer({ id: "atlas-plate-territories-hover-line", type: "line", source: "atlas-plate-territories", filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "line-color": "#833627", "line-width": 2.5, "line-opacity": 0.95 } });
      map.addLayer({ id: "atlas-plate-territories-selected-fill", type: "fill", source: "atlas-plate-territories", filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "fill-color": "#9b5948", "fill-opacity": 0.3 } });
      map.addLayer({ id: "atlas-plate-territories-selected-line", type: "line", source: "atlas-plate-territories", filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "line-color": "#6b2416", "line-width": 2.8, "line-opacity": 0.95 } });

      // حدود خريطة ٩٣٦٧٧ العثمانية (حوالي ١٩١٠م · OpenGulf / جامعة إسطنبول):
      // archival olive-emerald dashed register distinct from Dickson & historical_regions.
      map.addSource("atlas-ottoman-1910", { type: "geojson", data: ottomanDataRef.current as AnyCollection });
      map.addLayer({ id: "atlas-ottoman-1910-fill", type: "fill", source: "atlas-ottoman-1910", layout: { visibility: "none" }, paint: { "fill-color": "#3d6b52", "fill-opacity": 0.14 } });
      map.addLayer({ id: "atlas-ottoman-1910-line", type: "line", source: "atlas-ottoman-1910", layout: { visibility: "none" }, paint: { "line-color": "#2f5742", "line-width": 1.6, "line-opacity": 0.8, "line-dasharray": [4, 2] } });
      map.addLayer({ id: "atlas-ottoman-1910-hover-fill", type: "fill", source: "atlas-ottoman-1910", layout: { visibility: "none" }, filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "fill-color": "#3d6b52", "fill-opacity": 0.28 } });
      map.addLayer({ id: "atlas-ottoman-1910-hover-line", type: "line", source: "atlas-ottoman-1910", layout: { visibility: "none" }, filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "line-color": "#1e422f", "line-width": 2.6, "line-opacity": 0.95 } });
      map.addLayer({ id: "atlas-ottoman-1910-selected-fill", type: "fill", source: "atlas-ottoman-1910", layout: { visibility: "none" }, filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "fill-color": "#3d6b52", "fill-opacity": 0.34 } });
      map.addLayer({ id: "atlas-ottoman-1910-selected-line", type: "line", source: "atlas-ottoman-1910", layout: { visibility: "none" }, filter: ["==", ["get", "id"], ""] as maplibregl.FilterSpecification, paint: { "line-color": "#153322", "line-width": 2.9, "line-opacity": 0.98 } });

      const selectRegion = (event: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
        if (map.queryRenderedFeatures(event.point, { layers: ["atlas-ottoman-1910-fill", "atlas-plate-territories-fill"] }).length > 0) return;
        selectById(event, regionsRef.current, onSelectFeatureRef.current);
      };
      const selectMigration = (event: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
        selectById(event, migrationsRef.current, onSelectFeatureRef.current);
      };
      map.on("click", "atlas-ottoman-1910-fill", (event) => {
        if (compareArmedRef.current || !onSelectOttomanRef.current) return;
        const id = String(event.features?.[0]?.properties?.id ?? "");
        const match = ottomanHarita1910Territories.find((item) => item.id === id);
        if (match) onSelectOttomanRef.current(match);
      });
      map.on("click", "atlas-plate-territories-fill", (event) => {
        if (compareArmedRef.current || !onSelectCaptionRef.current) return;
        if (map.queryRenderedFeatures(event.point, { layers: ["atlas-ottoman-1910-fill"] }).length > 0) return;
        const props = event.features?.[0]?.properties;
        const captionId = String(props?.captionId ?? "");
        const nameAr = String(props?.nameAr ?? "");
        const plateKey = String(props?.plateKey ?? "");
        const match = plateTribesRef.current.find((item) => item.id === captionId)
          ?? plateTribesRef.current.find((item) => item.plateKey === plateKey && item.nameAr === nameAr);
        if (match) onSelectCaptionRef.current(match);
      });
      map.on("click", "atlas-region-fill", selectRegion);
      map.on("click", "atlas-migration-certain", selectMigration);
      map.on("click", "atlas-migration-approximate", selectMigration);
      map.on("click", "atlas-migration-uncertain", selectMigration);
      const clearHover = () => {
        map.getCanvas().style.cursor = "";
        if (map.getLayer("atlas-region-hover")) map.setFilter("atlas-region-hover", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-migration-hover")) map.setFilter("atlas-migration-hover", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-plate-territories-hover-fill")) map.setFilter("atlas-plate-territories-hover-fill", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-plate-territories-hover-line")) map.setFilter("atlas-plate-territories-hover-line", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-ottoman-1910-hover-fill")) map.setFilter("atlas-ottoman-1910-hover-fill", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-ottoman-1910-hover-line")) map.setFilter("atlas-ottoman-1910-hover-line", ["==", ["get", "id"], ""]);
        setHovered(null);
      };
      map.on("mousemove", "atlas-ottoman-1910-fill", (event) => {
        const props = event.features?.[0]?.properties;
        const id = String(props?.id ?? "");
        const nameAr = String(props?.nameAr ?? "");
        const ottomanName = String(props?.ottomanName ?? "");
        const summaryAr = String(props?.summaryAr ?? "");
        if (!id) return;
        map.getCanvas().style.cursor = "pointer";
        if (map.getLayer("atlas-ottoman-1910-hover-fill")) map.setFilter("atlas-ottoman-1910-hover-fill", ["==", ["get", "id"], id]);
        if (map.getLayer("atlas-ottoman-1910-hover-line")) map.setFilter("atlas-ottoman-1910-hover-line", ["==", ["get", "id"], id]);
        if (map.getLayer("atlas-plate-territories-hover-fill")) map.setFilter("atlas-plate-territories-hover-fill", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-plate-territories-hover-line")) map.setFilter("atlas-plate-territories-hover-line", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-region-hover")) map.setFilter("atlas-region-hover", ["==", ["get", "id"], ""]);
        setHovered({
          id,
          kind: "ottomanTerritory",
          title: ottomanName && ottomanName !== nameAr ? `${nameAr} (${ottomanName})` : nameAr,
          subtitle: summaryAr || "نطاق من خريطة ٩٣٦٧٧ العثمانية (حوالي ١٩١٠م)",
          statusLabel: "عثماني ١٩١٠",
          x: event.point.x,
          y: event.point.y,
        });
      });
      map.on("mouseleave", "atlas-ottoman-1910-fill", clearHover);
      map.on("mousemove", "atlas-plate-territories-fill", (event) => {
        const props = event.features?.[0]?.properties;
        const id = String(props?.id ?? "");
        const nameAr = String(props?.nameAr ?? "");
        const printedName = String(props?.printedName ?? "");
        const spanAr = String(props?.spanAr ?? "");
        if (!id) return;
        map.getCanvas().style.cursor = "pointer";
        if (map.getLayer("atlas-plate-territories-hover-fill")) map.setFilter("atlas-plate-territories-hover-fill", ["==", ["get", "id"], id]);
        if (map.getLayer("atlas-plate-territories-hover-line")) map.setFilter("atlas-plate-territories-hover-line", ["==", ["get", "id"], id]);
        if (map.getLayer("atlas-region-hover")) map.setFilter("atlas-region-hover", ["==", ["get", "id"], ""]);
        if (map.getLayer("atlas-migration-hover")) map.setFilter("atlas-migration-hover", ["==", ["get", "id"], ""]);
        setHovered({
          id,
          kind: "plateTerritory",
          title: printedName ? `${nameAr} (${printedName})` : nameAr,
          subtitle: spanAr || "نطاق تقريبي مرقَّم من لوحة ديكسون ١٩١٤",
          statusLabel: "حدود اللوحة",
          x: event.point.x,
          y: event.point.y,
        });
      });
      map.on("mouseleave", "atlas-plate-territories-fill", clearHover);
      map.on("mousemove", "atlas-region-fill", (event) => {
        const id = String(event.features?.[0]?.properties?.id ?? "");
        const feature = regionsRef.current.find((item) => item.id === id);
        if (!feature) return;
        map.getCanvas().style.cursor = "pointer";
        if (map.getLayer("atlas-region-hover")) map.setFilter("atlas-region-hover", ["==", ["get", "id"], id]);
        if (map.getLayer("atlas-migration-hover")) map.setFilter("atlas-migration-hover", ["==", ["get", "id"], ""]);
        setHovered({
          id,
          kind: "region",
          title: featureLabel(feature),
          subtitle: feature.period ? `${mapKindLabel(feature.kind)} · ${feature.period}` : mapKindLabel(feature.kind),
          statusLabel: mapStatusLabel(feature.status),
          x: event.point.x,
          y: event.point.y,
        });
      });
      map.on("mouseleave", "atlas-region-fill", clearHover);
      for (const layer of ["atlas-migration-certain", "atlas-migration-approximate", "atlas-migration-uncertain"]) {
        map.on("mousemove", layer, (event) => {
          const id = String(event.features?.[0]?.properties?.id ?? "");
          const feature = migrationsRef.current.find((item) => item.id === id);
          if (!feature) return;
          map.getCanvas().style.cursor = "pointer";
          if (map.getLayer("atlas-migration-hover")) map.setFilter("atlas-migration-hover", ["==", ["get", "id"], id]);
          if (map.getLayer("atlas-region-hover")) map.setFilter("atlas-region-hover", ["==", ["get", "id"], ""]);
          setHovered({
            id,
            kind: "migration",
            title: featureLabel(feature),
            subtitle: feature.period ? `${mapKindLabel(feature.kind)} · ${feature.period}` : mapKindLabel(feature.kind),
            statusLabel: mapStatusLabel(feature.status),
            x: event.point.x,
            y: event.point.y,
          });
        });
        map.on("mouseleave", layer, clearHover);
      }
      map.on("mousemove", "atlas-relations-arcs", (event) => {
        const props = event.features?.[0]?.properties;
        const fromName = String(props?.fromName ?? "");
        const toName = String(props?.toName ?? "");
        const relationType = String(props?.relationType ?? "علاقة مسجّلة");
        map.getCanvas().style.cursor = "pointer";
        setHovered({
          id: String(props?.id ?? `${fromName}-${toName}`),
          kind: "relation",
          title: fromName && toName ? `${fromName} ← ${toName}` : "علاقة مسجّلة",
          subtitle: relationType,
          x: event.point.x,
          y: event.point.y,
        });
      });
      map.on("mouseleave", "atlas-relations-arcs", clearHover);
      map.on("mousemove", "atlas-corridors-line", (event) => {
        const props = event.features?.[0]?.properties;
        const title = String(props?.title ?? "ممر حركة");
        const season = String(props?.season ?? "");
        map.getCanvas().style.cursor = "pointer";
        setHovered({
          id: String(props?.id ?? title),
          kind: "corridor",
          title,
          subtitle: season ? `ممر حركة · ${season}` : "ممر حركة موثّق",
          x: event.point.x,
          y: event.point.y,
        });
      });
      map.on("mouseleave", "atlas-corridors-line", clearHover);
      map.on("click", (event) => {
        const target = event.originalEvent.target as HTMLElement | null;
        if (target?.closest(".atlas-marker") || target?.closest(".atlas-inference-seal-pin")) return;
        // The compare mode owns the next click (plan 032-C), FEATURE OR NOT:
        // arming is the reader's explicit intent, and the pick needs the
        // point the reader asked for — not the selection a polygon below it
        // would answer. The URL holds the pick; the panel reads the plates.
        if (compareArmedRef.current && onComparePickRef.current) {
          onComparePickRef.current(event.lngLat.lng, event.lngLat.lat);
          return;
        }
        const hits = map.queryRenderedFeatures(event.point, {
          layers: ["atlas-ottoman-1910-fill", "atlas-plate-territories-fill", "atlas-region-fill", "atlas-migration-certain", "atlas-migration-approximate", "atlas-migration-uncertain"],
        });
        if (!hits.length) onClearSelectionRef.current();
      });

      syncPlatesRef.current();
      syncVisibilityRef.current();

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
        const vis = layerVisibilityRef.current;
        const next: OverlayPositions = {
          markers: {}, inferences: {}, regionLabels: {}, captions: {}, migration: null, compare: null,
          plateTribes: [], ottomanLabels: [],
        };
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
        for (const seal of inferencesRef.current) {
          next.inferences[seal.key] = project(seal.longitude, seal.latitude, 60, 60);
        }
        // Unified collision pass: markers + inference seals reserve space first,
        // then plate tribe labels, then region labels, then reference captions.
        const occupied = [...Object.values(next.markers), ...Object.values(next.inferences)]
          .filter((position) => position.visible)
          .map(({ left, top }) => ({ left: left - 24, right: left + 24, top: top - 18, bottom: top + 24 }));
        if (vis.plateLabels && plateTribesRef.current.length > 0) {
          next.plateTribes = placePlateTribeLabels(plateTribesRef.current, (coordinates) => map.project(coordinates), width, height, occupied);
          for (const placed of next.plateTribes) {
            occupied.push({ left: placed.left, right: placed.right, top: placed.top, bottom: placed.bottom });
          }
        }
        if (vis.ottomanHarita1910) {
          for (const territory of ottomanHarita1910Territories) {
            const pt = map.project([territory.centroid[0], territory.centroid[1]]);
            if (pt.x < 0 || pt.y < 0 || pt.x > width || pt.y > height) continue;
            const box = mapLabelBox(territory.nameAr, pt.x, pt.y);
            if (occupied.some((other) => mapLabelsOverlap(other, box))) continue;
            occupied.push(box);
            next.ottomanLabels.push({ territory, x: pt.x, y: pt.y });
          }
        }
        const placeSecondaryLabel = (name: string, position: OverlayPosition): OverlayPosition => {
          if (!position.visible || !name) return { ...position, visible: false };
          const box = mapLabelBox(name, position.left, position.top);
          if (occupied.some((other) => mapLabelsOverlap(other, box))) {
            return { ...position, visible: false };
          }
          occupied.push(box);
          return position;
        };
        if (vis.regions) {
          for (const region of regionsRef.current) {
            if (region.longitude === undefined || region.latitude === undefined) continue;
            next.regionLabels[region.id] = placeSecondaryLabel(region.placeName || region.entityName || "", project(region.longitude, region.latitude, 80, 40));
          }
        }
        if (vis.reference) {
          for (const caption of mapReferenceCaptions) {
            next.captions[caption.label] = placeSecondaryLabel(caption.label, project(caption.longitude, caption.latitude, 60, 30));
          }
        }
        const selectedMigration = migrationsRef.current.find((migration) => migration.id === selectedKeyRef.current);
        if (selectedMigration?.fromLongitude !== undefined && selectedMigration.fromLatitude !== undefined && selectedMigration.toLongitude !== undefined && selectedMigration.toLatitude !== undefined) {
          next.migration = project(
            (selectedMigration.fromLongitude + selectedMigration.toLongitude) / 2,
            (selectedMigration.fromLatitude + selectedMigration.toLatitude) / 2,
            60, 30,
          );
        }
        if (comparePointRef.current) next.compare = project(comparePointRef.current.longitude, comparePointRef.current.latitude, 40, 40);
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
    setSourceData(map, "atlas-relations-arcs", relationArcCollection(relationsRef.current) as AnyCollection);
    setSourceData(map, "atlas-corridors", corridors ?? { type: "FeatureCollection", features: [] });
    setSourceData(map, "atlas-plate-territories", plateTerritoriesData as AnyCollection);
    repositionRef.current();
    if (resetPendingRef.current) {
      resetPendingRef.current = false;
      fitAllRef.current();
    } else {
      fitRef.current();
    }
  }, [arrowsData, migrationsData, regionsData, selectedKey, inferences, corridors, relations, plateTerritoriesData]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const filter: maplibregl.FilterSpecification = selectedCaptionId || selectedCaptionNameAr
      ? [
          "any",
          ["==", ["get", "captionId"], selectedCaptionId || "__none__"],
          ...(selectedCaptionNameAr ? [["==", ["get", "nameAr"], selectedCaptionNameAr]] : []),
        ] as unknown as maplibregl.FilterSpecification
      : ["==", ["get", "id"], ""];
    if (map.getLayer("atlas-plate-territories-selected-fill")) {
      map.setFilter("atlas-plate-territories-selected-fill", filter);
    }
    if (map.getLayer("atlas-plate-territories-selected-line")) {
      map.setFilter("atlas-plate-territories-selected-line", filter);
    }
  }, [selectedCaptionId, selectedCaptionNameAr, plateTerritoriesData]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const filter: maplibregl.FilterSpecification = selectedOttomanId
      ? ["==", ["get", "id"], selectedOttomanId]
      : ["==", ["get", "id"], ""];
    if (map.getLayer("atlas-ottoman-1910-selected-fill")) {
      map.setFilter("atlas-ottoman-1910-selected-fill", filter);
    }
    if (map.getLayer("atlas-ottoman-1910-selected-line")) {
      map.setFilter("atlas-ottoman-1910-selected-line", filter);
    }
  }, [selectedOttomanId]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !focus) return;
    const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
    map.easeTo({ center: [focus.longitude, focus.latitude], zoom: Math.max(map.getZoom(), 6.2), duration: reduced ? 0 : 600 });
  }, [focus]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !fitBoundsRequest) return;
    const [west, south, east, north] = fitBoundsRequest.bounds;
    const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
    map.fitBounds([[west, south], [east, north]], { padding: 36, maxZoom: 7, duration: reduced ? 0 : 550 });
  }, [fitBoundsRequest]);

  // Curtain/swipe plate overlay: only mounts a minimal raster-only MapLibre
  // instance while swipeActive is true, synced 1:1 with the main map camera.
  useEffect(() => {
    const mainMap = mapRef.current;
    const container = swipeContainerRef.current;
    if (!swipeActive || !mainMap || !container) return undefined;
    let overlayMap: maplibregl.Map;
    try {
      overlayMap = new maplibregl.Map({
        container,
        interactive: false,
        attributionControl: false,
        style: {
          version: 8,
          sources: {},
          layers: [{ id: "swipe-bg", type: "background", paint: { "background-color": "#ded1b9" } }],
        },
        center: mainMap.getCenter(),
        zoom: mainMap.getZoom(),
        bearing: mainMap.getBearing(),
        pitch: mainMap.getPitch(),
        transformRequest: (url) => ({
          url, credentials: url.includes("/api/v1/plates/") ? "include" : undefined,
        }),
      });
    } catch {
      return undefined;
    }
    swipeMapRef.current = overlayMap;

    const addSwipePlates = () => {
      for (const spec of plateLayersRef.current) {
        const sourceId = `swipe-plate-${spec.key}`;
        if (!overlayMap.getSource(sourceId)) {
          overlayMap.addSource(sourceId, {
            type: "raster",
            tiles: [atlasTileUrlTemplate(spec.key)],
            tileSize: 256,
            minzoom: 2,
            maxzoom: 7,
            bounds: plateSourceBounds(spec.transform, spec.frame),
          });
        }
        if (!overlayMap.getLayer(`swipe-plate-fill-${spec.key}`)) {
          overlayMap.addLayer({
            id: `swipe-plate-fill-${spec.key}`,
            type: "raster",
            source: sourceId,
            paint: { "raster-opacity": 1, "raster-resampling": "linear" },
          });
        }
      }
    };
    overlayMap.on("load", addSwipePlates);

    const syncCamera = () => {
      overlayMap.jumpTo({
        center: mainMap.getCenter(),
        zoom: mainMap.getZoom(),
        bearing: mainMap.getBearing(),
        pitch: mainMap.getPitch(),
      });
    };
    mainMap.on("move", syncCamera);
    mainMap.on("zoom", syncCamera);
    const ro = new ResizeObserver(() => overlayMap.resize());
    ro.observe(container);

    return () => {
      mainMap.off("move", syncCamera);
      mainMap.off("zoom", syncCamera);
      ro.disconnect();
      overlayMap.remove();
      swipeMapRef.current = null;
    };
  }, [swipeActive, plateLayers]);

  const handleSwipePointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (!onSwipeChange || !containerRef.current) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget;
    target.setPointerCapture(event.pointerId);
    const updateFromClientX = (clientX: number) => {
      const rect = containerRef.current?.getBoundingClientRect();
      if (!rect || rect.width <= 0) return;
      const pct = Math.min(95, Math.max(5, ((clientX - rect.left) / rect.width) * 100));
      onSwipeChange(Math.round(pct));
    };
    updateFromClientX(event.clientX);
    const onMove = (moveEvent: PointerEvent) => updateFromClientX(moveEvent.clientX);
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  };

  const selectedMigrationPeriod = useMemo(() => {
    if (!selectedKey) return undefined;
    const migration = migrations.find((item) => item.id === selectedKey);
    return migration?.period ?? undefined;
  }, [migrations, selectedKey]);

  const visibleMarkers = markers.filter((marker) => overlay.markers[marker.key]?.visible);
  const visibleInferences = inferences.filter((seal) => overlay.inferences[seal.key]?.visible);
  const visibleRegionLabels = regions.filter((region) => overlay.regionLabels[region.id]?.visible);
  // The crosshair's own projected slot: set each frame by the same loop the
  // markers use, so the pinned point never drifts from its own coordinates.
  const comparePosition = overlay.compare;
  const comparePositionStyle: CSSProperties | undefined = comparePosition
    ? { left: comparePosition.left, top: comparePosition.top, opacity: comparePosition.visible ? 1 : 0 }
    : undefined;

  return (
    <div className="atlas-map" ref={containerRef} role="region" aria-label="خريطة الأطلس">
      {swipeActive && swipePercent !== null ? (
        <div
          className="atlas-swipe-layer"
          ref={swipeContainerRef}
          aria-hidden="true"
          style={{ clipPath: `inset(0 ${100 - swipePercent}% 0 0)` }}
        />
      ) : null}
      {plateLayers.filter((plate) => failedPlates.includes(plate.key)).map((plate) => (
        <p className="atlas-plate-tile-error" key={plate.key} role="status">تعذّر تحميل بعض أجزاء اللوحة · {plate.title}</p>
      ))}
      <div className="atlas-marker-overlay">
        {overlay.plateTribes.length > 0 && (
          <div role="group" aria-label="الأسماء العربية على اللوحة التاريخية" lang="ar" dir="rtl">
            {overlay.plateTribes.map(({ label, x, y }) => onSelectCaption ? (
              <button key={label.id} type="button" className="atlas-plate-tribe-label" style={{ left: x, top: y }}
                data-printed-name={label.printedName} onClick={(event) => {
                  event.stopPropagation();
                  onSelectCaption(label);
                }}>
                {label.nameAr}
              </button>
            ) : (
              <span key={label.id} className="atlas-plate-tribe-label" style={{ left: x, top: y }}
                data-printed-name={label.printedName}>{label.nameAr}</span>
            ))}
          </div>
        )}
        {overlay.ottomanLabels.length > 0 && (
          <div role="group" aria-label="قبائل خريطة ٩٣٦٧٧ العثمانية" lang="ar" dir="rtl">
            {overlay.ottomanLabels.map(({ territory, x, y }) => (
              <button
                key={territory.id}
                type="button"
                className={`atlas-plate-tribe-label atlas-ottoman-tribe-label${selectedOttomanId === territory.id ? " atlas-ottoman-tribe-label-selected" : ""}`}
                style={{ left: x, top: y }}
                onClick={(event) => {
                  event.stopPropagation();
                  onSelectOttomanTerritory?.(territory);
                }}
              >
                {territory.nameAr}
              </button>
            ))}
          </div>
        )}
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
              onMouseEnter={() => setHovered({
                id: marker.key,
                kind: "marker",
                title: marker.label,
                subtitle: `${formatMapCount(marker.count)} إشارة`,
                statusLabel: mapStatusLabel(marker.status),
                x: position.left,
                y: position.top,
              })}
              onMouseLeave={() => setHovered((current) => current?.id === marker.key ? null : current)}
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
          if (!position?.visible) return null;
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
      {swipeActive && swipePercent !== null && onSwipeChange ? (
        <div className="atlas-swipe-divider" style={{ left: `${swipePercent}%` }}>
          <button
            type="button"
            className="atlas-swipe-handle"
            role="slider"
            aria-label="ستارة مقارنة اللوحة التاريخية"
            aria-valuemin={5}
            aria-valuemax={95}
            aria-valuenow={swipePercent}
            onPointerDown={handleSwipePointerDown}
            onKeyDown={(event) => {
              if (event.key === "ArrowLeft") { event.preventDefault(); onSwipeChange(Math.max(5, swipePercent - 5)); }
              if (event.key === "ArrowRight") { event.preventDefault(); onSwipeChange(Math.min(95, swipePercent + 5)); }
            }}
          >
            <span aria-hidden="true">⇆</span>
          </button>
          <span className="atlas-swipe-tag atlas-swipe-tag-plate" aria-hidden="true">اللوحة التاريخية</span>
          <span className="atlas-swipe-tag atlas-swipe-tag-atlas" aria-hidden="true">الأطلس الموثّق</span>
        </div>
      ) : null}
      {hovered ? (
        <div
          className="atlas-hover-tooltip"
          role="tooltip"
          style={{ left: Math.max(16, hovered.x), top: Math.max(16, hovered.y - 16) }}
        >
          <strong>{hovered.title}</strong>
          <span>
            {hovered.subtitle}
            {hovered.statusLabel ? ` · ${hovered.statusLabel}` : ""}
          </span>
        </div>
      ) : null}
      {comparePoint ? (
        <span className="atlas-compare-crosshair" aria-hidden="true" style={comparePositionStyle}>✛</span>
      ) : null}
      {compareArmed ? (
        <div className="atlas-compare-arm-banner" role="status">انقر على الخريطة لاختيار نقطة المقارنة</div>
      ) : null}
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
