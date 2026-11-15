import { describe, expect, it } from "vitest";
import { COASTLINE_BOUNDS, coastlineLandCollection, coastlineLineCollection, coastlinePointCount } from "./coastline";

describe("coastline reference geography", () => {
  it("draws a peninsula-sized landmass", () => {
    const land = coastlineLandCollection();
    expect(land.features).toHaveLength(1);
    expect(land.features[0].geometry.type).toBe("MultiPolygon");
    expect(coastlinePointCount).toBeGreaterThan(400);
  });

  it("keeps every vertex inside the peninsula window the atlas frames", () => {
    const line = coastlineLineCollection();
    for (const feature of line.features) {
      for (const [longitude, latitude] of feature.geometry.coordinates) {
        expect(longitude).toBeGreaterThanOrEqual(30);
        expect(longitude).toBeLessThanOrEqual(62);
        expect(latitude).toBeGreaterThanOrEqual(8);
        expect(latitude).toBeLessThanOrEqual(32);
      }
    }
  });

  it("closes every ring so the landmass is a filled area", () => {
    const line = coastlineLineCollection();
    for (const feature of line.features) {
      const coords = feature.geometry.coordinates;
      expect(coords.length).toBeGreaterThanOrEqual(4);
      expect(coords[0]).toEqual(coords[coords.length - 1]);
    }
  });

  it("carries no record fields — the reference never pretends to be archive data", () => {
    expect(coastlineLandCollection().features[0].properties).toEqual({});
    for (const feature of coastlineLineCollection().features) {
      expect(feature.properties).toEqual({});
    }
  });

  it("frames the whole peninsula, not a corner of it", () => {
    expect(COASTLINE_BOUNDS.minLongitude).toBeLessThanOrEqual(34.5);
    expect(COASTLINE_BOUNDS.minLatitude).toBeLessThanOrEqual(11.5);
    expect(COASTLINE_BOUNDS.maxLongitude).toBeGreaterThanOrEqual(57);
    expect(COASTLINE_BOUNDS.maxLatitude).toBeGreaterThanOrEqual(31.5);
  });
});
