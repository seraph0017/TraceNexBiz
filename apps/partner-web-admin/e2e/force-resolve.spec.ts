import { test, expect } from "@playwright/test";

// e2e: admin saga force-resolve dual-control (two-step UI)
// 步骤：选择 saga → click Approve (issue 单次 approver_token) → submit force-resolve
// approver_ip 由服务端从 c.ClientIP() 解析；UI 不再上送 (Fix-D step 3)
test("admin saga force-resolve two-step", async ({ page, context }) => {
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
  // Step 1: Approve issues approver_token
  await context.route("**/api/admin/saga/saga-1/approver-token", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: { token: "ott-xxxxx", expires_at: Math.floor(Date.now() / 1000) + 300 },
        error: null,
      }),
    });
  });
  // Step 2: submit force-resolve
  await context.route("**/api/admin/saga/saga-1/force-resolve", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ success: true, data: { status: "resolved" }, error: null }),
    });
  });
  await page.goto("/saga/force-resolve");
  await page.getByLabel("saga-id").fill("saga-1");
  // Click Approve to issue an approver_token
  await page.getByLabel("issue-approver-token").click();
  // approver_token auto-populated by mutation onSuccess
  await page.getByLabel("approver-token").fill("ott-xxxxx");
  await page.getByLabel("reason").fill("manual reconciliation");
  await page.getByRole("button", { name: /提交 force-resolve/ }).click();
  await expect(page.locator("body")).toBeVisible();
});
