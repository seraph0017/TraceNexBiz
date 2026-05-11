// ComplianceFooter wrapper —— 把 useComplianceFooter() 的数据塞给可视化组件
// 视觉部分内联实现（不依赖 packages/ui-kit，避免 monorepo workspace 依赖）；
// 渲染契约与 packages/ui-kit/ComplianceFooter 一致 —— 9 备案号 + 12377 链接 + DPO 联系。
import { useTranslation } from "react-i18next";
import { useComplianceFooter } from "@/hooks/useComplianceFooter";
import type { ComplianceFooterDTO } from "@/api";

interface FooterRowProps {
  label: string;
  value?: string;
  href?: string;
}

function FooterRow({ label, value, href }: FooterRowProps): JSX.Element | null {
  if (!value) return null;
  return (
    <span style={{ marginRight: 16, whiteSpace: "nowrap" }}>
      <span style={{ color: "#888" }}>{label}：</span>
      {href ? (
        <a href={href} target="_blank" rel="noreferrer noopener" style={{ color: "#bbb" }}>
          {value}
        </a>
      ) : (
        <span>{value}</span>
      )}
    </span>
  );
}

export function ComplianceFooter(): JSX.Element {
  const { t } = useTranslation();
  const query = useComplianceFooter();
  const data: Partial<ComplianceFooterDTO> = query.data ?? {};
  return (
    <footer
      role="contentinfo"
      data-testid="compliance-footer"
      style={{
        padding: "20px 16px",
        borderTop: "1px solid #2a2f36",
        background: "#0E1116",
        color: "#cbd5e1",
        fontSize: 12,
        lineHeight: 1.8,
      }}
    >
      <div style={{ maxWidth: 1200, margin: "0 auto", display: "flex", flexWrap: "wrap" }}>
        <FooterRow label={t("footer.icp")} value={data.icp_record_no} />
        <FooterRow label={t("footer.icp_license")} value={data.icp_license_no} />
        <FooterRow label={t("footer.public_security")} value={data.public_security_filing_no} />
        <FooterRow label={t("footer.gen_ai")} value={data.gen_ai_filing_no} />
        <FooterRow label={t("footer.algorithm")} value={data.algorithm_filing_no} />
        <FooterRow label={t("footer.deep_synthesis")} value={data.deep_synthesis_filing_no} />
        <FooterRow
          label={t("footer.dpo")}
          value={data.dpo_email ?? data.dpo_phone}
          href={data.dpo_email ? `mailto:${data.dpo_email}` : undefined}
        />
        <FooterRow
          label={t("footer.report_12377")}
          value="12377"
          href={data.report_phone_12377_link ?? "https://www.12377.cn"}
        />
      </div>
      <div style={{ maxWidth: 1200, margin: "8px auto 0", color: "#6b7280" }}>
        {t("footer.copyright")}
      </div>
    </footer>
  );
}
