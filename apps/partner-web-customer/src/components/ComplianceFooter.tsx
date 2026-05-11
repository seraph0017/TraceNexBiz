// ComplianceFooter —— PIPL §11.5 备案号、ICP、算法备案
import { useTranslation } from "react-i18next";

export function ComplianceFooter(): JSX.Element {
  const { t } = useTranslation();
  return (
    <footer
      style={{
        textAlign: "center",
        padding: "16px 24px 24px",
        fontSize: 12,
        color: "#6b7280",
        borderTop: "1px solid #e5e7eb",
        background: "#fff",
      }}
    >
      {t("compliance.footer")}
    </footer>
  );
}
