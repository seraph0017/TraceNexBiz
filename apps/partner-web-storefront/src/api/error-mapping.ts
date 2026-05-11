// 错误码 → toast i18n key 映射 —— 与 packages/api-client/src/error-mapping.ts 保持一致
// W1f/W1g 切到 shared package 后这里 re-export
import type { ApiError } from "./types";

export interface ToastSpec {
  i18nKey: string;
  severity: "error" | "warning" | "info";
}

const TOAST_MAP: Record<string, ToastSpec> = {
  BIZ_AUTH_INVALID: { i18nKey: "errors.auth.invalid", severity: "error" },
  BIZ_AUTH_MFA_REQUIRED: { i18nKey: "errors.auth.mfa_required", severity: "warning" },
  BIZ_AUTH_JWT_REVOKED: { i18nKey: "errors.auth.jwt_revoked", severity: "error" },
  BIZ_PERM_FORBIDDEN: { i18nKey: "errors.perm.forbidden", severity: "error" },
  BIZ_VALID_CONSENT: { i18nKey: "errors.valid.consent", severity: "warning" },
  BIZ_VALID_AMOUNT_OUT_OF_RANGE: { i18nKey: "errors.valid.amount_range", severity: "warning" },
  BIZ_VALID_INVALID_INPUT: { i18nKey: "errors.valid.invalid_input", severity: "warning" },
  BIZ_IDEM_REUSED_DIFFERENT_BODY: { i18nKey: "errors.idem.reused", severity: "warning" },
  BIZ_RATELIMIT_EXCEEDED: { i18nKey: "errors.ratelimit", severity: "warning" },
  BIZ_PARTNER_EMAIL_DUP: { i18nKey: "errors.partner.email_dup", severity: "error" },
  BIZ_PARTNER_PHONE_DUP: { i18nKey: "errors.partner.phone_dup", severity: "error" },
  BIZ_KYC_REJECTED: { i18nKey: "errors.kyc.rejected", severity: "error" },
  BIZ_KYC_FROZEN_YEARLY: { i18nKey: "errors.kyc.frozen_yearly", severity: "error" },
  BIZ_RES_NOT_FOUND: { i18nKey: "errors.not_found", severity: "info" },
  BIZ_FYAPI_5XX: { i18nKey: "errors.fyapi.unknown", severity: "warning" },
};

export function mapApiError(err: ApiError): ToastSpec {
  return TOAST_MAP[err.code] ?? { i18nKey: "errors.unknown", severity: "error" };
}
