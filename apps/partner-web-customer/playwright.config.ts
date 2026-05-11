import { defineConfig, devices } from "@playwright/test";

// Playwright config —— 与 storefront 同模式；W2 review 时引 @playwright/test 后开跑
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  reporter: "list",
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:5174",
    trace: "on-first-retry",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
});
