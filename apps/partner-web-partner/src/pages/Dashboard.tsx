// Dashboard —— 实时余额 / 当月分润 / 客户活跃 / 趋势 sparkline
import { Card, Skeleton, Tag, Typography } from "@douyinfe/semi-ui";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { Sparkline } from "@/components/Sparkline";

export function Dashboard(): JSX.Element {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ["partner", "dashboard"],
    queryFn: () => api.getDashboard(),
    refetchInterval: 30_000, // 数据截至 30s 节奏
    staleTime: 15_000,
  });

  return (
    <Page
      title={t("nav.dashboard")}
      description={
        data
          ? t("app.data_as_of", { ts: new Date(data.data_as_of).toLocaleString() })
          : null
      }
    >
      {isLoading || !data ? (
        <Skeleton placeholder={<Skeleton.Paragraph rows={6} />} loading />
      ) : (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
            gap: 16,
            marginBottom: 16,
          }}
        >
          <Card title={t("dashboard.balance")}>
            <Typography.Title heading={3}>
              <MoneyDisplay fen={data.balance} />
            </Typography.Title>
            <div style={{ display: "flex", gap: 12, marginTop: 4, fontSize: 13 }}>
              <span>
                {t("dashboard.available")}: <MoneyDisplay fen={data.available} />
              </span>
              <Tag color="orange">
                {t("dashboard.held")}: <MoneyDisplay fen={data.held_total} />
              </Tag>
            </div>
          </Card>
          <Card title={t("dashboard.monthly_overview")}>
            <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: 8 }}>
              <div>
                <div style={{ color: "#6b7280", fontSize: 12 }}>{t("dashboard.gross")}</div>
                <strong>
                  <MoneyDisplay fen={data.monthly_gross} />
                </strong>
              </div>
              <div>
                <div style={{ color: "#6b7280", fontSize: 12 }}>{t("dashboard.cost")}</div>
                <strong>
                  <MoneyDisplay fen={data.monthly_cost} />
                </strong>
              </div>
              <div>
                <div style={{ color: "#6b7280", fontSize: 12 }}>{t("dashboard.net")}</div>
                <strong style={{ color: "#16a34a" }}>
                  <MoneyDisplay fen={data.monthly_net} />
                </strong>
              </div>
            </div>
          </Card>
          <Card title={t("dashboard.active_customers")}>
            <Typography.Title heading={3}>{data.customers_active}</Typography.Title>
            <div style={{ fontSize: 13, color: "#6b7280" }}>
              + {data.customers_new} {t("dashboard.new_customers")} · - {data.customers_churn} {t("dashboard.churn")}
            </div>
          </Card>
          <Card title={t("dashboard.kyc_due")}>
            <Typography.Title heading={3}>{data.kyc_due_within_30d}</Typography.Title>
          </Card>
        </div>
      )}
      <Card title={t("dashboard.trend")} style={{ marginBottom: 16 }}>
        {data ? (
          <Sparkline data={data.trend_30d} />
        ) : (
          <Skeleton placeholder={<Skeleton.Paragraph rows={3} />} loading />
        )}
      </Card>
    </Page>
  );
}
