import type { FeatureCollection, LineString, MultiPolygon, Polygon } from "geojson";
import landmass from "./coastline.geo.json";

/**
 * The Arabian coast, drawn for orientation only.
 *
 * This is the one piece of geography on the atlas that is not derived from the
 * archive, and it is here solely so a reader can orient themselves before the
 * archive has anything to say. It is deliberately silent about meaning: it
 * draws where land is and where water is, and nothing about what either of
 * them was called, or who held it, or when.
 *
 * The geometry is the public-domain Natural Earth 50m landmass, clipped to the
 * peninsula and simplified to roughly 660 vertices — close enough to read as
 * the Red Sea, the Gulf of Aqaba, the Gulf and the Arabian Sea, and wrong in
 * the places any 50m simplified line is wrong. No tile service, no live
 * network, no tiles of any kind. The coastline is reference chrome; it never
 * carries a record, and it never becomes a border.
 */
const LAND_GEOJSON = landmass as unknown as MultiPolygon;

/** Bounds for the default fit: the whole peninsula, without squeezing it. */
export const COASTLINE_BOUNDS = { minLongitude: 34.5, minLatitude: 11.2, maxLongitude: 57.5, maxLatitude: 31.9 };

export const coastlinePointCount = LAND_GEOJSON.coordinates.reduce(
  (total, polygon) => total + polygon.reduce((sum, ring) => sum + ring.length, 0),
  0,
);

/** The landmass, filled. The northern chord is data, not geography — never drawn. */
export function coastlineLandCollection(): FeatureCollection<Polygon | MultiPolygon> {
  return {
    type: "FeatureCollection",
    features: [{ type: "Feature", geometry: LAND_GEOJSON, properties: {} }],
  };
}

/** All coastlines, exterior rings and islands alike. */
export function coastlineLineCollection(): FeatureCollection<LineString> {
  const features: FeatureCollection<LineString>["features"] = [];
  for (const polygon of LAND_GEOJSON.coordinates) {
    for (const ring of polygon) {
      features.push({ type: "Feature", geometry: { type: "LineString", coordinates: [...ring] }, properties: {} });
    }
  }
  return { type: "FeatureCollection", features };
}
