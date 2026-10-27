import { describe, expect, it } from "vitest";

import { rankArabicNames } from "./arabicNames";
import { buildCandidateSnapshot, buildShadowCandidateSnapshot, matchSnapshotCandidate } from "./nameReviewEvidence";

describe("buildCandidateSnapshot", () => {
  it("keeps the sources, ranks, keys, and shown order of the ranked candidates", () => {
    const candidates = rankArabicNames("عبدالله", {
      reader: ["عبد الله"],
      file: ["عبدالله"],
      common: ["عبد الرحمن"],
    });
    const snapshot = buildCandidateSnapshot(candidates);

    expect(snapshot[0]).toMatchObject({
      value: "عبدالله",
      key: "عبدالله",
      compactKey: "عبدالله",
      sources: ["reader", "file"],
      ranks: { reader: 1, file: 1 },
      shown: true,
      position: 1,
    });
    expect(snapshot[0].particleKey).toBeTruthy();
    expect(snapshot[0].skeletonKey).toBeTruthy();
  });

  it("adds unmatched AI candidates after the ranked chips without duplicating a key", () => {
    const candidates = rankArabicNames("محمد", { reader: ["محمد"], common: [] });
    const snapshot = buildCandidateSnapshot(candidates, ["أحمد", "محمد", "محمود"]);

    expect(snapshot.map((candidate) => candidate.value)).toEqual(["محمد", "أحمد", "محمود"]);
    expect(snapshot[1]).toMatchObject({ sources: ["ai"], ranks: { ai: 1 }, position: 2 });
    expect(snapshot[2]).toMatchObject({ sources: ["ai"], ranks: { ai: 3 }, position: 3 });
  });

  it("caps the evidence at twelve entries", () => {
    const candidates = rankArabicNames("م", { common: [] }, 12);
    const extras = Array.from({ length: 12 }, (_, index) => `اسم${index}`);
    const snapshot = buildCandidateSnapshot(candidates, extras);
    expect(snapshot).toHaveLength(12);
    expect(new Set(snapshot.map((candidate) => candidate.position)).size).toBe(snapshot.length);
  });
});

describe("matchSnapshotCandidate", () => {
  it("returns the snapshot display value for a typed spelling with a harmless difference", () => {
    const snapshot = buildCandidateSnapshot(rankArabicNames("عبدالله", { reader: ["عبد الله"], common: [] }));
    expect(matchSnapshotCandidate(snapshot, "عبدالله")?.value).toBe("عبد الله");
  });

  it("does not invent an attribution when the text is not in the snapshot", () => {
    const snapshot = buildCandidateSnapshot(rankArabicNames("محمد", { reader: ["محمد"], common: [] }));
    expect(matchSnapshotCandidate(snapshot, "جاسم")).toBeUndefined();
  });
});

describe("buildShadowCandidateSnapshot", () => {
  it("builds the baseline from the original reading and prior file names, before a correction", () => {
    const snapshot = buildShadowCandidateSnapshot("محمد", ["محمود"], ["محمد"]);
    const interactiveAfterCorrection = buildCandidateSnapshot(rankArabicNames("خالد", { reader: ["محمد"] }));
    expect(snapshot.map((candidate) => candidate.value)).toContain("محمد");
    expect(snapshot.map((candidate) => candidate.value)).not.toContain("خالد");
    expect(interactiveAfterCorrection.map((candidate) => candidate.value)).toContain("خالد");
  });
});
