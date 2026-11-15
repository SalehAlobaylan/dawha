import { describe, expect, it } from "vitest";
import {
  mapCertainties,
  mapCertaintyLabel,
  mapEvidenceStatusLabel,
  mapKindVocabulary,
  mapKinds,
  mapRelationLabel,
  mapStatusLabel,
  mapStatusVocabulary,
  mapStatuses,
} from "./map-vocabulary";

describe("the map vocabulary", () => {
  it("has an entry for every status the API can return", () => {
    expect(Object.keys(mapStatusVocabulary).sort()).toEqual([...mapStatuses].sort());
    for (const status of mapStatuses) {
      expect(mapStatusVocabulary[status].label.length).toBeGreaterThan(0);
      expect(mapStatusVocabulary[status].description.length).toBeGreaterThan(0);
    }
  });

  it("distinguishes status by fill, contour or seal rather than colour alone", () => {
    const signatures = mapStatuses.map((status) => {
      const copy = mapStatusVocabulary[status];
      return `${copy.fill}:${copy.stroke}:${copy.dashed}:${copy.seal}:${copy.hollow}`;
    });
    expect(new Set(signatures).size).toBe(mapStatuses.length);
    expect(mapStatusVocabulary.platform_inferred.seal).toBe(true);
    expect(mapStatusVocabulary.unresolved.hollow).toBe(true);
    expect(mapStatusVocabulary.interpreted.dashed).toBe(true);
  });

  it("is total: an unknown status falls back to unresolved instead of throwing", () => {
    expect(mapStatusLabel("something_new")).toBe("غير محسوم");
    expect(mapStatusLabel(undefined)).toBe("غير محسوم");
  });

  it("covers every certainty and relation the map draws", () => {
    for (const certainty of mapCertainties) expect(mapCertaintyLabel(certainty)).not.toBe("غير محدد");
    expect(mapRelationLabel("documented_in")).toBe("مذكور في");
    expect(mapRelationLabel("migration")).toBe("مسار هجرة");
    expect(mapRelationLabel("custom_relation")).toBe("custom_relation");
    expect(mapRelationLabel(undefined)).toBe("إشارة جغرافية");
  });

  it("names every kind of feature", () => {
    for (const kind of mapKinds) expect(mapKindVocabulary[kind].length).toBeGreaterThan(0);
  });

  it("names the evidence states without inventing a verdict", () => {
    expect(mapEvidenceStatusLabel("accepted")).toBe("مقبول");
    expect(mapEvidenceStatusLabel("needs_review")).toBe("يحتاج مراجعة");
    expect(mapEvidenceStatusLabel(undefined)).toBe("دون دليل مكتمل");
    expect(mapEvidenceStatusLabel("something_else")).toBe("حالة غير معروفة");
  });
});
