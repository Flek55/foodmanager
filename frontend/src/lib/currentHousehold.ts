import { useCallback, useSyncExternalStore } from "react";
import type { Schemas } from "../api/client";

// Which household is "open" is a per-device preference, so it lives in
// localStorage. useSyncExternalStore lets every component that reads it
// re-render when it changes (even from another tab).

const STORAGE_KEY = "fm.currentHousehold";
const listeners = new Set<() => void>();

function read(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

export function setCurrentHousehold(id: string) {
  try {
    localStorage.setItem(STORAGE_KEY, id);
  } catch {
    // Private mode etc.: selection just won't persist across reloads.
  }
  listeners.forEach((l) => l());
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  window.addEventListener("storage", cb);
  return () => {
    listeners.delete(cb);
    window.removeEventListener("storage", cb);
  };
}

/**
 * Returns the household the user is working in: the saved one if they're
 * still a member of it, otherwise the first one they belong to.
 */
export function useCurrentHousehold(me: Schemas["Me"]) {
  const saved = useSyncExternalStore(subscribe, read, () => null);
  const current = me.households.find((h) => h.id === saved) ?? me.households[0] ?? null;
  const select = useCallback((id: string) => setCurrentHousehold(id), []);
  return [current, select] as const;
}
