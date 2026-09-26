import { afterEach, describe, expect, it, vi } from "vitest";

import { demoDashboard } from "../data/demo";

/**
 * Demo mode, on the client.
 *
 * The rule under test: a payload is labelled, and a FAILURE is a failure. The
 * previous version caught every error and returned the bundled demo, which meant a
 * deployment with its API down showed a reader a workspace that did not exist -
 * and the reader had no way to know, because `mode: "demo"` was on a payload the
 * client had manufactured itself.
 *
 * The API base URL is read when the module loads, so each case sets the
 * environment and then imports the module: a static import would pin it at the
 * empty string and every case would take the "no API configured" path, which is
 * the one path that is not what these tests are about.
 */
async function loadApi(baseURL: string) {
  vi.stubEnv("VITE_API_URL", baseURL);
  vi.resetModules();
  return import("./api");
}

describe("fetchDashboard", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("returns the bundled demo when no API is configured at all", async () => {
    // No API base URL is the static-preview case: there is nothing to fail, and
    // the demo is the honest answer rather than a substitute for one.
    const { fetchDashboard } = await loadApi("");
    const result = await fetchDashboard();
    expect(result.mode).toBe("demo");
    expect(result.demo).toBe(true);
  });

  it("throws rather than substituting the demo when the request fails", async () => {
    // A 503 from the dashboard route is the API saying demo mode is off. Turning
    // that into invented numbers is the failure this whole gate exists to stop.
    const { fetchDashboard } = await loadApi("http://localhost:8181");
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: "unavailable", code: "dashboard_unavailable" }), { status: 503 }),
      ),
    );
    await expect(fetchDashboard()).rejects.toThrow();
  });

  it("throws when the network is down, rather than showing the demo", async () => {
    const { fetchDashboard } = await loadApi("http://localhost:8181");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("Failed to fetch");
      }),
    );
    await expect(fetchDashboard()).rejects.toThrow();
  });

  it("labels a payload that arrives without a mode as demo", async () => {
    // The safe misreading of an unlabelled payload is the one that shows the
    // warning. A payload that says "api" is believed; anything else is demo.
    const { fetchDashboard } = await loadApi("http://localhost:8181");
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ workspaceName: "مساحة", metrics: [], activity: [], openQuestions: [], treePreview: {}, layerLegend: [] }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );
    const result = await fetchDashboard();
    expect(result.mode).toBe("demo");
  });

  it("passes an api payload through unchanged", async () => {
    const { fetchDashboard } = await loadApi("http://localhost:8181");
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ ...demoDashboard, mode: "api", demo: false }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    const result = await fetchDashboard();
    expect(result.mode).toBe("api");
  });

  it("takes the demo from the API when the API says it is a demo", async () => {
    // The labelled path: the API is in demo mode, says so, and the client passes
    // that label through rather than inventing one.
    const { fetchDashboard } = await loadApi("http://localhost:8181");
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(demoDashboard), {
            status: 200,
            headers: { "Content-Type": "application/json", "X-Data-Source": "demo" },
          }),
      ),
    );
    const result = await fetchDashboard();
    expect(result.mode).toBe("demo");
    expect(result.demo).toBe(true);
  });
});
