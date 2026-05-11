// 充值 —— 跳持牌方页 + 三阶段 escalated UI（frontend §7.5 v0.2.1 PM-HIGH-6）
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Banner,
  Button,
  Card,
  InputNumber,
  Spin,
  Typography,
} from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { useApiToast } from "@/hooks/useApiToast";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";

type Phase = "form" | "redirecting" | "processing" | "pending_unknown" | "escalated" | "funded" | "failed";

export function WalletTopup(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { showError } = useApiToast();
  const [yuan, setYuan] = useState(100);
  const [intent, setIntent] = useState<api.TopupIntent | null>(null);
  const [phase, setPhase] = useState<Phase>("form");

  const start = useThrottledSubmit(async (amountYuan: number) =>
    api.startTopup(Math.round(amountYuan * 100)),
  );

  // 轮询 intent 状态
  useEffect(() => {
    if (!intent) return;
    if (phase === "form" || phase === "redirecting") return;
    let cancelled = false;
    const startTs = Date.now();
    const tick = async (): Promise<void> => {
      try {
        const s = await api.getTopupIntent(intent.intent_id);
        if (cancelled) return;
        if (s.status === "funded") {
          setPhase("funded");
          return;
        }
        if (s.status === "failed") {
          setPhase("failed");
          return;
        }
        if (s.status === "escalated") {
          setPhase("escalated");
          return;
        }
        if (s.status === "pending_unknown") {
          setPhase("pending_unknown");
        }
        const interval = phase === "pending_unknown" ? 30_000 : 2_000;
        const total = Date.now() - startTs;
        if (total > 60_000 && phase === "processing") setPhase("pending_unknown");
        if (total > 5 * 60_000) {
          setPhase("escalated");
          return;
        }
        setTimeout(() => {
          if (!cancelled) void tick();
        }, interval);
      } catch {
        setTimeout(() => {
          if (!cancelled) void tick();
        }, 5_000);
      }
    };
    void tick();
    return () => {
      cancelled = true;
    };
  }, [intent, phase]);

  const onStart = async (): Promise<void> => {
    try {
      const res = await start.submit(yuan);
      if (res) {
        setIntent(res);
        setPhase("redirecting");
        // 真实环境跳转持牌方；这里 dev 模式提示
        setTimeout(() => {
          window.open(res.redirect_url, "_blank", "noopener,noreferrer");
          setPhase("processing");
        }, 800);
      }
    } catch (e) {
      showError(e);
    }
  };

  return (
    <Page
      title={t("wallet.topup")}
      actions={<Button onClick={() => navigate("/wallet")}>{t("app.back")}</Button>}
    >
      <Card style={{ maxWidth: 520 }}>
        {phase === "form" && (
          <>
            <Field label={t("wallet.topup_amount")}>
              <InputNumber
                value={yuan}
                onChange={(v) => setYuan(Number(v) || 0)}
                min={1}
                max={1_000_000}
                precision={2}
                style={{ width: "100%" }}
              />
            </Field>
            <Button
              theme="solid"
              type="primary"
              onClick={onStart}
              loading={start.state.submitting}
              style={{ marginTop: 12 }}
            >
              下一步
            </Button>
          </>
        )}
        {phase === "redirecting" && (
          <div>
            <Spin />
            <Typography.Paragraph style={{ marginTop: 12 }}>
              {t("wallet.topup_redirect")}
            </Typography.Paragraph>
          </div>
        )}
        {phase === "processing" && (
          <Banner type="info" description="支付中，确认到账..." closeIcon={null} />
        )}
        {phase === "pending_unknown" && (
          <Banner
            type="warning"
            description={t("wallet.topup_pending_unknown")}
            closeIcon={null}
          />
        )}
        {phase === "escalated" && intent && (
          <Banner
            type="danger"
            description={t("wallet.topup_escalated", { id: intent.saga_id })}
            closeIcon={null}
          />
        )}
        {phase === "funded" && (
          <>
            <Banner type="success" description={t("wallet.topup_funded")} closeIcon={null} />
            <Button onClick={() => navigate("/wallet")} style={{ marginTop: 12 }}>
              {t("app.back")}
            </Button>
          </>
        )}
        {phase === "failed" && (
          <>
            <Banner type="danger" description="支付失败，请重试" closeIcon={null} />
            <Button onClick={() => setPhase("form")} style={{ marginTop: 12 }}>
              重试
            </Button>
          </>
        )}
      </Card>
    </Page>
  );
}
