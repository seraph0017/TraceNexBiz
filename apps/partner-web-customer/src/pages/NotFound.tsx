// 404
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";

export function NotFound(): JSX.Element {
  const { t } = useTranslation();
  return (
    <Page title="404">
      <p>{t("app.empty")}</p>
      <Link to="/dashboard">{t("nav.dashboard")}</Link>
    </Page>
  );
}
