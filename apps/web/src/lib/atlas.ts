import type { FeatureCollection, LineString, Point, Polygon } from "geojson";
import type { MapFeature } from "../types";
import { normalizeArabicName } from "./arabicNames";
import { isMapStatus, mapCertaintyDash, mapKinds, mapRelationLabel, mapStatuses, mapStatusVocabulary, type MapKind, type MapStatus, type MapStatusCopy } from "./map-vocabulary";

/**
 * Pure map encoders.
 *
 * Everything the canvas draws is derived from the fetched `MapFeature[]` by these
 * functions, and they are tested without a browser. The rule they enforce is the
 * plan's encoding contract: a marker is a claim-with-evidence unit, a shared
 * place and status aggregates into ONE marker with a count, a region is a ring
 * rather than a centroid dot, and a migration is a line plus an arrowhead.
 */

export interface AtlasViewport {
  minLongitude: number;
  minLatitude: number;
  maxLongitude: number;
  maxLatitude: number;
}

export interface AtlasMarker {
  /** Stable across refetches: the anchor place and status, or the feature id. */
  key: string;
  status: MapStatus;
  kind: MapFeature["kind"];
  longitude: number;
  latitude: number;
  label: string;
  placeId?: string;
  placeName?: string;
  count: number;
  members: MapFeature[];
}

export interface AtlasFilters {
  statuses: MapStatus[];
  kinds: MapFeature["kind"][];
  fromYear: number;
  toYear: number;
  query: string;
}

export function featureKey(feature: MapFeature): string {
  return feature.claimId || feature.id;
}

export function featurePoint(feature: MapFeature): [number, number] | null {
  if (feature.kind === "migration") {
    if (feature.toLongitude !== undefined && feature.toLatitude !== undefined) return [feature.toLongitude, feature.toLatitude];
    if (feature.fromLongitude !== undefined && feature.fromLatitude !== undefined) return [feature.fromLongitude, feature.fromLatitude];
  }
  if (feature.longitude !== undefined && feature.latitude !== undefined) return [feature.longitude, feature.latitude];
  if (feature.ring && feature.ring.length > 0) return ringCentroid(feature.ring);
  return null;
}

export function ringCentroid(ring: [number, number][]): [number, number] {
  // The area-weighted centroid (the shoelace formula), not the vertex mean: a
  // vertex mean wanders with the sampling of the ring and can fall outside a
  // hard-concave outline like an Asir-type coastal wedge. The formula is the
  // planar one in degree space — coordinates this small describe approximate
  // areas by warrant, so the spherical correction is precision the record
  // does not claim.
  let twiceArea = 0;
  let centroidLongitude = 0;
  let centroidLatitude = 0;
  let meanLongitude = 0;
  let meanLatitude = 0;
  let previous = ring[ring.length - 1];
  for (const current of ring) {
    const cross = previous[0] * current[1] - current[0] * previous[1];
    twiceArea += cross;
    centroidLongitude += (previous[0] + current[0]) * cross;
    centroidLatitude += (previous[1] + current[1]) * cross;
    meanLongitude += current[0];
    meanLatitude += current[1];
    previous = current;
  }
  // A degenerate (collinear) ring has no area to weight by; the vertex mean
  // is the only answer left, wrongness included.
  if (Math.abs(twiceArea) < 1e-12) {
    return [meanLongitude / ring.length, meanLatitude / ring.length];
  }
  return [centroidLongitude / (3 * twiceArea), centroidLatitude / (3 * twiceArea)];
}

export function featureLabel(feature: MapFeature): string {
  return feature.placeName || feature.entityName || mapRelationLabel(feature.relationType) || feature.kind;
}

/**
 * One marker per (place, status). A place feature and the association rows that
 * share that place and status are the same statement at two levels of detail;
 * drawing them as three coincident dots is overdraw, not nuance.
 */
export function aggregateFeatures(features: MapFeature[]): AtlasMarker[] {
  const groups = new Map<string, MapFeature[]>();
  for (const feature of features) {
    if (feature.kind === "region" || feature.kind === "migration") continue;
    if (!featurePoint(feature)) continue;
    if (!isMapStatus(feature.status)) continue;
    const anchor = feature.placeId || feature.id;
    const key = `${anchor}:${feature.status}`;
    const group = groups.get(key);
    if (group) group.push(feature);
    else groups.set(key, [feature]);
  }
  return [...groups.entries()].map(([key, members]) => {
    const anchor = members[0];
    const point = featurePoint(anchor) as [number, number];
    return {
      key,
      status: anchor.status as MapStatus,
      kind: anchor.kind,
      longitude: point[0],
      latitude: point[1],
      label: anchor.placeName || anchor.entityName || featureLabel(anchor),
      placeId: anchor.placeId,
      placeName: anchor.placeName,
      count: members.length,
      members,
    };
  }).sort((left, right) => left.label.localeCompare(right.label, "ar") || left.key.localeCompare(right.key));
}

