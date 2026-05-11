// 客户详情 —— 4 tab：基础信息 / 配额 / 用量 / 工单
import { useNavigate, useParams } from "react-router-dom";
import { Button, Card, Descriptions, Empty, Spin, Tabs, Tag } from "@douyinfe/semi-ui";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { MoneyDisplay } from "@/components/MoneyDisplay";

export function CustomerDetail(): JSX.Element {
  const { id } = useParams();
  const numId = Number(id);
  const navigate = useNavigate();
  const { t } = useTranslation();

  const { data, isLoading } = useQuery({
    queryKey: ["partner", "customer", numId],
    queryFn: () => api.getCustomer(numId),
    enabled: Number.isFinite(numId),
  });

  const usageQ = useQuery({
    queryKey: ["partner", "customer", numId, "usage"],
    queryFn: () => api.getCustomerUsage(numId),
    enabled: Number.isFinite(numId),
  });

  if (isLoading) return <Spin />;
  if (!data) return <Empty title={t("errors.not_found")} />;

  const usage = usageQ.data ?? [];

  return (
    <Page
      title={`${t("customers.detail")} #${data.id}`}
      actions={
        <>
          <Button onClick={() => navigate("/customers")}>{t("app.back")}</Button>
          <Button
            theme="solid"
            type="primary"
            onClick={() => navigate(`/allocate?customer=${data.id}`)}
          >
            {t("nav.allocate")}
          </Button>
        </>
      }
    >
      <Card>
        <Tabs type="line">
          <Tabs.TabPane tab={t("customers.tab_overview")} itemKey="overview">
            <Descriptions
              data={[
                { key: t("customers.col_id"), value: String(data.id) },
                { key: t("customers.col_name"), value: data.display_name },
                { key: t("customers.col_email"), value: data.email_masked },
                { key: t("customers.col_status"), value: <Tag>{data.status}</Tag> },
                {
                  key: t("customers.col_created"),
                  value: new Date(data.created_at).toLocaleString(),
                },
              ]}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t("customers.tab_quota")} itemKey="quota">
            <Descriptions
              data={[
                {
                  key: "已分配额度",
                  value: <MoneyDisplay fen={data.quota_total} />,
                },
                {
                  key: "已用",
                  value: <MoneyDisplay fen={data.quota_used} />,
                },
                {
                  key: "剩余",
                  value: <MoneyDisplay fen={data.remaining_quota} />,
                },
                {
                  key: "本月调用",
                  value: data.monthly_calls,
                },
              ]}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t("customers.tab_usage")} itemKey="usage">
            {usage.length === 0 ? (
              <Empty title={t("app.empty")} />
            ) : (
              <table style={{ width: "100%", borderCollapse: "collapse" }}>
                <thead>
                  <tr>
                    <th style={th}>日期</th>
                    <th style={th}>调用</th>
                    <th style={th}>成本</th>
                  </tr>
                </thead>
                <tbody>
                  {usage.map((r: api.CustomerUsageRow) => (
                    <tr key={r.date}>
                      <td style={td}>{r.date}</td>
                      <td style={td}>{r.calls}</td>
                      <td style={td}>
                        <MoneyDisplay fen={r.cost} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Tabs.TabPane>
          <Tabs.TabPane tab={t("customers.tab_tickets")} itemKey="tickets">
            <p>该客户工单：{data.ticket_open_count} 待处理</p>
            <Button onClick={() => navigate(`/tickets?customer=${data.id}`)}>查看工单</Button>
          </Tabs.TabPane>
        </Tabs>
      </Card>
    </Page>
  );
}

const th: React.CSSProperties = {
  textAlign: "left",
  padding: "6px 8px",
  borderBottom: "1px solid #e5e7eb",
  fontSize: 13,
};
const td: React.CSSProperties = { padding: "6px 8px", fontSize: 13 };
