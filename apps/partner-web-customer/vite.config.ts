import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 终端客户后台（M2/M9/M13）；端口 5174
const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  server: {
    port: 5174,
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
