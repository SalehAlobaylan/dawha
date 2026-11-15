import { describe, expect, it } from "vitest";
import { atlasEras, eraOfWindow } from "./atlas-periods";

/**
 * The era chips are a named window on the same scrubber value, so the contract
 * is narrow: CE integer windows, exact matching on both edges, and no era that
 * overlaps another one by a day — an overlapping vocabulary would make a window
 * legitimately answer to two names, and the active chip would lie.
 */
describe("atlas era vocabulary", () => {
  it("carries a label and ordered CE bounds for every era", () => {
    for (const era of atlasEras) {
      expect(era.label).not.toBe("");
      expect(Number.isInteger(era.from)).toBe(true);
      expect(Number.isInteger(era.to)).toBe(true);
      expect(era.from).toBeLessThan(era.to);
      expect(era.from).toBeGreaterThan(500);
    }
  });

  it("never overlaps another era", () => {
    for (const era of atlasEras) {
      for (const other of atlasEras) {
        if (era.id === other.id) continue;
        const overlaps = era.from < other.to && other.from < era.to;
        expect(overlaps, `${era.id} overlaps ${other.id}`).toBe(false);
      }
    }
  });

  it("has unique ids and labels", () => {
    const ids = new Set(atlasEras.map((era) => era.id));
    const labels = new Set(atlasEras.map((era) => era.label));
    expect(ids.size).toBe(atlasEras.length);
    expect(labels.size).toBe(atlasEras.length);
  });

  it("matches the active window exactly on both edges", () => {
    const era = atlasEras[0];
    expect(eraOfWindow(era.from, era.to)?.id).toBe(era.id);
    expect(eraOfWindow(era.from + 1, era.to)).toBeUndefined();
    expect(eraOfWindow(era.from, era.to - 1)).toBeUndefined();
    expect(eraOfWindow(0, 0)).toBeUndefined();
  });
});
