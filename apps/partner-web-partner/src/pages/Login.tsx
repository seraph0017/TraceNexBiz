// 登录页 —— httpOnly cookie 鉴权；不存 token 到本地
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button, Input, Toast, Typography } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";
import { useApiToast } from "@/hooks/useApiToast";
import { Field } from "@/components/Field";

interface FormData {
  handle: string;
  password: string;
  otp?: string;
}

export function Login(): JSX.Element {
  const { t } = useTranslation();
  const { login } = useAuth();
  const navigate = useNavigate();
  const { showError } = useApiToast();
  const [values, setValues] = useState<FormData>({ handle: "", password: "", otp: "" });
  const { submit, state } = useThrottledSubmit(async (v: FormData) => {
    return login({
      site: "partner",
      handle: v.handle,
      password: v.password,
      otp: v.otp || undefined,
    });
  });

  const onSubmit = async (e: React.FormEvent): Promise<void> => {
    e.preventDefault();
    if (!values.handle || !values.password) {
      Toast.warning({ content: t("validation.required") });
      return;
    }
    try {
      const res = await submit(values);
      if (res) {
        Toast.success({ content: "登录成功" });
        navigate("/dashboard", { replace: true });
      }
    } catch (err) {
      showError(err);
    }
  };

  return (
    <main
      id="main"
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#f9fafb",
      }}
    >
      <form
        onSubmit={onSubmit}
        style={{
          background: "#fff",
          padding: 32,
          borderRadius: 8,
          width: 360,
          boxShadow: "0 2px 8px rgba(0,0,0,0.06)",
        }}
      >
        <Typography.Title heading={3} style={{ marginTop: 0 }}>
          {t("auth.login_title")}
        </Typography.Title>
        <Field label={t("auth.handle")}>
          <Input
            value={values.handle}
            onChange={(v) => setValues((s) => ({ ...s, handle: v }))}
            autoComplete="username"
          />
        </Field>
        <Field label={t("auth.password")}>
          <Input
            mode="password"
            value={values.password}
            onChange={(v) => setValues((s) => ({ ...s, password: v }))}
            autoComplete="current-password"
          />
        </Field>
        <Field label={t("auth.otp")}>
          <Input
            value={values.otp ?? ""}
            onChange={(v) => setValues((s) => ({ ...s, otp: v }))}
            autoComplete="one-time-code"
          />
        </Field>
        <Button
          theme="solid"
          type="primary"
          htmlType="submit"
          block
          loading={state.submitting}
          style={{ marginTop: 8 }}
        >
          {t("auth.login")}
        </Button>
      </form>
    </main>
  );
}
