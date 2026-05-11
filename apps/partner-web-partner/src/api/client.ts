// axios client + interceptors（frontend §5.2 / §6）
//   - withCredentials: 携带 httpOnly cookie tnbiz_access / tnbiz_refresh
//   - 自动注入 X-Oneapi-Request-Id（trace 透传）
//   - 写操作自动注入 X-Csrf-Token（cookie 双提交）+ Idempotency-Key（UUIDv4）
//   - 401/403 全局处理：401 silent refresh 一次后再失败 → 触发 logout 事件
import axios, { AxiosError } from "axios";
import type { AxiosInstance, AxiosRequestConfig } from "axios";
import { ApiException } from "./types";
import type { ApiEnvelope, ApiError } from "./types";

const API_BASE =
  (import.meta as unknown as { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ?? "/";

export function genUUID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  const r = (n: number): string =>
    Array.from({ length: n }, () => Math.floor(Math.random() * 16).toString(16)).join("");
  return `${r(8)}-${r(4)}-4${r(3)}-${(8 + Math.floor(Math.random() * 4)).toString(16)}${r(3)}-${r(12)}`;
}

function readCookie(name: string): string | undefined {
  if (typeof document === "undefined") return undefined;
  const m = document.cookie.match(new RegExp("(^|; )" + name + "=([^;]+)"));
  return m && m[2] ? decodeURIComponent(m[2]) : undefined;
}

export const apiClient: AxiosInstance = axios.create({
  baseURL: API_BASE,
  withCredentials: true,
  timeout: 15_000,
});

apiClient.interceptors.request.use((config) => {
  config.headers = config.headers ?? {};
  if (!config.headers["X-Oneapi-Request-Id"]) {
    config.headers["X-Oneapi-Request-Id"] = genUUID();
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

// silent refresh: 401 触发一次 /auth/refresh，成功后 retry，失败抛 logout 事件
let refreshing: Promise<void> | null = null;
async function trySilentRefresh(): Promise<void> {
  if (refreshing) return refreshing;
  refreshing = (async () => {
    try {
      await axios.post(
        `${API_BASE}auth/refresh`.replace("//", "/"),
        {},
        { withCredentials: true, timeout: 10_000 },
      );
    } finally {
      refreshing = null;
    }
  })();
  return refreshing;
}

apiClient.interceptors.response.use(
  (res) => res,
  async (err: AxiosError<ApiEnvelope<unknown>>) => {
    const status = err.response?.status ?? 0;
    const cfg = err.config as (AxiosRequestConfig & { __retried?: boolean }) | undefined;
    if (status === 401 && cfg && !cfg.__retried) {
      try {
        await trySilentRefresh();
        cfg.__retried = true;
        return apiClient.request(cfg);
      } catch {
        if (typeof window !== "undefined") {
          window.dispatchEvent(new CustomEvent("tnbiz:auth:expired"));
        }
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

/** 解包 envelope —— 成功返 data，失败抛 ApiException */
export async function unwrap<T>(promise: Promise<{ data: ApiEnvelope<T> }>): Promise<T> {
  const res = await promise;
  if (!res.data.success || res.data.data === null) {
    if (res.data.error) throw new ApiException(res.data.error, 200);
    throw new ApiException({ code: "BIZ_INVALID_ENVELOPE", trace_id: "" }, 200);
  }
  return res.data.data;
}

export type { AxiosRequestConfig };
