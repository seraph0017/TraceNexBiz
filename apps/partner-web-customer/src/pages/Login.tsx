// Login page —— customer
import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { Button, Card, Input } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { z } from "zod";
import { Field } from "@/components/Field";
import { useAuth } from "@/hooks/useAuth";
import { useApiToast } from "@/hooks/useApiToast";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";

const schema = z.object({
  handle: z.string().min(1),
  password: z.string().min(6),
  otp: z.string().optional(),
});

export function Login(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { login } = useAuth();
  const { showError } = useApiToast();
  const [handle, setHandle] = useState("");
  const [password, setPassword] = useState("");
  const [otp, setOtp] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  const { submit, state } = useThrottledSubmit(async () => {
    const parsed = schema.safeParse({ handle, password, otp: otp || undefined });
    if (!parsed.success) {
      const errs: Record<string, string> = {};
      parsed.error.issues.forEach((i) => {
        const k = (i.path[0] ?? "_") as string;
        if (!errs[k]) errs[k] = i.message;
      });
      setErrors(errs);
      throw new Error("validation");
    }
    setErrors({});
    await login({ site: "customer", ...parsed.data });
    navigate("/dashboard", { replace: true });
  });

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#f3f4f6",
      }}
    >
      <Card title={t("auth.login_title")} style={{ width: 380 }}>
        <Field label={t("auth.handle")} error={errors.handle}>
          <Input value={handle} onChange={setHandle} aria-label="handle" />
        </Field>
        <Field label={t("auth.password")} error={errors.password}>
          <Input mode="password" value={password} onChange={setPassword} aria-label="password" />
        </Field>
        <Field label={t("auth.otp")}>
          <Input value={otp} onChange={setOtp} aria-label="otp" />
        </Field>
        <Button
          theme="solid"
          type="primary"
          loading={state.submitting}
          onClick={() => {
            submit().catch((e) => {
              if (e instanceof Error && e.message !== "validation") showError(e);
            });
          }}
          style={{ width: "100%", marginTop: 8 }}
        >
          {t("auth.login")}
        </Button>
        <div style={{ marginTop: 12, fontSize: 13, textAlign: "center" }}>
          <Link to="/auth/forgot">{t("auth.forgot")}</Link>
        </div>
      </Card>
    </div>
  );
}
