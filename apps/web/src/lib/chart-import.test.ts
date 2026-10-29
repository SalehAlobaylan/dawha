import { describe, expect, it } from "vitest";
import { chartFileError, MAX_CHART_BYTES } from "./chart-import";

describe("chart upload validation", () => {
  it("accepts PDFs at the limit and PDFs without a declared media type", () => {
    expect(chartFileError({ name: "مشجر.pdf", type: "application/pdf", size: MAX_CHART_BYTES })).toBeNull();
    expect(chartFileError({ name: "chart.PDF", type: "", size: 100 })).toBeNull();
  });
  it("refuses other formats, empty files, and oversized files before creating a source", () => {
    expect(chartFileError({ name: "chart.png", type: "image/png", size: 100 })).toContain("PDF");
    expect(chartFileError({ name: "chart.pdf", type: "image/png", size: 100 })).toContain("PDF");
    expect(chartFileError({ name: "chart.pdf", type: "application/pdf", size: 0 })).toContain("فارغ");
    expect(chartFileError({ name: "chart.pdf", type: "application/pdf", size: MAX_CHART_BYTES + 1 })).toContain("128");
  });
});