export function markerCollection(markers: AtlasMarker[]): FeatureCollection<Point, Record<string, string | number>> {
  return {
    type: "FeatureCollection",
    features: markers.map((marker) => ({
      type: "Feature",
      geometry: { type: "Point", coordinates: [marker.longitude, marker.latitude] },
      properties: {
        key: marker.key,
        label: marker.label,
        status: marker.status,
        count: marker.count,
        placeId: marker.placeId ?? "",
      },
    })),
  };
}

export function ringPolygon(ring: [number, number][] | undefined): Polygon | null {
  if (!ring || ring.length < 3) return null;
  const positions = ring.map(([longitude, latitude]) => [longitude, latitude]);
  const first = positions[0];
  const last = positions[positions.length - 1];
  if (first[0] !== last[0] || first[1] !== last[1]) positions.push([first[0], first[1]]);
  if (positions.length < 4) return null;
  return { type: "Polygon", coordinates: [positions] };
}

/**
 * The paint match a status-driven canvas color is built from, with the
 * vocabulary as its only color source. The STRUCTURE is the contract the
 * tests pin — `["match", ["get", "status"], <label, value>×5, <fallback>]` —
 * because a broken paint expression does not fail loudly: MapLibre validates
 * it at paint time, the layer dies silently, and the map keeps rendering
 * everything that does not read the expression (fill patterns, labels, DOM
 * markers). One header element dropped and the region layer simply is not
 * there, which is exactly what happened.
 */
export function statusMatchExpression(pick: (copy: MapStatusCopy) => string, fallback: string): unknown[] {
  return ["match", ["get", "status"], ...mapStatuses.flatMap((status) => [status, pick(mapStatusVocabulary[status])]), fallback];
}

export function regionCollection(features: MapFeature[]): FeatureCollection<Polygon, Record<string, string>> {
  return {
    type: "FeatureCollection",
    features: features.flatMap((feature) => {
      const geometry = ringPolygon(feature.ring);
      if (!geometry) return [];
      return [{
        type: "Feature" as const,
        geometry,
        properties: {
          id: feature.id,
          label: feature.placeName || feature.entityName || "منطقة",
          status: feature.status,
          certainty: feature.certainty ?? "",
          // The undated marker, ALWAYS present as a string: the canvas filters
          // (`==`/`!=` on this property) error on a missing property — `get`
          // returns null and MapLibre refuses a null/string comparison — and a
          // layer that errors paints nothing at all. Absent → "true", the
          // ordinary voice; the weak voice only under an era window.
          dated: feature.datedInWindow === false ? "false" : "true",
        },
      }];
    }),
  };
}

export function migrationLine(feature: MapFeature): LineString | null {
  if (feature.kind !== "migration") return null;
  if (feature.fromLongitude === undefined || feature.fromLatitude === undefined) return null;
  if (feature.toLongitude === undefined || feature.toLatitude === undefined) return null;
  return { type: "LineString", coordinates: [[feature.fromLongitude, feature.fromLatitude], [feature.toLongitude, feature.toLatitude]] };
}

export function migrationCollection(features: MapFeature[]): FeatureCollection<LineString, Record<string, string | number[]>> {
  return {
    type: "FeatureCollection",
    features: features.flatMap((feature) => {
      const geometry = migrationLine(feature);
      if (!geometry) return [];
      return [{
        type: "Feature" as const,
        geometry,
        properties: {
          id: feature.id,
          status: feature.status,
          certainty: feature.certainty ?? "",
          dash: mapCertaintyDash(feature.certainty),
        },
      }];
    }),
  };
}

export function bearingBetween(from: [number, number], to: [number, number]): number {
  const radians = Math.PI / 180;
  const fromLat = from[1] * radians;
  const toLat = to[1] * radians;
  const deltaLongitude = (to[0] - from[0]) * radians;
  const y = Math.sin(deltaLongitude) * Math.cos(toLat);
  const x = Math.cos(fromLat) * Math.sin(toLat) - Math.sin(fromLat) * Math.cos(toLat) * Math.cos(deltaLongitude);
  return (Math.atan2(y, x) / radians + 360) % 360;
}

