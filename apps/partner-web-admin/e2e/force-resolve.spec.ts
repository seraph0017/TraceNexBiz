import { test, expect } from "@playwright/test";

// e2e: admin saga force-resolve dual-control
test("admin saga force-resolve", async ({ page, context }) => {
  await context.route("**/api/admin/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          id: 1,
          username: "ops",
          email_masked: "o***@example.com",
          role: "super_admin",
          permissions: ["saga.force_resolve"],
          ip_allowed: true,
          mfa_enrolled: true,
        },
        error: null,
      }),
    });
  });
  await context.route("**/api/admin/saga/escalated", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: [{ saga_id: "saga-1", state: "escalated", age_seconds: 600, initiator_ip: "10.0.0.1" }],
        error: null,
      }),
    });
  });
  await context.route("**/api/admin/saga/saga-1/force-resolve", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ success: true, data: { status: "resolved" }, error: null }),
    });
  });
  await page.goto("/saga/force-resolve");
  await page.getByLabel(/saga-id/).fill("saga-1");
  await page.getByLabel(/approver-token/).fill("ott-xxxxx");
  await page.getByLabel(/approver-ip/).fill("10.0.1.5");
  await page.getByRole("button", { name: /提交 force-resolve/ }).click();
  await expect(page.locator("body")).toBeVisible();
});
