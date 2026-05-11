// Wallet 列表 + 调账
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Card, InputNumber, Spin, Table, TextArea } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { listWallets, adminTopupWallet, type AdminWalletEntry } from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function Wallet(): JSX.Element {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "wallets", page],
    queryFn: () => listWallets({ page, limit: 50 }),
    placeholderData: (prev) => prev,
  });

  return (
    <Page title={t("wallet.title")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<AdminWalletEntry>
            dataSource={data?.items ?? []}
            rowKey="id"
            pagination={{ currentPage: page, pageSize: 50, total: data?.meta?.total ?? 0, onPageChange: setPage }}
            columns={[
              { title: "Partner", dataIndex: "partner_name" },
              { title: "Balance", dataIndex: "balance", render: (v: number) => <MoneyDisplay fen={v} /> },
              { title: "Updated", dataIndex: "updated_at" },
            ]}
          />
        </Card>
      )}
    </Page>
  );
}

export function WalletTopup(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [partnerId, setPartnerId] = useState<number>(0);
  const [amount, setAmount] = useState<number>(0);
  const [reason, setReason] = useState("");
  const mut = useMutation({
    mutationFn: adminTopupWallet,
    onSuccess: () => {
      showSuccess(t("wallet.topup"));
      void qc.invalidateQueries({ queryKey: ["admin", "wallets"] });
      setAmount(0);
      setReason("");
    },
    onError: showError,
  });
  return (
    <Page title={t("wallet.topup")}>
      <Card style={{ maxWidth: 600 }}>
        <Field label="Partner ID">
          <InputNumber value={partnerId} onChange={(v) => setPartnerId(typeof v === "number" ? v : 0)} aria-label="partner-id" />
        </Field>
        <Field label={t("wallet.topup_amount")}>
          <InputNumber value={amount} onChange={(v) => setAmount(typeof v === "number" ? v : 0)} min={0} aria-label="amount" />
        </Field>
        <Field label={t("wallet.topup_reason")}>
          <TextArea value={reason} onChange={setReason} rows={3} aria-label="reason" />
        </Field>
        <Button
          theme="solid"
          type="primary"
          loading={mut.isPending}
          disabled={!partnerId || !amount || !reason.trim()}
          onClick={() => mut.mutate({ partner_id: partnerId, amount: amount * 100, reason })}
        >
          {t("app.submit")}
        </Button>
      </Card>
    </Page>
  );
}
