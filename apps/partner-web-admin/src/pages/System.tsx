// 业务参数 + 安全参数 + Saga force-resolve dual-control
import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Input, InputNumber, Modal, Spin, Switch, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import {
  listBizSettings,
  updateBizSetting,
  getSecuritySettings,
  updateSecuritySettings,
  forceResolveSaga,
  listEscalatedSagas,
  type BizSetting,
  type SagaCandidate,
} from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function BizSettings(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "biz-settings"],
    queryFn: listBizSettings,
  });
  const [active, setActive] = useState<string | null>(null);
  const [val, setVal] = useState("");
  const mut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) => updateBizSetting(key, value),
    onSuccess: () => {
      setActive(null);
      showSuccess(t("app.save"));
      void qc.invalidateQueries({ queryKey: ["admin", "biz-settings"] });
    },
    onError: showError,
  });
  return (
    <Page title={t("system.biz_title")} description={t("system.biz_intro")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<BizSetting>
            dataSource={data ?? []}
            rowKey="key"
            pagination={false}
            columns={[
              { title: t("system.key"), dataIndex: "key" },
              { title: t("system.value"), dataIndex: "value", render: (v: string) => <code>{v}</code> },
              { title: "Description", dataIndex: "description" },
              { title: "Updated", dataIndex: "updated_at" },
              {
                title: "",
                render: (_, r: BizSetting) => (
                  <Button
                    size="small"
                    onClick={() => {
                      setActive(r.key);
                      setVal(r.value);
                    }}
                  >
                    {t("app.edit")}
                  </Button>
                ),
              },
            ]}
          />
        </Card>
      )}
      <Modal
        title={`Edit ${active}`}
        visible={active !== null}
        onCancel={() => setActive(null)}
        onOk={() => {
          if (active !== null) mut.mutate({ key: active, value: val });
        }}
        confirmLoading={mut.isPending}
      >
        <Field label={t("system.value")}>
          <Input value={val} onChange={setVal} aria-label="value" />
        </Field>
      </Modal>
    </Page>
  );
}

export function SecuritySettings(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "security"],
    queryFn: getSecuritySettings,
  });
  const [allowlist, setAllowlist] = useState("");
  const [stepUpTtl, setStepUpTtl] = useState<number>(900);
  const [policy, setPolicy] = useState("");
  const [watermark, setWatermark] = useState(true);
  const [sessionMax, setSessionMax] = useState<number>(28800);

  useEffect(() => {
    if (data) {
      setAllowlist(data.ip_allowlist.join(","));
      setStepUpTtl(data.step_up_ttl_seconds);
      setPolicy(data.password_policy);
      setWatermark(data.watermark_enabled);
      setSessionMax(data.session_max_seconds);
    }
  }, [data]);

  const mut = useMutation({
    mutationFn: updateSecuritySettings,
    onSuccess: () => {
      showSuccess(t("app.save"));
      void qc.invalidateQueries({ queryKey: ["admin", "security"] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;
  return (
    <Page title={t("system.security_title")} description={t("system.security_intro")}>
      <Card style={{ maxWidth: 720 }}>
        <Field label={t("system.ip_allowlist")}>
          <Input value={allowlist} onChange={setAllowlist} aria-label="ip-allowlist" />
        </Field>
        <Field label={t("system.step_up_ttl")}>
          <InputNumber value={stepUpTtl} onChange={(v) => setStepUpTtl(typeof v === "number" ? v : 0)} aria-label="step-up-ttl" />
        </Field>
        <Field label={t("system.password_policy")}>
          <Input value={policy} onChange={setPolicy} aria-label="password-policy" />
        </Field>
        <Field label={t("system.watermark")}>
          <Switch checked={watermark} onChange={setWatermark} />
        </Field>
        <Field label={t("system.session_max")}>
          <InputNumber value={sessionMax} onChange={(v) => setSessionMax(typeof v === "number" ? v : 0)} aria-label="session-max" />
        </Field>
        <Button
          theme="solid"
          type="primary"
          loading={mut.isPending}
          onClick={() =>
            mut.mutate({
              ip_allowlist: allowlist.split(",").map((s) => s.trim()).filter(Boolean),
              step_up_ttl_seconds: stepUpTtl,
              password_policy: policy,
              watermark_enabled: watermark,
              session_max_seconds: sessionMax,
            })
          }
        >
          {t("app.save")}
        </Button>
      </Card>
    </Page>
  );
}

export function SagaForceResolve(): JSX.Element {
  const { t } = useTranslation();
  const { showError, showSuccess } = useApiToast();
  const { data: candidates } = useQuery({
    queryKey: ["admin", "saga-escalated"],
    queryFn: listEscalatedSagas,
  });

  const [sagaId, setSagaId] = useState("");
  const [token, setToken] = useState("");
  const [ip, setIp] = useState("");
  const [outcome, setOutcome] = useState<"resolved" | "compensated">("resolved");
  const [reason, setReason] = useState("");

  const mut = useMutation({
    mutationFn: () =>
      forceResolveSaga(sagaId, { approver_token: token, approver_ip: ip, outcome, reason: reason || undefined }),
    onSuccess: () => {
      showSuccess(t("saga_force.submit"));
      setSagaId("");
      setToken("");
      setIp("");
      setReason("");
    },
    onError: showError,
  });

  return (
    <Page title={t("saga_force.title")}>
      <Banner type="warning" description={t("saga_force.intro")} closeIcon={null} />
      <Card title={t("saga_force.candidate_list")} style={{ marginTop: 12 }}>
        <Table<SagaCandidate>
          dataSource={candidates ?? []}
          rowKey="saga_id"
          pagination={false}
          columns={[
            { title: t("saga_force.saga_id"), dataIndex: "saga_id" },
            { title: "State", dataIndex: "state", render: (v: string) => <Tag>{v}</Tag> },
            { title: "Age (s)", dataIndex: "age_seconds" },
            { title: t("saga_force.cooldown_remaining"), dataIndex: "cooldown_until", render: (v) => v ?? "ready" },
            { title: "Initiator IP", dataIndex: "initiator_ip", render: (v) => v ?? "—" },
            {
              title: "",
              render: (_, r: SagaCandidate) => (
                <Button size="small" onClick={() => setSagaId(r.saga_id)}>
                  Use
                </Button>
              ),
            },
          ]}
        />
      </Card>
      <Card title="Force-resolve" style={{ marginTop: 12, maxWidth: 720 }}>
        <Field label={t("saga_force.saga_id")}>
          <Input value={sagaId} onChange={setSagaId} aria-label="saga-id" />
        </Field>
        <Field label={t("saga_force.approver_token")}>
          <Input value={token} onChange={setToken} aria-label="approver-token" />
        </Field>
        <Field label={t("saga_force.approver_ip")}>
          <Input value={ip} onChange={setIp} aria-label="approver-ip" />
        </Field>
        <Field label={t("saga_force.outcome")}>
          <select value={outcome} onChange={(e) => setOutcome(e.target.value as "resolved" | "compensated")}>
            <option value="resolved">{t("saga_force.outcome_resolved")}</option>
            <option value="compensated">{t("saga_force.outcome_compensated")}</option>
          </select>
        </Field>
        <Field label={t("saga_force.reason")}>
          <Input value={reason} onChange={setReason} aria-label="reason" />
        </Field>
        <Button
          theme="solid"
          type="primary"
          loading={mut.isPending}
          disabled={!sagaId || !token || !ip}
          onClick={() => mut.mutate()}
        >
          {t("saga_force.submit")}
        </Button>
      </Card>
    </Page>
  );
}
