import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import type { Schemas } from "../api/client";
import { keys } from "../api/hooks";
import { listKey } from "../api/list";

/**
 * Live updates: keeps an SSE connection to the household's event stream and
 * refetches whatever changed. EventSource reconnects by itself after network
 * drops; on every (re)connect the server sends "ready" with its current
 * version, so we catch up on anything missed while offline.
 */
export function useHouseholdEvents(householdId: string) {
  const qc = useQueryClient();

  useEffect(() => {
    const es = new EventSource(`/api/households/${householdId}/events`);

    const refreshListIfNewer = (version: number) => {
      const snap = qc.getQueryData<Schemas["ListSnapshot"]>(listKey(householdId));
      if (!snap || version > snap.version) {
        void qc.invalidateQueries({ queryKey: listKey(householdId) });
      }
    };

    es.addEventListener("ready", (e) => {
      const { version } = JSON.parse((e as MessageEvent<string>).data) as { version: number };
      refreshListIfNewer(version);
    });

    es.onmessage = (e: MessageEvent<string>) => {
      const ev = JSON.parse(e.data) as { kind: string; version?: number };
      if (ev.kind === "list" && ev.version !== undefined) {
        refreshListIfNewer(ev.version);
      } else if (ev.kind === "household") {
        void qc.invalidateQueries({ queryKey: keys.household(householdId) });
        void qc.invalidateQueries({ queryKey: keys.me });
      }
    };

    return () => es.close();
  }, [householdId, qc]);
}
