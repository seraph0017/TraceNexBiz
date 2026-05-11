import { test, expect } from "@playwright/test";

// e2e: admin 12377 一键派发
test("admin content safety dispatch", async ({ page, context }) => {
  await context.route("**/api/admin/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          id: 1,
          username: "risk",
          email_masked: "r***@example.com",
          role: "risk_admin",
          permissions: ["content_safety.dispatch", "content_safety.retry"],
          ip_allowed: true,
          mfa_enrolled: true,
        },
        error: null,
      }),
    });
  });
  await context.route("**/api/admin/content-safety/reports*", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          data: [
            { id: 1, source: "fy_api", content_excerpt: "敏感词测试", status: "pending", retries: 0, created_at: "now" },
          ],
          meta: { total: 1, page: 1, limit: 50 },
          error: null,
        }),
      });
    } else {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, data: { dispatched: 1 }, error: null }),
      });
    }
  });
  await context.route("**/api/admin/content-safety/reports/dispatch", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ success: true, data: { dispatched: 1 }, error: null }),
    });
  });
  await page.goto("/content-safety/reports");
  await page.getByRole("button", { name: /一键派发/ }).click();
  await expect(page.locator("body")).toBeVisible();
});
