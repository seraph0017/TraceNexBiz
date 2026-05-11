import { test, expect } from "@playwright/test";

// e2e: 登录流程（mock /api/public/auth/login + /api/partner/me）
test("partner login redirects to dashboard", async ({ page, context }) => {
  await context.route("**/api/public/auth/login", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: { actor_type: "partner", actor_id: 1, fy_user_id: 100, expires_at: "2099-01-01T00:00:00Z" },
        error: null,
      }),
    });
  });
  await context.route("**/api/partner/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          id: 1,
          type: "individual",
          status: "approved",
          contact_name: "Alice",
          contact_phone_masked: "138****5678",
          contact_email_masked: "a***e@example.com",
          kyc_status: "approved",
          mfa_enrolled: true,
        },
        error: null,
      }),
    });
  });
  await context.route("**/api/partner/dashboard", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          balance: 1245000,
          available: 1120000,
          held_total: 125000,
          open_holds_count: 3,
          monthly_gross: 340000,
          monthly_cost: 280000,
          monthly_net: 60000,
          customers_active: 42,
          customers_new: 5,
          customers_churn: 1,
          kyc_due_within_30d: 2,
          data_as_of: new Date().toISOString(),
          trend_30d: [],
        },
        error: null,
      }),
    });
  });
  await page.goto("/auth/login");
  await page.getByLabel(/邮箱/).fill("alice@example.com");
  await page.getByLabel(/密码/).fill("Passw0rd!");
  await page.getByRole("button", { name: /登录/ }).click();
  await expect(page).toHaveURL(/\/dashboard/);
  await expect(page.getByText("应付台账余额")).toBeVisible();
});
