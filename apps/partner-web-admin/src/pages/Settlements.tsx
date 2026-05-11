// Settlements 列表 + 详情 + 锁定 + 分账下发 + 回执对账 + 个税代扣
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Input, Modal, Spin, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import {
  listSettlements,
  getSettlement,
  lockSettlement,
  dispatchSettlement,
  reconcileSettlement,
  type SettlementBatch,
} from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function Settlements(): JSX.Element {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "settlements"],
    queryFn: listSettlements,
  });
  return (
    <Page title={t("settlements.title")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<SettlementBatch>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id", render: (v) => <Link to={`/settlements/${v}`}>#{v}</Link> },
              { title: t("settlements.col_period"), dataIndex: "period" },
              { title: t("settlements.col_partners"), dataIndex: "partners_count" },
              {
                title: t("settlements.col_total"),
                dataIndex: "total_amount",
                render: (v: number) => <MoneyDisplay fen={v} />,
              },
              { title: t("settlements.col_status"), dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              { title: "Locked", dataIndex: "locked_at", render: (v) => v ?? "—" },
              { title: "Dispatched", dataIndex: "dispatched_at", render: (v) => v ?? "—" },
            ]}
          />
        </Card>
      )}
    </Page>
  );
}

export function SettlementDetailPage(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const sid = Number(id ?? 0);
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [recOpen, setRecOpen] = useState(false);
  const [receiptId, setReceiptId] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "settlement", sid],
    queryFn: () => getSettlement(sid),
    enabled: sid > 0,
  });

  const lockMut = useMutation({
    mutationFn: () => lockSettlement(sid),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "settlement", sid] }),
    onError: showError,
  });
  const dispatchMut = useMutation({
    mutationFn: () => dispatchSettlement(sid),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "settlement", sid] }),
    onError: showError,
  });
  const recMut = useMutation({
    mutationFn: () => reconcileSettlement(sid, receiptId),
    onSuccess: () => {
      setRecOpen(false);
      showSuccess(t("settlements.reconcile"));
      void qc.invalidateQueries({ queryKey: ["admin", "settlement", sid] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page
      title={`Settlement #${data.id} (${data.period})`}
      actions={
        <>
          {data.status === "draft" && (
            <Button onClick={() => lockMut.mutate()} loading={lockMut.isPending}>
              {t("settlements.lock")}
            </Button>
          )}
          {data.status === "locked" && (
            <Button theme="solid" onClick={() => dispatchMut.mutate()} loading={dispatchMut.isPending}>
              {t("settlements.dispatch")}
            </Button>
          )}
          {data.status === "dispatched" && (
            <Button onClick={() => setRecOpen(true)}>{t("settlements.reconcile")}</Button>
          )}
        </>
      }
    >
      <Banner type="info" description={t("settlements.tax_withheld")} closeIcon={null} />
      <Card style={{ marginTop: 12 }}>
        <Table
          dataSource={data.rows}
          rowKey="partner_id"
          pagination={false}
          columns={[
            { title: "Partner", dataIndex: "partner_name" },
            { title: "Gross", dataIndex: "gross", render: (v: number) => <MoneyDisplay fen={v} /> },
            { title: "Cost", dataIndex: "cost", render: (v: number) => <MoneyDisplay fen={v} /> },
            { title: "Net", dataIndex: "net", render: (v: number) => <MoneyDisplay fen={v} /> },
            { title: "Tax", dataIndex: "tax", render: (v: number) => <MoneyDisplay fen={v} /> },
            { title: "Payable", dataIndex: "payable", render: (v: number) => <MoneyDisplay fen={v} /> },
          ]}
        />
        {data.receipt_url && (
          <a href={data.receipt_url} download style={{ marginTop: 12, display: "inline-block" }}>
            Receipt
          </a>
        )}
      </Card>
      <Modal
        title={t("settlements.reconcile")}
        visible={recOpen}
        onCancel={() => setRecOpen(false)}
        onOk={() => recMut.mutate()}
        confirmLoading={recMut.isPending}
      >
        <Field label={t("settlements.receipt_id")}>
          <Input value={receiptId} onChange={setReceiptId} aria-label="receipt-id" />
        </Field>
      </Modal>
    </Page>
  );
}
