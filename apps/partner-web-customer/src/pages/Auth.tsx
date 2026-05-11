// 找回密码 / 重置密码
import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { Button, Card, Input } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Field } from "@/components/Field";
import { forgotPassword, resetPassword } from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

export function Forgot(): JSX.Element {
  const { t } = useTranslation();
  const { showError, showSuccess } = useApiToast();
  const [email, setEmail] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  return (
    <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "#f3f4f6" }}>
      <Card title={t("auth.forgot_title")} style={{ width: 380 }}>
        <p style={{ color: "#6b7280", fontSize: 13 }}>{t("auth.forgot_desc")}</p>
        <Field label={t("auth.handle")}>
          <Input value={email} onChange={setEmail} aria-label="email" />
        </Field>
        <Button
          theme="solid"
          loading={busy}
          disabled={done}
          onClick={async () => {
            setBusy(true);
            try {
              await forgotPassword(email);
              setDone(true);
              showSuccess(t("auth.forgot_sent"));
            } catch (e) {
              showError(e);
            } finally {
              setBusy(false);
            }
          }}
          style={{ width: "100%" }}
        >
          {t("app.submit")}
        </Button>
        <div style={{ marginTop: 12, fontSize: 13, textAlign: "center" }}>
          <Link to="/auth/login">{t("app.back")}</Link>
        </div>
      </Card>
    </div>
  );
}

export function Reset(): JSX.Element {
  const { t } = useTranslation();
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const { showError, showSuccess } = useApiToast();
  const [pwd, setPwd] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "#f3f4f6" }}>
      <Card title={t("auth.reset_title")} style={{ width: 380 }}>
        <Field label={t("auth.reset_new")}>
          <Input mode="password" value={pwd} onChange={setPwd} aria-label="new-password" />
        </Field>
        <Button
          theme="solid"
          loading={busy}
          onClick={async () => {
            if (!token) return;
            setBusy(true);
            try {
              await resetPassword(token, pwd);
              showSuccess(t("auth.reset_done"));
              navigate("/auth/login", { replace: true });
            } catch (e) {
              showError(e);
            } finally {
              setBusy(false);
            }
          }}
          style={{ width: "100%" }}
        >
          {t("app.submit")}
        </Button>
      </Card>
    </div>
  );
}
