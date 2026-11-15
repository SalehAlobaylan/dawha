import { describe, expect, it } from "vitest";
import type { MapFeature } from "../types";
import { mapStatuses, mapStatusVocabulary } from "./map-vocabulary";
import {
  aggregateFeatures,
  atlasFilterValues,
  bearingBetween,
  encodeAtlasFilter,
  featureMatchesYear,
  filterFeatures,
  markerCollection,
  markerProvenance,
  migrationArrowCollection,
  migrationCollection,
  parseAtlasSearch,
  periodPresets,
  regionCollection,
  ringCentroid,
  ringPolygon,
  statusMatchExpression,
  viewportForCoordinates,
  viewportForFeatures,
  yearBounds,
} from "./atlas";

function feature(overrides: Partial<MapFeature> & Pick<MapFeature, "id" | "kind" | "status">): MapFeature {
  return { period: "غير محددة", ...overrides };
}

const riyadhDocumented = feature({ id: "place-riyadh", kind: "place", status: "documented", placeId: "riyadh", placeName: "الرياض", longitude: 46.6753, latitude: 24.7136 });
const riyadhAssociation = feature({ id: "assoc-1", kind: "association", status: "documented", placeId: "riyadh", placeName: "الرياض", entityName: "عبدالله", longitude: 46.6753, latitude: 24.7136, timeFrom: "1180-01-01", timeTo: "1210-12-31" });
const ahsaInferred = feature({ id: "assoc-2", kind: "association", status: "platform_inferred", placeId: "ahsa", placeName: "الأحساء", entityName: "محمد", longitude: 49.5658, latitude: 25.3647, timeFrom: "1100-01-01", timeTo: "1140-12-31" });
const migration = feature({ id: "migration-1", kind: "migration", status: "platform_inferred", entityName: "عبدالله", fromLongitude: 49.5658, fromLatitude: 25.3647, toLongitude: 46.6753, toLatitude: 24.7136, certainty: "uncertain", timeFrom: "1200-01-01", timeTo: "1210-12-31" });
const region = feature({ id: "region-1", kind: "region", status: "interpreted", placeName: "نجد", certainty: "approximate", ring: [[44, 24], [48, 24], [48, 27], [44, 27], [44, 24]] });

describe("aggregation", () => {
  it("draws one marker per place and status and keeps a truthful count", () => {
    const markers = aggregateFeatures([riyadhDocumented, riyadhAssociation, ahsaInferred, migration, region]);
    expect(markers).toHaveLength(2);
    const riyadh = markers.find((marker) => marker.placeId === "riyadh");
    expect(riyadh?.count).toBe(2);
    expect(riyadh?.members.map((member) => member.id)).toEqual(["place-riyadh", "assoc-1"]);
    const ahsa = markers.find((marker) => marker.placeId === "ahsa");
    expect(ahsa?.status).toBe("platform_inferred");
    expect(ahsa?.count).toBe(1);
  });

  it("reports the count through the marker's own properties, never through marker size", () => {
    const collection = markerCollection(aggregateFeatures([riyadhDocumented, riyadhAssociation]));
    expect(collection.features[0].properties.count).toBe(2);
    expect(collection.features[0].properties.status).toBe("documented");
  });
});

describe("time", () => {
  it("keeps an undated record under every year filter, exactly like the API", () => {
    expect(featureMatchesYear(riyadhDocumented, 1100, 1110)).toBe(true);
    expect(featureMatchesYear(riyadhAssociation, 1100, 1110)).toBe(false);
    expect(featureMatchesYear(riyadhAssociation, 1200, 1250)).toBe(true);
  });

  it("derives bounds and presets from the data rather than hardcoded decades", () => {
    const bounds = yearBounds([riyadhAssociation, ahsaInferred, migration]);
    expect(bounds).toEqual({ min: 1100, max: 1210 });
    const presets = periodPresets(bounds);
    expect(presets).toHaveLength(3);
    expect(presets[0].from).toBe(1100);
    expect(presets[2].to).toBe(1210);
    expect(presets.map((preset) => preset.label)).toEqual(["1100 — 1136", "1137 — 1173", "1174 — 1210"]);
  });

  it("returns no presets when the archive has no dated records", () => {
    expect(periodPresets(yearBounds([riyadhDocumented]))).toEqual([]);
  });
});

