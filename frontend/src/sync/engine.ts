// Offline-first sync for one household's shopping list.
//
// State:
//   server  — the last snapshot confirmed by the server (with its version)
//   outbox  — ops made locally and not yet accepted by the server, in order
//   display — server + outbox replayed on top: what the user sees
//
// Both server and outbox live in IndexedDB, so the list opens instantly and
// works with no network; queued changes survive closing the app.
//
// A sync round = flush (send the outbox, oldest first) + pull (fetch what
// others changed since our version). Rounds never overlap; triggers that
// arrive mid-round just schedule one more round.

import { clear, createStore, get, set } from "idb-keyval";
import { api, isApiError, unwrap, type ApiError } from "../api/client";
import { newId } from "../lib/uuid";
import {
  applyOp,
  applyServerResult,
  findForAdd,
  findProduct,
  mergeDelta,
  openItemOf,
  remapAfter,
  sendOp,
  type Op,
  type Snapshot,
  type StockStatus,
} from "./ops";

export type SyncStatus = {
  /** false after a network failure, until a request succeeds again */
  online: boolean;
  syncing: boolean;
  /** changes waiting to be sent */
  pending: number;
};

export type SyncState = {
  /** null until loaded from disk or server; stays null offline on first ever open */
  snapshot: Snapshot | null;
  loaded: boolean;
  status: SyncStatus;
};

export type SyncHooks = {
  /** the server rejected an op for good (validation, conflict…) */
  onDropped: (op: Op, err: ApiError) => void;
  /** session expired or the user lost access to the household */
  onAccessLost: () => void;
};

const db = createStore("foodmanager", "sync");
const RETRY_DELAYS = [2_000, 5_000, 15_000, 30_000];

/** Errors worth retrying later: no network, server hiccups, rate limits. */
function isTransient(err: unknown) {
  return isApiError(err) && (err.status === 0 || err.status === 408 || err.status === 429 || err.status >= 500);
}

export class ListSync {
  private server: Snapshot | null = null;
  private outbox: Op[] = [];
  private state: SyncState = { snapshot: null, loaded: false, status: { online: navigator.onLine, syncing: false, pending: 0 } };
  private listeners = new Set<() => void>();
  private running = false;
  private again = false;
  private retryTimer: ReturnType<typeof setTimeout> | undefined;
  private retryAttempt = 0;
  private started = false;

  constructor(
    readonly householdId: string,
    private hooks: SyncHooks,
  ) {}

  // ---- React glue (useSyncExternalStore) ----

  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  getState = () => this.state;

  // ---- lifecycle ----

  start() {
    if (this.started) return;
    this.started = true;
    window.addEventListener("online", this.onWake);
    document.addEventListener("visibilitychange", this.onVisible);
    void this.load().then(() => this.sync());
  }

  stop() {
    window.removeEventListener("online", this.onWake);
    document.removeEventListener("visibilitychange", this.onVisible);
    clearTimeout(this.retryTimer);
  }

  private onWake = () => this.sync();
  private onVisible = () => document.visibilityState === "visible" && this.sync();

  private async load() {
    try {
      const [server, outbox] = await Promise.all([
        get<Snapshot>(`snapshot:${this.householdId}`, db),
        get<Op[]>(`outbox:${this.householdId}`, db),
      ]);
      // Anything enqueued before the disk answered stays after the stored ops.
      this.server = this.server ?? server ?? null;
      this.outbox = [...(outbox ?? []), ...this.outbox];
    } catch {
      // IndexedDB unavailable (e.g. some private modes): work in memory.
    }
    this.publish({ loaded: true });
  }

  // ---- actions ----

  /** Adds an item; reports whether it merged into an item already on the list. */
  addItem(input: Omit<Extract<Op, { kind: "add" }>, "kind" | "id" | "at"> & { id: string }) {
    const merged = this.state.snapshot ? !!findForAdd(this.state.snapshot, input.name).active : false;
    this.enqueue({ kind: "add", at: new Date().toISOString(), ...input });
    return { merged };
  }

  updateItem(id: string, body: Extract<Op, { kind: "update" }>["body"]) {
    this.enqueue({ kind: "update", id, body });
  }

  setChecked(id: string, checked: boolean) {
    this.enqueue({ kind: "check", id, checked, at: new Date().toISOString() });
  }

  deleteItem(id: string) {
    this.enqueue({ kind: "delete", id });
  }

  clearChecked() {
    this.enqueue({ kind: "clearChecked" });
  }

  /** Marks home stock; reports whether the product was put on the list. */
  setStock(productId: string, status: StockStatus | "untracked", by?: string) {
    const s = this.state.snapshot;
    const addsItem = (status === "low" || status === "out") && !!s && !openItemOf(s, productId);
    this.enqueue({ kind: "stock", productId, status, itemId: newId(), at: new Date().toISOString(), by });
    return { addedToList: addsItem };
  }

