import { useEffect, useSyncExternalStore } from "react";
import { getListSync } from "./engine";

/**
 * The shopping list of a household, offline-first. Returns what to show
 * (snapshot), sync status, and the engine for making changes:
 *   const { snapshot, status, sync } = useListSync(id);
 *   sync.setChecked(itemId, true);
 */
export function useListSync(householdId: string) {
  const sync = getListSync(householdId);
  useEffect(() => sync.start(), [sync]);
  const state = useSyncExternalStore(sync.subscribe, sync.getState);
  return { ...state, sync };
}
