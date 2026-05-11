import { describe, it, expect } from "vitest";
import { mapApiError } from "@/api/error-mapping";

describe("error-mapping", () => {
  it("已知错误码映射到对应 i18n key", () => {
    expect(
      mapApiError({ code: "BIZ_AUTH_INVALID", trace_id: "t1" }).i18nKey,
    ).toBe("errors.auth.invalid");
    expect(
      mapApiError({ code: "BIZ_PARTNER_EMAIL_DUP", trace_id: "t1" }).i18nKey,
    ).toBe("errors.partner.email_dup");
    expect(
      mapApiError({ code: "BIZ_KYC_FROZEN_YEARLY", trace_id: "t1" }).i18nKey,
    ).toBe("errors.kyc.frozen_yearly");
  });

  it("未知错误码 fallback 到 errors.unknown", () => {
    expect(
      mapApiError({ code: "BIZ_NEW_CODE_999", trace_id: "t1" }).i18nKey,
    ).toBe("errors.unknown");
  });

  it("severity 区分 error / warning / info", () => {
    expect(mapApiError({ code: "BIZ_AUTH_INVALID", trace_id: "t1" }).severity).toBe("error");
    expect(mapApiError({ code: "BIZ_RATELIMIT_EXCEEDED", trace_id: "t1" }).severity).toBe(
      "warning",
    );
    expect(mapApiError({ code: "BIZ_RES_NOT_FOUND", trace_id: "t1" }).severity).toBe("info");
  });
});
