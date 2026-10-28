import { afterEach, expect, it, vi } from "vitest";
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.unstubAllEnvs(); vi.resetModules(); });
async function api() { vi.stubEnv("VITE_API_URL", "http://localhost:8181"); vi.resetModules(); return import("./api"); }
const capabilities = { mode: "local", localReader: { ready: false, configured: true, reason: "local_reader_not_implemented", languages: ["ara"], version: "pending", tools: [] }, hostedReader: { ready: false, configured: false, enabled: false, allowedForSource: false, reason: "disabled_by_policy", version: "fixture" } };
it("fetches source-authorized metadata separately with no request payload", async () => {
 const { fetchReadingCapabilities } = await api();
 const fetcher = vi.fn(async () => new Response(JSON.stringify(capabilities)));
 vi.stubGlobal("fetch", fetcher);
 await expect(fetchReadingCapabilities("file-1")).resolves.toEqual(capabilities);
 expect(fetcher).toHaveBeenCalledExactlyOnceWith("http://localhost:8181/api/v1/source-files/file-1/reading-capabilities", expect.objectContaining({ credentials: "include", cache: "no-store" }));
 expect(fetcher.mock.calls[0]).toHaveLength(2);
});
it("fails closed on malformed or contradictory reader availability", async () => {
 const { fetchReadingCapabilities } = await api();
 vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ ...capabilities, hostedReader: { ...capabilities.hostedReader, ready: true } }))));
 expect((await fetchReadingCapabilities("file-1")).hostedReader.ready).toBe(false);
 vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ mode: "typo" }))));
 await expect(fetchReadingCapabilities("file-1")).rejects.toThrow();
});
it.each([[403, "disabled_by_policy"], [503, "not_configured"], [503, "temporarily_unavailable"], [422, "unreadable_crop"]])("preserves typed reader failure %s/%s", async (status, code) => {
 const { fetchSourceSuggestionCandidates } = await api();
 vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: "unavailable", code }), { status: Number(status) })));
 await expect(fetchSourceSuggestionCandidates("suggestion-1")).rejects.toMatchObject({ status, code });
});