describe("filters", () => {
  const filters = { statuses: ["documented", "interpreted", "platform_inferred", "disputed", "unresolved"] as MapFeature["status"][], kinds: ["place", "association", "migration", "region"] as MapFeature["kind"][], fromYear: 0, toYear: 0, query: "" };

  it("filters status, kind, year and text over data already in memory", () => {
    expect(filterFeatures([riyadhDocumented, riyadhAssociation, ahsaInferred], { ...filters, statuses: ["documented"] })).toHaveLength(2);
    expect(filterFeatures([riyadhDocumented, riyadhAssociation, ahsaInferred], { ...filters, kinds: ["association"] })).toHaveLength(2);
    expect(filterFeatures([riyadhDocumented, riyadhAssociation, ahsaInferred], { ...filters, fromYear: 1150, toYear: 1200 })).toHaveLength(2);
    expect(filterFeatures([riyadhDocumented, riyadhAssociation, ahsaInferred], { ...filters, query: "الأحساء" })).toHaveLength(1);
  });
});

describe("geometry", () => {
  it("closes and draws a region ring instead of a centroid", () => {
    const collection = regionCollection([region]);
    expect(collection.features).toHaveLength(1);
    expect(collection.features[0].geometry.type).toBe("Polygon");
    expect(collection.features[0].geometry.coordinates[0]).toHaveLength(5);
  });

  it("refuses a degenerate ring rather than drawing a line as an area", () => {
    expect(ringPolygon([[1, 2], [3, 4]])).toBeNull();
    expect(regionCollection([feature({ id: "r", kind: "region", status: "interpreted" })]).features).toHaveLength(0);
  });

  it("turns a migration into a line and an arrowhead pointing at its destination", () => {
    const lines = migrationCollection([migration]);
    expect(lines.features).toHaveLength(1);
    expect(lines.features[0].geometry.coordinates).toEqual([[49.5658, 25.3647], [46.6753, 24.7136]]);
    const arrows = migrationArrowCollection([migration]);
    expect(arrows.features).toHaveLength(1);
    expect(arrows.features[0].properties.bearing).toBeGreaterThan(180);
    expect(arrows.features[0].properties.bearing).toBeLessThan(360);
  });

  it("computes a bearing that stays in range for every direction", () => {
    expect(bearingBetween([0, 0], [0, 1])).toBeCloseTo(0, 1);
    expect(bearingBetween([0, 0], [1, 0])).toBeCloseTo(90, 1);
    expect(bearingBetween([0, 0], [0, -1])).toBeCloseTo(180, 1);
    expect(bearingBetween([0, 0], [-1, 0])).toBeCloseTo(270, 1);
  });

  it("derives a viewport that contains points and rings", () => {
    const viewport = viewportForFeatures([riyadhDocumented, region]);
    expect(viewport).toEqual({ minLongitude: 44, minLatitude: 24, maxLongitude: 48, maxLatitude: 27 });
  });

  it("weights the fallback centroid by area, so a concave ring cannot label outside itself", () => {
    // A C-shaped ring whose vertex mean falls in the notch it cuts: the vertex
    // mean is wrong there, and the shoelace centroid is what keeps a label on
    // the shape. (The server sends its own ST_Centroid; this is the fallback.)
    const cShape: [number, number][] = [
      [0, 0], [3, 0], [3, 3], [2.2, 3], [2.2, 0.8], [0.8, 0.8], [0.8, 3], [0, 3], [0, 0],
    ];
    const centroid = ringCentroid(cShape);
    const mean: [number, number] = [
      cShape.reduce((sum, [x]) => sum + x, 0) / cShape.length,
      cShape.reduce((sum, [, y]) => sum + y, 0) / cShape.length,
    ];
    expect(centroid[1]).toBeLessThan(mean[1]); // the area pulls it down, out of the notch's mouth
    // And it stays inside the ring's own bounds in both axes.
    const xs = cShape.map(([x]) => x);
    const ys = cShape.map(([, y]) => y);
    expect(centroid[0]).toBeGreaterThan(Math.min(...xs));
    expect(centroid[0]).toBeLessThan(Math.max(...xs));
    expect(centroid[1]).toBeGreaterThan(Math.min(...ys));
    expect(centroid[1]).toBeLessThan(Math.max(...ys));
    // A rectangle keeps its geometric centre — the quality the vertex mean
    // pretended to have.
    const rectangle = [[0, 0], [4, 0], [4, 2], [0, 2], [0, 0]] as [number, number][];
    expect(ringCentroid(rectangle)).toEqual([2, 1]);
  });

  it("marks an undated region on the canvas only under an era window", () => {
    const undated = { ...region, datedInWindow: false };
    const collection = regionCollection([undated]);
    expect(collection.features[0].properties.dated).toBe("false");
    // No window, no marker: the property stays a string ("true"), because the
    // canvas's `==`/`!=` filters throw on a missing property (null vs string)
    // and a layer that errors paints nothing. datedInWindow=true (a dated
    // region under a window) keeps the ordinary voice too.
    expect(regionCollection([region]).features[0].properties.dated).toBe("true");
    expect(regionCollection([{ ...region, datedInWindow: true }]).features[0].properties.dated).toBe("true");
  });

  it("keeps the status match expression's structure intact", () => {
    // A broken paint expression does not fail loudly: MapLibre validates it at
    // paint time and the layer dies silently — which is how a refactoring
    // dropped the `["match", ["get", "status"]]` header once and every region
    // fill and outline on the atlas disappeared while the canvas, the hatch,
    // the labels and the DOM markers kept painting. This is the test that
    // would have caught it: header, input, complete label/value pairs over the
    // whole vocabulary, fallback last.
    const expression = statusMatchExpression((copy) => copy.fill, "rgba(45, 37, 32, 0.08)");
    expect(expression[0]).toBe("match");
    expect(expression[1]).toEqual(["get", "status"]);
    expect(expression[expression.length - 1]).toBe("rgba(45, 37, 32, 0.08)");
    const pairs = expression.slice(2, -1);
    expect(pairs).toHaveLength(mapStatuses.length * 2);
    for (let index = 0; index < mapStatuses.length; index += 1) {
      expect(pairs[index * 2]).toBe(mapStatuses[index]);
      expect(typeof pairs[index * 2 + 1]).toBe("string");
    }
    // And the stroke twin is built by the same rule, not by a second hand.
    const strokes = statusMatchExpression((copy) => copy.stroke, "#8b8176");
    expect(strokes[0]).toBe("match");
    expect(strokes[3]).toBe(mapStatusVocabulary[mapStatuses[0]].stroke);
  });

  it("gives a single coordinate a readable window instead of a zero-area box", () => {
    expect(viewportForCoordinates([[46.6, 24.7]])).toEqual({
      minLongitude: 46.1,
      minLatitude: 24.2,
      maxLongitude: 47.1,
      maxLatitude: 25.2,
    });
  });
});

