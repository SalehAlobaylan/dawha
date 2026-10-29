export type LibrarySearch = { q?: string; view?: "review" | "trees"; queue?: "deferred"; kind?: "all" | "charts" | "trees" | "pending"; create?: boolean };
export function parseLibrarySearch(search: Record<string, unknown>): LibrarySearch {
  return {
    q: typeof search.q === "string" ? search.q : undefined,
    view: search.view === "review" || search.view === "trees" ? search.view : search.kind === "pending" ? "review" : search.kind === "trees" ? "trees" : undefined,
    queue: search.queue === "deferred" ? "deferred" : undefined,
    kind: search.kind === "charts" || search.kind === "trees" || search.kind === "pending" ? search.kind : undefined,
    create: search.create === true || search.create === "true" ? true : undefined,
  };
}

export type NameReviewSearch = { queue?: "pending" | "deferred" | "history" };
export function parseNameReviewSearch(search: Record<string, unknown>): NameReviewSearch {
  return { queue: search.queue === "pending" || search.queue === "deferred" || search.queue === "history" ? search.queue : undefined };
}

