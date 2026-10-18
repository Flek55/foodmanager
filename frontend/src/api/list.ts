// Hooks for the shopping list. The whole list screen is one cached
// "snapshot" (items + products + categories + stores) per household.
//
// Optimistic updates: for check/uncheck and delete we change the cached
// snapshot immediately, before the server answers, so the UI reacts
// instantly even on a slow mobile network. If the request fails, the
// previous snapshot is restored (onError).

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

type Snapshot = Schemas["ListSnapshot"];
type Item = Schemas["ListItem"];

export const listKey = (householdId: string) => ["list", householdId] as const;

export function useList(householdId: string) {
  return useQuery({
    queryKey: listKey(householdId),
    queryFn: () => unwrap(api.GET("/households/{householdId}/list", { params: { path: { householdId } } })),
  });
}

function patchSnapshot(qc: QueryClient, householdId: string, fn: (s: Snapshot) => Snapshot) {
  qc.setQueryData<Snapshot>(listKey(householdId), (s) => (s ? fn(s) : s));
}

function upsertItem(s: Snapshot, item: Item): Snapshot {
  const exists = s.items.some((i) => i.id === item.id);
  return { ...s, items: exists ? s.items.map((i) => (i.id === item.id ? item : i)) : [...s.items, item] };
}

/** Snapshot-changing mutation: apply locally first, roll back on failure. */
function useOptimistic<V, R>(
  householdId: string,
  mutationFn: (vars: V) => Promise<R>,
  apply: (s: Snapshot, vars: V) => Snapshot,
  onServer?: (s: Snapshot, result: R) => Snapshot,
) {
  const qc = useQueryClient();
  const key = listKey(householdId);
  return useMutation({
    mutationFn,
    onMutate: async (vars: V) => {
      await qc.cancelQueries({ queryKey: key }); // don't let an in-flight refetch overwrite us
      const prev = qc.getQueryData<Snapshot>(key);
      patchSnapshot(qc, householdId, (s) => apply(s, vars));
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev);
    },
    onSuccess: (result) => {
      if (onServer) patchSnapshot(qc, householdId, (s) => onServer(s, result));
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });
}

export function useAddItem(householdId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["AddItemRequest"]) => {
      const request = api.POST("/households/{householdId}/items", { params: { path: { householdId } }, body });
      const item = await unwrap(request);
      // 201 = new item, 200 = product was already on the list and got updated.
      return { item, created: (await request).response.status === 201 };
    },
    onSuccess: ({ item }) => {
      patchSnapshot(qc, householdId, (s) => upsertItem(s, item));
      return qc.invalidateQueries({ queryKey: listKey(householdId) });
    },
  });
}

export function useSetChecked(householdId: string) {
  return useOptimistic(
    householdId,
    ({ id, checked }: { id: string; checked: boolean }) =>
      unwrap(
        api.PUT("/households/{householdId}/items/{itemId}/checked", {
          params: { path: { householdId, itemId: id } },
          body: { checked },
        }),
      ),
    (s, { id, checked }) => ({
      ...s,
      items: s.items.map((i) =>
        i.id === id ? { ...i, checkedAt: checked ? new Date().toISOString() : undefined, checkedBy: undefined } : i,
      ),
    }),
    upsertItem,
  );
}

export function useDeleteItem(householdId: string) {
  return useOptimistic(
    householdId,
    (id: string) =>
      unwrap(api.DELETE("/households/{householdId}/items/{itemId}", { params: { path: { householdId, itemId: id } } })),
    (s, id) => ({ ...s, items: s.items.filter((i) => i.id !== id) }),
  );
}

export function useClearChecked(householdId: string) {
  return useOptimistic(
    householdId,
    () => unwrap(api.POST("/households/{householdId}/items/clear-checked", { params: { path: { householdId } } })),
    (s) => ({ ...s, items: s.items.filter((i) => !i.checkedAt) }),
  );
}

export function useUpdateItem(householdId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Schemas["UpdateItemRequest"] }) =>
      unwrap(
        api.PUT("/households/{householdId}/items/{itemId}", { params: { path: { householdId, itemId: id } }, body }),
      ),
    onSuccess: (item) => {
      patchSnapshot(qc, householdId, (s) => upsertItem(s, item));
      return qc.invalidateQueries({ queryKey: listKey(householdId) });
    },
  });
}

// ---- Stores & categories: rare edits, so just refetch afterwards ----

function useCatalogMutation<V, R>(householdId: string, mutationFn: (vars: V) => Promise<R>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => qc.invalidateQueries({ queryKey: listKey(householdId) }),
  });
}

export function useCreateStore(householdId: string) {
  return useCatalogMutation(householdId, (name: string) =>
    unwrap(api.POST("/households/{householdId}/stores", { params: { path: { householdId } }, body: { name } })),
  );
}

export function useRenameStore(householdId: string) {
  return useCatalogMutation(householdId, ({ id, name }: { id: string; name: string }) =>
    unwrap(
      api.PUT("/households/{householdId}/stores/{storeId}", {
        params: { path: { householdId, storeId: id } },
        body: { name },
      }),
    ),
  );
}

export function useDeleteStore(householdId: string) {
  return useCatalogMutation(householdId, (id: string) =>
    unwrap(api.DELETE("/households/{householdId}/stores/{storeId}", { params: { path: { householdId, storeId: id } } })),
  );
}

export function useCreateCategory(householdId: string) {
  return useCatalogMutation(householdId, (body: Schemas["CategoryInput"]) =>
    unwrap(api.POST("/households/{householdId}/categories", { params: { path: { householdId } }, body })),
  );
}

export function useUpdateCategory(householdId: string) {
  return useCatalogMutation(householdId, ({ id, body }: { id: string; body: Schemas["CategoryInput"] }) =>
    unwrap(
      api.PUT("/households/{householdId}/categories/{categoryId}", {
        params: { path: { householdId, categoryId: id } },
        body,
      }),
    ),
  );
}

export function useDeleteCategory(householdId: string) {
  return useCatalogMutation(householdId, (id: string) =>
    unwrap(
      api.DELETE("/households/{householdId}/categories/{categoryId}", {
        params: { path: { householdId, categoryId: id } },
      }),
    ),
  );
}

export function useReorderCategories(householdId: string) {
  return useOptimistic(
    householdId,
    (ids: string[]) =>
      unwrap(api.PUT("/households/{householdId}/categories/order", { params: { path: { householdId } }, body: { ids } })),
    (s, ids) => ({
      ...s,
      categories: ids
        .map((id, i) => {
          const c = s.categories.find((c) => c.id === id);
          return c && { ...c, sortOrder: i + 1 };
        })
        .filter((c): c is Schemas["Category"] => !!c),
    }),
  );
}
