// MIGRATED (Fix-D, 2026-05-12) — this file is now a thin re-export shim over
// the canonical `@tnbiz/api-client` package. New code MUST import from
// `@tnbiz/api-client` directly. This file remains only so existing intra-app
// imports (`./client`, `@/api/client`) keep working without a 100-file churn.
export {
  apiClient,
  createApiClient,
  unwrap,
  genUUID,
} from "@tnbiz/api-client";
export type { AxiosRequestConfig } from "@tnbiz/api-client";
