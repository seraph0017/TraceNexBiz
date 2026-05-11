// ComplianceFooter — frontend §11.5 / Compliance CRIT-1 / M-8.
//
// 9 个备案号 key 来自 partner-api `biz_setting` （hot reload via /api/me/flags）：
//   - compliance.icp_record_no
//   - compliance.icp_license_no
//   - compliance.public_security_filing_no
//   - compliance.gen_ai_filing_no
//   - compliance.algorithm_filing_no
//   - compliance.deep_synthesis_filing_no
//   - compliance.dpo_contact_email
//   - compliance.dpo_contact_phone
//   - compliance.report_phone_12377_link
//
// W1e 实现：从 useBizSettings() hook 读 → 任一为空时 storefront CI build fail；prod 启动 readiness probe gate。
// W0 scaffold：仅给 props 接口与占位 DOM，确保 build 通过。
import * as React from 'react';

export interface ComplianceFooterProps {
  icpRecordNo?: string;
  icpLicenseNo?: string;
  publicSecurityFilingNo?: string;
  genAiFilingNo?: string;
  algorithmFilingNo?: string;
  deepSynthesisFilingNo?: string;
  dpoEmail?: string;
  dpoPhone?: string;
  report12377Link?: string;
}

export const ComplianceFooter: React.FC<ComplianceFooterProps> = (props) => {
  // TODO(W1e): per frontend §11.5 — render 9 fields with proper i18n + accessibility.
  return (
    <footer
      role="contentinfo"
      style={{ padding: '16px', textAlign: 'center', fontSize: 12, color: '#666' }}
      data-testid="compliance-footer"
    >
      <div>{props.icpRecordNo ?? 'ICP 备案号 TBD'}</div>
      <div>DPO: {props.dpoEmail ?? 'tbd@tracenex.cn'}</div>
      <div>
        举报: <a href={props.report12377Link ?? 'https://www.12377.cn'}>12377</a>
      </div>
    </footer>
  );
};
