// SwitchPartner —— 客户主动切换渠道商（场景 H）
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, InputNumber, Modal, Spin, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import {
  listSwitchRequests,
  submitSwitchRequest,
  confirmSwitchRequest,
  type SwitchPartnerRequest,
} from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

export function SwitchPartner(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [target, setTarget] = useState<number>(0);
  const [confirmOpen, setConfirmOpen] = useState<SwitchPartnerRequest | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "switch"],
    queryFn: listSwitchRequests,
  });

  const submitMut = useMutation({
    mutationFn: submitSwitchRequest,
    onSuccess: () => {
      showSuccess(t("app.submit"));
      void qc.invalidateQueries({ queryKey: ["customer", "switch"] });
    },
    onError: showError,
  });

  const confirmMut = useMutation({
    mutationFn: confirmSwitchRequest,
    onSuccess: () => {
      setConfirmOpen(null);
      void qc.invalidateQueries({ queryKey: ["customer", "switch"] });
    },
    onError: showError,
  });

  return (
    <Page title={t("switch_partner.title")}>
      <Banner type="info" description={t("switch_partner.intro")} closeIcon={null} />
      <Card style={{ marginTop: 12 }}>
        <Field label={t("switch_partner.target")}>
          <InputNumber value={target} onChange={(v) => setTarget(typeof v === "number" ? v : 0)} aria-label="target-partner-id" />
        </Field>
        <Button
          theme="solid"
          type="primary"
          loading={submitMut.isPending}
          disabled={!target}
          onClick={() => submitMut.mutate(target)}
        >
          {t("switch_partner.submit")}
        </Button>
      </Card>
      <Card title={t("switch_partner.list")} style={{ marginTop: 12 }}>
        {isLoading ? (
          <Spin />
        ) : (
          <Table<SwitchPartnerRequest>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id" },
              { title: "From", dataIndex: "current_partner_id" },
              { title: "To", dataIndex: "target_partner_id" },
              { title: "Status", dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              { title: "Created", dataIndex: "created_at" },
              {
                title: "",
                render: (_, row: SwitchPartnerRequest) =>
                  row.status === "approved" && (
                    <Button size="small" onClick={() => setConfirmOpen(row)}>
                      {t("switch_partner.confirm_step")}
                    </Button>
                  ),
              },
            ]}
          />
        )}
      </Card>
      <Modal
        title={t("switch_partner.confirm_step")}
        visible={!!confirmOpen}
        onCancel={() => setConfirmOpen(null)}
        onOk={() => {
          if (confirmOpen) confirmMut.mutate(confirmOpen.id);
        }}
        confirmLoading={confirmMut.isPending}
        okText={t("switch_partner.confirm_btn")}
      >
        <p>{t("switch_partner.intro")}</p>
      </Modal>
    </Page>
  );
}
