// Dashboard —— 余额 / 用量 / 模型调用统计图表
import { useQuery } from "@tanstack/react-query";
import { Card, Spin, Empty } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Sparkline } from "@/components/Sparkline";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { getDashboard } from "@/api/customer";

export function Dashboard(): JSX.Element {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ["customer", "dashboard"],
    queryFn: getDashboard,
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <Page title={t("nav.dashboard")}>
        <Spin />
      </Page>
    );
  }
  if (!data) {
    return (
      <Page title={t("nav.dashboard")}>
        <Empty description={t("app.empty")} />
      </Page>
    );
  }

  const trend = data.trend_30d.map((d) => ({ date: d.date, net: d.cost }));

  return (
    <Page
      title={t("nav.dashboard")}
      description={t("app.data_as_of", { ts: data.data_as_of })}
    >
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))", gap: 16, marginBottom: 16 }}>
        <Card title={t("dashboard.balance")}>
          <div style={{ fontSize: 22 }}>
            <MoneyDisplay fen={data.balance} />
          </div>
        </Card>
        <Card title={t("dashboard.monthly_calls")}>
          <div style={{ fontSize: 22 }}>{data.monthly_calls.toLocaleString("zh-CN")}</div>
        </Card>
        <Card title={t("dashboard.monthly_cost")}>
          <div style={{ fontSize: 22 }}>
            <MoneyDisplay fen={data.monthly_cost} />
          </div>
        </Card>
        <Card title={t("dashboard.remaining_quota")}>
          <div style={{ fontSize: 22 }}>{data.remaining_quota.toLocaleString("zh-CN")}</div>
        </Card>
        <Card title={t("dashboard.active_models")}>
          <div style={{ fontSize: 22 }}>{data.active_models}</div>
        </Card>
      </div>
      <Card title={t("dashboard.trend")}>
        <Sparkline data={trend} />
      </Card>
    </Page>
  );
}
