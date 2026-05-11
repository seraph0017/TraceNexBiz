// Usage —— 调用记录 / 模型 / 计费明细 / 导出
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button, Card, DatePicker, Input, Spin, Table } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { listUsage, type UsageRow } from "@/api/customer";

export function Usage(): JSX.Element {
  const { t } = useTranslation();
  const [start, setStart] = useState<string | undefined>();
  const [end, setEnd] = useState<string | undefined>();
  const [model, setModel] = useState("");
  const [page, setPage] = useState(1);

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "usage", { start, end, model, page }],
    queryFn: () => listUsage({ start, end, model: model || undefined, page, limit: 50 }),
    placeholderData: (prev) => prev,
  });

  const exportCsv = (): void => {
    const rows = data?.items ?? [];
    const header = ["date", "model", "calls", "prompt_tokens", "completion_tokens", "cost_yuan"];
    const csv = [header.join(",")]
      .concat(
        rows.map((r) =>
          [r.date, r.model, r.calls, r.prompt_tokens, r.completion_tokens, (r.cost / 100).toFixed(2)].join(","),
        ),
      )
      .join("\n");
    const blob = new Blob(["﻿" + csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `usage-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Page title={t("usage.title")} actions={<Button onClick={exportCsv}>{t("usage.export_csv")}</Button>}>
      <Card style={{ marginBottom: 12 }}>
        <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
          <Field label={t("usage.filter_start")}>
            <DatePicker
              type="date"
              onChange={(v) =>
                setStart(typeof v === "string" ? v : v instanceof Date ? v.toISOString().slice(0, 10) : undefined)
              }
            />
          </Field>
          <Field label={t("usage.filter_end")}>
            <DatePicker
              type="date"
              onChange={(v) =>
                setEnd(typeof v === "string" ? v : v instanceof Date ? v.toISOString().slice(0, 10) : undefined)
              }
            />
          </Field>
          <Field label={t("usage.filter_model")}>
            <Input value={model} onChange={setModel} aria-label="model" />
          </Field>
        </div>
      </Card>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<UsageRow>
            dataSource={data?.items ?? []}
            rowKey={(r?: UsageRow) => (r ? `${r.date}-${r.model}` : "")}
            pagination={{
              currentPage: page,
              pageSize: 50,
              total: data?.meta?.total ?? 0,
              onPageChange: setPage,
            }}
            columns={[
              { title: t("usage.col_date"), dataIndex: "date" },
              { title: t("usage.col_model"), dataIndex: "model" },
              { title: t("usage.col_calls"), dataIndex: "calls" },
              { title: t("usage.col_prompt"), dataIndex: "prompt_tokens" },
              { title: t("usage.col_completion"), dataIndex: "completion_tokens" },
              {
                title: t("usage.col_cost"),
                dataIndex: "cost",
                render: (v: number) => <MoneyDisplay fen={v} />,
              },
            ]}
          />
        </Card>
      )}
    </Page>
  );
}
