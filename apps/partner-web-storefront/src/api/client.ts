// TODO(Fix-D / 2026-05-12): MIGRATE TO `@tnbiz/api-client`.
//   This file is the simpler 88-line variant (no silent-refresh). After
//   migration this app gets silent-refresh "for free" via the canonical
//   client. Use partner-web-customer's `src/api/client.ts` re-export shim
//   as the reference. Steps:
//     1. Add `"@tnbiz/api-client": "workspace:*"` to package.json deps.
//     2. Replace this file's contents with:
//          export { apiClient, createApiClient, unwrap, genUUID } from "@tnbiz/api-client";
//          export type { AxiosRequestConfig } from "@tnbiz/api-client";
//     3. Replace `./types.ts` with a re-export shim against `@tnbiz/api-client`.
//     4. Run `pnpm typecheck && pnpm test && pnpm build` for this app.
//
// axios client + interceptors（frontend §5.2）
//
// 设计与 packages/api-client/src/client.ts 一致：
//   - withCredentials: 携带 httpOnly cookie tnbiz_access / tnbiz_csrf
//   - 自动注入 X-Oneapi-Request-Id（trace 透传，backend §12.4）
//   - 写操作自动注入 X-Csrf-Token（双提交，PRD §17.3）+ Idempotency-Key（UUID）
//   - 解包 envelope；error 抛 ApiException（含 code / trace_id）
import axios, { AxiosError } from "axios";
import type { AxiosInstance, AxiosRequestConfig } from "axios";
import { ApiException } from "./types";
import type { ApiEnvelope, ApiError } from "./types";

const API_BASE =
  (import.meta as unknown as { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ?? "/";

function genUUID(): string {
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

apiClient.interceptors.response.use(
  (res) => res,
  (err: AxiosError<ApiEnvelope<unknown>>) => {
    if (err.response?.data && typeof err.response.data === "object") {
      const body = err.response.data;
      if (body.error) {
        return Promise.reject(new ApiException(body.error, err.response.status));
      }
    }
    const fallback: ApiError = {
      code: "BIZ_NETWORK_ERROR",
      message_zh: "网络异常，请稍后重试",
      message_en: "Network error",
      trace_id: (err.config?.headers?.["X-Oneapi-Request-Id"] as string) ?? "",
    };
    return Promise.reject(new ApiException(fallback, err.response?.status ?? 0));
  },
);

/** 解包 envelope —— 成功返 data，失败抛 ApiException */
export async function unwrap<T>(promise: Promise<{ data: ApiEnvelope<T> }>): Promise<T> {
  const res = await promise;
  if (!res.data.success || res.data.data === null) {
    if (res.data.error) {
      throw new ApiException(res.data.error, 200);
    }
    throw new ApiException(
      { code: "BIZ_INVALID_ENVELOPE", trace_id: "" },
      200,
    );
  }
  return res.data.data;
}

export type { AxiosRequestConfig };
