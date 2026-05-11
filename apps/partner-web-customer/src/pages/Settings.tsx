// Settings + Consent
import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Card, Input, Select, Spin, Switch, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import {
  getSettings,
  updateSettings,
  listConsents,
  revokeConsent,
  type ConsentRecord,
} from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

export function Settings(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["customer", "settings"],
    queryFn: getSettings,
    staleTime: 0,
    gcTime: 0,
  });
  const [displayName, setDisplayName] = useState("");
  const [locale, setLocale] = useState<"zh-CN" | "en-US">("zh-CN");
  const [emailNotify, setEmailNotify] = useState(false);
  const [inappNotify, setInappNotify] = useState(false);

  // Sync into local state once data arrives (immutable)
  useEffect(() => {
    if (data) {
      setDisplayName(data.display_name);
      setLocale(data.preferred_locale);
      setEmailNotify(data.notify_email_enabled);
      setInappNotify(data.notify_inapp_enabled);
    }
  }, [data]);

  const mut = useMutation({
    mutationFn: updateSettings,
    onSuccess: () => {
      showSuccess(t("app.save"));
      void qc.invalidateQueries({ queryKey: ["customer", "settings"] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page title={t("settings.title")}>
      <Card title={t("settings.tab_profile")}>
        <Field label={t("settings.display_name")}>
          <Input value={displayName} onChange={setDisplayName} aria-label="display-name" />
        </Field>
        <Field label={t("settings.preferred_locale")}>
          <Select value={locale} onChange={(v) => setLocale(v as "zh-CN" | "en-US")}>
            <Select.Option value="zh-CN">简体中文</Select.Option>
            <Select.Option value="en-US">English</Select.Option>
          </Select>
        </Field>
        <Field label="Email">
          <code>{data.email_masked}</code>
        </Field>
        <Field label="Phone">
          <code>{data.phone_masked}</code>
        </Field>
      </Card>
      <Card title={t("settings.tab_notify")} style={{ marginTop: 12 }}>
        <Field label={t("settings.notify_email")}>
          <Switch checked={emailNotify} onChange={setEmailNotify} />
        </Field>
        <Field label={t("settings.notify_inapp")}>
          <Switch checked={inappNotify} onChange={setInappNotify} />
        </Field>
      </Card>
      <Button
        theme="solid"
        type="primary"
        style={{ marginTop: 12 }}
        loading={mut.isPending}
        onClick={() =>
          mut.mutate({
            display_name: displayName,
            preferred_locale: locale,
            notify_email_enabled: emailNotify,
            notify_inapp_enabled: inappNotify,
          })
        }
      >
        {t("app.save")}
      </Button>
    </Page>
  );
}

export function Consent(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["customer", "consents"],
    queryFn: listConsents,
  });

  const mut = useMutation({
    mutationFn: revokeConsent,
    onSuccess: () => {
      showSuccess(t("consent.revoke"));
      void qc.invalidateQueries({ queryKey: ["customer", "consents"] });
    },
    onError: showError,
  });

  return (
    <Page title={t("consent.title")} description={t("consent.intro")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<ConsentRecord>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: t("consent.scope"), dataIndex: "scope" },
              {
                title: "",
                dataIndex: "granted",
                render: (v: boolean) => <Tag color={v ? "green" : "grey"}>{v ? t("app.yes") : t("app.no")}</Tag>,
              },
              { title: t("consent.granted_at"), dataIndex: "signed_at" },
              { title: t("consent.version"), dataIndex: "text_version" },
              {
                title: "",
                render: (_, row: ConsentRecord) =>
                  row.granted && (
                    <Button
                      size="small"
                      type="danger"
                      onClick={() => {
                        if (window.confirm(t("consent.revoke_confirm"))) {
                          mut.mutate(row.scope);
                        }
                      }}
                    >
                      {t("consent.revoke")}
                    </Button>
                  ),
              },
            ]}
          />
        </Card>
      )}
    </Page>
  );
}
