// Data hooks: every server interaction the UI does goes through here.
//
// TanStack Query in two sentences: `useQuery` fetches and caches data under
// a key (["me"], ["household", id]) and re-renders components when it
// changes; `useMutation` runs a write and, on success, we update or
// invalidate the cached keys so every screen shows fresh data.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, isApiError, unwrap, type Schemas } from "./client";

export const keys = {
  me: ["me"] as const,
  household: (id: string) => ["household", id] as const,
  invites: (id: string) => ["household", id, "invites"] as const,
  invitePreview: (code: string) => ["invite", code] as const,
};

// ---- Account ----

/** Current user, or `null` when not logged in (401 is not an error here). */
export function useMe() {
  return useQuery({
    queryKey: keys.me,
    queryFn: async () => {
      try {
        return await unwrap(api.GET("/me"));
      } catch (err) {
        if (isApiError(err, 401)) return null;
        throw err;
      }
    },
    staleTime: 60_000,
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["LoginRequest"]) => unwrap(api.POST("/auth/login", { body })),
    onSuccess: (me) => qc.setQueryData(keys.me, me),
  });
}

export function useRegister() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["RegisterRequest"]) => unwrap(api.POST("/auth/register", { body })),
    onSuccess: (me) => qc.setQueryData(keys.me, me),
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(api.POST("/auth/logout")),
    onSettled: () => {
      // Drop everything cached for the previous user.
      qc.clear();
      qc.setQueryData(keys.me, null);
    },
  });
}

// ---- Households ----

export function useHousehold(id: string | undefined) {
  return useQuery({
    queryKey: keys.household(id ?? ""),
    queryFn: () => unwrap(api.GET("/households/{householdId}", { params: { path: { householdId: id! } } })),
    enabled: !!id,
  });
}

export function useCreateHousehold() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => unwrap(api.POST("/households", { body: { name } })),
    onSuccess: (h) => {
      qc.setQueryData(keys.household(h.id), h);
      return qc.invalidateQueries({ queryKey: keys.me });
    },
  });
}

export function useRenameHousehold(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      unwrap(api.PATCH("/households/{householdId}", { params: { path: { householdId: id } }, body: { name } })),
    onSuccess: (h) => {
      qc.setQueryData(keys.household(id), h);
      return qc.invalidateQueries({ queryKey: keys.me });
    },
  });
}

export function useRemoveMember(householdId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      unwrap(
        api.DELETE("/households/{householdId}/members/{userId}", {
          params: { path: { householdId, userId } },
        }),
      ),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: keys.household(householdId) }),
        qc.invalidateQueries({ queryKey: keys.me }),
      ]),
  });
}

// ---- Invites ----

export function useInvites(householdId: string) {
  return useQuery({
    queryKey: keys.invites(householdId),
    queryFn: () =>
      unwrap(api.GET("/households/{householdId}/invites", { params: { path: { householdId } } })),
  });
}

export function useCreateInvite(householdId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["InviteInput"] = {}) =>
      unwrap(api.POST("/households/{householdId}/invites", { params: { path: { householdId } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.invites(householdId) }),
  });
}

export function useRevokeInvite(householdId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (code: string) =>
      unwrap(
        api.DELETE("/households/{householdId}/invites/{code}", {
          params: { path: { householdId, code } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.invites(householdId) }),
  });
}

export function useInvitePreview(code: string) {
  return useQuery({
    queryKey: keys.invitePreview(code),
    queryFn: () => unwrap(api.GET("/invites/{code}", { params: { path: { code } } })),
    retry: false,
  });
}

export function useAcceptInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (code: string) => unwrap(api.POST("/invites/{code}/accept", { params: { path: { code } } })),
    onSuccess: (h) => {
      qc.setQueryData(keys.household(h.id), h);
      return qc.invalidateQueries({ queryKey: keys.me });
    },
  });
}
