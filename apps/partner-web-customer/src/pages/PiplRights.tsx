// PIPL 数据权利中心（场景 Q）
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Button, Card, Input, Spin, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { listPiplRequests, submitPiplRequest, type PiplRequest } from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

const KINDS: PiplRequest["kind"][] = ["export", "delete", "rectify", "portability"];

export function PiplRights(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [reason, setReason] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "pipl"],
    queryFn: listPiplRequests,
  });

  const mut = useMutation({
    mutationFn: ({ kind, reason: r }: { kind: PiplRequest["kind"]; reason?: string }) => submitPiplRequest(kind, r),
    onSuccess: () => {
      showSuccess(t("pipl.status_submitted"));
      setReason("");
      void qc.invalidateQueries({ queryKey: ["customer", "pipl"] });
    },
    onError: showError,
  });

  return (
    <Page title={t("pipl.title")} description={t("pipl.intro")}>
      <Card>
        <Field label={t("pipl.reason")}>
          <Input value={reason} onChange={setReason} aria-label="reason" />
        </Field>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          {KINDS.map((k) => (
            <Button key={k} loading={mut.isPending} onClick={() => mut.mutate({ kind: k, reason: reason || undefined })}>
              {t(`pipl.kind_${k}`)}
            </Button>
          ))}
        </div>
      </Card>
      <Card title={t("pipl.list")} style={{ marginTop: 12 }}>
        {isLoading ? (
          <Spin />
        ) : (
          <Table<PiplRequest>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id" },
              { title: t("pipl.title"), dataIndex: "kind", render: (v: string) => t(`pipl.kind_${v}`) },
              {
                title: "",
                dataIndex: "status",
                render: (v: string) => <Tag color={v === "completed" ? "green" : v === "rejected" ? "red" : "amber"}>{t(`pipl.status_${v}`)}</Tag>,
              },
              { title: "", dataIndex: "created_at" },
              {
                title: "",
                render: (_, row: PiplRequest) =>
                  row.download_url && (
                    <a href={row.download_url} download>
                      {t("pipl.download")}
                    </a>
                  ),
              },
            ]}
          />
        )}
      </Card>
    </Page>
  );
}
