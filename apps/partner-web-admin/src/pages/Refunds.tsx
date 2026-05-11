// Refunds + Red-flush
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Input, InputNumber, Modal, Select, Spin, Table, Tag, TextArea } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { listRefunds, createRefund, reviewRefund, listRedFlush, redFlushInvoice, type Refund, type RedFlushInvoice } from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function Refunds(): JSX.Element {
  const { t } = useTranslation();
  const { showError, showSuccess } = useApiToast();
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "refunds"],
    queryFn: listRefunds,
  });
  const [open, setOpen] = useState(false);
  const [originKind, setOriginKind] = useState<"topup" | "settlement">("topup");
  const [originId, setOriginId] = useState<number>(0);
  const [amount, setAmount] = useState<number>(0);
  const [reason, setReason] = useState("");

  const createMut = useMutation({
    mutationFn: createRefund,
    onSuccess: () => {
      setOpen(false);
      showSuccess(t("refunds.create"));
      void qc.invalidateQueries({ queryKey: ["admin", "refunds"] });
    },
    onError: showError,
  });

  const reviewMut = useMutation({
    mutationFn: ({ id, approve, note }: { id: number; approve: boolean; note?: string }) => reviewRefund(id, approve, note),
    onSuccess: () => {
      showSuccess(t("app.confirm"));
      void qc.invalidateQueries({ queryKey: ["admin", "refunds"] });
    },
    onError: showError,
  });

  return (
    <Page
      title={t("refunds.title")}
      actions={
        <Button theme="solid" onClick={() => setOpen(true)}>
          {t("refunds.create")}
        </Button>
      }
    >
      <Banner type="warning" description={t("refunds.saga_trigger_warn")} closeIcon={null} />
      {isLoading ? (
        <Spin />
      ) : (
        <Card style={{ marginTop: 12 }}>
          <Table<Refund>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id" },
              { title: "Origin", render: (_, r: Refund) => `${r.origin_kind} #${r.origin_id}` },
              { title: t("refunds.amount"), dataIndex: "amount", render: (v: number) => <MoneyDisplay fen={v} /> },
              { title: t("refunds.reason"), dataIndex: "reason" },
              { title: "Status", dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              {
                title: "",
                render: (_, r: Refund) =>
                  (r.status === "submitted" || r.status === "reviewing") && (
                    <>
                      <Button size="small" onClick={() => reviewMut.mutate({ id: r.id, approve: true })}>
                        {t("refunds.review_approve")}
                      </Button>
                      <Button
                        size="small"
                        type="danger"
                        onClick={() => reviewMut.mutate({ id: r.id, approve: false, note: "rejected" })}
                      >
                        {t("refunds.review_reject")}
                      </Button>
                    </>
                  ),
              },
            ]}
          />
        </Card>
      )}
      <Modal
        title={t("refunds.create")}
        visible={open}
        onCancel={() => setOpen(false)}
        onOk={() =>
          createMut.mutate({
            origin_kind: originKind,
            origin_id: originId,
            amount: amount * 100,
            reason,
          })
        }
        confirmLoading={createMut.isPending}
      >
        <Field label="Origin">
          <Select value={originKind} onChange={(v) => setOriginKind(v as "topup" | "settlement")}>
            <Select.Option value="topup">topup</Select.Option>
            <Select.Option value="settlement">settlement</Select.Option>
          </Select>
        </Field>
        <Field label="Origin ID">
          <InputNumber value={originId} onChange={(v) => setOriginId(typeof v === "number" ? v : 0)} aria-label="origin-id" />
        </Field>
        <Field label={t("refunds.amount")}>
          <InputNumber value={amount} onChange={(v) => setAmount(typeof v === "number" ? v : 0)} min={0} aria-label="amount" />
        </Field>
        <Field label={t("refunds.reason")}>
          <Input value={reason} onChange={setReason} aria-label="reason" />
        </Field>
      </Modal>
    </Page>
  );
}

export function RedFlush(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "red-flush"],
    queryFn: listRedFlush,
  });

  const mut = useMutation({
    mutationFn: ({ id, code, text }: { id: number; code: string; text: string }) =>
      redFlushInvoice(id, { reason_code: code, reason_text: text }),
    onSuccess: () => {
      showSuccess(t("red_flush.title"));
      void qc.invalidateQueries({ queryKey: ["admin", "red-flush"] });
    },
    onError: showError,
  });

  const [active, setActive] = useState<number | null>(null);
  const [code, setCode] = useState("");
  const [text, setText] = useState("");

  return (
    <Page title={t("red_flush.title")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<RedFlushInvoice>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id" },
              { title: "Origin", dataIndex: "origin_invoice_id" },
              { title: "Amount", dataIndex: "amount", render: (v: number) => <MoneyDisplay fen={v} /> },
              { title: "Reason", dataIndex: "reason_code" },
              { title: "Status", dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              {
                title: "",
                render: (_, r: RedFlushInvoice) =>
                  r.status === "applying" && (
                    <Button size="small" onClick={() => setActive(r.id)}>
                      {t("red_flush.review")}
                    </Button>
                  ),
              },
            ]}
          />
        </Card>
      )}
      <Modal
        title={t("red_flush.review")}
        visible={active !== null}
        onCancel={() => setActive(null)}
        onOk={() => {
          if (active !== null) {
            mut.mutate({ id: active, code, text });
            setActive(null);
          }
        }}
        confirmLoading={mut.isPending}
      >
        <Field label={t("red_flush.reason_code")}>
          <Input value={code} onChange={setCode} aria-label="reason-code" />
        </Field>
        <Field label="Reason text">
          <TextArea value={text} onChange={setText} rows={3} aria-label="reason-text" />
        </Field>
      </Modal>
    </Page>
  );
}
