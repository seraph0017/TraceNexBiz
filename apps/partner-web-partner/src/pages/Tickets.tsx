// Tickets 列表 + 详情 + 新建 + 回复
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  Avatar,
  Button,
  Card,
  Empty,
  Input,
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
import { useApiToast } from "@/hooks/useApiToast";

export function Tickets(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError } = useApiToast();
  const [open, setOpen] = useState(false);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [priority, setPriority] = useState<api.Ticket["priority"]>("normal");

  const { data, isLoading } = useQuery({
    queryKey: ["partner", "tickets"],
    queryFn: () => api.listTickets(),
  });

  const createMut = useMutation({
    mutationFn: () => api.createTicket({ subject, body, priority }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "tickets"] });
      setOpen(false);
      setSubject("");
      setBody("");
      Toast.success({ content: "已提交" });
    },
    onError: showError,
  });

  const cols: ColumnProps<api.Ticket>[] = [
    { title: "#", dataIndex: "id", width: 80, render: (v: number) => <Link to={`/tickets/${v}`}>#{v}</Link> },
    { title: t("tickets.subject"), dataIndex: "subject" },
    {
      title: t("tickets.priority"),
      dataIndex: "priority",
      render: (v: api.Ticket["priority"]) => (
        <Tag color={v === "urgent" ? "red" : v === "high" ? "orange" : "grey"}>{v}</Tag>
      ),
    },
    {
      title: t("tickets.status"),
      dataIndex: "status",
      render: (v: api.Ticket["status"]) => (
        <Tag color={v === "open" ? "blue" : v === "resolved" ? "green" : "grey"}>{v}</Tag>
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
      title={t("tickets.title")}
      actions={
        <Button theme="solid" type="primary" onClick={() => setOpen(true)}>
          {t("tickets.create")}
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
        title={t("tickets.create")}
        visible={open}
        onOk={() => createMut.mutate()}
        onCancel={() => setOpen(false)}
        confirmLoading={createMut.isPending}
      >
        <div>
          <Field label={t("tickets.subject")}>
            <Input value={subject} onChange={(v) => setSubject(v)} maxLength={200} />
          </Field>
          <Field label={t("tickets.body")}>
            <TextArea value={body} onChange={(v: string) => setBody(v)} maxLength={4000} />
          </Field>
          <Field label={t("tickets.priority")}>
            <Select
              value={priority}
              onChange={(v) => setPriority(v as api.Ticket["priority"])}
              optionList={[
                { value: "low", label: "low" },
                { value: "normal", label: "normal" },
                { value: "high", label: "high" },
                { value: "urgent", label: "urgent" },
              ]}
              style={{ width: "100%" }}
            />
          </Field>
        </div>
      </Modal>
    </Page>
  );
}

export function TicketDetail(): JSX.Element {
  const { id } = useParams();
  const numId = Number(id);
  const { t } = useTranslation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { showError } = useApiToast();
  const [reply, setReply] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["partner", "ticket", numId],
    queryFn: () => api.getTicket(numId),
    enabled: Number.isFinite(numId),
  });

  const replyMut = useMutation({
    mutationFn: () => api.replyTicket(numId, reply),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "ticket", numId] });
      setReply("");
    },
    onError: showError,
  });

  if (isLoading) return <Spin />;
  if (!data) return <Empty title={t("errors.not_found")} />;

  return (
    <Page
      title={`#${data.id} ${data.subject}`}
      actions={<Button onClick={() => navigate("/tickets")}>{t("app.back")}</Button>}
    >
      <Card style={{ marginBottom: 16 }}>
        <p>
          <Tag color="grey">{data.status}</Tag>{" "}
          <Tag>{data.priority}</Tag>
        </p>
        <p style={{ whiteSpace: "pre-wrap" }}>{data.body}</p>
      </Card>
      <Card title={t("tickets.messages")}>
        {data.messages.length === 0 ? (
          <Empty title={t("app.empty")} />
        ) : (
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {data.messages.map((m: api.TicketMessage) => (
              <li
                key={m.id}
                style={{
                  display: "flex",
                  gap: 12,
                  padding: "12px 0",
                  borderBottom: "1px solid #f3f4f6",
                }}
              >
                <Avatar size="small">{m.author.slice(0, 1).toUpperCase()}</Avatar>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 12, color: "#6b7280" }}>
                    {m.author} · {new Date(m.created_at).toLocaleString()}
                  </div>
                  <div style={{ whiteSpace: "pre-wrap" }}>{m.body}</div>
                </div>
              </li>
            ))}
          </ul>
        )}
        {data.status !== "closed" && (
          <div style={{ marginTop: 12 }}>
            <TextArea
              value={reply}
              onChange={(v: string) => setReply(v)}
              placeholder={t("tickets.reply")}
              maxLength={4000}
            />
            <Button
              theme="solid"
              type="primary"
              onClick={() => replyMut.mutate()}
              loading={replyMut.isPending}
              disabled={!reply.trim()}
              style={{ marginTop: 8 }}
            >
              {t("tickets.reply")}
            </Button>
          </div>
        )}
      </Card>
    </Page>
  );
}
