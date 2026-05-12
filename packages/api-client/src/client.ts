// Canonical axios client factory for all TNBIZ frontends.
//
// Behaviour (frontend §5.2 / §6 / backend §12.4 / PRD §17.3):
//   - withCredentials: 携带 httpOnly cookie tnbiz_access / tnbiz_csrf
//   - 自动注入 X-Oneapi-Request-Id（trace 透传）
//   - 写操作自动注入 X-Csrf-Token（cookie 双提交）+ Idempotency-Key (UUID)
//   - 401 silent refresh 一次 → 失败则 onAuthError() + 抛 ApiException
//   - response error → ApiException(code, trace_id)
//
// Per-app config knobs:
//   - baseURL         (default: import.meta.env.VITE_API_BASE || "/")
//   - getAuthToken    (optional, for non-cookie bearer-token flows)
//   - onAuthError     (callback on 401-after-refresh-failed; default = dispatch
//                      "tnbiz:auth:expired" CustomEvent on window)
//   - refreshPath     (default: "auth/refresh"; resolved against baseURL)
//   - timeout         (default 15s)

import axios from "axios";
import type {
  AxiosError,
  AxiosInstance,
  AxiosRequestConfig,
  InternalAxiosRequestConfig,
} from "axios";
import { ApiException, type ApiEnvelope, type ApiError } from "./envelope";

export interface CreateApiClientOptions {
  baseURL?: string;
  getAuthToken?: () => string | undefined;
  onAuthError?: () => void;
  refreshPath?: string;
  timeout?: number;
}

export function genUUID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // RFC4122 v4 fallback
  const r = (n: number): string =>
    Array.from({ length: n }, () => Math.floor(Math.random() * 16).toString(16)).join("");
  return `${r(8)}-${r(4)}-4${r(3)}-${(8 + Math.floor(Math.random() * 4)).toString(16)}${r(3)}-${r(12)}`;
}

function readCookie(name: string): string | undefined {
  if (typeof document === "undefined") return undefined;
  const m = document.cookie.match(new RegExp("(^|; )" + name + "=([^;]+)"));
  return m && m[2] ? decodeURIComponent(m[2]) : undefined;
}

function defaultBaseURL(): string {
  return (
    (import.meta as unknown as { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ??
    "/"
  );
}

function defaultOnAuthError(): void {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new CustomEvent("tnbiz:auth:expired"));
  }
}

export function createApiClient(opts: CreateApiClientOptions = {}): AxiosInstance {
  const baseURL = opts.baseURL ?? defaultBaseURL();
  const refreshPath = opts.refreshPath ?? "auth/refresh";
  const timeout = opts.timeout ?? 15_000;
  const onAuthError = opts.onAuthError ?? defaultOnAuthError;

  const instance: AxiosInstance = axios.create({
    baseURL,
    withCredentials: true,
    timeout,
  });

  instance.interceptors.request.use((config: InternalAxiosRequestConfig) => {
    config.headers = config.headers ?? {};
    if (!config.headers["X-Oneapi-Request-Id"]) {
      config.headers["X-Oneapi-Request-Id"] = genUUID();
    }
    if (opts.getAuthToken) {
      const tok = opts.getAuthToken();
      if (tok && !config.headers["Authorization"]) {
        config.headers["Authorization"] = `Bearer ${tok}`;
      }
    }
    const method = (config.method ?? "get").toLowerCase();
    if (["post", "put", "delete", "patch"].includes(method)) {
      const csrf = readCookie("tnbiz_csrf");
      if (csrf) config.headers["X-Csrf-Token"] = csrf;
      if (!config.headers["Idempotency-Key"]) {
        config.headers["Idempotency-Key"] = genUUID();
      }
    }
    return config;
  });

  // single-flight silent refresh
  let refreshing: Promise<void> | null = null;
  const trySilentRefresh = (): Promise<void> => {
    if (refreshing) return refreshing;
    refreshing = (async () => {
      try {
        await axios.post(
          `${baseURL}${refreshPath}`.replace(/([^:])\/\//g, "$1/"),
          {},
          { withCredentials: true, timeout: 10_000 },
        );
      } finally {
        refreshing = null;
      }
    })();
    return refreshing;
  };

  instance.interceptors.response.use(
    (res) => res,
    async (err: AxiosError<ApiEnvelope<unknown>>) => {
      const status = err.response?.status ?? 0;
      const cfg = err.config as (AxiosRequestConfig & { __retried?: boolean }) | undefined;
      if (status === 401 && cfg && !cfg.__retried) {
        try {
          await trySilentRefresh();
          cfg.__retried = true;
          return instance.request(cfg);
        } catch {
          onAuthError();
        }
      }
      if (err.response?.data && typeof err.response.data === "object") {
        const body = err.response.data;
        if (body.error) {
          return Promise.reject(new ApiException(body.error, status));
        }
      }
      const fallback: ApiError = {
        code: "BIZ_NETWORK_ERROR",
        message_zh: "网络异常，请稍后重试",
        message_en: "Network error",
        trace_id: (err.config?.headers?.["X-Oneapi-Request-Id"] as string) ?? "",
      };
      return Promise.reject(new ApiException(fallback, status));
    },
  );

  return instance;
}

/** Default singleton — use only if you don't need per-app config. */
export const apiClient: AxiosInstance = createApiClient();

/** Unwrap an envelope response: returns data on success, throws ApiException on failure. */
export async function unwrap<T>(promise: Promise<{ data: ApiEnvelope<T> }>): Promise<T> {
  const res = await promise;
  if (!res.data.success || res.data.data === null) {
    if (res.data.error) throw new ApiException(res.data.error, 200);
    throw new ApiException({ code: "BIZ_INVALID_ENVELOPE", trace_id: "" }, 200);
  }
  return res.data.data;
}

export type { AxiosRequestConfig };
