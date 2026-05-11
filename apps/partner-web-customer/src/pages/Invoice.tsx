// Invoice 列表 + 申请 + 详情 + 红冲（PRD §7.8 + M8）
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Input, InputNumber, Modal, Select, Spin, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { listInvoices, getInvoice, applyInvoice, applyRedFlush, type Invoice } from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

const RED_FLUSH_REASONS = ["wrong_title", "wrong_amount", "service_terminated", "buyer_request"];

export function Invoices(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState<number>(0);
  const [title, setTitle] = useState("");
  const [taxNo, setTaxNo] = useState("");
  const [email, setEmail] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "invoices"],
    queryFn: listInvoices,
  });

  const mut = useMutation({
    mutationFn: applyInvoice,
    onSuccess: () => {
      setOpen(false);
      showSuccess(t("app.submit"));
      void qc.invalidateQueries({ queryKey: ["customer", "invoices"] });
    },
    onError: showError,
  });

  return (
    <Page
      title={t("invoice.title")}
      actions={
        <Button theme="solid" type="primary" onClick={() => setOpen(true)}>
          {t("invoice.apply")}
        </Button>
      }
    >
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<Invoice>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id", render: (v) => <Link to={`/invoice/${v}`}>#{v}</Link> },
              {
                title: t("invoice.amount"),
                dataIndex: "amount",
                render: (v: number) => <MoneyDisplay fen={v} />,
              },
              {
                title: "Type",
                dataIndex: "type",
                render: (v: string) => <Tag color={v === "red" ? "red" : "blue"}>{t(v === "red" ? "invoice.type_red" : "invoice.type_blue")}</Tag>,
              },
              {
                title: "Status",
                dataIndex: "status",
                render: (v: string) => <Tag>{t(`invoice.status_${v}`)}</Tag>,
              },
              { title: t("invoice.title_field"), dataIndex: "title" },
              { title: t("invoice.tax_no"), dataIndex: "tax_no" },
              { title: "", dataIndex: "applied_at" },
            ]}
          />
        </Card>
      )}

      <Modal
        title={t("invoice.apply")}
        visible={open}
        onCancel={() => setOpen(false)}
        onOk={() => mut.mutate({ amount: amount * 100, title, tax_no: taxNo, email: email || undefined })}
        confirmLoading={mut.isPending}
      >
        <Field label={t("invoice.amount")}>
          <InputNumber value={amount} onChange={(v) => setAmount(typeof v === "number" ? v : 0)} min={0} aria-label="amount" />
        </Field>
        <Field label={t("invoice.title_field")}>
          <Input value={title} onChange={setTitle} aria-label="title" />
        </Field>
        <Field label={t("invoice.tax_no")}>
          <Input value={taxNo} onChange={setTaxNo} aria-label="tax-no" />
        </Field>
        <Field label={t("invoice.email")}>
          <Input value={email} onChange={setEmail} aria-label="email" />
        </Field>
      </Modal>
    </Page>
  );
}

export function InvoiceDetail(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const iid = Number(id ?? 0);
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState<string>(RED_FLUSH_REASONS[0] ?? "");
  const [reasonText, setReasonText] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "invoice", iid],
    queryFn: () => getInvoice(iid),
    enabled: iid > 0,
  });

  const mut = useMutation({
    mutationFn: () => applyRedFlush(iid, reason, reasonText || undefined),
    onSuccess: () => {
      showSuccess(t("invoice.red_flush"));
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["customer", "invoice", iid] });
      void qc.invalidateQueries({ queryKey: ["customer", "invoices"] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page
      title={`#${data.id}`}
      actions={
        data.status === "issued" && data.type === "blue" && (
          <Button onClick={() => setOpen(true)}>{t("invoice.red_flush")}</Button>
        )
      }
    >
      {data.type === "red" && data.origin_invoice_id && (
        <Banner
          type="warning"
          description={`Red-flush of #${data.origin_invoice_id}`}
          closeIcon={null}
        />
      )}
      <Card>
        <p>
          {t("invoice.amount")}: <MoneyDisplay fen={data.amount} />
        </p>
        <p>
          {t("invoice.title_field")}: {data.title}
        </p>
        <p>
          {t("invoice.tax_no")}: {data.tax_no}
        </p>
        <p>
          Status: <Tag>{t(`invoice.status_${data.status}`)}</Tag>
        </p>
        {data.pdf_url && (
          <a href={data.pdf_url} download>
            {t("invoice.download_pdf")}
          </a>
        )}
      </Card>
      <Modal
        title={t("invoice.red_flush")}
        visible={open}
        onCancel={() => setOpen(false)}
        onOk={() => mut.mutate()}
        confirmLoading={mut.isPending}
      >
        <Field label={t("invoice.red_flush_reason")}>
          <Select value={reason} onChange={(v) => setReason(v as string)} style={{ width: "100%" }}>
            {RED_FLUSH_REASONS.map((r) => (
              <Select.Option key={r} value={r}>
                {r}
              </Select.Option>
            ))}
          </Select>
        </Field>
        <Field label={t("invoice.red_flush_reason_text")}>
          <Input value={reasonText} onChange={setReasonText} aria-label="reason-text" />
        </Field>
      </Modal>
    </Page>
  );
}