export function migrationArrowCollection(features: MapFeature[]): FeatureCollection<Point, { id: string; bearing: number; status: string }> {
  return {
    type: "FeatureCollection",
    features: features.flatMap((feature) => {
      if (feature.kind !== "migration") return [];
      if (feature.fromLongitude === undefined || feature.fromLatitude === undefined || feature.toLongitude === undefined || feature.toLatitude === undefined) return [];
      return [{
        type: "Feature" as const,
        geometry: { type: "Point" as const, coordinates: [feature.toLongitude, feature.toLatitude] },
        properties: { id: feature.id, bearing: bearingBetween([feature.fromLongitude, feature.fromLatitude], [feature.toLongitude, feature.toLatitude]), status: feature.status },
      }];
    }),
  };
}

export function parseYear(value: string | undefined): number | null {
  if (!value) return null;
  const match = /^(-?\d{1,6})/.exec(value.trim());
  if (!match) return null;
  const year = Number(match[1]);
  return Number.isFinite(year) ? year : null;
}

/**
 * The same overlap rule the API applies: a record with no dates at all is kept
 * under every year filter. A platform does not know when a place was, and
 * dropping that row would turn "unknown" into "outside this period".
 */
export function featureMatchesYear(feature: MapFeature, fromYear: number, toYear: number): boolean {
  const from = parseYear(feature.timeFrom);
  const to = parseYear(feature.timeTo);
  if (from === null && to === null) return true;
  if (toYear > 0 && from !== null && from > toYear) return false;
  if (fromYear > 0 && to !== null && to < fromYear) return false;
  return true;
}

export function yearBounds(features: MapFeature[]): { min: number; max: number } | null {
  let min: number | null = null;
  let max: number | null = null;
  for (const feature of features) {
    const from = parseYear(feature.timeFrom);
    const to = parseYear(feature.timeTo) ?? from;
    if (from !== null) min = min === null ? from : Math.min(min, from);
    if (to !== null) max = max === null ? to : Math.max(max, to);
  }
  if (min === null || max === null) return null;
  if (min === max) return { min: min - 1, max: max + 1 };
  return { min, max };
}

export interface PeriodPreset {
  id: string;
  from: number;
  to: number;
  label: string;
}

/**
 * Presets are derived from the fetched record, not from three hardcoded
 * decades. Every option therefore filters something, and the toolbar never
 * offers a period the archive does not contain.
 */
export function periodPresets(bounds: { min: number; max: number } | null): PeriodPreset[] {
  if (!bounds) return [];
  const span = bounds.max - bounds.min;
  const first = bounds.min + Math.floor(span / 3);
  const second = bounds.min + Math.floor((span * 2) / 3);
  return [
    { id: "early", from: bounds.min, to: first, label: `${bounds.min} — ${first}` },
    { id: "middle", from: first + 1, to: second, label: `${first + 1} — ${second}` },
    { id: "late", from: second + 1, to: bounds.max, label: `${second + 1} — ${bounds.max}` },
  ];
}

export function filterFeatures(features: MapFeature[], filters: AtlasFilters): MapFeature[] {
  const needle = normalizeArabicName(filters.query);
  return features.filter((feature) => {
    if (!isMapStatus(feature.status) || !filters.statuses.includes(feature.status)) return false;
    if (!filters.kinds.includes(feature.kind)) return false;
    if (!featureMatchesYear(feature, filters.fromYear, filters.toYear)) return false;
    if (needle === "") return true;
    const haystack = normalizeArabicName(
      [feature.placeName, feature.entityName, feature.relationType, feature.period, mapRelationLabel(feature.relationType)]
        .filter(Boolean)
        .join(" "),
    );
    return haystack.includes(needle);
  });
}

export function countStatuses(features: MapFeature[]): Record<MapStatus, number> {
  const counts: Record<MapStatus, number> = { documented: 0, interpreted: 0, platform_inferred: 0, disputed: 0, unresolved: 0 };
  for (const feature of features) {
    if (isMapStatus(feature.status)) counts[feature.status] += 1;
  }
  return counts;
}

