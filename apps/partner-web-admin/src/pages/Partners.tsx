// Partners CRUD + 终止（场景 I 30 天宽限管理）
import { useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Descriptions, Input, Modal, Spin, Table, Tag, TextArea } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { listPartners, getPartner, createPartner, terminatePartner, type Partner } from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function Partners(): JSX.Element {
  const { t } = useTranslation();
  const { showError, showSuccess } = useApiToast();
  const qc = useQueryClient();
  const [page, setPage] = useState(1);
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "partners", page, q],
    queryFn: () => listPartners({ page, limit: 50, q }),
    placeholderData: (prev) => prev,
  });

  const mut = useMutation({
    mutationFn: createPartner,
    onSuccess: () => {
      setOpen(false);
      setName("");
      setEmail("");
      setPhone("");
      showSuccess(t("app.create"));
      void qc.invalidateQueries({ queryKey: ["admin", "partners"] });
    },
    onError: showError,
  });

  return (
    <Page
      title={t("partners.title")}
      actions={
        <>
          <Input value={q} onChange={setQ} placeholder={t("app.search")} style={{ width: 200 }} />
          <Button theme="solid" type="primary" onClick={() => setOpen(true)}>
            {t("partners.create")}
          </Button>
        </>
      }
    >
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<Partner>
            dataSource={data?.items ?? []}
            rowKey="id"
            pagination={{
              currentPage: page,
              pageSize: 50,
              total: data?.meta?.total ?? 0,
              onPageChange: setPage,
            }}
            columns={[
              { title: t("partners.col_id"), dataIndex: "id", render: (v) => <Link to={`/partners/${v}`}>#{v}</Link> },
              { title: t("partners.col_name"), dataIndex: "display_name" },
              { title: t("partners.col_email"), dataIndex: "contact_email_masked" },
              { title: t("partners.col_status"), dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              { title: t("partners.col_kyc"), dataIndex: "kyc_status" },
              { title: t("partners.col_terminated"), dataIndex: "terminated_at", render: (v) => v ?? "—" },
              { title: t("partners.col_grace"), dataIndex: "grace_period_ends_at", render: (v) => v ?? "—" },
            ]}
          />
        </Card>
      )}
      <Modal
        title={t("partners.create")}
        visible={open}
        onCancel={() => setOpen(false)}
        onOk={() => mut.mutate({ contact_name: name, contact_email: email, contact_phone: phone })}
        confirmLoading={mut.isPending}
      >
        <Field label="Name">
          <Input value={name} onChange={setName} aria-label="name" />
        </Field>
        <Field label="Email">
          <Input value={email} onChange={setEmail} aria-label="email" />
        </Field>
        <Field label="Phone">
          <Input value={phone} onChange={setPhone} aria-label="phone" />
        </Field>
      </Modal>
    </Page>
  );
}

export function PartnerNew(): JSX.Element {
  const navigate = useNavigate();
  // 直接 redirect to partners with modal open；保留路由位
  return (
    <Page title="">
      <Button onClick={() => navigate("/partners")}>{`Open ${"create dialog"}`}</Button>
    </Page>
  );
}

export function PartnerDetailPage(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const pid = Number(id ?? 0);
  const { showError, showSuccess } = useApiToast();
  const qc = useQueryClient();
  const [termOpen, setTermOpen] = useState(false);
  const [reason, setReason] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "partner", pid],
    queryFn: () => getPartner(pid),
    enabled: pid > 0,
  });

  const mut = useMutation({
    mutationFn: () => terminatePartner(pid, reason),
    onSuccess: () => {
      setTermOpen(false);
      showSuccess(t("partners.terminate"));
      void qc.invalidateQueries({ queryKey: ["admin", "partner", pid] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page
      title={`#${data.id} ${data.display_name}`}
      actions={
        data.status === "active" && (
          <Button type="danger" onClick={() => setTermOpen(true)}>
            {t("partners.terminate")}
          </Button>
        )
      }
    >
      <Card>
        <Descriptions
          data={[
            { key: "Status", value: <Tag>{data.status}</Tag> },
            { key: "Email", value: data.contact_email_masked },
            { key: "Phone", value: data.contact_phone_masked },
            { key: "Bank", value: data.bank_account_masked },
            { key: "Customers", value: data.customers_count },
            { key: "Monthly gross", value: <MoneyDisplay fen={data.monthly_gross} /> },
            { key: "Monthly net", value: <MoneyDisplay fen={data.monthly_net} /> },
            { key: t("partners.col_terminated"), value: data.terminated_at ?? "—" },
            { key: t("partners.col_grace"), value: data.grace_period_ends_at ?? "—" },
          ]}
        />
      </Card>
      <Modal
        title={t("partners.terminate")}
        visible={termOpen}
        onCancel={() => setTermOpen(false)}
        onOk={() => mut.mutate()}
        confirmLoading={mut.isPending}
        okType="danger"
      >
        <Banner type="warning" description={t("partners.terminate_warn")} closeIcon={null} />
        <Field label={t("partners.terminate_reason")}>
          <TextArea value={reason} onChange={setReason} rows={3} aria-label="reason" />
        </Field>
      </Modal>
    </Page>
  );
}
