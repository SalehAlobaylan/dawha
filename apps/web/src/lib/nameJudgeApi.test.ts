import { afterEach, describe, expect, it, vi } from "vitest";

async function loadApi(baseURL: string) {
  vi.stubEnv("VITE_API_URL", baseURL);
  vi.resetModules();
  return import("./api");
}

const result = {
  choice: "محمد",
  abstained: false,
  probabilities: { "محمد": 0.7, "أحمد": 0.2, unclear: 0.1 },
  confidence: 0.7,
  readable: null,
  provider: "jev",
  model: "jev-pinned",
  promptVersion: "name-judge-v1",
  cacheHit: false,
};

describe("name judge API", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("returns off when no API is configured", async () => {
    const { fetchNameJudgeConfig } = await loadApi("");
    await expect(fetchNameJudgeConfig()).resolves.toEqual({
      mode: "off",
      imageEnabled: false,
      available: false,
      sample: 0,
    });
  });

  it("fetches and bounds the judge configuration", async () => {
    const { fetchNameJudgeConfig } = await loadApi("http://localhost:8181");
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      mode: "shadow",
      imageEnabled: false,
      available: true,
      sample: 2,
    }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const config = await fetchNameJudgeConfig();
    expect(config).toEqual({ mode: "shadow", imageEnabled: false, available: true, sample: 1 });
    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8181/api/v1/name-judge/config",
      expect.objectContaining({ credentials: "include", cache: "no-store" }),
    );
  });

  it("rejects an unknown judge mode rather than treating it as on", async () => {
    const { fetchNameJudgeConfig } = await loadApi("http://localhost:8181");
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ mode: "actve", available: true }), { status: 200 })));
    await expect(fetchNameJudgeConfig()).rejects.toThrow();
  });

  it("sends only candidate values and maps the result", async () => {
    const { judgeSourceTreeSuggestion } = await loadApi("http://localhost:8181");
    let requestedURL = "";
    let requestedInit: RequestInit | undefined;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      requestedURL = String(input);
      requestedInit = init;
      return new Response(JSON.stringify(result), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);
    const mapped = await judgeSourceTreeSuggestion("suggestion-1", [
      { value: "محمد", source: "reader", rank: 1 },
      { value: "أحمد", source: "common", rank: 2 },
    ]);
    expect(mapped.choice).toBe("محمد");
    expect(mapped.probabilities["محمد"]).toBe(0.7);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(requestedURL).toBe("http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/judge");
    const body = JSON.parse(String(requestedInit?.body));
    expect(body).toEqual({
      candidates: [
        { value: "محمد", source: "reader", rank: 1 },
        { value: "أحمد", source: "common", rank: 2 },
      ],
    });
    expect(String(requestedInit?.body)).not.toContain("sourceId");
    expect(String(requestedInit?.body)).not.toContain("familyId");
    expect(String(requestedInit?.body)).not.toContain("accountId");
    expect(String(requestedInit?.body)).not.toContain("reviewer");
  });

  it("keeps local Liquid provider metadata instead of rewriting it", async () => {
    const { judgeSourceTreeSuggestion } = await loadApi("http://localhost:8181");
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      ...result,
      provider: "liquid",
      model: "d1:3b",
      promptVersion: "name-judge-liquid-v1",
    }), { status: 200 })));
    const mapped = await judgeSourceTreeSuggestion("suggestion-1", [
      { value: "محمد", source: "reader", rank: 1 },
    ]);
    expect(mapped.provider).toBe("liquid");
    expect(mapped.model).toBe("d1:3b");
    expect(mapped.promptVersion).toBe("name-judge-liquid-v1");
  });

  it("rejects an inconsistent or malformed result", async () => {
    const { judgeSourceTreeSuggestion } = await loadApi("http://localhost:8181");
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ ...result, abstained: true }), { status: 200 })));
    await expect(judgeSourceTreeSuggestion("suggestion-1", [{ value: "محمد", source: "reader", rank: 1 }])).rejects.toThrow();

    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ ...result, confidence: 2 }), { status: 200 })));
    await expect(judgeSourceTreeSuggestion("suggestion-1", [{ value: "محمد", source: "reader", rank: 1 }])).rejects.toThrow();
  });

  it("propagates a 503 instead of inventing a judge answer", async () => {
    const { judgeSourceTreeSuggestion } = await loadApi("http://localhost:8181");
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: "unavailable" }), { status: 503 })));
    await expect(judgeSourceTreeSuggestion("suggestion-1", [{ value: "محمد", source: "reader", rank: 1 }])).rejects.toThrow();
  });
});