describe("deep-link parsing", () => {
  it("accepts only vocabulary values and keeps the empty set expressible", () => {
    expect(parseAtlasSearch({ status: "documented,interpreted" })).toMatchObject({ status: "documented,interpreted" });
    expect(parseAtlasSearch({ status: "documented,fact" }).status).toBe("documented");
    expect(parseAtlasSearch({ status: "fact" }).status).toBe("none");
    expect(parseAtlasSearch({ status: "none" }).status).toBe("none");
    expect(parseAtlasSearch({ kind: "region,nonsense" }).kind).toBe("region");
  });

  it("swaps a reversed year range rather than forwarding an impossible window", () => {
    expect(parseAtlasSearch({ from: "1200", to: "1100" })).toMatchObject({ from: 1100, to: 1200 });
    expect(parseAtlasSearch({ from: "abc", to: "1200" })).toMatchObject({ from: undefined, to: 1200 });
  });

  it("round-trips a filter set, including all and none", () => {
    expect(atlasFilterValues(undefined, ["documented", "interpreted"] as const)).toEqual(["documented", "interpreted"]);
    expect(atlasFilterValues("none", ["documented"] as const)).toEqual([]);
    expect(atlasFilterValues("interpreted", ["documented", "interpreted"] as const)).toEqual(["interpreted"]);
    expect(encodeAtlasFilter(["documented", "interpreted"], ["documented", "interpreted"] as const)).toBeUndefined();
    expect(encodeAtlasFilter([], ["documented"] as const)).toBe("none");
    expect(encodeAtlasFilter(["interpreted"], ["documented", "interpreted"] as const)).toBe("interpreted");
  });

  it("collects the provenance an aggregated marker actually carries", () => {
    const members = [
      { ...riyadhAssociation, sourceId: "source-1", sourceTitle: "سجل", evidenceId: "e-1", claimId: "claim-1" },
      { ...riyadhAssociation, id: "assoc-2", sourceId: "source-1", sourceTitle: "سجل", evidenceId: "e-2", claimId: "claim-1" },
    ];
    const provenance = markerProvenance(members);
    expect([...provenance.sources.entries()]).toEqual([["source-1", "سجل"]]);
    expect(provenance.evidence).toHaveLength(2);
    expect(provenance.claims).toEqual(["claim-1"]);
  });
});
