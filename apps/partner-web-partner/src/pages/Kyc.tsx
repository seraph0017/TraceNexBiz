// KYC —— 状态展示 + 重审入口
import { useNavigate } from "react-router-dom";
import { Banner, Button, Card, Descriptions, Spin, Tag } from "@douyinfe/semi-ui";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";

export function Kyc(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data, isLoading } = useQuery({
    queryKey: ["partner", "kyc"],
    queryFn: () => api.getKycStatus(),
  });

  if (isLoading) return <Spin />;
  if (!data) return <Page title={t("kyc.title")}>{t("app.empty")}</Page>;

  const canResubmit =
    data.status === "rejected" || data.status === "none";

  return (
    <Page
      title={t("kyc.title")}
      actions={
        canResubmit && (
          <Button
            theme="solid"
            type="primary"
            onClick={() => navigate("/kyc/resubmit")}
          >
            {t("kyc.resubmit")}
          </Button>
        )
      }
    >
      {data.status === "rejected" && (
        <Banner
          type="danger"
          description={t("kyc.reject_banner", { reason: data.reject_reason ?? "—" })}
          closeIcon={null}
          style={{ marginBottom: 12 }}
        />
      )}
      {data.status === "frozen_yearly_limit" && (
        <Banner
          type="warning"
          description={t("kyc.frozen_banner")}
          closeIcon={null}
          style={{ marginBottom: 12 }}
        />
      )}
      <Card>
        <Descriptions
          data={[
            {
              key: t("customers.col_status"),
              value: <Tag>{t(`kyc.status_${data.status}`)}</Tag>,
            },
            {
              key: "submitted_at",
              value: data.submitted_at ? new Date(data.submitted_at).toLocaleString() : "—",
            },
            {
              key: "approved_at",
              value: data.approved_at ? new Date(data.approved_at).toLocaleString() : "—",
            },
            {
              key: "next_review_due_at",
              value: data.next_review_due_at
                ? new Date(data.next_review_due_at).toLocaleDateString()
                : "—",
            },
          ]}
        />
      </Card>
    </Page>
  );
}
