// Settings —— 基础信息 / 联系人 / 银行卡 (PII mask) / 通知偏好
import { useState } from "react";
import {
  Banner,
  Button,
  Card,
  Descriptions,
  Input,
  Spin,
  Switch,
  Tabs,
  Toast,
} from "@douyinfe/semi-ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as api from "@/api/partner";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { useApiToast } from "@/hooks/useApiToast";

export function Settings(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["partner", "settings"],
    queryFn: () => api.getSettings(),
    // PII：禁缓存到 disk；默认 staleTime=0；不缓存
    staleTime: 0,
    gcTime: 0,
  });

  const [contactName, setContactName] = useState("");
  const [notifyEmail, setNotifyEmail] = useState(true);
  const [notifyInapp, setNotifyInapp] = useState(true);

  const saveMut = useMutation({
    mutationFn: () =>
      api.updateSettings({
        contact_name: contactName || undefined,
        notify_email_enabled: notifyEmail,
        notify_inapp_enabled: notifyInapp,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["partner", "settings"] });
      Toast.success({ content: "已保存" });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page title={t("settings.title")}>
      <Card>
        <Tabs type="line">
          <Tabs.TabPane tab={t("settings.tab_basic")} itemKey="basic">
            <Descriptions
              data={[
                { key: "contact_name", value: data.contact_name },
                { key: "phone", value: data.contact_phone_masked },
                { key: "email", value: data.contact_email_masked },
              ]}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t("settings.tab_contact")} itemKey="contact">
            <Field label="联系人">
              <Input
                value={contactName || data.contact_name}
                onChange={(v) => setContactName(v)}
                maxLength={64}
              />
            </Field>
          </Tabs.TabPane>
          <Tabs.TabPane tab={t("settings.tab_bank")} itemKey="bank">
            <Banner type="warning" description={t("settings.bank_warning")} closeIcon={null} />
            <Descriptions
              style={{ marginTop: 12 }}
              data={[{ key: "bank_account", value: data.bank_account_masked || "—" }]}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t("settings.tab_notify")} itemKey="notify">
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <label style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <Switch
                  checked={notifyEmail}
                  onChange={(v) => setNotifyEmail(Boolean(v))}
                />
                <span>{t("settings.notify_email")}</span>
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <Switch
                  checked={notifyInapp}
                  onChange={(v) => setNotifyInapp(Boolean(v))}
                />
                <span>{t("settings.notify_inapp")}</span>
              </label>
            </div>
          </Tabs.TabPane>
        </Tabs>
        <Button
          theme="solid"
          type="primary"
          onClick={() => saveMut.mutate()}
          loading={saveMut.isPending}
          style={{ marginTop: 16 }}
        >
          {t("app.save")}
        </Button>
      </Card>
    </Page>
  );
}
