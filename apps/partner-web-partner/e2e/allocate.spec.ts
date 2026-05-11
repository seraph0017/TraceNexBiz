import { test, expect } from "@playwright/test";

// e2e: 分配额度 saga 三阶段 UI（mock 钱包 + saga 状态机：running → succeeded）
test("allocate saga shows stepper progressing to success", async ({ page, context }) => {
  // me + wallet
  await context.route("**/api/partner/me", (r) =>
    r.fulfill({
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
    }),
  );
  await context.route("**/api/partner/wallet", (r) =>
    r.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          wallet: { partner_id: 1, balance: 1120000, currency: "CNY", updated_at: new Date().toISOString() },
          held_total: 0,
          available: 1120000,
          open_holds_count: 0,
        },
        error: null,
      }),
    }),
  );
  await context.route("**/api/partner/allocate", (r) =>
    r.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ success: true, data: { saga_id: "sa-1" }, error: null }),
    }),
  );
  let saga_polls = 0;
  await context.route("**/api/partner/saga/sa-1", (r) => {
    saga_polls += 1;
    r.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          saga_id: "sa-1",
          state: saga_polls >= 2 ? "succeeded" : "pending",
          steps: [],
          trace_id: "t1",
        },
        error: null,
      }),
    });
  });

  await page.goto("/allocate?customer=42");
  await page.getByLabel(/本次分配金额/).fill("100");
  await page.getByRole("button", { name: /提交/ }).click();
  await expect(page.getByText(/锁定余额/)).toBeVisible();
  await expect(page.getByText(/完成/)).toBeVisible({ timeout: 15_000 });
});
