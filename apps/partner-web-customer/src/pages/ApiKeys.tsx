// API Keys 管理 —— 客户自己 sk-key；不展示渠道商维度 sk-key（M3-03 不变量）
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Modal, Input, Spin, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { listApiKeys, createApiKey, revokeApiKey, type ApiKey, type NewApiKeyResp } from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

export function ApiKeys(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [revealed, setRevealed] = useState<NewApiKeyResp | null>(null);

  const { data: items, isLoading } = useQuery({
    queryKey: ["customer", "api-keys"],
    queryFn: listApiKeys,
  });

  const createMut = useMutation({
    mutationFn: createApiKey,
    onSuccess: (res) => {
      setRevealed(res);
      setCreating(false);
      setName("");
      void qc.invalidateQueries({ queryKey: ["customer", "api-keys"] });
    },
    onError: showError,
  });

  const revokeMut = useMutation({
    mutationFn: revokeApiKey,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["customer", "api-keys"] });
      showSuccess(t("app.confirm"));
    },
    onError: showError,
  });

  return (
    <Page
      title={t("api_keys.title")}
      actions={
        <Button theme="solid" type="primary" onClick={() => setCreating(true)}>
          {t("api_keys.create")}
        </Button>
      }
    >
      <Banner type="info" description={t("api_keys.intro")} closeIcon={null} style={{ marginBottom: 12 }} />
      <Banner type="warning" description={t("api_keys.no_partner_keys")} closeIcon={null} style={{ marginBottom: 12 }} />
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<ApiKey>
            dataSource={items ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: t("api_keys.name"), dataIndex: "name" },
              { title: t("api_keys.prefix"), dataIndex: "prefix", render: (v: string) => <code>{v}…</code> },
              { title: t("api_keys.last_used"), dataIndex: "last_used_at", render: (v) => v ?? "—" },
              {
                title: t("api_keys.status"),
                dataIndex: "status",
                render: (v: string) => (
                  <Tag color={v === "active" ? "green" : "grey"}>{v}</Tag>
                ),
              },
              {
                title: "",
                render: (_, row: ApiKey) =>
                  row.status === "active" && (
                    <Button
                      type="danger"
                      size="small"
                      onClick={() => {
                        Modal.confirm({
                          title: t("api_keys.revoke"),
                          content: t("api_keys.revoke_confirm"),
                          onOk: () => revokeMut.mutate(row.id),
                        });
                      }}
                    >
                      {t("api_keys.revoke")}
                    </Button>
                  ),
              },
            ]}
          />
        </Card>
      )}

      <Modal
        title={t("api_keys.create")}
        visible={creating}
        onCancel={() => setCreating(false)}
        onOk={() => createMut.mutate(name)}
        confirmLoading={createMut.isPending}
      >
        <Field label={t("api_keys.name")}>
          <Input value={name} onChange={setName} aria-label="key-name" />
        </Field>
      </Modal>

      <Modal
        title={t("api_keys.raw_label")}
        visible={!!revealed}
        onOk={() => setRevealed(null)}
        onCancel={() => setRevealed(null)}
        cancelButtonProps={{ style: { display: "none" } }}
        closeOnEsc={false}
        maskClosable={false}
      >
        <Banner type="warning" description={t("api_keys.raw_warn")} closeIcon={null} />
        <pre style={{ background: "#f3f4f6", padding: 12, marginTop: 12, overflowX: "auto" }}>
          {revealed?.raw_key}
        </pre>
        <Button
          onClick={async () => {
            if (revealed?.raw_key) {
              await navigator.clipboard.writeText(revealed.raw_key);
              showSuccess(t("api_keys.copied"));
            }
          }}
        >
          {t("api_keys.copy")}
        </Button>
      </Modal>
    </Page>
  );
}
