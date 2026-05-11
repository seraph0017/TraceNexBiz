// Playwright config —— 占位；CI 引入 @playwright/test 后启用
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  retries: 1,
  use: {
    baseURL: "http://localhost:5173",
    headless: true,
  },
  webServer: {
    command: "pnpm dev",
    port: 5173,
    reuseExistingServer: true,
    timeout: 60_000,
  },
});
