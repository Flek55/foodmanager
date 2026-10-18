// List operations that can be made offline.
//
// Each op is applied twice:
//   1. locally and immediately (applyOp), so the UI reacts with no network;
//   2. on the server later (sendOp), when the connection allows.
// applyOp mirrors the server's rules (see backend/internal/shopping), so
// after syncing, the server's answer looks the same as what the user saw.

import { api, isApiError, unwrap, type Schemas } from "../api/client";
import { searchable } from "../lib/units";

export type Snapshot = Schemas["ListSnapshot"];
type Item = Schemas["ListItem"];
type Unit = Schemas["Unit"];

export type Op =
  | {
      kind: "add";
      id: string;
      name: string;
      quantity?: number;
      unit?: Unit;
      note?: string;
      storeId?: string;
      categoryId?: string;
      at: string; // ISO time the user added it
      by?: string; // display name, shown until the server answers
    }
  | { kind: "update"; id: string; body: Schemas["UpdateItemRequest"] }
  | { kind: "check"; id: string; checked: boolean; at: string }
  | { kind: "delete"; id: string }
  | { kind: "clearChecked" };

const normalizeName = (s: string) => s.trim().replace(/\s+/g, " ");
const capitalize = (s: string) => s.charAt(0).toLocaleUpperCase("ru") + s.slice(1);

/** Finds what an "add" would hit: the existing product and its open item. */
export function findForAdd(s: Snapshot, name: string) {
  const key = searchable(normalizeName(name));
  const product = s.products.find((p) => searchable(p.name) === key);
  const active = product && s.items.find((i) => i.productId === product.id && !i.checkedAt);
  return { product, active };
}

export function applyOp(s: Snapshot, op: Op): Snapshot {
  switch (op.kind) {
    case "add": {
      let { products, items } = s;
      let { product, active } = findForAdd(s, op.name);
      if (!product) {
        // New product: its id is temporary until the server assigns one.
        product = { id: op.id, name: capitalize(normalizeName(op.name)), categoryId: op.categoryId, useCount: 1 };
        products = [...products, product];
      } else if (op.categoryId && op.categoryId !== product.categoryId) {
        product = { ...product, categoryId: op.categoryId };
        products = products.map((p) => (p.id === product!.id ? product! : p));
        items = items.map((i) => (i.productId === product!.id ? { ...i, categoryId: op.categoryId } : i));
      }
      const unit = op.quantity !== undefined ? (op.unit ?? "pcs") : undefined;
      if (active) {
        const merged: Item = {
          ...active,
          quantity: op.quantity ?? active.quantity,
          unit: op.quantity !== undefined ? unit : active.unit,
          note: op.note ?? active.note,
          storeId: op.storeId ?? active.storeId,
        };
        items = items.map((i) => (i.id === active!.id ? merged : i));
      } else {
        items = [
          ...items,
          {
            id: op.id,
            productId: product.id,
            name: product.name,
            description: product.description,
            categoryId: product.categoryId,
            quantity: op.quantity,
            unit,
            note: op.note,
            storeId: op.storeId,
            addedBy: op.by,
            createdAt: op.at,
          },
        ];
      }
      return { ...s, products, items };
    }

    case "update": {
      const item = s.items.find((i) => i.id === op.id);
      if (!item) return s;
      const b = op.body;
      const productFields = { name: normalizeName(b.name), description: b.description, categoryId: b.categoryId };
      return {
        ...s,
        products: s.products.map((p) => (p.id === item.productId ? { ...p, ...productFields } : p)),
        items: s.items.map((i) => {
          if (i.id === op.id) {
            const unit = b.quantity !== undefined ? (b.unit ?? "pcs") : undefined;
            return { ...i, ...productFields, quantity: b.quantity, unit, note: b.note, storeId: b.storeId };
          }
          return i.productId === item.productId ? { ...i, ...productFields } : i;
        }),
      };
    }

    case "check":
      return {
        ...s,
        items: s.items.map((i) =>
          i.id === op.id ? { ...i, checkedAt: op.checked ? op.at : undefined, checkedBy: undefined } : i,
        ),
      };

    case "delete":
      return { ...s, items: s.items.filter((i) => i.id !== op.id) };

    case "clearChecked":
      return { ...s, items: s.items.filter((i) => !i.checkedAt) };
  }
}

