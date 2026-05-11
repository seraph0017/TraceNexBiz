// Error envelope 类型（与 integration §8.1 / overview §11.3 一致）.
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
