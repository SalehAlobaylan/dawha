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

describe("source processing reprocessing requests", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("queues higher-resolution reruns and exact reviewed-page revisions", async () => {
    const { reprocessSourceFile, reextractReviewedPage } = await loadApi("http://localhost:8181");
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      void args;
      return new Response(JSON.stringify({ id: "run-1" }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    await reprocessSourceFile("file/id", 400);
    await reextractReviewedPage("file/id", 2, {
      expectedPageEvidenceId: "evidence-1",
      pageReviewRevisionId: "review-4",
      expectedRevision: 4,
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][0]).toBe("http://localhost:8181/api/v1/source-files/file%2Fid/reprocess");
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({ renderDpi: 400 });
    expect(fetchMock.mock.calls[1][0]).toBe("http://localhost:8181/api/v1/source-files/file%2Fid/pages/2/reextract-reviewed");
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({
      expectedPageEvidenceId: "evidence-1",
      pageReviewRevisionId: "review-4",
      expectedRevision: 4,
    });
  });
});

describe("the suggestion queue", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("reads the queue and records a decision without a body that invents a name", async () => {
    const { decideSourceTreeSuggestion, fetchSourceSuggestionAccuracy, fetchSourceTreeSuggestions } = await loadApi("http://localhost:8181");
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      void args;
      if (String(args[0]).includes("/accuracy")) {
        return new Response(JSON.stringify({ total: 4, accepted: 2, corrected: 1, rejected: 1, deferred: 0, unchangedRate: 0.5, measurable: false }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (args[1]?.method === "POST") {
        return new Response(JSON.stringify({ id: "suggestion-1", status: "corrected" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify([{ id: "suggestion-1", suggestedTextAr: "عبدالله", confidence: 0.9 }]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const queue = await fetchSourceTreeSuggestions("file/id");
    expect(queue).toHaveLength(1);
    expect(fetchMock.mock.calls[0][0]).toContain("/api/v1/source-files/file%2Fid/review-queue?");

    const accuracy = await fetchSourceSuggestionAccuracy("file/id");
    // Four decisions is an anecdote, and the client must not present it as a
    // measurable rate.
    expect(accuracy.measurable).toBe(false);

    await decideSourceTreeSuggestion("suggestion-1", "corrected", "عبدالله");
    const decisionCall = fetchMock.mock.calls.find((call) => call[1]?.method === "POST");
    expect(decisionCall?.[0]).toBe("http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/decision");
    expect(JSON.parse(String(decisionCall?.[1]?.body))).toEqual({ decision: "corrected", textAr: "عبدالله", noteAr: "" });
  });

  it("adds the candidate evidence to the decision body without changing its authority", async () => {
    const { decideSourceTreeSuggestion } = await loadApi("http://localhost:8181");
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      void args;
      return new Response(JSON.stringify({ id: "suggestion-1", status: "corrected" }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    await decideSourceTreeSuggestion("suggestion-1", "corrected", "عبدالله", "", {
      candidateSnapshot: [{
        value: "عبدالله",
        key: "عبدالله",
        sources: ["reader"],
        ranks: { reader: 1 },
        score: 100,
        shown: true,
        position: 1,
      }],
      selectedCandidate: "عبدالله",
      inputMethod: "candidate",
      normalizerVersion: "ar-name-keys-v1",
      candidatePoolVersion: "ar-name-pool-v1",
      durationMs: 1200,
    });

    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toMatchObject({
      decision: "corrected",
      textAr: "عبدالله",
      selectedCandidate: "عبدالله",
      inputMethod: "candidate",
      normalizerVersion: "ar-name-keys-v1",
      candidatePoolVersion: "ar-name-pool-v1",
      durationMs: 1200,
    });
  });

  it("retries once without the snapshot when the server rejects the evidence", async () => {
    const { decideSourceTreeSuggestion } = await loadApi("http://localhost:8181");
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      void args;
      if (fetchMock.mock.calls.length === 1) {
        return new Response(JSON.stringify({ error: "invalid candidate snapshot" }), {
          status: 400,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ id: "suggestion-1", status: "corrected" }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await decideSourceTreeSuggestion("suggestion-1", "corrected", "عبدالله", "", {
      candidateSnapshot: [{
        value: "عبدالله",
        key: "عبدالله",
        sources: ["reader"],
        ranks: { reader: 1 },
        score: 100,
        shown: true,
        position: 1,
      }],
      selectedCandidate: "عبدالله",
      inputMethod: "candidate",
      normalizerVersion: "ar-name-keys-v1",
      candidatePoolVersion: "ar-name-pool-v1",
      durationMs: 1200,
      expectedRevision: 0,
      expectedGeometryRevision: 1,
    });

    expect(result.evidenceRecorded).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({
      decision: "corrected",
      textAr: "عبدالله",
      noteAr: "",
      expectedRevision: 0,
      expectedGeometryRevision: 1,
    });
    expect(warn).toHaveBeenCalledTimes(1);
    expect(String(warn.mock.calls[0][0])).not.toContain("عبدالله");
  });

  it("does not retry when the failure is not an evidence-validation rejection", async () => {
    const { decideSourceTreeSuggestion } = await loadApi("http://localhost:8181");
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      void args;
      return new Response(JSON.stringify({ error: "database unavailable" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(decideSourceTreeSuggestion("suggestion-1", "corrected", "عبدالله", "", {
      candidateSnapshot: [{
        value: "عبدالله",
        key: "عبدالله",
        sources: ["reader"],
        ranks: { reader: 1 },
        score: 100,
        shown: true,
        position: 1,
      }],
      inputMethod: "candidate",
      selectedCandidate: "عبدالله",
    })).rejects.toMatchObject({ status: 503 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("review history and geometry repair", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("reads history and reopens with the revision the reviewer saw", async () => {
    const { fetchSourceSuggestionHistory, reopenSourceSuggestion } = await loadApi("http://localhost:8181");
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      void args;
      if (String(args[0]).endsWith("/history")) {
        return new Response(JSON.stringify({
          suggestion: { id: "suggestion-1", status: "accepted", reviewRevision: 1, geometryRevision: 0, superseded: false },
          originalRegion: { x: 0.1, y: 0.1, width: 0.1, height: 0.1 },
          decisions: [], events: [], regionRevisions: [], lineage: [],
          canReopen: true, canReviseRegion: false, canSplit: false, canMerge: false,
        }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response(JSON.stringify({ id: "suggestion-1", status: "suggested", reviewRevision: 1, geometryRevision: 0, superseded: false }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const history = await fetchSourceSuggestionHistory("suggestion-1");
    expect(history.canReopen).toBe(true);
    await reopenSourceSuggestion("suggestion-1", 1, "أعدت القراءة");
    expect(fetchMock.mock.calls[0][0]).toBe("http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/history");
    expect(fetchMock.mock.calls[1][0]).toBe("http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/reopen");
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({ expectedRevision: 1, noteAr: "أعدت القراءة" });
  });

  it("previews, confirms, splits, and merges without inventing coordinates", async () => {
    const { previewSourceSuggestionRegion, reviseSourceSuggestionRegion, splitSourceTreeSuggestion, mergeSourceTreeSuggestions } = await loadApi("http://localhost:8181");
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      void args;
      const url = String(args[0]);
      if (url.endsWith("/region-preview")) {
        return new Response(JSON.stringify({
          geometryRevision: 0, region: { x: 0.2, y: 0.2, width: 0.3, height: 0.2 },
          coordinateSpace: "native_raster", analysisDpi: 0, sourceChecksum: "a".repeat(64),
          frameWidth: 600, frameHeight: 400, tightBounds: { x: 0, y: 0, width: 1, height: 1 },
          contextBounds: { x: 0, y: 0, width: 1, height: 1 },
          quality: { nameHeightPx: 20, contrast: 1, clipped: false, coloured: false }, tightPng: "ZmFrZQ==", contextPng: "ZmFrZQ==",
        }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response(JSON.stringify({ id: "suggestion-1", status: "suggested", reviewRevision: 0, geometryRevision: 1, superseded: false }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const region = { x: 0.2, y: 0.2, width: 0.3, height: 0.2 };
    const preview = await previewSourceSuggestionRegion("suggestion-1", { expectedGeometryRevision: 0, region });
    expect(preview.sourceChecksum).toHaveLength(64);
    await reviseSourceSuggestionRegion("suggestion-1", {
      expectedGeometryRevision: 0, region, coordinateSpace: preview.coordinateSpace,
      analysisDpi: preview.analysisDpi, sourceChecksum: preview.sourceChecksum,
      frameWidth: preview.frameWidth, frameHeight: preview.frameHeight,
    });
    await splitSourceTreeSuggestion("suggestion-1", {
      expectedReviewRevision: 0, expectedGeometryRevision: 1,
      regions: [{ x: 0.2, y: 0.2, width: 0.15, height: 0.2 }, { x: 0.35, y: 0.2, width: 0.15, height: 0.2 }],
    });
    await mergeSourceTreeSuggestions("suggestion-1", {
      members: [{ id: "suggestion-1", expectedReviewRevision: 0, expectedGeometryRevision: 1 }],
      region,
    });

    expect(fetchMock.mock.calls.map((call) => String(call[0]))).toEqual([
      "http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/region-preview",
      "http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/region",
      "http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/split",
      "http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/merge",
    ]);
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toMatchObject({
      expectedGeometryRevision: 0,
      region,
      sourceChecksum: "a".repeat(64),
      frameWidth: 600,
    });
    expect(JSON.parse(String(fetchMock.mock.calls[2][1]?.body))).toMatchObject({
      expectedReviewRevision: 0,
      expectedGeometryRevision: 1,
      regions: [{ x: 0.2, y: 0.2, width: 0.15, height: 0.2 }, { x: 0.35, y: 0.2, width: 0.15, height: 0.2 }],
    });
  });
});
