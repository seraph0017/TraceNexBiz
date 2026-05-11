// vitest setup
import "@testing-library/jest-dom/vitest";

if (typeof globalThis.crypto === "undefined") {
  // @ts-expect-error fallback
  globalThis.crypto = { randomUUID: () => "test-uuid-0000-0000-0000-000000000000" };
} else if (typeof globalThis.crypto.randomUUID !== "function") {
  globalThis.crypto.randomUUID = (() =>
    "test-uuid-0000-0000-0000-000000000000") as Crypto["randomUUID"];
}

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

if (typeof window !== "undefined" && !window.ResizeObserver) {
  window.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  } as unknown as typeof ResizeObserver;
}
