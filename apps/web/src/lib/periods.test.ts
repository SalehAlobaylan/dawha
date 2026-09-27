import { describe, expect, it } from "vitest";

import { formatRecordedPeriod, unknownPeriod } from "./periods";

// The date-display contract, client half. A period a person wrote is returned as
// they wrote it - including the calendar abbreviation inside it, which is their
// claim about their source and not something the platform may restate. The API's
// `dates.Unknown` and this file's `unknownPeriod` are the same word on purpose: a
// reader who meets "غير محددة" on the map and a dash on the place index has been
// given two answers to one question.
describe("recorded period strings", () => {
  it("returns a recorded period verbatim, including its era abbreviation", () => {
    expect(formatRecordedPeriod("قبل ١١٥٠هـ")).toBe("قبل ١١٥٠هـ");
    expect(formatRecordedPeriod("القرن الثاني عشر")).toBe("القرن الثاني عشر");
  });

  it("says the same word the API says when there is nothing recorded", () => {
    // Not a dash. A dash reads as an empty cell; this is a statement about what the
    // record does not fix.
    expect(formatRecordedPeriod("")).toBe(unknownPeriod);
    expect(formatRecordedPeriod("   ")).toBe(unknownPeriod);
    expect(formatRecordedPeriod(undefined)).toBe(unknownPeriod);
  });

  it("never converts a recorded string into a year", () => {
    // The failure this pins is a "helpful" future conversion: turning an author's
    // sentence into a number and comparing it with a stored date column. Both halves
    // of that are wrong - the record claims no calendar, and the stored column is
    // not the same thing as the sentence. If this test ever fails, read
    // docs/phase-status.md Phase 10 before changing the function.
    const recorded = "قبل ١١٥٠هـ";
    expect(formatRecordedPeriod(recorded)).toBe(recorded);
    expect(formatRecordedPeriod(recorded)).not.toMatch(/\d{3,4}\s*[-—]\s*\d{3,4}/);
    expect(formatRecordedPeriod(recorded)).not.toMatch(/^\d{3,4}$/);
  });
});
