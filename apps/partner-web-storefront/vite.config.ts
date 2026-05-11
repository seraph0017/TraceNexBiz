import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 公开商城（partner.tracenex.cn 公共页 + 招商落地）；端口 5173
// SEO 策略：SPA + 静态 meta + sitemap.xml；vite-plugin-ssr 暂不引入（节省依赖体积，frontend §1.3 待评估）
const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    host: "0.0.0.0",
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        secure: false,
      },
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
          // 注：storefront 当前不直接 import Semi UI 组件；W1f/W1g 使用 Semi 时再加 'semi' chunk
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
