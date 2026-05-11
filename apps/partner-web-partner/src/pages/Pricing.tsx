// 定价 —— 模型加价（per partner override）
import { useState } from "react";
import {
  Button,
  Card,
  Empty,
  InputNumber,
  Modal,
  Spin,
  Table,
  Toast,
} from "@douyinfe/semi-ui";
import type { ColumnProps } from "@douyinfe/semi-ui/lib/es/table";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { useApiToast } from "@/hooks/useApiToast";
import { MoneyDisplay } from "@/components/MoneyDisplay";

export function Pricing(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError } = useApiToast();
  const [editing, setEditing] = useState<api.PricingRule | null>(null);
  const [bps, setBps] = useState(0);

  const { data: rules, isLoading } = useQuery({
    queryKey: ["partner", "pricing"],
    queryFn: () => api.listPricing(),
  });

  const saveMut = useMutation({
    mutationFn: () => api.updatePricing(editing!.model_id, bps),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "pricing"] });
      setEditing(null);
      Toast.success({ content: "已保存" });
    },
    onError: showError,
  });

  const cols: ColumnProps<api.PricingRule>[] = [
    { title: t("pricing.model"), dataIndex: "model_name" },
    {
      title: t("pricing.base"),
      dataIndex: "base_per_million",
      align: "right" as const,
      render: (v: number) => <MoneyDisplay fen={v} />,
    },
    {
      title: t("pricing.markup"),
      dataIndex: "markup_bps",
      align: "right" as const,
      render: (v: number) => `${v} bps (${(v / 100).toFixed(2)}%)`,
    },
    {
      title: t("pricing.effective_from"),
      dataIndex: "effective_from",
      render: (v: string) => new Date(v).toLocaleDateString(),
    },
    {
      title: "",
      render: (_: unknown, r: api.PricingRule) => (
        <Button
          size="small"
          onClick={() => {
            setEditing(r);
            setBps(r.markup_bps);
          }}
        >
          {t("app.edit")}
        </Button>
      ),
    },
  ];

  return (
    <Page title={t("pricing.title")}>
      <Card>
        {isLoading ? (
          <Spin />
        ) : !rules || rules.length === 0 ? (
          <Empty title={t("app.empty")} />
        ) : (
          <Table columns={cols} dataSource={rules} rowKey="model_id" pagination={false} />
        )}
      </Card>
      <Modal
        title={`${t("pricing.markup")} — ${editing?.model_name ?? ""}`}
        visible={editing !== null}
        onOk={() => saveMut.mutate()}
        onCancel={() => setEditing(null)}
        confirmLoading={saveMut.isPending}
      >
        <InputNumber
          value={bps}
          onChange={(v) => setBps(Number(v) || 0)}
          min={0}
          max={10_000}
          step={10}
          suffix="bps"
        />
      </Modal>
    </Page>
  );
}
