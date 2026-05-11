// 邀请码管理 —— 列表 + 生成 + 失效 + 二维码
import { useMemo, useState } from "react";
import {
  Button,
  ButtonGroup,
  Card,
  Empty,
  Input,
  InputNumber,
  Modal,
  Select,
  Spin,
  Table,
  Tag,
  Toast,
  Typography,
} from "@douyinfe/semi-ui";
import type { ColumnProps } from "@douyinfe/semi-ui/lib/es/table";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { useApiToast } from "@/hooks/useApiToast";

export function Invitations(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [createOpen, setCreateOpen] = useState(false);
  const [type, setType] = useState<api.Invitation["type"]>("permanent");
  const [usageLimit, setUsageLimit] = useState(10);
  const [qrFor, setQrFor] = useState<string | null>(null);

  const { data: items, isLoading } = useQuery({
    queryKey: ["partner", "invitations"],
    queryFn: () => api.listInvitations(),
  });

  const createMut = useMutation({
    mutationFn: () =>
      api.createInvitation({
        type,
        usage_limit: type === "limited" ? usageLimit : undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "invitations"] });
      setCreateOpen(false);
      showSuccess("已生成");
    },
    onError: showError,
  });

  const revokeMut = useMutation({
    mutationFn: (id: number) => api.revokeInvitation(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "invitations"] });
    },
    onError: showError,
  });

  const columns: ColumnProps<api.Invitation>[] = useMemo(
    () => [
      { title: "ID", dataIndex: "id", width: 60 },
      {
        title: "Code",
        dataIndex: "code",
        render: (v: string) => <code>{v}</code>,
      },
      {
        title: t("invitations.title"),
        dataIndex: "type",
        render: (v: api.Invitation["type"]) => (
          <Tag>{t(`invitations.type_${v}`)}</Tag>
        ),
      },
      {
        title: t("invitations.used"),
        render: (_, r: api.Invitation) =>
          `${r.used_count}${r.usage_limit ? ` / ${r.usage_limit}` : ""}`,
      },
      {
        title: t("customers.col_status"),
        dataIndex: "status",
        render: (v: api.Invitation["status"]) => (
          <Tag color={v === "active" ? "green" : v === "revoked" ? "red" : "grey"}>{v}</Tag>
        ),
      },
      {
        title: t("customers.col_created"),
        dataIndex: "created_at",
        render: (v: string) => new Date(v).toLocaleDateString(),
      },
      {
        title: "",
        width: 220,
        render: (_, r: api.Invitation) => (
          <ButtonGroup>
            <Button
              size="small"
              onClick={() => {
                navigator.clipboard.writeText(r.code).then(
                  () => Toast.success({ content: "已复制" }),
                  () => Toast.error({ content: "复制失败" }),
                );
              }}
            >
              复制
            </Button>
            <Button size="small" onClick={() => setQrFor(r.code)}>
              {t("invitations.qr")}
            </Button>
            {r.status === "active" && (
              <Button
                size="small"
                type="danger"
                onClick={() => revokeMut.mutate(r.id)}
                loading={revokeMut.isPending}
              >
                {t("invitations.revoke")}
              </Button>
            )}
          </ButtonGroup>
        ),
      },
    ],
    [revokeMut, t],
  );

  return (
    <Page
      title={t("invitations.title")}
      actions={
        <Button
          theme="solid"
          type="primary"
          onClick={() => setCreateOpen(true)}
        >
          {t("invitations.create")}
        </Button>
      }
    >
      <Card>
        {isLoading ? (
          <Spin />
        ) : !items || items.length === 0 ? (
          <Empty title={t("app.empty")} />
        ) : (
          <Table columns={columns} dataSource={items} rowKey="id" pagination={false} />
        )}
      </Card>
      <Modal
        title={t("invitations.create")}
        visible={createOpen}
        onOk={() => createMut.mutate()}
        onCancel={() => setCreateOpen(false)}
        confirmLoading={createMut.isPending}
      >
        <div>
          <Field label={t("invitations.title")}>
            <Select
              value={type}
              onChange={(v) => setType(v as api.Invitation["type"])}
              optionList={[
                { value: "permanent", label: t("invitations.type_permanent") },
                { value: "one_time", label: t("invitations.type_one_time") },
                { value: "limited", label: t("invitations.type_limited") },
              ]}
              style={{ width: "100%" }}
            />
          </Field>
          {type === "limited" && (
            <Field label={t("invitations.limit")}>
              <InputNumber
                value={usageLimit}
                onChange={(v) => setUsageLimit(Number(v) || 1)}
                min={1}
                max={1000}
                style={{ width: "100%" }}
              />
            </Field>
          )}
        </div>
      </Modal>
      <Modal
        title={t("invitations.qr")}
        visible={qrFor !== null}
        onOk={() => setQrFor(null)}
        onCancel={() => setQrFor(null)}
        footer={null}
      >
        {qrFor && (
          <div style={{ textAlign: "center" }}>
            <img
              alt="invitation QR"
              width={240}
              height={240}
              src={`https://api.qrserver.com/v1/create-qr-code/?size=240x240&data=${encodeURIComponent(qrFor)}`}
            />
            <Input value={qrFor} readOnly style={{ marginBottom: 8 }} />
            <Typography.Paragraph copyable={{ content: qrFor }}>
              <code>{qrFor}</code>
            </Typography.Paragraph>
          </div>
        )}
      </Modal>
    </Page>
  );
}
