import { useTranslation } from "react-i18next";
import { useSeo } from "@/hooks/useSeo";

interface RuleProps {
  title: string;
  body: string;
}

function RuleCard({ title, body }: RuleProps): JSX.Element {
  return (
    <article
      style={{
        background: "#11151c",
        border: "1px solid #1f2937",
        borderRadius: 8,
        padding: 20,
        flex: "1 1 280px",
        minWidth: 260,
      }}
    >
      <h3 style={{ margin: 0, fontSize: 16, color: "#fff" }}>{title}</h3>
      <p style={{ marginTop: 8, color: "#9ca3af", lineHeight: 1.6, fontSize: 14 }}>{body}</p>
    </article>
  );
}

export function Pricing(): JSX.Element {
  const { t } = useTranslation();
  useSeo({
    title: `${t("pricing.title")} | ${t("app.title")}`,
    description: t("pricing.subtitle"),
    canonical: "https://partner.tracenex.cn/pricing",
    robots: "index,follow",
  });

  const rules: ReadonlyArray<RuleProps> = [
    { title: t("pricing.rule.token.title"), body: t("pricing.rule.token.body") },
    { title: t("pricing.rule.markup.title"), body: t("pricing.rule.markup.body") },
    { title: t("pricing.rule.invoice.title"), body: t("pricing.rule.invoice.body") },
  ];

  return (
    <section>
      <h1 style={{ color: "#fff", margin: 0 }}>{t("pricing.title")}</h1>
      <p style={{ color: "#9ca3af" }}>{t("pricing.subtitle")}</p>
      <div
        role="note"
        style={{
          padding: 12,
          background: "#1e293b",
          border: "1px solid #334155",
          borderRadius: 6,
          marginBottom: 16,
          color: "#fde68a",
        }}
      >
        {t("pricing.beta_notice")}
      </div>
      <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
        {rules.map((r) => (
          <RuleCard key={r.title} {...r} />
        ))}
      </div>
    </section>
  );
}
