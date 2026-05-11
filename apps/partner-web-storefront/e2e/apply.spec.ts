// Playwright e2e —— 招商完整流程（KYC 上传 mock + PIPL 单独同意 + 提交）
//
// 运行前：pnpm add -D -w @playwright/test && pnpm exec playwright install chromium
// 然后：pnpm --filter partner-web-storefront exec playwright test
//
// W0 脚手架未引 playwright 依赖 —— 本文件作为可运行规范，CI 接入时启用。
import { test, expect, type Route } from "@playwright/test";

test.describe("storefront 招商流程", () => {
  test.beforeEach(async ({ page }) => {
    // mock 公共接口
    await page.route("**/api/public/biz_setting/footer", (route: Route) =>
      route.fulfill({
        json: {
          success: true,
          error: null,
          data: {
            icp_record_no: "京 ICP 备 2026000001 号",
            gen_ai_filing_no: "Beijing-2026-001",
            algorithm_filing_no: "Algo-001",
            dpo_email: "dpo@tracenex.cn",
            report_phone_12377_link: "https://www.12377.cn",
          },
        },
      }),
    );
    await page.route("**/api/public/models", (route: Route) =>
      route.fulfill({
        json: {
          success: true,
          error: null,
          data: { icp_license_active: false, models: [] },
        },
      }),
    );
    await page.route("**/api/public/consent", (route: Route) =>
      route.fulfill({
        json: {
          success: true,
          error: null,
          data: { consent_id: 1, version: "2026-05-pipl-v1" },
        },
      }),
    );
    await page.route("**/api/public/kyc/presign", async (route: Route) => {
      await route.fulfill({
        json: {
          success: true,
          error: null,
          data: {
            upload_url: "http://localhost:5173/__upload",
            object_url: "http://oss.example.com/x",
            required_headers: {},
            expires_at: new Date(Date.now() + 60_000).toISOString(),
          },
        },
      });
    });
    await page.route("http://localhost:5173/__upload", (route: Route) =>
      route.fulfill({ status: 200, body: "" }),
    );
    await page.route("**/api/public/partner/apply", (route: Route) =>
      route.fulfill({
        json: { success: true, error: null, data: { id: 12345, status: "applied" } },
      }),
    );
  });

  test("首页可达 + 合规 footer 渲染", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByTestId("compliance-footer")).toContainText("ICP");
  });

  test("招商 happy path 个人申请提交成功", async ({ page }) => {
    await page.goto("/apply-partner");
    await page.selectOption("#apply-type", "individual");
    await page.fill("#apply-name", "测试用户");
    await page.fill("#apply-phone", "13800138000");
    await page.fill("#apply-email", "user@example.com");
    await page.getByRole("button", { name: /next|下一步/i }).click();

    await page.fill("#apply-legal-id", "11010119900101001X");
    await page.getByRole("button", { name: /next|下一步/i }).click();

    await page.fill("#apply-calls", "100000");
    await page.fill("#apply-usecase", "我们希望接入大模型用于客服自动化处理");
    await page.getByRole("button", { name: /next|下一步/i }).click();

    // KYC 上传：使用 setInputFiles 模拟文件
    const buf = Buffer.from([0xff, 0xd8, 0xff, 0xe0]); // JPEG magic bytes
    for (const id of ["kyc-id_front", "kyc-id_back", "kyc-legal_person_face"]) {
      await page.setInputFiles(`#${id}`, {
        name: "test.jpg",
        mimeType: "image/jpeg",
        buffer: buf,
      });
    }
    await page.getByRole("button", { name: /next|下一步/i }).click();

    // 同意框
    await page.getByRole("checkbox").check();
    await page.getByRole("button", { name: /submit|提交/i }).click();

    await expect(page.getByRole("heading", { name: /pending review|等待审核/i })).toBeVisible({
      timeout: 5000,
    });
  });
});
