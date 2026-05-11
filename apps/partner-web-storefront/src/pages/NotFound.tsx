import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useSeo } from "@/hooks/useSeo";

export function NotFound(): JSX.Element {
  const { t } = useTranslation();
  useSeo({
    title: `404 | ${t("app.title")}`,
    robots: "noindex,nofollow",
  });
  return (
    <section style={{ textAlign: "center", padding: "64px 0" }}>
      <h1 style={{ color: "#fff" }}>{t("notfound.title")}</h1>
      <p style={{ color: "#9ca3af" }}>{t("notfound.body")}</p>
      <Link to="/" style={{ color: "#60a5fa" }}>
        {t("common.back_home")}
      </Link>
    </section>
  );
}
