// Store & category management. These are rare settings edits, so unlike
// list items (see src/sync) they need a connection; on success we just ask
// the household's sync engine to pull the change.

import { useMutation } from "@tanstack/react-query";
import { getListSync } from "../sync/engine";
import { api, unwrap, type Schemas } from "./client";

function useCatalogMutation<V, R>(householdId: string, mutationFn: (vars: V) => Promise<R>) {
  return useMutation({
    mutationFn,
    onSuccess: () => getListSync(householdId).sync(),
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
  return useCatalogMutation(householdId, (ids: string[]) =>
    unwrap(api.PUT("/households/{householdId}/categories/order", { params: { path: { householdId } }, body: { ids } })),
  );
}
