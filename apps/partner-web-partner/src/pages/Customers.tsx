// 客户列表 —— Table + 筛选 + 分页 + 导出（脱敏 CSV）
import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Button,
  ButtonGroup,
  Empty,
  Input,
  Pagination,
  Select,
  Spin,
  Table,
  Tag,
} from "@douyinfe/semi-ui";
import type { ColumnProps } from "@douyinfe/semi-ui/lib/es/table";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { MoneyDisplay } from "@/components/MoneyDisplay";

const PAGE_LIMIT = 20;

export function Customers(): JSX.Element {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const [q, setQ] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["partner", "customers", { page, status, q }],
    queryFn: () => api.listCustomers({ page, limit: PAGE_LIMIT, status, q }),
    placeholderData: (prev) => prev,
  });

  const items = data?.items ?? [];
  const total = data?.meta?.total ?? items.length;

  const columns: ColumnProps<api.CustomerListItem>[] = [
    { title: t("customers.col_id"), dataIndex: "id", width: 80 },
    {
      title: t("customers.col_name"),
      dataIndex: "display_name",
      render: (v: string, r: api.CustomerListItem) => <Link to={`/customers/${r.id}`}>{v}</Link>,
    },
    { title: t("customers.col_email"), dataIndex: "email_masked" },
    {
      title: t("customers.col_status"),
      dataIndex: "status",
      render: (v: string) => <Tag color={v === "active" ? "green" : "grey"}>{v}</Tag>,
    },
    { title: t("customers.col_calls"), dataIndex: "monthly_calls", align: "right" as const },
    {
      title: t("customers.col_remaining"),
      dataIndex: "remaining_quota",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: t("customers.col_created"),
      dataIndex: "created_at",
      render: (v: string) => new Date(v).toLocaleDateString(),
    },
  ];

  const onExport = (): void => {
    // 客户端 CSV 导出（仅当前页 + 已脱敏字段）；批量导出走后端流式
    const rows: (string | number)[][] = [
      ["id", "name", "email_masked", "status", "monthly_calls", "remaining_quota_fen", "created_at"],
      ...items.map((r: api.CustomerListItem) => [
        r.id,
        r.display_name,
        r.email_masked,
        r.status,
        r.monthly_calls,
        r.remaining_quota,
        r.created_at,
      ]),
    ];
    const csv = rows.map((r) => r.map((c: string | number) => `"${String(c).replace(/"/g, '""')}"`).join(",")).join("\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `customers_p${page}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Page
      title={t("customers.title")}
      actions={
        <ButtonGroup>
          <Button onClick={onExport} disabled={items.length === 0}>
            {t("app.export")}
          </Button>
          <Link to="/customers/new">
            <Button theme="solid" type="primary">
              {t("customers.new")}
            </Button>
          </Link>
        </ButtonGroup>
      }
    >
      <div style={{ display: "flex", gap: 12, marginBottom: 12, flexWrap: "wrap" }}>
        <Input
          placeholder={t("customers.filter_q")}
          value={q}
          onChange={(v) => setQ(v)}
          style={{ width: 240 }}
          showClear
          onEnterPress={() => setPage(1)}
        />
        <Select
          placeholder={t("customers.filter_status")}
          value={status}
          onChange={(v) => {
            setStatus(typeof v === "string" ? v : undefined);
            setPage(1);
          }}
          style={{ width: 160 }}
          showClear
          optionList={[
            { value: "active", label: "active" },
            { value: "orphaned", label: "orphaned" },
            { value: "suspended", label: "suspended" },
          ]}
        />
      </div>
      {isLoading ? (
        <Spin />
      ) : items.length === 0 ? (
        <Empty title={t("app.empty")} />
      ) : (
        <>
          <Table columns={columns} dataSource={items} rowKey="id" pagination={false} />
          <div style={{ marginTop: 12, display: "flex", justifyContent: "flex-end" }}>
            <Pagination total={total} pageSize={PAGE_LIMIT} currentPage={page} onPageChange={setPage} />
          </div>
        </>
      )}
    </Page>
  );
}
