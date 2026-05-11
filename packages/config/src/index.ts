// Shared front-end env + biz_setting feature flag 镜像 (frontend §13).
// W1e: 接 partner-api `/api/me/flags` SSE / 60s polling fallback.

export interface FeatureFlags {
  /** ICP 经营许可证拿证前 storefront 仅显示 "招商内测"（Compliance MED-20） */
  icpLicenseActive: boolean;
  /** 生成式 AI 备案 */
  genAiFilingActive: boolean;
  /** 持牌分账方上线 → 充值 / 月结 enable */
  licensedProviderActive: boolean;
  /** 等保 2.0 二级 */
  epd2FilingActive: boolean;
}

export const DEFAULT_FLAGS: FeatureFlags = {
  icpLicenseActive: false,
  genAiFilingActive: false,
  licensedProviderActive: false,
  epd2FilingActive: false,
};
