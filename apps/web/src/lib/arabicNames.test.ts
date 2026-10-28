import { describe, expect, it } from "vitest";

import {
  arabicNameKeys,
  normalizeArabicName,
  rankArabicNames,
  suggestArabicNames,
} from "./arabicNames";

describe("normalizeArabicName", () => {
  it("unifies the alef, ta marbuta, alef maqsura, diacritics, and tatweel", () => {
    expect(normalizeArabicName("أَحْمَـد")).toBe("احمد");
    expect(normalizeArabicName("إبراهيم")).toBe("ابراهيم");
    expect(normalizeArabicName("آمنة")).toBe("امنه");
    expect(normalizeArabicName("ليلى")).toBe("ليلي");
  });

  it("trims repeated whitespace without stripping the space between name parts", () => {
    expect(normalizeArabicName("  عبد   الله  ")).toBe("عبد الله");
  });
});

describe("arabicNameKeys", () => {
  it("keeps the display spelling while deriving canonical, compact, particle, and skeleton keys", () => {
    const keys = arabicNameKeys("آل الصباح");
    expect(keys.display).toBe("آل الصباح");
    expect(keys.canonical).toBe("ال الصباح");
    expect(keys.compact).toBe("الالصباح");
    expect(keys.particle).toBe("صباح");
    expect(keys.skeleton).toBeTruthy();
    expect(keys.display).not.toBe(keys.canonical);
  });

  it("joins a compound written with and without a space without merging its display", () => {
    const spaced = arabicNameKeys("عبد الله");
    const joined = arabicNameKeys("عبدالله");
    expect(spaced.canonical).not.toBe(joined.canonical);
    expect(spaced.compact).toBe(joined.compact);
  });

  it("uses a weak dotless shape only as an additional key", () => {
    const first = arabicNameKeys("بيت");
    const second = arabicNameKeys("تيت");
    expect(first.display).not.toBe(second.display);
    expect(first.skeleton).toBe(second.skeleton);
  });

  it("does not rewrite the display value", () => {
    expect(arabicNameKeys("أَحْمَـد").display).toBe("أَحْمَـد");
  });
});

describe("suggestArabicNames", () => {
  it("puts an exact reader alternative first, even when a common name is a typo away", () => {
    const candidates = suggestArabicNames("احمد", {
      reader: ["محمد", "أحمد"],
      common: ["محمود"],
    });
    expect(candidates[0]).toEqual({ value: "أحمد", source: "reader" });
  });

  it("offers the reader's alternatives, AI candidates, and file names for an empty field, not random common names", () => {
    const candidates = suggestArabicNames("", {
      reader: ["محمد"],
      ai: ["أحمد"],
      file: ["الصباح"],
      common: ["خالد"],
    });
    expect(candidates.map((candidate) => candidate.value)).toEqual(["محمد", "أحمد", "الصباح"]);
    expect(candidates[1].source).toBe("ai");
  });

  it("completes a prefix and a one-letter spelling correction", () => {
    expect(suggestArabicNames("محم", {}).map((candidate) => candidate.value)).toContain("محمد");
    expect(suggestArabicNames("خال", {}).map((candidate) => candidate.value)).toContain("خالد");
    expect(suggestArabicNames("نوره", {}).map((candidate) => candidate.value)).toContain("نورة");
  });

  it("does not match unrelated names just because they share one letter", () => {
    const candidates = suggestArabicNames("ززز", { common: ["محمد", "خالد", "الصباح"] });
    expect(candidates).toEqual([]);
  });

  it("prefers the file's own spelling for a duplicate and caps the list", () => {
    const candidates = suggestArabicNames("عبد", {
      reader: ["عبد الله"],
      file: ["عبد الله", "عبد الرحمن"],
      common: ["عبد العزيز"],
    }, 2);
    expect(candidates).toHaveLength(2);
    expect(candidates[0]).toEqual({ value: "عبد الله", source: "file" });
  });

  it("matches a compound across its spacing key", () => {
    const candidates = suggestArabicNames("عبدالله", { reader: ["عبد الله"] });
    expect(candidates[0]).toEqual({ value: "عبد الله", source: "reader" });
  });

  it("matches a family name across the particle key", () => {
    const candidates = suggestArabicNames("الصباح", { reader: ["آل الصباح"], common: [] });
    expect(candidates[0]).toEqual({ value: "آل الصباح", source: "reader" });
  });

  it("does not invent a candidate when the true name is absent", () => {
    const candidates = suggestArabicNames("جاسم", {
      reader: ["علي", "عبد", "عيد", "عبدالله", "عبيد"],
      file: ["الصباح"],
      common: ["محمد", "أحمد"],
    });
    expect(candidates).toEqual([]);
  });

  it("keeps the evidence and the merged sources for a ranked candidate", () => {
    const candidates = rankArabicNames("عبدالله", {
      reader: ["عبد الله"],
      file: ["عبدالله"],
      common: [],
    });
    expect(candidates[0]).toMatchObject({
      value: "عبدالله",
      source: "file",
      sources: ["reader", "file"],
      ranks: { reader: 1, file: 1 },
    });
    expect(candidates[0].keys.compact).toBe("عبدالله");
  });
});

it("uses explicit file counts and retains a human display spelling without duplicating variants", () => {
 const result = rankArabicNames("زيد", { common: [], file: [
  { value: "زيدان", count: 3, variants: ["زيدان"] },
  { value: "زيدون", count: 40, variants: ["زيدون", "زِيدون"] },
 ] });
 expect(result[0].value).toBe("زيدون");
 expect(result[0].fileCount).toBe(40);
 expect(result.find((entry) => entry.value === "زيدان")?.fileCount).toBe(3);
});