export function viewportForCoordinates(points: [number, number][]): AtlasViewport | null {
  if (points.length === 0) return null;
  let minLongitude = points[0][0];
  let maxLongitude = points[0][0];
  let minLatitude = points[0][1];
  let maxLatitude = points[0][1];
  for (const [longitude, latitude] of points) {
    minLongitude = Math.min(minLongitude, longitude);
    maxLongitude = Math.max(maxLongitude, longitude);
    minLatitude = Math.min(minLatitude, latitude);
    maxLatitude = Math.max(maxLatitude, latitude);
  }
  // A single point has no extent; give it a small readable window rather than
  // asking the map to fit a zero-area box.
  if (minLongitude === maxLongitude) {
    minLongitude -= 0.5;
    maxLongitude += 0.5;
  }
  if (minLatitude === maxLatitude) {
    minLatitude -= 0.5;
    maxLatitude += 0.5;
  }
  return { minLongitude, minLatitude, maxLongitude, maxLatitude };
}

export function viewportForFeatures(features: MapFeature[]): AtlasViewport | null {
  const points: [number, number][] = [];
  for (const feature of features) {
    const point = featurePoint(feature);
    if (point) points.push(point);
    if (feature.ring) points.push(...feature.ring);
  }
  return viewportForCoordinates(points);
}

/**
 * The atlas URL is the research position, so it is parsed in one place and
 * validated against the same vocabularies the canvas draws from. A hand-edited
 * `status=fact` is ignored rather than forwarded to the API, where it would
 * silently return nothing and look like an empty archive.
 */
export interface AtlasSearch {
  place?: string;
  feature?: string;
  subject?: string;
  from?: number;
  to?: number;
  status?: string;
  kind?: string;
  q?: string;
}

const searchYearPattern = /^-?\d{1,6}$/;

function searchString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function searchYear(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isInteger(value)) return Math.max(-10000, Math.min(10000, value));
  if (typeof value !== "string" || !searchYearPattern.test(value.trim())) return undefined;
  return Math.max(-10000, Math.min(10000, Number(value.trim())));
}

function searchSet(value: unknown, allowed: readonly string[]): string | undefined {
  const raw = searchString(value);
  if (!raw) return undefined;
  if (raw === "none") return "none";
  const values = [...new Set(raw.split(",").map((item) => item.trim()).filter((item) => allowed.includes(item)))];
  if (values.length === 0) return "none";
  return values.join(",");
}

export function parseAtlasSearch(search: Record<string, unknown>): AtlasSearch {
  const from = searchYear(search.from);
  const to = searchYear(search.to);
  return {
    place: searchString(search.place),
    feature: searchString(search.feature),
    subject: searchString(search.subject),
    from: from !== undefined && to !== undefined ? Math.min(from, to) : from,
    to: from !== undefined && to !== undefined ? Math.max(from, to) : to,
    status: searchSet(search.status, mapStatuses),
    kind: searchSet(search.kind, mapKinds),
    q: searchString(search.q),
  };
}

/** Reads a CSV filter back into the values it names, with `none` meaning none. */
export function atlasFilterValues<T extends string>(raw: string | undefined, allowed: readonly T[]): T[] {
  if (raw === "none") return [];
  if (raw === undefined) return [...allowed];
  const values = raw.split(",").map((item) => item.trim()).filter((item): item is T => (allowed as readonly string[]).includes(item));
  return values.length > 0 ? [...new Set(values)] : [];
}

/** Encodes a filter set compactly, including the deliberate empty set. */
export function encodeAtlasFilter<T extends string>(values: T[], allowed: readonly T[]): string | undefined {
  if (values.length === 0) return "none";
  if (values.length === allowed.length && allowed.every((value) => values.includes(value))) return undefined;
  return allowed.filter((value) => values.includes(value)).join(",");
}

export function activeFilterCount(search: AtlasSearch): number {
  let count = 0;
  if (search.status !== undefined) count += 1;
  if (search.kind !== undefined) count += 1;
  if (search.from !== undefined || search.to !== undefined) count += 1;
  if (search.q) count += 1;
  if (search.place) count += 1;
  if (search.subject) count += 1;
  return count;
}

/** The kinds a feature can be filtered by, in the order the legend lists them. */
export const atlasKinds: MapKind[] = [...mapKinds];

/** Distinct sources, evidence and claims carried by an aggregated marker. */
export function markerProvenance(members: MapFeature[]): {
  sources: Map<string, string>;
  evidence: MapFeature[];
  claims: string[];
} {
  const sources = new Map<string, string>();
  const evidence: MapFeature[] = [];
  const claims: string[] = [];
  for (const member of members) {
    if (member.sourceId) sources.set(member.sourceId, member.sourceTitle || "مصدر بلا عنوان");
    if (member.evidenceId || member.evidenceText) evidence.push(member);
    if (member.claimId && !claims.includes(member.claimId)) claims.push(member.claimId);
  }
  return { sources, evidence, claims };
}
