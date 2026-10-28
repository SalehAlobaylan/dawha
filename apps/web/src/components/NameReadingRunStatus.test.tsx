// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { NameReadingRunStatus } from "./SourceProcessingPanel";
import * as api from "../lib/api";
vi.mock("../lib/api", async (original) => ({ ...await original<typeof import("../lib/api")>(), fetchLatestNameReadingRun: vi.fn(), controlNameReadingRun: vi.fn() }));
let root: Root | undefined; let host: HTMLDivElement; let client: QueryClient;
afterEach(async () => { if (root) await act(async () => root?.unmount()); client?.clear(); host?.remove(); vi.clearAllMocks(); vi.unstubAllGlobals(); });
it("reloads persisted progress and aborts subscriptions without cancelling the job", async () => {
 vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
 const signals: AbortSignal[] = [];
 vi.mocked(api.fetchLatestNameReadingRun).mockImplementation(async (_file, signal) => { signals.push(signal!); return { id: "run-1", status: "running", stage: "reading", detected: 2, named: 1, unreadable: 0, unavailable: 0, manualReview: 0, sourcePixelWarnings: 0, completedUnits: 2, totalUnits: 3, cacheHits: 1 } as never; });
 for (let reload = 0; reload < 2; reload++) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => { root!.render(<QueryClientProvider client={client}><NameReadingRunStatus fileId="file-1" /></QueryClientProvider>); await new Promise((resolve) => setTimeout(resolve, 0)); });
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });
  expect(host.textContent).toContain("2 موضعاً محفوظاً"); expect(host.textContent).toContain("إيقاف الإعداد");
  await act(async () => root!.unmount()); root = undefined; client.clear(); host.remove();
 }
 expect(api.fetchLatestNameReadingRun).toHaveBeenCalledTimes(2);
 expect(signals).toHaveLength(2);
 expect(api.controlNameReadingRun).not.toHaveBeenCalled();
});
