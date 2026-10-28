import { afterEach, expect, it, vi } from "vitest";
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.unstubAllEnvs(); vi.resetModules(); });
async function api() { vi.stubEnv("VITE_API_URL", "http://localhost:8181"); vi.resetModules(); return import("./api"); }
const run = { id: "run-1", sourceFileId: "file-1", status: "queued", stage: "queued", detected: 0 };
it("enqueues preparation and resumes/cancels only through explicit controls", async () => {
 const client = await api(); const fetcher = vi.fn(async () => new Response(JSON.stringify(run), { status: 202 })); vi.stubGlobal("fetch", fetcher);
 await expect(client.readSourceTreeSuggestions("file-1")).resolves.toEqual(run);
 expect(fetcher).toHaveBeenLastCalledWith("http://localhost:8181/api/v1/source-files/file-1/suggestions/read", expect.objectContaining({ method: "POST", credentials: "include" }));
 await client.rereadSourceSuggestionLocally("suggestion-1");
 expect(fetcher).toHaveBeenLastCalledWith("http://localhost:8181/api/v1/source-tree-suggestions/suggestion-1/local-reread", expect.objectContaining({ method: "POST" }));
 await client.controlNameReadingRun("run-1", "resume");
 expect(fetcher).toHaveBeenLastCalledWith("http://localhost:8181/api/v1/name-reading-runs/run-1/resume", expect.objectContaining({ method: "POST" }));
 await client.controlNameReadingRun("run-1", "cancel");
 expect(fetcher).toHaveBeenLastCalledWith("http://localhost:8181/api/v1/name-reading-runs/run-1/cancel", expect.objectContaining({ method: "POST" }));
});
it("finds the durable run after reload and treats a missing run as normal", async () => {
 const client = await api(); vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(run))));
 await expect(client.fetchLatestNameReadingRun("file-1")).resolves.toEqual(run);
 vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: "not found" }), { status: 404 })));
 await expect(client.fetchLatestNameReadingRun("file-1")).resolves.toBeNull();
 await expect(client.fetchLatestLocalRereadRun("suggestion-1")).resolves.toBeNull();
});
it("combines caller cancellation with the crop deadline", async () => {
 const client = await api(); const caller = new AbortController(); const deadline = new AbortController();
 vi.spyOn(AbortSignal, "timeout").mockReturnValue(deadline.signal);
 let received: AbortSignal | undefined;
 vi.stubGlobal("fetch", vi.fn((_url, init: RequestInit) => { received = init.signal as AbortSignal; return new Promise((_resolve, reject) => received!.addEventListener("abort", () => reject(received!.reason))); }));
 const crop = client.fetchSourceSuggestionCrop("suggestion-1", caller.signal);
 caller.abort(new DOMException("obsolete crop", "AbortError"));
 await expect(crop).rejects.toMatchObject({ name: "AbortError" });
 expect(received?.aborted).toBe(true); expect(deadline.signal.aborted).toBe(false);
 const combined = client.nameReadingSignal(new AbortController().signal); deadline.abort(); expect(combined.aborted).toBe(true);
});
