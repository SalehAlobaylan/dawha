import * as maplibregl from "maplibre-gl";
import type { Feature, FeatureCollection, LineString, Point } from "geojson";
import { useEffect, useMemo, useRef } from "react";
import type { MapFeature } from "../types";
import "maplibre-gl/dist/maplibre-gl.css";

type HistoricalMapCanvasProps = {
  features: MapFeature[];
  selectedId: string;
  onSelect: (feature: MapFeature) => void;
};

type PointProperties = { id: string; name: string; status: string; kind: string };
type LineProperties = { id: string; status: string; kind: string };

function pointFeature(feature: MapFeature): Feature<Point, PointProperties> | null {
  if (feature.longitude === undefined || feature.latitude === undefined) return null;
  const name = feature.placeName || feature.entityName || feature.relationType || feature.kind;
  return { type: "Feature", geometry: { type: "Point", coordinates: [feature.longitude, feature.latitude] }, properties: { id: feature.id, name, status: feature.status, kind: feature.kind } };
}

function lineFeature(feature: MapFeature): Feature<LineString, LineProperties> | null {
  if (feature.kind !== "migration" || feature.fromLongitude === undefined || feature.fromLatitude === undefined || feature.toLongitude === undefined || feature.toLatitude === undefined) return null;
  return { type: "Feature", geometry: { type: "LineString", coordinates: [[feature.fromLongitude, feature.fromLatitude], [feature.toLongitude, feature.toLatitude]] }, properties: { id: feature.id, status: feature.status, kind: feature.kind } };
}

function featureCollection(features: MapFeature[]): { points: FeatureCollection<Point, PointProperties>; lines: FeatureCollection<LineString, LineProperties> } {
  return {
    points: { type: "FeatureCollection", features: features.map(pointFeature).filter((feature): feature is Feature<Point, PointProperties> => feature !== null) },
    lines: { type: "FeatureCollection", features: features.map(lineFeature).filter((feature): feature is Feature<LineString, LineProperties> => feature !== null) },
  };
}

export function HistoricalMapCanvas({ features, selectedId, onSelect }: HistoricalMapCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const onSelectRef = useRef(onSelect);
  const collection = useMemo(() => featureCollection(features), [features]);

  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  useEffect(() => {
    if (!containerRef.current) return undefined;
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: { version: 8, sources: {}, layers: [{ id: "historical-background", type: "background", paint: { "background-color": "#ded1b9" } }] },
      center: [46.7, 24.7],
      zoom: 4.7,
      attributionControl: false,
    });
    mapRef.current = map;
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "bottom-left");
    map.on("load", () => {
      map.addSource("historical-lines", { type: "geojson", data: collection.lines });
      map.addSource("historical-points", { type: "geojson", data: collection.points });
      map.addLayer({ id: "historical-migration-lines", type: "line", source: "historical-lines", paint: { "line-color": ["match", ["get", "status"], "documented", "#747153", "disputed", "#994438", "#b66f4d"], "line-width": 2, "line-dasharray": [2, 2], "line-opacity": 0.72 } });
      map.addLayer({ id: "historical-point-rings", type: "circle", source: "historical-points", paint: { "circle-radius": 11, "circle-color": "rgba(153, 68, 56, 0.1)", "circle-stroke-color": "rgba(153, 68, 56, 0.3)", "circle-stroke-width": 1 } });
      map.addLayer({ id: "historical-points", type: "circle", source: "historical-points", paint: { "circle-radius": 6, "circle-color": ["match", ["get", "status"], "documented", "#747153", "interpreted", "#b66f4d", "disputed", "#994438", "#c79762"], "circle-stroke-color": "#2d2520", "circle-stroke-width": 1.4, "circle-opacity": 0.92 } });
      map.on("click", "historical-points", (event) => {
        const id = event.features?.[0]?.properties?.id;
        const feature = features.find((item) => item.id === id);
        if (feature) onSelectRef.current(feature);
      });
      map.on("click", "historical-migration-lines", (event) => {
        const id = event.features?.[0]?.properties?.id;
        const feature = features.find((item) => item.id === id);
        if (feature) onSelectRef.current(feature);
      });
      map.on("mouseenter", "historical-points", () => { map.getCanvas().style.cursor = "pointer"; });
      map.on("mouseleave", "historical-points", () => { map.getCanvas().style.cursor = ""; });
    });
    return () => { map.remove(); mapRef.current = null; };
  }, [collection, features]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !map.isStyleLoaded()) return;
    map.setPaintProperty("historical-points", "circle-radius", ["case", ["==", ["get", "id"], selectedId], 10, 6]);
  }, [selectedId, features]);

  return <div ref={containerRef} className="historical-map-canvas" aria-label="خريطة الإشارات الجغرافية المحفوظة" />;
}
