// Staff CRUD + step-up MFA
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Descriptions, Input, Modal, Select, Spin, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { listStaff, createStaff, getStaff, disableStaff, type Staff, type StaffRole } from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

const ROLES: StaffRole[] = ["super_admin", "risk_admin", "finance_admin", "cs_admin", "kyc_reviewer"];

export function StaffList(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "staff"],
    queryFn: listStaff,
  });

  const [open, setOpen] = useState(false);
  const [username, setUsername] = useState("");
  const [pw, setPw] = useState("");
  const [role, setRole] = useState<StaffRole>("cs_admin");
  const [email, setEmail] = useState("");

  const mut = useMutation({
    mutationFn: createStaff,
    onSuccess: () => {
      setOpen(false);
      setUsername("");
      setPw("");
      setEmail("");
      showSuccess(t("staff.create"));
      void qc.invalidateQueries({ queryKey: ["admin", "staff"] });
    },
    onError: showError,
  });

  return (
    <Page title={t("staff.title")} actions={<Button theme="solid" onClick={() => setOpen(true)}>{t("staff.create")}</Button>}>
      <Banner type="warning" description={t("staff.step_up_warn")} closeIcon={null} />
      {isLoading ? (
        <Spin />
      ) : (
        <Card style={{ marginTop: 12 }}>
          <Table<Staff>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id", render: (v) => <Link to={`/staff/${v}`}>#{v}</Link> },
              { title: t("staff.username"), dataIndex: "username" },
              { title: t("staff.email"), dataIndex: "email_masked" },
              { title: t("staff.role"), dataIndex: "role", render: (v) => <Tag>{v}</Tag> },
              { title: t("staff.col_status"), dataIndex: "status" },
              { title: t("staff.col_last_login"), dataIndex: "last_login_at" },
              { title: t("staff.col_mfa"), dataIndex: "mfa_enrolled", render: (v: boolean) => (v ? "✓" : "—") },
            ]}
          />
        </Card>
      )}
      <Modal
        title={t("staff.create")}
        visible={open}
        onCancel={() => setOpen(false)}
        onOk={() =>
          mut.mutate({
            username,
            password_hash: pw,
            role,
            email: email || undefined,
          })
        }
        confirmLoading={mut.isPending}
      >
        <Field label={t("staff.username")}>
          <Input value={username} onChange={setUsername} aria-label="username" />
        </Field>
        <Field label={t("staff.password")}>
          <Input mode="password" value={pw} onChange={setPw} aria-label="password" />
        </Field>
        <Field label={t("staff.role")}>
          <Select value={role} onChange={(v) => setRole(v as StaffRole)}>
            {ROLES.map((r) => (
              <Select.Option key={r} value={r}>
                {r}
              </Select.Option>
            ))}
          </Select>
        </Field>
        <Field label={t("staff.email")}>
          <Input value={email} onChange={setEmail} aria-label="email" />
        </Field>
      </Modal>
    </Page>
  );
}

export function StaffDetail(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const sid = Number(id ?? 0);
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "staff", sid],
    queryFn: () => getStaff(sid),
    enabled: sid > 0,
  });

  const mut = useMutation({
    mutationFn: () => disableStaff(sid),
    onSuccess: () => {
      showSuccess(t("staff.disable"));
      void qc.invalidateQueries({ queryKey: ["admin", "staff", sid] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;
  return (
    <Page title={`Staff #${data.id}`} actions={data.status === "active" && <Button type="danger" onClick={() => mut.mutate()}>{t("staff.disable")}</Button>}>
      <Card>
        <Descriptions
          data={[
            { key: t("staff.username"), value: data.username },
            { key: t("staff.role"), value: data.role },
            { key: t("staff.col_status"), value: data.status },
            { key: t("staff.email"), value: data.email_masked },
            { key: t("staff.col_mfa"), value: data.mfa_enrolled ? "✓" : "—" },
            { key: t("staff.col_last_login"), value: data.last_login_at ?? "—" },
          ]}
        />
      </Card>
    </Page>
  );
}
