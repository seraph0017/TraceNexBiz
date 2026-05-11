// 新建客户 —— 引导生成 invitation code（M3-02）+ 复制 / QR
// 真正的客户注册由 storefront /apply / customer 自助走 invitation code 完成
import { useState } from "react";
import { Button, Card, Input, Select, InputNumber, Toast, Typography } from "@douyinfe/semi-ui";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { useApiToast } from "@/hooks/useApiToast";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";

export function NewCustomer(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { showError } = useApiToast();
  const [type, setType] = useState<api.Invitation["type"]>("one_time");
  const [usageLimit, setUsageLimit] = useState<number>(1);
  const [generated, setGenerated] = useState<api.Invitation | null>(null);
  const { submit, state } = useThrottledSubmit(async () =>
    api.createInvitation({ type, usage_limit: type === "limited" ? usageLimit : undefined }),
  );

  const onCopy = (): void => {
    if (!generated) return;
    navigator.clipboard.writeText(generated.code).then(
      () => Toast.success({ content: "已复制" }),
      () => Toast.error({ content: "复制失败" }),
    );
  };

  const onGenerate = async (): Promise<void> => {
    try {
      const res = await submit();
      if (res) setGenerated(res);
    } catch (e) {
      showError(e);
    }
  };

  return (
    <Page
      title={t("customers.new")}
      actions={
        <Button onClick={() => navigate("/customers")}>{t("app.back")}</Button>
      }
    >
      <Card>
        <Typography.Paragraph type="tertiary">
          为新客户生成邀请码，客户使用邀请码注册后将自动归属到您。
        </Typography.Paragraph>
        <div>
          <Field label={t("invitations.title")}>
            <Select
              value={type}
              onChange={(v) => setType(v as api.Invitation["type"])}
              optionList={[
                { value: "one_time", label: t("invitations.type_one_time") },
                { value: "permanent", label: t("invitations.type_permanent") },
                { value: "limited", label: t("invitations.type_limited") },
              ]}
              style={{ width: "100%" }}
            />
          </Field>
          {type === "limited" && (
            <Field label={t("invitations.limit")}>
              <InputNumber
                value={usageLimit}
                onChange={(v) => setUsageLimit(Number(v) || 1)}
                min={1}
                max={1000}
                style={{ width: "100%" }}
              />
            </Field>
          )}
        </div>
        <Button
          theme="solid"
          type="primary"
          onClick={onGenerate}
          loading={state.submitting}
          style={{ marginTop: 16 }}
        >
          {t("invitations.create")}
        </Button>
        {generated && (
          <Card style={{ marginTop: 16, background: "#f9fafb" }}>
            <Typography.Title heading={5}>{generated.code}</Typography.Title>
            <Input value={generated.code} readOnly style={{ marginBottom: 8 }} />
            <Button onClick={onCopy}>复制</Button>
          </Card>
        )}
      </Card>
    </Page>
  );
}
