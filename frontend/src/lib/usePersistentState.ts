import { useEffect, useState } from "react";

/** useState that survives reloads (per device), e.g. the chosen store filter. */
export function usePersistentState<T>(key: string, initial: T) {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key);
      return raw === null ? initial : (JSON.parse(raw) as T);
    } catch {
      return initial;
    }
  });

  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch {
      // Storage unavailable: keep it in memory only.
    }
  }, [key, value]);

  return [value, setValue] as const;
}
