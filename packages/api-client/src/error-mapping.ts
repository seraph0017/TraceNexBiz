// 错误码 → toast i18n key 映射（frontend §5.4）.
import type { ApiError } from './types';

export interface ToastSpec {
  i18nKey: string;
  severity: 'error' | 'warning' | 'info';
}

const TOAST_MAP: Record<string, ToastSpec> = {
  BIZ_AUTH_JWT_REVOKED: { i18nKey: 'errors.auth.jwt_revoked', severity: 'error' },
  BIZ_PERM_FORBIDDEN: { i18nKey: 'errors.perm.forbidden', severity: 'error' },
  BIZ_VALID_AMOUNT_OUT_OF_RANGE: { i18nKey: 'errors.valid.amount_range', severity: 'warning' },
  BIZ_IDEM_REUSED_DIFFERENT_BODY: { i18nKey: 'errors.idem.reused', severity: 'warning' },
  BIZ_WALLET_INSUFFICIENT_AVAILABLE: { i18nKey: 'errors.wallet.insufficient', severity: 'warning' },
  BIZ_PRICING_OVERLAP_WINDOW: { i18nKey: 'errors.pricing.overlap', severity: 'error' },
  BIZ_SAGA_STUCK_UNKNOWN: { i18nKey: 'errors.saga.unknown', severity: 'warning' },
  BIZ_FYAPI_5XX: { i18nKey: 'errors.fyapi.unknown', severity: 'warning' },
  BIZ_KYC_REJECTED: { i18nKey: 'errors.kyc.rejected', severity: 'error' },
  BIZ_RES_NOT_FOUND: { i18nKey: 'errors.not_found', severity: 'info' },
};

export function mapApiError(err: ApiError): ToastSpec {
  return TOAST_MAP[err.code] ?? { i18nKey: 'errors.unknown', severity: 'error' };
}
