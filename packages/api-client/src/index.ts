// Canonical TNBIZ frontend API client package.
//
// Public API:
//   - createApiClient({baseURL, getAuthToken, onAuthError, refreshPath, timeout})
//   - apiClient                  (default singleton)
//   - unwrap<T>(promise)         → T  (throws ApiException on failure)
//   - genUUID()                  RFC4122 v4
//   - propagateTrace()           inject X-Oneapi-Request-Id
//   - mapApiError(err)           → {i18nKey, severity}
//   - ApiEnvelope<T> / ApiError / ApiException / PageMeta / PaginatedEnvelope
//
// W1e: codegen targets are added by orval/openapi-typescript against
// apps/partner-api/openapi/internal-api.yaml. This package keeps the runtime
// client + interceptors stable so generated specs only contribute types.
export { createApiClient, apiClient, unwrap, genUUID } from "./client";
export type { CreateApiClientOptions, AxiosRequestConfig } from "./client";
export { mapApiError } from "./error-mapping";
export type { ToastSpec } from "./error-mapping";
export { propagateTrace } from "./trace";
export {
  ApiException,
} from "./envelope";
export type {
  ApiEnvelope,
  ApiError,
  PageMeta,
  PaginatedEnvelope,
} from "./envelope";
