import { test, expect } from "@playwright/test";

// e2e: 客户提交工单
test("customer create ticket", async ({ page, context }) => {
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
  await context.route("**/api/customer/tickets", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          data: { id: 1, subject: "Test", status: "open", priority: "normal", target: "partner", created_at: "now", updated_at: "now" },
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
  await page.goto("/tickets");
  await page.getByRole("button", { name: /提交工单/ }).first().click();
  await expect(page.getByText(/处理方/)).toBeVisible();
});
