// vitest 全局 setup —— 注入 jest-dom matcher 并 mock crypto.randomUUID（jsdom 24 已内置但兜底）
import "@testing-library/jest-dom/vitest";

if (typeof globalThis.crypto === "undefined") {
  // @ts-expect-error - jsdom 缺省时兜底
  globalThis.crypto = { randomUUID: () => "test-uuid-0000-0000-0000-000000000000" };
} else if (typeof globalThis.crypto.randomUUID !== "function") {
  globalThis.crypto.randomUUID = (() =>
    "test-uuid-0000-0000-0000-000000000000") as Crypto["randomUUID"];
}

// fetch / matchMedia 简易桩（Semi UI Layout / Modal 偶尔依赖）
if (typeof window !== "undefined" && !window.matchMedia) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
}
