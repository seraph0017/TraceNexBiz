// Canonical envelope types — single source of truth for all TNBIZ frontends.
// Aligned with backend integration §8.1 / overview §11.3.
//
// Shape:
//   success === true  → data is non-null, error is null
//   success === false → data is null, error is non-null

export interface ApiError {
  code: string;
  message_zh?: string;
  message_en?: string;
  trace_id: string;
  details?: Record<string, unknown>;
}

export interface ApiEnvelope<T> {
  success: boolean;
  data: T | null;
  error: ApiError | null;
}

export interface PageMeta {
  total: number;
  page: number;
  limit: number;
}

export interface PaginatedEnvelope<T> {
  success: boolean;
  data: T[] | null;
  error: ApiError | null;
  meta?: PageMeta;
}

/**
 * Thrown by `unwrap()` or the response interceptor when the server returns a
 * non-success envelope or a transport error. Always carries `code` + `traceId`
 * so the toast layer can render an i18n message and the operator can grep
 * logs.
 */
export class ApiException extends Error {
  public readonly code: string;
  public readonly traceId: string;
  public readonly details?: Record<string, unknown>;
  public readonly httpStatus: number;
  constructor(err: ApiError, httpStatus: number) {
    super(err.message_zh ?? err.message_en ?? err.code);
    this.name = "ApiException";
    this.code = err.code;
    this.traceId = err.trace_id;
    this.details = err.details;
    this.httpStatus = httpStatus;
  }
}
