import { test, expect } from "@playwright/test";

// e2e: PIPL 数据导出（场景 Q）
test("customer pipl export request", async ({ page, context }) => {
  await context.route("**/api/customer/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          id: 1,
          fy_user_id: 100,
          display_name: "Bob",
          email_masked: "b***@example.com",
          phone_masked: "138****5678",
          status: "active",
          partner_id: 10,
          partner_name: "ABC",
          partner_terminated_at: null,
          kyc_status: "approved",
          consent_pipl_signed: true,
        },
        error: null,
      }),
    });
  });
  await context.route("**/api/customer/pipl", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          data: { id: 1, kind: "export", status: "submitted", created_at: "now" },
          error: null,
        }),
      });
    } else {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, data: [], error: null }),
      });
    }
  });
  await page.goto("/pipl-rights");
  await page.getByRole("button", { name: /导出我的数据/ }).click();
  await expect(page).toHaveURL(/pipl-rights/);
});
