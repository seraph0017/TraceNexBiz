// Topup —— 场景 D 充值；processing→pending_unknown(60s)→escalated(5min)→funded
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Banner, Button, Card, InputNumber, Spin } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Stepper } from "@/components/Stepper";
import { Field } from "@/components/Field";
import { startTopup, getTopupStatus } from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";

const POLL_BACKOFF = [1, 2, 4, 8, 16, 30] as const;

export function Topup(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { showError } = useApiToast();
  const [amount, setAmount] = useState<number>(100);

  const { submit, state } = useThrottledSubmit(async () => {
    if (!amount || amount < 1) {
      throw new Error(t("validation.amount_min", { n: 1 }));
    }
    const intent = await startTopup(amount * 100);
    // 跳持牌方收银台（PRD §7.5 v0.2.1 PM-HIGH-6）
    if (intent.redirect_url && typeof window !== "undefined") {
      window.location.href = intent.redirect_url;
    }
    navigate(`/topup/${intent.intent_id}`, { replace: true });
  });

  return (
    <Page title={t("topup.title")}>
      <Card style={{ maxWidth: 480 }}>
        <Field label={t("topup.amount")}>
          <InputNumber
            value={amount}
            onChange={(v) => setAmount(typeof v === "number" ? v : 0)}
            min={1}
            max={1_000_000}
            aria-label="amount"
          />
        </Field>
        <Button
          theme="solid"
          type="primary"
          loading={state.submitting}
          onClick={() =>
            submit().catch((e) => {
              if (e instanceof Error && !e.message.includes("validation")) showError(e);
            })
          }
        >
          {t("topup.submit")}
        </Button>
      </Card>
    </Page>
  );
}

export function TopupStatusPage(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const [tick, setTick] = useState(0);
  const { data, isLoading } = useQuery({
    queryKey: ["customer", "topup", id, tick],
    queryFn: () => (id ? getTopupStatus(id) : Promise.reject(new Error("no id"))),
    enabled: !!id,
    retry: 0,
  });

  useEffect(() => {
    if (!data) return;
    if (data.status === "funded" || data.status === "failed" || data.status === "escalated") return;
    const idx = Math.min(tick, POLL_BACKOFF.length - 1);
    const seconds = POLL_BACKOFF[idx] ?? 30;
    const timer = setTimeout(() => setTick((n) => n + 1), seconds * 1000);
    return () => clearTimeout(timer);
  }, [data, tick]);

  const phase = data?.status ?? "processing";
  const steps = [
    { label: t("topup.processing"), status: phase === "processing" ? "active" : "done" },
    {
      label: t("topup.pending_unknown"),
      status:
        phase === "pending_unknown" ? "active" : phase === "funded" || phase === "escalated" ? "done" : "pending",
    },
    {
      label: phase === "escalated" ? t("topup.escalated", { id: data?.saga_id }) : t("topup.funded"),
      status:
        phase === "escalated" ? "failed" : phase === "funded" ? "done" : "pending",
    },
  ] as const;

  return (
    <Page title={t("topup.title")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          {phase === "escalated" && (
            <Banner type="warning" description={t("topup.escalated", { id: data?.saga_id })} />
          )}
          {phase === "failed" && <Banner type="danger" description={t("topup.failed")} />}
          {phase === "funded" && <Banner type="success" description={t("topup.funded")} />}
          <div style={{ marginTop: 16 }}>
            <Stepper steps={steps as unknown as { label: string; status: "pending" | "active" | "done" | "failed" }[]} />
          </div>
        </Card>
      )}
    </Page>
  );
}