  /** Starts tracking a product at home by name (creating it if new). */
  trackProduct(name: string, status: StockStatus, by?: string) {
    const s = this.state.snapshot;
    const existing = s ? findProduct(s, name) : undefined;
    this.enqueue({ kind: "track", productId: newId(), name, status, itemId: newId(), at: new Date().toISOString(), by });
    return { alreadyTracked: existing?.stockStatus !== undefined };
  }

  private enqueue(op: Op) {
    this.outbox = [...this.outbox, op];
    this.persist();
    this.publish();
    this.sync();
  }

  /** Someone changed the list (SSE). Pull only if we're actually behind. */
  onRemoteVersion(version: number) {
    if (!this.server || version > this.server.version) this.sync();
  }

  // ---- the sync loop ----

  sync() {
    if (this.running) {
      this.again = true;
      return;
    }
    void this.runRounds();
  }

  private async runRounds() {
    this.running = true;
    clearTimeout(this.retryTimer);
    this.publish();
    try {
      do {
        this.again = false;
        const flushed = await this.flush();
        if (flushed) await this.pull();
      } while (this.again);
    } finally {
      this.running = false;
      this.publish();
    }
  }

  /** Sends queued ops in order. Returns false if it had to stop early. */
  private async flush(): Promise<boolean> {
    while (this.outbox.length > 0) {
      const op = this.outbox[0]!;
      try {
        const result = await sendOp(this.householdId, op);
        this.outbox = remapAfter(op, result, this.outbox.slice(1));
        if (this.server) this.server = applyServerResult(this.server, op, result);
        this.markOnline();
      } catch (err) {
        if (!this.handleError(err)) return false;
        // Permanent rejection: drop it, tell the user, carry on.
        this.outbox = this.outbox.slice(1);
        this.hooks.onDropped(op, err as ApiError);
      }
      this.persist();
      this.publish();
    }
    return true;
  }

  /** Brings the server snapshot up to date: delta if we have a base, else full. */
  private async pull() {
    const path = { householdId: this.householdId };
    try {
      if (this.server) {
        const delta = await unwrap(
          api.GET("/households/{householdId}/sync", { params: { path, query: { since: this.server.version } } }),
        );
        // Server behind us means its data was reset: start over from scratch.
        this.server =
          delta.version < this.server.version
            ? await unwrap(api.GET("/households/{householdId}/list", { params: { path } }))
            : mergeDelta(this.server, delta);
      } else {
        this.server = await unwrap(api.GET("/households/{householdId}/list", { params: { path } }));
      }
      this.markOnline();
      this.persist();
      this.publish();
    } catch (err) {
      this.handleError(err);
    }
  }

  /**
   * Classifies a failure. Returns true if it's a permanent rejection of this
   * specific request (the caller may drop the op); false if syncing should
   * pause (no network, or no access).
   */
  private handleError(err: unknown): boolean {
    if (isTransient(err)) {
      this.publish({ online: false });
      this.scheduleRetry();
      return false;
    }
    // Logged out, or removed from the household: stop until the app reacts.
    // (A 404 about a single item is a normal per-op rejection, below.)
    if (isApiError(err, 401) || (isApiError(err, 404) && err.code === "household_not_found")) {
      this.hooks.onAccessLost();
      return false;
    }
    return isApiError(err);
  }

  private scheduleRetry() {
    clearTimeout(this.retryTimer);
    const delay = RETRY_DELAYS[Math.min(this.retryAttempt, RETRY_DELAYS.length - 1)]!;
    this.retryAttempt++;
    this.retryTimer = setTimeout(() => this.sync(), delay);
  }

  private markOnline() {
    this.retryAttempt = 0;
    if (!this.state.status.online) this.publish({ online: true });
  }

  // ---- state plumbing ----

  private persist() {
    void set(`snapshot:${this.householdId}`, this.server, db).catch(() => {});
    void set(`outbox:${this.householdId}`, this.outbox, db).catch(() => {});
  }

  private publish(patch: { online?: boolean; loaded?: boolean } = {}) {
    const snapshot = this.server ? this.outbox.reduce(applyOp, this.server) : null;
    this.state = {
      snapshot,
      loaded: patch.loaded ?? this.state.loaded,
      status: {
        online: patch.online ?? this.state.status.online,
        syncing: this.running,
        pending: this.outbox.length,
      },
    };
    this.listeners.forEach((l) => l());
  }
}

// ---- registry: one engine per household for the app's lifetime ----

const engines = new Map<string, ListSync>();
let hooks: SyncHooks = { onDropped: () => {}, onAccessLost: () => {} };

export function configureSync(h: SyncHooks) {
  hooks = h;
}

export function getListSync(householdId: string) {
  let engine = engines.get(householdId);
  if (!engine) {
    engine = new ListSync(householdId, {
      onDropped: (op, err) => hooks.onDropped(op, err),
      onAccessLost: () => hooks.onAccessLost(),
    });
    engines.set(householdId, engine);
  }
  return engine;
}

/** On logout: forget everything stored on this device. */
export async function resetSync() {
  engines.forEach((e) => e.stop());
  engines.clear();
  await clear(db).catch(() => {});
}
