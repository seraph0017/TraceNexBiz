// Disputes 列表 + 提交（场景 K，frontend §3.2）
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  Button,
  Card,
  Empty,
  Input,
  InputNumber,
  Modal,
  Select,
  Spin,
  Table,
  Tag,
  TextArea,
  Toast,
} from "@douyinfe/semi-ui";
import type { ColumnProps } from "@douyinfe/semi-ui/lib/es/table";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { useApiToast } from "@/hooks/useApiToast";

export function Disputes(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError } = useApiToast();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<api.Dispute["account_kind"]>("fy_account");
  const [yuan, setYuan] = useState(0);
  const [reason, setReason] = useState("");
  const [evidence, setEvidence] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["partner", "disputes"],
    queryFn: () => api.listDisputes(),
  });

  const createMut = useMutation({
    mutationFn: () =>
      api.createDispute({
        account_kind: kind,
        amount: Math.round(yuan * 100),
        reason,
        evidence_url: evidence || undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "disputes"] });
      setOpen(false);
      setReason("");
      setEvidence("");
      Toast.success({ content: t("disputes.submitted") });
    },
    onError: showError,
  });

  const cols: ColumnProps<api.Dispute>[] = [
    { title: "ID", dataIndex: "id", width: 80, render: (v: number) => <Link to={`/disputes/${v}`}>#{v}</Link> },
    {
      title: t("disputes.kind"),
      dataIndex: "account_kind",
      render: (v: api.Dispute["account_kind"]) =>
        v === "fy_account" ? t("disputes.kind_fy_account") : t("disputes.kind_tn_account"),
    },
    {
      title: t("disputes.amount"),
      dataIndex: "amount",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: t("customers.col_status"),
      dataIndex: "status",
      render: (v: api.Dispute["status"]) => (
        <Tag color={v === "accepted" ? "green" : v === "rejected" ? "red" : "blue"}>{v}</Tag>
      ),
    },
    {
      title: t("customers.col_created"),
      dataIndex: "created_at",
      render: (v: string) => new Date(v).toLocaleDateString(),
    },
  ];

  return (
    <Page
      title={t("disputes.title")}
      actions={
        <Button theme="solid" type="primary" onClick={() => setOpen(true)}>
          {t("disputes.create")}
        </Button>
      }
    >
      <Card>
        {isLoading ? (
          <Spin />
        ) : !data || data.length === 0 ? (
          <Empty title={t("app.empty")} />
        ) : (
          <Table columns={cols} dataSource={data} rowKey="id" pagination={false} />
        )}
      </Card>
      <Modal
        title={t("disputes.create")}
        visible={open}
        onOk={() => createMut.mutate()}
        onCancel={() => setOpen(false)}
        confirmLoading={createMut.isPending}
      >
        <div>
          <Field label={t("disputes.kind")}>
            <Select
              value={kind}
              onChange={(v) => setKind(v as api.Dispute["account_kind"])}
              optionList={[
                { value: "fy_account", label: t("disputes.kind_fy_account") },
                { value: "tn_account", label: t("disputes.kind_tn_account") },
              ]}
              style={{ width: "100%" }}
            />
          </Field>
          <Field label={t("disputes.amount")}>
            <InputNumber
              value={yuan}
              onChange={(v) => setYuan(Number(v) || 0)}
              min={0.01}
              precision={2}
              style={{ width: "100%" }}
            />
          </Field>
          <Field label={t("disputes.reason")}>
            <TextArea value={reason} onChange={(v: string) => setReason(v)} maxLength={1000} />
          </Field>
          <Field label={t("disputes.evidence")}>
            <Input value={evidence} onChange={(v) => setEvidence(v)} />
          </Field>
        </div>
      </Modal>
    </Page>
  );
}

export function DisputeDetail(): JSX.Element {
  const { id } = useParams();
  const numId = Number(id);
  const navigate = useNavigate();
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ["partner", "dispute", numId],
    queryFn: () => api.getDispute(numId),
    enabled: Number.isFinite(numId),
  });
  if (isLoading) return <Spin />;
  if (!data) return <Empty title={t("errors.not_found")} />;
  return (
    <Page
      title={`${t("disputes.title")} #${data.id}`}
      actions={<Button onClick={() => navigate("/disputes")}>{t("app.back")}</Button>}
    >
      <Card>
        <p>
          <strong>{t("disputes.kind")}：</strong>
          {data.account_kind === "fy_account" ? t("disputes.kind_fy_account") : t("disputes.kind_tn_account")}
        </p>
        <p>
          <strong>{t("disputes.amount")}：</strong>
          <MoneyDisplay fen={data.amount} />
        </p>
        <p>
          <strong>{t("disputes.reason")}：</strong>
          {data.reason}
        </p>
        <p>
          <strong>{t("customers.col_status")}：</strong>
          <Tag>{data.status}</Tag>
        </p>
      </Card>
    </Page>
  );
}
