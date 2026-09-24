import * as maplibregl from "maplibre-gl";
import type { FeatureCollection, Point } from "geojson";
import { useEffect, useRef } from "react";
import type { PlaceRecord } from "../types";
import "maplibre-gl/dist/maplibre-gl.css";

interface MapLibreCanvasProps {
  places: PlaceRecord[];
  selectedId: string;
  onSelect: (place: PlaceRecord) => void;
}

type PlaceProperties = {
  id: string;
  name: string;
  status: string;
};

function placeCollection(places: PlaceRecord[]): FeatureCollection<Point, PlaceProperties> {
  return {
    type: "FeatureCollection",
    features: places.map((place) => ({
      type: "Feature",
      geometry: {
        type: "Point",
        coordinates: [place.longitude, place.latitude],
      },
      properties: {
        id: place.id,
        name: place.name,
        status: place.status,
      },
    })),
  };
}

export function MapLibreCanvas({ places, selectedId, onSelect }: MapLibreCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const onSelectRef = useRef(onSelect);

  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  useEffect(() => {
    if (!containerRef.current) return undefined;

    const map = new maplibregl.Map({
      container: containerRef.current,
      style: {
        version: 8,
        sources: {},
        layers: [{ id: "dawha-background", type: "background", paint: { "background-color": "#ded1b9" } }],
      },
      center: [46.7, 24.7],
      zoom: 4.7,
      attributionControl: false,
    });
    mapRef.current = map;
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "bottom-left");

    map.on("load", () => {
      map.addSource("dawha-places", { type: "geojson", data: placeCollection(places) });
      map.addLayer({
        id: "dawha-place-points",
        type: "circle",
        source: "dawha-places",
        paint: {
          "circle-radius": 7,
          "circle-color": [
            "match",
            ["get", "status"],
            "موثق",
            "#747153",
            "متنازع عليه",
            "#994438",
            "#B66F4D",
          ],
          "circle-stroke-color": "#2D2520",
          "circle-stroke-width": 1.5,
          "circle-opacity": 0.9,
        },
      });
      map.addLayer({
        id: "dawha-place-rings",
        type: "circle",
        source: "dawha-places",
        paint: {
          "circle-radius": 12,
          "circle-color": "rgba(153, 68, 56, 0.12)",
          "circle-stroke-color": "rgba(153, 68, 56, 0.3)",
          "circle-stroke-width": 1,
        },
      });
      map.on("click", "dawha-place-points", (event: maplibregl.MapLayerMouseEvent) => {
        const id = event.features?.[0]?.properties?.id;
        const place = places.find((item) => item.id === id);
        if (place) onSelectRef.current(place);
      });
      map.on("mouseenter", "dawha-place-points", () => {
        map.getCanvas().style.cursor = "pointer";
      });
      map.on("mouseleave", "dawha-place-points", () => {
        map.getCanvas().style.cursor = "";
      });
    });

    return () => {
      map.remove();
      mapRef.current = null;
    };
  }, [places]);

  useEffect(() => {
    const source = mapRef.current?.getSource("dawha-places") as maplibregl.GeoJSONSource | undefined;
    source?.setData(placeCollection(places));
  }, [places]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !map.isStyleLoaded()) return;
    map.setPaintProperty("dawha-place-rings", "circle-opacity", 0.12);
    map.setPaintProperty("dawha-place-points", "circle-radius", ["case", ["==", ["get", "id"], selectedId], 10, 7]);
  }, [selectedId]);

  return <div ref={containerRef} className="maplibre-shell" aria-label="خريطة مواضع البحث" />;
}
