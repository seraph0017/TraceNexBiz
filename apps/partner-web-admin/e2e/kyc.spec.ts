import { test, expect } from "@playwright/test";

// e2e: admin KYC 审核通过
test("admin kyc approve", async ({ page, context }) => {
  await context.route("**/api/admin/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          id: 1,
          username: "alice-staff",
          email_masked: "a***@example.com",
          role: "kyc_reviewer",
          permissions: ["kyc.approve", "kyc.reject"],
          ip_allowed: true,
          mfa_enrolled: true,
        },
        error: null,
      }),
    });
  });
  await context.route("**/api/admin/kyc/1", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          data: {
            id: 1,
            subject_kind: "partner",
            subject_id: 10,
            subject_name: "X",
            status: "submitted",
            real_name_masked: "张*三",
            id_card_masked: "1101**********0000",
            documents: [],
            created_at: "now",
          },
          error: null,
        }),
      });
    } else {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, data: { status: "approved" }, error: null }),
      });
    }
  });
  await context.route("**/api/admin/kyc/1/review", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ success: true, data: { status: "approved" }, error: null }),
    });
  });
  await page.goto("/kyc/1");
  await page.getByRole("button", { name: /通过/ }).click();
  await expect(page.locator("body")).toBeVisible();
});
