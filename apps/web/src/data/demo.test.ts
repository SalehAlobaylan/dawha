import { describe, expect, it } from "vitest";
import { demoDashboard } from "./demo";

describe("demo dashboard", () => {
  it("keeps epistemic layers separate", () => {
    expect(demoDashboard.layerLegend.map((layer) => layer.tone)).toEqual([
      "source",
      "claim",
      "interpretation",
      "question",
    ]);
    expect(demoDashboard.openQuestions[0].claimCount).toBe(2);
  });
});
