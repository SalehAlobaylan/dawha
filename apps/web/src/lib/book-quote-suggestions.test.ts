import { describe, expect, it } from "vitest";
import { suggestBookQuoteSpans } from "./book-quote-suggestions";

describe("suggestBookQuoteSpans", () => {
  it("suggests exact Arabic passages containing lineage words without structuring them", () => {
    const text = "ذكر المؤلف تاريخ الأسرة. محمد بن عبد الله من أهل البلدة. ثم انتقل إلى مكة.";
    const suggestions = suggestBookQuoteSpans(text);

    expect(suggestions.map((item) => item.text)).toEqual(["محمد بن عبد الله من أهل البلدة"]);
    expect(Array.from(text).slice(suggestions[0].start, suggestions[0].end).join("")).toBe(suggestions[0].text);
    expect(suggestions[0]).not.toHaveProperty("subject");
    expect(suggestions[0]).not.toHaveProperty("predicate");
  });

  it("uses Unicode codepoint offsets and respects whole Arabic words", () => {
    const text = "🧭مقدمة\nبنان مدينة، وبعدها محمد ابن سالم.";
    const suggestions = suggestBookQuoteSpans(text);

    expect(suggestions).toHaveLength(1);
    expect(suggestions[0].text).toBe("وبعدها محمد ابن سالم");
    expect(Array.from(text).slice(suggestions[0].start, suggestions[0].end).join("")).toBe(suggestions[0].text);
  });

  it("bounds the number and size of suggestions", () => {
    expect(suggestBookQuoteSpans("محمد بن سالم. عيسى ابن مريم.", 1)).toHaveLength(1);
    expect(suggestBookQuoteSpans(`${"أ".repeat(801)} ابن سالم.`)).toEqual([]);
  });
});
