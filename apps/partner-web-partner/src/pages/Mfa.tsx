// MFA 注册/验证页 —— WebAuthn 优先，TOTP 兜底
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button, Card, Input, Tabs, Typography } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Field } from "@/components/Field";
import { useApiToast } from "@/hooks/useApiToast";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";

export function Mfa(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { showError, showSuccess } = useApiToast();
  const [otp, setOtp] = useState("");

  const enroll = useThrottledSubmit(async () => {
    if (typeof window === "undefined" || !navigator.credentials) {
      throw new Error("当前浏览器不支持 WebAuthn");
    }
    const challenge = await api.mfaEnroll();
    return api.mfaVerify({ challenge: challenge.challenge });
  });

  const verifyOtp = useThrottledSubmit(async () => api.mfaVerify({ otp }));

  const onWebauthn = async (): Promise<void> => {
    try {
      const res = await enroll.submit();
      if (res?.verified) {
        showSuccess("MFA 已启用");
        navigate("/dashboard", { replace: true });
      }
    } catch (e) {
      showError(e);
    }
  };

  const onOtp = async (): Promise<void> => {
    try {
      const res = await verifyOtp.submit();
      if (res?.verified) {
        showSuccess("MFA 已启用");
        navigate("/dashboard", { replace: true });
      }
    } catch (e) {
      showError(e);
    }
  };

  return (
    <main id="main" style={{ maxWidth: 520, margin: "64px auto" }}>
      <Card>
        <Typography.Title heading={4}>{t("auth.mfa_required")}</Typography.Title>
        <Typography.Paragraph type="tertiary">{t("auth.kyc_required")}</Typography.Paragraph>
        <Tabs type="line">
          <Tabs.TabPane tab="WebAuthn" itemKey="webauthn">
            <Button onClick={onWebauthn} loading={enroll.state.submitting} type="primary" theme="solid">
              使用安全密钥 / Touch ID 注册
            </Button>
          </Tabs.TabPane>
          <Tabs.TabPane tab="TOTP" itemKey="totp">
            <Field label="动态口令">
              <Input value={otp} onChange={setOtp} />
            </Field>
            <Button onClick={onOtp} loading={verifyOtp.state.submitting} style={{ marginTop: 8 }}>
              验证
            </Button>
          </Tabs.TabPane>
        </Tabs>
      </Card>
    </main>
  );
}
