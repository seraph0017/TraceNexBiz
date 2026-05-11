// Tickets 列表 + 提交
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Card, Input, Modal, Select, Spin, Table, Tag, TextArea } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import {
  listTickets,
  getTicket,
  createTicket,
  replyTicket,
  type Ticket,
} from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

export function Tickets(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [open, setOpen] = useState(false);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [target, setTarget] = useState<"platform" | "partner">("partner");

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "tickets"],
    queryFn: listTickets,
  });

  const mut = useMutation({
    mutationFn: createTicket,
    onSuccess: () => {
      setOpen(false);
      setSubject("");
      setBody("");
      showSuccess(t("app.submit"));
      void qc.invalidateQueries({ queryKey: ["customer", "tickets"] });
    },
    onError: showError,
  });

  return (
    <Page
      title={t("tickets.title")}
      actions={
        <Button theme="solid" type="primary" onClick={() => setOpen(true)}>
          {t("tickets.create")}
        </Button>
      }
    >
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<Ticket>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id", render: (v) => <Link to={`/tickets/${v}`}>#{v}</Link> },
              { title: t("tickets.subject"), dataIndex: "subject" },
              {
                title: t("tickets.target"),
                dataIndex: "target",
                render: (v: string) => t(v === "platform" ? "tickets.target_platform" : "tickets.target_partner"),
              },
              {
                title: t("tickets.status"),
                dataIndex: "status",
                render: (v: string) => <Tag>{v}</Tag>,
              },
              { title: t("tickets.priority"), dataIndex: "priority" },
              { title: "", dataIndex: "updated_at" },
            ]}
          />
        </Card>
      )}

      <Modal
        title={t("tickets.create")}
        visible={open}
        onCancel={() => setOpen(false)}
        onOk={() => mut.mutate({ subject, body, target, priority: "normal" })}
        confirmLoading={mut.isPending}
      >
        <Field label={t("tickets.target")}>
          <Select value={target} onChange={(v) => setTarget(v as "platform" | "partner")}>
            <Select.Option value="partner">{t("tickets.target_partner")}</Select.Option>
            <Select.Option value="platform">{t("tickets.target_platform")}</Select.Option>
          </Select>
        </Field>
        <Field label={t("tickets.subject")}>
          <Input value={subject} onChange={setSubject} aria-label="subject" />
        </Field>
        <Field label={t("tickets.body")}>
          <TextArea value={body} onChange={setBody} rows={5} aria-label="body" />
        </Field>
      </Modal>
    </Page>
  );
}

export function TicketDetail(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const tid = Number(id ?? 0);
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [body, setBody] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "ticket", tid],
    queryFn: () => getTicket(tid),
    enabled: tid > 0,
  });

  const replyMut = useMutation({
    mutationFn: (b: string) => replyTicket(tid, b),
    onSuccess: () => {
      setBody("");
      showSuccess(t("tickets.reply"));
      void qc.invalidateQueries({ queryKey: ["customer", "ticket", tid] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;
  return (
    <Page title={`#${data.id} ${data.subject}`} description={<Tag>{data.status}</Tag>}>
      <Card title={t("tickets.messages")}>
        <ul style={{ listStyle: "none", padding: 0, display: "flex", flexDirection: "column", gap: 12 }}>
          <li style={{ background: "#f3f4f6", padding: 12, borderRadius: 6 }}>
            <strong>{data.id}</strong>
            <p style={{ whiteSpace: "pre-wrap" }}>{data.body}</p>
          </li>
          {data.messages.map((m) => (
            <li key={m.id} style={{ background: m.author === "customer" ? "#eff6ff" : "#f9fafb", padding: 12, borderRadius: 6 }}>
              <strong>{m.author}</strong>
              <span style={{ marginLeft: 8, color: "#9ca3af", fontSize: 12 }}>{m.created_at}</span>
              <p style={{ whiteSpace: "pre-wrap" }}>{m.body}</p>
            </li>
          ))}
        </ul>
      </Card>
      {data.status !== "closed" && (
        <Card title={t("tickets.reply")} style={{ marginTop: 12 }}>
          <TextArea value={body} onChange={setBody} rows={4} aria-label="reply" />
          <Button
            theme="solid"
            type="primary"
            style={{ marginTop: 8 }}
            loading={replyMut.isPending}
            disabled={!body.trim()}
            onClick={() => replyMut.mutate(body)}
          >
            {t("tickets.reply")}
          </Button>
        </Card>
      )}
    </Page>
  );
}
