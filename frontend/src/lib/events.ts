import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { keys } from "../api/hooks";
import { getListSync } from "../sync/engine";

/**
 * Live updates: keeps an SSE connection to the household's event stream.
 * List changes go to the sync engine (which pulls a delta if we're behind);
 * membership changes refetch the household. EventSource reconnects by
 * itself after network drops, and on every (re)connect the server sends
 * "ready" with its current version, so nothing missed offline is lost.
 */
export function useHouseholdEvents(householdId: string) {
  const qc = useQueryClient();

  useEffect(() => {
    const es = new EventSource(`/api/households/${householdId}/events`);
    const list = getListSync(householdId);

    es.addEventListener("ready", (e) => {
      const { version } = JSON.parse((e as MessageEvent<string>).data) as { version: number };
      list.onRemoteVersion(version);
    });

    es.onmessage = (e: MessageEvent<string>) => {
      const ev = JSON.parse(e.data) as { kind: string; version?: number };
      if (ev.kind === "list" && ev.version !== undefined) {
        list.onRemoteVersion(ev.version);
      } else if (ev.kind === "household") {
        void qc.invalidateQueries({ queryKey: keys.household(householdId) });
        void qc.invalidateQueries({ queryKey: keys.me });
      }
    };

    return () => es.close();
  }, [householdId, qc]);
}
