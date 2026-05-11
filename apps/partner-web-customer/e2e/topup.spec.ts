import { test, expect } from "@playwright/test";

// e2e: 客户充值流程（场景 D）—— 提交后进入 status 页
test("customer topup flow", async ({ page, context }) => {
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
          partner_name: "ABC Partner",
          partner_terminated_at: null,
          kyc_status: "approved",
          consent_pipl_signed: true,
        },
        error: null,
      }),
    });
  });
  await context.route("**/api/customer/topup", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: { intent_id: "intent-1", redirect_url: "", saga_id: "saga-1" },
        error: null,
      }),
    });
  });
  await context.route("**/api/customer/topup/intent-1", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: { status: "funded", saga_id: "saga-1", amount: 10000 },
        error: null,
      }),
    });
  });
  await page.goto("/topup");
  await page.getByRole("button", { name: /去支付/ }).click();
  await expect(page).toHaveURL(/\/topup\/intent-1/);
});