/** Performs the op on the server. Returns the server's version of the item, if any. */
export async function sendOp(householdId: string, op: Op): Promise<Item | null> {
  const path = { householdId };
  switch (op.kind) {
    case "add":
      return unwrap(
        api.POST("/households/{householdId}/items", {
          params: { path },
          body: {
            id: op.id,
            name: op.name,
            quantity: op.quantity,
            unit: op.unit,
            note: op.note,
            storeId: op.storeId,
            categoryId: op.categoryId,
          },
        }),
      );
    case "update":
      return unwrap(
        api.PUT("/households/{householdId}/items/{itemId}", {
          params: { path: { ...path, itemId: op.id } },
          body: op.body,
        }),
      );
    case "check":
      return unwrap(
        api.PUT("/households/{householdId}/items/{itemId}/checked", {
          params: { path: { ...path, itemId: op.id } },
          body: { checked: op.checked },
        }),
      );
    case "delete":
      try {
        await unwrap(api.DELETE("/households/{householdId}/items/{itemId}", { params: { path: { ...path, itemId: op.id } } }));
      } catch (err) {
        // Already gone (deleted by someone else, or our retried request
        // succeeded the first time): that's the outcome we wanted.
        if (!isApiError(err, 404)) throw err;
      }
      return null;
    case "clearChecked":
      await unwrap(api.POST("/households/{householdId}/items/clear-checked", { params: { path } }));
      return null;
  }
}

/** Folds the server's answer into the last known server snapshot. */
export function applyServerResult(s: Snapshot, op: Op, item: Item | null): Snapshot {
  if (op.kind === "delete") return { ...s, items: s.items.filter((i) => i.id !== op.id) };
  if (op.kind === "clearChecked") return { ...s, items: s.items.filter((i) => !i.checkedAt) };
  if (!item) return s;
  const exists = s.items.some((i) => i.id === item.id);
  return { ...s, items: exists ? s.items.map((i) => (i.id === item.id ? item : i)) : [...s.items, item] };
}

/**
 * The server may answer an "add" with a different item id: the product was
 * already on the list, so it merged into the existing item. Later queued
 * ops that refer to our temporary id must follow.
 */
export function remapId(outbox: Op[], from: string, to: string): Op[] {
  return outbox.map((op) => ("id" in op && op.id === from && op.kind !== "add" ? { ...op, id: to } : op));
}

type Delta = Schemas["SyncResponse"];

function mergeById<T extends { id: string }>(current: T[], changed: T[], deleted: string[]): T[] {
  const gone = new Set(deleted);
  const byId = new Map(changed.map((x) => [x.id, x]));
  const merged = current.filter((x) => !gone.has(x.id) && !byId.has(x.id));
  return [...merged, ...changed.filter((x) => !gone.has(x.id))];
}

const bySortOrder = <T extends { sortOrder: number; name: string }>(a: T, b: T) =>
  a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, "ru");

/** Applies a /sync delta to a snapshot. */
export function mergeDelta(s: Snapshot, d: Delta): Snapshot {
  return {
    version: d.version,
    items: mergeById(s.items, d.items, d.deleted.items).sort(
      (a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id),
    ),
    products: mergeById(s.products, d.products, d.deleted.products),
    categories: mergeById(s.categories, d.categories, d.deleted.categories).sort(bySortOrder),
    stores: mergeById(s.stores, d.stores, d.deleted.stores).sort(bySortOrder),
  };
}
