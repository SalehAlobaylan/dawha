import { describe, expect, it } from "vitest";
import { getSafeReturnPath } from "./return-path";

describe("getSafeReturnPath", () => {
  const origin = "http://localhost:5173";

  it("returns a requested in-app page with its query and fragment", () => {
    expect(getSafeReturnPath("/sources?tab=private#evidence-workspace", origin)).toBe("/sources?tab=private#evidence-workspace");
  });

  it("keeps invitation links working", () => {
    expect(getSafeReturnPath("/invitation/token-123", origin)).toBe("/invitation/token-123");
  });

  it.each(["https://attacker.example", "//attacker.example/path", "/\\attacker.example", "", null])(
    "falls back to the home page for an unsafe target (%s)",
    (candidate) => {
      expect(getSafeReturnPath(candidate, origin)).toBe("/");
    },
  );
});
