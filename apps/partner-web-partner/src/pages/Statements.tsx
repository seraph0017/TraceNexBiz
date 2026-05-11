// Statements 列表 + 详情 + 申请发票
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  Button,
  Card,
  Empty,
  Spin,
  Table,
  Tag,
} from "@douyinfe/semi-ui";
import type { ColumnProps } from "@douyinfe/semi-ui/lib/es/table";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { useApiToast } from "@/hooks/useApiToast";

export function Statements(): JSX.Element {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ["partner", "statements"],
    queryFn: () => api.listStatements(),
  });
  const cols: ColumnProps<api.Statement>[] = [
    {
      title: t("statements.period"),
      dataIndex: "period",
      render: (v: string, r: api.Statement) => <Link to={`/statements/${r.id}`}>{v}</Link>,
    },
    {
      title: t("statements.gross"),
      dataIndex: "gross",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: t("statements.cost"),
      dataIndex: "cost",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: t("statements.net"),
      dataIndex: "net",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: t("statements.status"),
      dataIndex: "status",
      render: (v: api.Statement["status"]) => (
        <Tag color={v === "issued" ? "blue" : v === "paid" ? "green" : "grey"}>{v}</Tag>
      ),
    },
    {
      title: t("statements.issued_at"),
      dataIndex: "issued_at",
      render: (v?: string) => (v ? new Date(v).toLocaleDateString() : "—"),
    },
  ];
  return (
    <Page title={t("statements.title")}>
      <Card>
        {isLoading ? (
          <Spin />
        ) : !data || data.length === 0 ? (
          <Empty title={t("app.empty")} />
        ) : (
          <Table columns={cols} dataSource={data} rowKey="id" pagination={false} />
        )}
      </Card>
    </Page>
  );
}

export function StatementDetail(): JSX.Element {
  const { id } = useParams();
  const navigate = useNavigate();
  const numId = Number(id);
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();

  const { data, isLoading } = useQuery({
    queryKey: ["partner", "statement", numId],
    queryFn: () => api.getStatement(numId),
    enabled: Number.isFinite(numId),
  });

  const applyMut = useMutation({
    mutationFn: () => api.applyInvoice({ statement_id: numId, title_id: 1 }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "statement", numId] });
      showSuccess("发票申请已提交");
    },
    onError: showError,
  });

  if (isLoading) return <Spin />;
  if (!data) return <Empty title={t("errors.not_found")} />;

  const itemCols: ColumnProps<(typeof data.line_items)[number]>[] = [
    { title: "描述", dataIndex: "description" },
    { title: "数量", dataIndex: "quantity", align: "right" as const },
    {
      title: "单价",
      dataIndex: "unit_price",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: "金额",
      dataIndex: "amount",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
  ];

  return (
    <Page
      title={`${t("statements.detail")} ${data.period}`}
      actions={
        <>
          <Button onClick={() => navigate("/statements")}>{t("app.back")}</Button>
          <Button
            theme="solid"
            type="primary"
            disabled={data.status === "draft" || data.invoice_id !== undefined}
            onClick={() => applyMut.mutate()}
            loading={applyMut.isPending}
          >
            {t("statements.apply_invoice")}
          </Button>
        </>
      }
    >
      <Card style={{ marginBottom: 16 }}>
        <div style={{ display: "flex", gap: 24, flexWrap: "wrap" }}>
          <Stat label={t("statements.gross")} value={<MoneyDisplay fen={data.gross} />} />
          <Stat label={t("statements.cost")} value={<MoneyDisplay fen={data.cost} />} />
          <Stat label={t("statements.net")} value={<MoneyDisplay fen={data.net} />} />
          <Stat label={t("statements.status")} value={<Tag>{data.status}</Tag>} />
        </div>
      </Card>
      <Card title="明细">
        {data.line_items.length === 0 ? (
          <Empty title={t("app.empty")} />
        ) : (
          <Table columns={itemCols} dataSource={data.line_items} rowKey="id" pagination={false} />
        )}
      </Card>
    </Page>
  );
}

function Stat({ label, value }: { label: string; value: React.ReactNode }): JSX.Element {
  return (
    <div>
      <div style={{ color: "#6b7280", fontSize: 13 }}>{label}</div>
      <strong style={{ fontSize: 18 }}>{value}</strong>
    </div>
  );
}
