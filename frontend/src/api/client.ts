import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

// Typed API client. `paths` is generated from api/openapi.yaml
// (`npm run gen:api`), so a wrong URL or field name is a compile error.
export const api = createClient<paths>({
  baseUrl: "/api",
  credentials: "include",
});

export type Schemas = components["schemas"];

/** Error thrown by `unwrap` for any non-2xx answer or network failure. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    /** Request field the error refers to, for showing it under an input. */
    readonly field?: string,
  ) {
    super(message);
  }
}

type FetchResult<T> = { data?: T; error?: unknown; response: Response };

/**
 * Turns openapi-fetch's `{ data, error }` result into "return data or throw",
 * which is what TanStack Query expects from query/mutation functions.
 */
export async function unwrap<T>(request: Promise<FetchResult<T>>): Promise<T> {
  let result: FetchResult<T>;
  try {
    result = await request;
  } catch {
    throw new ApiError(0, "network", "Нет связи с сервером. Проверьте интернет.");
  }
  const { data, error, response } = result;
  if (!response.ok) {
    const body = (error ?? {}) as Partial<Schemas["Error"]>;
    throw new ApiError(
      response.status,
      body.code ?? "unknown",
      body.message ?? `Ошибка сервера (${response.status})`,
      body.field,
    );
  }
  return data as T;
}

export function isApiError(err: unknown, status?: number): err is ApiError {
  return err instanceof ApiError && (status === undefined || err.status === status);
}
