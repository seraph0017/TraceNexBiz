// API envelope 类型 —— 与 packages/api-client/src/types.ts 保持一致
// 后续 W1f/W1g 接入 shared package 后此处可改为 re-export

export interface ApiEnvelope<T> {
  success: boolean;
  data: T | null;
  error: ApiError | null;
}

export interface ApiError {
  code: string;
  message_zh?: string;
  message_en?: string;
  trace_id: string;
  details?: Record<string, unknown>;
}

export class ApiException extends Error {
  public readonly code: string;
  public readonly traceId: string;
  public readonly details?: Record<string, unknown>;
  public readonly httpStatus: number;
  constructor(err: ApiError, httpStatus: number) {
    super(err.message_zh ?? err.message_en ?? err.code);
    this.code = err.code;
    this.traceId = err.trace_id;
    this.details = err.details;
    this.httpStatus = httpStatus;
  }
}
