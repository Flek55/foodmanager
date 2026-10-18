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
type Product = Schemas["Product"];
type Unit = Schemas["Unit"];
export type StockStatus = Schemas["StockStatus"];

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
  | { kind: "clearChecked" }
  // Home stock. itemId: id for the list item that low/out may create.
  | { kind: "stock"; productId: string; status: StockStatus | "untracked"; itemId: string; at: string; by?: string }
  // Start tracking by name; productId is used if the product is new.
  | { kind: "track"; productId: string; name: string; status: StockStatus; itemId: string; at: string; by?: string };

/** What the server answered for an op: the item and/or product it touched. */
export type ServerResult = { item?: Item | null; product?: Product };

const normalizeName = (s: string) => s.trim().replace(/\s+/g, " ");
const capitalize = (s: string) => s.charAt(0).toLocaleUpperCase("ru") + s.slice(1);
const replaceById = <T extends { id: string }>(list: T[], x: T) => list.map((y) => (y.id === x.id ? x : y));
const upsertById = <T extends { id: string }>(list: T[], x: T) =>
  list.some((y) => y.id === x.id) ? replaceById(list, x) : [...list, x];

export function findProduct(s: Snapshot, name: string) {
  const key = searchable(normalizeName(name));
  return s.products.find((p) => searchable(p.name) === key);
}

export const openItemOf = (s: Snapshot, productId: string) =>
  s.items.find((i) => i.productId === productId && !i.checkedAt);

/** Finds what an "add" would hit: the existing product and its open item. */
export function findForAdd(s: Snapshot, name: string) {
  const product = findProduct(s, name);
  return { product, active: product && openItemOf(s, product.id) };
}

function newProduct(id: string, name: string, categoryId?: string): Product {
  return { id, name: capitalize(normalizeName(name)), categoryId, useCount: 0, purchaseCount: 0 };
}

function itemFor(product: Product, id: string, at: string, by?: string): Item {
  return {
    id,
    productId: product.id,
    name: product.name,
    description: product.description,
    categoryId: product.categoryId,
    addedBy: by,
    createdAt: at,
  };
}

/** Stock change; low/out also puts the product on the list if it isn't. */
function applyStock(
  s: Snapshot,
  productId: string,
  status: StockStatus | "untracked",
  itemId: string,
  at: string,
  by?: string,
): Snapshot {
  const product = s.products.find((p) => p.id === productId);
  if (!product) return s;
  let updated: Product = { ...product, stockStatus: status === "untracked" ? undefined : status, stockUpdatedAt: at };
  let items = s.items;
  if ((status === "low" || status === "out") && !openItemOf(s, productId)) {
    updated = { ...updated, useCount: updated.useCount + 1 };
    items = [...items, itemFor(updated, itemId, at, by)];
  }
  return { ...s, products: replaceById(s.products, updated), items };
}

/** A check-off is a purchase: counters move, a tracked product is restocked. */
function applyPurchase(s: Snapshot, item: Item, checked: boolean, at: string): Snapshot {
  const wasChecked = !!item.checkedAt;
  if (wasChecked === checked) return s;
  return {
    ...s,
    products: s.products.map((p) => {
      if (p.id !== item.productId) return p;
      if (!checked) return { ...p, purchaseCount: Math.max(0, p.purchaseCount - 1) };
      const restock = p.stockStatus !== undefined && p.stockStatus !== "ok";
      return {
        ...p,
        purchaseCount: p.purchaseCount + 1,
        lastPurchasedAt: at,
        ...(restock ? { stockStatus: "ok" as const, stockUpdatedAt: at } : {}),
      };
    }),
  };
}

