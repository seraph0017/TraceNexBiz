// 错误码 → toast i18n key 映射 —— 与 storefront 对齐 + W1f 增量
// 注：app-local 因为含 customer 端独有 i18n key（invitation / dispute / ticket）；
// 通用映射在 `@tnbiz/api-client` 的 `mapApiError`，此处覆盖+扩展.
import type { ApiError } from "@tnbiz/api-client";

export interface ToastSpec {
  i18nKey: string;
  severity: "error" | "warning" | "info";
}

const TOAST_MAP: Record<string, ToastSpec> = {
  // 鉴权 / 权限（与 storefront 共有）
  BIZ_AUTH_INVALID: { i18nKey: "errors.auth.invalid", severity: "error" },
  BIZ_AUTH_MFA_REQUIRED: { i18nKey: "errors.auth.mfa_required", severity: "warning" },
  BIZ_AUTH_JWT_REVOKED: { i18nKey: "errors.auth.jwt_revoked", severity: "error" },
  BIZ_AUTH_LOCKED: { i18nKey: "errors.auth.locked", severity: "error" },
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

  // W1f append 项（partner 后台业务）
  BIZ_WALLET_INSUFFICIENT: { i18nKey: "errors.wallet.insufficient", severity: "error" },
  BIZ_WALLET_HOLD_FAILED: { i18nKey: "errors.wallet.hold_failed", severity: "error" },
  BIZ_SAGA_STUCK_UNKNOWN: { i18nKey: "errors.saga.unknown", severity: "warning" },
  BIZ_SAGA_ESCALATED: { i18nKey: "errors.saga.escalated", severity: "warning" },
  BIZ_INVITATION_EXHAUSTED: { i18nKey: "errors.invitation.exhausted", severity: "warning" },
  BIZ_INVITATION_REVOKED: { i18nKey: "errors.invitation.revoked", severity: "warning" },
  BIZ_CUSTOMER_INVITATION_REQUIRED: { i18nKey: "errors.customer.invitation_required", severity: "warning" },
  BIZ_DISPUTE_WINDOW_CLOSED: { i18nKey: "errors.dispute.window_closed", severity: "warning" },
  BIZ_TICKET_CLOSED: { i18nKey: "errors.ticket.closed", severity: "warning" },
  BIZ_PRICING_BELOW_FLOOR: { i18nKey: "errors.pricing.below_floor", severity: "warning" },
  BIZ_NETWORK_ERROR: { i18nKey: "errors.network", severity: "error" },
};

export function mapApiError(err: ApiError): ToastSpec {
  return TOAST_MAP[err.code] ?? { i18nKey: "errors.unknown", severity: "error" };
}
