import { test, expect } from "@playwright/test";

// e2e: 工单提交 + 列表刷新
test("ticket creation flow", async ({ page, context }) => {
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
  let tickets: { id: number; subject: string; status: string; priority: string; created_at: string; updated_at: string }[] = [];
  await context.route("**/api/partner/tickets", (r) => {
    if (r.request().method() === "POST") {
      const body = JSON.parse(r.request().postData() ?? "{}") as { subject: string; priority: string };
      const created = {
        id: tickets.length + 1,
        subject: body.subject,
        status: "open",
        priority: body.priority ?? "normal",
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      tickets = [...tickets, created];
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, data: created, error: null }),
      });
    } else {
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, data: tickets, error: null }),
      });
    }
  });

  await page.goto("/tickets");
  await page.getByRole("button", { name: /提交工单/ }).click();
  await page.getByLabel(/主题/).fill("无法登录");
  await page.getByLabel(/描述/).fill("两次刷新均 401");
  await page.getByRole("button", { name: /确定/ }).click();
  await expect(page.getByText("无法登录")).toBeVisible({ timeout: 5_000 });
});
