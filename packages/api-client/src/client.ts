// axios client + interceptors — frontend §5.2.
//
// W1e 实现 idempotency-key UUIDv7 / CSRF double-submit / trace_id 透传 / spec drift CI gate。
import axios, { AxiosError, AxiosInstance } from 'axios';
import { v4 as uuidv4 } from 'uuid';

const API_BASE = (import.meta as unknown as { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ?? '/';

export const apiClient: AxiosInstance = axios.create({
  baseURL: API_BASE,
  withCredentials: true, // 携带 httpOnly cookie tnbiz_access / tnbiz_csrf
  timeout: 15_000,
});

apiClient.interceptors.request.use((config) => {
  config.headers = config.headers ?? {};
  // trace_id（与 backend §12.4 同源）
  if (!config.headers['X-Oneapi-Request-Id']) {
    config.headers['X-Oneapi-Request-Id'] = uuidv4();
  }
  const method = (config.method ?? 'get').toLowerCase();
  if (['post', 'put', 'delete', 'patch'].includes(method)) {
    // CSRF double-submit (PRD §17.3)
    config.headers['X-Csrf-Token'] = readCookie('tnbiz_csrf') ?? '';
    if (!config.headers['Idempotency-Key']) {
      config.headers['Idempotency-Key'] = uuidv4();
    }
  }
  return config;
});

apiClient.interceptors.response.use(
  (res) => res,
  (err: AxiosError) => Promise.reject(err),
);

function readCookie(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined;
  const match = document.cookie.match(new RegExp('(^|; )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2] ?? '') : undefined;
}
