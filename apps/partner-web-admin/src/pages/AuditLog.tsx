// Audit log + verify chain（哈希链 timeline 展示）
import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Banner, Button, Card, Input, Spin, Table } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { listAudit, verifyAuditChain, type AuditEntry } from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function AuditLog(): JSX.Element {
  const { t } = useTranslation();
  const { showError, showSuccess } = useApiToast();
  const [page, setPage] = useState(1);
  const [action, setAction] = useState("");
  const [actor, setActor] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "audit", page, action, actor],
    queryFn: () => listAudit({ page, limit: 100, action: action || undefined, actor: actor || undefined }),
    placeholderData: (prev) => prev,
  });

  const verifyMut = useMutation({
    mutationFn: () => verifyAuditChain(),
    onSuccess: (res) => {
      if (res.ok) {
        showSuccess(t("audit.verify_ok", { n: res.checked }));
      } else {
        showError(new Error(t("audit.verify_broken", { id: res.broken_at ?? "?" })));
      }
    },
    onError: showError,
  });

  return (
    <Page
      title={t("audit.title")}
      actions={
        <Button onClick={() => verifyMut.mutate()} loading={verifyMut.isPending}>
          {t("audit.verify")}
        </Button>
      }
    >
      <Card style={{ marginBottom: 12 }}>
        <div style={{ display: "flex", gap: 12 }}>
          <Field label={t("audit.filter_action")}>
            <Input value={action} onChange={setAction} aria-label="filter-action" />
          </Field>
          <Field label={t("audit.filter_actor")}>
            <Input value={actor} onChange={setActor} aria-label="filter-actor" />
          </Field>
        </div>
      </Card>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<AuditEntry>
            dataSource={data?.items ?? []}
            rowKey="id"
            pagination={{
              currentPage: page,
              pageSize: 100,
              total: data?.meta?.total ?? 0,
              onPageChange: setPage,
            }}
            columns={[
              { title: t("audit.col_ts"), dataIndex: "ts" },
              { title: t("audit.col_actor"), render: (_, r: AuditEntry) => `${r.actor_kind}#${r.actor_id}` },
              { title: t("audit.col_action"), dataIndex: "action" },
              { title: t("audit.col_target"), dataIndex: "target" },
              { title: t("audit.col_result"), dataIndex: "result" },
              { title: t("audit.col_ip"), dataIndex: "ip" },
              {
                title: t("audit.col_trace"),
                dataIndex: "trace_id",
                render: (v: string) => <code style={{ fontSize: 11 }}>{v.slice(0, 8)}…</code>,
              },
            ]}
          />
        </Card>
      )}
      <Banner
        type="info"
        description="哈希链：每条记录包含 prev_hash + hash；CLI 校验工具与本面板对齐 ADR-006"
        closeIcon={null}
        style={{ marginTop: 12 }}
      />
    </Page>
  );
}
