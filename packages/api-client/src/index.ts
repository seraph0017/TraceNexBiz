// W0 scaffold — partner-api TypeScript SDK 占位。
// W1e: 用 orval / openapi-typescript 从 apps/partner-api/openapi/internal-api.yaml 自动生成 generated/*.ts；
// 本文件只导出 client + interceptors 骨架（frontend §5.2）.
export { apiClient } from './client';
export { mapApiError } from './error-mapping';
export { propagateTrace } from './trace';
export type { ApiEnvelope, ApiError } from './types';
