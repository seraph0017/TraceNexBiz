// MIGRATED (Fix-D, 2026-05-12) — types now live in `@tnbiz/api-client`.
// This file is a thin re-export shim; new code should import from
// `@tnbiz/api-client` directly.
export type {
  ApiEnvelope,
  ApiError,
  PageMeta,
  PaginatedEnvelope,
} from "@tnbiz/api-client";
export { ApiException } from "@tnbiz/api-client";
