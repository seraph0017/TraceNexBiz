import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 平台管理后台 (M4/M5/M6/M8/M10)；端口 5176
//
// per ADR-F1：独立 SPA on admin.tracenex.cn，不嵌入 Fy-api 既有 admin
const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  server: {
    port: 5176,
    strictPort: true,
    host: "0.0.0.0",
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true, secure: false },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: true,
    target: "es2022",
    cssCodeSplit: true,
    rollupOptions: {
      output: {
        manualChunks: {
          react: ["react", "react-dom", "react-router-dom"],
          query: ["@tanstack/react-query"],
          form: ["react-hook-form", "zod"],
          i18n: ["i18next", "react-i18next"],
          semi: ["@douyinfe/semi-ui"],
        },
      },
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    css: false,
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
    exclude: ["node_modules", "dist", "e2e"],
  },
});
