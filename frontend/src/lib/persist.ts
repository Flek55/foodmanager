import type { QueryClient, QueryKey } from "@tanstack/react-query";

// Saves "who am I" and household details to localStorage, so the app can
// start offline. The shopping list itself lives in IndexedDB (src/sync).
//
// Deliberately synchronous: we write on every change and restore before the
// first render. (A throttled/async persister can lose the write when a
// phone freezes the backgrounded app right after login.)

const KEY = "fm.queries.v1";

const shouldPersist = (key: QueryKey) =>
  key[0] === "me" || (key[0] === "household" && key.length === 2);

type Entry = [key: QueryKey, data: unknown, updatedAt: number];

/** Call once, before rendering. */
export function restoreQueries(qc: QueryClient) {
  try {
    const entries = JSON.parse(localStorage.getItem(KEY) ?? "[]") as Entry[];
    for (const [key, data, updatedAt] of entries) {
      // Restored data counts as old, so it's refreshed as soon as possible.
      qc.setQueryData(key, data, { updatedAt });
    }
  } catch {
    // Corrupt or unavailable storage: start fresh.
  }
}

/** Keeps localStorage in sync with the cache. Returns an unsubscribe fn. */
export function persistQueries(qc: QueryClient) {
  const save = () => {
    const entries: Entry[] = qc
      .getQueryCache()
      .getAll()
      // Keep a query as long as it HAS data, even if its last refresh
      // failed offline — that data is exactly what we need next time.
      .filter((q) => shouldPersist(q.queryKey) && q.state.data !== undefined)
      .map((q) => [q.queryKey, q.state.data, q.state.dataUpdatedAt]);
    try {
      localStorage.setItem(KEY, JSON.stringify(entries));
    } catch {
      // Quota or private mode: offline start just won't work.
    }
  };
  return qc.getQueryCache().subscribe((event) => {
    if (shouldPersist(event.query.queryKey) && event.type !== "observerAdded" && event.type !== "observerRemoved") {
      save();
    }
  });
}
