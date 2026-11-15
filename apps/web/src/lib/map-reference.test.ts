import { describe, expect, it } from "vitest";
import { mapReferenceCaptions, type MapReferenceCaption, type MapReferenceKind } from "./map-reference";

/**
 * The reference artifact is generated data (Wikidata, CC0) consumed by the
 * atlas. These tests pin the contract the generator must satisfy: unique
 * labels, known kinds, coordinates that could sit in the frame, wire-traceable
 * items, and honest placement marks. A regenerated artifact that breaks any of
 * these rules has no business drawing on the map.
 */
describe("generated reference captions", () => {
  const labels = new Set<string>();

  it("has no duplicate labels", () => {
    for (const caption of mapReferenceCaptions) {
      expect(labels.has(caption.label)).toBe(false);
      labels.add(caption.label);
    }
  });

  it("carries known kinds and a recorded source item for every entry", () => {
    for (const caption of mapReferenceCaptions) {
      expect(["sea", "mountain", "city"]).toContain(caption.kind);
      expect(caption.qid).toMatch(/^Q\d+$/);
      expect(["wikidata", "editorial"]).toContain(caption.placement);
    }
  });

  it("keeps coordinates plausible for the atlas frame", () => {
    for (const caption of mapReferenceCaptions) {
      expect(caption.longitude).toBeGreaterThan(30);
      expect(caption.longitude).toBeLessThan(60);
      expect(caption.latitude).toBeGreaterThan(12);
      expect(caption.latitude).toBeLessThan(34);
    }
  });

  it("marks exactly the editorial placements the generator allows (wide seas only)", () => {
    for (const caption of mapReferenceCaptions) {
      if (caption.placement === "editorial") {
        expect(caption.kind).toBe("sea");
        expect(caption.qid).toBe("Q58705");
      }
    }
  });

  it("keeps the three original sea captions stable across regeneration", () => {
    const seaLabels = mapReferenceCaptions
      .filter((caption): caption is MapReferenceCaption & { kind: MapReferenceKind } => caption.kind === "sea")
      .map((caption) => caption.label);
    expect(seaLabels).toContain("البحر الأحمر");
    expect(seaLabels).toContain("الخليج العربي");
    expect(seaLabels).toContain("بحر العرب");
  });
});
