// KYC 状态横幅 —— 驳回 / 冻结 时给入口
import { useNavigate } from "react-router-dom";
import { Banner, Button } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@/stores/authStore";

export function KycBanner(): JSX.Element | null {
  const me = useAuthStore((s) => s.me);
  const { t } = useTranslation();
  const navigate = useNavigate();
  if (!me) return null;
  if (me.kyc_status === "rejected") {
    return (
      <Banner
        type="danger"
        description={t("kyc.reject_banner", { reason: me.kyc_reject_reason ?? "—" })}
        closeIcon={null}
        style={{ borderRadius: 0 }}
      >
        <Button size="small" onClick={() => navigate("/kyc")}>
          {t("kyc.resubmit")}
        </Button>
      </Banner>
    );
  }
  if (me.kyc_status === "frozen_yearly_limit") {
    return (
      <Banner
        type="warning"
        description={t("kyc.frozen_banner")}
        closeIcon={null}
        style={{ borderRadius: 0 }}
      />
    );
  }
  return null;
}
