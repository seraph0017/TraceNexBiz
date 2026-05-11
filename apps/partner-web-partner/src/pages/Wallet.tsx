// 钱包 —— 余额 / 流水 / hold
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Button,
  Card,
  Empty,
  Pagination,
  Spin,
  Table,
  Tabs,
  Tag,
} from "@douyinfe/semi-ui";
import type { ColumnProps } from "@douyinfe/semi-ui/lib/es/table";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { MoneyDisplay } from "@/components/MoneyDisplay";

const PAGE_LIMIT = 20;

export function Wallet(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [page, setPage] = useState(1);

  const wallet = useQuery({
    queryKey: ["partner", "wallet"],
    queryFn: () => api.getWallet(),
    refetchInterval: 30_000,
  });

  const logs = useQuery({
    queryKey: ["partner", "wallet", "logs", page],
    queryFn: () => api.getWalletLogs({ page, limit: PAGE_LIMIT }),
    placeholderData: (prev) => prev,
  });

  const holds = useQuery({
    queryKey: ["partner", "wallet", "holds"],
    queryFn: () => api.getWalletHolds(),
  });

  const logCols: ColumnProps<api.WalletLog>[] = [
    { title: t("wallet.created_at"), dataIndex: "created_at", render: (v: string) => new Date(v).toLocaleString() },
    { title: t("wallet.type"), dataIndex: "type" },
    {
      title: t("wallet.amount"),
      dataIndex: "amount",
      align: "right" as const,
      render: (v: number) => (
        <span style={{ color: v >= 0 ? "#16a34a" : "#dc2626" }}>
          <MoneyDisplay fen={v} />
        </span>
      ),
    },
    {
      title: t("wallet.balance_after"),
      dataIndex: "balance_after",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    { title: t("wallet.ref"), dataIndex: "ref" },
  ];

  const holdCols: ColumnProps<api.WalletHold>[] = [
    { title: "saga_id", dataIndex: "saga_id" },
    {
      title: t("wallet.amount"),
      dataIndex: "amount",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: t("customers.col_status"),
      dataIndex: "status",
      render: (v: api.WalletHold["status"]) => (
        <Tag color={v === "held" ? "orange" : v === "released" ? "green" : "blue"}>{v}</Tag>
      ),
    },
    { title: "原因", dataIndex: "reason" },
    { title: t("wallet.created_at"), dataIndex: "created_at", render: (v: string) => new Date(v).toLocaleString() },
  ];

  return (
    <Page
      title={t("wallet.title")}
      actions={
        <Button theme="solid" type="primary" onClick={() => navigate("/wallet/topup")}>
          {t("wallet.topup")}
        </Button>
      }
    >
      <Card style={{ marginBottom: 16 }}>
        {!wallet.data ? (
          <Spin />
        ) : (
          <div style={{ display: "flex", gap: 32, flexWrap: "wrap" }}>
            <div>
              <div style={{ color: "#6b7280", fontSize: 13 }}>{t("dashboard.balance")}</div>
              <h2 style={{ margin: 0 }}>
                <MoneyDisplay fen={wallet.data.wallet.balance} />
              </h2>
            </div>
            <div>
              <div style={{ color: "#6b7280", fontSize: 13 }}>{t("dashboard.available")}</div>
              <h2 style={{ margin: 0, color: "#16a34a" }}>
                <MoneyDisplay fen={wallet.data.available} />
              </h2>
            </div>
            <div>
              <div style={{ color: "#6b7280", fontSize: 13 }}>{t("dashboard.held")}</div>
              <h2 style={{ margin: 0, color: "#d97706" }}>
                <MoneyDisplay fen={wallet.data.held_total} />
              </h2>
              <div style={{ fontSize: 12, color: "#6b7280" }}>
                {wallet.data.open_holds_count} 笔
              </div>
            </div>
          </div>
        )}
      </Card>
      <Card>
        <Tabs type="line">
          <Tabs.TabPane tab={t("wallet.logs")} itemKey="logs">
            {logs.isLoading ? (
              <Spin />
            ) : !logs.data?.items || logs.data.items.length === 0 ? (
              <Empty title={t("app.empty")} />
            ) : (
              <>
                <Table
                  columns={logCols}
                  dataSource={logs.data.items}
                  rowKey="id"
                  pagination={false}
                />
                <div style={{ marginTop: 12, display: "flex", justifyContent: "flex-end" }}>
                  <Pagination
                    total={logs.data.meta?.total ?? logs.data.items.length}
                    pageSize={PAGE_LIMIT}
                    currentPage={page}
                    onPageChange={setPage}
                  />
                </div>
              </>
            )}
          </Tabs.TabPane>
          <Tabs.TabPane tab={t("wallet.holds")} itemKey="holds">
            {holds.isLoading ? (
              <Spin />
            ) : !holds.data || holds.data.length === 0 ? (
              <Empty title={t("app.empty")} />
            ) : (
              <Table columns={holdCols} dataSource={holds.data} rowKey="id" pagination={false} />
            )}
          </Tabs.TabPane>
        </Tabs>
      </Card>
    </Page>
  );
}
