import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useSeo } from "@/hooks/useSeo";

interface FeatureProps {
  title: string;
  body: string;
}

function FeatureCard({ title, body }: FeatureProps): JSX.Element {
  return (
    <article
      style={{
        background: "#11151c",
        border: "1px solid #1f2937",
        borderRadius: 8,
        padding: 20,
        flex: "1 1 220px",
        minWidth: 220,
      }}
    >
      <h3 style={{ margin: 0, fontSize: 16, color: "#fff" }}>{title}</h3>
      <p style={{ marginTop: 8, color: "#9ca3af", lineHeight: 1.6, fontSize: 14 }}>{body}</p>
    </article>
  );
}

export function Home(): JSX.Element {
  const { t } = useTranslation();
  useSeo({
    title: `${t("app.title")} | ${t("home.hero.title")}`,
    description: t("home.hero.subtitle"),
    canonical: "https://partner.tracenex.cn/",
    robots: "index,follow",
  });

  const features: ReadonlyArray<FeatureProps> = [
    { title: t("home.feature.compliance.title"), body: t("home.feature.compliance.body") },
    { title: t("home.feature.audit.title"), body: t("home.feature.audit.body") },
    { title: t("home.feature.settle.title"), body: t("home.feature.settle.body") },
    { title: t("home.feature.kyc.title"), body: t("home.feature.kyc.body") },
  ];

  return (
    <div>
      <section
        style={{ padding: "48px 0", textAlign: "center", borderBottom: "1px solid #1f2937" }}
      >
        <h1 style={{ fontSize: 36, color: "#fff", margin: 0 }}>{t("home.hero.title")}</h1>
        <p
          style={{
            fontSize: 16,
            color: "#9ca3af",
            margin: "16px auto 32px",
            maxWidth: 720,
            lineHeight: 1.6,
          }}
        >
          {t("home.hero.subtitle")}
        </p>
        <div style={{ display: "flex", gap: 12, justifyContent: "center" }}>
          <Link
            to="/apply-partner"
            style={{
              padding: "10px 24px",
              background: "#2563eb",
              color: "#fff",
              borderRadius: 6,
              textDecoration: "none",
            }}
          >
            {t("home.hero.cta_apply")}
          </Link>
          <Link
            to="/models"
            style={{
              padding: "10px 24px",
              background: "transparent",
              color: "#cbd5e1",
              border: "1px solid #2a2f36",
              borderRadius: 6,
              textDecoration: "none",
            }}
          >
            {t("home.hero.cta_models")}
          </Link>
        </div>
      </section>

      <section
        aria-label={t("home.hero.title")}
        style={{ padding: "32px 0", display: "flex", gap: 16, flexWrap: "wrap" }}
      >
        {features.map((f) => (
          <FeatureCard key={f.title} {...f} />
        ))}
      </section>
    </div>
  );
}