export function applyOp(s: Snapshot, op: Op): Snapshot {
  switch (op.kind) {
    case "add": {
      let { products, items } = s;
      let { product, active } = findForAdd(s, op.name);
      if (!product) {
        // New product: its id is temporary until the server assigns one.
        product = { ...newProduct(op.id, op.name, op.categoryId), useCount: 1 };
        products = [...products, product];
      } else if (op.categoryId && op.categoryId !== product.categoryId) {
        product = { ...product, categoryId: op.categoryId };
        products = replaceById(products, product);
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
        items = replaceById(items, merged);
      } else {
        items = [...items, { ...itemFor(product, op.id, op.at, op.by), quantity: op.quantity, unit, note: op.note, storeId: op.storeId }];
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

    case "check": {
      const item = s.items.find((i) => i.id === op.id);
      if (!item) return s;
      const withPurchase = applyPurchase(s, item, op.checked, op.at);
      return {
        ...withPurchase,
        items: s.items.map((i) =>
          i.id === op.id ? { ...i, checkedAt: op.checked ? op.at : undefined, checkedBy: undefined } : i,
        ),
      };
    }

    case "delete":
      return { ...s, items: s.items.filter((i) => i.id !== op.id) };

    case "clearChecked":
      return { ...s, items: s.items.filter((i) => !i.checkedAt) };

    case "stock":
      return applyStock(s, op.productId, op.status, op.itemId, op.at, op.by);

    case "track": {
      let next = s;
      let product = findProduct(s, op.name);
      if (!product) {
        product = newProduct(op.productId, op.name);
        next = { ...s, products: [...s.products, product] };
      }
      return applyStock(next, product.id, op.status, op.itemId, op.at, op.by);
    }
  }
}

/** Performs the op on the server. */
export async function sendOp(householdId: string, op: Op): Promise<ServerResult> {
  const path = { householdId };
  switch (op.kind) {
    case "add": {
      const body = {
        id: op.id,
        name: op.name,
        quantity: op.quantity,
        unit: op.unit,
        note: op.note,
        storeId: op.storeId,
        categoryId: op.categoryId,
      };
      return { item: await unwrap(api.POST("/households/{householdId}/items", { params: { path }, body })) };
    }
    case "update":
      return {
        item: await unwrap(
          api.PUT("/households/{householdId}/items/{itemId}", { params: { path: { ...path, itemId: op.id } }, body: op.body }),
        ),
      };
    case "check":
      return {
        item: await unwrap(
          api.PUT("/households/{householdId}/items/{itemId}/checked", {
            params: { path: { ...path, itemId: op.id } },
            body: { checked: op.checked },
          }),
        ),
      };
    case "delete":
      try {
        await unwrap(api.DELETE("/households/{householdId}/items/{itemId}", { params: { path: { ...path, itemId: op.id } } }));
      } catch (err) {
        // Already gone (deleted by someone else, or our retried request
        // succeeded the first time): that's the outcome we wanted.
        if (!isApiError(err, 404)) throw err;
      }
      return {};
    case "clearChecked":
      await unwrap(api.POST("/households/{householdId}/items/clear-checked", { params: { path } }));
      return {};
    case "stock":
      return unwrap(
        api.PUT("/households/{householdId}/products/{productId}/stock", {
          params: { path: { ...path, productId: op.productId } },
          body: { status: op.status, itemId: op.itemId },
        }),
      );
    case "track":
      return unwrap(
        api.POST("/households/{householdId}/stock", {
          params: { path },
          body: { name: op.name, status: op.status, productId: op.productId, itemId: op.itemId },
        }),
      );
  }
}

/** Folds the server's answer into the last known server snapshot. */
export function applyServerResult(s: Snapshot, op: Op, res: ServerResult): Snapshot {
  if (op.kind === "delete") return { ...s, items: s.items.filter((i) => i.id !== op.id) };
  if (op.kind === "clearChecked") return { ...s, items: s.items.filter((i) => !i.checkedAt) };
  let next = s;
  // A check-off also changed the product (counters, stock) on the server,
  // but only the item comes back: apply the product side locally so the
  // UI doesn't flicker until the next pull brings the real product.
  if (op.kind === "check") {
    const before = s.items.find((i) => i.id === op.id);
    if (before) next = applyPurchase(next, before, op.checked, op.at);
  }
  if (res.product) next = { ...next, products: upsertById(next.products, res.product) };
  if (res.item) next = { ...next, items: upsertById(next.items, res.item) };
  return next;
}

/**
 * The server may answer with different ids than the client proposed:
 *  - "add" merged into an item already on the list;
 *  - "stock"/"track" found the product already on the list (other item id);
 *  - "track" found an existing product with that name (other product id).
 * Later queued ops that refer to our temporary ids must follow.
 */
export function remapAfter(op: Op, res: ServerResult, outbox: Op[]): Op[] {
  const itemFrom = op.kind === "add" ? op.id : op.kind === "stock" || op.kind === "track" ? op.itemId : undefined;
  const itemTo = res.item?.id;
  const productFrom = op.kind === "track" ? op.productId : undefined;
  const productTo = res.product?.id;

  return outbox.map((next) => {
    let o = next;
    if (itemFrom && itemTo && itemFrom !== itemTo && "id" in o && o.id === itemFrom && o.kind !== "add") {
      o = { ...o, id: itemTo };
    }
    if (productFrom && productTo && productFrom !== productTo && "productId" in o && o.productId === productFrom) {
      o = { ...o, productId: productTo };
    }
    return o;
  });
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
