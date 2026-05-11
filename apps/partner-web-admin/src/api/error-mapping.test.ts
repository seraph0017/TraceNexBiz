import { describe, it, expect } from "vitest";
import { mapApiError } from "./error-mapping";

describe("mapApiError", () => {
  it("maps known partner-side codes", () => {
    expect(mapApiError({ code: "BIZ_WALLET_INSUFFICIENT", trace_id: "x" })).toEqual({
      i18nKey: "errors.wallet.insufficient",
      severity: "error",
    });
    expect(mapApiError({ code: "BIZ_SAGA_STUCK_UNKNOWN", trace_id: "x" })).toEqual({
      i18nKey: "errors.saga.unknown",
      severity: "warning",
    });
  });
  it("falls back to unknown", () => {
    expect(mapApiError({ code: "BIZ_FOO_BAR", trace_id: "x" })).toEqual({
      i18nKey: "errors.unknown",
      severity: "error",
    });
  });
});
