// 分配额度 saga —— frontend §7.4
// 状态机：form → submitting → running → succeeded / failed_user / failed_system / pending_unknown / escalated
// idempotency-key 由 client interceptor 自动注入；server 探活 by saga_id
import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Button,
  Card,
  Descriptions,
  InputNumber,
  Spin,
  TextArea,
  Toast,
  Typography,
} from "@douyinfe/semi-ui";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Field } from "@/components/Field";
import { useTranslation } from "react-i18next";
import { z } from "zod";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { Stepper } from "@/components/Stepper";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { useApiToast } from "@/hooks/useApiToast";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";
import { useAllocateStore } from "@/stores/allocateStore";

const AllocateSchema = z.object({
  customer_id: z.number().int().positive(),
  amount: z.number().int().min(1).max(1_000_000_000),
  note: z.string().max(256).optional(),
});

const POLL_INTERVALS = [1000, 2000, 4000, 8000, 16000, 30000];

export function Allocate(): JSX.Element {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const initialCustomer = Number(params.get("customer") ?? 0);
  const { showError, showSuccess } = useApiToast();
  const qc = useQueryClient();

  const phase = useAllocateStore((s) => s.phase);
  const sagaId = useAllocateStore((s) => s.sagaId);
  const setPhase = useAllocateStore((s) => s.setPhase);
  const reset = useAllocateStore((s) => s.reset);

  const [customerId, setCustomerId] = useState<number>(initialCustomer);
  const [amount, setAmount] = useState<number>(0);
  const [note, setNote] = useState<string>("");

  const wallet = useQuery({
    queryKey: ["partner", "wallet"],
    queryFn: () => api.getWallet(),
  });

  const submission = useThrottledSubmit(async (input: api.AllocateInput) => {
    return api.startAllocate(input);
  });

  // 轮询 saga
  useEffect(() => {
    if (phase !== "running" && phase !== "pending_unknown") return;
    if (!sagaId) return;
    let cancelled = false;
    let attempt = 0;
    const startTs = Date.now();
    const tick = async (): Promise<void> => {
      try {
        const s = await api.getSagaState(sagaId);
        if (cancelled) return;
        if (s.state === "succeeded") {
          setPhase("succeeded", sagaId);
          showSuccess("分配成功");
          await qc.invalidateQueries({ queryKey: ["partner", "wallet"] });
        } else if (s.state === "failed") {
          setPhase("failed_system", sagaId, s.error_code ?? null);
        } else if (s.state === "escalated") {
          setPhase("escalated", sagaId);
        } else {
          if (Date.now() - startTs > 5 * 60_000) {
            setPhase("escalated", sagaId);
            return;
          }
          if (Date.now() - startTs > 60_000 && phase === "running") {
            setPhase("pending_unknown", sagaId);
          }
          attempt += 1;
          const wait = POLL_INTERVALS[Math.min(attempt, POLL_INTERVALS.length - 1)] ?? 30000;
          setTimeout(() => {
            if (!cancelled) void tick();
          }, wait);
        }
      } catch {
        // 5xx 不切 phase；下一轮再试
        attempt += 1;
        const wait = POLL_INTERVALS[Math.min(attempt, POLL_INTERVALS.length - 1)] ?? 30000;
        setTimeout(() => {
          if (!cancelled) void tick();
        }, wait);
      }
    };
    void tick();
    return () => {
      cancelled = true;
    };
  }, [phase, sagaId, qc, setPhase, showSuccess]);

  const onSubmit = async (): Promise<void> => {
    const parsed = AllocateSchema.safeParse({ customer_id: customerId, amount, note });
    if (!parsed.success) {
      Toast.warning({ content: parsed.error.issues[0]?.message ?? "输入不合法" });
      return;
    }
    setPhase("submitting");
    try {
      const res = await submission.submit(parsed.data);
      if (res) setPhase("running", res.saga_id);
    } catch (e) {
      // 5xx → pending_unknown（client 没有 saga_id，临时 unknown）
      // 4xx 业务错 → failed_user，回到 form
      const code =
        e && typeof e === "object" && "code" in e ? (e as { code: string }).code : "";
      if (code.startsWith("BIZ_VALID") || code === "BIZ_WALLET_INSUFFICIENT") {
        setPhase("failed_user", null, code);
        showError(e);
      } else {
        setPhase("pending_unknown");
      }
    }
  };

  const steps = useMemo(
    () => [
      {
        label: t("allocate.step_hold"),
        status:
          phase === "running" || phase === "pending_unknown"
            ? ("active" as const)
            : phase === "succeeded"
              ? ("done" as const)
              : phase === "failed_system" || phase === "escalated"
                ? ("failed" as const)
                : ("pending" as const),
      },
      {
        label: t("allocate.step_quota"),
        status:
          phase === "succeeded"
            ? ("done" as const)
            : phase === "running" || phase === "pending_unknown"
              ? ("active" as const)
              : ("pending" as const),
      },
      {
        label: t("allocate.step_reconcile"),
        status: phase === "succeeded" ? ("done" as const) : ("pending" as const),
      },
    ],
    [phase, t],
  );

  return (
    <Page
      title={t("allocate.title")}
      actions={
        phase !== "idle" && (
          <Button onClick={reset} type="tertiary">
            {t("app.cancel")}
          </Button>
        )
      }
    >
      <Card style={{ maxWidth: 560 }}>
        {phase === "idle" || phase === "failed_user" ? (
          <>
            <Descriptions
              data={[
                {
                  key: t("dashboard.available"),
                  value: wallet.data ? <MoneyDisplay fen={wallet.data.available} /> : <Spin />,
                },
              ]}
            />
            <div style={{ marginTop: 12 }}>
              <Field label="客户 ID">
                <InputNumber
                  value={customerId}
                  onChange={(v) => setCustomerId(Number(v) || 0)}
                  min={1}
                  style={{ width: "100%" }}
                />
              </Field>
              <Field label={t("allocate.amount")}>
                <InputNumber
                  value={amount / 100}
                  onChange={(v) => setAmount(Math.round(Number(v) * 100) || 0)}
                  min={0.01}
                  step={1}
                  precision={2}
                  style={{ width: "100%" }}
                />
              </Field>
              <Field label={t("allocate.note")}>
                <TextArea
                  value={note}
                  onChange={(v: string) => setNote(v)}
                  maxLength={256}
                />
              </Field>
            </div>
            <Typography.Paragraph type="warning" style={{ marginTop: 12 }}>
              {t("allocate.warning")}
            </Typography.Paragraph>
            <Button
              theme="solid"
              type="primary"
              onClick={onSubmit}
              loading={submission.state.submitting}
            >
              {t("allocate.submit")}
            </Button>
          </>
        ) : (
          <>
            <Stepper steps={steps} />
            {phase === "running" && (
              <Typography.Paragraph type="tertiary" style={{ marginTop: 12 }}>
                {t("allocate.saga_running")}
              </Typography.Paragraph>
            )}
            {phase === "pending_unknown" && (
              <Typography.Paragraph type="warning" style={{ marginTop: 12 }}>
                {t("allocate.saga_unknown")}
              </Typography.Paragraph>
            )}
            {phase === "escalated" && (
              <Typography.Paragraph type="danger" style={{ marginTop: 12 }}>
                {t("allocate.saga_escalated")} (saga_id: {sagaId})
              </Typography.Paragraph>
            )}
            {phase === "succeeded" && (
              <Button onClick={reset} type="primary" theme="solid" style={{ marginTop: 12 }}>
                完成
              </Button>
            )}
            {phase === "failed_system" && (
              <Button onClick={reset} type="warning" style={{ marginTop: 12 }}>
                重试
              </Button>
            )}
          </>
        )}
      </Card>
    </Page>
  );
}
