import createClient from "openapi-fetch";
import type { paths } from "./schema";

// Typed API client. `paths` is generated from api/openapi.yaml
// (`npm run gen:api`), so a wrong URL or field name is a compile error.
export const api = createClient<paths>({
  baseUrl: "/api",
  credentials: "include",
});
