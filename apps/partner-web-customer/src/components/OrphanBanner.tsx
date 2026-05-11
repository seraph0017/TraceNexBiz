// OrphanBanner —— 渠道商终止 30 天宽限提示
import { useNavigate } from "react-router-dom";
import { Banner, Button } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@/stores/authStore";

export function OrphanBanner(): JSX.Element | null {
  const me = useAuthStore((s) => s.me);
  const { t } = useTranslation();
  const navigate = useNavigate();
  if (!me) return null;
  if (me.status !== "orphaned" || !me.partner_terminated_at) return null;
  return (
    <Banner
      type="warning"
      description={t("orphan.intro", { at: me.partner_terminated_at })}
      closeIcon={null}
      style={{ borderRadius: 0 }}
    >
      <Button size="small" onClick={() => navigate("/orphan-notice")}>
        {t("nav.orphan_notice")}
      </Button>
    </Banner>
  );
}
