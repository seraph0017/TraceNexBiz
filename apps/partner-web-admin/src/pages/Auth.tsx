// admin Login + MFA enroll
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button, Card, Input } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Field } from "@/components/Field";
import { useAuth } from "@/hooks/useAuth";
import { useApiToast } from "@/hooks/useApiToast";

export function Login(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { login } = useAuth();
  const { showError } = useApiToast();
  const [handle, setHandle] = useState("");
  const [password, setPassword] = useState("");
  const [otp, setOtp] = useState("");
  const [busy, setBusy] = useState(false);

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#0f172a",
      }}
    >
      <Card title={t("auth.login_title")} style={{ width: 380 }}>
        <Field label={t("auth.handle")}>
          <Input value={handle} onChange={setHandle} aria-label="handle" />
        </Field>
        <Field label={t("auth.password")}>
          <Input mode="password" value={password} onChange={setPassword} aria-label="password" />
        </Field>
        <Field label={t("auth.otp")}>
          <Input value={otp} onChange={setOtp} aria-label="otp" />
        </Field>
        <Button
          theme="solid"
          type="primary"
          loading={busy}
          onClick={async () => {
            setBusy(true);
            try {
              await login({ site: "admin", handle, password, otp: otp || undefined });
              navigate("/partners", { replace: true });
            } catch (e) {
              showError(e);
            } finally {
              setBusy(false);
            }
          }}
          style={{ width: "100%", marginTop: 8 }}
        >
          {t("auth.login")}
        </Button>
      </Card>
    </div>
  );
}

export function MfaEnroll(): JSX.Element {
  const { t } = useTranslation();
  return (
    <div style={{ padding: 32, maxWidth: 600 }}>
      <h2>{t("auth.step_up_required")}</h2>
      <p>请在 Authenticator 中扫描二维码或输入种子码（占位 — 后端接 W1c 后接入）</p>
      <Button
        type="primary"
        onClick={() => {
          window.location.href = "/partners";
        }}
      >
        {t("app.confirm")}
      </Button>
    </div>
  );
}
